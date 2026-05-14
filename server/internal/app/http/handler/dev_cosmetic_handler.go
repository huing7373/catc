package handler

import (
	"github.com/gin-gonic/gin"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// DevCosmeticHandler 是 /dev/grant-cosmetic-batch 等 dev 装扮端点的 handler 集合（Story 20.8）。
//
// 与 ChestHandler (20.5 / 20.6) / DevStepsHandler (7.5) / DevChestHandler (20.7) 区分：
//   - DevCosmeticHandler 处理 /dev/grant-cosmetic-batch（dev 工具；不含 auth / rate_limit / 事务）
//   - 与 DevStepsHandler / DevChestHandler 平级：dev 工具按"业务模块"独立 handler，让未来加
//     /dev/grant-cosmetic-by-id 或其他 cosmetic 相关 dev 端点时有独立 handler 槽位，避免单文件膨胀。
//
// **节点 7 阶段 stub**：handler 路径完整（参数校验 / DTO / response 全实装），底层 service 是 stub。
//
//	节点 8 激活时 handler **不**改 —— service 内部从 stub 切真实写库即可。
type DevCosmeticHandler struct {
	svc service.DevCosmeticService
}

// NewDevCosmeticHandler 构造 DevCosmeticHandler。
func NewDevCosmeticHandler(svc service.DevCosmeticService) *DevCosmeticHandler {
	return &DevCosmeticHandler{svc: svc}
}

// PostGrantCosmeticBatchRequest 是 POST /dev/grant-cosmetic-batch 请求体的 Go mirror。
//
// epics.md §Story 20.8 行 2962 钦定：`{userId, rarity, count}`。
//
// **userId 用 *uint64 指针类型** + **rarity 用 *int8 指针类型** + **count 用 *int32 指针类型**：
//   - validator/v10 把 0 视为 zero value 会误判 "required"，与 7.5 PostGrantStepsRequest / 20.7
//     PostForceUnlockChestRequest 同模式
//   - 用指针 + handler 显式 nil 校验区分 "字段缺失" vs "显式传 0"
//   - userId=0 / rarity=0 / count=0 在业务上都是非法值，handler 显式拒让错误更早 + 错误消息更精确
//
// **rarity 范围**（数据库设计.md §6.9）：1=common / 2=rare / 3=epic / 4=legendary —— handler 校验 ∈ [1,4]
// **count 范围**：1 ≤ count ≤ 100 —— handler 校验；上限 100 防 demo 误传 1e6 砸 DB（节点 11 合成 demo 凑 10 件 common 用 count=10 / 12 即可）
//
// **不**接 cosmeticItemId 字段（dev 产品语义是"按品质随机抽"，不是"指定 cosmetic 发放"）。
// **不**接 idempotencyKey 字段（dev 端点是"故意可重复"语义）。
type PostGrantCosmeticBatchRequest struct {
	UserID *uint64 `json:"userId"`
	Rarity *int8   `json:"rarity"`
	Count  *int32  `json:"count"`
}

// PostGrantCosmeticBatch 处理 POST /dev/grant-cosmetic-batch（Story 20.8 节点 7 阶段 stub）。
//
// 流程：
//  1. ShouldBindJSON 兜一层（字段类型错 → 1002）
//  2. 手动校验：
//     - userId 非 nil + != 0（1002）
//     - rarity 非 nil + ∈ [1,4]（1002）
//     - count 非 nil + ∈ [1,100]（1002）
//  3. 调 svc.GrantCosmeticBatch(ctx, *userId, *rarity, *count) —— ctx = c.Request.Context()
//     **节点 7 阶段 service 是 stub return nil**；handler 与 stub / 激活后行为完全兼容
//  4. 成功 → response.Success(c, postGrantCosmeticBatchResponseDTO(...), "ok")
//  5. 失败 → c.Error(err) + return（middleware envelope；ADR-0006 单一 envelope 生产者）
//
// **不**做 auth 校验（dev 端点不要求 auth）；**不**取 c.Get(UserIDKey)（dev 路径无 auth 中间件）。
func (h *DevCosmeticHandler) PostGrantCosmeticBatch(c *gin.Context) {
	var req PostGrantCosmeticBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.Wrap(err, apperror.ErrInvalidParam, apperror.DefaultMessages[apperror.ErrInvalidParam]))
		return
	}

	// === userId 校验 ===
	if req.UserID == nil {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "userId 必填"))
		return
	}
	if *req.UserID == 0 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "userId 必须 > 0"))
		return
	}

	// === rarity 校验（§6.9 枚举：1=common / 2=rare / 3=epic / 4=legendary）===
	if req.Rarity == nil {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "rarity 必填"))
		return
	}
	if *req.Rarity < 1 || *req.Rarity > 4 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "rarity 必须 ∈ [1,4]"))
		return
	}

	// === count 校验（1 ≤ count ≤ 100）===
	if req.Count == nil {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "count 必填"))
		return
	}
	if *req.Count < 1 || *req.Count > 100 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "count 必须 ∈ [1,100]"))
		return
	}

	if err := h.svc.GrantCosmeticBatch(c.Request.Context(), *req.UserID, *req.Rarity, *req.Count); err != nil {
		_ = c.Error(err) // service 已 wrap *AppError；ErrorMappingMiddleware 写 envelope
		return
	}

	// 成功响应 —— 简单 ack，不返实际创建的 user_cosmetic_items 实例 id 列表
	// （节点 7 阶段 stub 不写库，没有实例 id 可返；节点 8 激活后由 23.5 owner 决定是否扩 response schema
	//  返实例 id 列表 —— 兼容性：先返简单 ack，等节点 11 demo / e2e 真有调用方需要再扩）
	response.Success(c, postGrantCosmeticBatchResponseDTO(*req.UserID, *req.Rarity, *req.Count), "ok")
}

// postGrantCosmeticBatchResponseDTO 拼装 ack response。
//
// **schema 选择**：返 `{userId, rarity, count}` 简单 ack —— 不返实际创建的实例 id 列表（节点 7 阶段 stub
// 没有实例 id；节点 8 激活后由 23.5 owner 决定是否扩 schema）。
//
// 与 Story 7.5 postGrantStepsResponseDTO / Story 20.7 postForceUnlockChestResponseDTO 同模式
// （dev 端点统一 ack 风格）。
func postGrantCosmeticBatchResponseDTO(userID uint64, rarity int8, count int32) gin.H {
	return gin.H{
		"userId": userID,
		"rarity": rarity,
		"count":  count,
	}
}
