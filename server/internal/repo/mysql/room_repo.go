package mysql

import (
	"context"
	stderrors "errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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

	// FindByIDForUpdate 取 rooms 行并对该行加排他锁（FOR UPDATE）。Story 11.4 引入。
	//
	// **必须在事务内调用**（caller 传入的 ctx 必须是 txMgr.WithTx 注入的 txCtx）；
	// 在事务外调用时 FOR UPDATE 锁会被 driver 立即释放（autocommit 模式下任何
	// SELECT 完成即 commit），等同于普通 SELECT，违反 V1 §10.4 "并发保护" 钦定。
	// ADR-0007 §2.4 钦定 fn 内全部 repo 调用用 txCtx；本方法是 join / leave 事务的
	// 第一步，是 ADR-0007 §2.4 的关键 hot path。
	//
	// 语义：取 rooms 行并对该行加排他锁，与并发 §10.5 leave 跨事务串行化（数据库
	// 设计.md §8.6 / §8.7 r9 P1#2 钦定 join + leave 必须都对 rooms 行加 FOR UPDATE）。
	//
	// 找不到 → 返 ErrRoomNotFound 哨兵（service 层 errors.Is 后翻译为 6001
	// apperror.ErrRoomNotFound）；query 失败 → 返 raw error 透传。
	// **注**：mysql.ErrRoomNotFound 与 apperror.ErrRoomNotFound 是**不同包**的同名
	// 常量 —— 前者是 repo 层哨兵 error，后者是业务码 6001；命名一致是**故意**的
	// （让阅读对照容易；review 时确认 import path 不冲突）。
	//
	// GORM clause.Locking{Strength: "UPDATE"} 路径生成 SELECT ... FOR UPDATE
	// SQL；与 raw SQL "SELECT ... FOR UPDATE" 等价但更安全（GORM 处理参数绑定 +
	// SQL injection；与 ADR-0003 §3.5 钦定一致）。
	//
	// **本方法不附加 status 过滤**（与 RoomMemberRepo.RoomExists 不同；后者是 WS
	// 握手期校验，需要排除 closed 房间）：本方法是 join 事务步骤 2a，需要先取行
	// 再判 status，让 step 2b 能产出明确的 6005 而非笼统的 6001。
	FindByIDForUpdate(ctx context.Context, roomID uint64) (*Room, error)
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

// FindByIDForUpdate 取 rooms 行并对该行加排他锁（FOR UPDATE）。Story 11.4 引入。
//
// **必须在事务内调用**（caller 传入的 ctx 必须是 txMgr.WithTx 注入的 txCtx）；
// 在事务外调用时 FOR UPDATE 锁会被 driver 立即释放（autocommit 模式下任何
// SELECT 完成即 commit），等同于普通 SELECT，违反 V1 §10.4 "并发保护" 钦定。
// ADR-0007 §2.4 钦定 fn 内全部 repo 调用用 txCtx；本方法是 join / leave 事务的
// 第一步，是 ADR-0007 §2.4 的关键 hot path。
//
// 找不到 → 返 ErrRoomNotFound（service 层 errors.Is 后翻译为 6001 ErrRoomNotFound）。
// **注**：mysql.ErrRoomNotFound 与 apperror.ErrRoomNotFound 是**不同包**的同名常量
// —— 前者是 repo 层哨兵 error，后者是业务码 6001；service 层用 errors.Is 翻译。
// 命名一致是**故意**的（让阅读对照容易；review 时确认 import path 不冲突）。
//
// GORM clause.Locking{Strength: "UPDATE"} 路径生成 SELECT ... FOR UPDATE
// SQL；与 raw SQL "SELECT ... FOR UPDATE" 等价但更安全（GORM 处理参数绑定 +
// SQL injection；与 ADR-0003 §3.5 钦定一致）。
func (r *roomRepo) FindByIDForUpdate(ctx context.Context, roomID uint64) (*Room, error) {
	db := tx.FromContext(ctx, r.db)
	var room Room
	err := db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", roomID).
		First(&room).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRoomNotFound
		}
		return nil, err
	}
	return &room, nil
}
