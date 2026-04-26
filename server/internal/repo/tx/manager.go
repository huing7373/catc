// Package tx 提供统一的事务管理器。
//
// 设计文档钦定（`docs/宠物互动App_Go项目结构与模块职责设计.md` §10）：
//
//	err := txManager.WithTx(ctx, func(txCtx context.Context) error { ... })
//
// 关键约束（CLAUDE.md "工作纪律" §ctx 必传 + ADR-0007 §2.4）：
//
//   - fn 内的所有 repo 调用必须用 **`txCtx`** 而非外层 ctx；用错 ctx 会绕过 tx 走
//     db pool 的新连接 → 该调用脱离事务 → 业务语义错乱（事务里未 commit 的数据
//     在外层 ctx 调用看不见）。
//   - repo 层用 `tx.FromContext(ctx, fallbackDB)` 取 db handle：ctx 携带 tx → 用 tx，
//     否则用 fallback。调用方（service 层）**不**直接拿 *gorm.DB。
//
// 不暴露的低层 API：
//
//   - 不暴露 Begin / Commit / Rollback —— 那是反模式：让业务层手写事务边界
//     违反 §5.1 "handler 不负责跨多个 repo 手动拼事务"。所有事务都走 WithTx。
//   - 不在 WithTx 实装内部打 slog —— 保留 fn 透明，避免每次事务两条 log 噪声；
//     如未来需要事务 trace，加 OTel span 而非 slog（详见 4-2 story Dev Notes §3）。
package tx

import (
	"context"

	"gorm.io/gorm"
)

// Manager 是事务管理器的对外接口。Production 实装由 NewManager(db) 构造；
// 测试场景可注入 mock 实装（service 层只 import 本 interface，不 import gorm）。
type Manager interface {
	// WithTx 开启事务，把 tx 句柄通过 ctx 传给 fn。fn 返回 nil → commit；返回 error → rollback。
	//
	// fn 内部所有 repo 调用必须用 `txCtx` 而非外层 ctx：
	//
	//	err := mgr.WithTx(ctx, func(txCtx context.Context) error {
	//	    user, err := userRepo.Create(txCtx, ...)  // ✅ 用 txCtx
	//	    if err != nil { return err }
	//	    return petRepo.Create(txCtx, ...)         // ✅ 用 txCtx
	//	})
	//
	// panic 走 GORM 默认行为（rollback + repanic），调用方不需要额外 defer recover。
	//
	// 外层 ctx cancel（client 断开 / deadline）会传播到 txCtx → MySQL driver 接收 cancel 信号
	// → 当前 SQL 立即返回 context.Canceled / DeadlineExceeded → fn 收到 err → rollback。
	WithTx(ctx context.Context, fn func(txCtx context.Context) error) error
}

// txKey 是 ctx 里携带 *gorm.DB tx 句柄的私有 key。
//
// 用未导出 struct{} 类型避免外部包用同样 key 类型撞 ctx（Go 标准模式）；
// `context.WithValue` 的 key 比较是类型 + 值双等，外部包无法构造同类型 key。
type txKey struct{}

// manager 是 Manager 的默认实装，持有外层 *gorm.DB 用于开启事务。
type manager struct {
	db *gorm.DB
}

// NewManager 构造 Manager。db 是已经过 db.Open + ping 成功的 *gorm.DB（见 internal/infra/db）。
func NewManager(db *gorm.DB) Manager {
	return &manager{db: db}
}

// WithTx 实装：内部用 GORM 的 db.Transaction 处理 BEGIN / COMMIT / ROLLBACK / panic
// 默认行为；通过 context.WithValue 把 tx 句柄注入新 ctx（即 txCtx），让 fn 内的 repo
// 通过 FromContext 拿到 tx handle。
func (m *manager) WithTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, txKey{}, tx)
		return fn(txCtx)
	})
}

// FromContext 是 repo 层取 db handle 的 helper：ctx 里有 tx → 返回 tx；否则返回 fallback。
//
// 典型使用模式（repo 层）：
//
//	func (r *UserRepo) FindByID(ctx context.Context, id uint64) (*User, error) {
//	    db := tx.FromContext(ctx, r.db)
//	    var user User
//	    if err := db.WithContext(ctx).Where("id = ?", id).First(&user).Error; err != nil {
//	        return nil, err
//	    }
//	    return &user, nil
//	}
//
// 注意事项：
//   - service / handler 层**不**应该调用 FromContext —— 它们应只调 repo 方法 + WithTx，
//     不直接接触 *gorm.DB。FromContext 是 repo 层的内部工具。
//   - fallback 一般是 repo 持有的外层 db（`r.db`）；ctx 里没有 tx 时退回 db pool。
//   - 即使 fallback 也建议在调用方 `.WithContext(ctx)` 一次以传播 ctx cancel
//     （ADR-0007 §2.3 repo 必须用 WithContext）。
func FromContext(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
		return tx
	}
	return fallback
}
