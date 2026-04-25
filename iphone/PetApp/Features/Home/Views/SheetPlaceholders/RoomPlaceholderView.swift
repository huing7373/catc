// RoomPlaceholderView.swift
// Story 2.3 placeholder; replaced by Epic 12 (RoomView).
//
// 临时归属 Home 模块（语义上是"主界面跳转的占位"）。
// Epic 12 Story 12.1 实装 RoomView 时：
//   1) 删除本文件
//   2) 修改 RootView.sheetContent(for:) 把 .room 分支指向真实 RoomView
//   3) 视情况保留 / 删除 SheetPlaceholder 命名空间下的 a11y 常量。

import SwiftUI

struct RoomPlaceholderView: View {
    let onClose: () -> Void

    var body: some View {
        // 关键约束：container 的 .accessibilityIdentifier 必须配合 .accessibilityElement(children: .contain)
        // 才能既给容器一个标识，又**不覆盖**子元素（按钮 / 标题）自己的 accessibilityIdentifier。
        // 不加 .contain 时 SwiftUI 会把 VStack 的 identifier 传播到所有子元素，导致测试中
        // app.staticTexts["sheetPlaceholder_roomTitle"] 找不到（实际所有子元素 id 都变成
        // "sheetPlaceholder_room"）。
        VStack(spacing: 16) {
            HStack {
                Spacer()
                Button("关闭", action: onClose)
                    .accessibilityIdentifier(AccessibilityID.SheetPlaceholder.btnClose)
            }
            .padding()

            Spacer()

            Text("Room Placeholder")
                .font(.title)
                .accessibilityIdentifier(AccessibilityID.SheetPlaceholder.roomTitle)

            Spacer()
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(AccessibilityID.SheetPlaceholder.roomContainer)
    }
}

#if DEBUG
struct RoomPlaceholderView_Previews: PreviewProvider {
    static var previews: some View {
        RoomPlaceholderView(onClose: {})
    }
}
#endif
