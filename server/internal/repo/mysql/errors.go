// Package mysql 提供基于 GORM + database/sql 的 MySQL repo 实装。
//
// 本包是节点 2 §Story 4.6 起的真实业务 repo 落地点，承载 users / pets /
// user_auth_bindings / user_step_accounts / user_chests 等表的 CRUD 抽象。
// 后续 Epic 7 / 11 / 14 / 17 / 20 / 23 / 26 / 32 都会往本包加新的 repo 文件。
//
// # 分层约束（与 docs/宠物互动App_Go项目结构与模块职责设计.md §5.3 对齐）
//
//   - repo 只做单表 CRUD + 错误识别（如 ER_DUP_ENTRY 1062 → ErrAuthBindingDuplicate）
//   - repo **不**承载业务规则（业务常量如 petTypeDefault=1 在 service 层定义）
//   - repo **不**调 txMgr.WithTx —— 事务边界归 service 控制
//   - repo **不**返业务码 / *AppError —— 只产 raw error / sentinel error，由 service 层
//     用 apperror.Wrap 翻译为业务码（ADR-0006 三层映射）
//
// # ctx 传播（ADR-0007 §2.3）
//
// 每个 repo 方法第一参数 ctx context.Context；所有 GORM 调用都必须 .WithContext(ctx)，
// 否则 ctx cancel（client 断开 / deadline）不会传播到 driver 层 → SQL 不会被中断。
//
// # 事务感知（tx.FromContext 模式）
//
// repo 内部用 tx.FromContext(ctx, r.db) 取 db handle：
//
//   - ctx 携带 tx（即 service 层 txMgr.WithTx 传入的 txCtx）→ 返 tx 句柄
//   - ctx 不带 tx → 返 fallback（即 r.db，repo 持有的外层 db pool）
//
// 这让同一个 repo 方法**既**能在事务内被调（用 txCtx），**又**能在事务外被调（用普通 ctx），
// 业务层无需为"是否在事务内"维护两套 API。详见 internal/repo/tx/manager.go 顶部注释。
package mysql

import "errors"

// 哨兵 error：service 层用 errors.Is 区分**业务可识别**的失败 vs **DB 异常**。
//
//   - ErrAuthBindingNotFound: FindByGuestUID 查不到行（非异常 —— 用于"首次登录" vs "复用登录"分支）
//   - ErrAuthBindingDuplicate: Create 时 UNIQUE(auth_type, auth_identifier) 冲突
//     （并发场景：两个并发请求同 guestUid，先入者已写入 binding 后第二个 INSERT 触发
//     MySQL ER_DUP_ENTRY 1062 → service 层捕获后回退到 reuseLogin 分支）
//   - ErrUsersGuestUIDDuplicate: users.Create 时 UNIQUE(guest_uid) `uk_guest_uid` 冲突
//     （并发场景：两个并发请求同 guestUid，先入者 Tx A 已 commit users 行 → Tx B
//     INSERT users 触发 MySQL ER_DUP_ENTRY 1062 → 由于 firstTimeLogin 内 users 是
//     **第一步**，比 user_auth_bindings 更早抛冲突 —— 必须独立哨兵 + 与
//     ErrAuthBindingDuplicate 同样回退到 reuseLogin。**不同表的唯一约束需要**
//     **独立哨兵** —— 用同一个会让 service 误以为冲突源是 binding 表）
//   - ErrUserNotFound: FindByID 查不到（理论不应发生 —— binding 存在但 user 不存在 → 数据脏）
//   - ErrPetNotFound: FindDefaultByUserID 查不到（理论不应发生 —— user 创建后必有默认 pet）
//   - ErrStepAccountNotFound: StepAccountRepo.FindByUserID 查不到（Story 4.6 firstTimeLogin
//     必建一行；查不到 → 数据脏，service 包成 1009）
//   - ErrChestNotFound: ChestRepo.FindByUserID 查不到（同上，登录初始化必建）
//
// 其他 DB 异常（连接断 / SQL 语法错 / 死锁等）原样透传给 service，由 service 兜底
// wrap 成 ErrServiceBusy(1009)。
var (
	ErrAuthBindingNotFound    = errors.New("mysql: auth binding not found")
	ErrAuthBindingDuplicate   = errors.New("mysql: auth binding duplicate (uk_auth_type_identifier conflict)")
	ErrUsersGuestUIDDuplicate = errors.New("mysql: users guest_uid duplicate (uk_guest_uid conflict)")
	ErrUserNotFound           = errors.New("mysql: user not found")
	ErrPetNotFound            = errors.New("mysql: default pet not found")
	ErrStepAccountNotFound    = errors.New("mysql: step_account not found")
	ErrChestNotFound          = errors.New("mysql: chest not found")

	// ErrStepSyncLogNotFound: StepSyncLogRepo.FindLatestByUserAndDate 查不到（合法：
	// 用户当日首次同步）。**不**包成 1009 / 1003 错误：service 层会用 errors.Is 捕获后
	// 走"首次同步 delta = clientTotalSteps"分支。Story 7.3 引入。
	ErrStepSyncLogNotFound = errors.New("mysql: step_sync_log not found")

	// ErrStepAccountVersionMismatch: StepAccountRepo.UpdateBalance 乐观锁失败
	// （WHERE version = ? 不匹配 → rows affected = 0）。service 层包成 1009
	// （节点 3 阶段无 retry；客户端下次主动 sync 时重试）。Story 7.3 引入。
	ErrStepAccountVersionMismatch = errors.New("mysql: step_account version mismatch (optimistic lock conflict)")

	// ErrRoomMembersUserIDDuplicate: RoomMemberRepo.Create 时 UNIQUE(user_id) `uk_user_id`
	// 冲突。语义："用户已在某个房间中"（V1接口设计.md §10.1 / §10.4 / 数据库设计.md §7.1）。
	// service 层用 errors.Is 识别后翻译为 6003 ErrUserAlreadyInRoom。
	// 由 Story 11.3 (POST /rooms) 和 Story 11.4 (POST /rooms/{roomId}/join) 共同消费。
	ErrRoomMembersUserIDDuplicate = errors.New("mysql: room_members user_id duplicate (uk_user_id conflict)")

	// ErrRoomMembersRoomUserDuplicate: RoomMemberRepo.Create 时 UNIQUE(room_id, user_id)
	// `uk_room_user` 冲突。语义："同一用户已在该房间中"（理论与 uk_user_id 兜底等价；
	// 但分两个独立哨兵让 service 层日志能区分哪个约束被打破，便于审计 / debug）。
	// service 层同样翻译为 6003 ErrUserAlreadyInRoom（Story 11.3 / 11.4 共同消费）。
	ErrRoomMembersRoomUserDuplicate = errors.New("mysql: room_members (room_id, user_id) duplicate (uk_room_user conflict)")

	// ErrRoomNotFound: RoomRepo.FindByIDForUpdate / FindByID 查不到 rooms 行。
	// 语义："房间不存在"（V1接口设计.md §10.4 步骤 2 钦定 6001）。
	// service 层用 errors.Is 识别后翻译为 6001 apperror.ErrRoomNotFound。
	// **包路径区分**：mysql.ErrRoomNotFound 是 repo 哨兵；apperror.ErrRoomNotFound
	// 是业务码 6001；同名不同包是故意的，让阅读对照容易。
	// 由 Story 11.4 (POST /rooms/{roomId}/join) 引入；Story 11.5 (leave) 也会消费同哨兵。
	ErrRoomNotFound = errors.New("mysql: room not found")

	// ErrIdempotencyRecordNotFound: IdempotencyRepo.FindByUserIDAndKey 查不到行
	// （合法 case —— 首次到达；service 层用 errors.Is 区分语义后走主流程）；
	// 同样由 IdempotencyRepo.MarkSuccess rows_affected=0 返回（理论不应发生 ——
	// 同事务前面 ClaimPending 必已 INSERT；实际触发说明上游调用顺序错乱，
	// 按 1009 透传）。由 Story 20.6 引入。
	ErrIdempotencyRecordNotFound = errors.New("mysql: idempotency record not found")

	// ErrUserCosmeticItemNotFound: UserCosmeticItemRepo.FindByIDForEquip 按实例
	// id 查不到行（V1 §8.3 服务端逻辑步骤 4："行不存在" → service 层用
	// errors.Is 识别后翻译为 5001 道具不存在）。**仅**对应"实例 id 在
	// user_cosmetic_items 完全无 row"（fix-review 26-1 r1 [P2] 锁定：实例存在
	// 但属他人恒为 5002，**不**走本哨兵 —— service 拿到行后比对 user_id 自行
	// 分流 5002，本哨兵只覆盖"完全无 row"）。Story 26.3 引入。
	ErrUserCosmeticItemNotFound = errors.New("mysql: user_cosmetic_item not found")

	// ErrUserPetEquipNotFound: UserPetEquipRepo.FindByPetSlot 按 (pet_id, slot)
	// 查不到行（V1 §8.3 服务端逻辑步骤 8："该 slot 无装备" → 跳过同槽换装）。
	// **合法 case，非异常**：service 层用 errors.Is 区分"slot 无装备 → 跳过
	// 同槽换装卸旧分支"vs "DB 异常 → 1009"（与 ErrStepSyncLogNotFound /
	// ErrIdempotencyRecordNotFound 合法 NotFound 哨兵同语义 —— **不**包成
	// 1009 / 业务码）。Story 26.3 引入；Story 26.4 (unequip) 复用（unequip
	// 时该 slot 无装备视为幂等无操作 / 业务码，由 26.4 决定）。
	ErrUserPetEquipNotFound = errors.New("mysql: user_pet_equip not found")

	// ErrUserPetEquipPetSlotDuplicate: UserPetEquipRepo.InsertInTx 时 UNIQUE
	// KEY uk_pet_slot (pet_id, slot) 冲突（0016 schema）。语义："同 pet 同 slot
	// 已有装备"——并发场景（同 pet 同 slot 并发 equip 不同实例，先入者 Tx 已
	// commit user_pet_equips 行 → 后到者 INSERT 触发 MySQL ER_DUP_ENTRY 1062）。
	// service 层用 errors.Is 识别后翻译为 1009 服务繁忙（V1 §8.3 关键约束
	// 行 1560 + NFR11：一件实例同一时间只能装备一次；equip 无 idempotency，
	// 并发兜底走 1009 而非业务码）。与 ErrRoomMembersUserIDDuplicate /
	// ErrRoomMembersRoomUserDuplicate 双哨兵注释风格一致：不同唯一约束需独立
	// 哨兵让 service 层日志能区分哪个约束被打破，便于审计 / debug。
	// 由 Story 26.3 (POST /cosmetics/equip) 引入；Story 26.4 (unequip) 不触发
	// （unequip 是 DELETE）。
	ErrUserPetEquipPetSlotDuplicate = errors.New("mysql: user_pet_equips (pet_id, slot) duplicate (uk_pet_slot conflict)")

	// ErrUserPetEquipItemDuplicate: UserPetEquipRepo.InsertInTx 时 UNIQUE KEY
	// uk_user_cosmetic_item_id (user_cosmetic_item_id) 冲突（0016 schema）。
	// 语义："同一实例已被装备到某 pet"——并发场景（同一实例并发 equip，先入者
	// 已写 user_pet_equips 行 → 后到者 INSERT 触发 1062）。service 层用
	// errors.Is 识别后同样翻译为 1009（V1 §8.3 关键约束行 1560 + NFR11）。
	// 与 uk_pet_slot 分两个独立哨兵让 service 层日志区分冲突源（与
	// ErrRoomMembersUserIDDuplicate / ErrRoomMembersRoomUserDuplicate 同模式）。
	// Story 26.3 引入。
	ErrUserPetEquipItemDuplicate = errors.New("mysql: user_pet_equips user_cosmetic_item_id duplicate (uk_user_cosmetic_item_id conflict)")
)
