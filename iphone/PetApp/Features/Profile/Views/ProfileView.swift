// ProfileView.swift
// Story 37.3 占位 stub → Story 37.11 落地真实内容.
//
// Story 37.3：仅 Text + a11y identifier，让 UITest 可断言 Tab 切换可见性.
// Story 37.11：body 占位 Text 替换为 ProfileScaffoldView(state: profileViewModel).
//   - 保留 NavigationStack 包裹层（让后续 epic 实装 NavigationLink push 子页面（成就 / 消息 /
//     喜欢的道具 / 设置）时无须再改 ProfileView 类型签名）
//   - 加 @EnvironmentObject var profileViewModel: ProfileViewModel（RootView .environmentObject 注入）
//   - 文件**不删**：保 git history 可读 + Story 37.13 a11y 总表归并时统一清理.

import SwiftUI

public struct ProfileView: View {
    @EnvironmentObject var profileViewModel: ProfileViewModel

    public init() {}

    public var body: some View {
        NavigationStack {
            ProfileScaffoldView(state: profileViewModel)
        }
    }
}

#if DEBUG
struct ProfileView_Previews: PreviewProvider {
    static var previews: some View {
        ProfileView()
            .environmentObject(MockProfileViewModel() as ProfileViewModel)
            .environment(\.theme, ThemeName.candy.theme)
    }
}
#endif
