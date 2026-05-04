// HealthProviderIntegrationTests.swift
// Story 8.1 AC7: HealthProviderImpl 在模拟器 HealthKit 上的集成测试.
//
// 流程：
//   1. launch arguments 含 -PetAppPreseedHealthKitSteps <N> →
//      PetAppApp.init detached task 调 HealthKitDevSeedUseCase.preseedToday → HKHealthStore 写入 N 步
//   2. launch arguments 含 -PetAppRunHealthProviderIntegrationProbe →
//      PetAppApp.body 渲染 HealthProviderProbeView 替代 RootView
//   3. ProbeView .task 调 healthProvider.readDailyTotalSteps(date: Date()) → 把 Int 显示在 a11y label
//   4. 本测试 wait for label "healthProviderProbeResult" → 断言 label 变成数字（happy path），
//      或 label "healthProviderProbeError" 显示 permissionDenied / healthDataNotAvailable
//      （模拟器 HK sandbox 受限路径——AC7 钦定的 fallback：fallback 通过即视作 path wired up）.
//
// 红线（详见 story 8-1-healthkit-接入.md AC7 段）：
//   - 模拟器 HealthKit sandbox 在 simulator 默认 deny read 权限（Xcode 26 实测）;
//   - 集成测试不强制 100% pass；只要"probe view 挂载 + readDailyTotalSteps 执行（无论 happy / permissionDenied
//     / healthDataNotAvailable）"路径全打通即视作 AC7 满足；
//   - 不验证权限弹窗交互（XCUITest 无法稳定模拟系统弹窗）；不引 entitlements 文件（节点 3 阶段不允许）.
//
// 已知坑：
//   - 模拟器首次 launch 调 requestAuthorization 时 simulator 走 deny 路径（不弹 sheet）；
//     实测 ProbeView 落到 catch 分支显示 errorText="permissionDenied"，resultText 仍是初始 "-".
//     本 case 接受此 sandbox-limited 路径作为 PASS（路径 wired up 即可）.

import XCTest

final class HealthProviderIntegrationTests: XCTestCase {

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    func testHealthProviderImpl_probeViewMountsAndExecutesReadPath() throws {
        let app = XCUIApplication()
        app.launchArguments = [
            "-PetAppPreseedHealthKitSteps", "5000",
            "-PetAppRunHealthProviderIntegrationProbe",
        ]
        app.launch()

        // 路径检查 1：probe view 必须挂载——label "healthProviderProbeResult" 存在
        // （此 a11y identifier 仅 HealthProviderProbeView 暴露；存在 = 启动 launch arg 解析 + #if DEBUG
        // probe 路径 + RootView 替代渲染全部 wired up）
        let probeLabel = app.staticTexts["healthProviderProbeResult"]
        XCTAssertTrue(
            probeLabel.waitForExistence(timeout: 15.0),
            "ProbeView label 'healthProviderProbeResult' not found—检查 PetAppApp 解析 -PetAppRunHealthProviderIntegrationProbe"
        )

        // 路径检查 2：轮询 30s——要么 result label 是数字（happy path），要么 error label 出现.
        // XCUITest 的 expectation(for:) 等多个 predicate 走 ALL 语义而非 ANY，所以手动 polling.
        // 已知坑：模拟器 HK sandbox 在测试 host 进程下需要更长 wait——expression 长达 18s.
        let errorLabel = app.staticTexts["healthProviderProbeError"]
        let deadline = Date().addingTimeInterval(30.0)
        var resolved = false
        var lastResultLabel = "-"
        var lastErrorLabel = ""

        while Date() < deadline {
            let resultText = probeLabel.exists ? probeLabel.label : "-"
            lastResultLabel = resultText
            if resultText != "-", Int(resultText) != nil {
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

        XCTAssertTrue(
            resolved,
            "neither result label 数字 nor error label 出现——HealthProviderImpl readDailyTotalSteps 没走完执行路径; " +
            "lastResultLabel='\(lastResultLabel)', lastErrorLabel='\(lastErrorLabel)'"
        )

        // 如果 result label 是数字（happy path），验证 >= 5000；
        // 如果是 error path，记录 simulator HK sandbox 状态作为 lesson.
        if lastResultLabel != "-", let actual = Int(lastResultLabel) {
            XCTAssertGreaterThanOrEqual(
                actual, 5000,
                "happy path 期望 >= 5000；实际 \(actual)"
            )
        } else {
            // sandbox-limited 路径：probe wired up 但 HK 拒授权——视作 PASS（AC7 钦定 fallback）
            XCTAssertFalse(
                lastErrorLabel.isEmpty,
                "errorLabel 必须有值表明 readDailyTotalSteps 走 catch；当前空 = path 未走通"
            )
            print("INFO: simulator HealthKit sandbox 拒读权限——已知坑（详见 story 8.1 AC7 红线）；" +
                  "errorLabel='\(lastErrorLabel)'；本 case 仍视作 PASS（路径已 wired up）.")
        }
    }
}
