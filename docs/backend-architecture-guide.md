# 后端架构开发规范（Backend Architecture Guide）

本文档是 cat（裤衩猫）Go 后端的**开发宪法**。架构与工程纪律**全面对齐 P2 游戏服务器**（作者验证过的生产实践），技术栈**尽量沿用 P2**，仅在下列两处偏离：

1. **不拆微服务**：cat 是 Apple Watch 伴侣应用，单体 + 内部模块化足够；P2 的 15+ 服务、route_server、registry 都不要。
2. **对外协议用 HTTP + WebSocket**：Watch/iOS 客户端无法调自定义 TCP protobuf RPC，HTTP/WS 是客户端可达的唯一协议。

其余（目录风格、Runnable 接口、TOML 配置、MongoDB、repository 接口+实现、typed IDs、callback 生命周期、table-driven tests）全部照抄 P2。

**适用范围**：`server/` 目录下所有 Go 代码。
**效力**：旧代码（Story 2-1 的 Postgres/GORM 版本）将按本规范重写；新代码直接遵守。
**冲突处理**：与本文档冲突的现存代码一律改文档为准。

---

## 1. 总体架构哲学（源自 P2）

1. **显式胜于隐式**：没有 DI 框架、没有 IoC 容器、没有全局单例。所有依赖在 `cmd/cat/initialize.go` 的 `initialize()` 函数里按正确顺序手工构造并注入。
2. **分层单向依赖**：handler → service（含 domain） → repository → infrastructure。严禁反向或跨层。
3. **接口消费方定义**（"accept interfaces, return structs"）：调用方声明契约，实现方返回具体结构体指针。不抄 P2 的 `IXxx` 前缀。
4. **错误码 > 错误字符串**：对外 `AppError`（code + message + httpStatus），内部 sentinel error + `errors.Is/As`。禁止 P2 式 `NewErrorMsg("登录失败: " + err.Error())`。
5. **结构化日志是唯一日志**：zerolog JSON 输出。禁止 `fmt.Printf` 或 P2 式 `log.Infof("[X] Y - uid:%d", uid)` 拼字符串。
6. **Context 贯穿到底**：所有 I/O 必传 `context.Context`。禁止 `context.TODO()`，`context.Background()` 仅用于启动期 / 后台 goroutine 根节点。
7. **数据访问只通过 repository**：service 与 handler **不能**直接持有 `*mongo.Client` / `*redis.Client`。

---

## 2. 技术栈（最终版）

| 类别 | 选择 | 说明 |
|---|---|---|
| 语言 | Go 1.25+ | 与 P2 对齐 |
| HTTP | `github.com/gin-gonic/gin` | 对外 API，客户端必须走 HTTP |
| WebSocket | `github.com/gorilla/websocket` | 实时触摸、好友猫状态 |
| 主库 | MongoDB（`go.mongodb.org/mongo-driver/v2`） | 文档模型适合皮肤/状态/好友等嵌套数据 |
| 缓存 | Redis（`github.com/redis/go-redis/v9`） | 会话、限流、热数据 |
| 配置 | TOML（`github.com/BurntSushi/toml`） | 仿 P2，比 env 更结构化 |
| 日志 | zerolog（`github.com/rs/zerolog`） | 结构化 JSON，**不用** P2 的 glog 封装 |
| JWT | `github.com/golang-jwt/jwt/v5` | Sign in with Apple + 自签 access/refresh |
| 校验 | `github.com/go-playground/validator/v10` | 通过 Gin binding 使用 |
| 测试 | `github.com/stretchr/testify` + table-driven | |
| 推送 | `github.com/sideshow/apns2` | |
| 定时 | `github.com/robfig/cron/v3` | |
| ID | `github.com/google/uuid` | UUID 字符串 |

**不采用**：GORM、Postgres、golang-migrate、env+`.env.local`、glog、protobuf、自定义 TCP RPC、route_server/registry。

---

## 3. 目录结构

```
server/
├── cmd/
│   └── cat/
│       ├── main.go            # 只做 flag.Parse → config.Load → initialize → app.Run
│       ├── initialize.go      # 所有依赖的显式装配（≤200 行）
│       ├── app.go             # App 容器 + Runnable 接口 + 信号处理
│       └── wire.go            # handler 聚合 struct、router build（可拆出）
├── internal/
│   ├── config/                # TOML 配置结构 + Load()
│   ├── domain/                # 领域实体 + 业务规则（值对象、状态机、不变量）
│   ├── service/               # 应用服务：跨 repo 协调、事务、副作用
│   ├── repository/            # 数据访问（interface + mongo impl 同目录）
│   ├── handler/               # Gin HTTP handler
│   ├── dto/                   # 请求/响应结构 + AppError + 响应辅助
│   ├── middleware/            # auth / logger / cors / ratelimit
│   ├── ws/                    # WebSocket hub + client + message router
│   ├── cron/                  # 定时任务
│   └── push/                  # APNs 封装
├── pkg/                       # 与业务无关的可复用库
│   ├── logx/                  # zerolog 初始化 + ctx 辅助
│   ├── mongox/                # mongo client 建立 + 健康检查 + 事务辅助
│   ├── redisx/                # redis client 建立 + 健康检查
│   ├── jwtx/                  # JWT 签发/校验
│   ├── fsm/                   # 从 P2 移植的有限状态机
│   └── ids/                   # typed ID 定义（UserID、SkinID...）
├── config/                    # default.toml、production.toml、local.toml
├── tools/                     # 一次性脚本（数据迁移、修复、批量导入）
├── scripts/                   # build.sh
├── deploy/                    # Dockerfile、docker-compose
└── go.mod
```

**规则**：
- `internal/` 包名 = 目录名，单数。
- `pkg/` 不得引用 `internal/`；`internal/` 可引用 `pkg/`。
- 一个子目录一个主类型时，文件名 = 类型 snake_case（`user_repo.go` 里的 `UserRepository`）。
- 测试与被测同目录，`*_test.go`。
- **不要**抄 P2 的 `base/` + `common/` + `pkg/` 三层拆分；单体里 `pkg/` 一级够用。

---

## 4. 入口与初始化（P2 风格）

`cmd/cat/main.go`（不超过 15 行）：

```go
package main

import "flag"

var configPath = flag.String("config", "config/local.toml", "path to toml config")

func main() {
    flag.Parse()
    cfg := config.MustLoad(*configPath)
    app := initialize(cfg)
    app.Run()
}
```

`cmd/cat/initialize.go`——**显式顺序**的构造：

```go
func initialize(cfg *config.Config) *App {
    // 1. 基础设施
    logx.Init(cfg.Log)
    mongoCli := mongox.MustConnect(cfg.Mongo)
    rdb := redisx.MustConnect(cfg.Redis)
    jwtMgr := jwtx.New(cfg.JWT)
    apns := push.NewAPNs(cfg.APNs)

    // 2. Repository 层（构造返回 *Struct；接口在消费方定义）
    userRepo := repository.NewUserRepo(mongoCli, rdb)
    skinRepo := repository.NewSkinRepo(mongoCli, rdb)
    friendRepo := repository.NewFriendRepo(mongoCli)

    // 3. Service 层
    authSvc := service.NewAuthService(userRepo, jwtMgr)
    userSvc := service.NewUserService(userRepo)
    skinSvc := service.NewSkinService(skinRepo, cfg.CDN.BaseURL)
    touchSvc := service.NewTouchService(friendRepo, apns)

    // 4. WebSocket hub
    hub := ws.NewHub(touchSvc)

    // 5. Handler 聚合
    h := &handlers{
        health: handler.NewHealthHandler(mongoCli, rdb),
        auth:   handler.NewAuthHandler(authSvc),
        user:   handler.NewUserHandler(userSvc),
        skin:   handler.NewSkinHandler(skinSvc),
        touch:  handler.NewTouchHandler(touchSvc),
        ws:     handler.NewWSHandler(hub, jwtMgr),
    }

    // 6. 路由
    router := buildRouter(cfg, h, jwtMgr)

    // 7. 定时任务
    scheduler := cron.New(userRepo, skinRepo)

    // 8. App 容器（持有所有 Runnable）
    return NewApp(cfg, router, hub, scheduler, mongoCli, rdb)
}
```

**纪律**：
- 构造函数内**不做 I/O**，除 `MustConnect` 的一次性连接+ping。
- 禁止 `func init()` 做任何业务 I/O（不读环境、不连 DB、不注册路由）。
- `initialize()` 是唯一能 `New*` 核心依赖的地方。

---

## 5. 生命周期：Runnable 接口（P2 风格）

```go
// cmd/cat/app.go
type Runnable interface {
    Name() string
    Start(ctx context.Context) error   // 启动；HTTP server 这类是阻塞在自己 goroutine
    Final(ctx context.Context) error   // 清理；必须幂等
}

type App struct {
    cfg  *config.Config
    runs []Runnable    // 按注册顺序 Start，逆序 Final
    stop chan os.Signal
}

func (a *App) Run() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    for _, r := range a.runs {
        go func(r Runnable) {
            if err := r.Start(ctx); err != nil {
                log.Fatal().Err(err).Str("runnable", r.Name()).Msg("start failed")
            }
        }(r)
    }

    signal.Notify(a.stop, os.Interrupt, syscall.SIGTERM)
    <-a.stop

    shutCtx, c := context.WithTimeout(context.Background(), 30*time.Second)
    defer c()
    for i := len(a.runs) - 1; i >= 0; i-- {
        if err := a.runs[i].Final(shutCtx); err != nil {
            log.Error().Err(err).Str("runnable", a.runs[i].Name()).Msg("final failed")
        }
    }
}
```

实现 `Runnable` 的对象：HTTP server、WebSocket hub、cron scheduler、mongo client、redis client。

---

## 6. 分层架构

### 6.1 Handler（`internal/handler/`）

**唯一职责**：HTTP ↔ service 翻译。
- 绑定请求（`ShouldBindJSON` / URI / Query）
- validator tag 校验
- 调用**一个** service 方法
- 翻译结果/错误成 HTTP 响应

**不允许**：直接持有 `*mongo.Client` / `*redis.Client`；做业务判断。

```go
type UserHandler struct {
    svc service.UserSvc      // 接口，在 service 包定义
}

func NewUserHandler(svc service.UserSvc) *UserHandler {
    return &UserHandler{svc: svc}
}

func (h *UserHandler) GetMe(c *gin.Context) {
    uid := middleware.UserIDFrom(c)
    u, err := h.svc.GetProfile(c.Request.Context(), uid)
    if err != nil {
        dto.RespondAppError(c, err)
        return
    }
    dto.RespondSuccess(c, http.StatusOK, dto.UserRespFromDomain(u))
}
```

### 6.2 Service（`internal/service/`）

**职责**：业务规则、跨仓库协调、事务边界、外部副作用（推送、CDN）。对 handler 暴露接口；对 repo 依赖接口（消费方定义）。

```go
package service

// UserSvc 是给 handler 的契约。
type UserSvc interface {
    GetProfile(ctx context.Context, id ids.UserID) (*domain.User, error)
    UpdateDisplayName(ctx context.Context, id ids.UserID, name string) error
}

// userRepo 是 service 需要的 repository 能力（消费方定义）。
type userRepo interface {
    FindByID(ctx context.Context, id ids.UserID) (*domain.User, error)
    UpdateDisplayName(ctx context.Context, id ids.UserID, name string) error
}

type UserService struct {
    repo userRepo
}

func NewUserService(repo userRepo) *UserService {
    return &UserService{repo: repo}
}

func (s *UserService) GetProfile(ctx context.Context, id ids.UserID) (*domain.User, error) {
    u, err := s.repo.FindByID(ctx, id)
    if err != nil {
        if errors.Is(err, repository.ErrNotFound) {
            return nil, ErrUserNotFound
        }
        return nil, fmt.Errorf("user service: find by id: %w", err)
    }
    return u, nil
}
```

**要点**：
- 构造返回 `*UserService`（具体）；handler 字段是 `UserSvc`（接口）。
- `userRepo` 接口在 service 包，不污染 repository 包。

### 6.3 Domain（`internal/domain/`）

**职责**：实体（非 BSON 结构）+ 业务规则 + 状态机 + 值对象。**不知道 HTTP / Mongo / Redis**。

```go
package domain

type User struct {
    ID          ids.UserID
    AppleID     string
    DisplayName string
    CreatedAt   time.Time
    IsDeleted   bool
}

// 业务规则示例
func (u *User) CanChangeName(newName string, cdCheck func() error) error {
    if u.DisplayName == newName {
        return ErrSameName
    }
    if err := ValidateNicknameLength(newName); err != nil {
        return err
    }
    if cdCheck != nil {
        if err := cdCheck(); err != nil { return err }
    }
    return nil
}
```

**纪律**：
- domain 实体和 mongo 的 BSON 结构**分开**。repository 层负责 domain ↔ BSON 互转。这抵消了 P2 里 `FromBson` 手写的问题。
- 状态机（猫的 Mirror/Active/Sleep 等）用 `pkg/fsm` 实现，domain 层持有 FSM 实例。

### 6.4 Repository（`internal/repository/`）

**组织**：一个领域一个文件；接口定义在同目录，暴露给 service 的消费方接口不强制在 repo 包里定义（P2 风格是在 repo 包里，cat 改为消费方定义，更 Go）。

**BSON 映射**：repo 内部私有 `userDoc` 结构 + `fromDoc/toDoc` 转换，domain 层看到的永远是 `*domain.User`。

```go
package repository

import "go.mongodb.org/mongo-driver/v2/mongo"

var (
    ErrNotFound = errors.New("repository: not found")
    ErrConflict = errors.New("repository: conflict")
)

type userDoc struct {
    ID          string    `bson:"_id"`
    AppleID     string    `bson:"apple_id"`
    DisplayName string    `bson:"display_name"`
    CreatedAt   time.Time `bson:"created_at"`
    IsDeleted   bool      `bson:"is_deleted"`
}

func (d *userDoc) toDomain() *domain.User { /* ... */ }
func docFromUser(u *domain.User) *userDoc  { /* ... */ }

type UserRepository struct {
    coll *mongo.Collection
    rdb  *redis.Client
}

func NewUserRepo(cli *mongo.Client, rdb *redis.Client) *UserRepository {
    return &UserRepository{
        coll: cli.Database("cat").Collection("users"),
        rdb:  rdb,
    }
}

func (r *UserRepository) FindByID(ctx context.Context, id ids.UserID) (*domain.User, error) {
    // 先查 Redis
    if u, ok := r.getCache(ctx, id); ok { return u, nil }

    var d userDoc
    err := r.coll.FindOne(ctx, bson.M{"_id": string(id), "is_deleted": false}).Decode(&d)
    if errors.Is(err, mongo.ErrNoDocuments) {
        return nil, ErrNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("user repo: find by id: %w", err)
    }

    u := d.toDomain()
    r.setCache(ctx, u)
    return u, nil
}
```

**纪律**：
- 每次查询 `ctx` 透传到 `mongo` / `redis` 调用。
- Mongo 的 `mongo.ErrNoDocuments` → repo 的 `ErrNotFound`；DuplicateKey → `ErrConflict`。
- 软删除一律 `is_deleted: false` 条件；**不用** timestamp 软删。
- **索引在代码里声明**：每个 repo 有 `EnsureIndexes(ctx context.Context) error` 方法，`initialize()` 启动期调用一次。
- 复杂聚合允许用 `aggregate pipeline`，但封装在 repo 私有方法里，service 看不到 pipeline。

### 6.5 DTO（`internal/dto/`）

请求/响应结构 + 标准错误 + 响应辅助。

```go
type LoginReq struct {
    AppleJWT string `json:"apple_jwt" binding:"required"`
    Nonce    string `json:"nonce" binding:"required,min=16,max=64"`
}

type UserResp struct {
    ID          string `json:"id"`
    DisplayName string `json:"display_name"`
}

func UserRespFromDomain(u *domain.User) UserResp {
    return UserResp{ID: string(u.ID), DisplayName: u.DisplayName}
}
```

**标准错误响应**：

```json
{ "error": { "code": "USER_NOT_FOUND", "message": "用户不存在" } }
```

---

## 7. 错误处理（AppError 模式）

```go
// internal/dto/app_error.go
type AppError struct {
    HTTPStatus int
    Code       string
    Message    string
    Wrapped    error   // 日志用，不回客户端
}

func (e *AppError) Error() string { return e.Code + ": " + e.Message }
func (e *AppError) Unwrap() error { return e.Wrapped }

// service 层定义
var (
    ErrUserNotFound   = &AppError{HTTPStatus: 404, Code: "USER_NOT_FOUND",   Message: "用户不存在"}
    ErrUnauthorized   = &AppError{HTTPStatus: 401, Code: "UNAUTHORIZED",     Message: "未授权"}
    ErrRateLimited    = &AppError{HTTPStatus: 429, Code: "RATE_LIMITED",     Message: "请求过于频繁"}
    ErrAppleAuthFail  = &AppError{HTTPStatus: 401, Code: "APPLE_AUTH_FAIL",  Message: "Apple 登录失败"}
)

func RespondAppError(c *gin.Context, err error) {
    var ae *AppError
    if errors.As(err, &ae) {
        log.Ctx(c.Request.Context()).Error().Err(ae.Wrapped).
            Str("code", ae.Code).Msg("app error")
        c.JSON(ae.HTTPStatus, gin.H{"error": gin.H{"code": ae.Code, "message": ae.Message}})
        return
    }
    log.Ctx(c.Request.Context()).Error().Err(err).Msg("unhandled error")
    c.JSON(500, gin.H{"error": gin.H{"code": "INTERNAL_ERROR", "message": "服务器内部错误"}})
}
```

**禁止**：
- 字符串匹配错误（`strings.Contains(err.Error(), "not found")`）。
- panic 替代 error（除 `log.Fatal` 启动期 / `log.Panic` 中间件兜底）。
- 把原始错误信息回给客户端（泄露 schema）。

---

## 8. 日志（zerolog，结构化字段）

**每条日志必带**（中间件注入，业务通过 `log.Ctx(ctx)` 继承）：
- `request_id`
- `user_id`（认证后）
- `endpoint`（METHOD + path）

```go
log.Ctx(ctx).Info().
    Str("user_id", string(uid)).
    Str("skin_id", string(sid)).
    Msg("skin equipped")
```

**级别**：
- `debug` 开发诊断（生产关）
- `info` 正常业务事件
- `warn` 可恢复异常
- `error` 需人工关注
- `fatal` 启动期致命，立即退出

**禁止**：
- `fmt.Printf` / `log.Printf` / `println`
- 打印完整 request body（可能含 token）
- 中文 `Msg` 值（业务含义放字段里）
- P2 式字符串拼接 `log.Infof("[X] uid:%d", uid)`

---

## 9. 配置（TOML，P2 风格）

`config/default.toml`：

```toml
[server]
port = 8080
mode = "release"   # debug | release

[log]
level = "info"
format = "json"

[mongo]
uri = "mongodb://localhost:27017"
database = "cat"
timeout_sec = 5

[redis]
addr = "localhost:6379"
password = ""
db = 0

[jwt]
access_secret = ""
refresh_secret = ""
access_ttl_min = 60
refresh_ttl_day = 30

[apns]
key_id = ""
team_id = ""
bundle_id = "com.example.cat"
key_path = ""

[cdn]
base_url = ""
upload_key = ""
```

**加载**：

```go
// internal/config/config.go
type Config struct {
    Server ServerCfg `toml:"server"`
    Log    LogCfg    `toml:"log"`
    Mongo  MongoCfg  `toml:"mongo"`
    Redis  RedisCfg  `toml:"redis"`
    JWT    JWTCfg    `toml:"jwt"`
    APNs   APNsCfg   `toml:"apns"`
    CDN    CDNCfg    `toml:"cdn"`
}

func MustLoad(path string) *Config {
    var c Config
    if _, err := toml.DecodeFile(path, &c); err != nil {
        log.Fatal().Err(err).Str("path", path).Msg("config load failed")
    }
    c.mustValidate()   // 敏感字段空值校验
    return &c
}
```

**纪律**：
- 所有代码禁止直接 `os.Getenv`（包括 secret）。敏感值从环境变量读入走 `config` 包的 `overrideFromEnv()` 辅助函数。
- `secret` 字段为空则 `log.Fatal`。
- 配置**只在 `initialize()` 读一次**，之后只读传参。

---

## 10. MongoDB 访问

### 10.1 连接

```go
// pkg/mongox/client.go
func MustConnect(cfg config.MongoCfg) *mongo.Client {
    ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutSec)*time.Second)
    defer cancel()
    cli, err := mongo.Connect(options.Client().ApplyURI(cfg.URI))
    if err != nil { log.Fatal().Err(err).Msg("mongo connect failed") }
    if err := cli.Ping(ctx, nil); err != nil { log.Fatal().Err(err).Msg("mongo ping failed") }
    return cli
}
```

### 10.2 索引管理

- 每个 repo 实现 `EnsureIndexes(ctx context.Context) error`。
- `initialize()` 启动时统一调用。
- 删除字段或改索引结构要在 `tools/` 放一次性脚本，线下执行。

### 10.3 事务

跨 collection 写操作用 session：

```go
// pkg/mongox/tx.go
func WithTx(ctx context.Context, cli *mongo.Client, fn func(sessCtx mongo.SessionContext) error) error {
    sess, err := cli.StartSession()
    if err != nil { return err }
    defer sess.EndSession(ctx)
    _, err = sess.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
        return nil, fn(sc)
    })
    return err
}
```

service 层调用 `mongox.WithTx(ctx, cli, func(sc) { ... })`；repo 方法需要支持接收已有 session 的 context。

### 10.4 文档 schema 演化

Mongo 无 DDL 迁移，但仍需纪律：
- 每个 collection 的文档结构定义成 `repository` 包里私有 struct（如 `userDoc`），字段改动视同 API 变更走 PR。
- 字段新增加默认值 / 兼容读；字段删除先经 shadow read 一个版本。
- 破坏性变更在 `tools/migrate_<name>/` 下写 Go 脚本。

---

## 11. Redis 缓存

**Key 命名函数集中**：

```go
// internal/repository/redis_keys.go
func userCacheKey(uid ids.UserID) string         { return "user:" + string(uid) }
func touchRateLimitKey(uid ids.UserID) string    { return "ratelimit:touch:" + string(uid) }
func tokenBlacklistKey(jti string) string        { return "token:blacklist:" + jti }
func skinListCacheKey(uid ids.UserID) string     { return "skins:owned:" + string(uid) }
```

**规则**：
- 不允许 key 字面量散落在代码里。
- 每个 `Set` 必须有 TTL，没有 TTL 的特例必须注释原因。
- Cache-aside：repo 先读 Redis，miss 再读 Mongo 并回填；写后立即 `Del`（不靠 TTL 自然失效）。

---

## 12. WebSocket

`internal/ws/`：

- **Hub**：管理所有 `Client`；`sync.Map` 持有 `uid → *Client`；消息路由用 channel。
- **Client**：每连接两个 goroutine（`readPump` / `writePump`），中间用带缓冲 channel 传消息。
- **认证**：Upgrade 前的 HTTP 阶段校验 JWT（query 或 header），注入 `uid` 进 `Client` 构造。
- **消息格式**：JSON，必含 `type` 字段。

```json
{ "type": "touch_send", "payload": { "to_uid": "..." } }
```

**背压**：`Client.send` chan 满→关闭该连接；**不能**阻塞 hub。
**心跳**：30s 一个 ping；60s 无 pong 关闭连接。

### 12.1 Message Registry（Story 0.14）

WS 消息类型的**唯一真相源**：`internal/dto/ws_messages.go`（`WSMessages` + `WSMessagesByType`）。本文件只描述模式；**当前已注册类型列表**在 `docs/api/ws-message-registry.md`（人类可读）和 `docs/api/openapi.yaml`（HTTP 端 `GET /v1/platform/ws-registry` schema）。

CI 漂移守门（双 gate，不得简化）：

1. **编译时 / 单元测试**：`internal/dto/ws_messages_test.go` 核对 `dto.WSMessages` 与 `*ws.Dispatcher.RegisteredTypes()` 一致（debug/release 两种模式各一个 case）。
2. **运行时 fail-fast**：`cmd/cat/initialize.go` 的 `validateRegistryConsistency` 在 `initialize()` 返回前校验；漂移则 `log.Fatal`。

新增消息四步走（出错顺序会被 CI 挡下，这是设计）：

1. `dto.WSMessages` 追加 entry（含 `Type/Version/Direction/RequiresAuth/RequiresDedup/DebugOnly/Description`）。
2. `cmd/cat/initialize.go` 调 `dispatcher.Register` / `RegisterDedup`。
3. `docs/api/ws-message-registry.md` 追加 `### <type>` 段。
4. `bash scripts/build.sh --test` 绿。

---

## 13. 中间件（`internal/middleware/`）

全局注册顺序：

1. `gin.Recovery()` — 最外层兜 panic。
2. `RequestLogger` — 生成 request_id，注入 ctx，结束时记一条访问日志。
3. `CORS` — 生产严格白名单。
4. `RateLimit` — 基于 Redis 令牌桶。
5. `AuthRequired` — **仅挂在** `/v1/...` 受保护组，不全局挂。

中间件**只做横切关注点**。`c.Set(UserIDKey, uid)` 传用户身份；handler 通过 `middleware.UserIDFrom(c)` 读。

---

## 14. 路由与 API 约定

- 所有业务挂 `/v1/...`。
- 路径 kebab-case；资源复数（`/v1/users/me`）；动作用子路径（`POST /v1/users/me/skins/:id/equip`）。
- 响应 JSON **snake_case**（与 iOS 客户端约定）。
- 成功：直接返回 payload，不包 `{data: ...}`。
- 失败：`{"error": {"code": "...", "message": "..."}}`。
- 分页：游标式 `{"items": [...], "next_cursor": "..."}`。

---

## 15. 代码风格

### 15.1 Import 分组

```go
import (
    "context"
    "errors"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/rs/zerolog/log"
    "go.mongodb.org/mongo-driver/v2/mongo"

    "github.com/huing7373/catc/server/internal/domain"
    "github.com/huing7373/catc/server/internal/repository"
    "github.com/huing7373/catc/server/pkg/ids"
)
```

顺序：标准库 → 第三方 → 本项目。别名仅在冲突时用。

### 15.2 命名

- 包名：全小写、单数、无下划线。
- 导出类型：`PascalCase`；私有：`camelCase`。
- 接口：行为词 + `er`（`Reader`）或领域词（`UserSvc`）；**不用 `I` 前缀**。
- 构造函数：`New<Type>`，返回 `*Type`；**禁止**构造内做业务 I/O。
- Receiver：1-3 字母（`(r *UserRepository)`）。
- 文件名：`snake_case.go`，匹配主要类型。

### 15.3 Typed IDs（强制，P2 经验）

```go
// pkg/ids/ids.go
type UserID string
type SkinID string
type FriendshipID string
type GiftID string
```

所有 ID 参数、返回值、结构体字段必须用 typed ID。杜绝 `string`↔`UserID` 混用。

### 15.4 注释

- 公开导出成员必须 godoc（首字母大写，以名称开头）。
- **注释统一英文**（P2 中英文混用是反例）。
- 注释写"为什么"，不写"是什么"。
- `// TODO(#123): ...` 必带 issue 号。

### 15.5 Context

- 任何可能 I/O 的函数第一参数 `ctx context.Context`。
- 禁止提交 `context.TODO()`。
- 启动期 / 后台 goroutine 根节点才允许 `context.Background()`，必须注释原因。

---

## 16. 测试

| 层 | 方法 | 工具 |
|---|---|---|
| repository | 集成测试，打真 Mongo（testcontainers 或本地 docker-compose） | testify |
| service | 单元测试 + 手写 mock repo | testify |
| handler | `httptest` + mock service | testify + Gin test context |
| e2e | `test/e2e/`，启动真 server + 真 Mongo/Redis | — |

**写法**：
- 表驱动优先；每 case 一个 `t.Run(name, ...)`。
- `require` 管前置，`assert` 管结果。
- **不要 sleep**；用 channel / `eventually` 辅助。
- 命名：`TestUserService_GetProfile_UserNotFound`。

**覆盖率目标**：service 层 ≥ 80%，repo / handler ≥ 60%。
**CI 门禁**：`bash scripts/build.sh --test` 必过。

---

## 17. 定时任务与推送

### 17.1 Cron（`internal/cron/`）
- 统一注册函数 `RegisterJobs(sch *cron.Cron, deps...)`。
- 任务签名 `func(ctx context.Context) error`，调度器 wrapper 包装 panic recover + logging。
- 任务**必须幂等**（可能被重复触发）。

### 17.2 推送（`internal/push/`）
- APNs 封装成 `Pusher` 接口，service 依赖接口。
- 推送失败**不阻塞**主业务：主业务写 `notification_queue` collection，后台 worker 异步推送；失败记录，重试最多 3 次后进死信。

---

## 18. P2 坏味道：明确不抄

| P2 坏味道 | cat 规定 |
|---|---|
| 字符串拼接日志 (`log.Infof("[X] uid:%d", uid)`) | zerolog 结构化字段 |
| 字符串化错误 (`NewErrorMsg("登录失败: " + err.Error())`) | sentinel + `AppError` |
| 接口 `I` 前缀 (`IAccountRepository`) | 去前缀，Go 风格 |
| 中英文注释混用 | 注释统一英文 |
| 拼写错误遗产 (`GetColomns`) | 发现即改 |
| 无 `context.Context` 传递 | 所有 I/O 必传 ctx |
| 无显式事务支持 | service 层 `mongox.WithTx` |
| Redis key 字面量散落 | 集中在 `redis_keys.go` 函数 |
| 仓库返回接口 | 返回 `*Struct`，接口在消费方 |
| 全局 log 单例 + 手动 prefix | zerolog ctx 继承字段 |
| 无请求超时 | 所有外部调用必有 ctx timeout |
| 自定义 protobuf RPC（服务间） | 单体，无需 |
| route_server / registry 服务发现 | 单体，无需 |
| glog 自研封装 | zerolog |

---

## 19. 新代码检查清单（PR 必过）

- [ ] 无 `fmt.Printf` / `log.Printf`（除 main 启动提示一行）。
- [ ] 所有 I/O 函数首参是 `ctx context.Context`。
- [ ] Handler 不直接引用 `*mongo.Client` / `*redis.Client`。
- [ ] Service 不直接引用 mongo/redis 原生 client（除事务场景用 `mongox.WithTx`）。
- [ ] 新增接口定义在消费方包；构造函数返回 `*Struct`。
- [ ] 所有 ID 是 typed ID（`UserID` 而非 `string`）。
- [ ] 每个 Redis `Set` 显式带 TTL（或注释原因）。
- [ ] 错误用 sentinel + `fmt.Errorf("%w")` 包装，不拼字符串。
- [ ] 新增 collection 在 repo 里有 `EnsureIndexes` 方法并被 `initialize` 调用。
- [ ] 公开成员有英文 godoc 注释。
- [ ] 有对应 `*_test.go`；service 层覆盖主要分支。
- [ ] `bash scripts/build.sh --test` 本地过。
- [ ] 无 `context.TODO()`，业务代码无 `context.Background()`。
- [ ] 无 `// TODO` 不带 issue 号。

---

## 20. 项目约束提醒

- 裤衩猫**只做 Apple Watch**，不做 Wear OS。
- 后端**自建 Go 服务**，不用 Firebase。
- 客户端：iOS（Swift）+ watchOS（SwiftUI / SpriteKit）。
- Claude 完成 99.99% 编码；瓶颈在美术与真机调试。

---

**本文档是活文档**：每次重大架构决策后补充；过时规则标注并移除（历史由 git log 承担，不在文档里留"已废弃"段落）。
