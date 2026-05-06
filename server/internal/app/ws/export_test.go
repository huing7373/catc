package ws

import (
	"context"
	"log/slog"
	"time"
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

// ScanOnceForTest 暴露 unexported scanOnce 给 ws_test 单测直接调用（绕过 ticker）。
func (s *HeartbeatScanner) ScanOnceForTest(ctx context.Context, now time.Time) {
	s.scanOnce(ctx, now)
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
