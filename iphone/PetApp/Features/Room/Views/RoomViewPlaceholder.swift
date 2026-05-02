// RoomViewPlaceholder.swift
// Story 37.3 占位 stub（Story 37.8 落地真实内容）.
//
// 与 Story 2.3 的 SheetPlaceholders/RoomPlaceholderView **不同**：后者是 .fullScreenCover 内容
// （带关闭按钮 + sheetPlaceholder a11y），随 Story 37.3 整目录删除；本文件是 HomeContainerView
// 内嵌的 RoomView 占位（无关闭按钮，靠 leaveRoom 把 currentRoomId 置 nil 退出）.

import SwiftUI

public struct RoomViewPlaceholder: View {
    public init() {}

    public var body: some View {
        Text("Room Placeholder")
            .accessibilityIdentifier(AccessibilityID.Room.viewPlaceholder)
    }
}

#if DEBUG
struct RoomViewPlaceholder_Previews: PreviewProvider {
    static var previews: some View {
        RoomViewPlaceholder()
    }
}
#endif
