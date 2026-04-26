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
}
