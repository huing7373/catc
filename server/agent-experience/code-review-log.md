# 代码审查日志

记录每次代码审查的发现，供后续蒸馏提取编码规范。
实际修复内容见同一 git commit 的 diff（`git log --grep="fix(review)"` + `git show <hash>`）。

---

## [0-3-infra-connectivity-and-clients] Round 1 — 2026-04-17

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | Verify 未校验 issuer claim | pkg/jwtx/manager.go:85 | 接受其他服务/环境签发的 token，打穿多环境 token 边界 |
| 2 | patch | Verify 只检查 *SigningMethodRSA 而非钉死 RS256 | pkg/jwtx/manager.go:86 | 放行 RS384/RS512，不符合 NFR-SEC-2 RS256 唯一要求 |
| 3 | patch | Issue 整体赋值 RegisteredClaims，静默丢失调用方传入的 Subject/Audience/NotBefore | pkg/jwtx/manager.go:70 | 后续基于标准 claim 的授权或时序约束失效，调用侧无法感知 |
| 4 | patch | active_kid/old_kid 允许为空且 Verify 接受空 kid header | pkg/jwtx/manager.go:40,90 | 轮换配置错误时不 fail-fast，可能签发/接受无 kid token |
| 5 | patch | Redis MustConnect Ping 无超时保护 | pkg/redisx/client.go:19 | Redis 地址不可达时 initialize() 无限挂起 |
| 6 | bad_spec | WithTx 回调签名在 AC/task/dev notes 三处不一致（mongo.SessionContext vs context.Context） | pkg/mongox/tx.go:11 | mongo-driver v2 无 SessionContext；已统一 AC 为 func(context.Context) error |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过
