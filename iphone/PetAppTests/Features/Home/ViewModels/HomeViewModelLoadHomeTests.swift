// HomeViewModelLoadHomeTests.swift
// Story 5.5 AC12: HomeViewModel.loadHome / applyHomeData / applyHomeError 路径覆盖（≥ 4 case）.
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

    // MARK: - case#1 happy: success → homeData 注入 + loadingState=.loaded

    func testLoadHomeSuccessUpdatesState() async {
        let mock = MockLoadHomeUseCase()
        let expectedData = makeHomeData()
        mock.executeStub = .success(expectedData)
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let viewModel = HomeViewModel(loadHomeUseCase: mock, errorPresenter: presenter)

        await viewModel.loadHome()

        XCTAssertEqual(viewModel.homeData, expectedData)
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
        let viewModel = HomeViewModel(loadHomeUseCase: mock, errorPresenter: presenter)

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
        XCTAssertEqual(viewModel.homeData?.pet?.name, "重试后猫")
        XCTAssertEqual(mock.callCount(of: "execute()"), 2)
    }

    // MARK: - case#5 edge: 未注入 UseCase → loadHome no-op

    func testLoadHomeIsNoOpWhenUseCaseNotInjected() async {
        let viewModel = HomeViewModel()  // 老 init，loadHomeUseCase=nil + errorPresenter=nil

        await viewModel.loadHome()

        XCTAssertNil(viewModel.homeData)
        XCTAssertEqual(viewModel.loadingState, .idle)
    }

    // MARK: - case#6 happy: applyHomeData 直接注入 → 后续 loadHome 短路

    func testApplyHomeDataInjectsAndShortCircuits() async {
        let mock = MockLoadHomeUseCase()
        mock.executeStub = .success(makeHomeData(petName: "stub"))
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let viewModel = HomeViewModel(loadHomeUseCase: mock, errorPresenter: presenter)

        let injectedData = makeHomeData(petName: "applyHomeData injected")
        viewModel.applyHomeData(injectedData)

        XCTAssertEqual(viewModel.homeData, injectedData)
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
        let viewModel = HomeViewModel()  // 老 init，无 init 路径注入

        viewModel.bind(loadHomeUseCase: mock, errorPresenter: presenter)
        await viewModel.loadHome()

        XCTAssertEqual(viewModel.homeData?.pet?.name, "bound")
        XCTAssertEqual(viewModel.loadingState, .loaded)
        XCTAssertEqual(mock.callCount(of: "execute()"), 1)
    }
}
