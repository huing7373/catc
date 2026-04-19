# Review 反模式速查（Epic 0 蒸馏）

Epic 0 共 19 轮代码审查，本文把其中**可复现、未来仍会踩**的错误模式按问题域归类，供未来 Claude 在"写新代码 / 做 self-review / 做二轮 review"时 grep。

- **战略纪律**（双 gate / Empty Provider / fail-closed）见 `docs/backend-architecture-guide.md` §21 和 `_bmad-output/implementation-artifacts/epic-0-retro-2026-04-19.md`
- **原始每轮记录**见同目录 `code-review-log.md`
- **本文定位**：战术级反模式清单，每条都有具体的"症状 → 为什么错 → 修复模式 → 自检问句 → 回链 review log round"

---

## TL;DR 自检清单（写代码前快速过一遍）

1. **close(channel)** 之前：publisher 可能还在 select-send 吗？→ 用 done channel，send 侧永不关闭
2. **启 goroutine** 前：panic 会冒到哪里？→ 加 recover，或 worker 内部 catch 并计数
3. **shutdown 敏感的 I/O**：ctx 已 cancel 时这条 cleanup / XACK / 重试写入还要跑吗？→ `writeCtxFor(parent)` = Background+timeout，覆盖**所有**早退出路径
4. **引入全局常量集合**（error code / WS msg type / cron job / redis key prefix / Provider）：漏加一个会被什么 CI 测试抓到？→ 双 gate（单测遍历源码 + 启动期 `validate*Consistency` fail-fast），且常量与 registry 由**同一个构造函数**产生，从结构上消除遗漏
5. **引入配置字段**：老的 `local.toml`（不含该字段）还能启动吗？→ validate 前先 `applyDefaults` 填零值
6. **写 JWT / 签名**：issuer / exp / kid / alg（RS256 钉死，不是 `*RSA`）逐个显式校验了吗？
7. **debug/release mode gate**：两边都写测试了吗？`TestX_DebugMode` + `TestX_ReleaseMode` 缺一不可
8. **组合 redis key**：key 组件的值可能包含分隔符（冒号 / 斜杠）吗？→ length-prefix 编码 `len:v:len:v` 保证 injection
9. **rate limit**：是真滑动窗口（sorted set by timestamp）还是伪装成滑动的固定窗口（INCR+EXPIRE NX，边界处流量翻倍）？
10. **度量 / 比率**：godoc 语义（分子 / 分母的精确定义）有测试锁死吗？"首次成功 connect 之后才算重连"这种话如果只写在注释里，代码会漂。
11. **中间件顺序**：panic 发生时，你期望运行的中间件收尾逻辑（access log / request id / metrics）在外层还是内层？→ Logger 必须在 Recover **外层**，panic 才走得到 access log

---

## 1. 并发安全（Concurrency）

### 1.1 close(channel) 时 publisher 可能仍在 select-send
- **症状**：正常断连路径 `close(c.send)` 触发 `send on closed channel` panic，整进程崩溃
- **为什么错**：`atomic.Bool.Load() == false` 之后、`send` 之前，另一 goroutine 已执行 `close(send)`。**"原子检查 + send"不是原子操作**
- **修复**：`send` 永不关闭；引入 `done channel`，trySend 用 `select { case <-done: / case send <- msg: / default: }` 三路复用，退出纯靠 `close(done)` 驱动
- **自检**：「如果我把 `close(send)` 和 `send <- msg` 放进两个 goroutine，代码是否仍安全？」
- **回链**：0-9 r1 #1 / 0-9 r2 #1

### 1.2 Goroutine panic 无 recover
- **症状**：cron job / WS handler panic 冒到 worker，拖垮整个 server
- **修复**：
  - cron：`cron.New(cron.WithChain(cron.Recover(logger)))`
  - WS handler / dispatcher：每个 handler 入口 `defer func() { if r := recover(); ... }`，并上报计数
- **自检**：「这个 goroutine panic 了，谁来兜底？log 会不会打出来？」
- **回链**：0-8 r1 #1

### 1.3 并行测试修改全局状态
- **症状**：`gin.SetMode`、`os.Setenv` 在各自 `t.Run` 里调 → flaky
- **修复**：全局状态统一在 `TestMain` 设置；或改用 `sync.Once` 锁死
- **回链**：0-6 r1 #4

### 1.4 返回指针暴露全局 sentinel 可变指针
- **症状**：`RegisteredCodes() map[string]*AppError`，调用方修改返回值污染全局
- **修复**：返回**值拷贝** `map[string]AppError`，或冻结构造函数（不可变封装）
- **自检**：「这个返回值的字段如果被改，影响的是谁？」
- **回链**：0-6 r3 #1

### 1.5 公开构造函数允许绕过不变量
- **症状**：`NewAppError(code, msg, category)` 不校验 category，包外可构造无效 category
- **修复**：构造函数内 panic / error 校验；或把构造函数设为包私有，外部只能用 `register()` 注册
- **回链**：0-6 r3 #2

---

## 2. Context 生命周期（Shutdown / Cancellation）

### 2.1 cron job 用 context.Background()
- **症状**：shutdown signal 到达后 `cron.Stop()` 等运行中的 job，job 在 Redis I/O 上阻塞不响应取消，拖过 30s 关机上限
- **修复**：Scheduler.Start(ctx) 接收 App 传入的 ctx；`addLockedJob` 把该 ctx 传给 `WithLock` 与 job body
- **自检**：「shutdown 信号到了以后，这个 goroutine 多久能返回？」
- **回链**：0-8 r1 #2 / 0-8 r2 #1

### 2.2 事务 EndSession 复用已取消的 ctx
- **症状**：`WithTx` 中 `defer session.EndSession(ctx)`，但 ctx 就是事务取消路径上被 cancel 的那个
- **修复**：cleanup 用 `context.Background()` 或单独的 `withTimeout(Background, 2s)`
- **回链**：0-3 r2 #3

### 2.3 log.Fatal 跳过所有 Final 清理
- **症状**：`App.Run` 中 `log.Fatal(Start)` 直接退出，Mongo/Redis 连接泄漏
- **修复**：错误通过 channel 通知 → 逆序调用 `Final()` → 最后 `os.Exit(1)`
- **自检**：「这次 log.Fatal 之前，哪些资源还没 close？」
- **回链**：0-3 r3 #2

### 2.4 shutdown 期的 cleanup / ACK 写入要 detach
- **症状**：APNs worker 的 `sender.Send` 返回 `context.Canceled`，handle 直接 `return`，既未 XACK 也未重投 → 消息永远卡在 PEL，重启后不被重消费 → **消息丢失**
- **修复**：helper `writeCtxFor(parent)`：parent 未 Done 返回原 ctx + no-op cancel；parent 已 Done 返回 `context.Background()+2s`。**所有**早退出路径（decode err / route err / no-token / send err）都要用它做 XACK / XADD DLQ
- **测试锚**：新增 3 个单测（decode / route-err / no-token）在 `ctx` 已 cancel 前提下断言 `XPENDING.Count == 0`
- **自检**：「handler 的**每一条** return 路径，shutdown 时数据一致性还守得住吗？只有 happy path 打补丁不够」
- **回链**：0-13 r1 #1 / 0-13 r2 #1

### 2.5 健康检查依赖无超时
- **症状**：`healthz` 复用 request ctx（无 deadline），依赖卡住时探针一起卡，p95 不可保证
- **修复**：`ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)`
- **回链**：0-4 r1 #2

### 2.6 初始化期 Ping 无超时
- **症状**：`MustConnect` 里 `client.Ping(context.Background(), nil)`，Redis 地址不可达时 `initialize()` 无限挂起
- **修复**：Ping 包 `WithTimeout(ctx, 3s)`；timeout 即 fail-fast `log.Fatal`
- **回链**：0-3 r1 #5

### 2.7 ready 信号早于端口 bind
- **症状**：`go httpServer.Start()` 后立即 `onReady()`，readyz 返回 200 时端口尚未 `net.Listen`
- **修复**：HTTP server 改 "Listen → 成功后调用 onReady → Serve" 三段式；App 通过 `ReadySignaler` 接口等待
- **自检**：「readyz=200 从语义上保证了什么？」
- **回链**：0-4 r1 #1

---

## 3. 安全边界（JWT / 身份）

JWT 默认库对关键字段的处理"很宽松"，几乎每一项都需要**显式**拒绝。

### 3.1 Verify 未校验 issuer claim
- **症状**：接受其他服务 / 环境签发的同算法 token，多环境 token 边界被打穿
- **修复**：`jwt.WithIssuer(cfg.Issuer)` + `jwt.WithIssuedAt()`
- **回链**：0-3 r1 #1

### 3.2 `*SigningMethodRSA` 不等于 RS256
- **症状**：只做 `method, ok := token.Method.(*jwt.SigningMethodRSA)` 放行 RS384 / RS512
- **修复**：钉死 `method.Alg() == "RS256"`（NFR-SEC-2 明确 RS256 唯一）
- **回链**：0-3 r1 #2

### 3.3 active_kid / old_kid 允许为空
- **症状**：轮换配置错误时不 fail-fast，可能签发 / 接受**无 kid** 的 token
- **修复**：config 校验 `active_kid != ""`；Verify 要求 `token.Header["kid"] != nil && != ""`
- **回链**：0-3 r1 #4

### 3.4 exp claim 默认非必填
- **症状**：jwt/v5 默认 exp 非必填，签发时漏写 exp 的 token 通过验签 → 永不过期
- **修复**：Verify 加 `jwt.WithExpirationRequired()`；Issue 侧也 panic-if-zero
- **回链**：0-3 r3 #1

### 3.5 Issue 静默覆盖调用方传入的标准 claim
- **症状**：`claims.RegisteredClaims = jwt.RegisteredClaims{ExpiresAt: ...}` 整体赋值，Subject / Audience / NotBefore 被丢弃
- **修复**：保留调用方传入的 RegisteredClaims，只 merge 必填字段
- **自检**：「标准 claim 不是我加的，会不会被我一行赋值抹掉？」
- **回链**：0-3 r1 #3

---

## 4. 配置 fail-fast 与默认值

### 4.1 非正值静默接受
- **症状**：`AccessExpirySec = 0` / 负数 → 签发已过期 token，故障延后到鉴权才暴露
- **修复**：validateConfig 里对所有"期待正整数"字段显式 `<=0 → log.Fatal`
- **回链**：0-3 r2 #2

### 4.2 新增必填字段没写 applyDefaults
- **症状**：新增 `[ws]` section 和 `mustValidate` 校验，但 `MustLoad` 只解析单文件；用户现有的 `local.toml`（不含 [ws]）启动就 `log.Fatal`，破坏"override 不用重复默认值"的既有契约
- **修复**：加载流程是 `defaultConfig() → merge file → applyDefaults(填零值) → validate`。validate 在 applyDefaults **之后**
- **自检**：「一个 clean override 配置（只写用户自己改的字段）还能启动吗？」
- **回链**：0-11 r1 #1

### 4.3 硬编码常量忽略配置字段
- **症状**：`tokenCleanupRetentionDays = 30` 写死，忽略 `cfg.APNs.TokenExpiryDays`（且该字段已过 `validateAPNs` 强校验）→ 合规窗口与配置脱节，"config 是装饰"
- **修复**：job 签名加 `retention time.Duration` 参数，Scheduler 从 config 传入；端到端测试 `TestX_CutoffNonDefaultRetention` 锚定
- **自检**：「改 config 里这个数值，会不会真的影响运行时行为？」
- **回链**：0-13 r1 #2

---

## 5. 注册表自证 / Registry Consistency

### 5.1 `allCodes` 切片自证正确
- **症状**：`init()` 基于 `allCodes` 生成 registry，测试"每个 allCodes 都在 registry" —— 漏加一个 sentinel 切片自己不知道就没测试能抓到
- **修复（结构性）**：删掉 `allCodes`；改 `register(code, ...) *AppError`：构造函数**同时**创建 sentinel 并注册到 registry。从结构上消除"两套维护"
- **自检**：「如果我漏加一个常量，会有测试失败吗？失败的是哪个测试？还是压根没人能发现？」
- **回链**：0-6 r1 #2 / 0-6 r2 #2

### 5.2 文档一致性测试只校验名字
- **症状**：`error_codes.md` 的 Category 列填错，测试只比对 code 名称存在不存在，不比 Category / HTTPStatus / Message 逐行
- **修复**：测试用 markdown 解析，逐行 full-match `(code, category, http_status, message)`
- **回链**：0-6 r1 #3

### 5.3 抽样测试覆盖不全
- **症状**：`TestCategoryHTTPStatus_Mapping` 抽样 9/19 码，未抽到的码带错 HTTPStatus 过 CI
- **修复**：遍历 `RegisteredCodes()` 全量校验
- **自检**：「我写的 case 覆盖率是多少？剩下的谁来保证？」
- **回链**：0-6 r2 #4

---

## 6. Spec / 实现漂移

### 6.1 Story 文档落后于实现
- **症状**：实现经过多轮迭代后文档没同步，后续开发者按文档回改会重新引入已修掉的 bug（Logger 中间件顺序、logx 实现细节）
- **修复纪律**：每轮 review 发现"实现已正确但文档描述错"的情况，类别标 `bad_spec`，commit message `fix(review)(docs):...`。**修文档和修代码同等重要**
- **自检**：「这次 review round 只是改代码了吗？story 文档 Dev Notes / File List / Completion Notes 里还在描述旧版本吗？」
- **回链**：0-5 r2 / 0-5 r3 / 0-3 r1 #6

### 6.2 接口签名在 AC / task / Dev Notes 三处不一致
- **症状**：`WithTx` 回调签名在 AC 写 `mongo.SessionContext`、Dev Notes 写 `context.Context`
- **修复**：AC 是唯一真相；在 AC draft 阶段做 AC review（见架构指南 §21.4）
- **回链**：0-3 r1 #6

---

## 7. Release 与 Debug 模式

### 7.1 handler 无条件注册在 Empty Provider 之上
- **症状**：release 模式下 `dispatcher.Register("session.resume", handler)` 照常执行；handler 返回的 6 字段全部来自 Empty Provider（`user=null / friends=[] / ...`），与"合法新注册无好友账号"不可区分；客户端 UI 长期显示空帐号视图
- **修复**：注册 gate 到 debug；release 明确不注册 + `log.Info("session.resume handler skipped: not yet wired in release")`。真实 Provider 上线时（Story 1.1 起）逐个放开
- **自检**：「这个 handler 在 release 模式下，返回的数据是真的还是 placeholder？客户端能分辨吗？」
- **回链**：0-12 r1 #2

### 7.2 mode gate 的条件分支写反
- **症状**：`if mode != "debug"` 把**不变式守门**整块跳过（retro §4 P1 finding）
- **修复**：双模式都要测；`TestX_DebugMode` + `TestX_ReleaseMode` 缺一不可
- **自检**：「这段守门在 release 下真的跑了吗？我有对应的单测吗？」
- **回链**：retro §4 / 0-14 r1

---

## 8. Redis Key 命名空间与 Injectivity

### 8.1 dedup key 缺 namespace
- **症状**：直接用 `env.ID` 作 key，不同用户或不同 RPC 在 5min TTL 内复用相同客户端生成 ID（如 "1"、"2"）会互相 `EVENT_PROCESSING` / 重放对方响应
- **修复**：scope 成 `(userId, msgType, eventID)` 元组
- **回链**：0-10 r1 #1

### 8.2 冒号拼接不是 injective
- **症状**：`"userId:msgType:eventId"` 简单拼接，字段值本身含冒号（debug validator 把 bearer token 原样当 userID；`Envelope.Type` 无格式校验）时 `("a:b","c","d")` 和 `("a","b:c","d")` 都 → `"a:b:c:d"`，跨三元组仍可能碰撞
- **修复**：length-prefix 编码 `"3:abc:3:def:1:x"`；或 sha256 hash + 原始字段存 log
- **自检**：「这个 key 的组件值里能不能出现分隔符？能的话两个不同三元组会不会映射到同一个 key？」
- **回链**：0-10 r2 #1

---

## 9. 速率限制与时间精度

### 9.1 INCR+EXPIRE-NX 是固定窗口
- **症状**：声称"滑动窗口 60s ≤ 5"；TTL 仅首次设置 → 客户端可以在窗口关闭前 5 次 + 窗口 reset 后立即 5 次 = 短时 10 次，绕过 NFR-SCALE-5
- **修复**：sorted set `ZADD ts` + `ZREMRANGEBYSCORE -inf (now-window)` + `ZCARD`；构造期注入 `clockx.Clock` 以便 FakeClock 驱动测试
- **自检**：「窗口边界处瞬间流量能不能翻倍？」
- **回链**：0-11 r1 #2

### 9.2 Unix nanoseconds 作 float64 score 损精度
- **症状**：ZSET score 为 float64，1.7e18 级纳秒时间戳超过 2^53 的精确表示范围，round-trip 损失百纳秒级精度 → `d == 0` 边界出现伪正 / 伪负，测试伪通过
- **修复**：换 Unix milliseconds（1.7e12 < 2^53，精确表示）
- **自检**：「我存到 float64 的整数是否在 2^53 以内？round-trip 能保证无损吗？」
- **回链**：0-11 r2 #2

### 9.3 `(cutoff` 保留边界项
- **症状**：`ZRemRangeByScore ... "("+cutoff` 保留 score == cutoff 的最老项；blocked 分支 `d > 0 && d <= window` 守卫在 `d == 0` 时 retry 回退到整个 window
- **修复**：`d <= 0 → retry = 1ms`（ceilSeconds 向上取 1s）作防御
- **回链**：0-11 r2 #1

---

## 10. 中间件与日志链路

### 10.1 Logger 必须在 Recover 外层
- **症状**：顺序 Recover → Logger，panic 时 Recover 写 500 后结束，Logger 后半段（access log）不执行；客户端视角有错但没日志
- **修复**：Logger → Recover → RequestID 顺序（Logger 最外层），access log 永远跑得到
- **自检**：「panic 发生时，我希望哪些中间件的 defer 收尾逻辑仍然执行？它们都在 Recover 外层吗？」
- **回链**：0-5 r1 #3

### 10.2 Recover 不检查 Writer.Written
- **症状**：handler 已 `c.Writer.Write(...)` 后 panic，Recover 强写 500 JSON → 客户端收到混合响应
- **修复**：`if !c.Writer.Written() { c.JSON(500, ...) }` else 关闭连接
- **回链**：0-5 r1 #4

### 10.3 `zerolog.Ctx(nil)` 返回 disabled logger
- **症状**：`logx.WithRequestID(ctx, id)` → `zerolog.Ctx(ctx)` 如果 ctx 未曾注入 logger，返回 **disabled logger**；后续 `.Str("requestId", id).Logger()` 的所有字段注入**静默失效**
- **修复**：`logx.Ctx(ctx)` 判断 `zerolog.Ctx(ctx)` 是否 disabled，回退到全局 logger；`WithRequestID` 也应基于 `logx.Ctx` 而不是裸 `zerolog.Ctx`
- **自检**：「context 里没 logger 的情况，我这行字段注入还会出现在输出里吗？」
- **回链**：0-5 r1 #1 / #2

---

## 11. 消息队列 / APNs / 持久性

（见 §2.4 的 writeCtxFor + §8 的 key namespace。此外：）

### 11.1 worker 只读 `">"`，从不回收 PEL
- **症状**：`XREADGROUP ... >` 只读新消息；PEL 中未 ACK 的消息重启后 orphan
- **修复**：重启时先扫 PEL（`XPENDING` + `XCLAIM`），再进入正常消费循环；或确保**所有** handler 退出路径都会 ACK（见 §2.4）
- **回链**：0-13 r1 #1 背景

---

## 12. 请求合并 / Cache Stampede

### 12.1 cache miss fan-out
- **症状**：`session.resume` cache miss 直接 fan-out 6 个 provider 调用；J4 Watch 重连风暴 N 个请求并发、Put 未完成 → N × 6 上游读
- **修复**：`golang.org/x/sync/singleflight.Group`，key = userID；winner 内部**再读一次 cache** 防止重复写入
- **自检**：「同一 key 同时 N 个请求进来，上游被打几次？」
- **回链**：0-12 r1 #1

---

## 13. 分层违规

### 13.1 pkg/ 引用 internal/
- **症状**：`pkg/mongox/client.go` 直接 `import "internal/config"`
- **修复**：`pkg/*` 定义 `Options struct`；`internal/initialize.go` 从 `cfg` 转成 `Options` 传入
- **自检**：「我这个 pkg 文件的 import 列表里有 `internal/` 吗？」
- **回链**：0-3 r2 #1

---

## 14. 度量 / 比率语义（retro P1 findings）

### 14.1 godoc 语义靠测试锁死
- **症状**：`workerLongLived` godoc 说"after the first successful connect"才算重连，代码是"外层循环第 2 次一律 `reconnects.Add(1)`"；启动期 dial 失败被误记为重连，数值虚高
- **修复**：godoc 声明的每一条语义都要有**对应的单元测试**锁死；尤其是"首次"、"成功之后"、"每 N 次" 这类条件
- **自检**：「godoc 里这句话如果被代码违反，哪个测试会失败？」
- **回链**：retro §4 / 0-15 r1

### 14.2 ratio 必须写清分子分母
- **症状**：`reconnectRatio = reconnects / successfulDials`；分母 = 所有成功 dial（含每次重连），语义本该是"% 会话被迫重连"；相对阈值（> 5%）严重依赖分母选择
- **修复**：metric 文档必须写 **precise numerator + precise denominator**；绝对计数优于 ratio；ratio 必配阈值 / 抽样基数
- **自检**：「这个 ratio 的分子和分母我各能用一句准确的业务话描述吗？」
- **回链**：retro §4 / 0-15 r2

---

## 附：类别标签速查

`code-review-log.md` 每条 finding 的"类别"列含义：

| 标签 | 含义 |
|---|---|
| `patch` | 代码问题，打补丁修复 |
| `bad_spec` | 文档与实现不一致，修文档 |
| `intent_gap→patch` | AC 意图与实现不符，修代码补齐语义（如 retry_after → Retry-After header） |

`git log --grep="fix(review)"` 能列出所有 review fix commit，`git show <hash>` 看完整 diff 和 commit message。

---

## 使用方式

**写新代码时**：先扫 TL;DR 自检清单；涉及具体域（并发 / context / 注册表 / config / metric）时看对应章节。

**做 self-review 时**：对着章节逐条问"我这次改动踩了吗？"

**做二轮 review 时**：结合 `bmad-review-adversarial-general` / `bmad-review-edge-case-hunter` skill；本文给"已知会踩的坑"，adversarial / edge-case 给"可能会踩的新坑"。

---

## 更新节奏（**不是**每轮 review 都更新）

**每轮 review（`/review-log` skill）**：只追加原始记录到 `code-review-log.md`，**不动本文**。蒸馏是判断活——自动蒸馏会出垃圾条目污染清单。

**每个 epic retro**：SM 会把该 epic 全部 review round 的 finding 统一蒸馏到本文。蒸馏流程（见 `bmad-retrospective/workflow.md` Step 11）：

1. 读 `code-review-log.md` 中属于本 epic 的所有 round
2. 对每个 finding 分类：
   - **已有同类**（模式和修复套路都对得上）→ 把 round 号加到对应条目的 **回链** 行末尾
   - **已有域的新变种**（落在现有章节下，但模式本身未被记录）→ 在对应章节新增一个子条目
   - **新问题域**（不落在任何现有章节下）→ 新增章节；必要时在 TL;DR 自检清单补一条
   - **一次性 / 不可复用**（配合特定 AC 的 bad_spec / intent_gap，无普适规律）→ 跳过
3. 先输出 diff 预览（分类 + 拟改动）给用户审；确认后写入
4. commit message `docs(review-antipatterns): distill epic X`，与 retro 文档同提交或单独提交

**为什么延后到 epic 边界**：蒸馏需要看**量**才能判断"这是一次性还是系统性"。单轮 finding 看不出来；一个 epic 20+ round 的数据才能识别重复模式。早蒸馏 = 误报多 + 用户判断反复。
