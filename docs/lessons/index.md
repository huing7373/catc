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
| 2026-05-04 | [DEBUG seed 必须串到 probe `.task` 里 await + HK 当日缓存是反优化](2026-05-04-debug-seed-vs-probe-await-coupling.md) | 2 | testing / concurrency / architecture | `55f5c4a` |
| 2026-05-04 | [HK read 授权必须 probe-read 推断 + UITest 0 是合法 fallback + preseed flag 不能只在 probe 路径生效](2026-05-04-healthkit-read-auth-and-preseed-flag-non-probe.md) | 3 | architecture / testing / api-contract | `851c87b` |
| 2026-05-04 | [HealthKit `.strictStartDate` 跨午夜 sample 的 trade-off（defer 而非 codex 钦定 fix）](2026-05-04-healthkit-strictstartdate-cross-midnight-tradeoff.md) | 1 | architecture | `61f6518` |
| 2026-05-04 | [HealthKit 当日窗口的 endDate 必须 clamp 到 now，不能用次日 0 点](2026-05-04-healthkit-today-enddate-clamp-to-now.md) | 1 | other (correctness) | `7d7b1b7` |
| 2026-05-04 | [HealthKit dev-seed sample 必须落在过去（与读端 endDate clamp 配套）](2026-05-04-healthkit-preseed-sample-must-be-past-dated.md) | 1 | testing | `9b55cc2` |
| 2026-05-04 | [MotionProvider stop/restart 的 stale callback 必须用 generation token 拦截 + UI test 不能把 (waiting) 占位当 PASS](2026-05-04-motion-stop-restart-stale-callback-race.md) | 2 | architecture / concurrency / testing | `4907398` |
| 2026-05-04 | [MotionProvider probe view 必须显式处理 deny 路径 + UI 集成测试不能依赖 simulator 自发 emit motion event](2026-05-04-motion-probe-deny-path-and-subscribed-marker.md) | 2 | error-handling / testing | `504ba40` |
| 2026-05-04 | [CoreMotion subscribe/stop 契约：handler invoke 必须与 generation check 共享同一锁段](2026-05-04-motion-handler-invoke-must-be-in-lock.md) | 1 | architecture / concurrency | `d875b7d` |
| 2026-05-04 | [pure mapper 内做 confidence debounce 让下游卡 stale；防抖应在带时间维度的上层做](2026-05-04-motion-mapper-confidence-debounce-removal.md) | 1 | architecture / correctness | `a0d869e` |
| 2026-05-04 | [MotionProvider.bind 必须先 gate authorizationStatus 再 startUpdates（first-launch 不弹权限红线）](2026-05-04-motion-bind-must-gate-on-authorization-status.md) | 1 | architecture | `9960e25` |
| 2026-05-04 | [SwiftUI body 内 switch 分支 swap 必须用 `.id() + .transition() + .animation(value:)` 三件套才能 fade](2026-05-04-swiftui-content-swap-needs-id-and-transition.md) | 1 | architecture | `28ee7c9` |
| 2026-05-04 | [auth-gated bind 必须挂 ScenePhase rebind 才能覆盖 "用户去 Settings 改权限再回来" 路径](2026-05-04-scenephase-rebind-for-auth-gated-subscriptions.md) | 1 | architecture | `28ee7c9` |
| 2026-05-04 | [UITest 不应钦定权限/异步事件依赖的 launch-time state；改为"三态任一"或"稳定 container"断言](2026-05-04-uitest-launch-state-must-not-hardcode-permission-gated-state.md) | 1 | testing | `ece7aa0` |
| 2026-05-04 | [auth-gated subscription 必须支持 downgrade，不能用单向 flag 短路 rebind](2026-05-04-auth-gated-subscription-must-handle-downgrade.md) | 1 | architecture | `0d597c3` |
| 2026-05-04 | [auth-gated feature 切片必须显式分配"权限申请 caller" story，spec gap 不能静默漏到末端 story](2026-05-04-auth-gated-feature-needs-explicit-requester-story.md) | 1 | architecture | `c457f93` |
| 2026-05-04 | [`Task { @MainActor in ... }` hop 引入 stale write race，generation token 是稳健解](2026-05-04-mainactor-task-hop-stale-write-needs-generation-token.md) | 1 | concurrency / architecture | `c457f93` |
| 2026-05-04 | [跨午夜时间字段必须从单一 captured Date 派生 & scenePhase 幂等 start 不能并排两条同效 API](2026-05-04-cross-midnight-single-captured-date-and-idempotent-start.md) | 2 | architecture | `8f24404` |
| 2026-05-04 | [fresh install HealthKit requestPermission gap：与 Story 8.4 motion 权限同坑（epic 切片漏专门 caller story 的二次复发）](2026-05-04-step-sync-fresh-install-requestpermission-gap.md) | 1 | architecture / process | `8f24404` |
| 2026-05-04 | [launch state 离开 .ready 时必须 stop 所有 .ready-attached 后台 service（对称生命周期）](2026-05-04-launch-state-leave-ready-must-stop-feature-services.md) | 1 | architecture | `f43bc89` |
| 2026-05-04 | [manual trigger 必须 await in-flight，不能用 gate 短路 return；公开 async API 的等待语义要明示](2026-05-04-manual-trigger-must-await-in-flight.md) | 1 | architecture | `1ec3855` |
| 2026-05-04 | [`await` 期间 race 让 single-flight gate 失效，必须 while-loop 链式等待 + @MainActor 同步段原子 assign（fresh install requestPermission gap 第 3 次复发 reaffirm wontfix）](2026-05-04-await-then-recheck-single-flight-gate.md) | 2 | concurrency / process | `1042af2` |
| 2026-05-05 | [WS 协议契约的"内部自洽"三连：reserved close code / 永久 null 字段引用 / 业务消息冻结边界](2026-05-05-ws-protocol-contract-internal-consistency.md) | 3 | architecture, docs | `f1c9057` |
| 2026-05-06 | [WS 协议冻结后的"示例字面量自洽"与"close code 全局保留"](2026-05-06-ws-frozen-examples-and-close-code-collision.md) | 2 | docs, architecture | `9f7b9e1` |
| 2026-05-06 | [`error` 消息的双重语义 & 心跳 close code 必须在冻结表里给具体值](2026-05-06-ws-error-dual-semantics-and-heartbeat-close-code.md) | 2 | docs, architecture | `397eea6` |
| 2026-05-06 | [WS 冻结段内部一致性（example 字段值 / 强制信封 / handshake 必发消息的失败路径）](2026-05-06-ws-frozen-section-internal-coherence-r4.md) | 3 | docs, architecture | `2a3936b` |
| 2026-05-06 | [WS close code 必须用 4xxx 应用自定义段隔离 §3 应用错误码 + 跨文档配置 key 双向锚定 (r5)](2026-05-06-ws-close-code-segment-discipline-r5.md) | 2 | architecture, docs, config | `9a78506` |
| 2026-05-06 | [Redis presence 不能替代 membership 授权 + room.snapshot 单一视图原则 + close code 表 1006 数字冲突清理 (r6)](2026-05-06-ws-frozen-section-authz-and-snapshot-coherence-r6.md) | 3 | architecture, docs | `11a1429` |
| 2026-05-06 | [握手 first-snapshot 契约的"启动时序原子性"+ placeholder 必须给真实可用最小值 + 跨文档时序图必须随冻结声明同步 (r7)](2026-05-06-ws-snapshot-startup-order-and-placeholder-r7.md) | 3 | architecture, docs | `4f52033` |
| 2026-05-06 | [room.snapshot placeholder 必须反映 room_members 全表（仅丰富字段降级）+ 节点 4 断连分支同样不广播 member.left (r8)](2026-05-06-ws-snapshot-placeholder-full-roster-and-disconnect-broadcast-r8.md) | 2 | architecture, docs | `c15f247` |
| 2026-05-06 | [V1 协议骨架冻结声明的范围必须收窄到具体 epic 阶段（不用"节点 X 阶段"）+ planning artifact 同步是契约最终化的最后一公里 (r9)](2026-05-06-ws-narrow-frozen-scope-and-planning-sync-r9.md) | 3 | architecture, docs | `9c82129` |
| 2026-05-06 | [`room.snapshot` authoritative-but-non-destructive merge 契约 + WS roomId 来源按场景分两路 (r10 收官)](2026-05-06-ws-snapshot-merge-contract-and-roomid-source-r10.md) | 2 | architecture, docs | `65b98f2` |
| 2026-05-06 | [Redis PoolSize 负值绕过 fail-fast，go-redis NewClient panic](2026-05-06-redis-poolsize-negative-makechan-panic.md) | 1 | error-handling, config | `b7c8dad` |
| 2026-05-06 | [go-redis ContextTimeoutEnabled 默认 false 让 ctx 形同虚设 + 抽象层 Close 真·幂等需要 sync.Once 兜底](2026-05-06-go-redis-context-timeout-and-close-idempotency.md) | 2 | config, architecture | `9acf65f` |
| 2026-05-06 | [WS Session.Send/Close 并发 panic & SessionManager 关停时 unregister hook 漏调](2026-05-06-ws-session-send-close-race-and-shutdown-hooks.md) | 2 | concurrency, error-handling, architecture | `63f505c` |
| 2026-05-06 | [WS reconnect 替换路径漏触发 onUnregister 钩子 + WSConfig 契约字段缺 prod 强制](2026-05-06-ws-reconnect-unregister-hook-and-prod-contract-gate.md) | 2 | architecture, config | `a5afc6c` |
| 2026-05-06 | [WS 路由必须 gate 在 backing tables migration 落地之后（启动期表存在性 sniff）](2026-05-06-ws-route-table-existence-gate.md) | 1 | architecture | `723fd99` |
| 2026-05-06 | [WS room 存在性来源 / pong 优先级 buffer / sessionID logger 字段（10-3 r4）](2026-05-06-ws-room-existence-source-and-pong-priority-r4.md) | 3 | architecture, observability | `e7cca25` |
| 2026-05-06 | [WS 路由"表存在性 gate"循环陷阱 + reconnect 替换的 broadcast 重叠窗口](2026-05-06-ws-table-existence-gate-and-reconnect-broadcast-window.md) | 2 | architecture | `ea60fbc` |
| 2026-05-06 | [WS 表存在性 sniff 不能依赖 information_schema 权限（r6）](2026-05-06-ws-table-probe-must-not-require-information-schema-privilege.md) | 1 | architecture | `5b88006` |
| 2026-05-06 | [房间存在校验补 status 过滤 & writeLoop priority 配额防 starvation（10-3 r7）](2026-05-06-ws-room-status-filter-and-priority-quota.md) | 2 | architecture, perf | `eeb3d11` |
| 2026-05-06 | [WS 表 probe 错误分流：misconfig 必须 fail-fast，transient 才能 warn-and-continue（r8 收窄 r6）](2026-05-06-ws-table-probe-misconfig-vs-transient-error-classification-r8.md) | 1 | error-handling | `248adc7` |
| 2026-05-06 | [sessionID 截断 8 字符 = birthday paradox 内存腐败（10-3 r9）](2026-05-06-ws-session-id-truncation-collision-r9.md) | 1 | architecture | `d063230` |
| 2026-05-06 | [reconnect 路径 destructive Register 必须延后到 transient IO 之后（10-3 r10）](2026-05-06-ws-handshake-register-after-snapshot-r10.md) | 1 | architecture | `8718b3f` |
| 2026-05-06 | [心跳超时扫描 TOCTOU & 4005 close frame 顺序保证（10-4 r1）](2026-05-06-ws-heartbeat-toctou-and-close-frame-ordering-10-4-r1.md) | 2 | concurrency | `0b68956` |
| 2026-05-06 | [closeInternal 必须 gate writeLoopDone wait 在 writeLoopStarted（10-4 r2）](2026-05-06-ws-close-skip-wait-when-writeloop-not-started-10-4-r2.md) | 1 | perf, concurrency | `50f7cb5` |
| 2026-05-06 | [closeInternal wait 上限不足 cover writeTimeout & scanner fanout 不响应 ctx（10-4 r3）](2026-05-06-ws-close-wait-timeout-and-shutdown-fanout-10-4-r3.md) | 2 | concurrency | `a9fd61b` |
| 2026-05-06 | [heartbeat scanner ctx 必须挂主 signal ctx & closeInternal wait 仅限 emitClose 路径（10-4 r4）](2026-05-06-ws-heartbeat-shutdown-ctx-and-close-wait-only-emit-10-4-r4.md) | 2 | architecture, perf | `b23aff3` |
| 2026-05-07 | [heartbeat fanout 必须用 WaitGroup drain & List 操作把 sort 移到 RUnlock 之后（10-4 r5）](2026-05-07-ws-heartbeat-fanout-drain-and-list-sort-outside-lock-10-4-r5.md) | 2 | architecture, perf | `33c7a5d` |
| 2026-05-07 | [shutdown 必须 wait goroutine 退出而不是只 signal cancel（10-4 r6）](2026-05-07-ws-shutdown-must-wait-for-goroutine-exit-not-just-signal-10-4-r6.md) | 1 | architecture | `d4cfc90` |
| 2026-05-07 | [WS BroadcastToRoom 同步 fanout + msg buffer ownership 隔离（10-5 r1）](2026-05-07-ws-broadcast-sync-fanout-and-msg-ownership-10-5-r1.md) | 2 | architecture | `c53abb2` |
| 2026-05-07 | [WS BroadcastToRoom 跨 goroutine 序一致性 & 大 N 性能测试 fixture 必须脱离 httptest（10-5 r2）](2026-05-07-ws-broadcast-cross-goroutine-ordering-and-stub-session-test-fixture-10-5-r2.md) | 2 | architecture, testing | `a06d9e2` |
| 2026-05-07 | [WS BroadcastToRoom 同 room hot path 必须 Load fast path 而非每次 LoadOrStore alloc 新 mutex（10-5 r3）](2026-05-07-ws-broadcast-load-fast-path-zero-alloc-hot-path-10-5-r3.md) | 1 | perf | `40b017e` |
| 2026-05-07 | [Redis presence RemoveOnline 必须带 sessionID guard 走 Lua atomic compare-and-delete（10-6 r1）](2026-05-07-redis-presence-remove-needs-session-id-guard-10-6-r1.md) | 1 | architecture | `6dd49ed` |
| 2026-05-07 | [Redis presence TTL 必须周期续期 & AddOnline 命令顺序必须让 partial-fail 不留永久 zombie（10-6 r2）](2026-05-07-presence-ttl-renewal-and-add-online-command-order-10-6-r2.md) | 2 | architecture, error-handling | `32b5d5b` |
| 2026-05-07 | [Presence reconcile 必须走 idempotent 全量重写而非纯 EXPIRE 续期 & TTL 显式配置必须 ≥ 2× scan interval（10-6 r3）](2026-05-07-presence-self-heal-and-ttl-min-bound-10-6-r3.md) | 2 | architecture, config | `0e75ede` |
| 2026-05-07 | [RemoveOnline 跨 room 必须 SREM 旧 room & scanner reconcile 必须 IsRegistered guard 防复活（10-6 r4）](2026-05-07-presence-cross-room-reconnect-srem-old-room-and-scanner-isregistered-guard-10-6-r4.md) | 2 | architecture | `74ec22f` |
| 2026-05-07 | [Scanner periodic reconcile 必须 fanout goroutine + per-call ctx timeout，不能在主 sweep 内同步调 Redis（10-6 r5）](2026-05-07-scanner-presence-reconcile-must-fanout-not-block-sweep-10-6-r5.md) | 1 | perf | `a80d8a8` |
| 2026-05-07 | [Presence lifecycle hook 必须 fire-and-forget & TTL 硬下限必须 prod-only env gate（10-6 r6）](2026-05-07-presence-hooks-fire-and-forget-and-ttl-floor-env-gate-10-6-r6.md) | 3 | perf, config | `b7ff1e4` |
| 2026-05-07 | [Presence same-room reconnect 必须 room-aware guard & shutdown 必须等 fire-and-forget hook 跑完才关共享 client（10-6 r7）](2026-05-07-presence-same-room-reconnect-needs-room-aware-guard-and-shutdown-must-wait-hooks-10-6-r7.md) | 2 | architecture, shutdown | `ed0e727` |
| 2026-05-07 | [fire-and-forget hooks 同 user 串行化 + scanner reconcile guard 升级 + AddOnline SADD/EXPIRE 原子化（10-6 r8）](2026-05-07-fire-and-forget-hooks-need-per-user-mutex-10-6-r8.md) | 3 | architecture, error-handling | `3329f33` |
| 2026-05-07 | [presence hook 必须改成同步调用 + 与 scanner reconcile 共享 per-user mutex 消除跨 goroutine 树 ordering race（10-6 r9）](2026-05-07-presence-hooks-must-be-synchronous-shared-mutex-10-6-r9.md) | 1 | architecture | `adcb1d3` |
| 2026-05-07 | [AddOnline 自动 SREM 旧 room stale member（cross-room reconnect 自愈）+ Register 替换路径必须先 onRegister(NEW) 再 replaced.Close() 消除 reconnect false offline window（10-6 r10）](2026-05-07-presence-add-online-cross-room-stale-srem-and-register-hook-order-10-6-r10.md) | 2 | architecture | `29d21fe` |
| 2026-05-08 | [房间 roster 契约自洽 / member.joined 必须自包含丰富字段（11-1 r1）](2026-05-08-room-roster-contract-self-consistency-11-1-r1.md) | 2 | architecture | `8f97a14` |
| 2026-05-08 | [HTTP leave 必须关闭 WS Session / member.joined trigger 示例必须含全 payload / 跨文档触发点声明对齐（11-1 r2）](2026-05-08-leave-must-close-ws-and-cross-doc-event-trigger-alignment-11-1-r2.md) | 3 | architecture, docs | `0b77824` |
| 2026-05-08 | [WS 断开仅清 ephemeral，房间归属只能由 HTTP leave 改变 / 跨文档 disconnect 语义必须合并为单条规则（11-1 r3）](2026-05-08-ws-disconnect-only-clears-ephemeral-not-membership-11-1-r3.md) | 2 | architecture | `507da05` |
| 2026-05-08 | [房间 create / leave 并发 race 必须落到正确业务码 / 冻结契约引用必须用稳定锚不依赖 commit hash（11-1 r4）](2026-05-08-create-leave-concurrency-races-and-stable-doc-anchors-11-1-r4.md) | 3 | architecture, docs | `c2996ba` |
| 2026-05-08 | [`pet.currentState` 枚举跨章节必须对齐 §6.4 `pets.current_state`（rest/walk/run）而非 §6.5 `motion_state`（stationary_or_unknown/walking/running）—— 同样是 1/2/3 三态不等于同义可复用命名（11-1 r5）](2026-05-08-pet-current-state-enum-must-not-alias-motion-state-11-1-r5.md) | 1 | docs | `bfe843b` |
| 2026-05-08 | [协议字段表 vs JSON 示例必须 zip 对齐 / prose 改了协议必须同步对应 mermaid sequenceDiagram（11-1 r6）](2026-05-08-snapshot-example-and-mermaid-diagram-must-mirror-prose-11-1-r6.md) | 2 | docs | `4471308` |
| 2026-05-08 | [接口默认 deny + 显式 allow（白名单 ACL）/ prose 步骤序与 mermaid sequenceDiagram 必须 zip 对齐（11-1 r7）](2026-05-08-default-deny-acl-and-prose-mermaid-zip-alignment-11-1-r7.md) | 2 | security, docs | `6a7866e` |
| 2026-05-08 | [ACL 校验 + 受 ACL 保护的数据返回必须共享同一事务 snapshot（11-1 r8）](2026-05-08-acl-and-protected-data-must-share-snapshot-tx-11-1-r8.md) | 1 | security, architecture | `814a5a1` |
| 2026-05-08 | [snapshot 隔离 ≠ 锁；ACL guard 需 FOR SHARE 锁不止 snapshot；跨事务状态字段 drift 需 FOR UPDATE 串行化（11-1 r9）](2026-05-08-snapshot-isolation-needs-row-locks-and-cross-tx-rooms-status-drift-11-1-r9.md) | 2 | security, concurrency, architecture | `9f3a569` |
| 2026-05-08 | [`pet` 字段必须随 §5.1 GET /home 全协议 nullable / WS close 4007 是 best-effort cleanup 而非 leave 完成的 authoritative confirmation（11-1 r10 收官）](2026-05-08-pet-nullable-and-cross-channel-ack-best-effort-11-1-r10.md) | 2 | architecture, docs, protocol | `86eba63` |
| 2026-05-08 | [给已有错误码新增接口用法时，必须反向扫描所有"该错误码仅用于 X"陈述（11-1 r11）](2026-05-08-stale-error-code-exclusivity-notes-when-adding-new-usage-11-1-r11.md) | 1 | docs | `ffeb321` |
| 2026-05-08 | [ACL FOR SHARE 锁的精确边界 = 事务持续期非 HTTP 响应发出时 / mermaid inline payload 必须随字段 nullable 改动同步（11-1 r12）](2026-05-08-acl-lock-precision-and-mermaid-pet-nullable-sync-11-1-r12.md) | 2 | docs | `2feff9e` |
| 2026-05-08 | [协议契约的步骤顺序必须可被现有 primitive 实装；step 顺序设计要校验 primitive capabilities（11-1 r13）](2026-05-08-protocol-step-order-must-match-primitive-capabilities-11-1-r13.md) | 1 | architecture | `f35c9e5` |
| 2026-05-08 | [跨章节同一 placeholder 阶段必须收敛到单一 going-forward 契约形态；既已落地实装与未来契约的差异要显式标注 backfill 路径，不留两种 shape 让 client parser 分流（11-1 r14）](2026-05-08-cross-section-placeholder-shape-must-converge-on-single-going-forward-contract-11-1-r14.md) | 1 | docs | `46ca85e` |
| 2026-05-08 | [启动期 fail-fast probe 的 if-guard 必须随依赖该资源的 handler 挂载条件同步前移（11-3 r1）](2026-05-08-schema-fail-fast-probe-must-track-handler-mount-condition-11-3-r1.md) | 1 | architecture, fail-fast | `4034e46` |
| 2026-05-09 | [Leave 路径 WS Session 清理跨 story defer 至 11.8 — SECURITY-DEFER 注释 + cross-story traceability 让 r2 codex 不重复 flag（11-5 r1）](2026-05-09-leave-room-ws-session-cleanup-defer-to-11-8-11-5-r1.md) | 1 | architecture | `312312e` |
| 2026-05-09 | [RoomService fire-and-forget 路径 nil sessionMgr guard 必须与 broadcastFn closure 同模式（11-8 r1）](2026-05-09-room-service-fire-and-forget-nil-sessionmgr-guard.md) | 1 | error-handling | `ada1773` |
| 2026-05-09 | [post-commit fire-and-forget 必须 detached ctx + 独立 goroutine + timeout 兜底（11-8 r2）](2026-05-09-post-commit-hooks-must-detached-ctx-async-timeout-11-8-r2.md) | 2 | architecture | `2c43802` |
| 2026-05-09 | [异步化必须保留同步可观察 invariants（11-8 r3：joiner self-fanout / leaver stale subscription）](2026-05-09-async-must-preserve-sync-observable-invariants-11-8-r3.md) | 2 | architecture | `a8082c7` |
| 2026-05-09 | [异步化路径必须按业务 key（roomID）做 serialization 才能保留 caller commit 顺序的 causal ordering（11-8 r4）](2026-05-09-async-causal-ordering-needs-per-room-mutex-11-8-r4.md) | 1 | architecture, concurrency | `4589ec5` |
| 2026-05-09 | [并发顺序保证：lock 必须在 caller 同步段获取；long-running side effect 不应持 serialization lock（11-8 r5）](2026-05-09-async-ordering-must-enqueue-in-caller-sync-section-11-8-r5.md) | 2 | architecture, concurrency | `f423c33` |
| 2026-05-09 | [commit-order = causal-order 必须 commit-time per-key serialization；caller 同步段任何工作都会破坏顺序（11-8 r6）](2026-05-09-commit-time-per-key-serialization-required-for-causal-order-11-8-r6.md) | 1 | architecture, concurrency | `084a125` |
| 2026-05-09 | [post-commit hook 三象限：sync 段必须 instant ops；slow ops 进 fire-and-forget；ordering-sensitive ops 进 worker queue（11-8 r7）](2026-05-09-post-commit-three-quadrants-instant-vs-slow-vs-ordered-11-8-r7.md) | 1 | architecture, concurrency | `b5e7c40` |
| 2026-05-09 | [per-room worker lifecycle defer 至 future story；MVP 节点 4 demo 阶段量化上界可控（11-8 r8）](2026-05-09-per-room-worker-lifecycle-defer-tech-debt-11-8-r8.md) | 1 (defer) | perf, architecture | `a23eae5` |
| 2026-05-09 | [fire-and-forget queue 满应阻塞背压而非 silent drop / defer tech-debt 必须在代码层加显著注释（11-8 r9）](2026-05-09-fire-and-forget-queue-blocking-backpressure-vs-silent-drop-11-8-r9.md) | 2 | architecture, process | `30b360f` |
| 2026-05-09 | [昂贵资源分配必须在 cheap validation 之后；attack vector 与 successful-path leak 同 family 时分层处理（11-8 r10）](2026-05-09-expensive-resource-allocation-after-cheap-validation-11-8-r10.md) | 1 | security, perf, architecture | `d3080fc` |
| 2026-05-09 | [snapshot+act 模式必须 atomic 持锁或 act 时 re-check 状态（11-8 r11）](2026-05-09-snapshot-then-act-must-recheck-or-be-atomic-11-8-r11.md) | 1 | concurrency | `3f50b78` |
| 2026-05-09 | [Published 订阅 dropFirst 丢 restored state & 房间切换需 roster 重置（12-1 r1）](2026-05-09-published-subscription-dropfirst-and-room-switch-roster-reset-12-1-r1.md) | 2 | architecture | `e73978f` |
| 2026-05-09 | [WebSocketClient 复用必须 prepareForReconnect 重置 stream & "空 roomId" 跨模块对齐 dispatcher（12-1 r2）](2026-05-09-ws-client-reuse-needs-stream-restart-and-empty-room-id-must-align-with-dispatcher-12-1-r2.md) | 2 | architecture | `8e5f182` |
| 2026-05-09 | [`room.snapshot` 必须按 room.id 校验丢弃 stale 消息（12-1 r3）](2026-05-09-stale-snapshot-discard-by-room-id-12-1-r3.md) | 1 | error-handling | `60ec81a` |
| 2026-05-09 | [`RoomMember.isHost` 不能用 snapshot 位置启发式推断（12-1 r4）](2026-05-09-snapshot-host-must-not-infer-from-position-12-1-r4.md) | 1 | other | `46ca502` |
| 2026-05-09 | [bind() 替换 client instance 必须先 disconnect 旧 client + cancel 旧 task（12-1 r5）](2026-05-09-bind-replace-must-disconnect-old-client-12-1-r5.md) | 1 | architecture | `791d942` |
| 2026-05-09 | [same-instance rebind 必须 true no-op；consumer restart 需 gated on 实际 client swap / first injection（12-1 r6）](2026-05-09-same-instance-rebind-must-true-noop-12-1-r6.md) | 1 | architecture, concurrency | `a4dd8dd` |
| 2026-05-09 | [WebSocket connect() 必须 await 握手结果 & DI 容器 fallback 必须跟随注入的 baseURL（12-2 r1）](2026-05-09-ws-connect-must-await-handshake-and-container-fallback-must-track-injected-baseurl-12-2-r1.md) | 2 | error-handling, architecture | `e9bcf77` |
| 2026-05-09 | [cross-room race 必须用 stream-startup 时刻捕获的 roomId 守护（payload 不带 room.id 的 WS 消息）（12-4 r1）](2026-05-09-cross-room-race-needs-stream-roomid-capture-12-4-r1.md) | 1 | architecture | `8841b92` |
| 2026-05-09 | [WS codec 必须在构造 payload 前校验 required 字段语义有效性（12-4 r2）](2026-05-09-ws-codec-must-validate-required-fields-12-4-r2.md) | 1 | error-handling | `7b4c0d2` |
| 2026-05-09 | [reconnect pre-handshake terminal close 必须分类 + connection-state 事件必须 streamRoomId 守护（12-5 r1）](2026-05-09-ws-reconnect-pre-handshake-close-classify-and-connection-state-stream-guard-12-5-r1.md) | 2 | error-handling, architecture | `7898ade` |
| 2026-05-09 | [WS reconnect 状态机用 generation counter 隔离 stale task 的共享状态写入（12-5 r2）](2026-05-09-ws-reconnect-generation-counter-isolates-stale-tasks-12-5-r2.md) | 2 | architecture | `c40fd4a` |
| 2026-05-09 | [WS reconnect: connectGate 也要 generation 守护（12-5 r3）](2026-05-09-ws-reconnect-connect-gate-needs-generation-scoping-12-5-r3.md) | 1 | architecture | `355fbe5` |
| 2026-05-09 | [WS reconnect: stream 复用 vs session 翻新必须双 generation 解耦（12-5 r4）](2026-05-09-ws-reconnect-stream-vs-session-generations-must-decouple-12-5-r4.md) | 1 | architecture | `26b9541` |
| 2026-05-09 | [WS reconnect: precondition 必须先于 gen 翻新 & connectGate 覆盖前必 resolve（12-5 r5）](2026-05-09-ws-reconnect-precondition-must-precede-gen-bump-and-gate-supersede-12-5-r5.md) | 2 | architecture | `0223877` |
| 2026-05-10 | [WS heartbeat 状态必须在 transient reconnect 前显式 reset（12-6 r1）](2026-05-10-ws-heartbeat-state-must-reset-before-transient-reconnect-12-6-r1.md) | 1 | architecture | `7af762a` |
| 2026-05-10 | [WS heartbeat .pong 必须按 requestId 配对当前 in-flight ping，不能无条件 ack 任意 pong（12-6 r2）](2026-05-10-ws-heartbeat-pong-requestid-correlation-12-6-r2.md) | 1 | error-handling | `27d8703` |
| 2026-05-10 | [WS heartbeat ping send 抛错必须强制走与 pong timeout 相同的 reconnect 路径，不能 silent return（12-6 r3）](2026-05-10-ws-heartbeat-ping-send-error-must-force-reconnect-12-6-r3.md) | 1 | error-handling | `b768fcb` |
| 2026-05-10 | [WS heartbeat lock-unlock 后必须 pre-send 重新校验 + 发送应使用 captured task 引用而非 re-read self.underlyingTask（12-6 r4）](2026-05-10-ws-heartbeat-unlock-window-pre-send-reverify-12-6-r4.md) | 1 | concurrency | `8d32439` |
| 2026-05-10 | [WS heartbeat send catch 路径必须用 task identity 守护，仅靠 sessionGeneration 不够（12-6 r5）](2026-05-10-ws-heartbeat-send-catch-must-guard-by-task-identity-12-6-r5.md) | 1 | concurrency | `f5d5c2c` |
| 2026-05-10 | [WS heartbeat: terminal post-handshake close 必须与 transient 分支对齐 cancel heartbeat 子系统（12-6 r6）](2026-05-10-ws-heartbeat-terminal-close-must-also-cancel-heartbeat-state-12-6-r6.md) | 1 | architecture | `0d8cc4f` |
| 2026-05-10 | [WS heartbeat send catch 路径不能用 cancel(.goingAway) 注入 1001 覆盖 server 真实 close code（12-6 r7）](2026-05-10-ws-heartbeat-send-catch-preserve-server-close-code-12-6-r7.md) | 1 | architecture | `7f46907` |
| 2026-05-10 | [WS heartbeat closeCode TOCTOU re-check 必须 atomic 折进 helper 内部，catch 入口先读 + helper 内 cancel 是 race window（12-6 r8）](2026-05-10-ws-heartbeat-toctou-recheck-must-be-atomic-in-helper-12-6-r8.md) | 1 | architecture / concurrency | `218639b` |
| 2026-05-10 | [WS connect 同步抛错与 URL path roomId 注入风险（12-7 r1）](2026-05-10-ws-connect-sync-failure-must-not-leave-stale-connected-12-7-r1.md) | 2 | error-handling, security | `<pending>` |
| 2026-05-11 | [UITEST 启动路径必须保留 UseCase nil-fallback + LeaveRoom 必须 guard target==current 防 stale-response 抹掉新房间（12-7 r2）](2026-05-11-uitest-fallback-and-leave-room-stale-response-guard.md) | 2 | testing, architecture | `<pending>` |
| 2026-05-11 | [WS connect 失败不走 errorPresenter & UITEST 路径必须跳过 real WS / errorPresenter wiring（12-7 r3）](2026-05-11-ws-connect-failure-and-uitest-real-ws-wiring-12-7-r3.md) | 2 | error-handling, testing | `<pending>` |
| 2026-05-11 | [stale connect(roomId:) failure 必须 gated on lastObservedRoomId（12-7 r4）](2026-05-11-ws-stale-connect-failure-must-be-gated-on-room-id-12-7-r4.md) | 1 | architecture | `<pending>` |
| 2026-05-11 | [leave thrown-error 也要 stale guard + create useCase nil 不能 hard no-op（12-7 r5）](2026-05-11-leave-thrown-error-stale-guard-and-create-nil-fallback-12-7-r5.md) | 2 | architecture | `<pending>` |
| 2026-05-11 | [CreateRoom / JoinRoom UseCase 必须 guard entry==current 防 stale-response 抹掉新房间（12-7 r6）](2026-05-11-create-join-room-guard-target-vs-current-against-stale-response-12-7-r6.md) | 1 | architecture | `<pending>` |
| 2026-05-11 | [RoomEndpoints percent-encode pre-escaped 输入也要 escape `%`（12-7 r7）](2026-05-11-room-endpoint-percent-escape-pre-encoded-12-7-r7.md) | 1 | security | `<pending>` |
