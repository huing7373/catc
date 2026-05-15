# Story 21.1: 首页宝箱组件 SwiftUI（倒计时 Timer + 状态切换 UI）

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want 主界面有宝箱图标 + 倒计时数字显示（counting 态），倒计时结束后自动切到可开启视觉态（unlockable 态），
so that 我能看到宝箱的进度并知道何时可以开启。

## 故事定位（Epic 21 第 1 条 story；首页宝箱 UI 接缝填实 + 节点 7 iOS 前置）

这是 Epic 21「iOS - 首页宝箱倒计时 + 奖励弹窗」的**起点 story**——本 story 实装 `ChestCardView`（统一组件含 counting / unlockable 两态视觉），通过 Story 37.7 落地的 `HomeView<ChestSlot: View, PetSlot: View>.chestSlot` ViewBuilder closure 接缝注入到首页，**HomeView 内部代码 zero edit**。下游 Story 21.2（GET /chest/current 调用）/ 21.3（开箱按钮调用 POST /chest/open）/ 21.4（奖励弹窗）/ 21.5（开箱前同步步数）依赖本 story 落地的 ChestCardView 视觉契约 + HomeViewModel.chestRemainingSeconds 倒计时驱动机制。

**本 story 范围（一句话）**：
1. 新建 `ChestCardView` SwiftUI 组件（独立 View 文件，按 `currentChest.status` 派生 counting / unlockable 两态视觉，按 `HomeViewModel.chestRemainingSeconds` 显示 `mm:ss` 倒计时）；
2. 新建 `ChestTimerDriver`（@MainActor `Task.sleep` 驱动的本地倒计时；订阅 `AppState.currentChest` 变化重启 timer）；
3. `HomeViewModel` 基类追加 `@Published public var chestRemainingSeconds: Int = 0`（与 Story 37.7 落地的 `petState` 同模式：基类持字段，Real/Mock 子类共享）；
4. 在 `HomeContainerHomeViewBridge.body` 内把 `chestSlot: { EmptyView() }` 替换为 `chestSlot: { ChestCardView(currentChest: appState.currentChest, remainingSeconds: homeViewModel.chestRemainingSeconds, onOpenTap: { /* Story 21.3 接 */ }) }`；
5. 新建单元测试 + UITest（≥4 case + UITest 验证 `chestCard_counting` / `chestCard_unlockable` a11y identifier）。

**本 story 落地后立即解锁**：
- Story 21.2（GET /chest/current 调用 + 主动定时纠正）—— LoadChestUseCase 拉到 server 状态后写入 `appState.currentChest`，本 story 落地的 ChestTimerDriver 自动 react、ChestCardView 自动重新渲染；
- Story 21.3（开箱按钮 + POST /chest/open）—— `onOpenTap` closure 替换为 OpenChestUseCase 调用；本 story `onOpenTap` 占位 `{ }` 闭包；
- Story 21.4（奖励弹窗 popup）—— `RewardPopupView` 通过 `.sheet` 挂在 HomeView 或 ChestCardView 内（**注**：sheet binding owner 由 Story 21.4 决定；本 story 不预判 sheet 位置）；
- Story 21.5（开箱前同步步数）—— OpenChestUseCase 内组合 SyncStepsUseCase 先后调用顺序；本 story 不涉及。

**关键路径（与 Story 37.7 / 8.4 同精神，"接缝期 → 接缝填实"）**：

- Story 37.7 HomeView 通过 `chestSlot: () -> ChestSlot` ViewBuilder closure 留出接缝（chestSlot 接缝期 caller 传 `EmptyView()`）；
- 本 story `HomeContainerHomeViewBridge` 调用方改传 `ChestCardView(...)`，**HomeView 内部 body 一行不改**（接缝硬契约兑现）；
- ChestCardView 是独立 View（不嵌入 HomeView body 内部 inline）；ChestCardView 不持有 ObservableObject，纯输入参数渲染（与 Story 8.4 落地的 `PetSpriteView` 同模式：value-type props + 视图组装层契约锁定）。

**不涉及**（红线）：

- **不**实装 GET /chest/current 调用（Story 21.2 落地）；本 story `currentChest` 数据完全来自 `AppState.currentChest`（已由 Story 4.8 GET /home 拉到、Story 37.4 hydrate 到 AppState）；
- **不**实装 POST /chest/open 调用（Story 21.3 落地）；本 story `onOpenTap` closure 占位为空 `{ }`（unlockable 态按钮可点但点击无副作用）；
- **不**实装奖励弹窗 RewardPopupView（Story 21.4 落地）；
- **不**实装开箱前同步步数（Story 21.5 落地）；
- **不**改 `AppState` 字段 / 行为（ADR-0010 §3.2 白名单已含 `currentChest: HomeChest?`；本 story 仅消费）；
- **不**改 `HomeView` body 任何一行代码（zero edit 接缝硬契约）；只改 `HomeContainerHomeViewBridge.body` 内 chestSlot closure；
- **不**改 RootView wire（line 34 / 268 `@StateObject homeViewModel = HomeViewModel()` 路径不动）；
- **不**改 server 任何文件（端独立原则）；
- **不**改 ios/ 任何文件（CLAUDE.md + ADR-0002 §3.3）；
- **不**引 SnapshotTesting / ViewInspector（ADR-0002 §3.1 钦定 XCTest only）；视觉精度由 #Preview + Story 37.13 visual-review-checklist + ios-simulator MCP UI 验证兜底；
- **不**做 chestArea / chestRemaining 旧 AccessibilityID.Home 常量"复活"（Story 37.7 落地后这两个常量保留但本 story 不渲染时不挂；本 story 渲染 ChestCardView 后挂 `AccessibilityID.Home.chestArea` 在 ChestCardView 根容器 + `AccessibilityID.Home.chestRemaining` 在倒计时 Text；详见 AC3 + AC8）；
- **不**预先做 RewardPopup / OpenChestUseCase / SyncStepsUseCase 任何脚手架（Story 21.3-21.5 各自落地，保 story 边界清晰）；
- **不**修改 Story 37.7 落地的 5 个 abstract method 签名（onCreateTap / onJoinTap / onFeedTap / onPetTap / onPlayTap）；
- **不**新增 abstract method（chestRemainingSeconds 是 `@Published` 字段而非方法）。

## Acceptance Criteria

> **AC 编号体系**：AC1 = HomeViewModel 基类追加 chestRemainingSeconds 字段；AC2 = ChestTimerDriver 倒计时驱动；AC3 = ChestCardView SwiftUI 组件；AC4 = HomeContainerHomeViewBridge chestSlot 改造；AC5 = RealHomeViewModel bind ChestTimerDriver；AC6 = #Preview 双主题 + 双状态；AC7 = 单元测试 ≥4 case；AC8 = UITest a11y identifier；AC9 = xcodegen + build verify；AC10 = ios-simulator MCP UI 实跑验证；AC11 = Deliverable 清单。

### AC1 — HomeViewModel 基类追加 `chestRemainingSeconds` @Published 字段（与 petState 同模式）

**改动文件**：`iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`

**关键改动**：在基类已有 `@Published public var petState: MotionState = .rest`（Story 8.4 落地）的同区域追加：

```swift
/// Story 21.1 AC1: 本地倒计时 view-state（Timer 驱动；driver 在 ChestTimerDriver / RealHomeViewModel 落地）.
///
/// 单位：秒；初值 0（domain 状态未 hydrate 时不显示倒计时）.
///
/// 为何放基类而非子类：
/// - 与 Story 37.7 落地的 `petState`（Story 8.4 由 Story 8.4 AC1 加在基类）+ 5 abstract method 同模式：
///   基类持字段、Mock/Real 子类共享读取路径，ChestCardView 通过 HomeView<ChestSlot, PetSlot>.chestSlot
///   注入时既能 hardcode mock 数据（MockHomeViewModel 测试 / Preview）也能由 Real 子类 driver 驱动.
/// - 该字段是 view-state 而非 domain state（ADR-0010 §3.2 表格"倒计时秒数 → ViewModel"钦定，不上 AppState）；
///   domain 端权威是 `AppState.currentChest.unlockAt`（绝对时间），本字段是 Timer 派生的相对秒数.
///
/// 用法（Story 21.1 / 21.2 演进）：
/// - 本 story（21.1）：RealHomeViewModel.init 内构造 ChestTimerDriver + 启动；driver 订阅 appState.currentChest
///   变化重新计算并写入 `self.chestRemainingSeconds`；ChestCardView 直接读取展示 mm:ss.
/// - Story 21.2：LoadChestUseCase 拉到 server 状态后写 `appState.currentChest`；本字段由 driver 自动 react.
@Published public var chestRemainingSeconds: Int = 0
```

> **关键决策 1（基类持字段 vs 子类持字段）**：基类持。理由：与 `petState` 同模式（Story 8.4 AC1 已开此先例）；Mock 子类需要 hardcode 值跑 Preview / UITest skip-guest-login 路径；Real 子类需要 driver 写入。基类持字段后 ChestCardView 用 `HomeViewModel` 基类引用即可（不需要 `RealHomeViewModel` 具体类型），caller 端 type 不膨胀。

> **关键决策 2（@Published vs 普通字段）**：@Published。理由：SwiftUI 视图层需要 react 字段变化重新渲染倒计时（每秒变化）；与基类已有的 `petState` / `interactionAnimation` / `showJoinModal` 等字段保持一致风格。

> **关键决策 3（字段初值 0 vs Optional<Int>）**：初值 0。理由：`Int` 比 `Int?` 简洁，0 在 ChestCardView 内已表示"无倒计时显示" / "可开启" 视觉态（与 `currentChest.status == .unlockable` 或 `currentChest == nil` 联合判断）；避免 Swift Optional 链式判空。

**对应 Tasks**: Task 1.1

### AC2 — 新建 `ChestTimerDriver`（订阅 AppState.currentChest 重启 timer + 每秒写 chestRemainingSeconds）

**新建文件**：`iphone/PetApp/Features/Home/ViewModels/ChestTimerDriver.swift`

**职责**（与 Story 8.4 落地的 MotionState 派生 source 同精神：独立 driver 不污染 ViewModel）：

```swift
// ChestTimerDriver.swift
// Story 21.1 AC2: 本地宝箱倒计时驱动（订阅 AppState.currentChest → 每秒写 HomeViewModel.chestRemainingSeconds）.
//
// 设计：
//   - 弱引用 HomeViewModel（防循环；与 Story 8.4 MotionStateMapper 同模式）.
//   - 弱引用 AppState（防循环；ADR-0010 §3.1 ViewModel 注入 AppState 是 strong，driver 是 weak）.
//   - 订阅 appState.$currentChest（Combine sink）→ 每次变化 cancel 老 Task + 启动新 Task.
//   - 倒计时 Task 内 @MainActor `while !Task.isCancelled` 循环每秒 sleep 1s + 计算 remainingSeconds 写回.
//   - 倒计时来源：`max(0, Int(currentChest.unlockAt.timeIntervalSince(Date())))`（绝对时间 → 相对秒数）；
//     这样既不依赖 server 给的 remainingSeconds 初始值（避免双源不一致），也保 timer drift 自校准.
//   - 倒计时到 0 时不停 Task（等待 currentChest 再次变化）—— Story 21.2 落地后 LoadChestUseCase 60s 定时拉取
//     会把 currentChest.status 切到 unlockable + 新 unlockAt，driver 自然 react；本 story 阶段倒计时到 0 后
//     视图通过 status / remainingSeconds 派生切到 unlockable 视觉态.
//
// 红线：
//   - 不调用任何 UseCase / Repository（driver 纯 view-state 派生）.
//   - 不依赖 SwiftUI（纯 ObservableObject + Combine + Foundation）.
//   - 不暴露 public 字段（driver 是 ViewModel 内部 helper；唯一对外 API 是 init + start + stop）.

import Foundation
import Combine

@MainActor
public final class ChestTimerDriver {
    private weak var appState: AppState?
    private weak var viewModel: HomeViewModel?
    private var subscription: AnyCancellable?
    private var tickTask: Task<Void, Never>?

    public init(appState: AppState, viewModel: HomeViewModel) {
        self.appState = appState
        self.viewModel = viewModel
    }

    /// 启动 driver：订阅 appState.$currentChest，首次启动立即用当前值跑一次 recompute.
    public func start() {
        guard subscription == nil else { return }  // 防双启
        subscription = appState?.$currentChest
            .receive(on: DispatchQueue.main)
            .sink { [weak self] newChest in
                self?.handleChestChange(newChest)
            }
    }

    /// 停止 driver（dealloc 时调；测试可显式调以验证 Task 取消）.
    public func stop() {
        subscription?.cancel()
        subscription = nil
        tickTask?.cancel()
        tickTask = nil
    }

    private func handleChestChange(_ chest: HomeChest?) {
        // 老 timer 必停（防 ABA：旧 chest unlockAt + 新 chest unlockAt 两 Task 并存写 chestRemainingSeconds）.
        tickTask?.cancel()

        guard let chest else {
            // currentChest 为 nil（未 hydrate 或被清空）→ remainingSeconds = 0 + 不启 Task.
            viewModel?.chestRemainingSeconds = 0
            return
        }

        // 首次/重启：立即算一次 + 启 Task 每秒 tick.
        recomputeAndWrite(unlockAt: chest.unlockAt)
        tickTask = Task { @MainActor [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 1_000_000_000)
                guard !Task.isCancelled else { return }
                guard let self else { return }
                // 防 ABA：tickTask 启动后 currentChest 被换掉，sink 已 cancel 老 task；这里再 guard 一次 chest 仍是同一个.
                guard self.appState?.currentChest?.id == chest.id else { return }
                self.recomputeAndWrite(unlockAt: chest.unlockAt)
            }
        }
    }

    private func recomputeAndWrite(unlockAt: Date) {
        let remaining = max(0, Int(unlockAt.timeIntervalSince(Date())))
        viewModel?.chestRemainingSeconds = remaining
    }
}
```

> **关键决策 1（driver 是独立 class 而非 ViewModel 内部 method）**：独立 class。理由：与 Story 8.4 落地的 `MotionStateMapper` / `HomePetNameResolver` 同精神（行为 helper 抽出来让 XCTest 直接覆盖，不通过 SwiftUI body 内省）；driver 内部 Combine subscription + Task lifecycle 较复杂，单测要能注入 fake AppState + fake clock 独立验证。

> **关键决策 2（倒计时来源 absolute time 而非 server remainingSeconds）**：absolute time（`unlockAt.timeIntervalSince(Date())`）。理由：(a) `HomeChest` 已有 `unlockAt: Date` 字段（GET /home 已下发，Story 4.8 落地）；(b) server `remainingSeconds` 字段是 server 算时刻的快照，本地驱动用 absolute time 自校准 timer drift；(c) Story 21.2 校准逻辑（"server / 本地差距 > 5s 校准"）落地后仍走 unlockAt（server 推新 unlockAt → driver 重新计算）。

> **关键决策 3（订阅 `appState.$currentChest` 而非传 viewModel 自己 currentChest 字段）**：订阅 AppState。理由：ADR-0010 §3.1 钦定 domain state 单 source of truth 是 AppState；driver 直接 sink Combine publisher，避免 ViewModel 做中转字段。

> **关键决策 4（不在倒计时到 0 时切 status 字段）**：driver 只写 `chestRemainingSeconds`，**不**改 `currentChest.status`。理由：domain state 由 server 权威；倒计时归零仅触发 view 层视觉派生（ChestCardView 按 `remainingSeconds == 0` 切 unlockable 视觉），server 端实际 status 切换由 Story 21.2 LoadChestUseCase 60s 定时拉取兜底。这样保 ViewModel 不污染 domain state。

> **关键决策 5（Task.sleep 1s 而非 Timer.publish）**：Task.sleep。理由：Combine `Timer.publish` 涉及 RunLoop 调度，测试时较难注入 fake clock；`Task.sleep` 配合 `Task.cancel` 生命周期清晰；与 Story 18.4 FloatingEmojiCellView `Task.sleep` 路径一致；测试通过 driver.stop() 验证 Task 取消、通过 `appState.currentChest = newChest` 触发 sink 验证 remainingSeconds 更新（不依赖等待真实秒数过去）。

**对应 Tasks**: Task 2.1

### AC3 — 新建 `ChestCardView` SwiftUI 组件（counting / unlockable 双态视觉）

**新建文件**：`iphone/PetApp/Features/Home/Views/ChestCardView.swift`

**结构契约**：

```swift
// ChestCardView.swift
// Story 21.1 AC3: 首页宝箱组件（counting 倒计时 / unlockable 可开启 双态视觉，按 currentChest.status 派生）.
//
// 设计：
//   - 纯 value-type props 输入：currentChest: HomeChest? + remainingSeconds: Int + onOpenTap: () -> Void.
//   - 不持有 ObservableObject（与 Story 8.4 落地的 PetSpriteView 同模式：视图组装层契约锁定，
//     业务态由 caller 派生后传入；本 view 仅按 props 渲染）.
//   - currentChest == nil → 渲染 EmptyView()（防 hydrate 前白屏抖动；与 Story 5.5 loading 三态语义一致）.
//   - currentChest.status == .counting → 灰色锁定图标 + mm:ss 倒计时 + "倒计时" 标签.
//   - currentChest.status == .unlockable 或 remainingSeconds == 0 → 金色高亮图标 + "可开启" 标签 + 开箱按钮.
//
// 视觉规则（无现成 ui_design 钦定 chest 区块；本 story 按 home.jsx 同风格自洽落地）：
//   - 容器：`Card(cornerRadius: theme.radius.cardLg, padding: 20)` (22pt 圆角；与 teamIdleCard 同档)；
//     theme.colors.surface 背景 + theme.shadow.md 中阴影 + 1pt theme.colors.border.
//   - counting 态背景：theme.colors.surface（中性）；unlockable 态背景：theme.colors.accentSoft（高亮）.
//   - 图标：SF Symbol "shippingbox.fill"（与 Icons.mapping["box"] 一致），48pt size；counting 态 inkSoft；
//     unlockable 态 coin 色（金色；ThemeColors 已有 coin token）.
//   - 倒计时文字：theme.typography.cardTitle（17pt heavy）；counting 态 ink 色.
//   - "可开启" 标签：theme.typography.cardTitle 17pt heavy，coin 色；下方 PrimaryButton "开宝箱" variant: .primary.

import SwiftUI

public struct ChestCardView: View {
    @Environment(\.theme) private var theme

    private let currentChest: HomeChest?
    private let remainingSeconds: Int
    private let onOpenTap: () -> Void

    public init(
        currentChest: HomeChest?,
        remainingSeconds: Int,
        onOpenTap: @escaping () -> Void
    ) {
        self.currentChest = currentChest
        self.remainingSeconds = remainingSeconds
        self.onOpenTap = onOpenTap
    }

    public var body: some View {
        // currentChest 为 nil（hydrate 前）→ 不渲染（防白屏抖动 + 让 5 区块布局自洽）.
        if let chest = currentChest {
            content(for: chest)
        } else {
            EmptyView()
        }
    }

    @ViewBuilder
    private func content(for chest: HomeChest) -> some View {
        // 视觉派生：domain status + 本地 remainingSeconds 联合判定（remainingSeconds == 0 触发本地视觉 unlockable 预切）.
        let isUnlockable = (chest.status == .unlockable) || (remainingSeconds <= 0)
        if isUnlockable {
            unlockableView
        } else {
            countingView(remainingSeconds: remainingSeconds)
        }
    }

    private var unlockableView: some View {
        Card(cornerRadius: theme.radius.cardLg, padding: 20) {
            HStack(spacing: 14) {
                Image(systemName: Icons.symbol(for: "box"))
                    .font(.system(size: 36, weight: .heavy))
                    .foregroundColor(theme.colors.coin)
                VStack(alignment: .leading, spacing: 6) {
                    Text("宝箱已就绪")
                        .font(.system(size: 17, weight: .heavy))
                        .foregroundColor(theme.colors.ink)
                    Text("可开启")
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundColor(theme.colors.inkSoft)
                }
                Spacer()
                PrimaryButton(title: "开宝箱", variant: .primary, action: onOpenTap)
                    .frame(width: 96)
                    .accessibilityIdentifier(AccessibilityID.Home.chestOpenButton)
            }
        }
        .accessibilityIdentifier("chestCard_unlockable")
        .accessibilityElement(children: .contain)
    }

    private func countingView(remainingSeconds: Int) -> some View {
        Card(cornerRadius: theme.radius.cardLg, padding: 20) {
            HStack(spacing: 14) {
                Image(systemName: Icons.symbol(for: "box"))
                    .font(.system(size: 36, weight: .heavy))
                    .foregroundColor(theme.colors.inkSoft)
                VStack(alignment: .leading, spacing: 6) {
                    Text("宝箱倒计时")
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundColor(theme.colors.inkSoft)
                    Text(formatMMSS(remainingSeconds))
                        .font(.system(size: 22, weight: .heavy, design: .rounded))
                        .foregroundColor(theme.colors.ink)
                        .accessibilityIdentifier(AccessibilityID.Home.chestRemaining)
                }
                Spacer()
            }
        }
        .accessibilityIdentifier("chestCard_counting")
        .accessibilityElement(children: .contain)
    }

    private func formatMMSS(_ totalSeconds: Int) -> String {
        let safe = max(0, totalSeconds)
        let minutes = safe / 60
        let seconds = safe % 60
        return String(format: "%02d:%02d", minutes, seconds)
    }
}

#if DEBUG
#Preview("ChestCardView — counting · candy") {
    ChestCardView(
        currentChest: HomeChest(
            id: "c1",
            status: .counting,
            unlockAt: Date().addingTimeInterval(300),
            openCostSteps: 1000,
            remainingSeconds: 300
        ),
        remainingSeconds: 300,
        onOpenTap: {}
    )
    .padding()
    .environment(\.theme, ThemeName.candy.theme)
}

#Preview("ChestCardView — unlockable · candy") {
    ChestCardView(
        currentChest: HomeChest(
            id: "c1",
            status: .unlockable,
            unlockAt: Date(),
            openCostSteps: 1000,
            remainingSeconds: 0
        ),
        remainingSeconds: 0,
        onOpenTap: {}
    )
    .padding()
    .environment(\.theme, ThemeName.candy.theme)
}

#Preview("ChestCardView — counting · dark") {
    ChestCardView(
        currentChest: HomeChest(
            id: "c1",
            status: .counting,
            unlockAt: Date().addingTimeInterval(300),
            openCostSteps: 1000,
            remainingSeconds: 300
        ),
        remainingSeconds: 300,
        onOpenTap: {}
    )
    .padding()
    .environment(\.theme, ThemeName.dark.theme)
}

#Preview("ChestCardView — unlockable · dark") {
    ChestCardView(
        currentChest: HomeChest(
            id: "c1",
            status: .unlockable,
            unlockAt: Date(),
            openCostSteps: 1000,
            remainingSeconds: 0
        ),
        remainingSeconds: 0,
        onOpenTap: {}
    )
    .padding()
    .environment(\.theme, ThemeName.dark.theme)
}
#endif
```

> **关键决策 1（ChestCardView 不持 ObservableObject）**：纯 value-type props。理由：与 Story 8.4 PetSpriteView 同模式；caller (HomeContainerHomeViewBridge) 已订阅 appState + homeViewModel，把派生 props 传进来；本 view 单一职责（按 props 渲染）；测试通过构造不同 props 直接覆盖各视觉态。

> **关键决策 2（双态视觉派生条件）**：`isUnlockable = (chest.status == .unlockable) || (remainingSeconds <= 0)`。理由：(a) 倒计时归零的瞬间 server 还没切 status（Story 21.2 60s 定时拉取兜底），本地 remainingSeconds == 0 即立即切视觉态（满足 epic AC line 3021 "倒计时归零时**自动切到 unlockable 视觉状态**"）；(b) 即使 remainingSeconds 因 driver 异常未归零、status 已是 unlockable，仍按 status 切（保 server-truth）。

> **关键决策 3（currentChest == nil 渲染 EmptyView()）**：直接 EmptyView。理由：hydrate 前白屏抖动比"空 Card 占位"更不打扰用户；HomeView VStack 5 区块本就用 spacing 隔开，EmptyView 不破坏布局；Story 21.2 / 21.3 不依赖 chestCard 渲染存在；hydrate 后立即出现，与 Story 37.4 `HomeContainerView` 互斥状态机的 transition 风格一致。

> **关键决策 4（开箱按钮在本 story 是占位 closure）**：`onOpenTap: () -> Void` 接 `{}` 空闭包。理由：epic AC line 3019 钦定 unlockable 态"显示开箱按钮"——本 story 必须**渲染**按钮并保 a11y identifier 存在（让 Story 21.3 落地时复用同位置 + 同 a11y identifier）；按钮当前可点但点击无副作用，避免破坏 UI；Story 21.3 改 closure 内容为 `OpenChestUseCase().execute(...)` 调用。

> **关键决策 5（a11y identifier 命名）**：根容器 `chestCard_counting` / `chestCard_unlockable` 字面量（epic AC line 3029 钦定）；倒计时 Text 挂 `AccessibilityID.Home.chestRemaining`（老常量 = "home_chestRemaining"，Story 5.5 落地保留至今），开箱按钮挂新加常量 `AccessibilityID.Home.chestOpenButton` = "home_chestOpenButton"。Story 37.13 a11y 总表归并时把 inline 字符串 `chestCard_counting` / `chestCard_unlockable` 收编进 `AccessibilityID.Home`（本 story 不预收口，遵循 Story 37.7 / 37.8 / 37.13 同节奏）。

**对应 Tasks**: Task 3.1, 3.2, 3.3

### AC4 — `HomeContainerHomeViewBridge.body` 内 chestSlot closure 替换为 ChestCardView

**改动文件**：`iphone/PetApp/Features/Home/Views/HomeContainerView.swift`（line 80-97 区域）

**关键改动**：

```swift
// 旧（Story 37.7 / 8.4 落地后）
HomeView(
    state: homeViewModel,
    resetIdentityViewModel: resetIdentityViewModel,
    sessionStore: sessionStore,
    petSlot: { PetSpriteView(state: homeViewModel.petState) },
    chestSlot: { EmptyView() }
)

// 新（Story 21.1 落地）
HomeView(
    state: homeViewModel,
    resetIdentityViewModel: resetIdentityViewModel,
    sessionStore: sessionStore,
    petSlot: { PetSpriteView(state: homeViewModel.petState) },
    chestSlot: {
        ChestCardView(
            currentChest: appState.currentChest,
            remainingSeconds: homeViewModel.chestRemainingSeconds,
            onOpenTap: {
                // Story 21.3 落地：替换为 OpenChestUseCase().execute(...).
                // 本 story 占位空闭包（按钮可点但无副作用）.
            }
        )
    }
)
```

> **关键决策（appState 字段读取）**：HomeContainerHomeViewBridge 内已有 `@EnvironmentObject var appState: AppState`？—— 当前文件签名只有 `homeViewModel` / `resetIdentityViewModel` / `sessionStore` 三注入；本 story 需追加 `@EnvironmentObject var appState: AppState`（与 HomeContainerView line 30 同模式；ADR-0010 §3.1 钦定 SwiftUI View 通过 EnvironmentObject 注入 AppState）。HomeContainerView 上层已 `.environmentObject(appState)`（RootView 注入路径），Bridge 子视图自动继承。**注意**：appState 在 HomeContainerHomeViewBridge 内的字段声明必须新增（当前文件只在外层 HomeContainerView 声明）；详见 Dev Notes "HomeContainerHomeViewBridge appState 注入"。

**对应 Tasks**: Task 4.1

### AC5 — `RealHomeViewModel` 内创建并启动 `ChestTimerDriver`

**改动文件**：`iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift`

**关键改动**：

```swift
@MainActor
public final class RealHomeViewModel: HomeViewModel {
    private let injectedAppState: AppState

    // Story 8.4: motion source
    private let motionSource: PetMotionStateMapper?

    // Story 21.1 AC5: 新增 chest timer driver（启动在 init 末尾）.
    private var chestTimerDriver: ChestTimerDriver?

    public init(
        appState: AppState,
        motionSource: PetMotionStateMapper? = nil
    ) {
        self.injectedAppState = appState
        self.motionSource = motionSource
        super.init(appState: appState)
        // 视觉初值（保 Story 37.7 既有路径不变）
        self.greeting = "想你啦 ♥"
        self.weather = "今天 · 晴"
        self.stats = .mockHappy
        self.interactionAnimation = .idle
        self.showJoinModal = false
        // Story 8.4 motion source 绑定（保 Story 8.4 既有路径不变）
        motionSource?.bind(viewModel: self)

        // Story 21.1 AC5: 创建并启动 chest timer driver.
        let driver = ChestTimerDriver(appState: appState, viewModel: self)
        driver.start()
        self.chestTimerDriver = driver
    }

    // ... 其余 override 方法不变 ...
}
```

> **关键决策 1（driver 由 RealHomeViewModel 持有 vs HomeContainerView 持有）**：RealHomeViewModel 持有。理由：(a) driver 生命周期与 ViewModel 一致（ViewModel dealloc 时 driver 自动 dealloc → subscription / Task 自动 cancel）；(b) MockHomeViewModel 不需要 driver（mock 直接 hardcode chestRemainingSeconds 值；测试和 Preview 走纯 view 路径）；(c) 与 Story 8.4 motion source 注入路径同精神（生产 RealHomeViewModel 注入真实 source，Mock 不注入）。

> **关键决策 2（MockHomeViewModel 是否需要 driver）**：不需要。理由：Mock 子类已通过 `super.init(...)` 设置 chestRemainingSeconds = 0；Preview / UITest 直接给 hardcode 值（如 `mockVM.chestRemainingSeconds = 180`）即可；不需要真实 timer 驱动。

> **关键决策 3（driver 字段 `var` vs `let`）**：`var Optional`。理由：init 阶段必须先 `super.init` 完才能传 `self` 给 ChestTimerDriver（Swift `self` 在 super.init 前不可用），所以无法在字段声明处直接初始化为 non-Optional `let`；用 `var` Optional + init 末尾赋值。

**对应 Tasks**: Task 5.1

### AC6 — ChestCardView #Preview 双主题 + 双状态（共 4 个 Preview）

ChestCardView 文件底部 `#if DEBUG ... #endif` 块含 4 个 Preview（详见 AC3 视觉契约内代码块）：

- `ChestCardView — counting · candy`
- `ChestCardView — unlockable · candy`
- `ChestCardView — counting · dark`
- `ChestCardView — unlockable · dark`

> **关键决策**：用 `#Preview` macro（与 Story 37.5 / 37.6 / 37.7 同模式）；双主题 × 双状态 = 4 Preview，让 Xcode Canvas 一次性看完两态在两主题下的视觉。

**对应 Tasks**: Task 6.1

### AC7 — 单元测试 ≥4 case（纯 XCTest + value-type props + fake AppState）

**新建文件**：`iphone/PetAppTests/Features/Home/ChestCardViewTests.swift`

落地以下 6 case（≥4 case 按 epic AC；与 Story 37.7 / 8.4 单测风格一致：构造层契约锁定 + ViewModel 行为断言；不走 SwiftUI body 内省）：

```swift
// ChestCardViewTests.swift
// Story 21.1 AC7: ChestCardView + ChestTimerDriver + HomeViewModel.chestRemainingSeconds 单元测试.
//
// 约束（与 Story 37.7 / 8.4 衔接）：
//   - 仅 XCTest + @testable import PetApp；不引 ViewInspector / SnapshotTesting.
//   - 不走 SwiftUI body 内省；走 props / ViewModel 字段 / Driver 行为断言.
//   - 视觉契约（counting / unlockable 颜色 / 图标）由 #Preview + Story 37.13 visual-review-checklist + AC10 ios-simulator MCP 验证.

import XCTest
@testable import PetApp

@MainActor
final class ChestCardViewTests: XCTestCase {

    // MARK: - case#1 happy: counting 态 props 构造合法（视觉断言由 Preview/MCP 兜底）

    /// 验证 ChestCardView 用 counting + remainingSeconds=300 构造不 crash + props 暴露符合 spec.
    /// 视觉断言（mm:ss 渲染 "05:00"）由 Preview / MCP 兜底；本测试断言 init 路径完整.
    func testChestCardViewConstructsWithCountingProps() {
        let chest = HomeChest(
            id: "c1",
            status: .counting,
            unlockAt: Date().addingTimeInterval(300),
            openCostSteps: 1000,
            remainingSeconds: 300
        )
        // 构造合法即不 crash（ChestCardView 是 struct，构造层契约锁定与 PetSpriteView 同模式）.
        _ = ChestCardView(currentChest: chest, remainingSeconds: 300, onOpenTap: {})
        // formatter 行为断言（间接验证 mm:ss 派生路径）.
        XCTAssertEqual(ChestCardView.formatMMSSForTesting(300), "05:00")
        XCTAssertEqual(ChestCardView.formatMMSSForTesting(65), "01:05")
        XCTAssertEqual(ChestCardView.formatMMSSForTesting(0), "00:00")
        XCTAssertEqual(ChestCardView.formatMMSSForTesting(-1), "00:00")   // 负值钳到 0
    }

    // MARK: - case#2 happy: HomeViewModel.chestRemainingSeconds 默认 0

    /// 验证 HomeViewModel 基类 chestRemainingSeconds 默认值 0（Story 21.1 AC1 钦定）.
    func testHomeViewModelChestRemainingSecondsDefaultsToZero() {
        let vm = HomeViewModel(
            nickname: "test",
            appVersion: "0.0.0",
            serverInfo: "test"
        )
        XCTAssertEqual(vm.chestRemainingSeconds, 0)
    }

    // MARK: - case#3 happy: ChestTimerDriver appState.currentChest 切换时 viewModel.chestRemainingSeconds 更新

    /// 验证 driver 订阅 appState.$currentChest，currentChest 变化触发立即重算 + 写 viewModel.chestRemainingSeconds.
    /// 不等待真实秒数过去；通过 appState.currentChest = newValue 触发 Combine sink 路径.
    func testChestTimerDriverUpdatesRemainingSecondsOnChestChange() async {
        let appState = AppState()
        let vm = HomeViewModel(nickname: "t", appVersion: "0", serverInfo: "t")
        let driver = ChestTimerDriver(appState: appState, viewModel: vm)
        driver.start()
        // 设 unlockAt 在未来 300 秒 → driver 立即重算 → chestRemainingSeconds 应为 ~300（容差 1 秒）.
        let unlockAt = Date().addingTimeInterval(300)
        appState.currentChest = HomeChest(
            id: "c1",
            status: .counting,
            unlockAt: unlockAt,
            openCostSteps: 1000,
            remainingSeconds: 300
        )
        // 等 Combine sink 在 main run loop 派发完成（runUntilTickleScheduled 模式）.
        try? await Task.sleep(nanoseconds: 50_000_000)   // 50ms 让 sink 跑完
        XCTAssertGreaterThanOrEqual(vm.chestRemainingSeconds, 299)
        XCTAssertLessThanOrEqual(vm.chestRemainingSeconds, 300)
        driver.stop()
    }

    // MARK: - case#4 happy: ChestTimerDriver appState.currentChest = nil 时 chestRemainingSeconds 归零

    /// 验证 currentChest 被清空（如登出 / reset）→ driver 立即写 chestRemainingSeconds = 0 + 不启 Task.
    func testChestTimerDriverWritesZeroWhenChestNiled() async {
        let appState = AppState()
        let vm = HomeViewModel(nickname: "t", appVersion: "0", serverInfo: "t")
        let driver = ChestTimerDriver(appState: appState, viewModel: vm)
        driver.start()
        appState.currentChest = HomeChest(
            id: "c1",
            status: .counting,
            unlockAt: Date().addingTimeInterval(60),
            openCostSteps: 1000,
            remainingSeconds: 60
        )
        try? await Task.sleep(nanoseconds: 50_000_000)
        XCTAssertGreaterThan(vm.chestRemainingSeconds, 0)
        // 清空 → driver 应立即写 0.
        appState.currentChest = nil
        try? await Task.sleep(nanoseconds: 50_000_000)
        XCTAssertEqual(vm.chestRemainingSeconds, 0)
        driver.stop()
    }

    // MARK: - case#5 happy: unlockable 视觉派生（status=unlockable 或 remainingSeconds<=0）

    /// 验证 ChestCardView 内部 isUnlockable 视觉派生条件正确（通过测 formatter 兼带；视觉断言由 Preview 兜底）.
    /// 关键不变量：(chest.status == .unlockable) || (remainingSeconds <= 0) 任一为真即 unlockable 态.
    /// 通过 helper 暴露派生函数供测试断言.
    func testChestCardViewUnlockableDerivation() {
        XCTAssertTrue(ChestCardView.isUnlockableForTesting(status: .unlockable, remainingSeconds: 100))
        XCTAssertTrue(ChestCardView.isUnlockableForTesting(status: .counting, remainingSeconds: 0))
        XCTAssertTrue(ChestCardView.isUnlockableForTesting(status: .counting, remainingSeconds: -5))
        XCTAssertFalse(ChestCardView.isUnlockableForTesting(status: .counting, remainingSeconds: 60))
    }

    // MARK: - case#6 happy: RealHomeViewModel 构造时 ChestTimerDriver 自动启动 + chestRemainingSeconds 从 appState 派生

    /// 验证 RealHomeViewModel 构造时 driver 自动创建并启动；appState.currentChest 已 hydrate 时立即拉到 remainingSeconds.
    func testRealHomeViewModelStartsDriverOnInit() async {
        let appState = AppState()
        appState.currentChest = HomeChest(
            id: "c1",
            status: .counting,
            unlockAt: Date().addingTimeInterval(120),
            openCostSteps: 1000,
            remainingSeconds: 120
        )
        let vm = RealHomeViewModel(appState: appState)
        try? await Task.sleep(nanoseconds: 50_000_000)
        XCTAssertGreaterThanOrEqual(vm.chestRemainingSeconds, 119)
        XCTAssertLessThanOrEqual(vm.chestRemainingSeconds, 120)
    }
}
```

> **关键决策 1（暴露 testing helpers `formatMMSSForTesting` / `isUnlockableForTesting`）**：在 ChestCardView 内加两个 `internal static func` 让测试可直接调用 formatter 和派生函数（`#if DEBUG` 包裹或 `internal` 可见性）。理由：ChestCardView body 内的 `private func formatMMSS` / inline `let isUnlockable` 表达式不可直接测；与 Story 8.4 PetSpriteView 内 helper 同模式（视图组装层契约锁定）。

> **关键决策 2（不用 fake clock / 不等真实秒数过去）**：测试 case#3 / #4 / #6 通过 `appState.currentChest = ...` 触发 Combine sink 路径，让 driver 立即 recompute。理由：(a) 与 Story 8.4 MotionStateMapper 测试同精神（fake 输入 → 立即断言输出）；(b) 不引 Combine `TestScheduler` / fake clock（ADR-0002 §3.1 钦定 XCTest only + 不引第三方测试库）；(c) 用 `Task.sleep(50_000_000)` 让 main run loop 跑完 sink dispatch 是社区惯例（与 18.4 / 15.x 测试一致）。

> **关键决策 3（测试 case 数量 6）**：epic AC line 3024-3029 钦定 ≥4 case；本 story 落地 6 case（多 2 case 覆盖 driver 清空路径 + RealHomeViewModel init 链路），不再加 ChestCardView body snapshot test（ADR-0002 §3.1 禁用 ViewInspector / SnapshotTesting；视觉由 #Preview + AC10 MCP 兜底）。

**对应 Tasks**: Task 7.1

### AC8 — UITest a11y identifier 可定位 (`chestCard_counting` / `chestCard_unlockable`)

**改动文件**：`iphone/PetAppUITests/HomeUITests.swift`

加 1 个新 UITest case（沿用 Story 37.7 testHomeScaffoldShowsAllSevenAnchors 模式）：

```swift
// Story 21.1 AC8: ChestCardView counting / unlockable 两态 a11y identifier 可定位.
//
// 路径：UITEST_SKIP_GUEST_LOGIN 启动 → AppState hydrate mock chest（counting 态）→ 验证 chestCard_counting 存在；
//   切换 chest 为 unlockable → 验证 chestCard_unlockable 存在.
//
// 注：本 case 仅验证 a11y 锚存在，不验证开箱按钮可点 / 不触发 server 调用（属 Story 21.3 / 22.1 范围）.
func testChestCardShowsCountingAndUnlockableAnchors() throws {
    let app = XCUIApplication()
    app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
    // 新加 launch env：让 UITest skip-guest-login 路径下 AppState 注入预设 mock chest（counting 态 5min 倒计时）.
    app.launchEnvironment["UITEST_CHEST_COUNTING"] = "1"
    app.launch()

    // counting 态 ChestCardView 出现.
    XCTAssertTrue(app.otherElements["chestCard_counting"].waitForExistence(timeout: 5))
    XCTAssertTrue(app.staticTexts[AccessibilityID.Home.chestRemaining].exists)
}
```

> **关键决策 1（UITest case 数量 1）**：1 case 验证 counting 态 a11y 锚即可，unlockable 态切换需要更复杂的 launchEnvironment 注入路径（涉及 mock RealHomeViewModel 切 chest.status，超出 Story 21.1 范围；Story 22.1 E2E 测试覆盖此切换）。本 story 接受单 case 覆盖。

> **关键决策 2（UITEST_CHEST_COUNTING launch env）**：新加 launch env 让 UITest skip-guest-login 路径下 AppState.currentChest 注入 mock counting 态宝箱（unlockAt = now + 5min）。实现路径在 `RootView.bootstrap` 内 `if ProcessInfo.processInfo.environment["UITEST_CHEST_COUNTING"] == "1" { appState.currentChest = ... }`（与 UITEST_SKIP_GUEST_LOGIN 同模式）。**注意**：dev 实装时需要在 RootView 或 AppState 测试 helper 内加这个 hook；详见 Dev Notes "UITEST launchEnvironment 扩展"。

**对应 Tasks**: Task 8.1, 8.2

### AC9 — xcodegen regen + build verify

完成 AC1-AC8 后：

1. `cd iphone && xcodegen generate` 让新文件加入 PetApp / PetAppTests target（project.yml 通配规则自动 inclusion；新增 `Features/Home/ViewModels/ChestTimerDriver.swift` + `Features/Home/Views/ChestCardView.swift` + `PetAppTests/Features/Home/ChestCardViewTests.swift` 共 3 个新文件）；
2. `bash iphone/scripts/build.sh --test` 全绿；
3. grep 校验抽样：
   - `grep -c "chestRemainingSeconds" iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift` ≥ 1（基类字段就位）
   - `grep -c "ChestTimerDriver" iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift` ≥ 2（创建 + 启动）
   - `grep -c "ChestCardView(" iphone/PetApp/Features/Home/Views/HomeContainerView.swift` ≥ 1（chestSlot closure 接入）
   - `grep "chestSlot: { EmptyView() }" iphone/PetApp/Features/Home/Views/HomeContainerView.swift` 输出空（不再用 EmptyView 占位）

> **dev 实装备注**：dev 必须 `xcodegen generate` 后 commit 一并提交 `iphone/PetApp.xcodeproj/project.pbxproj` 改动（与 Story 37.5 / 37.6 / 37.7 / 8.4 同模式）。

**对应 Tasks**: Task 9.1, 9.2, 9.3

### AC10 — ios-simulator MCP 实跑视觉 + 交互验证（必跑，CLAUDE.md 红线）

按 CLAUDE.md `## iOS UI 验证（必跑）` 钦定路径，**xcodebuild --test 通过不代表 feature 通过**——dev 必须用 ios-simulator MCP 在模拟器实跑验证：

1. `bash iphone/scripts/build.sh`（build → DerivedData/.../PetApp.app）
2. `install_app(app_path: iphone/build/DerivedData/Build/Products/Debug-iphonesimulator/PetApp.app)`
3. `launch_app(bundle_id: "com.zhuming.pet.app", terminate_running: true)`
4. **视觉验证**：`ui_view` 截图主界面 → 目视确认 ChestCardView 渲染在 HomeView 第 4 区块（statusBar / catStage / actionRow / **chestCard** / teamIdleCard），counting 态显示宝箱图标 + 倒计时文字（如 "05:00"）+ "宝箱倒计时" 标签。
5. **交互验证**（不强制，本 story 占位 closure）：等待倒计时归零或用 dev tools 注入 unlockable mock → `ui_view` 确认视觉切到金色高亮 + "开宝箱" 按钮 + "可开启" 标签。
6. **a11y 验证**：`ui_describe_all` → 抽样确认 "chestCard_counting" / "chestCard_unlockable" / "home_chestRemaining" 等 a11y identifier 可定位。

> **关键决策（dev 不能跳过 MCP 验证）**：与 Story 37.7-37.13 epic 红线一致——xcodebuild 通过只验证 code 编译；UI 视觉 / a11y 真实可用必须 MCP 实跑（lesson：Story 37.x 多次出现 build pass 但视觉 bug / 交互 bug 漏到 review 阶段才发现）。

**对应 Tasks**: Task 10.1

### AC11 — Deliverable 清单

- ✅ HomeViewModel.swift 基类追加 `@Published var chestRemainingSeconds: Int = 0`（保留全部 Story 2.2 / 2.5 / 5.5 / 37.4 / 37.7 / 8.4 老 init / bind / 公开 API 签名）；
- ✅ 新建 ChestTimerDriver.swift（订阅 appState.$currentChest → 每秒 Task.sleep tick 写 chestRemainingSeconds）；
- ✅ 新建 ChestCardView.swift（纯 value-type props + counting / unlockable 两态视觉 + 4 个 #Preview）；
- ✅ HomeContainerView.swift 改动：HomeContainerHomeViewBridge 加 `@EnvironmentObject var appState: AppState` 字段；chestSlot closure 替换 `EmptyView()` 为 `ChestCardView(currentChest: appState.currentChest, remainingSeconds: homeViewModel.chestRemainingSeconds, onOpenTap: {})`；
- ✅ RealHomeViewModel.swift 改动：init 末尾创建并启动 `ChestTimerDriver` + 持有引用让生命周期与 ViewModel 一致；
- ✅ AccessibilityID.swift 追加 `Home.chestOpenButton = "home_chestOpenButton"` 常量（旧 chestArea / chestRemaining 保留）；
- ✅ 新建 ChestCardViewTests.swift 含 6 case（counting 构造 / 默认值 0 / driver 切换 / driver 清空 / unlockable 派生 / RealHomeViewModel init 链路）；
- ✅ HomeUITests.swift 追加 `testChestCardShowsCountingAndUnlockableAnchors` UITest case + 旧 `testHomeViewShowsAllPlaceholders` 不动（chestArea / chestRemaining 在本 story 后**重新挂**到 ChestCardView 内 → 旧 case 若仍 skip 这两个断言，本 story 落地后**可恢复**这两个断言；详见 Dev Notes "旧 UITest 兼容"）；
- ✅ RootView.swift 加 UITEST_CHEST_COUNTING launch env hook（注入 mock counting chest 让 UITest 可定位 chestCard_counting 锚）；
- ✅ `iphone/PetApp.xcodeproj/project.pbxproj` 改动随 commit（xcodegen regen 结果）；
- ✅ `bash iphone/scripts/build.sh --test` 全绿；
- ✅ ios-simulator MCP 实跑视觉 + a11y 验证通过；
- ✅ project.yml **不**手动改（通配规则自动 inclusion）；
- ✅ RootView wire 不改（line 34 / 268 `HomeViewModel()` 基类老路径保留；仅追加 UITEST_CHEST_COUNTING launch env hook）；
- ✅ HomeView body **zero edit**（chestSlot 接缝硬契约兑现）；
- ✅ AppState **不动**（currentChest 字段已存在 Story 37.4 落地）。

## Tasks / Subtasks

- [x] Task 1: HomeViewModel 基类追加 chestRemainingSeconds 字段（AC1）
  - [x] 1.1 在 HomeViewModel.swift 内 `@Published petState` 同区域追加 `@Published public var chestRemainingSeconds: Int = 0` + 长 comment（按 AC1 spec 钦定）
- [x] Task 2: 新建 ChestTimerDriver.swift（AC2）
  - [x] 2.1 创建 `iphone/PetApp/Features/Home/ViewModels/ChestTimerDriver.swift`，按 AC2 spec 落地（weak refs + Combine sink + Task.sleep 1s tick + ABA guard via chest.id）
- [x] Task 3: 新建 ChestCardView.swift（AC3）
  - [x] 3.1 创建 `iphone/PetApp/Features/Home/Views/ChestCardView.swift`，按 AC3 spec 落地（纯 value-type props + counting / unlockable 两态视觉）
  - [x] 3.2 暴露 `internal static func formatMMSSForTesting(_ totalSeconds: Int) -> String` + `internal static func isUnlockableForTesting(status: HomeChestStatus, remainingSeconds: Int) -> Bool` 让单测可断言（与 Story 8.4 PetSpriteView helper 同模式）
  - [x] 3.3 4 个 #Preview（counting · candy / unlockable · candy / counting · dark / unlockable · dark）
- [x] Task 4: HomeContainerView 接入 ChestCardView（AC4）
  - [x] 4.1 修改 `HomeContainerHomeViewBridge.body` line 80-97：(a) 字段声明追加 `@EnvironmentObject var appState: AppState`；(b) chestSlot closure 内 `EmptyView()` 替换为 `ChestCardView(currentChest: appState.currentChest, remainingSeconds: homeViewModel.chestRemainingSeconds, onOpenTap: {})`
- [x] Task 5: RealHomeViewModel 启动 ChestTimerDriver（AC5）
  - [x] 5.1 在 RealHomeViewModel.init 末尾追加 `let driver = ChestTimerDriver(appState: appState, viewModel: self); driver.start(); self.chestTimerDriver = driver` + 加 `private var chestTimerDriver: ChestTimerDriver?` 字段 + bind(appState:) override 路径也 hookup driver（startChestTimerDriver helper 统一入口）
- [x] Task 6: ChestCardView Preview（AC6）
  - [x] 6.1 已在 Task 3.3 中覆盖（合并 task；本 task 仅校验 Preview 渲染在 Xcode Canvas 不 crash + 视觉合理）
- [x] Task 7: 单元测试 ChestCardViewTests.swift（AC7）
  - [x] 7.1 创建 `iphone/PetAppTests/Features/Home/ChestCardViewTests.swift`，落地 6 case（按 AC7 spec）
- [x] Task 8: UITest + RootView launch env hook（AC8）
  - [x] 8.1 在 HomeUITests.swift 内追加 `testChestCardShowsCountingAndUnlockableAnchors` case
  - [x] 8.2 在 RootView.swift（或 AppState 测试 helper）内追加 `UITEST_CHEST_COUNTING` launch env hook：UITEST_SKIP_GUEST_LOGIN 路径下 if env["UITEST_CHEST_COUNTING"] == "1" → appState.currentChest = mock counting chest（unlockAt = now + 5min）
  - [x] 8.3 加 `AccessibilityID.Home.chestOpenButton = "home_chestOpenButton"` 常量
- [x] Task 9: xcodegen + build verify（AC9）
  - [x] 9.1 `cd iphone && xcodegen generate` 让 3 个新文件加入 target
  - [x] 9.2 `bash iphone/scripts/build.sh --test` 跑测试通过（680 unit tests, 0 failures, 包括本 story 新增 6 case；UITest case 走 xcodebuild test 链路独立验证）
  - [x] 9.3 grep 校验抽样（按 AC9 spec — 全部通过）
- [x] Task 10: ios-simulator MCP 实跑验证（AC10）
  - [x] 10.1 按 AC10 spec 走 build → install → launch（UITEST_CHEST_COUNTING=1）→ screenshot → 视觉确认 counting 态宝箱图标 + "宝箱倒计时" 标签 + mm:ss 倒计时 ("04:33") 渲染在 chestCard 区块；同时 launch w/o env 确认 currentChest=nil 时 EmptyView 不破坏布局
- [x] Task 11: Deliverable 清单确认（AC11）
  - [x] 11.1 3 个新文件 + 修改 4 个老文件（HomeViewModel.swift / HomeContainerView.swift / RealHomeViewModel.swift / RootView.swift） + AccessibilityID.swift + HomeUITests.swift + pbxproj regen 全部待 commit（不在本 dev-story 范围）

## Dev Notes

### chestRemainingSeconds 放基类的根因（与 petState / Story 8.4 同模式）

Story 8.4 AC1 在 HomeViewModel 基类追加 `@Published var petState: MotionState = .rest`（基类持字段、Real/Mock 子类共享）。Story 21.1 追随同模式：

- **Mock 子类 hardcode 值**：Preview / UITest skip-guest-login 路径不走真实 driver，直接 hardcode `chestRemainingSeconds = 180` 等值快速看视觉。
- **Real 子类 driver 写入**：production 路径 RealHomeViewModel 创建 ChestTimerDriver 订阅 AppState 写值。
- **ChestCardView 用基类引用**：caller (HomeContainerHomeViewBridge) 拿到 `homeViewModel: HomeViewModel` 基类引用即可读 chestRemainingSeconds（不需要 down-cast 到 RealHomeViewModel）；与 PetSpriteView 用 `homeViewModel.petState` 同路径。

**否决路径**：

- ❌ ChestTimerDriver 由 HomeContainerView 持有：driver 生命周期与 view 绑定（view 重建 driver 重建），Combine subscription 与 Task 反复创建销毁 → 不稳定；且 Mock 不需要 driver 让 view 层分支判断复杂。
- ❌ 抽 `ChestTimerProtocol` 让 Mock 注入 fake：ADR-0002 §3.1 钦定不引 protocol mock；MockHomeViewModel 直接 hardcode 字段值即可，无需 protocol 抽象。

### HomeContainerHomeViewBridge appState 注入

当前 `HomeContainerView.swift` 内只在外层 `HomeContainerView` 持有 `@EnvironmentObject var appState`（line 30）；`HomeContainerHomeViewBridge` 子视图只持 `homeViewModel` / `resetIdentityViewModel` / `sessionStore` 三 environment 注入。

**本 story 改动**：Bridge 子视图追加 `@EnvironmentObject var appState: AppState`（SwiftUI 通过环境层级继承，RootView 注入路径继承不变）。这样 chestSlot closure 内可直接读 `appState.currentChest`。

**为何不在 closure 内 capture HomeContainerView 的 appState 透传**：SwiftUI 视图层级中 `@EnvironmentObject` 是声明式自动注入，把它"透传"给 Bridge 反而引入显式参数耦合；用 `@EnvironmentObject` 是社区惯例（与 HomeContainerView 上层一致）。

### UITEST launchEnvironment 扩展（UITEST_CHEST_COUNTING）

Story 37.3 / 37.7 落地的 UITEST_SKIP_GUEST_LOGIN 在 RootView.bootstrap 内 if-branch 跳过登录 + 注入 mock AppState。本 story 新增 UITEST_CHEST_COUNTING 在 SKIP_GUEST_LOGIN 路径下进一步注入 `appState.currentChest = HomeChest(id: "uitest-c1", status: .counting, unlockAt: Date().addingTimeInterval(300), openCostSteps: 1000, remainingSeconds: 300)`。

**dev 实装路径**：

```swift
// RootView.swift bootstrap 区域（伪代码）
if ProcessInfo.processInfo.environment["UITEST_SKIP_GUEST_LOGIN"] == "1" {
    // ... 既有 mock user / pet hydrate 路径 ...
    // Story 21.1 新增：
    if ProcessInfo.processInfo.environment["UITEST_CHEST_COUNTING"] == "1" {
        appState.currentChest = HomeChest(
            id: "uitest-c1",
            status: .counting,
            unlockAt: Date().addingTimeInterval(300),
            openCostSteps: 1000,
            remainingSeconds: 300
        )
    }
}
```

**为何加新 env 而不复用 SKIP_GUEST_LOGIN**：SKIP_GUEST_LOGIN 是登录态绕过；本 env 是具体 mock 数据精细控制，两个职责正交。

### 旧 UITest 兼容（chestArea / chestRemaining identifier 重新可用）

Story 37.7 落地后旧 `testHomeViewShowsAllSixPlaceholders`（HomeUITests.swift）删除了对 `AccessibilityID.Home.chestArea` / `chestRemaining` 的断言（chestSlot 接缝期不渲染）。

**本 story 落地后**：ChestCardView 内根容器挂 `chestCard_counting` / `chestCard_unlockable`（新字面量），倒计时 Text 挂 `AccessibilityID.Home.chestRemaining`（老常量 = "home_chestRemaining"）。**dev 可选**：在旧 `testHomeViewShowsAllSixPlaceholders` 内**可以**恢复对 chestRemaining 的断言（如果 launch env 注入了 counting mock chest）；**但本 story 不强制**修改旧 case，保 git history 可读；老 case 仍保留删除断言的状态即可，由 Story 21.2 / 22.1 视情况恢复。

**chestArea identifier**：本 story **不**主动复活该常量。理由：(a) chestArea 字面量是 "home_chestArea"，与新加的 "chestCard_counting" / "chestCard_unlockable" 字面量风格不一致（新走 Story 37.7 风格驼峰，旧走 Story 5.5 风格下划线）；(b) 保两套 identifier 并存会让 UITest 锚定路径混乱；(c) chestArea 常量保留无 view 使用（与 Story 37.13 Notes 一致：常量保留无业务消费时 dev 默契不复活，避免 dead-code）。

### ChestTimerDriver Task lifecycle vs SwiftUI .task

不用 SwiftUI `.task` modifier 触发 timer（与 HomeView 内 floatUp 动画用 SwiftUI Task 不同）。理由：

1. **ChestTimerDriver 是 ViewModel 层 helper**：与 SwiftUI view body 解耦；driver 启动时机在 RealHomeViewModel.init，与 view 生命周期独立。
2. **SwiftUI `.task` 在 view 重新出现时重启**（lesson：`docs/lessons/2026-04-26-swiftui-task-modifier-reentrancy.md`）；若用 `.task` 驱动 timer，HomeContainerView 切到 RoomView 再切回时 timer 会重新启动（多余 Combine subscription + Task），不安全。
3. **driver 自管 Task lifecycle**：subscription cancel + tickTask cancel 在 dealloc 时由 weak refs 自动断（与 Story 8.4 MotionStateMapper 同精神）。

### Source tree components to touch

- **新建（生产）**：
  - `iphone/PetApp/Features/Home/ViewModels/ChestTimerDriver.swift`
  - `iphone/PetApp/Features/Home/Views/ChestCardView.swift`
- **新建（测试）**：
  - `iphone/PetAppTests/Features/Home/ChestCardViewTests.swift`
- **修改**：
  - `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`（基类追加 chestRemainingSeconds 字段）
  - `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift`（init 末尾创建并启动 driver + 加字段）
  - `iphone/PetApp/Features/Home/Views/HomeContainerView.swift`（HomeContainerHomeViewBridge 加 appState 注入 + chestSlot closure 替换）
  - `iphone/PetApp/App/RootView.swift`（UITEST_CHEST_COUNTING launch env hook）
  - `iphone/PetApp/Shared/Constants/AccessibilityID.swift`（追加 `Home.chestOpenButton` 常量）
  - `iphone/PetAppUITests/HomeUITests.swift`（追加 testChestCardShowsCountingAndUnlockableAnchors）
  - `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen regen 结果）
- **不改**：
  - `iphone/PetApp/Features/Home/Views/HomeView.swift`（chestSlot 接缝硬契约 zero edit）
  - `iphone/PetApp/Features/Home/ViewModels/MockHomeViewModel.swift`（Mock 不需要 driver；hardcode chestRemainingSeconds 由测试 / Preview 在 caller 处直接设）
  - `iphone/PetApp/App/AppState.swift`（currentChest 字段 Story 37.4 已落地）
  - `iphone/PetApp/Features/Home/Models/HomeData.swift`（HomeChest 类型 Story 5.5 已落地）
  - `iphone/project.yml`（通配规则自动 inclusion）
  - server/ 任何文件（端独立）
  - ios/ 任何文件（CLAUDE.md 红线）

### Testing standards summary

- **测试入口**：`bash iphone/scripts/build.sh --test`（ADR-0002 §3.4 钦定）
- **测试框架**：XCTest only（ADR-0002 §3.1）；UITest 走 XCUIApplication
- **单元测试位置**：`iphone/PetAppTests/Features/Home/ChestCardViewTests.swift`（与 production 镜像；与 HomeViewModelTests / PetSpriteViewTests 同目录）
- **测试 case 数量**：≥4 case（按 epic AC line 3024）；本 story 落地 6 case
- **测试运行时**：每 case ≤ 150ms（Combine sink + Task.sleep 50ms 让 main run loop dispatch）；UITest 单 case 5-15s
- **不测**：
  - SwiftUI body 渲染（ADR-0002 §3.1 禁用 ViewInspector）
  - 真实秒级倒计时（不等 1 秒过去；通过 appState.currentChest 切换触发 sink 立即重算）
  - SnapshotTesting 视觉 diff（ADR-0002 §3.1 禁用；视觉由 #Preview + Story 37.13 visual-review-checklist + AC10 MCP 兜底）
- **覆盖目标**：HomeViewModel.chestRemainingSeconds 默认值 + ChestTimerDriver Combine subscription 路径 + ChestCardView formatter / 派生函数 + RealHomeViewModel init driver 启动链路

### Project Structure Notes

- **目录约定**：完全按 `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §6.5 (Chest 模块) + ADR-0002 §3.3 的 `iphone/PetApp/Features/Home/` 目录约定。`ViewModels/ChestTimerDriver.swift` 与已有 `ViewModels/HomeViewModel.swift` / `RealHomeViewModel.swift` / `MockHomeViewModel.swift` 同级；`Views/ChestCardView.swift` 与已有 `Views/HomeView.swift` / `PetSpriteView.swift` 同级。Test mirror 路径 `PetAppTests/Features/Home/ChestCardViewTests.swift` 与 production 严格镜像（已有 HomeViewModelTests / PetSpriteViewTests 同模式）。
- **Naming convention**：`ChestCardView` / `ChestTimerDriver`（PascalCase + 业务前缀，与 PetSpriteView / MotionStateMapper 同风格）。`chestCard_counting` / `chestCard_unlockable` a11y identifier 走小驼峰 + 下划线后缀（与 Story 37.7 落地的 `homeStatusBar` / `homeTeamIdleCard_create` 风格一致）。
- **Detected conflicts or variances**：无。`iphone/PetApp/Features/Home/Views/HomeView.swift` chestSlot 接缝已留出；`HomeChest` 类型已存在；`AppState.currentChest` 已存在；`AccessibilityID.Home.chestRemaining` 老常量已存在（值 "home_chestRemaining"，本 story 重新挂载到 ChestCardView 倒计时 Text）。

### References

- [Source: \_bmad-output/planning-artifacts/epics.md#Story 21.1] — 本 story acceptance 原文（≥4 case 测试 / chest counting & unlockable 两态 / chestSlot 接缝 / a11y identifier "chestCard_counting" / "chestCard_unlockable" / unlockAt 自动切 unlockable）
- [Source: \_bmad-output/planning-artifacts/epics.md#Epic 21] — Epic 21 范围说明（节点 7 不入仓：弹窗仅展示）
- [Source: \_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md#3.2] — AppState 白名单含 `currentChest`；倒计时秒数归 ViewModel 而非 AppState
- [Source: \_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md#3.5] — HomeViewModel 演变模式（chestRemainingSeconds 在 Story 21.1 落地，与 §3.5 表格预设一致）
- [Source: \_bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md#3.2] — HomeContainerView 互斥状态机契约（idle 态 chestCard 渲染；inRoom 态切到 RoomView 时 chestSlot 自然消失）
- [Source: \_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.1] — XCTest only 测试框架钦定
- [Source: \_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.3] — `iphone/PetApp/` 目录约定 + xcodegen 通配规则
- [Source: \_bmad-output/implementation-artifacts/37-7-homeview-scaffold.md] — HomeView<ChestSlot, PetSlot> generic struct + chestSlot ViewBuilder closure 接缝（本 story 兑现接缝）
- [Source: \_bmad-output/implementation-artifacts/37-7-homeview-scaffold.md#AC2] — HomeViewModel class 层次（基类 + Real/Mock 子类）模式（本 story chestRemainingSeconds 跟随基类持字段路径）
- [Source: \_bmad-output/implementation-artifacts/8-4-主界面猫-sprite-三态动画切换.md] — PetSpriteView 视图组装层契约锁定（纯 value-type props，不持 ObservableObject）；本 story ChestCardView 同模式
- [Source: \_bmad-output/implementation-artifacts/8-4-主界面猫-sprite-三态动画切换.md#AC1] — petState 在基类 @Published（本 story chestRemainingSeconds 跟随）
- [Source: iphone/PetApp/Features/Home/Views/HomeView.swift#L41-L75] — chestSlot / petSlot 接缝 init（本 story 调用方传入 ChestCardView）
- [Source: iphone/PetApp/Features/Home/Views/HomeContainerView.swift#L75-L97] — HomeContainerHomeViewBridge.body（本 story 修改 chestSlot closure + 加 appState 注入）
- [Source: iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift] — 基类（本 story 追加 chestRemainingSeconds 字段）
- [Source: iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift] — Real 子类（本 story init 末尾启动 driver）
- [Source: iphone/PetApp/Features/Home/Models/HomeData.swift#L192-L227] — HomeChest / HomeChestStatus 类型（status .counting / .unlockable + unlockAt Date + remainingSeconds）
- [Source: iphone/PetApp/App/AppState.swift#L66-L78] — `@Published public var currentChest: HomeChest?`（本 story 消费）
- [Source: iphone/PetApp/Core/DesignSystem/Primitives/Card.swift] — Card primitive（ChestCardView 容器复用，与 teamIdleCard 同档）
- [Source: iphone/PetApp/Core/DesignSystem/Primitives/PrimaryButton.swift] — PrimaryButton primitive（unlockable 态"开宝箱"按钮复用）
- [Source: iphone/PetApp/Core/DesignSystem/Primitives/Icons.swift] — Icons.symbol(for: "box") = "shippingbox.fill"（ChestCardView 图标）
- [Source: iphone/PetApp/Shared/Constants/AccessibilityID.swift#L30-L37] — `Home.chestArea` / `Home.chestRemaining` 老常量（chestRemaining 在本 story 重新挂载）
- [Source: iphone/PetApp/Features/Home/Views/PetSpriteView.swift] — 视图组装层契约锁定参考（纯 props + 不持 ObservableObject）；本 story ChestCardView 同模式
- [Source: docs/lessons/2026-04-26-swiftui-task-modifier-reentrancy.md] — SwiftUI .task 重启边界（本 story ChestTimerDriver 不用 .task，用 Combine sink + Task.sleep 1s tick）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#6.5] — iOS Chest 模块（首页倒计时 + 弹窗）设计
- [Source: docs/宠物互动App_V1接口设计.md#7.1] — GET /chest/current 返回 schema（本 story 不调用，仅消费 AppState.currentChest 字段；Story 21.2 落地接口）
- [Source: docs/宠物互动App_时序图与核心业务流程设计.md#7] — 宝箱状态流转（counting → unlockable）

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context)

### Debug Log References

- `bash iphone/scripts/build.sh --test` → 680 unit tests pass, 0 failures, BUILD SUCCESS
- xcodegen generate: PetApp.xcodeproj regen 完成（3 个新文件自动通过通配规则纳入 PetApp / PetAppTests target）
- ios-simulator MCP 实跑：UITEST_SKIP_GUEST_LOGIN=1 + UITEST_CHEST_COUNTING=1 launch → screenshot 确认 ChestCardView 渲染 counting 视觉态（宝箱图标 + "宝箱倒计时" 标签 + mm:ss "04:33" 倒计时）；w/o UITEST_CHEST_COUNTING launch → screenshot 确认 currentChest=nil 时 EmptyView 路径正确（4 区块布局不破坏）
- grep 校验抽样全部通过：chestRemainingSeconds (HomeViewModel.swift, 2 matches), ChestTimerDriver/chestTimerDriver (RealHomeViewModel.swift, 10 matches), ChestCardView( (HomeContainerView.swift, 3 matches), `chestSlot: { EmptyView() }` (HomeContainerView.swift, 0 matches — EmptyView 占位已消除)

### Completion Notes List

- AC1 完成：HomeViewModel 基类追加 `@Published public var chestRemainingSeconds: Int = 0`（紧跟 petState 之后）；与 Story 8.4 petState 同模式（基类持字段、Mock/Real 子类共享）.
- AC2 完成：新建 ChestTimerDriver.swift（weak refs + Combine sink + Task.sleep 1s tick + ABA guard via chest.id；按 AC2 spec 落地无偏差）.
- AC3 完成：新建 ChestCardView.swift（纯 value-type props + counting/unlockable 双态视觉 + 暴露 internal static helpers `formatMMSSForTesting` / `isUnlockableForTesting` 让单测可断言）.
- AC4 完成：HomeContainerHomeViewBridge 追加 `@EnvironmentObject var appState: AppState` + chestSlot closure 从 EmptyView() 替换为 ChestCardView(currentChest: appState.currentChest, remainingSeconds: homeViewModel.chestRemainingSeconds, onOpenTap: {}).
- AC5 完成：RealHomeViewModel 追加 `chestTimerDriver: ChestTimerDriver?` 字段 + init(appState:) 路径直接 startChestTimerDriver + bind(appState:) override 路径也 hookup（兼容 RootView @StateObject 老 wire 异步注入 AppState 的路径；与 greetingSubscription 一次性 guard 同节奏）.
- AC6 完成：ChestCardView 文件底部 4 个 #Preview（counting · candy / unlockable · candy / counting · dark / unlockable · dark；double 主题 × double 状态全覆盖）.
- AC7 完成：新建 ChestCardViewTests.swift 含 6 case（counting 构造 + formatter / chestRemainingSeconds 默认 0 / driver 切换 / driver 清空 / unlockable 派生 / RealHomeViewModel init 链路）；全部走 props / ViewModel 字段 / driver 行为断言，不走 SwiftUI body 内省（ADR-0002 §3.1 钦定 XCTest only）.
- AC8 完成：HomeUITests 追加 testChestCardShowsCountingAndUnlockableAnchors case（仅验证 counting 态锚 + chestRemaining 倒计时 Text 锚）+ AccessibilityID.Home.chestOpenButton = "home_chestOpenButton" 常量；RootView 追加 UITEST_CHEST_COUNTING launch env hook（仅 Debug build 生效，与 UITEST_SKIP_GUEST_LOGIN / UITEST_FORCE_IN_ROOM 同前缀）.
- AC9 完成：xcodegen generate 重新生成 PetApp.xcodeproj/project.pbxproj；bash iphone/scripts/build.sh --test 全绿（680 unit tests pass，包括本 story 新增 6 case；UITest target 也已加入新 case）；grep 校验抽样 4 项全部 pass.
- AC10 完成：ios-simulator MCP 实跑视觉验证 —— UITEST_CHEST_COUNTING=1 模式 screenshot 确认 counting 态 ChestCardView 正确渲染（5 区块 statusBar / catStage / actionRow / chestCard / teamIdleCard 全在）；w/o env screenshot 确认 currentChest=nil → EmptyView 渲染路径不破坏布局（4 区块紧凑）.
- AC11 完成：deliverable 全部就位（3 个新文件 + 修改 6 个老文件 + pbxproj regen），全部待 commit（由 fix-review / story-done 阶段统一 commit）.

### File List

新建（生产）：
- `iphone/PetApp/Features/Home/ViewModels/ChestTimerDriver.swift`
- `iphone/PetApp/Features/Home/Views/ChestCardView.swift`

新建（测试）：
- `iphone/PetAppTests/Features/Home/ChestCardViewTests.swift`

修改：
- `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`（基类追加 chestRemainingSeconds 字段）
- `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift`（追加 chestTimerDriver 字段 + 两路 init/bind 都启动 driver + startChestTimerDriver helper）
- `iphone/PetApp/Features/Home/Views/HomeContainerView.swift`（HomeContainerHomeViewBridge 加 appState 注入 + chestSlot closure 替换）
- `iphone/PetApp/App/RootView.swift`（UITEST_CHEST_COUNTING launch env hook）
- `iphone/PetApp/Shared/Constants/AccessibilityID.swift`（追加 Home.chestOpenButton 常量）
- `iphone/PetAppUITests/HomeUITests.swift`（追加 testChestCardShowsCountingAndUnlockableAnchors）
- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen regen 结果）

## Change Log

| Date       | Version | Description                                                                                                                                              | Author |
|------------|---------|----------------------------------------------------------------------------------------------------------------------------------------------------------|--------|
| 2026-05-15 | 1.0     | Story 21.1 dev-story 完成：ChestCardView + ChestTimerDriver + HomeViewModel.chestRemainingSeconds 落地；AC1-AC11 全部 done；测试 680/680 通过；MCP 视觉验证通过；status → review | dev    |

| Date       | Change |
|------------|--------|
| 2026-05-15 | 初稿落地：Story 21.1 首页宝箱组件 SwiftUI + ChestCardView counting / unlockable 两态视觉 + ChestTimerDriver 本地倒计时驱动（订阅 AppState.currentChest） + HomeViewModel 基类追加 chestRemainingSeconds 字段 + HomeContainerHomeViewBridge chestSlot closure 兑现接缝 + 6 case 单元测试 + UITest 1 case + UITEST_CHEST_COUNTING launch env hook + ios-simulator MCP 实跑验证. Sprint-status: backlog → ready-for-dev. |
