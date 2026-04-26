package migrate

import (
	"context"
	"errors"
	nurl "net/url"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// nurlParse 是 net/url.Parse 的本地别名，避免 import 名冲突。
var nurlParse = nurl.Parse

// TestNew_EmptyDSN_ReturnsError 验证 New 对空 DSN fail-fast。
func TestNew_EmptyDSN_ReturnsError(t *testing.T) {
	_, err := New("", "migrations")
	if err == nil {
		t.Fatalf("New(\"\", ...) returned nil error, want error")
	}
	if !strings.Contains(err.Error(), "dsn") {
		t.Errorf("error message %q does not mention 'dsn'", err.Error())
	}
}

// TestNew_EmptyMigrationsPath_ReturnsError 验证 New 对空 path fail-fast。
func TestNew_EmptyMigrationsPath_ReturnsError(t *testing.T) {
	_, err := New("user:pass@tcp(host:3306)/db", "")
	if err == nil {
		t.Fatalf("New(..., \"\") returned nil error, want error")
	}
	if !strings.Contains(err.Error(), "migrationsPath") {
		t.Errorf("error message %q does not mention 'migrationsPath'", err.Error())
	}
}

// TestNew_InvalidMigrationsPath_ReturnsError 验证 New 对不存在的路径返 error
// （golang-migrate 的 file source driver 在 New 阶段就会 stat 路径）。
func TestNew_InvalidMigrationsPath_ReturnsError(t *testing.T) {
	// 故意用一个 platform-agnostic 的不存在路径。
	_, err := New("user:pass@tcp(host:3306)/db", "/this/path/should/not/exist/anywhere/xxx")
	if err == nil {
		t.Fatalf("New(invalid path) returned nil error, want error")
	}
	// golang-migrate 的具体 error 文本在不同版本可能不同，这里只验证 wrap prefix
	if !strings.Contains(err.Error(), "migrate.New") {
		t.Errorf("error message %q does not contain 'migrate.New' wrap prefix", err.Error())
	}
}

// TestClose_DoubleClose_NoError 验证 Close 重入安全（sync.Once 包了底层 m.Close()）。
//
// 构造 migrator 时不走 New（避免依赖真实 migrate 工具实例），直接 zero-value
// migrator{} —— m 是 nil，Close 路径走 nil 分支，sync.Once 保证 Do 内函数只跑一次。
// 重复调 Close 都返 nil。
func TestClose_DoubleClose_NoError(t *testing.T) {
	m := &migrator{m: nil, closeOnce: sync.Once{}}
	if err := m.Close(); err != nil {
		t.Errorf("first Close: got %v, want nil", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("second Close: got %v, want nil (sync.Once should guard)", err)
	}
}

// TestUp_NilMigrator_ReturnsError 验证未初始化 / 已关闭的 migrator Up 返 error
// （而非 panic）。
//
// 用 context.TODO() 而非 nil ctx：本包未来如把 ctx 透传到底层 migrate 操作，nil ctx
// 会触发 panic；TODO() 是"未确定具体 ctx 但语义合法"的标准选择，也是 SA1012 推荐做法。
func TestUp_NilMigrator_ReturnsError(t *testing.T) {
	m := &migrator{m: nil}
	err := m.Up(context.TODO())
	if err == nil {
		t.Fatalf("Up on nil m returned nil error, want error")
	}
}

// TestDown_NilMigrator_ReturnsError 同上，Down 路径。
func TestDown_NilMigrator_ReturnsError(t *testing.T) {
	m := &migrator{m: nil}
	err := m.Down(context.TODO())
	if err == nil {
		t.Fatalf("Down on nil m returned nil error, want error")
	}
}

// TestStatus_NilMigrator_ReturnsError 同上，Status 路径。
func TestStatus_NilMigrator_ReturnsError(t *testing.T) {
	m := &migrator{m: nil}
	_, _, err := m.Status(context.TODO())
	if err == nil {
		t.Fatalf("Status on nil m returned nil error, want error")
	}
}

// TestPathToFileURI_RelativePath 验证相对路径会被 filepath.Abs 转成绝对路径，
// 然后拼成合法 file URI（不依赖 cwd 是 Windows 还是 Unix —— 都应通过）。
func TestPathToFileURI_RelativePath(t *testing.T) {
	got, err := pathToFileURI("migrations")
	if err != nil {
		t.Fatalf("pathToFileURI(\"migrations\"): %v", err)
	}
	if !strings.HasPrefix(got, "file://") {
		t.Errorf("got %q, want prefix 'file://'", got)
	}
	// backslash 必须全部已转成 forward slash
	if strings.Contains(got, "\\") {
		t.Errorf("file URI %q still contains backslash", got)
	}
	rest := strings.TrimPrefix(got, "file://")
	if !strings.HasSuffix(rest, "/migrations") {
		t.Errorf("file URI %q does not end with '/migrations'", got)
	}
}

// TestPathToFileURI_UnixAbsolutePath 验证 Unix 风格绝对路径（以 / 开头）拼成
// `file:///path/...` —— 三斜杠形态（Unix 路径本身以 / 开头 + file:// 自带的双斜杠）。
func TestPathToFileURI_UnixAbsolutePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows 上 filepath.Abs("/foo/bar") 会把当前盘符前置成 "C:\foo\bar"
		// → 走 Windows 路径分支不走 Unix 分支。这条 case 在 Windows 上没意义，跳过。
		t.Skip("Unix-style absolute path test skipped on Windows (filepath.Abs prepends drive letter)")
	}
	got, err := pathToFileURI("/usr/share/migrations")
	if err != nil {
		t.Fatalf("pathToFileURI: %v", err)
	}
	want := "file:///usr/share/migrations"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestPathToFileURI_WindowsAbsolutePath 验证 Windows 风格绝对路径
// （`C:\fork\cat\server\migrations`）会被转成 `file://C:/fork/cat/server/migrations`：
//   - backslash → forward slash
//   - **不**加 leading slash（关键：让 net/url 解析为 Host="C:"，golang-migrate 走 filepath.Abs 路径）
//   - 双斜杠形态（不是三斜杠）—— 这是 golang-migrate v4 source/file driver 在 Windows 能消费的形态
//
// 详见 pathToFileURI 函数注释 "为什么不用 file:/// 三斜杠形态"。
//
// 在非 Windows 系统上，filepath.Abs 不会前置 drive 字母，所以这条 case 只在 Windows 上跑。
func TestPathToFileURI_WindowsAbsolutePath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows absolute path test only runs on Windows")
	}
	// 用 filepath.Abs 拿一个本平台合法的绝对路径作为输入，避免硬编码 C: 盘符
	abs, err := filepath.Abs("migrations")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	got, err := pathToFileURI(abs)
	if err != nil {
		t.Fatalf("pathToFileURI: %v", err)
	}
	// Windows: file://X:/.../migrations（双斜杠！不是三斜杠）
	if !strings.HasPrefix(got, "file://") {
		t.Errorf("got %q, want prefix 'file://'", got)
	}
	if strings.HasPrefix(got, "file:///") {
		t.Errorf("got %q, must NOT have triple-slash on Windows (golang-migrate parseURL would feed '/C:/...' to os.DirFS which fails)", got)
	}
	// drive 字母位置：file:// 之后立即出现 X:/
	rest := strings.TrimPrefix(got, "file://")
	if len(rest) < 3 || rest[1] != ':' || rest[2] != '/' {
		t.Errorf("file URI path part %q does not look like 'X:/...' Windows drive form", rest)
	}
	// backslash 必须全部已转成 forward slash
	if strings.Contains(got, "\\") {
		t.Errorf("file URI %q still contains backslash", got)
	}
}

// TestPathToFileURI_RoundTripViaURLParse 验证生成的 URI 通过 net/url.Parse 后，
// 配合 golang-migrate v4 source/file 的 parseURL 逻辑（`Host + Path`），最终能得到
// 一个 os.DirFS 可消费的路径。这是端到端的"URI 拼接 → URL 解析 → 路径还原"链路验证。
func TestPathToFileURI_RoundTripViaURLParse(t *testing.T) {
	uri, err := pathToFileURI("migrations")
	if err != nil {
		t.Fatalf("pathToFileURI: %v", err)
	}
	u, err := nurlParse(uri)
	if err != nil {
		t.Fatalf("net/url.Parse(%q): %v", uri, err)
	}
	// 还原 golang-migrate file source 的逻辑：p = u.Host + u.Path
	p := u.Host + u.Path
	if p == "" {
		t.Fatalf("recovered path is empty for URI %q (Host=%q Path=%q)", uri, u.Host, u.Path)
	}
	// 在 Windows 上 p 应类似 "C:/.../migrations"（不是 "/C:/..."）
	if runtime.GOOS == "windows" {
		if strings.HasPrefix(p, "/") {
			t.Errorf("Windows recovered path %q starts with '/' which would make os.DirFS fail", p)
		}
		if len(p) >= 2 && p[1] != ':' {
			t.Errorf("Windows recovered path %q does not start with 'X:' drive form", p)
		}
	}
}

// fakeStopSender 是 stopSender 的测试假实现，记录是否被调用。
type fakeStopSender struct {
	mu       sync.Mutex
	stopped  bool
	stopCnt  int
}

func (f *fakeStopSender) sendGracefulStop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = true
	f.stopCnt++
}

// TestRunWithCtx_FnReturnsBeforeCancel 验证 fn 在 ctx cancel 之前完成时，
// 直接拿到 fn 的 error，不调 stop sender。
func TestRunWithCtx_FnReturnsBeforeCancel(t *testing.T) {
	ctx := context.Background()
	stop := &fakeStopSender{}
	wantErr := errors.New("boom")
	got := runWithCtx(ctx, stop, func() error { return wantErr })
	if !errors.Is(got, wantErr) {
		t.Errorf("got %v, want %v", got, wantErr)
	}
	if stop.stopped {
		t.Errorf("stop sender called when fn returned normally")
	}
}

// TestRunWithCtx_FnReturnsNilBeforeCancel 验证 fn 返 nil 时透传。
func TestRunWithCtx_FnReturnsNilBeforeCancel(t *testing.T) {
	ctx := context.Background()
	stop := &fakeStopSender{}
	got := runWithCtx(ctx, stop, func() error { return nil })
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
	if stop.stopped {
		t.Errorf("stop sender called when fn returned nil")
	}
}

// TestRunWithCtx_CancelWaitsForFnAfterStopSignal 验证 ctx cancel 时 runWithCtx：
//  1. 触发 stop.sendGracefulStop()
//  2. **等 fn 实际返回** 才退出（在 grace timeout 内）
//  3. 仍返回 ctx.Err()（不是 fn 的 error）
//
// 这是 review round 3 Finding 2 的核心断言：CLI SIGINT 后必须等 gomigrate 在
// statement 边界真停下，否则 defer Close 会让 schema_migrations 锁在 dirty=true。
func TestRunWithCtx_CancelWaitsForFnAfterStopSignal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stop := &fakeStopSender{}

	// fn 模拟 gomigrate 行为：收到 stop 信号后过 50ms 才让 done 返回（模拟 statement
	// boundary check）。ctx cancel 后 runWithCtx 应等到 fn 返回，再 return。
	fnStarted := make(chan struct{})
	fnReturned := make(chan struct{})
	stopObserved := make(chan struct{})

	// 监视 stop sender：当 stopped=true 时，触发 fn 在 50ms 后返回（模拟 gomigrate
	// 的"接到 GracefulStop 后在 statement 边界停下"语义）。
	go func() {
		for {
			stop.mu.Lock()
			s := stop.stopped
			stop.mu.Unlock()
			if s {
				close(stopObserved)
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()

	fn := func() error {
		close(fnStarted)
		<-stopObserved
		// 模拟 statement boundary 处理时间
		time.Sleep(50 * time.Millisecond)
		close(fnReturned)
		return errors.New("aborted at boundary")
	}

	got := make(chan error, 1)
	start := time.Now()
	go func() {
		got <- runWithCtxAndTimeout(ctx, stop, fn, 5*time.Second)
	}()

	<-fnStarted
	cancel()

	select {
	case err := <-got:
		elapsed := time.Since(start)
		// 必须等 fn 实际返回（fnReturned 已 close）
		select {
		case <-fnReturned:
			// good
		default:
			t.Errorf("runWithCtx returned before fn finished (no wait-for-done)")
		}
		// 必须返回 ctx.Err() 而不是 fn 的 "aborted at boundary"
		if !errors.Is(err, context.Canceled) {
			t.Errorf("got %v, want context.Canceled", err)
		}
		// 时间合理范围：> 50ms（等 fn 完成）但 < grace timeout
		if elapsed < 30*time.Millisecond {
			t.Errorf("runWithCtx returned too fast (%v), expected to wait for fn", elapsed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runWithCtx did not return within 2s after ctx cancel")
	}

	if !stop.stopped {
		t.Errorf("stop sender not called on ctx cancel")
	}
}

// TestRunWithCtx_GraceTimeoutFiresOnHangingFn 验证 fn 在 ctx cancel + GracefulStop
// 后**仍不返回**时，runWithCtx 在 graceTimeout 后强制退出（不无限等）。
//
// 模拟极端场景：长 ALTER 阻塞、metadata lock 锁住，gomigrate 即使收到 GracefulStop
// 也回不到 statement boundary。CLI 不应永远卡住。
func TestRunWithCtx_GraceTimeoutFiresOnHangingFn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stop := &fakeStopSender{}

	fnStarted := make(chan struct{})
	fnDone := make(chan struct{})
	fn := func() error {
		close(fnStarted)
		<-fnDone // 永不返回（直到测试末尾）
		return nil
	}

	graceTimeout := 100 * time.Millisecond
	got := make(chan error, 1)
	start := time.Now()
	go func() {
		got <- runWithCtxAndTimeout(ctx, stop, fn, graceTimeout)
	}()

	<-fnStarted
	cancel()

	select {
	case err := <-got:
		elapsed := time.Since(start)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("got %v, want context.Canceled", err)
		}
		// 应在 graceTimeout 之后退出（grace 触发），但不应无限等
		if elapsed < graceTimeout {
			t.Errorf("runWithCtx returned in %v, expected >= %v (grace timeout)", elapsed, graceTimeout)
		}
		if elapsed > graceTimeout+500*time.Millisecond {
			t.Errorf("runWithCtx took %v, much longer than grace timeout %v", elapsed, graceTimeout)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runWithCtx did not respect graceTimeout")
	}

	if !stop.stopped {
		t.Errorf("stop sender not called on ctx cancel")
	}

	// 让后台 fn goroutine 退出（避免 goroutine leak）
	close(fnDone)
}

// TestRunWithCtx_DeadlineExceededWaitsForFn 验证 ctx 超时（DeadlineExceeded）路径
// 也同样 wait-for-done。
func TestRunWithCtx_DeadlineExceededWaitsForFn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	stop := &fakeStopSender{}

	fnStarted := make(chan struct{})
	stopObserved := make(chan struct{})
	go func() {
		for {
			stop.mu.Lock()
			s := stop.stopped
			stop.mu.Unlock()
			if s {
				close(stopObserved)
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()

	fn := func() error {
		close(fnStarted)
		<-stopObserved
		return nil
	}

	start := time.Now()
	err := runWithCtxAndTimeout(ctx, stop, fn, 5*time.Second)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("got %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 1*time.Second {
		t.Errorf("runWithCtx took %v, expected < 1s after deadline+stop", elapsed)
	}
	if !stop.stopped {
		t.Errorf("stop sender not called on deadline")
	}
}

// TestUp_NilMigratorWithCanceledCtx 验证 nil migrator 路径上即使传 canceled ctx，
// 仍优先报"closed or uninitialized" —— 这是防护性 short-circuit。
func TestUp_NilMigratorWithCanceledCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := &migrator{m: nil}
	err := m.Up(ctx)
	if err == nil {
		t.Fatal("got nil, want error")
	}
	// 必须报 closed 而不是 ctx.Canceled —— nil migrator 比 ctx 优先级高
	if !strings.Contains(err.Error(), "closed or uninitialized") {
		t.Errorf("error %q does not mention 'closed or uninitialized'", err.Error())
	}
}

// TestPathToFileURI_CallableFromNew 验证 New 调用 pathToFileURI 路径能成功
// （path 不存在时仍走 gomigrate.New 报 error，但 file URI 拼接本身不应失败）。
func TestPathToFileURI_CallableFromNew(t *testing.T) {
	// 用一个明显不存在的相对路径 —— 应在 gomigrate.New 阶段失败（找不到文件），
	// 而不是在 pathToFileURI 拼接阶段失败（filepath.Abs 不要求路径存在）。
	_, err := New("user:pass@tcp(host:3306)/db", "this-path-does-not-exist-xyz")
	if err == nil {
		t.Fatal("expected error from non-existent path")
	}
	// 错误必须来自 gomigrate.New（"open source/database failed"），
	// 而不是来自 pathToFileURI（"build file URI failed"）—— 后者意味 URI 拼接路径炸了
	if strings.Contains(err.Error(), "build file URI failed") {
		t.Errorf("pathToFileURI itself failed unexpectedly: %v", err)
	}
}
