// AppCoordinator.swift
// Story 2.3 落地：iPhone App 全屏 Sheet 集中状态管理（部分 supersede 见下）.
//
// Story 37.3 修改（ADR-0009 §3.4 + §3.5 步骤 5）：
//   - 删 `.room` / `.inventory` SheetType case —— 主入口从 3 CTA + Sheet 改为 4 Tab + HomeContainer
//     互斥状态机，Sheet 路由仅保留次级 sheet（`.compose`）.
//   - 加 `@Published currentTab: AppTab = .home` —— TabView selection 的 single source of truth.
//   - 加 `switchTab(_:)` 方法 —— 程式化切 Tab 入口（深 link / 跨 ViewModel 跳转用）.
//   - 加 `@Published currentRoomId: String?` 临时占位字段（Story 37.4 ↔ 37.3 并行方案；
//     Story 37.4 落地 AppState 后 dev 把所有 `coordinator.currentRoomId` 引用改为
//     `appState.currentRoomId` + 删除本字段；详见 Story 37.3 §AC8 与 Dev Notes
//     "HomeContainerView 互斥状态机：appState 注入路径选择"）.
//
// 路由模式（Story 37.3 后）：
//   - 主入口 = MainTabView 4 Tab；用户点 Tab → coordinator.currentTab 改 → TabView 切.
//   - HomeView 主入口的"加入队伍" / "创建队伍"按钮（Story 37.7 落地）写 currentRoomId →
//     HomeContainerView 切到 RoomViewPlaceholder 互斥态.
//   - 次级 Sheet（`.compose`，Story 33.1 决定具体形式）通过 coordinator.present(.compose) 弹出.
//
// 后续扩展：Story 37.4 落地 AppState.currentRoomId 后删除本类的 currentRoomId 占位字段；
// Story 33.1 决定是否保留 .compose case 还是改路由模式.
//
// 重要约束：
//   - 顶部显式 import Combine（Story 2.2 review lesson learned：
//     ObservableObject / @Published 不能依赖 SwiftUI 隐式 transitive import）.
//   - @MainActor：与 HomeViewModel 同风格；coordinator 状态仅在 UI 主线程读写.

import Foundation
import Combine
import SwiftUI

/// Sheet 路由枚举：Story 37.3 后缩窄到次级 sheet（不再含主入口）.
///
/// Story 37.3 删除：`.room` 与 `.inventory` —— Home Tab 互斥状态机 / Wardrobe Tab 直接路由接管.
///
/// Identifiable：SwiftUI `.sheet(item:)` 要求 item 类型符合 Identifiable.
/// Equatable：测试中 `XCTAssertEqual(coordinator.presentedSheet, .compose)` 需要.
public enum SheetType: Identifiable, Equatable {
    case compose       // 保留：合成 sub-flow（Story 33.1 决定具体形式）.

    public var id: String {
        switch self {
        case .compose: return "sheet_compose"
        }
    }
}

/// AppCoordinator 角色变化（ADR-0009 §3.4）：
/// - 旧职责：主入口 sheet 路由（.room / .inventory / .compose）.
/// - 新职责：
///   1. presentedSheet 仅含次级 sheet（.compose）.
///   2. currentTab @Published：TabView selection 的 single source of truth.
///   3. switchTab 方法：程式化切 Tab 入口.
///   4. currentRoomId @Published（**临时占位**）：Story 37.4 落地 AppState 后由 dev 删除.
@MainActor
public final class AppCoordinator: ObservableObject {
    @Published public var presentedSheet: SheetType?
    @Published public var currentTab: AppTab = .home

    /// Story 37.3 临时占位字段（Story 37.4 ↔ 37.3 并行方案）：
    /// HomeContainerView 用 `coordinator.currentRoomId` 作为互斥状态机决策入参；
    /// Story 37.4 落地真实 AppState 后 dev 把所有引用改为 `appState.currentRoomId` + 删除本字段.
    /// 详见 Story 37.3 §AC8 + Dev Notes "HomeContainerView 互斥状态机：appState 注入路径选择".
    @Published public var currentRoomId: String?

    public init(
        presentedSheet: SheetType? = nil,
        currentTab: AppTab = .home,
        currentRoomId: String? = nil
    ) {
        self.presentedSheet = presentedSheet
        self.currentTab = currentTab
        self.currentRoomId = currentRoomId
    }

    /// 弹出指定类型的 Sheet.
    public func present(_ sheet: SheetType) {
        presentedSheet = sheet
    }

    /// 关闭当前 Sheet（即使已是 nil 也安全：直接覆盖赋值）.
    public func dismiss() {
        presentedSheet = nil
    }

    /// 程式化切换 Tab（如深 link、跨 ViewModel 跳转）.
    public func switchTab(_ tab: AppTab) {
        currentTab = tab
    }
}
