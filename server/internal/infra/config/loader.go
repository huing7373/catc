package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

const (
	envHTTPPort = "CAT_HTTP_PORT"
	envLogLevel = "CAT_LOG_LEVEL"
	// envLogFile 是日志文件路径覆盖。空串视为"不覆盖"（保留 YAML 默认）；
	// 非空时 logger.Init 会同时写 stdout + 该文件。dev / 单机部署便利路径，
	// 生产仍推荐只写 stdout 让外部工具收集（12-Factor App）。
	envLogFile = "CAT_LOG_FILE"
	// envMySQLDSN 是 staging / prod 注入 DB secret 的标准入口。
	// CLAUDE.md "配置格式：YAML，支持环境变量覆盖" 钦定；DSN 含密码 → 不写进
	// 提交到仓库的 YAML，部署侧用环境变量或 K8s Secret 注入。Story 4.2 review
	// 补漏，参见 docs/lessons/2026-04-26-config-env-override-and-gorm-auto-ping.md。
	envMySQLDSN = "CAT_MYSQL_DSN"
	// envAuthTokenSecret 是 staging / prod 注入 JWT signing secret 的标准入口。
	// Story 4.4 引入。token_secret 含密钥语义 → 不入仓库 YAML，部署侧用 K8s
	// Secret / Vault 注入；与 4.2 mysql.dsn env override 同模式。
	envAuthTokenSecret = "CAT_AUTH_TOKEN_SECRET"
	// envRedisAddr 是 staging / prod 注入 Redis 连接地址的标准入口。Story 10.2 引入。
	// 与 mysql.dsn / auth.token_secret 同模式（CLAUDE.md "配置格式：YAML，支持环境变量覆盖"）。
	envRedisAddr = "CAT_REDIS_ADDR"
	// envRedisPassword 是 staging / prod 注入 Redis AUTH 密码的标准入口。Story 10.2 引入。
	// 含密钥语义 → 不入仓 YAML；部署侧用 K8s Secret 注入。
	// 注意：未给 CAT_REDIS_DB / CAT_REDIS_POOL_SIZE 加 env override —— 这两个字段非
	// secret 且 prod / dev 一致，YAML 配置足够；保持 env 表面积小（详见 story §AC4）。
	envRedisPassword = "CAT_REDIS_PASSWORD"

	defaultHTTPPort       = 8080
	defaultLogLevel       = "info"
	defaultTokenExpireSec = 604800 // 7 天，epics.md §Story 4.4 行 1014 钦定
	// defaultRateLimitPerKeyPerMin 是 rate_limit 中间件每 key 每分钟默认允许的请求数。
	// epics.md §Story 4.5 行 1039 + V1 §4.1 行 218 钦定。Story 4.5 引入。
	defaultRateLimitPerKeyPerMin = 60
	// defaultRateLimitBucketsLimit 是 rate_limit 中间件内存 bucket 数上限（防 IP 洪泛 OOM）。
	// 约 1MB 内存（每 limiter ~100 字节）。Story 4.5 引入。
	defaultRateLimitBucketsLimit = 10000
	// defaultRedisPoolSize 是 go-redis 连接池默认大小。
	// epics.md §Story 10.2 行 1671 钦定 `pool_size` 字段；节点 4 阶段 Redis QPS 不高，
	// 10 足够；节点 9+ 上 prod 按观测调整。Story 10.2 引入。
	defaultRedisPoolSize = 10
	// defaultRedisPresenceTTLSec 是 Story 10.6 引入的 PresenceRepo TTL 默认值
	// （5 分钟 = 300s）。选型由 docs/数据库设计.md §9.1 + 心跳节奏（heartbeat=60s /
	// scan=30s）钦定 —— 远大于心跳路径让 TTL 永不到，远小于运维容忍窗口让僵尸 user
	// 5 分钟内自然清。
	defaultRedisPresenceTTLSec = 300
	// minRedisPresenceTTLSec 是 PresenceTTLSec 显式 YAML 写入时的下限（review
	// 10-6 r3 P2 加）。HeartbeatScanner 内部 tick 频率写死 30s（heartbeatScanIntervalSec），
	// scanner 每次 tick 调 PresenceRepo.AddOnline 重写并续 TTL；如果 TTL ≤ 30s，
	// 连续两次 tick 之间 TTL 已过期 → user 闪烁 offline 即使 session 活跃。最小
	// 60s = 2 × tick 让 scanner 有足够 buffer 应对 IO 抖动；显式写更小值视为
	// 配置错误，loader fail-fast 让运维侧能立即注意到（与 mysql.dsn 空串 fail-fast
	// 同模式）。零值 / 负值仍走默认 300 兜底（YAML 缺字段路径不应被卡住）。
	// 详见 docs/lessons/2026-05-07-presence-ttl-min-vs-scan-interval-10-6-r3.md。
	minRedisPresenceTTLSec = 60
	// defaultWSHeartbeatTimeoutSec 是 WS Session 心跳超时阈值（秒）。
	// V1 §12.2 钦定 60s；prod 必须使用默认值（契约一部分）；Story 10.3 引入。
	defaultWSHeartbeatTimeoutSec = 60
	// defaultWSMaxMessageSizeBytes 是 WS 单条 frame 最大字节数。
	// V1 §12.2 关键约束钦定 16 KB；prod 必须使用默认值（契约一部分）；Story 10.3 引入。
	defaultWSMaxMessageSizeBytes = 16384
	// defaultWSWriteTimeoutSec 是 WS writeLoop conn.WriteMessage 的 deadline。
	// 非契约字段；prod / dev 都可调。Story 10.3 引入。
	defaultWSWriteTimeoutSec = 5
)

// isProdEnv 返回 true 当且仅当 `CAT_ENV` 既非 "dev" 也非 "staging" 也非 "test"
// （safe-by-default：未注入 / 拼写错都视为 prod，避免 dev YAML 静默漂到 prod）。
//
// **prod gate 模式**（与 service.NewStepService / wsapp.NewGateway 同模式）：本项目
// 不在 cfg 上挂 Env 字段，统一让需要 prod-only 强校验的位置就近读 `CAT_ENV` env var。
// 详见 docs/lessons/2026-05-07-presence-hooks-fire-and-forget-and-ttl-floor-env-gate-10-6-r6.md。
func isProdEnv() bool {
	switch os.Getenv("CAT_ENV") {
	case "dev", "staging", "test":
		return false
	default:
		return true
	}
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
		return nil, fmt.Errorf("config read failed: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("config parse failed: %w", err)
	}

	if v := os.Getenv(envHTTPPort); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid %s=%q: %w", envHTTPPort, v, err)
		}
		cfg.Server.HTTPPort = port
	}
	if v := os.Getenv(envLogLevel); v != "" {
		cfg.Log.Level = v
	}
	if v := os.Getenv(envLogFile); v != "" {
		cfg.Log.File = v
	}
	// MySQL DSN 含密码不入仓 → 部署侧通过 CAT_MYSQL_DSN 注入；空串视为
	// "不覆盖"（保留 YAML 默认或留空让 db.Open fail-fast）。
	if v := os.Getenv(envMySQLDSN); v != "" {
		cfg.MySQL.DSN = v
	}
	// Auth token secret 含密钥语义不入仓 → 部署侧通过 CAT_AUTH_TOKEN_SECRET 注入；
	// 空串视为 "不覆盖"（保留 YAML 默认或留空让 auth.New fail-fast）。
	if v := os.Getenv(envAuthTokenSecret); v != "" {
		cfg.Auth.TokenSecret = v
	}
	// Redis addr 通过 CAT_REDIS_ADDR 覆盖；空串视为 "不覆盖"（保留 YAML 默认或留空让
	// redis.Open fail-fast）。Story 10.2 引入；与 mysql.dsn / auth.token_secret 同模式。
	if v := os.Getenv(envRedisAddr); v != "" {
		cfg.Redis.Addr = v
	}
	// Redis password 含密钥语义不入仓 → 部署侧通过 CAT_REDIS_PASSWORD 注入；
	// 空串视为 "不覆盖"（保留 YAML 默认空串 = 无密码）。Story 10.2 引入。
	if v := os.Getenv(envRedisPassword); v != "" {
		cfg.Redis.Password = v
	}

	if cfg.Server.HTTPPort == 0 {
		cfg.Server.HTTPPort = defaultHTTPPort
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = defaultLogLevel
	}
	if cfg.Auth.TokenExpireSec == 0 {
		cfg.Auth.TokenExpireSec = defaultTokenExpireSec
	}
	// RateLimit 默认值兜底（Story 4.5 引入；4.5 round 2 [P2] 改成 *int64 pointer 模式）：
	// 仅当 YAML **未提供**字段（pointer == nil）时填默认；YAML **显式写了** 的值
	// （含 *0 / *负数）原样透传 → middleware.RateLimit 工厂的 fail-fast 路径处理。
	//
	// 反例（旧 `int + == 0 兜底`）：YAML 显式 `per_key_per_min: 0` → loader 静默
	// 替换成 60 → middleware 看不到 0 → 用户期望禁限频或拼写错被掩盖。
	//
	// 详见 docs/lessons/2026-04-26-yaml-default-must-not-mask-explicit-invalid.md
	// + config.go RateLimitConfig 顶部"为什么字段是 *int64"。
	if cfg.RateLimit.PerKeyPerMin == nil {
		v := int64(defaultRateLimitPerKeyPerMin)
		cfg.RateLimit.PerKeyPerMin = &v
	}
	if cfg.RateLimit.BurstSize == nil {
		// BurstSize 默认 = PerKeyPerMin（已经填过默认或保持 YAML 显式值）；
		// 注意：PerKeyPerMin 此处必非 nil（上一段已填默认）。
		v := *cfg.RateLimit.PerKeyPerMin
		cfg.RateLimit.BurstSize = &v
	}
	if cfg.RateLimit.BucketsLimit == nil {
		v := int64(defaultRateLimitBucketsLimit)
		cfg.RateLimit.BucketsLimit = &v
	}

	// Redis PoolSize 默认值兜底（Story 10.2 引入；10.2 round 1 [P2] 把 == 0 改成 <= 0）：
	// YAML 缺字段 / 显式 0 / 显式负数都视为"用默认值"（zero-value + 负值兜底；与 RateLimit
	// *int64 模式不同 —— PoolSize 没有"用户想禁用连接池"的真实业务语义，与 ≤0 → 兜底一致）。
	//
	// 为什么必须涵盖负数（不能只兜 == 0）：go-redis NewClient(Options{PoolSize: -1}) 会在
	// 内部 makechan 处直接 panic("makechan: size out of range")，绕过 redis.Open 的 fail-fast
	// 路径 → 启动时进程 SIGABRT 而不是返回 startup error。loader 兜底必须把负值挤到合法
	// 范围（默认值），让 server 用 default pool 起来；YAML 拼错 / K8s ConfigMap 注入异常
	// 不能让 server panic。
	//
	// 与 docs/lessons/2026-04-26-yaml-default-must-not-mask-explicit-invalid.md 一致：
	// 本字段无"显式 0 = 禁用功能"的合法用法，不需要区分 nil / *0。
	if cfg.Redis.PoolSize <= 0 {
		cfg.Redis.PoolSize = defaultRedisPoolSize
	}
	// Redis PresenceTTLSec 默认值兜底（Story 10.6 引入；与 PoolSize <= 0 → 默认 同模式）：
	// YAML 缺字段 / 显式 0 / 显式负数都视为"用默认值"（PresenceTTLSec 没有"显式 0 = 禁用"
	// 的合法业务语义；presence repo 必须有 TTL 才能在 server crash 后自然清僵尸记录）。
	if cfg.Redis.PresenceTTLSec <= 0 {
		cfg.Redis.PresenceTTLSec = defaultRedisPresenceTTLSec
	}
	// 下限校验（review 10-6 r3 P2 加；review 10-6 r6 P2 改环境感知）：
	// HeartbeatScanner 30s tick 调 AddOnline 重写 + 续 TTL；TTL ≤ 30s 让连续两次
	// tick 之间 keys 过期，long-lived session 闪烁 offline。最小 60s = 2 × tick 让
	// IO 抖动 / 调度延迟仍有 buffer。显式配置低于下限视为错误，fail-fast 让运维侧
	// 立即注意（与 mysql.dsn 空串模式一致）。零值已在上一段兜底成 300，不会进本分支。
	//
	// **环境感知**（review 10-6 r6 P2）：r3 引入硬下限后与 RedisConfig.PresenceTTLSec
	// 注释 + sample-config "dev / test 可短到 5s 走 miniredis FastForward" 直接冲突 ——
	// dev 按文档配 5s 启动失败。修法：仅在 prod env 强校验下限，dev / staging / test
	// 允许任意正值（< 60s 也允许，让 fast TTL 测试 / FastForward fixture 能跑）。
	// 与 Story 7.3 review r6 [P2] StepsConfig prod-only cap-override gate 同模式
	// （CAT_ENV != "dev|staging|test" 视为 prod，safe-by-default：未注入 / typo 都
	// 走 prod 严格策略，避免 dev YAML 静默漂到 prod）。
	if isProdEnv() && cfg.Redis.PresenceTTLSec < minRedisPresenceTTLSec {
		return nil, fmt.Errorf("redis.presence_ttl_sec=%d below minimum %d in prod (must be >= 2x heartbeat scan interval 30s; dev/test 覆盖必须 export CAT_ENV=dev|staging|test)",
			cfg.Redis.PresenceTTLSec, minRedisPresenceTTLSec)
	}
	// Redis Addr / DB 不在 loader 兜底：
	//   - Addr == "" 让 fail-fast 在 redis.Open 层暴露（与 mysql.dsn 同模式）
	//   - DB == 0 是合法值（默认 db）；YAML 显式 0 与缺字段都是 0，无需区分

	// WS 三字段默认值兜底（Story 10.3 引入；与 RedisPoolSize <= 0 → 默认 同模式）：
	// YAML 缺字段 / 显式 0 / 显式负数都视为"用默认值"（HeartbeatTimeoutSec / MaxMessageSizeBytes /
	// WriteTimeoutSec 都没有"显式 0 = 禁用功能"的合法语义；与 StepsConfig 同模式 —— zero-value
	// + 负值兜底语义清晰）。
	if cfg.WS.HeartbeatTimeoutSec <= 0 {
		cfg.WS.HeartbeatTimeoutSec = defaultWSHeartbeatTimeoutSec
	}
	if cfg.WS.MaxMessageSizeBytes <= 0 {
		cfg.WS.MaxMessageSizeBytes = defaultWSMaxMessageSizeBytes
	}
	if cfg.WS.WriteTimeoutSec <= 0 {
		cfg.WS.WriteTimeoutSec = defaultWSWriteTimeoutSec
	}

	return &cfg, nil
}
