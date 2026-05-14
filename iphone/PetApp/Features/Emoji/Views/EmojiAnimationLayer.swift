// EmojiAnimationLayer.swift
// Story 18.4 AC6: 房间内表情动效 overlay —— 替换 18.3 的 EmojiAnimationLayerPlaceholder 完整动画实装.
//
// 设计原则:
//   - 输入: activeEmojis (来自 RoomViewModel @Published) + memberAnchors (SwiftUI PreferenceKey 收集) + centerAnchor (GeometryReader 算)
//   - ZStack overlay; .allowsHitTesting(false) 防遮挡底层 (与 18.3 placeholder 同精神)
//   - activeEmojis.isEmpty 时返 EmptyView (脱离 layout, 避免 XCUITest hittability computation 误判; 18.3 lesson)
//   - 每个 emoji 用 FloatingEmojiCellView 独立子视图; @State 驱动 .onAppear withAnimation (HomeView FloatingEmojiView pattern)
//
// V1 §12.3 行 2473 (c) center anchor 降级: roster miss userId → memberAnchors[userId] = nil → 用 centerAnchor.
// V1 §12.3 行 2474 (d) catalog miss: FloatingEmojiCellView 内 assetUrl=nil → 问号 SF Symbol fallback.
//
// import 仅 SwiftUI: AsyncImage / GeometryReader / .position / .offset 全在 stdlib.

import SwiftUI

public struct EmojiAnimationLayer: View {
    let activeEmojis: [RoomActiveEmoji]
    let memberAnchors: [String: CGPoint]
    let centerAnchor: CGPoint
    let loadEmojisUseCase: LoadEmojisUseCaseProtocol?

    public init(
        activeEmojis: [RoomActiveEmoji],
        memberAnchors: [String: CGPoint],
        centerAnchor: CGPoint,
        loadEmojisUseCase: LoadEmojisUseCaseProtocol?
    ) {
        self.activeEmojis = activeEmojis
        self.memberAnchors = memberAnchors
        self.centerAnchor = centerAnchor
        self.loadEmojisUseCase = loadEmojisUseCase
    }

    @ViewBuilder
    public var body: some View {
        if activeEmojis.isEmpty {
            EmptyView()
        } else {
            ZStack {
                ForEach(activeEmojis) { emoji in
                    FloatingEmojiCellView(
                        emoji: emoji,
                        anchor: EmojiAnimationLayer.anchor(
                            for: emoji.userId,
                            memberAnchors: memberAnchors,
                            centerAnchor: centerAnchor
                        ),
                        loadEmojisUseCase: loadEmojisUseCase
                    )
                }
            }
        }
    }

    /// AC6 helper: anchor 选择逻辑 (单测友好; V1 §12.3 行 2473 (c) center 降级).
    /// memberAnchors hit → 该成员 PetSpriteView 中心点;
    /// memberAnchors miss → centerAnchor (屏幕中央 fallback; roster miss / 渲染竞态).
    public static func anchor(
        for userId: String,
        memberAnchors: [String: CGPoint],
        centerAnchor: CGPoint
    ) -> CGPoint {
        memberAnchors[userId] ?? centerAnchor
    }
}

/// Story 18.4 AC6: per-emoji 子视图 —— @State 驱动 .onAppear withAnimation 1.5s easeOut 飞出动画.
///
/// 与 HomeView FloatingEmojiView (line 540-574) 同 lesson:
///   - 每个 emoji 入队 → SwiftUI ForEach 重建本 View → @State 全部重新初始化 → .onAppear 触发动画 fresh start
///   - 多个 emoji 同帧入队 → 各自独立 @State + 独立 .onAppear → 互不干扰 (epics.md 行 2717 钦定)
///
/// 动画参数 (epics.md 行 2715-2716 钦定):
///   - duration: 1.5s, easeOut
///   - 位移: y 0 → -100 (向上飘 100px)
///   - 透明: opacity 1.0 → 0.0
///   - 缩放: scale 1.0 → 1.5
///
/// catalog miss fallback (V1 §12.3 行 2474 (d)):
///   - .task 异步查 LoadEmojisUseCase cache 拿 assetUrl
///   - assetUrl=nil → 问号 SF Symbol + 文字 emojiCode (便于 debug)
///   - AsyncImage failure → 同 fallback (网络拉图失败时不报错)
struct FloatingEmojiCellView: View {
    let emoji: RoomActiveEmoji
    let anchor: CGPoint  // 起点 = 该成员 PetSpriteView 中心 (or center fallback)
    let loadEmojisUseCase: LoadEmojisUseCaseProtocol?

    @State private var animatedY: CGFloat = 0       // 起点 0, 向上 -100 (epics.md 行 2715)
    @State private var animatedOpacity: Double = 1.0 // 1 → 0
    @State private var animatedScale: CGFloat = 1.0  // 1 → 1.5
    @State private var assetUrl: String? = nil       // catalog 查 emojiCode 拿 assetUrl; miss 时 nil → fallback "?"

    var body: some View {
        VStack(spacing: 2) {
            Group {
                if let urlStr = assetUrl, let url = URL(string: urlStr) {
                    // V1 §11.1 行 1750 钦定 assetUrl 非空; AsyncImage 加载远程图; 占位用 placeholder("?"); failure 走 fallback.
                    AsyncImage(url: url) { phase in
                        switch phase {
                        case .empty:
                            Image(systemName: "questionmark.circle")
                                .font(.system(size: 32))
                                .foregroundColor(.secondary)
                        case .success(let img):
                            img.resizable().aspectRatio(contentMode: .fit).frame(width: 48, height: 48)
                        case .failure:
                            Image(systemName: "questionmark.circle")
                                .font(.system(size: 32))
                                .foregroundColor(.secondary)
                        @unknown default:
                            EmptyView()
                        }
                    }
                } else {
                    // catalog miss (V1 §12.3 行 2474 (d) fallback): 问号 SF Symbol.
                    Image(systemName: "questionmark.circle")
                        .font(.system(size: 32))
                        .foregroundColor(.secondary)
                }
            }
            // 永远渲染 emojiCode Text label —— 让 UITest staticTexts.matching(BEGINSWITH 'activeEmoji_').label
            // 仍能命中 (与 18.3 占位 RoomEmojiSendUITests 兼容; 也利于 catalog miss 时 debug 看 wire 输入).
            // 18.3 RoomEmojiSendUITests `XCTAssertEqual(activeEmoji.label, "wave")` 必须保留.
            Text(emoji.emojiCode)
                .font(.system(size: 10, weight: .semibold))
                .foregroundColor(.secondary)
                .accessibilityIdentifier("activeEmoji_\(emoji.id.uuidString)")
        }
        .position(x: anchor.x, y: anchor.y)  // SwiftUI .position: view 中心点对齐 anchor; 让 emoji 飞出起点 = 成员 PetSpriteView 中心
        .offset(y: animatedY)                 // y 上飘
        .opacity(animatedOpacity)
        .scaleEffect(animatedScale)
        .task {
            // 启动时异步查 catalog 拿 assetUrl (loadEmojisUseCase = nil 时跳过, 走 catalog-miss fallback).
            if let loader = loadEmojisUseCase {
                if let catalog = try? await loader.execute(),
                   let entry = catalog.first(where: { $0.code == emoji.emojiCode }) {
                    self.assetUrl = entry.assetUrl
                }
            }
        }
        .onAppear {
            // 1.5s easeOut: y 0 → -100, opacity 1 → 0, scale 1 → 1.5 (epics.md 行 2715-2716 钦定动画规格).
            // 与 HomeView FloatingEmojiView 同 lesson: .onAppear + @State + withAnimation, 不靠常量 offset.
            withAnimation(.easeOut(duration: 1.5)) {
                animatedY = -100
                animatedOpacity = 0
                animatedScale = 1.5
            }
        }
    }
}
