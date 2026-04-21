---
title: Architecture Index (三端架构文档入口)
owner: 跨端架构治理（iPhone / server / Watch 架构师联合）
lastUpdated: 2026-04-21
purpose: 让任何端的 dev / PM / stakeholder 都能从 repo 根 `docs/` 一步到位找到三端架构文档 · 不再有"我 clone 下来翻不到 architecture.md"的情况
---

# 📐 Architecture Index · 裤衩猫（CatC）三端架构入口

三端独立 repo，架构文档各自住在自己的 `_bmad-output/planning-artifacts/` 深层目录。**本文件是跨端 stakeholder 的统一入口**：你不需要知道哪个端的目录结构，从这里点过去就行。

---

## 三端架构文档

| 端 | 架构文档路径 | Status | 最后更新 | 架构师 |
|---|---|---|---|---|
| **iPhone（CatPhone）** | [`../../ios/CatPhone/_bmad-output/planning-artifacts/architecture.md`](../../ios/CatPhone/_bmad-output/planning-artifacts/architecture.md) | ✅ **complete** · MEDIUM-HIGH confidence · CONDITIONALLY READY FOR STORY 1.1 INCEPTION | 2026-04-21 | Developer（轮值 owner） |
| **server（Go）** | [`../../server/_bmad-output/planning-artifacts/architecture.md`](../../server/_bmad-output/planning-artifacts/architecture.md) | ✅ Epic 0 完成 · 详见 `server/_bmad-output/planning-artifacts/` | — | TBD |
| **Watch（watchOS）** | TBD（Watch 端架构文档待创建） | 🚧 pending | — | TBD |

---

## 跨端契约 SSoT（三端都依赖）

| 契约 | 路径 | 当前版本 | 漂移记录 |
|---|---|---|---|
| HTTP API | [`../api/openapi.yaml`](../api/openapi.yaml) | `0.14.0-epic0` | 见 drift-register |
| WS 消息注册表 | [`../api/ws-message-registry.md`](../api/ws-message-registry.md) | `v1` | 见 drift-register |
| 跨端 MVP 联调指南 | [`../api/integration-mvp-client-guide.md`](../api/integration-mvp-client-guide.md) | — | — |
| **跨仓契约漂移登记** | [`../contract-drift-register.md`](../contract-drift-register.md) | 当前 7-12 项（iPhone 架构 Step 2/7 登记）| **每次发现漂移必更新** |
| Fail-closed 决策表 | [`../fail-closed-policy.md`](../fail-closed-policy.md) | 待建（iPhone Epic 1 Story 1.1 内交付） | — |

**SSoT 裁决顺序**（iPhone 架构 `project-context.md` 约定 · 跨端一致）：
`server internal/dto/` > `docs/api/` > 各端 fixture

---

## 工程宪法（全端共享）

| 文档 | 路径 | 适用范围 |
|---|---|---|
| 根 CLAUDE.md | [`../../CLAUDE.md`](../../CLAUDE.md) | 所有端 · 跨仓协作纪律 · §21 工程纪律 |
| server 后端架构指南 | [`../backend-architecture-guide.md`](../backend-architecture-guide.md) | server 必读 · §21 纪律原始版 |

---

## 跨仓 Governance · 谁拥有什么

| 资产 | Primary Owner | Secondary | 更新流程 |
|---|---|---|---|
| `docs/api/openapi.yaml` | server 架构师 | iPhone / Watch 架构师 review | server 提 PR → 其他端 fixture 跟进 |
| `docs/api/ws-message-registry.md` | server 架构师 | iPhone / Watch 架构师 review | 同上 |
| `docs/contract-drift-register.md` | **跨端轮值**（月度）· 起始 iPhone 架构师 2026-Q2 | 所有端 | 任一端发现漂移 → 发现方开 PR → owner 合入 |
| 各端 architecture.md | 对应端架构师 | 跨端 sync 会议周期审视 | 架构重大变更通过 bmad workflow |
| **本 README** | **跨端联合**（iPhone 架构师起草 · 三端架构师 approve） | — | 任一端架构文档变更时更新"最后更新"列 |

---

## 阅读路径建议（按角色）

**新 dev（任一端）**：
1. 先读本文件（你在这里）
2. 读自己端的 architecture.md
3. 读 [跨端 CLAUDE.md](../../CLAUDE.md) §21 工程纪律
4. 读 [contract-drift-register.md](../contract-drift-register.md) 了解当前跨端漂移
5. 读 [跨端 API 契约](../api/)

**PM / stakeholder**：
1. 读本文件"三端架构文档 · Status 列"看整体状态
2. 读 [contract-drift-register.md](../contract-drift-register.md) 了解跨端阻塞
3. 按需深入某端架构文档 Executive Summary 节

**跨仓 sync 会议主持**：
1. 读本文件 · 更新表格"最后更新"列（会议前 24h）
2. 读 drift-register · 筛出本次会议焦点
3. 会议记录归档到 drift-register 附录

---

## 更新纪律

- 任一端 `architecture.md` 发布/重大更新 → **1 周内**更新本表"最后更新"列 + Status
- 本文件的"架构师"列变更 → **立即**更新（人员流动时）
- 本文件的"跨端 Governance" 表 **每季度** review（由轮值 owner 提 PR）
- 本文件**禁**承载实质架构内容（规则 / 决策 / 模式）—— 只做索引 · 实质内容去各端 architecture.md

**Anti-pattern**：把本 README 写成"架构概览"。它是目录卡，不是架构摘要。

---

## 已知 Gap（2026-04-21）

来自 iPhone 架构 `architecture.md` Step 7 Critical Gaps：

- **G12**（本文件存在即解决）：✅ repo 根 `docs/architectures/` 索引已建
- **G1**：server `S-SRV-15~18` 零实现 · 跨仓 sync 会议议题 1（待 2026-04-24/25 会议）
- **G6**：`contract-drift-register.md` 跨仓维护权今日 TBD → 本文件"Governance"表已初定"跨端轮值 · 起始 iPhone 架构师 2026-Q2"· **待跨仓 sync 会议 approve**

---

**本文件创建**：2026-04-21 · iPhone 架构师 initial（Step 7 G12 交付）
**next review**：跨仓 sync 会议后 · 根据会议共识调整 Governance 表
