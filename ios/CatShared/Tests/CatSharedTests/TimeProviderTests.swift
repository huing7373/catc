import XCTest
@testable import CatShared

final class TimeProviderTests: XCTestCase {

    func testSystemTimeProviderReturnsCurrentTime() {
        let provider = SystemTimeProvider()
        let before = Date()
        let result = provider.now
        let after = Date()
        XCTAssertGreaterThanOrEqual(result, before)
        XCTAssertLessThanOrEqual(result, after)
    }
}
