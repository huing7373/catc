package config

type Config struct {
	Server ServerConfig `yaml:"server"`
	MySQL  MySQLConfig  `yaml:"mysql"`
	Log    LogConfig    `yaml:"log"`
}

type ServerConfig struct {
	// BindHost 是 HTTP listener 绑定的 host 部分。空串（默认）= 绑所有接口
	// （0.0.0.0），与生产部署一致、Windows 环境会触发防火墙弹窗。测试场景
	// 注入 "127.0.0.1" → loopback-only，Windows Defender Firewall 对 loopback
	// 免检，避免每次 go test 新 hash 的 bootstrap.test.exe 反复弹授权窗。
	//
	// YAML key 可省略；仅在需要绑特定网卡 / localhost-only 场景时填。
	BindHost        string `yaml:"bind_host"`
	HTTPPort        int    `yaml:"http_port"`
	ReadTimeoutSec  int    `yaml:"read_timeout_sec"`
	WriteTimeoutSec int    `yaml:"write_timeout_sec"`
}

// MySQLConfig 是 MySQL 接入参数。Story 4.2 引入；选型 / DSN 策略 / 连接池参数
// 由 ADR-0003 (`_bmad-output/implementation-artifacts/decisions/0003-orm-stack.md`) 钦定。
//
// 字段不在 config 包做业务校验（无 Validate / Normalize 方法），fail-fast
// 由 `internal/infra/db.Open` 承担：DSN 为空或 ping 失败时直接返 error，
// main.go 走 `slog.Error + os.Exit(1)`。
type MySQLConfig struct {
	// DSN 是完整 MySQL 连接串（go-sql-driver/mysql 格式）。
	//
	// 本地默认（local.yaml）：
	//   cat:catdev@tcp(127.0.0.1:3306)/cat?charset=utf8mb4&parseTime=true&loc=Local
	//
	// 生产 / staging：通过环境变量 `CAT_MYSQL_DSN` 覆盖（loader.go 已挂；
	// CLAUDE.md "配置格式：YAML，支持环境变量覆盖" 钦定）。DSN 含密码 → 不入
	// 仓库的 YAML，部署侧用 K8s Secret 注入。
	//
	// 关键 query 参数：
	//   - charset=utf8mb4   必须；emoji / 用户名昵称 / 表情商品 4 字节字符
	//   - parseTime=true    必须；让 driver 把 DATETIME(3) 自动 parse 成 time.Time
	//                       （docs/宠物互动App_数据库设计.md §3.2）
	//   - loc=Local         本地时区。生产建议改 UTC（数据库侧也设 UTC）
	DSN string `yaml:"dsn"`

	// MaxOpenConns 是 *sql.DB 池的最大打开连接数。推荐 50（ADR-0003 §3.4）。
	// 0 = 无限制（不推荐生产用，会耗尽 MySQL `max_connections`）。
	MaxOpenConns int `yaml:"max_open_conns"`

	// MaxIdleConns 是空闲连接保活数。推荐 10。
	// 长连接保活避免每次请求 reconnect 增加延迟。
	MaxIdleConns int `yaml:"max_idle_conns"`

	// ConnMaxLifetimeSec 是连接最大存活时间（秒）。推荐 1800（30 min）。
	//
	// MySQL server 端 wait_timeout 默认 28800s（8h），但中间件 / LB 可能
	// 更短切 idle 连接 → *sql.DB 池里 idle 连接被切后下次复用会报
	// "invalid connection"。30 分钟主动 refresh 规避此类问题。
	//
	// 0 = 不限制（连接永远不主动 refresh，不推荐生产用）。
	ConnMaxLifetimeSec int `yaml:"conn_max_lifetime_sec"`
}

type LogConfig struct {
	Level string `yaml:"level"`
}
