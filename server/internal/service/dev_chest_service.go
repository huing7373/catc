package service

import (
	"context"
	stderrors "errors"
	"log/slog"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
)

// DevChestService 是 /dev/force-unlock-chest 端点的依赖 interface（Story 20.7）。
//
// **dev 端点的产品语义**：把指定 chestID（client 通过 GET /chest/current 先拿到的）unlock_at
// 推到 now() UTC，让 Story 20.5 GET /chest/current 动态判定 "unlock_at <= now → status=2
// unlockable" 立刻生效 → 调 POST /chest/open 即可开箱不被 4002 拦截。仅供 demo / 自动化 e2e /
// 手工调试，**不**走 prod。
//
// # 演进历史（race 修复路径）
//
// **r0 实装**：service 单 UPDATE WHERE user_id = ? —— 与 /chest/open 并发时跑偏到 next chest。
// **r1 实装**：service 改 FOR UPDATE SELECT 拿 chest.id → UPDATE WHERE id = ? 两步同事务 —— FOR UPDATE
//   阻塞结束后 SELECT 返回的是 commit 后的"当前 chest"（即 OpenChest 刚 INSERT 的 next chest Y），跑偏。
// **r2 实装**：把"哪个 chest"的决策权交给 client —— client 必须先 GET /chest/current 拿到当前
//   chest.id，再 POST /dev/force-unlock-chest 带这个 id。service 用 FindByID 校验存在 + 归属 +
//   UPDATE WHERE id（不开事务）；移除 RowsAffected==0 检查（顾虑同毫秒重复 unlock 误判）。
// **r3 实装**：r2 的"不看 RowsAffected"引入二阶 race（FindByID 与 UPDATE 之间 chest 被并发 OpenChest
//   删除 → UPDATE 0 行 → service 返 false success）。r3 在 repo 层把 RowsAffected==0 翻译为 ErrChestNotFound。
// **r4 实装（当前）**：r3 引入新 bug —— 同毫秒重复 unlock 同 chest → unlock_at 列（DATETIME(3) 毫秒精度）
//   值未变 → RowsAffected==0 → 误报 NotFound 1003。**根因解决**：跳出 r2-r3-r4 表层 RowsAffected 微调
//   chain，用 **事务 + SELECT FOR UPDATE + UPDATE** 三件套从 DB 层保证语义不变量：
//     1. txMgr.WithTx 开事务
//     2. FindByIDForUpdate 锁定 chest 行（并发 OpenChest 的 DELETE 必须等当前事务 commit）
//     3. UPDATE unlock_at —— 事务内行存在性已锁定保证，RowsAffected==0 唯一来源 = 值未变（视为 success）
//
// # 错误约定（ADR-0006 三层映射）
//
//   - mysql.ErrChestNotFound（FindByIDForUpdate 没行）→ 包成 ErrResourceNotFound (1003)
//   - chest.user_id != claimedUserID（越权）→ 同样返 1003（语义"该用户没这个 chest"，
//     避免暴露其他 user 信息）
//   - 其他 DB 异常（连接断 / SQL 错 / 死锁等）→ 包成 ErrServiceBusy (1009)
//
// **注意**：用 1003 而非 4001 ErrChestNotFound —— epics.md §20.7 行 2947 钦定 "用户无 chest → 1003"
// 与 Story 7.5 dev grant 用 1003 而非业务码同模式（dev 端点错误码统一在通用 1xxx 段）。
//
// **不**复用 Story 20.6 chest_service.OpenChest：
//   - OpenChest 含 8 步事务（幂等预声明 + 持锁查询 + 步数扣减 + 加权抽取 + 写日志 +
//     刷新下一轮）—— 全不适用 dev force-unlock（dev 端点只想"压时间"，不消费步数 / 不抽奖）
//   - 强行复用要绕过 5+ 步事务分支，反模式 → 独立 service 更清晰
//
// **不**复用 chest_service.GetCurrent：
//   - GetCurrent 是"纯读 + 动态翻译"；本 service 是"强制 UPDATE"，语义反向
type DevChestService interface {
	// ForceUnlockChest 把指定 chestID 的 user_chests.unlock_at 推到 now() UTC。
	//
	// **r4 [P2] 改造后 3 步事务**：
	//  1. txMgr.WithTx 开事务
	//  2. chestRepo.FindByIDForUpdate(txCtx, chestID) → 拿 chest + 行锁，校验存在 + chest.UserID == userID
	//  3. chestRepo.UpdateUnlockAtByID(txCtx, chestID, time.Now().UTC()) → UPDATE WHERE id=?；
	//     行锁保证存在性，RowsAffected==0 = 值未变（同毫秒重复 unlock 同 chest）→ 视为 success
	//
	// **不**改 chest.status 字段（Story 20.5 钦定 DB 字面 status 不动；动态判定全靠 unlock_at vs now）。
	// **不**改 chest.version 字段（dev 端点不参与乐观锁串行化）。
	// **不**接 unlock_at 参数（dev 产品语义是"立刻可开"；未来如需"滚动倒计时 demo"加独立端点）。
	//
	// 参数：
	//   - userID：claimed user（handler 从请求体取，**dev 端点无 auth** —— 信任 client；
	//     越权由 service 层 chest.user_id 校验防御）
	//   - chestID：client 通过 GET /chest/current 拿到的 chest.id（必传，> 0）
	ForceUnlockChest(ctx context.Context, userID uint64, chestID uint64) error
}

// devChestServiceImpl 是 DevChestService 的默认实装。
type devChestServiceImpl struct {
	txMgr     tx.Manager
	chestRepo mysql.ChestRepo
}

// NewDevChestService 构造 DevChestService。
//
// **r4 [P2] 改造**：重新引入 txMgr 注入 —— r2 移除 txMgr 是基于"事务对 race 修复无作用"的判断，
// 但 r4 用事务**不**是为修 race，而是为消除 RowsAffected==0 语义模糊性（详见 interface doc r4 改造说明）。
//
// 依赖：
//   - txMgr：事务管理器，让 FindByIDForUpdate + UpdateUnlockAtByID 在同一事务内执行
//   - chestRepo：FindByIDForUpdate 锁行 + 校验存在 + 归属 + UpdateUnlockAtByID 强制更新 unlock_at
//
// **不**接 userRepo（FindByIDForUpdate + chest.user_id 校验已覆盖存在性 + 越权）；
// **不**接 stepAccountRepo / stepSyncLogRepo（dev force-unlock 不读写步数账户）；
// **不**接 envName（dev 端点已被 build tag / env var 双闸门防 prod）。
func NewDevChestService(txMgr tx.Manager, chestRepo mysql.ChestRepo) DevChestService {
	return &devChestServiceImpl{txMgr: txMgr, chestRepo: chestRepo}
}

// ForceUnlockChest 实装：事务内 FindByIDForUpdate 校验 → UPDATE WHERE id（r4 [P2] 改造）。
//
// **time.Now().UTC()**：必须 UTC（与 V1 §2.5 ISO 8601 UTC 钦定一致；与 chest.UnlockAt
// 字段 UTC 语义对齐 —— chest_repo.go 顶部注释钦定，Story 4.6 firstTimeLogin
// 也用 time.Now().UTC().Add(10*time.Minute)，与本 story now() 同源 UTC）。
//
// **r4 改造**：用 txMgr.WithTx 包 FindByIDForUpdate + UpdateUnlockAtByID 两步 —— FOR UPDATE 行锁
// 保证事务期间 chest 行不会被并发 OpenChest 删除，UPDATE 时行存在性已锁定 → 跳出 r2-r3-r4
// over-correction chain（RowsAffected==0 不再需要"猜"是 NotFound 还是值未变；事务内 = 值未变 = success）。
func (s *devChestServiceImpl) ForceUnlockChest(ctx context.Context, userID uint64, chestID uint64) error {
	return s.txMgr.WithTx(ctx, func(txCtx context.Context) error {
		// (1) 事务内 FOR UPDATE 锁定 chest 行 + 校验存在 + 归属 user
		chest, err := s.chestRepo.FindByIDForUpdate(txCtx, chestID)
		if err != nil {
			if stderrors.Is(err, mysql.ErrChestNotFound) {
				// epics.md §20.7 行 2947 钦定：用户无 chest → 1003（**非** 4001）
				return apperror.Wrap(err, apperror.ErrResourceNotFound, apperror.DefaultMessages[apperror.ErrResourceNotFound])
			}
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}
		if chest.UserID != userID {
			// 越权 → 同样返 1003（语义"该用户没这个 chest"，避免暴露其他 user 信息）。
			// **不**返 1006 ErrForbidden / 4001 ErrChestNotFound —— 与 epics.md §20.7 行 2947 1003 钦定对齐。
			slog.WarnContext(txCtx, "dev force-unlock-chest cross-user attempt",
				"claimed_user_id", userID, "chest_id", chestID, "chest_owner_user_id", chest.UserID,
			)
			return apperror.New(apperror.ErrResourceNotFound, apperror.DefaultMessages[apperror.ErrResourceNotFound])
		}

		// (2) 事务内 UPDATE WHERE id=chestID。行锁已保证 chest 存在；RowsAffected==0 = 值未变
		// （同毫秒重复 unlock 同 chest）= success（r4 [P2] 改造后 repo 不再翻译为 NotFound）。
		now := time.Now().UTC()
		if err := s.chestRepo.UpdateUnlockAtByID(txCtx, chestID, now); err != nil {
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}

		slog.WarnContext(txCtx, "dev force-unlock-chest applied",
			"user_id", userID, "chest_id", chestID, "unlock_at", now.Format(time.RFC3339),
		)
		return nil
	})
}
