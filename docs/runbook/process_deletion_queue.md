# Runbook：`tools/process_deletion_queue`

Story 1.6 MVP 账户注销的 cascade cleanup 脚本。**仅生产 ops 手动执行**，按 backend-architecture-guide §21.5 「tools/* CLI 上线判据」的「生产 ops 手动执行 + runbook」路径落地。

---

## 1. 何时跑

- **默认节奏**：每日一次，运维人员手动触发（本 MVP 阶段不跑 cron，因为单实例体量小 + 需要人工把关）。
- **业务触发**：
  - Apple Privacy Policy 审计前（确认 30 天宽限期账号已清理）。
  - 接到合规 / 法务要求加急处理某具体 userId 时（走 `-older-than-days=N` 参数调窄窗口，详见 §4）。

---

## 2. 跑前 Preflight Checklist（两眼复核）

| # | 项目 | 操作 |
|---|---|---|
| 1 | 确认最近一次 Mongo PITR 备份可用 | 查 Mongo Atlas / 自建备份管理台，备份时间点 ≤ 24h |
| 2 | 确认无其他写入 in-flight | `db.users.stats()` 看 ops/sec 是否异常 |
| 3 | 先跑 dry-run 预览候选 | 见 §3 |
| 4 | 复核 dry-run 数量 | 与上一次扫描比对；突然激增需查 Epic 1.6 回归 |
| 5 | 两眼复核 | 另一名 ops / backend lead 核对 dry-run 输出 |
| 6 | 明确告知 oncall | Slack / PagerDuty 值班群公告「我要跑 process_deletion_queue」 |

---

## 3. 推荐执行流程

```bash
# 第 1 步：dry-run 预览候选
go run ./server/tools/process_deletion_queue \
  -config /etc/cat/production.toml \
  -dry-run \
  -older-than-days=30 \
  -limit=100 \
  | jq .
# 输入 `CONFIRM` 后脚本开始扫描
# 输出示例: {"deletedUsers": 4, "deletedApnsTokens": 0, "durationMs": 123, "dryRun": true, "olderThanDays": 30}
# 注意 dry-run 模式下 `deletedUsers` 字段含义是「候选数」，`deletedApnsTokens` 始终为 0

# 第 2 步：真实执行
go run ./server/tools/process_deletion_queue \
  -config /etc/cat/production.toml \
  -older-than-days=30 \
  -limit=100 \
  | tee /var/log/cat/deletion-$(date +%Y%m%d-%H%M%S).log \
  | jq .
# 输入 `CONFIRM` 触发实际删除
# 预期输出: {"deletedUsers": 4, "deletedApnsTokens": 6, "durationMs": 456, "dryRun": false, "olderThanDays": 30}
```

### 3.1 CONFIRM 守门

脚本启动后打印到 stderr：
```
!!! This will PERMANENTLY DELETE user data from Mongo + Redis. Type CONFIRM to proceed:
```
**必须**在 stdin 输入 `CONFIRM`（大小写敏感，前后无空格）才继续。任何其他输入（包括 `confirm` / ` CONFIRM` / `WRONG`）立即 abort 退出码 1，不做任何写入。

### 3.2 cascade 清理范围

当前 MVP 版本清理：

1. `apns_tokens.DeleteMany({user_id: <userID>})` —— 先删，避免 Epic 8 cold-start recall 推送给已删除用户（NFR-COMP-3）
2. `users.DeleteOne({_id: <userID>})` —— 后删

**尚未接入的** cascade（未来 epic 实现时需同步扩展 `run.go` 的 TODO 块）：
- `cat_states` —— Epic 2.x
- `friendships` / `blocks` —— Epic 3.x
- `blindbox_drops` / `blindbox_inventory` —— Epic 6.x
- `skin_ownership` —— Epic 7.x

**refresh blacklist**（Redis `refresh_blacklist:<jti>`）—— **不**做显式清理。blacklist 按 jti 索引，无法从 userId 枚举；条目自然 TTL 过期（与 refresh token 同生命周期，30 天）。

---

## 4. 参数参考

| Flag | 默认 | 说明 |
|---|---|---|
| `-config` | `config/default.toml` | TOML 配置路径，与 `cat` 主进程共享 |
| `-dry-run` | `false` | 仅打印候选数，不写 Mongo |
| `-older-than-days` | `30` | 宽限期天数；锁 NFR-COMP-5 「30 天内人工处理」|
| `-limit` | `100` | 单次执行上限，避免意外 full-sweep |

> ⚠️ `-older-than-days=0` 会删除**所有** `deletion_requested=true` 的用户，包括刚请求注销的。**严禁**在日常运维中使用，仅限法务紧急加急场景 + 两眼复核后。

---

## 5. 失败与 Rollback

### 5.1 脚本中途失败

- 脚本按「每条用户：先删 apns_tokens，再删 users」逐行处理；**已完成的用户 + 已删除的 apns_tokens 无法回滚**（`DeleteOne/DeleteMany` 是 Mongo 原子操作，无事务包裹）。
- 脚本失败时：
  1. 记录 stderr 到 `/var/log/cat/deletion-*.log`
  2. 查看 stdout 最后一次成功 summary（如果有）确认进度
  3. 从 Mongo PITR 备份恢复被误删的用户 —— 参见 NFR-REL-7
  4. 打开 Incident 通道，升级到 backend lead

### 5.2 误执行 `-older-than-days=0`

- **立即** 停止脚本（Ctrl+C）
- Mongo PITR 恢复到脚本启动时间点
- 事后 retro，考虑给 `-older-than-days` 加下限守护（< 7 禁用）

---

## 6. MVP 阶段不做的（明示）

- **无 Prometheus metric 导出**：MVP 单实例无 metric 系统消费，加了也没人看
- **无自动 alert**：运维手动执行，成功 / 失败都是人工汇报
- **无 cron 定时触发**：Epic 1.6 选择「两眼复核 + 手动执行」。如果未来体量 > 10k users/day ，可加 cron + PagerDuty alert；届时需 retro 评审

（§21.5 明示：MVP 阶段接受「runbook-only」形态，监控+alert 等 Epic 1.x retro 视流量决定）

---

## 7. 联系人

| 角色 | 备注 |
|---|---|
| DBA | 负责 Mongo PITR 恢复（NFR-REL-7） |
| Backend lead | 执行前两眼复核；执行失败时升级 |
| Oncall | 接收执行前公告，跑期间 standby |

（具体姓名/联系方式见内部 wiki `ops/contacts.md`，不写入 repo）

---

## 8. 修改历史

| 日期 | 变更 | 人 |
|---|---|---|
| 2026-04-19 | Story 1.6 首次落地 | Epic 1 |
