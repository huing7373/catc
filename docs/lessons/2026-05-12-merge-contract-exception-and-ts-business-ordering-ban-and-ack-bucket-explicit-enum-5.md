---
date: 2026-05-12
source_review: /tmp/epic-loop-review-14-1-r5.md (codex review r5)
story: 14-1-接口契约最终化
commit: 46242cb
lesson_count: 3
---

# Review Lessons — 2026-05-12 — 临时窗口优先级必须改写通用 merge contract 的覆盖规则（不能只声明）& `ts` 字段全局禁用业务排序必须章节间一致 & 权威等价桶的"四处字段"必须显式枚举不能引用章节号（14-1 r5）

## 背景

本 lesson 来自 Story 14.1（接口契约最终化）codex review 第 5 轮。r1-r4 已经把 `state-sync` 接口、`pet.state.changed` 字段表、跨章节字段等价层、self-broadcast 兜底、临时窗口权威信号优先级排序逐步收敛。r5 三条 finding 都是 **r3/r4 引入的新声明** 与 **既存通用规则** 的「未对齐」问题：声明了规则 A，但作用域内的另一处通用规则 B（数值字段直接覆盖 / `ts` 不可用作排序 / `data.state` 不入权威桶）仍按旧语义走，client 无法同时遵守两条规则。

核心模式：**新加的优先级排序 / 等价等"宏观语义声明"，必须 propagate 到所有具体 merge / 字段使用规则里，否则形同空文**。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联位置 |
|---|---|---|---|---|---|
| 1 | §12.3 line 2092 数值字段"直接覆盖"通用规则未给 14.3 前临时窗口开例外，导致 §5.2 line 612 / §12.3 line 2247 的优先级排序无法在 merge 层落地 | P2 (medium) | docs / architecture | fix | `docs/宠物互动App_V1接口设计.md:2092` |
| 2 | §12.3 line 2247 + 2250 让 client 按 `ts` 排序判断 state 新旧，与 §12.2 line 1961 全局 envelope "`ts` 不用作业务排序"硬性约束矛盾 | P2 (medium) | docs / architecture | fix | `docs/宠物互动App_V1接口设计.md:2247,2250` |
| 3 | §5.2 line 612 "四处 server→client 字段"措辞含糊（写"§5.2 / pet.state.changed / room.snapshot / member.joined"），与 line 610 "`data.state` 不入权威桶"声明歧义 | P3 (low) | docs | fix | `docs/宠物互动App_V1接口设计.md:610-612` |

## Lesson 1: 临时窗口权威信号优先级排序必须直接改写通用 merge contract 的字段覆盖规则，仅声明"优先级 X > Y"对 client 实装无意义

- **Severity**: medium (P2)
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:2092`

### 症状（Symptom）

§5.2 line 612 + §12.3 line 2247 都声明：Story 14.3 落地前的临时窗口内，client 实装层应遵守"权威信号优先级 `pet.state.changed` WS 广播 > `room.snapshot` / GET / `member.joined`"（因后三者是 placeholder `1`，不可反推权威状态）。

但 §12.3 line 2092 的 **client merge contract 第 3 条**（数值字段规则）仍写老版本："`pet.currentState` 数值字段无 placeholder 信号约定，client 收到该值时**应**直接覆盖 client 已有值"。

冲突场景：iOS Story 15.2 / 15.4 client 已经从 `pet.state.changed` 收到 `currentState: 2`（walk），随后因 reconnect / 其他成员 join 触发新的 `room.snapshot` / `member.joined`，server 在该窗口下发 `currentState: 1`（placeholder）。按 line 2092 client 必须直接覆盖 → 自己的 walk 状态被回退到 rest，直到下一次 `pet.state.changed` 重新广播。两条规则无法同时遵守。

### 根因（Root cause）

长 spec 多个层级声明同一字段的处理规则时，**宏观优先级声明** 与 **微观字段 merge 规则** 不是同一文本，更新会脱节：

- r3/r4 阶段引入"临时窗口权威信号优先级"宏观声明，写在 §5.2 line 612 + §12.3 line 2247，强调"placeholder `1` 不能反推权威"
- 但 §12.3 line 2092 的通用 merge contract 第 3 条是 r1 阶段定的，当时所有数值字段恒为 `1`（节点 4 placeholder + Story 11.7 真实实装都固定 `1`），所以写"直接覆盖"完全合理
- 当 Story 14.3 引入"placeholder `1` vs 真实 `2/3` 临时窗口期共存"语义后，宏观优先级声明 propagate 了，但通用 merge contract 第 3 条没被同步改写
- client 实装层（Story 15.2 / 15.4）按合同条款逐条遵守时，硬性矛盾出现

### 修复（Fix）

在 §12.3 line 2092 数值字段规则末尾追加**显式临时窗口例外**：

```
**Story 14.3 落地前的临时窗口例外**（适用于 `pet.currentState` 单字段）：
在该窗口内，`room.snapshot` / GET / `member.joined` 三处的 placeholder `1` 值**不**触发"直接覆盖"，
client **应**按"如果新值来自上述三个 placeholder 源 + client 已有值为非 `1` 真实值
（来自 `pet.state.changed` WS 广播的 `2/3`）→ 跳过覆盖，保留已有真实值"分支处理；
仅当 client 已有值缺失 / 也是 `1` 时才接受 placeholder `1`。
该例外**仅对 `pet.currentState` 字段** + **仅在 14.3 落地前**生效；
Story 14.3 落地后例外失效，回归"数值字段直接覆盖"通用规则。
```

这样 client 实装层只读 line 2092 一处即可同时满足 line 612 / line 2247 的优先级排序声明。

### 预防规则（Rule for future Claude）⚡

> **一句话**：在长 spec 引入"宏观优先级排序 / 字段权威等价层" 等高层声明时，**必须**同步检查所有具体 merge / 解析 / 覆盖规则的措辞，把高层声明 propagate 到操作层；**禁止**只在高层加声明而不改 client 必须遵守的具体 merge 第 N 条。
>
> **展开**：
> - 标志：当 spec 加一条"X > Y > Z"或"X 等价于 Y"的宏观排序 / 等价声明 → 立即搜全文检查所有跟 X / Y / Z 字段相关的「覆盖 / merge / 直接赋值」措辞是否同步
> - 实操：grep 字段名（如 `pet.currentState`）→ 列出所有出现 → 对每处判断 merge / 覆盖语义是否与新宏观声明一致
> - 临时窗口语义（"Story X 落地前 vs 落地后"）的例外必须**写在通用规则的同段落紧邻位置**，而非另起一节 —— 否则 client 实装层只读 merge contract 第 N 条不会看到例外
> - **反例**：r5 之前的 line 2092 / line 612 / line 2247 三段彼此独立声明，client 实装层逐条遵守时硬性矛盾 → 蒸馏后留作 few-shot 负例

## Lesson 2: WS envelope 的 `ts` 字段全局禁止业务排序时，所有具体业务消息章节都禁止"按 `ts` 排序 / 判定新旧"的措辞，章节间硬性一致

- **Severity**: medium (P2)
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:2247,2250`

### 症状（Symptom）

§12.2 line 1961 全局 WS envelope 关键约束第 1 条明确写："`ts` 用途是客户端日志关联 + UI 排序，**不**用作业务时序判断（业务时序仍由 server 单调时钟保证；client 不应基于 `ts` 比较推断'谁先发生'，因为不同 server 实例时钟可能漂移）"。

但 §12.3 `### 宠物状态变更` 章节两处给出相反指令：

- line 2247 末尾："如四处任一在权威等价生效后仍返回不同值表示 server 端状态不一致（race condition），client 实装层应以最近一次 WS 广播为最新真相源（**按 `ts` 字段时间戳判断新旧**）"
- line 2250："client 可用作时序排序 / 状态新旧判断辅助信号（**如多条 `pet.state.changed` 乱序到达时按 `ts` 排序**）"

两处都要求 client 按 `ts` 排序判断新旧，与全局 envelope 约束矛盾。iOS Story 15.x 实装无法判断 `ts` 到底能不能用作 state ordering。

### 根因（Root cause）

`ts` 字段在多个 WS 消息章节里被反复"局部赋能" —— 每个具体消息章节描述 `ts` 时容易顺手加一句"client 可用作时序排序辅助信号"，这与全局 envelope 的硬性禁用规则反向：

- §12.2 全局 envelope 章节先定 `ts` 硬性约束（"不用作业务时序判断"）
- §12.3 / §12.4 / ... 各具体消息章节描述自己的 `ts` 字段时，往往"复述 + 加细节"，描述场景"多条消息乱序到达"时直觉性加"按 `ts` 排序"
- spec 撰写者没意识到这条局部细节直接违反全局约束 —— `ts` 是 server 时钟戳，跨实例 / 跨 server 重启都可能 skew，client 不能依赖

实装层（client）按"业务消息章节优先于全局约束"理解，会按 `ts` 排序，导致 time skew 时业务逻辑出错。

### 修复（Fix）

§12.3 line 2247 / 2250 两处全部删除"按 `ts` 判断新旧" / "按 `ts` 排序" 措辞，明确写：

- line 2247："最近"由**同一 WS 连接内消息的物理到达顺序（FIFO 保证）**决定，**不**依赖 `ts` 字段做新旧比较（`ts` 是日志关联 / UI 排序辅助信号，详见 §12.2 line 1961，禁止用作业务排序判定）；reconnect 后由 `room.snapshot` 全量重新对齐 + 14.3 落地后的权威等价层兜底
- line 2250：`ts` 用途**仅限**客户端日志关联 + UI 辅助展示（如显示"X 秒前更新"），**禁止**用作业务排序 / 状态新旧判定；状态新旧判定由 (i) WS 连接内 FIFO + (ii) reconnect `room.snapshot` 全量对齐 + (iii) 14.3 权威等价层 共同兜底

### 预防规则（Rule for future Claude）⚡

> **一句话**：在全局信封 / 全局协议层已明确禁用某字段用途的前提下（如 `ts` 不用作业务时序），任何具体业务消息章节描述该字段时**必须**与全局约束硬性一致，**禁止**在局部章节给该字段"加新用途"。
>
> **展开**：
> - 标志：全局信封 / 全局协议章节有"某字段 X **不**可用作 Y 用途"硬性声明 → 检查 spec 内所有提及字段 X 的位置，确保没人"加 Y 用途回来"
> - 实操：grep 字段名 → 对每处描述判断是否与全局声明一致 → 不一致即修
> - 实装层应优先依赖"协议层物理保证"（如 WS 连接内 FIFO）+ "握手层 / reconnect 全量同步"机制做状态对齐，而非"应用层时间戳比较" —— 后者依赖时钟同步，跨实例不可靠
> - **反例**：r5 之前 line 2247 / 2250 让 client 按 `ts` 排序，与 line 1961 全局约束矛盾 → client 实装无法同时遵守 → 蒸馏后留作 few-shot 负例

## Lesson 3: 权威等价桶 / 字段集合等"群体声明"必须显式枚举字段名，不能用章节号代指，避免与同段已有的排除规则歧义

- **Severity**: low (P3)
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:610-612`

### 症状（Symptom）

§5.2 字段语义跨章节等价段：

- line 610 明确说：`§5.2 data.state` 是 ack-only 信号，**不进入**权威等价桶
- line 612（紧接着）说：Story 14.3 落地后，"四处 server → client `pet.currentState` 字段（**§5.2** / `pet.state.changed` / `room.snapshot` / `member.joined`）统一切换到权威等价层"

矛盾点：line 612 写的"§5.2"是指 `data.state`（line 610 刚说不入桶）还是别的字段？两条紧邻措辞自相矛盾。Story 15.4 实装层可能误将 HTTP ack `data.state` 提升到与 WS / snapshot 同等的信任级别（因为 line 612 看起来允许）。

### 根因（Root cause）

"群体声明"（如"四处字段"/"以下三处都"/"上述六处"）用章节号代指字段时，章节号本身可能对应多个字段（§5.2 既有 `data.state` 又有请求体 `state` 又有响应体其他字段），歧义产生：

- 撰写者写 line 612 时心里想的"§5.2"是 §5.2 章节的 `pet.currentState` 概念（其实在 §5.2 没有这个具体字段名，§5.2 只有 `state` / `data.state`）
- 读者（client 实装层 / 后续 review）读 line 612 看到"§5.2"会按字面联系到 line 610 刚说的 `data.state` → 推导出"`data.state` 也入权威桶"
- 这种"用章节号代指字段"的捷径在长 spec 里频繁出现，是 docs ambiguity 的高频陷阱

### 修复（Fix）

line 612 改写"四处 server → client `pet.currentState` 字段"为显式枚举：

```
四处 server → client `pet.currentState` 字段 —— 即
(i) `pet.state.changed.payload.currentState`
(ii) `room.snapshot.payload.members[].pet.currentState`
(iii) `member.joined.payload.pet.currentState`
(iv) `GET /rooms/{roomId}.data.members[].pet.currentState`
—— 统一切换到权威等价层；**不**包括 `POST /pets/current/state-sync` 的 response `data.state`
（该字段按 line 610 是 ack-only 信号，**永远不**进入权威等价桶，14.3 落地前后语义不变；
Story 15.4 实装层 **禁止**把 HTTP ack `data.state` 提升到与 WS / snapshot 同等的信任级别）。
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：在 spec 写"N 处字段统一遵守 X" / "群体声明"时，**必须**用「字段完整路径名（章节号.payload.字段名 或 数据类型.字段名）」显式枚举每一处，**禁止**用纯章节号（如"§5.2"）代指字段 —— 章节号通常对应多个字段，会产生歧义。
>
> **展开**：
> - 标志：spec 出现"N 处字段"/"以下 N 处"/"上述 N 处"等群体声明 → 立即检查每处是否用完整字段路径名（如 `pet.state.changed.payload.currentState`）枚举
> - 实操：写 "四处都..." 这种声明前，先把 4 个字段写出完整路径名 → 不能写完的话说明分组本身有歧义
> - 若群体声明含"例外 / 排除项"（如"四处入桶，但 X 不入"），更要把排除项的完整路径名一起列出，避免与紧邻段落矛盾
> - **反例**：r5 之前 line 612 用 `§5.2` 代指 `pet.currentState` 字段，与 line 610 `data.state` 的排除规则歧义 → 蒸馏后留作 few-shot 负例

## Meta: 本次 review 的宏观教训

r5 三条 finding 共享同一个 root cause：**"宏观语义声明"与"操作层细节规则"的脱节**。spec 在长迭代过程中：

- r3/r4 引入新的宏观语义（临时窗口优先级 / 四处字段权威等价 / `ts` 全局禁用业务排序）
- 但 client 实装层真正读的操作层规则（merge contract 数值字段第 3 条 / 具体业务消息的 `ts` 描述 / 群体字段声明的具体字段名）没被同步改写

→ **client 实装层无法兼顾两层规则**，必出 bug（reconnect 状态回退 / 按 `ts` 排序错乱 / HTTP ack 误提升信任级别）。

预防：**未来 Claude 在 spec 引入任何"宏观语义声明"后，必须立即 grep 全文相关字段名 / 概念，把宏观语义 propagate 到所有操作层措辞，并产生一份"已检查所有相关处"的 checklist 后才视为该 review 轮真正闭环**。仅在新章节加宏观声明 = 形同空文，client 实装层只读自己负责的具体规则。
