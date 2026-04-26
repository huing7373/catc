package cli

import (
	"context"
	"errors"
	"io"
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

// TestRunMigrate_NilCfg 验证 RunMigrate 对 nil cfg fail-fast。
func TestRunMigrate_NilCfg(t *testing.T) {
	err := RunMigrate(context.Background(), nil, []string{"up"})
	if err == nil {
		t.Fatalf("RunMigrate(nil cfg): got nil, want error")
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
