package auth_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/pkg/auth"
)

const testSecret = "test-secret-must-be-at-least-16-bytes"

// TestSignVerify_RoundTrip_Happy 验证 Sign + Verify happy path：
// claims.user_id 正确 + iat / exp 范围合理（epics.md §Story 4.4 行 1016）。
func TestSignVerify_RoundTrip_Happy(t *testing.T) {
	t.Parallel()

	signer, err := auth.New(testSecret, 3600)
	require.NoError(t, err)

	const userID uint64 = 12345
	tok, err := signer.Sign(userID, 0) // 用默认 expireSec
	require.NoError(t, err)
	require.NotEmpty(t, tok)

	claims, err := signer.Verify(tok)
	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
	assert.NotZero(t, claims.IssuedAt)
	assert.True(t, claims.ExpiresAt > claims.IssuedAt,
		"exp (%d) must be after iat (%d)", claims.ExpiresAt, claims.IssuedAt)

	// exp - iat 应接近 default expireSec（允许 ±2s 浮动，跨秒边界 + JWT NumericDate 精度）
	delta := claims.ExpiresAt - claims.IssuedAt
	assert.InDelta(t, int64(3600), delta, 2,
		"exp-iat delta = %d, want ~3600 (default expireSec)", delta)
}

// TestSignVerify_CustomExpireSec 验证 Sign 接受显式 expireSec 覆盖默认值。
// 边界 case：调用方（4.6 login handler）显式传 cfg.Auth.TokenExpireSec 时一致性。
func TestSignVerify_CustomExpireSec(t *testing.T) {
	t.Parallel()

	signer, err := auth.New(testSecret, 3600)
	require.NoError(t, err)

	tok, err := signer.Sign(42, 7200) // 显式 2h
	require.NoError(t, err)

	claims, err := signer.Verify(tok)
	require.NoError(t, err)
	delta := claims.ExpiresAt - claims.IssuedAt
	assert.InDelta(t, int64(7200), delta, 2,
		"custom expireSec=7200 should win over default=3600; got delta=%d", delta)
}

// TestVerify_Expired_ReturnsErrTokenExpired 验证过期 token → ErrTokenExpired
// （epics.md §Story 4.4 行 1017）。
//
// 用 expireSec=1 + time.Sleep(1100ms) 跨秒边界（HS256 时间分辨率秒级）；
// 必须用 errors.Is 穿透检查（让 4.5 中间件统一用此模式区分日志级别）。
func TestVerify_Expired_ReturnsErrTokenExpired(t *testing.T) {
	t.Parallel()

	signer, err := auth.New(testSecret, 3600)
	require.NoError(t, err)

	tok, err := signer.Sign(1, 1)
	require.NoError(t, err)

	time.Sleep(1100 * time.Millisecond)

	_, err = signer.Verify(tok)
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrTokenExpired),
		"expected ErrTokenExpired, got %v", err)
	assert.False(t, errors.Is(err, auth.ErrTokenInvalid),
		"过期错误不应同时是 ErrTokenInvalid（4.5 中间件用 errors.Is 区分日志级别）")
}

// TestVerify_TamperedSignature_ReturnsErrTokenInvalid 验证签名被篡改 →
// ErrTokenInvalid（epics.md §Story 4.4 行 1018）。
//
// 篡改 token 末尾字符，HMAC 签名验证 mismatch → 返 ErrTokenInvalid。
// 这是潜在攻击场景，4.5 中间件用 errors.Is(ErrTokenInvalid) 走 WARN 级别日志。
func TestVerify_TamperedSignature_ReturnsErrTokenInvalid(t *testing.T) {
	t.Parallel()

	signer, err := auth.New(testSecret, 3600)
	require.NoError(t, err)

	tok, err := signer.Sign(1, 3600)
	require.NoError(t, err)
	require.NotEmpty(t, tok)

	// 篡改签名段最后一个字符（base64url charset 内变换 → HMAC mismatch）
	last := tok[len(tok)-1]
	swap := byte('A')
	if last == 'A' {
		swap = 'B'
	}
	tampered := tok[:len(tok)-1] + string(swap)

	_, err = signer.Verify(tampered)
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrTokenInvalid),
		"expected ErrTokenInvalid for tampered signature, got %v", err)
}

// TestVerify_DifferentSecret_ReturnsErrTokenInvalid 验证用不同 secret 验证 token →
// ErrTokenInvalid。覆盖 "签名 secret 不匹配" 这条具体路径（与篡改 token 互补）。
func TestVerify_DifferentSecret_ReturnsErrTokenInvalid(t *testing.T) {
	t.Parallel()

	signerA, err := auth.New(testSecret, 3600)
	require.NoError(t, err)
	signerB, err := auth.New("another-secret-must-be-at-least-16-bytes", 3600)
	require.NoError(t, err)

	tok, err := signerA.Sign(1, 3600)
	require.NoError(t, err)

	_, err = signerB.Verify(tok)
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrTokenInvalid),
		"expected ErrTokenInvalid for wrong secret, got %v", err)
}

// TestVerify_MalformedFormat_ReturnsErrTokenInvalid 验证格式不合法 token →
// ErrTokenInvalid（epics.md §Story 4.4 行 1019）。
//
// table-driven 覆盖多种 malformed 输入（空 / 缺段 / random / 非 base64）。
func TestVerify_MalformedFormat_ReturnsErrTokenInvalid(t *testing.T) {
	t.Parallel()

	signer, err := auth.New(testSecret, 3600)
	require.NoError(t, err)

	testCases := []struct {
		name, token string
	}{
		{"empty", ""},
		{"only-header", "eyJhbGciOiJIUzI1NiJ9"},
		{"missing-sig", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0"},
		{"random-string", "this is not a jwt at all"},
		{"two-dots-no-base64", "abc.def.ghi"},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := signer.Verify(tc.token)
			require.Error(t, err, "Verify(%q) should fail", tc.token)
			assert.True(t, errors.Is(err, auth.ErrTokenInvalid),
				"Verify(%q): expected ErrTokenInvalid, got %v", tc.token, err)
		})
	}
}

// TestVerify_AlgNone_ReturnsErrTokenInvalid 防 alg=none 攻击：
// 攻击者构造 alg=none token（无签名）尝试绕过校验。本包 keyfunc 必须显式
// 拒绝 alg != HS256（即使 jwt-v5 默认严格，仍要 explicit assert）。
//
// 这是 jwt 库使用 footgun 的经典 case，必须有 test 覆盖防止 future 退化。
func TestVerify_AlgNone_ReturnsErrTokenInvalid(t *testing.T) {
	t.Parallel()

	signer, err := auth.New(testSecret, 3600)
	require.NoError(t, err)

	// 手工构造一个 alg=none token：
	// header: {"alg":"none","typ":"JWT"} → eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0
	// payload: {"user_id":1,"iat":1000000000,"exp":9999999999} → eyJ1c2VyX2lkIjoxLCJpYXQiOjEwMDAwMDAwMDAsImV4cCI6OTk5OTk5OTk5OX0
	// 签名段：空（alg=none 规范允许）
	noneToken := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJ1c2VyX2lkIjoxLCJpYXQiOjEwMDAwMDAwMDAsImV4cCI6OTk5OTk5OTk5OX0."

	_, err = signer.Verify(noneToken)
	require.Error(t, err, "alg=none token must be rejected")
	assert.True(t, errors.Is(err, auth.ErrTokenInvalid),
		"alg=none → expected ErrTokenInvalid, got %v", err)
}

// TestNew_EmptySecret_ReturnsError 验证 secret 异常输入 fail-fast
// （epics.md §Story 4.4 行 1020）。
//
// 多个 sub-cases 覆盖：空 secret / 短 secret / 0 expire / 负 expire / 超 30 天 expire。
// 任一异常输入 → New 返 error，main.go 走 fail-fast 路径（os.Exit 1）。
func TestNew_EmptySecret_ReturnsError(t *testing.T) {
	t.Parallel()

	t.Run("empty secret", func(t *testing.T) {
		t.Parallel()
		_, err := auth.New("", 604800)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "secret",
			"error should mention 'secret', got: %v", err)
	})

	t.Run("short secret < 16 bytes", func(t *testing.T) {
		t.Parallel()
		_, err := auth.New("short", 604800)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "16",
			"error should hint length requirement '16', got: %v", err)
	})

	t.Run("zero expireSec", func(t *testing.T) {
		t.Parallel()
		_, err := auth.New(testSecret, 0)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "expire",
			"error should mention 'expire', got: %v", err)
	})

	t.Run("negative expireSec", func(t *testing.T) {
		t.Parallel()
		_, err := auth.New(testSecret, -1)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "expire",
			"error should mention 'expire', got: %v", err)
	})

	t.Run("expireSec exceeds 30 days", func(t *testing.T) {
		t.Parallel()
		_, err := auth.New(testSecret, 31*86400) // 31 天
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "max",
			"error should mention 'max' upper bound, got: %v", err)
	})
}

// TestVerify_MissingUserIDClaim_ReturnsErrTokenInvalid 验证 user_id 字段缺失 →
// ErrTokenInvalid（review round 1 P2）。
//
// 攻击面：攻击者拿到 secret 或者通过其他途径签一个 minimal HS256 token，claims
// 仅 {iat, exp} 而没有 user_id。tokenClaims.UserID 若用 uint64 类型则默认 zero
// value 0 → Verify 通过 → 调用方把 user 0 当成已认证用户处理。
//
// 修复：tokenClaims.UserID 改成 *uint64，缺失字段时 nil；Verify 显式检查 nil 拒绝。
//
// 用 jwt-v5 直接签一个不含 user_id 字段的 claims map，模拟攻击者构造的 token。
func TestVerify_MissingUserIDClaim_ReturnsErrTokenInvalid(t *testing.T) {
	t.Parallel()

	signer, err := auth.New(testSecret, 3600)
	require.NoError(t, err)

	// 直接用 jwt-v5 签一个 minimal claims（仅 iat / exp，无 user_id）。
	// 用 MapClaims 而非自定义 struct → JSON 不输出 user_id 字段。
	now := time.Now()
	mapClaims := jwt.MapClaims{
		"iat": now.Unix(),
		"exp": now.Add(1 * time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, mapClaims)
	signed, err := tok.SignedString([]byte(testSecret))
	require.NoError(t, err)

	_, err = signer.Verify(signed)
	require.Error(t, err, "缺 user_id 的 token 必须拒绝")
	assert.True(t, errors.Is(err, auth.ErrTokenInvalid),
		"missing user_id → expected ErrTokenInvalid, got %v", err)
	assert.False(t, errors.Is(err, auth.ErrTokenExpired),
		"missing claim 不应映射为 ErrTokenExpired")
}

// TestSign_ExpireSecExceedsMax_ReturnsError 验证 Sign 也 enforce maxExpireSec
// （review round 1 P3）。
//
// New 校验 defaultExpireSec ≤ 30 天，但 Sign(userID, expireSec) 若不校验，调用方
// 可绕过 New 的策略 mint 任意长 token（违反 V1 §4.1 默认 7 天 + 本包严格 cap
// 30 天的 invariant）。
//
// 修复：Sign 也用同一 maxExpireSec 常量校验；超出返 error。
func TestSign_ExpireSecExceedsMax_ReturnsError(t *testing.T) {
	t.Parallel()

	signer, err := auth.New(testSecret, 3600)
	require.NoError(t, err)

	t.Run("expireSec=31 days", func(t *testing.T) {
		t.Parallel()
		_, err := signer.Sign(1, 31*86400)
		require.Error(t, err, "Sign(expireSec=31d) 必须 reject")
		assert.Contains(t, strings.ToLower(err.Error()), "max",
			"error should mention 'max' upper bound, got: %v", err)
	})

	t.Run("expireSec=30 days exactly is allowed", func(t *testing.T) {
		t.Parallel()
		// 边界值：恰好 30 天必须接受（与 New 的边界判定一致）
		_, err := signer.Sign(1, 30*86400)
		require.NoError(t, err, "expireSec=30d 是上限边界，必须接受")
	})

	t.Run("default fallback path still works", func(t *testing.T) {
		t.Parallel()
		// expireSec=0 走 default fallback，不应被 max 校验影响
		_, err := signer.Sign(1, 0)
		require.NoError(t, err)
	})
}

// TestNew_BoundaryValid 验证恰好满足下限的输入能成功构造 Signer
// （16 字节 secret / 1 秒 expire / 30 天 expire 上限）。
func TestNew_BoundaryValid(t *testing.T) {
	t.Parallel()

	t.Run("exactly 16 bytes secret", func(t *testing.T) {
		t.Parallel()
		_, err := auth.New("abcdefghijklmnop", 3600) // 正好 16 字节
		require.NoError(t, err)
	})

	t.Run("expireSec=1", func(t *testing.T) {
		t.Parallel()
		_, err := auth.New(testSecret, 1)
		require.NoError(t, err)
	})

	t.Run("expireSec=30 days exactly", func(t *testing.T) {
		t.Parallel()
		_, err := auth.New(testSecret, 30*86400)
		require.NoError(t, err)
	})
}
