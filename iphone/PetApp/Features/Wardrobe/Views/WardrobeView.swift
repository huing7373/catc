// WardrobeView.swift
// Story 37.3 占位 stub（Story 37.9 落地真实内容）.
//
// 当前：仅 Text + a11y identifier，让 UITest 可断言 Tab 切换可见性.
// Story 37.9：实装真实 wardrobe 内容（NavigationStack 内嵌路由保留）.

import SwiftUI

public struct WardrobeView: View {
    public init() {}

    public var body: some View {
        NavigationStack {
            Text("Wardrobe Tab Placeholder")
                .accessibilityIdentifier("wardrobeView")
        }
    }
}

#if DEBUG
struct WardrobeView_Previews: PreviewProvider {
    static var previews: some View {
        WardrobeView()
    }
}
#endif
