// HomeViewModelLoadHomeTests.swift
// Story 5.5 AC12: HomeViewModel.loadHome / applyHomeData / applyHomeError 路径覆盖（≥ 4 case）.
// Story 37.4 改造（AC8）：domain state 断言从 viewModel.homeData 改为 appState.* 投影
// （HomeViewModel 不再持 homeData 字段；测试通过 viewModel.bind(appState:) 注入 AppState 实例后断言）.
//
// 用 MockLoadHomeUseCase（继承 MockBase + executeStub: Result<HomeData, Error>）.
// ErrorPresenter 注入 ErrorPresenter(toastDuration: 0.05) 加速.

import XCTest
@testable import PetApp

@MainActor
final class HomeViewModelLoadHomeTests: XCTestCase {

    private func makeHomeData(petName: String = "默认小猫") -> HomeData {
        HomeData(
            user: HomeUser(id: "10001", nickname: "u", avatarUrl: ""),
            pet: HomePet(
                id: "20001",
                petType: 1,
                name: petName,
                currentState: .rest,
                equips: []
            ),
            stepAccount: HomeStepAccount(totalSteps: 0, availableSteps: 0, consumedSteps: 0),
            chest: HomeChest(
                id: "30001",
                status: .counting,
                unlockAt: Date(timeIntervalSince1970: 0),
                openCostSteps: 100,
                remainingSeconds: 600
            ),
            room: HomeRoom(currentRoomId: nil)
        )
    }

    // MARK: - case#1 happy: success → AppState 注入 + loadingState=.loaded

    func testLoadHomeSuccessUpdatesState() async {
        let mock = MockLoadHomeUseCase()
        let expectedData = makeHomeData()
        mock.executeStub = .success(expectedData)
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let appState = AppState()
        let viewModel = HomeViewModel(loadHomeUseCase: mock, errorPresenter: presenter)
        viewModel.bind(appState: appState)

        await viewModel.loadHome()

        XCTAssertEqual(appState.currentUser, expectedData.user)
        XCTAssertEqual(appState.currentPet, expectedData.pet)
        XCTAssertEqual(appState.currentStepAccount, expectedData.stepAccount)
        XCTAssertEqual(appState.currentChest, expectedData.chest)
        XCTAssertEqual(appState.currentRoomId, expectedData.room.currentRoomId)
        XCTAssertEqual(viewModel.loadingState, .loaded)
        XCTAssertEqual(mock.callCount(of: "execute()"), 1)
    }

    // MARK: - case#2 edge: network failure → loadingState=.failed + presenter 弹 retry

    /// AppErrorMapper 把 .network → .retry 呈现态；其他错误（business/decoding/unauthorized 等）走 .alert.
    /// 本 case 验证 .network 路径走完 ViewModel.applyHomeError → ErrorPresenter.present 后能弹出 RetryView.
    func testLoadHomeNetworkFailureUpdatesStateAndPresentsRetry() async {
        let mock = MockLoadHomeUseCase()
        mock.executeStub = .failure(APIError.network(underlying: URLError(.timedOut)))
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let viewModel = HomeViewModel(loadHomeUseCase: mock, errorPresenter: presenter)

        await viewModel.loadHome()

        if case .failed = viewModel.loadingState {
            // pass
        } else {
            XCTFail("loadingState 应为 .failed，实际 \(viewModel.loadingState)")
        }

        // ErrorPresenter.current 应为 .retry 类型呈现项
        if case .retry = presenter.current {
            // pass
        } else {
            XCTFail("ErrorPresenter.current 应为 .retry 呈现项，实际 \(String(describing: presenter.current))")
        }
    }

    /// case#2b: permanent business error → loadingState=.failed + presenter 弹 alert（非 retry）.
    /// 验证 ViewModel 透传错误给 mapper，由 mapper 决定呈现样式（不在 ViewModel 内做错误码翻译）.
    /// **Story 5.5 round 5 [P1] fix**: 用 4002 (permanent) 而非 1009 (transient → .retry).
    /// transient 业务码（1005/1007/1008/1009）改派 .retry 后,本测试改用 permanent 码做 .alert 验证;
    /// 另增 case#2c 单测覆盖 transient 业务码 → .retry 路径.
    func testLoadHomeBusinessFailureUpdatesStateAndPresentsAlert() async {
        let mock = MockLoadHomeUseCase()
        mock.executeStub = .failure(APIError.business(code: 4002, message: "宝箱尚未解锁", requestId: "req_x"))
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let viewModel = HomeViewModel(loadHomeUseCase: mock, errorPresenter: presenter)

        await viewModel.loadHome()

        if case .failed = viewModel.loadingState {} else {
            XCTFail("loadingState 应为 .failed，实际 \(viewModel.loadingState)")
        }
        if case .alert = presenter.current {
            // pass —— permanent business 走 alert
        } else {
            XCTFail("permanent business 错误应弹 .alert，实际 \(String(describing: presenter.current))")
        }
    }

    /// case#2c (Story 5.5 round 5 [P1] fix): transient business error → .retry.
    /// 1009 (服务繁忙) 是 transient 类,mapper 改派 .retry —— ViewModel 仍透传给 mapper,
    /// 让 ErrorPresenter 渲染 RetryView 让用户重试.
    func testLoadHomeTransientBusinessFailurePresentsRetry() async {
        let mock = MockLoadHomeUseCase()
        mock.executeStub = .failure(APIError.business(code: 1009, message: "服务繁忙", requestId: "req_x"))
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let viewModel = HomeViewModel(loadHomeUseCase: mock, errorPresenter: presenter)

        await viewModel.loadHome()

        if case .failed = viewModel.loadingState {} else {
            XCTFail("loadingState 应为 .failed，实际 \(viewModel.loadingState)")
        }
        if case .retry = presenter.current {
            // pass —— transient business (1009) 走 retry
        } else {
            XCTFail("transient business 错误应弹 .retry，实际 \(String(describing: presenter.current))")
        }
    }

    // MARK: - case#3 happy: 重复 loadHome 短路（hasLoadedHome）

    func testLoadHomeShortCircuitsAfterFirstSuccess() async {
        let mock = MockLoadHomeUseCase()
        mock.executeStub = .success(makeHomeData())
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let viewModel = HomeViewModel(loadHomeUseCase: mock, errorPresenter: presenter)

        await viewModel.loadHome()
        XCTAssertEqual(mock.callCount(of: "execute()"), 1)

        // 再调一次 → 短路（hasLoadedHome=true 已置）
        await viewModel.loadHome()
        XCTAssertEqual(mock.callCount(of: "execute()"), 1, "已成功一次后应短路，不重发请求")
    }

    // MARK: - case#4 happy: 重试 — 失败 → reset → 再 loadHome 成功

    func testLoadHomeRetryAfterFailure() async {
        let mock = MockLoadHomeUseCase()
        mock.executeStub = .failure(APIError.network(underlying: URLError(.timedOut)))
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let appState = AppState()
        let viewModel = HomeViewModel(loadHomeUseCase: mock, errorPresenter: presenter)
        viewModel.bind(appState: appState)

        await viewModel.loadHome()
        if case .failed = viewModel.loadingState {} else {
            XCTFail("第一次应失败")
        }
        XCTAssertEqual(mock.callCount(of: "execute()"), 1)

        // 用户重试：清 flag → 改 stub → 再调 loadHome
        viewModel.resetLoadHomeForRetry()
        XCTAssertEqual(viewModel.loadingState, .idle)
        let expectedData = makeHomeData(petName: "重试后猫")
        mock.executeStub = .success(expectedData)

        await viewModel.loadHome()

        XCTAssertEqual(viewModel.loadingState, .loaded)
        XCTAssertEqual(appState.currentPet?.name, "重试后猫")
        XCTAssertEqual(mock.callCount(of: "execute()"), 2)
    }

    // MARK: - case#5 edge: 未注入 UseCase → loadHome no-op

    func testLoadHomeIsNoOpWhenUseCaseNotInjected() async {
        let appState = AppState()
        let viewModel = HomeViewModel()  // 老 init，loadHomeUseCase=nil + errorPresenter=nil
        viewModel.bind(appState: appState)

        await viewModel.loadHome()

        // loadHome 未注入 UseCase 直接 no-op，AppState 应保持空态
        XCTAssertNil(appState.currentUser)
        XCTAssertNil(appState.currentPet)
        XCTAssertEqual(viewModel.loadingState, .idle)
    }

    // MARK: - case#6 happy: applyHomeData 直接注入 → 写 AppState + 后续 loadHome 短路

    func testApplyHomeDataInjectsAndShortCircuits() async {
        let mock = MockLoadHomeUseCase()
        mock.executeStub = .success(makeHomeData(petName: "stub"))
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let appState = AppState()
        let viewModel = HomeViewModel(loadHomeUseCase: mock, errorPresenter: presenter)
        viewModel.bind(appState: appState)

        let injectedData = makeHomeData(petName: "applyHomeData injected")
        viewModel.applyHomeData(injectedData)

        XCTAssertEqual(appState.currentPet?.name, "applyHomeData injected")
        XCTAssertEqual(appState.currentUser, injectedData.user)
        XCTAssertEqual(viewModel.loadingState, .loaded)

        // 之后调 loadHome → 因 hasLoadedHome=true 应短路（mock 不被调用）
        await viewModel.loadHome()
        XCTAssertEqual(mock.callCount(of: "execute()"), 0,
                       "applyHomeData 之后 loadHome 应短路，UseCase 不应被调用")
    }

    // MARK: - case#7 happy: bind 路径 + loadHome（生产路径模拟）

    func testBindThenLoadHomeWorks() async {
        let mock = MockLoadHomeUseCase()
        mock.executeStub = .success(makeHomeData(petName: "bound"))
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let appState = AppState()
        let viewModel = HomeViewModel()  // 老 init，无 init 路径注入

        viewModel.bind(loadHomeUseCase: mock, errorPresenter: presenter)
        viewModel.bind(appState: appState)
        await viewModel.loadHome()

        XCTAssertEqual(appState.currentPet?.name, "bound")
        XCTAssertEqual(viewModel.loadingState, .loaded)
        XCTAssertEqual(mock.callCount(of: "execute()"), 1)
    }

    // MARK: - case#8 regress: init 路径注入 fresh AppState() → 仍能写 AppState（codex round 1 [P2] regress）

    /// 回归测试：`HomeViewModel(appState: AppState())` 这种 caller 不在外部留 strong owner 的路径,
    /// applyHomeData 必须能写到注入的 AppState. 旧实现里 `private weak var appState` 会让这个
    /// fresh AppState 立刻被释放,断言会全部 fail（appState.currentPet == nil）.
    /// 修为 strong 后该 case 通过. 见 docs/lessons/2026-04-30-strong-vs-weak-for-constructor-injected-state.md.
    func testInitInjectionWithFreshAppStateRetainsReference() {
        // 注意：故意**不**在测试 stack 上留 `let appState = AppState()` 引用 ——
        // 否则 weak 实现也会过测（外部 strong owner 把 appState 保活）.
        // 通过 viewModel 暴露的写入路径 + observable 字段反查 ViewModel 持有的 appState 引用是否仍活.
        let viewModel = HomeViewModel(
            nickname: "u",
            appVersion: "0.0.0",
            serverInfo: "----",
            appState: AppState()
        )
        let injectedData = makeHomeData(petName: "fresh-init-injection")

        viewModel.applyHomeData(injectedData)

        // ViewModel 上的 transient flag 仍可断言（不依赖 appState 是否活着）.
        XCTAssertEqual(viewModel.loadingState, .loaded)

        // 真正的回归断言：让 viewModel 重置 short-circuit flag 后再次 applyHomeData,
        // 若 appState 在第一次 apply 后没释放（strong 持有正确）,赋值仍然生效.
        // 用 mirror 反射拿 viewModel.appState 引用做 nil 断言（比起 appState.currentPet 更直接）.
        let mirror = Mirror(reflecting: viewModel)
        let appStateChild = mirror.children.first { $0.label == "appState" }
        XCTAssertNotNil(appStateChild, "HomeViewModel 应有 appState 字段")
        // Optional<AppState> 通过 mirror 取出后是 Optional.some(AppState) 或 Optional.none
        if let value = appStateChild?.value {
            // 反射后得到的 Optional 值用 displayStyle 判别非 nil
            let valueMirror = Mirror(reflecting: value)
            XCTAssertEqual(valueMirror.displayStyle, .optional)
            XCTAssertNotNil(
                valueMirror.children.first?.value,
                "init 注入的 fresh AppState 必须仍被 ViewModel 持有（strong）；旧 weak 实现这里会是 nil"
            )
        } else {
            XCTFail("appState 字段反射失败")
        }
    }
}
