<!-- auto-maintained by /fix-review; human edits OK, next run preserves non-list text -->

# Review Lessons Index

本目录收集每次 `/fix-review` 产出的经验沉淀，供后续 `/bmad-distillator` 蒸馏成紧凑 cheatsheet，指导未来 Claude 不再踩同类坑。

**ADR 蒸馏状态**（2026-04-29）：以下 lesson 行末尾标 `**[ADR-0008]**` 表示已被 ADR-0008 v2 蒸馏；其中 silent relogin 相关 4 条因 silent relogin 整体退役而**完全 superseded**（lesson 教训仅保留作为未来 single-flight 协调器引用）；transient/terminal 系列 4 条**部分 superseded**（具体 case 分类已收录 ADR §4.2，但反模式 8.2/8.3/8.4 等元教训保留）。受影响 lesson 在 ADR-0008 §8 反模式登记 + §6 退役决策中有完整去向记录。详见 `_bmad-output/implementation-artifacts/decisions/0008-error-protocol.md`。

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
| 2026-04-26 | [V1 接口设计 /home chest.status 必须严格按节点阶段限定状态空间](2026-04-26-v1接口设计-home-chest-status-必须严格按节点阶段限定状态空间.md) | 1 | docs | `47c7998` |
| 2026-04-26 | [V1 接口契约文档全局规则与具体字段 / planned story 必须一致](2026-04-26-v1接口契约全局规则与字段一致性.md) | 2 | docs | `d4b2aa6` |
| 2026-04-26 | [V1 契约 VARCHAR 长度按字符数 + 改文档某处必须 grep 全节防自相矛盾](2026-04-26-v1接口契约-varchar语义与同节自相矛盾.md) | 2 | docs | `bcf665d` |
| 2026-04-26 | [改契约文档时必须做完整 grep + 包括所有副本（产物 / story 文件）](2026-04-26-契约文档全文sweep与跨文件副本同步.md) | 3 | docs | `ad6789e` |
| 2026-04-26 | [契约 schema 字段可空性必须显式声明 + JSON 示例标签必须与示例内容一致](2026-04-26-契约schema字段可空性必须显式声明.md) | 2 | docs | `ce5ef55` |
| 2026-04-26 | [infrastructure 接入必须配齐 env override + 第三方库默认行为陷阱](2026-04-26-config-env-override-and-gorm-auto-ping.md) | 2 | config, architecture | `40f5d01` |
| 2026-04-26 | [启动路径阻塞 IO 必须有 deadline & dev 文档命令必须与默认配置自洽](2026-04-26-startup-blocking-io-needs-deadline.md) | 2 | error-handling, docs | `c96ef29` |
| 2026-04-26 | [CLI 子命令 flag 解析必须用 NewFlagSet + 跨平台 file URI 必须避免 backslash 拼接](2026-04-26-cli-subcommand-flag-and-windows-file-uri.md) | 3 | error-handling, architecture, testing | `6368594` |
| 2026-04-26 | [CLI 子命令必须 lazy load config + 长 IO 操作必须 ctx-aware GracefulStop](2026-04-26-cli-lazy-config-and-gomigrate-gracefulstop.md) | 2 | config, architecture | `460b346` |
| 2026-04-26 | [CLI 默认相对路径必须 auto-detect 多 cwd & gomigrate GracefulStop 必须等 fn 真停](2026-04-26-cli-relative-path-and-graceful-stop-wait.md) | 2 | config, error-handling | `c1b7e4b` |
| 2026-04-26 | [ctx-aware 包装必须 short-circuit pre-canceled ctx & 文件路径转 URI 必须 escape 元字符](2026-04-26-ctx-precancel-shortcircuit-and-uri-escape.md) | 2 | error-handling | `fc91816` |
| 2026-04-26 | [locate auto-detect 逻辑必须 cwd + exe-relative 双 fallback（与 config.LocateDefault 一致）](2026-04-26-locate-cwd-and-exe-relative-fallback.md) | 1 | config | `88e07bd` |
| 2026-04-26 | [JWT util 校验必填 claim + 所有 sign 路径必须 enforce 配置约束](2026-04-26-jwt-required-claim-and-sign-policy-enforcement.md) | 2 | security | `174caab` |
| 2026-04-26 | [checked-in dev config 必须能直接跑 + 部署文档必须与新增配置项同步](2026-04-26-checked-in-config-must-boot-default.md) | 1 | config | `d0870c1` |
| 2026-04-26 | [secret 字段必须空字符串 + fail-fast，不能 checked-in dev fallback（即使加了警告注释）](2026-04-26-checked-in-secret-must-fail-fast.md) | 1 | security, config | `c9396aa` |
| 2026-04-26 | [JWT 篡改测试必须改非末尾字节（base64url padding bits 共享导致末尾字符 flip 可能 decode 出相同字节）](2026-04-26-jwt-tamper-test-must-mutate-non-terminal-byte.md) | 1 | testing | `3344dee` |
| 2026-04-26 | [IP 限频 key 必须用 RemoteIP + SetTrustedProxies 锁定 Gin / atomic + sync.Map 必须用 CAS 才真正 bounded](2026-04-26-rate-limit-xff-spoof-and-buckets-cas.md) | 2 | security, perf | `933c71b` |
| 2026-04-26 | [YAML 配置默认值不能掩盖显式无效值（用 *int64 区分 nil 与 explicit 0）](2026-04-26-yaml-default-must-not-mask-explicit-invalid.md) | 1 | config | `b67cf45` |
| 2026-04-26 | [多表事务必须穷举所有唯一约束的 race 路径（不同表的唯一约束需要独立 sentinel + 全部走 reuseLogin）](2026-04-26-multi-table-tx-must-cover-all-unique-constraint-races.md) | 1 | error-handling, architecture | `be8c418` |
| 2026-04-27 | [SwiftUI UITest a11y marker 必须 gate 在副作用 returned 之后才进入 view tree](2026-04-27-swiftui-uitest-marker-after-side-effect.md) | 1 | testing | `8695ec0` |
| 2026-04-27 | [Keychain service namespace 必须可注入，测试不得复用生产 namespace](2026-04-27-keychain-service-namespace-injectable.md) | 1 | testing, architecture | `686f53e` |
| 2026-04-27 | [DI 容器 production 默认值切换后，所有触发外部存储副作用的容器测试都必须改走注入路径](2026-04-27-appcontainertests-must-inject-isolated-keychain-namespace.md) | 1 | testing | `4c08fc6` |
| 2026-04-27 | [SessionStore 写入但视图未订阅会渲染陈旧身份](2026-04-27-sessionstore-home-nickname-source-of-truth.md) | 1 | architecture | `f08878c` |
| 2026-04-27 | [Reset 类操作必须同步清空 in-memory session 状态](2026-04-27-reset-identity-must-clear-in-memory-session.md) | 1 | architecture | `9ed4f97` |
| 2026-04-27 | [actor coalesce 协调器的 inFlight 清理必须绑定 spawned task 生命周期，而不是 caller defer](2026-04-27-actor-coalesce-cleanup-must-bind-resource-not-caller.md) | 1 | architecture | `31c4fe7` | **[ADR-0008 superseded]**
| 2026-04-27 | [静默重登必须区分"本地无凭证"vs"server 拒绝 token"，前者**不**走 relogin](2026-04-27-silent-relogin-must-distinguish-local-vs-server-unauthorized.md) | 1 | architecture, error-handling | `99e8afd` | **[ADR-0008 partial-superseded]** （case 拆分保留 §6.5；relogin 路径退役）
| 2026-04-27 | [actor coalesce 仅靠 inFlight 字段不足以拦 stale-401，需要 generation snapshot](2026-04-27-silent-relogin-stale-401-needs-generation-dedup.md) | 1 | architecture | `1579c9c` | **[ADR-0008 superseded]**
| 2026-04-27 | [actor coalesce 失败路径必须连带清空 cached result，否则 generation 短路会返回已被 invalidate 的旧 token](2026-04-27-actor-coalesce-failure-must-clear-cached-token.md) | 1 | error-handling | `83f8292` | **[ADR-0008 superseded]**
| 2026-04-27 | [Retry decorator 上线后，原 `.unauthorized` 文案的语义会反转 — 必须同步审计所有 user-visible mapping](2026-04-27-retry-decorator-changes-unauthorized-presentation-semantics.md) | 1 | error-handling | `8892a9a` | **[ADR-0008 partial-superseded]** （decorator 退役；元规则保留 §8.16）
| 2026-04-27 | [bootstrap /home 失败必须经 AppErrorMapper + 可空 domain 字段必须区分 loading 与 server-null 两种 placeholder](2026-04-27-bootstrap-error-and-optional-pet-must-route-via-mapper.md) | 2 | architecture, error-handling | `ac03578` |
| 2026-04-27 | [Launch state machine 必须携带完整 ErrorPresentation 语义 + bootstrap 重试不能重发已成功的 guest-login](2026-04-27-launch-state-machine-must-carry-presentation.md) | 2 | error-handling, architecture, perf | `b39e7a5` |
| 2026-04-27 | [冷启动 HTTP 预算钦定 ≤2 时不能保留任何 nice-to-have 探针 + bootstrap retry 必须 fail-safe 重跑 auth](2026-04-27-cold-start-http-budget-and-bootstrap-retry-fail-safe.md) | 2 | architecture, error-handling | `e32184f` |
| 2026-04-27 | [bootstrap 全部错误路径必经 mapper / ping 复活回 .ready 分支 / alert-only dismiss 不能隐式 retry](2026-04-27-bootstrap-all-error-paths-route-via-mapper.md) | 3 | error-handling, architecture | `5dcfa4b` |
| 2026-04-27 | [Business 错误必须区分 transient/terminal & alert OK 按钮必须有真实动作 & 4 轮 fix-review 单点 patch 反模式](2026-04-27-business-error-transient-vs-terminal.md) | 3 | error-handling, ui, architecture | `e1598a3` |
| 2026-04-27 | [SwiftUI 多 .task 之间无顺序保证：bind 与 start 必须在同一闭包](2026-04-27-swiftui-multi-task-no-ordering.md) | 1 | architecture | `ef018bd` |
| 2026-04-27 | [wire DTO → domain 转换：未知 enum 必须 fail-fast，禁止 silent fallback](2026-04-27-home-data-fail-fast-on-unknown-enum.md) | 1 | error-handling | `ef018bd` |
| 2026-04-27 | [Bootstrap alert dismiss 必须 user-driven recovery (禁 exit(0)) & alert 文案 4 轮迭代史防 regress](2026-04-27-bootstrap-alert-dismiss-must-be-user-driven-recovery.md) | 2 | error-handling, ui, architecture | `460ab92` |
| 2026-04-27 | [Bootstrap terminal error 必须用静态全屏 fallback page (禁 dismiss-able overlay) & 5 轮 fix-review 元根因复盘 (跳出 framing 的元方法论)](2026-04-27-bootstrap-terminal-error-static-fallback-page.md) | 2 | error-handling, ui, architecture, process | `ef1d866` |
| 2026-04-28 | [`.decoding` / `.unauthorized` 必须按 transient 二分原则归 `.retry` & 9 轮 fix-review 累积出 transient/terminal 通用判则](2026-04-28-decoding-and-unauthorized-must-be-transient-retry.md) | 2 | error-handling, process | `2d89afe` |
| 2026-04-28 | [`AppErrorMapper` 非 APIError fallback 必须按 transient 二分原则归 `.retry`，不是 `.alert`](2026-04-28-non-api-error-fallback-must-be-transient-retry.md) | 1 | error-handling | `9f4ad26` |
| 2026-04-28 | [error case 不该 conflate transient (store 读失败) vs terminal (store 读成功但空) — 信息保真度必须从 case 设计层做对](2026-04-28-local-store-transient-vs-terminal-must-distinguish.md) | 1 | error-handling, architecture | `fb4bfb7` | **[ADR-0008]** （case 拆分保留 §6.5；反模式 8.5 保留）
| 2026-04-30 | [文档时态精确性 vs 路径 B ADR Accepted 语义（与 codex review 的天然张力）](2026-04-30-doc-tense-vs-path-b-adr-acceptance.md) | 2 | docs, architecture | `55ae68c` |
| 2026-04-30 | [路径 B ADR §6 验证语义的 inline forward annotation（codex round 2 协调）](2026-04-30-adr-section-6-path-b-inline-semantics.md) | 1 | architecture, docs, process | `8a11f52` |
| 2026-04-30 | [Coordinator 必须镜像 server 加载的房间态 & 路由白名单缩窄时不能丢 presenter](2026-04-30-coordinator-must-mirror-loaded-home-room-state.md) | 2 | architecture | `5bb6ed5` |
| 2026-04-30 | [构造注入参数 + weak 存储字段在 fresh-instance 调用路径下的语义陷阱（init 注入字段必须 strong）](2026-04-30-strong-vs-weak-for-constructor-injected-state.md) | 1 | architecture | `8c9d991` |
| 2026-04-30 | [spec 钦定 SF Symbol 字符串前必须物理验证可用性](2026-04-30-spec-must-physically-verify-sf-symbol-strings.md) | 1 | spec-design, process | `b18c9d5` |
| 2026-04-30 | [codex `os_log CVarArg` 误报 + ui_design FadeIn 方向反转 + Avatar inset shadow 漏实现](2026-04-30-codex-os-log-cvararg-misdetect-and-ui-design-fidelity-drift.md) | 3 | process, style, ui-fidelity | `b18c9d5` |
| 2026-04-30 | [SwiftUI `.frame(maxWidth: .infinity)` 与 `.padding` 顺序对齐 CSS box-sizing（fullWidth 按钮溢出修复）](2026-04-30-swiftui-modifier-order-frame-vs-padding.md) | 1 | ui-fidelity | `d7abcbc` |
| 2026-04-30 | [SwiftUI strokeBorder vs stroke 内外绘语义 & ButtonStyle vs 自定义 DragGesture 取消语义](2026-04-30-swiftui-strokeborder-vs-stroke-and-buttonstyle-vs-draggesture.md) | 2 | style, architecture | `abc8ab3` |
| 2026-04-30 | [SwiftUI `.id(nil)` 共享 explicit identity 陷阱（ViewModifier 默认参容易踩）](2026-04-30-swiftui-explicit-id-nil-shared-identity.md) | 1 | architecture | `6a94989` |
| 2026-04-30 | [ViewModifier @State 跨 `.id` 重建幸存 + `.shadow` 投影到 children + fix-review 5 轮 cap 破例决议](2026-04-30-swiftui-state-survives-id-and-shadow-over-children.md) | 4 | architecture, ui-fidelity, process | `d7baa12` |
| 2026-04-30 | [引入 abstract method base class 时必须同步迁移所有 caller，不能留 fatalError 在 production 注入路径](2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md) | 1 | architecture, swift, refactor-discipline | `5f439a4` |
| 2026-04-30 | [SwiftUI `onChange(of:)` Equatable 重放契约 + `Task.sleep` 重置 timer 必须 cancel](2026-04-30-swiftui-onchange-equatable-and-stale-task-cancel.md) | 2 | architecture | `e8bb0cb` |
| 2026-04-30 | [Real ViewModel 的派生字段必须 override hydrate 入口 & 空 Text overlay 是 VoiceOver 陷阱](2026-04-30-realhomeviewmodel-greeting-and-empty-text-overlay.md) | 2 | architecture, a11y, ui | `0b2df22` |
| 2026-04-30 | [`@Published` 派生字段必须订阅 publisher（hydrate 入口 override 不够覆盖 reset 路径）+ `@Published` 用 import 必须显式](2026-04-30-published-derived-state-needs-publisher-subscription.md) | 2 | architecture, dependency | `a54481e` |
| 2026-04-30 | [SwiftUI 浮动动画必须由 @State position 变化驱动 + `.id()` 触发子视图重建（与 round 4 `.id(nil)` 共享 identity 陷阱不冲突）](2026-04-30-swiftui-floating-emoji-needs-state-driven-position.md) | 1 | ui-fidelity, architecture | `80d0ee6` |
| 2026-04-30 | [Real ViewModel 的占位 init 必须 seed UI Scaffold 全部字段（不能"等 sink 派发"）](2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md) | 1 | architecture | `32a9d3c` |
| 2026-04-30 | [RootView 持有的 ViewModel 必须在第一次 paint 之前同步 bind AppState](2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md) | 1 | architecture | `7556329` |
| 2026-04-30 | [Room host 名不得派生自 appState.currentPet（local 猫 ≠ room host 猫）](2026-04-30-room-host-name-must-not-derive-from-local-current-pet.md) | 1 | architecture | `4ee34b3` |
| 2026-04-30 | [Real 子类 override abstract method 的"占位实装"必须 mutate state，不能只 log（37-7 lesson 复犯）](2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md) | 1 | architecture, swift, ui-state | `7094e69` |
| 2026-04-30 | [Spec 边界灰区的 fallback 路径在 review 触发时必须立即兑现 epic AC](2026-04-30-spec-boundary-grey-area-fallback-must-honor-epic-ac-when-review-flags-it.md) | 1 | architecture, ui-fidelity, process | `b87d373` |
| 2026-04-30 | [Real ViewModel transient state 必须在 appState reset 路径同步清回 defaults（不只重算派生字段）](2026-04-30-real-viewmodel-must-clear-transient-state-on-reset.md) | 1 | architecture | `2929d78` |
| 2026-05-01 | [Real ViewModel transient state 清理判据必须用「user 身份变化」而非仅「user == nil」（cold-start 路径不经 appState.reset()）](2026-05-01-real-viewmodel-transient-must-clear-on-any-identity-change.md) | 1 | architecture | `18c0860` |
| 2026-05-01 | [Scaffold View 必须经 ViewModel method seam（按钮闭包 / sheet swipe-dismiss 都不能绕过）](2026-05-01-scaffold-bypass-viewmodel-seam.md) | 2 | architecture | `cc4d914` |
| 2026-05-02 | [SwiftUI `.sheet(onDismiss:)` 在按钮触发关闭时也会跑，必须用 dismissReason tag 做意图分发](2026-05-02-sheet-onDismiss-fires-on-button-close-too.md) | 1 | architecture | `c3f52ab` |
| 2026-04-30 | [UITest 自补 helper 与 XCTest SDK 方法 redeclaration](2026-04-30-uitest-helper-redeclares-sdk-method.md) | 1 | testing | `c0d7165` |
| 2026-05-01 | [单测复刻 view 内规则等于零守护：必须把规则下沉到纯函数 helper 与 view 共用](2026-05-01-test-must-share-helper-with-view-not-replicate-rules.md) | 1 | testing | `a65ae1b` |
| 2026-04-30 | [a11y coverage CI 脚本盲区：SCAN_DIRS 缺 App/ + regex 不覆盖 custom wrapper](2026-04-30-a11y-coverage-script-blind-spots-scan-dirs-and-custom-wrapper-regex.md) | 2 | testing | `ac6eb46` |
| 2026-05-02 | [ADR layering guard 必须 token-match 而非 `TypeName(` 构造调用 match](2026-05-02-static-guard-regex-must-token-match-not-constructor-call.md) | 1 | architecture | `331222b` |
| 2026-04-30 | [a11y coverage CI 脚本 window 算法 sound 性 (multi-control 同 view 的 sibling 顺势遮蔽 false negative)](2026-04-30-a11y-coverage-script-window-soundness.md) | 2 | testing | `4244c5f` |
| 2026-04-30 | [identifier helper 必须 source 自声明常量而非重新拼接（防 single-source-of-truth 静默 drift）](2026-04-30-identifier-helper-must-source-from-declared-constants.md) | 1 | architecture | `152c22b` |
| 2026-04-30 | [白名单条目必须 cite「完整流程文档 / 真实渲染路径」而非只挂视觉壳入口](2026-04-30-whitelist-entry-must-cite-full-flow-doc-and-real-render-path.md) | 2 | docs | `f04d82d` |
| 2026-04-30 | [白名单 r2：deferred artifact 的位置必须落到「真实承载源」而非视觉壳入口（roomCode/JoinRoomModal 在 app.jsx；ui_design SVG vs SwiftUI cat.fill）](2026-04-30-whitelist-entry-must-cite-full-flow-doc-and-real-render-path-2.md) | 2 | docs | `0e99f01` |
| 2026-04-30 | [白名单 r3：未来 routing 必须 cite 真实 Story scope（不能把 3D spike 错挂 Story 30.x、不能把 SwiftUI 实装路径错写成 prototype 替换）](2026-04-30-whitelist-future-routing-must-cite-real-story-scope.md) | 2 | docs | `20aec06` |
| 2026-04-30 | [白名单 r4：deferred 集合的"成员名 / 数量"必须以真实 code token 为准（theme set: candy/matcha/sky/dark 四套 vs 误写的 candy/dark/mono 三套）](2026-04-30-whitelist-deferred-set-must-mirror-real-code-tokens.md) | 1 | docs | `296ebca` |
| 2026-05-02 | [步数同步接口的"封顶判断方向"与"输入硬上限语义"两类边界契约陷阱](2026-05-02-step-cap-boundary-and-input-bound-contract.md) | 2 | docs | `848abd9` |
| 2026-05-02 | [步数同步接口 1005 限频维度错配 与 3001 "粘性错误码" 误述](2026-05-02-step-sync-rate-limit-scope-and-3001-stickiness-myth.md) | 2 | docs | `98283f9` |
| 2026-05-02 | [接口契约 story 必须连同时序图 + 数据库枚举一起锚定，不能只改 V1 接口文档](2026-05-02-cross-doc-contract-anchor-scope.md) | 2 | docs | `abbfa30` |
| 2026-05-02 | [step_account 示例数值不变量 & 同类已认证路由限频 scope 必须重复显式](2026-05-02-step-account-example-invariant-and-cross-section-rate-limit-scope.md) | 2 | docs | `5b68aef` |
| 2026-05-02 | [Story artifact 里的 AC 副本必须与主文档每轮 review fix 同步刷新](2026-05-02-story-artifact-ac-copy-must-mirror-doc-edits.md) | 3 | docs | `4cda156` |
| 2026-05-02 | [fix-review 修主文档时必须双向对齐"AC 副本 + 跨文档枚举注释"两类衍生文档](2026-05-02-fix-review-must-mirror-symmetric-edits-across-twin-files.md) | 2 | docs | `284fa42` |
| 2026-05-02 | [契约冻结必须钉死 prod 阈值，跨文档枚举名必须 canonical 化](2026-05-02-contract-freeze-must-pin-prod-thresholds-and-canonicalize-enum-names.md) | 2 | docs | `472cf3e` |
| 2026-05-02 | [fix-review 跨文档扫描必须包含上游 planning artifact（不能只扫 docs/ + story file）](2026-05-02-cross-doc-fix-must-sweep-planning-artifacts.md) | 1 | docs | `e844220` |
| 2026-05-02 | [Story file 内部规则副本必须通过"标准答案表"全文核对（不能只看 review 指出的两条）](2026-05-02-story-file-internal-rule-copies-must-pass-standard-answer-table-sweep.md) | 2 | docs / process | `030647f` |
| 2026-05-02 | [MySQL DATE 列 + GORM time.Time 的时区陷阱 & 配置 int64→int32 narrowing 静默扣款](2026-05-02-mysql-date-gorm-time-tz-pitfall.md) | 2 | architecture / config | `2d2b84a` |
| 2026-05-02 | [步数 sync 基线必须单调 + required 字段必须用指针 + DATE 列必须 string 透传](2026-05-02-step-sync-baseline-monotonic-required-pointer-and-date-string-transit.md) | 3 | architecture / error-handling | `b7a342a` |
| 2026-05-03 | [步数 sync 基线综合方案：id DESC + SUM 兜底，而非"乱序 vs reset 二选一"](2026-05-03-step-sync-baseline-sum-cap-not-max-order-by.md) | 1 | architecture | `5f3794b` |
| 2026-05-02 | [输入校验边界必须考虑下游存储真实约束（time.Parse 接受不代表 MySQL DATE 接受）](2026-05-02-input-validation-must-cover-downstream-storage-range.md) | 1 | error-handling / input-validation | `6f41b27` |
| 2026-05-03 | [步数 sync 第三层防御：截断 + 乱序组合下 SUM 兜底仍漏，需叠加 max-reported clamp](2026-05-03-step-sync-truncation-plus-ooo-needs-max-reported-clamp.md) | 1 | architecture | `9ba23b0` |
| 2026-05-04 | [步数 sync r6：reset 与"截断+乱序"二选一的产品权衡 + prod 配置覆盖必须靠 env var 强制](2026-05-04-step-sync-r6-reset-vs-ooo-tradeoff-and-prod-env-gate.md) | 2 | architecture / config | `be64bc3` |
| 2026-05-03 | [信任客户端 syncDate 的 anti-cheat 漏洞 + ±N 天容忍窗口的 trade-off](2026-05-03-step-sync-syncdate-rotation-attack-tolerance-window.md) | 2 | security / docs | `bf876ba` |
| 2026-05-04 | [DEBUG seed 必须串到 probe `.task` 里 await + HK 当日缓存是反优化](2026-05-04-debug-seed-vs-probe-await-coupling.md) | 2 | testing / concurrency / architecture | `<pending>` |
| 2026-05-04 | [HK read 授权必须 probe-read 推断 + UITest 0 是合法 fallback + preseed flag 不能只在 probe 路径生效](2026-05-04-healthkit-read-auth-and-preseed-flag-non-probe.md) | 3 | architecture / testing / api-contract | `<pending>` |
| 2026-05-04 | [HealthKit `.strictStartDate` 跨午夜 sample 的 trade-off（defer 而非 codex 钦定 fix）](2026-05-04-healthkit-strictstartdate-cross-midnight-tradeoff.md) | 1 | architecture | `<pending>` |
| 2026-05-04 | [HealthKit 当日窗口的 endDate 必须 clamp 到 now，不能用次日 0 点](2026-05-04-healthkit-today-enddate-clamp-to-now.md) | 1 | other (correctness) | `<pending>` |
| 2026-05-04 | [HealthKit dev-seed sample 必须落在过去（与读端 endDate clamp 配套）](2026-05-04-healthkit-preseed-sample-must-be-past-dated.md) | 1 | testing | `<pending>` |
