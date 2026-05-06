//go:build integration
// +build integration

// Story 10.2 集成测试：用 dockertest 起真实 redis:7-alpine 容器跑 4 条 case：
//  1. Open + Ping happy path（启动 → ping → 返回 RedisClient）
//  2. SET → GET 返回相同值（验证真 redis 下 client.Set/Get 工作）
//  3. SADD 多次 → SMEMBERS 返回去重集合
//  4. Open 后关停容器 → 再 Ping fail-fast（验证 fail-fast 在容器挂了之后仍生效）
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑这些；
// 只在 `bash scripts/build.sh --integration`（即 `go test -tags=integration`）触发。
//
// docker 不可用时 t.Skip("docker not available")，不让 CI 阻塞（与 mysql_integration_test.go
// 同模式）。
//
// 每条测试独立起一个容器（`t.Cleanup` 释放）—— 避免测试间状态污染。容器端口动态分配
// 不固定 6379，避免与本机 Redis 冲突。

package redis_test

import (
	"context"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	goredis "github.com/redis/go-redis/v9"

	"github.com/huing/cat/server/internal/infra/config"
	redisinfra "github.com/huing/cat/server/internal/infra/redis"
)

// startRedis 起一个 redis:7-alpine 容器，等 ping 通后返回 (Addr, cleanup)。
//
// 关键参数：
//   - 镜像 redis:7-alpine（轻量 ~30MB）
//   - 端口由 dockertest 动态分配（不固定 6379）
//   - retry 30 次每次 1 秒（redis 冷启 < 5s，给点 buffer）
func startRedis(t *testing.T) (string, func()) {
	t.Helper()

	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Skipf("docker not available: %v", err)
	}
	if err := pool.Client.Ping(); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "redis",
		Tag:        "7-alpine",
	}, func(hc *docker.HostConfig) {
		hc.AutoRemove = true
		hc.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		t.Skipf("could not start redis container: %v", err)
	}

	hostPort := resource.GetPort("6379/tcp")
	addr := "127.0.0.1:" + hostPort

	pool.MaxWait = 30 * time.Second
	if err := pool.Retry(func() error {
		c := goredis.NewClient(&goredis.Options{Addr: addr})
		defer c.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		return c.Ping(ctx).Err()
	}); err != nil {
		_ = pool.Purge(resource)
		t.Skipf("redis container did not become ready: %v", err)
	}

	cleanup := func() {
		_ = pool.Purge(resource)
	}
	return addr, cleanup
}

// TestRedisIntegration_OpenAndPing 起容器 → redisinfra.Open 返回 RedisClient + ping 成功
// → 关闭 → 资源清理。这是 Open 函数 happy path 的真实集成验证。
func TestRedisIntegration_OpenAndPing(t *testing.T) {
	addr, cleanup := startRedis(t)
	defer cleanup()

	cfg := config.RedisConfig{
		Addr:     addr,
		Password: "",
		DB:       0,
		PoolSize: 5,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := redisinfra.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("redisinfra.Open: %v", err)
	}
	defer client.Close()

	// 简单 SET/GET 烟测确认连接真的可用
	if _, err := client.Set(ctx, "smoke", "ok", 0, false); err != nil {
		t.Errorf("Set after Open: %v", err)
	}
	val, err := client.Get(ctx, "smoke")
	if err != nil {
		t.Errorf("Get after Open: %v", err)
	}
	if val != "ok" {
		t.Errorf("Get returned %q, want %q", val, "ok")
	}
}

// TestRedisIntegration_SetGet_E2E 起容器 → SET → GET 返回相同值。
// epics.md §Story 10.2 行 1681 钦定的 "启动 → ping → SET / GET" 锚定项。
func TestRedisIntegration_SetGet_E2E(t *testing.T) {
	addr, cleanup := startRedis(t)
	defer cleanup()

	cfg := config.RedisConfig{Addr: addr, PoolSize: 5}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := redisinfra.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("redisinfra.Open: %v", err)
	}
	defer client.Close()

	if _, err := client.Set(ctx, "key1", "value1", 0, false); err != nil {
		t.Fatalf("Set: %v", err)
	}
	val, err := client.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "value1" {
		t.Errorf("Get = %q, want %q", val, "value1")
	}

	// 验证 GET 不存在的 key 返 ("", nil)（抽象层语义在真 redis 下也保持）
	val, err = client.Get(ctx, "nonexist")
	if err != nil {
		t.Errorf("Get nonexist returned error %v, want nil", err)
	}
	if val != "" {
		t.Errorf("Get nonexist = %q, want empty string", val)
	}
}

// TestRedisIntegration_SAdd_SMembers_E2E 起容器 → SADD 多次 → SMEMBERS 返回去重集合。
// epics.md §Story 10.2 行 1681 钦定的 "SADD / SMEMBERS" 锚定项。
func TestRedisIntegration_SAdd_SMembers_E2E(t *testing.T) {
	addr, cleanup := startRedis(t)
	defer cleanup()

	cfg := config.RedisConfig{Addr: addr, PoolSize: 5}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := redisinfra.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("redisinfra.Open: %v", err)
	}
	defer client.Close()

	added, err := client.SAdd(ctx, "set1", "alice", "bob", "alice")
	if err != nil {
		t.Fatalf("SAdd: %v", err)
	}
	if added != 2 {
		t.Errorf("SAdd returned %d newly added, want 2 (alice + bob)", added)
	}

	members, err := client.SMembers(ctx, "set1")
	if err != nil {
		t.Fatalf("SMembers: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("SMembers len = %d, want 2", len(members))
	}
	got := map[string]bool{}
	for _, m := range members {
		got[m] = true
	}
	if !got["alice"] || !got["bob"] {
		t.Errorf("SMembers = %v, want set containing alice + bob", members)
	}

	// SREM 一个，再 SMEMBERS
	removed, err := client.SRem(ctx, "set1", "alice")
	if err != nil {
		t.Fatalf("SRem: %v", err)
	}
	if removed != 1 {
		t.Errorf("SRem returned %d, want 1", removed)
	}
}

// TestRedisIntegration_PingFail_FailFast 起容器 → Open 成功 → 关掉容器 →
// 再调命令（依赖底层 ping 失败）→ 返 error 而不是悬挂。
//
// 这是 fail-fast 在 "运行时" Redis 不可达场景的验证：Open 时通了，但服务过程中
// Redis down 掉，下次命令必须返 error 而不是悬挂。
//
// 注意：本测试**不**测 Open 阶段的 fail-fast（那条已在 Open 函数内实装并由单测覆盖）；
// 而是测"Open 之后容器挂"路径，因为 Open 阶段 fail-fast 需要给一个错误的 addr 即可，
// 容器没必要在 Open 之前关 —— 如需测 Open 阶段，可启容器后取 addr 再 Purge 再 Open。
func TestRedisIntegration_PingFail_FailFast(t *testing.T) {
	addr, cleanup := startRedis(t)
	cleanedUp := false
	defer func() {
		if !cleanedUp {
			cleanup()
		}
	}()

	cfg := config.RedisConfig{Addr: addr, PoolSize: 5}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := redisinfra.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("redisinfra.Open: %v", err)
	}
	defer client.Close()

	// 主动关闭容器
	cleanup()
	cleanedUp = true

	// 给容器一点时间真正退出（dockertest Purge 是异步的）
	time.Sleep(2 * time.Second)

	// 用短 timeout ctx 防止挂死
	cmdCtx, cmdCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cmdCancel()

	if _, err := client.Get(cmdCtx, "anything"); err == nil {
		t.Errorf("Get after container purge returned nil error, want error")
	}
}
