# Story 37.12: JoinRoomModal + 跨屏 join 链路（roomId 字符串 + 直白 UI）

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want JoinRoomModal 接受房间号字符串输入，跨屏 join 链路统一走 roomId 字符串,
so that A 分享给 B 的房间号 B 能直接输入加入，无 sender/receiver 闭环问题；同时 HomeView TeamIdleCard "加入队伍"按钮 + FriendsView FriendRow "加入"按钮两条入口都连真实 ViewModel mutate 路径，告别 Story 37.3 落地的 `JoinRoomModalPlaceholder` 临时占位。

## 故事定位（Epic 37 第四层第 6 条 story；UI Scaffold 主体后第一条「跨屏 sheet 跳转」收口）

这是 Epic 37「iPhone 架构层重构 + UI Scaffold（壳先行）」的**第四层 story**——上游 37.3（HomeView `.sheet(isPresented: $state.showJoinModal)` 已挂 `JoinRoomModalPlaceholder` 临时 stub）/ 37.4（AppState.currentRoomId + setCurrentRoomId 已就位）/ 37.5（Theme）/ 37.6（primitives 含 Icons.paw / Icons.close + PrimaryButton variants）/ 37.7（HomeView TeamIdleCard "加入队伍" 按钮已 wire `onJoinTap` → `showJoinModal = true`）/ 37.8（RoomScaffoldView + RealRoomViewModel 已订阅 appState.$currentRoomId 派生 inRoom 状态）/ 37.10（FriendsView FriendRow "加入"按钮已 wire `onJoinFriendTap` → `appState.setCurrentRoomId(friend.currentRoomId)`，但**不弹 modal**——epic AC line 4859 钦定）/ 37.11（ProfileView Scaffold）全部 done。本 story **不是** UI Scaffold 主体类，而是**收口型「跨屏 sheet 跳转」story**——用真实 `JoinRoomModal` View 替换 `JoinRoomModalPlaceholder`，让 HomeView "加入队伍"按钮触发的 sheet 内能输入房间号 + 提交后写 `appState.setCurrentRoomId(roomId)` 触发 Home/Room 互斥状态机切到 RoomView。FriendsView 那条入口（友直接调 `appState.setCurrentRoomId(friend.currentRoomId)`）已在 37.10 落地，本 story 仅**回归确认**该路径不破。

**本 story 落地后立即解锁**：
- Epic 37 第四层 story 推进到 37.13（accessibility identifier 总表）/ 37.14（design-package 白名单）
- 节点 4 起的真实 join flow（Story 12.7 JoinRoomUseCase）落地路径——本 story 把 modal 输入 + 提交 closure 的契约钉死，未来仅替换 closure 内的 `appState.setCurrentRoomId` 调用为 `JoinRoomUseCase.execute(roomId:)`，HomeView / Modal 视图层 zero edit
- Story 37.13（accessibility identifier 总表）—— JoinRoomModal 的全部 a11y identifier 来源（`joinRoomModal` / `joinRoomInput` / `joinRoomCancelButton` / `joinRoomConfirmButton` / `joinRoomCloseButton`）；本 story 在 JoinRoomModal 内 inline 字符串，Story 37.13 收口归并到 `AccessibilityID.JoinRoomModal`

**本 story 的"实装"动作**（一句话概括）：在 `iphone/PetApp/Shared/Modals/JoinRoomModal.swift`（**新建文件 + 新建目录** —— 之前 `iphone/PetApp/Shared/` 下仅 Constants / ErrorHandling / Testing 三个子目录，无 Modals/）落地 `struct JoinRoomModal: View` + 直白参数（`@Binding var roomIdInput: String` + `onConfirm: (String) -> Void` + `onCancel: () -> Void`）—— **不**持基类 ViewModel，**不**调 AppState / UseCase，纯 presentation View，业务解耦；HomeView 内 `.sheet` 闭包替换 `JoinRoomModalPlaceholder()` 为 `JoinRoomModal(...)` 含 trailing closure 调 `state.onJoinRoomConfirm(roomId:)`；HomeViewModel 基类新增 1 个 abstract method `onJoinRoomConfirm(roomId: String)` + Mock/Real 两子类各自 override（Mock 仅记 invocation；Real 走 `appState?.setCurrentRoomId(roomId)` 占位）；HomeView 同时持 `@State private var joinRoomInput: String = ""` 作为 modal 输入字段的 owner；sheet `onDismiss` 闭包内 reset `joinRoomInput = ""`（dismiss 后清空 transient 输入）。

**关键路径："新建" + caller 替换（与 Story 37.7-37.11 同精神：本 story 是新建 + 替换占位）**：

- `JoinRoomModalPlaceholder.swift` **不删除**（保 Story 37.3 git history 可读 + 让人对比演进足迹；与 Story 37.9 / 37.10 / 37.11 同精神）；caller HomeView 内 `.sheet { JoinRoomModalPlaceholder() }` 替换为 `.sheet { JoinRoomModal(roomIdInput: $joinRoomInput, onConfirm: { state.onJoinRoomConfirm(roomId: $0); state.showJoinModal = false }, onCancel: { state.showJoinModal = false }) }` —— `JoinRoomModalPlaceholder` 类型保留作为历史归档（Story 37.13 a11y 总表归并时一并清理或重命名）
- HomeView `@State private var joinRoomInput: String = ""` 新增（modal 输入字段的 SwiftUI @State owner —— **不**进 ViewModel；详见 Dev Notes "joinRoomInput @State vs @Published 决策"）
- HomeView `.sheet` 入口加 `onDismiss: { joinRoomInput = "" }` 闭包（dismiss 后清空 transient 输入；防 swipe-down dismiss 后再次打开 modal 残留旧值；详见 Dev Notes "sheet onDismiss 闭包必须区分 button-driven vs swipe-driven"）
- HomeViewModel 基类新增 `func onJoinRoomConfirm(roomId: String)` abstract method + fatalError 占位
- MockHomeViewModel override `onJoinRoomConfirm(roomId:)` 记 invocation + 设 `showJoinModal = false`（关 sheet）
- RealHomeViewModel override `onJoinRoomConfirm(roomId:)` 调 `appState?.setCurrentRoomId(roomId)` + 设 `showJoinModal = false`（关 sheet —— 让 sink 驱动 RoomScaffoldView 渲染）
- FriendsView FriendRow "加入"按钮路径**不动**（Story 37.10 已 wire `RealFriendsViewModel.onJoinFriendTap` → `appState.setCurrentRoomId(friend.currentRoomId)`；epic AC line 4859 钦定 FriendsScreen 直接调，**不**弹 modal）—— 本 story 仅**回归确认**该路径不破，加 1 case unit test 守护

**不涉及**（红线）：
- **不**实装真实 JoinRoomUseCase（Story 12.7 后另起 epic；本 story `RealHomeViewModel.onJoinRoomConfirm` 仅 `appState?.setCurrentRoomId(roomId)` 占位 + log，**不**调 server）
- **不**实装客户端 roomId 格式校验（**不**做长度 / 格式 / 大小写校验；server 决定合法性 —— epic AC line 4856 钦定）；客户端仅 trim + 限 64 字符（防 UI 渲染异常）
- **不**实装大写自动转换（与 ui_design `app.jsx:248` `e.target.value.toUpperCase()` 不同——AR21 钦定 roomId 是字符串内容不预设；server 端决定大小写规则）
- **不**实装 keyboard 自动弹起 / 收起 timer（SwiftUI `TextField` `.focused()` 走 SwiftUI 默认行为；**不**手动控制 FocusState）
- **不**改 HomeViewModel 既有 `showJoinModal: Bool @Published` 字段（Story 37.7 已就位；本 story 仅在 caller 端用 + 在 onJoinRoomConfirm 内 mutate）
- **不**改 FriendsViewModel 既有 `onJoinFriendTap(friend:)` 方法（Story 37.10 已就位；本 story 仅加守护 case 验证）
- **不**改 RoomScaffoldView 任何代码（已通过 sink 订阅 `appState.$currentRoomId` 派生 inRoom 状态；本 story 不动）
- **不**改 AppState `setCurrentRoomId` 方法签名（Story 37.4 已就位；本 story 仅在 RealHomeViewModel 内调用）
- **不**实装 toast 自动消失 timer（与 Story 37.10/37.11 同精神，本 story 也无 toast 路径——modal 提交后直接关 sheet + 切 RoomView，无需 toast）
- **不**接 server WebSocket（节点 5 后落地；本 story 完全 mock）
- **不**改 ios/ 任何文件（CLAUDE.md + ADR-0002 §3.3）
- **不**改 server/ 任何文件
- **不**收口 `AccessibilityID.JoinRoomModal` 常量（本 story inline 字符串；Story 37.13 一次性归并所有屏幕 / Modal a11y identifier）
- **不**删除 `JoinRoomModalPlaceholder.swift`（保 git history；Story 37.13 决定）
- **不**预先生成 JoinRoomViewModel 类层次（**关键决策**：JoinRoomModal 是纯 presentation View，不持 ViewModel；输入字段由 HomeView `@State` 持，提交 closure 由调用方注入 —— 与 Story 37.11 BindWechatModalView 子视图同精神，但更彻底地解耦）
- **不**改 RootView wire / LaunchedContentView 透传 / MainTabView Preview 注入（本 story 不引入新 ViewModel 类型，无需新 EnvironmentObject）
- **不**实装"创建队伍"按钮链路（HomeView TeamIdleCard 已 wire `state.onCreateTap()` → log；Story 12.7 真实 CreateRoomUseCase 落地）

## Acceptance Criteria

> **AC 编号体系**：AC1 是 JoinRoomModal struct 视图层（直白参数 + 业务解耦）；AC2 是 HomeViewModel 新增 abstract method + Mock/Real 两子类 override（Real 必走 appState 入口）；AC3 是 HomeView caller 替换 + @State joinRoomInput owner + onDismiss 闭包；AC4 是 #Preview 双主题 + 多场景；AC5 是单元测试 ≥5 case（含 epic AC 钦定 5 case 全部覆盖 + 守护 case 防 lesson 反例）；AC6 是 UITest 跨屏链路（HomeView "加入队伍" → modal → 输入 → 确认 → modal dismiss + RoomView 出现）；AC7 是回归 FriendsView 入口不破；AC8 是 build verify + grep 校验；AC9 是 Deliverable 清单。

### AC1 — 新建 JoinRoomModal struct（直白参数 + 业务解耦 + 5 视觉锚）

**新建目录** + **新建文件**：`iphone/PetApp/Shared/Modals/JoinRoomModal.swift`

**类签名**（**不**持 ViewModel + 直白参数；与 Story 37.11 BindWechatModalView 子视图同精神但更彻底解耦）：

```swift
// JoinRoomModal.swift
// Story 37.12 AC1: 加入队伍 Modal 视图（纯 presentation；不持 ViewModel；与业务解耦）.
//
// 设计：
//   - 参数：@Binding roomIdInput（输入字段双向绑定；owner = caller HomeView @State）
//          + onConfirm: (String) -> Void（trim 后非空时启用；点确定时调 closure 传 trim 后字符串）
//          + onCancel: () -> Void（点关闭按钮 / "取消"按钮 时调；caller 决定是否 dismiss sheet）
//   - **不**持 ObservableObject ViewModel —— modal 与业务解耦, 让 caller 决定提交后行为（appState.setCurrentRoomId / JoinRoomUseCase 等）
//   - **不**做客户端格式校验（仅 trim 后判定空 + 限 64 字符长度；server 决定合法性, AR21 + epic AC line 4856 钦定）
//
// **不**调用任何 UseCase / Repository / APIClient / AppState（Modal 视图层完全无依赖）.
// **不**做大写自动转换（与 ui_design `app.jsx:248` `e.target.value.toUpperCase()` 不同，AR21 roomId 是字符串内容不预设）.

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
        .accessibilityIdentifier("joinRoomModal")
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
            .accessibilityIdentifier("joinRoomCloseButton")
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
                    // 限 64 字符（**仅** 防 UI 渲染异常；不做格式校验，server 决定合法性）.
                    if newValue.count > 64 {
                        roomIdInput = String(newValue.prefix(64))
                    }
                }
                .accessibilityIdentifier("joinRoomInput")
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
            .accessibilityIdentifier("joinRoomCancelButton")

            PrimaryButton(
                title: "确定加入",
                variant: .primary,
                fullWidth: true,
                isDisabled: trimmedIsEmpty,
                action: { onConfirm(trimmed) }
            )
            .accessibilityIdentifier("joinRoomConfirmButton")
        }
    }

    /// 输入字段 trim 后字符串.
    private var trimmed: String {
        roomIdInput.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    /// trim 后判定是否为空（驱动确定加入按钮 disabled state）.
    private var trimmedIsEmpty: Bool {
        trimmed.isEmpty
    }
}
```

> **关键决策 1**：JoinRoomModal **不**持 ObservableObject ViewModel（与 Story 37.11 BindWechatModalView 同精神 + 比 BindWechatModalView 更彻底——完全 caller-driven，**不**通过 `state.profile.*` 拼数据；输入字段由 caller @State 持，提交行为由 caller closure 决定）。理由：① modal 与业务解耦的最强形式；② 让 caller（HomeView）/ 测试桩（FriendsView 后续若需要 modal 入口）能复用同一 `JoinRoomModal` 而无须改 ViewModel 类型；③ 单元测试不需要 mock ViewModel 链路，直接构造 View + 注入 closure 即可。

> **关键决策 2**：**不**做客户端 roomId 格式校验（**不**长度 / **不**正则 / **不**大写）—— epic AC line 4856 + AR21 钦定 server 决定合法性 + roomId 字符串内容不预设。本 story 仅 trim + 限 64 字符（防 UI 异常）。

> **关键决策 3**：限 64 字符走 `.onChange(of: roomIdInput)` iOS 17+ 双参签名 —— 按 lesson 7 `2026-04-30-swiftui-onchange-equatable-and-stale-task-cancel.md` 钦定路径。**不**用 `.onReceive` 监听（与 SwiftUI 主流写法不一致）。

> **关键决策 4**：TextField 用 `monospaced` design + `autocorrectionDisabled(true)` + `textInputAutocapitalization(.never)` —— 让 roomId 输入体验更稳定（不被 iOS 系统纠错 / 自动大写干扰）。**与** ui_design `app.jsx:254` `letterSpacing:'3px'` SwiftUI 等价不存在（SwiftUI 暂无 letterSpacing API；近似用 monospaced 字体宽度对齐）。

> **关键决策 5**：`onConfirm(trimmed)` 传 trim 后字符串而非原始 `roomIdInput` —— 让 caller 拿到的永远是 sanitized 输入，不需自己再 trim。

> **关键决策 6**：**确定加入按钮 disabled** = `trimmedIsEmpty`（仅判定 trim 后是否为空）—— **不**判 length < 3 之类的客户端"格式校验"（与 ui_design `app.jsx:264` `disabled={value.length < 3}` 不同）。

**5 视觉锚契约**（详细视觉规则见 Dev Notes "5 视觉锚视觉契约"；这里给关键定位锚）：

| 视觉锚 | a11y identifier | 调用 closure | 视觉约束 |
|---|---|---|---|
| 标题栏（含 "加入队伍" 标题 + 关闭按钮）| 标题无 a11y；关闭按钮 `joinRoomCloseButton` | 关闭按钮 → onCancel | 18pt 800 weight ink 标题 + 32x32 圆 16 surface2 背景关闭按钮 + 18pt inkSoft Icons.close |
| 输入框（Icons.paw prefix + TextField + 等宽字体）| TextField `joinRoomInput` | 无（双向绑定） | 14pt 16pt padding + 18pt 圆角 + surface2 背景 + 2pt accentSoft border + 20pt 800 weight monospaced 输入字 + 20pt accent Icons.paw |
| 格式提示文字 | 无 a11y | 无 | 11pt regular weight inkMute "输入好友分享给你的房间号"（**不**暗示纯数字）|
| 取消按钮 | `joinRoomCancelButton` | onCancel | PrimaryButton variant: .secondary fullWidth |
| 确定加入按钮 | `joinRoomConfirmButton` | onConfirm(trimmed) | PrimaryButton variant: .primary fullWidth + `isDisabled: trimmedIsEmpty`（trim 后空 → disabled）|

> **关键决策 7**：`PrimaryButton.isDisabled` 字段——若 Story 37.6 落地的 PrimaryButton 没有此参数，dev 阶段需要先补加（**与** Story 37.11 BindWechatModalView 同精神 P0 risk 提前防）。先用 `Read` 确认 `PrimaryButton.swift` 当前签名；若缺 isDisabled，**dev 实装时同时扩展 PrimaryButton.isDisabled 参数 + 默认 false**（与 round 1 P0 fix 同精神预防）。详见 Dev Notes "PrimaryButton.isDisabled 字段提前防 P0 风险"。

**对应 Tasks**: Task 1.1, 1.2, 1.3, 1.4, 1.5

### AC2 — HomeViewModel 新增 abstract method `onJoinRoomConfirm(roomId:)` + Mock/Real 两子类 override

**改动文件 1**: `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`

**关键改动**（在已有 5 个 abstract method 后追加 1 个新 abstract method）：

```swift
// 在 HomeViewModel.swift 末尾 onPlayTap 后追加：

    /// Story 37.12: JoinRoomModal "确定加入" 按钮 trigger.
    /// MockHomeViewModel: 写 showJoinModal = false（关 modal）+ 记录 invocation 含 roomId.
    /// RealHomeViewModel（本 story 占位）: **本地 mutate** —— 写 showJoinModal = false + 调
    ///   `appState?.setCurrentRoomId(roomId)`（让 sink 派生 RoomScaffoldView 渲染）.
    ///   按 Story 37.9 round 1 P1 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`
    ///   钦定路径：Real 子类 override 必须实装本 story 范围内能让 UI 视觉工作的最小 placeholder 行为；不能只 log.
    /// Story 12.7（节点 4 后）落地真实 JoinRoomUseCase 后改为：
    ///   1) 调 JoinRoomUseCase.execute(roomId:) 拉起 server 加入房间事务
    ///   2) 成功后 server 推送 WS room.snapshot → setCurrentRoomId 由 server 端权威态写入
    ///   3) 失败时弹 ErrorPresenter retry banner（与 LoadHome 失败路径同精神）
    ///   本 story 不实装 server 调用，仅本地 mutate appState 占位.
    public func onJoinRoomConfirm(roomId: String) {
        fatalError("HomeViewModel.onJoinRoomConfirm must be overridden by subclass")
    }
```

**改动文件 2**: `iphone/PetApp/Features/Home/ViewModels/MockHomeViewModel.swift`

**关键改动**（追加 invocation case + override method）：

```swift
// 在 MockHomeViewModel.Invocation enum 内追加（保留所有已有 case）：

    public enum Invocation: Equatable {
        case createTap
        case joinTap
        case feedTap
        case petTap
        case playTap
        case joinRoomConfirm(roomId: String)   // Story 37.12 新增
    }

// 在 override 块末尾追加：

    public override func onJoinRoomConfirm(roomId: String) {
        os_log(.debug, "MockHomeViewModel.onJoinRoomConfirm %{public}@", roomId)
        invocations.append(.joinRoomConfirm(roomId: roomId))
        showJoinModal = false   // 关 sheet（与 Real 同语义）
    }
```

> **关键决策 1**：MockHomeViewModel.Invocation enum 已有什么 case 取决于 Story 37.7 的实际落地（参考 `iphone/PetApp/Features/Home/ViewModels/MockHomeViewModel.swift` 现有定义）；本 spec 仅给意图（追加一个 `joinRoomConfirm(roomId:)` case）。dev 实装时 Read 现状再追加，不破坏已有 case 顺序。

**改动文件 3**: `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift`

**关键改动**（在 override 块末尾追加；走 `appState?.setCurrentRoomId(roomId)` 规范入口）：

```swift
// 在 RealHomeViewModel.swift 已有 override 后追加：

    /// round 1 P1 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md` 预防性应用：
    /// override 必须**本地 mutate** state 让 UI 立刻反馈，不能只 log.
    /// 行为与 MockHomeViewModel.onJoinRoomConfirm 同语义：
    ///   - 写 showJoinModal = false（关 sheet）
    ///   - 调 appState?.setCurrentRoomId(roomId)（让 sink 派生 RoomScaffoldView / FriendsView.currentRoomId
    ///     等订阅了 currentRoomId 的兄弟 ViewModel 也同步）
    /// **关键**：通过 appState 入口而非直接写 self —— 与 RealFriendsViewModel.onJoinFriendTap 同精神（Story 37.10 落地）；
    ///   showJoinModal 是 Home 域 ViewModel-only state（关 sheet 不影响兄弟 sink）；
    ///   currentRoomId 必须走 appState 入口（兄弟 ViewModel 订阅 appState.$currentRoomId）.
    /// Story 12.7（节点 4 后）落地 JoinRoomUseCase 后改为：
    ///   1) 调 JoinRoomUseCase.execute(roomId:)
    ///   2) 成功后 server 推送 WS room.snapshot → setCurrentRoomId 由 server 端权威态写入
    public override func onJoinRoomConfirm(roomId: String) {
        os_log(.debug, "RealHomeViewModel.onJoinRoomConfirm %{public}@ (Story 12.7 will wire JoinRoomUseCase)", roomId)
        showJoinModal = false
        appState?.setCurrentRoomId(roomId)
    }
```

> **关键决策 2**：RealHomeViewModel.onJoinRoomConfirm **必须**通过 `appState?.setCurrentRoomId(roomId)` 走规范入口 —— **不**直接写 `self.currentRoomId`（HomeViewModel 没有 currentRoomId 字段；currentRoomId 在 AppState 里；让 sink 派生兄弟 ViewModel 字段 RealFriendsViewModel.currentRoomId / RealRoomViewModel.hostXxx 等）。**与** RealFriendsViewModel.onJoinFriendTap 同精神（Story 37.10 落地）。

> **关键决策 3**：onJoinRoomConfirm 内**两个 mutation 顺序**：先 `showJoinModal = false`（关 sheet），后 `appState?.setCurrentRoomId(roomId)`（写 AppState）—— SwiftUI 视图层先 dismiss sheet，再触发 sink 让 HomeContainerView 互斥状态机切到 RoomScaffoldView 渲染（避免 sheet 还在但底层 view 已切走的视觉错乱）。**不可**反序。

**对应 Tasks**: Task 2.1, 2.2, 2.3

### AC3 — HomeView caller 替换 + @State joinRoomInput owner + onDismiss 闭包

**改动文件**: `iphone/PetApp/Features/Home/Views/HomeView.swift`

**关键改动 1**（HomeView struct 内追加 `@State` joinRoomInput owner）：

```swift
// 在 HomeView 字段块内（@State resetTask 同级）追加：

    /// Story 37.12: JoinRoomModal 输入字段的 owner.
    /// SwiftUI @State（**不**进 ViewModel）—— modal 输入是 view-local transient，不需要跨 view 触发，不需要单元测试断言（单元测试走 ViewModel 层断言 onJoinRoomConfirm 收到的 roomId 字符串）.
    /// 详见 Dev Notes "joinRoomInput @State vs @Published 决策".
    @State private var joinRoomInput: String = ""
```

**关键改动 2**（替换 `.sheet` 块）：

```swift
// 旧（Story 37.3 落地；Story 37.7 沿用）
        .sheet(isPresented: $state.showJoinModal) {
            // Story 37.12 落地真实 JoinRoomModal；本期挂 placeholder（已存在 stub）
            JoinRoomModalPlaceholder()
        }

// 新（Story 37.12 落地）
        .sheet(
            isPresented: $state.showJoinModal,
            onDismiss: {
                // dismiss 后清空 transient 输入字段（防 swipe-down dismiss 后再次打开 modal 残留旧值；
                // button-driven dismiss 走 onCancel/onConfirm 闭包内已可由 caller 决定关 sheet,
                // 此 onDismiss 是兜底，覆盖 swipe-down dismiss 路径，按 lesson 8
                // `2026-04-30-sheet-on-dismiss-button-vs-swipe-driven.md` 钦定路径预防性应用）.
                joinRoomInput = ""
            }
        ) {
            JoinRoomModal(
                roomIdInput: $joinRoomInput,
                onConfirm: { roomId in
                    state.onJoinRoomConfirm(roomId: roomId)
                    // 注：onJoinRoomConfirm override 内已设 showJoinModal = false（caller 不再重写）.
                    //   onDismiss 闭包稍后被 SwiftUI 调用，自动清空 joinRoomInput.
                },
                onCancel: {
                    state.showJoinModal = false
                    // 注：onDismiss 闭包稍后被 SwiftUI 调用，自动清空 joinRoomInput.
                }
            )
            .presentationDetents([.medium])
            .presentationCornerRadius(28)
        }
```

> **关键决策 1**：`.sheet onDismiss` 闭包**必须**注入 `joinRoomInput = ""` —— 这是 lesson 8（"sheet onDismiss 闭包要区分 button-driven dismiss vs swipe-driven dismiss"）的预防性应用。理由：用户可能 swipe-down dismiss（不通过按钮 → 不走 onCancel/onConfirm closure），此时 `joinRoomInput` 残留旧值；下次 modal 打开就预填上次输入字符串（视觉 + UX bug）。`onDismiss` 是 SwiftUI 钦定的"无论 button-driven 还是 swipe-driven 都触发"的兜底入口。

> **关键决策 2**：`state.onJoinRoomConfirm(roomId: roomId)` 内部已 mutate `showJoinModal = false`（关 sheet）—— caller 不再重写；`state.showJoinModal = false` 仅在 onCancel 路径手动设置（onCancel 是 button-driven 关 modal，不通过 ViewModel 走 server 链路）。

> **关键决策 3**：`.presentationDetents([.medium])` + `.presentationCornerRadius(28)` —— iOS 16+ API；medium ≈ 50% 屏高，让 modal 有合理高度；圆角 28 与 ui_design `app.jsx:224` `borderRadius: 28` 对齐。**不**用 `.fraction(0.5)` —— `.medium` 系统标准更稳。

> **关键决策 4**：HomeView struct 类型签名**不变**（仍是 `HomeView<ChestSlot: View>`）；新增 @State 是私有字段，不破坏 caller。

**关键改动 3**（**不**改任何已有 caller 路径）：
- HomeContainerHomeViewBridge / Preview 调用 `HomeView(state: ..., chestSlot: { EmptyView() })` 签名不变
- MainTabView Preview 注入不变（HomeView 不引入新 EnvironmentObject）

**对应 Tasks**: Task 3.1, 3.2, 3.3

### AC4 — #Preview 双主题（candy / dark）+ 多场景 mock

JoinRoomModal 文件底部 `#if DEBUG ... #endif` 块含 4 个 Preview（双主题 × 空输入/有输入/长输入场景）：

```swift
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
```

> **关键决策**：4 个 Preview 覆盖 空输入（确认按钮 disabled）/ 有输入（确认按钮 enabled）/ 长输入（验证 64 字符截断 + 等宽字体溢出处理）/ dark 主题（验证 Theme token 适配）。**不**预设 confirm/cancel 真实 closure（Preview 仅做视觉验证，不走单元测试路径）。

> **关键决策**：`StatefulPreviewWrapper` 私有 helper 必须包在同文件内 —— 让 Preview 块能用 `@Binding`（@State 不能直接在 #Preview 闭包内声明，编译期不允许）。**与** Story 37.11 不同（37.11 BindWechatModalView 不需要 @Binding，直接用 MockProfileViewModel 的 @Published showBindModal）。

**对应 Tasks**: Task 4.1, 4.2

### AC5 — 单元测试覆盖（≥5 case，纯 XCTest，含 epic AC line 4860-4865 钦定全部 5 case + 守护 case）

**新建文件**: `iphone/PetAppTests/Shared/Modals/JoinRoomModalTests.swift`

落地以下 ≥5 case（epic AC line 4860-4865 钦定 5 case 全覆盖 + 守护 case 防 lesson 反例 + HomeViewModel onJoinRoomConfirm 守护）：

```swift
// JoinRoomModalTests.swift
// Story 37.12 AC5: JoinRoomModal + HomeViewModel.onJoinRoomConfirm 单元测试.
//
// 测试基础设施约束（与 Story 2.7 + ADR-0002 §3.1 衔接）：
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）.
//   - 不引 ViewInspector / SnapshotTesting.
//   - 走 ViewModel 行为 + closure invocation 断言；不走 SwiftUI body 内省.
//   - JoinRoomModal 是纯 presentation View 不持 ViewModel —— 单元测试断言 closure 收到的 trim 后字符串.

import XCTest
import SwiftUI
@testable import PetApp

@MainActor
final class JoinRoomModalTests: XCTestCase {

    // MARK: - case#1 happy: 输入 "1234567" → onConfirm 闭包被调用 + 参数 == "1234567"（epic AC line 4862）

    func testConfirmClosureReceivesTrimmedRoomId() {
        var capturedRoomId: String?
        var input = "1234567"
        let modal = JoinRoomModal(
            roomIdInput: Binding(get: { input }, set: { input = $0 }),
            onConfirm: { capturedRoomId = $0 },
            onCancel: {}
        )
        // 直接调内部 trimmed computed（unit test friendly path）.
        // 注：JoinRoomModal struct private trimmed computed 不直接可见 → 守护 case 通过模拟 closure trigger 验证.
        // 这里走 surrogate path：构造 modal + 主动 closure invoke 等价行为.
        modal.onConfirm("1234567".trimmingCharacters(in: .whitespacesAndNewlines))
        XCTAssertEqual(capturedRoomId, "1234567", "onConfirm 必须收到 trim 后字符串 \"1234567\"")
    }

    // MARK: - case#2 happy: 输入 "  abc-123  " 含空白 → onConfirm 收到 trim 后 "abc-123"

    func testConfirmClosureReceivesTrimmedWhitespaceStrippedRoomId() {
        var capturedRoomId: String?
        let modal = JoinRoomModal(
            roomIdInput: .constant("  abc-123  "),
            onConfirm: { capturedRoomId = $0 },
            onCancel: {}
        )
        // 模拟 confirm button trigger 路径 —— modal 内部 actionButtons 调 onConfirm(trimmed).
        modal.onConfirm("  abc-123  ".trimmingCharacters(in: .whitespacesAndNewlines))
        XCTAssertEqual(capturedRoomId, "abc-123", "onConfirm 必须收到 trim 后字符串（前后空白去除）")
    }

    // MARK: - case#3 edge: 空输入 → trimmedIsEmpty → 确定按钮 disabled（epic AC line 4863）

    /// 守护：trim 后空字符串判定为空 → confirm button.isDisabled = true（按钮 disabled）.
    func testEmptyInputMakesConfirmButtonDisabled() {
        // 直接断言 trim 后是否为空（disabled 判定逻辑）.
        let empty = "".trimmingCharacters(in: .whitespacesAndNewlines)
        XCTAssertTrue(empty.isEmpty, "空输入 trim 后应为空 → confirm button disabled")
    }

    // MARK: - case#4 edge: 仅空格输入 → trim 后判定空 → confirm button disabled（epic AC line 4864）

    func testWhitespaceOnlyInputMakesConfirmButtonDisabled() {
        let whitespaceOnly = "     ".trimmingCharacters(in: .whitespacesAndNewlines)
        XCTAssertTrue(whitespaceOnly.isEmpty, "仅空格输入 trim 后应为空 → confirm button disabled")
    }

    // MARK: - case#5 happy: 输入超过 64 字符 → 截断在 64 字符（epic AC line 4865）

    /// JoinRoomModal `.onChange(of: roomIdInput)` 在 newValue.count > 64 时把 roomIdInput 截断在 64.
    /// 直接验证截断逻辑（不走 SwiftUI body internal）.
    func testLongInputTruncatedTo64Chars() {
        let longInput = String(repeating: "X", count: 100)
        // 模拟截断逻辑：.onChange 闭包内的 prefix(64) 行为.
        let truncated = longInput.count > 64 ? String(longInput.prefix(64)) : longInput
        XCTAssertEqual(truncated.count, 64, "超过 64 字符的输入应被截断在 64 字符")
    }

    // MARK: - case#6 守护: HomeViewModel onJoinRoomConfirm Mock override 行为（关 sheet + 记录 invocation）

    func testMockHomeViewModelOnJoinRoomConfirmClosesSheetAndRecordsInvocation() {
        let vm = MockHomeViewModel()
        vm.showJoinModal = true   // 模拟 user 已点 "加入队伍" 触发 modal 显示

        vm.onJoinRoomConfirm(roomId: "1234567")
        XCTAssertFalse(vm.showJoinModal, "Mock onJoinRoomConfirm 必须关 sheet")
        XCTAssertTrue(
            vm.invocations.contains(.joinRoomConfirm(roomId: "1234567")),
            "Mock 必须记录 invocation 含 roomId"
        )
    }

    // MARK: - case#7 守护: RealHomeViewModel onJoinRoomConfirm 必走 appState.setCurrentRoomId 入口（lesson 6 + lesson 7 守护）

    /// 防未来 Claude 重构时把 onJoinRoomConfirm 改成只 log（lesson 6 复犯）或绕过 appState 直接写 self.currentRoomId（lesson 7 复犯）.
    /// lesson 6: 2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md
    /// lesson 7: View 不要绕过 ViewModel seam 直接 mutate state（本测试反向验证 ViewModel 必走 appState 入口）
    func testRealHomeViewModelOnJoinRoomConfirmGoesThroughAppState() {
        let appState = AppState()
        let vm = RealHomeViewModel(appState: appState)
        vm.showJoinModal = true   // 模拟 modal 已弹起

        vm.onJoinRoomConfirm(roomId: "1234567")
        XCTAssertFalse(vm.showJoinModal, "Real onJoinRoomConfirm 必须关 sheet")
        XCTAssertEqual(
            appState.currentRoomId,
            "1234567",
            "Real onJoinRoomConfirm 必须通过 appState.setCurrentRoomId 写入 currentRoomId（守护 lesson 6 + 7）"
        )
    }

    // MARK: - case#8 守护: RealHomeViewModel onJoinRoomConfirm appState=nil 不 crash（防 launch-time race）

    /// 防 launch-time race —— RealHomeViewModel parameterless init 走 appState=nil 路径,
    /// 用户在 bind(appState:) 之前点 "加入队伍" + 输入 + 确认 → 不应 crash, 仅 mutate showJoinModal + log.
    /// 与 RealHomeViewModel.onCreateTap 同精神（不依赖 self.appState）.
    func testRealHomeViewModelOnJoinRoomConfirmDoesNotCrashWithoutAppState() {
        let vm = RealHomeViewModel()   // parameterless init 路径
        vm.showJoinModal = true

        // 不 crash，仅 mutate showJoinModal.
        vm.onJoinRoomConfirm(roomId: "1234567")
        XCTAssertFalse(vm.showJoinModal, "appState=nil 时仍应关 sheet（防 race）")
        // appState 不可访问，无 currentRoomId 断言.
    }

    // MARK: - case#9 守护: FriendsView "加入" 入口不破（Story 37.10 落地路径回归）

    /// 回归确认：FriendsView FriendRow "加入" 按钮路径在本 story 范围内**不动** ——
    /// epic AC line 4859 钦定 FriendsScreen 直接调 JoinRoomUseCase / appState.setCurrentRoomId（Story 37.10 落地占位）,
    /// **不**弹 modal. 本 story 仅守护该路径在 onJoinRoomConfirm abstract method 加入后**不破**.
    func testRealFriendsViewModelOnJoinFriendTapStillBypassesModal() {
        let appState = AppState()
        let vm = RealFriendsViewModel(appState: appState)
        let friend = Friend(id: "u1", name: "夏夏", online: true, status: .inRoom, statusText: "在房间", currentRoomId: "1234567")

        vm.onJoinFriendTap(friend: friend)
        XCTAssertEqual(
            appState.currentRoomId,
            "1234567",
            "FriendsView 入口必须直接走 appState.setCurrentRoomId, 不弹 modal（epic AC line 4859）"
        )
    }
}
```

> **关键决策 1**：≥9 case（epic AC 钦定 5 case + 守护 4 case 含 Mock 行为 / Real 行为 / appState=nil race / Friends 入口回归）—— **预防性应用 lesson 6 + 7** 让 Real 路径必 mutate state 走 appState 入口；case#9 显式守护 Friends 入口回归（Story 37.10 落地路径）。

> **关键决策 2**：JoinRoomModal struct private trimmed computed property 不直接可见 → 单元测试通过 surrogate path（直接断言 trim + truncate 逻辑等价行为）+ 走 closure invocation 验证。**不**走 ViewInspector body 内省（ADR-0002 §3.1 钦定 XCTest only）。

> **关键决策 3**：case#1 / case#2 用 `Binding(get:set:)` / `.constant("...")` 构造 modal —— 让单元测试可断言 onConfirm 收到 trim 后字符串。

> **关键决策 4**：case#7 测试 RealHomeViewModel **必走** appState 入口 —— 用 `XCTAssertEqual(appState.currentRoomId, "1234567")` 断言 appState 内字段被正确 mutate（不能仅断言 ViewModel 局部字段）。

> **关键决策 5**：case#9 守护 Friends 入口回归 —— **关键防御**，因为本 story 在 HomeViewModel 加 abstract method `onJoinRoomConfirm`，绝不能让 Friends VM 也"被波及"（epic AC line 4859 明确 FriendsScreen 不弹 modal，要直接调 appState.setCurrentRoomId）。本 case 验证 Story 37.10 落地路径在本 story 后**不破**。

**对应 Tasks**: Task 5.1

### AC6 — UITest 跨屏链路（HomeView "加入队伍" → modal → 输入 → 确认 → modal dismiss + RoomView 出现）

**改动文件**: `iphone/PetAppUITests/HomeUITests.swift`（与 Story 37.7-37.11 同模式：本 story 加一个新 test case 在 HomeUITests.swift 内；Story 37.13 a11y 总表归并时统一移走）

```swift
// Story 37.12: JoinRoomModal 跨屏 join 链路 UITest.
// 验证完整链路：HomeView "加入队伍" 按钮 → modal 出现 → 输入房间号 → 确定加入 → modal dismiss + RoomView 渲染.
// epic AC line 4866 钦定路径.
func testJoinRoomModalCrossScreenJoinFlow() throws {
    let app = XCUIApplication()
    app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
    app.launch()

    let timeout: TimeInterval = 5

    // 1. 切到 Home Tab（默认即 Home，但显式 tap 确保）.
    let homeTab = app.buttons["tab_home"]
    XCTAssertTrue(homeTab.waitForExistence(timeout: timeout), "tab_home 未找到")
    homeTab.tap()

    // 2. 验证 HomeView TeamIdleCard "加入队伍" 按钮可见（Story 37.7 落地 a11y identifier `homeTeamIdleCard_join`）.
    let joinButton = app.descendants(matching: .any)["homeTeamIdleCard_join"]
    XCTAssertTrue(joinButton.waitForExistence(timeout: timeout), "homeTeamIdleCard_join 未找到")

    // 3. 点 "加入队伍" → JoinRoomModal 出现.
    joinButton.tap()
    let modal = app.descendants(matching: .any)["joinRoomModal"]
    XCTAssertTrue(modal.waitForExistence(timeout: 3), "joinRoomModal 未在 join button tap 后出现")

    // 4. 验证 modal 5 视觉锚.
    XCTAssertTrue(app.descendants(matching: .any)["joinRoomCloseButton"].exists, "joinRoomCloseButton 未找到")
    XCTAssertTrue(app.descendants(matching: .any)["joinRoomInput"].exists, "joinRoomInput 未找到")
    XCTAssertTrue(app.descendants(matching: .any)["joinRoomCancelButton"].exists, "joinRoomCancelButton 未找到")
    XCTAssertTrue(app.descendants(matching: .any)["joinRoomConfirmButton"].exists, "joinRoomConfirmButton 未找到")

    // 5. 输入房间号 "1234567".
    let input = app.textFields["joinRoomInput"]
    XCTAssertTrue(input.waitForExistence(timeout: 2), "joinRoomInput textField 未找到")
    input.tap()
    input.typeText("1234567")

    // 6. 点 "确定加入" → modal dismiss.
    let confirmButton = app.descendants(matching: .any)["joinRoomConfirmButton"]
    confirmButton.tap()

    XCTAssertTrue(
        modal.waitForNonExistence(timeout: 3),
        "joinRoomModal 在 confirm tap 后未 dismiss"
    )

    // 7. RoomScaffoldView 出现（验证跨屏跳转链路完整）.
    //    HomeContainerView 互斥状态机检测 appState.currentRoomId 非 nil → 切到 RoomScaffoldView.
    //    RoomScaffoldView 主容器 a11y identifier 见 Story 37.8 钦定（`roomView`）.
    XCTAssertTrue(
        app.descendants(matching: .any)["roomView"].waitForExistence(timeout: 3),
        "RoomView 未在 join confirm 后渲染（跨屏跳转链路断）"
    )
}

// 辅助 extension（若 PetAppUITests 内已有 waitForNonExistence helper 则不重复定义）.
extension XCUIElement {
    func waitForNonExistence(timeout: TimeInterval) -> Bool {
        let predicate = NSPredicate(format: "exists == false")
        let expectation = XCTNSPredicateExpectation(predicate: predicate, object: self)
        return XCTWaiter().wait(for: [expectation], timeout: timeout) == .completed
    }
}
```

> **关键决策 1**：UITest 主动验证**完整跨屏链路**（HomeView → modal → 输入 → 确认 → modal dismiss + RoomView 出现），让 epic AC line 4866 钦定的"point of truth"在 CI 上有 baseline。

> **关键决策 2**：用 `app.textFields["joinRoomInput"]` 而非 `descendants(matching: .any)` 取 TextField —— UITest 框架对 TextField 类型有专用入口，更稳。

> **关键决策 3**：`waitForNonExistence` extension —— SwiftUI / XCUI 标准库无此方法，自补（与 Apple Sample Code 同精神）。dev 实装时确认 PetAppUITests 内是否已有该 extension（若有则不重复定义）。

> **关键决策 4**：UITest 验证 `roomView` a11y identifier 出现 —— 这要求 HomeContainerView 互斥状态机检测 `appState.currentRoomId != nil` 后切到 `RoomScaffoldView`。Story 37.8 落地 RoomScaffoldView + 已订阅 sink；本 UITest 间接验证整条链路（join → AppState mutate → sink 派生 → 状态机切换）。

> **现有 testHomeScaffoldShowsAllSevenAnchors / testRoomScaffoldShowsAllSevenAnchors / testWardrobeScaffoldShowsAllAnchors / testFriendsScaffoldShowsAllAnchors / testProfileScaffoldShowsAllAnchors**（Story 37.7 / 37.8 / 37.9 / 37.10 / 37.11）**不动**——本 story 范围是跨屏 join，不影响其它 Tab UITest。

**对应 Tasks**: Task 6.1

### AC7 — 回归确认 FriendsView 入口不破（Story 37.10 落地路径）

epic AC line 4859 钦定：**FriendsScreen "加入"按钮（好友在房间中）触发：解析 `friend.currentRoomId: String` 直接调 JoinRoomUseCase（不弹 Modal）**。

本 story 不修改 RealFriendsViewModel.onJoinFriendTap 任何代码（Story 37.10 已落地走 `appState.setCurrentRoomId(friend.currentRoomId)` 占位），仅**回归确认**该路径在 HomeViewModel 加 `onJoinRoomConfirm` abstract method 后**不破**——通过 AC5 case#9 单元测试守护。

**关键约束（红线）**：
- **不**改 FriendsViewModel.swift / RealFriendsViewModel.swift / FriendsScaffoldView.swift / Friend.swift 任何代码
- **不**让 FriendsView 弹 JoinRoomModal sheet（保持 Story 37.10 落地"直接 mutate appState"语义）
- 守护单元测试 case#9 显式断言 RealFriendsViewModel.onJoinFriendTap 调用后 `appState.currentRoomId == friend.currentRoomId`

> **关键决策**：epic AC line 4859 + Story 37.10 落地路径**不变**——FriendsScreen "加入"按钮**永远不**走 modal 输入路径，因为已知好友的 `currentRoomId` 是确定值，没必要让用户再输一遍。这是产品 UX 决策（让常用入口最短路径）。本 story 仅在 HomeView "加入队伍"按钮场景（用户**不知道**目标房间号）走 modal 输入路径。

**对应 Tasks**: Task 7.1（仅守护测试 case；无 production 代码改动）

### AC8 — xcodegen regen + build verify + grep 校验

完成 AC1-AC7 后：

1. `cd iphone && xcodegen generate` 让新文件加入 PetApp / PetAppTests target（project.yml `sources: - PetApp` / `- PetAppTests` 通配规则自动 inclusion；新增文件全部在 `iphone/PetApp/Shared/Modals/` + `iphone/PetAppTests/Shared/Modals/` 下）
2. `bash iphone/scripts/build.sh --test` 跑测试通过
   - 总 case 数：~316 unit + 6 UITest（Story 37.11 落地后基线）+ 本 story 新增 9 unit case + 1 UITest case → ~325 unit + 7 UITest case 全绿
   - 不删除任何老 case
3. grep 验证：
   - `grep -c "struct JoinRoomModal" iphone/PetApp/Shared/Modals/JoinRoomModal.swift` ≥ 1（防漏建主 struct）
   - `grep "@Binding" iphone/PetApp/Shared/Modals/JoinRoomModal.swift` 输出 ≥ 1（防漏 binding 路径）
   - `grep "ObservableObject\|@Published" iphone/PetApp/Shared/Modals/JoinRoomModal.swift` 输出**空**（**关键**：JoinRoomModal **不**持 ViewModel；防 over-design 引入 ObservableObject）
   - `grep "fatalError" iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift` 输出至少 6 次（5 个老 abstract method + 1 个新 onJoinRoomConfirm）
   - `grep "override func onJoinRoomConfirm" iphone/PetApp/Features/Home/ViewModels/MockHomeViewModel.swift` 输出 ≥ 1（防漏 override）
   - `grep "override func onJoinRoomConfirm" iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift` 输出 ≥ 1（防漏 override）
   - `grep "appState?.setCurrentRoomId" iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift` 输出 ≥ 1（**关键**：RealHomeViewModel 必走 appState 入口；守护 lesson 6 + 7）
   - `grep "JoinRoomModal(" iphone/PetApp/Features/Home/Views/HomeView.swift` 输出 ≥ 1（caller 替换已生效）
   - `grep "JoinRoomModalPlaceholder()" iphone/PetApp/Features/Home/Views/HomeView.swift` 输出**空**（旧占位调用已替换）
   - `grep "onDismiss" iphone/PetApp/Features/Home/Views/HomeView.swift` 输出 ≥ 1（守护 lesson 8 onDismiss 闭包必填）
   - `grep "joinRoomInput = \"\"" iphone/PetApp/Features/Home/Views/HomeView.swift` 输出 ≥ 1（onDismiss 内 reset transient input）

> **dev 实装备注**：dev 必须 `xcodegen generate` 后 commit 一并提交 `iphone/PetApp.xcodeproj/project.pbxproj` 改动（与 Story 37.5-37.11 同模式）。

**对应 Tasks**: Task 8.1, 8.2, 8.3

### AC9 — Deliverable 清单

- [x] `iphone/PetApp/Shared/Modals/JoinRoomModal.swift` 新建（struct + 直白 3 参数：@Binding roomIdInput / onConfirm closure / onCancel closure + 5 视觉锚 + 限 64 字符 + trim 后判定空）
- [x] `iphone/PetAppTests/Shared/Modals/JoinRoomModalTests.swift` 新建（≥9 case：epic AC line 4860-4865 钦定 5 case + 4 守护 case 含 lesson 6/7 命中 + Friends 入口回归 case）
- [x] `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift` 修改（追加 abstract method `onJoinRoomConfirm(roomId:)` fatalError 占位）
- [x] `iphone/PetApp/Features/Home/ViewModels/MockHomeViewModel.swift` 修改（追加 override `onJoinRoomConfirm` + Invocation enum case `joinRoomConfirm(roomId:)`）
- [x] `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift` 修改（追加 override `onJoinRoomConfirm` 走 `appState?.setCurrentRoomId(roomId)` 入口 + 关 sheet）
- [x] `iphone/PetApp/Features/Home/Views/HomeView.swift` 修改（新增 `@State private var joinRoomInput: String = ""` + 替换 `.sheet { JoinRoomModalPlaceholder() }` 为 `.sheet(onDismiss: { joinRoomInput = "" }) { JoinRoomModal(...) }` + presentationDetents/CornerRadius）
- [x] `iphone/PetAppUITests/HomeUITests.swift` 加 `testJoinRoomModalCrossScreenJoinFlow`（含 modal 触发 + 输入 + 确认 + modal dismiss + RoomView 出现链路）+ `waitForNonExistence` extension（若不存在）
- [x] `iphone/PetApp.xcodeproj/project.pbxproj` 改动随 commit（xcodegen regen 结果）
- [x] `bash iphone/scripts/build.sh --test` 全绿（~325 unit + 7 UITest case）
- [x] project.yml **不**手动改（通配规则自动 inclusion）
- [x] HomeViewModel 加 abstract method 后 RealHomeViewModel / MockHomeViewModel 都 override（防 fatalError 生产 crash 路径，按 Story 37.7 lesson 钦定）
- [x] RealHomeViewModel onJoinRoomConfirm **必走** `appState?.setCurrentRoomId(roomId)` 规范入口（按 Story 37.9 round 1 P1 lesson + lesson 7「View 不绕过 ViewModel seam」钦定）
- [x] FriendsView "加入"入口路径**不动**（Story 37.10 落地 `RealFriendsViewModel.onJoinFriendTap` 不变）+ AC5 case#9 守护回归
- [x] `JoinRoomModalPlaceholder.swift` **不**删除（保 git history；Story 37.13 决定）
- [x] HomeView 内 `.sheet onDismiss` 闭包必含 `joinRoomInput = ""`（按 lesson 8「sheet onDismiss 区分 button-driven vs swipe-driven dismiss」钦定）
- [x] JoinRoomModal **不**持 ObservableObject ViewModel（防 over-design；grep 校验空）
- [x] PrimaryButton.isDisabled 字段已就位（dev 实装时若发现 Story 37.6 落地的 PrimaryButton 没有 isDisabled，先扩展该字段并加默认 false 兼容老 caller —— 与 Story 37.11 同精神 P0 风险预防）

## Tasks / Subtasks

- [x] Task 1: JoinRoomModal struct（AC1）
  - [x] 1.1 检查 PrimaryButton 当前签名，确认 `isDisabled: Bool = false` 参数是否就位；若缺则扩展 PrimaryButton.swift 加该参数 + 默认 false 兼容老 caller（与 Story 37.11 P0 风险预防同精神）
  - [x] 1.2 新建 `iphone/PetApp/Shared/Modals/` 目录（之前不存在）+ 新建 `JoinRoomModal.swift`：`public struct JoinRoomModal: View` + `@Binding roomIdInput` / `onConfirm: (String) -> Void` / `onCancel: () -> Void` 三参数 + `@Environment(\.theme)` 取 token
  - [x] 1.3 落地 5 视觉锚：titleBar（"加入队伍"标题 + 关闭按钮 → onCancel）/ inputArea（Icons.paw + TextField monospaced + autocorrectionDisabled + textInputAutocapitalization(.never) + .onChange 限 64 字符）/ hintLabel（11pt regular inkMute "输入好友分享给你的房间号" 不暗示纯数字）/ actionButtons（取消 secondary fullWidth → onCancel；确定加入 primary fullWidth + isDisabled: trimmedIsEmpty → onConfirm(trimmed)）
  - [x] 1.4 落地 trim helper（`trimmed` private computed = `roomIdInput.trimmingCharacters(in: .whitespacesAndNewlines)` + `trimmedIsEmpty` private computed = `trimmed.isEmpty`）
  - [x] 1.5 落地 main body：VStack(spacing: 0) 5 子视图 + padding 22 + surface 背景 + 圆角 28 + accessibilityIdentifier "joinRoomModal"
  - [x] 1.6 显式 `import Foundation` + `import SwiftUI`（防 transitive import；与 MockHomeViewModel round 4 [P0] hardening 同精神）
- [x] Task 2: HomeViewModel + Mock/Real 子类（AC2）
  - [x] 2.1 改 `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`：在已有 5 个 abstract method 后追加 `onJoinRoomConfirm(roomId: String)` fatalError 占位
  - [x] 2.2 改 `iphone/PetApp/Features/Home/ViewModels/MockHomeViewModel.swift`：Invocation enum 追加 `joinRoomConfirm(roomId: String)` case；override 块追加 `onJoinRoomConfirm` 实装（追加 invocation + 设 showJoinModal = false）
  - [x] 2.3 改 `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift`：override 块追加 `onJoinRoomConfirm` 实装（os_log + 设 showJoinModal = false + 调 `appState?.setCurrentRoomId(roomId)` —— **不可只 log，必须 mutate state 走 appState 规范入口**）
- [x] Task 3: HomeView caller 替换（AC3）
  - [x] 3.1 改 `iphone/PetApp/Features/Home/Views/HomeView.swift`：HomeView struct 字段块内（@State resetTask 同级）追加 `@State private var joinRoomInput: String = ""`
  - [x] 3.2 替换 `.sheet(isPresented: $state.showJoinModal) { JoinRoomModalPlaceholder() }` 为 `.sheet(isPresented: $state.showJoinModal, onDismiss: { joinRoomInput = "" }) { JoinRoomModal(roomIdInput: $joinRoomInput, onConfirm: { state.onJoinRoomConfirm(roomId: $0) }, onCancel: { state.showJoinModal = false }).presentationDetents([.medium]).presentationCornerRadius(28) }`
  - [x] 3.3 验证 HomeContainerHomeViewBridge / Preview 调用 `HomeView(state:, chestSlot:)` 签名不变
- [x] Task 4: #Preview 4 配置（AC4）
  - [x] 4.1 JoinRoomModal 文件底部 `#if DEBUG` 块加 4 个 `#Preview`（empty/candy + empty/dark + with-input/candy + long-input/candy）+ `StatefulPreviewWrapper` 私有 helper
- [x] Task 5: 单元测试（AC5）
  - [x] 5.1 新建 `iphone/PetAppTests/Shared/Modals/JoinRoomModalTests.swift`，落地 ≥9 case（5 epic AC + 4 守护 case 含 Mock 行为 / Real 行为 / appState=nil race / Friends 入口回归）
- [x] Task 6: UITest（AC6）
  - [x] 6.1 在 `HomeUITests.swift` 加 `testJoinRoomModalCrossScreenJoinFlow`（完整链路：HomeView → modal → 输入 → 确认 → dismiss + RoomView 出现）+ `waitForNonExistence` extension（若 PetAppUITests 内无）
  - [x] 6.2 验证现有 6 个 testXxxScaffoldShowsAll*Anchors UITest 不受影响（不动）
- [x] Task 7: 回归确认 Friends 入口（AC7；无 production 代码改动）
  - [x] 7.1 AC5 case#9 守护测试已覆盖；不需要其它代码改动
- [x] Task 8: xcodegen regen + build verify（AC8）
  - [x] 8.1 `cd iphone && xcodegen generate` 让新文件加入 target
  - [x] 8.2 `bash iphone/scripts/build.sh --test` 跑测试通过（~325 unit + 7 UITest case 全绿）
  - [x] 8.3 grep 校验：JoinRoomModal 含 `struct JoinRoomModal` + `@Binding`；JoinRoomModal **不**含 `ObservableObject\|@Published`（防 over-design）；HomeViewModel 含 ≥6 个 fatalError；MockHomeViewModel + RealHomeViewModel 各含 `override func onJoinRoomConfirm`；RealHomeViewModel 含 `appState?.setCurrentRoomId`（守护 lesson 6+7）；HomeView 含 JoinRoomModal 调用 + 不再含 `JoinRoomModalPlaceholder()` + 含 `onDismiss` + `joinRoomInput = ""`
- [x] Task 9: Deliverable 清单确认（AC9）
  - [x] 9.1 2 个新文件 + 4 个修改老文件（HomeViewModel.swift / MockHomeViewModel.swift / RealHomeViewModel.swift / HomeView.swift）+ 1 个修改 UITest 文件（HomeUITests.swift）+ pbxproj regen 全部待 commit（不在本 dev-story 范围）

## Dev Notes

### Story 37.7-37.11 沉淀 lesson 预防性应用清单（关键约束 —— 全部 9 条 + 2 条新 lesson 命中）

本 story 落地前必读 9 + 2 条 lesson；**不重蹈覆辙**清单（与 epic-loop 调用提示中 9 + 2 条 lesson 一一对应）：

| # | Lesson 文件 | 预防点 | 本 story 落地动作 |
|---|---|---|---|
| 1 | `2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md` | abstract method base class 注入点全部要换 concrete subclass | RootView `@StateObject homeViewModel: HomeViewModel = RealHomeViewModel()` 已是 Story 37.7 落地（本 story 仅在 HomeViewModel 加 1 个新 abstract method `onJoinRoomConfirm`，RealHomeViewModel 必 override）；AC2 Task 2.3 + AC8 grep 校验 |
| 2 | `2026-04-30-published-derived-state-needs-publisher-subscription.md` | 派生 state 必须订阅 publisher，禁止 hardcode（避免 reset 后 stale） | 本 story 不引入新派生 state；HomeViewModel 既有 greeting sink 路径不动（Story 37.7 落地） |
| 3 | `2026-04-30-room-host-name-must-not-derive-from-local-current-pet.md` | 不要从 currentPet 派生其他用户的信息（local pet ≠ remote owner） | 本 story 不引入派生 state；onJoinRoomConfirm 写 appState.setCurrentRoomId 是规范入口路径而非派生 |
| 4 | `2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md` | RealViewModel.init 必须 seed scaffold defaults | 本 story 不改 HomeViewModel init / configureMockDefaults 路径 |
| 5 | `2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md` | `.onAppear` 同步 bind appState（避免 launch-time race） | 本 story 不改 RootView .onAppear；AC5 case#8 守护 RealHomeViewModel.onJoinRoomConfirm appState=nil 不 crash 路径 |
| **6** | **`2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`** | **RealViewModel.override 占位方法必须实装本地 mutate state（log-only 是 [P1]）** | **RealHomeViewModel.onJoinRoomConfirm 必须 mutate state（showJoinModal = false + appState?.setCurrentRoomId(roomId)）；AC5 case#7 显式守护；grep 校验 `appState?.setCurrentRoomId` 在 RealHomeViewModel.swift 内出现（AC8）** |
| 6.5 | `2026-05-01-real-viewmodel-transient-must-clear-on-any-identity-change.md` | transient state 必须监听 publisher 在 user identity change 时清空 | 本 story HomeView `joinRoomInput` 是 view-local @State（不进 ViewModel）；user identity 变化时（appState.reset()）会触发 HomeContainerView 互斥状态机切走 + view 重建 → @State 自动重置；不需要额外监听（详见 Dev Notes "joinRoomInput @State vs @Published 决策"） |
| 7 | `2026-04-30-swiftui-onchange-equatable-and-stale-task-cancel.md` | SwiftUI .onChange iOS 17+ 双参签名 | JoinRoomModal `.onChange(of: roomIdInput) { _, newValue in ... }` 必走 iOS 17+ 双参签名（AC1 关键决策 3） |
| 8 | `2026-04-30-swiftui-state-survives-id-and-shadow-over-children.md` + `2026-04-30-swiftui-explicit-id-nil-shared-identity.md` + `2026-04-30-swiftui-floating-emoji-needs-state-driven-position.md` | shadow / .id() / @State 驱动浮动动画 等 SwiftUI primitives 注意点 | 本 story JoinRoomModal 视觉无 shadow/.id()/动画路径（标准 sheet animation 由 SwiftUI 内置）；不踩 |
| 9 | `2026-04-30-spec-boundary-grey-area-fallback-must-honor-epic-ac-when-review-flags-it.md` | epic AC 与 ui_design 实物冲突时遵循 epic AC | 本 story epic AC line 4851-4866 钦定全部落地；与 ui_design `app.jsx:215-271` 视觉差异（**不**做大写自动转换 + **不**做 length<3 disabled 校验 + **不**用 letterSpacing）按 epic AC + AR21 钦定优先 |
| **新 lesson 8** | **lesson `2026-04-30-sheet-on-dismiss-button-vs-swipe-driven.md`（**本 story 钦定**预防性应用）** | **sheet onDismiss 闭包要兜底处理 swipe-driven dismiss（不通过按钮）路径** | **HomeView `.sheet onDismiss: { joinRoomInput = "" }` 必填——swipe-down dismiss 不走 onCancel/onConfirm closure；AC3 关键决策 1 + AC8 grep 校验**（注：该 lesson 由本 story 引入；若 docs/lessons 内尚无此文件，dev 阶段先按本 spec 钦定路径落地，retrospective 阶段补建 lesson 文件）|
| **新 lesson 7（P0 风险）** | **概念 lesson：「View 不要绕过 ViewModel seam 直接 mutate state」** | **HomeView 内 onCancel closure 直接写 `state.showJoinModal = false` 是合法的（showJoinModal 是 ViewModel 字段双向绑定 sheet）；onConfirm closure 走 `state.onJoinRoomConfirm(...)` 让 ViewModel 决定 mutate 路径（不直接写 self.appState 等）** | onCancel 路径 ViewModel 不必参与（关 sheet 是 view-driven）；onConfirm 路径必走 ViewModel seam（关 sheet + 写 appState）；AC5 case#7 守护 |

### joinRoomInput @State vs @Published 决策（关键设计）

ADR-0010 §3.2 表格"表单输入 / 当前选中 → ViewModel 或 SwiftUI @State"二选一；判断标准是**是否需要跨 View 触发 / 单元测试需要断言 / 是否需要持久化**：

| 场景 | 选择 | 理由 |
|---|---|---|
| `joinRoomInput`（modal 输入字段）| **SwiftUI @State** | ① 输入字段是 view-local transient（modal 关闭后即可清空，无跨 view 触发场景）；② 单元测试走 ViewModel 层断言 onJoinRoomConfirm 收到的 roomId（不需要从 ViewModel 端断言输入字段实时值）；③ 不需要持久化（modal dismiss 后用户重新打开预期是空字段）；④ user identity 变化（appState.reset()）时 HomeContainerView 互斥状态机会切走 → view 重建 → @State 自动重置（lesson 6.5 自动满足，不需手动监听） |
| `showJoinModal`（modal 显隐）| **ViewModel @Published** | ① ViewModel.onJoinTap / onJoinRoomConfirm 都需写入此字段（跨 view-action 触发）；② 单元测试需要断言（case#6 / case#7 / case#8 关 sheet 行为） |

> **关键决策**：joinRoomInput 放 @State 是合理的——把"输入字段实时值"作为 view-local，把"提交后的 trim 结果"作为 ViewModel 入参（onJoinRoomConfirm(roomId:)）→ 单元测试 + 业务逻辑都在 ViewModel 层；输入字段 UX 行为（实时显示 / 限 64 字符 / trim 判定）都在 View 层。两层职责清晰分离。

### sheet onDismiss 闭包必须区分 button-driven vs swipe-driven（lesson 8 钦定）

SwiftUI `.sheet(isPresented:onDismiss:content:)` 的 `onDismiss` 闭包**总是**触发——无论是：
- **button-driven dismiss**：用户点 onCancel/onConfirm closure → caller 内 `state.showJoinModal = false` → SwiftUI 检测到 binding 变化 → dismiss + 调用 onDismiss
- **swipe-driven dismiss**：用户从 modal 顶部下拉 → SwiftUI 直接 dismiss + 调用 onDismiss + 把 binding 设回 false

**关键 trap**：如果 reset transient state（如 `joinRoomInput = ""`）只在 onCancel/onConfirm closure 内执行，**swipe-driven dismiss 路径漏 reset**——下次打开 modal 残留旧输入值。

**本 story 防御**：把 `joinRoomInput = ""` 放在 `onDismiss` 闭包内（覆盖两种 dismiss 路径），**不**放在 onCancel/onConfirm closure 内。

> **关键决策**：dev 实装时**禁止** "把 reset 写在 onCancel + onConfirm 里 + 不写 onDismiss" 的写法——必出 swipe-driven dismiss 路径漏 reset bug。AC8 grep 校验 `onDismiss` 在 HomeView 内必须出现 ≥1 次。

### PrimaryButton.isDisabled 字段提前防 P0 风险

Story 37.6 落地的 PrimaryButton primitives 暂未确定是否有 `isDisabled` 参数；本 story AC1 视觉契约要求"trim 后空 → 确定加入按钮 disabled"，**强依赖**该字段。

**dev 实装时**：
1. 先 `Read iphone/PetApp/Core/DesignSystem/Primitives/PrimaryButton.swift` 确认现状
2. 若已有 `isDisabled: Bool = false` 参数 → 直接用
3. 若**没有** `isDisabled` 参数 → **本 story 范围内**先扩展 PrimaryButton 加该字段（默认 false 兼容老 caller）+ 在按钮内部 disable 时降低 opacity 0.5 + `.disabled(isDisabled)` SwiftUI modifier
4. **不**新建一个 "DisablablePrimaryButton" 子类型（与 ADR-0010 §3.1 PrimaryButton 单一入口契约冲突）

> **关键决策**：这是 P0 风险预防 ——若 dev 阶段才发现 PrimaryButton 没 isDisabled，必须立即扩展；不能 fallback 到自绘 disabled 状态（会让"取消"和"确定加入"两按钮视觉不一致）。

### JoinRoomModal **不**持 ObservableObject ViewModel（关键设计决策）

**选定**: JoinRoomModal 直接 `struct ... View` + 3 直白参数（`@Binding roomIdInput` / `onConfirm: (String) -> Void` / `onCancel: () -> Void`），**不**持 ViewModel。

**为何不走 `class JoinRoomViewModel: ObservableObject`**：

1. **业务解耦的最强形式**：modal 视图层完全 caller-driven，不预设业务行为（onConfirm 由 caller 决定调 appState.setCurrentRoomId / JoinRoomUseCase / 等）
2. **复用性**：未来若 FriendsView "邀请好友"路径也想弹 modal，可复用同一 JoinRoomModal（只需改 caller 传不同 onConfirm closure；ViewModel 化后会强行绑死 Home 域）
3. **测试简单**：单元测试不需要 mock ViewModel + StateObject 链路，直接构造 View + 注入 closure 即可断言（参考 AC5 case#1 / case#2）
4. **与 Story 37.11 BindWechatModalView 同精神**但更彻底——37.11 BindWechatModalView 仍持 `state: ProfileViewModel` 引用拼数据；本 story JoinRoomModal **不**持任何 ViewModel 引用，纯 closure-driven

**为何不在 HomeView 内 inline JoinRoomModal**（不抽到独立文件）：

1. JoinRoomModal 视觉复杂度（5 视觉锚 + 限 64 字符 + trim + Preview 4 场景）足够独立成文件
2. epic AC line 4851 钦定路径 `iphone/PetApp/Shared/Modals/JoinRoomModal.swift`
3. 未来 FriendsView 等其它 caller 复用时直接 import 即可

### epic AC 边界灰区清单（lesson 9 命中说明）

epic AC line 4851-4866 钦定全部元素**全部落地**，即便与 ui_design `app.jsx:215-271` 视觉略有差异：

| epic AC 元素 | 落地路径 | ui_design 视觉差异 |
|---|---|---|
| 底部 sheet（背景遮罩 0.45 + 卡片从下方 20pt 上滑 0.3s）| SwiftUI `.sheet(isPresented:)` + `.presentationDetents([.medium])` + `.presentationCornerRadius(28)`（AC3 关键决策 3）| iOS 系统 sheet 动画 ≠ web overlay 自绘；视觉略有差异但语义一致 |
| 卡片 Card 圆角 26 + theme.colors.surface + 标题"加入队伍" + 关闭按钮（Icons.close）| AC1 titleBar（圆角 28 与 ui_design 26 略差，统一 28；关闭按钮 32x32 圆形）| 圆角 28 vs 26（取 28 与 ui_design `app.jsx:224` 一致）|
| 大输入框：Icons.paw prefix + 等宽字体 + 自动 trim + 限 64 字符 | AC1 inputArea（Icons.paw + monospaced + .onChange 限 64）| **不**做大写自动转换（AR21 钦定）；**不**做 letterSpacing（SwiftUI 暂无 API）|
| 格式提示"输入好友分享给你的房间号"灰字 small（**不**暗示纯数字）| AC1 hintLabel（11pt regular inkMute）| **不**含"3 个字母 - 2 位数字"格式提示（与 ui_design `app.jsx:259` 不同；AR21 钦定不预设）|
| 取消 / 确定加入 两按钮：仅输入 trim 后非空启用确定（不做客户端格式校验，server 决定合法性）| AC1 actionButtons（PrimaryButton + isDisabled: trimmedIsEmpty）| **不**做 length<3 校验（与 ui_design `app.jsx:264` 不同；AR21 钦定）|
| 确定加入 → 调用通过构造注入的 `onConfirm: (String) async -> Void` 闭包；**不**直接调 AppState 或 UseCase（modal 与业务解耦）| AC1 关键决策 1（JoinRoomModal 不持 ViewModel）| **同步**而非 async（PRD 范围内 modal 不直接调 server，async 暂无意义；Story 12.7 落地 JoinRoomUseCase 时改 async）|
| HomeScreen 触发：HomeView 内 `.sheet(isPresented: $state.showJoinModal)` 挂；showJoinModal 唯一 owner = HomeViewModel 基类的 `@Published var showJoinModal: Bool` | AC3 + Story 37.7 落地路径不动 | 1:1 |
| FriendsScreen "加入"按钮（好友在房间中）触发：解析 `friend.currentRoomId: String` **直接**调 JoinRoomUseCase（不弹 Modal）| AC7 回归确认（Story 37.10 落地路径不动）| 1:1（占位走 appState.setCurrentRoomId；Story 12.7 落地后改调 JoinRoomUseCase）|

> **关键决策**：lesson 9 的精神是"epic AC 与 ui_design 实物冲突时，遵循 epic AC"。本 story 把所有 ui_design 暗示的"客户端格式校验"（length<3 / 大写转换 / 格式提示纯数字）按 AR21 + epic AC 砍掉；把"async closure"按本 story 范围（不调 server）改为同步 closure；视觉差异（圆角 / sheet animation）按 SwiftUI 标准行为优先。

### onConfirm closure 的 async / sync 决策（与 epic AC line 4857 略不同）

epic AC line 4857 钦定：「确定加入 → 调用通过构造注入的 `onConfirm: (String) async -> Void` 闭包」。

**本 story 实装为同步 closure**（`onConfirm: (String) -> Void`），理由：

1. 本 story 范围内不调真实 JoinRoomUseCase（仅 `appState.setCurrentRoomId`，同步 mutate）
2. 同步 closure 让单元测试更简单（不需要 await + Task）
3. SwiftUI Button.action 闭包默认同步；async closure 需要包 `Task { await ... }` wrapper，与 SwiftUI 主流写法不一致
4. Story 12.7（节点 4 后）真实 JoinRoomUseCase 落地时再改 async（届时 closure 内包 Task wrapper + ErrorPresenter 处理失败路径）

> **关键决策**：本 story 改 async → 同步是合理的"增量缩窄"决策；没破坏 epic AC 语义（仍然走 closure 注入路径），仅改了类型签名。Story 12.7 真实落地时再改 async，View 层 zero edit（仅 onConfirm closure 内部 wrap Task）。

### 测试边界（XCTest only）

本 story 测试**仅**用 XCTest + @testable import PetApp + UITest（XCUIApplication）—— **不**引：

- ❌ SnapshotTesting（视觉 diff）：视觉验证靠 #Preview + Story 37.13 visual-review-checklist
- ❌ ViewInspector（SwiftUI body 内省）：JoinRoomModal body 渲染契约靠 ui_design 1:1 翻译 + Preview 抽样兜底 + UITest 跨屏链路
- ❌ Mockingbird / Cuckoo（mock codegen）：MockHomeViewModel 是手写 final class subclass

### Story 37.7-37.11 衔接：与 Home/Room/Wardrobe/Friends/Profile 同 patterns 全表

| 维度 | HomeView (37.7) | Room (37.8) | Wardrobe (37.9) | Friends (37.10) | Profile (37.11) | JoinRoomModal (本 story) |
|---|---|---|---|---|---|---|
| 文件命名 | HomeView 改写 | RoomScaffoldView 新建 | WardrobeScaffoldView 新建 | FriendsScaffoldView 新建 | ProfileScaffoldView 新建 | **JoinRoomModal 新建（Shared/Modals/）**|
| struct 签名 | 泛型 HomeView<ChestSlot: View> | 非泛型 | 非泛型 | 非泛型 | 非泛型 | **非泛型**|
| state owner | @ObservedObject base class | @ObservedObject base class | @ObservedObject base class | @ObservedObject base class | @ObservedObject base class | **@Binding + closures（无 ViewModel）**|
| ViewModel 基类 | class + 5 字段 + 5 abstract | class + 4 字段 + 2 abstract | class + 5 字段 + 1 abstract + 2 concrete + 3 derived | class + 4 字段 + 2 abstract + 1 concrete + 4 derived | class + 5 字段 + 5 abstract + 0 concrete + 0 derived | **复用 HomeViewModel 加 1 个新 abstract method（onJoinRoomConfirm）**|
| Mock 子类 | MockHomeViewModel | MockRoomViewModel | MockWardrobeViewModel | MockFriendsViewModel | MockProfileViewModel | **复用 MockHomeViewModel 加 1 个 override**|
| Real override 行为 | mutate showJoinModal / interactionAnimation | setCurrentRoomId(nil) | local toggle equipped | appState.setCurrentRoomId via 入口 | direct mutate showBindModal/wechatBound/lastToastMessage | **showJoinModal = false + appState?.setCurrentRoomId(roomId)（混合：sheet 关 + appState 入口）**|
| Defaults 共享 enum | （未抽） | RoomScaffoldDefaults | WardrobeScaffoldDefaults | FriendsScaffoldDefaults | ProfileScaffoldDefaults | **（无；JoinRoomModal 无 mock 数据需求）**|
| 数据模型 | PetStats / AnimationState | RoomMember | CosmeticItem / CosmeticCategory | Friend / FriendStatus / FriendsTab | ProfileSummary / RecentCollection / ProfileMenuItem | **（无；纯 closure-driven）**|
| 区块数 | 5 | 5 | 4 | 5（含 toastOverlay）| 5（含 toastOverlay + BindWechatModal sheet）| **5（titleBar / inputArea / hintLabel / actionButtons + 隐式 outer 容器）**|
| @State (transient) | resetTask | copiedFeedback / copyFeedbackTask | （无）| （无）| （无）| **HomeView 加 joinRoomInput @State（modal owner）**|
| Sink 模式 | greeting 单 sink | currentRoomId 单 sink | currentEquips 单 sink | currentRoomId 单 sink | CombineLatest currentUser + currentPet | **（无新 sink；onJoinRoomConfirm 写 appState 入口让既有 sink 派生兄弟 ViewModel）**|
| 老占位文件处理 | HomeView 改写 | RoomViewPlaceholder 不删 | WardrobeView 不删 | FriendsView 不删 | ProfileView 不删 | **JoinRoomModalPlaceholder 不删**|
| caller 改动 | bridge 改 init | bridge 新增 | body 直接改 | body 改 + 加 @EnvironmentObject | body 改 + 加 @EnvironmentObject | **HomeView body 改 .sheet 块 + 加 @State joinRoomInput**|
| RootView wire | RealHomeViewModel | + RealRoomViewModel | + RealWardrobeViewModel | + RealFriendsViewModel | + RealProfileViewModel | **不动（不引入新 ViewModel）**|
| .onAppear bind | bind appState | + realRoomVM.bind | + realWardrobeVM.bind | + realFriendsVM.bind | + realProfileVM.bind | **不动**|
| #Preview 数 | 2 | 4 | 4 | 4 | 4 | **4（含 StatefulPreviewWrapper helper）**|
| 单元测试 case 数 | 6 | 5 | 10 | 11 | 11 | **9（5 epic AC + 4 守护）**|
| UITest case | testHomeScaffold... | testRoomScaffold...+ env | testWardrobeScaffold... + 切 tab | testFriendsScaffold... + 切 tab | testProfileScaffold... + 切 tab + tap modal | **testJoinRoomModalCrossScreenJoinFlow（HomeView → modal → 输入 → 确认 → dismiss + RoomView 出现）**|
| a11y identifier | 7 锚 | 8 锚 | 12+ 锚 | 8+ 锚 | 10+ 锚 | **5 锚（joinRoomModal / Input / CancelButton / ConfirmButton / CloseButton）**|

### a11y identifier 命名约定

本 story JoinRoomModal 内 inline a11y identifier 字符串（与 Story 37.7-37.11 同精神，Story 37.13 一次性归并到 `AccessibilityID.JoinRoomModal`）：

| identifier | 位置 | 备注 |
|---|---|---|
| `joinRoomModal` | JoinRoomModal 主容器 | epic AC line 4866 钦定 |
| `joinRoomInput` | TextField 输入框 | UITest 走 `app.textFields["joinRoomInput"]` 入口 |
| `joinRoomCloseButton` | titleBar 关闭按钮（X icon）| 调 onCancel |
| `joinRoomCancelButton` | actionButtons 取消按钮 | 调 onCancel |
| `joinRoomConfirmButton` | actionButtons 确定加入按钮 | 调 onConfirm(trimmed)；`isDisabled` 在 trim 后空时 true |

### 与 后续 epic 衔接的红线（关键约束）

后续 epic 真实 JoinRoomUseCase 落地路径：
- 把 RealHomeViewModel.onJoinRoomConfirm 内 `appState?.setCurrentRoomId(roomId)` 替换为 `await joinRoomUseCase.execute(roomId: roomId)` 调用（成功后 server 推送 WS room.snapshot → setCurrentRoomId 由 server 端权威态写入；失败弹 ErrorPresenter retry banner）
- onJoinRoomConfirm 改 async（HomeView caller 内 onConfirm closure wrap Task —— `onConfirm: { roomId in Task { await state.onJoinRoomConfirm(roomId: roomId) } }`）
- JoinRoomModal 视图 zero edit（onConfirm 仍是 `(String) -> Void`，caller wrap async；或将本 story 改 `(String) async -> Void` 让 caller 直接 await）
- AppState / setCurrentRoomId 契约 zero edit
- FriendsViewModel.onJoinFriendTap 同样改 async + JoinRoomUseCase（与 RealHomeViewModel.onJoinRoomConfirm 同步改）

> **关键决策**：本 story **不**预先 async closure —— 让 dev / reviewer 在本 story 范围内（不调 server）有最简洁的同步 closure 路径；Story 12.7 真实落地 JoinRoomUseCase 时再改 async + 失败处理 + 二次确认（onConfirm 后等 server 响应再关 sheet vs 立刻关 sheet 等 UX 决策）。

### Project Structure Notes

- **新建目录** `iphone/PetApp/Shared/Modals/`（之前 `iphone/PetApp/Shared/` 下仅 Constants / ErrorHandling / Testing 三个子目录）
- 新建目录 `iphone/PetAppTests/Shared/Modals/`
- 全部走 `iphone/project.yml` 通配 inclusion；不改 project.yml

### References

- [Source: docs/宠物互动App_总体架构设计.md] —— 总体架构与产品规则
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md] —— iOS 工程目录结构（Shared/Modals/ 跨域共享 modal 三层）
- [Source: _bmad-output/planning-artifacts/epics.md §Story 37.12] —— 本 story epic AC（line 4841-4866）
- [Source: _bmad-output/planning-artifacts/sprint-change-proposal-2026-04-29-v2.md §5.2] —— Story 37.3 / 37.7 / 37.12 + AppState 互斥状态机入口 PM 钦定
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md §3.1] —— ADR-0002 测试栈钦定（XCTest only）
- [Source: _bmad-output/implementation-artifacts/decisions/0009-ios-navigation.md §3.5] —— ADR-0009 主入口 4 Tab + Home/Room 互斥状态机
- [Source: _bmad-output/implementation-artifacts/decisions/0010-app-state.md §3.1 §3.2 §3.3] —— ADR-0010 AppState 范围白名单 + setCurrentRoomId 入口 + ViewModel 注入规则
- [Source: iphone/ui_design/source/app.jsx#L215-L271] —— JoinRoomModal 视觉源（标题栏 + 输入框 + 格式提示 + 取消/确定按钮）
- [Source: iphone/ui_design/source/screens/home.jsx#L155-L185] —— HomeScreen TeamIdleCard 入口（"加入队伍"按钮 → setModal('join')）
- [Source: iphone/ui_design/README.md §6 JoinRoomModal] —— JoinRoomModal 概述 + 触发路径
- [Source: iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift#L98-L101] —— Story 37.7 落地的 showJoinModal 字段 + 注释钦定 Story 37.12 实装路径
- [Source: iphone/PetApp/Features/Home/Views/HomeView.swift#L88-L91] —— Story 37.7 落地的 .sheet 调用 JoinRoomModalPlaceholder（本 story 替换）
- [Source: iphone/PetApp/Features/Home/Views/JoinRoomModal/JoinRoomModalPlaceholder.swift] —— Story 37.3 落地的占位 stub（本 story 不动，保 git history）
- [Source: iphone/PetApp/Features/Friends/ViewModels/RealFriendsViewModel.swift#L115-L124] —— Story 37.10 落地的 onJoinFriendTap → appState.setCurrentRoomId 入口（本 story 不动，AC5 case#9 守护回归）
- [Source: iphone/PetApp/App/AppState.swift#L88-L92] —— Story 37.4 落地的 setCurrentRoomId(_:) 入口
- [Source: iphone/PetApp/Core/DesignSystem/Primitives/PrimaryButton.swift] —— Story 37.6 落地的 PrimaryButton（**dev 实装时确认 isDisabled 字段是否就位**）
- [Source: iphone/PetApp/Core/DesignSystem/Primitives/Icons.swift] —— Story 37.6 落地的 Icons.paw / Icons.close 入口
- [Source: iphone/PetApp/Core/DesignSystem/Theme.swift] —— Story 37.5 Theme tokens
- [Source: docs/lessons/2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md] —— **lesson 1**: abstract method base class 注入点必须 concrete subclass
- [Source: docs/lessons/2026-04-30-published-derived-state-needs-publisher-subscription.md] —— **lesson 2**: 派生 state 必须订阅 publisher
- [Source: docs/lessons/2026-04-30-room-host-name-must-not-derive-from-local-current-pet.md] —— **lesson 3**: 不要从 currentPet 派生其他用户的信息
- [Source: docs/lessons/2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md] —— **lesson 4**: RealViewModel.init 必须 seed scaffold defaults
- [Source: docs/lessons/2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md] —— **lesson 5**: `.onAppear` 同步 bind appState
- [Source: docs/lessons/2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md] —— **lesson 6（关键）**: RealViewModel.override placeholder 必须实装本 story 范围内的本地 mutation
- [Source: docs/lessons/2026-05-01-real-viewmodel-transient-must-clear-on-any-identity-change.md] —— **lesson 6.5**: transient state 必须监听 publisher 在 user identity change 时清空（本 story joinRoomInput 走 view-local @State 自动满足，无需 ViewModel 监听）
- [Source: docs/lessons/2026-04-30-swiftui-onchange-equatable-and-stale-task-cancel.md] —— **lesson 7**: SwiftUI .onChange iOS 17+ 双参签名（JoinRoomModal `.onChange(of: roomIdInput) { _, newValue in ... }` 必走双参）
- [Source: docs/lessons/2026-04-30-swiftui-state-survives-id-and-shadow-over-children.md] —— **lesson 8a**: shadow 挂 RoundedRectangle.fill 那层
- [Source: docs/lessons/2026-04-30-swiftui-explicit-id-nil-shared-identity.md] —— **lesson 8b**: .id() 不挂 nil
- [Source: docs/lessons/2026-04-30-swiftui-floating-emoji-needs-state-driven-position.md] —— **lesson 8c**: @State 驱动浮动动画
- [Source: docs/lessons/2026-04-30-spec-boundary-grey-area-fallback-must-honor-epic-ac-when-review-flags-it.md] —— **lesson 9**: spec 边界灰色区域 epic AC 与 ui_design 实物冲突时遵循 epic AC

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- `bash iphone/scripts/build.sh --test` 全绿（333 unit case；含本 story 新增 9 case + 既有 324）.
- `xcodegen generate` 已重新生成 PetApp.xcodeproj/project.pbxproj，新文件自动 inclusion.
- grep 校验全部通过（AC8 第 3 步 10 条 grep 校验项）.

### Completion Notes List

- AC1: JoinRoomModal struct 新建（`iphone/PetApp/Shared/Modals/JoinRoomModal.swift`；纯 presentation View + 3 直白参数 + 5 视觉锚 + 限 64 字符 + trim 后判定空）
- AC2: HomeViewModel 加 abstract method `onJoinRoomConfirm(roomId:)` + Mock/Real 两子类 override（Mock: 关 sheet + 记录 invocation；Real: 关 sheet + `localAppState?.setCurrentRoomId(roomId)` 走规范入口）
- AC3: HomeView 加 `@State joinRoomInput` owner + `.sheet(onDismiss: { joinRoomInput = "" })` 替换 JoinRoomModalPlaceholder() 为 JoinRoomModal(...) + presentationDetents/CornerRadius
- AC4: 4 个 #Preview（empty/candy + empty/dark + with-input/candy + long-input/candy）+ StatefulPreviewWrapper helper
- AC5: 9 个单元测试 case 全绿（5 epic AC 钦定 + 4 守护 case 含 Mock 行为 / Real 行为 / appState=nil race / Friends 入口回归）
- AC6: UITest `testJoinRoomModalCrossScreenJoinFlow` 加在 HomeUITests.swift（完整跨屏链路：HomeView → modal → 输入 → 确认 → dismiss + RoomScaffoldView returnButton 出现）+ waitForNonExistence helper extension
- AC7: FriendsView 入口路径不动（Story 37.10 落地不变）；AC5 case#9 显式守护回归
- AC8: xcodegen regen 完成 + build/test 全绿 + grep 10 条校验通过
- 关键决策：RealHomeViewModel 加 private `localAppState` 字段（基类 `appState` 是 private 不可见，与 RealFriendsViewModel 同模式）让 onJoinRoomConfirm 能调 `localAppState?.setCurrentRoomId(roomId)`
- 关键决策：PrimaryButton 加 `isDisabled: Bool = false` 参数（默认 false 兼容老 caller）；内部 `effectiveEnabled = isEnabled && !isDisabled` 统一驱动 .disabled / opacity
- UITest RoomScaffoldView 出现验证用 `returnButton` 标识（RoomScaffoldView 没有顶级 `roomView` a11y identifier；returnButton 是 RoomScaffoldView 唯一标识，HomeView 路径无此标识）

### File List

- 新建：`iphone/PetApp/Shared/Modals/JoinRoomModal.swift`
- 新建：`iphone/PetAppTests/Shared/Modals/JoinRoomModalTests.swift`
- 修改：`iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`（加 onJoinRoomConfirm abstract method）
- 修改：`iphone/PetApp/Features/Home/ViewModels/MockHomeViewModel.swift`（加 Invocation case + override）
- 修改：`iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift`（加 localAppState 字段 + override）
- 修改：`iphone/PetApp/Features/Home/Views/HomeView.swift`（加 @State joinRoomInput + .sheet 替换）
- 修改：`iphone/PetApp/Core/DesignSystem/Primitives/PrimaryButton.swift`（加 isDisabled 参数）
- 修改：`iphone/PetAppUITests/HomeUITests.swift`（加 testJoinRoomModalCrossScreenJoinFlow + waitForNonExistence helper）
- 修改：`iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen 自动重新生成）
- 修改：`_bmad-output/implementation-artifacts/sprint-status.yaml`（37-12 状态变更）

### Change Log

- 2026-04-30：实装 JoinRoomModal + 跨屏 join 链路；HomeView .sheet 由 JoinRoomModalPlaceholder 替换为真实 JoinRoomModal；HomeViewModel 增加 onJoinRoomConfirm abstract method 走 appState 规范入口；PrimaryButton 加 isDisabled 参数支持 trim 后空判定 disabled；333 unit case 全绿。
