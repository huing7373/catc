---
date: 2026-05-02
source_review: codex review (epic-loop r4) — /tmp/epic-loop-review-7-1-r4.md
story: 7-1-接口契约最终化
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-02 — step_account 示例数值不变量 & 同类已认证路由限频 scope 必须重复显式

## 背景

Story 7.1 第 4 轮 codex review。前三轮（r1/r2/r3）依次解决了：3001 的"封顶判断方向"+ 5000 cap 的输入语义、`POST /steps/sync` 的 1005 限频 scope（同 IP → user_id）+ 3001 粘性误述、跨文件契约锚定（V1 文档 + 时序图 + 数据库枚举注释三处同步）。第 4 轮命中两条遗留 P2：

1. §6.1 / §6.2 新增的 `step_account` JSON 示例值 `totalSteps=12560, availableSteps=840, consumedSteps=300` 违反了账户模型隐含等式 `total_steps = available_steps + consumed_steps`（后两者是 earn vs spend 的分区）—— 12560 ≠ 840 + 300，会让 Story 7.3/7.4 实装者和测试 fixture 抄走错误的不变量
2. §6.2 `GET /steps/account` 的限频 scope 只写"rate_limit 默认配置"/"rate_limit 中间件拦截"。同样是已认证业务路由，§6.1 在 r2 已修成"按 user_id 每分钟 60 次"的明确表述；§6.2 没跟进，重新引入了 r2 已经修过的歧义

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | step_account 示例三档值违反账户模型等式（totalSteps ≠ availableSteps + consumedSteps） | medium (P2) | docs | fix | `docs/宠物互动App_V1接口设计.md`, `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md` |
| 2 | §6.2 GET /steps/account 限频 scope 未明确（同类已认证路由 §6.1 已显式 user_id-scoped） | medium (P2) | docs | fix | `docs/宠物互动App_V1接口设计.md`, `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md` |

## Lesson 1: step_account 示例三档值必须满足账户模型等式

- **Severity**: medium (P2)
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:577-579`（§6.1 sync 响应示例）+ `:637-639`（§6.2 account 响应示例）

### 症状（Symptom）

V1 文档 §6.1 `POST /steps/sync` 响应示例和 §6.2 `GET /steps/account` 响应示例同时出现：

```json
{
  "totalSteps": 12560,
  "availableSteps": 840,
  "consumedSteps": 300
}
```

但按 `数据库设计.md` §4 `user_step_accounts` 字段语义：

- `total_steps` = 累计获得步数（earn 累加）
- `available_steps` = 当前可消费步数
- `consumed_steps` = 累计消耗步数（spend 累加）

`available + consumed` 必然等于 `total`（earn 是 spend 的源头，account 模型把 earn balance 切成 available 和 consumed 两半），但 12560 ≠ 840 + 300 = 1140。示例自相矛盾。

### 根因（Root cause）

写示例值时把 `total_steps` 当成"显示用累计步数"（类似 fitness app 里只用于展示的"今日步数"），随手取了一个看起来正常的大数字（12560 像一个真实日步数），却没意识到本系统里 `total_steps` 是**账户模型下的 earn 累计**，必须与 `available + consumed` 在任何时刻保持等式（不变量）。

更深层的失误是：写 JSON 示例时**没读对应的数据库字段语义注释**。`数据库设计.md` §4 的"字段说明"+§4 的"设计说明"明确写了"按账户模型记账"，但写文档示例的人只读了 V1 接口文档自身的字段表（`字段` / `类型` / `说明` 三列），没回头查数据库模型的语义注释。

### 修复（Fix）

把两处示例的三档值统一改为满足等式 `total = available + consumed` 的合理一组：

```json
{
  "totalSteps": 1140,
  "availableSteps": 840,
  "consumedSteps": 300
}
```

(1140 = 840 + 300，余下三档值含义保持不变)

同步更新：
- `docs/宠物互动App_V1接口设计.md` §6.1 line 577-579
- `docs/宠物互动App_V1接口设计.md` §6.2 line 637-639
- `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md` AC2.5 §6.1 示例 + AC3 §6.2.2 示例（story 是 V1 §6.x AC 的 source-of-truth，必须同步避免再次漂移；这条沿用 r3 已建立的"跨文件契约锚定"规则）

**未触动**：§5.1 GET /home 和 §7.4 chest open 的 step_account 示例值也违反同一不变量（12560 ≠ ...），但属于 §5.x / §7.x 节点 2 / 节点 4 范畴，**不属于 Story 7.1 节点 3 锚定 scope**，且 §5.1 已在 2026-04-26 进入冻结。这两处的修复应该在对应 epic（如 17.1 或专门的契约 sweep story）里处理。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 V1 接口文档里写**字段被账户/资产模型分区**的 JSON 示例时，**必须**先回查数据库设计文档对应表的"字段说明"+"设计说明"，找出隐含等式（如 `total = available + consumed` / `balance = sum(ledger.delta)`），然后构造满足该等式的示例值；**禁止**用孤立数字或抄真实 fitness app 数字。
>
> **展开**：
> - **触发条件**：JSON 示例里同一个对象内有 ≥ 3 个数值字段，且这些字段在数据库表层有"分区"语义（earn / spend / available / consumed / total / pending 之类的搭配）
> - **强制动作**：写示例前把数据库设计文档对应表的字段注释贴进刮 hard buffer，列出所有隐含不变量，然后选满足等式的最小整数组合（如 1140/840/300 而非 12560/840/300）
> - **强制动作**：跨章节出现的同一资源（如 §6.1 stepAccount + §6.2 stepAccount + §5.1 home.stepAccount + §7.4 chest open.stepAccount）必须用**同一组**满足等式的示例值，避免每个章节自己造一组
> - **反例**：写 §6.1 sync 响应示例时随手填 `totalSteps=12560, availableSteps=840, consumedSteps=300`，因为 12560 看起来像个"看着就是真实步数"的数。这种思路对**展示型字段**成立，但对**账户型字段**绝对错误 —— 任何抄走该 fixture 的下游 unit test 会在第一次写 invariant assertion 时崩
> - **反例**：跨章节示例值不一致 —— §6.1 写 12560/840/300，§5.1 也写 12560/840/300（两处不变量都坏，但同时出现给读者"它们本来就该一样"的错觉），这种"全局一致的错"比"局部不一致的错"更危险

## Lesson 2: 同类已认证路由的限频 scope 修复必须横扫，不能只修触发 review 的那个端点

- **Severity**: medium (P2)
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:614 / :647 / :658`（§6.2 GET /steps/account 三处限频措辞）

### 症状（Symptom）

r2 review 已经把 §6.1 `POST /steps/sync` 的 1005 限频 scope 从"同 IP 每分钟 > 60 次"修正为"**已认证路由**按 user_id 限频，每用户每分钟 > 60 次"。但同节点同类型的 §6.2 `GET /steps/account` 三处限频措辞还是模糊的：

- 元信息表："限频 | 默认"
- 服务端行为 #1："rate_limit 默认配置"
- 错误码表 1005："rate_limit 中间件拦截"

`server/internal/app/bootstrap/router.go` + Story 4.5 钦定：**所有已认证业务路由按 `user_id` 限频，未认证路由按 IP 限频**。§6.2 是已认证路由，但模糊措辞会让 Story 7.4 实装者 / 客户端 / 测试可能假设"默认 = IP-scoped"，重新引入 r2 已修过的歧义。

### 根因（Root cause）

r2 修 §6.1 时**只改了被 review 直接 cite 的那段**，没顺手扫整个 §6.x（甚至整个文档里所有"已认证路由"的 §)，把同类已认证路由的限频措辞也对齐。这是典型的"哪里痒挠哪里"修法 —— 修 review 命中的 finding 但不外推到结构同型的兄弟章节，导致 r3 / r4 review 一轮一轮回来挑相邻段落。

更深层失误：写 §6.2 时认为"限频 默认"和"rate_limit 默认配置"是足够的 —— 假设"默认"会被读者按 router.go 的实装回填语义。但**契约文档**的设计原则是**所有契约语义在文档侧自描述**，不能让读者跳读到 router.go 才能知道"默认"是 IP 还是 user_id scope。

### 修复（Fix）

§6.2 三处限频措辞统一对齐到 §6.1 已落地的 user_id-scoped 显式表述：

- 元信息表："限频" 改为 "**已认证路由**按 `user_id` 每分钟 60 次（按 Story 4.5 默认值，配置可调；与 §6.1 `POST /steps/sync` 同语义）"
- 服务端行为 #1：rate_limit 描述改为 "rate_limit 中间件按 `user_id` 限频（每用户每分钟 60 次，按 Story 4.5 默认值，配置可调；与 §6.1 `POST /steps/sync` 同 scope —— 已认证业务路由统一按 `user_id` 而非 IP）"
- 错误码表 1005 触发条件加上 "（**已认证路由**按 `user_id` 限频，每用户每分钟 > 60 次；按 Story 4.5 默认值，配置可调）"

同步更新 `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md` AC3 §6.2.1 / §6.2.3 / §6.2.4 三块 spec fragment（story 是 §6.2 AC 的 source-of-truth）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 修一条"契约表述模糊 / 错错"的 review finding 时，**必须**在 commit 前对**结构同型的兄弟章节**（同节点同类型路由 / 同模型同 schema 字段 / 同 enum 同 transition）做一次正则扫荡，把同型措辞**一次性**全部对齐，**禁止**只修被 review 直接 cite 的那个 anchor。
>
> **展开**：
> - **触发条件**：review 找到的 finding 类型是"措辞模糊 / 默认值未显式 / scope 没标"等**结构性**问题（不是"具体数值错"那种点状问题）
> - **强制动作**：identify 触发 finding 的"结构 key"（如"已认证路由的限频 scope"），对所有出现该 key 的章节做 grep；对照修复
> - **反例**：r2 只修 §6.1 line 592 的 1005 措辞，没扫 §6.2 同样三处 1005 / 限频措辞 —— 结果 r4 又把 §6.2 单独提一遍，浪费一轮 review
> - **反例**：把"默认配置"当成可省略的措辞 —— 在契约文档里"默认"是必须自描述的；契约文档写的所有内容都不该需要读者跳到 router.go / config.yaml 才能补全语义。在 V1 文档表"限频"列写"默认"等于没写

---

## Meta: 本次 review 的宏观教训

r1 → r2 → r3 → r4 四轮 review 命中的全是同一类问题：**契约文档的"局部修复"导致"兄弟章节漂移"**。r1 修 §6.1 的边界语义；r2 修 §6.1 的 1005 措辞但漏 §6.2；r3 修 §6.1 的锚定但漏时序图 + 数据库枚举；r4 修 §6.1 的示例但漏 §6.2，以及 §6.2 1005 措辞自身漏掉。

宏观规则：**契约 story（X.1 系列）的 review 修复**必须以"扫荡"心态做 —— 每条 finding 修完都问一遍"这个结构 key 在文档其他章节是否还出现 / 是否同型 / 是否需要同步对齐"。如果不对齐，下一轮 review 必然回来挑同型兄弟章节。

未来在 7.1 / 11.1 / 17.1 / 23.1 这类**接口契约最终化** story 的 review fix 阶段，**第一动作**应该是"按结构 key 横扫"，第二动作才是"修触发 finding 的具体位置"。

---

## 历史 lesson 链（同 story 不同轮次）

- r1 (`docs/lessons/2026-05-02-step-cap-boundary-and-input-bound-contract.md`) — 5000 cap 输入硬边界 / 50000 封顶判断方向
- r2 (`docs/lessons/2026-05-02-step-sync-rate-limit-scope-and-3001-stickiness-myth.md`) — 1005 user_id-scoped / 3001 非粘性
- r3 (`docs/lessons/2026-05-02-cross-doc-contract-anchor-scope.md`) — 跨文件契约锚定（V1 + 时序图 + 数据库枚举）
- r4（本文件）— step_account 示例不变量 / §6.2 限频 scope 横扫漏修
