// MotionProviderProbeView.swift
// Story 8.2 AC7: 仅 DEBUG / UITest 路径——PetAppApp 在 launch argument
// `-PetAppRunMotionProviderIntegrationProbe` 时挂载本 view 替换正常 UI.
//
// 渲染：调 `motionProvider.requestPermission` 然后 `startUpdates` → 把最近一次接收的
// CMMotionActivity 关键 flag 拼成字符串显示（a11y identifier "motionProviderProbeResult"）;
// startUpdates 调用完成（不论是否已收到首个 activity）→ status label "motionProviderProbeStatus"
// 立即显示 "subscribed"——表示 wiring 已 wired up；UITest 用此路径作为 happy path 断言，不再
// 依赖 simulator 自发 emit activity event（review round 2 P2）.
// 错误（如 systemFailure / permissionDenied）通过 errorText 暴露在 a11y identifier "motionProviderProbeError" 上.
//
// 不挂 production code 路径（仅 #if DEBUG 编译；PetAppApp.swift 启动期检测 launch argument 决定是否挂载）.
//
// 与 8.1 HealthProviderProbeView 同精神 + round 1 lesson：
// requestPermission → startUpdates 必须串行 await（防 race + 防未授权时 startUpdates 静默失败误判）.
// 详见 docs/lessons/2026-05-04-debug-seed-vs-probe-await-coupling.md.
//
// review round 2 P2 修订：requestPermission() 返 false（deny 路径，不抛）时 **不能** 继续调
// startUpdates——CoreMotion 在未授权下静默不响应，UITest 30s 超时也不报错. 改为：deny 时
// 设 errorText="permissionDenied" 直接 return，不走 startUpdates.
// 详见 docs/lessons/2026-05-04-motion-probe-deny-path-and-subscribed-marker.md.

#if DEBUG
import SwiftUI
import CoreMotion

/// CoreMotion 集成测试用 probe view；显示 motionProvider.startUpdates 接收到的最新 CMMotionActivity flag 状态.
struct MotionProviderProbeView: View {
    let motionProvider: MotionProvider

    @State private var resultText: String = "(waiting)"
    @State private var statusText: String = "(idle)"
    @State private var errorText: String = ""

    var body: some View {
        VStack(spacing: 16) {
            Text("MotionProvider Probe")
                .font(.title3)
            Text(resultText)
                .font(.system(size: 24, weight: .bold))
                .accessibilityIdentifier("motionProviderProbeResult")
            Text(statusText)
                .font(.footnote)
                .foregroundColor(.secondary)
                .accessibilityIdentifier("motionProviderProbeStatus")
            if !errorText.isEmpty {
                Text(errorText)
                    .foregroundColor(.red)
                    .accessibilityIdentifier("motionProviderProbeError")
            }
        }
        .task {
            // 1. 申请权限. 必须先 await permission 再调 startUpdates，避免未授权时 startUpdates 静默失败.
            //    三态处理（review round 2 P2）：
            //      - throw → 写 errorText（systemFailure / cancelled），不走 startUpdates.
            //      - return false → permissionDenied，写 errorText 后 return，不走 startUpdates.
            //        （CoreMotion 在 deny 状态下 startUpdates 不抛也不回调，会让 UI 永远停在 "(waiting)"）
            //      - return true → 继续到 step 2 注册 handler.
            let granted: Bool
            do {
                granted = try await motionProvider.requestPermission()
            } catch {
                await MainActor.run {
                    errorText = String(describing: error)
                }
                return
            }
            guard granted else {
                await MainActor.run {
                    errorText = "permissionDenied"
                }
                return
            }

            // 2. startUpdates 注册 handler. 调用完成后立即 set status="subscribed"——
            //    UITest happy path 断言此 marker，不再依赖 simulator 自发 emit activity event
            //    （review round 2 P2：CoreMotion idle simulator 30s 不发 event 是常态）.
            motionProvider.startUpdates { activity in
                Task { @MainActor in
                    resultText = MotionProviderProbeView.describeActivity(activity)
                }
            }
            await MainActor.run {
                statusText = "subscribed"
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
