//go:build integration
// +build integration

// Story 4.6 集成测试：用 dockertest 起真实 mysql:8.0 容器跑 4 条 happy 链路 case：
//   1. 首次调用 → 5 张表各新增 1 行
//   2. 同 guestUid 第二次调用 → 不新增行，返回同一 user_id（自然幂等）
//   3. 同 guestUid 2 goroutine race → 两条都成功 + 同 user_id（review-r1 lesson 回归）
//   4. 不同 guestUid 第三次调用 → 再新增 5 行
//
// Story 4.7 在同一份文件追加 7 case（共 8 类 epics.md 钦定场景中的 7 类，
// 第 8 类 边界 2 handler 层拦截在 internal/app/http/handler/auth_handler_integration_test.go）：
//   5. 回滚 1: pet repo 第 4 步失败 → 整体回滚（5 表全空）
//   6. 回滚 2: chest repo 第 6 步失败 → 整体回滚（5 表全空）
//   7. 回滚 3: user repo 第 1 步失败 → 整体回滚（5 表全空）
//   8. 并发 1: 100 goroutine 同 guestUid → 1 user + 全部拿到同 user_id
//   9. 并发 2: 100 goroutine 不同 guestUid → 100 独立 user + 无串数据
//  10. 边界 1: guestUid 长度 128（最大允许）→ 成功 + SELECT 验证完整存储
//  11. 重入: 同 guestUid 第二次走 reuseLogin → DB 行数完全不变 + token 不同
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑这些；
// 只在 `bash scripts/build.sh --integration`（即 `go test -tags=integration`）触发。
//
// 复用 4.2 / 4.3 的 startMySQL / migrationsPath helper 模式（每 case 独立起容器，
// 不抽到跨包 helper —— 避免新建跨包 testing util 的范围扩散）。
//
// **fault injection 范式（4.7 引入）**：faultUserRepo / faultPetRepo / faultChestRepo
// 包装真实 mysql repo —— 注入方法直接返 injectErr，其他方法透传给 delegate；
// 这样在事务内触发 ROLLBACK 时走真实 InnoDB undo log 路径，验证 5 表
// 全空 ≠ "未到达" 而是真实 rollback 行为。**不**用 stub repo（不真开 MySQL 事务）。

package service_test

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
	"github.com/huing/cat/server/internal/infra/migrate"
	"github.com/huing/cat/server/internal/pkg/auth"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
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

// ============================================================
// Story 4.7 增量：fault injection 包装真实 mysql repo（每个 wrapper 持有 delegate
// 真实 repo + injectErr；指定方法直接返 injectErr，其他方法透传给 delegate）。
//
// 这是 Layer 2 集成测试 fault injection 的标准范式（与 stub repo "全部方法都不
// 真调"截然不同）：包装真实 repo 让 service.firstTimeLogin 在事务内调到注入方法
// 时触发 InnoDB ROLLBACK 真实行为，验证多表事务原子性 —— **不是** "未到达" 而是
// 真实 undo log 回滚。
//
// 设计决策：
//   - **不**用 testify mock / gomock：与 4.5 / 4.6 / 4.8 同模式（显式 stub struct +
//     函数字段）；编译期检查 + 跨平台无依赖
//   - **必须实装完整 interface 全部方法**：编译期保证 service 注入不破，UpdateNickname /
//     FindByID 等非注入方法透传给 delegate（防止 service 层在 reuseLogin 路径或
//     事务后查询时调用未实装方法 panic）
// ============================================================

// faultUserRepo 包装真实 UserRepo，让 Create 抛 injectErr，其他方法透传。
//
// 用于 AC4: 回滚 3（user repo 第 1 步失败 → 什么都没创建）。
//
// **必须实装完整 UserRepo 全部 3 方法**：Create 注入 fault；UpdateNickname / FindByID
// 透传 delegate。即使 AC4 case 不会调到 UpdateNickname / FindByID（事务在 Create
// 即失败 → 后续 reuseLogin 也不会触发），编译期 interface 必须满足。
type faultUserRepo struct {
	delegate  mysql.UserRepo
	injectErr error
}

func (f *faultUserRepo) Create(ctx context.Context, u *mysql.User) error {
	return f.injectErr
}

func (f *faultUserRepo) UpdateNickname(ctx context.Context, userID uint64, nickname string) error {
	return f.delegate.UpdateNickname(ctx, userID, nickname)
}

func (f *faultUserRepo) FindByID(ctx context.Context, id uint64) (*mysql.User, error) {
	return f.delegate.FindByID(ctx, id)
}

// faultPetRepo 包装真实 PetRepo，让 Create 抛 injectErr，FindDefaultByUserID 透传。
//
// 用于 AC2: 回滚 1（pet repo 第 4 步失败 → 整体回滚）。
type faultPetRepo struct {
	delegate  mysql.PetRepo
	injectErr error
}

func (f *faultPetRepo) Create(ctx context.Context, p *mysql.Pet) error {
	return f.injectErr
}

func (f *faultPetRepo) FindDefaultByUserID(ctx context.Context, userID uint64) (*mysql.Pet, error) {
	return f.delegate.FindDefaultByUserID(ctx, userID)
}

// faultChestRepo 包装真实 ChestRepo，让 Create 抛 injectErr，FindByUserID 透传。
//
// 用于 AC3: 回滚 2（chest repo 第 6 步失败 → 前 5 步全部回滚）。
type faultChestRepo struct {
	delegate  mysql.ChestRepo
	injectErr error
}

func (f *faultChestRepo) Create(ctx context.Context, c *mysql.UserChest) error {
	return f.injectErr
}

func (f *faultChestRepo) FindByUserID(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
	return f.delegate.FindByUserID(ctx, userID)
}

// queryCount 返 SELECT COUNT(*) FROM <table>（不带 WHERE 子句的简写）。
//
// 与 assertCount 不同：assertCount 同时断言 + 失败 t.Errorf；queryCount 只查不断言，
// 用于 AC9 重入 case 的"前后快照对比"模式（取第一次调用后行数 → 第二次调用后再取 →
// 两次必须相等）。
//
// 失败 → t.Fatalf（与 assertCount 内部 SQL 错误处理一致）。
func queryCount(t *testing.T, sqlDB *sql.DB, table string) int64 {
	t.Helper()
	var n int64
	if err := sqlDB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("queryCount %s: %v", table, err)
	}
	return n
}

// ============================================================
// AC2: 回滚 1 — pet repo 第 4 步失败 → 5 表全空（事务原子性）
//
// fn 在第 4 步（pets INSERT）失败：前 3 步（users INSERT + UpdateNickname + binding
// INSERT）已 INSERT 但未 commit；ROLLBACK 后 5 表全空。
//
// 关键断言：
//   - service 必须返 *AppError code=ErrServiceBusy(1009)（事务回滚后包装的兜底）
//   - errors.As 解包穿透 wrap 链拿到 *AppError
//   - 5 张表全部 count=0（不只是 pets/step/chest 没到达 —— users/binding 也被 rollback）
// ============================================================
func TestAuthService_GuestLogin_PetRepoFailsTx_AllRowsRollback(t *testing.T) {
	dsn, dockerCleanup := startMySQL(t)
	defer dockerCleanup()
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
		t.Fatalf("db.Open: %v", err)
	}

	rawDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("gormDB.DB(): %v", err)
	}
	defer rawDB.Close()

	signer, err := auth.New("test-secret-must-be-at-least-16-bytes", 3600)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}

	// 用真实 4 个 mysql repo + faultPetRepo（仅在 PetRepo.Create 注入 fault）。
	// **关键**：必须真实 repo（非 stub）—— 经过真实 InnoDB 事务才能验证 rollback 真行为。
	txMgr := tx.NewManager(gormDB)
	userRepo := mysql.NewUserRepo(gormDB)
	bindingRepo := mysql.NewAuthBindingRepo(gormDB)
	stepRepo := mysql.NewStepAccountRepo(gormDB)
	chestRepo := mysql.NewChestRepo(gormDB)
	petRepoFault := &faultPetRepo{
		delegate:  mysql.NewPetRepo(gormDB),
		injectErr: stderrors.New("synthetic pet repo failure"), // 非 sentinel → service 走 1009 默认分支
	}

	svc := service.NewAuthService(txMgr, signer, userRepo, bindingRepo, petRepoFault, stepRepo, chestRepo)

	_, err = svc.GuestLogin(context.Background(), service.GuestLoginInput{
		GuestUID:    "uid-rollback-pet",
		Platform:    "ios",
		AppVersion:  "1.0.0",
		DeviceModel: "iPhone15,2",
	})

	// service 必须返 *AppError code=1009（事务回滚后包装 ErrServiceBusy）
	if err == nil {
		t.Fatalf("expected error, got nil (rollback path 没触发)")
	}
	var appErr *apperror.AppError
	if !stderrors.As(err, &appErr) {
		t.Fatalf("expected *AppError, got %T: %v", err, err)
	}
	if appErr.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d (ErrServiceBusy)", appErr.Code, apperror.ErrServiceBusy)
	}

	// **核心断言**：5 张表全部为空（前 2 步 INSERT users + binding 都被 rollback；
	// pets / step / chest 没到达 —— 全部 count=0 验证事务原子性，不是部分降级）
	assertCount(t, rawDB, "users", nil, 0, "users (rollback)")
	assertCount(t, rawDB, "user_auth_bindings", nil, 0, "bindings (rollback)")
	assertCount(t, rawDB, "pets", nil, 0, "pets (not reached)")
	assertCount(t, rawDB, "user_step_accounts", nil, 0, "step_accounts (not reached)")
	assertCount(t, rawDB, "user_chests", nil, 0, "chests (not reached)")
}

// ============================================================
// AC3: 回滚 2 — chest repo 第 6 步失败 → 5 表全空（事务最末步失败 rollback）
//
// 与 AC2 的差异：AC2 fn 在第 4 步失败（前 3 步已 INSERT），AC3 fn 在第 6 步失败
// （前 5 步都已 INSERT 但未 commit），InnoDB undo log 完整 rollback 5 表 INSERT。
// ============================================================
func TestAuthService_GuestLogin_ChestRepoFailsTx_AllRowsRollback(t *testing.T) {
	dsn, dockerCleanup := startMySQL(t)
	defer dockerCleanup()
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
		t.Fatalf("db.Open: %v", err)
	}

	rawDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("gormDB.DB(): %v", err)
	}
	defer rawDB.Close()

	signer, err := auth.New("test-secret-must-be-at-least-16-bytes", 3600)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}

	txMgr := tx.NewManager(gormDB)
	userRepo := mysql.NewUserRepo(gormDB)
	bindingRepo := mysql.NewAuthBindingRepo(gormDB)
	petRepo := mysql.NewPetRepo(gormDB)
	stepRepo := mysql.NewStepAccountRepo(gormDB)
	chestRepoFault := &faultChestRepo{
		delegate:  mysql.NewChestRepo(gormDB),
		injectErr: stderrors.New("synthetic chest repo failure"),
	}

	svc := service.NewAuthService(txMgr, signer, userRepo, bindingRepo, petRepo, stepRepo, chestRepoFault)

	_, err = svc.GuestLogin(context.Background(), service.GuestLoginInput{
		GuestUID:    "uid-rollback-chest",
		Platform:    "ios",
		AppVersion:  "1.0.0",
		DeviceModel: "iPhone15,2",
	})

	if err == nil {
		t.Fatalf("expected error, got nil (rollback path 没触发)")
	}
	var appErr *apperror.AppError
	if !stderrors.As(err, &appErr) {
		t.Fatalf("expected *AppError, got %T: %v", err, err)
	}
	if appErr.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d (ErrServiceBusy)", appErr.Code, apperror.ErrServiceBusy)
	}

	// **核心断言**：5 张表全部为空（前 5 步 INSERT 都已发生但 ROLLBACK 完整撤销）
	assertCount(t, rawDB, "users", nil, 0, "users (rollback)")
	assertCount(t, rawDB, "user_auth_bindings", nil, 0, "bindings (rollback)")
	assertCount(t, rawDB, "pets", nil, 0, "pets (rollback)")
	assertCount(t, rawDB, "user_step_accounts", nil, 0, "step_accounts (rollback)")
	assertCount(t, rawDB, "user_chests", nil, 0, "chests (not reached)")
}

// ============================================================
// AC4: 回滚 3 — user repo 第 1 步失败 → 5 表全空（无任何 INSERT 发生）
//
// 与 AC2/AC3 的差异：第 1 步就失败 → 没有任何 INSERT 发生（不是"已 INSERT 后 rollback"
// 而是"INSERT 都没 issue"）；rawDB 5 表全空源自"未到达"而非 rollback。测试不区分两种
// 语义来源，只断言 DB 状态最终一致。
// ============================================================
func TestAuthService_GuestLogin_UserRepoFailsTx_NoRowsCreated(t *testing.T) {
	dsn, dockerCleanup := startMySQL(t)
	defer dockerCleanup()
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
		t.Fatalf("db.Open: %v", err)
	}

	rawDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("gormDB.DB(): %v", err)
	}
	defer rawDB.Close()

	signer, err := auth.New("test-secret-must-be-at-least-16-bytes", 3600)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}

	txMgr := tx.NewManager(gormDB)
	userRepoFault := &faultUserRepo{
		delegate:  mysql.NewUserRepo(gormDB),
		injectErr: stderrors.New("synthetic user repo failure"),
	}
	bindingRepo := mysql.NewAuthBindingRepo(gormDB)
	petRepo := mysql.NewPetRepo(gormDB)
	stepRepo := mysql.NewStepAccountRepo(gormDB)
	chestRepo := mysql.NewChestRepo(gormDB)

	svc := service.NewAuthService(txMgr, signer, userRepoFault, bindingRepo, petRepo, stepRepo, chestRepo)

	_, err = svc.GuestLogin(context.Background(), service.GuestLoginInput{
		GuestUID:    "uid-rollback-user",
		Platform:    "ios",
		AppVersion:  "1.0.0",
		DeviceModel: "iPhone15,2",
	})

	if err == nil {
		t.Fatalf("expected error, got nil (rollback path 没触发)")
	}
	var appErr *apperror.AppError
	if !stderrors.As(err, &appErr) {
		t.Fatalf("expected *AppError, got %T: %v", err, err)
	}
	if appErr.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d (ErrServiceBusy)", appErr.Code, apperror.ErrServiceBusy)
	}

	// **核心断言**：5 张表全部为空（第 1 步即失败 → 后续未到达；DB 最终状态一致）
	assertCount(t, rawDB, "users", nil, 0, "users (not reached)")
	assertCount(t, rawDB, "user_auth_bindings", nil, 0, "bindings (not reached)")
	assertCount(t, rawDB, "pets", nil, 0, "pets (not reached)")
	assertCount(t, rawDB, "user_step_accounts", nil, 0, "step_accounts (not reached)")
	assertCount(t, rawDB, "user_chests", nil, 0, "chests (not reached)")
}

// ============================================================
// AC5: 并发 1 — 100 goroutine 同 guestUid → 1 user + 全部拿到同 user_id
//
// 把 4.6 已有 2 goroutine race 测试推到 100 goroutine 高负载，验证：
//   - InnoDB insert intent gap lock 路径与 service-level reuseLogin fallback 路径
//     在高强度并发下都能 race coverage matrix 全展开穷举
//   - 任一 goroutine 拿到 1009 → 立即 fail（review-r1 lesson 在 100 规模回归）
//   - DB 最终 1 行 user / binding / pet / step / chest（自然幂等通过 UNIQUE INDEX）
//
// N=100 必须（epics.md 行 1098 钦定）；不能减少。
// ============================================================
func TestAuthService_GuestLogin_Concurrent100SameGuestUID_OnlyOneUser(t *testing.T) {
	svc, sqlDB, cleanup := buildAuthService(t)
	defer cleanup()

	const guestUID = "uid-concurrent-100-same"
	const N = 100
	type result struct {
		userID uint64
		err    error
	}
	results := make([]result, N)

	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			out, err := svc.GuestLogin(context.Background(), service.GuestLoginInput{
				GuestUID:    guestUID,
				Platform:    "ios",
				AppVersion:  "1.0.0",
				DeviceModel: "iPhone15,2",
			})
			if err != nil {
				results[i] = result{err: err}
				return
			}
			results[i] = result{userID: out.UserID}
		}()
	}
	wg.Wait()

	// 断言 1：所有 100 goroutine 都成功 + 全部拿到同一 user_id
	var firstUserID uint64
	for i, r := range results {
		if r.err != nil {
			t.Fatalf("goroutine %d returned error %v (race 路径必须 reuseLogin 而非 1009)", i, r.err)
		}
		if i == 0 {
			firstUserID = r.userID
		} else if r.userID != firstUserID {
			t.Errorf("goroutine %d userID=%d ≠ first userID=%d (违反幂等)", i, r.userID, firstUserID)
		}
	}

	// 断言 2：DB 最终状态 5 张表各 1 行
	assertCount(t, sqlDB, "users WHERE guest_uid=?", []any{guestUID}, 1, "users (only 1)")
	assertCount(t, sqlDB, "user_auth_bindings WHERE auth_type=1 AND auth_identifier=?", []any{guestUID}, 1, "bindings (only 1)")
	assertCount(t, sqlDB, "pets WHERE user_id=?", []any{firstUserID}, 1, "pets (only 1)")
	assertCount(t, sqlDB, "user_step_accounts WHERE user_id=?", []any{firstUserID}, 1, "step (only 1)")
	assertCount(t, sqlDB, "user_chests WHERE user_id=?", []any{firstUserID}, 1, "chests (only 1)")
}

// ============================================================
// AC6: 并发 2 — 100 goroutine 100 不同 guestUid → 100 独立 user + 无串数据
//
// "无串数据"语义：100 个 goroutine 并发各创建自己的 user，必须保证 user_A 的
// binding 不会指向 user_B / user_A 的 pet 不会有 user_B 的 user_id —— 即每个 user
// 的 4 行关联（binding / pet / step / chest）都和 user.id 一一对应。
//
// 验证 NFR1 资产事务原子的"无跨 user 数据污染"维度（事务隔离性）。
//
// N=100 必须（epics.md 行 1099 钦定）。
// ============================================================
func TestAuthService_GuestLogin_Concurrent100DifferentGuestUIDs_NoCrossData(t *testing.T) {
	svc, sqlDB, cleanup := buildAuthService(t)
	defer cleanup()

	const N = 100
	type result struct {
		guestUID string
		userID   uint64
		petID    uint64
		err      error
	}
	results := make([]result, N)

	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		guestUID := fmt.Sprintf("uid-diff-%03d", i) // %03d 永远 11 字符（uid-diff-099 / uid-diff-100 长度一致）
		go func() {
			defer wg.Done()
			out, err := svc.GuestLogin(context.Background(), service.GuestLoginInput{
				GuestUID:    guestUID,
				Platform:    "ios",
				AppVersion:  "1.0.0",
				DeviceModel: "iPhone15,2",
			})
			if err != nil {
				results[i] = result{guestUID: guestUID, err: err}
				return
			}
			results[i] = result{guestUID: guestUID, userID: out.UserID, petID: out.PetID}
		}()
	}
	wg.Wait()

	// 断言 1：所有 100 都成功 + user_id / pet_id 互不相同
	seenUIDs := make(map[uint64]string, N)
	seenPetIDs := make(map[uint64]struct{}, N)
	for i, r := range results {
		if r.err != nil {
			t.Fatalf("goroutine %d guestUID=%s err=%v", i, r.guestUID, r.err)
		}
		if existing, dup := seenUIDs[r.userID]; dup {
			t.Errorf("userID %d collision: guestUID %s ↔ %s", r.userID, r.guestUID, existing)
		}
		seenUIDs[r.userID] = r.guestUID
		if _, dup := seenPetIDs[r.petID]; dup {
			t.Errorf("petID %d collision (guestUID %s)", r.petID, r.guestUID)
		}
		seenPetIDs[r.petID] = struct{}{}
	}

	// 断言 2：DB 总计 5 张表各 100 行
	assertCount(t, sqlDB, "users", nil, N, "users (100 distinct)")
	assertCount(t, sqlDB, "user_auth_bindings", nil, N, "bindings (100 distinct)")
	assertCount(t, sqlDB, "pets", nil, N, "pets (100 distinct)")
	assertCount(t, sqlDB, "user_step_accounts", nil, N, "step (100 distinct)")
	assertCount(t, sqlDB, "user_chests", nil, N, "chests (100 distinct)")

	// 断言 3：每个 user 的关联数据完整（核心 —— 验证"无串数据"）
	for _, r := range results {
		assertCount(t, sqlDB, "users WHERE id=? AND guest_uid=?", []any{r.userID, r.guestUID}, 1, "user "+r.guestUID)
		assertCount(t, sqlDB, "user_auth_bindings WHERE user_id=? AND auth_identifier=?", []any{r.userID, r.guestUID}, 1, "binding "+r.guestUID)
		assertCount(t, sqlDB, "pets WHERE user_id=? AND is_default=1", []any{r.userID}, 1, "pet "+r.guestUID)
		assertCount(t, sqlDB, "user_step_accounts WHERE user_id=?", []any{r.userID}, 1, "step "+r.guestUID)
		assertCount(t, sqlDB, "user_chests WHERE user_id=?", []any{r.userID}, 1, "chest "+r.guestUID)
	}
}

// ============================================================
// AC7: 边界 1 — guestUid 长度 128（最大允许）→ 成功 + 完整存储
//
// service 层入口（不走 handler）：epics.md 行 1100 钦定 service / repo / DB 链路
// 在 128 字符下不退化。验证：
//   - svc.GuestLogin 不返 err
//   - 5 张表各 1 行
//   - SELECT guest_uid FROM users 完整存储（防 schema 字段长度配错导致 MySQL 静默截断）
//
// 用 ASCII（每字符 1 字节）—— utf8mb4 多字节字符与 VARCHAR(N) "N 是字符还是字节" 的歧义
// 由 future epic fuzz 覆盖。
// ============================================================
func TestAuthService_GuestLogin_GuestUIDExactly128Chars_Succeeds(t *testing.T) {
	svc, sqlDB, cleanup := buildAuthService(t)
	defer cleanup()

	guestUID128 := strings.Repeat("a", 128)
	if utf8.RuneCountInString(guestUID128) != 128 {
		t.Fatalf("setup error: guestUID rune count = %d, want 128", utf8.RuneCountInString(guestUID128))
	}

	out, err := svc.GuestLogin(context.Background(), service.GuestLoginInput{
		GuestUID:    guestUID128,
		Platform:    "ios",
		AppVersion:  "1.0.0",
		DeviceModel: "iPhone15,2",
	})
	if err != nil {
		t.Fatalf("expected success at 128 chars boundary, got err=%v", err)
	}

	// 断言 5 张表各 1 行 + guestUID 完整存储
	assertCount(t, sqlDB, "users WHERE guest_uid=?", []any{guestUID128}, 1, "users (128 char)")
	assertCount(t, sqlDB, "user_auth_bindings WHERE auth_type=1 AND auth_identifier=?", []any{guestUID128}, 1, "binding (128 char)")
	assertCount(t, sqlDB, "pets WHERE user_id=?", []any{out.UserID}, 1, "pet (128 char)")
	assertCount(t, sqlDB, "user_step_accounts WHERE user_id=?", []any{out.UserID}, 1, "step (128 char)")
	assertCount(t, sqlDB, "user_chests WHERE user_id=?", []any{out.UserID}, 1, "chest (128 char)")

	// 验证 guestUid 字段完整存储（防 MySQL 静默截断到 127 / 64）
	var stored string
	if err := sqlDB.QueryRow("SELECT guest_uid FROM users WHERE id=?", out.UserID).Scan(&stored); err != nil {
		t.Fatalf("query stored guest_uid: %v", err)
	}
	if stored != guestUID128 {
		t.Errorf("stored guestUid len=%d, want 128 (full storage; MySQL VARCHAR 静默截断回归)", len(stored))
	}
}

// ============================================================
// AC9: 重入 — 同 guestUid 第二次走 reuseLogin → DB 行数完全不变 + token 不同
//
// 与 4.6 SameGuestUID 测试的差异（深层 reuseLogin 语义验证）：
//   - 4.6 用 1=1 硬断言 count（仅验证最终状态）
//   - 本 case 用前后快照对比（initialUserCount → 第二次调用 → 验证未变）
//   - 4.6 不断言 token 不同 / nickname 一致 —— 本 case 三重断言
//
// V1 §4.1 行 168 钦定 "重入登录 → 新 token"（HS256 + iat / exp 时间戳）
// reuseLogin 调 signer.Sign(user.ID, 0) 必返新 token。
// ============================================================
func TestAuthService_GuestLogin_ReentryAfterSuccess_ReusesWithoutNewRows(t *testing.T) {
	svc, sqlDB, cleanup := buildAuthService(t)
	defer cleanup()

	const guestUID = "uid-reentry"
	input := service.GuestLoginInput{
		GuestUID:    guestUID,
		Platform:    "ios",
		AppVersion:  "1.0.0",
		DeviceModel: "iPhone15,2",
	}

	// 第一次调用：走 firstTimeLogin → 5 行入库
	out1, err := svc.GuestLogin(context.Background(), input)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if out1.Token == "" {
		t.Errorf("first token empty")
	}

	// 第一次调用后的初始 DB 状态快照
	initialUserCount := queryCount(t, sqlDB, "users")
	initialBindingCount := queryCount(t, sqlDB, "user_auth_bindings")
	initialPetCount := queryCount(t, sqlDB, "pets")
	initialStepCount := queryCount(t, sqlDB, "user_step_accounts")
	initialChestCount := queryCount(t, sqlDB, "user_chests")
	if initialUserCount != 1 {
		t.Fatalf("post-first-call users count = %d, want 1", initialUserCount)
	}

	// HS256 token 的 iat / exp 取 time.Now().Unix() 是秒级精度。如果第二次调用与
	// 第一次发生在同一秒内，token claims 完全相同 → token 字符串相等。reuseLogin
	// 每次签新 token 的语义靠时间差驱动；为避免 flaky，sleep 足够长（>1s）让秒级
	// 时间戳必然变化。1100ms 在容器化 dockertest case 中可接受（单 case 总开销
	// ~30s 容器启动；额外 1s 占比 ~3%）。
	time.Sleep(1100 * time.Millisecond)

	// 第二次调用（同 guestUid）：必须走 reuseLogin → DB 行数不变
	out2, err := svc.GuestLogin(context.Background(), input)
	if err != nil {
		t.Fatalf("second call (reentry): %v", err)
	}

	// 断言 1：返回 user_id 一致
	if out1.UserID != out2.UserID {
		t.Errorf("reentry UserID changed: %d → %d (违反 reuseLogin 语义)", out1.UserID, out2.UserID)
	}
	// 断言 2：返回 pet_id 一致（reuseLogin 加载同一只默认猫）
	if out1.PetID != out2.PetID {
		t.Errorf("reentry PetID changed: %d → %d", out1.PetID, out2.PetID)
	}
	// 断言 3：返回 nickname 一致（reuseLogin 不重写 nickname）
	if out1.Nickname != out2.Nickname {
		t.Errorf("reentry Nickname changed: %q → %q", out1.Nickname, out2.Nickname)
	}
	// 断言 4：token 是新签发的（V1 §4.1 钦定 reuseLogin 每次签新 token）
	if out1.Token == out2.Token {
		t.Errorf("reentry Token == first Token (reuseLogin 应每次签新 token)")
	}
	if out2.Token == "" {
		t.Errorf("second token empty")
	}

	// 断言 5：DB 5 张表行数完全不变（reuseLogin 不写入任何表 —— 快照前后相等）
	if c := queryCount(t, sqlDB, "users"); c != initialUserCount {
		t.Errorf("post-reentry users count = %d, want %d (no new row)", c, initialUserCount)
	}
	if c := queryCount(t, sqlDB, "user_auth_bindings"); c != initialBindingCount {
		t.Errorf("post-reentry bindings count = %d, want %d", c, initialBindingCount)
	}
	if c := queryCount(t, sqlDB, "pets"); c != initialPetCount {
		t.Errorf("post-reentry pets count = %d, want %d", c, initialPetCount)
	}
	if c := queryCount(t, sqlDB, "user_step_accounts"); c != initialStepCount {
		t.Errorf("post-reentry step count = %d, want %d", c, initialStepCount)
	}
	if c := queryCount(t, sqlDB, "user_chests"); c != initialChestCount {
		t.Errorf("post-reentry chest count = %d, want %d", c, initialChestCount)
	}
}
