package redis

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

// NewRedisClientFromMiniredis 接受一个已启动的 miniredis server，返回连到该 server
// 的 RedisClient 接口实例。client 在 t.Cleanup 自动关闭，无需调用方手动 Close。
//
// 用法：
//
//	mr, _ := testhelper.NewMiniRedis(t)         // pkg/testing/helpers.go 既有 helper
//	client := redisinfra.NewRedisClientFromMiniredis(t, mr)  // 本 story 加
//	val, err := client.Get(ctx, "foo")          // 走真 go-redis client + miniredis backend
//
// 与 NewMiniRedis 配合使用，让单测以 in-process 速度（< 10ms）跑通业务 redis 逻辑。
//
// 关键设计决策：
//   - **不**写一个"纯 in-memory map" Mock —— miniredis 已经是 in-process 实装，
//     准确度更高（覆盖 Redis 90%+ 命令、TTL FastForward、SADD 去重等真实语义）。
//     "in-memory map" 选项会让单测在 mock 下跑通但在真 Redis 下行为不一致。
//   - 本函数是**桥接**helper，不重复实装 mock；ADR-0001 §3.2 已锚定测试栈一致性。
//   - cleanup 顺序：本函数注册的 client.Close → testhelper.NewMiniRedis 注册的
//     miniredis.Close（t.Cleanup 是 LIFO，所以 client 先关）。
//
// 文件不带 `_test.go` 后缀（直接在包里），但仅依赖 `testing.T` + miniredis（test fixture），
// production binary 不会引用本函数（无引用 = dead code，链接器自动剔除）；
// 与 go-sqlmock helpers 同模式（pkg/testing/helpers.go）。
func NewRedisClientFromMiniredis(t *testing.T, mr *miniredis.Miniredis) RedisClient {
	t.Helper()
	rc := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() {
		_ = rc.Close()
	})
	return &redisClient{client: rc}
}
