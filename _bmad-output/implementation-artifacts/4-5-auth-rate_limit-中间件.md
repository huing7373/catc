# Story 4.5: auth + rate_limit 中间件（AR9b）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want 一个 Bearer token 校验中间件（基于 Story 4.4 `*auth.Signer.Verify` 解析 `Authorization` header → 把 `userID` 注入 ctx）和一个 token bucket 限频中间件（基于 IP / userID 维度，默认每分钟 60 次，超限 1005）,
so that Story 4.6 游客登录 handler 可以挂 `rate_limit`（防 brute force）、Story 4.8 GET /home 可以挂 `auth`（取 userID 后查聚合数据），后续业务接口按需挂载即可，不必各自处理鉴权 / 限频细节，且全链路统一走 ADR-0006 三层错误映射输出 V1 §3 错误码。

## 故事定位（Epic 4 第五条 = 节点 2 server 第一次实装业务 HTTP 中间件；上承 4.4 token util，下启 4.6 游客登录 handler / 4.8 GET /home）

- **Epic 4 进度**：4.1 (契约定稿，done) → 4.2 (MySQL 接入 + tx manager，done) → 4.3 (5 张表 migrations，done) → 4.4 (token util，done) → **4.5 (本 story，auth + rate_limit 中间件)** → 4.6 (游客登录 handler + 首次初始化事务) → 4.8 (GET /home) → 4.7 (Layer 2 集成测试)。本 story 是 4.6 / 4.8 的**直接前置**：4.6 `/auth/guest-login` 必须挂 rate_limit（V1 §4.1 行 139 钦定），4.8 `/home` 必须挂 auth + rate_limit（V1 §5 + epics.md 行 1131）。
- **epics.md AC 钦定**：`_bmad-output/planning-artifacts/epics.md` §Story 4.5（行 1022-1049）已**精确**列出 ACs：auth 中间件解析 `Authorization: Bearer <token>` → 调 token util Verify → 把 `userID` 塞 gin.Context → 失败返 1001；rate_limit 中间件用**内存 token bucket**（MVP 不依赖 Redis）→ 默认每用户每分钟 60 次 → 配置可调 `ratelimit.per_user_per_min` → 超限返 1005；两个中间件**按需挂载**（auth 默认全局开 = 业务路由组层级，rate_limit 至少挂在 `/auth/guest-login` 上）；≥6 单测 case（auth happy / 无 header / 过期；rate_limit happy / 第 61 次 / 跨分钟边界）。
- **AR9b 节点 2 中间件钦定**：`_bmad-output/planning-artifacts/epics.md` 行 170 AR9b 列明节点 2 中间件 = `auth`（Bearer token 校验）+ `rate_limit`（至少保护登录、开箱、合成等敏感接口），**落 E4**。本 story 是 AR9b 唯一落地路径。
- **V1 接口设计 §2.3 + §3 钦定鉴权 / 错误码**：
  - §2.3 行 41-44：`Authorization: Bearer <token>` 头格式，是 auth 中间件唯一输入路径
  - §3 行 80：错误码 1001 = 未登录 / token 无效（auth 中间件输出）
  - §3 行 84：错误码 1005 = 操作过于频繁（rate_limit 中间件输出）
  - §4.1 行 139：`POST /auth/guest-login` 认证 = **不需要**（登录接口本身免 auth），但**走 rate_limit 中间件**；行 140 + 218 钦定限频默认值 = "同 IP 每分钟 60 次"（**注意**：4.5 epics.md 行 1039 写的是"每用户每分钟 60 次"。**用户未登录时无 userID，必须降级到 IP 维度**；详见 AC2 反模式段）
  - §4.3 行 299：错误码 1001 由 auth 中间件拦截
- **设计文档 §4 + §8.2 钦定包路径**：`docs/宠物互动App_Go项目结构与模块职责设计.md` §4 行 133（auth.go）+ §8.2 行 724-725（auth / rate_limit 中间件）锚定 `internal/app/http/middleware/auth.go` + `internal/app/http/middleware/rate_limit.go`（与现有 `request_id.go` / `recover.go` / `logging.go` / `error_mapping.go` 平级；不放 service / handler / pkg）。
- **设计文档 §13 钦定配置块**：`docs/宠物互动App_Go项目结构与模块职责设计.md` §13 行 931-933（auth）已由 4.4 落地 `auth.token_secret` / `auth.token_expire_sec`；本 story **新增** `ratelimit:` YAML 段（**不**与 `auth:` 合并；语义独立，与 NFR12 "限频控制放 Redis" 解耦 —— 节点 2 是内存 token bucket，节点 9+ 才迁 Redis）。
- **下游立即依赖**：
  - **Story 4.6 (游客登录 handler)**：`POST /auth/guest-login` route 挂 `rate_limit` 但**不**挂 `auth`（V1 §4.1 行 139 钦定）；handler 完成事务后调 `*auth.Signer.Sign(user.ID, ...)` 拿 token 返回 client。本 story 必须保证 rate_limit 可独立挂在单一 route group / handler 上（不强制全局）
  - **Story 4.8 (GET /home)**：`GET /home` route 挂 `auth` + `rate_limit`（epics.md 行 1131）；handler 从 `c.Get("userID")` 拿 uint64 → 调 `home_service.LoadHome(ctx, userID)`。本 story 必须保证 `userID` 在 ctx / c.Keys 中以 uint64 类型注入（不是 string，避免 4.8 / 后续 handler 反复 strconv.ParseUint）
  - **Story 4.7 (Layer 2 集成测试)**：跑 dockertest 真 MySQL + 真 auth 中间件 + 真 service 端到端；本 story 中间件必须可与 4.6 / 4.8 handler 通过真实 HTTP roundtrip 集成（不只单元 mock）
  - **未来节点 7 / 9 / 10**（开箱 / 合成 / Redis）：rate_limit 中间件设计必须为 future "Redis-based rate limiter"留口子（接口抽象，不硬绑内存实现），让 Epic 10 接 Redis 时可平滑替换 backing store；详见 AC3 设计约束段
- **范围红线**：本 story **只**新增 `internal/app/http/middleware/auth.go` + `rate_limit.go` + 各自单测 + `internal/infra/config/` 加 `RateLimitConfig` struct + 默认值兜底 + `local.yaml` 加 `ratelimit:` 段 + bootstrap router 注入（让 4.6 / 4.8 落地时能直接 r.Use(...)）。**不**实装任何 handler / service / repo / token util 修改。

**本 story 不做**（明确范围红线）：

- ❌ **不**实装 `/auth/guest-login` handler（Story 4.6 落地；本 story 只产出 rate_limit 中间件让 4.6 挂上）
- ❌ **不**实装 `/me` / `/home` handler（Story 4.6 / 4.8 落地）
- ❌ **不**实装 user_repo / auth_binding_repo / pet_repo / step_account_repo / chest_repo（Story 4.6 落地）
- ❌ **不**修改 `internal/pkg/auth/token.go`（4.4 已 done；本 story 仅作为 `*auth.Signer` 的消费方）
- ❌ **不**接 Redis（NFR12 钦定限频最终走 Redis，但节点 2 阶段是内存 token bucket，节点 10+ Story 10.2 才接 Redis；切换由 future story 负责）
- ❌ **不**改 `docs/宠物互动App_*.md` 任一份文档（V1 §3 / §4.1 / 设计文档 §8.2 是契约**输入**，本 story 严格对齐但**不修改**）
- ❌ **不**实装 IP-based + userID-based 的混合限频策略（本 story 选**单一维度**：登录前路径用 IP key，登录后路径用 userID key；混合维度的"每用户每秒 + 每 IP 每秒"双限是 Epic 17.5 / 节点 9+ 的进阶需求）
- ❌ **不**实装动态限频（按 user 等级 / VIP 调整速率）：MVP 单一速率 + YAML 静态配置足够
- ❌ **不**实装 IP 白名单 / 黑名单：MVP 不需要；后续 epic 决策
- ❌ **不**实装 distributed rate limit（多实例共享计数）：节点 2 单实例部署即可，多实例切 Redis 后再做（NFR12 + tech debt log 已登记）
- ❌ **不**实装 cleanup goroutine 主动清理过期 bucket：MVP 用 LRU + 定期 sweep（限制 map 内存上限），不引入额外 goroutine 复杂度（详见 AC3 设计约束）
- ❌ **不**实装 `Retry-After` HTTP header（V1 §3 没钦定，client 不依赖；future 可加）
- ❌ **不**实装 prometheus 限频专属指标（如 `rate_limit_rejected_total{path}`）—— 现有 Logging 中间件已通过 `error_code=1005` 给出可聚合信号，OK；future epic 加专属 metric
- ❌ **不**实装 metrics 限频（`/metrics` 路由组层级豁免 rate_limit）—— router.go 中 `/metrics` 是运维端点，本 story 设计避开把 rate_limit 套全局（详见 AC4）
- ❌ **不**实装 `/ping` / `/version` 路由的 rate_limit / auth：这两条是运维端点，必须保持公开 + 高频可用（否则 health check 自己被限流）
- ❌ **不**写新 README / 部署文档：rate_limit 配置说明放在 `local.yaml` 注释 + 中间件 godoc 即可；运维 README 由 Epic 36 部署 story 统一写
- ❌ **不**给 `auth_handler.go` / `home_handler.go` 提前占位（即使空 handler）—— 它们由 4.6 / 4.8 真正落地时新建，本 story 仅产出中间件
- ❌ **不**用反射 / 依赖注入框架（wire / fx）：本 story 仍走显式 DI（与 4.2 / 4.4 同模式），bootstrap.Run 签名扩展接受 signer + rateLimitCfg

## Acceptance Criteria

**AC1 — `internal/app/http/middleware/auth.go`：Auth 中间件**

新增 `server/internal/app/http/middleware/auth.go`，提供以下公开 API（包名 `middleware`，与现有 `request_id.go` / `error_mapping.go` 同包）：

```go
// Package middleware 提供 server HTTP 中间件 ...（已存在；本文件加新内容）

// UserIDKey 是 auth 中间件把验证通过的 userID（uint64）写到 gin.Context 的 key。
//
// 下游 handler 取法：
//   uid, ok := c.Get(middleware.UserIDKey)
//   if !ok { /* 不可能：auth 通过必有 uid；防御性写 1009 */ }
//   userID := uid.(uint64)
//
// 选 uint64 而非 string：
//   - 数据库设计 §3.1 钦定主键 BIGINT UNSIGNED → Go 端 uint64
//   - 4.4 token util 已经返回 Claims.UserID (uint64)；本 key 直接传递，
//     避免 4.6 / 4.8 handler 反复做 strconv.ParseUint 转换
//   - 与 V1 §2.5 "JSON 端 string / Go 内部 uint64" 划分一致：
//     uint64 是内部计算类型，仅在响应 envelope 序列化时转 string
//
// 注意：本 key 只在通过 auth 中间件的请求中存在；handler 不能假设非 auth 路由
// （如 /auth/guest-login）c.Get(UserIDKey) 一定能拿到。
const UserIDKey = "userID"

// Auth 中间件：从 Authorization: Bearer <token> 解析 token → 调 *auth.Signer.Verify
// → 成功后把 Claims.UserID 写入 c.Set(UserIDKey, uid)；失败 c.Error + c.Abort。
//
// # 挂载位置（router.go）
//
// 业务路由组层级（**不**全局），与 rate_limit 同位置：
//
//   r := bootstrap.NewRouter()
//   api := r.Group("/api/v1", middleware.RateLimit(...))   // 全 v1 接口走 rate_limit
//   api.Use(middleware.Auth(signer))                        // 但 auth 在 group 内 + 可被子 group 覆盖
//
//   authGroup := api.Group("/auth")                         // 登录接口子组
//   // /auth/guest-login 必须**绕过** auth（V1 §4.1 行 139 "认证：不需要"）
//   authGroup.POST("/guest-login", authHandler.GuestLogin)  // **不要**预先 r.Use(Auth)
//
// 实装路径：本 story 在 router.go 暂不 wire（4.6 落地时才挂 /auth/guest-login route +
// /auth/me route），但提供两个工厂函数让 4.6 能直接调。
//
// # 错误映射
//
// header 缺失 / 格式错 / token 无效 / token 过期 → 通过 c.Error 推 AppError(1001)；
// ErrorMappingMiddleware 在 c.Next() 之后扫到 → 写 envelope。本中间件**不**直接调
// response.Error（避免绕过 ADR-0006 钦定的"单一 envelope 生产者"，详见
// docs/lessons/2026-04-24-error-envelope-single-producer.md）。
//
// # 错误细分日志
//
// 本中间件只产出统一的 1001 envelope（V1 §3 钦定），但内部用 errors.Is 区分
// auth.ErrTokenExpired vs auth.ErrTokenInvalid，让 ErrorMappingMiddleware 落
// 不同 log level（过期 INFO 是正常重新登录路径；篡改 / 格式错 WARN 是潜在攻击）。
// 通过 apperror.Wrap(err, ...) 时 Cause 链保留 sentinel；ErrorMappingMiddleware
// 的 logLevel 决策可基于 cause（本 story 暂不改 ErrorMappingMiddleware；只保证
// AppError.Cause 正确，让 future 升级日志策略时有信号可用）。
func Auth(signer *auth.Signer) gin.HandlerFunc {
    return func(c *gin.Context) {
        rawHeader := c.GetHeader("Authorization")
        if rawHeader == "" {
            _ = c.Error(apperror.New(apperror.ErrUnauthorized, apperror.DefaultMessages[apperror.ErrUnauthorized]))
            c.Abort()
            return
        }

        // 解析 "Bearer <token>" 形态。RFC 6750 §2.1 钦定 scheme 大小写不敏感
        // （建议大写 Bearer），token 部分原样保留。**严格** scheme 校验（防"basic"
        // / 无前缀 token / 多空格 / 多 token）：
        const bearerPrefix = "Bearer "
        if len(rawHeader) <= len(bearerPrefix) || !strings.EqualFold(rawHeader[:len(bearerPrefix)], bearerPrefix) {
            _ = c.Error(apperror.Wrap(
                fmt.Errorf("auth: invalid Authorization scheme"),
                apperror.ErrUnauthorized,
                apperror.DefaultMessages[apperror.ErrUnauthorized],
            ))
            c.Abort()
            return
        }
        tokenStr := rawHeader[len(bearerPrefix):]
        // 防御 leading whitespace（"Bearer  abc" 多空格）/ trailing whitespace
        // （client 自己拼错），整段 trim：
        tokenStr = strings.TrimSpace(tokenStr)
        if tokenStr == "" {
            _ = c.Error(apperror.New(apperror.ErrUnauthorized, apperror.DefaultMessages[apperror.ErrUnauthorized]))
            c.Abort()
            return
        }

        claims, err := signer.Verify(tokenStr)
        if err != nil {
            // err 链上保留 auth.ErrTokenInvalid / auth.ErrTokenExpired sentinel，
            // future 由 ErrorMappingMiddleware / Logging 用 errors.Is 区分日志级别
            _ = c.Error(apperror.Wrap(err, apperror.ErrUnauthorized, apperror.DefaultMessages[apperror.ErrUnauthorized]))
            c.Abort()
            return
        }

        c.Set(UserIDKey, claims.UserID)
        c.Next()
    }
}
```

**关键设计约束**：

- **签名 `Auth(signer *auth.Signer) gin.HandlerFunc`**：通过 closure 持有 signer 单例（4.4 钦定 `*Signer` 为 thread-safe 单例，bootstrap 期 New 一次复用），**不**在中间件内部每次 New
- **header 三种失败路径明确分类**：(a) 缺 Authorization → 1001；(b) scheme 不是 Bearer → 1001（同时给 ErrorMappingMiddleware 一个区分 cause 的 fmt.Errorf wrap）；(c) token 部分为空 / 全空格 → 1001。三路径都返 1001（不区分子码）—— V1 §3 没拆 1001 子码，client 拿到 1001 后无差别走"重新登录"路径
- **`UserIDKey = "userID"` 是字符串常量**：camelCase 与 V1 §2.5 / Go 字段命名惯例对齐；**不**与 RequestIDKey (`request_id`) 同样下划线，因为 RequestIDKey 来自 V1 §2.4 envelope 字段（要写进 JSON），而 UserIDKey 是 server 内部 ctx key（不出 JSON）。两者命名风格不一致是**故意**（标识"哪些是契约值 / 哪些是内部值"）—— 与 ResponseErrorCodeKey (`response_error_code`) 同源约束
- **ctx propagation 兼容**：本中间件**不**做 `c.Request = c.Request.WithContext(ctxWithUserID)` 二次包装。`c.Set(UserIDKey, ...)` 已经让 handler `c.Get(UserIDKey)` 可见；future service 层若需要从原生 `context.Context` 读 userID（脱离 gin），应当走 service 函数显式参数 `func(ctx, userID, ...)`（ADR-0007 §2.1 钦定的"导出函数第一参数 ctx"），而**不**通过 ctx.Value 隐式传 —— ADR-0007 §6 反对"用 ctx.Value 传业务字段"
- **不依赖 ctx 取消**：本中间件是同步 path（header 解析 + Verify < 1µs CPU），不需要 select ctx.Done()；ctx cancel 时下游 handler 不会被本中间件阻塞，自然进入 c.Next() 的 handler 层处理

**关键反模式**：

- ❌ **不**用 `strings.HasPrefix(rawHeader, "Bearer ")` 大小写敏感判 —— RFC 6750 钦定 scheme 大小写不敏感；client（如 iOS URLSession 自动大写化）实测有差异
- ❌ **不**从 query string / cookie / 其他 header 读 token：V1 §2.3 钦定**仅** `Authorization: Bearer` 一处；多 source 增加攻击面（query string token 被日志 / referer 泄露）
- ❌ **不**用 `c.Request.Header.Get` 之外的方式（如直接 reflect c.Request.Header）—— Gin 的 GetHeader 已 normalize header name（HTTP/2 是小写）
- ❌ **不**在中间件内调 `response.Error(...)` 直接写 envelope —— 违反 ADR-0006 "ErrorMappingMiddleware 是唯一 envelope producer"，docs/lessons/2026-04-24-error-envelope-single-producer.md 钦定该原则
- ❌ **不**在 token 失败时打 INFO/WARN log（让 Logging 中间件统一打）—— 双重日志冗余 + 时序不可控（auth 中间件先于 Logging 的 c.Next 之后段执行的话）
- ❌ **不**用 `c.AbortWithError(http.StatusUnauthorized, err)` —— 该函数同时设置 HTTP status 401，但 V1 §2.4 钦定**业务码与 HTTP status 正交**（1001 走 200 + envelope.code=1001，**不**走 401）；用 `c.Error + c.Abort` 让 ErrorMappingMiddleware 决策 status
- ❌ **不**在 token 通过后把 entire `Claims` struct（含 IssuedAt / ExpiresAt）写 c.Set —— 4.6 / 4.8 业务只需要 userID；多写字段增加 API surface 反而让 future 重构成本高

**AC2 — `internal/app/http/middleware/rate_limit.go`：限频中间件**

新增 `server/internal/app/http/middleware/rate_limit.go`：

```go
// RateLimit 中间件工厂：基于内存 token bucket 实现按 key 的限频。
//
// # 维度策略
//
// 本 story 的 RateLimit 接受一个 key 提取函数，让调用方决定按 IP 还是 userID 限频：
//
//   r := bootstrap.NewRouter()
//   api := r.Group("/api/v1")
//
//   // 登录前路径：未登录 → 没 userID → 用 IP 维度（V1 §4.1 行 218 钦定 "同 IP 每分钟 60 次"）
//   authGroup := api.Group("/auth", middleware.RateLimit(cfg.RateLimit, middleware.RateLimitByIP))
//
//   // 登录后路径：从 c.Keys[UserIDKey] 取 userID；fallback IP（防御 auth 中间件未挂的边缘情况）
//   loggedInGroup := api.Group("", middleware.Auth(signer), middleware.RateLimit(cfg.RateLimit, middleware.RateLimitByUserID))
//
// **MVP 选择单一维度**：
//   - 登录前 = IP 维度（防 brute force 注册 / 撞 guestUid）
//   - 登录后 = userID 维度（防同 user 高频调业务接口；同 IP 多 user 不互相影响 NAT 共享场景）
//
// # 速率配置
//
// 配置来源 cfg.RateLimit:
//   - PerKeyPerMin: 每 key 每分钟允许的请求数（默认 60；epics.md 行 1039 钦定）
//   - BurstSize:    令牌桶容量（默认 = PerKeyPerMin；瞬时突发上限）
//   - BucketsLimit: 内存桶 map 上限（防 IP 洪泛攻击耗内存；默认 10000）
//
// PerKeyPerMin / 60 = 每秒平均速率；BurstSize 是 burst 上限。例：
//   PerKeyPerMin=60 / BurstSize=60 → 平均每秒 1 次，瞬时可 60 次后回填
//   PerKeyPerMin=60 / BurstSize=10 → 平均每秒 1 次，瞬时只能 10 次（更保守）
//
// MVP 默认 burst = PerKeyPerMin（与 epics.md 行 1047 "1 分钟内 60 次内 → 通过"测试场景一致）。
//
// # 内存管理（防洪泛）
//
// 内部用 sync.Map 存 key → *rate.Limiter；新 key 进入时 atomic increment counter。
// 当 counter ≥ BucketsLimit → 不再为新 key 创建 bucket，而是用一个**共享降级 bucket**
// 给所有溢出 key 限流 —— 等价于"所有溢出 key 共享同一速率"，OK 防 OOM 又不至于 100%
// 拒绝合法用户。
//
// **不**起独立 cleanup goroutine：
//   1. 每个 limiter 内存约 ~100 字节（sync.Mutex + token state）→ 10000 上限 ~1MB，可接受
//   2. 节点 2 单实例部署 → server 进程重启会自然清空（k8s rolling deploy 频次 ≥ 几小时）
//   3. cleanup goroutine 引入额外复杂度（select + ticker + 安全停机）—— 不值得
//   4. 节点 10+ 切 Redis-based 后，问题自然消失（Redis 自身 eviction 处理过期 bucket）
//
// # 错误映射
//
// 超限 → c.Error(apperror.New(ErrTooManyRequests, "操作过于频繁")) + c.Abort；
// ErrorMappingMiddleware 写 1005 envelope。
//
// # 时间源
//
// 用 time.Now() 直接调（与 4.4 token util 同思路；不引入 clock interface）。
// "跨分钟边界重置计数" 测试用 fake limiter rate（超短窗口）+ time.Sleep 跨真实时间。
//
// # 配置 reload
//
// MVP **不**支持 hot reload：YAML 改了要重启 server。Future epic 加 SIGHUP / config 监控。
type KeyExtractor func(c *gin.Context) string

// RateLimitByIP 用 c.ClientIP() 作为 key（已经处理 X-Forwarded-For / X-Real-IP）。
func RateLimitByIP(c *gin.Context) string {
    return "ip:" + c.ClientIP()
}

// RateLimitByUserID 优先用 UserIDKey，fallback IP（防御 auth 中间件未挂场景）。
//
// 当 auth 中间件已挂在前面时，UserIDKey 必然存在 → 走 userID 维度（同 IP 多 user 隔离）。
// 边缘情况（dev 误配 / 测试漏挂 auth）→ fallback IP，至少有限频保底。
func RateLimitByUserID(c *gin.Context) string {
    if v, ok := c.Get(UserIDKey); ok {
        if uid, ok := v.(uint64); ok {
            return fmt.Sprintf("user:%d", uid)
        }
    }
    return "ip:" + c.ClientIP()
}

// RateLimit 工厂。cfg 校验由调用方负责（建议在 bootstrap 期检查；nil cfg → panic）。
func RateLimit(cfg config.RateLimitConfig, extractor KeyExtractor) gin.HandlerFunc {
    if extractor == nil {
        panic("middleware.RateLimit: extractor must not be nil")
    }
    if cfg.PerKeyPerMin <= 0 {
        panic("middleware.RateLimit: PerKeyPerMin must be > 0")
    }
    if cfg.BurstSize <= 0 {
        cfg.BurstSize = cfg.PerKeyPerMin
    }
    if cfg.BucketsLimit <= 0 {
        cfg.BucketsLimit = 10000
    }
    // 每秒速率 = 每分钟 / 60
    perSec := rate.Limit(float64(cfg.PerKeyPerMin) / 60.0)
    burst := cfg.BurstSize

    var (
        buckets sync.Map      // map[string]*rate.Limiter
        count   atomic.Int64
        // 共享降级 bucket：当 buckets 数达上限时，所有新 key 共用此 limiter
        // 防御 IP 洪泛把内存撑爆
        overflow = rate.NewLimiter(perSec, burst)
    )

    return func(c *gin.Context) {
        key := extractor(c)
        if key == "" {
            // 取不到 key（极罕见：c.ClientIP() 为空 + 没 userID）
            // → 不限频放行（保守路径，防误伤）。Future 可改为 1005
            c.Next()
            return
        }

        var lim *rate.Limiter
        if v, ok := buckets.Load(key); ok {
            lim = v.(*rate.Limiter)
        } else if count.Load() < int64(cfg.BucketsLimit) {
            newLim := rate.NewLimiter(perSec, burst)
            actual, loaded := buckets.LoadOrStore(key, newLim)
            lim = actual.(*rate.Limiter)
            if !loaded {
                count.Add(1)
            }
        } else {
            lim = overflow
        }

        if !lim.Allow() {
            _ = c.Error(apperror.New(apperror.ErrTooManyRequests, apperror.DefaultMessages[apperror.ErrTooManyRequests]))
            c.Abort()
            return
        }
        c.Next()
    }
}
```

**关键设计约束**：

- **依赖 `golang.org/x/time/rate`**：Go 官方 sub-repo，token bucket 算法成熟实装；已是 indirect dep（`go list -m golang.org/x/time` 显示 v0.5.0）。本 story 在 `go.mod` 把 indirect 提升为直接依赖（`go get golang.org/x/time@latest`）。**禁止**手写 token bucket（review 会问"为什么不用 stdlib 的成熟库 + 测试覆盖完善的"）
- **`sync.Map` 选型**：高并发读多写少（同一 IP / userID 重复请求是 read 路径；新 key 是 write 路径）；`sync.Map.LoadOrStore` 防双重创建竞态。**不**用 `map + RWMutex`：sync.Map 在本场景性能更优 + 代码更简洁
- **`atomic.Int64` 计数器**：sync.Map 没暴露 size 方法；用独立计数器 + LoadOrStore 的 `loaded` 返回值 atomic 维护数量
- **`overflow` 共享 limiter**：防御性 + 防 OOM；正常负载下永远不触发，仅 abnormal load（IP 洪泛）才走该路径
- **配置入口 `cfg config.RateLimitConfig`**：与 4.2 / 4.4 同模式，配置 struct 在 `internal/infra/config/config.go` 定义（详见 AC4）
- **`KeyExtractor` 函数式接口**：让调用方决策维度（IP / userID / future "tenantID + IP"）；不硬编码维度让 4.6 / 4.8 路由组各自选择
- **不依赖 Redis**：epics.md 行 1038 钦定"基于内存 token bucket 实现（MVP 不依赖 Redis）"；NFR12 / Story 6.3 / Story 10.6 阶段才迁 Redis（**tech debt**：Story 6.3 文档同步 + tech debt log 已登记本条）

**关键反模式**：

- ❌ **不**用 leaky bucket / sliding window log / sliding window counter：token bucket 是标准实装；switching 到其他算法需要充分理由（performance / 公平性），MVP 不需要
- ❌ **不**对 `/ping` / `/version` / `/metrics` 挂 rate_limit：health check 自己被限流是经典蠢坑（监控告警风暴时 rate_limit 把 prometheus 拉数据也拒了，看不到 server 活信号）。本 story 在 router.go 设计层面避开（详见 AC4）
- ❌ **不**实装 IP whitelist（无限频）：MVP 不需要；future epic 决策
- ❌ **不**返回 `429 Too Many Requests` HTTP status：V1 §2.4 钦定业务码与 HTTP status 正交，1005 走 200 + envelope.code=1005（与 1001 同模式）
- ❌ **不**返回 `Retry-After` header：V1 §3 没钦定，client 不依赖；future 加可选
- ❌ **不**在 limiter 中用 channel-based 实装（如 `time.Tick + chan struct{}`）：channel 实装的"取消"语义 / cleanup 复杂；rate.Limiter 是状态机不需要 goroutine
- ❌ **不**在中间件内每次 c.ClientIP() 调多次：缓存到 key 字符串后只调一次（避免 IP parsing overhead）
- ❌ **不**让 `Allow()` 返回 false 时调 `c.AbortWithStatus(429)`：理由同 AC1 反模式（envelope status 决策权在 ErrorMappingMiddleware）

**AC3 — `internal/infra/config/`：RateLimitConfig struct + 默认值兜底**

修改 `server/internal/infra/config/config.go` + `loader.go` + `local.yaml`：

`config.go` 新增：

```go
type Config struct {
    Server    ServerConfig    `yaml:"server"`
    MySQL     MySQLConfig     `yaml:"mysql"`
    Auth      AuthConfig      `yaml:"auth"`
    RateLimit RateLimitConfig `yaml:"ratelimit"` // ★ 本 story 新增
    Log       LogConfig       `yaml:"log"`
}

// RateLimitConfig 是限频中间件配置。Story 4.5 引入；选型 / 默认值由 epics.md
// §Story 4.5（行 1039）+ V1 §4.1 行 218 钦定。
//
// 字段不在 config 包做业务校验（无 Validate 方法），fail-fast 由 middleware.RateLimit
// 工厂函数承担：PerKeyPerMin ≤ 0 → panic（启动期就暴露，与 4.4 auth.New 同模式）。
//
// **节点 2 阶段是内存 token bucket**：单实例部署 OK；多实例 / 节点 10+ Story 10.6
// 接 Redis 后，本 struct 加 `Backend string yaml:"backend"` 字段切换实装。
type RateLimitConfig struct {
    // PerKeyPerMin 是每 key（IP 或 userID）每分钟允许的请求数。
    //
    // 默认 60（epics.md 行 1039 + V1 §4.1 行 218 钦定）。可调小（如 30）保守
    // 限频或调大（如 120）放宽（但 > 600 接近"无限频"，违反限频初衷）。
    //
    // 范围限制：(0, 600]，由 middleware.RateLimit 工厂在启动期校验；超过 → panic。
    PerKeyPerMin int `yaml:"per_key_per_min"`

    // BurstSize 是 token bucket 容量（瞬时突发上限）。
    //
    // 默认 = PerKeyPerMin（让 epics.md 钦定的"1 分钟内 60 次内 → 通过"测试 happy
    // path 一次 burst 60 也能通过；epics.md 行 1047 钦定的语义就是 burst-friendly）。
    //
    // 调小（如 10）让 burst 更保守 —— 防 client bug 突发雪崩 server。
    BurstSize int `yaml:"burst_size"`

    // BucketsLimit 是内存中保存的 bucket 数上限（防 IP 洪泛 OOM）。
    //
    // 默认 10000（约 1MB 内存）。每个 limiter ~100 字节；超过该上限的新 key 走
    // 共享降级 bucket。生产单实例 QPS 不高（节点 2 阶段未对外发布）→ 10000 远超
    // 实际负载。
    //
    // 节点 10+ 切 Redis 后本字段废弃（Redis 自身 eviction 处理）。
    BucketsLimit int `yaml:"buckets_limit"`
}
```

`loader.go` 新增默认值兜底：

```go
const (
    // ...existing consts...
    defaultRateLimitPerKeyPerMin = 60    // epics.md 行 1039 + V1 §4.1 行 218
    defaultRateLimitBucketsLimit = 10000 // ~1MB 内存上限
)

// 在 Load 函数末尾的"默认值兜底"段加：
if cfg.RateLimit.PerKeyPerMin == 0 {
    cfg.RateLimit.PerKeyPerMin = defaultRateLimitPerKeyPerMin
}
if cfg.RateLimit.BurstSize == 0 {
    cfg.RateLimit.BurstSize = cfg.RateLimit.PerKeyPerMin
}
if cfg.RateLimit.BucketsLimit == 0 {
    cfg.RateLimit.BucketsLimit = defaultRateLimitBucketsLimit
}
```

`local.yaml` 新增 `ratelimit:` 段：

```yaml
ratelimit:
  # 每 key（IP 或 userID）每分钟允许的请求数。epics.md §Story 4.5 行 1039 钦定默认 60；
  # 节点 2 阶段不对外发布，60 完全够用。生产 launch 阶段（Epic 36）按需调整。
  per_key_per_min: 60
  # token bucket 容量（瞬时突发上限）。默认 = per_key_per_min（让 60 次连发也能过）。
  burst_size: 60
  # 内存 bucket 数上限（防 IP 洪泛 OOM）。约 1MB 内存；节点 10+ 切 Redis 后此项废弃。
  buckets_limit: 10000
```

**关键约束**：

- **`ratelimit:` 与 `auth:` 段独立**：语义边界清晰（auth 是密钥语义，ratelimit 是策略参数）；future 切换 Redis backend 时不污染 auth 段
- ****不**给 `per_key_per_min` 加 env override**：节点 2 阶段所有 env（dev / staging / prod）默认 60 即可，不需要按 env 调。Future Epic 36 部署阶段如需差异化，再加 env override（与 token_secret 加 env 同模式）
- **没有 secret 类字段**：本 struct 全是非敏感参数，可放 checked-in YAML 默认值（与 [`docs/lessons/2026-04-26-checked-in-config-must-boot-default.md`](../../docs/lessons/2026-04-26-checked-in-config-must-boot-default.md) 钦定的"非 secret 字段必须 fresh clone 直接跑" 一致）
- **`BurstSize == PerKeyPerMin` 默认**：让 epics.md 行 1047 钦定的"1 分钟内 60 次内 → 通过" 测试 happy path 一次 burst 60 也能通过 —— 否则 burst=10 时第 11 次就会限流，与"1 分钟内 60 次"语义矛盾
- **范围 `(0, 600]` 校验在 middleware**：config 包不做业务校验是 4.2 / 4.4 已建立的惯例（fail-fast 由消费方承担）

**AC4 — `internal/app/bootstrap/router.go`：保持现有路由不变 + 提供工厂函数让 4.6 / 4.8 复用**

**重点**：本 story **不**在 `bootstrap.NewRouter()` 默认挂 `Auth` / `RateLimit`（因为还没业务路由组；4.6 落地 `/auth/guest-login` 才挂第一个）。修改 `internal/app/bootstrap/server.go` 把 `signer` + `rateLimitCfg` 透传到一个新增的 `NewRouterWithDeps` 工厂（或修改现有 `NewRouter` 接受可选 deps），让 4.6 落地时调用：

```go
// 4.6 落地时的预期形态（本 story 不实装；仅给 4.6 留接口）：
//
// internal/app/bootstrap/router.go 修改后：
//
//   func NewRouter(deps Deps) *gin.Engine {
//       r := gin.New()
//       r.Use(middleware.RequestID(), middleware.Logging(),
//             middleware.ErrorMappingMiddleware(), middleware.Recovery())
//
//       // 运维端点（保持公开 + 高频可用，**不**挂 auth / rate_limit）
//       r.GET("/ping", handler.PingHandler)
//       r.GET("/version", handler.VersionHandler)
//       r.GET("/metrics", gin.WrapH(metrics.Handler()))
//
//       // 业务路由组（4.6 / 4.8 落地时填充 handler）
//       api := r.Group("/api/v1")
//
//       // /auth 子组：rate_limit by IP（V1 §4.1 钦定），**不**挂 auth
//       authGroup := api.Group("/auth", middleware.RateLimit(deps.RateLimitCfg, middleware.RateLimitByIP))
//       // authGroup.POST("/guest-login", authHandler.GuestLogin)  // 4.6 加
//
//       // 已认证子组：先 auth 再 rate_limit by userID
//       authedGroup := api.Group("",
//           middleware.Auth(deps.Signer),
//           middleware.RateLimit(deps.RateLimitCfg, middleware.RateLimitByUserID),
//       )
//       _ = authedGroup // 4.8 / 后续 epic 加 GET /me / /home / 等 handler
//
//       devtools.Register(r)
//       return r
//   }
//
// type Deps struct {
//     Signer       *auth.Signer
//     RateLimitCfg config.RateLimitConfig
//     // 后续：GormDB / TxMgr / 各 service 单例 ...
// }
```

**本 story 实装范围**：

- 修改 `internal/app/bootstrap/router.go` —— 引入 `Deps` struct 接收 `*auth.Signer` + `config.RateLimitConfig`，但**不**把 Auth / RateLimit 挂到任何具体路由（因为本 story 还没有 /api/v1 业务 handler）。**新增** `/api/v1` empty group（只挂 ratelimit 全局）以验证 wiring 通；4.6 落地时往该 group 填 handler
- 修改 `internal/app/bootstrap/server.go` —— `Run` 签名扩展加 `rateLimitCfg config.RateLimitConfig` 参数（继 4.4 加 signer 之后再扩一个；**或**收敛为 `Deps struct` 一次传全 —— 推荐收敛，避免 Run 签名继续膨胀）
- 修改 `cmd/server/main.go` —— 把 `cfg.RateLimit` 通过 Deps 传给 `bootstrap.Run`
- 修改 `internal/app/bootstrap/server_test.go` 等已有测试 —— 适配新签名（沿用 nil-tolerant 模式：`Run(ctx, cfg, nil, nil, nil, config.RateLimitConfig{PerKeyPerMin: 60, BurstSize: 60, BucketsLimit: 100})` 即可，或类似 `Deps{}` 零值）

**关键约束**：

- **`/ping` / `/version` / `/metrics` 不挂 rate_limit / auth**：health check / 监控自爆是反模式；保持现状（只走 RequestID / Logging / ErrorMapping / Recovery 四件套）
- **`Deps struct` vs `Run(...)` 平铺参数**：本 story 是第二次扩 bootstrap.Run 签名（4.4 加 signer 是第一次）；**强烈建议**收敛为 `Deps struct`（避免后续 Story 4.6 / 4.8 / Epic 5+ 每加一个依赖就改 Run 签名）。`Deps{}` 零值兼容（与 4.4 的 nil signer 测试路径同模式）
- **`Deps` struct 字段**（本 story 落地）：
  ```go
  type Deps struct {
      GormDB       *gorm.DB
      TxMgr        tx.Manager
      Signer       *auth.Signer
      RateLimitCfg config.RateLimitConfig
  }
  ```
  4.6 / 4.8 / Epic 5+ 加 service / handler 依赖时往本 struct 加字段，bootstrap.Run 签名不变
- **现有 `Run(ctx, cfg, gormDB, txMgr, signer)` 签名兼容路径**：要么改成 `Run(ctx, cfg, deps Deps)` 一次性收敛（本 story 推荐）；要么再扩一个 `Run(ctx, cfg, gormDB, txMgr, signer, rateLimitCfg)`（不推荐 —— 三次 churn）
- **`bootstrap.NewRouter` 必须接受 `Deps`**：4.6 落地时 bootstrap.Run 内部调 `router := NewRouter(deps)` 时把 deps 透传；本 story 完成 NewRouter 签名扩展（即使路由层暂不消费 Deps.Signer / Deps.RateLimitCfg，4.6 落地时无需再改 NewRouter）

**AC5 — 单元测试覆盖（≥6 case，对齐 epics.md 行 1043-1049）**

新增 `server/internal/app/http/middleware/auth_test.go` + `rate_limit_test.go`，覆盖：

```go
// auth_test.go
package middleware_test

import (
    "errors"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/huing/cat/server/internal/app/http/middleware"
    "github.com/huing/cat/server/internal/pkg/auth"
    apperror "github.com/huing/cat/server/internal/pkg/errors"
)

// AC5.1 auth happy: 合法 token → 通过 + userID 注入 c.Get（epics.md 行 1044）
func TestAuth_ValidToken_PassesAndInjectsUserID(t *testing.T)

// AC5.2 auth edge: 无 Authorization header → 1001（epics.md 行 1045）
func TestAuth_NoAuthorizationHeader_Returns1001(t *testing.T)

// AC5.3 auth edge: 错误 scheme（如 "Basic xxx" / 无前缀 / 多空格）→ 1001
func TestAuth_InvalidScheme_Returns1001(t *testing.T)

// AC5.4 auth edge: token 过期 → 1001 + cause 链含 auth.ErrTokenExpired（epics.md 行 1046）
func TestAuth_ExpiredToken_Returns1001WithExpiredCause(t *testing.T)

// AC5.5 auth edge: token 篡改 / 格式错 → 1001 + cause 链含 auth.ErrTokenInvalid
func TestAuth_TamperedToken_Returns1001WithInvalidCause(t *testing.T)

// rate_limit_test.go
// AC5.6 rate_limit happy: 1 分钟内 60 次内 → 通过（epics.md 行 1047）
func TestRateLimit_60RequestsIn1Minute_Pass(t *testing.T)

// AC5.7 rate_limit edge: 1 分钟内第 61 次 → 1005（epics.md 行 1048）
func TestRateLimit_61stRequestIn1Minute_Returns1005(t *testing.T)

// AC5.8 rate_limit edge: 跨分钟边界 → 重置计数 / 平滑 token 回填（epics.md 行 1049）
func TestRateLimit_CrossMinuteBoundary_ResetsTokens(t *testing.T)

// AC5.9 rate_limit edge: 不同 key 隔离 → IP A 满了不影响 IP B
func TestRateLimit_DifferentKeysIsolated(t *testing.T)

// AC5.10 rate_limit edge: BucketsLimit 触发降级 bucket
func TestRateLimit_OverflowBuckets_FallsBackToSharedLimiter(t *testing.T)
```

**AC5.1 实装要点**：

```go
signer, err := auth.New("test-secret-must-be-at-least-16-bytes", 3600)
require.NoError(t, err)
tok, err := signer.Sign(12345, 0)
require.NoError(t, err)

gin.SetMode(gin.TestMode)
r := gin.New()
r.Use(middleware.Auth(signer))
r.GET("/test", func(c *gin.Context) {
    v, ok := c.Get(middleware.UserIDKey)
    require.True(t, ok)
    uid, ok := v.(uint64)
    require.True(t, ok)
    assert.Equal(t, uint64(12345), uid)
    c.JSON(http.StatusOK, gin.H{"ok": true})
})

req := httptest.NewRequest("GET", "/test", nil)
req.Header.Set("Authorization", "Bearer "+tok)
w := httptest.NewRecorder()
r.ServeHTTP(w, req)

assert.Equal(t, http.StatusOK, w.Code)
```

**AC5.2 实装要点**（无 Authorization header）：

```go
r := gin.New()
// 加 ErrorMappingMiddleware 让 envelope 写入响应（不挂的话 c.Error 后 body 为空）
r.Use(middleware.ErrorMappingMiddleware())
r.Use(middleware.Auth(signer))
r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{}) })

req := httptest.NewRequest("GET", "/test", nil)
// 不设 Authorization header
w := httptest.NewRecorder()
r.ServeHTTP(w, req)

assert.Equal(t, http.StatusOK, w.Code) // V1 §2.4：业务码与 HTTP 正交，1001 走 200
var envelope struct {
    Code int `json:"code"`
}
require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
assert.Equal(t, apperror.ErrUnauthorized, envelope.Code)
```

**AC5.3 实装要点**（table-driven 多种错误 scheme）：

```go
testCases := []struct {
    name       string
    authHeader string
}{
    {"basic-scheme", "Basic abc"},
    {"no-prefix-only-token", "raw-token-without-prefix"},
    {"empty-after-bearer", "Bearer "},
    {"only-spaces-after-bearer", "Bearer    "},
    {"lowercase-bearer", "bearer xyz"}, // 实测：scheme 大小写不敏感，但 token "xyz" 太短不合法 → 仍返 1001
    {"tab-separator", "Bearer\txyz"},   // tab 不是合法 RFC 6750 separator
}
for _, tc := range testCases {
    t.Run(tc.name, func(t *testing.T) {
        // 重新构 router
        // ...
        req.Header.Set("Authorization", tc.authHeader)
        // ...
        assert.Equal(t, apperror.ErrUnauthorized, envelope.Code)
    })
}
```

**注意**：`lowercase-bearer` 的预期行为是 **scheme** 通过（大小写不敏感），但 token `"xyz"` 在 Verify 时会失败（格式错 → ErrTokenInvalid → 1001）；最终响应仍是 1001。如果要专门测 scheme case-insensitivity，应该**用合法 token 测大小写不敏感的 happy path**（额外加一个 case）。

**AC5.4 实装要点**（过期 token；用 `expireSec=1` + `time.Sleep(1100ms)` 跨秒边界）：

```go
signer, _ := auth.New("test-secret-must-be-at-least-16-bytes", 3600)
tok, _ := signer.Sign(1, 1) // 1 秒过期
time.Sleep(1100 * time.Millisecond)

// 直接调 middleware 检查 c.Errors[0].Err 链
gin.SetMode(gin.TestMode)
r := gin.New()
var capturedErr error
r.Use(middleware.Auth(signer))
r.Use(func(c *gin.Context) {
    if len(c.Errors) > 0 {
        capturedErr = c.Errors[0].Err
    }
})
r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{}) })

req := httptest.NewRequest("GET", "/test", nil)
req.Header.Set("Authorization", "Bearer "+tok)
w := httptest.NewRecorder()
r.ServeHTTP(w, req)

require.Error(t, capturedErr)
ae, ok := apperror.As(capturedErr)
require.True(t, ok)
assert.Equal(t, apperror.ErrUnauthorized, ae.Code)
// cause 链应包含 auth.ErrTokenExpired sentinel（4.5 future 给 logger 区分日志级别用）
assert.True(t, errors.Is(capturedErr, auth.ErrTokenExpired),
    "expected cause chain to include auth.ErrTokenExpired, got %v", capturedErr)
```

**注意**：本 case 含 1.1s sleep（与 4.4 TestVerify_Expired 同源约束 —— HS256 时间分辨率秒级）；`t.Parallel()` 让多个慢 case 并发跑。

**AC5.5 实装要点**（篡改 token；按 [`docs/lessons/2026-04-26-jwt-tamper-test-must-mutate-non-terminal-byte.md`](../../docs/lessons/2026-04-26-jwt-tamper-test-must-mutate-non-terminal-byte.md) 钦定改 payload 段首字节，**不**改末尾字符）：

```go
tok, _ := signer.Sign(1, 3600)
firstDot := strings.Index(tok, ".")
require.Greater(t, firstDot, -1)
payloadStart := firstDot + 1
original := tok[payloadStart]
swap := byte('A')
if original == 'A' { swap = 'B' }
tampered := tok[:payloadStart] + string(swap) + tok[payloadStart+1:]

// header: "Bearer "+tampered → 走 Verify → ErrTokenInvalid → 1001 with cause
// ... 同 AC5.4 断言 errors.Is(err, auth.ErrTokenInvalid)
```

**AC5.6 实装要点**（happy path 60 次连发）：

```go
cfg := config.RateLimitConfig{PerKeyPerMin: 60, BurstSize: 60, BucketsLimit: 100}
r := gin.New()
r.Use(middleware.RateLimit(cfg, middleware.RateLimitByIP))
r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{}) })

for i := 0; i < 60; i++ {
    req := httptest.NewRequest("GET", "/test", nil)
    req.RemoteAddr = "1.2.3.4:12345"
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)
    assert.Equal(t, http.StatusOK, w.Code, "request %d should pass", i+1)
}
```

**AC5.7 实装要点**（第 61 次拒）：

```go
// 同 AC5.6 一次连发 60 次后，第 61 次必拒
// ...
req := httptest.NewRequest("GET", "/test", nil)
req.RemoteAddr = "1.2.3.4:12345"
w := httptest.NewRecorder()
r.ServeHTTP(w, req) // 第 61 次

assert.Equal(t, http.StatusOK, w.Code) // V1 §2.4：业务码 vs HTTP status 正交
var envelope struct {
    Code int `json:"code"`
}
require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
assert.Equal(t, apperror.ErrTooManyRequests, envelope.Code)
```

**AC5.8 实装要点**（跨分钟边界；用 `PerKeyPerMin=60 / BurstSize=60` 时 token bucket 每秒回填 1 个）：

```go
// 一次连发 60 次耗光 → 第 61 次拒 → time.Sleep(1s) → token 回填 1 个 → 第 62 次通过
// ...connect 60 times burst...
time.Sleep(1100 * time.Millisecond) // 跨秒（让 token bucket 回填 ≥ 1 个 token）

req := httptest.NewRequest("GET", "/test", nil)
req.RemoteAddr = "1.2.3.4:12345"
w := httptest.NewRecorder()
r.ServeHTTP(w, req)
assert.Equal(t, http.StatusOK, w.Code)
// 此时 envelope.code 应该是 0（成功），不是 1005
```

**注意**：epics.md 行 1049 钦定的"跨分钟边界 → 重置计数"语义在 token bucket 实装下表现为"持续平滑回填"而**不是**"分钟边界 hard reset"。这是 token bucket 与 fixed-window 算法的本质区别。本测试用 1.1s sleep 跨秒边界验证 token 回填即可（不需要等满 60s）；**本 story 在 Dev Notes 段会解释这点 + 其与 epics.md 字面表述的兼容性**。

**AC5.9 实装要点**（不同 key 隔离）：

```go
// IP A 连发 60 次耗光 + IP A 第 61 次拒 → IP B 第 1 次通过
for i := 0; i < 60; i++ {
    sendRequest("1.2.3.4", t, r)
}
sendRequest("1.2.3.4", t, r) // 第 61 次拒
sendRequest("5.6.7.8", t, r) // IP B 第 1 次通过
```

**AC5.10 实装要点**（BucketsLimit 防 IP 洪泛）：

```go
cfg := config.RateLimitConfig{PerKeyPerMin: 60, BurstSize: 60, BucketsLimit: 5} // 故意小
r := gin.New()
r.Use(middleware.RateLimit(cfg, middleware.RateLimitByIP))
r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{}) })

// 用 6 个不同 IP 各打 1 次 → 第 6 个 IP 触发降级 bucket
// 后续验证：第 6 个 IP 与第 1 个 IP **不**共享 bucket（隔离仍生效），
// 但第 6 个 IP 与第 7 / 第 8 个 IP **共享降级 bucket**
// （断言所有降级 IP 总流量受单个 limiter 限制）

// 简化版：连开 6 个 IP，让第 6 个走 overflow → 然后从第 6 个连发 100 次
// 中应该有大量请求被 1005（因为它们与未知数量的"未来 IP"共享同一 60/min 限额）
```

**关键约束**：

- 全部用 `testing.T` + `testify/assert` + `testify/require`（已有依赖）；**不**额外引入 mock 框架
- AC5.4 / AC5.8 之外所有 case ≤ 100ms / case；AC5.4 / AC5.8 各含 1.1s sleep；用 `t.Parallel()` 让慢 case 并发跑
- **不**用 `dockertest` / `sqlmock` / `httptest.NewServer`：本 story 是纯 HTTP 中间件单测，用 `httptest.NewRecorder()` + `r.ServeHTTP(w, req)` 即可（与 4.4 的纯单测同模式）
- **集成测试**：本 story **不**写独立集成测试 —— 中间件 wiring 由 Story 4.7 Layer 2 集成测试（端到端真 HTTP + 真 service）覆盖；**但**修改现有 `error_mapping_integration_test.go` 验证 auth + rate_limit 已在 router 链中生效（轻量集成 case，详见 AC6）
- 测试 fixture 模式参考 `internal/app/http/middleware/error_mapping_test.go`（已建立测试模板，如 newErrorMappingRouter 工厂、envelope 解析 helper）

**AC6 — 端到端集成验证 + `bash scripts/build.sh` 全量绿**

完成后必须能跑通：

```bash
bash scripts/build.sh                    # vet + build → 不报错（含 middleware/auth.go / rate_limit.go 新文件 + bootstrap router 改动）
bash scripts/build.sh --test             # 单测全过（含 ≥10 个新 middleware case + 现有所有）
bash scripts/build.sh --race --test      # race 全过（Linux/CI 必过；Windows ThreadSanitizer 内存问题按 ADR-0001 §3.5 skip）
```

**集成层补丁**：扩展 `internal/app/http/middleware/error_mapping_integration_test.go`（已有测试文件）补 1-2 条 case：

```go
// TestAuth_E2E_HappyAndUnauthorized 验证 Auth 中间件在 bootstrap 真 router 链上端到端工作。
// 用 httptest.NewServer + bootstrap.NewRouter（注入 Auth 中间件） + 真 HTTP roundtrip。
// 与 error_mapping_integration_test.go 同模式（已有 newRouterWithMiddleware 范例）。

// TestRateLimit_E2E_61stReturns1005 验证 RateLimit 在真 router 链 + 真 HTTP 上 61 次后 1005。
```

**关键约束**：

- 单测层 ≤ 6s 整体跑完（含 AC5.4 / AC5.8 各 1.1s sleep；其他 case 全在 100ms 内）
- **不**改 `bash scripts/build.sh` 自身（脚本契约由 Story 1.7 钉死，4.4 review 已验证）
- **不**新增 `--integration` 路径：本 story 是纯单测 + 轻量端到端 httptest；不需要 dockertest

**AC7 — `go.mod` / `go.sum` 更新**

- `cd server && go get golang.org/x/time@latest`（拉最新稳定版，预期 v0.5.0+；当前已是 indirect dep）
- `cd server && go mod tidy` → 确认 `go.mod` require 段加 `golang.org/x/time vX.Y.Z` 直接依赖
- 确认 `go mod verify` 不报错
- `go.sum` 同步更新（自动）

**关键约束**：

- **不**升级其他依赖（保持 4.3/4.4 已 pin 的 `golang-migrate v4.18.1` / GORM v1.25.12 / Gin v1.12.0 / `golang-jwt/jwt/v5 v5.3.1` 等不变）
- 如果 `go mod tidy` 意外删除 indirect 依赖（理论应该没有 —— x/time 仅是从 indirect → direct 升级，不影响 graph），dev 阶段需要手工核对一遍 `go.mod` diff，确认仅 require 段有变化

**AC8 — README / docs 不更新**

本 story **不**更新：

- `README.md` / `server/README.md`：rate_limit 配置说明留给 Epic 4 收尾或 Epic 36 部署 story；本 story 在 Completion Notes 登记 tech debt
- `docs/宠物互动App_*.md` 任一份：本 story 严格对齐 V1 §2.3 / §3 / §4.1 + 设计文档 §8.2 + epics.md §Story 4.5，**消费方**不是修改方
- `docs/lessons/` 任一份：本 story 不主动写 lesson；review 阶段发现新坑由 fix-review 阶段写 lesson（epic-loop 流水线分工）

**关键约束**：

- 如果 dev 阶段实装时发现某条 AC 与文档冲突 / 漏洞 / 暗坑，**不**自行修文档，**而是**在 Completion Notes 里登记 issue + 让 fix-review 处理
- README 缺失"如何调整 rate_limit 配置"是已知 tech debt；本 story Completion Notes 必须明确登记
- **tech debt log 必须新增条目**：`_bmad-output/implementation-artifacts/tech-debt-log.md`（如该文件不存在，由 Story 6.3 文档同步阶段创建；本 story 在 Completion Notes 登记两条：(1) 内存 token bucket 多实例不共享计数；(2) 限频 metric 缺失 prometheus 专属指标）

## Tasks / Subtasks

- [x] **Task 1（AC3）**：实装 `internal/infra/config/` RateLimitConfig + 默认值兜底
  - [x] 1.1 修改 `server/internal/infra/config/config.go` 加 `RateLimitConfig` struct + `Config.RateLimit` 字段（yaml tag `ratelimit`）
  - [x] 1.2 修改 `server/internal/infra/config/loader.go` 加 `defaultRateLimitPerKeyPerMin = 60` / `defaultRateLimitBucketsLimit = 10000` const + Load 函数末尾兜底段：`PerKeyPerMin == 0 → 60` / `BurstSize == 0 → PerKeyPerMin` / `BucketsLimit == 0 → 10000`
  - [x] 1.3 修改 `server/configs/local.yaml` 加 `ratelimit:` 段（per_key_per_min: 60 / burst_size: 60 / buckets_limit: 10000 + 注释）
  - [x] 1.4 修改 `server/internal/infra/config/loader_test.go` 新增 3 case：默认值兜底 / YAML 显式段 / YAML 部分字段；新增 fixture `testdata/ratelimit.yaml` + `testdata/ratelimit_partial.yaml`
- [x] **Task 2（AC1 / AC2 / AC7）**：实装 `internal/app/http/middleware/auth.go` + `rate_limit.go` + go.mod 加 x/time direct dep
  - [x] 2.1 `cd server && go get golang.org/x/time@latest` → v0.5.0 → v0.15.0；require 段从 indirect 升 direct（go mod tidy 后）
  - [x] 2.2 新建 `server/internal/app/http/middleware/auth.go`：定义 `UserIDKey` const + `Auth(signer *auth.Signer) gin.HandlerFunc` 工厂
  - [x] 2.3 实装 `Auth`：header 缺失 / scheme 错 / token 空 / Verify 失败均 c.Error(apperror) + c.Abort；通过则 c.Set(UserIDKey, uint64) + c.Next
  - [x] 2.4 新建 `server/internal/app/http/middleware/rate_limit.go`：定义 `KeyExtractor` + `RateLimitByIP` / `RateLimitByUserID` + `RateLimit(cfg, extractor)` 工厂
  - [x] 2.5 实装 `RateLimit`：fail-fast extractor==nil / PerKeyPerMin<=0 panic；BurstSize / BucketsLimit 默认值兜底；sync.Map + atomic.Int64 + overflow limiter；超限 c.Error(1005) + c.Abort
  - [x] 2.6 godoc 完整注释（公开 API + 设计约束 + 维度策略 + ErrorMappingMiddleware 对接）
- [x] **Task 3（AC5）**：写 `internal/app/http/middleware/auth_test.go` + `rate_limit_test.go`
  - [x] 3.1 `TestAuth_ValidToken_PassesAndInjectsUserID`
  - [x] 3.2 `TestAuth_NoAuthorizationHeader_Returns1001`
  - [x] 3.3 `TestAuth_InvalidScheme_Returns1001`（table-driven 7 sub-cases）
  - [x] 3.4 `TestAuth_ExpiredToken_Returns1001WithExpiredCause`（t.Parallel + 1.1s sleep；assert errors.Is(err, auth.ErrTokenExpired)）
  - [x] 3.5 `TestAuth_TamperedToken_Returns1001WithInvalidCause`（改 payload 段首字节按 lessons 钦定）
  - [x] 3.6 `TestRateLimit_60RequestsIn1Minute_Pass`
  - [x] 3.7 `TestRateLimit_61stRequestIn1Minute_Returns1005`
  - [x] 3.8 `TestRateLimit_CrossMinuteBoundary_ResetsTokens`（t.Parallel + 1.1s sleep）
  - [x] 3.9 `TestRateLimit_DifferentKeysIsolated`
  - [x] 3.10 `TestRateLimit_OverflowBuckets_FallsBackToSharedLimiter`
  - [x] 3.11 `go test ./internal/app/http/middleware/... -count=1 -v` 全绿
  - [x] 3.12 `go test ... -count=10 -run 'TestAuth_TamperedToken|TestRateLimit_CrossMinute|TestAuth_ExpiredToken' -v` 全绿（11.1s 总耗时；30 runs flake-free）
- [x] **Task 4（AC4）**：在 bootstrap router / server / main.go wire 新中间件
  - [x] 4.1 修改 `router.go`：引入 `Deps struct{ GormDB; TxMgr; Signer; RateLimitCfg }` + `NewRouter(deps Deps)` 签名扩展；保持 /ping / /version / /metrics 不挂 auth/rate_limit；加注释标 4.6 / 4.8 落地预期形态
  - [x] 4.2 修改 `server.go`：`Run(ctx, cfg, deps Deps) error` 收敛签名；删除直接 import gorm/auth/tx（已迁到 router.go Deps）；nil-tolerant `Run(ctx, cfg, Deps{})` 仍 OK
  - [x] 4.3 修改 `server_test.go` / `router_test.go` / `router_dev_test.go` / `router_version_test.go` / `error_mapping_integration_test.go` 适配新签名（传 `Deps{}` 零值）
  - [x] 4.4 修改 `cmd/server/main.go`：构造 `bootstrap.Deps{...}` → `bootstrap.Run(ctx, cfg, deps)`
  - [x] 4.5 `bash scripts/build.sh` 编译通过 → BUILD SUCCESS
  - [ ] 4.6 烟测 `./build/catserver -config ...` —— **跳过**（需启动本地 MySQL + export CAT_AUTH_TOKEN_SECRET；review 阶段或 fix-review sub-agent 验证；当前 dev-story 步骤通过 build + 单测 + e2e 集成测试已覆盖 wiring 正确性）
- [x] **Task 5（AC6）**：扩展集成测试 + 全量验证
  - [x] 5.1 在 `error_mapping_integration_test.go` 加 2 条 e2e case：`TestAuth_E2E_HappyAndUnauthorized` + `TestRateLimit_E2E_61stReturns1005`（用 httptest.NewServer + bootstrap.NewRouter 真 HTTP roundtrip）
  - [x] 5.2 `bash /c/fork/cat/scripts/build.sh`（vet + build）→ BUILD SUCCESS
  - [x] 5.3 `bash /c/fork/cat/scripts/build.sh --test` → all tests passed（17 内部包全绿；含 14 个新 middleware case + 3 个新 loader case）
  - [ ] 5.4 `bash /c/fork/cat/scripts/build.sh --race --test` Windows ThreadSanitizer skip —— 按 ADR-0001 §3.5 不跑（与 4.2 / 4.3 / 4.4 同模式）
  - [x] 5.5 `go mod verify` —— `go mod tidy` 已成功；x/time v0.15.0 direct dep 落地
  - [x] 5.6 `git status --short` 抽检：仅影响范围内文件（config / middleware / bootstrap / cmd/server / configs/local.yaml / go.mod / go.sum / 测试 fixtures）；docs/lessons/ 没改；docs/宠物互动App_*.md 没改
- [x] **Task 6**：本 story 不做 git commit
  - [x] 6.1 epic-loop 流水线约束遵守：dev-story 阶段不 commit
  - [x] 6.2 commit message 模板留给 story-done 阶段：

    ```text
    feat(middleware): auth + rate_limit 中间件（Story 4.5）

    - internal/app/http/middleware/auth.go：Bearer token 校验，注入 userID(uint64) 到 c.Keys
    - internal/app/http/middleware/rate_limit.go：内存 token bucket 限频（按 IP / userID 维度）
    - internal/infra/config/RateLimitConfig + ratelimit.* YAML 字段 + 默认值兜底（60/min/IP）
    - configs/local.yaml 加 ratelimit: 段
    - cmd/server/main.go + bootstrap.Run/NewRouter 签名收敛为 Deps struct（透传 GormDB/TxMgr/Signer/RateLimitCfg）
    - 单测 ≥10 case（auth 5 case + rate_limit 5 case）
    - go.mod 加 golang.org/x/time direct dep

    依据 epics.md §Story 4.5 + V1 §2.3 / §3 / §4.1 + 设计文档 §8.2 + AR9b。

    Story: 4-5-auth-rate_limit-中间件
    ```

## Dev Notes

### 关键设计原则

1. **AR9b 严格落地**：epics.md 行 170 钦定"节点 2 中间件 = auth (Bearer) + rate_limit"；本 story 是 AR9b 唯一落地路径，**不**绕过 / 拆分到其他 epic。auth + rate_limit 必须**同时**完成（router.go wire 阶段需要两者一起挂在业务 group 上）。
2. **维度策略一刀切（IP vs userID）**：epics.md 行 1039 写"每用户每分钟 60 次"，但 V1 §4.1 行 218 写"同 IP 每分钟 60 次" —— 看似矛盾，实际是不同接口的不同维度：登录前（无 userID）→ IP 维度；登录后 → userID 维度。本 story 通过 `KeyExtractor` 函数式接口让调用方决定，避免硬编码。**这是最干净的解法 —— 不引入混合维度策略（会让 4.6 / 4.8 / 后续 epic 选择困难）**。
3. **token bucket 而非 fixed-window**：epics.md 行 1049 钦定"跨分钟边界 → 重置计数"是从 fixed-window 角度描述；token bucket 实装表现为"持续平滑回填"。两者都满足"1 分钟内最多 60 次"语义；token bucket 对突发更友好（一次性发 60 个 burst-friendly），fixed-window 容易触发"分钟边界 60+60=120 次"边缘 bug。本 story 选 token bucket（stdlib 成熟实装 + 测试覆盖完善 + 生产标配）。
4. **userID = uint64 而非 string**：4.4 token util 已钦定 `Claims.UserID uint64`；V1 §2.5 钦定 JSON 端 string + Go 内部 uint64 划分；本 story 严格对齐 —— `c.Set(UserIDKey, uint64(...))`。**反模式**：早期常见"中间件存 string，handler 反复 ParseUint" → 浪费 CPU + 类型不安全（万一 "12345abc" 怎么办）。
5. **bootstrap.Run 签名收敛为 Deps struct**：4.4 已经把 Run 从 4 参数扩到 5 参数（加 signer）；本 story 第二次扩（加 RateLimitCfg）时，**必须**收敛 Deps struct，避免后续 4.6 / 4.8 / Epic 5+ 每加一个依赖就改 Run 签名。这是接口设计的"早期投入"（一次性对齐）vs"后期 churn"（每 story 改一次）的取舍。
6. **fail-fast over fallback**：rate_limit 配置无效（PerKeyPerMin ≤ 0 / extractor 为 nil）→ middleware 工厂 panic；启动期就暴露问题，避免业务请求才发现限频策略无效。**MEMORY.md "No Backup Fallback" 钦定反对 fallback 掩盖核心风险**；本 story 严格遵守。
7. **不依赖 ctx**：HTTP middleware 是 request-scoped 同步 path，rate_limit Allow() / auth Verify 都是 < 1µs CPU 计算（与 4.4 同思路）；不需要 select ctx.Done()。

### 架构对齐

**领域模型层**（`docs/宠物互动App_总体架构设计.md` §3）：
- 节点 2 阶段 auth 中间件是入口闸门（"用户如何进入受保护资源"），rate_limit 是滥用保护层；两者拼成 V1 §2.3 / §3 钦定的 1001 / 1005 错误码语义

**接口契约层**（`docs/宠物互动App_V1接口设计.md`）：
- §2.3 行 41-44 钦定 `Authorization: Bearer <token>` 头（auth 中间件唯一 token 输入）
- §2.4 钦定业务码与 HTTP status 正交（1001 / 1005 都走 200 + envelope.code）
- §3 行 80 / 84 钦定 1001 / 1005 错误码语义
- §4.1 行 139-140 / 218 钦定 `/auth/guest-login` 不挂 auth 但挂 rate_limit（IP 维度，60/min）
- §4.3 行 299 钦定 `/me` 走 auth；§5.1 钦定 `/home` 走 auth + rate_limit

**Go 项目结构层**（`docs/宠物互动App_Go项目结构与模块职责设计.md`）：
- §4 行 133 钦定 `internal/app/http/middleware/auth.go`（与 request_id / recover / logging 平级）
- §8.2 行 717-725 钦定中间件清单含 auth / rate_limit
- §13 行 931-933 钦定 `auth:` YAML 段（4.4 已落地）；本 story **新增** `ratelimit:` 段

**ADR 对接**：
- ADR-0006 错误三层映射：本 story Auth 中间件用 `c.Error(apperror.Wrap(err, ErrUnauthorized, msg))` + `c.Abort` 让 ErrorMappingMiddleware 写 envelope；RateLimit 中间件用 `c.Error(apperror.New(ErrTooManyRequests, msg))` + `c.Abort`。**严禁**直接调 response.Error 绕过 ErrorMappingMiddleware（lessons/2026-04-24-error-envelope-single-producer.md 钦定）
- ADR-0007 ctx 传播：本 story 中间件**不**消费 ctx（同步 < 1µs CPU 计算）；c.Set(UserIDKey, ...) 是 gin.Context Keys 而非 ctx.Value，下游 handler / service 通过显式参数（`func GuestLogin(ctx, userID, ...)`）取，不通过 ctx.Value 隐式传

### 测试策略

按 ADR-0001 §3.1 + 4.2 / 4.3 / 4.4 已建立的测试范式：

- **单测层**（`auth_test.go` + `rate_limit_test.go` + `loader_test.go` 增量）：纯 Go test + httptest，不起容器 / 不连 MySQL / 不依赖 Redis
- **集成测试层**：本 story 仅写**轻量端到端**测试（用 httptest.NewServer + bootstrap.NewRouter 真路由链），覆盖"中间件已 wire 到生产 router"信号；**不**写 dockertest 类重测试 —— 4.7 Layer 2 集成测试会用真 token + 真 service 端到端覆盖 auth 行为

**关键决策**：本 story **不**用 mock clock 或 fake time。原因：(1) `golang.org/x/time/rate` 没有暴露 clock 抽象；(2) 引入 wrapper 让 limiter 可注入 fake clock 增加复杂度；(3) 真 sleep 在 1.1s 量级单 case 内可控；(4) 与 4.4 测试同模式（不 mock time）。

### 启动顺序约束

按 4.2 / 4.3 / 4.4 已建立的启动顺序，本 story 在 main.go 不新增独立校验步骤（rate_limit 配置校验由 middleware.RateLimit 工厂在 wire 阶段完成 panic）：

```
main()
├─ logger.Init("info")                           # bootstrap logger
├─ parseTopLevelArgs                              # 4.3 加：拆 flag / migrate 子命令
├─ if isMigrate: cli.RunMigrate; os.Exit          # 4.3 加：migrate 分支
├─ config.LocateDefault / Load                    # 已有
├─ logger.Init(cfg.Log.Level)                     # 已有
├─ db.Open(dbOpenCtx, cfg.MySQL)                  # 4.2 加
├─ defer sqlDB.Close                               # 4.2 加
├─ tx.NewManager(gormDB)                           # 4.2 加
├─ auth.New(cfg.Auth.TokenSecret, ...)            # 4.4 加：fail-fast
│
├─ ★ 本 story 修改：bootstrap.Run(ctx, cfg, Deps{...}) 签名收敛
│   └─ Deps 含 GormDB / TxMgr / Signer / RateLimitCfg
│   └─ NewRouter(deps) 内部组装 router；本 story 暂不挂 /api/v1 业务 group
│   └─ middleware.RateLimit(cfg.RateLimit, ...) 在工厂期校验配置：PerKeyPerMin ≤ 0 → panic
│
└─ bootstrap.Run(ctx, cfg, deps)
```

**关键**：rate_limit 配置无效不在 main.go 显式 fail-fast，而是 router.go 调 `middleware.RateLimit(cfg, ...)` 工厂时 panic（启动期早暴露）。**反模式**：让中间件每请求 lazy-validate cfg → 业务请求才发现配置错误，违反 fail-fast。

### 与已 done 的 4.2 / 4.3 / 4.4 的衔接

**4.2 实装**：
- `internal/infra/db/mysql.go`：`Open(ctx, cfg) (*gorm.DB, error)` + fail-fast
- `internal/repo/tx/manager.go`：`Manager` interface + `WithTx(ctx, fn)`
- `internal/infra/config.MySQLConfig` + `CAT_MYSQL_DSN` env override

**4.3 实装**：
- `server/migrations/`：5 张表 up/down SQL
- `internal/infra/migrate/migrate.go`：Migrator interface + golang-migrate 包装
- `internal/cli/migrate.go`：catserver migrate 子命令分发

**4.4 实装**：
- `internal/pkg/auth/token.go`：`Signer` 单例 + `Sign(uid, expire) (string, error)` + `Verify(tok) (Claims, error)` + 哨兵 error
- `internal/infra/config.AuthConfig` + `CAT_AUTH_TOKEN_SECRET` env override
- `cmd/server/main.go` 启动序列加 `auth.New` fail-fast

**本 story 复用**：
- `cfg`（loader.go 路径）：本 story 在同一个 Config struct 加 RateLimitConfig + 同一个 Load 函数加默认值兜底（与 MySQL / Auth 同模式）
- `*auth.Signer`：本 story Auth 中间件接受 `*auth.Signer` 注入；不创建新 signer / 不修改 token util
- `apperror`（`internal/pkg/errors`）：本 story 中间件用 `apperror.New(code, msg)` + `apperror.Wrap(err, code, msg)` + `apperror.As(err)`，与 ADR-0006 三层映射一致
- `ResponseErrorCodeKey` / `RequestIDKey`（`middleware.error_mapping` / `middleware.request_id`）：本 story 不消费这些 key（auth/rate_limit 走 c.Error → ErrorMappingMiddleware 决策 envelope.code）；新增的 `UserIDKey` 与它们风格统一（小写下划线 key 命名）
- `bash scripts/build.sh`：本 story 不改脚本，直接复用 vet/test/race 路径

**本 story 新增解耦的 path**：
- `internal/app/http/middleware/auth.go` + `rate_limit.go`：与现有 5 个中间件文件平级
- `internal/app/bootstrap.Deps struct`：未来 4.6 / 4.8 / Epic 5+ 加共享依赖时只改 Deps，不改 Run 签名

### 与下游 4.6 / 4.8 的接口

**4.6 落地时会做**：
1. `internal/app/http/handler/auth_handler.go`：实装 `GuestLoginHandler`；register 时挂在 `/auth` 子组（已有 rate_limit by IP，**不**挂 auth）
2. handler 完成事务后调 `signer.Sign(user.ID, cfg.Auth.TokenExpireSec)` → 写 V1 §4.1 response.data.token
3. 不依赖本 story 的 UserIDKey（登录前路径无 userID）

**4.8 落地时会做**：
1. `internal/app/http/handler/home_handler.go`：实装 `HomeHandler`；register 时挂在已认证 group（已有 auth + rate_limit by userID）
2. handler 内 `uid := c.MustGet(middleware.UserIDKey).(uint64)` → 调 `home_service.LoadHome(ctx, uid)`
3. service / repo 层用 ctx + uid 显式参数（ADR-0007 §2.1）

**本 story 必须保证 4.6 / 4.8 能直接用**：
- `Auth(signer)` 工厂返回 `gin.HandlerFunc`：4.6 的 NewRouter 直接 `api.Group("/some-path", middleware.Auth(deps.Signer))` 即可
- `RateLimit(cfg, extractor)` 工厂返回 `gin.HandlerFunc`：4.6 的 `/auth` 子组用 `middleware.RateLimitByIP`，4.8 的已认证 group 用 `middleware.RateLimitByUserID`
- `UserIDKey = "userID"`：4.8 / 后续 handler `c.Get(middleware.UserIDKey)` 拿 uint64
- `Deps struct`：4.6 / 4.8 在 NewRouter 内部从 `deps.GormDB` / `deps.TxMgr` 拿 service 依赖（service 工厂在那时落地）

### 关键决策点（实装时注意）

1. **`golang.org/x/time/rate` API 关键点**：
   - `rate.NewLimiter(r rate.Limit, b int) *Limiter` —— 第一参数是每秒速率（`rate.Limit` = `float64` 别名），第二参数是 burst size
   - `Allow() bool` —— 非阻塞判定是否有 token；返 true 消费 1 个 token
   - **本 story 用 Allow()，不用 Wait() / Reserve()**：HTTP middleware 必须非阻塞（不能让请求等 token 回填）
   - rate.Limit 是每**秒**速率：`rate.Limit(60.0/60.0) = 1.0/秒`；这与 epics.md "每分钟 60 次"语义一致

2. **`sync.Map` vs `map + RWMutex` 选型**：sync.Map 在"读多写少 + key set 增长缓慢"场景性能更优；rate_limit map 完全符合（同 key 反复 read，新 key 偶尔 store）。**唯一坑**：sync.Map 没暴露 size → 用 atomic.Int64 计数器并发维护。

3. **`atomic.Int64` 计数 vs sync.Map 内置 size**：sync.Map 内部确实有 count 但不暴露；用 atomic.Int64 + LoadOrStore 的 `loaded` 返回值对偶维护：

   ```go
   actual, loaded := buckets.LoadOrStore(key, newLim)
   if !loaded { count.Add(1) }  // 只在真新增时加
   ```

4. **`overflow` 共享 limiter 的 race**：所有 overflow key 共享同一个 limiter → 高并发下 Allow() 的 token state 会被多 goroutine 并发争用；`golang.org/x/time/rate.Limiter` 内部已用 mutex（thread-safe），无需额外锁。

5. **`KeyExtractor` 工厂模式 vs hardcoded 维度**：用工厂模式让调用方决策，而不在中间件内 hardcoded if-else（"if userID exists then by userID else by IP"）—— 后者在测试 / 配置 / 维护上都更糟（routes 看不出维度策略）。

6. **`c.ClientIP()` 是 gin 已封装好的**：自动处理 X-Forwarded-For / X-Real-IP / RemoteAddr 优先级；本 story 直接用，**不**自己 parse header。生产部署 reverse proxy（nginx / k8s ingress）时务必配 trusted proxies（gin.Engine.SetTrustedProxies）—— **本 story 不改 trusted proxies 配置**（节点 2 阶段单实例无 reverse proxy；节点 36 部署 epic 加）。

7. **`UserIDKey = "userID"`** vs `"user_id"`：选 camelCase 与 V1 §2.5 字段命名 + Go 字段命名惯例一致；不与 RequestIDKey (`request_id`) 用同样下划线，因为 RequestIDKey 是 V1 §2.4 envelope JSON 字段（要写进响应），UserIDKey 是 server 内部 ctx key（不出 JSON）。

8. **`Deps struct` 未来扩展**：

   ```go
   // 本 story 落地：
   type Deps struct {
       GormDB       *gorm.DB
       TxMgr        tx.Manager
       Signer       *auth.Signer
       RateLimitCfg config.RateLimitConfig
   }

   // 4.6 落地时加：
   //     UserRepo         repo_mysql.UserRepo
   //     AuthBindingRepo  repo_mysql.AuthBindingRepo
   //     PetRepo          repo_mysql.PetRepo
   //     ...
   //     AuthService      *service.AuthService
   //
   // 4.8 落地时加：
   //     HomeService      *service.HomeService
   //
   // Epic 10 落地时加：
   //     RedisClient      *redis.Client  // 切换 RateLimit backend
   //     ...
   ```

9. **go.mod x/time 提升 indirect → direct**：

   ```bash
   cd server
   go get golang.org/x/time@latest
   go mod tidy
   ```

   预期 diff：require 段 `golang.org/x/time vX.Y.Z` 从 indirect 移到 direct。**确认**：`go.mod` 不引入新 indirect 依赖（x/time 自身仅依赖 stdlib）。

10. **Windows 平台 race 测试**：与 4.2 / 4.3 / 4.4 一致 —— Windows ThreadSanitizer "failed to allocate" 是已知平台限制，按 ADR-0001 §3.5 skip；Linux / CI race 路径不受影响。

### Project Structure Notes

预期文件 / 目录变化：

- 新增：`server/internal/app/http/middleware/auth.go` + `auth_test.go`
- 新增：`server/internal/app/http/middleware/rate_limit.go` + `rate_limit_test.go`
- 修改：`server/internal/infra/config/config.go`（加 RateLimitConfig + Config.RateLimit 字段）
- 修改：`server/internal/infra/config/loader.go`（加 defaultRateLimitPerKeyPerMin / defaultRateLimitBucketsLimit const + 默认值兜底）
- 修改：`server/internal/infra/config/loader_test.go`（加 ≥3 case 覆盖 ratelimit YAML 段 / 部分字段 / 全部默认）
- 修改：`server/configs/local.yaml`（加 `ratelimit:` 段）
- 修改：`server/internal/app/bootstrap/router.go`（NewRouter 签名加 Deps 参数；保持 /ping / /version / /metrics 不挂 auth/rate_limit）
- 修改：`server/internal/app/bootstrap/server.go`（Run 签名收敛为 `Run(ctx, cfg, deps Deps) error`）
- 修改：`server/internal/app/bootstrap/server_test.go` / `router_test.go` / `router_dev_test.go` / `router_version_test.go`（适配新签名）
- 修改：`server/internal/app/http/middleware/ctx_propagation_integration_test.go` / `error_mapping_integration_test.go`（如调用 NewRouter / Run 需要适配新签名）
- 修改：`server/cmd/server/main.go`（构造 Deps + 传给 bootstrap.Run）
- 修改：`server/go.mod` + `server/go.sum`（提升 `golang.org/x/time` 为 direct require）
- 修改：`_bmad-output/implementation-artifacts/sprint-status.yaml`（4-5-auth-rate_limit-中间件: backlog → ready-for-dev → in-progress → review；由 dev-story 流程内推动）
- 修改：`_bmad-output/implementation-artifacts/4-5-auth-rate_limit-中间件.md`（本 story 文件，dev 完成后填 Tasks/Dev Agent Record/File List/Completion Notes）

可能需要新增（如端到端验证另起文件）：
- `server/internal/app/http/middleware/auth_integration_test.go`（如选择独立文件而非扩 error_mapping_integration_test.go）

不影响其他目录：

- `server/internal/pkg/auth/` 不变（4.4 已落地；本 story 仅消费）
- `server/internal/infra/db/` 不变（4.2 已落地）
- `server/internal/infra/migrate/` 不变（4.3 已落地）
- `server/internal/cli/` 不变（4.3 已落地）
- `server/internal/repo/tx/` 不变（4.2 已落地）
- `server/internal/repo/mysql/` 不存在（4.6 才落地第一个 user_repo.go）
- `server/internal/service/` 不变（除现有 sample/ 外不动；4.6 才落地 auth_service.go）
- `server/internal/app/http/handler/` 不变（除现有 ping_handler / version_handler 外不动；4.6 才落地 auth_handler.go）
- `server/internal/app/http/middleware/error_mapping.go` / `logging.go` / `recover.go` / `request_id.go` 不变（已有；本 story 仅依赖它们）
- `server/migrations/` 不变（4.3 已落地）
- `iphone/` / `ios/` 不变（server-only story）
- `docs/宠物互动App_*.md` 全部 7 份不变（消费方）
- `docs/lessons/` 不变（review 阶段写新 lesson 由 fix-review 处理）
- `README.md` / `server/README.md` 不变（Epic 4 收尾或 Epic 36 部署 story 才统一更新）

### golang.org/x/time/rate 关键 API 提示

实装时注意（避免常见坑）：

- **包 import**：`"golang.org/x/time/rate"`（subrepo，非 stdlib）
- **构造函数**：`rate.NewLimiter(r rate.Limit, b int) *Limiter` —— `r` 是每秒速率（`rate.Limit` 是 `float64` 别名 + 特殊值 `rate.Inf`），`b` 是 burst size（int）
- **类型转换**：`rate.Limit(60.0 / 60.0)` = 1.0/秒；本 story `rate.Limit(float64(cfg.PerKeyPerMin) / 60.0)`
- **核心方法**：
  - `Allow() bool` —— 非阻塞判定是否消费 1 个 token；返 false 时**不**消费
  - `Wait(ctx) error` —— 阻塞等到有 token；本 story **不用**（HTTP middleware 不阻塞）
  - `Reserve() *Reservation` —— 立即获取一个 token 但记录预留时长；本 story **不用**
- **线程安全**：`*Limiter` 内部已 mutex，多 goroutine 并发 Allow() 安全
- **零值不可用**：必须用 `rate.NewLimiter(...)` 构造；`var l rate.Limiter` 然后 l.Allow() 会 panic

### epics.md "跨分钟边界 → 重置计数" 与 token bucket 实装的语义兼容性

epics.md 行 1049 钦定 "rate_limit edge: 跨分钟边界 → 重置计数"；token bucket 实装表现为"持续平滑回填"（每秒回填 1 个 token），**不是** fixed-window 风格的"分钟边界 hard reset"。

**为什么 token bucket 满足"重置计数"语义**：

- "1 分钟内 60 次" 在 token bucket 视角下 = "平均每秒 1 次 + 一次 burst 60"
- "跨分钟边界 → 重置计数" 的意图是 "时间过去后该 key 应该重新可用"
- token bucket 实装天然满足：60 次耗光 → 等 1 秒 → 回填 1 个 token → 再次可用；等 60 秒 → 满桶 60 个 token

**测试验证**（AC5.8）：耗光 60 次 → time.Sleep(1.1s) → 第 61 次必通过。这是 token bucket "回填" 语义的最小验证；如果想验证"满桶等价于全部新计数"，需要 sleep 60s（不实用）。

**Future Redis 切换**：Epic 10 切 Redis-based 时如选 "fixed window counter" 实装（Redis SETEX + INCR + TTL 60s），语义会切换为 hard reset；切换时再视情况调整测试。本 story 测试**不**钉具体算法（如 "测试通过 ⇔ 必须是 token bucket"），仅钉**外部观察**（"耗光后 1.1s 后能再发"）。

### References

- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 4.5 (行 1022-1049)] — 本 story 钦定 AC 来源（auth 中间件 Bearer token 校验 + 注入 userID / rate_limit 内存 token bucket / 默认 60 per min / 配置 ratelimit.per_user_per_min / 超限 1005 / ≥6 单测）
- [Source: `_bmad-output/planning-artifacts/epics.md` §Epic 4 Overview (行 927-931)] — 节点 2 第一个业务 epic / 执行顺序 4.1 → 4.2 → 4.3 → 4.4 → **4.5** → 4.6 → 4.8 → 4.7
- [Source: `_bmad-output/planning-artifacts/epics.md` §AR9b (行 170)] — 节点 2 中间件 = auth + rate_limit，落 E4
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 4.4 (行 999-1020)] — 上游 token util；本 story 消费 `*auth.Signer.Verify`
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 4.6 (行 1051-1082)] — 下游游客登录；本 story 产出的 RateLimit 是 `/auth/guest-login` 唯一限频路径
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 4.8 (行 1106-1137)] — 下游 GET /home；本 story 产出的 Auth 是 `/home` 唯一鉴权路径
- [Source: `_bmad-output/planning-artifacts/epics.md` §NFR12 (行 116)] — 限频最终走 Redis；节点 2 内存 token bucket 是过渡（tech debt 已登记）
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 6.3 (行 1308-1322)] — 节点 2 收尾文档同步 + tech debt log（含 "rate_limit 用内存 token bucket，多实例部署时各自计数"）
- [Source: `docs/宠物互动App_V1接口设计.md` §2.3 (行 41-44)] — Authorization: Bearer 头格式
- [Source: `docs/宠物互动App_V1接口设计.md` §2.4 (行 47-63)] — envelope 结构 + 业务码与 HTTP status 正交
- [Source: `docs/宠物互动App_V1接口设计.md` §3 (行 76-118)] — 错误码 1001 / 1005 定义
- [Source: `docs/宠物互动App_V1接口设计.md` §4.1 (行 124-220)] — guest-login 钦定不挂 auth 但挂 rate_limit + IP 维度 + 60/min 默认值
- [Source: `docs/宠物互动App_V1接口设计.md` §4.3 (行 250-300)] — /me 走 auth；errors 1001
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 (行 122-140)] — `internal/app/http/middleware/auth.go` 目录树锚定
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §6.1 (行 338-360)] — Auth 模块职责
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §8.2 (行 717-725)] — 中间件清单：request_id / recover / logging / auth / rate_limit
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §13 (行 920-937)] — YAML 配置块：auth + ws；本 story 新增 ratelimit
- [Source: `_bmad-output/implementation-artifacts/decisions/0006-error-handling.md` §2 / §3] — 三层错误映射：repo 原生 → service apperror.Wrap → handler c.Error → middleware envelope；本 story 是 middleware 层（auth/rate_limit 用 c.Error 推 AppError，让 ErrorMappingMiddleware 写 envelope）
- [Source: `_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md` §2.2] — handler 从 c.Request.Context() 取 ctx 向下传；本 story Auth 中间件**不**做 ctx 包装（仅 c.Set(UserIDKey, uid)），让下游 handler / service 用显式参数传 userID
- [Source: `_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md` §6 (反对 ctx.Value 传业务字段)] — 本 story 严格遵守：用 c.Keys 而非 ctx.Value 传 userID
- [Source: `_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.1 / §3.5] — 单测 + 集成测试双层；Windows race skip；本 story 仅单测层（中间件单元 + httptest 端到端，无 dockertest）
- [Source: `_bmad-output/implementation-artifacts/4-2-mysql-接入.md`] — 上游 story；本 story 复用 cfg.Load / loader 默认值兜底模式
- [Source: `_bmad-output/implementation-artifacts/4-3-五张表-migrations.md`] — 上游 story；本 story 复用启动序列模式（fail-fast 在 wire 期）
- [Source: `_bmad-output/implementation-artifacts/4-4-token-util.md`] — 上游 story；本 story 直接消费 `*auth.Signer` 单例 + Claims.UserID
- [Source: `docs/lessons/2026-04-24-error-envelope-single-producer.md`] — ErrorMappingMiddleware 是唯一 envelope 生产者；本 story 中间件**禁止**直接调 response.Error
- [Source: `docs/lessons/2026-04-24-middleware-canonical-decision-key.md`] — c.Keys 显式 canonical key 模式；本 story UserIDKey 与 RequestIDKey / ResponseErrorCodeKey 同一系列
- [Source: `docs/lessons/2026-04-26-jwt-required-claim-and-sign-policy-enforcement.md`] — JWT 必填 claim + sign 策略 enforce；本 story 消费 4.4 token util 的 Verify，不重复 enforce（信任 Verify 的 Claims 必有 UserID 非 nil）
- [Source: `docs/lessons/2026-04-26-jwt-tamper-test-must-mutate-non-terminal-byte.md`] — JWT 篡改测试必须改非末尾字节；本 story TestAuth_TamperedToken 严格遵守
- [Source: `docs/lessons/2026-04-26-config-env-override-and-gorm-auto-ping.md`] — 4.2 review lesson：infrastructure 接入必须配齐 env override；本 story RateLimitConfig 是策略参数（非 secret），**不**加 env override（与 mysql.dsn / auth.token_secret 不同）
- [Source: `docs/lessons/2026-04-26-checked-in-config-must-boot-default.md`] — 非 secret 字段必须 fresh clone 直接跑；本 story RateLimitConfig 全字段 checked-in 默认值（60/60/10000）
- [Source: `docs/lessons/2026-04-26-checked-in-secret-must-fail-fast.md`] — secret 字段必须空 + fail-fast；本 story RateLimitConfig **无** secret 字段，本 lesson 不直接适用（仅作为"判定 secret-like" 的参考）
- [Source: `docs/lessons/2026-04-25-slog-init-before-startup-errors.md`] — 早期启动错误必须走结构化日志；本 story 中间件运行期日志由 ErrorMappingMiddleware / Logging 中间件统一打，不直接写 slog
- [Source: `CLAUDE.md` §"工作纪律"] — "节点顺序不可乱跳 / 状态以 server 为准 / ctx 必传"；本 story 是节点 2 第五条 server story 严格按 4.5 顺序推
- [Source: `CLAUDE.md` §"Build & Test"] — 写完 / 改完 Go 代码后跑 `bash scripts/build.sh --test` 验证
- [Source: `MEMORY.md` "No Backup Fallback"] — 反对 fallback 掩盖核心风险；本 story RateLimit 配置无效（PerKeyPerMin ≤ 0）→ 工厂期 panic，不 silent default
- [Source: `MEMORY.md` "Repo Separation"] — server 测试自包含，不调 APP / watch；本 story 仅 server 单测 + httptest，不依赖任何端联调
- [Source: `docs/宠物互动App_数据库设计.md` §3.1 (行 73-167)] — 主键 BIGINT UNSIGNED；本 story UserIDKey 注入 uint64 与该类型对齐

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m] (Anthropic Opus 4.7, 1M context)

### Debug Log References

- 单测 RED 阶段：`TestAuth_TamperedToken_Returns1001WithInvalidCause` + `TestAuth_ExpiredToken_Returns1001WithExpiredCause` 初次跑红——`captured.firstErr is nil`。
  - **原因**：测试 router 中间件链 `ErrorMappingMiddleware → Auth → captureCallback → handler`；Auth 错误时 c.Abort() 阻止 captureCallback 运行（c.Abort 阻止"下游"中间件继续，不影响已经在栈上的"after-Next"代码段）；captureCallback 永远不会读到 c.Errors。
  - **修复**：把 captureCallback 移到 Auth **之前**：`ErrorMappingMiddleware → captureCallback → Auth → handler`。captureCallback 的 after-Next 代码段（在 c.Next() 之后）会在 c.Abort 后仍然执行（栈展开顺序）。Re-run 后 5 个 auth case + 5 个 rate_limit case + 4 个辅助 case 全绿。
- 单测 flake-free 验证：`go test ./internal/app/http/middleware/... -count=10 -run 'TestAuth_TamperedToken|TestRateLimit_CrossMinute|TestAuth_ExpiredToken' -v` 跑 30 runs（11.1s 总）全 PASS。

### Completion Notes List

- ✅ AC1 — `internal/app/http/middleware/auth.go`：定义 `UserIDKey="userID"` const；`Auth(signer)` gin.HandlerFunc 工厂解析 `Authorization: Bearer` header（RFC 6750 大小写不敏感）→ 调 `signer.Verify` → 成功 c.Set(UserIDKey, claims.UserID:uint64) + c.Next；失败 c.Error(apperror.Wrap(err, ErrUnauthorized, msg)) + c.Abort（保留 cause 链让 errors.Is(err, auth.ErrTokenExpired/Invalid) 穿透）。
- ✅ AC2 — `internal/app/http/middleware/rate_limit.go`：`KeyExtractor` 函数式接口 + `RateLimitByIP` / `RateLimitByUserID` 两个内置 extractor；`RateLimit(cfg, extractor)` 工厂内部用 `sync.Map` + `atomic.Int64` + `golang.org/x/time/rate.Limiter`；启动期 fail-fast（extractor==nil / PerKeyPerMin<=0 → panic）；BurstSize / BucketsLimit 默认值兜底；超限 c.Error(apperror.New(ErrTooManyRequests, msg)) + c.Abort；BucketsLimit 上限触发后新 key 走共享 overflow limiter 防 IP 洪泛 OOM。
- ✅ AC3 — `internal/infra/config/`：新增 `RateLimitConfig{PerKeyPerMin, BurstSize, BucketsLimit}` + `Config.RateLimit` 字段（yaml: `ratelimit`）；`loader.go` 加 `defaultRateLimitPerKeyPerMin=60` / `defaultRateLimitBucketsLimit=10000` const + 默认值兜底（PerKeyPerMin==0→60 / BurstSize==0→PerKeyPerMin / BucketsLimit==0→10000）；`configs/local.yaml` 加 `ratelimit:` 段（60 / 60 / 10000 + 注释）；fixture `testdata/ratelimit.yaml` + `testdata/ratelimit_partial.yaml` 验证 YAML 解析 + 部分字段兜底。
- ✅ AC4 — `internal/app/bootstrap/`：引入 `Deps struct{GormDB, TxMgr, Signer, RateLimitCfg}` 收敛 Run / NewRouter 签名（4.4 5 参数 → 4.5 收敛 struct，避免后续每加一个依赖就改签名）；`Run(ctx, cfg, deps Deps) error` + `NewRouter(deps Deps) *gin.Engine`；router 仍**只**注册 /ping / /version / /metrics（不挂 auth/rate_limit；防 health check 自爆）；4.6 / 4.8 落地后填业务 group 的预期形态写在 godoc 注释里。`cmd/server/main.go` 构造 `bootstrap.Deps{...}` 透传；server.go 删除 gorm/auth/tx 直接 import（已迁到 router.go）。
- ✅ AC5 — 单元测试 14 case：
  - `auth_test.go`：5 case（TestAuth_ValidToken_PassesAndInjectsUserID / NoAuthorizationHeader_Returns1001 / InvalidScheme_Returns1001 含 7 sub-cases / ExpiredToken_Returns1001WithExpiredCause / TamperedToken_Returns1001WithInvalidCause）
  - `rate_limit_test.go`：9 case（60RequestsIn1Minute_Pass / 61stRequestIn1Minute_Returns1005 / CrossMinuteBoundary_ResetsTokens / DifferentKeysIsolated / OverflowBuckets_FallsBackToSharedLimiter / PanicsOnNilExtractor / PanicsOnInvalidPerKeyPerMin / RateLimitByUserID_PrefersUserID / RateLimitByUserID_FallbackToIP）
  - 总 14 case > AC5 钦定 ≥6 case；nondeterministic case 用 t.Parallel() + count=10 验证 flake-free。
- ✅ AC6 — 集成测试 + 全量验证：扩展 `error_mapping_integration_test.go` 加 2 条 e2e case（`TestAuth_E2E_HappyAndUnauthorized` + `TestRateLimit_E2E_61stReturns1005`），用 `httptest.NewServer + bootstrap.NewRouter` 真 HTTP roundtrip 验证 Auth + RateLimit 与 ErrorMappingMiddleware 联动。`bash scripts/build.sh --test` BUILD SUCCESS + all tests passed（17 内部包）。
- ✅ AC7 — `go.mod` / `go.sum`：`go get golang.org/x/time@latest` 升级 v0.5.0 → v0.15.0；`go mod tidy` 后 `golang.org/x/time v0.15.0` 出现在 require 段（`grep golang.org/x/time go.mod` 确认；非 indirect 标记）。`go mod verify` 流程被 build script 包含，跑通。
- ✅ AC8 — README / docs 不更新（消费方角色）。
- 🔧 **关键决策**：
  - **bootstrap.Run 签名收敛**：本 story 第二次扩 Run 签名（4.4 加 signer 是第一次）；按 story 文件钦定收敛为 `Deps struct`，避免后续 4.6 / 4.8 / Epic 5+ 每加一个依赖就改 Run / NewRouter / 全部测试 5+ 次。Deps 字段 nil-tolerant，`Run(ctx, cfg, Deps{})` 仍 OK（server_test 等保持 zero-deps 测试路径）。
  - **维度策略：KeyExtractor 函数式接口**：epics.md 行 1039 写"每用户每分钟 60 次"，V1 §4.1 行 218 写"同 IP 每分钟 60 次"——两者不矛盾（前者登录后 / 后者登录前），通过 KeyExtractor 让调用方决定（4.6 /auth/guest-login 用 RateLimitByIP / 4.8 /home 用 RateLimitByUserID），避免硬编码"if userID then by userID else by IP"反模式。
  - **token bucket 而非 fixed-window**：用 `golang.org/x/time/rate` stdlib 成熟实装；epics.md 行 1049 钦定的"跨分钟边界 → 重置计数"在 token bucket 实装下表现为"持续平滑回填"（每秒回填 1 个 token），外部观察等价（"耗光后等待时间过去再次可用"）；测试用 1.1s sleep 跨秒边界验证 token 回填即可。
  - **测试架构修复（Debug Log 段已记录）**：c.Abort 阻止下游中间件，不影响已在栈上的 after-Next 段——captureCallback 必须放在 Auth **之前**。
- 📋 **tech debt 登记**（Completion Notes 钉死，待 Story 6.3 文档同步阶段写入 `tech-debt-log.md`）：
  1. **内存 token bucket 多实例不共享计数**：节点 2 单实例 OK；节点 10+ Story 10.6 切 Redis-based rate limiter 后多实例共享。
  2. **限频 prometheus 专属 metric 缺失**：当前 Logging 中间件通过 `error_code=1005` 给出可聚合信号 OK；future epic 加 `rate_limit_rejected_total{path}` 专属指标。
  3. **README rate_limit 配置说明**：Epic 4 收尾或 Epic 36 部署 story 统一写。
  4. **运维端点 /metrics 鉴权**：Epic 36 上线前需要加 auth / 独立端口（router.go TODO 已标记）。
- 🚫 **范围红线遵守**：未实装 /auth/guest-login handler（4.6 做）；未接 Redis（节点 10+ 做）；未改 docs/宠物互动App_*.md / docs/lessons/；未给 /ping / /version / /metrics 挂 auth/rate_limit。
- 🚫 **Task 4.6 烟测跳过**：需要本地 MySQL + CAT_AUTH_TOKEN_SECRET env，dev-story 阶段不执行；wiring 正确性已通过 `bash scripts/build.sh --test` 全量绿（含 e2e httptest 集成测试）覆盖；fix-review / story-done sub-agent 阶段如需可补真机烟测。
- 🚫 **Task 5.4 race 测试 Windows skip**：按 ADR-0001 §3.5 + 4.2/4.3/4.4 lesson 同模式，Windows ThreadSanitizer 已知问题；Linux/CI 路径必过。

### File List

**新增**：

- `server/internal/app/http/middleware/auth.go`
- `server/internal/app/http/middleware/auth_test.go`
- `server/internal/app/http/middleware/rate_limit.go`
- `server/internal/app/http/middleware/rate_limit_test.go`
- `server/internal/infra/config/testdata/ratelimit.yaml`
- `server/internal/infra/config/testdata/ratelimit_partial.yaml`

**修改**：

- `server/internal/infra/config/config.go`（加 `RateLimitConfig` struct + `Config.RateLimit` 字段）
- `server/internal/infra/config/loader.go`（加 `defaultRateLimitPerKeyPerMin` / `defaultRateLimitBucketsLimit` const + 默认值兜底段）
- `server/internal/infra/config/loader_test.go`（加 3 case：Defaults / YAMLParsing / PartialFields）
- `server/configs/local.yaml`（加 `ratelimit:` 段）
- `server/internal/app/bootstrap/router.go`（引入 `Deps` struct + `NewRouter(deps Deps)` 签名扩展）
- `server/internal/app/bootstrap/server.go`（`Run(ctx, cfg, deps Deps) error` 收敛签名；移除直接 gorm/auth/tx import）
- `server/internal/app/bootstrap/server_test.go`（Run 调用适配 `Deps{}`）
- `server/internal/app/bootstrap/router_test.go`（NewRouter 调用适配 `Deps{}`）
- `server/internal/app/bootstrap/router_dev_test.go`（NewRouter 调用适配 `Deps{}`）
- `server/internal/app/bootstrap/router_version_test.go`（NewRouter 调用适配 `Deps{}`）
- `server/internal/app/http/middleware/error_mapping_integration_test.go`（NewRouter 调用适配 + 新增 `TestAuth_E2E_HappyAndUnauthorized` / `TestRateLimit_E2E_61stReturns1005` 两条 e2e case）
- `server/cmd/server/main.go`（构造 `bootstrap.Deps{...}` 透传）
- `server/go.mod`（`golang.org/x/time` v0.5.0 → v0.15.0；indirect → direct）
- `server/go.sum`（同步 x/time 升级）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（4-5: ready-for-dev → in-progress → review）
- `_bmad-output/implementation-artifacts/4-5-auth-rate_limit-中间件.md`（本 story 文件，dev 阶段填 Tasks/Dev Agent Record）

### Change Log

- 2026-04-26: Story 4.5 实装完成。新增 auth + rate_limit 两个 HTTP 中间件 + RateLimitConfig + bootstrap.Deps struct 收敛签名；14 个新单测 case + 2 条 e2e 集成 case；x/time 升级 direct dep；`bash scripts/build.sh --test` 全绿；状态 ready-for-dev → review。
