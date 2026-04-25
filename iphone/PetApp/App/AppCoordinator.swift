// AppCoordinator.swift
// Story 2.3 落地：iPhone App 全屏 Sheet 集中状态管理。
//
// 路由模式：
//   - 主界面三个 CTA 按钮（进入房间 / 仓库 / 合成）通过 RootView 注入的闭包
//     调用 coordinator.present(.room/.inventory/.compose) 触发 Sheet 弹出。
//   - Sheet 内部点击"关闭"按钮调用 coordinator.dismiss() 返回主界面。
//   - SwiftUI .fullScreenCover(item: $coordinator.presentedSheet) 双向绑定：
//     系统 dismiss 时自动把 presentedSheet 复位为 nil。
//
// 后续扩展：Epic 12 / 24 / 33 实装真实 RoomView / InventoryView / ComposeView 时，
// SheetType 枚举值不变，仅在 RootView.sheetContent(for:) 中替换返回的真实 view。
//
// 重要约束：
//   - 顶部显式 import Combine（Story 2.2 review lesson learned：
//     ObservableObject / @Published 不能依赖 SwiftUI 隐式 transitive import）。
//   - @MainActor：与 HomeViewModel 同风格；coordinator 状态仅在 UI 主线程读写。

import Foundation
import Combine
import SwiftUI

/// Sheet 路由枚举：列举主界面可弹出的全屏 Sheet 类型。
///
/// Identifiable：SwiftUI `.fullScreenCover(item:)` 要求 item 类型符合 Identifiable，
/// 通过 id 区分"同一类 sheet"vs"切到另一类 sheet"。
///
/// Equatable：测试中 `XCTAssertEqual(coordinator.presentedSheet, .room)` 需要。
public enum SheetType: Identifiable, Equatable {
    case room
    case inventory
    case compose

    public var id: String {
        switch self {
        case .room: return "sheet_room"
        case .inventory: return "sheet_inventory"
        case .compose: return "sheet_compose"
        }
    }
}

/// 集中管理 App 内全屏 Sheet 的状态。
///
/// 主界面三个 CTA 按钮通过 coordinator.present(.room/.inventory/.compose) 触发 Sheet 弹出。
/// Sheet 内部点击关闭按钮调用 coordinator.dismiss() 返回主界面。
@MainActor
public final class AppCoordinator: ObservableObject {
    @Published public var presentedSheet: SheetType?

    public init(presentedSheet: SheetType? = nil) {
        self.presentedSheet = presentedSheet
    }

    /// 弹出指定类型的 Sheet。
    ///
    /// 已有 Sheet 时被新值覆盖（@Published 直接赋值即可）。
    /// SwiftUI .fullScreenCover(item:) 在 item 从非 nil 切到非 nil 时
    /// 会先 dismiss 旧 sheet 再 present 新 sheet（系统行为，本 story 不改）。
    public func present(_ sheet: SheetType) {
        presentedSheet = sheet
    }

    /// 关闭当前 Sheet（即使已是 nil 也安全：直接覆盖赋值）。
    public func dismiss() {
        presentedSheet = nil
    }
}
