import SwiftUI
import CoreMotion
import Combine
import UserNotifications
import WatchKit
import UIKit
import CatShared
import CatCore

@main
struct CatWatchApp: App {
    var body: some Scene {
        WindowGroup {
            WatchHomeView()
        }
    }
}

struct WatchHomeView: View {
    @Environment(\.scenePhase) private var scenePhase
    @StateObject private var controller = WatchMotionController()
    @StateObject private var reminderManager = StandReminderManager()
    @StateObject private var blindBoxManager = BlindBoxManager()
    @StateObject private var friendStore: FriendCatStore
    @StateObject private var syncCoordinator: WatchSyncCoordinator
    private let isPreview = ProcessInfo.processInfo.environment["XCODE_RUNNING_FOR_PREVIEWS"] == "1"

    init() {
        let config = BackendConfig.current
        let friendStore = FriendCatStore(localUserID: config.debugToken)
        _friendStore = StateObject(wrappedValue: friendStore)
        _syncCoordinator = StateObject(wrappedValue: WatchSyncCoordinator(config: config, friendStore: friendStore))
    }

    var body: some View {
        GeometryReader { proxy in
            let compact = proxy.size.width < 170
            let hasFriends = !friendStore.topThreeFriends.isEmpty

            ZStack {
                LinearGradient(
                    colors: [
                        Color(red: 0.06, green: 0.07, blue: 0.09),
                        Color(red: 0.12, green: 0.13, blue: 0.16)
                    ],
                    startPoint: .topLeading,
                    endPoint: .bottomTrailing
                )
                .ignoresSafeArea()

                VStack(spacing: compact ? 6 : 10) {
                    RoomCatGridView(
                        localState: controller.currentState,
                        friendCats: friendStore.topThreeFriends,
                        blindBox: blindBoxManager.currentBlindBox,
                        spendableSteps: blindBoxManager.spendableSteps,
                        dropCycleStartDate: blindBoxManager.dropCycleStartDate,
                        compact: compact,
                        openBlindBox: {
                            blindBoxManager.openBlindBox()
                        }
                    )

                    if !hasFriends {
                        VStack(spacing: compact ? 3 : 4) {
                            Text(stateSubtitle(controller.currentState))
                                .font(.system(size: compact ? 11 : 12, weight: .medium, design: .rounded))
                                .foregroundStyle(.white.opacity(0.82))
                                .multilineTextAlignment(.center)

                            Text("当前点数 \(blindBoxManager.spendableSteps) 点")
                                .font(.system(size: compact ? 10 : 11, weight: .semibold, design: .rounded))
                                .foregroundStyle(.white.opacity(0.72))
                                .multilineTextAlignment(.center)
                                .lineLimit(1)
                                .minimumScaleFactor(0.8)

                            Text(controller.sensorStatus)
                                .font(.system(size: compact ? 9 : 10, weight: .medium, design: .rounded))
                                .foregroundStyle(.white.opacity(0.55))
                                .multilineTextAlignment(.center)
                                .lineLimit(1)
                                .minimumScaleFactor(0.75)

                            Text(syncCoordinator.statusText)
                                .font(.system(size: compact ? 9 : 10, weight: .medium, design: .rounded))
                                .foregroundStyle(syncCoordinator.isHealthy ? .green.opacity(0.8) : .white.opacity(0.5))
                                .multilineTextAlignment(.center)
                                .lineLimit(1)
                                .minimumScaleFactor(0.75)
                        }
                        .padding(.horizontal, 10)
                    }
                }
                .padding(compact ? 10 : 16)
                .offset(y: hasFriends ? (compact ? -8 : -4) : 0)

                if let reminderMessage = reminderManager.foregroundBannerText {
                    VStack {
                        Text(reminderMessage)
                            .font(.system(size: compact ? 10 : 11, weight: .semibold, design: .rounded))
                            .foregroundStyle(.white)
                            .multilineTextAlignment(.center)
                            .padding(.horizontal, 12)
                            .padding(.vertical, 8)
                            .background(
                                Capsule()
                                    .fill(Color.black.opacity(0.72))
                            )
                            .overlay(
                                Capsule()
                                    .stroke(Color.white.opacity(0.12), lineWidth: 1)
                            )
                            .shadow(color: .black.opacity(0.22), radius: 12, y: 6)
                            .padding(.top, 8)

                        Spacer()
                    }
                    .padding(.horizontal, 12)
                    .transition(.move(edge: .top).combined(with: .opacity))
                }
            }
            .frame(width: proxy.size.width, height: proxy.size.height, alignment: .center)
            .overlay(alignment: .bottomLeading) {
                #if DEBUG
                Button {
                    controller.injectDebugWalkAndSteps()
                } label: {
                    Text("+50")
                        .font(.system(size: 11, weight: .black, design: .rounded))
                        .foregroundStyle(.white)
                        .frame(width: 42, height: 30)
                        .background(
                            Capsule()
                                .fill(Color.black.opacity(0.80))
                        )
                        .overlay(
                            Capsule()
                                .stroke(Color.white.opacity(0.28), lineWidth: 1)
                        )
                }
                .buttonStyle(.plain)
                .shadow(color: .black.opacity(0.18), radius: 4, y: 2)
                .padding(.leading, 10)
                .padding(.bottom, 12)
                .zIndex(999)
                #endif
            }
            .animation(.spring(response: 0.32, dampingFraction: 0.82), value: reminderManager.foregroundBannerText)
            .task {
                guard !isPreview else { return }
                controller.onMotionInputChanged = { input in
                    reminderManager.registerMotion(input)
                }
                controller.onStateChanged = { state in
                    reminderManager.registerCatState(state)
                    blindBoxManager.registerCatState(state)
                    syncCoordinator.handleLocalStateChange(state)
                }
                controller.onTodayStepsChanged = { steps in
                    reminderManager.registerTodaySteps(steps)
                    blindBoxManager.registerTodaySteps(steps)
                }
                controller.start()
                reminderManager.start()
                blindBoxManager.start()
                syncCoordinator.start()
            }
            .onChange(of: scenePhase) { _, newPhase in
                guard !isPreview else { return }
                if newPhase == .active {
                    controller.handleWristRaise()
                    reminderManager.handleScenePhaseChange(.active)
                    blindBoxManager.handleScenePhaseChange(.active)
                    syncCoordinator.start()
                } else {
                    reminderManager.handleScenePhaseChange(newPhase)
                    blindBoxManager.handleScenePhaseChange(newPhase)
                }
            }
        }
    }

    private func stateSubtitle(_ state: CatState) -> String {
        switch state {
        case .idle:
            return "idle"
        case .walking:
            return "walking"
        case .running:
            return "running"
        case .sleeping:
            return "sleeping"
        case .microYawn:
            return "idle"
        case .microStretch:
            return "idle"
        }
    }
}

#Preview("Watch Home") {
    WatchHomeView()
}

private struct RoomCatGridView: View {
    let localState: CatState
    let friendCats: [FriendCatPresence]
    let blindBox: BlindBoxStatus?
    let spendableSteps: Int
    let dropCycleStartDate: Date
    let compact: Bool
    let openBlindBox: () -> Void

    private let columns = [
        GridItem(.flexible(), spacing: 8),
        GridItem(.flexible(), spacing: 8)
    ]

    var body: some View {
        Group {
            if friendCats.isEmpty {
                SingleCatHeroCard(
                    state: localState,
                    compact: compact,
                    accessory: {
                        Group {
                            if let blindBox {
                                FloatingBlindBoxBubble(
                                    blindBox: blindBox,
                                    spendableSteps: spendableSteps,
                                    openBlindBox: openBlindBox
                                )
                                .scaleEffect(compact ? 0.82 : 0.86)
                            } else {
                                BlindBoxCountdownBubble(startDate: dropCycleStartDate)
                                    .scaleEffect(compact ? 0.82 : 0.86)
                            }
                        }
                    }
                )
            } else {
                ZStack {
                    LazyVGrid(columns: columns, spacing: compact ? 6 : 8) {
                        RoomCatCard(
                            title: "You",
                            state: localState,
                            compact: compact,
                            accessory: {
                                Group {
                                    if let blindBox {
                                        FloatingBlindBoxBubble(
                                            blindBox: blindBox,
                                            spendableSteps: spendableSteps,
                                            openBlindBox: openBlindBox
                                        )
                                        .scaleEffect(compact ? 0.64 : 0.72)
                                    } else {
                                        BlindBoxCountdownBubble(startDate: dropCycleStartDate)
                                            .scaleEffect(compact ? 0.64 : 0.72)
                                    }
                                }
                            }
                        )

                        ForEach(friendCats) { friend in
                            RoomCatCard(
                                title: shortName(for: friend.userID),
                                state: friend.state,
                                compact: compact
                            )
                        }

                        ForEach(friendCats.count..<3, id: \.self) { _ in
                            EmptyRoomCatCard(compact: compact)
                        }
                    }

                    CenterClockView(compact: compact)
                        .allowsHitTesting(false)
                }
            }
        }
    }

    private func shortName(for userID: String) -> String {
        if let token = userID.split(separator: "-").last, !token.isEmpty {
            return String(token.prefix(8))
        }
        return String(userID.prefix(8))
    }
}

private struct RoomCatCard<Accessory: View>: View {
    let title: String
    let state: CatState
    let compact: Bool
    @ViewBuilder var accessory: Accessory

    init(
        title: String,
        state: CatState,
        compact: Bool = false,
        @ViewBuilder accessory: () -> Accessory = { EmptyView() }
    ) {
        self.title = title
        self.state = state
        self.compact = compact
        self.accessory = accessory()
    }

    var body: some View {
        VStack(spacing: compact ? 0 : 4) {
            if !compact {
                Text(title)
                    .font(.system(size: 10, weight: .bold, design: .rounded))
                    .foregroundStyle(.white.opacity(0.88))
                    .lineLimit(1)
            }
            ZStack(alignment: .topTrailing) {
                Circle()
                    .fill(Color.white.opacity(0.06))
                    .overlay(
                        Circle()
                            .stroke(Color.white.opacity(0.08), lineWidth: 1)
                    )
                    .frame(width: compact ? 62 : 68, height: compact ? 62 : 68)

                SpineWatchCatView(state: state)
                    .frame(width: compact ? 52 : 58, height: compact ? 52 : 58)
                    .offset(y: compact ? 2 : 4)

                accessory
                    .offset(x: compact ? 8 : 10, y: compact ? -6 : -8)
            }
            .frame(height: compact ? 64 : 72)

            if !compact {
                Text(displayName(for: state))
                    .font(.system(size: 9, weight: .medium, design: .rounded))
                    .foregroundStyle(.white.opacity(0.66))
            }
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, compact ? 4 : 6)
        .padding(.horizontal, compact ? 3 : 4)
        .background(
            RoundedRectangle(cornerRadius: compact ? 16 : 18, style: .continuous)
                .fill(Color.white.opacity(0.05))
        )
        .overlay(
            RoundedRectangle(cornerRadius: compact ? 16 : 18, style: .continuous)
                .stroke(Color.white.opacity(0.06), lineWidth: 1)
        )
    }

    private func displayName(for state: CatState) -> String {
        switch state {
        case .walking:
            return "walking"
        case .running:
            return "running"
        case .sleeping:
            return "sleeping"
        case .idle, .microYawn, .microStretch:
            return "idle"
        }
    }
}

private struct SingleCatHeroCard<Accessory: View>: View {
    let title: String?
    let state: CatState
    let compact: Bool
    @ViewBuilder var accessory: Accessory

    init(
        title: String? = nil,
        state: CatState,
        compact: Bool = false,
        @ViewBuilder accessory: () -> Accessory = { EmptyView() }
    ) {
        self.title = title
        self.state = state
        self.compact = compact
        self.accessory = accessory()
    }

    var body: some View {
        VStack(spacing: compact ? 6 : 8) {
            if let title {
                Text(title)
                    .font(.system(size: compact ? 10 : 11, weight: .bold, design: .rounded))
                    .foregroundStyle(.white.opacity(0.88))
            }

            ZStack(alignment: .topTrailing) {
                Circle()
                    .fill(Color.white.opacity(0.06))
                    .overlay(
                        Circle()
                            .stroke(Color.white.opacity(0.08), lineWidth: 1)
                    )
                    .frame(width: compact ? 104 : 112, height: compact ? 104 : 112)

                SpineWatchCatView(state: state)
                    .frame(width: compact ? 90 : 98, height: compact ? 90 : 98)
                    .offset(y: compact ? 6 : 8)

                accessory
                    .offset(x: compact ? 12 : 14, y: compact ? -8 : -10)
            }
            .frame(maxWidth: .infinity)
            .frame(height: compact ? 108 : 118)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, compact ? 6 : 8)
    }
}

private struct EmptyRoomCatCard: View {
    let compact: Bool

    var body: some View {
        VStack(spacing: compact ? 0 : 4) {
            Circle()
                .fill(Color.white.opacity(0.03))
                .overlay(
                    Circle()
                        .stroke(style: StrokeStyle(lineWidth: 1, dash: [4, 4]))
                        .foregroundStyle(Color.white.opacity(0.12))
                )
                .frame(width: compact ? 62 : 68, height: compact ? 62 : 68)
                .overlay(
                    Text("+")
                        .font(.system(size: compact ? 20 : 22, weight: .light, design: .rounded))
                        .foregroundStyle(.white.opacity(0.25))
                )

            if !compact {
                Text("idle")
                    .font(.system(size: 9, weight: .medium, design: .rounded))
                    .foregroundStyle(.white.opacity(0.36))
            }
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, compact ? 4 : 6)
        .padding(.horizontal, compact ? 3 : 4)
        .background(
            RoundedRectangle(cornerRadius: compact ? 16 : 18, style: .continuous)
                .fill(Color.white.opacity(0.03))
        )
        .overlay(
            RoundedRectangle(cornerRadius: compact ? 16 : 18, style: .continuous)
                .stroke(Color.white.opacity(0.05), lineWidth: 1)
        )
    }
}

private struct CenterClockView: View {
    let compact: Bool

    var body: some View {
        TimelineView(.periodic(from: .now, by: 30)) { context in
            Text(formattedTime(for: context.date))
                .font(.system(size: compact ? 18 : 22, weight: .bold, design: .rounded))
                .foregroundStyle(.white.opacity(0.92))
                .shadow(color: .black.opacity(0.18), radius: 4, y: 1)
                .padding(.horizontal, compact ? 8 : 10)
                .padding(.vertical, compact ? 4 : 6)
                .background(
                    Capsule()
                        .fill(Color.black.opacity(0.16))
                )
        }
    }

    private func formattedTime(for date: Date) -> String {
        let formatter = DateFormatter()
        formatter.locale = .autoupdatingCurrent
        formatter.timeStyle = .short
        formatter.dateStyle = .none
        return formatter.string(from: date)
    }
}

private struct LiveCatView: View {
    let state: CatState

    var body: some View {
        ZStack {
            Circle()
                .fill(Color.white.opacity(0.06))
                .overlay(
                    Circle()
                        .stroke(Color.white.opacity(0.08), lineWidth: 1)
                )

            TimelineView(.periodic(from: .now, by: frameDuration)) { context in
                if let frameImage = AtlasFrameStore.shared.frame(for: normalizedState, at: frameIndex(for: context.date)) {
                    Image(uiImage: frameImage)
                        .resizable()
                        .interpolation(.none)
                        .scaledToFit()
                        .padding(8)
                        .shadow(color: .black.opacity(0.18), radius: 10, y: 6)
                }
            }
        }
    }

    private var normalizedState: CatState {
        switch state {
        case .microYawn, .microStretch:
            return .idle
        default:
            return state
        }
    }

    private var frameDuration: TimeInterval {
        switch normalizedState {
        case .idle:
            return 0.26
        case .walking:
            return 0.16
        case .running:
            return 0.1
        case .sleeping:
            return 0.38
        case .microYawn, .microStretch:
            return 0.26
        }
    }

    private func frameIndex(for date: Date) -> Int {
        let tick = Int(date.timeIntervalSinceReferenceDate / frameDuration)
        return tick % 4
    }
}

private final class AtlasFrameStore {
    static let shared = AtlasFrameStore()

    private var cachedFrames: [CatState: [UIImage]] = [:]

    func frame(for state: CatState, at index: Int) -> UIImage? {
        let frames = framesForState(state)
        guard frames.indices.contains(index) else { return nil }
        return frames[index]
    }

    private func framesForState(_ state: CatState) -> [UIImage] {
        if let cached = cachedFrames[state] {
            return cached
        }

        guard let atlas = loadAtlasImage(), let cgImage = atlas.cgImage else {
            return []
        }

        let row = rowIndex(for: state)
        let tileWidth = cgImage.width / 4
        let tileHeight = cgImage.height / 4
        let scale = atlas.scale

        let frames: [UIImage] = (0..<4).compactMap { column in
            let rect = CGRect(
                x: column * tileWidth,
                y: row * tileHeight,
                width: tileWidth,
                height: tileHeight
            )

            guard let frameCG = cgImage.cropping(to: rect) else { return nil }
            return UIImage(cgImage: frameCG, scale: scale, orientation: .up)
        }

        cachedFrames[state] = frames
        return frames
    }

    private func rowIndex(for state: CatState) -> Int {
        switch state {
        case .idle, .microYawn, .microStretch:
            return 0
        case .walking:
            return 1
        case .running:
            return 2
        case .sleeping:
            return 3
        }
    }

    private func loadAtlasImage() -> UIImage? {
        guard let url = Bundle.main.url(forResource: "default_cat_atlas", withExtension: "png"),
              let data = try? Data(contentsOf: url) else {
            return nil
        }
        return UIImage(data: data)
    }
}

@MainActor
final class WatchMotionController: ObservableObject {
    @Published var currentState: CatState = .idle
    @Published var sensorStatus = "默认状态猫"
    @Published var lastMotionInput: MotionInput = .stationary
    @Published var todaySteps: Int = 0

    var onMotionInputChanged: ((MotionInput) -> Void)?
    var onStateChanged: ((CatState) -> Void)?
    var onTodayStepsChanged: ((Int) -> Void)?

    private let activityManager = CMMotionActivityManager()
    private let motionManager = CMMotionManager()
    private let pedometer = CMPedometer()
    private let machine = CatStateMachine.shared
    private let activityQueue = OperationQueue()
    private var cancellables = Set<AnyCancellable>()
    private var didStart = false
    private var pedometerRefreshTimer: Timer?
    private var fastWalkingSamples = 0
    private var fastStillSamples = 0
    private var recentDynamicSamples: [(time: Date, magnitude: Double)] = []
    private var lastStepDrivenWalkAt: Date?
    private var fastPatternConfirmations = 0
    private var motionInactivityTimer: Timer?
    private var lastMovementSignalAt = Date()

    private enum FastMotionTuning {
        static let updateInterval = 0.2
        static let walkingAccelerationThreshold = 0.075
        static let walkingSampleCount = 4
        static let stillSampleCount = 6
        static let sampleWindowDuration: TimeInterval = 2.6
        static let requiredPeakCount = 4
        static let peakThreshold = 0.09
        static let minPeakInterval: TimeInterval = 0.3
        static let maxPeakInterval: TimeInterval = 0.85
        static let allowedIntervalDrift: TimeInterval = 0.2
        static let minimumAverageMagnitude = 0.065
        static let minimumCadenceSpan: TimeInterval = 1.0
        static let minimumEnergeticSampleRatio = 0.55
        static let requiredPatternConfirmations = 2
    }

    private enum IdleFallbackTuning {
        static let checkInterval: TimeInterval = 3.0
        static let movementTimeout: TimeInterval = 3.0
    }

    func start() {
        guard !didStart else { return }
        didStart = true

        currentState = machine.currentState

        machine.statePublisher
            .receive(on: RunLoop.main)
            .sink { [weak self] state in
                self?.currentState = state
                self?.onStateChanged?(state)
            }
            .store(in: &cancellables)

        startActivityUpdates()
        startPedometerUpdates()
        startIdleFallbackChecks()
    }

    func handleWristRaise() {
        machine.handleMotionInput(.wristRaise)
    }

    private func startActivityUpdates() {
        guard CMMotionActivityManager.isActivityAvailable() else {
            sensorStatus = "当前环境不支持运动同步"
            return
        }

        sensorStatus = "正在同步你的运动状态"
        activityQueue.qualityOfService = .userInitiated

        activityManager.startActivityUpdates(to: activityQueue) { [weak self] activity in
            guard let self, let activity else { return }

            let input = self.map(activity: activity)
            let optimisticState = self.optimisticDisplayState(for: input)

            DispatchQueue.main.async {
                self.sensorStatus = "已同步：\(self.displayName(for: input))"
                self.lastMotionInput = input
                if input == .walking || input == .running {
                    self.lastMovementSignalAt = Date()
                }
                if let optimisticState {
                    self.currentState = optimisticState
                    self.onStateChanged?(optimisticState)
                }
                self.onMotionInputChanged?(input)
                self.machine.handleMotionInput(input)
            }
        }
    }

    private func startFastMotionUpdates() {
        guard motionManager.isAccelerometerAvailable else { return }

        motionManager.accelerometerUpdateInterval = FastMotionTuning.updateInterval
        motionManager.startAccelerometerUpdates(to: activityQueue) { [weak self] data, _ in
            guard let self, let acceleration = data?.acceleration else { return }

            let magnitude = sqrt(
                acceleration.x * acceleration.x +
                acceleration.y * acceleration.y +
                acceleration.z * acceleration.z
            )
            let dynamicMagnitude = abs(magnitude - 1.0)

            DispatchQueue.main.async {
                self.processFastMotionSample(dynamicMagnitude: dynamicMagnitude)
            }
        }
    }

    private func processFastMotionSample(dynamicMagnitude: Double) {
        let now = Date()
        recentDynamicSamples.append((time: now, magnitude: dynamicMagnitude))
        recentDynamicSamples.removeAll { now.timeIntervalSince($0.time) > FastMotionTuning.sampleWindowDuration }

        let isLikelyWalking = dynamicMagnitude >= FastMotionTuning.walkingAccelerationThreshold

        if isLikelyWalking {
            fastWalkingSamples += 1
            fastStillSamples = 0
        } else {
            fastStillSamples += 1
            fastWalkingSamples = 0
        }

        let hasWalkingPattern =
            fastWalkingSamples >= FastMotionTuning.walkingSampleCount &&
            hasWalkingCadence(in: recentDynamicSamples)

        if hasWalkingPattern {
            fastPatternConfirmations += 1
        } else if !isLikelyWalking {
            fastPatternConfirmations = 0
        }

        if fastPatternConfirmations >= FastMotionTuning.requiredPatternConfirmations,
           currentState == .idle || currentState == .microYawn || currentState == .microStretch {
            currentState = .walking
            sensorStatus = "快速通道识别到 walking"
            onStateChanged?(.walking)
            onMotionInputChanged?(.walking)
            machine.handleMotionInput(.walking)
        }

        if fastStillSamples >= FastMotionTuning.stillSampleCount,
           currentState == .walking,
           lastMotionInput == .stationary {
            currentState = .idle
            onStateChanged?(.idle)
            fastPatternConfirmations = 0
        }
    }

    private func hasWalkingCadence(in samples: [(time: Date, magnitude: Double)]) -> Bool {
        guard samples.count >= 5 else { return false }

        let averageMagnitude = samples.map(\.magnitude).reduce(0, +) / Double(samples.count)
        guard averageMagnitude >= FastMotionTuning.minimumAverageMagnitude else { return false }

        let energeticSampleRatio =
            Double(samples.filter { $0.magnitude >= FastMotionTuning.walkingAccelerationThreshold }.count) /
            Double(samples.count)
        guard energeticSampleRatio >= FastMotionTuning.minimumEnergeticSampleRatio else { return false }

        var peakTimes: [Date] = []

        for index in 1..<(samples.count - 1) {
            let previous = samples[index - 1].magnitude
            let current = samples[index].magnitude
            let next = samples[index + 1].magnitude

            guard current >= FastMotionTuning.peakThreshold,
                  current > previous,
                  current >= next else {
                continue
            }

            if let lastPeakTime = peakTimes.last,
               samples[index].time.timeIntervalSince(lastPeakTime) < FastMotionTuning.minPeakInterval {
                continue
            }

            peakTimes.append(samples[index].time)
        }

        guard peakTimes.count >= FastMotionTuning.requiredPeakCount else { return false }

        let recentPeaks = Array(peakTimes.suffix(FastMotionTuning.requiredPeakCount))
        guard let firstPeak = recentPeaks.first,
              let lastPeak = recentPeaks.last,
              lastPeak.timeIntervalSince(firstPeak) >= FastMotionTuning.minimumCadenceSpan else {
            return false
        }

        let peakIntervals = zip(recentPeaks.dropFirst(), recentPeaks).map { later, earlier in
            later.timeIntervalSince(earlier)
        }

        guard peakIntervals.allSatisfy({
            $0 >= FastMotionTuning.minPeakInterval && $0 <= FastMotionTuning.maxPeakInterval
        }) else {
            return false
        }

        if let minInterval = peakIntervals.min(), let maxInterval = peakIntervals.max() {
            return (maxInterval - minInterval) <= FastMotionTuning.allowedIntervalDrift
        }

        return false
    }

    private func map(activity: CMMotionActivity) -> MotionInput {
        if activity.running {
            return .running
        }
        if activity.walking {
            return .walking
        }
        return .stationary
    }

    private func displayName(for input: MotionInput) -> String {
        switch input {
        case .stationary:
            return "idle"
        case .walking:
            return "walking"
        case .running:
            return "running"
        case .wristRaise:
            return "idle"
        }
    }

    private func optimisticDisplayState(for input: MotionInput) -> CatState? {
        switch input {
        case .walking:
            return .walking
        case .running:
            return .running
        case .stationary, .wristRaise:
            return nil
        }
    }

    private func startPedometerUpdates() {
        guard CMPedometer.isStepCountingAvailable() else { return }

        let startOfDay = Calendar.current.startOfDay(for: Date())

        refreshPedometerTotal()

        pedometer.startUpdates(from: startOfDay) { [weak self] data, _ in
            DispatchQueue.main.async {
                let steps = data?.numberOfSteps.intValue ?? 0
                self?.applyPedometerSteps(steps)
            }
        }

        pedometerRefreshTimer?.invalidate()
        pedometerRefreshTimer = Timer.scheduledTimer(withTimeInterval: 3, repeats: true) { [weak self] _ in
            self?.refreshPedometerTotal()
        }
    }

    private func startIdleFallbackChecks() {
        motionInactivityTimer?.invalidate()
        motionInactivityTimer = Timer.scheduledTimer(withTimeInterval: IdleFallbackTuning.checkInterval, repeats: true) { [weak self] _ in
            Task { @MainActor in
                self?.checkIdleFallback()
            }
        }
    }

    private func checkIdleFallback() {
        guard currentState == .walking || currentState == .running else { return }
        guard Date().timeIntervalSince(lastMovementSignalAt) >= IdleFallbackTuning.movementTimeout else { return }

        lastMotionInput = .stationary
        sensorStatus = "兜底检测切回 idle"
        onMotionInputChanged?(.stationary)
        machine.handleMotionInput(.stationary)
    }

    private func refreshPedometerTotal() {
        let startOfDay = Calendar.current.startOfDay(for: Date())

        pedometer.queryPedometerData(from: startOfDay, to: Date()) { [weak self] data, _ in
            DispatchQueue.main.async {
                let steps = data?.numberOfSteps.intValue ?? 0
                guard let self else { return }
                self.applyPedometerSteps(steps)
            }
        }
    }

    private func applyPedometerSteps(_ steps: Int) {
        let delta = max(steps - todaySteps, 0)
        guard steps != todaySteps else { return }

        todaySteps = steps
        onTodayStepsChanged?(steps)

        guard delta > 0 else { return }
        triggerWalkingFromConfirmedSteps()
    }

    private func triggerWalkingFromConfirmedSteps() {
        let now = Date()
        if let lastStepDrivenWalkAt,
           now.timeIntervalSince(lastStepDrivenWalkAt) < 2.5 {
            return
        }

        lastStepDrivenWalkAt = now
        lastMovementSignalAt = now
        lastMotionInput = .walking
        sensorStatus = "步数同步触发 walking"

        if currentState == .idle || currentState == .microYawn || currentState == .microStretch {
            currentState = .walking
            onStateChanged?(.walking)
        }

        onMotionInputChanged?(.walking)
        machine.handleMotionInput(.walking)
    }
}

#if DEBUG
extension WatchMotionController {
    func injectDebugWalkAndSteps() {
        todaySteps += 50
        lastMotionInput = .walking
        currentState = .walking
        sensorStatus = "调试注入：walking +50"

        onTodayStepsChanged?(todaySteps)
        onMotionInputChanged?(.walking)
        onStateChanged?(.walking)
        machine.handleMotionInput(.walking)
    }
}
#endif

@MainActor
final class StandReminderManager: ObservableObject {
    @Published var foregroundBannerText: String?

    private var timer: Timer?
    private let center = UNUserNotificationCenter.current()
    private var authorizationGranted = false
    private let notificationIdentifier = "cat.hourly.stand.reminder"
    private var currentCatState: CatState = .idle
    private var bannerDismissWorkItem: DispatchWorkItem?
    private var todaySteps = 0
    private var hourlyWindowStart = Date()
    private var baselineStepsAtWindowStart = 0
    private var isAppActive = true

    private let inactivityThresholdSteps = 50
    private let inactivityWindowDuration: TimeInterval = 120

    func start() {
        requestAuthorization()
        startForegroundTimer()
        resetHourlyWindow()
    }

    func handleScenePhaseChange(_ phase: ScenePhase) {
        isAppActive = (phase == .active)
        if phase == .active {
            startForegroundTimer()
            checkInactivityReminder()
        } else {
            stopForegroundTimer()
        }
    }

    func registerMotion(_ input: MotionInput) {
        switch input {
        case .walking, .running:
            break
        case .stationary:
            if currentCatState == .sleeping {
                removePendingNotification()
            }
        case .wristRaise:
            break
        }
    }

    func registerCatState(_ state: CatState) {
        let previousState = currentCatState
        currentCatState = state

        if state == .sleeping {
            removePendingNotification()
        } else if previousState == .sleeping {
            resetHourlyWindow()
        }
    }

    func registerTodaySteps(_ steps: Int) {
        todaySteps = steps

        guard currentCatState != .sleeping else { return }

        let stepsInWindow = todaySteps - baselineStepsAtWindowStart
        if stepsInWindow >= inactivityThresholdSteps {
            resetHourlyWindow()
        }
    }

    private func requestAuthorization() {
        center.requestAuthorization(options: [.alert, .sound]) { [weak self] granted, _ in
            Task { @MainActor in
                guard let self else { return }
                self.authorizationGranted = granted
                self.resetHourlyWindow()
            }
        }
    }

    private func startForegroundTimer() {
        guard timer == nil else { return }

        timer = Timer.scheduledTimer(withTimeInterval: 30, repeats: true) { [weak self] _ in
            Task { @MainActor in
                guard let self else { return }
                self.checkInactivityReminder()
            }
        }
    }

    private func stopForegroundTimer() {
        timer?.invalidate()
        timer = nil
    }

    private func checkInactivityReminder() {
        guard currentCatState != .sleeping else { return }
        guard Date().timeIntervalSince(hourlyWindowStart) >= inactivityWindowDuration else { return }

        let stepsInWindow = todaySteps - baselineStepsAtWindowStart
        let shouldRemind = stepsInWindow < inactivityThresholdSteps

        if shouldRemind {
            if isAppActive {
                WKInterfaceDevice.current().play(.directionUp)
                showForegroundBanner("猫猫想陪你走几步")
            }
        }

        resetHourlyWindow()
    }

    private func removePendingNotification() {
        center.removePendingNotificationRequests(withIdentifiers: [notificationIdentifier])
    }

    private func resetHourlyWindow() {
        hourlyWindowStart = Date()
        baselineStepsAtWindowStart = todaySteps
        scheduleInactivityNotification()
    }

    private func scheduleInactivityNotification() {
        guard authorizationGranted else { return }
        guard currentCatState != .sleeping else {
            removePendingNotification()
            return
        }

        removePendingNotification()

        let content = UNMutableNotificationContent()
        content.title = "该起身走走了"
        content.body = "让猫陪你活动一会儿，慢慢走几步就好。"
        content.sound = .default

        let request = UNNotificationRequest(
            identifier: notificationIdentifier,
            content: content,
            trigger: UNTimeIntervalNotificationTrigger(timeInterval: inactivityWindowDuration, repeats: false)
        )

        center.add(request) { _ in }
    }

    private func showForegroundBanner(_ message: String) {
        bannerDismissWorkItem?.cancel()
        foregroundBannerText = message

        let dismissWorkItem = DispatchWorkItem { [weak self] in
            guard let self else { return }
            withAnimation {
                self.foregroundBannerText = nil
            }
        }

        bannerDismissWorkItem = dismissWorkItem
        DispatchQueue.main.asyncAfter(deadline: .now() + 2.8, execute: dismissWorkItem)
    }
}

@MainActor
final class BlindBoxManager: ObservableObject {
    @Published var currentBlindBox: BlindBoxStatus?
    @Published var statusText = ""
    @Published var dropCycleStartDate = Date()
    @Published var spendableSteps = 0

    static let dropInterval: TimeInterval = 30
    static let stepsCost = 300

    private let localStore = LocalStore()
    private var timer: Timer?
    private var currentCatState: CatState = .idle
    private var lastObservedTodaySteps = 0
    private let rewardPool = ["奶油项圈", "厨师帽", "草莓围兜", "月亮吊坠", "小雨衣"]

    func start() {
        currentBlindBox = localStore.loadBlindBoxStatus()
        spendableSteps = localStore.loadBlindBoxSpendableSteps()
        lastObservedTodaySteps = localStore.loadBlindBoxObservedTodaySteps()
        dropCycleStartDate = localStore.loadBlindBoxLastDropDate() ?? Date()
        if localStore.loadBlindBoxLastDropDate() == nil {
            localStore.saveBlindBoxLastDropDate(dropCycleStartDate)
        }
        startTimer()
        refreshDropStatus()
    }

    func handleScenePhaseChange(_ phase: ScenePhase) {
        if phase == .active {
            startTimer()
            refreshDropStatus()
        } else {
            stopTimer()
        }
    }

    func registerCatState(_ state: CatState) {
        currentCatState = state
        refreshDropStatus()
    }

    func registerTodaySteps(_ steps: Int) {
        let delta = max(steps - lastObservedTodaySteps, 0)
        lastObservedTodaySteps = steps
        localStore.saveBlindBoxObservedTodaySteps(steps)

        guard delta > 0 else { return }

        spendableSteps += delta
        localStore.saveBlindBoxSpendableSteps(spendableSteps)

        if currentBlindBox != nil {
            statusText = spendableSteps >= Self.stepsCost
                ? "盲盒可领取，消耗 300 点"
                : "当前点数 \(spendableSteps)，还差 \(Self.stepsCost - spendableSteps) 点"
        }
    }

    func openBlindBox() {
        guard let blindBox = currentBlindBox, spendableSteps >= Self.stepsCost else { return }
        spendableSteps -= Self.stepsCost
        localStore.saveBlindBoxSpendableSteps(spendableSteps)
        statusText = "你打开了盲盒，获得 \(blindBox.rewardName)"
        currentBlindBox = nil
        localStore.saveBlindBoxStatus(nil)
        dropCycleStartDate = Date()
        localStore.saveBlindBoxLastDropDate(dropCycleStartDate)
    }

    private func startTimer() {
        guard timer == nil else { return }

        timer = Timer.scheduledTimer(withTimeInterval: 1, repeats: true) { [weak self] _ in
            Task { @MainActor in
                self?.refreshDropStatus()
            }
        }
    }

    private func stopTimer() {
        timer?.invalidate()
        timer = nil
    }

    private func refreshDropStatus() {
        if let blindBox = currentBlindBox {
            statusText = spendableSteps >= Self.stepsCost
                ? "盲盒可领取，消耗 300 点"
                : "当前点数 \(spendableSteps)，还差 \(Self.stepsCost - spendableSteps) 点"
            return
        }

        guard currentCatState != .sleeping else {
            statusText = ""
            return
        }

        let lastDropDate = localStore.loadBlindBoxLastDropDate() ?? Date()
        dropCycleStartDate = lastDropDate
        let elapsed = Date().timeIntervalSince(lastDropDate)

        if elapsed >= Self.dropInterval {
            dropBlindBox()
        } else {
            statusText = ""
        }
    }

    private func dropBlindBox() {
        let blindBox = BlindBoxStatus(
            droppedAt: Date(),
            rewardName: rewardPool.randomElement() ?? "神秘配件"
        )
        currentBlindBox = blindBox
        localStore.saveBlindBoxStatus(blindBox)
        statusText = spendableSteps >= Self.stepsCost
            ? "掉落了一个盲盒，点击即可领取"
            : "掉落了一个盲盒，还差 \(Self.stepsCost - spendableSteps) 点"
        WKInterfaceDevice.current().play(.notification)
    }
}

private struct BlindBoxCard: View {
    @ObservedObject var manager: BlindBoxManager

    var body: some View {
        guard let blindBox = manager.currentBlindBox else {
            return AnyView(EmptyView())
        }

        return AnyView(
        VStack(alignment: .leading, spacing: 8) {
            Text("盲盒")
                .font(.system(size: 12, weight: .semibold, design: .rounded))
                .foregroundStyle(.white)

            Text(manager.statusText)
                .font(.system(size: 10, weight: .medium, design: .rounded))
                .foregroundStyle(.white.opacity(0.72))

            VStack(alignment: .leading, spacing: 4) {
                Text(manager.spendableSteps >= BlindBoxManager.stepsCost ? "可领取" : "待领取")
                    .font(.system(size: 11, weight: .semibold, design: .rounded))
                    .foregroundStyle(manager.spendableSteps >= BlindBoxManager.stepsCost ? .mint : .orange)

                Text("当前点数：\(manager.spendableSteps)")
                    .font(.system(size: 10, weight: .medium, design: .rounded))
                    .foregroundStyle(.white.opacity(0.7))
            }

            if manager.spendableSteps >= BlindBoxManager.stepsCost {
                Button("打开盲盒") {
                    manager.openBlindBox()
                }
                .buttonStyle(.borderedProminent)
                .tint(.orange)
            }
        }
        .padding(10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color.white.opacity(0.08), in: RoundedRectangle(cornerRadius: 16, style: .continuous))
        )
    }
}

private struct FloatingBlindBoxBubble: View {
    let blindBox: BlindBoxStatus
    let spendableSteps: Int
    let openBlindBox: () -> Void

    @State private var isFloating = false

    var body: some View {
        VStack(alignment: .center, spacing: 4) {
            Button {
                if canClaim {
                    openBlindBox()
                }
            } label: {
                ZStack {
                    Circle()
                        .fill(canClaim ? Color.orange.opacity(0.28) : Color.white.opacity(0.14))
                        .frame(width: 38, height: 38)

                    Text("🎁")
                        .font(.system(size: 20))
                }
            }
            .buttonStyle(.plain)
            .scaleEffect(isFloating ? 1.04 : 0.96)
            .offset(y: isFloating ? -3 : 3)

            if canClaim {
                Text("可领取")
                    .font(.system(size: 9, weight: .semibold, design: .rounded))
                    .foregroundStyle(.orange)
            } else {
                Text("差 \(max(BlindBoxManager.stepsCost - spendableSteps, 0))")
                    .font(.system(size: 9, weight: .semibold, design: .rounded))
                    .foregroundStyle(.white.opacity(0.78))
            }
        }
        .padding(.horizontal, 6)
        .padding(.vertical, 5)
        .background(Color.black.opacity(0.22), in: Capsule())
        .onAppear {
            withAnimation(.easeInOut(duration: 1.2).repeatForever(autoreverses: true)) {
                isFloating = true
            }
        }
    }

    private var canClaim: Bool {
        spendableSteps >= BlindBoxManager.stepsCost
    }
}

private struct BlindBoxCountdownBubble: View {
    let startDate: Date

    @State private var isFloating = false

    var body: some View {
        VStack(alignment: .center, spacing: 4) {
            ZStack {
                Circle()
                    .fill(Color.white.opacity(0.12))
                    .frame(width: 38, height: 38)

                Text("⏳")
                    .font(.system(size: 18))
            }
            .scaleEffect(isFloating ? 1.03 : 0.97)
            .offset(y: isFloating ? -2 : 2)

            TimelineView(.periodic(from: startDate, by: 1)) { context in
                Text(countdownText(now: context.date))
                    .font(.system(size: 9, weight: .semibold, design: .rounded))
                    .foregroundStyle(.white.opacity(0.78))
            }
        }
        .padding(.horizontal, 6)
        .padding(.vertical, 5)
        .background(Color.black.opacity(0.22), in: Capsule())
        .onAppear {
            withAnimation(.easeInOut(duration: 1.2).repeatForever(autoreverses: true)) {
                isFloating = true
            }
        }
    }

    private func countdownText(now: Date) -> String {
        let remaining = max(Int(BlindBoxManager.dropInterval) - Int(now.timeIntervalSince(startDate)), 0)
        let minutes = remaining / 60
        let seconds = remaining % 60
        return String(format: "%02d:%02d", minutes, seconds)
    }
}
