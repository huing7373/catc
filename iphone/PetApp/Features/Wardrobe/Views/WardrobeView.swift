// WardrobeView.swift
// Story 37.3 占位 stub → Story 37.9 落地真实内容.
//
// Story 37.3：仅 Text + a11y identifier，让 UITest 可断言 Tab 切换可见性.
// Story 37.9：body 占位 Text 替换为 WardrobeScaffoldView(state: wardrobeViewModel).
//   - 保留 NavigationStack 包裹层（让 Story 33.1 实装 NavigationLink push 合成页时无须再改 WardrobeView 类型签名）
//   - 加 @EnvironmentObject var wardrobeViewModel: WardrobeViewModel（RootView .environmentObject 注入）
//   - 文件**不删**：保 git history 可读 + Story 37.13 a11y 总表归并时统一清理.

import SwiftUI

public struct WardrobeView: View {
    @EnvironmentObject var wardrobeViewModel: WardrobeViewModel

    public init() {}

    public var body: some View {
        NavigationStack {
            WardrobeScaffoldView(state: wardrobeViewModel)
        }
    }
}

#if DEBUG
struct WardrobeView_Previews: PreviewProvider {
    static var previews: some View {
        WardrobeView()
            .environmentObject(MockWardrobeViewModel() as WardrobeViewModel)
            .environment(\.theme, ThemeName.candy.theme)
    }
}
#endif
