// ErrorPresenterTests.swift
// Story 2.6 AC9：ErrorPresenter 单元测试。
//
// 覆盖：
// - present(_:onRetry:) 主入口（business / network + onRetry 暂存）
// - dismiss(triggerOnRetry:) 默认不触发 vs 显式触发
// - 队列：连续 present 时 FIFO 推进
// - Toast 自动消失 + 后续推进队列
// - presentAlert / presentToast 辅助入口
// - dismiss 边界：current=nil 不崩 / 手动 dismiss 取消 toast 定时器

import XCTest
@testable import PetApp

@MainActor
final class ErrorPresenterTests: XCTestCase {

    // MARK: - present(_:onRetry:) 主入口

    /// case#1：.business → presenter.current = .alert
    func testPresentBusinessErrorSetsAlertPresentation() {
        let presenter = ErrorPresenter()
        presenter.present(APIError.business(code: 3002, message: "x", requestId: "y"))
        XCTAssertEqual(presenter.current, .alert(title: "提示", message: "步数不足，再走走吧"))
    }

    /// case#2：.network → presenter.current = .retry；onRetry 闭包暂存
    func testPresentNetworkErrorSetsRetryPresentationAndStoresOnRetry() {
        let presenter = ErrorPresenter()
        var retryCallCount = 0

        presenter.present(APIError.network(underlying: URLError(.timedOut)), onRetry: {
            retryCallCount += 1
        })
        XCTAssertEqual(presenter.current, .retry(message: "网络异常，请检查后重试"))
        XCTAssertEqual(retryCallCount, 0, "onRetry 不应在 present 时立即触发")

        // dismiss(triggerOnRetry: true) 触发 onRetry 后退场
        presenter.dismiss(triggerOnRetry: true)
        XCTAssertEqual(retryCallCount, 1, "RetryView 重试按钮应触发 onRetry 一次")
        XCTAssertNil(presenter.current)
    }

    /// case#3：dismiss(triggerOnRetry: false) 默认不触发 onRetry（如 alert OK 按钮路径）
    func testDismissWithoutTriggerOnRetryDoesNotInvokeOnRetry() {
        let presenter = ErrorPresenter()
        var retryCallCount = 0
        presenter.present(APIError.network(underlying: URLError(.timedOut)), onRetry: {
            retryCallCount += 1
        })
        presenter.dismiss()   // 不传 triggerOnRetry
        XCTAssertEqual(retryCallCount, 0)
        XCTAssertNil(presenter.current)
    }

    // MARK: - 队列：连续 present(...) 时 FIFO 推进

    /// 队列项携带 onRetry —— present(.toast)（占用 current）+ present(.network, onRetry: spy)（入队） →
    /// dismiss toast → retry 弹出 → dismiss(triggerOnRetry: true) → spy 被调用 1 次。
    /// 防回归：codex round 1 [P1] finding "enqueue() 丢 onRetry callback"。
    /// 用 case 模式而非文案断言，避开 mapper 文案变更对测试的耦合。
    func testQueuedRetryPreservesOnRetryCallback() {
        let presenter = ErrorPresenter(toastDuration: 60)   // 长时长避免 toast 自动消失干扰
        var retryCallCount = 0

        // 第一次 present：toast 占用 current
        presenter.presentToast("first")
        XCTAssertEqual(presenter.current, .toast(message: "first"))

        // 第二次 present：.retry 入队（带 onRetry 闭包）
        presenter.present(APIError.network(underlying: URLError(.timedOut)), onRetry: {
            retryCallCount += 1
        })
        // current 仍是 toast；retry 在队列中（不应直接调用 onRetry）
        XCTAssertEqual(presenter.current, .toast(message: "first"))
        XCTAssertEqual(retryCallCount, 0, "入队期间 onRetry 不应触发")

        // dismiss toast → retry 弹出（具体 message 由 mapper 决定，case 模式断言）
        presenter.dismiss()
        guard case .retry = presenter.current else {
            return XCTFail("dismiss toast 后 current 应是 .retry，实际: \(String(describing: presenter.current))")
        }
        XCTAssertEqual(retryCallCount, 0, "队列推进到 retry 时仍不应触发 onRetry")

        // 用户点 retry 按钮 → dismiss(triggerOnRetry: true) → onRetry 被调用一次
        presenter.dismiss(triggerOnRetry: true)
        XCTAssertEqual(retryCallCount, 1, "队列里入队的 onRetry 闭包应在 retry 弹出后被 triggerOnRetry 触发")
        XCTAssertNil(presenter.current)
    }

    /// case#4：第二次 present 时 current 仍非 nil → 入队；dismiss 后弹下一个
    func testSecondPresentEnqueuesAndDismissAdvancesQueue() {
        let presenter = ErrorPresenter()

        presenter.present(APIError.business(code: 3002, message: "", requestId: ""))
        let firstAlert = presenter.current
        XCTAssertEqual(firstAlert, .alert(title: "提示", message: "步数不足，再走走吧"))

        // 第二次 present：入队（current 不变）
        presenter.present(APIError.business(code: 4002, message: "", requestId: ""))
        XCTAssertEqual(presenter.current, firstAlert, "current 不应被第二次 present 覆盖")

        // dismiss 后弹出队列中的 4002
        presenter.dismiss()
        XCTAssertEqual(presenter.current, .alert(title: "提示", message: "宝箱尚未解锁"))

        // 再 dismiss 队列空了
        presenter.dismiss()
        XCTAssertNil(presenter.current)
    }

    // MARK: - Toast 自动消失

    /// case#5：toast 在 toastDuration 后自动消失
    func testToastAutoDismissesAfterToastDuration() async {
        let presenter = ErrorPresenter(toastDuration: 0.05)   // 50ms 加速
        presenter.presentToast("已同步")
        XCTAssertEqual(presenter.current, .toast(message: "已同步"))

        // 等到自动消失（多给点缓冲，应对测试机调度抖动）
        try? await Task.sleep(nanoseconds: 200_000_000)   // 200ms
        XCTAssertNil(presenter.current, "toast 应在 toastDuration 后自动消失")
    }

    /// case#6：toast 自动消失后续推进队列
    func testToastAutoDismissAdvancesQueue() async {
        let presenter = ErrorPresenter(toastDuration: 0.05)
        presenter.presentToast("first")
        presenter.presentAlert(title: "提示", message: "second")   // 入队

        // 等 toast 自动消失
        try? await Task.sleep(nanoseconds: 200_000_000)
        XCTAssertEqual(presenter.current, .alert(title: "提示", message: "second"))
    }

    // MARK: - presentAlert / presentToast 辅助入口

    /// case#7：presentAlert 直接展示 alert
    func testPresentAlertShowsAlertImmediately() {
        let presenter = ErrorPresenter()
        presenter.presentAlert(title: "提示", message: "本地校验失败")
        XCTAssertEqual(presenter.current, .alert(title: "提示", message: "本地校验失败"))
    }

    /// case#8：presentToast 直接展示 toast（**不**自动消失测试在 case#5）
    func testPresentToastShowsToastImmediately() {
        let presenter = ErrorPresenter(toastDuration: 60)   // 长时长避免自动消失干扰
        presenter.presentToast("已同步")
        XCTAssertEqual(presenter.current, .toast(message: "已同步"))
    }

    // MARK: - dismiss 边界

    /// case#9：current=nil 时 dismiss 不崩
    func testDismissWhenIdleIsNoOp() {
        let presenter = ErrorPresenter()
        presenter.dismiss()
        XCTAssertNil(presenter.current)
    }

    /// case#10：手动 dismiss 取消 toast 自动消失定时器（防过期触发覆盖后续展示项）
    func testManualDismissCancelsToastAutoDismissTimer() async {
        let presenter = ErrorPresenter(toastDuration: 0.05)
        presenter.presentToast("first")

        // 立即手动 dismiss 并 push 一个 alert（current 应是 alert，不应被定时器误清）
        presenter.dismiss()
        presenter.presentAlert(title: "提示", message: "after-manual-dismiss")
        XCTAssertEqual(presenter.current, .alert(title: "提示", message: "after-manual-dismiss"))

        // 等到定时器原本应该触发的时间窗
        try? await Task.sleep(nanoseconds: 200_000_000)

        // alert 不应被取消的定时器误清
        XCTAssertEqual(presenter.current, .alert(title: "提示", message: "after-manual-dismiss"),
                       "已取消的 toast 定时器不应误清后续 alert")
    }
}
