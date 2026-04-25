// RootView.swift
// Story 2.2 起占位 RootView：渲染 HomeView。
// Story 2.3 起：注入 AppCoordinator，把 HomeView 三个 CTA 闭包连到 coordinator.present(...)，
// 并通过 .fullScreenCover(item:) 弹出对应 Sheet placeholder。
//
// 设计选择：
//   - 两个 @StateObject（coordinator + homeViewModel），都在 RootView 持有生命周期。
//   - closure wire 放 .onAppear 而非 init：@StateObject 在 init 阶段未真正初始化，
//     init 写会捕获到错误的实例；.onAppear 时 view 已显示，coordinator 已稳定。
//   - capture list `[coordinator]` 显式声明（防强引用 self；闭包都是值类型，重复赋值仅覆盖）。
//
// Story 2.9 落地 AppLaunchState 时改为根据状态路由到 LaunchingView / HomeView / RetryView。

import SwiftUI

struct RootView: View {
    @StateObject private var coordinator = AppCoordinator()
    @StateObject private var homeViewModel = HomeViewModel()

    var body: some View {
        HomeView(viewModel: homeViewModel)
            .onAppear {
                wireHomeViewModelClosures()
            }
            .fullScreenCover(item: $coordinator.presentedSheet) { sheet in
                sheetContent(for: sheet)
            }
    }

    /// 把 HomeViewModel 三个 CTA 闭包接到 coordinator.present(...)。
    /// .onAppear 时机重新 wire 一次（防止 RootView 重新构建后失去引用），
    /// 不重复注册不会导致 leak —— 闭包都是值类型，每次赋值覆盖前一个。
    private func wireHomeViewModelClosures() {
        homeViewModel.onRoomTap = { [coordinator] in
            coordinator.present(.room)
        }
        homeViewModel.onInventoryTap = { [coordinator] in
            coordinator.present(.inventory)
        }
        homeViewModel.onComposeTap = { [coordinator] in
            coordinator.present(.compose)
        }
    }

    @ViewBuilder
    private func sheetContent(for sheet: SheetType) -> some View {
        switch sheet {
        case .room:
            RoomPlaceholderView(onClose: { coordinator.dismiss() })
        case .inventory:
            InventoryPlaceholderView(onClose: { coordinator.dismiss() })
        case .compose:
            ComposePlaceholderView(onClose: { coordinator.dismiss() })
        }
    }
}
