package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// DevChestHandler 是 /dev/force-unlock-chest 等 dev 宝箱端点的 handler 集合（Story 20.7）。
//
// 与 ChestHandler (20.5 / 20.6) 区分：
//   - ChestHandler 处理 /api/v1/chest/*（业务接口；含 auth + rate_limit + 业务事务 / 幂等）
//   - DevChestHandler 处理 /dev/force-unlock-chest（dev 工具；不含 auth / rate_limit / 事务）
//
// 与 DevStepsHandler (Story 7.5) 平级：dev 工具按"业务模块"独立 handler,让未来加
// /dev/grant-cosmetic-batch (Epic 20.8) 时有独立 handler 槽位（DevCosmeticHandler），
// 避免单文件膨胀。
//
// 独立 handler 文件让"dev 工具"与"业务接口"边界清晰，dev 路径未来加新 chest 端点
// （如 /dev/reset-chest）也走本 handler。
type DevChestHandler struct {
	svc service.DevChestService
}

// NewDevChestHandler 构造 DevChestHandler。
func NewDevChestHandler(svc service.DevChestService) *DevChestHandler {
	return &DevChestHandler{svc: svc}
}

// PostForceUnlockChestRequest 是 POST /dev/force-unlock-chest 请求体的 Go mirror。
//
// **r2 [P2] 改造**：增加 chestId 字段 —— 详见 service.DevChestService r2 改造说明。
// client 必须先 GET /chest/current 拿到 chest.id（response.data.id 已 BIGINT 字符串化），
// 再 POST 这个 id 来。server 不再猜"current"语义。
//
// # ⚠️ 契约变更通告（Story 20.7 r2 起，r5 文档化）
//
// **本字段 schema 自 r2 起从 `{userId}` 变为 `{userId, chestId}` —— chestId 字段从
// optional / not present 变成 *必填*。epics.md §20.7 行 2941 早期钦定的
// `POST /dev/force-unlock-chest {userId: int64}` 与本实装语义不一致。**
//
// **变更原因**（r2 race 修复决策；r5 review 终审仍维持）：
//   - r1 实装"server 端推断 current chest"在并发场景下 race —— FindByUserIDForUpdate
//     的 FOR UPDATE 阻塞后看到的可能是 OpenChest 刚 INSERT 的 next chest，而非 client
//     看到的那个 chest，跑偏到错的 row
//   - 选项 A（codex r5 建议）：回退 chestId，让 server 内部找 current → race 复活
//   - 选项 B（本实装维持）：让 client 传具体 chestId，server 用事务 + FOR UPDATE
//     保证 unlock 正好是 client 看到的那个 chest → race 根因解决
//   - 选项 B 是正确的工程决策 —— **dev 端点正确性 > contract 美感**
//
// **stale-ID 失败模式**（选项 B 引入的代价；可接受）：
//   - 若 client 缓存了旧 chest.id 后被 OpenChest 鬼掉，POST 时 server 返 1003，
//     提示 client 重 GET /chest/current 拿新 id
//   - dev 端点仅供 demo / 自动化 e2e / 手工调试 —— 自动化脚本应 GET → POST 串行
//     执行（chest id 时延 < 1ms 内 stale 概率极低，除非中间有 /chest/open 并发）
//   - 1003 失败 = 让 client 重 GET 一次拿新 id 即可恢复；不是阻塞性 bug
//
// 详见 docs/lessons/2026-05-15-dev-endpoint-correctness-over-contract-aesthetics-20-7-r5.md
//
// # 字段约定
//
// **userId 用 *uint64 指针类型**（不挂 binding:"required"）：
//   - validator/v10 把 0 视为 zero value 会误判 "required"，与 7.5 PostGrantStepsRequest 同模式
//   - 用 *uint64 指针 + handler 显式 nil 校验区分 "字段缺失" vs "显式传 0"
//   - userId=0 在 MySQL users 表里**不存在**（AUTO_INCREMENT 从 1 起），handler 显式拒
//     让错误更早 + 错误消息更精确（"userId 必须 > 0"）
//
// **chestId 用 *string 指针类型**（BIGINT 字符串化；V1 §2.5 全局约定）：
//   - 与 ChestDTO.id / GET /chest/current.data.id（chest_handler.go 行 227）保持同类型
//   - handler 用 strconv.ParseUint 解析；解析失败 / 0 / 缺失 → 1002
//   - 与 V1 §10.4 roomId path 参数解析模式同（room_handler.JoinRoom 行 176）
//
// **不**接 unlockAt 字段（dev 产品语义是"立刻可开"；未来如需"滚动倒计时 demo"加独立端点）。
// **不**接 idempotencyKey 字段（dev 端点是"故意可重复"语义；重复调都把 unlock_at 推到本次 now）。
type PostForceUnlockChestRequest struct {
	UserID  *uint64 `json:"userId"`
	ChestID *string `json:"chestId"`
}

// PostForceUnlockChest 处理 POST /dev/force-unlock-chest（Story 20.7）。
//
// 流程：
//  1. ShouldBindJSON 兜一层（字段类型错 → 1002）
//  2. 手动校验：userId 非 nil + != 0；chestId 非 nil + 长度 1-20 + ParseUint 成功 + != 0
//  3. 调 svc.ForceUnlockChest(ctx, *userId, chestID) —— ctx = c.Request.Context()
//  4. 成功 → response.Success(c, postForceUnlockChestResponseDTO(...), "ok")
//  5. 失败 → c.Error(err) + return（middleware envelope；ADR-0006 单一 envelope 生产者）
//
// **不**做 auth 校验（dev 端点不要求 auth）；**不**取 c.Get(UserIDKey)（dev 路径无 auth 中间件）。
func (h *DevChestHandler) PostForceUnlockChest(c *gin.Context) {
	var req PostForceUnlockChestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.Wrap(err, apperror.ErrInvalidParam, apperror.DefaultMessages[apperror.ErrInvalidParam]))
		return
	}

	// === userId 校验（指针 nil → 字段未传 → 1002）===
	if req.UserID == nil {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "userId 必填"))
		return
	}
	if *req.UserID == 0 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "userId 必须 > 0"))
		return
	}

	// === chestId 校验（BIGINT 字符串化；与 room_handler.JoinRoom 同模式）===
	if req.ChestID == nil {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "chestId 必填"))
		return
	}
	if l := len(*req.ChestID); l < 1 || l > 20 {
		// BIGINT UNSIGNED max = 20 位十进制
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "chestId 长度非法"))
		return
	}
	chestID, err := strconv.ParseUint(*req.ChestID, 10, 64)
	if err != nil {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "chestId 非法"))
		return
	}
	if chestID == 0 {
		// 防御性：长度已限 1 ≤ length 但 "0" 字面值仍能 parse；业务上 chestID 必为正整数
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "chestId 必须 > 0"))
		return
	}

	if err := h.svc.ForceUnlockChest(c.Request.Context(), *req.UserID, chestID); err != nil {
		_ = c.Error(err) // service 已 wrap *AppError；ErrorMappingMiddleware 写 envelope
		return
	}

	// 成功响应 —— 简单 ack，不返当前 chest 状态（如要查 chest 用 GET /chest/current；端点单一职责）
	response.Success(c, postForceUnlockChestResponseDTO(*req.UserID, chestID), "ok")
}

// postForceUnlockChestResponseDTO 拼装 ack response。
//
// **schema 选择**：返 `{userId, chestId}` 简单 ack —— 不返当前 chest 状态。
//   - 调用方（demo / 自动化测试 / Epic 21 iOS）调本端点后再调 GET /chest/current
//     验证 status=2，而不是依赖本端点 response —— 端点单一职责（force-unlock 只负责"做了"，
//     get-current 只负责"读了"）
//   - chestId 用 string 类型（BIGINT 字符串化，与请求体同；V1 §2.5 全局约定）
//   - 与 Story 7.5 postGrantStepsResponseDTO 同模式（dev 端点统一 ack 风格）
func postForceUnlockChestResponseDTO(userID uint64, chestID uint64) gin.H {
	return gin.H{
		"userId":  userID,
		"chestId": strconv.FormatUint(chestID, 10),
	}
}
