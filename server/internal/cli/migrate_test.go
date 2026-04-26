package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/huing/cat/server/internal/infra/config"
)

// fakeMigrator 是 cli.Migrator 的 fake 实装，单测注入用。
//
// 字段语义：
//   - upErr / downErr：Up / Down 调用时返回的 error
//   - statusVersion / statusDirty / statusErr：Status 返回的三元组
//   - upCalls / downCalls / statusCalls / closeCalls：调用计数（验证 dispatcher 行为）
type fakeMigrator struct {
	upErr         error
	downErr       error
	statusVersion uint
	statusDirty   bool
	statusErr     error

	upCalls     int
	downCalls   int
	statusCalls int
	closeCalls  int
}

func (f *fakeMigrator) Up(ctx context.Context) error {
	f.upCalls++
	return f.upErr
}

func (f *fakeMigrator) Down(ctx context.Context) error {
	f.downCalls++
	return f.downErr
}

func (f *fakeMigrator) Status(ctx context.Context) (uint, bool, error) {
	f.statusCalls++
	return f.statusVersion, f.statusDirty, f.statusErr
}

func (f *fakeMigrator) Close() error {
	f.closeCalls++
	return nil
}

// TestRunMigrateAction_UpSuccess 验证 action=up 路径分发到 Migrator.Up。
func TestRunMigrateAction_UpSuccess(t *testing.T) {
	f := &fakeMigrator{}
	err := runMigrateAction(context.Background(), f, "up")
	if err != nil {
		t.Errorf("runMigrateAction(up): got %v, want nil", err)
	}
	if f.upCalls != 1 {
		t.Errorf("Up calls = %d, want 1", f.upCalls)
	}
}

// TestRunMigrateAction_UpFailure 验证 Up 错误透传。
func TestRunMigrateAction_UpFailure(t *testing.T) {
	wantErr := errors.New("disk full")
	f := &fakeMigrator{upErr: wantErr}
	err := runMigrateAction(context.Background(), f, "up")
	if !errors.Is(err, wantErr) {
		t.Errorf("runMigrateAction(up): got %v, want %v", err, wantErr)
	}
}

// TestRunMigrateAction_DownSuccess 验证 action=down 路径分发到 Migrator.Down。
func TestRunMigrateAction_DownSuccess(t *testing.T) {
	f := &fakeMigrator{}
	err := runMigrateAction(context.Background(), f, "down")
	if err != nil {
		t.Errorf("runMigrateAction(down): got %v, want nil", err)
	}
	if f.downCalls != 1 {
		t.Errorf("Down calls = %d, want 1", f.downCalls)
	}
}

// TestRunMigrateAction_StatusClean 验证 status 干净路径返 nil。
func TestRunMigrateAction_StatusClean(t *testing.T) {
	f := &fakeMigrator{statusVersion: 5, statusDirty: false}
	err := runMigrateAction(context.Background(), f, "status")
	if err != nil {
		t.Errorf("runMigrateAction(status, clean): got %v, want nil", err)
	}
	if f.statusCalls != 1 {
		t.Errorf("Status calls = %d, want 1", f.statusCalls)
	}
}

// TestRunMigrateAction_StatusDirty 验证 dirty 状态返 error 让 CI 感知。
func TestRunMigrateAction_StatusDirty(t *testing.T) {
	f := &fakeMigrator{statusVersion: 3, statusDirty: true}
	err := runMigrateAction(context.Background(), f, "status")
	if err == nil {
		t.Fatalf("runMigrateAction(status, dirty): got nil, want error")
	}
	if !strings.Contains(err.Error(), "dirty") {
		t.Errorf("error message %q does not contain 'dirty'", err.Error())
	}
}

// TestRunMigrateAction_StatusError 验证 Status 自身错误透传。
func TestRunMigrateAction_StatusError(t *testing.T) {
	wantErr := errors.New("connection lost")
	f := &fakeMigrator{statusErr: wantErr}
	err := runMigrateAction(context.Background(), f, "status")
	if !errors.Is(err, wantErr) {
		t.Errorf("runMigrateAction(status, err): got %v, want %v", err, wantErr)
	}
}

// TestRunMigrateAction_UnknownAction 验证未知 action 返 error。
func TestRunMigrateAction_UnknownAction(t *testing.T) {
	f := &fakeMigrator{}
	err := runMigrateAction(context.Background(), f, "foo")
	if err == nil {
		t.Fatalf("runMigrateAction(foo): got nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error message %q does not contain 'unknown'", err.Error())
	}
	// 未知 action 不应触发任何 dispatch 调用
	if f.upCalls+f.downCalls+f.statusCalls != 0 {
		t.Errorf("unknown action triggered %d dispatch calls, want 0", f.upCalls+f.downCalls+f.statusCalls)
	}
}

// TestRunMigrate_NilCfgNoOverrideFallsBackToLocate 验证 RunMigrate(nil, action) 走
// LocateDefault 路径 —— 测试 cwd 是 internal/cli/，server/configs/local.yaml
// 不存在，所以 LocateDefault 失败 → RunMigrate 返 error（review round 2 修法：
// 不再对 nil cfg fail-fast，而是 lazy locate；找不到时才报错）。
func TestRunMigrate_NilCfgNoOverrideFallsBackToLocate(t *testing.T) {
	err := RunMigrate(context.Background(), nil, []string{"up"})
	if err == nil {
		t.Fatalf("RunMigrate(nil cfg, no -config): got nil, want error from LocateDefault")
	}
	if !strings.Contains(err.Error(), "locate default config") && !strings.Contains(err.Error(), "no config file found") {
		t.Errorf("expected LocateDefault failure, got %v", err)
	}
}

// TestRunMigrate_NilCfgWithBadConfigOverride 验证 RunMigrate(nil, "-config bad.yaml ...")
// 不会先走 LocateDefault（避免 main 流程 bug：CI 只 ship dev.yaml 的场景必须直接尝试
// Load 用户给的 path，而不是因为 local.yaml 不存在就先 fail）。
//
// review round 2 Finding 1 的核心场景：cfg=nil + 显式 -config → 直接走 -config 路径。
func TestRunMigrate_NilCfgWithBadConfigOverride(t *testing.T) {
	err := RunMigrate(context.Background(), nil, []string{"-config", "/nonexistent/path/bad.yaml", "up"})
	if err == nil {
		t.Fatalf("RunMigrate(nil, -config bad): got nil, want error from Load override")
	}
	// 关键：错误必须来自 Load -config 路径，而不是 LocateDefault
	if !strings.Contains(err.Error(), "load -config") {
		t.Errorf("expected error from -config override Load (not LocateDefault), got %v", err)
	}
}

// TestRunMigrate_EmptyArgs 验证 RunMigrate 对空 args 返 error。
func TestRunMigrate_EmptyArgs(t *testing.T) {
	// cfg 用一个非 nil 但内容不重要的实例（不会走到 New）
	cfg := &config.Config{}
	err := RunMigrate(context.Background(), cfg, []string{})
	if err == nil {
		t.Fatalf("RunMigrate([]): got nil, want error")
	}
	if !strings.Contains(err.Error(), "action") {
		t.Errorf("error message %q does not mention 'action'", err.Error())
	}
}

// TestParseMigrateArgs_PlainAction 验证最简形态：`migrate up`（"migrate" 已被 main 剥掉，
// args 仅含 ["up"]）。
func TestParseMigrateArgs_PlainAction(t *testing.T) {
	action, override, err := ParseMigrateArgs([]string{"up"}, io.Discard)
	if err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	if action != "up" {
		t.Errorf("action = %q, want \"up\"", action)
	}
	if override != "" {
		t.Errorf("override = %q, want empty", override)
	}
}

// TestParseMigrateArgs_ConfigAfterAction 验证文档化的关键形态：
// `migrate up -config configs/dev.yaml`（"-config" 在 action **后面**）。
//
// 这是 codex review Finding 1 的核心场景 —— 旧实装直接走 main flag.Parse 会丢掉这个 -config。
func TestParseMigrateArgs_ConfigAfterAction(t *testing.T) {
	// 注意：Go flag 包要求 flag 在 positional args 之前（Parse 在第一个非 flag 处停）
	// 所以 `migrate up -config X` 在 ParseMigrateArgs 内会失败 —— 我们必须把 args 重排
	// 或者支持任意顺序。Go 的 flag 包**不**支持 flag 出现在 positional 之后。
	//
	// 解决方案：ParseMigrateArgs 内部把 args 拆成 (positional, flagPart) 两部分再 Parse。
	// 测试期待的是这条调用形态最终能跑通。
	action, override, err := ParseMigrateArgs([]string{"up", "-config", "configs/dev.yaml"}, io.Discard)
	if err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	if action != "up" {
		t.Errorf("action = %q, want \"up\"", action)
	}
	if override != "configs/dev.yaml" {
		t.Errorf("override = %q, want \"configs/dev.yaml\"", override)
	}
}

// TestParseMigrateArgs_ConfigBeforeAction 验证另一种形态：
// `migrate -config configs/dev.yaml up`（flag 在 action 前面，符合 Go flag 包默认期望）。
func TestParseMigrateArgs_ConfigBeforeAction(t *testing.T) {
	action, override, err := ParseMigrateArgs([]string{"-config", "configs/dev.yaml", "up"}, io.Discard)
	if err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	if action != "up" {
		t.Errorf("action = %q, want \"up\"", action)
	}
	if override != "configs/dev.yaml" {
		t.Errorf("override = %q, want \"configs/dev.yaml\"", override)
	}
}

// TestParseMigrateArgs_NoArgs 验证空 args 返 error。
func TestParseMigrateArgs_NoArgs(t *testing.T) {
	_, _, err := ParseMigrateArgs(nil, io.Discard)
	if err == nil {
		t.Fatal("got nil, want error")
	}
	if !strings.Contains(err.Error(), "action") {
		t.Errorf("error %q does not mention 'action'", err.Error())
	}
}

// TestParseMigrateArgs_TooManyPositional 验证多余 positional args 返 error
// （避免 `migrate up extra-junk` 这种调用静默成功）。
func TestParseMigrateArgs_TooManyPositional(t *testing.T) {
	_, _, err := ParseMigrateArgs([]string{"up", "extra"}, io.Discard)
	if err == nil {
		t.Fatal("got nil, want error")
	}
}

// TestParseMigrateArgs_UnknownFlag 验证未知 flag 报错（FlagSet ContinueOnError 模式下
// 不会 os.Exit，而是返 error）。
func TestParseMigrateArgs_UnknownFlag(t *testing.T) {
	_, _, err := ParseMigrateArgs([]string{"-unknown", "up"}, io.Discard)
	if err == nil {
		t.Fatal("got nil, want error")
	}
}

// TestLocateMigrations_CwdHasMigrations 验证 cwd 下有 migrations 目录（dev 在 server/）
// 时 LocateMigrations 返 "migrations"（第一个候选）。
func TestLocateMigrations_CwdHasMigrations(t *testing.T) {
	tmp := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmp, "migrations"), 0o755); err != nil {
		t.Fatalf("mkdir migrations: %v", err)
	}
	chdir(t, tmp)

	got, err := LocateMigrations()
	if err != nil {
		t.Fatalf("LocateMigrations: %v", err)
	}
	if got != "migrations" {
		t.Errorf("got %q, want %q", got, "migrations")
	}
}

// TestLocateMigrations_CwdHasServerMigrations 验证 cwd 是 repo-root（有 server/migrations）
// 时 LocateMigrations 返 "server/migrations"。这是 review round 3 Finding 1 的核心场景：
// scripts/build.sh 产物在 repo-root/build/catserver，运维从 repo-root 跑 `./build/catserver migrate up`
// 时 cwd=repo-root，旧实装找 "migrations" 失败 → 现在 fallback 到 "server/migrations"。
func TestLocateMigrations_CwdHasServerMigrations(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "server", "migrations"), 0o755); err != nil {
		t.Fatalf("mkdir server/migrations: %v", err)
	}
	chdir(t, tmp)

	got, err := LocateMigrations()
	if err != nil {
		t.Fatalf("LocateMigrations: %v", err)
	}
	want := filepath.Join("server", "migrations")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestLocateMigrations_NoCandidates 验证两个候选都不存在时返带提示的 error。
func TestLocateMigrations_NoCandidates(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)

	_, err := LocateMigrations()
	if err == nil {
		t.Fatal("got nil, want error")
	}
	if !strings.Contains(err.Error(), "migrations dir not found") {
		t.Errorf("error %q does not mention 'migrations dir not found'", err.Error())
	}
	if !strings.Contains(err.Error(), "CAT_MIGRATIONS_PATH") {
		t.Errorf("error %q does not mention CAT_MIGRATIONS_PATH override hint", err.Error())
	}
}

// TestLocateMigrations_FilePrefersDir 验证 "migrations" 是文件而不是目录时跳过它，
// 找下一个候选 server/migrations（防止误把同名文件当成 migrations 目录）。
func TestLocateMigrations_FilePrefersDir(t *testing.T) {
	tmp := t.TempDir()
	// migrations 是文件，不是目录
	if err := os.WriteFile(filepath.Join(tmp, "migrations"), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("write migrations file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "server", "migrations"), 0o755); err != nil {
		t.Fatalf("mkdir server/migrations: %v", err)
	}
	chdir(t, tmp)

	got, err := LocateMigrations()
	if err != nil {
		t.Fatalf("LocateMigrations: %v", err)
	}
	want := filepath.Join("server", "migrations")
	if got != want {
		t.Errorf("got %q (probably matched a file), want %q", got, want)
	}
}

// TestLocateMigrationsIn_Empty 验证空候选列表返 error（防御性测试）。
func TestLocateMigrationsIn_Empty(t *testing.T) {
	_, err := locateMigrationsIn(nil)
	if err == nil {
		t.Fatal("got nil, want error")
	}
}

// chdir 临时切到 dir，用 t.Cleanup 在测试结束时切回。
func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir(%q): %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(prev); err != nil {
			t.Errorf("restore cwd: %v", err)
		}
	})
}
