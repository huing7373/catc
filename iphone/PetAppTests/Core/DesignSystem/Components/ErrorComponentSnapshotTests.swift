// ErrorComponentSnapshotTests.swift
// Story 2.6 AC9：错误 UI 组件的轻量行为校验（不做截图比对）。
//
// 真正的截图回归测试 future 接入 swift-snapshot-testing 时再补（不在本 story 范围）。
// 这里只断言 ToastView / AlertOverlayView / RetryView 持有正确的参数，
// 以及 ErrorPresentationHostModifier 能正确构造（不调 body 避开 SwiftUI host 环境依赖）。

import XCTest
import SwiftUI
@testable import PetApp

@MainActor
final class ErrorComponentSnapshotTests: XCTestCase {

    /// case#1：ToastView 渲染传入 message
    func testToastViewRendersMessage() {
        let view = ToastView(message: "已同步")
        XCTAssertEqual(view.message, "已同步")
    }

    /// case#2：AlertOverlayView 持有 title / message / onDismiss
    func testAlertOverlayViewHoldsParameters() {
        var dismissed = false
        let view = AlertOverlayView(title: "提示", message: "数据异常", onDismiss: { dismissed = true })
        XCTAssertEqual(view.title, "提示")
        XCTAssertEqual(view.message, "数据异常")
        view.onDismiss()
        XCTAssertTrue(dismissed)
    }

    /// case#3：RetryView 持有 message / onRetry
    func testRetryViewHoldsParameters() {
        var retried = false
        let view = RetryView(message: "网络异常", onRetry: { retried = true })
        XCTAssertEqual(view.message, "网络异常")
        view.onRetry()
        XCTAssertTrue(retried)
    }

    /// case#4：ErrorPresentationHostModifier 订阅 presenter.current 变化（轻量行为校验）
    /// 只验证 ViewModifier 能正确构造且 presenter 引用挂载成功；具体渲染走 ErrorPresenterTests。
    func testErrorPresentationHostModifierConstructible() {
        let presenter = ErrorPresenter()
        let modifier = ErrorPresentationHostModifier(presenter: presenter)
        // 不调 modifier.body —— SwiftUI ViewModifier 在没有 host SwiftUI 环境时调 body 会触发各种 layout assertion
        XCTAssertTrue(modifier.presenter === presenter)
    }

    // MARK: - Modal blocking 语义（fix-review round 2 / P2）
    //
    // 背景：codex round 2 指出 alert 路径下 underlying content 仍可被 VoiceOver focus + tap，
    // 违反 alert 的"必须通过'知道了'退场"语义。修复后 alert / retry 都通过 `modalActive` 计算属性
    // 同步给下层 content 上 `.accessibilityHidden(true)` + `.allowsHitTesting(false)`。
    // 这里直接断言 `modalActive` 计算逻辑，绕开 SwiftUI host 环境。

    /// case#5：current = nil → 非 modal，下层不屏蔽
    func testModalActiveFalseWhenCurrentIsNil() {
        let presenter = ErrorPresenter()
        let modifier = ErrorPresentationHostModifier(presenter: presenter)
        XCTAssertNil(presenter.current)
        XCTAssertFalse(modifier.modalActive)
        XCTAssertFalse(modifier.retryActive)
    }

    /// case#6：toast 不是 modal，下层正常可交互
    func testModalActiveFalseForToast() {
        let presenter = ErrorPresenter()
        presenter.presentToast("已同步")
        let modifier = ErrorPresentationHostModifier(presenter: presenter)
        guard case .toast = presenter.current else {
            return XCTFail("expected current = .toast, got \(String(describing: presenter.current))")
        }
        XCTAssertFalse(modifier.modalActive, "toast 不是 modal，下层应保持可交互")
        XCTAssertFalse(modifier.retryActive)
    }

    /// case#7：alert 是 modal，下层 hit-testing + a11y 必须屏蔽
    /// 这是本 round fix 的核心断言——回归 codex round 2 的 P2 finding。
    func testAlertHidesUnderlyingContentForAccessibility() {
        let presenter = ErrorPresenter()
        presenter.presentAlert(title: "提示", message: "数据异常")
        let modifier = ErrorPresentationHostModifier(presenter: presenter)
        guard case .alert = presenter.current else {
            return XCTFail("expected current = .alert, got \(String(describing: presenter.current))")
        }
        XCTAssertTrue(
            modifier.modalActive,
            "alert 必须 modal block——下层 accessibilityHidden + allowsHitTesting=false"
        )
        XCTAssertFalse(modifier.retryActive, "alert 不是 retry，下层 opacity 保留 1")
    }

    /// case#8：retry 是 modal，下层 hit-testing + a11y + opacity 全屏蔽
    /// retry 没有公开 enqueue 入口——通过 APIError.network（mapper 映射成 .retry）触发。
    func testRetryHidesUnderlyingContentForAccessibility() {
        let presenter = ErrorPresenter()
        let underlying = NSError(domain: NSURLErrorDomain, code: NSURLErrorNotConnectedToInternet)
        presenter.present(APIError.network(underlying: underlying))
        let modifier = ErrorPresentationHostModifier(presenter: presenter)
        guard case .retry = presenter.current else {
            return XCTFail("expected current = .retry, got \(String(describing: presenter.current))")
        }
        XCTAssertTrue(modifier.modalActive, "retry 必须 modal block")
        XCTAssertTrue(modifier.retryActive, "retry 下层 opacity 应置 0")
    }
}
