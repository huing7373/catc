// HomeView.swift
// Story 2.2 主界面骨架：6 大占位区块
//   ① 用户昵称 + 头像位（顶部）
//   ② 猫展示区（中间，屏幕中心区）
//   ③ 步数显示位（中间下方）
//   ④ 宝箱位（中间右侧）
//   ⑤ 三个主按钮（底部）：进入房间 / 仓库 / 合成
//   ⑥ 版本号小字（右下角）
//
// 后续 story 范围红线：
// - 不实装 Sheet / 路由（→ Story 2.3）
// - 不实装 APIClient 调用（→ Story 2.4 / 2.5；版本号目前 hardcode）
// - 不实装错误 UI（→ Story 2.6）
//
// Story 5.2 codex round 1 [P1] fix：userInfoBar 接 SessionStore 订阅 nickname
// （详见 docs/lessons/2026-04-27-sessionstore-home-nickname-source-of-truth.md）。
// 原方案：bootstrapStep1 把登录返回写入 sessionStore，但 HomeView 仍渲染 HomeViewModel.nickname
// 默认值 "用户1001" —— 持久化 guestUid 映射到不同 user.id 时显示错误身份。
// 修复方案：HomeView 接受 optional sessionStore，nickname 在 session 非 nil 时优先取 session.user.nickname
// （fallback 到 viewModel.nickname 保持 Preview / 老测试 / UITest skip-guest-login 路径零回归）。
// 不动 HomeViewModel 内部状态：避免把 SessionStore 耦合进 ViewModel 触发 Story 5.5 LoadHomeUseCase 重构。

import SwiftUI

public struct HomeView: View {
    @ObservedObject public var viewModel: HomeViewModel

    // Story 2.8: optional dev "重置身份" 按钮的 ViewModel（仅在 Debug build 由 RootView 注入）。
    // 用 plain `let` 持有 — `@ObservedObject` 不接受 Optional；ResetIdentityButton 自己内部
    // 持 `@ObservedObject viewModel`，订阅由 button 子 view 完成，HomeView 只做引用透传。
    // 默认 nil 让 Story 2.2 / 2.5 既有 `HomeView(viewModel:)` 调用零改动；旧测试 / Preview 兼容。
    private let resetIdentityViewModel: ResetIdentityViewModel?

    /// Story 5.2 codex round 1 [P1] fix：optional SessionStore，nickname 显示来源。
    /// nil 时 fallback 到 viewModel.nickname（Preview / 老测试 / UITest skip-guest-login 路径）。
    /// 非 nil 时通过 `SessionAwareUserInfoBar` 子视图 `@ObservedObject` 订阅；session 写入后 SwiftUI 重渲染。
    /// 与 `resetIdentityViewModel` 同模式（HomeView 仅持引用，订阅由子视图完成）。
    private let sessionStore: SessionStore?

    public init(viewModel: HomeViewModel) {
        self.viewModel = viewModel
        self.resetIdentityViewModel = nil
        self.sessionStore = nil
    }

    public init(viewModel: HomeViewModel, resetIdentityViewModel: ResetIdentityViewModel?) {
        self.viewModel = viewModel
        self.resetIdentityViewModel = resetIdentityViewModel
        self.sessionStore = nil
    }

    /// Story 5.2 新增 init：携带 SessionStore（生产路径 RootView 调用）。
    /// resetIdentityViewModel 仍可选（仅 Debug 注入）；sessionStore 显式区分新旧路径让调用点明确。
    public init(
        viewModel: HomeViewModel,
        resetIdentityViewModel: ResetIdentityViewModel?,
        sessionStore: SessionStore?
    ) {
        self.viewModel = viewModel
        self.resetIdentityViewModel = resetIdentityViewModel
        self.sessionStore = sessionStore
    }

    public var body: some View {
        // 单一 VStack：所有 6 区块都参与同一纵向布局，避免 ZStack overlay 在小屏上覆盖底部 CTA。
        // 版本号 (⑥) 作为最后一个子视图占据 footer 行，靠右对齐；它与 bottomButtonRow (⑤) 在
        // 垂直方向严格分隔，不会再遮挡或截获"合成"按钮的点击。
        VStack(spacing: 16) {
            userInfoBar
            Spacer()
            petAndChestRow
            stepBalanceLabel
            Spacer()
            bottomButtonRow
            versionFooter
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 24)
    }

    // MARK: - ① 用户昵称 + 头像位

    /// Story 5.2 codex round 1 [P1] fix：根据 sessionStore 是否注入分发到不同子视图。
    /// 两条路径返回的视觉 / a11y 结构完全一致；唯一差别是 nickname 来源（session vs viewModel）。
    /// 抽子视图原因：`@ObservedObject` 不支持 Optional；要订阅 SessionStore.@Published session
    /// 必须在持有该字段的 view 内部用 `@ObservedObject`。
    /// 与 ResetIdentityButton 同模式（HomeView 持引用、子视图订阅）。
    @ViewBuilder
    private var userInfoBar: some View {
        if let sessionStore = sessionStore {
            SessionAwareUserInfoBar(
                sessionStore: sessionStore,
                fallbackNickname: viewModel.nickname,
                resetIdentityViewModel: resetIdentityViewModel
            )
        } else {
            StaticUserInfoBar(
                nickname: viewModel.nickname,
                resetIdentityViewModel: resetIdentityViewModel
            )
        }
    }

    // MARK: - ② 猫展示区 + ④ 宝箱位（横向同一行：中间 + 中间右侧）

    private var petAndChestRow: some View {
        HStack(alignment: .center, spacing: 16) {
            Spacer()
            petColumn
            chestColumn
            Spacer()
        }
    }

    /// Story 5.5 AC7: petArea 下方追加 pet 名称 Text.
    /// 不渲染真实 sprite —— 节点 5 / Epic 8 才接.
    ///
    /// **三态文案语义**（Story 5.5 codex round 1 [P2] fix）:
    /// 1. homeData == nil（首屏未加载完）→ "默认小猫" 占位
    /// 2. homeData != nil && pet == nil（V1 §5.1 schema 明确允许：首次注册 / Reset 后）→ "暂无宠物"
    /// 3. pet != nil → 渲染 pet.name
    ///
    /// 关键：状态 2 必须有独立文案,**不**能 fallback 回 "默认小猫"——会让 server 明确说"无宠物"
    /// 的账号显示成"已有宠物"且名字是占位串,误导用户/掩盖 bug.
    /// 详见 docs/lessons/2026-04-27-optional-domain-field-vs-loading-placeholder.md.
    private var petColumn: some View {
        VStack(spacing: 8) {
            petArea
            Text(petNameDisplay)
                .font(.caption)
                .accessibilityIdentifier(AccessibilityID.Home.petName)
        }
    }

    /// pet 名称显示决策（与 `HomeNicknameResolver` 同精神：抽纯函数 helper 让单测可独立锁住语义）.
    /// 三态分支对应 petColumn 文案语义注释.
    private var petNameDisplay: String {
        HomePetNameResolver.resolve(homeData: viewModel.homeData)
    }

    /// Story 5.5 AC7: chestArea 上方追加倒计时 Text（占位 "--:--" 当 homeData 为 nil）.
    /// 静态显示 server 返回的 remainingSeconds，不起本地 timer 动态倒计时（节点 7 / Story 21.2 才接）.
    private var chestColumn: some View {
        VStack(spacing: 8) {
            Text(viewModel.homeData?.chestRemainingDisplay ?? "--:--")
                .font(.caption)
                .accessibilityIdentifier(AccessibilityID.Home.chestRemaining)
            chestArea
        }
    }

    private var petArea: some View {
        Rectangle()
            .fill(Color.gray)
            .frame(width: 200, height: 200)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(Text("猫展示区"))
            .accessibilityIdentifier(AccessibilityID.Home.petArea)
    }

    private var chestArea: some View {
        Rectangle()
            .fill(Color.brown)
            .frame(width: 64, height: 64)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(Text("宝箱"))
            .accessibilityIdentifier(AccessibilityID.Home.chestArea)
    }

    // MARK: - ③ 步数显示位

    /// Story 5.5 AC7: 步数显示从 hardcode "0 步" 升级为读 viewModel.homeData?.stepAccount.availableSteps.
    /// homeData 为 nil 时显示 "0 步"（保 Preview / UITest skip-guest-login 路径）.
    private var stepBalanceLabel: some View {
        Text("\(viewModel.homeData?.stepAccount.availableSteps ?? 0) 步")
            .accessibilityIdentifier(AccessibilityID.Home.stepBalance)
    }

    // MARK: - ⑤ 三个主按钮

    private var bottomButtonRow: some View {
        HStack(spacing: 16) {
            Button("进入房间") {
                viewModel.onRoomTap()
            }
            .accessibilityIdentifier(AccessibilityID.Home.btnRoom)

            Button("仓库") {
                viewModel.onInventoryTap()
            }
            .accessibilityIdentifier(AccessibilityID.Home.btnInventory)

            Button("合成") {
                viewModel.onComposeTap()
            }
            .accessibilityIdentifier(AccessibilityID.Home.btnCompose)
        }
    }

    // MARK: - ⑥ 版本号小字（footer 行，靠右）

    private var versionFooter: some View {
        HStack {
            Spacer()
            versionLabel
        }
    }

    private var versionLabel: some View {
        Text("v\(viewModel.appVersion) · \(viewModel.serverInfo)")
            .font(.caption)
            .foregroundStyle(.secondary)
            .accessibilityIdentifier(AccessibilityID.Home.versionLabel)
    }
}

// MARK: - userInfoBar 子视图（Story 5.2 codex round 1 [P1] fix）

/// 订阅 `SessionStore.@Published session` 的 userInfoBar 渲染.
/// session 非 nil → 显示 `session.user.nickname`（真实持久化身份）.
/// session = nil → 显示 `fallbackNickname`（HomeViewModel.nickname；启动早期 / Reset 后状态）.
///
/// 为何抽子视图：`@ObservedObject` 不接受 Optional，HomeView 持有的是 `SessionStore?`；
/// 子视图签名收紧为非 Optional `SessionStore`，HomeView 在 ViewBuilder 内分发，
/// 符合 SwiftUI 的"以子视图边界承担订阅生命周期"惯用法.
private struct SessionAwareUserInfoBar: View {
    @ObservedObject var sessionStore: SessionStore
    let fallbackNickname: String
    let resetIdentityViewModel: ResetIdentityViewModel?

    var body: some View {
        UserInfoBarLayout(
            nickname: HomeNicknameResolver.resolve(
                session: sessionStore.session,
                fallback: fallbackNickname
            ),
            resetIdentityViewModel: resetIdentityViewModel
        )
    }
}

/// 不订阅 SessionStore 的 userInfoBar（Preview / 老测试 / UITest skip-guest-login 路径）.
/// 直接显示 viewModel 透传过来的 nickname.
private struct StaticUserInfoBar: View {
    let nickname: String
    let resetIdentityViewModel: ResetIdentityViewModel?

    var body: some View {
        UserInfoBarLayout(
            nickname: nickname,
            resetIdentityViewModel: resetIdentityViewModel
        )
    }
}

/// userInfoBar 的视觉 / a11y 布局（共享）.
/// 抽 layout 而不是直接重复：保证 SessionAware / Static 两条路径的视觉结构完全一致，
/// 修任一条 a11y / spacing 时不会出现两边漂移.
private struct UserInfoBarLayout: View {
    let nickname: String
    let resetIdentityViewModel: ResetIdentityViewModel?

    var body: some View {
        HStack(spacing: 8) {
            Text(nickname)
            Circle()
                .fill(Color.gray)
                .frame(width: 32, height: 32)
            Spacer()
            #if DEBUG
            if let resetIdentityViewModel = resetIdentityViewModel {
                ResetIdentityButton(viewModel: resetIdentityViewModel)
            }
            #endif
        }
        // Story 2.8: children 从 .ignore 改为 .contain —— 让按钮的 a11y identifier 可被 XCUITest 独立定位；
        // 父容器 a11y identifier 仍存在（既有 testHomeViewShowsAllSixPlaceholders 仍可定位 home_userInfo）。
        // 父级 .accessibilityLabel(nickname) 与 .contain 并存：VoiceOver 仍能读到 nickname summary，
        // 子元素（ResetIdentityButton）也仍可被 a11y 树独立访问。
        // lesson 2026-04-26 SwiftUI 父容器 a11y identifier 默认传播覆盖子元素 / a11y contain + label 兼容。
        .accessibilityElement(children: .contain)
        .accessibilityLabel(Text(nickname))
        .accessibilityIdentifier(AccessibilityID.Home.userInfo)
    }
}

/// nickname 显示决策的纯函数 helper，方便单元测试覆盖（fileprivate 的子视图 `body` 难直接断言；
/// 把决策逻辑抽出来后用纯输入/输出 case 锁住"session 优先 → fallback 兜底"语义）.
///
/// 单一职责：`UserProfile.nickname` 优先；无 session 时 fallback 到 viewModel 默认值.
/// 当未来扩展（如 nickname 编辑、降级显示等）时，新规则集中在此处修改.
public enum HomeNicknameResolver {
    /// 决定 userInfoBar 应显示哪个 nickname.
    /// - Parameters:
    ///   - session: 当前 SessionStore.session（nil 表示未登录 / 启动早期）.
    ///   - fallback: viewModel 透传的默认 nickname（如 "用户1001"）.
    /// - Returns: 渲染用 nickname：session 非 nil 时返回 session.user.nickname；否则返回 fallback.
    public static func resolve(session: SessionState?, fallback: String) -> String {
        session?.user.nickname ?? fallback
    }
}

/// pet 名称显示决策（Story 5.5 codex round 1 [P2] fix）.
///
/// 区分"未加载完"与"已加载但 server 返回 pet=null"两种语义,前者是占位 placeholder,
/// 后者是 V1 §5.1 schema 明确允许的"账号无宠物"状态(首次注册 / Reset 后).
///
/// 抽纯函数 helper 同 `HomeNicknameResolver` 的精神：把决策逻辑抽出来后用纯输入/输出 case
/// 锁住"loading vs no-pet vs has-pet"三态语义,fileprivate 子视图 body 难直接断言.
public enum HomePetNameResolver {
    /// loading 期占位文案（homeData == nil）.
    public static let loadingPlaceholder = "默认小猫"

    /// server 明确返回 pet=null 时的文案（homeData != nil && pet == nil）.
    /// V1 §5.1 schema 允许 pet: null —— 首次注册或 Reset 后的合法状态.
    public static let noPetPlaceholder = "暂无宠物"

    /// 决定 petColumn 应显示哪个名称.
    /// - Parameter homeData: 当前 ViewModel.homeData（nil 表示首屏未加载完）.
    /// - Returns:
    ///   - homeData == nil → loadingPlaceholder（"默认小猫"）
    ///   - homeData != nil && pet == nil → noPetPlaceholder（"暂无宠物"）
    ///   - pet != nil → pet.name
    public static func resolve(homeData: HomeData?) -> String {
        guard let homeData = homeData else {
            return loadingPlaceholder
        }
        guard let pet = homeData.pet else {
            return noPetPlaceholder
        }
        return pet.name
    }
}

#if DEBUG
struct HomeView_Previews: PreviewProvider {
    static var previews: some View {
        HomeView(viewModel: HomeViewModel())
    }
}
#endif
