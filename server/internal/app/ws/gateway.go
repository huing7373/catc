package ws

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/pkg/auth"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// WS 契约 prod 不可变值（V1 §1 节点 4 冻结声明 + §12.2 关键约束钦定）。
// prod 部署必须用这两个值；YAML 覆盖会引发跨实例 / 跨端契约漂移（不同节点心跳
// 阈值不同 → presence 抖动；max_message_size_bytes 不同 → 一边能收一边超 limit
// 静默断连）。Story 10.3 review r2 P2 引入。
const (
	wsProdHeartbeatTimeoutSec = 60
	wsProdMaxMessageSizeBytes = 16384
)

// closeWriteDeadline 是 closeWithCode 写 close frame 的 deadline。
//
// 短（500ms）足够：写 control frame 是单 packet 操作；超时通常意味着对端已掉线，
// 此时 close frame 已经无意义 —— 与其等到 5s+ 才放手，不如快速 Close 让上层流程
// 推进。lesson：2026-04-26-startup-blocking-io-needs-deadline 钦定 IO 必须有
// 本地 timeout（启动期 / 运行期 / cleanup 期都适用）。
const closeWriteDeadline = 500 * time.Millisecond

// Gateway 是 WS 网关 handler 的依赖容器（与 handler/auth_handler.go 同模式）。
type Gateway struct {
	signer       *auth.Signer
	mgr          SessionManager
	roomMember   mysql.RoomMemberRepo
	upgrader     *websocket.Upgrader
	logger       *slog.Logger
	cfg          config.WSConfig
	writeTimeout time.Duration
	// builder（Story 10.7 引入）：room.snapshot 构造路径。
	//
	// 节点 4 placeholder 实装：placeholderSnapshotBuilder（NewPlaceholderSnapshotBuilder
	// 构造），走 RoomMemberRepo.ListMembers 单表查询。Story 11.7 真实实装时替换为
	// realSnapshotBuilder，gateway 层不感知。
	//
	// **不可 nil**：NewGateway 内 fail-fast 校验。
	builder SnapshotBuilder
}

// NewGateway 构造 Gateway（main.go bootstrap wire 用）。
//
// upgrader 内部构造（CheckOrigin 节点 4 阶段返 true 让 iOS / dev 联调免 CORS；
// 节点 9+ prod launch 阶段改成白名单，与 RateLimit 切 Redis 同期）。
//
// envName 是部署环境名（"prod" / "staging" / "dev" / "test"，默认 "prod"），
// 由 main.go 从环境变量 `CAT_ENV` 读取传入；用于"prod 必须用契约默认值"强制
// （review r2 P2 引入；与 NewStepService 同模式）：
//   - envName == "prod"（含空 / 不识别值，按 prod 严格策略）+ HeartbeatTimeoutSec
//     != 60 → panic
//   - envName == "prod" + MaxMessageSizeBytes != 16384 → panic
//   - envName ∈ {"dev", "staging", "test"} → 接受 YAML 任何值覆盖（仅供单测 / 调试）
//
// 为什么 fail-fast：V1 §12.2 钦定 heartbeat_timeout_sec=60 + max_message_size_bytes=
// 16384 是跨节点 / 跨端协议契约一部分。生产配置错（如 K8s ConfigMap 误注入）会让
// 该节点和其他节点 / iOS 客户端的协议漂移 —— heartbeat 不匹配让 presence 状态抖动；
// max_frame_size 不匹配让一边能收一边超 limit 静默断连。这正是 NewStepService prod-cap
// 强制的同类问题。WriteTimeoutSec 不在契约，不强制。
func NewGateway(
	signer *auth.Signer,
	mgr SessionManager,
	roomMember mysql.RoomMemberRepo,
	cfg config.WSConfig,
	envName string,
	builder SnapshotBuilder,
) *Gateway {
	// **prod 配置覆盖强制**（review r2 P2；与 Story 7.3 NewStepService 同模式）：
	// envName 归一化为小写；只有显式 "dev" / "staging" / "test" 才允许 contract 字段
	// 覆盖。**空 / 未知 / "prod" / "production"** 全部按 prod 严格策略（safe-by-default：
	// 未注入 CAT_ENV 或 typo 都视为 prod）。
	envLower := strings.ToLower(strings.TrimSpace(envName))
	isOverrideAllowed := envLower == "dev" || envLower == "staging" || envLower == "test"
	if !isOverrideAllowed {
		if cfg.HeartbeatTimeoutSec != wsProdHeartbeatTimeoutSec {
			panic(fmt.Sprintf(
				"ws gateway: prod env (CAT_ENV=%q) must use default heartbeat_timeout_sec=%d; got %d (V1 §12.2 钦定；dev/test 覆盖必须 export CAT_ENV=dev|staging|test)",
				envName, wsProdHeartbeatTimeoutSec, cfg.HeartbeatTimeoutSec,
			))
		}
		if cfg.MaxMessageSizeBytes != wsProdMaxMessageSizeBytes {
			panic(fmt.Sprintf(
				"ws gateway: prod env (CAT_ENV=%q) must use default max_message_size_bytes=%d; got %d (V1 §12.2 钦定；dev/test 覆盖必须 export CAT_ENV=dev|staging|test)",
				envName, wsProdMaxMessageSizeBytes, cfg.MaxMessageSizeBytes,
			))
		}
	}

	// **builder fail-fast**（Story 10.7 引入；与 signer / mgr / roomMember 同模式）：
	// Gateway struct 字段无 nil-safe 保护，缺失依赖在启动期 fail-fast 比 request 期
	// nil-deref panic 更安全，运维 CrashLoopBackOff 立即可见。
	if builder == nil {
		panic("ws gateway: SnapshotBuilder is required (Story 10.7; bootstrap 期必须 wire NewPlaceholderSnapshotBuilder)")
	}

	upgrader := &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		// CheckOrigin 节点 4 阶段返 true（不限制源；iOS / dev 联调免 CORS 烦恼）；
		// 节点 9+ prod launch 阶段（Epic 36）改成白名单。
		CheckOrigin: func(r *http.Request) bool { return true },
		// EnableCompression: false（V1 §12.2 钦定 text frame；不压缩简化协议；
		// 节点 4 单消息 ≤ 16 KB，压缩收益不大）
	}
	writeTimeout := time.Duration(cfg.WriteTimeoutSec) * time.Second
	if writeTimeout <= 0 {
		writeTimeout = 5 * time.Second
	}
	return &Gateway{
		signer:       signer,
		mgr:          mgr,
		roomMember:   roomMember,
		upgrader:     upgrader,
		logger:       slog.Default(),
		cfg:          cfg,
		writeTimeout: writeTimeout,
		builder:      builder,
	}
}

// Handle 是 Gin 路由 GET /ws/rooms/:roomId 的 handler。
//
// 严格按 V1 §12.1 服务端校验顺序实装（任何顺序错都会让 close code 不一致）：
//
//  1. 解析 query "token"；缺失 → close 4001 reason "missing token"
//  2. 解析路径参数 roomId；非数字 / 缺失 → close 4002 reason "invalid roomId"
//  3. token 校验（signer.Verify(token) → claims）；失败 → close 4001 reason
//     区分 "token expired" / "invalid token"（用 errors.Is(err, auth.ErrTokenExpired)
//     判定）
//  4. room 存在性校验（roomMemberRepo.RoomExists(ctx, roomID)）；
//     false → close 4004 reason "room not found"
//  5. user 在 room_members 校验（roomMemberRepo.IsUserInRoom(ctx, userID, roomID)）；
//     false → close 4003 reason "user not in room"
//  6. （上述任一异常）→ close 1011 reason "<short error message>"
//     （如 DB query 失败 / panic）
//
// 校验全通过后**严格按以下顺序**执行（review r10 P1 修：reconnect 路径下
// 把 Register 推迟到 snapshot 写成功之后，避免 transient snapshot 失败把旧
// session 误 evict 让 user 完全断线）：
//
//  1. upgrader.Upgrade 升级到 WS 协议（已在 step 2 / 3 失败路径中提前 Upgrade
//     才能写 close frame；happy path 不需要在这里再 Upgrade —— 实装上 V1 §12.1
//     close code 表钦定校验失败必须先 Upgrade，所以 Handle 内一进 handler 立即
//     Upgrade，再走校验链路）
//  2. 创建 Session 对象（newSession，sessionID 留空；**不** Register）
//  3. **同步段**写 placeholder room.snapshot：
//     - 调 SendRoomSnapshot(ctx, conn, roomID, g.builder, g.writeTimeout)
//       —— Story 10.7 引入；内部走 SnapshotBuilder.BuildSnapshot 构造 +
//       conn.WriteMessage 同步写入；失败统一返 error
//     - SendRoomSnapshot 返 error → close 1011 reason "snapshot build failed"，
//       **不**启动读/写 goroutine、**不** Register —— 旧 session（如有）保持
//       活跃，user 可继续用旧连接
//  4. mgr.Register(ctx, session)（触发 OnSessionRegister 钩子；reconnect 路径
//     在此点 evict 旧 session）—— **必须**在 snapshot 写成功之后才执行；
//     Register 失败 → close 1011 reason "session register failed"
//  5. go session.readLoop() 启动读 goroutine
//  6. go session.writeLoop() 启动写 goroutine
//
// **顺序 rationale**（review r10 P1）：V1 §12.1 字面写"先 Register 再 snapshot"，
// 但 reconnect 路径下 Register 会 evict 旧 session；如果 snapshot 步骤 transient
// 失败（DB 抖动 / client 中途断），handler close 1011 → user 既无新 session（被
// 1011 close）也无旧 session（已被 evict）→ **完全断线**。把 Register 推迟到
// snapshot 写成功之后实现"事务性 reconnect"：snapshot 失败时旧 session 仍活跃，
// snapshot 成功才替换。这是局部偏离 spec 字面顺序但保持其精神（"snapshot 必为
// 第一条 authoritative msg"在新 conn 上仍成立）的正确性修复。
//
// **关键反模式**（不要做）：
//   - 不在 Upgrade 之前发 close code（HTTP 403 是错的；V1 §12.1 钦定校验失败
//     必须**先 Upgrade 成功** 再发 close frame）
//   - 不在 readLoop 启动后再写 snapshot（窗口期 client 可能发 ping，server 已
//     有读 goroutine → server 可能先回 pong 让 snapshot 不再是第一条）
//   - 不在 Register 之前启动 read/write goroutine（review r4 P3 修过：
//     session.notifier 在 Register 之前是 nil，readLoop / writeLoop 内 Close
//     触发 notifier 会 nil panic；必须 Register 完成才启 goroutine）
func (g *Gateway) Handle(c *gin.Context) {
	ctx := c.Request.Context()

	// V1 §12.1 钦定：校验失败必须发 close frame，而非 HTTP 错误 → 必须**先**升级
	// 协议（HTTP 101 Switching Protocols）才能 emit close frame。Upgrade 自身
	// 失败（如 client 不发 Upgrade header）走 HTTP 400 错误路径（gorilla 内部
	// 已写 HTTP 400 + 调用方仅记日志）。
	conn, err := g.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		g.logger.Warn("ws upgrade failed", slog.Any("error", err))
		return
	}
	// **关键**：不能在这里 `defer conn.Close()` —— happy path 启动 read/write
	// goroutine 后 Handle 立即返回，defer 会在 goroutine 还在用 conn 时关闭它，
	// 导致 readLoop 立刻拿到 EOF / "use of closed network connection"。
	// conn 生命周期由 Session.Close 接管（happy path）；非 happy 路径在写完
	// close frame 后**显式**调 conn.Close()（见各 close 分支）。

	// 1. token query 解析
	token := c.Query("token")
	if token == "" {
		g.closeWithCode(conn, 4001, "missing token")
		g.logger.Info("ws handshake rejected: missing token")
		return
	}

	// 2. roomId 路径参数解析
	roomIDStr := c.Param("roomId")
	roomID, err := strconv.ParseUint(roomIDStr, 10, 64)
	if err != nil || roomID == 0 {
		g.closeWithCode(conn, 4002, "invalid roomId")
		g.logger.Warn("ws handshake rejected: invalid roomId", slog.String("roomId", roomIDStr))
		return
	}

	// 3. token verify
	claims, err := g.signer.Verify(token)
	if err != nil {
		reason := "invalid token"
		if errors.Is(err, auth.ErrTokenExpired) {
			reason = "token expired"
		}
		g.closeWithCode(conn, 4001, reason)
		g.logger.Info("ws handshake rejected: token verify failed",
			slog.String("reason", reason), slog.Any("error", err))
		return
	}
	userID := claims.UserID

	// 4. room 存在性校验
	exists, err := g.roomMember.RoomExists(ctx, roomID)
	if err != nil {
		g.closeWithCode(conn, 1011, "internal error")
		g.logger.Error("ws handshake: RoomExists failed",
			slog.Uint64("roomID", roomID), slog.Any("error", err))
		return
	}
	if !exists {
		g.closeWithCode(conn, 4004, "room not found")
		g.logger.Warn("ws handshake rejected: room not found", slog.Uint64("roomID", roomID))
		return
	}

	// 5. user 在 room_members 校验
	inRoom, err := g.roomMember.IsUserInRoom(ctx, userID, roomID)
	if err != nil {
		g.closeWithCode(conn, 1011, "internal error")
		g.logger.Error("ws handshake: IsUserInRoom failed",
			slog.Uint64("userID", userID), slog.Uint64("roomID", roomID), slog.Any("error", err))
		return
	}
	if !inRoom {
		g.closeWithCode(conn, 4003, "user not in room")
		g.logger.Warn("ws handshake rejected: user not in room",
			slog.Uint64("userID", userID), slog.Uint64("roomID", roomID))
		return
	}

	// 校验全通过 → V1 §12.1 握手成功流程（review r10 P1 调整顺序：Register
	// 推迟到 snapshot 写成功之后，避免 reconnect 路径 transient snapshot 失败
	// 让旧 session 被无故 evict。详见 Handle 头注释"顺序 rationale"段）。
	//
	// 6.1 创建 Session（sessionID 暂为空，Register 内回填）；**不** Register。
	// 此时 Session 尚未进入 manager 索引，旧 session（如有）保持活跃。
	session := newSession("", userID, roomID, conn, g.logger, g.cfg.MaxMessageSizeBytes, g.writeTimeout)

	// 6.2 同步段构造 + 写 placeholder room.snapshot（**先于** Register；
	// transient 失败时旧 session 不被 evict）—— Story 10.7 把原 inline 三段
	// （ListMembers + buildPlaceholderSnapshot + conn.WriteMessage）抽离到
	// SendRoomSnapshot 包级函数；失败处理路径（close 1011 + Session.Close）保留
	// 在调用点（Handle 仍是错误处理 single source of truth）。
	//
	// **未** Register —— 不需要 manager 侧 cleanup；Session 结构无副作用占用，
	// 但需要显式 Close 释放 Session 持有的 ctx / chan（newSession 内已分配）+
	// gorilla conn（closeWithCode 已 conn.Close，session.conn 是同一指针 ——
	// session.Close 内 _ = s.conn.Close 是 idempotent no-op，不会 double close
	// panic）。
	if err := SendRoomSnapshot(ctx, conn, roomID, g.builder, g.writeTimeout); err != nil {
		g.closeWithCode(conn, 1011, "snapshot build failed")
		g.logger.Error("ws handshake: snapshot send failed",
			slog.Uint64("roomID", roomID), slog.Any("error", err))
		_ = session.Close()
		return
	}

	// 6.3 Register（触发 OnSessionRegister 钩子；ErrSessionReplaced 是合法路径
	// 不阻塞握手）—— **必须**在 snapshot 写成功之后才执行（review r10 P1）。
	// 注意：Register 内若 reconnect 命中，会 evict + Close 旧 session；snapshot
	// 已在 NEW conn 上写完，OLD conn 此刻被 close 是预期行为（user 在 NEW 上活跃）。
	sessionID, regErr := g.mgr.Register(ctx, session)
	if regErr != nil && !errors.Is(regErr, ErrSessionReplaced) {
		// Register 失败（如 manager 已 Close 返 ErrSessionManagerClosed）→
		// snapshot 已经在 wire 上但 Session 没进 manager 索引 → 必须 close 1011
		// + Session.Close 释放本 Session 资源；旧 session（如有）保持活跃因为
		// Register 没真正 evict（manager 内 Register 失败路径不动 userToSessionID）。
		g.closeWithCode(conn, 1011, "session register failed")
		g.logger.Error("ws handshake: session register failed", slog.Any("error", regErr))
		_ = session.Close()
		return
	}
	if errors.Is(regErr, ErrSessionReplaced) {
		g.logger.Info("ws session replaced previous connection",
			slog.String("sessionID", sessionID), slog.Uint64("userID", userID), slog.Uint64("roomID", roomID))
	}

	// 6.4 启动读 / 写 goroutine（snapshot 已经在 wire 上 + session 已在 manager
	// 索引内 + session.notifier 已在 Register 内 set，readLoop/writeLoop 自闭路径
	// 触达 notifier 不会 nil panic）
	go session.readLoop()
	go session.writeLoop()

	g.logger.Info("ws handshake completed",
		slog.String("sessionID", sessionID),
		slog.Uint64("userID", userID),
		slog.Uint64("roomID", roomID),
	)
}

// closeWithCode 写一个 control close frame（gorilla.FormatCloseMessage 标准格式）
// 然后**立即关闭** conn（不再 defer，详见 Handle 注释）。
//
// 写 close frame 必须带 deadline（避免对端僵死时本调用阻塞）；用包级 const
// closeWriteDeadline 兜底。
//
// 不返 error：close frame 是 best-effort —— 写不出去通常意味着 conn 已挂，没有
// 进一步处理意义；调用方只关心校验结果，不关心 close frame 是否真到达 client。
//
// 调用方按校验失败 / handshake 不完整路径调用本方法；happy path 不调用（conn
// 生命周期由 Session.Close 接管）。
func (g *Gateway) closeWithCode(conn *websocket.Conn, code int, reason string) {
	deadline := time.Now().Add(closeWriteDeadline)
	_ = conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(code, reason),
		deadline,
	)
	_ = conn.Close()
}

// **历史**：Story 10.3 ~ 10.6 在本文件 inline 实装了 placeholder snapshot 的
// helper（snapshotPayload / snapshotRoom / snapshotMember / snapshotPet 4 个
// struct + buildPlaceholderSnapshot 包级函数）。Story 10.7 把这些抽离到
// snapshot.go 中作为 SnapshotBuilder 接口的 placeholderSnapshotBuilder 实装 +
// SendRoomSnapshot 包级函数 —— gateway.Handle 单点调用 SendRoomSnapshot 替换
// 原本的三段直连代码（ListMembers + buildPlaceholderSnapshot + conn.WriteMessage）。
//
// 编译时接口断言：确保 mysql.RoomMemberRepo 是 nilRoomMemberRepo 的接口形态
// （让 Gateway struct 字段定义在编译期被 type system 校验）。
var _ mysql.RoomMemberRepo = (*nilRoomMemberRepo)(nil)

type nilRoomMemberRepo struct{}

func (*nilRoomMemberRepo) RoomExists(_ context.Context, _ uint64) (bool, error) {
	return false, nil
}
func (*nilRoomMemberRepo) IsUserInRoom(_ context.Context, _ uint64, _ uint64) (bool, error) {
	return false, nil
}
func (*nilRoomMemberRepo) ListMembers(_ context.Context, _ uint64) ([]uint64, error) {
	return nil, nil
}

// Create 兜底（Story 11.3 给 RoomMemberRepo interface 加 Create 方法后编译需要；
// nil-pattern struct 不应被 ws 路径调用 —— gateway 只读 room 状态，写入由 Epic 11
// HTTP service 层负责）。
func (*nilRoomMemberRepo) Create(_ context.Context, _ *mysql.RoomMember) error {
	return nil
}
