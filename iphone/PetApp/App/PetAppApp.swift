// PetAppApp.swift
// Story 2.2: SwiftUI App 入口（@main）
//
// Story 2.9 起：RootView 内含 AppLaunchStateMachine 启动状态机；
// 根据 launchStateMachine.state 路由到 LaunchingView / HomeView / RetryView。
// 详见 RootView.swift + AppLaunchStateMachine.swift。
//
// Story 8.1 AC7：DEBUG-only HealthKit 集成测试 launch argument 处理：
//   - `-PetAppPreseedHealthKitSteps <N>`：启动期把 N 步预置到 HKHealthStore（DEBUG only）
//   - `-PetAppRunHealthProviderIntegrationProbe`：用 HealthProviderProbeView 替换 RootView
//     渲染（让 XCUITest 拿 a11y label 断言 readDailyTotalSteps 返回值）
// Release build 这两段全部 #if DEBUG 包裹，不参与编译。

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
    /// Story 8.1 AC7: 待 seed 步数；nil = 不预置. ProbeView 在 .task 入口先 await seed 完成
    /// 再触发 read，消除"detached seed 与 probe view read 之间的 race"（codex r1 [P1]）.
    /// 详见 docs/lessons/2026-05-04-debug-seed-vs-probe-await-coupling.md.
    private let preseedStepsForProbe: Int?
    #endif

    init() {
        #if DEBUG
        // 解析 -PetAppPreseedHealthKitSteps <N> launch argument.
        // ProcessInfo.arguments 是 ["<binary>", "-PetAppPreseedHealthKitSteps", "5000", ...]
        // 顺序解析；缺值 / 非数字 / N <= 0 时静默跳过.
        //
        // ⚠️ 不再用 detached Task 在 init 期 fire-and-forget seed（codex r1 [P1] race）—— 改成
        // 把待 seed 数量挂在 instance var 上，由 ProbeView 在 .task 入口 `await` 完成 seed 后再 read.
        let args = ProcessInfo.processInfo.arguments
        if let idx = args.firstIndex(of: "-PetAppPreseedHealthKitSteps"),
           idx + 1 < args.count,
           let steps = Int(args[idx + 1]),
           steps > 0 {
            preseedStepsForProbe = steps
        } else {
            preseedStepsForProbe = nil
        }

        useHealthProviderProbe = args.contains("-PetAppRunHealthProviderIntegrationProbe")
        probeHealthProvider = HealthProviderImpl()
        #endif
    }

    var body: some Scene {
        WindowGroup {
            #if DEBUG
            if useHealthProviderProbe {
                HealthProviderProbeView(
                    healthProvider: probeHealthProvider,
                    preseedSteps: preseedStepsForProbe
                )
            } else {
                RootView()
            }
            #else
            RootView()
            #endif
        }
    }
}
