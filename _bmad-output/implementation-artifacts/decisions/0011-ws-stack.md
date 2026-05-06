# ADR-0011 — WebSocket 库选型（server 端）

- 状态：Accepted（2026-05-06，Story 10.3 落地）
- 上下文：Epic 10 节点 4 引入 WebSocket，需为 Go server 选定 WS 库；Story 10.1
  已冻结 V1 §12.1 / §12.2 / §12.3 协议骨架契约，本 ADR 是契约**实装**层选型。
- 关联：Story 10.1（V1 §12 协议骨架冻结）/ Story 10.3（本 ADR 创建）/
  Story 10.4-10.7（沿用本 ADR 决策，不再重新评审）

## §1 上下文

节点 4（Epic 10 ~ 13）引入 WebSocket 房间会话能力，server 端需选一个 Go WS 库
承载：

- HTTP → WS Upgrade（在 Gin engine 内挂 `GET /ws/rooms/:roomId` 路由）
- 单条 frame 读 / 写（V1 §12.2 钦定 text frame + 16 KB 上限）
- close frame emit（V1 §12.1 close code 表 4001 / 4002 / 4003 / 4004 / 4005 /
  4006 / 1011）
- 节点 4 单实例假设 → 不需要 cluster-aware
- 测试栈：与现有 testify / sqlmock / dockertest 兼容

库需满足：

1. 字段层 ABI 稳定（API 不频繁破坏性改动）
2. 与 Gin v1.12.0 直接集成（hijack 协议升级）
3. close frame 写控制 + 自定义 code / reason 支持完善
4. ReadLimit / WriteDeadline 等基础流量控制能力

## §2 候选方案

### 2.1 gorilla/websocket（v1.5.3）

- ✅ Go 生态事实标准，许多 Go HTTP 中间件 / framework 默认 reference
- ✅ 字段层 ABI 稳定（v1.x API 多年未破坏性改动）
- ✅ 与 Gin 通过 `c.Writer` Hijack 直接兼容（Upgrader.Upgrade 调 `http.Hijacker`）
- ✅ FormatCloseMessage / WriteControl / SetReadLimit / SetWriteDeadline 等基础
  能力齐全
- ✅ 测试栈 httptest.NewServer + Dialer.Dial 是 gorilla 文档钦定的测试模式
- ⚠️ 包级 maintenance 模式（仍接受 PR 但活跃度不高）

注：story 钦定 v1.5.4，但 go module proxy 仅可获取到 v1.5.3（`go list -m
-versions` 截止 2026-05-06 最高 v1.5.3）；选 v1.5.3，与 story §AC1 "或当前
最新稳定 v1.5.x" 兜底语义一致。

### 2.2 nhooyr.io/websocket（v1.8.x）

- ✅ context-aware API 更现代（每个 Read / Write 都接 ctx）
- ⚠️ 与 Gin 集成需要更多 boilerplate（不直接支持 hijack 模式）
- ⚠️ 与 miniredis-style in-process server 的测试 fixture 兼容性未验证
- ⚠️ 更小的生态（次于 gorilla）

### 2.3 coder/websocket（v1.8.x）

- ⚠️ 较新（fork from nhooyr），生态成熟度低于 gorilla
- ⚠️ 当前没有 Gin 集成参考

## §3 决策

**选 gorilla/websocket v1.5.3**。

## §4 理由

1. ADR-0001 §3 钦定 "先求一致再求性能" —— gorilla 是事实标准，团队上手成本低
2. V1 §12.x 协议骨架已通过 Story 10.1 冻结，gorilla 字段层语义匹配（text frame +
   custom close code + control message 全支持）
3. 与 Gin（已集成）兼容性最好（`c.Writer.Hijack` 模式是 gorilla 推荐路径）
4. 测试栈 miniredis / sqlmock / dockertest 均与 gorilla 兼容；
   `httptest.NewServer + websocket.DefaultDialer.Dial` 是 gorilla 文档示例
5. 节点 4 单实例 + 单 user 单 session 假设 → 不需要 nhooyr 的 context-aware 优势
   带来的高并发优化

## §5 反例（不选其他库）

- 不选 nhooyr.io/websocket：API 更现代但 Gin 集成 boilerplate 增加 + 团队学习
  曲线 + 测试 fixture 兼容性未验证
- 不选 coder/websocket：fork 生态成熟度未达 gorilla 同级

**强约束**：禁止两个 WS 库共存（增加测试矩阵 / 维护负担）；任何 future ADR 切换
WS 库需 supersede 本 ADR + 一次性迁移所有 ws.* 代码。

## §6 Future / 性能演进

节点 9+ 多实例部署 + 性能压测时若发现 gorilla 性能瓶颈（如单进程 ≥ 10k 并发
WS），再评估切 nhooyr / coder。当前节点 4 单实例 + 单房间 4 user 的负载远低于
gorilla 性能上限，本 ADR 不预先优化。

## §7 实装影响

- `server/go.mod` 加 `require github.com/gorilla/websocket v1.5.3`
- `server/internal/app/ws/` 包内部消费 `*websocket.Conn` / `websocket.Upgrader` /
  `websocket.FormatCloseMessage` / `websocket.WriteControl` 等具体类型
- Session 接口**不**导出 `*websocket.Conn`（外部通过 `Session.Send` / `Session.Close`
  访问），让 future ADR 切换 WS 库时 import scope 仅限于 `internal/app/ws/` 包
  内部，不波及 service / handler / 业务 epic 的代码
