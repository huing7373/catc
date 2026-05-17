// WardrobeView.swift
// Story 37.3 占位 stub → Story 37.9 落地真实内容 → Story 24.2 接 LoadInventoryUseCase 触发.
//
// Story 37.3：仅 Text + a11y identifier，让 UITest 可断言 Tab 切换可见性.
// Story 37.9：body 占位 Text 替换为 WardrobeScaffoldView(state: wardrobeViewModel).
//   - 保留 NavigationStack 包裹层（让 Story 33.1 实装 NavigationLink push 合成页时无须再改 WardrobeView 类型签名）
//   - 加 @EnvironmentObject var wardrobeViewModel: WardrobeViewModel（RootView .environmentObject 注入）
//   - 文件**不删**：保 git history 可读 + Story 37.13 a11y 总表归并时统一清理.
// Story 24.2（本 story）：接「Wardrobe Tab 出现 → GET /cosmetics/inventory → 写 appState.currentInventory
//   → Story 24.1 既有 sink 零 edit 渲染真实装扮」闭环触发点.
//   - 触发机制：`.task(id: coordinator.currentTab)` —— currentTab 变化时 cancel 旧 + 起新 task；
//     guard `== .wardrobe` 才真正 fetch；切走时旧 in-flight 自动 cancel（loading 可取消，不阻塞
//     tab 切换）；**每次切到 Wardrobe Tab 重拉不缓存**（epics.md 行 3367）.
//     选定理由详见 story Dev Notes「Wardrobe Tab 首次/每次出现触发点决策」.
//   - loading：@State isLoading 包裹 execute() 前后；loading 时在 WardrobeScaffoldView 之上叠加
//     ProgressView（.overlay，**不**改 WardrobeScaffoldView 本体视觉 / a11y 锚 / 不改 24.1 sink）.
//   - 失败：errorPresenter.present(error, onRetry:) —— RootView 最外层 .errorPresentationHost
//     已挂，自动渲染全屏 RetryView；onRetry 重新调 execute()（epics.md 行 3366「复用 ErrorPresenter」）.
//   - LoadInventoryUseCase / ErrorPresenter 经 EnvironmentKey 注入（与 \.loadEmojisUseCase 同模式；
//     default nil → 触发 no-op，Preview / 未注入路径不 crash）.

import SwiftUI

public struct WardrobeView: View {
    @EnvironmentObject var wardrobeViewModel: WardrobeViewModel
    @EnvironmentObject var coordinator: AppCoordinator

    /// Story 24.2: 背包加载 UseCase（RootView 经 `\.loadInventoryUseCase` 注入；
    /// default nil → 触发短路 no-op，Preview 不 crash）.
    @Environment(\.loadInventoryUseCase) private var loadInventoryUseCase
    /// Story 24.2: 失败展示中心（RootView 经 `\.wardrobeErrorPresenter` 注入与
    /// container.errorPresenter 同实例；default nil → 失败仅吞，不 crash）.
    @Environment(\.wardrobeErrorPresenter) private var errorPresenter

    @State private var isLoading = false

    public init() {}

    public var body: some View {
        NavigationStack {
            WardrobeScaffoldView(state: wardrobeViewModel)
                .overlay {
                    if isLoading {
                        // loading 叠加层（不改 WardrobeScaffoldView 本体；首屏空 inventory 时
                        // 24.1 sink 已渲染空仓库 placeholder，ProgressView 叠其上不白屏）.
                        ProgressView()
                            .controlSize(.large)
                            .padding(24)
                            .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12))
                            .accessibilityIdentifier(WardrobeView.loadingIndicatorA11yID)
                    }
                }
        }
        // Story 24.2 触发点：currentTab 变化时 task 重启（cancel 旧 + 起新）.
        // guard == .wardrobe → 仅切到「仓库」Tab 才真正发请求；切走时旧 in-flight 被
        // .task(id:) 自动 cancel（loading 可取消，符合「不阻塞 tab 切换」）.
        // 启动默认 currentTab == .home；用户首次点「仓库」→ id 从 .home 变 .wardrobe →
        // task 重启 → 触发首次 load（无需额外 .onAppear）；每次再切回都重拉（不缓存）.
        .task(id: coordinator.currentTab) {
            guard coordinator.currentTab == .wardrobe else { return }
            await loadInventory()
        }
    }

    /// 调 LoadInventoryUseCase.execute()：成功后写 appState.currentInventory（经 UseCase 内部），
    /// Story 24.1 既有 sink 零 edit 反映真实装扮；失败经 ErrorPresenter 派生 RetryView.
    ///
    /// 取消静默吞 —— 不弹 RetryView（切走不是错误；与既有 UseCase 触发失败不破坏 UI 同精神）.
    /// **关键**：用户切走 Tab 时 `.task(id:)` cancel，URLSession 抛 `URLError(.cancelled)`，
    /// 但 `APIClient` 把它包成 `APIError.network(URLError.cancelled)`（APIClient.swift:139-140），
    /// **不**是裸 `CancellationError`。只 catch `CancellationError` 会让"切走"误判成网络失败弹
    /// RetryView（codex review 24-2 r1 [P2]）。故用 `Self.isSilentCancellation(_:)` 统一识别
    /// 「裸 CancellationError」「裸 URLError.cancelled」「被网络层包裹的 URLError.cancelled」三种形态，
    /// 只对真正的取消静默吞；真实网络失败（notConnectedToInternet / timeout 等）仍照常弹 RetryView.
    private func loadInventory() async {
        guard let useCase = loadInventoryUseCase else { return }
        isLoading = true
        defer { isLoading = false }
        do {
            try await useCase.execute()
        } catch {
            guard !Self.isSilentCancellation(error) else {
                // 用户切走 Tab → task cancel（裸 CancellationError 或被网络层包裹的
                // URLError.cancelled）；不是真实错误，静默吞.
                return
            }
            // 真实失败 → ErrorPresenter 派生全屏 RetryView（RootView .errorPresentationHost 已挂）；
            // onRetry 重新发起 load（不缓存语义：重试 = 再 execute 一次）.
            errorPresenter?.present(error, onRetry: {
                Task { await loadInventory() }
            })
        }
    }
}

extension WardrobeView {
    /// 判定 `error` 是否「用户切走 Tab 触发的取消」——应静默吞而非弹 RetryView.
    ///
    /// 覆盖三种形态（缺一会让某条取消路径误弹 RetryView 或让真实失败被静默掉）：
    /// 1. **裸 `CancellationError`** —— Swift Concurrency 结构化取消（如 UseCase 内部 `Task.checkCancellation()`）.
    /// 2. **裸 `URLError(.cancelled)`** —— 万一未经 APIClient 包裹直达（防御性；当前链路罕见但不应漏）.
    /// 3. **`APIError.network(URLError.cancelled)`** —— **本 review finding 的主路径**：
    ///    `URLSession.data(for:)` 被 `.task(id:)` cancel 抛 `URLError(.cancelled)`，
    ///    `APIClient` catch `as URLError` 包成 `APIError.network(underlying:)`（APIClient.swift:139-140）.
    ///
    /// **不**把所有 `APIError.network` 当取消（那会把 timeout / 离线 / DNS 失败也静默掉 →
    /// 用户切到 Wardrobe 却永远看不到 RetryView，下一轮 review 必反弹）。只下钻 underlying
    /// 是否恰为 `URLError.Code.cancelled`。
    static func isSilentCancellation(_ error: Error) -> Bool {
        if error is CancellationError { return true }
        if let urlError = error as? URLError, urlError.code == .cancelled { return true }
        if case let .network(underlying) = (error as? APIError),
           let urlError = underlying as? URLError,
           urlError.code == .cancelled {
            return true
        }
        return false
    }
}

extension WardrobeView {
    /// loading 指示器 a11y identifier（inline 字符串；Story 37.13 a11y 总表归并时改常量，
    /// 与 MainTabView `tab_<rawValue>` inline 占位同精神；不破坏 37.9/24.1 既有锚）.
    static let loadingIndicatorA11yID = "wardrobe_loading_indicator"
}

// MARK: - Story 24.2: EnvironmentKey 注入入口（与 \.loadEmojisUseCase / \.theme 同模式）

/// `\.loadInventoryUseCase`：RootView 经 `.environment(\.loadInventoryUseCase, ...)` 注入
/// .ready 子树（与 \.loadEmojisUseCase 同模式 —— WardrobeView 是 MainTabView 内嵌子视图，
/// bridge 直接拿 environment 即可，不层层透传 AppContainer）.
/// default nil → WardrobeView.loadInventory() 内 `guard let useCase` 短路 no-op
/// （Preview / 未注入路径不 crash；与 \.loadEmojisUseCase default nil 同精神）.
private struct LoadInventoryUseCaseKey: EnvironmentKey {
    static let defaultValue: LoadInventoryUseCaseProtocol? = nil
}

/// `\.wardrobeErrorPresenter`：RootView 经 `.environment(\.wardrobeErrorPresenter, container.errorPresenter)`
/// 注入与 RootView 既有 `.errorPresentationHost(presenter: container.errorPresenter)` 同实例 ——
/// WardrobeView 调 present(error, onRetry:) → 既有 host modifier 渲染全屏 RetryView.
/// default nil → 失败仅静默（不 crash；Preview / 未注入路径安全）.
private struct WardrobeErrorPresenterKey: EnvironmentKey {
    static let defaultValue: ErrorPresenter? = nil
}

extension EnvironmentValues {
    var loadInventoryUseCase: LoadInventoryUseCaseProtocol? {
        get { self[LoadInventoryUseCaseKey.self] }
        set { self[LoadInventoryUseCaseKey.self] = newValue }
    }

    var wardrobeErrorPresenter: ErrorPresenter? {
        get { self[WardrobeErrorPresenterKey.self] }
        set { self[WardrobeErrorPresenterKey.self] = newValue }
    }
}

#if DEBUG
struct WardrobeView_Previews: PreviewProvider {
    static var previews: some View {
        WardrobeView()
            .environmentObject(MockWardrobeViewModel() as WardrobeViewModel)
            .environmentObject(AppCoordinator())
            .environment(\.theme, ThemeName.candy.theme)
    }
}
#endif
