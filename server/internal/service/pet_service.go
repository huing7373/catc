// Package service 内 pet_service.go 节点 5 / Epic 14 引入。
//
// **范围红线**：本文件仅承载 /api/v1/pets/* 路由的 service 层。当前只实装
// `SyncCurrentState`（POST /api/v1/pets/current/state-sync, Story 14.2）。
//
// **future 演进**：
//   - Story 14.4: 在 SyncCurrentState 成功路径加挂 `pet.state.changed` WS 广播；
//     本 story 已**预留** service struct 字段 `sessionMgr` / `broadcastFn`（nil-tolerant，
//     14.4 才 wire 真实实例）+ TODO 占位注释，**不**真正调用 ws 包导出函数。
//   - Future Story 14.6 / Epic 26 可能加 GetCurrent / GetWardrobe 等接口；同 interface
//     扩签名即可，不另起 PetServiceV2。
//
// **不**承载：
//   - WS 广播实装（14.4 才落地）
//   - 装扮 / 仓库 / 合成 业务（Epic 23+ 才落地，独立 service / handler）
//   - GetCurrent / GET /pets/current 等查询接口（节点 5 阶段未规划）
package service

import (
	"context"
	stderrors "errors"
	"log/slog"

	ws "github.com/huing/cat/server/internal/app/ws"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// SyncCurrentStateInput 是 PetService.SyncCurrentState 的输入 DTO（service 层 DTO，
// **不**是 wire DTO；handler 转换）。
//
// State 字段范围 [1,3] 由 handler 层校验 + 1002 拦截；service 层入参假设已校验。
// State 字段方向：client → server 写入信号（请求体入参，r10 lessons 锁定）。
type SyncCurrentStateInput struct {
	UserID uint64
	State  int8 // 1 = rest, 2 = walk, 3 = run
}

// SyncCurrentStateOutput 是 PetService.SyncCurrentState 的输出 DTO；handler 翻译成
// V1 §5.2 wire DTO `data: {state: int}`。
//
// State 字段是回显入参（**server → client** 方向，**ack-only** 信号；V1 §5.2 响应体
// + r2 / r10 lessons 锁定 ack 不入权威等价桶）；service 层**不**重新查 DB
// pet.current_state 反推（哪怕 happy path 值相等也不读 DB —— 减少一次查询 + 让
// ack-only 语义清晰）。
type SyncCurrentStateOutput struct {
	State int8
}

// PetService 是 /api/v1/pets/* 路由的 service 层接口（Story 14.2 引入）。
//
// 节点 5 / Epic 14 范围：
//   - Story 14.2（本 story）：SyncCurrentState —— UPDATE pets.current_state + pet-less noop
//   - Story 14.4（future）：在 SyncCurrentState 成功路径加挂 pet.state.changed WS 广播
//
// **不**在本 story 落地：
//   - GetCurrent（GET /pets/current；节点 5 阶段未规划，可能由 Story 14.6 / Epic 26 引入）
//   - WS 广播实装（14.4 才落地，service struct 已预留 sessionMgr / broadcastFn 字段）
type PetService interface {
	// SyncCurrentState 处理 POST /api/v1/pets/current/state-sync 业务（Story 14.2）。
	//
	// 流程（V1 §5.2 服务端逻辑 + 数据库设计 §5.3 / §6.4 + r6 / r7 lessons 锁定）：
	//  1. petRepo.FindDefaultByUserID(ctx, userID) 查默认 pet：
	//     - ErrPetNotFound（pet-less，V1 §5.2 line 530-531 + r7 钦定**合法 edge case**） →
	//       **server-acknowledged noop 路径**：跳 UPDATE + 跳广播 + 直接返
	//       SyncCurrentStateOutput{State: in.State}（回显入参），nil error
	//     - 其他 DB 异常 → apperror.Wrap(err, ErrServiceBusy, ...)（1009）
	//     - happy → 进步骤 2
	//  2. petRepo.UpdateCurrentStateByID(ctx, pet.ID, in.State)：
	//     - **err == nil** → 进步骤 3（**禁止**读 RowsAffected；r6 / r9 实装锁定）
	//     - **err != nil** → apperror.Wrap(err, ErrServiceBusy, ...)（1009）
	//  3. **broadcast trigger TODO（Story 14.4）**：检查 users.current_room_id 非 null →
	//     调 s.broadcastPetStateChanged(ctx, userID, pet.ID, in.State) （fire-and-forget）；
	//     本 story **不**实装 —— service struct 字段 sessionMgr / broadcastFn 先 nil 注入，
	//     该位置仅留 // TODO(Story 14.4) 注释 + 单测覆盖"广播路径未被调用"。**禁止**真的
	//     调用任何 ws 包导出函数 / BroadcastFn。
	//  4. 返 SyncCurrentStateOutput{State: in.State}（回显入参，**不**读 DB pet.current_state
	//     反推 —— ack-only 信号，r2 + r10 lessons 锁定 ack 不入权威等价桶）
	//
	// **service 不广播任何 WS 消息**（r3 lessons）：本 service 层执行 UPDATE
	// pets.current_state 后，房间内其他成员通过 14.4 落地的 `pet.state.changed`
	// WS 广播感知；本 service 层**不**广播 `member.joined` / `room.snapshot` /
	// 其他 WS 消息。
	//
	// **不**走事务：仅 1 个 SELECT + 1 个 UPDATE 单语句（DB 引擎默认 autocommit）；
	// 与 11.3 CreateRoom 的 4 步事务不同（参见 V1 §5.2 服务端逻辑"事务边界规则"段）。
	//
	// 错误约定（ADR-0006 三层映射）：
	//   - pet-less（ErrPetNotFound）→ **不**包成 error，走 noop 返 (output, nil)（r7 锁定）
	//   - 其他 DB 异常 → apperror.Wrap(err, ErrServiceBusy, ...)（1009）
	//   - **不**触发 ErrResourceNotFound (1003)（r7 锁定；1003 仍在 §3 全局表保留但本接口不触发）
	SyncCurrentState(ctx context.Context, in SyncCurrentStateInput) (*SyncCurrentStateOutput, error)
}

// petServiceImpl 是 PetService 的默认实装。
//
// 依赖（DI 注入；bootstrap.NewRouter 内 wire）：
//   - petRepo: pets 表访问；调 FindDefaultByUserID + UpdateCurrentStateByID
//   - userRepo: 为 Story 14.4 预留（14.4 落地时 `broadcastPetStateChanged` 内部需要查
//     `users.current_room_id`）；本 story service 层**不**调用 userRepo —— 但**禁止**省略
//     字段定义，因为下游 14.4 实装需要它，本 story 提前 wire 进 service struct 让 14.4
//     落地仅需补方法实装，不动 service 构造函数 / router.go wire
//   - sessionMgr: 为 Story 14.4 预留（WS Session 注册中心，14.4 落地时
//     `broadcastPetStateChanged` 内部可能需要 SessionManager 查同 roomID 在线 session）；
//     本 story 不使用，nil-tolerant 注入；与 11.8 roomServiceImpl.sessionMgr 同模式
//   - broadcastFn: 为 Story 14.4 预留（10.5 落地的 BroadcastFn type alias）；本 story
//     不使用，nil-tolerant 注入；14.4 在 router.go wire 真实实例
type petServiceImpl struct {
	petRepo  mysql.PetRepo
	userRepo mysql.UserRepo

	// 14.4 预留字段（本 story 不使用，**禁止**在 method body 调用）：
	sessionMgr  ws.SessionManager
	broadcastFn ws.BroadcastFn
}

// NewPetService 构造 PetService（Story 14.2 引入；4 参数与 11.8 NewRoomService
// 同模式 —— repo 在前，sessionMgr / broadcastFn 在后）。
//
// **sessionMgr / broadcastFn 字段是 14.4 预留**：本 story 调用方（router.go wire +
// 测试场景）传 nil 即可；NewPetService 函数体内**不**做 nil-check fail-fast（与 11.8
// NewRoomService 不做 nil-check 同模式 —— 字段为 nil 时调用方就不会调到广播路径，
// 不存在 nil pointer panic 风险）。
func NewPetService(
	petRepo mysql.PetRepo,
	userRepo mysql.UserRepo,
	sessionMgr ws.SessionManager,
	broadcastFn ws.BroadcastFn,
) PetService {
	return &petServiceImpl{
		petRepo:     petRepo,
		userRepo:    userRepo,
		sessionMgr:  sessionMgr,
		broadcastFn: broadcastFn,
	}
}

// SyncCurrentState 实装严格按 PetService doc comment 流程 + V1 §5.2 服务端逻辑 +
// r6 / r7 lessons 锁定。
func (s *petServiceImpl) SyncCurrentState(ctx context.Context, in SyncCurrentStateInput) (*SyncCurrentStateOutput, error) {
	// (1) 查默认 pet（与 home_service.LoadHome 同模式）
	pet, err := s.petRepo.FindDefaultByUserID(ctx, in.UserID)
	if err != nil {
		if stderrors.Is(err, mysql.ErrPetNotFound) {
			// pet-less 路径（r7 锁定）：跳 UPDATE + 跳广播 + 返 server-acknowledged noop
			// **不** log error / warn（pet-less 是 contract-valid 状态，不是 invariant 损坏）；
			// log info 级 "pet-less state-sync noop" 作可观测性（与 V1 §5.2 line 531 一致）。
			slog.InfoContext(ctx, "pet-less state-sync noop",
				slog.Uint64("userId", in.UserID),
				slog.Int("state", int(in.State)))
			return &SyncCurrentStateOutput{State: in.State}, nil
		}
		// 其他 DB 异常 → 1009（与 home_service / room_service 同模式）
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// (2) UPDATE pets.current_state
	// **err 二分锁定**（V1 §5.2 line 532-537 + r1 / r6 / r9 lessons）：
	//   - err == nil → 成功（**不**读 RowsAffected）
	//   - err != nil → 1009
	if err := s.petRepo.UpdateCurrentStateByID(ctx, pet.ID, in.State); err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// happy 路径 info log（业务事件可观测性，与 step_service / room_service 同模式）
	slog.InfoContext(ctx, "pet state-sync succeeded",
		slog.Uint64("userId", in.UserID),
		slog.Uint64("petId", pet.ID),
		slog.Int("state", int(in.State)))

	// (3) TODO(Story 14.4): broadcast pet.state.changed if users.current_room_id != NULL
	//
	// 实装方向（14.4 接管，本 story **不**写代码）：
	//   - user, err := s.userRepo.FindByID(ctx, in.UserID)  // 拿 user.CurrentRoomID *uint64
	//   - if err == nil && user.CurrentRoomID != nil {
	//       go s.broadcastPetStateChanged(detachedCtx, *user.CurrentRoomID, in.UserID, pet.ID, in.State)
	//     }
	//   - 注意 fire-and-forget 严格语义（与 11.8 broadcastMemberJoined 同模式）：
	//     广播失败仅 log warn，**不**回滚 UPDATE，**不**影响 HTTP 200 响应
	//   - envelope schema（14.1 锚定 V1 §12.3 `### 宠物状态变更`）：
	//     type = "pet.state.changed", requestId = "", payload = {userId, petId, currentState}, ts = server now ms
	//   - 广播范围：包含发起者自己（V1 §12.3 关键约束 + §1 节点 5 冻结声明；与
	//     member.joined / member.left 排除发起者**不同**语义）
	//   - 14.4 wire 后 NewPetService 第 3/4 参数（sessionMgr / broadcastFn）传真实实例，
	//     本 story 传 nil
	//
	// 本 story service 不调用 ws 包任何导出函数；不读 users.current_room_id；
	// 单测覆盖"广播路径未被触发"（sessionMgr / broadcastFn 字段 nil 时也不 panic）。
	_ = s.sessionMgr  // **不**实际调用；预留字段防 future 编译期遗漏检测
	_ = s.broadcastFn // 同上
	_ = s.userRepo    // 同上 —— 14.4 才用 userRepo 查 current_room_id

	// (4) 返回 ack 信号（回显入参）
	return &SyncCurrentStateOutput{State: in.State}, nil
}
