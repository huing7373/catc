---
date: 2026-05-12
source_review: /tmp/epic-loop-review-14-1-r4.md (codex review r4)
story: 14-1-接口契约最终化
commit: df8c2a3
lesson_count: 3
---

# Review Lessons — 2026-05-12 — self-broadcast UI 驱动语义在 §5.2 / §12.3 必须对称一致 & 冻结边界声明必须区分"抽象触发条件"与"可调阈值" & 临时窗口优先级表必须显式标注 self vs 他人 entry 适用范围（14-1 r4）

## 背景

本 lesson 来自 Story 14.1（接口契约最终化）codex review 第 4 轮。前 3 轮（r1 / r2 / r3）已经把 `state-sync` 接口契约、`pet.state.changed` 字段表、跨章节字段等价层声明、self-broadcast 兜底规则、`member.joined` stale `1` 风险等议题逐一收敛。r4 三条 finding 都是**多次迭代后的余量措辞 misalignment** —— 两处章节同写一个语义但措辞跑偏。这类长契约文档的"双源声明"是个反复触发陷阱：r4 三条 finding 都是 §5.2（接口侧）与 §12.3（WS 侧）写同一规则时一处先迭代到位、另一处遗留旧措辞。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联位置 |
|---|---|---|---|---|---|
| 1 | §12.3 line 2244 self-broadcast UI 驱动语义与 §5.2 line 547-551 矛盾（"仅 ack/活性探测"否定了 WS-first 路径的 UI 驱动职责） | P2 (medium) | docs | fix | `docs/宠物互动App_V1接口设计.md:2244` |
| 2 | §1 line 49 把 1005 触发条件纳入冻结范围，但 §5.2 line 595 说 60/min 阈值"配置可调"，冻结边界不清 | P3 (low) | docs | fix | `docs/宠物互动App_V1接口设计.md:49` |
| 3 | §12.3 line 2247 临时窗口优先级排序删了 §5.2 line 612 的 HTTP `data.state` 那层，client 实装无法据此区分 self/他人 entry 路径 | P3 (low) | docs | fix | `docs/宠物互动App_V1接口设计.md:2247` |

## Lesson 1: §12.3 self-broadcast UI 驱动语义必须与 §5.2 对称到达顺序规则一致，不能在另一处章节遗留"self-broadcast 仅作 ack/活性探测，不作本地 UI 唯一来源"的旧措辞

- **Severity**: medium (P2)
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:2244`

### 症状（Symptom）

§5.2 line 547-551 已迭代到位：明确写"HTTP 200 **或** self-broadcast 先到任一信号即立即更新 self entry，后到信号 no-op"，并展开 (a) HTTP 先到 / (b) WS 先到 / (c) 对称无操作不变量三条规则。但 §12.3 `### 宠物状态变更` 关键约束第 1 条（line 2244）仍写老版本："发起者本地 UI 由 HTTP 200 立即驱动，self-broadcast 作 (a) 跨设备一致性校验 + (b) WS 链路活性探测，**不作发起者本地 UI 唯一来源**" —— 这条措辞**直接否定了** §5.2 (b) 路径"WS self-broadcast 先到时立即驱动 UI"的合法性。两处契约自相矛盾，client 实装层（Story 15.4）无法判断到底 WS 先到时要不要更新自己。

### 根因（Root cause）

长 spec 文档里"同一规则在两处章节并行声明"是反复触发的措辞漂移陷阱：

- r1 / r2 阶段先迭代了 §5.2（接口章节）的 self-broadcast 兜底规则，反复打磨到 line 547-551 三段细化展开
- §12.3（WS 消息章节）line 2244 的关键约束第 1 条**指向** §5.2 说明（"详见 §5.2"），但**自己也写了一句独立结论**总结对外 contract："不作发起者本地 UI 唯一来源"
- 当 §5.2 自身的结论从"仅 ack" 演化为"对称到达顺序"时，§12.3 的总结句没有同步更新 → 两条措辞产生方向相反的硬性结论
- 反映出更深层的文档结构问题：**不应在两处章节同时写"硬性结论句"**，应该让一处是 single source of truth，另一处只写"详见 X 章节"链接而不复制结论

### 修复（Fix）

把 §12.3 line 2244 的 self-broadcast UI 驱动总结句改成与 §5.2 line 547-551 完整对称：

before：
> 发起者本地 UI 由 HTTP 200 立即驱动，self-broadcast 作 (a) 跨设备一致性校验 + (b) WS 链路活性探测，不作发起者本地 UI 唯一来源

after：
> 任一路径先到的信号（HTTP 200 或 self-broadcast）都立即驱动本地 self entry 的 roster pet state 更新，后到的信号按字段级 merge 走 no-op 路径 … self-broadcast 的剩余职责仅在它是后到信号时生效（跨设备一致性校验 / WS 链路活性探测）

并把 single source of truth 总结改为："对自己 → HTTP 200 与 self-broadcast 任一先到即驱动 UI（对称 no-op 不变量）"，与 §5.2 line 547-551 (c) 项对称无操作不变量声明完全一致。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **同一规则在两个章节并行声明（X 接口章节 + Y WS 消息章节，或 X spec + Y code comment 等"双源声明"场景）** 时，**禁止在两处都写硬性结论句**，必须**只在一处写完整结论，另一处只写"详见 X 章节 line N"的纯引用链接，不复制结论句**。
>
> **展开**：
> - 反例 1（本次踩坑）：§5.2 写"任一信号先到都驱动 UI"，§12.3 又自己总结一句"不作发起者本地 UI 唯一来源" —— 第一句迭代后，第二句的方向硬性反着，client 无法判断到底听谁
> - 反例 2（一般化）：spec 写"必须用 idempotency key"，handler 注释又写"幂等性由 DB 唯一索引兜底" —— 实装时该用哪条？
> - 正解：在每次迭代规则结论时，**grep 一次本文档**找出所有引用该规则的章节，确认所有引用只写"详见 X" 链接，不复制结论句；若某处必须重写一遍（如 contract 文档需要 self-contained），**复制完整规则**而**不**只复制半句结论 —— 半句结论必然会随源处迭代而漂移
> - 元规则：长契约文档里"双源声明"是次结构性问题，**single source of truth + 纯引用链接**是正解，发现两处都写硬性结论句要警觉 —— 这是迭代过程中 90% 会产生漂移的反模式
> - 触发条件：契约 / spec 文档迭代时，**只要修了某条规则的核心结论**，必须在文档里 grep 该规则关键词（如 self-broadcast / freeze / authority signal）找出所有提及位置，确认要么全部同步要么改写成"详见"链接形式

## Lesson 2: 冻结声明必须区分"抽象触发条件层"与"可调阈值层" —— 不能笼统说"错误码触发条件冻结"而让契约消费方误以为阈值常量也冻结

- **Severity**: low (P3，但措辞清晰度直接影响后续 epic 回归 scope 识别)
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:49`

### 症状（Symptom）

§1 接口契约冻结声明 line 49 把 `1005` 的"触发条件"列入 Story 14.1 冻结范围，但 §5.2 错误码表 line 595 又写"每用户每分钟 > 60 次；按 Story 4.5 默认值，**配置可调**"。两条措辞硬冲突：

- 如果阈值（60/min）可调 → 那 1005 的实际触发条件就不是冻结契约（同一接口在不同部署 / 不同环境下触发条件不同）
- 如果触发条件真冻结 → 那 60/min 也应被视为契约常量不能再配
- 不修措辞 → 后续 epic 回归 1005 行为时无法判断"调整阈值"是否需要走"§1 冻结流程 4 步"

### 根因（Root cause）

"冻结某错误码的触发条件"这条措辞天然有**抽象层歧义**：

- 抽象层（中间件级）："被 rate_limit 中间件按 `user_id` 维度拦截时返回 1005" → 这是真正的接口契约不变量
- 具体阈值层（参数级）："每分钟 60 次" → 这是 Story 4.5 默认值 + 配置层管理的运维参数
- 写"触发条件冻结"时如果不指明是哪一层，默认读者会按字面理解为"具体阈值也冻结"——但实际工程中 rate limit 阈值往往需要按流量调优，不可能真的契约级冻结

### 修复（Fix）

§1 line 49 的"错误码触发条件冻结"措辞内联补一条"冻结边界说明"，显式声明：

> 1005 的"触发条件"冻结在**抽象层**（走通用 rate_limit 中间件按 `user_id` 维度限频拦截）这一不变量；**具体阈值**（如 60/min）由 Story 4.5 默认值 + 配置层管理，调整阈值**不**视为本接口契约变更，**不**触发下文 1-4 步流程；删除限频中间件 / 切换限频维度 / 把 1005 改成抛其他错误码才视为契约变更

这样后续 epic 回归识别 scope 时一眼就能判断："调 60 改 100" → 不需要触发 14.1 冻结流程；"把 user_id 维度改成 IP 维度限频" → 需要触发。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **声明某契约元素（错误码触发条件 / 限频 / 重试 / 超时）"冻结"时**，**必须显式指明冻结的是"抽象层"还是"具体参数层"**，绝不能笼统说"X 的触发条件冻结"而不区分。
>
> **展开**：
> - 反例 1（本次踩坑）：把 "1005 触发条件冻结" 写进契约，没指明"是冻结中间件机制还是冻结 60/min 阈值"
> - 反例 2（一般化）：声明 "超时行为冻结"，但没说"是冻结 timeout 错误码语义还是冻结 30s timeout 时长"
> - 正解模板：冻结声明必须分两层写
>   - 抽象不变量层：被 X 中间件按 Y 维度拦截 / 超时后抛 Z 错误码
>   - 可调参数层：具体阈值 / 时长 / 重试次数 在配置层管，调整**不**触发契约变更流程
> - 元规则：契约文档冻结的"是行为本身的存在性 + 触发抽象路径"，**不**是"行为的具体参数值"；后者天然属于运维参数，与"契约消费方需要据此实装"无关
> - 触发条件：写 "X 进入冻结状态" / "Y 修改需要触发回归" / "Z 属契约一部分" 之类的句子时，**必须**自问 "Z 是抽象机制还是具体参数？"，若 Z 是参数（阈值 / 时长 / 数量等），措辞必须显式标注"具体值不在冻结范围"，否则后续 epic 回归无法识别 scope

## Lesson 3: 临时不一致窗口的"权威信号优先级表"必须显式标注 self entry vs 他人 entry 适用范围 —— 因为两者在 client 端可见的信号集本质不同

- **Severity**: low (P3，但 client 实装会按错误的优先级做差异化处理)
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:2247`

### 症状（Symptom）

§5.2 line 612 给出的 Story 14.3 落地前临时窗口权威信号优先级表是 4 层：

```
pet.state.changed > state-sync HTTP data.state > room.snapshot / GET / member.joined
```

而 §12.3 line 2247 同一临时窗口的优先级表只有 3 层（删掉了 HTTP `data.state`）：

```
pet.state.changed > room.snapshot / GET / member.joined
```

两套不同排序让 client 实装层无法判断该按哪个走。本质原因：§5.2 优先级表是站在 self entry 视角（自己上报状态时本端 client 可见 HTTP 200 + self-broadcast 两路信号），§12.3 优先级表是站在他人 entry 视角（他人状态变化时本端 client 只可见 WS 信号，没有 HTTP 信号承载他人状态），但两处都没标注适用范围。

### 根因（Root cause）

权威信号优先级表"在 self entry 与他人 entry 之间天然不同"这件事是 r1-r4 整个迭代的核心议题之一，但措辞层一直把"信号源差异"和"优先级排序"混在一起表达：

- self entry 在本端 client 视角下**有 HTTP 200 + self-broadcast 两路信号**（HTTP 是 caller 自己的请求响应，承载自己的状态）
- 他人 entry 在本端 client 视角下**只有 WS 信号**（caller 自己的 HTTP 响应不承载别人的状态）
- 因此 HTTP `data.state` 这一层**只有 self entry 有**，他人 entry 的优先级表里不应该出现 HTTP 层
- 但 §5.2 和 §12.3 两处优先级表都没显式标注"这里说的是哪种 entry"，§5.2 落入"self 视角"，§12.3 落入"他人视角"，措辞相互打架而不是相互补充

### 修复（Fix）

在 §12.3 line 2247 临时窗口优先级表述末尾追加一段显式标注：

> 该优先级排序适用于他人 entry；对 self entry 的 HTTP `data.state` ack 信号详见 §5.2 "WS 广播 vs HTTP 响应的关系（含发起者自己的 self-broadcast 兜底规则）" + §5.2 line 612 的 self-only 优先级（`pet.state.changed` WS 广播 > `state-sync` HTTP `data.state` (ack) > `room.snapshot` / GET / `member.joined`）—— self entry 与他人 entry 走不同路径：self entry 有 HTTP 200 + self-broadcast 两路信号到达本端 client，HTTP `data.state` 是 server-acknowledged ack 兜底信号；他人 entry 在 client 端没有对应的 HTTP 信号（HTTP 是 caller 自己的请求响应，不承载别人状态），因此他人 entry 优先级表不含 HTTP 层

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写"权威信号优先级表"** 时（或任何形如 "X 信号 > Y 信号 > Z 信号" 的排序声明），**必须**先明确**适用范围**（哪类 entry / 哪类对象 / 哪类 caller 视角），不能笼统给出排序而不标注"在什么场景下这个排序成立"。
>
> **展开**：
> - 反例 1（本次踩坑）：§5.2 给 4 层排序（含 HTTP），§12.3 给 3 层排序（无 HTTP），两处都没说"这是 self entry 视角 / 他人 entry 视角"，client 无法决策按哪个走
> - 反例 2（一般化）：写 "cache > DB > 远程 API"，但没说是 "read path 还是 write path"，实装时 write path 误用这个排序导致 cache 写不到 DB
> - 正解模板：优先级表声明必须先一句话标注适用范围
>   - "对 **self entry**，权威信号优先级为：HTTP 200 > self-broadcast > room.snapshot"
>   - "对 **他人 entry**，权威信号优先级为：pet.state.changed > room.snapshot / GET / member.joined"
>   - 两条排序并列声明而不是合在一句话里
> - 元规则：**信号源集合**取决于观察者视角（caller 自己 vs 别人），**优先级排序**只在固定信号源集合内才有意义；跨视角直接对比排序号是无效语义
> - 触发条件：写 "A > B > C" 形式的排序时，自问"这个排序在所有场景下都成立吗？"，若答案是"取决于观察者视角"（self vs 其他 / read vs write / sender vs receiver 等），**必须**显式标注适用范围，否则不同视角下的读者会得出相反结论

## Meta: 本次 review 的宏观教训（r4 三条 finding 的共同根因）

r4 的三条 finding 在表面上是三个独立的措辞 misalignment，但深层都指向**长契约文档里"双源声明"导致的迭代漂移**：

- finding 1：§5.2 vs §12.3 写同一规则（self-broadcast UI 驱动），一处迭代到位另一处遗留旧措辞
- finding 2：§1 vs §5.2 写同一概念（1005 是否冻结），一处说"冻结"另一处说"可调"，没明确冻结层级
- finding 3：§5.2 vs §12.3 写同一排序（临时窗口优先级），两处分别站在不同视角写但都没标注适用范围

**根因汇总**：长契约文档（本文 2800+ 行）的迭代过程中，**同一规则在多处声明是次结构性问题，不是文档风格问题**。每次修改一处声明，**必须**自动假设"这条规则在文档其他地方也有声明"，必须 grep 全文确认所有引用点。

**给未来 Claude 的元规则**：在长 spec 文档（500 行以上）写或改 contract 类规则时，**每次修订规则结论前**先执行：

1. grep 本规则的关键词（错误码名 / 字段名 / 规则名）找所有出现位置
2. 对每个出现位置判断：是 SSOT（写完整规则）还是引用点（"详见 X"链接）
3. 若发现多个 SSOT（两个地方都写完整规则） → 改成 1 个 SSOT + N 个引用点，删除其他 SSOT 的硬性结论句
4. 若发现引用点写了硬性结论句而不只是链接 → 改成纯链接 + 删除独立结论
5. 修订完成后再 grep 一次确认所有引用点都同步指向最新 SSOT

这五步是长契约文档防漂移的**机械步骤**，不依赖人类记忆 / 不依赖 review 兜底；review 兜底 4 轮（r1-r4）才把同一类问题清出来已经说明 ad-hoc 修改的低效，本 lesson 落地后下次 spec 改动应直接套这套五步流程。
