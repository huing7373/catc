// ErrorPresenter.swift
// Story 2.6 AC3：错误展示态的中心（含队列 + toast 自动消失定时器）。
//
// 职责：
// - 接收 `present(error:onRetry:)` / `presentToast(_:)` / `presentAlert(...)` 调用 → 转 ErrorPresentation
// - 维护当前展示项 `current`（@Published）+ 等待队列 `queue`（FIFO）
// - 为 toast 启动自动消失定时器（默认 2 秒；通过 `toastDuration` 注入测试快路径）
// - 暴露 `dismiss()` 让 SwiftUI alert 的 OK 按钮 / RetryView 的 onRetry 闭包推进队列
//
// 不职责：
// - **不**渲染 UI（UI 由 SwiftUI ViewModifier `errorPresentationHost` 订阅 `current` 完成）
// - **不**处理"重试"语义（onRetry 闭包透传给 RetryView，由调用方决定重试逻辑）
// - **不**写日志（→ Story 2.7 logger 接入后再加 sink）
//
// import 备注（继承 lesson 2026-04-25-swift-explicit-import-combine.md）：
// `ObservableObject` / `@Published` 来自 Combine，必须显式 `import Combine`。

import Foundation
import Combine

@MainActor
public final class ErrorPresenter: ObservableObject {

    /// 当前正在展示的呈现项；nil = 无。SwiftUI 视图订阅此字段渲染对应组件。
    @Published public private(set) var current: ErrorPresentation?

    /// 队列：current 消失后弹出下一个。
    /// 复合错误场景：caller 短时间内多次 present(...) 时，按 FIFO 排队；不丢弃也不合并。
    /// 元素为 `(presentation, onRetry)` tuple：让 `.retry` 入队时 onRetry 闭包随 presentation 一起排队，
    /// 等弹出时仍能驱动 caller 注入的重试动作（codex round 1 [P1] finding 修复）。
    private var queue: [(presentation: ErrorPresentation, onRetry: (() -> Void)?)] = []

    /// retry 呈现态对应的 onRetry 闭包；与 `current` 的 `.retry(...)` 配对存活。
    /// 单独存放避免污染 `ErrorPresentation` 的 Equatable 合成。
    private var pendingOnRetry: (() -> Void)?

    /// toast 自动消失定时任务的句柄；`dismiss()` 时取消（防 toast 被手动 dismiss 后定时器误触）。
    private var toastDismissTask: Task<Void, Never>?

    /// toast 自动消失时长（秒）。生产默认 2.0；测试注入 0.05 加速。
    private let toastDuration: Double

    public init(toastDuration: Double = 2.0) {
        self.toastDuration = toastDuration
    }

    // MARK: - Public API

    /// 主入口：把任意 Error 映射到 ErrorPresentation 并展示。
    /// - Parameters:
    ///   - error: 任意错误；APIError 走 `AppErrorMapper`，其他类型走 generic fallback。
    ///   - onRetry: 仅在 mapper 选定 `.retry(...)` 时使用；其他呈现样式忽略本闭包。
    ///              传 nil 时 RetryView 仍能展示，但点"重试"按钮只 dismiss（无副作用）。
    public func present(_ error: Error, onRetry: (() -> Void)? = nil) {
        let presentation = AppErrorMapper.presentation(for: error)
        enqueue(presentation, onRetry: onRetry)
    }

    /// 直接展示一条 toast（用于成功提示 / 业务无错但需要轻量反馈，如"已同步"）。
    public func presentToast(_ message: String) {
        enqueue(.toast(message: message), onRetry: nil)
    }

    /// 直接展示一条 alert（用于本地校验失败、不来自 server 的业务规则提示）。
    public func presentAlert(title: String, message: String) {
        enqueue(.alert(title: title, message: message), onRetry: nil)
    }

    /// 关闭当前展示并推进队列。供 AlertOverlay OK 按钮、RetryView 重试按钮、Toast 定时器调用。
    /// `triggerOnRetry`：仅 `.retry` 时有意义——true 时调用 caller 注入的 onRetry 闭包后再 dismiss。
    public func dismiss(triggerOnRetry: Bool = false) {
        // 取消可能进行中的 toast 自动消失任务（手动 dismiss 时清理）
        toastDismissTask?.cancel()
        toastDismissTask = nil

        // 仅 .retry 且明确请求时调 onRetry
        if triggerOnRetry, case .retry = current {
            pendingOnRetry?()
        }
        pendingOnRetry = nil
        current = nil

        // 推进队列：弹下一个（如果有），onRetry 随 presentation 一起出队
        if !queue.isEmpty {
            let next = queue.removeFirst()
            present(next.presentation, onRetry: next.onRetry)
        }
    }

    // MARK: - Private

    /// 入队 / 立即展示。current=nil → 直接 set；current 非 nil → push (presentation, onRetry) 到 queue 末尾。
    /// onRetry 与 presentation 一起排队，弹出时仍可驱动 caller 注入的重试动作。
    private func enqueue(_ presentation: ErrorPresentation, onRetry: (() -> Void)?) {
        if current == nil {
            present(presentation, onRetry: onRetry)
        } else {
            queue.append((presentation: presentation, onRetry: onRetry))
        }
    }

    /// 真正展示一项（不入队）。toast 启动自动消失定时器；alert / retry 等手动 dismiss。
    private func present(_ presentation: ErrorPresentation, onRetry: (() -> Void)?) {
        current = presentation
        pendingOnRetry = onRetry

        if case .toast = presentation {
            toastDismissTask?.cancel()
            toastDismissTask = Task { [weak self, toastDuration] in
                try? await Task.sleep(nanoseconds: UInt64(toastDuration * 1_000_000_000))
                guard !Task.isCancelled else { return }
                self?.handleToastTimeout()
            }
        }
    }

    /// toast 定时器触发：仅当当前仍是 toast 时 dismiss（防止用户在 2s 内手动 dismiss 后定时器还触发）。
    private func handleToastTimeout() {
        guard case .toast = current else { return }
        dismiss()
    }
}
