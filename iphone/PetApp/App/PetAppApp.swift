// PetAppApp.swift
// Story 2.2: SwiftUI App 入口（@main）
//
// Story 2.9 起：RootView 内含 AppLaunchStateMachine 启动状态机；
// 根据 launchStateMachine.state 路由到 LaunchingView / HomeView / RetryView。
// 详见 RootView.swift + AppLaunchStateMachine.swift。
//
// Story 8.1 AC7：DEBUG-only HealthKit 集成测试 launch argument 处理：
//   - `-PetAppPreseedHealthKitSteps <N>`：启动期把 N 步预置到 HKHealthStore（DEBUG only）.
//     该 flag 在 probe / 非 probe 两条路径都生效——见 RootBootstrapView（codex r2 [P2] regression fix）.
//   - `-PetAppRunHealthProviderIntegrationProbe`：用 HealthProviderProbeView 替换 RootView
//     渲染（让 XCUITest 拿 a11y label 断言 readDailyTotalSteps 返回值）
// Story 8.2 AC7：DEBUG-only CoreMotion 集成测试 launch argument 处理：
//   - `-PetAppRunMotionProviderIntegrationProbe`：用 MotionProviderProbeView 替换 RootView
//     渲染（让 XCUITest 拿 a11y label 断言 startUpdates 路径已 wired up）
//   - 与 health probe 互斥：health 优先 → motion 其次 → 默认（RootBootstrapView/RootView）
// Release build 这几段全部 #if DEBUG 包裹，不参与编译。

import SwiftUI

@main
struct PetAppApp: App {
    #if DEBUG
    /// Story 8.1 AC7: HealthProvider 集成测试 probe view 入口标记.
    /// 由 ProcessInfo.arguments 解析触发；非 UITest 路径恒为 false.
    private let useHealthProviderProbe: Bool
    /// Story 8.1 AC7: 共享 HealthProvider 实例（probe view 与 AppContainer 不共用，
    /// 因为 probe view 路径下 RootView/AppContainer 不挂载——直接 new 一个 HealthProviderImpl 用即可）.
    private let probeHealthProvider: HealthProvider
    /// Story 8.1 AC7: 待 seed 步数；nil = 不预置.
    /// - probe 路径：HealthProviderProbeView 在 .task 入口 `await` 完成 seed 后再 read.
    /// - 非 probe 路径（codex r2 [P2] fix）：RootBootstrapView 在 .task 入口 `await` 完成 seed 后再 mount RootView.
    /// 两条路径都把 seed **串到下游消费者的 .task 里 await**，遵守 round 1 lesson
    /// （docs/lessons/2026-05-04-debug-seed-vs-probe-await-coupling.md）.
    private let preseedSteps: Int?
    /// Story 8.2 AC7: MotionProvider 集成测试 probe view 入口标记.
    /// 由 ProcessInfo.arguments 解析触发；非 UITest 路径恒为 false.
    /// 与 useHealthProviderProbe 互斥（health 优先；body 内分支顺序保证）.
    private let useMotionProviderProbe: Bool
    /// Story 8.2 AC7: 共享 MotionProvider 实例（probe view 与 AppContainer 不共用，
    /// 因为 probe view 路径下 RootView/AppContainer 不挂载——直接 new 一个 MotionProviderImpl 用即可）.
    private let probeMotionProvider: MotionProvider
    #endif

    init() {
        #if DEBUG
        // 解析 -PetAppPreseedHealthKitSteps <N> launch argument.
        // ProcessInfo.arguments 是 ["<binary>", "-PetAppPreseedHealthKitSteps", "5000", ...]
        // 顺序解析；缺值 / 非数字 / N <= 0 时静默跳过.
        //
        // ⚠️ 不在 init 期触发 detached Task seed（codex r1 [P1] race）—— 把数量挂在 instance var 上，
        // 由下游消费者 view 在 .task 入口 `await` 完成 seed 后再继续.
        let args = ProcessInfo.processInfo.arguments
        if let idx = args.firstIndex(of: "-PetAppPreseedHealthKitSteps"),
           idx + 1 < args.count,
           let steps = Int(args[idx + 1]),
           steps > 0 {
            preseedSteps = steps
        } else {
            preseedSteps = nil
        }

        useHealthProviderProbe = args.contains("-PetAppRunHealthProviderIntegrationProbe")
        probeHealthProvider = HealthProviderImpl()

        // Story 8.2 AC7: motion probe launch arg.
        useMotionProviderProbe = args.contains("-PetAppRunMotionProviderIntegrationProbe")
        probeMotionProvider = MotionProviderImpl()
        #endif
    }

    var body: some Scene {
        WindowGroup {
            #if DEBUG
            if useHealthProviderProbe {
                // probe 路径：seed 串在 ProbeView .task 里
                HealthProviderProbeView(
                    healthProvider: probeHealthProvider,
                    preseedSteps: preseedSteps
                )
            } else if useMotionProviderProbe {
                // Story 8.2 AC7：motion probe 路径——RootView/AppContainer 不挂载，直接渲染 probe view.
                // 与 health probe 互斥（health 优先；如同时传两个 flag，按代码顺序——只挂第一个）.
                MotionProviderProbeView(motionProvider: probeMotionProvider)
            } else {
                // 非 probe 路径（codex r2 [P2] fix）：RootBootstrapView 在 .task 入口 await seed
                // 完成后再 mount RootView——让 -PetAppPreseedHealthKitSteps 在常规 UI 路径下也工作.
                // preseedSteps == nil 时 RootBootstrapView 直接 mount RootView（零开销）.
                RootBootstrapView(preseedSteps: preseedSteps)
            }
            #else
            RootView()
            #endif
        }
    }
}

#if DEBUG
/// DEBUG-only wrapper：在 mount RootView 之前先 await `HealthKitDevSeedUseCase.preseedToday` 完成
/// （仅当 `-PetAppPreseedHealthKitSteps <N>` launch arg 解析出 N>0 时）.
///
/// 设计要点（codex r2 [P2] regression fix——`-PetAppPreseedHealthKitSteps` 在 non-probe 路径下也必须生效）：
/// - seed 串到 RootBootstrapView .task 里 await，**禁止** detached fire-and-forget
///   （遵守 round 1 lesson：docs/lessons/2026-05-04-debug-seed-vs-probe-await-coupling.md）
/// - preseedSteps == nil 时直接渲染 RootView，零开销路径
/// - seed 失败不阻断 mount——继续渲染 RootView（生产不依赖 seed；UITest 通过 read 路径自行表达失败）
struct RootBootstrapView: View {
    let preseedSteps: Int?

    @State private var seedDone: Bool = false

    var body: some View {
        Group {
            if preseedSteps == nil || seedDone {
                RootView()
            } else {
                // 占位：极短窗口（HK seed 通常 < 100ms）；故意不显示进度，避免 UITest 锚定到 spinner
                Color.clear
            }
        }
        .task {
            guard let steps = preseedSteps, !seedDone else { return }
            // 失败吞掉——RootView 不依赖 seed；UITest 用 probe view 路径专门验证 seed 结果.
            try? await HealthKitDevSeedUseCase.preseedToday(steps: steps)
            await MainActor.run { seedDone = true }
        }
    }
}
#endif
