// Package redis_test 是 Story 10.6 PresenceRepo 的黑盒单测。
//
// 测试栈：testify (require) + miniredis (in-process Redis) + RedisClient 抽象桥接。
// 与 server/internal/infra/redis/redis_test.go 既有 9 case 模式严格一致 ——
// 不写"纯 in-memory map" mock，复用 redisinfra.NewRedisClientFromMiniredis 跑真
// Redis 协议，覆盖 SADD/SREM 去重语义、TTL FastForward 等真实行为。
package redis_test

import (
	"context"
	"errors"
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

// TestPresenceRepo_RemoveOnline_ReconnectRace_GuardsUserKey 验证 Story 10.6 r1
// P1 + r4 P1 修后的 user key 保护语义：sessionID 不匹配（user key 已被新 session
// 覆盖）时**不**动 user key 的 DEL —— 仅 SREM 旧 room（r4 P1 修；让 cross-room
// reconnect 旧 room 总被清）。
//
// 同 room 的同步快照下 SREM 会让 user 短暂离开 room set，但生产路径上紧跟着
// 的 NEW AddOnline / scanner reconcile 会 SADD 把 user 加回，最终自愈。本 case
// 仅断言：r1 P1 的 user key 保护语义保留（sessionID guard 仍然在 DEL 路径生效）。
//
// 同 room 全态自愈链路由 TestPresenceRepo_RemoveOnline_SameRoomReconnect_SelfHealsAfterReAdd
// 覆盖；本 case 仅锁定"user key DEL 受 sessionID guard 保护"这一关键不变量。
func TestPresenceRepo_RemoveOnline_ReconnectRace_GuardsUserKey(t *testing.T) {
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
	// → 期望 user key 不动（sessionID guard 阻止 DEL）；但 SREM 旧 room 仍执行
	// （r4 P1 修：cross-room 不变量也覆盖 same-room 路径，user key 保护与 SREM 解耦）
	require.NoError(t, repo.RemoveOnline(ctx, roomID, userID, oldSessionID))

	// Step 4: user:{id}:ws_session 仍 == newSessionID（**关键不变量** —— DEL 受
	// sessionID guard 保护，不会被旧 session 的延后 RemoveOnline 误删，让上层
	// 后续 RemoveOnline(newSessionID) 路径仍能正确比对）
	val, err = client.Get(ctx, "user:42:ws_session")
	require.NoError(t, err)
	require.Equal(t, newSessionID, val, "user:{id}:ws_session 应保留 newSessionID（DEL 受 sessionID guard 保护）")
}

// TestPresenceRepo_RemoveOnline_SameRoomReconnect_SelfHealsAfterReAdd 验证
// review 10-6 r4 P1 修后的 same-room 自愈语义：旧 RemoveOnline（sessionID 不匹配）
// SREM 旧 room → user 暂离 room set；NEW session 的 AddOnline / scanner reconcile
// SADD 把 user 加回 → IsOnline 自愈。
//
// 时序：
//  1. AddOnline(roomA, userID, oldID)
//  2. AddOnline(roomA, userID, newID) —— 模拟 NEW session AddOnline 抢先（scanner
//     reconcile race / hook adapter 顺序）
//  3. RemoveOnline(roomA, userID, oldID) —— 旧 unregister 钩子；r4 P1 新 Lua case 3
//     SREM 旧 room（user 暂离）
//  4. AddOnline(roomA, userID, newID) —— 后续 scanner reconcile / NEW Register
//     onRegister 钩子；SADD 自愈
//
// 期望：最终 IsOnline 返 true、user key 仍 == newID。
func TestPresenceRepo_RemoveOnline_SameRoomReconnect_SelfHealsAfterReAdd(t *testing.T) {
	repo, _, client := newPresenceRepo(t)
	ctx := context.Background()

	const roomID = uint64(100)
	const userID = uint64(42)
	const oldSessionID = "session-old"
	const newSessionID = "session-new"

	require.NoError(t, repo.AddOnline(ctx, roomID, userID, oldSessionID))
	require.NoError(t, repo.AddOnline(ctx, roomID, userID, newSessionID))

	// 旧 RemoveOnline 走 Lua case 3 —— SREM 旧 room（user 暂离 set），保留 user key
	require.NoError(t, repo.RemoveOnline(ctx, roomID, userID, oldSessionID))

	// 中间瞬时态：user 不在 room set，但 user key 仍指 newID
	online, err := repo.IsOnline(ctx, roomID, userID)
	require.NoError(t, err)
	require.False(t, online, "SREM 后 user 暂离 room set（瞬时态，预期）")
	val, err := client.Get(ctx, "user:42:ws_session")
	require.NoError(t, err)
	require.Equal(t, newSessionID, val, "user key 受 sessionID guard 保护，不被旧 RemoveOnline DEL")

	// 后续 scanner reconcile / NEW Register 钩子的 AddOnline 把 user 加回
	require.NoError(t, repo.AddOnline(ctx, roomID, userID, newSessionID))

	online, err = repo.IsOnline(ctx, roomID, userID)
	require.NoError(t, err)
	require.True(t, online, "AddOnline 后 user 自愈回 room set")
	val, err = client.Get(ctx, "user:42:ws_session")
	require.NoError(t, err)
	require.Equal(t, newSessionID, val)
}

// TestPresenceRepo_RemoveOnline_CrossRoomReconnect_SREMsOldRoom 验证 review
// 10-6 r4 P1 修的核心场景：cross-room reconnect 路径下旧 RemoveOnline 的 SREM
// 必须执行（无论 sessionID 是否匹配）。
//
// 时序模拟（scanner reconcile race 路径）：
//  1. user 在 roomA：AddOnline(roomA, userID, oldID)
//  2. user 重连到 roomB：AddOnline(roomB, userID, newID) 覆盖 user key = newID +
//     SADD user 到 roomB（roomA set 仍有 user member）
//  3. 旧 unregister 钩子：RemoveOnline(roomA, userID, oldID)
//     → r4 P1 修前：Lua GET → newID ≠ oldID → 跳过 SREM → user 永久留在 roomA set
//     → r4 P1 修后：Lua GET → newID ≠ oldID → 走 case 3 → SREM roomA → user 离开 roomA set
//  4. ListOnline(roomA) 不含 user；ListOnline(roomB) 含 user
//
// 这是 r4 P1 修的**最核心 case**：旧版本 cross-room reconnect 让 user 同时在
// roomA + roomB 两个 set，roomA 其他活跃 user 周期续 TTL → user 永远看似 online
// 在 roomA。新版本 SREM 总是执行 → user 干净离开 roomA。
func TestPresenceRepo_RemoveOnline_CrossRoomReconnect_SREMsOldRoom(t *testing.T) {
	repo, _, client := newPresenceRepo(t)
	ctx := context.Background()

	const roomA = uint64(100)
	const roomB = uint64(200)
	const userID = uint64(42)
	const oldSessionID = "session-old"
	const newSessionID = "session-new"

	// Step 1: user 在 roomA
	require.NoError(t, repo.AddOnline(ctx, roomA, userID, oldSessionID))

	// Step 2: 重连到 roomB（覆盖 user key + SADD roomB；roomA set 仍有 user）
	require.NoError(t, repo.AddOnline(ctx, roomB, userID, newSessionID))

	// 中间状态：roomA + roomB 都含 user（user key 已切到 newID）
	online, err := repo.IsOnline(ctx, roomA, userID)
	require.NoError(t, err)
	require.True(t, online, "roomA 中 user 残留（AddOnline 不清旧 room）")
	online, err = repo.IsOnline(ctx, roomB, userID)
	require.NoError(t, err)
	require.True(t, online, "roomB 中 user 已加入")

	// Step 3: 旧 unregister 钩子触发 RemoveOnline(roomA, oldSessionID)
	// → 新 Lua case 3：SREM roomA（无论 sessionID 是否匹配），不动 user key
	require.NoError(t, repo.RemoveOnline(ctx, roomA, userID, oldSessionID))

	// Step 4: roomA 应不再含 user；roomB 仍含 user；user key 保留 newID
	online, err = repo.IsOnline(ctx, roomA, userID)
	require.NoError(t, err)
	require.False(t, online, "r4 P1 修：roomA 的 SREM 总是执行，user 应离开 roomA set")

	users, err := repo.ListOnline(ctx, roomA)
	require.NoError(t, err)
	require.Empty(t, users, "roomA 应空（user 唯一成员已被 SREM）")

	online, err = repo.IsOnline(ctx, roomB, userID)
	require.NoError(t, err)
	require.True(t, online, "roomB 不应受影响（RemoveOnline 仅清传入的 roomA）")

	val, err := client.Get(ctx, "user:42:ws_session")
	require.NoError(t, err)
	require.Equal(t, newSessionID, val, "user key 仍 == newSessionID（DEL 受 sessionID guard 保护）")
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

// ---------- review 10-6 r2 P2: AddOnline partial-fail 测试 ----------

// faultInjectingClient 把所有调用透传到底层 RedisClient，但允许测试代码控制
// 某次特定命令（SAdd/Set/Expire）失败。用 invocation count 让单测可决定"第 N
// 次调用某命令时返 error" —— 配合 review 10-6 r2 P2 修后的命令顺序
// （SET → SADD → EXPIRE）覆盖三种 partial-fail 场景。
//
// 设计：跟踪每个命令的调用次数，对照 *FailAt 字段判断是否在该次调用注入 error；
// nil *FailAt 表示该命令永不失败。
//
// **mock 边界**：本结构仅在本测试文件用，**不**导出 / **不**复用 ——
// production / 其他包测试需要类似 fault-injection 时各自定义独立 wrapper，
// 避免本结构被滥用导致 mock 行为偏离实际 RedisClient。
type faultInjectingClient struct {
	inner redisinfra.RedisClient

	setCount    int
	sAddCount   int
	expireCount int

	// 命令调用 N 次（1-indexed）时是否返 error；nil = 不注入错误
	setFailAt    *int
	sAddFailAt   *int
	expireFailAt *int
}

func intp(i int) *int { return &i }

var errFault = errors.New("injected fault")

func (f *faultInjectingClient) Get(ctx context.Context, key string) (string, error) {
	return f.inner.Get(ctx, key)
}

func (f *faultInjectingClient) Set(ctx context.Context, key, value string, expiration time.Duration, nx bool) (bool, error) {
	f.setCount++
	if f.setFailAt != nil && f.setCount == *f.setFailAt {
		return false, errFault
	}
	return f.inner.Set(ctx, key, value, expiration, nx)
}

func (f *faultInjectingClient) Del(ctx context.Context, keys ...string) (int64, error) {
	return f.inner.Del(ctx, keys...)
}

func (f *faultInjectingClient) Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	f.expireCount++
	if f.expireFailAt != nil && f.expireCount == *f.expireFailAt {
		return false, errFault
	}
	return f.inner.Expire(ctx, key, expiration)
}

func (f *faultInjectingClient) SAdd(ctx context.Context, key string, members ...string) (int64, error) {
	f.sAddCount++
	if f.sAddFailAt != nil && f.sAddCount == *f.sAddFailAt {
		return 0, errFault
	}
	return f.inner.SAdd(ctx, key, members...)
}

func (f *faultInjectingClient) SRem(ctx context.Context, key string, members ...string) (int64, error) {
	return f.inner.SRem(ctx, key, members...)
}

func (f *faultInjectingClient) SMembers(ctx context.Context, key string) ([]string, error) {
	return f.inner.SMembers(ctx, key)
}

func (f *faultInjectingClient) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	return f.inner.Eval(ctx, script, keys, args...)
}

func (f *faultInjectingClient) Close() error { return f.inner.Close() }

// newFaultInjectingRepo 构造带 fault injection 的 PresenceRepo + 暴露
// fault client 让测试 case 设 FailAt + 真 miniredis 让验证残留状态。
func newFaultInjectingRepo(t *testing.T) (redisrepo.PresenceRepo, *faultInjectingClient, redisinfra.RedisClient) {
	t.Helper()
	mr, _ := testhelper.NewMiniRedis(t)
	inner := redisinfra.NewRedisClientFromMiniredis(t, mr)
	fault := &faultInjectingClient{inner: inner}
	repo := redisrepo.NewPresenceRepo(fault, 5*time.Minute)
	return repo, fault, inner
}

// TestPresenceRepo_AddOnline_SetFails_NoLeftover 验证 review 10-6 r2 P2：
// SET 失败（命令顺序首位）→ AddOnline return → SADD 没执行 → 不留任何残留。
//
// 这是 P2 修后命令顺序（SET → SADD → EXPIRE）的第一类 partial-fail 场景；
// 也是修法的"最干净"路径 —— 修前是 SADD 在第一位，SADD 成功后 SET 失败让 room
// set 永远没 EXPIRE → zombie 永久存活；修后 SET 在第一位，SET 失败时其他命令
// 完全没动，无残留。
func TestPresenceRepo_AddOnline_SetFails_NoLeftover(t *testing.T) {
	repo, fault, inner := newFaultInjectingRepo(t)
	ctx := context.Background()

	// 第 1 次 Set 调用失败
	fault.setFailAt = intp(1)

	err := repo.AddOnline(ctx, 100, 42, "s1")
	require.Error(t, err, "SET 失败应让 AddOnline 返 error")

	// 验证 SAdd 未被调用（命令顺序首位是 SET，失败立即 return）
	require.Equal(t, 0, fault.sAddCount, "SET 失败 → SADD 不应被调用")
	require.Equal(t, 0, fault.expireCount, "SET 失败 → EXPIRE 不应被调用")

	// 验证 user:{id}:ws_session 没被写入（SET 失败前没数据）
	val, err := inner.Get(ctx, "user:42:ws_session")
	require.NoError(t, err)
	require.Empty(t, val, "SET 失败 → user:{id}:ws_session 不应有残留")

	// 验证 room set 也没被写入
	members, err := inner.SMembers(ctx, "room:100:online_users")
	require.NoError(t, err)
	require.Empty(t, members, "SET 失败 → room set 不应有残留")
}

// TestPresenceRepo_AddOnline_SAddFails_UserKeyHasTTL_RoomSetEmpty 验证 review
// 10-6 r2 P2：SET 成功 + SADD 失败 → user:{id}:ws_session 已写入但**有 TTL**
// （SET KEY VAL EX 单命令原子）→ 5min 后自然过期 → 不 zombie。room set 还没 SADD
// 过 → 没 member。
//
// 关键不变量：user:{id}:ws_session 必须**有** TTL（不能是永久 key），否则
// reconnect / 其他 user 路径会看到 stale sessionID 且永不自愈。
func TestPresenceRepo_AddOnline_SAddFails_UserKeyHasTTL_RoomSetEmpty(t *testing.T) {
	repo, fault, inner := newFaultInjectingRepo(t)
	ctx := context.Background()

	fault.sAddFailAt = intp(1)

	err := repo.AddOnline(ctx, 100, 42, "s1")
	require.Error(t, err, "SADD 失败应让 AddOnline 返 error")

	// 验证 EXPIRE 没被调用（SADD 失败立即 return）
	require.Equal(t, 0, fault.expireCount, "SADD 失败 → EXPIRE 不应被调用")

	// 验证 user:{id}:ws_session 已被 SET（含 TTL，因为 Set 走 KEY VAL EX 原子）
	val, err := inner.Get(ctx, "user:42:ws_session")
	require.NoError(t, err)
	require.Equal(t, "s1", val, "SET 已成功 → user:{id}:ws_session 应有值")

	// 验证 room set 没 member（SADD 失败前没数据）
	members, err := inner.SMembers(ctx, "room:100:online_users")
	require.NoError(t, err)
	require.Empty(t, members, "SADD 失败 → room set 不应有残留")

	// **关键**：下一次 AddOnline 同 user 应能恢复（覆盖 user:{id}:ws_session +
	// 重新 SADD），让"暂时性 SADD 失败"路径有自然 retry 通道。
	fault.sAddFailAt = nil // 解除 SADD 失败注入
	require.NoError(t, repo.AddOnline(ctx, 100, 42, "s2"))

	online, err := repo.IsOnline(ctx, 100, 42)
	require.NoError(t, err)
	require.True(t, online, "重试 AddOnline 应能完整恢复 presence")
}

// TestPresenceRepo_AddOnline_ExpireFails_UserKeyHasTTL 验证 review 10-6 r2 P2：
// SET + SADD 都成功 + EXPIRE 失败 → user:{id}:ws_session 已写入**有 TTL**，room
// set 没 TTL。Lesson 中详述：room set 没 TTL 是已知的轻度不一致 —— RemoveOnline
// 走 Lua script 仍能正确 SREM 干净；最坏路径是某 user 5min 内未 unregister 时
// room set 上残留 member 直到下一次 AddOnline 触发 EXPIRE 续期。
//
// 关键不变量：user:{id}:ws_session 仍**有 TTL**（5min），不会留永久 zombie key。
func TestPresenceRepo_AddOnline_ExpireFails_UserKeyHasTTL(t *testing.T) {
	mr, _ := testhelper.NewMiniRedis(t)
	inner := redisinfra.NewRedisClientFromMiniredis(t, mr)
	fault := &faultInjectingClient{inner: inner}
	repo := redisrepo.NewPresenceRepo(fault, 5*time.Minute)
	ctx := context.Background()

	fault.expireFailAt = intp(1)

	err := repo.AddOnline(ctx, 100, 42, "s1")
	require.Error(t, err, "EXPIRE 失败应让 AddOnline 返 error")

	// 验证 user:{id}:ws_session 有值（SET 已成功）
	val, err := inner.Get(ctx, "user:42:ws_session")
	require.NoError(t, err)
	require.Equal(t, "s1", val)

	// 验证 user:{id}:ws_session **有** TTL（由 Set KEY VAL EX 原子带入；不依赖
	// 后续 EXPIRE 命令）—— 走 FastForward 6 分钟后应过期
	mr.FastForward(6 * time.Minute)
	val, err = inner.Get(ctx, "user:42:ws_session")
	require.NoError(t, err)
	require.Empty(t, val, "user:{id}:ws_session 应在 5min TTL 后自然过期（验证 SET KEY VAL EX 已带 TTL）")
}
