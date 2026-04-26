// ResetKeychainUseCaseTests.swift
// Story 2.8 AC9: ResetKeychainUseCase 单元测试。
// 用 MockKeychainStore（继承 MockBase）记录 invocations + stub 错误。

import XCTest
@testable import PetApp

#if DEBUG

final class ResetKeychainUseCaseTests: XCTestCase {

    var mockKeychainStore: MockKeychainStore!
    var sut: DefaultResetKeychainUseCase!

    override func setUp() {
        super.setUp()
        mockKeychainStore = MockKeychainStore()
        sut = DefaultResetKeychainUseCase(keychainStore: mockKeychainStore)
    }

    override func tearDown() {
        sut = nil
        mockKeychainStore = nil
        super.tearDown()
    }

    /// happy: execute() 调一次 keychainStore.removeAll()。
    func testExecuteCallsRemoveAll() async throws {
        try await sut.execute()

        XCTAssertEqual(mockKeychainStore.callCount(of: "removeAll()"), 1)
        XCTAssertTrue(mockKeychainStore.wasCalled(method: "removeAll()"))
    }

    /// edge: keychainStore 抛错 → execute() 透传同一错。
    func testExecutePropagatesError() async {
        struct StubError: Error, Equatable {}
        mockKeychainStore.removeAllStubError = StubError()

        await assertThrowsAsyncError(try await sut.execute()) { error in
            error is StubError
        }
        // 即使抛错也应记录调用
        XCTAssertEqual(mockKeychainStore.callCount(of: "removeAll()"), 1)
    }

    /// edge: 多次 execute 都调用 removeAll（无短路 / cache）。
    func testExecuteIdempotentMultipleCalls() async throws {
        try await sut.execute()
        try await sut.execute()
        try await sut.execute()

        XCTAssertEqual(mockKeychainStore.callCount(of: "removeAll()"), 3)
    }
}

#endif
