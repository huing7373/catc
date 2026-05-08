package mysql

import (
	"context"
	stderrors "errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/repo/tx"
)

// RoomMember 是 room_members 表的完整 GORM domain struct。
//
// **演进**：
//   - Story 10.3 引入最小骨架（仅 RoomID / UserID + (room_id, user_id) 复合 PK 标注），
//     当时 0008 migration 还没落地，struct 仅为 WS 握手期 user-in-room 校验 +
//     placeholder snapshot 全成员查询提供最小字段映射；
//   - Story 11.2 升级为与 0008 真实 schema 1:1 对齐的完整字段集合
//     （id / room_id / user_id / joined_at / updated_at），让下游 Story 11.3 / 11.4 /
//     11.5 / 11.6 的事务 INSERT / DELETE 路径 + ORDER BY joined_at 排序 + RowsAffected
//     兜底语义都能直接复用本 struct 的 GORM 字段映射。
//
// 字段（与 server/migrations/0008_init_room_members.up.sql 真实 schema 1:1 对齐）：
//   - ID:        BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§5.14 + §3.1 主键约定）
//   - RoomID:    BIGINT UNSIGNED NOT NULL（归属 rooms.id）
//   - UserID:    BIGINT UNSIGNED NOT NULL（归属 users.id）
//   - JoinedAt:  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
//   - UpdatedAt: DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
//
// 表层 UNIQUE 约束（uk_user_id / uk_room_user）+ 普通索引（idx_room_id）由 SQL DDL
// 定义，**不**在 struct tag 中重复声明（与 ADR-0003 §3.2 "禁止 GORM AutoMigrate"
// 同源；GORM struct 仅为 Find / Create 提供字段映射，**不**作为 schema 真相源）。
type RoomMember struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	RoomID    uint64    `gorm:"column:room_id;not null"`
	UserID    uint64    `gorm:"column:user_id;not null"`
	JoinedAt  time.Time `gorm:"column:joined_at;not null;default:CURRENT_TIMESTAMP(3)"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

// TableName 显式声明 "room_members"。
func (RoomMember) TableName() string { return "room_members" }

// Room 是 rooms 表的完整 GORM domain struct（Story 11.2 引入；
// 与 server/migrations/0007_init_rooms.up.sql 真实 schema 1:1 对齐）。
//
// 字段（与数据库设计.md §5.13 + 0007_init_rooms.up.sql 1:1 对齐）：
//   - ID:            BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT
//   - CreatorUserID: BIGINT UNSIGNED NOT NULL（创建者 user.id）
//   - Status:        TINYINT NOT NULL DEFAULT 1（数据库设计.md §6.12 钦定 1=active / 2=closed）
//   - MaxMembers:    TINYINT UNSIGNED NOT NULL DEFAULT 4（节点 4 阶段固定 4）
//   - CreatedAt / UpdatedAt: DATETIME(3)（毫秒精度时间戳）
//
// 字段类型选择：
//   - Status int8（带符号 TINYINT，范围 -128~127；status 枚举 1 / 2 在范围内）
//   - MaxMembers uint8（无符号 TINYINT UNSIGNED，范围 0~255；房间容量 4 在范围内）
//
// **范围红线**（Story 11.2 钦定）：本 struct 仅为下游 Story 11.3（创建房间事务）/
// 11.6（房间详情查询）提供字段映射；本 story 阶段**不**新建 RoomRepo interface /
// 实装 Create / Update / FindByID 等方法（YAGNI；Story 11.3 / 11.6 才落地）。
// 也**不**引入 Pets / Members 等关联字段 / preload tag（Story 11.6 落地 GET
// /rooms/{roomId} 时再加）。
type Room struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	CreatorUserID uint64    `gorm:"column:creator_user_id;not null"`
	Status        int8      `gorm:"column:status;not null;default:1"`
	MaxMembers    uint8     `gorm:"column:max_members;not null;default:4"`
	CreatedAt     time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
	UpdatedAt     time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

// TableName 显式声明 "rooms"。
func (Room) TableName() string { return "rooms" }

// RosterRow 是 RoomMemberRepo.ListRosterByRoomID 的返回元素（Story 11.6 引入）。
//
// 字段（与 V1 §10.3 钦定 wire DTO `data.members[].{userId, nickname, avatarUrl, pet.petId}` 1:1 对齐）：
//   - UserID:    BIGINT UNSIGNED → uint64（来自 room_members.user_id）
//   - Nickname:  VARCHAR(64) → string（来自 users.nickname；INNER JOIN users）
//   - AvatarURL: VARCHAR(255) → string（来自 users.avatar_url；INNER JOIN users）
//   - PetID:     BIGINT UNSIGNED NULL → *uint64（来自 pets.id；LEFT JOIN pets，
//     pet-less 时该列为 NULL，必须用 *uint64 接 NULL）
//
// **关键**：用 *uint64 而非 uint64 因 LEFT JOIN pets 在 pet-less 时该列为 NULL，
// GORM Scan 会把 NULL 映射为 zero-value（0）→ service 层无法区分"pet.id == 0"
// 与"pet-less"；用 pointer 让 NULL → nil pointer，service 层据此把 wire DTO 的
// `pet` 整体下发为 `null`（V1 §10.3 字段表 nullable 钦定）。
//
// **不**包含 pet.currentState / pet.equips：节点 4 阶段固定 `1` / `[]`（V1 §10.3
// 字段表节点 4 列钦定），由 service 层硬编码；query 层不查 pets.current_state /
// user_pet_equips 表（YAGNI，节点 5 / 9 由 Epic 14 / 26 真实驱动时再扩展）。
//
// **不**复用 mysql.Pet struct：RosterRow 仅含本 story 需要的 4 个字段，避免
// 拉过多 pets 字段污染 query payload（pet.created_at / pet_type / name / is_default
// 等都不需要）。future 若需要扩展更多字段（如 currentState 由 Epic 14 真实驱动），
// 在本 struct 内加字段而非新建 RosterRowExt。
type RosterRow struct {
	UserID    uint64  `gorm:"column:user_id"`
	Nickname  string  `gorm:"column:nickname"`
	AvatarURL string  `gorm:"column:avatar_url"`
	PetID     *uint64 `gorm:"column:pet_id"` // LEFT JOIN pets，pet-less 时为 nil
}

// RoomMemberRepo 是 room_members 表的最小读取接口（Story 10.3 引入）。
//
// 范围边界：
//   - 仅含 WS 握手期 user 在 room 校验 + snapshot 全成员查询所需的 3 个方法
//   - **不**含写操作（加入 / 退出房间事务由 Epic 11.4 / 11.5 才落地）
//   - rooms / room_members migration 由 Epic 11.2 才落地；本 story 阶段集成测试
//     用 setup 临时建表（ws_integration_test.go startMySQLWithRoomMemberFixture
//     helper 内 CREATE TABLE）
//
// 设计选择：
//   - 用 interface（不是 struct）让 gateway 单元测试可注入 mock 实装
//   - 三方法都接 ctx：与 4.6 / 7.3 既有 repo ctx-aware 模式一致（ADR-0007）
//   - 三方法返 (bool, error) / ([], error)；**不**返 sentinel ErrXxxNotFound
//     —— 与 step_account_repo.FindByUserID 不同，因为 IsUserInRoom 的"不在
//     房间"是合法业务态（不是数据脏），不需要 errors.Is 判定路径
type RoomMemberRepo interface {
	// RoomExists 校验 rooms 表中存在该 roomID **且 status = 1 (active / open)**。
	// 不存在 / closed → (false, nil)；查询失败 → (false, err)。
	//
	// **关键语义**（review r4 P2 修 + r7 P2 加 status 过滤）：必须查 rooms 表
	// **不是** room_members 表，且必须过滤 closed 房间。Gateway.Handle 用本方法
	// 决定 close 4004 (room not found) 路径；如果不过滤 status，archived rooms
	// （status=closed 但 stale room_members 残留）仍被判定 "存在" 而错误 accept
	// WS 连接 + 发 room.snapshot，违反 Story 10.3 AC2 钦定的 close-on-room-missing
	// 协议路径。
	//
	// **status 枚举**（数据库设计.md §6.12 / 0007_init_rooms migration §17 注释钦定）：
	//   - 1 = active / open（业务可加入 / 接受 WS 连接）
	//   - 2 = closed（不可加入 / WS 拒绝；prod 由 Epic 11.6 close 事务转换）
	// 用 `WHERE status = 1` 而非 `WHERE status != 2` 让语义更显式（"只要 open
	// 房间"），未来若枚举扩展（如 3=archived / 4=banned）默认排除新状态更安全。
	//
	// **集成测试 fixture 兼容**：ws_integration_test.go startMySQLWithRoomMemberFixture
	// helper 已在 r7 同步从 VARCHAR(16) 'active' 改为 TINYINT 1（与 prod 0007
	// migration schema 一致），让 raw query `WHERE status = 1` 在两端等价。
	//
	// 与之前 placeholder 行为差异：之前查 room_members → 空房间被视为不存在；
	// 现在查 rooms (status=1) → 空 active 房间也算存在（与 §设计说明 "只要还有
	// 成员保持 active；没人后状态置 closed" 一致；本 story 阶段不区分 active 空
	// 房间）；closed 房间无论 room_members 是否残留都返 false。
	RoomExists(ctx context.Context, roomID uint64) (bool, error)

	// IsUserInRoom 校验 room_members 表中存在 (user_id, room_id) 关联。
	// 不存在 → (false, nil)；查询失败 → (false, err)。
	// 用 SELECT 1 FROM room_members WHERE user_id=? AND room_id=? LIMIT 1
	IsUserInRoom(ctx context.Context, userID uint64, roomID uint64) (bool, error)

	// ListMembers 返回 roomID 下所有 room_members 行（仅 user_id 字段，本 story
	// 阶段 placeholder snapshot 只需 user_id 即可填 members[].userId）。
	// 0 行 → ([], nil)；查询失败 → (nil, err)。
	// 用 SELECT user_id FROM room_members WHERE room_id=? ORDER BY user_id ASC（
	// 排序让 placeholder snapshot 输出确定，便于集成测试 assert）
	ListMembers(ctx context.Context, roomID uint64) ([]uint64, error)

	// Create 插入一行 room_members（Story 11.3 引入；写方法）。
	//
	// GORM 在成功后回填 m.ID（AUTO_INCREMENT）；必须传 *RoomMember 指针。
	//
	// **ER_DUP_ENTRY 1062 双路径翻译**：room_members 表有两个唯一约束（0008 schema）：
	//   - uk_user_id (user_id)         → ErrRoomMembersUserIDDuplicate
	//   - uk_room_user (room_id, user_id) → ErrRoomMembersRoomUserDuplicate
	//
	// service 层用 errors.Is 识别后翻译为 6003 ErrUserAlreadyInRoom；两个独立哨兵
	// 让 service 层日志能区分哪个约束被打破（便于审计 / debug），即使从 client 视角
	// 6003 语义等价。
	//
	// 由 Story 11.3 (POST /rooms) 和 Story 11.4 (POST /rooms/{roomId}/join) 共同消费。
	Create(ctx context.Context, m *RoomMember) error

	// CountByRoomID 返回 roomID 下当前 room_members 行数（Story 11.4 引入；读方法）。
	//
	// 用于 V1 §10.4 步骤 4 容量校验（< 4 → 可加入，>= 4 → 6002 房间已满）。
	//
	// **必须在事务内调用**（与同事务 SELECT rooms ... FOR UPDATE 步骤一起执行）；事务外
	// 调用 race 窗口仍存在 —— 步骤序列必须是：
	//   1. SELECT rooms FOR UPDATE（锁住 rooms 行）
	//   2. CountByRoomID（受锁保护，并发 join / leave wait）
	//   3. INSERT room_members（容量已校验，安全）
	// 这是 V1 §10.4 服务端逻辑步骤 2-4-5 钦定的串行化路径；缺步骤 1 锁则 step 2 / 3
	// 之间存在并发 race（5 用户同时 join 满员房间 → 5 个都看到 count == 3 → 5 个
	// 同时 INSERT，超员 → 4 个被 UNIQUE(user_id) 兜底，但仍多 1 行）。
	//
	// query 失败 → 返 raw error 透传（service 包 1009）。
	CountByRoomID(ctx context.Context, roomID uint64) (int, error)

	// DeleteByRoomAndUser 按 (room_id, user_id) 双键定位删除单行 room_members
	// （Story 11.5 引入；用于 V1 §10.5 步骤 3 leave 事务删除 leaver 自己的成员行）。
	//
	// **必须在事务内调用**（与同事务 SELECT rooms ... FOR UPDATE 步骤一起执行）；
	// 事务外调用违反 V1 §10.5 步骤 2-3 钦定的串行化路径 —— FOR UPDATE 锁立即释放，
	// 跨事务 leave-vs-join race 重新出现。
	//
	// 返回 RowsAffected 让 service 层做 6004 兜底分流：
	//   - == 1：删除成功（happy path）
	//   - == 0：同一 user 并发两次 leave 输家场景（赢家已先删该行）→ service 层翻译为 6004
	//   - >  1：理论不可能（room_members 有 UNIQUE(room_id, user_id)，最多 1 行匹配）
	//
	// query 失败 → 返 (0, raw error) 透传给 service（service 包成 1009）。
	DeleteByRoomAndUser(ctx context.Context, roomID, userID uint64) (int64, error)

	// ExistsForShareByRoomAndUser 取 FOR SHARE 共享锁锁定 (room_id, user_id) 双键
	// 定位的单行 room_members（Story 11.6 引入；用于 V1 §10.3 步骤 1b ACL 共享锁）。
	//
	// **必须在事务内调用**（与同事务 userRepo.FindByID 步骤 1a + roomRepo.FindByID
	// 步骤 2 + ListRosterByRoomID 步骤 3 一起执行）；事务外调用 FOR SHARE 锁立即
	// 释放（autocommit 模式下任何 SELECT 完成即 commit），违反 V1 §10.3 + 数据库
	// 设计 §8.8 钦定的串行化路径 —— 跨事务并发 leave 的 DELETE 不再被阻塞，
	// post-leave 数据泄漏 race 重新出现。
	//
	// FOR SHARE vs FOR UPDATE：
	//   - FOR SHARE 取共享锁（S 锁），多个读事务可同时持锁，但与排他锁（X 锁，
	//     leave 事务的 DELETE 取）互斥；本 story 多个 GET /rooms/{roomId} 同时进
	//     行可彼此并行，仅与并发 leave 事务串行化
	//   - FOR UPDATE 取排他锁（X 锁），与所有锁互斥，性能更差；本 story 是只读
	//     接口，用 FOR SHARE 已足够（snapshot 隔离 + FOR SHARE 锁双机制提供
	//     ACL 边界保护，详见 V1 §10.3 rationale + 数据库设计 §8.8）
	//
	// 用 GORM Raw("SELECT 1 FROM room_members WHERE room_id = ? AND user_id = ? LIMIT 1 FOR SHARE") +
	// Scan(&dummy) 路径生成 SELECT ... FOR SHARE SQL；走 idx_room_id 索引 +
	// uk_room_user 唯一约束高效定位（最多 1 行匹配）。**MySQL 8.0 钦定 LIMIT 必须在
	// locking clause 之前**（FOR SHARE LIMIT 1 是 ER_PARSE_ERROR 1064；详见 dev review
	// r0 sub-loop fix）。
	//
	// 返：
	//   - 命中 1 行 → (true, nil)
	//   - 0 行 → (false, nil) —— 与 RoomExists / IsUserInRoom 同模式，Raw + Scan
	//     在 0 行时不返 ErrRecordNotFound 而是保持 dummy zero-value
	//   - query 失败 → (false, raw error 透传给 service（service 包成 1009）)
	ExistsForShareByRoomAndUser(ctx context.Context, roomID, userID uint64) (bool, error)

	// ListRosterByRoomID 查 room_members + INNER JOIN users + LEFT JOIN pets 聚合的
	// roster，按 room_members.joined_at ASC 稳定排序（Story 11.6 引入；用于 V1 §10.3
	// 步骤 3 GET /rooms/{roomId} 房间详情查询）。
	//
	// **必须在事务内调用**（与同事务 ExistsForShareByRoomAndUser 步骤 1b + roomRepo.FindByID
	// 步骤 2 一起执行）；事务外调用违反 V1 §10.3 + 数据库设计 §8.8 钦定的"读快照事务
	// （含 ACL 共享锁）"路径 —— 步骤 1 ~ 3 必须共享同一 REPEATABLE READ snapshot 才
	// 能保证内部一致性（两次 SELECT 看到同一时刻状态）+ FOR SHARE 锁才能阻止并发
	// leave 的 DELETE 在本读事务期间提交（外部一致性）。
	//
	// **LEFT JOIN pets 必须**（不能 INNER JOIN）：pet-less 账号（用户无 is_default=1
	// 的活跃 pet 行）若用 INNER JOIN 会被静默丢行 → 违反 memberCount === members.length
	// 不变量；LEFT JOIN 时 pets.* 列为 NULL，GORM Scan 把 NULL 映射为 *uint64 nil
	// （RosterRow.PetID）让 service 层据此下发 wire DTO `pet: null`。
	//
	// INNER JOIN users（不是 LEFT JOIN）：users 行必存在（room_members.user_id
	// FOREIGN KEY 引用 users.id；orphan 不可能）；INNER JOIN 比 LEFT JOIN 性能略
	// 好且语义明确"members[].nickname / avatarUrl 必非空 string（可空字符串 ""）"。
	//
	// pets.is_default = 1 过滤：每个用户最多一只默认 pet（uk_user_default_pet 唯一
	// 约束，§5.3）；本接口节点 4 阶段仅返回默认 pet（节点 9 / 10 由 Epic 26 / 29
	// 扩展多 pet 时再调整 query —— future 工作）。
	//
	// ORDER BY room_members.joined_at ASC：稳定排序，便于 client 渲染顺序稳定 +
	// 集成测试 assert（V1 §10.3 行 1374 钦定 server 决定顺序，建议按 joined_at ASC）。
	//
	// 返：
	//   - 0 行（房间存在但无成员，理论 closed 房间路径） → ([]RosterRow{}, nil)
	//   - 多行 → ([]RosterRow{...}, nil)
	//   - DB error → (nil, raw error 透传给 service（service 包成 1009）)
	ListRosterByRoomID(ctx context.Context, roomID uint64) ([]RosterRow, error)
}

// roomMemberRepo 是 RoomMemberRepo 的默认实装。
type roomMemberRepo struct {
	db *gorm.DB
}

// NewRoomMemberRepo 构造 RoomMemberRepo。Story 10.3 引入。
func NewRoomMemberRepo(db *gorm.DB) RoomMemberRepo {
	return &roomMemberRepo{db: db}
}

// RoomExists 实装（review r4 P2 改查 rooms 表；r7 P2 加 status = 1 过滤）：查
// `rooms` 表**不**查 `room_members`，并明确 `status = 1`（active / open），避免
// archived / closed rooms（status=2）残留 stale memberships 时误判 "存在" 而让
// Gateway 错误 accept WS 连接。
//
// status 枚举源：数据库设计.md §6.12 / migrations/0007_init_rooms.up.sql §17 注释
// 钦定（1=active/open, 2=closed）。
//
// 用 raw query + LIMIT 1 让 GORM 不做 SELECT * 浪费 IO。返 NotFound → (false, nil)
// 转译。
func (r *roomMemberRepo) RoomExists(ctx context.Context, roomID uint64) (bool, error) {
	db := tx.FromContext(ctx, r.db)
	var dummy int
	err := db.WithContext(ctx).
		Raw("SELECT 1 FROM rooms WHERE id = ? AND status = 1 LIMIT 1", roomID).
		Scan(&dummy).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	// Raw + Scan 在 0 行时**不**返 ErrRecordNotFound（GORM 行为），而是返 nil + dummy
	// 保持 zero-value（0）；用 dummy 是否被赋值判断
	return dummy == 1, nil
}

// IsUserInRoom 走 idx_user_room secondary 索引（PK (room_id, user_id) 已含
// (user_id, room_id) 的反向索引覆盖；不需要额外 idx）。
func (r *roomMemberRepo) IsUserInRoom(ctx context.Context, userID uint64, roomID uint64) (bool, error) {
	db := tx.FromContext(ctx, r.db)
	var dummy int
	err := db.WithContext(ctx).
		Raw("SELECT 1 FROM room_members WHERE user_id = ? AND room_id = ? LIMIT 1", userID, roomID).
		Scan(&dummy).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return dummy == 1, nil
}

// ListMembers 返回 roomID 下全部 user_id（按 user_id ASC 排序，让 placeholder
// snapshot 输出确定，便于集成测试 assert）。0 行 → ([], nil)。
func (r *roomMemberRepo) ListMembers(ctx context.Context, roomID uint64) ([]uint64, error) {
	db := tx.FromContext(ctx, r.db)
	var ids []uint64
	err := db.WithContext(ctx).
		Raw("SELECT user_id FROM room_members WHERE room_id = ? ORDER BY user_id ASC", roomID).
		Scan(&ids).Error
	if err != nil {
		return nil, err
	}
	if ids == nil {
		// GORM Scan 在 0 行时可能保持 nil；统一返 [] 让调用方不需要 nil-check
		return []uint64{}, nil
	}
	return ids, nil
}

// Create 插入一行 room_members；GORM 成功后回填 m.ID（AUTO_INCREMENT）。
//
// **ER_DUP_ENTRY 1062 双路径翻译**：room_members 表有两个唯一约束（0008 schema）：
//   - uk_user_id (user_id)         → ErrRoomMembersUserIDDuplicate
//   - uk_room_user (room_id, user_id) → ErrRoomMembersRoomUserDuplicate
//
// 不解析 mysql.MySQLError.Message 字符串的引号 / locale 部分（"Duplicate entry
// '...' for key '...'"）—— 字符串国际化 / 版本不可靠。用 Message contains
// "uk_user_id" / "uk_room_user" substring 是稳定 contract（key 名 part 在所有版本
// + 语言下都是英文 ASCII，与 ER_DUP_ENTRY 1062 错误号配套；与 user_repo / auth_binding_repo
// 同模式但需要双路径分流，故走 substring 而非简单"任何 1062 → 单一哨兵"）。
//
// **fallback 路径**：1062 但 Message 既不含 uk_user_id 也不含 uk_room_user → raw error
// 透传给 service（service 包成 1009）。节点 4 阶段两个 UNIQUE 约束已穷举，本 fallback
// 路径理论不触发；future 给 room_members 加新唯一索引时本函数注释要补按 key 名分流。
func (r *roomMemberRepo) Create(ctx context.Context, m *RoomMember) error {
	db := tx.FromContext(ctx, r.db)
	err := db.WithContext(ctx).Create(m).Error
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if stderrors.As(err, &mysqlErr) && mysqlErr.Number == mysqlErrCodeDupEntry {
			msg := mysqlErr.Message
			if strings.Contains(msg, "uk_user_id") {
				return ErrRoomMembersUserIDDuplicate
			}
			if strings.Contains(msg, "uk_room_user") {
				return ErrRoomMembersRoomUserDuplicate
			}
			// 极罕见：1062 但既不含 uk_user_id 也不含 uk_room_user → raw 透传
		}
		return err
	}
	return nil
}

// CountByRoomID 返回 roomID 下 room_members 行数（用于 V1 §10.4 步骤 4 容量校验）。
//
// **必须在事务内调用**（与同事务 SELECT rooms ... FOR UPDATE 步骤一起执行）；事务外
// 调用 race 窗口仍存在 —— 步骤序列必须是：
//  1. SELECT rooms FOR UPDATE（锁住 rooms 行）
//  2. CountByRoomID（受锁保护，并发 join / leave wait）
//  3. INSERT room_members（容量已校验，安全）
//
// 这是 V1 §10.4 服务端逻辑步骤 2-4-5 钦定的串行化路径；缺步骤 1 锁则 step 2 / 3
// 之间存在并发 race（5 用户同时 join 满员房间 → 5 个都看到 count == 3 → 5 个
// 同时 INSERT，超员 → 4 个被 UNIQUE(user_id) 兜底，但仍多 1 行）。
//
// 用 SELECT COUNT(*) FROM room_members WHERE room_id = ?，走 idx_room_id 索引。
// query 失败 → 返 raw error 透传。
func (r *roomMemberRepo) CountByRoomID(ctx context.Context, roomID uint64) (int, error) {
	db := tx.FromContext(ctx, r.db)
	var count int64
	err := db.WithContext(ctx).
		Model(&RoomMember{}).
		Where("room_id = ?", roomID).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// DeleteByRoomAndUser 按 (room_id, user_id) 双键定位删除单行 room_members（Story 11.5
// 引入；用于 V1 §10.5 步骤 3 leave 事务删除 leaver 自己的成员行）。
//
// **必须在事务内调用**（与同事务 SELECT rooms ... FOR UPDATE 步骤一起执行）；事务外
// 调用违反 V1 §10.5 步骤 2-3 钦定的串行化路径 —— FOR UPDATE 锁立即释放，跨事务
// leave-vs-join race 重新出现。
//
// 返回 RowsAffected 让 service 层做 6004 兜底分流：
//   - == 1：删除成功（happy path）
//   - == 0：同一 user 并发两次 leave 输家场景（赢家已先删该行）→ service 层翻译为 6004
//   - >  1：理论不可能（room_members 有 UNIQUE(room_id, user_id)，最多 1 行匹配）；
//     若发生属严重数据脏 —— **service 层兜底视为成功路径**（rowsAffected != 0 即 commit
//     继续走步骤 4 ~ 6；多删的副作用由 schema UNIQUE 约束兜底，不会发生）
//
// 用 GORM Where(...).Delete(&RoomMember{}) 路径生成 DELETE FROM room_members WHERE
// room_id = ? AND user_id = ?；走 idx_room_id 索引 + uk_room_user 唯一约束高效定位。
// 不支持软删除（room_members 无 deleted_at 字段）。
//
// query 失败 → 返 (0, raw error) 透传给 service（service 包成 1009）。
func (r *roomMemberRepo) DeleteByRoomAndUser(ctx context.Context, roomID, userID uint64) (int64, error) {
	db := tx.FromContext(ctx, r.db)
	result := db.WithContext(ctx).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		Delete(&RoomMember{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// ExistsForShareByRoomAndUser 取 FOR SHARE 共享锁锁定 (room_id, user_id) 双键定位
// 的单行 room_members（Story 11.6 引入；用于 V1 §10.3 步骤 1b ACL 共享锁）。
//
// **必须在事务内调用**（与同事务 userRepo.FindByID 步骤 1a + roomRepo.FindByID
// 步骤 2 + ListRosterByRoomID 步骤 3 一起执行）；事务外调用 FOR SHARE 锁立即
// 释放（autocommit 模式下任何 SELECT 完成即 commit），违反 V1 §10.3 + 数据库
// 设计 §8.8 钦定的串行化路径 —— 跨事务并发 leave 的 DELETE 不再被阻塞，
// post-leave 数据泄漏 race 重新出现。
//
// 用 GORM Raw + Scan 路径生成 SELECT ... FOR SHARE SQL；走 idx_room_id 索引 +
// uk_room_user 唯一约束高效定位（最多 1 行匹配）。
//
// 返：
//   - 命中 1 行 → (true, nil)
//   - 0 行 → (false, nil) —— 与 RoomExists / IsUserInRoom 同模式，Raw + Scan
//     在 0 行时不返 ErrRecordNotFound 而是保持 dummy zero-value
//   - query 失败 → (false, raw error 透传给 service（service 包成 1009）)
func (r *roomMemberRepo) ExistsForShareByRoomAndUser(ctx context.Context, roomID, userID uint64) (bool, error) {
	db := tx.FromContext(ctx, r.db)
	var dummy int
	// MySQL 8.0 SQL syntax: LIMIT 必须在 locking clause（FOR SHARE / FOR UPDATE）**之前**；
	// `... FOR SHARE LIMIT 1` 在 MySQL 5.7+ 是 ER_PARSE_ERROR (1064)。GORM 不会重写顺序，
	// raw SQL 必须按 MySQL 钦定顺序写。RoomExists / IsUserInRoom 用普通 SELECT 不带锁
	// 所以无此约束。
	err := db.WithContext(ctx).
		Raw("SELECT 1 FROM room_members WHERE room_id = ? AND user_id = ? LIMIT 1 FOR SHARE", roomID, userID).
		Scan(&dummy).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return dummy == 1, nil
}

// ListRosterByRoomID 查 room_members + INNER JOIN users + LEFT JOIN pets 聚合的
// roster，按 room_members.joined_at ASC 稳定排序（Story 11.6 引入；用于 V1 §10.3
// 步骤 3 GET /rooms/{roomId} 房间详情查询）。
//
// **必须在事务内调用**（与同事务 ExistsForShareByRoomAndUser 步骤 1b + roomRepo.FindByID
// 步骤 2 一起执行）；事务外调用违反 V1 §10.3 + 数据库设计 §8.8 钦定的"读快照事务
// （含 ACL 共享锁）"路径 —— 步骤 1 ~ 3 必须共享同一 REPEATABLE READ snapshot 才
// 能保证内部一致性 + FOR SHARE 锁才能阻止并发 leave 的 DELETE 在本读事务期间提交。
//
// **LEFT JOIN pets 必须**（不能 INNER JOIN）：pet-less 账号若用 INNER JOIN 会被
// 静默丢行 → 违反 memberCount === members.length 不变量；LEFT JOIN 时 pets.* 列
// 为 NULL，GORM Scan 把 NULL 映射为 *uint64 nil（RosterRow.PetID）让 service 层
// 据此下发 wire DTO `pet: null`。
//
// INNER JOIN users（不是 LEFT JOIN）：users 行必存在（room_members.user_id
// 引用 users.id；orphan 不可能）。
//
// pets.is_default = 1 过滤：每个用户最多一只默认 pet（uk_user_default_pet
// 唯一约束 §5.3）；本接口节点 4 阶段仅返回默认 pet。
//
// ORDER BY room_members.joined_at ASC：稳定排序便于 client 渲染顺序稳定。
//
// 用 GORM Raw 路径而非 Joins / Preload —— Raw 让 SQL 显式可读 + Scan 到自定
// 义 RosterRow struct 避免 GORM Preload 多查（preload 路径会触发额外 1 query
// 加载 pets，违反"读快照事务步骤 3 单 SQL"钦定）。
//
// 返：
//   - 0 行（房间存在但无成员，理论 closed 房间路径） → ([]RosterRow{}, nil)
//   - 多行 → ([]RosterRow{...}, nil)
//   - DB error → (nil, raw error 透传给 service（service 包成 1009）)
func (r *roomMemberRepo) ListRosterByRoomID(ctx context.Context, roomID uint64) ([]RosterRow, error) {
	db := tx.FromContext(ctx, r.db)
	var rows []RosterRow
	err := db.WithContext(ctx).
		Raw(`SELECT room_members.user_id AS user_id, users.nickname AS nickname, users.avatar_url AS avatar_url, pets.id AS pet_id
		     FROM room_members
		     INNER JOIN users ON room_members.user_id = users.id
		     LEFT JOIN pets ON pets.user_id = room_members.user_id AND pets.is_default = 1
		     WHERE room_members.room_id = ?
		     ORDER BY room_members.joined_at ASC`, roomID).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if rows == nil {
		// GORM Scan 在 0 行时可能保持 nil；统一返 [] 让调用方不需要 nil-check
		return []RosterRow{}, nil
	}
	return rows, nil
}
