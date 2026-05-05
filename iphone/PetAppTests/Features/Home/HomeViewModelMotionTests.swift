// HomeViewModelMotionTests.swift
// Story 8.4 AC7: HomeViewModel.petState 订阅 MotionProvider 链路单元测试.
//
// 测试用 Story 8.2 MotionProviderMock 注入 → bind(motionProvider:) → injectActivity → 断言 petState 切换.
// 不引第三方断言 lib（XCTest only；ADR-0002 §3.1）.

import XCTest
import CoreMotion
@testable import PetApp

@MainActor
final class HomeViewModelMotionTests: XCTestCase {

    // happy: ViewModel 启动时订阅 MotionProvider，初始状态 = .rest（epics.md AC 行 1544）
    func testInitialPetStateIsRest() {
        let viewModel = HomeViewModel()
        XCTAssertEqual(viewModel.petState, .rest, "初始 petState 应为 .rest")
    }

    // happy: bind(motionProvider:) 后 startUpdates 被调一次
    func testBindMotionProvider_callsStartUpdatesOnce() {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()

        viewModel.bind(motionProvider: mock)

        XCTAssertEqual(mock.startUpdatesCallCount, 1, "bind 后 startUpdates 应被调一次")
    }

    // happy: bind(motionProvider:) 二次调用被短路（不重复订阅）
    func testBindMotionProvider_secondCallIsIgnored() {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()

        viewModel.bind(motionProvider: mock)
        viewModel.bind(motionProvider: mock)   // 二次 bind 应被 guard 短路

        XCTAssertEqual(mock.startUpdatesCallCount, 1, "二次 bind 应被 guard 短路，startUpdates 仍只调一次")
    }

    // happy: MotionProvider 推 walk activity → mapper 转 .walk → ViewModel.petState = .walk
    // （epics.md AC 行 1545）
    func testInjectWalkingActivity_drivesPetStateToWalk() async {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()
        viewModel.bind(motionProvider: mock)

        let walkActivity = MotionProviderMock.makeActivity(walking: true)
        mock.injectActivity(walkActivity)

        // 给 Task { @MainActor in ... } 一个 runloop tick 完成异步派发.
        // 不用 XCTestExpectation（轻量；与 8.1 / 8.2 单测既有 yield 模式同精神）.
        await Task.yield()

        XCTAssertEqual(viewModel.petState, .walk, "注入 walking activity 后 petState 应为 .walk")
    }

    // happy: 连续切换 rest → walk → run → rest，ViewModel 状态正确流转（epics.md AC 行 1546）
    func testSequentialActivityChange_drivesPetStateThroughAllThreeStates() async {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()
        viewModel.bind(motionProvider: mock)

        XCTAssertEqual(viewModel.petState, .rest, "初始 .rest")

        // rest → walk
        mock.injectActivity(MotionProviderMock.makeActivity(walking: true))
        await Task.yield()
        XCTAssertEqual(viewModel.petState, .walk, "walking → .walk")

        // walk → run
        mock.injectActivity(MotionProviderMock.makeActivity(running: true))
        await Task.yield()
        XCTAssertEqual(viewModel.petState, .run, "running → .run")

        // run → rest
        mock.injectActivity(MotionProviderMock.makeActivity(stationary: true))
        await Task.yield()
        XCTAssertEqual(viewModel.petState, .rest, "stationary → .rest")
    }

    // edge: 未 bind motionProvider → injectActivity 后 petState 仍 .rest
    // 防御性 case：caller 漏 bind 时 ViewModel 不应崩溃（mock 内 startUpdatesCallCount=0 → handler nil → injectActivity no-op）
    func testInjectActivityWithoutBind_doesNotChangePetState() async {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()

        // 未调 bind(motionProvider:)
        let walkActivity = MotionProviderMock.makeActivity(walking: true)
        mock.injectActivity(walkActivity)
        await Task.yield()

        XCTAssertEqual(viewModel.petState, .rest, "未 bind 时 petState 应保持 .rest 默认值")
        XCTAssertEqual(mock.startUpdatesCallCount, 0, "未 bind 时 startUpdates 不应被调")
    }

    // edge: ViewModel deinit 时 stopUpdates 被调（防泄漏；epics.md AC 行 1547）
    // 验证 deinit { motionProvider?.stopUpdates() } 路径生效.
    func testViewModelDeinit_callsStopUpdatesOnMotionProvider() {
        let mock = MotionProviderMock()
        do {
            let viewModel = HomeViewModel()
            viewModel.bind(motionProvider: mock)
            XCTAssertEqual(mock.stopUpdatesCallCount, 0, "bind 后 stopUpdates 不应被调")
        }
        // 出 do-block ARC 释放 viewModel → deinit 触发.
        // 注：Swift deinit 是 nonisolated；mock.stopUpdates 是同步方法；可立即断言.
        XCTAssertEqual(mock.stopUpdatesCallCount, 1, "ViewModel deinit 后 stopUpdates 应被调一次")
    }

    // MARK: - Story 8.4 review round 1 P1: bind 前必须 gate authorizationStatus

    // edge: bind 时 authorizationStatus == .notDetermined → 不调 startUpdates（防 first-launch 弹权限）
    // round 1 P1 修复的核心契约：未授权时 bind 是"仅持引用，不订阅"，否则 first-launch 下
    // CMMotionActivityManager.startActivityUpdates 会触发系统 NSMotionUsageDescription 弹窗.
    // 详见 docs/lessons/2026-05-04-motion-bind-must-gate-on-authorization-status.md.
    func testBindMotionProvider_whenNotDetermined_doesNotCallStartUpdates() {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()
        mock.authorizationStatusStub = .notDetermined

        viewModel.bind(motionProvider: mock)

        XCTAssertEqual(mock.authorizationStatusCallCount, 1,
                       "bind 必须查询 authorizationStatus 决定是否订阅")
        XCTAssertEqual(mock.startUpdatesCallCount, 0,
                       ".notDetermined 时禁止 startUpdates（会触发系统权限弹窗）")
    }

    // edge: bind 时 authorizationStatus == .denied → 不调 startUpdates
    func testBindMotionProvider_whenDenied_doesNotCallStartUpdates() {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()
        mock.authorizationStatusStub = .denied

        viewModel.bind(motionProvider: mock)

        XCTAssertEqual(mock.startUpdatesCallCount, 0, ".denied 时不订阅")
    }

    // edge: bind 时 authorizationStatus == .restricted → 不调 startUpdates
    func testBindMotionProvider_whenRestricted_doesNotCallStartUpdates() {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()
        mock.authorizationStatusStub = .restricted

        viewModel.bind(motionProvider: mock)

        XCTAssertEqual(mock.startUpdatesCallCount, 0, ".restricted 时不订阅")
    }

    // happy: bind 时 authorizationStatus == .authorized → 调 startUpdates 一次
    // （等价于"既有 mock 默认 .authorized stub 下的行为"，但显式 stub 让测试意图清晰）
    func testBindMotionProvider_whenAuthorized_callsStartUpdatesOnce() {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()
        mock.authorizationStatusStub = .authorized

        viewModel.bind(motionProvider: mock)

        XCTAssertEqual(mock.authorizationStatusCallCount, 1)
        XCTAssertEqual(mock.startUpdatesCallCount, 1, ".authorized 时 startUpdates 立即被调")
    }

    // edge: 未授权 bind → 后续授权 + 再次 bind → 这次 startUpdates 才被调（idempotent rebind 升级路径）
    // 模拟生产路径：8.4 onAppear bind（first launch 未授权 → noop）→ 8.6 授权 flow 后再次调
    // bind 升级到 authorized startUpdates；第二次 bind 走 hasStartedMotionUpdates 短路 + 升级.
    func testBindMotionProvider_unauthorizedThenAuthorizedRebind_startsUpdatesOnSecondBind() {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()

        mock.authorizationStatusStub = .notDetermined
        viewModel.bind(motionProvider: mock)
        XCTAssertEqual(mock.startUpdatesCallCount, 0, "first bind .notDetermined 不订阅")

        // 模拟 8.6 授权 flow 走完
        mock.authorizationStatusStub = .authorized
        viewModel.bind(motionProvider: mock)
        XCTAssertEqual(mock.startUpdatesCallCount, 1, "second bind .authorized 升级到订阅")

        // 再次 bind → hasStartedMotionUpdates 短路（防重复订阅 / 双倍事件）
        viewModel.bind(motionProvider: mock)
        XCTAssertEqual(mock.startUpdatesCallCount, 1, "third bind 已订阅 → 短路 noop")
    }

    // edge: 未授权 bind 后 deinit 仍调 stopUpdates（持引用 → deinit 路径不变；幂等 stop 安全）
    func testBindMotionProviderUnauthorized_thenDeinit_stillCallsStopUpdates() {
        let mock = MotionProviderMock()
        mock.authorizationStatusStub = .denied
        do {
            let viewModel = HomeViewModel()
            viewModel.bind(motionProvider: mock)
            XCTAssertEqual(mock.startUpdatesCallCount, 0)
        }
        // deinit 仍调 stopUpdates——MotionProvider.stopUpdates 是 idempotent，
        // 哪怕本次 bind 没 startUpdates 也安全（mock 内部 registeredHandler == nil 直接 noop）.
        XCTAssertEqual(mock.stopUpdatesCallCount, 1, "deinit 始终调 stopUpdates（hold-ref 路径不变）")
    }

    // MARK: - Story 8.4 review round 4 P2: bind 必须支持 permission downgrade

    // edge（round 4 P2 修复核心）：先 .authorized → 订阅 + 推 walk → 用户撤销权限 → rebind →
    // 验证 stopUpdates 被调 + petState 回到 .rest（不卡 stale .walk）+ hasStartedMotionUpdates 重置.
    // 详见 docs/lessons/2026-05-04-auth-gated-subscription-must-handle-downgrade.md.
    func testBind_authorizedSubscribed_thenRevoked_stopsUpdatesAndResetsState() async {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()

        // ① 第一次 bind：authorized → 订阅生效
        mock.authorizationStatusStub = .authorized
        viewModel.bind(motionProvider: mock)
        XCTAssertEqual(mock.startUpdatesCallCount, 1, "first bind authorized 应订阅")

        // ② 推 walk activity → petState = .walk
        mock.injectActivity(MotionProviderMock.makeActivity(walking: true))
        await Task.yield()
        XCTAssertEqual(viewModel.petState, .walk, "walk activity 后 petState = .walk（被 stale 起点）")

        // ③ 用户去 Settings 撤销权限 → mock 切到 .denied
        mock.authorizationStatusStub = .denied

        // ④ 第二次 bind（模拟 RootView 在 ScenePhase active 触发的 rebind）
        viewModel.bind(motionProvider: mock)

        // ⑤ 断言：downgrade 路径生效
        XCTAssertEqual(mock.stopUpdatesCallCount, 1,
                       "downgrade 时 stopUpdates 必须被调（拆老订阅，不留 stale callback 入口）")
        XCTAssertEqual(viewModel.petState, .rest,
                       "downgrade 后 petState 必须 reset 到 .rest（UI 端不卡 stale .walk）")
        XCTAssertEqual(mock.startUpdatesCallCount, 1,
                       "downgrade 路径不应再次 startUpdates（still 1 from first bind）")

        // ⑥ Bonus：再次授权 + rebind → 应能升级回 startUpdates（hasStartedMotionUpdates 已 reset）
        mock.authorizationStatusStub = .authorized
        viewModel.bind(motionProvider: mock)
        XCTAssertEqual(mock.startUpdatesCallCount, 2,
                       "re-grant 后 rebind 应升级回订阅（hasStartedMotionUpdates 已被 downgrade 重置）")
    }

    // edge（round 4 P2 衍生）：已订阅 + 再次 .authorized rebind → idempotent noop（不重复订阅 / 不动 petState）
    // 防御 RootView 在 ScenePhase active 频繁触发 rebind 时的常态路径（权限没变化）.
    func testBind_authorizedTwice_idempotent() async {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()
        mock.authorizationStatusStub = .authorized

        viewModel.bind(motionProvider: mock)
        XCTAssertEqual(mock.startUpdatesCallCount, 1)

        // 推一个 walk activity 让 petState 进入 .walk
        mock.injectActivity(MotionProviderMock.makeActivity(walking: true))
        await Task.yield()
        XCTAssertEqual(viewModel.petState, .walk)

        // 再次 bind（仍 .authorized）→ 应 idempotent，不动状态
        viewModel.bind(motionProvider: mock)
        XCTAssertEqual(mock.startUpdatesCallCount, 1, "已订阅 + 仍 authorized 不应重复 startUpdates")
        XCTAssertEqual(mock.stopUpdatesCallCount, 0, "已订阅 + 仍 authorized 不应触发 stopUpdates")
        XCTAssertEqual(viewModel.petState, .walk, "已订阅 + 仍 authorized 不应回踩 petState")
    }

    // edge（round 4 P2 衍生）：未订阅 + 未授权 rebind → 仍仅持引用 return（不调 stopUpdates 也不 reset petState）.
    // 这是 first-launch path 的多次 rebind 常态——权限始终未授，bind 应纯 noop.
    func testBind_unauthorizedTwice_remainsNoOp() {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()
        mock.authorizationStatusStub = .notDetermined

        viewModel.bind(motionProvider: mock)
        viewModel.bind(motionProvider: mock)

        XCTAssertEqual(mock.startUpdatesCallCount, 0, "始终未授权 → 不订阅")
        XCTAssertEqual(mock.stopUpdatesCallCount, 0, "未订阅过的状态下不该 stopUpdates")
        XCTAssertEqual(viewModel.petState, .rest, "始终 .rest，不应被 downgrade 路径误改写")
    }

    // MARK: - Story 8.4 review round 5 P2: motion handler Task @MainActor stale write race

    // edge（round 5 P2 修复核心）：在飞 Task @MainActor stale callback 不能覆盖 downgrade 后已 reset 的 petState.
    //
    // 时序（generation token 拦截路径）：
    //   ① bind authorized → mock.startUpdates 注入 handler；motionBindGeneration += 1（=1），closure capture token=1.
    //   ② 推 walk activity → Task { check generation==1, set petState = .walk } 排队入 main actor.
    //      （此处用 await Task.yield() 让 Task 实际跑掉，让 petState=.walk 先生效）
    //   ③ 用户撤权限 → 第二次 bind → downgrade 路径推进 motionBindGeneration += 1（=2）+ petState=.rest.
    //   ④ "**模拟 stale handler 仍能 invoke**"——直接调 mock.injectActivity，handler closure 还指向老 captured token=1
    //      但 motionBindGeneration 已 = 2 → Task 内 generation guard 拦截 → drop, petState 仍 .rest.
    //
    // before 修复（round 4 实现）：Task 不 check generation → 无差别覆盖 petState = .walk → 测试 fail（expect .rest got .walk）.
    // after 修复（round 5）：Task check generation == captured token，token 不等就 drop → petState 仍 .rest.
    //
    // 详见 docs/lessons/2026-05-04-mainactor-task-hop-stale-write-needs-generation-token.md.
    func testStaleMotionCallback_afterDowngrade_doesNotOverwriteRestState() async {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()

        // ① first bind: authorized → startUpdates handler 注册
        mock.authorizationStatusStub = .authorized
        viewModel.bind(motionProvider: mock)
        XCTAssertEqual(mock.startUpdatesCallCount, 1)

        // ② 推 walk activity 让 petState 先进入 .walk（与 stale 起点对齐）
        mock.injectActivity(MotionProviderMock.makeActivity(walking: true))
        await Task.yield()
        XCTAssertEqual(viewModel.petState, .walk)

        // ③ downgrade：撤权限 + rebind
        mock.authorizationStatusStub = .denied
        viewModel.bind(motionProvider: mock)
        XCTAssertEqual(mock.stopUpdatesCallCount, 1, "downgrade 必须 stopUpdates")
        XCTAssertEqual(viewModel.petState, .rest, "downgrade 立刻 reset 到 .rest")

        // ④ 模拟"在飞 stale Task @MainActor 仍排队中"——直接 inject activity 让 mock 调老 handler closure
        //   （mock 在 stopUpdates 后不主动清 handler；handler closure 仍指向老 captured token，与现 token 不等）
        //   即使 mock handler 仍存在，handler 派发 Task 进 main actor 后 generation check 拦截 → drop.
        mock.injectActivity(MotionProviderMock.makeActivity(walking: true))
        await Task.yield()

        XCTAssertEqual(viewModel.petState, .rest,
                       "stale callback Task 必须被 generation token 拦截，不覆盖已 reset 的 .rest")
    }

    // edge（round 5 P2 衍生）：再次授权后 rebind → 新 startUpdates handler capture 新 token，新 callback 正常 mutate;
    // 老 in-flight stale callback（来自第一次订阅）仍被 token mismatch 拦截.
    func testStaleMotionCallback_afterRegrant_doesNotOverwriteNewState() async {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()

        // ① first bind authorized → token=1
        mock.authorizationStatusStub = .authorized
        viewModel.bind(motionProvider: mock)
        XCTAssertEqual(mock.startUpdatesCallCount, 1)

        // ② walk → .walk
        mock.injectActivity(MotionProviderMock.makeActivity(walking: true))
        await Task.yield()
        XCTAssertEqual(viewModel.petState, .walk)

        // ③ downgrade → token=2, petState=.rest
        mock.authorizationStatusStub = .denied
        viewModel.bind(motionProvider: mock)
        XCTAssertEqual(viewModel.petState, .rest)

        // ④ re-grant authorized → 第三次 bind → 升级到新 startUpdates，token=3
        mock.authorizationStatusStub = .authorized
        viewModel.bind(motionProvider: mock)
        XCTAssertEqual(mock.startUpdatesCallCount, 2, "re-grant 后应再次 startUpdates")

        // ⑤ 推 running activity 走新 handler → 新 token 匹配 → petState = .run
        mock.injectActivity(MotionProviderMock.makeActivity(running: true))
        await Task.yield()
        XCTAssertEqual(viewModel.petState, .run, "re-grant 后新 handler callback 应正常 mutate petState")
    }
}
