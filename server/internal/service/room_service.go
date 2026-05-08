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
	roomStatusActive = 1
	// roomStatusClosed: rooms.status 2=closed（数据库设计 §6.12 钦定枚举；Story 11.5 引入）。
	// V1 §10.5 钦定"最后一人离开 → status=2 closed"路径写入；rooms.status 严格单调
	// （1 → 2，无回退路径），节点 4 阶段无"重启房间"接口。
	roomStatusClosed = 2
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
	// errLeaverNotInRoomInternal Story 11.5 引入：让 fn 内部把"步骤 2b DELETE
	// RowsAffected==0"与"DB 异常"分流，外层 errors.Is 翻译为 6004 ErrUserNotInRoom。
	// 与 errRoomInvalidStateInternal / errRoomFullInternal 同模式 —— 用未导出哨兵
	// 让 service 包内部翻译，避免外层包 errors.Is 误用。
	errLeaverNotInRoomInternal = stderrors.New("room_service: leaver not in room (DELETE rows_affected == 0)")
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

// LeaveRoomInput 是 RoomService.LeaveRoom 的输入 DTO（Story 11.5 引入）。
// caller 身份由 handler 从 auth middleware 注入的 c.Keys 取 userID 后填入；
// RoomID 由 handler 从 path 参数 ":roomId" 解析（V1 §10.5 钦定 BIGINT 字符串化 +
// 1 ≤ length ≤ 20 字符；handler 层失败时返 1002，不会调到 service）。
type LeaveRoomInput struct {
	UserID uint64 // 当前 caller user.id
	RoomID uint64 // 目标 room.id（来自 path ":roomId"）
}

// LeaveRoomOutput 是 RoomService.LeaveRoom 的输出 DTO（Story 11.5 引入）；handler
// 翻译成 V1 §10.5 钦定的 wire DTO（roomId BIGINT 字符串化 + left 固定 true）。
//
// **响应中不暴露 "房间是否已 closed" 字段**（V1 §10.5 行 1587 钦定）—— client 不直接
// 感知房间状态变化，仅 left=true 表示当前 user 已离开；房间状态变化是 server 内部
// 副作用，**不**对外暴露（避免 client 围绕"我是否是最后一人"做特殊 UX 分支）。
type LeaveRoomOutput struct {
	RoomID uint64 // 同 input.RoomID（V1 §10.5 钦定回带方便 client 校验）
	Left   bool   // 必为 true（V1 §10.5 钦定固定值，失败路径返业务码而非 left: false）
}

// GetCurrentRoomInput 是 RoomService.GetCurrentRoom 的输入 DTO（Story 11.6 引入）。
// 仅含 caller userID（V1 §10.2 钦定无请求体；当前用户身份从 c.Keys 取）。
type GetCurrentRoomInput struct {
	UserID uint64 // 当前 caller user.id
}

// GetCurrentRoomOutput 是 RoomService.GetCurrentRoom 的输出 DTO（Story 11.6 引入）；
// handler 翻译成 V1 §10.2 钦定 wire DTO `{roomId: <string>|null}`。
//
// **关键**：用 *uint64 让 nil → JSON `null`（用户不在任何房间是合法场景）。
// handler 层翻译：roomID == nil → wire `*string` 为 nil → JSON `"roomId": null`；
// roomID != nil → wire `*string` 指向 strconv.FormatUint(*roomID, 10) → JSON
// `"roomId": "3001"`（V1 §2.5 BIGINT 字符串化）。
type GetCurrentRoomOutput struct {
	RoomID *uint64 // nil = 用户不在任何房间；非 nil = 当前所在房间 id
}

// GetRoomDetailInput 是 RoomService.GetRoomDetail 的输入 DTO（Story 11.6 引入）。
// RoomID 由 handler 从 path 参数 ":roomId" 解析（V1 §10.3 钦定 BIGINT 字符串化 +
// 1 ≤ length ≤ 20 字符；handler 层失败时返 1002，不会调到 service）。
type GetRoomDetailInput struct {
	UserID uint64 // 当前 caller user.id（用于 ACL 校验）
	RoomID uint64 // 目标 room.id（来自 path ":roomId"）
}

// GetRoomDetailOutput 是 RoomService.GetRoomDetail 的输出 DTO（Story 11.6 引入）；
// handler 翻译成 V1 §10.3 钦定 wire DTO（嵌套结构 + BIGINT 字符串化 + pet nullable）。
//
// 不变量（V1 §10.3 钦定）：
//   - len(Members) === MemberCount（service 层硬保证）
//   - Members 已按 room_members.joined_at ASC 排序（repo 层 ORDER BY 保证）
//   - Members[].Pet == nil 表示该 user 是 pet-less 账号（LEFT JOIN pets 在
//     该 user 无 is_default=1 的 pet 行时下发 nil）
type GetRoomDetailOutput struct {
	RoomID        uint64
	CreatorUserID uint64
	MaxMembers    uint8
	Status        int8
	MemberCount   int
	Members       []MemberOutput
}

// MemberOutput 是 GetRoomDetailOutput.Members 的元素（Story 11.6 引入）。
type MemberOutput struct {
	UserID    uint64
	Nickname  string
	AvatarURL string
	Pet       *MemberPetOutput // nil = pet-less（LEFT JOIN pets 行 NULL）
}

// MemberPetOutput 是 MemberOutput.Pet 的容器（Story 11.6 引入）；仅当 Pet ≠ nil 时存在。
// CurrentState 节点 4 阶段固定 1 (rest)（V1 §10.3 字段表节点 4 列钦定）；节点 5 由
// Epic 14 真实驱动 motion_state。
// Equips 节点 4 阶段固定 []（V1 §10.3 Future Fields 钦定）；节点 9 由 Epic 26 真实
// 回填非空数组 + 节点 10 由 Epic 29 加 renderConfig 子字段。
type MemberPetOutput struct {
	PetID        uint64
	CurrentState int8          // 固定 1 (rest)
	Equips       []EquipOutput // 节点 4 阶段固定 []
}

// EquipOutput 是 MemberPetOutput.Equips 的元素占位（Story 11.6 引入）；
// 节点 4 阶段空数组占位，**无字段**；节点 9 由 Epic 26 真实回填（cosmeticItemId
// / slot 等字段）+ 节点 10 由 Epic 29 加 renderConfig。本 story 不为 EquipOutput
// 添加字段（YAGNI；future epic 落地时一并填充）。
type EquipOutput struct{}

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

	// LeaveRoom: 退出当前房间（事务；Story 11.5 引入）。
	//
	// 流程（详见 V1 §10.5 服务端逻辑 + 数据库设计 §8.7 钦定）：
	//   1. 预检 user.current_room_id：与 input.RoomID 不一致（含 nil）→ 立即返回 6004，
	//      不开事务
	//   2. 开事务（txMgr.WithTx）：
	//      a. SELECT rooms WHERE id = ? FOR UPDATE → 找不到 → 1009（数据不一致兜底，
	//         不对外暴露 6001 —— V1 §10.5 行 1597 钦定）
	//      b. DELETE room_members WHERE room_id = ? AND user_id = ?；rowsAffected == 0
	//         → 回滚 + 6004（同一 user 并发两次 leave 输家兜底）
	//      c. UPDATE users.current_room_id = NULL（首次启用 nil 入参路径）
	//      d. SELECT COUNT(*) FROM room_members WHERE room_id = ? → 剩余 0 → 步骤 e；
	//         否则跳过步骤 e
	//      e. UPDATE rooms.status = 2 closed（仅最后一人离开路径执行）
	//   3. commit + 返回 LeaveRoomOutput{RoomID, Left: true}
	//
	// 错误码触发顺序（V1 §10.5 钦定）：
	//   - 步骤 1 → 6004（预检：current_room_id != roomId 含 nil）
	//   - 步骤 2a → 1009（数据不一致兜底；**不**对外暴露 6001）
	//   - 步骤 2b → 6004（DELETE RowsAffected == 0 兜底；与预检 6004 message / errorCode 完全等价）
	//   - 任何 DB 异常 → 1009
	//
	// **6004 双路径必须等价**（V1 §10.5 行 1601 钦定）：预检路径（步骤 1）+ DB 兜底
	// 路径（步骤 2b）都返 apperror.New(ErrUserNotInRoom, ...)，handler 端响应 envelope
	// 完全一致 —— client **不**应区分这三个子场景（含"用户不在任何房间" + "用户在
	// 其他房间" + "并发两次 leave 输家"，client 不区分）。
	//
	// **broadcast member.left + close 4007 unregister leaver Session 由 Story 11.8 实装**：
	// 本 story 实装层 commit 成功后**直接 return**，**不**调任何 WS broadcast / 不动
	// SessionManager；Story 11.8 review 时在 return 前按 V1 §10.5 步骤 7-8 顺序（r13
	// 锁定）插入：
	//   1. close 4007 + unregister leaver Session（V1 §10.5 步骤 7）
	//   2. BroadcastToRoom(roomID, member.left)（V1 §10.5 步骤 8）
	//
	// 顺序由 V1 §10.5 r13 钦定 —— BroadcastToRoom primitive 不带 excludeUserID 参数，
	// 先 close + unregister 后 broadcast 让 fanout 列表自然不含 leaver。
	LeaveRoom(ctx context.Context, in LeaveRoomInput) (*LeaveRoomOutput, error)

	// GetCurrentRoom: 查询当前用户所在房间号（Story 11.6 引入）。
	//
	// 流程（V1 §10.2 服务端逻辑钦定）：
	//   1. userRepo.FindByID(ctx, in.UserID)（**事务外**普通连接池查询；不开事务）
	//   2. 取 user.CurrentRoomID 字段直接返回 GetCurrentRoomOutput{RoomID: user.CurrentRoomID}
	//
	// 错误码（V1 §10.2 钦定）：
	//   - 1009：DB 异常 / FindByID 返 ErrUserNotFound（理论不应发生 —— caller 通过
	//     auth middleware 必有 user 行；race 兜底走 1009）/ 内部 panic
	//   - **不**触发 6001 ~ 6005 —— 用户不在房间是合法场景（返回 RoomID=nil + code=0），
	//     不视为业务错误
	//
	// **注**：本接口与 GET /home.data.room.currentRoomId 字段语义等价（Story 11.10 真实
	// 实装），但接口路径独立 —— home 是首页聚合（多字段），本接口是单字段轻量查询。
	// client 在房间页用本接口、在首页加载用 GET /home。
	GetCurrentRoom(ctx context.Context, in GetCurrentRoomInput) (*GetCurrentRoomOutput, error)

	// GetRoomDetail: 查询房间详情含 roster（Story 11.6 引入）。
	//
	// 流程（详见 V1 §10.3 服务端逻辑 + 数据库设计 §8.8 读快照事务（含 ACL 共享锁）钦定）：
	//   1. 开事务（txMgr.WithTx；REPEATABLE READ 隔离级别 = InnoDB 默认）：
	//      a. **步骤 1a**：userRepo.FindByID(txCtx, in.UserID) → user.CurrentRoomID
	//         != &in.RoomID（含 nil）→ 返回 6004
	//      b. **步骤 1b**：roomMemberRepo.ExistsForShareByRoomAndUser(txCtx, RoomID,
	//         UserID) → false (FOR SHARE 0 行兜底) → 返回 6004
	//      c. **步骤 2**：roomRepo.FindByID(txCtx, RoomID) → ErrRoomNotFound → 返回 6001
	//      d. **步骤 3**：roomMemberRepo.ListRosterByRoomID(txCtx, RoomID) → roster
	//   2. commit + 拼装 GetRoomDetailOutput
	//
	// 错误码触发顺序（V1 §10.3 钦定）：
	//   - 步骤 1a → 6004（预检 ACL：current_room_id != roomId 含 nil）
	//   - 步骤 1b → 6004（FOR SHARE 0 行兜底；与预检 6004 message / errorCode 完全等价）
	//   - 步骤 2 → 6001（rooms 兜底；理论步骤 1a 通过即意味 rooms 行存在；race 兜底）
	//   - 任何 DB 异常 → 1009
	//
	// **不**对外暴露 6002 / 6003 / 6005（V1 §10.3 行 1347 钦定纯查询不涉及 join 路径）。
	//
	// **6004 双路径必须等价**（V1 §10.3 行 1258 钦定）：步骤 1a 预检 + 步骤 1b 兜底
	// 都返 apperror.New(ErrUserNotInRoom, ...)，handler 端响应 envelope 完全一致。
	//
	// **ctx 用法**（ADR-0007 §2.4）：fn 内全部 4 个 repo 调用必须用 txCtx 而非外层
	// ctx —— 用错 ctx 会绕过 tx 走 db pool 新连接，FOR SHARE 锁立即释放，并发保护
	// 失效，post-leave 数据泄漏 race 重新出现（V1 §10.3 rationale 钦定）。
	//
	// **不实装**：close 4007 / WS 广播 / member.* —— 本接口纯查询无副作用。
	GetRoomDetail(ctx context.Context, in GetRoomDetailInput) (*GetRoomDetailOutput, error)
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

// LeaveRoom 实装严格按 V1 §10.5 + 数据库设计 §8.7 钦定的事务边界（Story 11.5 引入）：
//
//	步骤 1（事务外）：FindByID + 检查 user.CurrentRoomID == &input.RoomID → 否则预检 6004
//	步骤 2（事务内 5 步）：
//	  2a. roomRepo.FindByIDForUpdate（SELECT ... FOR UPDATE）→ 找不到 → 1009
//	  2b. roomMemberRepo.DeleteByRoomAndUser → rowsAffected == 0 → 回滚 + 6004 兜底
//	  2c. userRepo.UpdateCurrentRoomID(userID, nil)（set NULL）
//	  2d. roomMemberRepo.CountByRoomID → remaining
//	  2e. if remaining == 0 → roomRepo.UpdateStatus(roomID, 2 closed)
//	步骤 3（事务后）：commit / rollback；commit 成功 → 返回 LeaveRoomOutput；
//	  rollback 后按 err 类型分流 6004 / 1009
//
// **错误码触发顺序锁定**（V1 §10.5 钦定）：步骤 1 → 6004（预检）；2a → 1009；
// 2b → 6004（DELETE RowsAffected == 0 兜底）；任何 DB 异常 → 1009。**不**对外
// 暴露 6001 / 6002 / 6003 / 6005（V1 §10.5 行 1599 钦定）。
//
// **6004 双路径必须等价**（V1 §10.5 行 1601 钦定）：预检路径（步骤 1）+ DB 兜底路径
// （步骤 2b）都返 apperror.New(ErrUserNotInRoom, ...)，handler 端响应 envelope
// 完全一致。
//
// **ctx 用法**（ADR-0007 §2.4）：fn 内全部 5 个 repo 调用必须用 txCtx 而非外层
// ctx —— 用错 ctx 会绕过 tx 走 db pool 新连接，FOR UPDATE 锁立即释放，并发保护
// 失效，r9 cross-tx race 重新出现。
//
// **SECURITY-DEFER (Story 11.8 钦定范围)**：broadcast `member.left` + close 4007
// unregister leaver Session 由 **Story 11.8 实装**，本 story 严守范围红线**不**调
// 任何 WS primitive（不动 SessionManager / BroadcastToRoom）。post-commit 钩子的
// 详细 trade-off + 当前 11.5↔11.8 窗口的影响范围（leaver Session 仍 registered →
// SessionManager.ListSessionsByRoomID / BroadcastToRoom / presence 仍把 leaver 当
// active member 直到心跳超时被动清理）见**函数末尾 (4) 段 SECURITY-DEFER 警告块**
// 与 lesson `docs/lessons/2026-05-09-leave-room-ws-session-cleanup-defer-to-11-8-11-5-r1.md`。
// codex review r1 (2026-05-08) 把这条 flag 为 [P1]；review 工程上 correct，但
// 跨 story scope，按 spec 钦定 defer 至 11.8。
func (s *roomServiceImpl) LeaveRoom(ctx context.Context, in LeaveRoomInput) (*LeaveRoomOutput, error) {
	// (1) 预检 user.current_room_id（事务外，普通连接池查询）
	user, err := s.userRepo.FindByID(ctx, in.UserID)
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	// 与 input.RoomID 不一致（含 user.CurrentRoomID == nil）→ 6004 预检路径
	if user.CurrentRoomID == nil || *user.CurrentRoomID != in.RoomID {
		return nil, apperror.New(apperror.ErrUserNotInRoom, apperror.DefaultMessages[apperror.ErrUserNotInRoom])
	}

	// (2) 开事务（数据库设计 §8.7 + V1 §10.5 钦定）
	err = s.txMgr.WithTx(ctx, func(txCtx context.Context) error {
		// (2a) SELECT rooms WHERE id = ? FOR UPDATE → 找不到 → 1009 数据不一致兜底
		if _, err := s.roomRepo.FindByIDForUpdate(txCtx, in.RoomID); err != nil {
			return err // 含 ErrRoomNotFound 哨兵 / DB raw error；service 层兜底翻译为 1009
		}

		// (2b) DELETE room_members WHERE room_id = ? AND user_id = ?；rowsAffected == 0 → 6004 兜底
		rowsAffected, err := s.roomMemberRepo.DeleteByRoomAndUser(txCtx, in.RoomID, in.UserID)
		if err != nil {
			return err
		}
		if rowsAffected == 0 {
			return errLeaverNotInRoomInternal
		}

		// (2c) UPDATE users.current_room_id = NULL（**首次启用 nil 入参路径**）
		if err := s.userRepo.UpdateCurrentRoomID(txCtx, in.UserID, nil); err != nil {
			return err
		}

		// (2d) SELECT COUNT(*) FROM room_members WHERE room_id = ? → remaining
		remaining, err := s.roomMemberRepo.CountByRoomID(txCtx, in.RoomID)
		if err != nil {
			return err
		}

		// (2e) 最后一人离开 → UPDATE rooms.status = 2 closed
		if remaining == 0 {
			if err := s.roomRepo.UpdateStatus(txCtx, in.RoomID, roomStatusClosed); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		// (3) 业务码分流（**errors.Is 顺序关键**：6004 必须先于 generic 1009 判定）。
		if stderrors.Is(err, errLeaverNotInRoomInternal) {
			return nil, apperror.New(apperror.ErrUserNotInRoom, apperror.DefaultMessages[apperror.ErrUserNotInRoom])
		}
		// **关键**：mysql.ErrRoomNotFound（步骤 2a SELECT FOR UPDATE 找不到 rooms 行）
		// 走 **1009** 路径而**不是** 6001 —— V1 §10.5 行 1597 / 行 1599 钦定 leave
		// 接口**不**对外暴露 6001（理论不会发生，因为步骤 1 已通过意味着 caller 在该
		// 房间，rooms 行必存在；race 兜底按 DB 异常处理走 1009）。直接走 generic
		// ErrServiceBusy 兜底分支（不需要单独识别 mysql.ErrRoomNotFound）。
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// (4) 事务 commit 成功 → 返回。
	//
	// ============================================================================
	// SECURITY-DEFER (Story 11.8 钦定范围 — 11.5 严守范围红线，刻意不实装)
	// ============================================================================
	//
	// **当前现状**（本 story 11.5 done 后到 11.8 done 之前的窗口）：
	//   LeaveRoom 事务 commit 后**直接 return**，**不**触碰 SessionManager。
	//   若 leaver 此刻仍持有 `/ws/rooms/:roomId` WS 连接（典型场景：iOS 端 client
	//   先发 HTTP leave 再自行 close WS，但中间存在 ms 级窗口；或 client 实装层
	//   依赖 server close 4007 才退订），其 Session 仍 registered 在 SessionManager
	//   的 room → sessions 索引里。
	//
	// **影响范围**（11.8 实装前）：
	//   - SessionManager.ListSessionsByRoomID(roomID) 在 leave 事务 commit 后仍把
	//     leaver Session 算作该房间的 active member；
	//   - BroadcastToRoom(roomID, ...)（Story 10.5 primitive）fanout 列表仍含 leaver，
	//     leaver 在 close 4007 / 心跳超时（默认 60s）窗口内会继续收到本 roomId 的
	//     `member.joined` / 后续 epic 广播（如 14.x `pet.state.changed` /
	//     17.x `emoji.received`），违反 V1 §10.5 行 1544 "HTTP leave 后立即与房间
	//     WS 解耦" 语义；
	//   - presence renewal（如 Story 11.x heartbeat tick）仍把 leaver 当作该房间在
	//     线成员处理，直到 socket 自然断开 / 心跳超时框架被动清理。
	//
	// **11.8 必装内容（V1 §10.5 钦定）**：
	//   (a) 步骤 7：close 4007 + unregister leaver Session —— 从 SessionManager
	//       撤销 leaver 在该 roomID 的 Session + close underlying WebSocket
	//       (close code = 4007, reason = "left room via HTTP")；
	//   (b) 步骤 8：BroadcastToRoom(roomID, {type: "member.left", ...}) —— payload
	//       字段表见 §12.3 `### 成员离开`；
	//   (c) 顺序由 V1 §10.5 r13 钦定：**先 close + unregister，后 broadcast**，
	//       让 fanout 时 ListSessionsByRoomID 返回列表自然不含 leaver Session
	//       —— 无需 BroadcastToRoom primitive 加 excludeUserID 参数。
	//
	// **11.5 与 11.8 之间窗口的暴露面**：节点 4 demo 验收（epic 13）触发 epic 11
	// 全 done 才会出现该窗口在 prod 出现的可能；epic 11 中间不存在 prod release
	// 节奏暴露此问题（11.6 / 11.7 / 11.8 仍在同一节点内连续推进）。
	//
	// **codex review r1 (2026-05-08) flag 此问题为 [P1] 状态不一致** —— review 工程
	// 上 correct，但跨 story scope，本 story spec
	// (`_bmad-output/implementation-artifacts/11-5-退出房间事务.md` 行 53-54 / 251-259 /
	// 740 / 767 / 777) 严守 "本 story 不调任何 WS primitive" 红线 ——
	// 转 **defer-to-11.8**。详细 trade-off / cross-story traceability 见 lesson
	// `docs/lessons/2026-05-09-leave-room-ws-session-cleanup-defer-to-11-8-11-5-r1.md`。
	// ============================================================================
	return &LeaveRoomOutput{
		RoomID: in.RoomID,
		Left:   true,
	}, nil
}

// GetCurrentRoom 实装 V1 §10.2 钦定服务端逻辑（Story 11.6 引入）。
//
// 仅 1 步：userRepo.FindByID 取 user.CurrentRoomID 字段；不开事务（单字段查询，
// 无并发一致性诉求）。
//
// **错误处理**：
//   - userRepo.FindByID 返 ErrUserNotFound（理论 race 兜底）→ 包成 1009
//   - DB error → 包成 1009
//   - happy → 直接返 GetCurrentRoomOutput{RoomID: user.CurrentRoomID}（含 nil 路径）
//
// **不**触发 6001 ~ 6005（V1 §10.2 钦定），不区分"用户不在任何房间"vs"房间已 closed"。
func (s *roomServiceImpl) GetCurrentRoom(ctx context.Context, in GetCurrentRoomInput) (*GetCurrentRoomOutput, error) {
	user, err := s.userRepo.FindByID(ctx, in.UserID)
	if err != nil {
		// 任何 err（含 ErrUserNotFound 兜底）都包成 1009 —— 与 11.3 / 11.4 / 11.5
		// 预检 FindByID 失败路径同模式（auth middleware 已确保 caller 有 user 行；
		// race 下兜底）
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	return &GetCurrentRoomOutput{RoomID: user.CurrentRoomID}, nil
}

// GetRoomDetail 实装 V1 §10.3 + 数据库设计 §8.8 钦定的"读快照事务（含 ACL 共享锁）"
// 4 步事务（Story 11.6 引入）：
//
//	步骤 1（事务内 4 步）：
//	  1a. userRepo.FindByID(txCtx, UserID) → CurrentRoomID 校验 → 不一致 → 6004
//	  1b. roomMemberRepo.ExistsForShareByRoomAndUser(txCtx, RoomID, UserID) →
//	      false → 6004 兜底（FOR SHARE 0 行）
//	  2.  roomRepo.FindByID(txCtx, RoomID) → ErrRoomNotFound → 6001
//	  3.  roomMemberRepo.ListRosterByRoomID(txCtx, RoomID) → roster
//	步骤 2（事务后）：commit / rollback；commit 成功 → 拼装 GetRoomDetailOutput
//	  含 BIGINT 字段 + LEFT JOIN pet-less nullable + 节点 4 固定字段
//
// **错误码触发顺序锁定**（V1 §10.3 钦定）：步骤 1a → 6004（预检）；1b → 6004（兜底）；
// 步骤 2 → 6001；任何 DB 异常 → 1009。**不**对外暴露 6002 / 6003 / 6005。
//
// **6004 双路径必须等价**：步骤 1a + 步骤 1b 都返 apperror.New(ErrUserNotInRoom, ...)，
// handler 端响应 envelope 完全一致。
//
// **ctx 用法**：fn 内全部 4 个 repo 调用用 txCtx；用错 ctx 让 FOR SHARE 锁立即释放，
// post-leave 数据泄漏 race 重新出现。
//
// **节点 4 硬编码字段**：MemberPetOutput.CurrentState 固定 1 (rest)；Equips 固定 [];
// 节点 5 / 9 / 10 由 Epic 14 / 26 / 29 真实驱动时改为 query 结果。
//
// **LEFT JOIN pets 行为**：RosterRow.PetID 为 *uint64；nil 表示 pet-less（LEFT JOIN
// 行 NULL）→ MemberOutput.Pet == nil；非 nil → MemberOutput.Pet = &MemberPetOutput{
// PetID: *RosterRow.PetID, ...}（节点 4 硬编码 CurrentState / Equips）。
func (s *roomServiceImpl) GetRoomDetail(ctx context.Context, in GetRoomDetailInput) (*GetRoomDetailOutput, error) {
	var out *GetRoomDetailOutput

	err := s.txMgr.WithTx(ctx, func(txCtx context.Context) error {
		// 步骤 1a: ACL 预检（事务内查 users.current_room_id）
		user, err := s.userRepo.FindByID(txCtx, in.UserID)
		if err != nil {
			return err // 含 ErrUserNotFound 哨兵 / DB raw error；外层包成 1009
		}
		if user.CurrentRoomID == nil || *user.CurrentRoomID != in.RoomID {
			return apperror.New(apperror.ErrUserNotInRoom, apperror.DefaultMessages[apperror.ErrUserNotInRoom])
		}

		// 步骤 1b: ACL FOR SHARE 兜底（race 防御）
		inRoom, err := s.roomMemberRepo.ExistsForShareByRoomAndUser(txCtx, in.RoomID, in.UserID)
		if err != nil {
			return err // DB raw error；外层包成 1009
		}
		if !inRoom {
			// 步骤 1a 通过但步骤 1b 0 行 → 并发 leave 已删该行 → 6004 兜底
			return apperror.New(apperror.ErrUserNotInRoom, apperror.DefaultMessages[apperror.ErrUserNotInRoom])
		}

		// 步骤 2: 查 rooms（不带锁普通查询）
		room, err := s.roomRepo.FindByID(txCtx, in.RoomID)
		if err != nil {
			if stderrors.Is(err, mysql.ErrRoomNotFound) {
				return apperror.New(apperror.ErrRoomNotFound, apperror.DefaultMessages[apperror.ErrRoomNotFound])
			}
			return err
		}

		// 步骤 3: 查 roster（INNER JOIN users + LEFT JOIN pets，ORDER BY joined_at ASC）
		roster, err := s.roomMemberRepo.ListRosterByRoomID(txCtx, in.RoomID)
		if err != nil {
			return err
		}

		// 拼装 output
		members := make([]MemberOutput, 0, len(roster))
		for _, r := range roster {
			m := MemberOutput{
				UserID:    r.UserID,
				Nickname:  r.Nickname,
				AvatarURL: r.AvatarURL,
			}
			if r.PetID != nil {
				// 非 pet-less：节点 4 阶段硬编码 CurrentState=1 / Equips=[]
				m.Pet = &MemberPetOutput{
					PetID:        *r.PetID,
					CurrentState: 1,                // V1 §10.3 节点 4 阶段固定 1 (rest)
					Equips:       []EquipOutput{}, // 节点 4 阶段固定 []
				}
			}
			members = append(members, m)
		}

		out = &GetRoomDetailOutput{
			RoomID:        room.ID,
			CreatorUserID: room.CreatorUserID,
			MaxMembers:    room.MaxMembers,
			Status:        room.Status,
			MemberCount:   len(members), // 不变量：== len(Members)
			Members:       members,
		}
		return nil
	})

	if err != nil {
		// 业务码分流：apperror 已包装的直接透传（步骤 1a / 1b 6004 + 步骤 2 6001）
		var ae *apperror.AppError
		if stderrors.As(err, &ae) {
			return nil, err
		}
		// 所有未识别错误（含 ErrUserNotFound / DB raw / ListRosterByRoomID error
		// / ExistsForShareByRoomAndUser error）走 generic 1009 兜底
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	return out, nil
}
