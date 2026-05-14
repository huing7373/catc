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
// **dev 端点的产品语义**：把指定用户的当前 chest 强制推到 unlockable 状态（直接
// UPDATE `user_chests.unlock_at = now()` UTC），与 Story 20.5 GET /chest/current
// 动态判定 "unlock_at <= now → status=2 unlockable" 串联 —— 调本端点后立刻调
// GET /chest/current 拿到 status=2 / remainingSeconds=0，调 POST /chest/open
// 即可开箱不被 4002「chest 尚未解锁」拦截。仅供 demo / 自动化 e2e / 手工调试，**不**走 prod。
//
// **不**复用 Story 20.6 chest_service.OpenChest：
//   - OpenChest 含 8 步事务（幂等预声明 + 持锁查询 + 步数扣减 + 加权抽取 + 写日志 +
//     刷新下一轮）—— 全不适用 dev force-unlock（dev 端点只想"压时间"，不消费步数 / 不抽奖）
//   - 强行复用要绕过 5+ 步事务分支，反模式 → 独立 service 更清晰
//
// **不**复用 chest_service.GetCurrent：
//   - GetCurrent 是"纯读 + 动态翻译"；本 service 是"强制 UPDATE"，语义反向
//
// 错误约定（ADR-0006 三层映射）：
//   - mysql.ErrChestNotFound（FindByUserIDForUpdate 没行 / UpdateUnlockAtByID rows_affected=0）
//     → 包成 ErrResourceNotFound (1003)
//     **注意**：用 1003 而非 4001 ErrChestNotFound —— epics.md §20.7 行 2947 钦定 "用户无 chest → 1003"
//     与 Story 7.5 dev grant 用 1003 而非业务码同模式（dev 端点错误码统一在通用 1xxx 段）
//   - 其他 DB 异常（连接断 / SQL 错 / 死锁等）→ 包成 ErrServiceBusy (1009)
//
// # 事务边界（Story 20.7 review r1 [P2] 改造）
//
// **必须开事务** —— 与 r1 [P2] 之前的"单 UPDATE 不开事务"形成对比。
//
// **r1 改造前**：service 直接调 `chestRepo.UpdateUnlockAt(ctx, userID, now)` 单 UPDATE。
// MySQL 单 UPDATE 是原子的，但 race 不在原子性，而在"**WHERE 子句二次匹配**"：
//
//	T0  client A: POST /chest/open → 事务内 FOR UPDATE 锁 chest row id=X
//	T1  client B: POST /dev/force-unlock-chest → UPDATE WHERE user_id=? 阻塞在 X 锁上
//	T2  A: Delete(id=X) + Create(new chest id=Y) → uk_user_id 仍指向 user_id
//	T3  A: commit → 锁释放
//	T4  B: UPDATE 恢复 → WHERE user_id=? **重新匹配** → 命中 id=Y (next chest)
//	T5  Y 被推到 unlock_at=now → 用户连开 2 次（next chest 已 unlockable）
//
// **r1 改造后**：service 用 txManager.WithTx 包"先 FOR UPDATE SELECT 拿 id → UPDATE WHERE id"。
// FOR UPDATE 让 B 阻塞到 A commit 之后，B 的 SELECT 拿到的是 **commit 后的当前 chest**
// （即新插入的 Y），然后 UPDATE WHERE id=Y 也是把"当前"chest 推到 now —— 符合"force-unlock current"语义，
// **不再**因 WHERE user_id 二次匹配跑偏到 future chest。
//
// 这是"FOR UPDATE 后做 UPDATE WHERE-ID-of-fetched-row，避免 WHERE 二次匹配跑偏"
// 通用模式的一个实例（详见 docs/lessons/2026-05-15-...）。
type DevChestService interface {
	// ForceUnlockChest 把指定 userID 的当前 chest unlock_at 推到 now() UTC。
	//
	// **事务内 2 步**：
	//  1. chestRepo.FindByUserIDForUpdate(txCtx, userID) → 拿到 current chest 的 id（FOR UPDATE 持锁）
	//  2. chestRepo.UpdateUnlockAtByID(txCtx, chest.ID, time.Now().UTC()) → UPDATE WHERE id=?
	//
	// **不**改 chest.status 字段（Story 20.5 钦定 DB 字面 status 不动；动态判定全靠 unlock_at vs now）。
	// **不**改 chest.version 字段（dev 端点不参与乐观锁串行化，与 Story 20.6 OpenChest 持锁路径独立）。
	// **不**接 unlock_at 参数（dev 产品语义是"立刻可开"；未来如需"滚动倒计时 demo"加独立端点）。
	ForceUnlockChest(ctx context.Context, userID uint64) error
}

// devChestServiceImpl 是 DevChestService 的默认实装。
type devChestServiceImpl struct {
	txMgr     tx.Manager
	chestRepo mysql.ChestRepo
}

// NewDevChestService 构造 DevChestService。
//
// 依赖：
//   - txMgr：事务边界控制（FindByUserIDForUpdate + UpdateUnlockAtByID 必须同事务，
//     让 FOR UPDATE 锁贯穿到 commit；详见 interface doc r1 [P2] race 分析）
//   - chestRepo：FindByUserIDForUpdate 拿 current chest id + UpdateUnlockAtByID 强制更新 unlock_at
//
// 与 NewDevStepService (Story 7.5) 平级：都注入 txMgr（dev 端点也可能需要事务原子性）。
// **不**接 userRepo（FindByUserIDForUpdate rows_affected=0 已经表达"用户无 chest"语义，
// 不需要额外的 FindByID(user) 前置验证 —— dev 端点容忍 "user 不存在" 与 "chest 不存在" 都映射 1003）；
// **不**接 stepAccountRepo / stepSyncLogRepo（dev force-unlock 不读写步数账户）；
// **不**接 envName（dev 端点已被 build tag / env var 双闸门防 prod，不再做 envName 检查）。
func NewDevChestService(txMgr tx.Manager, chestRepo mysql.ChestRepo) DevChestService {
	return &devChestServiceImpl{txMgr: txMgr, chestRepo: chestRepo}
}

// ForceUnlockChest 实装：事务内 SELECT ... FOR UPDATE → UPDATE WHERE id=?。
//
// **time.Now().UTC()**：必须 UTC（与 V1 §2.5 ISO 8601 UTC 钦定一致；与 chest.UnlockAt
// 字段 UTC 语义对齐 —— chest_repo.go 顶部注释钦定，Story 4.6 firstTimeLogin
// 也用 time.Now().UTC().Add(10*time.Minute)，与本 story now() 同源 UTC）。
//
// **txCtx 透传**：事务 fn 内**所有** repo 调用必须用 txCtx（ADR-0007 §2.4 钦定），
// 否则 FOR UPDATE 锁不会覆盖到 UpdateUnlockAtByID，race 修复就破功了。
func (s *devChestServiceImpl) ForceUnlockChest(ctx context.Context, userID uint64) error {
	now := time.Now().UTC()
	return s.txMgr.WithTx(ctx, func(txCtx context.Context) error {
		// (1) FOR UPDATE 锁拿当前 chest 的 id —— 让本事务串行化在 OpenChest commit 之后
		chest, err := s.chestRepo.FindByUserIDForUpdate(txCtx, userID)
		if err != nil {
			if stderrors.Is(err, mysql.ErrChestNotFound) {
				// epics.md §20.7 行 2947 钦定：用户无 chest → 1003（**非** 4001）
				return apperror.Wrap(err, apperror.ErrResourceNotFound, apperror.DefaultMessages[apperror.ErrResourceNotFound])
			}
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}

		// (2) UPDATE WHERE id=chest.ID —— 不用 WHERE user_id 二次匹配（race 修复核心）
		if err := s.chestRepo.UpdateUnlockAtByID(txCtx, chest.ID, now); err != nil {
			if stderrors.Is(err, mysql.ErrChestNotFound) {
				// 理论不可达：步 (1) FOR UPDATE 锁覆盖到 commit；并发 OpenChest 不能在
				// 锁释放前 Delete 本 chest 行。防御性映射 1003 保持错误码一致性。
				return apperror.Wrap(err, apperror.ErrResourceNotFound, apperror.DefaultMessages[apperror.ErrResourceNotFound])
			}
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}

		slog.WarnContext(txCtx, "dev force-unlock-chest applied",
			"user_id", userID, "chest_id", chest.ID, "unlock_at", now.Format(time.RFC3339),
		)
		return nil
	})
}
