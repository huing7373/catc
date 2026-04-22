---
name: Sprint Change Proposal · S-SRV-15~18 + 跨仓漂移吸收
date: 2026-04-21
author: Claude (bmad-correct-course 预案草稿)
project: server (裤衩猫后端)
scope_classification: Moderate (新增 1 个小 epic + 2 个 epic 内联修订 + 1 项横切 ADR + 跨仓 drift 同步)
status: draft-pending-team-review
affects:
  - _bmad-output/planning-artifacts/prd.md
  - _bmad-output/planning-artifacts/epics.md
  - _bmad-output/implementation-artifacts/sprint-status.yaml
  - docs/api/openapi.yaml（FR 落地时）
  - docs/api/ws-message-registry.md（FR 落地时）
  - docs/contract-drift-register.md（待建 / 跨端联合维护）
triggers:
  - iPhone UX Step 10 Party Mode 评审 → S-SRV-15/16/17/18 新契约（2026-04-20）
  - iPhone 架构哲学 B 升级（Server 为主，WC 为辅）
  - iPhone 架构 Known Contract Drift D1-D7（2026-04-21 调研）
  - 根 CLAUDE.md §"Pending Cross-Repo Action Items" 登记 2026-04-20
upstream_docs:
  - ios/CatPhone/_bmad-output/planning-artifacts/server-handoff-ux-step10-2026-04-20.md
  - ios/CatPhone/_bmad-output/planning-artifacts/architecture.md §Known Contract Drift + §Cross-Repo Sync Action Items
  - ios/CatPhone/_bmad-output/planning-artifacts/sprint-change-proposal-2026-04-20.md（对称参考）
downstream:
  - /bmad-edit-prd（追加 FR + NFR）
  - /bmad-create-epics-and-stories（新建 Epic 11 + Epic 5/6 story 修订 + 横切 ADR）
  - /bmad-sprint-planning（刷新 sprint-status.yaml）
  - /bmad-create-story（Epic 11 首 story）
---

# Sprint Change Proposal · S-SRV-15~18 吸收 + 跨仓漂移对齐

**模式**：Incremental（每 section 独立审批）
**路径**：Direct Adjustment（新增 + 修订，不回滚既有 epic）
**Epic 1 状态**：done（2026-04-19）· 本提案不影响已交付内容

---

## Section 1 · Issue Summary

### 问题陈述

iPhone UX Step 10 Party Mode 评审（5 视角 · Amelia/Murat/Sally/Winston/Victor）产生 **19 条修订决策**，其中 **4 条对 server 契约产生新需求**（S-SRV-15/16/17/18），以及 **7 项跨仓契约漂移**（D1-D7）登记。server PRD / epics / sprint-status 目前对这 4 条 **零实现 · 零痕迹**；iPhone 架构侧已明确标注 "阻塞 Story 1.1 inception" 与 "阻塞 Epic 1/2 部分 story"。

### 发现背景

- iPhone handoff 文档 `server-handoff-ux-step10-2026-04-20.md` 2026-04-20 落地
- iPhone 架构 `architecture.md` Step 7 Known Contract Drift 2026-04-21 调研交付，Status 从 READY 降级为 CONDITIONALLY READY
- 根 CLAUDE.md 已登记 pending 4 条、标注 "server 团队请在下次 epic 规划前处理"
- server 侧 grep：`milestone|unlocked_pending|S-SRV|emote.delivered` → **0 match**（prd.md / epics.md 均无）

### 关键事实更正（反向同步 iPhone 团队）

**iPhone 架构 Drift 表 D5 条 "SIWA+JWT refresh 未实装" 已过期**：

| Drift ID | iPhone 架构记录 | server 实际状态 |
|---|---|---|
| D5 | "未实装；Debug 模式任意 Bearer token = userID；Release 模式 401" | **已实装 2026-04-19**（Epic 1 done · Stories 1.1-1.6 全部 merged · SIWA + refresh rotation + JWT middleware + APNs token + profile + account deletion 六件套 + tools/process_deletion_queue 生产 ops CLI） |

**影响**：iPhone 架构"挂 server gate"的 Story 1.3a/b/c/1.5/1.7/1.8 **其实已可启动**。需跨仓 sync 会议议题 2 重新校准 —— 从 "定 Story 1.8 unblock 日期" 转为 "iPhone 立即可 dewire mock auth，接入 openapi 1.6.0-epic1"。

### 影响证据

| ID | 项 | 证据 |
|---|---|---|
| R1 | S-SRV-15 user_milestones 零实现 | 无 `user_milestones` collection / 无 `/v1/user/milestones` endpoint / 无 domain type |
| R2 | S-SRV-16 unlocked_pending_reveal 零实现 | 盲盒状态机在 epics.md §6.1 仅 `pending/redeemed` 两态 |
| R3 | S-SRV-17 emote.delivered ack 未显式禁止 | epics.md §5.2 未显式标"发送者不收 ack"；需 AC 加硬约束防后人加回 |
| R4 | S-SRV-18 metric 清单零实现 | Epic 0 `logx` 有结构化日志，无 Prometheus exporter / 无 metric name registry |
| R5 | D1 dedup TTL 漂移 | server 300s（`config/default.toml:35 dedup_ttl_sec = 300`）是权威，iPhone PRD 60s 需改；server 侧无动作 |
| R6 | D7 action.update 幂等缺失 | Epic 10 MVP RoomManager 设计如此；Epic 4.1-4.5 上线时整块删除，D7 届时 supersede |

---

## Section 2 · Impact Analysis

### Epic Impact（现状 vs 本提案后）

| Epic | 状态 | 本提案影响 |
|---|---|---|
| Epic 0 · 平台骨架 | done | 无影响 |
| Epic 1 · 身份与账户 | done | 无影响（Epic 1 已收官，不重开） |
| Epic 2 · 猫活着 | backlog | 无影响（S-SRV-18 metric 登记是 Dev Notes 层追加，不改 AC 结构） |
| Epic 3 · 好友圈 | backlog | 同上 |
| Epic 4 · 房间 / 在场 | backlog | D7 超前 AC（action.update 幂等 + 宽限期）为 Epic 4.1 硬需求 |
| Epic 5 · 触碰 / 触觉社交 | backlog | **S-SRV-17 合并入 Story 5.2 AC**（硬约束：发送者不收 emote.delivered） |
| Epic 6 · 盲盒 | backlog | **S-SRV-16 合并入 Story 6.2 + 6.4，新增 Story 6.5**（Watch 重连揭晓） |
| Epic 7 · 皮肤 | backlog | 无直接影响 |
| Epic 8 · 冷启动 | backlog | 无影响 |
| Epic 9 · Spike 真机 | in-progress | 无影响 |
| Epic 10 · 联调 MVP | done | 无影响（D7 由 Epic 4 supersede） |
| **Epic 11 · 用户里程碑（新建）** | — | **本提案新建** · 承载 S-SRV-15 · 2-3 story · **iPhone Epic 1 硬依赖 → 排 Epic 2 之前** |

### 横切 ADR（新增）

| ADR | 承载 | 归属 |
|---|---|---|
| **ADR-007 · Fail-Node 可观测性三元组 metric** | S-SRV-18 8 个 metric（`ws_ack_timeout_total` / `box_unlock_timeout_total` / `craft_fail_total` / `registry_fetch_failed_total` / `room_invite_expired_total` / `milestone_update_conflict_total` / `box_unlocked_pending_reveal_count` / `watch_reachability_false_ratio`） | 每 Epic 新增 fail 分支 story 在 Dev Notes 登记对应 metric name；收敛到 `internal/metrics` 单包；落地节奏随 epic 自然推进，不独立切 epic |

### Artifact 冲突

| Artifact | 冲突 | 操作 |
|---|---|---|
| `prd.md` §FR + §Success Criteria + §WS Message Registry + §NFR Observability | 需追加 FR / metric 清单 / WS 消息 | `/bmad-edit-prd` 应用（Section 4 清单） |
| `epics.md` §Epic List + §Epic 5 + §Epic 6 | 需插入 Epic 11 + Story 5.2 AC 修订 + Story 6.2/6.4/6.5 修订 | `/bmad-create-epics-and-stories` additive 运行 |
| `sprint-status.yaml` | Epic 11 登记 + 建议排序 | `/bmad-sprint-planning` 或手工追加 |
| `docs/api/openapi.yaml` | `/v1/user/milestones` GET/POST + `/v1/boxes/pending-reveal` GET | Epic 11 / Epic 6 story 落地时并发修改（双 gate §21.1） |
| `docs/api/ws-message-registry.md` | `box.unlock.revealed` 新消息 + `box.state` unlocked_pending_reveal 枚举扩展 | 同上 |
| `docs/contract-drift-register.md` | **不存在 · 待建**（跨端联合维护，iPhone 架构师起 2026-Q2） | 本提案建议 server 团队在跨仓 sync 会议 approve 维护权后立即建骨架（承载 D1-D7） |
| `docs/architectures/README.md` | 已在 2026-04-21 由 iPhone 架构师创建 | 更新 "server 架构师" 列（目前 TBD） |
| 根 `CLAUDE.md` "Pending Cross-Repo Action Items" | Epic 11 排期确定后精简此段 | 延后至本提案 approve + Epic 11 Story 排期锁定 |

### Technical Impact

- **兼容性**：Epic 11 新 collection + 新 endpoint；Epic 5/6 修订不破坏已 done 的 Epic 1 契约；Release/Debug gate 保持
- **跨端契约漂移**：双 gate §21.1（errCodes / WS msg types / feature flags / Redis key prefixes）必须 pass；openapi + ws-message-registry 每 PR 并发改
- **§21.2 Empty Provider 逐步填实**：S-SRV-18 的 `MetricsRecorder` 接口建议先 Noop，Epic 2-8 每 fail 分支 story 填实对应分支
- **§21.3 fail-closed vs fail-open**：
  - S-SRV-15 milestone 幂等冲突 → fail-closed（不允许覆盖已设里程碑 · 语义正确性刚需）
  - S-SRV-15 query 失败 → 客户端侧 fail-open 已在 handoff 文档约定；server 侧无策略点
  - S-SRV-16 Watch 不可达判定失败 → fail-closed 推 `unlocked_pending_reveal`（Watch 重连补揭晓，不丢皮肤）
- **§21.4 语义正确性 AC review 早启**：
  - Epic 11 Story 1.1 milestone 幂等是 measurement/guard 类 → AC review 必须在实现前跑
  - S-SRV-18 metric 名 / label / 类型（Counter vs Gauge）是 measurement 类 → ADR-007 确认时跑
- **§21.7 测试自包含**：Epic 11 所有测试通过 `go test` 本地绿；无需真 iPhone

---

## Section 3 · Recommended Approach

### 选定路径：**Direct Adjustment**

（Rollback 不可行 —— Epic 1 已收官 · S-SRV-15~18 是前进方向；MVP Review 不单列 —— MVP 范围反而收窄了一点点因为 S-SRV-17 删了 ack 推送）

### Epic 11 设计建议（2-3 story）

> 完整 AC 在 `/bmad-create-epics-and-stories` 阶段产出；此处只列切片原则

**Story 11.1 · UserMilestones Domain + Collection + 幂等 API**
- 领域：`internal/domain/user_milestone.go`（kind enum: `onboarding_completed / first_emote_sent / first_room_entered / first_craft_completed`）
- Repo：MongoDB `user_milestones` collection（按 handoff 文档 schema）· EnsureIndexes 启动期调用
- Service：`UserMilestoneService.Set(ctx, userID, kind, roomID?, clientMsgID)` 幂等（already_set_at 返回原时间戳）
- Handler：GET/POST `/v1/user/milestones`（走 Story 1.3 JWTAuth middleware）
- 幂等：`client_msg_id` 60s dedup（**注：统一对齐 server 的 300s dedup window**（D1）—— handoff 文档写 60s 是基于 iPhone PRD 的过期假设）
- §21.1 双 gate：kind enum + 错误码 `MILESTONE_ALREADY_SET` 进 `error_codes` 全局注册表
- §21.3：幂等冲突 fail-closed 不覆盖（已设的里程碑是 monotonic）+ `milestone_update_conflict_total{kind}` 打点（S-SRV-18 首个 metric 落地点）
- §21.8 语义正确性思考题：设想题 6 条（覆盖过 / kind 拼写 / client_msg_id 跨用户碰撞 / time-before-epoch / first_room_entered 同 room_id 重设 / index 缺失下的线上压测爆炸）

**Story 11.2 · `internal/metrics` Prometheus exporter 骨架 + ADR-007 Fail-Node Metric Registry**
- Empty Provider：`MetricsRecorder` 接口定义（Counter/Gauge/Histogram 三方法）
- 先落 `milestone_update_conflict_total{kind}` 一处 · 其他 7 个 metric 占位 TODO 挂到对应 epic story
- 双 gate §21.1：metric name registry（`internal/metrics/registry.go`）+ 文档（新建 `docs/metrics-registry.md`）
- 不独立切 epic，因为 metric 落地节奏与业务 epic 绑定；此处先立骨架

**Story 11.3（可选）· Client guide §user_milestones + openapi 1.7.0-epic11 + 客户端验证**
- 文档交付（双 gate 第二道门）
- 若跨仓 sync 会议约定 iPhone 先行集成，可拆独立 PR

### Epic 5 · Story 5.2 AC 修订（S-SRV-17）

在 Story 5.2 AC 追加：
```gherkin
AND 发送者不收 `emote.delivered` ack 事件 —— fire-and-forget 硬约束
   （UX Party Mode A4 决策 · 违反产品哲学"社交信号无身份 / 无历史 / 无义务"）
AND 未来任何 PR 不得恢复 delivery ack 推送路径到发送者
AND server 侧独立 log 结构化事件 `emote_processed {from_user_id, from_device_type, room_id, target_device_count, target_has_watch_user}` 代替（产品分析 channel）
```

### Epic 6 · Story 6.2 / 6.4 修订 + 新增 Story 6.5（S-SRV-16）

**Story 6.1 修订**：`BlindboxStatus` enum 扩展 `BlindboxStatusUnlockedPendingReveal = "unlocked_pending_reveal"`（三态而非两态）

**Story 6.2 修订**：drop 流程不变 · 但 stepsRequired 达成判定后的状态转移改为：
- Watch 可达 → `redeemed`（原路径）
- Watch 不可达 → `unlocked_pending_reveal` + 推 `box.state.updated` 给 iPhone（不含 skin 信息）+ `box_unlocked_pending_reveal_count` gauge +1

**Story 6.4 修订**：Redeem 流程在 preflight 处加分支：若 state 是 `unlocked_pending_reveal` 则拒绝（已 settle · 等 Watch 揭晓）

**新增 Story 6.5 · Watch 重连揭晓**：
- WS onConnect hook：查 `boxes.state == unlocked_pending_reveal AND userId = X` 列表
- 依次推 `box.unlock.revealed` 事件到 Watch · 状态转 `redeemed` · gauge -1
- GET `/v1/boxes/pending-reveal`（Watch 主动拉取兜底 · WS 事件漏时的降级路径）
- Watch 可达判定：方案 B（纯 server 推断 · 无活跃 Watch WS 连接 = 不可达）· iPhone 架构确认 MVP 方案 B 足够
- §21.3 fail-closed：推不到 Watch 保持 `unlocked_pending_reveal`（下次重连再推）· 绝不自动升 redeemed
- §21.4 AC review：必须在实现前跑（guard / measurement 类）

### ADR-007 · Fail-Node Metric Registry

- 新文档 `docs/metrics-registry.md` 承载 8 个 metric（name / type / label / 触发 PR）
- 每新 fail 分支 PR checklist 追加一条 §21.8 追问："此 fail 对应的 metric name 是什么？"
- `internal/metrics` 包提供 `Recorder` 接口 + Prometheus impl + Noop impl（§21.2）
- `cmd/cat` 注入 `/metrics` HTTP endpoint（prometheus text format）
- 不独立切 epic · 随 Epic 2-8 自然落地

### 跨仓 D1-D7 drift 吸收路径

| Drift | server 动作 | iPhone 侧配合 | Owner |
|---|---|---|---|
| D1 dedup TTL 60s/300s | **无**（server 300s 正确） | PRD 改 300s · 本地缓存窗口对齐 | iPhone |
| D2 TTL 24h + last_ack_seq | **无**（MVP 不做） | PRD Cross-Device Messaging Contract 整段重写 · 降级为 "重连 + 可选 resume + 重 join" | iPhone |
| D3 业务 WS 消息 release 空 | **无**（Epic 2-8 自然填实） | ws-registry 运行时发现 · 预期 | iPhone |
| D4 业务 HTTP 空 | **无**（MVP 走 WS · Epic 11 /milestones 是 HTTP 例外） | HTTP client 占位 · 预期 | iPhone |
| **D5 SIWA+JWT refresh** | **反向通知 iPhone Epic 1 已实装**（2026-04-19） | Drift 表更新 · mock auth dewire · 接 openapi 1.6.0-epic1 | iPhone（重大喜讯） |
| D6 S-SRV-15~18 | **本提案即吸收** | 跨仓 sync 会议 approve 交付时间线 | server |
| D7 action.update 幂等 + 宽限期 | Epic 4.1 设计时吸收（`action.update` 幂等 key + presence 宽限期） | 重连后禁无脑重发 action | server（Epic 4） |

---

## Section 4 · Proposed Changes（清单）

### PRD edits（/bmad-edit-prd 输入）

1. **§FR · 身份与鉴权**：无新增（Epic 1 done）
2. **§FR · 新增 §账号级里程碑**：FR64 用户可上报账号级里程碑 / FR65 server 幂等拒绝覆盖 / FR66 查询里程碑 · 引 S-SRV-15
3. **§FR · 盲盒**：FR32 修订 + 新增 FR67 Watch 不可达时 server 延迟揭晓态 + FR68 Watch 重连自动揭晓 · 引 S-SRV-16
4. **§FR · 触碰 / 触觉社交**：FR24（emote）追加硬约束 "发送者不收 delivered ack" · 引 S-SRV-17
5. **§NFR · Observability**：NFR-OBS-6 新增 · 8 个 Prometheus metric 清单 · 引 S-SRV-18
6. **§WS Message Registry**：追加 `box.unlock.revealed` · `box.state` 枚举扩展 `unlocked_pending_reveal`
7. **§Endpoint Specifications — HTTP**：追加 `GET/POST /v1/user/milestones` + `GET /v1/boxes/pending-reveal`
8. **§Data Schemas**：追加 `user_milestones` collection schema
9. **§Error Codes**：追加 `MILESTONE_ALREADY_SET` · `BLINDBOX_NOT_YET_REVEALED`
10. **§Epic 拆分预览**：追加 Epic 11 · 修订 Epic 5/6 story 清单

### Epics edits（/bmad-create-epics-and-stories 输入）

11. §Epic List 追加 Epic 11
12. Epic 5 §Story 5.2 AC 追加 S-SRV-17 硬约束
13. Epic 6 §Story 6.1 enum 扩展 + §6.2 分支 + §6.4 preflight + 新 §6.5
14. 新增 Epic 11 完整 story 定义（AC 含 §21.1 双 gate + §21.3 fail 策略 + §21.4 AC review 锚点 + §21.8 思考题）

### Sprint-status.yaml edits

15. 追加 `epic-11: backlog` + 3 story
16. 更新顶部注释登记 Epic 11 创建
17. Epic 排序建议注释：Epic 11 优先级 > Epic 2（iPhone Epic 1 硬依赖）

### Cross-repo artifacts

18. 建 `docs/contract-drift-register.md` 骨架（iPhone 架构师 2026-Q2 轮值 owner）· 承载 D1-D7 + 未来新增
19. `docs/architectures/README.md` 更新 server 架构师列（目前 TBD）
20. openapi.yaml bump 1.7.0-epic11（Epic 11 落地 PR 时）
21. ws-message-registry.md §box.unlock.revealed + §box.state 枚举扩展（Epic 6.5 落地 PR 时）

### CLAUDE.md 精简（条件：本提案 approve + Epic 11 Story 排期锁定）

22. 根 `CLAUDE.md` §"Pending Cross-Repo Action Items · 2026-04-20" 段落删除或精简为 "已吸收到 server sprint-change-proposal-2026-04-21 + Epic 11"

---

## Section 5 · Execution Plan

### 立即（本提案 approve 后当周）

1. **跨仓 sync 会议 60 min**（iPhone 架构 Step 7 精简版 3 议题）· 本提案作为 server 侧输入：
   - 议题 1：S-SRV-15 `user_milestones` 排期 → server 明确 Epic 11 排 Epic 2 之前
   - 议题 2：**SIWA + JWT refresh 实装节奏 → 反向同步 iPhone：Epic 1 已 done 2026-04-19 · 可立即接入 openapi 1.6.0-epic1**
   - 议题 3：`contract-drift-register.md` 维护权（iPhone 架构师 2026-Q2 轮值）+ S-SRV-16/17/18 epic 归属（本提案答案）
2. `/bmad-edit-prd` 应用 10 条 PRD edits（Section 4.1-4.10）
3. `/bmad-create-epics-and-stories` 增量运行 · 产出 Epic 11 + Epic 5/6 修订
4. `/bmad-sprint-planning` 刷新 sprint-status.yaml · Epic 11 排优先级
5. `/bmad-create-story` 产出 Epic 11 Story 11.1

### 短期（Epic 11 落地期）

6. Story 11.1 dev（`bmad-dev-story`）· AC review 早启（§21.4）
7. Story 11.1 done + 客户端通知 iPhone Epic 1 Onboarding 集成点解除
8. Story 11.2 · `internal/metrics` 骨架 + ADR-007 + `milestone_update_conflict_total` 首个落地
9. CLAUDE.md 精简

### 中期（Epic 5/6 排期到时）

10. Story 5.2 实现时吸收 S-SRV-17 硬约束 + log 结构化事件
11. Story 6.1-6.5 实现时吸收 S-SRV-16（三态 FSM + 新 WS 消息 + Watch 重连揭晓）· 每 fail 分支补 metric 打点
12. Epic 4.1 设计时吸收 D7（`action.update` 幂等 + presence 宽限期）

### 跨 epic 横切（S-SRV-18 metric 清单）

13. 每个 fail 分支 PR（Epic 2-8）在 Dev Notes 登记对应 metric name · checklist 追加 §21.8 追问
14. `/metrics` endpoint 暴露 · 部署侧加 Prometheus scrape 配置（Epic 9 归属）

---

## Section 6 · Trade-offs Acknowledged

- ❌ **未选"S-SRV-15 塞 Epic 1 重开"**：Epic 1 已收官（retro done）· 重开破坏纪律 · 独立小 epic（Epic 11）更清洁
- ❌ **未选"S-SRV-18 独立切 epic"**：metric 本质横切 · 绑定业务 fail 分支自然落地 · 强独立切 epic 会制造"monster guards epic"陷阱（Epic 0 retro 教训）
- ❌ **未选"Epic 11 塞 Epic 10 扩展"**：Epic 10 是 deletion target（Epic 4.1 上线时整块删）· 塞就没法删
- ❌ **未选"推迟 S-SRV-15 到 Epic 8 后"**：iPhone Epic 1 硬依赖 · 推迟 = iPhone 团队空转
- ✅ **选 "新小 Epic 11 + Epic 5/6 内联修订 + ADR-007 横切"**：影响面最小 · 排期最清晰 · 与 iPhone 团队耦合面最窄

---

## Section 7 · Effort & Risk

| 维度 | 评估 |
|---|---|
| **文档编辑 Effort** | 🟢 Medium · PRD 10 条 + Epics 4 条 + sprint-status 3 条 + openapi/ws-registry 2 条 + contract-drift-register 骨架 · 预计 4-6h |
| **Epic 11 实现 Effort** | 🟡 Medium · 2-3 story · 预计 1-2 周（Story 11.1 是重头） |
| **Epic 5/6 修订 Effort** | 🟢 Low-Medium · AC 修订 + 状态机扩展 + 新 Story 6.5 · 落地在对应 epic 排期内吸收 |
| **ADR-007 Effort** | 🟢 Low · 骨架 1 天 · 具体 metric 每 epic 增量 |
| **跨仓影响** | 🟡 Medium · 需 60 min sync 会议 + iPhone 架构 Drift 表更新（D5 修订） |
| **Timeline Impact** | Epic 2 开工延后约 1-2 周（Epic 11 前置） · 但 iPhone Epic 1 因此解封 · 净跨端价值为正 |
| **Risk** | 🟢 Low · 纯 additive 设计 · 无回滚 · §21 纪律全覆盖 |

---

## Section 8 · Approval Checklist

- [ ] Epic 11 位置（排 Epic 2 之前）同意
- [ ] S-SRV-17 合并入 Story 5.2 AC（不独立 story）同意
- [ ] S-SRV-16 合并入 Epic 6 + 新增 Story 6.5 同意
- [ ] S-SRV-18 走 ADR-007 横切 · 不独立 epic · 同意
- [ ] Milestone 幂等 dedup 窗口对齐 server 300s（而非 handoff 文档 60s）同意
- [ ] 跨仓 sync 会议议题 2 转为"反向同步 iPhone Epic 1 已实装"同意
- [ ] D1-D7 处置方向（表 4.Section 3）同意
- [ ] `contract-drift-register.md` 由 iPhone 架构师轮值维护同意
- [ ] 启动 `/bmad-edit-prd` + `/bmad-create-epics-and-stories` 增量运行授权

---

## Appendix A · 与 iPhone sprint-change-proposal-2026-04-20 对照

| iPhone 约束 | server 吸收位置 |
|---|---|
| C1 盲盒无 push | server 无动作（Epic 6 本就是 WS-only drop/redeem） |
| C2 WC 仅 fast-path | S-SRV-16 方案 B（server 推断 Watch 可达） |
| C3 Watch 独立 SIWA | server 已支持（Epic 1 per-device session） |
| **C4 user_milestones 客户端集成** | **Epic 11 server 侧实装** |
| **C5 unlocked_pending_reveal UI** | **Epic 6 server 侧实装** |
| **C6 emote.delivered 无 UI** | **Epic 5 Story 5.2 AC 硬约束** |
| **C7 fail 节点 metric** | **ADR-007 + Epic 2-8 Dev Notes 横切** |

---

**本草稿 owner**：server 团队（TBD 架构师接手）
**下一步**：团队评审 → approve → 跨仓 sync 会议 → `/bmad-edit-prd` 执行
**参考文档**：
- `ios/CatPhone/_bmad-output/planning-artifacts/server-handoff-ux-step10-2026-04-20.md`
- `ios/CatPhone/_bmad-output/planning-artifacts/architecture.md` §Known Contract Drift + §Cross-Repo Sync Action Items
- `ios/CatPhone/_bmad-output/planning-artifacts/sprint-change-proposal-2026-04-20.md`（对称路径参考）
- `docs/backend-architecture-guide.md` §21 工程纪律
- `_bmad-output/implementation-artifacts/epic-0-retro-2026-04-19.md`
