package service

import (
	"context"
	"log/slog"
	"math/rand"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
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
// **节点 8 / Story 23.5 阶段（当前实装；fix-review 23-5 r2 [P2] 根因修复后）**
//   - Story 23.2 落地 user_cosmetic_items migration；Story 23.5 落地
//     user_cosmetic_item_repo.CreateInTx + cosmetic_item_repo.ListEnabledIDsByRarity
//   - 本 service 实装：移除 stub 1010 分支 → cosmeticItemRepo.ListEnabledIDsByRarity(ctx, rarity)
//     取该 rarity **全部** enabled cosmetic_item_id（池）→ Go 层**有放回**
//     抽 count 个 → **一个事务内**逐条 userCosmeticItemRepo.CreateInTx 写
//     user_cosmetic_items（status=1 in_bag / **source=3 admin_grant** / source_ref_id=NULL
//     / obtained_at=now）→ happy return nil；任一步失败整批回滚 + wrap 1009
//   - **count 语义（根因 / over-correction chain 收敛，fix-review 23-5 r2）**：
//     `count` 是要授予的**实例数**，**不是** distinct 配置数。user_cosmetic_items
//     （DB §5.9）无 UNIQUE(user_id, cosmetic_item_id)，同 cosmetic_item_id
//     多实例合法且为合成 feature 核心所必需（§22 喂 10 个同品质实例升级）。
//     因此 pick 必须**有放回**从池里选 count 个（pool=8, count=10 → 返 10 个
//     允许重复的 id）。r1 引入的 `len<count→1009` 拒绝基于错误契约（把 count
//     误读成 distinct 上限），把合法主 demo 用例（common 池 8 但要 grant 10
//     个实例供合成）打死 —— **本轮已撤销**。仅保留"池**完全为空**（该 rarity
//     无任何 enabled 配置）→ 1009"这一**真正**的 seed 数据完整性错误档。
//   - **source disambiguation（Story 23.5）**：原下方接口注释行写 source=2，与 §6.11
//     枚举（2=compose / 3=admin_grant）冲突；dev 发放语义是 admin_grant 应取
//     **source=3**，以 §6.11 + UserCosmeticItem struct 注释为准（不反向改文档；
//     与 23.4 r1 同源原则）
//   - **dev grant 批量发放包进一个独立事务**（CLAUDE.md「资产类操作必须事务」
//     铁律 —— dev grant 是资产写入；fix-review 23-5 r2 [P2] #2 修复）：
//     txManager.WithTx(ctx, ...) 内逐条 CreateInTx(txCtx, ...)，all-or-nothing，
//     任一失败整批回滚，杜绝部分提交 → 调用方重试致部分授予/重复批次。
//     **注意**：这是 dev grant 自己的独立事务，与 Epic 20 race-fix 的
//     runOpenChestTx 是两条独立路径（无 idempotency / 无步数语义）
//   - **接口签名 / 路由 / 客户端调用代码不变** —— 兼容已部署的 e2e 脚本
//
// # 错误约定（ADR-0006 三层映射）
//
//   - rarity / count 越界由 handler 1002 拦截，service 不收到
//   - 真实写库 happy path → return nil
//   - ListEnabledIDsByRarity 返回**空池**（该 rarity 无任何 enabled 配置 ——
//     理论 seed ≥15 行覆盖 4 rarity 不该发生）→ 包成 ErrServiceBusy (1009)。
//     池**非空但 < count 不是错误**（有放回抽满 count；撤销 r1 拒绝）
//   - ListEnabledIDsByRarity DB error / 事务内任一 CreateInTx 失败（整批回滚）
//     → 包成 ErrServiceBusy (1009)
type DevCosmeticService interface {
	// GrantCosmeticBatch 给指定 userID 批量发放 count 个 rarity 品质的 cosmetic_items 实例。
	//
	// **节点 8 / Story 23.5 真实写库行为**（节点 7 stub 1010 已退役；
	// fix-review 23-5 r2 [P2] 根因修复后）：
	//  1. cosmeticItemRepo.ListEnabledIDsByRarity(ctx, rarity) 返回该 rarity 全部
	//     enabled cosmetic_item_id（池）；池**空**（无任何 enabled 配置）→ 1009
	//  2. Go 层从池中**有放回**随机抽 count 个 id（count 是实例数非 distinct 数；
	//     pool=8, count=10 → 返 10 个允许重复的 id）
	//  3. **一个事务内**逐条 userCosmeticItemRepo.CreateInTx(txCtx, ...) 写
	//     user_cosmetic_items（status=1 in_bag / **source=3 admin_grant**
	//     （§6.11 枚举；见上 disambiguation）/ source_ref_id=NULL / obtained_at=now）
	//  4. happy path return nil；空池 / DB error / 事务内任一 CreateInTx 失败
	//     （整批回滚，杜绝部分提交）→ wrap 1009
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
// **节点 8 / Story 23.5 激活**：注入 cosmeticItemRepo（ListEnabledIDsByRarity 取池）
// + userCosmeticItemRepo（CreateInTx 写实例）+ txManager（批量发放原子事务，
// fix-review 23-5 r2 [P2] #2）真实写库。
type devCosmeticServiceImpl struct {
	cosmeticItemRepo     mysql.CosmeticItemRepo
	userCosmeticItemRepo mysql.UserCosmeticItemRepo
	txManager            tx.Manager
}

// NewDevCosmeticService 构造 DevCosmeticService。
//
// **Story 23.5 扩签名（节点 8 激活）**：注入 cosmeticItemRepo + userCosmeticItemRepo
// + txManager（fix-review 23-5 r2 [P2] #2 —— dev grant batch 资产写入必须原子事务，
// 与 NewDevChestService(txMgr, chestRepo) 同模式）。接口签名 / 路由 / 客户端调用
// 代码不变 → 兼容已部署的 e2e 脚本（仅 constructor 签名扩参 + service 实装升级）。
func NewDevCosmeticService(
	cosmeticItemRepo mysql.CosmeticItemRepo,
	userCosmeticItemRepo mysql.UserCosmeticItemRepo,
	txManager tx.Manager,
) DevCosmeticService {
	return &devCosmeticServiceImpl{
		cosmeticItemRepo:     cosmeticItemRepo,
		userCosmeticItemRepo: userCosmeticItemRepo,
		txManager:            txManager,
	}
}

// GrantCosmeticBatch 节点 8 / Story 23.5 真实写库实装（节点 7 stub 1010 已退役；
// fix-review 23-5 r2 [P2] 根因修复后）：
//
//  1. cosmeticItemRepo.ListEnabledIDsByRarity(ctx, rarity) 取该 rarity 全部
//     enabled cosmetic_item_id（池）；DB error → 1009；池**空**（该 rarity
//     无任何 enabled 配置 —— 真正的 seed 数据完整性错误）→ 1009
//  2. 从池中**有放回**随机抽 count 个 id（count 是实例数，非 distinct 数 ——
//     根因；pool=8, count=10 → 返 10 个允许重复的 id；撤销 r1 `len<count→1009`）
//  3. txManager.WithTx 内逐条 userCosmeticItemRepo.CreateInTx(txCtx, ...) 写
//     user_cosmetic_items（status=1 in_bag / **source=3 admin_grant**（§6.11
//     枚举，见 interface 注释 disambiguation）/ source_ref_id=NULL /
//     obtained_at=now）；事务内任一条失败 → 整批回滚 + 1009
//  4. happy → slog.InfoContext "dev grant cosmetic batch applied" + return nil
//
// **dev grant batch 包进一个独立事务**（CLAUDE.md「资产类操作必须事务」铁律 ——
// dev grant 是资产写入；fix-review 23-5 r2 [P2] #2）：all-or-nothing，杜绝
// "部分提交 + 返错 → 调用方重试致部分授予/重复批次"。这是 dev grant 自己的
// 独立事务（无 idempotency / 无步数语义），与开箱事务 runOpenChestTx 是两条
// 独立路径（区别于 5g.5 走开箱 txCtx 同事务）。
func (s *devCosmeticServiceImpl) GrantCosmeticBatch(ctx context.Context, userID uint64, rarity int8, count int32) error {
	// 1. 取该 rarity 全部 enabled cosmetic_item_id（池 —— repo 不再 LIMIT count）
	pool, err := s.cosmeticItemRepo.ListEnabledIDsByRarity(ctx, rarity)
	if err != nil {
		return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	// **空池才是错误**（该 rarity 无任何 enabled 配置 —— 理论 Story 20.3
	// seed ≥15 行覆盖 4 rarity 不该发生的 seed 数据完整性异常）。
	//
	// 池**非空但 < count 不是错误**（fix-review 23-5 r2 [P2] #1 根因 +
	// over-correction chain 收敛）：`count` 是要授予的**实例数**，不是
	// distinct 配置数；user_cosmetic_items（DB §5.9）无 UNIQUE(user_id,
	// cosmetic_item_id），同 cosmetic_item_id 多实例合法且为合成 feature
	// 核心所必需（§22 喂 10 个同品质实例升级）。r1 引入的 `len<count→1009`
	// 拒绝基于错误契约（把 count 误读成 distinct 上限，把 common 池 8 但要
	// grant 10 个实例的合法主 demo 用例打死）—— **本轮已撤销**。
	if len(pool) == 0 {
		return apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 2. 从池中**有放回**抽 count 个 cosmetic_item_id（允许重复 —— 数量语义
	// 与池大小彻底解耦：pool=8, count=10 → 返 10 个允许重复的 id）。
	// rand.Intn 在 dev 小样本（count ≤ 100 handler 已校验）足够；**不**用于
	// prod 抽奖热路径（prod 走 ListEnabledForWeightedPick + 加权采样）。
	cosmeticItemIDs := make([]uint64, count)
	for i := int32(0); i < count; i++ {
		cosmeticItemIDs[i] = pool[rand.Intn(len(pool))]
	}

	// 3. **一个事务内**逐条 CreateInTx 写 user_cosmetic_items（AC1 已落地
	// 方法复用 —— CreateInTx 实现不变，仅由 dev grant 自己的事务复用）。
	//
	// **source=3 admin_grant**（§6.11 枚举 + UserCosmeticItem struct 注释钦定
	// 3=admin_grant）—— **disambiguation**：本文件原行注释写 source=2，与
	// §6.11 枚举（2=compose / 3=admin_grant）冲突；dev 发放语义是 admin_grant
	// 应取 source=3，以 §6.11 枚举为准（与 23.4 r1 同源原则"契约/文档不一致时
	// 以更权威的枚举定义为准，记录 disambiguation，不反向改文档"）。
	//
	// source_ref_id=NULL（dev 发放无来源记录，传 nil）；status=1 in_bag；
	// obtained_at 传 time.Now().UTC()（与项目 UTC 时间钦定一致）。
	// **事务内 CreateInTx 必须用 txCtx 而非外层 ctx**（CLAUDE.md ctx 必传铁律
	// + tx.Manager 注释钦定）—— 任一条失败 fn 返 err → 整批回滚。
	now := time.Now().UTC()
	if err := s.txManager.WithTx(ctx, func(txCtx context.Context) error {
		for _, cid := range cosmeticItemIDs {
			item := &mysql.UserCosmeticItem{
				UserID:         userID,
				CosmeticItemID: cid,
				Status:         1,   // 1=in_bag
				Source:         3,   // 3=admin_grant（§6.11 枚举，见上 disambiguation）
				SourceRefID:    nil, // dev 发放无来源记录
				ObtainedAt:     now,
			}
			if err := s.userCosmeticItemRepo.CreateInTx(txCtx, item); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	slog.InfoContext(ctx, "dev grant cosmetic batch applied",
		"user_id", userID,
		"rarity", rarity,
		"count", count,
		"granted", len(cosmeticItemIDs),
		"pool_size", len(pool),
		"source", 3,
	)
	return nil
}
