// ResetIdentityButton.swift
// Story 2.8: dev "重置身份" 按钮（SF Symbol arrow.counterclockwise.circle）。
// **仅在 Debug build 渲染**；Release build 视图树物理不存在（#if DEBUG 包裹整个 type 定义）。
//
// 整个 type 定义 + Identifiable extension 都包 #if DEBUG：
// - Release build：编译器看不到此 type，调用方（HomeView）也必须 #if DEBUG 包裹引用，
//   否则 Release build "cannot find 'ResetIdentityButton'" 编译失败 —— 这是**有意**的 fail-closed
// - 与 .opacity(0) / .hidden() / .disabled(true) 不同：编译期剔除，视图树物理不存在

#if DEBUG

import SwiftUI

public struct ResetIdentityButton: View {
    @ObservedObject public var viewModel: ResetIdentityViewModel

    public init(viewModel: ResetIdentityViewModel) {
        self.viewModel = viewModel
    }

    public var body: some View {
        Button {
            Task { await viewModel.tap() }
        } label: {
            Image(systemName: "arrow.counterclockwise.circle")
                .font(.title3)
        }
        .accessibilityLabel(Text("重置身份"))
        .accessibilityIdentifier(AccessibilityID.Home.btnResetIdentity)
        .alert(item: $viewModel.alertContent) { content in
            switch content {
            case .success:
                return Alert(
                    title: Text("已重置"),
                    message: Text("请杀进程后重新启动 App 模拟首次安装"),
                    dismissButton: .default(Text("OK")) { viewModel.alertDismissed() }
                )
            case .failure(let message):
                return Alert(
                    title: Text("操作失败"),
                    message: Text(message),
                    dismissButton: .default(Text("OK")) { viewModel.alertDismissed() }
                )
            }
        }
    }
}

extension ResetIdentityAlertContent: Identifiable {
    public var id: String {
        switch self {
        case .success: return "alert_reset_success"
        case .failure: return "alert_reset_failure"
        }
    }
}

#endif
