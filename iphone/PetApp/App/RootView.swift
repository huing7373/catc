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
//   - resetIdentityViewModel（仅 Debug）走 @State Optional + .onAppear 注入路径，
//     与 container 共享同一 keychainStore instance（codex round 1 [P1] 修复）。
//     详见 docs/lessons/2026-04-26-stateobject-debug-instance-aliasing.md。
//
// Story 2.9 落地 AppLaunchState 时改为根据状态路由到 LaunchingView / HomeView / RetryView。

import SwiftUI

struct RootView: View {
    @StateObject private var coordinator = AppCoordinator()
    @StateObject private var container = AppContainer()
    @StateObject private var homeViewModel = HomeViewModel()

    #if DEBUG
    /// Story 2.8: dev "重置身份" 按钮 ViewModel。仅 Debug build 存在；Release build 字段不存在。
    ///
    /// **注入路径**：用 `@State Optional` + `.onAppear` lazy 注入，**不**用 `@StateObject` + init 阶段构造。
    /// 因为 `@StateObject` 必须在 init 阶段给值，但 RootView 的 `container` 同样是 `@StateObject`、
    /// init 期间还**没**真正实体化，无法 `_resetIdentityViewModel = StateObject(...container.make...())`。
    /// 早期实装走 standalone `AppContainer()` bootstrap 喂初值——但那会构造一个**新的** `InMemoryKeychainStore`
    /// 实例，与 `container.keychainStore` 是两个不同的字典，重置按钮调的 `removeAll()` 清的是 bootstrap 那个，
    /// container 实际持有的 keychain 仍残留——UI 显示成功但功能失效（codex round 1 [P1] finding）。
    /// 详见 docs/lessons/2026-04-26-stateobject-debug-instance-aliasing.md。
    @State private var resetIdentityViewModel: ResetIdentityViewModel?
    #endif

    var body: some View {
        homeView
            .onAppear {
                wireHomeViewModelClosures()
                #if DEBUG
                // lazy 注入：第一次 .onAppear 时从已稳定的 container 拿 keychainStore，
                // 保证 reset 按钮调的 removeAll() 清的是 container.keychainStore 这同一份。
                // nil 守卫让 RootView 重建（如旋转 / 离开返回）时不会重新构造覆盖既有 instance。
                if resetIdentityViewModel == nil {
                    resetIdentityViewModel = container.makeResetIdentityViewModel()
                }
                #endif
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

    /// Debug build 走带 resetIdentityViewModel 的 init;Release build 走旧 init（按钮不存在）。
    /// resetIdentityViewModel 在 .onAppear 之前为 nil；HomeView 已支持 Optional（按钮在 nil 时不渲染），
    /// 短暂的 nil 期对 UX 无影响（dev 工具，非 release 关键路径）。
    @ViewBuilder
    private var homeView: some View {
        #if DEBUG
        HomeView(viewModel: homeViewModel, resetIdentityViewModel: resetIdentityViewModel)
        #else
        HomeView(viewModel: homeViewModel)
        #endif
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
