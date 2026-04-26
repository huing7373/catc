---
date: 2026-04-26
source_review: codex review of Story 4-1 (round 1, /tmp/epic-loop-review-4-1-r1.md)
story: 4-1-接口契约最终化
commit: 47c7998
lesson_count: 1
---

# Review Lessons — 2026-04-26 — V1 接口设计 /home chest.status 必须严格按节点阶段限定状态空间

## 背景

Story 4.1 把节点 2 三个接口（`POST /auth/guest-login` / `GET /me` / `GET /home`）的 schema 在 `docs/宠物互动App_V1接口设计.md` 里逐字段锚定并冻结。Codex review 发现 `/home` 的 `data.chest.status` 字段把 enum 写成 `1=counting / 2=unlockable / 3=opened`，但节点 2 的真实状态空间只有 `1 / 2`，`3=opened` 既不存在于 §7.1 chest 接口章节，也不存在于数据库设计 §6.7 `user_chests.status` 枚举。冻结契约里携带未实装的状态值 → 下游 Story 4.8（server）和 iOS Story 5.5 会按错误的状态空间写实装/客户端 DTO。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `/home` 的 `chest.status` enum 写出节点 2 不存在的 `3=opened` | P2 / medium | docs | fix | `docs/宠物互动App_V1接口设计.md:345`、`_bmad-output/implementation-artifacts/4-1-接口契约最终化.md:252` |

## Lesson 1: 跨章节引用的 enum 必须以"当前节点真实状态空间"为唯一锚，不要把未来节点才出现的状态值偷偷塞进当前节点契约

- **Severity**: P2 / medium
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:345`（GET /home 响应字段表 `data.chest.status` 行）；同步影响 `_bmad-output/implementation-artifacts/4-1-接口契约最终化.md:252` AC5 schema

### 症状（Symptom）

V1 接口设计 §5.1（GET /home）字段表里 `data.chest.status` 的说明写成：

> 宝箱状态枚举（1=counting, 2=unlockable, 3=opened —— 见 §7.1 status 枚举）

但同文件 §7.1 status 枚举只列了 `1 = counting / 2 = unlockable`，`docs/宠物互动App_数据库设计.md` §6.7 `user_chests.status` CHECK 约束也只允许 `1 / 2`。`opened` 状态对应"开箱已结算"，是 Epic 20 / 节点 7 才上线的功能；在节点 2 阶段，`/home` 永远不会返回 `3`。

### 根因（Root cause）

写 §5.1 字段表时复用了"未来 §7.1 status 枚举"这个引用思路 —— 觉得"`/home` 的 chest.status 当然继承 §7.1 完整枚举"。但当前节点（节点 2）下：

1. §7.1 自身只列了 1/2（因为节点 2 还没 opened 状态）
2. 数据库 §6.7 也只允许 1/2
3. 节点 7 的 `/chest/open` 接口落地时会重建下一轮 chest（见 epics.md 节点 7 流程），因此即便开箱后，下次 `GET /home` 返回的也仍是新一轮的 `counting`，**`/home` 永远观察不到 opened 状态**

也就是说，`3=opened` 写进 §5.1 是**对未来设计的错误预测** —— 既错地以为 §7.1 后续会扩成 1/2/3，又错地以为 `/home` 会观察到 opened（实际上 opened 是 chest_open_logs 的状态语义，而非 user_chests.status 的状态语义）。

### 修复（Fix）

把 §5.1 `data.chest.status` 行的 enum 描述从 `1=counting, 2=unlockable, 3=opened` 改成 `1=counting, 2=unlockable`，并显式补一句"节点 2 不存在 `opened` 状态：开箱功能在节点 7 / Epic 20 才上线，届时开箱后会立即重建下一轮 chest（仍为 `counting`），故 `/home` 永远不返回 opened 状态"。同时同步更新引用 §7.1 与数据库 §6.7 的来源，避免未来读者只追到 §7.1 一个锚。

Story 4.1 的 AC5 schema 副本（`_bmad-output/implementation-artifacts/4-1-接口契约最终化.md:252`）做同样的同步修改 —— story 文件是 dev-story 阶段产出的契约副本，必须与目标 V1 文档严格一致，否则 future Claude 重读 story 时会取到旧值。

修改片段（before / after）：

```diff
- 宝箱状态枚举（1=counting, 2=unlockable, 3=opened —— 见 §7.1 status 枚举）；节点 2 阶段所有宝箱在登录初始化后均为 `1`（counting），到 `unlockAt` 后服务端**动态判定**为 `2`（unlockable）—— 见 Story 4.8 happy path 第 2 case
+ 宝箱状态枚举（1=counting, 2=unlockable —— 见 §7.1 status 枚举 + 数据库设计 §6.7 user_chests.status）；节点 2 阶段所有宝箱在登录初始化后均为 `1`（counting），到 `unlockAt` 后服务端**动态判定**为 `2`（unlockable）—— 见 Story 4.8 happy path 第 2 case。**节点 2 不存在 `opened` 状态**：开箱功能在节点 7 / Epic 20 才上线，届时开箱后会立即重建下一轮 chest（仍为 `counting`），故 `/home` 永远不返回 opened 状态
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **冻结某个跨节点接口的字段 enum 描述时**，**必须**把 enum 候选值与"当前节点的 DB schema CHECK / 当前节点的状态机文档"做交叉对账，不能写"此字段未来会扩展"的任何值。
>
> **展开**：
> - 每个 enum 字段的契约写法应当是"**当前节点该字段真实可能取的所有值**" + 来源锚（DB schema 章节 + status 枚举章节 + 状态机时序图）。如果 DB CHECK 只允许 1/2，文档就只写 1/2，并显式说明"未来扩展见 X"。
> - 跨章节引用 enum 时（如"见 §X.Y status 枚举"），**先打开 §X.Y 看清里面写了什么**，不要假设它包含某个值。如果 §X.Y 自身只写了 1/2，本接口就**不能写 3**，无论你觉得"未来肯定会有 3"。
> - 注意"接口可观察的状态空间" ≠ "DB 实体的全部状态空间"。`user_chests.status` 即便未来扩到 1/2/3，也要确认 `GET /home` 能否实际观察到 3（如本案例：开箱后立即重建 chest，`/home` 永远观察不到 opened）。状态语义可能藏在另一张表里（如本案例的 `chest_open_logs`），**不**要把"日志态"塞进"实体态"。
> - 节点 N 的契约只能携带节点 N 实装的状态空间。Future fields 的标准做法是 §X.Y 章节末尾加 `> Future Fields (节点 X 落地)` 引用块，**不**是把未来值偷塞进 enum 描述。
> - 改完目标文档后，story 文件的 AC schema 副本必须同步改 —— story 文件是 future Claude 复读 story 时的契约镜像，与目标文档不一致 = 未来某次实装漂移。
> - **反例**（踩坑形态）：
>   - "`status` 枚举：1=counting, 2=unlockable, 3=opened（节点 2 阶段只用 1/2）" —— 写法错。节点 2 阶段就不该列 3，"只用 1/2"是注释，不是契约。
>   - "见 §7.1 完整枚举"（但 §7.1 自己也没 3）—— 引用与被引用的内容不一致。
>   - 把"日志状态"（如 `chest_open_logs.status = opened`）和"实体状态"（如 `user_chests.status`）混淆，把日志状态写进实体接口的 enum。
