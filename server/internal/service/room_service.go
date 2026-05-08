package service

import (
	"context"
	stderrors "errors"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
)

// 房间业务常量（数据库设计 §5.13 / §6.12 + V1 §10.1 钦定）。在 service 包定义的理由：
// service 是业务规则归属层（设计文档 §5.2）；handler 不应硬编码业务值（handler 只做
// wire），repo 不应承载业务规则（repo 只做 CRUD）。
//
// Story 11.3 引入；Story 11.4 ~ 11.6 演进时新增的业务常量同包平级（**不**新建
// room_service_constants.go）。
const (
	// roomStatusActive: rooms.status 1=active（数据库设计 §6.12 钦定枚举）。
	// Story 11.5 leave 时如房间空 → status=2 closed（不在本 story 落地）。
	roomStatusActive = 1
	// roomMaxMembersDef: rooms.max_members 节点 4 阶段固定 4（数据库设计 §5.13 +
	// V1 §10.1 钦定；TINYINT UNSIGNED）。Future 节点 8 / 9 / 10 阶段如需动态扩容，
	// 改为从 config 读取再下推 service。
	roomMaxMembersDef = 4
)

// 事务内部哨兵 error（Story 11.4 引入）：让 fn 内部把"步骤 2b status != 1" 与
// "步骤 2c count >= 4" 两个**纯业务规则失败**与"DB 异常"分流，让外层 errors.Is
// 翻译为对应业务码。
//
// 不直接在 fn 内调 apperror.New 然后返 *AppError —— 那样会让外层 errors.Is(err,
// mysql.ErrRoomNotFound) 等判定失效（apperror 不实现 Is 哨兵 protocol）。本模式
// 与 auth_service.firstTimeLogin 的 errAuthBindingDuplicateInternal 同思路。
//
// **未导出**（小写开头）：仅 service 包内部使用，不让其他包 errors.Is —— 业务
// 码翻译统一在 JoinRoom 内完成。
var (
	errRoomInvalidStateInternal = stderrors.New("room_service: room invalid state (status != active)")
	errRoomFullInternal         = stderrors.New("room_service: room full (count >= max_members)")
)

// CreateRoomInput 是 RoomService.CreateRoom 的输入 DTO（service 层 DTO，**不**是
// wire DTO）。本接口不消费 client 业务字段（V1 §10.1 钦定请求体为空对象 {}），
// 仅依赖 caller 身份；caller 身份由 handler 从 auth middleware 注入的 c.Keys 取
// userID 后填入本 DTO（与 step_service 同模式）。
type CreateRoomInput struct {
	UserID uint64 // 当前 caller user.id（非 zero；handler 必须保证非 0 由 auth middleware 兜底）
}

// CreateRoomOutput 是 RoomService.CreateRoom 的输出 DTO；handler 翻译成 V1 §10.1
// 钦定的 wire DTO（BIGINT id 转 string / status / maxMembers / memberCount 保 number）。
//
// **memberCount 的来源**：本接口创建房间后必为 1（创建者自动加入）；不需要查 DB
// count(*) —— 业务规则保证此处必为 1，service 层硬编码即可（与 V1 §10.1 钦定一致）。
type CreateRoomOutput struct {
	RoomID        uint64 // rooms.id
	CreatorUserID uint64 // rooms.creator_user_id（必为 input.UserID；冗余字段方便 handler 不需要回查）
	MaxMembers    uint8  // 节点 4 阶段固定 4（来自 schema default + service 常量）
	MemberCount   int    // 必为 1（业务规则）
	Status        int8   // 必为 1 (active)（业务规则）
}

// JoinRoomInput 是 RoomService.JoinRoom 的输入 DTO（Story 11.4 引入）。
// caller 身份由 handler 从 auth middleware 注入的 c.Keys 取 userID 后填入；
// RoomID 由 handler 从 path 参数 ":roomId" 解析（V1 §10.4 钦定 BIGINT 字符串化 +
// 1 ≤ length ≤ 20 字符；handler 层失败时返 1002，不会调到 service）。
type JoinRoomInput struct {
	UserID uint64 // 当前 caller user.id
	RoomID uint64 // 目标 room.id（来自 path ":roomId"）
}

// JoinRoomOutput 是 RoomService.JoinRoom 的输出 DTO（Story 11.4 引入）；handler 翻译成
// V1 §10.4 钦定的 wire DTO（roomId BIGINT 字符串化 + joined 固定 true）。
type JoinRoomOutput struct {
	RoomID uint64 // 同 input.RoomID（V1 §10.4 钦定回带方便 client 校验）
	Joined bool   // 必为 true（V1 §10.4 钦定固定值，失败路径返业务码而非 joined: false）
}

// RoomService 是 /api/v1/rooms 系列 handler 的依赖 interface（便于 handler 单测 mock）。
//
// Epic 11 演进路径：
//   - Story 11.3 (本 story): CreateRoom（POST /rooms）
//   - Story 11.4: JoinRoom（POST /rooms/{roomId}/join）
//   - Story 11.5: LeaveRoom（POST /rooms/{roomId}/leave）
//   - Story 11.6: GetCurrentRoom（GET /rooms/current）+ GetRoomDetail（GET /rooms/{roomId}）
//
// 错误约定：handler 透传，由 ErrorMappingMiddleware 写 envelope（ADR-0006）。
// service 层永远只产出 *AppError，不返 raw error。
type RoomService interface {
	// CreateRoom: 创建房间 + 创建者自动加入（事务）。
	//
	// 流程（详见 V1 §10.1 服务端逻辑 + 数据库设计 §7.1 钦定）：
	//   1. 预检 user.current_room_id：非 null → 立即返回 6003，不开事务
	//   2. 开事务（txMgr.WithTx）：
	//      a. 插入 rooms (creator_user_id, status=1, max_members=4)
	//      b. 插入 room_members (room_id, user_id)；撞 UNIQUE(user_id) → 回滚 + 6003 兜底
	//      c. 更新 users.current_room_id = room_id
	//   3. commit + 返回 CreateRoomOutput（memberCount=1, status=1）
	//
	// 错误：
	//   - 6003 双路径：步骤 1 预检（用户已在房间中）+ 步骤 2b 兜底（并发 race）
	//   - 1009：DB 异常 / 内部 panic（事务自动回滚）
	CreateRoom(ctx context.Context, in CreateRoomInput) (*CreateRoomOutput, error)

	// JoinRoom: 加入已有房间（事务；Story 11.4 引入）。
	//
	// 流程（详见 V1 §10.4 服务端逻辑 + 数据库设计 §8.6 钦定）：
	//   1. 预检 user.current_room_id：非 null → 立即返回 6003（含"已在目标房间"
	//      与"已在其他房间"两个子场景，client 不区分）
	//   2. 开事务（txMgr.WithTx）：
	//      a. SELECT rooms WHERE id = ? FOR UPDATE → 找不到 → 6001
	//      b. status != 1 → 6005
	//      c. SELECT COUNT(*) FROM room_members WHERE room_id = ? → >= 4 → 6002
	//      d. INSERT room_members；撞 UNIQUE(user_id) → 回滚 + 6003 兜底
	//      e. UPDATE users.current_room_id = roomID
	//   3. commit + 返回 JoinRoomOutput{RoomID, Joined: true}
	//
	// 错误码触发顺序（V1 §10.4 钦定，**不允许实装层重排**）：
	//   - 步骤 1 → 6003（预检）
	//   - 步骤 2a → 6001（房间不存在）
	//   - 步骤 2b → 6005（房间状态异常 / closed）
	//   - 步骤 2c → 6002（房间已满）
	//   - 步骤 2d → 6003（DB UNIQUE 兜底；与预检 6003 message / errorCode 完全等价）
	//   - 任何 DB 异常 → 1009
	//
	// **broadcast member.joined 由 Story 11.8 实装**：本 story 事务 commit 成功后
	// 直接返回；11.8 在 commit 与 return 之间插入 BroadcastToRoom 调用。
	JoinRoom(ctx context.Context, in JoinRoomInput) (*JoinRoomOutput, error)
}

// roomServiceImpl 是 RoomService 的默认实装。
//
// 依赖（DI 注入；bootstrap.NewRouter 内 wire）：
//   - txMgr: 事务管理器（4.2 落地）
//   - userRepo: users 表访问；本 story 调 FindByID（预检）+ UpdateCurrentRoomID（事务内）
//   - roomRepo: rooms 表访问；本 story 调 Create
//   - roomMemberRepo: room_members 表访问；本 story 调 Create（双 UNIQUE 哨兵兜底 6003）
type roomServiceImpl struct {
	txMgr          tx.Manager
	userRepo       mysql.UserRepo
	roomRepo       mysql.RoomRepo
	roomMemberRepo mysql.RoomMemberRepo
}

// NewRoomService 构造 RoomService。
//
// 全部依赖通过参数显式注入；不引入 wire / fx 框架（与 4.2 / 4.4 / 4.5 / 4.6 / 7.3 同模式）。
func NewRoomService(
	txMgr tx.Manager,
	userRepo mysql.UserRepo,
	roomRepo mysql.RoomRepo,
	roomMemberRepo mysql.RoomMemberRepo,
) RoomService {
	return &roomServiceImpl{
		txMgr:          txMgr,
		userRepo:       userRepo,
		roomRepo:       roomRepo,
		roomMemberRepo: roomMemberRepo,
	}
}

// CreateRoom 实装严格按 V1 §10.1 + 数据库设计 §7.1 钦定的事务边界：
//
//	步骤 1（事务外）：FindByID + 检查 user.CurrentRoomID != nil → 预检 6003
//	步骤 2（事务内 4 步）：
//	  2a. roomRepo.Create（GORM 回填 room.ID）
//	  2b. roomMemberRepo.Create（撞 UNIQUE 兜底 6003）
//	  2c. userRepo.UpdateCurrentRoomID（set to room.ID）
//	步骤 3（事务后）：commit / rollback；commit 成功 → 返回 CreateRoomOutput；
//	  rollback 后按 err 类型分流 6003 / 1009
//
// **6003 双路径必须等价**（V1 §10.1 钦定）：预检路径（步骤 1）+ DB 兜底路径（步骤 2b）
// 都返 apperror.New(ErrUserAlreadyInRoom, ...)，handler 端响应 envelope 完全一致 ——
// client **不**应区分这两种场景。
//
// **ctx 用法**（ADR-0007 §2.4）：fn 内全部 repo 调用必须用 txCtx 而非外层 ctx ——
// 用错 ctx 会绕过 tx 走 db pool 新连接，该调用脱离事务，业务语义错乱。
func (s *roomServiceImpl) CreateRoom(ctx context.Context, in CreateRoomInput) (*CreateRoomOutput, error) {
	// (1) 预检 user.current_room_id（事务外，普通连接池查询）
	user, err := s.userRepo.FindByID(ctx, in.UserID)
	if err != nil {
		// ErrUserNotFound 理论不应发生（auth middleware 已校验 token 对应有效 user）；
		// 任何 err 都包成 1009（与 auth_service.reuseLogin 同模式："理论不应发生但兜底为 1009"）。
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	if user.CurrentRoomID != nil {
		// 用户已在房间中（V1 §10.1 步骤 1 钦定的预检路径）→ 6003，不开事务
		return nil, apperror.New(apperror.ErrUserAlreadyInRoom, apperror.DefaultMessages[apperror.ErrUserAlreadyInRoom])
	}

	// (2) 开事务（数据库设计 §7.1 / V1 §10.1 钦定）
	var roomID uint64
	err = s.txMgr.WithTx(ctx, func(txCtx context.Context) error {
		// (2a) 插入 rooms（status=1, max_members=4；GORM 回填 r.ID）
		r := &mysql.Room{
			CreatorUserID: in.UserID,
			Status:        roomStatusActive,
			MaxMembers:    roomMaxMembersDef,
		}
		if err := s.roomRepo.Create(txCtx, r); err != nil {
			return err
		}
		roomID = r.ID

		// (2b) 插入 room_members；撞 UNIQUE(user_id) / UNIQUE(room_id, user_id) → 6003 兜底
		m := &mysql.RoomMember{
			RoomID: roomID,
			UserID: in.UserID,
		}
		if err := s.roomMemberRepo.Create(txCtx, m); err != nil {
			return err
		}

		// (2c) 更新 users.current_room_id = room_id
		if err := s.userRepo.UpdateCurrentRoomID(txCtx, in.UserID, &roomID); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		// (3) UNIQUE(user_id) / UNIQUE(room_id, user_id) 兜底 → 6003。
		// **errors.Is 顺序关键**（review r10 同源风险）：必须在 apperror.Wrap(...,
		// ErrServiceBusy) **之前**判定，否则 6003 路径会被 generic 1009 兜底覆盖。
		// 与 auth_service.firstTimeLogin 处理 ErrAuthBindingDuplicate /
		// ErrUsersGuestUIDDuplicate 同模式。
		if stderrors.Is(err, mysql.ErrRoomMembersUserIDDuplicate) ||
			stderrors.Is(err, mysql.ErrRoomMembersRoomUserDuplicate) {
			return nil, apperror.New(apperror.ErrUserAlreadyInRoom, apperror.DefaultMessages[apperror.ErrUserAlreadyInRoom])
		}
		// (3') 其他失败 → 1009（事务已 rollback）
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// (4) 事务 commit 成功 → 返回
	return &CreateRoomOutput{
		RoomID:        roomID,
		CreatorUserID: in.UserID,
		MaxMembers:    roomMaxMembersDef,
		MemberCount:   1,
		Status:        roomStatusActive,
	}, nil
}

// JoinRoom 实装严格按 V1 §10.4 + 数据库设计 §8.6 钦定的事务边界（Story 11.4 引入）：
//
//	步骤 1（事务外）：FindByID + 检查 user.CurrentRoomID != nil → 预检 6003
//	步骤 2（事务内 5 步）：
//	  2a. roomRepo.FindByIDForUpdate（SELECT ... FOR UPDATE）→ 找不到 → 6001
//	  2b. status != 1 → 6005
//	  2c. roomMemberRepo.CountByRoomID → >= 4 → 6002
//	  2d. roomMemberRepo.Create（撞 UNIQUE 兜底 6003）
//	  2e. userRepo.UpdateCurrentRoomID（set to room.ID）
//	步骤 3（事务后）：commit / rollback；commit 成功 → 返回 JoinRoomOutput；
//	  rollback 后按 err 类型分流 6001 / 6005 / 6002 / 6003 / 1009
//
// **错误码触发顺序锁定**（V1 §10.4 钦定，**不允许实装层重排**）：步骤 1 → 6003；
// 2a → 6001；2b → 6005；2c → 6002；2d → 6003。重排（如先查容量再查 status）会让
// "closed 房间满员"场景错误返 6002 而非 6005，违反 client UX 期望。
//
// **6003 双路径必须等价**（V1 §10.4 钦定）：预检路径（步骤 1）+ DB 兜底路径
// （步骤 2d）都返 apperror.New(ErrUserAlreadyInRoom, ...)，handler 端响应 envelope
// 完全一致 —— client **不**应区分这两种场景（含"已在目标房间" + "已在其他房间"
// 两个子场景，client 不区分）。
//
// **ctx 用法**（ADR-0007 §2.4）：fn 内全部 5 个 repo 调用必须用 txCtx 而非外层
// ctx —— 用错 ctx 会绕过 tx 走 db pool 新连接，FOR UPDATE 锁立即释放，并发保护
// 失效，r9 cross-tx race 重新出现。
//
// **broadcast member.joined 由 Story 11.8 实装**：本 story 实装层 commit 成功后
// **直接 return**，**不**调任何 WS broadcast；Story 11.8 review 时在 return 前
// 插入 s.broadcastMemberJoined(ctx, in.RoomID, in.UserID) 调用。
func (s *roomServiceImpl) JoinRoom(ctx context.Context, in JoinRoomInput) (*JoinRoomOutput, error) {
	// (1) 预检 user.current_room_id（事务外，普通连接池查询）
	user, err := s.userRepo.FindByID(ctx, in.UserID)
	if err != nil {
		// ErrUserNotFound 理论不应发生（auth middleware 已校验 token 对应有效 user）；
		// 任何 err 都包成 1009。
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	if user.CurrentRoomID != nil {
		// 用户已在房间中（V1 §10.4 步骤 1 钦定预检路径；含"已在目标房间" +
		// "已在其他房间"两个子场景，client 不区分）→ 6003，不开事务
		return nil, apperror.New(apperror.ErrUserAlreadyInRoom, apperror.DefaultMessages[apperror.ErrUserAlreadyInRoom])
	}

	// (2) 开事务（数据库设计 §8.6 + V1 §10.4 钦定）
	err = s.txMgr.WithTx(ctx, func(txCtx context.Context) error {
		// (2a) SELECT rooms WHERE id = ? FOR UPDATE → 找不到 → 6001
		room, err := s.roomRepo.FindByIDForUpdate(txCtx, in.RoomID)
		if err != nil {
			return err // 含 ErrRoomNotFound 哨兵 / DB raw error
		}

		// (2b) status != 1 → 6005
		if room.Status != roomStatusActive {
			return errRoomInvalidStateInternal
		}

		// (2c) SELECT COUNT(*) FROM room_members WHERE room_id = ? → >= 4 → 6002
		count, err := s.roomMemberRepo.CountByRoomID(txCtx, in.RoomID)
		if err != nil {
			return err
		}
		if count >= roomMaxMembersDef {
			return errRoomFullInternal
		}

		// (2d) INSERT room_members；撞 UNIQUE(user_id) / UNIQUE(room_id, user_id) → 6003 兜底
		m := &mysql.RoomMember{
			RoomID: in.RoomID,
			UserID: in.UserID,
		}
		if err := s.roomMemberRepo.Create(txCtx, m); err != nil {
			return err
		}

		// (2e) UPDATE users.current_room_id = roomID
		if err := s.userRepo.UpdateCurrentRoomID(txCtx, in.UserID, &in.RoomID); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		// (3) 业务码分流（**errors.Is 顺序关键** —— review r10 同源风险：errors.Is
		// 顺序写错会让具体业务码被 generic 1009 兜底覆盖）
		if stderrors.Is(err, mysql.ErrRoomNotFound) {
			return nil, apperror.New(apperror.ErrRoomNotFound, apperror.DefaultMessages[apperror.ErrRoomNotFound])
		}
		if stderrors.Is(err, errRoomInvalidStateInternal) {
			return nil, apperror.New(apperror.ErrRoomInvalidState, apperror.DefaultMessages[apperror.ErrRoomInvalidState])
		}
		if stderrors.Is(err, errRoomFullInternal) {
			return nil, apperror.New(apperror.ErrRoomFull, apperror.DefaultMessages[apperror.ErrRoomFull])
		}
		if stderrors.Is(err, mysql.ErrRoomMembersUserIDDuplicate) ||
			stderrors.Is(err, mysql.ErrRoomMembersRoomUserDuplicate) {
			return nil, apperror.New(apperror.ErrUserAlreadyInRoom, apperror.DefaultMessages[apperror.ErrUserAlreadyInRoom])
		}
		// (3') 其他失败 → 1009（事务已 rollback）
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// (4) 事务 commit 成功 → 返回（broadcast 由 Story 11.8 在此处插入调用）
	return &JoinRoomOutput{
		RoomID: in.RoomID,
		Joined: true,
	}, nil
}
