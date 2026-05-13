// Package ws 内 emoji_handler.go：WS emoji.send 消息的服务端 handler
// （Story 17.5 引入；首次落地 client → server 业务消息路由 + WS error envelope
// 推送路径）。
//
// 角色：
//   - EmojiHandler interface：抽象 HandleEmojiSend 方法，让 Session 注入 stub
//     struct 单测；与既有 HomeService / RoomService / EmojiService 同模式
//   - emojiHandlerImpl 默认实装：在 §12.2 服务端逻辑步骤 1-5 串行流程内调
//     EmojiService.ValidateCode + UserRepo.FindByID + BroadcastFn
//   - 错误响应走 BuildErrorEnvelope + Session.SendPriority（priority chan 防业务
//     buffer 满载时丢失 error；与 handlePing 一致），requestId 回带 client 的
//     emoji.send.requestId
//   - 广播成功路径走 BuildEmojiReceivedEnvelope + BroadcastFn 全 fanout（**包含**
//     发起者自己，与 pet.state.changed 同语义；与 member.joined / left 排除发起者
//     不同语义）
//   - **永远不返 error**（fire-and-forget 入口；V1 §12.2 钦定 server 处理成功无
//     server → client ack 消息；处理失败也已通过 Session.SendPriority 推 error
//     消息到 client，无需透传 error 给 caller）
//
// 与 11.8 / 14.4 broadcast 路径同模式：本 handler **同步**调 broadcastFn（WS
// Session 生命周期 ≥ broadcast 时长 + broadcast 是 O(1) 入队操作 + 不需要 detached
// ctx）；lesson 2026-05-12-detached-ctx-for-async-broadcast 不适用本路径（该
// lesson 针对 HTTP handler 路径）。
package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// EmojiValidator 是 emoji_handler 对 service 层 EmojiService.ValidateCode 方法
// 的本包局部接口（Story 17.5 引入；用于打破 service → ws 的 import 反向依赖）。
//
// **为什么不直接 import service.EmojiService**：service 包已 import 本包 ws（如
// pet_service / room_service 用 ws.BroadcastFn / ws.BuildPetStateChangedEnvelope
// 等），ws 再反向 import service 会触发 Go import cycle。Go 推荐的解法是
// "consumer-defined interface"：消费方按需声明最小接口，生产方实现该接口（duck
// typing）。service.EmojiService 自身已含 ValidateCode 方法签名，本接口仅是其
// 一个子集 —— bootstrap wire 阶段把 service.NewEmojiService(...) 产物直接传给
// NewEmojiHandler 即可（编译期自动满足 EmojiValidator 接口）。
//
// **签名**：必须与 service.EmojiService.ValidateCode 完全一致（含 error 类型，
// 返 *apperror.AppError）；任何漂移会让 wire 编译失败。
type EmojiValidator interface {
	ValidateCode(ctx context.Context, code string) error
}

// EmojiHandler 是 WS emoji.send 消息的服务端 handler（Story 17.5 引入）。
//
// **interface 而非具体类型**：让 Session 注入 stub struct 单测，与 service 层
// HomeService / RoomService 同模式。
//
// 节点 6 阶段仅 HandleEmojiSend（emoji.send）；future epic 加 HandleEmojiBatch /
// HandleEmojiCancel 等扩展（不在 MVP 范围）。
//
// **nil-safe 约定**：Session.emojiHandler 可为 nil（单测 / HTTP-only 部署）；
// readLoop 在 dispatch 前显式 `if s.emojiHandler != nil` check，nil 时 log warn
// + continue 走 unknown type 路径，与既有 dispatcher 行为一致。
type EmojiHandler interface {
	// HandleEmojiSend 处理 client 发来的 emoji.send 消息（V1 §12.2 钦定）。
	//
	// **入参**：
	//   - ctx:     session.ctx 或调用方 ctx；用于 service / repo 调用链路
	//   - session: 发起 emoji.send 的 Session（提供 UserID / RoomID / SendPriority 三能力）
	//   - env:     已解析的 clientEnvelope（type/requestId/payload）；payload 仍是
	//              json.RawMessage 由本方法二次解析 emojiCode 字段
	//
	// **返回**：无（fire-and-forget 入口 —— V1 §12.2 钦定 server 处理成功无
	// server → client ack 消息；处理失败也已通过 Session.SendPriority 推 error
	// 消息到 client，无需透传 error 给 caller）。
	HandleEmojiSend(ctx context.Context, session *Session, env clientEnvelope)
}

// emojiHandlerImpl 是 EmojiHandler 的默认实装。
type emojiHandlerImpl struct {
	svc         EmojiValidator
	userRepo    mysql.UserRepo
	broadcastFn BroadcastFn
}

// NewEmojiHandler 构造 EmojiHandler。
//
// 注入：
//   - svc:         EmojiValidator（本包局部接口，service.EmojiService 自动满足）—— 单测可注入 stub
//   - userRepo:    UserRepo（17.5 用 FindByID 拿 users.current_room_id）
//   - broadcastFn: WS 广播函数值（10.5 落地的 BroadcastFn type alias）；
//                  bootstrap wire 阶段传 `wsapp.BroadcastFn(func(...) { ... wsapp.BroadcastToRoom(...) })`
//                  closure；单测直接传 capture closure 记录调用次数 / 入参
//
// **不**注入 SessionManager：handler 不直接读 sessionsByRoom / 不主动 close 任何
// session；fanout 由 broadcastFn closure 内部委托给 BroadcastToRoom 完成（与
// 14.4 broadcastPetStateChanged 同模式）。
func NewEmojiHandler(svc EmojiValidator, userRepo mysql.UserRepo, broadcastFn BroadcastFn) EmojiHandler {
	return &emojiHandlerImpl{
		svc:         svc,
		userRepo:    userRepo,
		broadcastFn: broadcastFn,
	}
}

// emojiSendPayload 是 client → server `emoji.send` payload 的解码 struct（V1 §12.2
// 字段表 emoji.send.payload）。
//
// 字段：
//   - EmojiCode: 必填；service.ValidateCode 二次校验字符集 / 长度 + 存在性
type emojiSendPayload struct {
	EmojiCode string `json:"emojiCode"`
}

// HandleEmojiSend 主流程（V1 §12.2 服务端逻辑步骤 1-5 严格对齐 + 17.1 r1 review
// 锁定的反 stale-Session 跨房间双校验）：
//
//  1. **接收 & 解析**：caller（Session.readLoop）已按 type=="emoji.send" 路由进来
//     并解析顶层 envelope；本方法再解析 payload.emojiCode（json.Unmarshal 失败 →
//     回 error 1002 + return）
//  2. **参数校验 + 表情存在性校验**：调 svc.ValidateCode（service.ValidateCode 内
//     合并字符集 / 长度 + Exists 三段校验）；失败按 apperror.Code 分流为 1002 /
//     1009 / 7001 → 回 error 消息 + return
//  3. **房间归属双校验**（V1 §12.2 服务端逻辑步骤 3 + 17.1 r1 锁定）：调
//     userRepo.FindByID 拿 users.current_room_id 与 session.RoomID() 比对：
//       - DB error → 回 error 1009 + return
//       - current_room_id == nil → 回 error 6004 + return
//       - *current_room_id != session.RoomID() → 回 error 6004 + log warn 含
//         stale Session 三字段（userId / Session.roomID / users.current_room_id）+ return
//  4. **广播（fire-and-forget）**：构造 EmojiReceivedPayload + BuildEmojiReceivedEnvelope
//     + broadcastFn(ctx, session.RoomID(), msg) 全 fanout（**包含**发起者自己 ——
//     V1 §12.3 钦定广播范围）；广播失败仅 log warn 不回 error 给发起者
//
// **关键差异 vs HTTP handler**（home_handler / room_handler / pets_handler）：
//   - HTTP handler 通过 `c.Error(err) + return` 让 ErrorMappingMiddleware 翻译；
//     WS handler 通过 `BuildErrorEnvelope + session.SendPriority` 直接推回 client
//     （V1 §12.2 钦定"错误响应通过 §12.3 error 消息回送"，与 HTTP envelope 完全
//     独立路径）
//   - HTTP handler 用 c.Request.Context() 取 ctx；WS handler 用 caller 传入 ctx
//     （readLoop 用 session.ctx；具体由 readLoop 决定）
func (h *emojiHandlerImpl) HandleEmojiSend(ctx context.Context, session *Session, env clientEnvelope) {
	logger := slog.Default().With(
		slog.String("component", "ws-emoji-handler"),
		slog.String("event", "emoji.send"),
		slog.Uint64("userId", session.UserID()),
		slog.Uint64("sessionRoomId", session.RoomID()),
		slog.String("requestId", env.RequestID),
	)

	// (1) 解析 payload.emojiCode
	//
	// **注**：payload 缺 emojiCode 字段（如 `{}`）→ json.Unmarshal 成功但
	// payload.EmojiCode = ""，由后续 svc.ValidateCode 字符集 / 长度校验拦截返
	// 1002；只有 payload 本身非 JSON object 时 Unmarshal 才会返 err。
	var payload emojiSendPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		logger.Warn("payload unmarshal failed", slog.Any("error", err))
		h.sendErrorToSession(session, env.RequestID, apperror.ErrInvalidParam,
			apperror.DefaultMessages[apperror.ErrInvalidParam], logger)
		return
	}

	// (2) 参数校验 + 表情存在性校验（service.ValidateCode 内合并 §12.2 步骤 2 + 4）
	if err := h.svc.ValidateCode(ctx, payload.EmojiCode); err != nil {
		var appErr *apperror.AppError
		if !errors.As(err, &appErr) {
			// 防御性兜底：service 层契约保证返 *AppError；非 AppError → 包成 1009
			logger.Error("ValidateCode returned non-AppError; wrapping as 1009",
				slog.Any("error", err))
			h.sendErrorToSession(session, env.RequestID, apperror.ErrServiceBusy,
				apperror.DefaultMessages[apperror.ErrServiceBusy], logger)
			return
		}
		logger.Info("emoji.send rejected by ValidateCode",
			slog.Int("code", appErr.Code),
			slog.String("emojiCode", payload.EmojiCode))
		h.sendErrorToSession(session, env.RequestID, appErr.Code, appErr.Message, logger)
		return
	}

	// (3) 房间归属双校验（V1 §12.2 服务端逻辑步骤 3 + r1 review 锁定的反 stale-Session 校验）
	//
	// **权威源**：session.RoomID() 来自 WS 握手 path /ws/rooms/{roomId}（gateway
	// 校验阶段已通过 IsUserInRoom 验证过当时 user 确实在该 room）。但 stale Session
	// 在 user 离开/重新加入其他 room 后仍可能存活（HTTP leave 才触发 Unregister；
	// 网络抖动等场景可能让 OS-level WS 连接残留，但 SessionManager 仍持有）。本步骤
	// 通过 users.current_room_id 拿"DB 权威态"与 Session.roomID 比对，拦截 stale。
	user, err := h.userRepo.FindByID(ctx, session.UserID())
	if err != nil {
		// user 不存在不是合法状态（token 已校验过 user.id）；任何 err（含
		// ErrUserNotFound）都视为 1009 服务异常。
		logger.Error("FindByID failed", slog.Any("error", err))
		h.sendErrorToSession(session, env.RequestID, apperror.ErrServiceBusy,
			apperror.DefaultMessages[apperror.ErrServiceBusy], logger)
		return
	}
	if user.CurrentRoomID == nil {
		logger.Info("user not in any room", slog.String("emojiCode", payload.EmojiCode))
		h.sendErrorToSession(session, env.RequestID, apperror.ErrUserNotInRoom,
			apperror.DefaultMessages[apperror.ErrUserNotInRoom], logger)
		return
	}
	if *user.CurrentRoomID != session.RoomID() {
		// stale Session 跨房间注入 —— V1 §12.2 + r1 review 锁定的拦截路径
		logger.Warn("cross-room emoji.send blocked (stale Session)",
			slog.Uint64("usersCurrentRoomId", *user.CurrentRoomID),
			slog.String("emojiCode", payload.EmojiCode))
		h.sendErrorToSession(session, env.RequestID, apperror.ErrUserNotInRoom,
			apperror.DefaultMessages[apperror.ErrUserNotInRoom], logger)
		return
	}

	// (4) 广播 emoji.received（fire-and-forget；**包含**发起者自己 —— V1 §12.3
	// 钦定广播范围，与 pet.state.changed 同语义，与 member.joined / member.left
	// 排除发起者**不同**语义。广播目标 = Session.RoomID()，§12.2 步骤 5 钦定的
	// 权威源，二者在步骤 3 已校验相等）。
	broadcastPayload := EmojiReceivedPayload{
		UserID:    strconv.FormatUint(session.UserID(), 10), // BIGINT 字符串化（V1 §2.5）
		EmojiCode: payload.EmojiCode,
	}
	msg, err := BuildEmojiReceivedEnvelope(broadcastPayload)
	if err != nil {
		// marshal 失败极罕见（payload 全是 string）；防御性 log warn 不回 error
		// 给 client（fire-and-forget 边界）
		logger.Warn("BuildEmojiReceivedEnvelope failed", slog.Any("error", err))
		return
	}

	sent, err := h.broadcastFn(ctx, session.RoomID(), msg)
	if err != nil {
		// fire-and-forget：广播失败仅 log warn，**不**回 error 给发起者（V1
		// §12.2 钦定 + 与 11.8 / 14.4 同模式）。**不** persistence（V1 §14.3
		// 钦定 emoji 默认不持久化）。
		logger.Warn("broadcast emoji.received failed",
			slog.Int("targetSessions", sent),
			slog.Any("error", err))
		return
	}
	logger.Info("emoji.received broadcast sent",
		slog.Int("targetSessions", sent),
		slog.String("emojiCode", payload.EmojiCode))
}

// sendErrorToSession 是 BuildErrorEnvelope + session.SendPriority 的小工具（私有方法）。
//
// **走 SendPriority 而非 Send**：error 是 protocol-level msg，走 priority chan
// 让业务 buffer 满载时仍能投递（与 handlePing 走 SendPriority 同模式 —— session.go
// 注释钦定 "protocol-level msg 走 priority"）。
//
// **fire-and-forget**：marshal / Send 失败仅 log warn，不返 error 给 caller —— 在
// caller HandleEmojiSend 已是 fire-and-forget 入口的语义下，error 推送本身的失败
// 也无意义（client 可能已断开）。
func (h *emojiHandlerImpl) sendErrorToSession(session *Session, requestID string, code int, message string, logger *slog.Logger) {
	msg, err := BuildErrorEnvelope(requestID, code, message)
	if err != nil {
		logger.Warn("BuildErrorEnvelope failed",
			slog.Int("code", code),
			slog.Any("error", err))
		return
	}
	if err := session.SendPriority(msg); err != nil {
		logger.Warn("send error envelope failed",
			slog.Int("code", code),
			slog.Any("error", err))
	}
}
