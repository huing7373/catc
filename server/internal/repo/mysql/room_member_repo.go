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
