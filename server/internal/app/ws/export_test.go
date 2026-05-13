package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// 本文件让 black-box 测试包 ws_test 能访问 unexported 测试 helper / 内部字段
// （Go 标准 export_test.go 模式；详见 stdlib 大量先例如 net/http、encoding/json）。
//
// **禁止**在 production 路径调用本文件导出的标识符 —— 命名上 *ForTest 后缀 +
// 文件名 *_test.go 让 go build 自动忽略本文件。

// NewHeartbeatScannerForTest 是 newHeartbeatScannerForTest 的 exported 别名
// （Story 10.4 引入），让 ws_test 包能注入小 interval / 小 timeoutMs 让单测加速。
//
// 参数语义见 newHeartbeatScannerForTest 注释。
func NewHeartbeatScannerForTest(mgr SessionManager, timeoutMs int64, interval time.Duration, logger *slog.Logger) *HeartbeatScanner {
	return newHeartbeatScannerForTest(mgr, timeoutMs, interval, logger)
}

// NewHeartbeatScannerForTestWithRenewer 是 newHeartbeatScannerForTestWithRenewer
// 的 exported 别名（review 10-6 r2 P1 引入），让 ws_test 包能注入 PresenceRenewer
// stub 验证续期路径。
//
// 参数语义见 newHeartbeatScannerForTestWithRenewer 注释。**review 10-6 r9 P1**：
// 不在 signature 里加 userPresenceMu —— 既有测试无须传锁（默认 nil 走无锁路径）；
// 验证 mutex 串行不变量的新单测走 NewHeartbeatScannerForTestWithMutex。
func NewHeartbeatScannerForTestWithRenewer(mgr SessionManager, timeoutMs int64, interval time.Duration, logger *slog.Logger, renewer PresenceRenewer) *HeartbeatScanner {
	return newHeartbeatScannerForTestWithRenewer(mgr, timeoutMs, interval, logger, renewer, nil)
}

// NewHeartbeatScannerForTestWithMutex 是 newHeartbeatScannerForTestWithRenewer
// 的 exported 别名（review 10-6 r9 P1 加），让 ws_test 包注入 UserPresenceMutex
// 验证 scanner reconcile 与 hook 共享 mutex 的串行不变量。
//
// 参数语义见 newHeartbeatScannerForTestWithRenewer 注释。
func NewHeartbeatScannerForTestWithMutex(mgr SessionManager, timeoutMs int64, interval time.Duration, logger *slog.Logger, renewer PresenceRenewer, userPresenceMu UserPresenceMutex) *HeartbeatScanner {
	return newHeartbeatScannerForTestWithRenewer(mgr, timeoutMs, interval, logger, renewer, userPresenceMu)
}

// ScanOnceForTest 暴露 unexported scanOnce 给 ws_test 单测直接调用（绕过 ticker）。
//
// **不**包 wg.Wait —— 与 production Run 路径行为一致（fire-and-forget dispatch）。
// 既有 close-fanout 测试（如 TestHeartbeatScanner_ScanOnce_RaceRefreshAfterListing_
// NotClosed）依赖此 fire-and-forget 语义在 ScanOnceForTest 返回后到 fanout
// goroutine 实际跑 recheck 之间塞 SetLastHeartbeatAt 模拟 race —— 不能在此 helper
// 内 wg.Wait 把窗口关掉。
//
// reconcile 路径（review 10-6 r5 P1：fanout 化）的单测需要 sync 断言 renewer
// state 时，**显式**调 DrainFanoutForTest（或 ScanOnceAndDrainForTest）等 wg。
func (s *HeartbeatScanner) ScanOnceForTest(ctx context.Context, now time.Time) {
	s.scanOnce(ctx, now)
}

// ScanOnceAndDrainForTest 是 scanOnce + wg.Wait 的组合（review 10-6 r5 P1）。
//
// 用途：reconcile 路径单测需要在 ScanOnceForTest 返回后立刻断言 fakeRenewer 状态
// （count / lastSession），**必须**先等 fanout 跑完才能可靠读。
//
// 与 ScanOnceForTest 的区别：本 helper 多了 wg.Wait —— 让 fanout drain 后才返回。
// 不要把 wg.Wait 直接塞进 ScanOnceForTest（会破坏 close-fanout race 测试的语义，
// 那类测试依赖 ScanOnceForTest 返回后 fanout 仍未跑的窗口期）。
func (s *HeartbeatScanner) ScanOnceAndDrainForTest(ctx context.Context, now time.Time) {
	s.scanOnce(ctx, now)
	s.wg.Wait()
}

// DrainFanoutForTest 显式 wg.Wait 让单测可在调过 ScanOnceForTest 后
// 主动 drain（review 10-6 r5 P1）。
//
// 与 ScanOnceAndDrainForTest 内含 wg.Wait 等价，但允许测试**先**测主 loop 耗时，
// **后** drain fanout 验证 count，让两段时序断言分开测量。
func (s *HeartbeatScanner) DrainFanoutForTest() {
	s.wg.Wait()
}

// TimeoutMsForTest 暴露 internal cfg.timeoutMs 给单测断言（如 zero-input 兜底）。
func (s *HeartbeatScanner) TimeoutMsForTest() int64 {
	return s.cfg.timeoutMs
}

// SetLastHeartbeatAtForTest 让单测直接覆盖 Session.lastHeartbeatAt（review r1
// P1 TOCTOU regression test 用）—— 不走 wire 路径，避免 ping/pong 真实写入触发
// readLoop 副作用，让 race 窗口测试可控。
//
// **禁止**在 production 路径调用 —— 命名 *ForTest 后缀 + export_test.go 文件
// 让 go build 自动忽略本入口。
func (s *Session) SetLastHeartbeatAtForTest(unixMs int64) {
	s.lastHeartbeatAt.Store(unixMs)
}

// CloseWaitTimeoutForTest 暴露 internal closeWaitTimeout 给 ws_test 包断言
// （review r3 P2：closeWaitTimeout = writeTimeout + 200ms 不变量）。
func (s *Session) CloseWaitTimeoutForTest() time.Duration {
	return s.closeWaitTimeout
}

// WriteTimeoutForTest 暴露 internal writeTimeout 给单测断言对照用。
func (s *Session) WriteTimeoutForTest() time.Duration {
	return s.writeTimeout
}

// NewSessionForTest 是 newSession 的 exported 别名（Story 17.5 引入），让
// ws_test 包 emoji_handler_test 能跨包访问 unexported newSession 构造 + 内部启动
// writeLoop。
//
// **使用约定**：
//   - sid: 单测可传任意非空 string；不进 SessionManager 索引，仅用于 logger 字段
//   - uid / rid: caller 自定义 user/room 标识
//   - conn: 必须是 server-side *websocket.Conn（gorilla.Upgrade 后的 conn，不是
//     client.Dial 拿到的 conn）；本 helper 内**自动启动** writeLoop，让 SendPriority
//     入队的 msg 真的被消费写到 wire
//   - emojiHandler: 可为 nil；不为 nil 时 readLoop dispatch 到该 handler
//
// 与 14.4 既有 handler test 同模式：handler test 不依赖完整 readLoop（test 是
// 同步调 HandleEmojiSend 直接传 envelope），但**必须**启动 writeLoop —— 否则
// SendPriority 入队的 error envelope 永不被消费 → client.ReadMessage 超时。
//
// **不**启动 readLoop：handler test 不需要从 client 读消息，只验证 SendPriority
// 的 wire 输出；readLoop 启动会触发 client/server-conn 双向 ReadMessage 互相阻塞，
// 把 test 复杂度提升而无收益。
func NewSessionForTest(_ testing.TB, sid string, uid, rid uint64, conn *websocket.Conn, emojiHandler EmojiHandler) *Session {
	s := newSession(sid, uid, rid, conn, slog.Default(), 16384, 5*time.Second, emojiHandler)
	// 自动启动 writeLoop（让 SendPriority 入队的 msg 真的写到 conn）。**不**启
	// readLoop（emoji_handler_test 不消费 client → server 方向消息）。
	go s.writeLoop()
	return s
}

// NewClientEnvelopeForTest 是 clientEnvelope 的构造便利函数（Story 17.5 引入）。
//
// 用途：emoji_handler_test 直接构造 envelope 调 HandleEmojiSend，绕过完整的
// readLoop JSON 解析路径。
func NewClientEnvelopeForTest(typ, reqID string, payload []byte) ClientEnvelopeForTest {
	return ClientEnvelopeForTest{
		Type:      typ,
		RequestID: reqID,
		Payload:   json.RawMessage(payload),
	}
}

// ClientEnvelopeForTest 是 clientEnvelope 的可导出别名（Story 17.5 引入）；
// 字段集合与 clientEnvelope 1:1 对齐，但首字母大写让 ws_test 包能直接构造。
//
// **使用**：通过 NewClientEnvelopeForTest 构造，然后调
// `handler.HandleEmojiSend(ctx, session, env.AsInternal())` 转回 internal type。
type ClientEnvelopeForTest struct {
	Type      string
	RequestID string
	Payload   json.RawMessage
}

// AsInternal 把 ClientEnvelopeForTest 转回 unexported clientEnvelope（让 ws_test
// 包能传给 EmojiHandler.HandleEmojiSend）。
func (e ClientEnvelopeForTest) AsInternal() clientEnvelope {
	return clientEnvelope{
		Type:      e.Type,
		RequestID: e.RequestID,
		Payload:   e.Payload,
	}
}

// BroadcastToRoomForTest 是 BroadcastToRoom 的测试别名（review 10-5 r1 后保留
// 作语义标注 + 既有测试调用站不破坏）。
//
// **历史**：r1 之前生产路径走 fire-and-forget goroutine fanout，ForTest 变体内
// wg.Wait() 等所有 goroutine drain 让单测可同步 assert。r1 review 指出 fanout
// goroutine 不保证 per-session 顺序 + msg buffer ownership 泄漏 → 生产路径改成
// 同步 for-range 调 Session.Send，wg / goroutine 完全去掉。同步后 ForTest 与
// 生产路径行为一致 —— 保留本别名仅为：
//   - 既有测试调用站（TestBroadcastToRoom_* in ws_test.go）不破坏
//   - 测试代码语义清晰（ForTest 后缀提示"测试场景，不依赖任何 fire-and-forget
//     语义"）
//
// **禁止**在生产路径调用 —— 命名 *ForTest 后缀 + export_test.go 文件让 go build
// 自动忽略本入口；生产路径必须用 BroadcastToRoom。
func BroadcastToRoomForTest(ctx context.Context, mgr SessionManager, roomID uint64, msg []byte) (sent int, err error) {
	return BroadcastToRoom(ctx, mgr, roomID, msg)
}
