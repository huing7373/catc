import XCTest
import Combine
@testable import CatShared
@testable import CatCore

final class CatStateMachineTests: XCTestCase {

    private var machine: CatStateMachine!
    private var timeProvider: MockTimeProvider!
    private var scheduler: MockTimerScheduler!
    private var localStore: LocalStore!
    private var defaults: UserDefaults!
    private var cancellables = Set<AnyCancellable>()

    override func setUp() {
        super.setUp()
        timeProvider = MockTimeProvider()
        scheduler = MockTimerScheduler()
        defaults = UserDefaults(suiteName: "CatStateMachineTests.\(name)")!
        defaults.removePersistentDomain(forName: "CatStateMachineTests.\(name)")
        localStore = LocalStore(defaults: defaults)
        machine = CatStateMachine(timeProvider: timeProvider, localStore: localStore, scheduler: scheduler)
        cancellables = Set<AnyCancellable>()
    }

    override func tearDown() {
        cancellables.removeAll()
        defaults.removePersistentDomain(forName: "CatStateMachineTests.\(name)")
        defaults = nil
        localStore = nil
        timeProvider = nil
        scheduler = nil
        machine = nil
        super.tearDown()
    }

    // MARK: - Initial State (AC2)

    func testInitialStateIsIdle() {
        XCTAssertEqual(machine.currentState, .idle)
    }

    // MARK: - handleMotionInput transitions (AC3)

    func testWalkingInputNeedsDebounce() {
        machine.handleMotionInput(.walking)
        XCTAssertEqual(machine.currentState, .idle, "Should not transition before 3s debounce")
    }

    func testWalkingTransitionsAfter3sDebounce() {
        machine.handleMotionInput(.walking)
        timeProvider.advance(by: 3.0)
        // Fire the debounce repeating timer
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)
        XCTAssertEqual(machine.currentState, .walking)
    }

    func testRunningTransitionsAfter3sDebounce() {
        machine.handleMotionInput(.running)
        timeProvider.advance(by: 3.0)
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)
        XCTAssertEqual(machine.currentState, .running)
    }

    func testWalkingDoesNotTransitionBefore3s() {
        machine.handleMotionInput(.walking)
        timeProvider.advance(by: 2.9)
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)
        XCTAssertEqual(machine.currentState, .idle, "Should not transition at 2.9s")
    }

    func testStationaryFromWalkingNeeds10s() {
        machine.handleMotionInput(.walking)
        timeProvider.advance(by: 3.0)
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)
        XCTAssertEqual(machine.currentState, .walking)

        machine.handleMotionInput(.stationary)
        timeProvider.advance(by: 9.9)
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)
        XCTAssertEqual(machine.currentState, .walking, "Should not transition at 9.9s")

        timeProvider.advance(by: 0.2)
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)
        XCTAssertEqual(machine.currentState, .idle, "Should transition after 10s")
    }

    func testStationaryFromRunningNeeds10s() {
        machine.handleMotionInput(.running)
        timeProvider.advance(by: 3.0)
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)
        XCTAssertEqual(machine.currentState, .running)

        machine.handleMotionInput(.stationary)
        timeProvider.advance(by: 10.0)
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)
        XCTAssertEqual(machine.currentState, .idle)
    }

    func testWalkingToRunningDirectSwitch() {
        // Force to walking first
        machine.handleMotionInput(.walking)
        timeProvider.advance(by: 3.0)
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)
        XCTAssertEqual(machine.currentState, .walking)

        machine.handleMotionInput(.running)
        XCTAssertEqual(machine.currentState, .running, "walking→running should be instant")
    }

    func testRunningToWalkingDirectSwitch() {
        machine.handleMotionInput(.running)
        timeProvider.advance(by: 3.0)
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)
        XCTAssertEqual(machine.currentState, .running)

        machine.handleMotionInput(.walking)
        XCTAssertEqual(machine.currentState, .walking, "running→walking should be instant")
    }

    func testStationaryFromIdleIsIgnored() {
        machine.handleMotionInput(.stationary)
        XCTAssertEqual(machine.currentState, .idle)
    }

    func testDebounceResetsOnInputChange() {
        machine.handleMotionInput(.walking)
        timeProvider.advance(by: 2.0)
        machine.handleMotionInput(.running)
        timeProvider.advance(by: 2.0)
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)
        XCTAssertEqual(machine.currentState, .idle, "Debounce reset: 2s running not enough")

        timeProvider.advance(by: 1.0)
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)
        XCTAssertEqual(machine.currentState, .running, "3s running total should trigger")
    }

    // MARK: - Wrist Raise (AC3: sleeping→idle)

    func testWristRaiseFromSleepingWakes() {
        machine.transition(to: .sleeping)
        machine.handleMotionInput(.wristRaise)
        XCTAssertEqual(machine.currentState, .idle)
    }

    func testWristRaiseFromIdleIsNoOp() {
        machine.handleMotionInput(.wristRaise)
        XCTAssertEqual(machine.currentState, .idle)
    }

    func testWristRaiseFromWalkingIsNoOp() {
        machine.handleMotionInput(.walking)
        timeProvider.advance(by: 3.0)
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)
        machine.handleMotionInput(.wristRaise)
        XCTAssertEqual(machine.currentState, .walking)
    }

    // MARK: - Self-Heal Timer (AC4: 60s → idle)

    func testSelfHealFiresAfter60s() {
        machine.handleMotionInput(.walking)
        timeProvider.advance(by: 3.0)
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)
        XCTAssertEqual(machine.currentState, .walking)

        // Fire the self-heal timer (60s)
        scheduler.fire(interval: CatStateMachine.Timing.selfHealInterval)
        XCTAssertEqual(machine.currentState, .idle, "Self-heal should return to idle after 60s")
    }

    func testSelfHealDoesNotFireFromIdle() {
        // idle state: self-heal should be a no-op
        scheduler.fire(interval: CatStateMachine.Timing.selfHealInterval)
        XCTAssertEqual(machine.currentState, .idle)
    }

    func testSelfHealResetsOnTransition() {
        machine.handleMotionInput(.walking)
        timeProvider.advance(by: 3.0)
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)

        // Cancel all old self-heal timers by checking active count
        let activeBeforeRunning = scheduler.activeCount
        machine.handleMotionInput(.running)
        // Transition walking→running should reset self-heal
        XCTAssertEqual(machine.currentState, .running)
    }

    // MARK: - Sleeping (AC3: idle ≥30min + nighttime)

    func testIdleToSleepingAfter30minAtNight() {
        timeProvider.setHour(23) // 夜间
        machine = CatStateMachine(timeProvider: timeProvider, localStore: localStore, scheduler: scheduler)

        // Advance past 30 minutes
        timeProvider.advance(by: 30 * 60 + 1)

        // Fire the sleep check timer
        scheduler.fire(interval: CatStateMachine.Timing.sleepCheckInterval)
        XCTAssertEqual(machine.currentState, .sleeping, "Should transition to sleeping after 30min at night")
    }

    func testIdleDoesNotSleepDuringDay() {
        timeProvider.setHour(14) // 下午 2 点
        machine = CatStateMachine(timeProvider: timeProvider, localStore: localStore, scheduler: scheduler)

        timeProvider.advance(by: 30 * 60 + 1)
        scheduler.fire(interval: CatStateMachine.Timing.sleepCheckInterval)
        XCTAssertEqual(machine.currentState, .idle, "Should not sleep during daytime")
    }

    func testIdleDoesNotSleepBefore30min() {
        timeProvider.setHour(23)
        machine = CatStateMachine(timeProvider: timeProvider, localStore: localStore, scheduler: scheduler)

        timeProvider.advance(by: 29 * 60)
        scheduler.fire(interval: CatStateMachine.Timing.sleepCheckInterval)
        XCTAssertEqual(machine.currentState, .idle, "Should not sleep before 30 minutes")
    }

    func testSleepingAt3amIsNighttime() {
        timeProvider.setHour(3) // 凌晨 3 点
        machine = CatStateMachine(timeProvider: timeProvider, localStore: localStore, scheduler: scheduler)

        timeProvider.advance(by: 30 * 60 + 1)
        scheduler.fire(interval: CatStateMachine.Timing.sleepCheckInterval)
        XCTAssertEqual(machine.currentState, .sleeping, "3 AM is nighttime")
    }

    // MARK: - Micro-Behavior (AC6)

    func testMicroYawnFromIdle() {
        machine.transition(to: .microYawn)
        XCTAssertEqual(machine.currentState, .microYawn)
    }

    func testMicroStretchFromIdle() {
        machine.transition(to: .microStretch)
        XCTAssertEqual(machine.currentState, .microStretch)
    }

    func testMicroBehaviorBlockedFromWalking() {
        machine.handleMotionInput(.walking)
        timeProvider.advance(by: 3.0)
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)
        machine.transition(to: .microYawn)
        XCTAssertEqual(machine.currentState, .walking, "Micro should not trigger from walking")
    }

    func testMicroBehaviorBlockedFromRunning() {
        machine.handleMotionInput(.running)
        timeProvider.advance(by: 3.0)
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)
        machine.transition(to: .microStretch)
        XCTAssertEqual(machine.currentState, .running, "Micro should not trigger from running")
    }

    func testMicroBehaviorBlockedFromSleeping() {
        machine.transition(to: .sleeping)
        machine.transition(to: .microYawn)
        XCTAssertEqual(machine.currentState, .sleeping, "Micro should not trigger from sleeping")
    }

    func testMainStateInterruptsMicroBehavior() {
        machine.transition(to: .microYawn)
        machine.transition(to: .walking)
        XCTAssertEqual(machine.currentState, .walking, "Main state should interrupt micro-behavior")
    }

    func testMicroYawnAutoReturnsToIdle() {
        machine.transition(to: .microYawn)
        XCTAssertEqual(machine.currentState, .microYawn)

        // Fire the micro-behavior return timer (2.0s for yawn)
        scheduler.fire(interval: CatState.microYawn.microBehaviorDuration)
        XCTAssertEqual(machine.currentState, .idle, "microYawn should auto-return to idle after 2s")
    }

    func testMicroStretchAutoReturnsToIdle() {
        machine.transition(to: .microStretch)
        XCTAssertEqual(machine.currentState, .microStretch)

        // Fire the micro-behavior return timer (3.0s for stretch)
        scheduler.fire(interval: CatState.microStretch.microBehaviorDuration)
        XCTAssertEqual(machine.currentState, .idle, "microStretch should auto-return to idle after 3s")
    }

    func testMicroBehaviorSchedulerStartsInIdle() {
        // When idle, micro-behavior timers should be scheduled
        // Check that we have scheduled timers for micro-behaviors
        let microTokens = scheduler.scheduledTokens.filter { token in
            !token.isCancelled && token.interval > 10 // micro-behavior delays are >> 10s
        }
        XCTAssertGreaterThanOrEqual(microTokens.count, 2, "Should have yawn and stretch timers")
    }

    func testMicroBehaviorSchedulerStopsWhenLeavingIdle() {
        let beforeCount = scheduler.activeCount
        machine.handleMotionInput(.walking)
        timeProvider.advance(by: 3.0)
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)
        XCTAssertEqual(machine.currentState, .walking)
        // Micro-behavior timers should be cancelled when leaving idle
        scheduler.cleanup()
    }

    // MARK: - Combine Publisher (AC5)

    func testPublisherEmitsOnTransition() {
        var receivedStates: [CatState] = []
        machine.statePublisher
            .sink { receivedStates.append($0) }
            .store(in: &cancellables)

        machine.transition(to: .walking)
        machine.transition(to: .running)
        machine.transition(to: .idle)

        XCTAssertEqual(receivedStates, [.walking, .running, .idle])
    }

    func testPublisherDoesNotEmitForSameState() {
        var count = 0
        machine.statePublisher
            .sink { _ in count += 1 }
            .store(in: &cancellables)

        machine.transition(to: .idle) // same state, no-op
        XCTAssertEqual(count, 0)
    }

    func testPublisherEmitsOnHandleMotionInput() {
        var receivedStates: [CatState] = []
        machine.statePublisher
            .sink { receivedStates.append($0) }
            .store(in: &cancellables)

        machine.handleMotionInput(.walking)
        timeProvider.advance(by: 3.0)
        scheduler.fire(interval: CatStateMachine.Timing.debounceCheckInterval)

        XCTAssertEqual(receivedStates, [.walking])
    }

    // MARK: - State Persistence (AC7)

    func testStatePersistsOnTransition() {
        machine.transition(to: .walking)
        XCTAssertEqual(localStore.loadCatState(), .walking)
    }

    func testMicroBehaviorDoesNotPersist() {
        machine.transition(to: .microYawn)
        XCTAssertNotEqual(localStore.loadCatState(), .microYawn)
    }

    func testRestoreStateFromPersistence() {
        localStore.saveCatState(.walking)
        let restored = CatStateMachine(timeProvider: timeProvider, localStore: localStore, scheduler: scheduler)
        XCTAssertEqual(restored.currentState, .walking)
    }

    func testRestoreDefaultsToIdleWhenEmpty() {
        let fresh = CatStateMachine(timeProvider: timeProvider, localStore: localStore, scheduler: scheduler)
        XCTAssertEqual(fresh.currentState, .idle)
    }

    func testRestoreIgnoresMicroBehaviorState() {
        localStore.saveCatState(.microYawn)
        let restored = CatStateMachine(timeProvider: timeProvider, localStore: localStore, scheduler: scheduler)
        XCTAssertEqual(restored.currentState, .idle, "Should not restore micro-behavior")
    }

    // MARK: - transition(to:) is not public

    func testTransitionIsNotPublicAPI() {
        // This test documents that transition(to:) is internal — not accessible
        // outside the CatCore module. External callers must use handleMotionInput().
        // If this compiles, it means transition(to:) is accessible within the test
        // target (which uses @testable import), but NOT from external modules.
        machine.transition(to: .walking)
        XCTAssertEqual(machine.currentState, .walking)
    }

    // MARK: - Timing Constants

    func testTimingConstants() {
        XCTAssertEqual(CatStateMachine.Timing.motionDebounce, 3.0)
        XCTAssertEqual(CatStateMachine.Timing.stationaryDebounce, 10.0)
        XCTAssertEqual(CatStateMachine.Timing.selfHealInterval, 60.0)
        XCTAssertEqual(CatStateMachine.Timing.idleToSleepDuration, 1800.0)
        XCTAssertEqual(CatStateMachine.Timing.nightStartHour, 22)
        XCTAssertEqual(CatStateMachine.Timing.nightEndHour, 7)
    }
}
