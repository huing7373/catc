package service

import (
	"context"
	"log/slog"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
)

// DevCosmeticService 是 /dev/grant-cosmetic-batch 端点的依赖 interface（Story 20.8）。
//
// **dev 端点的产品语义**：给指定用户批量发放指定品质的 cosmetic_items 实例（按 rarity
// 从 cosmetic_items 池中随机抽 count 个 cosmetic_item_id 创建 user_cosmetic_items 实例），
// 让节点 11 合成 demo 不必反复开箱凑齐 10 件 common。仅供 demo / 自动化 e2e / 手工调试，
// **不**走 prod。
//
// # 节点 7 vs 节点 8 阶段实装策略（**选项 C**，epics.md §20.8 行 2964 钦定）
//
// **节点 7 阶段（本 story 范围）：stub 显式失败实装**
//   - 路由 /dev/grant-cosmetic-batch + handler 框架（DTO + 1002 参数校验）+ service 接口 final
//   - service 实装内部 slog.WarnContext 输出"endpoint called in node-7 stub phase, returns 503 by design"
//     警告，然后 **return apperror.New(ErrServiceBusy, "...")**（1009 + middleware 自动翻 HTTP 503）
//   - **关键设计**：stub 期不能返 200 success —— silent false-positive 会让 e2e / demo 拿到"调用成功 +
//     仓库空"的矛盾态，调试链路无故拉长。explicit failure 让调用方在请求层立刻看到"endpoint 还没激活"
//   - 全套单测（service 3 case + handler 6 case + devtools 2 case + bootstrap 1 case），断言 1009
//   - **不**新建 user_cosmetic_items_repo / **不**新建 migration / **不**改 cosmetic_item_repo
//
// **节点 8 / Epic 23.5 阶段（**不**在本 story 范围 —— 由 23.5 owner 在本 service 内激活）：真实写库**
//   - Story 23.2 落地 user_cosmetic_items migration + 23.5 落地 user_cosmetic_item_repo.BatchCreate
//     + cosmetic_item_repo.FindRandomByRarity（若不存在则同步落地）后
//   - 修改本 service 实装：移除"stub 返 1009"分支 → 加 cosmeticItemRepo.FindRandomByRarity(ctx, rarity, count)
//     + userCosmeticItemRepo.BatchCreate(ctx, userID, []cosmeticItemIDs) 两步写库 → 成功 return nil
//   - 修改 NewDevCosmeticService 构造函数签名加新 repo 依赖
//   - 同步改本 service 单测：把 1009 断言换成"happy path return nil + repo BatchCreate 被调"
//   - **接口签名 / 路由 / 客户端调用代码不变** —— 兼容已部署的 e2e 脚本
//
// # 错误约定（ADR-0006 三层映射）
//
// **节点 7 阶段（本 story）**：
//   - rarity / count 越界由 handler 1002 拦截，service 不收到
//   - service stub 实装 **永远 return ErrServiceBusy (1009)**：endpoint 物理可达但功能未激活
//     → middleware 自动翻 HTTP 503，调用方明确知道"endpoint not yet active"
//
// **节点 8 阶段（激活后）**：
//   - 真实写库 happy path → return nil
//   - mysql.ErrCosmeticItemNotFound（FindRandomByRarity 没数据 —— 理论 Story 20.3 seed ≥15 行不该发生）
//     → 包成 ErrServiceBusy (1009)（seed 数据完整性异常）
//   - userCosmeticItemRepo.BatchCreate 失败 → 包成 ErrServiceBusy (1009)
//   - userRepo.FindByID 验用户存在（可选，节点 8 owner 决定）→ ErrUserNotFound → ErrResourceNotFound (1003)
type DevCosmeticService interface {
	// GrantCosmeticBatch 给指定 userID 批量发放 count 个 rarity 品质的 cosmetic_items 实例。
	//
	// **节点 7 阶段 stub 行为**：slog.WarnContext 记录调用 + **return apperror.ErrServiceBusy (1009)**
	// → middleware 自动翻 HTTP 503。endpoint 物理可达（路由 / handler / DTO 校验完整），但 service 层
	// 显式拒绝 —— 让调用方立刻知道"endpoint not yet active in node-7 phase"，避免 silent false-positive。
	//
	// **节点 8 激活后行为**：事务内或事务外（节点 8 owner 决定）：
	//  1. cosmeticItemRepo.FindRandomByRarity(ctx, rarity, count) 返回 count 个 cosmetic_item_id（来自 enabled 池）
	//  2. userCosmeticItemRepo.BatchCreate(ctx, userID, cosmeticItemIDs, source=2 admin_grant)
	//     → INSERT 多条 user_cosmetic_items 行（status=1 in_bag / source=2 / source_ref_id=NULL / obtained_at=now）
	//  3. happy path return nil；任一步失败 wrap 成 1009
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

// devCosmeticServiceImpl 是 DevCosmeticService 的节点 7 阶段 stub 实装。
//
// **节点 7 阶段**：无 repo 依赖（不写库；显式返 1009）。
// **节点 8 激活后**：在本 struct 加 cosmeticItemRepo + userCosmeticItemRepo 字段（节点 8 owner 改）。
type devCosmeticServiceImpl struct{}

// NewDevCosmeticService 构造 DevCosmeticService 节点 7 阶段 stub。
//
// **节点 7 阶段**：无参数（stub 不需要 repo）。
// **节点 8 激活时**：节点 8 owner 改签名加 repo 依赖，如：
//
//	func NewDevCosmeticService(cosmeticItemRepo mysql.CosmeticItemRepo,
//	    userCosmeticItemRepo mysql.UserCosmeticItemRepo) DevCosmeticService { ... }
//
// 接口签名 / 路由 / 客户端调用代码不变 → 兼容已部署的 e2e 脚本。
func NewDevCosmeticService() DevCosmeticService {
	return &devCosmeticServiceImpl{}
}

// GrantCosmeticBatch 节点 7 阶段 stub 实装：WARN 日志 + return ErrServiceBusy (1009)。
//
// **设计原则**：stub endpoint **绝不返 success** —— silent false-positive 会让 e2e / demo
// 链路在"调用成功 + 仓库空"的矛盾态里调试很久才发现根因。显式返 1009 → middleware 翻 HTTP 503，
// 调用方立刻看到"endpoint not yet active in node-7 phase, awaits Story 23.5 to activate"。
//
// WARN log 级别保留（不是 ERROR）—— 节点 7 阶段被调用是预期"未激活路径"，不是系统错误；
// 运维 / dev 可通过 `phase=node-7-stub` grep 找出还在 stub 状态的端点。
//
// **节点 8 激活后** 替换为：
//
//	cosmeticItemIDs, err := s.cosmeticItemRepo.FindRandomByRarity(ctx, rarity, count)
//	if err != nil { return apperror.Wrap(err, apperror.ErrServiceBusy, "...") }
//	if err := s.userCosmeticItemRepo.BatchCreate(ctx, userID, cosmeticItemIDs, ...); err != nil {
//	    return apperror.Wrap(err, apperror.ErrServiceBusy, "...")
//	}
//	slog.InfoContext(ctx, "dev grant cosmetic batch applied", ...)
//	return nil
func (s *devCosmeticServiceImpl) GrantCosmeticBatch(ctx context.Context, userID uint64, rarity int8, count int32) error {
	slog.WarnContext(ctx, "dev grant-cosmetic-batch called in node-7 stub phase, returns 503 by design (endpoint not yet active; awaits Story 23.5 to activate after Story 23.2 user_cosmetic_items migration)",
		"user_id", userID,
		"rarity", rarity,
		"count", count,
		"phase", "node-7-stub",
		"todo", "activate real writes in Story 23.5 (after Story 23.2 user_cosmetic_items migration)",
	)
	return apperror.New(apperror.ErrServiceBusy, "dev/grant-cosmetic-batch not yet implemented (node-7 stub; awaits Story 23.5 to activate)")
}
