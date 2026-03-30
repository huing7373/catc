import XCTest
@testable import CatShared

final class CatStateTests: XCTestCase {

    // MARK: - All Cases Exist

    func testAllCasesExist() {
        let allCases = CatState.allCases
        XCTAssertEqual(allCases.count, 6)
        XCTAssertTrue(allCases.contains(.idle))
        XCTAssertTrue(allCases.contains(.walking))
        XCTAssertTrue(allCases.contains(.running))
        XCTAssertTrue(allCases.contains(.sleeping))
        XCTAssertTrue(allCases.contains(.microYawn))
        XCTAssertTrue(allCases.contains(.microStretch))
    }

    // MARK: - isMainState

    func testMainStates() {
        XCTAssertTrue(CatState.idle.isMainState)
        XCTAssertTrue(CatState.walking.isMainState)
        XCTAssertTrue(CatState.running.isMainState)
        XCTAssertTrue(CatState.sleeping.isMainState)
    }

    func testMicroBehaviorsAreNotMainState() {
        XCTAssertFalse(CatState.microYawn.isMainState)
        XCTAssertFalse(CatState.microStretch.isMainState)
    }

    // MARK: - isMicroBehavior

    func testIsMicroBehavior() {
        XCTAssertTrue(CatState.microYawn.isMicroBehavior)
        XCTAssertTrue(CatState.microStretch.isMicroBehavior)
    }

    func testMainStatesAreNotMicroBehavior() {
        XCTAssertFalse(CatState.idle.isMicroBehavior)
        XCTAssertFalse(CatState.walking.isMicroBehavior)
        XCTAssertFalse(CatState.running.isMicroBehavior)
        XCTAssertFalse(CatState.sleeping.isMicroBehavior)
    }

    // MARK: - microBehaviorDuration

    func testMicroYawnDuration() {
        XCTAssertEqual(CatState.microYawn.microBehaviorDuration, 2.0)
    }

    func testMicroStretchDuration() {
        XCTAssertEqual(CatState.microStretch.microBehaviorDuration, 3.0)
    }

    func testMainStateDurationsAreZero() {
        for state in CatState.allCases where state.isMainState {
            XCTAssertEqual(state.microBehaviorDuration, 0, "\(state) should have 0 duration")
        }
    }

    // MARK: - Codable

    func testCodableRoundTrip() throws {
        for state in CatState.allCases {
            let data = try JSONEncoder().encode(state)
            let decoded = try JSONDecoder().decode(CatState.self, from: data)
            XCTAssertEqual(decoded, state)
        }
    }

    // MARK: - Raw Value

    func testRawValues() {
        XCTAssertEqual(CatState.idle.rawValue, "idle")
        XCTAssertEqual(CatState.walking.rawValue, "walking")
        XCTAssertEqual(CatState.running.rawValue, "running")
        XCTAssertEqual(CatState.sleeping.rawValue, "sleeping")
        XCTAssertEqual(CatState.microYawn.rawValue, "microYawn")
        XCTAssertEqual(CatState.microStretch.rawValue, "microStretch")
    }
}
