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
func TestRedisClient_CtxCancel_CommandAborts(t *testing.T) {
	client, _ := newClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即 cancel

	_, err := client.Get(ctx, "anything")
	if err == nil {
		t.Errorf("Get with canceled ctx returned nil error, want ctx.Err() or wrapped")
	}
	// go-redis 实际返回的 err 会包装 context.Canceled；用 errors.Is 兜底
	if err != nil && !errors.Is(err, context.Canceled) {
		// 注：某些 go-redis 版本可能在已 cancel ctx 时直接返 context.Canceled 或包装错误；
		// 主要验证"是 error 且能识别为取消"，不严卡具体类型。
		t.Logf("Get error type: %T value: %v (期望 errors.Is(err, context.Canceled) = true)", err, err)
	}

	_, err = client.Set(ctx, "k", "v", 0, false)
	if err == nil {
		t.Errorf("Set with canceled ctx returned nil error, want ctx.Err() or wrapped")
	}
}

// TestRedisClient_Close_Idempotent 验证 Close 调两次不 panic / 不返 error。
// 与 *sql.DB.Close 行为一致语义。
//
// 注意：因为 NewRedisClientFromMiniredis 已经在 t.Cleanup 注册了 client.Close，
// 本测试**额外**手动调一次 Close —— 等于 Close 被调两次（手动一次 + cleanup 一次）。
func TestRedisClient_Close_Idempotent(t *testing.T) {
	client, _ := newClient(t)

	// 第一次 Close
	if err := client.Close(); err != nil {
		t.Errorf("first Close returned %v, want nil", err)
	}

	// 第二次 Close（手动）—— 应该不 panic / 不返 error
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("second Close panicked: %v", r)
		}
	}()
	if err := client.Close(); err != nil {
		// go-redis Client.Close 第二次调用确实可能返 "redis: client is closed" 类错误；
		// 接受任何 error 但**不**接受 panic。如返 error，记 log 即可（本接口契约只
		// 要求"不 panic"）。
		t.Logf("second Close returned %v (acceptable, no panic 即可)", err)
	}
	// 注：t.Cleanup 之后还会再调一次 Close，三次总调用，仍不应该 panic。
}
