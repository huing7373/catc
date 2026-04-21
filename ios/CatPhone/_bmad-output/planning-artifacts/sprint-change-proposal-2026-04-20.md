---
name: Sprint Change Proposal · PRD v2 对齐回填
date: 2026-04-20
author: Developer (BMad correct-course workflow)
project: CatPhone (iOS)
scope_classification: Moderate (文档层回填 + 跨仓契约同步)
status: approved-pending-edit-execution
affects:
  - _bmad-output/planning-artifacts/prd.md
  - _bmad-output/planning-artifacts/ux-design-specification.md (仅 metadata 建议)
  - CLAUDE.md (PRD 回填完成后精简 pending 段落)
triggers:
  - PRD v2 修订 5 处（2026-04-20）
  - UX Step 10 Party Mode 评审 → S-SRV-15..18 新契约
  - 架构哲学升级 B · Server 为主 / WC 为辅
downstream:
  - /bmad-edit-prd (应用 13 条编辑)
  - /bmad-validate-prd (验证回填一致性)
  - /bmad-create-architecture (消化 C1-C7 约束)
  - /bmad-create-epics-and-stories (消化 C1-C7 约束)
  - /bmad-check-implementation-readiness (重跑 readiness)
---

# Sprint Change Proposal · PRD v2 对齐回填

**日期**：2026-04-20
**模式**：Incremental（逐条审批）
**路径**：Direct Adjustment（文档层对齐，不回滚，MVP 范围不变）

---

## Section 1 · Issue Summary

### 问题陈述

PRD v2 在 2026-04-20 产生 **5 处修订**（盲盒无通知 / J1-S4 叙事 / C-BOX-01 重定义 / Success Criteria 口径 / WatchConnectivity 降级）+ UX Step 10 评审产出 **4 条 server 新契约**（S-SRV-15..18）+ 架构哲学升级（**B · Server 为主，WC 为辅**）。这些改动目前以 **附加注记**（PRD 末尾 §v2 修订注记）和 **外部 handoff 文档**（`server-handoff-ux-step10-2026-04-20.md`）形式存在，未全部回填到 PRD 主体 FR 列表、Capability Index、Journey、Success Criteria、NFR。

### 发现背景

- `/bmad-check-implementation-readiness` 2026-04-20 运行于 Step 3 BLOCKED，报告显式标出 "⚠️ v2 修订未回填原文：5 处 v2 修订仍是覆写注记形式，下游 story 写作时需主动 reconcile"
- Phase 3（Architecture + Epics/Stories 创建）即将启动，如不先对齐会放大 reconciliation 成本

### 影响证据

| 风险 | 证据 |
|---|---|
| R1 · v2 修订未回填 | PRD line 930-1055 §v2 修订注记 段存在；line 178 / 282 / 346 / 612 / 732 / 889 等仍是 v1 原文 |
| R2 · S-SRV-15..18 | 已在 PRD line 923-926 追加 ✅；但 §Server-Driven Stories 开头有 pre-existing 重复 bug（line 903-908 与 line 909-914 重复） |
| R3 · 哲学 B 波及 Watch FR | 哲学升级只在 NFR Integration WC 条目部分更新；FR2 未同步 |
| R4 · FR55 盲盒频道开关 | v2 修订 1 决策（盲盒无通知）要求移除该开关，未执行 |
| R5 · `user_milestones` / `unlocked_pending_reveal` 客户端 FR 缺口 | S-SRV-15/16 server 契约在列，但客户端 FR 未显式覆盖 |

---

## Section 2 · Impact Analysis

### Epic Impact

**[N/A]** — iOS 端尚无 epic/story，本次是 **Phase 2→3 过渡期的预规划对齐**。但本次产出的 C1-C7 约束将作为 `/bmad-create-epics-and-stories` 的硬输入。

#### 未来 epic 必须吸收的 7 条约束（C1-C7）

| # | 约束 | 来源 | 归属 epic（猜测） |
|---|---|---|---|
| C1 | 盲盒发现流程不走 push · AC 不允许 "push 通知用户" | v2 修订 1+3+4 | 盲盒/经济 epic |
| C2 | WC 仅作 fast-path 优化层 · 所有 Watch 交互 story 必须走 server 为主 + WC 兜底加速的双路径设计 | 哲学 B | 所有涉 Watch 的 epic |
| C3 | Watch 端独立 SIWA · onboarding epic 形态变化 · FR2 流程改写 | v2 修订 5 | 账号 & Onboarding epic |
| C4 | `user_milestones` 客户端集成 · 首开仪式/首次配对/首次盲盒等里程碑写入读取 | S-SRV-15 | Onboarding + 可能 UX 生存层 epic |
| C5 | `unlocked_pending_reveal` 态 UI · 盲盒 story 必须处理 Watch 不可达的延迟揭晓 | S-SRV-16 | 盲盒/经济 epic |
| C6 | `emote.delivered` ack 不能有 UI · 表情 story AC 显式禁止"已读/已送达" | S-SRV-17 | 社交/表情 epic |
| C7 | fail 节点 Prometheus metric · 所有 fail-closed story 在 Dev Notes 登记对应 metric 名 | S-SRV-18 | 横切，影响每个 epic 的 Dev Notes |

#### Epic 排序建议

- **UserMilestones（S-SRV-15）作为 iOS Epic 1 的跨仓 blocking dependency**（CLAUDE.md 根目录已标）
- 建议在 `/bmad-create-epics-and-stories` 时将账号级里程碑集成（C4）前置到 Epic 1 Onboarding 切片

### Artifact Conflicts

| Artifact | 冲突 | 操作 |
|---|---|---|
| **PRD** | 13 条编辑（详见 Section 4） | **本提案批准后，执行 `/bmad-edit-prd` 应用** |
| **UX 规范** | 无冲突（UX 是真相源头） | 无需改；可选 metadata 加回向链接 |
| **Architecture** | 不存在 | 本提案的 C1-C7 + 13 条编辑作为 Architecture 创建输入 |
| **CLAUDE.md** 根目录 "Pending Cross-Repo Action Items" | PRD 对齐完成后可精简 | 延后至 server 团队确认接收 handoff 后处理 |
| **server-handoff-ux-step10-2026-04-20.md** | 保留至 server 消化 | 不动 |
| **project-context.md** | 无冲突 | 无需改 |

### Technical Impact

- **零代码影响** — 纯文档操作
- **测试/CI 无影响** — iOS 端尚未建立代码
- **跨仓影响** — server 团队需在下次 epic 规划前接收 S-SRV-15..18 契约（CLAUDE.md 根目录已登记 pending action）

---

## Section 3 · Recommended Approach

### 选定路径：**Option 1 · Direct Adjustment**

（Option 2 Rollback 不可行——无代码可回滚，v2 修订是前进方向；Option 3 MVP Review 不单列——MVP 范围未变，其复核内容融入 Option 1）

### Justification

1. **阶段匹配**：Phase 2→3 过渡，问题本质是文档一致性而非代码失败 → 直接调整 PRD 即可解决根因
2. **成本最低**：13 条编辑 + 注记段删除，预计 2-3 小时完成
3. **风险最小**：纯文档操作，无代码/CI/部署影响
4. **对齐下游**：C1-C7 约束为 `/bmad-create-architecture` 和 `/bmad-create-epics-and-stories` 提供输入锚点
5. **纪律遵守**：CLAUDE.md §21.3 fail-closed/open 场景化 + §21.4 语义正确性 AC 早启——本次对齐把 fail 节点（S-SRV-18 metric 清单）显式化

### Trade-offs Acknowledged

- ❌ 未选"推迟对齐到 Story 层逐 story reconcile"：会把 reconciliation 成本 × 67 次，单点集中对齐更经济
- ❌ 未单独做 MVP Review workflow：v2 修订未扩大范围（反而简化通知链路）

### Effort & Risk

- **Effort**：🟢 Low · 2-3h（PRD inline 编辑 + validate）
- **Risk**：🟢 Low · 纯文档
- **Timeline impact**：0-1 天

---

## Section 4 · Detailed Change Proposals

### 已批准的 13 条编辑

#### 编辑 #1 · P1 · FR36 标记废弃（`prd.md` line 783）

**OLD**:
```
- **FR36**：iPhone Push 不剧透具体皮肤名，仅"有宝箱等你接" `[U]`
```

**NEW**:
```
- **FR36**：~~原文：iPhone Push 不剧透具体皮肤名~~ —— **已作废**（v2 修订 1：盲盒完全无通知，此 FR 前提不成立）
```

**Rationale**：沿用 PRD line 798 已有的"标记废弃不重编号"约定，保持 FR37-FR43 编号稳定（避免 Self-Validation / S-SRV-N / Capability Index 等下游引用失效）。

---

#### 编辑 #2 · P1 · Push Notification Strategy 表格（`prd.md` line 612）

**OLD**:
```
| 盲盒掉落（挂机 30min 成熟） | 轻震 + "有宝箱等你接" | **不剧透皮肤名**，保留 Watch 揭晓仪式 |
```

**NEW**:
```
| 盲盒掉落（挂机 30min 成熟） | **无通知**（旅行青蛙式纯发现制 · 无 Watch 震动 / 无 APNs / 无 badge） | 用户下次抬腕或打开 app 自然发现 |
```

**Rationale**：回填 v2 修订 1 决策。

---

#### 编辑 #3 · P4 · Journey 1 · J1-S4 叙事（`prd.md` line 346）

**OLD**:
```
...下午 2:30，手表轻震一下——**第一个盲盒掉落**（J1-S4），显示"待解锁 · 需 1000 步"。
```

**NEW**:
```
...下午 3:10，他抬腕看时间——**屏幕右上角出现一个小礼物图标，猫蹲在它旁边好奇地打量**（J1-S4），点进去显示"待解锁 · 需 1000 步"。
```

**Rationale**：回填 v2 修订 2 决策。

---

#### 编辑 #4 · P5 · Success Criteria 首次掉盒（`prd.md` line 178）

**OLD**:
```
| 首次挂机掉盒 | 戴表/打开 app 后 30 min 的第一次盲盒掉落（最早 aha） | 安装后 **12h 内 ≥ 85%** |
```

**NEW**:
```
| 首次发现盲盒 | 戴表/打开 app 后 30 min 的第一次盲盒掉落 + 用户抬腕或打开 app 主动发现（最早 aha） | 安装后 **24h 内 ≥ 85%** |
```

**Rationale**：回填 v2 修订 4 决策——指标口径从"被掉落"改为"被发现"，窗口 12h→24h。

---

#### 编辑 #5 · P6 · C-BOX-01 能力定义（`prd.md` line 282）

**OLD**:
```
- **C-BOX-01**：在线挂机计时 30 min → server 掉落事件（server 权威）
```

**NEW**:
```
- **C-BOX-01**：在线挂机计时 30 min → server 掉落事件入**待领取队列**（server 权威 · 无主动推送 · 用户主动 GET 或自然打开 app 发现）
```

**Rationale**：回填 v2 修订 3 决策。

---

#### 编辑 #6 · P2 · FR2 Watch 配对流程改写（`prd.md` line 732）

**OLD**:
```
- **FR2**：有 Apple Watch 的 User 可在 onboarding 完成 Watch 配对 `[I (FakeWatchTransport), E9]`
```

**NEW**:
```
- **FR2**：有 Apple Watch 的 User 可在 onboarding **引导去 App Store 安装 CatWatch app**，Watch 端独立完成 Sign in with Apple，Server 根据 SIWA user_id 自动关联双端 `[I (FakeWatchTransport as fast-path only), E9]`
```

**Rationale**：回填 v2 修订 5 决策——原 WC probing 路径作废（因哲学 B）；双端通过 server user_id 关联。

---

#### 编辑 #7 · P7 · NFR Integration · WatchConnectivity 条目大改增补（`prd.md` line 889-893）

**OLD**:
```
- **WatchConnectivity**（抽象 `WatchTransport`）：三档通道用途钉死——
  - `updateApplicationContext`：最新状态覆盖（当前皮肤 / 当前房间 ID）
  - `transferUserInfo`：队列化事件（盲盒解锁 / 签到事件）
  - `sendMessage`：实时双工，需双方前台；不适合后台
  - **禁** `transferFile` 走 JSON
```

**NEW**:
```
- **WatchConnectivity**（抽象 `WatchTransport` · **架构哲学 B：Server 为主，WC 为辅**）：**MVP 不使用 WC 做核心交互**——所有跨端状态同步（盲盒、装扮、表情、房间、步数）走 `server ↔ iPhone` + `server ↔ Watch` 两条独立 WSS 链路；WC 仅作为 **fast-path 优化层**（局域网双端都前台时的加速通道，可选），非 MVP 必需。三档通道若启用则用途钉死——
  - `updateApplicationContext`：最新状态覆盖（当前皮肤 / 当前房间 ID），仅做加速
  - `transferUserInfo`：队列化事件补偿，仅做加速
  - `sendMessage`：实时双工，仅双方前台可用，非关键路径
  - **禁** `transferFile` 走 JSON
  - **禁** 把 WC 当作核心通路（核心交互必须 server 权威 + WS 双链路）
```

**Rationale**：回填 v2 修订 5 + 哲学 B 升级。

---

#### 编辑 #8 · P8 · Watch 相关 FR 审查结论（`prd.md` FR23/24/28/44/46）

**操作**：**无修改** · 仅记录审查结论到本 proposal

**审查结果**：

| FR | 原文表述 | 结论 |
|---|---|---|
| FR23 | 房间同屏渲染（纯 Watch 本地） | ✅ 无跨端依赖 |
| FR24 | "**server 判定下发**，Watch 禁本地计时触发" | ✅ 已隐含哲学 B |
| FR28 | "**server 广播给房间成员**" | ✅ 已隐含哲学 B |
| FR44 | 纯 Watch 物理状态同步 | ✅ 无跨端 |
| FR46 | "纯交互触发" | ✅ 无跨端 |

**Rationale**：Watch 相关 FR 在文本层已体现 server 权威 / 本地触发分离，哲学 B 升级主要影响点在 FR2（Edit #6）和 NFR Integration（Edit #7）；其他 FR 不受波及。

---

#### 编辑 #9 · P3 · FR55 盲盒频道删除（`prd.md` line 816）

**OLD**:
```
- **FR55**：User 可分频道开/关 push（盲盒 / 好友 / 表情）`[U, I]`
```

**NEW**:
```
- **FR55**：User 可分频道开/关 push（好友 / 表情）`[U, I]`（盲盒频道已移除——v2 修订 1：盲盒全程无通知）
```

**Rationale**：D1=a 决策落地。

---

#### 编辑 #10 · P10 · 新增 FR10a · `user_milestones` 客户端契约（`prd.md` §1 末尾，line 740 之后）

**NEW**（新增行）:
```
- **FR10a**：System 将账号级里程碑（`onboarding_completed` / `first_emote_sent` / `first_room_entered[room_id]` / `first_craft_completed`）存储在 **server `user_milestones` collection**（联动 `S-SRV-15`），**禁止使用 UserDefaults 作为权威源**（保证换机/重装无缝恢复）`[U, I]` → `C-ONB-01`
```

**Rationale**：回填 S-SRV-15 客户端契约（此前 FR 层缺口）。

---

#### 编辑 #11 · P11 · 新增 FR35d · `unlocked_pending_reveal` UI（`prd.md` §5 FR35c 之后，line 782 之后）

**NEW**（新增行）:
```
- **FR35d**：Watch 不可达时盲盒走满 1000 步进入 **`unlocked_pending_reveal` 中间态**（联动 `S-SRV-16`）；iPhone 仓库 UI 显示"已解锁·待揭晓（等待 Watch 上线）"占位；Watch 下次可达时 server 推送 `box.unlock.revealed` 事件触发开箱动画 `[U (fake Watch reachability), I]`
```

**Rationale**：回填 S-SRV-16 客户端契约（此前 FR 层缺口）。

---

#### 编辑 #12 · pre-existing bug 清理 · S-SRV-1..6 去重（`prd.md` line 903-908）

**OLD**（§Server-Driven New Stories 开头 6 行重复条目，无 `derived from`）：
```
- **S-SRV-1**：`pet.state.broadcast` 消息（含他人状态 + 隐私 gate 字段分级）
- **S-SRV-2**：`pet.emote.broadcast` fan-out + fire-and-forget 语义（60s 去重 + 房间订阅者 fan-out，不回发送方 ack）
- **S-SRV-3**：`room.effect.parkour` 等同屏特效 server 判定下发（Watch 禁本地计时触发）
- **S-SRV-4**：待推队列 TTL=24h + GC 审计日志
- **S-SRV-5**：皮肤合成系统（材料计数 + 合成规则引擎 + 事务性 ack）
- **S-SRV-6**：久坐召唤触发事件（`time-since-last-activity` 阈值 + haptic trigger event；**无情绪状态机**，纯 idle timer）
```

**NEW**：整块删除，保留 line 909-926 带 `derived from` 的正式条目。

**Rationale**：pre-existing 复制粘贴 bug；两份 S-SRV-1..6 造成阅读困惑。

---

#### 编辑 #13 · P12 · 删除 §PRD v2 修订注记 段落 + 追加 changelog（`prd.md` line 928-1055）

**OLD**：`## PRD v2 修订注记 (2026-04-20 · UX Step 10 Party Mode Outcomes)` 整个 section（约 126 行，包括 v2 修订 1-5 + 其他同步影响 + 跨仓 sync 待办 + v2 版本兼容性）

**NEW**：
1. 整段删除
2. 在 PRD 开头 `## Executive Summary` 之前追加一行 changelog：
```
> **Changelog**: 2026-04-20 · v2 · 回填 UX Step 10 Party Mode 5 处修订 + S-SRV-15..18 至正文 FR / 表格 / Journey / Capability / NFR。详细 diff 见 `sprint-change-proposal-2026-04-20.md`。
```

**Rationale**：D2=a 决策。回填完成后双源真相风险消除；审计轨迹通过 git + 本 proposal 保留，PRD 主体保持干净。

---

## Section 5 · Implementation Handoff

### 范围分级

**Moderate**（中等）——涉及 PRD 主体文档多点编辑 + 跨仓契约同步，但无代码/CI 影响。

### Handoff 对象

**单人项目** — Developer 同时承担 PM / PO / Architect / Dev 角色；handoff = 自我接力。

### 执行步骤（建议顺序）

| 步骤 | 指令 | 目的 | 前置依赖 |
|---|---|---|---|
| 1 | `/bmad-edit-prd` | 按本提案 Section 4 的 13 条编辑修改 `prd.md` | 本提案批准 |
| 2 | `/bmad-validate-prd` | 验证回填后一致性（v2 修订无残留 / FR 号稳定 / Capability 引用完整） | 步骤 1 完成 |
| 3 | （可选）精简 `CLAUDE.md` 根目录 "Pending Cross-Repo Action Items" 段 | 标注"PRD 已回填，待 server 团队接收" | 步骤 2 完成 + server 团队确认 |
| 4 | `/bmad-create-architecture` | 创建 iOS Architecture · 消化 C1-C7 约束 | 步骤 2 完成 |
| 5 | `/bmad-create-epics-and-stories` | 拆分 67 个 sub-FR 到 epic/story · 消化 C1-C7 约束 + 标注 `UserMilestones` 跨仓 blocking | 步骤 4 完成 |
| 6 | `/bmad-check-implementation-readiness` | 重跑 readiness · 期望 Step 3 不再 BLOCKED | 步骤 5 完成 |

### 跨仓依赖

- **S-SRV-15..18** 在本次对齐后已在 PRD 正文固化 → 下次跨仓 sync 会议推送给 server 团队确认契约 → server 团队排期到合适 epic（建议 `UserMilestones` 先行）
- `server-handoff-ux-step10-2026-04-20.md` 文档不删除，作为 server 团队的**单一接口文档**
- server 团队确认接收后，精简 `CLAUDE.md` 根目录 pending 段（简化为 "已交付，见 PRD"）

### 成功标准

- ✅ `prd.md` 应用完 13 条编辑后 `grep -n "v2 修订"` 只命中 changelog 一行
- ✅ `/bmad-validate-prd` 通过（无孤立 Capability 引用 / 无断裂的 S-SRV 引用）
- ✅ `/bmad-check-implementation-readiness` Step 3 从 BLOCKED 转 PASS（epic/story 已创建 + FR 覆盖矩阵可生成）

---

## Appendix · Checklist Summary

| Section | 状态 |
|---|---|
| 1. Trigger & Context | ✅ Done（3 触发事件 + 问题类型分类：Strategic pivot + New requirement） |
| 2. Epic Impact Assessment | ✅ Done（C1-C7 约束产出 + Epic 排序建议） |
| 3. Artifact Conflict Analysis | ✅ Done（P1-P12 识别 + UX 无冲突 + Architecture 待建） |
| 4. Path Forward Evaluation | ✅ Done（选 Option 1 Direct Adjustment） |
| 5. Proposal Components | ✅ Done（本文档 5 节） |
| 6. Final Review & Handoff | ⏳ Pending · 本提案待 Developer 签字批准 |

### 关键决策记录

- **D1**：FR55 盲盒频道开关 → **删除**（a · 纯净路径）
- **D2**：§v2 修订注记段落 → **整段删除 + 开头加 changelog**（a · 干净路径）
- **Path**：Option 1 · Direct Adjustment
- **Execution mode**：Incremental（Batch A+B+C+D+E+F 全部已逐批批准）

---

_生成于 `/bmad-correct-course` workflow · Step 4_
