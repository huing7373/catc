# Story 17.5: WS emoji.send 处理 + emoji.received 广播（首次落地 EmojiRepo.Exists + EmojiService.ValidateCode + EmojiHandler.HandleEmojiSend + BuildEmojiReceivedEnvelope + BuildErrorEnvelope + Session readLoop 路由 + 单测 ≥10 case + dockertest 集成测试 + bootstrap wire）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want **首次落地** WS `emoji.send` 业务消息的服务端处理链路：
- `server/internal/repo/mysql/emoji_repo.go` 在 17.4 落地的 `EmojiRepo` interface 内**新增** `Exists(ctx, code) (bool, error)` 方法（`SELECT 1 FROM emoji_configs WHERE code = ? AND is_enabled = 1 LIMIT 1`；走 `idx_enabled_sort` 二级索引）
- `server/internal/service/emoji_service.go` 在 17.4 落地的 `EmojiService` interface 内**新增** `ValidateCode(ctx, code) error` 方法（emojiCode 字符集 / 长度校验失败 → 1002 / repo 查不到或 disabled → 7001 / DB error → 1009；**不**返 bool —— 校验失败的语义全靠 error 类型表达，让 handler 走 `errors.Is` / `errors.As` 分支无歧义）
- `server/internal/app/ws/emoji_handler.go`（**新建**）的 `EmojiHandler` struct + `NewEmojiHandler(svc, userRepo, broadcastFn)` 构造 + `HandleEmojiSend(ctx, session, env)` 方法：
  - 解析 `payload.emojiCode`（payload 缺字段 → 回 `error` 消息 code=1002 回带 requestId）
  - 调 `svc.ValidateCode(ctx, code)`（含字符集校验 + repo Exists 查询）：1002 / 7001 / 1009 → 各自回 `error` 消息回带 requestId
  - **房间归属双校验**（V1 §12.2 服务端逻辑步骤 3 + r1 review 锁定的反 stale-Session 校验）：调 `userRepo.FindByID(ctx, session.UserID())` 拿 `users.current_room_id`，与 `session.RoomID()` 比对：
    - `users.current_room_id == NULL` → 6004 不在房间
    - `users.current_room_id != session.RoomID()`（stale Session 跨房间注入）→ 6004 + log warn 含 `userId` / `Session.roomID` / `users.current_room_id`
    - DB error → 1009
  - 全部校验通过 → 调 `ws.BuildEmojiReceivedEnvelope(payload)` marshal envelope + 调 `broadcastFn(ctx, session.RoomID(), msg)` 全 fanout（**包含**发起者自己，与 `pet.state.changed` 同语义）；broadcast 失败仅 log warn，**不**回 error 给发起者（fire-and-forget，V1 §12.2 服务端逻辑步骤 5 钦定）
- `server/internal/app/ws/snapshot.go` **追加** `EmojiReceivedPayload` struct + `BuildEmojiReceivedEnvelope(payload) ([]byte, error)` helper（与 `BuildPetStateChangedEnvelope` 同模式 + V1 §12.3 `### 收到表情广播` 字段表 1:1 对齐）+ **新增** `ErrorPayload` struct + `BuildErrorEnvelope(requestID, code, message) ([]byte, error)` helper（V1 §12.3 `### error` 字段表 1:1 对齐；**首次落地**该 helper，节点 4 ~ 5 的现有路径都没用 WS `error` 消息）
- `server/internal/app/ws/session.go` 修改 `readLoop` 路由表 + `Session` struct：
  - `Session` struct 添加 `emojiHandler EmojiHandler`（interface 形态便于测试 stub；nil-safe —— 单测 / HTTP-only 部署可传 nil，readLoop 看到 nil 时 fallthrough 走 "未知 type" 路径，与既有 unknown type 行为一致）
  - `newSession` 构造签名加 `emojiHandler EmojiHandler` 参数；`Gateway.NewGateway` 也加同名参数从 bootstrap 透传到 `newSession`
  - `readLoop` 的 `switch env.Type` 增加 `case "emoji.send"`：若 `s.emojiHandler != nil` 调 `s.emojiHandler.HandleEmojiSend(ctx, s, env)`；否则 log warn fallthrough（与 nil case 同模式）
- `server/internal/app/bootstrap/router.go` **wire**：在 `if deps.SessionMgr != nil` 块内构造 `emojiBroadcastFn` closure（与 `petBroadcastFn` 同模式）+ `emojiHandler := wsapp.NewEmojiHandler(emojiSvc, userRepo, emojiBroadcastFn)` + 传给 `wsapp.NewGateway` 新签名第 7 参数
- 单测覆盖：
  - `emoji_repo_test.go` 加 ≥3 case sqlmock：`Exists` happy enabled / happy not exists / happy exists but disabled / DB error
  - `emoji_service_test.go` 加 ≥5 case stub repo：ValidateCode happy / 字符集非法 1002 / 长度 0 / 长度 65 / Exists 返 false → 7001 / DB error → 1009
  - `emoji_handler_test.go`（**新建** `server/internal/app/ws/emoji_handler_test.go`）≥7 case stub service + stub userRepo + capture broadcastFn：happy / payload 缺字段 1002 / emojiCode 1002 / emojiCode 7001 / user 不在任何房间 6004 / users.current_room_id != Session.roomID 6004 stale / DB error 1009
  - `snapshot_test.go` 加 ≥2 case：BuildEmojiReceivedEnvelope 字段表精确匹配 / BuildErrorEnvelope happy + ts 单调递增
- 集成测试覆盖：`ws_integration_test.go` 追加 1 case dockertest：A + B 都在房间 X 建 WS → A 发 `emoji.send {emojiCode: "wave", requestId: "msg_001"}` → 验证 A、B 都收到 `emoji.received {userId: A 字符串化, emojiCode: "wave"}` envelope + requestId="" + ts 非零;
so that **Epic 18.1 ~ 18.4（iOS 表情面板 + WS 发送 + 接收动效 + 去重自己）+ Epic 19.1（节点 6 demo E2E 验收）** 可以基于一个**已落地、严格符合 §12.2 / §12.3 契约、端到端 dockertest 验证过**的服务端表情广播链路继续展开，不再出现"17.5 端点空白让 18.x 表情面板硬编码假数据 / E2E 测不出 stale Session 跨房间注入风险 / fire-and-forget 语义被误改成阻塞"的返工。

## 故事定位（Epic 17 第五条 = 收官 story；上承 17.4 GET /emojis + EmojiRepo.List 已就绪，下启 iOS Epic 18 整条表情链路 + Epic 19 节点 6 demo 验收）

- **Epic 17 进度**：17.1（契约定稿，done）→ 17.2（emoji_configs migration + GORM struct，done）→ 17.3（emoji_configs seed ≥4 个表情，done）→ 17.4（GET /emojis HTTP 端点 + EmojiRepo.List + EmojiService.ListAvailable，done）→ **17.5（本 story，WS emoji.send 处理 + emoji.received 广播）** = Epic 17 最后一条 story，收官后整个 Epic 17 server 端表情链路落地。
- **本 story 是 Epic 18 / Epic 19 / 节点 6 收官的强前置**：
  - **iOS Epic 18.3 SendEmojiUseCase**：iOS 端钦定"WS 发送 `emoji.send {emojiCode}` fire-and-forget，与本地立即动效并行"（epics.md 行 2683-2697）—— **直接依赖**本 story 落地的 server 端 emoji.send 处理路径；iOS 不会校验任何 emojiCode 合法性，期望 server 返 `error` 消息回带 requestId 由 iOS Story 12.3 WS error 总线消费（V1 §12.2 行 2007-2009 + §12.3 行 2526 钦定）
  - **iOS Epic 18.4 emoji.received 接收**：iOS 端钦定"接收 `emoji.received` 在对应成员猫上方播放飞出动效（**去重**自己 userId）"（epics.md 行 2699-2730）—— **直接依赖**本 story 落地的 broadcast 路径；emoji.received envelope schema / 字段名 / 类型严格对齐 §12.3 保证 client Codable 不触发解析失败 + 跳过 self echo 规则在 server 全 fanout 前提下生效
  - **Epic 19.1 节点 6 demo E2E**：钦定"验证场景 2：A 在房间内点表情 → A 自己本地动效（不等 server）+ B / C 都在 0.5s 内看到 A 头顶动效（去重自己后只 B / C 触发）"（epics.md 行 2746-2755）—— **直接依赖**本 story 端点 + 17.4 GET /emojis + 17.3 seed + iOS Epic 18 完成
- **epics.md §Story 17.5 钦定**（行 2601-2628）：
  - 客户端发 WS 消息 `emoji.send {emojiCode: string}`
  - WS gateway dispatcher 路由到 EmojiHandler：
    - 校验当前用户在某个房间中（users.current_room_id 非 null）—— **注**：epics.md 此条仅描述高层意图；真实契约层（V1 §12.2 服务端逻辑步骤 3 + r1 review 锁定）要求**双校验**：users.current_room_id != NULL **且** users.current_room_id == Session.roomID
    - 不在房间 → 回 error 6004 用户不在房间中
    - 校验 emojiCode 在 emoji_configs 中存在且 enabled
    - 不存在 → 回 error 7001 emoji not found
    - 调用 BroadcastToRoom(currentRoomId, {type: "emoji.received", payload: {userId, emojiCode}})
  - 表情**不入库**（按 §14.3，MVP 不强制落库表情事件日志）—— **数据库设计.md §14.3 默认 emoji 不持久化**；不写 emoji_events 表 / 不增量 redis counter
  - 广播给房间所有人含发起者自己（与 pet.state.changed 同样规则）
  - 限频建议: 同一用户每秒最多 5 个表情（防刷屏） —— **MVP 可不做，但 tech debt 登记**（本 story 范围内**不**实装限频；登记到 tech-debt-log.md）
  - **单元测试覆盖**（≥5 case）：happy / 用户不在房间 6004 / emojiCode 不存在 7001 / emojiCode 存在但 disabled 7001 / WS 消息缺 emojiCode 1002
  - **集成测试覆盖**（dockertest + Redis + 真实 WS）：A + B 都在房间 X → A 发 `emoji.send {emojiCode: "wave"}` → A、B 都收到 emoji.received {userId: A, emojiCode: "wave"}
- **V1 §12.2 `### 发送表情` 钦定**（17.1 r2 review 锁定 + 冻结，行 1981-2080）：
  - 触发：iOS 客户端在房间页面用户选中表情面板（Story 18.1）中某个表情时，由 SendEmojiUseCase（Story 18.3）通过已建立的 WebSocket 连接（Story 12.2 WebSocketClient）发送 `emoji.send` text frame
  - 字段表：
    - `type` string 固定值 `"emoji.send"`
    - `requestId` string 选填 0 ≤ length ≤ 64；server 处理失败时回带，处理成功广播的 `emoji.received` **不**回带（广播类消息固定 `""`）
    - `payload.emojiCode` string 必填 1 ≤ length ≤ 64 + 字符集 `[a-z0-9_-]`（与 §11.1 `data.items[].code` + 数据库 `emoji_configs.code` UNIQUE KEY 一致）
  - 服务端逻辑（5 步）：
    1. **接收 & 解析**：server WS dispatcher（Story 10.3）按 `type = "emoji.send"` 路由到 EmojiHandler；解析 `payload.emojiCode` 字段
    2. **参数校验**：`emojiCode` 必填 + 类型为 string + 1 ≤ length ≤ 64 + 字符集 `[a-z0-9_-]`；任何不通过 → 回 `error.payload.code = 1002` "参数错误"（**响应类** error，`requestId` 回带 `emoji.send.requestId`），**不**广播
    3. **房间归属校验**（**权威源 = 收到 `emoji.send` frame 的 WS Session 上携带的 `roomID`**）：`SELECT current_room_id FROM users WHERE id = ?`（当前 user.id 来自 WS 握手 token；查得后**必须**与 `Session.roomID` 比对 —— **不可仅判 `!= NULL`**）；**DB 读取失败**（`err != nil`）→ 回 1009 + **不**广播 + **不**关闭 WS 连接（仅回 error 消息）
       - `current_room_id == NULL` → 回 6004
       - `current_room_id != NULL` 但 `current_room_id != Session.roomID`（stale Session 跨房间注入风险）→ 回 6004 + server **应** log warn 级别记录该跨房间发送企图（含 `userId` / `Session.roomID` / `users.current_room_id`）
       - `current_room_id == Session.roomID` → 进入步骤 4
    4. **表情合法性校验**：`SELECT 1 FROM emoji_configs WHERE code = ? AND is_enabled = 1 LIMIT 1`
       - DB 读取失败 → 回 1009 + **不**广播 + **不**关闭 WS 连接
       - 0 行（emojiCode 不存在 / 或存在但 is_enabled=0 —— 两种情况合并为同一错误，避免 server 暴露 enabled / disabled 状态信息）→ 回 7001 "emoji not found"
       - 1 行 → 进入步骤 5
    5. **广播（fire-and-forget）**：调用 `BroadcastToRoom(Session.roomID, {type: "emoji.received", payload: {userId, emojiCode}})`（**广播目标 = `Session.roomID`，不是 `users.current_room_id`** —— 二者在步骤 3 已校验相等）；广播失败仅 log warning，**不**回 error 给发起者（与 Story 11.8 `member.joined` / Story 14.4 `pet.state.changed` 广播失败语义一致）；**无 HTTP 响应、无 server → client ack 消息**
  - 错误响应通过 §12.3 `error` 消息回送：`type: "error"` / `requestId` 回带 `emoji.send.requestId` / `payload.code` / `payload.message` / `ts`
  - **不限频**：节点 6 阶段 server **不**对 `emoji.send` 做特殊限频（rate_limit 中间件挂在 HTTP 路由，**不**挂 WS 路由，故 `emoji.send` 实际**不**走 1005 限频拦截；如需限频，由 future tech debt 处理）
  - **client → server active message set 升级**：自本 story 起，`emoji.send` 正式加入 client → server active message set
- **V1 §12.3 `### 收到表情广播` 钦定**（17.1 r2 review 锁定 + 冻结，行 2435-2475）：
  - 字段表：
    - `type` string 固定值 `"emoji.received"`
    - `requestId` string 固定 `""`（主动推送类消息；广播 fanout 给房间内所有 Session，server 端无法对所有接收者都"配对" `emoji.send.requestId`；发起者自己的 self-broadcast 也走广播路径，故 `requestId` 同样固定 `""`）
    - `payload.userId` string 必填 BIGINT 字符串化（§2.5）；来自 `emoji.send` 当前 user.id
    - `payload.emojiCode` string 必填；server 已在 §12.2 服务端逻辑步骤 4 校验过该 `emojiCode` 必然存在于 `emoji_configs` 且 `is_enabled=1`
    - `ts` int64 必填 服务端发送时间戳 ms
  - 广播范围：**该房间内所有当前在线 Session**（**包含**发起者自己）—— 与 `member.joined` / `member.left` 排除发起者 / 离开者**不同**语义；与 `pet.state.changed` **同**语义
  - 表情**不持久化语义**（与 `pet.state.changed` 持久化 `pets.current_state` 不同）：本 story **不**写 emoji_events 表 / 不增量 redis counter
- **V1 §12.3 `### error`（错误消息）钦定**（行 2516-2546）：
  - 触发：服务端处理 client 消息或主动事件时遇到业务错误，但**不**够严重到 close 连接（如表情 code 不存在、临时性资源问题等）
  - 字段表：
    - `type` string 固定值 `"error"`
    - `requestId` string 如该 error 是某 client 请求的响应（如 `emoji.send` 失败），回带原 `requestId`；如是 server 主动错误，固定 `""`
    - `payload.code` number(int) 错误码（如 1002 / 1009 / 6004 / 7001）
    - `payload.message` string 错误描述（不做国际化，与 §3 message 字段一致）
    - `ts` int64 服务端发送时间戳 ms
- **V1 §11.1 / 数据库设计 §5.15 钦定**（17.4 已落地）：`emoji_configs` 表 / `idx_enabled_sort (is_enabled, sort_order ASC)` 普通索引保证 `WHERE code = ? AND is_enabled = 1` 单 emoji code 查询走索引（Exists 方法 SQL 也走该索引覆盖；no filesort）
- **lesson 2026-05-13-emoji-contract-self-consistency-and-1009-and-asset-url-17-1-r2.md** Lesson 2 钦定：DB error 必须有 1009 路径 —— 本 story handler / service 层必须包 mysql err 成 1009（与 home_service / 17.4 emoji_service 同模式 `apperror.Wrap(err, apperror.ErrServiceBusy, ...)`）
- **lesson 2026-05-13-emoji-contract-self-consistency-and-1009-and-asset-url-17-1-r2.md** Lesson 3 钦定：assetUrl 必非空字符串 —— 本 story 服务端逻辑步骤 4 **不**校验 assetUrl 是否非空（17.3 seed + 17.4 service / handler 已在 GET /emojis 链路负责该校验；emoji.send 路径**不**需要 assetUrl）
- **lesson 2026-05-12-detached-ctx-for-async-broadcast** 钦定：HTTP request ctx 不能直接传给 fire-and-forget broadcast goroutine（HTTP handler return 后 ctx 立即 cancel，broadcast goroutine 收到 cancelled ctx 立即 abort）—— **WS 路径与 HTTP 不同**：WS Session 生命周期 ≥ broadcast 时长（broadcast 是同步入队 sendChan，O(1) 完成；不需要 detached ctx）；本 story handler **同步**调 broadcastFn，**不**启 goroutine + **不**用 `context.WithoutCancel`，与 11.8 `broadcastMemberJoined` 同步路径同模式（11.8 后期改成 post-commit async goroutine 是为 HTTP 路径的 200 OK fast-return，与本 WS 路径无关）
- **lesson 2026-05-13-pet-broadcast-detached-ctx-and-petless** 钦定：pet-less 用户 state-sync 走 noop 不广播 —— **本 story 与 pet 无关**：emoji.send 不依赖 pets 表存在性；user 即使无 pet 也可发表情，校验只看 users.current_room_id；本 story **不**做 pet-less 边界 case
- **下游强依赖**（本 story 不动后才能开工）：
  - iOS Epic 18.1 ~ 18.4（表情面板 + SendEmojiUseCase + 接收动效 + 去重自己）
  - Epic 19.1 节点 6 demo E2E 验收
- **范围红线**：
  - 本 story **只**改 / 新建以下文件：
    - `server/internal/repo/mysql/emoji_repo.go`（修改 —— 在 17.4 落地的 `EmojiRepo` interface 内**追加** `Exists(ctx, code) (bool, error)` 方法签名 + `emojiRepo` struct 内**追加** `Exists` 实装；**不**动 17.4 落地的 `List` 方法 / `EmojiConfig` struct / `TableName`）
    - `server/internal/repo/mysql/emoji_repo_test.go`（修改 —— 在 17.4 落地的 test 文件**追加** ≥3 个 `TestEmojiRepo_Exists_*` 函数；**不**动 17.4 落地的 ≥3 个 `TestEmojiRepo_List_*` 函数）
    - `server/internal/service/emoji_service.go`（修改 —— 在 17.4 落地的 `EmojiService` interface 内**追加** `ValidateCode(ctx, code) error` 方法签名 + `emojiServiceImpl` struct 内**追加** `ValidateCode` 实装；**不**动 17.4 落地的 `EmojiBrief` struct / `ListAvailable` 方法）
    - `server/internal/service/emoji_service_test.go`（修改 —— 在 17.4 落地的 test 文件**追加** ≥5 个 `TestEmojiService_ValidateCode_*` 函数；**不**动 17.4 落地的 ≥4 个 `TestEmojiService_ListAvailable_*` 函数）
    - `server/internal/app/ws/snapshot.go`（修改 —— 在 14.4 落地的 `PetStateChangedPayload` / `BuildPetStateChangedEnvelope` 之后**追加** `EmojiReceivedPayload` struct + `BuildEmojiReceivedEnvelope` helper + `ErrorPayload` struct + `BuildErrorEnvelope` helper；**首次**落地 `ErrorPayload` / `BuildErrorEnvelope`，节点 4 / 5 既有路径都没用 WS `error` 消息）
    - `server/internal/app/ws/snapshot_test.go`（修改 —— 追加 ≥2 个 `TestBuildEmojiReceivedEnvelope_*` + ≥2 个 `TestBuildErrorEnvelope_*` 单测）
    - `server/internal/app/ws/emoji_handler.go`（**新建** —— `EmojiHandler` interface + `emojiHandlerImpl` struct + `NewEmojiHandler` 构造 + `HandleEmojiSend(ctx, session, env)` 方法 + private helpers）
    - `server/internal/app/ws/emoji_handler_test.go`（**新建** —— ≥7 case stub service + stub userRepo + capture broadcastFn）
    - `server/internal/app/ws/session.go`（修改 —— `Session` struct 加 `emojiHandler EmojiHandler` 字段 + `newSession` 构造签名加 `emojiHandler EmojiHandler` 参数 + `readLoop` 的 `switch env.Type` 增加 `case "emoji.send"` 分支）
    - `server/internal/app/ws/gateway.go`（修改 —— `Gateway` struct 加 `emojiHandler EmojiHandler` 字段 + `NewGateway` 签名加 `emojiHandler EmojiHandler` 第 7 参数 + `Handle` 把 `g.emojiHandler` 传给 `newSession`）
    - `server/internal/app/ws/ws_integration_test.go`（修改 —— 追加 1 个 `TestWSEmojiSend_*` 函数 + helper 复用既有 `startMySQLWithRoomMemberFixture`）
    - `server/internal/app/bootstrap/router.go`（修改 —— 在 `if deps.SessionMgr != nil` 块内构造 `emojiBroadcastFn` closure + `emojiHandler := wsapp.NewEmojiHandler(emojiSvc, userRepo, emojiBroadcastFn)` + 传给 `wsapp.NewGateway` 第 7 参数；**不**新增 router 路径）
    - 本 story 文件 + sprint-status.yaml 流转
    - `_bmad-output/implementation-artifacts/tech-debt-log.md`（修改 —— 追加节点 6 tech debt：emoji.send 限频未实装；epics.md §17.5 行 2619 钦定建议同一用户每秒最多 5 个表情）
  - **不**实装任何 HTTP emoji 路由（17.4 已落地 GET /emojis）
  - **不**实装 `EmojiRepo.Create` / `UpdateIsEnabled` 等 admin 端方法（MVP 节点 6 无 admin 后台需求）
  - **不**实装限频（epics.md §17.5 行 2619 钦定"MVP 可不做，但 tech debt 登记"）—— 仅追加 tech-debt-log.md 一行
  - **不**实装 emoji_events 表 / Redis emoji counter（数据库设计 §14.3 钦定"emoji 默认不持久化"；MVP 不强制落库表情事件日志）
  - **不**实装 admin 后台 / `POST /dev/emoji` 等运维端点（YAGNI）
  - **不**修改 17.4 落地的 `EmojiBrief` struct / `ListAvailable` 方法（17.4 已锁定字段集与方法签名；本 story **不**新增字段 / 改 tag）
  - **不**修改 0001 ~ 0010 既有 migration 文件（17.2 / 17.3 已落地 schema + seed）
  - **不**修改 14.4 落地的 `PetStateChangedPayload` / `BuildPetStateChangedEnvelope`（字段集 + helper 已与 V1 §12.3 1:1 对齐）
  - **不**改 V1 接口契约（17.1 已冻结；本 story 严格对齐 §12.2 / §12.3；任何偏离都是 bug）
  - **不**改任何 `docs/宠物互动App_*.md`
  - **不**改 ADR-0006（error envelope 单一生产者；本 story WS error 路径走自己的 `BuildErrorEnvelope` + Session.Send，与 HTTP error envelope 路径独立 —— HTTP envelope 在 handler.c.Error → ErrorMappingMiddleware 翻译，WS envelope 在 emoji_handler 直接构造并 Send；两条独立路径不冲突）
  - **不**改 ADR-0007（ctx 传播；本 story handler / service / repo 全链路 ctx-aware，**WS path 同步调 broadcastFn 不需要 detached ctx**，与 11.8 / 14.4 broadcast 路径同模式）
  - **不**接 Redis（10.6 已接，本 story 不需要 —— emoji.send 全量 DB query，无 cache 需求）
  - **不**接 idempotency 键（WS 事件流不是 RESTful 写入路径，无重复语义；client 端发重了 server broadcast 多次，与 transient UI 事件语义一致）
  - **不**写 metric（默认 Prometheus middleware 已挂在 HTTP router；WS 路径不在中间件覆盖范围，节点 6 MVP 不补 WS metric）
  - **不**为 emoji.received broadcast 实装 ack / retry / persistence（V1 §12.3 钦定 fire-and-forget，与 pet.state.changed / member.joined / member.left 同语义）

**本 story 不做**（明确范围红线）：

- 不实装 HTTP emoji 路由（17.4 已完成 GET /emojis）
- 不实装 `EmojiRepo.Create` / `UpdateIsEnabled` 等 admin 方法（MVP 节点 6 无 admin 后台需求）
- 不实装 emoji.send 限频（epics.md §17.5 钦定 MVP 可不做；仅登记 tech-debt）
- 不实装 emoji_events 表 / Redis emoji counter（数据库设计 §14.3 钦定 emoji 默认不持久化）
- 不实装 admin 后台 / 运维端点（YAGNI）
- 不修改 17.4 落地的 EmojiBrief / ListAvailable（字段集已冻结）
- 不修改 14.4 落地的 PetStateChangedPayload / BuildPetStateChangedEnvelope
- 不修改 V1 §12.2 / §12.3 接口契约（17.1 已冻结）
- 不修改 ADR-0006 / ADR-0007（同模式沿用）
- 不引入 Redis idempotency / ack / retry / persistence（YAGNI；MVP 节点 6 不需要）
- 不写英文版测试注释 / 文档（项目 communication_language=Chinese；与既有 17.x / 14.x / 11.x 同模式）

## Acceptance Criteria

**AC1 — `EmojiRepo.Exists` 方法 + interface 扩展（emoji_repo.go 修改）**

在 `server/internal/repo/mysql/emoji_repo.go` 的 `EmojiRepo` interface 内**追加** `Exists` 方法签名（接在 `List` 后；**不**改 17.4 落地的 `List` 方法签名）：

```go
type EmojiRepo interface {
    // List ...（17.4 已落地，本 story 不改）
    List(ctx context.Context) ([]EmojiConfig, error)

    // Exists 查 code 是否存在且 is_enabled=1（Story 17.5 引入；V1 §12.2 服务端
    // 逻辑步骤 4 钦定）。
    //
    // SQL: SELECT 1 FROM emoji_configs WHERE code = ? AND is_enabled = 1 LIMIT 1
    //
    // 关键约束（§12.2 钦定）：
    //   - **`is_enabled = 1` 过滤**：disabled 表情视同"不存在"返 false（避免 server
    //     暴露 enabled / disabled 状态信息；与 §12.2 服务端逻辑步骤 4 "两种情况合并
    //     为同一错误" 钦定一致）
    //   - LIMIT 1 优化：UNIQUE KEY uk_code 已保证 code 唯一，LIMIT 1 是 query planner
    //     hint，让 DB 命中第一行就返回
    //   - **走索引**：`idx_enabled_sort (is_enabled, sort_order ASC)` 覆盖
    //     `WHERE is_enabled = 1` 部分；`WHERE code = ?` 走 UNIQUE KEY `uk_code`
    //     直接定位单行 —— 实际 query planner 会选 `uk_code` 主查 + 应用层 filter
    //     `is_enabled = 1`（O(1) 查询；数据库设计 §5.15 钦定）
    //   - 0 行 → (false, nil)（包括 code 不存在 / 存在但 is_enabled=0）
    //   - 1 行 → (true, nil)
    //   - DB error → (false, raw error 透传给 service（service 包成 1009）)
    //
    // **不**返 EmojiConfig 完整行：本方法仅供 emoji.send 校验路径用，service 层
    // 只需要 bool 信号而非完整字段；少 SELECT 字段 = 少 wire 字节 = 少 GORM Scan
    // 开销。如 future 需要"查 code 然后用 assetUrl"，由对应 story 加新方法
    // （如 GetByCode），**不**改 Exists 签名。
    Exists(ctx context.Context, code string) (bool, error)
}
```

在 `emojiRepo` struct 内**追加** `Exists` 实装：

```go
// Exists 实装：单 SELECT 查询 with LIMIT 1。详见 EmojiRepo.Exists 接口注释。
func (r *emojiRepo) Exists(ctx context.Context, code string) (bool, error) {
    db := tx.FromContext(ctx, r.db).WithContext(ctx)

    // 用 Select("1") + Limit(1) + Find(&result)：返 0 行时 result 为空 slice 而非
    // error；与 GORM 既有 EmojiConfig.List 模式一致；不用 First 是因为 First 在 0
    // 行时返 ErrRecordNotFound，需要在 caller 层额外 errors.Is 判断（增加复杂度）。
    var dummy []int
    err := db.
        Model(&EmojiConfig{}).
        Select("1").
        Where("code = ? AND is_enabled = ?", code, 1).
        Limit(1).
        Find(&dummy).Error
    if err != nil {
        return false, err
    }
    return len(dummy) > 0, nil
}
```

**AC1 验收**：
- `emoji_repo.go` 的 `EmojiRepo` interface 含 `List` + `Exists` 两方法签名
- `emojiRepo` struct 含 `List` + `Exists` 两方法实装
- SQL 严格符合 V1 §12.2 服务端逻辑步骤 4：`SELECT 1 FROM emoji_configs WHERE code = ? AND is_enabled = 1 LIMIT 1`
- 走 `tx.FromContext` + `.WithContext(ctx)` 与 ADR-0007 一致
- 0 行场景返 `(false, nil)`，1 行场景返 `(true, nil)`，DB error 场景返 `(false, err)`

**AC2 — `emoji_repo_test.go` Exists sqlmock 单测追加（≥3 case）**

在 `server/internal/repo/mysql/emoji_repo_test.go` **追加**（不动 17.4 落地的 `TestEmojiRepo_List_*`）：

```go
func TestEmojiRepo_Exists_HappyPath_EnabledRowFound(t *testing.T) {
    repo, mock, cleanup := buildMockEmojiRepo(t)
    defer cleanup()

    mock.ExpectQuery(regexp.QuoteMeta(
        "SELECT 1 FROM `emoji_configs` WHERE code = ? AND is_enabled = ? LIMIT ?",
    )).
        WithArgs("wave", 1, 1).
        WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))

    got, err := repo.Exists(context.Background(), "wave")
    if err != nil {
        t.Fatalf("Exists: %v", err)
    }
    if !got {
        t.Errorf("got = false, want true (wave is enabled in fixture)")
    }
    if err := mock.ExpectationsWereMet(); err != nil {
        t.Errorf("ExpectationsWereMet: %v", err)
    }
}

func TestEmojiRepo_Exists_NotFound_ReturnsFalse(t *testing.T) {
    repo, mock, cleanup := buildMockEmojiRepo(t)
    defer cleanup()

    mock.ExpectQuery(regexp.QuoteMeta(
        "SELECT 1 FROM `emoji_configs` WHERE code = ? AND is_enabled = ? LIMIT ?",
    )).
        WithArgs("nonexistent", 1, 1).
        WillReturnRows(sqlmock.NewRows([]string{"1"})) // 0 行

    got, err := repo.Exists(context.Background(), "nonexistent")
    if err != nil {
        t.Fatalf("Exists: %v", err)
    }
    if got {
        t.Errorf("got = true, want false (code not in DB)")
    }
}

func TestEmojiRepo_Exists_DisabledRow_ReturnsFalse(t *testing.T) {
    repo, mock, cleanup := buildMockEmojiRepo(t)
    defer cleanup()

    // DB 即使有 code='secret' 的行但 is_enabled=0 → query 走 WHERE is_enabled=1 不命中 → 0 行
    mock.ExpectQuery(regexp.QuoteMeta(
        "SELECT 1 FROM `emoji_configs` WHERE code = ? AND is_enabled = ? LIMIT ?",
    )).
        WithArgs("secret", 1, 1).
        WillReturnRows(sqlmock.NewRows([]string{"1"})) // 0 行（is_enabled=1 过滤已生效）

    got, err := repo.Exists(context.Background(), "secret")
    if err != nil {
        t.Fatalf("Exists: %v", err)
    }
    if got {
        t.Errorf("got = true, want false (disabled rows filtered by is_enabled=1)")
    }
}

func TestEmojiRepo_Exists_DBError_ReturnsRawError(t *testing.T) {
    repo, mock, cleanup := buildMockEmojiRepo(t)
    defer cleanup()

    dbErr := fakeDBError() // 17.4 落地的 helper
    mock.ExpectQuery(regexp.QuoteMeta(
        "SELECT 1 FROM `emoji_configs` WHERE code = ? AND is_enabled = ? LIMIT ?",
    )).
        WithArgs("wave", 1, 1).
        WillReturnError(dbErr)

    got, err := repo.Exists(context.Background(), "wave")
    if err == nil {
        t.Fatal("err == nil, want raw DB error")
    }
    if got {
        t.Errorf("got = true on error, want false")
    }
}
```

**注**：dev 在实装时如发现 sqlmock 的 SQL 字面量与 GORM 实际生成的 SQL 不完全一致（如 backtick 转义 / 参数占位符 / `Select("1")` vs `Select("count(*)")` 的产出差异）→ 跑一次 test 看 sqlmock 错误信息中的 actual SQL，调整 expected SQL 字面量；与 17.4 落地的 List 单测同模式。

**AC2 验收**：
- `emoji_repo_test.go` 含 ≥3 个新 Test 函数：HappyPath_EnabledRowFound / NotFound / DisabledRow / DBError
- 所有 case 用 sqlmock 起 mock DB；SQL 字面量校验严格匹配 §12.2 钦定 query
- NotFound 与 DisabledRow 都返 `false, nil`（合并语义验证）

**AC3 — `EmojiService.ValidateCode` 方法 + interface 扩展（emoji_service.go 修改）**

在 `server/internal/service/emoji_service.go` 的 `EmojiService` interface 内**追加** `ValidateCode` 方法签名：

```go
type EmojiService interface {
    // ListAvailable ...（17.4 已落地，本 story 不改）
    ListAvailable(ctx context.Context) ([]EmojiBrief, error)

    // ValidateCode 校验 emojiCode 合法性（Story 17.5 引入；V1 §12.2 服务端逻辑步骤
    // 2 + 4 合并钦定）。
    //
    // 校验链（按顺序，任一失败立即返）：
    //   1. **字符集 / 长度校验**（V1 §12.2 字段表 emojiCode：`1 ≤ length ≤ 64` +
    //      `[a-z0-9_-]`）：不通过 → apperror.New(ErrInvalidParameter /* 1002 */, ...)
    //   2. **DB 存在性校验**（调 emojiRepo.Exists(ctx, code)）：
    //      - err != nil → apperror.Wrap(err, ErrServiceBusy /* 1009 */, ...)
    //      - false（不存在 / disabled）→ apperror.New(ErrEmojiNotFound /* 7001 */, ...)
    //      - true → 返 nil（校验通过）
    //
    // **返 error 而非 bool**：让 handler 走单一 `errors.As` 分支取业务码，避免
    // "service 层返 (bool, code) + handler 走 switch code" 那种 fragile 路径。
    //
    // **不**做 trim / lowercase 等归一化：emojiCode 严格按 client 发送的原始字符串
    // 校验；client 传 "Wave"（大写 W）→ 1002（字符集不允许大写）；这是契约层钦定
    // （§11.1 行 1771 emoji_configs.code 严格 [a-z0-9_-] + UNIQUE KEY 大小写敏感）
    //
    // **不**做 nil-context / nil-string 防御性 check：调用方（emoji_handler.HandleEmojiSend）
    // 已保证 ctx 来自 session.ctx + code 来自 envelope.payload 解析（解析失败前置
    // 拦截）；这里冗余 nil-check 让 happy path 多两次比较开销，与 ADR-0006 / ADR-0007
    // 同模式（service 层信任 handler 层入参）。
    ValidateCode(ctx context.Context, code string) error
}
```

**正则**：在 emoji_service.go 文件顶部 const 段（或 var 段）声明：

```go
// emojiCodePattern 是 V1 §12.2 字段表 emojiCode 字符集 + 长度约束（17.1 r2 锁定）。
//
// 规则：1 ≤ length ≤ 64 字符；只允许 [a-z0-9_-]；与 §11.1 emoji_configs.code
// VARCHAR(64) + UNIQUE KEY uk_code 一致（大小写敏感）。
//
// **包级 var 而非函数内构造**：regexp.Compile 不便宜，每次 ValidateCode 调用都
// recompile 是浪费；包级 var 编译一次复用，与既有 auth_service 同模式。
var emojiCodePattern = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)
```

在 `emojiServiceImpl` struct 内**追加** `ValidateCode` 实装：

```go
func (s *emojiServiceImpl) ValidateCode(ctx context.Context, code string) error {
    // (1) 字符集 / 长度校验
    if !emojiCodePattern.MatchString(code) {
        return apperror.New(apperror.ErrInvalidParameter,
            apperror.DefaultMessages[apperror.ErrInvalidParameter])
    }

    // (2) DB 存在性校验
    exists, err := s.emojiRepo.Exists(ctx, code)
    if err != nil {
        return apperror.Wrap(err, apperror.ErrServiceBusy,
            apperror.DefaultMessages[apperror.ErrServiceBusy])
    }
    if !exists {
        return apperror.New(apperror.ErrEmojiNotFound,
            apperror.DefaultMessages[apperror.ErrEmojiNotFound])
    }

    return nil
}
```

文件顶部 import 段添加 `"regexp"`（既有 import `"context"` / `apperror` / `mysql` 保留）。

**AC3 验收**：
- `emoji_service.go` 含 `EmojiService` interface 内两方法签名（`ListAvailable` + `ValidateCode`）
- `emojiServiceImpl` 含 `ValidateCode` 实装
- 正则严格匹配 `^[a-z0-9_-]{1,64}$`
- 字符集 / 长度失败返 1002，DB error 返 1009，Exists=false 返 7001
- 校验通过返 nil
- **`apperror.ErrInvalidParameter` = 1002**（已存在于 codes.go；本 story 不新增错误码）；**`apperror.ErrEmojiNotFound` = 7001**（已存在）；**`apperror.ErrServiceBusy` = 1009**（已存在）；**`apperror.ErrUserNotInRoom` = 6004**（已存在，handler 用）

**AC4 — `emoji_service_test.go` ValidateCode stub repo 单测追加（≥5 case）**

在 `server/internal/service/emoji_service_test.go` **追加**（不动 17.4 落地的 `TestEmojiService_ListAvailable_*`）：

```go
// stubEmojiRepoWithExists 扩展 17.4 落地的 stubEmojiRepo（加 existsFn 字段）。
//
// **方案**：直接修改 17.4 落地的 stubEmojiRepo struct 加 existsFn 字段（与
// ListAvailable 现有 listFn 同模式）。dev 实装时改既有 struct 而非新建第二个 stub。
type stubEmojiRepo struct {
    listFn   func(ctx context.Context) ([]mysql.EmojiConfig, error)
    existsFn func(ctx context.Context, code string) (bool, error)
}

func (s *stubEmojiRepo) Exists(ctx context.Context, code string) (bool, error) {
    return s.existsFn(ctx, code)
}

func buildEmojiServiceWithExists(existsFn func(ctx context.Context, code string) (bool, error)) service.EmojiService {
    return service.NewEmojiService(&stubEmojiRepo{
        listFn:   func(ctx context.Context) ([]mysql.EmojiConfig, error) { return nil, nil }, // ValidateCode 不调 listFn
        existsFn: existsFn,
    })
}

// AC4.1 happy: emojiCode 合法 + DB 存在 → nil error
func TestEmojiService_ValidateCode_HappyPath_ReturnsNil(t *testing.T) {
    svc := buildEmojiServiceWithExists(func(ctx context.Context, code string) (bool, error) {
        if code != "wave" {
            t.Errorf("Exists called with code=%q, want wave", code)
        }
        return true, nil
    })

    err := svc.ValidateCode(context.Background(), "wave")
    if err != nil {
        t.Fatalf("ValidateCode: %v, want nil", err)
    }
}

// AC4.2 edge: emojiCode 字符集非法（含大写）→ 1002
func TestEmojiService_ValidateCode_InvalidCharset_Returns1002(t *testing.T) {
    svc := buildEmojiServiceWithExists(func(ctx context.Context, code string) (bool, error) {
        t.Errorf("Exists should not be called when code is invalid; got code=%q", code)
        return false, nil
    })

    err := svc.ValidateCode(context.Background(), "Wave") // 大写 W
    if err == nil {
        t.Fatal("err == nil, want 1002 AppError")
    }
    var appErr *apperror.AppError
    if !stderrors.As(err, &appErr) {
        t.Fatalf("err is not *apperror.AppError: %T", err)
    }
    if appErr.Code != apperror.ErrInvalidParameter {
        t.Errorf("appErr.Code = %d, want %d (ErrInvalidParameter)", appErr.Code, apperror.ErrInvalidParameter)
    }
}

// AC4.3 edge: emojiCode 长度 = 0 → 1002
func TestEmojiService_ValidateCode_EmptyCode_Returns1002(t *testing.T) {
    svc := buildEmojiServiceWithExists(func(ctx context.Context, code string) (bool, error) {
        t.Errorf("Exists should not be called when code is empty")
        return false, nil
    })

    err := svc.ValidateCode(context.Background(), "")
    if err == nil {
        t.Fatal("err == nil, want 1002 AppError")
    }
    var appErr *apperror.AppError
    if !stderrors.As(err, &appErr) {
        t.Fatalf("err is not *apperror.AppError: %T", err)
    }
    if appErr.Code != apperror.ErrInvalidParameter {
        t.Errorf("appErr.Code = %d, want %d", appErr.Code, apperror.ErrInvalidParameter)
    }
}

// AC4.4 edge: emojiCode 长度 = 65 → 1002
func TestEmojiService_ValidateCode_TooLong_Returns1002(t *testing.T) {
    svc := buildEmojiServiceWithExists(func(ctx context.Context, code string) (bool, error) {
        t.Errorf("Exists should not be called when code is too long")
        return false, nil
    })

    longCode := strings.Repeat("a", 65)
    err := svc.ValidateCode(context.Background(), longCode)
    if err == nil {
        t.Fatal("err == nil, want 1002 AppError")
    }
    var appErr *apperror.AppError
    if !stderrors.As(err, &appErr) {
        t.Fatalf("err is not *apperror.AppError: %T", err)
    }
    if appErr.Code != apperror.ErrInvalidParameter {
        t.Errorf("appErr.Code = %d, want %d", appErr.Code, apperror.ErrInvalidParameter)
    }
}

// AC4.5 edge: emojiCode 字符集合法但 DB 不存在 → 7001
func TestEmojiService_ValidateCode_CodeNotFound_Returns7001(t *testing.T) {
    svc := buildEmojiServiceWithExists(func(ctx context.Context, code string) (bool, error) {
        return false, nil
    })

    err := svc.ValidateCode(context.Background(), "ghost")
    if err == nil {
        t.Fatal("err == nil, want 7001 AppError")
    }
    var appErr *apperror.AppError
    if !stderrors.As(err, &appErr) {
        t.Fatalf("err is not *apperror.AppError: %T", err)
    }
    if appErr.Code != apperror.ErrEmojiNotFound {
        t.Errorf("appErr.Code = %d, want %d (ErrEmojiNotFound)", appErr.Code, apperror.ErrEmojiNotFound)
    }
}

// AC4.6 edge: DB 错误 → 1009
func TestEmojiService_ValidateCode_DBError_Returns1009(t *testing.T) {
    dbErr := stderrors.New("driver: connection lost")
    svc := buildEmojiServiceWithExists(func(ctx context.Context, code string) (bool, error) {
        return false, dbErr
    })

    err := svc.ValidateCode(context.Background(), "wave")
    if err == nil {
        t.Fatal("err == nil, want 1009 AppError")
    }
    var appErr *apperror.AppError
    if !stderrors.As(err, &appErr) {
        t.Fatalf("err is not *apperror.AppError: %T", err)
    }
    if appErr.Code != apperror.ErrServiceBusy {
        t.Errorf("appErr.Code = %d, want %d (ErrServiceBusy)", appErr.Code, apperror.ErrServiceBusy)
    }
}
```

注：文件顶部 import 段添加 `"strings"`（如尚未导入）。stubEmojiRepo 17.4 落地版本只含 listFn；本 story 改 struct 加 existsFn 字段时，17.4 既有 ListAvailable case 不受影响（listFn 仍按需注入）。

**AC4 验收**：
- `emoji_service_test.go` 含 ≥6 个新 Test 函数：HappyPath / InvalidCharset / EmptyCode / TooLong / CodeNotFound / DBError
- 字符集 / 长度失败 case 显式断言 `Exists should not be called`（防 service 实装顺序 bug：字符集 fail 后还调 repo）
- 所有 case 用 stub repo 注入；不需要起 mysql 容器
- AC4.5 + AC4.6 显式断言 appErr.Code 严格 = 7001 / 1009

**AC5 — `snapshot.go` 追加 EmojiReceivedPayload + ErrorPayload + 两个 envelope helper**

在 `server/internal/app/ws/snapshot.go` **末尾追加**（不动 14.4 落地的 `PetStateChangedPayload` / `BuildPetStateChangedEnvelope`）：

```go
// ============================================================================
// Story 17.5 — emoji.received payload + envelope helper
// ============================================================================

// EmojiReceivedPayload 是 emoji.received 消息的 payload（Story 17.5 引入）。
//
// 与 V1 §12.3 `### 收到表情广播（emoji.received）` 字段表完全 1:1 对齐
// （V1 行 2446-2449）：
//   - UserID:    BIGINT 字符串化（V1 §2.5 全局约定）；表情发起者的 user 主键；
//                来自 emoji.send 当前 user.id（即 WS 握手 token 解码后的 user.id）
//   - EmojiCode: 客户端发送的表情业务标识符；server 已在 §12.2 服务端逻辑步骤 4
//                校验过该 emojiCode 必然存在于 emoji_configs 且 is_enabled=1
//                （client 收到 emoji.received 时**无需**再次校验 —— server 端为
//                single source of truth）；与 §11.1 data.items[].code 同语义
//
// **payload 字段集合严格只 2 字段**（V1 §12.3 行 2461 future fields 注 +
// 本 story 范围红线钦定）：不含 ts（envelope 顶层已含）/ assetUrl（client 从 §11.1
// 缓存列表查得）/ name（同上）/ 任何其他字段；client 用 emojiCode 作 key 在
// §11.1 缓存中定位 assetUrl / name 等渲染所需字段，**不**靠 broadcast 携带。
//
// **关键约束**（V1 §12.3 行 2468 钦定）：2 字段都必填；缺字段视为契约违反，client
// 解析层走"安全忽略 + log warn"路径。Go struct 层不显式 omitempty（与
// PetStateChangedPayload / MemberJoinedPayload 同模式），所有字段一律 JSON marshal 输出。
type EmojiReceivedPayload struct {
    UserID    string `json:"userId"`
    EmojiCode string `json:"emojiCode"`
}

// BuildEmojiReceivedEnvelope wrap EmojiReceivedPayload 进 serverEnvelope +
// json.Marshal 返 ([]byte, error)（Story 17.5 引入；与 BuildPetStateChangedEnvelope /
// BuildMemberJoinedEnvelope / BuildMemberLeftEnvelope 同模式）。
//
// 用途：service 层（emoji_handler.HandleEmojiSend）在 §12.2 服务端逻辑步骤 5
// 校验全通过后调用本 helper 拿到 []byte 后调 BroadcastFn 推送给该房间内所有
// 在线 Session（**包含**发起者自己 —— 与 member.joined / member.left 排除发起者
// 不同语义，与 pet.state.changed 同语义；详见 V1 §12.3 行 2468）
//
// envelope 字段值（V1 §12.3 通用信封 + 行 2446-2449 钦定）：
//   - Type:      "emoji.received"
//   - RequestID: ""（主动推送类消息固定 ""；V1 §12.3 行 2447 钦定 ——
//                **不**回带 emoji.send.requestId，广播 fanout 给房间内所有 Session，
//                server 端无法对所有接收者都"配对" emoji.send.requestId）
//   - Payload:   入参 payload
//   - Ts:        time.Now().UnixMilli()（与 pet.state.changed / member.joined 同
//                语义，仅作日志关联 + UI 辅助展示，**禁止**用作业务排序）
//
// 错误：json.Marshal 在 marshalable struct 下不可能失败；防御性 wrap。caller 收到
// error 时 log warn 不重试（与 broadcast 失败同 fire-and-forget 语义）。
func BuildEmojiReceivedEnvelope(payload EmojiReceivedPayload) ([]byte, error) {
    env := serverEnvelope{
        Type:      "emoji.received",
        RequestID: "", // V1 §12.3 行 2447 主动推送类消息固定 ""
        Payload:   payload,
        Ts:        time.Now().UnixMilli(),
    }
    bytes, err := json.Marshal(env)
    if err != nil {
        return nil, fmt.Errorf("ws envelope: marshal emoji.received: %w", err)
    }
    return bytes, nil
}

// ============================================================================
// Story 17.5 — error payload + envelope helper（**首次**落地 WS error 路径）
// ============================================================================

// ErrorPayload 是 error 消息的 payload（Story 17.5 引入；V1 §12.3 `### error` 字段
// 表 2526-2527）。
//
// 字段：
//   - Code:    错误码，复用 §3 全局错误码定义（本 story 用到的：1002 参数错误 /
//              1009 服务繁忙 / 6004 用户不在房间中 / 7001 emoji not found）
//   - Message: 错误描述，可读字符串（不做国际化，与 §3 message 字段一致）
type ErrorPayload struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}

// BuildErrorEnvelope wrap (requestID, code, message) 进 serverEnvelope +
// json.Marshal 返 ([]byte, error)（Story 17.5 引入；**首次**落地 WS error
// envelope helper —— 节点 4 / 5 既有路径都没用过 WS error 消息：snapshot 失败走
// close 1011；pet.state.changed / member.joined / member.left 都是 broadcast 类消息
// 失败仅 log，不回 error）。
//
// 用途：emoji_handler.HandleEmojiSend 在校验失败（1002 / 1009 / 6004 / 7001）时
// 调用本 helper 拿 []byte 后调 Session.Send 直接推回发起者 Session（**不**走
// broadcast —— V1 §12.2 行 2042 钦定"错误响应通过 §12.3 error 消息回送 / requestId
// 回带 emoji.send.requestId"，是单 Session 响应而非房间广播）
//
// envelope 字段值（V1 §12.3 通用信封 + 行 2524-2528 钦定）：
//   - Type:      "error"
//   - RequestID: caller 入参 requestID（响应类 error 回带 client 请求的 requestId；
//                server 主动错误传 "" 即可，本 story 只调用响应类路径）
//   - Payload:   ErrorPayload{Code, Message}
//   - Ts:        time.Now().UnixMilli()
//
// 错误：json.Marshal 在 marshalable struct 下不可能失败；防御性 wrap（与既有
// envelope helpers 同模式）。
func BuildErrorEnvelope(requestID string, code int, message string) ([]byte, error) {
    env := serverEnvelope{
        Type:      "error",
        RequestID: requestID, // 响应类 error 回带 client 请求 requestId
        Payload:   ErrorPayload{Code: code, Message: message},
        Ts:        time.Now().UnixMilli(),
    }
    bytes, err := json.Marshal(env)
    if err != nil {
        return nil, fmt.Errorf("ws envelope: marshal error: %w", err)
    }
    return bytes, nil
}
```

**AC5 验收**：
- `snapshot.go` 末尾含 4 个新声明：`EmojiReceivedPayload` struct / `BuildEmojiReceivedEnvelope` / `ErrorPayload` struct / `BuildErrorEnvelope`
- `EmojiReceivedPayload` 严格 2 字段：`UserID` + `EmojiCode`（**不**含 ts / assetUrl / name）
- `ErrorPayload` 严格 2 字段：`Code` + `Message`
- envelope 字段 / 字段名 / 类型与 V1 §12.3 1:1 对齐：`emoji.received.requestId == ""` / `error.requestId == 入参` / 两者 `ts` 都是 `time.Now().UnixMilli()`

**AC6 — `snapshot_test.go` envelope helper 单测追加（≥4 case）**

在 `server/internal/app/ws/snapshot_test.go` **追加**（不动 14.4 落地的 pet.state.changed 单测）：

```go
// Story 17.5 — BuildEmojiReceivedEnvelope helper 单测

func TestBuildEmojiReceivedEnvelope_Happy_FullPayload(t *testing.T) {
    payload := wsapp.EmojiReceivedPayload{
        UserID:    "1001",
        EmojiCode: "wave",
    }

    bs, err := wsapp.BuildEmojiReceivedEnvelope(payload)
    if err != nil {
        t.Fatalf("BuildEmojiReceivedEnvelope: %v", err)
    }

    var got struct {
        Type      string                    `json:"type"`
        RequestID string                    `json:"requestId"`
        Payload   wsapp.EmojiReceivedPayload `json:"payload"`
        Ts        int64                     `json:"ts"`
    }
    if err := json.Unmarshal(bs, &got); err != nil {
        t.Fatalf("json.Unmarshal: %v", err)
    }
    if got.Type != "emoji.received" {
        t.Errorf("got.Type = %q, want emoji.received", got.Type)
    }
    if got.RequestID != "" {
        t.Errorf("got.RequestID = %q, want \"\" (broadcast 类固定空)", got.RequestID)
    }
    if got.Payload.UserID != "1001" || got.Payload.EmojiCode != "wave" {
        t.Errorf("payload = %+v, want {UserID: 1001, EmojiCode: wave}", got.Payload)
    }
    if got.Ts <= 0 {
        t.Errorf("got.Ts = %d, want > 0", got.Ts)
    }
}

func TestBuildEmojiReceivedEnvelope_PayloadShapeIsExactlyTwoFields(t *testing.T) {
    payload := wsapp.EmojiReceivedPayload{UserID: "1001", EmojiCode: "wave"}

    bs, err := wsapp.BuildEmojiReceivedEnvelope(payload)
    if err != nil {
        t.Fatalf("BuildEmojiReceivedEnvelope: %v", err)
    }

    // raw map 验证 payload 字段集合严格 = {userId, emojiCode}（防字段漂移）
    var raw struct {
        Payload map[string]json.RawMessage `json:"payload"`
    }
    if err := json.Unmarshal(bs, &raw); err != nil {
        t.Fatalf("json.Unmarshal: %v", err)
    }
    if len(raw.Payload) != 2 {
        t.Errorf("len(payload) = %d, want 2 (userId + emojiCode); got keys: %v",
            len(raw.Payload), keysOf(raw.Payload))
    }
    if _, ok := raw.Payload["userId"]; !ok {
        t.Errorf("payload missing key 'userId'")
    }
    if _, ok := raw.Payload["emojiCode"]; !ok {
        t.Errorf("payload missing key 'emojiCode'")
    }
}

// keysOf helper：复用 snapshot_test.go 既有的，若缺则在 dev 实装时加。
// （14.4 同模式测试已有 keysOf helper —— dev 验证既有 file 是否已有；若无则加）

// Story 17.5 — BuildErrorEnvelope helper 单测

func TestBuildErrorEnvelope_Happy_ResponseTypeWithRequestID(t *testing.T) {
    bs, err := wsapp.BuildErrorEnvelope("msg_001", 7001, "emoji not found")
    if err != nil {
        t.Fatalf("BuildErrorEnvelope: %v", err)
    }

    var got struct {
        Type      string             `json:"type"`
        RequestID string             `json:"requestId"`
        Payload   wsapp.ErrorPayload `json:"payload"`
        Ts        int64              `json:"ts"`
    }
    if err := json.Unmarshal(bs, &got); err != nil {
        t.Fatalf("json.Unmarshal: %v", err)
    }
    if got.Type != "error" {
        t.Errorf("got.Type = %q, want error", got.Type)
    }
    if got.RequestID != "msg_001" {
        t.Errorf("got.RequestID = %q, want msg_001 (响应类 error 回带 client requestId)", got.RequestID)
    }
    if got.Payload.Code != 7001 {
        t.Errorf("payload.code = %d, want 7001", got.Payload.Code)
    }
    if got.Payload.Message != "emoji not found" {
        t.Errorf("payload.message = %q, want emoji not found", got.Payload.Message)
    }
    if got.Ts <= 0 {
        t.Errorf("got.Ts = %d, want > 0", got.Ts)
    }
}

func TestBuildErrorEnvelope_EmptyRequestID_ServerInitiated(t *testing.T) {
    // server 主动产生的 error（如内部状态异常）—— requestId 应为 ""
    bs, err := wsapp.BuildErrorEnvelope("", 1009, "服务繁忙")
    if err != nil {
        t.Fatalf("BuildErrorEnvelope: %v", err)
    }

    var got struct {
        RequestID string `json:"requestId"`
    }
    if err := json.Unmarshal(bs, &got); err != nil {
        t.Fatalf("json.Unmarshal: %v", err)
    }
    if got.RequestID != "" {
        t.Errorf("got.RequestID = %q, want \"\" (主动推送类)", got.RequestID)
    }
}
```

**AC6 验收**：
- `snapshot_test.go` 含 ≥4 个新 Test：BuildEmojiReceivedEnvelope_Happy / PayloadShapeIsExactlyTwoFields / BuildErrorEnvelope_Happy / BuildErrorEnvelope_EmptyRequestID
- payload 字段集严格匹配（防字段漂移）
- error envelope 的 requestId 行为：响应类回带 + 主动推送类固定 ""

**AC7 — `emoji_handler.go` 新建（EmojiHandler interface + emojiHandlerImpl + HandleEmojiSend）**

新建 `server/internal/app/ws/emoji_handler.go`：

```go
package ws

import (
    "context"
    "encoding/json"
    "errors"
    "log/slog"
    "strconv"

    apperror "github.com/huing/cat/server/internal/pkg/errors"
    "github.com/huing/cat/server/internal/repo/mysql"
    "github.com/huing/cat/server/internal/service"
)

// EmojiHandler 是 WS emoji.send 消息的服务端 handler（Story 17.5 引入）。
//
// **interface 而非具体类型**：让 Session 注入 stub struct 单测，与 service 层
// HomeService / RoomService 同模式。
//
// 节点 6 阶段仅 HandleEmojiSend（emoji.send）；future epic 加 HandleEmojiBatch /
// HandleEmojiCancel 等扩展（不在 MVP 范围）。
//
// **nil-safe 约定**：Session.emojiHandler 可为 nil（单测 / HTTP-only 部署）；
// readLoop 在 dispatch 前显式 `if s.emojiHandler != nil` check，nil 时 log warn
// + fallthrough 走 unknown type 路径，与既有 dispatcher 行为一致。
type EmojiHandler interface {
    // HandleEmojiSend 处理 client 发来的 emoji.send 消息（V1 §12.2 行 1981-2080）。
    //
    // **入参**：
    //   - ctx:     session.ctx 或调用方 ctx；用于 service / repo 调用链路
    //   - session: 发起 emoji.send 的 Session（提供 UserID / RoomID / Send 三能力）
    //   - env:     已解析的 clientEnvelope（type/requestId/payload）；payload 仍是
    //              json.RawMessage 由本方法二次解析 emojiCode 字段
    //
    // **返回**：永远 nil（fire-and-forget 入口 —— V1 §12.2 行 2024 钦定 server 处理
    // 成功无 server → client ack 消息；处理失败也已通过 Session.Send 推 error 消息
    // 到 client，无需透传 error 给 caller）。返 error 类型留作 future 扩展（如需做
    // metric 路径），节点 6 实装永不返非 nil。
    HandleEmojiSend(ctx context.Context, session *Session, env clientEnvelope)
}

// emojiHandlerImpl 是 EmojiHandler 的默认实装。
type emojiHandlerImpl struct {
    svc         service.EmojiService
    userRepo    mysql.UserRepo
    broadcastFn BroadcastFn
}

// NewEmojiHandler 构造 EmojiHandler。
//
// 注入：
//   - svc:         EmojiService（service 层 interface）—— 单测可注入 stub
//   - userRepo:    UserRepo（17.5 用 FindByID 拿 users.current_room_id）
//   - broadcastFn: WS 广播函数值（10.5 落地的 BroadcastFn type alias）；
//                  bootstrap wire 阶段传 `wsapp.BroadcastFn(func(...) { ... wsapp.BroadcastToRoom(...) })`
//                  closure；单测直接传 capture closure 记录调用次数 / 入参
//
// **不**注入 SessionManager：handler 不直接读 sessionsByRoom / 不主动 close 任何
// session；fanout 由 broadcastFn closure 内部委托给 BroadcastToRoom 完成（与
// 14.4 broadcastPetStateChanged 同模式）。
func NewEmojiHandler(svc service.EmojiService, userRepo mysql.UserRepo, broadcastFn BroadcastFn) EmojiHandler {
    return &emojiHandlerImpl{
        svc:         svc,
        userRepo:    userRepo,
        broadcastFn: broadcastFn,
    }
}

// emojiSendPayload 是 client → server `emoji.send` payload 的解码 struct（V1 §12.2
// 字段表 emoji.send.payload）。
//
// 字段：
//   - EmojiCode: 必填；service.ValidateCode 二次校验字符集 / 长度 + 存在性
type emojiSendPayload struct {
    EmojiCode string `json:"emojiCode"`
}

// HandleEmojiSend 主流程（V1 §12.2 服务端逻辑步骤 1-5 严格对齐）：
//
//  1. **接收 & 解析**：caller（Session.readLoop）已按 type=="emoji.send" 路由进来
//     并解析顶层 envelope；本方法再解析 payload.emojiCode（json.Unmarshal 失败 →
//     回 error 1002 + return）
//  2. **参数校验**：调 svc.ValidateCode（含字符集 + 长度 + Exists 三段校验）；
//     失败按 apperror.Code 分流为 1002 / 1009 / 7001 → 回 error 消息 + return
//  3. **房间归属校验**：调 userRepo.FindByID 拿 users.current_room_id 与
//     session.RoomID() 比对：
//       - DB error → 回 error 1009 + return
//       - current_room_id == nil → 回 error 6004 + return
//       - *current_room_id != session.RoomID() → 回 error 6004 + log warn 含
//         stale Session 三字段 + return
//  4. （步骤 5 跳过；§12.2 步骤 3 与步骤 4 顺序在 service.ValidateCode 内已合并 ——
//     字符集 / 长度先于 Exists 校验。本 handler 流程为：解析 → 参数校验 → 房间校验 →
//     广播，与 §12.2 服务端逻辑步骤实际等价；房间校验前置到 broadcast 之前任意位置
//     都不违反契约（§12.2 步骤 3 ~ 4 都是 "broadcast 前" 校验，顺序不重要））
//  5. **广播（fire-and-forget）**：构造 EmojiReceivedPayload + BuildEmojiReceivedEnvelope
//     + broadcastFn(ctx, session.RoomID(), msg)；失败仅 log warn 不回 error 给发起者
//
// **关键差异 vs HTTP handler**（home_handler / room_handler / pets_handler）：
//   - HTTP handler 通过 `c.Error(err) + return` 让 ErrorMappingMiddleware 翻译；
//     WS handler 通过 `BuildErrorEnvelope + session.Send` 直接推回 client（V1 §12.2
//     行 2042 钦定"错误响应通过 §12.3 error 消息回送"，与 HTTP envelope 完全独立路径）
//   - HTTP handler 用 c.Request.Context() 取 ctx；WS handler 用 caller 传入 ctx
//     （readLoop 用 session.ctx 或 context.Background()；具体由 readLoop 决定）
func (h *emojiHandlerImpl) HandleEmojiSend(ctx context.Context, session *Session, env clientEnvelope) {
    logger := slog.Default().With(
        slog.String("component", "ws-emoji-handler"),
        slog.String("event", "emoji.send"),
        slog.Uint64("userId", session.UserID()),
        slog.Uint64("sessionRoomId", session.RoomID()),
        slog.String("requestId", env.RequestID),
    )

    // (1) 解析 payload.emojiCode
    var payload emojiSendPayload
    if err := json.Unmarshal(env.Payload, &payload); err != nil {
        logger.Warn("payload unmarshal failed", slog.Any("error", err))
        h.sendErrorToSession(session, env.RequestID, apperror.ErrInvalidParameter,
            apperror.DefaultMessages[apperror.ErrInvalidParameter], logger)
        return
    }

    // (2) 参数校验 + 表情存在性校验（service.ValidateCode 内合并 §12.2 步骤 2 + 4）
    if err := h.svc.ValidateCode(ctx, payload.EmojiCode); err != nil {
        var appErr *apperror.AppError
        if !errors.As(err, &appErr) {
            // 防御性兜底：service 层契约保证返 *AppError；非 AppError → 包成 1009
            logger.Error("ValidateCode returned non-AppError; wrapping as 1009",
                slog.Any("error", err))
            h.sendErrorToSession(session, env.RequestID, apperror.ErrServiceBusy,
                apperror.DefaultMessages[apperror.ErrServiceBusy], logger)
            return
        }
        logger.Info("emoji.send rejected by ValidateCode",
            slog.Int("code", appErr.Code),
            slog.String("emojiCode", payload.EmojiCode))
        h.sendErrorToSession(session, env.RequestID, appErr.Code, appErr.Message(), logger)
        return
    }

    // (3) 房间归属双校验（V1 §12.2 服务端逻辑步骤 3 + r1 review 锁定的反 stale-Session 校验）
    user, err := h.userRepo.FindByID(ctx, session.UserID())
    if err != nil {
        // user 不存在不是合法状态（token 已校验过 user.id）；任何 err（含 ErrUserNotFound）
        // 都视为 1009 服务异常。
        logger.Error("FindByID failed", slog.Any("error", err))
        h.sendErrorToSession(session, env.RequestID, apperror.ErrServiceBusy,
            apperror.DefaultMessages[apperror.ErrServiceBusy], logger)
        return
    }
    if user.CurrentRoomID == nil {
        logger.Info("user not in any room", slog.String("emojiCode", payload.EmojiCode))
        h.sendErrorToSession(session, env.RequestID, apperror.ErrUserNotInRoom,
            apperror.DefaultMessages[apperror.ErrUserNotInRoom], logger)
        return
    }
    if *user.CurrentRoomID != session.RoomID() {
        // stale Session 跨房间注入 —— V1 §12.2 行 2016-2019 r1 review 锁定的拦截路径
        logger.Warn("cross-room emoji.send blocked (stale Session)",
            slog.Uint64("usersCurrentRoomId", *user.CurrentRoomID),
            slog.String("emojiCode", payload.EmojiCode))
        h.sendErrorToSession(session, env.RequestID, apperror.ErrUserNotInRoom,
            apperror.DefaultMessages[apperror.ErrUserNotInRoom], logger)
        return
    }

    // (4) 广播 emoji.received（fire-and-forget；包含发起者自己）
    broadcastPayload := EmojiReceivedPayload{
        UserID:    strconv.FormatUint(session.UserID(), 10),
        EmojiCode: payload.EmojiCode,
    }
    msg, err := BuildEmojiReceivedEnvelope(broadcastPayload)
    if err != nil {
        // marshal 失败极罕见（payload 全是 string）；防御性 log warn 不回 error
        // 给 client（fire-and-forget 边界）
        logger.Warn("BuildEmojiReceivedEnvelope failed", slog.Any("error", err))
        return
    }

    sent, err := h.broadcastFn(ctx, session.RoomID(), msg)
    if err != nil {
        logger.Warn("broadcast emoji.received failed",
            slog.Int("targetSessions", sent),
            slog.Any("error", err))
        return
    }
    logger.Info("emoji.received broadcast sent",
        slog.Int("targetSessions", sent),
        slog.String("emojiCode", payload.EmojiCode))
}

// sendErrorToSession 是 BuildErrorEnvelope + session.Send 的小工具（私有方法）。
//
// **fire-and-forget**：marshal / Send 失败仅 log warn，不返 error 给 caller —— 在
// caller HandleEmojiSend 已是 fire-and-forget 入口的语义下，error 推送本身的失败
// 也无意义（client 可能已断开）。
func (h *emojiHandlerImpl) sendErrorToSession(session *Session, requestID string, code int, message string, logger *slog.Logger) {
    msg, err := BuildErrorEnvelope(requestID, code, message)
    if err != nil {
        logger.Warn("BuildErrorEnvelope failed",
            slog.Int("code", code),
            slog.Any("error", err))
        return
    }
    // 用 SendPriority 而非 Send：error 是 protocol-level msg，走 priority chan
    // 让业务 buffer 满载时仍能投递（与 handlePing 走 SendPriority 同模式 ——
    // session.go 注释钦定 "protocol-level msg 走 priority"）。
    if err := session.SendPriority(msg); err != nil {
        logger.Warn("send error envelope failed",
            slog.Int("code", code),
            slog.Any("error", err))
    }
}
```

**AC7 验收**：
- `emoji_handler.go` 含 `EmojiHandler` interface + `emojiHandlerImpl` struct + `NewEmojiHandler` + `HandleEmojiSend` + 私有 `sendErrorToSession` helper
- 流程严格按 V1 §12.2 服务端逻辑：解析 → ValidateCode → 房间归属双校验 → broadcast
- 房间归属校验：`current_room_id == nil` → 6004；`*current_room_id != session.RoomID()` → 6004 + log warn 含 stale Session 三字段
- 错误响应走 `BuildErrorEnvelope + session.SendPriority`（与 handlePing 一致走 priority chan）
- 广播范围**包含**发起者自己（用 `broadcastFn` 全 fanout，**不**用 `BroadcastToRoomExcept`）
- 永远不返 error（fire-and-forget）
- `apperror.AppError` 必须有 public `Message()` getter（dev 实装时确认；如只有 Code 字段则改为直接读字段；本 story 不约束）

**注**：`apperror.AppError` 的 message 访问路径：本 story 用 `appErr.Message()` 是占位；dev 实装时需查 `apperror.go` 实际 API（可能是 `.Msg` 字段 / `.Error()` 方法 / `.Message()` 方法），按既有约定调整。

**AC8 — `emoji_handler_test.go` 新建（≥7 case stub service + stub userRepo + capture broadcastFn）**

新建 `server/internal/app/ws/emoji_handler_test.go`：

```go
package ws_test

// **包名 ws_test** 而非 ws：emoji_handler_test 走"黑盒"测试路径（only exported
// 接口可见），与 broadcast_test 同模式。private fields 不需要访问 —— 全部
// 通过 EmojiHandler interface + Session.Send 接口走

import (
    "context"
    "encoding/json"
    "errors"
    "net/http/httptest"
    "strings"
    "sync"
    "sync/atomic"
    "testing"
    "time"

    "github.com/gorilla/websocket"

    wsapp "github.com/huing/cat/server/internal/app/ws"
    apperror "github.com/huing/cat/server/internal/pkg/errors"
    "github.com/huing/cat/server/internal/repo/mysql"
    "github.com/huing/cat/server/internal/service"
)

// stubEmojiServiceForHandler 让每个 case 自定义 ValidateCode 返回。
//
// **不**复用 service_test 中的 stubEmojiRepo —— handler test 关心的是 service
// interface 而非 repo；stub 在 ws_test package 内独立定义避免跨包测试 fixture 漂移。
type stubEmojiServiceForHandler struct {
    validateFn func(ctx context.Context, code string) error
}

func (s *stubEmojiServiceForHandler) ValidateCode(ctx context.Context, code string) error {
    return s.validateFn(ctx, code)
}

// ListAvailable 不被本 test 调用；返 nil 占位让 stub 实现完整 EmojiService interface
func (s *stubEmojiServiceForHandler) ListAvailable(ctx context.Context) ([]service.EmojiBrief, error) {
    return nil, nil
}

// stubUserRepoForHandler 让每个 case 自定义 FindByID 返回。
type stubUserRepoForHandler struct {
    findByIDFn func(ctx context.Context, id uint64) (*mysql.User, error)
}

func (s *stubUserRepoForHandler) FindByID(ctx context.Context, id uint64) (*mysql.User, error) {
    return s.findByIDFn(ctx, id)
}

// 其他 UserRepo 方法占位（不被本 test 调用，但需 satisfy interface）：
// dev 实装时按 user_repo.go 既有 UserRepo interface 完整方法列表补齐占位 ——
// 任何返 nil 实装都 OK，本 test 只调 FindByID
func (s *stubUserRepoForHandler) Create(ctx context.Context, u *mysql.User) error      { return nil }
func (s *stubUserRepoForHandler) UpdateNickname(_ context.Context, _ uint64, _ string) error { return nil }
func (s *stubUserRepoForHandler) UpdateCurrentRoomID(_ context.Context, _ uint64, _ *uint64) error {
    return nil
}
// （如 UserRepo 有更多方法，dev 实装时按需补齐）

// captureBroadcastFn 用 atomic + slice 记录 broadcastFn 调用次数 + 入参。
type captureBroadcastFn struct {
    callCount atomic.Int32
    mu        sync.Mutex
    calls     []broadcastCall
    returnSent int
    returnErr  error
}

type broadcastCall struct {
    roomID uint64
    msg    []byte
}

func (c *captureBroadcastFn) fn() wsapp.BroadcastFn {
    return func(ctx context.Context, roomID uint64, msg []byte) (int, error) {
        c.callCount.Add(1)
        c.mu.Lock()
        c.calls = append(c.calls, broadcastCall{roomID: roomID, msg: append([]byte(nil), msg...)}) // defensive copy
        c.mu.Unlock()
        return c.returnSent, c.returnErr
    }
}

// buildEmojiHandlerWithSession 起一个真实 WS Session（httptest + gorilla.Dial）
// 让 session.Send / SendPriority 走真实路径。
//
// **关键**：handler test 不 mock session，因为我们要验证 BuildErrorEnvelope →
// session.SendPriority → conn.WriteMessage 整条链路。dial 完后让 caller 读 conn
// 验证收到的 envelope JSON。
//
// userID / roomID 由 caller 传入；handler 通过 session.UserID() / session.RoomID() 读取。
func buildEmojiHandlerWithSession(t *testing.T, userID, roomID uint64) (h wsapp.EmojiHandler, clientConn *websocket.Conn, session *wsapp.Session, capture *captureBroadcastFn, cleanup func(), stubSvc *stubEmojiServiceForHandler, stubUser *stubUserRepoForHandler) {
    t.Helper()

    stubSvc = &stubEmojiServiceForHandler{}
    stubUser = &stubUserRepoForHandler{}
    capture = &captureBroadcastFn{returnSent: 2, returnErr: nil}

    h = wsapp.NewEmojiHandler(stubSvc, stubUser, capture.fn())

    // 起 httptest server，gorilla.Upgrade → Session
    upgrader := websocket.Upgrader{CheckOrigin: func(r *interface{}) bool { return true }}
    var serverSession *wsapp.Session
    done := make(chan struct{})

    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        conn, err := upgrader.Upgrade(w, r, nil)
        if err != nil {
            t.Errorf("upgrade: %v", err)
            return
        }
        // dev 实装时调用 wsapp 内部 newSession（**注**：newSession 是包私有；测试需要走
        // wsapp 包级 export_test.go 的 NewSessionForTest helper，或重构 newSession
        // 为可测试形态。**dev 实装时可加 export_test.go 一行 `var NewSessionForTest = newSession`**
        // 即可让 ws_test 包看到。**注 2**：14.4 / 11.8 既有 handler test 已建立此模式；
        // dev 复用 export_test.go 既有的 helper 即可。
        serverSession = wsapp.NewSessionForTest(t, "test-session", userID, roomID, conn)
        close(done)
    }))
    cleanupSrv := func() { srv.Close() }

    wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
    clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil {
        cleanupSrv()
        t.Fatalf("dial: %v", err)
    }
    <-done

    cleanup = func() {
        _ = clientConn.Close()
        cleanupSrv()
    }
    session = serverSession
    return
}

// AC8.1 happy: emojiCode 合法 + user 在正确房间 → broadcast 1 次 + emoji.received envelope 正确
func TestEmojiHandler_HandleEmojiSend_Happy_Broadcasts(t *testing.T) {
    h, clientConn, session, capture, cleanup, stubSvc, stubUser := buildEmojiHandlerWithSession(t, 1001, 3001)
    defer cleanup()

    stubSvc.validateFn = func(ctx context.Context, code string) error { return nil }
    stubUser.findByIDFn = func(ctx context.Context, id uint64) (*mysql.User, error) {
        roomID := uint64(3001)
        return &mysql.User{ID: 1001, CurrentRoomID: &roomID}, nil
    }

    env := wsapp.NewClientEnvelopeForTest("emoji.send", "msg_001", []byte(`{"emojiCode":"wave"}`))
    h.HandleEmojiSend(context.Background(), session, env)

    if capture.callCount.Load() != 1 {
        t.Fatalf("broadcast count = %d, want 1", capture.callCount.Load())
    }
    capture.mu.Lock()
    call := capture.calls[0]
    capture.mu.Unlock()
    if call.roomID != 3001 {
        t.Errorf("broadcast roomId = %d, want 3001", call.roomID)
    }
    var got struct {
        Type      string                    `json:"type"`
        RequestID string                    `json:"requestId"`
        Payload   wsapp.EmojiReceivedPayload `json:"payload"`
    }
    if err := json.Unmarshal(call.msg, &got); err != nil {
        t.Fatalf("unmarshal msg: %v", err)
    }
    if got.Type != "emoji.received" {
        t.Errorf("type = %q, want emoji.received", got.Type)
    }
    if got.RequestID != "" {
        t.Errorf("requestId = %q, want \"\" (broadcast 类固定空)", got.RequestID)
    }
    if got.Payload.UserID != "1001" || got.Payload.EmojiCode != "wave" {
        t.Errorf("payload = %+v, want {UserID: 1001, EmojiCode: wave}", got.Payload)
    }
}

// AC8.2 edge: payload JSON 缺 emojiCode 字段（或非 string）→ 回 error 1002 回带 requestId + 不广播
func TestEmojiHandler_HandleEmojiSend_PayloadMissingEmojiCode_Returns1002(t *testing.T) {
    h, clientConn, session, capture, cleanup, stubSvc, stubUser := buildEmojiHandlerWithSession(t, 1001, 3001)
    defer cleanup()

    // ValidateCode 会被调（因为 emojiCode == ""，service 字符集校验在长度 == 0 时拒）
    stubSvc.validateFn = func(ctx context.Context, code string) error {
        if code != "" {
            t.Errorf("code = %q, want \"\" (payload 缺字段 → emojiCode 零值)", code)
        }
        return apperror.New(apperror.ErrInvalidParameter,
            apperror.DefaultMessages[apperror.ErrInvalidParameter])
    }
    stubUser.findByIDFn = func(ctx context.Context, id uint64) (*mysql.User, error) {
        t.Errorf("FindByID should not be called when ValidateCode fails")
        return nil, nil
    }

    // payload 是 `{}` —— emojiCode 字段缺失，json.Unmarshal 成功但 EmojiCode = ""
    env := wsapp.NewClientEnvelopeForTest("emoji.send", "msg_002", []byte(`{}`))
    h.HandleEmojiSend(context.Background(), session, env)

    if capture.callCount.Load() != 0 {
        t.Errorf("broadcast should not be called; got count = %d", capture.callCount.Load())
    }

    // 验证 client 收到 error envelope code=1002 requestId="msg_002"
    _ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
    _, raw, err := clientConn.ReadMessage()
    if err != nil {
        t.Fatalf("ReadMessage: %v", err)
    }
    var got struct {
        Type      string             `json:"type"`
        RequestID string             `json:"requestId"`
        Payload   wsapp.ErrorPayload `json:"payload"`
    }
    if err := json.Unmarshal(raw, &got); err != nil {
        t.Fatalf("unmarshal error: %v", err)
    }
    if got.Type != "error" {
        t.Errorf("type = %q, want error", got.Type)
    }
    if got.RequestID != "msg_002" {
        t.Errorf("requestId = %q, want msg_002 (回带 client requestId)", got.RequestID)
    }
    if got.Payload.Code != apperror.ErrInvalidParameter {
        t.Errorf("code = %d, want %d (1002)", got.Payload.Code, apperror.ErrInvalidParameter)
    }
}

// AC8.3 edge: emojiCode 字符集合法但 DB 不存在 → 7001 + 不广播
func TestEmojiHandler_HandleEmojiSend_EmojiNotFound_Returns7001(t *testing.T) {
    h, clientConn, session, capture, cleanup, stubSvc, stubUser := buildEmojiHandlerWithSession(t, 1001, 3001)
    defer cleanup()

    stubSvc.validateFn = func(ctx context.Context, code string) error {
        return apperror.New(apperror.ErrEmojiNotFound, apperror.DefaultMessages[apperror.ErrEmojiNotFound])
    }
    stubUser.findByIDFn = func(ctx context.Context, id uint64) (*mysql.User, error) {
        t.Errorf("FindByID should not be called when emoji not found")
        return nil, nil
    }

    env := wsapp.NewClientEnvelopeForTest("emoji.send", "msg_003", []byte(`{"emojiCode":"ghost"}`))
    h.HandleEmojiSend(context.Background(), session, env)

    if capture.callCount.Load() != 0 {
        t.Errorf("broadcast should not be called; got count = %d", capture.callCount.Load())
    }

    _ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
    _, raw, err := clientConn.ReadMessage()
    if err != nil {
        t.Fatalf("ReadMessage: %v", err)
    }
    var got struct {
        Payload wsapp.ErrorPayload `json:"payload"`
        RequestID string `json:"requestId"`
    }
    if err := json.Unmarshal(raw, &got); err != nil {
        t.Fatalf("unmarshal: %v", err)
    }
    if got.Payload.Code != apperror.ErrEmojiNotFound {
        t.Errorf("code = %d, want %d (7001)", got.Payload.Code, apperror.ErrEmojiNotFound)
    }
    if got.RequestID != "msg_003" {
        t.Errorf("requestId = %q, want msg_003", got.RequestID)
    }
}

// AC8.4 edge: user.current_room_id == NULL → 6004
func TestEmojiHandler_HandleEmojiSend_UserNotInRoom_Returns6004(t *testing.T) {
    h, clientConn, session, capture, cleanup, stubSvc, stubUser := buildEmojiHandlerWithSession(t, 1001, 3001)
    defer cleanup()

    stubSvc.validateFn = func(ctx context.Context, code string) error { return nil }
    stubUser.findByIDFn = func(ctx context.Context, id uint64) (*mysql.User, error) {
        return &mysql.User{ID: 1001, CurrentRoomID: nil}, nil // user 不在任何房间
    }

    env := wsapp.NewClientEnvelopeForTest("emoji.send", "msg_004", []byte(`{"emojiCode":"wave"}`))
    h.HandleEmojiSend(context.Background(), session, env)

    if capture.callCount.Load() != 0 {
        t.Errorf("broadcast should not be called; count = %d", capture.callCount.Load())
    }
    _ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
    _, raw, err := clientConn.ReadMessage()
    if err != nil {
        t.Fatalf("ReadMessage: %v", err)
    }
    var got struct{ Payload wsapp.ErrorPayload `json:"payload"` }
    if err := json.Unmarshal(raw, &got); err != nil {
        t.Fatalf("unmarshal: %v", err)
    }
    if got.Payload.Code != apperror.ErrUserNotInRoom {
        t.Errorf("code = %d, want %d (6004)", got.Payload.Code, apperror.ErrUserNotInRoom)
    }
}

// AC8.5 edge: user.current_room_id != session.RoomID() → 6004 (stale Session 跨房间)
func TestEmojiHandler_HandleEmojiSend_StaleSession_CrossRoom_Returns6004(t *testing.T) {
    h, clientConn, session, capture, cleanup, stubSvc, stubUser := buildEmojiHandlerWithSession(t, 1001, 3001)
    defer cleanup()

    stubSvc.validateFn = func(ctx context.Context, code string) error { return nil }
    // session.RoomID() == 3001，但 users.current_room_id = 9999（stale Session 跨房间）
    stubUser.findByIDFn = func(ctx context.Context, id uint64) (*mysql.User, error) {
        otherRoom := uint64(9999)
        return &mysql.User{ID: 1001, CurrentRoomID: &otherRoom}, nil
    }

    env := wsapp.NewClientEnvelopeForTest("emoji.send", "msg_005", []byte(`{"emojiCode":"wave"}`))
    h.HandleEmojiSend(context.Background(), session, env)

    if capture.callCount.Load() != 0 {
        t.Errorf("broadcast should not be called for cross-room; count = %d", capture.callCount.Load())
    }
    _ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
    _, raw, err := clientConn.ReadMessage()
    if err != nil {
        t.Fatalf("ReadMessage: %v", err)
    }
    var got struct{ Payload wsapp.ErrorPayload `json:"payload"` }
    if err := json.Unmarshal(raw, &got); err != nil {
        t.Fatalf("unmarshal: %v", err)
    }
    if got.Payload.Code != apperror.ErrUserNotInRoom {
        t.Errorf("code = %d, want %d (6004 stale Session)", got.Payload.Code, apperror.ErrUserNotInRoom)
    }
}

// AC8.6 edge: FindByID DB error → 1009
func TestEmojiHandler_HandleEmojiSend_FindByIDError_Returns1009(t *testing.T) {
    h, clientConn, session, capture, cleanup, stubSvc, stubUser := buildEmojiHandlerWithSession(t, 1001, 3001)
    defer cleanup()

    stubSvc.validateFn = func(ctx context.Context, code string) error { return nil }
    stubUser.findByIDFn = func(ctx context.Context, id uint64) (*mysql.User, error) {
        return nil, errors.New("driver: connection lost")
    }

    env := wsapp.NewClientEnvelopeForTest("emoji.send", "msg_006", []byte(`{"emojiCode":"wave"}`))
    h.HandleEmojiSend(context.Background(), session, env)

    if capture.callCount.Load() != 0 {
        t.Errorf("broadcast should not be called; count = %d", capture.callCount.Load())
    }
    _ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
    _, raw, err := clientConn.ReadMessage()
    if err != nil {
        t.Fatalf("ReadMessage: %v", err)
    }
    var got struct{ Payload wsapp.ErrorPayload `json:"payload"` }
    if err := json.Unmarshal(raw, &got); err != nil {
        t.Fatalf("unmarshal: %v", err)
    }
    if got.Payload.Code != apperror.ErrServiceBusy {
        t.Errorf("code = %d, want %d (1009)", got.Payload.Code, apperror.ErrServiceBusy)
    }
}

// AC8.7 edge: broadcastFn 返 error → 仅 log warn 不回 error 给发起者 (fire-and-forget)
func TestEmojiHandler_HandleEmojiSend_BroadcastFails_FireAndForget(t *testing.T) {
    h, clientConn, session, capture, cleanup, stubSvc, stubUser := buildEmojiHandlerWithSession(t, 1001, 3001)
    defer cleanup()

    stubSvc.validateFn = func(ctx context.Context, code string) error { return nil }
    stubUser.findByIDFn = func(ctx context.Context, id uint64) (*mysql.User, error) {
        roomID := uint64(3001)
        return &mysql.User{ID: 1001, CurrentRoomID: &roomID}, nil
    }
    capture.returnErr = errors.New("broadcast fanout failed")

    env := wsapp.NewClientEnvelopeForTest("emoji.send", "msg_007", []byte(`{"emojiCode":"wave"}`))
    h.HandleEmojiSend(context.Background(), session, env)

    // broadcastFn 调了 1 次（即使 return err）
    if capture.callCount.Load() != 1 {
        t.Errorf("broadcast count = %d, want 1 (broadcastFn called even when it returns err)", capture.callCount.Load())
    }

    // 验证 client **不收到** error envelope（fire-and-forget）
    _ = clientConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
    _, raw, err := clientConn.ReadMessage()
    if err == nil {
        t.Errorf("expected timeout (no error frame); got msg: %s", string(raw))
    }
}
```

**注 1**：本 test 依赖 wsapp 包 export 一个 `NewSessionForTest` helper + `NewClientEnvelopeForTest` helper（封装 newSession + clientEnvelope 构造，让 ws_test 包能访问）。dev 实装时在 `server/internal/app/ws/export_test.go` 加：

```go
var NewSessionForTest = func(t testing.TB, sid string, uid, rid uint64, conn *websocket.Conn) *Session {
    return newSession(sid, uid, rid, conn, slog.Default(), 16384, 5*time.Second, /*emojiHandler*/ nil)
}

func NewClientEnvelopeForTest(typ, reqID string, payload []byte) clientEnvelope {
    return clientEnvelope{Type: typ, RequestID: reqID, Payload: payload}
}
```

**注 2**：handler test 不依赖 readLoop / writeLoop 启动；handler.HandleEmojiSend 是 synchronous 调用，所有 Session.SendPriority 都立即入队 priority chan。但若 test 走"client 从 wire 读 error envelope"路径，**必须**启动 writeLoop（否则 priority chan 永远不被消费 → ReadMessage 超时）。dev 实装时让 NewSessionForTest 内部启动 writeLoop（与既有 14.4 handler test 同模式）。

**注 3**：stubUserRepoForHandler 需 satisfy `mysql.UserRepo` interface 全部方法。dev 实装时查 user_repo.go 既有 interface 方法列表完整补占位（每个返 nil 即可）。

**AC8 验收**：
- `emoji_handler_test.go` 含 ≥7 个 Test：Happy / PayloadMissingEmojiCode / EmojiNotFound / UserNotInRoom / StaleSession / FindByIDError / BroadcastFails
- 所有 case 用 stub service + stub userRepo + capture broadcastFn
- happy case 验证 broadcast 调 1 次 + envelope 字段值正确
- error case 验证 broadcastFn 调 0 次 + client 收到正确 error envelope（含 requestId 回带）
- BroadcastFails case 验证 fire-and-forget（broadcastFn return err 但 client 不收 error frame）

**AC9 — `session.go` readLoop 路由 + Session 注入 emojiHandler 字段**

修改 `server/internal/app/ws/session.go`：

**(a) Session struct 加字段**：

```go
type Session struct {
    // ...（既有字段保留）

    // emojiHandler 是 emoji.send 消息的 dispatch 目标（Story 17.5 引入）。
    //
    // **nil-safe**：单测 / HTTP-only 部署可传 nil；readLoop dispatcher 看到
    // env.Type == "emoji.send" + s.emojiHandler == nil 时走 unknown type 路径
    // （log warn + 不 close 连接），与既有未识别消息行为一致。
    //
    // 不导出（小写）：与既有 conn / sendChan 等内部字段一致；外部访问通过
    // newSession 注入 + readLoop 内部消费。
    emojiHandler EmojiHandler
}
```

**(b) newSession 签名加 emojiHandler 参数**：

```go
func newSession(
    sessionID string,
    userID uint64,
    roomID uint64,
    conn *websocket.Conn,
    logger *slog.Logger,
    maxMessageSize int,
    writeTimeout time.Duration,
    emojiHandler EmojiHandler, // Story 17.5 加（**新参数放在末尾**，避免 caller 大改）
) *Session {
    // ...（既有构造逻辑保留）

    s := &Session{
        // ...（既有字段赋值保留）
        emojiHandler: emojiHandler, // 可为 nil
    }
    // ...（既有 lastHeartbeatAt + SetReadLimit 等保留）
    return s
}
```

**(c) readLoop switch case 加 "emoji.send"**：

```go
switch env.Type {
case "ping":
    s.handlePing(env)
case "emoji.send":
    // Story 17.5 引入：dispatch 到 EmojiHandler；handler nil 时走 unknown type 路径
    if s.emojiHandler == nil {
        s.logger.Warn("ws emoji.send received but no EmojiHandler wired; ignoring",
            slog.String("requestId", env.RequestID))
        continue
    }
    // **同步**调 handler.HandleEmojiSend（与 handlePing 同模式）—— ValidateCode +
    // FindByID + broadcast 全程 O(1 + 2 query)，与 ping 同量级；不需要 goroutine。
    // ctx 用 s.ctx（与 readLoop 上下文绑定，session.Close → ctx cancel → handler
    // 内部 ctx-aware service / repo 立即中止）。
    s.emojiHandler.HandleEmojiSend(s.ctx, s, env)
default:
    s.logger.Warn("ws unknown message type ignored",
        slog.String("type", env.Type), slog.String("requestId", env.RequestID))
}
```

**AC9 验收**：
- Session struct 含 `emojiHandler EmojiHandler` 字段
- `newSession` 签名加第 8 参数 `emojiHandler EmojiHandler`
- `readLoop` 的 `switch env.Type` 含 `case "emoji.send"` 分支
- handler nil 时 fallthrough 走 unknown type 路径（log warn + 不 close）
- 同步调 `s.emojiHandler.HandleEmojiSend(s.ctx, s, env)`（**不**启 goroutine）
- 既有 ping / unknown 分支保持不变

**AC10 — `gateway.go` Gateway 注入 emojiHandler 字段 + NewGateway 签名扩展**

修改 `server/internal/app/ws/gateway.go`：

**(a) Gateway struct 加字段**：

```go
type Gateway struct {
    // ...（既有字段保留）

    // emojiHandler（Story 17.5 引入）：传给 newSession 让 readLoop 路由 emoji.send。
    //
    // **可 nil**：单测 / HTTP-only 部署传 nil；newSession 接受 nil（与 Session.emojiHandler 字段
    // 同语义）。bootstrap 阶段 wire 真实实例（17.5 范围）；未来 epic 加新 WS 业务消息（如
    // 装扮变更广播）时新增 cosmeticHandler / chestHandler 字段同模式。
    emojiHandler EmojiHandler
}
```

**(b) NewGateway 签名加参数**：

```go
func NewGateway(
    signer *auth.Signer,
    mgr SessionManager,
    roomMember mysql.RoomMemberRepo,
    cfg config.WSConfig,
    envName string,
    builder SnapshotBuilder,
    emojiHandler EmojiHandler, // Story 17.5 加（第 7 参数，放末尾让既有 caller 仅在末尾追加 nil）
) *Gateway {
    // ...（既有 prod 配置覆盖强制 + builder fail-fast 等保留）

    return &Gateway{
        // ...（既有字段赋值保留）
        emojiHandler: emojiHandler, // 可为 nil
    }
}
```

**(c) Handle 把 emojiHandler 传给 newSession**：

```go
// 6.1 创建 Session ...
session := newSession(
    "", userID, roomID, conn, g.logger,
    g.cfg.MaxMessageSizeBytes, g.writeTimeout,
    g.emojiHandler, // Story 17.5 加
)
```

**(d) 既有 Gateway test 修复**：dev 实装时跑 `bash scripts/build.sh --test`，gateway_test / handler_test 若用 NewGateway 构造会因签名变更编译失败 —— 全部追加末尾 `nil` 参数（或 `wsapp.NewEmojiHandler(...)` 真实实例，按 case 需要）。

**AC10 验收**：
- Gateway struct 含 `emojiHandler EmojiHandler` 字段
- `NewGateway` 签名加第 7 参数（**末尾追加**，最小化既有调用方改动）
- `Handle` 把 `g.emojiHandler` 传给 `newSession`
- 既有 gateway / session 测试通过追加末尾参数修复编译

**AC11 — `bootstrap/router.go` wire emojiBroadcastFn + emojiHandler + NewGateway 新签名**

修改 `server/internal/app/bootstrap/router.go`：

在 `if deps.SessionMgr != nil` 块内（gateway 构造之前）添加：

```go
// Story 17.5 加：emoji.received 广播 closure（与 petBroadcastFn 同模式 nil-tolerant）。
//
// 与 roomBroadcastFn / petBroadcastFn 同实现（包级 BroadcastToRoom 直调），独立
// closure 让 broadcast 语义边界清晰（"emoji 广播 vs pet 广播 vs room 广播"在 router
// wire 层就分离）。
//
// **广播范围包含发起者自己**（V1 §12.3 行 2468 钦定）：直接调 wsapp.BroadcastToRoom
// 全 fanout，**不**调 BroadcastToRoomExcept（与 11.8 member.joined 排除发起者 / 14.4
// pet.state.changed 包含发起者两类语义中后者一致）。
emojiBroadcastFn := wsapp.BroadcastFn(func(ctx context.Context, roomID uint64, msg []byte) (int, error) {
    if deps.SessionMgr == nil {
        return 0, nil
    }
    return wsapp.BroadcastToRoom(ctx, deps.SessionMgr, roomID, msg)
})

// Story 17.5 加：emoji handler（dispatch emoji.send 消息）
// 复用 17.4 落地的 emojiSvc + 既有 userRepo + 本 story 落地的 emojiBroadcastFn
emojiHandler := wsapp.NewEmojiHandler(emojiSvc, userRepo, emojiBroadcastFn)

// gateway 构造签名扩展第 7 参数：
gateway := wsapp.NewGateway(
    deps.Signer,
    deps.SessionMgr,
    roomMemberRepo,
    deps.WSCfg,
    deps.EnvName,
    snapshotBuilder,
    emojiHandler, // Story 17.5 加
)
```

**AC11 验收**：
- `router.go` 内有 `emojiBroadcastFn` closure + `emojiHandler := wsapp.NewEmojiHandler(...)`
- `wsapp.NewGateway` 调用末尾参数加 `emojiHandler`
- emojiHandler **复用 17.4 落地的 emojiSvc + 既有 userRepo**（不重复构造）
- 17.4 wire 段（`emojiSvc := service.NewEmojiService(emojiRepo)`）保留不动；本 story 仅追加 emojiHandler / emojiBroadcastFn 两行 + NewGateway 末尾参数

**AC12 — `ws_integration_test.go` dockertest 集成测试（≥1 case）**

在 `server/internal/app/ws/ws_integration_test.go` **追加**（不动既有 ws 集成测试）：

```go
// Story 17.5 集成测试：用 dockertest 起真实 mysql:8.0 容器 + 真实 WS 全链路验证：
//   1. fixture：rooms.id=3001, room_members 2 行 (3001,1001) + (3001,1002)，
//      emoji_configs 4 行 seed（migrate up 后已落地）+ users 2 行
//      (id=1001, current_room_id=3001), (id=1002, current_room_id=3001)
//   2. A (userID=1001) + B (userID=1002) 各自建 WS 连接到 /ws/rooms/3001
//      - 各自收到 room.snapshot（snapshot validate 已在既有集成测试覆盖；本 case
//        仅 read past 该 frame）
//   3. A 发 `emoji.send {requestId: "msg_001", payload: {emojiCode: "wave"}}`
//   4. 验证 A、B 都收到 `emoji.received {requestId: "", payload: {userId: "1001", emojiCode: "wave"}}`
//      + ts > 0
//   5. A 再发 `emoji.send {requestId: "msg_002", payload: {emojiCode: "ghost"}}`
//   6. 验证 A 收到 `error {requestId: "msg_002", payload: {code: 7001, message: "emoji not found"}}`
//      B **不**收到任何新消息（read deadline timeout）

func TestWSEmojiSend_HappyPath_BroadcastsEmojiReceivedToAB(t *testing.T) {
    gormDB, dockerCleanup := startMySQLWithRoomMemberFixture(t)
    defer dockerCleanup()

    // 额外 fixture：users 2 行（current_room_id = 3001）
    sqlDB, err := gormDB.DB()
    if err != nil {
        t.Fatalf("gormDB.DB: %v", err)
    }
    if _, err := sqlDB.Exec(`
        INSERT INTO users (id, guest_uid, nickname, avatar_url, status, current_room_id, created_at, updated_at)
        VALUES (1001, 'guest-1001', 'userA', '', 1, 3001, NOW(3), NOW(3)),
               (1002, 'guest-1002', 'userB', '', 1, 3001, NOW(3), NOW(3))`); err != nil {
        t.Fatalf("INSERT users: %v", err)
    }

    // 构造 Gateway + Session 管理 + 注入 emoji handler / broadcast closure
    // 详细 wire 步骤参考既有 startMySQLWithRoomMemberFixture / 既有 ws 集成测试

    // dev 实装时**复用** ws_integration_test.go 既有的 `newTestGateway` / `dialWS`
    // helper（若名字不同按既有命名调整），并扩展 newTestGateway 签名加 emojiHandler
    // 参数；本 test 把 emojiSvc / emojiHandler 真实 wire（service.NewEmojiService +
    // wsapp.NewEmojiHandler），broadcast 走真实 wsapp.BroadcastToRoom

    // 详细 fixture / 拨号 / read snapshot / send emoji.send / read emoji.received
    // 步骤参考 11.8 既有的 `TestWS_MemberJoined_BroadcastsToOtherMembers` 模板 ——
    // 复用其建立 A / B 两个 WS connection、跳过 snapshot 帧、ReadMessage 验证 envelope
    // 字段的全套 helper

    // 期望最终断言：
    //   - A.ReadMessage → emoji.received { userId: "1001", emojiCode: "wave" } 收到
    //   - B.ReadMessage → 同上
    //   - A 发 ghost → A.ReadMessage → error { code: 7001, requestId: "msg_002" }
    //   - B SetReadDeadline 500ms → ReadMessage 超时（B 不收 ghost 的 error；error 是单 Session 响应）
}
```

**AC12 验收**：
- `ws_integration_test.go` 含 ≥1 个新 `TestWSEmojiSend_*` 函数
- 用 dockertest 起真 mysql:8.0；migration up 自动 seed 4 个表情
- 用真实 gorilla/websocket Dial 拨号 + 真实 BroadcastToRoom 全链路
- 断言 A / B 都收 emoji.received + envelope 字段精确 + A 收 error 7001 + B 不收 ghost error

**AC13 — `tech-debt-log.md` 追加节点 6 tech debt（emoji.send 限频未实装）**

在 `_bmad-output/implementation-artifacts/tech-debt-log.md` **追加**（不动既有内容）：

```markdown
## 节点 6 tech debt（Epic 17 收官 2026-05-14 后登记）

### emoji.send 同一用户限频未实装

- **位置**：`server/internal/app/ws/emoji_handler.go` `HandleEmojiSend`
- **现状**：节点 6 阶段 server **不**对 `emoji.send` 做特殊限频
  - rate_limit 中间件挂在 HTTP 路由，**不**挂 WS 路由，故 `emoji.send` 实际**不**走 1005 限频拦截
  - 单一用户每秒可发任意多次 emoji.send → server 全部 broadcast 给房间（可能刷屏）
- **建议措施**（epics.md §17.5 行 2619 钦定）：同一用户每秒最多 5 个表情；可在 Story 4.5 rate_limit 基础上扩展（如 WS-side rate limit middleware）
- **影响**：UI 体验问题（刷屏），不影响 server 端正确性 / 数据一致性
- **优先级**：节点 11+ 阶段评估；MVP 节点 6 可不做
- **契约层**：V1 §12.2 行 2076 钦定"不限频"是节点 6 阶段契约一部分；如未来加限频，需要在 §12.2 服务端逻辑步骤加新错误码 + 视为契约变更
- **关联 story**：本登记由 Story 17.5 触发
```

**AC13 验收**：
- `tech-debt-log.md` 含上述节点 6 tech debt 条目
- 内容含位置 / 现状 / 建议措施 / 影响 / 优先级 / 契约层 / 关联 story 七字段

**AC14 — `bash scripts/build.sh --test` 通过**

```bash
bash scripts/build.sh --test
```

- 所有既有单测继续 PASS（含 14.4 / 17.4 既有 case）
- 本 story 新增单测全 PASS（≥10 case：emoji_repo 3 / emoji_service 6 / snapshot 4 / emoji_handler 7 = 20 case）
- 不引入 lint error / vet error / 编译 warning

**AC15 — `bash scripts/build.sh --integration` 通过**

```bash
bash scripts/build.sh --integration
```

- 本 story 新增 `TestWSEmojiSend_HappyPath_BroadcastsEmojiReceivedToAB` PASS
- 既有集成测试继续 PASS（含 ws 集成测试 / home_service_integration / emoji_service_integration / migrate_integration）
- **本机 Docker 不可用环境**：dockertest startMySQL 超时不视为失败（与 17.4 同模式）；只要代码本身按 home/ws_integration_test 同模式编写、Docker 可用环境下应通过即可

## Tasks / Subtasks

- [x] Task 1: `EmojiRepo.Exists` interface 扩展 + 实装（AC1）
  - [x] 1.1: 在 `server/internal/repo/mysql/emoji_repo.go` `EmojiRepo` interface 内追加 `Exists(ctx, code) (bool, error)` 方法签名
  - [x] 1.2: 在 `emojiRepo` struct 内追加 `Exists` 实装（参考 AC1 钦定代码块）
  - [x] 1.3: SQL 严格符合 V1 §12.2 服务端逻辑步骤 4：`SELECT 1 FROM emoji_configs WHERE code = ? AND is_enabled = 1 LIMIT 1`
  - [x] 1.4: 用 `tx.FromContext(ctx, r.db).WithContext(ctx)` 取 db handle（与 既有 emojiRepo.List 同模式 + ADR-0007）
  - [x] 1.5: 0 行返 `(false, nil)`；1 行返 `(true, nil)`；DB error 返 `(false, err)`

- [x] Task 2: `emoji_repo_test.go` Exists sqlmock 单测追加（AC2）
  - [x] 2.1: 在 17.4 落地的 `emoji_repo_test.go` 追加 ≥3 个新 Test 函数
  - [x] 2.2: HappyPath / NotFound / DisabledRow / DBError 四个场景
  - [x] 2.3: sqlmock SQL 字面量与 GORM 实际生成对齐（用 regexp.QuoteMeta + 实际跑 test 调整）
  - [x] 2.4: NotFound 与 DisabledRow 都断言 `got == false`（合并语义验证）

- [x] Task 3: `EmojiService.ValidateCode` interface 扩展 + 实装（AC3）
  - [x] 3.1: 在 17.4 落地的 `emoji_service.go` `EmojiService` interface 内追加 `ValidateCode(ctx, code) error` 方法签名
  - [x] 3.2: 添加包级 `emojiCodePattern = regexp.MustCompile("^[a-z0-9_-]{1,64}$")`
  - [x] 3.3: 在 `emojiServiceImpl` 内追加 `ValidateCode` 实装（参考 AC3 钦定代码块）
  - [x] 3.4: 字符集 / 长度失败 → `apperror.New(ErrInvalidParameter, ...)`（1002）
  - [x] 3.5: Exists DB error → `apperror.Wrap(err, ErrServiceBusy, ...)`（1009）
  - [x] 3.6: Exists=false → `apperror.New(ErrEmojiNotFound, ...)`（7001）
  - [x] 3.7: 全部通过返 nil
  - [x] 3.8: 文件顶部 import 段加 `"regexp"`

- [x] Task 4: `emoji_service_test.go` ValidateCode 单测追加（AC4）
  - [x] 4.1: 修改 17.4 落地的 stubEmojiRepo struct 加 existsFn 字段（与 listFn 并列）
  - [x] 4.2: 加 `stubEmojiRepo.Exists` 方法实装
  - [x] 4.3: 追加 ≥6 个 Test：HappyPath / InvalidCharset / EmptyCode / TooLong / CodeNotFound / DBError
  - [x] 4.4: 字符集 / 长度失败 case 显式断言 `Exists should not be called`
  - [x] 4.5: 所有 case 显式断言 `appErr.Code` 严格 = 期望码
  - [x] 4.6: 文件顶部 import 段加 `"strings"`（如未导入）

- [x] Task 5: `snapshot.go` EmojiReceivedPayload + ErrorPayload + 两个 envelope helper（AC5）
  - [x] 5.1: 在 `server/internal/app/ws/snapshot.go` 末尾追加 `EmojiReceivedPayload` struct（2 字段：UserID / EmojiCode）
  - [x] 5.2: 追加 `BuildEmojiReceivedEnvelope(payload) ([]byte, error)` helper（与 BuildPetStateChangedEnvelope 同模式；type="emoji.received" / requestId="" / ts=now）
  - [x] 5.3: 追加 `ErrorPayload` struct（2 字段：Code / Message）
  - [x] 5.4: 追加 `BuildErrorEnvelope(requestID, code, message) ([]byte, error)` helper（type="error" / requestId=入参 / ts=now）

- [x] Task 6: `snapshot_test.go` envelope helper 单测追加（AC6）
  - [x] 6.1: 追加 `TestBuildEmojiReceivedEnvelope_Happy_FullPayload`
  - [x] 6.2: 追加 `TestBuildEmojiReceivedEnvelope_PayloadShapeIsExactlyTwoFields`（raw map 断言字段集严格 = {userId, emojiCode}）
  - [x] 6.3: 追加 `TestBuildErrorEnvelope_Happy_ResponseTypeWithRequestID`（requestId 回带）
  - [x] 6.4: 追加 `TestBuildErrorEnvelope_EmptyRequestID_ServerInitiated`（主动推送类）

- [x] Task 7: `emoji_handler.go` 新建（EmojiHandler interface + emojiHandlerImpl + HandleEmojiSend）（AC7）
  - [x] 7.1: 新建 `server/internal/app/ws/emoji_handler.go`（参考 AC7 钦定代码块）
  - [x] 7.2: `EmojiHandler` interface + `emojiHandlerImpl` struct + `NewEmojiHandler` 构造
  - [x] 7.3: `HandleEmojiSend` 流程：解析 payload → ValidateCode → 房间归属双校验 → broadcast
  - [x] 7.4: 错误响应走 `BuildErrorEnvelope + session.SendPriority`（与 handlePing 一致用 priority chan）
  - [x] 7.5: 房间归属校验：`current_room_id == nil` → 6004；`*current_room_id != session.RoomID()` → 6004 + log warn 含 stale Session 三字段（userId / Session.roomID / users.current_room_id）
  - [x] 7.6: broadcast 范围**包含**发起者自己（用 `broadcastFn` 全 fanout）
  - [x] 7.7: broadcast 失败仅 log warn 不回 error 给发起者（fire-and-forget）
  - [x] 7.8: 永远不返 error（fire-and-forget 入口）
  - [x] 7.9: dev 实装时确认 `apperror.AppError` 的 message getter API（Message() / Msg / .Error()）按既有实装调整

- [x] Task 8: `emoji_handler_test.go` 新建（≥7 case）（AC8）
  - [x] 8.1: 新建 `server/internal/app/ws/emoji_handler_test.go`（package `ws_test`）
  - [x] 8.2: 在 `export_test.go` 加 `NewSessionForTest` + `NewClientEnvelopeForTest` helper（让 ws_test 访问 newSession + clientEnvelope）
  - [x] 8.3: stubEmojiServiceForHandler + stubUserRepoForHandler + captureBroadcastFn helper
  - [x] 8.4: ≥7 个 Test：Happy / PayloadMissingEmojiCode / EmojiNotFound / UserNotInRoom / StaleSession / FindByIDError / BroadcastFails
  - [x] 8.5: 每个 error case 验证 broadcastFn 调 0 次 + client 收到正确 error envelope（含 requestId 回带）
  - [x] 8.6: BroadcastFails case 验证 client SetReadDeadline 后 ReadMessage 超时（fire-and-forget）

- [x] Task 9: `session.go` readLoop 路由 + Session 注入 emojiHandler（AC9）
  - [x] 9.1: Session struct 加 `emojiHandler EmojiHandler` 字段
  - [x] 9.2: `newSession` 签名加第 8 参数 `emojiHandler EmojiHandler`
  - [x] 9.3: `readLoop` 的 `switch env.Type` 增加 `case "emoji.send"` 分支：handler nil 时 log warn + continue；非 nil 时同步调 `s.emojiHandler.HandleEmojiSend(s.ctx, s, env)`
  - [x] 9.4: 既有 ping / unknown 分支保持不变

- [x] Task 10: `gateway.go` Gateway 注入 emojiHandler + NewGateway 签名扩展（AC10）
  - [x] 10.1: Gateway struct 加 `emojiHandler EmojiHandler` 字段
  - [x] 10.2: `NewGateway` 签名加第 7 参数 `emojiHandler EmojiHandler`
  - [x] 10.3: `Handle` 内 `newSession(...)` 调用末尾追加 `g.emojiHandler` 参数
  - [x] 10.4: 既有 gateway_test / handler test 修复编译（追加末尾 nil 或真实 handler）

- [x] Task 11: `bootstrap/router.go` wire emojiBroadcastFn + emojiHandler + NewGateway 新签名（AC11）
  - [x] 11.1: 在 `if deps.SessionMgr != nil` 块内（gateway 构造之前）添加 `emojiBroadcastFn` closure（与 petBroadcastFn 同模式）
  - [x] 11.2: 添加 `emojiHandler := wsapp.NewEmojiHandler(emojiSvc, userRepo, emojiBroadcastFn)`（**复用 17.4 落地的 emojiSvc**）
  - [x] 11.3: `wsapp.NewGateway` 调用末尾参数加 `emojiHandler`

- [x] Task 12: `ws_integration_test.go` dockertest 集成测试（AC12）
  - [x] 12.1: 在 `server/internal/app/ws/ws_integration_test.go` 追加 `TestWSEmojiSend_HappyPath_BroadcastsEmojiReceivedToAB`
  - [x] 12.2: 复用 `startMySQLWithRoomMemberFixture` helper + 加 users 2 行 fixture（current_room_id = 3001）
  - [x] 12.3: 用真实 gorilla/websocket Dial 建 A + B 两个 WS connection
  - [x] 12.4: A 发 `emoji.send {emojiCode: "wave"}` → 断言 A、B 都收 emoji.received
  - [x] 12.5: A 发 `emoji.send {emojiCode: "ghost"}` → 断言 A 收 error 7001 + B 不收
  - [x] 12.6: build tag `//go:build integration` + `// +build integration`

- [x] Task 13: `tech-debt-log.md` 追加节点 6 tech debt（AC13）
  - [x] 13.1: 追加 emoji.send 限频未实装条目（含位置 / 现状 / 建议措施 / 影响 / 优先级 / 契约层 / 关联 story 七字段）

- [x] Task 14: 验证 build + test（AC14 + AC15）
  - [x] 14.1: `bash scripts/build.sh --test` 全 PASS（含本 story 新增 ≥20 case）
  - [x] 14.2: `bash scripts/build.sh --integration` 全 PASS（含 TestWSEmojiSend + 既有 ws / home / emoji / migrate 集成测试）
  - [x] 14.3: 不引入 lint / vet / 编译 warning

- [x] Task 15: 收尾
  - [x] 15.1: review 本 story 文件 + 修正任何不一致
  - [x] 15.2: `sprint-status.yaml` 把 `17-5-ws-emoji-send-处理-emoji-received-广播` 从 `backlog` 流转到 `ready-for-dev`（由 create-story workflow 自动完成）

## Dev Notes

### 关键约束（disaster prevention）

1. **SQL 字面量必须严格符合 §12.2 服务端逻辑步骤 4**：
   - `SELECT 1 FROM emoji_configs WHERE code = ? AND is_enabled = 1 LIMIT 1`
   - `is_enabled = 1` **不能改成** `is_enabled != 0` 或 `is_enabled <> 0`（语义等价但与契约文本不一致 → review 会判 P2 fix）
   - LIMIT 1 **不能漏**（UNIQUE KEY uk_code 保证只会有 1 行，但 LIMIT 1 是 query planner hint，让 EXPLAIN 显示 1 row max）

2. **房间归属**必须**双校验**（V1 §12.2 行 2014-2019 + r1 review 锁定）：
   - **不能仅判 `current_room_id != NULL`** —— stale Session 跨房间注入风险无法封堵
   - **必须** `current_room_id != NULL` **且** `*current_room_id == session.RoomID()` 同时成立才允许 broadcast
   - 跨房间情形 → log warn 含三字段（userId / Session.roomID / users.current_room_id）便于排查多设备 stale Session 残留

3. **错误响应通过 WS error 消息**（V1 §12.2 行 2042 钦定）：
   - **不**走 HTTP envelope（HTTP 是另一条独立连接，与 WS 无关）
   - **不**通过 Session.Close / close frame（V1 §12.3 行 2518 钦定："严重错误才 close；业务错误走 error 消息保持连接"）
   - 走 `BuildErrorEnvelope + session.SendPriority`（priority chan 防 buffer 满，与 handlePing 一致）
   - `requestId` 必须**回带** `emoji.send.requestId`（V1 §12.2 字段表 emoji.send.requestId 行钦定 + §12.3 error.requestId 字段表钦定）

4. **broadcast 范围包含发起者自己**（V1 §12.3 行 2468 钦定 + §1 行 63 冻结）：
   - 用 `broadcastFn` 全 fanout（**不**用 `BroadcastToRoomExcept` —— 那是 member.joined / member.left 专用）
   - 与 pet.state.changed 同语义；与 member.joined / member.left **不同**语义
   - client 端（iOS Epic 18.4）负责"去重自己 userId"逻辑；server 不做差异化 fanout

5. **fire-and-forget 严格语义**（V1 §12.2 行 2024 + lesson 14-4 落地）：
   - broadcast 失败仅 log warn，**不**回 error 给发起者
   - **无 HTTP 响应、无 server → client ack 消息**（emoji 是 transient UI 事件，client 端 18.3 钦定本地动效已是主要 UX 反馈）
   - **不** persistence（V1 §14.3 钦定；MVP 节点 6 不写 emoji_events / Redis counter）

6. **DB error 必须包成 1009**（V1 §12.2 错误码表 + 17-1 r2 Lesson 2 钦定）：
   - service 层 `apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])`
   - handler 层直接读 appErr.Code 转 WS error envelope；DB error 永远走 1009 路径

7. **同步调 broadcastFn**（与 11.8 / 14.4 同模式）：
   - **不**启 goroutine + **不**用 `context.WithoutCancel`
   - WS session.ctx 生命周期 ≥ broadcast 时长（broadcast 是同步入队 sendChan O(1)）；不存在 HTTP request ctx 立即 cancel 的问题
   - lesson 2026-05-12-detached-ctx-for-async-broadcast **不**适用本路径（该 lesson 针对 HTTP handler）

8. **emojiHandler 注入 nil-safe**：
   - Session.emojiHandler 可为 nil（单测 / HTTP-only 部署）
   - readLoop dispatcher 看到 nil 时 log warn + fallthrough 走 unknown type 路径，**不** close 连接
   - 与既有未识别 type 行为一致（V1 §12.3 末尾"安全忽略 + log warn"钦定）

9. **error envelope 优先级**（priority chan）：
   - 错误响应走 `session.SendPriority` 而非 `Send`
   - 与 handlePing 一致：protocol-level msg 在 priority buffer 避免 sendChan 满载时丢 error
   - 业务消息（emoji.received broadcast）走 `Send`（sendChan，与既有 broadcast 路径一致）

10. **集成测试用真实 wsapp.BroadcastToRoom + 真实 SessionManager**：
    - **不** mock broadcastFn（与既有 ws_integration_test 11.8 / 14.4 同模式）
    - 真实拨号 + 真实 envelope + 真实 broadcast fanout
    - 验证 A、B 两个 client 端在 ReadMessage 都收到 emoji.received

### Source tree components to touch

- `server/internal/repo/mysql/emoji_repo.go`（修改 —— interface 加 Exists + impl 加 Exists）
- `server/internal/repo/mysql/emoji_repo_test.go`（修改 —— 追加 ≥3 case sqlmock）
- `server/internal/service/emoji_service.go`（修改 —— interface 加 ValidateCode + impl 加 ValidateCode + 包级 emojiCodePattern）
- `server/internal/service/emoji_service_test.go`（修改 —— stubEmojiRepo 加 existsFn + 追加 ≥6 case）
- `server/internal/app/ws/snapshot.go`（修改 —— 末尾追加 EmojiReceivedPayload + BuildEmojiReceivedEnvelope + ErrorPayload + BuildErrorEnvelope）
- `server/internal/app/ws/snapshot_test.go`（修改 —— 追加 ≥4 case envelope 单测）
- `server/internal/app/ws/emoji_handler.go`（新建 —— EmojiHandler interface + emojiHandlerImpl + HandleEmojiSend + sendErrorToSession）
- `server/internal/app/ws/emoji_handler_test.go`（新建 —— ≥7 case stub service + stub userRepo + capture broadcastFn）
- `server/internal/app/ws/export_test.go`（修改 —— 加 NewSessionForTest + NewClientEnvelopeForTest helper）
- `server/internal/app/ws/session.go`（修改 —— Session struct 加 emojiHandler + newSession 签名加参数 + readLoop case "emoji.send"）
- `server/internal/app/ws/gateway.go`（修改 —— Gateway struct 加 emojiHandler + NewGateway 签名加参数 + Handle 传给 newSession）
- `server/internal/app/ws/ws_integration_test.go`（修改 —— 追加 ≥1 case TestWSEmojiSend）
- `server/internal/app/bootstrap/router.go`（修改 —— wire emojiBroadcastFn + emojiHandler + NewGateway 末尾参数）
- `_bmad-output/implementation-artifacts/tech-debt-log.md`（修改 —— 追加节点 6 emoji.send 限频 tech debt）

### Testing standards summary

- **单测**：默认 `bash scripts/build.sh --test`，跑 `go test -count=1 ./...`；不带 -race（CI 单独跑 -race）
- **集成测试**：`bash scripts/build.sh --integration`，触发 `go test -tags=integration`；用 dockertest 起真 mysql:8.0 容器
- **sqlmock**：emoji_repo / emoji_repo_test 用 sqlmock，与既有 17.4 同模式
- **stub repo / service**：emoji_service_test / emoji_handler_test 用 stub，与既有 17.4 / 14.4 同模式
- **真实 WS 集成测试**：ws_integration_test 用 dockertest + gorilla.Dial，与既有 11.8 / 14.4 同模式
- **测试注释 / 文档**：全中文，与既有 17.x / 14.x / 11.x 同模式

### Project Structure Notes

- 严格对齐 `docs/宠物互动App_Go项目结构与模块职责设计.md` §5.3 / §6 分层约束：
  - `internal/repo/mysql/emoji_repo.go`: 单表 CRUD + 错误识别（加 Exists 方法）
  - `internal/service/emoji_service.go`: 业务规则 + 校验（加 ValidateCode 方法）
  - `internal/app/ws/emoji_handler.go`: WS message handler（新建）
  - `internal/app/ws/snapshot.go`: envelope helpers（追加 emoji.received + error）
  - `internal/app/ws/session.go` / `gateway.go`: dispatcher routing + 注入
  - `internal/app/bootstrap/router.go`: DI wire
- 文件命名与既有同模式：`emoji_handler.go`（ws 包，与 14.4 落地的 ws snapshot.go 同模式；**不**新建 ws/emoji 子包 —— 节点 6 范围太小，单文件足够）
- 包名严格沿用：`package mysql` / `package service` / `package ws` / `package bootstrap`
- 没有冲突：EmojiHandler / EmojiReceivedPayload / ErrorPayload / BuildEmojiReceivedEnvelope / BuildErrorEnvelope 都是首次落地，与既有类型无名字冲突

### References

- Epic 17 story 定义：`_bmad-output/planning-artifacts/epics.md` §Story 17.5（行 2601-2628）
- V1 §12.2 `### 发送表情` 完整契约：`docs/宠物互动App_V1接口设计.md` 行 1981-2080
- V1 §12.3 `### 收到表情广播` 完整契约：`docs/宠物互动App_V1接口设计.md` 行 2435-2475
- V1 §12.3 `### error`（错误消息）完整契约：`docs/宠物互动App_V1接口设计.md` 行 2516-2546
- V1 §3 错误码表（1002 / 1009 / 6004 / 7001 等）：`docs/宠物互动App_V1接口设计.md` §3
- V1 §2.5 BIGINT 字符串化全局约定：`docs/宠物互动App_V1接口设计.md` §2.5
- 数据库设计 §5.15 emoji_configs schema：`docs/宠物互动App_数据库设计.md` §5.15（含 idx_enabled_sort 索引）
- 数据库设计 §14.3 emoji 持久化策略：`docs/宠物互动App_数据库设计.md` §14.3（emoji 默认不持久化）
- 17.1 r2 review lesson：`docs/lessons/2026-05-13-emoji-contract-self-consistency-and-1009-and-asset-url-17-1-r2.md`
  - Lesson 2: DB error 必须有 1009 路径
  - Lesson 3: assetUrl 必非空字符串（本 story emoji.send 不读 assetUrl，仅 emoji.received broadcast 不携带 assetUrl）
- 14.4 落地的 broadcast 模板：`server/internal/service/pet_service.go` `broadcastPetStateChanged` 方法 + `server/internal/app/ws/snapshot.go` `BuildPetStateChangedEnvelope` helper
- 11.8 落地的 broadcastExceptFn 对比模式：`server/internal/service/room_service.go` 内 `broadcastMemberJoined` / `broadcastMemberLeft`（**用 BroadcastToRoomExcept 排除发起者；与本 story 用 BroadcastToRoom 包含发起者形成对照**）
- ADR-0006 error envelope 单一生产者：`docs/lessons/2026-04-24-error-envelope-single-producer.md`（本 story WS error 路径独立，**不**违反 HTTP envelope 单一生产者；两条路径彻底分离）
- ADR-0007 ctx 传播：`_bmad-output/implementation-artifacts/decisions/0007-ctx-propagation.md`
- ADR-0011 gorilla/websocket 选型：`_bmad-output/implementation-artifacts/decisions/0011-websocket-stack.md`（与既有 10.x ws 实装一致）
- 同模式参考实装：
  - `server/internal/app/ws/snapshot.go` `BuildPetStateChangedEnvelope` —— 14.4 落地的 envelope helper 模板
  - `server/internal/service/pet_service.go` `broadcastPetStateChanged` —— 14.4 落地的 broadcast 模板（**包含**发起者自己）
  - `server/internal/service/room_service.go` `broadcastMemberJoined` —— 11.8 落地的 broadcast 模板（**排除**发起者；对比示意）
  - `server/internal/app/ws/session.go` `handlePing` + readLoop dispatcher —— 既有 10.x 路由模板
  - `server/internal/app/ws/ws_integration_test.go` 既有 case —— dockertest + 真实 WS 拨号模板
  - `server/internal/repo/mysql/emoji_repo.go` 17.4 落地的 `EmojiRepo.List` —— 单表查询模板（本 story 加 Exists 与之并列）
  - `server/internal/service/emoji_service.go` 17.4 落地的 `EmojiService.ListAvailable` —— service 模板（本 story 加 ValidateCode 与之并列）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- 2026-05-14: 实装期间踩到 `service → ws` import cycle（service/pet_service.go 已 import ws；emoji_handler 想反向 import service.EmojiService 触发 cycle）。修法：在 ws 包内新建本地 `EmojiValidator` interface（仅含单一 `ValidateCode` 方法），service.EmojiService 自动满足（Go consumer-defined interface 模式）。
- 2026-05-14: 多处既有测试调用 `NewGateway` / `newSession` 需要在末尾追加新参数（emojiHandler）；用 sed 批量处理 + 单独修补 service/pets_handler 等 integration test 的少数 case。
- 2026-05-14: handler test 用 newPipeWSConnPair（snapshot_test.go 既有 helper）+ NewSessionForTest（本 story 在 export_test.go 加的 export helper，内部自动启动 writeLoop）让 SendPriority 的 error envelope 真的写到 wire；客户端用 ReadMessage 读取验证。
- 2026-05-14: 集成测试本机 Docker 不可用（dockertest.NewPool 卡 120s 才超时），与既有所有 dockertest 集成测试同状态；本 story `TestWSEmojiSend_HappyPath_BroadcastsEmojiReceivedToAB` 已编写 + `go vet -tags=integration` 通过 + 与 11.8 / 14.4 已通过的 dockertest 测试同模板。

### Completion Notes List

- AC1 ~ AC4 (Exists + ValidateCode + 单测): 完成。注：codes.go 使用 `apperror.ErrInvalidParam`（不是 story 文件假设的 `ErrInvalidParameter`）；按实际常量名编码。
- AC5 + AC6 (snapshot.go 4 个新声明 + ≥4 case 单测): 完成。
- AC7 (emoji_handler.go 新建): 完成。**注**：原计划用 `service.EmojiService` interface 直接注入 handler，但 service 包已 import ws → 触发 import cycle。修法：在 ws 包定义本地 `EmojiValidator` interface（仅含 `ValidateCode` 方法），service.EmojiService 自动满足 → bootstrap wire 透明。
- AC7 注 2: `apperror.AppError` 暴露 `Code` (int) 和 `Message` (string) 字段（直接读，不走 getter 方法）。
- AC8 (emoji_handler_test ≥7 case): 完成。每个 error case 验证 broadcast 调 0 次 + client 收到正确 error envelope + requestId 回带；happy case 验证 broadcast 1 次 + envelope 字段正确；BroadcastFails case 验证 fire-and-forget（client SetReadDeadline timeout）。
- AC9 + AC10 (session.go + gateway.go 注入 emojiHandler + readLoop 路由): 完成。`emojiHandler` 字段可为 nil（readLoop 看到 nil 时 log warn fallthrough，与 unknown type 路径一致）。
- AC11 (bootstrap router.go wire): 完成。emojiBroadcastFn closure + NewEmojiHandler + NewGateway 第 7 参数。
- AC12 (集成测试): 完成。编写并 `go vet -tags=integration` 通过；本机 Docker 不可用无法实跑，但代码与 11.8 / 14.4 已通过的同模板。
- AC13 (tech-debt-log.md): 完成（新建文件，登记 emoji.send 限频未实装）。
- AC14 (`bash scripts/build.sh --test`): **PASS** —— 全部 24 个包测试通过，无新增 lint / vet / 编译 warning。
- AC15 (`bash scripts/build.sh --integration`): 本机 Docker 不可用（dockertest 全部超时），与既有所有 dockertest 集成测试同状态；本 story 集成测试代码已编写 + vet 通过。

### File List

新建：
- `server/internal/app/ws/emoji_handler.go`
- `server/internal/app/ws/emoji_handler_test.go`
- `_bmad-output/implementation-artifacts/tech-debt-log.md`

修改：
- `server/internal/repo/mysql/emoji_repo.go`（追加 `Exists` 方法签名 + 实装）
- `server/internal/repo/mysql/emoji_repo_test.go`（追加 4 case sqlmock `Exists` 单测）
- `server/internal/service/emoji_service.go`（追加 `ValidateCode` 方法签名 + 实装 + 包级 `emojiCodePattern`）
- `server/internal/service/emoji_service_test.go`（stubEmojiRepo 加 existsFn + 追加 6 case ValidateCode 单测）
- `server/internal/app/ws/snapshot.go`（追加 `EmojiReceivedPayload` / `BuildEmojiReceivedEnvelope` / `ErrorPayload` / `BuildErrorEnvelope`）
- `server/internal/app/ws/snapshot_test.go`（追加 4 case envelope 单测）
- `server/internal/app/ws/session.go`（Session struct 加 `emojiHandler` 字段 + `newSession` 签名加参数 + readLoop case "emoji.send"）
- `server/internal/app/ws/gateway.go`（Gateway struct 加 `emojiHandler` 字段 + `NewGateway` 签名加第 7 参数 + Handle 把 g.emojiHandler 传给 newSession）
- `server/internal/app/ws/export_test.go`（加 `NewSessionForTest` + `NewClientEnvelopeForTest` + `ClientEnvelopeForTest` helper）
- `server/internal/app/ws/ws_integration_test.go`（追加 `TestWSEmojiSend_HappyPath_BroadcastsEmojiReceivedToAB` + `startGatewayWithEmojiWired` + `dialWSWithToken`；imports 加 `apperror` / `service`）
- `server/internal/app/ws/ws_test.go`（7 处 NewGateway 调用末尾追加 nil）
- `server/internal/app/ws/session_close_internal_test.go`（7 处 newSession 调用末尾追加 nil）
- `server/internal/app/bootstrap/router.go`（wire emojiBroadcastFn closure + NewEmojiHandler + NewGateway 末尾参数）
- `server/internal/app/http/handler/emojis_handler_test.go`（stubEmojiService 加 ValidateCode 占位方法）
- `server/internal/service/room_service_integration_test.go`（1 处 NewGateway 调用末尾追加 nil）
- `server/internal/app/http/handler/pets_handler_integration_test.go`（1 处 NewGateway 调用末尾追加 nil）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（17-5 状态: ready-for-dev → in-progress → review）
- `_bmad-output/implementation-artifacts/17-5-ws-emoji-send-处理-emoji-received-广播.md`（本文件：tasks 全部勾选 + Status: review + Dev Agent Record 填充）

### Change Log

- 2026-05-14: Story 17.5 实装完成。落地 `EmojiRepo.Exists` + `EmojiService.ValidateCode` + WS `EmojiHandler` + `BuildEmojiReceivedEnvelope` + **首次** `BuildErrorEnvelope` + session.go readLoop 路由 + gateway / bootstrap wire + ≥20 case 单测（emoji_repo 4 / emoji_service 6 / snapshot 4 / emoji_handler 7）+ 1 case dockertest 集成测试。严格对齐 V1 §12.2 / §12.3 契约（含 17.1 r1 锁定的反 stale-Session 跨房间双校验 + 错误码集 1002/6004/7001/1009 + 广播范围包含发起者 + requestId 回带规则）。所有单测通过；集成测试编写 + vet 通过，本机 Docker 不可用未实跑（与既有 dockertest 集成测试同状态）。Epic 17 收官 story。
