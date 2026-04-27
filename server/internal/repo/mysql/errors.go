// Package mysql 提供基于 GORM + database/sql 的 MySQL repo 实装。
//
// 本包是节点 2 §Story 4.6 起的真实业务 repo 落地点，承载 users / pets /
// user_auth_bindings / user_step_accounts / user_chests 等表的 CRUD 抽象。
// 后续 Epic 7 / 11 / 14 / 17 / 20 / 23 / 26 / 32 都会往本包加新的 repo 文件。
//
// # 分层约束（与 docs/宠物互动App_Go项目结构与模块职责设计.md §5.3 对齐）
//
//   - repo 只做单表 CRUD + 错误识别（如 ER_DUP_ENTRY 1062 → ErrAuthBindingDuplicate）
//   - repo **不**承载业务规则（业务常量如 petTypeDefault=1 在 service 层定义）
//   - repo **不**调 txMgr.WithTx —— 事务边界归 service 控制
//   - repo **不**返业务码 / *AppError —— 只产 raw error / sentinel error，由 service 层
//     用 apperror.Wrap 翻译为业务码（ADR-0006 三层映射）
//
// # ctx 传播（ADR-0007 §2.3）
//
// 每个 repo 方法第一参数 ctx context.Context；所有 GORM 调用都必须 .WithContext(ctx)，
// 否则 ctx cancel（client 断开 / deadline）不会传播到 driver 层 → SQL 不会被中断。
//
// # 事务感知（tx.FromContext 模式）
//
// repo 内部用 tx.FromContext(ctx, r.db) 取 db handle：
//
//   - ctx 携带 tx（即 service 层 txMgr.WithTx 传入的 txCtx）→ 返 tx 句柄
//   - ctx 不带 tx → 返 fallback（即 r.db，repo 持有的外层 db pool）
//
// 这让同一个 repo 方法**既**能在事务内被调（用 txCtx），**又**能在事务外被调（用普通 ctx），
// 业务层无需为"是否在事务内"维护两套 API。详见 internal/repo/tx/manager.go 顶部注释。
package mysql

import "errors"

// 哨兵 error：service 层用 errors.Is 区分**业务可识别**的失败 vs **DB 异常**。
//
//   - ErrAuthBindingNotFound: FindByGuestUID 查不到行（非异常 —— 用于"首次登录" vs "复用登录"分支）
//   - ErrAuthBindingDuplicate: Create 时 UNIQUE(auth_type, auth_identifier) 冲突
//     （并发场景：两个并发请求同 guestUid，先入者已写入 binding 后第二个 INSERT 触发
//     MySQL ER_DUP_ENTRY 1062 → service 层捕获后回退到 reuseLogin 分支）
//   - ErrUsersGuestUIDDuplicate: users.Create 时 UNIQUE(guest_uid) `uk_guest_uid` 冲突
//     （并发场景：两个并发请求同 guestUid，先入者 Tx A 已 commit users 行 → Tx B
//     INSERT users 触发 MySQL ER_DUP_ENTRY 1062 → 由于 firstTimeLogin 内 users 是
//     **第一步**，比 user_auth_bindings 更早抛冲突 —— 必须独立哨兵 + 与
//     ErrAuthBindingDuplicate 同样回退到 reuseLogin。**不同表的唯一约束需要**
//     **独立哨兵** —— 用同一个会让 service 误以为冲突源是 binding 表）
//   - ErrUserNotFound: FindByID 查不到（理论不应发生 —— binding 存在但 user 不存在 → 数据脏）
//   - ErrPetNotFound: FindDefaultByUserID 查不到（理论不应发生 —— user 创建后必有默认 pet）
//
// 其他 DB 异常（连接断 / SQL 语法错 / 死锁等）原样透传给 service，由 service 兜底
// wrap 成 ErrServiceBusy(1009)。
var (
	ErrAuthBindingNotFound    = errors.New("mysql: auth binding not found")
	ErrAuthBindingDuplicate   = errors.New("mysql: auth binding duplicate (uk_auth_type_identifier conflict)")
	ErrUsersGuestUIDDuplicate = errors.New("mysql: users guest_uid duplicate (uk_guest_uid conflict)")
	ErrUserNotFound           = errors.New("mysql: user not found")
	ErrPetNotFound            = errors.New("mysql: default pet not found")
)
