// JoinRoomModalPlaceholder.swift
// Story 37.3 占位 stub（Story 37.12 落地真实 modal）.
//
// 当前：仅 Text + a11y identifier；不实装"创建队伍" / "加入队伍"按钮（Story 37.7 TeamIdleCard 实装）.
// Story 37.12：实装真实 modal 内容 + sheet 挂在 HomeView 通过 HomeViewModel.showJoinModal 驱动.

import SwiftUI

public struct JoinRoomModalPlaceholder: View {
    public init() {}

    public var body: some View {
        Text("Join Room Modal Placeholder")
            .accessibilityIdentifier("joinRoomModalPlaceholder")
    }
}

#if DEBUG
struct JoinRoomModalPlaceholder_Previews: PreviewProvider {
    static var previews: some View {
        JoinRoomModalPlaceholder()
    }
}
#endif
