// Package service 内 pet_service.go 节点 5 / Epic 14 引入。
//
// **范围红线**：本文件仅承载 /api/v1/pets/* 路由的 service 层。当前实装：
//   - `SyncCurrentState`（POST /api/v1/pets/current/state-sync, Story 14.2 + 14.4）：
//     UPDATE pets.current_state + UPDATE 成功后 fire-and-forget 触发
//     pet.state.changed WS 广播（Story 14.4 起激活；广播范围**含**发起者自己）。
//
// **future 演进**：
//   - Future Story 14.6 / Epic 26 可能加 GetCurrent / GetWardrobe 等接口；同 interface
//     扩签名即可，不另起 PetServiceV2。
//
// **不**承载：
//   - 装扮 / 仓库 / 合成 业务（Epic 23+ 才落地，独立 service / handler）
//   - GetCurrent / GET /pets/current 等查询接口（节点 5 阶段未规划）
package service

import (
	"context"
	stderrors "errors"
	"log/slog"
	"strconv"
	"time"

	ws "github.com/huing/cat/server/internal/app/ws"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// petBroadcastTimeout 是 pet.state.changed broadcast goroutine 的超时上限
// （Story 14.4 引入；与 room_service.go postCommitTimeout 同模式）。
//
// **为何需要超时**：detached ctx (context.WithoutCancel) 解除 request ctx
// cancel 信号 —— 这是为了让 broadcast 不被 client 主动断开 / handler deadline
// 触发的 cancel 误中断（否则 broadcastFn 在 SessionManager 内部依赖 ctx 的路径
// 会 fail "context canceled" → broadcast 静默 skip）。但完全 detached 会引入
// goroutine 泄漏风险（DB 卡死 / SessionManager 死锁 → goroutine 永不返回）。
// 所以**必须**给 detached ctx 加独立 timeout 兜底。
//
// **10s 选型**（与 room_service.go postCommitTimeout 一致）：pet 广播全部 work
// （1 次 marshal + broadcastFn fanout）总时间上界 ~3s；取 10s 留冗余允许
// worst-case write loop 排队。Future 节点如有 SessionManager 性能压测可调小到
// 5s（与 room broadcast 同步调整）。
const petBroadcastTimeout = 10 * time.Second

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

// PetService 是 /api/v1/pets/* 路由的 service 层接口（Story 14.2 引入，Story 14.4
// 起激活广播路径）。
//
// 节点 5 / Epic 14 范围：
//   - Story 14.2 + 14.4：SyncCurrentState —— UPDATE pets.current_state + pet-less noop
//     + UPDATE 成功后 fire-and-forget pet.state.changed WS 广播
//
// **不**在本 story 落地：
//   - GetCurrent（GET /pets/current；节点 5 阶段未规划，可能由 Story 14.6 / Epic 26 引入）
//
// **WS 广播实装**：自 Story 14.4 起在 SyncCurrentState UPDATE 成功路径触发
// pet.state.changed WS 广播给同房间全员（**包含**发起者自己；与 member.joined /
// member.left 排除发起者**不同**语义）；fire-and-forget 严格语义（广播失败不影响
// HTTP 200 响应、不回滚 DB UPDATE）。
type PetService interface {
	// SyncCurrentState 处理 POST /api/v1/pets/current/state-sync 业务（Story 14.2 +
	// Story 14.4）。
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
	//  3. **Story 14.4 broadcast trigger**：检查 users.current_room_id：
	//     - userRepo.FindByID(ctx, userID) err → log warn + 不广播（fire-and-forget；
	//       不影响 HTTP 200）
	//     - user.CurrentRoomID == nil → 不广播（用户不在房间是合法路径，不 log warn）
	//     - user.CurrentRoomID != nil → 启 goroutine（detached ctx + timeout 兜底）
	//       调 broadcastPetStateChanged 触发 pet.state.changed WS 广播给该房间内
	//       所有在线 Session（**包含**发起者自己；与 member.joined / member.left
	//       排除发起者**不同**语义，V1 §12.3 line 2249 钦定）
	//  4. 返 SyncCurrentStateOutput{State: in.State}（回显入参，**不**读 DB pet.current_state
	//     反推 —— ack-only 信号，r2 + r10 lessons 锁定 ack 不入权威等价桶）
	//
	// **fire-and-forget 严格语义**（V1 §5.2 line 539 + §12.3 line 2217 / line 2254
	// 钦定）：广播失败 / FindByID 失败 / marshal 失败一律 log warn，**不**返 error /
	// **不**回滚 pets.current_state UPDATE / **不**影响 HTTP 200 响应。
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
//   - userRepo: users 表访问；Story 14.4 起在 SyncCurrentState UPDATE 成功后调
//     FindByID 查 users.current_room_id 决定是否广播 pet.state.changed
//   - sessionMgr: WS Session 注册中心（10.3 落地）；自 Story 14.4 起 router.go wire
//     真实实例 —— 实装层目前**未直接调用** sessionMgr（防御性预留 / future expansion；
//     与 11.8 roomServiceImpl.sessionMgr 同模式，pet 广播路径不需要操作 Session lifecycle）
//   - broadcastFn: WS 广播函数值（10.5 落地的 BroadcastFn type alias）；自 Story 14.4
//     起在 broadcastPetStateChanged 私有方法内调用 —— 走全 fanout（**包含**发起者
//     自己，与 11.8 broadcastExceptFn 排除发起者**不同**语义）
type petServiceImpl struct {
	petRepo  mysql.PetRepo
	userRepo mysql.UserRepo

	// 自 Story 14.4 起 sessionMgr / broadcastFn 全部进入业务路径
	// （sessionMgr 当前仅防御性预留 —— pet 广播路径不需要操作 Session lifecycle，
	// 但保持字段一致让 NewPetService 签名稳定 + 11.8 roomServiceImpl 同形态）
	sessionMgr  ws.SessionManager
	broadcastFn ws.BroadcastFn
}

// NewPetService 构造 PetService（Story 14.2 引入；4 参数与 11.8 NewRoomService
// 同模式 —— repo 在前，sessionMgr / broadcastFn 在后）。
//
// **sessionMgr / broadcastFn 字段**：自 Story 14.4 起 router.go wire 时传
// deps.SessionMgr + petBroadcastFn closure（与 11.8 NewRoomService 同模式）；测试
// 场景可注入 mock / stub 验证 broadcast 路径，或传 nil 让广播路径 no-op。
//
// **nil-tolerant 注入**：本函数体内**不**做 nil-check fail-fast（与 11.8
// NewRoomService 不做 nil-check 同模式）——
//   - broadcastFn == nil：SyncCurrentState 广播路径走"用户在房间 → goroutine 内调
//     s.broadcastFn(...) → 函数值 nil 直接 panic"。production 不应发生（router.go
//     总是 wire petBroadcastFn closure）；单测 case 1-5 传 nil 时配合 stubUserRepo
//     缺省 CurrentRoomID=nil 跳广播路径，不进 nil 调用
//   - sessionMgr == nil：实装层不直接调用 sessionMgr 字段，与 nil 调用无关
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
// r6 / r7 lessons 锁定 + Story 14.4 激活广播路径。
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

	// (3) Story 14.4: 触发 pet.state.changed WS 广播
	//
	// 流程（V1 §5.2 服务端逻辑步骤 5 + §12.3 `### 宠物状态变更` 字段表钦定）：
	//   - userRepo.FindByID 拿 user.CurrentRoomID *uint64
	//   - CurrentRoomID == nil → 用户不在房间，**不**广播（HTTP 仍 200 OK）
	//   - CurrentRoomID != nil → BroadcastToRoom 广播给该房间所有在线 Session
	//     （**包含**发起者自己，与 member.joined / member.left 排除发起者**不同**语义）
	//
	// **fire-and-forget 严格语义**（V1 §5.2 line 539 + §12.3 line 2217 / line 2254
	// + 11.8 broadcastMemberJoined 同模式）：
	//   - broadcast 失败 / FindByID 失败 / marshal 失败一律 log warn，**不**返 error
	//   - **不**回滚 pets.current_state UPDATE（DB 真实状态以 server 为准）
	//   - **不**影响 HTTP 200 响应（client 已通过 HTTP 拿到 server-acknowledged ack 信号）
	//
	// **完全 detached（14-4 r1 lesson 钦定）**：所有 broadcast 相关 IO（**含**
	// userRepo.FindByID 查 CurrentRoomID）都必须在 detached goroutine 内、用
	// detached ctx 调用 —— HTTP 响应路径不被任何 broadcast IO 阻塞或影响。
	//
	// 反例（14-4 r1 之前的实装，**禁止**）：先在请求 ctx 上同步调
	// `userRepo.FindByID(ctx, ...)` 再决定是否启 goroutine。两个 contract 破坏：
	//   1. **client 断开后**：UPDATE 已 commit 但 client 在 FindByID 返回前断开 →
	//      请求 ctx cancel，FindByID 返 ctx.Err()，broadcast 被跳过；DB 状态已落地
	//      但其他成员永远看不到（事件丢失）
	//   2. **FindByID 卡死**：user 查询慢（DB 卡 / 网络抖）→ HTTP 响应被广播侧 IO
	//      延迟 —— 违反"broadcast 是 detached async，不阻塞 HTTP 响应"的契约
	// `context.WithoutCancel(...) + WithTimeout(10s)` 只保护最终 fanout，不保护
	// 决定"是否 fanout"的 user 查询；user 查询必须一起搬进 detached path。
	//
	// **detached ctx + goroutine**（与 11.8 enqueueRoomEvent 同模式）：
	//   - `context.WithoutCancel(ctx)` 解除 request ctx cancel 信号 —— 让 broadcast
	//     不被 client 主动断开 / handler deadline 触发的 cancel 误中断
	//   - `context.WithTimeout(detached, petBroadcastTimeout)` 加 10s 超时兜底 ——
	//     防 goroutine 泄漏（DB 卡死 / SessionManager 死锁 → goroutine 永不返回）
	//   - `go func() { ... }()` 启 goroutine 异步执行 —— 不阻塞 HTTP 200 响应
	//
	// **goroutine 内分流**：
	//   - userRepo.FindByID 失败 → log warn 后 return（fire-and-forget；与 11.8 同
	//     模式 —— load joiner user failed → skip broadcast）
	//   - user.CurrentRoomID == nil → return 不广播（V1 §5.2 line 540 + §12.3
	//     line 2218 钦定 null → 不广播路径，**不** log warn —— 合法业务路径而非异常）
	//   - user.CurrentRoomID != nil → 调 broadcastPetStateChanged fanout
	detached := context.WithoutCancel(ctx)
	timedCtx, cancel := context.WithTimeout(detached, petBroadcastTimeout)
	go func() {
		defer cancel()
		// 在 detached ctx 内查 user.CurrentRoomID（**不**用请求 ctx，
		// 防 client 断开 cancel 误中断 / FindByID 卡死阻塞 HTTP 响应路径）
		user, err := s.userRepo.FindByID(timedCtx, in.UserID)
		if err != nil {
			slog.WarnContext(timedCtx, "pet state-sync: load user for broadcast failed; skip broadcast",
				slog.Uint64("userId", in.UserID),
				slog.Any("error", err))
			return // fire-and-forget：不广播也不返 error
		}
		if user.CurrentRoomID == nil {
			return // 用户不在任何房间，不广播（合法路径，**不** log warn）
		}
		// 14-3 r1 nil-deref defense：解引用前 nil-check 已通过
		s.broadcastPetStateChanged(timedCtx, *user.CurrentRoomID, in.UserID, pet.ID, in.State)
	}()

	// (4) 返回 ack 信号（回显入参）
	return &SyncCurrentStateOutput{State: in.State}, nil
}

// broadcastPetStateChanged 触发 pet.state.changed WS 广播（Story 14.4 引入）。
//
// 流程（V1 §12.3 `### 宠物状态变更` 字段表钦定）：
//  1. 构造 ws.PetStateChangedPayload{UserID: 字符串化, PetID: 字符串化,
//     CurrentState: int(state)}（BIGINT 字符串化遵循 V1 §2.5）
//  2. 调 ws.BuildPetStateChangedEnvelope(payload) 拿 marshal 后 []byte：
//     - err != nil → log warn + return（fire-and-forget）
//  3. s.broadcastFn(ctx, roomID, msgBytes) 推送（**包含**发起者自己 ——
//     与 11.8 broadcastMemberJoined 用 broadcastExceptFn 排除发起者**不同**语义，
//     详见 V1 §12.3 line 2249 关键约束段；service 层调 broadcastFn 而非
//     broadcastExceptFn 是契约钦定的设计选择）
//     - err != nil → log warn（fire-and-forget，**不**返 error）
//
// **fire-and-forget 严格语义**（V1 §5.2 line 539 + §12.3 line 2217 / line 2254
// 钦定 + 与 11.8 broadcastMemberJoined 同模式）：本方法**永远不返 error** ——
// 任何步骤失败一律 log warn 不返；caller (SyncCurrentState 内的 goroutine) 不需要
// 走错误分流。原因：broadcast 失败不应影响 HTTP 200 响应（client 已通过 HTTP 拿到
// server-acknowledged ack 信号，broadcast 是事件通知，不参与 ack 原子性）。
//
// **不**回滚 DB UPDATE（V1 §5.2 line 539 钦定）：本方法在 UPDATE 成功之后调用，
// 任何 broadcast 失败**不**回滚 pets.current_state 写入 —— DB 真实状态以 server
// 为准（与 CLAUDE.md §"工作纪律 / 状态以 server 为准"一致），broadcast 是事件
// 通知层职责。
//
// **广播范围包含发起者自己**（V1 §12.3 line 2249 关键约束 + §1 line 55 节点 5
// 冻结声明）：service 层调 broadcastFn 全 fanout，client 端自己识别 payload.userId
// == self 走 §5.2 self-broadcast 对称兜底规则（V1 line 547-551，归属 iOS Story 15.x
// 实装）；server 层**不**为 self vs 他人做差异化 fanout。
//
// **detached ctx + timeout**：caller SyncCurrentState 用 `context.WithoutCancel(ctx)` +
// `context.WithTimeout(detached, petBroadcastTimeout)` 构造 detached ctx；本方法接受
// 该 ctx 透传到 broadcastFn 即可。
func (s *petServiceImpl) broadcastPetStateChanged(
	ctx context.Context,
	roomID, userID, petID uint64,
	state int8,
) {
	logger := slog.Default().With(
		slog.String("component", "pet-service-broadcast"),
		slog.String("event", "pet.state.changed"),
		slog.Uint64("roomId", roomID),
		slog.Uint64("userId", userID),
		slog.Uint64("petId", petID),
		slog.Int("state", int(state)),
	)

	// (1) 构造 payload
	payload := ws.PetStateChangedPayload{
		UserID:       strconv.FormatUint(userID, 10),
		PetID:        strconv.FormatUint(petID, 10),
		CurrentState: int(state),
	}

	// (2) marshal envelope
	msgBytes, err := ws.BuildPetStateChangedEnvelope(payload)
	if err != nil {
		logger.Warn("ws broadcast: marshal envelope failed; skip broadcast",
			slog.Any("error", err))
		return
	}

	// (3) fire-and-forget broadcast；用 broadcastFn 全 fanout（**包含**发起者自己 ——
	// 与 11.8 broadcastMemberJoined 用 broadcastExceptFn 排除发起者**不同**语义，
	// V1 §12.3 line 2249 钦定的设计选择）。
	sent, err := s.broadcastFn(ctx, roomID, msgBytes)
	if err != nil {
		logger.Warn("ws broadcast: broadcastFn failed",
			slog.Int("targetSessions", sent),
			slog.Any("error", err))
		return
	}
	logger.Info("ws broadcast: pet.state.changed sent",
		slog.Int("targetSessions", sent))
}
