// Package redis_test 是 Story 10.6 PresenceRepo 的黑盒单测。
//
// 测试栈：testify (require) + miniredis (in-process Redis) + RedisClient 抽象桥接。
// 与 server/internal/infra/redis/redis_test.go 既有 9 case 模式严格一致 ——
// 不写"纯 in-memory map" mock，复用 redisinfra.NewRedisClientFromMiniredis 跑真
// Redis 协议，覆盖 SADD/SREM 去重语义、TTL FastForward 等真实行为。
package redis_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	redisinfra "github.com/huing/cat/server/internal/infra/redis"
	testhelper "github.com/huing/cat/server/internal/pkg/testing"
	redisrepo "github.com/huing/cat/server/internal/repo/redis"
)

// newPresenceRepo 是本文件 case 共用的 setup helper：起 miniredis + 拿
// RedisClient + 构造 PresenceRepo 实例。与 redis_test.go 行 28 newClient 模式一致。
//
// miniredis 自动在 t.Cleanup 关闭；client 也在 NewRedisClientFromMiniredis 内部
// 注册 t.Cleanup 关闭。
func newPresenceRepo(t *testing.T) (redisrepo.PresenceRepo, *miniredisAdapter, redisinfra.RedisClient) {
	t.Helper()
	mr, _ := testhelper.NewMiniRedis(t)
	client := redisinfra.NewRedisClientFromMiniredis(t, mr)
	repo := redisrepo.NewPresenceRepo(client, 5*time.Minute)
	return repo, &miniredisAdapter{mr: mr}, client
}

// miniredisAdapter 封装 *miniredis.Miniredis 的 FastForward 方法供 TTL 测试使用。
// 与 redis_test.go 行 37 同模式（窄化 import 段）。
type miniredisAdapter struct {
	mr fastForwarder
}

type fastForwarder interface {
	FastForward(d time.Duration)
}

func (a *miniredisAdapter) FastForward(d time.Duration) {
	a.mr.FastForward(d)
}

// TestPresenceRepo_AddOnline_IsOnline_ReturnsTrue 验证 happy path：
// AddOnline 后 IsOnline 返 true。
func TestPresenceRepo_AddOnline_IsOnline_ReturnsTrue(t *testing.T) {
	repo, _, _ := newPresenceRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.AddOnline(ctx, 100, 42, "session-abc"))

	online, err := repo.IsOnline(ctx, 100, 42)
	require.NoError(t, err)
	require.True(t, online)
}

// TestPresenceRepo_RemoveOnline_IsOnline_ReturnsFalse 验证 happy path：
// RemoveOnline（sessionID 匹配）后 IsOnline 返 false。
func TestPresenceRepo_RemoveOnline_IsOnline_ReturnsFalse(t *testing.T) {
	repo, _, _ := newPresenceRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.AddOnline(ctx, 100, 42, "session-abc"))
	require.NoError(t, repo.RemoveOnline(ctx, 100, 42, "session-abc"))

	online, err := repo.IsOnline(ctx, 100, 42)
	require.NoError(t, err)
	require.False(t, online)
}

// TestPresenceRepo_ListOnline_ReturnsCorrectUserIDs 验证 happy path：
// 多 user 加入同 room 后 ListOnline 正确返回 []uint64。
func TestPresenceRepo_ListOnline_ReturnsCorrectUserIDs(t *testing.T) {
	repo, _, _ := newPresenceRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.AddOnline(ctx, 100, 1, "s1"))
	require.NoError(t, repo.AddOnline(ctx, 100, 2, "s2"))
	require.NoError(t, repo.AddOnline(ctx, 100, 3, "s3"))

	online, err := repo.ListOnline(ctx, 100)
	require.NoError(t, err)
	require.ElementsMatch(t, []uint64{1, 2, 3}, online)
}

// TestPresenceRepo_AddOnline_Duplicates_DedupedBySADD 验证 edge：
// 同一 user 多次 AddOnline 同一 room → SADD 去重，ListOnline 仍返 1 个。
// sessionID 应被覆盖到最新值（reconnect 替换语义；SET 走 nx=false）。
func TestPresenceRepo_AddOnline_Duplicates_DedupedBySADD(t *testing.T) {
	repo, _, client := newPresenceRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.AddOnline(ctx, 100, 42, "session-v1"))
	require.NoError(t, repo.AddOnline(ctx, 100, 42, "session-v2"))

	online, err := repo.ListOnline(ctx, 100)
	require.NoError(t, err)
	require.Equal(t, []uint64{42}, online)

	// sessionID 应被覆盖到 v2（reconnect 替换语义；SET 走 nx=false → 第二次覆盖第一次）
	val, err := client.Get(ctx, "user:42:ws_session")
	require.NoError(t, err)
	require.Equal(t, "session-v2", val)
}

// TestPresenceRepo_TTLExpire_ListOnline_RemovesUser 验证 edge：
// TTL 到期后 miniredis FastForward 让 key 过期，ListOnline 不再含该 user。
// 这是"server crash 后僵尸 user 自然清"语义的正向 case。
func TestPresenceRepo_TTLExpire_ListOnline_RemovesUser(t *testing.T) {
	mr, _ := testhelper.NewMiniRedis(t)
	client := redisinfra.NewRedisClientFromMiniredis(t, mr)
	repo := redisrepo.NewPresenceRepo(client, 5*time.Second)
	ctx := context.Background()

	require.NoError(t, repo.AddOnline(ctx, 100, 42, "s1"))

	online, err := repo.ListOnline(ctx, 100)
	require.NoError(t, err)
	require.Equal(t, []uint64{42}, online)

	mr.FastForward(6 * time.Second) // 让 TTL 过期

	online, err = repo.ListOnline(ctx, 100)
	require.NoError(t, err)
	require.Empty(t, online)
}

// TestPresenceRepo_IsOnline_RoomNotExists_ReturnsFalseNoError 验证 edge：
// 不存在 room set 时 IsOnline 返 (false, nil) —— 不视为 error（与 redisinfra
// 内化 nil error 模式一致）。
func TestPresenceRepo_IsOnline_RoomNotExists_ReturnsFalseNoError(t *testing.T) {
	repo, _, _ := newPresenceRepo(t)

	online, err := repo.IsOnline(context.Background(), 999, 42)
	require.NoError(t, err)
	require.False(t, online)
}

// TestPresenceRepo_RenewTTL_KeepsKeyAlive 验证 happy path：
// RenewTTL 让 key 不过期 —— TTL 5min 上 8s + RenewTTL + 8s = 16s 总流逝，
// 但中间 RenewTTL 让 TTL 重置回 5min；如果不续期，key 会在 5s 后就过期。
//
// 用 10s TTL 让窗口缩小：8s 后剩 2s；RenewTTL → TTL 回到 10s；再 8s 后还剩 2s。
// 没 RenewTTL 的对照是不必要的（前面已有 TTL expire case 覆盖）。
func TestPresenceRepo_RenewTTL_KeepsKeyAlive(t *testing.T) {
	mr, _ := testhelper.NewMiniRedis(t)
	client := redisinfra.NewRedisClientFromMiniredis(t, mr)
	repo := redisrepo.NewPresenceRepo(client, 10*time.Second)
	ctx := context.Background()

	require.NoError(t, repo.AddOnline(ctx, 100, 42, "s1"))

	mr.FastForward(8 * time.Second) // TTL 还剩 2s
	require.NoError(t, repo.RenewTTL(ctx, 100, 42))
	mr.FastForward(8 * time.Second) // 续期后再走 8s（共 16s；如果没续期早过期了）

	online, err := repo.IsOnline(ctx, 100, 42)
	require.NoError(t, err)
	require.True(t, online, "RenewTTL 应让 key 不过期")
}

// TestPresenceRepo_RemoveOnline_NotExists_NoError 验证 edge idempotent：
// 多次 RemoveOnline 同一 user 不抛 error（与 SessionManager.Unregister
// 多次调用 idempotent 行为一致 —— Lua script GET 返 false 走 no-op 删除分支，
// 底层 SREM / DEL 对不存在 key 都是 no-op）。
func TestPresenceRepo_RemoveOnline_NotExists_NoError(t *testing.T) {
	repo, _, _ := newPresenceRepo(t)
	ctx := context.Background()

	// 直接 RemoveOnline 没 AddOnline 过的 user
	require.NoError(t, repo.RemoveOnline(ctx, 100, 42, "session-x"))
	// 再调一次依然无 error
	require.NoError(t, repo.RemoveOnline(ctx, 100, 42, "session-x"))
}

// TestPresenceRepo_RemoveOnline_ReconnectRace_GuardsNewSession 验证 Story 10.6 r1
// P1 修的关键场景：reconnect 路径下旧 Session 的延后 Unregister 钩子调
// RemoveOnline(oldSessionID) 时，user:{id}:ws_session 已被新 Session 覆盖为
// newSessionID → sessionID guard 让 SREM/DEL no-op，新 Session 的 presence 完整保留。
//
// 时序模拟（与 SessionManager.Register 替换路径行为一致）：
//  1. old session: AddOnline(roomID, userID, oldSessionID)
//  2. new session: AddOnline(roomID, userID, newSessionID) 覆盖 user:{id}:ws_session
//  3. old session 异步 Unregister 钩子触发: RemoveOnline(roomID, userID, oldSessionID)
//     → 期望: no-op（不清新 session 的 presence）
//  4. IsOnline / ListOnline 仍返 user 在线（新 session active）
//  5. user:{id}:ws_session 仍 == newSessionID（未被误删）
func TestPresenceRepo_RemoveOnline_ReconnectRace_GuardsNewSession(t *testing.T) {
	repo, _, client := newPresenceRepo(t)
	ctx := context.Background()

	const roomID = uint64(100)
	const userID = uint64(42)
	const oldSessionID = "session-old"
	const newSessionID = "session-new"

	// Step 1: old session AddOnline
	require.NoError(t, repo.AddOnline(ctx, roomID, userID, oldSessionID))

	// Step 2: new session AddOnline 覆盖 user:{id}:ws_session
	require.NoError(t, repo.AddOnline(ctx, roomID, userID, newSessionID))

	// 确认 user:{id}:ws_session 已切到 newSessionID
	val, err := client.Get(ctx, "user:42:ws_session")
	require.NoError(t, err)
	require.Equal(t, newSessionID, val)

	// Step 3: old session 延后 Unregister 钩子触发: RemoveOnline(oldSessionID)
	// → 期望 no-op，不清新 session 的 presence
	require.NoError(t, repo.RemoveOnline(ctx, roomID, userID, oldSessionID))

	// Step 4: IsOnline / ListOnline 仍返 user 在线（presence 完整保留）
	online, err := repo.IsOnline(ctx, roomID, userID)
	require.NoError(t, err)
	require.True(t, online, "新 session 的 presence 不应被旧 session 的延后 RemoveOnline 误删")

	users, err := repo.ListOnline(ctx, roomID)
	require.NoError(t, err)
	require.Equal(t, []uint64{userID}, users)

	// Step 5: user:{id}:ws_session 仍 == newSessionID（未被误删）
	val, err = client.Get(ctx, "user:42:ws_session")
	require.NoError(t, err)
	require.Equal(t, newSessionID, val, "user:{id}:ws_session 应保留 newSessionID（旧 RemoveOnline 不应清）")
}

// TestPresenceRepo_RemoveOnline_MatchingSessionID_ClearsPresence 验证
// sessionID guard 命中路径：旧 session 在新 session 出现**之前** Unregister
// （正常关闭路径，无 reconnect race），传入的 sessionID 与 user:{id}:ws_session
// 当前值匹配 → 走删除分支，presence 被清。
func TestPresenceRepo_RemoveOnline_MatchingSessionID_ClearsPresence(t *testing.T) {
	repo, _, client := newPresenceRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.AddOnline(ctx, 100, 42, "session-abc"))
	require.NoError(t, repo.RemoveOnline(ctx, 100, 42, "session-abc"))

	online, err := repo.IsOnline(ctx, 100, 42)
	require.NoError(t, err)
	require.False(t, online, "sessionID 匹配 → presence 应被清")

	// user:{id}:ws_session 应被 DEL
	val, err := client.Get(ctx, "user:42:ws_session")
	require.NoError(t, err)
	require.Empty(t, val, "user:{id}:ws_session 应被 DEL")
}

// TestPresenceRepo_RemoveOnline_TTLExpiredKey_StillCleansSetMember 验证
// edge：user:{id}:ws_session 因 TTL 自然过期被清除（GET 返 false），
// 但 room set 内的 member 因 TTL 偏差还残留 → script 走 "key 不存在 → 仍执行
// SREM/DEL no-op 兜底" 分支，把残留 member 清干净。
//
// 这是"sessionID guard 不能反过来漏清残留"的反向验证 —— 如果 script 严格
// "key 不存在就跳过 SREM"，会让 SREM 漏清残留。
func TestPresenceRepo_RemoveOnline_TTLExpiredKey_StillCleansSetMember(t *testing.T) {
	repo, _, client := newPresenceRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.AddOnline(ctx, 100, 42, "session-abc"))

	// 模拟 user:{id}:ws_session TTL 提前过期：手动 DEL
	_, err := client.Del(ctx, "user:42:ws_session")
	require.NoError(t, err)

	// 此时 GET 返 nil；script 走 "key 不存在" 分支，仍执行 SREM/DEL
	require.NoError(t, repo.RemoveOnline(ctx, 100, 42, "session-abc"))

	// room set 内 member 应被清干净
	online, err := repo.IsOnline(ctx, 100, 42)
	require.NoError(t, err)
	require.False(t, online, "TTL 过期路径下 SREM 应仍清残留 member")
}

// TestPresenceRepo_ListOnline_EmptyRoom_ReturnsEmptySlice 验证 edge：
// 空 room（从未 Add 过）ListOnline 返 ([], nil)，不视为 error
// （与 SMembers 内化语义一致）。
func TestPresenceRepo_ListOnline_EmptyRoom_ReturnsEmptySlice(t *testing.T) {
	repo, _, _ := newPresenceRepo(t)

	online, err := repo.ListOnline(context.Background(), 999)
	require.NoError(t, err)
	require.Empty(t, online)
}

// TestPresenceRepo_NewPresenceRepo_DefaultTTL 验证构造函数 ttl <= 0
// 兜底为 5 分钟（与 godoc 钦定一致；让构造侧不必复制默认值常量）。
func TestPresenceRepo_NewPresenceRepo_DefaultTTL(t *testing.T) {
	mr, _ := testhelper.NewMiniRedis(t)
	client := redisinfra.NewRedisClientFromMiniredis(t, mr)
	// ttl 传 0 → 默认 5 分钟
	repo := redisrepo.NewPresenceRepo(client, 0)
	ctx := context.Background()

	require.NoError(t, repo.AddOnline(ctx, 100, 42, "s1"))

	// FastForward 4 分钟（< 5 分钟默认 TTL）应仍在线
	mr.FastForward(4 * time.Minute)
	online, err := repo.IsOnline(ctx, 100, 42)
	require.NoError(t, err)
	require.True(t, online, "4 min 后还在 5 min TTL 默认窗口内")

	// FastForward 再 2 分钟（共 6 分钟 > 5 分钟默认 TTL）应已过期
	mr.FastForward(2 * time.Minute)
	online, err = repo.IsOnline(ctx, 100, 42)
	require.NoError(t, err)
	require.False(t, online, "6 min 后超过 5 min 默认 TTL，应已过期")
}
