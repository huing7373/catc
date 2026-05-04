// MotionProviderProbeView.swift
// Story 8.2 AC7: 仅 DEBUG / UITest 路径——PetAppApp 在 launch argument
// `-PetAppRunMotionProviderIntegrationProbe` 时挂载本 view 替换正常 UI.
//
// 渲染：调 `motionProvider.requestPermission` 然后 `startUpdates` → 把最近一次接收的
// CMMotionActivity 关键 flag 拼成字符串显示（a11y identifier "motionProviderProbeResult"）;
// 错误（如 systemFailure）通过 errorText 暴露在 a11y identifier "motionProviderProbeError" 上.
//
// 不挂 production code 路径（仅 #if DEBUG 编译；PetAppApp.swift 启动期检测 launch argument 决定是否挂载）.
//
// 与 8.1 HealthProviderProbeView 同精神 + round 1 lesson：
// requestPermission → startUpdates 必须串行 await（防 race + 防未授权时 startUpdates 静默失败误判）.
// 详见 docs/lessons/2026-05-04-debug-seed-vs-probe-await-coupling.md.

#if DEBUG
import SwiftUI
import CoreMotion

/// CoreMotion 集成测试用 probe view；显示 motionProvider.startUpdates 接收到的最新 CMMotionActivity flag 状态.
struct MotionProviderProbeView: View {
    let motionProvider: MotionProvider

    @State private var resultText: String = "(waiting)"
    @State private var errorText: String = ""

    var body: some View {
        VStack(spacing: 16) {
            Text("MotionProvider Probe")
                .font(.title3)
            Text(resultText)
                .font(.system(size: 24, weight: .bold))
                .accessibilityIdentifier("motionProviderProbeResult")
            if !errorText.isEmpty {
                Text(errorText)
                    .foregroundColor(.red)
                    .accessibilityIdentifier("motionProviderProbeError")
            }
        }
        .task {
            // 1. 申请权限（模拟器自动授予；UITest 容忍失败仍调 startUpdates，
            //    与 8.1 同模式——sandbox 行为是 PASS 路径）.
            //    必须先 await permission 再调 startUpdates，避免未授权时 startUpdates 静默失败 + 误判 wired-up.
            do {
                _ = try await motionProvider.requestPermission()
            } catch {
                await MainActor.run {
                    errorText = String(describing: error)
                }
            }

            // 2. startUpdates 注册 handler，写最近一次 activity 字段到 result label.
            motionProvider.startUpdates { activity in
                Task { @MainActor in
                    resultText = MotionProviderProbeView.describeActivity(activity)
                }
            }
        }
    }

    /// 把 CMMotionActivity 转成可读字符串（probe view 显示用）.
    /// 多个 flag 同时 true 时按 stationary/walking/running/cycling/automotive/unknown 顺序拼接.
    static func describeActivity(_ activity: CMMotionActivity) -> String {
        var parts: [String] = []
        if activity.stationary { parts.append("stationary") }
        if activity.walking { parts.append("walking") }
        if activity.running { parts.append("running") }
        if activity.cycling { parts.append("cycling") }
        if activity.automotive { parts.append("automotive") }
        if activity.unknown { parts.append("unknown") }
        let joined = parts.isEmpty ? "none" : parts.joined(separator: "+")
        let confidenceStr: String
        switch activity.confidence {
        case .low: confidenceStr = "low"
        case .medium: confidenceStr = "medium"
        case .high: confidenceStr = "high"
        @unknown default: confidenceStr = "unknown"
        }
        return "\(joined)|\(confidenceStr)"
    }
}
#endif
