# Story 1.4: /version 接口

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 测试与运维者,
I want 调用 `GET /version` 看到当前服务的 git commit 和构建时间,
so that demo / 联调 / 事故排查时能**一眼确认**运行的是哪个版本的二进制.

## 故事定位（节点 1 第四条实装 story）

- Story 1.2 已建好 `cmd/server/main.go` 入口 + Gin 裸引擎 + `/ping`
- Story 1.3 已挂 request_id / logging / recover 三件套 + slog JSON Handler + Prometheus `/metrics`
- 本 story 新增**一个运维端点** `GET /version`，通过 **`-ldflags -X`** 在编译期注入 git short commit 与 builtAt，运行期以统一 envelope 返回
- 按**严格按需引入**原则（`MVP节点规划与里程碑.md` §2 原则 7），这条 story **只做 /version**：
  - **不**引入 `internal/app/http/request/` / `internal/app/http/response/` 分层（业务接口才建；Epic 4 引入）
  - **不**提前做 `scripts/build.sh` 重做（Story 1.7 的 scope；本 story 仅要求 `go build -ldflags "-X ..."` **命令行**可用，不必动 `build.sh`）
  - **不**引入 `AppError`（Story 1.8；`/version` 本身无业务错误路径，不需要）
  - **不**动 Story 1.3 已完成的中间件 / logger / metrics；本 story 的 `/version` 自然经过三件套，**请求会被 logging + metrics 正常记录**（验收用）

**范围红线**：
- 只新建 `internal/app/http/handler/version_handler.go` + 其测试 + 修改 `cmd/server/main.go` 接收 ldflags 注入变量 + 修改 `bootstrap/router.go` 注册路由
- **不**建 `internal/domain/version/` 或任何 service / repo —— `/version` 是无业务语义的纯展示端点，handler 直接读变量即可
- **不**动 `bootstrap/server.go` 的 `Run` 拆分逻辑（Story 1.2 review fix 成果）
- **不**把 ldflags 变量放进 `cfg` / YAML —— 它们是**编译期**注入、**非运行期可配**，与 `config.Config` 语义不同
- **不**在 `/version` 响应里塞 `goVersion` / `os` / `arch` 等扩展字段 —— 按 AC 严格对齐 `{commit, builtAt}` 两字段；后续 story 要加再说

## Acceptance Criteria

**AC1 — 依赖**

本 story **零新依赖**：不新增任何第三方库；只用 stdlib（`runtime/debug` **也不用**，靠 ldflags 就够）。

`go.mod` / `go.sum` 不应被本 story 修改。

**AC2 — 目录与文件**

必须**新建**：

```
server/
├─ internal/
│  └─ app/
│     └─ http/
│        └─ handler/
│           ├─ version_handler.go          # VersionHandler + VersionResponse 结构
│           └─ version_handler_test.go
```

必须**修改**：

- `server/cmd/server/main.go`：新增包级变量 `commit` / `builtAt`（接受 ldflags 注入；默认 `"unknown"`），**启动时**传给 bootstrap（或放在独立 `internal/buildinfo/` 包，见 AC3 决策）
- `server/internal/app/bootstrap/router.go`：在三件套中间件挂好之后、在 `GET /ping` 旁边**新增** `GET /version`，**不**走 `/api/v1` 前缀；注释里更新"Future"清单
- `server/internal/app/bootstrap/router_test.go` 或新增 `router_version_test.go`：补一个集成 case 验证 `/version` 端点 happy path（见 AC7）

**不**新建：`internal/buildinfo/` **或** `internal/infra/buildinfo/`（见 AC3 决策）、`internal/app/http/response/*`、`internal/app/http/request/*`。

**AC3 — ldflags 变量位置决策**

两个位置二选一，各有 trade-off：

| 位置 | 写法 | 优点 | 缺点 |
|---|---|---|---|
| **方案 A：放 `main` 包** | `var commit, builtAt string` 在 `cmd/server/main.go` 顶部；ldflags `-X main.commit=...` | 符合 epics.md AC 原字面（`"-X main.commit=..."`）；零额外包 | main 包变量不能被 handler 直接读；必须**启动时**把两个值**显式传参**注入到 handler（通过闭包 factory 或全局 setter） |
| **方案 B：独立 `internal/buildinfo/` 包** | `var Commit, BuiltAt string` 在 `internal/buildinfo/buildinfo.go`；ldflags `-X github.com/huing/cat/server/internal/buildinfo.Commit=...` | handler 可直接 import 读取；测试时可 override 包变量；未来想加 `goVersion` / `tag` 也有地方放 | 多一个小包；ldflags 路径变长 |

**本 story 选方案 B（`internal/buildinfo/`）**，理由：
- 方案 A 的"通过闭包 factory / setter 注入 handler"会让 `NewRouter()` 签名变成 `NewRouter(commit, builtAt string) *gin.Engine`，污染 bootstrap API —— 而 Story 1.6（dev flag）、Story 1.8（AppError）、Epic 4（auth 配置）可能还要往 router 塞更多参数，这条路最后会变成上帝签名
- buildinfo 是**纯只读常量集**，包级变量 + 编译期 `-ldflags -X` 是 Go 生态事实标准（cobra / kubectl / docker CLI 都是这么做的）
- handler `import "buildinfo"; return buildinfo.Commit` 读起来干净

**具体落地**：
```go
// internal/buildinfo/buildinfo.go
package buildinfo

// Commit 是编译期通过 -ldflags -X 注入的 git short hash。
// 未注入时（例如直接 `go run` 或 test binary）默认为 "unknown"。
var Commit = "unknown"

// BuiltAt 是编译期通过 -ldflags -X 注入的 ISO8601 UTC 构建时间戳。
// 未注入时默认为 "unknown"。
var BuiltAt = "unknown"
```

注意：`epics.md` Story 1.4 AC 文字是 `"-X main.commit=..."`；本 story AC 明确**用方案 B 代替**（更好的工程惯例），不违反 epics 精神（epics 本意是"ldflags 注入 commit / builtAt"，路径细节是实现裁量权）。在 Completion Notes 里**明示**这点偏离。

**AC4 — `/version` handler 实装（`internal/app/http/handler/version_handler.go`）**

```go
package handler

import (
    "github.com/gin-gonic/gin"

    "github.com/huing/cat/server/internal/buildinfo"
    "github.com/huing/cat/server/internal/pkg/response"
)

// VersionResponse 是 /version 的 data 字段结构。
// JSON tag 对齐 V1接口设计 §2.4 小驼峰约定（builtAt 而非 built_at）。
type VersionResponse struct {
    Commit  string `json:"commit"`
    BuiltAt string `json:"builtAt"`
}

// VersionHandler 返回当前服务的 git commit / 构建时间。
// 两字段的值在编译期通过 -ldflags -X 注入 buildinfo 包；
// 未注入时（例如直接 go run 或 test binary）默认返回 "unknown"。
//
// 响应严格对齐统一 envelope：{code:0, message:"ok", data:{commit, builtAt}, requestId}。
func VersionHandler(c *gin.Context) {
    response.Success(c, VersionResponse{
        Commit:  buildinfo.Commit,
        BuiltAt: buildinfo.BuiltAt,
    }, "ok")
}
```

**契约**：
- `message` 固定 `"ok"`（不用 `"version"` 或其他——`response.Success` 会放进 envelope.message；与 `/ping` 的 `"pong"` 对齐为"简短 lowercase 动词/状态词"风格）
- `data` 是 `VersionResponse` 对象（非 `map[string]any`），便于 iOS 侧 Codable 解析（Epic 2 Story 2.5 会用到）
- **不**手动判断 `Commit == "" → "unknown"`：buildinfo 包的**默认值就是** `"unknown"`，handler 不再做 nil-check（AC5 的"空串 → unknown"语义在 buildinfo 层已保证）
- **不**输出 `time.Now()` 或任何运行期时间戳 —— `builtAt` 是**构建时间**不是"响应时间"

**AC5 — 空值保护（AC5 from epics）**

`epics.md` 原文：*"edge: ldflags 未注入（空字符串）→ 返回 commit/builtAt 都是 'unknown'（而非空字符串导致 JSON 异常）"*。

本 story 的落地方式：
- buildinfo 包变量**声明时赋默认值** `"unknown"`（见 AC3 代码）
- ldflags **未传**时，变量保持默认值 `"unknown"` —— 天然满足 AC
- ldflags **传空串**（`-X "...Commit="`）时：Go ldflags 会把空串覆盖进去，**默认值丢失** —— 需要 handler 层做兜底吗？

**决策**：**不**在 handler 层做"空串 → unknown"转换。理由：
- `go build -ldflags "-X ...Commit="` 是人类**显式**写进去的空值，语义上就是"明确标记为空"
- Story 1.7 重做 `build.sh` 会写 `COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")` —— shell 层兜底更合适
- handler 层做转换会让单元测试难以区分"真的未注入"和"注入了空串" —— 测试只能靠"设变量 = 空 → 断言 response = unknown"，丢失 fidelity

**对测试的影响**：AC6 的 "edge: 未注入" case **直接用 buildinfo 包默认值**（`buildinfo.Commit == "unknown"`）验证 handler；**不**写"注入空串"的 case。

**AC6 — 单元测试覆盖（≥ 3 case，`version_handler_test.go`）**

继续用 stdlib `testing`（Story 1.5 之前不引入 testify）。

| # | case | 构造 | 断言 |
|---|---|---|---|
| 1 | happy: 注入了 commit + builtAt → handler 返回正确 JSON | 测试 `TestMain` 或测试顶部 `buildinfo.Commit = "abc1234"; buildinfo.BuiltAt = "2026-04-26T10:00:00Z"`（保存原值并 defer 恢复）；`gin.TestMode`；注册 `/version`；httptest 请求 | status=200；body envelope `code==0` / `message=="ok"` / `requestId != ""`；`data.commit=="abc1234"` / `data.builtAt=="2026-04-26T10:00:00Z"` |
| 2 | edge: 未注入 → 默认 `"unknown"` | 保存 buildinfo 原值，**设回默认** `buildinfo.Commit="unknown"` / `buildinfo.BuiltAt="unknown"`；同上构造 | `data.commit=="unknown"` / `data.builtAt=="unknown"`（**不是** 空串，**不是** `null`）|
| 3 | edge: response 严格符合统一 envelope | 同 case 1 构造；断言 body JSON unmarshal 后含 4 个顶层字段：`code` (int) / `message` (string) / `data` (object，含 `commit` / `builtAt` 两字段) / `requestId` (string) | 4 个字段都存在；`code==0`；`requestId` 非空（中间件挂好时应为 UUIDv4，测试里也可能是兜底值）；`data` 是 object 非 null 非 array |

**测试工具约定**：
- `gin.SetMode(gin.TestMode)` 建 `gin.New()` + `r.GET("/version", VersionHandler)`（**不**调 `bootstrap.NewRouter` —— handler 单测不需要中间件链）
- 但 case 3 的 `requestId` 字段 —— 不走 middleware 时 `response.Success` 的 `requestIDFromCtx` 会回落 header → 再回落 `"req_xxx"`。测试构造 `httptest.NewRequest` 时**不传** `X-Request-Id` header → 期望值 `"req_xxx"`（`response.go:46` 的兜底值），**不是**空串
- **变量保护模板**：
  ```go
  func TestVersionHandler_HappyPath(t *testing.T) {
      origCommit, origBuiltAt := buildinfo.Commit, buildinfo.BuiltAt
      defer func() { buildinfo.Commit, buildinfo.BuiltAt = origCommit, origBuiltAt }()
      buildinfo.Commit = "abc1234"
      buildinfo.BuiltAt = "2026-04-26T10:00:00Z"
      // ... 构造 router + request ...
  }
  ```
  **重要**：**不**用 `t.Cleanup` —— Go 1.14+ 支持，但 `defer` 更清晰、与 Story 1.2 / 1.3 测试风格对齐

**AC7 — 集成测试（router 层 + buildinfo 注入模拟）**

`epics.md` 原 AC：*"集成测试覆盖: 编译时注入 mock commit `abc1234` → 启动 → curl `GET /version` → 返回的 commit = `abc1234`"*。

**真实** `go build -ldflags` 注入需要在 test 里 `os/exec` 拉起一个子进程 binary —— 过重、慢、脆弱（CI 环境下 `go build` 路径 / Windows `.exe` 后缀 / 端口占用都是坑）。**等价的集成测试**：在测试里**直接覆盖** `buildinfo.Commit = "abc1234"` 然后走**完整 router**（`bootstrap.NewRouter()` → 三件套中间件 → VersionHandler）。覆盖包变量与 ldflags 注入**在运行期语义完全等价**（都是改 `.rodata` / `.data` 段里的字符串），唯一差别是注入时机 —— 而本 story 的集成测试验证的是**读取路径**的正确性，不是"编译器 ldflags 机制本身"（后者是 Go 工具链的事，不该由业务 story 验证）。

**真正的 ldflags 编译期注入**由 **AC8 端到端手工验证** 覆盖。dev 会实际 `go build -ldflags` 出一个 binary，curl 它，把输出贴 Completion Notes —— 这是节点 1 Demo 验收前的常规手工检查。

**集成 case**（`bootstrap/router_test.go` 扩展或新增 `router_version_test.go`）：

| # | case | 断言 |
|---|---|---|
| 4 | 挂完三件套中间件后，模拟 ldflags 注入 `commit="abc1234"` / `builtAt="2026-04-26T00:00:00Z"` → `/version` 端点返回完整 envelope | status=200；`code==0` / `message=="ok"`；`data.commit=="abc1234"` / `data.builtAt=="2026-04-26T00:00:00Z"`；`requestId` 匹配 UUIDv4 正则；响应 header `X-Request-Id` 与 body `requestId` 一致 |

**实现模板**：

```go
func TestRouter_Version(t *testing.T) {
    // 模拟 ldflags 注入
    origCommit, origBuiltAt := buildinfo.Commit, buildinfo.BuiltAt
    defer func() { buildinfo.Commit, buildinfo.BuiltAt = origCommit, origBuiltAt }()
    buildinfo.Commit = "abc1234"
    buildinfo.BuiltAt = "2026-04-26T00:00:00Z"

    gin.SetMode(gin.TestMode)
    r := NewRouter()
    req := httptest.NewRequest(http.MethodGet, "/version", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)
    // ... 断言 ...
}
```

**注意**：该测试**不能** `t.Parallel()`（全局变量污染问题；见 Dev Notes §常见陷阱 #5）。

**AC8 — 端到端 ldflags 注入验证（手工，记录到 Completion Notes）**

> 本 story **不**修改 `scripts/build.sh`（Story 1.7 的 scope），所以以下命令是 dev 在本地**手工**跑一次，把输出贴 Completion Notes 即可。

**步骤**：

```bash
cd server
COMMIT_HASH="$(git rev-parse --short HEAD)"   # 比如 "b3aad1b"
BUILT_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"     # 比如 "2026-04-26T10:05:42Z"
go build \
  -ldflags "-X 'github.com/huing/cat/server/internal/buildinfo.Commit=${COMMIT_HASH}' -X 'github.com/huing/cat/server/internal/buildinfo.BuiltAt=${BUILT_AT}'" \
  -o ../build/catserver.exe \
  ./cmd/server/
../build/catserver.exe -config configs/local.yaml &   # 或用 start 命令启动
sleep 1
curl http://127.0.0.1:18082/version
```

**期望输出**（JSON 摘录）：
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "commit": "b3aad1b",
    "builtAt": "2026-04-26T10:05:42Z"
  },
  "requestId": "<UUIDv4>"
}
```

Dev 需要把**实际** curl 输出贴到 Completion Notes。

**手工验证步骤 2** —— 未注入（`go run`）：

```bash
cd server
go run ./cmd/server/ -config configs/local.yaml &
sleep 1
curl http://127.0.0.1:18082/version
# 期望 data.commit == "unknown" AND data.builtAt == "unknown"
```

**AC9 — router 挂载更新**

修改 `internal/app/bootstrap/router.go`：

```go
r.GET("/ping", handler.PingHandler)
r.GET("/version", handler.VersionHandler)   // 新增本行
r.GET("/metrics", gin.WrapH(metrics.Handler()))
```

**并同步**更新顶部 Future 注释：
```go
// Future: Story 1.6 registers /dev/* group behind BUILD_DEV flag.
```

（移除"Story 1.4 adds GET /version"那条——已完成）

**AC10 — 验收 build 可跑**

- `cd server && go vet ./...` 无 error
- `cd server && go test ./...` 通过（本 story 新增 3 个 handler 单测 + 1 个 router 集成 case；Story 1.2 / 1.3 全部既有测试继续绿）
- `cd server && go build -o ../build/catserver.exe ./cmd/server/` 成功（**不带 ldflags**，用于验证默认 `"unknown"` 路径）
- 从 repo root 启动 `./build/catserver.exe`：
  1. `curl http://127.0.0.1:18082/version` → `data.commit == "unknown"` / `data.builtAt == "unknown"`
- 再带 ldflags 编译一次（见 AC8）：
  2. `curl http://127.0.0.1:18082/version` → `data.commit == "<真实 short hash>"` / `data.builtAt == "<ISO8601>"`
- dev 需把两次 curl 输出贴 Completion Notes

## Tasks / Subtasks

- [x] **T1**：buildinfo 包（AC3）
  - [x] T1.1 建 `server/internal/buildinfo/buildinfo.go`（2 个包级变量 `Commit` / `BuiltAt`，默认值 `"unknown"`，带包头注释说明 ldflags 注入）

- [x] **T2**：Version handler（AC2 / AC4）
  - [x] T2.1 建 `server/internal/app/http/handler/version_handler.go`（`VersionResponse` struct + `VersionHandler` 函数）
  - [x] T2.2 建 `server/internal/app/http/handler/version_handler_test.go`（≥ 3 case，按 AC6 表格）

- [x] **T3**：Router 挂载（AC9）
  - [x] T3.1 修改 `server/internal/app/bootstrap/router.go`：新增 `r.GET("/version", handler.VersionHandler)`；更新 Future 注释

- [x] **T4**：Router 集成测试（AC7）
  - [x] T4.1 新增 `server/internal/app/bootstrap/router_version_test.go`：添加 `TestRouter_Version` integration case，模拟 ldflags 注入 `commit="abc1234"` / `builtAt="2026-04-26T00:00:00Z"`，断言完整 envelope + UUIDv4 requestId + header/body 一致

- [x] **T5**：本地 AC10 + AC8 验收
  - [x] T5.1 `go vet ./...` pass
  - [x] T5.2 `go test ./...` pass（含本 story 新增 4 个 case）
  - [x] T5.3 `go build` pass（不带 ldflags）
  - [x] T5.4 启动后 curl `/version` 返回 `unknown` / `unknown` —— 贴 Completion Notes
  - [x] T5.5 带 ldflags 重编译（按 AC8 命令）→ curl `/version` 返回真实 hash / ISO 时间戳 —— 贴 Completion Notes

- [x] **T6**：收尾
  - [x] T6.1 Completion Notes 补全（含 AC8 两次 curl 输出 + AC3 方案 B 偏离说明）
  - [x] T6.2 File List 填充
  - [x] T6.3 状态流转 `ready-for-dev → in-progress → review`

## Dev Notes

### 项目关键约束（必读，勿绕过）

1. **ldflags 变量放 `internal/buildinfo/` 不放 `main` 包**：见 AC3 决策表。epics.md 原 AC 字面是 `"-X main.commit=..."`，本 story 偏离到方案 B，在 Completion Notes 明示"为何偏离 + 具体 ldflags 路径"。
2. **handler 层不做"空串 → unknown"回落**：buildinfo 包变量的**默认值**已经是 `"unknown"`，handler 只负责读。空串（dev 显式用 `-X "...Commit="` 覆盖）属于"人类故意标空"，不做兜底 —— 见 AC5 决策。
3. **`/version` 是运维端点**，挂在**所有业务路由之外**（不走 `/api/v1` 前缀），与 `/ping` / `/metrics` 对齐。Story 1.3 router 顶部已留 "Story 1.4 adds GET /version" 占位，本 story 落地后需要**同步移除**该占位（AC9）。
4. **`/version` 请求会被 Story 1.3 的 logging 中间件计入日志、被 metrics 中间件计入 `cat_api_requests_total`**。这是接受的行为（运维端点也是端点）；验收时可以观察到日志里有 `api_path="/version"` 的一条，metrics 里有 `cat_api_requests_total{api_path="/version",code="200",method="GET"}`。**不**在 `/version` 里 skip 这些中间件。
5. **`message` 字段用 `"ok"` 不用 `"version"`**：与 `/ping` 的 `"pong"` 对齐为"简短 lowercase 状态词"。V1接口设计 §2.4 对 `message` 语义是"给人读的短语"，`"ok"` 是最标准选择。
6. **ISO8601 builtAt 格式用 UTC (`+%Y-%m-%dT%H:%M:%SZ`)**：`Z` 后缀明示 UTC，避免本地时区污染 demo/联调现场；Story 1.7 重做 `build.sh` 时 `date -u` 必须是 `-u`。
7. **handler 变量名约定**：`VersionHandler`（大驼峰导出 + `Handler` 后缀）与既有 `PingHandler` 对齐；**不要**写成 `GetVersion` / `HandleVersion` / `Version`。

### 为什么不做 response 分层

Story 1.2 的 `/ping` 直接用 `response.Success(c, map[string]any{}, "pong")`；本 story 的 `/version` 类似用 `response.Success(c, VersionResponse{...}, "ok")`。**不**引入 `internal/app/http/response/version_response.go` 级别的专属响应层，理由：

- `VersionResponse` 只有 2 字段，放 handler 文件顶部更易读（与 handler 共生共死）
- 未来业务 handler（Epic 4+）的 response struct 可能复杂（含嵌套、Codable 兼容、i18n message key 等），**那时**才抽 `response/` 目录更合理
- 现在就抽会造出"只有 /version 一个孤独实例"的空目录，违反**按需引入**原则

### `GET /version` 是否加到 V1接口设计文档？

`docs/宠物互动App_V1接口设计.md` 目前没有 `/ping` / `/version` / `/metrics` 的条目。epics.md §节点-1 Demo 验收 AC 标了"docs/宠物互动App_V1接口设计.md: 是否需要把 `/ping` / `/version` 接口加入文档？建议加，作为最简接口示范"。

**本 story 不做这个文档同步**。理由：
- 节点 1 Demo 验收 story（3.3）才是"文档同步"的正式 story，那时一批加
- 本 story 只管 handler 实装 + 测试

不过 Completion Notes 里可以**留一行** TODO 提醒 Story 3.3 补文档。

### 为什么 VersionHandler 不吃 context

可能的设计替代：`func VersionHandler(commit, builtAt string) gin.HandlerFunc`（闭包 factory 注入值）。否决理由：
- 闭包 factory 让 router.go 挂载写法变成 `r.GET("/version", handler.VersionHandler(buildinfo.Commit, buildinfo.BuiltAt))`，**每次**启动时把值 snapshot 进闭包
- 但 buildinfo.Commit / BuiltAt 本身就是**包级常量（编译期固定）**，不会在运行期变化 —— snapshot vs 读变量**语义完全等价**
- 直读包变量写法更简单，测试时 override 包变量更灵活（无需构造闭包）

### `requestIDFromCtx` 兜底值对测试的影响

`response.go:46` 兜底返回字符串 `"req_xxx"`（见 Story 1.3 AC10）。本 story 的 AC6 case 3 直接用 `r := gin.New(); r.GET("/version", VersionHandler)` 不挂 middleware，所以：
- `c.Get("request_id")` 拿不到（没有 RequestID middleware）
- `c.Request.Header.Get("X-Request-Id")` 也拿不到（测试请求没设 header）
- 回落到 `"req_xxx"`

测试断言应为 `env.RequestID != ""`（宽松，兼容未来兜底值变化），**不**写死 `env.RequestID == "req_xxx"`（与 Story 1.2 `TestRouter_Ping` 做法一致）。

### Previous Story Intelligence（Story 1.3 交付物关键点）

- Story 1.3 的三件套中间件顺序 **request_id → logging → recover → handler** 已锁，本 story 新增的 `/version` 自然沿用该链路
- Story 1.3 `router.go:20-21` 的 Future 注释含 "Story 1.4 adds GET /version" —— 本 story 必须把这行**改掉**（只留 "Story 1.6 registers /dev/* group behind BUILD_DEV flag."）
- Story 1.3 `router_test.go` 已建好 4 个 integration case（`TestRouter_Ping` / `TestRouter_PingRequestIDIsUUIDv4` / `TestRouter_PanicRouteAndSubsequentPing` / `TestRouter_MetricsEndpoint`）。本 story 新增 case 放同文件末尾或独立 `router_version_test.go`，**不**动这 4 个 case
- Story 1.3 完成后 `logger.Init` 被 `main.go` 在 config 加载**之前**调一次 bootstrap（见 Lesson 2 `2026-04-25-slog-init-before-startup-errors.md`）—— 本 story 不改这段
- Story 1.2 / 1.3 的 `bootstrap.Run` 里 `net.Listen` + `srv.Serve` 拆分逻辑**不动**

### Lessons Index（可能相关的过去教训）

- `docs/lessons/2026-04-24-config-path-and-bind-banner.md` Lesson 2 Meta："声明 vs 现实" —— 本 story 的 buildinfo 变量是**编译期注入**的事实，handler 不要尝试在运行期"重新计算"（`time.Now()`、`runtime/debug.ReadBuildInfo()` 都不要用）；尊重 ldflags 注入结果 = 尊重构建事实
- `docs/lessons/2026-04-25-slog-init-before-startup-errors.md` —— 和本 story 关系不大，但读一眼保持 `main.go` 的 `logger.Init("info")` → config → `logger.Init(cfg.Log.Level)` 两段式初始化不变

### Git intelligence

- 最近 5 个 commit（按时间逆序）：
  - `b3aad1b chore(story-1-3): 收官 Story 1.3 + 归档 story 文件`
  - `841afd6 chore(lessons): backfill commit hash for 2026-04-25 lesson`
  - `0a0d108 feat(server): Epic1/1.3 中间件三件套 + slog + prometheus metrics`
  - `4516b31 chore(story-1-2): 收官 Story 1.2 + 归档 story 文件 + 新增 /fix-review 命令`
  - `90e40e1 chore(lessons): backfill commit hash for 2026-04-24 lesson`
- commit message 风格：Conventional Commits，中文 subject，scope 用 `story-X-Y` 或模块名
- 本 story 建议 commit message：`feat(server): Epic1/1.4 /version 接口 + buildinfo 包 + ldflags 注入验证`

### 常见陷阱

1. **ldflags 变量**必须是**包级 var，不是 const**：Go 的 `-ldflags -X` **只能**覆盖 var，不能覆盖 const。写成 `const Commit = "unknown"` 会编译通过但注入无效，`/version` 永远返回 `"unknown"`。
2. **ldflags 路径**必须是**完整 import path + 变量名**：`-X 'github.com/huing/cat/server/internal/buildinfo.Commit=abc1234'`。写成 `-X 'buildinfo.Commit=...'`（缺 module path）会静默失效。
3. **ldflags 值**含空格 / 冒号（ISO8601 时间戳含 `:`）时需要**单引号包裹**：`-X 'path.BuiltAt=2026-04-26T10:05:42Z'` —— 本 story AC8 的命令已经写了单引号，照抄即可。Windows cmd / PowerShell 下单引号处理不同，dev 若用 git-bash / MSYS 照跑。
4. **`go run`（非 `go build`）默认无 ldflags** —— 手工测"未注入"路径就直接 `go run`，不用强设空串。
5. **测试并发污染**：`buildinfo.Commit` / `BuiltAt` 是**全局包级变量**，多测试并发跑（`go test -parallel`）会互踩。**不要**在 test 里 `t.Parallel()`；本 story 的 3 个 handler 测试保持**串行**执行（默认行为），保存/恢复靠 `defer`。
6. **JSON tag 大小写**：V1接口设计 §2.4 约定**小驼峰** —— `builtAt` 不是 `built_at`、`buildTime`。与 `requestId` 对齐。
7. **`/version` 不要暴露 go version / 构建路径 / 主机名**等敏感信息：epics.md AC 只要 2 字段，严格遵守。Epic 36 上线前再决定是否增加 `/version/detail`。

### Project Structure Notes

- 新建 `internal/buildinfo/` 目录：`docs/宠物互动App_Go项目结构与模块职责设计.md` §4 没有明示这个包，但按**按需引入**原则（项目刚起步）可先建；后续 §4 更新时把它加进"基础设施层"清单
- `internal/buildinfo/` **不**放到 `internal/infra/` 下：`infra/` 语义是"外部设施接入（config / db / redis / logger / metrics）"；buildinfo 是**编译期常量**，与"外部设施"无关。与 Go 生态惯例（cobra / kubectl 的 `pkg/version/`）对齐
- 在 Completion Notes 记录这个路径决策，便于后续读 §4 的人能找到实际位置

### References

- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#4-项目目录建议] — 基础设施层结构（本 story 新增 `internal/buildinfo/`）
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#8.1-路由规范] — 运维端点与业务端点分离原则（`/ping` / `/version` / `/metrics` 不走 `/api/v1` 前缀）
- [Source: docs/宠物互动App_V1接口设计.md#2.4-通用响应结构] — `{code, message, data, requestId}` 四字段 envelope（本 story 遵守）
- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.4] — 本 story 原始 AC（ldflags 注入 + 3 个单测 case + 集成 case）
- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.7] — 下游 `scripts/build.sh` 重做会正式化 ldflags 注入命令
- [Source: _bmad-output/planning-artifacts/epics.md#Story-2.5] — iOS 端 `/version` 调用（本 story 的 `VersionResponse` 形状会被 iOS Codable 解析）
- [Source: _bmad-output/planning-artifacts/epics.md#Story-3.2-节点-1-Demo-验收] — 验证步骤 4 `curl http://localhost:8080/version` 返回正确 commit
- [Source: _bmad-output/implementation-artifacts/1-2-cmd-server-入口-配置加载-gin-ping.md] — Story 1.2 的 handler 风格模板（`PingHandler`）
- [Source: _bmad-output/implementation-artifacts/1-3-中间件-request_id-recover-logging.md] — Story 1.3 router.go Future 注释位置 + `requestIDFromCtx` 优先级链
- [Source: docs/lessons/2026-04-24-config-path-and-bind-banner.md#Meta] — "声明 vs 现实"原则（编译期注入是事实，handler 不重算）
- [Source: CLAUDE.md#Build-Test] — build 脚本现状（Story 1.7 重做；本 story 用原生 `go build -ldflags` 命令行验收）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- 本地 HTTP 端口 8080 已被其他进程占用；为完成 AC8 / AC10 的 curl 验证，临时新建 `server/configs/_tmp_story14.yaml`（`http_port: 18082`）→ 验证完毕后删除，未纳入 File List
- 首次 `go build -ldflags` 命令按 AC8 模板执行，输出如下：
  - `COMMIT=b3aad1b BUILT_AT=2026-04-24T07:48:59Z`（`date -u +%Y-%m-%dT%H:%M:%SZ` UTC）

### Completion Notes List

**实现摘要**
- 新增 `internal/buildinfo/` 包（方案 B），`Commit` / `BuiltAt` 两个 `var` 包级变量，默认 `"unknown"`
- 新增 `internal/app/http/handler/version_handler.go`：`VersionHandler` 直接读 `buildinfo` 包变量，经 `response.Success` 发统一 envelope；`message="ok"`、`data` 字段小驼峰 `builtAt`
- 扩展 `internal/app/bootstrap/router.go`：在 `/ping` 与 `/metrics` 之间挂 `r.GET("/version", handler.VersionHandler)`；同步修订顶部 Future 注释（移除 Story 1.4 占位、列出 `/version` 的语义）
- 新增 2 个测试文件：handler 层 3 个 case（happy / default-unknown / envelope-shape）+ router 层 1 个集成 case（完整三件套 + UUIDv4 requestId）

**AC3 偏离记录**
- epics.md 原 AC 字面是 `-X main.commit=...`；本 story 按故事文件 AC3 决策使用**方案 B** `-X github.com/huing/cat/server/internal/buildinfo.Commit=...`
- 偏离理由：main 包变量无法被 handler 直接读取，否则要把值沿 `NewRouter()` 签名传进去，污染 bootstrap API；buildinfo 独立包是 Go 生态事实标准（cobra / kubectl / docker）
- 不违反 epics 精神 —— epics 本意是"ldflags 注入 commit / builtAt"，路径细节属实现裁量权

**AC8 端到端 curl 验证**

**步骤 1：未注入（`go build` 不带 ldflags）**
```
$ ../build/catserver.exe -config configs/_tmp_story14.yaml &
$ curl http://127.0.0.1:18082/version
{"code":0,"message":"ok","data":{"commit":"unknown","builtAt":"unknown"},"requestId":"34c4bc68-6051-4df4-910c-a78dd33d0366"}
```

**步骤 2：注入 ldflags**
```
$ go build \
  -ldflags "-X 'github.com/huing/cat/server/internal/buildinfo.Commit=b3aad1b' \
            -X 'github.com/huing/cat/server/internal/buildinfo.BuiltAt=2026-04-24T07:48:59Z'" \
  -o ../build/catserver.exe ./cmd/server/
$ ../build/catserver.exe -config configs/_tmp_story14.yaml &
$ curl http://127.0.0.1:18082/version
{"code":0,"message":"ok","data":{"commit":"b3aad1b","builtAt":"2026-04-24T07:48:59Z"},"requestId":"9fb3aa1f-bcf9-45a8-ae71-54f0424fdd1f"}
```

**AC10 quality gate**
- `go vet ./...` ✅ 无 error
- `go test ./...` ✅ 全绿（bootstrap / handler / middleware / config / logger / metrics 全部 pass）
- `go build` ✅ 不带 ldflags 成功

**TODO（留给后续 story 落地，非本 story scope）**
- Story 1.7：`scripts/build.sh` 重做，正式化 `go build -ldflags -X ...` 命令（本 story 仅手工命令行验收）
- Story 3.3 节点 1 Demo 收官：把 `/ping` / `/version` / `/metrics` 三个运维端点补进 `docs/宠物互动App_V1接口设计.md`（或确认不补）

**Review response — [P1] Wire /version to actual build metadata before exposing it**

Review finding：reviewer 指出 `go run ./cmd/server` / `.claude/settings.local.json` / `scripts/build.sh` 三条 build path 都没注入 ldflags → `/version` 在"正常环境"下永远返回 `"unknown"`。要求把这三条里至少一条打通再发。

**裁定：defer 到 Story 1.7，不在本 story 修 `scripts/build.sh`。** 理由：

1. `.claude/settings.local.json` 是 Claude Code 的 **permission allowlist**，里面的 `"Bash(go build:*)"` 等条目是*权限规则*，不是实际 build 命令 —— 该仓库没有"从 settings.local.json 发起的 build 命令"。此条论据基于误读，不成立。
2. `scripts/build.sh` 现状**整体已坏**（不是"能跑但缺 ldflags"）：
   - L59 `go build ... ./cmd/cat/` —— `./cmd/cat/` 路径不存在（新架构入口是 `./cmd/server/`）
   - L43 引用 `scripts/check_time_now.sh` —— 旧架构 M9 time check
   - L47-52 引用 `docs/api/openapi.yaml` —— 旧架构残留
   - CLAUDE.md §Build & Test 已把"`scripts/build.sh` 重做"作为 TODO 写进项目 context；sprint-status `1-7-重做-scripts-build-sh: backlog` 是**专门**承接这件事的 story
3. `go run ./cmd/server` 返回 `"unknown"` 是 Go 工具链固有行为（`go run` 跳过 build 产物，`-ldflags -X` 无从注入）。本 story 的 AC5 / AC6 case#2 / Dev Notes §常见陷阱 #4 把"`go run` → `unknown`"列为**预期行为**而非 bug —— 它就是测试用例 #2 的构造场景。
4. 在 `build.sh` 已坏的前提下单独给它加 ldflags 一行是"脏修"：脚本仍跑不动，但片段被后人复制时会误以为已正式化。完整修（改入口路径 / 移除 M9 time check / 评估 openapi 断言）=  Story 1.7 的 scope。本 story AC8 / §范围红线已显式把 build.sh 重做划给 Story 1.7，是**设计决策**而非遗漏。

**风险登记**：
- Story 1.4 → Story 1.7 之间（中间还有 1.5 / 1.6），唯一能看到真实 commit / builtAt 的方式是手工复制 AC8 里的 ldflags 命令。对 dev demo / 联调**不影响**（ldflags 命令已在本 story AC8 写清楚）
- 如果 Story 1.7 被无限期 delay（sprint 切走、优先级降），需要在 Story 3.2（节点 1 Demo 验收）前拉回来；本条 review 已作为锁定证据归档在这里

### File List

**新增**
- `server/internal/buildinfo/buildinfo.go`
- `server/internal/app/http/handler/version_handler.go`
- `server/internal/app/http/handler/version_handler_test.go`
- `server/internal/app/bootstrap/router_version_test.go`

**修改**
- `server/internal/app/bootstrap/router.go`（挂载 `/version` 路由 + Future 注释同步）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（story 1-4 状态流转 + last_updated）
- `_bmad-output/implementation-artifacts/1-4-version-接口.md`（本 story 文件：Tasks 勾选 / Dev Agent Record 填充 / Status 流转）

## Change Log

| 日期 | 版本 | 描述 | 作者 |
|---|---|---|---|
| 2026-04-24 | 0.1 | 初稿（ready-for-dev） | SM |
| 2026-04-24 | 1.0 | 实装完成；`/version` 端点上线（ldflags 注入 commit/builtAt）；状态流转 review | Dev |
| 2026-04-24 | 1.1 | 回应 review [P1]（wire /version to actual build metadata）：defer 到 Story 1.7；本文件 Completion Notes 已加 Review response 段落锁定证据 | Dev |
