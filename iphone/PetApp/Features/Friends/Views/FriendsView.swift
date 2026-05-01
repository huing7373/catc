// FriendsView.swift
// Story 37.3 占位 stub → Story 37.10 落地真实内容.
//
// Story 37.3：仅 Text + a11y identifier，让 UITest 可断言 Tab 切换可见性.
// Story 37.10：body 占位 Text 替换为 FriendsScaffoldView(state: friendsViewModel).
//   - 保留 NavigationStack 包裹层（让后续 epic 实装 NavigationLink push 好友详情页时无须再改 FriendsView 类型签名）
//   - 加 @EnvironmentObject var friendsViewModel: FriendsViewModel（RootView .environmentObject 注入）
//   - 文件**不删**：保 git history 可读 + Story 37.13 a11y 总表归并时统一清理.

import SwiftUI

public struct FriendsView: View {
    @EnvironmentObject var friendsViewModel: FriendsViewModel

    public init() {}

    public var body: some View {
        NavigationStack {
            FriendsScaffoldView(state: friendsViewModel)
        }
    }
}

#if DEBUG
struct FriendsView_Previews: PreviewProvider {
    static var previews: some View {
        FriendsView()
            .environmentObject(MockFriendsViewModel() as FriendsViewModel)
            .environment(\.theme, ThemeName.candy.theme)
    }
}
#endif
