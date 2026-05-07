package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/huing/cat/server/internal/app/bootstrap"
	"github.com/huing/cat/server/internal/app/http/devtools"
	wsapp "github.com/huing/cat/server/internal/app/ws"
	"github.com/huing/cat/server/internal/cli"
	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
	"github.com/huing/cat/server/internal/infra/logger"
	redisinfra "github.com/huing/cat/server/internal/infra/redis"
	"github.com/huing/cat/server/internal/pkg/auth"
	redisrepo "github.com/huing/cat/server/internal/repo/redis"
	"github.com/huing/cat/server/internal/repo/tx"
)

// presenceHookTimeout 是 Story 10.6 钩子 adapter 内 RedisClient 调用的 short-timeout
// 上限。
//
// 选 2s 的理由：
//   - 单条 Redis 命令在 local Redis < 1ms / remote Redis < 100ms（含 RTT）；2s 是
//     "Redis 卡住"病态场景的兜底，让钩子 adapter 不会无限阻塞 SessionManager.Close
//     的 graceful shutdown 主流程
//   - 钩子 adapter 走 context.Background() 派生的 short-timeout ctx，**不**走 main
//     ctx；理由见 main.go wire 处的注释（main ctx 在 SIGTERM 时被 cancel，graceful
//     shutdown 期所有 RemoveOnline 都会因 ctx.Canceled 返 error → presence 留僵尸
//     5 分钟 → 违反"graceful shutdown 必须清空 presence"语义）
//
// 详见 docs/lessons/2026-05-07-ws-shutdown-must-wait-for-goroutine-exit-not-just-signal-10-4-r6.md
const presenceHookTimeout = 2 * time.Second

// dbOpenTimeout 是启动阶段调 db.Open 的最大等待。
//
// 为什么独立于 main 的 signal-ctx：`signal.NotifyContext` 创建的 ctx 只在收到
// SIGINT/SIGTERM 时 cancel，**没有 deadline**。如果 DSN 指向 blackholed host 或
// DNS 解析慢，PingContext 会被 driver 的 default dial timeout（典型 30s+）卡住，
// fail-fast 语义实际不快。这里包一层短 timeout，强制启动阶段 5s 内见结果。
//
// 选 5s 是平衡：
//   - 太短（< 2s）会误杀慢机 / VPN / 本地 docker mysql 启动延迟场景
//   - 太长（> 10s）违反 fail-fast 初衷（运维等不到 readiness 失败信号）
//
// 短 timeout 仅作用于 db.Open；后续 bootstrap.Run 仍用主 signal-ctx，正常 server
// lifecycle 不受影响。详见 docs/lessons/2026-04-26-startup-blocking-io-needs-deadline.md。
const dbOpenTimeout = 5 * time.Second

// userKeyedMutex 是同 userID 串行化的小工具：每个 userID 关联一把 *sync.Mutex，
// 同 userID 的多次 Lock 排队，不同 userID 互不阻塞。
//
// review 10-6 r8 P1 加：fire-and-forget hook goroutines 同 userID 的 Add/Remove
// 必须**串行**执行 —— quick connect-then-close 或 reconnect 替换路径下 AddOnline(new)
// 与 RemoveOnline(old) 两个独立 goroutine 调度顺序不定，可能 RemoveOnline 先跑 →
// AddOnline 后跑，让 presence 复活已离线 session 的 keys 直到 TTL 自然过期（IsOnline
// / ListOnline over-report 5min）。
//
// 用 sync.Map[userID]*sync.Mutex 而非裸 map + 全局 RWMutex：sync.Map 在 read-heavy
// + key 集合稳定的场景（同 userID 短时间内多次 Add/Remove，不同 userID 之间几乎
// 无重叠）下走无锁 fast path；与 r3 P2 lesson "Load fast path" 模式一致。
//
// 内存模型：sync.Map entry 不会自动回收 —— 长生命周期 server 累积大量 user 的
// mutex 是 O(active users) 内存开销。考虑到单实例 MVP < 万级 user，每条 entry
// 约 48 字节（uint64 key + *Mutex value + sync.Map 内部开销）= < 0.5 MB，可接受。
// 节点 13+ 多实例水平扩展时如果出现内存压力，再考虑加 LRU evict（需要锁保护让
// evict 与正在持锁的 goroutine 不冲突）。
//
// 详见 docs/lessons/2026-05-07-fire-and-forget-hooks-need-per-user-mutex-10-6-r8.md。
type userKeyedMutex struct {
	m sync.Map // map[uint64]*sync.Mutex
}

// lockFor 返回 userID 对应的 *sync.Mutex —— 调用方拿到后调 Lock/Unlock 自行控制
// 临界区。多次 lockFor 同 userID 返回同一 *Mutex 实例（sync.Map LoadOrStore 保证
// 首次 store 后续 Load 命中同地址）。
//
// 走 r3 P2 lesson 的 "Load fast path"：先 m.Load 单次原子命中，miss 才走
// LoadOrStore（避免每次 hook 都分配一个 *sync.Mutex 然后又被 GC）。
func (u *userKeyedMutex) lockFor(userID uint64) *sync.Mutex {
	if muVal, ok := u.m.Load(userID); ok {
		return muVal.(*sync.Mutex)
	}
	muVal, _ := u.m.LoadOrStore(userID, &sync.Mutex{})
	return muVal.(*sync.Mutex)
}

// redisOpenTimeout 是启动阶段调 redis.Open 的最大等待。
//
// 与 dbOpenTimeout 同模式（详见上文）：主 signal-ctx 没 deadline，碰到 blackhole
// host / 慢 DNS 时 PingContext 会被 driver 卡 30s+，fail-fast 实际不快。
// Story 10.2 引入。
const redisOpenTimeout = 5 * time.Second

func main() {
	// Bootstrap logger 先拿 "info" level 的 JSON handler（filePath 留空 → 只写 stdout），
	// 确保 config 加载失败这类**早期启动错误**也走结构化 JSON 输出。config 加载成功后会用
	// 实际配置的 level + file 再 Init 一次。
	// 见 docs/lessons/2026-04-25-slog-init-before-startup-errors.md。
	logger.Init("info", "")

	// Story 4.3 review fix：子命令路径必须**先**拆 args 再 flag.Parse。
	//
	// `flag.Parse()` 在第一个非 flag 参数后停止解析。文档化的调用形态
	// `catserver migrate up -config configs/dev.yaml` 中 args[0]="migrate" 是
	// 第一个非 flag → 默认 flag.Parse 直接停在这里，**不**会解析后面的 -config，
	// migrate 子命令会用 LocateDefault 找到的 local.yaml（错误的 DB）。
	//
	// 修法：parseTopLevelArgs 把 os.Args[1:] 拆成 (preMigrate, postMigrate)，
	// preMigrate 走 flag.NewFlagSet 解析（覆盖 `catserver -config X migrate up` 形态），
	// postMigrate 通过 cli.RunMigrate → cli.ParseMigrateArgs 用独立 NewFlagSet 解析
	// （覆盖 `catserver migrate up -config X` 形态）。两条路径都能拿到正确的 -config。
	configPath, migrateArgs, isMigrate := parseTopLevelArgs(os.Args[1:])

	// Story 4.3 review round 2 fix：migrate 子命令必须**绕过** main 的 default config load。
	//
	// 否则 `catserver migrate up -config dev.yaml`（CI/container 只 ship dev.yaml，
	// 没有 local.yaml）会在下面 LocateDefault → Load 阶段直接 os.Exit(1)，根本走不到
	// migrate 分支让 RunMigrate 消费自己的 -config。
	//
	// 修法：检测到 migrate 路径后立刻进入分支；让 RunMigrate 自己负责 config 加载
	// （cfg=nil 传入，args 里有 -config 用 -config，没有走 LocateDefault）。
	// 详见 docs/lessons/2026-04-26-cli-cancellation-and-gomigrate-gracefulstop.md。
	if isMigrate {
		// migrate 分支用**自己的** signal-ctx —— migrate up 跑多个 SQL 文件可能耗时
		// 几十秒，且现在 ctx-aware（gomigrate.GracefulStop chan）能让 SIGINT 在
		// statement 边界提前停下。详见 internal/infra/migrate/migrate.go。
		migrateCtx, migrateStop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer migrateStop()
		// preMigrate 路径上若也有 -config（`catserver -config X migrate up` 形态），
		// 仍优先用它 —— 通过 Load 拿到 cfg 传给 RunMigrate；RunMigrate 内部 -config
		// override 优先级仍比传入 cfg 高（`catserver -config A migrate up -config B`
		// 用 B；这种调用形态边缘但语义一致）。
		var preCfg *config.Config
		if configPath != "" {
			c, err := config.Load(configPath)
			if err != nil {
				slog.Error("config load failed", slog.Any("error", err), slog.String("path", configPath))
				os.Exit(1)
			}
			preCfg = c
			logger.Init(c.Log.Level, c.Log.File)
		}
		if err := cli.RunMigrate(migrateCtx, preCfg, migrateArgs); err != nil {
			slog.Error("migrate failed", slog.Any("error", err))
			os.Exit(1)
		}
		os.Exit(0)
	}

	// 非 migrate 路径：走原有 default config load 流程。
	if configPath == "" {
		p, err := config.LocateDefault()
		if err != nil {
			slog.Error("config locate failed", slog.Any("error", err))
			os.Exit(1)
		}
		configPath = p
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("config load failed", slog.Any("error", err))
		os.Exit(1)
	}

	logger.Init(cfg.Log.Level, cfg.Log.File)
	slog.Info("config loaded",
		slog.String("path", configPath),
		slog.Int("http_port", cfg.Server.HTTPPort),
		slog.String("log_level", cfg.Log.Level),
		slog.String("log_file", cfg.Log.File),
	)

	// Dev 模式（BUILD_DEV=true 或 build tag `devtools`）启用时在启动阶段打一条
	// 醒目 WARN。放在 logger.Init(cfg.Log.Level) 之后 → 用户配置的 log level
	// 已生效；devtools.Register 内部会在路由实际注册完成后再打一条同内容 WARN，
	// 两条共同构成完整的"触发源 → 注册完成"链路，便于 ops 排障。
	if devtools.IsEnabled() {
		slog.Warn("DEV MODE ENABLED - DO NOT USE IN PRODUCTION")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Story 4.2：MySQL 接入 + tx manager 构造。失败必须 fail-fast（os.Exit(1)）：
	//   - DSN 空（local.yaml 漏配）
	//   - DSN 解析错（驱动 parse 失败）
	//   - PingContext 失败（network unreachable / auth / MySQL 未起来）
	//
	// 不容忍降级：用空连接池继续启动会让用户在第一个业务请求时才发现问题，
	// 违反 NFR3 / 总体架构设计 §"状态以 server 为准" + MEMORY.md "No Backup Fallback"。
	//
	// **必须用本地短 timeout ctx**：主 signal-ctx 没 deadline，碰到 blackhole host /
	// 慢 DNS 时 PingContext 会被 driver 卡 30s+，fail-fast 实际不快。dbOpenTimeout 强制
	// 启动阶段 5s 内见结果；timeout 触发后 cancel 仅影响 db.Open 这一阻塞 IO，主 ctx
	// 不受影响。Story 4.2 review round 2 补漏。
	dbOpenCtx, dbOpenCancel := context.WithTimeout(ctx, dbOpenTimeout)
	gormDB, err := db.Open(dbOpenCtx, cfg.MySQL)
	dbOpenCancel()
	if err != nil {
		slog.Error("mysql open failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		sqlDB, sqlErr := gormDB.DB()
		if sqlErr != nil {
			// 几乎不可能（gormDB 已经 Open 成功，DB() 这时不应该失败）；保险起见 log
			slog.Error("mysql close: get *sql.DB failed", slog.Any("error", sqlErr))
			return
		}
		if cerr := sqlDB.Close(); cerr != nil {
			slog.Error("mysql close failed", slog.Any("error", cerr))
		}
	}()

	// Tx manager 构造。本 story 阶段 router / handler 暂不消费（Epic 4 Story 4.6
	// 才挂业务 handler）—— 这里只确保依赖能 wire 起来，不让 gormDB / txMgr 变成
	// 一个全局变量（避免 ADR-0001 §3.4 接口边界 mock 的反模式）。
	txMgr := tx.NewManager(gormDB)
	slog.Info("mysql connected", slog.Int("max_open_conns", cfg.MySQL.MaxOpenConns))

	// Story 10.2：Redis 接入。失败必须 fail-fast：
	//   - addr 空（local.yaml 漏配）→ "redis.addr is empty"
	//   - 连接失败 / authentication failed / wrong db → fmt.Errorf("redis ping: %w", err)
	//
	// 与 db.Open 同模式：本地短 timeout ctx 强制 5s 内见结果，避免 blackhole host /
	// 慢 DNS 卡 30s+。本 story 阶段 router / handler 暂不消费 RedisClient（Story 10.6
	// 才挂 presence repo）—— 这里只确保依赖能 wire 起来。
	redisOpenCtx, redisOpenCancel := context.WithTimeout(ctx, redisOpenTimeout)
	redisClient, err := redisinfra.Open(redisOpenCtx, cfg.Redis)
	redisOpenCancel()
	if err != nil {
		slog.Error("redis open failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		if cerr := redisClient.Close(); cerr != nil {
			slog.Error("redis close failed", slog.Any("error", cerr))
		}
	}()
	slog.Info("redis connected",
		slog.String("addr", cfg.Redis.Addr),
		slog.Int("pool_size", cfg.Redis.PoolSize),
		slog.Int("db", cfg.Redis.DB),
	)

	// Story 10.6：构造 PresenceRepo + 注入 SessionManager lifecycle 钩子。
	//
	// 流程：
	//  1. 用 cfg.Redis.PresenceTTLSec（YAML 字段 redis.presence_ttl_sec；默认 300 = 5min）
	//     构造 PresenceRepo 实例（loader 兜底 <= 0 → 默认；本调用点直接消费即可）
	//  2. 通过 functional option 把 AddOnline / RemoveOnline 钩子挂到 SessionManager:
	//     - WithRegisterHook(adapter func(*Session)) → presenceRepo.AddOnline
	//     - WithUnregisterHook(adapter func(*Session)) → presenceRepo.RemoveOnline
	//  3. adapter closure 内：拿 Session.UserID() / RoomID() / SessionID() 走 presence
	//     方法；走 short-timeout ctx (presenceHookTimeout)，**不**走 main ctx
	//
	// **关键决策**：钩子 adapter 内的 ctx 走 context.WithTimeout(context.Background(),
	// presenceHookTimeout)，**不**走 main ctx。理由：lifecycle 钩子在 ws connection
	// register / unregister 时刻触发，与 main ctx 的 SIGTERM cancel 时机正交；如果
	// 走 main ctx，graceful shutdown 期 ctx 已 cancel 后所有 RemoveOnline 都会返
	// ctx.Canceled error 让 presence 留僵尸记录到 TTL 自然清除（5 分钟）—— 违反
	// graceful shutdown 必须清空 presence 的语义。详见 lesson
	// 2026-05-07-ws-shutdown-must-wait-for-goroutine-exit-not-just-signal-10-4-r6.md。
	presenceRepo := redisrepo.NewPresenceRepo(redisClient, time.Duration(cfg.Redis.PresenceTTLSec)*time.Second)
	slog.Info("presence repo ready",
		slog.Int("ttl_sec", cfg.Redis.PresenceTTLSec),
	)

	// Story 10.3 + 10.6：WS SessionManager 构造（10.3 加，10.6 加 lifecycle 钩子）。
	// SessionManager 是纯内存对象（map + sync.RWMutex），构造无 IO；不需要 timeout ctx。
	//
	// **lifecycle 钩子**（Story 10.6 加）：
	//   - Register → AddOnline（Session 注册时把 user 加入 Redis presence）
	//   - Unregister → RemoveOnline（Session 注销时从 Redis presence 移除）
	//   - Reconnect 替换路径：oldSession.onUnregister 触发恰好一次（10.3 r2 P1 不变量
	//     已锁定；本钩子直接信任 SessionManager 实装层兜底）
	//
	// 钩子失败仅 log warn + 包含 sessionId / userId / roomId 上下文；**不** os.Exit /
	// panic（lifecycle 钩子失败 ≠ server 必须停机；TTL 兜底 + scanner 路径双重保险）。
	//
	// **review 10-6 r6 P1 修**：钩子内部 Redis I/O 走 **fire-and-forget goroutine**，
	// 不阻塞 SessionManager.Register / Unregister 主路径。理由：
	//   - Register 同步路径：gateway.handleWS 等 Register 返回才启 read/write loop。
	//     原版 AddOnline 同步阻塞 → Redis 慢 / 挂时每次 connect/reconnect 卡 2s
	//     （presenceHookTimeout 上限）→ Redis brownout 期所有用户握手 visible 延迟。
	//   - Unregister 串行路径：SessionManager.Close 串行调 Unregister；reconnect
	//     替换路径也串行调旧 Session.Close → notifyClosed → Unregister。原版同步
	//     RemoveOnline → O(session 数 × 2s) 关停延迟，多会话部署 graceful shutdown
	//     直接超 K8s termination grace。
	//   - Fire-and-forget 后 register/unregister 主路径 < 1ms 完成；presence 写入
	//     在 background goroutine 跑。失败兜底已经多重保险：
	//       (a) presence key TTL 5min 自然过期（server crash / shutdown 时）
	//       (b) HeartbeatScanner 30s tick 调 AddOnline reconcile（10-6 r2/r5 加），
	//           漏写 / partial-fail 30s 内自愈
	//       (c) RemoveOnline guard 走 sessionID atomic compare（10-6 r1 加），
	//           reconnect 替换路径下旧 RemoveOnline 即便延迟跑也不会误清新 Session
	//
	// **review 10-6 r7 P2 修**：fire-and-forget hook goroutines 必须用 sync.WaitGroup
	// 跟踪，graceful shutdown 路径上**等所有 hook goroutine 跑完才关 Redis client**。
	// 原版 r6 注释里说"shutdown 期 fire-and-forget goroutine 可能没机会跑完 → 可接受
	// （TTL 5min 兜底）"，但实际后果是：sessionMgr.Close() 串行触发 N 个 Unregister
	// 钩子 → 每个 dispatch 一个 RemoveOnline goroutine 后立即返；defer LIFO 让
	// redisClient.Close() 紧接着关闭共享 client → in-flight RemoveOnline goroutines
	// 撞 "redis: client is closed" → 大量 / 全部 presence removal 被跳过 → 每次
	// clean restart 后 room:*:online_users 留 stale member 直到 TTL 5min 过期。
	// 即便有 TTL 兜底，IsOnline / ListOnline 在 5min 内会 over-report，违反"graceful
	// shutdown 必须清空 presence"的设计意图。修法：sync.WaitGroup + 在 sessionMgr.Close()
	// 之后调 wg.Wait() 让 Redis 关闭推迟到所有 hook goroutine 真正退出。
	// 详见 lesson 2026-05-07-presence-same-room-reconnect-needs-room-aware-guard-10-6-r7.md。
	//
	// **review 10-6 r8 P1 修**：fire-and-forget hook goroutines 同 userID 的 Add/Remove
	// 必须**串行**执行 —— 否则 quick connect-then-close 或 reconnect 替换路径下
	// AddOnline(new) 与 RemoveOnline(old) 两个独立 goroutine 调度顺序不定，可能
	// RemoveOnline 先跑 → AddOnline 后跑，让 presence 复活已离线 session 的 keys
	// 直到 TTL 自然过期（IsOnline / ListOnline over-report 5min）。
	//
	// 修法：用 sync.Map[userID]*sync.Mutex；同 userID 的 hook goroutine 在内部抢同
	// 一把锁 → 强制 register / unregister 在 Redis 端的命令顺序与 SessionManager
	// 端的钩子触发顺序一致。同 user 不同 session 的 Add/Remove 也走这把锁（reconnect
	// 替换路径下 AddOnline(new) 与 RemoveOnline(old) 都是同 userID）。
	//
	// 锁拿取走 r3 P2 lesson 的 "Load fast path"：见 userKeyedMutex 文档。
	// 详见 lesson 2026-05-07-fire-and-forget-hooks-need-per-user-mutex-10-6-r8.md。
	var presenceHooksWG sync.WaitGroup
	var userPresenceMu userKeyedMutex // 同 userID 的 hook 串行（r8 P1 加；类型 godoc 详述）
	sessionMgr := wsapp.NewSessionManager(
		wsapp.WithRegisterHook(func(s *wsapp.Session) {
			presenceHooksWG.Add(1)
			go func() {
				defer presenceHooksWG.Done()
				// 同 userID 的 Add/Remove 串行（r8 P1）—— Lock 必须在 ctx WithTimeout
				// 之前拿，否则 timeout 期已经从锁队列里出去了再做 Redis I/O 没意义
				mu := userPresenceMu.lockFor(s.UserID())
				mu.Lock()
				defer mu.Unlock()
				hookCtx, cancel := context.WithTimeout(context.Background(), presenceHookTimeout)
				defer cancel()
				if err := presenceRepo.AddOnline(hookCtx, s.RoomID(), s.UserID(), s.SessionID()); err != nil {
					slog.Warn("presence add online failed",
						slog.String("sessionId", s.SessionID()),
						slog.Uint64("userId", s.UserID()),
						slog.Uint64("roomId", s.RoomID()),
						slog.Any("error", err),
					)
				}
			}()
		}),
		wsapp.WithUnregisterHook(func(s *wsapp.Session) {
			presenceHooksWG.Add(1)
			go func() {
				defer presenceHooksWG.Done()
				// 同 userID 的 Add/Remove 串行（r8 P1）
				mu := userPresenceMu.lockFor(s.UserID())
				mu.Lock()
				defer mu.Unlock()
				hookCtx, cancel := context.WithTimeout(context.Background(), presenceHookTimeout)
				defer cancel()
				// **关键**（Story 10.6 r1 P1 修）：必须传入 s.SessionID() 让 RemoveOnline
				// 走 sessionID guard 的 Lua script 路径。reconnect 替换场景下旧 Session
				// 延后 Unregister 触发本钩子时，user:{id}:ws_session 已被新 Session 覆盖
				// 为新 sessionID → script 比较失败走 no-op，不会清掉新 Session 的 presence。
				// 详见 lesson 2026-05-07-redis-presence-remove-needs-session-id-guard-10-6-r1.md。
				if err := presenceRepo.RemoveOnline(hookCtx, s.RoomID(), s.UserID(), s.SessionID()); err != nil {
					slog.Warn("presence remove online failed",
						slog.String("sessionId", s.SessionID()),
						slog.Uint64("userId", s.UserID()),
						slog.Uint64("roomId", s.RoomID()),
						slog.Any("error", err),
					)
				}
			}()
		}),
	)
	slog.Info("ws session manager ready (with presence hooks)")

	// Story 10.4：HeartbeatScanner 构造 + 启动后台 goroutine。
	// scanner 周期性扫描 SessionManager 内所有 Session，超时（cfg.WS.HeartbeatTimeoutSec）
	// 触发 close 4005 frame + 释放本地 Session 资源（V1 §12.1 钦定 4005 = transient
	// network failure，触发 iOS Story 12.5 自动重连指数退避路径）。
	//
	// 用 ctx + cancel 让 scanner 在 main 退出（graceful shutdown）时优雅退出；
	// 不需要 .Stop() 方法（与 Story 8.5 步数同步触发器同模式）。
	//
	// **review r4 P1 修**：heartbeatCtx 必须从 main 的 signal ctx 派生（而非
	// context.Background()）。原版用 Background → SIGTERM 只 cancel main ctx 让
	// bootstrap.Run 退出，scanner 仍然在跑直到 main 返回执行 deferred cancelHeartbeat。
	// 这个 graceful-shutdown window 内（bootstrap.Run 收尾期间），任何 idle ws session
	// 仍可能被 scanner 走 4005 "heartbeat timeout" 关闭，触发 client 的指数退避自动
	// 重连 —— 与 sessionMgr.Close 钦定的"标准下线（无 4005）"路径直接冲突。改成
	// WithCancel(ctx) 后，SIGTERM 立即 cancel scanner.Run 主循环 + 已 dispatched
	// fanout goroutines（fanout 内部本就有 ctx.Done() 入口 check / recheck）。
	// 详见 docs/lessons/2026-05-06-heartbeat-scanner-ctx-tie-to-shutdown-signal.md。
	//
	// **review r6 P1 修**：shutdown 必须**串行等 scanner.Run 真正返回**才能调
	// sessionMgr.Close。原版只 cancelHeartbeat() 然后立刻让另一个 deferred 函数跑
	// sessionMgr.Close —— 由于 r5 给 scanner.Run 的 defer 加了 wg.Wait()（让 Run 在
	// 所有 in-flight fanout drain 完才返回），cancel 与 Run 实际 return 之间存在窗口。
	// 这个窗口内已经过 ctx-check 的 fanout goroutine 仍可调 CloseWithCode(4005,...)，
	// 而 sessionMgr.Close 也在并行跑标准 close 路径 —— 双方对同 Session 并发 close
	// 会让 idle client 收到 4005 而非正常 shutdown close，恰好触发 r4 想消灭的"误重连"。
	//
	// 修法：scannerDone chan 跟踪 Run 真正退出；把 cancelHeartbeat → wait scannerDone →
	// sessionMgr.Close 收成**一个 deferred 函数**（顺序确定，不依赖 LIFO 间接对齐）。
	// 详见 docs/lessons/2026-05-07-shutdown-must-wait-for-goroutine-exit-not-just-signal.md。
	// **review 10-6 r2 P1**：注入 presenceRepo 让 scanner 每 30s tick 给 active
	// session 续 presence TTL（默认 5min）—— 防 long-lived WS session 被 Redis
	// 自动过期误报为 offline。详见 docs/lessons/2026-05-07-presence-ttl-renewal-via-heartbeat-scanner-10-6-r2.md。
	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	heartbeatScanner := wsapp.NewHeartbeatScanner(sessionMgr, cfg.WS.HeartbeatTimeoutSec, slog.Default(), presenceRepo)
	scannerDone := make(chan struct{})
	go func() {
		defer close(scannerDone)
		heartbeatScanner.Run(heartbeatCtx)
	}()
	defer func() {
		// 1. signal scanner 开始退出。
		cancelHeartbeat()
		slog.Info("ws heartbeat scanner cancel signaled")
		// 2. 等 Run 真正返回（含 wg.Wait drain 所有 in-flight fanout）。这一步
		//    完成 = scanner 不会再 emit 任何 4005，sessionMgr.Close 可独占走标准
		//    close 路径。
		<-scannerDone
		slog.Info("ws heartbeat scanner stopped")
		// 3. 现在批量清 Session，没有 4005 race。每个 Session.Close 触发 onUnregister
		//    钩子 → dispatch 一个 fire-and-forget RemoveOnline goroutine（presenceHooksWG.Add(1)）。
		if cerr := sessionMgr.Close(); cerr != nil {
			slog.Error("session manager close failed", slog.Any("error", cerr))
		}
		// 4. **review 10-6 r7 P2 修**：等所有 hook goroutine 跑完才让本 defer 返回 ——
		//    本 defer 是 main 退出栈上**最早**注册的（在 redisClient defer 之后），
		//    LIFO 让 redisClient.Close() 在本 defer 返回**之后**才跑。如果不等
		//    presenceHooksWG，Close() 会立刻关 client → in-flight RemoveOnline 撞
		//    "redis: client is closed" → presence 留 stale 到 TTL 5min 过期。
		//    每个 hook goroutine 上限 = presenceHookTimeout（2s），N 个 session 并发
		//    跑总等待时间 ≈ 单个 timeout 上限（不是 O(N×2s) 串行），完全在 K8s
		//    termination grace 内。详见 lesson
		//    2026-05-07-presence-same-room-reconnect-needs-room-aware-guard-10-6-r7.md。
		presenceHooksWG.Wait()
		slog.Info("ws presence hooks drained")
	}()
	slog.Info("ws heartbeat scanner ready",
		slog.Int("heartbeatTimeoutSec", cfg.WS.HeartbeatTimeoutSec),
	)

	// Story 4.4：JWT signer 构造 fail-fast。
	//
	// auth.New 校验：secret 空 / < 16 字节 / expireSec ≤ 0 / expireSec > 30 天 → 返 error。
	// 任一失败必须 os.Exit(1)：
	//   - 空 secret 让所有 token 可被任意伪造（严重安全漏洞）
	//   - secret 过短易被暴力破解
	//   - 异常 expireSec 违反 V1 §4.1 "默认过期 7 天" 契约
	//
	// 与 db.Open 同 fail-fast 模式（MEMORY.md "No Backup Fallback" 钦定反对 fallback
	// 掩盖核心架构风险；启动时尽早暴露问题，避免业务请求才发现 secret 缺失）。
	//
	// 不需要 timeout ctx：auth.New 是纯 CPU < 1µs，不阻塞 IO（与 db.Open 不同）。
	signer, err := auth.New(cfg.Auth.TokenSecret, cfg.Auth.TokenExpireSec)
	if err != nil {
		slog.Error("auth token signer init failed", slog.Any("error", err))
		os.Exit(1)
	}
	slog.Info("auth token signer ready", slog.Int64("default_expire_sec", cfg.Auth.TokenExpireSec))

	// Story 4.5：bootstrap.Run 签名收敛为 Deps struct（替代之前 5 个平铺参数）。
	// 后续 Story 4.6 / 4.8 / Epic 5+ 加共享依赖时只改 Deps 字段，不再改 Run 签名。
	//
	// **EnvName**（Story 7.3 review r6 [P2]）：从 `CAT_ENV` env 读取，默认 "prod"。
	// 用于 service.NewStepService 强制 prod 必须用默认 cap（5000/50000，跨端契约一部分）。
	// 部署侧只需在 dev / staging / test 显式 `export CAT_ENV=dev|staging|test` 即可允许 YAML 覆盖；
	// 漏配 / prod 部署直接走严格分支（safe-by-default，反向纠偏比要求"运维记得配"更可靠）。
	envName := os.Getenv("CAT_ENV")
	if envName == "" {
		envName = "prod"
	}
	deps := bootstrap.Deps{
		GormDB:       gormDB,
		TxMgr:        txMgr,
		Signer:       signer,
		RateLimitCfg: cfg.RateLimit,
		StepsCfg:     cfg.Steps,   // Story 7.3 加：步数同步防作弊阈值
		EnvName:      envName,     // Story 7.3 review r6 [P2] 加：prod cap override 强制
		RedisClient:  redisClient, // Story 10.2 加：Redis 单例 client
		SessionMgr:   sessionMgr,  // Story 10.3 加：WS Session 注册中心
		WSCfg:        cfg.WS,      // Story 10.3 加：WS 配置（心跳超时 / max message size / write timeout）
	}
	if err := bootstrap.Run(ctx, cfg, deps); err != nil {
		slog.Error("server run failed", slog.Any("error", err))
		os.Exit(1)
	}
}

// parseTopLevelArgs 把 os.Args[1:] 拆分成 (configPath, migrateArgs, isMigrate)。
//
// 支持以下两种文档化调用形态：
//
//  1. `catserver -config X` / `catserver` —— 普通 server 启动；isMigrate=false
//  2. `catserver -config X migrate <action> [-config Y]` ——
//     migrate 子命令；isMigrate=true，migrateArgs 是 "migrate" 之后的全部参数
//     （含可能再次出现的 -config，由 cli.RunMigrate 内部 NewFlagSet 解析）
//
// 实装策略：手动扫描 args 找到 "migrate" 的位置（不走 flag.Parse —— 它在第一个
// 非 flag 参数停止）。"migrate" 之前的参数走主 flag.Parse 解析 -config；之后的
// 参数原样转发给 cli.RunMigrate。
//
// 如果 "migrate" 之前 + 之后都给了 -config，**子命令的优先**（更靠近 action 的语义胜出）；
// 这与"被显式指定的更晚的值覆盖默认"的常规 CLI 直觉一致。
func parseTopLevelArgs(rawArgs []string) (configPath string, migrateArgs []string, isMigrate bool) {
	// 找 "migrate" 在 rawArgs 里的位置。它必须是一个独立的 token（不是 -migrate 之类）。
	migrateIdx := -1
	for i, a := range rawArgs {
		if a == "migrate" {
			migrateIdx = i
			break
		}
	}

	var preMigrate []string
	if migrateIdx == -1 {
		preMigrate = rawArgs
	} else {
		preMigrate = rawArgs[:migrateIdx]
		// migrateArgs 是 "migrate" **之后**的全部参数，cli.RunMigrate 自己拆 action + flags
		migrateArgs = rawArgs[migrateIdx+1:]
		isMigrate = true
	}

	// 用 ContinueOnError 避免 preMigrate 解析失败直接 os.Exit —— 主流程仍然 fail-fast，
	// 但出错路径走 main 的 slog.Error + os.Exit（统一退出）。
	fs := flag.NewFlagSet("catserver", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "path to config YAML (default: auto-detect server/configs/local.yaml or configs/local.yaml)")
	if err := fs.Parse(preMigrate); err != nil {
		slog.Error("flag parse failed", slog.Any("error", err))
		os.Exit(2)
	}
	return configPath, migrateArgs, isMigrate
}
