// ErrorPresentation.swift
// Story 2.6 AC2：错误呈现样式 + 关联文案。
//
// 三态对应 epics.md Story 2.6 的三个 UI 组件：
// - `.toast(message:)` → `ToastView`：顶部短暂浮现，2 秒自动消失。
// - `.alert(title:message:)` → `AlertOverlayView`：全屏阻塞 alert，单 OK 按钮。
// - `.retry(message:)` → `RetryView`：全屏 placeholder + 重试按钮，由调用方注入 `onRetry` 闭包。
//
// 设计选择：
// - `Equatable` 用于测试断言；`Identifiable` 用于 SwiftUI 的 `.fullScreenCover(item:)` / `.sheet(item:)` 等需要
//   stable id 的 modifier（虽然本 story 用 `.overlay(...)` 而非 sheet，仍 conform 让未来切换轻便）。
// - **不**带 `onRetry: () -> Void` 闭包字段：闭包不是 Equatable 一等公民，会让 Equatable 合成失败。
//   `RetryView` 的 onRetry 闭包由 `ErrorPresenter.present(_:onRetry:)` 单独传，存在 presenter 内部
//   的私有 `pendingOnRetry` 字段（详见 Dev Note #8）。

import Foundation

public enum ErrorPresentation: Equatable, Identifiable {
    case toast(message: String)
    case alert(title: String, message: String)
    case retry(message: String)

    /// 用 String(describing:) 拼 id：保证不同 case 的 id 一定不同；
    /// 同 case 不同 message 的 id 也不同（避免 alert 队列里两个 "数据异常" 被 SwiftUI 当作同一项）。
    public var id: String {
        switch self {
        case let .toast(message):
            return "toast::\(message)"
        case let .alert(title, message):
            return "alert::\(title)::\(message)"
        case let .retry(message):
            return "retry::\(message)"
        }
    }
}
