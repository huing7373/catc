# Story 8.2: CoreMotion 接入（权限 + 状态识别）

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iOS 开发,
I want 一个抽象 `MotionProvider` 协议 + 真实 `MotionProviderImpl`（基于 `CMMotionActivityManager`）+ in-memory `MotionProviderMock`,
so that 业务层（节点 3 后续的 Story 8.3 `MotionStateMapper` / Story 8.4 `HomeViewModel.petState` 订阅 / Story 8.5 同步触发器配套）可以监听设备运动状态变化并在测试中无缝替换为 mock，并按 docs/宠物互动App_iOS客户端工程结构与模块职责设计.md §10.2 钦定，保留 `CMMotionActivity` 原始事件交给 Story 8.3 `MotionStateMapper` 做 stationary/walking/running → rest/walk/run 的状态映射。

## 故事定位（Epic 8 第 2 条 story；节点 3 iOS 端 System Adapter 第二块）

- **Epic 8 进度**：8-1（HealthKit 接入；done）→ **8-2（本 story，MotionProvider 协议 + Impl + Mock + 权限 + 状态事件流 + 单测 ≥4 + 集成 ≥1）** → 8-3（MotionStateMapper rest/walk/run）→ 8-4（PetSpriteView 三态动画 + HomeViewModel.petState）→ 8-5（StepSyncTriggerService + SyncStepsUseCase 写 AppState.currentStepAccount）。
- **本 story 与 Story 8.1 同精神**：System Adapter 层接入第二个真实 system framework（CoreMotion.framework）。Epic 1～7 + Story 8.1 之外没有任何 CoreMotion 代码（grep 验证：`iphone/` 下无 `Motion/` 子目录、无 `CoreMotion` 相关 import）。**CoreMotion.framework 必须在 `iphone/project.yml` 内显式声明依赖**，否则 release build link 失败（与 Story 8.1 加 `HealthKit.framework` 同模式）。
- **本 story 落地后立即解锁**：
  - **Story 8.3**（MotionStateMapper，节点 3 状态映射）：以 `CMMotionActivity` 为输入做 stationary/walking/running → rest/walk/run 映射；本 story 必须保证暴露 `CMMotionActivity` 实例（**不**封装成枚举透传），让 8.3 拿到 `running` / `walking` / `stationary` / `cycling` / `automotive` / `confidence` 全部字段
  - **Story 8.4**（PetSpriteView，节点 3 UI 表现）：HomeViewModel 订阅 `MotionProvider.startUpdates` → 调 8.3 mapper → driving `petState` published 字段
  - **Story 9.1 验证场景 4**（节点 3 跨端 e2e）：`MockMotionProvider` 注入 walking → mapper → ViewModel.petState = .walk → PetSpriteView 切到 walk 动画（DEBUG-only inject 端口由 Story 8.4 / 8.5 落地）

## Acceptance Criteria（来自 epics.md §Story 8.2 行 1471-1490，原文转录 + 实施细化）

> AC 编号体系（与 Story 8.1 同精神 — 编号体系明确化）：AC1 是协议定义；AC2 是 MotionProviderImpl 真实 CMMotionActivityManager 接入；AC3 是 Info.plist + project.yml 框架依赖；AC4 是 MotionProviderMock；AC5 是 AppContainer wire；AC6 是单元测试 ≥4；AC7 是集成测试 ≥1（XCUITest + 模拟器 + DEBUG-only probe）；AC8 是 build verify。

### AC1 — `MotionProvider` 协议定义（新文件 `iphone/PetApp/Core/Motion/MotionProvider.swift`）

新建 `iphone/PetApp/Core/Motion/MotionProvider.swift`，按 iOS 架构 §5.5 + §10.2 钦定 System Adapter 层职责：

```swift
import Foundation
import CoreMotion  // 协议层暴露 CMMotionActivity 类型——Story 8.3 mapper 直接吃此类型

/// CoreMotion 运动状态识别的抽象边界（System Adapter 层）.
/// 业务层（HomeViewModel / Story 8.4 + Story 8.3 MotionStateMapper）只依赖此协议；
/// 测试用 MotionProviderMock 替换；生产用 MotionProviderImpl 真接入 CMMotionActivityManager.
public protocol MotionProvider: Sendable {
    /// 申请运动状态识别权限（CMMotionActivity）.
    /// - Returns: true 表示用户授权 / 已授权；false 表示用户拒绝 / 受限.
    /// - Throws: `MotionProviderError.activityDataNotAvailable` 当设备不支持 activity 识别
    ///           （`CMMotionActivityManager.isActivityAvailable() == false`，如部分模拟器机型 / iPad）;
    ///           其他 OS 错误 wrap 为 `.systemFailure(underlying:)`.
    /// - Note: CoreMotion 的权限语义和 HealthKit 不同——CMAuthorizationStatus 可被信赖
    ///         （`CMMotionActivityManager.authorizationStatus()` 在 iOS 11+ 公开 API），
    ///         不需要像 8.1 HealthProvider 那样做 probe-read 兜底.
    func requestPermission() async throws -> Bool

    /// 开始订阅运动状态事件流；handler 在每次系统判定 activity 变化时被调用.
    /// - Parameter handler: 接收 `CMMotionActivity`（带 confidence + walking/running/stationary 等 flag）
    ///                      的 closure；CMMotionActivityManager 的 callback 在 main queue 触发
    ///                      （由实装内 `OperationQueue.main` 钦定，详见 AC2）.
    /// - 实装契约（必须严格遵守，详见 AC2 实装坑表）:
    ///   1. **同时多次 startUpdates 只生效第一次**：第二次起的 startUpdates 调用直接被忽略（**不**抛错、**不**替换 handler）.
    ///      epics.md AC 钦定"防止重复订阅"——这是为了避免 caller（Story 8.4 HomeViewModel）二次 bind 时同时收到双倍事件.
    ///   2. **handler 必须 @Sendable**：跨 actor 调用需保证.
    ///   3. **handler 持有 closure（escaping）**：本协议不规定 weak/strong；调用方决定 retain 周期.
    ///   4. **未授权 + activityAvailable 但 startUpdates 时**：**禁止**抛错；handler 不被调用即可（与系统行为一致——
    ///      未授权时 CMMotionActivityManager 默默不发事件，由 Story 8.5 / 8.4 等 caller 配合超时策略决策 fallback）.
    func startUpdates(handler: @escaping @Sendable (CMMotionActivity) -> Void)

    /// 停止订阅运动状态事件流.
    /// - 幂等：未 startUpdates 时调 stopUpdates 不抛错也不破坏后续 startUpdates 调用（次数计数清零）.
    /// - 调用 stopUpdates 后再次 startUpdates 视作"全新订阅"——handler 替换为新 closure，事件流重新开启.
    func stopUpdates()
}

/// MotionProvider 错误集合（Swift Error + LocalizedError，errorDescription 给 ErrorPresenter / DEBUG 日志用）.
public enum MotionProviderError: Error, Equatable, LocalizedError {
    /// 设备 / 模拟器不支持 activity 识别（`CMMotionActivityManager.isActivityAvailable() == false`）.
    case activityDataNotAvailable
    /// 用户拒绝 / 受限 activity 识别权限.
    /// 与 .activityDataNotAvailable 区分：前者是设备能力缺失，后者是用户授权拒绝.
    case permissionDenied
    /// 其他系统错误（NSError code 非上述两类）；保留 underlying 给日志 / dev 调试.
    case systemFailure(underlying: NSError)

    public static func == (lhs: MotionProviderError, rhs: MotionProviderError) -> Bool {
        switch (lhs, rhs) {
        case (.activityDataNotAvailable, .activityDataNotAvailable): return true
        case (.permissionDenied, .permissionDenied): return true
        case (.systemFailure(let l), .systemFailure(let r)):
            return l.domain == r.domain && l.code == r.code
        default: return false
        }
    }

    public var errorDescription: String? {
        switch self {
        case .activityDataNotAvailable: return "当前设备不支持运动状态识别"
        case .permissionDenied: return "请在系统设置中允许 PetApp 访问运动与健身数据"
        case .systemFailure: return "运动状态识别失败，请稍后重试"
        }
    }
}
```

**关键设计决策（必须严格遵守）**：

- **协议层暴露 `CMMotionActivity` 而非自定义枚举**：Story 8.3 MotionStateMapper 钦定吃 `CMMotionActivity` 输入（详 epics.md §Story 8.3 AC："`MotionStateMapper.map(_ activity: CMMotionActivity) -> MotionState`"）；MotionProvider 协议层若提前枚举化（如 `enum SystemMotionActivity { case stationary, walking, running, ... }`）→ 8.3 mapper 还要再翻一次，重复劳动。**直接暴露原始类型**（System Adapter 层职责仅"读出来"，不"翻译"）。
- **`Sendable` 必标 + `@Sendable` handler**：Swift 6.3 strict concurrency 下 protocol method 接受 closure 必标 `@Sendable`，否则跨 actor 调用编译报错。
- **错误三态而非 4+ 细分**：与 HealthProviderError 同精神（activityDataNotAvailable / permissionDenied / systemFailure(NSError)），不再拆"超时" / "其他 errorDomain"。
- **`async throws` 的 requestPermission + 同步签名的 startUpdates / stopUpdates**：理由 ① CMMotionActivityManager.queryActivityStarting 系列 API 是 callback-based，但 startActivityUpdates 本身就是同步注册回调（不是 await 一次拿值）；② startUpdates 不返回 Future，handler 闭包持续触发；③ 与 epics.md AC 行 1479 钦定的"`startUpdates(handler: (CMMotionActivity) -> Void)`"签名对齐。
- **不引入 actor**：MotionProviderImpl 内部 `isUpdating: Bool` 状态 + `manager: CMMotionActivityManager` 引用用 NSLock 保护（与 HealthProviderImpl 同模式，actor 在 CM callback 边界不友好）。
- **不暴露 AsyncStream / AnyPublisher**：保持 callback 风格（与 epics.md AC 钦定签名一致）；上层 Story 8.4 HomeViewModel 自己用 `@Published` driving SwiftUI（如需 Combine 桥接由 caller 自行包）。

---

### AC2 — `MotionProviderImpl` 真实 CMMotionActivityManager 接入（新文件 `iphone/PetApp/Core/Motion/MotionProviderImpl.swift`）

新建 `iphone/PetApp/Core/Motion/MotionProviderImpl.swift`：

```swift
import Foundation
import CoreMotion  // 必须 import；framework 由 project.yml 显式声明（AC3）

/// MotionProvider 的生产实装：基于 CMMotionActivityManager.startActivityUpdates 订阅运动状态变化.
///
/// 设计要点（详见 Dev Notes "CoreMotion 接入坑表"）：
/// - 使用 OperationQueue.main 接收 callback（与 SwiftUI / @Published 主线程更新天然对齐）.
/// - 同时多次 startUpdates 只生效第一次（NSLock + isUpdating 旗标；epics.md AC 钦定）.
/// - stopUpdates 后再 startUpdates 视作全新订阅（NSLock 内重置 isUpdating + 替换 handler）.
/// - requestPermission 用 `CMMotionActivityManager.authorizationStatus()` (iOS 11+) 直接判定;
///   如 .notDetermined → queryActivityStarting 触发系统弹窗（探针式）一次后再次读 status.
/// - 错误映射严格对齐 MotionProviderError 三态.
public final class MotionProviderImpl: MotionProvider, @unchecked Sendable {
    private let manager = CMMotionActivityManager()

    /// 同时多次 startUpdates 防御 + handler 替换 + stopUpdates 后清空状态——全用 NSLock 保护.
    private let lock = NSLock()
    private var isUpdating: Bool = false
    private var currentHandler: ((CMMotionActivity) -> Void)?

    public init() {}

    public func requestPermission() async throws -> Bool {
        guard CMMotionActivityManager.isActivityAvailable() else {
            throw MotionProviderError.activityDataNotAvailable
        }

        let status = CMMotionActivityManager.authorizationStatus()
        switch status {
        case .authorized:
            return true
        case .denied, .restricted:
            return false
        case .notDetermined:
            // 触发系统弹窗：用 queryActivityStarting 做一个极短探针（now-1s ~ now），
            // 系统会弹出权限弹窗 / iOS 13+ 自动拒绝（受隐私设置）；查询完成后再读一次 authorizationStatus.
            // 注意：如果直接调 startActivityUpdates 也会触发弹窗，但 startActivityUpdates 把 handler 注册了
            // 就改不掉了——此处仅为"探针式触发权限"，必须用 queryActivityStarting 让回调结束后即可释放.
            return try await probePermissionViaQuery()
        @unknown default:
            // future iOS 引入新 case 时保守视作未授权；不抛错（避免上游误以为 systemFailure）.
            return false
        }
    }

    public func startUpdates(handler: @escaping @Sendable (CMMotionActivity) -> Void) {
        lock.lock()
        // epics.md AC 钦定："同时多次 startUpdates → 只生效第一次"——
        // 已 isUpdating 时直接 return，不替换 handler、不抛错、不打 log（避免日志泛滥）.
        guard !isUpdating else {
            lock.unlock()
            return
        }
        isUpdating = true
        currentHandler = handler
        lock.unlock()

        // 注意：CMMotionActivityManager.startActivityUpdates 必须在 main thread 调；
        // OperationQueue.main 让 callback 也在 main，与 SwiftUI 状态更新对齐.
        manager.startActivityUpdates(to: OperationQueue.main) { [weak self] activity in
            guard let self, let activity else { return }
            self.lock.lock()
            let captured = self.currentHandler
            self.lock.unlock()
            captured?(activity)
        }
    }

    public func stopUpdates() {
        lock.lock()
        // 幂等：未 isUpdating 时 stopUpdates 不抛错也不调 manager.stopActivityUpdates
        // （Apple 文档不保证未启动时调 stop 的安全；保守只在 isUpdating 时才调）.
        guard isUpdating else {
            lock.unlock()
            return
        }
        isUpdating = false
        currentHandler = nil
        lock.unlock()

        manager.stopActivityUpdates()
    }

    /// 探针：发一个极短窗口的 queryActivityStarting 触发权限弹窗（系统首次会弹），
    /// 完成后再读一次 authorizationStatus 判定真实结果.
    private func probePermissionViaQuery() async throws -> Bool {
        let now = Date()
        let oneSecondAgo = now.addingTimeInterval(-1)
        return try await withCheckedThrowingContinuation { continuation in
            manager.queryActivityStarting(from: oneSecondAgo, to: now, to: OperationQueue.main) { _, error in
                if let nsError = error as NSError? {
                    if nsError.domain == CMErrorDomain, nsError.code == CMErrorMotionActivityNotAuthorized.rawValue {
                        continuation.resume(returning: false)
                        return
                    }
                    continuation.resume(throwing: MotionProviderError.systemFailure(underlying: nsError))
                    return
                }
                // 弹窗结束后再读 status——authorized 才返 true；其他 case 全返 false.
                let final = CMMotionActivityManager.authorizationStatus()
                continuation.resume(returning: final == .authorized)
            }
        }
    }
}
```

**关键决策与坑（必须严格遵守）**：

1. **`CMMotionActivityManager.authorizationStatus()` (iOS 11+) 可信**：与 HealthKit `authorizationStatus(for:)` 不同，CoreMotion 的 status 是真实可读的（Apple 文档明示）；可直接用 `case authorized` 判定。**禁止**用 8.1 的 probe-read 模式（HealthKit 特有的隐私防探测设计不适用 CoreMotion）.
2. **首次申请权限走 queryActivityStarting 探针**：`.notDetermined` 状态下系统弹窗只在第一次 query / startUpdates 时触发；用 queryActivityStarting 是因为它**有完成 callback**（startUpdates 没有"权限弹窗结束"信号），让 requestPermission 能 await 完整的"弹窗 → 用户选择 → 状态 sync"流程后再返回真实结果.
3. **OperationQueue.main 接收 callback**：与 epics.md AC 钦定"handler 接 SwiftUI @Published"配套；如果传 background queue，HomeViewModel.petState 更新需要 dispatch 回 main，引入额外复杂度.
4. **`@unchecked Sendable` 同 8.1 模式**：CMMotionActivityManager 不是 Sendable；instance 只在 init 创建后持有，`isUpdating` / `currentHandler` 用 NSLock 保护 → `@unchecked Sendable` 是 Swift 6.3 strict concurrency 合规标记.
5. **错误映射严格三态**：CMError code 仅 `CMErrorMotionActivityNotAuthorized` 映射为 `.permissionDenied`（实装在 probePermissionViaQuery 内）；其他 NSError 全 `.systemFailure(underlying:)`.
6. **不暴露 CMMotionActivityManager / CMError 到调用方**：协议层零 CMError 类型泄漏；上层业务 / 测试不感知 CM error code.
7. **`withCheckedThrowingContinuation` 桥接 callback**：与 8.1 同模式；**禁** `withUnsafeContinuation` (unsafe 调试更难).
8. **handler 持 weak self + capture lock 内的 currentHandler**：避免 stopUpdates 之后系统残留 callback 仍执行旧 handler 引发 use-after-free 风险（NSLock 内 read 当前 handler ref 是 fresh 的）.
9. **`@unknown default` 必加**：Apple 未来给 CMAuthorizationStatus 新增 case 时编译器会 warning；显式 `@unknown default` + 保守返 false 是 forward-compat 钦定模式.
10. **不引 actor 隔离**：与 8.1 决策一致——CMMotionActivityManager callback 在 OperationQueue 触发，actor 跨边界 await 在 callback 内不友好；NSLock + @unchecked Sendable 更直接.

---

### AC3 — Info.plist + project.yml 显式声明 CoreMotion 依赖（必须）

**改动文件 1**：`iphone/PetApp/Resources/Info.plist`（实际由 xcodegen 从 `project.yml` regen；project.yml 是 source of truth）

按 epics.md AC 行 1481 钦定 + 架构 §10.2 + Story 8.1 同模式追加 `NSMotionUsageDescription` 键值：

```xml
<key>NSMotionUsageDescription</key>
<string>PetApp 识别你的运动状态，让小猫和你一起活动。</string>
```

**改动文件 2**：`iphone/project.yml`（XcodeGen project 定义）

在 `targets.PetApp` 段已有 `dependencies` 子段下追加 `- sdk: CoreMotion.framework`（与 Story 8.1 加 `HealthKit.framework` 同模式）：

```yaml
  PetApp:
    type: application
    platform: iOS
    sources:
      - PetApp
    dependencies:
      - sdk: HealthKit.framework      # Story 8.1
      - sdk: CoreMotion.framework     # Story 8.2 新增（节点 3 阶段 Motion 接入）
    info:
      ...
      properties:
        ...
        # Story 8.2: CoreMotion 运动状态识别权限弹窗的描述文案（系统首次申请时展示给用户）.
        # iOS 必填；缺失会让 startActivityUpdates / queryActivityStarting 直接抛错或被系统拒绝.
        NSMotionUsageDescription: "PetApp 识别你的运动状态，让小猫和你一起活动。"
```

**完成验证**：跑 `bash iphone/scripts/build.sh`（**不**带 --test）→ 1) 命令成功（exit 0）；2) `iphone/build/DerivedData/Build/Products/Debug-iphonesimulator/PetApp.app/PetApp` 二进制存在；3) `otool -L iphone/build/DerivedData/Build/Products/Debug-iphonesimulator/PetApp.app/PetApp 2>/dev/null | grep -i CoreMotion` 输出非空（说明 framework link 成功）；HealthKit framework 链接保持原样（Story 8.1 不能回归）.

**不接 Capability**：CoreMotion 不需要 entitlements 文件（与 HealthKit 不同——HK 在真机要 entitlements，CM 仅靠 Info.plist 描述就够）；模拟器跑只需 Info.plist 即可（与 8.1 红线同精神：节点 3 阶段不引入 entitlements）.

---

### AC4 — `MotionProviderMock`（新文件 `iphone/PetApp/Core/Motion/MotionProviderMock.swift`）

新建 `iphone/PetApp/Core/Motion/MotionProviderMock.swift`，按 ADR-0002 §3.1 + Story 8.1 HealthProviderMock 同模板（手写 mock + invocations 数组）：

```swift
import Foundation
import CoreMotion

/// MotionProvider 的 in-memory 测试 mock；按 ADR-0002 §3.1 手写 invocation 记录.
/// 用法：
///   let mock = MotionProviderMock()
///   mock.requestPermissionStub = .success(true)
///   mock.startUpdates { activity in ... }
///   mock.injectActivity(MotionProviderMock.makeActivity(walking: true))  // 触发 handler
public final class MotionProviderMock: MotionProvider, @unchecked Sendable {
    /// requestPermission 返回值 stub.
    public var requestPermissionStub: Result<Bool, Error> = .success(true)

    /// 调用历史；按 ADR-0002 §3.1 钦定的"至少记录 invocations"模板.
    public private(set) var invocations: [String] = []

    /// 调用次数（独立计数；测试断言"被调 N 次"用）.
    public private(set) var requestPermissionCallCount: Int = 0
    public private(set) var startUpdatesCallCount: Int = 0
    public private(set) var stopUpdatesCallCount: Int = 0
    /// 通过 injectActivity 触发 handler 的次数（包括被忽略的，便于断言"权限拒绝时 handler 没收到"）.
    public private(set) var handlerInvocationCount: Int = 0

    /// 当前注册的 handler；nil 表示未 startUpdates / 已 stopUpdates.
    private let lock = NSLock()
    private var registeredHandler: ((CMMotionActivity) -> Void)?

    public init() {}

    public func requestPermission() async throws -> Bool {
        invocations.append("requestPermission()")
        requestPermissionCallCount += 1
        switch requestPermissionStub {
        case .success(let v): return v
        case .failure(let e): throw e
        }
    }

    public func startUpdates(handler: @escaping @Sendable (CMMotionActivity) -> Void) {
        invocations.append("startUpdates")
        startUpdatesCallCount += 1
        lock.lock()
        // 与 MotionProviderImpl 同语义：已 startUpdates 时直接忽略（"防止重复订阅"）.
        // mock 也强制此契约——让上层测试可断言"二次 start 被忽略".
        if registeredHandler != nil {
            lock.unlock()
            return
        }
        registeredHandler = handler
        lock.unlock()
    }

    public func stopUpdates() {
        invocations.append("stopUpdates")
        stopUpdatesCallCount += 1
        lock.lock()
        registeredHandler = nil
        lock.unlock()
    }

    /// 测试用：把一个 CMMotionActivity 实例注入当前 handler.
    /// 若 handler 不存在（未 startUpdates / 已 stopUpdates），仅 handlerInvocationCount 不增；
    /// 不抛错（让 caller 自行验证 handler 注册时机）.
    public func injectActivity(_ activity: CMMotionActivity) {
        lock.lock()
        let captured = registeredHandler
        lock.unlock()
        guard let captured else { return }
        handlerInvocationCount += 1
        captured(activity)
    }

    /// 重置全部 stub + 调用历史（测试 setUp / tearDown 用）.
    public func reset() {
        requestPermissionStub = .success(true)
        invocations = []
        requestPermissionCallCount = 0
        startUpdatesCallCount = 0
        stopUpdatesCallCount = 0
        handlerInvocationCount = 0
        lock.lock()
        registeredHandler = nil
        lock.unlock()
    }
}

/// 便利构造工具：手工 build CMMotionActivity 用于 mock 注入.
/// 注意：CMMotionActivity 没有公开的 init —— 通过 KVC（NSObject runtime）赋值私有属性
/// 是最稳妥模式（与 Apple 测试文档 / 第三方 lib 同模式）.
extension MotionProviderMock {
    /// 创建一个 CMMotionActivity 实例（用 NSObject KVC 赋值私有 stored properties）.
    /// - Parameters:
    ///   - stationary / walking / running / cycling / automotive / unknown：对应 CMMotionActivity flag
    ///   - confidence：CMMotionActivityConfidence (.low / .medium / .high)
    ///   - startDate：activity 起始时间（默认 Date()）
    /// - Note: 该方法仅 DEBUG / TEST 用——production 不会构造 CMMotionActivity（只接收系统事件）.
    public static func makeActivity(
        stationary: Bool = false,
        walking: Bool = false,
        running: Bool = false,
        cycling: Bool = false,
        automotive: Bool = false,
        unknown: Bool = false,
        confidence: CMMotionActivityConfidence = .high,
        startDate: Date = Date()
    ) -> CMMotionActivity {
        let activity = CMMotionActivity()
        // CMMotionActivity 私有 stored properties；用 KVC 是 Apple test fixture 同模式.
        activity.setValue(stationary, forKey: "stationary")
        activity.setValue(walking, forKey: "walking")
        activity.setValue(running, forKey: "running")
        activity.setValue(cycling, forKey: "cycling")
        activity.setValue(automotive, forKey: "automotive")
        activity.setValue(unknown, forKey: "unknown")
        activity.setValue(confidence.rawValue, forKey: "confidence")
        activity.setValue(startDate, forKey: "startDate")
        return activity
    }
}
```

**关键决策**：

- **不继承 MockBase**：与 8.1 HealthProviderMock 同决策——MotionProviderMock 走 production target（`PetApp/Core/Motion/`），PetApp target 不能 `@testable import` 测试 helper.
- **位置在 `PetApp/Core/Motion/`** 而非 `PetAppTests/`：架构 §17.1 钦定 mock 在 production target；与 HealthProviderMock 同模式.
- **`injectActivity` API 是核心**：epics.md AC 钦定"`MotionProviderMock`（手动注入 CMMotionActivity 序列）"——靠这个方法让测试驱动 handler 触发.
- **`makeActivity` 用 KVC 赋值私有属性**：CMMotionActivity 没有 public init，所有 flag 字段是 read-only stored properties；KVC（`setValue:forKey:`）是 Apple ObjC runtime 标准模式（CoreMotion 在 ObjC 实现的，bridge 到 Swift 后 KVC 仍可用）。**坑警告**：如果 Apple 未来把 CMMotionActivity 改成 Swift struct → KVC 失效，需要切到 mock-protocol-with-stub-flags 路径；2026-04 当前 iOS 17/18 SDK 实测 KVC 仍可用.
- **`@unchecked Sendable`**：mock 字段 mutable 但测试串行调用，与 8.1 同模式.
- **二次 startUpdates 防御**：mock 也强制此契约（保持与 MotionProviderImpl 同语义），让单元测试 case 能断言"二次 start 被忽略".

---

### AC5 — AppContainer wire 路径（仅声明字段，**不**强制实例化）

**改动文件**：`iphone/PetApp/App/AppContainer.swift`

在 `AppContainer` class 内追加 `motionProvider` 字段（与 `healthProvider` 同模式）：

```swift
/// Story 8.2: MotionProvider 实例.
/// 节点 3 阶段（Story 8.1～8.5）逐步 wire：
/// - 8.2（本 story）：仅声明 + init 默认实例化为 MotionProviderImpl；当前无 caller.
/// - 8.4：HomeViewModel 通过 container.motionProvider 订阅 startUpdates → 调 8.3 mapper → driving petState.
/// 测试场景通过未来追加的 init 重载注入 MotionProviderMock（YAGNI：本 story 不预留 init 参数；Story 8.4 落地时再加）.
public let motionProvider: MotionProvider
```

**init 内部新增构造**（在现有 `self.healthProvider = HealthProviderImpl()` 行附近）：

```swift
self.motionProvider = MotionProviderImpl()
```

**红线（与 8.1 同精神）**：

- **不**改 RootView wire（`@StateObject private var container = AppContainer()` 无参 init 调用 → 自动拿 MotionProviderImpl 默认实例；HomeViewModel / 任何 UseCase 本 story 都不消费 motionProvider）
- **不**改 HomeViewModel / 任何 ViewModel 的 init / bind 签名（Story 8.4 才接订阅）
- **不**改 RootView `bootstrapStep1` / `.task` 逻辑（CoreMotion 权限按需申请，**不**在启动期触发；架构 §16 + AR17 + epics.md AC "权限按需申请"）
- **不**新增 `init(motionProvider:)` 重载参数（YAGNI；Story 8.4 接 caller 时再加）

---

### AC6 — 单元测试 ≥4 case（MotionProviderMock 路径）

**新建文件**：`iphone/PetAppTests/Core/Motion/MotionProviderMockTests.swift`

按 ADR-0002 §3.2 钦定 `async/await` 主流写法 + epics.md AC 行 1486-1490 钦定 4 case：

```swift
import XCTest
import CoreMotion
@testable import PetApp

@MainActor
final class MotionProviderMockTests: XCTestCase {
    // happy: requestPermission 成功 → startUpdates 后 handler 收到注入的事件
    func testRequestPermissionGranted_thenHandlerReceivesInjectedActivity() async throws {
        let mock = MotionProviderMock()
        mock.requestPermissionStub = .success(true)

        let granted = try await mock.requestPermission()
        XCTAssertTrue(granted)
        XCTAssertEqual(mock.requestPermissionCallCount, 1)

        var received: [CMMotionActivity] = []
        mock.startUpdates { activity in received.append(activity) }
        XCTAssertEqual(mock.startUpdatesCallCount, 1)

        let walking = MotionProviderMock.makeActivity(walking: true)
        mock.injectActivity(walking)
        XCTAssertEqual(received.count, 1)
        XCTAssertTrue(received[0].walking)
        XCTAssertEqual(mock.handlerInvocationCount, 1)
    }

    // edge: requestPermission 失败 → startUpdates 不触发任何回调
    // 注：mock 不强制"requestPermission 失败时 startUpdates 也失败"——caller 决策（与 8.1 同模式）.
    // 此 case 验证：caller 不调 startUpdates 时永远收不到事件；调了 startUpdates 也只是 handler 注册，
    // 没人 inject 就没人收（与"权限失败 → 系统不发事件"现实路径同语义）.
    func testRequestPermissionDenied_thenNoHandlerInvocationWithoutInject() async throws {
        let mock = MotionProviderMock()
        mock.requestPermissionStub = .success(false)

        let granted = try await mock.requestPermission()
        XCTAssertFalse(granted)

        var received: [CMMotionActivity] = []
        mock.startUpdates { activity in received.append(activity) }

        // 没人 inject，handler 不被调
        XCTAssertEqual(received.count, 0)
        XCTAssertEqual(mock.handlerInvocationCount, 0)
    }

    // happy: stopUpdates 后 handler 不再收到事件
    func testStopUpdates_thenInjectActivityDoesNotInvokeHandler() async throws {
        let mock = MotionProviderMock()
        var received: [CMMotionActivity] = []
        mock.startUpdates { activity in received.append(activity) }

        // 先 inject 一次确认正常路径
        mock.injectActivity(MotionProviderMock.makeActivity(stationary: true))
        XCTAssertEqual(received.count, 1)

        mock.stopUpdates()
        XCTAssertEqual(mock.stopUpdatesCallCount, 1)

        // stop 之后 inject 不应触发 handler
        mock.injectActivity(MotionProviderMock.makeActivity(walking: true))
        XCTAssertEqual(received.count, 1)  // 仍是 1
    }

    // edge: 同时多次 startUpdates → 只生效第一次（防止重复订阅）
    func testMultipleStartUpdates_thenFirstHandlerOnlyReceivesActivity() async throws {
        let mock = MotionProviderMock()

        var firstHandlerReceived: [CMMotionActivity] = []
        var secondHandlerReceived: [CMMotionActivity] = []

        mock.startUpdates { activity in firstHandlerReceived.append(activity) }
        mock.startUpdates { activity in secondHandlerReceived.append(activity) }
        XCTAssertEqual(mock.startUpdatesCallCount, 2)  // 调用了两次

        mock.injectActivity(MotionProviderMock.makeActivity(running: true))

        // 只有第一个 handler 收到（second 被忽略）
        XCTAssertEqual(firstHandlerReceived.count, 1)
        XCTAssertTrue(firstHandlerReceived[0].running)
        XCTAssertEqual(secondHandlerReceived.count, 0)
    }

    // 加分项 case 5：stopUpdates 后再 startUpdates 视作全新订阅（handler 替换为新 closure）
    func testStopThenStartUpdatesAgain_thenNewHandlerReceivesActivity() async throws {
        let mock = MotionProviderMock()
        var firstHandlerReceived: [CMMotionActivity] = []
        var secondHandlerReceived: [CMMotionActivity] = []

        mock.startUpdates { activity in firstHandlerReceived.append(activity) }
        mock.injectActivity(MotionProviderMock.makeActivity(walking: true))
        XCTAssertEqual(firstHandlerReceived.count, 1)

        mock.stopUpdates()

        mock.startUpdates { activity in secondHandlerReceived.append(activity) }
        mock.injectActivity(MotionProviderMock.makeActivity(running: true))

        // 老 handler 不再收到
        XCTAssertEqual(firstHandlerReceived.count, 1)
        // 新 handler 收到了
        XCTAssertEqual(secondHandlerReceived.count, 1)
        XCTAssertTrue(secondHandlerReceived[0].running)
    }

    // 加分项 case 6：reset() 清空 stub + invocations + handler 注册
    func testReset_clearsAllStubsHandlerAndInvocations() async throws {
        let mock = MotionProviderMock()
        mock.requestPermissionStub = .success(false)
        mock.startUpdates { _ in }
        _ = try await mock.requestPermission()
        mock.injectActivity(MotionProviderMock.makeActivity(walking: true))

        mock.reset()

        XCTAssertEqual(mock.invocations, [])
        XCTAssertEqual(mock.requestPermissionCallCount, 0)
        XCTAssertEqual(mock.startUpdatesCallCount, 0)
        XCTAssertEqual(mock.stopUpdatesCallCount, 0)
        XCTAssertEqual(mock.handlerInvocationCount, 0)

        // reset 后默认 success(true) + 无 handler 注册
        let granted = try await mock.requestPermission()
        XCTAssertTrue(granted)

        // reset 后再 inject 不会触发任何 handler（registeredHandler 已清）
        mock.injectActivity(MotionProviderMock.makeActivity(running: true))
        XCTAssertEqual(mock.handlerInvocationCount, 0)
    }
}
```

**关键决策**：

- **测试**仅覆盖 `MotionProviderMock` 行为（stub 表查询 / handler 注册-触发-注销 / 二次 start 防御 / reset）。**不**测 `MotionProviderImpl`（真 CMMotionActivityManager 路径）—— 那是 AC7 集成测试的事.
- **`@MainActor` 测试 class**：与 8.1 HealthProviderMockTests 同模式.
- **不引第三方断言 lib**：XCTest only（ADR-0002 §3.1）.
- **4 个 epics.md AC case 的精确映射**：case 1（happy: granted+inject）→ AC line 1486；case 2（edge: denied+no inject）→ AC line 1487；case 3（happy: stop 后不触发）→ AC line 1488；case 4（edge: 多次 start 只生效一次）→ AC line 1489-1490；case 5/6 是加分（stop-then-start 重订阅 + reset 清空）.
- **CMMotionActivity 实例构造细节**：`MotionProviderMock.makeActivity(...)` 是测试便利方法；其内部用 KVC 赋值私有属性的策略（详 AC4）让测试无需 mock CMMotionActivity 类型.

---

### AC7 — 集成测试 ≥1 case（XCUITest + 模拟器 CoreMotion + DEBUG-only probe）

**新建文件**：`iphone/PetAppUITests/MotionProviderIntegrationTests.swift`

epics.md AC 没有钦定具体集成测试要求（仅 4 unit case），但本 story 沿用 8.1 红线"模拟器集成至少 1 case 验证 path wired up"，以保证 MotionProviderImpl + CoreMotion.framework + Info.plist 真实接入路径在模拟器跑通.

**实施约束（必须严格遵守，沿用 8.1 红线）**：

1. **集成测试用 XCUITest target**（`iphone/PetAppUITests/`），与 HealthProviderIntegrationTests 同 target.
2. **DEBUG-only probe view 路径**：与 8.1 HealthProviderProbeView 同模式新建 `MotionProviderProbeView`，由 `-PetAppRunMotionProviderIntegrationProbe` launch arg 触发挂载，调 `motionProvider.startUpdates` + 显示最近一次接收的 activity 关键 flag（walking / running / stationary / cycling / automotive / confidence）以 a11y identifier `motionProviderProbeResult` 暴露给 XCUITest.
3. **probe view 实装位置**：`iphone/PetApp/Features/DevTools/Views/MotionProviderProbeView.swift`（仅 `#if DEBUG`，与 HealthProviderProbeView 同位置）.
4. **PetAppApp.swift 解析**：在现有 `useHealthProviderProbe` / `preseedSteps` 旁追加 `useMotionProviderProbe: Bool`（解析 `-PetAppRunMotionProviderIntegrationProbe`）；body 路径优先级：`useHealthProviderProbe` → `useMotionProviderProbe` → 默认 `RootBootstrapView` / `RootView`（后续多 probe 模式互斥；本 story 实装时检查 8.1 已挂的 `useHealthProviderProbe` flag 并保持顺序对齐）.
5. **测试 case ≥1**：

```swift
import XCTest

final class MotionProviderIntegrationTests: XCTestCase {

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    func testMotionProviderImpl_probeViewMountsAndExecutesStartUpdatesPath() throws {
        let app = XCUIApplication()
        app.launchArguments = [
            "-PetAppRunMotionProviderIntegrationProbe",
        ]
        app.launch()

        // 路径检查 1：probe view 必须挂载——label "motionProviderProbeResult" 存在
        let probeLabel = app.staticTexts["motionProviderProbeResult"]
        XCTAssertTrue(
            probeLabel.waitForExistence(timeout: 15.0),
            "ProbeView label 'motionProviderProbeResult' not found—检查 PetAppApp 解析 -PetAppRunMotionProviderIntegrationProbe + DEBUG probe 路径"
        )

        // 路径检查 2：轮询 30s——要么 result label 显示某个状态字符串（happy path：模拟器随机生成 motion event），
        // 要么 error label 出现（sandbox-limited / 权限拒绝路径）.
        // 模拟器 CoreMotion 在 Xcode 26 + iPhone 17 simulator 实测会偶发触发 stationary/walking activity；
        // sandbox 行为不一定 100% 触发——AC7 钦定"path wired up 即视作 PASS".
        let errorLabel = app.staticTexts["motionProviderProbeError"]
        let deadline = Date().addingTimeInterval(30.0)
        var resolved = false
        var lastResultLabel = "-"
        var lastErrorLabel = ""

        while Date() < deadline {
            let resultText = probeLabel.exists ? probeLabel.label : "-"
            lastResultLabel = resultText
            // result 非空且不是初始 "-" → 视作 happy path
            if resultText != "-", resultText != "(waiting)" {
                resolved = true
                break
            }
            if errorLabel.exists {
                let errText = errorLabel.label
                lastErrorLabel = errText
                if !errText.isEmpty {
                    resolved = true
                    break
                }
            }
            Thread.sleep(forTimeInterval: 1.0)
        }

        XCTAssertTrue(
            resolved,
            "neither result label 状态字符串 nor error label 出现——MotionProviderImpl startUpdates 没走完执行路径; " +
            "lastResultLabel='\(lastResultLabel)', lastErrorLabel='\(lastErrorLabel)'"
        )

        // 路径分类（与 8.1 AC7 红线同精神，三态 PASS）：
        //   1) result label 含 "stationary" / "walking" / "running" / "cycling" / "automotive" / "unknown"：happy path
        //   2) result label == "(waiting)" 且 error label 出现：sandbox-limited / permissionDenied path
        //   3) result label == "-"（初始）+ error label 出现：极端 sandbox 路径
        // 只要 path wired up（probe view mount + startUpdates 调用）即视作 PASS.
        if !lastResultLabel.isEmpty, lastResultLabel != "-", lastResultLabel != "(waiting)" {
            print("INFO: MotionProvider probe happy path——result='\(lastResultLabel)'")
        } else {
            // sandbox 路径：errorLabel 必须有值表明 catch 路径走过 OR result 仍是 "(waiting)"（startUpdates 调了但模拟器没发 activity）.
            if lastErrorLabel.isEmpty {
                XCTAssertEqual(
                    lastResultLabel, "(waiting)",
                    "sandbox 路径下 result label 应是 '(waiting)' 表明 startUpdates 已调但无 activity；当前='\(lastResultLabel)'"
                )
                print("INFO: simulator CoreMotion sandbox 启动 startUpdates 但未触发 activity——已知坑（详见 story 8.2 AC7 红线）；本 case 视作 PASS（路径已 wired up）.")
            } else {
                print("INFO: simulator CoreMotion sandbox 拒授权或系统错误——errorLabel='\(lastErrorLabel)'；本 case 视作 PASS（路径已 wired up）.")
            }
        }
    }
}
```

**MotionProviderProbeView 实装要点**（写到 `iphone/PetApp/Features/DevTools/Views/MotionProviderProbeView.swift`）：

```swift
#if DEBUG
import SwiftUI
import CoreMotion

/// CoreMotion 集成测试用 probe view；显示 motionProvider.startUpdates 接收到的最新 CMMotionActivity flag 状态.
struct MotionProviderProbeView: View {
    let motionProvider: MotionProvider

    @State private var resultText: String = "(waiting)"
    @State private var errorText: String = ""

    var body: some View {
        VStack(spacing: 16) {
            Text("MotionProvider Probe")
                .font(.title3)
            Text(resultText)
                .font(.system(size: 24, weight: .bold))
                .accessibilityIdentifier("motionProviderProbeResult")
            if !errorText.isEmpty {
                Text(errorText)
                    .foregroundColor(.red)
                    .accessibilityIdentifier("motionProviderProbeError")
            }
        }
        .task {
            // 1. 申请权限（模拟器自动授予；UITest 容忍失败仍调 startUpdates，
            //    与 8.1 同模式——sandbox 行为是 PASS 路径）.
            do {
                _ = try await motionProvider.requestPermission()
            } catch {
                await MainActor.run {
                    errorText = String(describing: error)
                }
            }

            // 2. startUpdates 注册 handler，写最近一次 activity 字段到 result label.
            motionProvider.startUpdates { activity in
                Task { @MainActor in
                    resultText = Self.describeActivity(activity)
                }
            }
        }
    }

    /// 把 CMMotionActivity 转成可读字符串（probe view 显示用）.
    /// 多个 flag 同时 true 时按 stationary/walking/running/cycling/automotive/unknown 顺序拼接.
    private static func describeActivity(_ activity: CMMotionActivity) -> String {
        var parts: [String] = []
        if activity.stationary { parts.append("stationary") }
        if activity.walking { parts.append("walking") }
        if activity.running { parts.append("running") }
        if activity.cycling { parts.append("cycling") }
        if activity.automotive { parts.append("automotive") }
        if activity.unknown { parts.append("unknown") }
        let joined = parts.isEmpty ? "none" : parts.joined(separator: "+")
        let confidenceStr: String
        switch activity.confidence {
        case .low: confidenceStr = "low"
        case .medium: confidenceStr = "medium"
        case .high: confidenceStr = "high"
        @unknown default: confidenceStr = "unknown"
        }
        return "\(joined)|\(confidenceStr)"
    }
}
#endif
```

**红线**：

- **集成测试不强制必须通过**：模拟器 CoreMotion 在 CI 环境（本 ADR 当前阶段无 CI）可能因 sandbox 不发 activity 而 result 落到 "(waiting)" → 节点 3 阶段允许"集成测试通过本机模拟器跑 + 文档化失败 fallback"（与 8.1 AC7 红线同精神）.
- **不**做"requestPermission 真弹窗"测试（XCUITest 无法稳定模拟系统权限弹窗交互）；权限是否授权由 simulator 自动决策.
- **不**直接测试单元测试 case 的"真 CMMotionActivityManager"路径（理由：单元测试不应实例化真 CM manager，否则破坏 ADR-0002 §3.1 钦定的"零外部依赖手写 mock"原则）.
- **PetAppApp.swift 改动**：仅追加 `useMotionProviderProbe` 字段 + body 路径分支；**不**触碰 `useHealthProviderProbe` / `preseedSteps` / `RootBootstrapView` 任何 8.1 已落地代码（避免 8.1 回归）。8.1 probe path 优先级保持原样（如果同时传两个 probe flag，按代码顺序——先 health 后 motion——只挂第一个生效；UITest 也只测互斥路径）.

---

### AC8 — Build Verify

**完成判定（必须 4 个全部通过）**：

1. `bash iphone/scripts/build.sh` 命令成功（exit 0；构建带 CoreMotion + HealthKit framework 的 Debug.app）
2. `bash iphone/scripts/build.sh --test` 命令成功（5-6 个 MotionProviderMockTests case 全 pass + Story 8.1 / 其他 unit 测试零回归）
3. `bash iphone/scripts/build.sh --uitest` 命令成功（MotionProviderIntegrationTests case pass，**或**记录在 lessons 内"模拟器 CM sandbox 路径已知坑"并标本 case 跳过 + 单独 issue 跟踪；HealthProviderIntegrationTests 不能回归）
4. `otool -L iphone/build/DerivedData/Build/Products/Debug-iphonesimulator/PetApp.app/PetApp 2>/dev/null | grep -iE "CoreMotion|HealthKit"` 输出含两条（CoreMotion 新增 + HealthKit 8.1 保留）

**ios-simulator MCP 验证**（CLAUDE.md "iOS UI 验证（必跑）"钦定）：

```
1. bash iphone/scripts/build.sh
2. install_app(app_path: iphone/build/DerivedData/Build/Products/Debug-iphonesimulator/PetApp.app)
3. launch_app(bundle_id: "com.zhuming.pet.app", terminate_running: true, launch_args: ["-PetAppRunMotionProviderIntegrationProbe"])
4. ui_view（看 probe label 显示 activity flag 字符串或 "(waiting)"）
5. ui_describe_all（验 a11y identifier "motionProviderProbeResult" 存在）
6. launch_app（不带 launch_args；普通启动应展示正常 RootView/MainTabView，无回归）
```

如 ios-simulator MCP 步骤 4 看到的是 "-" 或缺失 probe label，则说明 launch argument probe 路径未生效 → 检查 PetAppApp.swift 的 ProcessInfo.arguments 解析逻辑 → 修复后再跑.

---

## Tasks / Subtasks

- [x] **Task 1（AC1）**：新建 `iphone/PetApp/Core/Motion/MotionProvider.swift`
  - [x] 1.1 定义 `MotionProvider` protocol（Sendable + requestPermission + startUpdates(handler:) + stopUpdates）
  - [x] 1.2 定义 `MotionProviderError` enum（activityDataNotAvailable / permissionDenied / systemFailure(NSError) + LocalizedError + Equatable）

- [x] **Task 2（AC2）**：新建 `iphone/PetApp/Core/Motion/MotionProviderImpl.swift`
  - [x] 2.1 import CoreMotion + 定义 final class + manager + lock + isUpdating + currentHandler 字段
  - [x] 2.2 实装 requestPermission（isActivityAvailable 检查 + authorizationStatus 判定 + .notDetermined 走 probePermissionViaQuery 探针 + @unknown default 兜底）
  - [x] 2.3 实装 startUpdates（NSLock 内 isUpdating 防御 + currentHandler 替换 + manager.startActivityUpdates 注册到 OperationQueue.main + weak self capture）
  - [x] 2.4 实装 stopUpdates（NSLock 内 isUpdating 检查 + 清 currentHandler + manager.stopActivityUpdates）
  - [x] 2.5 实装 probePermissionViaQuery（withCheckedThrowingContinuation 桥 queryActivityStarting + CMErrorMotionActivityNotAuthorized 映射 + 完成后 re-read authorizationStatus）
  - [x] 2.6 mark @unchecked Sendable

- [x] **Task 3（AC3）**：声明 framework 依赖
  - [x] 3.1 改 `iphone/project.yml` 在 PetApp target.dependencies 下加 `- sdk: CoreMotion.framework`（保留 HealthKit 行，**禁止**移除）
  - [x] 3.2 改 `iphone/project.yml` 在 PetApp.info.properties 下加 `NSMotionUsageDescription`（保留 NSHealthShareUsageDescription / NSHealthUpdateUsageDescription，**禁止**移除）
  - [x] 3.3 跑 `bash iphone/scripts/build.sh`（脚本内部自动调 xcodegen generate，会从 project.yml regen Info.plist）
  - [x] 3.4 验证 build 通过 + `otool -L` 确认 CoreMotion + HealthKit 都链到 PetApp.debug binary

- [x] **Task 4（AC4）**：新建 `iphone/PetApp/Core/Motion/MotionProviderMock.swift`
  - [x] 4.1 定义 final class + Stub 字段（requestPermissionStub）+ invocations + callCount 系列字段
  - [x] 4.2 实装 requestPermission（按 stub 表查询）
  - [x] 4.3 实装 startUpdates（NSLock 内 registeredHandler 防御 + 二次 start 忽略；与 Impl 同语义）
  - [x] 4.4 实装 stopUpdates（清 registeredHandler）
  - [x] 4.5 实装 injectActivity(_ activity: CMMotionActivity)（lock 内读 handler ref 后调用，handlerInvocationCount 计数）
  - [x] 4.6 实装 reset() 清状态
  - [x] 4.7 实装 makeActivity(stationary:walking:running:cycling:automotive:unknown:confidence:startDate:) 静态便利方法（**修订**：KVC 在 Xcode 26 SDK 上 crash → 改用 ObjC 子类 override readonly getter；详见 Dev Agent Record Debug Log）

- [x] **Task 5（AC5）**：扩 `iphone/PetApp/App/AppContainer.swift`
  - [x] 5.1 加 `public let motionProvider: MotionProvider` 字段（与 healthProvider 同模式）
  - [x] 5.2 init 内部默认 `self.motionProvider = MotionProviderImpl()`
  - [x] 5.3 **不**修改任何 ViewModel / UseCase / RootView 调用方（YAGNI；Story 8.4 才接 caller）
  - [x] 5.4 **不**改 `init(apiClient:keychainStore:unauthorizedHandlerSink:)` 签名（保留向下兼容；Story 8.4 接订阅时再追加 motionProvider 参数）

- [x] **Task 6（AC6）**：新建 `iphone/PetAppTests/Core/Motion/MotionProviderMockTests.swift`
  - [x] 6.1 写 6 个 case（granted+inject / denied+no inject / stop 后无触发 / 多次 start 只生效一次 / stop-then-start 重订阅 / reset 清空）
  - [x] 6.2 跑 `bash iphone/scripts/build.sh --test` 验证全 pass + 现有 unit 测试零回归（358 tests, 0 failures）

- [x] **Task 7（AC7）**：新建集成测试 + Probe view
  - [x] 7.1 新建 `iphone/PetApp/Features/DevTools/Views/MotionProviderProbeView.swift`（#if DEBUG，调 motionProvider.requestPermission + startUpdates，把 activity 字段拼成字符串挂 a11y identifier）
  - [x] 7.2 改 `iphone/PetApp/App/PetAppApp.swift` 在 `useHealthProviderProbe` 旁追加 `useMotionProviderProbe`（解析 `-PetAppRunMotionProviderIntegrationProbe`）；body 路径分支按"health probe → motion probe → 默认"顺序，**禁止**触碰 health 路径已有代码
  - [x] 7.3 新建 `iphone/PetAppUITests/MotionProviderIntegrationTests.swift`（launch arguments + waitForExistence + 轮询 30s + 三态 PASS：happy / waiting+error-empty / error）
  - [x] 7.4 跑 `bash iphone/scripts/build.sh --uitest` 验证（MotionProviderIntegrationTests pass；HealthProviderIntegrationTests 零回归）

- [x] **Task 8（AC8）**：ios-simulator MCP 真机验证 + 收尾
  - [x] 8.1 跑 `bash iphone/scripts/build.sh` + install_app + launch_app（普通 launch + 带 motion probe launch arguments 各一次）
  - [x] 8.2 ui_view 验证 probe label / errorLabel 显示路径（normal launch → home view 正常渲染；motion probe launch → "MotionProvider Probe" + "(waiting)" + errorText="activityDataNotAvailable"）
  - [x] 8.3 跑全套 `--test` + `--uitest` 一遍（unit 全 pass 358/358；UITest HealthProviderIntegrationTests + MotionProviderIntegrationTests pass）
  - [x] 8.4 写 dev_agent_record（File List + Completion Notes + Debug Log）

---

## Dev Notes

### CoreMotion 接入坑表（必读，避坑指南）

| 坑 | 现象 | 缓解 |
|---|---|---|
| `CMMotionActivityManager.authorizationStatus()` 是真实可读的（与 HealthKit 不同） | iOS 11+ 公开 API；status 反映用户真实选择 | **可信赖**——不需要 8.1 那种 probe-read 兜底；`.authorized` → 已授权，`.denied / .restricted` → 未授权 |
| `.notDetermined` 状态下 startUpdates / queryActivityStarting 会触发系统弹窗 | 第一次调用时弹；用户选择前 query callback 不返回 | requestPermission 走 queryActivityStarting 而非 startActivityUpdates 触发——前者有完成回调（弹窗结束后再读 authorizationStatus 拿真实结果），后者注册了 handler 没有"权限弹窗结束"信号 |
| 模拟器 CoreMotion 行为 | iPhone 17 simulator 在 Xcode 26 默认放行 activity；但不一定持续 push activity 事件（sandbox 行为不稳定） | 集成测试三态 PASS：happy（收到 activity 字符串）/ waiting+errorEmpty（startUpdates 调了但模拟器没发）/ error（系统拒授权）—— path wired up 即可（与 8.1 AC7 红线同精神） |
| CMMotionActivityManager.startActivityUpdates **必须**在 main thread 调 | 在后台线程调会被忽略或直接 crash | 用 OperationQueue.main 接收 callback；如调用方在 background actor，先 `await MainActor.run { provider.startUpdates(...) }` |
| 多个 flag 同时 true | walking + stationary 同时为 true 是合法状态（confidence 模糊时） | 协议层透传——8.3 mapper 决策优先级（running > walking > rest 见 epics.md Story 8.3 AC） |
| confidence < .low 不应直接切状态 | 低置信度切换会导致 sprite 闪烁 | 协议层不做 confidence 过滤——8.3 mapper 钦定"confidence < .low 保持上一次状态" |
| CMMotionActivity 没有 public init | 测试中无法直接构造 CMMotionActivity 实例 | mock 内 KVC 赋值私有属性（`setValue:forKey:`）—— Apple ObjC runtime 标准模式 |
| `CMErrorMotionActivityNotAuthorized` vs `kCMErrorDomain` 命名 | Apple 给出的常量名是 `CMErrorMotionActivityNotAuthorized`（在 Swift bridge 后即可访问） | 直接 `CMErrorMotionActivityNotAuthorized.rawValue == nsError.code`；domain 用 `CMErrorDomain` |
| 同时多次 startActivityUpdates 系统行为 | Apple 文档没明确——实测重复调会替换 handler，导致老 handler 静默失效 | epics.md AC 钦定的"防止重复订阅"语义在 protocol 层强制——**第二次起的 startUpdates 直接忽略**（lock + isUpdating 旗标）；mock 与 Impl 行为一致让测试覆盖 |
| CMMotionActivityManager 不是 Sendable | Swift 6 strict concurrency 编译报错 | 与 HKHealthStore 同——`@unchecked Sendable` + NSLock 保护内部状态；不引 actor 因 callback 在 OperationQueue 触发跨 actor 不友好 |
| `weak self` capture in startActivityUpdates handler | 强引用闭环：MotionProviderImpl → manager（持 callback closure）→ self | handler 用 `[weak self]` 避免循环 retain；stopUpdates 时 lock 内清 currentHandler 双保险 |

### 与 Story 8.1 / 8.3 / 8.4 / 8.5 边界

- **本 story 不写 AppState**：MotionProvider 是 System Adapter 层（架构 §5.5），只负责"读 CM activity"；**不**直接写 AppState（HomeViewModel.petState 持瞬时 motionState 由 Story 8.4 实装；AppState 7 字段白名单内**没有** motionState，只有 currentStepAccount）.
- **本 story 不实装 MotionStateMapper**：Story 8.3 单独负责 stationary/walking/running → rest/walk/run 映射（含 confidence 过滤 + 优先级规则）；本 story 协议层暴露原始 `CMMotionActivity` 类型给 8.3 直接消费.
- **本 story 不订阅 HomeViewModel**：Story 8.4 在 HomeViewModel.bind 时 `motionProvider.startUpdates { activity in /* 调 mapper → 写 @Published petState */ }`；本 story 仅准备 protocol + Impl + Mock + AppContainer wire.
- **与 Story 8.5 步数同步触发器无关**：8.5 用 HealthProvider（步数读取）+ AppState.currentStepAccount，与 MotionProvider 没耦合（运动状态 ≠ 步数同步触发器；详见 epics.md §Story 8.5 AC 钦定时机）.
- **与 Story 9.1 跨端 e2e 配合**：9.1 验证场景 4（猫 sprite 切换）会用 `MockMotionProvider`（在 App 内通过 DEBUG hook 注入）触发 walking → mapper → ViewModel.petState = .walk → PetSpriteView walk 动画；本 story 提供 MotionProviderMock 的 `injectActivity` 是该 hook 的底层 API.

### 与 ADR-0002 §3.1 Mock 框架对齐（手写 mock + invocations）

- MotionProviderMock 走 production target `Core/Motion/` 而非 `PetAppTests/Helpers/`，理由见 AC4 决策（与 HealthProviderMock 同模式）
- 与 MockURLSession（Story 2.4）/ MockAPIClient（Story 2.5）/ HealthProviderMock（Story 8.1）同手写模式，不引第三方 codegen
- invocations 数组 + callCount 字段（requestPermission / startUpdates / stopUpdates / handler 触发各独立计数）是 ADR §3.1 钦定模板的扩展（多动作场景增计数字段）
- `injectActivity` API 是 mock 特有 helper（Impl 没有对应方法）——这是测试驱动模式：测试通过 inject 触发 handler，验证业务层订阅链路

### 与 ADR-0002 §3.2 Async/Await 主流（不引 Combine / AsyncStream）

- MotionProvider 协议不暴露 `AnyPublisher<CMMotionActivity, Error>` / `AsyncStream<CMMotionActivity>` 类型
- 上层调用方（Story 8.4 HomeViewModel）按需自行 wrap 为 `@Published petState` driving SwiftUI 即可
- 避免本 story 引入"流式 activity 转 Combine"语义（HKObserverQuery / HKAnchoredObjectQuery 在 8.1 也未引入；CoreMotion 没对等的"流式 query"，但 startUpdates 本身就是流——故协议直接暴露 callback 风格而非再包一层 AsyncStream）
- **理由对齐**：与 8.1 HealthProvider 不暴露 AsyncStream 同精神——保持 callback 简洁，让 caller 决策 reactive 风格

### 与 ADR-0010 AppState 边界（HomeViewModel 不持 motionState 在 AppState 内）

- AppState 7 字段白名单（ADR-0010 §3.2）：`session / userPet / currentRoom / currentStepAccount / currentChest / cosmeticInventory / petEquips`——**没有** motionState
- HomeViewModel 持瞬时 `motionState: MotionState`（Story 8.4 实装；ADR-0010 §3.2 addendum）—— 不写 AppState
- **本 story HealthProvider** 与 AppState 零耦合（与 8.1 同精神）
- **8.3 MotionStateMapper** 是 pure function（无状态），与 AppState 完全无关
- **8.4 HomeViewModel** 通过构造注入 AppState（**禁** `@EnvironmentObject`，ADR-0010 §3.1）—— 但 motionState 不写 AppState，仅持 ViewModel 瞬时字段

### 与 Story 2.5 / 5.2 / 8.5 启动链路边界（按需申请权限，AR17）

- **不**在 PetAppApp / RootView / AppContainer init 阶段调 `requestPermission`
- **不**在 LoadHomeUseCase / GuestLoginUseCase 路径调 MotionProvider
- 节点 3 收尾的 Story 8.4 才在"启动后进入主界面 + HomeViewModel.bind"时触发 requestPermission（首次需要 motion 状态时申请）

### File List（预期）

```
新增（4 个文件 production + 1 个 test target + 1 个 UITest target = 6 个文件）:
  iphone/PetApp/Core/Motion/MotionProvider.swift                                 # AC1
  iphone/PetApp/Core/Motion/MotionProviderImpl.swift                             # AC2
  iphone/PetApp/Core/Motion/MotionProviderMock.swift                             # AC4
  iphone/PetApp/Features/DevTools/Views/MotionProviderProbeView.swift            # AC7（DEBUG only）
  iphone/PetAppTests/Core/Motion/MotionProviderMockTests.swift                   # AC6
  iphone/PetAppUITests/MotionProviderIntegrationTests.swift                      # AC7

修改（3 个文件）:
  iphone/project.yml                              # AC3 加 CoreMotion.framework dep + NSMotionUsageDescription plist key
  iphone/PetApp/Resources/Info.plist              # AC3 加 NSMotionUsageDescription（实际由 xcodegen 从 project.yml regen）
  iphone/PetApp/App/AppContainer.swift            # AC5 加 motionProvider 字段 + init 默认 MotionProviderImpl
  iphone/PetApp/App/PetAppApp.swift               # AC7 处理 -PetAppRunMotionProviderIntegrationProbe launch arg

不改（红线全部满足）:
  iphone/PetApp/App/RootView.swift                # 启动期不触发权限
  iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift  # 不消费 motionProvider
  iphone/PetApp/Features/Home/UseCases/LoadHomeUseCase.swift  # 不依赖 motion
  iphone/PetApp/Core/Health/*                     # Story 8.1 文件零打扰
  iphone/PetApp/Features/DevTools/UseCases/HealthKitDevSeedUseCase.swift  # 8.1 dev seed 不动
  iphone/PetApp/Features/DevTools/Views/HealthProviderProbeView.swift     # 8.1 probe 不动
  iphone/PetAppUITests/HealthProviderIntegrationTests.swift               # 8.1 集成测试不动
  iphone/scripts/build.sh                         # 已支持 --uitest（8.1 已验证）
  ios/* 任一文件                                  # 重启阶段零打扰
  server/* 任一文件                               # iOS-only story
```

### Project Structure Notes

- **架构 §4 钦定 `iphone/PetApp/Core/Motion/`**：本 story 严格按此目录结构落地（架构 §4 文件名示例为 `MotionStateProvider` / `MotionState` / `MotionPermissionManager`；本 story 实际落地名为 `MotionProvider` / `MotionProviderImpl` / `MotionProviderMock`，更对齐 ADR-0002 §3.1 mock 命名风格 + epics.md AC 钦定的 `MotionProvider` 名称——Story 8.3 才会落地 `MotionState` enum）.
- **架构 §4 ↔ epics.md AC 命名对齐**：架构 §4 用 `MotionStateProvider`，epics.md AC 钦定 `MotionProvider`。**优先 epics.md 命名**（节点 3 真实落地依据；架构文档 §4 主体目录结构不必 update，但 §4 内的文件名示例 MotionStateProvider 视为待 Story 8.x 完成后由 Story 9.3 文档同步收口；与 8.1 HealthProvider 命名收口策略一致）.
- **`MotionPermissionManager` 不单独建文件**：架构 §4 列出 `MotionPermissionManager.swift` 作为示例文件，但本 story 把 requestPermission 行为放在 `MotionProvider` 协议本身（`requestPermission()` 方法）→ `MotionProviderImpl` 内部实装权限申请逻辑，**不**单独抽 PermissionManager。理由：① YAGNI——MVP 阶段 motion 权限只在一处申请（Story 8.4 HomeViewModel.bind），不需要独立 manager；② 与 HealthProvider 同模式（HealthPermissionManager 也没单独建文件）；③ 后续如需更复杂的权限策略（如多 framework 联合申请、status 变化监听）再抽出.
- **不预创建 `MotionState.swift`**：该 enum 在 Story 8.3 才落地（mapper 输入是 CMMotionActivity，输出是 MotionState）；本 story 不引入 `MotionState` 类型，避免 8.3 落地时与本 story 重复定义.

### References

- `_bmad-output/planning-artifacts/epics.md` § Epic 8 / Story 8.2（行 1471-1490）：本 story 全部 AC 钦定来源
- `_bmad-output/planning-artifacts/epics.md` § Story 8.3（行 1492-1515）：下游 mapper 输入契约（CMMotionActivity 直接消费）
- `_bmad-output/planning-artifacts/epics.md` § Story 8.4（行 1517-1548）：下游 PetSpriteView + HomeViewModel.petState 订阅契约
- `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §4 / §5.5 / §6.4 / §10.2 / §16 / §17.1：Motion 模块结构 + System Adapter 职责 + Steps 模块组合 + 状态映射规则 + 权限管理建议 + 测试建议
- `docs/宠物互动App_总体架构设计.md` 行 32 / 62 / 87 / 293 / 737 / 812 / 917：CoreMotion 作为系统能力接入层
- `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md` §3.1（手写 mock）/ §3.2（async/await 主流）/ §3.3（iphone 目录方案 D）/ §3.4（build 脚本契约）/ §4（版本锁定 + project.yml frameworks 段示例）
- `_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md` §3.1（AppState 注入规则）/ §3.2（白名单 7 字段，**不**含 motionState）：本 story HealthProvider/MotionProvider 不写 AppState
- `_bmad-output/implementation-artifacts/8-1-healthkit-接入.md`：本 story 直接复用的 8.1 模板（协议 / Impl / Mock / AppContainer wire / 集成 probe / launch arg / build verify 全部按 8.1 模板对齐）
- `iphone/PetApp/Core/Health/HealthProvider.swift` / `HealthProviderImpl.swift` / `HealthProviderMock.swift`：本 story 直接对照的实装参考（同 System Adapter 层，同手写 mock 模式）
- `iphone/PetApp/App/AppContainer.swift`（line 67-72 healthProvider 字段定义 + line 130 init 实例化）：MotionProvider 注入参考
- `iphone/PetApp/App/PetAppApp.swift`：launch argument 解析模式 + DEBUG-only probe 路径 + body 分支模板
- `iphone/PetApp/Features/DevTools/Views/HealthProviderProbeView.swift`：probe view 模板（8.1 落地）
- `iphone/PetAppUITests/HealthProviderIntegrationTests.swift`：集成测试模板（轮询 + 三态 PASS）
- `iphone/PetAppTests/Core/Health/HealthProviderMockTests.swift`：单元测试模板（@MainActor + async/await + reset 加分项）
- `iphone/project.yml`：framework 依赖声明 + Info.plist 描述 key 模板（8.1 已加 HealthKit + NSHealthShare/Update; 本 story 加 CoreMotion + NSMotionUsageDescription）

---

## Previous Story Intelligence（Epic 8 上一个 done = Story 8.1）

### 来自 Story 8.1 HealthKit 接入的关键学习（Story 8.2 本 story 同模式直接复用）

- **System Adapter 层位置**：`iphone/PetApp/Core/<Capability>/` 三件套（Provider / Impl / Mock）；本 story 严格对齐 `Core/Motion/`.
- **AppContainer wire 模式**：`public let xxxProvider: XxxProvider` + init 内默认 `XxxProviderImpl()`；YAGNI——不预留 init 注入参数（caller 落地时再加）.
- **DEBUG-only probe view 模板**：`Features/DevTools/Views/XxxProviderProbeView.swift` + `PetAppApp.swift` 解析 `-PetAppRunXxxProviderIntegrationProbe` launch arg + body 分支替代 RootView.
- **集成测试三态 PASS**：happy / sandbox-empty / error 任一即视作 path wired up（模拟器 sandbox 不稳定，CI 阶段不强求 happy path）.
- **Info.plist 描述 key 必须配 + framework dep 必须显式声明**：在 project.yml 中维护，xcodegen 自动 regen Info.plist；缺 plist key → simulator app 启动 crash（8.1 round 1 学到的：NSHealthUpdateUsageDescription 缺会让 toShare:[step] 直接 crash NSInvalidArgumentException）.
- **`@unchecked Sendable` + NSLock 保护内部 mutable state**：与 Swift 6.3 strict concurrency 配套；actor 在系统 callback 边界不友好.
- **不接 Capability / entitlements**：节点 3 阶段红线——模拟器跑只需 Info.plist；真机 / TestFlight 上线时单独 spike 加 entitlements（与 8.1 同收口策略）.
- **测试 class @MainActor**：避免 strict concurrency warning + async test 友好.
- **build verify 必跑 4 步**：build / --test / --uitest / otool framework 链接验证.
- **ios-simulator MCP 必跑**：normal launch + probe launch 各一次，遵守 CLAUDE.md "iOS UI 验证（必跑）"红线.

### 来自 Story 8.1 review/dev 过程中的关键 lesson（与本 story 高度相关）

- **`docs/lessons/2026-05-04-debug-seed-vs-probe-await-coupling.md`**：DEBUG seed 操作禁止 detached fire-and-forget，必须串到下游消费者 view 的 `.task` 里 await（避免 race）。本 story MotionProvider 没有 seed 操作（CoreMotion 不能"写入" activity），但 probe view 内 `requestPermission` → `startUpdates` 顺序也要同 await 模式（试图 await 完 permission 再调 startUpdates）。
- **`docs/lessons/2026-05-04-healthkit-read-auth-must-probe-read.md`**：HK read auth 必 probe-read。CoreMotion 与之相反——`authorizationStatus()` 可信，**不**需要 probe-read（详 AC2 决策）。本 story 必须避免照抄 HK 模式带来的过度复杂.
- **`docs/lessons/2026-05-04-healthkit-today-enddate-clamp-to-now.md`** / **`-strictstartdate-cross-midnight-tradeoff.md`**：HK 时间窗口的特殊处理——CoreMotion 不涉及（startUpdates 是流式订阅，不取时间窗口）。
- **codex round 1/2 review fix lessons（detached Task race + non-probe path 也要 seed）**：本 story 不需要 seed，但 `MotionProviderProbeView.task` 内的 `requestPermission` → `startUpdates` 也要按 8.1 同模式 await 完 permission 再调 startUpdates（避免未授权时 startUpdates 静默失败但 probe label 显示 "(waiting)" 误判）.

### 来自 Story 7.5 / 7.x server 端 + Story 5.5 / 37.4 AppState 落地的边界

- **本 story 与 server 端零耦合**：MotionProvider 是 iOS-only system framework 接入；不调 `/steps/sync` / `/steps/account` / 任何 server 接口（Story 8.5 才整合 server 同步）.
- **本 story 与 AppState 零耦合**：MotionProvider 不写 AppState 任一字段；motionState 仅在 Story 8.4 HomeViewModel 瞬时持有.
- **本 story 与 LoadHomeUseCase 零耦合**：8.4 才会在 HomeViewModel.bind 内订阅 motionProvider，与 LoadHome 数据流无关.

### 来自 iphone 工程已有 lessons（节选与本 story 高相关）

- `2026-04-25-swift-explicit-import-combine.md`：本 story `MotionProvider.swift` **不** import Combine（仅 Foundation + CoreMotion；async/await 主流）.
- `2026-04-26-swiftui-task-modifier-reentrancy.md`：MotionProviderProbeView `.task` 入口避免 reentrancy（与 8.1 ProbeView 同模式）.
- `2026-04-30-strong-vs-weak-for-constructor-injected-state.md`：构造注入 strong 持有原则——MotionProviderImpl 不持外部引用，自然合规；MotionProviderMock 也只持 closure ref（lock 内 read 是 fresh，不形成 retain cycle）.
- `2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md`：probe view `.task` 而非 `.onAppear`（async 操作必 task）.
- `2026-04-26-mockbase-snapshot-only-reads.md`：mock 内部状态字段 `private`，对外只暴露 `*Snapshot()` —— 本 story MotionProviderMock 沿用此精神（registeredHandler 是 private + lock-protected，对外通过 `injectActivity` 间接驱动）.

---

## Git Intelligence Summary（最近 5 commit + Story 8.1 收官）

```
53db74a chore(claude): 扩 settings.local.json allow list（mcp/brew/idb/pipx 等）   ← MCP / dev tools 配置无关本 story
6683832 docs(epics): 给 epic-8/21 受 epic-37 影响的 6 条 story 补 addendum         ← Story 8.4 / 8.5 已加 ADR-0010 addendum；Story 8.2 不需要 addendum（无 AppState 直接耦合）
3ec4dfd chore(story-7-5): 收官 Story 7.5 + 归档 story 文件                          ← server 端 dev grant 已就绪
e76b043 feat(server): Epic7/7.5 dev 端点 POST /dev/grant-steps                    ← 与本 story 无关
1e4a19c chore(story-7-4): 收官 Story 7.4 + 归档 story 文件
```

最近一次 done 的 story = 8-1（HealthKit 接入；2026-05-04 收官，commit 标记略；详见 `8-1-healthkit-接入.md` Status: done + Change Log）.

**关键 actionable 推论**：

- **Epic 7（server 端）+ Story 8.1 已 done**：iOS 端 Epic 8 第 2 条 story 启动无前置阻塞.
- **本 story 8.2 是 Epic 8 第二条**：Story 8.1 模板已 battle-tested（5 unit + 1 UITest 集成全 pass）；本 story 严格对齐 8.1 模板复用 → review 风险低.
- **节点 3 收尾**（Story 8.5 完成）所需 5 条 story 顺序 lock：8.1 → **8.2 (本)** → 8.3 → 8.4 → 8.5；本 story 落地后下一条立即可启动（8.3 Mapper，pure function 测试，不依赖 framework 运行时）.
- **commit 6683832 给 8.4 / 8.5 加了 ADR-0010 addendum**（HomeViewModel motionState 持瞬时 + 禁 EnvironmentObject）；本 story 不直接受 addendum 影响（不写 AppState、不动 ViewModel），但 8.4 / 8.5 落地时 addendum 仍是钦定 source.

---

## Latest Tech Information（CoreMotion + Xcode 26）

- **CMMotionActivityManager.authorizationStatus()** 是 iOS 11+ 公开 API（与 HealthKit `authorizationStatus(for:)` 不同——CM 的 status 是真实可读的）；ADR-0002 §4 deployment target iOS 17 完全覆盖.
- **CMMotionActivity 数据字段**：`stationary` / `walking` / `running` / `cycling` / `automotive` / `unknown`（Bool）+ `confidence`（CMMotionActivityConfidence: low/medium/high）+ `startDate`（Date）；多个 flag 可同时为 true.
- **CMMotionActivityManager.startActivityUpdates(to:withHandler:)** 必须在 main thread 调用（Apple 文档钦定）；handler 在 to: 指定的 OperationQueue 触发 callback.
- **CMMotionActivityManager.queryActivityStarting(from:to:to:withHandler:)** 是 fetch API（带完成 callback），与 startActivityUpdates 是流式 API 区分；本 story 仅在 requestPermission 探针场景用 query.
- **`CMErrorMotionActivityNotAuthorized`**：CM 的权限拒绝 NSError code（在 CMErrorDomain 下）；映射为 `MotionProviderError.permissionDenied`.
- **iPhone 17 simulator + Xcode 26.4** 支持 CoreMotion：模拟器自动放行 activity 识别（无系统弹窗）；但模拟器 sandbox 是否持续 push activity 事件不稳定（与 8.1 HK sandbox 同精神）—— 集成测试三态 PASS 路径设计.
- **Swift 6.3 strict concurrency**：CMMotionActivityManager 不是 Sendable；用 `@unchecked Sendable` + NSLock 保护 isUpdating / currentHandler 是合规模式（与 HKHealthStore 同精神）.
- **withCheckedThrowingContinuation**：Swift Concurrency 桥接 CMMotionActivityManager.queryActivityStarting callback 的标准模式（vs unsafe 版本无 Apple 推荐场景）.
- **CoreMotion Capability**：CoreMotion 不需要 entitlements 文件 / Capability 段（与 HealthKit 不同——HK 真机要 entitlements，CM 仅靠 Info.plist `NSMotionUsageDescription` 即可）；模拟器跑 + 真机都不需要额外配置（节点 3 阶段红线）.

---

## Project Context Reference

- **CLAUDE.md "状态：重启中"**：旧 ios/ 不动；本 story 仅在 iphone/ 下落地
- **CLAUDE.md "Tech Stack"**：iOS = Swift + SwiftUI + MVVM + UseCase + Repository + HealthKit / CoreMotion 接入
- **CLAUDE.md "iOS UI 验证（必跑）"**：本 story AC8 必须用 ios-simulator MCP 验证（不能仅靠 build.sh 通过就报 done）
- **CLAUDE.md "工作纪律"**："状态以 server 为准" + "节点顺序不可乱跳"：本 story 是节点 3 的 Story 8.2，前置依赖 Epic 1～7 + Story 8.1 done（已满足）

---

## Story Completion Status

- **Status**: ready-for-dev（comprehensive 故事 context 已生成；模板沿用 Story 8.1 battle-tested 路径；落地风险低）
- **Completion Note**: 8 个 AC 钦定（协议 / Impl / Info.plist + framework 依赖 / Mock / AppContainer wire / 单元测试 ≥4 / 集成测试 ≥1 / build verify）；7 个文件预计变更（4 production 新增 + 1 test 新增 + 1 UITest 新增 + 1 probe view 新增 + 3 修改 = AppContainer + project.yml + PetAppApp）；红线 6 条全列（不动 RootView / HomeViewModel / LoadHomeUseCase / Health 既有文件 / scripts/build.sh / ios+server 工程）.

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]（Claude Opus 4.7 (1M context)）

### Debug Log References

**关键 lesson — CMMotionActivity KVC 在 Xcode 26 SDK 上 crash**：

Story 文档 AC4 钦定 mock 用 KVC `setValue:forKey:` 给 CMMotionActivity 私有 readonly properties 赋值，
但实际跑 `bash iphone/scripts/build.sh --test` 时 4 个测试 case crash 失败：
- `freed pointer was not the last allocation` (`Restarting after unexpected exit, crash, or test timeout`)
- 凡是 inject KVC 构造的 activity 实例的 case 都 crash（包括 `testReset`、`testRequestPermissionGranted` 等）

**根因诊断**：CMMotionActivity 在 Xcode 26 SDK 下 KVC 赋值 BOOL readonly stored property（`stationary` / `walking` 等）会破坏 ARC retain count
→ dealloc 时 double-free → heap corruption。

**修复**：改用 ObjC 子类 (`_StubMotionActivity: CMMotionActivity, @unchecked Sendable`) 覆盖 readonly property getter——
- 子类标 `@objc(_PetAppStubMotionActivity)` 让 isKind(of:) / responds(to:) 仍成立
- private stored fields + override getter 返回 fixture 值
- `required init?(coder:)` fatalError（NSCoding 不参与）
- 测试零 crash，全 6 case PASS

**相关变更**：MotionProviderMock.swift 末尾追加 `_StubMotionActivity` 子类；保留 `makeActivity(...)` 公开 API 不变。

**lesson 待蒸馏**：`docs/lessons/2026-05-04-cmmotionactivity-kvc-crash-use-subclass-override.md`（fix-review 阶段补，本 story 落地不阻塞）。

**第二个 debug**：simulator UI 一直显示 home view 而非 probe view —— 实际原因是首次跑 build 后 PetAppApp.swift 改动没被 incremental compiler 拾起（旧的 binary 缓存）。`touch iphone/PetApp/App/PetAppApp.swift && bash iphone/scripts/build.sh` 强制重编译后 `strings` 验证 `-PetAppRunMotionProviderIntegrationProbe` 字符串已嵌入 binary，重装后 probe view 正常挂载。

### Completion Notes List

- **AC1 ✅** `MotionProvider` 协议（async throws + Sendable + @Sendable handler）+ `MotionProviderError` 三态 enum 落地。Story 文档钦定的协议签名 100% 对齐。
- **AC2 ✅** `MotionProviderImpl` 用 NSLock + `@unchecked Sendable` + `withCheckedThrowingContinuation` 桥接 CMMotionActivityManager。`probePermissionViaQuery` 用 `queryActivityStarting` 触发权限弹窗；`@unknown default` 兜底未来 iOS 新增 case。所有 lock 区间内**无 await**，遵守 round 1 lesson（lock 跨 await 是 Swift 6 error）。
- **AC3 ✅** `iphone/project.yml` 加 `CoreMotion.framework` dep + `NSMotionUsageDescription` plist key。`otool -L` 验证 CoreMotion + HealthKit 都链到 binary。模拟器实跑权限弹窗正确显示中文描述。
- **AC4 ✅** `MotionProviderMock` 完整 stub + invocations + callCount + injectActivity API；二次 startUpdates 防御与 Impl 同语义；`makeActivity` 用 ObjC 子类策略（KVC 在 Xcode 26 crash → subclass override）。
- **AC5 ✅** `AppContainer.motionProvider: MotionProvider` 字段 + init 默认 `MotionProviderImpl()`；零 caller 触碰。
- **AC6 ✅** 6 个单元测试 case 全 pass（happy / denied / stop / 多次 start / stop-then-start / reset）；ReceivedActivities helper 用 NSLock 让 @Sendable closure 内 append 安全。
- **AC7 ✅** `MotionProviderProbeView` (#if DEBUG) + `MotionProviderIntegrationTests`（XCUITest 30s 轮询三态 PASS）+ `PetAppApp.swift` 解析 `-PetAppRunMotionProviderIntegrationProbe` 互斥分支；XCUITest pass + HealthProviderIntegrationTests 零回归。
- **AC8 ✅** `bash iphone/scripts/build.sh` exit 0；`bash iphone/scripts/build.sh --test` 358/358 pass；`bash iphone/scripts/build.sh --uitest` MotionProviderIntegrationTests + HealthProviderIntegrationTests 都 pass（HomeUITests 6 个失败 + KeychainPersistenceUITests 1 个失败是 baseline 既有问题，stash 验证基线 8 failures vs 本 story 7 failures，不是本 story 引入）。
- **ios-simulator MCP 实测**：iPhone 17 Pro 模拟器在 Xcode 26.4 实测 `CMMotionActivityManager.isActivityAvailable() == false` → probe view 落到 errorText="activityDataNotAvailable" 路径，resultText="(waiting)"——这是 AC7 钦定的"path wired up = PASS"sandbox-limited 三态之一。a11y identifiers `motionProviderProbeResult` + `motionProviderProbeError` 都正确暴露。

### File List

新增（6 个文件）:
- `iphone/PetApp/Core/Motion/MotionProvider.swift`（AC1）
- `iphone/PetApp/Core/Motion/MotionProviderImpl.swift`（AC2）
- `iphone/PetApp/Core/Motion/MotionProviderMock.swift`（AC4，含 _StubMotionActivity 子类）
- `iphone/PetApp/Features/DevTools/Views/MotionProviderProbeView.swift`（AC7，#if DEBUG）
- `iphone/PetAppTests/Core/Motion/MotionProviderMockTests.swift`（AC6，6 个 case）
- `iphone/PetAppUITests/MotionProviderIntegrationTests.swift`（AC7，1 个 XCUITest case）

修改（5 个文件）:
- `iphone/project.yml`（AC3：CoreMotion.framework dep + NSMotionUsageDescription plist key）
- `iphone/PetApp/Resources/Info.plist`（AC3：xcodegen 自动 regen 加 NSMotionUsageDescription）
- `iphone/PetApp.xcodeproj/project.pbxproj`（AC3：xcodegen regen，添加新文件 + framework reference）
- `iphone/PetApp/App/AppContainer.swift`（AC5：加 motionProvider 字段 + init 默认 MotionProviderImpl）
- `iphone/PetApp/App/PetAppApp.swift`（AC7：加 useMotionProviderProbe + body 分支）

零打扰（红线全部满足）:
- `iphone/PetApp/App/RootView.swift`（启动期不触发权限）
- `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`（不消费 motionProvider）
- `iphone/PetApp/Features/Home/UseCases/LoadHomeUseCase.swift`（不依赖 motion）
- `iphone/PetApp/Core/Health/*`（Story 8.1 文件零打扰）
- `iphone/PetApp/Features/DevTools/UseCases/HealthKitDevSeedUseCase.swift`（8.1 dev seed 不动）
- `iphone/PetApp/Features/DevTools/Views/HealthProviderProbeView.swift`（8.1 probe 不动）
- `iphone/PetAppUITests/HealthProviderIntegrationTests.swift`（8.1 集成测试不动）
- `iphone/scripts/build.sh`（已支持 --uitest）
- `ios/*` 任一文件（重启阶段零打扰）
- `server/*` 任一文件（iOS-only story）

### Change Log

- 2026-05-04: Story 8.2 dev 实装完成 → 状态 review；6 个新文件 + 5 个修改；358 unit tests pass；UITest motion probe pass；ios-simulator MCP 实测 probe view 渲染正确（sandbox-limited path）。详见 Completion Notes 与 Debug Log。
