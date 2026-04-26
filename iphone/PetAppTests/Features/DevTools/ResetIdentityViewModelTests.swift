// ResetIdentityViewModelTests.swift
// Story 2.8 AC9: ResetIdentityViewModel 单元测试。
// happy / error / dismiss 三态 + helper-demo。
//
// import Combine 必显式（lesson 2026-04-25-swift-explicit-import-combine.md）。
// @MainActor 标 class —— ResetIdentityViewModel 是 @MainActor，测试 method 必须在 main actor 跑。

import Combine
import XCTest
@testable import PetApp

#if DEBUG

@MainActor
final class ResetIdentityViewModelTests: XCTestCase {

    /// 本地 mock UseCase，避免依赖 DefaultResetKeychainUseCase + MockKeychainStore 双层 mock 链路；
    /// 直接 stub UseCase 行为更聚焦。
    final class MockResetKeychainUseCase: MockBase, ResetKeychainUseCaseProtocol, @unchecked Sendable {
        var stubError: Error?

        func execute() async throws {
            record(method: "execute()")
            if let e = stubError { throw e }
        }
    }

    var mockUseCase: MockResetKeychainUseCase!
    var sut: ResetIdentityViewModel!

    override func setUp() {
        super.setUp()
        mockUseCase = MockResetKeychainUseCase()
        sut = ResetIdentityViewModel(useCase: mockUseCase)
    }

    override func tearDown() {
        sut = nil
        mockUseCase = nil
        super.tearDown()
    }

    /// happy: useCase 成功 → alertContent == .success
    func testTapHappyPathSetsSuccessAlert() async {
        await sut.tap()

        XCTAssertEqual(sut.alertContent, .success)
        XCTAssertEqual(mockUseCase.callCount(of: "execute()"), 1)
    }

    /// edge: useCase 抛错 → alertContent 为 .failure(message:)，message 含"重置失败" + error description
    func testTapErrorPathSetsFailureAlert() async {
        struct DemoError: Error, LocalizedError, Equatable {
            var errorDescription: String? { "demo failure" }
        }
        mockUseCase.stubError = DemoError()

        await sut.tap()

        guard case .failure(let message) = sut.alertContent else {
            XCTFail("expected .failure alertContent, got \(String(describing: sut.alertContent))")
            return
        }
        XCTAssertTrue(message.contains("重置失败"), "message should contain '重置失败' prefix; got: \(message)")
        XCTAssertTrue(message.contains("demo failure"), "message should contain error description; got: \(message)")
    }

    /// happy: alertDismissed() 把 alertContent 复位为 nil
    func testAlertDismissedClearsAlertContent() async {
        await sut.tap()
        XCTAssertNotNil(sut.alertContent)

        sut.alertDismissed()

        XCTAssertNil(sut.alertContent)
    }

    /// happy: alertDismissed() 在 .failure 之后也能正确清空
    func testAlertDismissedClearsFailureAlert() async {
        struct StubError: Error {}
        mockUseCase.stubError = StubError()

        await sut.tap()
        XCTAssertNotNil(sut.alertContent)

        sut.alertDismissed()

        XCTAssertNil(sut.alertContent)
    }

    /// initial: 新建 ViewModel 后 alertContent 为 nil（默认状态不显 alert）
    func testInitialAlertContentIsNil() {
        XCTAssertNil(sut.alertContent)
    }
}

#endif
