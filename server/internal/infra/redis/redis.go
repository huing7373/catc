package redis

import (
	"context"
	"errors"
	"fmt"
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
type redisClient struct {
	client *goredis.Client
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

// Close 实装 RedisClient.Close。
//
// go-redis Client.Close 自身已经幂等（多次调用不 panic / 不返 error），
// 本层直接 wrap。与 *sql.DB.Close 行为一致。
func (c *redisClient) Close() error {
	return c.client.Close()
}
