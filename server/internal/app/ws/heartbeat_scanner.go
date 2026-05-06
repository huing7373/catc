package ws

import (
	"context"
	"log/slog"
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
//     cfg.WS.HeartbeatTimeoutSec, slog.Default())
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
type HeartbeatScanner struct {
	mgr      SessionManager
	cfg      heartbeatScannerConfig
	logger   *slog.Logger
	interval time.Duration
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
//
// timeoutSec ≤ 0 → 走默认 60s（防御性兜底；正常路径 cfg.WS.HeartbeatTimeoutSec
// 已经在 loader 内兜底过 60，但本构造函数也兜底一次让 scanner 在 testing 场景
// 不必依赖 loader）。
//
// **不**接受 interval 参数（30s SLO 契约不开放调整）；单测路径用 unexported
// helper newHeartbeatScannerForTest 注入小 interval（如 200ms）让单测加速。
func NewHeartbeatScanner(mgr SessionManager, timeoutSec int, logger *slog.Logger) *HeartbeatScanner {
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
	}
}

// newHeartbeatScannerForTest 是单测 / 集成测试用的同包 helper（unexported）。
//
// 让 interval 从 30s 缩到自定义值（如 200ms），让 ticker 路径在测试中能在
// 几秒内触发多次扫描；timeoutMs 也接受任意小值（如 50ms）让"超时检测"测试
// 不必 sleep 60s。
//
// **禁止**在生产路径调用：production 路径必须用 NewHeartbeatScanner（30s SLO
// 契约不可破防）。
func newHeartbeatScannerForTest(mgr SessionManager, timeoutMs int64, interval time.Duration, logger *slog.Logger) *HeartbeatScanner {
	if logger == nil {
		logger = slog.Default()
	}
	return &HeartbeatScanner{
		mgr:      mgr,
		cfg:      heartbeatScannerConfig{timeoutMs: timeoutMs},
		logger:   logger.With(slog.String("component", "ws-heartbeat")),
		interval: interval,
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
func (s *HeartbeatScanner) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
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
//  3. idle > timeoutMs → 并发 go { 重新校验 idle > timeoutMs（review r1 P1
//     TOCTOU 修） → s.CloseWithCode(4005, "heartbeat timeout") }
//     （并发避免某一 Session 写 close frame 慢阻塞其他 Session 检测）
//  4. log info（V1 §12.1 钦定 4005 写 log info，因为心跳超时是常态网络抖动 /
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
			continue
		}
		// 启动 fire-and-forget goroutine 写 close frame + 释放本地资源。
		// 不等待 — 单 Session close frame 写超时 500ms，1000 个 Session 同时超时
		// 串行写会卡 500s；fanout 后整个 scanOnce 不超过 500ms。
		// **不**捕获 ctx：CloseWithCode 内部用 conn.WriteControl + deadline，不
		// 走 ctx-aware 路径；scanner.Run 退出后的尾部 fanout 也能跑完（fire-and-forget
		// 语义本身允许，Close 幂等）。
		go func(target *Session) {
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
