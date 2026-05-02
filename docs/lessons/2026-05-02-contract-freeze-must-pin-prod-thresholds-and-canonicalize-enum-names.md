---
date: 2026-05-02
source_review: codex review r7 of Story 7.1 (`/tmp/epic-loop-review-7-1-r7.md`)
story: 7-1-接口契约最终化
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-02 — 契约冻结必须钉死 prod 阈值，跨文档枚举名必须 canonical 化

## 背景

Story 7.1 第 7 轮 codex review。前 6 轮（r1~r6）解决了：3001 封顶判断方向 + clientTotalSteps 业务硬上限删除（r1）、1005 限频 scope user_id-scoped（r2）、跨文档锚定 V1+时序图+数据库枚举（r3）、step_account 数值不变量 + §6.2 限频 scope（r4）、story file AC 副本同步 r2 改的 3001 非粘性 + r4 改的数值不变量（r5）、`/dev/grant-steps` 路径前缀去 `/api/v1`（r6）。r7 命中两条遗留 P2，本质是**契约冻结声明**自身留了运行时漂移漏洞 + **数据库枚举改名**没传导到 story artifact 副本：

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 契约冻结声明自相矛盾：默认值是契约 + 运行时配置可覆盖 | P2 | docs | fix | `docs/宠物互动App_V1接口设计.md:28` 主文档 + `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md:302` 副本 |
| 2 | story artifact line 424 source 枚举写 `1=client_normal`（数据库设计已 canonical 化为 `1=healthkit`） | P2 | docs | fix | `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md:424` |

## Lesson 1: 契约冻结声明里"默认值是契约"和"运行时可覆盖"不能同时成立

- **Severity**: P2
- **Category**: docs / contract
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:28` + 副本 `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md:302`

### 症状（Symptom）

V1 文档 §1 节点 3 契约冻结声明原文写：

> `steps.single_sync_cap` / `steps.daily_cap` 两个配置 key 的**默认值**（5000 / 50000）属契约一部分；运行时通过配置文件覆盖默认值**不**视为契约变更，但**修改默认值**视为契约变更走完整冻结流程。

两句相互矛盾：

- 前半句声称默认值是契约的一部分（客户端可以假设 5000 / 50000）
- 后半句允许运行时配置文件覆盖且不算契约变更

任何一个 prod 实例只要把 `steps.single_sync_cap` 配置成非 5000，`/steps/sync` 就会在不同的阈值上 truncate / 返 3001。这个跨实例 / 跨部署的阈值漂移**正是** Story 7.1 想消除的"客户端不知道 server 阈值 → 客户端做了错误的预期 / UX 提示"问题。换句话说：契约冻结声明自己在重新引入它本来要消除的漂移。

### 根因（Root cause）

写"配置可调"时把"配置可调"当成纯粹的运维灵活性词汇，没区分**部署面**：

- prod 部署的阈值改变 → 客户端能观察到差异 → **是契约变更**
- dev / test 环境（fixture / 单测 / 本地调试）的阈值改变 → 客户端不会触达 → **不是契约变更**

把这两类合在一句话里写"运行时覆盖不视为契约变更"会同时放过 prod 漂移。契约冻结的本质 = "客户端可观察到的行为锁死"；任何改变 prod 可观察阈值的事件都必须走契约变更流程，不能混进"运维例外"。

### 修复（Fix）

V1 文档 line 28 + story artifact line 302（副本）双向同步改成：

```diff
- `steps.single_sync_cap` / `steps.daily_cap` 两个配置 key 的**默认值**（5000 / 50000）属契约一部分；运行时通过配置文件覆盖默认值**不**视为契约变更，但**修改默认值**视为契约变更走完整冻结流程。
+ `steps.single_sync_cap` / `steps.daily_cap` 两个配置 key 的**默认值**（5000 / 50000）属契约一部分：**prod 部署必须使用默认值**（5000 / 50000），不允许通过配置文件覆盖 —— 否则不同 prod 实例会在不同阈值上 truncate / 返 3001，重新引入本 story 想消除的客户端/服务端契约漂移；**dev / test 环境**可通过配置文件覆盖默认值（仅用于单测 / 调试 / fixture），**不**视为契约变更（这些环境不对外提供 prod 体验，跨端契约一致性不受影响）；**修改默认值本身**视为契约变更走完整冻结流程。
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写 **任何"配置可调 + 默认值是契约"型声明** 时，**必须**显式区分 **prod 部署** 与 **dev/test 环境**：prod **必须钉死默认值**（不允许配置覆盖），dev/test 可放开。
>
> **展开**：
> - "契约一部分" 与 "运行时可覆盖" 在同一段文字里出现 → 一定要追问"覆盖在哪些部署面发生"。如果允许 prod 覆盖，则等于宣告默认值不是契约
> - 客户端契约的判定标准 = "客户端能否观察到差异"。prod 是客户端看得到的部署面，dev/test 不是
> - **反例**：写"`X` 默认值属契约一部分；运行时配置可覆盖不视为契约变更"—— 这句话直接把"契约"二字废掉，因为 prod 实例可以悄悄改 X 而客户端无从感知
> - **正例**：写"`X` 默认值属契约一部分：prod 必须用默认值（不允许覆盖）；dev/test 可覆盖（仅用于单测 / 调试 fixture，不影响客户端可观察行为）；修改默认值本身走契约变更流程"
> - **判别工具**：写"配置可调"前自检 — 阈值改变后客户端会不会看到不同的 truncate 行为 / 错误码触发条件 / 响应时延。会 → prod 必须钉死；不会 → 可不钉

## Lesson 2: 数据库枚举名 canonical 化后必须 grep 全 repo 同步副本

- **Severity**: P2
- **Category**: docs / cross-doc-consistency
- **分诊**: fix
- **位置**: `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md:424`

### 症状（Symptom）

数据库设计文档 §6.6 已经 canonical 化 `user_step_sync_logs.source` 枚举为：

```
1 = healthkit       # 客户端正常上报
2 = admin_grant     # dev / 运营手动发放
```

但 story artifact line 424 还停留在旧名：

```
- §6.6 source 枚举（1=client_normal, 2=admin_grant）→ §6.1 sync_log 写入用 source=1，dev grant 用 source=2（Story 7.5）
```

下游 Story 7.3 / 7.5 / 8.5 dev 读 story artifact 当 upstream contract 时，会照抄 `client_normal` 命名到 enum 常量 / 测试 fixture / 注释里，跑到的仓库其他位置（数据库设计文档 / V1 文档 §6.1 line 553）则用 `healthkit` —— 第三种枚举名飘出来，cross-doc drift 复发。

### 根因（Root cause）

第 r3 轮把数据库设计文档的枚举从 `client_normal` 改成 `healthkit`（更准确反映"步数来源是 HealthKit framework"，因为 admin_grant 也是"client normal" 路径下的伪造，命名上的对立词应该是数据来源 healthkit vs admin_grant，不是 normal vs grant）。但 r3 的 sub-agent 改完数据库枚举后，**没** grep 全 repo 找其他副本。story artifact 在 r0 创建时直接复制了当时的旧枚举名 `client_normal` 到 cross-reference 段，r3 ~ r6 四轮 review 都没扫到这个副本。

更深层的根因 = **enum 改名比 enum 加值更隐蔽**：

- enum 加值（`+ 2 = admin_grant`）→ grep 旧值就能找到所有需要扩展的位置
- enum 改名（`1 = client_normal` → `1 = healthkit`）→ 旧名 `client_normal` 在不同文档里以"引用语境"方式存在（注释 / 跨文档 ref），grep 新名 `healthkit` 会被泛 match（HealthKit framework 在很多地方提到），grep 旧名 `client_normal` 则需要主动想到才会做

### 修复（Fix）

```diff
- §6.6 source 枚举（1=client_normal, 2=admin_grant）→ §6.1 sync_log 写入用 source=1，dev grant 用 source=2（Story 7.5）
+ §6.6 source 枚举（1=healthkit, 2=admin_grant）→ §6.1 sync_log 写入用 source=1，dev grant 用 source=2（Story 7.5）
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude **canonical 化任何 enum 命名**（不仅是加值，**改名**也算）后，**必须**用**旧名**做一次全 repo grep（不仅限 docs/，包括 `_bmad-output/`、`server/`、`iphone/` 全部代码与文档），把所有副本同步到新名。
>
> **展开**：
> - **改名场景的 grep 关键 token = 旧名**（如 `client_normal`）。新名（如 `healthkit`）会被泛 match（HealthKit framework / NSHealthShareUsageDescription 等）淹没在噪声里
> - **跨文档 enum 一致性核对清单**：改 `docs/宠物互动App_数据库设计.md` 的枚举后，必扫 `docs/宠物互动App_V1接口设计.md` / `_bmad-output/planning-artifacts/epics.md` / `_bmad-output/implementation-artifacts/<本 story>.md` / `_bmad-output/implementation-artifacts/<下游 story>.md` 四处副本
> - **改名 + 加值同时发生**时（如 r3 把 `1=client_normal` 改成 `1=healthkit` 同时加 `2=admin_grant`）→ 注意力会被加值占满，改名静默漏网；显式列两个独立动作再做
> - **反例**：r3 改完 `1=client_normal → 1=healthkit` 没 grep 旧名 `client_normal`，story artifact 副本停留在旧名 4 轮没人发现，r7 codex 抓出
> - **正例**：每次改 enum，commit message body 列两条 grep 命令证据：`grep <旧名>`（应 0 命中除 lesson 文档外）+ `grep <新名>`（应在所有应有位置命中）

---

## Meta: 本次 review 的宏观教训

r1~r7 七轮 review 命中的全部是同一类**契约文档跨文档 / 跨副本一致性** 问题：

| 轮次 | 命中类型 | 关键 token |
|---|---|---|
| r1 | 数值范围语义不一致 | `200000`、`≥ 50000` |
| r2 | 限频 scope user_id vs IP 错配 | `1005 同 IP` |
| r3 | clientTimestamp 缺时序图、source 枚举缺数据库扩 | `clientTimestamp`、`admin_grant` |
| r4 | step_account 不变量、§6.2 限频措辞 | `12560`（违反 invariant）、`同 IP` 残留 |
| r5 | 3001 粘性 vs 条件性 wording | `当日剩余 sync 调用` |
| r6 | `/dev/grant-steps` 路径前缀 | `/api/v1/dev/` |
| r7 | 契约冻结自相矛盾 + enum 改名残留副本 | `运行时通过配置文件覆盖默认值`、`client_normal` |

**总结**：纯文档 story（contract-first）的本质风险**不是**写错字段类型 / 错误码，而是 **同一份语义在多个文档 / 副本被表达，但每轮 review 修复只触达一处副本**。

**给未来 Claude 的总规则**：

1. 每次接到纯文档 story（如 X.1 接口契约锚定型 story），开工前先列出**所有可能携带本 story 关键 token 的文件**：
   - 主文档（`docs/宠物互动App_V1接口设计.md`）
   - 跨文档 ref（`docs/宠物互动App_数据库设计.md` / `docs/宠物互动App_时序图与核心业务流程设计.md`）
   - story artifact 自身（AC 副本、Dev Notes、Refactor Guidelines）
   - 上游 epics 锚定（`_bmad-output/planning-artifacts/epics.md`）
   - 下游 story artifact（如本 story 是 X.1，则 X.2 / X.3 / X.4 / X.5 都可能引用本 story 字段）
2. 每次 review fix，**修完主文档后立即对所有上述位置做一次 grep**，使用：
   - **旧值** token（如旧的 5000 阈值 / `client_normal` / `200000`）→ 期望 0 命中（除 lesson 文档外）
   - **新值** token（如新的 user_id scope / `healthkit` / `prevAccepted`）→ 期望在所有应有位置命中
3. 在 commit message body 列出本轮用到的 grep 命令 + 命中结果，作为下一轮 sub-agent 的扫描清单。本 lesson + r5 lesson + r6 lesson 都给出了关键 token 列表，下一轮 fix-review 必须把全部列表都跑一遍
