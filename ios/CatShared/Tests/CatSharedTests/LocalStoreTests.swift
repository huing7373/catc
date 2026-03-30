import XCTest
@testable import CatShared

final class LocalStoreTests: XCTestCase {

    private var store: LocalStore!
    private var defaults: UserDefaults!

    override func setUp() {
        super.setUp()
        defaults = UserDefaults(suiteName: "CatSharedTests.\(name)")!
        defaults.removePersistentDomain(forName: "CatSharedTests.\(name)")
        store = LocalStore(defaults: defaults)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: "CatSharedTests.\(name)")
        defaults = nil
        store = nil
        super.tearDown()
    }

    // MARK: - Save & Load Cat State

    func testSaveAndLoadCatState() {
        store.saveCatState(.walking)
        XCTAssertEqual(store.loadCatState(), .walking)
    }

    func testLoadCatStateReturnsNilWhenEmpty() {
        XCTAssertNil(store.loadCatState())
    }

    func testSaveOverwritesPreviousState() {
        store.saveCatState(.walking)
        store.saveCatState(.sleeping)
        XCTAssertEqual(store.loadCatState(), .sleeping)
    }

    func testSaveAllMainStates() {
        let mainStates: [CatState] = [.idle, .walking, .running, .sleeping]
        for state in mainStates {
            store.saveCatState(state)
            XCTAssertEqual(store.loadCatState(), state, "Failed for \(state)")
        }
    }

    func testSaveMicroBehaviorStates() {
        store.saveCatState(.microYawn)
        XCTAssertEqual(store.loadCatState(), .microYawn)
    }

    // MARK: - Timestamp

    func testTimestampSavedWithState() {
        let before = Date()
        store.saveCatState(.idle)
        let after = Date()

        let ts = store.loadCatStateTimestamp()
        XCTAssertNotNil(ts)
        // Allow 1 second tolerance
        XCTAssertGreaterThanOrEqual(ts!.timeIntervalSince1970, before.timeIntervalSince1970 - 1)
        XCTAssertLessThanOrEqual(ts!.timeIntervalSince1970, after.timeIntervalSince1970 + 1)
    }

    func testTimestampReturnsNilWhenEmpty() {
        XCTAssertNil(store.loadCatStateTimestamp())
    }
}
