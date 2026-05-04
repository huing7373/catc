// MotionProviderIntegrationTests.swift
// Story 8.2 AC7: MotionProviderImpl 在模拟器 CoreMotion 上的集成测试.
//
// 流程：
//   1. launch arguments 含 -PetAppRunMotionProviderIntegrationProbe →
//      PetAppApp.body 渲染 MotionProviderProbeView 替代 RootView
//   2. ProbeView .task 串行调 motionProvider.requestPermission → motionProvider.startUpdates →
//      startUpdates 注册完毕立即 set status label "motionProviderProbeStatus" = "subscribed"
//   3. 本测试 wait for label "motionProviderProbeStatus" → 30s 内轮询
//      要么 status label 显示 "subscribed"（happy path：requestPermission grant + startUpdates 已注册）
//      要么 errorLabel "motionProviderProbeError" 显示错误（permissionDenied / systemFailure 路径）
//
// 红线（详见 story 8-2-coremotion-接入.md AC7 段，沿用 8.1 红线 + review round 1 P3 + round 2 P2 修订）：
//   - **不依赖** simulator 自发 emit CMMotionActivity event——CoreMotion idle 时 30s 都不会 emit
//     activity，那条路径不可靠（round 2 P2）；改为断言 startUpdates 调用完成后立即写入的
//     "subscribed" status marker，覆盖 wiring 链路（launch arg → probe mount → requestPermission
//     grant → startUpdates 调用完成）.
//   - 旧版本（round 1 修订）要求 result label 必须脱离 "(waiting)" 才算 happy path，依赖 simulator
//     自发 motion event；round 2 P2 指出该断言在 idle simulator 上 nondeterministic / flaky.
//   - 二态 PASS：statusLabel == "subscribed" **或** errorLabel 非空.
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

        // 路径检查 2：轮询 30s——statusLabel 必须出现 "subscribed" 文本 **或** error label 必须出现.
        // round 2 P2：不再依赖 simulator 自发 emit activity event（idle CoreMotion 不会发），
        // 改为断言 startUpdates 调用完成后写入的 "subscribed" marker——覆盖 wiring 全链路
        // （launch arg 解析 + probe mount + requestPermission grant + startUpdates 调用完成）.
        let statusLabel = app.staticTexts["motionProviderProbeStatus"]
        let errorLabel = app.staticTexts["motionProviderProbeError"]
        let deadline = Date().addingTimeInterval(30.0)
        var resolved = false
        var lastStatusLabel = "(idle)"
        var lastResultLabel = "(waiting)"
        var lastErrorLabel = ""

        while Date() < deadline {
            let statusText = statusLabel.exists ? statusLabel.label : "(idle)"
            lastStatusLabel = statusText
            // status == "subscribed" → happy path（startUpdates 已注册，wiring 全链路通）
            if statusText == "subscribed" {
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
            // 也读 result label——纯诊断用（happy path 时 simulator 偶尔会 emit activity，记下 last 值方便 fail 时排查）
            if probeLabel.exists {
                lastResultLabel = probeLabel.label
            }
            Thread.sleep(forTimeInterval: 1.0)
        }

        // 二态 PASS（review round 2 P2 修订）：
        //   1) statusLabel == "subscribed"：happy path——requestPermission grant + startUpdates 调用完成.
        //   2) errorLabel 出现：permissionDenied / systemFailure 路径（probeView .task catch / deny return 走过）.
        //
        // **不再** 依赖 simulator 自发 emit activity event——round 2 P2 指出 CoreMotion idle 时
        // 30s 都不发 activity，那条断言路径在 CI 上 flaky.
        XCTAssertTrue(
            resolved,
            """
            MotionProvider probe path 死链：30s 内 statusLabel 既未变成 "subscribed"，errorLabel 也未出现.
            预期至少出现以下一种：
              - statusLabel 显示 "subscribed"（startUpdates 调用完成；happy path）
              - errorLabel 显示错误（permissionDenied / systemFailure 路径走过）
            实际 lastStatusLabel='\(lastStatusLabel)'，lastResultLabel='\(lastResultLabel)'，lastErrorLabel='\(lastErrorLabel)'.
            诊断方向：① ProbeView .task 是否被调（PetAppApp 启动期 launch arg 解析）；
                     ② motionProvider.requestPermission 是否 grant（false 路径会写 errorLabel="permissionDenied"，应在上面捕获）；
                     ③ motionProvider.startUpdates 是否被调到（statusLabel 写入紧跟其后）.
            """
        )

        if lastStatusLabel == "subscribed" {
            print("INFO: MotionProvider probe happy path——statusLabel='subscribed'，lastResult='\(lastResultLabel)'.")
        } else {
            print("INFO: MotionProvider probe error path（permission / sandbox / systemFailure）——errorLabel='\(lastErrorLabel)'；本 case 视作 PASS（路径已 wired up + 错误暴露）.")
        }
    }
}
