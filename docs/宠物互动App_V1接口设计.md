# 宠物互动 App V1 接口设计

## 1. 文档说明

本文档定义当前版本 MVP 的 API 设计，包括：

- 统一协议约定
- 鉴权方案
- HTTP 接口
- WebSocket 消息协议
- 错误码
- 幂等与事务建议

**契约冻结策略**：

- 自 2026-04-26（Story 4.1 完成日，对应 git commit hash 见 commit message）起，§4.1（POST /auth/guest-login）/ §4.3（GET /me）/ §5.1（GET /home）三个节点 2 接口的 schema 进入**冻结**状态。
- 任何字段名 / 字段类型 / 错误码的修改都必须：
  1. 触发 iOS Epic 5 重新评审（影响 Story 5.2 / 5.5）
  2. 触发 server Epic 4 已完成 story 的回归（影响 Story 4.6 / 4.8 已落地的 handler）
  3. 在本 story 文件 + epics.md 同步标注变更原因 + 影响范围
- 节点 3+ 接口（如 §6 步数 / §7 宝箱 / §8 装扮 / §9 合成 / §10 房间 / §11 表情）的契约锚定由对应 epic 的 §X.1 story 负责（如 Story 7.1 / 11.1 / 17.1 等），不在本 story 范围内。
- Future Fields 标记的字段（`pet.equips` / `pet.equips[].renderConfig` / `room.currentRoomId` 等）**不**视为契约变更，由对应节点 epic 自然激活。注：`user.currentRoomId`（仅出现在 `GET /me`）**不**属于 Future Fields —— 该字段是**永久 schema 占位**（始终 `null`，无后续节点回填计划），见 §4.3 Future Fields 引用块。
- 自 2026-05-02（Story 7.1 完成日，对应 git commit hash 见 commit message）起，§6.1（POST /steps/sync）/ §6.2（GET /steps/account）两个节点 3 接口的 schema 进入**冻结**状态。
- 任何字段名 / 字段类型 / 错误码 / 防作弊阈值默认值的修改都必须：
  1. 触发 iOS Epic 8 重新评审（影响 Story 8.5 步数同步触发器）
  2. 触发 server Epic 7 已完成 story 的回归（影响 Story 7.3 / 7.4 已落地的 handler）
  3. 在本 story 文件 + epics.md 同步标注变更原因 + 影响范围
- `steps.single_sync_cap` / `steps.daily_cap` 两个配置 key 的**默认值**（5000 / 50000）属契约一部分：**prod 部署必须使用默认值**（5000 / 50000），不允许通过配置文件覆盖 —— 否则不同 prod 实例会在不同阈值上 truncate / 返 3001，重新引入本 story 想消除的客户端/服务端契约漂移；**dev / test 环境**可通过配置文件覆盖默认值（仅用于单测 / 调试 / fixture），**不**视为契约变更（这些环境不对外提供 prod 体验，跨端契约一致性不受影响）；**修改默认值本身**视为契约变更走完整冻结流程。
- 自 2026-05-05（Story 10.1 完成日，对应 git commit hash 见 commit message）起，§12.1（连接地址 + close code 表 + 服务端校验顺序）/ §12.2（客户端 → 服务端通用消息信封 + `ping`）/ §12.3（服务端 → 客户端通用消息信封 + `room.snapshot` + `pong` + `error`）三个节点 4 WS 协议骨架章节进入**冻结**状态。
- 任何字段名 / 字段类型 / close code / 信封字段（`type` / `requestId` / `payload` / `ts`）的修改都必须：
  1. 触发 iOS Epic 12 重新评审（影响 Story 12.2 ~ 12.6 所有 WS 集成 story）
  2. 触发 server Epic 10 已完成 story 的回归（影响 Story 10.3 / 10.4 / 10.7 已落地的 gateway / heartbeat / snapshot framework）
  3. 触发后续业务 Epic 11 / 14 / 17 的契约 story（11.1 / 14.1 / 17.1）回归（业务消息基于本骨架扩展）
  4. 在本 story 文件 + epics.md 同步标注变更原因 + 影响范围
- `ws.heartbeat_timeout_sec`（默认 60s）/ `ws.max_message_size_bytes`（默认 16 KB）两个配置 key 的**默认值**属契约一部分：**prod 部署必须使用默认值**，不允许通过配置文件覆盖 —— 否则不同 prod 实例会在不同心跳超时 / message 大小上限上踢人，重新引入跨端契约漂移；**dev / test 环境**可通过配置文件覆盖默认值（仅用于单测 / 调试 / fixture），**不**视为契约变更（这些环境不对外提供 prod 体验，跨端契约一致性不受影响）；**修改默认值本身**视为契约变更走完整冻结流程。
- WS 业务消息（`member.joined` / `member.left` / `pet.state.changed` / `emoji.send` / `emoji.received` 等）的字段层契约**不**在节点 4 协议骨架冻结范围内，由对应 Epic 的 §X.1 story（Story 11.1 / 14.1 / 17.1）独立锚定 + 独立冻结 —— 即"协议骨架在 Epic 10 冻结，业务消息按 epic 顺序逐步冻结"。
- **server / client active message set 的冻结范围**：本 story（Story 10.1）冻结的"server → client 仅 `room.snapshot` / `pong` / `error`"与"client → server 仅 `ping`"声明（见 §12.1.3 / §12.2 / §12.3 对应注解块）**仅适用于 Epic 10 阶段**（Story 10.1 ~ 10.7）；Epic 11 / 14 / 17 起，对应 §X.1 锚定 story（Story 11.1 / 14.1 / 17.1）按各自语义把新业务消息（如 `member.joined` / `member.left` / `pet.state.changed` / `emoji.received` / `emoji.send`）合法加入对应方向的 active message set，**不**视为本 story 冻结的违反。Epic 10 之后的 server / client active message set 由对应 epic §X.1 story 各自负责，本 story 不预先锁定。
- 自 2026-05-08（Story 11.1 完成日，对应 git commit hash 见 commit message）起，§10.1（POST /rooms）/ §10.2（GET /rooms/current）/ §10.3（GET /rooms/{roomId}）/ §10.4（POST /rooms/{roomId}/join）/ §10.5（POST /rooms/{roomId}/leave）五个节点 4 房间 REST 接口的 schema + §12.3 `### 成员加入`（`member.joined`）/ `### 成员离开`（`member.left`）两个节点 4 业务 WS 消息的字段表进入**冻结**状态。
- 任何字段名 / 字段类型 / 错误码（6001 ~ 6005）触发条件 / 业务 WS 消息 payload 字段的修改都必须：
  1. 触发 iOS Epic 12 重新评审（影响 Story 12.3 / 12.4 / 12.7 所有房间 client / WS 集成 story）
  2. 触发 server Epic 11 已完成 story 的回归（影响 Story 11.3 ~ 11.10 已落地的 handler / service / 广播路径）
  3. 触发后续业务 Epic 14 / 17 的契约 story（14.1 / 17.1）回归（业务消息基于本骨架扩展）
  4. 在本 story 文件 + epics.md 同步标注变更原因 + 影响范围
- `rooms.max_members` 默认值 **4** 属契约一部分：**prod 部署必须使用默认值**，不允许通过 schema migration / 配置覆盖 —— 否则不同 prod 实例会在不同容量上拒绝 join，重新引入跨端契约漂移；**dev / test 环境**可通过 fixture 覆盖默认值（仅用于单测 / 调试 / fixture），**不**视为契约变更；**修改默认值本身**视为契约变更走完整冻结流程。
- `rooms.status` 枚举（`1 = active` / `2 = closed`）属契约一部分；新增枚举值（如 `3 = paused`）视为契约变更。
- Future Fields 标记的字段（`members[].pet.equips` 节点 9 落地 / `members[].pet.equips[].renderConfig` 节点 10 落地 / `members[].pet.currentState` 节点 5 真实驱动；其中 `pet.currentState` 节点 5 真实驱动同时覆盖 §12.3 `room.snapshot.payload.members[].pet.currentState` + `member.joined.payload.pet.currentState`，三处由 Story 14.3 同一落地点同步切真实路径，避免 join 房间后 stale `1` 风险）**不**视为契约变更，由对应节点 epic 自然激活；具体过渡见 §10.3.5 五阶段过渡表。
- WS 业务消息（`pet.state.changed` / `emoji.send` / `emoji.received` 等）的字段层契约**不**在节点 4 房间业务冻结范围内，由对应 Epic 的 §X.1 story（Story 14.1 / 17.1）独立锚定 + 独立冻结。
- 自 2026-05-12（Story 14.1 完成日，对应 git commit hash 见 commit message）起，§5.2（POST /pets/current/state-sync）节点 5 REST 接口的 schema + §12.3 `### 宠物状态变更（pet.state.changed）` 节点 5 业务 WS 消息的字段表进入**冻结**状态。
- 任何字段名 / 字段类型 / `state` 枚举值（1 / 2 / 3）/ 错误码（1001 / 1002 / 1005 / 1009）触发条件 / **pet-less 账号走 server-acknowledged noop 路径（200 OK + code = 0，与 §5.1 / §10.3 / §12.3 pet-less 合法 edge case 语义一致；本接口**不**触发 1003）** / `pet.state.changed` payload 字段（`userId` / `petId` / `currentState`）+ 顶层 envelope 字段（`type` / `requestId` / `ts`，遵循 §12.3 通用信封）/ 广播范围（包含发起者自己 vs 排除）/ fire-and-forget 语义 / **Story 14.3 落地点同时覆盖 §10.3 + §12.3 `room.snapshot` + §12.3 `member.joined` 三处 `pet.currentState` 字段切真实路径**（避免 join 房间后 stale `1` 风险）的修改都必须（**冻结边界说明**：1005 的"触发条件"冻结在**抽象层**——"走通用 rate_limit 中间件按 `user_id` 维度限频拦截"——这一不变量；**具体阈值**（如 60/min）由 Story 4.5 默认值 + 配置层管理，调整阈值**不**视为本接口契约变更，**不**触发下文 1-4 步流程；删除限频中间件 / 切换限频维度 / 把 1005 改成抛其他错误码才视为契约变更）：
  1. 触发 iOS Epic 15 重新评审（影响 Story 15.1 / 15.2 / 15.4 / 15.5 房间内宠物状态展示 + 上报 + 跨房间恢复 story）
  2. 触发 server Epic 14 已完成 story 的回归（影响 Story 14.2 / 14.3 / 14.4 已落地的 handler / RoomSnapshotBuilder / 广播路径）
  3. 触发后续业务 Epic 17 的契约 story（17.1）回归（如新业务消息基于本骨架扩展）
  4. 在本 story 文件 + epics.md 同步标注变更原因 + 影响范围
- `pets.current_state` 枚举（`1 = rest` / `2 = walk` / `3 = run`，来源数据库设计 §6.4）属契约一部分；新增枚举值（如 `4 = sleep`）视为契约变更。
- `pet.state.changed` 广播范围（**包含**发起者自己）属契约一部分；与 `member.joined` / `member.left` 排除发起者 / 离开者**不同**语义，是显式设计选择（state 是事件本身，让 client 用统一路径处理；joined/left 是关系变化，发起者已通过 HTTP response 知道结果）；如未来需要切换为排除发起者，视为契约变更。
- `pet.state.changed` fire-and-forget 语义（广播失败仅 log，不回滚 DB UPDATE，不影响 HTTP 200 响应）属契约一部分；如未来需要切换为强一致（如广播失败回滚），视为契约变更。
- 自 2026-05-13（Story 17.1 完成日，对应 git commit hash 见 commit message）起，§11.1（GET /api/v1/emojis）节点 6 REST 接口的 schema + §12.2 `### 发送表情`（`emoji.send`）+ §12.3 `### 收到表情广播`（`emoji.received`）两个节点 6 业务 WS 消息的字段表进入**冻结**状态。
- 任何字段名 / 字段类型 / `emojiCode` 字符集约束（`[a-z0-9_-]` + length 1-64） / 错误码（1001 / 1002 / 1009 / 6004 / 7001；**注**：`emoji.send` 走 WS 路由 → 不经 HTTP rate_limit 中间件 → **不**暴露 1005 路径，故 1005 **不**在本接口冻结错误码列表内；与 §12.2 "不限频"段 + §12.2 错误响应表对齐）触发条件 / `emoji.received` payload 字段（`userId` / `emojiCode`）+ 顶层 envelope 字段（`type` / `requestId` / `ts`，遵循 §12.3 通用信封）/ 广播范围（**包含**发起者自己，与 `pet.state.changed` 同语义）/ fire-and-forget 语义 / **表情不持久化语义**（与 `pet.state.changed` 持久化 `pets.current_state` 不同）/ **client 端对 self-broadcast 的去重规则**（跳过 `payload.userId == 当前 user.id` 的 echo，与 `pet.state.changed` self-broadcast 双轨兜底**不同**语义）/ GET /emojis 不分页 + 不接受 query 参数 + 不返回 `is_enabled=0` 表情等核心契约的修改都必须（**冻结边界说明**：7001 / 6004 的"触发条件"冻结在**抽象层** —— "走 emoji_configs `is_enabled=1` 查询 / 走 `users.current_room_id == Session.roomID` 比对（**权威源 = `Session.roomID`，不是 `users.current_room_id`**；与 §12.2 服务端逻辑步骤 3 + r1 review 锁定的反 stale-Session 校验对齐）"这一不变量；**具体 DB 查询路径调优**（如加索引 / 改 cache）**不**视为契约变更；**注**：本接口**不**冻结 1005 路径，故无 1005 抽象层条款 —— 如未来 WS 路由新增按 user_id 限频中间件 + 暴露 1005 错误码，视为契约变更）：
  1. 触发 iOS Epic 18 重新评审（影响 Story 18.1 / 18.2 / 18.3 / 18.4 所有表情面板 + 发送 + 接收动效 story）
  2. 触发 server Epic 17 已完成 story 的回归（影响 Story 17.2 / 17.3 / 17.4 / 17.5 已落地的 migration / seed / handler / dispatcher / 广播路径）
  3. 在本 story 文件 + epics.md 同步标注变更原因 + 影响范围
- `emoji_configs.code` 字符集约束（`[a-z0-9_-]` + length 1-64，与 §5.15 数据库 `VARCHAR(64)` 字段一致）属契约一部分；新增字符集字符（如允许 `.` / `:` 等点分形式）视为契约变更；`emoji_configs.is_enabled` 仅 0 / 1 两值（与 §6 状态枚举一致）属契约一部分。
- `emoji.received` 广播范围（**包含**发起者自己）属契约一部分；与 `member.joined` / `member.left` 排除发起者 / 离开者**不同**语义、与 `pet.state.changed` **同**语义，是显式设计选择（表情是事件本身，让 client 用统一 WS 入口处理；server 不区分接收者）；如未来需要切换为排除发起者，视为契约变更。
- `emoji.received` fire-and-forget 语义（广播失败仅 log，不回 error 给发起者）属契约一部分；如未来需要切换为强一致（如广播失败回错给发起者），视为契约变更。
- `emoji.received` self-broadcast 去重契约（client 端跳过 `payload.userId == 当前 user.id` 的 echo，与 `pet.state.changed` self-broadcast 双轨兜底**不同**）属契约一部分；如未来 client 改为"接收 self-echo 并触发某 UI 反馈"（如 ack indicator），视为契约变更。
- **表情不持久化**契约（按数据库设计 §14.3，server 不写入任何表）属契约一部分；如未来引入 `emoji_events` 表 + 历史回放接口，视为契约变更。
- **GET /emojis 不分页 + 不接受 query 参数 + 不返回 disabled 表情**契约属契约一部分；如未来引入分页 / 筛选 / disabled 表情对 admin 可见，视为契约变更。
- 自 2026-05-14（Story 20.1 完成日，对应 git commit hash 见 commit message）起，§7.1（GET /api/v1/chest/current）/ §7.2（POST /api/v1/chest/open）两个节点 7 宝箱 REST 接口的 schema 进入**冻结**状态。
- 任何字段名 / 字段类型 / `status` 枚举值（1 / 2）/ 错误码（1001 / 1002 / 1005 / 1008 / 1009 / 3002 / 4001 / 4002）触发条件 / `idempotencyKey` 字符集（`[A-Za-z0-9_:-]` + length 1-128）/ **DB 幂等原子声明语义（`INSERT ... ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)` 借 `chest_open_idempotency_records.UNIQUE(user_id, idempotency_key)` 在业务事务**内**首条语句单语句原子声明；同 key 并发请求通过 InnoDB unique-key X-lock 阻塞排队，首个事务结束后其他事务再分支：`affected_rows = 1`（行不存在，含首次到达 + 首次 rollback 后到达）→ 进业务步骤 / `affected_rows = 0` + `status = 'success'` → 200 + 首次缓存）** / **DB 同事务幂等持久化语义（预声明 + 业务写入 + 步骤 5k `UPDATE chest_open_idempotency_records SET status='success', response_json=?` 在同一事务原子 commit；rollback 时 pending 行也跟着回滚，无残留；**无任何 best-effort post-rollback 异步补偿**；移除 Redis 在 chest_open idempotency 路径上的所有角色）** / **幂等状态机为二态 `('pending', 'success')`**（r7 review 锁定，移除 `failed` 状态以消除 best-effort failed upsert race）/ **`response_json` 缓存内容范围（**不**包含 `nextChest.remainingSeconds` 等时间派生字段 / **不**包含顶层 `requestId` trace 字段；这两类字段由 server 在响应序列化时实时填入 —— `remainingSeconds` 按 `max(0, ceil((unlock_at - now) / 1s))` 计算；`requestId` 填**当前**请求的 trace ID；首次成功路径与重试 cached 路径走同一序列化逻辑）** / **rate_limit 位置语义（r10 review 锁定）**：rate_limit 检查在 handler 内层、**幂等命中预检之后**做 —— cached replay 路径（同 key 重试命中 `status = 'success'` / `status = 'pending'`）**不**计入 rate_limit 配额；仅未命中 idempotency 的"真新请求"按 user_id 60/min 限频。该结构是 §7.2 对 §4.5 钦定的"统一 rate_limit middleware"基线的契约级例外/ MySQL 事务边界 / 加权抽取语义 / **节点 7 阶段 `reward.userCosmeticItemId` 占位值 `"0"` 契约** / `nextChest` 永远非 null 且 server 端固定 status=1 / unlock_at=now+10min 的修改都必须（**冻结边界说明**：`reward.userCosmeticItemId` 字段值**从节点 8 Story 23.5 起切换为真实主键**这一升级**不**视为契约变更 —— 字段类型 / 名 / 必填性都不变，仅服务端语义升级；Story 23.5 落地时应在 §7.2.6 标注升级日期 + commit hash；1008 / 1005 / 4003 等"触发条件"冻结在**抽象层** —— "走 §3 错误码表对应映射"这一不变量；**幂等存储介质**冻结在**抽象层** —— "幂等记录 + 业务数据原子写"这一不变量（r5 锁定 MySQL 同事务方案；r6 进一步把预声明纳入同一事务消除 pending 卡死悖论；r7 简化为二态机消除 failed upsert race；移除 Redis）；删除 idempotencyKey + DB 幂等机制 / 把原子声明退化为非原子两步 / 把预声明退回到业务事务外 / 把 `remainingSeconds` / `requestId` 等动态字段写回 `response_json` 缓存 / 重新引入异步 `failed` 补偿写 / 切换为非加权抽奖 / 修改 nextChest 创建时机 / 把幂等记录退回 Redis 才视为契约变更）：
  1. 触发 iOS Epic 21 重新评审（影响 Story 21.1 ~ 21.5 所有首页宝箱组件 + 倒计时 + 开箱 + 奖励弹窗 + 主动同步步数 story）
  2. 触发 server Epic 20 已完成 story 的回归（影响 Story 20.2 ~ 20.9 已落地的 migration / seed / handler / service / 集成测试）
  3. 触发下游 Epic 23 的契约 story（23.1）回归（节点 8 入仓事务基于本骨架扩展）
  4. 在本 story 文件 + epics.md 同步标注变更原因 + 影响范围
- `user_chests.open_cost_steps` 默认值 **1000** 属契约一部分：**prod 部署必须使用默认值**，不允许通过配置 / migration 覆盖；**dev / test 环境**可通过 fixture 覆盖（仅用于单测 / 调试 / fixture），**不**视为契约变更；**修改默认值本身**视为契约变更走完整冻结流程。
- `user_chests.status` 枚举（`1 = counting` / `2 = unlockable`）属契约一部分；新增枚举值（如 `3 = opened`）视为契约变更（**注**：当前设计中 opened 状态不存在 —— 开箱后立即创建下一轮 chest，opened 是 transient state，仅在事务内瞬间存在）。
- `cosmetic_items` 表 seed 数量约束（epics.md AR18 钦定 common ≥ 8 / rare ≥ 4 / epic ≥ 2 / legendary ≥ 1）属契约一部分；admin 后台增删 cosmetic_items 行**不**视为契约变更，但删到不足 AR18 约束视为线上事故。
- `drop_weight` 加权抽奖算法（按 `cumulative_weight / total_weight` 比例抽取，仅 `is_enabled = 1` 行参与）属契约一部分；切换为非加权 / 加权但不按 drop_weight 排序 视为契约变更。

---

## 2. 统一约定

## 2.1 协议

- 普通业务接口：`HTTPS + JSON`
- 实时互动：`WebSocket`

## 2.2 接口前缀

```text
/api/v1
```

## 2.3 鉴权方式

除登录接口外，统一使用：

```http
Authorization: Bearer <token>
```

## 2.4 通用响应结构

```json
{
  "code": 0,
  "message": "ok",
  "data": {},
  "requestId": "req_123456"
}
```

字段说明：

- `code`：业务状态码，`0` 表示成功
- `message`：错误提示或状态说明
- `data`：业务数据
- `requestId`：链路追踪 id

## 2.5 字段类型与编码约定

- **主键 / 外键 ID**：所有 BIGINT 主键 / 外键在 JSON 里以**字符串**形式下发（如 `"id": "1001"`），避免 JavaScript 端 `Number.MAX_SAFE_INTEGER` 精度丢失；客户端解析为 `String`，如需算术运算自行转换为 `Int64` 或等价类型。
- **时间字段**：**response 中的 datetime 类型字段**统一使用 ISO 8601 UTC 字符串（如 `"unlockAt": "2026-04-23T10:20:00Z"`），客户端按本地时区显示；**request 中的 epoch 时间戳类字段**（如 `POST /steps/sync` 的 `clientTimestamp`）保留 number 类型（毫秒整数），不做 ISO 字符串化 —— 该类字段在各接口章节单独标注。
- **可空字段**：可空字段在 JSON 中显式以 `null` 表示（如 `"currentRoomId": null`），不省略 key；客户端解析为 `Optional<T>` / `T?`。
- **占位字段（节点 2 阶段）**：节点 2 阶段尚未实装的字段（如 `pet.equips` / `room.currentRoomId` / `chest.unlockable` 等动态状态）按 "future fields" 标注，文档中会在每个接口章节末尾以 `> Future Fields (节点 X 落地)` 引用块列出。
- **字符串长度约束**：所有字符串字段的最大长度以**字符数**计（与 MySQL 表 `VARCHAR(N)` 语义一致 —— `N` 是字符数 limit，**不是**字节数；utf8mb4 编码下 1 个字符在底层存储占 1-4 字节，但长度校验按字符计数）。客户端 / server 实施长度校验时**必须**按 Unicode 字符数（如 Go `utf8.RuneCountInString` / Swift `String.count`）判断，**不**按字节数判断，否则会误拒合法的多字节输入。
- **枚举字段**：枚举字段以 `TINYINT` 数值下发（如 `petType: 1`），不下发字符串字面量；具体取值见各接口章节。

---

## 3. 错误码定义

```text
0       成功
1001    未登录 / token 无效
1002    参数错误
1003    资源不存在
1004    权限不足
1005    操作过于频繁
1006    状态不允许当前操作
1007    数据冲突
1008    幂等冲突
1009    服务繁忙

2001    游客账号不存在
2002    微信已绑定其他账号
2003    当前账号已绑定微信

3001    步数同步数据异常
3002    可用步数不足

4001    当前宝箱不存在
4002    宝箱尚未解锁
4003    宝箱开启条件不满足

5001    道具不存在
5002    道具不属于当前用户
5003    道具状态不可用
5004    装备槽位不匹配
5005    合成材料数量错误
5006    合成材料品质不一致
5007    合成目标品质不合法
5008    装扮已装备

6001    房间不存在
6002    房间已满
6003    用户已在房间中
6004    用户不在房间中
6005    房间状态异常

7001    表情不存在
7002    WebSocket 未连接
```

---

## 4. 认证与账号接口

## 4.1 游客登录

### `POST /api/v1/auth/guest-login`

用于：

- 首次创建游客账号
- 已存在游客账号自动登录

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | POST |
| Path | /api/v1/auth/guest-login |
| 认证 | **不需要**（登录接口本身免 auth 中间件，但走 rate_limit 中间件） |
| 限频 | 默认每客户端 IP 每分钟 60 次（按 Story 4.5 rate_limit 中间件全局默认值） |
| 幂等 | 幂等（同一 guestUid 重复调用 → 同一 user_id；不需要 idempotencyKey） |
| 节点 | 节点 2（Epic 4 落地，Epic 5 客户端集成） |

#### 请求体

| 字段 | 类型 | 必填 | 长度约束 | 说明 |
|---|---|---|---|---|
| `guestUid` | string | 必填 | 1 ≤ length ≤ 128 字符 | 客户端 Keychain 持久化的游客身份 UID（推荐 UUID v4 字符串）。空字符串或 > 128 字符 → 1002 参数错误 |
| `device` | object | 必填 | - | 设备信息对象，子字段见下 |
| `device.platform` | string | 必填 | enum: `"ios"` / `"android"`（节点 2 仅 `"ios"`） | 客户端平台标识 |
| `device.appVersion` | string | 必填 | 1 ≤ length ≤ 32 字符 | 客户端版本号（如 `"1.0.0"`） |
| `device.deviceModel` | string | 必填 | 1 ≤ length ≤ 64 字符 | 设备型号（如 `"iPhone15,2"`） |

JSON 示例：

```json
{
  "guestUid": "ios_keychain_unique_id",
  "device": {
    "platform": "ios",
    "appVersion": "1.0.0",
    "deviceModel": "iPhone15,2"
  }
}
```

#### 服务端行为

- 根据 `guestUid` 查找已有绑定
- 若存在则直接登录
- 若不存在则初始化用户、默认猫咪、步数账户、当前宝箱
- 详细事务流程见 Story 4.6 + 数据库设计 §8.1

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `data.token` | string | JWT token，HS256 + auth.token_secret 签名（见 Story 4.4 token util）；默认过期 7 天 |
| `data.user.id` | string | 用户主键（BIGINT 序列化为字符串，见 §2.5） |
| `data.user.nickname` | string | 自动生成的昵称 `"用户{id}"`（首次创建时由 server 写入） |
| `data.user.avatarUrl` | string | 头像 URL；首次创建为 `""`（空字符串而非 null） |
| `data.user.hasBoundWechat` | boolean | 是否已绑定微信；游客首次创建为 `false` |
| `data.pet.id` | string | 默认猫主键（BIGINT 序列化为字符串） |
| `data.pet.petType` | number | 宠物类型枚举，节点 2 固定 `1`（猫） |
| `data.pet.name` | string | 宠物名，首次创建为 `"默认小猫"` |

JSON 示例：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "token": "xxx",
    "user": {
      "id": "1001",
      "nickname": "用户1001",
      "avatarUrl": "",
      "hasBoundWechat": false
    },
    "pet": {
      "id": "2001",
      "petType": 1,
      "name": "默认小猫"
    }
  },
  "requestId": "req_xxx"
}
```

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1002 | 参数错误 | `guestUid` 缺失 / 为空 / 长度超过 128 字符；`device` 字段缺失或子字段不全；`device.platform` 不在枚举中 |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截（同 IP 每分钟 > 60 次） |
| 1009 | 服务繁忙 | 数据库异常 / 事务回滚 / 内部 panic（见 Story 4.6 事务实装） |

---

## 4.2 绑定微信

### `POST /api/v1/auth/bind-wechat`

#### 请求体

```json
{
  "wechatCode": "wx_auth_code"
}
```

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "hasBoundWechat": true
  },
  "requestId": "req_xxx"
}
```

---

## 4.3 获取当前用户信息

### `GET /api/v1/me`

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | GET |
| Path | /api/v1/me |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | 默认（按 Story 4.5 rate_limit 默认值） |
| 节点 | 节点 2（Epic 4 落地，且 `currentRoomId` **始终**返回 `null` —— 该字段保留为 schema 占位但不计划在后续节点回填真实数据；获取真实房间归属请改用 `GET /home`，详见本节 Future Fields） |

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `data.user.id` | string | 用户主键 |
| `data.user.nickname` | string | 用户昵称 |
| `data.user.avatarUrl` | string | 头像 URL，首次创建为 `""` |
| `data.user.hasBoundWechat` | boolean | 是否已绑定微信 |
| `data.user.currentRoomId` | string \| null | 当前房间主键。**始终返回 `null`** —— schema 占位字段，无后续节点回填计划（详见本节 Future Fields）；客户端必须按 `Optional<String>` 解析 |

JSON 示例（服务端始终返回 `null`；获取真实房间归属请改用 `GET /home`，详见 Future Fields）：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "user": {
      "id": "1001",
      "nickname": "用户1001",
      "avatarUrl": "",
      "hasBoundWechat": true,
      "currentRoomId": null
    }
  },
  "requestId": "req_xxx"
}
```

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截 |
| 1009 | 服务繁忙 | DB 查询失败 |

#### Future Fields

> **关于 `data.user.currentRoomId`**：本字段**保留为 schema 占位**但**不计划在后续节点回填真实数据**（planning artifacts 中无对应 story）。服务端**始终**返回 `null`，客户端**必须**按可空解析；获取真实房间归属请改用 `GET /home` 的 `data.room.currentRoomId`（节点 4 由 Story 11.10 回填真实数据）。

---

## 5. 首页与宠物接口

## 5.1 获取首页聚合数据

### `GET /api/v1/home`

用于首页一次拉取主要展示内容。

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | GET |
| Path | /api/v1/home |
| 认证 | **需要** Bearer token |
| 限频 | 默认 |
| 节点 | 节点 2 initial 版（含 user + pet + stepAccount + chest + room 占位）；后续节点 increment 填充 pet.equips / room.currentRoomId / pet.equips[].renderConfig |

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 节点 2 阶段值 | 说明 |
|---|---|---|---|
| `data.user.id` | string | 真实数据 | 用户主键 |
| `data.user.nickname` | string | 真实数据 | 用户昵称 |
| `data.user.avatarUrl` | string | `""` | 头像 URL |
| `data.pet` | object \| null | 真实数据 | 默认猫信息容器；用户**无默认 pet（理论不该发生，但 Story 4.8 edge case 强制覆盖）时返回 `null`**。客户端必须按可空对象解析（iOS 端 `Optional<PetDTO>` / Go 端 `*PetDTO`），不得假设 pet 永远非空。下方 `data.pet.*` 子字段**仅当 `data.pet ≠ null` 时存在** |
| `data.pet.id` | string | 真实数据 | 默认猫主键（仅当 `data.pet ≠ null`） |
| `data.pet.petType` | number | `1` | 宠物类型枚举（仅当 `data.pet ≠ null`） |
| `data.pet.name` | string | `"默认小猫"` | 宠物名（仅当 `data.pet ≠ null`） |
| `data.pet.currentState` | number | `1`（rest） | 宠物当前状态枚举（1=rest, 2=walk, 3=run）；节点 2 阶段读 pets.current_state，初始为 `1`（仅当 `data.pet ≠ null`） |
| `data.pet.equips` | array | `[]` | 装扮列表；**节点 2 阶段强制返回 `[]`**（穿戴在节点 9 由 Story 26.6 落地）（仅当 `data.pet ≠ null`） |
| `data.stepAccount.totalSteps` | number | `0`（首次登录） | 累计步数 |
| `data.stepAccount.availableSteps` | number | `0`（首次登录） | 可用步数 |
| `data.stepAccount.consumedSteps` | number | `0`（首次登录） | 已消耗步数 |
| `data.chest.id` | string | 真实数据 | 当前宝箱主键 |
| `data.chest.status` | number | `1`（counting） | 宝箱状态枚举（1=counting, 2=unlockable —— 见 §7.1 status 枚举 + 数据库设计 §6.7 user_chests.status）；节点 2 阶段所有宝箱在登录初始化后均为 `1`（counting），到 `unlockAt` 后服务端**动态判定**为 `2`（unlockable）—— 见 Story 4.8 happy path 第 2 case。**节点 2 不存在 `opened` 状态**：开箱功能在节点 7 / Epic 20 才上线，届时开箱后会立即重建下一轮 chest（仍为 `counting`），故 `/home` 永远不返回 opened 状态 |
| `data.chest.unlockAt` | string (ISO 8601 UTC) | now + 10 min | 解锁时间 |
| `data.chest.openCostSteps` | number | `1000` | 开启所需步数（节点 2 固定 1000） |
| `data.chest.remainingSeconds` | number | 600 ~ 0 | 距离 unlockAt 的剩余秒数；> 0 表示 counting，≤ 0 表示已可开启 |
| `data.room.currentRoomId` | string \| null | `null` | 当前房间主键；**节点 2 阶段强制 `null`**（节点 4 由 Story 11.10 注入真实数据） |

JSON 示例（节点 2 阶段示例 — 首次登录后立即调用）：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "user": {
      "id": "1001",
      "nickname": "用户1001",
      "avatarUrl": ""
    },
    "pet": {
      "id": "2001",
      "petType": 1,
      "name": "默认小猫",
      "currentState": 1,
      "equips": []
    },
    "stepAccount": {
      "totalSteps": 0,
      "availableSteps": 0,
      "consumedSteps": 0
    },
    "chest": {
      "id": "5001",
      "status": 1,
      "unlockAt": "2026-04-23T10:20:00Z",
      "openCostSteps": 1000,
      "remainingSeconds": 600
    },
    "room": {
      "currentRoomId": null
    }
  },
  "requestId": "req_xxx"
}
```

JSON 示例（节点 9 之后的真实数据，pet.equips 由 Story 26.6 填充；节点 10 起 pet.equips[] 还会带 renderConfig 子对象，本示例**不**展示 —— 待 Story 29.6 落地时单独补充节点 10 版本示例）：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "user": {
      "id": "1001",
      "nickname": "用户1001",
      "avatarUrl": ""
    },
    "pet": {
      "id": "2001",
      "petType": 1,
      "name": "默认小猫",
      "currentState": 2,
      "equips": [
        {
          "slot": 1,
          "userCosmeticItemId": "90001",
          "cosmeticItemId": "12",
          "name": "小黄帽",
          "rarity": 1,
          "assetUrl": "https://..."
        }
      ]
    },
    "stepAccount": {
      "totalSteps": 12560,
      "availableSteps": 840,
      "consumedSteps": 300
    },
    "chest": {
      "id": "5001",
      "status": 1,
      "unlockAt": "2026-04-23T10:20:00Z",
      "openCostSteps": 1000,
      "remainingSeconds": 253
    },
    "room": {
      "currentRoomId": "3001"
    }
  },
  "requestId": "req_xxx"
}
```

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截 |
| 1009 | 服务繁忙 | 任一聚合查询失败（按 epics.md §Story 4.8 AC：各部分 repo 错误 → 整体 1009 服务繁忙，不部分降级） |

#### Future Fields

> **Future Fields**（按节点 increment）：
> - `data.pet.equips[]`：节点 9（Epic 26）穿戴链路落地后，由 Story 26.6 把 user_pet_equips JOIN cosmetic_items 的真实数据填充。每个元素 schema 见上方"节点 9 之后的真实数据"示例，含 `slot / userCosmeticItemId / cosmeticItemId / name / rarity / assetUrl`。
> - `data.pet.equips[].renderConfig`：节点 10（Epic 29）渲染配置落地后，由 Story 29.6 在每个 equips 元素上追加 `renderConfig: { offsetX, offsetY, scale, rotation, zLayer }` 子对象。具体字段在 Epic 29 落地时再展开。
> - `data.room.currentRoomId`：节点 4 起由 Story 11.10 注入真实房间 ID。

---

## 5.2 同步宠物当前展示状态

### `POST /api/v1/pets/current/state-sync`

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | POST |
| Path | /api/v1/pets/current/state-sync |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | 默认（按 Story 4.5 rate_limit 默认值 60 次/分；状态同步是低频操作 —— iOS 端只在状态切换瞬间上报，业务上每用户每分钟 << 60 次） |
| 幂等 | **幂等**（同 user 同 state 重复上报：service 层 UPDATE `pets.current_state = ?` 即便值未变也合法 —— DB `err == nil` 即视为成功，**不**读 `RowsAffected`，因为 MySQL/GORM 语义下"把字段更新为原值"常见 `RowsAffected == 0` 但不代表失败；接口仍返回 200 OK + code = 0，不返回任何"未变"特殊状态码 —— client 不需要感知"已是该值"。详见 §5.2 服务端逻辑步骤 4）；**不**接受 `idempotencyKey` 头（与 chest.open / compose.upgrade 不同 —— state-sync 不消耗资产，重复执行无副作用） |
| 节点 | 节点 5（Epic 14 落地，Epic 15 客户端集成） |

#### 请求体

| 字段 | 类型 | 必填 | 范围/约束 | 说明 |
|---|---|---|---|---|
| `state` | number (int) | 必填 | enum: `1` / `2` / `3` | 客户端识别到的当前宠物状态（来源 iOS 端动作识别状态机 / HealthKit / CoreMotion 综合判断，节点 3 Epic 8 已实装）。`1 = rest`, `2 = walk`, `3 = run`（与数据库设计 §6.4 `pets.current_state` 同义）。任何不在 {1,2,3} 范围内的值 → 1002 参数错误 |

JSON 示例：

```json
{
  "state": 2
}
```

> **注**：本接口**不**接受 `petId` 字段 —— 节点 5 阶段每个 user 只有 1 个默认 pet（数据库设计 §5.3 `uk_user_default_pet (user_id, is_default)` 唯一约束保证；Story 4.6 游客登录初始化事务创建），service 层从 `pets WHERE user_id=? AND is_default=1` 查到唯一 pet 后 UPDATE；后续 epic 若引入"多宠物切换"诉求，由对应 epic 的契约 story 决定是否扩展请求体加 `petId` 字段（**不**在节点 5 范围）。

#### state 枚举

- `1 = rest`
- `2 = walk`
- `3 = run`

#### 服务端逻辑

1. **认证 & 限频**：auth 中间件校验 Bearer token；rate_limit 中间件按默认配置限频（60 次/分按 user_id 计；已认证业务路由统一按 user_id 而非 IP，与 §6.1 / §6.2 / §10.x 同语义）
2. **参数校验**：`state` 必填 + 类型为 number (int) + 值 ∈ {1, 2, 3}；任何不通过 → 返回 1002 参数错误（不开 DB 事务）
3. **查找当前用户的默认 pet**：`SELECT id FROM pets WHERE user_id = ? AND is_default = 1`（数据库设计 §5.3 唯一约束保证最多 1 行）
   - **0 行**（pet-less 账号 —— 与 §5.1 GET /home `data.pet = null` / §10.3 `data.members[].pet = null` / §12.3 `member.joined.payload.pet = null` 是**同一类合法 edge case**，contract 内显式覆盖；非 invariant 损坏）→ 走 **server-acknowledged noop 路径**：跳过步骤 4（无 pet 行可 UPDATE）+ 跳过步骤 5（无 pet 实体可广播 `pet.state.changed`）+ 直接进入步骤 6 返回 **200 OK + code = 0 + `data.state` = 入参 state 值**（回显）。service 层 **不** log error（pet-less 是 contract-valid 状态，不是 invariant 损坏；如需可观测性可 log info 级"pet-less state-sync noop"，但**不**触发任何业务错误码）
   - **1 行** → 进入步骤 4
4. **UPDATE**：`UPDATE pets SET current_state = ?, updated_at = NOW() WHERE id = ?`（按上一步查到的 pet.id）
   - **`err == nil`**（DB 调用无错误）→ **一律**返回 200 OK + code = 0，进入步骤 5；service 层**不**读 `RowsAffected`、**不**根据该值分支业务逻辑
   - **`err != nil`**（driver / 网络 / 约束冲突 / 任何 DB 异常）→ 返回 1009 服务繁忙
   - 理由：(a) `RowsAffected == 1` 是常规情形（state 实际改变 + `updated_at` 写新值）；(b) `RowsAffected == 0` 在 MySQL/GORM 语义下**主要触发条件是"WHERE 命中 1 行但所有列值与入参完全一致"** —— 即"同 user 同 state 重复上报"幂等场景，业务上必须返回 200 OK + code = 0（与本节"幂等"声明 + line 500 元信息表对齐）。**注**：本接口的 UPDATE 把 `updated_at = NOW()` 也写入，理论上即便 `current_state` 未变 `updated_at` 也会变 → MySQL 通常仍报 `RowsAffected == 1`；但 GORM/driver 在某些 client/server time-zone 边界 + 某些 MySQL 配置（如 `innodb_flush_log_at_trx_commit` 异常）下可能仍返回 0，service 层**不**依赖该值判断成功失败。
   - **关于"步骤 3 到步骤 4 之间 pet 行消失"的极端 race**：节点 5 阶段无业务路径触发 DELETE pets（无该接口、无该 service 路径），该场景在实装层视为**理论不可达**；即便发生（DB 层 invariant 损坏），实装侧仍按本规则走 `err == nil` ⇒ 200 OK + code = 0 路径 —— **不**降级为 1009、**不**降级为 1003、**不**读 `RowsAffected`。client 实装侧的影响仅为"单次 self UI 状态未真正落库" 的本地短窗口偏差，下一次 state-sync 调用会重新写入，**不**导致跨用户状态泄露或数据丢失。**契约层不为该 0 概率分支预留任何错误码出口**，避免 service 层为兜底而引入 `RowsAffected` 判定（与下方"实装锁定"两个互斥二分闭环一致）
   - **以上规则的实装锁定**：1009 ⇔ `err != nil`；200 OK + code = 0 ⇔ `err == nil`；这是**两个互斥的二分**，service 层**不**存在第三条路径（不再读 RowsAffected、不区分"row 消失"vs"幂等同值"）
5. **广播（fire-and-forget，仅当用户在房间时）**：service 层检查 `users.current_room_id`：
   - **非 null** → 调用 `BroadcastToRoom(currentRoomId, {type: "pet.state.changed", payload: {userId, petId, currentState}})`（详见 §12.3 `### 宠物状态变更（pet.state.changed）`）；广播失败仅 log warning，**不**回滚 UPDATE，**不**影响 HTTP 响应（与 Story 11.8 `member.joined` 广播失败语义一致）
   - **null** → 用户当前不在任何房间，**不**广播（无房间内成员需要被通知）
6. **响应**：返回 `data.state` = 入参 state 值（回显，client 用作 server-acknowledged 确认信号）；详见下方"响应体"小节

**事务边界规则**：本接口**不**需要 MySQL 事务（仅 1 个 UPDATE 单语句，DB 引擎默认 autocommit；广播是 fire-and-forget 不在事务内）—— 与 §10.1 / §10.4 / §10.5 房间事务接口的多语句事务边界**不同**（参见数据库设计 §8.x 关键事务设计，本接口**不**入 §8.x 事务列表）。

**WS 广播 vs HTTP 响应的关系（含发起者自己的 self-broadcast 兜底规则）**：HTTP 200 是 server-acknowledged 入账确认信号；`pet.state.changed` WS 广播是事件通知信号。两者通过两条独立连接送达 client（HTTP 经 ApiClient；WS 经 WebSocketClient）；client 同房间内**会同时**收到自己的 HTTP 200 + 自己的 `pet.state.changed`（广播范围包含发起者自己，详见 §12.3 `### 宠物状态变更` 关键约束）。

**对"别人的状态变化"的权威信号**：以 WS 广播 `pet.state.changed` 为**唯一权威信号**（与 CLAUDE.md §"工作纪律 / 状态以 server 为准"一致）；client **不**通过任何其他渠道驱动别人的 roster pet state 更新。

**对"发起者自己的状态变化"的权威信号（self-broadcast 例外，基于到达顺序的对称无操作）**：考虑到 self-broadcast 在 §12.3 line 2230 是 fire-and-forget 不重试、且若该唯一信号丢失会让发起者 UI 永久 stale（HTTP 200 + 本地 self-broadcast 丢失 → 永远等不到下一次状态切换 / WS 重连），契约层**允许**发起者在收到 HTTP 200 OK **或** self-broadcast WS 消息（取**先到的任一信号**）后**立即更新自己**的本地 roster pet state，**不等**另一路信号到达。该规则按到达顺序对称展开：

- **(a) HTTP 200 先到（典型场景）**：client 立即用 `response.data.state`（= request `state`，参见上文"设计权衡"）更新本地 self entry 的 roster pet state；后续 self-broadcast 到达时按 §12.3 client merge contract 字段级 merge → 值已相同 → **no-op**（merge 结果幂等，无状态闪烁）
- **(b) WS self-broadcast 先到（罕见但合法）**：client 立即用 `payload.currentState` 更新本地 self entry（**与"别人的广播"统一走 client merge contract 字段级 merge 路径**，不为 self 走特殊分支）；后续 HTTP 200 到达时 client 比对 `response.data.state` 与本地已 merge 值 → 值已相同 → **no-op**（HTTP ack 作 server 端入账成功的二次确认信号，**不**触发再次写入）
- **(c) 对称无操作不变量**：**任一路径先到的信号都立即驱动 UI 更新**，后到的信号是 no-op —— client 实装层**不**假设固定的"HTTP 先到 / WS 先到"顺序；两路信号在 server 端来源同一次 `UPDATE pets ...` 写库行为（service 步骤 4 UPDATE 成功后才触发 BroadcastToRoom，HTTP 200 也由同一 service 函数返回），值上必定相同 —— 这保证后到信号的 merge 必为 no-op，不依赖具体到达顺序

该例外的语义自洽：(i) 该例外**仅**对"发起者自己的 entry"生效，对房间内其他成员仍以 WS 广播为唯一权威信号；(ii) `pet.state.changed.payload.currentState` 与 `response.data.state` 字段层等价（参见上文 line 601 字段语义跨章节等价 + §12.3 line 2241 关键约束），值域 / DB 来源恒等价，先到信号写入后到信号 merge 等价 no-op；(iii) 该规则与上文"对'别人的状态变化'走 WS 唯一权威"语义不矛盾 —— 别人的状态本来就没有"对应的 HTTP 200"信号到达 client，HTTP 信号是 caller 自己发的请求响应，**不**承载别人的状态；self entry 是唯一一个**同时**有两路信号到达 client 的 entry 类型，因此**仅**对 self entry 适用"到达顺序对称无操作"规则。

**self-broadcast 的剩余职责（在 self entry 已被先到信号驱动后）**：当 self-broadcast 是 (a) 路径中**后到**的信号时（HTTP 先到 → UI 已更新 → self-broadcast 到达），它的 client 端职责是 (i) 跨设备一致性校验（未来多端登录场景：另一端在同房间的 client 通过该消息更新自己的视图，与本端无关） + (ii) 服务端入账后真实 broadcast 链路的活性探测信号（client 实装层可统计 self-broadcast 到达率作为 WS 连接健康度指标）—— **不**作为本端 self entry UI 的额外驱动信号（merge 已 no-op）；当 self-broadcast 是 (b) 路径中**先到**的信号时（WS 先到 → UI 已更新 → HTTP 200 到达），它已经承担了 self entry UI 驱动职责，HTTP 200 后到时仅作 server 端入账成功的二次确认信号（client 实装层可作 sync log 审计，不再触发 UI 更新）。

**iOS 实装层后续 Story 引用**：本规则的 client 落地见 Story 15.4（state-sync HTTP 响应触发本地 self entry 更新 + self-broadcast 到达走标准 merge 路径，对 self entry 做 no-op）/ Story 15.2（房间页 ViewModel 接 WS 广播驱动他人 roster）。

**客户端节流约束（iOS 端 self-imposed，server 侧不强制）**：

- **server 不对 `state-sync` 做特殊限频**（仅走 Story 4.5 默认 60 次/分通用 rate_limit 中间件 → 1005）
- **iOS 端**应在动作识别状态机切换的瞬间上报（如 `idle → walk` 转折点），而**不**每秒重复上报当前状态；这条约束是 iOS 端 self-imposed 优化（避免无谓网络流量 + 减轻 server 日志噪声），server 不强制实施，违反不会拒绝请求
- server 应在 log 层关注异常高频上报（如同 user 同 state 1 秒内 > 3 次）作为客户端实装 bug 的间接信号，但**不**触发 3xxx 业务错误码（与 §6.1 步数防作弊 3001 不同 —— 步数同步是资产入账，需 server 端强制阈值；状态同步只是 UI 展示，无资产，无防作弊需求）

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `data.state` | number (int) | 回显入参 `state` 值（server-acknowledged 入账确认）；client **不**应用此值作为"**他人** server 端最终态"反推（理论上该值就是入参，但 client 不应依赖回显语义来推断别人或全局聚合态；如需获取**其他成员** / 全局 pet 状态请通过 `room.snapshot` / `pet.state.changed` WS 广播 / 下次 `GET /home` 获取）。**self-broadcast 例外**：在"发起者自己"的本地 roster pet state 更新场景下，HTTP 200 + `data.state` **是**契约允许的 self-only 权威信号（见上方"WS 广播 vs HTTP 响应的关系（含发起者自己的 self-broadcast 兜底规则）" + line 593 等价分层声明 §5.2 `data.state` 不进入"权威等价桶"的边界界定 —— 该例外仅作 self entry 本地 UI 立即更新依据，**不**让 `data.state` 进入跨 client / 多设备权威等价桶） |

JSON 示例：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "state": 2
  },
  "requestId": "req_xxx"
}
```

> **设计权衡**：本接口 response data 选择**回显入参**（即 `data.state` = request `state`）而非**返回 server 端写入后的最终值**。原因：两者在 happy path 下完全等价（service 层步骤 4 UPDATE 入参值入库，无任何转换 / 截断 / 业务转换），让 client 反推没有信息增量；同时回显语义让 client 实装层（如 iOS Story 15.4）可以用 `response.data.state == request.state` 作为简单的 ack 校验，不需要额外解析 DB 写入结果。后续若引入"server 端状态聚合"语义（如多端同时上报取最新），可由对应 epic 的契约 story 重新审视回显 vs 真实态的选择。

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截（无 Authorization 头 / token 非法 / token 过期 / token 解析失败） |
| 1002 | 参数错误 | `state` 字段缺失 / 类型非 int / 值不在 {1, 2, 3} 范围内 |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截（**已认证路由**按 `user_id` 限频，每用户每分钟 > 60 次；按 Story 4.5 默认值，配置可调） |
| 1009 | 服务繁忙 | DB 异常（UPDATE 执行返回 `err != nil`，含 driver / 网络 / 约束冲突等）/ 内部 panic（见 Story 14.2 service 实装）。**不**包含 `RowsAffected == 0` —— 该分支在本接口下视为幂等成功（详见 §5.2 服务端逻辑步骤 4） |

> **注**：本接口**不**会触发 6001 ~ 6005（房间相关错误） —— 用户不在房间是合法场景（仅不广播 WS，HTTP 仍 200 OK + code = 0），不视为业务错误。同样**不**触发 3001 / 3002（步数相关）/ 4xxx（宝箱）/ 5xxx（装扮）/ 7xxx（表情 / WS）。`pet.state.changed` WS 广播失败也**不**触发任何错误码（仅 server 端 log warning，HTTP response 仍 200 OK + code = 0）。本接口**亦不触发 1003** —— pet-less（用户无活跃默认 pet 行）是与 §5.1 / §10.3 / §12.3 同语义的**合法 edge case**，server 对 pet-less 用户的 state-sync 走 server-acknowledged noop 路径返回 200 OK + code = 0（详见 §5.2 服务端逻辑步骤 3）。

**关键约束**：

- **1002 语义（唯一保留的请求侧错误码）**：1002 是**请求参数本身**不合法（`state` 字段缺失 / 类型非 int / 值不在 {1, 2, 3}），客户端应停止重试并修正参数。本接口**不**触发 1003 —— pet-less 账号（用户无活跃默认 pet 行）与 §5.1 GET /home `data.pet = null` / §10.3 `data.members[].pet = null` / §12.3 `member.joined.payload.pet = null` 是**同一类合法 edge case**（contract 内显式覆盖，非 invariant 损坏），server 对 pet-less 用户走 server-acknowledged noop 路径返回 200 OK + code = 0（详见 §5.2 服务端逻辑步骤 3）；client 实装层（iOS Story 15.4）**不**需要为 pet-less state-sync 做 special-case suppress，正常发起 state-sync 即可，server 自行决定 UPDATE / 广播 / noop 分支
- 用户不在房间（`users.current_room_id == NULL`）**不**视为业务错误 —— 接口仍返回 200 OK + code = 0，**不**广播 WS（与 §10.4 join 失败抛 6004 的语义**不同**：join 是要"操作房间"，无房间归属是前置不满足；state-sync 是要"更新 pet 状态"，无房间归属仅意味着无需广播给他人 —— pet 状态更新本身仍合法 + 仍写库）
- BIGINT 主键 / 外键（`userId` / `petId` 字段在 `pet.state.changed` payload 中）严格按 §2.5 全局约定字符串化下发；**本接口请求 / 响应 body 中无 BIGINT 字段**（仅 `state` 是 int 枚举），无字符串化诉求
- `state` 字段类型为 `number (int)`，**不**字符串化（枚举字段不受 §2.5 BIGINT 字符串化规则约束 —— 类似 §6.1 `motionState`）
- **HTTP 200 vs WS 广播的端到端语义（含 self-broadcast 例外，基于到达顺序对称）**：HTTP 200 = server-acknowledged 入账成功；WS 广播 = 事件通知。两者通过两条独立连接送达，client 实装层不应**假设**两者同时到达 —— 可能 HTTP 200 先到、WS 广播后到（典型场景）；也可能 WS 广播先到、HTTP 200 后到（罕见，但合法）；甚至 WS 广播可能完全收不到（连接断开 / fire-and-forget 失败）。client 实装层（iOS Story 15.2 / 15.4）**必须**对两者的到达顺序做幂等设计：(a) 对**别人**的状态变化 —— 收到 WS 广播 → 更新 roster；HTTP 不参与（自己发的 HTTP 与别人的状态无关）；(b) 对**自己**的状态变化（self-broadcast 例外，见上方"WS 广播 vs HTTP 响应的关系（含发起者自己的 self-broadcast 兜底规则）"完整对称展开 (a)/(b)/(c) 三条规则）—— **任一路径先到的信号都立即更新**本地 self entry 的 roster pet state；后到的信号按字段级 merge 走 **no-op** 路径（因两路信号在 server 端来源同一次 `UPDATE pets ...` 写库行为，值必定相同，merge 幂等）；client 实装层**不**假设固定到达顺序，**不**因 self-broadcast 丢失而 stale（HTTP 200 已驱动 UI），也**不**因 HTTP 200 后到而忽略 self-broadcast 的 UI 驱动职责（当 self-broadcast 先到时）
- **字段语义跨章节等价（分两层 + 受 Story 14.3 前置条件约束）**：§5.2 请求体 `state` 字段 / §5.2 响应体 `data.state` 字段 / §10.3 `data.members[].pet.currentState` / §12.3 `room.snapshot.payload.members[].pet.currentState` / §12.3 `pet.state.changed.payload.currentState` / **§12.3 `member.joined.payload.pet.currentState`** —— 这六处字段的"等价"必须区分**值域/DB 来源层**和**权威/信任层**：
  - **值域 / DB 来源等价（恒成立）**：六处字段类型都是 `number (int)` 枚举 `1 = rest` / `2 = walk` / `3 = run`；语义上都映射到 `pets.current_state` 列（参见数据库设计 §6.4）；happy path 下值相同（无类型转换 / 截断 / 业务变换）
  - **权威 / client 信任层等价（自 Story 14.3 起成立 + 仅对 server → client 四处）**：`pet.state.changed.payload.currentState` / `room.snapshot.payload.members[].pet.currentState` / §10.3 `data.members[].pet.currentState` / **`member.joined.payload.pet.currentState`** 四处自 Story 14.3 起均切换为真实读取 `pets.current_state`（不再固定 `1`），四者承载相同权威级别的状态信号，client 实装层不需要为四者做差异化处理
  - **§5.2 `data.state` 不进入权威等价桶**：本节响应体回显入参（见上文"设计权衡"），是 server-acknowledged ack 信号而非"server 端最终态读出"，与上述三处的权威级别不同；client **不**应把 `data.state` 与 `room.snapshot` / `pet.state.changed` 同等对待（值相同 ≠ 信任级别相同；详见上文 line 557 字段说明）
  - **§5.2 `state`（请求体入参）**：是 client → server 单向写入信号，不参与 server → client 权威等价讨论
  - **Story 14.3 落地前的临时不一致窗口**（Story 14.2 / 14.4 先于 14.3 实装时）：§5.2 / `pet.state.changed` 可下发 2 / 3 真实值，而 §10.3 GET / `room.snapshot` / **`member.joined.payload.pet.currentState`** 在该窗口仍**固定返回 `1`**（详见 §10.3 line 1369 + §12.3 `room.snapshot` line 1968 / line 2073 placeholder 说明 + §12.3 `### 成员加入` `payload.pet.currentState` 字段说明）；该窗口内的**客户端权威信号优先级**为：`pet.state.changed` WS 广播 > `state-sync` HTTP `data.state`（仅 ack）> `room.snapshot` / GET `data.members[].pet.currentState` / **`member.joined.payload.pet.currentState`**（前者一致性最新，后三者在 14.3 前均固定为 placeholder `1`）；这是节点 5 内部容忍的临时契约不一致，14.3 落地后该不一致窗口消失，**四处** server → client `pet.currentState` 字段 —— 即 (i) `pet.state.changed.payload.currentState` (ii) `room.snapshot.payload.members[].pet.currentState` (iii) `member.joined.payload.pet.currentState` (iv) `GET /rooms/{roomId}.data.members[].pet.currentState` —— 统一切换到权威等价层；**不**包括 `POST /pets/current/state-sync` 的 response `data.state`（该字段按 line 610 是 ack-only 信号，**永远不**进入权威等价桶，14.3 落地前后语义不变；Story 15.4 实装层 **禁止**把 HTTP ack `data.state` 提升到与 WS / snapshot 同等的信任级别，HTTP ack 仅作 (a) state-sync 调用成功标志 (b) self-broadcast 按 line 606 self-only 兜底规则在到达顺序对称中担任先到/后到信号之一）。**已知风险（14.3 落地前）**：用户在房间外切 walk/run（§5.2 步骤 5 `current_room_id == NULL` 仅写 DB 不广播）后 join 房间，房间内其他成员通过 `member.joined` 拿到 `currentState: 1`（placeholder），需等该用户**再次**触发 `state-sync` 才能看到真实值；该 race 仅存在于 14.3 落地前的过渡窗口，14.3 落地后 `member.joined` 即时下发真实 `pets.current_state`，stale 风险消失

---

## 6. 步数接口

## 6.1 同步步数

### `POST /api/v1/steps/sync`

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | POST |
| Path | /api/v1/steps/sync |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | 默认（按 Story 4.5 rate_limit 默认值 60 次/分） |
| 幂等 | **非幂等**（每次调用都会写一条 sync_log；但同一 clientTotalSteps 重复同步 → delta = 0 + 仍写日志） |
| 节点 | 节点 3（Epic 7 落地，Epic 8 客户端集成） |

#### 请求体

| 字段 | 类型 | 必填 | 范围/约束 | 说明 |
|---|---|---|---|---|
| `syncDate` | string | 必填 | ISO 8601 date 格式 `YYYY-MM-DD`（如 `"2026-04-23"`），长度严格 10 字符；**且 ∈ [server today - 2 days, server today + 2 days]**（跨时区合理容忍窗口；超出 → 1002） | 客户端按**本机时区**算出的"今天"。**server 直接采用不二次转换**（GAP E：避免跨时区漂移）；同时 **server 校验 syncDate ∈ [server today - 2 days, server today + 2 days] 范围内**（覆盖极端 PST↔JST 17h 时差 + 客户端时钟轻微漂移；防止恶意客户端旋转日期重复入账绕过 daily_cap，Story 7.3 review r7 [P1] anti-cheat）；超出此范围 → 1002 参数错误 |
| `clientTotalSteps` | number (int) | 必填 | `value ≥ 0`（int32 自然上限即可，**不**设业务硬上限；server 不对总值本身做防作弊判断，所有反作弊只针对 delta，见下方"防作弊阈值"） | 客户端读取到的"当天系统累计步数"（如 HealthKit 累计当日 steps）。**不**是增量；增量由 server 按上次同步记录差值计算 |
| `motionState` | number (int) | 必填 | enum: `1` / `2` / `3` | 同步时客户端活动状态。`1 = stationary_or_unknown`, `2 = walking`, `3 = running`（与下方 motionState 枚举一致） |
| `clientTimestamp` | number (int64) | 必填 | Unix 毫秒时间戳，> 0 | 客户端调用接口时的本机时间（毫秒）。仅写 sync_log 审计用，**不**参与差值计算逻辑 |

JSON 示例：

```json
{
  "syncDate": "2026-04-23",
  "clientTotalSteps": 3580,
  "motionState": 2,
  "clientTimestamp": 1776920345000
}
```

#### motionState 枚举

- `1 = stationary_or_unknown`
- `2 = walking`
- `3 = running`

#### 服务端逻辑

1. **认证 & 限频**：auth 中间件校验 Bearer token；rate_limit 中间件按默认配置限频
2. **参数校验**：`syncDate` 格式 + `clientTotalSteps` ≥ 0 + `motionState` ∈ {1,2,3} + `clientTimestamp` > 0；任何不通过 → 返回 1002 参数错误
3. **差值计算**（Story 7.3 service 层实装）：
   - 查 `user_step_sync_logs WHERE user_id=? AND sync_date=?` 取最近一条
   - 若**无**最近记录（首次同步） → `delta = clientTotalSteps`
   - 若**有**最近记录 → `delta = max(0, clientTotalSteps - lastClientTotalSteps)`
   - 倒退场景（clientTotalSteps < lastClientTotalSteps）→ delta = 0，仍**写日志**
   - 跨自然日（sync_date 不同） → 按新一天起算，**不**读上一天的 lastTotal
4. **防作弊阈值**（Story 7.3 增量 AC GAP K 修补；server 权威实施，**iOS 端不加限制**）：
   - **单次截断**：若 `delta > 5000`（配置 `steps.single_sync_cap`） → delta 截断为 5000 + log warning（**不**返回错误，避免误伤真实跑步用户）；接口仍返回 200 OK + `acceptedDeltaSteps = 5000`
   - **当日封顶**：若同 sync_date 当日 `accepted_delta_steps` 历史累计 + 本次截断后 delta **将超过** 50000（配置 `steps.daily_cap`，即 `prevAccepted + curDelta > 50000`） → 当次 delta 强制 = 0 + log warning + **返回 3001 步数同步数据异常**（注：判断条件是"本次入账后是否越界"，**不**是"已达上限"，否则会放过最后一次跨界 sync，例如 prev=49000 + cur=4000 应被拒）
   - 阈值通过配置文件可调；本 story 文档侧只锚定**默认值** 5000 / 50000；具体配置 key 见 Story 7.3 实装
5. **事务**（一个 MySQL 事务内完成，对应数据库设计 §8.2）：
   - UPDATE `user_step_accounts` SET `total_steps += delta, available_steps += delta, version += 1` WHERE user_id=?
   - INSERT `user_step_sync_logs` (user_id, sync_date, client_total_steps, accepted_delta_steps=delta, motion_state, source=1, client_ts=clientTimestamp)
   - 注：`source=1` 代表客户端正常上报；dev grant 走 source=2（admin_grant），见数据库设计 §6.6 + Story 7.5
   - 任一步失败 → 整体回滚，返回 1009 服务繁忙
6. **响应**：返回 `acceptedDeltaSteps`（实际入账增量，可能因截断 / 封顶 ≠ 客户端预期）+ 最新 `stepAccount` 三字段（见下方响应体）

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `data.acceptedDeltaSteps` | number (int) | 实际入账的增量步数（**可能 < clientTotalSteps - lastClientTotalSteps**，因截断 / 封顶 / 倒退场景）；客户端**不**应用此值反推自己的步数（server 权威），仅做日志展示 |
| `data.stepAccount.totalSteps` | number (int64) | 用户累计步数（更新后） |
| `data.stepAccount.availableSteps` | number (int64) | 用户可用步数（更新后） |
| `data.stepAccount.consumedSteps` | number (int64) | 用户已消耗步数（本接口不修改此值，仅返回当前值） |

JSON 示例：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "acceptedDeltaSteps": 120,
    "stepAccount": {
      "totalSteps": 1140,
      "availableSteps": 840,
      "consumedSteps": 300
    }
  },
  "requestId": "req_xxx"
}
```

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截（无 Bearer token / token 过期 / token 解析失败） |
| 1002 | 参数错误 | `syncDate` 格式不符 `YYYY-MM-DD` / `syncDate` 越出 server today ± 2 天容忍窗口（review r7 anti-cheat）/ `clientTotalSteps` 缺失或为负数 / `motionState` 不在 {1,2,3} / `clientTimestamp` ≤ 0 / 任一字段缺失 |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截（**已认证路由**按 user_id 限频，每用户每分钟 > 60 次；按 Story 4.5 默认值，配置可调） |
| 3001 | 步数同步数据异常 | 本次同步入账后当日 `accepted_delta_steps` 累计**将超过** 50000 上限（即 `prevAccepted + curDelta > 50000`），本次 delta 被强制 = 0（防作弊封顶；GAP K 修补） |
| 1009 | 服务繁忙 | DB 异常 / 事务回滚 / 内部 panic（见 Story 7.3 service 实装） |

**关键约束**：

- 错误码 3001 与 1002 的语义差异**必须**明确：1002 是**参数本身**不合法（客户端错），3001 是**参数合法但触发服务端业务限制**（防作弊封顶，本次 delta 被强制 = 0）；客户端处理策略不同（1002 应停止重试并修正参数；3001 应静默接受本次返回 + UX 提示"今日步数已达上限"）。**注意**：3001**不是粘性错误码** —— 当日只要 `prevAccepted + curDelta > 50000` 触发条件成立才返 3001；若客户端后续 sync 是倒退（`clientTotalSteps < lastClientTotalSteps`）或重复（`clientTotalSteps == lastClientTotalSteps`），按"差值计算"步骤 delta 自然 = 0，**仍返 code = 0**，不会再次触发 3001。客户端**不应**假设当日首次 3001 后所有 sync 都失败而停止上报（仍需上报维持 sync_log 审计）
- 单次截断（`delta > 5000`）**不**返回错误，仅 log warning + 截断；客户端从 `acceptedDeltaSteps` 字段感知实际入账值，**不**通过错误码感知截断（avoid false negative：真实跑步用户单次同步可能合法超 5000，体验上不应被错误码打断）

---

## 6.2 获取步数账户

### `GET /api/v1/steps/account`

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | GET |
| Path | /api/v1/steps/account |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | **已认证路由**按 `user_id` 每分钟 60 次（按 Story 4.5 默认值，配置可调；与 §6.1 `POST /steps/sync` 同语义） |
| 幂等 | 幂等（纯查询） |
| 节点 | 节点 3（Epic 7 落地） |

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `data.totalSteps` | number (int64) | 用户累计步数（来自 `user_step_accounts.total_steps`） |
| `data.availableSteps` | number (int64) | 用户可用步数（来自 `user_step_accounts.available_steps`） |
| `data.consumedSteps` | number (int64) | 用户已消耗步数（来自 `user_step_accounts.consumed_steps`） |

> **schema 嵌套差异**：本接口 `data` 字段直接是三档值对象，**不**包一层 `stepAccount`（区别于 §6.1 `POST /steps/sync` 响应的 `data.stepAccount.totalSteps` 嵌套结构）。两端不同设计的原因：§6.1 是"动作型"接口（响应需附带本次动作影响的"账户最新态"，故聚合在 `stepAccount` 子对象下避免与 `acceptedDeltaSteps` 等动作返回值平铺混淆）；§6.2 是"纯读型"接口（响应**只**含账户三字段，无嵌套必要）。客户端 Codable 解析时**必须**区分 `StepsSyncResponse.data.stepAccount.*` 与 `StepsAccountResponse.data.*` 两种结构。

JSON 示例：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "totalSteps": 1140,
    "availableSteps": 840,
    "consumedSteps": 300
  },
  "requestId": "req_xxx"
}
```

#### 服务端行为

1. **认证 & 限频**：auth 中间件校验 Bearer token；rate_limit 中间件按 `user_id` 限频（每用户每分钟 60 次，按 Story 4.5 默认值，配置可调；与 §6.1 `POST /steps/sync` 同 scope —— 已认证业务路由统一按 `user_id` 而非 IP）
2. **查询**：SELECT `total_steps, available_steps, consumed_steps` FROM `user_step_accounts` WHERE `user_id` = ? (token 解析的用户)
3. **响应**：返回三档值；理论上**所有**已登录用户都已在游客登录初始化事务（数据库设计 §8.1 + Story 4.6）创建 step_account 行，故**正常情况**下不会触发 1003
4. **edge case**：如查询返回 0 行（理论不该发生，但作为兜底）→ 返回 1003 资源不存在

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截 |
| 1003 | 资源不存在 | step_account 行不存在（理论不该发生 —— 登录时由 Story 4.6 五表事务初始化；触发即数据 invariant 已损坏，应该 log error） |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截（**已认证路由**按 `user_id` 限频，每用户每分钟 > 60 次；按 Story 4.5 默认值，配置可调） |
| 1009 | 服务繁忙 | DB 查询失败 |

---

## 7. 宝箱接口

## 7.1 获取当前宝箱

### `GET /api/v1/chest/current`

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | GET |
| Path | /api/v1/chest/current |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | 默认（按 Story 4.5 rate_limit 默认值 60 次/分按 user_id 计） |
| 幂等 | 天然幂等（GET 查询，无副作用） |
| 节点 | 节点 7（Epic 20 落地 / Epic 21 客户端集成） |
| 分页 | 无（单条 chest 查询） |
| Query 参数 | **无**（不接受任何 query string） |

#### 请求

无请求体（GET 接口）；仅需 `Authorization: Bearer <token>` Header。

#### 服务端逻辑

1. **认证 & 限频**：auth 中间件校验 Bearer token；rate_limit 中间件按默认配置限频
2. **查询当前 chest**：`SELECT id, status, unlock_at, open_cost_steps FROM user_chests WHERE user_id = ? LIMIT 1`
   - 用户没有 chest 行（理论不该发生，因 Story 4.6 登录初始化已创建首个 chest）→ 4001
3. **动态判定 status**（**不**更新 DB —— 仅在 response 计算，节省写入；真正状态变更在开箱时 Story 20.6 触发）：
   - 数据库 `status = 1 (counting)` AND `unlock_at <= now` → 返回 `status = 2 (unlockable)`, `remainingSeconds = 0`
   - 数据库 `status = 1 (counting)` AND `unlock_at > now` → 返回 `status = 1`, `remainingSeconds = max(0, ceil((unlock_at - now) / 1s))`
   - 数据库 `status = 2 (unlockable)` → 返回 `status = 2`, `remainingSeconds = 0`（已解锁待开）
4. **响应**：返回 `{id, status, unlockAt, openCostSteps, remainingSeconds}` 5 字段

**事务边界规则**：本接口**不**需要 MySQL 事务（仅 1 个 SELECT 查询 + 内存判定）。

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 必填 | 范围/约束 | 说明 |
|---|---|---|---|---|
| `data.id` | string | 必填 | BIGINT 字符串化(与 §2.5 全局约定 + 数据库 `user_chests.id BIGINT UNSIGNED` 一致) | 当前宝箱主键 |
| `data.status` | number (int) | 必填 | 枚举 `1` / `2` | 当前宝箱状态：`1 = counting`（未解锁，倒计时中），`2 = unlockable`（已解锁，可点击开箱）；与数据库 §6.7 `user_chests.status` 同义；与 §5.1 `data.chest.status` 同义；**节点 7 起 server 端按服务端逻辑步骤 3 动态判定，不再以 DB 原始值返回** |
| `data.unlockAt` | string (ISO 8601 UTC) | 必填 | length = 20(如 `"2026-04-23T10:20:00Z"`) | 宝箱解锁时间；与数据库 `user_chests.unlock_at` 同义；client 用作本地倒计时显示基线 |
| `data.openCostSteps` | number (int) | 必填 | 节点 7 阶段固定 `1000` | 开启所需步数；与数据库 `user_chests.open_cost_steps` 同义；与 §5.1 `data.chest.openCostSteps` 同义；MVP 阶段固定为 1000，未来如调整需视为契约变更 |
| `data.remainingSeconds` | number (int) | 必填 | `0 ≤ value ≤ 2^31 - 1` | 距离 unlockAt 的剩余秒数；> 0 表示 counting，= 0 表示已可开启（与 `status = 2` 等价 —— client 可二选一判定）；server 端按 `ceil((unlock_at - now) / 1s)` 计算，到 0 时**不**会出现负数（见服务端逻辑步骤 3 的 `max(0, ...)` 兜底）；client 应基于此本地 `Timer` 每秒递减做 UI 倒计时显示，到 0 时**主动**重新 GET 一次以确认 server 端 status 已切换为 2（避免 server / client 时钟漂移导致 UI 与 server 端状态错位） |

JSON 示例（`remainingSeconds = 253` 表示距解锁还有 253 秒）：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": "5001",
    "status": 1,
    "unlockAt": "2026-04-23T10:20:00Z",
    "openCostSteps": 1000,
    "remainingSeconds": 253
  },
  "requestId": "req_xxx"
}
```

JSON 示例（已解锁待开 edge case）：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": "5001",
    "status": 2,
    "unlockAt": "2026-04-23T10:20:00Z",
    "openCostSteps": 1000,
    "remainingSeconds": 0
  },
  "requestId": "req_xxx"
}
```

#### status 枚举

- `1 = counting`（未解锁，倒计时中）
- `2 = unlockable`（已解锁，可点击开箱）

> **注**：本接口**不**下发 `created_at` / `updated_at` / `version` 字段 —— client 无 UI 用途；`version` 是 server 端 user_chests 表乐观锁版本号，仅在开箱事务内用（Story 20.6 实装层决定），client 不需要感知。

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截（无 Authorization 头 / token 非法 / token 过期 / token 解析失败） |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截（**已认证路由**按 `user_id` 限频，每用户每分钟 > 60 次；按 Story 4.5 默认值，配置可调） |
| 1009 | 服务繁忙 | DB 异常（SELECT 执行返回 `err != nil`，含 driver / 网络 / 慢查询超时等）/ 内部 panic |
| 4001 | 当前宝箱不存在 | 用户在 user_chests 表中无任何行（理论上 Story 4.6 登录初始化必然创建首个 chest，此错误表征数据完整性异常 —— 如 DB 数据被 admin 误删 / migration 顺序错乱等）；client 收到 4001 时应展示"系统异常，请重新登录"+ log error 上报（不应静默） |

> **注**：本接口**不**触发 1002 参数错误（无 query / body 字段可校验）/ **不**触发 4002 / 4003（4002 / 4003 是开箱时机相关错误，仅在 §7.2 POST /chest/open 触发）/ **不**触发 3xxx / 5xxx / 6xxx / 7xxx。

#### 关键约束

- **status 动态判定 vs DB 原始值**：server 端 status 字段值由"DB 原始值 + 当前时间"动态计算，**不**等同于 DB `user_chests.status` 列值；client 收到 `status = 2` 不可推断 DB 行也是 `status = 2`（DB 可能仍为 `status = 1` 但 `unlock_at <= now`）；该设计是为节省一次 UPDATE 写入（每次 GET 都更新 DB 状态会严重放大写入压力，而 status = unlockable 是无副作用的查询时态判定）。真正的 DB 状态变更仅在开箱事务（Story 20.6）成功后发生（删旧 chest + 新建下一轮 status = 1 chest）
- **remainingSeconds 不会为负**：服务端按 `max(0, ceil((unlock_at - now) / 1s))` 计算；即使 server 时钟跳变 / `unlock_at` 略早于 now，也只会返回 0；client 解析层**应**按 `Int` 处理（不是 `UInt` —— Swift 端 `UInt` 在解析时若收到负数会 crash，虽 server 保证不发但 client 应防御性按 `Int`）
- **本地倒计时与 server 状态校对**：client **应**在 `remainingSeconds` 倒到 0 时**主动**调一次 GET /chest/current 校正 status（避免本地时钟漂移导致 UI 误判）；如果 GET 返回 `status = 1` 且 `remainingSeconds > 0`（如本地早跳到 0，但 server 仍认为未解锁）→ client 以 server 响应为准重置本地 Timer；如果 GET 返回 `status = 2` 且 `remainingSeconds = 0`（与本地一致）→ UI 切换"点击开箱"按钮可点状态
- **首次登录后 1 秒内调本接口**：刚 Story 4.6 登录初始化的 chest `unlock_at = now + 10 min`；首次 GET 返回 `status = 1`, `remainingSeconds ≈ 600`，与 `GET /home` 节点 2 阶段示例一致（§5.1 行 423-425）
- **BIGINT 主键字符串化**：`data.id` 遵循 §2.5 全局约定字符串化；其他字段（`status` / `openCostSteps` / `remainingSeconds`）是 int，**不**字符串化
- **client 缓存策略**：本接口数据有时效性（每秒变化），client **不应**做长时间本地缓存；建议每次进入首页 / 从其他页面返回首页时主动重新调一次（不依赖任何 cache 中间件 / ETag）

---

## 7.2 开启宝箱

### `POST /api/v1/chest/open`

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | POST |
| Path | /api/v1/chest/open |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | 默认（按 Story 4.5 rate_limit 默认值 60 次/分按 user_id 计 —— 开箱业务上单用户不会高频触发，此限频是兜底） |
| 幂等 | **支持**（通过 request body `idempotencyKey` 字段 + MySQL `chest_open_idempotency_records` 表同事务持久化实现；详见服务端逻辑） |
| 节点 | 节点 7（Epic 20 落地 / Epic 21 客户端集成） |
| 事务边界 | **MySQL 事务**（FOR UPDATE 行锁 + 多表写入；详见服务端逻辑） |

#### 请求体

| 字段 | 类型 | 必填 | 范围/约束 | 说明 |
|---|---|---|---|---|
| `idempotencyKey` | string | 必填 | 1 ≤ length ≤ 128；只允许 `[A-Za-z0-9_:-]` | 幂等键；client **应**在每次点击"开箱"按钮时生成新的 key（如 `"chest_open_{userId}_{nanoTimestamp}"`），网络抖动重试时**复用同一 key**（避免重复扣步数 + 重复开箱）；server 端用此 key 在 **MySQL `chest_open_idempotency_records` 表**做**同事务持久化**幂等记录（预声明 + 业务写入 + 最终化原子提交在同一事务内，详见服务端逻辑步骤 5a / 5k）—— 同 key 重试时直接从 DB 取首次结果返回（**不**触发新业务事务）；详见 §7.2 服务端逻辑步骤 3 / 5a / 5b / 5k / 7 + §7.2「client 重试策略」 |

JSON 示例：

```json
{
  "idempotencyKey": "open_chest_20260423_001"
}
```

#### 服务端逻辑

1. **认证**：auth 中间件校验 Bearer token（必走；**注**：rate_limit 中间件**不**在本接口的 middleware 链上 —— 见步骤 4 + 本节末「关键约束」「rate_limit 位置 r10 调整」段；这是本接口对"统一 rate_limit middleware"基线的契约级例外，由 r10 review 锁定）
2. **参数校验**：`idempotencyKey` 必填 + length 1-128 + 字符集 `[A-Za-z0-9_:-]` —— 不满足 → 1002
3. **幂等命中预检**（**在 rate_limit 之前**；handler 入口，**不开** MySQL 事务 —— 这一步是"轻量只读 SELECT + 短路返回"）：
   - 执行 `SELECT status, response_json FROM chest_open_idempotency_records WHERE user_id = ? AND idempotency_key = ?`（autocommit / `txCtx` 之外的普通 conn）
   - **命中 `status = 'success'`**（首次开箱已成功，`response_json` 已写入）→ 跳过步骤 4（**不**计入 rate_limit 配额）+ 跳过步骤 5（**不**进业务事务）→ **反序列化 `response_json` + 补算 `data.nextChest.status = (unlock_at > now) ? 1 : 2` + `data.nextChest.remainingSeconds = max(0, ceil((nextChest.unlockAt - now) / 1s))` + 填入当前请求的 `requestId`**（`response_json` 缓存**不**包含 `nextChest.status` / `nextChest.remainingSeconds` / `requestId`，由 server 在响应序列化时实时填入；详见步骤 5j 序列化说明）→ 返回 `200`
   - **命中 `status = 'pending'`**（极窄 race 窗口：在 InnoDB unique-key 锁释放后、首个事务对 idempotency 行的 UPDATE 尚未对当前 snapshot 可见的瞬间；本设计下首次事务的 `status` 推进与 commit 原子，本 case 实际几乎不会出现 —— 仅作兜底）→ 跳过步骤 4（**不**计入 rate_limit 配额；理由：本请求不是"真新业务消费"，是首次事务的并发同 key 重试观察）+ 跳过步骤 5（**不**进业务事务，避免与首个事务在 InnoDB unique-key 锁上无意义阻塞）→ 返回 **1008**（幂等冲突；client 应稍后用**同** key 退避重试，详见 §7.2「client 重试策略」1008 条款）
   - **未命中**（行不存在 —— 首次到达 / 首次事务已 rollback 后到达）→ 继续步骤 4（限频检查）
4. **限频检查**（**仅对未命中 idempotency 的"真新请求"做**；handler 内层，**不**在 middleware 链上）：按 `user_id` 维度，每用户每分钟限 60 次（与 §4.5 钦定的默认 rate_limit policy 一致）—— 超限 → 返回 **1005 操作过于频繁**；未超 → 继续步骤 5
5. **MySQL 事务开始**（`txManager.WithTx(ctx, fn)` 包；**幂等预声明与业务写入在同一个事务内**；所有 repo 调用用 `txCtx`）：
   a. **幂等记录预声明**（事务内第一条语句，借 `UNIQUE(user_id, idempotency_key)` 约束做单语句原子声明 + 同事务回滚兜底）：
      - 执行 `INSERT INTO chest_open_idempotency_records (user_id, idempotency_key, status, response_json, created_at, updated_at) VALUES (?, ?, 'pending', NULL, NOW(3), NOW(3)) ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`
      - **InnoDB 锁语义**（关键）：本 INSERT 会对 unique key `(user_id, idempotency_key)` 取**排他锁**；同 key 并发请求里只有一个事务能进入"持锁状态"，其他事务**在 INSERT 语句上阻塞**等待，直到首个事务 commit 或 rollback 释放锁。这是契约层对"同 key 并发不进双业务事务"的硬保证（**不**靠 MySQL 业务表行锁兜底——FOR UPDATE 在步骤 5c 才执行，已晚于幂等声明）
      - **`affected_rows = 1`**（新行 INSERT，本请求是同 key 首次到达 **或** 首次事务已 rollback 把 pending 行回滚后到达）→ 进入步骤 5c 业务步骤
      - **`affected_rows = 0`**（行已存在，且首次事务已 commit；锁释放后 `ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)` 是 no-op）→ 进入步骤 5b 短路分支
   b. **短路分支**（命中已存在的成功行）：
      - 在**同一事务内**执行 `SELECT status, response_json FROM chest_open_idempotency_records WHERE user_id = ? AND idempotency_key = ?`（首次事务已 commit；本事务读到的就是 final 状态。**注**：首次事务 rollback 的 case 不会进本分支 —— rollback 时 pending 行随之消失，同 key 重试在步骤 5a 拿到 `affected_rows = 1` 直接走全流程，**不**进入步骤 5b）
      - `status = 'success'`（首次开箱已成功，`response_json` 已写入）→ COMMIT（本事务无写操作，COMMIT / ROLLBACK 等价）→ **反序列化 `response_json` + 补算 `data.nextChest.remainingSeconds = max(0, ceil((nextChest.unlockAt - now) / 1s))` + 填入当前请求的 `requestId`**（`response_json` 缓存**不**包含 `requestId` —— 该字段是**本次**请求的 trace ID，每次重试都不同；详见步骤 5j `response_json` 字段选择说明）→ 返回 `200`
      - `status = 'pending'`（极窄 race 窗口：在 InnoDB unique-key 锁释放后、首个事务对 idempotency 行的 UPDATE 尚未对当前 snapshot 可见的瞬间；本设计下首次事务的 `status` 推进与 commit 原子，本 case 实际不会出现 —— 仅作兜底）→ COMMIT → 返回 **1008**（幂等冲突；client 应稍后用**同** key 退避重试，详见 §7.2「client 重试策略」1008 条款）
   c. **FOR UPDATE 锁 chest**（步骤 5a `affected_rows = 1` 路径继续）：`SELECT id, status, unlock_at, open_cost_steps, version FROM user_chests WHERE user_id = ? FOR UPDATE`
      - 没有 chest 行 → rollback → 4001
   d. **判定 unlockable**：动态判定逻辑与 §7.1 服务端逻辑步骤 3 一致 —— `(status = 1 AND unlock_at <= now) OR status = 2` 视为可开启；否则 → rollback → 4002
   e. **FOR UPDATE 锁 user_step_accounts**：`SELECT total_steps, available_steps, consumed_steps, version FROM user_step_accounts WHERE user_id = ? FOR UPDATE`
      - **没有行**（理论不该，Story 4.6 已初始化）→ rollback → **1009 服务繁忙**（数据完整性异常，**非**步数不足；本 case 与"available_steps < 1000"语义不同：前者表征 server 端数据缺失，client 不应提示用户"走路赚步数"，应作为 server 错误重试 / 联系客服；详见"可能的错误码"表 1009 行）
      - `available_steps < 1000` → rollback → 3002（**仅**此路径映射 3002）
   f. **扣步数 + 加 consumed**：`UPDATE user_step_accounts SET available_steps = available_steps - 1000, consumed_steps = consumed_steps + 1000, version = version + 1 WHERE user_id = ? AND version = ?`（乐观锁；`user_step_accounts` 主键是 `user_id` 不是 `id` —— 见数据库设计 §5.4）
      - `affected_rows = 0`（version 不匹配，并发写入）→ rollback → 1009（按"数据冲突重试"映射；事务回滚时步骤 5a INSERT 的 `pending` 行也跟着回滚 → client 用同 key 重试在步骤 5a 拿到 `affected_rows = 1` 走全流程；推荐 client 用新 idempotencyKey 重试以语义清晰，与"网络层重试"区分）
   g. **加权抽取 cosmetic_item**：`SELECT id, code, name, slot, rarity, drop_weight, asset_url, icon_url FROM cosmetic_items WHERE is_enabled = 1` → 按 `drop_weight` 加权抽取 1 条 → 拿到 `cosmetic_item_id`（`drop_weight` 必须在 SELECT 子句中，否则后续加权抽取无权重输入；与数据库设计 §5.8 `cosmetic_items.drop_weight` 同义）
      - 没有任何 enabled cosmetic_items（seed 未执行）→ rollback → 1009
   h. **写 chest_open_logs**：`INSERT INTO chest_open_logs (user_id, chest_id, cost_steps, reward_user_cosmetic_item_id, reward_cosmetic_item_id, reward_rarity, created_at) VALUES (?, ?, 1000, 0, ?, ?, NOW())` —— **节点 7 阶段 `reward_user_cosmetic_item_id` 固定为 `0`**（占位值，因不创建 user_cosmetic_items 实例；节点 8 Story 23.5 修改本步骤为先 INSERT user_cosmetic_items 拿到 id 再填入此处）

> **跨文档分阶段契约说明（重要）**：本步骤 h「节点 7 阶段**不**创建 `user_cosmetic_items` 实例」是**渐进式契约的节点 7 切片**，与"最终契约" §14.1「开箱事务必须发放装扮实例」+ 数据库设计 §8.3「插入一条 `user_cosmetic_items`」**有意分阶段**，并非矛盾：
> - §14.1 / DB §8.3 描述的是**最终契约**（节点 8 / Epic 23 完成后稳态）—— 开箱事务包含「发放装扮实例」步骤
> - §7.2.4h（本步骤）描述的是**节点 7 阶段切片**（Epic 20 / Epic 21 / 节点 7 demo 验收阶段）—— 暂不创建实例，`reward.userCosmeticItemId` 返回占位 `"0"`，详见本节末「关键约束」§7.2 节点 7 vs 节点 8 阶段说明
> - **server / iOS 实装者读到此处的 disambiguation 规则**：节点 7 阶段（Story 20.6 / Epic 21）**必须**遵循本步骤 h 的"不创建实例 + reward_user_cosmetic_item_id = 0"路径；节点 8 阶段（Story 23.5 落地后）由 Story 23.5 acceptance 修改本步骤为"先 INSERT user_cosmetic_items 拿到 id 再填入此处"，同时 §14.1 / DB §8.3 自然回归一致语义
> - **本契约 finalize（Story 20.1）冻结的是"两段语义都生效，按节点切换"的状态**，**不是**§7.2 推翻 §14.1 / DB §8.3
   i. **刷新下一轮 chest**：`DELETE FROM user_chests WHERE id = ?`（旧 chest）→ `INSERT INTO user_chests (user_id, status, unlock_at, open_cost_steps, version) VALUES (?, 1, NOW() + INTERVAL 10 MINUTE, 1000, 0)`（新 chest）
   j. **序列化可缓存 response payload**（**不**包含时间派生字段、**不**包含 `requestId` trace 字段）：在内存中构造 `{reward, stepAccount, nextChest}` 三段嵌套结构 —— **`nextChest` 内仅持久化 `{id, unlockAt, openCostSteps}` 三字段；`nextChest.status` 与 `nextChest.remainingSeconds` 均是时间派生字段（与 §7.1 GET /chest/current 一致：`status = (unlock_at > now) ? 1 : 2`、`remainingSeconds = max(0, ceil((unlock_at - now) / 1s))`）、`requestId` 是每次请求独立的 trace ID（顶层信封字段），三者均由 server 端在响应序列化时实时填入（`status` / `remainingSeconds` 按上述公式；`requestId` 填**当前**请求的 trace ID），不写入 `response_json` 缓存**（避免同 key 重试时回放 stale `status` / stale 倒计时与 GET /chest/current 实时计算结果漂移 —— 尤其当重试发生在新 chest 已到期解锁的时刻，回放 `status=1 (counting)` 会与重新补算的 `remainingSeconds=0` 形成不可能组合；同时避免重试请求返回首次请求的 `requestId` 破坏 log/trace 关联语义）；序列化在事务内完成是因为响应数据来自事务内的查询结果 + 业务语义需要原子持久化"首次的响应"，避免 commit 后再补 UPDATE 引入二次 commit 失败窗口
   k. **更新 idempotency 记录**（**在 commit 之前**，与业务表写入同一事务原子提交）：`UPDATE chest_open_idempotency_records SET status = 'success', response_json = ?, updated_at = NOW(3) WHERE user_id = ? AND idempotency_key = ?`（行已在步骤 5a INSERT；本步骤仅做字段更新，写入 j 步序列化的可缓存 payload）
   l. **事务提交**
6. **首次成功路径的响应组装**（步骤 5l commit 成功后）：
   - 内存里的 `{reward, stepAccount, nextChest}` payload 上**补算** `nextChest.status = (nextChest.unlockAt > now) ? 1 : 2` + `nextChest.remainingSeconds = max(0, ceil((nextChest.unlockAt - now) / 1s))`（两个字段**同源同时刻**计算，与 §7.1 GET /chest/current 同一公式；新 chest 刚 INSERT 后通常 `status = 1` + `remainingSeconds ≈ 600`）→ 返回 `200`
7. **事务后处理**（无 post-tx Redis 写回 / 无 best-effort post-rollback failed upsert）：
   - **事务 commit 成功**（步骤 5a~5l 全部成功）→ idempotency 记录的 `status = 'success'` + `response_json`（**不含** `nextChest.status` / **不含** `nextChest.remainingSeconds` / **不含** `requestId`）已**原子**落盘；同 key 后续重试在步骤 3 幂等命中预检命中 `status = 'success'` 分支，反序列化 `response_json` + 重新按"当前时刻"补算 `nextChest.status = (unlock_at > now) ? 1 : 2` + `nextChest.remainingSeconds = max(0, ceil((unlock_at - now) / 1s))`（两字段同源同时刻计算保证内部一致；尤其重试发生在新 chest 已到期解锁的时刻会返回 `status = 2` + `remainingSeconds = 0`，与 §7.1 GET 同一秒查询完全对齐）+ 填入**当前**请求的 `requestId` 后返回，**无任何 Redis 依赖**、**无 stale status**、**无 stale 倒计时**、**无 stale requestId**；**重试请求绕过步骤 4 限频检查**（r10 review 锁定，详见步骤 3 / 步骤 4 + 本节末「关键约束」「rate_limit 位置 r10 调整」段）
   - **事务 rollback**（步骤 5a~5k 任一子步骤失败 / 5l commit 失败）→ 整个事务回滚（**包括步骤 5a INSERT 的 `pending` 行也跟着回滚**，因为预声明与业务写入在同一事务）→ 同 key 重试在步骤 3 未命中（行不存在）→ 步骤 4 限频通过（首次失败的同 key 重试也走"真新请求"路径计入配额，确保 retry storm 在限频上有节流）→ 步骤 5a `affected_rows = 1`，重新走完整流程；本路径**无副作用**（chest 状态未变 / 步数未扣 / log 未写）、**无 pending 残留**（rollback 已彻底清除）；**无需任何 best-effort 异步补偿** —— 事务原子性已保证"rollback 路径无副作用 + 同 key 重试安全"，无 client 卡死风险，写 `status = 'failed'` 反而会与"client 立即同 key 重试 → 新事务 commit success"的合法路径在 UNIQUE 约束上竞争，把后到达的 success 行错误覆盖为 failed（r7 review 锁定的 race condition；详见本节末「关键约束」"r7 移除 best-effort failed upsert 决策"段）
8. **响应**：返回 `{reward, stepAccount, nextChest}` 三段嵌套 payload（首次成功路径来自步骤 6 补算后的结果；重试 cached 路径来自步骤 3 反序列化 + 补算 `nextChest.status` 与 `nextChest.remainingSeconds`（同源同时刻）+ 填入当前 `requestId`）

**事务边界规则**（与 数据库设计 §8.3 一致）：

- 步骤 5 是单一 MySQL 事务，**包含**幂等记录预声明（步骤 5a INSERT）+ 幂等命中短路读（步骤 5b SELECT）+ 业务表写入（5c~5i）+ 幂等记录最终化（步骤 5k UPDATE 设 `status='success'` + `response_json`）的**原子提交** —— 这是 r6 review 锁定的核心契约升级（r5 把幂等预声明写在"业务事务之前的独立 INSERT"，rollback 时 pending 行不跟着回滚 → 同 key 永久 1008 卡死；r6 将预声明纳入业务事务彻底消除该悖论）：**幂等状态 + 业务数据必须同事务原子写**，从存储层根治"幂等记录 / 业务数据状态不一致 + pending 残留卡死"风险；**事务内部任何操作都不依赖 Redis**（r4 的 Redis sentinel / 双 TTL 设计在 r5 已废弃，详见步骤 5a 注解 + r6 修订说明）
- **DB claim 在事务内首条语句**：步骤 5a 的 `INSERT ... ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)` 借 `UNIQUE(user_id, idempotency_key)` 约束做 single-statement 原子声明；同 key 并发请求里只有一个事务能拿到 unique-key X-lock，其他事务**在 INSERT 语句上阻塞**等待，直到首个事务 commit / rollback 释放锁后再继续：
  - 首个事务 **commit** → 锁释放 → 其他事务的 INSERT 看到行已存在 → `affected_rows = 0` → 走步骤 5b 短路 → 返回 200 + cached `response_json`（首次成功语义；r7 review 后**无** `failed` 短路分支）
  - 首个事务 **rollback** → 锁释放 + pending 行回滚 → 其他事务的 INSERT 看到行不存在 → `affected_rows = 1` → 走步骤 5c 业务全流程（与首次到达等价；**无**异步 failed 补偿写）
  - 这是契约层对"同 key 并发不进双业务事务" + "rollback 后同 key 重试不卡 pending"两条不变量的硬保证（**不**靠 MySQL 业务表行锁兜底——FOR UPDATE 在 5c 才执行，已晚于幂等声明）
- 加权抽奖（步骤 5g）使用 mocked `random.Reader` 接口（Story 20.6 实装时 inject），便于单测断言权重分布；本接口契约层**不**钦定具体算法（alias method / 线性扫描皆可）
- **事务失败时 idempotency 状态**：步骤 5 任何子步骤（a~l）失败 → 整个事务（**包含**步骤 5a INSERT 的 `pending` 行）一起回滚 → 同 key 重试在步骤 3 幂等命中预检未命中（行不存在）→ 步骤 4 限频通过 → 步骤 5a 拿到 `affected_rows = 1`，**与首次到达等价**走全流程；本路径**无任何残留状态需要清理**、**无需任何 best-effort 补偿写 `failed` 占位**（r7 review 锁定）—— 事务原子性已保证 rollback 路径"无副作用 + 同 key 重试安全"，写 `failed` 反而引入"client 同 key 重试 success 被异步 failed upsert 覆盖"的 race condition
- **DB 持久化幂等的写后保证**（r7 review 修订，与 §7.2.3 / §7.2.5 配套）：
  - **成功路径**：步骤 5l commit 成功 → idempotency 行的 `status = 'success'` + `response_json`（**不含** `nextChest.remainingSeconds` / **不含** `requestId`，避免回放 stale 倒计时 / 破坏 trace 语义）已**原子**落盘 → 同 key 重试在步骤 5a 拿到 `affected_rows = 0` + 步骤 5b 走 `status = 'success'` 分支直接返回缓存的 `response_json` + 实时补算 `remainingSeconds` + 填入**当前**请求的 `requestId`；**零 Redis 依赖**、**零"SET 失败 client 卡死"风险**、**零回放 stale 倒计时**、**零回放 stale requestId**
  - **失败路径**：业务事务回滚（**包含** idempotency 行）→ 同 key 重试自然走全流程，**与首次到达等价**；**无任何异步补偿写**（r7 review 锁定移除 best-effort failed upsert，详见本节末关键约束 r7 决策段）
  - **client 视角始终安全**：DB 同事务保证"server 端首次到底成不成功"由 DB 单一可信源决定（pending / success 二态；`pending` 仅存在于事务持锁期间，对其他事务不可见 —— 它们在 InnoDB unique-key 锁上阻塞；事务 rollback 时 pending 行自动消失），client 同 key 重试**永远**能拿到准确状态：要么命中 success cached response，要么 INSERT `affected_rows = 1` 等价首次到达，UI 也无需展示"奖励可能已发放，请联系客服"等不确定文案 —— r4 的 60s 外重复出箱风险 + r5 的 pending 卡死风险 + r6 的 failed-upsert race 风险均已彻底消除
  - **server 端不变量**（实装锁定）：
    - 步骤 5a / 5k 任何 DB 错误（driver / 网络 / 约束冲突等）→ 整体回滚 → 返回 1009；**不**对已 commit 事务做反向补偿（不允许 server 主动撤销 reward / 退步数）
    - **禁止** server 在事务 rollback 后异步写 `failed` 占位行（r7 review 锁定 —— 该补偿与 client 同 key 重试的合法 success 路径在 UNIQUE 约束上竞争，可能错误覆盖 success → failed；事务原子性已保证 rollback 路径同 key 重试安全，无需补偿）
    - **`response_json` 缓存内容钦定**（r7 review 锁定，r9 review 进一步收紧时间派生字段范围）：仅持久化 `{code, message, data: {reward.*, stepAccount.*, nextChest.{id, unlockAt, openCostSteps}}}`；**`nextChest.status` + `nextChest.remainingSeconds` 均不写入 `response_json`**（与 §7.1 GET /chest/current 一致：两者都是 time-derived 字段 —— `status = (unlock_at > now) ? 1 : 2`、`remainingSeconds = max(0, ceil((unlock_at - now) / 1s))`；由 server 在响应序列化时按"当前时刻"**同源同时刻**实时计算填入，r9 review 锁定：若仅回放 stale `status=1` 而仅重算 `remainingSeconds`，则同 key 重试发生在新 chest 已解锁时刻会返回 `status=1` + `remainingSeconds=0` 不可能组合，与 §7.1 GET 同一秒查询结果漂移）；**顶层 `requestId` 也不写入 `response_json`**（每次请求独立的 trace ID，重试请求必须返回当前请求的 `requestId` 以维持 log / trace 关联语义；详见 §7.2 字段表 `requestId` 行注解）。首次成功路径与同 key 重试 cached 路径走同一序列化逻辑，与 §7.1 GET /chest/current + §2 / §13 全局信封语义对齐，无任何 stale 漂移
    - **彻底移除 Redis 在 chest_open idempotency 路径上的角色**（r4 的 sentinel / 双 TTL / SET / DEL / counter `chest_open_idem_cache_writeback_failed_total` 全部废弃；本接口的 Redis 仅在 rate_limit 中间件中保留，与 §4.5 一致）
- **rate_limit 位置 r10 调整**（r10 review 锁定，**修订** r5~r9 的"rate_limit 走统一 middleware + 命中 idempotency 仍消耗配额"决策）：
  - **背景 / 被反驳的 r5~r9 设计**：r5~r9 沿用 §4.5 钦定的"统一 rate_limit middleware（auth 之后、handler 之前；按 user_id 60/min）"基线，本接口 client 同 key 重试时 rate_limit 在 middleware 层先消耗配额、idempotency 命中检查在 handler 内才执行。
  - **r10 review 锁定的 break 路径**：用户首次 `POST /chest/open` 成功（server 端 idempotency 行 `status = 'success'` 已 commit）+ client 因网络 timeout 未收到 200 → client 用同 key 退避重试 → **每次重试都被 rate_limit middleware 计入 quota** → 60 次重试后 client 拿到 **1005 操作过于频繁**，永远无法读取 cached success → client UX 无法确认"奖励是否已发放"，破坏 §7.2 「同 key 重试始终安全」契约承诺（详见 §7.2 client 重试策略段）。极端 case 下 client 可能因此向用户错误展示"开箱失败"或诱导用户联系客服 + 跨端数据状态严重不一致。
  - **r10 决策（方案 A）**：把 **rate_limit 检查从 middleware 层挪到 handler 内层**，**置于幂等命中预检之后**：
    - **认证 middleware 仍是 middleware 层**（步骤 1）—— Bearer token 校验对所有请求强制
    - **rate_limit 不再走 middleware 链**（对本接口而言）—— `POST /chest/open` 在路由层挂载时**显式 opt-out 全局 rate_limit middleware**（实装层由 Story 20.6 钦定路由配置；contract 层仅要求：rate_limit 检查发生在步骤 4 而非 middleware 链）
    - **handler 入口先做幂等命中预检**（步骤 3，autocommit SELECT chest_open_idempotency_records）：命中 `status = 'success'` → 返回 cached response（**跳过步骤 4 rate_limit**）；命中 `status = 'pending'` → 返回 1008（**跳过步骤 4 rate_limit**）；未命中 → 继续步骤 4
    - **rate_limit 仅对"真新请求"做**（步骤 4）：按 user_id 60/min；超限 → 1005；未超 → 继续步骤 5 业务事务
  - **理由**：(a) cached replay 路径 = 用户已有"首次成功落盘"事实 + client 仅做"读取已发放奖励"动作 + server 不消耗任何业务资源（无业务事务）→ 与 GET 类查询等价，不该消耗 rate_limit 配额 = 与"60/min 限频是对业务消费的节流"语义对齐；(b) pending 路径 = 与首个事务并发 + server 不进新业务事务（避免 InnoDB unique-key 锁上无意义阻塞）→ 也不该消耗配额；(c) 真新请求（未命中 / rollback 后到达）= server 进业务事务、占 chest / step_account FOR UPDATE 锁、走加权抽奖、INSERT chest_open_logs 等资源 → 必须计入 rate_limit 配额，否则被攻击者通过"频繁换新 idempotencyKey"绕过限频；(d) 同 key 重试 storm（如 client bug 退避策略错误）= 首次成功后所有同 key 重试都命中 cached → 不计入配额，无 starvation 风险；首次失败后所有同 key 重试都走真新路径 → 计入配额，retry storm 在限频上有节流（详见步骤 7 「事务后处理」rollback 路径）
  - **r10 决策不采纳方案 B 的理由**：方案 B（middleware 接受 cache-hit hint / response 完成时 decrement counter）有 race 风险（计数器在 middleware-pre 与 handler-post 之间有窗口）+ 需要修改 Story 4.5 钦定的 rate_limit middleware 接口，跨 epic 改动成本高于方案 A
  - **跨接口影响**（Story 20.6 实装 + 节点 11 / Epic 32 复用启示）：本调整**仅对 §7.2 POST /chest/open 生效**，**不**影响节点 2 / 3 / 4 / 5 / 6 其他已认证业务路由（它们继续走 §4.5 钦定的统一 rate_limit middleware）；若节点 11 `POST /api/v1/compose/upgrade` 复用同一持久化 idempotency 模式（Story 32.4 锚定），**应**采用相同的"handler 内层 rate_limit + idempotency 预检在前"结构，**禁止**沿用 r5~r9 的"middleware 层 rate_limit + 重试消耗配额"老路径
  - **实装层落点**：详见 Story 20.6 handler 实装；契约层只要求"幂等命中预检（步骤 3）在 rate_limit 检查（步骤 4）之前 + cached replay 不计入配额"两条不变量
- **为什么不用 Redis 做幂等声明**（r5 review 锁定 / r6 review 沿用）：Redis 是**非事务存储**，与 MySQL 不能形成原子写。r4 的"sentinel TTL = 60s + final-response TTL = 24h"双 TTL 方案在"事务 commit 成功 + Redis SET final-response 失败"的 case 下，sentinel 60s 自然过期后 **client 无法区分**两种语义："首次 commit 成功，资产已落盘"vs"首次事务回滚，资产未落盘"——前者 client 换新 key 重试会**重复扣 1000 步 + 重复出箱**，后者换新 key 是安全的；client 没有可观测信号做这个区分（server 异步 SET 重试失败 + log.Error 仅在 server 端可见）。把 idempotency 记录搬到 MySQL **同事务**写后（r5 锁定 DB 持久化路径，r6 进一步把预声明也纳入业务事务），"首次是否成功"由 DB 单一可信源决定（步骤 5k 在 commit 前更新 `status = 'success'` + 写 `response_json`；commit 原子）—— client 同 key 重试**始终安全**，无需区分 case，也无需 UI 提示"奖励可能已发放"。该方案彻底消除 Redis 写回失败导致的"60s 外重复出箱"风险

#### 响应体

成功（code = 0）：

**`data.reward` 字段表**（开箱奖励，节点 7 阶段仅展示不入仓）：

| 字段 | 类型 | 必填 | 范围/约束 | 说明 |
|---|---|---|---|---|
| `data.reward.userCosmeticItemId` | string | 必填 | **节点 7 阶段固定字符串 `"0"`**（占位值；节点 8 Story 23.5 起为真实 BIGINT 字符串化主键） | 玩家装扮实例 id；client UI 层**禁止**展示此字段（节点 7 是占位 / 节点 8 仅用于后端审计 + 跨端关联，UI 无展示需求）；client 解析层**应**按 `String` 处理（不是 Optional）；client **禁止**通过 `userCosmeticItemId == "0"` 动态判断节点 ——业务路径以部署版本决定，避免节点 8 升级后 dead branch |
| `data.reward.cosmeticItemId` | string | 必填 | BIGINT 字符串化(与 §2.5 全局约定 + 数据库 `cosmetic_items.id BIGINT UNSIGNED` 一致)；length ≥ 1 | 装扮配置 id（**不是**玩家实例 id —— 与 `userCosmeticItemId` 区分清楚）；与 §8.1 / §8.2 `cosmeticItemId` 同义；与 数据库 §5.8 `cosmetic_items.id` 同义 |
| `data.reward.name` | string | 必填 | 1 ≤ length ≤ 64；与数据库 `cosmetic_items.name VARCHAR(64)` 一致 | 装扮名称（如 `"星星围巾"`），client UI 层用作奖励弹窗文字 |
| `data.reward.slot` | number (int) | 必填 | 枚举 `1` / `2` / `3` / `4` / `5` / `6` / `7` / `99`（与数据库 §6.8 `cosmetic_items.slot` 同义：1=hat / 2=gloves / 3=glasses / 4=neck / 5=back / 6=body / 7=tail / 99=other） | 部位枚举；client UI 层可用作图标分类 / 排序辅助 |
| `data.reward.rarity` | number (int) | 必填 | 枚举 `1` / `2` / `3` / `4`（与数据库 §6.9 `cosmetic_items.rarity` 同义：1=common / 2=rare / 3=epic / 4=legendary） | 品质枚举；client UI 层用作奖励弹窗颜色 / 边框 / 特效 |
| `data.reward.assetUrl` | string | 必填 | 1 ≤ length ≤ 255；不允许空字符串 `""`（开箱奖励必须有 asset） | 装扮资源 URL（PNG / GIF / WebP / SVG 等图片资源）；MVP 阶段允许 placeholder URL（如 `https://placehold.co/128x128?text=Hat-Yellow`，由 Story 20.3 seed 决定）；与 §8.1 / §8.2 `assetUrl` 同义；与 数据库 §5.8 `cosmetic_items.asset_url` 同义 |
| `data.reward.iconUrl` | string | 必填 | 1 ≤ length ≤ 255；不允许空字符串 `""`（开箱奖励必须有 icon） | 装扮图标 URL（小尺寸预览图，奖励弹窗 + 仓库列表展示用）；与 §8.1 / §8.2 `iconUrl` 同义；与 数据库 §5.8 `cosmetic_items.icon_url` 同义 |

**`data.stepAccount` 字段表**（开箱后步数余额，与 §6.2 GET /steps/account 字段同义）：

| 字段 | 类型 | 必填 | 范围/约束 | 说明 |
|---|---|---|---|---|
| `data.stepAccount.totalSteps` | number (int64) | 必填 | `0 ≤ value ≤ 2^63 - 1` | 累计步数；与 §6.2 `totalSteps` 同义 + 数据库 `user_step_accounts.total_steps`（BIGINT UNSIGNED）同义；开箱不修改此字段 |
| `data.stepAccount.availableSteps` | number (int64) | 必填 | `0 ≤ value ≤ 2^63 - 1` | 可用步数（扣除 1000 之后的值）；与 §6.2 `availableSteps` 同义 + 数据库 `user_step_accounts.available_steps`（BIGINT UNSIGNED）同义；client 用此更新主界面步数余额 UI |
| `data.stepAccount.consumedSteps` | number (int64) | 必填 | `0 ≤ value ≤ 2^63 - 1` | 已消耗步数（加 1000 之后的值）；与 §6.2 `consumedSteps` 同义 + 数据库 `user_step_accounts.consumed_steps`（BIGINT UNSIGNED）同义 |

**`data.nextChest` 字段表**（开箱后立即创建的下一轮宝箱，字段集与 §7.1 完全一致）：

| 字段 | 类型 | 必填 | 范围/约束 | 说明 |
|---|---|---|---|---|
| `data.nextChest.id` | string | 必填 | BIGINT 字符串化 | 下一轮宝箱主键（与刚开的宝箱 id 不同 —— 旧记录已 DELETE） |
| `data.nextChest.status` | number (int) | 必填 | 枚举 `1` (counting) / `2` (unlockable)（与 §7.1 GET /chest/current `data.status` 同义对齐） | 下一轮状态；**计算字段，不持久化到 idempotency `response_json` 缓存；server 在响应序列化时按 §7.1 服务端逻辑同一规则实时计算** —— `(unlock_at > now) ? 1 (counting) : 2 (unlockable)`；首次成功路径下新 chest 刚 INSERT（`unlock_at = now + 10 min`）后通常 = `1`；同 key 重试 cached 路径下按"当前时刻"重新补算（若重试发生在新 chest 已到期解锁的时刻，按现实时间应返回 `2` —— 不能回放首次时刻 stale `1`，否则会与 §7.1 GET 同一秒查询结果漂移 + 与 `remainingSeconds` 互相矛盾，如 `status=1` + `remainingSeconds=0` 这种不可能组合）；**禁止**实装时写死 `1` 字面量（会与 §7.1 GET 语义漂移）/ **禁止**把首次成功时刻计算的值写入 `response_json` 缓存（r9 review 锁定，与 `remainingSeconds` 一同处理） |
| `data.nextChest.unlockAt` | string (ISO 8601 UTC) | 必填 | length = 20 | 下一轮解锁时间；server 端固定为开箱时刻 + 10 分钟 |
| `data.nextChest.openCostSteps` | number (int) | 必填 | 节点 7 阶段固定 `1000` | 下一轮开启所需步数 |
| `data.nextChest.remainingSeconds` | number (int) | 必填 | `0 ≤ value ≤ 600`（新 chest `unlock_at = now + 10 min`，故 ceil 上界 = 600） | 距 unlockAt 剩余秒数；**计算字段，不持久化到 idempotency `response_json` 缓存；server 在响应序列化时按 §7.1 服务端逻辑同一规则实时计算** —— `max(0, ceil((unlock_at - now) / 1s))`；首次成功路径下新 chest 刚 INSERT 后通常 = 600（响应序列化耗时 > 1s 可能下降为 599 / 598）；同 key 重试 cached 路径下按"当前时刻"重新补算（与 §7.1 GET 返回值保持一致，**不**回放首次时刻的 stale 值；详见服务端逻辑步骤 5j / 5b）；**禁止**实装时写死 600 字面量（会与 §7.1 GET 语义漂移）/ **禁止**把首次成功时刻计算的值写入 `response_json` 缓存（r6 review 锁定，防 stale 倒计时） |

**顶层信封字段补充说明**（与 §2 全局响应信封一致；此处单独列出是因为它们与 idempotency `response_json` 缓存有特殊关系）：

| 字段 | 类型 | 必填 | 范围/约束 | 说明 |
|---|---|---|---|---|
| `requestId` | string | 必填 | 0 ≤ length ≤ 64（参 §2 全局响应信封） | **本次**请求的 trace ID（server 端从 request 入口生成 / 透传），用于日志关联 / client-server trace 配对；**计算字段，不持久化到 idempotency `response_json` 缓存**（r7 review 锁定：缓存若包含 `requestId`，同 key 重试就会回放**首次**请求的 trace ID 给**本次**重试响应，破坏 log/trace 关联语义）；首次成功路径填本次 request `requestId`；同 key 重试 cached 路径下 server 在响应序列化时**重新填**本次重试请求的 `requestId`（与 `nextChest.remainingSeconds` 同样作为"上层动态字段"处理，详见服务端逻辑步骤 5j / 5b） |

JSON 示例：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "reward": {
      "userCosmeticItemId": "0",
      "cosmeticItemId": "24",
      "name": "星星围巾",
      "slot": 4,
      "rarity": 2,
      "assetUrl": "https://placehold.co/512x512?text=Scarf-Star",
      "iconUrl": "https://placehold.co/128x128?text=Scarf-Star"
    },
    "stepAccount": {
      "totalSteps": 12560,
      "availableSteps": 11160,
      "consumedSteps": 1400
    },
    "nextChest": {
      "id": "5002",
      "status": 1,
      "unlockAt": "2026-04-23T10:35:00Z",
      "openCostSteps": 1000,
      "remainingSeconds": 600
    }
  },
  "requestId": "req_xxx"
}
```

> **注**：示例中 `reward.userCosmeticItemId` 为 `"0"`（节点 7 阶段占位值）；节点 8 Story 23.5 落地后将更新为真实主键并由 Story 23.5 acceptance 钦定。

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截（无 Authorization 头 / token 非法 / token 过期 / token 解析失败） |
| 1002 | 参数错误 | `idempotencyKey` 缺失 / length < 1 或 > 128 / 字符集含 `[A-Za-z0-9_:-]` 之外的字符 |
| 1005 | 操作过于频繁 | **r10 review 锁定**：本接口的 rate_limit 检查在**服务端逻辑步骤 4**（handler 内层，**不**在 middleware 链上），**仅对未命中 idempotency 的"真新请求"做**（按 `user_id` 限频，每用户每分钟 > 60 次）；同 key 重试命中 `status = 'success'` / `status = 'pending'` 时**跳过步骤 4 限频**，**不**触发 1005 —— 这保证 client 用同 key 退避重试始终能读到 cached success / pending，不会被限频卡死（详见本节末「关键约束」「rate_limit 位置 r10 调整」段） |
| 1008 | 幂等冲突 | 极窄 race 兜底路径：服务端逻辑步骤 5b SELECT 到 `status = 'pending'`（首个事务对 idempotency 行的 `success` UPDATE 尚未对当前事务 snapshot 可见的瞬间）；本设计下首次事务的 `status` 推进与 commit 原子（步骤 5k → 5l），同 key 并发请求被 InnoDB unique-key X-lock 阻塞排队，首个事务结束（commit / rollback）后看到的必然是 `success` final 状态（commit 路径）或行不存在（rollback 路径 → 步骤 5a `affected_rows = 1` 走全流程）；故 1008 在正常实装下**几乎不会触发**，仅作兜底语义保留（覆盖 driver 层 read-uncommitted bug 等异常情形）。client 收到 1008 应稍后用**同**一 idempotencyKey 退避重试；server 端短路返回 1008，**不**进新业务事务。**注**：当 idempotencyKey 命中 `status = 'success'` 行时 server 直接返回 200 + 首次缓存的 `response_json`，**不**返回 1008——1008 仅表征"并发瞬态"，不表征"已完成" |
| 1009 | 服务繁忙 | DB 异常（业务事务失败 / SELECT 返回 err / INSERT 返回 err 等）/ 步数 UPDATE 乐观锁 version 不匹配导致 affected_rows = 0（并发冲突，client 可重试）/ 加权抽奖时 enabled cosmetic_items 为空（seed 未执行 —— 数据完整性异常）/ **`user_step_accounts` 行缺失**（理论上 Story 4.6 登录初始化必创建，缺失视为数据完整性异常，与"步数不足"语义区分）。**注（r7 review 锁定）**：1009 触发条件**不**包含"idempotency 行 `status = 'failed'`"—— `failed` 状态已从 ENUM 移除（参见数据库设计 §5.16 + 本节末关键约束 r7 决策段），事务 rollback 路径下同 key 重试在步骤 5a 拿 `affected_rows = 1` 走全流程而**非**返回 1009 |
| 3002 | 可用步数不足 | `user_step_accounts.available_steps < 1000`（开箱所需，行存在但余额不足）；client 收到 3002 时应展示"步数不足，请先走路赚步数"+ 引导用户走步 / 主动同步步数（Story 21.5 锚定的"开箱前主动同步步数"流程可减少此 case）。**注**：`user_step_accounts` 行**不存在**的 case **不**映射 3002，走 1009（详见上行 1009 触发条件） |
| 4001 | 当前宝箱不存在 | 用户在 user_chests 表中无任何行（理论上 Story 4.6 登录初始化必然创建首个 chest）—— 表征数据完整性异常 |
| 4002 | 宝箱尚未解锁 | 当前 chest 状态判定为 counting（数据库 status = 1 AND `unlock_at > now`）；client 不应让 4002 触发（Story 21 应在 button 可点状态校验上规避：仅 `status = 2` 时按钮可点），如果触发说明 client 端 button 状态校验有 bug 或 server / client 时钟漂移严重；client 收到 4002 时应主动重新调一次 GET /chest/current 校正本地状态 |

> **注**：本接口**不**触发 4003（4003 在 §3 表保留但本 MVP 暂未使用；如未来引入"宝箱开启条件不满足"细分业务场景如"先完成新手任务"，由对应 epic 锚定）/ **不**触发 5xxx（5xxx 是合成 / 装扮事务相关，与开箱无关）/ **不**触发 6xxx / 7xxx。

#### 关键约束

- **节点 7 vs 节点 8 阶段 `userCosmeticItemId` 占位**：
  - **节点 7 阶段（Epic 20 / Epic 21 / 节点 7 demo 验收）**：开箱奖励**不入仓**（不创建 `user_cosmetic_items` 实例，仅展示弹窗）；`reward.userCosmeticItemId` 字段**固定为字符串 `"0"`**（占位值；不下发 `null` 防 Swift Codable 解析失败 / 不下发数字 `0` 防 client 误把它当合法主键算术运算）
  - **节点 8 阶段（Epic 23 完成后）**：开箱事务由 Story 23.5 修改 → 创建 `user_cosmetic_items` 实例 → `chest_open_logs.reward_user_cosmetic_item_id` 填真实 id → API 层 `reward.userCosmeticItemId` 返回真实 BIGINT 字符串化主键
  - **client 处理规则**：iOS 端 `ChestRewardDTO` Codable struct 严格按 `String` 解析（不是 `Optional<String>`）；client UI 层**禁止**展示 `userCosmeticItemId` 字段值；client **可**在内部 log 该字段供 debug，但**不**作为业务路径分支判断（避免节点 8 升级后 dead branch）
  - **契约层升级路径**：节点 7 → 节点 8 升级**不**视为契约变更（`userCosmeticItemId` 类型 / 字段名 / 必填性都不变，只是 server 端语义从"占位"变成"真实主键"）；Story 23.5 落地时**应**在本 §7.2.6 中标注升级日期 + commit hash

- **idempotencyKey 字符集与长度**：`[A-Za-z0-9_:-]` + length 1-128；选用此字符集是为兼容主流 client 端 UUID / nanoid / ULID 等格式 + 允许 ":" 作为业务前缀分隔符（如 `"chest_open:user_1001:1714281600000"`）；**禁止**空字符串 `""`（character set 已隐含 length ≥ 1）；**禁止**包含 `/` / `?` / `&` 等 URL 保留字符（DB 列 VARCHAR(128) 不会因 URL 编码异常，但 client 实装时如果 idempotencyKey 串入了任何 URL / SQL / shell 上下文可能引发注入风险，故契约层在 character set 层面收紧）

- **client 重试策略**（r5 review 锁定 / r6 review 修订 / r7 review 简化；**取代** r4 的"60s sentinel + 60s 外换新 key + UI 提示"分段方案；DB 同事务幂等 + InnoDB 锁阻塞排队让所有 client 重试路径**始终安全**）：
  - **网络层重试**（如 client 收到 5xx / timeout）→ 用**同** idempotencyKey 重试 + 指数退避（**r10 review 锁定**：同 key 重试在步骤 3 幂等命中预检命中 `status = 'success'` / `status = 'pending'` 时**不**计入 rate_limit 配额，因此**同 key 重试本身不会触发 1005**；client 仍应做指数退避以缓解 server / DB 压力，但**无需**担心"重试 60 次后撞限频读不到 cached success"的卡死路径 —— 该路径已被 r10 review 移除）；server 端处理路径：
    - 首个事务**仍在执行中** → 重试请求在步骤 3 幂等命中预检读到 `status = 'pending'`（极窄 race 窗口）→ 跳过步骤 4 限频 + 跳过步骤 5 事务 → 返回 **1008**（client 应继续同 key 退避重试，直到首个事务 commit / rollback）；或 race 窗口已过去 → 步骤 3 预检读到 `status = 'success'`（commit 路径）/ 未命中（rollback 路径） → 走对应分支
    - 行已 `status = 'success'` → 步骤 3 预检命中 → **跳过步骤 4 限频** → 返回 200 + cached response（与首次结果一致；`nextChest.status` + `nextChest.remainingSeconds` 均按当前时刻实时**同源同时刻**补算 + `requestId` 填本次重试请求的 trace ID，不复用首次时刻的 stale 值与 stale trace ID —— 尤其重试发生在新 chest 已到期解锁的时刻会自然返回 `status = 2` + `remainingSeconds = 0`，与 §7.1 GET 同一秒查询完全对齐）
    - 行**不存在**（首次事务已 rollback，pending 行自动消失）→ 步骤 3 预检未命中 → **走步骤 4 限频检查（本次计入配额，因事务已 rollback 是"真新请求"等价首次到达）** → 步骤 5a `affected_rows = 1` → 走全流程
  - **业务错误重试**（如收到 3002 / 4002）→ server 端业务事务 rollback 时 idempotency 行也跟着回滚 → 同 key 重试在步骤 5a `affected_rows = 1` 自然走全流程（与首次到达等价）。client **可**选用同 key 或新 key 重试；推荐**新** idempotencyKey 重试（语义更清晰，避免与"网络重试"语义混淆；DB 同事务幂等下两者都安全）
  - **1008 重试**（极窄 race 兜底路径：步骤 5b SELECT 读到 `status = 'pending'`；正常实装下几乎不会触发）→ client **必须**用**同** idempotencyKey + 指数退避重试（建议 2s / 4s / 8s，累计 ≤ 30s）。**没有 60s 边界**（r4 的 60s 边界来源于 Redis sentinel TTL，DB 持久化幂等已无此约束）；下一次重试通常会进入 InnoDB 锁等待或直接命中 `success` final 状态。**禁止**换新 key（理由：首次事务可能即将 commit；换新 key 会绕过 idempotency 直接进新事务 → 重复扣 1000 步 + 重复出箱）
  - **1009 重试**（DB 异常 / 数据完整性异常 / 步数乐观锁冲突 / server 5xx）→ client **可**用**同** idempotencyKey 或**新** idempotencyKey 退避重试（首次事务已回滚，pending 行已消失，两者都安全；同 key 路径在 5a `affected_rows = 1`，新 key 路径走新事务）；UI 无需"奖励可能已发放，请联系客服"等不确定文案（r4 的 60s 外重复出箱风险已通过 DB 同事务幂等彻底消除；r7 移除 best-effort failed upsert 后**不**会再出现"同 key 看到 failed 必须换 key"的语义负担）
  - **client 实装伪代码**（Story 20.6 / Epic 21 实装锚定）：
    ```
    key = generateIdempotencyKey()
    retries = 0
    loop:
      resp = POST /chest/open {idempotencyKey: key}
      if resp.code == 0: return resp.data  // 成功
      if resp.code == 1008:
        // 命中 pending：首次请求执行中；同 key 退避重试
        sleep(min(2s * 2^retries, 8s))
        retries += 1
        if retries >= 4: showUserToast("网络繁忙，请稍后再试"); return error
        continue
      if resp.code == 1009 && retries < 2:
        // DB 异常 / 并发冲突 / server 5xx；首次事务已 rollback、pending 行已消失，同 key 重试安全（也可选新 key，两者等价）
        sleep(min(1s * 2^retries, 4s))
        retries += 1
        continue
      return resp.code  // 其他错误码按对应分支处理（3002 / 4001 / 4002 等）
    ```
  - **重复点击防抖**：client UI 层应在用户点击"开箱"按钮后**立即禁用**按钮（loading 状态），直到收到 server 响应再启用；服务端层面 idempotencyKey + DB UNIQUE 原子声明提供二次防护

- **事务边界**（r6 review 锁定，**修订** r5 的"事务外预声明"边界设计；r7 review 进一步简化为二态机）：单一 MySQL 事务覆盖步骤 4 全部子步骤 —— 步骤 5a `INSERT ... ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)` 借 `UNIQUE(user_id, idempotency_key)` 在事务内首条语句做原子预声明 + 步骤 5c~5i 业务表写入（FOR UPDATE 锁 chest + step_account + UPDATE + INSERT 日志 + DELETE + INSERT chest）+ 步骤 5k `UPDATE chest_open_idempotency_records SET status='success', response_json=?` 与业务表写入**同一事务原子提交**，构成"幂等记录 + 业务数据"单一可信源。这种顺序保证：(a) 同 key 并发请求通过 InnoDB unique-key X-lock 阻塞排队，**只有首个事务持锁推进**；首个事务结束后其他事务再分支 —— 首个 commit → 其他事务 INSERT `affected_rows = 0` → SELECT 命中 `success` 短路；首个 rollback → 其他事务 INSERT `affected_rows = 1` → 走全流程（**不**靠 MySQL 业务表行锁兜底，FOR UPDATE 在 5c 才生效，已晚于幂等声明）；(b) 业务事务失败 → idempotency 行（包括步骤 5a INSERT 的 `pending`）也回滚 → 同 key 重试在步骤 5a 拿 `affected_rows = 1` 走全流程，**等价于首次到达**，无副作用、无残留状态、**无 pending 卡死**、**无 failed 占位行 race**；(c) 业务事务成功 → idempotency `status = 'success'` + `response_json` 已**原子**落盘 → 同 key 重试**确定性**命中 cached response，**不会**重复出箱

- **r7 移除 best-effort failed upsert 决策**（r7 review 锁定，**修订** r6 设计中保留的"可选 post-rollback failed 占位写入"）：
  - **背景 / 被反驳的 r6 设计**：r6 在「事务后处理」步骤 6 留了"server 端**可**在事务 rollback 后新开独立短事务写 `INSERT ... ON DUPLICATE KEY UPDATE status='failed'` 把同 key 锁定为 failed，让 client 立即明确'首次失败 → 换新 key 重试'"作为 UX 优化。
  - **r7 review 锁定的 race condition**：rollback 后 best-effort failed upsert 是**异步**的；与此同时 client 立即用**同**一 idempotencyKey 重试 → 新事务在步骤 5a INSERT pending（rollback 已让上一轮 pending 消失 → `affected_rows = 1`）→ 业务成功 → 步骤 5k UPDATE status='success' → COMMIT。**此时** server A 的 best-effort 异步 compensation 写入 → 因 UNIQUE 冲突触发 `ON DUPLICATE KEY UPDATE status='failed'` → 把第二次请求已 commit 的 `success` 行**错误覆盖**为 `failed`。后果：(a) 后续同 key 重试看到 `status='failed'` 短路返回 1009，但**业务事务实际已 commit success**（步数已扣 / chest 已发 / nextChest 已 INSERT）→ client UX 与数据状态**严重不一致**；(b) `chest_open_logs` 已落盘但 idempotency 表标记 failed → 数据审计错位
  - **r7 决策**：彻底**移除** best-effort post-rollback failed upsert。理由：(a) 事务 rollback 后 pending 行自动消失（事务原子性已保证 rollback 路径无副作用），同 key 重试在步骤 5a `affected_rows = 1` 自然走全流程 = 与首次到达等价 = **不需要**写 failed 行；(b) `failed` 状态的唯一意义本是表示"首次确认失败 + client 应换 key"，但在事务原子性保证 rollback 路径"无副作用"的前提下，client 同 key 重试**已经是安全的**，UX 无需通过 failed 状态强制换 key；(c) 移除该补偿同时彻底消除上述 race condition，无任何安全性损失
  - **结果 — schema 简化为二态机**：`chest_open_idempotency_records.status` ENUM 从 `('pending', 'success', 'failed')` **简化为** `('pending', 'success')`（数据库设计 §5.16 同步更新）；§7.2 服务端逻辑步骤 5b 不再有 "status = 'failed'" 分支；§7.2 错误码表 1009 触发条件不再包含 "idempotency 行 status='failed'"；§7.2 client 重试策略不再有 "1009 必须换新 key" 限制（同 key / 新 key 重试均等价安全）
  - **节点 11 / Epic 32 复用启示**：若 `POST /api/v1/compose/upgrade` 复用同一持久化模式，由 Story 32.4 锚定其 idempotency 表时**应**直接采用二态机（`pending` / `success`），**禁止**引入异步 failed 补偿写

- **TTL 与 Redis 决策 vs round 3 / round 4 / round 5 / round 6**（r7 修订）：
  - **round 3**：Redis sentinel TTL = 24h → SET / DEL 失败时 client 卡死 24h，破坏幂等承诺（round 4 已纠正）
  - **round 4**：Redis sentinel TTL = 60s + final-response TTL = 24h → SET-fail-after-commit 时 60s 外 client 换新 key 仍可能重复出箱（Redis 是非事务存储，client 无法区分"事务已 commit"vs"事务已 rollback"两种 60s 残留 case；round 5 review 锁定的核心缺陷）
  - **round 5**：DB `chest_open_idempotency_records` 表 + UPDATE 与业务事务同事务原子写；**但预声明 INSERT 仍在事务外独立执行** → 业务事务 rollback 时 pending 行**不**跟着回滚 → 同 key 重试永久卡 1008（r6 review 锁定的悖论）
  - **round 6**：把预声明 INSERT 也纳入同一业务事务（事务内首条语句）。同 key 并发依赖 InnoDB unique-key X-lock 阻塞排队 + rollback 同事务回滚 pending 行 → 同 key 重试**等价于首次到达**，彻底消除 pending 卡死。**移除** Redis 在 chest_open idempotency 路径上的所有角色；同时 `response_json` 缓存**不**包含 `nextChest.remainingSeconds` 等时间派生字段，回放时实时补算，**消除 stale 倒计时**与 §7.1 GET 的语义漂移。**但仍留** best-effort post-rollback failed upsert 作为可选 UX 优化（r7 review 锁定该补偿与 client 同 key 重试的 success 路径存在 race）
  - **round 7（本轮锁定）**：彻底**移除** best-effort post-rollback failed upsert（race condition 详见上方 r7 决策段）；schema 简化为 `status ENUM('pending', 'success')` 二态机；`response_json` 缓存**不**包含 `requestId`（顶层 trace ID，重试请求需重新填**当前**请求的 trace ID，不回放 stale 首次值）；§7.2 错误码 1009 不再包含 "idempotency 行 status='failed'" 触发条件；client 1009 重试策略简化为"同 key / 新 key 均安全"。接口对外承诺"同 key 至多产生一次副作用"完全由 DB 原子性 + InnoDB 锁阻塞兜底

- **加权抽奖语义**：`drop_weight` 是抽奖权重（值越大越容易抽到），server 端按 `cumulative_weight / total_weight` 比例抽取；同 `drop_weight` 值的 cosmetic 抽到概率相等；`drop_weight = 0` 的 cosmetic（不应在 enabled 集合中）等价于不可抽到；加权抽取**仅**对 `is_enabled = 1` 行有效（disabled 行不参与抽奖）

- **跨字段同义对齐**：
  - `data.stepAccount.{totalSteps, availableSteps, consumedSteps}` 与 §6.2 GET /steps/account 同义字段对齐（字段名 / 类型 / 语义完全一致）
  - `data.nextChest.{id, status, unlockAt, openCostSteps, remainingSeconds}` 与 §7.1 GET /chest/current 同义字段对齐
  - `data.reward.{cosmeticItemId, name, slot, rarity, assetUrl, iconUrl}` 与 §8.1 GET /cosmetics/catalog 同义字段对齐（节点 8 Story 23.1 锚定 §8.1 时应保证字段名 / 类型一致）
  - `data.reward.userCosmeticItemId` 节点 7 占位 `"0"` / 节点 8 起与 §8.2 GET /cosmetics/inventory 中 `items[].userCosmeticItemId` 字段同义对齐

- **BIGINT 主键字符串化**：`reward.userCosmeticItemId` / `reward.cosmeticItemId` / `nextChest.id` 均遵循 §2.5 字符串化（即使 `userCosmeticItemId = "0"` 也是字符串）

---

## 8. 装扮与背包接口

## 8.1 获取装扮配置目录

### `GET /api/v1/cosmetics/catalog`

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [
      {
        "cosmeticItemId": "12",
        "code": "hat_yellow_01",
        "name": "小黄帽",
        "slot": 1,
        "rarity": 1,
        "iconUrl": "https://...",
        "assetUrl": "https://..."
      }
    ]
  },
  "requestId": "req_xxx"
}
```

---

## 8.2 获取背包

### `GET /api/v1/cosmetics/inventory`

返回“聚合展示 + 实例列表”。

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "groups": [
      {
        "cosmeticItemId": "12",
        "name": "小黄帽",
        "slot": 1,
        "rarity": 1,
        "iconUrl": "https://...",
        "assetUrl": "https://...",
        "count": 3,
        "instances": [
          {
            "userCosmeticItemId": "90001",
            "status": 1
          },
          {
            "userCosmeticItemId": "90005",
            "status": 1
          },
          {
            "userCosmeticItemId": "90008",
            "status": 2
          }
        ]
      }
    ]
  },
  "requestId": "req_xxx"
}
```

#### 实例状态

- `1 = in_bag`
- `2 = equipped`
- `3 = consumed`

---

## 8.3 穿戴装扮

### `POST /api/v1/cosmetics/equip`

#### 请求体

```json
{
  "petId": "2001",
  "userCosmeticItemId": "90001"
}
```

#### 服务端逻辑

- 校验实例属于当前用户
- 校验实例当前可装备
- 查询配置槽位
- 若槽位已有装备，则先卸下旧装备
- 绑定到宠物对应槽位
- 更新实例状态为 equipped

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "petId": "2001",
    "equipped": {
      "slot": 1,
      "userCosmeticItemId": "90001",
      "cosmeticItemId": "12",
      "name": "小黄帽"
    }
  },
  "requestId": "req_xxx"
}
```

---

## 8.4 卸下装扮

### `POST /api/v1/cosmetics/unequip`

#### 请求体

```json
{
  "petId": "2001",
  "slot": 1
}
```

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "petId": "2001",
    "slot": 1,
    "unequipped": true
  },
  "requestId": "req_xxx"
}
```

---

## 9. 合成接口

当前规则：

- 玩家手动选择要消耗的道具实例
- 必须正好 10 个
- 必须同品质
- 不要求相同部位
- 不要求相同配置 id

## 9.1 获取合成概览

### `GET /api/v1/compose/overview`

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "rarities": [
      {
        "rarity": 1,
        "availableCount": 24,
        "canCompose": true
      },
      {
        "rarity": 2,
        "availableCount": 8,
        "canCompose": false
      },
      {
        "rarity": 3,
        "availableCount": 12,
        "canCompose": true
      },
      {
        "rarity": 4,
        "availableCount": 2,
        "canCompose": false
      }
    ]
  },
  "requestId": "req_xxx"
}
```

---

## 9.2 合成升级

### `POST /api/v1/compose/upgrade`

#### 请求体

```json
{
  "fromRarity": 1,
  "userCosmeticItemIds": [
    "90001",
    "90002",
    "90008",
    "90110",
    "90111",
    "90120",
    "90201",
    "90202",
    "90333",
    "90340"
  ],
  "idempotencyKey": "compose_20260423_001"
}
```

#### 服务端校验规则

- `userCosmeticItemIds` 长度必须为 10
- 不能有重复 id
- 这 10 个实例必须都属于当前用户
- 这 10 个实例必须都是 `in_bag`
- 这 10 个实例对应配置的品质必须都等于 `fromRarity`
- `fromRarity` 必须可升级：
  - `1 -> 2`
  - `2 -> 3`
  - `3 -> 4`
  - `4` 不允许

#### 服务端执行逻辑

- 锁定这 10 个实例
- 将 10 个实例更新为 consumed
- 从高一阶品质装扮池中随机抽取 1 个配置
- 创建 1 个新的装扮实例
- 写合成日志与材料日志

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "fromRarity": 1,
    "toRarity": 2,
    "consumedItemIds": [
      "90001",
      "90002",
      "90008",
      "90110",
      "90111",
      "90120",
      "90201",
      "90202",
      "90333",
      "90340"
    ],
    "reward": {
      "userCosmeticItemId": "99001",
      "cosmeticItemId": "61",
      "name": "月光围巾",
      "slot": 4,
      "rarity": 2,
      "assetUrl": "https://..."
    }
  },
  "requestId": "req_xxx"
}
```

---

## 10. 房间接口

## 10.1 创建房间

### `POST /api/v1/rooms`

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | POST |
| Path | /api/v1/rooms |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | 默认（按 Story 4.5 rate_limit 默认值 60 次/分；房间创建是低频操作，无需特殊限频） |
| 幂等 | **非幂等**（每次调用都创建一个新房间；用户已在房间中再调将返回 6003）；**不**接受 `idempotencyKey` |
| 节点 | 节点 4（Epic 11 落地，Epic 12 客户端集成） |

#### 请求体

空对象（`{}`）—— 当前用户身份从 `Authorization: Bearer <token>` 解析，房间属性（`max_members=4` / `status=1`）由 server 端写死，client 不传任何业务字段。

JSON 示例：

```json
{}
```

> 注：未来若引入"自定义房间名 / 私有房间标识"等扩展字段，将在新 epic 的 §X.1 story 锚定（不在本节点 4 范围内）。

#### 服务端逻辑

事务内严格按顺序执行（详见 Story 11.3 实装）：

1. 查 `users.current_room_id`，**非 null** → 立即返回 **6003 用户已在房间中**（不开事务）
2. 插入 `rooms`（`creator_user_id = 当前 user`, `status = 1`, `max_members = 4`），拿到 `room_id`
3. 插入 `room_members`（`room_id`, `user_id = 当前 user`）；如果遇到 DB UNIQUE(`user_id`) 兜底冲突（理论不会，因为步骤 1 已查过 `users.current_room_id`，但同一用户并发两次 `POST /rooms` 时可能发生 —— 两请求都通过步骤 1，赢家先插 `room_members`，输家在步骤 3 撞 UNIQUE）→ 回滚 + 返回 **6003**（与 §10.4 join 接口步骤 5 兜底语义对齐）
4. 更新 `users.current_room_id = room_id`
5. 提交事务
6. 返回 `data.room` 见下文响应体字段表

**事务边界规则**：步骤 2 ~ 4 必须在同一 MySQL 事务中（参见数据库设计 §8.6 房间事务边界）；步骤 3 撞 `room_members.UNIQUE(user_id)` 时回滚并返回 **6003**（不是 1009 —— 该路径是"用户已在某房间"的正常 race，不是服务异常，client 仅识别 6003 即可统一处理）；其他事务失败（DB 异常 / 内部 panic）→ 全部回滚 → 返回 1009 服务繁忙。

**WS 广播**：本接口**不**触发 `member.joined` 广播 —— 房间刚创建、当前用户是房间内**唯一**成员，无其他在线成员需要被通知；`member.joined` 仅在 `POST /rooms/{roomId}/join` 成功后由 Story 11.8 触发（详见 §12.3 `### 成员加入` 触发条件段）。

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `data.room.id` | string | 必填 | 房间主键（BIGINT 字符串化下发，遵循 §2.5 全局约定） |
| `data.room.creatorUserId` | string | 必填 | 创建者 user 主键（BIGINT 字符串化）；来自 `rooms.creator_user_id` |
| `data.room.maxMembers` | number (int) | 必填 | 房间容量上限；节点 4 阶段固定 `4`（来源数据库设计 §5.13 `max_members TINYINT UNSIGNED NOT NULL DEFAULT 4`） |
| `data.room.memberCount` | number (int) | 必填 | 当前成员数；本接口创建后必为 `1`（自动加入了创建者自己） |
| `data.room.status` | number (int) | 必填 | 房间状态枚举：`1 = active` / `2 = closed`（来源数据库设计 §6.12 `rooms.status`）；本接口创建后必为 `1` |

JSON 示例：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "room": {
      "id": "3001",
      "creatorUserId": "1001",
      "maxMembers": 4,
      "memberCount": 1,
      "status": 1
    }
  },
  "requestId": "req_xxx"
}
```

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截（无 Authorization 头 / token 非法 / token 过期） |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截（同 IP 每分钟 > 60 次） |
| 6003 | 用户已在房间中 | 服务端逻辑步骤 1（预检）或步骤 3（DB UNIQUE 兜底）：用户已在任意房间（含并发 race 场景） |
| 1009 | 服务繁忙 | 事务回滚 / DB 异常 / 内部 panic（**不**含 `room_members.UNIQUE(user_id)` 撞冲突 —— 那条走 6003，详见 Story 11.3 实装） |

> **注**：本接口**不**会触发 6001 / 6002 / 6004 / 6005 —— 房间在事务内才被创建，不存在"找不到 / 已满 / 状态异常"场景；6004（用户不在房间中）仅出现在 `GET /rooms/{roomId}`（§10.3，ACL fail：caller 不是该房间成员）和 `POST /rooms/{roomId}/leave`（§10.5，重复 leave / 不在该房间）两条接口。

**关键约束**：

- 字段表中所有 BIGINT 主键 / 外键（`id` / `creatorUserId`）严格按 §2.5 全局约定字符串化下发
- `memberCount` 与 `maxMembers` 类型为 `number (int)`，**不**字符串化（数值字段不受 §2.5 BIGINT 字符串化规则约束 —— `memberCount` ≤ `maxMembers` ≤ 4，远低于 `Number.MAX_SAFE_INTEGER`，client `Int` / Swift `Int` 解析无精度风险）
- 6003 在两条路径下触发（步骤 1 预检 + 步骤 3 DB UNIQUE 兜底），但 client **不**应区分这两种场景（都是"用户已在某房间"，UX 处理一致）；这与 §10.4 join 接口的双路径 6003 语义对称
- 6003 预检路径不消耗事务资源；6003 兜底路径（步骤 3 撞 UNIQUE）会回滚已开事务，但**不**降级为 1009 —— race 是正常并发，不是服务异常

---

## 10.2 获取当前所在房间

### `GET /api/v1/rooms/current`

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | GET |
| Path | /api/v1/rooms/current |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | 默认（按 Story 4.5 rate_limit 默认值 60 次/分） |
| 幂等 | 幂等（GET，无副作用） |
| 节点 | 节点 4（Epic 11 落地，Epic 12 客户端集成） |

#### 请求

无请求体（GET 方法）；当前用户身份从 `Authorization: Bearer <token>` 解析。

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `data.roomId` | string \| null | 必填（值可为 null） | 当前用户所在房间主键（BIGINT 字符串化），即 `users.current_room_id` 当前值；用户不在任何房间时为 `null`（**显式** null，**不**省略 key） |

JSON 示例（用户当前在房间）：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "roomId": "3001"
  },
  "requestId": "req_xxx"
}
```

JSON 示例（用户当前不在任何房间）：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "roomId": null
  },
  "requestId": "req_xxx"
}
```

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截 |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截 |
| 1009 | 服务繁忙 | DB 异常 / 内部 panic |

> **注**：本接口**不**会触发 6001 ~ 6005 —— 用户不在房间是合法场景（返回 `roomId: null` + code = 0），不视为业务错误。

**关键约束**：

- 与 `GET /me.user.currentRoomId` **不**等价 —— `GET /me` 中的 `currentRoomId` 是**永久 schema 占位**始终返回 `null`（详见 §4.3 Future Fields 引用块），本接口才是获取真实房间归属的权威路径
- 与 `GET /home.data.room.currentRoomId` 字段语义**等价**（两者都查 `users.current_room_id`），但 `GET /home` 是首页聚合接口（含 user / pet / stepAccount / chest / room 五段），本接口是单字段轻量查询；client 在房间页内用本接口、在首页加载用 `GET /home`（节点 4 由 Story 11.10 真实实装 `room.currentRoomId`）
- 客户端 WS 重连时**不**应优先用本接口拿 roomId（按 §12.1 客户端 roomId 来源规则：`本地持久化最近一次 WS roomId` 优先；本接口可作为冷启场景兜底，但不是首选）

---

## 10.3 获取房间详情

### `GET /api/v1/rooms/{roomId}`

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | GET |
| Path | /api/v1/rooms/{roomId} |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | 默认（按 Story 4.5 rate_limit 默认值 60 次/分） |
| 幂等 | 幂等（GET，无副作用） |
| 节点 | 节点 4（Epic 11 落地，Epic 12 客户端集成） |

#### 请求

| 参数位置 | 字段 | 类型 | 必填 | 长度/范围约束 | 说明 |
|---|---|---|---|---|---|
| Path | `roomId` | string | 必填 | 必须是 BIGINT 数字字符串（如 `"3001"`），1 ≤ length ≤ 20 字符 | 目标房间主键；server 内部转 `uint64` |

> **注（ACL — r7 锁定）**：本接口**强制要求** caller 是该房间的当前成员（即 `users.current_room_id == path roomId`），否则返回 **6004 用户不在房间中**（HTTP 200，业务错误码层；含 `current_room_id == NULL` / `current_room_id != path roomId` 两种场景）。**rationale**：响应体 `members[].nickname` / `avatarUrl` / `pet.petId` / `pet.currentState` 是其他用户的隐私字段（注：`pet` 整体可空 nullable，详见字段表 `data.members[].pet` 行；空时不下发 `pet.*` 子字段），且 `rooms.id` 是 BIGINT auto_increment 顺序号（数据库设计.md §6.6），任何认证用户都能查任意 roomId 会形成"枚举 roomId → 抓全站房间成员关系"的隐私 / security 攻击面。本接口的访问控制模型与 §10.5 leave 接口步骤 1 一致（caller 必须是该房间成员），统一"接口默认 deny + 显式 allow（白名单 ACL）"基线。**未来路径**：节点 4 之后若产品需要"加入前预览房间"语义（如分享链接），由对应 epic 单开 story 设计"轻量预览接口"（仅返回 `room.id` / `status` / `memberCount` 不含成员隐私字段）或"高熵 roomId 改造"（rooms.id 从 BIGINT auto_increment 改 nanoid），**不**回退本 ACL。

#### 服务端逻辑

**事务边界**：开 MySQL 事务（隔离级别 = **REPEATABLE READ**，MySQL InnoDB 默认级别即是；事务内所有 SELECT 共享同一 snapshot）包以下全部 4 个步骤。**snapshot 隔离 + FOR SHARE 行锁两个机制是互补的，缺一不可**：

- **snapshot 隔离**保证步骤 1 + 步骤 3 看到同一时刻的状态（**事务内部一致性**），即两次 SELECT 在 snapshot 这一维度同步；
- **FOR SHARE 行锁**（步骤 1）保证 caller **在本读事务持续期间（含步骤 3 SELECT roster）仍是房间成员**（**外部一致性**：阻止并发 `POST /rooms/{roomId}/leave` 在本读事务 commit 前提交其 DELETE）—— snapshot 仅锁定"读到的状态"，**不**阻止其他 tx 在期间提交对相同行的写入；只有显式行锁才能让并发 leave 的 DELETE 必须等本读事务提交后才能 commit。FOR SHARE 锁随事务 commit 释放，因此该外部一致性保证的精确边界是**事务持续期间（含 SELECT roster 步骤；commit 时 lock 释放）**，而**不**是"HTTP 响应字节发出时"。

**rationale**：r8 用 REPEATABLE READ snapshot 包步骤 1 + 步骤 3 的 fix **不充分** —— 具体 race：(t0) 本 GET 事务开始，snapshot 锁定在 t0 状态（caller 是成员）→ (t1) 并发 `POST /rooms/{roomId}/leave` 提交 DELETE + UPDATE → (t2) 本 GET 步骤 3 用 t0 snapshot SELECT，看到 caller 仍是成员，照返完整 roster → (t3) HTTP 响应发出，但 caller 已离开，roster 仍泄漏给已离开用户。snapshot 隔离仅保证两次读看到同一时刻的状态（**内部一致性**），**不**保证 "caller 在事务持续期间仍是成员"（**外部一致性**）。须在步骤 1 显式加 FOR SHARE 共享锁锁定 caller 自己的 `room_members` 行，让并发 leave 的 DELETE 必须等本读事务 commit / rollback 才能继续 —— 这才是真正阻止 post-leave 数据泄漏的 lock。

**关于 commit-after-response 残留窗口（r12 锁定）**：FOR SHARE 锁在事务 commit 时释放，而 handler 通常的执行顺序是 `commit tx → 序列化 JSON → ctx.WriteJSON / flush response body`，因此存在一个 **commit → flush** 之间的窗口（μs 量级）：在该窗口内并发 leave 可能 commit 其 DELETE，从而 caller 的 roster 字节流是在 caller 已离开后才到达对端 —— 这是**协议层面**承认的 best-effort 残留 race，**不**强制要求 server 在 commit 前先序列化 + WriteJSON 后再 commit（该模式实装代价大，需重构 handler 流程，与 ROI 不匹配）。**责任划分**：本协议层面只保证"事务持续期间 ACL 成立"；client 解析层应**防御性接受**"我刚 leave 但仍收到一份 stale roster"的极端 race（窗口宽度 ≤ 一次 DB commit + 内核 socket 写出耗时，量级 μs ~ 单 ms）—— 收到时直接 discard / 不写入 RoomView state（client 已知道自己 leave 结果）。该窗口不可在协议层完全消除，但可通过"client 防御性 discard"达成端到端实质安全。

按顺序执行（**全部在同一 REPEATABLE READ 事务内**）：

1. 查 `users.current_room_id`，**与请求的 `roomId` 不一致**（含 `current_room_id` 为 null）→ 立即返回 **6004 用户不在房间中**（caller 不是该房间成员，禁止查看其他成员隐私字段）；**通过后追加** `SELECT 1 FROM room_members WHERE room_id = ? AND user_id = caller FOR SHARE`：取 **共享锁** 锁定 caller 自己的成员行，确保本读事务持续期间该行不会被并发 leave 的 DELETE 提交（DELETE 需要排他锁，与 FOR SHARE 互斥，必须等本读事务 commit 后才能继续）；该 SELECT 命中 0 行 → 视同步骤 1 ACL 失败，返回 **6004**（兜底语义；正常情况下步骤 1 前半段 `users.current_room_id` 已通过即意味着该行存在，本步骤是 race 兜底）
2. 查 `rooms WHERE id = ?`，**找不到** → 返回 **6001 房间不存在**（理论上步骤 1 已通过意味着 caller 在该房间，rooms 行必存在；本步骤是兜底）
3. 查 `room_members WHERE room_id = ?` + INNER JOIN `users` + **LEFT JOIN `pets`** 聚合（按 `room_members.joined_at ASC` 稳定排序）—— **必须用 LEFT JOIN `pets`**（不能用 INNER JOIN）：pet-less 账号（用户无活跃 pet 行，§5.1 / Story 4.8 已将其作为 contract 内合法 edge case 覆盖）若用 INNER JOIN 会被静默丢行 → 违反 `memberCount === members.length` 不变量；LEFT JOIN 时 `pets.*` 列为 NULL，service 实装层据此把 `data.members[].pet` 整体下发为 `null`（详见字段表 `data.members[].pet` 行 + JSON 示例 `userId: "1003"` 边界案例）
4. 提交事务并返回 `data.{room, members}`

**注**：虽然本接口只读、不修改任何行，但事务的 snapshot 隔离 + FOR SHARE 行锁双机制对 ACL 边界至关重要 —— 不开事务等于把 ACL race 的风险转嫁给"client 自洽不变量"，而 ACL 是隐私边界、不能由 client 兜底。"`memberCount === members[].length`"等不变量仍然成立（步骤 3 内部一次 SELECT 自然原子），但**跨步骤的 ACL 一致性由 snapshot + FOR SHARE 共同提供**。本读事务在数据库设计 §8.8 中归类为"读快照事务（含 ACL 共享锁）"，与写事务（§8.1 / §8.6 / §8.7）并列。

**关于 FOR SHARE 是否会 deadlock**：caller 锁的是 `room_id = ? AND user_id = caller` 这一**特定行**（同一 caller 不会并发自我互锁）；并发 leave 也只锁 `user_id = caller` 这一行（取排他锁后 DELETE）；其他成员 / 其他房间走的是不同 `(room_id, user_id)` 锁路径，不存在锁顺序循环 → 无 deadlock 风险。最坏情况是并发 leave 等本读事务 commit 后再提交 DELETE，等待时长 = 本读事务全步骤时长（μs ~ 数 ms 量级），可忽略。

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `data.room.id` | string | 必填 | 房间主键（BIGINT 字符串化） |
| `data.room.creatorUserId` | string | 必填 | 创建者 user 主键（BIGINT 字符串化） |
| `data.room.maxMembers` | number (int) | 必填 | 房间容量上限（节点 4 阶段固定 `4`） |
| `data.room.memberCount` | number (int) | 必填 | 当前成员总数；**必须严格等于** `data.members[]` 数组长度（不变量，详见本节末"不变量"小节） |
| `data.room.status` | number (int) | 必填 | 房间状态枚举：`1 = active` / `2 = closed` |
| `data.members` | array | 必填 | 房间全成员列表 = `room_members WHERE room_id = ?` 全部行 + JOIN `users` / `pets` 聚合；**不做"WS 此刻是否连接"层的过滤**（即 service 实装层**禁止**只返回 SessionManager 当前在线的 user）—— roster 反映的是 server 端"仍在房间"的状态（详见本节末"roster 语义与 WS 断线交互"小节）；节点 4 阶段**不**下发"online / offline"区分字段，client 不感知具体连接态 |
| `data.members[].userId` | string | 必填 | 成员 user 主键（BIGINT 字符串化） |
| `data.members[].nickname` | string | 必填 | 成员昵称；来自 `users.nickname`（节点 2 阶段首次创建时 server 写入 `"用户{id}"`，可被未来"修改昵称"功能覆盖） |
| `data.members[].avatarUrl` | string | 必填 | 成员头像 URL；首次创建为空字符串 `""`（不为 null）；空字符串语义 = "暂无头像"，client 渲染时降级为占位头像 |
| `data.members[].pet` | object \| null | 必填（nullable） | 成员当前宠物容器；与 §5.1 GET /home `data.pet` 同语义，**pet-less 账号**（用户**无活跃 pet**，理论不该发生但 §5.1 / Story 4.8 已将其作为 contract 内合法 edge case 覆盖）时下发 `null`；客户端必须按可空对象解析（iOS `Optional<MemberPetDTO>` / Go `*MemberPetDTO`），不得假设 `pet` 永远非空。下方 `data.members[].pet.*` 子字段**仅当 `pet ≠ null` 时存在**；client 渲染时 `pet == null` 应降级为"无宠物"占位（不渲染 cat sprite），**不**视为契约违反 |
| `data.members[].pet.petId` | string | 必填（仅当 `pet ≠ null`） | 成员当前宠物主键（BIGINT 字符串化）；来自 `pets.id`（每个 user 节点 2 阶段唯一 1 只默认猫，详见 Story 4.6 首次初始化事务）；当 `pet ≠ null` 时**必非空字符串** |
| `data.members[].pet.currentState` | number (int) | 必填（仅当 `pet ≠ null`） | 宠物当前状态枚举：`1 = rest` / `2 = walk` / `3 = run`（来源数据库设计 §6.4 `pets.current_state`）；**节点 4 阶段固定返回 `1`**（Epic 14 才真实驱动 motion_state） |
| `data.members[].pet.equips` | array | 必填（仅当 `pet ≠ null`） | 成员当前装备数组；**节点 4 阶段固定返回 `[]`**（Future Fields，详见本节末"五阶段过渡表"） |

JSON 示例（节点 4 阶段，3 成员房间，含 1 个 pet-less 边界案例）：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "room": {
      "id": "3001",
      "creatorUserId": "1001",
      "maxMembers": 4,
      "memberCount": 3,
      "status": 1
    },
    "members": [
      {
        "userId": "1001",
        "nickname": "用户1001",
        "avatarUrl": "",
        "pet": {
          "petId": "2001",
          "currentState": 1,
          "equips": []
        }
      },
      {
        "userId": "1002",
        "nickname": "用户1002",
        "avatarUrl": "",
        "pet": {
          "petId": "2002",
          "currentState": 1,
          "equips": []
        }
      },
      {
        "userId": "1003",
        "nickname": "用户1003",
        "avatarUrl": "",
        "pet": null
      }
    ]
  },
  "requestId": "req_xxx"
}
```

> **示例说明**：`userId: "1003"` 是 pet-less 账号（`pet: null`） —— 与 §5.1 GET /home `data.pet = null` 同语义（Story 4.8 edge case：用户无活跃 pet 行）；client 解析层按 `Optional<MemberPetDTO>` 处理，`pet == null` 降级为"无宠物"占位渲染（不渲染 cat sprite），**不**视为响应 malformed。绝大部分用户场景下 `pet ≠ null`（节点 2 首次初始化事务保证默认 pet 行），但 contract 层必须覆盖该 edge case，避免 server 端 JOIN `pets` 时丢行（违反 `memberCount === members.length` 不变量）或 fabricate 假 pet 数据（违反真实性）。

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截 |
| 1002 | 参数错误 | `roomId` 路径参数格式错（非数字 / 长度 > 20 字符） |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截 |
| 6001 | 房间不存在 | 服务端逻辑步骤 2：`rooms WHERE id = ?` 查不到记录（兜底场景，详见步骤 2 注释） |
| 6004 | 用户不在房间中 | 服务端逻辑步骤 1：`users.current_room_id != path roomId`（含 `current_room_id == NULL`）—— caller 不是该房间成员，**禁止**查看 |
| 1009 | 服务繁忙 | DB 异常 / 内部 panic |

> **注**：本接口**不**触发 6002 / 6003 —— 这两个错误码是 join 路径上的"房间已满 / 用户已在房间"语义，纯查询不涉及；6005（房间状态异常）在节点 4 阶段**不**用于本接口（即使 `rooms.status = 2 closed` 也允许查询 —— 但前提是 caller 仍是该房间成员才能走到这步，因为步骤 1 已先校验 ACL）。**6004 触发条件与 §10.5 leave 接口步骤 1 同源**（`users.current_room_id != path roomId`），statement 层语义统一：caller 不是该房间成员 → 6004，不区分"未加入任何房间" vs "加入了其他房间"。

#### `data.members[]` 字段五阶段过渡表

各字段在 MVP 不同节点 / Epic 落地后的下发值演进：

| 字段 | 节点 4 placeholder（Story 10.7） | 节点 4 真实（Story 11.6 / 11.7） | 节点 5 真实（Epic 14） | 节点 9 真实（Epic 26） | 节点 10 真实（Epic 29） |
|---|---|---|---|---|---|
| `userId` | `room_members.user_id` 字符串化 | 同 placeholder | 同 placeholder | 同 placeholder | 同 placeholder |
| `nickname` | `""`（不 JOIN `users`，详见 §12.3 placeholder 说明） | `users.nickname`（GET /rooms/{roomId} 始终走真实路径；WS room.snapshot 由 Story 11.7 SnapshotBuilder 真实实装） | 同节点 4 真实 | 同节点 4 真实 | 同节点 4 真实 |
| `avatarUrl` | **不下发**（§12.3 字段表 placeholder 不含此字段） | `users.avatar_url`（首次创建为 `""`，本接口始终走真实路径） | 同节点 4 真实 | 同节点 4 真实 | 同节点 4 真实 |
| `pet` 整体 | LEFT JOIN `pets`：用户**无活跃 pet 行**时下发 `null`（pet-less edge case，与 §5.1 GET /home `data.pet = null` 同语义） | 同 placeholder | 同 placeholder | 同 placeholder | 同 placeholder |
| `pet.petId`（仅 `pet ≠ null`） | `pets.id` 字符串化（与节点 4 真实同 —— LEFT JOIN `pets` 后 `pet ≠ null` 分支即 `pets.id` 真实值） | `pets.id` 字符串化 | 同节点 4 真实 | 同节点 4 真实 | 同节点 4 真实 |
| `pet.currentState`（仅 `pet ≠ null`） | 固定 `1` | 固定 `1` | `pets.current_state`（真实 1/2/3，由 Epic 14 状态机驱动；**同一 epic 落地点 Story 14.3 同时覆盖 §12.3 `room.snapshot` + `member.joined` 两处 `pet.currentState` 字段**，三处 server → client `pet.currentState` 同步切真实路径，避免 join 房间后看到 `1` 永远 stale 的 race） | 同节点 5 | 同节点 5 |
| `pet.equips`（仅 `pet ≠ null`） | **不下发**（§12.3 字段表 placeholder 不含此字段） | `[]`（节点 4 阶段 Story 11.6 GET /rooms/{roomId} 固定返回空数组；WS room.snapshot 同样不下发） | 同节点 4 真实 | `user_pet_equips JOIN cosmetic_items` 聚合（Story 26.6 真实回填，**不**含 `renderConfig` 子字段） | 同节点 9 + 加 `renderConfig` 子字段（Story 29.6 真实回填 `pet.equips[].renderConfig`） |

**关键解读**：

1. **GET /rooms/{roomId}（本接口）从节点 4 起就走"真实"路径**（Story 11.6 实装时 JOIN `users` / `pets`）；`pet.currentState` / `pet.equips` 仍按上表节点 4 列返回固定值，但 `nickname` / `avatarUrl` / `pet.petId` 在节点 4 真实路径下就是真实值（与 placeholder 不同）。即"REST 接口本身节点 4 真实，仅 pet 状态字段后续 epic 真实驱动"。
2. **WS `room.snapshot` placeholder（Story 10.7）/ 真实（Story 11.7）路径**与 REST 接口路径独立：placeholder 阶段允许 `nickname` 空字符串（不 JOIN `users`），真实阶段填真实值；`pet` 整体 placeholder 阶段已与真实阶段对齐（LEFT JOIN `pets`，pet-less 下发 `null`，否则 `pet.petId` 为 `pets.id` 真实值），**不**做 placeholder 简化（与本表 `pet.petId` 行 placeholder / 真实两列同值一致）；详见 §12.3 placeholder 字段值来源说明。**Story 10.7 落地实装的过渡兼容**：Story 10.7 当前实装走单表查询（不 JOIN `pets`），所有 member 一律下发 `pet ≠ null` + `petId: ""` —— 这是 r10 之前的契约形态，r14 起本节五阶段表已锁定 Story 10.7 placeholder 的 going-forward 契约为 LEFT JOIN + nullable，但 Story 10.7 已 done 不回工；Story 11.7 真实 SnapshotBuilder 落地时**必须**切换到 LEFT JOIN `pets` + pet-less 下发 `null` 的 going-forward 契约形态（详见 §12.3 placeholder 字段值来源说明末尾"Story 10.7 落地实装与 going-forward 契约的差异"段）。
3. **client merge contract**（§12.3 末尾 client merge contract 引用块）保证：client 通过 GET /rooms/{roomId}（本接口节点 4 真实路径）拿到的真实 nickname / petId / avatarUrl，在收到 `room.snapshot` placeholder 的空字符串字段时**保留**真实值，不被空串覆盖。这条契约对本接口的节点 4 真实路径同样适用。
4. **`pet.equips` 节点 4 阶段固定 `[]`**：与 §5.1 GET /home 的 `pet.equips` 节点 4 阶段语义一致（详见 §5.1 字段表 + Future Fields）；本接口 `members[].pet.equips` 与 §5.1 单 user `pet.equips` 是两个独立字段路径，但占位语义相同。

#### 不变量（response 内部一致性）

- `data.room.memberCount` **必须严格等于** `data.members[]` 数组长度
- `data.members[]` 必须包含**全部** `room_members WHERE room_id = ?` 行；service 实装层**禁止**在 query 后做"WS 此刻是否连接"层的过滤（与 §12.3 `room.snapshot` 不变量一致：snapshot / GET /rooms 都返回当前 `room_members` 全行 —— full roster view，不是 online-only view）。**注**：roster 反映的是 server 端"仍在房间"状态，不区分"WS 当前在线 / 已断开但 row 还在"；任何 WS 断开（含心跳超时 / 客户端主动 close / TCP 1006 / app 关闭）**只**清 ephemeral 层（SessionManager + Redis presence），**不**触动 `room_members` 行 —— 只有 HTTP `POST /rooms/{roomId}/leave` 才删 row + 触发 `member.left` 广播；详见本节末"roster 语义与 WS 断线交互"小节
- `data.members[]` 顺序由 server 决定（建议按 `room_members.joined_at ASC` 稳定排序，便于 client 渲染顺序稳定）；client 解析层**不**应假设特定顺序（除"稳定"外的语义）
- `data.room.status` 与 `data.members[]` 的关系：`status = 2 closed` 时 `members[]` 必为 `[]`（因为退出房间事务在最后一人离开时同时删除最后一行 `room_members` + 更新 `rooms.status = 2`，详见 §10.5 服务端逻辑）；`status = 1 active` 时 `members[]` 必非空（房间至少有 1 个成员，否则按业务规则 status 应已变 closed）

**关键约束**：

- 字段表中所有 BIGINT 主键 / 外键（`id` / `creatorUserId` / `userId` / `petId`）严格按 §2.5 全局约定字符串化下发
- `data.members[].avatarUrl` 必须存在且为 string（可空字符串 `""`，**不**为 null）；与 §4.1 / §4.3 / §5.1 中 `user.avatarUrl` 处理一致
- `data.members[].pet` 是 nullable object（`pet-less` 账号下发 `null`，详见字段表 `data.members[].pet` 行）；client 解析层按 `Optional<MemberPetDTO>` 处理；当 `pet ≠ null` 时下方所有 `pet.*` 子字段约束生效，当 `pet == null` 时**整个 pet 子树不下发**（不下发空 object `{}`，避免 client 误判为"有 pet 但字段缺失"）
- `data.members[].pet.equips`（仅当 `pet ≠ null`）节点 4 阶段必须为 `[]`（空数组，**不**省略 key、**不**为 null）；client 解析层按 `[<EquipDTO>]` 解析，节点 4 阶段数组永远为空，节点 9 由 Epic 26 真实回填非空数组
- 节点 4 阶段**不**下发 `members[].pet.equips[].renderConfig`（Future Fields，节点 10 由 Epic 29 / Story 29.6 加；本字段表**不**列入）

#### roster 语义与 WS 断线交互

节点 4 阶段 `data.members[]` 的"成员"定义 = "**server 端仍认为在房间**的成员"（即 `room_members WHERE room_id = ?` 当前存在的行）；**不**在字段层引入"online / offline"区分。

**核心钦定（持久层 vs ephemeral 层职责分离）**：

- **持久层 = `room_members` 行 + `users.current_room_id`**：**唯一**变更路径是 HTTP `POST /api/v1/rooms/{roomId}/join`（添加行）和 HTTP `POST /api/v1/rooms/{roomId}/leave`（删除行）—— 详见 §10.4 / §10.5 服务端逻辑。任何 WS 层事件（含心跳超时、TCP 1006、客户端主动 close、app 关闭 / 切后台）**禁止**修改持久层
- **ephemeral 层 = SessionManager 内存映射 + Redis presence**：由 WS 连接生命周期管理（onRegister / onUnregister 钩子）；任何 WS 断开（含主动 close / 心跳超时 / TCP 异常）**只**清理 ephemeral 层（撤销 Session、清 presence），**不**触动持久层

→ "WS 断开 = 离开房间" **不**成立；只有 HTTP leave（authoritative signal = HTTP 200 响应，详见 §10.5 步骤 9）才是真正的"离开房间"路径；§10.5 步骤 7 的 close 4007 是 server 端 best-effort cleanup（让 leaver Session 立即与房间 WS 解耦 —— 既保证 broadcast `member.left` 时 leaver Session 已不在 fanout 列表，也避免 leaver 拖到心跳超时仍收房间广播），不构成独立"离开"路径。这条钦定是 §10.5 / §12.1 4005 retryable 语义 / §12.3 `### 成员离开` 触发条件三处协同的契约基础。

**各类 WS 断开场景的语义**：

- **WS 当前连接但暂时停留**（如 client 网络抖动、TCP 重传中）：心跳尚未超时，Session 仍活跃，`room_members` 行仍在；roster 不变
- **WS 心跳超时**（默认 60s 无 pong → server close 4005，reason = `"heartbeat timeout"`）：server 撤销 Session（onUnregister 清 ephemeral），**不**改 `room_members` / `users.current_room_id`，**不**广播 `member.left`；client 按 §12.1 close code 表 4005 行钦定 transient 语义自动重连，重连握手时 §12.1 校验顺序步骤 5（`room_members` 表查询）**通过**（行仍在），4005 reconnect 设计**有意义且自洽**
- **client 主动 close / app 关闭 / 前台切后台超过保活窗口 / TCP 异常断开（1006）**：与心跳超时同路径 —— 仅清 ephemeral 层，**不**改 `room_members` / `users.current_room_id`，**不**广播 `member.left`；用户后续若重新打开 app 或 reconnect，`room_members` 行仍在，可直接 WS 握手回到原房间
- **HTTP leave**（用户主动通过 `POST /rooms/{roomId}/leave` 请求离开）：删 `room_members` 行 + `users.current_room_id = NULL` + 广播 `member.left` + close leaver 自己的 WS Session（close code 4007）—— 详见 §10.5 服务端逻辑

**memberCount 与 reconnect 语义的 self-consistency**：`memberCount = members[].length = 当前 room_members 行数`；只有 HTTP leave 会让 `memberCount` 递减并广播 `member.left`；任何 WS 断开（含心跳超时）**不**改变 `memberCount`，roster 在 client 端保持稳定，重连后 server 推送的 `room.snapshot` 与 reconnect 前一致 —— 三层（持久层、ephemeral 层、广播事件）语义对齐，无 drift。

> **设计 rationale**：r1 review 指出"transient WS disconnect 不应整人踢出房间，否则 `含离线成员` 契约自相矛盾"；r2 修复尝试通过"心跳超时 = 删行 + 广播 member.left"路径让 roster 不区分在线 / 离线但仍正确清理僵尸用户。但 r3 review 指出该方案与 §12.1 4005 retryable 语义内在冲突 —— client 按钦定 transient 语义重连，握手时 §12.1 步骤 5 立即 fail 4003（行已删），reconnect 形同虚设。本契约最终定位（r3 锁定）：**WS 断开仅清 ephemeral，房间归属只能由 HTTP leave 改变**；transient 容错（含心跳超时窗口）由 ephemeral 层独立处理，行不删 → 4005 reconnect 自洽 → 跨文档（V1接口设计.md / 时序图设计.md）只有一种 disconnect 语义可遵循。"含离线成员"概念依然不引入（roster 字段层不区分），但保留 row 的实现机制变成"WS 断开不删 row"而非"心跳超时阈值"。

---

## 10.4 加入房间

### `POST /api/v1/rooms/{roomId}/join`

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | POST |
| Path | /api/v1/rooms/{roomId}/join |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | 默认（按 Story 4.5 rate_limit 默认值 60 次/分） |
| 幂等 | **非幂等**（同一用户重复 join 自己已在的房间 → 返回 6003；不能用 `idempotencyKey` 跳过校验） |
| 节点 | 节点 4（Epic 11 落地，Epic 12 客户端集成） |

#### 请求

| 参数位置 | 字段 | 类型 | 必填 | 长度/范围约束 | 说明 |
|---|---|---|---|---|---|
| Path | `roomId` | string | 必填 | 必须是 BIGINT 数字字符串（如 `"3001"`），1 ≤ length ≤ 20 字符 | 目标房间主键 |
| Body | - | object | 必填 | - | 空对象 `{}`；当前用户身份从 token 解析 |

JSON 示例（请求体）：

```json
{}
```

#### 服务端逻辑

事务内严格按顺序执行（详见 Story 11.4 实装）：

1. 查 `users.current_room_id`，**非 null** → 立即返回 **6003 用户已在房间中**（不开事务）；**特例**：若 `current_room_id == 当前请求的 roomId`（用户尝试加入自己已在的房间）也返回 6003 —— 用同一码统一处理"用户已在某房间"语义，client 仅需识别 6003 即可，不需要区分"已在目标房间" vs "已在其他房间"
2. 开事务，对 `rooms` 行加排他锁：`SELECT ... FROM rooms WHERE id = ? FOR UPDATE`（**同时锁住"房间存在性 / status 字段 / 成员计数判断"三件事，与并发 leave 串行化**，详见下文"并发保护"段）；**找不到** → 回滚 + 返回 **6001 房间不存在**
3. 查到的 `rooms.status != 1` → 回滚 + 返回 **6005 房间状态异常**（`status = 2 closed` 房间禁止加入）
4. 查 `room_members WHERE room_id = ?` 数量，`>= 4` → 回滚 + 返回 **6002 房间已满**
5. 插入 `room_members`（`room_id`, `user_id = 当前 user`）；如果遇到 DB UNIQUE(`user_id`) 兜底冲突（理论不会，因为步骤 1 已查过 `users.current_room_id`，但并发竞态下可能发生）→ 回滚 + 返回 **6003**
6. 更新 `users.current_room_id = roomId`
7. 提交事务
8. **事务成功提交后**触发 WS 广播 `member.joined`（payload 见 §12.3 `### 成员加入`，由 Story 11.8 实装；广播失败 fire-and-forget 仅 log，不影响 HTTP 200 响应）
9. 返回 `data.{roomId, joined: true}`

**事务边界规则**：步骤 2 ~ 6 必须在同一 MySQL 事务中（参见数据库设计 §8.6 加入房间事务边界）；步骤 8 在事务**外**触发（fire-and-forget，与 Story 10.5 BroadcastToRoom primitive 既有语义一致 —— 广播是事件通知，不参与事务原子性）。

**并发保护**：步骤 2 的 `SELECT rooms ... FOR UPDATE` 行锁是关键 —— 同时承担两个职责：

1. **同房间并发 join 串行化**：4 个用户已在房间，5 个用户同时调用 join，必须只有 1 个成功（或 0 个，若房间已 closed），其他全部返回 6002（或 6005）；
2. **与 §10.5 leave 跨事务串行化**（解决 r9 P1#2 race）：用户 A leave 与用户 B join 跨事务，若两者都未对 `rooms` 行加锁，可产生 timeline："A 删 room_members 行 → A 看到 remaining=0 但**未** UPDATE rooms.status → B join 锁 rooms（看到 status=1）+ insert room_members + commit → A UPDATE rooms.status=2 closed + commit"，结果 `rooms.status=2 closed` 但 `room_members` 含 B 的行，状态 drift。锁 `rooms` 行让两类事务在 rooms 行 lock 上串行 —— A leave 锁 rooms → B join wait → A 提交（含 status=2）后 B 才能继续 → B 步骤 3 看到 `rooms.status=2` → 直接返回 **6005 房间状态异常**（房间已关闭），状态保持一致。

详见 Story 11.4 单测 / 11.9 集成测试并发场景（含 cross-tx leave-then-join race 用例）。

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `data.roomId` | string | 必填 | 加入的房间主键（BIGINT 字符串化）；与请求 path 中 `roomId` 一致，回带方便 client 校验 |
| `data.joined` | boolean | 必填 | 固定 `true`（成功路径必返）；**不**为 `false` —— 失败时返回错误码而非 `joined: false` |

JSON 示例：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "roomId": "3001",
    "joined": true
  },
  "requestId": "req_xxx"
}
```

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截 |
| 1002 | 参数错误 | `roomId` 路径参数格式错（非数字 / 长度 > 20 字符） |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截 |
| 6001 | 房间不存在 | 服务端逻辑步骤 2：`rooms WHERE id = ?` 找不到 |
| 6002 | 房间已满 | 服务端逻辑步骤 4：`room_members` 当前数 `>= 4` |
| 6003 | 用户已在房间中 | 服务端逻辑步骤 1（预检）或步骤 5（DB UNIQUE 兜底）：用户已在任意房间（含目标房间） |
| 6005 | 房间状态异常 | 服务端逻辑步骤 3：`rooms.status != 1`（如已 closed） |
| 1009 | 服务繁忙 | 事务回滚 / DB 异常 / 内部 panic |

> **注**：本接口**不**触发 6004（用户不在房间中）—— 6004 用于 `GET /rooms/{roomId}`（§10.3，ACL fail：caller 不是该房间成员）和 `POST /rooms/{roomId}/leave`（§10.5，重复 leave / 不在该房间），与本接口（join）的语义场景正交。

**关键约束**：

- 错误码触发顺序严格按服务端逻辑步骤：步骤 1 → 6003（预检）；步骤 2 → 6001；步骤 3 → 6005；步骤 4 → 6002；步骤 5 → 6003（兜底）。**不**允许实装层重排顺序（如先查容量再查 status，会让"closed 房间满员"场景错误返回 6002 而非 6005）
- 6003 在两条路径下触发（步骤 1 预检 + 步骤 5 DB UNIQUE 兜底），但 client **不**应区分这两种场景（都是"用户已在某房间"，UX 处理一致）
- `data.joined` 固定 `true`：成功路径必返；失败路径返回错误码 + 不返回 `joined` 字段（按 §2.4 通用响应结构，错误时 `data` 为空对象 / 不含业务字段）

---

## 10.5 退出房间

### `POST /api/v1/rooms/{roomId}/leave`

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | POST |
| Path | /api/v1/rooms/{roomId}/leave |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | 默认（按 Story 4.5 rate_limit 默认值 60 次/分） |
| 幂等 | **非幂等**（同一用户重复 leave 同一房间 → 第二次返回 6004；client 应在收到 200 后清本地状态，不要重试） |
| 节点 | 节点 4（Epic 11 落地，Epic 12 客户端集成） |

#### 请求

| 参数位置 | 字段 | 类型 | 必填 | 长度/范围约束 | 说明 |
|---|---|---|---|---|---|
| Path | `roomId` | string | 必填 | 必须是 BIGINT 数字字符串，1 ≤ length ≤ 20 字符 | 目标房间主键 |
| Body | - | object | 必填 | - | 空对象 `{}` |

JSON 示例（请求体）：

```json
{}
```

#### 服务端逻辑

事务内严格按顺序执行（详见 Story 11.5 实装）：

1. 查 `users.current_room_id`，**与请求的 `roomId` 不一致**（含 `current_room_id` 为 null）→ 立即返回 **6004 用户不在房间中**（不开事务）
2. 开事务，对 `rooms` 行加排他锁：`SELECT ... FROM rooms WHERE id = ? FOR UPDATE`（**与并发 join 串行化**，确保后续步骤 4 的 remaining-count + 步骤 5 的 status update 与并发 join 不交错；详见下文"事务边界规则"段 r9 race 说明）；**找不到 rooms 行**：理论上不会（步骤 1 已通过意味着 caller 在该房间，rooms 行必存在），但 race 兜底：回滚 + 返回 **1009 服务繁忙**（数据不一致，按 DB 异常处理）
3. 删除 `room_members WHERE room_id = ? AND user_id = ?`（删除当前 user 的成员行）；**检查 `RowsAffected`**：若 `== 0`（同一用户两次并发 leave 都通过步骤 1，赢家已先删该行，输家此处 0 行受影响）→ 回滚 + 返回 **6004 用户不在房间中**（与步骤 1 同语义统一）；该兜底是关键并发护栏，否则输家会继续走完步骤 4 ~ 8 产生重复广播 + 重复 close 4007
4. 更新 `users.current_room_id = NULL`
5. 查 `room_members WHERE room_id = ?` 剩余数量；若 `== 0`（最后一人离开）→ 更新 `rooms.status = 2 closed`
6. 提交事务
7. **关闭 leaver 自己的 WS Session**（若 leaver 仍持有该 `roomId` 的 WS 连接）：从 SessionManager 撤销 (unregister) leaver 在该 `roomId` 的 Session + close underlying WebSocket（close code = `4007`，reason = `"left room via HTTP"`，详见 §12.1 close code 表 4007 行；client 解析层按 4xxx 业务级终态语义处理，**不**自动重连）；该步骤**必须**发生在步骤 8 广播之**前**（r13 锁定）—— 顺序由"广播 fanout 时 leaver Session 已被 unregister，BroadcastToRoom primitive 调 ListSessionsByRoomID 时返回的列表里自然不含 leaver Session"语义钦定（§12.3 `### 成员离开` 关键约束），从而无需 BroadcastToRoom primitive 提供 `excludeUserID` 参数即可保证 leaver 物理上收不到自己的 `member.left`。leaver 的 Session 撤销也不能拖到心跳超时（默认 60s）才触发，否则在 close 前的窗口内 leaver 仍会收到该 roomId 的 `member.joined` / 后续 epic 广播消息（如 Story 14.x `pet.state.changed` / Story 17.x `emoji.received`），违反"HTTP leave 后立即与房间 WS 解耦"语义。Session 撤销失败（leaver 未持 WS 连接 / 已断开）→ no-op，不影响 HTTP 200 响应（fire-and-forget）。**该步骤是 server 端 best-effort cleanup，不是 client 侧 leave 完成的 authoritative confirmation**：leaver 的 client 状态推进**必须**以 HTTP 200 响应（步骤 9）为唯一权威信号 —— close 4007 frame 与 HTTP 200 走两条独立连接，client 可能完全收不到 4007（leaver 的 WS 早已断开 / 4007 比 HTTP 200 晚到 / 中间网络丢包均合法），下方 close 顺序 rationale 段 + §12.1 close code 表 4007 行明确"4007 仅作 best-effort UX 辅助信号"，client **禁止**等 4007 才 tear down 房间状态。**注意**：步骤 7 提前到 broadcast 之前**只**改变 server-side 内部时序（让 broadcast fanout 时 leaver session 已 unregister），**不**改变 client-perceived 协议契约 —— HTTP 200 仍是 authoritative success signal；close 4007 仍是 best-effort cleanup（leaver client 收到 4007 时是协议确认；收不到时 client 仍按收到 HTTP 200 时 tear down，不依赖 4007）
8. **事务成功提交后**触发 WS 广播 `member.left`（payload 见 §12.3 `### 成员离开`，由 Story 11.8 实装；广播失败 fire-and-forget 仅 log，不影响 HTTP 200 响应）；调用 `BroadcastToRoom(roomID, {type: "member.left", ...})` 时 `ListSessionsByRoomID(roomID)` 返回列表里**不含** leaver Session（步骤 7 已 unregister），fanout 自然跳过 leaver —— **因此 BroadcastToRoom primitive 无需 `excludeUserID` 参数**（这是步骤 7 / 8 当前顺序设计的关键收益，r13 锁定）；**特例**：若步骤 5 触发了 closed 转换（房间已无在线广播对象）—— 广播路径仍调用 `BroadcastToRoom`，但 fanout 时房间内已无其他在线 Session，广播自然 no-op（详见 Story 10.5 BroadcastToRoom primitive 实装：空房间走 fast path 直接返回）
9. 返回 `data.{roomId, left: true}`（**这是 leave 成功的 authoritative 信号**：client 收到 HTTP 200 + `data.left: true` 即应立即清本地房间状态并退出 RoomView，**不**等待 close 4007；4007 是 server 端 fire-and-forget WS cleanup，到达不可保证 —— 详见步骤 7 末尾 + §12.1 close code 4007 行 client 行为指引）

**事务边界规则**：步骤 2 ~ 5 必须在同一 MySQL 事务中（参见数据库设计 §8.7 退出房间事务边界）；步骤 3 的 `RowsAffected == 0` 兜底**必须**在事务内回滚（不允许在事务外做 SELECT 校验后再开事务 —— 那种实现仍存在 step1-SELECT 与 DELETE 之间的竞态窗口，无法消除并发 race）；步骤 7 / 8 都在事务**外**触发（fire-and-forget）。**步骤 7（close 4007 + unregister leaver Session）必须先于步骤 8（broadcast `member.left`）**（r13 锁定）—— 该顺序由"BroadcastToRoom primitive 不带 `excludeUserID` 参数（与 Story 10.5 落地实装一致）→ fanout 给 ListSessionsByRoomID 全部 session"约束推导：若先 broadcast 再 close，leaver Session 仍在 SessionManager 列表中，broadcast 会把 `member.left` 投递给 leaver 自己的 Session，与§12.3 `### 成员离开`"leaver 不收自己 member.left（fanout 列表中物理排除离开者）"语义直接矛盾。先 close + unregister leaver Session 后，broadcast 时 ListSessionsByRoomID 返回列表自然不含 leaver，无需扩展 primitive 即可满足契约。

**HTTP 200 vs WS close 4007 — authority 与 best-effort 分工**（r10 锁定）：

- **HTTP 200（步骤 9）**：leave 成功的**唯一 authoritative signal**。client 收到 HTTP 200 + `data.left: true` 即应**立即**清本地房间状态、退出 RoomView、清房间相关订阅 —— **禁止**等待 close 4007 才推进。原因：HTTP 响应与 WS close frame 走两条独立 TCP 连接，无任何协议层 ordering / delivery 保证：
  - leaver 的 WS 可能早已断开（如 client 离开房间页前主动 close 了 WS）→ server 步骤 7 走"Session 撤销失败 no-op"路径，**不会**有 4007 frame 被发出
  - 4007 frame 可能比 HTTP 200 晚到（WS 与 HTTP 是不同连接，TCP 层无序保证）→ client 等 4007 会引入不必要的 UX 延迟
  - 中间网络 / 代理层可能丢 4007 frame（best-effort 投递）
- **close 4007（步骤 7）**：server 端**best-effort cleanup** + **辅助 UX 信号**。其 server 端职责双重 —— (a) 让 broadcast `member.left`（步骤 8）调用 `BroadcastToRoom` 时 `ListSessionsByRoomID` 列表中已不含 leaver Session（步骤 7 已 unregister），从而无需 primitive 提供 `excludeUserID` 参数即满足"leaver 不收自己 member.left"语义（r13 锁定）；(b) 立刻让 leaver 在 close 之前的窗口内不再收到本房间的后续 WS 广播（避免 leaver Session 拖到心跳超时才被清理时持续收到 `member.joined` / 后续 epic 广播）。其在 client 侧的角色仅是**冗余辅助 UX 信号**：若 client 因极端场景没收到 HTTP 200（如 HTTP 请求超时但 server 已成功处理）但收到了 4007，可作为 fallback 触发 RoomView 退出 UX。**禁止**把 4007 当作 leave 完成的 authoritative confirmation —— client 实装层不应等 4007 才 tear down 房间状态，否则在 4007 不到达的合法场景（leaver WS 已断 / 4007 丢包 / 4007 晚到）会出现 client 卡在 leaving 状态。

> **rationale**：r10 review 指出"HTTP 响应和 WS close frame 是两个独立连接，client 可能根本收不到 4007（WS 已断 / 4007 比 HTTP 200 晚到）；如果 client 把 4007 当作 leave 完成的 authoritative confirmation，等 4007 才 tear down 房间状态 → flaky"。本契约最终定位：HTTP 200 是 protocol-level authoritative success signal，WS close 4007 是 best-effort cleanup（不保证 client 收到，到达即作 fallback UX 辅助）。该 invariant 与 §12.1 close code 表 4007 行 + iOS Story 12.7 LeaveRoomUseCase 实装层一致。

**步骤 2 `SELECT rooms FOR UPDATE` 必要性**（解决 r9 P1#2 race）：若 leave 不锁 `rooms` 行，可与并发 join 产生 timeline："A leave 步骤 3 删 room_members → A 步骤 5 看到 remaining=0 但**未** UPDATE rooms.status → B join 此时锁 rooms（看到 status=1）+ insert 自己的 room_members + commit → A UPDATE rooms.status=2 closed + commit"，结果 `rooms.status=2 closed` 但 `room_members` 有 B 的行 → B 后续 join 行为 / GET roster 全部失败，状态 drift。两类事务在 `rooms` 行 lock 上串行后：A leave 先锁 → B join wait → A 提交（含 status=2）后 B 才进入 → B 步骤 3 看到 `rooms.status=2` → 返回 **6005 房间状态异常**（与 §10.4 join 步骤 3 一致），状态保持一致。**join + leave 必须都对 `rooms` 行加 FOR UPDATE，缺一不可**。

**WS 断线场景与本接口的关系**（r3 锁定语义）：心跳超时（→ close 4005）、client 主动 close、app 关闭 / 切后台、TCP 1006 异常断开等任何 WS 层断开**都不**走本接口的事务路径（不删 `room_members` 行、不改 `users.current_room_id`、不广播 `member.left`）—— 它们仅清理 ephemeral 层（SessionManager 撤销 Session + Redis presence 清理），由 Story 10.4 onUnregister 钩子统一负责；详见 §10.3 "roster 语义与 WS 断线交互" 小节。换言之，**只有本接口（HTTP leave）的事务**是真正的"离开房间"路径（authoritative signal = HTTP 200 步骤 9，详见上方 "HTTP 200 vs WS close 4007 — authority 与 best-effort 分工" 段）；步骤 7 的 close 4007 是 server 端 best-effort cleanup（让 leaver Session 立即解耦房间 WS，既保证 broadcast `member.left` 时 fanout 列表已不含 leaver，也避免拖到心跳超时仍收广播），不构成独立的"离开房间"路径。WS 断线后 `room_members` 行仍在，user 仍在 roster 中；用户通过 reconnect / 重新打开 app 即可回到原房间，无需重新 join —— 这是 §12.1 4005 retryable 语义的契约基础。

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `data.roomId` | string | 必填 | 离开的房间主键（BIGINT 字符串化）；与请求 path 中 `roomId` 一致 |
| `data.left` | boolean | 必填 | 固定 `true`（成功路径必返） |

JSON 示例：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "roomId": "3001",
    "left": true
  },
  "requestId": "req_xxx"
}
```

> **注**：响应中**不**包含"房间是否已 closed"字段 —— client 不直接感知房间状态变化，仅 `data.left = true` 表示当前 user 已离开；房间状态变化是 server 内部副作用，**不**对外暴露（避免 client 围绕"我是否是最后一人"做特殊 UX 分支）。

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截 |
| 1002 | 参数错误 | `roomId` 路径参数格式错（非数字 / 长度 > 20 字符） |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截 |
| 6004 | 用户不在房间中 | 服务端逻辑步骤 1（预检）：`users.current_room_id != roomId`（含 null）；或步骤 3（事务内兜底）：`DELETE room_members ... RowsAffected == 0`（并发 race 输家） |
| 1009 | 服务繁忙 | 事务回滚 / DB 异常 / 内部 panic（**不**含步骤 3 的 `RowsAffected == 0` —— 那条走 6004）；含步骤 2 `SELECT rooms FOR UPDATE` 找不到 rooms 行（与 step 1 通过状态不一致，按 DB 异常处理） |

> **注**：本接口**不**触发 6001 / 6002 / 6003 / 6005 —— leave 仅校验"用户与房间归属"，不校验房间是否存在；步骤 2 的 `SELECT rooms FOR UPDATE` 仅用于 row lock（与并发 join 串行化），找不到行视为内部状态不一致（按 1009 处理），**不**对外暴露 6001。

**关键约束**：

- 6004 触发条件包含三种场景（client UX 一致处理）：(a) `users.current_room_id == NULL`（步骤 1 预检，用户当前不在任何房间）+ (b) `users.current_room_id != path roomId`（步骤 1 预检，用户在其他房间）+ (c) 同一用户并发两次 leave 同一房间，赢家事务内已删除 `room_members` 行，输家步骤 3 `DELETE` 0 行受影响（事务内兜底回滚）—— 都返回 6004，client 收到 6004 就清本地房间状态、不重试
- 步骤 3 的 `RowsAffected == 0` 兜底是**必须**项（不是优化）：缺失则两次并发 leave 中输家会继续走完步骤 4 ~ 8 —— 即使步骤 4 把已 NULL 的 `current_room_id` 再次写 NULL（idempotent）、步骤 5 房间剩余数已经被赢家算过、步骤 7 会试图关闭已不属于该房间的 leaver Session、步骤 8 广播会在房间内产生**重复** `member.left` 事件 —— 全部是错误副作用。
- 步骤 2 的 `SELECT rooms FOR UPDATE` 与步骤 3 的 `RowsAffected == 0` 兜底**职责正交，不可互替**：步骤 2 锁解决"跨事务 leave-vs-join 状态 drift"（详见服务端逻辑段 r9 race 说明）；步骤 3 兜底解决"同一 user 并发两次 leave 输家走完后续步骤"。两者都是**必须**项，且不冲突 —— 步骤 2 锁的是 `rooms` 行（与 join 互斥），步骤 3 删的是 `room_members` 行（与 leave 自身重入互斥），锁对象不同。
- "最后一人离开 → 房间 closed"是 server 端**单向**状态变化：closed 房间无法被重新激活（节点 4 阶段无"重启房间"接口）；这条规则确保 `rooms.status` 严格单调（1 → 2，无回退路径），简化 server 状态机
- `data.left` 固定 `true`：与 §10.4 `data.joined` 设计对称，避免引入"left: false" 模糊语义

---

> **§10 房间章节末尾引用**：`GET /home.data.room.currentRoomId` 由 Story 11.10 真实实装（节点 4，Epic 11 收官）—— 本 story（11.1）**不**改 §5.1 GET /home schema；§5.1 已在 Story 4.1 锚定 + Future Fields 已标注 `room.currentRoomId` 节点 4 由 Story 11.10 注入真实数据。


## 11. 表情接口

## 11.1 获取系统表情配置

### `GET /api/v1/emojis`

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | GET |
| Path | /api/v1/emojis |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | 默认（按 Story 4.5 rate_limit 默认值 60 次/分；表情列表是静态配置，client 应在 App 生命周期内缓存 —— 每用户每分钟实际调用 << 60 次） |
| 幂等 | 天然幂等（GET 查询，无副作用） |
| 节点 | 节点 6（Epic 17 落地 / Epic 18 客户端集成） |
| 分页 | **不**分页（表情列表 4 ~ 20 个 short list，全量返回） |
| Query 参数 | **无**（不接受任何 query string，不支持筛选 / 排序参数） |

#### 请求

无请求体（GET 接口）；仅需 `Authorization: Bearer <token>` Header。

#### 服务端逻辑

1. **认证 & 限频**：auth 中间件校验 Bearer token；rate_limit 中间件按默认配置限频（60 次/分按 user_id 计；已认证业务路由统一按 user_id 而非 IP，与 §6.1 / §6.2 / §10.x / §5.2 同语义）
2. **查询**：`SELECT id, code, name, asset_url, sort_order FROM emoji_configs WHERE is_enabled = 1 ORDER BY sort_order ASC, id ASC`
   - **次要排序键 `id ASC`**：保证 `sort_order` 相同时返回顺序确定（避免 client 端"同 sort_order 表情顺序在不同请求间不一致"问题）
   - **`is_enabled = 1`** 过滤：disabled 表情**不**返回给 client（被 server 管理员临时下架 / WIP 阶段不放出的表情）
3. **DTO 转换**：把 DB row 映射为 response item（`id` 字段**不**下发，client 不需要数据库主键；`code` 作为业务标识符；`is_enabled` 不下发；`created_at` / `updated_at` 不下发）
4. **响应**：返回 `data.items` 数组，**不**分页（即使数组为空也返回 `items: []`，**不**返回 `null`）

**事务边界规则**：本接口**不**需要 MySQL 事务（仅 1 个 SELECT 查询）。

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 必填 | 范围/约束 | 说明 |
|---|---|---|---|---|
| `data.items` | array | 必填 | length ≥ 0（**不**分页 / 无上限） | 已 enabled 的表情列表（全量返回，与 §11.1 服务端逻辑步骤 4 "不分页" 对齐 —— **不**设条目数上限）；按 `sort_order` 升序 + 次要键 `id` 升序排列；列表可能为空（如 seed 未执行 / 全部 disabled）—— 空时返回 `items: []`，**不**返回 `null`；MVP 阶段 enabled 表情数量较少（Story 17.3 seed 钦定 8-10 个），未来如需限制由对应 epic 引入分页 query 参数（如 `?limit=N&offset=M`）+ 服务端 `LIMIT/OFFSET` SQL 改造，本接口契约**不**预留 |
| `data.items[].code` | string | 必填 | 1 ≤ length ≤ 64；只允许 `[a-z0-9_-]`（与数据库 `emoji_configs.code VARCHAR(64)` + `UNIQUE KEY uk_code` 一致） | 表情业务标识符（如 `"wave"` / `"love"`）；client 用作 `emoji.send.payload.emojiCode` 入参；全局唯一（DB 唯一约束保证） |
| `data.items[].name` | string | 必填 | 1 ≤ length ≤ 64 | 表情中文名（如 `"挥手"` / `"爱心"`），client 用作 UI 展示文字；DB 来源 `emoji_configs.name` |
| `data.items[].assetUrl` | string | 必填 | 1 ≤ length ≤ 255；**禁止**空字符串 `""` | 表情资源 URL —— 标准 web 可访问的静态图片资源 URL（**推荐 PNG**；MVP 阶段允许 placeholder URL，如 `https://placehold.co/64x64?text=Wave`），但**必须**是可访问的资源 URL；具体 client 渲染器选型与可接受图片格式由各 client 实装层决定（iOS 端见 Story 18.1 dev notes），**不**在契约层强制绑定（避免契约耦合特定 SwiftUI / Android / Web 组件能力）；Story 17.3 seed 钦定每个 enabled 表情必须有非空 `asset_url`，空字符串会在 client 端触发渲染失败 / 占位降级；server 端 seed 层 / admin 写入层**应**校验非空（数据库 `emoji_configs.asset_url VARCHAR(255) NOT NULL DEFAULT ''` 的 `DEFAULT ''` 仅是 DDL 兜底，**不**意味着允许 enabled 表情留空 —— enabled 表情 `is_enabled=1` 必须有非空 `asset_url`） |
| `data.items[].sortOrder` | number (int) | 必填 | 0 ≤ value ≤ 2^31 - 1 | 表情显示顺序（升序）；DB 来源 `emoji_configs.sort_order`；client 接收时**不**需要二次排序（server 已按 `sort_order ASC, id ASC` 排序） |

JSON 示例：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [
      {
        "code": "wave",
        "name": "挥手",
        "assetUrl": "https://...",
        "sortOrder": 1
      },
      {
        "code": "love",
        "name": "爱心",
        "assetUrl": "https://...",
        "sortOrder": 2
      }
    ]
  },
  "requestId": "req_xxx"
}
```

JSON 示例（空列表 edge case）：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": []
  },
  "requestId": "req_xxx"
}
```

> **注**：本接口**不**下发 `id` / `is_enabled` / `created_at` / `updated_at` 字段 —— `id` 是数据库主键 client 不需要（client 用 `code` 作为业务标识符）；`is_enabled` 已在 server 端过滤（仅下发 enabled 表情）；时间戳字段 client 无 UI 用途。
>
> **client 缓存契约**：表情列表是**静态配置**，client **应**在 App 生命周期内首次拉取后缓存，后续不再重复拉取（iOS Story 18.1 实装：表情面板首次显示 → GET /emojis → 缓存到 ViewModel / Singleton；后续表情面板显示直接读缓存）。如 server 端表情配置变更（admin 后台改 `is_enabled` / `sort_order` / 新增 emoji），client 需要重启 App 或主动刷新才能看到新值 —— 节点 6 MVP 阶段无 push 通知机制，可接受；后续若产品要求"实时刷新表情列表"，由对应 epic 决定是否引入 ETag / If-None-Match / WS 通知 emoji_configs.changed 等机制。

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截（无 Authorization 头 / token 非法 / token 过期 / token 解析失败） |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截（**已认证路由**按 `user_id` 限频，每用户每分钟 > 60 次；按 Story 4.5 默认值，配置可调） |
| 1009 | 服务繁忙 | DB 异常（SELECT 执行返回 `err != nil`，含 driver / 网络 / 慢查询超时等）/ 内部 panic（见 Story 17.4 service 实装） |

> **注**：本接口**不**触发 1002 参数错误（无 query / body 字段可校验）/ **不**触发 7001 表情不存在（这是列表查询，无单 emoji code 校验，7001 仅在 `emoji.send` WS 路径校验失败时触发，详见 §12.2 `### 发送表情` 错误响应段）/ **不**触发 6xxx 房间相关错误（本接口与房间无关）/ **不**触发 3xxx / 4xxx / 5xxx（步数 / 宝箱 / 装扮）。

**关键约束**：

- **不分页**：表情列表全量返回，**无条目数上限**（与上方响应体 `data.items` "length ≥ 0（不分页 / 无上限）" 字段约束对齐；MVP 阶段实际 8-10 个，Story 17.3 seed 钦定）；client 不需要处理"加载更多" / "下一页" UI；如未来表情数量增长到需要分页（业务上低概率），由对应 epic 重新评审契约（引入 `?limit=N&offset=M` query 参数 + 服务端 SQL `LIMIT/OFFSET` 改造）
- **不接受 query 参数**：本接口不支持任何 query string 筛选（如 `?category=greeting`）/ 排序（如 `?orderBy=name`）/ 分页（如 `?page=1`）；client 端如需 UI 层分类，自己实装本地分组 —— server 不引入分类字段（MVP 节点 6 不规划表情分类，KISS）
- **响应空列表 `items: []` 与 `items: null` 语义不同**：server **永远**返回 `items: []`（空数组）而**不**返回 `null`；client 解析层按"数组"假设处理（与 §10.3 `data.members[]` / §5.1 `data` 数组字段处理一致）—— `null` 会触发 Swift Codable 解析失败（除非 client 把字段标 Optional，但本字段为必填）
- **assetUrl 必非空字符串**（**禁止** `""`）：API 契约层钦定 `data.items[].assetUrl` 必须是 1 ≤ length ≤ 255 的非空字符串，且为**标准 web 可访问的静态图片资源 URL**（推荐 PNG）；具体 client 渲染器选型（如 iOS 的 `AsyncImage` / Android 的 `Coil` / Web 的 `<img>`）与可接受图片格式由各 client 实装层（如 iOS Story 18.1 dev notes）决定，**不**在契约层强制绑定；Story 17.3 seed 钦定每个 enabled 表情都有可访问的资源 URL（MVP 阶段允许 placeholder 如 `https://placehold.co/64x64?text=Wave`），空字符串会在 client 端触发渲染失败 / 占位降级；server 端 seed 层 / admin 写入层**应**校验 enabled 表情的 `asset_url` 非空（数据库 DDL `VARCHAR(255) NOT NULL DEFAULT ''` 的 `DEFAULT ''` 仅是兜底 schema，**不**代表允许 enabled 表情留空，**不**应被 server 端业务层利用为"合法空字符串"）；client 解析层**应**按 `String` 处理（不是 Optional）；**禁止** server 下发 `null` 触发 client 解析失败，**禁止** server 下发 `""` 让 client UI 层降级渲染问号
- **BIGINT 主键字段**（`emoji_configs.id`）**不**下发 → 无 §2.5 字符串化诉求；其余字段都是 string / int / int32，遵循 §2.5 全局约定（int 不字符串化）
- **`sortOrder` 不字符串化**：是普通 int 顺序字段，**不**是主键，不受 §2.5 BIGINT 字符串化规则约束（与 §11.1 `data.items[].sortOrder` 同语义）

---

## 12. WebSocket 协议设计

## 12.1 连接地址

#### 连接元信息

| 字段 | 值 |
|---|---|
| Protocol | WebSocket（生产 wss / 本地 dev ws） |
| Path | /ws/rooms/{roomId} |
| Query | `token`（必填，URL-encoded Bearer token，复用 §2.3 `Authorization: Bearer <token>` 的 token util，详见 Story 4.4） |
| 鉴权 | **需要**（连接握手时校验 token + 用户在该房间） |
| 限频 | **不**走 HTTP rate_limit（Story 4.5 中间件不挂在 WS 路由），但 Session 创建后受限于 Story 10.4 心跳超时（60s 静默自动清理） |
| 节点 | 节点 4（Epic 10 落地骨架，Epic 11 / 12 业务集成） |

连接 URL 模板：

```text
{ws_scheme}://{host}/ws/rooms/{roomId}?token={url_encoded_token}
```

- `ws_scheme`：生产环境 **`wss`**（TLS）；dev / test 环境 **`ws`**（明文，仅本地）
- `host`：与 HTTP 接口同 host（不分离 WS 域名，简化 Info.plist / CORS 配置）
- `roomId`：路径参数，必须是用户当前所在房间的 ID。client 端 roomId 来源**按场景分两路**（**不要**使用 `GET /me.user.currentRoomId` —— 该字段是永久 `null` schema 占位，无后续节点回填计划，详见 §4.3 Future Fields 引用块）：
  - **首次连接 / 刚完成房间动作（热路径）**：client 直接从 **本次会话内的房间动作响应** 取 roomId —— `POST /rooms` 创建房间响应的 `data.room.id`、`POST /rooms/{roomId}/join` 加入房间响应的 `data.roomId`、用户点击外部分享链接 / 主动输入 roomId 等显式输入路径，建连前 client 内存中已经持有 roomId
  - **冷启 / token 刷新 reconnect（Epic 12 reconnect 场景）**：client **本地持久化**最近一次成功 WS 连接的 roomId（iOS Story 12.5 实装：UserDefaults / Keychain 存最近一次 `roomId`，由"成功握手并接受 `room.snapshot`"事件触发写入；用户主动 `POST /rooms/{roomId}/leave` 或服务端 close 4003 / 4004 时清除）。app 冷启 / token 刷新后从本地读取，发起 WS 连接验证：服务端按 §12.1 校验顺序判定，4003 = 用户已不在该房间（按 close code 表回退到主界面），4004 = 房间已不存在（同样回退）
  - **未来增强**：节点 4 之后 Story 11.10 真实实装 `GET /home.room.currentRoomId`（详见 §5.1 `GET /home` 字段表 + Future Fields）后，client 可以用 server 端权威来源**替代**本地持久化作为冷启场景的 roomId 来源；在此之前，**禁止**依赖该字段，因为它返回 `null`
- `token`：query 参数，URL-encoded（必须 url-encode，避免 token 中 `+` `/` `=` 等字符影响 query 解析）；token 来自 §2.3 Bearer token 同源（`POST /auth/guest-login` 返回的 `token`）

兼容性 / 简化形式（保留旧 README 中的写法，等价于上方完整模板）：

```text
GET /ws/rooms/{roomId}?token=xxx
```

> 兼容性提示：上述裸 URL 与"完整 URL 模板"小节是同一契约的两种书写，二者并存不冲突；以"完整 URL 模板"+ 字段说明为字段层锚定权威。

服务端连接建立后需要校验：

- token 合法（解码 + 签名校验 + 未过期）
- 用户确实在该房间中（查 `room_members` 表，节点 4 阶段 Story 10.3 实装查询路径）

成功后返回房间快照。

#### 握手成功流程

成功握手后服务端必须**主动**推送：

1. **第一条**：`room.snapshot`（payload schema 见 §12.3，节点 4 阶段 going-forward 契约：room 三字段 + members 数组反映 `room_members` 全部成员行 + LEFT JOIN `pets` 取 pet 真实状态 —— `nickname` 在 placeholder 阶段（Story 10.7）允许空字符串（不 JOIN `users`），`pet` 整体 placeholder 与真实阶段一致：pet-less 下发 `null`、否则 `petId` 为 `pets.id` 字符串化真实值；详见 §12.3 placeholder 示例与字段表 placeholder 行为；Story 10.7 落地实装的过渡兼容形态见 §12.3 "Story 10.7 落地实装与 going-forward 契约的差异" 段）

> **权威性 + Merge 语义（client 必读）**：`room.snapshot` 是握手成功后**必发**的第一条 authoritative 消息，但其权威性是 **enrich/correct** 而**非** wipe-out —— client **必须**对每个 `members[]` entry 做**字段级 merge**（详见 §12.3 "client merge contract" 小节）。简言之：snapshot 中**非空**字段覆盖 client 已有值（这是真实 authoritative 数据）；snapshot 中**空字符串**字段（`""`）保留 client 已有值（空字符串 = "我不知道这个值" 的 placeholder 信号，**不是** "请清空")；snapshot 中**未出现**的字段保留 client 已有值。该契约让 Story 10.7 placeholder 阶段（部分字段空）和 Story 11.7 真实阶段（全字段非空）行为一致：snapshot 永远只能 enrich/correct client state，**禁止** wipe out client 已通过 §15.6 推荐流程从 `GET /api/v1/rooms/{roomId}` 加载的真实丰富字段。

> **Epic 10 阶段**（即本 story 范围 / Story 10.1 ~ 10.7）服务端 → 客户端**只**会主动发送 `room.snapshot` / `pong` / `error` 三种消息（与本节末 §12.3 业务消息延后锚定块 + §1 节点 4 协议骨架冻结声明一致）；`member.joined` 等业务广播消息的字段层契约与"是否在握手时广播"的语义由 Story 11.1（Epic 11）锚定 —— 即 Epic 10 阶段服务端**不**在握手时广播 `member.joined`，但 Epic 11 起按对应 story 锚定的语义扩展消息集合（Story 11.1 锚定 `member.joined` / `member.left` 加入消息集，Story 14.1 锚定 `pet.state.changed`，Story 17.1 锚定 `emoji.received`）；本声明仅冻结 Epic 10 阶段的 server-active message set，不阻止后续 epic 的合法扩展。

服务端不要求 client 在握手后发送任何"我准备好了"的初始化消息，握手成功 = 服务端可以推快照。

时序图见 `docs/宠物互动App_时序图与核心业务流程设计.md` §13.2（本 story **不**重画时序图，只引用）。

#### close code 表

握手 / 连接期间任一校验失败，服务端必须主动 close 连接，并使用以下 close code：

| close code | 由谁产生 | 触发条件 | 服务端是否带 reason | 客户端推荐处理 |
|---|---|---|---|---|
| 4001 | server 主动 close | token 缺失 / 解码失败 / 签名不匹配 / token 已过期 | 是（reason = `"invalid token"` / `"token expired"`） | 触发"无效 token 静默重新登录"流程（参考 iOS Story 5.4）：清 keychain 旧 token → guest-login 拿新 token → 重连；**不**自动无限重试 |
| 4003 | server 主动 close | token 合法但 user 不在该 roomId 的 `room_members` 表中 | 是（reason = `"user not in room"`） | **不**自动重连（业务级拒绝，重试无意义）；UX 提示用户"未加入该房间"，回退到主界面 |
| 4002 | server 主动 close | roomId 路径参数格式错（非数字 / 缺失） | 是（reason = `"invalid roomId"`） | **不**自动重连；视为客户端实装 bug，记 log 后回退 |
| 4004 | server 主动 close | room 不存在（`rooms` 表无该 ID 或已 archived） | 是（reason = `"room not found"`） | **不**自动重连；UX 提示"房间不存在"，回退到主界面 |
| 4005 | server 主动 close | 心跳超时（60 秒未收到任何 client 消息含 ping，由 Story 10.4 心跳框架触发；详见 §12.2 `ping` 小节） | 是（reason = `"heartbeat timeout"`） | **应**自动重连（指数退避；视为 transient network failure，与 1006 / 1011 同等对待 —— 不视为业务级拒绝，因为底层多半是网络抖动 / 客户端切后台超时）；reconnect 自洽性 by §10.3 "roster 语义与 WS 断线交互" + §10.5 "WS 断线场景与本接口的关系"：心跳超时**不**删 `room_members` 行 / **不**广播 `member.left`，仅清 ephemeral 层 → reconnect 握手时 §12.1 校验顺序步骤 5（`room_members` 表查询）通过，用户保留原房间座位 |
| 1000 | server 或 client 任一方主动 close | 服务端 / 客户端正常关闭 | 否 | 客户端主动 close 不重连；服务端主动 close（如 graceful shutdown）客户端可选重连 |
| 1001 | server 或 client 任一方主动 close | going away（服务端重启 / 客户端切后台） | 否 | 服务端重启 → 客户端**应**自动重连（指数退避，参考 iOS Story 12.5）；客户端切后台 → 客户端自身决定 |
| 4006 | server 主动 close | 客户端违反协议策略 —— 节点 4 阶段唯一触发条件：单条消息 frame 超过 `ws.max_message_size_bytes`（默认 16 KB） | 是（reason = `"message too large"`） | **不**自动重连（视为客户端实装 bug，重连仍会被 close）；记 log error 后回退 |
| 4007 | server 主动 close | leaver 通过 HTTP `POST /rooms/{roomId}/leave` 主动离开房间，且该 leaver 仍持有同一 `roomId` 的 WS 连接 —— server 在 leave 事务成功提交后**先**触发本 close（unregister + close 4007），**后**广播 `member.left`，从而 fanout 时 leaver Session 已不在 SessionManager 列表（r13 锁定）；详见 §10.5 服务端逻辑步骤 7 | 是（reason = `"left room via HTTP"`） | **不**自动重连（业务级拒绝，leaver 已主动离开房间，重连会因 §12.1 校验顺序步骤 5"用户房间归属校验"失败被 close 4003）；**4007 是 best-effort cleanup signal，不是 leave 完成的 authoritative confirmation** —— **HTTP `POST /rooms/{roomId}/leave` 的 200 响应（`data.left: true`）是 leave 完成的唯一权威信号**（§10.5 步骤 9 锚定），client **必须**以 HTTP 200 为推进 RoomView 退出 / 清房间状态的唯一触发点，**禁止**等待 4007 才 tear down（因 HTTP 与 WS 走两条独立连接，无 ordering / delivery 保证 —— leaver WS 早断 / 4007 比 HTTP 晚到 / 4007 丢包均合法）；client 收到 4007 时仅作**冗余 UX 辅助信号**：若已通过 HTTP 200 完成 RoomView 退出 → noop；若未收到 HTTP 200（极端场景如 HTTP 请求超时但 server 已成功处理）→ 作 fallback 触发 RoomView 退出 / RoomViewModel 清理 UX；client 实装层细节由 iOS Story 12.7 LeaveRoomUseCase 锚定 |
| 1011 | server 主动 close | 服务端内部错误（panic / 不可恢复异常）；**含**握手完成后 SnapshotBuilder 构建初始 `room.snapshot` 失败的场景（reason = `"snapshot build failed"`） —— 因为 §12.1 钦定 `room.snapshot` 是握手成功后必发的第一条消息，构建失败若不 close 而仅推 `error`，client 会永远等待一个永不到达的 snapshot，房间页无法初始化 | 是（reason 应包含简短错误提示但**不**泄漏 stack trace） | 客户端**应**自动重连（指数退避，但限制最大重试次数避免雪崩） |

**关键约束**：

- 4001 / 4002 / 4003 / 4004 / 4006 / 4007 是**业务级**拒绝（4xxx 段是应用自定义；WebSocket RFC 6455 规定 4000-4999 为应用保留段），重试无意义，客户端**不**应自动重连；客户端应展示明确 UX 提示并回退（4006 = 客户端实装 bug，记 log error 后回退；4007 = server 端 best-effort cleanup 信号，仅作 fallback UX 辅助 —— **leave 完成的 authoritative signal 是 HTTP 200 响应**，client **禁止**等 4007 才推进 RoomView 退出，详见 4007 行 client 行为指引）
- **4005 是 4xxx 段中的例外**：虽然位于 4xxx 应用自定义段，但语义是 transient network failure（心跳超时多半是网络抖动 / 客户端切后台），不是业务级拒绝；客户端**应**自动重连，与 1006 / 1011 同等对待（指数退避 + 最大重试次数限制）。该 retryable 语义的可行性由 §10.3 / §10.5 / §12.3 钦定共同保证："WS 断开（含心跳超时）仅清 ephemeral 层；持久层 `room_members` / `users.current_room_id` 仅由 HTTP join / leave 改变" —— 因此 4005 reconnect 握手时 `room_members` 行必然仍在，§12.1 校验顺序步骤 5 通过，client 重新进入原房间无需重新 join
- 1000 / 1001 / 1011 是**协议 / 网络**级断开（1xxx 段是 RFC 标准段，可由服务端主动 emit），客户端**应**自动重连（除 1000 主动关闭外）
- **不使用 RFC close code 1006 / 1008 / 1009**（**不**出现在上方 close code 表内，**也不**由服务端主动 emit，原因分两类）：
  - **1006**：RFC 6455 §7.1.5 reserved status code —— **MUST NOT** be set as a status code in a Close control frame。1006 仅由客户端 WebSocket runtime 在底层 TCP 异常断开 / 网络抖动且未收到 close frame 时**本地合成**通知上层；服务端实装层（Story 10.3 / 12.5）**禁止**写 1006 到 close frame。客户端收到 1006 时按 transient network failure 处理 —— 与 1011 / 4005 同等对待（指数退避 + 最大重试次数限制自动重连）。1006 不进入 close code 表的另一个理由：§3 全局错误码表已将 `1006`（状态不允许当前操作）用作应用层错误码（在 §12.3 `error.payload.code` 中出现），与其它"1xxx 段被 §3 占用"的 code 同类，避免数字空间冲突
  - **1008 / 1009**：RFC 6455 §7.4.1 分别定义 1008 (Policy Violation) / 1009 (Message Too Big) 为标准 close code，但本协议**禁止**这两个 1xxx 段值用于 close frame，原因是 §3 全局错误码表已将 `1008`（幂等冲突）/ `1009`（服务繁忙）用作应用层错误码（在 §12.3 `error.payload.code` 中出现）。"消息超大"场景统一改用 **4006**（4xxx 应用自定义段，与 4001-4005 同段位，数字空间不与 §3 重叠）；其他 policy violation 场景同样走 4xxx 段
  - **共同根因**：同一数值同时出现在 close frame 和 application error frame 会让客户端、日志、监控无法仅凭数字区分 transport-level fatal 与 application-level transient/retryable（前者要 close + 不重连或固定重连分类，后者要忽略 + 保连接）。因此 §3 已占用的 1xxx 段值（含 1006 / 1008 / 1009）一律**禁止**作为 close code；服务端主动 emit 的 close code 限定在 1000 / 1001 / 1011 + 4001 / 4002 / 4003 / 4004 / 4005 / 4006 / 4007 这 10 个值内
- 4001 触发时 server **不**写 log error（这是常态，token 过期是正常业务），写 log info；4003 / 4004 写 log warn（疑似客户端实装 bug 或数据不一致）；4005 写 log info（这是常态，心跳超时多半是网络抖动 / 切后台；写 warn 会让正常网络抖动场景下日志噪声暴涨）；4006 写 log error（必排查，疑似客户端 bug 或恶意流量）；4007 写 log info（这是常态，用户主动 leave 的协议确认）；1011 写 log error（必排查）

#### 服务端校验顺序

握手期间服务端必须按以下顺序校验，任一步失败立即 close 并使用对应 close code：

1. **解析 query**：缺 `token` 参数 → close 4001（reason = `"missing token"`）
2. **路径参数校验**：`roomId` 非数字 / 缺失 → close 4002
3. **token 校验**（不查 DB，仅本地校验签名 + 过期，复用 Story 4.4 token util）：失败 → close 4001
4. **room 存在性校验**（查 `rooms` WHERE `id = ?`）：失败 → close 4004
5. **用户房间归属校验**（查 `room_members` WHERE `user_id = ? AND room_id = ?`）：失败 → close 4003
6. **内部错误**（panic / DB 异常等）：close 1011

校验通过后**严格按以下顺序**执行（顺序由 §12.1.3"`room.snapshot` 是握手成功后必发的**第一条** authoritative 消息"契约钦定，**禁止**调换）：

1. 创建 Session 对象（详见 Go 项目结构 §9.1）
2. 注册到 SessionManager
3. **同步**构建并发送 `room.snapshot`（§12.3 schema） —— 必须在握手响应路径里**同步**写入 underlying `*websocket.Conn`（或 enqueue 到 send buffer 后**flush**完成）；**不**通过"投递到尚未启动的写 goroutine 队列"完成，因为本步骤完成时写 goroutine 尚未启动，消息将永不被发送
4. 启动读 / 写 goroutine —— 此步骤**必须**在第 3 步 snapshot 写入完成（write call 返回 nil error）之后才启动；snapshot 写失败时**不**启动 goroutine，按 §12.1 close code 表 1011 行（reason = `"snapshot build failed"`）close 连接
5. 在 Redis presence 记录在线（详见 Story 10.6）

> **顺序 rationale**：若先启动读 goroutine 再推 snapshot，client 在 upgrade 完成的下一个 tick 即可发送合法 `ping`（client 不知道 server 还没推过 snapshot），server 写 goroutine 已经启动 → 直接回 `pong`，client 收到 `pong` 在 `room.snapshot` 之前，违反 §12.1.3 "first must be snapshot" 契约；尤其是 SnapshotBuilder 命中 DB 慢路径 / 锁等待场景下窗口被放大。把 snapshot 写入放在 goroutine 启动**之前**的同步段执行 = 物理上保证 snapshot frame 必定先于任何"对 client 消息的响应 frame"出现在 wire 上（写 goroutine 此时还不存在，无法产生任何竞争性写入）。

**注意**：第 5 步是**协议层面强制的授权环节**，必须以**持久化 membership 数据**（即 `room_members` 表）为 single source of truth —— **禁止**使用 Redis presence 替代该校验。理由：presence 仅表示 ephemeral 在线态，不代表 user 是房间合法成员；以下两种常见场景下，"用 presence 做 membership 校验"会错误返回"非成员"并 close 4003：(a) server 重启 / Redis 清空后第一个合法成员重连（presence 全空）；(b) 合法成员的 presence entry TTL 过期后重连。如未来需要为该热路径降低 MySQL 读压（Story 10.6 / 11.4 实装阶段评估），**必须**引入与 presence 区分的"durable membership cache"（持久层语义、由加入 / 退出房间事务原子失效），而**不是**复用 presence；本协议层面的 §12.1 校验顺序不因任何缓存方案而改变。

---

## 12.2 客户端 -> 服务端消息

#### 通用消息信封（client → server）

所有客户端 → 服务端 WS 消息共享同一信封结构：

| 字段 | 类型 | 必填 | 范围/约束 | 说明 |
|---|---|---|---|---|
| `type` | string | 必填 | 1 ≤ length ≤ 64 字符；只允许 `[a-z0-9.]` | 消息类型，全小写点分（如 `"ping"` / `"emoji.send"`）；服务端按 `type` 路由到对应 handler |
| `requestId` | string | 选填 | 0 ≤ length ≤ 64 字符；客户端定义格式（推荐 `<short-prefix>_<timestamp_ms>` 或 UUID） | 客户端生成的请求 ID，用于追踪 / 日志关联；服务端**响应**当前请求时**应**带回同一 `requestId`（如 `pong` 回带 `ping` 的 `requestId`）；**广播消息**（如 `emoji.received`）的 `requestId` 由服务端置空字符串 `""` |
| `payload` | object | 必填 | 任意 JSON 对象；具体 schema 由 `type` 决定 | 业务负载；空 payload 也必须**显式**写 `"payload": {}`，**不**省略 key |

JSON 通用骨架：

```json
{
  "type": "<message_type>",
  "requestId": "<optional_client_id>",
  "payload": { "...": "..." }
}
```

**关键约束**：

- 信封**不**带 `ts` 字段（client → server 方向不需要 client 时间戳；如某业务消息需要 client 时间戳，由该业务消息 payload 自带，如 `emoji.send.payload.clientTs`，详见 Story 17.1）
- `requestId` **不**进 server 持久化日志的关键字段，仅用于本次连接 session 内的 request-response 配对（避免无意义的 cross-session 追踪）
- `type` 字段值大小写敏感：服务端 strict match；客户端**必须**按文档锚定的字面量发送（如 `"ping"` 不是 `"PING"` 或 `"Ping"`）
- 消息 frame 必须是 WebSocket text frame（opcode 0x1）+ UTF-8 JSON；**不**使用 binary frame（opcode 0x2）
- 单条消息 frame **大小上限 16 KB**（默认值；配置 `ws.max_message_size_bytes`，**prod 必须使用默认值不可覆盖**，dev/test 可覆盖）；超限服务端 close **4006**（policy violation, 4xxx 应用自定义段，reason = `"message too large"`） + 不重连（客户端实装 bug）。**不使用 RFC 1008 / 1009**：本协议 §3 全局错误码表已将 `1008`（幂等冲突）/ `1009`（服务繁忙）用作应用层错误码（在 §12.3 `error.payload.code` 中出现），为避免 close frame code 与 application error code 数字冲突，"消息超大"统一走 4006（与 4001-4005 同段位，数字空间不与 §3 重叠）；详见 §12.1 close code 表关键约束

### 发送表情

**触发**：

- iOS 客户端在房间页面用户选中表情面板（Story 18.1）中某个表情时，由 SendEmojiUseCase（Story 18.3）通过已建立的 WebSocket 连接（Story 12.2 WebSocketClient）发送 `emoji.send` text frame；client 在发送的**同时**触发本地立即动效（不等 server `emoji.received` 回信），与 server 广播解耦
- 节点 6 阶段**仅** Story 18.3 一处触发；其他客户端流程（如自动重连 / 心跳）**都不**触发 `emoji.send`

**字段**（基于 §12.2 通用消息信封 + 业务字段）：

| 字段 | 类型 | 必填 | 范围/约束 | 说明 |
|---|---|---|---|---|
| `type` | string | 必填 | 固定值 `"emoji.send"` | 消息类型，遵循 §12.2 通用信封约束（全小写点分 + `[a-z0-9.]`） |
| `requestId` | string | 选填 | 0 ≤ length ≤ 64 字符 | client 生成的请求 ID，遵循 §12.2 通用信封约束；server 处理失败时（如 7001 / 6004）回 `error` 消息**回带**该 `requestId`；server 处理成功后广播的 `emoji.received` **不**回带 `requestId`（广播类消息固定 `""`，详见 §12.3 通用信封）；client 端推荐格式 `"emoji_<seq>"` / `"emoji_<ts_ms>"`，方便与 `error` 响应配对 |
| `payload.emojiCode` | string | 必填 | 1 ≤ length ≤ 64 字符；只允许 `[a-z0-9_-]`（与 §11.1 `data.items[].code` 同语义 + 数据库 `emoji_configs.code` 同语义） | 客户端选中的表情业务标识符；server 校验该 `emojiCode` 必须在 `emoji_configs` 中存在 + `is_enabled=1`，否则回 `error.payload.code = 7001`（见下方"错误响应"段） |

JSON 示例（与本节字段表对齐）：

```json
{
  "type": "emoji.send",
  "requestId": "msg_001",
  "payload": {
    "emojiCode": "wave"
  }
}
```

> **注**：本消息走 §12.2 client → server 通用信封（**不**带 `ts` 字段 —— 客户端发起方向不需要 client 时间戳，详见 §12.2 通用消息信封字段表 + 注），且 `payload` 仅 `emojiCode` 一个字段（保持最小契约）。

**服务端逻辑**（Story 17.5 实装锚定）：

1. **接收 & 解析**：server WS dispatcher（Story 10.3）按 `type = "emoji.send"` 路由到 EmojiHandler；解析 `payload.emojiCode` 字段
2. **参数校验**：`emojiCode` 必填 + 类型为 string + 1 ≤ length ≤ 64 + 字符集 `[a-z0-9_-]`；任何不通过 → 回 `error.payload.code = 1002` "参数错误"（**响应类** error，`requestId` 回带 `emoji.send.requestId`），**不**广播
3. **房间归属校验**（**权威源 = 收到 `emoji.send` frame 的 WS Session 上携带的 `roomID`** —— 该 `Session.roomID` 由 §12.1 握手 path `/ws/rooms/{roomId}` 写入、并已在 §12.1 校验顺序步骤 5 通过 `room_members WHERE user_id=? AND room_id=?` 校验，**是协议层钦定的"本次 WS 连接所属房间"权威源**；详见 §9.1 `Session` 字段表 + §12.1 校验顺序步骤 4 / 5）：`SELECT current_room_id FROM users WHERE id = ?`（当前 user.id 来自 WS 握手 token；查得后**必须**与 `Session.roomID` 比对 —— **不可仅判 `!= NULL`**，否则 stale Session 跨房间注入风险无法封堵，详见本步骤末"r1 review 锁定的反 stale-Session 校验" 注）；**DB 读取失败**（`err != nil`，含 driver / 网络 / 慢查询超时）→ 回 `error.payload.code = 1009` "服务繁忙"（**响应类** error，`requestId` 回带 `emoji.send.requestId`），**不**广播 + **不**关闭 WS 连接（仅回 error 消息）
   - **`current_room_id == NULL`**（用户当前不在任何房间 —— 与 §10.4 / §10.5 房间归属同义）→ 回 `error.payload.code = 6004` "用户不在房间中"（**响应类** error，`requestId` 回带 `emoji.send.requestId`），**不**广播
   - **`current_room_id != NULL` 但 `current_room_id != Session.roomID`**（用户在房间 A 持有 stale Session、`users.current_room_id` 已切到房间 B —— 来源场景：(a) §10.5 leave 事务步骤 7 的 close 4007 是 best-effort cleanup，4007 frame 写入失败 / 客户端尚未读到时 leaver Session 在内存中残留；(b) 多设备 / 跨标签场景：用户 A 在设备 1 持房间 X Session、在设备 2 通过 HTTP join 房间 Y 后 `users.current_room_id` = Y、设备 1 Session 未收到任何 close 仍在；(c) 客户端实装 bug 持 stale socket 继续发送）→ 回 `error.payload.code = 6004` "用户不在房间中"（**响应类** error，`requestId` 回带 `emoji.send.requestId`），**不**广播；同时 server **应** log warn 级别记录该跨房间发送企图（含 `userId` / `Session.roomID` / `users.current_room_id`），便于排查多设备 stale Session 残留
   - **`current_room_id != NULL` 且 `current_room_id == Session.roomID`** → 记录 `currentRoomId = Session.roomID` 进入步骤 4

   > **r1 review 锁定的反 stale-Session 校验**：仅判 `current_room_id != NULL` **不足以**保证 `emoji.send` 广播范围正确。`GET /ws/rooms/{roomId}` 的 path 已经决定了"本次 WS 连接所属房间"，但 `users.current_room_id` 可在 WS 连接生命周期内被 HTTP `POST /rooms/{roomId}/leave` + `POST /rooms/{otherRoomId}/join` 改变；若仅以 `users.current_room_id` 为广播目标，stale Session（房间 A）发来的 `emoji.send` 会被广播到当前 `users.current_room_id` = 房间 B，造成**跨房间消息注入**。该校验改用 `Session.roomID` 作为广播目标 + 同时比对 `users.current_room_id == Session.roomID`，与 §10.4 / §10.5 HTTP 房间接口 ACL 行为对齐（HTTP 路径用 path roomId 校验，WS 路径同样用握手 path roomId 校验）。
4. **表情合法性校验**：`SELECT 1 FROM emoji_configs WHERE code = ? AND is_enabled = 1`（按入参 `emojiCode` 查）
   - **DB 读取失败**（`err != nil`，含 driver / 网络 / 慢查询超时）→ 回 `error.payload.code = 1009` "服务繁忙"（**响应类** error，`requestId` 回带 `emoji.send.requestId`），**不**广播 + **不**关闭 WS 连接
   - **0 行**（`emojiCode` 不存在 / 或存在但 `is_enabled=0` —— 两种情况合并为同一错误，避免 server 暴露 enabled / disabled 状态信息）→ 回 `error.payload.code = 7001` "emoji not found"（**响应类** error，`requestId` 回带 `emoji.send.requestId`），**不**广播
   - **1 行** → 进入步骤 5
5. **广播（fire-and-forget）**：调用 `BroadcastToRoom(Session.roomID, {type: "emoji.received", payload: {userId, emojiCode}})`（**广播目标 = `Session.roomID`，不是 `users.current_room_id`** —— 二者在步骤 3 已校验相等，此处显式以 `Session.roomID` 为参表达"本次 WS 连接所属房间是广播权威源"，详见 §12.3 `### 收到表情广播`）；广播失败仅 log warning，**不**回 error 给发起者（与 Story 11.8 `member.joined` / Story 14.4 `pet.state.changed` 广播失败语义一致；**注**：与 `pet.state.changed` 的区别 —— `pet.state.changed` HTTP 路径仍返回 200 OK；`emoji.send` 是 WS 路径，**无 HTTP 响应、无 server → client ack 消息**，server 端"成功"= 仅完成上述步骤 1-5 + 广播尝试本身（广播是否真正送达任何接收者，包括发起者自己，**不**影响"成功"判定）；**本契约不提供"server 已接受 emoji.send"的 client 可观测信号**：self-broadcast `emoji.received` **不**承担 ACK 职责 —— 由于 fanout fire-and-forget 允许发起者自己的 Session 漏收 self-broadcast（见下文"广播失败容忍"），任何"以 self-broadcast 到达视为 server ACK"的 client / 测试假设都会在 self-Session 单腿丢包时假阴性；与本接口"emoji 是 transient UI 事件、本地动效已是发起者主要 UX 反馈、18.3 钦定本地动效立即播放不等 server"的设计一致；如未来确需 server → client ack 信号，视为契约变更）
6. **不入库**：表情事件**不**持久化到任何表（按数据库设计 §14.3，MVP 不强制落库表情事件日志；与 `pets.current_state` UPDATE 持久化的 `pet.state.changed` 语义**不同**）

**WS 广播 vs 客户端本地动效的关系（含发起者自己的 self-broadcast 去重规则）**：`emoji.send` 是 client → server 单向消息，**无**像 HTTP 那样的 server response 信号；server 处理后通过 `emoji.received` 广播事件通知（包含发起者自己）。client 端发出 `emoji.send` 后**应**立即在本地触发自己的动效（不等 server 广播 —— iOS Story 18.3 钦定），后续 server 广播到达时按以下规则去重：

- **对"别人的表情"的权威信号**：以 WS 广播 `emoji.received` 为**唯一**信号（与 CLAUDE.md §"工作纪律 / 状态以 server 为准"一致）；client **不**通过任何其他渠道驱动别人的动效
- **对"发起者自己的表情"的去重规则（与 §5.2 `pet.state.changed` self-broadcast 兜底规则**语义不同**）**：考虑到表情是 **transient UI 事件**（server 不持久化、UI 上动效结束即消失、状态对齐不存在"stale 风险"），client **应**对 `emoji.received.payload.userId == 当前 user.id` 走"跳过 / 不重复触发动效"路径（iOS Story 18.4 钦定）。理由：(a) 本地动效已在 SendEmojiUseCase 触发瞬间播完；(b) 重复触发会让发起者看到"两次同一动效"，UX 差；(c) 与 `pet.state.changed` self-broadcast **必须接收并 merge** 的语义**不同** —— `pet.state.changed` 是持久化状态变更，self-broadcast 是双轨之一防 stale；`emoji.received` 是 transient 事件，本地动效已是完整 UI 体验，self-broadcast 仅作为 server 端 "广播路径完整执行了" 的探测信号（client 端 log debug 即可，不驱动 UI）
- **广播失败容忍**：如 `emoji.received` 广播因网络抖动 / Session 已 close 失败（含发起者自己 Session 失败 → 自己收不到 self-broadcast），fire-and-forget 仅 log warning；client 端不应假设每条 `emoji.send` 都能 server-acknowledged via self-broadcast，本地动效是发起者的**主要** UX 反馈（与 18.3 钦定一致）；网络极差时 server 也可能根本没收到 `emoji.send`（WS 连接断开）→ 同样仅本地动效 + 18.3 钦定弹温和 toast 提示"网络不佳，对方可能看不到"（toast 不阻塞 UI）

**错误响应**：

| code | message | 触发条件 | 失败后 client 推荐处理 |
|---|---|---|---|
| 1002 | 参数错误 | `payload.emojiCode` 字段缺失 / 类型非 string / length 不在 1 ~ 64 / 字符不在 `[a-z0-9_-]`（与 §3 错误码表 1002 一致） | client 端代码 bug（按 §11.1 缓存的表情列表选取，不应触发）；不重试，log error 上报 |
| 6004 | 用户不在房间中 | service 层步骤 3 查到 (a) `users.current_room_id == NULL`，**或** (b) `users.current_room_id != NULL` 但 `users.current_room_id != Session.roomID`（即"用户已切到别的房间但仍在本 WS Session 上发送" stale-Session 跨房间企图，详见步骤 3 r1 review 锁定的反 stale-Session 校验）；两类合并报同一 `6004`，避免 server 暴露具体房间归属细节（与 §3 错误码表 6004 一致；该错误码原由 §10.4 / §10.5 房间 HTTP 接口触发，本接口是 WS 路径首次复用） | client 端房间状态与 server 不一致（本地以为在该房间，server 已踢出 / 已切到别的房间）；UX 提示用户"未在房间中"，回退到主界面 + 刷新 GET /home 状态；不重试本次发送 |
| 7001 | emoji not found | service 层步骤 4 查到 `emojiCode` 不在 `emoji_configs` / 或存在但 `is_enabled=0`（与 §3 错误码表 7001 一致） | client 端缓存的表情列表与 server 不一致（如 admin 临时下架某表情但 client 未刷新缓存）；建议清除本地表情缓存 + 下次表情面板打开时重新 GET /emojis；本次发送已失败（不重试） |
| 1009 | 服务繁忙 | service 层步骤 3 / 步骤 4 DB 读取失败（`SELECT current_room_id FROM users` / `SELECT 1 FROM emoji_configs` 返回 `err != nil`，含 driver / 网络 / 慢查询超时等）/ 内部 panic（与 §3 错误码表 1009 一致 + §11.1 同语义；server **不**因此错误关闭 WS 连接，仅回 `error` 消息，连接保留以便 client 继续后续操作） | 服务端瞬时故障（DB / 网络抖动）；client 端按 §3 错误码表通用规则处理（提示"服务繁忙，稍后重试"toast，不主动重试本次发送 —— 避免雪崩；WS 连接不主动断开） |

错误响应通过 §12.3 `error` 消息回送，遵循 `error` 消息字段表（`type: "error"` / `requestId` 回带 `emoji.send.requestId` / `payload.code` / `payload.message` / `ts`）。

JSON 示例（7001 错误响应）：

```json
{
  "type": "error",
  "requestId": "msg_001",
  "payload": {
    "code": 7001,
    "message": "emoji not found"
  },
  "ts": 1776920345000
}
```

JSON 示例（6004 错误响应）：

```json
{
  "type": "error",
  "requestId": "msg_001",
  "payload": {
    "code": 6004,
    "message": "用户不在房间中"
  },
  "ts": 1776920345000
}
```

**关键约束**：

- **client 端发送约束**：iOS client 在调用 `WebSocketClient.send` 前**应**校验 `emojiCode` 来自 §11.1 缓存的合法表情列表（取 `data.items[].code` 字段值），**禁止**直接 hardcode `emojiCode` 字面量（避免 client / server 不同步导致 7001 频发）；如 §11.1 GET /emojis 尚未拉取过 → client 应先拉取再启用表情面板（与 iOS Story 18.1 表情列表缓存契约一致）
- **房间归属校验顺序 + 权威源**：server 端先做参数校验（1002）再做房间归属校验（6004）再做表情校验（7001）—— 这是契约层钦定的校验顺序，避免出现"参数不合法但报房间问题"误导 client；server 实装层（Story 17.5）严格按此顺序。**房间归属权威源 = `Session.roomID`（握手 path 写入，§12.1 步骤 5 已校验），不是 `users.current_room_id`**：步骤 3 必须比对 `users.current_room_id == Session.roomID`，仅判 `!= NULL` 不足以封堵 stale-Session 跨房间注入风险（详见 r1 review 锁定的反 stale-Session 校验注），与 §10.4 / §10.5 HTTP 房间接口"用 path roomId 校验" ACL 语义对齐
- **不限频**：节点 6 阶段 server **不**对 `emoji.send` 做特殊限频（仅走 §12.1 / Story 10.4 心跳层 + Story 4.5 通用 rate_limit 中间件按 user_id 每分钟 60 次默认 —— **注**：rate_limit 中间件挂在 HTTP 路由，**不**挂 WS 路由，故 `emoji.send` 实际**不**走 1005 限频拦截；如需限频，由 future tech debt 处理）；epics.md §Story 17.5 已钦定"限频建议：同一用户每秒最多 5 个表情（防止刷屏，可在 Story 4.5 rate_limit 基础上扩展）—— MVP 可不做，但 tech debt 登记"，本契约层**不**预留限频错误码字段
- **不入库**：表情事件**不**持久化（按数据库设计 §14.3）—— 与 `pet.state.changed` 持久化到 `pets.current_state` 不同；reconnect 后**不**重放历史表情事件（无历史可重放），与 `room.snapshot` 不下发"近期表情列表"语义一致
- **client → server active message set 升级**：自本 story 起，`emoji.send` 正式加入 **client → server active message set**（继 Epic 10 `ping` 之后的首次扩展，详见 §12.3 末尾"server / client active message set 升级"块）

> **业务消息延后锚定**：上文 `### 发送表情`（`emoji.send`）已由 **Story 17.1** 锚定字段表 + 触发条件 + 关键约束 —— 自此**接入** Epic 17 / 节点 6 client → server active message set（具体升级见 §12.3 末尾"server / client active message set 升级"块）。
>
> **节点 6 阶段 client → server active message set**：`ping`（Epic 10 锚定）+ **`emoji.send`（本 story 锚定）**；其他 `type` 值 client **不**应发送，server 收到非 active set 内消息当前阶段**忽略**（log warn + 不 close 连接，避免兼容性问题；具体行为 Story 17.5 实装时锁定，与 Story 10.3 既有规则一致）。

### ping（心跳）

**心跳间隔**：客户端**应**每 30 秒发送一次 `ping`（默认值；配置 `ws.heartbeat_interval_sec` 在 client 侧）；服务端 60 秒未收到任何消息（含 ping）→ Session 被 Story 10.4 心跳框架自动清理 + close **4005**（reason = `"heartbeat timeout"`），客户端**应**自动重连（指数退避，参考 iOS Story 12.5；视为 transient network failure，与 1006 / 1011 同等对待）。该 code 由本 story 锚定，Story 10.4 实装时**必须**使用，**不**可改用其他 code。

**字段**：本消息无业务字段，`payload` 为空对象 `{}`。

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | string | 必填 | 固定值 `"ping"` |
| `requestId` | string | 选填 | 客户端可生成（如 `"ping_<seq>"` / `"ping_<ts_ms>"`），服务端 `pong` 必须回带；省略 → server `pong.requestId = ""` |
| `payload` | object | 必填 | 固定空对象 `{}` |

JSON 示例：

```json
{
  "type": "ping",
  "requestId": "ping_001",
  "payload": {}
}
```

服务端响应：见 §12.3 `pong` 消息。

---

## 12.3 服务端 -> 客户端消息

#### 通用消息信封（server → client）

所有服务端 → 客户端 WS 消息共享同一信封结构：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | string | 必填 | 消息类型（如 `"room.snapshot"` / `"pong"` / `"error"`）；客户端按 `type` 路由解析 |
| `requestId` | string | 必填 | **响应类**消息（如 `pong`，及作为某 client 请求响应的 `error`）回带 client 请求的 `requestId`（client 未提供 → 回 `""`）；**广播 / 主动推送类**消息（如 `room.snapshot` / `member.joined` / `emoji.received`，及 server 主动产生的 `error`）固定 `""`；**不**省略 key（即使值为空）；`error` 的双重语义详见 §12.3 `error` 小节字段表 |
| `payload` | object | 必填 | 业务负载；空 payload 也必须**显式**写 `"payload": {}` |
| `ts` | number (int64) | 必填 | 服务端发送时的 Unix 毫秒时间戳，> 0；客户端用于本地排序 / 日志，**不**用于业务时序判断 |

JSON 通用骨架：

```json
{
  "type": "<message_type>",
  "requestId": "<optional_request_correlation_or_empty>",
  "payload": { "...": "..." },
  "ts": 1776920345000
}
```

**关键约束**：

- server → client 信封**多**一个 `ts` 字段（client → server 没有），用途是客户端日志关联 + UI 排序（如多条 `member.joined` 接连到达时按 `ts` 排序避免乱序）；**不**用作业务时序判断（业务时序仍由 server 单调时钟保证；client 不应基于 `ts` 比较推断"谁先发生"，因为不同 server 实例时钟可能漂移 —— 节点 4 单实例无此风险，但接口契约层面提前规避）
- `requestId` 在响应类消息中是**回带**（必须等于 client 请求的 `requestId`，包括 client 未提供时回 `""`）；在广播 / 主动推送类消息中是固定 `""`（不要让客户端误以为这是一个新 request 的 ID）
- 特例：`error` 消息是**双重语义**的 —— 当它作为某 client 请求的响应（如 `emoji.send` 失败）时按"响应类"语义回带 `requestId`；当它是 server 主动产生（如运行时业务状态异常 —— 注意**不含**握手期 SnapshotBuilder 失败，该场景统一走 close 1011 而非 `error`，详见 §12.1 close code 表 1011 行 + §12.3 `error` 小节末尾"snapshot 构建失败处理路径"注）时按"主动推送类"语义固定 `""`。客户端解析层（Story 12.3）**应**把 `error` 与 `pong` 同等对待：`requestId != ""` → 走 request-response 配对；`requestId == ""` → 走全局错误事件总线。详见 §12.3 `error` 小节
- frame 类型 + 编码 + 大小上限 同 §12.2 通用信封（text frame / UTF-8 JSON / 16 KB 默认上限）

### 房间快照（room.snapshot）

**触发**：

- WS 握手成功后，服务端**主动**推送（必发）
- 服务端**可选**在 Session 生命周期内重复推送（如房间状态变化触发全量重发；节点 4 阶段不实装，仅 Story 10.7 placeholder 阶段在握手时发一次）

**字段**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | string | 必填 | 固定值 `"room.snapshot"` |
| `requestId` | string | 必填 | 固定 `""`（主动推送类消息） |
| `payload.room.id` | string | 必填 | 房间 ID（BIGINT 字符串化下发，遵循 §2.5 全局约定） |
| `payload.room.maxMembers` | number (int) | 必填 | 房间容量上限（节点 4 阶段固定 4，由 Story 11.3 创建房间事务写入；本接口仅返回当前值） |
| `payload.room.memberCount` | number (int) | 必填 | 房间总成员数 = 当前 `room_members WHERE room_id = ?` 行数；与下文 `payload.members` 数组长度严格相等（见本节末"不变量"小节）；节点 4 阶段 Story 10.7 placeholder 实现 = `SELECT COUNT(*) FROM room_members WHERE roomId=?` 的真实行数（或同一次 query 直接取 `len(members)`，二者必须一致），**禁止**写死为 1；Story 11.7 真实实现 = 同样的 `room_members` 行数，差异仅在丰富字段（placeholder 阶段不 JOIN `users` 故 `nickname` 降级为空字符串；`pet` 整体 going-forward 契约要求 placeholder 与真实两阶段均 LEFT JOIN `pets`，详见本节 `payload.members[].pet` 字段行），**不**在成员数量上与真实实装区分 |
| `payload.members` | array | 必填 | 房间全成员列表 = 当前 `room_members WHERE room_id = ?` 全部行（**不**做"WS 此刻是否连接"层的过滤，roster 反映 server 端"仍在房间"的成员；与 §10.3 `data.members[]` 同语义，详见 §10.3 "roster 语义与 WS 断线交互"小节）；going-forward 契约要求节点 4 阶段（Story 10.7 placeholder + Story 11.7 真实）均按 `room_members` LEFT JOIN `pets` 聚合（**必须 LEFT JOIN `pets`**：pet-less 账号无活跃 pet 行，INNER JOIN 会丢行 → 违反 `memberCount === members.length` 不变量；与 §10.3 服务端逻辑步骤 3 一致；pet-less 时 `pets.*` 列为 NULL，下发 `pet: null`），**禁止**只返回当前握手用户自己 —— 房间已有 ≥2 成员时漏返其他成员会让 client 把 snapshot 当 authoritative state 错误清空已加载的 roster；丰富字段在 placeholder 阶段降级 `nickname` 为空字符串（不 JOIN `users`，由 Story 11.7 真实实装时 INNER JOIN `users` 回填）；Story 11.7 真实实现 = Story 10.7 placeholder + INNER JOIN `users`（即把 `nickname` 改为 `users.nickname` 真实值），其余 `pet` 处理与 placeholder 一致，**仅 `nickname` 一字段差异**，成员条目数量与 placeholder 一致；**Story 10.7 落地实装的过渡兼容**：Story 10.7 当前实装走单表查询不 JOIN `pets`，所有 member 下发 `pet ≠ null + petId: ""` —— r14 锁定 going-forward 契约后 Story 10.7 已 done 不回工，Story 11.7 落地时**必须**切换到 LEFT JOIN `pets` 形态（详见本节 `payload.members[].pet` 行 + "Story 10.7 落地实装与 going-forward 契约的差异" 段） |
| `payload.members[].userId` | string | 必填 | 成员用户 ID（BIGINT 字符串化）；node-4 placeholder 阶段直接来自 `room_members.userId`，**所有成员行都返回**（不限于握手用户） |
| `payload.members[].nickname` | string | 必填 | 成员昵称；node-4 placeholder 阶段（Story 10.7）允许返回**空字符串** `""`（不 JOIN `users` 表，避免 placeholder 过度耦合 Story 11.7 的多表 JOIN）；**空字符串语义 = "我不知道这个值"**，client 按本节末"client merge contract"**保留** client 已有真实昵称（如来自 `GET /api/v1/rooms/{roomId}` 响应），**禁止**用空串覆盖；client 渲染时若本地无真实值，空串可降级为占位文案；Story 11.x（具体由 Story 11.7 真实 SnapshotBuilder 实装）由 `users.nickname` 真实回填 |
| `payload.members[].pet` | object \| null | 必填（nullable） | 成员当前宠物容器；与 §10.3 `data.members[].pet` / §5.1 GET /home `data.pet` 同语义，**pet-less 账号**（用户无活跃 pet 行，contract 内合法 edge case）下发 `null`；client 解析层按 `Optional<MemberPetDTO>` 处理（iOS / Go），`pet == null` 时**整个 pet 子树不下发**，渲染降级为"无宠物"占位；下方 `payload.members[].pet.*` 子字段**仅当 `pet ≠ null` 时存在**。**节点 4 阶段（Story 10.7 placeholder + Story 11.7 真实）going-forward 契约一致**：均必须 LEFT JOIN `pets` 后判定，pet-less → `null`、否则 `pet ≠ null` + `petId` 为 `pets.id` 字符串化值（与 §10.3 五阶段过渡表 `pet` 整体 / `pet.petId` 行 placeholder 与真实两列同值一致），**禁止**单表查询 + 一律下发 `pet ≠ null + petId: ""` 的简化路径（避免 client parser 同时支持两种 shape；Story 10.7 落地实装与 going-forward 契约的差异详见本节末"placeholder 字段值来源说明" + "Story 10.7 落地实装与 going-forward 契约的差异"段） |
| `payload.members[].pet.petId` | string | 必填（仅当 `pet ≠ null`） | 成员当前宠物 ID（BIGINT 字符串化）；going-forward 契约（节点 4 placeholder Story 10.7 + 真实 Story 11.7 一致）：来自 `pets.id` 字符串化（LEFT JOIN `pets` 后 `pet ≠ null` 分支即 `pets.id` 真实值），**必非空字符串**；**Story 10.7 落地实装的过渡兼容形态**：Story 10.7 当前实装走单表查询不 JOIN `pets`，所有 member 一律下发 `pet ≠ null` + `petId: ""`（空字符串）—— 这是 r10 之前的契约形态，r14 锁定 going-forward 契约为 LEFT JOIN + 真实 `pets.id` / `null`，但 Story 10.7 已 done 不回工；client 按本节末"client merge contract" 空字符串路径**保留** client 已有真实 petId（如来自 `GET /api/v1/rooms/{roomId}` 响应），**禁止**用空串覆盖；Story 11.7 真实 SnapshotBuilder 落地时切换到 going-forward 契约形态（LEFT JOIN + 真实 `pets.id` / pet-less 下发 `pet: null`） |
| `payload.members[].pet.currentState` | number (int) | 必填（仅当 `pet ≠ null`） | 宠物当前状态枚举：`1 = rest` / `2 = walk` / `3 = run`（与数据库设计 §6.4 `pets.current_state` 同义）；node-4 placeholder 阶段（Story 10.7）固定返回 `1`；Story 11.7 真实实现亦固定返回 `1`（Epic 14 才真实驱动）；**自 Story 14.3 起切真实值**（读 `pets.current_state`，与 §10.3 五阶段过渡表 `pet.currentState` 节点 5 真实列 / §12.3 `### 成员加入（member.joined）` `payload.pet.currentState` 同时切真实路径，由 Story 14.3 落地 `RoomSnapshotBuilder` 真实化时同步覆盖 `member.joined` —— 三处 server → client `pet.currentState` 字段切真实值的 epic 落地点统一在 Story 14.3，同一 `pets.current_state` 来源） |
| `ts` | number (int64) | 必填 | 服务端发送时间戳（ms） |

> **Future Fields（节点 4 阶段为占位 / 节点 5 / 9 落地）**：
>
> - `payload.members[].avatarUrl`（成员头像 URL）：Story 11.7 真实实现时启用；节点 4 阶段 placeholder 不返回该字段（不要在 §12.3 字段表添加；client 解析为 `String?` 可选字段）；按本节末 "client merge contract" 中"未出现字段保留 client 已有值"规则，client **不**清空已通过 `GET /api/v1/rooms/{roomId}` 加载的真实 avatarUrl
> - `payload.members[].pet.equips`（成员当前装备）：Epic 26 / Story 26.6 落地后由 Story 11.7 同步扩展；节点 4 阶段不返回（按 merge contract 保留 client 已有值）
> - `payload.members[].pet.equips[].renderConfig`（装备渲染配置）：Epic 29 / Story 29.6 落地后由 Story 11.7 同步扩展；节点 4 阶段不返回（按 merge contract 保留 client 已有值）

JSON 示例（真实示例，Story 11.7 落地后形态；含 1 个 pet-less 边界案例 `userId: "1003"`）：

```json
{
  "type": "room.snapshot",
  "requestId": "",
  "payload": {
    "room": {
      "id": "3001",
      "maxMembers": 4,
      "memberCount": 3
    },
    "members": [
      {
        "userId": "1001",
        "nickname": "A",
        "pet": {
          "petId": "2001",
          "currentState": 1
        }
      },
      {
        "userId": "1002",
        "nickname": "B",
        "pet": {
          "petId": "2002",
          "currentState": 1
        }
      },
      {
        "userId": "1003",
        "nickname": "C",
        "pet": null
      }
    ]
  },
  "ts": 1776920345000
}
```

> **示例说明**：`userId: "1003"` 是 pet-less 账号（`pet: null`）—— 与 §10.3 GET /rooms/{roomId} 同语义，由 LEFT JOIN `pets` 在用户无活跃 pet 行时下发；client 解析层按 `Optional<MemberPetDTO>` 处理，`pet == null` 渲染降级为"无宠物"占位（不渲染 cat sprite），并按本节末 client merge contract `null` 处理路径直接覆盖 client 已有 pet 状态（authoritative pet-less 信号）；绝大部分用户场景下 `pet ≠ null`，但 contract 层必须覆盖该 edge case。

> **不变量（snapshot 内部一致性）**：`memberCount` 必须**严格等于** `members[]` 数组长度。两者**统一表示当前 `room_members` 行数**（即 server 端"仍在房间"的全部成员），**不**做"WS 此刻是否连接"层的过滤 —— 即 snapshot 是房间的 full roster view（按 `room_members` 全行），**不是** WS-online-only view。理由：(a) 若 `memberCount` 与 `members[].length` 任一方做"WS 在线过滤"而另一方不做，违反不变量；(b) 节点 4 阶段服务端**不**在握手时广播 `member.joined`（详见 §12.1 末尾 placeholder 注），客户端无法靠后续推送补齐 row 还在但 WS 暂未连上的成员，snapshot 必须自包含 `room_members` 全行；(c) 节点 4 阶段**不**引入"online / offline" 字段层区分 —— `room_members` 行的删除**唯一**路径是 HTTP `POST /rooms/{roomId}/leave` 事务（删行后触发 `member.left` 广播，该 user 不再出现在后续 snapshot / GET /rooms 响应中）；任何 WS 层断开（含心跳超时 / TCP 1006 / app 关闭）**仅**清 ephemeral 层（SessionManager + Redis presence），**不**触动 `room_members` 行 —— 因此该 user 在断线期间仍出现在后续 snapshot / GET /rooms roster 中，与 §12.1 4005 retryable 语义自洽（详见 §10.3 末尾"roster 语义与 WS 断线交互"小节）。节点 4 阶段 Story 10.7 placeholder 实装时 `members[]` 必须**反映 `room_members` 全部成员行**（最少 1 个，即握手用户自己；房间已有 ≥2 成员时必须返回全部）—— **禁止**写"全零 placeholder"（`memberCount: 0` + `members: []`），也**禁止**写"单成员快照"（仅当前握手用户自己，`memberCount` 写死为 1），因为：(i) §12.1 第 5 步握手成功**充分条件**只校验"当前用户已在 `room_members` 表中"，**不**保证房间只有 1 个成员；房间已有 ≥2 成员时单成员 snapshot 会让 client 把首条 authoritative 消息当成"房间被清空"，错误清空已加载的合法 roster；(ii) 推荐房间进入流程要求 client 先 `GET /api/v1/rooms/{roomId}` 加载房间状态后再开 WS（详见 §11.5 客户端推荐调用顺序），client 已经持有合法 roster 视图，再收到一个比真实成员少的 snapshot 同样会错误覆盖（无论是零成员还是单成员都属于"少返"）；(iii) §12.1.3 钦定 `room.snapshot` 是握手成功后**必发**的第一条 authoritative 消息，client 把它作为权威态采用，placeholder 必须给出**结构上真实**的快照（成员条目齐全），仅在丰富字段（`nickname` / `pet.*`）层面降级为占位默认值，**不**在成员数量上偷工。Story 11.7 真实实装时由**同一次** `room_members` JOIN `users` JOIN `pets` 聚合产出 `members[]`，`memberCount` 取该数组长度（或同一次 query 的 `COUNT(*)`），server 实装层面**禁止**让两者出现 drift；与 Story 10.7 placeholder 的差异**仅在丰富字段**（`nickname` / `pet.*` 真实回填），**不在成员数量**。

**节点 4 阶段 placeholder going-forward 示例**（Story 10.7 placeholder + Story 11.7 真实 SnapshotBuilder 共同 going-forward 契约形态；与上述真实示例的差异**仅在 `nickname` 字段**（placeholder 阶段不 JOIN `users` 故 `nickname` 为空字符串 `""`，真实阶段填 `users.nickname` 真实值）—— `pet` 整体已 LEFT JOIN `pets`，pet-less 下发 `null`、否则 `petId` 为 `pets.id` 真实值（与 §10.3 五阶段过渡表 placeholder / 真实两列同值一致）；`members[]` 必须反映 `room_members` 全部成员行；下例为房间已有 3 成员的场景，含 1 个 pet-less 边界案例 `userId: "1003"`；`memberCount` = `members[].length` = 3，不变量不破）。**注意**：下例中 `nickname: ""` 是"server 不知道"的 placeholder 信号 —— client 按下方 "client merge contract" **保留** 已通过 `GET /api/v1/rooms/{roomId}` 加载的真实昵称，**禁止**用空串覆盖；`pet.petId` 是 LEFT JOIN `pets` 后的真实值，client 直接覆盖；`pet: null`（`userId: "1003"`）是 authoritative pet-less 信号，client 直接覆盖为"无 pet"：

```json
{
  "type": "room.snapshot",
  "requestId": "",
  "payload": {
    "room": {
      "id": "3001",
      "maxMembers": 4,
      "memberCount": 3
    },
    "members": [
      {
        "userId": "1001",
        "nickname": "",
        "pet": {
          "petId": "2001",
          "currentState": 1
        }
      },
      {
        "userId": "1002",
        "nickname": "",
        "pet": {
          "petId": "2002",
          "currentState": 1
        }
      },
      {
        "userId": "1003",
        "nickname": "",
        "pet": null
      }
    ]
  },
  "ts": 1776920345000
}
```

> **placeholder 字段值来源说明（going-forward 契约）**：上例 `members[]` 直接来自 `SELECT * FROM room_members WHERE roomId=?` 全部行（取得每个成员的 userId）+ **LEFT JOIN `pets` ON pets.user_id = room_members.user_id**（取得每个成员的 `pets.id` / `pets.current_state`，pet-less 时 `pets.*` 列为 NULL）—— `userId` 取 `room_members.userId`；`nickname` 在 placeholder 阶段返回空字符串 `""`（**不** JOIN `users`，由 Story 11.7 真实实装时由 `users.nickname` 回填）；`pet` 整体按 LEFT JOIN 结果判定：若 `pets.*` 列为 NULL（pet-less 账号，contract 内合法 edge case，与 §5.1 / Story 4.8 一致）→ 下发 `pet: null`；否则下发 `pet ≠ null` + `petId: pets.id` 字符串化 + `currentState: 1`（节点 4 阶段固定 `1`，与数据库设计 §6.4 `pets.current_state` 同义；Epic 14 才真实驱动）。**Story 11.7 真实实装** = Story 10.7 placeholder + INNER JOIN `users`（即把 `nickname` 从空字符串改为 `users.nickname` 真实值），其余 `pet` 处理与 placeholder 一致 —— 这是 "placeholder vs 真实仅 `nickname` 一字段差异" 的最小过渡。

> **Story 10.7 落地实装与 going-forward 契约的差异**（r14 锁定，Story 11.7 必须 backfill）：上例 `pet` 字段处理是 r14 起的 going-forward 契约形态（LEFT JOIN `pets` + pet-less `null`）。Story 10.7 已 done 的实装走单表查询不 JOIN `pets`，所有 member 一律下发 `pet ≠ null` + `petId: ""`（即对所有成员发 `{petId: "", currentState: 1}`，**包括** pet-less 成员 —— pet-less edge case 在 Story 10.7 当前实装下被退化为"`pet ≠ null + petId: ""`" 的兼容占位，client merge contract 走"保留已有 petId"路径不会清空真实值，但同时 client 也收不到 authoritative 的 `pet: null` 信号；该 edge case 占比极低且 Story 11.7 落地后即修复，因此不回工 Story 10.7）。Story 11.7 真实 SnapshotBuilder 落地时**必须**切换到 going-forward 契约形态（LEFT JOIN `pets` + pet-less `null` + 否则 `pets.id` 真实值），与本节字段表 / 上例严格对齐；client 解析层**不**做"两种 shape 分流"逻辑，按 client merge contract 单一规则处理（空字符串 → 保留、`null` → 覆盖、非空真实值 → 覆盖），即可同时正确处理 Story 10.7 落地实装与 Story 11.7 going-forward 形态。

> **Client merge contract（client 解析 `room.snapshot` 时必须遵守）**：snapshot 是握手后**必发**的第一条 authoritative 消息（§12.1 握手成功流程），但其权威性是 **enrich/correct** 而**非** wipe-out。client 在收到 `room.snapshot` 时，**禁止**做 "把 `members[]` 整体替换 client 当前 roster" 的暴力赋值，**必须**对每个 member entry 做**字段级 merge**，规则如下：
>
> 1. **roster 集合层（`members[]` 数组本身）**：以 snapshot 的 `userId` 集合为权威集合 —— snapshot 没有的 `userId` → client 应**移除**（视为已离开房间的真实 authoritative 信号）；snapshot 有但 client 之前没有的 `userId` → client 应**新增** entry（用 snapshot 的字段值填充，空字符串字段保持空，由后续 `member.joined` / 重新 `GET /api/v1/rooms/{roomId}` enrich）。"成员存在性"是结构信息，snapshot 在这一层是 authoritative。
> 2. **字段级（每个 entry 的字段值）**：
>    - **非空值**（如 `nickname: "Alice"` / `pet.petId: "2002"` / `pet.currentState: 2`）：用 snapshot 的值**覆盖** client 已有值（这是真实 authoritative 数据，覆盖正确）。
>    - **空字符串**（`""`）：**保留** client 已有值。空字符串 = "server 不知道这个值"的 placeholder 信号（节点 4 placeholder 阶段 / 任何未来未 enrich 的字段同义），**不是** "请清空" 的指令；若 client 通过 §15.6 推荐流程从 `GET /api/v1/rooms/{roomId}` 已加载真实昵称 / petId，必须保留这些真实值，避免"每次重连退化为空昵称 / 空 petId"。
>    - **`null` 值**：与空字符串语义不同 —— `null` 在本协议中保留给"明确无值"语义（如 §4.3 `currentRoomId: null` = 用户当前不在任何房间，§5.1 `data.pet = null` = 用户 pet-less，本节 `payload.members[].pet = null` = 该成员 pet-less）；当 server 下发 `payload.members[].pet = null` 时，**这是 authoritative 的"该成员当前确实无 pet"信号**（与"server 不知道"的空字符串语义不同），client **应**直接覆盖 client 已有值（即把该成员 pet 状态置为 null，渲染降级为"无宠物"占位，不渲染 cat sprite）；其余成员级别字段（`userId` / `nickname` / `pet.petId` / `pet.currentState`）当前协议中**不**取 `null` 值。
>    - **未出现的字段**（如 placeholder 阶段不下发 `avatarUrl` / `pet.equips`）：**保留** client 已有值（与空字符串等价处理）。
> 3. **数值字段**（`pet.currentState`）：节点 4 阶段 placeholder 与 Story 11.7 真实实装均固定 `1`（Epic 14 才真实驱动 motion_state）；client 收到该值时**应**直接覆盖 client 已有值（无 placeholder 信号约定 —— 数值字段不存在"空字符串"语义；当未来 Epic 14 真实驱动后，server 下发 2/3 即真实值）。**Story 14.3 落地前的临时窗口例外**（适用于 `pet.currentState` 单字段）：在该窗口内，`room.snapshot.payload.members[].pet.currentState` / `GET /rooms/{roomId}.data.members[].pet.currentState` / `member.joined.payload.pet.currentState` 三处的 placeholder `1` 值**不**触发"直接覆盖"，client **应**按"如果新值来自上述三个 placeholder 源 + client 已有值为非 `1` 真实值（来源限定见下方"真实值来源白名单"）→ 跳过覆盖，保留已有真实值"分支处理；仅当 client 已有值缺失 / 也是 `1` 时才接受 placeholder `1`。**真实值来源白名单（self entry 与 others entry 分桶）**：
>    - **others entry**（房间内非 caller 自己的成员）的"非 `1` 真实值"**仅**来自 `pet.state.changed` WS 广播 `payload.currentState` 字段（others 不存在 HTTP ack 来源 —— caller 自己发的 `POST /pets/current/state-sync` HTTP 响应**不**承载别人的状态，参见 §5.2 line 545 "对'别人的状态变化'走 WS 唯一权威"）
>    - **self entry**（caller 自己的成员）的"非 `1` 真实值"可来自 **(i) `pet.state.changed` WS 广播 `payload.currentState`** **或 (ii) `POST /pets/current/state-sync` HTTP 200 OK `response.data.state`**（即 self-broadcast 例外规则下的 self-only 权威 HTTP ack 信号，参见 §5.2 line 547 "对'发起者自己的状态变化'的权威信号 (a)/(b)/(c) 对称展开"）；只要 client 已有 self entry `currentState` 是来自上述任一信号源的非 `1` 值，placeholder `1` 即跳过覆盖。**漏洞封堵说明**：若仅把白名单限定为 `pet.state.changed` WS 广播（不含 HTTP 200 ack），则 self-broadcast 丢失场景下 self state 从 HTTP 200 写入后，晚到的 placeholder `room.snapshot` / `member.joined` 会触发"已有值不在白名单 → 直接覆盖"路径，把 self state 回退到 `rest`（即本临时窗口例外**本意要避免**的 stale 风险），违反 §5.2 line 547 self-broadcast 兜底规则的 invariant。把 HTTP 200 ack 纳入 self-entry 白名单后，self UI 在两路 server → client 信号（HTTP 200 / `pet.state.changed`）任一到达后即获 placeholder override 保护，回退路径关闭
>
>    该例外**仅对 `pet.currentState` 字段** + **仅在 14.3 落地前**生效；Story 14.3 落地后，三处统一返回真实值（权威等价层生效）→ 例外失效，回归"数值字段直接覆盖"通用规则。该例外目的：避免 reconnect / 晚到的 `room.snapshot` / `member.joined` placeholder `1` 把更新的 `walk/run` 状态回退到 `rest`（与 §5.2 line 612 / §12.3 `pet.state.changed` line 2247 临时窗口权威信号优先级排序一致）。
>
> **rationale**：在 placeholder 阶段（Story 10.7）和真实阶段（Story 11.7）应用同一套 merge 规则，行为一致：snapshot 永远只能 enrich/correct client state。Story 11.7 真实实装时，`nickname` / `pet.petId` 等字段都会下发非空值，触发 "覆盖" 路径；placeholder 阶段下发空字符串，触发 "保留" 路径 —— **server 实装层无需为两个阶段写不同 client merge 代码**，只需 client 始终按本契约执行。
>
> **client 实装位置**：iOS 侧由 Story 12.3（WS 消息解码层）+ Story 12.4（房间 ViewModel 状态合并层）实装；server 实装层（Story 10.7 SnapshotBuilder placeholder / Story 11.7 真实实装）只保证下发字段语义符合本契约，**不**关心 client 如何 merge。

### 成员加入（member.joined）

**触发**：

- `POST /api/v1/rooms/{roomId}/join` 加入房间事务**成功提交后**，server 调用 `BroadcastToRoom(roomID, {type: "member.joined", payload: {userId, nickname, avatarUrl, pet: {petId, currentState} | null}})` 广播给该房间内所有**其他**在线成员（不发给加入者自己 —— 加入者自己应收 HTTP 响应即知自己已加入，且加入者后续如建立 WS 连接，握手时 server 会下发含自己的 `room.snapshot`，与已连接成员通过 `member.joined` enrich roster 的路径互补）；payload **必须**含 `avatarUrl` + `pet`（其中 `pet` 是 nullable —— pet-less 账号下发 `null`，详见下方"字段"表 `payload.pet` 行；与 §10.3 / §12.3 `room.snapshot` `members[].pet` 同语义保持一致），不能简化为仅 `userId + nickname` —— 已连接成员仅在握手时收一次 `room.snapshot`（§12.1.3 钦定），后续无新 snapshot 触发，**唯一**enrich 新成员展示字段（头像、宠物 ID、宠物状态）的路径就是 `member.joined` 自身 payload；若 trigger 实装层简化为仅下发 `userId + nickname`，已连接成员将永远看不到新成员的头像 / 宠物，违反"`member.joined` 必须自包含展示所需全部字段"语义（详见下方"字段"表 + Story 11.8 实装锚定）
- 节点 4 阶段**仅** Story 11.8 一处触发；任何 WS 层事件（含握手、心跳超时、断开重连）**都不**触发 `member.joined` —— `member.joined` 与 `room_members` 行的"新增"语义严格 1:1 对应，而新增持久层行**仅**通过 HTTP join 接口完成（详见 §10.4 服务端逻辑）；后续 epic 不在本路径加新触发条件（如 Story 14.x 状态广播走独立 `pet.state.changed`）

**字段**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | string | 必填 | 固定值 `"member.joined"` |
| `requestId` | string | 必填 | 固定 `""`（主动推送类消息，遵循 §12.3 通用信封） |
| `payload.userId` | string | 必填 | 加入的成员 user 主键（BIGINT 字符串化）；来自加入事务的当前 user |
| `payload.nickname` | string | 必填 | 加入的成员昵称；来自 `users.nickname`；**必非空字符串**（节点 2 阶段首次创建时 server 写入 `"用户{id}"`，必有真实值；不存在 placeholder 阶段空字符串场景） |
| `payload.avatarUrl` | string | 必填 | 加入的成员头像 URL；来自 `users.avatar_url`；可空字符串 `""`（首次创建用户时 `users.avatar_url` 为空；节点 4 阶段 server 端未做头像上传链路），**不**为 null —— client 解析层按 `String` 处理（与 §10.3 `data.members[].avatarUrl` 一致） |
| `payload.pet` | object \| null | 必填（nullable） | 加入的成员当前宠物容器；与 §10.3 `data.members[].pet` / §12.3 `room.snapshot` `payload.members[].pet` / §5.1 GET /home `data.pet` 同语义，**pet-less 账号**（用户无活跃 pet 行，contract 内合法 edge case，与 §5.1 / Story 4.8 保持一致）下发 `null`；client 解析层按 `Optional<MemberPetDTO>` 处理；下方 `payload.pet.*` 子字段**仅当 `pet ≠ null` 时存在**；client 收到 `pet == null` 时 append entry 时 pet 字段置空，渲染降级为"无宠物"占位（与 `room.snapshot` 同 merge 规则） |
| `payload.pet.petId` | string | 必填（仅当 `pet ≠ null`） | 加入的成员当前宠物 ID（BIGINT 字符串化）；来自加入事务前 server 已查询该 user 的活跃 `pets.id`；当 `pet ≠ null` 时**必非空字符串**（绝大部分用户场景：节点 2 阶段首次注册时 server 写入默认 pet 行，每个正常 user 都有活跃 pet；pet-less edge case 下整个 `pet` 已是 `null`，不进入本字段约束） |
| `payload.pet.currentState` | number (int) | 必填（仅当 `pet ≠ null`） | 加入时刻宠物当前状态枚举（`1 = rest` / `2 = walk` / `3 = run`，与数据库设计 §6.4 `pets.current_state` 同义）；**节点 4 阶段固定 `1`**（与 §10.3 `data.members[].pet.currentState` / §12.3 `room.snapshot` placeholder 同语义）；**自 Story 14.3 起切真实值**（读 `pets.current_state`，与 §10.3 五阶段过渡表 `pet.currentState` 节点 5 真实列 / §12.3 `room.snapshot.payload.members[].pet.currentState` 同时切真实路径，由 Story 14.3 落地 `RoomSnapshotBuilder` 真实化时同步覆盖 `member.joined` —— 同一 `pets.current_state` 来源，三处 server → client `pet.currentState` 字段切真实值的 epic 落地点统一在 Story 14.3，避免 join 房间后看到 `1` 永远 stale 的 race：用户房间外切 walk/run（§5.2 服务端逻辑步骤 5 `current_room_id == NULL` 写库但不广播）→ join 房间 → `member.joined` 若仍下发 `1`、且后续 `pet.state.changed` 仅当该 user 再次切状态时才广播 → 房间内其他成员永久看到 stale `1`） |
| `ts` | number (int64) | 必填 | 服务端发送时间戳（ms） |

> **Future Fields（节点 4 阶段为占位 / 节点 9 / 10 落地）**：
>
> - `payload.pet.equips`（成员当前装备）：Epic 26 / Story 26.6 落地后由 Story 11.8 同步扩展；节点 4 阶段**不**下发该字段；client 解析层按 `[<EquipDTO>]?` 可选数组处理，未出现时不视为契约违反；当 `pet == null` 时本字段亦不下发（整个 pet 子树缺席）
> - `payload.pet.equips[].renderConfig`（装备渲染配置）：Epic 29 / Story 29.6 落地后由 Story 11.8 同步扩展；节点 4 阶段不下发

JSON 示例（常规场景，`pet ≠ null`）：

```json
{
  "type": "member.joined",
  "requestId": "",
  "payload": {
    "userId": "1002",
    "nickname": "用户1002",
    "avatarUrl": "",
    "pet": {
      "petId": "2002",
      "currentState": 1
    }
  },
  "ts": 1776920345000
}
```

JSON 示例（pet-less edge case，`pet == null`）：

```json
{
  "type": "member.joined",
  "requestId": "",
  "payload": {
    "userId": "1003",
    "nickname": "用户1003",
    "avatarUrl": "",
    "pet": null
  },
  "ts": 1776920345000
}
```

**关键约束**：

- `payload.nickname` **必非空字符串** —— 与 `room.snapshot.payload.members[].nickname` 在 placeholder 阶段允许空字符串的语义**不同**：`member.joined` 在加入事务**成功提交后**触发，server 必有真实 nickname（`users` 表已被读取过用于事务决策），无 placeholder 阶段；client 解析层应按 `nickname != ""` 假设处理（节点 4 阶段所有 `member.joined` 消息字段值都是真实数据）
- 广播范围：**仅**该房间内当前在线的其他 Session（不含加入者自己）—— 由 BroadcastToRoom primitive（Story 10.5）实装的 fanout 路径决定；加入者自己 = 当前 HTTP 请求方，自己收 HTTP 200 响应已知道结果，不需要再收一份 WS 通知
- `payload` 字段**完整自包含成员展示所需的全部丰富字段**（`userId` / `nickname` / `avatarUrl` / `pet`，其中 `pet` 是 nullable —— pet-less 账号下发 `null`，否则下发 `{petId, currentState}` 完整 object），client 收到 `member.joined` 后**可直接** append 一条完整的 roster entry（`pet = null` 时 entry 中 pet 字段置空，渲染降级"无宠物"占位），**不**需要走 `GET /api/v1/rooms/{roomId}` 二次拉取 —— 这是因为 `room.snapshot` 仅在握手时下发一次（§12.1.3），server **不**会在 `member.joined` 之后给已连接成员追发 snapshot；若 `member.joined` 仅含 `userId` / `nickname`，已连接成员将永远拿不到该新成员的 `avatarUrl` / `pet.*` 真实值（避坑：r1 review 指出过这条不一致）
- `payload.avatarUrl` 与 `payload.pet`（含子字段，仅当 `pet ≠ null`）的填充语义与 `GET /api/v1/rooms/{roomId}.data.members[]` 同字段一致（都是 `users.avatar_url` / 该 user 活跃 `pets.*` 的真实回填，`pet-less` 时 `pet = null`），**不**走"placeholder 空字符串"路径 —— 加入事务**成功提交后**才广播，server 必有完整 authoritative 值（含 `pet = null` 的 authoritative pet-less 信号）
- client 收到 `member.joined` 后**应**走 §12.3 client merge contract 字段级 merge：(a) roster 中已存在该 `userId` entry → 字段级覆盖（按 client merge contract"非空覆盖、空字符串保留 client 已有值、null 直接覆盖（authoritative）"规则；本 story 中 `avatarUrl` 可能下发空串，按规则**保留** client 已有真实值；`pet = null` 是 authoritative pet-less 信号，**应**直接覆盖）；(b) roster 中不存在该 `userId` entry → 新增完整 entry（含本字段表所有字段，`pet = null` 时 entry 中 pet 字段置空），渲染层立即可用，无需等待二次 snapshot 或额外 HTTP 拉取
- `payload.userId` / `payload.nickname` / `payload.avatarUrl` / `payload.pet` 都必填（**禁止** payload 为 `{}` 或仅 `userId` / `nickname`）；`payload.pet` 是 nullable 字段（`pet-less` 时下发 `null`，否则下发完整 `{petId, currentState}` object —— 不允许下发 `pet: {}` 空 object 或省略 `pet` key）；缺字段视为契约违反，client 解析层**应**走"安全忽略 + log warn"路径（与 Story 10.1 `安全忽略未识别 type` 相同的容错策略，避免单条 malformed 消息把房间页搞崩）
- 节点 4 阶段加入者**不**主动收到自己的 `member.joined`：server 实装上应从 fanout 列表中排除加入者自己的 Session，client 解析层不需要做"自己 != 自己"的过滤（防御性编程层面 client 仍**应**对 `payload.userId == 当前 user.id` 走 noop 安全路径）

### 成员离开（member.left）

**触发**（r3 锁定）：

- `POST /api/v1/rooms/{roomId}/leave` 退出房间事务**成功提交后**，server **先**关闭 leaver 自己的 WS Session（§10.5 步骤 7：unregister + close 4007 best-effort cleanup），**后**调用 `BroadcastToRoom(roomID, {type: "member.left", payload: {userId}})`（§10.5 步骤 8）—— broadcast 时 `ListSessionsByRoomID` 返回列表已不含 leaver Session，fanout 自然只到达该房间内所有**其他**在线成员（不发给离开者自己 —— 离开者以 HTTP 200 响应作为 leave 完成的 authoritative signal；close 4007 不是 leave 完成的协议层确认）
- **唯一触发条件**就是上一条 HTTP leave 路径；任何 WS 层断开（含心跳超时 → close 4005、client 主动 close、app 关闭 / 切后台、TCP 1006 异常断开）**都不**触发 `member.left` —— WS 断开仅清 ephemeral 层（SessionManager + Redis presence），不改 `room_members` / `users.current_room_id`，不广播 `member.left`；详见 §10.3 "roster 语义与 WS 断线交互" 小节 + §10.5 "WS 断线场景与本接口的关系" 注解块
- 这条钦定与 §12.1 close code 4005 行"client 应自动重连"的 transient 语义自洽：心跳超时不删 row → reconnect 握手时 §12.1 校验顺序步骤 5 通过 → 用户保留座位

**字段**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | string | 必填 | 固定值 `"member.left"` |
| `requestId` | string | 必填 | 固定 `""`（主动推送类消息，遵循 §12.3 通用信封） |
| `payload.userId` | string | 必填 | 离开的成员 user 主键（BIGINT 字符串化）；来自 HTTP leave 退出事务的当前 user |
| `ts` | number (int64) | 必填 | 服务端发送时间戳（ms） |

JSON 示例：

```json
{
  "type": "member.left",
  "requestId": "",
  "payload": {
    "userId": "1002"
  },
  "ts": 1776920345000
}
```

**关键约束**：

- `payload` 字段**精简**为仅 `userId`（与 `member.joined` 含 `nickname` 不同）—— 离开事件 client UX 不需要显示昵称（"X 离开了房间"中的 X 可由 client 从已有 roster 查到 nickname；即使没查到，UX 文案降级为"有人离开"也可接受），减少 server 加载压力
- 广播范围：**仅**该房间内当前在线的其他 Session（不含离开者自己）；离开者以 HTTP 200 响应作为 leave 完成的 authoritative signal（§10.5 步骤 9）；步骤 7 的 close 4007 是 server 端 best-effort cleanup（让 leaver Session 立即解耦），且 step 7 在 step 8 broadcast **之前**执行（r13 锁定）—— 因此 broadcast 时 `BroadcastToRoom` 调用 `ListSessionsByRoomID` 返回的列表里物理上不含 leaver Session，fanout 自然跳过离开者，**无需** BroadcastToRoom primitive 提供 `excludeUserID` 参数（与 Story 10.5 落地实装一致）
- client 解析层收到 `member.left` 时**应**按 §12.3 client merge contract 集合层规则：从 client roster 中**移除** `payload.userId` 对应 entry（这是 authoritative 的离开信号，与 snapshot 集合层 authoritative 一致）；client **不**应等下一次 `room.snapshot` 才更新 roster
- 节点 4 阶段离开者**不**主动收到自己的 `member.left`：server 实装上 leaver Session 在 §10.5 步骤 7（close 4007 + SessionManager unregister）之后**已不在** SessionManager 列表中，**步骤 8** broadcast 时 `BroadcastToRoom` 调 `ListSessionsByRoomID` 返回列表自然不含 leaver Session，fanout 物理上跳过离开者 —— 该顺序（step 7 close 在 step 8 broadcast 之前，r13 锁定）是契约层钦定的实装路径，无需扩展 BroadcastToRoom primitive 即可满足语义。注：close 4007 frame 是 best-effort cleanup（client 可能收不到），但 server 端 SessionManager unregister 动作总是发生（无论 socket write 是否成功），broadcast 时 fanout 列表过滤永远生效；client 解析层防御性走 `payload.userId == 当前 user.id` 的 noop 安全路径
- **触发不重复**：server 实装层应保证 `member.left` 广播严格 1:1 对应"`room_members` 行被删除"事件 —— 由于唯一删行路径是 HTTP leave 事务（详见 §10.5 服务端逻辑），同一 user 在同一 leave 事件中**只触发一次** `member.left` 广播；任何 WS 断开（含心跳超时）**禁止**走"被动 leave"路径删 row + 广播 `member.left`（详见 §10.3 / §10.5 + Story 11.8 实装）

### 宠物状态变更（pet.state.changed）

**触发**：

- `POST /api/v1/pets/current/state-sync`（详见 §5.2）服务端成功 UPDATE `pets.current_state` **之后**，service 层检查当前 user 的 `users.current_room_id`：
  - **非 null** → server 调用 `BroadcastToRoom(currentRoomId, {type: "pet.state.changed", payload: {userId, petId, currentState}})` 广播给该房间内所有当前在线 Session（**包含**发起者自己 —— 与 `### 成员加入` `### 成员离开` 排除发起者**不同**语义，详见下方"关键约束"段）；广播失败 fire-and-forget（仅 log warning，**不**影响 HTTP 响应 / **不**回滚 DB UPDATE）
  - **null** → 用户当前不在任何房间，**不**广播（无房间内成员需要被通知；HTTP response 仍 200 OK，仅省略 WS 广播步骤）
- 节点 5 阶段**仅** Story 14.4 一处触发；任何 WS 层事件（含握手、心跳超时、断开重连、用户进出房间）**都不**触发 `pet.state.changed` —— `pet.state.changed` 与 `pets.current_state` 列的"实际写入"语义严格 1:1 对应，而该列写入**仅**通过 `POST /pets/current/state-sync` 接口完成（节点 5 阶段；后续 epic 不在本路径加新触发条件，如假设性的"自动状态衰减"等）

**字段**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | string | 必填 | 固定值 `"pet.state.changed"` |
| `requestId` | string | 必填 | 固定 `""`（主动推送类消息，遵循 §12.3 通用信封） |
| `payload.userId` | string | 必填 | 状态变更的 user 主键（BIGINT 字符串化，遵循 §2.5）；来自 `POST /pets/current/state-sync` 当前 user.id |
| `payload.petId` | string | 必填 | 状态变更的 pet 主键（BIGINT 字符串化，遵循 §2.5）；来自 service 层步骤 3 查到的 `pets.id` |
| `payload.currentState` | number (int) | 必填 | 变更后宠物当前状态枚举（`1 = rest` / `2 = walk` / `3 = run`，与数据库设计 §6.4 `pets.current_state` 同义；与 §10.3 `data.members[].pet.currentState` / §12.3 `room.snapshot` `payload.members[].pet.currentState` 同语义；与 §5.2 request `state` 等价 —— 都是入参回显） |
| `ts` | number (int64) | 必填 | 服务端发送时间戳（ms） |

JSON 示例：

```json
{
  "type": "pet.state.changed",
  "requestId": "",
  "payload": {
    "userId": "1002",
    "petId": "2002",
    "currentState": 2
  },
  "ts": 1776920345000
}
```

**关键约束**：

- 广播范围：**该房间内所有当前在线 Session**（**包含**发起者自己）—— 与 `### 成员加入` 排除加入者自己、`### 成员离开` 排除离开者自己（leaver 物理上收不到自己的 `member.left`）的语义**不同**。原因：(a) `member.joined` / `member.left` 是"成员关系变化通知"，发起者已通过 HTTP response 知道结果，再收一份 WS 通知是冗余；(b) `pet.state.changed` server 端**不区分** broadcast 接收者是发起者 / 其他成员（统一发给房间内全员），让 client 端**对别人的状态变化**走单一 WS 权威信号路径。**发起者自己的 self-broadcast UI 驱动职责（基于到达顺序对称）**：见 §5.2 "WS 广播 vs HTTP 响应的关系（含发起者自己的 self-broadcast 兜底规则）" line 547-551 完整对称展开 (a)/(b)/(c) 三条规则 —— **任一路径先到的信号（HTTP 200 或 self-broadcast）都立即驱动本地 self entry 的 roster pet state 更新**，后到的信号按字段级 merge 走 **no-op** 路径（两路信号在 server 端来源同一次 `UPDATE pets ...` 写库行为，值必定相同，merge 幂等）；client 实装层**不**假设固定到达顺序，**不**因 self-broadcast 先到而忽略其 UI 驱动职责，也**不**因 HTTP 200 先到后让 self-broadcast 沦为"仅 ack/活性探测"。**self-broadcast 的剩余职责仅在它是后到信号时生效**（HTTP 先到 → UI 已更新 → self-broadcast 到达走 merge no-op 时附带承担）：(a) 跨设备一致性校验；(b) WS 链路活性探测；详见 §5.2 line 555 "self-broadcast 的剩余职责（在 self entry 已被先到信号驱动后）"。这条 single source of truth 原则在 CLAUDE.md §"工作纪律 / 状态以 server 为准" 的统一表述下，对"自己 vs 别人"采用对称但不同的具体路径：对别人 → WS 唯一权威；对自己 → HTTP 200 与 self-broadcast 任一先到即驱动 UI（对称 no-op 不变量）。
- `payload.userId` / `payload.petId` / `payload.currentState` 都必填（**禁止** payload 为 `{}` 或缺任一字段）；缺字段视为契约违反，client 解析层**应**走"安全忽略 + log warn"路径（与 Story 10.1 `安全忽略未识别 type` 相同的容错策略，避免单条 malformed 消息把房间页搞崩）
- client 收到 `pet.state.changed` 后**应**走 §12.3 client merge contract 字段级 merge：(a) roster 中已存在该 `userId` entry → 更新其 `pet.currentState`（仅覆盖该字段，不影响 `nickname` / `avatarUrl` / `pet.petId`）；(b) roster 中**不**存在该 `userId` entry（理论不该发生 —— 同房间内其他成员的 entry 应在握手 `room.snapshot` 时已建立，或后续 `member.joined` 增量；如果 entry 不存在表示 roster 与 server 状态严重不一致）→ 走"安全忽略 + log warn"路径（**不**为单条 `pet.state.changed` 新增 roster entry —— 状态变更广播不携带 `nickname` / `avatarUrl` 等成员展示字段，新增 entry 无法正常渲染）
- `payload.currentState` 字段语义与 `room.snapshot.payload.members[].pet.currentState` / `GET /rooms/{roomId}.data.members[].pet.currentState` / **`member.joined.payload.pet.currentState`** 的等价分两层 + 受 Story 14.3 前置条件约束（与 §5.2 line 593 等价分层声明保持一致）：(a) **值域 / DB 来源等价（恒成立）** —— 四处类型都是 `number (int)` 枚举 1/2/3，DB 来源都映射 `pets.current_state`；(b) **权威 / client 信任层等价（自 Story 14.3 起成立）** —— 自 Story 14.3 起四处都读真实值，承载相同权威级别，iOS 端解析层**不**需要为四种来源做差异化处理；**Story 14.3 落地前的临时窗口**（Story 14.2 / 14.4 先于 14.3 实装时）：`pet.state.changed` 已能广播 2/3 真实值，但 `room.snapshot` / GET `data.members[].pet.currentState` / `member.joined.payload.pet.currentState` 仍**固定返回 `1`**（详见 §12.3 `room.snapshot` line 2073 / §10.3 line 1369 placeholder 说明 / §12.3 `### 成员加入` `payload.pet.currentState` 字段说明）；client 实装层在该窗口内的**权威信号优先级**为 `pet.state.changed` WS 广播 > `room.snapshot` / GET / `member.joined`（后三者在 14.3 前是 placeholder `1`，不能反推为权威状态）。**该优先级排序适用于他人 entry**；对 **self entry** 的 HTTP `data.state` ack 信号详见 §5.2 "WS 广播 vs HTTP 响应的关系（含发起者自己的 self-broadcast 兜底规则）" + §5.2 line 612 的 self-only 优先级（`pet.state.changed` WS 广播 > `state-sync` HTTP `data.state` (ack) > `room.snapshot` / GET / `member.joined`）—— self entry 与他人 entry 走不同路径：self entry 有 HTTP 200 + self-broadcast 两路信号到达本端 client，HTTP `data.state` 是 server-acknowledged ack 兜底信号（适用 §5.2 line 547-551 对称到达顺序规则）；他人 entry 在 client 端**没有**对应的 HTTP 信号（HTTP 是 caller 自己的请求响应，**不**承载别人状态），因此他人 entry 优先级表不含 HTTP 层。14.3 落地后四处 server → client 字段统一切换到权威等价层。如四处任一在权威等价生效后仍返回不同值表示 server 端状态不一致（race condition），client 实装层**应**以"最近一次 WS 广播"为最新真相源 —— 此处"最近"由**同一 WS 连接内消息的物理到达顺序（FIFO 保证）**决定，**不**依赖 `ts` 字段做新旧比较（`ts` 是日志关联 / UI 排序辅助信号，详见 §12.2 line 1961，禁止用作业务排序判定）；reconnect 后由 `room.snapshot` 全量重新对齐 + 14.3 落地后的权威等价层兜底，进一步避免 client 维护跨连接的乱序判定逻辑
- `payload.userId == 当前 user.id` 的"自己的状态广播"也是合法消息（server 不过滤），client **应**正常接收 + 走字段级 merge（与"别人的广播"统一处理路径），**禁止** client 仅因 `userId == self` 而丢弃消息（保留 self-broadcast 作 §5.2 "self-broadcast 的剩余职责" 中所述跨设备一致性校验 + WS 链路活性探测信号）；按 §5.2 self-broadcast 兜底规则（**基于到达顺序对称**）：(a) 若 HTTP 200 先到 → 本地 UI 已由 HTTP 200 驱动到目标状态 → self-broadcast 到达时 merge 结果是 no-op（值已相同）；(b) 若 self-broadcast 先到 → 本地 UI 由 WS 广播立即驱动（与"别人的广播"统一路径）→ 后续 HTTP 200 到达时为 no-op（仅作 server 端入账成功二次确认）；两路径均**不**触发"状态闪烁"，client 实装层**不**假设固定到达顺序
- 广播 fire-and-forget：如该房间内某些 Session 因网络抖动 / 已 close / SessionManager 状态不一致导致广播失败，server **不**重试（仅 log warning）—— client 实装层（iOS Story 15.x）**不**应假设每条 `pet.state.changed` 都到达；状态对齐 fallback：(a) **对别人**的状态：由别人下次状态切换时再次广播 / WS 重连后 `room.snapshot` 全量重新下发兜底（在 14.3 落地权威等价层后；落地前 `room.snapshot` 该字段是 placeholder `1`，详见 §5.2 line 593 临时不一致窗口说明）；(b) **对自己**的状态（self-broadcast 丢失场景）：由 §5.2 self-broadcast 兜底规则**对称展开**覆盖 —— 若 HTTP 200 先到达（典型）则发起者本地 UI 由 HTTP 200 立即驱动，self-broadcast 即便完全丢失也不会造成自己 UI 永久 stale；若 self-broadcast 是先到信号（罕见路径）但**整体丢失**（既未先到也未后到），则由 HTTP 200（必到达 —— 若 HTTP 都失败 client 已知 state-sync 调用失败，重试机制由 Story 15.4 实装）驱动自己 UI；房间内**其他成员**对该发起者的状态视图若 self-broadcast 丢失则需走 (a) 兜底路径（无 HTTP 替代信号）。这条 fire-and-forget 语义与 Story 10.5 BroadcastToRoom primitive、Story 11.8 `member.joined` / `member.left` 广播失败语义一致
- `ts` 字段（int64 ms）来源是 server 端 `time.Now().UnixMilli()`（具体调用方式由 Story 14.4 实装层决定），与 `### 成员加入` / `### 成员离开` 的 `ts` 字段语义一致；用途**仅限**客户端日志关联 + UI 辅助展示（如显示"X 秒前更新"），**禁止**用作业务排序 / 状态新旧判定（与 §12.2 line 1961 全局 WS envelope `ts` 字段约束一致 —— `ts` 不是业务时序信号，client 不应基于 `ts` 比较推断"哪条 `pet.state.changed` 更新"，因为 time skew 可能存在）；状态新旧判定由 (i) 同一 WS 连接内消息的物理到达顺序（FIFO 保证）+ (ii) reconnect 时 `room.snapshot` 全量重新对齐 + (iii) Story 14.3 落地后的权威等价层共同兜底

> **Future Fields（节点 5 阶段为占位 / 后续 epic 落地）**：
>
> - 节点 5 阶段 `pet.state.changed` payload 仅含 `userId` / `petId` / `currentState`，**不**含 `equips` / `equips[].renderConfig` 等装备字段（装备变更走独立路径：Epic 27 `POST /cosmetics/equip` / `POST /cosmetics/unequip` 接口；本消息**仅**广播 currentState 变化）。后续若产品需要"装备变更广播"语义，由对应 epic 单开 WS 消息（如 `pet.equips.changed`）而**不**扩展 `pet.state.changed` payload（保持每条业务 WS 消息单一职责）。

### 收到表情广播（emoji.received）

**触发**：

- WS 客户端发送 `emoji.send`（详见 §12.2 `### 发送表情`）→ server 端 EmojiHandler 完成 5 步校验（鉴权 / 参数校验 / 房间归属校验 / 表情合法性校验 / 广播）全部通过后，service 层调用 `BroadcastToRoom(currentRoomId, {type: "emoji.received", payload: {userId, emojiCode}})` 广播给该房间内所有当前在线 Session（**包含**发起者自己 —— 与 `### 成员加入` `### 成员离开` 排除发起者**不同**语义，与 `### 宠物状态变更（pet.state.changed）` 包含发起者同语义）；广播失败 fire-and-forget（仅 log warning，**不**回 error 给发起者）
- 节点 6 阶段**仅** Story 17.5 一处触发；任何 WS 层事件（含握手、心跳超时、断开重连、用户进出房间）**都不**触发 `emoji.received`；后续 epic 不在本路径加新触发条件（如假设性的"server 主动广播促销 emoji"等不在 MVP 范围）

**字段**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | string | 必填 | 固定值 `"emoji.received"` |
| `requestId` | string | 必填 | 固定 `""`（主动推送类消息，遵循 §12.3 通用信封；**不**回带 `emoji.send.requestId` —— 广播 fanout 给房间内所有 Session，server 端无法对所有接收者都"配对" `emoji.send.requestId`；发起者自己的 self-broadcast 也走广播路径，故 `requestId` 同样固定 `""`） |
| `payload.userId` | string | 必填 | 发送表情的 user 主键（BIGINT 字符串化，遵循 §2.5）；来自 `emoji.send` 当前 user.id（即 WS 握手 token 解码后的 user.id） |
| `payload.emojiCode` | string | 必填 | 客户端发送的表情业务标识符；server 已在 §12.2 `### 发送表情` 服务端逻辑步骤 4 校验过该 `emojiCode` 必然存在于 `emoji_configs` 且 `is_enabled=1`（client 收到 `emoji.received` 时无需再次校验 —— server 端为 single source of truth）；与 §11.1 `data.items[].code` 同语义；client 用作查找 §11.1 缓存表情列表的 key，定位 `assetUrl` / `name` 等渲染所需字段 |
| `ts` | number (int64) | 必填 | 服务端发送时间戳（ms）；遵循 §12.3 通用信封约束 |

JSON 示例（与本节字段表对齐）：

```json
{
  "type": "emoji.received",
  "requestId": "",
  "payload": {
    "userId": "1002",
    "emojiCode": "wave"
  },
  "ts": 1776920345000
}
```

**关键约束**：

- 广播范围：**该房间内所有当前在线 Session**（**包含**发起者自己）—— 与 `### 成员加入` 排除加入者自己、`### 成员离开` 排除离开者自己的语义**不同**；与 `### 宠物状态变更（pet.state.changed）` 包含发起者**同**语义。原因：(a) `member.joined` / `member.left` 是"成员关系变化通知"，发起者已通过 HTTP response 知道结果，再收一份 WS 通知是冗余；(b) `emoji.received` 是"表情事件本身"，server **不**区分 broadcast 接收者是发起者 / 其他成员（统一发给房间内全员），让 client 端用单一 WS 入口处理表情事件，**简化 server 实装**。**发起者自己 self-broadcast 的 UI 处理职责（与 `pet.state.changed` 语义不同）**：client 端**应**对 `payload.userId == 当前 user.id` 走"跳过 / 不触发动效"路径（iOS Story 18.4 钦定）—— 表情是 transient UI 事件，本地动效已在 SendEmojiUseCase 触发瞬间（Story 18.3）播完，self-broadcast 仅作为 server 端 "广播路径完整执行了" 的探测信号（log debug，不驱动 UI）；这与 `pet.state.changed` "self-broadcast 与 HTTP 200 ack 任一先到都驱动 UI" 的双轨兜底**不同**（`pet.state.changed` 是持久化状态变更，self-broadcast 是状态对齐的双轨之一；`emoji.received` 是 transient 事件，无 stale 风险，跳过自己 echo 完全无副作用）
- `payload.userId` / `payload.emojiCode` 都必填（**禁止** payload 为 `{}` 或缺任一字段）；缺字段视为契约违反，client 解析层**应**走"安全忽略 + log warn"路径（与 Story 10.1 `安全忽略未识别 type` 相同的容错策略，避免单条 malformed 消息把房间页搞崩）
- client 收到 `emoji.received` 后**应**按以下规则处理（与 §12.3 `room.snapshot` client merge contract 字段级 merge **不同语义** —— 表情是 transient 事件，**不**进入 client roster / 持久化状态）：
  - (a) `payload.userId == 当前 user.id`（self-broadcast）→ **跳过**（本地动效已播过）；log debug "self-emoji-broadcast received"
  - (b) `payload.userId ∈ 当前房间 roster` 且 `payload.userId != 当前 user.id` → 在该成员 PetSpriteView 上方触发飞出动效（iOS Story 18.4 实装）
  - (c) `payload.userId` 不在 roster（**合法 race window**，**不**作契约违反处理）—— 触发场景：sender A 发送 `emoji.send` 后**立即** leave 房间，server 端 `emoji.received` 广播与 `member.left` 广播走**不同**服务端路径（Story 17.5 EmojiService BroadcastToRoom vs Story 11.x RoomService leave 路径），到达 receiver B 的物理顺序**不保证**严格一致；可能出现 receiver B 先收 `member.left`（已把 A 从本地 roster 移除）→ 后收 A 的 `emoji.received`，此时本地 roster 已查不到 A。client **不**得简单 drop，应执行**降级渲染**：① 优先使用 `payload` 自带字段（`userId` / `emojiCode`）渲染飞出动效；② 由于 `emoji.received` payload **不**自带 sender 头像 / 昵称（见本节"Future Fields"，仅含 `userId` + `emojiCode`，渲染所需的 `assetUrl` / `name` 由 §11.1 表情缓存查），client 端无法在该用户原 PetSpriteView 位置触发动效（其 sprite 已随 `member.left` 移除）→ 降级到**房间中心位置**（或屏幕中央安全区）展示该表情飞出动效；log info "emoji.received after member.left race, fell back to center anchor"（**不** log warn / error —— 这是预期合法行为）；iOS Story 18.4 实装层需具体落实"无 anchor 时的中心位置降级"渲染策略
  - (d) `payload.emojiCode` 不在 client 已缓存的表情列表（理论不该 —— server 已在 §12.2 服务端逻辑步骤 4 校验 emojiCode 合法；除非 race condition：用户 A 持 stale cache，用户 B 发送新 seed 的 emoji，A 收到 emoji.received 时本地 cache 找不到）→ 显示 fallback 占位（如问号 / 默认 emoji icon），**不**报错（与 §12.3 `pet.state.changed` `payload.currentState` 默认 fallback 渲染同语义）
- **广播 fire-and-forget**：如该房间内某些 Session 因网络抖动 / 已 close / SessionManager 状态不一致导致广播失败，server **不**重试（仅 log warning）—— client 实装层（iOS Story 18.x）**不**应假设每条 `emoji.received` 都到达；表情是 transient 事件，**不**存在"状态对齐 fallback"路径（与 `pet.state.changed` 不同 —— `pet.state.changed` 有 reconnect 后 `room.snapshot` 全量重新对齐兜底；`emoji.received` 没有兜底，丢失即丢失，UX 表现为"对方看不到我的表情"，是可接受的弱网降级行为）。这条 fire-and-forget 语义与 Story 10.5 BroadcastToRoom primitive、Story 11.8 `member.joined` / `member.left` / Story 14.4 `pet.state.changed` 广播失败语义一致
- `ts` 字段（int64 ms）来源是 server 端 `time.Now().UnixMilli()`（具体调用方式由 Story 17.5 实装层决定），与 `### 成员加入` / `### 成员离开` / `### 宠物状态变更` 的 `ts` 字段语义一致；用途**仅限**客户端日志关联 + UI 辅助展示，**禁止**用作业务排序 / 表情新旧判定（与 §12.2 line 1961 全局 WS envelope `ts` 字段约束一致）；表情新旧判定由 **同一 WS 连接内消息的物理到达顺序（FIFO 保证）** 决定，**不**依赖 `ts` 字段
- **不持久化**：server 端**不**把 `emoji.received` 事件写入任何表（与 `pet.state.changed` 触发的 `pets.current_state` UPDATE **不同**；表情按数据库设计 §14.3 不入库）；client 端 reconnect 后**收不到**历史表情事件（无历史可重放，与 `room.snapshot` 不下发"近期表情列表"语义一致）；如未来产品要求"历史表情回放"或"未读提醒"，由对应 epic 决定是否引入 `emoji_events` 持久化表 + 接口

> **Future Fields（节点 6 阶段为占位 / 后续 epic 落地）**：
>
> - 节点 6 阶段 `emoji.received` payload 仅含 `userId` / `emojiCode`，**不**含 `assetUrl` / `name` 等渲染字段（client 端通过 §11.1 缓存的表情列表查 `emojiCode` 对应的 `assetUrl` / `name` —— 避免每次广播都重复传递静态配置数据，减少 WS 流量）。后续若产品要求"广播携带动效配置"（如 server 控制每个表情的动画时长 / 路径），由对应 epic 单开 WS 消息或扩展 `emoji.received` payload，本 story 不预留字段。
> - 节点 6 阶段**不**支持"表情自定义文案"（如类似 Slack reaction 的自定义文字），server 不广播自定义文本字段；如未来产品引入文本表情或 reaction，由对应 epic 决定是否新增字段或独立 WS 消息。

### pong（心跳响应）

**触发**：服务端收到客户端 `ping`（§12.2 ping 小节）后**立即**回复 `pong`（同步路径，不走异步队列）。

**字段**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | string | 必填 | 固定值 `"pong"` |
| `requestId` | string | 必填 | **必须**回带 client `ping.requestId`；client 未提供 → 回 `""` |
| `payload` | object | 必填 | 固定空对象 `{}` |
| `ts` | number (int64) | 必填 | 服务端响应时间戳（ms） |

JSON 示例（与本节字段表对齐，含 `requestId`）：

```json
{
  "type": "pong",
  "requestId": "msg_001",
  "payload": {},
  "ts": 1776920345000
}
```

> 注：`requestId` 回带客户端 `ping.requestId`；客户端 `ping` 未提供时回 `""`。该示例可作为 Story 10.3（server 实装） / Story 12.3（iOS 解析层 Codable 结构与 mock fixture）的复制粘贴样本使用。

**关键约束**：

- `pong` 是同步响应（同一 ping 事件触发，server 立即回；不通过 broadcast / 业务路径）
- `pong.requestId` 必须等于 `ping.requestId`（含 `""` 情况），方便 client 测心跳 RTT
- `pong` **不**是广播（仅回给发 ping 的连接），server 实装层面与 `BroadcastToRoom`（Story 10.5）走不同路径

### error（错误消息）

**触发**：服务端处理 client 消息或主动事件时遇到业务错误，但**不**够严重到 close 连接（如表情 code 不存在、临时性资源问题等）。严重错误（如 Session 内部 panic）走 close 流程而非 error 消息（详见 §12.1 close code 表）。

**字段**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | string | 必填 | 固定值 `"error"` |
| `requestId` | string | 必填 | 如该 error 是某 client 请求的响应（如 `emoji.send` 失败），回带原 `requestId`；如是 server 主动错误（如内部状态异常），固定 `""` |
| `payload.code` | number (int) | 必填 | 错误码，复用 §3 全局错误码定义（如 `7001` 表情不存在 / `1009` 服务繁忙；**注**：`6005` 房间状态异常**不**用于握手后初始 `room.snapshot` 构建失败 —— 该场景走 close 1011，详见 §12.1 close code 表 1011 行 + 本节末"节点 4 阶段适用错误码"表下的注） |
| `payload.message` | string | 必填 | 错误描述，可读字符串（不做国际化，与 §3 message 字段一致） |
| `ts` | number (int64) | 必填 | 服务端发送时间戳（ms） |

JSON 示例（与本节字段表对齐，含 `requestId`）：

```json
{
  "type": "error",
  "requestId": "msg_001",
  "payload": {
    "code": 7001,
    "message": "emoji not found"
  },
  "ts": 1776920345000
}
```

> 注：当 error 是某 client 请求的响应（如 `emoji.send` 失败）时，`requestId` 回带原请求 `requestId`；当 error 是 server 主动推送（如运行时业务状态异常）时，`requestId` 固定 `""`。该示例可作为 Story 10.3 / Story 12.3 的复制粘贴样本使用。
>
> **不在 `error` 范围**：握手后初始 `room.snapshot` 构建失败**不**走 `error` 路径，统一走 close 1011（详见 §12.1 close code 表 1011 行 + 本节末"节点 4 阶段适用错误码"表下的注）。本节 `error` 仅承载"连接已可用、业务侧出问题但不致死"的非致命错误。

**节点 4 阶段适用错误码**（仅 Epic 10 协议骨架范围内）：

| code | 触发条件（节点 4 阶段） |
|---|---|
| 1009 | 服务繁忙（Session 内部异常但不致死，如临时 Redis 慢；client 应忽略并继续） |

> 业务错误码（如 `7001` 表情不存在）由对应 Epic 各自的 §X.1 锚定（如 Story 17.1）；本 story 不预先列入节点 4 阶段适用错误码表。
>
> **注（snapshot 构建失败的处理路径）**：握手完成后 Story 10.7 SnapshotBuilder 构建初始 `room.snapshot` 抛 error 时，server **不**走 "推 `error` 消息" 路径 —— 因为 §12.1 钦定 `room.snapshot` 是握手成功后必发的第一条消息，若仅推 `error` 而保持连接，client 会永远等待一个永不到达的 snapshot，房间页无法初始化、auto-reconnect 也不会触发。该场景统一走 close 路径：close **1011**（reason = `"snapshot build failed"`，详见 §12.1 close code 表 1011 行），客户端按 1011 语义自动重连（指数退避 + 最大重试次数限制）。错误码 `6005`（房间状态异常）保留给后续业务 epic 在房间已可用之后的运行时状态错误推送（如 Story 11.x / 14.x 业务流程中），**不**用于初始 snapshot 失败场景。

> **业务消息延后锚定**：上文 `### 成员加入` / `### 成员离开` / `### 宠物状态变更` / `### 收到表情广播` 四个小节已全部锚定 —— 已由 **Story 11.1**（Epic 11 房间业务契约，节点 4 中段；锚定 revision 见 `git log --grep='story-11-1'` 检出的 Story 11.1 收官 commit `chore(story-11-1): ...`，以及 `_bmad-output/implementation-artifacts/sprint-status.yaml` 中 `11-1-接口契约最终化` 状态行）/ **Story 14.1**（Epic 14 宠物状态同步契约，节点 5；锚定 revision 见 `git log --grep='story-14-1'` 检出的 Story 14.1 收官 commit `chore(story-14-1): ...`，以及 `_bmad-output/implementation-artifacts/sprint-status.yaml` 中 `14-1-接口契约最终化` 状态行）/ **Story 17.1**（Epic 17 表情广播契约，节点 6；锚定 revision 见 `git log --grep='story-17-1'` 检出的 Story 17.1 收官 commit `chore(story-17-1): ...`，以及 `_bmad-output/implementation-artifacts/sprint-status.yaml` 中 `17-1-接口契约最终化` 状态行）正式锚定字段表 + 触发条件 + 关键约束 —— 自此**接入** Epic 11 / Epic 14 / Epic 17 / 节点 4 / 节点 5 / 节点 6 server / client active message set。所有 §12.3 业务消息（`member.joined` / `member.left` / `pet.state.changed` / `emoji.received`）的字段层契约已 100% 锚定，节点 6 之后无新增业务消息字段层契约任务。
>
> 各消息字段层契约的正式锚定 epic 如下（**已升级状态以粗体标注**）：
>
> - `member.joined` / `member.left` → **Story 11.1（已锚定）**（Epic 11 房间业务契约，节点 4 中段）
> - `pet.state.changed` → **Story 14.1（已锚定）**（Epic 14 宠物状态同步契约，节点 5）
> - `emoji.received` → **Story 17.1（已锚定）**（Epic 17 表情广播契约，节点 6）

> **server / client active message set 升级**（自 Story 11.1 起生效，覆盖 Story 10.1 r10 钉死的"Epic 10 阶段冻结"边界；自 Story 14.1 起 server → client 集合再次扩展；自 Story 17.1 起 server → client + client → server 双向集合再次扩展）：
>
> - **server → client active message set**：`room.snapshot` / `pong` / `error`（Epic 10 锚定）+ `member.joined` / `member.left`（Story 11.1 锚定）+ `pet.state.changed`（Story 14.1 锚定）+ **`emoji.received`（本 story 锚定）**
> - **client → server active message set**：`ping`（Epic 10 锚定）+ **`emoji.send`（本 story 锚定，节点 6 起 client 首次获得新业务消息发送能力）**
>
> 客户端 Story 12.3 / 12.4 / 15.2 / 18.4 解析层在节点 4 / 节点 5 / 节点 6 阶段**应**对未识别的 `type` 值走"安全忽略 + log warn"路径，**不**因未识别消息 close 连接 / crash app（Story 10.1 既有规则，本 story 沿用）。

---

## 13. 幂等与防重

## 13.1 需要幂等的接口

建议以下接口支持 `idempotencyKey`：

- `POST /api/v1/chest/open`（**节点 7 已锚定 DB 持久化方案**，详见 §7.2 服务端逻辑步骤 3 / 5a / 5k / 7 + §13.2 / §13.3）
- `POST /api/v1/compose/upgrade`（节点 11 / Epic 32 落地，预期复用同一 DB 持久化模式 —— 起新表 `compose_upgrade_idempotency_records` 或共用通用表，由 Story 32.4 锚定）

## 13.2 幂等规则

同一用户、同一接口、同一 `idempotencyKey`：

- 第一次成功，后续重复请求返回第一次结果（`status = 'success'`，server 直接返回缓存的 `response_json` + 实时**同源同时刻**补算时间派生字段 `nextChest.status` 与 `nextChest.remainingSeconds`（与 §7.1 GET /chest/current 一致）+ 实时填入**当前**请求的顶层 `requestId`）
- 第一次处理中（事务尚未提交），同 key 并发请求被 InnoDB unique-key X-lock 阻塞排队等待首个事务结束（不返回 1008；首个事务结束后再分支到 `success` 或全流程）；仅在极窄 race 兜底路径下返回 **1008**
- 第一次失败（事务回滚，pending 行也一起回滚），同 key 重试在事务内首条 INSERT 拿 `affected_rows = 1` 等价于首次到达走全流程；**无任何 server 端 best-effort 补偿写**（r7 review 锁定移除：rollback 路径的事务原子性已保证"无副作用 + 同 key 重试安全"，写 `failed` 占位行反而引入 race condition 把后到达的 success 行错误覆盖为 failed）

## 13.3 持久化存储

**r7 review 锁定**：幂等记录使用 **MySQL 同事务持久化**（非 Redis 缓存）；**预声明 + 业务写入 + 最终化**全部在同一业务事务内原子提交；schema 为二态机 `status ENUM('pending', 'success')`。**r10 review 锁定**：rate_limit 检查在 handler 内层、幂等命中预检（步骤 3）之后做（步骤 4），cached replay 不计入配额。详见数据库设计 §5.16 `chest_open_idempotency_records` 表 schema + V1接口设计 §7.2 服务端逻辑步骤 3 / 5a / 5k + 关键约束「r7 移除 best-effort failed upsert 决策」+「rate_limit 位置 r10 调整」段。

**为什么不用 Redis**：Redis 是非事务存储，与 MySQL 不能形成原子写。"事务 commit 成功 + Redis SET 失败"的 case 下 client 无法区分"首次已生效"vs"首次未生效"，导致后续重试可能重复扣步数 + 重复出箱（r4 review 已锁定该缺陷，r5 通过 DB 同事务持久化根治；r6 进一步把预声明纳入业务事务，消除"事务外独立 INSERT 留 pending 卡死"悖论；r7 移除 best-effort failed upsert 消除"同 key 重试 success 被异步 failed 覆盖"race condition）。

**实装关键字段**（详见 §5.16）：

- `(user_id, idempotency_key)` UNIQUE 约束 → 原子声明 + 并发阻塞排队（InnoDB unique-key X-lock）
- `status` ENUM('pending', 'success') → **二态机**（r7 review 锁定，从 r6 的三态机简化）；`pending` 仅在事务持锁期间出现（对锁等待者不可见），事务 commit 推进到 `success`，事务 rollback 让 pending 行消失（**无** `failed` 状态 —— 详见 §5.16 + V1接口设计 §7.2 r7 决策段）
- `response_json` JSON NULL → 缓存首次成功响应，**不**包含时间派生字段（`nextChest.status` 与 `nextChest.remainingSeconds` 均是 time-derived，由 server 在响应序列化时按"当前时刻"**同源同时刻**实时计算填入；r9 review 锁定 `nextChest.status` 与 `remainingSeconds` 同处理 —— 防止重试发生在新 chest 已到期解锁时刻回放 stale `status=1` + 实时 `remainingSeconds=0` 不可能组合）/ **不**包含顶层 `requestId`（每次请求独立 trace ID，重试请求需重新填**当前**请求的 trace ID 以维持 log / trace 关联语义，r7 review 锁定）

---

## 14. 关键事务建议

## 14.1 开箱事务

必须放在一个事务里：

- 校验宝箱状态
- 扣 1000 步数
- 发放装扮实例
- 写开箱日志
- 刷新下一轮宝箱

> **节点 7 vs 节点 8 阶段差异（与 §7.2.4 一致）**：本节描述的是**最终契约**（节点 8 / Epic 23 完成后稳态）。**节点 7 阶段（Story 20.6 / Epic 21 验收期）**「发放装扮实例」步骤暂不执行 —— `chest_open_logs.reward_user_cosmetic_item_id` 写占位 `0`，`user_cosmetic_items` 实例**不**创建；详见 §7.2.4h 与 §7.2 节点 7 vs 节点 8「关键约束」。Story 23.5 落地后回归本节最终契约语义。

## 14.2 穿戴事务

建议放在一个事务里：

- 校验实例状态
- 校验槽位
- 卸下旧装备
- 装备新实例
- 更新实例状态

## 14.3 合成事务

必须放在一个事务里：

- 锁定 10 个实例
- 校验归属、状态、品质
- 标记 consumed
- 创建奖励实例
- 写合成日志
- 写材料日志

## 14.4 加入房间事务

建议放在一个事务里：

- 校验用户不在其他房间
- 校验房间人数
- 插入成员关系
- 更新用户当前房间

---

## 15. 前端推荐调用顺序

## 15.1 App 启动

- `POST /api/v1/auth/guest-login`
- `GET /api/v1/home`

## 15.2 首页展示

- `GET /api/v1/home`
- 本地运行倒计时
- 关键时机用 `GET /api/v1/chest/current` 纠正状态

## 15.3 开箱前

推荐顺序：

1. `POST /api/v1/steps/sync`
2. `POST /api/v1/chest/open`

## 15.4 进入装扮页

- `GET /api/v1/cosmetics/inventory`

## 15.5 进入合成页

- `GET /api/v1/compose/overview`
- `GET /api/v1/cosmetics/inventory`

## 15.6 进入房间

- `GET /api/v1/rooms/{roomId}`
- 建立 WebSocket 连接

---

## 16. 当前 V1 接口清单

```text
POST   /api/v1/auth/guest-login
POST   /api/v1/auth/bind-wechat
GET    /api/v1/me

GET    /api/v1/home
POST   /api/v1/pets/current/state-sync

POST   /api/v1/steps/sync
GET    /api/v1/steps/account

GET    /api/v1/chest/current
POST   /api/v1/chest/open

GET    /api/v1/cosmetics/catalog
GET    /api/v1/cosmetics/inventory
POST   /api/v1/cosmetics/equip
POST   /api/v1/cosmetics/unequip

GET    /api/v1/compose/overview
POST   /api/v1/compose/upgrade

POST   /api/v1/rooms
GET    /api/v1/rooms/current
GET    /api/v1/rooms/{roomId}
POST   /api/v1/rooms/{roomId}/join
POST   /api/v1/rooms/{roomId}/leave

GET    /api/v1/emojis

GET    /ws/rooms/{roomId}
```

---

## 17. 后续文档建议

建议在后续继续拆分以下 Markdown 文档：

1. `数据库详细设计.md`
2. `实时通信协议设计.md`
3. `iOS工程设计.md`
4. `Go服务端工程设计.md`
5. `接口错误码规范.md`

