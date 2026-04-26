package config

import (
	"strings"
	"testing"
)

const fixturePath = "testdata/local.yaml"

func TestLoad_Happy(t *testing.T) {
	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Server.HTTPPort != 8080 {
		t.Errorf("Server.HTTPPort = %d, want 8080", cfg.Server.HTTPPort)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "info")
	}
}

func TestLoad_FileMissing(t *testing.T) {
	_, err := Load("testdata/nonexistent.yaml")
	if err == nil {
		t.Fatalf("Load returned nil error for missing file, want error")
	}
	if !strings.Contains(err.Error(), "config file not found") {
		t.Errorf("error message = %q, want substring %q", err.Error(), "config file not found")
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv(envHTTPPort, "9999")

	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Server.HTTPPort != 9999 {
		t.Errorf("Server.HTTPPort = %d, want 9999", cfg.Server.HTTPPort)
	}
}

func TestLoad_EnvInvalidInt(t *testing.T) {
	t.Setenv(envHTTPPort, "notanumber")

	_, err := Load(fixturePath)
	if err == nil {
		t.Fatalf("Load returned nil error for invalid env, want error")
	}
	if !strings.Contains(err.Error(), envHTTPPort) {
		t.Errorf("error message = %q, want substring %q", err.Error(), envHTTPPort)
	}
}

// TestLoad_MySQLDSNEnvOverride 验证 CAT_MYSQL_DSN 环境变量覆盖 YAML 的 mysql.dsn。
// 这是 staging / prod 部署注入 DB secret 的标准入口（不把密码写进 ConfigMap）。
// Story 4.2 review 补漏，CLAUDE.md "配置格式：YAML，支持环境变量覆盖" 钦定。
func TestLoad_MySQLDSNEnvOverride(t *testing.T) {
	const overrideDSN = "u:p@tcp(prod-mysql:3306)/cat?charset=utf8mb4&parseTime=true&loc=UTC"
	t.Setenv(envMySQLDSN, overrideDSN)

	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.MySQL.DSN != overrideDSN {
		t.Errorf("MySQL.DSN = %q, want %q (env override should win)", cfg.MySQL.DSN, overrideDSN)
	}
}

// TestLoad_MySQLDSNNoEnv_KeepsYAMLDefault 验证未设 CAT_MYSQL_DSN 时
// loader 不动 cfg.MySQL.DSN（保留 YAML 默认值，可能为空 → 由 db.Open 做 fail-fast）。
func TestLoad_MySQLDSNNoEnv_KeepsYAMLDefault(t *testing.T) {
	// 显式 unset 防 host 环境污染：t.Setenv 设空串再 unset 不可行，但 testdata fixture
	// 本身没 mysql 段（DSN 默认空）—— 验证空环境变量 == 空 DSN 即可。
	t.Setenv(envMySQLDSN, "")

	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.MySQL.DSN != "" {
		t.Errorf("MySQL.DSN = %q, want empty (fixture has no mysql section + env empty)", cfg.MySQL.DSN)
	}
}

// TestLoad_AuthDefaultTokenExpireSec 验证 fixture 没显式写 auth.token_expire_sec
// 时，loader 兜底为 604800（7 天，epics.md §Story 4.4 行 1014 钦定）。
// Story 4.4 引入。
func TestLoad_AuthDefaultTokenExpireSec(t *testing.T) {
	// 防 env 污染（host 环境若设了 CAT_AUTH_TOKEN_SECRET 不影响本 case 的 expire 默认值）
	t.Setenv(envAuthTokenSecret, "")

	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Auth.TokenExpireSec != 604800 {
		t.Errorf("Auth.TokenExpireSec = %d, want 604800 (fixture has no auth section → loader default)", cfg.Auth.TokenExpireSec)
	}
	// fixture 没 auth 段 → TokenSecret 也应为空（让 auth.New fail-fast）
	if cfg.Auth.TokenSecret != "" {
		t.Errorf("Auth.TokenSecret = %q, want empty (fixture has no auth section)", cfg.Auth.TokenSecret)
	}
}

// TestLoad_AuthTokenSecretEnvOverride 验证 CAT_AUTH_TOKEN_SECRET 环境变量
// 覆盖 YAML 的 auth.token_secret。Story 4.4 引入。
//
// 这是 staging / prod 部署注入 JWT signing secret 的标准入口（不把 secret
// 写进 ConfigMap / 仓库）。CLAUDE.md "配置格式：YAML，支持环境变量覆盖" 钦定；
// 与 4.2 review lesson `2026-04-26-config-env-override-and-gorm-auto-ping.md`
// "infrastructure 接入必须配齐 env override" 一致。
func TestLoad_AuthTokenSecretEnvOverride(t *testing.T) {
	const overrideSecret = "prod-secret-from-vault-must-be-at-least-16-bytes"
	t.Setenv(envAuthTokenSecret, overrideSecret)

	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Auth.TokenSecret != overrideSecret {
		t.Errorf("Auth.TokenSecret = %q, want %q (env override should win)", cfg.Auth.TokenSecret, overrideSecret)
	}
}

// TestLoad_AuthYAMLParsing 验证 YAML 含 auth: 段时正确解析（用专属 fixture）。
// Story 4.4 引入。
func TestLoad_AuthYAMLParsing(t *testing.T) {
	t.Setenv(envAuthTokenSecret, "") // 防 host 环境污染

	cfg, err := Load("testdata/auth.yaml")
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Auth.TokenSecret != "yaml-only-secret-must-be-at-least-16-bytes" {
		t.Errorf("Auth.TokenSecret = %q, want %q", cfg.Auth.TokenSecret, "yaml-only-secret-must-be-at-least-16-bytes")
	}
	if cfg.Auth.TokenExpireSec != 3600 {
		t.Errorf("Auth.TokenExpireSec = %d, want 3600 (explicit YAML value, no env override)", cfg.Auth.TokenExpireSec)
	}
}
