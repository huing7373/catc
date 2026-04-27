package mysql

import (
	"context"
	stderrors "errors"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/repo/tx"
)

// AuthTypeGuest 是 user_auth_bindings.auth_type 表示游客绑定的取值（数据库设计 §5.2）。
//
// 在 repo 层独立定义而非从 service 包 import：避免 service → repo 的反向 import
// 循环（service 包 import repo/mysql；如果 repo/mysql 反过来 import service 会环）。
// service 层有同名常量 authTypeGuest（包内未导出），两边语义同源但不互相引用。
const AuthTypeGuest = 1

// mysqlErrCodeDupEntry 是 MySQL 唯一索引冲突的 error number（参见 MySQL 8.0
// 官方 error code 列表 ER_DUP_ENTRY = 1062）。go-sql-driver/mysql 把 server 端
// 错误号原样填到 *mysql.MySQLError.Number。
//
// **不**用字符串匹配（"Error 1062" / "Duplicate entry"）—— 不可靠 + 国际化版本不同
// + 未来 server 升级文案可能调整。
const mysqlErrCodeDupEntry = 1062

// AuthBinding 是 user_auth_bindings 表的 GORM domain struct（数据库设计 §5.2 +
// migrations/0002）。
//
// 字段语义：
//   - AuthType: TINYINT NOT NULL；1=guest（节点 2 阶段唯一值），2=wechat（future epic）
//   - AuthIdentifier: VARCHAR(128) NOT NULL；guest 场景 = guest_uid，wechat 场景 = openid
//   - AuthExtra: JSON NULL；扩展登录信息（节点 2 阶段不消费，留空 NULL）
//
// **唯一约束**：UNIQUE KEY uk_auth_type_identifier (auth_type, auth_identifier) —— 保证
// 同一登录身份只绑一个账号；并发场景下第二次 INSERT 触发 ER_DUP_ENTRY 1062 → service
// 层捕获后回退到 reuseLogin（详见 service.firstTimeLogin）。这是节点 2 §AR3 钦定的
// "同 guestUid 自然幂等" 实装基础。
type AuthBinding struct {
	ID             uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	UserID         uint64    `gorm:"column:user_id"`
	AuthType       int8      `gorm:"column:auth_type"`
	AuthIdentifier string    `gorm:"column:auth_identifier"`
	AuthExtra      []byte    `gorm:"column:auth_extra;type:json"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime:milli"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime:milli"`
}

// TableName 显式声明表名（默认 inflection 会推 "user_auth_bindings" 与表名一致；
// 显式 override 避免 GORM 升级时的隐性漂移）。
func (AuthBinding) TableName() string { return "user_auth_bindings" }

// AuthBindingRepo 是 user_auth_bindings 表的访问接口。
//
// 节点 2 阶段消费方：
//   - service.AuthService.GuestLogin: FindByGuestUID（事务外，前置查询）
//   - service.AuthService.firstTimeLogin: Create（事务内，可能 ER_DUP_ENTRY）
type AuthBindingRepo interface {
	// Create 插入一行 binding。
	//
	// 特殊错误处理：
	//   - 唯一索引 uk_auth_type_identifier 冲突 → 返 ErrAuthBindingDuplicate 哨兵
	//     （并发场景：两个并发请求同 guestUid，第二个 INSERT 被 MySQL 拒绝 ER_DUP_ENTRY 1062）
	//   - 其他 error 透传给 service
	Create(ctx context.Context, b *AuthBinding) error

	// FindByGuestUID 查 (auth_type=1, auth_identifier=guestUid) 的 binding 行。
	//
	// NotFound → 返 (nil, ErrAuthBindingNotFound)；其他 error 透传。
	// service 用 errors.Is 区分 ErrAuthBindingNotFound（→ 走首次初始化分支）vs
	// 其他 DB 异常（→ 包成 1009）。
	FindByGuestUID(ctx context.Context, guestUID string) (*AuthBinding, error)
}

// authBindingRepo 是 AuthBindingRepo 的默认实装。
type authBindingRepo struct {
	db *gorm.DB
}

// NewAuthBindingRepo 构造 AuthBindingRepo。
func NewAuthBindingRepo(db *gorm.DB) AuthBindingRepo {
	return &authBindingRepo{db: db}
}

// Create 插入 binding；ER_DUP_ENTRY 1062 → ErrAuthBindingDuplicate。
//
// 识别 MySQL 错误号用 errors.As 拿到 *mysql.MySQLError 后比 Number；不要硬编码
// 字符串匹配 "Duplicate entry" —— 不可靠（非英文 locale / 未来版本可能改文案）。
func (r *authBindingRepo) Create(ctx context.Context, b *AuthBinding) error {
	db := tx.FromContext(ctx, r.db)
	err := db.WithContext(ctx).Create(b).Error
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if stderrors.As(err, &mysqlErr) && mysqlErr.Number == mysqlErrCodeDupEntry {
			return ErrAuthBindingDuplicate
		}
		return err
	}
	return nil
}

// FindByGuestUID 查 binding 行；NotFound → ErrAuthBindingNotFound。
//
// 显式 auth_type 过滤是必须的（不能只按 auth_identifier）—— uk_auth_type_identifier
// 是 (auth_type, auth_identifier) 复合唯一，理论上 wechat openid 与 guest_uid 字符串可能
// 撞值（极小概率但不能依赖统计学保证）；显式 auth_type=1 让查询走索引最左前缀。
func (r *authBindingRepo) FindByGuestUID(ctx context.Context, guestUID string) (*AuthBinding, error) {
	db := tx.FromContext(ctx, r.db)
	var b AuthBinding
	err := db.WithContext(ctx).
		Where("auth_type = ? AND auth_identifier = ?", AuthTypeGuest, guestUID).
		First(&b).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAuthBindingNotFound
		}
		return nil, err
	}
	return &b, nil
}
