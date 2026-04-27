package mysql

import (
	"context"
	stderrors "errors"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/repo/tx"
)

// User 是 users 表的 GORM domain struct（对齐数据库设计 §5.1 + migrations/0001）。
//
// MVP 选择不做 domain 与 ORM 分离（4.2 sample/ 已建立先例：直接用 GORM tag 当 domain）；
// service 层与 repo 层共用同一 struct，减少重复维护成本。future 业务增长（domain 与表
// 字段语义大幅分离）再做拆分。
//
// 字段语义：
//   - ID: BIGINT UNSIGNED AUTO_INCREMENT 主键（uint64，对齐 §3.1）
//   - GuestUID: VARCHAR(128) NOT NULL，UNIQUE KEY uk_guest_uid（业务上由 binding 表
//     引用；本表保留该列方便审计 / debug，但**实际**唯一性约束由 user_auth_bindings
//     的 uk_auth_type_identifier 落地）
//   - Nickname: VARCHAR(64) NOT NULL DEFAULT ''；首次创建时占位空串，紧接 UpdateNickname
//     写真实昵称 "用户{id}"（详见 service.firstTimeLogin）
//   - AvatarURL: VARCHAR(255) NOT NULL DEFAULT ''；游客首次创建为空串
//   - Status: TINYINT NOT NULL DEFAULT 1；§6.x 钦定 1=active
//   - CurrentRoomID: BIGINT UNSIGNED NULL；快照字段，节点 4 房间 epic 才消费
//   - CreatedAt / UpdatedAt: DATETIME(3) 毫秒精度，GORM autoCreateTime/autoUpdateTime
//     处理（与 4.3 migrations 钦定的 CURRENT_TIMESTAMP(3) 默认值一致）
type User struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	GuestUID      string    `gorm:"column:guest_uid"`
	Nickname      string    `gorm:"column:nickname"`
	AvatarURL     string    `gorm:"column:avatar_url"`
	Status        int8      `gorm:"column:status"`
	CurrentRoomID *uint64   `gorm:"column:current_room_id"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime:milli"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime:milli"`
}

// TableName 显式声明表名（GORM 默认会按 struct 名 pluralize → "users"，正好与本
// 项目表名一致；显式声明避免未来 GORM 升级 / inflection 改动导致的隐性漂移）。
func (User) TableName() string { return "users" }

// UserRepo 是 users 表的访问接口。
//
// 节点 2 阶段消费方：
//   - service.AuthService.firstTimeLogin: Create + UpdateNickname（事务内）
//   - service.AuthService.reuseLogin: FindByID（事务外）
//
// 后续 Epic 4.8 GET /home / Epic 11 房间 / 等扩展方法（FindByGuestUID / UpdateRoom...）
// 时往本 interface 加新签名，**不**新建 UserRepoV2 拆分。
type UserRepo interface {
	// Create 插入一行 users。GORM 在成功后回填 u.ID（AUTO_INCREMENT）。
	//
	// **关键**：必须传 *User 指针 —— 传值类型 ID 不会回填。
	// 调用方典型用法见 service.firstTimeLogin。
	Create(ctx context.Context, u *User) error

	// UpdateNickname 仅更新 nickname 字段（不会覆盖其他字段如 created_at）。
	//
	// 用 GORM Updates 单字段（Update("nickname", v)）—— 与 Save(&u) 不同，
	// 后者会用 struct 全字段 UPDATE 包括 zero value，会把 created_at 等覆盖为零值。
	UpdateNickname(ctx context.Context, userID uint64, nickname string) error

	// FindByID 查单行；NotFound → ErrUserNotFound 哨兵；其他 DB 异常透传。
	FindByID(ctx context.Context, id uint64) (*User, error)
}

// userRepo 是 UserRepo 的默认实装；持有 *gorm.DB fallback（事务外用）。
type userRepo struct {
	db *gorm.DB
}

// NewUserRepo 构造 UserRepo。db 由 bootstrap 期注入（4.2 db.Open 产物）。
func NewUserRepo(db *gorm.DB) UserRepo {
	return &userRepo{db: db}
}

// Create 插入一行 users。
//
// tx.FromContext(ctx, r.db) 取 db handle：ctx 内有 tx → 用 tx；否则 r.db（事务外）。
// 必须 .WithContext(ctx) —— ADR-0007 §2.3 钦定 repo 必须 WithContext，否则 ctx
// cancel 不会传播到 driver。
//
// **ER_DUP_ENTRY 1062 翻译**：users 表唯一约束只有 `uk_guest_uid`（migrations/0001
// + 数据库设计 §5.1）—— PRIMARY KEY 是 AUTO_INCREMENT 不会冲突。所以本 Create 路径
// 任何 1062 都必然是 guest_uid 冲突 → 翻译为 ErrUsersGuestUIDDuplicate 哨兵；service
// 层并发场景下要把它和 ErrAuthBindingDuplicate 一起视为"先入者已建账户" → 走 reuseLogin。
//
// 不解析 mysql.MySQLError.Message 字符串（"Duplicate entry ... for key 'uk_guest_uid'"）—— 只比
// Number == 1062，原因和 auth_binding_repo.go 同一处注释一样：字符串国际化 / 版本不可靠。
// users 表只有这一个唯一索引，所以不需要按 key 名做 fan-out 也不会有歧义；future 给 users
// 加新唯一索引时本函数注释要补按 key 名分流。
func (r *userRepo) Create(ctx context.Context, u *User) error {
	db := tx.FromContext(ctx, r.db)
	err := db.WithContext(ctx).Create(u).Error
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if stderrors.As(err, &mysqlErr) && mysqlErr.Number == mysqlErrCodeDupEntry {
			return ErrUsersGuestUIDDuplicate
		}
		return err
	}
	return nil
}

// UpdateNickname 仅更新 nickname 列；不影响其他字段（包括 updated_at —— GORM
// 的 autoUpdateTime 在 Update 时会自动写当前时间，符合数据库 §3.2 ON UPDATE
// CURRENT_TIMESTAMP(3) 语义）。
func (r *userRepo) UpdateNickname(ctx context.Context, userID uint64, nickname string) error {
	db := tx.FromContext(ctx, r.db)
	return db.WithContext(ctx).
		Model(&User{}).
		Where("id = ?", userID).
		Update("nickname", nickname).Error
}

// FindByID 查单行 users；NotFound → ErrUserNotFound（哨兵 error，service 用
// errors.Is 识别），其他 DB error 透传给 service 由 service 包成 1009。
func (r *userRepo) FindByID(ctx context.Context, id uint64) (*User, error) {
	db := tx.FromContext(ctx, r.db)
	var u User
	err := db.WithContext(ctx).Where("id = ?", id).First(&u).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}
