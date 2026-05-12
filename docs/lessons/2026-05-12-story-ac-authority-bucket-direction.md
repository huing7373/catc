---
date: 2026-05-12
source_review: codex review r10 on Story 14-1 接口契约最终化（文件 /tmp/epic-loop-review-14-1-r10.md）
story: 14-1-接口契约最终化
commit: a62e401
lesson_count: 1
---

# Review Lessons — 2026-05-12 — Story AC 在"权威等价"语义中必须区分字段方向（client→server / server→client / ack-only），不能把所有"值域等价"字段一概并入"权威等价桶"

## 背景

Story 14.1（接口契约最终化）AC7 描述 §5.2 state-sync request/response 与 §10.3 / §12.3 三处 server → client 字段的等价关系。前几轮（r8 / r9）虽已修正 V1 doc 主文（§5.2 line 608-613 明确区分"值域等价（六处恒成立）"vs"权威等价（仅四处 server → client 字段，自 14.3 起）"），但 story 文件 AC7 描述（line 293）+ AC7 验证清单（line 526）残留旧措辞，仍说"14.3 后六处都读真实值，承载相同权威级别"，把 §5.2 request `state` + response `data.state` 错误地与四处 server → client 字段并列进入权威等价桶。codex r10 一发命中。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | story AC7 line 293 + 526 把 §5.2 request.state + data.state 错误归入"14.3 后权威等价六处" | P2 (medium) | docs | fix | `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md:293,526` |

## Lesson 1: AC 在描述"字段语义跨章节等价"时，必须区分字段**方向 + 角色**（client→server 写入信号 / server→client 权威信号 / server→client ack-only 信号），不能用"六处都 …"或"N 处都 …"的并列句式抹平这些区别

- **Severity**: P2 (medium)
- **Category**: docs
- **分诊**: fix
- **位置**: `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md:293,526`

### 症状（Symptom）

Story AC7 表述：
> "六处字段（含 §5.2 request `state` + §5.2 response `data.state` + §10.3 + `room.snapshot` + `pet.state.changed` + `member.joined`）等价分两层 …… 14.3 后六处都读真实值，承载相同权威级别"

但 V1 doc 主文（同一 Story 已锚定）明确：

- `§5.2 request state` 是 client → server **单向写入信号**，不参与 server → client 权威等价讨论
- `§5.2 response data.state` 是 server → client **ack-only 信号**（回显入参，server-acknowledged 入账确认），**永不**进入"权威等价桶"；14.3 前后语义不变
- 权威等价桶**仅四处** server → client 字段：(i) `pet.state.changed.payload.currentState` (ii) `room.snapshot.payload.members[].pet.currentState` (iii) `member.joined.payload.pet.currentState` (iv) `GET /rooms/{roomId}.data.members[].pet.currentState`

Story AC 的"六处都进入权威等价"句式与 V1 doc 主文自相矛盾，可能误导 iOS Story 15.4 / server follow-up story 把 HTTP `data.state` 当成全局权威信号（值同 ≠ 信任级别同）。

### 根因（Root cause）

写 AC 描述时，**为了"概括"把所有"值域 / DB 来源相同"的字段并列成一个集合（"六处字段"），然后给这个集合套用统一的"等价分两层"框架。** 但"值域等价"与"权威等价"的成员集合不同：

- **值域等价**：成员包含所有"DB 来源相同 + 类型相同"的字段，无方向限制 → 六处
- **权威等价**：成员**仅限** server → client 且承担"权威信号"角色的字段；client → server 写入信号、server → client ack-only 信号都**不**入桶 → 四处

并列句式（"六处字段等价分两层 …… 14.3 后六处都读真实值"）让"权威等价"的成员集合被错误扩展到六处。Reviewer 多轮 review（r8 / r9）已抓到 V1 doc 主文需要拆分（line 608-613 现已分三个 sub-bullet 显式标注），但 story 文件作为"copy + 简化"复述同语义时，又抹回成"六处都 …"的并列句式 —— 这是**简化句式吃掉技术区别**的典型踩坑。

### 修复（Fix）

**line 293（AC7 描述段）**：把单一 bullet 拆成 8 行嵌套结构，先列六处字段的**方向 + 角色**（§5.2 request = 写入信号、§5.2 response = ack-only 信号、四处 server → client = 权威信号），再列"等价分两层"，其中：
- (a) 值域 / DB 来源等价 → 标注"恒成立 + 含六处"
- (b) 权威 / client 信任层等价 → 标注"**仅**针对四处 server → client 字段；§5.2 request `state` + response `data.state` **不**包含在此权威等价桶中，14.3 前后语义不变"

**line 526（AC7 验证清单）**：把"六处字段等价分两层 + 14.3 前置"压缩描述改写成三段：六处值域等价（恒）+ 四处 server → client 权威等价（14.3 后）+ §5.2 request / response 两处不入权威桶（永不变）。

**未改动**：line 229（§12.3 关键约束）已正确说"四处"；line 158（§5.2 设计权衡）已正确说 `data.state` 是 ack + self-only 例外仅限 self entry UI 驱动。

**未改动**：V1 doc 主文（按本轮 review override 显式要求 "本轮修复仅改 story 文件"）；主文 §5.2 line 608-613 在 r8/r9 已修正。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **AC 描述里写"N 处字段等价"或"N 处字段分两层"** 时，**必须**先按**字段方向 + 角色**分类（client → server 写入 / server → client 权威 / server → client ack-only / log-only / UI-only），再为每个类别独立声明等价层；**禁止**把不同方向 / 不同角色的字段并列进同一个集合后套用统一的"等价"或"权威"句式。
>
> **展开**：
> - **字段方向 ≠ 字段语义**：值域 / DB 来源相同（"happy path 下值相同"）不蕴含"权威级别相同"。前者是序列化层属性，后者是**信任层 / 业务层属性**
> - **ack-only 信号 ≠ 权威信号**：HTTP response 回显入参（"server-acknowledged 入账确认"）是 ack，只能驱动 caller 自己的本地 UI，**不**能跨 client / 多设备承担权威角色。即便值相同，信任级别也不同
> - **AC 措辞校验**：在 AC 文档定稿前，用 grep 抓"N 处字段都 X" / "N 处都读真实值" / "承载相同 X" 这类并列句式，逐条核对每一处字段的方向 / 角色是否真的同质；如有 client → server 写入 / ack-only 信号混入 server → client 权威集合，**必须**拆出独立 bullet 标注
> - **Story AC 文档与主契约文档（V1 doc）双向对齐**：当 V1 doc 主文已经把"N 处等价"拆成"N 处值域 + M 处权威"（M < N）后，story 文件 AC 描述若仍残留旧"N 处"措辞，必须同步改写；reviewer 在多轮 review 后才发现这类残留是**最常见的 docs 漂移源**之一
> - **反例**：
>   - "六处字段等价分两层 …… 14.3 后六处都读真实值，承载相同权威级别"（混了 client → server 写入信号、server → client ack-only 信号、server → client 权威信号；权威集合被错误扩展到六处）
>   - "三处字段都是 server-acknowledged 权威信号"（如果其中一处是 HTTP response 回显入参，则它是 ack-only，不是权威）
>   - "request 和 response 字段值域 / 权威双等价"（request 是 client → server 写入信号，根本不参与 server → client 权威讨论；这种并列句式抹平了方向区别）
> - **正例**：
>   - "六处字段值域 / DB 来源等价（恒成立）；其中四处 server → client 字段承载权威等价（自 14.3 起）；§5.2 request `state` 是 client → server 写入信号 + §5.2 response `data.state` 是 server → client ack-only 信号，两处不入权威等价桶（永不变）"
>   - 描述等价时按"方向 + 角色"先分类，再为各类独立声明等价层
