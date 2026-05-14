package middleware

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"github.com/huing/cat/server/internal/infra/config"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
)

// KeyExtractor 是 RateLimit 的 key 提取函数，让调用方决定按 IP 还是 userID
// （或其他维度）限频。
//
// 返回空串会被 RateLimit 视为"无 key"信号 → 放行（保守路径，防误伤）；
// 调用方应保证 extractor 返回非空 key。
type KeyExtractor func(c *gin.Context) string

// RateLimitByIP 用 c.RemoteIP() 作为 key（直接取 Request.RemoteAddr 的 host
// 部分，**不**解析 X-Forwarded-For / X-Real-IP）。
//
// 适用：登录前路径（无 userID） —— V1 §4.1 行 218 钦定的 "同 IP 每分钟 60 次"
// 限频路径（如 /auth/guest-login）。
//
// # 安全性：为什么不是 c.ClientIP()
//
// c.ClientIP() 默认会信任 X-Forwarded-For / X-Real-IP（除非 engine 调过
// SetTrustedProxies(nil) 或显式白名单）。在限频维度，**信任客户端 header 是
// 高危反模式** —— 攻击者每次请求伪造一个新 X-Forwarded-For → 每次都被认为
// 是新 IP key → 60/min 限制被完全绕过。
//
// **限频 key 必须基于 server 端唯一可信的源 IP**（TCP 连接 RemoteAddr），所以
// 这里直接 c.RemoteIP() 跳过 XFF 解析。即使生产部署后 SetTrustedProxies 配
// 错或漂移，限频维度仍然有连接层兜底。
//
// 反代部署（CDN / nginx）下 RemoteAddr 是反代 IP（所有请求看似同 IP）→ 需要
// 反代层做 IP 限频，或者切到 X-Real-IP **白名单校验**后才信任。这是节点 36
// 部署 epic 的事，本 story 阶段无反代，c.RemoteIP() 即真客户端 IP。
//
// **配套**：bootstrap.NewRouter 调 r.SetTrustedProxies(nil) 让 ClientIP() 也
// 不信任 XFF（防其他依赖 ClientIP 的代码踩坑）。详见 router.go 顶部注释。
func RateLimitByIP(c *gin.Context) string {
	return "ip:" + c.RemoteIP()
}

// RateLimitByUserID 优先用 UserIDKey（Auth 中间件注入），fallback IP
// （防御 Auth 中间件未挂场景）。
//
// 适用：登录后路径（已认证）—— 防同 user 高频调业务接口；同 IP 多 user
// 不互相影响 NAT 共享场景。
//
// 当 Auth 中间件已挂在前面时，UserIDKey 必然存在 → 走 userID 维度（同 IP 多
// user 隔离）；边缘情况（dev 误配 / 测试漏挂 auth）→ fallback IP，至少有
// 限频保底。
func RateLimitByUserID(c *gin.Context) string {
	if v, ok := c.Get(UserIDKey); ok {
		if uid, ok := v.(uint64); ok {
			return fmt.Sprintf("user:%d", uid)
		}
	}
	// fallback IP 维度同样必须用 RemoteIP（不信任 XFF），理由同 RateLimitByIP。
	return "ip:" + c.RemoteIP()
}

// RateLimit 工厂：构造一个基于内存 token bucket 的限频中间件。
//
// # 配置
//
// cfg.PerKeyPerMin: 每 key 每分钟允许的请求数（默认 60；epics.md 行 1039 钦定）
// cfg.BurstSize:    令牌桶容量（瞬时突发上限；缺省 = PerKeyPerMin）
// cfg.BucketsLimit: 内存桶 map 上限（防 IP 洪泛攻击耗内存；缺省 10000）
//
// PerKeyPerMin / 60 = 每秒平均速率；BurstSize 是 burst 上限。
//
// # 启动期 fail-fast
//
// extractor == nil → panic
// cfg.PerKeyPerMin == nil 或 *cfg.PerKeyPerMin <= 0 → panic
// cfg.BurstSize == nil 或 cfg.BucketsLimit == nil → panic（caller bug：未走 loader）
// （MEMORY.md "No Backup Fallback" 钦定反对 silent fallback；启动期暴露问题，
// 避免业务请求才发现限频策略无效。）
//
// 缺省：*cfg.BurstSize <= 0 → 用 PerKeyPerMin 兜底；
// *cfg.BucketsLimit <= 0 → 兜底 10000。
//
// **指针类型说明**：cfg 字段为 *int64 而非 int —— 4.5 round 2 [P2] 引入，
// 用于让 loader 区分"YAML 缺字段"（nil → 填默认）与"YAML 显式 0"（&0 → 透传 panic）。
// 详见 config.RateLimitConfig 顶部说明。
//
// # 内存管理（防洪泛）
//
// 内部用 sync.Map 存 key → *rate.Limiter；新 key 进入时**先 CAS 预占一个槽位**
// 再 LoadOrStore（避免多 goroutine 同时观察 count<limit 各自创建超过 limit 个
// bucket 的 race —— 旧实现"先 LoadOrStore 再 count.Add"在并发洪泛下 map size
// 可膨胀到任意大）。CAS 失败（已达上限）→ 走共享降级 bucket。
//
// 当 counter ≥ BucketsLimit → 不再为新 key 创建 bucket，而是用一个**共享降级
// bucket** 给所有溢出 key 限流 —— 等价于"所有溢出 key 共享同一速率"，OK 防 OOM
// 又不至于 100% 拒绝合法用户。
//
// **不**起独立 cleanup goroutine：
//  1. 每个 limiter 内存约 ~100 字节 → 10000 上限 ~1MB，可接受
//  2. 节点 2 单实例部署 → server 进程重启会自然清空
//  3. cleanup goroutine 引入额外复杂度（select + ticker + 安全停机）—— 不值得
//  4. 节点 10+ 切 Redis-based 后，问题自然消失（Redis 自身 eviction 处理）
//
// # 错误映射
//
// 超限 → c.Error(apperror.New(ErrTooManyRequests, "操作过于频繁")) + c.Abort；
// ErrorMappingMiddleware 写 1005 envelope（V1 §3 钦定）。
//
// # 时间源
//
// 用 time.Now() 直接调（rate.Limiter 内部）。"跨分钟边界 token 回填" 测试用
// time.Sleep 跨真实时间。
//
// # 线程安全
//
// rate.Limiter 内部已用 mutex；sync.Map.LoadOrStore 防双重创建竞态；
// atomic.Int64 维护计数；并发安全。
//
// # 配置 reload
//
// MVP **不**支持 hot reload：YAML 改了要重启 server。Future epic 加 SIGHUP /
// config 监控。
func RateLimit(cfg config.RateLimitConfig, extractor KeyExtractor) gin.HandlerFunc {
	h, _ := newRateLimit(cfg, extractor)
	return h
}

// userIDRateChecker 是 Story 20.6 引入的 handler 内层 rate_limit 专用限频器
// （V1 §7.2.5.4 r10 钦定：POST /chest/open 路由层 opt-out RateLimit middleware，
// rate_limit 检查由 handler 在 idempotency 命中预检之后显式调用，cached success
// replay 免配额）。
//
// **设计权衡**：
//   - 现有 `RateLimit` 工厂每次返回独立闭包（带独立 buckets map）；handler 内层
//     直接调 `RateLimit(...)` 会让每次请求都构造新闭包 → 完全失效
//   - 解决路径：复用 newRateLimit 核心限频逻辑（per-key Limiter + buckets sync.Map
//     + count atomic + overflow 兜底）；新增 Check 方法接受 user-ID key 直接执行
//   - 与 `RateLimit` 工厂共享内部行为（user-ID extractor 同模式 + 同 cfg 解析）；
//     仅替换 caller-facing 路径（gin.HandlerFunc → 返 error）
type userIDRateChecker struct {
	buckets      *sync.Map
	count        *atomic.Int64
	overflow     *rate.Limiter
	perSec       rate.Limit
	burst        int
	bucketsLimit int64
}

// newUserIDRateChecker 按 cfg 构造一个新的限频实例；cfg 解析逻辑与 newRateLimit 一致
// （任一 cfg 字段 nil / 非法 → panic；与 RateLimit 工厂的启动期 fail-fast 约束一致）。
func newUserIDRateChecker(cfg config.RateLimitConfig) *userIDRateChecker {
	if cfg.PerKeyPerMin == nil {
		panic("middleware.CheckRateLimitByUserID: PerKeyPerMin must not be nil")
	}
	if cfg.BurstSize == nil {
		panic("middleware.CheckRateLimitByUserID: BurstSize must not be nil")
	}
	if cfg.BucketsLimit == nil {
		panic("middleware.CheckRateLimitByUserID: BucketsLimit must not be nil")
	}
	perKeyPerMin := *cfg.PerKeyPerMin
	if perKeyPerMin <= 0 {
		panic(fmt.Sprintf("middleware.CheckRateLimitByUserID: PerKeyPerMin must be > 0, got %d", perKeyPerMin))
	}
	burstSize := *cfg.BurstSize
	if burstSize <= 0 {
		burstSize = perKeyPerMin
	}
	bucketsLimit := *cfg.BucketsLimit
	if bucketsLimit <= 0 {
		bucketsLimit = 10000
	}
	perSec := rate.Limit(float64(perKeyPerMin) / 60.0)
	burst := int(burstSize)
	return &userIDRateChecker{
		buckets:      &sync.Map{},
		count:        &atomic.Int64{},
		overflow:     rate.NewLimiter(perSec, burst),
		perSec:       perSec,
		burst:        burst,
		bucketsLimit: bucketsLimit,
	}
}

// Check 按 userID 走限频判定（与 newRateLimit handler 内 buckets / overflow / CAS
// 逻辑等价）。
//
// 返回值：
//   - nil: 通过
//   - *apperror.AppError(ErrTooManyRequests=1005, ...): 超限
//
// **userID == 0 兜底**：按 "key 为空 → 放行" 路径（与 newRateLimit handler 内
// `if key == ""` 分支一致）。实际场景 auth 中间件已注入合法 userID，应是不可达分支；
// 保守放行避免阻塞合法请求。
func (rc *userIDRateChecker) Check(userID uint64) error {
	if userID == 0 {
		// key 为空场景 → 放行（与 newRateLimit `if key == ""` 一致）
		return nil
	}
	key := fmt.Sprintf("user:%d", userID)
	var lim *rate.Limiter
	if v, ok := rc.buckets.Load(key); ok {
		lim = v.(*rate.Limiter)
	} else {
		// CAS 预占一个 bucket 槽位再 LoadOrStore（与 newRateLimit 同模式）
		reserved := false
		for {
			cur := rc.count.Load()
			if cur >= rc.bucketsLimit {
				break
			}
			if rc.count.CompareAndSwap(cur, cur+1) {
				reserved = true
				break
			}
		}
		if !reserved {
			lim = rc.overflow
		} else {
			newLim := rate.NewLimiter(rc.perSec, rc.burst)
			actual, loaded := rc.buckets.LoadOrStore(key, newLim)
			lim = actual.(*rate.Limiter)
			if loaded {
				rc.count.Add(-1)
			}
		}
	}
	if !lim.Allow() {
		return apperror.New(apperror.ErrTooManyRequests, apperror.DefaultMessages[apperror.ErrTooManyRequests])
	}
	return nil
}

// chestOpenUserIDLimiter 是 process 级别的 handler 内层 rate_limit 单例
// （sync.Once 守护 lazy init；首次 CheckRateLimitByUserID 调用时按 cfg 构造）。
//
// **限制**：cfg 一次性冻结；首次构造后即使再传入不同 cfg 也只用首次的 cfg
// （与 RateLimit 工厂 "YAML 改了要重启" 约束一致）。
var (
	chestOpenUserIDLimiterOnce sync.Once
	chestOpenUserIDLimiter     *userIDRateChecker
)

// CheckRateLimitByUserID 是 Story 20.6 引入的 handler 内层 rate_limit 入口
// （V1 §7.2.5.4 r10 钦定）。
//
// **使用场景**：POST /chest/open 路由层 opt-out RateLimit middleware；handler 在
// idempotency autocommit 预检后，仅对**未命中** committed success replay 的请求
// 显式调本函数做限频兜底。命中 success replay 的请求**跳过**本函数（免配额）。
//
// 返回值：
//   - nil：通过（请求未超频；调用方继续走业务事务）
//   - *apperror.AppError(ErrTooManyRequests=1005, ...)：超限；调用方应
//     `c.Error(err) + return`（中间件链会把 1005 envelope 写回）
//
// **cfg 一次性冻结**：首次调用本函数时按 cfg 构造 limiter 实例；后续调用即使
// 传入不同 cfg，也只用首次的 cfg。process 重启才能切到新 cfg。这与 RateLimit
// 工厂 "YAML 改了要重启" 约束一致（详见 RateLimit 顶部 # 配置 reload 段）。
//
// **ctx 当前未消费**：保留 ctx 参数为 future log / trace ID 关联预留；当前实装内部
// 限频逻辑不消费 ctx（rate.Limiter.Allow() 不接 ctx）。
func CheckRateLimitByUserID(ctx context.Context, cfg config.RateLimitConfig, userID uint64) error {
	_ = ctx // 未消费；future log / trace 关联预留
	chestOpenUserIDLimiterOnce.Do(func() {
		chestOpenUserIDLimiter = newUserIDRateChecker(cfg)
	})
	return chestOpenUserIDLimiter.Check(userID)
}

// resetChestOpenUserIDLimiterForTest 是测试专用 hook（同 package 单测访问）。
//
// **不**导出（仅同包测试访问）；跨包测试用 ResetChestOpenUserIDLimiterForTest 导出版本。
func resetChestOpenUserIDLimiterForTest() {
	chestOpenUserIDLimiterOnce = sync.Once{}
	chestOpenUserIDLimiter = nil
}

// ResetChestOpenUserIDLimiterForTest 是测试专用导出 hook。
//
// **关键警告**：本函数**仅供测试用**！生产代码绝对不应调用 —— 会重置 sync.Once
// + 丢失现有 limiter 实例，让所有进行中的限频窗口归零（可被滥用为绕过限频）。
//
// 用于跨包测试（如 handler 包测试 chest_open）需要重置 process 级 limiter 状态，
// 避免不同 case 之间状态串扰。
func ResetChestOpenUserIDLimiterForTest() {
	resetChestOpenUserIDLimiterForTest()
}

// newRateLimit 是 RateLimit 的内部实现，额外返回一个 *atomic.Int64（buckets
// 当前 key 数）给同包白盒测试断言"buckets size 不超过 BucketsLimit"用。
//
// 生产路径走 RateLimit 包装，丢弃 counter。
//
// **指针 deref 约定**（Story 4.5 round 2 [P2] 引入）：cfg 字段全部 *int64。
// 调用方（loader.Load 或测试）必须保证非 nil；若任一字段 nil → panic（caller bug）。
// 详见 config.RateLimitConfig 顶部 "为什么字段是 *int64"。
func newRateLimit(cfg config.RateLimitConfig, extractor KeyExtractor) (gin.HandlerFunc, *atomic.Int64) {
	if extractor == nil {
		panic("middleware.RateLimit: extractor must not be nil")
	}
	if cfg.PerKeyPerMin == nil {
		panic("middleware.RateLimit: PerKeyPerMin must not be nil (caller must populate via loader or struct literal)")
	}
	if cfg.BurstSize == nil {
		panic("middleware.RateLimit: BurstSize must not be nil (caller must populate via loader or struct literal)")
	}
	if cfg.BucketsLimit == nil {
		panic("middleware.RateLimit: BucketsLimit must not be nil (caller must populate via loader or struct literal)")
	}
	perKeyPerMin := *cfg.PerKeyPerMin
	if perKeyPerMin <= 0 {
		panic(fmt.Sprintf("middleware.RateLimit: PerKeyPerMin must be > 0, got %d", perKeyPerMin))
	}
	burstSize := *cfg.BurstSize
	if burstSize <= 0 {
		burstSize = perKeyPerMin
	}
	bucketsLimit := *cfg.BucketsLimit
	if bucketsLimit <= 0 {
		bucketsLimit = 10000
	}
	// 每秒速率 = 每分钟 / 60
	perSec := rate.Limit(float64(perKeyPerMin) / 60.0)
	burst := int(burstSize)
	limit := bucketsLimit

	var (
		buckets sync.Map // map[string]*rate.Limiter
		count   atomic.Int64
		// 共享降级 bucket：当 buckets 数达上限时，所有新 key 共用此 limiter
		// 防御 IP 洪泛把内存撑爆
		overflow = rate.NewLimiter(perSec, burst)
	)

	handler := func(c *gin.Context) {
		key := extractor(c)
		if key == "" {
			// 取不到 key（极罕见：c.ClientIP() 为空 + 没 userID）
			// → 不限频放行（保守路径，防误伤）。Future 可改为 1005。
			c.Next()
			return
		}

		var lim *rate.Limiter
		if v, ok := buckets.Load(key); ok {
			lim = v.(*rate.Limiter)
		} else {
			// CAS 预占一个 bucket 槽位，再做 LoadOrStore。
			//
			// 旧实现先 LoadOrStore 再 count.Add(1) 在并发洪泛下不 bounded：
			// 多 goroutine 都看到 count<limit → 各自 LoadOrStore 不同 key →
			// count 后 +1 但 map 已经 > limit。
			//
			// CAS 模式：先抢配额，抢到才创建 limiter；抢不到走 overflow。
			// LoadOrStore 发现 key 已存在（loaded=true，少见但可能：另一
			// goroutine 同 key 已抢配额并写入）→ 撤销刚抢到的配额。
			reserved := false
			for {
				cur := count.Load()
				if cur >= limit {
					break
				}
				if count.CompareAndSwap(cur, cur+1) {
					reserved = true
					break
				}
				// CAS 失败 → 别的 goroutine 改了 count，重读重试
			}
			if !reserved {
				lim = overflow
			} else {
				newLim := rate.NewLimiter(perSec, burst)
				actual, loaded := buckets.LoadOrStore(key, newLim)
				lim = actual.(*rate.Limiter)
				if loaded {
					// 同 key 已存在：撤销刚抢到的配额，避免 count 膨胀
					count.Add(-1)
				}
			}
		}

		if !lim.Allow() {
			_ = c.Error(apperror.New(apperror.ErrTooManyRequests, apperror.DefaultMessages[apperror.ErrTooManyRequests]))
			c.Abort()
			return
		}
		c.Next()
	}
	return handler, &count
}
