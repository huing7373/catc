# Story 1.2: cmd/server 入口 + 配置加载 + Gin + ping

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want 一个能在本地 `bash scripts/build.sh && ./build/catserver` 启动并监听 8080 的 Gin 应用，响应 `GET /ping`,
so that 后续所有 server 工作都基于同一个入口推进，端到端 ping 联调（Epic 3）有真实目标可打.

## 故事定位（节点 1 第一条实装 story）

这是 Epic 1 从决策态转入实装态的**第一条代码 story**：
- Story 1.1（Spike）已锁定工具栈（slog / prometheus client_golang / sqlmock+miniredis / testify / Gin v1.12.0）。
- 本 story 建立 `server/go.mod`，落实**最小可运行的 Gin 应用**：入口 + 配置加载 + 一个路由。
- **不涉及**中间件（→ 1.3）、`/version`（→ 1.4）、测试基础设施依赖完整安装（→ 1.5）、build.sh 重做（→ 1.7）、AppError（→ 1.8）、ctx 约定（→ 1.9）。
- 测试本 story 自身单测时，允许直接 `cd server && go test ./...`，无需依赖 build.sh 新形态（build.sh 重做在 Story 1.7）。

**范围红线**：仅建 `cmd/server` + `configs/local.yaml` + `internal/infra/config/` + `internal/app/bootstrap/` + `internal/app/http/handler/ping_handler.go` + `internal/pkg/response/`。禁止提前建 `domain/*` / `repo/*` / `service/*` 子目录骨架（会触发空目录 import 告警；等对应 Epic 用到时再建）。

## Acceptance Criteria

**AC1 — `go.mod` 建立并锁死依赖版本（对应决策 ADR-0001 §6）**

- `server/go.mod` 顶部声明 `go 1.22`（或更高；slog 需要 Go 1.21+，决策 §6 要求 `go 1.22`）。
- module 路径：`github.com/huing/cat/server`（与 git user `huing` + 仓库名对齐；若团队约定不同，由 dev 决定但需在 Completion Notes 记录并在 README 提及）。
- 初始 `require` 段包含且版本固定：
  - `github.com/gin-gonic/gin v1.12.0`（HTTP 框架，CLAUDE.md + ADR-0001 §6）
  - `gopkg.in/yaml.v3 v3.0.1` 或等价 YAML 解析库（下方 Dev Notes §配置库选择 详述）
- `go mod tidy` 后提交 `go.sum`。
- **不安装**测试类依赖（testify / sqlmock / miniredis / dockertest / prometheus）—— 那些归 Story 1.5。
- **不安装** gorm / go-redis —— 那些分别归 Epic 4 / Epic 10。

**AC2 — 目录结构与文件就位（对应 `docs/宠物互动App_Go项目结构与模块职责设计.md` §4）**

以下目录与文件**本 story 必须建立**：

```
server/
├─ go.mod
├─ go.sum
├─ cmd/
│  └─ server/
│     └─ main.go
├─ configs/
│  └─ local.yaml
└─ internal/
   ├─ app/
   │  ├─ bootstrap/
   │  │  ├─ server.go       # 组装：load config → build gin engine → http.Server
   │  │  └─ router.go       # 注册 /ping（未来 1.3/1.4 在此扩展）
   │  └─ http/
   │     └─ handler/
   │        └─ ping_handler.go
   ├─ infra/
   │  └─ config/
   │     ├─ config.go        # 结构体 Config / ServerConfig / LogConfig
   │     └─ loader.go        # Load(path string) (*Config, error) + env 覆盖
   └─ pkg/
      └─ response/
         └─ response.go      # 统一 {code, message, data, requestId} 封装
```

**禁止**在本 story 建立：`configs/{dev,staging,prod}.yaml`（未到需要时；Story 1.10 README 或未来 ops story 补齐）、`migrations/`（Epic 4）、`internal/domain/` 下任何子目录（对应 Epic 启动时再建）、`internal/repo/` / `internal/service/` 占位（避免空目录）。

**AC3 — `configs/local.yaml` 初始内容最小集**

```yaml
server:
  http_port: 8080
  read_timeout_sec: 5
  write_timeout_sec: 10

log:
  level: info       # debug / info / warn / error
```

字段严格对齐 `Go项目结构与模块职责设计.md` §12.2 建议的 `server` 与 `log` 两类（节点 1 不需要 `mysql` / `redis` / `auth` / `ws` 段，等相应 Epic 再补）。

**AC4 — 配置加载契约**

- `infra/config.Load(path string) (*Config, error)`：
  - 读取指定 YAML 文件，用 `yaml.v3` 反序列化到 `Config` struct。
  - 缺省值：如果 `server.http_port` 为 0，默认 8080；如果 `log.level` 为空串，默认 `info`。
  - **环境变量覆盖规则**（逐项 explicit 覆盖，**不**引入 viper 全自动映射）：
    - `CAT_HTTP_PORT` → `Config.Server.HTTPPort`（整数，解析失败则 return error）
    - `CAT_LOG_LEVEL` → `Config.Log.Level`（字符串，任意非空值覆盖）
  - 文件不存在 → return `fmt.Errorf("config file not found: %s", path)`，调用方决定是否致命。
- `main.go` 的读取顺序：
  1. 读取命令行 flag `-config`（默认 `configs/local.yaml`，相对于当前工作目录）
  2. 调 `config.Load(path)`
  3. 失败时用 stdlib `log.Fatalf("config load failed: %v", err)` 退出，exit code 非 0
  4. 成功时把加载结果打印一行摘要（不打印敏感字段，本 story 没有；打印 `http_port` + `log.level`）

**AC5 — Gin 路由与 `/ping` response 契约（对应 `V1接口设计.md` §2.4 通用响应结构）**

- 使用 `gin.New()`（不是 `gin.Default()`，因为默认中间件与 Story 1.3 会冲突；本 story 刻意不挂任何中间件）。
- 注册路由：`GET /ping` → `PingHandler`。
- **`/ping` 不加 `/api/v1` 前缀**（与业务接口前缀不同；`/ping` 视为运维探活端点，同 `/version`、`/metrics` 同一类），依据：`V1接口设计.md` §2.2 只规定业务接口前缀 `/api/v1`，未提 `/ping`。
- 响应体严格符合 `V1接口设计.md` §2.4：
  ```json
  {
    "code": 0,
    "message": "pong",
    "data": {},
    "requestId": "req_xxx"
  }
  ```
  - `code` = 0（int）
  - `message` = `"pong"`（固定字符串，和 V1接口设计 §2.4 示例 `"ok"` 有意不同，`/ping` 的 message 用 `"pong"` 语义更贴合）
  - `data` = `map[string]any{}`（空对象而非 `null`，JSON 输出为 `{}`）
  - `requestId` = `"req_xxx"`（**本 story 暂用占位值**，正式 UUID 生成在 Story 1.3 request_id 中间件）
- `pkg/response.go` 暴露 `Success(c *gin.Context, data any, message string)` 与 `Error(c *gin.Context, httpStatus int, code int, message string)` 两个 helper，`PingHandler` 调 `response.Success(c, map[string]any{}, "pong")`。response helper 内暂从 `c.Request.Header.Get("X-Request-Id")` 取 requestId，取不到时 fallback `"req_xxx"`（Story 1.3 中间件落地后该 fallback 自然被替换）。

**AC6 — 启动日志**

应用启动成功时（`http.Server.ListenAndServe` 前），通过 stdlib `log` 输出一行：

```
server started on :<port>
```

例：`server started on :8080`。节点 1 阶段故意用 stdlib `log` 而非 `slog`：slog 的 JSON handler 会在 Story 1.3 logging 中间件里一并初始化；本 story 只需能看到进程启动痕迹。

**AC7 — 优雅关闭**

- 监听 `SIGINT` / `SIGTERM`（`signal.NotifyContext`）。
- 收到信号后 `http.Server.Shutdown(ctx)`，`ctx` 超时 5 秒。
- 退出前打印 `server stopped`。
- 正常退出 exit code = 0；Shutdown 超时 exit code = 1。

**AC8 — 单元测试覆盖（≥ 3 case，实际 4 case）**

> 本 story **允许直接引 `testing` stdlib**，不强制 testify（testify 依赖 Story 1.5 安装）。既然已 pin 住版本，dev 也可以顺手 `go get github.com/stretchr/testify@v1.11.1` 并加到 go.mod —— 如引入请同时更新 go.sum。**作者倾向 stdlib `testing`**（减少本 story 范围），给出两条 path 供 dev 判断。

| # | case 名 | 输入 | 期望 |
|---|---|---|---|
| 1 | happy: 配置文件存在 + 合法 | `configs/local.yaml`（fixture） | `Load` 返回无错，`Server.HTTPPort == 8080`，`Log.Level == "info"` |
| 2 | edge: 配置文件缺失 | 路径 `"testdata/nonexistent.yaml"` | `Load` 返回 error，error message 含 `"config file not found"` |
| 3 | edge: env 覆盖 | 置环境变量 `CAT_HTTP_PORT=9999` | `Load` 返回无错，`Server.HTTPPort == 9999`（**测试结束必须 `t.Setenv` 或 `os.Unsetenv` 恢复**） |
| 4 | edge: env 非法值 | `CAT_HTTP_PORT=notanumber` | `Load` 返回 error，error message 含 `CAT_HTTP_PORT` |

测试文件位置：`server/internal/infra/config/loader_test.go`。fixture YAML 放 `server/internal/infra/config/testdata/local.yaml`（与 AC3 内容一致）。

**AC9 — 集成测试覆盖（1 case，用 httptest）**

- 文件：`server/internal/app/bootstrap/router_test.go`。
- 用 `httptest.NewRecorder()` + `router.ServeHTTP(w, req)` pattern（ADR-0001 §3.2 选定方案）。
- 测试：构造 `GET /ping` → 调 `router.ServeHTTP` → 断言：
  - `w.Code == 200`
  - `w.Body` 合法 JSON，字段 `code == 0`、`message == "pong"`、`data` 为 `{}` 或空对象、存在 `requestId` 字段（值任意非空字符串）。
- 用 stdlib `encoding/json` 反序列化断言即可，不需要 testify。

**AC10 — 验收 build 可跑**

- `cd server && go vet ./...` 无 error。
- `cd server && go build -o /tmp/catserver ./cmd/server/` 产出可执行（Windows 下扩展名不必强制）。
- `cd server && go test ./...` 通过（含 AC8 + AC9 的测试）。
- **不要求**当前 `scripts/build.sh` 能跑本 story —— 旧 build.sh 仍引用 `cmd/cat` / `check_time_now.sh`，**必然失败**，这是 Story 1.7 的事。本 story 完成后若 dev 机上顺手跑 `scripts/build.sh` 失败是**预期**，Story 1.7 会修。
- 启动 `./build/catserver`（或 `go run ./cmd/server/`）后 `curl http://127.0.0.1:8080/ping` 返回 AC5 规定的 JSON。dev 需在 Completion Notes 贴一次 curl 的实际响应作为证据。

## Tasks / Subtasks

- [x] **T1**：初始化 `server/go.mod`（AC1）
  - [x] T1.1 `cd server && go mod init github.com/huing/cat/server`（module 路径如需调整，先与用户确认）
  - [x] T1.2 `go get github.com/gin-gonic/gin@v1.12.0`
  - [x] T1.3 `go get gopkg.in/yaml.v3@v3.0.1`
  - [x] T1.4 `go mod tidy`，检查 go.sum 已生成

- [x] **T2**：配置层（AC2 / AC3 / AC4）
  - [x] T2.1 建 `server/internal/infra/config/config.go`：
    ```go
    type Config struct {
        Server ServerConfig `yaml:"server"`
        Log    LogConfig    `yaml:"log"`
    }
    type ServerConfig struct {
        HTTPPort        int `yaml:"http_port"`
        ReadTimeoutSec  int `yaml:"read_timeout_sec"`
        WriteTimeoutSec int `yaml:"write_timeout_sec"`
    }
    type LogConfig struct {
        Level string `yaml:"level"`
    }
    ```
  - [x] T2.2 建 `server/internal/infra/config/loader.go`：实现 `Load(path string) (*Config, error)`，含 AC4 列出的缺省值 + env 覆盖
  - [x] T2.3 建 `server/configs/local.yaml`，内容同 AC3

- [x] **T3**：response 封装（AC5）
  - [x] T3.1 建 `server/internal/pkg/response/response.go`：定义 `Envelope` struct + `Success` / `Error` helper
  - [x] T3.2 `Success` 取 `c.Request.Header.Get("X-Request-Id")`，空则填 `"req_xxx"`
  - [x] T3.3 data 字段用 `omitempty`？**否** —— V1接口设计 §2.4 示例固定写 `data: {}`，强制序列化空对象（空 `map[string]any{}` 而非 `nil`，否则 JSON 会输出 `null`）

- [x] **T4**：bootstrap 与路由（AC2 / AC5）
  - [x] T4.1 建 `server/internal/app/http/handler/ping_handler.go`：`PingHandler(c *gin.Context)` 调 `response.Success(c, map[string]any{}, "pong")`
  - [x] T4.2 建 `server/internal/app/bootstrap/router.go`：`NewRouter() *gin.Engine`，内部 `gin.New()` + 注册 `GET /ping`
  - [x] T4.3 建 `server/internal/app/bootstrap/server.go`：`Run(cfg *config.Config) error`，组装 `*http.Server` + 优雅关闭逻辑（AC6 / AC7）

- [x] **T5**：入口 main（AC4 / AC6 / AC7）
  - [x] T5.1 建 `server/cmd/server/main.go`：flag parse → `config.Load` → `bootstrap.Run`
  - [x] T5.2 加启动 banner `server started on :<port>` 与退出 banner `server stopped`

- [x] **T6**：测试（AC8 / AC9）
  - [x] T6.1 建 `server/internal/infra/config/testdata/local.yaml`（与 AC3 内容一致）
  - [x] T6.2 建 `server/internal/infra/config/loader_test.go`，写入 4 个 case（AC8 表）
  - [x] T6.3 建 `server/internal/app/bootstrap/router_test.go`，写入集成测试（AC9）

- [x] **T7**：本地验收（AC10）
  - [x] T7.1 `cd server && go vet ./...` pass
  - [x] T7.2 `cd server && go test ./...` pass（race 见 Completion Notes 的环境限制说明）
  - [x] T7.3 `cd server && go build -o ../build/catserver.exe ./cmd/server/` 产出可执行
  - [x] T7.4 启动 `./build/catserver.exe` 后 curl `/ping`，把响应体 copy 到 Completion Notes

- [x] **T8**：收尾
  - [x] T8.1 把 T7.4 的 curl 结果贴到 Dev Agent Record / Completion Notes
  - [x] T8.2 把新增/修改文件列到 File List
  - [x] T8.3 把状态流转到 review：`ready-for-dev → in-progress → review`（in-progress 在开工时改，review 在 T7 全 pass 后改）

## Dev Notes

### 项目关键约束（必读，勿绕过）

1. **Server 目录当前为空**：`server/` 下只有 `.` 和 `..`（Story 1.1 Completion Notes 已核实）。本 story **从零建**目录骨架。
2. **按需引入原则**（`MVP节点规划与里程碑.md` §2 原则 7 / §4.1）：节点 1 不接 MySQL / Redis / WS / auth 中间件。本 story **禁止**建这些目录或安装对应依赖。
3. **中间件禁忌**：Story 1.3 专职挂 `request_id` / `recover` / `logging` 三件套。本 story 用 `gin.New()`**不挂任何中间件**，以免 Story 1.3 重挂时产生冲突。`gin.Default()` 会自动挂 Gin 官方的 logger + recovery —— **不要用**。
4. **build.sh 是旧残留**：`scripts/build.sh` 当前仍引用 `cmd/cat` / `check_time_now.sh` / `docs/api/openapi.yaml`，必然跑不过新 `cmd/server`。**本 story 不修它**（Story 1.7 专职），dev 直接用 `go build ./cmd/server/` 验收。
5. **决策文档已锁**：Story 1.1 交付的 ADR-0001 (`_bmad-output/implementation-artifacts/decisions/0001-test-stack.md`) 已锁 Gin v1.12.0 + slog + prometheus client_golang + sqlmock+miniredis + testify。**本 story 仅激活 Gin**，logger / metrics / 测试依赖留给后续 story。
6. **logger 选型 vs 本 story**：ADR-0001 选 `log/slog`。但 slog 完整初始化（JSON handler + context 字段 + middleware 注入）归 Story 1.3；本 story 启动 banner 和 config 加载错误只用 stdlib `log`，避免半成品 logger 语义。
7. **Module 路径命名**：建议 `github.com/huing/cat/server`（git user 是 `huing`，仓库名 `cat`，子目录 `server/`）。若用户有公司 namespace 偏好，dev 在 T1.1 前先向用户确认；一旦定好，后续 Story 1.3+ 全部 import 路径都依赖此决定。

### 配置库选择

决策 ADR-0001 **未覆盖**配置库选型（只锁了 logger / metrics / mock）。常见选项对比：

| 候选 | 取舍 |
|---|---|
| `gopkg.in/yaml.v3`（**推荐**） | 仅做 YAML 反序列化，零魔法；env 覆盖手写（AC4 只要 2 个变量，手写 5 行代码）。零 transitive 依赖，go.mod 干净。 |
| `github.com/spf13/viper` | 功能齐但依赖树大（~15 个 transitive）、自动 env 映射反而对"明确覆盖列表"不利（容易漂移）、启动性能略差。节点 1 不需要。 |
| `github.com/knadh/koanf` | 介于两者之间，支持多 source；本项目配置来源单一（YAML + 少量 env），杀鸡用牛刀。 |
| `github.com/ilyakaznacheev/cleanenv` | env-only 优先，YAML 二等公民，与本项目"YAML 主 + env 覆盖"顺序反了。 |

**推荐**：`gopkg.in/yaml.v3 v3.0.1` + 手写 env 覆盖（≤20 行）。dev 如坚持 viper / koanf，需在 Completion Notes 写理由 + 记录 go.mod 多出的 transitive 依赖。

### Gin 使用 pin 注意事项

- Gin v1.12.0 API 与 v1.9+ 基本一致：`gin.New()` / `gin.Engine.GET` / `gin.Context.JSON` 等核心 API 稳定。
- **不**导入 `gin.ReleaseMode` / `gin.SetMode(gin.DebugMode)`：节点 1 阶段默认模式即可，Story 1.3 / 1.7 会讨论 release mode 和 ldflags 注入。
- **注意**：`gin.New()` 不挂任何中间件；如果发现启动时 Gin 打印大量 `[GIN-debug]` 日志，那是正常的 Debug 模式输出（stdout），Story 1.3 logging 落地时会切换到 Release 模式屏蔽这些。

### response helper 实现要点

```go
// internal/pkg/response/response.go
package response

import (
    "net/http"

    "github.com/gin-gonic/gin"
)

type Envelope struct {
    Code      int    `json:"code"`
    Message   string `json:"message"`
    Data      any    `json:"data"`
    RequestID string `json:"requestId"`
}

func Success(c *gin.Context, data any, message string) {
    if data == nil {
        data = map[string]any{}
    }
    c.JSON(http.StatusOK, Envelope{
        Code:      0,
        Message:   message,
        Data:      data,
        RequestID: requestIDFromCtx(c),
    })
}

func Error(c *gin.Context, httpStatus, code int, message string) {
    c.JSON(httpStatus, Envelope{
        Code:      code,
        Message:   message,
        Data:      map[string]any{},
        RequestID: requestIDFromCtx(c),
    })
}

func requestIDFromCtx(c *gin.Context) string {
    if v := c.Request.Header.Get("X-Request-Id"); v != "" {
        return v
    }
    return "req_xxx"
}
```

- `Data` 字段**不**加 `omitempty`：V1接口设计 §2.4 明确 `data: {}`，空对象需显式输出。
- `requestIDFromCtx` 从 header 读取的逻辑是**临时 bridge**：Story 1.3 挂 request_id 中间件后，`requestIDFromCtx` 会改为优先从 `c.Get("request_id")` 读（中间件存入），header fallback 降级或移除。本 story 先按 header fallback 实装，Story 1.3 会自然接管。

### 入口 `main.go` 骨架建议

```go
package main

import (
    "context"
    "flag"
    "log"
    "os/signal"
    "syscall"
    "time"

    "github.com/huing/cat/server/internal/app/bootstrap"
    "github.com/huing/cat/server/internal/infra/config"
)

func main() {
    var configPath string
    flag.StringVar(&configPath, "config", "configs/local.yaml", "path to config YAML")
    flag.Parse()

    cfg, err := config.Load(configPath)
    if err != nil {
        log.Fatalf("config load failed: %v", err)
    }

    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    if err := bootstrap.Run(ctx, cfg); err != nil {
        log.Fatalf("server run failed: %v", err)
    }
}
```

### bootstrap.Run 骨架建议

```go
// internal/app/bootstrap/server.go
func Run(ctx context.Context, cfg *config.Config) error {
    router := NewRouter()
    addr := fmt.Sprintf(":%d", cfg.Server.HTTPPort)
    srv := &http.Server{
        Addr:         addr,
        Handler:      router,
        ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSec) * time.Second,
        WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSec) * time.Second,
    }
    errCh := make(chan error, 1)
    go func() {
        log.Printf("server started on %s", addr)
        if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
            errCh <- err
        }
        close(errCh)
    }()
    select {
    case <-ctx.Done():
        shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        if err := srv.Shutdown(shutdownCtx); err != nil {
            log.Printf("server shutdown error: %v", err)
            return err
        }
        log.Printf("server stopped")
        return nil
    case err := <-errCh:
        return err
    }
}
```

### Windows / Unix 兼容

- dev 在 Windows bash 下跑（见 CLAUDE.md Shell 提示）；`syscall.SIGTERM` 在 Windows 下部分版本不支持，但 Go 运行时会把 Ctrl+C 映射到 SIGINT，够用。
- 构建输出 `build/catserver`（Linux）/ `build/catserver.exe`（Windows，由 `go build` 自动补扩展名）。本 story 不强制跨平台产物命名，AC10 用默认 `go build` 行为即可。

### 路由分组预留（不实装，仅注释）

`NewRouter` 内只注册 `GET /ping`；**不要**提前建 `/api/v1` group 或 `/dev` group（那些分别归 Story 4.x / 1.6）。一句注释提示后续扩展点即可：

```go
// Future: Story 1.3 wraps middleware; Story 1.4 adds GET /version;
//         Story 1.6 registers /dev/* group behind BUILD_DEV flag.
```

### Previous Story Intelligence（Story 1.1 交付物关键点）

- ADR-0001 `_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` 已锁：Gin v1.12.0 / slog / prometheus client_golang v1.23.2 / sqlmock v1.5.2 + miniredis v2.37.0 / testify v1.11.1 / dockertest v3.12.0（layer-2）。本 story 只激活 Gin；其他库等 Story 1.3/1.5/1.8/Epic 10+ 激活。
- 日志 6 字段（§4）：`request_id` / `user_id` / `api_path` / `latency_ms` / `business_result` / `error_code`。**本 story 不落地** —— 只写占位 requestId，真实字段下沉到 Story 1.3。
- Metrics 7 位（§5）：本 story 不注册任何 Collector；`/metrics` 端点暴露归 Story 1.3（和 logging middleware 一起上 prometheus）。
- Story 1.1 **不写任何 Go 代码**，也不建 `server/go.mod`（AC6 强制）—— 所以本 story 是**真正的第一行 Go 代码**。
- Git intelligence：Story 1.1 commit `e7f5e9c docs(decision): 0001 test stack - slog / prometheus / sqlmock+miniredis / testify` 已落；本 story 完成后 commit message 建议形如 `feat(server): Epic1/1.2 cmd/server 入口 + config 加载 + Gin + /ping`，由 user 触发（dev-story 流程不直接 commit）。

### 测试选型灵活度

- **不建议**本 story 安装 testify —— 会导致 go.mod 多 4 个 transitive（testify 依赖 stretchr/objx / davecgh/go-spew / pmezard/go-difflib / yaml.v3），而 AC8 4 个 case 用 stdlib testing 写出来 <80 行，没必要。Story 1.5 会统一安装所有测试依赖。
- **如果 dev 选安装 testify**：那就连带把 ADR-0001 §6 全部版本 pin 都加进 go.mod，并在 Completion Notes 记录"本 story 提前激活了 testify，Story 1.5 只需安装 sqlmock/miniredis/dockertest/client_golang"。保持团队内信息对齐即可，不是错误选择，只是不同的 trade-off。

### 常见陷阱

1. **`gin.Default()` 陷阱**：会自动挂 Logger + Recovery middleware，与 Story 1.3 挂自定义三件套起冲突。**必须 `gin.New()`**。
2. **YAML 字段 tag 拼写**：`yaml:"http_port"` 下划线命名，不是 camelCase；与 `local.yaml` 写法对齐。
3. **空 data 序列化**：`map[string]any{}` → `{}`；`nil` 或未赋值 `any` → `null`。AC5 要求 `{}`，不要偷懒。
4. **env 覆盖时 `int` 解析**：`strconv.Atoi(os.Getenv("CAT_HTTP_PORT"))` 的 err 必须传回给调用方（AC4 / AC8 case 4），不要静默 fallback 到 0 —— 否则 happy path 的 case 1 与 env 非法 case 4 断言会漂移。
5. **测试 env 变量泄漏**：用 `t.Setenv(...)` 而非 `os.Setenv(...)`；`t.Setenv` 在测试结束自动恢复，避免跨 case 污染。Go 1.17+ 支持。
6. **module 路径与 import 路径耦合**：一旦 `go mod init github.com/huing/cat/server`，所有内部 import 必须写 `github.com/huing/cat/server/internal/...`。如果中途改 module 路径，每个 .go 文件的 import 全要改。T1.1 前和用户确认。

### Project Structure Notes

- 目录与 `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 严格对齐：`cmd/server/main.go`、`configs/local.yaml`、`internal/app/{bootstrap,http/handler}/`、`internal/infra/config/`、`internal/pkg/response/`。
- **尚未建立的目录**（不在本 story 范围）：`migrations/`（Epic 4 建）、`internal/app/http/middleware/`（Story 1.3）、`internal/app/http/request,response/`（Epic 2 业务接口开始时）、`internal/app/ws/`（Epic 10）、`internal/domain/*`、`internal/service/`、`internal/repo/*`（每个 Epic 建自己的子目录）、`internal/infra/{db,redis,logger,clock,idgen}/`（对应依赖引入时建）、`internal/pkg/{errors,auth,utils}/`（Story 1.8 / Epic 4 / 各 Epic）。
- **冲突点**：旧 `scripts/build.sh` 指向 `cmd/cat` —— 本 story 不修，Story 1.7 重做。AC10 明确说明本 story 用 `go build ./cmd/server/` 验收，不依赖 build.sh。
- **与统一项目结构一致**：目录 / 命名 / 分层与总体架构设计 §6.3 + Go项目结构设计 §4 完全一致；无 variance。

### References

- [Source: docs/宠物互动App_总体架构设计.md#6.1-技术选型] — Go + 模块化单体 + Gin
- [Source: docs/宠物互动App_总体架构设计.md#6.3-推荐目录] — `cmd/server/main.go` + `internal/app/{http,ws,bootstrap}` + `internal/infra` + `internal/pkg` 结构
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#4-项目目录建议] — 完整目录树（本 story 建其中子集）
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#12-配置与环境管理建议] — YAML + 环境变量覆盖；server / log 两类配置段
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#8.1-路由分组] — 节点 1 只注册 `/ping`，`/api/v1` group 留给后续 Epic
- [Source: docs/宠物互动App_V1接口设计.md#2.2-接口前缀] — 业务接口前缀 `/api/v1`；`/ping` 不属业务接口，不加前缀
- [Source: docs/宠物互动App_V1接口设计.md#2.4-通用响应结构] — `{code, message, data, requestId}` 四字段固定
- [Source: docs/宠物互动App_MVP节点规划与里程碑.md#4.1-节点1] — 节点 1 完整范围 + 不做列表 + 验收标准
- [Source: docs/宠物互动App_MVP节点规划与里程碑.md#2-当前MVP节点规划原则] — 原则 7（按需引入）
- [Source: CLAUDE.md#Tech-Stack新方向] — 技术栈锁定（Gin / MySQL / Redis / YAML）
- [Source: CLAUDE.md#Build-Test] — 当前 build.sh 状态与 TODO（Story 1.7 专职修）
- [Source: CLAUDE.md#节点-1-之后的目录形态] — 目标目录形态
- [Source: _bmad-output/planning-artifacts/epics.md#Epic-1] — Epic 1 完整范围 + 不做列表
- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.2] — 本 story 原始 AC（本文档对齐并细化）
- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.3] — 下游：中间件三件套挂载方式（影响本 story 为何 `gin.New()` 不挂中间件）
- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.4] — 下游：`/version` 落地点（本 story 留注释占位）
- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.6] — 下游：`/dev/*` 路由组（本 story 留注释占位）
- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.7] — 下游：build.sh 重做（本 story 绕过旧 build.sh 验收）
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#6-Version-Lock] — Gin v1.12.0 + 其他库版本 pin
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#7-Follow-ups] — Story 1.2 分工（建 `server/go.mod`，Gin 必装）
- [Source: _bmad-output/implementation-artifacts/1-1-mock-库选型-spike-logger-metrics-框架选型.md#Completion-Notes] — Story 1.1 已交付确认，server/ 目录确认为空

## Dev Agent Record

### Agent Model Used

claude-opus-4-7 (1M context) — BMM dev-story workflow v6.2

### Debug Log References

- `go test ./...` — 4 unit cases (config loader) + 1 integration case (router) 全通过
- `go vet ./...` — 无告警
- `go build -o ../build/catserver.exe ./cmd/server/` — 产出 21MB Windows binary
- 启动验证：`CAT_HTTP_PORT=18080 ./build/catserver.exe` + `curl /ping` — 两次：无 header / 带 `X-Request-Id`

### Completion Notes List

- **Module 路径**：按 Dev Notes 推荐使用 `github.com/huing/cat/server`，无额外协商需求（git user = `huing`，仓库 = `cat`，子目录 = `server/`）。
- **go directive = 1.25.0（非 1.22）**：`go mod init` 默认写入当前 toolchain 版本；`go mod tidy` 后 Gin v1.12.0 的 indirect 依赖（bytedance/sonic 等）又把 directive 向上锁到 1.25.0。AC1 原文允许"1.22 或更高"，因此保留 1.25.0 满足约束，同时避免每次 tidy 反复覆写。
- **依赖广度提示**：Gin v1.12.0 意外带入较大 transitive 树（含 `go.mongodb.org/mongo-driver/v2`、`github.com/quic-go/quic-go`、`github.com/bytedance/sonic` 等）。这是 ADR-0001 §6 锁死版本的既有代价，不在本 story 的版本选型范围内；若 Story 1.5/1.7 需要收敛，建议单独开 tech-debt 项复核。
- **配置库**：按 Dev Notes 推荐使用 `gopkg.in/yaml.v3 v3.0.1` + 手写环境变量覆盖，未引入 viper / koanf。
- **测试框架**：使用 stdlib `testing`，未引入 testify —— 与 Dev Notes 作者倾向一致，testify 留给 Story 1.5。
- **环境变量覆盖**：`CAT_HTTP_PORT`（int，解析失败 return error）+ `CAT_LOG_LEVEL`（string，任意非空覆盖）。解析顺序为"YAML → env 覆盖 → 缺省值填充"，保证非法 env 能被 case 4 捕获。
- **`data: {}` 序列化**：`map[string]any{}` 非 `nil`，JSON 编码后严格为 `{}`（`response.Success` 内置 nil→empty-map fallback）。
- **response helper `requestIDFromCtx`**：临时读取 `X-Request-Id` header，header 缺失时回落 `"req_xxx"`。Story 1.3 挂 request_id 中间件后会改为优先从 `c.Get("request_id")` 读取，header fallback 自然降级或移除。
- **启动/退出 banner**：`log.Printf("server started on %s", addr)` + `log.Printf("server stopped")`，均用 stdlib `log`。slog 初始化留给 Story 1.3。
- **优雅关闭**：`signal.NotifyContext(ctx, SIGINT, SIGTERM)` + `srv.Shutdown(ctx with 5s timeout)`。Go runtime 在 Windows 下把 Ctrl+C 映射到 SIGINT，msys2 `kill -INT` 到 Windows console process 的信号语义略有偏差（本次 curl 验证后用 SIGINT 退出时 "server stopped" 日志未完全 flush 到后台日志，但进程正常退出），代码路径按 story 规约实装。
- **`-race` 限制**：本机 Go 1.25.0 toolchain 的 `cgo.exe` 在 Windows + msys2 gcc (ucrt64) 组合下以 exit status 2 失败（`runtime/cgo: ... cgo.exe: exit status 2`，无更多错误输出）。这是 Go 工具链 × MinGW 的环境层问题，与本 story 代码无关。常规 `go test ./...` 通过。Story 1.5 搭测试基础设施时建议一并 revisit race 检测环境。
- **build.sh 未修**：按 Dev Notes §4 / AC10 说明，旧 `scripts/build.sh` 仍引用 `cmd/cat`，本 story 不触碰，用 `go build ./cmd/server/` 直接验收；Story 1.7 专职重做。
- **curl 实测响应**（端口 18080，因本机 8080 被其它进程占用）：
  ```
  $ curl -sS -i http://127.0.0.1:18080/ping
  HTTP/1.1 200 OK
  Content-Type: application/json; charset=utf-8
  Content-Length: 59

  {"code":0,"message":"pong","data":{},"requestId":"req_xxx"}

  $ curl -sS -i -H "X-Request-Id: test-req-123" http://127.0.0.1:18080/ping
  HTTP/1.1 200 OK
  Content-Type: application/json; charset=utf-8
  Content-Length: 64

  {"code":0,"message":"pong","data":{},"requestId":"test-req-123"}
  ```
  严格符合 AC5：`code=0` / `message="pong"` / `data={}` / `requestId` 存在且 header 能透传。

### File List

新增：
- `server/go.mod`
- `server/go.sum`
- `server/cmd/server/main.go`
- `server/configs/local.yaml`
- `server/internal/app/bootstrap/router.go`
- `server/internal/app/bootstrap/server.go`
- `server/internal/app/bootstrap/router_test.go`
- `server/internal/app/http/handler/ping_handler.go`
- `server/internal/infra/config/config.go`
- `server/internal/infra/config/loader.go`
- `server/internal/infra/config/loader_test.go`
- `server/internal/infra/config/testdata/local.yaml`
- `server/internal/pkg/response/response.go`

修改：
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（`1-2-...` 从 `ready-for-dev` → `in-progress` → `review`；`last_updated` 更新）
- `_bmad-output/implementation-artifacts/1-2-cmd-server-入口-配置加载-gin-ping.md`（Tasks 全部勾选；Dev Agent Record 填充；Status → `review`）

产出（不入 git）：
- `build/catserver.exe`（本地验收产物，21MB）

### Change Log

| 日期 | 变更 | Story |
|---|---|---|
| 2026-04-24 | 初次实装：`server/` 骨架 + Gin v1.12.0 + /ping + config loader + 5 个测试 case 全绿 | 1.2 |
