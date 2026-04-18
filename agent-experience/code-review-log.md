# 代码审查日志

记录每次代码审查的发现，供后续蒸馏提取编码规范。
实际修复内容见同一 git commit 的 diff（`git log --grep="fix(review)"` + `git show <hash>`）。

---

## [0-14-ws-message-type-registry-and-version-query] Round 1 — 2026-04-18

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | 一致性守门在 debug 模式下完全跳过"`WSMessages` 条目是否已注册"的校验 —— 条件分支仅在 `mode != "debug"` 时执行 missing 检测 | server/cmd/cat/initialize.go:216-227 | 若 debug 构建因 feature flag 或误删跳过了某个 DebugOnly 条目的 `dispatcher.Register`，启动仍通过；而 `/v1/platform/ws-registry` 仍从 `dto.WSMessages` 广告该类型，客户端送来即被 Dispatcher 返回 `UNKNOWN_MESSAGE_TYPE` —— G2 想拦的正是这条漂移，且是该 endpoint 唯一能对外暴露给客户端的漂移场景 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过
