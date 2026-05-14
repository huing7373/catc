---
date: 2026-05-15
source_review: codex review round 1 (epic-loop-review-20-8-r1.md) for Story 20-8 dev/grant-cosmetic-batch node-7 stub
story: 20-8-dev-端点-post-dev-grant-cosmetic-batch
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-15 — Stub endpoint 必须 explicit-failure 而非 silent false-positive（20-8 r1）

## 背景

Story 20.8 实装 `POST /dev/grant-cosmetic-batch` 节点 7 阶段骨架。按 epics.md §20.8 选项 C，user_cosmetic_items 表节点 8（Epic 23.2）才建 —— 本 story 阶段 service 不能真实写库。原 dev-story 落地的 stub 实装是：

```go
// service stub（旧）
func (s *devCosmeticServiceImpl) GrantCosmeticBatch(ctx, userID, rarity, count) error {
    slog.WarnContext(ctx, "STUB — user_cosmetic_items table NOT YET CREATED", ...)
    return nil    // ⚠️ silent false-positive
}
```

handler 拿到 `err = nil` → `response.Success(..., {"userId":1001,"rarity":1,"count":10}, "ok")` → HTTP 200 + envelope.code=0。

Codex review r1 指出：BUILD_DEV=true 下，任何合法 POST /dev/grant-cosmetic-batch 都返 200 + code=0，但实际**未写入** user_cosmetic_items 表（节点 7 阶段 stub）。e2e / demo 脚本基于"成功响应"继续走 → 后续 GET /cosmetics/inventory 拿到空仓库 → 才在远端发现 grant 没真正生效，调试链路长。

修复方向（用户在 fix-review 任务里钦定）：service 改为返 `apperror.ErrServiceBusy (1009)` + 文案明确"endpoint not yet active in node-7 phase" → middleware 自动翻 HTTP **503**，调用方在请求层立刻看到失败。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | stub endpoint silent false-positive | high (P1) | architecture / error-handling | fix | `server/internal/service/dev_cosmetic_service.go:90-98` + `dev_cosmetic_service_test.go` + `dev_cosmetic_handler_test.go` (HappyPath case) + story 20-8 文件 |

## Lesson 1: Stub endpoint 必须 explicit-failure 而非 silent false-positive

- **Severity**: high (P1)
- **Category**: architecture / error-handling
- **分诊**: fix
- **位置**: `server/internal/service/dev_cosmetic_service.go:90-98`（节点 7 阶段 stub 实装）

### 症状（Symptom）

dev / e2e 脚本调用 `POST /dev/grant-cosmetic-batch {userId:1001, rarity:1, count:10}` 时收到 HTTP 200 + envelope.code=0 + data ack `{userId:1001, rarity:1, count:10}` —— 但 user_cosmetic_items 表里**没有任何新行**（节点 7 阶段表都还没建）。脚本继续走"开箱 → 仓库 → 合成"链路，到 `GET /cosmetics/inventory` 拿到空仓库时才发现根因，调试链路从"grant 那行"被拉长到整条流水线。

### 根因（Root cause）

**"分阶段实装"时把 endpoint 立起来很容易，但默认让 stub 返 success 是反直觉的危险设计**：

1. **stub 实装写"return nil"是最直觉的占位**（甚至 service 单测会先长成"HappyPathStub_ReturnsNil"模式自我强化）；
2. handler 层的逻辑机械：`err == nil → response.Success(...)` —— 没有任何地方区分"成功 = 真写了库"还是"成功 = stub 占位"；
3. 上游脚本 / e2e / demo 看到的契约是"endpoint 成功 = side-effect 已生效"，而 stub 把这个契约**单边毁约**还不告知调用方；
4. WARN 日志只在 server 侧可见，**对调用方不可见**（API consumer 不读 server 日志）—— "WARN 日志已经标了 stub"的自我安慰救不了上游。

本质：**silent false-positive 比 explicit failure 危险得多** —— 后者让上游在最早的位置就 fail-fast，前者把根因调试链路拉长 N 步。

### 修复（Fix）

1. **service `GrantCosmeticBatch` stub 实装改为显式失败**（节点 7 阶段独有；节点 8 激活时替换回 happy path return nil）：
   ```go
   // service stub（新；fix-review r1 后）
   func (s *devCosmeticServiceImpl) GrantCosmeticBatch(ctx, userID, rarity, count) error {
       slog.WarnContext(ctx, "dev grant-cosmetic-batch called in node-7 stub phase, returns 503 by design ...", ...)
       return apperror.New(apperror.ErrServiceBusy,
           "dev/grant-cosmetic-batch not yet implemented (node-7 stub; awaits Story 23.5 to activate)")
   }
   ```
   middleware 看到 `*AppError{Code: 1009}` → 自动翻 HTTP 503 + envelope.code=1009 + envelope.message 透传 → 调用方在 503 状态码 + 1009 业务码 + "not yet implemented (node-7 stub; awaits Story 23.5)" 文案三层都看得见"endpoint 还没激活"。

2. **service 单测 3 case 全部改成断言 1009**（HappyPathStub_ReturnsServiceBusy_LogsWarn / BoundaryCases_AlwaysReturnsServiceBusy / StubIgnoresInvalidParams_StillReturnsServiceBusy），用 helper `assertServiceBusyError` 封装"`err.(*AppError).Code == 1009` + message 含'node-7 stub'或'not yet implemented'"两条断言。节点 8 激活时这 3 case 必须改写，helper 删除。

3. **handler 单测 HappyPath case 改名 + 改语义**：从 `HappyPath_ReturnsAck`（断言 200+code=0+data 透传）→ `HappyPath_ServiceReturnsServiceBusy_Forwards503`（断言 503+code=1009+message 含 stub 提示，stub service 仍验三参数透传）。节点 8 激活时改回 happy path 语义。

4. **handler 代码不动**：handler 已经是"`c.Error(err); return`"模式（让 middleware 写 envelope）—— 自带 nil/non-nil 分支区分，service 改 explicit-failure 后 handler 路径无需改动。这是好的"单一责任"分层让本次修复定位在 service + 测试两层即可。

5. **story 文件同步**：AC1 / AC5 / AC6 + 范围红线 + 故事定位等多处把"stub return nil + 节点 7 阶段成功 200"全部改为"stub return 1009 + 节点 7 阶段返 503"语义，并在每处节点 8 激活路径标注"激活时把 1009 换成 nil"。

6. **不新增错误码**：评估过新增 `ErrNotImplemented` —— 但 `ErrServiceBusy (1009)` + 文案"not yet implemented (node-7 stub)"已经完全表达语义（middleware 已经把 1009 翻 HTTP 503，符合 "Service Unavailable" 语义），无需扩 codes.go。这是 YAGNI 落地：能复用既有错误码就不引入新的。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在写"分阶段实装"的 stub endpoint 时，**必须**让 stub 显式返业务错误（如 `apperror.ErrServiceBusy` + 明确文案）让调用方在 HTTP/envelope 层立刻看到失败，**禁止**让 stub 返 nil success 把"未激活"伪装成"成功 + 空 side-effect"。

> **展开**：
> - **"分阶段实装"判定标志**：endpoint 接口签名 final，但 service 实装内部还没接真实下游（repo / 外部 API / 事务等）—— 比如 stub return nil、stub return 假数据、stub 只打日志不做事。出现任一即触发本规则。
> - **正确做法**：stub 实装 = `slog.WarnContext` 标注"stub 阶段 + 激活路径"+ `return apperror.New(ErrServiceBusy, "<endpoint> not yet implemented (<phase>; awaits <story> to activate)")`。让 middleware 翻 HTTP 503 + 1009，调用方在请求层立刻看到。
> - **错误码选择**：默认用 `ErrServiceBusy (1009)`（已有 + middleware 自动翻 HTTP 503，语义最贴近 "Service Unavailable"）；除非业务真的需要细分（如 stub 阶段需要区分"endpoint 不存在"vs"endpoint 临时关闭"），否则**不**为 stub 新增错误码（YAGNI）。
> - **WARN 文案必须自我描述**：包含 `phase` / `todo` / "awaits Story X.Y to activate" 等结构化字段，让运维 grep `phase=node-7-stub` 能找出所有还在 stub 状态的端点。
> - **错误 message 也必须自我描述**：包含"not yet implemented"或"node-7 stub"或"awaits Story X.Y to activate"，让调用方读 envelope.message 就能知道"不是我参数错，是 endpoint 还没激活"。
> - **service 单测断言 1009**：用 helper（如 `assertServiceBusyError`）统一封装 "err is `*AppError` + Code == 1009 + Message 含 stub/not-yet-implemented" 三条断言。激活时改写测试有明显信号（断言不再通过）。
> - **handler 单测 HappyPath case 命名必须诚实**：节点 7 阶段不要叫 `HappyPath_ReturnsAck`（暗示 200），改叫 `HappyPath_ServiceReturnsServiceBusy_Forwards503`（暗示 service 显式失败）。激活时统一改名回 `HappyPath_ReturnsAck`。
> - **story 文件每处节点 7/8 切片说明都必须同步**：范围红线 / AC / Tasks 三处任一处遗漏"stub 返 1009 + 503"语义，节点 8 激活的 owner 就可能照旧 AC 文字写出"return nil"错误实装。
> - **激活路径备注必须可机械执行**：service doc / lesson 文档明确写"激活时把 `return apperror.New(ErrServiceBusy, ...)` 替换成 `return nil`（happy path）+ 把 service 单测 1009 断言换成 happy path 断言"—— 让节点 8 owner 不必猜测设计意图。
> - **反例 1**：stub `return nil` + handler `response.Success(c, data, "ok")` —— 调用方拿 200，看不到"未激活"信号；e2e 链路在下游才发现"成功但 side-effect 空"，调试时间倍增。
> - **反例 2**：stub `return errors.New("not implemented")`（非 `*AppError`）—— middleware 会把它兜底翻成 1009，但 envelope.message 变成 `"服务繁忙"`（DefaultMessages[1009]）丢掉了 "node-7 stub" 提示。必须用 `apperror.New(ErrServiceBusy, "<具体上下文 + not yet implemented + 激活 story>")` 透传业务文案。
> - **反例 3**：stub 阶段在 service 内 panic（`panic("not implemented")`）—— 触发 Recovery 中间件兜底，对外仍是 1009 + 500，但日志会变成 ERROR 级别（污染告警）且 stack trace 没什么用。WARN log + 显式返 1009 是更克制的"我知道我在 stub"模式。

## Meta: 本次 review 的宏观教训

本 finding 与同期 Story 20-7 r5 lesson `dev-endpoint-correctness-over-contract-aesthetics` 形成对偶：

- **20-7 r5**：dev endpoint 不能为了"contract 美感"（不传 chestId）而牺牲正确性（猜 current chest 引入 race）
- **20-8 r1（本条）**：dev endpoint 不能为了"实装简洁性"（stub return nil）而牺牲诊断性（silent false-positive 拉长调试链路）

合起来的更深教训：**dev endpoint 是 ops / debug 工具，它的 UX 是"调用方看到的请求 / 响应"**，不是"server 侧日志 / 注释里写得多清楚"。任何让调用方在 server 响应层无法分辨"成功 vs 占位"/"成功 vs race"的设计，都是把 dev 工具的诊断价值反向消解 —— 跟"dev endpoint 存在"的初衷正相反。
