---
title: Tech Debt Registry (iPhone 端)
owner: iPhone 架构师（轮值 · 月度）
created: 2026-04-21
lastUpdated: 2026-04-21
status: active
purpose: 登记 architecture.md 决策中为 MVP 过度工程化风险而延迟到 Growth 的技术债 · 每条含触发条件 + owner + 补偿 deadline · 防止"延迟清单成为永不清算的墓碑"
source:
  - _bmad-output/planning-artifacts/architecture.md · Step 5 Growth 延迟清单
  - _bmad-output/planning-artifacts/architecture.md · Step 7 Winston 硬门槛 1
reviewFrequency: 每 Epic 收尾 + MVP 上线前
---

# Tech Debt Registry · iPhone 端

**本文件是 architecture.md 决策的"债务账本"**。架构文档 Step 4-6 多处采纳"先简化 MVP · Growth 再补"策略——这些决策如果不登记 + 不定触发条件 + 不定补偿 deadline，半年后就会漂移、被遗忘、或在线上事故时才被动暴露。

**Winston 硬门槛 1**（Step 7 Validation）：**无此登记表 = No-Go**。`_bmad-output/planning-artifacts/architecture.md · Architecture Readiness Assessment` 明确要求。

---

## 使用约束（Agent / Dev / PM 必读）

- **禁**"好心补回来"：若未出现本表登记的 trigger condition，**禁**在 MVP epic 内实施 Growth 版本（违反 PM John "过度工程化"批判）
- **禁**无 trigger 就延期：每条 debt 必须标 trigger · 无 trigger 的建议**不属于 tech debt**，属于"好想法墓地"（放别处，不进本表）
- **禁**无 owner 就关闭：trigger 触发 → owner 评估 → 决策"实施 / 继续延迟（更新 trigger）/ 放弃（写 rationale）"· **禁**静默消失
- **每 Epic 收尾必 review**：Epic retro 固定议程含"tech-debt-registry 触发扫描"

---

## 登记表（索引）

| ID | 延迟项 | 类型 | Trigger | Owner | Compensation Deadline | Status |
|---|---|---|---|---|---|---|
| TD-01 | P6 Error 4 类细分（`retryable` / `clientError` / `retryAfter` / `fatal`） | Pattern 细化 | 出现单错误码 4 类混用 · 或 `C-OPS-*` epic 启动 | iPhone 架构师 | Growth epic `C-OPS-*` 启动时 | ⏸ deferred |
| TD-02 | P7 Log 6 分类 × 5 级别（`network/ui/spine/health/watch/ws` × `debug/info/notice/error/fault`） | Pattern 细化 | MVP 3×3 证据不足（超过 2 次需"细分"的线上 issue） · 或 Crashlytics 接入时 | iPhone 架构师 | Crashlytics 接入时 · 或 MVP+3 月 | ⏸ deferred |
| TD-03 | P9 Feature Store 互 import SwiftLint lint | 工具守门 | Feature 数 ≥ 5 · 或发现 ≥ 1 次 Feature 互 import 违规 | iPhone 架构师 | Feature 数达 5 的 Story 落地前 | ⏸ deferred |
| TD-04 | O2 `@unchecked Sendable` 必须同行 `// FB\d+` SwiftLint custom rule | 工具守门 | 代码中出现任意 `@unchecked Sendable` 未归档 · 或跨 actor race bug 出现 | iPhone 架构师 | Trigger 出现后 1 周内 | 🔍 观察窗口 |
| TD-05 | O4 `#if WATCH_TRANSPORT_ENABLED` compile flag + CI linker symbol check | 工具守门 | `WatchTransportWCSession` 实装 Story 启动前 | iPhone 架构师 + Watch 团队 | Growth Watch fast-path Story 启动前 | ⏸ deferred |
| TD-06 | O5 SwiftUI PhaseAnimator/KeyframeAnimator 三栈分层（emoji 浮起 / 微回应改原生动画） | 技术栈选型 | Lottie 包维护成本超 2 人时/月 · 或发现 Lottie 与 Swift 6 并发冲突 | iPhone 架构师 + UX | MVP+3 月 reassess | ⏸ deferred |
| TD-07 | C-OPS-01 崩溃上报升级（Crashlytics vs Sentry） | 基础设施 | MVP 环形 JSONL 证据链覆盖率 < 60% · 或 App Store 审核有 crash 相关要求 | iPhone PM + 架构师 | MVP+1 月 | ⏸ deferred |
| TD-08 | C-OPS-02 版本检查 in-app 强更 | 基础设施 | MVP 原生 App Store 弹窗转化率 < 70% · 或出现严重 version-mismatch issue | iPhone PM | MVP+3 月 · 或出现 breaking API | ⏸ deferred |
| TD-09 | C-OPS-03 远程配置 / Feature Flag / Kill Switch | 基础设施 | 出现需紧急关闭功能的线上 issue · 或 A/B 测试需求 | iPhone PM + 架构师 | MVP+1 月 · 或上述 trigger | ⏸ deferred |

**Status 图例**：
- ⏸ **deferred**：等待 trigger · 未采取行动
- 🔍 **观察窗口**：已部分触发 · 持续监测（如首次出现 `@unchecked Sendable`）
- 🚧 **in progress**：trigger 已触发 · 实施中
- ✅ **closed**：已实施并关闭
- ❌ **abandoned**：评估后决定不做（必须写 rationale）

---

## 详细登记

### TD-01 · P6 Error 4 类细分

**背景**：`architecture.md` Step 5 P6 最终定 **MVP 2 类**（`retryable` / `failSilent`）· 原 Party Mode 设计的 4 类（`retryable` / `clientError` / `retryAfter` / `fatal`）推 Growth。PM John 批"30 维笛卡尔积是给 Growth 的"。

**MVP 现状**：
- `retryable`：5xx + 429 → 指数退避（max 3 次 / 500ms-4s · Retry-After header 覆盖）
- `failSilent`：4xx（非 429）→ 不重试 + 人话提示

**Growth 目标**：
- `retryable`（5xx）· `clientError`（4xx 非 429）· `retryAfter`（429 · 读 Retry-After）· `fatal`（401/403 清 token + 踢登录）

**Trigger（具体 · 二选一）**：
- 出现 **单错误码跨 2 类混用** bug（如 RATE_LIMITED 被当 clientError 处理导致 UX 不等 retry）
- `C-OPS-*` Growth epic（远程配置 / Crashlytics）启动——届时错误处理精细度需求提升

**Owner**：iPhone 架构师（月度轮值）

**Compensation Deadline**：Growth `C-OPS-*` epic 启动时 · 2 小时工作量

**关闭判据**：
- `ServerErrorCategory` 扩为 4 类 enum · 每个错误码在 `ServerError+Category.swift` 注册分类
- `architecture.md` Step 5 P6 更新为 4 类版本
- 测试覆盖 4 类各自重试路径（TestClock 注入）

**相关位置**：
- `CatShared/Sources/CatShared/Networking/ServerErrorCategory.swift`
- `architecture.md` Step 5 · Pattern P6

---

### TD-02 · P7 Log 6 分类 × 5 级别

**背景**：原 `project-context.md` 定 6 分类（`Log.network / ui / spine / health / watch / ws`）× 5 级别（`debug / info / notice / error / fault`）· Step 5 Party Mode PM John 砍到 **3×3 MVP**（`Log.network / ui / health` × `debug / info / error`）。

**MVP 现状**：
- 3 分类 × 3 级别
- `LogSink` protocol + `OSLogSink` + `RingBufferJSONLSink` + 崩后首启上报（零三方）

**Growth 目标**：
- 完整 6 分类 × 5 级别
- 加 `Log.spine` / `Log.watch` / `Log.ws`（`@Observable` 细分用）
- 加 `notice`（介于 info/error 之间 · 用于"非错但值得记"）/ `fault`（系统级崩溃前兆）

**Trigger（具体 · 二选一）**：
- MVP 3×3 证据不足：**超过 2 次线上 issue** 因日志分类/级别不够细而无法快速定位
- Crashlytics 接入（TD-07 触发）· 届时日志分类需求对齐 Crashlytics severity

**Owner**：iPhone 架构师

**Compensation Deadline**：Crashlytics 接入时 · 或 MVP+3 月 · 1 天工作量

**关闭判据**：
- `Log.swift` facade 扩为 6 分类
- `LogLevel` enum 扩为 5 级别
- 全项目 grep `Log.network | Log.ui | Log.health` 的调用点，按业务语义分散到新 6 分类
- `architecture.md` Step 5 P7 更新

**相关位置**：
- `CatShared/Sources/CatShared/Utilities/Log.swift`
- `architecture.md` Step 5 · Pattern P7

---

### TD-03 · P9 Feature Store 互 import SwiftLint lint

**背景**：`architecture.md` Step 5 P9 约定 Feature Store 禁互 import，跨 Feature 通过 `CatCore/<X>Service` protocol 或 `EventBus`。MVP 阶段 6 Feature 手工 review + PR checklist 勾选。PM John 批"现在写 lint 是给未来的自己"。

**MVP 现状**：
- PR checklist 第 N 条：`[ ] Feature Store 无互相 import`（人工勾）
- 6 Feature 数量小 · reviewer 可扫

**Growth 目标**：
- SwiftLint custom rule：regex 检测 `import .*Features\..*Store` 模式 · 非同 Feature 目录则 fail
- 集成到 `ios/scripts/build.sh --lint`

**Trigger（具体 · 二选一）**：
- Feature 数 ≥ 5（当前 6 个 Feature）· 实际上**已触发**但本规则是 Epic 2+ 新 Feature 加入时才重要
- 发现 ≥ 1 次 Feature 互 import 违规（通过线上 code review 漏检出现）

**Owner**：iPhone 架构师

**Compensation Deadline**：Feature 数达 5 的 Story 落地前 · 约 2 小时（写 regex + 测试）

**关闭判据**：
- `.swiftlint.yml` 新增 `feature_store_isolation` custom rule
- `ios/scripts/build.sh` 运行 SwiftLint lint 步骤通过
- 测试：故意写一个互 import → lint fail；改 protocol 通信 → lint pass
- `architecture.md` Step 5 P9 状态改为 "enforced by SwiftLint"

**相关位置**：
- `ios/.swiftlint.yml`
- `architecture.md` Step 5 · Pattern P9

---

### TD-04 · O2 `@unchecked Sendable` 必须同行 `// FB\d+` SwiftLint custom rule

**背景**：Step 4 Party Mode O2 · Murat 判必纳（16/25）· Amelia 判"Swift 6 严格并发已编译期报错 · 规则冗余"。用户选 **Amelia 立场延迟**。Winston 在 Step 7 第 1 条硬门槛 tech debt registry 提"先炸序 #1"——**O2 swiftlint 最先炸，观察窗口无触发条件 = 永不行动，代码风格分化最快**。

**MVP 现状**：
- D3 Sendable 解法 C 方案（`@unchecked Sendable` 逃逸舱）PR review 逐条质询
- 无自动 lint 守门

**Growth 目标**：
- SwiftLint custom rule：regex `@unchecked Sendable` 必须同一行或下一行有注释 `// FB\d+`（FB issue 归档）
- 无归档则 lint fail

**Trigger（具体 · 任一）**：
- 代码中出现**任意** `@unchecked Sendable` 未归档（PR reviewer 遗漏 · grep 扫描发现）
- 出现跨 actor race bug 线上 issue · 用户投诉 flaky behavior
- Epic 2+ retro 扫描发现 `@unchecked Sendable` 使用数 ≥ 3

**Owner**：iPhone 架构师

**Compensation Deadline**：Trigger 出现后 **1 周内** · 不超过 3 天工作量

**关闭判据**：
- `.swiftlint.yml` 新增 `unchecked_sendable_requires_fb_comment` custom rule
- 所有既有 `@unchecked Sendable` 点补 FB 归档（若无 FB，开 internal issue）
- `architecture.md` Step 5 P4 / Step 4 D3 状态更新

**相关位置**：
- `ios/.swiftlint.yml`
- `architecture.md` Step 4 · D3 · Step 5 · P2

**状态说明**：当前标 🔍 **观察窗口**（无触发条件 = 永不行动的风险是 Winston 明确点名的 · 本条 trigger 具体化防墓碑）

---

### TD-05 · O4 `#if WATCH_TRANSPORT_ENABLED` + CI linker symbol check

**背景**：Step 4 D5 WatchTransport 哲学 B 降级版 · MVP 仅 `WatchTransportNoop`，release binary 不链 `WCSession`。Winston 在 Party Mode 提 compile-time flag + CI linker symbol check 硬验证。用户选推延 Growth。

**MVP 现状**：
- `WatchTransportNoop` 唯一实现 · 无 `import WatchConnectivity`
- PR checklist 第 N 条：`[ ] 未 import WatchConnectivity`（人工勾）

**Growth 目标**：
- `#if WATCH_TRANSPORT_ENABLED` compile flag 包裹 `WatchTransportWCSession` 实装
- `project.yml` 分 build configuration：`Debug / Release / ReleaseWithWC` 三档
- CI 加 script：对 release binary 跑 `nm | grep WCSession` · 有匹配则 fail
- `architecture.md` Step 4 D5 状态更新

**Trigger（具体）**：
- `WatchTransportWCSession` 实装 Story 启动前（Growth Watch fast-path epic · 不早于 MVP 后）

**Owner**：iPhone 架构师 + Watch 团队（跨端协调）

**Compensation Deadline**：Growth Watch fast-path Story 启动前 · 1 天工作量

**关闭判据**：
- `project.yml` 三档 build configuration 配置
- CI `nm` symbol check 脚本就位
- 测试：debug build 含 WCSession / release build 不含 / ReleaseWithWC 含
- `architecture.md` Step 4 D5 + Step 5 P10 PR checklist 更新

**相关位置**：
- `ios/project.yml`
- `ios/scripts/check-binary-symbols.sh`（新增）
- `.github/workflows/ios.yml`

---

### TD-06 · O5 SwiftUI PhaseAnimator/KeyframeAnimator 三栈分层

**背景**：Step 4 Party Mode UX Sally 提议 emoji 浮起 / 微回应动画改用 SwiftUI 原生 `PhaseAnimator` / `KeyframeAnimator`（iOS 17+ 原生 · 比 Lottie 省一个量级）。用户选推延 Growth · 三人一致判"不纳"（Amelia/Murat/John）· 属 UI 表现层非架构 pattern。

**MVP 现状**：
- Lottie（猫凝视 onboarding · 呼吸 · 抬眼）+ 帧序列 PNG（微回应 3 个 · emoji 浮起）
- 一栈 Lottie + 一栈 PNG

**Growth 目标**：
- **三栈分层**：
  - Lottie：猫角色复杂表情（呼吸 / 抬眼 / 凝视 2.5s）· 设计师交 `.json`
  - **SwiftUI 原生**（`PhaseAnimator` / `KeyframeAnimator` / `TimelineView`）：emoji 浮起 2.5s / 微回应 3 个 0.4-0.5s / 装饰动画
  - 帧序列 PNG：仅留特殊特效 case（当前 UX v0.3 未列 · 可能 Growth 删除本栈）

**Trigger（具体 · 任一）**：
- **Lottie 包维护成本 > 2 人时/月**（升级 · bug 修复 · 资产生成）
- 发现 Lottie 与 Swift 6 严格并发 **Sendable 冲突**（Spine Spike 归 Epic 9 · Lottie 若踩同类坑）
- UX 设计师提出增加 ≥ 5 个新 Lottie 动画 · 触发"栈选型重估"

**Owner**：iPhone 架构师 + UX 设计师

**Compensation Deadline**：MVP+3 月 reassess · 或上述 trigger 出现后 2 周

**关闭判据**：
- 3 栈分工文档化到 `architecture.md` Step 4 D6
- emoji 浮起 / 微回应 3 个从帧序列 PNG 迁移到 SwiftUI 原生（或做 A/B 对比后决定保留 PNG）
- Lottie 使用范围明确限定（猫角色复杂表情）
- reduce-motion 降级路径 3 栈统一由 `@Environment(\.accessibilityReduceMotion)` 贯穿

**相关位置**：
- `CatPhone/Features/Account/CatMicroReactionView.swift`
- `CatShared/UI/EmojiEmitView.swift`
- `architecture.md` Step 4 · D6

---

### TD-07 · C-OPS-01 崩溃上报升级（Crashlytics vs Sentry）

**背景**：PRD `C-OPS-01` Round 6 Winston 拆分为 MVP / Growth。MVP 用 Xcode Organizer + 本地 RingBufferJSONL + 崩后首启上报（零三方 · Step 5 P7 + O6）。Growth 升级 Crashlytics 或 Sentry。

**MVP 现状**：
- `RingBufferJSONLSink` 本地环形缓冲（5MB 上限）
- 崩后首启检测上次非正常退出 → 读 ring buffer → `POST /v1/platform/crash-reports`（端点 G3 待 server 实装 · MVP 可先本地存）
- Xcode Organizer 基础 crash 查看

**Growth 目标**：
- 接入 Crashlytics（Firebase SDK）**或** Sentry（自托管 / 云）
- 选型评估：隐私 · 成本 · 与现有 `Log.*` facade 兼容性 · 合规
- 保留 `RingBufferJSONLSink` 作为 offline 兜底 · Crashlytics/Sentry 作为主通路

**Trigger（具体 · 任一）**：
- MVP RingBufferJSONL 证据链覆盖率 < 60%（线上 crash 有多少能从 ring buffer 恢复上下文）
- App Store 审核有 crash 相关要求（如要求 crash symbolication 报告）
- 用户投诉"app 崩了但没反馈"数 ≥ 10 / 月

**Owner**：iPhone PM + 架构师（联合评估）

**Compensation Deadline**：MVP+1 月 evaluate · 决定选型后 2 周接入

**关闭判据**：
- 选型决策文档化（Crashlytics vs Sentry）
- SDK 接入 · privacy manifest 更新（`PrivacyInfo.xcprivacy`）
- 与 `Log.*` facade 集成 · 崩溃自动上报
- TD-02 P7 Log 扩 6×5 协同（本 debt 依赖 TD-02）
- `architecture.md` Step 4 D7 + Step 5 P7 更新

**相关位置**：
- `CatShared/Sources/CatShared/Utilities/Log.swift`
- `CatPhone/Resources/PrivacyInfo.xcprivacy`
- `architecture.md` Step 4 · D7 (Deferred)

---

### TD-08 · C-OPS-02 版本检查 in-app 强更

**背景**：PRD `C-OPS-02` MVP 用 App Store 原生弹窗（`SKStoreProductViewController` 或系统提示）。Growth 升级 in-app 强更（force update gate · 用户无法跳过）。

**MVP 现状**：
- 启动时调 `/v1/platform/ws-registry` 检测 schema 版本 · 不兼容 → "请更新 app"（fail-closed 屏蔽主功能）
- 用户自行去 App Store 更新

**Growth 目标**：
- 新增 `/v1/platform/version-check` HTTP 端点（服务端返回 min_supported_version）
- 客户端启动时调 · 版本过低 → 全屏拦截 + 跳 App Store 更新链接
- 保留"查看更新说明"入口（温和，非生硬）

**Trigger（具体 · 任一）**：
- MVP 原生 App Store 弹窗转化率 < 70%（通过 analytics 追踪"见到更新提示 vs 实际更新"）
- 出现**严重 version-mismatch issue**（如旧版本调用新 API schema 导致崩溃）
- 推送 breaking API 变更前必须先落地 force update 机制

**Owner**：iPhone PM（主）+ 架构师（技术）

**Compensation Deadline**：MVP+3 月 · 或出现 breaking API 变更（trigger 提前）

**关闭判据**：
- server 端 `/v1/platform/version-check` 实装 · 契约登记 `docs/contract-drift-register.md`
- iPhone 启动流程加版本检查步骤
- UX 设计师出温和强更提示文案（避开"禁止使用"类生硬词 · 对齐 UX v0.3 "fail-closed 温柔文案"纪律）
- `architecture.md` Step 4 D7 更新

**相关位置**：
- `CatCore/AppCoordinator.swift`
- `architecture.md` Step 4 · D7 (Deferred)

---

### TD-09 · C-OPS-03 远程配置 / Feature Flag / Kill Switch

**背景**：PRD `C-OPS-03` MVP 硬编码配置（如 `Timeouts.swift` / `DesignTokens.swift` 编译期常量）。Growth 引入远程配置平台（Firebase Remote Config / LaunchDarkly / 自建 server endpoint）。

**MVP 现状**：
- `CatShared/Configuration/Timeouts.swift` 编译期常量
- 功能开关通过 `#if DEBUG` / `#if TESTING_HOOKS` compile flag
- 无紧急关闭通路

**Growth 目标**：
- server 端 `/v1/platform/config?userID=...&version=...` 返回 JSON 配置
- 客户端启动时拉 · 本地 cache · 关键配置可不重启生效
- A/B 测试支持（userID 分桶）
- Kill switch：紧急关闭表情广播 / 盲盒掉落等功能

**Trigger（具体 · 任一）**：
- 出现**需紧急关闭功能**的线上 issue（如 server bug 触发 iOS 客户端错误行为 · 需在不发版情况下关功能）
- A/B 测试需求提出（如 Growth "比心"、"晒太阳"、"困意"表情 vs 10+ 新表情的用户接受度对比）
- 某个配置（如盲盒 30min 计时 / 1000 步门槛）需运营热调整

**Owner**：iPhone PM + 架构师

**Compensation Deadline**：MVP+1 月 · 或上述 trigger 出现后 3 周

**关闭判据**：
- server 端 `/v1/platform/config` 实装 · 契约登记
- iPhone 启动流程加 config 拉取步骤 · 启动慢则 fail-closed "网络累了"
- `CatCore/RemoteConfig.swift` 新增 `@Observable` 全局 Store
- Feature Flag enum 定义 · PR review 新增勾选项"是否需要 Feature Flag 门禁"
- Kill switch 覆盖的核心 feature list 文档化（表情广播 / 盲盒掉落 / 房间加入）
- `architecture.md` Step 4 D7 更新

**相关位置**：
- `CatCore/Sources/CatCore/Config/RemoteConfig.swift`（新增）
- `architecture.md` Step 4 · D7 (Deferred)

---

## 附录 · 观察窗口项（trigger 已部分触发 · 持续监测）

### TD-04 · `@unchecked Sendable` swiftlint（🔍 观察窗口）

**监测指标**：
- 每 Epic 收尾扫 `grep -r "@unchecked Sendable" ios/` 统计出现数 · 记本文件
- PR reviewer 遇到 `@unchecked Sendable` 时确认是否有 FB 归档注释 · 若无则**本 Epic retro 必提**

**扫描记录表**（每 Epic 填）：

| Epic | `@unchecked Sendable` 数 | 未归档数 | 扫描日期 | 扫描人 | 备注 |
|---|---|---|---|---|---|
| Epic 0 | N/A | N/A | 2026-04-21 | iPhone 架构师 | 基线：无 Swift 代码 |
| Epic 1 | TBD | TBD | TBD | TBD | |
| Epic 2 | TBD | TBD | TBD | TBD | |

**触发升级条件**：任一行"未归档数 ≥ 1" → TD-04 状态从 🔍 观察窗口 → 🚧 in progress · 立即实施

---

## 附录 · Retro Checklist（每 Epic 收尾必过）

每个 Epic retro 会议固定议程：

```
[ ] 扫描 tech-debt-registry · 是否有 trigger 已触发？
  [ ] TD-01 · TD-02 · TD-03（pattern 类）
  [ ] TD-04（观察窗口 · 扫描 @unchecked Sendable 数）
  [ ] TD-05 · TD-06（工具 / 栈类）
  [ ] TD-07 · TD-08 · TD-09（基础设施类）
[ ] 若有 trigger 触发：决策（实施 / 继续延迟 · 更新 trigger / 放弃 · 写 rationale）
[ ] 若无：更新扫描记录表（TD-04）
[ ] 检查是否有**新 tech debt**需要登记（本 Epic 内的新 "先简化 · Growth 再补" 决策）
[ ] 本文件 lastUpdated 字段更新
```

**新 tech debt 登记模板**：见本文件 TD-01 至 TD-09 格式 · 必填 Trigger + Owner + Deadline + 关闭判据

---

## 附录 · 放弃（abandoned）项记录

若某条 tech debt 评估后决定不做（如"市场不再要求" / "技术栈升级自动解决"），状态改为 ❌ **abandoned** · 必须在本节写**放弃 rationale**：

| ID | 放弃日期 | 放弃人 | Rationale |
|---|---|---|---|
| — | — | — | —（当前无 abandoned 项） |

---

**最后更新**：2026-04-21 · iPhone 架构师 initial 创建
**下次 review**：Epic 1 收尾时
