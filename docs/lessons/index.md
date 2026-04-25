<!-- auto-maintained by /fix-review; human edits OK, next run preserves non-list text -->

# Review Lessons Index

本目录收集每次 `/fix-review` 产出的经验沉淀，供后续 `/bmad-distillator` 蒸馏成紧凑 cheatsheet，指导未来 Claude 不再踩同类坑。

| 日期 | 主题 | 条数 | 分类 | commit |
|---|---|---|---|---|
| 2026-04-24 | [配置路径 CWD 耦合 与 启动 banner 时序错位](2026-04-24-config-path-and-bind-banner.md) | 2 | config, error-handling | `8913fa7` |
| 2026-04-25 | [slog 初始化时机 vs 启动失败路径](2026-04-25-slog-init-before-startup-errors.md) | 1 | error-handling, observability | `0a0d108` |
| 2026-04-24 | [Sample 模板的 nil DTO 兜底 & slog 测试 fixture 的 WithGroup 语义](2026-04-24-sample-service-nil-dto-and-slog-test-group.md) | 2 | architecture, error-handling, testing | `4519274` |
| 2026-04-24 | [`go vet` 必须跟随 build/test 的 build tag，保持 validation 可见文件集一致](2026-04-24-go-vet-build-tags-consistency.md) | 1 | testing | `707b070` |
| 2026-04-24 | [中间件之间的 canonical decision 必须走显式 c.Keys 而非各自从 c.Errors 推断](2026-04-24-middleware-canonical-decision-key.md) | 2 | error-handling, observability | `f85064c` |
| 2026-04-24 | [error envelope 必须经 ErrorMappingMiddleware 单一产出，中间件绕过写 envelope 是反模式](2026-04-24-error-envelope-single-producer.md) | 1 | error-handling, observability, architecture | `26b5692` |
| 2026-04-24 | [README dev-mode 二进制名错配 & settings.local 硬编码 PID 漏到 tracked file](2026-04-24-readme-dev-mode-binary-mismatch-and-stale-pids.md) | 2 | docs, config | `<pending>` |
