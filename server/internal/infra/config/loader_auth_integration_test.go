package config_test

import (
	"path/filepath"
	"testing"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/pkg/auth"
)

// TestLoad_RealLocalYAML_AuthNewSucceeds 是 fix-review round 2 引入的回归测试：
//
// 锁定"checked-in dev config 必须能直接跑"的契约 —— 仓库根的
// `server/configs/local.yaml` 用 default dev secret，loader.Load 后传给
// auth.New 必须**不**报错（bootstrap 期 fail-fast 不会被触发）。
//
// 防护场景：未来有人把 `auth.token_secret` 改回空串 / `< 16` 字节，CI 立即拦下，
// 避免重蹈 4-4 round 1 那次"checked-in local.yaml 启动 exit 1"的 review 触发。
//
// 实装策略：
//   - 不复制 yaml fixture（一旦 fixture 与 source-of-truth 漂移就废了），
//     直接读 `../../../configs/local.yaml`（test cwd = internal/infra/config，
//     向上 3 级到 server/，再 configs/local.yaml）。
//   - 显式 unset CAT_AUTH_TOKEN_SECRET 防 host 环境覆盖（让本测试只校验 yaml 默认值）。
//   - 走完整链路 loader.Load → config.AuthConfig → auth.New，模拟 main.go bootstrap。
func TestLoad_RealLocalYAML_AuthNewSucceeds(t *testing.T) {
	// 防 env 污染：host 环境若设了 CAT_AUTH_TOKEN_SECRET 会覆盖 yaml，
	// 我们要测的恰是 "yaml 默认值 + 没 env 覆盖" 这条路径。
	t.Setenv("CAT_AUTH_TOKEN_SECRET", "")

	// 路径：test cwd 是包目录 server/internal/infra/config/，
	// 向上 3 级到 server/，再进 configs/local.yaml。
	yamlPath := filepath.Join("..", "..", "..", "configs", "local.yaml")

	cfg, err := config.Load(yamlPath)
	if err != nil {
		t.Fatalf("Load(%q) returned unexpected error: %v", yamlPath, err)
	}

	if cfg.Auth.TokenSecret == "" {
		t.Fatalf("cfg.Auth.TokenSecret is empty in checked-in local.yaml; "+
			"fresh clone + ./build/catserver -config server/configs/local.yaml will fail. "+
			"See docs/lessons/2026-04-26-checked-in-config-must-boot-default.md")
	}

	// 模拟 main.go 的 bootstrap 行为：cfg.Auth → auth.New。
	signer, err := auth.New(cfg.Auth.TokenSecret, cfg.Auth.TokenExpireSec)
	if err != nil {
		t.Fatalf("auth.New with checked-in local.yaml defaults returned error: %v "+
			"(token_secret length=%d, token_expire_sec=%d)",
			err, len(cfg.Auth.TokenSecret), cfg.Auth.TokenExpireSec)
	}
	if signer == nil {
		t.Fatalf("auth.New returned nil signer without error")
	}

	// 烟囱测试一下 signer 真能 Sign + Verify（确保 secret 可用，
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
