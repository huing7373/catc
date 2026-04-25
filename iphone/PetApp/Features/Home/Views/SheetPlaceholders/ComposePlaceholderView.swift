// ComposePlaceholderView.swift
// Story 2.3 placeholder; replaced by Epic 33 (ComposeView).
//
// 临时归属 Home 模块（语义上是"主界面跳转的占位"）。
// Epic 33 Story 33.1 / 33.6 实装 ComposeView 时：
//   1) 删除本文件
//   2) 修改 RootView.sheetContent(for:) 把 .compose 分支指向真实 ComposeView。

import SwiftUI

struct ComposePlaceholderView: View {
    let onClose: () -> Void

    var body: some View {
        // 容器 identifier 必须配 .accessibilityElement(children: .contain)，
        // 否则会覆盖所有子元素 identifier。详见 RoomPlaceholderView 注释。
        VStack(spacing: 16) {
            HStack {
                Spacer()
                Button("关闭", action: onClose)
                    .accessibilityIdentifier(AccessibilityID.SheetPlaceholder.btnClose)
            }
            .padding()

            Spacer()

            Text("Compose Placeholder")
                .font(.title)
                .accessibilityIdentifier(AccessibilityID.SheetPlaceholder.composeTitle)

            Spacer()
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(AccessibilityID.SheetPlaceholder.composeContainer)
    }
}

#if DEBUG
struct ComposePlaceholderView_Previews: PreviewProvider {
    static var previews: some View {
        ComposePlaceholderView(onClose: {})
    }
}
#endif
