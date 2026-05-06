// Package redis 提供 Redis 连接初始化与抽象 RedisClient 接口。
//
// Story 10.2 引入；选型 / 默认值 / fail-fast 模式由 epics.md §Story 10.2（行 1661-1681）
// + Go 项目结构 §12.2（行 926-929）+ ADR-0007（context-propagation）钦定。
//
// 设计原则：
//   - **fail-fast over fallback**：Addr 空 / NewClient 失败 / Ping 失败都直接返 error，
//     不容忍降级（与 db.Open 同模式；MEMORY.md "No Backup Fallback"）。
//   - **不导出 *redis.Client**：让调用方走 RedisClient 接口；测试可注入
//     RedisClientMock（基于 miniredis）；future 切 Cluster / 加 Pipeline 时只改实装。
//   - **ctx 传播严格**（ADR-0007）：所有方法第一参数 ctx context.Context；
//     底层 go-redis 命令本身就是 ctx-aware。
//   - **error 语义内化**：Get 不存在 key 返 ("", nil)；SMembers 空 set 返 ([], nil)。
//     不向调用方透传 redis.Nil error，减少 boilerplate。
//
// 包名注意：与 go-redis 的 `github.com/redis/go-redis/v9` 包名都是 `redis`，
// 调用方（如 main.go / 业务 repo）建议用 alias `redisinfra` import 避免冲突。
package redis

import (
	"context"
	"time"
)

// RedisClient 抽象 Story 10.2 引入的 7 个基础 Redis 命令 + SET 的 NX/EX 选项。
//
// 选型决策：抽象层而非直接暴露 *redis.Client 是为了：
//  1. 测试替身：单测用 RedisClientMock（基于 miniredis）替换具体实装
//  2. future-proof：节点 9+ 切 Redis Cluster / 加 Pipeline 时只改实装不改调用方
//  3. 与 ADR-0007 ctx 传播一致：所有方法第一参数 ctx，禁止裸命令
//
// **设计边界**：本接口**只**含 epics.md §Story 10.2 行 1673 钦定的 7 个命令 +
// SET 的 NX/EX 选项（让 Epic 20 / 32 幂等键无需 raw 命令路径）。如果 future
// Story（10.6 / Epic 20 / 32）需要 INCR / HSET / Pub/Sub 等，**新增方法**而非
// 让调用方走 raw client 绕过接口（保持单一抽象边界，避免渐进式失控）。
//
// 所有命令必须 ctx-aware：传入的 ctx 通过 redis.Conn / WithContext 传递
// 给底层 driver；ctx 取消（如 request timeout）时命令必须中断而不是裸跑到底。
// ADR-0007 钦定。
type RedisClient interface {
	// Get 返回 key 对应的 value；key 不存在返 ("", nil)（不视为 error；与 epics.md
	// §Story 10.2 行 1679 "edge: GET 不存在的 key → 返回 nil（不抛 error）" 钦定一致）。
	Get(ctx context.Context, key string) (string, error)

	// Set 设置 key=value，可选 expiration / mode（NX = key 不存在时才设）。
	//
	// expiration == 0 → 永不过期；> 0 → 设为该 TTL（go-redis 接受 time.Duration）。
	// nx == true → 走 SET key value NX [EX] 路径；返 (true, nil) = set 成功；
	// 返 (false, nil) = key 已存在未 set。
	// nx == false → 走 SET key value [EX] 路径，永远返 (true, nil)（除非命令出错）。
	//
	// 为什么 NX 走同一接口而不是单独 SetNX 方法：
	//   - Epic 20 / 32 幂等键需 SET NX EX 原子组合（先 SETNX 再 EXPIRE 在 Redis 是
	//     非原子的，崩溃时间窗会让 key 永久存活）；go-redis 支持原生 SET NX EX
	//     单命令，本接口直接暴露 nx 参数让调用方一步到位
	//   - 减少接口面积（7 命令 + 选项 ≠ 9 命令）
	Set(ctx context.Context, key, value string, expiration time.Duration, nx bool) (bool, error)

	// Del 删除一个或多个 key；返成功删除的 key 数量。
	// key 不存在被视为 "0 deleted"，不视为 error。
	Del(ctx context.Context, keys ...string) (int64, error)

	// Expire 给 key 设 TTL；key 不存在返 (false, nil)；
	// 设置成功返 (true, nil)。
	Expire(ctx context.Context, key string, expiration time.Duration) (bool, error)

	// SAdd 把 members 加入 set；返 newly added count（已存在的 member 不计入）。
	SAdd(ctx context.Context, key string, members ...string) (int64, error)

	// SRem 从 set 删除 members；返 actually removed count。
	SRem(ctx context.Context, key string, members ...string) (int64, error)

	// SMembers 返回 set 全部 members；set 不存在返 ([], nil)（不视为 error；
	// 与 Get 不存在 key 返 nil 行为一致）。
	SMembers(ctx context.Context, key string) ([]string, error)

	// Close 关闭底层连接池；进程退出前由 main.go defer 调用一次。
	//
	// 必须**真·幂等**：多次调用全部返 nil（或第一次的 error），**不**返 spurious
	// "client is closed" 错误。与 *sql.DB.Close() 行为对齐。注意 go-redis 自身的
	// Close 不满足该契约（第二次返 "redis: client is closed"），所以实装层必须用
	// sync.Once 等机制补齐 —— review 2026-05-06 r2 钦定。
	Close() error
}
