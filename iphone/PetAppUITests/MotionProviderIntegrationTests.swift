// MotionProviderIntegrationTests.swift
// Story 8.2 AC7: MotionProviderImpl 在模拟器 CoreMotion 上的集成测试.
//
// 流程：
//   1. launch arguments 含 -PetAppRunMotionProviderIntegrationProbe →
//      PetAppApp.body 渲染 MotionProviderProbeView 替代 RootView
//   2. ProbeView .task 串行调 motionProvider.requestPermission → motionProvider.startUpdates →
//      handler 把 activity 字段拼成字符串显示在 a11y label "motionProviderProbeResult"
//   3. 本测试 wait for label "motionProviderProbeResult" → 30s 内轮询
//      要么 label 显示 activity 字符串（happy path：模拟器 CoreMotion 触发了 activity event）
//      要么 errorLabel "motionProviderProbeError" 显示错误（sandbox-limited / permissionDenied 路径）
//
// 红线（详见 story 8-2-coremotion-接入.md AC7 段，沿用 8.1 红线 + review round 1 P3 修订）：
//   - 模拟器 CoreMotion 在 Xcode 26 + iPhone 17 simulator 实测**会**触发 stationary/walking activity；
//     30s 仍然停留在 "(waiting)" 占位 + 没有 error → **判定为 wiring 死链**（review round 1 P3）.
//   - 旧版本曾把"30s 后仍是 (waiting) + 没 error"也视作 PASS，结果会让 startUpdates 路径整段 dead wiring
//     却保持绿；本版要求 result label 必须脱离 "(waiting)" **或** error label 必须出现，否则 XCTFail.
//   - 不验证权限弹窗交互（XCUITest 无法稳定模拟系统弹窗）；不引 entitlements 文件（节点 3 阶段不允许）.

import XCTest

final class MotionProviderIntegrationTests: XCTestCase {

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    func testMotionProviderImpl_probeViewMountsAndExecutesStartUpdatesPath() throws {
        let app = XCUIApplication()
        app.launchArguments = [
            "-PetAppRunMotionProviderIntegrationProbe",
        ]
        app.launch()

        // 路径检查 1：probe view 必须挂载——label "motionProviderProbeResult" 存在
        // （此 a11y identifier 仅 MotionProviderProbeView 暴露；存在 = 启动 launch arg 解析 + #if DEBUG
        // probe 路径 + RootView 替代渲染全部 wired up）
        let probeLabel = app.staticTexts["motionProviderProbeResult"]
        XCTAssertTrue(
            probeLabel.waitForExistence(timeout: 15.0),
            "ProbeView label 'motionProviderProbeResult' not found—检查 PetAppApp 解析 -PetAppRunMotionProviderIntegrationProbe + DEBUG probe 路径"
        )

        // 路径检查 2：轮询 30s——result label 必须脱离 "(waiting)" 占位 **或** error label 必须出现.
        // 否则视作 wiring dead（review round 1 P3 修订）：旧版本把"30s waiting + no error"也视作 PASS，
        // 那种状态在 startUpdates 完全没被调（dead wiring）时也会出现，导致 UI test 哑掉. 本版收紧.
        let errorLabel = app.staticTexts["motionProviderProbeError"]
        let deadline = Date().addingTimeInterval(30.0)
        var resolved = false
        var lastResultLabel = "(waiting)"
        var lastErrorLabel = ""

        while Date() < deadline {
            let resultText = probeLabel.exists ? probeLabel.label : "(waiting)"
            lastResultLabel = resultText
            // result 非 "(waiting)" → 视作 happy path（handler 收到了 activity）
            if resultText != "(waiting)", !resultText.isEmpty {
                resolved = true
                break
            }
            if errorLabel.exists {
                let errText = errorLabel.label
                lastErrorLabel = errText
                if !errText.isEmpty {
                    resolved = true
                    break
                }
            }
            Thread.sleep(forTimeInterval: 1.0)
        }

        // 二态 PASS（review round 1 P3 收紧）：
        //   1) result label 脱离 "(waiting)" 占位（含 "stationary"/"walking"/"running"/"cycling"/"automotive"/"unknown"）：
        //      happy path——handler 收到了 activity event.
        //   2) error label 出现：permissionDenied / systemFailure 路径（probeView .task catch 走过）.
        //
        // **不再** 把"30s waiting + no error"视作 PASS——那种状态下 startUpdates 路径可能整段 dead
        // 而 UI test 仍然绿（review round 1 P3 指出的 dead-wiring blind spot）.
        XCTAssertTrue(
            resolved,
            """
            MotionProvider probe path 死链：30s 内 result label 既未脱离 "(waiting)" 占位，errorLabel 也未出现.
            预期至少出现以下一种：
              - resultLabel 显示具体 activity 状态（startUpdates handler 收到了 event）
              - errorLabel 显示错误（sandbox-limited / permissionDenied / systemFailure 路径走过）
            实际 lastResultLabel='\(lastResultLabel)'，lastErrorLabel='\(lastErrorLabel)'.
            诊断方向：① ProbeView .task 是否被调（PetAppApp 启动期 launch arg 解析）；
                     ② motionProvider.startUpdates handler closure 是否注册到 manager；
                     ③ Xcode 26 / iPhone 17 simulator CoreMotion 是否退化为完全静默（如是则需在 setUp 里加 simctl push activity 触发）.
            """
        )

        if !lastResultLabel.isEmpty, lastResultLabel != "(waiting)" {
            print("INFO: MotionProvider probe happy path——result='\(lastResultLabel)'")
        } else {
            print("INFO: MotionProvider probe error path（sandbox / permission）——errorLabel='\(lastErrorLabel)'；本 case 视作 PASS（路径已 wired up + 错误暴露）.")
        }
    }
}
