---
date: 2026-05-12
source_review: codex review (/tmp/epic-loop-review-14-1-r8.md) — Story 14.1 第 8 轮
story: 14-1-接口契约最终化
commit: 97bacf9
lesson_count: 2
---

# Review Lessons — 2026-05-12 — story 文件本身必须与 frozen V1 doc 同步：self-broadcast 对称兜底语义 + ts 业务排序禁令 + 三处等价分两层必须在 story 描述章节也复述一致

## 背景

Story 14.1 接口契约最终化第 8 轮 codex review。前 7 轮反复打磨 V1 doc §5.2 / §12.3 锚定字段，r2-r4-r5-r6 把 self entry UI 驱动从"WS 广播唯一权威 / HTTP 200 仅作 ack"切换为"基于到达顺序对称无操作"（HTTP 200 与 self-broadcast 任一先到都立即驱动 self entry，后到的 no-op）；r5 把"三处完全等价 + ts 排序辅助"升级为"等价分两层（值域 / 权威）+ 14.3 前置窗口 + ts 全局禁用业务排序"。但 r1-r7 的修复**仅作用于 V1 doc**，story 文件（`_bmad-output/implementation-artifacts/14-1-接口契约最终化.md`）里 V1 doc 编辑指令 fenced block + AC / Task / Dev Notes / 关键设计点 / 下游 story 引用等描述章节**仍是旧措辞**。r8 codex 对照 frozen V1 doc 与 story 文件后判定 story 文件本身严重 drift：若 Story 15.4 / 15.2 / 14.2 / 14.4 实装工程师只看 story 文件（不点开 V1 doc），会得到与 frozen 契约完全相反的实装指引（self UI 永久 stale / 用 ts 做业务排序 / 三处 placeholder 反推权威态）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | story 文件 self-broadcast 语义未与 V1 doc r2-r4 同步：仍写"WS 广播唯一权威 / HTTP 200 仅作 ack" | high (P1) | docs | fix | `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md` |
| 2 | story 文件三处等价 + ts 排序语义未与 V1 doc r5 同步：仍写"完全等价"+"按 ts 排序判定新旧" | medium (P2) | docs | fix | `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md` |

## Lesson 1: 改契约 doc 时如果同 story 的 story 文件里嵌入了 doc 编辑指令的 fenced block（"V1 doc 这一段应改为如下"），doc 改了 fenced block 不跟改 = story 文件成 stale source of truth

- **Severity**: high (P1)
- **Category**: docs（跨文件一致性）
- **分诊**: fix
- **位置**: `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md` line 22-23（下游 iOS Story 15.2 / 15.4 引用段）+ line 127（V1 doc §5.2 "WS 广播 vs HTTP 响应的关系" 编辑指令 fenced block）+ line 154（V1 doc §5.2.5 设计权衡 blockquote）+ line 178（V1 doc §5.2 关键约束 "HTTP 200 vs WS 广播的端到端语义"）+ line 222 / 226 / 227（V1 doc §12.3 关键约束 broadcast range / userId == self / 广播 fire-and-forget recovery）+ line 399（关键设计点 #6 HTTP 200 vs WS 广播分离）

### 症状（Symptom）

V1 doc §5.2 line 549-555 经 r2-r4 多轮修复后已锁定：

> 对"发起者自己的状态变化"的权威信号（self-broadcast 例外，基于到达顺序的对称无操作）：考虑到 self-broadcast 是 fire-and-forget 不重试、若该唯一信号丢失会让发起者 UI 永久 stale，契约层允许发起者在收到 HTTP 200 OK 或 self-broadcast WS 消息（取先到的任一信号）后立即更新自己的本地 roster pet state，不等另一路信号到达。(a) HTTP 200 先到 / (b) WS self-broadcast 先到 / (c) 对称无操作不变量。

但 story 文件中的描述章节（iOS Story 15.2 / 15.4 引用段、关键设计点 #6、AC 验证清单）+ V1 doc 编辑指令的 fenced block（line 127 / 154 / 178 / 222 / 226-227）**仍是 r1 旧措辞**：

- "client 实装层**应**优先信任 WS 广播作为状态切换的最终一致信号"
- "HTTP 响应仅作为 ack 用途 / HTTP 200 → 仅作 ack 不重复更新 roster"
- "让 server WS 广播成为状态切换的最终一致路径"
- "client 收到 200 OK 后**应**信任 server 广播作为状态切换的最终一致路径，**不**应在 client 本地立即写一份 state"

两份文档语义完全相反：V1 doc 说 "HTTP 200 也是 self entry UI 驱动信号，先到即驱动"；story 文件说 "HTTP 200 不驱动 UI，只 ack，等 WS"。若 Story 15.4 工程师只看 story 文件实装，self entry UI 在 self-broadcast 丢失时永久 stale，违反 frozen contract。

### 根因（Root cause）

**story 文件嵌入了 V1 doc 编辑指令的 fenced block（"V1 doc §5.2 应替换为如下内容"）+ 大量下游 story 引用段（iOS Story 15.2 / 15.4 / 15.5 实装应如何对照 14.1 锚定）+ 关键设计点段（HTTP 200 vs WS 广播分离）**。这些描述段在 dev 阶段第一轮 r1 落地时与 V1 doc 完全一致；但 review 循环 r2-r6 中只修 V1 doc 一份文件，**没把 story 文件视为"contract 的二级镜像"也跟改**。story 文件在 BMAD 流程里既是 AC checklist 又是 dev 实装指引，工程师 navigate path 经常是"读 story 文件的 V1 doc 编辑指令 → 不开 V1 doc 直接照编辑指令推断契约语义"。一旦 r2-r6 在 V1 doc 上做了 substantive 修改而不同步 story 文件，story 文件就成了 stale source of truth。

更深层：契约 story 的修订脑模型默认"frozen doc = 唯一权威；story 文件只是定稿过程记录，可以漂移"。但 BMAD 流程实际把 story 文件留作下游 story（14.2 / 14.4 / 15.x）的 dev 引用源（"按 14.1 锚定的 schema 实装"），如果 story 文件本身 drift，下游 dev sub-agent 拿到 stale schema 执行，最终代码就违约。

### 修复（Fix）

story 文件 `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md` 中所有"self-broadcast 仅 ack / HTTP 200 仅 ack" 旧措辞重写为"基于到达顺序对称无操作"：

- **line 22**（iOS Story 15.2 引用段）：把"client 收到后既可选择更新自己 roster entry 也可选择忽略 ... 应选择'更新本地 roster'路径以保持 single source of truth"改写为"client **禁止**仅因 `userId == self` 而丢弃消息 —— 对 self entry 走 §5.2 self-broadcast 兜底规则的字段级 merge 路径（(a) HTTP 200 先到 → self-broadcast 到达 merge no-op；(b) self-broadcast 先到 → 立即驱动 self entry UI 更新）；保留 self-broadcast 作跨设备一致性校验 + WS 链路活性探测信号"
- **line 23**（iOS Story 15.4 引用段）：把"client 收到 200 OK 后**应**信任 server 广播作为状态切换的最终一致路径，**不**应在 client 本地立即写一份 state 而不等 server 广播"改写为"client 收到 200 OK 后**应**按 §5.2 self-broadcast 兜底规则（基于到达顺序对称）立即用 `response.data.state` 更新本地 self entry，**不等** self-broadcast 到达"
- **line 127**（V1 doc §5.2 "WS 广播 vs HTTP 响应的关系" fenced block）：把"client 实装层**应**优先信任 WS 广播作为状态切换的最终一致信号 ... HTTP 响应仅作为 ack 用途"重写为分两条 bullet："对'别人的状态变化' → WS 唯一权威" + "对'发起者自己' → 基于到达顺序对称无操作 (a)/(b)/(c) 完整展开"
- **line 154**（V1 doc §5.2.5 设计权衡 blockquote）：在原 "回显入参 vs 真实态选择" 后**补**"HTTP `data.state` 的 self entry UI 驱动职责：依据 V1 doc §5.2 line 549-555 的对称兜底规则，HTTP `data.state` 在 (a) 路径中承担 self entry UI 驱动职责 ... 该规则仅对 self entry 生效"
- **line 178**（V1 doc §5.2 关键约束 "HTTP 200 vs WS 广播的端到端语义"）：把"收到 WS 广播 → 更新 roster（包括自己的）；收到 HTTP 200 → 仅作 ack 不重复更新 roster"重写为"**对 self entry**：任一信号先到都立即驱动；**对他人 entry**：仅以 WS 广播为唯一权威信号驱动"
- **line 222**（V1 doc §12.3 关键约束 broadcast range / single source of truth）：在 broadcast range 段保留"包含发起者自己"+原因 (a)/(b) 后**补**"发起者自己的 self-broadcast UI 驱动职责（基于到达顺序对称）见 §5.2 line 549-555 (a)/(b)/(c) 完整对称展开 + self-broadcast 剩余职责仅在它是后到信号时生效"
- **line 226**（V1 doc §12.3 关键约束 userId == self）：把"client **应**正常处理（更新自己的 roster entry pet state ... 避免 ... 状态闪烁问题）"重写为"client **应**正常接收 + 走字段级 merge ... 按 §5.2 self-broadcast 兜底规则：(a) HTTP 200 先到 → merge no-op；(b) self-broadcast 先到 → 立即驱动 + 后续 HTTP 200 no-op"
- **line 227**（V1 doc §12.3 关键约束 broadcast fire-and-forget recovery）：在原 "(a) 下次状态切换时再次广播 (b) WS 重连后 room.snapshot 全量重新下发" 后**补**"对自己的状态（self-broadcast 丢失场景）由 §5.2 self-broadcast 兜底规则对称展开覆盖 —— 若 HTTP 200 先到达（典型）则发起者本地 UI 由 HTTP 200 立即驱动 ... 房间内其他成员对该发起者的状态视图若 self-broadcast 丢失则需走 (a) 兜底路径（无 HTTP 替代信号）"
- **line 399**（关键设计点 #6 HTTP 200 vs WS 广播分离）：标题升级为"基于到达顺序的对称无操作"+ body 重写为"对 self entry：任一信号先到都立即驱动 + no-op；对他人 entry：仅以 WS 广播为唯一权威"

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **修 frozen V1 doc / 跨章节契约 doc** 的同 story review 循环中（r2 / r3 / ...）做实质语义修改时，**必须**同步检查 + 修改 **同一 story 文件**（`_bmad-output/implementation-artifacts/<story>.md`）里所有引用同一契约段的 fenced block + 下游 story 引用段 + 关键设计点段 + AC 验证清单段。
>
> **展开**：
> - story 文件不是"定稿过程记录、可以漂移"；story 文件是 BMAD 流程里下游 dev sub-agent（14.2 / 14.4 / 15.x）的**实装一手引用源**。下游 dev 默认 navigate path 是"读 story 文件的 AC + 关键设计点 → 不打开外部 frozen V1 doc 直接照 story 推断契约"。
> - 改 V1 doc / frozen doc 的修订 commit 之前**必须** `Grep` story 文件全文，找所有 (i) 嵌入的 doc 编辑指令 fenced block；(ii) 下游 story（如 iOS Story 15.x）引用段；(iii) 关键设计点 / AC / Task 描述段；(iv) AC 验证清单（已 ✅ 项也要重写）；(v) Change Log entry，把所有重复描述同步修订。
> - 单 review 循环若改了 V1 doc 但**没**改 story 文件 = drift；下次 codex review 会用 frozen V1 doc 当 ground truth 把 story 文件本身列为新的 P1 finding（本轮 r8 就是该情况）。
> - **反例**：r2 把 V1 doc §5.2 self-broadcast 切到"基于到达顺序对称无操作"，但只 commit `docs/宠物互动App_V1接口设计.md` + lesson md，没改 `14-1-接口契约最终化.md`。结果 r3 / r4 / r5 / r6 / r7 都没扫到这个 drift（每轮 codex 都聚焦 V1 doc 自身一致性），直到 r8 显式抽 story file vs V1 doc 对照检查才浮出。

## Lesson 2: 三处等价 + ts 排序的语义升级（r5 在 V1 doc 完成）必须同步到 story 文件的 AC 章节 + 下游 story 引用段，否则 14.2 / 14.4 / 15.x dev 工程师按 story 文件写出 ts-based 业务排序代码

- **Severity**: medium (P2)
- **Category**: docs（跨文件 + 跨章节一致性）
- **分诊**: fix
- **位置**: `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md` line 225（V1 doc §12.3 关键约束 `payload.currentState` 四处等价）+ line 228（V1 doc §12.3 关键约束 `ts` 字段语义）+ line 289（AC7 字段语义跨章节一致性"五处等价"声明）+ line 522（AC7 验证清单 ✅）

### 症状（Symptom）

V1 doc §12.3 line 2252 + 2255 经 r5 修复后已锁定：

> `payload.currentState` 字段语义与 `room.snapshot.payload.members[].pet.currentState` / `GET /rooms/{roomId}.data.members[].pet.currentState` / `member.joined.payload.pet.currentState` 的等价分两层 + 受 Story 14.3 前置条件约束：(a) 值域 / DB 来源等价（恒成立）；(b) 权威 / client 信任层等价（自 Story 14.3 起成立）。Story 14.3 落地前的临时窗口 ... 仍固定返回 `1`；client 实装层在该窗口内的权威信号优先级为 `pet.state.changed` WS 广播 > `room.snapshot` / GET / `member.joined`。
> 
> `ts` 字段用途仅限客户端日志关联 + UI 辅助展示，禁止用作业务排序 / 状态新旧判定。状态新旧判定由 (i) 同一 WS 连接内消息的物理到达顺序（FIFO 保证）+ (ii) reconnect 时 `room.snapshot` 全量重新对齐 + (iii) Story 14.3 落地后的权威等价层共同兜底。

但 story 文件 line 225 / 228 / 289 / 522 仍是 r1 旧措辞：

- line 225: "`payload.currentState` 字段语义与 `room.snapshot` / `GET /rooms/{roomId}` **完全等价**（自 Story 14.3 起三处都读真实值，不再固定 `1`），client 应以最近一次 WS 广播为最新真相源（**按 `ts` 字段时间戳判断新旧**）"
- line 228: "client 可用作时序排序 / 状态新旧判断辅助信号（**如多条 `pet.state.changed` 乱序到达时按 `ts` 排序**）"
- line 289 + 522: AC7 "五处字段语义**完全等价**"无分层 + 无 14.3 前置条件 + 无 ts 业务排序禁令

若 Story 14.4 dev 工程师按 story 文件 line 228 实装，会写出"多条 pet.state.changed 乱序到达时按 ts 排序"的客户端逻辑 —— 违反 V1 doc line 2255 业务排序禁令 + time skew 实际可能存在让排序结果错误。若 Story 14.2 / 14.4 / 15.x 工程师按 story 文件 line 225 把"三处完全等价"当 invariant 信任，会在 14.3 落地**前**临时窗口用 placeholder `1` 覆盖 `pet.state.changed` 广播的 2/3 真实值。

### 根因（Root cause）

r5 在 V1 doc 上升级"完全等价 → 分两层 + 14.3 前置"+"ts 排序辅助 → ts 业务排序禁令"是 substantive 语义修改，但 r5 lesson md 和 commit 只 cover 了 `docs/宠物互动App_V1接口设计.md` + r5 lesson md 自身。story 文件 line 225 / 228（V1 doc §12.3 关键约束的 fenced block 镜像）+ line 289 / 522（AC7 章节 / AC7 验证清单 ✅）没同步修订，与 Lesson 1 同病：story 文件被默认当成"定稿过程记录"而非"下游 dev 一手引用源"。

更深层：story 文件的 AC 章节（line 289 AC7）+ AC 验证清单（line 522 AC7 ✅）是 BMAD 的"我做完了什么"记录段；改 V1 doc 后**不更新这段 = AC 仍在声明"五处完全等价"的旧规则**，下游 review sub-agent 看 story 文件会以为"AC7 已通过 ✅ + 完全等价是定稿规则"。

### 修复（Fix）

story 文件中所有"三处 / 五处完全等价 / ts 排序"旧措辞重写为"等价分两层 + 14.3 前置 + ts 全局禁用业务排序"：

- **line 225**（V1 doc §12.3 关键约束 fenced block）：把"`payload.currentState` 字段语义与 `room.snapshot` / `GET /rooms/{roomId}` **完全等价** ... client 应以最近一次 WS 广播为最新真相源（按 `ts` 字段时间戳判断新旧）"重写为完整的"等价分两层（值域 / 权威）+ 14.3 前置 + 14.3 前临时窗口优先级表（`pet.state.changed` > snapshot/GET/member.joined）+ self vs 他人 entry 不同路径 + FIFO + room.snapshot 重对齐 + 14.3 权威等价兜底"，与 V1 doc line 2252 措辞对齐
- **line 228**（V1 doc §12.3 关键约束 fenced block `ts` 段）：把"client 可用作时序排序 / 状态新旧判断辅助信号（如多条 `pet.state.changed` 乱序到达时按 `ts` 排序）"重写为"用途**仅限**客户端日志关联 + UI 辅助展示，**禁止**用作业务排序 / 状态新旧判定 ... 状态新旧判定由 FIFO + reconnect room.snapshot + 14.3 权威等价层共同兜底"，与 V1 doc line 2255 措辞对齐
- **line 289**（AC7 字段语义跨章节一致性）：把"**五处字段语义完全等价**"重写为"**等价分两层**：(a) 值域 / DB 来源等价（恒成立，六处）+ (b) 权威 / client 信任层等价（自 Story 14.3 起成立）"，并把六处枚举加上 `member.joined.payload.pet.currentState`（与 V1 doc line 2252 四处+§5.2 request/response 共六处一致）
- **line 522**（AC7 验证清单 ✅）：升级标题为"AC7 字段语义跨章节一致性（**分两层 + Story 14.3 前置 + `ts` 业务排序禁令**）" + 描述补"`ts` 字段语义升级为'仅日志关联 + UI 辅助展示，禁止业务排序'（与 §12.2 全局 WS envelope `ts` 字段约束一致），状态新旧由 FIFO + reconnect room.snapshot + 14.3 权威等价层兜底"

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **frozen doc 的同 story review 循环里升级"跨字段等价"措辞**（如 "完全等价" → "分两层 + 前置条件" 或 "字段可用作 X" → "字段禁用作 X"）时，**必须**把 story 文件 AC 章节 + AC 验证清单 ✅ 项 + Dev Notes 关键设计点 一起同步重写，且 AC 验证清单的 ✅ 标记**只能**保留在"措辞已与 frozen doc 一致"的 AC 项上。
>
> **展开**：
> - 跨字段等价 / 业务排序信号 这类 contract 措辞的精度直接决定下游实装代码行为（如 ts 全局禁用 → client 不能写 `messages.sort(by: \.ts)` ）。改 doc 时不同步 story 文件 AC 章节 = 下游 dev 默认照 AC ✅ 抄实装。
> - 改 frozen doc 的修订步骤**必须**把同 story 文件的 AC 章节扫一遍：(i) 该 AC 项目前的 ✅ 是否还成立（描述与 doc 一致）；(ii) AC 描述里的核心措辞（如"完全等价" / "可用作排序"）是否需要同步升级；(iii) AC 验证清单（如"AC7 已验证 ✅"段）是否需要重写描述并保留 ✅ 或临时降为 🔄。
> - **反例**：r5 在 V1 doc §12.3 line 2252 把"完全等价"升级为"分两层 + 14.3 前置"，r5 commit 只改 V1 doc + 添加 r5 lesson md。story 文件 line 289 AC7 描述仍是"五处字段语义完全等价"+ line 522 AC7 仍标 ✅。下游 14.4 dev sub-agent 若按 story 文件 AC7 ✅ 实装，会假设 14.3 前 `room.snapshot` `pet.currentState` 与 `pet.state.changed` 等价，从而用 placeholder `1` 覆盖广播的 2/3 真实值，违约。

## Meta: 本次 review 的宏观教训

**story 文件不是"定稿过程的副产品"，而是 BMAD 流程里下游 dev sub-agent 的一手实装引用源**。frozen doc（V1 doc / 数据库设计 / 总体架构）是契约最终态，但 BMAD 的 dev-story sub-agent 的 navigate path 是"读 story 文件的 AC + Dev Notes + 关键设计点 → 在 story 文件指引下打开 V1 doc 校验 schema"。若 story 文件本身和 frozen doc drift，下游 dev 大概率不会"穿透"到 frozen doc，而是直接照 story 文件实装 stale 契约。

**修订循环（review r2 / r3 / ...）做语义级修改时的修订单元应该是"frozen doc + story 文件 + lesson md"三位一体**，而不是只 cover frozen doc + lesson md。否则会出现本轮 r8 这种"V1 doc 已经 frozen 最终态，story 文件还在前 7 轮的某个版本飘着"的 drift。

下次创建 review-fix lesson md 时，**预防规则段一定要明示"修改 frozen doc 必须同步同名 story 文件"**作为 universal rule，避免后续 story 重复踩这个坑。
