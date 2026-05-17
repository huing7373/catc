package service

import (
	"context"
	"log/slog"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// DevCosmeticService 是 /dev/grant-cosmetic-batch 端点的依赖 interface（Story 20.8）。
//
// **dev 端点的产品语义**：给指定用户批量发放指定品质的 cosmetic_items 实例（按 rarity
// 从 cosmetic_items 池中随机抽 count 个 cosmetic_item_id 创建 user_cosmetic_items 实例），
// 让节点 11 合成 demo 不必反复开箱凑齐 10 件 common。仅供 demo / 自动化 e2e / 手工调试，
// **不**走 prod。
//
// # 节点 7 → 节点 8 阶段实装策略（**选项 C**，epics.md §20.8 行 2964 钦定）
//
// **节点 7 阶段（已退役，Story 20.8 交付）：stub 显式失败实装**
//   - 路由 + handler 框架（DTO + 1002 参数校验）+ service 接口 final；service stub
//     slog.WarnContext + return apperror.ErrNotImplemented (1010 → HTTP 501 + WARN log)
//   - 设计原则：stub 期不返 200 success —— explicit failure 让调用方立刻看到"endpoint 还没激活"
//
// **节点 8 / Story 23.5 阶段（当前实装）：真实写库（本 service 已激活）**
//   - Story 23.2 落地 user_cosmetic_items migration；Story 23.5 落地
//     user_cosmetic_item_repo.CreateInTx + cosmetic_item_repo.FindRandomByRarity
//   - 本 service 实装：移除 stub 1010 分支 → cosmeticItemRepo.FindRandomByRarity(ctx, rarity, count)
//     抽 count 个 enabled cosmetic_item_id → 逐条 userCosmeticItemRepo.CreateInTx 写
//     user_cosmetic_items（status=1 in_bag / **source=3 admin_grant** / source_ref_id=NULL
//     / obtained_at=now）→ happy return nil；任一步失败 wrap 1009
//   - **source disambiguation（Story 23.5）**：原下方接口注释行写 source=2，与 §6.11
//     枚举（2=compose / 3=admin_grant）冲突；dev 发放语义是 admin_grant 应取
//     **source=3**，以 §6.11 + UserCosmeticItem struct 注释为准（不反向改文档；
//     与 23.4 r1 同源原则）
//   - **dev grant 走事务外批量发放**（无 idempotency / 无步数语义）—— 逐条
//     CreateInTx；tx.FromContext 在无 txCtx 时走 r.db 直连，行为正确
//   - **接口签名 / 路由 / 客户端调用代码不变** —— 兼容已部署的 e2e 脚本
//
// # 错误约定（ADR-0006 三层映射）
//
//   - rarity / count 越界由 handler 1002 拦截，service 不收到
//   - 真实写库 happy path → return nil
//   - FindRandomByRarity 返回数量 < count（含返空 —— rarity 的 enabled 池
//     < count，如 seed common 仅 8 件而 count=10）→ 包成 ErrServiceBusy (1009)，
//     **写库前明确拒绝不静默少发**（fix-review 23-5 r1 [P1]）
//   - FindRandomByRarity DB error / CreateInTx 失败 → 包成 ErrServiceBusy (1009)
type DevCosmeticService interface {
	// GrantCosmeticBatch 给指定 userID 批量发放 count 个 rarity 品质的 cosmetic_items 实例。
	//
	// **节点 8 / Story 23.5 真实写库行为**（节点 7 stub 1010 已退役）：
	//  1. cosmeticItemRepo.FindRandomByRarity(ctx, rarity, count) 返回 count 个 cosmetic_item_id（enabled 池）
	//  2. 逐条 userCosmeticItemRepo.CreateInTx 写 user_cosmetic_items
	//     （status=1 in_bag / **source=3 admin_grant**（§6.11 枚举；见上 disambiguation）
	//     / source_ref_id=NULL / obtained_at=now）
	//  3. happy path return nil；FindRandomByRarity 返回数量 < count（含空 —— 池
	//     不足请求量，写库前拒绝不静默少发）/ DB error / CreateInTx 失败 wrap 1009
	//
	// 参数：
	//   - userID：目标用户 ID（handler 已校验 > 0）
	//   - rarity：装扮品质，1=common / 2=rare / 3=epic / 4=legendary（§6.9 钦定；handler 已校验 ∈ [1,4]）
	//   - count：发放数量，1 ≤ count ≤ 100（handler 已校验；上限 100 防 demo 误传 1e6 砸 DB）
	//
	// **不**接 cosmeticItemID 参数（dev 产品语义是"按品质随机抽"，不是"指定 cosmetic 发放"；
	// 未来如需"指定 cosmetic 发放"加独立 /dev/grant-cosmetic-by-id 端点，YAGNI 本 story 不预实装）。
	GrantCosmeticBatch(ctx context.Context, userID uint64, rarity int8, count int32) error
}

// devCosmeticServiceImpl 是 DevCosmeticService 的实装。
//
// **节点 7 阶段（已退役）**：无 repo 依赖（不写库；显式返 1010）。
// **节点 8 / Story 23.5 激活**：注入 cosmeticItemRepo（FindRandomByRarity 抽配置 id）
// + userCosmeticItemRepo（CreateInTx 写实例）真实写库。
type devCosmeticServiceImpl struct {
	cosmeticItemRepo     mysql.CosmeticItemRepo
	userCosmeticItemRepo mysql.UserCosmeticItemRepo
}

// NewDevCosmeticService 构造 DevCosmeticService。
//
// **Story 23.5 扩签名（节点 8 激活）**：注入 cosmeticItemRepo + userCosmeticItemRepo。
// 接口签名 / 路由 / 客户端调用代码不变 → 兼容已部署的 e2e 脚本（仅 constructor
// 签名扩参 + service 实装从 stub 转真实写库）。
func NewDevCosmeticService(
	cosmeticItemRepo mysql.CosmeticItemRepo,
	userCosmeticItemRepo mysql.UserCosmeticItemRepo,
) DevCosmeticService {
	return &devCosmeticServiceImpl{
		cosmeticItemRepo:     cosmeticItemRepo,
		userCosmeticItemRepo: userCosmeticItemRepo,
	}
}

// GrantCosmeticBatch 节点 8 / Story 23.5 真实写库实装（节点 7 stub 1010 已退役）：
//
//  1. cosmeticItemRepo.FindRandomByRarity(ctx, rarity, count) 抽 count 个 enabled
//     cosmetic_item_id；返回数量 < count（含空 —— rarity 池不足请求量）→ 1009
//     写库前明确拒绝不静默少发（fix-review 23-5 r1 [P1]）；DB error → 1009
//  2. 逐条 userCosmeticItemRepo.CreateInTx 写 user_cosmetic_items（status=1 in_bag /
//     **source=3 admin_grant**（§6.11 枚举，见 interface 注释 disambiguation）/
//     source_ref_id=NULL / obtained_at=now）；任一条失败 → 1009
//  3. happy → slog.InfoContext "dev grant cosmetic batch applied" + return nil
//
// **dev grant 走事务外批量发放**（无 idempotency / 无步数语义）—— tx.FromContext
// 在无 txCtx 时走 r.db 直连，逐条 CreateInTx 行为正确（区别于开箱事务步骤 5g.5
// 走 txCtx 同事务）。
func (s *devCosmeticServiceImpl) GrantCosmeticBatch(ctx context.Context, userID uint64, rarity int8, count int32) error {
	// 1. 按 rarity 随机抽 count 个 enabled cosmetic_item_id
	cosmeticItemIDs, err := s.cosmeticItemRepo.FindRandomByRarity(ctx, rarity, count)
	if err != nil {
		return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	// 池不足 count 时**明确拒绝**，不静默少发（fix-review 23-5 r1 [P1]）：
	// FindRandomByRarity 用 `ORDER BY RAND() LIMIT ?`，当该 rarity 的 enabled 池
	// < count 时**合法**返回少于 count 个 id（如 seed common 仅 8 件、handler 接受
	// count 至 100、demo 用 count=10 → 只返 8 个）。原实装只把 len==0 当异常，
	// 遗漏 0 < len < count 这一档 → 调用方静默拿到比请求少的实例（success 但短发）。
	// 这里用 FindRandomByRarity **实际返回的 len** 与请求 count 直接比对（不引入
	// "先 count 再 fetch" 双查询的 race），len < count 即在**写库前**拒绝并返回
	// 明确错误（杜绝部分插入），与既有空池异常同族 ErrServiceBusy(1009)
	// （语义一致：池数据无法满足请求；len==0 是 count>0 时本分支的子集）。
	if len(cosmeticItemIDs) < int(count) {
		// 池不足请求量（含 len==0 的 seed 数据完整性异常子集）→ 1009，不静默少发
		return apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 2. 逐条 CreateInTx 写 user_cosmetic_items（AC1 已落地方法复用）。
	//
	// **source=3 admin_grant**（§6.11 枚举 + UserCosmeticItem struct 注释钦定
	// 3=admin_grant）—— **disambiguation**：本文件原行 64 注释写 source=2，与
	// §6.11 枚举（2=compose / 3=admin_grant）冲突；dev 发放语义是 admin_grant
	// 应取 source=3，以 §6.11 枚举为准（与 23.4 r1 同源原则"契约/文档不一致时
	// 以更权威的枚举定义为准，记录 disambiguation，不反向改文档"）。
	//
	// dev grant 是事务外批量发放（无 idempotency / 无步数扣减语义）—— 逐条
	// CreateInTx；tx.FromContext 在无 txCtx 时走 r.db 直连，行为正确。
	// source_ref_id=NULL（dev 发放无来源记录，传 nil）；status=1 in_bag；
	// obtained_at 传 time.Now().UTC()（与项目 UTC 时间钦定一致）。
	now := time.Now().UTC()
	for _, cid := range cosmeticItemIDs {
		item := &mysql.UserCosmeticItem{
			UserID:         userID,
			CosmeticItemID: cid,
			Status:         1,   // 1=in_bag
			Source:         3,   // 3=admin_grant（§6.11 枚举，见上 disambiguation）
			SourceRefID:    nil, // dev 发放无来源记录
			ObtainedAt:     now,
		}
		if err := s.userCosmeticItemRepo.CreateInTx(ctx, item); err != nil {
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}
	}

	slog.InfoContext(ctx, "dev grant cosmetic batch applied",
		"user_id", userID,
		"rarity", rarity,
		"count", count,
		"granted", len(cosmeticItemIDs),
		"source", 3,
	)
	return nil
}
