// Package db 提供 MySQL 连接初始化与 fail-fast 启动校验。
//
// 选型背景见 ADR-0003 (`_bmad-output/implementation-artifacts/decisions/0003-orm-stack.md`)：
// GORM v2 + go-sql-driver/mysql + golang-migrate v4。
//
// 设计原则：
//   - **fail-fast over fallback**：DSN 空 / 驱动 parse 失败 / ping 不通都直接返 error，
//     不容忍降级（MEMORY.md "No Backup Fallback"；docs/lessons/2026-04-25-slog-init-before-startup-errors.md）。
//   - **不导出 *sql.DB**：让 db 句柄统一以 *gorm.DB 形式向上层暴露，避免业务代码绕过 ORM
//     写裸 SQL。`gorm.DB.DB()` 仍可拿到底层 *sql.DB（关闭连接 / 查 stats 时使用）。
//   - **测试可注入**：`Open` 接受 ctx + cfg 两参数；测试场景可绕过 Open 直接构造 *gorm.DB
//     （sqlmock + gorm.io/driver/mysql.New(mysql.Config{Conn: sqlMockDB})），见 mysql_test.go。
package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/infra/config"
)

// Open 按 cfg 打开 MySQL 连接 + 设置连接池参数 + 用 ctx 做一次 PingContext。
//
// 失败路径（**不**容忍降级，全部直接返 error）：
//   - cfg.DSN == "" → 立刻返 errors.New("mysql.dsn is empty")，不调驱动
//   - gorm.Open 失败（DSN 解析错 / driver 未注册等） → 返 fmt.Errorf("mysql open: %w", err)
//   - 取 *sql.DB 失败 → 返 fmt.Errorf("mysql sql.DB: %w", err)
//   - PingContext 失败（network unreachable / authentication failed / ...） → 返 fmt.Errorf("mysql ping: %w", err)
//
// 调用方（cmd/server/main.go）失败时走 `slog.Error + os.Exit(1)`，不允许"用空连接池继续启动"。
//
// 成功路径：返 (*gorm.DB, nil)；进程退出前调用方负责 `db.DB().Close()` 释放连接池。
func Open(ctx context.Context, cfg config.MySQLConfig) (*gorm.DB, error) {
	if cfg.DSN == "" {
		return nil, errors.New("mysql.dsn is empty")
	}

	gormDB, err := gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{
		// 节点 2 阶段使用 GORM 默认 logger（stdout）；slog 桥接列为 tech debt
		// （ADR-0003 §3.6）。本 story 不动 Logger 字段。
		//
		// SkipDefaultTransaction：禁用 GORM "单条写自动包事务"。我们自己用
		// txManager.WithTx 显式管理事务边界，关掉这个隐式行为减少噪声。
		SkipDefaultTransaction: true,
		// DisableAutomaticPing：禁用 gorm.Open 内部的隐式 Ping（GORM 默认
		// 在 Open 末尾会做一次 *sql.DB.Ping，**不**尊重传入的 ctx，会被 driver
		// 的 default dial timeout（典型 30s+）阻塞）。我们的 fail-fast 语义靠
		// 下面显式的 sqlDB.PingContext(ctx) 实现 —— ctx 一般由调用方设短
		// timeout（main.go 走 SIGINT/SIGTERM ctx，测试走 context.WithTimeout）。
		// 不关掉这个开关，PingContext 之前先被卡 30s 等于破坏 fail-fast。
		// Story 4.2 review 补漏，参见 docs/lessons/2026-04-26-config-env-override-and-gorm-auto-ping.md。
		DisableAutomaticPing: true,
	})
	if err != nil {
		return nil, fmt.Errorf("mysql open: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("mysql sql.DB: %w", err)
	}

	// 连接池参数按 ADR-0003 §3.4。0 表示 "由调用方明示不设限"，driver 默认行为生效；
	// 实际生产配置必须填非 0 值，由 local.yaml 默认值兜底。
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	if cfg.ConnMaxLifetimeSec > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeSec) * time.Second)
	}

	// fail-fast：ping 失败必须启动失败。失败原因可能是 network unreachable / auth /
	// MySQL server 未起来 / DSN 指向错误库 —— 都不应该让 server 假装启动成功后
	// 在第一个业务请求时报错。
	if err := sqlDB.PingContext(ctx); err != nil {
		// ping 失败时主动关闭已打开的 sqlDB，避免 leak（gorm.Open 已经分配过资源）
		_ = sqlDB.Close()
		return nil, fmt.Errorf("mysql ping: %w", err)
	}

	return gormDB, nil
}
