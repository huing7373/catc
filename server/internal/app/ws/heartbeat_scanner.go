package ws

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// heartbeatScanIntervalSec 是 HeartbeatScanner 的扫描周期（秒）。
//
// 选 30s 的理由：
//   - V1 §12.2 钦定 client 应每 30 秒发一次 ping；server-side 60 秒未收到任何
//     消息触发 4005 close
//   - server-side 扫描周期取 30s = 半个阈值 —— 让"刚好达到 60s 阈值的 Session"
//     在最多 30s 内被检测到，总检测延迟最大 60+30 = 90s
//   - 不取 5s（频率过高，扫描 goroutine CPU 浪费）；不取 60s（与阈值同周期，
//     最坏检测延迟 60+60 = 120s 接近 2 倍阈值 → 客户端体感"server 反应慢"）
//
// 不走 YAML：30s 是契约一部分（与 60s 阈值组合产生最大 90s 检测延迟），prod
// 不可覆盖；与 cfg.WS.HeartbeatTimeoutSec 不同 —— HeartbeatTimeoutSec 是协议
// 钦定的 client-side 阈值（dev/test 可覆盖让单测加速），ScanInterval 是 server
// 内部 tick 频率（与协议字段无关，但与 60s 阈值组合产生检测延迟 SLO，不开放
// 用户调整避免 SLO 漂移）。
const heartbeatScanIntervalSec = 30

// HeartbeatScanner 是后台心跳超时扫描器（Story 10.4 引入）。
//
// 用途：周期性遍历 SessionManager 内所有 active Session，对超过 cfg.WS.
// HeartbeatTimeoutSec * 1000 ms 未活跃的 Session 触发 CloseWithCode(4005,
// "heartbeat timeout")，自动释放 leaked Session 资源 + 通知 client 走 iOS
// Story 12.5 自动重连路径。
//
// 生命周期：
//  1. main.go bootstrap wire：scanner := NewHeartbeatScanner(mgr,
//     cfg.WS.HeartbeatTimeoutSec, slog.Default(), presenceRepo)
//  2. ctx, cancel := context.WithCancel(context.Background())
//  3. go scanner.Run(ctx)
//  4. defer cancel()（让 scanner.Run 在 ctx.Done 时优雅退出）
//
// 并发安全：
//   - Run 是单 goroutine 主循环（不 fanout）—— SessionManager.ListAllSessions
//     已经做了 read-lock copy 切片，scanner 拿到切片后并发 CloseWithCode 安全
//   - Stop 通过 ctx 取消语义间接触发（main.go cancel ctx）；本身没有 Stop()
//     方法 —— 与 *time.Ticker 风格 Stop 不同，因为 scanner 没有需要外部释放的
//     资源（ticker 在 Run 内 defer Stop）
//
// 关键不变量：
//   - 30s 周期是 SLO 契约（与 60s 阈值组合产生最大 90s 检测延迟）
//   - log level info（V1 §12.1 钦定 4005 = 常态网络抖动，写 warn 会让正常
//     网络抖动场景下日志噪声暴涨）
//   - close 路径走 Session.CloseWithCode → Session.Close → notifier.notifyClosed
//     → manager.Unregister → onUnregister 钩子（review 10-3 r2 P1 不变量保留）
//   - **fanout goroutine 必须 ctx-aware**（review r3 P2）：scanner.Run 退出后已
//     dispatched 的 per-session goroutine 不能再 emit 4005，否则 shutdown 期间
//     正常下线的 client 会错误地收到 "heartbeat timeout" 触发自动重连
//   - **Run 退出前必须 drain fanout goroutines**（review r5 P2）：仅 ctx-check
//     不够 —— "已通过最后一次 ctx.Done() check + 即将调 CloseWithCode" 的 goroutine
//     在 ctx cancel 后仍会 emit 4005。用 sync.WaitGroup 跟踪 in-flight fanout，
//     Run 退出前 wg.Wait() 阻塞到所有 goroutine 跑完。两道防线（fanout 入口
//     ctx-check 让 ctx-cancelled 路径立即 return + Run defer wg.Wait() 让残余
//     goroutine 全跑完）共同保证 ctx cancel 后没有 stale fanout 在跑。
//   - **每 tick reconcile presence**（review 10-6 r2 P1 / r3 P2）：long-lived WS session
//     超过 redis.presence_ttl_sec（默认 300s）后 presence keys 会过期 ——
//     IsOnline / ListOnline 误报 user offline，但 manager 还认为 session active。
//     scanOnce 拿到 ListAllSessions 后对每个 active（idle <= timeoutMs）session
//     同步调 PresenceRenewer.AddOnline 重写 + 续期 —— 与 client 实际 ping 频率解耦（即使
//     client ping 慢一点，scanner 30s tick 仍写，远小于 TTL 300s）。**仅对 active
//     session reconcile**，idle > timeoutMs 的 session 已经走 fanout close 路径，没必要
//     续（close 完成后 onUnregister 钩子会 RemoveOnline 清干净）。详见 lesson
//     2026-05-07-presence-ttl-renewal-via-heartbeat-scanner-10-6-r2.md。
//   - **走 AddOnline 而非纯 RenewTTL**（review 10-6 r3 P2）：Register hook 调
//     AddOnline 失败仅 log warn 仍接受 session；如果只调 RenewTTL（EXPIRE）扫描
//     路径无法重建缺失的 room set 成员 → IsOnline/ListOnline 整个 session 生命周期
//     内误报 offline。AddOnline 是 idempotent（SET 覆盖、SADD 已存在 no-op、EXPIRE
//     总是续）—— 每 30s 全量重写 presence 让 partial-fail 路径自然 self-heal。
//     IO 比 RenewTTL 略多（每 session 3 次 Redis command vs 2 次）但 SLO 内可接受。
//     详见 lesson 2026-05-07-presence-add-online-self-heal-via-scanner-10-6-r3.md。
type HeartbeatScanner struct {
	mgr      SessionManager
	cfg      heartbeatScannerConfig
	logger   *slog.Logger
	interval time.Duration

	// renewer 是 presence reconcile 接口（review 10-6 r2 P1 / r3 P2）。**可空** ——
	// production 路径 main.go wire 时传 PresenceRepo；单测路径默认 nil 跳过 reconcile
	// 不影响心跳超时检测语义。窄化接口（仅 AddOnline 一个方法；r3 P2 从 RenewTTL
	// 改成 AddOnline 让 scanner 路径 self-heal Register hook partial-fail）让 scanner
	// 不直接 import repo/redis 包，保持 ws → repo/redis 单向依赖最小化（gateway.go
	// 已 import repo/mysql；加 repo/redis 只是同模式扩展，无新跨层依赖问题）。
	renewer PresenceRenewer

	// wg 跟踪 scanOnce 已 dispatch 的 in-flight fanout goroutines（review r5 P2）。
	// Run defer wg.Wait() 让 ctx cancel 后 Run 仍阻塞到所有 fanout 跑完才返回 ——
	// 配合 fanout 入口 ctx-check 让 ctx-cancelled 路径立即 return，shutdown 期间
	// 不再有 stale 4005 emit。
	wg sync.WaitGroup
}

// PresenceRenewer 是 HeartbeatScanner reconcile presence 用的窄化接口（review
// 10-6 r2 P1 / r3 P2）。
//
// 接口边界：仅含 AddOnline 一个方法 —— scanner 不需要 RemoveOnline / IsOnline /
// ListOnline / RenewTTL；让单测可注入 stub 不必拉 miniredis；production 路径
// PresenceRepo 接口超集自动满足本接口。
//
// **r3 P2 决策**：从 RenewTTL 改成 AddOnline。原因：Register hook 调 AddOnline
// 失败仅 log warn 仍接受 session；后续 scanner 路径若只跑 RenewTTL（EXPIRE 双
// key）不会重建缺失的 room set 成员（room:{id}:online_users 没 SADD 过就不会有
// user-string member 让 EXPIRE 续命）。改调 AddOnline 走 SET → SADD → EXPIRE
// 三命令 idempotent 重写 → partial-fail 场景下 30s 内自愈。
//
// 失败语义：AddOnline 返 error → scanner log warn 继续遍历下一 session（一次失败
// 不影响其他 session reconcile，与 close fanout 错误隔离同模式）；下一 tick 30s 后
// 重试，TTL 5min 远 > 30s × 几次重试，足以容忍偶发 Redis 抖动。
type PresenceRenewer interface {
	AddOnline(ctx context.Context, roomID, userID uint64, sessionID string) error
}

// heartbeatScannerConfig 是 HeartbeatScanner 接受的最小配置面（让单测可注入
// 自定义阈值不依赖完整 config.WSConfig 全字段）。
type heartbeatScannerConfig struct {
	timeoutMs int64 // = cfg.WS.HeartbeatTimeoutSec * 1000
}

// NewHeartbeatScanner 构造 scanner（Story 10.4 加）。
//
// 参数：
//   - mgr: 既有 SessionManager 单例（main.go 已构造）
//   - timeoutSec: 心跳超时阈值（秒）；通常传 cfg.WS.HeartbeatTimeoutSec
//   - logger: base logger（传 slog.Default() 即可；scanner 内部 With 加
//     component=ws-heartbeat）
//   - renewer: presence reconcile 接口（review 10-6 r2 P1 加 / r3 P2 改：方法
//     从 RenewTTL 换成 AddOnline 让 scanner 路径自愈 Register hook partial-fail；
//     可空 —— 单测 / 没接 Redis 的最小路径传 nil 跳过 reconcile，不影响心跳超时
//     检测语义）；production 路径传 PresenceRepo 单例（PresenceRepo 接口含
//     AddOnline 方法满足 PresenceRenewer 接口约束）
//
// timeoutSec ≤ 0 → 走默认 60s（防御性兜底；正常路径 cfg.WS.HeartbeatTimeoutSec
// 已经在 loader 内兜底过 60，但本构造函数也兜底一次让 scanner 在 testing 场景
// 不必依赖 loader）。
//
// **不**接受 interval 参数（30s SLO 契约不开放调整）；单测路径用 unexported
// helper newHeartbeatScannerForTest 注入小 interval（如 200ms）让单测加速。
func NewHeartbeatScanner(mgr SessionManager, timeoutSec int, logger *slog.Logger, renewer PresenceRenewer) *HeartbeatScanner {
	if timeoutSec <= 0 {
		timeoutSec = 60
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &HeartbeatScanner{
		mgr:      mgr,
		cfg:      heartbeatScannerConfig{timeoutMs: int64(timeoutSec) * 1000},
		logger:   logger.With(slog.String("component", "ws-heartbeat")),
		interval: heartbeatScanIntervalSec * time.Second,
		renewer:  renewer,
	}
}

// newHeartbeatScannerForTest 是单测 / 集成测试用的同包 helper（unexported）。
//
// 让 interval 从 30s 缩到自定义值（如 200ms），让 ticker 路径在测试中能在
// 几秒内触发多次扫描；timeoutMs 也接受任意小值（如 50ms）让"超时检测"测试
// 不必 sleep 60s。
//
// renewer 可空（与 NewHeartbeatScanner 同语义）—— 既有 10.4 测试不传 renewer
// 仍正常工作；review 10-6 r2 P1 续期路径走 newHeartbeatScannerForTestWithRenewer。
//
// **禁止**在生产路径调用：production 路径必须用 NewHeartbeatScanner（30s SLO
// 契约不可破防）。
func newHeartbeatScannerForTest(mgr SessionManager, timeoutMs int64, interval time.Duration, logger *slog.Logger) *HeartbeatScanner {
	return newHeartbeatScannerForTestWithRenewer(mgr, timeoutMs, interval, logger, nil)
}

// newHeartbeatScannerForTestWithRenewer 是 newHeartbeatScannerForTest 的扩展版
// （review 10-6 r2 P1 加），让单测可注入 PresenceRenewer stub 验证续期路径。
func newHeartbeatScannerForTestWithRenewer(mgr SessionManager, timeoutMs int64, interval time.Duration, logger *slog.Logger, renewer PresenceRenewer) *HeartbeatScanner {
	if logger == nil {
		logger = slog.Default()
	}
	return &HeartbeatScanner{
		mgr:      mgr,
		cfg:      heartbeatScannerConfig{timeoutMs: timeoutMs},
		logger:   logger.With(slog.String("component", "ws-heartbeat")),
		interval: interval,
		renewer:  renewer,
	}
}

// Run 启动扫描主循环；ctx.Done 时优雅退出（Story 10.4 加）。
//
// 主流程：
//  1. ticker := time.NewTicker(s.interval)  // 30s（生产路径）
//  2. defer ticker.Stop()
//  3. for {
//       select {
//       case <-ctx.Done(): return
//       case now := <-ticker.C:
//           s.scanOnce(ctx, now)
//       }
//     }
//
// **关键约束**：
//   - 用 time.NewTicker 而非 time.AfterFunc：ticker 的 panic-safe 语义更强
//     （即使 scanOnce 抛 panic，ticker 自身仍在跑；下一 tick 仍触发）
//   - **不**在 ctx.Done 后再做 cleanup（除 ticker.Stop）：scanner 没有持久状态
//   - **不**在每 tick 启动新 goroutine：scanOnce 内部并发 CloseWithCode 即可，
//     单 ticker 串行触发避免 goroutine 数量随时间无限增长
//   - **defer wg.Wait()**（review r5 P2）：Run 退出前阻塞到所有 in-flight fanout
//     goroutine 跑完。fanout 入口 ctx-check 会让 ctx-cancelled 路径立即 return；
//     已经过 ctx-check 的 goroutine 会跑到 CloseWithCode 完成 —— 两种路径都收敛
//     后 wg.Wait() 解除阻塞，Run 才返回。这样保证 Run 返回 = scanner 全部安静，
//     与 main.go shutdown helper（review r6 P1：cancelHeartbeat → wait scannerDone →
//     sessionMgr.Close 串行 deferred 函数）的钦定时序对齐。仅靠 defer LIFO 不够 ——
//     LIFO 只保证 deferred 函数注册顺序，不保证 cancelHeartbeat 后 scanner 实际
//     已退出，必须 chan signal 显式 wait。
func (s *HeartbeatScanner) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	// review r5 P2：退出前等所有 fanout goroutine 跑完才返回，避免 ctx cancel 后
	// 已 dispatched 但已通过 ctx-check 的 goroutine 仍在 race 4005 emit。
	defer s.wg.Wait()
	s.logger.Info("ws heartbeat scanner started",
		slog.Int64("timeoutMs", s.cfg.timeoutMs),
		slog.Duration("interval", s.interval),
	)
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("ws heartbeat scanner stopped", slog.Any("ctx", ctx.Err()))
			return
		case now := <-ticker.C:
			s.scanOnce(ctx, now)
		}
	}
}

// scanOnce 是单次扫描逻辑（提取出来方便单测直接调用，绕过 ticker）。
//
// 流程：
//  1. mgr.ListAllSessions(ctx) → 拿当前所有 active Session 切片
//  2. 对每个 Session 计算 idle = now.UnixMilli() - s.LastHeartbeatAt()
//  3. idle <= timeoutMs（active）→ renewer != nil 时同步调
//     PresenceRenewer.AddOnline（roomID, userID, sessionID）重写 + 续期 presence
//     keys（review 10-6 r2 P1 加；r3 P2 把方法从 RenewTTL 改成 AddOnline 让
//     Register hook partial-fail 路径在 30s 内自愈）
//  4. idle > timeoutMs → 并发 go { ctx-check → 重新校验 idle > timeoutMs（review
//     r1 P1 TOCTOU 修） → ctx-check → s.CloseWithCode(4005, "heartbeat timeout") }
//     （并发避免某一 Session 写 close frame 慢阻塞其他 Session 检测）
//  5. log info（V1 §12.1 钦定 4005 写 log info，因为心跳超时是常态网络抖动 /
//     切后台）
//
// **关键约束**：
//   - 用 goroutine fanout 触发 CloseWithCode：单 Session 的 close frame 写超时
//     是 500ms（closeFrameWriteDeadline），如果 1000 个 Session 同时超时 + 串行
//     写 = 500s 才能跑完一轮扫描，明显违反 SLO。fanout 后所有 close frame 在
//     500ms 窗口内并发写完
//   - **不**等待所有 goroutine 退出（用 fire-and-forget）：scanOnce 单次扫描
//     不需要"原子完成"语义；Close 自身幂等，下一轮扫描如果同 Session 仍在 list
//     里且仍 idle 就再触发一次（Close 走 sync.Once 兜底，第二次是 no-op）
//   - log level **必须**是 info（V1 §12.1 钦定 4005 = 常态网络抖动）
//   - reason 字符串字面量必须是 "heartbeat timeout"（V1 §12.1 钦定）
//   - **TOCTOU 防护**（review r1 P1）：goroutine 内**必须重新读** LastHeartbeatAt
//     再计算 idle 决定是否 close。原因：scanOnce 主循环读到的 idle 是"判定瞬间"
//     值；进入 fanout goroutine 后，readLoop 仍可能刚收到 client 的 ping 刷新
//     lastHeartbeatAt（典型场景：client 在阈值边界附近发 ping，server 端 readLoop
//     已经处理完更新了 atomic，但 scanner 主循环已经读到旧值）。如果不重新校验，
//     会把刚刚发了心跳的健康连接误踢，触发 client 不必要的 reconnect。
//   - **shutdown race 防护**（review r3 P2）：fanout goroutine 必须监听 ctx —— 入口
//     check + recheck 后 close 之前再 check。原因：scanner.Run 在 ctx.Done 后主
//     循环立即退出，但已经 dispatch 的 per-session goroutines 仍然在跑。如果
//     SIGTERM 落在 tick 与 fanout 之间，goroutine 会在 cancelHeartbeat 之后继续
//     调 CloseWithCode 写 4005 close frame，与 main.go shutdown helper 钦定的
//     "scanner 先停 → wait scannerDone → sessionMgr.Close 走标准 close 路径"流程 race —— 用户
//     SIGTERM 期间正常下线却收到 4005 错误地触发自动重连。修法：在 fanout
//     goroutine 入口 select ctx.Done → return，绕过 close path；recheck 之后再
//     check 一次防 sleep 在 nanos 间被 cancel。
//   - **shutdown drain 兜底**（review r5 P2）：仅 ctx-check 不够 —— 已通过最后
//     一次 ctx-check 的 goroutine 会继续跑到 CloseWithCode 完成。本函数把每个
//     fanout goroutine 注册到 s.wg，Run 退出前 defer s.wg.Wait() 阻塞到所有
//     goroutine 真正跑完（无论是 ctx-aware return 还是真的执行完 CloseWithCode）。
//     wg.Add 在 dispatch 前主线程同步调用，wg.Done 在 goroutine defer 内 —— 标准
//     "Add before go, Done in defer" 模式不会触发 wg 自身的 race。
func (s *HeartbeatScanner) scanOnce(ctx context.Context, now time.Time) {
	sessions := s.mgr.ListAllSessions(ctx)
	if len(sessions) == 0 {
		return
	}
	nowMs := now.UnixMilli()
	timeoutMs := s.cfg.timeoutMs
	for _, sess := range sessions {
		if sess == nil {
			continue
		}
		idle := nowMs - sess.LastHeartbeatAt()
		if idle <= timeoutMs {
			// review 10-6 r2 P1 / r3 P2：active session 调 AddOnline reconcile
			// presence —— scanner 30s tick 远小于 TTL 5min，让 long-lived session
			// 不被 Redis 自动过期误报为 offline；同时让 Register hook AddOnline
			// 失败（partial fail）的 session 在 30s 内通过 scanner 路径自愈
			// 重建缺失的 room set 成员（纯 RenewTTL 路径只 EXPIRE 双 key，不会
			// 重建 SADD member）。AddOnline 是 idempotent（SET nx=false 覆盖、
			// SADD 已存在 no-op、EXPIRE 总是续）—— 重复调安全。renewer == nil
			// （未接 Redis 的最小路径 / 单测）跳过。
			//
			// **review 10-6 r4 P2**：AddOnline 之前必须 IsRegistered guard。
			// snapshot 与 AddOnline 之间窗口期里 session 可能已 disconnect —— manager
			// 已 Unregister + onUnregister 钩子已 RemoveOnline 清干净 presence；
			// 此时 scanner 仍 AddOnline 会"复活" presence keys 让已离线 session
			// 看似 online 直到 TTL 过期（zombie online entry，5min 视野污染）。
			// IsRegistered 走 RLock map lookup（O(1)，nanos 量级），开销可忽略 ——
			// 比 zombie 风险换性能成本对 SLO 更友好。
			if s.renewer != nil {
				if !s.mgr.IsRegistered(ctx, sess.SessionID()) {
					// snapshot 后 session 已 unregister，跳过 reconcile 避免复活
					// 已离线 session 的 presence。下一 tick 不会再看到它（已从
					// sessionsByID 移除），AddOnline 不会被错误重写。
					continue
				}
				if err := s.renewer.AddOnline(ctx, sess.RoomID(), sess.UserID(), sess.SessionID()); err != nil {
					// 一次失败不阻塞遍历下一 session；下一 tick 30s 后重试，TTL
					// 5min 远 > 30s × 几次重试，足以容忍偶发 Redis 抖动。log warn
					// 让运维侧能看到累计失败信号。
					s.logger.Warn("ws presence reconcile failed",
						slog.String("sessionID", sess.SessionID()),
						slog.Uint64("userID", sess.UserID()),
						slog.Uint64("roomID", sess.RoomID()),
						slog.Any("error", err),
					)
				}
			}
			continue
		}
		// 启动 fire-and-forget goroutine 写 close frame + 释放本地资源。
		// 不等待 — 单 Session close frame 写超时 500ms，1000 个 Session 同时超时
		// 串行写会卡 500s；fanout 后整个 scanOnce 不超过 500ms。
		// **review r3 P2**：必须捕获 ctx，让 shutdown 期间已 dispatched 的 goroutine
		// 不再 emit 4005（避免与 sessionMgr.Close 标准路径 race）。
		// **review r5 P2**：wg.Add(1) 在 dispatch 前同步调用，goroutine defer wg.Done()。
		// Run defer s.wg.Wait() 退出前 drain，让 ctx cancel 后已 dispatched 的
		// goroutine 全跑完才让 Run 返回。
		s.wg.Add(1)
		go func(target *Session) {
			defer s.wg.Done()
			// review r3 P2 入口 ctx check：scanner.Run 退出（cancelHeartbeat()
			// 在 main.go defer 内被调）后 ctx 已 Done，本 goroutine 应放弃 close
			// path，让 sessionMgr.Close 走标准 close 流程而不是 4005。
			select {
			case <-ctx.Done():
				return
			default:
			}
			// review r1 P1 TOCTOU 防护：在执行 CloseWithCode 之前**重新**读
			// LastHeartbeatAt。主循环读时刻与本 goroutine 实际执行 close 之间，
			// readLoop 仍可能收到 client 心跳并刷新 lastHeartbeatAt —— 那种情况下
			// 应跳过 close（避免误踢健康连接）。
			recheckNowMs := time.Now().UnixMilli()
			idleMs := recheckNowMs - target.LastHeartbeatAt()
			if idleMs <= timeoutMs {
				// race 窗口期内 client 已刷新心跳；本次 fanout 跳过，下一轮再判
				return
			}
			// review r3 P2 二次 ctx check：recheck 与 CloseWithCode 之间还有
			// goroutine 调度间隙，shutdown 信号可能在此期间到达。再 check 一次
			// 让 close-vs-shutdown race 窗口尽可能小。
			select {
			case <-ctx.Done():
				return
			default:
			}
			s.logger.Info("ws session heartbeat timeout",
				slog.String("sessionID", target.SessionID()),
				slog.Uint64("userID", target.UserID()),
				slog.Uint64("roomID", target.RoomID()),
				slog.Int64("idleMs", idleMs),
				slog.Int64("timeoutMs", timeoutMs),
			)
			if err := target.CloseWithCode(4005, "heartbeat timeout"); err != nil {
				// ErrSessionClosed 是预期路径（同一 Session 上 readLoop 已先自闭 /
				// 上一 tick 的 fanout 已 close），不算异常。其他 error 当前 Close
				// 永远返 nil，CloseWithCode 仅可能返 ErrSessionClosed —— 防御性
				// 走 info 不 escalate。
				s.logger.Info("ws CloseWithCode skipped",
					slog.String("sessionID", target.SessionID()),
					slog.Any("reason", err),
				)
			}
		}(sess)
	}
}
