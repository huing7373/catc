// EmojiPanelView.swift
// Story 18.1 AC4: 表情面板 SwiftUI 组件 (LazyVGrid 4 列 + AsyncImage + onSelect 闭包).
//
// 构造参数:
//   - `viewModel: EmojiPanelViewModel` —— 通过 init 一次性注入 + @StateObject 内部持有
//     (Dev Note #5 钦定: @StateObject 而非 @ObservedObject —— 让 view 自己持有 vm 生命周期,
//     不会因父 view 刷新而丢 state).
//   - `onSelect: (String) -> Void` —— emojiCode 回调闭包 (caller 实装外部逻辑:
//     18.2 关闭 panel + 18.3 触发 SendEmojiUseCase).
//
// 状态机分支 (按 viewModel.state):
//   - `.loading` → ProgressView 居中
//   - `.loaded([EmojiConfig])` → LazyVGrid 4 列 + AsyncImage cell + onSelect
//   - `.failed(message)` → RetryView (复用 Core/DesignSystem/Components/RetryView.swift)
//
// AsyncImage 渲染 (Dev Note #2):
//   - phase-based: `.empty` → ProgressView / `.success(img)` → resizable / `.failure` → 问号 SF symbol
//   - 不带 retry 按钮；URL 加载失败自动 fallback 问号 (V1 §11.1 client 缓存契约钦定 App 生命周期内
//     不重试图片).
//
// `.task { await viewModel.load() }` —— view 启动时触发首次加载 (RoomView 同精神).
//
// import 显式：SwiftUI + Foundation —— ObservableObject 间接订阅 (@StateObject)，**不**需要 import
// Combine (Combine 仅在 ViewModel 文件需要)；lesson 2026-04-25-swift-explicit-import-combine.md.

import SwiftUI
import Foundation

public struct EmojiPanelView: View {
    @StateObject private var viewModel: EmojiPanelViewModel
    private let onSelect: (String) -> Void

    /// `_viewModel = StateObject(wrappedValue:)` 模式 (SwiftUI 标准 @StateObject DI 模式,
    /// 与 RootView 同源；详见 lesson 2026-04-26-stateobject-debug-instance-aliasing.md).
    public init(viewModel: EmojiPanelViewModel, onSelect: @escaping (String) -> Void) {
        _viewModel = StateObject(wrappedValue: viewModel)
        self.onSelect = onSelect
    }

    public var body: some View {
        Group {
            switch viewModel.state {
            case .loading:
                ProgressView()
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .accessibilityIdentifier(AccessibilityID.Emoji.panelLoading)
            case .loaded(let emojis):
                LazyVGrid(
                    columns: Array(
                        repeating: GridItem(.flexible(), spacing: 12),
                        count: 4
                    ),
                    spacing: 12
                ) {
                    ForEach(emojis) { emoji in
                        cellView(for: emoji)
                    }
                }
                .padding(16)
                .accessibilityIdentifier(AccessibilityID.Emoji.panel)
            case .failed(let message):
                RetryView(
                    message: message,
                    onRetry: { Task { await viewModel.retry() } }
                )
                .accessibilityIdentifier(AccessibilityID.Emoji.panelError)
            }
        }
        .task {
            await viewModel.load()
        }
    }

    @ViewBuilder
    private func cellView(for emoji: EmojiConfig) -> some View {
        VStack(spacing: 4) {
            AsyncImage(url: URL(string: emoji.assetUrl)) { phase in
                switch phase {
                case .empty:
                    ProgressView()
                case .success(let image):
                    image.resizable().aspectRatio(contentMode: .fit)
                case .failure:
                    Image(systemName: "questionmark.circle")
                        .foregroundStyle(.secondary)
                @unknown default:
                    Image(systemName: "questionmark.circle")
                        .foregroundStyle(.secondary)
                }
            }
            .frame(width: 48, height: 48)

            Text(emoji.name)
                .font(.caption)
        }
        .frame(maxWidth: .infinity)
        .padding(8)
        .contentShape(Rectangle())
        .onTapGesture {
            onSelect(emoji.code)
        }
        // `accessibilityElement(children: .combine)` 把 cell 折叠成单一 a11y 节点 ——
        // 避免 SwiftUI 把 identifier 同时挂到 Image / Text 两个子节点造成 UITest 多匹配歧义.
        // .combine 保留子元素 label 文本组合, .ignore 完全忽略子树.
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(.isButton)
        .accessibilityLabel(emoji.name)
        .accessibilityIdentifier(AccessibilityID.Emoji.cell(emoji.code))
    }
}

#if DEBUG
struct EmojiPanelView_Previews: PreviewProvider {
    private static let fixture: [EmojiConfig] = [
        EmojiConfig(code: "wave", name: "挥手", assetUrl: "https://placehold.co/64x64?text=Wave", sortOrder: 1),
        EmojiConfig(code: "love", name: "爱心", assetUrl: "https://placehold.co/64x64?text=Love", sortOrder: 2),
        EmojiConfig(code: "laugh", name: "大笑", assetUrl: "https://placehold.co/64x64?text=Laugh", sortOrder: 3),
        EmojiConfig(code: "cry", name: "哭泣", assetUrl: "https://placehold.co/64x64?text=Cry", sortOrder: 4)
    ]

    static var previews: some View {
        Group {
            // loading 预览：用永不返回的 mock useCase
            EmojiPanelView(
                viewModel: EmojiPanelViewModel(useCase: NeverReturnsUseCase()),
                onSelect: { _ in }
            )
            .previewDisplayName("loading")

            // loaded 预览：手动注入 loaded 状态（用同步 useCase 让 .task 立即推到 loaded）
            EmojiPanelView(
                viewModel: EmojiPanelViewModel(useCase: ImmediateLoadedUseCase(emojis: fixture)),
                onSelect: { code in print("preview onSelect: \(code)") }
            )
            .previewDisplayName("loaded")

            // failed 预览：注入永远抛错的 useCase
            EmojiPanelView(
                viewModel: EmojiPanelViewModel(useCase: ImmediateFailedUseCase()),
                onSelect: { _ in }
            )
            .previewDisplayName("failed")
        }
    }
}

/// preview-only mock：execute 永不返回 (await 阻塞)，让 viewModel.state 保持 .loading.
private struct NeverReturnsUseCase: LoadEmojisUseCaseProtocol {
    func execute() async throws -> [EmojiConfig] {
        try? await Task.sleep(nanoseconds: UInt64.max / 2)
        return []
    }
}

private struct ImmediateLoadedUseCase: LoadEmojisUseCaseProtocol {
    let emojis: [EmojiConfig]
    func execute() async throws -> [EmojiConfig] { emojis }
}

private struct ImmediateFailedUseCase: LoadEmojisUseCaseProtocol {
    func execute() async throws -> [EmojiConfig] {
        throw APIError.network(underlying: URLError(.notConnectedToInternet))
    }
}
#endif
