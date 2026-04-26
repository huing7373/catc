// RootView.swift
// Story 2.2 起占位 RootView：渲染 HomeView。
// Story 2.3 起：注入 AppCoordinator，把 HomeView 三个 CTA 闭包连到 coordinator.present(...)，
// 并通过 .fullScreenCover(item:) 弹出对应 Sheet placeholder。
// Story 2.5 起（追加）：
//   - 新增 @StateObject private var container = AppContainer()
//   - 新增 .task：bind PingUseCase 后触发 start() 拉 /ping + /version
//
// 设计选择：
//   - 三个 @StateObject（coordinator + container + homeViewModel），都在 RootView 持有生命周期。
//   - homeViewModel 仍走老 init（hardcode 默认）：避免 SwiftUI @StateObject init 注入陷阱
//     （详见 Story 2.5 Dev Note #3）；运行时通过 .task 调 bind(pingUseCase:) 注入真实 UseCase。
//   - closure wire 放 .onAppear 而非 init：@StateObject 在 init 阶段未真正初始化，
//     init 写会捕获到错误的实例；.onAppear 时 view 已显示，coordinator 已稳定。
//   - capture list `[coordinator]` 显式声明（防强引用 self；闭包都是值类型，重复赋值仅覆盖）。
//
// Story 2.9 落地 AppLaunchState 时改为根据状态路由到 LaunchingView / HomeView / RetryView。

import SwiftUI

struct RootView: View {
    @StateObject private var coordinator = AppCoordinator()
    @StateObject private var container = AppContainer()
    @StateObject private var homeViewModel = HomeViewModel()

    var body: some View {
        HomeView(viewModel: homeViewModel)
            .onAppear {
                wireHomeViewModelClosures()
            }
            .task {
                // 把 container 创建的 PingUseCase 单次绑定到 homeViewModel，再触发 start()。
                // bind() 是单次生效（second call 会被 ViewModel guard 短路），start() 内部
                // 也通过 pingTask != nil 防重复请求。两条防御 cover SwiftUI .task 多次触发场景。
                homeViewModel.bind(pingUseCase: container.makePingUseCase())
                await homeViewModel.start()
            }
            .fullScreenCover(item: $coordinator.presentedSheet) { sheet in
                sheetContent(for: sheet)
            }
            .errorPresentationHost(presenter: container.errorPresenter)
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

    /// sheet 内容也挂一份 errorPresentationHost：
    /// SwiftUI 的 `.fullScreenCover` 在独立 window scene 渲染，外层 modifier 链不传播到 sheet 子树；
    /// 主 host（在 RootView body 末尾）会被 sheet 整片盖住，导致 sheet 打开时全局错误 UI 隐形。
    /// 让 sheet 子树共用同一个 ErrorPresenter 实例 → presenter.current 是 @Published source of truth →
    /// 两个 host 都监听同一份状态、同步渲染 → sheet 内的 host 渲染在 sheet 顶层，错误 UI 始终可见。
    /// 详见 docs/lessons/2026-04-26-fullscreencover-isolated-environment.md（codex round 1 [P1] finding 修复）。
    @ViewBuilder
    private func sheetContent(for sheet: SheetType) -> some View {
        Group {
            switch sheet {
            case .room:
                RoomPlaceholderView(onClose: { coordinator.dismiss() })
            case .inventory:
                InventoryPlaceholderView(onClose: { coordinator.dismiss() })
            case .compose:
                ComposePlaceholderView(onClose: { coordinator.dismiss() })
            }
        }
        .errorPresentationHost(presenter: container.errorPresenter)
    }
}
