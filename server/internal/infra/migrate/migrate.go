// Package migrate 提供 server 内部用的 migration Go API。封装 golang-migrate v4
// 让 cmd/server/main.go 子命令 + 集成测试都能直接调；不暴露 golang-migrate 的
// *Migrate 对象给上层（避免上层耦合具体工具）。
//
// # 设计来源
//
//   - ADR-0003 §3.2：钦定 golang-migrate v4 + 纯 SQL 文件双向 + CLI/Go API 双形态
//   - Story 4.3 AC2：Migrator interface（Up/Down/Status/Close）+ ctx-aware
//
// # 与 4.2 的 db.Open 解耦
//
// 本包**不**复用 4.2 的 *gorm.DB 实例。理由：
//
//  1. golang-migrate v4 期望自己的 driver instance 上 advisory lock（防并发 migrate）
//  2. migrate 子命令场景（schema 还不存在时）不能先调 db.Open（ping 会失败）
//  3. 测试 / CLI 双形态都直接 New(dsn, path) 构造，路径一致
//
// # ctx 透传策略
//
// golang-migrate v4 的 Up/Down/Version 内部不接 context.Context（API 限制），
// 同步阻塞调用。本包通过 goroutine + select ctx.Done() 的模式让 ctx cancel
// 能让 caller 提前返回；底层操作借 *Migrate.GracefulStop chan 通知 migrate
// 在下一个 statement 边界停下（不是立即停 —— 这是 gomigrate 设计：保证 schema
// 不会处于半执行状态）。
//
// 取舍：
//   - cancel 后 runWithCtx **等 done channel 实际返回**再退出（最长
//     gracefulStopTimeout = 30s）；若 grace timeout 触发会 slog.Warn 提示 schema
//     可能 dirty。这样保证 caller 拿到 ctx.Err() 时，underlying migration 要么
//     已经在 schema_migrations 写完 partial-progress + dirty=false，要么超时
//     warn —— 不会"caller 立刻 return → defer Close 把 driver 关了 → 后台 goroutine
//     失去连接 → schema_migrations 留在 dirty=true 状态"。
//   - Close 路径上 sync.Once 仍保证只调一次底层 m.Close
//
// 详见 docs/lessons/2026-04-26-cli-relative-path-and-graceful-stop-wait.md。
package migrate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	gomigrate "github.com/golang-migrate/migrate/v4"
	// 注册 mysql database driver（处理 mysql:// URL）
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	// 注册 file source driver（读 file:// URL）
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// gracefulStopTimeout 是 ctx cancel 后等待 gomigrate 在 statement 边界真停下的
// 最长时间。触发后 runWithCtx 仍返回 ctx.Err()，但允许 underlying migration goroutine
// 把当前 statement wrap up（让 schema_migrations 表的 dirty 标记保持一致）。
//
// 选 30s 是平衡：
//   - 太短（< 5s）一些大 ALTER 单语句可能跑不完，刚好 cancel 进来导致 dirty=true 锁住
//   - 太长（> 1min）违反"用户按 Ctrl+C 期望快速退出"的体感
//
// 若超过这个时间 gomigrate 仍不返回，slog.Warn 提示 schema 可能 dirty —— 是极端
// 情况（DDL 阻塞 / metadata lock 长锁），让运维介入而不是无限等。
//
// 详见 docs/lessons/2026-04-26-cli-relative-path-and-graceful-stop-wait.md。
const gracefulStopTimeout = 30 * time.Second

// Migrator 是 migration 操作的抽象，让 cli 包 / 测试可以注入 fake。
type Migrator interface {
	// Up 把 schema 推到最新。已经是最新版本时（migrate.ErrNoChange）返 nil。
	Up(ctx context.Context) error
	// Down 把 schema 全回滚（删除所有 migration）。慎用，仅 dev / test 场景。
	Down(ctx context.Context) error
	// Status 返回当前 schema 版本（uint）+ 是否 dirty。
	// 还没跑过任何 migration 时（migrate.ErrNilVersion）返 (0, false, nil)。
	Status(ctx context.Context) (version uint, dirty bool, err error)
	// Close 释放底层资源（migrate.Close 会关 source / driver 的连接）。可重入。
	Close() error
}

// migrator 是 Migrator interface 的标准实装。
type migrator struct {
	m         *gomigrate.Migrate
	closeOnce sync.Once
	closeErr  error
}

// New 构造 Migrator。
//
//   - dsn：MySQL 连接串（与 cfg.MySQL.DSN 同格式）。空 → 返 error。
//   - migrationsPath：migrations 目录路径（绝对或相对皆可）。本项目默认 "migrations"。
//
// 内部用 pathToFileURI 把 path 转成合法 file URI 后调 migrate.New(sourceURL, "mysql://"+dsn)。
// 返回的 Migrator 必须在 Close 时释放底层资源（避免连接泄漏）。
func New(dsn, migrationsPath string) (Migrator, error) {
	if dsn == "" {
		return nil, errors.New("migrate: dsn is empty")
	}
	if migrationsPath == "" {
		return nil, errors.New("migrate: migrationsPath is empty")
	}

	sourceURL, err := pathToFileURI(migrationsPath)
	if err != nil {
		return nil, fmt.Errorf("migrate.New: build file URI failed: %w", err)
	}
	databaseURL := "mysql://" + dsn

	m, err := gomigrate.New(sourceURL, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("migrate.New: open source/database failed: %w", err)
	}

	return &migrator{m: m}, nil
}

// pathToFileURI 把文件系统路径转成 golang-migrate file source 接受的 file:// URI。
//
// # 为什么需要这个函数
//
// 直接 "file://" + path 在 Windows 上炸：
//
//   - Windows 绝对路径 `C:\fork\cat\server\migrations` → `file://C:\fork\cat\server\migrations`
//     net/url.Parse 把 `C:\fork...` 当 host:port → 报 "invalid port"
//
// # 为什么不用 "file:///" 三斜杠形态
//
// 看起来 RFC 8089 §3 推荐 `file:///C:/path/to/file` 三斜杠形态，但 golang-migrate v4 的
// source/file driver 用 net/url 解析后取 `Host + Path`：
//
//   - `file:///C:/fork/cat/server/migrations`
//     → Host="", Path="/C:/fork/cat/server/migrations" → p = "/C:/fork/cat/server/migrations"
//   - golang-migrate 的 parseURL 看到 p[0]=='/' 就跳过 filepath.Abs，直接交给 os.DirFS
//   - os.DirFS("/C:/...") 在 Windows 上炸（"open .: filename, directory name, or volume
//     label syntax is incorrect"）—— 因为这不是合法的 Windows 路径
//
// # 正确做法：双斜杠形态（Host=drive, Path=rest）
//
//   - `file://C:/fork/cat/server/migrations`
//     → Host="C:", Path="/fork/cat/server/migrations" → p = "C:/fork/cat/server/migrations"
//   - p[0]='C'（不是 '/'），golang-migrate parseURL 走 filepath.Abs(p)（在 Windows 上 noop
//     因为 p 已绝对），最终 os.DirFS("C:/fork/cat/server/migrations") 正确工作
//
// # URI 元字符 escape（review round 4 P2）
//
// 直接 "file://" + slashed 在路径含 URI 元字符（`#` / `?` / 空格等）时炸：
//   - 路径 `C:\work\repo#1\migrations` → ToSlash → `C:/work/repo#1/migrations`
//     拼成 `file://C:/work/repo#1/migrations` → golang-migrate 内部用 net/url.Parse 解析 →
//     `#1/migrations` 被当 fragment → Host="C:" Path="/work/repo" → os.DirFS 找不到目录
//   - 同理 `?` 被当 query；空格也违反 URI 语法
//
// 修法：按 `/` 拆段后**逐段 url.PathEscape**，再用 `/` 拼回；这样 `/` 仍保留为路径分段符，
// 而 `#` / `?` / 空格 / 其它非保留字符按 RFC 3986 percent-encoded。Windows drive 字母
// 中的 `:` 也合法（PathEscape 不 escape `:`，net/url.Parse 解析 file://C:/... 时仍把 `C:`
// 当 Host）—— 上面"双斜杠形态"约定不变。
//
// # 实装步骤
//
//  1. filepath.Abs 把相对路径转绝对（让结果稳定，不依赖 cwd）；Unix 输入返绝对 Unix 路径
//  2. filepath.ToSlash 把 backslash 转 forward slash（Windows 必须，Unix no-op）
//  3. 按 `/` 拆段，每段过 url.PathEscape，再 `/` 拼回 → escapedPath
//  4. 拼成 "file://" + escapedPath —— Unix 得到 file:///usr/...（slashed 自身以 / 开头，自然形成三斜杠），
//     Windows 得到 file://C:/...（双斜杠 + drive 字母）
//
// 参考：RFC 8089 §2 + RFC 3986 §3.3 + golang-migrate v4 source/file/file.go::parseURL。
func pathToFileURI(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("filepath.Abs(%q): %w", p, err)
	}
	slashed := filepath.ToSlash(abs)
	// 按 `/` 拆段后逐段 escape（保留分段符 `/` 不转义，仅转义段内的 URI 元字符）
	parts := strings.Split(slashed, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	escaped := strings.Join(parts, "/")
	// 显式不加 leading / —— Unix 路径本身已以 / 开头（→ file:///usr/...），
	// Windows 路径以 drive 字母开头（→ file://C:/...，让 net/url 把 drive 当 Host）。
	// 加 leading / 在 Windows 上会让 golang-migrate 把 "/C:/..." 直接喂给 os.DirFS 而炸。
	return "file://" + escaped, nil
}

// runWithCtx 在独立 goroutine 跑 fn，并 select ctx.Done() 提前返回。
//
// 当 ctx cancel 时：
//   - 用非阻塞 send 把 GracefulStop=true 推到 *Migrate（让底层在下一个 statement
//     边界停下；gomigrate 的设计：避免半执行状态）
//   - **再等 done channel 返回**（最长 gracefulStopTimeout）—— 因为 GracefulStop
//     只是个信号，实际 stop 在 next statement 边界异步发生；这段时间 gomigrate 还
//     在持有 driver 连接、可能正在 commit schema_migrations 行。如果 caller 立刻
//     return 让 defer Close 把 driver 关了，会让后台 goroutine 失去连接 → 半执行
//     状态留在 schema_migrations（dirty=true 锁住，需手工修复）。
//   - 等到 done 或 grace timeout 后，仍返回 ctx.Err()（语义不变：caller 知道是
//     被 cancel 的，不是 fn 自己出错的）；grace timeout 触发额外 slog.Warn 提示
//     schema 可能 dirty。
//
// 接 stopSender interface 而非具体 *gomigrate.Migrate，方便单测注入 fake。
type stopSender interface {
	sendGracefulStop()
}

// realStopSender wraps *gomigrate.Migrate 暴露 GracefulStop chan。
type realStopSender struct{ m *gomigrate.Migrate }

func (r realStopSender) sendGracefulStop() {
	// 非阻塞 send：channel 容量有限，重复 cancel 不应阻塞；丢失 send 也无副作用
	// （下一次 boundary check 仍会停）
	select {
	case r.m.GracefulStop <- true:
	default:
	}
}

func runWithCtx(ctx context.Context, stop stopSender, fn func() error) error {
	return runWithCtxAndTimeout(ctx, stop, fn, gracefulStopTimeout)
}

// runWithCtxAndTimeout 是 runWithCtx 的可测试核心，参数化 graceTimeout 让单测能
// 在毫秒级验证"等 done 返回"的行为。
//
// # 早 cancel short-circuit（review round 4 P1）
//
// 如果 caller 传入的 ctx 已经 cancel / 超时（例：测试 / CLI 已经接到 SIGINT 后才调 Up），
// 必须**先**返回 ctx.Err()，**不**启动 goroutine 调 fn —— 否则 fn 已经把 SQL 发给 MySQL
// 开始改 schema，即使马上 sendGracefulStop 也已不可逆（已发出去的 DDL 一旦 server-side
// 开始执行就改不回来）。这违反 ctx-aware API 的语义："caller 显式说取消 → 不应碰 DB"。
func runWithCtxAndTimeout(ctx context.Context, stop stopSender, fn func() error, graceTimeout time.Duration) error {
	// 早 cancel short-circuit：caller 已 cancel ctx 时不调 fn、不碰 DB
	if err := ctx.Err(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		stop.sendGracefulStop()
		// ctx 已 cancel；GracefulStop 是异步信号 —— 必须等 done 实际返回再退出，
		// 否则 caller 的 defer Close 会把 driver 关了，后台 goroutine 失去连接，
		// schema_migrations 被锁在 dirty=true。
		grace := time.NewTimer(graceTimeout)
		defer grace.Stop()
		select {
		case <-done:
			// gomigrate 已在 statement 边界干净停下；done 里的 err 通常是
			// gomigrate.ErrAborted —— 但 caller 关心的是"是被 cancel 的"，
			// 所以仍返回 ctx.Err() 让上层 errors.Is(ctx.Canceled) 路径生效。
			return ctx.Err()
		case <-grace.C:
			// grace 超时：gomigrate 仍未在 statement 边界停（极端情况，
			// 比如长 ALTER / metadata lock）。slog.Warn 提示 schema 可能
			// dirty，让运维介入；caller 仍拿到 ctx.Err() 退出。
			//
			// 不再无限等 —— 长时间等没意义（caller 已 cancel）；让进程退出
			// 比卡死更好，dirty 状态下次 status 会暴露。
			slog.Warn("migrate: GracefulStop did not return within grace period, schema may be dirty",
				slog.Duration("grace_timeout", graceTimeout))
			return ctx.Err()
		}
	}
}

// Up 把 schema 推到最新。
//
// migrate.ErrNoChange（已是最新）→ 吞掉返 nil（业务语义上是"成功无操作"）。
// ctx cancel → 返 ctx.Err()，底层借 GracefulStop 在 statement 边界停下。
// 其他 error 直接 wrap 透传。
func (mg *migrator) Up(ctx context.Context) error {
	if mg.m == nil {
		return errors.New("migrate: migrator is closed or uninitialized")
	}
	err := runWithCtx(ctx, realStopSender{m: mg.m}, mg.m.Up)
	if err != nil {
		if errors.Is(err, gomigrate.ErrNoChange) {
			return nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// Down 把 schema 全回滚。
//
// migrate.ErrNoChange（已无 migration 可回滚）→ 吞掉返 nil。
// ctx cancel → 返 ctx.Err()。
func (mg *migrator) Down(ctx context.Context) error {
	if mg.m == nil {
		return errors.New("migrate: migrator is closed or uninitialized")
	}
	err := runWithCtx(ctx, realStopSender{m: mg.m}, mg.m.Down)
	if err != nil {
		if errors.Is(err, gomigrate.ErrNoChange) {
			return nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("migrate down: %w", err)
	}
	return nil
}

// Status 返回 (version, dirty, err)。
//
// migrate.ErrNilVersion（还没跑过任何 migration）→ 吞掉返 (0, false, nil)。
// ctx cancel → 返 (0, false, ctx.Err())。
//
// 注意：gomigrate.Version() 通常不阻塞（只读 schema_migrations 单行），但
// metadata lock 抢占 / 慢网络仍可能让它挂；ctx-aware 兜底统一 CLI 体验。
func (mg *migrator) Status(ctx context.Context) (uint, bool, error) {
	if mg.m == nil {
		return 0, false, errors.New("migrate: migrator is closed or uninitialized")
	}
	var version uint
	var dirty bool
	err := runWithCtx(ctx, realStopSender{m: mg.m}, func() error {
		v, d, verr := mg.m.Version()
		version, dirty = v, d
		return verr
	})
	if err != nil {
		if errors.Is(err, gomigrate.ErrNilVersion) {
			return 0, false, nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 0, false, err
		}
		return 0, false, fmt.Errorf("migrate status: %w", err)
	}
	return version, dirty, nil
}

// Close 释放底层 source / database driver。
//
// 用 sync.Once 包重入：CLI 短跑场景退出前会 Close，集成测试每个 case 都 Close，
// 重复 Close 不应 panic。即使底层 m 是 nil（New 失败的极少数路径）也 safe。
func (mg *migrator) Close() error {
	mg.closeOnce.Do(func() {
		if mg.m == nil {
			return
		}
		// migrate.Close 返回 (sourceErr, databaseErr) 两个 error；任一非 nil 都 wrap。
		sourceErr, databaseErr := mg.m.Close()
		if sourceErr != nil && databaseErr != nil {
			mg.closeErr = fmt.Errorf("migrate close: source=%v, database=%v", sourceErr, databaseErr)
			return
		}
		if sourceErr != nil {
			mg.closeErr = fmt.Errorf("migrate close source: %w", sourceErr)
			return
		}
		if databaseErr != nil {
			mg.closeErr = fmt.Errorf("migrate close database: %w", databaseErr)
			return
		}
	})
	return mg.closeErr
}
