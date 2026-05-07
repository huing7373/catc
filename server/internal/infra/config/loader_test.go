package config

import (
	"strings"
	"testing"
)

const fixturePath = "testdata/local.yaml"

func TestLoad_Happy(t *testing.T) {
	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Server.HTTPPort != 8080 {
		t.Errorf("Server.HTTPPort = %d, want 8080", cfg.Server.HTTPPort)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "info")
	}
}

func TestLoad_FileMissing(t *testing.T) {
	_, err := Load("testdata/nonexistent.yaml")
	if err == nil {
		t.Fatalf("Load returned nil error for missing file, want error")
	}
	if !strings.Contains(err.Error(), "config file not found") {
		t.Errorf("error message = %q, want substring %q", err.Error(), "config file not found")
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv(envHTTPPort, "9999")

	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Server.HTTPPort != 9999 {
		t.Errorf("Server.HTTPPort = %d, want 9999", cfg.Server.HTTPPort)
	}
}

func TestLoad_EnvInvalidInt(t *testing.T) {
	t.Setenv(envHTTPPort, "notanumber")

	_, err := Load(fixturePath)
	if err == nil {
		t.Fatalf("Load returned nil error for invalid env, want error")
	}
	if !strings.Contains(err.Error(), envHTTPPort) {
		t.Errorf("error message = %q, want substring %q", err.Error(), envHTTPPort)
	}
}

// TestLoad_MySQLDSNEnvOverride 验证 CAT_MYSQL_DSN 环境变量覆盖 YAML 的 mysql.dsn。
// 这是 staging / prod 部署注入 DB secret 的标准入口（不把密码写进 ConfigMap）。
// Story 4.2 review 补漏，CLAUDE.md "配置格式：YAML，支持环境变量覆盖" 钦定。
func TestLoad_MySQLDSNEnvOverride(t *testing.T) {
	const overrideDSN = "u:p@tcp(prod-mysql:3306)/cat?charset=utf8mb4&parseTime=true&loc=UTC"
	t.Setenv(envMySQLDSN, overrideDSN)

	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.MySQL.DSN != overrideDSN {
		t.Errorf("MySQL.DSN = %q, want %q (env override should win)", cfg.MySQL.DSN, overrideDSN)
	}
}

// TestLoad_MySQLDSNNoEnv_KeepsYAMLDefault 验证未设 CAT_MYSQL_DSN 时
// loader 不动 cfg.MySQL.DSN（保留 YAML 默认值，可能为空 → 由 db.Open 做 fail-fast）。
func TestLoad_MySQLDSNNoEnv_KeepsYAMLDefault(t *testing.T) {
	// 显式 unset 防 host 环境污染：t.Setenv 设空串再 unset 不可行，但 testdata fixture
	// 本身没 mysql 段（DSN 默认空）—— 验证空环境变量 == 空 DSN 即可。
	t.Setenv(envMySQLDSN, "")

	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.MySQL.DSN != "" {
		t.Errorf("MySQL.DSN = %q, want empty (fixture has no mysql section + env empty)", cfg.MySQL.DSN)
	}
}

// TestLoad_AuthDefaultTokenExpireSec 验证 fixture 没显式写 auth.token_expire_sec
// 时，loader 兜底为 604800（7 天，epics.md §Story 4.4 行 1014 钦定）。
// Story 4.4 引入。
func TestLoad_AuthDefaultTokenExpireSec(t *testing.T) {
	// 防 env 污染（host 环境若设了 CAT_AUTH_TOKEN_SECRET 不影响本 case 的 expire 默认值）
	t.Setenv(envAuthTokenSecret, "")

	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Auth.TokenExpireSec != 604800 {
		t.Errorf("Auth.TokenExpireSec = %d, want 604800 (fixture has no auth section → loader default)", cfg.Auth.TokenExpireSec)
	}
	// fixture 没 auth 段 → TokenSecret 也应为空（让 auth.New fail-fast）
	if cfg.Auth.TokenSecret != "" {
		t.Errorf("Auth.TokenSecret = %q, want empty (fixture has no auth section)", cfg.Auth.TokenSecret)
	}
}

// TestLoad_AuthTokenSecretEnvOverride 验证 CAT_AUTH_TOKEN_SECRET 环境变量
// 覆盖 YAML 的 auth.token_secret。Story 4.4 引入。
//
// 这是 staging / prod 部署注入 JWT signing secret 的标准入口（不把 secret
// 写进 ConfigMap / 仓库）。CLAUDE.md "配置格式：YAML，支持环境变量覆盖" 钦定；
// 与 4.2 review lesson `2026-04-26-config-env-override-and-gorm-auto-ping.md`
// "infrastructure 接入必须配齐 env override" 一致。
func TestLoad_AuthTokenSecretEnvOverride(t *testing.T) {
	const overrideSecret = "prod-secret-from-vault-must-be-at-least-16-bytes"
	t.Setenv(envAuthTokenSecret, overrideSecret)

	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Auth.TokenSecret != overrideSecret {
		t.Errorf("Auth.TokenSecret = %q, want %q (env override should win)", cfg.Auth.TokenSecret, overrideSecret)
	}
}

// TestLoad_AuthYAMLParsing 验证 YAML 含 auth: 段时正确解析（用专属 fixture）。
// Story 4.4 引入。
func TestLoad_AuthYAMLParsing(t *testing.T) {
	t.Setenv(envAuthTokenSecret, "") // 防 host 环境污染

	cfg, err := Load("testdata/auth.yaml")
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Auth.TokenSecret != "yaml-only-secret-must-be-at-least-16-bytes" {
		t.Errorf("Auth.TokenSecret = %q, want %q", cfg.Auth.TokenSecret, "yaml-only-secret-must-be-at-least-16-bytes")
	}
	if cfg.Auth.TokenExpireSec != 3600 {
		t.Errorf("Auth.TokenExpireSec = %d, want 3600 (explicit YAML value, no env override)", cfg.Auth.TokenExpireSec)
	}
}

// TestLoad_RateLimitDefaults 验证 fixture 没显式写 ratelimit: 段时，loader
// 兜底为 (60, 60, 10000)（Story 4.5 引入；epics.md §Story 4.5 行 1039 + V1 §4.1
// 行 218 钦定默认每 key 每分钟 60；BurstSize 默认 = PerKeyPerMin；BucketsLimit
// 默认 10000）。
//
// 4.5 round 2 [P2] 改成 *int64：YAML 缺字段 → loader 应填默认（pointer 非 nil 且 deref = 默认值）。
func TestLoad_RateLimitDefaults(t *testing.T) {
	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.RateLimit.PerKeyPerMin == nil {
		t.Fatalf("RateLimit.PerKeyPerMin = nil, want non-nil pointer to 60 (loader should fill default)")
	}
	if *cfg.RateLimit.PerKeyPerMin != 60 {
		t.Errorf("*RateLimit.PerKeyPerMin = %d, want 60", *cfg.RateLimit.PerKeyPerMin)
	}
	if cfg.RateLimit.BurstSize == nil {
		t.Fatalf("RateLimit.BurstSize = nil, want non-nil pointer to 60 (= PerKeyPerMin default)")
	}
	if *cfg.RateLimit.BurstSize != 60 {
		t.Errorf("*RateLimit.BurstSize = %d, want 60 (= PerKeyPerMin default)", *cfg.RateLimit.BurstSize)
	}
	if cfg.RateLimit.BucketsLimit == nil {
		t.Fatalf("RateLimit.BucketsLimit = nil, want non-nil pointer to 10000")
	}
	if *cfg.RateLimit.BucketsLimit != 10000 {
		t.Errorf("*RateLimit.BucketsLimit = %d, want 10000", *cfg.RateLimit.BucketsLimit)
	}
}

// TestLoad_RateLimitYAMLParsing 验证 YAML 显式写 ratelimit: 段时正确解析。
// Story 4.5 引入。
func TestLoad_RateLimitYAMLParsing(t *testing.T) {
	cfg, err := Load("testdata/ratelimit.yaml")
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.RateLimit.PerKeyPerMin == nil || *cfg.RateLimit.PerKeyPerMin != 120 {
		got := "nil"
		if cfg.RateLimit.PerKeyPerMin != nil {
			got = itoa64(*cfg.RateLimit.PerKeyPerMin)
		}
		t.Errorf("*RateLimit.PerKeyPerMin = %s, want 120 (explicit YAML)", got)
	}
	if cfg.RateLimit.BurstSize == nil || *cfg.RateLimit.BurstSize != 30 {
		got := "nil"
		if cfg.RateLimit.BurstSize != nil {
			got = itoa64(*cfg.RateLimit.BurstSize)
		}
		t.Errorf("*RateLimit.BurstSize = %s, want 30 (explicit YAML)", got)
	}
	if cfg.RateLimit.BucketsLimit == nil || *cfg.RateLimit.BucketsLimit != 5000 {
		got := "nil"
		if cfg.RateLimit.BucketsLimit != nil {
			got = itoa64(*cfg.RateLimit.BucketsLimit)
		}
		t.Errorf("*RateLimit.BucketsLimit = %s, want 5000 (explicit YAML)", got)
	}
}

// TestLoad_RateLimitPartialFields 验证 YAML 仅显式写一部分 ratelimit 字段时，
// 其它字段走默认值（per_key_per_min: 30 显式 → BurstSize 兜底成 30；
// BucketsLimit 兜底成 10000）。Story 4.5 引入。
func TestLoad_RateLimitPartialFields(t *testing.T) {
	cfg, err := Load("testdata/ratelimit_partial.yaml")
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.RateLimit.PerKeyPerMin == nil || *cfg.RateLimit.PerKeyPerMin != 30 {
		t.Errorf("*RateLimit.PerKeyPerMin want 30 (explicit YAML); got %v", cfg.RateLimit.PerKeyPerMin)
	}
	// burst_size 未显式 → 兜底 = PerKeyPerMin (30)
	if cfg.RateLimit.BurstSize == nil || *cfg.RateLimit.BurstSize != 30 {
		t.Errorf("*RateLimit.BurstSize want 30 (default = PerKeyPerMin); got %v", cfg.RateLimit.BurstSize)
	}
	// buckets_limit 未显式 → 默认 10000
	if cfg.RateLimit.BucketsLimit == nil || *cfg.RateLimit.BucketsLimit != 10000 {
		t.Errorf("*RateLimit.BucketsLimit want 10000 (default); got %v", cfg.RateLimit.BucketsLimit)
	}
}

// TestLoad_RateLimitExplicitZero_PreservedNotDefaulted 验证 YAML 显式写
// `per_key_per_min: 0` 时，loader **必须**保留为 *0（不静默替换成默认 60）。
//
// 这是 4.5 round 2 [P2] 拦下的反向纠偏：旧实现 `int + == 0 兜底` 会把显式 0 替
// 换成 60 → 用户期望禁限频或拼写错被掩盖 → 启动正常但策略不符预期。
//
// 修复后语义：YAML key 提供（pointer 非 nil）→ loader 透传不改；fail-fast 由
// middleware.RateLimit 工厂的 *cfg.PerKeyPerMin <= 0 → panic 路径处理。
func TestLoad_RateLimitExplicitZero_PreservedNotDefaulted(t *testing.T) {
	cfg, err := Load("testdata/ratelimit_zero.yaml")
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.RateLimit.PerKeyPerMin == nil {
		t.Fatalf("RateLimit.PerKeyPerMin = nil, want non-nil pointer to 0 (YAML explicit 0 must be preserved)")
	}
	if *cfg.RateLimit.PerKeyPerMin != 0 {
		t.Errorf("*RateLimit.PerKeyPerMin = %d, want 0 (YAML explicit 0 must NOT be replaced with default 60)",
			*cfg.RateLimit.PerKeyPerMin)
	}
}

// TestLoad_LogFileEnvOverride 验证 CAT_LOG_FILE 环境变量覆盖 YAML 的 log.file。
// 与 mysql.dsn / auth.token_secret 的 env override 同模式。
func TestLoad_LogFileEnvOverride(t *testing.T) {
	const overridePath = "/var/log/catserver-from-env.log"
	t.Setenv(envLogFile, overridePath)

	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Log.File != overridePath {
		t.Errorf("Log.File = %q, want %q (env override should win)", cfg.Log.File, overridePath)
	}
}

// TestLoad_LogFileNoEnv_KeepsYAMLDefault 验证未设 CAT_LOG_FILE 时
// loader 不动 cfg.Log.File（fixture 没 log.file → 空串，等价于"只写 stdout"）。
func TestLoad_LogFileNoEnv_KeepsYAMLDefault(t *testing.T) {
	t.Setenv(envLogFile, "")

	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Log.File != "" {
		t.Errorf("Log.File = %q, want empty (fixture has no log.file + env empty)", cfg.Log.File)
	}
}

// TestLoad_RateLimitOmitted_DefaultedTo60 验证 YAML 完全不写 ratelimit 段时，
// loader 兜底 PerKeyPerMin = 60（pointer 非 nil 且 deref = 60）。
//
// 与 TestLoad_RateLimitExplicitZero_PreservedNotDefaulted 配对：
// nil（YAML omitted）→ 填默认；&0（YAML explicit 0）→ 透传。
//
// 注意：local.yaml fixture 没 ratelimit 段，已经被 TestLoad_RateLimitDefaults
// 覆盖；本测试用同一 fixture 重新断言 omitted 路径，强调 `nil → default` 语义。
func TestLoad_RateLimitOmitted_DefaultedTo60(t *testing.T) {
	cfg, err := Load(fixturePath) // local.yaml: 无 ratelimit 段
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.RateLimit.PerKeyPerMin == nil {
		t.Fatalf("RateLimit.PerKeyPerMin = nil, want non-nil pointer (YAML omitted → loader must fill default)")
	}
	if *cfg.RateLimit.PerKeyPerMin != 60 {
		t.Errorf("*RateLimit.PerKeyPerMin = %d, want 60 (YAML omitted → default 60)",
			*cfg.RateLimit.PerKeyPerMin)
	}
}

// TestLoad_RedisAddrEnvOverride 验证 CAT_REDIS_ADDR 环境变量覆盖 YAML 的 redis.addr。
// 这是 staging / prod 部署注入 Redis 连接地址的标准入口。Story 10.2 引入；
// 与 mysql.dsn / auth.token_secret 同模式（CLAUDE.md "配置格式：YAML，支持环境变量覆盖"）。
func TestLoad_RedisAddrEnvOverride(t *testing.T) {
	const overrideAddr = "prod-redis.example.com:6380"
	t.Setenv(envRedisAddr, overrideAddr)

	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Redis.Addr != overrideAddr {
		t.Errorf("Redis.Addr = %q, want %q (env override should win)", cfg.Redis.Addr, overrideAddr)
	}
}

// TestLoad_RedisPasswordEnvOverride 验证 CAT_REDIS_PASSWORD 环境变量覆盖
// YAML 的 redis.password。Story 10.2 引入；密码含密钥语义不入仓 YAML，部署侧
// 用 K8s Secret 注入 env，与 mysql.dsn / auth.token_secret 同模式。
func TestLoad_RedisPasswordEnvOverride(t *testing.T) {
	const overridePass = "prod-redis-secret-from-vault"
	t.Setenv(envRedisPassword, overridePass)

	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Redis.Password != overridePass {
		t.Errorf("Redis.Password = %q, want %q (env override should win)", cfg.Redis.Password, overridePass)
	}
}

// TestLoad_RedisNoEnv_KeepsYAMLDefault 验证未设 CAT_REDIS_ADDR / CAT_REDIS_PASSWORD 时
// loader 不动 cfg.Redis.{Addr,Password}（保留 YAML 默认或留空让 redis.Open fail-fast）。
// fixturePath 没 redis 段 → Addr 默认空、Password 默认空，PoolSize 兜底默认 10。
func TestLoad_RedisNoEnv_KeepsYAMLDefault(t *testing.T) {
	t.Setenv(envRedisAddr, "")
	t.Setenv(envRedisPassword, "")

	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Redis.Addr != "" {
		t.Errorf("Redis.Addr = %q, want empty (fixture has no redis section + env empty)", cfg.Redis.Addr)
	}
	if cfg.Redis.Password != "" {
		t.Errorf("Redis.Password = %q, want empty", cfg.Redis.Password)
	}
}

// TestLoad_RedisPoolSizeDefault 验证 fixture 没显式写 redis.pool_size 时，
// loader 兜底为 10（Story 10.2 引入；epics.md §Story 10.2 行 1671 钦定）。
// fixturePath 没 redis 段 → cfg.Redis.PoolSize 是 zero-value 0 → loader 兜底成 10。
func TestLoad_RedisPoolSizeDefault(t *testing.T) {
	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Redis.PoolSize != 10 {
		t.Errorf("Redis.PoolSize = %d, want 10 (fixture has no redis section → loader default)", cfg.Redis.PoolSize)
	}
	// DB 没兜底（0 是合法值），fixture 没 redis 段 → DB == 0 也是预期
	if cfg.Redis.DB != 0 {
		t.Errorf("Redis.DB = %d, want 0 (default db, no fallback in loader)", cfg.Redis.DB)
	}
}

// TestLoad_RedisPoolSizeNegative_FallbackToDefault 验证 YAML 显式写
// `redis.pool_size: -1`（或任何负数）时，loader 必须兜底成默认值 10。
//
// 这是 10.2 round 1 [P2] 拦下的正确性修复：go-redis NewClient(Options{PoolSize: -1})
// 内部 makechan 会 panic("makechan: size out of range")，绕过 redis.Open 的
// fail-fast 路径 → 进程 SIGABRT 而不是 startup error。loader 必须把负值（YAML
// 拼错 / ConfigMap 注入异常）挤到合法范围，让 server 用默认 pool 启动。
//
// 与 TestLoad_RedisPoolSizeDefault 配对：== 0 兜底 + 负值兜底 = 注释里"<=0 → 默认"
// 的实现。
func TestLoad_RedisPoolSizeNegative_FallbackToDefault(t *testing.T) {
	cfg, err := Load("testdata/redis_negative_pool.yaml")
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Redis.PoolSize != 10 {
		t.Errorf("Redis.PoolSize = %d, want 10 (negative YAML value must fallback to default)", cfg.Redis.PoolSize)
	}
}

// TestLoad_RedisPresenceTTLSecDefault 验证 fixture 没显式写 redis.presence_ttl_sec
// 时，loader 兜底为 300（5 分钟；docs/数据库设计.md §9.1 钦定）。Story 10.6 引入。
//
// fixturePath 没 redis 段 → cfg.Redis.PresenceTTLSec 是 zero-value 0 → loader 兜底成 300。
func TestLoad_RedisPresenceTTLSecDefault(t *testing.T) {
	cfg, err := Load(fixturePath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Redis.PresenceTTLSec != 300 {
		t.Errorf("Redis.PresenceTTLSec = %d, want 300 (fixture has no redis section → loader default)", cfg.Redis.PresenceTTLSec)
	}
}

// TestLoad_RedisPresenceTTLSecExplicitYAML 验证 YAML 显式写 redis.presence_ttl_sec
// 正值时正确解析（不被 loader 默认值覆盖）。Story 10.6 引入。
func TestLoad_RedisPresenceTTLSecExplicitYAML(t *testing.T) {
	cfg, err := Load("testdata/redis_presence_ttl.yaml")
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Redis.PresenceTTLSec != 30 {
		t.Errorf("Redis.PresenceTTLSec = %d, want 30 (explicit YAML)", cfg.Redis.PresenceTTLSec)
	}
}

// TestLoad_RedisPresenceTTLSecNegative_FallbackToDefault 验证 YAML 显式写
// `redis.presence_ttl_sec: -10`（或任何非正值）时，loader 必须兜底成默认值 300。
//
// 与 RedisPoolSizeNegative 同模式：防 K8s ConfigMap 注入异常 / YAML 拼错让 server
// 起来后 presence TTL 异常（如 EXPIRE -10 在 Redis 上表现不定）。Story 10.6 引入。
func TestLoad_RedisPresenceTTLSecNegative_FallbackToDefault(t *testing.T) {
	cfg, err := Load("testdata/redis_presence_ttl_negative.yaml")
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Redis.PresenceTTLSec != 300 {
		t.Errorf("Redis.PresenceTTLSec = %d, want 300 (negative YAML must fallback to default)", cfg.Redis.PresenceTTLSec)
	}
}

// TestLoad_RedisYAMLParsing 验证 YAML 含 redis: 段时正确解析（用专属 fixture）。
// Story 10.2 引入。
func TestLoad_RedisYAMLParsing(t *testing.T) {
	t.Setenv(envRedisAddr, "")     // 防 host 环境污染
	t.Setenv(envRedisPassword, "") // 防 host 环境污染

	cfg, err := Load("testdata/redis.yaml")
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Redis.Addr != "127.0.0.1:6379" {
		t.Errorf("Redis.Addr = %q, want %q", cfg.Redis.Addr, "127.0.0.1:6379")
	}
	if cfg.Redis.Password != "yaml-only-pass" {
		t.Errorf("Redis.Password = %q, want %q", cfg.Redis.Password, "yaml-only-pass")
	}
	if cfg.Redis.DB != 3 {
		t.Errorf("Redis.DB = %d, want 3 (explicit YAML)", cfg.Redis.DB)
	}
	if cfg.Redis.PoolSize != 25 {
		t.Errorf("Redis.PoolSize = %d, want 25 (explicit YAML; not defaulted)", cfg.Redis.PoolSize)
	}
}

// TestLoad_WSDefaults 验证 fixture 没显式写 ws: 段时，loader 兜底
// HeartbeatTimeoutSec=60 / MaxMessageSizeBytes=16384 / WriteTimeoutSec=5。
// Story 10.3 引入；V1 §12.2 钦定 60s / 16 KB；WriteTimeoutSec 5s 非契约。
func TestLoad_WSDefaults(t *testing.T) {
	cfg, err := Load(fixturePath) // local.yaml fixture 没 ws 段
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.WS.HeartbeatTimeoutSec != 60 {
		t.Errorf("WS.HeartbeatTimeoutSec = %d, want 60 (default)", cfg.WS.HeartbeatTimeoutSec)
	}
	if cfg.WS.MaxMessageSizeBytes != 16384 {
		t.Errorf("WS.MaxMessageSizeBytes = %d, want 16384 (default)", cfg.WS.MaxMessageSizeBytes)
	}
	if cfg.WS.WriteTimeoutSec != 5 {
		t.Errorf("WS.WriteTimeoutSec = %d, want 5 (default)", cfg.WS.WriteTimeoutSec)
	}
}

// TestLoad_WSYAMLParsing 验证 YAML 显式写 ws: 段时正确解析。Story 10.3 引入。
func TestLoad_WSYAMLParsing(t *testing.T) {
	cfg, err := Load("testdata/ws.yaml")
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.WS.HeartbeatTimeoutSec != 30 {
		t.Errorf("WS.HeartbeatTimeoutSec = %d, want 30 (explicit YAML)", cfg.WS.HeartbeatTimeoutSec)
	}
	if cfg.WS.MaxMessageSizeBytes != 8192 {
		t.Errorf("WS.MaxMessageSizeBytes = %d, want 8192 (explicit YAML)", cfg.WS.MaxMessageSizeBytes)
	}
	if cfg.WS.WriteTimeoutSec != 10 {
		t.Errorf("WS.WriteTimeoutSec = %d, want 10 (explicit YAML)", cfg.WS.WriteTimeoutSec)
	}
}

// TestLoad_WSExplicitZero_FallbackToDefault 验证 YAML 显式写 0 时 loader 兜底
// 默认值（HeartbeatTimeoutSec / MaxMessageSizeBytes / WriteTimeoutSec 都没有
// "显式 0 = 禁用功能"的合法语义；与 RedisPoolSize <= 0 → 默认 同模式）。
// Story 10.3 引入。
func TestLoad_WSExplicitZero_FallbackToDefault(t *testing.T) {
	cfg, err := Load("testdata/ws_zero.yaml")
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.WS.HeartbeatTimeoutSec != 60 {
		t.Errorf("WS.HeartbeatTimeoutSec = %d, want 60 (zero YAML must fallback to default)", cfg.WS.HeartbeatTimeoutSec)
	}
	if cfg.WS.MaxMessageSizeBytes != 16384 {
		t.Errorf("WS.MaxMessageSizeBytes = %d, want 16384 (zero YAML must fallback to default)", cfg.WS.MaxMessageSizeBytes)
	}
	if cfg.WS.WriteTimeoutSec != 5 {
		t.Errorf("WS.WriteTimeoutSec = %d, want 5 (zero YAML must fallback to default)", cfg.WS.WriteTimeoutSec)
	}
}

// TestLoad_WSNegative_FallbackToDefault 验证 YAML 显式写负值时 loader 兜底默认值。
// 与 RedisPoolSizeNegative 同模式 —— 防 K8s ConfigMap 注入异常 / YAML 拼错让 server
// 起来后行为异常（如 SetReadLimit(-1) 行为不定）。Story 10.3 引入。
func TestLoad_WSNegative_FallbackToDefault(t *testing.T) {
	cfg, err := Load("testdata/ws_negative.yaml")
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.WS.HeartbeatTimeoutSec != 60 {
		t.Errorf("WS.HeartbeatTimeoutSec = %d, want 60 (negative YAML must fallback)", cfg.WS.HeartbeatTimeoutSec)
	}
	if cfg.WS.MaxMessageSizeBytes != 16384 {
		t.Errorf("WS.MaxMessageSizeBytes = %d, want 16384 (negative YAML must fallback)", cfg.WS.MaxMessageSizeBytes)
	}
	if cfg.WS.WriteTimeoutSec != 5 {
		t.Errorf("WS.WriteTimeoutSec = %d, want 5 (negative YAML must fallback)", cfg.WS.WriteTimeoutSec)
	}
}

// itoa64 是简化版 strconv.FormatInt（避免在 _test.go 引入额外 import）。
func itoa64(i int64) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
