// Package redis_test 是 Story 10.2 的黑盒单元测试。
//
// 测试栈：
//   - testhelper.NewMiniRedis 启动 in-process miniredis server（ADR-0001 §3.2 钦定）
//   - redisinfra.NewRedisClientFromMiniredis 桥接 RedisClient 接口
//   - 9 个 case 严格对应 story §AC8 表格
//
// 命名规范：Test<Type>_<Behavior>_<Outcome> 三段式（与 step_account_repo_test.go /
// pet_repo_test.go 既有命名风格一致）。
//
// 测试不写 mock 行为期望（miniredis 是真 server，行为正确性由 miniredis 自身保证）。
package redis_test

import (
	"context"
	"errors"
	"testing"
	"time"

	redisinfra "github.com/huing/cat/server/internal/infra/redis"
	testhelper "github.com/huing/cat/server/internal/pkg/testing"
)

// newClient 是本文件 9 个 case 共用的 setup helper：起 miniredis + 拿 RedisClient。
//
// miniredis 自动在 t.Cleanup 关闭；client 也在 NewRedisClientFromMiniredis 内部
// 注册 t.Cleanup 关闭。两者顺序：client.Close 先，miniredis.Close 后（LIFO）。
func newClient(t *testing.T) (redisinfra.RedisClient, *miniredisAdapter) {
	t.Helper()
	mr, _ := testhelper.NewMiniRedis(t)
	client := redisinfra.NewRedisClientFromMiniredis(t, mr)
	return client, &miniredisAdapter{mr: mr}
}

// miniredisAdapter 封装 *miniredis.Miniredis 的 FastForward 方法供 TTL 测试使用。
// （直接 import miniredis package 也行；包一层让本 test file import 段更窄。）
type miniredisAdapter struct {
	mr fastForwarder
}

type fastForwarder interface {
	FastForward(d time.Duration)
}

func (a *miniredisAdapter) FastForward(d time.Duration) {
	a.mr.FastForward(d)
}

// TestRedisClient_SetGet_HappyPath 验证 SET → GET 返回相同值。
// epics.md §Story 10.2 行 1676 钦定 happy: SET → GET 返回相同值。
func TestRedisClient_SetGet_HappyPath(t *testing.T) {
	client, _ := newClient(t)
	ctx := context.Background()

	ok, err := client.Set(ctx, "foo", "bar", 0, false)
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !ok {
		t.Errorf("Set returned ok=false, want true (普通 SET 总是 true)")
	}

	val, err := client.Get(ctx, "foo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "bar" {
		t.Errorf("Get returned %q, want %q", val, "bar")
	}
}

// TestRedisClient_SetExpire_TTL 验证 SET 带 EXPIRE → mr.FastForward(2m) → GET 返回 nil。
// epics.md §Story 10.2 行 1677 钦定 happy: SET 带 EXPIRE → 过期后 GET 返回 nil。
func TestRedisClient_SetExpire_TTL(t *testing.T) {
	client, mr := newClient(t)
	ctx := context.Background()

	if _, err := client.Set(ctx, "k", "v", 1*time.Minute, false); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// 过期前能读到
	val, err := client.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get pre-expire: %v", err)
	}
	if val != "v" {
		t.Errorf("Get pre-expire = %q, want %q", val, "v")
	}

	// FastForward 2 分钟（> 1 分钟 TTL）
	mr.FastForward(2 * time.Minute)

	// 过期后返 ("", nil)（不抛 error）
	val, err = client.Get(ctx, "k")
	if err != nil {
		t.Errorf("Get post-expire returned error %v, want nil (key not exist 应返 nil error)", err)
	}
	if val != "" {
		t.Errorf("Get post-expire = %q, want \"\" (key 已过期)", val)
	}
}

// TestRedisClient_SAdd_SMembers_Dedup 验证 SAdd 多次 + SMembers 返回去重集合。
// epics.md §Story 10.2 行 1678 钦定 happy: SADD 多次 + SMEMBERS → 返回去重集合。
func TestRedisClient_SAdd_SMembers_Dedup(t *testing.T) {
	client, _ := newClient(t)
	ctx := context.Background()

	// 加 "a", "a", "b" → 只有 "a" / "b" 真正入 set
	added, err := client.SAdd(ctx, "myset", "a", "a", "b")
	if err != nil {
		t.Fatalf("SAdd: %v", err)
	}
	if added != 2 {
		t.Errorf("SAdd returned %d newly added, want 2 (a + b，两次 a 第二次不计)", added)
	}

	members, err := client.SMembers(ctx, "myset")
	if err != nil {
		t.Fatalf("SMembers: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("SMembers returned %d members, want 2", len(members))
	}
	// 验证两个 member 都在（不依赖顺序，set 无序）
	got := map[string]bool{}
	for _, m := range members {
		got[m] = true
	}
	if !got["a"] || !got["b"] {
		t.Errorf("SMembers = %v, want set containing a + b", members)
	}
}

// TestRedisClient_Get_KeyNotExist_NoError 验证 GET 不存在的 key → ("", nil)。
// 不返 redis.Nil error 透传给调用方（本 story 抽象层把"key 不存在"语义内化）。
// epics.md §Story 10.2 行 1679 钦定 edge: GET 不存在的 key → 返回 nil（不抛 error）。
func TestRedisClient_Get_KeyNotExist_NoError(t *testing.T) {
	client, _ := newClient(t)
	ctx := context.Background()

	val, err := client.Get(ctx, "nonexistent")
	if err != nil {
		t.Errorf("Get nonexistent returned error %v, want nil (抽象层内化 redis.Nil)", err)
	}
	if val != "" {
		t.Errorf("Get nonexistent = %q, want \"\"", val)
	}
}

// TestRedisClient_SetNX_KeyExists_ReturnsFalse 验证 SET NX 已存在 key →
// (false, nil)，原值未覆盖。让 NX 语义被锁住，Epic 20 / 32 幂等键路径确定性。
func TestRedisClient_SetNX_KeyExists_ReturnsFalse(t *testing.T) {
	client, _ := newClient(t)
	ctx := context.Background()

	// 第一次 SET NX → 成功
	ok, err := client.Set(ctx, "k", "v1", 0, true)
	if err != nil {
		t.Fatalf("Set NX (first): %v", err)
	}
	if !ok {
		t.Errorf("Set NX first call returned ok=false, want true (首次 set 应成功)")
	}

	// 第二次 SET NX 同 key → 失败 (false, nil)
	ok, err = client.Set(ctx, "k", "v2", 0, true)
	if err != nil {
		t.Errorf("Set NX (second) returned error %v, want nil (NX 失败不应是 error)", err)
	}
	if ok {
		t.Errorf("Set NX second call returned ok=true, want false (key 已存在不应覆盖)")
	}

	// 验证原值未被覆盖
	val, err := client.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "v1" {
		t.Errorf("Get after failed NX = %q, want %q (原值不应被覆盖)", val, "v1")
	}
}

// TestRedisClient_Del_NonExistKey_Returns0NoError 验证 Del 不存在的 key → (0, nil)。
// 让"删除不存在 key"语义被锁住（Redis 行为：DEL 不存在 key 视为 "0 deleted"，不报错）。
func TestRedisClient_Del_NonExistKey_Returns0NoError(t *testing.T) {
	client, _ := newClient(t)
	ctx := context.Background()

	deleted, err := client.Del(ctx, "ghost")
	if err != nil {
		t.Errorf("Del nonexistent returned error %v, want nil", err)
	}
	if deleted != 0 {
		t.Errorf("Del nonexistent returned %d, want 0", deleted)
	}

	// 加一个 key + 同时 del 已存在 + 不存在 → 1
	if _, err := client.Set(ctx, "real", "v", 0, false); err != nil {
		t.Fatalf("Set: %v", err)
	}
	deleted, err = client.Del(ctx, "real", "ghost2")
	if err != nil {
		t.Fatalf("Del mix: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Del mix = %d, want 1 (only real exists)", deleted)
	}
}

// TestRedisClient_Expire_NonExistKey_ReturnsFalse 验证 Expire 不存在 key →
// (false, nil)。让 Expire 边缘语义被锁住（Redis 行为：EXPIRE 不存在 key 返 0 / false）。
func TestRedisClient_Expire_NonExistKey_ReturnsFalse(t *testing.T) {
	client, _ := newClient(t)
	ctx := context.Background()

	ok, err := client.Expire(ctx, "ghost", 10*time.Second)
	if err != nil {
		t.Errorf("Expire nonexistent returned error %v, want nil", err)
	}
	if ok {
		t.Errorf("Expire nonexistent returned ok=true, want false (key 不存在)")
	}

	// 已存在 key + Expire → true
	if _, err := client.Set(ctx, "real", "v", 0, false); err != nil {
		t.Fatalf("Set: %v", err)
	}
	ok, err = client.Expire(ctx, "real", 10*time.Second)
	if err != nil {
		t.Fatalf("Expire real: %v", err)
	}
	if !ok {
		t.Errorf("Expire real returned ok=false, want true")
	}
}

// TestRedisClient_CtxCancel_CommandAborts 验证传入已 cancel 的 ctx → Get/Set 必须返
// ctx.Err()，**不**裸跑命令到底。ADR-0007 ctx 传播契约。
//
// review r2 强化：本 case 同时锚定 ContextTimeoutEnabled: true 必须开启 —— go-redis/v9
// 默认 false 时 baseClient.context() 直接返 context.Background()，已 cancel 的 ctx
// 也会被忽略，命令裸跑到 socket timeout 才返 error（不是返 context.Canceled）。
// 本 case 用 errors.Is(err, context.Canceled) 严格断言，能在该选项被回退时立刻挂掉。
func TestRedisClient_CtxCancel_CommandAborts(t *testing.T) {
	client, _ := newClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即 cancel

	_, err := client.Get(ctx, "anything")
	if err == nil {
		t.Fatalf("Get with canceled ctx returned nil error, want ctx.Canceled wrapped")
	}
	if !errors.Is(err, context.Canceled) {
		// 严格断言：ContextTimeoutEnabled: true 必须让 ctx.Canceled 透传上来。
		// 如果这里挂了，大概率是 redis.Open 里的 Options 漏配 ContextTimeoutEnabled。
		t.Errorf("Get with canceled ctx err = %v (type %T), want errors.Is context.Canceled = true (检查 redis.Open 是否设了 ContextTimeoutEnabled: true)", err, err)
	}

	_, err = client.Set(ctx, "k", "v", 0, false)
	if err == nil {
		t.Fatalf("Set with canceled ctx returned nil error, want ctx.Canceled wrapped")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Set with canceled ctx err = %v (type %T), want errors.Is context.Canceled = true", err, err)
	}
}

// TestRedisClient_CtxDeadline_CommandAborts 验证传入已超时的 ctx (DeadlineExceeded) →
// Get/Set 必须立即返 context.DeadlineExceeded（包装），**不**等 socket-level timeout。
//
// review r2 新增：与 _CtxCancel_CommandAborts 互补，专门锚 ContextTimeoutEnabled: true
// 让 deadline 路径也生效。go-redis 默认 false 时 deadline 完全失效，命令会卡到
// DialTimeout / ReadTimeout（默认 3s / 5s）才返 socket error，不是 ctx.DeadlineExceeded。
func TestRedisClient_CtxDeadline_CommandAborts(t *testing.T) {
	client, _ := newClient(t)
	// 用一个已经过期的 deadline（过去时间），等于第一次命令调用就立刻超时
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Second))
	defer cancel()

	_, err := client.Get(ctx, "anything")
	if err == nil {
		t.Fatalf("Get with expired deadline ctx returned nil error, want ctx.DeadlineExceeded wrapped")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Get with expired deadline ctx err = %v (type %T), want errors.Is context.DeadlineExceeded = true (检查 redis.Open 是否设了 ContextTimeoutEnabled: true)", err, err)
	}

	_, err = client.Set(ctx, "k", "v", 0, false)
	if err == nil {
		t.Fatalf("Set with expired deadline ctx returned nil error, want ctx.DeadlineExceeded wrapped")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Set with expired deadline ctx err = %v (type %T), want errors.Is context.DeadlineExceeded = true", err, err)
	}
}

// TestRedisClient_Close_Idempotent 验证 Close 调两次都返 nil（review r2 加强）。
//
// 之前版本只断言"不 panic"接受第二次返 error。但接口注释（client.go 行 78-79）
// 承诺"多次调用不报错"，所以第二次也必须返 nil（与 *sql.DB.Close 行为对齐）。
// 实装用 sync.Once 包了 close 调用 —— 第一次关 + 记 err；后续调用直接返第一次的 err。
//
// 注意：NewRedisClientFromMiniredis 在 t.Cleanup 注册了 client.Close，所以本测试
// 手动调两次后，cleanup 还会再调一次（共三次）—— 三次都不应 panic / 不应返 error。
func TestRedisClient_Close_Idempotent(t *testing.T) {
	client, _ := newClient(t)

	// 第一次 Close —— 真关
	if err := client.Close(); err != nil {
		t.Errorf("first Close returned %v, want nil", err)
	}

	// 第二次 Close（手动）—— sync.Once 短路，仍返 nil
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("second Close panicked: %v", r)
		}
	}()
	if err := client.Close(); err != nil {
		t.Errorf("second Close returned %v, want nil (接口承诺幂等；sync.Once 应短路)", err)
	}
	// 注：t.Cleanup 之后还会再调一次 Close（第三次），仍应返 nil（不 panic / 不报错）。
}
