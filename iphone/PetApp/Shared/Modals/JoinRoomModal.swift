// JoinRoomModal.swift
// Story 37.12 AC1: 加入队伍 Modal 视图（纯 presentation；不持 ViewModel；与业务解耦）.
//
// 设计：
//   - 参数：@Binding roomIdInput（输入字段双向绑定；owner = caller HomeView @State）
//          + onConfirm: (String) -> Void（trim 后非空时启用；点确定时调 closure 传 trim 后字符串）
//          + onCancel: () -> Void（点关闭按钮 / "取消"按钮 时调；caller 决定是否 dismiss sheet）
//   - **不**持 reactive store ViewModel —— modal 与业务解耦, 让 caller 决定提交后行为
//     （appState.setCurrentRoomId / JoinRoomUseCase 等）
//   - **不**做客户端格式校验（仅 trim 后判定空 + 限 64 字符长度；server 决定合法性,
//     AR21 + epic AC line 4856 钦定）
//
// **不**调用任何 UseCase / Repository / APIClient / AppState（Modal 视图层完全无依赖）.
// **不**做大写自动转换（与 ui_design `app.jsx:248` `e.target.value.toUpperCase()` 不同;
// AR21 roomId 是字符串内容不预设）.
//
// Lesson 7 (`2026-04-30-swiftui-onchange-equatable-and-stale-task-cancel.md`) 预防性应用：
//   .onChange(of: roomIdInput) 走 iOS 17+ 双参签名 `{ _, newValue in ... }`;
//   不用 `.onReceive` 监听（与 SwiftUI 主流写法不一致）.
//
// review r2 [P2] fix：trim / 64-char / disabled 规则下沉到 `JoinRoomInputNormalizer` 纯函数
// helper，view body + tests 共用同一函数 —— 测试断言 helper = 断言 view 行为（因为 view 直接
// 调用 helper），覆盖 .onChange / .disabled / confirm action 三处共用规则的回归.
// 与 `HomeRoomDispatcher` / `HomePetNameResolver` 同精神.
//
// 显式 import Foundation + SwiftUI（防 transitive import；与 MockHomeViewModel round 4 [P0]
// hardening 同精神）.

import Foundation
import SwiftUI

public struct JoinRoomModal: View {
    /// 房间号输入字段（双向绑定）；owner = caller HomeView @State.
    @Binding public var roomIdInput: String

    /// 点确定加入按钮的 closure；接受 trim 后字符串.
    /// caller 决定调 appState.setCurrentRoomId / JoinRoomUseCase / 等.
    public let onConfirm: (String) -> Void

    /// 点关闭 / 取消按钮的 closure；caller 决定 dismiss sheet（一般 state.showJoinModal = false）.
    public let onCancel: () -> Void

    /// Story 37.5: 主题 token 取值入口；caller 注入 `.environment(\.theme, currentTheme.theme)`.
    @Environment(\.theme) private var theme

    public init(
        roomIdInput: Binding<String>,
        onConfirm: @escaping (String) -> Void,
        onCancel: @escaping () -> Void
    ) {
        self._roomIdInput = roomIdInput
        self.onConfirm = onConfirm
        self.onCancel = onCancel
    }

    public var body: some View {
        VStack(spacing: 0) {
            // 标题栏 + 关闭按钮
            titleBar
            // 大输入框（Icons.paw + 等宽字体 + trim + 限 64 字符）
            inputArea
            // 格式提示（灰字，不暗示纯数字）
            hintLabel
            // 取消 / 确定加入 两按钮
            actionButtons
        }
        .padding(22)
        .background(theme.colors.surface)
        .clipShape(RoundedRectangle(cornerRadius: 28))
        .accessibilityIdentifier(AccessibilityID.JoinRoomModal.modal)
    }

    // MARK: - 5 视觉锚（详见 AC1 视觉契约表 + Dev Notes "5 视觉锚契约"）

    private var titleBar: some View {
        HStack {
            Text("加入队伍")
                .font(.system(size: 18, weight: .heavy))
                .foregroundColor(theme.colors.ink)
            Spacer()
            Button(action: onCancel) {
                Image(systemName: Icons.symbol(for: "close"))
                    .font(.system(size: 18, weight: .semibold))
                    .foregroundColor(theme.colors.inkSoft)
                    .frame(width: 32, height: 32)
                    .background(
                        Circle()
                            .fill(theme.colors.surface2)
                    )
            }
            .accessibilityIdentifier(AccessibilityID.JoinRoomModal.closeButton)
        }
        .padding(.bottom, 14)
    }

    private var inputArea: some View {
        HStack(spacing: 10) {
            Image(systemName: Icons.symbol(for: "paw"))
                .font(.system(size: 20, weight: .semibold))
                .foregroundColor(theme.colors.accent)
            TextField("", text: $roomIdInput)
                .font(.system(size: 20, weight: .heavy, design: .monospaced))
                .foregroundColor(theme.colors.ink)
                .autocorrectionDisabled(true)
                .textInputAutocapitalization(.never)
                .onChange(of: roomIdInput) { _, newValue in
                    // trim + 限 64 字符 —— 走 JoinRoomInputNormalizer 共享 helper（review r2 [P2] fix）.
                    // **仅**防 UI 渲染异常 + 让 confirm enable / disabled 判定与 .onChange 同源；
                    // 不做格式校验，server 决定合法性. iOS 17+ 双参签名：lesson 7 钦定路径.
                    let normalized = JoinRoomInputNormalizer.normalize(newValue)
                    if normalized != newValue {
                        roomIdInput = normalized
                    }
                }
                .accessibilityIdentifier(AccessibilityID.JoinRoomModal.input)
        }
        .padding(.vertical, 14)
        .padding(.horizontal, 16)
        .background(
            RoundedRectangle(cornerRadius: 18)
                .fill(theme.colors.surface2)
        )
        .overlay(
            RoundedRectangle(cornerRadius: 18)
                .stroke(theme.colors.accentSoft, lineWidth: 2)
        )
    }

    private var hintLabel: some View {
        HStack {
            Text("输入好友分享给你的房间号")
                .font(.system(size: 11, weight: .regular))
                .foregroundColor(theme.colors.inkMute)
            Spacer()
        }
        .padding(.top, 4)
        .padding(.horizontal, 4)
        .padding(.bottom, 18)
    }

    private var actionButtons: some View {
        HStack(spacing: 10) {
            PrimaryButton(
                title: "取消",
                variant: .secondary,
                fullWidth: true,
                action: onCancel
            )
            .accessibilityIdentifier(AccessibilityID.JoinRoomModal.cancelButton)

            PrimaryButton(
                title: "确定加入",
                variant: .primary,
                fullWidth: true,
                isDisabled: JoinRoomInputNormalizer.isSubmitDisabled(roomIdInput),
                action: { onConfirm(JoinRoomInputNormalizer.normalize(roomIdInput)) }
            )
            .accessibilityIdentifier(AccessibilityID.JoinRoomModal.confirmButton)
        }
    }
}

#if DEBUG
#Preview("JoinRoomModal — empty / candy") {
    StatefulPreviewWrapper("") { binding in
        JoinRoomModal(
            roomIdInput: binding,
            onConfirm: { _ in },
            onCancel: {}
        )
    }
    .environment(\.theme, ThemeName.candy.theme)
    .padding()
    .background(Color.black.opacity(0.45))
}

#Preview("JoinRoomModal — empty / dark") {
    StatefulPreviewWrapper("") { binding in
        JoinRoomModal(
            roomIdInput: binding,
            onConfirm: { _ in },
            onCancel: {}
        )
    }
    .environment(\.theme, ThemeName.dark.theme)
    .padding()
    .background(Color.black.opacity(0.45))
}

#Preview("JoinRoomModal — with input / candy") {
    StatefulPreviewWrapper("1234567") { binding in
        JoinRoomModal(
            roomIdInput: binding,
            onConfirm: { _ in },
            onCancel: {}
        )
    }
    .environment(\.theme, ThemeName.candy.theme)
    .padding()
    .background(Color.black.opacity(0.45))
}

#Preview("JoinRoomModal — long input / candy") {
    StatefulPreviewWrapper("9X2-L8-VERY-LONG-ROOM-CODE") { binding in
        JoinRoomModal(
            roomIdInput: binding,
            onConfirm: { _ in },
            onCancel: {}
        )
    }
    .environment(\.theme, ThemeName.candy.theme)
    .padding()
    .background(Color.black.opacity(0.45))
}

/// Preview helper：包装 @Binding 让 #Preview 能用闭包构造可变 state.
/// 与 SwiftUI #Preview 标准模式一致（@State 不能直接放 #Preview 块作用域，需要包装 wrapper）.
private struct StatefulPreviewWrapper<Value, Content: View>: View {
    @State private var value: Value
    private let content: (Binding<Value>) -> Content

    init(_ initial: Value, @ViewBuilder content: @escaping (Binding<Value>) -> Content) {
        self._value = State(wrappedValue: initial)
        self.content = content
    }

    var body: some View {
        content($value)
    }
}
#endif
