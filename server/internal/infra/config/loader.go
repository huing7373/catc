package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

const (
	envHTTPPort = "CAT_HTTP_PORT"
	envLogLevel = "CAT_LOG_LEVEL"
	// envMySQLDSN 是 staging / prod 注入 DB secret 的标准入口。
	// CLAUDE.md "配置格式：YAML，支持环境变量覆盖" 钦定；DSN 含密码 → 不写进
	// 提交到仓库的 YAML，部署侧用环境变量或 K8s Secret 注入。Story 4.2 review
	// 补漏，参见 docs/lessons/2026-04-26-config-env-override-and-gorm-auto-ping.md。
	envMySQLDSN = "CAT_MYSQL_DSN"

	defaultHTTPPort = 8080
	defaultLogLevel = "info"
)

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
		return nil, fmt.Errorf("config read failed: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("config parse failed: %w", err)
	}

	if v := os.Getenv(envHTTPPort); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid %s=%q: %w", envHTTPPort, v, err)
		}
		cfg.Server.HTTPPort = port
	}
	if v := os.Getenv(envLogLevel); v != "" {
		cfg.Log.Level = v
	}
	// MySQL DSN 含密码不入仓 → 部署侧通过 CAT_MYSQL_DSN 注入；空串视为
	// "不覆盖"（保留 YAML 默认或留空让 db.Open fail-fast）。
	if v := os.Getenv(envMySQLDSN); v != "" {
		cfg.MySQL.DSN = v
	}

	if cfg.Server.HTTPPort == 0 {
		cfg.Server.HTTPPort = defaultHTTPPort
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = defaultLogLevel
	}

	return &cfg, nil
}
