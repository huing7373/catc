package handler

import (
	"strconv"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// 长度上限常量（V1 §4.1 行 144-152 钦定，按 utf8.RuneCountInString 字符数）。
//
// 用 utf8.RuneCountInString 而非 len()：V1 §2.5 钦定按 Unicode 字符数计算长度
// （utf8mb4 1 字符可能 4 字节，len() 误判会拒绝合法多字节输入）。
const (
	guestUIDMinLen    = 1
	guestUIDMaxLen    = 128
	appVersionMinLen  = 1
	appVersionMaxLen  = 32
	deviceModelMinLen = 1
	deviceModelMaxLen = 64
)

// AuthHandler 是 /auth/* 子组的 handler 集合。
//
// 节点 2 阶段仅 GuestLogin（POST /api/v1/auth/guest-login）；future epic 加
// BindWechat（V1 §4.2）/ Refresh 等。
type AuthHandler struct {
	svc service.AuthService
}

// NewAuthHandler 构造 AuthHandler。
//
// 注入 AuthService（service 层 interface）—— handler 单测直接传 stub struct
// 实现该 interface，不需要起 *gorm.DB / 真 mysql。
func NewAuthHandler(svc service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// GuestLoginRequest 是 V1 §4.1 钦定请求体的 Go 端 mirror。
//
// JSON tag 与 V1 §4.1 行 144-152 钦定字段名严格对齐。
//
// **不**用 Gin binding 的 min / max 标签做长度校验：
//   - validator/v10 的 min/max 在 string 上是按字节数；V1 §2.5 钦定按字符数
//   - 错误信息英文且不可控；手动校验后用 apperror.New(ErrInvalidParam, "<中文具体描述>")
//     给客户端可读文案
//
// **手动 platform enum 校验**而非 binding:"oneof=ios android"：跨 reviewer 一目了然，
// 不依赖 validator 标签语法的隐式行为。
type GuestLoginRequest struct {
	GuestUID string           `json:"guestUid" binding:"required"`
	Device   GuestLoginDevice `json:"device" binding:"required"`
}

// GuestLoginDevice 是请求体的 device 子结构。
//
// V1 §4.1 行 150-152 钦定的 platform / appVersion / deviceModel 三字段。
type GuestLoginDevice struct {
	Platform    string `json:"platform" binding:"required"`
	AppVersion  string `json:"appVersion" binding:"required"`
	DeviceModel string `json:"deviceModel" binding:"required"`
}

// GuestLogin 处理 POST /api/v1/auth/guest-login。
//
// # 流程
//
//  1. ShouldBindJSON 解析 + Gin binding:"required" 兜一层（字段缺失 → 1002）
//  2. **手动**校验长度（utf8.RuneCountInString）+ platform enum
//  3. 调 svc.GuestLogin(ctx, GuestLoginInput) —— ctx 来自 c.Request.Context()
//     （ADR-0007 §2.2 钦定，**不**用 *gin.Context 当 ctx）
//  4. 成功 → response.Success(c, dto, "ok")
//  5. 失败 → c.Error(err) + return（让 ErrorMappingMiddleware 写 envelope）
//
// # ADR-0006 单一 envelope 生产者
//
// 本 handler **不**直接调 response.Error 写 1002 / 1009 envelope —— 一律走
// c.Error + return，由 ErrorMappingMiddleware 兜底翻译成 envelope。
// 见 docs/lessons/2026-04-24-error-envelope-single-producer.md。
//
// # 反模式（已避免）
//
//   - **不**用 c.JSON(http.StatusBadRequest, ...) 直接写 400：V1 §2.4 钦定业务码与
//     HTTP status 正交（1002 走 200 + envelope.code=1002）
//   - **不**做"先调 svc.FindByGuestUID 再决定调 svc.Create" 拆分：判断必须在 service 内做
//   - **不**接 idempotencyKey header：V1 §4.1 钦定靠 DB UNIQUE 自然幂等
//   - **不**返回 device 信息：V1 §4.1 response 只含 token / user / pet 三块
func (h *AuthHandler) GuestLogin(c *gin.Context) {
	var req GuestLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// BindJSON 失败 → 字段缺失 / 类型错；映射 1002
		_ = c.Error(apperror.Wrap(err, apperror.ErrInvalidParam, apperror.DefaultMessages[apperror.ErrInvalidParam]))
		return
	}

	// 长度校验（utf8.RuneCountInString 按 Unicode 字符数，V1 §2.5 钦定）
	if n := utf8.RuneCountInString(req.GuestUID); n < guestUIDMinLen || n > guestUIDMaxLen {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "guestUid 长度必须在 1-128 字符"))
		return
	}
	if n := utf8.RuneCountInString(req.Device.AppVersion); n < appVersionMinLen || n > appVersionMaxLen {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "appVersion 长度必须在 1-32 字符"))
		return
	}
	if n := utf8.RuneCountInString(req.Device.DeviceModel); n < deviceModelMinLen || n > deviceModelMaxLen {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "deviceModel 长度必须在 1-64 字符"))
		return
	}

	// platform 枚举校验：节点 2 阶段仅"ios"在用，但 schema 占位 android 让 future
	// 端接入零改动（V1 §4.1 行 150 行声明 enum 含 android）。
	switch req.Device.Platform {
	case "ios", "android":
		// OK
	default:
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "platform 必须是 ios 或 android"))
		return
	}

	out, err := h.svc.GuestLogin(c.Request.Context(), service.GuestLoginInput{
		GuestUID:    req.GuestUID,
		Platform:    req.Device.Platform,
		AppVersion:  req.Device.AppVersion,
		DeviceModel: req.Device.DeviceModel,
	})
	if err != nil {
		// service 已 wrap apperror；handler 只透传 c.Error 让 ErrorMappingMiddleware 写 envelope
		_ = c.Error(err)
		return
	}

	response.Success(c, guestLoginResponseDTO(out), "ok")
}

// guestLoginResponseDTO 把 service 输出转成 V1 §4.1 钦定的 wire 格式。
//
// **关键**：BIGINT id 必须按 V1 §2.5 转字符串（避免 JS Number.MAX_SAFE_INTEGER 精度丢失）。
// 用 strconv.FormatUint 而非 fmt.Sprintf("%d", ...)：更快 + 不依赖 fmt reflect。
func guestLoginResponseDTO(out *service.GuestLoginOutput) gin.H {
	return gin.H{
		"token": out.Token,
		"user": gin.H{
			"id":             strconv.FormatUint(out.UserID, 10),
			"nickname":       out.Nickname,
			"avatarUrl":      out.AvatarURL,      // 首次创建为 ""（V1 §4.1 行 183）
			"hasBoundWechat": out.HasBoundWechat, // 游客首次为 false（V1 §4.1 行 184）
		},
		"pet": gin.H{
			"id":      strconv.FormatUint(out.PetID, 10),
			"petType": out.PetType, // 节点 2 固定 1（V1 §4.1 行 186）
			"name":    out.PetName, // 首次创建 "默认小猫"（V1 §4.1 行 187）
		},
	}
}
