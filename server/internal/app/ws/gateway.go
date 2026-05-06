package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/pkg/auth"
	"github.com/huing/cat/server/internal/repo/mysql"
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
}

// NewGateway 构造 Gateway（main.go bootstrap wire 用）。
//
// upgrader 内部构造（CheckOrigin 节点 4 阶段返 true 让 iOS / dev 联调免 CORS；
// 节点 9+ prod launch 阶段改成白名单，与 RateLimit 切 Redis 同期）。
func NewGateway(
	signer *auth.Signer,
	mgr SessionManager,
	roomMember mysql.RoomMemberRepo,
	cfg config.WSConfig,
) *Gateway {
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
// 校验全通过后**严格按以下顺序**执行（V1 §12.1 钦定，禁止调换）：
//
//  1. upgrader.Upgrade 升级到 WS 协议（已在 step 2 / 3 失败路径中提前 Upgrade
//     才能写 close frame；happy path 不需要在这里再 Upgrade —— 实装上 V1 §12.1
//     close code 表钦定校验失败必须先 Upgrade，所以 Handle 内一进 handler 立即
//     Upgrade，再走校验链路）
//  2. 创建 Session 对象（newSession）
//  3. mgr.Register(ctx, session)（触发 OnSessionRegister 钩子）
//  4. **同步段**写 placeholder room.snapshot：
//     - 调 roomMemberRepo.ListMembers(ctx, roomID) 拿全成员行
//     - 构造 placeholder snapshot（payload.room.{id, maxMembers=4, memberCount=len(members)}
//     + payload.members[]：每行 {userId, nickname:"", pet:{petId:"", currentState:1}}）
//     - 调 conn.WriteMessage(TextMessage, snapshotBytes)；失败 → close 1011
//     reason "snapshot build failed"，**不**启动读/写 goroutine
//  5. go session.readLoop() 启动读 goroutine
//  6. go session.writeLoop() 启动写 goroutine
//
// **关键反模式**（不要做）：
//   - 不在 Upgrade 之前发 close code（HTTP 403 是错的；V1 §12.1 钦定校验失败
//     必须**先 Upgrade 成功** 再发 close frame）
//   - 不在 readLoop 启动后再写 snapshot（窗口期 client 可能发 ping，server 已
//     有读 goroutine → server 可能先回 pong 让 snapshot 不再是第一条）
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

	// 校验全通过 → V1 §12.1 5 步握手成功流程
	// 6.1 创建 Session（sessionID 暂为空，Register 内回填）
	session := newSession("", userID, roomID, conn, g.logger, g.cfg.MaxMessageSizeBytes, g.writeTimeout)

	// 6.2 Register（触发 OnSessionRegister 钩子；ErrSessionReplaced 是合法路径
	// 不阻塞握手）
	sessionID, regErr := g.mgr.Register(ctx, session)
	if regErr != nil && !errors.Is(regErr, ErrSessionReplaced) {
		g.closeWithCode(conn, 1011, "session register failed")
		g.logger.Error("ws handshake: session register failed", slog.Any("error", regErr))
		return
	}
	if errors.Is(regErr, ErrSessionReplaced) {
		g.logger.Info("ws session replaced previous connection",
			slog.String("sessionID", sessionID), slog.Uint64("userID", userID), slog.Uint64("roomID", roomID))
	}

	// 6.3 同步段构造 + 写 placeholder room.snapshot
	members, err := g.roomMember.ListMembers(ctx, roomID)
	if err != nil {
		g.closeWithCode(conn, 1011, "snapshot build failed")
		g.logger.Error("ws handshake: ListMembers failed",
			slog.Uint64("roomID", roomID), slog.Any("error", err))
		// 已 Register；显式 Unregister 让钩子退出（避免泄漏）。Session 结构占用最小，
		// 不需要再单独 Close —— defer conn.Close() 已经会触发 readLoop 的 ReadMessage
		// 失败链路；但本 story 阶段 readLoop 还没启动，安全起见显式调 Session.Close。
		_ = session.Close()
		return
	}
	snapshotBytes, err := buildPlaceholderSnapshot(roomID, members)
	if err != nil {
		g.closeWithCode(conn, 1011, "snapshot build failed")
		g.logger.Error("ws handshake: build snapshot failed",
			slog.Uint64("roomID", roomID), slog.Any("error", err))
		_ = session.Close()
		return
	}
	// V1 §12.1.3 钦定 snapshot 是握手成功后**必发**的第一条 authoritative 消息；
	// 必须**同步**写入 conn（不走异步 sendChan）—— writeLoop 此时还没启动。
	if g.writeTimeout > 0 {
		_ = conn.SetWriteDeadline(time.Now().Add(g.writeTimeout))
	}
	if err := conn.WriteMessage(websocket.TextMessage, snapshotBytes); err != nil {
		// 写 snapshot 失败 → 不再启动 goroutine；按 V1 §12.1 close 1011 reason
		// "snapshot build failed"
		g.closeWithCode(conn, 1011, "snapshot build failed")
		g.logger.Error("ws handshake: snapshot write failed",
			slog.Uint64("roomID", roomID), slog.Any("error", err))
		_ = session.Close()
		return
	}

	// 6.4 启动读 / 写 goroutine（snapshot 已经在 wire 上）
	go session.readLoop()
	go session.writeLoop()

	g.logger.Info("ws handshake completed",
		slog.String("sessionID", sessionID),
		slog.Uint64("userID", userID),
		slog.Uint64("roomID", roomID),
		slog.Int("memberCount", len(members)),
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

// snapshotPayload 是 V1 §12.3 room.snapshot payload 的 Go 结构。
type snapshotPayload struct {
	Room    snapshotRoom     `json:"room"`
	Members []snapshotMember `json:"members"`
}

type snapshotRoom struct {
	ID          string `json:"id"`
	MaxMembers  int    `json:"maxMembers"`
	MemberCount int    `json:"memberCount"`
}

type snapshotMember struct {
	UserID   string      `json:"userId"`
	Nickname string      `json:"nickname"`
	Pet      snapshotPet `json:"pet"`
}

type snapshotPet struct {
	PetID        string `json:"petId"`
	CurrentState int    `json:"currentState"`
}

// buildPlaceholderSnapshot 构造 V1 §12.3 节点 4 placeholder snapshot
// （Story 10.7 真实 SnapshotBuilder 接管前的 inline 实装）。
//
// 字段语义（V1 §12.3 钦定）：
//   - room.id: roomID 字符串化（V1 §2.5 全局约定 BIGINT 字符串化）
//   - room.maxMembers: 节点 4 阶段固定 4
//   - room.memberCount: len(members)（与 members[] 数组长度严格相等，V1 §12.3
//     "不变量" 钦定）
//   - members[].userId: 字符串化
//   - members[].nickname: ""（placeholder 阶段，V1 §12.3 钦定空字符串语义 = "
//     server 不知道"，client 按 merge contract 保留已有值；Story 11.7 真实
//     SnapshotBuilder 接管时由 users.nickname 真实回填）
//   - members[].pet.petId: ""（placeholder；同上）
//   - members[].pet.currentState: 1（节点 4 阶段固定，V1 §12.3 钦定；Epic 14
//     真实驱动）
//
// **不变量**：memberCount == len(members) —— 节点 4 阶段 placeholder 必须严格
// 反映 room_members 全表行数（V1 §12.3 不变量小节钦定，禁止"全零 placeholder"
// 或"单成员快照"）。
func buildPlaceholderSnapshot(roomID uint64, members []uint64) ([]byte, error) {
	memberList := make([]snapshotMember, 0, len(members))
	for _, uid := range members {
		memberList = append(memberList, snapshotMember{
			UserID:   strconv.FormatUint(uid, 10),
			Nickname: "",
			Pet: snapshotPet{
				PetID:        "",
				CurrentState: 1,
			},
		})
	}
	env := serverEnvelope{
		Type:      "room.snapshot",
		RequestID: "", // V1 §12.3 钦定主动推送类固定 ""
		Payload: snapshotPayload{
			Room: snapshotRoom{
				ID:          strconv.FormatUint(roomID, 10),
				MaxMembers:  4,
				MemberCount: len(memberList),
			},
			Members: memberList,
		},
		Ts: time.Now().UnixMilli(),
	}
	bytes, err := json.Marshal(env)
	if err != nil {
		// json.Marshal 在 marshalable struct 下不可能失败；防御性 wrap
		return nil, fmt.Errorf("ws: marshal snapshot: %w", err)
	}
	return bytes, nil
}

// 编译时接口断言：确保 RoomMemberRepo 是 mysql.RoomMemberRepo
// （让 Gateway struct 字段定义在编译期被 type system 校验）
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
