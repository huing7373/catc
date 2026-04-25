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
| 2026-04-24 | [README dev-mode 二进制名错配 & settings.local 硬编码 PID 漏到 tracked file](2026-04-24-readme-dev-mode-binary-mismatch-and-stale-pids.md) | 2 | docs, config | `2a2ebfb` |
| 2026-04-25 | [devtools 双闸门是 OR 语义，SOP 不能写"任一漏放兜得住"](2026-04-25-dev-mode-or-gate-sop-accuracy.md) | 1 | docs | `bcfcf71` |
| 2026-04-25 | [SwiftUI ZStack overlay 不能盖在底部 CTA 行上](2026-04-25-swiftui-zstack-overlay-bottom-cta.md) | 1 | architecture | `<pending>` |
| 2026-04-25 | [ObservableObject / @Published 必须显式 `import Combine`](2026-04-25-swift-explicit-import-combine.md) | 1 | style, dependency | `<pending>` |
| 2026-04-26 | [Sendable 类内共享 JSONDecoder/JSONEncoder 的语义分歧](2026-04-26-jsondecoder-encoder-thread-safety.md) | 1 | concurrency | `<pending>` |
| 2026-04-26 | [URLProtocol 测试 stub 的 process-global static 状态隔离](2026-04-26-urlprotocol-stub-global-state.md) | 1 | testing | `<pending>` |
| 2026-04-26 | [baseURL 与 endpoint.path 字符串拼接的双斜杠陷阱](2026-04-26-url-trailing-slash-concat.md) | 1 | architecture | `<pending>` |
| 2026-04-26 | [URLProtocol 测试拦截：session-local 注入 vs process-global register](2026-04-26-urlprotocol-session-local-vs-global.md) | 1 | testing | `<pending>` |
| 2026-04-26 | [@StateObject 老 init + bind() 注入路径漏掉副作用初始化](2026-04-26-stateobject-init-vs-bind-injection.md) | 1 | bug, architecture | `<pending>` |
| 2026-04-26 | [默认 baseURL 应从 Info.plist 读，不应硬编码 localhost](2026-04-26-baseurl-from-info-plist.md) | 1 | architecture, config | `<pending>` |
| 2026-04-26 | [iOS ATS 默认拒 cleartext HTTP，Info.plist 必须显式加例外](2026-04-26-ios-ats-cleartext-http.md) | 1 | security | `<pending>` |
