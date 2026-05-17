package service

// Story 20.6 — chest_open_service.go：OpenChest 8 步事务实装（V1 §7.2.5 + DB §5.16 +
// §8.3 钦定）。
//
// 范围：本文件落地 ChestService.OpenChest 方法、OpenChestInput / OpenChestOutput /
// ChestRewardBrief DTO、cacheableResponse 中间结构 + 序列化 / 反序列化 helper。
//
// 与 chest_service.go 关系：
//   - chest_service.go 持有 ChestService interface + chestServiceImpl struct +
//     NewChestService 构造 + GetCurrent 实装（Story 20.5）
//   - 本文件追加 OpenChest 方法到既有 chestServiceImpl + DTO 类型 + 内部 helper
//
// **决策**（V1 §7.2 r1~r15 + epics.md §20.6）：
//   - r5: DB 持久化幂等替代 Redis（避 Redis 非事务写回失败导致重复出箱风险）
//   - r6: 幂等预声明 INSERT 在业务事务首条语句
//   - r7: schema 二态机（pending / success）
//   - r9: response_json 缓存不含 nextChest.status / nextChest.remainingSeconds
//   - r10: rate_limit 在 handler 内层，service 层不感知
//   - r11: time-derived 字段同源同时刻重算 + MVCC pending 不可见 + 1008 退役
//   - r15: 1008 在本接口节点 7 不可达，service 兜底分支翻译为 1009
//   - 23.5: 节点 8 入仓 —— 5g 与 5h 之间插 user_cosmetic_items INSERT（5g.5），
//     回填 reward id 三处（5h log / 5j output / buildCacheableResponse 透传），
//     同 txCtx 原子提交；5a~5f / 5i / 5k race-fix 不变量一律不动

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/random"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// 业务常量（V1 §7.2.5 + DB §5.6 钦定）。
const (
	// chestOpenCostSteps: 开箱固定消耗步数（V1 §7.2.5 + epics.md §FR13 钦定；节点 7 阶段固定 1000）
	chestOpenCostSteps uint64 = 1000

	// chestRefreshNextDelay: 下一轮 chest unlock_at 偏移（V1 §7.2.5i + epics.md §FR11 钦定；
	// 节点 7 阶段固定 10 分钟）
	chestRefreshNextDelay = 10 * time.Minute

	// idempotencyKeyMinLength / idempotencyKeyMaxLength: V1 §7.2 + DB §5.16 钦定
	// 1 ≤ length ≤ 128 + 字符集 [A-Za-z0-9_:-]（字符集校验在 handler 层做 regex；本 service
	// 仅做 length 兜底防御 —— 与 handler 双重校验，避免 handler 误漏校直接进 service）
	idempotencyKeyMinLength = 1
	idempotencyKeyMaxLength = 128
)

// OpenChestInput 是 ChestService.OpenChest 输入 DTO（handler → service 转换）。
type OpenChestInput struct {
	UserID         uint64 // auth 中间件注入；handler 兜底校验非 0
	IdempotencyKey string // V1 §7.2 钦定 1-128 字符集 [A-Za-z0-9_:-]；handler 已 regex 校验
	RequestID      string // 顶层 trace ID（handler 从 c.Get(RequestIDKey) 取）；用于响应填充，**不**写入 response_json 缓存（V1 §7.2 r7 锁定）
}

// OpenChestOutput 是 ChestService.OpenChest 输出 DTO（handler 转译为 V1 §7.2 wire DTO）。
//
// 字段构造规则（V1 §7.2.5j 钦定）：
//   - Reward / StepAccount / NextChest 由 service 层填充
//   - **NextChest.Status / RemainingSeconds 由 handler 层补算**（time-derived；
//     service 透传 UnlockAt 即可）—— 同源同时刻规则（V1 §7.2 r11）
type OpenChestOutput struct {
	Reward      ChestRewardBrief
	StepAccount StepAccountBrief
	NextChest   ChestBrief // 注意：Status / RemainingSeconds 由 handler 按 (UnlockAt > now ? 1 : 2) 与 max(0, ceil((UnlockAt-now)/1s)) 补算
}

// ChestRewardBrief: 开箱奖励三段嵌套之 reward 段。
type ChestRewardBrief struct {
	UserCosmeticItemID uint64 // **Story 23.5 节点 8 起回填真实 user_cosmetic_items.id**（V1 §7.2.4h + DB §5.7 注解；节点 7 阶段曾固定 0 占位）
	CosmeticItemID     uint64 // 真实 cosmetic_items.id
	Name               string // cosmetic_items.name
	Slot               int8   // cosmetic_items.slot
	Rarity             int8   // cosmetic_items.rarity
	AssetURL           string // cosmetic_items.asset_url
	IconURL            string // cosmetic_items.icon_url
}

// cacheableResponse 是写入 chest_open_idempotency_records.response_json 的子集；
// **不**含 NextChest.Status / NextChest.RemainingSeconds（time-derived） / 顶层 requestId
// （每次请求独立 trace ID）。
//
// JSON 字段命名与 V1 §7.2 wire DTO 一致（便于 future 直接 echo back without re-translate）。
type cacheableResponse struct {
	Code    int                  `json:"code"`
	Message string               `json:"message"`
	Data    cacheableResponseDTO `json:"data"`
}

type cacheableResponseDTO struct {
	Reward      cacheableRewardDTO      `json:"reward"`
	StepAccount cacheableStepAccountDTO `json:"stepAccount"`
	NextChest   cacheableNextChestDTO   `json:"nextChest"`
}

type cacheableRewardDTO struct {
	UserCosmeticItemID uint64 `json:"userCosmeticItemId"`
	CosmeticItemID     uint64 `json:"cosmeticItemId"`
	Name               string `json:"name"`
	Slot               int8   `json:"slot"`
	Rarity             int8   `json:"rarity"`
	AssetURL           string `json:"assetUrl"`
	IconURL            string `json:"iconUrl"`
}

type cacheableStepAccountDTO struct {
	TotalSteps     uint64 `json:"totalSteps"`
	AvailableSteps uint64 `json:"availableSteps"`
	ConsumedSteps  uint64 `json:"consumedSteps"`
}

type cacheableNextChestDTO struct {
	ID            uint64    `json:"id"`
	UnlockAt      time.Time `json:"unlockAt"`
	OpenCostSteps uint32    `json:"openCostSteps"`
	// **不**含 Status / RemainingSeconds（time-derived；handler 同源同时刻补算）
}

// OpenChest 实装（V1 §7.2.5 8 步事务 + handler 内层 rate_limit 不在本 service 做）。
//
// **本 service 不做 rate_limit 检查**（V1 §7.2.5.4 r10 钦定 rate_limit 在 handler 内层）；
// 本 service 入口只做：
//  1. 入参校验（idempotencyKey length 兜底；非业务必走，handler 已校验）
//  2. 步骤 3: 幂等命中预检（autocommit SELECT idempotency 行）→ 命中 success → 直接返 cached
//  3. 步骤 5: 业务事务（txMgr.WithTx fn）：5a 预声明 → 5b 短路 / 5c-l 全流程
//
// 步骤 4 (rate_limit) 由 handler 层做；本 service 不感知。
//
// 错误码翻译（service 层完成；handler 仅 c.Error + return）：
//   - chest NotFound (5c) → ErrChestNotFound (4001)
//   - chest 不可解锁 (5d) → ErrChestNotUnlocked (4002)
//   - step_account NotFound (5e) → ErrServiceBusy (1009) —— V1 §7.2 1009 行钦定（数据完整性异常）
//   - step_account.available_steps < 1000 (5e) → ErrInsufficientSteps (3002)
//   - 乐观锁 (5f rows_affected=0) → ErrServiceBusy (1009)
//   - cosmetic_items enabled 为空 (5g) → ErrServiceBusy (1009)
//   - 任何其他 DB 错 → ErrServiceBusy (1009)
//   - 步骤 5b 兜底分支读到 status='pending' → ErrServiceBusy (1009)（V1 §7.2 r11 钦定，**非** 1008）
func (s *chestServiceImpl) OpenChest(ctx context.Context, in OpenChestInput) (*OpenChestOutput, error) {
	// 入参校验（兜底；handler 已校验过 regex + length）
	if in.UserID == 0 {
		return nil, apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	if len(in.IdempotencyKey) < idempotencyKeyMinLength || len(in.IdempotencyKey) > idempotencyKeyMaxLength {
		return nil, apperror.New(apperror.ErrInvalidParam, "idempotencyKey length out of range")
	}

	// 步骤 3: committed success 幂等命中预检（autocommit SELECT；MVCC 决定 pending 不可见）
	cached, err := s.idempotencyRepo.FindByUserIDAndKey(ctx, in.UserID, in.IdempotencyKey)
	if err != nil && !stderrors.Is(err, mysql.ErrIdempotencyRecordNotFound) {
		// DB 错（非 NotFound 哨兵）→ 1009
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	if cached != nil && cached.Status == mysql.IdempotencyStatusSuccess {
		// committed success replay → 反序列化 response_json + 返回（handler 层补 status / remainingSeconds / requestId）
		// **注**：V1 §7.2.3 钦定本路径**不**进业务事务 + **不**走步骤 4 rate_limit；handler 已用
		// 同款 idempotencyRepo SELECT 做"是否需要 rate_limit"的决策。
		return s.replayFromCachedResponse(cached.ResponseJSON)
	}

	// 未命中或 cached.Status='pending'（理论上不可观察到 pending；MVCC 让 autocommit 看不到首事务的 pending；
	// 但若 driver 异常读到 pending，此处按 1009 兜底 —— V1 §7.2 r11 钦定不走 1008）
	if cached != nil && cached.Status == mysql.IdempotencyStatusPending {
		return nil, apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 步骤 5: 业务事务
	var output *OpenChestOutput
	err = s.txMgr.WithTx(ctx, func(txCtx context.Context) error {
		// 5a: 幂等预声明 INSERT（事务首条语句，借 UNIQUE 做 single-statement 原子 claim）
		affectedRows, err := s.idempotencyRepo.ClaimPending(txCtx, in.UserID, in.IdempotencyKey)
		if err != nil {
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}

		if affectedRows == 0 {
			// 5b: 短路分支（行已存在，且首事务已 commit）
			// 在同一事务内 SELECT → status 必然 'success'（V1 §7.2.5b r11 锁定）
			rec, err := s.idempotencyRepo.FindByUserIDAndKey(txCtx, in.UserID, in.IdempotencyKey)
			if err != nil {
				return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
			}
			if rec.Status != mysql.IdempotencyStatusSuccess {
				// 理论不应观察到 pending（详见 V1 §7.2.5b 注解）；按 1009 兜底（**非** 1008）
				return apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
			}
			out, err := s.replayFromCachedResponse(rec.ResponseJSON)
			if err != nil {
				return err
			}
			output = out
			return nil
		}

		// affectedRows = 1 → 走步骤 5c 业务全流程
		out, err := s.runOpenChestTx(txCtx, in)
		if err != nil {
			return err
		}
		output = out
		return nil
	})
	if err != nil {
		return nil, err
	}
	return output, nil
}

// runOpenChestTx 步骤 5c~5l 业务全流程（事务内调用）。
//
// **关键**：本函数内所有 repo 调用必须用传入的 `txCtx`（**不**是外层 ctx）；
// 与 ADR-0007 §2.4 钦定一致。
func (s *chestServiceImpl) runOpenChestTx(txCtx context.Context, in OpenChestInput) (*OpenChestOutput, error) {
	// 5c: SELECT user_chests ... FOR UPDATE
	chest, err := s.chestRepo.FindByUserIDForUpdate(txCtx, in.UserID)
	if err != nil {
		if stderrors.Is(err, mysql.ErrChestNotFound) {
			return nil, apperror.Wrap(err, apperror.ErrChestNotFound, apperror.DefaultMessages[apperror.ErrChestNotFound])
		}
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 5d: 判定 unlockable（V1 §7.1.4 同公式：DB status=2 或 (DB status=1 AND unlock_at <= now)）
	now := s.nowFn()
	isUnlockable := chest.Status == 2 || (chest.Status == 1 && !chest.UnlockAt.After(now))
	if !isUnlockable {
		return nil, apperror.New(apperror.ErrChestNotUnlocked, apperror.DefaultMessages[apperror.ErrChestNotUnlocked])
	}

	// 5e: SELECT user_step_accounts ... FOR UPDATE
	account, err := s.stepAccountRepo.FindByUserIDForUpdate(txCtx, in.UserID)
	if err != nil {
		if stderrors.Is(err, mysql.ErrStepAccountNotFound) {
			// V1 §7.2 1009 行钦定：account 行缺失视为数据完整性异常，**非** 3002
			return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	if account.AvailableSteps < chestOpenCostSteps {
		return nil, apperror.New(apperror.ErrInsufficientSteps, apperror.DefaultMessages[apperror.ErrInsufficientSteps])
	}

	// 5f: 扣步数（available_steps - 1000, consumed_steps + 1000, version + 1）
	err = s.stepAccountRepo.Spend(txCtx, in.UserID, chestOpenCostSteps, account.Version)
	if err != nil {
		// 乐观锁失败 / DB error 都翻译为 1009（V1 §7.2 1009 行钦定）
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 5g: 加权抽取 cosmetic_items
	items, err := s.cosmeticItemRepo.ListEnabledForWeightedPick(txCtx)
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	if len(items) == 0 {
		// seed 未执行 → 1009（V1 §7.2 1009 行钦定）
		return nil, apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	weightedItems := make([]random.WeightedItem, len(items))
	for i, item := range items {
		weightedItems[i] = random.WeightedItem{Weight: uint64(item.DropWeight)}
	}
	pickedIndex, err := s.weightedPicker.Pick(weightedItems)
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	pickedItem := items[pickedIndex]

	// 5g.5: 创建 user_cosmetic_items 实例（Story 23.5 节点 8 入仓；epics.md §23.5 +
	// V1 §7.2.4h 节点 8 + DB §8.3"插入一条 user_cosmetic_items"钦定）。
	// **必须在 5h 写 chest_open_logs 之前** —— 要先拿到 user_cosmetic_items.id 才能
	// 回填 chest_open_logs.reward_user_cosmetic_item_id（之前节点 7 阶段固定 0）。
	// 全部在 txCtx 同事务（ADR-0007 §2.4 + DB §8.3）—— 任一步失败本 INSERT 跟随回滚。
	chestID := chest.ID
	newItem := &mysql.UserCosmeticItem{
		UserID:         in.UserID,
		CosmeticItemID: pickedItem.ID, // 5g 抽中的配置 id
		Status:         1,             // 1=in_bag（§6.10 + struct 注释钦定）
		Source:         1,             // 1=chest（§6.11 + struct 注释钦定）
		SourceRefID:    &chestID,      // 被开启的宝箱 id（epics.md 行 3306 + struct 注释钦定；*uint64 非空指针）
		ObtainedAt:     now,           // 复用 5d 已取的 now（同源同时刻，**不**重新 time.Now()）
	}
	if err := s.userCosmeticItemRepo.CreateInTx(txCtx, newItem); err != nil {
		// 与同事务其他写步骤一致包成 1009（V1 §7.2 "任何其他 DB 错 → 1009"）
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	// newItem.ID 已由 GORM 回填（AUTO_INCREMENT）—— 用于 5h logRow + 5j output 回填

	// 5h: 写 chest_open_logs（Story 23.5 节点 8 回填真实 user_cosmetic_items.id）
	logRow := &mysql.ChestOpenLog{
		UserID:                   in.UserID,
		ChestID:                  chest.ID,
		CostSteps:                uint32(chestOpenCostSteps),
		RewardUserCosmeticItemID: newItem.ID, // Story 23.5 节点 8 回填真实 user_cosmetic_items.id（节点 7 阶段曾固定 0）
		RewardCosmeticItemID:     pickedItem.ID,
		RewardRarity:             pickedItem.Rarity,
	}
	if err := s.chestOpenLogRepo.Create(txCtx, logRow); err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 5i: 刷新下一轮 chest（DELETE 旧 + INSERT 新）
	if err := s.chestRepo.Delete(txCtx, chest.ID); err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	nextChest := &mysql.UserChest{
		UserID:        in.UserID,
		Status:        1, // counting
		UnlockAt:      now.Add(chestRefreshNextDelay),
		OpenCostSteps: uint32(chestOpenCostSteps),
		Version:       0,
	}
	if err := s.chestRepo.Create(txCtx, nextChest); err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 5j: 序列化可缓存 response payload
	output := &OpenChestOutput{
		Reward: ChestRewardBrief{
			UserCosmeticItemID: newItem.ID, // Story 23.5 节点 8 真实 user_cosmetic_items.id（节点 7 阶段曾固定 0）
			CosmeticItemID:     pickedItem.ID,
			Name:               pickedItem.Name,
			Slot:               pickedItem.Slot,
			Rarity:             pickedItem.Rarity,
			AssetURL:           pickedItem.AssetURL,
			IconURL:            pickedItem.IconURL,
		},
		StepAccount: StepAccountBrief{
			TotalSteps:     account.TotalSteps,
			AvailableSteps: account.AvailableSteps - chestOpenCostSteps,
			ConsumedSteps:  account.ConsumedSteps + chestOpenCostSteps,
		},
		NextChest: ChestBrief{
			ID:            nextChest.ID, // GORM Create 后回填
			UnlockAt:      nextChest.UnlockAt.UTC(),
			OpenCostSteps: nextChest.OpenCostSteps,
			// Status / RemainingSeconds 由 handler 层补算
		},
	}

	// 序列化 cacheable response_json（subset of output：不含 time-derived 字段 + 不含 requestId）
	cacheable := buildCacheableResponse(output)
	responseJSON, err := json.Marshal(cacheable)
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 5k: UPDATE idempotency.status='success' + response_json
	if err := s.idempotencyRepo.MarkSuccess(txCtx, in.UserID, in.IdempotencyKey, responseJSON); err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 5l: 事务由 WithTx 自动 commit（fn return nil）
	return output, nil
}

// buildCacheableResponse 构造 cacheableResponse —— OpenChestOutput 的子集，
// **不**含 NextChest.Status / NextChest.RemainingSeconds（time-derived；handler 补算）
// + **不**含顶层 requestId（每次请求独立）。
func buildCacheableResponse(out *OpenChestOutput) cacheableResponse {
	return cacheableResponse{
		Code:    0,
		Message: "ok",
		Data: cacheableResponseDTO{
			Reward: cacheableRewardDTO{
				UserCosmeticItemID: out.Reward.UserCosmeticItemID,
				CosmeticItemID:     out.Reward.CosmeticItemID,
				Name:               out.Reward.Name,
				Slot:               out.Reward.Slot,
				Rarity:             out.Reward.Rarity,
				AssetURL:           out.Reward.AssetURL,
				IconURL:            out.Reward.IconURL,
			},
			StepAccount: cacheableStepAccountDTO{
				TotalSteps:     out.StepAccount.TotalSteps,
				AvailableSteps: out.StepAccount.AvailableSteps,
				ConsumedSteps:  out.StepAccount.ConsumedSteps,
			},
			NextChest: cacheableNextChestDTO{
				ID:            out.NextChest.ID,
				UnlockAt:      out.NextChest.UnlockAt.UTC(),
				OpenCostSteps: out.NextChest.OpenCostSteps,
			},
		},
	}
}

// replayFromCachedResponse 反序列化 cached response_json → OpenChestOutput。
// handler 层后续按当前时刻补算 NextChest.Status + RemainingSeconds + 顶层 requestId。
func (s *chestServiceImpl) replayFromCachedResponse(responseJSON []byte) (*OpenChestOutput, error) {
	var cached cacheableResponse
	if err := json.Unmarshal(responseJSON, &cached); err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	return &OpenChestOutput{
		Reward: ChestRewardBrief{
			UserCosmeticItemID: cached.Data.Reward.UserCosmeticItemID,
			CosmeticItemID:     cached.Data.Reward.CosmeticItemID,
			Name:               cached.Data.Reward.Name,
			Slot:               cached.Data.Reward.Slot,
			Rarity:             cached.Data.Reward.Rarity,
			AssetURL:           cached.Data.Reward.AssetURL,
			IconURL:            cached.Data.Reward.IconURL,
		},
		StepAccount: StepAccountBrief{
			TotalSteps:     cached.Data.StepAccount.TotalSteps,
			AvailableSteps: cached.Data.StepAccount.AvailableSteps,
			ConsumedSteps:  cached.Data.StepAccount.ConsumedSteps,
		},
		NextChest: ChestBrief{
			ID:            cached.Data.NextChest.ID,
			UnlockAt:      cached.Data.NextChest.UnlockAt.UTC(),
			OpenCostSteps: cached.Data.NextChest.OpenCostSteps,
			// Status / RemainingSeconds 由 handler 层按当前时刻补算
		},
	}, nil
}
