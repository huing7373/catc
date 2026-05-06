---
date: 2026-05-06
source_review: codex review (epic-loop r5, /tmp/epic-loop-review-10-1-r5.md)
story: 10-1-接口契约最终化
commit: 9a78506
lesson_count: 2
---

# Review Lessons — 2026-05-06 — WS close code 必须用 4xxx 应用自定义段隔离 §3 应用错误码 + 跨文档配置 key 双向锚定

## 背景

Story 10-1（V1 协议契约节点 4 冻结）codex review 第 5 轮。前几轮 r1-r4 已修过 "error 双重语义"、"4005 心跳超时 close code"、"信封字段冻结"、"冻结段内部前后矛盾"、"reserved close code 1006/1009"等问题。r4 已经把"消息超大"close code 从 RFC 1009 改用 1008（policy violation），但 r5 codex 指出**这个改动只解决了 1009 冲突，没解决 1008 冲突** —— §3 全局错误码表里 `1008 = 幂等冲突`，跟 close code 1008 仍然撞数字空间，下游 client / log / 监控仅凭数字无法区分 transport-level fatal 与 application-level retryable。同时 r5 也指出 V1 文档新冻结的 `ws.max_message_size_bytes` 配置 key 没有同步到 Go 项目结构 §12.2 config schema，两份"权威"文档对该 key 是否存在结论不一致，下游实装 Story 10.3 / 10.4 会拿到分歧的输入。

r5 是 epic-loop 5 轮上限的最后一轮，必须修彻底（架构层、不再修补丁）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | close code 1008 (message too large) 与 §3 全局错误码 1008 (幂等冲突) 数字空间冲突 — 必须改用 4xxx 应用自定义段（4006） | high (P1) | architecture / docs | fix | `docs/宠物互动App_V1接口设计.md:1329` `:1334-1339` `:1392` |
| 2 | `ws.max_message_size_bytes` 在 V1 文档冻结但未同步到 Go 项目结构 §12.2 config schema YAML 示例 | medium (P2) | config / docs | fix | `docs/宠物互动App_Go项目结构与模块职责设计.md:935-937` |

## Lesson 1: close code 1xxx 段任何挑选都会撞 §3 应用错误码段 — 必须用 4xxx 应用自定义段

- **Severity**: high (P1)
- **Category**: architecture / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1329`（close code 表行）/ `:1334-1339`（关键约束）/ `:1392`（§12.2 关键约束）

### 症状

r4 修复中把"消息超大"close code 从 RFC 1009 改用 RFC 1008（policy violation），理由是 §3 全局错误码表里 `1009 = 服务繁忙` 已被占用、close 1009 会与 application error 1009 数字冲突。但是 r5 codex 指出 §3 里 `1008 = 幂等冲突` **同样**已被占用（且按 0008 错误协议决议被归类为 transient business code，client 端要按 retry 处理），所以 close 1008 仍然撞冲突 —— 下游 client / log / 监控只看到数字 `1008` 时无法区分这是 fatal WS close 还是可重试的业务错误，重连/告警分类还是会被误导。r4 的修复是表面修补，没解决根因。

### 根因

**1xxx close code 段（RFC 6455 标准段）与本协议 §3 全局错误码段（应用层用 1xxx）在数字空间上整体重叠**。本协议 §3 占用了 1001-1009 这一连串 1xxx 段值作为应用错误码（语义见 §3 表），所以 1xxx 段任何挑选（1008 / 1009 / 1010 …）都会撞 §3。r1-r4 一直在 1xxx 段内"换数字"——避开 1009 改 1008，下次还会被指出 1008 也撞——这是死循环，因为根因不在"具体哪个数字"，而在"段位策略"。

正确的架构层修法：**应用层自定义的 close code 必须走 4xxx 段（RFC 6455 §7.4.2 application-specific 4000-4999），与 §3 应用错误码（1xxx 段）数字空间完全隔离**。本协议已经在 4xxx 段建立了先例：4001 token 失效 / 4002 invalid roomId / 4003 user not in room / 4004 room not found / 4005 heartbeat timeout —— 只是 r4 心存"侥幸"复用 1xxx 段挑了一个"看起来空着"的 1008，结果 1008 在 §3 里也是被占的，只是没在 §12.3 节点 4 适用错误码表里直接列出（节点 4 阶段适用错误码表只有 1009）让人误以为安全。

### 修复

- 把 close code 从 1008 改用 **4006**（4xxx 段下一个可用值，4001-4005 已用）。
- §12.1 close code 表（line 1329）：`1008` → `4006`
- §12.1 关键约束业务级拒绝清单（line 1334）：`4001 / 4002 / 4003 / 4004` → `4001 / 4002 / 4003 / 4004 / 4006`，并加注 4006 的定位（"4006 = 客户端实装 bug，记 log error 后回退"）
- §12.1 关键约束 1xxx 网络级断开清单（line 1336）：`1000 / 1001 / 1008 / 1011` → `1000 / 1001 / 1011`（1008 移出 1xxx 段）
- §12.1 关键约束"不使用 1009"段（line 1338）：升级为"不使用 1008 / 1009"段，理由从"§3 1009 服务繁忙"扩展为"§3 1008 幂等冲突 / 1009 服务繁忙都被应用层占用"，统一引导到"4006（4xxx 段）"
- §12.1 log level 行（line 1339）：`1008 写 log error` → `4006 写 log error`
- §12.2 关键约束（line 1392）：close `1008` → close `4006`，理由说明扩为"不使用 RFC 1008 / 1009"

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在为 WS / 应用协议设计 close code（或类似与 RFC 标准段共存的"应用扩展码"）时，**必须**先检查"应用层错误码表"已经占用了 RFC 标准段哪些数字，**禁止**在已被占用的 RFC 标准段（1xxx）内为应用语义挑选任何 close code，**必须**走应用自定义段（4xxx）。
>
> **展开**：
> - 立刻去枚举两份字典：(A) RFC 标准 close code 段（1xxx，0-2999 reserved + 3000-3999 IANA），(B) 本协议 §3 全局错误码表占用的所有数字。两份重叠区里**任何**数字都不能用作应用层 close code，无论看起来"空着"还是"语义匹配"。RFC 6455 §7.4.1 列了 1000 / 1001 / 1002 / 1003 / 1005 / 1006 / 1007 / 1008 / 1009 / 1010 / 1011 / 1015 这些保留语义；本协议 §3 1001-1009 几乎全部占用 —— 重叠区基本覆盖了 1xxx 整段
> - **段位选择策略**：close code 用 4xxx（RFC 6455 §7.4.2 application-specific 4000-4999 留给应用），与 1xxx 标准段 + §3 应用错误码段都不冲突。本协议已经建立先例：4001 / 4002 / 4003 / 4004 / 4005 / 4006 全在 4xxx 段
> - **段位例外约定**：4xxx 段默认是"业务级拒绝（不重连）"语义（如 4001 token 失效）；如某个 4xxx code 需要 transient → 应重连语义（如 4005 心跳超时），必须在表的"关键约束"里**显式列为例外**（lesson 2026-05-06-ws-error-dual-semantics-and-heartbeat-close-code.md 已沉淀）
> - **多轮 review 警示**：如果第 N 轮 review 指出 X 数字撞 §3，第 N+1 轮**不要**只换一个 1xxx 段数字（侥幸"这个数字 §3 可能没占"）—— **必须**换段位（1xxx → 4xxx），否则下一轮 review 会指出新挑的数字也撞。这是架构层 vs 表面打补丁的分水岭
> - **反例**：r4 把 RFC 1009 改 RFC 1008（一阶补丁，没换段位）；r5 codex 立刻指出 1008 在 §3 也被占。正确做法应该 r4 直接换段到 4xxx
> - **反例**：在协议表里看到 "1008 message too large（policy violation）" 时不去交叉查 §3，因为"1008 RFC 6455 §7.4.1 是 policy violation 标准语义、看起来很合适"—— 标准语义匹配 ≠ 与本协议字典安全；段位策略才是终局判据

## Lesson 2: 跨文档配置 key 必须双向锚定 — 一边声明冻结另一边连 schema 都没列 = 自证未冻结

- **Severity**: medium (P2)
- **Category**: config / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_Go项目结构与模块职责设计.md:935-937` (§12.2 ws config schema)

### 症状

V1 文档 `docs/宠物互动App_V1接口设计.md` §1 节点 4 冻结声明（line 35）把 `ws.max_message_size_bytes` 锚定为 prod 不可覆盖的契约（默认 16 KB），但 Go 项目结构文档 `docs/宠物互动App_Go项目结构与模块职责设计.md` §12.2 config schema YAML 示例（line 935-937）只列 `ws.heartbeat_timeout_sec`，没列 `max_message_size_bytes`。两份"权威"文档对该 key 是否存在结论不一致：V1 说已冻结、Go 项目结构连 schema 都没列。下游 Story 10.3 / 10.4 实装方先读哪份就拿到不一样的结论 —— 一份说 key 已冻结、另一份连 key 都没有 —— 容易落成不同的配置名（如硬编码 16384 / 用别的 key 名）或硬编码默认值，直接破坏冻结想保证的"跨端约定"。

### 根因

冻结一个配置 key 时，光在 V1 协议文档（"对外契约"）锚定还不够，必须**同时**在 Go 项目结构文档（"对内 schema"）的 config 段同步加上对应 YAML 字段 + 默认值 + 与 V1 文档的反向引用注释，**双向锚定**。如果一边声明"已冻结"而另一边连定义都没有，等于自证未冻结 —— 实装方读到 schema 缺这个 key 时，理性反应是"V1 文档可能在抢跑、Go 项目结构才是 schema 真相"，反而会忽略 V1 的冻结声明。这与 r3 lesson "Story AC 里把决定列为 deliverable" 同思路：决定不能推到下游 story，跨文档锚定也不能推到下游实装。

具体到本 case：r4 review 时 codex 已经指出过"V1 文档锚定 max_message_size_bytes 但 §12.2 没列"的潜在风险（在 10-1 implementation artifact line 651 的设计 note 里有提及"本 story 不修改 §12.2，由 Story 10.3 实装时同步"）—— 但这个 punt 策略本轮被 r5 codex 直接判为不合格："Story 10.3 实装时同步"等于把契约约束的成立**条件性**地推到下游 story 落地之前，冻结声明在落地前那段时间是悬空的 / 自相矛盾的。正确做法是：协议骨架冻结 story 自己**就地**完成跨文档双向锚定，**不**推给实装 story。

### 修复

在 `docs/宠物互动App_Go项目结构与模块职责设计.md` §12.2 YAML 示例 ws 段加 `max_message_size_bytes: 16384`（含 inline 注释引用 V1 §1 / §12.2），并在 YAML 块下加一个引用块明示"该 ws 段两个 key 的默认值属契约一部分，由 V1 §1 节点 4 冻结声明锚定"+"prod 不可覆盖, dev/test 可覆盖"+"修改默认值视为契约变更走完整冻结流程"。

before:

```yaml
ws:
  heartbeat_timeout_sec: 60
```

after:

```yaml
ws:
  heartbeat_timeout_sec: 60
  max_message_size_bytes: 16384  # 16 KB；与 V1 接口设计 §1 节点 4 冻结声明 + §12.2 关键约束对齐；prod 不可覆盖，dev/test 可覆盖
```

并在 YAML 后追加一段 `> 配置 key 与 V1 协议契约对齐` 引用块说明。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在协议契约 story 里冻结一个**配置 key**（不是字段、不是错误码、不是消息字段，而是 YAML config key）时，**必须**在同一 PR / 同一 commit 内同步更新所有"对内 schema"文档（Go 项目结构 / iOS 工程结构 / DB 设计等），**禁止**把更新推到下游实装 story。
>
> **展开**：
> - 配置 key 冻结至少涉及两份文档：(A) "对外契约"（V1 协议文档）—— 声明默认值属契约一部分、prod 不可覆盖；(B) "对内 schema"（Go 项目结构 / iOS 工程结构）—— 在 config struct / YAML schema 示例里**显式**列出该 key + 默认值 + 反向引用 (A) 的注释。两份必须双向锚定
> - **判据**：fix-review review 时如果发现"V1 文档锚定了 X 配置 key 但 Go 项目结构 §12.2 schema 没列"，这就是 r5 类型的 P2 finding；不论 story 文档怎么 punt（"由 Story 10.3 实装时同步"），都得就地修
> - **反例**：在 Story 10.1 implementation artifact 里写"本 story 仅预定该 key 名称 + 默认值在 V1 文档 §1 节点 4 冻结声明里，Story 10.3 实装时按本 story 锚定的 key 名 + 默认值写 YAML 配置"。这是把冻结约束的成立**条件性**地推到下游 story 之前，冻结在落地前那段时间是自相矛盾的
> - **反例**：跨文档锚定**只**靠引用（"详见 Story 10.3"）而不在两份 schema 都写出 key + 默认值。引用是辅助，**字面定义**才是 schema 真相 —— 实装方先读到哪份取决于翻文档顺序，schema 缺的那份会被拿去当真相
> - **更广义的规则**：协议契约 story 里只要"冻结"一个东西（字段名 / 类型 / close code / 配置 key），就得检查这个东西的"完整定义场所"全集（可能不止两份），全部就地完成同步，**不**留 stub 给下游 story

---

## Meta: 本次 review 的宏观教训

r5 是 epic-loop 5 轮上限的最后一轮（再不通过 epic-loop 会 HALT），暴露的两条 finding 共享同一个**根因模式**：**协议契约冻结 story 在挑选数字 / 配置 key 等字面量时，只在"目标段位"内做了局部决策，没做全局段位/全局文档双向 sweep**。

具体表现：
- Lesson 1：close code 在 1xxx 段内挑数字（一阶决策），没意识到 1xxx 段整体跟 §3 撞段位（高阶决策）
- Lesson 2：在 V1 文档锚定配置 key（一边决策），没在 Go 项目结构同步（双向决策）

**统一的预防规则**：每次"冻结某个字面量"操作，都必须做两步 sweep：
1. **全段位 sweep**：该字面量所属的整个数字段位 / 命名空间，是否与本协议其他字典段位重叠？如果重叠，换段位而不是换数字
2. **全文档 sweep**：该字面量在哪些文档可能出现 schema 定义？所有这些文档的 schema 段都得就地同步，不准 punt

`/fix-review` 命令的 r5 sweep 自检流程已经按这个模式做了，未来契约 story 在 r1-r3 阶段就应该自带这两步 sweep，避免反复"补丁式"修复。
