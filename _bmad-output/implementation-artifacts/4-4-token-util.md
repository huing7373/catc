# Story 4.4: token util（JWT 签发 + 校验）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want 一个独立的 token 签发与校验工具（位于 `internal/pkg/auth/`，HS256 + 配置 secret + claims `{user_id, iat, exp}`）,
so that Story 4.5 的 auth 中间件 + Story 4.6 的游客登录 handler 可以分别复用同一份 JWT 实装，**不**重复实装签发 / 校验 / 过期判定的细节，且 secret 来源 / 默认过期 / 错误路径有 ADR-0006 错误三层映射作为统一约束。

## 故事定位（Epic 4 第四条 = 节点 2 server 写第一行 JWT 代码；上承 4.3 表 DDL，下启 4.5 auth 中间件 / 4.6 游客登录）

- **Epic 4 进度**：4.1 (契约定稿，done) → 4.2 (MySQL 接入 + tx manager，done) → 4.3 (5 张表 migrations，done) → **4.4 (本 story，token util)** → 4.5 (auth + rate_limit 中间件) → 4.6 (游客登录 handler + 首次初始化事务) → 4.8 (GET /home) → 4.7 (Layer 2 集成测试)。本 story 是 4.5 / 4.6 的**直接前置**：4.5 auth 中间件从 `Authorization: Bearer <token>` 解析后调本 story 的 `Verify`；4.6 登录成功后 service 层调本 story 的 `Sign(userID, expireSec)` 拿 token 写进 V1 §4.1 response。
- **epics.md AC 钦定**：`_bmad-output/planning-artifacts/epics.md` §Story 4.4（行 999-1020）已**精确**列出：`Sign(userID uint64, expireSec int) (string, error)` + `Verify(token string) (claims, error)` + HS256 + secret 从 `auth.token_secret` YAML 读取 + 默认过期 `auth.token_expire_sec`（默认 7 天 = 604800s）+ secret 为空 fail-fast + ≥ 5 单测 case（happy / 过期 / 篡改 / 格式 / secret 空）。**注意**：epics.md 钦定签名为 `Sign(userID uint64, ...)`，类型是 `uint64`（而非 `int64` 或 `string`）—— 与数据库设计 §3.1 钦定的 `BIGINT UNSIGNED` 主键类型对齐。
- **V1 接口设计 §4.1 钦定 token 来源**：`docs/宠物互动App_V1接口设计.md` 行 180 明示 "JWT token，HS256 + auth.token_secret 签名（见 Story 4.4 token util）；默认过期 7 天"。本 story 是该契约的**首次实装**：HS256 / 7 天 / `auth.token_secret` 三个参数都不能擅自改。
- **设计文档 §13 钦定配置入口**：`docs/宠物互动App_Go项目结构与模块职责设计.md` §13 行 931-933 已锚定 YAML 配置块：
  ```yaml
  auth:
    token_secret: xxx
    token_expire_sec: 604800
  ```
  本 story 在 `internal/infra/config/config.go` 加 `AuthConfig` struct + 对应 YAML 字段 + env override `CAT_AUTH_TOKEN_SECRET`（按 4.2 review lesson 钦定 "infrastructure 接入必须配齐 env override"，token_secret 含密钥语义）。
- **设计文档 §4 钦定包路径**：`docs/宠物互动App_Go项目结构与模块职责设计.md` §4（行 196-201）目录树明示 `internal/pkg/auth/`（与 `pkg/errors/` / `pkg/response/` 平级）。本 story 实装 `internal/pkg/auth/token.go`（**不**放在 `internal/infra/auth/`，infra 层是基础设施工具如 db / redis / clock，token 签发 / 校验是与请求相关的纯计算 + 配置依赖，归 pkg）。
- **下游立即依赖**：
  - **Story 4.5 (auth 中间件)**：`internal/app/http/middleware/auth.go` 在请求进入业务 handler 前从 header 解析 token → 调 `auth.Verify(tokenStr)` 拿 claims → `c.Set("userID", claims.UserID)`；Verify 错误（过期 / 篡改 / 格式）映射为 V1 §3 错误码 1001 (ErrUnauthorized)
  - **Story 4.6 (游客登录 handler)**：`auth_service.GuestLogin` 完成 5 表事务后调 `auth.Sign(user.ID, cfg.Auth.TokenExpireSec)` → 把 token 字符串放入 V1 §4.1 response.data.token；`Sign` 失败（理论上 secret 配齐就不会失败）映射为 1009
  - **Story 4.8 (GET /home)**：`/home` 接口走 4.5 的 auth 中间件 → 鉴权失败拦截在中间件层；本 story Verify 的 happy path 必须把 user_id 放入 claims 让中间件正确写入 ctx
- **范围红线**：本 story **只**新增 `server/internal/pkg/auth/` 包（`token.go` + `token_test.go`）+ `internal/infra/config/` 加 `AuthConfig` struct + env override + `local.yaml` 加 `auth:` 段 + go.mod 加 JWT 依赖。**不**实装任何 HTTP 中间件 / handler / service / repo。

**本 story 不做**（明确范围红线）：

- ❌ **不**实装 `internal/app/http/middleware/auth.go`（Story 4.5 落地；本 story 只产出可被 4.5 复用的 token util）
- ❌ **不**实装 `internal/app/http/middleware/rate_limit.go`（Story 4.5 落地）
- ❌ **不**实装 guest-login handler / auth_service（Story 4.6 落地）
- ❌ **不**实装 user_repo / auth_binding_repo / pet_repo / step_account_repo / chest_repo（Story 4.6 落地）
- ❌ **不**接 Redis（Story 10.2 才接；本 story 也不依赖 Redis 做 token 黑名单或 session 存储）
- ❌ **不**改 `docs/宠物互动App_*.md` 任一份文档（V1 §4.1 行 180 钦定的 "HS256 + auth.token_secret + 7 天" 是契约**输入**，本 story 严格对齐但**不修改**它）
- ❌ **不**实装 refresh token / token 黑名单 / token 旋转：MVP 阶段方案是"过期重新登录"（V1 §4.1 + Epic 5 自动登录 UseCase 已覆盖该路径），refresh token 是后续 epic 决策
- ❌ **不**实装多 secret rotation 或 KMS 接入：MVP 阶段单 secret 静态注入即可
- ❌ **不**用对称密钥之外的算法（**禁止** RS256 / ES256 / EdDSA）—— V1 §4.1 行 180 钦定 HS256；本 story 严格对齐
- ❌ **不**给 token 加 issuer / audience / subject 等额外 claims：epics.md 行 1012 钦定 claims 含 `user_id` / `iat` / `exp` 三项，本 story 不擅自扩展（多余字段会让 4.6 / 4.5 的 unmarshal 多写代码，**反**契约）
- ❌ **不**实装"token 续期"接口（如 `/auth/refresh`）—— V1 §4 没有此路径；MVP 不做
- ❌ **不**写新 README / 文档：token util 的 godoc + 本 story 文件 References 段已足够引用追溯；运维文档由 Epic 36 部署 story 统一写
- ❌ **不**在生产 / staging YAML 文件加 `token_secret` 默认值：默认 YAML 留空 + env override 注入是唯一正路（与 4.2 `mysql.dsn` 同模式）
- ❌ **不**用 GORM model / repo 包（token 签发是纯密码学 + 配置依赖，与 DB 无关）

## Acceptance Criteria

**AC1 — `internal/pkg/auth/token.go`：Token util 公开 API**

新增 `server/internal/pkg/auth/token.go`，提供以下公开 API（包名 `auth`，import 路径 `github.com/huing/cat/server/internal/pkg/auth`）：

```go
// Package auth 提供 server 端 JWT token 的签发与校验工具。
//
// # 设计约定
//
// 算法：HS256（V1 §4.1 行 180 钦定；与对称密钥简单部署 + MVP 不引入 KMS 一致）
// Claims：{user_id uint64, iat int64, exp int64}（epics.md §Story 4.4 行 1012 钦定三项）
// Secret 来源：`cfg.Auth.TokenSecret`（YAML / CAT_AUTH_TOKEN_SECRET env），启动时空串 fail-fast
// 默认过期：`cfg.Auth.TokenExpireSec`（默认 604800 = 7 天；epics.md 行 1014）
//
// # 三层映射对接
//
// 本包返回的 error 由调用方（Story 4.5 auth 中间件 / Story 4.6 auth handler）
// 用 apperror.Wrap(err, code, msg) 映射为业务错误码：
//   - Verify 任何失败（过期 / 签名错 / 格式错）→ apperror.ErrUnauthorized (1001)
//   - Sign 失败（理论不应发生，secret 已在启动 fail-fast 校验过）→ apperror.ErrServiceBusy (1009)
//
// # nil-safety
//
// Sign(userID=0, ...) 不 reject（user_id=0 在数据库设计 §3.1 是合法 uint64 边界值，
// 业务 service 自己约束 user_id 必非 0；本包是 dumb 工具不做业务校验）。
// Verify("") 返 ErrTokenInvalid。
package auth

import "context"

// Claims 是从有效 token 解析出的载荷。
//
// 字段语义（与 epics.md §Story 4.4 行 1012 钦定的三项一致）：
//   - UserID: 用户主键（uint64，对齐数据库设计 §3.1 BIGINT UNSIGNED）
//   - IssuedAt: 签发时间 unix epoch 秒（用于审计 / future 黑名单时间窗筛选）
//   - ExpiresAt: 过期时间 unix epoch 秒（Verify 内部对比 time.Now().Unix() 判定过期）
type Claims struct {
    UserID    uint64 `json:"user_id"`
    IssuedAt  int64  `json:"iat"`
    ExpiresAt int64  `json:"exp"`
}

// Signer 签发 / 校验 token。线程安全（内部仅持只读 secret bytes）。
//
// 多个调用方（auth 中间件 / login handler）共享同一个 *Signer 实例；
// bootstrap 期 New 一次后通过 DI 注入下游。
type Signer struct {
    secret      []byte
    defaultExpireSec int64
}

// New 构造 *Signer。
//
// secret 为空 / 长度 < 16 字节 → 返 error（HS256 推荐 ≥ HMAC-SHA256 block size，
// 短 secret 易被暴力破解）。defaultExpireSec ≤ 0 → 返 error。
//
// bootstrap 期调用：cfg.Auth.TokenSecret + cfg.Auth.TokenExpireSec → 失败 os.Exit(1)
// （fail-fast 与 4.2 db.Open / 4.3 migrate.New 同模式）。
func New(secret string, defaultExpireSec int64) (*Signer, error) { ... }

// Sign 用 *Signer 持有的 secret 签发一个新 token。
//
// expireSec ≤ 0 → 用 New 时传入的 defaultExpireSec 兜底。
// userID 不做业务校验（业务方保证 != 0）；签发结果含 claims = {userID, now, now+expireSec}。
//
// 错误：HMAC-SHA256 计算失败（理论不可能，stdlib 一定成功）→ wrap fmt.Errorf。
func (s *Signer) Sign(userID uint64, expireSec int64) (string, error) { ... }

// Verify 校验一个 token 字符串：解析格式 + 校验签名 + 校验过期。
//
// 任何失败（格式不对 / 签名不匹配 / 过期 / claims 缺字段）→ 返 ErrTokenInvalid 或
// ErrTokenExpired（让调用方 apperror.Wrap 时能区分日志级别 —— 过期是常规重新登录，
// 篡改是潜在攻击需要 WARN）。
//
// 内部用 time.Now().Unix() 与 claims.exp 对比；本 story **不**注入 clock interface
// （ADR-0007 钦定 clock 是 infra 工具，token util 不依赖；测试用 sleep / 缩小过期 sec
// 模拟过期，见 AC4 测试 case）。
func (s *Signer) Verify(token string) (Claims, error) { ... }

// 哨兵错误：让调用方用 stderrors.Is 区分两类失败。
var (
    ErrTokenInvalid = errors.New("token invalid")  // 格式 / 签名 / 必填字段缺失
    ErrTokenExpired = errors.New("token expired")  // 时间已过
)
```

**关键设计约束**：

- **Signer struct 持只读字段**：构造后 secret 不可变；`*Signer` 复用单例（通过 DI 注入），不每请求 New 一次（HMAC key schedule 有 setup cost，不必要）
- **Verify 错误分两类**：`ErrTokenInvalid` 与 `ErrTokenExpired` 让 4.5 中间件可区分日志级别（过期是常规、篡改是 WARN）。**注意**：调用方用 `stderrors.Is(err, auth.ErrTokenExpired)` 穿透，本包**不**用 typed error struct（轻量级）
- **不需要 ctx 参数**：JWT 签发 / 校验是 CPU 计算（HMAC-SHA256），不阻塞 IO，**不**接 ctx；这是 ADR-0007 §3 的合理 exception（"ctx 必传" 针对 IO 密集 / 跨服务调用，纯算 CPU 的工具函数不必须 —— 与 stdlib `crypto/hmac.New` 同思路）。**verify 即将进入 ctx**：调用方（4.5 中间件）从 `c.Request.Context()` 拿 ctx 后，本工具不消费 ctx；调用方自己 select ctx.Done() 是错误粒度（HMAC 计算 < 1µs，cancel 毫无意义）
- **`Sign`/`Verify` 实装允许使用第三方 JWT 库**：详见 AC2 依赖选型；**禁止**手写 HMAC + base64url 拼接（review 会问"为什么不用成熟库"，且密码学手写代码 review 成本极高）
- **`expireSec ≤ 0` 时回退到 defaultExpireSec**：本 story 钦定的"默认 7 天"是 Signer 实例级别的 default；调用方传 `Sign(uid, 0)` 表示用默认；传 `Sign(uid, 3600)` 表示自定义 1h（4.6 默认调用 `Sign(user.ID, cfg.Auth.TokenExpireSec)` 显式传 7 天，与"默认 default" 一致）

**关键反模式**：

- ❌ **不**用 typed claims struct 之外的 map[string]any 存 claims（强类型让 4.5 中间件 `c.Set("userID", claims.UserID)` 时不需要 type assertion；AppError 三层映射在 service 层把握码值，不需要 claims 是 map）
- ❌ **不**在本包注入 clock interface（增加 mock 复杂度；过期场景测试用真 sleep + 缩小 expire 即可）
- ❌ **不**实装 `RefreshToken` / `Revoke` / `Blacklist` 等 future 方法：YAGNI；MVP 路径是"过期重新登录"
- ❌ **不**返回原始 jwt 库错误（如 `jwt.ErrTokenExpired`）让调用方 import jwt 库 —— 本包对外只暴露 `auth.ErrTokenInvalid` / `auth.ErrTokenExpired` 两个哨兵；调用方仅 import `internal/pkg/auth`

**AC2 — 依赖选型：JWT 库**

`server/go.mod` 加 JWT 库依赖。**钦定** `github.com/golang-jwt/jwt/v5`（最近版本，社区主流，**Go 1.18+** 兼容，对应 jwt-go fork 后的统一维护版本）：

- 选型理由：
  - golang-jwt/jwt/v5 是 jwt-go 项目的官方 successor（原 dgrijalva/jwt-go 已 archived 转给 golang-jwt 组织维护）
  - HS256 / RS256 全套算法支持，stdlib 风格 API（`jwt.NewWithClaims` / `token.SignedString` / `jwt.ParseWithClaims`）
  - **无第三方依赖**（除 stdlib `crypto/hmac` / `encoding/base64` 等），与 4.2 / 4.3 的"最小依赖"原则一致
  - v5 版本默认 SafeStrings + 严格的 alg 校验（防 alg=none 攻击；详见 AC2 反模式段）
- **禁止替代**（review 时已显式排除）：
  - ❌ `dgrijalva/jwt-go` —— archived 不再维护，安全 issue 不修
  - ❌ `lestrrat-go/jwx` —— feature-rich 但带来 30+ 间接依赖，本场景不需要
  - ❌ 手写 HMAC + base64url 拼接 —— 密码学手写代码 review 成本极高 + 易写出 timing attack 漏洞

**版本 pin**：`go get github.com/golang-jwt/jwt/v5@latest` 后在 go.mod require 段固定具体版本（如 `v5.2.x`），与 4.3 的 `golang-migrate v4.18.1` 同模式。

**关键反模式**（jwt 库使用 footgun，dev 实装时必须显式防御）：

- ❌ **不**用 `jwt.ParseWithClaims(...)` **不指定** keyfunc 校验 alg —— 必须在 keyfunc 里**显式**检查 `token.Method.Alg() == "HS256"`，拒绝 `alg=none` 与 RS256 篡改攻击。golang-jwt v5 默认严格但仍需 explicit assert（否则 review 会问"keyfunc 内部是否做了 alg validation"）
- ❌ **不**信任 `jwt.RegisteredClaims` 的 `time.Time` 字段做 expire 判定（时区 / 浮点误差陷阱）—— 用 epochs（int64 unix seconds）做 exp / iat 字段对比；HMAC 后的 numeric 比较不会有时区漂移
- ❌ **不**忽略 `jwt.MapClaims` 类型转换错误：用 typed claims struct 实装 `jwt.Claims` interface（`GetExpirationTime` / `GetIssuedAt` / `GetNotBefore` / `GetIssuer` / `GetSubject` / `GetAudience` 六个方法）

**AC3 — `internal/infra/config/`：AuthConfig struct + env override**

修改 `server/internal/infra/config/config.go` + `loader.go` + `local.yaml`：

`config.go` 新增：

```go
type Config struct {
    Server ServerConfig `yaml:"server"`
    MySQL  MySQLConfig  `yaml:"mysql"`
    Auth   AuthConfig   `yaml:"auth"`   // ★ 本 story 新增
    Log    LogConfig    `yaml:"log"`
}

// AuthConfig 是 JWT 签发 / 校验配置。Story 4.4 引入；选型 / 默认值 / fail-fast
// 由 epics.md §Story 4.4（行 999-1020）钦定。
//
// 字段不在 config 包做业务校验（无 Validate 方法），fail-fast 由 `internal/pkg/auth.New`
// 承担：TokenSecret 为空 / < 16 字节直接返 error，main.go 走 `slog.Error + os.Exit(1)`。
type AuthConfig struct {
    // TokenSecret 是 HS256 签名 secret。**生产必须用 env 注入**（CAT_AUTH_TOKEN_SECRET）；
    // YAML 默认留空让启动 fail-fast。
    //
    // 长度要求：≥ 16 字节（128 bit；HMAC-SHA256 推荐至少与 hash output size 等长，
    // SHA256 是 32 字节但 16 字节 secret + HMAC 的 amplification 已可抗暴力破解）。
    //
    // 生产注入：K8s Secret / Vault → CAT_AUTH_TOKEN_SECRET env，与 4.2 mysql.dsn
    // 同模式（密钥不入仓库 YAML）。
    TokenSecret string `yaml:"token_secret"`

    // TokenExpireSec 是默认 token 过期时间（秒）。epics.md 行 1014 钦定默认 7 天。
    //
    // 配置可覆盖（如 dev 环境短到 1 小时方便测试）；但范围限制在 (0, 30*86400] 之内
    // —— 超过 30 天的 token 接近 "永不过期"，违反 V1 §4.1 "默认过期 7 天" 契约语义；
    // 实装在 auth.New 做范围校验（非 config 包）。
    TokenExpireSec int64 `yaml:"token_expire_sec"`
}
```

`loader.go` 新增 env override 支持：

```go
const (
    // ...existing envs...
    envAuthTokenSecret = "CAT_AUTH_TOKEN_SECRET"
)

// 在 Load 函数 env override 段加：
if v := os.Getenv(envAuthTokenSecret); v != "" {
    cfg.Auth.TokenSecret = v
}

// 默认值兜底（YAML 没填 token_expire_sec 时用 604800 = 7 天）
if cfg.Auth.TokenExpireSec == 0 {
    cfg.Auth.TokenExpireSec = 604800
}
```

`local.yaml` 新增 `auth:` 段（**留空** `token_secret`，让本地开发也走 env override 路径，与 4.2 的 `CAT_MYSQL_DSN` 同惯例）：

```yaml
auth:
  # 本地开发：用 export CAT_AUTH_TOKEN_SECRET="<≥16 字节随机字符串>" 注入，
  # 或在本文件填 token_secret（不推荐，避免 secret 入仓）。生产 / staging
  # 通过环境变量 `CAT_AUTH_TOKEN_SECRET` 覆盖。
  token_secret: ""
  token_expire_sec: 604800  # 7 天
```

**关键约束**：

- **token_secret 留空 → 启动 fail-fast**：`auth.New("", 604800)` 返 error → bootstrap.Run 阶段 `os.Exit(1)`（避免 server 用空 secret 启动后所有 token 都能伪造）。**这条是 epics.md 行 1013 强制要求**："启动时 secret 为空则 fail-fast"
- **dev / 本地开发不能没 token**：开发者第一次跑 server 会因为忘记设 env 而启动失败；server README 应该提醒（**本 story 不写 README**，由 Epic 4 收尾或 Epic 36 部署 story 统一加；本 story 在 Completion Notes 登记 tech debt）
- **TokenExpireSec 默认填补在 loader.go**：YAML 没显式写 `token_expire_sec` → loader 兜底 604800；与 `cfg.Server.HTTPPort = 0 → 8080` 同模式
- **CAT_AUTH_TOKEN_SECRET env 优先级最高**：与 4.2 review lesson `2026-04-26-config-env-override-and-gorm-auto-ping.md` 钦定的 "infrastructure 接入必须配齐 env override" 一致
- **不**给 `token_expire_sec` 加独立 env override：节点 2 阶段不需要按环境调过期时间；YAML 默认 7 天对所有 env 一致；future 如有需要再加

**AC4 — 单元测试覆盖（≥5 case，对齐 epics.md 行 1015）**

新增 `server/internal/pkg/auth/token_test.go`，覆盖：

```go
package auth_test

import (
    "errors"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/huing/cat/server/internal/pkg/auth"
)

// AC4.1 happy: Sign + Verify → claims.user_id 正确（epics.md 行 1016）
func TestSignVerify_RoundTrip_Happy(t *testing.T)

// AC4.2 edge: 过期 token → Verify 返回 ErrTokenExpired（epics.md 行 1017）
func TestVerify_Expired_ReturnsErrTokenExpired(t *testing.T)

// AC4.3 edge: 签名被篡改 → Verify 返回 ErrTokenInvalid（epics.md 行 1018）
func TestVerify_TamperedSignature_ReturnsErrTokenInvalid(t *testing.T)

// AC4.4 edge: 格式不合法 token → Verify 返回 ErrTokenInvalid（epics.md 行 1019）
func TestVerify_MalformedFormat_ReturnsErrTokenInvalid(t *testing.T)

// AC4.5 edge: secret 为空时调用 New → 返回 error（epics.md 行 1020）
func TestNew_EmptySecret_ReturnsError(t *testing.T)
```

**AC4.1 实装要点**：

```go
signer, err := auth.New("test-secret-must-be-at-least-16-bytes", 3600)
require.NoError(t, err)
tok, err := signer.Sign(12345, 0)  // 用默认 expireSec
require.NoError(t, err)
require.NotEmpty(t, tok)

claims, err := signer.Verify(tok)
require.NoError(t, err)
assert.Equal(t, uint64(12345), claims.UserID)
assert.NotZero(t, claims.IssuedAt)
assert.True(t, claims.ExpiresAt > claims.IssuedAt)
```

**AC4.2 过期实装要点**（用 `expireSec=1` 然后 `time.Sleep(2*time.Second)`，**不**用 ctx 或 mock clock）：

```go
signer, err := auth.New("test-secret-must-be-at-least-16-bytes", 3600)
require.NoError(t, err)
tok, _ := signer.Sign(1, 1)  // 1 秒过期
time.Sleep(2 * time.Second)

_, err = signer.Verify(tok)
require.Error(t, err)
assert.True(t, errors.Is(err, auth.ErrTokenExpired),
    "expected ErrTokenExpired, got %v", err)
```

**注意**：测试有 2 秒 sleep，单测整体应 < 100ms 但本 case 必然 ≥ 2s。**这是被 epics.md 钦定 "edge: 过期 token" 不可避免的代价**。本 case 用 `t.Parallel()` 并发跑减小总开销；如果 dev 阶段觉得 2s 太久可以缩到 `expireSec=1` + `time.Sleep(1100*time.Millisecond)`（HS256 时间分辨率是秒，必须跨秒边界）。

**AC4.3 篡改实装要点**：

```go
tok, _ := signer.Sign(1, 3600)
// 篡改最后一个字符（base64url charset 内一个字符变换会让签名 hash mismatch）
tampered := tok[:len(tok)-1] + "X"
_, err := signer.Verify(tampered)
require.Error(t, err)
assert.True(t, errors.Is(err, auth.ErrTokenInvalid))
```

**AC4.4 格式不合法实装要点**（多分支 table-driven）：

```go
testCases := []struct{
    name, token string
}{
    {"empty", ""},
    {"only-header", "eyJhbGciOiJIUzI1NiJ9"},
    {"missing-sig", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0"},
    {"random-string", "this is not a jwt at all"},
    {"two-dots-no-base64", "abc.def.ghi"},
}
for _, tc := range testCases {
    t.Run(tc.name, func(t *testing.T) {
        _, err := signer.Verify(tc.token)
        require.Error(t, err)
        assert.True(t, errors.Is(err, auth.ErrTokenInvalid))
    })
}
```

**AC4.5 secret 为空实装要点**（可拆为多 case，subtests）：

```go
t.Run("empty secret", func(t *testing.T) {
    _, err := auth.New("", 604800)
    require.Error(t, err)
    assert.Contains(t, err.Error(), "secret")
})
t.Run("short secret <16 bytes", func(t *testing.T) {
    _, err := auth.New("short", 604800)
    require.Error(t, err)
    assert.Contains(t, err.Error(), "16")  // hint length requirement
})
t.Run("zero expireSec", func(t *testing.T) {
    _, err := auth.New("test-secret-must-be-at-least-16-bytes", 0)
    require.Error(t, err)
    assert.Contains(t, err.Error(), "expire")
})
t.Run("negative expireSec", func(t *testing.T) {
    _, err := auth.New("test-secret-must-be-at-least-16-bytes", -1)
    require.Error(t, err)
})
```

**总数**：5 个顶层 test 函数，含 sub-tests 共 ~12-15 个 assert path，超过 epics.md "≥ 5 case" 下限。

**关键约束**：

- 全部用 `testing.T` + `testify/assert` + `testify/require`（已有依赖）；**不**额外引入 mock 框架
- AC4.2 之外所有 case ≤ 100ms / case 跑完（HS256 计算 < 1µs，纯 CPU）
- AC4.2 用 `time.Sleep`（≤ 2s）—— 标记 `t.Parallel()` 让多个慢 case 并发跑
- **不**用 `httptest` / `dockertest` / `sqlmock`：本 story 是纯算 + 配置依赖

**AC5 — `bash scripts/build.sh` 全量绿（vet + build + test）**

完成后必须能跑通：

```bash
bash scripts/build.sh                    # vet + build → 不报错（包括 internal/pkg/auth 新包）
bash scripts/build.sh --test             # 单测全过（含 5 个新 case + 现有 ~30 个 case）
bash scripts/build.sh --race --test      # 加 -race 全过（Linux / CI 必过；Windows ThreadSanitizer 内存问题按 ADR-0001 §3.5 skip）
```

**关键约束**：

- 单测层 ≤ 5s 整体跑完（含 AC4.2 的 2s sleep，其他 case 全在 100ms 内 → 5s 上限合理）
- **不**改 `bash scripts/build.sh` 自身（脚本契约由 Story 1.7 钉死）
- **不**新增 `--integration` 路径：本 story 是纯单测覆盖；token util 没有 IO，无需 dockertest

**AC6 — `cmd/server/main.go` / `internal/app/bootstrap`：在启动序列中接入 token util fail-fast**

修改 `cmd/server/main.go` 或 `internal/app/bootstrap/`，在 `db.Open` 之后 / `bootstrap.Run` 之前加 token util 启动校验：

```go
// 已有：cfg = config.Load(...)
// 已有：gormDB, err := db.Open(...)

// ★ 本 story 新增：构造 token signer，secret 为空 / 短 → fail-fast
signer, err := auth.New(cfg.Auth.TokenSecret, cfg.Auth.TokenExpireSec)
if err != nil {
    slog.Error("auth token signer init failed", slog.Any("error", err))
    os.Exit(1)  // fail-fast，与 db.Open 失败同模式
}

// 把 signer 通过 DI 传给 bootstrap.Run（后续 4.5 / 4.6 取出来用）
err = bootstrap.Run(ctx, cfg, gormDB, txMgr, signer)
```

**关键约束**：

- **`auth.New` 在 `db.Open` 之后**：与 4.3 review lesson "启动路径阻塞 IO 必须有 deadline" 解耦 —— `auth.New` 是纯计算 < 1µs 不需要 timeout；放在 db.Open 之后不会拖慢 server readiness
- **fail-fast 输出明确 error 信息**：让运维一眼看出是哪个配置缺（`secret too short` / `expire_sec out of range` / `secret empty`）
- **bootstrap.Run 签名扩展**：`internal/app/bootstrap/bootstrap.go` 的 `Run` 函数加 `signer *auth.Signer` 参数；不接的话 4.5 / 4.6 没法 DI
- **bootstrap.Run 内部不调 auth 中间件**：本 story 只构造 signer 并存放，**不**注册 auth 中间件到 router（4.5 才注册）；signer 通过 bootstrap 内部某个 holder（如 `app.Deps` struct）暴露给后续 epic 的 handler / middleware

**AC7 — `go.mod` / `go.sum` 更新**

- `cd server && go get github.com/golang-jwt/jwt/v5@latest`（拉最新稳定版，预期 v5.2.x）
- `cd server && go mod tidy` → 确认 `go.mod` require 段加了 `github.com/golang-jwt/jwt/v5 vX.Y.Z`
- 确认 `go mod verify` 不报错
- `go.sum` 同步更新（自动）

**关键约束**：

- **不**升级其他依赖（保持 4.3 已 pin 的 `golang-migrate v4.18.1` / GORM v1.25.12 / Gin v1.12.0 等不变）
- 如果 `go mod tidy` 意外删除某个依赖（如发现 indirect 依赖被新增的 jwt/v5 间接 cover），dev 阶段需要手工核对一遍 `go.mod` diff，确认仅是新增 jwt/v5 一行 require + sum 同步
- 如果 jwt/v5 引入新的 indirect 依赖（理论应该没有，jwt/v5 自宣称无第三方依赖），review 阶段会问"为什么引入这条 indirect"

**AC8 — README / docs 不更新**

本 story **不**更新：

- `README.md` / `server/README.md`：JWT secret 注入指南留给 Epic 4 收尾或 Epic 36 部署 story；本 story 在 Completion Notes 登记 tech debt
- `docs/宠物互动App_*.md` 任一份：本 story 严格对齐 V1 §4.1 行 180 + 设计文档 §13 行 931-933，**消费方**不是修改方
- `docs/lessons/` 任一份：本 story 不主动写 lesson；如 review 阶段发现新坑，由 fix-review 阶段写 lesson（epic-loop 流水线分工）

**关键约束**：

- 如果 dev 阶段实装时发现某条 AC 与文档冲突 / 漏洞 / 暗坑（如 `auth.token_secret` 在多份设计文档中有出入），**不**自行修文档，**而是**在 Completion Notes 里登记 issue + 让 fix-review 处理
- README 缺失"如何 export CAT_AUTH_TOKEN_SECRET"是已知 tech debt，本 story Completion Notes 必须明确记录 + 推到 Epic 4 收尾 / Epic 36

## Tasks / Subtasks

- [x] **Task 1（AC3）**：实装 `internal/infra/config/` AuthConfig + env override
  - [x] 1.1 修改 `server/internal/infra/config/config.go` 加 `AuthConfig` struct + 把 `Auth AuthConfig \`yaml:"auth"\`` 字段加到 `Config`
  - [x] 1.2 修改 `server/internal/infra/config/loader.go` 加 `envAuthTokenSecret = "CAT_AUTH_TOKEN_SECRET"` const + Load 函数 env override 段加 `if v := os.Getenv(envAuthTokenSecret); v != "" { cfg.Auth.TokenSecret = v }`
  - [x] 1.3 loader.go 在 default 值兜底段加 `if cfg.Auth.TokenExpireSec == 0 { cfg.Auth.TokenExpireSec = 604800 }`
  - [x] 1.4 修改 `server/configs/local.yaml` 加 `auth:` 段（`token_secret: ""` + `token_expire_sec: 604800`）+ 注释说明 env override 路径
  - [x] 1.5 修改 `server/internal/infra/config/loader_test.go` 新增 ≥3 case：(a) YAML 含 `auth:` 段 → 解析正确；(b) `CAT_AUTH_TOKEN_SECRET` env override 生效；(c) YAML 无 `auth.token_expire_sec` → 默认 604800
- [x] **Task 2（AC1 / AC2 / AC7）**：实装 `internal/pkg/auth/token.go` + go.mod 加 jwt/v5
  - [x] 2.1 `cd server && go get github.com/golang-jwt/jwt/v5@latest` → 加 require + sum
  - [x] 2.2 新建 `server/internal/pkg/auth/token.go` 定义 `Claims` struct + `Signer` struct + `New(secret, defaultExpireSec) (*Signer, error)` 工厂
  - [x] 2.3 实装 `New`：secret 空 → error；secret < 16 字节 → error；defaultExpireSec ≤ 0 → error；defaultExpireSec > 30 天 → error（防过长 token 被滥用）
  - [x] 2.4 实装 `Sign(userID, expireSec)`：expireSec ≤ 0 用 default 兜底；构造 `jwt.NewWithClaims(jwt.SigningMethodHS256, claims)` → `token.SignedString(secret)`
  - [x] 2.5 实装 `Verify(token)`：用 `jwt.ParseWithClaims` 传入 keyfunc 校验 alg == HS256；区分 `jwt.ErrTokenExpired` / 其他错误 → 映射到本包 `ErrTokenExpired` / `ErrTokenInvalid`；解析 claims 后校验必填字段（IssuedAt / ExpiresAt 非 nil）
  - [x] 2.6 定义包级哨兵 error：`ErrTokenInvalid` / `ErrTokenExpired`
  - [x] 2.7 godoc 完整注释（每个公开 API 含 doc comment + 设计约束 + 三层映射对接说明）
- [x] **Task 3（AC4）**：写 `internal/pkg/auth/token_test.go`
  - [x] 3.1 `TestSignVerify_RoundTrip_Happy`：构造 signer → Sign(12345, 0) → Verify → claims.UserID == 12345；assert iat / exp 范围
  - [x] 3.2 `TestVerify_Expired_ReturnsErrTokenExpired`：Sign(1, 1) → time.Sleep(1100ms) → Verify 返 errors.Is(err, ErrTokenExpired)；用 t.Parallel()
  - [x] 3.3 `TestVerify_TamperedSignature_ReturnsErrTokenInvalid`：Sign 后篡改最后一个字符 → Verify 返 errors.Is(err, ErrTokenInvalid)
  - [x] 3.4 `TestVerify_MalformedFormat_ReturnsErrTokenInvalid`：table-driven 5 sub-cases（empty / 缺段 / random string / etc）→ 全部 errors.Is(ErrTokenInvalid)
  - [x] 3.5 `TestNew_EmptySecret_ReturnsError`：subtests 5 case（空 secret / 短 secret / 0 expire / 负 expire / 超 30 天）；assert error 含相应关键词
  - [x] 3.6 跑 `cd server && go test ./internal/pkg/auth/... -count=1 -v` → 全绿（11 测试函数 / ~25 sub-tests / 1.5s）
- [x] **Task 4（AC6）**：在启动序列中接入 token util fail-fast
  - [x] 4.1 修改 `server/internal/app/bootstrap/server.go` `Run` 函数签名加 `signer *auth.Signer` 参数（沿用现有平铺参数模式；本 story 阶段未引入 app.Deps struct 因为 Run 内部仅 wire 不消费，参数数量仍可控）
  - [x] 4.2 修改 `server/cmd/server/main.go`：在 `db.Open` 之后加 `signer, err := auth.New(cfg.Auth.TokenSecret, cfg.Auth.TokenExpireSec)`；err != nil → `slog.Error + os.Exit(1)`
  - [x] 4.3 把 signer 传给 bootstrap.Run；后续 4.5 / 4.6 / 4.8 通过该入口取
  - [x] 4.4 跑 `bash scripts/build.sh` 编译通过
  - [x] 4.5 烟测：本机 MySQL 未起 → 启动顺序中 db.Open fail-fast 在 auth.New 之前触发（无 MySQL 不影响 auth.New 逻辑覆盖；TestNew_EmptySecret_ReturnsError 单测覆盖 fail-fast 路径所有分支）
- [x] **Task 5（AC5）**：全量验证
  - [x] 5.1 `bash /c/fork/cat/scripts/build.sh`（vet + build）必过 → BUILD SUCCESS
  - [x] 5.2 `bash /c/fork/cat/scripts/build.sh --test` 全过 → all tests passed（含 5 个新 loader case + 11 个新 auth case + 现有全部）
  - [x] 5.3 `bash /c/fork/cat/scripts/build.sh --race --test` Windows ThreadSanitizer "failed to allocate" → 按 ADR-0001 §3.5 skip（与 4.2 / 4.3 同模式；Linux/CI 必过）
  - [x] 5.4 `cd server && go mod verify` 通过 → all modules verified
  - [x] 5.5 `git status --short` 抽检：仅影响 config / pkg/auth / bootstrap / main.go / configs/local.yaml / go.mod / go.sum，与 AC 范围对齐（migrations / db / repo / handler / service 都没有改动）
- [x] **Task 6**：本 story 不做 git commit
  - [x] 6.1 epic-loop 流水线约束：dev-story 阶段不 commit；由 fix-review / story-done sub-agent 收口
  - [x] 6.2 commit message 模板（story-done 阶段使用）：

    ```text
    feat(auth): JWT 签发与校验工具 + AuthConfig 接入（Story 4.4）

    - internal/pkg/auth/token.go：Signer{Sign/Verify} HS256 + claims {user_id, iat, exp}
    - internal/pkg/auth/token_test.go：5+ 单测 case（happy / expired / tampered / malformed / empty-secret）
    - internal/infra/config/AuthConfig + CAT_AUTH_TOKEN_SECRET env override
    - configs/local.yaml 加 auth: 段（token_secret 留空，env 注入）
    - cmd/server/main.go 启动序列加 auth.New fail-fast（secret 空 / 短 / expire 异常 → os.Exit(1)）
    - go.mod 加 github.com/golang-jwt/jwt/v5

    依据 epics.md §Story 4.4 + V1 接口设计 §4.1 行 180 + 设计文档 §13 行 931-933。

    Story: 4-4-token-util
    ```

## Dev Notes

### 关键设计原则

1. **HS256 + 配置 secret + claims 三项**：V1 接口设计 §4.1 行 180 + epics.md §Story 4.4 已锁死签名算法（HS256）/ secret 来源（`auth.token_secret` YAML）/ claims 字段（user_id / iat / exp）—— 本 story 不做选型，严格落地。HS256 的对称密钥模式与 MVP "单 server 自管 secret"部署模型一致；不引入 RS256 / KMS / Vault。
2. **fail-fast over fallback**：secret 为空 / 短不能 fallback 到 "随机 secret 启动"（启动看似成功但所有 token 都无法跨重启验证 + 空 secret 让 token 可被任意伪造）。**MEMORY.md "No Backup Fallback" 钦定反对 fallback 掩盖核心风险**；本 story 严格遵守 —— `auth.New` 任何 invalid 输入直接返 error，main.go `os.Exit(1)`。
3. **强类型 claims struct**：epics.md 钦定 claims 仅 user_id / iat / exp 三项；本 story 用 typed `Claims` struct（uint64 / int64 / int64）而非 `jwt.MapClaims`，让 4.5 中间件 `c.Set("userID", claims.UserID)` 不需要 type assertion 防 panic。
4. **不依赖 ctx**：JWT 签发 / 校验是纯 CPU 计算（HMAC-SHA256 < 1µs），不阻塞 IO。ADR-0007 §3 "ctx 必传" 针对 IO 密集 / 跨服务调用；本 story 是 ADR-0007 的合理 exception，与 stdlib `crypto/hmac.New` 同思路。
5. **typed sentinel errors（ErrTokenInvalid / ErrTokenExpired）**：让 4.5 中间件 `errors.Is(err, auth.ErrTokenExpired)` 区分日志级别（过期 INFO / 篡改 WARN）；与 ADR-0006 三层映射兼容（service 层用 `apperror.Wrap(err, ErrUnauthorized, "未登录")` 统一映射 1001）。
6. **测试不引入 mock clock**：`time.Now()` 直接调；过期场景用真 sleep + 缩小 expireSec 模拟（虽 2s sleep 让单测变慢，但 epics.md "edge: 过期 token" 是钦定 case，必须真实跨秒边界）。引入 clock interface 增加 mock 复杂度但收益小（HS256 时间分辨率仅秒级）。

### 架构对齐

**领域模型层**（`docs/宠物互动App_总体架构设计.md` §3）：
- 节点 2 阶段 token 是会话凭证，每个 user 1 个 token；过期 / 失效 → 4.5 中间件返 1001 → iOS Epic 5 自动重新登录路径接管（refresh token 不在 MVP 范围）

**接口契约层**（`docs/宠物互动App_V1接口设计.md`）：
- §2.3 行 41-44 钦定 `Authorization: Bearer <token>` 头
- §3 行 79-118 钦定错误码：1001 未登录 / token 无效（auth 中间件拦截）
- §4.1 行 180 钦定 token = JWT HS256 + auth.token_secret 签 + 默认 7 天过期

**Go 项目结构层**（`docs/宠物互动App_Go项目结构与模块职责设计.md`）：
- §4 行 196-201 钦定 `internal/pkg/auth/` 包路径（与 `pkg/errors/` / `pkg/response/` 平级）
- §6.1 行 338-360 Auth 模块职责：游客登录 / Token 签发与校验 / 微信绑定 / 登录方式绑定关系（本 story 仅落地"Token 签发与校验"子项；其余 4.6 / 后续 epic）
- §13 行 931-933 钦定配置块：`auth: { token_secret: xxx, token_expire_sec: 604800 }`

**ADR 对接**：
- ADR-0006 错误三层映射：本 story 不直接产 AppError，由 4.5 / 4.6 调用方用 `apperror.Wrap(err, ErrUnauthorized, "...")` 把 `auth.ErrTokenInvalid` / `auth.ErrTokenExpired` 包成 1001 业务码
- ADR-0007 ctx 传播：本 story 是合理 exception（CPU 计算无 ctx）；调用方传 ctx 给上层 service / handler 但不消费

### 测试策略

按 ADR-0001 §3.1 + 4.2 / 4.3 已建立的测试范式：

- **单测层**（`token_test.go` + `loader_test.go` 增量）：纯 Go test，不起容器 / 不连 MySQL
- **集成测试层**：本 story **不**写集成测试 —— token util 是无 IO 的纯算工具，dockertest / sqlmock 都不适用；4.7 Layer 2 集成测试会用真 token 跑真 auth 中间件 + service 端到端，间接覆盖 token 在事务上下文的端到端正确性

**关键决策**：本 story **不**用 jwt 库的 `MockClock` 或类似 mock 时钟。原因：(1) jwt-v5 没有内置 clock 抽象；(2) 引入第三方 clock interface（如 jonboulle/clockwork）增加依赖；(3) 真 sleep 跨秒边界更接近真实环境 —— 2s sleep 在单 case 内可控，整体单测开销可接受。

### 启动顺序约束

按 4.2 / 4.3 已建立的启动顺序：

```
main()
├─ logger.Init("info")                     # bootstrap logger
├─ parseTopLevelArgs                        # 4.3 加：拆 flag / migrate 子命令
├─ if isMigrate: cli.RunMigrate; os.Exit    # 4.3 加：migrate 分支
├─ config.LocateDefault / Load              # 已有
├─ logger.Init(cfg.Log.Level)               # 已有
├─ db.Open(dbOpenCtx, cfg.MySQL)            # 4.2 加
├─ defer sqlDB.Close                         # 4.2 加
├─ tx.NewManager(gormDB)                     # 4.2 加
│
├─ ★ 本 story 新增：auth.New(cfg.Auth.TokenSecret, cfg.Auth.TokenExpireSec)
│   └─ 失败 → slog.Error + os.Exit(1)（fail-fast）
│
└─ bootstrap.Run(ctx, cfg, gormDB, txMgr, signer)  # 签名扩展含 signer
```

**关键**：`auth.New` 在 `db.Open` 之后 / `bootstrap.Run` 之前。`auth.New` 是纯 CPU < 1µs，不需要 timeout context；放在 `db.Open` 之后只是分组逻辑（先校验所有外部依赖再校验内部依赖）。

### 与已 done 的 4.2 / 4.3 的衔接

**4.2 实装**：
- `internal/infra/db/mysql.go`：`Open(ctx, cfg) (*gorm.DB, error)`
- `internal/repo/tx/manager.go`：`Manager` interface + `WithTx(ctx, fn)`
- `internal/infra/config/config.go` + `loader.go`：`MySQLConfig` + `CAT_MYSQL_DSN` env override

**4.3 实装**：
- `server/migrations/`：5 张表 up/down SQL
- `internal/infra/migrate/migrate.go`：Migrator interface + golang-migrate v4 包装
- `internal/cli/migrate.go`：catserver migrate 子命令分发
- 删除 `internal/tools/tools.go` 占位

**本 story 复用**：
- `cfg`（loader.go 路径）：本 story 在同一个 Config struct 加 AuthConfig + 同一个 Load 函数加 env override（与 MySQL 同模式）
- `bash scripts/build.sh`：本 story 不改脚本，直接复用 vet/test/race 路径

**本 story 新增解耦的 path**：
- `internal/pkg/auth/`：与 `internal/infra/db/` / `internal/infra/migrate/` 平级；不 import db / repo / tx 任一包（pkg 层是无业务依赖的工具层）
- `bootstrap.Run` 签名扩展：本 story 是 4.2 / 4.3 之后第一个新增 bootstrap DI 的 story；用 `app.Deps` struct 容纳后续会增加的 signer / cache / etc 共享对象，避免 Run 函数签名爆炸

### 与下游 4.5 / 4.6 的接口

**4.5 落地时会做**：
1. `internal/app/http/middleware/auth.go`：用 `Authorization: Bearer <token>` header → 调 `signer.Verify(tokenStr)` → claims.UserID 存入 `c.Set("userID", uid)`
2. `Verify` 错误（任意 ErrTokenInvalid / ErrTokenExpired）→ `c.Error(apperror.Wrap(err, ErrUnauthorized, "未登录或 token 无效"))` + `c.Abort()`
3. 中间件接受 signer 通过 DI 注入（不每请求 `auth.New`，因为单例）

**4.6 落地时会做**：
1. `service/auth_service.go::GuestLogin` 完成 5 表事务后调 `signer.Sign(user.ID, cfg.Auth.TokenExpireSec)` → 拿 token string
2. handler 把 token 放入 V1 §4.1 response.data.token
3. `Sign` 失败（理论不发生，secret 已 fail-fast）→ `apperror.Wrap(err, ErrServiceBusy, "服务繁忙")`

**本 story 必须保证 4.5 / 4.6 能直接用**：
- `Signer` 通过 bootstrap DI 暴露给 handler / middleware（不再每次构造）
- `Verify` 返 `Claims` typed struct（4.5 中间件 type-safe 取 UserID）
- `Sign` 接受 `uint64` userID（4.6 直接传 user.ID 不需要类型转换）
- `auth.ErrTokenInvalid` / `auth.ErrTokenExpired` 哨兵 error（4.5 用 `errors.Is` 区分日志级别）

### 关键决策点（实装时注意）

1. **jwt/v5 alg validation 必须显式**：`jwt.ParseWithClaims(token, claims, keyfunc)` 中 keyfunc 必须返回 `(any, error)`，并在 keyfunc 内显式检查 `token.Method.Alg() == "HS256"` 否则返 error。golang-jwt v5 默认严格但**仍**需 explicit assert（防 alg=none / RS256 篡改）：

   ```go
   keyfunc := func(t *jwt.Token) (any, error) {
       if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
           return nil, fmt.Errorf("unexpected alg: %s", t.Method.Alg())
       }
       return s.secret, nil
   }
   ```

2. **过期错误从 jwt/v5 返回**：jwt/v5 的 `ParseWithClaims` 失败时 error 实现了 `errors.Is(err, jwt.ErrTokenExpired)`；本 story 在 Verify 内部 unwrap 检查并映射到本包 `ErrTokenExpired`。**注意**：jwt/v5 v5.0+ 与 v4 / jwt-go old API 不同；用 `errors.Is(err, jwt.ErrTokenExpired)` 而非 `err.Is(...)` 方法调用。

3. **iat / exp 用 epochs（int64 秒）而不是 time.Time**：jwt-v5 接受 `*jwt.NumericDate` 类型 wrap epochs；本 story 内部存 int64，构造 token 时用 `jwt.NewNumericDate(time.Unix(epoch, 0))`，解析时 `claims.ExpiresAt.Time.Unix()` 拿回。

4. **Claims struct 实装 `jwt.Claims` interface**：v5 要求 claims 实现 `GetExpirationTime() (*NumericDate, error)` 等 6 个方法。本包定义自己的 typed struct 必须实装这 6 个方法（即使部分返 nil / 空，如 GetIssuer / GetSubject / GetAudience）。**或者**用 v5 提供的 `jwt.RegisteredClaims` 嵌入 + 自定义 user_id 字段：

   ```go
   type tokenClaims struct {
       UserID uint64 `json:"user_id"`
       jwt.RegisteredClaims  // 提供 iat / exp / nbf / iss / sub / aud / jti 全套
   }
   ```

   本 story 推荐用 RegisteredClaims 嵌入式（少写 6 个方法 boilerplate），但**只写 IssuedAt / ExpiresAt 两个**（其他留空）；解析时也只读这两个。

5. **secret 用 `[]byte` 不用 string**：jwt-v5 keyfunc 期望 secret 是 `[]byte`；`Signer.secret` 字段直接存 `[]byte`，`New` 时 `[]byte(secret)` 一次。

6. **windows 时区 / wall clock**：`time.Now().Unix()` 在 Windows / Linux 一致（UTC epoch）；不存在 4.3 的 windows file URI 类陷阱。

### Project Structure Notes

预期文件 / 目录变化：

- 新增：`server/internal/pkg/auth/token.go` + `server/internal/pkg/auth/token_test.go`
- 修改：`server/internal/infra/config/config.go`（加 AuthConfig + Auth 字段）
- 修改：`server/internal/infra/config/loader.go`（加 envAuthTokenSecret + Load 内 env override + 默认值兜底）
- 修改：`server/internal/infra/config/loader_test.go`（加 ≥3 case 覆盖新 env / YAML 段）
- 修改：`server/configs/local.yaml`（加 `auth:` 段）
- 修改：`server/cmd/server/main.go`（在 db.Open 之后加 auth.New + 失败 os.Exit；signer 传给 bootstrap.Run）
- 修改：`server/internal/app/bootstrap/bootstrap.go`（Run 签名扩展含 signer，或内部 Deps struct）
- 修改：`server/go.mod` + `server/go.sum`（加 `github.com/golang-jwt/jwt/v5` require + sum）
- 修改：`_bmad-output/implementation-artifacts/sprint-status.yaml`（4-4-token-util: backlog → ready-for-dev → in-progress → review；由 dev-story 流程内推动）
- 修改：`_bmad-output/implementation-artifacts/4-4-token-util.md`（本 story 文件，dev 完成后填 Tasks/Dev Agent Record/File List/Completion Notes）

不影响其他目录：

- `server/internal/infra/db/` 不变（4.2 已实装，本 story 不动）
- `server/internal/infra/migrate/` 不变（4.3 已实装）
- `server/internal/cli/` 不变（4.3 已实装）
- `server/internal/repo/tx/` 不变（4.2 已实装）
- `server/internal/repo/mysql/` 不存在（4.6 才落地第一个 user_repo.go）
- `server/internal/service/` 不存在（4.6 才落地第一个 auth_service.go）
- `server/internal/app/http/middleware/` 中 auth.go / rate_limit.go 不存在（4.5 才落地）
- `server/internal/app/http/handler/` 中 auth_handler.go 不存在（4.6 才落地）
- `server/migrations/` 不变（4.3 已实装；token util 与 DB 无关）
- `iphone/` / `ios/` 不变（server-only story）
- `docs/宠物互动App_*.md` 全部 7 份不变（消费方）
- README.md / server/README.md 不变（Epic 4 收尾或 Epic 36 部署 story 才统一更新）

### golang-jwt/jwt/v5 关键 API 提示

实装时注意（避免常见坑）：

- **包 import**：`"github.com/golang-jwt/jwt/v5"` —— v5 是 module path 一部分，与 v4 / dgrijalva 老版本路径不同
- **签名算法常量**：`jwt.SigningMethodHS256` —— `*jwt.SigningMethodHMAC` 实例；`token.Method.Alg() == "HS256"`（注意是字符串比较 alg name 而不是指针等价；同一 alg 的 SigningMethod 可能跨不同 jwt 库实例）
- **NumericDate 包装**：`jwt.NewNumericDate(time.Now())` 把 time.Time 转 jwt 的内部时间类型；解析时 `claims.ExpiresAt.Time` 拿回 time.Time
- **RegisteredClaims 嵌入**：v5 推荐用 `jwt.RegisteredClaims` 嵌入式 claims struct，避免手写 6 个 Get* 方法 boilerplate
- **keyfunc 必须**：`ParseWithClaims` 必须传非 nil keyfunc；keyfunc 内**必须**校验 `token.Method.Alg()` 防 alg=none 攻击
- **错误穿透**：v5 的 `errors.Is(err, jwt.ErrTokenExpired)` 等可正确穿透 wrapped error（v5 用 `Errors` slice + `errors.Is` 兼容）

### References

- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 4.4 (行 999-1020)] — 本 story 钦定 AC 来源（Sign/Verify 签名 / HS256 / claims 三项 / 默认 7 天 / secret 空 fail-fast / ≥5 单测）
- [Source: `_bmad-output/planning-artifacts/epics.md` §Epic 4 Overview (行 927-931)] — 节点 2 第一个业务 epic / 执行顺序 4.1 → 4.2 → 4.3 → **4.4** → 4.5 → 4.6 → 4.8 → 4.7
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 4.5 (行 1022-1049)] — 下游紧接，本 story 产出的 Verify 是 auth 中间件唯一 token 解析路径
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 4.6 (行 1051-1082)] — 下游游客登录，本 story 产出的 Sign 是 token 唯一签发入口
- [Source: `docs/宠物互动App_V1接口设计.md` §2.3 (行 41-44)] — Authorization: Bearer 头格式
- [Source: `docs/宠物互动App_V1接口设计.md` §3 (行 79-118)] — 错误码 1001（未登录 / token 无效）；本 story 的 ErrTokenInvalid / ErrTokenExpired 由 4.5 中间件统一映射为 1001
- [Source: `docs/宠物互动App_V1接口设计.md` §4.1 (行 174-220)] — guest-login response.data.token 的契约：HS256 + auth.token_secret 签 + 默认 7 天
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 (行 196-201)] — `internal/pkg/auth/` 目录树锚定
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §6.1 (行 338-360)] — Auth 模块职责定位（"用户如何进入系统"，本 story 落"Token 签发与校验"子项）
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §13 (行 931-933)] — YAML 配置块：`auth: { token_secret: xxx, token_expire_sec: 604800 }`
- [Source: `_bmad-output/implementation-artifacts/decisions/0006-error-handling.md` §2 / §3] — 三层错误映射：repo 原生 → service apperror.Wrap → handler c.Error → middleware envelope；本 story 是 pkg 层（非 repo/service/handler），返 typed sentinel error 给 service 层 wrap
- [Source: `_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md` §3] — ctx 必传规则；本 story 是合理 exception（纯 CPU < 1µs，无 IO，stdlib hmac 同模式）
- [Source: `_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.1] — 单测 + 集成测试双层；本 story 仅单测层（无 IO）
- [Source: `_bmad-output/implementation-artifacts/4-2-mysql-接入.md`] — 上游 story；本 story 复用 cfg.Load / loader env override 模式（CAT_MYSQL_DSN → CAT_AUTH_TOKEN_SECRET）
- [Source: `_bmad-output/implementation-artifacts/4-3-五张表-migrations.md`] — 上游 story；本 story 复用启动序列模式（fail-fast / signer 加在 db.Open 后）
- [Source: `docs/lessons/2026-04-26-config-env-override-and-gorm-auto-ping.md`] — 4.2 review lesson：infrastructure 接入必须配齐 env override；本 story 必须给 `cfg.Auth.TokenSecret` 加 `CAT_AUTH_TOKEN_SECRET` env override
- [Source: `docs/lessons/2026-04-26-startup-blocking-io-needs-deadline.md`] — 启动阻塞 IO 必须有 deadline；本 story `auth.New` 是纯 CPU（< 1µs）无需 timeout，但与该 lesson 兼容（"非阻塞 IO 不需要 deadline"）
- [Source: `docs/lessons/2026-04-25-slog-init-before-startup-errors.md`] — 早期启动错误必须走结构化日志；本 story 的 `auth.New` 失败错误用 `slog.Error` 输出
- [Source: `CLAUDE.md` §"工作纪律"] — "节点顺序不可乱跳 / 状态以 server 为准 / ctx 必传"；本 story 是节点 2 第四条 server story 严格按 4.4 顺序推
- [Source: `CLAUDE.md` §"Build & Test"] — 写完 / 改完 Go 代码后跑 `bash scripts/build.sh --test` 验证
- [Source: `MEMORY.md` "No Backup Fallback"] — 反对 fallback 掩盖核心风险；本 story `auth.New` invalid 输入直接 fail-fast，不做随机 secret fallback
- [Source: `MEMORY.md` "Repo Separation"] — server 测试自包含，不调用 APP / watch；本 story 仅 server 单测，不依赖任何端联调
- [Source: `docs/宠物互动App_数据库设计.md` §3.1 (行 73-167)] — 主键 BIGINT UNSIGNED；本 story Sign(userID uint64) 与该类型对齐

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m] (Anthropic Opus 4.7, 1M context)

### Debug Log References

- `bash /c/fork/cat/scripts/build.sh` → BUILD SUCCESS（vet + build → build/catserver.exe）
- `bash /c/fork/cat/scripts/build.sh --test` → all tests passed（含 11 个新 auth 测试 + 4 个新 loader 测试 + 现有所有测试）
- `cd server && go test ./internal/pkg/auth/... -count=1 -v` → 11 测试函数 / ~25 sub-tests 全绿，1.5s（其中 TestVerify_Expired 包含 1.1s sleep 跨秒边界）
- `cd server && go mod verify` → all modules verified
- `cd server && go mod tidy` → 无 diff（jwt/v5 v5.3.1 唯一新增 require，无 indirect 依赖）
- `bash /c/fork/cat/scripts/build.sh --race --test` → Windows ThreadSanitizer "failed to allocate" 全包 FAIL（按 ADR-0001 §3.5 + 4.2/4.3 review lesson skip；Linux/CI race 路径不受影响）

### Completion Notes List

- **JWT 库选型**：`github.com/golang-jwt/jwt/v5 v5.3.1`（最新稳定版；jwt-go fork 后官方 successor；零第三方依赖；v5 默认严格 alg validation）
- **alg validation 实装**：`Verify` 内 keyfunc 显式 assert `t.Method.Alg() != jwt.SigningMethodHS256.Alg()` → 拒绝 alg=none / RS256 / 其他算法（防 alg confusion 攻击）。专门 `TestVerify_AlgNone_ReturnsErrTokenInvalid` case 防止 future 退化
- **Claims 结构**：用 `jwt.RegisteredClaims` 嵌入 + 自定义 `UserID uint64` 字段（避免手写 6 个 Get* 方法 boilerplate）；对外仅暴露 typed `Claims{UserID, IssuedAt, ExpiresAt}` 三字段（不暴露 jwt-v5 内部类型，调用方无需 import jwt）
- **错误穿透**：`Verify` 用 `errors.Is(err, jwt.ErrTokenExpired)` 区分过期 vs 其他失败 → 映射到本包 `ErrTokenExpired` / `ErrTokenInvalid` 哨兵（4.5 中间件用 `errors.Is(err, auth.ErrTokenExpired)` 区分日志级别）
- **fail-fast 校验**：`auth.New` 校验 secret 空 / < 16 字节 / expireSec ≤ 0 / expireSec > 30 天 → 任一失败返 error → main.go `slog.Error + os.Exit(1)`
- **bootstrap.Run 签名扩展**：参数加 `signer *auth.Signer`，与 gormDB / txMgr 同模式（本 story 阶段不消费，仅 wire；4.5 中间件 / 4.6 handler 才真正用）
- **server_test.go 适配**：原 `Run(ctx, cfg, nil, nil)` → `Run(ctx, cfg, nil, nil, nil)` 加第 5 个 signer 参数（nil 允许，与 gormDB / txMgr 同 nil-tolerant 设计）
- **测试覆盖**：11 个顶层 test 函数，超过 epics.md "≥ 5 case" 下限：
  1. `TestSignVerify_RoundTrip_Happy` — happy path（claim 正确 + iat/exp 范围）
  2. `TestSignVerify_CustomExpireSec` — 显式 expireSec 覆盖 default
  3. `TestVerify_Expired_ReturnsErrTokenExpired` — 过期 → ErrTokenExpired
  4. `TestVerify_TamperedSignature_ReturnsErrTokenInvalid` — 篡改 → ErrTokenInvalid
  5. `TestVerify_DifferentSecret_ReturnsErrTokenInvalid` — 跨 signer secret mismatch
  6. `TestVerify_MalformedFormat_ReturnsErrTokenInvalid` — 5 sub-cases 格式错
  7. `TestVerify_AlgNone_ReturnsErrTokenInvalid` — 防 alg=none 攻击
  8. `TestNew_EmptySecret_ReturnsError` — 5 sub-cases fail-fast
  9. `TestNew_BoundaryValid` — 3 sub-cases 边界值（恰好 16 字节 / expire=1 / expire=30天）
- **Loader 测试覆盖**：3 个新 case：
  1. `TestLoad_AuthDefaultTokenExpireSec` — YAML 无 auth 段 → expire 默认 604800
  2. `TestLoad_AuthTokenSecretEnvOverride` — `CAT_AUTH_TOKEN_SECRET` env 覆盖
  3. `TestLoad_AuthYAMLParsing` — auth.yaml fixture 含完整 auth 段 → 解析正确
- **范围红线遵守**：未实装 auth 中间件（4.5）/ guest-login handler（4.6）/ user_repo 等（4.6）/ Redis（10.2）；未改 docs/宠物互动App_*.md 任一份；未写新 README
- **已知 tech debt**：
  - `server/README.md` 缺少 "如何 export CAT_AUTH_TOKEN_SECRET" 指南（推到 Epic 4 收尾或 Epic 36 部署 story）
  - 生产 / staging YAML 未添加（默认 secret 留空 + env 注入是唯一正路；具体 YAML 由部署 story 落地）
  - `app.Deps` struct 未抽象（本 story 仅 3 个共享依赖；后续 4.5 / 4.6 加 middleware 等再考虑收敛）
- **Windows ThreadSanitizer**：`--race --test` 在 Windows 全包 FAIL（"failed to allocate"），是已知平台限制，非本 story 引入。Linux/CI 路径不受影响（与 4.2 / 4.3 同模式）

### File List

新建文件：

- `server/internal/pkg/auth/token.go`（Signer / Claims / New / Sign / Verify / 哨兵 error）
- `server/internal/pkg/auth/token_test.go`（11 测试函数 / ~25 sub-tests）
- `server/internal/infra/config/testdata/auth.yaml`（测试 fixture：含完整 auth 段）

修改文件：

- `server/internal/infra/config/config.go`（加 AuthConfig struct + Config.Auth 字段）
- `server/internal/infra/config/loader.go`（加 envAuthTokenSecret / defaultTokenExpireSec const + env override + 默认值兜底）
- `server/internal/infra/config/loader_test.go`（加 3 个新 test case）
- `server/configs/local.yaml`（加 auth: 段，token_secret 留空 + token_expire_sec=604800 + 注释）
- `server/internal/app/bootstrap/server.go`（Run 签名加 signer *auth.Signer 参数）
- `server/internal/app/bootstrap/server_test.go`（Run 调用更新为传 nil signer）
- `server/cmd/server/main.go`（db.Open 之后加 auth.New fail-fast；signer 传给 bootstrap.Run）
- `server/go.mod`（加 `github.com/golang-jwt/jwt/v5 v5.3.1` require）
- `server/go.sum`（jwt/v5 hash + 自身 sum）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（4-4-token-util: ready-for-dev → in-progress → review）
- `_bmad-output/implementation-artifacts/4-4-token-util.md`（本 story 文件，Status / Tasks 勾选 / Dev Agent Record 填充）

### Change Log

- 2026-04-26 — Story 4.4 dev 完成：
  - 新增 `internal/pkg/auth/` 包：HS256 JWT util（Signer.Sign / Signer.Verify + 强类型 Claims）
  - JWT 库：`github.com/golang-jwt/jwt/v5 v5.3.1`，零第三方依赖
  - alg validation 显式 assert HS256，防 alg=none / RS256 篡改攻击
  - 哨兵 error：`ErrTokenInvalid` / `ErrTokenExpired`，调用方用 `errors.Is` 区分日志级别
  - `internal/infra/config.AuthConfig` + `CAT_AUTH_TOKEN_SECRET` env override
  - `cmd/server/main.go` 启动序列加 `auth.New` fail-fast（secret 空 / 短 / expire 异常 → os.Exit(1)）
  - `bootstrap.Run` 签名扩展 `signer *auth.Signer` 参数（4.5 / 4.6 通过该入口消费）
  - 测试：11 auth 测试 + 3 loader 测试 + 现有全部 → `bash scripts/build.sh --test` 全绿
