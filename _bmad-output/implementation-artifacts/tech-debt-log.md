# Tech Debt Log

本文件登记 MVP 节点开发过程中刻意延后 / 砍掉的功能 + 已知次优实装，避免被遗忘。
每条登记必含：位置 / 现状 / 建议措施 / 影响 / 优先级 / 契约层 / 关联 story 七字段。

---

## 节点 6 tech debt（Epic 17 收官 2026-05-14 后登记）

### emoji.send 同一用户限频未实装

- **位置**：`server/internal/app/ws/emoji_handler.go` `HandleEmojiSend`
- **现状**：节点 6 阶段 server **不**对 `emoji.send` 做特殊限频
  - rate_limit 中间件挂在 HTTP 路由，**不**挂 WS 路由，故 `emoji.send` 实际**不**走 1005 限频拦截
  - 单一用户每秒可发任意多次 emoji.send → server 全部 broadcast 给房间（可能刷屏）
- **建议措施**（epics.md §17.5 钦定 + V1 §12.2 钦定 "MVP 可不做"）：同一用户每秒最多 5 个表情；可在 Story 4.5 rate_limit 基础上扩展（如 WS-side rate limit middleware）
- **影响**：UI 体验问题（刷屏），不影响 server 端正确性 / 数据一致性
- **优先级**：节点 11+ 阶段评估；MVP 节点 6 可不做
- **契约层**：V1 §12.2 钦定 "不限频" 是节点 6 阶段契约一部分；如未来加限频，需要在 §12.2 服务端逻辑步骤加新错误码 + 视为契约变更
- **关联 story**：本登记由 Story 17.5 触发
