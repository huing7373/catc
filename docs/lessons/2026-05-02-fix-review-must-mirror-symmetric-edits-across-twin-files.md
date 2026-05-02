---
date: 2026-05-02
source_review: codex review r6 of Story 7-1 接口契约最终化
story: 7-1-接口契约最终化
commit: 284fa42
lesson_count: 2
---

# Review Lessons — 2026-05-02 — fix-review 修主文档时必须双向对齐"AC 副本 + 跨文档枚举注释"两类衍生文档

## 背景

Story 7.1（POST /steps/sync + GET /steps/account 契约锚定）经历 r1~r5 共 5 轮 codex review。每一轮主文档 `docs/宠物互动App_V1接口设计.md` 改完后都会有一个或多个**派生文档**需要同步更新：

1. **AC 副本**：story file `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md` 的 AC2 / AC3 章节里嵌入了大量 V1 doc 片段（"V1 文档 §6.1 编辑后必须包含以下完整内容"格式），**整段拷贝**主文档预期成果作为验收基线。每改一次主文档就要同步改 AC 副本。
2. **跨文档枚举注释**：`docs/宠物互动App_数据库设计.md` §6.6 user_step_sync_logs.source 枚举注释里**反向引用**了 V1 文档的端点路径（`POST /dev/grant-steps`）。每次只要有人改这个枚举注释，必须确保引用的端点路径与全 repo 其他文档一致。

r5 review sub-agent 主动扫描时已经 grep 了 `200000` / `clientTimestamp` / `1005 user_id-scoped` / `step_account 数值不变量`，但 r6 codex 又挑出 2 条 P2：

- **r3 fix 引入的小 bug**：r3 codex review 让 sub-agent 在 user_step_sync_logs.source 枚举注释里加 `POST /api/v1/dev/grant-steps` 路径，但 sub-agent **没**核对全 repo 其他文档（epics.md / 1-6 / 1-10 / 7-1 等）—— 后者全部用 `/dev/grant-steps`（不带 `/api/v1` 前缀）。
- **r5 sub-agent 漏扫的 AC 副本**：story file line 174「关键约束」段是 V1 doc line 598 的**复制粘贴版**，r2 改 V1 doc 时把"3001 当日剩余 sync 都返 3001（粘性）"改成"3001 不是粘性错误码（条件性）"，但 story file 副本仍停留在旧版本。r5 sub-agent grep 了 `200000`、grep 了 `1005`、grep 了 `step_account` 数值不变量，但**没** grep "粘性" / "当日剩余 sync 调用" 这两个关键 token。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | user_step_sync_logs.source 枚举注释里 dev grant 路径多了 `/api/v1` 前缀 | P2 | docs | fix | `docs/宠物互动App_数据库设计.md:769` |
| 2 | story 7-1 file AC 副本 line 174 残留"3001 粘性错误码"误述 | P2 | docs | fix | `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md:174` |

## Lesson 1: user_step_sync_logs.source 枚举注释里 dev grant 路径加错前缀

- **Severity**: P2
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_数据库设计.md:769`

### 症状（Symptom）

数据库设计文档 §6.6 source 枚举注释写：

```
2 = admin_grant     # dev / 运营手动发放（POST /api/v1/dev/grant-steps, 见 Story 7.5）
```

但仓库其他**所有**位置（`_bmad-output/planning-artifacts/epics.md` Story 7.5 / Story 7.6 / `_bmad-output/implementation-artifacts/1-6-dev-tools-框架.md` / `1-10-server-readme-本地开发指南.md` / `2-10-ios-readme-模拟器开发指南.md` / `4-1-接口契约最终化.md` / `7-1-接口契约最终化.md` / 所有 dev 端点描述）一律用 `/dev/grant-steps`（**不**带 `/api/v1` 前缀）。

如果 Story 7.5 的 dev 直接读这个枚举注释当 source-of-truth 实装路由，handler 会被挂在错误路径，且任何基于该注释的 test fixture / curl 例子全部和 repo 其他位置不一致。

### 根因（Root cause）

r3 fix-review sub-agent 修「user_step_sync_logs.source 枚举缺端点路径锚定」时，**主动**给两个枚举值都加了 V1 文档路径 + Story 引用。但加路径时**惯性**地给 `/dev/grant-steps` 也加了 `/api/v1` 前缀（因为另一个枚举值 `1 = healthkit` 加的是 `POST /api/v1/steps/sync`，sub-agent 复制粘贴格式时没意识到 dev 端点的路径**契约**不挂在 `/api/v1` 路由组下）。

更深层：**dev 端点的 mount 决策**（不挂 `/api/v1`）是 Story 1.6 dev tools 框架定下的隐性契约 —— Story 1.6 把 dev 路由组直接挂在 `/dev`，不走 `/api/v1` 公共路由组（因为 dev 端点不是公共 API，无版本控制语义）。这条决策没在 V1 文档总章里钉死，sub-agent 修数据库 enum 注释时**没**回到 Story 1.6 / Story 7.5 实装契约校对。

### 修复（Fix）

```diff
- 2 = admin_grant     # dev / 运营手动发放（POST /api/v1/dev/grant-steps, 见 Story 7.5）
+ 2 = admin_grant     # dev / 运营手动发放（POST /dev/grant-steps, 见 Story 7.5）
```

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在文档里**反向引用** API 路径（如枚举注释 / cross-ref / Source: 引用）时，**必须**用 `git grep` 该端点在 repo 其他位置的真实路径形式（带不带前缀 / `/v1` / `/api/v1`），**确认与多数派一致**再写入。
>
> **展开**：
> - **强制动作**：写「`POST <path>`」之前先跑 `git grep -n "<endpoint-key>"` 看至少 3 个其他文档的写法 —— 如果其他文档都不带 `/api/v1` 前缀，就**禁止**自己加前缀
> - **特别警惕**「按格式补齐」反模式：如果上一行 `1 = healthkit` 注释里写的是 `POST /api/v1/steps/sync`，**不**因此惯性给下一行 `2 = admin_grant` 也加 `/api/v1` —— 不同端点的 mount 路径决策可能不同
> - **dev / debug 端点路径**：除非 Story 1.6（dev tools 框架）明确改决策，dev 端点统一挂 `/dev/*`（**不**走 `/api/v1` 路由组）；这是因为 dev 端点不是公共 API，没有版本控制语义
> - **反例**：r3 sub-agent 给 `2 = admin_grant` 加注释时直接写 `POST /api/v1/dev/grant-steps`，没 grep 其他文档，导致单文档异常 `/api/v1` 前缀和 repo 其他 9 处 `/dev/grant-steps` 不一致

## Lesson 2: r5 sub-agent 主动扫描必须用**所有**已修条款的关键 token，不能只挑显眼的

- **Severity**: P2
- **Category**: docs
- **分诊**: fix
- **位置**: `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md:174`

### 症状（Symptom）

Story 7.1 file line 174 关键约束段落写：

```
错误码 3001 与 1002 的语义差异必须在文档侧明确：... 客户端处理策略不同（1002 应停止重试；3001 当日剩余 sync 调用都会得到 3001，应静默接受 + 第二天再试）
```

而 V1 文档 line 598（同一段在主文档的对应位置）已经在 r2 改成：

```
... 注意：3001 不是粘性错误码 —— 当日只要 prevAccepted + curDelta > 50000 触发条件成立才返 3001；若客户端后续 sync 是倒退或重复，按"差值计算"步骤 delta 自然 = 0，仍返 code = 0，不会再次触发 3001 ...
```

两份文档一份说"3001 后当日剩余都返 3001"（粘性），一份说"3001 不是粘性"。下游 Story 7.3（server）/ 8.5（iOS）实装时如果以 story file 为准，会写出错误的 3001 处理逻辑（如客户端遇 3001 后停止上报、server 维护"已封顶"状态）。

### 根因（Root cause）

r5 fix-review skill 的 override 要求 sub-agent **主动扫描 story file 副本**找漏改。sub-agent 当时 grep 了：

- `200000`（r1 已删的硬上限）
- `1005` rate limit user_id-scoped wording
- `step_account` 数值不变量
- `clientTimestamp` 字段是否同步

但**没** grep `粘性` / `第二天再试` / `当日剩余 sync` 这些 r2 fix 的关键 token。原因是 r5 sub-agent 把"主动扫描"理解为"扫高频出现的 token"，而不是"扫 r1~r4 每一轮 fix 的所有关键 token"。

更深层：**fix-review skill 的"主动扫描"指令缺少机械化清单** —— 把"扫 r1~r4 修过的所有规则"留给 sub-agent 自己列，sub-agent 容易抓大放小。每个 finding 修完后没强制要求把"修复关键 token"沉淀到一个清单（如 lesson frontmatter）供后续 review 反查。

### 修复（Fix）

把 story file line 174 整段替换为 V1 doc line 598 的当前权威版本：

```diff
- 错误码 3001 与 1002 的语义差异必须在文档侧明确：1002 是参数本身不合法（客户端错），3001 是参数合法但触发服务端业务限制（防作弊封顶，客户端可继续用，但当日 sync 不再入账）；客户端处理策略不同（1002 应停止重试；3001 当日剩余 sync 调用都会得到 3001，应静默接受 + 第二天再试）
+ 错误码 3001 与 1002 的语义差异必须明确：1002 是参数本身不合法（客户端错），3001 是参数合法但触发服务端业务限制（防作弊封顶，本次 delta 被强制 = 0）；客户端处理策略不同（1002 应停止重试并修正参数；3001 应静默接受本次返回 + UX 提示"今日步数已达上限"）。注意：3001 不是粘性错误码 —— 当日只要 prevAccepted + curDelta > 50000 触发条件成立才返 3001；若客户端后续 sync 是倒退或重复，按"差值计算"步骤 delta 自然 = 0，仍返 code = 0，不会再次触发 3001。客户端不应假设当日首次 3001 后所有 sync 都失败而停止上报（仍需上报维持 sync_log 审计）
```

**附带验证（grep 证据）**：本次修复后跑了 4 个 grep 命令确认无残留：

```
grep -rn "200000" docs/ _bmad-output/implementation-artifacts/  → 0 hit（外排 docs/lessons/）
grep -rn "粘性\|当日剩余 sync 调用都会得到 3001\|3001.*第二天再试\|sync 不再入账" docs/ _bmad-output/implementation-artifacts/  → 2 hit，均为修复后的"3001 不是粘性"权威版（V1 doc line 598 + story file line 174）
grep -rn "/api/v1/dev/" docs/ _bmad-output/  → 0 hit（外排 docs/lessons/）
grep -rn "同 IP.*60\|按 IP 限频" docs/ _bmad-output/implementation-artifacts/  → 仅 §4.1 guest login 上下文（unauthenticated, IP-scoped 是正确设计），非 step-sync 范围
```

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 跑 fix-review 时，扫描 story artifact 副本**必须**用 r1~rN 每一轮已 fix 条目的关键 token 清单，**不能**靠"看哪个 token 显眼"凭感觉挑。
>
> **展开**：
> - **机械化清单**：每跑一次 fix-review，在 commit message body 或 lesson frontmatter 里列出本轮修复用到的"关键 token"（如 `200000`, `粘性`, `当日剩余 sync`, `按 IP 限频`, `/api/v1/dev/`）。下一轮 fix-review 主动扫描时**必须**把上一轮所有 token 全 grep 一遍，看是否还有遗漏副本
> - **跨文档对称编辑原则**：当 fix 涉及"主文档 + AC 副本 / 枚举注释 / 时序图调用签名"等多份**互为镜像**的文档时，每改一处必须**当场**对所有镜像跑 `git grep` 校对，**不**留到下一轮
> - **关键 token 选取**：优先选**改前的具体值**（如旧的 `200000` / `当日剩余 sync 调用都会得到 3001`）—— 因为改后的新值难以反向定位旧残留，但旧值在残留副本里一定还在
> - **反例**：r5 sub-agent 只 grep 了 `200000` / `1005` / `step_account` 三类显眼 token，**没** grep `粘性` / `当日剩余 sync` —— 这两个 token 的 r2 fix 距离当时已 3 轮，sub-agent 注意力优先放最近改的 r4 上，结果漏扫 r2 残留

---

## Meta: 本次 review 的宏观教训

Story 7.1 是**纯文档 story**，理论上风险只有"文档漏改 / 多改"。但已经走了 6 轮 review，每轮都还有 P2 级 finding。这暴露了一类 fix-review 失败模式：**"修主文档 + 修镜像副本"两步合一是高错误率操作**，因为：

1. AC 副本是**段落级**复制粘贴（不是 file-level link / include），每次改主文档时副本不会自动同步
2. 跨文档**反向引用**（数据库 enum 注释引用 V1 文档路径）在文档作者视角是"加点上下文"，但在跨文档一致性视角是"违反 single source of truth"
3. 主动扫描如果靠**人脑**抓大放小，必然漏；必须把扫描脚本化（如把每轮关键 token 写进 lesson frontmatter，下轮 sub-agent 必读）

**给 fix-review skill 的潜在改进**：未来跑 fix-review 时，在 lesson frontmatter 加 `verified_tokens: [...]` 字段，把本轮 grep 通过的关键 token 沉淀进去；下轮 fix-review 启动时从 `docs/lessons/<story>-*` 读历史 token 清单**强制**全跑一遍。这把"主动扫描"从凭感觉提到机械化。
