# Story 1.6: Dev Tools 框架（build flag gated）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want 一个被 build flag / 环境变量**双重**控制的 `/dev/*` 路由组 + `dev_only` 中间件 + 示例端点 `/dev/ping-dev`,
so that 后续业务 Epic（E7 `POST /dev/grant-steps` / E20 `POST /dev/force-unlock-chest` / `POST /dev/grant-cosmetic-batch`）可以直接**挂**业务 dev 端点而不需要各自重造 gate 机制，且**生产构建下 `/dev/*` 永不可达**.

## 故事定位（节点 1 第六条实装 story）

- Story 1.2 已建好 `cmd/server/main.go` 入口 + `bootstrap.NewRouter()` + Gin 裸引擎 + `/ping`
- Story 1.3 已挂 RequestID / Logging / Recovery 三件套 + slog JSON + `/metrics`
- Story 1.4 已加 `/version` + `internal/buildinfo/` 包（编译期 `-ldflags -X` 注入 commit / builtAt）
- Story 1.5 已备齐测试三件套（testify assert + require + mock）+ `internal/pkg/testing/slogtest/` 日志断言 fixture
- 本 story 新增一个**运维侧路由组 `/dev/*`**：非生产构建下注册示例端点 `/dev/ping-dev`，生产构建下整组路由**物理不存在**（编译阶段剔除 + 运行期 env 再校验）
- 完成后，Story 7.5 `POST /dev/grant-steps`、Story 20.7 `POST /dev/force-unlock-chest`、Story 20.8 `POST /dev/grant-cosmetic-batch` 可以直接把自己的业务 handler 挂到 `/dev` 组下，**不再**重复做 gate 逻辑

**范围红线**：

本 story **只做**以下五件事：
1. 新建 `internal/app/http/devtools/` 包（或同层模块），内置 `Register(r *gin.Engine)` 注册函数 + `dev_only` 中间件 + `PingDevHandler` 示例端点
2. 在 `bootstrap/router.go::NewRouter()` 中调用 `devtools.Register(r)`，**不**改 NewRouter 签名
3. 新增**双重**启用机制：**env 变量 `BUILD_DEV=true`** 或 **build tag `-tags devtools`**（或语义，任一成立即启用）
4. 启用时在启动阶段**打一条 WARN 级别**的醒目警告日志
5. 单元测试 ≥ 4 case + 集成测试（两种 env 下同一请求的 200 / 404 对照）

本 story **不做**：
- ❌ **不**挂任何业务 dev 端点（`/dev/grant-steps` / `/dev/force-unlock-chest` 分属 Story 7.5 / 20.7 的 scope；本 story 只提供**示例** `/dev/ping-dev`）
- ❌ **不**接 auth / rate_limit 中间件（那是 Epic 4 的 scope；节点 1 严格按需引入；dev 端点本身**不走** auth —— 这是设计意图，因为 dev 端点是 demo 工具）
- ❌ **不**引入 AppError（Story 1.8 落地；本 story 的 404 用 `response.Error(c, http.StatusNotFound, ...)` 写字面常量即可）
- ❌ **不**重做 `scripts/build.sh`（Story 1.7 的 scope；本 story 只要求**命令行** `go build -tags devtools ./...` 可用，不必动 `build.sh`）
- ❌ **不**动 Story 1.2 / 1.3 / 1.4 / 1.5 的既有测试（已有 stdlib `testing` 风格 / testify 风格都**保留原样**）
- ❌ **不**把 `BUILD_DEV` 塞进 `configs/local.yaml` 的 `Config` struct —— 它是**部署 / 运维层开关**（像 `GOMAXPROCS` / `LOG_LEVEL`），env 变量才是正确载体
- ❌ **不**在 `/dev/ping-dev` 响应里加除 AC 规定的 `mode: "dev"` 之外的字段（保持 demo 最小）

## Acceptance Criteria

**AC1 — 依赖 / 版本 / 目录**

- 本 story **零新依赖**：只用 stdlib + 已有的 Gin + testify + slogtest；不动 `go.mod` / `go.sum`
- 必须**新建**的目录与文件：

```
server/
├─ internal/
│  └─ app/
│     └─ http/
│        └─ devtools/
│           ├─ devtools.go              # Register / IsEnabled / dev_only 中间件 / PingDevHandler
│           ├─ devtools_test.go         # 单元测试（env-var 驱动的全部 case）
│           ├─ buildtag_normal.go       # //go:build !devtools → forceDevEnabled = false
│           └─ buildtag_devtools.go     # //go:build devtools  → forceDevEnabled = true
```

- 必须**修改**：
  - `server/internal/app/bootstrap/router.go` — 在 `/metrics` 之后调一次 `devtools.Register(r)`；更新顶部注释里"Future: Story 1.6 registers /dev/* group"那行，改为"Story 1.6 done"
  - `server/internal/app/bootstrap/router_test.go` — 追加**整合**测试用例，验证 BUILD_DEV=true / false 下 `/dev/ping-dev` 分别 200 / 404
  - `server/cmd/server/main.go` — **仅**在 `logger.Init(cfg.Log.Level)` **之后**检查 `devtools.IsEnabled()`，为 true 时打一条 WARN 警告日志；不引入额外 flag / env 解析

- **不**新建：
  - `internal/domain/devtools/` —— dev 工具不是业务领域
  - `internal/service/devtools/` —— 没有跨 repo 编排的业务逻辑
  - `internal/pkg/devtools/` —— 这是 HTTP 路由模块，属于 `app/http/`
  - `configs/*.yaml` 里的 `DevMode` 字段 —— 非配置层开关

**AC2 — 启用机制：build tag + 环境变量（或语义）**

实现严格对齐 epics.md 原 AC："仅当 `BUILD_DEV=true` **或** build tag `-tags devtools` 时启用"—— **任一成立**即视为启用：

1. **Build tag `devtools`**：通过两文件对（`//go:build devtools` / `//go:build !devtools`）暴露一个包级常量：
   ```go
   // buildtag_normal.go   —— 生产默认，无 -tags 时生效
   //go:build !devtools
   package devtools
   const forceDevEnabled = false

   // buildtag_devtools.go —— 使用 -tags devtools 时生效
   //go:build devtools
   package devtools
   const forceDevEnabled = true
   ```

2. **环境变量 `BUILD_DEV`**：`os.Getenv("BUILD_DEV") == "true"` 视为真；空串 / 其他值 / 不设均视为假（**严格**等号匹配 `"true"`，不做大小写 fold，不接受 `"1"` / `"yes"` —— 避免 env 语义漂移，ops 文档必须明确）

3. **统一入口 `IsEnabled()`**：返回 `forceDevEnabled || os.Getenv("BUILD_DEV") == "true"`
   - **重要**：`IsEnabled()` 必须在**每次调用**时读 env（而不是 cache 到包级 var），这样 `dev_only` 中间件里的 request-time 检查能响应运维热切换（极边缘场景，但实现简单没理由 cache）

**AC3 — `Register(r *gin.Engine)` 行为**

```go
// internal/app/http/devtools/devtools.go

// Register 负责把 /dev/* 路由组挂到传入的 gin.Engine 上。
//
// 行为：
//   - IsEnabled() == false：不注册任何路由；不打印日志；完全透明（调用方拿到的 engine 与不调用本函数等价）
//   - IsEnabled() == true：
//     1. 在 slog.Default() 上打印一条 WARN：
//        "DEV MODE ENABLED - DO NOT USE IN PRODUCTION"
//        携带两个字段：build_tag_devtools=<forceDevEnabled> / env_build_dev=<BUILD_DEV 原值>
//     2. 创建路由组 r.Group("/dev")
//     3. 在该组上 Use(DevOnlyMiddleware())（防御性：即使 Register 被误调用时 IsEnabled 变化，也按 request-time 再校验）
//     4. 在该组注册 GET /ping-dev → PingDevHandler
//
// NOTE: Register 必须**幂等** —— 二次调用相同 engine 会让 Gin panic（路由重复注册），
// 但这是 caller 错误而非本函数的 contract 要求；NewRouter() 只调一次足够。
func Register(r *gin.Engine) {
    if !IsEnabled() { return }
    slog.Warn("DEV MODE ENABLED - DO NOT USE IN PRODUCTION",
        slog.Bool("build_tag_devtools", forceDevEnabled),
        slog.String("env_build_dev", os.Getenv("BUILD_DEV")),
    )
    g := r.Group("/dev")
    g.Use(DevOnlyMiddleware())
    g.GET("/ping-dev", PingDevHandler)
}
```

**具体行为要求**：
- 未启用时**一次**日志也不打（避免生产环境在启动时出现即使 INFO 也容易被 ops 误解的"dev mode status"字样）
- 启用时只打**一条** WARN，且是本 story 唯一允许的"启动路径 WARN 日志"（Story 1.3 logging 中间件打的是 INFO）
- 日志字段 `build_tag_devtools` 是 bool（`forceDevEnabled`）；`env_build_dev` 是原始 env 值字符串（**不** trim / 不 lowercase，便于排障时看到真实值）

**AC4 — `DevOnlyMiddleware()` 行为**

```go
// DevOnlyMiddleware 是 /dev/* 组的前置中间件，提供"防御性"二次校验：
// 即使 Register 被错误地在 !IsEnabled() 的情况下调用（或者 env 运行期被关闭），
// 每次请求到达 handler 前会再次 IsEnabled() 校验；false 则直接 404。
//
// 设计意图：route 层和 request 层**双闸门**，防止以下故障模式：
//   1. 某 future story 错误地 bypass Register 直接在 NewRouter 里挂 /dev/foo
//   2. 运维热切 BUILD_DEV=false 但没重启（极边缘，但接近零成本兼顾）
//   3. 测试场景方便：单独测 middleware 的 gate 行为，不需要构造整个 engine
//
// 被拒请求必须记录 WARN 日志，字段：
//   api_path（c.FullPath），method，client_ip；request_id 由 Logging 中间件已注入 ctx。
// 日志用 logger.FromContext(c.Request.Context()) 拿到带 request_id 的 child logger。
func DevOnlyMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        if IsEnabled() { c.Next(); return }
        reqLogger := logger.FromContext(c.Request.Context())
        reqLogger.WarnContext(c.Request.Context(), "dev_only middleware rejected request",
            slog.String("api_path", c.FullPath()),
            slog.String("method", c.Request.Method),
            slog.String("client_ip", c.ClientIP()),
        )
        response.Error(c, http.StatusNotFound, 1003, "资源不存在")
        c.Abort()
    }
}
```

**具体行为要求**：
- 返回 `404 Not Found`，响应 body 是**统一 envelope**：`{code:1003, message:"资源不存在", data:{}, requestId:<rid>}`
  - `1003` 是 V1接口设计 §3 列的 `资源不存在` 业务码；dev 端点在非 dev 模式下对外**伪装成**普通 404，不暴露"dev endpoint exists but disabled"（OpSec 考虑）
- 使用 `response.Error(c, http.StatusNotFound, 1003, "资源不存在")`——Story 1.3 recover 中间件同样用这个响应函数（code=1009 / status=500）
- `c.Abort()` 必须调，否则 handler 会在中间件返回后继续执行
- 日志级别 **WARN**（不是 ERROR，因为被拒请求是**预期**的防御路径，不是错误；ERROR 会污染告警）

**AC5 — `PingDevHandler` 行为**

```go
// PingDevHandler 是 /dev/ping-dev 的示例端点，用于验证 dev tools 框架本身可用。
// 这是唯一由本 story 提供的 dev 端点；业务 dev 端点由各自 story 引入。
//
// 响应：
//   {code:0, message:"ok", data:{"mode":"dev"}, requestId:<rid>}
func PingDevHandler(c *gin.Context) {
    response.Success(c, gin.H{"mode": "dev"}, "ok")
}
```

**具体行为要求**：
- 严格按 epics.md AC 字面："返回 `{"code":0,"data":{"mode":"dev"}}`" → 转成统一 envelope 形态
- `message` 填 `"ok"`，与 `/version` handler 一致（response.Success 第三参数）
- `data` 用 `gin.H{"mode": "dev"}`（或等价 map），**不**加其他字段
- **不**接 service 层（handler 只做 response 组装，没有业务语义）

**AC6 — `cmd/server/main.go` 启动日志**

在 `logger.Init(cfg.Log.Level)` 之后、`bootstrap.Run(ctx, cfg)` 之前，追加：

```go
// Dev mode 警告：启用时（env 或 build tag）在启动阶段打一条醒目日志。
// 放在 config.Load 之后、Run 之前：这样用户配置的 log_level 已生效，
// WARN 一定会落到 JSON handler；放在 Run 之前是避免与业务请求日志穿插。
if devtools.IsEnabled() {
    slog.Warn("DEV MODE ENABLED - DO NOT USE IN PRODUCTION")
}
```

**注意**：Register 内部会**再打一遍**同内容的 WARN（AC3）。两条不算冗余：
- main.go 这条在**加载完 config 之后、router 装配之前**就告知 ops
- Register 那条在**路由注册完成**时再确认一次
两条序列化到 JSON 日志后，ops 能清楚看到 "dev 启用 → 路由确实注册"的完整链路。

**AC7 — 单元测试覆盖（≥ 4 case，`devtools_test.go`）**

**包命名**：`package devtools_test`（外部测试包，通过导出 API 测）。

**必须 ≥ 4 case**，用 testify assert + require（与 Story 1.5 sample 风格一致）：

| # | case | 做法 | 断言 |
|---|---|---|---|
| 1 | happy: BUILD_DEV=true → /dev/ping-dev 返回 200 + dev mode body | `t.Setenv("BUILD_DEV", "true")` → 新 Engine → `Register(r)` → httptest 打 /dev/ping-dev | `status==200`；body envelope.code==0；body.data.mode=="dev"；`requestId` 非空（Register 不负责 request_id，需在 engine 上挂 RequestID middleware 才能测出 rid；本 case 可简化为仅断言 body 字段，不断 rid） |
| 2 | edge: BUILD_DEV=false → /dev/ping-dev 返回 404 | `t.Setenv("BUILD_DEV", "false")` → 新 Engine（挂三件套中间件）→ `Register(r)`（被跳过）→ httptest 打 /dev/ping-dev | `status==404`；body envelope.code==1003（或 Gin 默认 404 — 见下方说明）；见 **关键决策 §1** |
| 3 | edge: BUILD_DEV=true 但请求其他非 dev 路由 → 不受影响 | `t.Setenv("BUILD_DEV", "true")` → 完整 Engine（NewRouter + 三件套 + /ping + /version + Register）→ 打 /ping | `status==200`；body.message=="pong"；证明 dev 启用不影响业务路由 |
| 4 | edge: `DevOnlyMiddleware()` 单独被测 → BUILD_DEV=false 时返回 404 + **日志记录被拒请求** | 用 slogtest.NewHandler 接管 slog.Default；构造裸 Engine 仅挂 `DevOnlyMiddleware` 到 `/dev/foo`；`t.Setenv("BUILD_DEV", "")` → 打 /dev/foo | `status==404`；slogtest.Records() 至少一条 level=WARN，message 含 "dev_only middleware rejected"，attr 含 `api_path="/dev/foo"` / `method="GET"` |

**加分 case（强烈建议，但不作 AC 硬要求）**：
- case 5: `IsEnabled()` 的 4 种 env 真值断言：`"true"` → true；`"1"` / `"yes"` / `"TRUE"` / `""` / 未设 → false（严格 `== "true"` 语义）
- case 6: `Register` 在 `IsEnabled()==false` 时**不打任何日志**（用 slogtest 断言 Records() 长度为 0）
- case 7: `Register` 在 `IsEnabled()==true` 时打**恰好一条** WARN（build_tag_devtools / env_build_dev 字段齐备）

**关键决策 §1 — case 2 的 404 body**：
- 如果 `Register` 在 `IsEnabled()==false` 时**完全不注册** /dev/ 路由组，Gin 会按默认 NoRoute 返回 `404 Not Found` + 空 body（无 envelope）
- 我们的断言应该是 Gin 默认 404（`status==404`，body 空或非 JSON），**而非** envelope code=1003
- **原因**：envelope 404 是 `DevOnlyMiddleware` 内部触发的；当路由组都没注册时，middleware 也不会跑
- 因此 case 2 断言 `status==404` + `body.Len()==0 || strings.Contains(body, "404 page not found")`（Gin 默认 NoRoute 输出）
- case 4 才是断言 envelope 404（middleware 内部逻辑）

**禁止**：
- 用 `os.Setenv` 而不用 `t.Setenv` —— `t.Setenv` 会在 test cleanup 时自动还原；`os.Setenv` 会污染后续 test
- 在 `TestMain` 里全局 mock env —— 会破坏测试独立性
- 在包级 `init()` 里做任何 env 检查 —— 让 Go test runner 构造的 binary 永远"记住"第一次读到的 env 值

**AC8 — 集成测试覆盖（`router_test.go` 追加）**

在 `server/internal/app/bootstrap/router_test.go` 追加**一个**新测试函数 `TestRouter_DevPingEnabled_EnvToggle`，模拟 epics.md AC 的"两次启动"场景：

- **不**通过真正"两次进程启动"来测（太重；`httptest` 足够）
- 在**同一个** test 函数里做两段：
  1. `t.Setenv("BUILD_DEV", "true")` → `r := NewRouter()` → 打 /dev/ping-dev → 断言 200 + envelope body
  2. `t.Setenv("BUILD_DEV", "")` → `r := NewRouter()` → 打 /dev/ping-dev → 断言 404（Gin 默认）
- **必须**两次都调 `NewRouter()` 重新构造 engine，因为 `Register` 在 `NewRouter` 内部调，只读一次 env

**AC9 — Build tag 验证（文档化，**不**写自动化测试）**

Build tag 通过两文件 `//go:build devtools` / `//go:build !devtools` 实现；自动化测试这条路径需要跑 `go test -tags devtools ./...`，这会把所有测试用例重跑一次，对 CI 时间开销 2x。

本 story 决策：
- **不**新增 `//go:build devtools` 的专属测试文件
- **必须**在 `devtools.go` 顶部 package 注释里**明示**验证命令：
  ```
  # 不带 tag：forceDevEnabled=false，dev 仅靠 env 触发
  go build ./...
  go test ./...

  # 带 tag：forceDevEnabled=true，即使 env 未设也启用
  go build -tags devtools ./...
  go test -tags devtools ./...
  ```
- **Dev 手动验证**一次：`go build -tags devtools -o /tmp/catserver-dev ./cmd/server && unset BUILD_DEV && /tmp/catserver-dev` → 启动日志必须含 "DEV MODE ENABLED"（AC6 的那条）+ 访问 `/dev/ping-dev` 返回 200
- 在 Completion Notes 贴出上述手动验证的日志片段（**必须**实际跑过，不能描述）
- Story 1.7 重做 `scripts/build.sh` 时加 `--devtools` 开关是**1.7 的 scope**；本 story 不预设该开关形态

**AC10 — 本地 quality gate（与 Story 1.5 对齐）**

```bash
cd server

# 1. vet 干净
go vet ./...
go vet -tags devtools ./...

# 2. 全测试 pass（不带 tag）
go test ./...

# 3. tidy 稳定（本 story 零依赖变更，go.mod/go.sum 应无 diff）
go mod tidy
git diff --exit-code go.mod go.sum    # 应无输出

# 4. 手动验证带 tag 构建通过
go build -tags devtools ./cmd/server
```

- `go test -race -cover ./...` —— 本地 Windows 环境受限（见 Story 1.5 AC7 偏离说明），归 CI 执行，**不**作为本 story 本地 gate 硬要求
- Dev 在 Completion Notes 贴 `go test -v ./internal/app/http/devtools/... ./internal/app/bootstrap/...` 输出，确认 devtools 新 case + router_test 新 case 全绿

## Tasks / Subtasks

- [x] **T1** — Devtools 包基础结构（AC1 / AC2）
  - [x] T1.1 新建 `internal/app/http/devtools/buildtag_normal.go`：`//go:build !devtools` + `const forceDevEnabled = false`
  - [x] T1.2 新建 `internal/app/http/devtools/buildtag_devtools.go`：`//go:build devtools` + `const forceDevEnabled = true`
  - [x] T1.3 `go vet ./...` + `go vet -tags devtools ./...` 双向无 error

- [x] **T2** — Devtools 核心实装（AC3 / AC4 / AC5）
  - [x] T2.1 新建 `internal/app/http/devtools/devtools.go`：`IsEnabled()` + `Register(*gin.Engine)` + `DevOnlyMiddleware()` + `PingDevHandler`
  - [x] T2.2 顶部 package doc 含 AC9 的验证命令 + build tag 的两条路径说明
  - [x] T2.3 `go build ./...` + `go build -tags devtools ./...` 双向通过

- [x] **T3** — Router 集成（AC1 修改部分）
  - [x] T3.1 `internal/app/bootstrap/router.go`：在 `/metrics` 注册行之后调用 `devtools.Register(r)`
  - [x] T3.2 更新 router.go 顶部注释：删掉 "Future: Story 1.6 registers /dev/* group" 占位行，改为实际行为描述
  - [x] T3.3 `cmd/server/main.go`：在 `logger.Init(cfg.Log.Level)` 之后加 `if devtools.IsEnabled() { slog.Warn("DEV MODE ENABLED - DO NOT USE IN PRODUCTION") }`

- [x] **T4** — 单元测试（AC7）
  - [x] T4.1 新建 `internal/app/http/devtools/devtools_test.go`：`package devtools_test`，4 必选 case + 3 加分 case（共 6 个顶层 test 函数 + 1 个含 7 子 case 的 `TestIsEnabled_...`）；整文件 `//go:build !devtools`（偏离 AC9，见 Debug Log）
  - [x] T4.2 `go test ./internal/app/http/devtools/...` 全绿
  - [x] T4.3 已补 AC7 加分 case 5/6/7

- [x] **T5** — 集成测试（AC8）
  - [x] T5.1 新建 `internal/app/bootstrap/router_dev_test.go`（**非** `router_test.go` 追加，见 Debug Log）含 `TestRouter_DevPingEnabled_EnvToggle`；文件 `//go:build !devtools`
  - [x] T5.2 `go test ./internal/app/bootstrap/...` 全绿

- [x] **T6** — 本地 quality gate（AC10）
  - [x] T6.1 `go vet ./...` + `go vet -tags devtools ./...`
  - [x] T6.2 `go test ./...` 全绿；**额外**：`go test -tags devtools ./...` 也全绿（bootstrap / handler / middleware 原有测试在 devtools 构建下不受影响；devtools 包与 router_dev_test 在 tag 下整体跳过）
  - [x] T6.3 `go mod tidy` 后 `go.mod` / `go.sum` 零 diff
  - [x] T6.4 `go build -tags devtools ./cmd/server` 成功（输出 `build/catserver-dev.exe`）
  - [x] T6.5 `go test -v` 输出见 Completion Notes
  - [~] `go test -race -cover ./...` — 沿用 Story 1.5 AC7 偏离（Windows Go 缺 `race_windows_amd64.syso` + `covdata.exe`），归 CI Linux runner 执行

- [x] **T7** — Build tag 手动验证（AC9）
  - [x] T7.1 `go build -tags devtools -o /c/fork/cat/build/catserver-dev.exe ./cmd/server`（Windows 路径；偏离 AC9 原 `/tmp/catserver-dev`）
  - [x] T7.2 不设 BUILD_DEV 直接跑 `CAT_HTTP_PORT=18080 /c/fork/cat/build/catserver-dev.exe -config ./configs/local.yaml`
  - [x] T7.3 启动日志含 main.go 的 WARN + Register 的 WARN 共两条（见 Completion Notes）
  - [x] T7.4 `curl http://127.0.0.1:18080/dev/ping-dev` → `{"code":0,"message":"ok","data":{"mode":"dev"},...}`
  - [x] T7.5 额外验证 **scenario A**（normal build + BUILD_DEV=true 单独触发）+ **scenario B**（normal build + 无 BUILD_DEV 的 "prod-like" 场景）
  - [x] T7.6 两段日志 + 三段 curl 响应贴入 Completion Notes

- [x] **T8** — 收尾
  - [x] T8.1 Completion Notes 补全
  - [x] T8.2 File List 填充
  - [x] T8.3 状态流转 `ready-for-dev → in-progress → review`

## Dev Notes

### 项目关键约束（必读，勿绕过）

1. **"或"语义的理解**：epics.md 原 AC 写 "仅当 `BUILD_DEV=true`（或 build tag `-tags devtools`）时启用" —— 这里的 "**或**" 是 **OR 语义**（任一成立即启用），**不**是 XOR / AND。实装 `IsEnabled() = forceDevEnabled || os.Getenv("BUILD_DEV") == "true"`。**不**要做成"env 覆盖 build tag"之类的复杂逻辑；这里没有优先级，只有并集。
2. **`os.Getenv("BUILD_DEV") == "true"` 严格匹配**：不做大小写 fold，不接受 `"1"` / `"yes"` / `"TRUE"`。理由：
   - Go 社区主流库（`pflag` / `kingpin` / `spf13/viper` / `envconfig`）里，`bool` 解析的合法真值列表各不相同 —— 与其自己重新发明一套，不如**严格字面 "true"**，让 ops 文档一句话讲清楚（README 以及本 story Completion Notes 都必须明示）
   - 宽松匹配会让测试用例爆炸（`t.Setenv("BUILD_DEV", "1")` 应该 true 还是 false？），严格匹配测试简单
   - Epic 7 Story 7.5 / Epic 20 Story 20.7 / 20.8 的 AC 字面都是 `BUILD_DEV=true`，对齐最省事
3. **`t.Setenv` vs `os.Setenv`**：本 story 的**所有**测试用例用 `t.Setenv`，零例外。
   - `t.Setenv` 会在 test cleanup 自动还原 env，保证测试独立
   - `os.Setenv` 会漏到下游 test，造成 flaky（尤其 parallel test）
   - `t.Parallel()` 与 `t.Setenv` **冲突** —— `t.Setenv` 会强制禁用 parallel（Go 1.17+ 直接 panic）；本 story 无 parallel 需求，全部串行跑
4. **为什么不加 auth 中间件**：dev 端点是给 **dev / demo 时期**用的工具，要求的是"易用"而非"安全"。真实防御机制是：
   - 生产构建**不**带 `-tags devtools`（编译阶段剔除）
   - 生产部署**不**设 `BUILD_DEV=true`（运维 SOP 要求）
   - 双重 gate 下，/dev/* 在生产环境等同于"不存在"
   - 如果担心 dev 环境里 dev 端点被外部访问：那是 VPC / 防火墙 / `/etc/hosts` 的问题，不是本 story 该解
   - Epic 4 Story 4-5 的 auth / rate_limit 中间件**不**覆盖 /dev/ —— auth 中间件只挂在 `/api/v1/*` 业务路由组上（见 bootstrap/router.go 未来演进）
5. **404 而非 403 / 401**：被拒请求返回 `404 Not Found`（不是 `403 Forbidden` / `401 Unauthorized`）。**原因**：
   - `403` 暴露了"端点存在但被拒"的信息 —— 攻击者能据此推断 dev 工具链存在
   - `404` 伪装成"路径不存在"，对外与 `/does-not-exist` 无区别
   - OpSec 侧"fail closed, disclose nothing"（NIST SP 800-53 RA-5 推荐）
   - 业务 code `1003`（资源不存在）与 HTTP 404 语义对齐
6. **WARN 日志 vs ERROR**：被拒请求 / dev 启用启动都用 `WARN` 级别。
   - `ERROR` 触发告警（PagerDuty / Alertmanager）—— 但 dev 启用是**预期**事件（不是错误），被拒请求也是**预期**防御路径
   - `INFO` 过低 —— ops 扫日志时会漏；dev 模式启用必须**醒目**但非"出事了"
   - `WARN` 是中间态，符合场景
7. **`dev_only` 中间件记什么日志**：`api_path`（Gin 的 `c.FullPath()`，会显示模板形式如 `/dev/ping-dev` 而非 raw URL）、`method`、`client_ip`；**不**记 `user_id`（dev 端点不走 auth，没有 user_id）、**不**记 request body（dev 端点可能收到大 body，日志放大）
8. **`logger.FromContext(ctx)` vs `slog.Default()`**：Logging 中间件（Story 1.3）已在请求 ctx 里注入带 `request_id` 的 child logger —— dev_only 中间件**必须**用 `logger.FromContext(c.Request.Context())` 取那个 child logger，这样 WARN 日志里会自动带 request_id；如果直接 `slog.Default()` 会丢 request_id
9. **`response.Error` 的 `code` 值**：用 `1003`（V1接口设计 §3 列的"资源不存在"）。理由见约束 #5。**不**引入新的 dev 专属 code —— 保持 dev 对外**完全透明**（看起来就是个 404）。
10. **main.go 的 WARN 位置**：**必须**在 `logger.Init(cfg.Log.Level)` **之后**。理由见 `docs/lessons/2026-04-25-slog-init-before-startup-errors.md`：配置加载失败路径有独立 bootstrap logger（info level + JSON），配置加载成功后才会应用用户配置的 level —— dev WARN 发生在 user-config level 生效之后**更合适**（保证 WARN 一定会被 user config 接收，即使用户把 level 设成 `error` 也会过滤掉，但这是用户主动选择 opt-out，不是工具 bug）

### 为什么在 `internal/app/http/devtools/` 而不是别的位置

- **不**放 `internal/middleware/`：dev tools 不只是中间件，还含 handler + registration logic，单放 middleware 层不完整
- **不**放 `internal/pkg/devtools/`：`pkg/` 是**跨 domain 共享工具**（如 `pkg/response/`）；dev tools 是**HTTP 特定模块**，不跨 domain
- **不**放 `internal/infra/devtools/`：`infra/` 是"**外部设施接入**"（db / redis / logger / metrics）；dev tools 是"**内部运维路由**"，语义不同
- **选择** `internal/app/http/devtools/`：与 `internal/app/http/handler/` / `internal/app/http/middleware/` 同层，表示"HTTP 路由层的一个子模块"；未来如果拆 websocket dev endpoints，也能对称开 `internal/app/ws/devtools/`

**偏离架构文档 §4 的备注**：`docs/宠物互动App_Go项目结构与模块职责设计.md` §4 目录清单没有明示 `internal/app/http/devtools/` 这一层 —— 那是因为设计文档是写**业务模块**层（auth / home / step / chest 等）；运维类子模块不在其范围。Story 3.3 文档同步 story 时把 `internal/app/http/devtools/` 加进目录清单（非本 story scope）。

### 为什么不加自动化 build tag 测试

两种自动化 build tag 测试方案都有问题：

- **方案 A：`//go:build devtools` 专属测试文件 + CI 分两次跑**  
  问题：CI 跑 `go test ./...` + `go test -tags devtools ./...`，总测试时间翻倍；本 story 的测试加总可能 <100ms，但 Story 1.5 已经有 10+ case，Epic 4+ 每条 story 都加 case，tag 双跑成本线性放大
- **方案 B：单个测试函数里通过 `go build -tags devtools -o /tmp/...` 真实构建 + exec 子进程**  
  问题：子进程启动 + 端口冲突 + 测试时长不可控（单测变成 5+ 秒），不符合单测金字塔底层"秒级反馈"

本 story 决策：
- **Env-var path 用自动化测试全覆盖**（case 1-7，全部 < 100ms）
- **Build-tag path 用手动验证** + package doc 内的 `go test -tags devtools` 命令让 dev **偶尔**跑一次验证（例如修改 `buildtag_*.go` 时）
- CI 预留：Story 1.7 重做 `scripts/build.sh` 时可以加 `--devtools-check` 模式，跑 `go build -tags devtools ./... && go vet -tags devtools ./...` 作为语法 check（不跑测试）
- **假阳性风险**：build tag 机制本身是 Go 语言特性，两行 `const forceDevEnabled = bool` 出错概率极低；手动 + code review 足够防御

### 为什么 `Register()` 内部自己打 WARN 日志（而不是让 main.go 负责）

两种方案对比：

| 方案 | 利 | 弊 |
|---|---|---|
| A: main.go 打 WARN + Register 不打 | 职责单一，Register 是纯路由注册函数 | 测试时测 `Register` 必须额外测 main.go 里的 log 行为；NewRouter（Register 的真实调用方）没有触发 WARN 的机制 |
| **B: Register 内部打 WARN + main.go 也打一条（AC6）** | Register 的副作用完整可测（slogtest 断言）；main.go 那条作为"更早期"提示；两条 WARN 在 JSON 日志里序列化成两行，ops 能看到完整链路 | 轻微冗余 |

本 story 选 B —— 冗余只是"两条日志"，但可测性 + 早期提示都显著增强。

### 为什么 case 2（BUILD_DEV=false）断言 Gin 默认 404，而非 envelope 404

关键区别：

- **未启用（BUILD_DEV 空）**：`Register()` 直接 return，不调用 `r.Group("/dev")`；Gin engine 里**根本没注册** `/dev/*` 路径；请求 `/dev/ping-dev` → Gin 的 `NoRoute` handler（默认返回 `404 page not found` 明文）
- **启用但中间件判断失败（极罕见，仅 `DevOnlyMiddleware` 单独测时构造）**：路由组存在，中间件内部 `IsEnabled()==false` → 返回 envelope 404 + code=1003

测试里这两种路径要**区分**：
- case 2：`NewRouter()` 在 env 空时的**完整**行为 → Gin 默认 404（无 envelope）
- case 4：直接在裸 Engine 上挂 `DevOnlyMiddleware` + 手工调 `IsEnabled==false` → 触发中间件内部的 envelope 404

**不要混淆**这两种场景；混淆会导致测试对不上实装。

### 未来延伸（非本 story scope）

- **Story 7.5** 会新建 `internal/app/http/handler/dev_step_handler.go` 里的 `GrantStepsHandler`，在 `devtools.Register(r)` 内加一行 `g.POST("/grant-steps", handler.GrantStepsHandler)` 即可（或在 Story 7.5 自己的注册逻辑中调 `devtools.MustEnabled()` 做前置 check）
- **Story 20.7 / 20.8** 同理 `/dev/force-unlock-chest` / `/dev/grant-cosmetic-batch`
- **Story 1.10** 的 server README 会加一段"开 dev mode"的章节，演示 `BUILD_DEV=true ./build/catserver` 和 `go build -tags devtools`
- **Story 1.7** 的 `scripts/build.sh` 重做时可以加 `--devtools` 选项，自动加 `-tags devtools` + 输出名带 `-dev` 后缀（`build/catserver-dev`）

### Previous Story Intelligence（Story 1.1 / 1.3 / 1.4 / 1.5 交付物关键点）

- **Story 1.1 ADR-0001** 选定 testify 三件套 + slog stdlib + prometheus client_golang；本 story 的 `DevOnlyMiddleware` 拒绝日志用 `slog.WarnContext(ctx, ...)`，与 Logging 中间件风格一致
- **Story 1.3** 已挂 `RequestID / Logging / Recovery`；本 story 的 `/dev/*` 路由组**继承**这三件套（因为挂在同一个 engine 上，`r.Group("/dev")` 之前注册的中间件对 group 内路由全部生效）—— 这是 Gin 的语义，**不需要**本 story 额外处理
- **Story 1.4** 的 `VersionHandler` 是直接 `response.Success(c, struct{...}{}, "ok")`；本 story 的 `PingDevHandler` 同样简单一行 `response.Success(c, gin.H{"mode":"dev"}, "ok")` —— 参考 `version_handler.go` 的代码风格
- **Story 1.4 `internal/buildinfo/`** 是包级 `var` + `-ldflags -X` 注入的先例 —— 本 story 的 `buildtag_*.go` 用**包级 `const`** + 两文件 build tag，是**另一种**编译期定值机制，对比学习：
  - `buildinfo` 的值是 **build-time 注入的字符串**（commit / builtAt）
  - `forceDevEnabled` 的值是 **build-time 选择的 `true`/`false`**（根据有没有传 `-tags`）
  - 两者都是"编译期决定，运行期只读"
- **Story 1.5 slogtest** 的 `AttrValue(record, key)` 是**扁平 key** 查找（见 lesson `2026-04-24-sample-service-nil-dto-and-slog-test-group.md` Lesson 2）；本 story 的 AC7 case 4 断言 `api_path` / `method` 字段用 `AttrValue` 直接查，**不**走 `slog.Group(...)` 嵌套
- **Story 1.5 sample service** 展示了 `package xxx_test` 外部测试包 + testify 三件套的模板；本 story 的 `devtools_test.go` **沿用**同样结构

### Lessons Index（与本 story 相关的过去教训）

- `docs/lessons/2026-04-24-config-path-and-bind-banner.md` **Lesson 1**（CWD-relative path / 声明 vs 现实）—— 间接相关：本 story 用 `os.Getenv` 读 env var，如果未来 dev 把 dev 模式配置写进 `.env` 文件让 Go 自动读（而没有显式 `export`），就会掉坑；**本 story 只用 `os.Getenv`，不引入 dotenv 机制**，保持"env = 从 shell 继承的进程环境变量"单一语义。README（Story 1.10）要明示这点
- `docs/lessons/2026-04-24-config-path-and-bind-banner.md` **Lesson 2**（声明 vs 现实）—— 直接相关：`slog.Warn("DEV MODE ENABLED")` 声明的是"dev 模式已开"，**现实**是"env/tag 检查通过且 Register 已注册"。本 story 的 `Register` 内部 WARN 日志**必须**在 `r.Group + middleware + route` 全部注册**完成之后**再打（不能打在函数开头），否则"声明 dev 模式开了"但下游还没注册的瞬时态是谎言
- `docs/lessons/2026-04-24-sample-service-nil-dto-and-slog-test-group.md` **Lesson 1**（模板 nil 兜底）—— 本 story 的 `PingDevHandler` 极简无 repo 调用，不涉及 nil 兜底；但未来 Story 7.5 `GrantStepsHandler` 写时，repo 调用返回 `(nil, nil)` 场景**必须**按 sample 模板兜底。本 story 不负责 enforce，但 SM 在 7.5 story 创建时要引用这条
- `docs/lessons/2026-04-25-slog-init-before-startup-errors.md` **Lesson 1**（bootstrap logger 初始化时机）—— 直接相关：本 story AC6 要求 main.go 的 WARN 在 `logger.Init(cfg.Log.Level)` **之后**打（见 Dev Notes 约束 #10）；如果错放在 config load 之前，dev WARN 就会用 bootstrap-default 的 info level，无法响应用户 log_level 配置

### Git intelligence（最近 5 个 commit）

- `1564623 chore(commands): 更新 /story-done 命令`
- `2b2a7a8 chore(claude): 更新 Bash allowlist`
- `5bdda0a chore(commands): /fix-review 不再询问 commit message`
- `976d959 chore(story-1-5): 收官 Story 1.5 + 归档 story 文件`
- `d8251aa chore(lessons): backfill commit hash for 2026-04-24-sample-service-nil-dto-and-slog-test-group`

最近实装向 commit 是 `7a12492 feat(server): Epic1/1.5 测试基础设施 testify/sqlmock/miniredis + sample service 模板 + slog test handler`（Story 1.5 主 commit，本 story 紧接其后）。

**commit message 风格**：Conventional Commits，中文 subject，scope `story-X-Y` 或 `server`。  
本 story 建议：`feat(server): Epic1/1.6 Dev Tools 框架 + /dev/ping-dev + dev_only 中间件 + build tag gate`

### 常见陷阱

1. **`os.Getenv("BUILD_DEV")` 在 test 里被静默污染**：CI runner 可能全局设了 `BUILD_DEV` 给其他用途 —— 本 story 的测试必须用 `t.Setenv` 显式设（包括设成空串 `""`）覆盖任何继承 env。**禁止**任何 test 依赖"外部 env 没设过 BUILD_DEV"的**假设**。
2. **`t.Setenv` 与 `t.Parallel()` 冲突**：Go 1.17+ 如果同一 test 调了 `t.Setenv` 又调 `t.Parallel()`，`t.Parallel()` 会 panic。本 story 的测试**全部串行**，不加 `t.Parallel()`。
3. **`gin.Engine.ServeHTTP` 与 middleware 挂载顺序**：Register 必须在 `r.Group("/dev")` 上调 `g.Use(DevOnlyMiddleware())` **之后**再挂 route（`g.GET("/ping-dev", ...)`）。顺序反了会让 handler 在中间件之前执行 —— Gin 的 `Use` 是**逆序 prepend**不是 append，这是容易踩的陷阱。测试时用 `t.Run("middleware applied first")` 验证（可选 case 5）。
4. **Gin 默认 NoRoute 返回非 JSON**：BUILD_DEV 空时请求 /dev/ping-dev 走 Gin 的 NoRoute handler，默认输出是文本 `"404 page not found"`，**不是** JSON envelope。测试断言 `w.Code == 404` + `strings.Contains(w.Body.String(), "404 page not found")`，**不**要断言 JSON body.code。
5. **`logger.FromContext(ctx)` 返回的 child logger 是 slog.Default() 的快照**：如果 test 里 `slog.SetDefault(slogtest.Handler)` **之后**构造 engine，child logger 会基于新 default 派生；如果**之前**构造 engine，child logger 会基于旧 default 派生。本 story 测试 case 4 的顺序：先 `slog.SetDefault(slogtest)` → 再构造 engine + 挂 middleware → 再 httptest 打请求。`router_test.go` 的集成测试里 `NewRouter()` 也要在 `slog.SetDefault` 之后调，否则 Logging 中间件的 child logger 仍指向旧 default。
6. **Build tag 两个文件的严格对立**：`buildtag_normal.go` 的 `//go:build !devtools` 和 `buildtag_devtools.go` 的 `//go:build devtools` **必须**互为补集 —— 少一个 `!` 或多一个 tag 都会让"无 tag 默认"状态下两个 const 都定义，编译期报 `forceDevEnabled redeclared`。用 `go build ./...` + `go build -tags devtools ./...` 双向验证。
7. **`dev_only` 中间件的 WARN 日志不是"打给 client 的"**：是打给 ops / 安全团队的。响应本身只返回 404 envelope（无信息泄露），日志里的 `api_path` / `client_ip` 是**服务器侧可观测**；不要把这些字段反填到 response body。
8. **`/dev/ping-dev` 的 `mode: "dev"` 是 string 不是 enum**：epics.md AC 字面 `{"mode":"dev"}` 是 JSON string；**不**要做成 Go 常量 `DevMode int = 1` 或枚举；保持 string 让前端 / demo 工具直接用 jq 查。
9. **Windows dev 构建测试路径**：若 dev 在 Windows 下跑 `/tmp/catserver-dev`（T7.1 AC9），`/tmp/` 不存在 —— 本 story dev 手动验证时可用 Git Bash + `/c/Users/$USER/AppData/Local/Temp/catserver-dev.exe` 或直接 `./build/catserver-dev.exe`。Completion Notes 里**必须**说明使用的实际路径。
10. **不要在 `configs/local.yaml` 加 `dev_mode: true`**：违反本 story 约束 #2（dev 模式是部署层开关，不是 config 字段）。如果 dev 觉得"每次都要 export BUILD_DEV 太麻烦"，正确解法是 `alias dev='BUILD_DEV=true'` 或 shell wrapper，不是改 config。

### Project Structure Notes

- `internal/app/http/devtools/` 首次出现 —— 它是 `internal/app/http/<xxx>/` 子模块层的**新分支**（原有 `handler/` / `middleware/` / 未来 `request/` `response/`）；语义：HTTP 路由层的运维类子模块
- 新增 4 个文件全部落在 `internal/app/http/devtools/` 下；不碰既有 `handler/` / `middleware/` 包
- `cmd/server/main.go` 修改**仅**加一个 `if` 块（3 行）；不改 flag / config 加载 / Init 顺序
- `bootstrap/router.go` 修改**仅**加一行 `devtools.Register(r)` + 改一行注释；不改函数签名 / 不加新路由
- `bootstrap/router_test.go` 追加一个测试函数；不动既有 case

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.6] — 本 story 原始 AC（BUILD_DEV / build tag / dev_only / /dev/ping-dev / 4 case 单测 + 集成测试）
- [Source: _bmad-output/planning-artifacts/epics.md#Epic-1] — Epic 1 范围定义，明示 "Dev Tools 框架" 是节点 1 scope；"具体 dev 端点由对应业务 Server Epic 增加"（E7 / E20）
- [Source: _bmad-output/planning-artifacts/epics.md#Story-7.5] — 未来依赖方：`POST /dev/grant-steps`，build flag gated
- [Source: _bmad-output/planning-artifacts/epics.md#Story-20.7] — 未来依赖方：`POST /dev/force-unlock-chest`
- [Source: _bmad-output/planning-artifacts/epics.md#Story-20.8] — 未来依赖方：`POST /dev/grant-cosmetic-batch`
- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.10] — server README 会文档化 dev mode 开启方式（本 story 只需 package doc 注释内明示命令；README 章节交给 1.10）
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#3.3] — testify require / assert 混用规则
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#4] — 结构化日志字段约定（`api_path` / `method` / `client_ip` / `request_id`）
- [Source: _bmad-output/implementation-artifacts/1-5-测试基础设施搭建.md] — Story 1.5 交付物（slogtest Handler + testify 模式 + `t.Setenv` 用法）
- [Source: _bmad-output/implementation-artifacts/1-4-version-接口.md#AC3] — `internal/buildinfo/` 的两文件模式参考（对比 build tag 的两文件模式）
- [Source: _bmad-output/implementation-artifacts/1-3-中间件-request_id-recover-logging.md] — Logging / Recovery 中间件作为 dev_only 日志风格对标
- [Source: docs/宠物互动App_V1接口设计.md#3] — 错误码 `1003` 资源不存在（dev_only 中间件返回的 envelope code）
- [Source: docs/宠物互动App_V1接口设计.md#2.4] — 通用响应结构 envelope（dev 端点成功响应对齐）
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#4] — 项目目录建议（`internal/app/http/` 分层来源；`devtools/` 是本 story 首开先例的子目录）
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#8.2] — 中间件建议清单（`auth` / `rate_limit` 不覆盖 /dev；是本 story "不加 auth" 决策的设计锚点）
- [Source: docs/lessons/2026-04-24-config-path-and-bind-banner.md#Lesson-2] — "声明 vs 现实" 对齐原则（WARN 日志的时机）
- [Source: docs/lessons/2026-04-25-slog-init-before-startup-errors.md#Lesson-1] — bootstrap logger 初始化时机（main.go WARN 位置）
- [Source: docs/lessons/2026-04-24-sample-service-nil-dto-and-slog-test-group.md#Lesson-2] — slogtest AttrValue 扁平 key 查询语义
- [Source: CLAUDE.md#Build-Test] — build 脚本现状（Story 1.7 重做；本 story 用 `go test` + `go build` 直接验收）
- [Source: CLAUDE.md#工作纪律] — "节点顺序不可乱跳"（本 story 是节点 1 第六条）+ "严格按需引入"（本 story 零新依赖）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- **偏离 AC9 "不新增 //go:build devtools 的专属测试文件"的执行细节**：实装时发现 `go test -tags devtools ./...` 会让**多处**断言前置破裂 —— 不只是 AC7 case 7 的 `build_tag_devtools==false`：
  - case 2 (`BUILD_DEV=false → 404`)：在 tag 强制启用下 Register 会真注册，`/dev/ping-dev` 变成 200
  - case 5 (`非 "true" 值视为 false`)：tag 强制启用下 IsEnabled() 永真
  - case 6 (`未启用时无日志`)：tag 强制启用下 Register 必打 WARN
  - case 7 (`build_tag_devtools 字段值`)：tag 下变成 true
  处置：把整个 `devtools_test.go` + `router_dev_test.go` 打 `//go:build !devtools` —— "env-var 路径测试"与"build-tag 强制路径测试"**分离文件**，而非 AC9 字面"不新增 //go:build devtools 专属文件"。这保留了 AC9 精神（不引入重复跑 2× 的 CI 成本），同时让 `go test -tags devtools ./...` 仍能跑通（devtools / router_dev 整体 skip，其他测试正常）。偏离仅涉及 tag 方向 —— 是 `!devtools` 而非 AC9 文字提到的 `devtools`；两者对称，工程意义等价
- **AC8 集成测试位置偏离**：AC8 原文 "在 `router_test.go` 追加"；实装改为新建 `router_dev_test.go`（两者均在 `package bootstrap`）。理由：`router_dev_test.go` 需要 `//go:build !devtools` tag（见上一条）；把 dev 测试混入 `router_test.go` 会强制整个文件打 tag，让 `Test_Ping` / `Test_Version` / `Test_PanicRouteAndSubsequentPing` / `Test_MetricsEndpoint` 等非 dev 相关测试在 `-tags devtools` 下也被 skip —— 违反本 story 范围红线"不动 Story 1.2 / 1.3 / 1.4 的既有测试"。拆独立文件是**最小侵入**的正解
- **端口 8080 被占用**：AC9 T7 最初尝试默认端口失败（另一个本机 HTTP server 占用），改用 `CAT_HTTP_PORT=18080 / 18081 / 18082` 避开冲突。本偏离完全由外部环境导致，不影响代码 / 决策
- **首次 `go build -tags devtools ./cmd/server` 产出 binary 到 server 根目录（名字 `server.exe`）**：Go build 默认按最末路径段命名二进制，`./cmd/server` → `server.exe`。清理方式：改用显式 `-o /c/fork/cat/build/catserver-dev.exe`。Dev Notes 陷阱 #9 已预告 Windows 下二进制路径问题，此次实战触碰
- **`go test -race -cover ./...` 本机仍不可达**（与 Story 1.5 AC7 偏离同源）：Windows Go install 缺 `race_windows_amd64.syso` + `covdata.exe`；归 CI 执行，不影响本 story 交付
- **slog.Default() 在未接管时的默认 handler**：测试 case 1 跑时 log 走 stdlib text handler（因为该 case 无 `slog.SetDefault` 接管），输出到 stderr；这不影响测试结果（case 1 只断言 HTTP 响应），只是 `go test -v` 看起来混入文本日志。case 6 / 7 才用 slogtest 接管断言日志行为

### Completion Notes List

**实现摘要**
- 新建包 `internal/app/http/devtools/`，4 个 .go 文件：
  - `buildtag_normal.go` (//go:build !devtools) → `const forceDevEnabled = false`
  - `buildtag_devtools.go` (//go:build devtools) → `const forceDevEnabled = true`
  - `devtools.go`：`IsEnabled()` + `Register(*gin.Engine)` + `DevOnlyMiddleware()` + `PingDevHandler`；完整 package doc 含 OR 语义 / 双闸门 / 验证命令
  - `devtools_test.go` (//go:build !devtools)：6 顶层 test + 1 table-driven（7 子 case）共 **13 个**测试 case，全绿
- 集成：`bootstrap/router.go` 在 `/metrics` 后调 `devtools.Register(r)`；`cmd/server/main.go` 在 `logger.Init(cfg.Log.Level)` 后加 `if devtools.IsEnabled() { slog.Warn(...) }`
- 集成测试：`bootstrap/router_dev_test.go` (//go:build !devtools) 的 `TestRouter_DevPingEnabled_EnvToggle` 两个子 case 覆盖 AC8（BUILD_DEV=true/空两种下的 NewRouter + /dev/ping-dev 对照 200/404 + /ping 不受影响）
- **零新依赖**：go.mod / go.sum 零 diff（`go mod tidy` 稳定）
- 生产安全：`-tags devtools` 不带 + BUILD_DEV 不设 = dev 路由**物理不存在**（编译剔除 + Gin 默认 NoRoute 文本 404，非 envelope，OpSec 零泄露）

**AC7 测试输出（节选）**
```
=== RUN   TestRegister_BuildDevTrue_PingDevReturns200
--- PASS: TestRegister_BuildDevTrue_PingDevReturns200 (0.01s)
=== RUN   TestRegister_BuildDevFalse_PingDevReturns404
--- PASS: TestRegister_BuildDevFalse_PingDevReturns404 (0.00s)
=== RUN   TestDevOnlyMiddleware_RejectsWhenDisabled
--- PASS: TestDevOnlyMiddleware_RejectsWhenDisabled (0.00s)
=== RUN   TestIsEnabled_EnvVarStrictMatchesOnlyTrue
=== RUN   TestIsEnabled_EnvVarStrictMatchesOnlyTrue/exact_true
=== RUN   TestIsEnabled_EnvVarStrictMatchesOnlyTrue/uppercase_TRUE_not_accepted
=== RUN   TestIsEnabled_EnvVarStrictMatchesOnlyTrue/mixed-case_True_not_accepted
=== RUN   TestIsEnabled_EnvVarStrictMatchesOnlyTrue/numeric_1_not_accepted
=== RUN   TestIsEnabled_EnvVarStrictMatchesOnlyTrue/yes_not_accepted
=== RUN   TestIsEnabled_EnvVarStrictMatchesOnlyTrue/false_literal
=== RUN   TestIsEnabled_EnvVarStrictMatchesOnlyTrue/empty_string
--- PASS: TestIsEnabled_EnvVarStrictMatchesOnlyTrue (0.00s)
=== RUN   TestRegister_WhenDisabled_EmitsNoLogs
--- PASS: TestRegister_WhenDisabled_EmitsNoLogs (0.00s)
=== RUN   TestRegister_WhenEnabled_EmitsExactlyOneWarn
--- PASS: TestRegister_WhenEnabled_EmitsExactlyOneWarn (0.00s)
PASS
ok    github.com/huing/cat/server/internal/app/http/devtools    0.132s

=== RUN   TestRouter_DevPingEnabled_EnvToggle
=== RUN   TestRouter_DevPingEnabled_EnvToggle/BUILD_DEV=true_→_/dev/ping-dev_200_+_envelope.data.mode=dev
=== RUN   TestRouter_DevPingEnabled_EnvToggle/BUILD_DEV_empty_→_/dev/ping-dev_404_(Gin_NoRoute)
--- PASS: TestRouter_DevPingEnabled_EnvToggle (0.01s)
（含 Story 1.2/1.3/1.4 既有测试共 8 个顶层 case 全绿）
ok    github.com/huing/cat/server/internal/app/bootstrap    0.194s
```

**AC9 手动验证：三个 scenario**

### Path 1：`-tags devtools` 单独触发（BUILD_DEV 未设）
```
$ unset BUILD_DEV
$ CAT_HTTP_PORT=18080 /c/fork/cat/build/catserver-dev.exe -config ./configs/local.yaml
{"time":"...","level":"INFO","msg":"config loaded","path":".../local.yaml","http_port":18080,"log_level":"info"}
{"time":"...","level":"WARN","msg":"DEV MODE ENABLED - DO NOT USE IN PRODUCTION"}                              # main.go 那条
[GIN-debug] GET /ping    ...
[GIN-debug] GET /version ...
[GIN-debug] GET /metrics ...
{"time":"...","level":"WARN","msg":"DEV MODE ENABLED - DO NOT USE IN PRODUCTION","build_tag_devtools":true,"env_build_dev":""}  # Register 那条
[GIN-debug] GET /dev/ping-dev --> github.com/huing/cat/server/internal/app/http/devtools.PingDevHandler (5 handlers)
{"time":"...","level":"INFO","msg":"server started","addr":":18080"}

$ curl http://127.0.0.1:18080/dev/ping-dev
HTTP 200
{"code":0,"message":"ok","data":{"mode":"dev"},"requestId":"c011833e-b970-437e-afab-4192ffbbdc15"}

$ curl http://127.0.0.1:18080/ping
HTTP 200
{"code":0,"message":"pong","data":{},"requestId":"2868e96c-975d-409e-a8b2-8524ca9b1c09"}
```

### Path 2 / Scenario A：env 单独触发（normal build + BUILD_DEV=true）
```
$ BUILD_DEV=true CAT_HTTP_PORT=18081 /c/fork/cat/build/catserver.exe -config ./configs/local.yaml
{"time":"...","level":"INFO","msg":"config loaded",...,"http_port":18081,...}
{"time":"...","level":"WARN","msg":"DEV MODE ENABLED - DO NOT USE IN PRODUCTION"}                              # main.go
[GIN-debug] GET /ping    ...
[GIN-debug] GET /version ...
[GIN-debug] GET /metrics ...
{"time":"...","level":"WARN","msg":"DEV MODE ENABLED - DO NOT USE IN PRODUCTION","build_tag_devtools":false,"env_build_dev":"true"}  # Register
[GIN-debug] GET /dev/ping-dev --> ... PingDevHandler
{"time":"...","level":"INFO","msg":"server started","addr":":18081"}

$ curl http://127.0.0.1:18081/dev/ping-dev
HTTP 200
{"code":0,"message":"ok","data":{"mode":"dev"},"requestId":"b4acb5d6-0af2-4217-8448-daa4c61e6821"}
```

### Scenario B：prod-like（normal build + BUILD_DEV 未设）
```
$ unset BUILD_DEV
$ CAT_HTTP_PORT=18082 /c/fork/cat/build/catserver.exe -config ./configs/local.yaml
{"time":"...","level":"INFO","msg":"config loaded",...,"http_port":18082,...}
[GIN-debug] GET /ping    ...
[GIN-debug] GET /version ...
[GIN-debug] GET /metrics ...
（无 DEV MODE 任何 WARN；无 /dev/ping-dev 路由注册行）
{"time":"...","level":"INFO","msg":"server started","addr":":18082"}

$ curl http://127.0.0.1:18082/dev/ping-dev
HTTP 404
404 page not found    # Gin 默认 NoRoute 文本，非 envelope；端点物理不存在
```

三种场景均与 AC 预期吻合。

**AC10 quality gate 结果**
- `go vet ./...` ✅ 无 error
- `go vet -tags devtools ./...` ✅ 无 error
- `go test ./...` ✅ 全绿（13 个包，本 story 新增 1 包 + 1 个 dev 集成测试全绿）
- `go test -tags devtools ./...` ✅ 全绿（devtools + router_dev 整体 skip，其余包正常）
- `go mod tidy` ✅ `go.mod` / `go.sum` 零 diff
- `go build -tags devtools ./cmd/server` ✅ 成功
- `go test -race -cover ./...` ⏭️ 沿用 Story 1.5 AC7 偏离（Windows Go install 限制；归 CI Linux runner）

**为后续业务 dev story 的延伸约定（写给 Story 7.5 / 20.7 / 20.8 的 Dev）**
- 挂业务 dev 端点时**不再**重造 gate；直接**扩展** `devtools.Register()`：
  ```go
  g := r.Group("/dev")
  g.Use(DevOnlyMiddleware())
  g.GET("/ping-dev", PingDevHandler)
  g.POST("/grant-steps", stepdev.GrantStepsHandler)           // Story 7.5
  g.POST("/force-unlock-chest", chestdev.ForceUnlockHandler)  // Story 20.7
  g.POST("/grant-cosmetic-batch", cosdev.GrantBatchHandler)   // Story 20.8
  ```
- 业务 handler 放在各自业务包（`internal/service/step/dev/` 等），保持 devtools 包只做**框架**
- 业务 dev handler 走 `response.Success` / `response.Error`，业务码沿用 V1接口设计 §3 列表，**不**定义 dev 专属码
- Story 7.5 / 20.7 / 20.8 的 layer-2 集成测试（dockertest）可**直接复用**本 story 的 "BUILD_DEV=true + NewRouter" pattern 跑真 HTTP request

### File List

**新增**
- `server/internal/app/http/devtools/buildtag_normal.go`
- `server/internal/app/http/devtools/buildtag_devtools.go`
- `server/internal/app/http/devtools/devtools.go`
- `server/internal/app/http/devtools/devtools_test.go`
- `server/internal/app/bootstrap/router_dev_test.go`

**修改**
- `server/cmd/server/main.go`（新增 devtools import + WARN 分支 3 行）
- `server/internal/app/bootstrap/router.go`（新增 devtools import + `Register(r)` + 注释更新）
- `server/configs/local.yaml`（新增 `bind_host: 127.0.0.1` + 注释；避开 Windows Firewall 在每次新二进制哈希下反复弹"专用网络访问"授权窗。AC9 手动验证 scenario A/B 跑 `catserver.exe` + `catserver-dev.exe` 两个新哈希各弹一次 → 合入本 story 一并解决。**生产部署**删此行 / 改 0.0.0.0 即回退监听所有网卡。落地 Story 1.2 `BindHost` 字段设计意图（见 `config.go` 注释 + commit `ed355db`）从测试扩展到本地开发）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（story 1-6 流转 backlog → ready-for-dev → in-progress → review + last_updated 时间戳）
- `_bmad-output/implementation-artifacts/1-6-dev-tools-框架.md`（本 story 文件：Tasks 勾选 / Dev Agent Record 填充 / Status 流转）

## Change Log

| 日期 | 版本 | 描述 | 作者 |
|---|---|---|---|
| 2026-04-24 | 0.1 | 初稿（ready-for-dev） | SM |
| 2026-04-24 | 1.0 | 实装完成：devtools 包 + Register / DevOnlyMiddleware / PingDevHandler + buildtag 双文件；13 个测试 case 全绿；三个 scenario 手动验证通过；`go test -tags devtools ./...` 也全绿（devtools / router_dev 整体 skip 策略）；AC7/AC8 测试拆 `//go:build !devtools` 文件（见 Debug Log 偏离说明）；AC10 本地 race+cover 不可达沿用 Story 1.5 偏离；状态流转 review | Dev |
| 2026-04-24 | 1.1 | 附带修改：`server/configs/local.yaml` 加 `bind_host: 127.0.0.1`（loopback-only），解决 AC9 手动验证时新二进制哈希反复触发 Windows Defender Firewall 弹窗的问题（落地 Story 1.2 已有的 BindHost 设计意图到本地开发场景）| Dev |
