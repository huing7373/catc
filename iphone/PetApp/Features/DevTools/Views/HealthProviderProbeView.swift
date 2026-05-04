// HealthProviderProbeView.swift
// Story 8.1 AC7: 仅 DEBUG / UITest 路径——RootView 在 launch argument
// `-PetAppRunHealthProviderIntegrationProbe` 时挂载本 view 替换正常 UI.
//
// 渲染：调 `healthProvider.readDailyTotalSteps(date: Date())` → 把 Int 转 String 显示，
// 加 a11y identifier `healthProviderProbeResult` 让 XCUITest 拿 label 断言.
//
// 不挂 production code 路径（仅 #if DEBUG 编译；PetAppApp.swift 启动期检测 launch argument 决定是否挂载）.
//
// codex r1 [P1] fix：seed 不再在 PetAppApp.init 用 detached Task fire-and-forget——改成
// preseedSteps 注入本 view，由 .task 在 read 之前**串行 await** preseedToday（…），
// 消除 seed 未完成就 read 的 race. 详见 docs/lessons/2026-05-04-debug-seed-vs-probe-await-coupling.md.

#if DEBUG
import SwiftUI

/// HealthKit 集成测试用 probe view；显示 healthProvider.readDailyTotalSteps 当日返回值.
struct HealthProviderProbeView: View {
    let healthProvider: HealthProvider
    /// 当 nil 时跳过 seed；非 nil 时在 read 之前 await `HealthKitDevSeedUseCase.preseedToday(steps:)`
    /// 完成（codex r1 [P1] race fix）.
    let preseedSteps: Int?

    @State private var resultText: String = "-"
    @State private var errorText: String = ""

    var body: some View {
        VStack(spacing: 16) {
            Text("HealthProvider Probe")
                .font(.title3)
            Text(resultText)
                .font(.system(size: 48, weight: .bold))
                .accessibilityIdentifier("healthProviderProbeResult")
            if !errorText.isEmpty {
                Text(errorText)
                    .foregroundColor(.red)
                    .accessibilityIdentifier("healthProviderProbeError")
            }
        }
        .task {
            // 1. 串行 seed（如果 launch arg 指定了步数）.
            //    seed 失败时不抛 view-level error——继续 read 让模拟器/error 路径自行表达
            //    （seed 失败 → read 返回 0/permissionDenied 都是 AC7 钦定的 wired-up PASS 路径）.
            if let steps = preseedSteps {
                try? await HealthKitDevSeedUseCase.preseedToday(steps: steps)
            }

            // 2. 申请权限（模拟器 HealthKit 模式下自动授予；UITest 容忍失败仍 read，
            //    模拟器实际行为是放行）.
            _ = try? await healthProvider.requestPermission()

            // 3. read 并填 a11y label.
            do {
                let value = try await healthProvider.readDailyTotalSteps(date: Date())
                await MainActor.run {
                    resultText = String(value)
                }
            } catch {
                await MainActor.run {
                    errorText = String(describing: error)
                }
            }
        }
    }
}
#endif
