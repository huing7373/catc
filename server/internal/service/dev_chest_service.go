package service

import (
	"context"
	stderrors "errors"
	"log/slog"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
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
// **r1 实装**：service 改 FOR UPDATE SELECT 拿 chest.id → UPDATE WHERE id = ? 两步同事务 —— 看似能让
//   force-unlock 等到 OpenChest commit 后串行化执行，但 FOR UPDATE 阻塞结束后 SELECT 返回的是 commit
//   后的"当前 chest"（即 OpenChest 刚 INSERT 的 next chest Y），还是跑偏。
// **r2 实装（当前）**：把"哪个 chest"的决策权交给 client —— client 必须先 GET /chest/current 拿到当前
//   chest.id，再 POST /dev/force-unlock-chest 带这个 id。service 用 FindByID 校验 (a) chest 存在
//   (b) chest.user_id == claimedUserID（防越权 unlock 他人 chest），UPDATE WHERE id 直接动 unlock_at。
//   race 不再成立 —— 因为 server 不再猜"current"语义。
//
// # 为何不用事务（r2 改造后）+ race 处理（r3 改造）
//
// chest 一旦绑定 user_id，user_id 字段永不变；UPDATE 走 PK 索引。
//
// **r3 [P2] 改造**：FindByID 拿到 chest + 校验归属后到 UpdateUnlockAtByID 之间存在二阶 race：
// 若另一并发 /chest/open 删除 chest，UPDATE 会命中 0 行。r2 把"0 行视为成功"是 over-correction，
// 导致 dev 端点 false success（声称 unlock 成功但 GET /chest/current 仍 counting）。r3 在 repo 层
// 把 RowsAffected==0 翻译为 ErrChestNotFound 哨兵 → service 透传翻译为 1003 → client 重新 GET
// /chest/current 拿新 chest.id 后重试。仍无须事务（user_id 不会改 + 重试可恢复）。
//
// # 错误约定（ADR-0006 三层映射）
//
//   - mysql.ErrChestNotFound（FindByID 没行）→ 包成 ErrResourceNotFound (1003)
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
	// **2 步**（不开事务）：
	//  1. chestRepo.FindByID(ctx, chestID) → 拿到 chest，校验存在 + chest.UserID == userID
	//  2. chestRepo.UpdateUnlockAtByID(ctx, chestID, time.Now().UTC()) → UPDATE WHERE id=?
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
	chestRepo mysql.ChestRepo
}

// NewDevChestService 构造 DevChestService。
//
// **r2 [P2] 改造**：移除 txMgr 参数 —— 详见 interface doc 改造历史。
//
// 依赖：
//   - chestRepo：FindByID 拿 chest 校验存在 + 归属 + UpdateUnlockAtByID 强制更新 unlock_at
//
// **不**接 userRepo（FindByID rows_affected=0 已表达"chest 不存在"语义；越权由 chest.user_id 校验防御）；
// **不**接 stepAccountRepo / stepSyncLogRepo（dev force-unlock 不读写步数账户）；
// **不**接 envName（dev 端点已被 build tag / env var 双闸门防 prod）。
func NewDevChestService(chestRepo mysql.ChestRepo) DevChestService {
	return &devChestServiceImpl{chestRepo: chestRepo}
}

// ForceUnlockChest 实装：FindByID 校验 → UPDATE WHERE id。
//
// **time.Now().UTC()**：必须 UTC（与 V1 §2.5 ISO 8601 UTC 钦定一致；与 chest.UnlockAt
// 字段 UTC 语义对齐 —— chest_repo.go 顶部注释钦定，Story 4.6 firstTimeLogin
// 也用 time.Now().UTC().Add(10*time.Minute)，与本 story now() 同源 UTC）。
//
// **r2 改造**：不再走 txMgr.WithTx —— FindByID + UpdateUnlockAtByID 各自独立 DB call；
// race 不再成立（详见 interface doc）。
func (s *devChestServiceImpl) ForceUnlockChest(ctx context.Context, userID uint64, chestID uint64) error {
	// (1) 取 chest，校验存在 + 归属 user
	chest, err := s.chestRepo.FindByID(ctx, chestID)
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
		slog.WarnContext(ctx, "dev force-unlock-chest cross-user attempt",
			"claimed_user_id", userID, "chest_id", chestID, "chest_owner_user_id", chest.UserID,
		)
		return apperror.New(apperror.ErrResourceNotFound, apperror.DefaultMessages[apperror.ErrResourceNotFound])
	}

	// (2) UPDATE WHERE id=chestID。r3 [P2] 改造：repo 在 RowsAffected==0 时返 ErrChestNotFound 哨兵
	// （二阶 race —— FindByID 后 chest 被并发 OpenChest 删除），service 透传翻译为 1003。
	now := time.Now().UTC()
	if err := s.chestRepo.UpdateUnlockAtByID(ctx, chestID, now); err != nil {
		if stderrors.Is(err, mysql.ErrChestNotFound) {
			// r3 [P2]: 二阶 race（FindByID 后 chest 被并发删除）→ UPDATE 0 行 → 1003。
			// 与步骤 (1) FindByID NotFound 同码（client 重试时 GET /chest/current 拿新 id）。
			return apperror.Wrap(err, apperror.ErrResourceNotFound, apperror.DefaultMessages[apperror.ErrResourceNotFound])
		}
		return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	slog.WarnContext(ctx, "dev force-unlock-chest applied",
		"user_id", userID, "chest_id", chestID, "unlock_at", now.Format(time.RFC3339),
	)
	return nil
}
