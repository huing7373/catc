---
type: cross-repo-handoff
from: iPhone UX Step 10 Party Mode
to: Server team
date: 2026-04-20
status: pending-sync
priority: medium-to-high
requires: sync-meeting-before-implementation
---

# Server Handoff · iPhone UX Step 10 Party Mode Outcomes

## 背景

iPhone 端 UX 设计规范（`ios/CatPhone/_bmad-output/planning-artifacts/ux-design-specification.md`）在 **Step 10 User Journey Flows** 的 Party Mode 评审中（召集 Amelia / Murat / Sally / Winston / Victor 5 视角），共识出 **19 条修订决策**。其中 4 条对 server 契约产生新需求，归档为 **S-SRV-15~18**。

**核心架构哲学升级**：**"哲学 B · Server 为主，WC 为辅"** —— 所有跨端状态走 server 单一真相源；WatchConnectivity 降级为可选 fast-path 优化层，不再是 Watch-iPhone 主通道。

**强烈建议**：server 团队在 iOS 开始 Epic 1 前完成 sync 会议对齐契约。

---

## S-SRV-15 · UserMilestones Collection

### 背景

iPhone 端多个"首次"状态需要跨设备、跨换机保持一致（首次发表情 / 首次进每个房间等）。本地 UserDefaults 不够——换机会丢失，引发用户二次看引导的糟糕体验。

### 需求

新建 MongoDB collection `user_milestones`，记录**账号级里程碑**。

### Schema

```javascript
{
  _id: ObjectId,
  user_id: String,           // 关联 users collection
  onboarding_completed_at: DateTime?,
  first_emote_sent_at: DateTime?,
  first_room_entered: {       // 对象，按 room_id 索引
    "<room_id_1>": DateTime,
    "<room_id_2>": DateTime,
    // ...
  },
  first_craft_completed_at: DateTime?,
  created_at: DateTime,
  updated_at: DateTime
}
```

### API

#### 1. 查询里程碑

```http
GET /v1/user/milestones
Authorization: Bearer <jwt>

Response 200:
{
  "onboarding_completed_at": "2026-04-20T10:00:00Z",
  "first_emote_sent_at": null,
  "first_room_entered": {
    "room-abc": "2026-04-21T15:30:00Z"
  },
  "first_craft_completed_at": null
}
```

#### 2. 上报里程碑（幂等）

```http
POST /v1/user/milestones
Authorization: Bearer <jwt>
Content-Type: application/json

Request:
{
  "kind": "first_emote_sent",  // or "onboarding_completed", "first_room_entered", "first_craft_completed"
  "room_id": "room-abc",       // 仅当 kind == "first_room_entered" 时需要
  "client_msg_id": "uuid-v4"    // 幂等 key
}

Response 200:
{
  "updated": true,
  "already_set_at": null  // 如已经被设过则返回原时间戳，表示幂等 skip
}
```

**幂等纪律**：
- 同 `client_msg_id` 60s 内去重（对齐现有 WS envelope 60s 去重窗口）
- 已设过的里程碑**不允许覆盖**（`updated: false, already_set_at: <原时间>`）

### iPhone 端行为

客户端在以下时刻**主动 POST** `/v1/user/milestones`：
- Onboarding 完成（命名猫完成）→ `onboarding_completed`
- 首次点击"我的猫"发表情（发送动作触发那一秒）→ `first_emote_sent`
- 首次进入某个房间（成功进入 room_id 的那一秒）→ `first_room_entered`
- 首次合成完成 → `first_craft_completed`

启动时 `GET` 一次缓存到本地（用于 UI 决定是否显示 tooltip / 引导）。

**禁止**客户端本地持久化替代此 API —— 账号级里程碑**必须 server 权威**。

### §21 纪律对齐

- §21.1 双 gate 漂移守门：里程碑 kind 常量需和 iOS / watchOS 同步
- §21.3 fail-closed vs fail-open：query 失败 → 客户端 fail-open（假设未设过，弹引导；用户体验可接受因 UserDefaults 兜底一次）
- §21.7 测试自包含：server 端 `go test` 覆盖幂等 + schema 校验

---

## S-SRV-16 · box.state 新增 `unlocked_pending_reveal` 态 + Watch 重连触发揭晓

### 背景

iPhone UX Party Mode A5' 决策：**Watch 不可达时盲盒解锁延迟揭晓**（iPhone 不接管开箱动画，保 PRD `C-BOX-03`）。这要求 server 盲盒状态机新增一个中间态。

### Schema 变更

`boxes` collection 的 `state` 字段枚举扩展：

| State | 原 | 新 | 含义 |
|---|---|---|---|
| `pending` | ✅ | ✅ | 已掉落、待解锁（需 1000 步） |
| `unlocked` | ✅ | ✅ | 已完全揭晓、入仓库已解锁区 |
| **`unlocked_pending_reveal`** | ❌ | ✅ **新增** | 已走满 1000 步、server 已校验，但 Watch 不可达，等待 Watch 重连揭晓 |

### 状态转移

```
pending → unlocked (Watch 可达时，走满 1000 步且 server 校验通过)
pending → unlocked_pending_reveal (Watch 不可达时，同上条件)
unlocked_pending_reveal → unlocked (Watch 重连时 server 自动触发)
```

### 判定 Watch 是否可达

server 需要知道"此刻用户的 Watch 是否在线"。由 iPhone 端通过 WS envelope 的 `meta.watch_reachable: bool` 字段上报（见下文"WS Envelope 变更"）。

**降级**：若 server 无法确定（用户只在 APNs 侧有连接），默认按 Watch 不可达处理 → `unlocked_pending_reveal`。

### Watch 重连触发揭晓

Watch WS 重连后，server 查询该用户 `boxes.state == 'unlocked_pending_reveal'` 的盲盒列表 → 依次通过 WS 推送揭晓事件到 Watch → Watch 播放开箱动画。

#### 新增 WS event

```typescript
// server → Watch
{
  type: "box.unlock.revealed",
  payload: {
    box_id: "xxx",
    skin_id: "yyy",
    skin_name: "条纹裤衩",
    rarity: "blue",
    // 完整 payload，Watch 播揭晓动画
  }
}
```

### API

#### 查询待揭晓列表（Watch 重连后用）

```http
GET /v1/boxes/pending-reveal
Authorization: Bearer <jwt>

Response 200:
{
  "boxes": [
    {"box_id": "xxx", "skin_id": "yyy", "skin_name": "...", "rarity": "blue"}
  ]
}
```

### iPhone 端展示

iPhone 订阅 WS `box.state` 变更：
- `pending → unlocked_pending_reveal`：仓库 pending 卡片更新为"接回家了 · 戴上 Watch 看揭晓"（不剧透皮肤名）
- `unlocked_pending_reveal → unlocked`（Watch 重连揭晓后）：仓库更新为已解锁卡片，显示皮肤

---

## S-SRV-17 · 取消 `emote.delivered` 发送者 ack 推送

### 背景

iPhone UX Party Mode A4 决策：**fire-and-forget 对称性**要求 server **不能推回** `emote.delivered` 事件给发送者，否则违反产品哲学"社交信号无身份 / 无历史 / 无义务"。

### 变更要求

- Server `pet.emote.broadcast` 处理逻辑里，**不要**向发送者推送 delivery ack 事件
- Fan-out 只发给**接收方**（房间其他成员）
- 发送者的 POST `/emote` 返回 202 accepted 即可（无进一步 WS 推送）

### 如果已经实现 delivered 推送

立即删除推送路径。Story AC 里写明："发送者不收 emote.delivered 事件 —— 这是 fire-and-forget 硬约束，未来任何 PR 不得恢复"。

### Server 侧独立追踪（产品分析）

"observer → 完整用户转化"等分析指标仍需追踪，通过 server 独立 log 完成（不影响客户端）：

```go
// 每次 emote 处理时 log 结构化事件
Log.emote.info("emote_processed", map[string]any{
    "from_user_id": ...,
    "from_device_type": "iphone_only" | "watch_paired",
    "room_id": ...,
    "target_device_count": N,
    "target_has_watch_user": true | false,
})
```

分析阶段跑 SQL/metric 查询，不占用 client / 跨端 bandwidth。

---

## S-SRV-18 · 所有 fail 节点 Prometheus metric 打点

### 背景

iPhone UX Party Mode B1 决策：**所有 fail 节点 AC 必须三元组 `(超时阈值, UI 终态, 可观测 metric)`**。客户端侧已约束；server 侧需要对应 metric 打点。

### 新增 metric 清单（建议 Prometheus）

| Metric 名 | 类型 | Label | 触发场景 |
|---|---|---|---|
| `ws_ack_timeout_total` | Counter | `{ack_type, route}` | WS 消息超时未 ack |
| `box_unlock_timeout_total` | Counter | `{reason}` | 盲盒解锁 server 校验超时 |
| `craft_fail_total` | Counter | `{reason}` | 合成失败 |
| `registry_fetch_failed_total` | Counter | `{reason}` | `/v1/platform/ws-registry` 失败 |
| `room_invite_expired_total` | Counter | 无 | 房间邀请过期 |
| `milestone_update_conflict_total` | Counter | `{kind}` | `UserMilestones` 幂等冲突 |
| `box_unlocked_pending_reveal_count` | Gauge | 无 | 当前等待 Watch 揭晓的盲盒数 |
| `watch_reachability_false_ratio` | Gauge | 无 | Watch 不可达比例（从 envelope.meta 汇总） |

### §21.3 对齐

每个 fail 分支 PR 必须回答"跑通但结果错会误导谁？" —— metric 打点是答案的一部分（审计可见）。

---

## WS Envelope 新增字段（可选，取决于是否保留 WC fast-path）

### 背景

哲学 B 下，WC 降级为可选优化，但 server 仍需要知道用户 Watch 是否在线来决定推送策略（如 A5' 中的 `unlocked_pending_reveal` 状态判定）。

### 方案 A · 保留 `envelope.meta.watch_reachable`

iPhone 通过 WS envelope 上报：

```json
{
  "id": "msg-xxx",
  "type": "action.xxx",
  "meta": {
    "watch_reachable": true,        // iPhone 端通过 WCSession 本地判定
    "client_version": "ios-1.2.3"
  },
  "payload": {...}
}
```

server 端 `internal/ws/envelope.go` 的 `Envelope.Meta` struct 加 `WatchReachable *bool` 字段（nullable，兼容旧 client）。

### 方案 B · 纯 server 推断

完全不依赖 client 上报，server 通过是否有活跃 Watch WS 连接推断。

**权衡**：
- 方案 A 更准确（client 知道 WC 状态）但需要 client 实现 WC probing（和哲学 B "WC 可选" 略冲突）
- 方案 B 完全 server-centric 但可能有 race condition（Watch 刚断 server 还没感知）

### iPhone 端立场

**可以实现方案 A**，但**不是 Epic 1 硬需求**。MVP 可以先走方案 B，Watch 不可达时 server 始终按"不可达"推测（反正 A5' 的 `unlocked_pending_reveal` 态下次 Watch 上线也会补推揭晓）。

方案 A 作为 Growth 优化保留。

---

## Story AC 模板（Fail 节点三元组）

Server 侧涉及 fail 分支的 story，AC 必须包含：

```gherkin
Scenario: <Fail 场景名>
  Given <前置条件>
  When <外部失败>
  Then <action>
    And <UI 终态>（由 server 返回标准 error code 或保持 pending）
    And Log <structured log 行，含 metric label>
    And Prometheus metric <metric_name>_total 计数 +1

示例：
Scenario: 盲盒解锁时 Watch 不可达
  Given box.state = 'pending' and steps_met = true
  When server 校验通过但 envelope.meta.watch_reachable = false
  Then box.state 转为 'unlocked_pending_reveal'
    And WS 推送 'box.state.updated' 给 iPhone（不含 skin 信息）
    And Log box.info("unlocked_pending_reveal", {"user_id": X, "box_id": Y})
    And box_unlocked_pending_reveal_count gauge +1
```

---

## Sync 会议建议议程

建议召集跨仓 sync 会议（约 60 min）：

1. **契约评审**（30 min）：
   - `UserMilestones` schema + API（S-SRV-15）
   - `box.state` 新态 + 揭晓事件（S-SRV-16）
   - emote.delivered 取消（S-SRV-17）
   - Metric 清单（S-SRV-18）
   - WS envelope 变更方案 A/B 选择
2. **依赖关系**（15 min）：
   - Epic 排期 → server 先上 `UserMilestones` → iPhone Epic 1 / 2 可并行开发
   - `box.state` 新态是 Epic 3（盲盒）硬依赖
3. **PRD 修订**（15 min）：
   - server PRD 是否需要同步更新
   - 跨仓 PRD 版本号同步策略

**会议前读档（必读）**：
- 本 handoff 文档
- iPhone UX 规范 §Step 10 · Party Mode Revisions v0.2
- iPhone UX 规范 §Component Strategy + §UX Patterns 相关章节

---

## 附录 · 完整决策 traceability

| S-SRV | UX 规范决策 ID | 对应 UX Pattern |
|---|---|---|
| S-SRV-15 | A3 完整方案 | Pattern · 里程碑 server 权威 |
| S-SRV-16 | A5' 延迟揭晓 | Pattern · 延迟揭晓 |
| S-SRV-17 | A4 拒绝 | Pattern · fire-and-forget 对称性 |
| S-SRV-18 | B1 三元组 AC | Pattern · fail 三元组 AC |

---

**文档源**：iPhone UX 规范 `ios/CatPhone/_bmad-output/planning-artifacts/ux-design-specification.md` §Party Mode Revisions v0.2 + §PRD 修订请求清单 + §跨仓契约变更清单

**联系**：通过 PR review 或跨仓 sync 会议交互
