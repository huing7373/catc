# Story 0.14: WS 消息类型注册表与版本查询 — 实现总结

给服务端所有 WebSocket 消息类型建一个**单一真相源**：`internal/dto/ws_messages.go` 的 `WSMessages` 常量表，配套一个**无鉴权的 HTTP 查询端点** `GET /v1/platform/ws-registry`，让 Watch / iPhone 客户端在升级时能先问一句"服务器现在懂哪些 type、什么版本"，再决定是否向 WS 发请求。这是 architecture §Gap Analysis G2（"WS 消息注册表/版本没有持久化单一真相源，长期漂移必然发生"）的正面修复，也把 Story 0.7 Clock interface 第一次挂到真实业务代码上（epics line 581 指定本 story 负责验证 Clock 真的被业务用）。

**本 story 交付骨架**：`dto.WSMessages` + 3 条初始 entries（均 `DebugOnly: true`：`session.resume` / `debug.echo` / `debug.echo.dedup`）+ `Dispatcher.RegisteredTypes()` 枚举 API + `handler.PlatformHandler.WSRegistry` endpoint + **双 gate 漂移守门**（dto 单元测试 + initialize fail-fast）+ OpenAPI 3.0.3 占位 `docs/api/openapi.yaml` + 人类可读 `docs/api/ws-message-registry.md` + 架构指南 §12.1 Message Registry 段落。未来每加一条 WS 消息都要**四步同改**（常量 → dispatcher.Register → md → 跑构建），漏任何一步 CI 挡下，这是设计目的。

## 做了什么

### `internal/dto/ws_messages.go` — 常量真相源（AC1 / AC10）

- `WSDirection` 类型 + 三个枚举值（`up` / `down` / `bi`），描述消息方向
- `WSMessageMeta` struct，每个 WS 消息一条元数据：
  - `Type` —— canonical envelope.type（P3 `domain.action` 点分，e.g. `session.resume`）
  - `Version` —— MVP 统一 `"v1"`；AC10 未来破坏性变更时 `"v2"` 与 `"v1"` 共存 30 天过渡
  - `Direction` —— `up` / `down` / `bi`
  - `RequiresAuth` —— 今日 3 条都是 true；未来可能有无鉴权 `ping` 之类
  - `RequiresDedup` —— true ⇔ dispatcher 用 `RegisterDedup`（NFR-SEC-9 idempotency）
  - `DebugOnly` —— true ⇔ 只在 `cfg.Server.Mode == "debug"` 时注册
  - `Description` —— 一行英文摘要，给 `ws-message-registry.md` 人类文档消费（AC7）
- `WSMessages []WSMessageMeta` 全局 var，本 story 首批 3 条 entries（**全部 `DebugOnly: true`**）：
  - `session.resume` —— client 请求全量会话快照，Story 0.12 的 60s 缓存。**`DebugOnly: true` 是刻意的**——release 模式下 6 个 Provider 全 Empty，返回 `{user:null, friends:[], ...}` 跟"新账号"无法区分，会腐蚀客户端 UI；Story 1.1 上线第一个真实 Provider 时同步去 flag
  - `debug.echo` —— 调试用，原样回显 payload
  - `debug.echo.dedup` —— 调试用，走 dedup 中间件，验证幂等重放路径
- `WSMessagesByType map[string]WSMessageMeta` —— 包 init 时一次构造，O(1) lookup；**init 中遇到重复 Type 直接 panic**（belt-and-braces，fail-fast）。godoc 标注 "MVP invariant: Type 唯一，未来 AC10 v1/v2 共存时需要 rekey on Type+Version"

### `Dispatcher.RegisteredTypes()` —— 枚举 API（AC2）

- `internal/ws/dispatcher.go` 加 `func (d *Dispatcher) RegisteredTypes() []string`：读 `d.types` map → copy keys → `sort.Strings` → 返回 fresh slice（调用方可随便改，不影响内部）
- 为什么读 `d.types` 不读 `d.handlers`：两个 map 的 key 集一样，但 `d.handlers` 里 dedup-wrapped 的 entry 是中间件 wrap 过的值，`d.types` 是 Register / RegisterDedup **写之前 check 用**的 clean set
- godoc 明确并发前提："**only call after initialize() returns, i.e. before Hub.Start**"——RegisteredTypes 不上锁；dispatcher registration 只在 initialize.go 发生，早于任何 readPump goroutine。Hub.Start 后调用是 undefined
- `internal/ws/dispatcher_test.go` 追加 `TestDispatcher_RegisteredTypes`（3 子表：空 / 单 Register / Register+RegisterDedup 排序），外加反向断言 "返回 slice 的 mutation 不影响 dispatcher 内部"

### `internal/handler/platform_handler.go` —— HTTP endpoint（AC3 / AC5 / AC6 / AC11）

- `PlatformHandler` 结构体持两字段：`clock clockx.Clock` + `serverMode string`；构造函数 `NewPlatformHandler(clock, serverMode)` 对 nil clock panic
- `WSRegistryResponse` / `WSRegistryMessage` struct 定义**在 handler 包**（不是 dto 包，M8 明确 HTTP 响应结构属于 handler 边界；dto 包专职 WS envelope domain 和 AppError 注册表）
- JSON tag 全部 camelCase（iOS `JSONDecoder` 约定）：`apiVersion / serverTime / messages / type / version / direction / requiresAuth / requiresDedup`
- **`DebugOnly` 不暴露在 wire 上**—— release 模式过滤掉 `DebugOnly: true` 的 entry，debug 模式也不输出该字段（client 看到 `debugOnly` 会对"升级协商"产生混淆）
- `WSRegistry(c *gin.Context)` 实现 ~10 行：
  1. `msgs := make([]WSRegistryMessage, 0, len(dto.WSMessages))` —— 预分配 + **nil-slice-safe**（空 slice JSON 编码为 `[]` 不是 `null`，iOS `Array.decode` 在 `null` 时崩）
  2. iterate `dto.WSMessages`，skip `meta.DebugOnly && serverMode != "debug"`
  3. `c.JSON(200, WSRegistryResponse{APIVersion: "v1", ServerTime: h.clock.Now().UTC().Format(time.RFC3339), Messages: msgs})`
- **handler 不额外打 INFO 日志** —— middleware.Logger 已经记访问日志；handler 层再打一条 double 噪音。P5 纪律

### 路由装配 + fail-fast（`cmd/cat/wire.go` + `initialize.go`，AC5 / AC6 / AC15）

- `wire.go` `handlers` struct 追加 `platform *handler.PlatformHandler`；`buildRouter` 在 `/healthz /readyz` 后、`/ws` 前插入 `r.GET("/v1/platform/ws-registry", h.platform.WSRegistry)`，紧邻一行注释指向 architecture line 814 "bootstrap endpoints 不挂 JWT"——提醒未来加 JWT middleware 的 PR 不要反射性地把它一并包进 `/v1/*` group
- `initialize.go` 在 `upgradeHandler` 构造之后、`handlers` struct 赋值之前调用 `validateRegistryConsistency(dispatcher, cfg.Server.Mode)`，漂移则 `log.Fatal`：
  - 不变式 1：每个 dispatcher 注册项必须有 `dto.WSMessages` 对应条目（防"代码注册了陌生 type"）
  - 不变式 2：**debug 模式下每条 `dto.WSMessages` 条目都必须已注册**（防"代码漏注册但 endpoint 还在广告"——review round 1 扩展）
  - 不变式 3：release 模式下每条非 `DebugOnly` 条目必须注册，且**任何 `DebugOnly` 条目都不得注册**（防 Story 0.12 回归——`session.resume` 泄漏到 release）
- 错误信息按桶分类输出，方便运维 triage：`ws registry drift: unknownRegistered=[...] missingInDebug=[...] missingInRelease=[...] debugOnlyInRelease=[...]`

### 双 gate 漂移守门（AC4 / AC15）

这是 G2 修复的核心结构，同一纪律上 double-gate（沿用 Story 0.6 error-codes 的模式，不要简化为单层）：

- **Gate 1 — 编译时 / 单元测试（CI blocking）**：`internal/dto/ws_messages_test.go`
  - **外部测试包 `package dto_test`** —— 本 story 对原规格的唯一偏离。story 文本写 `package dto`，但 `internal/ws` 已经 import `internal/dto`（为了 `dto.AppError`），同包测试 import `internal/ws` 会循环。外部测试包是 Go 标准规避手段，测试仍在同一目录同一文件名。在 Completion Notes 中已说明
  - `TestWSMessages_AllFieldsPopulated` —— 断言 `Type ≠ ""` / 符合 `^[a-z0-9]+(?:\.[a-z0-9]+)*$` / `Version` 符合 `^v\d+$` / `Direction ∈ {up, down, bi}` / `Description ≠ ""`
  - `TestWSMessages_NoDuplicates` —— 显式断言无重复 `Type`
  - `TestWSMessagesByType_DuplicateTypePanics` —— 独立复刻 init 的 build 逻辑验证 panic
  - `TestWSMessages_ConsistencyWithDispatcher_DebugMode` —— 手工镜像 `initialize.go` debug 分支（Register debug.echo / RegisterDedup debug.echo.dedup / Register session.resume），`RegisteredTypes()` 必须 ElementsMatch 全部 `WSMessages` 的 `Type` 列
  - `TestWSMessages_ConsistencyWithDispatcher_ReleaseMode` —— 空 dispatcher，`RegisteredTypes()` 必须 ElementsMatch 所有非 `DebugOnly` 的 Type（今日为空 list；Story 1.1 起逐步长出来）
  - 消息头顶的 `noopHandler` + `fakeDedupStore`（本文件内 stub，满足 `ws.DedupStore` 三方法接口签名）

- **Gate 2 — 运行时 fail-fast（process-startup blocking）**：`validateRegistryConsistency` 已在上一节描述；`cmd/cat/initialize_test.go` 新建覆盖 5 个场景（DebugModeFullyRegistered / ReleaseModeNothingRegistered / UnknownRegisteredFails / DebugModeMissingRegistrationFails / DebugOnlyInReleaseFails）
- 为什么 CI 有了 Gate 1 还要 Gate 2：Gate 1 挡"代码审查 / PR 时的静态漂移"；Gate 2 挡"feature flag / 环境变量在某个部署上条件注册 handler"这种动态路径——CI 覆盖不到

### OpenAPI 3.0.3 占位（`docs/api/openapi.yaml`，AC8）

- 只正式化本 story 的 `/v1/platform/ws-registry` endpoint（`/healthz` / `/readyz` 是 infra 端点，不属于对外 API 契约，刻意省略；Story 1.x 落地 `/auth/apple /auth/refresh` 时再扩）
- `components.schemas` 下 `WSRegistryResponse` / `WSRegistryMessage` 字段严格 camelCase，每个都 `required`，`type` 带 P3 正则 pattern，`direction` 带 enum，字段描述对齐 `dto` godoc
- `info.version: "0.14.0-epic0"` —— spec 本身的版本，不是 API 的 `apiVersion`（两个语义分开：前者是 yaml 文件版本，后者是 `/v1/platform/ws-registry` 的响应 schema 版本，二者独立演进）

### CI 校验（AC9，**本 story 唯一对原规格的实质偏离**）

原 AC9 指定 `go run github.com/go-swagger/go-swagger/cmd/swagger@v0.31.0 validate docs/api/openapi.yaml` 作为 CI 校验手段。实测该 CLI 只支持 Swagger 2.0，不支持 OpenAPI 3.0.3：

```
.servers in body is a forbidden property
.components in body is a forbidden property
.openapi in body is a forbidden property
.swagger in body is required
```

这与 AC8 要求的 "OpenAPI 3.0.3" 直接冲突。两条路：
1. 把 openapi.yaml 降级为 Swagger 2.0 —— 违反 AC8
2. 换工具（kin-openapi / libopenapi）—— 需要新增依赖，workflow 规定新依赖要用户审批

采用第三条：**在 Go test lane 用 `gopkg.in/yaml.v3`（已是 `go.mod` indirect，无新增）做结构校验**。新建 `cmd/cat/openapi_spec_test.go`：

- `TestOpenAPISpec_StructurallyValid` —— 解析 yaml；断言 `openapi` 字段正则 `^3\.0\.\d+$`、`info.title / info.version` 非空、`paths` 含 `/v1/platform/ws-registry`、`components.schemas` 含 `WSRegistryResponse / WSRegistryMessage`
- `TestOpenAPISpec_SchemaFieldsMatchWireShape` —— 更严：逐字段断言 `WSRegistryResponse` 含 `apiVersion / serverTime / messages`（必须都在 `required` 里），`WSRegistryMessage` 含 `type / version / direction / requiresAuth / requiresDedup`。这道闸拦的正是 G2 的失败模式——spec 说 `api_version`，code 发 `apiVersion`，客户端静默拆解失败

CI 严重度等同（test failure ≡ build failure，`bash scripts/build.sh --test` 会挂）。`scripts/build.sh` 删掉最初写的 `validate_openapi` 函数，保留注释解释原因。**在 Dev Agent Record 里留 review 讨论点**——如果 reviewer 要求必须用外部 CLI，可以后续换 `kin-openapi` 或 `libopenapi` 做 round 2 调整。

### 人类可读注册表 + 架构指南（`docs/api/ws-message-registry.md` + guide §12.1，AC7 / AC13）

- `docs/api/ws-message-registry.md`：头部明确 "source of truth = `internal/dto/ws_messages.go`"，引用三个 CI 漂移守门测试（Gate 1 dto 单测 / Gate 2 initialize_test / integration test）；envelope shapes 三种（upstream request / downstream response / downstream push）；version strategy 段；3 条消息各一个 `### <type>` section
- "**新增消息四步走**" 流程写在文件头部（1. 加 WSMessages entry → 2. initialize.go Register → 3. 本文件加 section → 4. 跑 build），出错顺序会被 Gate 1 挡下——这是设计
- `docs/backend-architecture-guide.md` §12 下加 "§12.1 Message Registry" 段落：指向 dto 包是真相源、registry.md 是当前内容、双 gate 漂移守门、四步走流程。**故意不**把消息列表本身塞进架构指南（架构描述 *pattern*，registry.md 描述 *current contents*，避免 double-source）

## 怎么实现的

**为什么 3 条初始 entries 都是 `DebugOnly: true`**：`session.resume` 的原因已在 AC1 说明——Empty Provider 状态下 release 模式挂这个路由等于向客户端发错误信号。`debug.echo` / `debug.echo.dedup` 是纯调试用（Story 0.9 / 0.10 留下的骨架回显），release 模式留它没意义。**"release 模式今日零注册"是本 story 的数据特征而非 bug**——AC4 `TestWSMessages_ConsistencyWithDispatcher_ReleaseMode` 直接断言这一点；Story 1.1 上线第一个非 DebugOnly 消息时，该 case 的 `want` slice 自然长出第一个元素。

**为什么 `internal/dto/` 不是"本 story 首次创建"但 story 文档这么写**：story 文本早于 Story 0.6 error-codes 注册表的落地。Story 0.6 已经把 `internal/dto/` 建好（`error.go` + `error_codes.go`），本 story 在同一目录追加 `ws_messages.go`。架构 §Project Structure line 867 对该目录的定位是 "handler 层 DTO + 错误码注册表 + WS 消息常量"——三块 AppError / WSMessages / 未来 HTTP DTO 汇聚。本 story 填入第二块。

**为什么测试用外部 `package dto_test` 而不是 story 明写的 `package dto`**：story 作者没意识到的循环导入——`internal/ws/dispatcher.go` 已经 `import "internal/dto"`（为 `dto.AppError`）。在 `internal/dto/ws_messages_test.go` 写 `package dto` 然后 `import "internal/ws"` 会 Go 编译器直接拒绝："import cycle not allowed in test"。外部测试包 `package dto_test` 是 Go 标准的规避 pattern（测试文件、测试名、目录都不变），并不违反 story 的实质意图——测试仍然测 `dto` 包，也仍然能导入 `internal/ws`。相关正则 `wsTypeRegex / wsVersionRegex` 在 dto 包不导出，测试文件里本地重复一份即可（3 行代码）。

**为什么 `RegisteredTypes` 不上锁也能正确**：dispatcher 的 `Register` / `RegisterDedup` 只被 `cmd/cat/initialize.go` 在启动阶段（同步、单 goroutine）调用；`Hub.Start()` 在 `initialize()` 返回后才启动 readPump/writePump 多 goroutine，此时 types map 已 frozen。AC4 consistency test 也是在 Hub 未 start 的测试环境下调用，和生产场景一致。godoc 把这个前提 explicit 写出来，避免未来有人在 runtime 调用这个方法然后怪"为什么并发 panic"。

**为什么 `WSRegistry` 用 `h.clock.Now()` 而不是 `time.Now()`**：M9 禁业务代码直接 `time.Now()`；`clockx.RealClock.Now()` 已经 `.UTC()`（pkg/clockx/clock.go line 13），但 handler 里再显式 `.UTC()` 是防御性的（`FakeClock` 使用者可能传 local time）。epics line 581 明文指定 "Story 0.14 的 endpoint 时间戳使用 Clock（验证 Clock 真的被业务代码使用）"——这是 Story 0.7 Clock interface **第一次**通过业务路径被验证，`TestPlatformHandler_WSRegistry_ServerTimeUsesClock` / `TestWSRegistryEndpoint_ServerTimeUsesInjectedClock` 用 FakeClock 固定 `2030-01-02T03:04:05Z` 精确断言 response 的 `serverTime` 字段。

**为什么 `WSRegistry` 用 `make([]T, 0, N)` 而不是 `var msgs []T` 或 `new([]T)`**：nil slice 在 Go JSON encoding 里变成 `null`，非 nil 空 slice 变成 `[]`。Swift `JSONDecoder` 解码 `Array<Struct>` 遇到 `null` 抛 `typeMismatch`，遇到 `[]` 正常返回空数组。release 模式响应 `"messages": []` 是客户端解析前提；`make(..., 0, N)` 即使 for 循环一次没 append 也留下非 nil 空 slice。`TestPlatformHandler_WSRegistry_ReleaseMode` 和 `TestWSRegistryEndpoint_ReleaseMode` 都包含 `strings.Contains(body, \`"messages":[]\`)` 这一行断言作为契约锁死。

**为什么 `validateRegistryConsistency` 不叫 `validateWSRegistry` 或 `checkRegistry`**：启动自检 helper 按 architecture §19 "启动期校验" 惯例命名——`validateXxxConsistency` 表明"检查跨数据源的一致性"（这里是 `dto.WSMessages` vs `Dispatcher.types` 两个数据源）。未来若有 `validateCronJobsConsistency` / `validateRepositoriesConsistency` 同族命名一致。

**review round 1 修复的漂移场景为什么在 debug 模式也要挡**：review finding 指出——原版 `validateRegistryConsistency` 在 `mode != "debug"` 时才检查 missing 注册，debug 模式分支完全跳过。这样如果有人在 `initialize.go` debug 分支误删 `dispatcher.Register("session.resume", ...)`，启动会 pass，`WSMessages` 里 `session.resume` 还在，`/v1/platform/ws-registry` 仍广告 `session.resume` 给客户端，客户端发来却被 Dispatcher 返回 `UNKNOWN_MESSAGE_TYPE`——**这正是 G2 想拦的漂移、且是本 endpoint 唯一对外暴露漂移的场景**。修复：debug 模式同样要求每条 `WSMessages` entry 都已注册（DebugOnly 与否一样），新增 `missingInDebug` bucket；新增 `TestValidateRegistryConsistency_DebugModeMissingRegistrationFails` 锁死。

**为什么 "新增消息四步走" 的 CI 惩罚是故意的**：开发者中途停下或忘记任一步都会失败 Gate 1 或 Gate 2。例如只在 `dto.WSMessages` 加了 entry 没在 `initialize.go` Register → `TestWSMessages_ConsistencyWithDispatcher_DebugMode` ElementsMatch fail。只 Register 没加 entry → `validateRegistryConsistency` unknownRegistered fail + `TestValidateRegistryConsistency_UnknownRegisteredFails` fail。都做了但忘记改 registry.md → 当前 **AC7 故意没要求 CI 校验** md 与常量一致（epics 原话"checking the .md against the constants is a future enhancement"），靠 PR checklist item 保障。真写 codegen 把 md 变成 generated 是未来优化，这次不做。

## 怎么验证的

**单元测试（全部 `t.Parallel()`，无 Testcontainers 无 miniredis）**：

- `internal/dto/ws_messages_test.go` 6 用例（外部测试包）：
  - `TestWSMessages_AllFieldsPopulated` —— 3 子表（每条 entry 一格）断言字段格式
  - `TestWSMessages_NoDuplicates` —— 无重复 Type
  - `TestWSMessagesByType_DuplicateTypePanics` —— 复刻 init 逻辑验证 panic 分支
  - `TestWSMessages_ConsistencyWithDispatcher_DebugMode` —— 手工镜像 initialize debug 分支
  - `TestWSMessages_ConsistencyWithDispatcher_ReleaseMode` —— 空 dispatcher 对齐非 DebugOnly（今日为空）
- `internal/ws/dispatcher_test.go` 追加 `TestDispatcher_RegisteredTypes` 3 子表：
  - empty_dispatcher / single_register / register_and_register_dedup_sorted
  - 每子表还断言"mutate 返回 slice 不影响 dispatcher"
- `internal/handler/platform_handler_test.go` 4 用例：
  - `TestNewPlatformHandler_NilClockPanics`
  - `TestPlatformHandler_WSRegistry_DebugMode` —— FakeClock `2026-04-18T12:34:56Z` 注入，断言 3 条 messages 全在、RequiresDedup 布尔值对齐 dto 常量、**raw body 不含 "debugOnly"/"DebugOnly"**（wire leak 负断言）
  - `TestPlatformHandler_WSRegistry_ReleaseMode` —— 0 条 message，`strings.Contains(body, \`"messages":[]\`)` nil-safe 契约断言
  - `TestPlatformHandler_WSRegistry_ServerTimeUsesClock` —— FakeClock `2030-01-02T03:04:05Z` 注入断言 serverTime 精确匹配（Story 0.7 epics line 581 bind-to-business verification）
- `cmd/cat/initialize_test.go` 5 用例：
  - DebugModeFullyRegistered / ReleaseModeNothingRegistered / UnknownRegisteredFails（断言错误信息含 "ghost.type" + "unknownRegistered" 桶名）
  - **DebugModeMissingRegistrationFails**（review round 1 新增 —— registerred 只有 debug.echo / debug.echo.dedup，缺 session.resume，error 含 "session.resume" + "missingInDebug"）
  - DebugOnlyInReleaseFails（断言 error 含 "session.resume" + "debugOnlyInRelease"，锁死 Story 0.12 回归守门）
- `cmd/cat/ws_registry_test.go` 3 用例（无 `//go:build integration` tag，属默认 test lane）：
  - `TestWSRegistryEndpoint_ReleaseMode` —— 通过 `buildRouter(cfg, h)` 真实路由模拟生产装配；health/wsUpgrade handler 用 nil 依赖构造（路由注册期不会触发这些 handler 的方法，只会注册函数指针——Go 允许 nil 指针的 method value binding）；断言 release 模式 0 messages + `[]` 非 null
  - `TestWSRegistryEndpoint_DebugMode` —— 3 messages 全在，RequiresDedup 布尔对齐常量
  - `TestWSRegistryEndpoint_ServerTimeUsesInjectedClock` —— FakeClock 注入 round-trip
- `cmd/cat/openapi_spec_test.go` 2 用例（替代 go-swagger CLI 校验）：
  - `TestOpenAPISpec_StructurallyValid` —— openapi 字段 3.0.x 正则、info 必填、关键 path/schema 存在
  - `TestOpenAPISpec_SchemaFieldsMatchWireShape` —— yaml 字段名逐项 vs `json` tag camelCase 对齐

**构建验证**：

- `bash scripts/build.sh --test` —— 全绿（go vet + check_time_now + go build + 全量 go test）
- `go vet ./...` —— clean
- race 测试本地因 Windows CGO 环境问题无法跑，非代码问题；CI（Linux）覆盖

**对应 PR checklist §19 自审全部通过**：无 `fmt.Printf`/`log.Printf`；handler 不持有 mongo/redis client；无 `context.TODO()`/`context.Background()` 业务代码；所有公开成员有英文 godoc；对应 `*_test.go` 齐；`bash scripts/build.sh --test` 绿；唯一 `// TODO` 在 ws-message-registry.md 引用"未来 story"（AC14 显式允许）。

## 后续 story 怎么用

- **Story 1.1（Sign in with Apple）** —— 是第一个**不再 DebugOnly** 的消息的契机：
  1. `dto.WSMessages` 里 `session.resume` 条目移除 `DebugOnly: true`（改 false 或 `// DebugOnly: false` 显式默认）
  2. `cmd/cat/initialize.go` release 分支（今日 line 131-138 空地）开始 `dispatcher.Register("session.resume", sessionResumeHandler.Handle)`
  3. `dto.WSMessages_ConsistencyWithDispatcher_ReleaseMode` 的 `want` slice 自然长出 `["session.resume"]`（`TestWSMessages_ConsistencyWithDispatcher_ReleaseMode` 会 fail 提醒"别忘同步改这里"）
  4. `validateRegistryConsistency` 的 `missingInRelease` bucket 开始起作用 —— release 部署漏 register 会立刻 log.Fatal
- **Story 1.3（JWT middleware）** —— 首次引入 `/v1/*` 的 `r.Group("/v1", middleware.JWTAuth())` 时**必须**保留 `/v1/platform/ws-registry` 在 JWT group 之外（本 story wire.go 注释已写在路由这一行）。Story 1.3 的 dev 看到这行注释直接跳过，不需要猜
- **Story 0.15（Spike-OP1 watchOS WS 稳定性矩阵）** —— 本 story 是 0.15 的硬前置。0.15 的测试手册大概率要 curl 本 endpoint 先验证 "服务器承认哪些 type"，再根据返回构造 WS 测试矩阵
- **Story 2.x 起凡新增 WS 消息**（`state.tick` / `touch.send` / `friend.accept` / `blindbox.redeem` / `skin.equip` / ...）—— 四步走：
  1. `dto.WSMessages` append 一条 entry（填对 `Direction / RequiresAuth / RequiresDedup / DebugOnly`）
  2. `cmd/cat/initialize.go` `dispatcher.Register(...)` or `RegisterDedup(...)`
  3. `docs/api/ws-message-registry.md` 加 `### <type>` section
  4. `bash scripts/build.sh --test` 绿 → 提交。漏一步 CI 挡，这是纪律
- **Story 2.1（FSM 引入 state.tick）** —— 是 AC10 v1/v2 coexistence 的**第一个真实触发点**（epics 已提示）。那一刻 `WSMessagesByType` 要重构为 keyed on `Type+Version`——godoc 已标注这一未来动作（"MVP invariant: Type 唯一；AC10 v1/v2 共存时 rekey on Type+Version"），真做时只需一行 `m[meta.Type+"@"+meta.Version] = meta` + 更新所有 caller（WSRegistry handler 遍历时额外按 Type 聚合相同 Type 的多个版本）。`TestWSMessages_NoDuplicates` 同步改为"同 Type+Version 唯一"
- **Story 1.x / 2.x 首次扩 OpenAPI** —— 加第一个真实 HTTP endpoint（`POST /auth/apple` 等）时，`docs/api/openapi.yaml` 的 `info.version` 从 `0.14.0-epic0` 升到 `1.0.0`（或按 epic 拐点约定）；`cmd/cat/openapi_spec_test.go` 的 `TestOpenAPISpec_SchemaFieldsMatchWireShape` 模式可以复制给每个新 endpoint 的 DTO（一个表驱动测试覆盖所有 HTTP DTO 字段名 vs schema）
- **code-review round 2+** —— 如果 reviewer 对 AC9 的偏离不满意，最快的补救是换工具：`go run github.com/getkin/kin-openapi/cmd/openapi-validator@latest`（需确认该 CLI 是否存在，否则写一个 3 行的 Go main 调 `openapi3.T.Validate`），或 `npx @redocly/cli lint`。当前 Go test lane 的结构校验已把"spec 和 wire shape 的字段名对齐"这一 G2 核心意图 cover 掉了，更换工具更多是"CI 工具多样化"的美学差异
