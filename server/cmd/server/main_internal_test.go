package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestUserKeyedMutex_SameUserSerializes：review 10-6 r8 P1 修后的核心断言。
//
// 多个 goroutine 同时对同一 userID 调 LockFor → Lock → 临界区 → Unlock。
// 临界区内修改 atomic 计数器（"开始"自增 + "结束"自减），任何时刻 inFlight 必须
// <= 1（不重叠 = 串行）。
//
// 这是用来锁定"fire-and-forget hook 同 user Add/Remove 必须串行"的核心不变量
// 的回归测试。修前用裸 goroutine + 不持锁，多 goroutine 并发调 AddOnline /
// RemoveOnline 可能 RemoveOnline 先跑完 AddOnline 后跑 → presence 复活已离线
// session。修后所有同 userID 的 hook 在 LockFor 锁上排队，跑完一个再开下一个。
func TestUserKeyedMutex_SameUserSerializes(t *testing.T) {
	var keyed userKeyedMutex
	const userID = uint64(42)
	const goroutines = 100

	var wg sync.WaitGroup
	var inFlight atomic.Int32
	var maxConcurrent atomic.Int32
	var violation atomic.Int32

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			mu := keyed.LockFor(userID)
			mu.Lock()
			defer mu.Unlock()

			// 进入临界区：inFlight 自增；如果 > 1 表示串行不变量被破坏
			cur := inFlight.Add(1)
			if cur > 1 {
				violation.Add(1)
			}
			// 更新 maxConcurrent（仅观测；不变量是 == 1）
			for {
				m := maxConcurrent.Load()
				if cur <= m || maxConcurrent.CompareAndSwap(m, cur) {
					break
				}
			}
			// 故意 sleep 让"重叠"如果存在的话明显暴露（修前并行执行很容易 > 1）
			time.Sleep(50 * time.Microsecond)
			inFlight.Add(-1)
		}()
	}
	wg.Wait()

	if got := violation.Load(); got != 0 {
		t.Fatalf("inFlight > 1 happened %d times; same-userID Add/Remove must serialize (r8 P1 invariant)", got)
	}
	if got := maxConcurrent.Load(); got != 1 {
		t.Fatalf("maxConcurrent = %d, want 1 (same-userID LockFor must serialize)", got)
	}
}

// TestUserKeyedMutex_DifferentUsersDoNotBlock：不同 userID 的 LockFor 必须**不**互相
// 阻塞 —— 否则全局串行化会让 N user 的 hook 退化到 O(N × Redis latency) shutdown。
//
// 验证：同时锁 user A + user B 的 mutex，两个临界区可以重叠。
func TestUserKeyedMutex_DifferentUsersDoNotBlock(t *testing.T) {
	var keyed userKeyedMutex

	muA := keyed.LockFor(1)
	muB := keyed.LockFor(2)

	muA.Lock()
	defer muA.Unlock()

	// 期望：muB.Lock() 立即拿到（没被 muA 阻塞），不会 deadlock / 不会等
	done := make(chan struct{})
	go func() {
		muB.Lock()
		muB.Unlock()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("muB.Lock() blocked by muA.Lock(); different users must not serialize")
	}
}

// TestUserKeyedMutex_LockFor_ReturnsSamePointerForSameUser：
// 多次 LockFor 同 userID 必须返回**同一** *sync.Mutex 实例 —— 否则不同 goroutine
// 抢的不是同一把锁，串行化语义直接失效。
//
// 这是 sync.Map LoadOrStore 的关键不变量：首次 store 后续 Load 命中同地址。
func TestUserKeyedMutex_LockFor_ReturnsSamePointerForSameUser(t *testing.T) {
	var keyed userKeyedMutex
	a := keyed.LockFor(7)
	b := keyed.LockFor(7)
	if a != b {
		t.Fatalf("LockFor(7) returned different *Mutex instances (%p vs %p); same userID must reuse same mutex", a, b)
	}
}
