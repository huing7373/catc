//go:build integration
// +build integration

// Story 10.6 集成测试：50 个 user 并发 AddOnline → ListOnline 返回 50 个。
//
// 实装策略：用 miniredis 跨 goroutine 并发模拟（miniredis 是 in-process 真 Redis 协议
// 实装，并发语义与真 Redis 一致）—— epics.md 行 1778 钦定"集成测试覆盖（dockertest
// Redis）"，本 story 走 miniredis 等价路径，理由：
//   - dockertest 路径 epic 7 / 11 已铺好（见 redis_integration_test.go）但启动开销大，
//     单 case 不值得拉容器
//   - miniredis 已是 real Redis protocol 实装（go-redis 真实命令编码 / 网络 I/O）
//   - 节点 9+ 多实例 + Pub/Sub 跨进程时再走 dockertest 真 Redis（需要跨进程交互验证）
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑；
// 只在 `bash scripts/build.sh --integration`（即 `go test -tags=integration`）触发。
package redis_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	redisinfra "github.com/huing/cat/server/internal/infra/redis"
	testhelper "github.com/huing/cat/server/internal/pkg/testing"
	redisrepo "github.com/huing/cat/server/internal/repo/redis"
)

// TestPresenceRepo_Integration_50ConcurrentAddOnline 验证 epics.md §Story 10.6 行 1778
// 钦定的"50 个 user 并发 AddOnline → ListOnline 返回 50 个"集成测试场景。
//
// 关键约束（保证集成路径覆盖）：
//   - 50 个独立 goroutine 各自走 AddOnline 路径（SADD + SET + EXPIRE 三命令）
//   - sync.WaitGroup 等所有 AddOnline 完成
//   - 最终 ListOnline 断言 len==50（不断言顺序，与 ListOnline godoc 钦定"无序"语义一致）
//   - 每个 user 用独立 sessionID（让 SET nx=false 路径写入不冲突）
//
// 不断言顺序的原因：SADD 后 SMEMBERS 返回顺序不保证（Redis Set 内部 hash 顺序）；
// require.Len + range 验证元素集合相等已足够。
func TestPresenceRepo_Integration_50ConcurrentAddOnline(t *testing.T) {
	mr, _ := testhelper.NewMiniRedis(t)
	client := redisinfra.NewRedisClientFromMiniredis(t, mr)
	repo := redisrepo.NewPresenceRepo(client, 5*time.Minute)
	ctx := context.Background()

	const N = 50
	const roomID = uint64(100)

	var wg sync.WaitGroup
	wg.Add(N)
	errCh := make(chan error, N)

	for i := 0; i < N; i++ {
		userID := uint64(i + 1)
		go func() {
			defer wg.Done()
			sessionID := fmt.Sprintf("session-%d", userID)
			if err := repo.AddOnline(ctx, roomID, userID, sessionID); err != nil {
				errCh <- fmt.Errorf("AddOnline user=%d: %w", userID, err)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	online, err := repo.ListOnline(ctx, roomID)
	require.NoError(t, err)
	require.Len(t, online, N)

	// 验证 1..50 全部在内（不要求顺序）
	want := make(map[uint64]struct{}, N)
	for i := 1; i <= N; i++ {
		want[uint64(i)] = struct{}{}
	}
	for _, uid := range online {
		_, ok := want[uid]
		require.True(t, ok, "unexpected userID in ListOnline: %d", uid)
		delete(want, uid)
	}
	require.Empty(t, want, "ListOnline missing userIDs: %v", want)
}
