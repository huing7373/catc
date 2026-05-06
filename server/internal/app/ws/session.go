// Package ws 提供 server 端 WebSocket 网关骨架（Story 10.3 引入）。
//
// 三对象：
//   - Session（session.go）：单 user 的单 WS 连接 + 读 / 写 goroutine 主循环
//   - SessionManager（session_manager.go）：进程内全局 Session 注册中心 +
//     lifecycle 钩子（10.6 Redis presence 挂这里）
//   - Gateway（gateway.go）：HTTP → WS Upgrade handler + V1 §12.1 校验顺序 +
//     同步段 placeholder snapshot 写入
//
// 边界：
//   - 不导出 *websocket.Conn（外部走 Session.Send / Close 接口）
//   - 不在本 story 实装 心跳超时 / BroadcastToRoom / Redis presence /
//     SnapshotBuilder（10.4 / 10.5 / 10.6 / 10.7 各自接管）
//   - 不挂 RateLimit / Auth HTTP 中间件（V1 §12.1 钦定 WS 不走 HTTP rate_limit；
//     鉴权在 Gateway.Handle 内部按 §12.1 校验顺序实装，失败发 close frame 而非
//     HTTP 401）
//
// 详见 ADR-0011（gorilla/websocket 选型） + V1 §12.1 / §12.2 / §12.3。
package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// sendChanCapacity 是 Session.sendChan 容量。
//
// 选 32 的理由（详见 AC3 sendChan 容量决策）：
//   - 0（unbuffered）：BroadcastToRoom（10.5）对一个 room 1000 个 Session 串行
//     Send 会变成线性时间，违反"广播是 fast path"
//   - 1024（过大）：内存浪费 + 客户端慢 / 阻塞时积压消息让用户感受到的是大段
//     stale 消息批量到达
//   - 32：足够吸收节点 4 阶段（最多 4 user / room）的瞬时 burst（如 4 个 ping 同时
//     到达 server，server 同时回 4 个 pong；同房间 broadcast 4 条消息），同时
//     保持内存占用合理（每 Session 32 * pointer-sized = ~256B）
const sendChanCapacity = 32

// closeFrameWriteDeadline 是 Session.CloseWithCode **WriteControl close frame**
// 步骤的 deadline（Story 10.4 引入）。
//
// 与 gateway.go 包级 const `closeWriteDeadline` 同 semantics（500ms），但因为不同
// 包内 const 不能直接复用，复制一份并显式标注：未来有人重构合并时一并改两处即可。
//
// 选 500ms 的理由：写 control frame 是单 packet 操作；超时通常意味着对端已掉线，
// 此时 close frame 已无意义 —— 与其等到 5s+ 才放手，不如快速 Close 让上层流程推进。
// lesson 2026-04-26-startup-blocking-io-needs-deadline 钦定 IO 必须有本地 timeout。
//
// **不复用为 wait writeLoopDone 的上限**（review r3 P2 修）：原版用同一个 500ms
// 既做 wait 上限又做 WriteControl deadline 是 bug —— writeLoop 内部
// conn.WriteMessage 的 deadline 是 writeTimeout（默认 5s），意味着慢/卡住的 client
// 会让 writeLoop 在最坏情况下阻塞到 writeTimeout 才退出。500ms < 5s 让 wait 提前
// 超时 → closeInternal 误以为 writeLoop 已退出 → 走 WriteControl 写 close frame
// → 之后 writeLoop 才真正结束 conn.WriteMessage 写出 data frame，造成 V1 §12.1
// 钦定的"close frame 必须是 connection 最后一个 frame"反过来。修法：用单独的
// closeWaitTimeout = writeTimeout + 200ms 做 wait 上限（详见 Session.closeWaitTimeout
// 字段注释）。
const closeFrameWriteDeadline = 500 * time.Millisecond

// closeWaitBufferDuration 是 closeInternal 等 writeLoop 退出的额外缓冲时间
// （review r3 P2 加）。closeWaitTimeout = writeTimeout + closeWaitBufferDuration。
//
// 选 200ms 的理由：writeLoop 内 conn.WriteMessage 命中 writeTimeout 触发 deadline
// 错误后 → writeFrame 返 error → writeLoop break + close(writeLoopDone)。这条路径
// 是纯 in-process 操作（无 IO），200ms buffer 足够 cover Windows / CI 时序抖动 +
// goroutine 调度延迟，**不**至于让正常 cleanup 也付不必要的 200ms tax —— 正常
// cleanup 走 sendChan close → writeLoop select 立即命中关闭分支退出，远在 buffer
// 用完之前就解锁 wait。
const closeWaitBufferDuration = 200 * time.Millisecond

// sendPriorityChanCapacity 是 Session.sendPriorityChan 容量（review r4 P2 加）。
//
// 用途：让 protocol-level 消息（pong / 内部 close error）走独立 buffer，**不**
// 与业务消息（snapshot / broadcast / emoji）共享 sendChan。即使业务 buffer 满
// （慢 client 导致 sendChan 32 容量被填爆），心跳 pong 仍能在 priority buffer
// 投递并被 writeLoop 优先消费 —— 客户端不会因 server-side backpressure 收不到
// 心跳回应而误判 connection dead → reconnect 重试风暴。
//
// 选 4 的理由：
//   - 节点 4 阶段单 Session 的 protocol msg 频率上限：60s 一次 ping → pong；
//     最坏情况下连续 2 次 ping race 进入 readLoop（client 偷跑 / 重发）→ 容量 4
//     有 2x 缓冲
//   - 不需要更大：writeLoop 总会消费 priority；只要 writeLoop 存活就不会卡满
//   - 不需要更小：unbuffered 会让 readLoop 在 priority send 时阻塞等 writeLoop
//     消费，劣化为同步路径
const sendPriorityChanCapacity = 4

// Sentinel errors（暴露给调用方，让 errors.Is 判定）。
var (
	// ErrSessionClosed: 已 Close 的 Session 调 Send → 返此错误（fire-and-forget
	// 保护，防止 caller 在循环里盲发到死 Session）。
	ErrSessionClosed = errors.New("ws: session closed")

	// ErrSessionSendBufferFull: sendChan 满 → fire-and-forget 丢弃此消息，调用方
	// 收到此错误可以选择重试 / 跳过 / 关 Session（取决于消息语义）。
	ErrSessionSendBufferFull = errors.New("ws: session send buffer full")

	// ErrSessionSendPriorityBufferFull: sendPriorityChan 满 → priority msg 入队
	// 失败。理论不会发生（容量 4，writeLoop 优先消费）；返回此 sentinel 让调用方
	// 在异常路径上识别区别于 ErrSessionSendBufferFull（review r4 P2 加）。
	ErrSessionSendPriorityBufferFull = errors.New("ws: session priority send buffer full")

	// ErrSessionReplaced: 同 user 重复 Register → 旧 Session 被强制 Close，
	// SessionManager.Register 在替换路径上返此错误（让上层日志区分"主动 Close"
	// vs "被替换 Close"）。
	ErrSessionReplaced = errors.New("ws: session replaced by new connection")
)

// clientEnvelope 是 V1 §12.2 客户端 → 服务端通用消息信封。
//
// 字段：
//   - Type: 消息类型（"ping" / "emoji.send" / ...）；本 story 阶段仅 "ping" 走
//     pong 回复路径，其他 type "安全忽略 + log warn"
//   - RequestID: client 生成的请求 ID；pong 回带（V1 §12.2 ping 字段表）
//   - Payload: 业务负载；ping 阶段为空 object {}
type clientEnvelope struct {
	Type      string          `json:"type"`
	RequestID string          `json:"requestId"`
	Payload   json.RawMessage `json:"payload"`
}

// serverEnvelope 是 V1 §12.3 服务端 → 客户端通用消息信封。
//
// 字段：
//   - Type: "room.snapshot" / "pong" / "error" / ...
//   - RequestID: 响应类回带 client 请求的 RequestID；广播 / 主动推送类固定 ""
//   - Payload: 业务负载；空 object 必须显式 {}（不省略 key）
//   - Ts: 服务端发送时的 unix ms epoch（V1 §12.3 通用信封钦定）
type serverEnvelope struct {
	Type      string `json:"type"`
	RequestID string `json:"requestId"`
	Payload   any    `json:"payload"`
	Ts        int64  `json:"ts"`
}

// closeNotifier 是 SessionManager 注入给 Session 的 close 通知钩子，让 Session
// 在 readLoop / writeLoop 收到底层错误自闭时**自动**触发 SessionManager.Unregister。
//
// 不直接传 SessionManager 接口避免循环引用：Session 不需要知道完整 manager
// 接口，只需要 sessionID + close 通知能力。
type closeNotifier interface {
	notifyClosed(sessionID string)
}

// Session 表示单个用户的单个 WS 连接。
//
// 内部状态字段对照 Go 项目结构 §9.1 钦定（userID / roomID / conn /
// sendChan / lastHeartbeatAt 五字段），其中：
//   - lastHeartbeatAt 在本 story（10.3）阶段**仅写入**（每次收到 client 消息
//     更新），但**不读**（10.4 心跳超时扫描才会读这个字段做超时判定）；本 story
//     预留字段是为了让 10.4 接管时不需要修改 Session 结构
//
// Session 生命周期（V1 §12.1 钦定的 5 步握手成功顺序）：
//
//  1. gateway.go upgrade 完成 → newSession 构造（userID / roomID / conn /
//     sendChan / lastHeartbeatAt=now）
//  2. SessionManager.Register(s)（触发 OnSessionRegister 钩子）
//  3. 同步段写 placeholder room.snapshot（**不**走 sendChan，直接 conn.WriteMessage）
//  4. go s.readLoop() 启动读 goroutine
//  5. go s.writeLoop() 启动写 goroutine（消费 sendChan）
//  6. （生命周期内）readLoop 读到 ping → 推 pong 占位 message 到 sendChan
//  7. （正常断开）client close / readLoop 出错 → s.Close() → SessionManager.
//     Unregister(s)（触发 OnSessionUnregister 钩子）→ writeLoop 退出
//
// 并发安全：
//   - sendChan buffered=32；多 goroutine 可并发推消息，writeLoop 单 goroutine
//     消费
//   - lastHeartbeatAt 用 atomic.Int64（unix ms epoch）
//   - closed 用 atomic.Bool**仅供锁外快速读**（如 readLoop 错误日志判断）；Send /
//     Close 路径**必须**走 sendMu（RWMutex）保证 send-on-closed-channel 不发生：
//       Send 拿 RLock → 读 closed flag → select send；
//       Close 拿 Lock → 写 closed flag → close(sendChan)。
//     RWMutex 让多 Send 并发（RLock 可重入），但 Close 与所有 Send 互斥。
//   - closeOnce 包裹整个关闭副作用（cancelCtx + close(sendChan) + conn.Close）
//   - writeLoopDone 是 writeLoop 退出信号（review r1 P2 加）；CloseWithCode
//     需要在 WriteControl close frame 之前**等** writeLoop 退出，确保没有任何
//     data frame 会跟在 close frame 之后写到 wire（V1 §12.1 协议钦定 close
//     frame 是 connection 上的最后一个 frame）
//   - writeLoopStarted 是 writeLoop 是否已启动的 flag（review r2 P2 加）。
//     Gateway.Handle 的 handshake 失败路径会在启动 readLoop/writeLoop **之前**
//     调 session.Close 释放资源；此时 writeLoop 从未运行，writeLoopDone 永远不
//     close → wait 必然落到 closeWaitTimeout。该 flag 让 closeInternal 区分
//     "writeLoop 跑过了，必须等它收尾" vs "writeLoop 没启动，直接跳过 wait"
//   - closeWaitTimeout 是 closeInternal 等 writeLoop 退出的上限（review r3 P2 加）。
//     必须 ≥ writeTimeout + 一些 buffer，否则 writeLoop 可能仍卡在 conn.WriteMessage
//     里没退出 → wait 提前超时 → WriteControl 写出 close frame，之后 writeLoop
//     才结束写完 data frame → wire 上 close frame 后跟 data frame，违反 V1 §12.1
//     "close frame 是 connection 最后一个 frame"。closeFrameWriteDeadline 仍仅
//     用于 WriteControl 自身的 deadline（500ms 写一帧足够）。
//
// 不导出字段：所有字段小写；外部访问通过 Send / Close。
type Session struct {
	sessionID         string
	userID            uint64
	roomID            uint64
	conn              *websocket.Conn
	sendChan          chan []byte
	sendPriorityChan  chan []byte  // review r4 P2 加：protocol-level msg 独立 buffer（pong 等）
	sendMu            sync.RWMutex // 保护 Send / SendPriority 与 Close 互斥（防 send-on-closed-channel panic）
	lastHeartbeatAt   atomic.Int64
	closed            atomic.Bool
	closeOnce         sync.Once
	writeLoopDone     chan struct{} // writeLoop 退出信号（review r1 P2 加：CloseWithCode 在 WriteControl 前等 writeLoop 退出）
	writeLoopStarted  atomic.Bool   // writeLoop 是否已经启动（review r2 P2 加：handshake 失败路径调 Close 时 writeLoop 没启动 → 不需要 wait）
	logger            *slog.Logger
	writeTimeout      time.Duration
	closeWaitTimeout  time.Duration // closeInternal 等 writeLoop 退出的上限（review r3 P2 加）
	notifier          closeNotifier // SessionManager 注入；nil 安全（不通知）
	ctx               context.Context
	cancelCtx         context.CancelFunc
}

// newSession 构造 Session（私有，仅供 SessionManager / Gateway 调用）。
//
// 字段：
//   - sessionID: 构造时**留空**（SessionManager.Register 内部生成 short uuid 后回填，
//     review r4 P3 修：避免 logger 出现"sessionID="""空字段污染日志关联）
//   - userID / roomID: 来自 V1 §12.1 校验通过后的 token claims + 路径参数
//   - conn: gorilla/websocket Upgrade 后的 *websocket.Conn
//   - logger: gateway 持有的 base logger，加上 userID / roomID 两字段形成
//     contextual logger（**不**在此处加 sessionID —— sessionID 由 Register
//     拿到真实值后 With 叠加，保证 grep "sessionID=<id>" 命中且不碰到空值）
//   - maxMessageSize: 由 cfg.WS.MaxMessageSizeBytes 传入；调 conn.SetReadLimit
//   - writeTimeout: writeLoop 的 SetWriteDeadline 时长
//
// 构造时立即调 conn.SetReadLimit（V1 §12.2 关键约束 16 KB；prod 必须默认值）+
// 启动 ctx + 写入 lastHeartbeatAt 初值。
func newSession(
	sessionID string,
	userID uint64,
	roomID uint64,
	conn *websocket.Conn,
	logger *slog.Logger,
	maxMessageSize int,
	writeTimeout time.Duration,
) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	// closeWaitTimeout = writeTimeout + 200ms（review r3 P2 加）。**必须** ≥
	// writeTimeout，否则 closeInternal wait writeLoopDone 可能在 writeLoop 还卡在
	// conn.WriteMessage 内时提前超时，让 WriteControl close frame 与之后写出的
	// data frame 在 wire 上顺序错乱。
	// writeTimeout ≤ 0 防御性兜底：fall back 到 closeFrameWriteDeadline + buffer，
	// 与原版 500ms 行为相近（writeTimeout=0 = 不设 deadline，writeLoop 不会因
	// timeout 主动退；此时 wait 几乎只能等 sendChan 被 closeInternal 关 → 立刻
	// 命中关 chan 分支，无需大 buffer）。
	closeWait := writeTimeout + closeWaitBufferDuration
	if writeTimeout <= 0 {
		closeWait = closeFrameWriteDeadline + closeWaitBufferDuration
	}
	s := &Session{
		sessionID:        sessionID,
		userID:           userID,
		roomID:           roomID,
		conn:             conn,
		sendChan:         make(chan []byte, sendChanCapacity),
		sendPriorityChan: make(chan []byte, sendPriorityChanCapacity),
		writeLoopDone:    make(chan struct{}),
		// **不**在此处 With sessionID（review r4 P3 修）：让 Register 拿到真实
		// sessionID 后再 With 叠加，保证 logger 不出现 "sessionID="" 空字段。
		logger:           logger.With(slog.Uint64("userID", userID), slog.Uint64("roomID", roomID)),
		writeTimeout:     writeTimeout,
		closeWaitTimeout: closeWait,
		ctx:              ctx,
		cancelCtx:        cancel,
	}
	s.lastHeartbeatAt.Store(time.Now().UnixMilli())
	if maxMessageSize > 0 {
		conn.SetReadLimit(int64(maxMessageSize))
	}
	return s
}

// SessionID 返回 manager 分配的短 uuid。仅供测试 / 日志关联用；业务代码**不应**
// 依赖该字段做 equality 比较（Session 自身指针唯一）。
func (s *Session) SessionID() string { return s.sessionID }

// UserID 返回 Session 关联的已认证用户 ID。
func (s *Session) UserID() uint64 { return s.userID }

// RoomID 返回 Session 当前所在房间 ID。
func (s *Session) RoomID() uint64 { return s.roomID }

// LastHeartbeatAt 返回 Session 最后一次收到 client 消息的 unix ms epoch。
// 本 story 阶段**仅供 10.4 心跳超时扫描** goroutine 读取；业务代码**不应**消费。
func (s *Session) LastHeartbeatAt() int64 { return s.lastHeartbeatAt.Load() }

// Send 把消息字节流入队 sendChan（fire-and-forget；详见包顶部 sendChan 容量决策）。
//
// 错误：
//   - Session 已 Close → ErrSessionClosed
//   - sendChan 满 → ErrSessionSendBufferFull
//   - 入队成功 → nil（**不**保证消息已经被 client 收到，仅保证已入队）
//
// **关键约束**：调用方**不应**调用 Send 后假设消息已发送；如需"必送达"语义，
// 必须在 Service 层用 ack message + retry 实装。
//
// 并发：Send 持 sendMu.RLock 完成 closed-check + 入队；Close 持 sendMu.Lock
// 设置 closed flag + close(sendChan)。RWMutex 保证：多 Send 可并发（read 锁
// 可重入），但 Close 与所有正在执行的 Send 互斥 —— 永远不会发生
// "Send 看到 closed=false 进 select，Close 同时 close(sendChan) 让 select
// 命中已关 chan panic" 这条 race。
func (s *Session) Send(msg []byte) error {
	s.sendMu.RLock()
	defer s.sendMu.RUnlock()
	if s.closed.Load() {
		return ErrSessionClosed
	}
	select {
	case s.sendChan <- msg:
		return nil
	default:
		return ErrSessionSendBufferFull
	}
}

// SendPriority 把 protocol-level 消息（pong / 内部 close error）入队 priority
// buffer（review r4 P2 加）。writeLoop 优先消费 priority chan，业务 sendChan
// 满时仍能投递心跳回应。
//
// 错误：
//   - Session 已 Close → ErrSessionClosed
//   - sendPriorityChan 满 → ErrSessionSendPriorityBufferFull（理论不发生，
//     priority cap=4 + writeLoop 优先消费）
//   - 入队成功 → nil
//
// **使用约束**：仅供 protocol-level msg 用（pong / 协议错误）；业务消息**必须**
// 走 Send。如果业务路径滥用 SendPriority 会污染 priority buffer，pong 在突发
// 流量下仍可能被挤掉。
//
// 并发：与 Send 共用 sendMu.RLock；Close 持 sendMu.Lock 同时关 sendChan +
// sendPriorityChan，保证 send-on-closed-channel 不发生。
func (s *Session) SendPriority(msg []byte) error {
	s.sendMu.RLock()
	defer s.sendMu.RUnlock()
	if s.closed.Load() {
		return ErrSessionClosed
	}
	select {
	case s.sendPriorityChan <- msg:
		return nil
	default:
		return ErrSessionSendPriorityBufferFull
	}
}

// Close 关闭 Session（必须幂等）：
//   - 持 sendMu.Lock 标记 closed + close(sendChan)（与 Send 路径互斥，
//     防 send-on-closed-channel panic）
//   - cancel ctx（让 readLoop 内任何 ctx-aware 操作快速退出）
//   - 关闭底层 *websocket.Conn（gorilla 自身幂等）
//   - 通知 SessionManager 触发 OnSessionUnregister 钩子（如有 notifier）
//
// 多次调用不 panic / 不返 error（与 *sql.DB.Close / RedisClient.Close 一致）。
//
// **注**：与 CloseWithCode 不同 —— Close 在已 Close 的 Session 上仍返 nil（既有
// 入口语义；TestSession_Close_Idempotent 钦定）；CloseWithCode 在已 Close 上
// 返 ErrSessionClosed。差别原因：CloseWithCode 是 scanner / 协议错误路径调用，
// caller 需要区分"我刚 close" vs "已经被别人 close 了"做日志；Close 是常态收尾
// 路径（readLoop / writeLoop defer），不需要这种区分。
func (s *Session) Close() error {
	// closeInternal 在已关 Session 上返 ErrSessionClosed —— Close 路径
	// **吞掉**该 sentinel 保留入口"幂等不返错"语义。
	if err := s.closeInternal(false, 0, ""); err != nil && !errors.Is(err, ErrSessionClosed) {
		return err
	}
	return nil
}

// CloseWithCode 先写 WS close frame（code + reason）再释放本地资源。
//
// 用途（Story 10.4 引入）：HeartbeatScanner 在心跳超时时调本方法，让 client
// 收到 close code 4005 + reason "heartbeat timeout"，触发 iOS Story 12.5 指数
// 退避自动重连路径（V1 §12.1 钦定 4005 = transient network failure，应自动重连）。
//
// 行为（review r1 P2 引入，r3 P2 收紧 wait 上限）：
//  1. atomic CAS 翻 closed flag + 关 sendChan + 关 sendPriorityChan（与 Close
//     共用 closeInternal）—— 让 writeLoop 立即看到 chan 关闭、退出循环
//  2. **等** writeLoop 退出（writeLoopDone），上限 closeWaitTimeout =
//     writeTimeout + 200ms —— 此后没有任何 goroutine 会再 conn.WriteMessage
//     data frame；选 writeTimeout + buffer 作为上限的原因：writeLoop 在最坏
//     情况会被 conn.WriteMessage 卡到 writeTimeout 才超时退出，wait 上限**必须**
//     ≥ 这个时间，否则 wait 提前超时让 close frame 写在 data frame 之前
//  3. WriteControl close frame（deadline=closeFrameWriteDeadline=500ms）—— 此时
//     是 wire 上的唯一写者，没有 race；500ms 写一帧足够
//  4. cancelCtx + conn.Close + notifier.notifyClosed
//
// 错误：
//   - Session 已 Close → 返 ErrSessionClosed（不 emit close frame；scanner 看到
//     closed=true 就 skip，避免在已死 conn 上写无意义 close frame）
//   - WriteControl 失败 → log warn 但**不**返 error 给 caller（close frame 是
//     best-effort；wire 已死时 client 收不到也是预期）；继续释放本地资源
//   - 入口成功 → nil
//
// 并发：与 Close 安全并发（共用 closed atomic.Bool + sync.Once 兜底）。
//
// **顺序约束**（review r1 P2 修，原版有 race）：必须**先**关 sendChan/Priority
// + 等 writeLoop 退出，**再** WriteControl —— 顺序反了的话 writeLoop 仍可能在
// WriteControl 完成与 conn.Close 之间继续 drain 业务 sendChan / pong 数据，
// 让 client 看到 "close frame → data frame" 的协议错误（V1 §12.1 钦定 close
// frame 必须是 connection 最后一个 frame）。
func (s *Session) CloseWithCode(code int, reason string) error {
	return s.closeInternal(true, code, reason)
}

// closeInternal 是 Close / CloseWithCode 共用的 close 路径（review r1 P2 重构）。
//
// emitClose=true → CloseWithCode 路径，关 channel + 等 writeLoop 退出后**才**
// WriteControl 写 close frame；
// emitClose=false → Close 路径，跳过 WriteControl（仅释放本地资源）。
//
// 全部副作用包在 closeOnce 内部，多次调用幂等（第二次返 ErrSessionClosed）。
func (s *Session) closeInternal(emitClose bool, code int, reason string) error {
	if s.closed.Load() {
		return ErrSessionClosed
	}
	first := false
	notify := false
	s.closeOnce.Do(func() {
		first = true
		// Step 1: 锁内原子翻 closed flag + 关 sendChan / sendPriorityChan。
		// 任何并发 Send / SendPriority 此刻要么还没拿到 RLock（被本写锁阻塞），
		// 要么已经完成入队释放了 RLock；拿到 RLock 后看到 closed=true 立即返
		// ErrSessionClosed，不会再触达 select 的 send case。
		s.sendMu.Lock()
		s.closed.Store(true)
		close(s.sendChan)
		close(s.sendPriorityChan)
		s.sendMu.Unlock()

		// Step 2: 等 writeLoop 退出（review r1 P2 加 / r2 P2 修）。channel 已关，
		// writeLoop 在下一次 select 命中关 chan 立即 return；defer 内 _=s.Close()
		// 走幂等路径直接返 ErrSessionClosed 不会重入本 closeOnce。
		// **关键**：必须**先** wait writeLoop done，**才** WriteControl —— 否则
		// 仍可能 close frame 与 writeLoop 最后一帧并发写到 wire 触发协议错误。
		//
		// **review r2 P2 修**：必须 gate 在 writeLoopStarted。Gateway.Handle 的
		// handshake 失败路径（ListMembers / buildSnapshot / WriteMessage / Register
		// 失败）会在启动 readLoop/writeLoop **之前**调 session.Close 释放资源。
		// 此时 writeLoop 从没运行 → writeLoopDone 永远不会 close → 走 wait 分支
		// 必然落到 timeout 才返回。Story 10.3 之前的实装没有此 wait，所以
		// 失败 handshake cleanup 是亚毫秒级；r1 引入 wait 让每个 handshake 失败
		// 路径都付不必要的 tax，把 socket 释放从亚毫秒拖到亚秒。
		// 修法：用 writeLoopStarted atomic.Bool 标记 writeLoop 是否启动；只有
		// started=true 才等 writeLoopDone，否则直接跳过 —— 没启动的 goroutine
		// 不会写 wire，没必要等任何东西。
		// **review r3 P2 修**：wait 上限改用 closeWaitTimeout = writeTimeout + 200ms
		// 而非 closeFrameWriteDeadline（500ms）。原版 500ms < writeTimeout (默认 5s)
		// → writeLoop 卡在 conn.WriteMessage 时 wait 提前超时，WriteControl 写出
		// close frame 后 writeLoop 才结束写出 data frame，wire 上 close frame 后
		// 跟 data frame 违反 V1 §12.1。closeWaitTimeout 取 writeTimeout + 200ms
		// 是 writeLoop 实际最坏退出时间（writeMessage 最多卡 writeTimeout 后超时
		// 返 error，writeLoop 立即 break）+ goroutine 调度 buffer。
		// 防御性兜底：writeLoopDone 可能为 nil（极端测试场景未走 newSession
		// 构造直接 zero-init Session 的路径），nil channel select 永远阻塞 ——
		// 用 non-nil 检查 + 超时让本路径在异常 fixture 下不死锁。
		if s.writeLoopStarted.Load() && s.writeLoopDone != nil {
			waitTimeout := s.closeWaitTimeout
			if waitTimeout <= 0 {
				// zero-init Session（测试 fixture）兜底：用 closeFrameWriteDeadline
				// 作为最后防线，至少不死锁
				waitTimeout = closeFrameWriteDeadline
			}
			select {
			case <-s.writeLoopDone:
				// writeLoop 已退出
			case <-time.After(waitTimeout):
				// 防御性兜底：writeLoop 卡住（理论不发生，sendChan 已关 → select
				// 应立即命中关闭分支返回；超时仅用于防止异常 conn 卡 WriteMessage）
				s.logger.Warn("ws closeInternal writeLoop wait timeout",
					slog.Duration("waitTimeout", waitTimeout),
				)
			}
		}

		// Step 3: WriteControl close frame（仅 CloseWithCode 路径）。
		// 此时 writeLoop 已退出，WriteControl 是 conn 上的唯一写者，没有 race。
		if emitClose {
			deadline := time.Now().Add(closeFrameWriteDeadline)
			if err := s.conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(code, reason),
				deadline,
			); err != nil {
				s.logger.Warn("ws CloseWithCode write frame failed",
					slog.Int("code", code),
					slog.String("reason", reason),
					slog.Any("error", err),
				)
				// 不 return；继续释放本地资源（close frame 是 best-effort）
			}
		}

		// Step 4: cancel ctx + 关底层 conn + 触发 notifier 钩子。
		s.cancelCtx()
		// gorilla/websocket Close() 在已 close 的 conn 上是幂等的（返 net.ErrClosed
		// 或 nil；无 panic）；忽略错误 —— 关连接的具体失败模式与 Session 行为无关
		_ = s.conn.Close()
		notify = true
	})

	if !first {
		// closeOnce 已经被另一个 caller 进入过；本次返 ErrSessionClosed 与
		// "已 Close 的 Session 调 Close*" 入口语义一致
		return ErrSessionClosed
	}

	// 通知 manager unregister；仅在**第一次** Close 触发（与 closeOnce 语义对齐）；
	// 放在 closeOnce 外让 notifier 调用不持 sessionMu 写锁，避免 manager 钩子若反
	// 向调 Session 接口形成锁顺序死锁。manager.Unregister 自身不调 Session.Close
	// （只清索引），所以也不会形成 Close → notifyClosed → Unregister → Close 死循环。
	if notify && s.notifier != nil {
		s.notifier.notifyClosed(s.sessionID)
	}
	return nil
}

// readLoop 是读 goroutine 主循环（私有方法）。
//
// 主流程：
//  1. for { msg, err := s.conn.ReadMessage() }
//  2. err != nil → break loop（client 主动 close / 网络错 / read deadline 超时）
//  3. 收到消息 → 更新 s.lastHeartbeatAt = time.Now().UnixMilli()
//  4. 根据 envelope.type 路由：
//     - "ping" → 推 pong 占位 message 到 sendChan（writeLoop 消费）
//     - 其他 type → log warn + 安全忽略（与 V1 §12.3 末尾"安全忽略 + log warn"
//     钦定一致）
//  5. loop 退出 → s.Close()
//
// **不在本 story 范围**：
//   - 60s 心跳超时扫描（10.4 才做）
//   - 业务消息（emoji.send 等）路由到 service 层（Epic 11 / 17 才做）
//   - close code 4006 / message too large（10.4 阶段补完 reason 字符串；本 story
//     仅靠 conn.SetReadLimit 让 gorilla 自动 close）
//   - 真正校验 ping payload 字段表（10.4 阶段补完精确字段表）
func (s *Session) readLoop() {
	defer func() {
		// 任何 panic / 正常退出都触发 Close（确保资源回收）
		_ = s.Close()
	}()

	for {
		msgType, msg, err := s.conn.ReadMessage()
		if err != nil {
			// EOF / network error / SetReadLimit 触发的 close 都走这里
			if !s.closed.Load() {
				s.logger.Info("ws session read closed", slog.Any("error", err))
			}
			return
		}
		// 更新心跳时间戳（10.4 才会消费这个字段）
		s.lastHeartbeatAt.Store(time.Now().UnixMilli())

		// V1 §12.2 钦定 client → server 仅 text frame；其他 type log warn 忽略
		if msgType != websocket.TextMessage {
			s.logger.Warn("ws non-text frame ignored", slog.Int("messageType", msgType))
			continue
		}

		var env clientEnvelope
		if err := json.Unmarshal(msg, &env); err != nil {
			// V1 §12.3 末尾"安全忽略 + log warn"钦定：解析失败不 close 连接
			s.logger.Warn("ws envelope parse failed", slog.Any("error", err))
			continue
		}

		switch env.Type {
		case "ping":
			s.handlePing(env)
		default:
			// V1 §12.3 末尾草稿示例延后锚定声明：未识别 type 安全忽略 + log warn
			s.logger.Warn("ws unknown message type ignored", slog.String("type", env.Type), slog.String("requestId", env.RequestID))
		}
	}
}

// handlePing 收到 ping 后**占位**回 pong（V1 §12.3 pong 字段表的精确实装由
// Story 10.4 补完；本 story 阶段保证 RequestID 回带 + payload {} + ts）。
//
// 用 SendPriority（review r4 P2 修）：pong 走 protocol-level priority buffer，
// 不与业务 sendChan 共享 32 容量。writeLoop 优先消费 sendPriorityChan，让心跳
// 回应在业务 buffer 压力下仍能传达 —— 避免 server-side backpressure 让 client
// 误判 connection dead → reconnect 重试风暴。
func (s *Session) handlePing(env clientEnvelope) {
	pong := serverEnvelope{
		Type:      "pong",
		RequestID: env.RequestID, // V1 §12.3 pong 钦定回带 client 请求 RequestID
		Payload:   struct{}{},
		Ts:        time.Now().UnixMilli(),
	}
	bytes, err := json.Marshal(pong)
	if err != nil {
		// 理论不可能（serverEnvelope 全是 json-marshalable 字段）；防御性 log
		s.logger.Error("ws pong marshal failed", slog.Any("error", err))
		return
	}
	if err := s.SendPriority(bytes); err != nil {
		// Buffer 满 / Session 已关 → 客户端通常已下线；log warn 不 escalate
		s.logger.Warn("ws pong send failed", slog.Any("error", err))
	}
}

// maxConsecutivePriority 是 writeLoop 中连续消费 sendPriorityChan 的硬上限
// （review r7 P3 加）。
//
// **背景**（review r7 P3）：r4 给 writeLoop 加了 priority 优先消费模式 ——
// `select { priority }` 优先于 `select { priority / normal }`。该模式在 happy
// path 下让 pong 在业务 buffer 压力下快速送达。但**严格**优先 + 没有上限会让
// buggy / malicious client 持续高频 ping → handlePing 不断填 sendPriorityChan
// → priority 始终非空 → writeLoop 永远走"优先"分支，sendChan（业务消息：
// snapshot / broadcast / emoji 等）**永不被消费**。connection 心跳层看起来健康
// 但 client 永远收不到真实业务更新（典型 starvation bug）。
//
// **修法**（方案 a，review r7 推荐）：连续 drain priority 不超过 N 次后强制走
// 一次"双分支阻塞 select"，让 sendChan 至少有一次被命中机会。N = 4 与
// sendPriorityChanCapacity 对齐 —— 即"一次 priority buffer 容量 worth 的
// pong 发完，就给业务消息一次让路机会"。go select 多分支随机性保证让路那次
// 不偏 priority（约 50% 概率给 sendChan）。
//
// **不**选其它方案：
//   - 方案 b（select 多分支不带 priority 偏序）：破坏 r4 的 priority 设计 ——
//     pong 在业务 buffer 32 满载时不再快速响应；
//   - 方案 c（handlePing 入口限流）：违反 V1 §12.3 钦定的"pong 必发"协议
//     语义，client 自然超时 reconnect → 重连风暴。
const maxConsecutivePriority = 4

// writeLoop 是写 goroutine 主循环（私有方法）。
//
// 主流程（review r4 P2 加 priority chan，r7 P3 加 starvation 防护）：
//  1. for { select { sendPriorityChan / sendChan }; conn.WriteMessage(...) }
//  2. **优先**消费 sendPriorityChan（pong 等 protocol-level msg）：用 nested
//     select + non-blocking priority drain → 业务 sendChan 与 priority chan 都
//     有数据时**通常**先发 priority msg
//  3. 但连续 priority drain 超过 maxConsecutivePriority 次后，**强制**走双分支
//     阻塞 select 一次，避免 high-frequency ping 持续填 priority 让 sendChan
//     starve（review r7 P3）
//  4. 两个 chan 都关闭 → loop 退出
//  5. 写错 → log warn + s.Close()（wire 错通常意味着 conn 已死）
//
// **关键约束**：
//   - 必须用 TextMessage（V1 §12.2 关键约束钦定）
//   - 必须设 WriteDeadline（避免慢 client 卡住 server 写）
//   - 写错 → 触发 s.Close() 而非简单 log（wire-level 写失败通常表示 conn 已死，
//     继续 writeLoop 也会持续失败）
//   - 必须双 chan 都耗尽才能退出（防 priority 还有数据但 sendChan 已关 → 漏发
//     最后的 pong / 协议错误 msg）
//   - sendChan 不能 starve（review r7 P3，maxConsecutivePriority 上限）
//   - 退出时**必须先** close(writeLoopDone) **再** 调 s.Close()（review r1 P2 加）：
//     CloseWithCode 路径在 closeOnce.Do 内 wait writeLoopDone；如果 writeLoop
//     defer 顺序反了（先 Close 后 close(writeLoopDone)），sync.Once 会让 defer
//     的 Close 阻塞等 closeOnce.Do 跑完，而 closeOnce.Do 又在等 writeLoopDone
//     —— 死锁。把 close(writeLoopDone) 放最前确保 wait 路径先解锁。
//   - 入口**必须立刻** s.writeLoopStarted.Store(true)（review r2 P2 加）：
//     closeInternal 在 wait writeLoopDone 之前先 check writeLoopStarted；只有
//     started=true 才 wait。Store 在 defer 之前执行确保任何 close 路径（包括
//     即将 panic / 立刻 return 的极端情况）都能让 wait 正常等到 writeLoopDone。
func (s *Session) writeLoop() {
	// review r2 P2 加：标记 writeLoop 已启动。closeInternal 看到 started=true
	// 才会 wait writeLoopDone；handshake 失败路径调 Close 时 started=false →
	// 跳过 500ms wait，立即释放 socket。**必须**在任何可能 panic / return 的
	// 操作之前置位（即使 writeLoop 立刻退出，wait 路径也能正常等到 done）。
	s.writeLoopStarted.Store(true)
	defer func() {
		// **顺序关键**：先 close writeLoopDone 让 closeInternal 的 wait 解锁，
		// 再 _=s.Close()（幂等；如果 closeInternal 已在跑，sync.Once 让本 Close
		// 走 fast-path 返 ErrSessionClosed 被 Close 包装吞掉返 nil）。
		close(s.writeLoopDone)
		_ = s.Close()
	}()
	// consecutivePriority 跟踪连续命中 priority 分支的次数；命中 sendChan 或
	// 走"强制让路"分支后清零。
	consecutivePriority := 0
	for {
		if consecutivePriority < maxConsecutivePriority {
			// 优先级 select：先 nested 非阻塞 select 检查 priority chan；为空才走
			// 阻塞 select 等任意一边来消息。Go 没有内建优先级 select 语法，这是
			// 标准的两段式 priority 模式。
			select {
			case msg, ok := <-s.sendPriorityChan:
				if !ok {
					// priority chan 关 → Close 路径在跑，退出 loop
					return
				}
				if err := s.writeFrame(msg); err != nil {
					return
				}
				consecutivePriority++
				continue
			default:
				// priority 没数据 → 阻塞等任意一边
			}
		}
		// 三种到达本分支的情况，都走"双分支阻塞 select"（priority + normal 平等）：
		//   1) priority chan 当前为空（fast path 让出）
		//   2) 已经连续 drain 了 maxConsecutivePriority 个 priority，强制让 sendChan
		//      有一次被选中机会（review r7 P3 starvation 防护）
		// Go select 多分支自带随机选择 → 两边都有数据时 ~50/50 命中。
		select {
		case msg, ok := <-s.sendPriorityChan:
			if !ok {
				return
			}
			if err := s.writeFrame(msg); err != nil {
				return
			}
			// 走到此分支说明刚才 fast path 看到 priority 空（或刚好 quota 用完
			// 进双分支随机选中 priority）；都视为 priority 流不再"连续"，重置
			// 计数让下一轮 fast path 重新积累，避免 quota 永久卡死。
			consecutivePriority = 0
		case msg, ok := <-s.sendChan:
			if !ok {
				return
			}
			if err := s.writeFrame(msg); err != nil {
				return
			}
			// 命中 sendChan → priority 不再"连续"，清零让下一轮 fast path 重新生效。
			consecutivePriority = 0
		}
	}
}

// writeFrame 是 writeLoop 内部的"写一帧"小工具：设 WriteDeadline + WriteMessage
// + log warn。返 error 让 caller 决定是否退出 loop（写错 = 退出）。
func (s *Session) writeFrame(msg []byte) error {
	if s.writeTimeout > 0 {
		_ = s.conn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
	}
	if err := s.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
		s.logger.Warn("ws write failed", slog.Any("error", err))
		return err
	}
	return nil
}
