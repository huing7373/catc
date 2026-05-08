package handler

import (
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/middleware"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// RoomHandler 是 /api/v1/rooms/* 路由的 handler 集合。
//
// 节点 4 阶段：CreateRoom（POST /rooms，Story 11.3）；
// future Story 11.4 加 Join（POST /rooms/{roomId}/join）；
// Story 11.5 加 Leave（POST /rooms/{roomId}/leave）；
// Story 11.6 加 GetCurrent / GetDetail。
type RoomHandler struct {
	svc service.RoomService
}

// NewRoomHandler 构造 RoomHandler。注入 RoomService（service 层 interface）—— handler
// 单测直接传 stub struct 实现该 interface，不需要起 *gorm.DB / 真 mysql。
func NewRoomHandler(svc service.RoomService) *RoomHandler {
	return &RoomHandler{svc: svc}
}

// CreateRoomResponseRoom 是 V1 §10.1 钦定的 wire DTO（room 子对象）。
//
// **关键**（V1 §2.5 BIGINT 字符串化全局约定）：
//   - id / creatorUserId 是 BIGINT → 字符串化（避免 JS Number.MAX_SAFE_INTEGER 精度丢失）
//   - maxMembers / memberCount / status 是数值字段（非 BIGINT）→ 保 number
//
// 用 strconv.FormatUint 而非 fmt.Sprintf("%d", ...)：更快 + 不依赖 fmt reflect。
type CreateRoomResponseRoom struct {
	ID            string `json:"id"`            // BIGINT 字符串化
	CreatorUserID string `json:"creatorUserId"` // BIGINT 字符串化
	MaxMembers    int    `json:"maxMembers"`    // number (int)
	MemberCount   int    `json:"memberCount"`   // number (int)
	Status        int    `json:"status"`        // number (int)
}

// CreateRoomResponseData 是 V1 §10.1 钦定的 wire DTO（data 顶层结构）。
type CreateRoomResponseData struct {
	Room CreateRoomResponseRoom `json:"room"`
}

// CreateRoom 处理 POST /api/v1/rooms。
//
// # 流程
//
//  1. 取 caller userID（auth 中间件已注入到 c.Keys）—— 缺失走 1009 unreachable 兜底
//  2. **不**做 ShouldBindJSON（V1 §10.1 钦定请求体为空对象 {}；client **不**传任何
//     业务字段；handler 接受 nil body / `{}` / 任意 JSON 内容）
//  3. 调 svc.CreateRoom(ctx, CreateRoomInput{UserID: userID}) —— ctx = c.Request.Context()
//     （ADR-0007 §2.2，**不**用 *gin.Context 当 ctx）
//  4. 成功 → response.Success(c, dto, "ok") + 业务事件 log "room.created"
//  5. 失败 → c.Error(err) + return（让 ErrorMappingMiddleware 写 envelope）
//
// # ADR-0006 单一 envelope 生产者
//
// 本 handler **不**直接调 response.Error 写 6003 / 1009 envelope —— 一律走 c.Error +
// return，由 ErrorMappingMiddleware 兜底翻译成 envelope。
//
// # 反模式（已避免）
//
//   - **不**消费 idempotencyKey header / 字段（V1 §10.1 钦定本接口非幂等；每次调用都
//     创建新房间，"用户已在房间中"由 6003 兜底）
//   - **不**触发 WS 广播（V1 §10.1 钦定本接口不广播 member.joined —— 房间刚创建只有
//     创建者一人，无其他在线成员需要被通知；任何 WS 调用都属范围越界）
//   - **不**做 GET 风格的 ShouldBindQuery（POST 请求体；future Story 加业务字段时再补
//     ShouldBindJSON）
func (h *RoomHandler) CreateRoom(c *gin.Context) {
	// 从 auth 中间件取 userID（与 home_handler.LoadHome / steps_handler.PostSync 同模式）
	v, ok := c.Get(middleware.UserIDKey)
	if !ok {
		// unreachable: Auth 中间件挂在前；保险兜底走 1009
		_ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
		return
	}
	userID, ok := v.(uint64)
	if !ok {
		// unreachable: Auth 中间件 c.Set(UserIDKey, claims.UserID) 永远是 uint64
		_ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
		return
	}

	ctx := c.Request.Context()
	out, err := h.svc.CreateRoom(ctx, service.CreateRoomInput{UserID: userID})
	if err != nil {
		_ = c.Error(err) // service 已 wrap *AppError；ErrorMappingMiddleware 写 envelope
		return
	}

	// 业务事件 log（与 auth_handler / steps_handler 同模式；让运维聚合
	// `count(msg=room.created)` 监控房间创建活跃度）。
	// msg "room.created" 是稳定 audit anchor —— Story 11.4 / 11.5 演进时新增的
	// "room.joined" / "room.left" 业务事件命名延续此模式。
	slog.InfoContext(ctx, "room.created",
		slog.Uint64("user_id", userID),
		slog.Uint64("room_id", out.RoomID),
		slog.Int("member_count", out.MemberCount),
	)

	response.Success(c, createRoomResponseDTO(out), "ok")
}

// createRoomResponseDTO 把 service 输出转成 V1 §10.1 钦定的 wire 格式。
//
// 字段映射（与 V1 §10.1 行 1145 钦定字段表 1:1 对齐）：
//   - data.room.id           = strconv.FormatUint(out.RoomID, 10)         // BIGINT → string
//   - data.room.creatorUserId = strconv.FormatUint(out.CreatorUserID, 10) // BIGINT → string
//   - data.room.maxMembers   = int(out.MaxMembers)                        // uint8 → int
//   - data.room.memberCount  = out.MemberCount                            // 业务规则保证 == 1
//   - data.room.status       = int(out.Status)                            // int8 → int
func createRoomResponseDTO(out *service.CreateRoomOutput) CreateRoomResponseData {
	return CreateRoomResponseData{
		Room: CreateRoomResponseRoom{
			ID:            strconv.FormatUint(out.RoomID, 10),
			CreatorUserID: strconv.FormatUint(out.CreatorUserID, 10),
			MaxMembers:    int(out.MaxMembers),
			MemberCount:   out.MemberCount,
			Status:        int(out.Status),
		},
	}
}
