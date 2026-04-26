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
// golang-migrate v4 的 Up/Down/Version 内部不接 context.Context（API 限制）。
// 本包公开方法保留 ctx 第一参数（CLAUDE.md "ctx 必传"），实际未透传到底层。
// 未来如需"外层 cancel 时停止 migrate"，可改为 goroutine + select ctx.Done()
// 的模式，本 story 范围内保持最小实装。
package migrate

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	gomigrate "github.com/golang-migrate/migrate/v4"
	// 注册 mysql database driver（处理 mysql:// URL）
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	// 注册 file source driver（读 file:// URL）
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

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
// # 实装步骤
//
//  1. filepath.Abs 把相对路径转绝对（让结果稳定，不依赖 cwd）；Unix 输入返绝对 Unix 路径
//  2. filepath.ToSlash 把 backslash 转 forward slash（Windows 必须，Unix no-op）
//  3. 拼成 "file://" + slashed —— Unix 得到 file:///usr/...（slashed 自身以 / 开头，自然形成三斜杠），
//     Windows 得到 file://C:/...（双斜杠 + drive 字母）
//
// 参考：RFC 8089 §2 + golang-migrate v4 source/file/file.go::parseURL。
func pathToFileURI(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("filepath.Abs(%q): %w", p, err)
	}
	slashed := filepath.ToSlash(abs)
	// 显式不加 leading / —— Unix 路径本身已以 / 开头（→ file:///usr/...），
	// Windows 路径以 drive 字母开头（→ file://C:/...，让 net/url 把 drive 当 Host）。
	// 加 leading / 在 Windows 上会让 golang-migrate 把 "/C:/..." 直接喂给 os.DirFS 而炸。
	return "file://" + slashed, nil
}

// Up 把 schema 推到最新。
//
// migrate.ErrNoChange（已是最新）→ 吞掉返 nil（业务语义上是"成功无操作"）。
// 其他 error 直接 wrap 透传。
func (mg *migrator) Up(ctx context.Context) error {
	if mg.m == nil {
		return errors.New("migrate: migrator is closed or uninitialized")
	}
	if err := mg.m.Up(); err != nil {
		if errors.Is(err, gomigrate.ErrNoChange) {
			return nil
		}
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// Down 把 schema 全回滚。
//
// migrate.ErrNoChange（已无 migration 可回滚）→ 吞掉返 nil。
func (mg *migrator) Down(ctx context.Context) error {
	if mg.m == nil {
		return errors.New("migrate: migrator is closed or uninitialized")
	}
	if err := mg.m.Down(); err != nil {
		if errors.Is(err, gomigrate.ErrNoChange) {
			return nil
		}
		return fmt.Errorf("migrate down: %w", err)
	}
	return nil
}

// Status 返回 (version, dirty, err)。
//
// migrate.ErrNilVersion（还没跑过任何 migration）→ 吞掉返 (0, false, nil)。
func (mg *migrator) Status(ctx context.Context) (uint, bool, error) {
	if mg.m == nil {
		return 0, false, errors.New("migrate: migrator is closed or uninitialized")
	}
	version, dirty, err := mg.m.Version()
	if err != nil {
		if errors.Is(err, gomigrate.ErrNilVersion) {
			return 0, false, nil
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
