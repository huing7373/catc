package mysql

import (
	"context"
	stderrors "errors"

	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/repo/tx"
)

// RoomMember 是 room_members 表的最小 GORM domain struct（Story 10.3 引入；
// Epic 11.2 落地 0007_init_rooms / 0008_init_room_members migration 后扩展为
// 完整字段集合）。
//
// **关键**：本 story 阶段 migrations 仓库内**没有** rooms / room_members migration
// （Epic 11.2 才落地）。此处 domain struct 是为 WS 握手期 user-in-room 校验 +
// placeholder snapshot 全成员查询提供的**最小**结构骨架。集成测试用临时建表
// （见 ws_integration_test.go startMySQLWithRoomMemberFixture helper）；prod
// 部署在 Epic 11.2 之前**不应**走 WS 路径（rooms / room_members 表不存在）。
//
// 字段：
//   - RoomID: BIGINT UNSIGNED NOT NULL（与 users.id 同类型）
//   - UserID: BIGINT UNSIGNED NOT NULL
//
// 表 PK = (room_id, user_id) 复合主键 + INDEX idx_user_room (user_id, room_id)
// 让 IsUserInRoom 走 secondary 索引 + RoomExists / ListMembers 走主键扫描。
type RoomMember struct {
	RoomID uint64 `gorm:"column:room_id;primaryKey"`
	UserID uint64 `gorm:"column:user_id;primaryKey"`
}

// TableName 显式声明 "room_members"。
func (RoomMember) TableName() string { return "room_members" }

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
