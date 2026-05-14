// EmojiPanelHostView.swift
// Story 18.1 AC8: UITest stub host view —— **仅 DEBUG 编译** + 仅在 `--uitest-emoji-panel-host`
// launch arg 路径下渲染.
//
// 设计:
//   - 内嵌 `EmojiPanelView(viewModel: container.makeEmojiPanelViewModel(), onSelect: ...)`
//   - 隐藏 `Text(lastSelectedCode).accessibilityIdentifier("emojiPanel_uitestSelectedCode")` 字段
//     用于 UITest 断言 onSelect 回调 emojiCode.
//
// Production / 正常启动**不**渲染本 view (RootView 通过 launch arg gate);
// 编译期 #if DEBUG 包裹避免污染 Release binary.

#if DEBUG

import SwiftUI

/// UITest stub host view (仅 DEBUG). 通过 `--uitest-emoji-panel-host` launch arg 触发.
struct EmojiPanelHostView: View {
    let container: AppContainer
    @State private var lastSelectedCode: String = ""

    var body: some View {
        VStack(spacing: 0) {
            EmojiPanelView(
                viewModel: container.makeEmojiPanelViewModel(),
                onSelect: { code in
                    lastSelectedCode = code
                }
            )
            // 隐藏 Text 用于 UITest 断言 onSelect 回调 emojiCode.
            // 字号 1pt + frame 1x1 让 sighted user 看不见，但 XCUIElement 仍能找到节点读 label.
            Text(lastSelectedCode)
                .font(.system(size: 1))
                .frame(width: 1, height: 1)
                .opacity(0.01)
                .accessibilityIdentifier(AccessibilityID.Emoji.uitestSelectedCode)
        }
    }
}

#endif
