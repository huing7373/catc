// InventoryPlaceholderView.swift
// Story 2.3 placeholder; replaced by Epic 24 (InventoryView).
//
// 临时归属 Home 模块（语义上是"主界面跳转的占位"）。
// Epic 24 Story 24.1 / 24.4 实装 InventoryView 时：
//   1) 删除本文件
//   2) 修改 RootView.sheetContent(for:) 把 .inventory 分支指向真实 InventoryView。

import SwiftUI

struct InventoryPlaceholderView: View {
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

            Text("Inventory Placeholder")
                .font(.title)
                .accessibilityIdentifier(AccessibilityID.SheetPlaceholder.inventoryTitle)

            Spacer()
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(AccessibilityID.SheetPlaceholder.inventoryContainer)
    }
}

#if DEBUG
struct InventoryPlaceholderView_Previews: PreviewProvider {
    static var previews: some View {
        InventoryPlaceholderView(onClose: {})
    }
}
#endif
