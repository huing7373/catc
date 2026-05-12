---
date: 2026-05-12
source_review: codex review (/tmp/epic-loop-review-14-1-r7.md) — Story 14.1 第 7 轮
story: 14-1-接口契约最终化
commit: 51f1972
lesson_count: 1
---

# Review Lessons — 2026-05-12 — state-sync pet-less 必须与 /home / room / member.joined 同语义合法 edge case，不能既"理论不该发生 → 1003"又"client 必须支持 pet = null"

## 背景

Story 14.1 接口契约最终化第 7 轮 codex review。此前 r1-r6 反复迭代 §5.2 POST /pets/current/state-sync 接口 schema、§12.3 `pet.state.changed` WS 广播契约、self-broadcast 例外、HTTP vs WS 到达顺序对称无操作、merge contract 与 ts 业务排序禁令等。r7 codex 发现一个跨章节**一致性矛盾**：§5.2 把 pet-less 用户（`pets WHERE user_id=? AND is_default=1` 查询返回 0 行）当作 invariant 损坏 → 1003 资源不存在 + log error，但 §5.1 GET /home、§10.3 GET /rooms、§12.3 `member.joined` 三处都**显式**把 `pet = null` 当作 client 必须支持的合法 edge case。两种立场在同一接口契约里并存，会让 Story 14.2 service 工程师和 iOS Story 15.4 client 工程师做出互不兼容的两种实装：一种把 pet-less 当 unreachable → 收到 1003 即报警，另一种把 pet-less 当合法 → 对 state-sync 调用做 special-case suppress（client 端检测自己 `pet == null` 时不调 state-sync）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | state-sync 1003 触发 vs §5.1/§10.3/§12.3 pet-less 合法 edge case 矛盾 | medium (P2) | docs/architecture | fix | `docs/宠物互动App_V1接口设计.md` + `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md` |

## Lesson 1: pet-less 账号语义必须**全契约一致**，不能 GET 类接口说"合法 + null 占位"、POST 类接口说"invariant 损坏 + 1003"

- **Severity**: medium (P2)
- **Category**: docs / architecture (跨章节一致性)
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §5.2（行 530 服务端逻辑步骤 3 + 行 596 错误码表 1003 行 + 行 603 关键约束 1002 vs 1003 段 + 行 49 §1 冻结声明）

### 症状（Symptom）

§5.2 POST /pets/current/state-sync 把"用户无默认 pet 行"（pets 查询返回 0 行）作为"理论不该发生（数据 invariant 损坏） → 返回 1003 资源不存在 + log error"分支处理。但同一份 V1 接口设计文档的：

- §5.1 GET /home 行 369：`data.pet` 类型 `object | null`，"用户**无默认 pet（理论不该发生，但 Story 4.8 edge case 强制覆盖）时返回 `null`**。客户端必须按可空对象解析（iOS `Optional<PetDTO>` / Go `*PetDTO`），不得假设 pet 永远非空"
- §10.3 GET /rooms/{roomId} 行 1388：`data.members[].pet` 类型 `object | null`，"**pet-less 账号**（用户**无活跃 pet**，理论不该发生但 §5.1 / Story 4.8 已将其作为 contract 内合法 edge case 覆盖）时下发 `null`"
- §12.3 `member.joined` 行 2120：`payload.pet` 类型 `object | null`，"加入的成员当前宠物容器；与 §10.3 `data.members[].pet` / §12.3 `room.snapshot` / §5.1 GET /home `data.pet` 同语义，**pet-less 账号**下发 `null`"

三处都把 pet-less 当**合法 edge case**，client 必须支持；§5.2 却把 pet-less 当 invariant 损坏。Story 14.2 实装工程师和 iOS Story 15.4 工程师无法判断 pet-less 是"应该 server 静默处理 + client 正常调用"还是"client 自己负责不调用 + server 兜底报警"，会出现两种互不兼容的实装。

### 根因（Root cause）

写 §5.2 接口契约时，独立地基于"数据库设计 §5.3 唯一约束保证每个 user 必有 1 行 pets"推理出"pet-less ⇒ invariant 损坏 ⇒ 1003"。但 §5.1 / §10.3 / §12.3 **在更早的 story（如 Story 4.8 / Story 11.1）已经显式放开**"pet-less 是 contract-valid 合法 edge case，client 必须支持 `pet = null`"。**写新接口契约时未做跨章节一致性检查**：契约 story 落地时只看了"epics.md 钦定的当前 story AC"，没回头扫"同一份 V1 doc 里其他章节对同一概念（pet-less 账号）已经定义了什么语义"。

更深层：契约 story 的脑模型默认"DB invariant ⇒ contract invariant"，但 contract 层完全可以选择"承认 DB invariant 可能被破坏（数据腐烂 / 历史脏数据 / 未来扩展点）→ 把破坏后状态当合法 edge case 显式覆盖"。后者一旦在某接口生效，**所有相关接口必须同语义**，不能局部接口反向上锁定 invariant。

### 修复（Fix）

选**方案 a：统一为"pet-less 是合法状态，state-sync 对 pet-less 用户走 server-acknowledged noop"**：

- `docs/宠物互动App_V1接口设计.md` §5.2 服务端逻辑步骤 3 0-row 分支：从"理论不该发生 → 1003 + log error"改为"pet-less 账号（与 §5.1 / §10.3 / §12.3 合法 edge case 同语义）→ 走 server-acknowledged noop 路径：跳过步骤 4（无 pet 可 UPDATE）+ 跳过步骤 5（无 pet 实体可广播）+ 直接进入步骤 6 返回 200 OK + code = 0 + `data.state` = 入参 state（回显）"
- §5.2 错误码表移除 1003 行（仅保留 1001 / 1002 / 1005 / 1009 四行）
- §5.2 "注"段补一句："本接口**亦不触发 1003** —— pet-less 是与 §5.1 / §10.3 / §12.3 同语义的合法 edge case，server 对 pet-less 用户走 server-acknowledged noop 路径返回 200 OK + code = 0"
- §5.2 关键约束第 1 条从"1002 vs 1003 的语义差异"改写为"1002 语义（唯一保留的请求侧错误码） + 本接口不触发 1003 + client 实装层不需要为 pet-less state-sync 做 special-case suppress"
- §1 节点 5 冻结声明（行 49）移除 1003，新增"pet-less 账号走 server-acknowledged noop 路径"为契约不变量
- Story 14.1 文件同步：服务端逻辑步骤 3 / 错误码表 / 关键约束 / AC6 标题 + 描述 / Change Log 追加 r7 entry
- 1003 仍在 §3 全局错误码表保留（其他接口如 §6.x step-sync 仍可触发），仅 §5.2 不触发

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写新 REST/WS 接口契约定义"用户没有某资源行"的处理分支** 时，**必须**先在同一份接口设计文档里 `Grep "pet-less" / "= null" / "object | null"`（或对应资源词）扫所有兄弟接口已有的语义，**确保新接口对该 edge case 的处理（noop / 错误码 / 报警）与已有接口一致**。
>
> **展开**：
> - 跨章节一致性检查清单：写 §X.Y 新接口前，先 `Grep "<资源名>"`（如 pet-less / step-account-less / chest-less / room-less）扫 V1 接口设计 + 数据库设计 + 时序图三份 doc。如果其他接口对该 edge case **已经放开为合法 null**，新接口**禁止**单方面收紧为 invariant 损坏 + 错误码
> - "DB 唯一约束 / 必有行"≠"contract invariant"：DB 层约束是实装细节，contract 层完全可以选择把"约束被破坏后的状态"当合法 edge case 显式覆盖。这种选择一旦在某接口生效，全契约同步
> - server-acknowledged noop 路径标准模板：对资源不存在的写类操作 → 跳过 DB 写 + 跳过广播 + 返回 200 OK + code = 0 + 回显入参（如 state-sync 的 `data.state` 回显 = 入参 state）。client 实装层不需要为 noop 做 special-case，正常调用即可
> - 1003（资源不存在）的合法触发条件 = "请求 path 或 query 显式指向的资源不存在 + 该资源对应**写操作有副作用**"，如 `GET /pets/{petId}` 路径 ID 在 DB 找不到。"用户上下文隐含资源不存在"（state-sync 走 user_id 查 default pet 拿不到）**不**走 1003，应走 noop 或合法 null
> - **反例 1**：`POST /pets/current/state-sync` 服务端逻辑：`SELECT default pet → 0 行 → 返回 1003 资源不存在 + log error "数据 invariant 已损坏"`。错在：(a) 该 user 的 pet-less 状态已被 §5.1 / §10.3 / §12.3 接受为合法，§5.2 单方面拒绝相当于声明同一个用户在 GET 类接口合法、在 POST 类接口非法；(b) client 必须为 §5.2 增加额外 special-case 逻辑（"先检查自己 pet 是否 null，再决定要不要调 state-sync"），违反"server 自洽，client 不需要做契约外推理"原则
> - **反例 2**：在 GET 类接口已显式覆盖 `null` edge case 的前提下，新接口在错误码表"1003 资源不存在"+ 关键约束"1002 vs 1003 的语义差异**必须**明确"长篇展开 1003 的"P0 报警 / 数据 invariant 损坏 / 理论不可达"语义。错在：这种长篇语义反而**强化**了与其他接口的矛盾，让两个章节都看起来"权威 + 一致"，更难发现冲突。修法应反过来：在 §5.2 显式标注"本接口**不**触发 1003 —— pet-less 是合法 edge case，与 §5.1 / §10.3 / §12.3 同语义"
> - **反例 3**：把"DB 唯一约束保证最多 1 行" + "Story 4.6 登录初始化事务保证必有 1 行"当作 "pet-less 不可达" 的论据。错在：这是**实装层**的现状保证，不是**契约层**的不变量声明 —— 数据 invariant 可能被未来 story（如"删除账号 cascade"、"运维数据修复"、"多端登录 race"）破坏，contract 层一旦把"破坏后状态"暴露为合法 null（如 §5.1 行 369 已暴露），就**不能**在其他接口反向再次锁定为 invariant
> - **检查动作**：写完新接口契约 → `Grep "pet-less\|object | null\|可空对象\|nullable"` 扫整份 V1 doc → 列出所有命中点 → 逐点对比新接口对该 edge case 的处理是否一致 → 不一致就改新接口（**不**反向去改老接口，老接口的合法 null 语义是 client 已经实装的契约，**不**可回退）

---

## Meta: 本次 review 的宏观教训（r7 单条 finding，但触及跨章节一致性的根本纪律）

r1-r6 反复打磨 §5.2 接口内部的字段表 / 服务端逻辑 / 错误码 / 关键约束等，但**所有打磨都局限在 §5.2 内部**。r7 才发现：§5.2 与 §5.1 / §10.3 / §12.3 三处对**同一个 edge case（pet-less 账号）**的处理是矛盾的。这暴露 lesson：**契约 review 不能只看单一接口的内部自洽性，必须跨章节扫一遍同概念 / 同 edge case 的处理是否一致**。这条 meta 规则适用于所有接口契约 story：14.1 / 17.1 / 后续 Epic 20+ 的契约 story 都应在 review 阶段做一次"跨章节同概念扫描"作为强制检查项。
