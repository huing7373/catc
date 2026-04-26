// Package auth 提供 server 端 JWT token 的签发与校验工具。
//
// # 设计约定
//
// 算法：HS256（V1 §4.1 行 180 钦定；与对称密钥简单部署 + MVP 不引入 KMS 一致）
// Claims：{user_id uint64, iat int64, exp int64}（epics.md §Story 4.4 行 1012 钦定三项）
// Secret 来源：cfg.Auth.TokenSecret（YAML / CAT_AUTH_TOKEN_SECRET env），启动时空串 fail-fast
// 默认过期：cfg.Auth.TokenExpireSec（默认 604800 = 7 天；epics.md 行 1014）
//
// # 三层映射对接
//
// 本包返回的 error 由调用方（Story 4.5 auth 中间件 / Story 4.6 auth handler）
// 用 apperror.Wrap(err, code, msg) 映射为业务错误码：
//   - Verify 任何失败（过期 / 签名错 / 格式错）→ apperror.ErrUnauthorized (1001)
//   - Sign 失败（理论不应发生，secret 已在启动 fail-fast 校验过）→ apperror.ErrServiceBusy (1009)
//
// 调用方用 errors.Is(err, auth.ErrTokenExpired) 区分日志级别（过期 INFO / 篡改 WARN）。
//
// # nil-safety / ctx
//
// JWT 签发 / 校验是纯 CPU 计算（HMAC-SHA256 < 1µs），不阻塞 IO，**不**接 ctx；
// 这是 ADR-0007 §3 "ctx 必传" 规则的合理 exception（与 stdlib crypto/hmac.New 同思路）。
// 调用方（4.5 中间件）从 c.Request.Context() 拿 ctx 后，本工具不消费 ctx；
// 调用方自己 select ctx.Done() 是错误粒度（HMAC 计算 < 1µs，cancel 毫无意义）。
//
// Sign(userID=0, ...) 不 reject（user_id=0 在数据库设计 §3.1 是合法 uint64 边界值，
// 业务 service 自己约束 user_id 必非 0；本包是 dumb 工具不做业务校验）。
// Verify("") 返 ErrTokenInvalid。
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// 安全 / 范围常量：
//
//   - minSecretBytes: HS256 最小 secret 长度（128 bit；HMAC-SHA256 推荐至少与
//     hash output size 等长，SHA256 是 32 字节但 16 字节 secret + HMAC 的
//     amplification 已可抗暴力破解）。
//   - maxExpireSec: 默认过期上限（30 天）。超过该值的 token 接近 "永不过期"，
//     违反 V1 §4.1 "默认过期 7 天" 契约语义；调用方若有特殊场景可调整本包常量
//     或在更高层做策略，但**默认实装严格 cap 30 天**。
const (
	minSecretBytes = 16
	maxExpireSec   = int64(30 * 86400)
)

// 哨兵错误：让调用方用 errors.Is 区分两类失败。
//
//   - ErrTokenInvalid: 格式错 / 签名错 / 必填字段缺失 / 算法不匹配（潜在攻击 → WARN）
//   - ErrTokenExpired: 时间已过（常规重新登录 → INFO）
//
// 调用方（4.5 auth 中间件）用 errors.Is 区分日志级别后，统一用
// apperror.Wrap(err, ErrUnauthorized, "未登录或 token 无效") 映射为业务码 1001。
var (
	ErrTokenInvalid = errors.New("token invalid")
	ErrTokenExpired = errors.New("token expired")
)

// Claims 是从有效 token 解析出的载荷。
//
// 字段语义（与 epics.md §Story 4.4 行 1012 钦定的三项一致）：
//   - UserID: 用户主键（uint64，对齐数据库设计 §3.1 BIGINT UNSIGNED）
//   - IssuedAt: 签发时间 unix epoch 秒（用于审计 / future 黑名单时间窗筛选）
//   - ExpiresAt: 过期时间 unix epoch 秒（Verify 内部对比 time.Now().Unix() 判定过期）
//
// 强类型设计：4.5 中间件 c.Set("userID", claims.UserID) 不需要 type assertion 防 panic。
type Claims struct {
	UserID    uint64 `json:"user_id"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// tokenClaims 是 jwt-v5 内部用的 claims struct，嵌入 RegisteredClaims 拿到
// jwt.Claims interface 的 6 个方法（GetExpirationTime / GetIssuedAt / GetNotBefore /
// GetIssuer / GetSubject / GetAudience）的默认实装，避免手写 boilerplate。
//
// 仅显式使用 IssuedAt / ExpiresAt 两个字段；其他 RegisteredClaims 字段（issuer /
// subject / audience / nbf / jti）留空 —— 本 story 严格对齐 epics.md "claims 含
// user_id / iat / exp 三项" 的契约，**不**擅自扩展。
//
// UserID 用 *uint64 而非 uint64：JSON unmarshal 时缺失 user_id 字段会让
// pointer 保持 nil；用 zero value (uint64=0) 无法区分 "字段缺失" vs "user_id 显式
// 为 0" 两种攻击面。Verify 解析后必须检查 UserID == nil 拒绝伪造 token，否则
// 攻击者可签 claims = {iat, exp}（漏 user_id）的 HS256 token，让 UserID
// 默认 0 通过 Verify，被调用方当成 user 0 已认证用户处理。
type tokenClaims struct {
	UserID *uint64 `json:"user_id,omitempty"`
	jwt.RegisteredClaims
}

// Signer 签发 / 校验 token。线程安全（内部仅持只读 secret bytes 与 default expire）。
//
// 多个调用方（auth 中间件 / login handler）共享同一个 *Signer 实例；
// bootstrap 期 New 一次后通过 DI 注入下游。
type Signer struct {
	secret           []byte
	defaultExpireSec int64
}

// New 构造 *Signer。
//
// 校验：
//   - secret 为空 / 长度 < 16 字节 → 返 error
//   - defaultExpireSec ≤ 0 → 返 error
//   - defaultExpireSec > 30 天（30*86400 秒）→ 返 error（防过长 token 滥用）
//
// bootstrap 期调用：cfg.Auth.TokenSecret + cfg.Auth.TokenExpireSec → 失败 os.Exit(1)
// （fail-fast 与 4.2 db.Open / 4.3 migrate.New 同模式）。
func New(secret string, defaultExpireSec int64) (*Signer, error) {
	if secret == "" {
		return nil, fmt.Errorf("auth: secret is empty (set CAT_AUTH_TOKEN_SECRET or auth.token_secret)")
	}
	if len(secret) < minSecretBytes {
		return nil, fmt.Errorf("auth: secret too short, need >= %d bytes, got %d", minSecretBytes, len(secret))
	}
	if defaultExpireSec <= 0 {
		return nil, fmt.Errorf("auth: defaultExpireSec must be > 0, got %d (configure auth.token_expire_sec)", defaultExpireSec)
	}
	if defaultExpireSec > maxExpireSec {
		return nil, fmt.Errorf("auth: defaultExpireSec %d exceeds max %d (30 days)", defaultExpireSec, maxExpireSec)
	}
	return &Signer{
		secret:           []byte(secret),
		defaultExpireSec: defaultExpireSec,
	}, nil
}

// Sign 用 *Signer 持有的 secret 签发一个新 token。
//
// expireSec ≤ 0 → 用 New 时传入的 defaultExpireSec 兜底。
// expireSec > maxExpireSec（30 天）→ 返 error；与 New 的 cap 同源约束，
// 避免调用方绕过 New 的策略直接 mint 多年期 token（V1 §4.1 默认 7 天 + 本包
// 严格 cap 30 天的 invariant 必须在所有签发路径生效）。
// userID 不做业务校验（业务方保证 != 0）；签发结果含 claims = {userID, now, now+expireSec}。
//
// 错误：
//   - expireSec 超 maxExpireSec → fmt.Errorf 描述上限
//   - HMAC-SHA256 计算失败（理论不可能，stdlib 一定成功）→ wrap fmt.Errorf
func (s *Signer) Sign(userID uint64, expireSec int64) (string, error) {
	if expireSec <= 0 {
		expireSec = s.defaultExpireSec
	}
	if expireSec > maxExpireSec {
		return "", fmt.Errorf("auth: expireSec %d exceeds max %d (30 days)", expireSec, maxExpireSec)
	}
	now := time.Now()
	uid := userID // 取地址前先 copy 到局部，避免 caller 改 userID 时数据竞争
	claims := tokenClaims{
		UserID: &uid,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(expireSec) * time.Second)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("auth: sign token: %w", err)
	}
	return signed, nil
}

// Verify 校验一个 token 字符串：解析格式 + 校验签名 + 校验过期。
//
// 任何失败映射为本包哨兵 error：
//   - jwt.ErrTokenExpired（v5 已实装 errors.Is 穿透）→ ErrTokenExpired
//   - 其他失败（格式错 / 签名错 / alg 不匹配 / claims 缺字段）→ ErrTokenInvalid
//
// 内部 keyfunc 显式 assert alg == HS256，防 alg=none / RS256 篡改攻击；
// 即使 jwt-v5 默认严格也 explicit 检查（review 阶段会问 keyfunc 是否做了 alg validation）。
//
// 校验通过后返回 typed Claims struct（不暴露 jwt-v5 内部类型）让调用方
// 不需要 import "jwt"（保持包边界清晰）。
func (s *Signer) Verify(token string) (Claims, error) {
	if token == "" {
		return Claims{}, ErrTokenInvalid
	}

	keyfunc := func(t *jwt.Token) (any, error) {
		// 显式 alg validation：拒绝 alg=none / RS256 / 其他算法（防篡改 / 攻击）
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("auth: unexpected signing method: %s", t.Method.Alg())
		}
		return s.secret, nil
	}

	var tc tokenClaims
	parsed, err := jwt.ParseWithClaims(token, &tc, keyfunc)
	if err != nil {
		// 过期错误优先识别（v5 实装 errors.Is(err, jwt.ErrTokenExpired) 穿透）
		if errors.Is(err, jwt.ErrTokenExpired) {
			return Claims{}, ErrTokenExpired
		}
		return Claims{}, ErrTokenInvalid
	}
	if !parsed.Valid {
		return Claims{}, ErrTokenInvalid
	}
	if tc.UserID == nil || tc.IssuedAt == nil || tc.ExpiresAt == nil {
		// claims 缺必填字段（理论不可能 —— 我们 Sign 时一定填了；防御构造 token 的攻击）。
		// 尤其 UserID == nil：攻击者签 {iat, exp} 但漏 user_id 时，若用 uint64 zero
		// value 会被错误识别为 user 0 已认证；必须显式拒绝。
		return Claims{}, ErrTokenInvalid
	}

	return Claims{
		UserID:    *tc.UserID,
		IssuedAt:  tc.IssuedAt.Unix(),
		ExpiresAt: tc.ExpiresAt.Unix(),
	}, nil
}
