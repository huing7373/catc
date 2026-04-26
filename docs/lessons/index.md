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
| 2026-04-25 | [SwiftUI ZStack overlay 不能盖在底部 CTA 行上](2026-04-25-swiftui-zstack-overlay-bottom-cta.md) | 1 | architecture | `4de0140` |
| 2026-04-25 | [ObservableObject / @Published 必须显式 `import Combine`](2026-04-25-swift-explicit-import-combine.md) | 1 | style, dependency | `6b1f45e` |
| 2026-04-26 | [Sendable 类内共享 JSONDecoder/JSONEncoder 的语义分歧](2026-04-26-jsondecoder-encoder-thread-safety.md) | 1 | concurrency | `2b0449a` |
| 2026-04-26 | [URLProtocol 测试 stub 的 process-global static 状态隔离](2026-04-26-urlprotocol-stub-global-state.md) | 1 | testing | `2b0449a` |
| 2026-04-26 | [baseURL 与 endpoint.path 字符串拼接的双斜杠陷阱](2026-04-26-url-trailing-slash-concat.md) | 1 | architecture | `5d97a74` |
| 2026-04-26 | [URLProtocol 测试拦截：session-local 注入 vs process-global register](2026-04-26-urlprotocol-session-local-vs-global.md) | 1 | testing | `5d97a74` |
| 2026-04-26 | [@StateObject 老 init + bind() 注入路径漏掉副作用初始化](2026-04-26-stateobject-init-vs-bind-injection.md) | 1 | bug, architecture | `d0d7c7a` |
| 2026-04-26 | [默认 baseURL 应从 Info.plist 读，不应硬编码 localhost](2026-04-26-baseurl-from-info-plist.md) | 1 | architecture, config | `d0d7c7a` |
| 2026-04-26 | [SwiftUI `.task` 在 view 重新出现时会重启，"一次性"语义需 ViewModel 自己 short-circuit](2026-04-26-swiftui-task-modifier-reentrancy.md) | 1 | concurrency, ui | `0b1dae2` |
| 2026-04-26 | [iOS ATS 默认拒 cleartext HTTP，Info.plist 必须显式加例外](2026-04-26-ios-ats-cleartext-http.md) | 1 | security | `8054f23` |
| 2026-04-26 | [`URL(string:)` 对 malformed 输入过于宽容，配置入口须显式校验 scheme + host](2026-04-26-url-string-malformed-tolerance.md) | 1 | reliability | `367403f` |
| 2026-04-26 | [baseURL host-only 契约：设计承诺与 validator 必须对齐](2026-04-26-baseurl-host-only-contract.md) | 1 | reliability | `f6d910b` |
| 2026-04-26 | [队列型 UI 状态机存储 presentation 必须连带 callback 一起入队](2026-04-26-error-presenter-queue-onretry-loss.md) | 1 | error-handling | `634c564` |
| 2026-04-26 | [SwiftUI fullScreenCover 是隔离 window scene，全局 overlay UI 必须在 sheet 子树重复 attach](2026-04-26-fullscreencover-isolated-environment.md) | 1 | architecture, ui | `634c564` |
| 2026-04-26 | [SwiftUI modal overlay 必须做下层 hit-testing + accessibility 双屏蔽](2026-04-26-modal-overlay-content-shield.md) | 1 | accessibility, ui | `3b40ba8` |
| 2026-04-26 | [Shell 包装脚本的 flag 组合矩阵必须显式枚举 + 默认行为按主路径选](2026-04-26-build-script-flag-matrix.md) | 1 | config | `e0c3617` |
| 2026-04-26 | [`ObservableObject.objectWillChange` 不 emit initial value，helper API contract 必须显式声明](2026-04-26-objectwillchange-no-initial-emit.md) | 1 | testing | `e0c3617` |
| 2026-04-26 | [`Published.Publisher` 是 mutation 之前同步 emit NEW value，比 `objectWillChange + dispatch async` 更可靠](2026-04-26-published-publisher-vs-objectwillchange.md) | 1 | testing | `18bab17` |
| 2026-04-26 | [`MockBase` 内部存储字段一律 `private`，只通过 snapshot helper 读 — 不要 expose mutable storage](2026-04-26-mockbase-snapshot-only-reads.md) | 1 | testing, concurrency | `18bab17` |
| 2026-04-26 | [Combine `.prefix(N)` 替代手工 fulfill counter，避免 over-fulfillment + 让 publisher 自然 backpressure](2026-04-26-combine-prefix-vs-manual-fulfill.md) | 1 | testing | `6a2f62d` |
| 2026-04-26 | [`xcodebuild -showdestinations` 必须按段过滤，grep 全文会选中 Ineligible 段](2026-04-26-xcodebuild-showdestinations-section-aware.md) | 1 | config | `61eecbc` |
| 2026-04-26 | [Shell 判 simulator 可用性必须排除 `Any iOS Simulator Device` placeholder，concrete entry 才算真有 runtime](2026-04-26-simulator-placeholder-vs-concrete.md) | 1 | testing, config | `e328838` |
| 2026-04-26 | [SwiftUI @StateObject init 阶段构造的 standalone container 与 RootView container 是别名陷阱](2026-04-26-stateobject-debug-instance-aliasing.md) | 1 | architecture | `3e5ad68` |
| 2026-04-26 | [SwiftUI 父级 a11y `.contain` 必须保留 `.accessibilityLabel` 才不丢父 summary](2026-04-26-swiftui-a11y-contain-with-label.md) | 1 | a11y | `6bccf5a` |
| 2026-04-26 | [Swift `Error.localizedDescription` 对非 LocalizedError 返回系统串而非空，"isEmpty 兜底" 永远不触发](2026-04-26-error-localizeddescription-system-fallback.md) | 1 | error-handling | `c94209b` |
| 2026-04-26 | [用户触发的 retry 类异步 action 必须自带并发短路 guard，不能复用 idempotency flag 替代](2026-04-26-user-triggered-action-reentrancy.md) | 1 | concurrency | `c94209b` |
| 2026-04-26 | [SwiftUI `.animation(_:value:)` 不会让 switch 分支切换淡入淡出，必须 ZStack + 每分支显式 `.transition`](2026-04-26-swiftui-switch-transition-explicit.md) | 1 | ui | `c94209b` |
| 2026-04-26 | [onboarding 文档的可移植性 & 跨目录 markdown 相对链接](2026-04-26-readme-portable-paths-and-relative-links.md) | 2 | docs | `c95b1a6` |
| 2026-04-26 | [Onboarding README 的 runbook 必须与工具语义 / 实际 UI 文案对齐](2026-04-26-readme-runbook-must-match-actual-behavior.md) | 2 | docs | `4d50993` |
| 2026-04-26 | [真机联调 runbook 必须含 code signing 步骤 + config-change-then-restart 序列](2026-04-26-readme-physical-device-runbook-completeness.md) | 2 | docs | `d254a28` |
| 2026-04-26 | [README 命令必须 cover 所有合法网段 + 工具输出格式不能假设固定字符数](2026-04-26-readme-portable-network-and-tool-output.md) | 2 | docs | `0da147e` |
| 2026-04-26 | [Onboarding 文档必须考虑 build-wrapper 副作用 + lesson index 必须随 lesson 同步](2026-04-26-readme-onboarding-vs-tooling-side-effects.md) | 2 | docs | `6e24b57` |
