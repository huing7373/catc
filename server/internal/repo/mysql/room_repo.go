package mysql

import (
	"context"

	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/repo/tx"
)

// RoomRepo 是 rooms 表的写入接口（Story 11.3 引入）。
//
// 本 story 阶段仅含 Create 方法（创建房间事务）；future Story 11.4 加
// FindByIDForUpdate（取 FOR UPDATE 锁的查询；用于 join 事务并发串行化）；
// Story 11.5 加 UpdateStatus（leave 后房间空 → status=2 closed）；
// Story 11.6 加 FindByID（GET /rooms/{roomId} 查房间详情）。
//
// 不在本 story 落地：Update（节点 4 阶段无修改 rooms 字段需求） / Delete（房间永远不删
// —— close 走 status=2，保留 audit trail）/ List（节点 4 阶段无房间列表需求）。
//
// **同包依赖**：`Room` GORM domain struct 由 Story 11.2 落地于 `room_member_repo.go`
// （历史原因 —— 11.2 同时引入 Room / RoomMember 两 struct，复用同一文件；本 story
// 不重复声明 / 不移文件，让 Room 与 RoomMember 在同一文件保持"两 struct 同源"语义）。
type RoomRepo interface {
	// Create 插入一行 rooms。GORM 在成功后回填 r.ID（AUTO_INCREMENT）。
	//
	// 必须传 *Room 指针 —— 传值类型 ID 不会回填。
	// 调用方典型用法见 service.RoomService.CreateRoom。
	//
	// **ER_DUP_ENTRY 1062 翻译**：rooms 表唯一约束**只有** PRIMARY KEY
	// （AUTO_INCREMENT 不会冲突），无业务唯一约束 → 1062 在 rooms.Create 路径
	// **理论不会触发**。本实装**不**做特殊翻译，任何 DB error 透传给 service
	// 层（service 包成 1009）；与 step_account_repo.Create 同模式（无哨兵需求）。
	Create(ctx context.Context, r *Room) error
}

// roomRepo 是 RoomRepo 的默认实装。
type roomRepo struct {
	db *gorm.DB
}

// NewRoomRepo 构造 RoomRepo。Story 11.3 引入。
func NewRoomRepo(db *gorm.DB) RoomRepo {
	return &roomRepo{db: db}
}

// Create 插入一行 rooms；GORM 成功后回填 r.ID。
//
// tx.FromContext(ctx, r.db) 取 db handle：ctx 内有 tx → 用 tx；否则 r.db（事务外）。
// 必须 .WithContext(ctx) —— ADR-0007 §2.3 钦定 repo 必须 WithContext，否则 ctx
// cancel 不会传播到 driver。
//
// 任何 DB error 透传 —— 与 step_account_repo.Create 同模式（无哨兵翻译需求；rooms
// 表只有 PRIMARY KEY 无业务唯一约束 → 1062 路径不可达）。
func (r *roomRepo) Create(ctx context.Context, room *Room) error {
	db := tx.FromContext(ctx, r.db)
	return db.WithContext(ctx).Create(room).Error
}
