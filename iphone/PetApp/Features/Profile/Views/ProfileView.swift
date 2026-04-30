// ProfileView.swift
// Story 37.3 占位 stub（Story 37.11 落地真实内容）.

import SwiftUI

public struct ProfileView: View {
    public init() {}

    public var body: some View {
        NavigationStack {
            Text("Profile Tab Placeholder")
                .accessibilityIdentifier("profileView")
        }
    }
}

#if DEBUG
struct ProfileView_Previews: PreviewProvider {
    static var previews: some View {
        ProfileView()
    }
}
#endif
