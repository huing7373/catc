package redis

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/huing/cat/server/internal/infra/config"
)

// Open 按 cfg 打开 go-redis 连接 + 启动期 Ping fail-fast。
//
// 失败路径（**不**容忍降级，全部直接返 error）：
//   - cfg.Addr == "" → 立刻返 errors.New("redis.addr is empty")，不调驱动
//   - PingContext 失败（network unreachable / authentication failed / wrong db / ...）
//     → close + return fmt.Errorf("redis ping: %w", err)
//
// 调用方（cmd/server/main.go）失败时走 `slog.Error + os.Exit(1)`，不允许"用空 client 继续启动"
// （与 db.Open 同 fail-fast 模式；MEMORY.md "No Backup Fallback" 钦定）。
//
// 成功路径：返 (RedisClient, nil)；进程退出前调用方负责 client.Close() 释放连接池。
//
// **必须用本地短 timeout ctx**（与 db.Open 同模式）：主 signal-ctx 没 deadline，
// 碰到 blackhole host / 慢 DNS 时 PingContext 会被 driver 卡 30s+，fail-fast 实际不快。
// main.go 用 redisOpenTimeout = 5s 强制启动期 5s 内见结果。
func Open(ctx context.Context, cfg config.RedisConfig) (RedisClient, error) {
	if cfg.Addr == "" {
		return nil, errors.New("redis.addr is empty")
	}

	rc := goredis.NewClient(&goredis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
		// ContextTimeoutEnabled 必须显式开启 —— go-redis/v9 默认 false，此时
		// baseClient.context() 直接 fallback 到 context.Background()，所有命令
		// **忽略** caller 传入的 ctx（cancel / deadline 都不生效）。
		//
		// 后果（如果不开）：
		//   - main.go redisOpenTimeout = 5s 的 ctx 对 Ping 不生效，碰 blackhole
		//     host 实际卡到 socket-level timeout（ReadTimeout/DialTimeout，默认 3s/5s
		//     但与 ctx deadline 解耦）—— fail-fast 表现退化到驱动 socket timeout
		//   - handler 收到 client cancel / request timeout 后，下游 RedisClient
		//     方法仍裸跑命令到底，违反 ADR-0007 "ctx 必传 + 必生效" 钦定
		//
		// v9.7.0 源码参考：redis.go baseClient.context() 在该选项 false 时返回
		// context.Background()，命令路径完全绕开传入 ctx 的 deadline / cancel 检查。
		ContextTimeoutEnabled: true,
	})

	// fail-fast：ping 失败必须启动失败。失败原因可能是 network unreachable / auth /
	// Redis server 未起来 / DB 编号超出范围 —— 都不应该让 server 假装启动成功后
	// 在第一个业务请求时报错。与 db.Open 同模式。
	if err := rc.Ping(ctx).Err(); err != nil {
		// ping 失败时主动关闭已分配的连接池，避免 leak
		_ = rc.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return &redisClient{client: rc}, nil
}

// redisClient 是 RedisClient 接口的默认实装，包装 *goredis.Client。
//
// 不导出（lowercase）—— 所有调用方走 RedisClient 接口，避免业务代码绕过抽象层
// 拿到底层 *goredis.Client。如 future Story 需要 Pipeline / Pub/Sub / Lua 等
// 7 命令外的能力，**新增** RedisClient 接口方法 + 在本 struct 加实现，而非破抽象。
//
// 与 db.Open 不导出底层 *gorm.DB / *sql.DB 同思路（mysql.go 顶部注释钦定）。
//
// closeOnce 保证 Close 幂等（多次调用都返 nil / 第一次的 err）。go-redis 自身
// Close 在第二次调用会返 "redis: client is closed"，与本接口注释承诺的"多次调用
// 不报错"冲突 —— 用 sync.Once 在抽象层补齐契约（main.go defer + test t.Cleanup
// + 业务 caller 自己再 Close 都安全）。
type redisClient struct {
	client    *goredis.Client
	closeOnce sync.Once
	closeErr  error
}

// Get 实装 RedisClient.Get。
//
// 关键边界：go-redis 在 key 不存在时返 redis.Nil error，本抽象层把 redis.Nil
// 转换成 ("", nil)（不向上透传），让上层业务代码不需要每次再 errors.Is 判断。
// epics.md §Story 10.2 行 1679 钦定。
func (c *redisClient) Get(ctx context.Context, key string) (string, error) {
	val, err := c.client.Get(ctx, key).Result()
	if errors.Is(err, goredis.Nil) {
		return "", nil
	}
	return val, err
}

// Set 实装 RedisClient.Set。
//
// 路径分支：
//   - nx == true → 走 SET key value [EX seconds] NX 单命令；
//     返 (true, nil) = set 成功；返 (false, nil) = key 已存在未 set
//     （go-redis 在 NX 失败时返 redis.Nil error，本层转成 (false, nil)）
//   - nx == false → 走 SET key value [EX seconds] 普通 SET；
//     永远返 (true, nil)（除非命令真出错）
//
// expiration == 0 → 永不过期（go-redis Set / SetNX / SetXX 接 0 等价于 no expiration）。
//
// 为什么不拆成两个方法（Set + SetNX）：见 client.go RedisClient.Set 注释。
func (c *redisClient) Set(ctx context.Context, key, value string, expiration time.Duration, nx bool) (bool, error) {
	if nx {
		ok, err := c.client.SetNX(ctx, key, value, expiration).Result()
		if err != nil {
			return false, err
		}
		return ok, nil
	}
	if err := c.client.Set(ctx, key, value, expiration).Err(); err != nil {
		return false, err
	}
	return true, nil
}

// Del 实装 RedisClient.Del。
// key 不存在被 Redis 视为 "0 deleted"，不返 error；本层透传。
func (c *redisClient) Del(ctx context.Context, keys ...string) (int64, error) {
	return c.client.Del(ctx, keys...).Result()
}

// Expire 实装 RedisClient.Expire。
// key 不存在 Redis 返 (false, nil)；本层透传。
func (c *redisClient) Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	return c.client.Expire(ctx, key, expiration).Result()
}

// SAdd 实装 RedisClient.SAdd。
//
// 接口签名是 `members ...string`，但 go-redis 接 `...interface{}`；这里把每个 member
// 升格成 interface{} 转发。返 newly added count（已存在的 member 不计入）。
func (c *redisClient) SAdd(ctx context.Context, key string, members ...string) (int64, error) {
	args := make([]interface{}, len(members))
	for i, m := range members {
		args[i] = m
	}
	return c.client.SAdd(ctx, key, args...).Result()
}

// SRem 实装 RedisClient.SRem。
// 同 SAdd 升格 interface{}。返 actually removed count。
func (c *redisClient) SRem(ctx context.Context, key string, members ...string) (int64, error) {
	args := make([]interface{}, len(members))
	for i, m := range members {
		args[i] = m
	}
	return c.client.SRem(ctx, key, args...).Result()
}

// SMembers 实装 RedisClient.SMembers。
// set 不存在 Redis 返 ([], nil)；本层透传（与 Get 不存在 key 行为对称）。
func (c *redisClient) SMembers(ctx context.Context, key string) ([]string, error) {
	return c.client.SMembers(ctx, key).Result()
}

// Eval 实装 RedisClient.Eval。
//
// 透明 forward 到 go-redis Client.Eval；返回值 / error 不做内化（与接口注释钦定一致 ——
// 不同 script 语义不同，调用方按需 handle redis.Nil）。
//
// 路径：直接调 c.client.Eval(ctx, script, keys, args...).Result()；
// go-redis 自身负责 EVAL 命令编码、ARGV marshal、ctx 传播。
func (c *redisClient) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	return c.client.Eval(ctx, script, keys, args...).Result()
}

// Close 实装 RedisClient.Close。
//
// **真·幂等**（review r2 修法）：go-redis Client.Close 实际**不**幂等 —— 第二次
// 调用会返 "redis: client is closed"（v9.7.0 baseClient.Close 不挡重复关闭）。
// 本层用 sync.Once 包装：第一次真关 + 记 err；后续调用直接返第一次的 err（通常 nil）。
//
// 这样 caller 不需要关心调用次数：main.go 的 defer Close + test 的 t.Cleanup +
// 业务自己的 cleanup 路径可以叠加，**不**会拿到 spurious "client is closed" 错误。
// 与接口注释"必须幂等"承诺一致。
func (c *redisClient) Close() error {
	c.closeOnce.Do(func() {
		c.closeErr = c.client.Close()
	})
	return c.closeErr
}
