# 代码审查日志

记录每次代码审查的发现，供后续蒸馏提取编码规范。
实际修复内容见同一 git commit 的 diff（`git log --grep="fix(review)"` + `git show <hash>`）。

---

## [0-14-ws-message-type-registry-and-version-query] Round 1 — 2026-04-18

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | 一致性守门在 debug 模式下完全跳过"`WSMessages` 条目是否已注册"的校验 —— 条件分支仅在 `mode != "debug"` 时执行 missing 检测 | server/cmd/cat/initialize.go:216-227 | 若 debug 构建因 feature flag 或误删跳过了某个 DebugOnly 条目的 `dispatcher.Register`，启动仍通过；而 `/v1/platform/ws-registry` 仍从 `dto.WSMessages` 广告该类型，客户端送来即被 Dispatcher 返回 `UNKNOWN_MESSAGE_TYPE` —— G2 想拦的正是这条漂移，且是该 endpoint 唯一能对外暴露给客户端的漂移场景 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过

---

## [0-15-spike-op1-watchos-ws-primary-stability-test-matrix] Round 1 — 2026-04-18

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | long_lived worker 在 read/write/echo 首次错误即 `return` 永久退出，外层无重连循环 | server/tools/ws_loadgen/main.go:390-394（旧） | 10-min ADR-003 压测中任何一条 WS 瞬时错误都会让该 worker 永久离场，N 静默塌陷；报告的 `broadcastLatencyP95/P99` 与 connectSuccessRate 因此比配置档位（1k/3k/5k/10k）的实际稳态容量要乐观得多 —— 恰恰在"高负载 / 弱网"这个要度量的场景下失真最严重 |
| 2 | patch | 工具把 `cold_connect` / `raise_wrist` 场景标注为 "Models the NFR-REL-4 measures" / AC5 度量，但 `runOneCycle` 只做 dial + `debug.echo`，从不触发 `session.resume` 链路 | server/tools/ws_loadgen/main.go:428-435（旧）+ docs/spikes/op1-ws-stability.md §5 | AC5 `connectColdMs*` / `reconnectRaiseWristMs*` 的定义是"首个 `session.resume.result` / 业务消息到达"，包含 provider 链路开销；工具度量只是"dial + debug.echo RTT"。若架构师用工具输出填 §5 cell 表，将系统性低估真实重连延迟，NFR-REL-4 目标 `reconnectWithin5sRate ≥ 98%` 可能假通过，Epic 4 的解锁决策基于错误数据 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过（含 `go vet ./tools/ws_loadgen`，`long_lived` smoke 验证 worker 在无服务器下正确循环重连 + `reconnectAttempts` 计数）

---

## [0-15-spike-op1-watchos-ws-primary-stability-test-matrix] Round 2 — 2026-04-18

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | `workerLongLived` 外层循环第 2 次及之后一律 `reconnects.Add(1)`，与 `Summary.ReconnectAttempts` godoc 声称的"after the first successful connect"语义不一致 —— 启动期 dial 反复失败（服务不可达 / 鉴权拒绝）的每次重试都被误记为 reconnect | server/tools/ws_loadgen/main.go:415（round 1 引入） | `reconnectAttempts` 在"服务器没起 / 起来但鉴权全部拒"等场景会被高估到几乎等于总 dial 次数，后续 §7 稳态判据以此为输入会把本来就没建过连的压测档位标成"重连风暴"，掩盖了"配置不对 / 服务不可用"这个真正的根因 |
| 2 | patch | `reconnectRatio = reconnectAttempts / connectSuccess` 分母把"所有成功 dial"（含每次重连成功）都算进去，但文档语义是"% 会话被迫重连" | server/tools/ws_loadgen/main.go:116（Summary godoc round 1 版）+ docs/spikes/op1-ws-stability.md:91 / :232 | 系统性低估 churn 约 `R/(N+R)`。1000 worker × 50 次重连的例子下，公式给出 50/1050 = 4.76% < 5%，判"稳态"；但按文档语义实际 50/1000 = 5% 已触达边界。§7 ADR-003 的稳态/非稳态标签因此在 50-60 次重连的窗口区间不可靠，可能让本应 fail 的 10k 压测假通过 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过（含 `long_lived` smoke 再验证："无服务器"下 16 次启动期 dial fail 现在 `reconnectAttempts=0`，round 1 版本会错报为 16）

---

## [10-1-integration-mvp-room-and-action] Round 1 — 2026-04-18

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | `HandleJoin` 把 `connMap[newConn]=userID` 新增之后，旧 conn 的 `connMap` 条目与 room.members 内的旧 `Member.ConnID` 都没清理；`OnDisconnect` 又仅用 `userLoc` 判断是否还在 room，不核对 `member.ConnID == connID` | server/internal/ws/room_mvp.go:173-178（idempotent 分支未清 stale connMap）+ 154-156（switch-room 分支同问题）+ OnDisconnect 293-298（缺 member.ConnID 二次校验） | 同一 user 在旧 socket 尚未完成 disconnect 前用新 socket 重连并 `room.join`：短暂（毫秒级）之后旧 socket 的 `OnDisconnect` 会查到 `userLoc[user]` 仍指向当前房间，按原逻辑走 `leaveRoomLocked`，把 user 从**新 session 正常在用的**房间里踢掉；后续 `action.update` 报 `VALIDATION_ERROR: user not in any room`、peers 停收广播，直到 client 察觉并手动再 join。真机 watch ↔ iOS 联调最常触发该 race（Wi-Fi 切换、wake from sleep、process relaunch 都能在毫秒级重建 socket） |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过（`TestRoomManager_Rejoin_SameRoom_StaleDisconnectDoesNotEvict` + `TestRoomManager_Rejoin_SwitchRoom_StaleDisconnectDoesNotEvict` 两条新 regression 测试锁死修复；`go vet -tags=integration ./...` 亦绿）
