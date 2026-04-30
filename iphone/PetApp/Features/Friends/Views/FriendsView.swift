// FriendsView.swift
// Story 37.3 占位 stub（Story 37.10 落地真实内容）.

import SwiftUI

public struct FriendsView: View {
    public init() {}

    public var body: some View {
        NavigationStack {
            Text("Friends Tab Placeholder")
                .accessibilityIdentifier("friendsView")
        }
    }
}

#if DEBUG
struct FriendsView_Previews: PreviewProvider {
    static var previews: some View {
        FriendsView()
    }
}
#endif
