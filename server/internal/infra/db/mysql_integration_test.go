//go:build integration
// +build integration

// Story 4.2 集成测试：用 dockertest 起真实 mysql:8.0 容器跑 4 条 case：
//   1. Open + Ping happy path
//   2. WithTx commit happy path（INSERT + return nil → 容器外 SELECT 数 = 1）
//   3. WithTx rollback edge（INSERT + return error → 容器外 SELECT 数 = 0）
//   4. Open 后关闭容器 → 再 Ping 失败（fail-fast 行为验证）
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑这些；
// 只在 `bash scripts/build.sh --integration`（即 `go test -tags=integration`）触发。
//
// docker 不可用时 t.Skip("docker not available")，不让 CI 阻塞（不要写 panic / fail）。
//
// 每条测试独立起一个容器（`t.Cleanup` 释放）—— 避免测试间状态污染。容器端口动态分配
// 不固定 3306，避免与本机 MySQL 冲突。

package db_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
	"github.com/huing/cat/server/internal/repo/tx"
)

// startMySQL 起一个 mysql:8.0 容器，等 ping 通后返回 (DSN, cleanup)。
//
// 关键参数：
//   - MYSQL_ROOT_PASSWORD=catdev / MYSQL_DATABASE=cat_test
//   - 端口由 dockertest 动态分配（不固定 3306）
//   - retry 60 次，每次 2 秒（mysql:8.0 冷启 ~10-30s）
func startMySQL(t *testing.T) (string, func()) {
	t.Helper()

	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Skipf("docker not available: %v", err)
	}
	if err := pool.Client.Ping(); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "mysql",
		Tag:        "8.0",
		Env: []string{
			"MYSQL_ROOT_PASSWORD=catdev",
			"MYSQL_DATABASE=cat_test",
		},
	}, func(hc *docker.HostConfig) {
		// 容器自动清理（即使测试 panic 也清）
		hc.AutoRemove = true
		hc.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		t.Skipf("could not start mysql container: %v", err)
	}

	hostPort := resource.GetPort("3306/tcp")
	dsn := fmt.Sprintf("root:catdev@tcp(127.0.0.1:%s)/cat_test?charset=utf8mb4&parseTime=true&loc=Local&multiStatements=true", hostPort)

	pool.MaxWait = 90 * time.Second
	if err := pool.Retry(func() error {
		sqlDB, err := sql.Open("mysql", dsn)
		if err != nil {
			return err
		}
		defer sqlDB.Close()
		return sqlDB.Ping()
	}); err != nil {
		_ = pool.Purge(resource)
		t.Skipf("mysql container did not become ready: %v", err)
	}

	cleanup := func() {
		_ = pool.Purge(resource)
	}
	return dsn, cleanup
}

// TestMySQLIntegration_OpenAndPing 起容器 → db.Open 返回 *gorm.DB + ping 成功
// → 关闭 → 资源清理。这是 Open 函数 happy path 的真实集成验证。
func TestMySQLIntegration_OpenAndPing(t *testing.T) {
	dsn, cleanup := startMySQL(t)
	defer cleanup()

	cfg := config.MySQLConfig{
		DSN:                dsn,
		MaxOpenConns:       10,
		MaxIdleConns:       2,
		ConnMaxLifetimeSec: 60,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gormDB, err := db.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer func() {
		sqlDB, _ := gormDB.DB()
		_ = sqlDB.Close()
	}()

	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("gormDB.DB(): %v", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Errorf("PingContext after Open: %v", err)
	}
}

// TestMySQLIntegration_WithTx_Commit 起容器 → 预 CREATE 一张测试表 →
// WithTx 内 INSERT + return nil → 容器外 SELECT COUNT = 1。
//
// 这是 WithTx commit 路径的真实集成验证：fn 内的 INSERT 走 tx 连接，
// COMMIT 后外层 query 能看到。
func TestMySQLIntegration_WithTx_Commit(t *testing.T) {
	dsn, cleanup := startMySQL(t)
	defer cleanup()

	cfg := config.MySQLConfig{
		DSN:                dsn,
		MaxOpenConns:       10,
		MaxIdleConns:       2,
		ConnMaxLifetimeSec: 60,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gormDB, err := db.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer func() {
		sqlDB, _ := gormDB.DB()
		_ = sqlDB.Close()
	}()

	// 预 CREATE 测试表（DDL 在 InnoDB 下隐式 COMMIT，必须放 tx 外执行）
	createSQL := `CREATE TABLE IF NOT EXISTS tx_test (id BIGINT AUTO_INCREMENT PRIMARY KEY, val VARCHAR(64))`
	if err := gormDB.WithContext(ctx).Exec(createSQL).Error; err != nil {
		t.Fatalf("create test table: %v", err)
	}

	mgr := tx.NewManager(gormDB)
	err = mgr.WithTx(ctx, func(txCtx context.Context) error {
		txDB := tx.FromContext(txCtx, gormDB)
		return txDB.WithContext(txCtx).Exec(`INSERT INTO tx_test (val) VALUES (?)`, "committed").Error
	})
	if err != nil {
		t.Fatalf("WithTx commit: %v", err)
	}

	var count int64
	if err := gormDB.WithContext(ctx).Raw(`SELECT COUNT(*) FROM tx_test WHERE val = ?`, "committed").Scan(&count).Error; err != nil {
		t.Fatalf("post-commit count: %v", err)
	}
	if count != 1 {
		t.Errorf("post-commit count = %d, want 1", count)
	}
}

// TestMySQLIntegration_WithTx_Rollback 起容器 → 预 CREATE 测试表 →
// WithTx 内 INSERT + return error → 容器外 SELECT COUNT = 0。
//
// 这是 WithTx rollback 路径的真实集成验证：fn 返回 error 后所有写入回滚，
// 外层 query 看不到 dirty data。
//
// 注意：MySQL 8.0 InnoDB 下 DDL 隐式 COMMIT，所以**不**用 CREATE TABLE 验证回滚
// （DDL 不会回滚），改用预先 CREATE 一张测试表然后 WithTx 内 INSERT + force error。
func TestMySQLIntegration_WithTx_Rollback(t *testing.T) {
	dsn, cleanup := startMySQL(t)
	defer cleanup()

	cfg := config.MySQLConfig{
		DSN:                dsn,
		MaxOpenConns:       10,
		MaxIdleConns:       2,
		ConnMaxLifetimeSec: 60,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gormDB, err := db.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer func() {
		sqlDB, _ := gormDB.DB()
		_ = sqlDB.Close()
	}()

	createSQL := `CREATE TABLE IF NOT EXISTS tx_test_rb (id BIGINT AUTO_INCREMENT PRIMARY KEY, val VARCHAR(64))`
	if err := gormDB.WithContext(ctx).Exec(createSQL).Error; err != nil {
		t.Fatalf("create test table: %v", err)
	}

	forceErr := fmt.Errorf("force rollback for test")
	mgr := tx.NewManager(gormDB)
	err = mgr.WithTx(ctx, func(txCtx context.Context) error {
		txDB := tx.FromContext(txCtx, gormDB)
		if err := txDB.WithContext(txCtx).Exec(`INSERT INTO tx_test_rb (val) VALUES (?)`, "rolled-back").Error; err != nil {
			return err
		}
		return forceErr
	})
	if err == nil {
		t.Fatalf("WithTx returned nil, want %v", forceErr)
	}

	var count int64
	if err := gormDB.WithContext(ctx).Raw(`SELECT COUNT(*) FROM tx_test_rb WHERE val = ?`, "rolled-back").Scan(&count).Error; err != nil {
		t.Fatalf("post-rollback count: %v", err)
	}
	if count != 0 {
		t.Errorf("post-rollback count = %d, want 0 (rollback should erase the INSERT)", count)
	}
}

// TestMySQLIntegration_PingFailedAfterClose 起容器 → Open 成功 →
// 关掉容器 → 再 PingContext → 返 error。
//
// 这是 fail-fast 在"运行时" MySQL 不可达场景的验证：Open 时通了，但服务过程中
// MySQL down 掉，下次 Ping 必须返 error 而不是悬挂或返 nil。
func TestMySQLIntegration_PingFailedAfterClose(t *testing.T) {
	dsn, cleanup := startMySQL(t)
	cleanedUp := false
	defer func() {
		if !cleanedUp {
			cleanup()
		}
	}()

	cfg := config.MySQLConfig{
		DSN:                dsn,
		MaxOpenConns:       10,
		MaxIdleConns:       2,
		ConnMaxLifetimeSec: 60,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gormDB, err := db.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer func() {
		sqlDB, _ := gormDB.DB()
		_ = sqlDB.Close()
	}()

	// 主动关闭容器（提前清理 cleanup → 后续 Ping 必失败）
	cleanup()
	cleanedUp = true

	// 给 MySQL container 一点时间真正退出（dockertest Purge 是异步的）
	time.Sleep(2 * time.Second)

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()

	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("gormDB.DB(): %v", err)
	}
	if err := sqlDB.PingContext(pingCtx); err == nil {
		t.Errorf("PingContext after container purge returned nil, want error")
	}
}

// 防御 import 静态分析：保证 gorm 包被 import（被 db.Open 返回值用）
var _ *gorm.DB
