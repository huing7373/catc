// MainTabView.swift
// Story 37.3: 主入口 4 Tab 浮动 TabBar 容器（ADR-0009 §3.5 步骤 2）.
//
// 功能：
//   - 持有 4 个 Tab 根视图 (HomeContainerView / WardrobeView / FriendsView / ProfileView)
//   - 与 AppCoordinator.currentTab 双向绑定（程式化切 Tab + 用户点击 Tab 都走同一 source）
//   - 隐藏 SwiftUI 默认 TabBar，自绘浮动 FloatingTabBar overlay（ui_design §iOS 设备规格）
//
// 关键设计：
//   - `AppTab` 而非 `Tab` 命名（Story 37.3 Dev Notes "Tab enum 类型放置 + 命名空间策略"
//     明确：iOS 18 起 SwiftUI 内置 `SwiftUI.Tab` 类型 → App 自定义类型用 `AppTab` 显式区分）
//   - 4 个 a11y identifier 走 inline 字符串 `tab_<rawValue>`（Story 37.13 a11y 总表归并时改常量）
//   - TabView 通过 `.toolbar(.hidden, for: .tabBar)` 隐藏默认 TabBar；浮动 TabBar 用
//     `.safeAreaInset(edge: .bottom)` 让 SwiftUI 自动给内容预留 safe area，避免硬算 padding
//   - TabBar 视觉数值（72pt 高 / 14pt 距底 / 12pt 距左右 / 圆角 20）按 ui_design §iOS 设备
//     规格硬编码占位；Story 37.5 接 theme 后改 token

import SwiftUI

/// Tab enum：MainTabView selection binding 的 type-safe 标识.
///
/// 命名 `AppTab` 而非 `Tab`：iOS 18 起 SwiftUI 引入内置 `SwiftUI.Tab` 类型（用于 TabView modifier
/// 内部）；App 自定义类型用 `AppTab` 显式区分，避免命名冲突 / 调用站点歧义.
///
/// CaseIterable + Identifiable：让 ForEach + a11y identifier 自动衍生.
public enum AppTab: String, CaseIterable, Identifiable {
    case home, wardrobe, friends, profile

    public var id: String { rawValue }
}

public struct MainTabView: View {
    @EnvironmentObject var coordinator: AppCoordinator

    public init() {}

    public var body: some View {
        TabView(selection: $coordinator.currentTab) {
            HomeContainerView().tag(AppTab.home)
            WardrobeView().tag(AppTab.wardrobe)        // Story 37.9 实装真实内容；本 story 占位 stub
            FriendsView().tag(AppTab.friends)          // Story 37.10 实装真实内容；本 story 占位 stub
            ProfileView().tag(AppTab.profile)          // Story 37.11 实装真实内容；本 story 占位 stub
        }
        // 隐藏 SwiftUI 默认 TabBar（自绘浮动 overlay 取代之）.
        .toolbar(.hidden, for: .tabBar)
        // 用 safeAreaInset 让 SwiftUI 自动为内容预留底部 safe area，
        // 避免内容被浮动 TabBar 遮挡（Story 37.3 Dev Notes "iOS 17+ TabView API 偏差" 钦定路径）.
        .safeAreaInset(edge: .bottom) {
            FloatingTabBar(selection: $coordinator.currentTab)
                .padding(.horizontal, 12)
                .padding(.bottom, 14)
        }
    }
}

/// 浮动自绘 TabBar：高 72pt + 距底 14pt + 距左右 12pt + 圆角 20.
/// Story 37.7 HomeView Scaffold 落地时同步把 Color 改用 theme.colors / shadow 改用 theme.shadow
/// （Story 37.5 已落地 Theme 类型契约，可直接消费；本 story 不强制收口该硬编码占位）.
private struct FloatingTabBar: View {
    @Binding var selection: AppTab

    var body: some View {
        HStack(spacing: 0) {
            ForEach(AppTab.allCases) { tab in
                tabButton(tab)
            }
        }
        .frame(height: 72)
        .background(Color(.systemBackground))
        .cornerRadius(20)
        .shadow(color: Color.black.opacity(0.14), radius: 16, x: 0, y: 6)
    }

    private func tabButton(_ tab: AppTab) -> some View {
        Button(action: { selection = tab }) {
            VStack(spacing: 4) {
                Image(systemName: iconName(for: tab))
                    .font(.system(size: 22))
                    .scaleEffect(selection == tab ? 1.1 : 1.0)
                Text(label(for: tab))
                    .font(.caption2)
            }
            .frame(maxWidth: .infinity)
            .foregroundColor(selection == tab ? .accentColor : .secondary)
        }
        .accessibilityIdentifier("tab_\(tab.rawValue)")
    }

    private func iconName(for tab: AppTab) -> String {
        switch tab {
        case .home: return "house.fill"
        case .wardrobe: return "shippingbox.fill"
        case .friends: return "person.2.fill"
        case .profile: return "person.crop.circle.fill"
        }
    }

    private func label(for tab: AppTab) -> String {
        switch tab {
        case .home: return "家"
        case .wardrobe: return "仓库"
        case .friends: return "好友"
        case .profile: return "我的"
        }
    }
}

#if DEBUG
struct MainTabView_Previews: PreviewProvider {
    static var previews: some View {
        // Story 37.4: HomeContainerView 改读 @EnvironmentObject AppState；
        // Preview 也必须注入空态 AppState 让子树渲染不 crash.
        // Story 37.7 codex round 1 [P1] fix：Preview 也注入 `MockHomeViewModel`（而非裸 HomeViewModel），
        // 防开发者在 Preview 内点 actionRow / teamIdleCard 触发基类 fatalError. MainTabView 持的
        // `@EnvironmentObject var homeViewModel: HomeViewModel` 接受任意 HomeViewModel 子类.
        MainTabView()
            .environmentObject(AppCoordinator())
            .environmentObject(AppState())
            .environmentObject(MockHomeViewModel() as HomeViewModel)
            // Story 37.9 AC6: WardrobeView 子树需要 wardrobeViewModel；Preview 也注入 Mock.
            .environmentObject(MockWardrobeViewModel() as WardrobeViewModel)
            // Story 37.10 AC5 Task 5.5: FriendsView 子树需要 friendsViewModel；Preview 也注入 Mock.
            .environmentObject(MockFriendsViewModel() as FriendsViewModel)
            // Story 37.11 AC5 Task 5.5: ProfileView 子树需要 profileViewModel；Preview 也注入 Mock.
            .environmentObject(MockProfileViewModel() as ProfileViewModel)
    }
}
#endif
