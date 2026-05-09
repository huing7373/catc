package service

import (
	"context"
	stderrors "errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	ws "github.com/huing/cat/server/internal/app/ws"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
)

// postCommitTimeout 是 post-commit fire-and-forget goroutine 的超时上限（Story 11.8
// codex review r2 [P1] / [P2] 修复引入）。
//
// **为何需要超时**（Story 11.8 review r2 lesson）：post-commit hook 用 detached ctx
// （context.WithoutCancel）解除 request ctx cancel 信号 —— 这是为了让 broadcast /
// session close 不被 client 主动断开 / handler deadline 触发的 cancel 误中断（否则
// member.joined 静默 skip / leaver session 残留）。但完全 detached 会引入 goroutine
// 泄漏风险（DB 卡死 / SessionManager 死锁 → goroutine 永不返回）。所以**必须**给
// detached ctx 加独立 timeout 兜底。
//
// **10s 选型**：post-commit 全部 work（user/pet lookup + 1 次 marshal + broadcastFn
// fanout + Session.CloseWithCode 含 ~5s WS write timeout drain）总时间上界 ~6s；
// 取 10s 留冗余 + 允许 worst-case write loop 排队。Future 节点如有 SessionManager
// 性能压测可调小到 5s。
const postCommitTimeout = 10 * time.Second

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
	// post-commit 阶段触发 fire-and-forget WS 广播 member.joined（V1 §10.4 步骤 8）。
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
	// **post-commit 阶段**触发（V1 §10.5 步骤 7 + 步骤 8 钦定）：
	//   1. close 4007 + SessionManager.Unregister leaver Session（步骤 7；fire-and-forget）
	//   2. BroadcastToRoom(roomID, member.left) 给该房间其他在线成员（步骤 8；fire-and-forget）
	//
	// 顺序由 V1 §10.5 r13 钦定 —— 步骤 7 先于步骤 8，让 broadcast fanout 时
	// ListSessionsByRoomID 返回列表自然不含 leaver Session（BroadcastToRoom primitive
	// 不带 excludeUserID 参数）。
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
//   - userRepo: users 表访问；调 FindByID（预检 / member.joined enrichment）+ UpdateCurrentRoomID（事务内）
//   - roomRepo: rooms 表访问；调 Create / FindByIDForUpdate / FindByID / UpdateStatus
//   - roomMemberRepo: room_members 表访问；调 Create / DeleteByRoomAndUser / CountByRoomID / ListRosterByRoomID / ExistsForShareByRoomAndUser
//   - petRepo: pets 表访问（Story 11.8 引入）；用于 broadcastMemberJoined 查询加入者
//     默认宠物以构造 member.joined.payload.pet（pet-less → ErrPetNotFound 走 nil 路径）
//   - sessionMgr: WS Session 注册中心（Story 11.8 引入；10.3 落地）；用于 LeaveRoom
//     post-commit 阶段查询 leaver 在该 roomID 的 Session + close 4007 + Unregister
//   - broadcastFn: WS 广播函数值（Story 11.8 引入；10.5 落地的 BroadcastFn type alias）；
//     用于 fire-and-forget 推 member.joined / member.left 给房间其他在线 Session
type roomServiceImpl struct {
	txMgr             tx.Manager
	userRepo          mysql.UserRepo
	roomRepo          mysql.RoomRepo
	roomMemberRepo    mysql.RoomMemberRepo
	petRepo           mysql.PetRepo        // Story 11.8 加：member.joined 事件 pet enrichment
	sessionMgr        ws.SessionManager    // Story 11.8 加：close leaver Session（leave 路径）
	broadcastFn       ws.BroadcastFn       // Story 11.8 加：fire-and-forget broadcast（保留兼容；当前 join/leave 都走 broadcastExceptFn）
	broadcastExceptFn ws.BroadcastExceptFn // Story 11.8 r3 P1 fix：member.joined / member.left 必须排除事件主体自己（V1 §12.3 行 2063）

	// postCommitWG **仅供测试同步**（Story 11.8 codex review r2 修复引入）：tests
	// 注入一个 *sync.WaitGroup 让 enqueueRoomEvent enqueue 时 Add(1)、worker 跑完
	// fn 时 Done()，let tests 调 wg.Wait() 后再断言 broadcast / close 副作用是否
	// 符合预期。production 路径不注入 → nil → 不做 WG 簿记，与 production 行为
	// 完全一致（fire-and-forget 严格语义不引入额外开销）。
	//
	// 通过 SetPostCommitWaitGroupForTest 注入；不出现在 NewRoomService 签名里
	// （production caller 永不需要）。
	postCommitWG *sync.WaitGroup

	// roomQueues **per-room FIFO queue pool**（Story 11.8 codex review r5 [P1]
	// 修复引入；取代 r4 的 perRoomMu 方案）：保证同一 roomID 的 post-commit hook
	// **严格按 caller commit 顺序**执行，不同 roomID 仍并行。
	//
	// **背景**（review r5 [P1]）：r4 用 sync.Map[roomID]*sync.Mutex 试图保留 causal
	// ordering，但 mutex Lock 在 goroutine **内**才执行 —— 两个 caller 同步 commit
	// (join → leave) 后分别 `go func(){...}()`，两个 goroutine 启动顺序由 scheduler
	// 决定，后者可能抢先拿到 mutex 并 broadcast，因果顺序仍然破坏。**根因**：保序
	// 必须在 caller **同步段**完成（enqueue 时 lock-step 入队），不能等到 goroutine
	// 调度起来才取序。
	//
	// **修法**（path A，FIFO channel queue + worker goroutine）：每个 roomID 一个
	// buffered channel + 一个 worker goroutine（sync.Once 保证只启一次）。caller
	// **同步段** non-blocking 入 channel —— enqueue 顺序就是 commit 顺序。worker
	// 顺序消费 channel，broadcast 严格按 enqueue 顺序执行。**关键**：enqueue 是
	// caller 同步段在 LoadOrStore 后立即 channel send，不依赖 goroutine 调度。
	//
	// **不同 roomID 仍并行**：每个 roomID 独立 channel + worker，互不阻塞，吞吐不损。
	//
	// **queue 满时背压**（r9 [P2] 修订，原 r5 silent drop 路径已废弃）：channel
	// 容量 1024；满了 caller 阻塞 send 等 worker 消费 —— silent drop 会让 client
	// 漏 member.joined / member.left → roster 永久 stale，是 silent corruption；
	// 阻塞 caller 自然形成背压，最坏延迟 HTTP 200 但不丢事件，监控可见可 alarm。
	// 节点 4 阶段单 room ≤ 4 user，post-commit 处理速率 ~10ms 级，1024 容量需要
	// burst > 1024 / 30s 才会满 —— 极端场景。
	//
	// **queue / worker 不清理**（intentional）：节点 4 阶段 V1 §10.5 钦定房间 status
	// 严格单调（active → closed 无回退），单进程生命周期内活跃 room 数量有界；不会
	// 无限增长。worker goroutine 通过 channel close 机制下线；当前实装不主动 close
	// （room closed 不触发 worker 退出，让残留事件能完成）。如未来引入 dynamic
	// room reuse 或活跃 room 爆炸 → 加 LRU eviction 或定时 GC。
	//
	// LIFECYCLE-DEFER: Story 11-8 r8 决策 defer，详见 docs/lessons/2026-05-09-per-room-worker-lifecycle-defer-tech-debt-11-8-r8.md
	// （codex r8 [P2] / r9 [P1] 重复 flag worker leak；MVP 节点 4 阶段量化上界可控
	// —— 单进程活跃 room ≤ 上千、worker 栈 ~2KB、roomQueue ~8KB（1024-cap chan），
	// 总开销 < 10MB；future epic 单独 story 处理 lifecycle 回收。三处 LIFECYCLE-DEFER
	// 标记 ——`roomQueues` 字段 / `enqueueRoomEvent` / `runRoomQueueWorker` ——
	// 防 r10 review 再 flag。）
	roomQueues sync.Map

	// roomCommitLocks **per-room commit serialization mutex pool**（Story 11.8
	// codex review r6 [P1] 修复引入）：保证同一 roomID 的 (业务事务 commit + 事件
	// enqueue) **作为一个原子段**串行化，让 caller commit 顺序严格 == enqueue 顺序
	// == worker 消费顺序 == client 感知顺序。
	//
	// **r6 [P1] 背景**（review r6 两条 finding 同根因）：r5 用 channel FIFO 队列保
	// caller-commit-order，前提**必须**是 "commit 后立刻 enqueue，期间无其他同步
	// 操作可被 concurrent 路径夹塞"。但 r5 LeaveRoom 实装在 commit 后插入了同步段
	// `unregisterLeaverSessionSync`，再 enqueue —— 该 gap 内：
	//
	//   (a) leaver 仍在 SessionManager；concurrent JoinRoom 同时 commit + enqueue
	//       member.joined → fanout 列表包含 leaver → leaver 收到 stale event。
	//   (b) concurrent JoinRoom 的 enqueue 可抢先 LeaveRoom 的 enqueue → client
	//       收到 member.joined 早于 member.left → 违反 commit-order = causal-order。
	//
	// **修法**（commit-time per-room serialization；r7 修订：unregister 同步段
	// 留在 lock 内）：JoinRoom / LeaveRoom 在事务之前 acquire per-roomID mutex，
	// 事务 commit + (LeaveRoom: unregister) + enqueueRoomEvent + (LeaveRoom: close
	// goroutine 启动) 都在 lock 内完成，再 unlock。这样：
	//
	//   1. **同 room 事务串行 commit**：两个 same-room 事务不能 interleave commit
	//      → commit 顺序确定 → enqueue 顺序确定（lock 内紧接 commit）。
	//   2. **unregister 留在 lock 同步段**（r7 修订）：LeaveRoom 在 commit 之后、
	//      enqueue 之前同步执行 unregisterLeaverSessionSync —— HTTP 200 返回前
	//      leaver 已从 SessionManager 双索引清除，"HTTP leave immediately
	//      detaches WS" 钦定达成；worker backlog 中前序排队的 broadcast 跑到时
	//      ListSessionsByRoomID 已不再返 leaver。Map op O(1) nano 级，不破坏
	//      lock 内 "全 instant op" 不变量。
	//   3. **close goroutine 仍 fire-and-forget**：runCloseLeaverAsync 启动是
	//      instant op，可以在 lock 内启动；CloseWithCode drain 慢路径仍跑在独立
	//      goroutine，不阻塞 lock / 不阻塞 worker。
	//
	// **trade-off**：same-room 事务 commit 串行化（不同 room 仍并行）。节点 4 阶段
	// 单 room ≤4 人，并发极低，可接受（设计 §10.4 / §10.5 join/leave QPS 极小）。
	// 未来如有"高并发 same-room"场景，需要重新评估（用户协议层 sequence number 或
	// 客户端排序兜底）。
	//
	// **lock 内只允许 instant op**：DB commit + channel send（buffered 默认非阻塞）+
	// goroutine 启动。**不允许**做 IO / wait / sleep / 远程调用。CloseWithCode drain
	// 这种慢路径必须留在 lock 之外的独立 goroutine（`runCloseLeaverAsync`）。
	//
	// **defer Unlock 兜底**：JoinRoom / LeaveRoom 内拿 lock 后立即 `defer mu.Unlock()`
	// 兜底（panic / err 路径也保证 unlock）。defer 触发在 return 计算后、HTTP 响应
	// 写出前 —— lock 内只有 instant op，HTTP 延迟增加 < 1ms，client 无感。
	//
	// **生命周期同 roomQueues**：LoadOrStore 模式，节点 4 阶段不主动清理；如未来
	// 引入 dynamic room reuse → 同步加 LRU eviction。
	roomCommitLocks sync.Map
}

// roomQueue 是单 roomID 的 post-commit FIFO 通道（Story 11.8 r5 [P1] 修复引入）。
//
// **结构**：
//   - ch: buffered channel，caller 同步段 send，worker 顺序 receive。channel 自身
//     即 FIFO 语义（Go runtime 保证）。
//   - once: 保证 worker goroutine 只启动一次 —— 多个 caller 并发 LoadOrStore 同
//     一 roomID 时，第一个 winner 把 *roomQueue 存入 sync.Map，后续 caller 读到
//     同一 *roomQueue 实例；once.Do 让 worker 只在首次 enqueue 时启动。
type roomQueue struct {
	ch   chan func()
	once sync.Once
}

// SetPostCommitWaitGroupForTest 仅供测试使用：注入一个 *sync.WaitGroup 让测试可以
// 同步等待 post-commit 异步 goroutine 完成（Story 11.8 codex review r2 修复引入）。
//
// **production 严格禁止调用**：post-commit hook 必须 fire-and-forget，加 WaitGroup
// 等待会破坏 fire-and-forget 语义（让 caller 阻塞等异步 goroutine 完成）。
//
// 测试使用模式：
//
//	wg := &sync.WaitGroup{}
//	svc := service.NewRoomService(...)
//	service.SetPostCommitWaitGroupForTest(svc, wg)
//	_, _ = svc.JoinRoom(ctx, in)
//	wg.Wait() // 等 post-commit goroutine 完成
//	// 此时安全断言 bcast.callCount() / sessionMgr.unregisterCalls 等副作用
func SetPostCommitWaitGroupForTest(svc RoomService, wg *sync.WaitGroup) {
	if impl, ok := svc.(*roomServiceImpl); ok {
		impl.postCommitWG = wg
	}
}

// acquireCommitLock 取得 roomID 对应的 *sync.Mutex 并返回（Story 11.8 codex
// review r6 [P1] 修复引入）。caller 拿到 mutex 后**自行**调用 Lock —— 本函数
// 只负责 LoadOrStore 拿同一 *sync.Mutex 实例，不锁定。
//
// **使用模式**（JoinRoom / LeaveRoom 同模式）：
//
//	mu := s.acquireCommitLock(in.RoomID)
//	mu.Lock()
//	defer mu.Unlock()
//	// 事务 commit + enqueueRoomEvent 都在 lock 内
//
// **为什么 Lock / Unlock 显式调用而不在 acquire 内做**：caller 需要 defer
// mu.Unlock() 兜底 panic / 早返回路径；在 acquire 内 Lock 会让 caller 漏掉
// defer 导致 lock 永远不释放。显式 Lock + 显式 defer Unlock 是 Go 标准模式。
//
// **不同 roomID 不共享 mutex**：sync.Map LoadOrStore 保证同一 roomID 拿到同一
// *sync.Mutex；不同 roomID 拿到不同实例，互不阻塞。
//
// **生命周期**：LoadOrStore 模式不主动清理（与 roomQueues 同策略，节点 4 阶段
// 活跃 room 数有界）。
//
// **LIFECYCLE-DEFER 分层**（review r8 / r9 / r10 累积结论）：
//
//	r8 / r9: successful-join 路径下 real roomID 的 lock entry 不回收 —— tech-debt
//	         已 defer，与 roomQueues / worker goroutine 同源，节点 4 阶段不构成
//	         attack vector（attacker 必须先获得 valid roomID 才能触发，速率受限）。
//	r10:     **failed-join 路径**（room 不存在）必须**不进**本函数 —— attacker
//	         可用任意 fake roomID 暴力调用 JoinRoom，若进 lock 段则每次都
//	         LoadOrStore 一个新 entry → memory leak 攻击面扩大到
//	         attacker-controlled。修法：JoinRoom 在调用本函数**之前**先做
//	         cheap room-exists check（普通 SELECT），不存在 → 直接 6001 返回，
//	         **不进**本函数，attack vector 关闭。详见 JoinRoom 注释。
//
// **关系**：r10 修复**不**解决 r8/r9 的 successful-join leak（仍是 tech-debt）；
// r10 仅关闭 unauthenticated/non-existent attack vector。两者同 family（map entry
// 不回收），分层应对。
func (s *roomServiceImpl) acquireCommitLock(roomID uint64) *sync.Mutex {
	muIface, _ := s.roomCommitLocks.LoadOrStore(roomID, &sync.Mutex{})
	return muIface.(*sync.Mutex)
}

// enqueueRoomEvent 把 post-commit fn 入队到 roomID 对应的 FIFO channel，由 worker
// goroutine 顺序消费（Story 11.8 codex review r5 [P1] 修复引入；取代 r4 的
// runPostCommitAsyncPerRoom + sync.Map[*sync.Mutex] 方案）。
//
// **fire-and-forget 严格语义**（V1 §10.4 步骤 8 / §10.5 步骤 7-8 钦定）：
//
//  1. **caller 不等异步结果**（r9 [P2] 修订：satisfied 但 caller 在 queue 满时
//     会 block 等 worker 消费一个槽位；非 1024-cap 满状态下立即返回）。caller
//     同步段做 channel send，worker goroutine 独立消费 channel 跑 fn；caller
//     拿不到 fn 结果（无 future / promise）—— "fire-and-forget" 语义本质指
//     "不等结果"，不指"不阻塞"；queue 满时阻塞背压优于 silent drop（r5 实装
//     的 silent drop 路径会让 client 漏 broadcast → roster permanently stale）。
//
//  2. **detached ctx**（context.WithoutCancel）：fn 收到的 ctx 与 request ctx 解耦
//     cancel 信号但保留 values。client 断开 / handler deadline cancel request
//     ctx 时，post-commit 不被误中断。
//
//  3. **timeout 兜底**（postCommitTimeout）：detached ctx 完全解耦 cancel 信号
//     会引入 goroutine 泄漏风险；显式加 10s timeout。
//
//  4. **strict caller-commit-order causal ordering**（r5 [P1] 修复核心）：
//     **enqueue 必须在 caller 同步段完成** —— 这样 enqueue 顺序就是 caller commit
//     顺序（Go channel 是 FIFO；同一 roomID 的所有事件走同一 channel；同一 channel
//     send 在 receive 前严格保序）。worker goroutine receive 顺序 = enqueue 顺序 =
//     commit 顺序，causal ordering 严格保留。
//
//     **关键差异 vs r4 perRoomMu**：r4 在 goroutine 内 Lock 同一 mu —— 但两个 caller
//     commit 后各自 `go func()`，goroutine 启动顺序由 Go scheduler 决定，后者
//     可能抢先 Lock，破坏因果序。本方案 enqueue 走 channel send 是 caller 同步
//     段动作，scheduler race 完全消除。
//
//  5. **WG 簿记**（测试同步）：caller 同步段 wg.Add(1)；worker 跑完 fn 后 wg.Done()。
//     test 调 wg.Wait() 时，所有 enqueue 的事件必已 Done（worker 顺序消费 +
//     wg.Done 在 fn 返回后立即调）。production wg=nil，零开销。
//
// **panic safety**：worker 内**不**用 defer recover —— fn 内 panic 走 default Go
// runtime 行为（abort process）。与 r2 / r4 行为一致。
//
// **queue 启动顺序**（重要）：必须先 LoadOrStore 拿到 *roomQueue → 再 once.Do
// 启动 worker → 最后 channel send。**不可**先 send 再 once.Do —— 那样 worker 还
// 没起来事件就到达 channel，会一直留 channel 里等到 once.Do 才被消费（语义
// 仍正确但增加无谓延迟）。当前顺序：LoadOrStore 后立即 once.Do（启 worker），
// 再 wg.Add + send，让 worker 已 ready 时事件入队即被消费。
//
// LIFECYCLE-DEFER: Story 11-8 r8 决策 defer，详见 docs/lessons/2026-05-09-per-room-worker-lifecycle-defer-tech-debt-11-8-r8.md
// （per-room worker / sync.Map entry 永不 reclaim 是已知 tech-debt；MVP 节点 4
// 阶段量化上界可控；future epic 单独 story 处理。codex r9 [P1] 重复 flag —— 三处
// LIFECYCLE-DEFER 标记防 r10 再 flag。）
func (s *roomServiceImpl) enqueueRoomEvent(ctx context.Context, roomID uint64, fn func(detachedCtx context.Context)) {
	// LIFECYCLE-DEFER: Story 11-8 r8 决策 defer，详见 docs/lessons/2026-05-09-per-room-worker-lifecycle-defer-tech-debt-11-8-r8.md
	// 容量 1024（r9 [P2] 修复：双层防御 —— blocking send 是首选背压，1024 容量是常态吞吐覆盖）。
	qIface, _ := s.roomQueues.LoadOrStore(roomID, &roomQueue{ch: make(chan func(), 1024)})
	q := qIface.(*roomQueue)
	q.once.Do(func() {
		go s.runRoomQueueWorker(q)
	})

	// **caller 同步段**：构造 wrapped fn（含 detached ctx + timeout）
	// 之所以放在 enqueue 端而不是 worker 端构造：detached ctx 需要继承 caller ctx
	// 的 values（trace ID / request ID）；caller ctx 在 caller 同步段是 live 的，
	// 跨 channel 传递时只传 fn closure 即可。
	wrapped := func() {
		detached := context.WithoutCancel(ctx)
		timedCtx, cancel := context.WithTimeout(detached, postCommitTimeout)
		defer cancel()
		fn(timedCtx)
	}

	// **WG 簿记必须在 send 前完成**（caller 同步段 Add 才能保证后续 wg.Wait
	// 不 race miss Add）。Done() 在 worker 内 fn 返回后立即调（runRoomQueueWorker
	// 持有 wg ref；fire-and-forget 路径 wg=nil 时 worker 不动 wg）。
	if s.postCommitWG != nil {
		s.postCommitWG.Add(1)
	}

	// **blocking enqueue**（r9 [P2] 修复，取代 r5 的 non-blocking select default）：
	// channel 满 → caller 阻塞等待 worker 消费一个槽位再送入。理由：
	//   (a) silent drop（r5 默认行为）会让 client 漏 member.joined / member.left →
	//       roster permanently stale，是 silent correctness bug，比 HTTP 延迟严重。
	//   (b) 阻塞 caller 自然形成背压：caller 慢 → 上游 HTTP 慢 → 不会继续制造更
	//       多 backlog，系统自洽收敛。
	//   (c) 容量已 1024（双层防御），节点 4 阶段单 room ≤4 user，post-commit 处理
	//       速率 ~10ms 级，1024 容量需要 burst > 1024 / 30s 才会满 —— 极端场景。
	//   (d) 阻塞最坏延迟 HTTP 200 而不是丢事件，监控可见（HTTP latency P99 上升）
	//       可被 alarm 捕获；silent drop 无任何信号。
	q.ch <- wrapped
}

// runRoomQueueWorker 是单 roomID 的 worker goroutine（Story 11.8 r5 [P1] 修复引入）。
//
// **职责**：从 q.ch 顺序消费 wrapped fn 并执行；fn 返回后调用 wg.Done() 解除测试
// wait。channel close 时 worker 退出（当前实装不主动 close —— 节点 4 阶段不需要
// 显式回收 worker，参见 roomQueues 字段注释）。
//
// **顺序保证**：channel receive 严格 FIFO（Go runtime spec），所以 worker 跑 fn
// 的顺序 = enqueue 顺序 = caller commit 顺序。
//
// **panic safety**：fn panic 会 abort process（default Go 行为）；worker goroutine
// 不加 recover，与 r2 / r4 行为一致。
//
// LIFECYCLE-DEFER: Story 11-8 r8 决策 defer，详见 docs/lessons/2026-05-09-per-room-worker-lifecycle-defer-tech-debt-11-8-r8.md
// （worker 永不退出，当前实装不主动 close channel；MVP 节点 4 阶段量化上界可控；
// future epic 单独 story 处理 lifecycle 回收。三处 LIFECYCLE-DEFER 标记防 r10 再 flag。）
func (s *roomServiceImpl) runRoomQueueWorker(q *roomQueue) {
	for fn := range q.ch {
		fn()
		if s.postCommitWG != nil {
			s.postCommitWG.Done()
		}
	}
}

// NewRoomService 构造 RoomService（Story 11.8 扩展为 8 参数；r3 加 broadcastExceptFn）。
//
// 全部依赖通过参数显式注入；不引入 wire / fx 框架（与 4.2 / 4.4 / 4.5 / 4.6 / 7.3 /
// 11.3 ~ 11.7 同模式）。
//
// Story 11.8 新增参数：
//   - petRepo: pets 表访问；用于 JoinRoom post-commit 阶段查询加入者默认宠物以
//     构造 member.joined.payload.pet（pet-less → ErrPetNotFound 走 nil 路径）
//   - sessionMgr: WS Session 注册中心（10.3 落地）；用于 LeaveRoom post-commit
//     阶段查询 leaver 在该 roomID 的 Session + close 4007 + Unregister
//   - broadcastFn: WS 广播函数值（10.5 落地的 BroadcastFn type alias）；保留兼容，
//     当前 member.joined / member.left 路径已切到 broadcastExceptFn（不再调用；
//     未来其他广播路径如 chat 类全 fanout 时可使用本函数）
//   - broadcastExceptFn: WS 广播函数值（r3 加；对应 ws.BroadcastToRoomExcept）；
//     用于 fire-and-forget 推 member.joined / member.left 给**除事件主体外**
//     的其他在线 Session（V1 §12.3 行 2063 钦定 "joiner 不收自己的 member.joined"
//     语义；Story 11.8 r3 [P1] fix 由 service 层显式 exclude 防御 joiner 在
//     post-commit 异步段已建立 WS 的 race）
//
// **关键决策**：sessionMgr 直接传 SessionManager 实例（用于 ListSessionsByRoomID
// / Unregister 双方法调用，无现成函数 alias 抽象）；broadcastFn / broadcastExceptFn
// 传 BroadcastFn / BroadcastExceptFn 函数值（让单测注入 mock closure 简单，与
// ws/broadcast.go 行 40-69 钦定的注入模式一致）。
func NewRoomService(
	txMgr tx.Manager,
	userRepo mysql.UserRepo,
	roomRepo mysql.RoomRepo,
	roomMemberRepo mysql.RoomMemberRepo,
	petRepo mysql.PetRepo,
	sessionMgr ws.SessionManager,
	broadcastFn ws.BroadcastFn,
	broadcastExceptFn ws.BroadcastExceptFn,
) RoomService {
	return &roomServiceImpl{
		txMgr:             txMgr,
		userRepo:          userRepo,
		roomRepo:          roomRepo,
		roomMemberRepo:    roomMemberRepo,
		petRepo:           petRepo,
		sessionMgr:        sessionMgr,
		broadcastFn:       broadcastFn,
		broadcastExceptFn: broadcastExceptFn,
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
// post-commit 阶段调 s.broadcastMemberJoined(ctx, in.RoomID, in.UserID) 触发
// fire-and-forget WS 广播（V1 §10.4 步骤 8 钦定，事务外 fire-and-forget；broadcast
// 失败不影响 HTTP 200 响应）。
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

	// (1') **r10 [P1] 修复**：lock 之前先做 cheap room-exists check（普通 SELECT，
	// 无 FOR UPDATE）—— 防止 attacker 用随机 / 不存在的 roomID 暴力 join 在
	// roomCommitLocks sync.Map 内 LoadOrStore 出无穷多 entry 制造 memory leak
	// （attacker-controlled）。
	//
	// **不变量**：本 check **best-effort**（lock 之前；race 内 room 可能被 close），
	// 真正的正确性保证仍由事务内 SELECT FOR UPDATE 维护 —— 本 check 仅过滤掉
	// 100% 不存在的 roomID，绝大多数 attacker 暴力 join 路径在此被丢弃，不再
	// 污染 commit lock map。
	//
	// **与 r8/r9 worker leak 关系**：r8/r9 已 defer 的是"successful join 路径
	// 留下 commit lock entry"（real roomID 累积，tech-debt 已记 lesson）；
	// 本 r10 修复的是"failed join 路径（room 不存在）也留 entry"的 attack
	// vector —— 二者同 family（map entry 不 reclaim），分层处理。
	//
	// **性能影响**：每 join 多 1 次普通 SELECT（ms 级；走 PRIMARY KEY 索引，无锁）
	// —— 节点 4 阶段 join 频率低（人类操作级），可接受。
	if _, err = s.roomRepo.FindByID(ctx, in.RoomID); err != nil {
		if stderrors.Is(err, mysql.ErrRoomNotFound) {
			return nil, apperror.New(apperror.ErrRoomNotFound, apperror.DefaultMessages[apperror.ErrRoomNotFound])
		}
		// 其他 DB 异常（连接 / 超时 / etc.）→ 1009，同样不进 lock 段
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// (2) acquire per-room commit lock + 开事务（数据库设计 §8.6 + V1 §10.4 钦定）
	//
	// **r6 [P1] 修复关键**：lock 必须在 txMgr.WithTx **之前**取得，且包住整个
	// commit + enqueueRoomEvent 段。这样 same-room 的 JoinRoom / LeaveRoom 不能
	// interleave commit —— commit 顺序 = enqueue 顺序 = client 感知顺序。
	//
	// **defer Unlock**：兜底 panic / 业务 err 早返回路径；defer 在 return 后触发，
	// 单次 HTTP 路径 lock 内只有 instant op（commit + channel send）→ HTTP 延迟
	// 增加 < 1ms。
	mu := s.acquireCommitLock(in.RoomID)
	mu.Lock()
	defer mu.Unlock()

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

	// (4) 事务 commit 成功 → fire-and-forget 触发 member.joined 广播（V1 §10.4
	// 步骤 8 钦定：事务**外**触发，broadcast 失败不影响 HTTP 200 响应）。
	//
	// **r2 修复**（codex review r2 [P2]）：post-commit hook 走独立 goroutine + detached
	// ctx + 10s timeout —— 让 broadcast 不被 request ctx cancel 误中断（否则
	// userRepo.FindByID / petRepo.FindDefaultByUserID 会 fail "context canceled"
	// → broadcast 静默 skip / payload 字段空），且不阻塞 HTTP 200 响应。详见
	// runPostCommitAsync 注释。
	//
	// **r3 修复**（codex review r3 [P1]）：HTTP join 200 → client 立即建 WS →
	// joiner Session 完成 SessionManager.Register → 此时异步 goroutine 才 fanout
	// → joiner Session 在 ListSessionsByRoomID 列表中 → joiner 收到自己的
	// member.joined（违反 V1 §12.3 行 2063 钦定）。修法：broadcastMemberJoined
	// 内部走 BroadcastToRoomExcept(joinerUserID) 显式 exclude joiner Session。
	//
	// **r4 修复**（codex review r4 [P1]）：runPostCommitAsync → runPostCommitAsyncPerRoom
	// 加 roomID 参数走 per-room mutex 串行化，保留 join → leave 的 causal ordering。
	//
	// **r5 修复**（codex review r5 [P1]）：runPostCommitAsyncPerRoom → enqueueRoomEvent。
	// 用 caller 同步段 channel send 替代 goroutine 内 mutex Lock —— mutex 方案在
	// goroutine 启动阶段仍受 scheduler race 影响（commit 顺序 join → leave 但 leave
	// goroutine 可能抢先 Lock 抢跑 broadcast）。channel queue 方案在 caller 同步
	// 段就完成 enqueue，scheduler 不能重排 enqueue 顺序，causal ordering 严格保留。
	//
	// **r6 修复**（codex review r6 [P1]）：r5 通过 channel FIFO 保 enqueue 顺序，
	// 但要求 "commit 后立刻 enqueue 期间无任何同步 op 可被 concurrent 路径夹塞"。
	// LeaveRoom r5 实装在 commit 后插入了同步段 unregisterLeaverSessionSync，再
	// enqueue —— gap 内 concurrent JoinRoom 可 commit + enqueue 抢先，破坏 commit
	// = enqueue order。修法：commit-time per-room mutex 把 (commit + enqueue)
	// 包成原子段；JoinRoom / LeaveRoom 同模式。详见 acquireCommitLock 注释。
	s.enqueueRoomEvent(ctx, in.RoomID, func(detachedCtx context.Context) {
		s.broadcastMemberJoined(detachedCtx, in.RoomID, in.UserID)
	})
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
// **post-commit 阶段（V1 §10.5 步骤 7 + 步骤 8 钦定，r13 顺序；r3 hybrid 切分；
// r7 lock-sync unregister 修订）**：commit 成功后**在 commit-time lock 内**按
// 顺序同步执行：(1) unregisterLeaverSessionSync 让 leaver 立即从 SessionManager
// 索引消失；(2) enqueueRoomEvent 入 per-room queue 排 broadcast member.left
// （worker 串行 fanout，与同 room JoinRoom 的 broadcastMemberJoined 保持
// commit-order = causal-order）；(3) runCloseLeaverAsync 启动独立 goroutine
// 跑 CloseWithCode（drain ~5s 慢路径，不阻塞 lock / HTTP）。
//
// **r3 hybrid 切分背景**：r2 整体异步化引入"HTTP leave 200 → leaver 仍在
// SessionManager → stale broadcast"regression（违反 §10.5 步骤 7 "HTTP leave
// immediately detaches WS"）。r3 把 Unregister 提回同步段（map 操作 O(1) 不阻塞
// HTTP），CloseWithCode 留异步段（~5s drain）。
//
// **r6 → r7 修订**：r6 把 unregister 移进 worker 闭包以避开 caller 同步段中的
// race，但 worker 是 FIFO 串行消费 —— 前序排队事件未跑完时 LeaveRoom HTTP 200
// 已返回，leaver 仍在 SessionManager → 期间 worker 处理 backlog 中 broadcast
// 时仍把 leaver 当 active member fanout，再次违反 detach 钦定。r7 把 unregister
// 移回 lock 段同步执行（commit 之后、enqueue 之前），同时保留 r6 的 commit-time
// lock 包 (commit + unregister + enqueue + close-spawn) → HTTP 200 前 leaver
// 已 detached + commit-order 仍由 lock 保证。
//
// **r3 BroadcastToRoomExcept**：broadcast 路径显式 exclude leaver UserID（V1 §12.3
// 行 2063 钦定 + belt-and-suspenders 防御 race）。
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

	// (2) acquire per-room commit lock + 开事务（数据库设计 §8.7 + V1 §10.5 钦定）
	//
	// **r6 [P1] 修复关键**：lock 必须在 txMgr.WithTx **之前**取得，且包住 commit +
	// enqueueRoomEvent 段（含 unregister 移进 worker 闭包后的状态）。同 JoinRoom，
	// 详见 acquireCommitLock 注释。
	mu := s.acquireCommitLock(in.RoomID)
	mu.Lock()
	defer mu.Unlock()

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

	// (4) 事务 commit 成功 → 按 V1 §10.5 r13 钦定顺序处理 post-commit。
	//
	// **r7 [P1] fix（unregister 移回 lock 段同步执行）**：
	//
	// r6 把 unregisterLeaverSessionSync 移进 worker 闭包以避开 caller 同步段中的
	// race，但引入了新 regression：worker 是 FIFO 串行消费，**前序排队事件**
	// （如同 room 之前 JoinRoom 的 broadcastMemberJoined）尚未跑完时，LeaveRoom
	// HTTP 200 已返回但 leaver 仍在 SessionManager —— 期间 worker 处理 backlog
	// 中那些 broadcast 时仍把 leaver 当 active member fanout，leaver 收到 stale
	// event。违反 V1 §10.5 步骤 7 "HTTP leave immediately detaches WS" 钦定。
	//
	// **r7 修法**（unregister 放进 lock 段同步段，commit 之后、enqueue 之前）：
	//
	//   1. caller 同步段 acquire per-roomID mutex（acquireCommitLock）；
	//   2. mutex 内 step1: txMgr.WithTx commit；
	//   3. mutex 内 step2: unregisterLeaverSessionSync 同步执行
	//      （SessionManager map op O(1) instant，nano 级；HTTP 200 之前 leaver
	//       已从 SessionManager 双索引清除）；
	//   4. mutex 内 step3: enqueueRoomEvent（broadcast member.left 入 queue；
	//      channel send instant op）；
	//   5. mutex 内 step4: runCloseLeaverAsync 启动独立 goroutine
	//      （go spawn instant op；CloseWithCode drain ~5s 慢路径在独立 goroutine
	//       内跑，不阻塞 lock）；
	//   6. mutex 解锁（defer 兜底）；HTTP 200 返回。
	//
	// **关键不变量**（lock 内每步都是 instant op）：
	//   - DB commit（毫秒级）
	//   - SessionManager.Unregister（map 操作 nano 级）
	//   - channel send（buffered 1024-cap，常态 nano 级；r9 [P2] 修订为 blocking
	//     send 替代 silent drop，满时阻塞但 instant-op 不变量在 1024-cap 未满路径
	//     成立 —— 极端 burst > 1024 同 room 才会触发阻塞，节点 4 阶段不会发生）
	//   - go spawn（runtime 创建 goroutine 是 nano 级；close drain 在 goroutine
	//     内 5s 但不计入 lock 持有时间）
	//
	// **r7 解决的两条 invariant**（同时成立）：
	//   (a) **HTTP 200 时 leaver 已 detached**：unregister 在 lock 内同步段，
	//       HTTP 返回前 leaver 必已从 SessionManager 移除 → 任何后续
	//       broadcast（含 worker backlog）调 ListSessionsByRoomID 都不会再
	//       命中 leaver。
	//   (b) **commit-order = enqueue-order = causal-order**（r6 钦定保留）：
	//       same-room 两个事务不能 interleave commit 与 enqueue —— per-room
	//       lock 包住 (commit + unregister + enqueue + close-spawn)。
	//
	// **r2 / r3 / r4 / r5 / r6 演进追溯**（保留供未来 review 追源）：
	//   - r2: 整体异步化 → 引入 R1/R2 regression
	//   - r3: hybrid sync/async + BroadcastToRoomExcept 修 R1/R2
	//   - r4: per-room mutex 试图保 causal ordering（goroutine 内 Lock）
	//   - r5: per-room channel queue（caller 同步段 enqueue），但 LeaveRoom 同
	//        步段 unregister 夹在 commit 与 enqueue 之间 → r6 finding
	//   - r6: commit-time mutex 包 (commit + enqueue)；unregister 进 worker 闭包
	//        → r7 finding（worker backlog 中 leaver 仍 active，违反 detach 钦定）
	//   - r7: unregister 移回 lock 段同步执行；lock 内全 instant op；HTTP 200
	//        前 leaver 已 detached + commit-order 仍由 lock 保证

	// (4a) **lock 段内**：先 unregister leaver Session（让 worker backlog 与后
	//      续 same-room broadcast 都看到 SessionManager 已无 leaver；HTTP 200
	//      返回前完成 detach 钦定）。O(1) map op，nano 级。
	target, _ := s.unregisterLeaverSessionSync(ctx, in.RoomID, in.UserID)

	// (4b) **lock 段内**：enqueue broadcast member.left。channel send + select
	//      default 兜底，instant op。worker 顺序消费保证 same-room 事件按
	//      commit 顺序广播（commit-order = enqueue-order = causal-order）。
	s.enqueueRoomEvent(ctx, in.RoomID, func(detachedCtx context.Context) {
		s.broadcastMemberLeft(detachedCtx, in.RoomID, in.UserID)
	})

	// (4c) **lock 段内**：fire-and-forget 启动 close goroutine。go spawn 是
	//      instant op（runtime 创建 goroutine 纳秒级），CloseWithCode drain
	//      ~5s 慢路径跑在独立 goroutine 内，不阻塞 lock / 不阻塞 HTTP / 不
	//      阻塞 worker。target 来自 (4a) 同步段返回的 leaver Session 引用。
	if target != nil {
		s.runCloseLeaverAsync(ctx, in.RoomID, in.UserID, target)
	}

	return &LeaveRoomOutput{
		RoomID: in.RoomID,
		Left:   true,
	}, nil
}

// runCloseLeaverAsync 在独立 goroutine 内调用 closeLeaverSessionAsync，与 per-room
// queue 解耦（Story 11.8 r5 [P2] 修复引入）。
//
// **为什么必须与 per-room queue 解耦**（r5 [P2] 核心）：CloseWithCode 内部 drain
// WS write loop ~5s（worst-case 更长），如果它跑在 per-room queue worker 里，会
// 阻塞该 room 后续**所有** broadcast 事件 —— 一个 stuck leaver 拖累整 room 的
// member.joined / member.left 几秒钟。拆出独立 goroutine 后，slow close 在自己
// 的 goroutine 慢慢走，queue worker 立刻处理下一条事件，其他成员的 roster 视图
// 即时更新。
//
// **WG 簿记**：本路径**不**走 postCommitWG —— close 是 best-effort cleanup，不影响
// broadcast 副作用断言；测试只需要 wg.Wait() 确保 broadcast 完成后断言 broadcast
// 内容。如未来 test 需要等 close 完成（如断言 sessionMgr.unregisterCalls 或 close
// frame 发出），加独立 closeWG 注入路径，**不**复用 postCommitWG（语义不同）。
//
// **fire-and-forget 严格语义**：goroutine 内自带 detached ctx + timeout 兜底；
// 任何 close 失败 log warn 不返。
func (s *roomServiceImpl) runCloseLeaverAsync(ctx context.Context, roomID, leaverUserID uint64, target *ws.Session) {
	go func() {
		detached := context.WithoutCancel(ctx)
		timedCtx, cancel := context.WithTimeout(detached, postCommitTimeout)
		defer cancel()
		s.closeLeaverSessionAsync(timedCtx, roomID, leaverUserID, target)
	}()
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

// ============================================================================
// Story 11.8 — post-commit WS 广播 / close 4007 unregister leaver session helpers
// ============================================================================

// broadcastMemberJoined 触发 member.joined WS 广播（Story 11.8 引入）。
//
// 流程（V1 §10.4 步骤 8 + §12.3 `### 成员加入` 字段表钦定）：
//  1. userRepo.FindByID(ctx, joinerUserID) 拿 nickname / avatar_url（事务外，
//     普通连接池查询）
//  2. petRepo.FindDefaultByUserID(ctx, joinerUserID) 拿默认宠物：
//     - ErrPetNotFound → pet-less 路径，pet=nil 构造 payload `pet: null`
//     - 其他 raw error → log warn + pet=nil 路径降级
//     - happy → pet=&ws.SnapshotPet{PetID: strconv.FormatUint(pet.ID, 10),
//     CurrentState: 1}（节点 4 阶段固定 1 rest，V1 §12.3 钦定）
//  3. 构造 ws.MemberJoinedPayload{UserID: 字符串化, Nickname: real, AvatarURL: real, Pet: <如上>}
//  4. 调 ws.BuildMemberJoinedEnvelope(payload) 拿 marshal 后 []byte
//  5. s.broadcastFn(ctx, roomID, msgBytes) 推送（fire-and-forget）
//
// **fire-and-forget 严格语义**（V1 §10.4 步骤 8 钦定）：本方法**永远不返 error** ——
// 任何步骤失败（DB / marshal / broadcast）一律 log warn 不返；caller (JoinRoom)
// 不需要走错误分流。原因：broadcast 失败不应影响 HTTP 200 响应（client 已通过
// HTTP 拿到 join 成功 authoritative signal，broadcast 是事件通知，不参与事务原子性）。
//
// **事务外严格性**：本方法在 JoinRoom 事务 commit 成功**之后**调用，**不**包入
// txMgr.WithTx fn 内 —— 与数据库设计 §8.6 加入房间事务边界 + V1 §10.4 步骤 8
// 钦定一致。
//
// **加入者收不到自己的 member.joined**（V1 §12.3 行 2063 钦定）：本路径调用
// `s.broadcastExceptFn(ctx, roomID, joinerUserID, msg)`（即包级 BroadcastToRoomExcept
// 经 closure 包装），fanout 时显式跳过 Session.UserID() == joinerUserID 的 Session。
//
// **r3 [P1] fix 引入显式 exclude 的原因**：r2 把 broadcastMemberJoined 整体放入
// 异步 post-commit goroutine 后，HTTP join 200 → client 立即建 WS → joiner Session
// 完成 SessionManager.Register → 此时异步 goroutine 才开始 ListSessionsByRoomID
// → 列表含 joiner 自己 → joiner 收到自己的 member.joined（违反 V1 §12.3 行 2063）。
// r1 同步路径下"broadcast 入队时加入者 WS 还没握手"的隐含 race-free 假设在 r2
// 之后不再成立 → r3 用 BroadcastToRoomExcept 显式 exclude 修复。
func (s *roomServiceImpl) broadcastMemberJoined(ctx context.Context, roomID, joinerUserID uint64) {
	logger := slog.Default().With(
		slog.String("component", "room-service-broadcast"),
		slog.String("event", "member.joined"),
		slog.Uint64("roomId", roomID),
		slog.Uint64("joinerUserId", joinerUserID),
	)

	// (1) 查 joiner user 信息
	user, err := s.userRepo.FindByID(ctx, joinerUserID)
	if err != nil {
		logger.Warn("ws broadcast: load joiner user failed; skip broadcast",
			slog.Any("error", err))
		return
	}

	// (2) 查 joiner 默认宠物
	var pet *ws.SnapshotPet
	petRow, err := s.petRepo.FindDefaultByUserID(ctx, joinerUserID)
	if err != nil {
		if !stderrors.Is(err, mysql.ErrPetNotFound) {
			// 非 pet-less 兜底；log warn 后 pet=nil 降级（不阻塞 broadcast）
			logger.Warn("ws broadcast: load joiner default pet failed; pet-less downgrade",
				slog.Any("error", err))
		}
		// ErrPetNotFound 走 pet=nil 合法路径，不 log warn（pet-less 是合法场景）
	} else {
		pet = &ws.SnapshotPet{
			PetID:        strconv.FormatUint(petRow.ID, 10),
			CurrentState: 1, // V1 §12.3 节点 4 阶段固定 1 rest
		}
	}

	// (3) 构造 payload
	payload := ws.MemberJoinedPayload{
		UserID:    strconv.FormatUint(joinerUserID, 10),
		Nickname:  user.Nickname,
		AvatarURL: user.AvatarURL,
		Pet:       pet,
	}

	// (4) marshal envelope
	msgBytes, err := ws.BuildMemberJoinedEnvelope(payload)
	if err != nil {
		logger.Warn("ws broadcast: marshal envelope failed; skip broadcast",
			slog.Any("error", err))
		return
	}

	// (5) fire-and-forget broadcast；r3 [P1] fix：用 BroadcastToRoomExcept 显式
	// exclude joiner UserID，防御 joiner 在 post-commit 异步段已完成 WS Register
	// 的 race（HTTP join 200 → client 立即建 WS → joiner Session 进入
	// SessionManager → 异步 goroutine 此时才开始 fanout 会包含 joiner 自己）。
	sent, err := s.broadcastExceptFn(ctx, roomID, joinerUserID, msgBytes)
	if err != nil {
		logger.Warn("ws broadcast: broadcastExceptFn failed",
			slog.Int("targetSessions", sent),
			slog.Any("error", err))
		return
	}
	logger.Info("ws broadcast: member.joined sent",
		slog.Int("targetSessions", sent))
}

// unregisterLeaverSessionSync 实装 V1 §10.5 步骤 7 的**同步部分**（Story 11.8 r3
// [P1] fix 引入 hybrid sync/async 切分）：
//
// 从 SessionManager 找 leaver 在该 roomID 的 Session + 立即 Unregister（清空双索引）。
// **不**调 Session.CloseWithCode —— close 走 closeLeaverSessionAsync 异步段处理。
//
// **为什么必须同步执行**（r3 [P1] R2 fix）：r2 把整个 closeLeaverSession 放进
// post-commit goroutine 后引入 regression：HTTP leave 200 → leaver Session 仍
// 在 SessionManager → 期间任何 broadcast（如另一 user 的 join）仍 fanout 给
// stale leaver session，违反 V1 §10.5 步骤 7 "HTTP leave immediately detaches
// WS" 语义。
//
// **修复策略**：把 Unregister（map 操作 O(1) 瞬时完成，不涉及 IO）放回 LeaveRoom
// 同步段 —— HTTP 200 返回前 leaver Session 已从 SessionManager 索引消失，任何
// 后续 broadcast 不会再 fanout 给 leaver。CloseWithCode（drain write loop ~5s
// 慢路径）仍走异步 goroutine，不阻塞 HTTP 200。
//
// 流程：
//  1. s.sessionMgr.ListSessionsByRoomID(ctx, roomID) 拿该 roomID 全部 active Session
//  2. 线性扫描找到 Session.UserID() == leaverUserID 的 Session（节点 4 阶段单 room
//     最多 4 user，O(N) 可接受）
//  3. 命中 → s.sessionMgr.Unregister(ctx, sessionID)；返回 (target, true) 让
//     caller 调用方接力发起 CloseWithCode 异步段
//  4. 未命中（leaver 未持该 roomID 的 WS / 已断开）→ 返回 (nil, false)，async 段
//     无需启动
//
// **nil sessionMgr guard**：HTTP-only / test wiring 场景可能不注入 sessionMgr ——
// 此时直接返 (nil, false) 不 panic（fire-and-forget 严格语义）。
//
// **fire-and-forget 严格语义**（V1 §10.5 步骤 7 钦定）：本方法**永远不返 error**——
// Unregister 的 error 一律 log warn 不返。
//
// 返回：
//   - target: 命中的 leaver Session（caller 用于 closeLeaverSessionAsync 调用）；
//     未命中或 nil sessionMgr → nil
//   - found: 是否命中并已成功 Unregister
func (s *roomServiceImpl) unregisterLeaverSessionSync(ctx context.Context, roomID, leaverUserID uint64) (target *ws.Session, found bool) {
	if s.sessionMgr == nil {
		return nil, false
	}

	logger := slog.Default().With(
		slog.String("component", "room-service-broadcast"),
		slog.String("event", "close.4007.sync"),
		slog.Uint64("roomId", roomID),
		slog.Uint64("leaverUserId", leaverUserID),
	)

	sessions := s.sessionMgr.ListSessionsByRoomID(ctx, roomID)
	for _, sess := range sessions {
		if sess.UserID() == leaverUserID {
			target = sess
			break
		}
	}
	if target == nil {
		logger.Info("ws close 4007 sync: leaver session not registered; skip (合法场景)")
		return nil, false
	}

	sessionID := target.SessionID()
	// **关键同步段**：Unregister 立即从 SessionManager 双索引清除 leaver Session ——
	// 后续任何 broadcast 调 ListSessionsByRoomID 都不会再返 leaver Session，
	// "HTTP leave immediately detaches WS" 语义达成。
	if err := s.sessionMgr.Unregister(ctx, sessionID); err != nil {
		logger.Warn("ws close 4007 sync: Unregister returned error",
			slog.String("sessionId", sessionID),
			slog.Any("error", err))
		// Unregister 失败仍返回 target + found=true 让异步段尝试 CloseWithCode
		// 兜底（idempotent，二次调用 no-op 无副作用）
	} else {
		logger.Info("ws close 4007 sync: leaver session unregistered",
			slog.String("sessionId", sessionID))
	}
	return target, true
}

// closeLeaverSessionAsync 实装 V1 §10.5 步骤 7 的**异步部分**（Story 11.8 r3 [P1]
// fix 引入 hybrid sync/async 切分）：
//
// 调用 Session.CloseWithCode(4007, "left room via HTTP") 写 close frame + drain
// write loop。
//
// **为什么必须异步执行**（r2 [P1] fix 保留）：Session.CloseWithCode 内部走
// notifyClosed → write loop drain，最坏耗时 ~5s WS write timeout —— 同步调用会
// 让 LeaveRoom HTTP 响应延迟 ~5s，违反 fire-and-forget 语义。
//
// **idempotent**：CloseWithCode 内部走 notifyClosed → SessionManager.Unregister
// 自动闭环，但 caller 已在 unregisterLeaverSessionSync 同步段先 Unregister；
// CloseWithCode 路径触发的 Unregister 二次调用 no-op（map delete missing key
// 是合法 no-op，10.3 钦定的 idempotent 语义）。
//
// **fire-and-forget 严格语义**：本方法**永远不返 error** —— CloseWithCode 的
// error 一律 log warn 不返。
//
// 参数：
//   - target: 由 unregisterLeaverSessionSync 同步段返回的 leaver Session 引用；
//     不应为 nil（caller 在 found==true 时才调用本方法）
func (s *roomServiceImpl) closeLeaverSessionAsync(ctx context.Context, roomID, leaverUserID uint64, target *ws.Session) {
	if target == nil {
		return
	}

	logger := slog.Default().With(
		slog.String("component", "room-service-broadcast"),
		slog.String("event", "close.4007.async"),
		slog.Uint64("roomId", roomID),
		slog.Uint64("leaverUserId", leaverUserID),
		slog.String("sessionId", target.SessionID()),
	)

	if err := target.CloseWithCode(4007, "left room via HTTP"); err != nil {
		// ErrSessionClosed (target 已被并发 close) / 其他 raw error → log warn 不返
		logger.Warn("ws close 4007 async: CloseWithCode returned error",
			slog.Any("error", err))
		return
	}
	logger.Info("ws close 4007 async: leaver session close frame written")
}

// broadcastMemberLeft 触发 member.left WS 广播（Story 11.8 引入）。
//
// 流程（V1 §10.5 步骤 8 + §12.3 `### 成员离开` 字段表钦定）：
//  1. 构造 ws.MemberLeftPayload{UserID: 字符串化}（V1 §12.3 行 2073-2080 字段表
//     钦定仅 1 字段 userId）
//  2. 调 ws.BuildMemberLeftEnvelope(payload) 拿 marshal 后 []byte
//  3. s.broadcastFn(ctx, roomID, msgBytes) 推送（fire-and-forget）
//
// **与 broadcastMemberJoined 的差异**：member.left payload 不需要查 user / pet
// 信息（V1 §12.3 行 2097 钦定 leave 事件 client UX 不需要显示昵称 + pet 信息），
// 直接拿 leaverUserID 字符串化即可，**不**走 userRepo.FindByID / petRepo.FindDefaultByUserID。
//
// **fire-and-forget 严格语义**（V1 §10.5 步骤 8 钦定）：本方法**永远不返 error** ——
// 任何步骤失败（marshal / broadcast）一律 log warn 不返。
//
// **事务外严格性 + r13 顺序约束**：本方法在 LeaveRoom 事务 commit 成功之后调用，
// **必须**在 closeLeaverSession Unregister 步骤之**后**调用 —— 让 broadcast fanout
// 时 leaver Session 已被 Unregister 不在 ListSessionsByRoomID 返回列表中。
//
// **r3 [P1] fix 双保险**：除 closeLeaverSession 同步 Unregister 已让 leaver 从
// 列表消失之外，本方法**亦**调用 BroadcastToRoomExcept 显式 exclude leaver UserID
// （belt-and-suspenders）—— 即使未来某条 race 路径让 leaver 短暂回到 ListSessionsByRoomID
// 列表，本路径也能拦截，绝不下发 stale member.left 给 leaver 自己（V1 §12.3 钦定
// "广播范围：仅该房间内当前在线的其他 Session"语义）。
//
// **特例**：若 LeaveRoom 步骤 5 触发了 closed 转换（最后一人离开 → rooms.status=2），
// 房间内已无其他在线广播对象 —— broadcast 路径仍调用 broadcastExceptFn，但 fanout
// 时房间内已无其他在线 Session，broadcastExceptFn 自然 no-op（详见 ws/broadcast.go：
// 0 个 active Session → 返 (0, nil)）。
func (s *roomServiceImpl) broadcastMemberLeft(ctx context.Context, roomID, leaverUserID uint64) {
	logger := slog.Default().With(
		slog.String("component", "room-service-broadcast"),
		slog.String("event", "member.left"),
		slog.Uint64("roomId", roomID),
		slog.Uint64("leaverUserId", leaverUserID),
	)

	// (1) 构造 payload + (2) marshal envelope
	payload := ws.MemberLeftPayload{
		UserID: strconv.FormatUint(leaverUserID, 10),
	}
	msgBytes, err := ws.BuildMemberLeftEnvelope(payload)
	if err != nil {
		logger.Warn("ws broadcast: marshal envelope failed; skip broadcast",
			slog.Any("error", err))
		return
	}

	// (3) fire-and-forget broadcast；r3 [P1] fix：用 BroadcastToRoomExcept 显式
	// exclude leaver UserID（双保险 —— closeLeaverSession 同步 Unregister 已让
	// leaver 从 ListSessionsByRoomID 列表消失，本路径再 belt-and-suspenders 防御
	// 任何潜在 race 让 stale member.left 误发给 leaver）。
	sent, err := s.broadcastExceptFn(ctx, roomID, leaverUserID, msgBytes)
	if err != nil {
		logger.Warn("ws broadcast: broadcastExceptFn failed",
			slog.Int("targetSessions", sent),
			slog.Any("error", err))
		return
	}
	logger.Info("ws broadcast: member.left sent",
		slog.Int("targetSessions", sent))
}
