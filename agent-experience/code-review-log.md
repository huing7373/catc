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
