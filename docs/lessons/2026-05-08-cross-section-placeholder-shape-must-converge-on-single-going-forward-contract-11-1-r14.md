---
date: 2026-05-08
source_review: codex review round 14 of Story 11.1（接口契约最终化）
story: 11-1-接口契约最终化
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-08 — 跨章节同一 placeholder 阶段必须收敛到单一 going-forward 契约形态；既已落地实装与未来契约的差异要显式标注 backfill 路径，不留两种 shape 让 client parser 分流（11-1 r14）

## 背景

Story 11.1（接口契约最终化）codex review 第 14 轮针对 r10 后的 §10.3 五阶段过渡表 vs §12.3 `room.snapshot` placeholder 注解做"跨章节同一字段同一阶段 shape 一致性"复核。codex 指出：同一 Story 10.7 placeholder 阶段，§10.3 五阶段表说"LEFT JOIN `pets`，pet-less 下发 `pet: null`"；§12.3 `payload.members[].pet` 字段表注解仍残留"单表查询不 JOIN `pets`，可一律下发 `pet ≠ null` + `petId: ""`"的简化路径 —— 两种 shape 同时合法会让 Story 10.7 / 11.7 实装方 + iOS parser 不知该遵守哪一个。一条 P2（提示）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | §10.3 五阶段过渡表 vs §12.3 `room.snapshot` placeholder 注解对同一 Story 10.7 placeholder 阶段 `pet` 字段 shape 不一致（"`pet: null`（LEFT JOIN）" vs "`pet ≠ null + petId: ""`（单表）"两种合法形态） | P2 | docs（cross-section contract consistency） | fix | `docs/宠物互动App_V1接口设计.md` §10.3 五阶段表 + §12.3 `room.snapshot` 字段表 + placeholder 示例 + 字段值来源说明；`_bmad-output/implementation-artifacts/11-1-接口契约最终化.md` §10.3 五阶段表 + 关键解读 |

## Lesson 1: 跨章节同一 placeholder 阶段必须收敛到单一 going-forward 契约形态；既已落地实装与未来契约的差异要显式标注 backfill 路径，不留两种 shape 让 client parser 分流

- **Severity**: P2
- **Category**: docs（cross-section contract consistency）
- **分诊**: fix
- **位置**:
  - `docs/宠物互动App_V1接口设计.md` §10.3 五阶段过渡表 `pet` 整体 / `pet.petId` 行（行 1358-1359）+ 关键解读 #2（行 1366）
  - `docs/宠物互动App_V1接口设计.md` §12.1 握手成功后必发消息列表第 1 项（行 1696）
  - `docs/宠物互动App_V1接口设计.md` §12.3 `room.snapshot` 字段表 `payload.room.memberCount` / `payload.members` / `payload.members[].pet` / `payload.members[].pet.petId` 行（行 1877-1882）
  - `docs/宠物互动App_V1接口设计.md` §12.3 placeholder 示例段（caption + JSON 例子，行 1936-1974）
  - `docs/宠物互动App_V1接口设计.md` §12.3 placeholder 字段值来源说明 + 新增 "Story 10.7 落地实装与 going-forward 契约的差异" 段（行 1976-1978）
  - `_bmad-output/implementation-artifacts/11-1-接口契约最终化.md` §10.3 五阶段表 `pet.petId` 行 + 关键解读 #2

### 症状（Symptom）

同一份 V1 接口设计文档对同一 Story 10.7 placeholder 阶段 `payload.members[].pet` 字段下发形态给出**两种互不兼容的合法 shape**：

- §10.3 五阶段过渡表（r10 引入）行 1358 说：node-4 placeholder 阶段也走 LEFT JOIN `pets`，pet-less 时下发 `pet: null`（`pet 整体` 行 placeholder 列与"节点 4 真实"列同值）。
- §12.3 `payload.members[].pet` 字段注解（r10 改后残留）说：节点 4 placeholder 阶段（Story 10.7）的"LEFT JOIN 简化路径"——单表查询不 JOIN `pets`，service 实装层若无法判定该 user 是否 pet-less，可一律下发 `pet ≠ null` + `petId: ""` 占位结构（保持向后兼容，client merge contract 处理空串保留），但 Story 11.7 真实实装时必须 LEFT JOIN `pets`，pet-less 时下发 `pet: null`。

冲突影响：

- Story 10.7 实装方读 §12.3 简化路径 → 实装单表查询 + `pet ≠ null + petId: ""`（已 done，commit 3486e58）；Story 11.7 实装方如果只读 §10.3 五阶段表 → 期望 placeholder 阶段也已是 LEFT JOIN + nullable。两种实装下发给 iOS 的 `pet` 字段 shape 完全不同（pet-less 场景下，前者发 `{petId: "", currentState: 1}`，后者发 `null`）。
- iOS parser 必须同时支持两种 shape，否则 placeholder 阶段过渡过程中要么 wipe out 真实 petId（如果只按 `null` 处理逻辑），要么把"server 不知道"占位当作 authoritative 真实 petId（如果只按真实值覆盖逻辑）。

### 根因（Root cause）

**契约文档迭代时，新增 invariant（r10 加入"`pet` 全协议 nullable"）只局部更新部分章节，没有对该 invariant 在所有相关章节的 placeholder 阶段交互做一次 sweep**。具体思维漏洞链：

1. **r10 引入 nullable invariant 时聚焦在 §10.3 字段表 + 五阶段表**：r10 把"`pet` 是 nullable object，pet-less 下发 `null`"这条契约加进 §10.3 / §5.1 / §12.3 字段表的"主形态"，让所有"真实路径"都遵守。
2. **§12.3 placeholder 简化路径（pre-r10 残留）没被 sweep**：r8/r10 之前 §12.3 写的是"placeholder 阶段不 JOIN `pets`，下发 `pet ≠ null + petId: ""` 占位"——这个简化路径在 r10 加入 nullable 后**变成与五阶段表互不兼容的第二种 shape**，但 r10 没有把它收敛掉，反而留了一段"placeholder 阶段简化路径 + Story 11.7 真实实装必须 LEFT JOIN"的过渡兼容文字 —— 文字里没明说"两种 shape 都合法"，但实际语义就是这样，造成 Story 10.7 / 11.7 实装方读两段会做两种不同实装。
3. **同一阶段（Story 10.7 placeholder）跨章节定义不一致没人查**：契约文档 review 通常在单章节内做"字段表 vs 示例 vs 不变量"的 zip 校验（r6 / r12 lessons 已沉淀），但跨章节同一字段同一阶段的 shape 一致性校验没有作为 review 标准 —— r10 ~ r13 review 重点在 ACL / 锁 / step 顺序 / mermaid 同步，没专门 sweep 跨章节 placeholder shape 一致性。
4. **"placeholder 简化"是一类高风险表述**：placeholder 阶段从语义上就是"reduced shape"，但 reduced 只能在两个维度合法：(a) 未来 epic 才填充的 enrich 字段（如 `nickname` / `avatarUrl`，placeholder 阶段下发空字符串 = "我不知道"，merge contract 保留 client 已有值）；(b) 未来 epic 才驱动的状态字段（如 `pet.currentState`，placeholder 阶段固定 `1`）。**`pet` 整体的 nullable shape 不属于这两类** —— 它是一个 contract-level edge case 信号（pet-less 是 contract 内合法状态，与节点 / epic 进度无关），不该被 placeholder 简化路径退化。r10 加入 nullable 时混淆了"placeholder 简化（对 enrich 字段合法）"和"contract edge case 信号（不能被简化）"。

**蒸馏出的元教训**：契约文档每次加入新 invariant（特别是 nullable / edge case 类）时，必须做"该 invariant 在所有 placeholder 阶段 / 简化路径 / 兼容路径中是否被违反"的 sweep；否则新 invariant 只在主形态生效，placeholder 简化路径会留一个 invariant-violating 的合法形态，在跨 story 实装时引爆为 shape 分歧。

### 修复（Fix）

**路径选择**：路径 A（让 §12.3 placeholder 注解收敛到 §10.3 五阶段过渡表的 LEFT JOIN + nullable shape，Story 10.7 已落地实装作为过渡兼容形态显式标注，Story 11.7 必须 backfill 到 going-forward 契约）。

**为什么是路径 A 而非 B / C**：

- **路径 A（采用）**：让 §12.3 `payload.members[].pet` 字段注解 + placeholder 字段值来源说明 + placeholder 示例 + 五阶段表的 `pet.petId` placeholder 列 全部对齐到"LEFT JOIN `pets` + pet-less 下发 `null` + 否则 `pet ≠ null + petId: pets.id` 真实值"。这意味着 going-forward 契约下 placeholder vs 真实只有 `nickname` 一个字段差异（不 JOIN `users` 还是 JOIN `users`），是最小过渡。
  - **优点**：(i) 一种 shape，client parser 不分流；(ii) 与 §10.3 五阶段表 `pet 整体` 行 / `pet.petId` 行 placeholder/真实两列同值的现有钦定一致；(iii) `pet` 字段是 contract edge case 信号（pet-less），不该被 placeholder 退化。
  - **代价**：Story 10.7 已 done 实装走的是单表查询不 JOIN `pets`，所有 member 一律下发 `pet ≠ null + petId: ""`（包括 pet-less 成员）—— 这是 going-forward 契约的违反，但因 Story 10.7 已 done（commit 3486e58 / Epic 10 retro 完成）不回工，Story 11.7 真实 SnapshotBuilder 落地时**必须**切换到 going-forward 契约形态。client parser 在过渡期内同时收到两种 shape，但按 client merge contract 单一规则处理（空字符串 → 保留、`null` → 覆盖、非空真实值 → 覆盖）即可，**不**做"两种 shape 分流"逻辑。
- **路径 B（不采用）**：让 §10.3 五阶段过渡表 placeholder 行允许"单表不 JOIN + `pet ≠ null + petId: ""` 占位（Story 10.7 兼容路径）"。优点：保留 Story 10.7 当前实装的 spec 兼容性；缺点：iOS parser 需要支持两种 shape（pet-less 情况下，前者发 `pet ≠ null + petId: ""`，后者发 `pet: null`），契约层留两个合法形态。**与 r10 引入 nullable 的方向相反，回退**。
- **路径 C（不采用）**：在 §12.3 注解里只保留 "Story 11.7 真实实装时 LEFT JOIN + pet null"（删 placeholder 简化路径）；但加一句 "Story 10.7 placeholder 实装如果未走 LEFT JOIN，下发 `pet ≠ null + petId: ""` 视为合法过渡值；client 解析层应能同时处理两种 shape"。即明确"两种 shape 都合法 + client 必须兼容"。优点：不要求 Story 10.7 backfill；缺点：(i) 还是两种 shape 合法，client parser 仍要分流；(ii) Story 11.7 实装方如果忘记 backfill，going-forward 永远卡在两种 shape；(iii) "client 必须兼容两种 shape" 的负担永久转嫁给 client，违反"server-side 收敛 contract"原则。**比路径 A 工程负担更重**。

**改动**：

1. `docs/宠物互动App_V1接口设计.md` §10.3 五阶段过渡表（行 1359 `pet.petId` placeholder 列）：
   - 旧 "`""`（不 JOIN `pets`）" → 新 "`pets.id` 字符串化（与节点 4 真实同 —— LEFT JOIN `pets` 后 `pet ≠ null` 分支即 `pets.id` 真实值）"
   - placeholder 列与"节点 4 真实"列同值，与 `pet 整体` 行 placeholder 与真实两列同值一致
2. §10.3 关键解读 #2（行 1366）：
   - 旧 "placeholder 阶段允许 `nickname` / `pet.petId` 空字符串（不 JOIN）" → 新 "placeholder 阶段允许 `nickname` 空字符串（不 JOIN `users`）；`pet` 整体 placeholder 阶段已与真实阶段对齐（LEFT JOIN `pets`，pet-less 下发 `null`，否则 `pet.petId` 为 `pets.id` 真实值）"
   - 加 "Story 10.7 落地实装的过渡兼容" 段：明示 Story 10.7 已 done 走单表查询、Story 11.7 真实实装必须 backfill 到 LEFT JOIN
3. §12.1 握手成功后必发消息列表第 1 项（行 1696）：
   - 旧 "丰富字段 `nickname` / `pet.petId` 在 placeholder 阶段允许空字符串" → 新 "节点 4 阶段 going-forward 契约：room 三字段 + members 数组反映 `room_members` 全部成员行 + LEFT JOIN `pets` 取 pet 真实状态"
4. §12.3 字段表（行 1877-1882）：
   - `payload.room.memberCount` 行（1877）：placeholder vs 真实差异从"不 JOIN `users` / `pets`" 改为"placeholder 阶段不 JOIN `users` 故 `nickname` 降级为空字符串；`pet` 整体 going-forward 契约要求 placeholder 与真实两阶段均 LEFT JOIN `pets`"
   - `payload.members` 行（1878）：going-forward 契约改为 placeholder + 真实均 LEFT JOIN `pets`；标注 Story 10.7 落地实装的过渡兼容形态 + Story 11.7 必须 backfill
   - `payload.members[].pet` 行（1881）：删除"placeholder 简化路径"段；改为"placeholder + 真实均 LEFT JOIN `pets`，pet-less → `null`、否则 `petId` 为 `pets.id` 字符串化值；禁止单表查询 + 一律 `pet ≠ null + petId: ""` 简化"；引到 "Story 10.7 落地实装与 going-forward 契约的差异" 段
   - `payload.members[].pet.petId` 行（1882）：going-forward 契约从"placeholder 允许空字符串" 改为"placeholder + 真实均来自 `pets.id` 真实值"；标注 Story 10.7 落地实装的过渡兼容形态 + Story 11.7 backfill 路径
5. §12.3 placeholder 示例段（行 1936-1974）：
   - caption 改为"placeholder going-forward 示例"，强调 placeholder 与真实差异**仅在 `nickname`**
   - JSON 例子从 2 成员 + 都 `pet ≠ null + petId: ""` 改为 3 成员含 1 个 pet-less 边界案例（`userId: "1003"` 下发 `pet: null`），与真实示例同结构（仅 `nickname` 不同）
6. §12.3 placeholder 字段值来源说明（行 1976）+ 新增 "Story 10.7 落地实装与 going-forward 契约的差异" 段（行 1978）：
   - 字段值来源说明改为"LEFT JOIN `pets`，pet-less → `null`"（going-forward 契约）
   - 新增专门一段说明 Story 10.7 已 done 实装的过渡兼容形态，明示 Story 11.7 必须 backfill；同时明示 client 解析层不做 shape 分流，按 client merge contract 单一规则即可同时正确处理两种形态
7. `_bmad-output/implementation-artifacts/11-1-接口契约最终化.md` §10.3 五阶段表（`pet.petId` 行）+ 关键解读 #2：与主 V1 spec 同步改动

**Story 11.7 follow-up（明确标注的 tech-debt）**：

- Story 11.7 真实 SnapshotBuilder 实装时必须从 Story 10.7 的 placeholder 形态切换到 going-forward 契约形态：
  - 从单表 `SELECT * FROM room_members WHERE roomId=?` 改为 `room_members LEFT JOIN pets ON pets.user_id = room_members.user_id`
  - pet-less 时（`pets.*` 列为 NULL）下发 `pet: null`
  - 否则下发 `pet ≠ null` + `petId: pets.id` 字符串化 + `currentState: 1`
- 此切换 + INNER JOIN `users` 改 `nickname` 为真实值，是 Story 11.7 vs Story 10.7 的唯二差异
- Story 10.7 不回工（已 done + Epic 10 retro 完成）；Story 11.7 落地后 going-forward 形态自动生效，过渡期 client parser 按 client merge contract 单一规则处理两种 shape

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **契约文档加入新 invariant（特别是 nullable / contract edge case 类）** 时，**必须** **sweep 该 invariant 在所有 placeholder 阶段 / 简化路径 / 兼容路径中是否被违反，把违反的合法形态收敛到主形态**；不允许"主形态遵守 + placeholder 简化路径有自己的 shape" 这种"局部 invariant" 状态长期存在。
>
> **展开**：
>
> - **invariant 必须全局生效**：契约层 invariant 是"对所有 conforming 实装的硬约束"，不存在"主形态遵守 + 简化路径例外" 的 invariant —— 那叫两种合法形态，不叫 invariant。如果某个 placeholder 阶段实装无法满足 invariant，要么 (a) 升级 placeholder 让它满足（如本 case 路径 A），要么 (b) 撤回 invariant（不再 nullable），要么 (c) 把违反的 placeholder 形态显式标注为"transitional shape，going-forward 契约不承认它，由具体 story 在具体时点 backfill 到主形态"。**不能含混"两者都合法"**。
> - **placeholder 简化只对两类字段合法**：(a) 未来 epic 才填充的 enrich 字段（如 `nickname` / `avatarUrl`，简化为空字符串 = "server 不知道"，merge contract 保留 client 已有值）；(b) 未来 epic 才驱动的状态字段（如 `pet.currentState`，简化为固定值）。**Contract edge case 信号字段（如 pet-less 的 `pet: null`、currentRoomId 的 `null`）不能被 placeholder 简化退化** —— edge case 是 contract-level 状态，不是 epic 进度状态。
> - **跨章节同一字段同一阶段 shape 必须一致**：契约文档 review 不只做单章节内"字段表 vs 示例 vs 不变量" zip 校验（r6 / r12 已沉淀），还要做**跨章节同一字段同一阶段** shape 一致性 sweep。具体清单：
>   1. grep 所有提到该字段名的章节（如本 case 的 `pet` / `payload.members[].pet`）
>   2. 对每个 stage（placeholder / 真实 / future epic）列出该字段在每个章节的 shape
>   3. 同 stage 同字段不同章节给出不同 shape → 立即收敛
> - **新增 invariant 时的 sweep checklist**：
>   1. 该 invariant 是否在主形态字段表都体现？
>   2. 该 invariant 是否在所有 JSON 示例（真实示例 + placeholder 示例 + edge case 示例）都覆盖？
>   3. 该 invariant 是否在所有 placeholder 简化路径 / 兼容路径中也成立？如不成立 → 路径 A（升级 placeholder）/ B（撤回 invariant）/ C（显式标注 transitional 兼容）三选一，**不留隐式 "两者都合法"**
>   4. 该 invariant 是否在所有 mermaid sequenceDiagram inline payload / 示例代码 / 关键解读段都同步？
>   5. 该 invariant 是否在 sprint planning artifact（story spec）/ story 文件里同步引用？
> - **既已落地实装 vs going-forward 契约的差异处理**：当契约改动 retroactively invalidates 已落地 story 实装时，三选一：
>   1. 回工已 done story（重 epic 决策，违反"已 done 不改"纪律 + retro 已完成）
>   2. 回退契约（如本 case 不可，因为 nullable invariant 已是 going-forward 共识）
>   3. **显式标注"transitional 兼容"**：在 spec 文档里专门写一段"<old-story> 落地实装与 going-forward 契约的差异"，明示 (a) 旧实装是什么形态、(b) going-forward 契约是什么形态、(c) 哪个未来 story 必须 backfill、(d) 过渡期 client / consumer 如何兼容两种形态。**不允许只在某个字段注解里夹带 "Story X 必须 LEFT JOIN" 一句话** —— 这种文字 review 时容易被忽略，专门段落 + 引用锚点更稳。
> - **client merge contract 是 shape 收敛的最后防线，但不是借口**：本 case 中 Story 10.7 落地实装的 `pet ≠ null + petId: ""` 形态恰好被 client merge contract 的"空字符串 → 保留 client 已有值"路径兜住，所以 client 不会 wipe out 真实 petId。但这个兜底**不是**保留两种 shape 的理由 —— 兜底只是过渡期的 safety net，going-forward 契约必须收敛到单一形态。如果未来类似改动的字段没有 client merge contract 兜底（如纯数值字段、或 contract 信号字段），两种 shape 就会直接 bug。
> - **反例 1**：r10 引入 `pet` nullable 时，只更新 §10.3 / §5.1 / §12.3 字段表的"主形态"行，没 sweep §12.3 placeholder 字段值来源说明的 "placeholder 简化路径"段。结果该段保留了 r10 之前的"`pet ≠ null + petId: ""`"形态，与新 invariant 互不兼容，r14 才被 codex 抓出。
> - **反例 2（特别危险）**：在 spec 文档里写"placeholder 阶段允许 X 简化（保留向后兼容），但 Story Y 真实实装时必须 Z" —— 这种表述有两层语义：(a) "Story Y 之前 X 简化合法"（默认存在两种合法形态）；(b) "Story Y 后必须 Z"（going-forward 收敛）。表述模糊导致 Story Y 实装方可能读 (a) 不读 (b)，留两种合法形态长期共存。**修正模板**：把这种表述拆成两段 —— "going-forward 契约：Z（适用于所有 conforming 实装）" + "<old-story> 落地实装的过渡兼容形态：X（已 done 不回工，Story Y 必须 backfill 到 Z）"，明确划清 going-forward vs transitional 边界。
> - **反例 3**：依赖 client / consumer 处理两种 shape。"client 解析层应能同时处理 X 和 Y" 这种话出现时，要立刻问"为什么不让 server 收敛到一种 shape？"。除非确认 (i) 改 server 不可能（已落地 + 不回工） + (ii) 两种 shape 在 client 兜底下行为等价（如本 case 的 merge contract 兜底） + (iii) 显式标注 going-forward 契约形态 + 具体 story backfill 时点，否则不允许"client 兼容两种 shape"作为 spec 决策。
