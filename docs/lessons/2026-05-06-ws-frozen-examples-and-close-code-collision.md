---
date: 2026-05-06
source_review: codex review (Story 10.1 r2, file: /tmp/epic-loop-review-10-1-r2.md)
story: 10-1-接口契约最终化
commit: 9f7b9e1
lesson_count: 2
---

# Review Lessons — 2026-05-06 — WS 协议冻结后的"示例字面量自洽"与"close code 全局保留"

## 背景

Story 10.1（接口契约最终化）r1 修复并冻结 §12.1 / §12.2 / §12.3 三节 WS 协议骨架后，r2 codex review 进一步识别出 2 处仍存在的契约级矛盾。两条 finding 的共性：**冻结只是冻字段表，但 reviewer 抓的是字段表外围的"示例字面量"和"全局编号空间"** —— 这两类问题在 r1 因为聚焦"表头/字段名/close code 列表本身是否对"而漏过。

- Finding 1：`pong` / `error` 的 JSON 示例 r1 时被作者明确"延后到 V2 修"（写了"修订说明"段落），实际是把"示例字面量与字段表的契约偏差"以"diff 最小化"为名延期 —— 但下游 Story 12.3 会把示例当 Codable fixture 复制粘贴
- Finding 2：`1009` 同时承担 RFC 6455 close code "Message too big" 的语义和 §3 全局错误码 `服务繁忙` 的语义，写在同一份 WS 协议文档的 close code 段和 `error.payload.code` 段，client 看到 `1009` 无法仅凭数字区分 close frame（fatal，断连不重连）和 application error frame（transient，忽略保连接）

2 条全部 fix，无 defer / wontfix。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 冻结协议的 JSON 示例必须与字段表自洽，不能用"延后到 V2"延期 | P2 (medium) | docs | fix | `docs/宠物互动App_V1接口设计.md:1587-1597, 1619-1632` |
| 2 | WS close code 不能复用应用层 `error.payload.code` 已占用的数字（1009 冲突） | P2 (medium) | architecture | fix | `docs/宠物互动App_V1接口设计.md:1388, §12.1 close code 表` |

## Lesson 1: 冻结协议的"示例字面量"必须与字段表同步，不能用"延后到下个大版本"逃避

- **Severity**: P2 (medium)
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1587-1597`（pong 示例段），`1619-1632`（error 示例段）

### 症状（Symptom）

§12.3 `pong` 字段表声明 `requestId` "必填"，但其下方的 JSON 示例只有 `type` / `payload` / `ts` 三个字段。作者在示例下方加了一段"既有示例修订说明"，明确写"本 story（10.1）保留示例 JSON 字面量不动以维持 git diff 最小化；正式实装时 server **必须**带 `requestId`；该示例下次 V1 文档大版本（V2）合并时**统一**补字段"。`error` 示例同问题。

下游影响：Story 10.3（server）/ Story 12.3（iOS Codable + mock fixture）会把示例当复制粘贴样本，生成的代码 / 测试夹具与字段表（已冻结）契约**不一致**，Story 12.3 的解析单测会通过（因为它用的是同一个错误示例），但真实联调时 server 一旦按字段表发 `requestId` → iOS Codable 因 fixture 不匹配而出现 silent 字段丢失或 decode 失败。

### 根因（Root cause）

冻结这一动作只覆盖了"字段表 / close code 表 / 信封字段名"三类**结构化**契约对象，没有覆盖"示例字面量"。作者潜意识里把"示例只是辅助说明"看作可延后的装饰物，于是用"git diff 最小化"作为理由把示例修复延后到 V2。但本仓库的工作流（CLAUDE.md + V1 接口设计.md §29 freeze 声明）明确规定 §12.3 进入冻结后任何**字段层面**的修改都要触发 Epic 10 / Epic 12 / Epic 11/14/17 全链回归 —— 一旦下游 story 按错误示例落地代码，这个 fixture 偏差会卡在"已 done 的 Story 12.3 测试夹具"层，必须新开 fix story 拉回归。

更深层：**示例与字段表之间存在隐式契约：示例必须是字段表的合法实例**。任何"示例与字段表不一致 + 用注释解释为什么不一致"的写法都是技术债，因为读者（人类 + 未来 Claude）按"字段表是规则、示例是样本"的认知去用，注释里的"延后"声明会被忽略。

### 修复（Fix）

`pong` 示例 `requestId: "msg_001"` 补全 + 删除"既有示例修订说明"段（替换为简洁的 `requestId` 语义注释 + 标注"可作为 Story 10.3 / 12.3 复制粘贴样本"）；`error` 示例同处理。

```diff
- JSON 示例（保留旧示例字面量；该示例缺 `requestId`，见下方修订说明）：
+ JSON 示例（与本节字段表对齐，含 `requestId`）：

  ```json
  {
    "type": "pong",
+   "requestId": "msg_001",
    "payload": {},
    "ts": 1776920345000
  }
  ```
- > **既有示例修订说明**：上述示例缺 `requestId` 字段...本 story（10.1）保留示例 JSON 字面量不动以维持 git diff 最小化...
+ > 注：`requestId` 回带客户端 `ping.requestId`；客户端 `ping` 未提供时回 `""`。该示例可作为 Story 10.3 / Story 12.3 的复制粘贴样本使用。
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **冻结一份契约文档（字段表 / 信封 / close code）** 时，**必须**同时修复同节区内所有 **JSON 示例 / curl 示例 / fixture 字面量**，使其成为字段表的合法实例 —— **禁止**用"延后到下个大版本"或"git diff 最小化"为名延期示例修复。
>
> **展开**：
> - 字段表是规则（rule），示例是合法样本（instance）。两者不一致 = 文档自破坏，下游 story（特别是 Codable / mock / 测试夹具生成）会按示例而非字段表落地，制造跨 epic 的 fixture 漂移
> - 冻结时如果发现示例与字段表不一致，**正确做法**是把示例改对（diff 几行）；**错误做法**是写"修订说明"段落把不一致合理化（这是用文字掩盖技术债）
> - "git diff 最小化"在文档冻结场景下不是合法理由 —— 冻结的代价就是字段表与示例必须 100% 自洽，差几行 diff 不影响 freeze 语义
> - 若示例当前真的不能改（如有大量下游已按旧示例对齐），正确路径是**先把字段表回退到与示例一致**，再走完整冻结流程；**不**把字段表冻结后留个矛盾示例
> - **反例**：在 §12.3 冻结声明（§1）下方紧接着冻结 `requestId` 字段 mandatory，但同节 JSON 示例不带该字段 + 写"延后修"注释 —— 下游 Story 12.3 解析层会按示例落 fixture，server Story 10.3 按字段表写 envelope，联调时 fixture 与 envelope 不匹配但单测都过，问题暴露在真机联调阶段（最贵的发现成本）
> - **反例 2**：示例段写"参考字段表"代替写完整字面量 —— 等于把字段表的合法性证明义务推给读者；下游会去复制示例代码块字面量，不会去读字段表交叉验证

## Lesson 2: 同一份协议文档的 close code 数字段与应用 error code 数字段必须**全局唯一**，不能复用 RFC 标准段

- **Severity**: P2 (medium)
- **Category**: architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1388`（消息大小约束 close code 1009），`§12.1 close code 表`（新增 1008 行 + 关键约束新增 "不使用 1009" 段）

### 症状（Symptom）

§12.3 关键约束写"超限服务端 close 1009（message too big）+ 不重连"；§3 全局错误码表 + §12.3 `error.payload.code` 同时定义 `1009 = 服务繁忙`（应用层 transient 错误，client 应忽略并继续）。一个 WebSocket client 在同一连接生命周期内可能收到：

1. 一个 close frame 携带 close code `1009` —— 期望行为：close socket，不重连（fatal）
2. 一个 text frame `{"type":"error","payload":{"code":1009,...}}` —— 期望行为：忽略并保持连接（transient）

仅凭数字 `1009`，client 无法判断该数字来自 close frame 还是 application error frame —— 在多数 WS client SDK 里 close 事件 callback 和 message 事件 callback 是分开的，但日志 / 监控 / 错误码统一处理路径常常把"WS 错误码"扁平化成单一数字字段。这种扁平化路径下，1009 的两种语义会被合并，造成监控告警混乱、自动重连决策错误。

### 根因（Root cause）

RFC 6455 §7.4.1 的 standard close code 段（1000-1015）和本协议 §3 的 application error code 段（1000-9999）**未做命名空间隔离**。RFC 1009 = "Message Too Big"，本协议 §3 1009 = "服务繁忙"，两者数字相同但语义来自不同标准空间（前者来自 RFC，后者来自本协议自定义全局错误码表）。

写文档时作者直接照抄了 RFC 1009 这个最贴合"消息超大"语义的标准 close code，没意识到 §3 全局错误码段已经把 1009 占了。这是典型的**namespace overlap blindness** —— 两个看似独立的编号空间，因为人脑把"WS 协议"当成统一语境，会自然假设"协议范围内编号唯一"，但实际上 RFC close code 空间和应用 error code 空间是不同来源。

### 修复（Fix）

`message too big` 场景的 close code 从 RFC 1009 改用 RFC 1008（Policy Violation）—— 1008 在 RFC 6455 中定义为"客户端违反协议策略"，"消息超过 server 配置的 max_message_size_bytes"完全符合"违反 server 策略"语义，且 1008 在本协议 §3 全局错误码表中**未被占用**（1008 = 幂等冲突，但那是 HTTP 业务层用，不会出现在 WS close frame）。

具体改动：

- §12.1 close code 表新增 1008 行：`server 主动 close` / `客户端违反协议策略 —— 节点 4 唯一触发：单条消息超 ws.max_message_size_bytes` / `reason = "message too large"` / `不自动重连（客户端 bug）`
- §12.1 关键约束新增"**不使用 RFC close code 1009**"段：明确说明禁用原因（与 §3 应用错误码 1009 冲突），并交叉引用 §12.3 `error.payload.code`
- §12.1 关键约束更新"协议/网络级断开"清单：`1000 / 1001 / 1011` → `1000 / 1001 / 1008 / 1011`
- §12.1 关键约束更新 log level 规则：新增"1008 写 log error（必排查，疑似客户端 bug 或恶意流量）"
- §12.3 关键约束行（line 1388）：`close 1009（message too big）` → `close 1008（policy violation, reason = "message too large"）`，并就地说明"不使用 RFC 1009"的理由（避免读者疑惑）

注意 §3 §12.3 的 `1009 = 服务繁忙`（应用 error code）保留不动 —— 它是文档既有契约，没有冲突理由动；冲突方是后加的 close code 1009。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **同一份协议文档中同时定义"传输层 close code"和"应用层 error code"** 时，**必须**确保两个编号空间**全局唯一**（即使来自不同标准空间，如 RFC 6455 close code vs. 自定义业务 error code）。**禁止**只检查"我自己的命名空间内是否冲突"。
>
> **展开**：
> - WS / HTTP / gRPC 等协议都会有 transport-level status code（来自 RFC）和 application-level error code（来自业务定义）两套数字，client 在监控 / 日志 / 错误处理路径中常会把它们扁平化成单一字段。冲突会让"仅看数字判断行为"的代码路径决策错误
> - 引入新 transport code（如新增 close frame code）时，**必须**先去全局错误码表 / 应用 error code 表里搜该数字 —— 即使两个表来自不同章节、不同语义空间
> - 选择 transport code 时，优先选 RFC 段中**应用 error code 没占用**的数字，而不是选"语义最贴合 RFC 文字描述"的数字。语义贴合度可以靠 reason 字符串补，命名空间冲突无法靠 reason 字符串补
> - close frame 的 reason 字符串是消歧的最后防线，但**不能依赖它**：reason 在很多客户端 SDK 中要专门读 close event 才能拿到，监控 / 错误码扁平化路径常常丢 reason 只留 code
> - **反例**：WS 协议规定 close 1009 = "消息超大"（直接用 RFC 6455 §7.4.1 文字），同协议 §3 全局错误码表又规定 1009 = "服务繁忙"用于 `error.payload.code`。client 监控面板看到一条 `WS 1009` 告警 → 不知道是 transport fatal（要 alert + 工程师介入）还是 transient app error（要忽略 + auto-recover），结果要么误报警噪一片，要么漏报真正的客户端实装 bug
> - **反例 2**：通过加注释"close code 1009 仅在 close frame 出现，不会出现在 error.payload.code"来回避冲突 —— 这种"靠读者注意上下文"的设计在跨语言客户端实装时会被忽略，因为 client 拿到的就是一个 int 字段
> - **反例 3**：为冲突的数字加 prefix（如 close code 写"C1009"，error code 写"E1009"）—— 这违反 RFC，close frame code 必须是 raw uint16 数字，不能加前缀

---

## Meta: 本次 review 的宏观教训

r1 已经把 close code 表本身、信封字段表本身、节点 4 业务消息冻结边界都修对了，r2 命中的两条仍然是"协议骨架的二阶不一致"：

- 一阶：字段表 / close code 表 / 错误码表本身的内部正确性（r1 修完）
- 二阶：示例字面量是否是字段表的合法实例 + close code 数字空间是否与应用 error code 空间无冲突（r2 修）

**冻结一份协议文档的检查清单**应该包含至少这三层（按发现成本递增排序）：

1. **结构化契约对象**：字段表 / close code 表 / error code 表本身的字段名 / 类型 / 必填性是否互相自洽（最容易抓，r1 已覆盖）
2. **示例字面量与结构化契约对齐**：同节内所有 JSON / curl / 命令行示例必须是字段表的合法实例（r2 finding 1）
3. **跨章节命名空间唯一性**：close code 数字段 / error code 数字段 / route 路径段等编号空间必须全局唯一（r2 finding 2）

未来再做 §X.1 类契约最终化 story 时（如 Story 11.1 / 14.1 / 17.1），sm/dev/review 三方都应至少检查这三层；用本 lesson 的 finding 1 / 2 作为反例触发自检。
