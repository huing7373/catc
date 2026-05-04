// MotionProviderMock.swift
// Story 8.2 AC4: MotionProvider 的 in-memory 测试 mock；按 ADR-0002 §3.1 手写 invocation 记录.
//
// 用法：
//   let mock = MotionProviderMock()
//   mock.requestPermissionStub = .success(true)
//   mock.startUpdates { activity in ... }
//   mock.injectActivity(MotionProviderMock.makeActivity(walking: true))  // 触发 handler
//
// 设计决策（详见 story 8-2-coremotion-接入.md AC4 段）:
// - 不继承 MockBase（class）：MotionProviderMock 走 production target（PetApp/Core/Motion），
//   PetApp target 不能 @testable import test helper.
// - 位置在 PetApp/Core/Motion/ 而非 PetAppTests/：架构 §17.1 钦定 mock 在 production target
//   让 Preview / DevTools / 集成测试都能消费.
// - 二次 startUpdates 防御与 MotionProviderImpl 同语义：第二次起的 startUpdates 直接被忽略.
// - injectActivity 是 mock 特有 helper（Impl 没有对应方法）——这是测试驱动模式：
//   测试通过 inject 触发 handler，验证业务层订阅链路.
// - @unchecked Sendable：字段 mutable 但测试串行调用，与 8.1 同模式.

import Foundation
import CoreMotion

/// MotionProvider 的 in-memory 测试 mock；按 ADR-0002 §3.1 手写 invocation 记录.
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
    private var registeredHandler: (@Sendable (CMMotionActivity) -> Void)?

    /// generation token——与 MotionProviderImpl 同精神（防 stop/restart race 时 stale callback 串到新订阅）.
    /// mock 暴露此机制是为了测试可以模拟"stop 之后还残留 enqueue 的 stale callback"时序，验证 forward 拦截.
    /// 详见 review round 1 P2 + docs/lessons/2026-05-04-motion-stop-restart-stale-callback-race.md.
    private var generation: UInt64 = 0

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
        // generation 自增——与 MotionProviderImpl 同语义；测试可通过 captureGeneration() / injectActivity(generation:)
        // 模拟 stop 之后才执行的 stale callback 时序.
        generation &+= 1
        lock.unlock()
    }

    public func stopUpdates() {
        invocations.append("stopUpdates")
        stopUpdatesCallCount += 1
        lock.lock()
        registeredHandler = nil
        // 与 MotionProviderImpl 同语义：stop 自增 generation 让 stale callback 失效.
        generation &+= 1
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

    /// 测试用：捕获当前 generation token——配合 injectActivity(generation:) 模拟 stop/restart race
    /// 时序（"先 capture generation，再 stopUpdates，再 startUpdates，再以旧 generation inject"
    /// 应当被丢弃；详见 MotionProviderMockTests case 7）.
    public func captureGeneration() -> UInt64 {
        lock.lock(); defer { lock.unlock() }
        return generation
    }

    /// 测试用：以"指定 generation"的口径 inject activity——模拟"系统已经 enqueue 但还没 invoke 的 stale callback"场景.
    /// 行为：若 expectedGeneration != 当前 generation 则丢弃（handlerInvocationCount 不增）；
    /// 若 expectedGeneration == 当前 generation 且 handler 存在 → 正常 forward.
    /// 这模仿 MotionProviderImpl 中 callback closure 内的 generation check 行为，
    /// 让 unit test 在 mock 上可重现 race 时序而不依赖真 CMMotionActivityManager.
    public func injectActivity(_ activity: CMMotionActivity, expectedGeneration: UInt64) {
        lock.lock()
        guard generation == expectedGeneration, let captured = registeredHandler else {
            lock.unlock()
            return
        }
        lock.unlock()
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
        generation = 0
        lock.unlock()
    }
}

/// 便利构造工具：手工 build CMMotionActivity 用于 mock 注入.
///
/// 实装策略（从 CMMotionActivity readonly properties + Xcode 26 SDK 实测崩溃推导）:
/// - **不**用 KVC `setValue:forKey:` 给 BOOL property 赋值——Xcode 26 实测 simulator 上
///   会触发 "freed pointer was not the last allocation" 内存越界 crash，原因疑似 CMMotionActivity
///   的 readonly stored property 在 ARC 下用 KVC 赋值会让 ivar 状态不一致 → dealloc 时 double-free.
/// - 改用 ObjC 子类覆盖 readonly property 的 getter——这是 Apple 自身测试 fixture 的稳妥模式
///   （对 NSObject 子类 readonly 属性 mock，最安全是 inherit + override getter）.
/// - 子类标 `@objc` 让 isKind(of:) / responds(to:) 仍然成立——`MotionProvider` 协议层暴露
///   `CMMotionActivity` 类型，调用方对 mock instance 的运行时行为透明（Story 8.3 mapper 直接读 flag）.
extension MotionProviderMock {
    /// 创建一个 CMMotionActivity 子类实例（覆盖 readonly getter 返回 fixture 值）.
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
        return _StubMotionActivity(
            stationary: stationary,
            walking: walking,
            running: running,
            cycling: cycling,
            automotive: automotive,
            unknown: unknown,
            confidence: confidence,
            startDate: startDate
        )
    }
}

/// CMMotionActivity 测试 stub 子类——覆盖 readonly properties 的 getter.
///
/// 设计理由：
/// - CMMotionActivity 的 stored properties 全 `readonly`；KVC 赋值在 Xcode 26 SDK 上 crash
///   ("freed pointer was not the last allocation"），改用 subclass override getter 是稳妥路径.
/// - `@objc` 暴露 ObjC runtime 让 isKind(of:) / responds(to:) 仍成立——上游对此实例的
///   `activity.walking` 等访问走子类 getter，与系统 instance 行为完全一致.
/// - `final` 防进一步派生（mock 边界明确）；`@unchecked Sendable` 与 MotionProviderMock 同精神.
@objc(_PetAppStubMotionActivity)
private final class _StubMotionActivity: CMMotionActivity, @unchecked Sendable {
    private let _stationary: Bool
    private let _walking: Bool
    private let _running: Bool
    private let _cycling: Bool
    private let _automotive: Bool
    private let _unknown: Bool
    private let _confidence: CMMotionActivityConfidence
    private let _startDate: Date

    init(
        stationary: Bool,
        walking: Bool,
        running: Bool,
        cycling: Bool,
        automotive: Bool,
        unknown: Bool,
        confidence: CMMotionActivityConfidence,
        startDate: Date
    ) {
        self._stationary = stationary
        self._walking = walking
        self._running = running
        self._cycling = cycling
        self._automotive = automotive
        self._unknown = unknown
        self._confidence = confidence
        self._startDate = startDate
        super.init()
    }

    /// Required NSCoder init from NSObject hierarchy——本子类不参与归档/反归档场景，
    /// 抛 fatalError 即可（与 Apple 推荐"NSCoding 不实装的子类"模式一致）.
    required init?(coder: NSCoder) {
        fatalError("_StubMotionActivity 不支持 NSCoding")
    }

    override var stationary: Bool { _stationary }
    override var walking: Bool { _walking }
    override var running: Bool { _running }
    override var cycling: Bool { _cycling }
    override var automotive: Bool { _automotive }
    override var unknown: Bool { _unknown }
    override var confidence: CMMotionActivityConfidence { _confidence }
    override var startDate: Date { _startDate }
}
