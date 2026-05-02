---
date: 2026-05-02
source_review: codex review r8 of Story 7-1-接口契约最终化（file: /tmp/epic-loop-review-7-1-r8.md）
story: 7-1-接口契约最终化
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-02 — fix-review 跨文档扫描必须包含上游 planning artifact（不能只扫 docs/ + story file）

## 背景

Story 7.1 第 8 轮 codex review。前 7 轮都聚焦 `docs/宠物互动App_*.md` 实装文档（V1 接口 / 数据库设计 / 时序图）+ story artifact 副本（`_bmad-output/implementation-artifacts/7-1-接口契约最终化.md`）。每轮 fix-review 都补一条"跨 X 类文档对齐"规则（r3 跨数据库 enum、r4 跨 §6.2、r5 story 副本、r6 dev path、r7 prod 阈值 + canonical 化、…），但**没有任何一轮**把上游 planning artifact `_bmad-output/planning-artifacts/epics.md` 当 fix 范围。r8 codex 命中两条 P2 都是 V1 文档 vs `epics.md` Story 7.3 AC 跨文档不一致：3001 粘性语义（V1 已是非粘性，epics.md 仍写"当日后续 sync 一律 delta=0 + 返回 3001"）+ prod 阈值可覆盖性（V1 已写 prod 不可覆盖，epics.md 仍写"配置可调"未限定环境）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | epics.md Story 7.3 AC 仍写 3001 粘性语义，与 V1 §6.1 line 598 非粘性语义冲突 | P2 (medium) | docs | fix | `_bmad-output/planning-artifacts/epics.md:1388, 1390` |
| 2 | epics.md Story 7.3 AC 把 `single_sync_cap` / `daily_cap` 写成 runtime-configurable 未限定环境，与 V1 §1 line 28 prod 不可覆盖冲突 | P2 (medium) | docs | fix | `_bmad-output/planning-artifacts/epics.md:1389` |
| extra | epics.md line 1434 `source=admin_grant`（用 enum 名）应 canonical 化为 `source=2`（admin_grant），匹配 V1 §6.1 + DB §6.6 风格 | P3 (nit) | docs | fix | `_bmad-output/planning-artifacts/epics.md:1434` |

## Lesson 1: fix-review 跨文档扫描必须显式覆盖 `_bmad-output/planning-artifacts/`，不能只扫 `docs/` + story 副本

- **Severity**: medium
- **Category**: docs
- **分诊**: fix
- **位置**: `_bmad-output/planning-artifacts/epics.md:1386-1391, 1434`

### 症状（Symptom）

V1 文档 §6.1 line 598（关键约束段）已经把 3001 写成"非粘性"语义（仅 `prevAccepted + curDelta > 50000` 触发，倒退/重复 sync 仍返 code=0），但 `_bmad-output/planning-artifacts/epics.md` Story 7.3 AC（行 1388）还写"单日累计 > 50000 → 当日后续 sync 一律 delta=0 + 返回 3001"。同样地，V1 §1 line 28 写"prod 必须用默认值 5000/50000，不可覆盖"，但 epics.md Story 7.3 AC（行 1389）只写"阈值通过配置 `steps.daily_cap` / `steps.single_sync_cap` 可调"未限定环境。下游 Story 7.3 实装时若读 epics.md 会写出错误的 3001 处理逻辑（认为粘性）+ 错误的配置覆盖逻辑（允许 prod 覆盖）。

### 根因（Root cause）

之前 r1-r7 的 fix-review 都把扫描范围圈在 `docs/` 实装文档 + `_bmad-output/implementation-artifacts/<story>.md` 副本，**没有把 `_bmad-output/planning-artifacts/epics.md` 当 in-scope**。Mental model 是"epics.md 是规划阶段文档，story 已经从 epics.md 衍生，下游只读 story file 不读 epics.md"。但实际上 `epics.md` 是**契约规则的上游源头之一**：

- 它定义每个 story 的 AC（包括 Story 7.3 的防作弊阈值规则）
- 下游 Story 7.3 / 7.4 / 7.5 dev 实装 sub-agent 在 bootstrap 时**会读** epics.md（取 story scope + AC 钦定）
- 所以 epics.md 与 V1 文档不一致 → 下游 sub-agent 读到两套契约

更深层：r1 改 V1 文档 line 598 `当日剩余 sync 都返 3001` → `非粘性`时，没意识到这条规则在 epics.md Story 7.3 AC 里也有副本（line 1388）。r7 加 V1 §1 line 28 "prod 必须用默认值"时，也没意识到 epics.md Story 7.3 AC（line 1389）的"阈值可调"是个上游规则需要同步收紧。

### 修复（Fix）

`_bmad-output/planning-artifacts/epics.md`：

1. Story 7.3 AC 防作弊阈值段（行 1386-1391）整段改写：
   - 把"单日累计 > 50000 → 当日后续 sync 一律 delta=0 + 返回 3001" 改成 "仅当 `prevAccepted + curDelta > 50000` 触发；倒退/重复 sync 走 delta=0 仍返 code=0；3001 不是粘性错误码"
   - 把"阈值通过配置 `steps.daily_cap` / `steps.single_sync_cap` 可调" 改成 "阈值通过配置 key 暴露，但 prod 部署必须使用默认值不可覆盖；dev/test 可覆盖（仅用于单测/调试 fixture）"
   - 单元测试 case 列表加一条 "prev=50000 后重复/倒退 sync → delta=0 + code=0（非 3001，验证非粘性语义）"
2. Story 7.5 AC（行 1434）`source=admin_grant` 改为 `source=2`（admin_grant），并加 "见数据库设计 §6.6"（与 V1 §6.1 / DB §6.6 canonical 风格对齐）
3. 同步更新 story file `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md` Change Log 加一条 r8 记录

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 跑 fix-review 做"跨文档对齐"扫描时，**必须**显式把 `_bmad-output/planning-artifacts/epics.md` 包进扫描范围，不能因为它是"规划阶段文档"就跳过。
>
> **展开**：
> - **跨文档对齐 checklist 永远三层**：①`docs/宠物互动App_*.md` 实装文档（共 7 份）②`_bmad-output/implementation-artifacts/<story>.md` 副本（story file 自身的 AC 副本 + Dev Notes 副本）③`_bmad-output/planning-artifacts/epics.md`（**这层 r1-r7 全部漏扫**）+ 同目录下其他 planning artifact（PRD / sprint-change-proposal / decisions/ ADR）
> - **每条规则改完 V1 文档（或任何 `docs/宠物互动App_*.md` 主文档），立即跑** `grep -n "<改前的旧 token>" _bmad-output/planning-artifacts/epics.md _bmad-output/implementation-artifacts/<story>.md`，期望除 lesson 外 0 命中
> - **关键 token 选取永远选改前的旧值**（如本轮："当日后续 sync 一律"、"阈值通过配置.*可调" 不限定环境的写法），改后的新值难以反向定位旧残留
> - **特别警惕 epics.md 的 Story X.X AC 副本**：每个 story 的 AC 在 epics.md 都有一份原始版本，dev-story workflow 会把 epics.md 的 AC 复制到 story file 当 "epics.md AC 钦定" 引用块，**两份都要扫**
> - **不要被 superseded / deleted 的 story 行误导**：epics.md 中已 superseded（如 5-5 / 24-4 / 33-6 被 ADR-0009 supersede）的 story AC**不**改，状态由 ADR 接管；只改还在 active 的 story AC（本轮只改 7.3 / 7.5）
> - **反例 1**：r1 改 V1 §6.1 line 598 把 3001 改成非粘性，r5 sub-agent 扫描"粘性 / 当日剩余 sync 调用"两个 token 时只扫 `docs/` + story file，**没**扫 `_bmad-output/planning-artifacts/epics.md`，导致 r2 ~ r7 五轮 review 都没抓到 epics.md Story 7.3 line 1388 的旧表述，r8 codex 才命中
> - **反例 2**：r7 加 V1 §1 line 28 "prod 必须用默认值不可覆盖" 时，sub-agent 同步改了 story file 副本（line 302），但**没**扫 epics.md Story 7.3 AC（line 1389）的"阈值可调"——同一组规则改前/改后语义都对得上 V1 / story file 但与 epics.md 上游对不上

---

## Meta: 跨 8 轮 review 之后的整体观察

8 轮 review 的命中模式重复出现的根因都在"**跨 X 类文档对齐**"的 X 类型每轮都有新发现：

| 轮次 | 新发现的"跨 X 对齐"维度 | 修补的关键 token |
|---|---|---|
| r1 | V1 文档内三方语义不一致（字段表 + 校验 + 错误码） | `200000`、`≥ 50000` |
| r2 | V1 文档内 1005 限频 scope（已认证路由 → user_id-scoped 而非 IP） | `按 IP`、`x-real-ip` |
| r3 | V1 文档 ↔ 时序图 / 数据库 enum 缺扩展 | `clientTimestamp`、`admin_grant` |
| r4 | V1 文档 §6.1 ↔ §6.2 step_account 数值不变量 + 限频 scope 重复显式 | `total_steps - consumed_steps`、`同语义` |
| r5 | story artifact AC 副本停留在旧版本 V1 文档 wording | `当日剩余 sync 调用`、`累计 ≥ 50000` |
| r6 | dev path 前缀 `/api/v1` 误加 | `/api/v1/dev/grant-steps` |
| r7 | 默认值是契约 vs 运行时可覆盖 + enum canonical 化 | `不可覆盖`、`source=2` |
| r8 | **`_bmad-output/planning-artifacts/epics.md` 上游 AC 副本停留在旧版本** | `当日后续 sync 一律`、`阈值通过配置.*可调` 不限定环境 |

**meta 教训**：每轮 review 都在补"扫描覆盖面"。下次 fix-review 一开始就**机械化**列出三层扫描清单（`docs/` 实装 + `_bmad-output/implementation-artifacts/<story>.md` + `_bmad-output/planning-artifacts/epics.md`），而**不是**等 codex 抓出后才被动补。同时 r1-r8 累积的所有"关键 token"（见上表）必须在每次 fix-review 主动扫描阶段全部 grep 一遍 —— 这是 r5 lesson `fix-review-must-mirror-symmetric-edits-across-twin-files.md` 的"机械化清单"原则在 8 轮之后的强化版。
