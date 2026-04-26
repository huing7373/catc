package config_test

import (
	"path/filepath"
	"testing"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/pkg/auth"
)

// TestLoad_RealLocalYAML_AuthNewSucceedsWithEnvOverride 锁定**安全 + 可工作**双不变量：
//
// 当 host 通过 `CAT_AUTH_TOKEN_SECRET` env 注入合法 secret，loader.Load 后传给
// auth.New 必须**不**报错（bootstrap 期 fail-fast 不会被触发），且 signer 能 Sign/Verify。
//
// 防护场景：未来有人把 env override 链路弄坏（loader.go 不再读 CAT_AUTH_TOKEN_SECRET
// / auth 配置块字段名漂移 / 优先级颠倒），CI 立即拦下。
//
// 实装策略：
//   - 不复制 yaml fixture（一旦 fixture 与 source-of-truth 漂移就废了），
//     直接读 `../../../configs/local.yaml`（test cwd = internal/infra/config，
//     向上 3 级到 server/，再 configs/local.yaml）。
//   - 显式 SET `CAT_AUTH_TOKEN_SECRET` 到一个测试用合法值，模拟 dev / prod
//     "用 env 注入 secret" 的工作流。
//   - 走完整链路 loader.Load → config.AuthConfig → auth.New，模拟 main.go bootstrap。
func TestLoad_RealLocalYAML_AuthNewSucceedsWithEnvOverride(t *testing.T) {
	// 模拟正常工作流：host 已经 export CAT_AUTH_TOKEN_SECRET。
	// 用语义化字符串而非真随机 hex —— test fixture 别假装是生产值。
	t.Setenv("CAT_AUTH_TOKEN_SECRET", "test-secret-32-bytes-minimum-for-test")

	yamlPath := filepath.Join("..", "..", "..", "configs", "local.yaml")

	cfg, err := config.Load(yamlPath)
	if err != nil {
		t.Fatalf("Load(%q) returned unexpected error: %v", yamlPath, err)
	}

	if cfg.Auth.TokenSecret == "" {
		t.Fatalf("cfg.Auth.TokenSecret is empty even though CAT_AUTH_TOKEN_SECRET was set; "+
			"loader env override chain broken (check loader.go envAuthTokenSecret 优先级)")
	}

	// 模拟 main.go 的 bootstrap 行为：cfg.Auth → auth.New。
	signer, err := auth.New(cfg.Auth.TokenSecret, cfg.Auth.TokenExpireSec)
	if err != nil {
		t.Fatalf("auth.New with env-injected secret returned error: %v "+
			"(token_secret length=%d, token_expire_sec=%d)",
			err, len(cfg.Auth.TokenSecret), cfg.Auth.TokenExpireSec)
	}
	if signer == nil {
		t.Fatalf("auth.New returned nil signer without error")
	}

	// 烟囱测试 signer 真能 Sign + Verify（确保 secret 链路完全通，
	// 而不只是 New 时校验长度通过）。
	const fakeUserID uint64 = 12345
	token, err := signer.Sign(fakeUserID, 0) // 0 → 用 defaultExpireSec
	if err != nil {
		t.Fatalf("signer.Sign returned error: %v", err)
	}
	claims, err := signer.Verify(token)
	if err != nil {
		t.Fatalf("signer.Verify(self-signed token) returned error: %v", err)
	}
	if claims.UserID != fakeUserID {
		t.Errorf("claims.UserID = %d, want %d", claims.UserID, fakeUserID)
	}
}

// TestLoad_RealLocalYAML_AuthNewFailsWithoutEnv 锁定**安全默认**不变量：
//
// checked-in `server/configs/local.yaml` 的 `auth.token_secret` **必须为空**。
// 当 host 未注入 `CAT_AUTH_TOKEN_SECRET` env 时，loader.Load 后 cfg.Auth.TokenSecret
// 必须为空字符串，且传给 auth.New 必须返 error → bootstrap 期 fail-fast。
//
// 防护场景：未来有人把 token_secret 改回有值（即使加 "do-not-use-in-prod" 注释），
// CI 立即拦下。原因（round 3 codex review [P1] 拦下的 trade-off）：
//
//   - LocateDefault() 和 README 都把这个 yaml 路径作为推荐启动入口；
//   - 任何 staging / prod 环境只要误用此 yaml + 漏注入 env，server 就会用
//     **公开仓库已知**的 secret 启动 → 任何看过 repo 的人都能伪造 HS256 token；
//   - 留空（""）反而**更安全**：auth.New 报错 → main.go fail-fast → 永远阻止
//     "工作但 insecure" 的 misconfiguration 上线。
//
// 关键：dev 友好（"fresh clone 一步上手"）让 README 的 export 步骤承担，**不**让
// 代码 / 配置默认值承担。
func TestLoad_RealLocalYAML_AuthNewFailsWithoutEnv(t *testing.T) {
	// 显式清空 CAT_AUTH_TOKEN_SECRET，防 host 环境覆盖让本测试看似过了。
	t.Setenv("CAT_AUTH_TOKEN_SECRET", "")

	yamlPath := filepath.Join("..", "..", "..", "configs", "local.yaml")

	cfg, err := config.Load(yamlPath)
	if err != nil {
		t.Fatalf("Load(%q) returned unexpected error: %v", yamlPath, err)
	}

	if cfg.Auth.TokenSecret != "" {
		t.Fatalf("cfg.Auth.TokenSecret = %q, want \"\"; "+
			"checked-in local.yaml must NOT carry a fallback secret. "+
			"原因 + trade-off 详见 docs/lessons/2026-04-26-checked-in-secret-must-fail-fast.md。"+
			"如果你正在 review 一个把 token_secret 填上的 PR，请回退该改动 + 让 README export 步骤承担 dev 友好。",
			cfg.Auth.TokenSecret)
	}

	// 模拟 main.go bootstrap：cfg.Auth.TokenSecret == "" → auth.New 必须返 error。
	signer, err := auth.New(cfg.Auth.TokenSecret, cfg.Auth.TokenExpireSec)
	if err == nil {
		t.Fatalf("auth.New(\"\", %d) returned nil error; "+
			"empty secret must trigger fail-fast (signer=%v)",
			cfg.Auth.TokenExpireSec, signer)
	}
	if signer != nil {
		t.Errorf("auth.New returned non-nil signer (%v) alongside error %v; "+
			"on error path signer must be nil", signer, err)
	}
}
