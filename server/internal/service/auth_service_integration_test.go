//go:build integration
// +build integration

// Story 4.6 集成测试：用 dockertest 起真实 mysql:8.0 容器跑 3 条 happy 链路 case：
//   1. 首次调用 → 5 张表各新增 1 行
//   2. 同 guestUid 第二次调用 → 不新增行，返回同一 user_id（自然幂等）
//   3. 不同 guestUid 第三次调用 → 再新增 5 行
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑这些；
// 只在 `bash scripts/build.sh --integration`（即 `go test -tags=integration`）触发。
//
// 不覆盖回滚 / 并发 / 边界（**Story 4.7 专门做**）；本 story 集成测试只验证
// V1 §4.1 钦定的 3 条 happy 链路 + 自然幂等。
//
// 复用 4.2 / 4.3 的 startMySQL / migrationsPath helper 模式（每 case 独立起容器，
// 不抽到跨包 helper —— 避免新建跨包 testing util 的范围扩散）。

package service_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
	"github.com/huing/cat/server/internal/infra/migrate"
	"github.com/huing/cat/server/internal/pkg/auth"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
	"github.com/huing/cat/server/internal/service"
)

// ============================================================
// dockertest helpers（与 4.2 / 4.3 同模式；故意复制不抽包，避免范围扩散）
// ============================================================

// startMySQL 起一个 mysql:8.0 容器，等 ping 通后返回 (DSN, cleanup)。
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

// migrationsPath 返回 server/migrations 的绝对路径。
// 测试文件在 server/internal/service/ 下 → 相对 ../../migrations。
func migrationsPath(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../../migrations")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	return abs
}

// runMigrations 起容器后跑 migrate Up，把 5 张表建好。
func runMigrations(t *testing.T, dsn string) {
	t.Helper()
	mig, err := migrate.New(dsn, migrationsPath(t))
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	defer mig.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := mig.Up(ctx); err != nil {
		t.Fatalf("migrate Up: %v", err)
	}
}

// buildAuthService: 起容器 → migrate → 装配 svc + 返清理 closure。
func buildAuthService(t *testing.T) (svc service.AuthService, sqlDB *sql.DB, cleanup func()) {
	t.Helper()

	dsn, dockerCleanup := startMySQL(t)
	runMigrations(t, dsn)

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
		dockerCleanup()
		t.Fatalf("db.Open: %v", err)
	}

	signer, err := auth.New("test-secret-must-be-at-least-16-bytes", 3600)
	if err != nil {
		dockerCleanup()
		t.Fatalf("auth.New: %v", err)
	}

	txMgr := tx.NewManager(gormDB)
	userRepo := mysql.NewUserRepo(gormDB)
	bindingRepo := mysql.NewAuthBindingRepo(gormDB)
	petRepo := mysql.NewPetRepo(gormDB)
	stepRepo := mysql.NewStepAccountRepo(gormDB)
	chestRepo := mysql.NewChestRepo(gormDB)

	svc = service.NewAuthService(txMgr, signer, userRepo, bindingRepo, petRepo, stepRepo, chestRepo)

	rawDB, err := gormDB.DB()
	if err != nil {
		dockerCleanup()
		t.Fatalf("gormDB.DB(): %v", err)
	}

	cleanup = func() {
		_ = rawDB.Close()
		dockerCleanup()
	}
	return svc, rawDB, cleanup
}

// assertCount 直接用 SELECT COUNT(*) 验证（不用 GORM —— 更纯粹的 DB 状态验证）。
func assertCount(t *testing.T, sqlDB *sql.DB, query string, args []any, want int64, label string) {
	t.Helper()
	var got int64
	row := sqlDB.QueryRow("SELECT COUNT(*) FROM "+query, args...)
	if err := row.Scan(&got); err != nil {
		t.Fatalf("count query failed (%s): %v", label, err)
	}
	if got != want {
		t.Errorf("count %s = %d, want %d", label, got, want)
	}
}

// ============================================================
// AC8.1: 首次调用 → DB 5 张表各新增 1 行
// ============================================================
func TestAuthService_GuestLogin_FirstTime_CreatesFiveRows(t *testing.T) {
	svc, sqlDB, cleanup := buildAuthService(t)
	defer cleanup()

	out, err := svc.GuestLogin(context.Background(), service.GuestLoginInput{
		GuestUID:    "uid-test-1",
		Platform:    "ios",
		AppVersion:  "1.0.0",
		DeviceModel: "iPhone15,2",
	})
	if err != nil {
		t.Fatalf("GuestLogin: %v", err)
	}
	if out.Token == "" {
		t.Errorf("token is empty")
	}
	if out.UserID == 0 {
		t.Errorf("UserID = 0, want > 0")
	}
	if out.Nickname == "" || out.Nickname[:6] != "用户" {
		// "用户" 是 6 字节（utf8mb4 每字符 3 字节），但前 6 字节恰好是 "用户" 的前缀
		// 严谨断言：应当是 "用户" + strconv 的 ID 字符串
		expectedNick := "用户" + strconv.FormatUint(out.UserID, 10)
		if out.Nickname != expectedNick {
			t.Errorf("Nickname = %q, want %q", out.Nickname, expectedNick)
		}
	}
	if out.PetID == 0 {
		t.Errorf("PetID = 0, want > 0")
	}
	if out.PetName != "默认小猫" {
		t.Errorf("PetName = %q, want 默认小猫", out.PetName)
	}
	if out.PetType != 1 {
		t.Errorf("PetType = %d, want 1", out.PetType)
	}
	if out.HasBoundWechat {
		t.Errorf("HasBoundWechat must be false for guest")
	}

	// 验证 5 张表各新增 1 行
	assertCount(t, sqlDB, "users WHERE guest_uid=?", []any{"uid-test-1"}, 1, "users")
	assertCount(t, sqlDB, "user_auth_bindings WHERE auth_type=1 AND auth_identifier=?", []any{"uid-test-1"}, 1, "user_auth_bindings")
	assertCount(t, sqlDB, "pets WHERE user_id=? AND is_default=1", []any{out.UserID}, 1, "pets")
	assertCount(t, sqlDB, "user_step_accounts WHERE user_id=?", []any{out.UserID}, 1, "user_step_accounts")
	assertCount(t, sqlDB, "user_chests WHERE user_id=? AND status=1", []any{out.UserID}, 1, "user_chests")
}

// ============================================================
// AC8.2: 同一 guestUid 第二次调用 → 不新增行，返回同一 user_id（自然幂等）
// ============================================================
func TestAuthService_GuestLogin_SameGuestUID_ReturnsSameUserIDWithoutDup(t *testing.T) {
	svc, sqlDB, cleanup := buildAuthService(t)
	defer cleanup()

	// 第一次调
	out1, err := svc.GuestLogin(context.Background(), service.GuestLoginInput{
		GuestUID: "uid-same", Platform: "ios", AppVersion: "1.0.0", DeviceModel: "iPhone15,2",
	})
	if err != nil {
		t.Fatalf("first GuestLogin: %v", err)
	}

	// 第二次调（相同 guestUid）
	out2, err := svc.GuestLogin(context.Background(), service.GuestLoginInput{
		GuestUID: "uid-same", Platform: "ios", AppVersion: "1.0.0", DeviceModel: "iPhone15,2",
	})
	if err != nil {
		t.Fatalf("second GuestLogin: %v", err)
	}

	// user_id 必须相同
	if out1.UserID != out2.UserID {
		t.Errorf("first UserID=%d, second UserID=%d, want same", out1.UserID, out2.UserID)
	}
	// pet_id 也应相同（同一默认猫）
	if out1.PetID != out2.PetID {
		t.Errorf("first PetID=%d, second PetID=%d, want same", out1.PetID, out2.PetID)
	}
	// token 可能不同（每次签发都签新的；这是预期行为）
	if out1.Token == "" || out2.Token == "" {
		t.Errorf("both tokens must be non-empty")
	}

	// DB 状态：仍只有 1 个 user / binding / pet / step_account / chest
	assertCount(t, sqlDB, "users WHERE guest_uid=?", []any{"uid-same"}, 1, "users (no dup)")
	assertCount(t, sqlDB, "user_auth_bindings WHERE auth_type=1 AND auth_identifier=?", []any{"uid-same"}, 1, "bindings (no dup)")
	assertCount(t, sqlDB, "pets WHERE user_id=?", []any{out1.UserID}, 1, "pets (no dup)")
	assertCount(t, sqlDB, "user_step_accounts WHERE user_id=?", []any{out1.UserID}, 1, "step_accounts (no dup)")
	assertCount(t, sqlDB, "user_chests WHERE user_id=?", []any{out1.UserID}, 1, "chests (no dup)")
}

// ============================================================
// review-r1 P1 回归: 同 guestUid 并发调用 → 两个请求都成功 + 同 user_id + DB 仅 1 行
//
// 这是真实 race 验证（区别于 unit test 用 stub 模拟 ErrUsersGuestUIDDuplicate）：
//   - 起两个 goroutine 同时 svc.GuestLogin 同 guestUid
//   - 一个会赢（INSERT users 成功 → 完成事务），另一个会输（INSERT users 触发
//     uk_guest_uid 1062 / INSERT binding 触发 uk_auth_type_identifier 1062）
//   - **两个**返回的 UserID 必须相同 + DB 只有 1 行 user
//
// 不验证哪个先 / 哪个抛了哪个 sentinel —— InnoDB 的 insert intent gap lock 行为依赖
// 隔离级别 + timing；只断言"幂等结果"（两次都成功 + 同 user_id）。
//
// 失败模式：若 review-r1 修复回退（service 没识别 ErrUsersGuestUIDDuplicate），输的
// 那条会拿到 1009 → t.Fatalf。
// ============================================================
func TestAuthService_GuestLogin_ConcurrentSameGuestUID_BothSucceedSameUserID(t *testing.T) {
	svc, sqlDB, cleanup := buildAuthService(t)
	defer cleanup()

	const guestUID = "uid-concurrent-race"
	type result struct {
		userID uint64
		err    error
	}
	results := make([]result, 2)

	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		i := i
		go func() {
			defer wg.Done()
			out, err := svc.GuestLogin(context.Background(), service.GuestLoginInput{
				GuestUID: guestUID, Platform: "ios", AppVersion: "1.0.0", DeviceModel: "iPhone15,2",
			})
			if err != nil {
				results[i] = result{err: err}
				return
			}
			results[i] = result{userID: out.UserID}
		}()
	}
	wg.Wait()

	// 两个调用都必须成功（race 修复后任一失败 → 修复回归）
	for i, r := range results {
		if r.err != nil {
			t.Fatalf("goroutine %d returned error %v (race 路径必须走 reuseLogin 而非 1009)", i, r.err)
		}
	}
	// 必须返回同一 user_id
	if results[0].userID != results[1].userID {
		t.Errorf("two concurrent GuestLogin returned different UserIDs: %d vs %d (违反幂等)", results[0].userID, results[1].userID)
	}
	// DB 只有 1 行 user / binding / pet / step / chest（无重复）
	assertCount(t, sqlDB, "users WHERE guest_uid=?", []any{guestUID}, 1, "users (no dup race)")
	assertCount(t, sqlDB, "user_auth_bindings WHERE auth_type=1 AND auth_identifier=?", []any{guestUID}, 1, "bindings (no dup race)")
	assertCount(t, sqlDB, "pets WHERE user_id=?", []any{results[0].userID}, 1, "pets (no dup race)")
	assertCount(t, sqlDB, "user_step_accounts WHERE user_id=?", []any{results[0].userID}, 1, "step_accounts (no dup race)")
	assertCount(t, sqlDB, "user_chests WHERE user_id=?", []any{results[0].userID}, 1, "chests (no dup race)")
}

// ============================================================
// AC8.3: 不同 guestUid 第三次调用 → 再新增 5 行
// ============================================================
func TestAuthService_GuestLogin_DifferentGuestUID_CreatesNewFiveRows(t *testing.T) {
	svc, sqlDB, cleanup := buildAuthService(t)
	defer cleanup()

	// 第一个 guestUid
	out1, err := svc.GuestLogin(context.Background(), service.GuestLoginInput{
		GuestUID: "uid-A", Platform: "ios", AppVersion: "1.0.0", DeviceModel: "iPhone15,2",
	})
	if err != nil {
		t.Fatalf("first GuestLogin: %v", err)
	}

	// 第二个**不同** guestUid
	out2, err := svc.GuestLogin(context.Background(), service.GuestLoginInput{
		GuestUID: "uid-B", Platform: "ios", AppVersion: "1.0.0", DeviceModel: "iPhone15,2",
	})
	if err != nil {
		t.Fatalf("second GuestLogin: %v", err)
	}

	if out1.UserID == out2.UserID {
		t.Errorf("different guestUid produced same UserID=%d", out1.UserID)
	}
	if out1.PetID == out2.PetID {
		t.Errorf("different guestUid produced same PetID=%d", out1.PetID)
	}

	// DB 总计：2 个 user / binding / pet / step / chest
	assertCount(t, sqlDB, "users", nil, 2, "users (2 distinct)")
	assertCount(t, sqlDB, "user_auth_bindings", nil, 2, "bindings (2 distinct)")
	assertCount(t, sqlDB, "pets", nil, 2, "pets (2 distinct)")
	assertCount(t, sqlDB, "user_step_accounts", nil, 2, "step_accounts (2 distinct)")
	assertCount(t, sqlDB, "user_chests", nil, 2, "chests (2 distinct)")

	// 第二个用户的 5 行验证
	assertCount(t, sqlDB, "users WHERE guest_uid=?", []any{"uid-B"}, 1, "user-B exists")
	assertCount(t, sqlDB, "user_auth_bindings WHERE auth_identifier=?", []any{"uid-B"}, 1, "binding-B")
	assertCount(t, sqlDB, "pets WHERE user_id=? AND is_default=1", []any{out2.UserID}, 1, "pet-B")
	assertCount(t, sqlDB, "user_step_accounts WHERE user_id=?", []any{out2.UserID}, 1, "step-B")
	assertCount(t, sqlDB, "user_chests WHERE user_id=? AND status=1", []any{out2.UserID}, 1, "chest-B")
}
