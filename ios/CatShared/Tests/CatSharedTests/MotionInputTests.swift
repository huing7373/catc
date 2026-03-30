import XCTest
@testable import CatShared

final class MotionInputTests: XCTestCase {

    func testAllCasesRawValues() {
        XCTAssertEqual(MotionInput.stationary.rawValue, "stationary")
        XCTAssertEqual(MotionInput.walking.rawValue, "walking")
        XCTAssertEqual(MotionInput.running.rawValue, "running")
        XCTAssertEqual(MotionInput.wristRaise.rawValue, "wristRaise")
    }

    func testCodableRoundTrip() throws {
        let inputs: [MotionInput] = [.stationary, .walking, .running, .wristRaise]
        for input in inputs {
            let data = try JSONEncoder().encode(input)
            let decoded = try JSONDecoder().decode(MotionInput.self, from: data)
            XCTAssertEqual(decoded, input)
        }
    }
}
