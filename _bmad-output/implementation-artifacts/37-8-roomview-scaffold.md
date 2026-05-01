# Story 37.8: RoomView Scaffold + RoomViewModel class 层次 + Mock/Real 两子类

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want Home Tab inRoom 态显示 ui_design 高保真房间界面（顶部返回 / 房间号 Card + 复制 / 共享舞台 + 4 格成员列表 + 离开按钮）+ 接缝设计支持 Story 12.1（真实 RealRoomViewModel + WS 接入）后续注入,
so that 既有视觉壳又有可持续接缝（RoomScaffoldView 内部代码 zero edit 让 Epic 12.x WS 链路打开），同时把 Story 37.3 落地的 `RoomViewPlaceholder` 占位 stub 替换为 ui_design `room.jsx` 像素级匹配的高保真 Scaffold。

## 故事定位（Epic 37 第四层第 2 条 story；Scaffold 主体 6 屏并行链路第二条，与 37.7 同模式）

这是 Epic 37「iPhone 架构层重构 + UI Scaffold（壳先行）」的**第四层 story**——上游 37.3（HomeContainerView 互斥状态机 + RoomViewPlaceholder 占位挂点）/ 37.4（AppState.currentRoomId）/ 37.5（Theme）/ 37.6（primitives）全部 done；37.7（HomeView Scaffold）已用 class 层次 + Mock/Real 两子类模式落地，**本 story 1:1 复刻该模式于 RoomView**。本 story 是 **UI Scaffold 主体** 类——属于 Epic 37 §AC 红线的「数据完全 mock + 禁 import APIClient/Repository/UseCase + 视觉像素级匹配 ui_design + 主题用 `@Environment(\.theme)` 取 token + 测试不引 SnapshotTesting/ViewInspector + 通过 `bash iphone/scripts/build.sh --test`」适用范围。

**本 story 落地后立即解锁**：
- Story 12.1（RoomView 真实 ViewModel 注入）—— 缩窄为「在本 story 已交付的 RoomScaffoldView 上把 MockRoomViewModel 替换为 RealRoomViewModel（持 WSState + members + memberPetStates；roomId 从 AppState 读 String 类型，按 AR21 ID 字符串约定）」（详见 sprint-change-proposal-2026-04-29-v2.md §5.1 落地条款）
- Story 12.2-12.7（WebSocketClient / snapshot 解析 / 加入退出广播 / 自动重连 / 心跳 / UseCase wire）—— 在 Story 12.1 RealRoomViewModel 上接 WS 消息驱动 `@Published var members / wsState / memberPetStates`；RoomScaffoldView 视图内部 zero edit
- Story 35.2（房间页内"分享"按钮）—— RoomScaffoldView 顶部添加 share button；本 story 范围**不**实装 share，仅声明顶部 leading/trailing 留白契约（详 Dev Notes "顶部留白契约"）
- Story 37.13（accessibility identifier 总表）—— RoomScaffoldView 全部 a11y identifier 来源；本 story 在 RoomScaffoldView 内 inline 字符串（`returnButton` / `roomIdDisplay` / `copyButton` / `roomMember_0..3` / `leaveButton`），Story 37.13 收口归并到 `AccessibilityID.Room`

**本 story 的"实装"动作**（一句话概括）：在 `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift`（新建文件，**不**改 RoomViewPlaceholder.swift —— 见 Dev Notes "RoomViewPlaceholder 不删保 git history"）落地 generic struct `RoomScaffoldView` + `@ObservedObject var state: RoomViewModel`（基类直接，**不**泛型 state）；新建基类 `class RoomViewModel: ObservableObject`（去 final 让子类可继承；与 HomeViewModel 改造同精神）+ 4 个 `@Published` 字段（`roomCodeForCopy: String / hostCatName: String / members: [RoomMember] / userIsHost: Bool`）+ 2 个 abstract method（`onLeaveTap()` / `onCopyTap()`）；新建 `MockRoomViewModel: RoomViewModel` 子类（硬编码 mock 4/3/2 成员场景 + override 方法仅 print + invocations 数组）+ `RealRoomViewModel: RoomViewModel` 子类骨架（构造注入 AppState；override 方法本期为占位，Story 12.1+ 实装真实 WS / UseCase 调用）；新建 `RoomMember` value type（id / name / level / status / isHost）+ `Avatar` 复用现有 primitive。RoomScaffoldView 视觉按 `iphone/ui_design/source/screens/room.jsx` + `iphone/ui_design/README.md` §RoomScreen 像素级翻译（顶部 / 房间号 Card / 共享舞台 Card / 成员列表 / 离开按钮 5 区块）。**HomeContainerView 内 `RoomViewPlaceholder()` 调用替换为 `RoomScaffoldView(state: ...)` 真实 Scaffold**（caller 漏改靠编译器报错驱动；与 Story 37.3 / 37.7 同精神）。

**关键路径："新建" + caller 替换（与 Story 37.7 HomeView 重写不同：本 story 是新建 + 替换占位）**：

- `RoomViewPlaceholder.swift` **不删除**（保 Story 37.3 git history 可读 + 让人对比演进足迹）；仅在 `HomeContainerView.swift` 内 line 30 把 `RoomViewPlaceholder()` 调用替换为 `RoomScaffoldView(state: roomViewModel)`（caller 漏改靠编译器报错驱动）；`RoomViewPlaceholder` 类型本身保留（无 caller 引用）。Story 37.13 a11y 总表归并时再决定是否一并清理（属下游决策，本 story 不收口）
- `roomViewModel: RoomViewModel` 注入路径走与 HomeView 相同模式：RootView 内 `@StateObject private var roomViewModel = RoomViewModel()`（基类无参 init）+ `.environmentObject(roomViewModel)`；HomeContainerView 内 `@EnvironmentObject var roomViewModel: RoomViewModel` 取出后传给 `RoomScaffoldView(state:)`（基类 ObservableObject 兼容；本 story **不**改 RootView wire 切到 RealRoomViewModel —— Story 12.1 决定何时切换）
- 顶部"返回"按钮 `state.onLeaveTap()` 行为：本 story Mock 路径只 print + 设 invocations；Real 路径（RealRoomViewModel）调 `appState.setCurrentRoomId(nil)` 让 HomeContainerView 互斥状态机自动切回 idle 态（依靠 Story 37.3 落地的 `appState.currentRoomId` 数据流；不引入 LeaveRoomUseCase 真实调用——属 Story 12.7 范围）

**不涉及**（红线）：
- **不**实装 `LeaveRoomUseCase` / `WebSocketClient` / `JoinRoomUseCase` 真实 UseCase（Story 12.7 / 12.2 落地；本 story 占位 print/setRoomId）
- **不**接 WS 真实消息（Story 12.x 落地；本 story `members` 字段是 mock 硬编码）
- **不**改 RootView `@StateObject` wire（基类 RoomViewModel 仍允许无参 init；Story 12.1 决定何时切到 RealRoomViewModel）
- **不**改 HomeContainerView 互斥状态机决策（`HomeRoomDispatcher.shouldShowRoom(currentRoomId:)` 不动；仅替换 inRoom 分支渲染目标 `RoomViewPlaceholder()` → `RoomScaffoldView(state: roomViewModel)`）
- **不**改 AppState / HomeData / HomePet / HomeUser 类型（Story 37.4 已锁定）
- **不**实装 share 按钮（Story 35.2 落地；本 story 顶部右侧仅留 40pt 空白对齐 `<div style={{width:40}}/>` ui_design 占位）
- **不**实装小猫弹跳错峰动画的 keyframe 真实细节（用 SwiftUI `.scaleEffect` + `.animation(.easeInOut(duration: 1.1).repeatForever().delay(Double(index) * 0.2))` 简化版；视觉精度由 Story 37.13 visual-review-checklist 把关）
- **不**引 SnapshotTesting / ViewInspector（ADR-0002 §3.1 钦定 XCTest only）
- **不**改 ios/ 任何文件（CLAUDE.md + ADR-0002 §3.3）
- **不**改 server/ 任何文件
- **不**收口 `AccessibilityID.Room` 常量（本 story inline 字符串；Story 37.13 一次性归并所有 7 屏 a11y identifier）
- **不**删除 `RoomViewPlaceholder.swift`（保 git history；下游 Story 37.13 / 12.1 决定）
- **不**预先生成 `RoomMember` 之外的额外 helper / mapping 类型

## Acceptance Criteria

> **AC 编号体系**：AC1 是 RoomViewModel class 层次基类（class + 4 字段 + 2 abstract method）；AC2 是 MockRoomViewModel / RealRoomViewModel 两子类；AC3 是 RoomMember 值类型；AC4 是 RoomScaffoldView struct + 5 区块视觉；AC5 是 HomeContainerView caller 替换 + RootView wire；AC6 是 #Preview 双主题；AC7 是单元测试 ≥4 case；AC8 是 UITest a11y 定位 7 锚；AC9 是 build verify；AC10 是 Deliverable 清单。

### AC1 — 新建 RoomViewModel 基类（class 层次 + 4 字段 + 2 abstract method）

**新建文件**：`iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift`

**类签名**（class 而非 final，让 Mock/Real 子类可继承；与 HomeViewModel Story 37.7 同精神）：

```swift
// RoomViewModel.swift
// Story 37.8 AC1: RoomScaffoldView 基类 ViewModel（class 层次 + 4 字段 + 2 abstract method）.
//
// 设计：与 HomeViewModel 同精神（class 而非 final + abstract method 用 fatalError 强制 override）.
// 字段范围：仅 4 字段（roomCodeForCopy / hostCatName / members / userIsHost）.
// 节点 4 后 Story 12.1 RealRoomViewModel 子类扩 wsState / memberPetStates 字段（不在本 story 范围）.

import Foundation
import Combine

@MainActor
public class RoomViewModel: ObservableObject {
    /// 房间代码（mock "1234567"；Story 12.1 RealRoomViewModel 从 appState.currentRoomId 派生）.
    @Published public var roomCodeForCopy: String = ""

    /// 房主猫名（mock "小花"；用于顶部"{猫名}的小屋"标题；Story 12.1 后从 hostMember 派生）.
    @Published public var hostCatName: String = ""

    /// 成员数组（mock 0-4 成员；Story 12.1 后从 WS room.snapshot 派生）.
    @Published public var members: [RoomMember] = []

    /// 当前用户是否为房主（mock false；Story 12.1 后从 user.id == host.id 派生）.
    /// 本 story 仅声明字段，不在视觉中区分（队长标签由 RoomMember.isHost 标记，不依赖此字段）.
    @Published public var userIsHost: Bool = false

    public init() {}

    // MARK: - abstract method（基类 fatalError 占位，子类必 override）

    /// 离开房间按钮回调（顶部返回按钮 / 底部"离开房间" PrimaryButton 共用同一回调）.
    /// MockRoomViewModel: 记录 invocation + print log.
    /// RealRoomViewModel（Story 12.1+）: 调 LeaveRoomUseCase + appState.setCurrentRoomId(nil).
    public func onLeaveTap() {
        fatalError("RoomViewModel.onLeaveTap must be overridden by subclass")
    }

    /// 复制房间号按钮回调.
    /// MockRoomViewModel: 记录 invocation + print log（视图内 1.2s "已复制" feedback 由 SwiftUI @State 本地持有）.
    /// RealRoomViewModel: 调 UIPasteboard.general.string = roomCodeForCopy + 同 mock 行为.
    public func onCopyTap() {
        fatalError("RoomViewModel.onCopyTap must be overridden by subclass")
    }
}
```

> **关键决策**：abstract method 用 `fatalError` 而非 default empty body —— 与 HomeViewModel Story 37.7 同精神（让漏 override 立刻 crash + 测试覆盖逻辑路径）；**不接受**默认 empty 实现（会让 RealRoomViewModel 漏 override 不 crash 但行为静默错）。

> **基类无参 init 兼容路径**：RootView `@StateObject private var roomViewModel = RoomViewModel()` 走基类无参 init；调用 `roomViewModel.onLeaveTap()` 等会触发 fatalError —— 但**本 story 范围内 RootView 走的 wire 路径下不直接调 `roomViewModel.onLeaveTap()`**（HomeContainerView 把 roomViewModel 传给 RoomScaffoldView 的 `state:` 参数；RoomScaffoldView body 只在 inRoom 态渲染并调用 onLeaveTap/onCopyTap —— 即 user 在 idle 态绝不会触发；切到 inRoom 态前必由 Story 12.7 / 37.12 落地的 join/create flow 把基类 roomViewModel 替换或子类化）。Preview / UITest skip-guest-login 路径**必须用 MockRoomViewModel**（详见 AC2 + Dev Notes "RootView wire 不动 + 基类 vs Mock 选择策略"）。

**对应 Tasks**: Task 1.1, 1.2

### AC2 — 新建 MockRoomViewModel / RealRoomViewModel 两子类（独立文件）

**新建文件**: `iphone/PetApp/Features/Room/ViewModels/MockRoomViewModel.swift`

```swift
// MockRoomViewModel.swift
// Story 37.8 AC2: RoomViewModel mock 子类，用于 #Preview / UITest skip-guest-login / Scaffold 单元测试.
//
// 设计：
//   - 硬编码 mock 数据（roomCodeForCopy "1234567" / hostCatName "小花" / members 0-4 可配 / userIsHost false）
//   - override 2 个 abstract method 仅 print log + 记录 invocations 数组
//   - **不**依赖 AppState（Mock 路径走纯 ViewModel-only 数据）
//
// 与 MockHomeViewModel Story 37.7 同模式（invocations 数组而非 closure spy）.

import Foundation
import Combine
import os.log

@MainActor
public final class MockRoomViewModel: RoomViewModel {
    /// 单元测试用：记录所有方法调用
    public enum Invocation: Equatable {
        case leaveTap
        case copyTap
    }

    @Published public var invocations: [Invocation] = []

    /// 默认构造 — 4 成员（房主 + 3 普通成员）满员场景，对齐 ui_design room.jsx 默认 demo 数据.
    public override init() {
        super.init()
        self.roomCodeForCopy = "1234567"
        self.hostCatName = "小花"
        self.members = MockRoomViewModel.fourMembersMock
        self.userIsHost = false
    }

    /// 测试 / Preview 灵活构造 — 可注入任意 members 数（0/1/2/3/4）+ 自定 roomCode + hostCatName.
    /// 用于单元测试 case#1 (4 成员)、case#2 (2 成员) 等；与 HomeViewScaffoldTests 同模式.
    public init(
        roomCodeForCopy: String = "1234567",
        hostCatName: String = "小花",
        members: [RoomMember] = MockRoomViewModel.fourMembersMock,
        userIsHost: Bool = false
    ) {
        super.init()
        self.roomCodeForCopy = roomCodeForCopy
        self.hostCatName = hostCatName
        self.members = members
        self.userIsHost = userIsHost
    }

    // MARK: - override abstract methods

    public override func onLeaveTap() {
        os_log(.debug, "MockRoomViewModel.onLeaveTap")
        invocations.append(.leaveTap)
    }

    public override func onCopyTap() {
        os_log(.debug, "MockRoomViewModel.onCopyTap")
        invocations.append(.copyTap)
    }

    // MARK: - mock 数据

    /// 4 成员满员 mock（房主 + 3 普通；level / status 与 ui_design room.jsx demo 一致）.
    public static let fourMembersMock: [RoomMember] = [
        RoomMember(id: "u1", name: "小花", level: 8, status: "在玩耍", isHost: true),
        RoomMember(id: "u2", name: "Mocha", level: 7, status: "在散步", isHost: false),
        RoomMember(id: "u3", name: "Latte", level: 9, status: "在玩耍", isHost: false),
        RoomMember(id: "u4", name: "Espresso", level: 6, status: "在休息", isHost: false),
    ]

    /// 2 成员场景 mock（房主 + 1）；用于单元测试 case#2 验证空位渲染.
    public static let twoMembersMock: [RoomMember] = [
        RoomMember(id: "u1", name: "小花", level: 8, status: "在玩耍", isHost: true),
        RoomMember(id: "u2", name: "Mocha", level: 7, status: "在散步", isHost: false),
    ]
}
```

**新建文件**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`

```swift
// RealRoomViewModel.swift
// Story 37.8 AC2: RoomViewModel 生产实装子类（构造注入 AppState；override 2 个 abstract method 占位 stub）.
//
// 范围（本 story 占位；Story 12.1 / 12.7 / 12.2-12.6 等填充真实 WS / UseCase 调用）：
//   - 构造注入 AppState（按 ADR-0010 §3.1 ViewModel 注入规则）+ parameterless init() 走 bind(appState:)
//   - override onLeaveTap / onCopyTap 为占位行为：
//     · onLeaveTap: 调 appState.setCurrentRoomId(nil) 让 HomeContainerView 互斥状态机切回 idle
//       （依赖 Story 37.3 数据流；不调 LeaveRoomUseCase 真实 — Story 12.7 落地）
//     · onCopyTap: print log（实际复制行为靠 RoomScaffoldView 内 SwiftUI UIPasteboard 调用 + @State 1.2s feedback）
//
// **不**调用任何 UseCase / Repository / APIClient（Epic 37 红线：UI Scaffold 数据完全 mock）.
// **不**订阅 WS / appState.$currentRoomId（Story 12.1 落地；本 story RealRoomViewModel 仅占位骨架）.

import Foundation
import Combine
import os.log

@MainActor
public final class RealRoomViewModel: RoomViewModel {
    /// 构造注入 AppState（与 RealHomeViewModel Story 37.7 round 1 P1 fix 同模式）.
    /// `bind(appState:)` 路径让 RootView `@StateObject` 老模式可用 — AppState 也是同级 @StateObject,
    /// 不能在属性初始化器内交叉引用（编译期不允许 self 提前求值）.
    private var appState: AppState?

    /// parameterless init —— RootView `@StateObject` 老模式可用; AppState 通过 bind 异步注入.
    public override init() {
        super.init()
        self.appState = nil
    }

    public init(appState: AppState) {
        super.init()
        self.appState = appState
        // 节点 1 阶段 mock：从 appState.currentRoomId 派生 roomCodeForCopy（Story 12.1 后改 WS snapshot 派生）.
        if let roomId = appState.currentRoomId {
            self.roomCodeForCopy = roomId
        } else {
            self.roomCodeForCopy = ""
        }
        self.hostCatName = appState.currentPet?.name ?? "默认小猫"
        // members 暂为空（Story 12.1 接 WS room.snapshot 后填充）.
        self.members = []
        self.userIsHost = false
    }

    /// AppState 异步注入入口（与 HomeViewModel.bind(appState:) 同模式）.
    public func bind(appState: AppState) {
        self.appState = appState
        if let roomId = appState.currentRoomId {
            self.roomCodeForCopy = roomId
        }
        self.hostCatName = appState.currentPet?.name ?? "默认小猫"
    }

    // MARK: - override abstract methods（本 story 占位；Story 12.7 实装真实 LeaveRoomUseCase）

    public override func onLeaveTap() {
        os_log(.debug, "RealRoomViewModel.onLeaveTap (Story 12.7 will wire LeaveRoomUseCase)")
        // 节点 1 占位：直接置 currentRoomId = nil 让 HomeContainerView 切回 idle
        // （依赖 Story 37.3 互斥状态机数据流；不调 server LeaveRoom API — Story 12.7 落地）.
        self.appState?.setCurrentRoomId(nil)
    }

    public override func onCopyTap() {
        os_log(.debug, "RealRoomViewModel.onCopyTap")
        // 实际 UIPasteboard 复制 + 1.2s 视觉反馈由 RoomScaffoldView 内 SwiftUI @State 持有 + 调用此方法即触发.
        // 本 ViewModel 层仅记录 / 透传；不直接操作 UIKit（Epic 37 红线：ViewModel 层不依赖 UIKit）.
    }
}
```

> **关键决策 1**：MockRoomViewModel / RealRoomViewModel 都 `final` —— 子类不可再被继承（与 ADR-0010 §3.1 mock 模式钦定 + Story 37.7 同精神）；只有基类 `RoomViewModel` 是 `class`（非 final）。

> **关键决策 2**：MockRoomViewModel 用 invocations 数组而非 closure spy —— 与 MockHomeViewModel Story 37.7 同精神（直接断言 ObservableObject 内 @Published 字段）。

> **关键决策 3**：RealRoomViewModel.onLeaveTap 调 `appState.setCurrentRoomId(nil)` —— 依赖 Story 37.3 / 37.4 落地的 `HomeRoomDispatcher.shouldShowRoom` 数据流让 HomeContainerView 自动切回 idle；**不**调 LeaveRoomUseCase（Story 12.7 范围）。RealRoomViewModel.onCopyTap 仅 log（实际 UIPasteboard 复制 + 1.2s 视觉反馈由 RoomScaffoldView 内 SwiftUI @State 持有）。

**对应 Tasks**: Task 2.1, 2.2

### AC3 — 新建 RoomMember 值类型

**新建文件**: `iphone/PetApp/Features/Room/Models/RoomMember.swift`

```swift
// RoomMember.swift
// Story 37.8 AC3: RoomScaffoldView 成员列表数据模型.
//
// 设计：value type + Equatable + Sendable + Identifiable，纯展示数据；mock 值在 Mock子类静态属性.
// 节点 4 后 Story 12.1 接 WS room.snapshot 时复用该类型（字段对齐：id / nickname / petLevel / status / isHost）；
// 若发现需要扩展（如 lastSeenAt / pet currentState 等）走 ADR-0010 §4.4 缓解策略，本 story 不预 over-design.

import Foundation

public struct RoomMember: Equatable, Identifiable, Sendable {
    public let id: String         // userId（Story 12.1 后对齐 server user.id）
    public let name: String       // 成员昵称
    public let level: Int         // 小猫等级（mock 6-9；节点 8 后接真实 user_pet level）
    public let status: String     // mock "在玩耍" / "在散步" / "在休息"（节点 5 后接真实 pet.currentState 派生）
    public let isHost: Bool       // 是否房主（决定"队长"标签渲染）

    public init(
        id: String,
        name: String,
        level: Int,
        status: String,
        isHost: Bool
    ) {
        self.id = id
        self.name = name
        self.level = level
        self.status = status
        self.isHost = isHost
    }
}
```

> **关键决策**：字段名 `name / level / status / isHost` 直接对齐 ui_design room.jsx:117-121 显示位（`m.name` / `m.isHost && '队长'` / `Lv.{m.level}` / `m.status`）；不引入 nickname / petLevel 等长名（Swift 习惯 + ui_design 已是简短约定）。`status` 暂用 String 而非 enum —— mock 字面量"在玩耍/在散步/在休息"是节点 5 前临时占位，节点 5 后由 `HomePetState` 派生（避免预 over-engineer）。

**对应 Tasks**: Task 3.1

### AC4 — 新建 RoomScaffoldView struct + 5 区块视觉

**新建文件**: `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift`

**关键签名**（与 HomeView Story 37.7 同模式：`@ObservedObject var state: RoomViewModel` 基类直接，**不**泛型 state）：

```swift
public struct RoomScaffoldView: View {
    @ObservedObject public var state: RoomViewModel

    /// Story 37.5: 主题 token 取值入口；RootView 注入 `.environment(\.theme, currentTheme.theme)`.
    @Environment(\.theme) private var theme

    /// 复制按钮 1.2s 已复制视觉反馈状态（本地 @State，不进 ViewModel —— "Sheet 是否打开 / 倒计时秒数 → ViewModel 或 SwiftUI @State"
    /// ADR-0010 §3.2 表格 + Story 37.7 showJoinModal 反例的镜像决策：复制 feedback 是纯本地视觉 transient,
    /// 跨 View 触发场景不存在 → 走 @State 比走 ViewModel 更合理）.
    @State private var copiedFeedback: Bool = false

    /// 复制 feedback timer 句柄（防多次点击 race；与 HomeView resetTask 同模式）.
    @State private var copyFeedbackTask: Task<Void, Never>?

    public init(state: RoomViewModel) {
        self.state = state
    }

    public var body: some View {
        ZStack {
            // 背景渐变（room.jsx:14 钦定 linear-gradient(180deg, accent-soft 0%, page-bg 45%)）.
            LinearGradient(
                colors: [theme.colors.accentSoft, theme.colors.pageBg],
                startPoint: .top,
                endPoint: .bottom
            )
            .ignoresSafeArea()

            ScrollView {
                VStack(spacing: theme.spacing.s14) {
                    topBar               // 区块 1: 返回按钮 + 标题
                    roomCodeCard         // 区块 2: 房间号 + 复制按钮
                    sharedStage          // 区块 3: 共享舞台（粉橙渐变 + 装饰 + MiniCat 弹跳）
                    membersList          // 区块 4: 4 格成员列表
                    leaveButton          // 区块 5: 底部"离开房间" PrimaryButton
                }
                .padding(.horizontal, theme.spacing.s20)
                .padding(.top, 68)         // ui_design §iOS 设备规格: 状态栏 padding 68pt
                .padding(.bottom, 100)     // 浮动 TabBar 让出空间
            }
        }
    }
    // ... 5 区块子视图实现略（Dev Notes "5 区块视觉契约"详述每块视觉 + a11y + 颜色 / spacing 规则）
}
```

**5 区块要点**（详细视觉规则见 Dev Notes "5 区块视觉契约"；这里给关键定位锚）：

- **topBar**（room.jsx:18-31）：HStack（左：返回 IconButton 调 `state.onLeaveTap()`，accessibilityIdentifier `returnButton`，圆形 surface 背景 40x40 + sm shadow + chevron.left；中：VStack 11pt 700 "队伍房间" + 18pt 800 "{state.hostCatName}的小屋"；右：40pt 空白占位，留给 Story 35.2 share button），HStack 顶部 padding 4pt
- **roomCodeCard**（room.jsx:33-56）：Card（surface 背景 + 22pt 圆角 + sm shadow + border 1pt + 14pt padding）含 HStack space-between：左 VStack 11pt 700 "房间代码" + 22pt 800 monospace 3pt 字距 accent-deep `state.roomCodeForCopy`，accessibilityIdentifier `roomIdDisplay`；右 复制按钮（accessibilityIdentifier `copyButton`），点击调 `state.onCopyTap()` + 启动本地 `copiedFeedback` @State 1.2s 切到 success 绿底 + checkmark icon + "已复制" 文案
- **sharedStage**（room.jsx:58-100）：粉橙固定渐变 Card（fixed colors `#fff2e0 → #ffe0e9` linear-gradient 180deg + 28pt 圆角 + md shadow + border 1pt + minHeight 260pt + overflow clipped），含底部 50pt 棕色光晕装饰 / 4 个固定位 emoji（🧶/🐟/☁️x2，opacity 0.5-0.6）/ "X 只小猫在玩耍" inline pill (rgba(255,255,255,0.7) 背景 + 10pt 圆角)/HStack MiniCat 数组（成员数动态；错峰 0.2s 弹跳动画）—— accessibilityIdentifier `sharedStage`
- **membersList**（room.jsx:102-137）：VStack 13pt 800 "成员 ({members.count}/4)" 标题 + VStack(spacing:8) 成员行（HStack `Avatar(name: m.name, size: 40)` + VStack 14pt 800 名字（含 "队长" 小标签 if `m.isHost`）+ 11pt 600 "小猫 Lv.{m.level} · {m.status}" + Spacer + Icons.paw accent；Card surface 背景 + sm shadow + border + 10pt padding + 16pt 圆角；accessibilityIdentifier `roomMember_\(index)`）+ 空位 dashed border 占位行（h:60 + 2pt dashed border + 16pt 圆角 + center "+ 等待好友加入"，accessibilityIdentifier `roomMember_\(index)`）—— 所有 4 格按 index 0-3 编号
- **leaveButton**（room.jsx:139-147）：底部 `PrimaryButton(title: "离开房间", variant: .secondary, fullWidth: true) { state.onLeaveTap() }` + `.accessibilityIdentifier("leaveButton")`；marginTop 4pt

> **关键决策**：MiniCat 弹跳动画用 SwiftUI 简化版 `.scaleEffect(bouncing ? 1.0 : 0.94)` + `.animation(.easeInOut(duration: 1.1).repeatForever(autoreverses: true).delay(Double(index) * 0.2))`，不严格匹配 room.jsx `@keyframes bounce { 50% translateY(-6px) }` 细节；视觉精度由 Story 37.13 visual-review-checklist 把关（与 Story 37.7 floatUp 简化版同精神）。

> **关键决策**：复制 feedback 走 @State 而非 ViewModel —— ADR-0010 §3.2 表格"Loading / 倒计时秒数 → ViewModel 或 SwiftUI @State"二选一；本场景是纯本地视觉 transient（无跨 View 触发场景），用 @State 更轻；与 Story 37.7 showJoinModal 走 ViewModel 路径相反（showJoinModal 有跨 View 触发场景：FriendsView "加入"按钮也可能触发 HomeView 弹 modal），决策标准是"是否需要跨 View 触发"。

**对应 Tasks**: Task 4.1, 4.2, 4.3, 4.4, 4.5, 4.6

### AC5 — HomeContainerView caller 替换 + RootView wire

**改动文件 1**: `iphone/PetApp/Features/Home/Views/HomeContainerView.swift`

**关键改动**（line 28-31 inRoom 分支替换；caller 漏改靠编译器报错驱动）：

```swift
// 旧（Story 37.3 落地）
if HomeRoomDispatcher.shouldShowRoom(currentRoomId: appState.currentRoomId) {
    // inRoom 态：显示 RoomView 占位 stub（Story 37.8 实装真实内容）.
    RoomViewPlaceholder()
        .transition(.opacity)
} else { ... }

// 新（Story 37.8 落地）
if HomeRoomDispatcher.shouldShowRoom(currentRoomId: appState.currentRoomId) {
    // inRoom 态：渲染 RoomScaffoldView，state 由 RootView 通过 environmentObject 注入.
    HomeContainerRoomViewBridge()
        .transition(.opacity)
} else { ... }
```

**新增子视图**（与 `HomeContainerHomeViewBridge` 同模式；line 50-66 附近）：

```swift
/// HomeContainerView 内的 RoomScaffoldView 注入桥接子视图（与 HomeContainerHomeViewBridge 同模式）.
///
/// 为何抽出来：保 RoomViewModel 通过 EnvironmentObject 注入；与 HomeViewModel 注入路径同精神.
/// Story 12.1 落地时改用 RealRoomViewModel 替换基类（RootView wire 决定）.
private struct HomeContainerRoomViewBridge: View {
    @EnvironmentObject var roomViewModel: RoomViewModel

    var body: some View {
        RoomScaffoldView(state: roomViewModel)
    }
}
```

**改动文件 2**: `iphone/PetApp/App/RootView.swift`

**关键改动**：在 `@StateObject private var homeViewModel = HomeViewModel()` 同级新增 `@StateObject private var roomViewModel = RoomViewModel()` + 同级 `.environmentObject(roomViewModel)`：

```swift
// 旧（Story 37.4 落地）
@StateObject private var homeViewModel = HomeViewModel()
// ...
.environmentObject(homeViewModel)

// 新（Story 37.8 追加；homeViewModel 不动）
@StateObject private var homeViewModel = HomeViewModel()
@StateObject private var roomViewModel = RoomViewModel()
// ...
.environmentObject(homeViewModel)
.environmentObject(roomViewModel)
```

> **关键决策**：本 story **不**改 RootView 切到 RealRoomViewModel —— 基类 RoomViewModel 仍允许无参 init（与 HomeViewModel Story 37.7 wire 同精神）；Story 12.1 决定何时切到 RealRoomViewModel。Mock 路径走 Preview / UITest，不进 RootView wire。

> **caller 漏改靠编译器报错驱动**：HomeContainerView 内 `RoomViewPlaceholder()` 调用替换为 `HomeContainerRoomViewBridge()` —— 旧代码引用 `RoomViewPlaceholder` 类型本身保留（无 caller 引用，type 不删；保 git history）。**不依赖 grep 兜底**。

**对应 Tasks**: Task 5.1, 5.2, 5.3

### AC6 — #Preview 双主题（candy / dark）+ 多场景 mock

RoomScaffoldView 文件底部 `#if DEBUG ... #endif` 块含 4 个 Preview（双主题 × 满员/部分场景）：

```swift
#if DEBUG
#Preview("RoomScaffoldView — 4 members / candy") {
    RoomScaffoldView(state: MockRoomViewModel())
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("RoomScaffoldView — 4 members / dark") {
    RoomScaffoldView(state: MockRoomViewModel())
        .environment(\.theme, ThemeName.dark.theme)
}

#Preview("RoomScaffoldView — 2 members + 2 empty / candy") {
    RoomScaffoldView(state: MockRoomViewModel(members: MockRoomViewModel.twoMembersMock))
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("RoomScaffoldView — host alone / candy") {
    RoomScaffoldView(state: MockRoomViewModel(
        members: [RoomMember(id: "u1", name: "小花", level: 8, status: "在玩耍", isHost: true)]
    ))
    .environment(\.theme, ThemeName.candy.theme)
}
#endif
```

> **关键决策**：Preview 用 `#Preview` macro 而非 `PreviewProvider`（与 Story 37.5 / 37.6 / 37.7 同模式）；4 个 Preview 覆盖最少 1 / 2 / 4 成员 + 双主题视觉抽样。

**对应 Tasks**: Task 6.1

### AC7 — 单元测试覆盖（≥4 case，纯 XCTest + MockRoomViewModel + AppState）

**新建文件**: `iphone/PetAppTests/Features/Room/RoomViewScaffoldTests.swift`

落地以下 5 case（≥4 case 按 epic AC line 4764-4768；额外 +1 给 RealRoomViewModel 构造路径稳定性更稳）：

```swift
// RoomViewScaffoldTests.swift
// Story 37.8 AC7: RoomScaffoldView + RoomViewModel class 层次单元测试.
//
// 测试基础设施约束（与 Story 2.7 + ADR-0002 §3.1 衔接）：
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）.
//   - 不引 ViewInspector / SnapshotTesting.
//   - 走 ViewModel 行为 + invocations 数组断言；不走 SwiftUI body 内省.

import XCTest
@testable import PetApp

@MainActor
final class RoomViewScaffoldTests: XCTestCase {

    // MARK: - case#1 happy: MockRoomViewModel 默认 4 成员状态

    /// 验证 MockRoomViewModel 默认值与 Story 37.8 spec 一致（roomCode / hostCatName / 4 成员 / userIsHost）.
    /// 对应 epic AC line 4765 "happy: 注入 mock 4 成员 → View 渲染 4 格无占位".
    /// （视觉断言由 #Preview + UITest 兜底；本测试断言 ViewModel 数据契约）.
    func testMockRoomViewModelDefaultStateMatchesSpec() {
        let vm = MockRoomViewModel()
        XCTAssertEqual(vm.roomCodeForCopy, "1234567")
        XCTAssertEqual(vm.hostCatName, "小花")
        XCTAssertEqual(vm.members.count, 4)
        XCTAssertEqual(vm.members[0].name, "小花")
        XCTAssertTrue(vm.members[0].isHost)
        XCTAssertFalse(vm.userIsHost)
        XCTAssertEqual(vm.invocations, [])
    }

    // MARK: - case#2 happy: 2 成员场景注入 → members.count = 2（驱动 View 渲染 2 + 2 占位）

    /// 验证可注入任意 members 数（mock 可配场景）.
    /// 对应 epic AC line 4766 "happy: 注入 mock 2 成员 → View 渲染 2 实 + 2 虚线占位".
    /// 占位 dashed border 的视觉断言由 #Preview + UITest 兜底.
    func testMockRoomViewModelTwoMembersScenario() {
        let vm = MockRoomViewModel(members: MockRoomViewModel.twoMembersMock)
        XCTAssertEqual(vm.members.count, 2)
        XCTAssertEqual(vm.members[0].name, "小花")
        XCTAssertTrue(vm.members[0].isHost)
        XCTAssertEqual(vm.members[1].name, "Mocha")
        XCTAssertFalse(vm.members[1].isHost)
    }

    // MARK: - case#3 happy: 点击复制按钮 → onCopyTap 触发 + invocations 含 .copyTap

    /// 验证 onCopyTap 调用后 invocations 含 .copyTap.
    /// 对应 epic AC line 4767 "happy: 点击复制按钮 → onCopyTap 触发 + UI 显示绿色对勾 1.2s".
    /// （UI 1.2s feedback 由 RoomScaffoldView 内 @State 控制；视觉断言由 UITest case 测点击后 a11y 状态）.
    func testOnCopyTapAppendsInvocation() {
        let vm = MockRoomViewModel()
        vm.onCopyTap()
        XCTAssertEqual(vm.invocations, [.copyTap])
    }

    // MARK: - case#4 happy: 点击离开 → onLeaveTap 触发

    /// 验证 onLeaveTap 调用后 invocations 含 .leaveTap.
    /// 对应 epic AC line 4768 "happy: 点击离开 → onLeaveTap 触发".
    func testOnLeaveTapAppendsInvocation() {
        let vm = MockRoomViewModel()
        vm.onLeaveTap()
        XCTAssertEqual(vm.invocations, [.leaveTap])
    }

    // MARK: - case#5 happy: RealRoomViewModel 构造注入 AppState 不 crash

    /// 验证 RealRoomViewModel(appState:) 构造正常 + override 方法可调用（不触发 fatalError 路径）.
    /// 防止 RealRoomViewModel.onLeaveTap 等忘记 override 时本测试立刻 fail（fatalError 在测试中 → trap）.
    /// onLeaveTap 调 appState.setCurrentRoomId(nil) → 验证 appState.currentRoomId == nil 作为 override 路径已执行的代理证据.
    func testRealRoomViewModelConstructionAndAbstractMethodsDoNotCrash() {
        let appState = AppState()
        appState.setCurrentRoomId("room_1234567")
        let vm = RealRoomViewModel(appState: appState)
        XCTAssertEqual(vm.roomCodeForCopy, "room_1234567")
        // 调用 2 个 override 方法验证不进入基类 fatalError 路径.
        vm.onCopyTap()      // 仅 log，不改 state
        vm.onLeaveTap()     // 调 appState.setCurrentRoomId(nil)
        XCTAssertNil(appState.currentRoomId, "onLeaveTap 应通过 appState 写 nil 触发互斥状态机切回 idle")
    }
}
```

> **关键决策**：与 Story 37.7 HomeViewScaffoldTests 同模式 —— 不走 `UIHostingController` 渲染 SwiftUI body；ADR-0002 §3.1 钦定 XCTest only + ViewModel 行为可独立断言；视觉断言由 #Preview + UITest a11y identifier 兜底。

> **不**测 RoomScaffoldView body 渲染含 a11y identifier（属 UITest 范围；详见 AC8 + Dev Notes "测试边界"）。

> **不**测 fatalError 路径（基类 abstract method 覆盖在 case#5 间接证明 override 已生效；显式 fatalError trap 测试 ADR-0002 §3.1 不强制）。

**对应 Tasks**: Task 7.1

### AC8 — UITest a11y identifier 7 锚可定位

**改动文件**: `iphone/PetAppUITests/HomeUITests.swift`（或新建 `RoomUITests.swift` —— 见下方决策）

**决策**：本 story 加一个 **新 test case** 在 HomeUITests.swift 内（保 Story 37.7 testHomeScaffoldShowsAllSevenAnchors 同位置；Story 37.13 a11y 总表归并时统一移走）：

```swift
// Story 37.8: RoomScaffoldView 7 锚 a11y identifier 可定位验证.
// 通过 launch env `UITEST_FORCE_IN_ROOM=1` 让 RootView/HomeContainerView 启动即切到 inRoom 态.
// 与 Story 37.7 testHomeScaffoldShowsAllSevenAnchors 同模式；本 test 验证 ui_design 高保真 5 区块各 a11y 锚.
func testRoomScaffoldShowsAllSevenAnchors() throws {
    let app = XCUIApplication()
    app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
    app.launchEnvironment["UITEST_FORCE_IN_ROOM"] = "1"   // 新增 env flag；详见 Dev Notes "UITest force-in-room flag"
    app.launch()

    XCTAssertTrue(app.buttons["returnButton"].waitForExistence(timeout: 5))
    XCTAssertTrue(app.staticTexts["roomIdDisplay"].exists)
    XCTAssertTrue(app.buttons["copyButton"].exists)
    XCTAssertTrue(app.otherElements["roomMember_0"].exists)
    XCTAssertTrue(app.otherElements["roomMember_1"].exists)
    XCTAssertTrue(app.otherElements["roomMember_2"].exists)
    XCTAssertTrue(app.otherElements["roomMember_3"].exists)
    XCTAssertTrue(app.buttons["leaveButton"].exists)
}
```

**改动文件 2**: `iphone/PetApp/App/RootView.swift` 或 `AppCoordinator.swift` —— 加 `UITEST_FORCE_IN_ROOM` env flag 处理：

```swift
// Story 37.8 AC8: UITest 路径强制切到 inRoom 态（避免依赖 mock server 的 join flow）.
// 仅 Debug build 生效；Production build 此 env 被忽略.
#if DEBUG
if ProcessInfo.processInfo.environment["UITEST_FORCE_IN_ROOM"] == "1" {
    appState.setCurrentRoomId("1234567")
}
#endif
```

> **关键决策**：UITest 不主动点击按钮 / 验证退出 / 复制功能链路（属 Story 12.x 范围）；仅验证视觉锚存在（让 Story 37.13 a11y 总表归并时有 baseline）。

> **关键决策**：`UITEST_FORCE_IN_ROOM` env flag 比"在 UITest 内手动模拟 join flow"更稳 —— 本 story RealRoomViewModel 的 join flow 还没接（Story 12.1 落地），强行点 join 按钮会 segfault。env flag 让 UITest 直接切到 inRoom 态走 mock 数据（基类 RoomViewModel 默认空 members 不渲染足量 a11y 锚 → 必须用 MockRoomViewModel 注入路径，详见 Dev Notes "UITest force-in-room flag"）。

> **现有 testHomeScaffoldShowsAllSevenAnchors / testHomeViewShowsAllSixPlaceholders**（Story 37.7 / 2.2）**不动**——本 story 范围是 Room Tab，不影响 Home Tab UITest。

**对应 Tasks**: Task 8.1, 8.2, 8.3

### AC9 — xcodegen regen + build verify

完成 AC1-AC8 后：

1. `cd iphone && xcodegen generate` 让新文件加入 PetApp / PetAppTests target（project.yml `sources: - PetApp` / `- PetAppTests` 通配规则自动 inclusion；新增 `Features/Room/ViewModels/RoomViewModel.swift` + `Features/Room/ViewModels/MockRoomViewModel.swift` + `Features/Room/ViewModels/RealRoomViewModel.swift` + `Features/Room/Models/RoomMember.swift` + `Features/Room/Views/RoomScaffoldView.swift` + `PetAppTests/Features/Room/RoomViewScaffoldTests.swift`）
2. `bash iphone/scripts/build.sh --test` 跑测试通过
   - 总 case 数：~272（Story 37.7 落地后基线 ~272）+ 本 story 新增 5 unit case + 1 UITest case → ~278 case 全绿
   - 不删除任何老 case
3. grep 验证：
   - `grep -c "class RoomViewModel" iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift` ≥ 1（防漏建基类）
   - `grep -c "fatalError" iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift` ≥ 2（2 个 abstract method 的 fatalError 占位）
   - `grep "final class RoomViewModel" iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift` 输出空（基类不能 final）
   - `grep -c "override func" iphone/PetApp/Features/Room/ViewModels/MockRoomViewModel.swift` ≥ 2（2 abstract method override）
   - `grep -c "override func" iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` ≥ 2
   - `grep -c "RoomScaffoldView" iphone/PetApp/Features/Home/Views/HomeContainerView.swift` ≥ 1（caller 替换已生效）
   - `grep -c "RoomViewPlaceholder()" iphone/PetApp/Features/Home/Views/HomeContainerView.swift` 输出 0（旧调用已替换）

> **dev 实装备注**：dev 必须 `xcodegen generate` 后 commit 一并提交 `iphone/PetApp.xcodeproj/project.pbxproj` 改动（与 Story 37.5 / 37.6 / 37.7 同模式）。

**对应 Tasks**: Task 9.1, 9.2, 9.3

### AC10 — Deliverable 清单

- ✅ `iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift` 新建（class + 4 字段 + 2 abstract method fatalError 占位 + parameterless init）
- ✅ `iphone/PetApp/Features/Room/ViewModels/MockRoomViewModel.swift` 新建（final + invocations + 默认 4 成员 mock + 可注入构造）
- ✅ `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` 新建（final + appState 构造注入 + parameterless init + 2 override 占位 stub + onLeaveTap 调 appState.setCurrentRoomId(nil)）
- ✅ `iphone/PetApp/Features/Room/Models/RoomMember.swift` 新建（id/name/level/status/isHost + Equatable + Identifiable + Sendable）
- ✅ `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift` 新建（generic struct 否；签名 `state: RoomViewModel` 基类直接 + 5 区块视觉按 ui_design room.jsx 像素级翻译 + #Preview 4 配置 candy/dark x 4/2 成员）
- ✅ `iphone/PetAppTests/Features/Room/RoomViewScaffoldTests.swift` 新建（5 case：Mock 默认状态 / 2 成员场景 / onCopyTap / onLeaveTap / RealRoomViewModel 构造）
- ✅ `iphone/PetApp/Features/Home/Views/HomeContainerView.swift` 修改（inRoom 分支 `RoomViewPlaceholder()` → `HomeContainerRoomViewBridge()` + 新增 bridge 子视图）
- ✅ `iphone/PetApp/App/RootView.swift` 修改（追加 `@StateObject roomViewModel = RoomViewModel()` + `.environmentObject(roomViewModel)`）
- ✅ `iphone/PetApp/App/RootView.swift` 或 `AppCoordinator.swift` 加 `UITEST_FORCE_IN_ROOM` env flag 处理（仅 Debug build）
- ✅ `iphone/PetAppUITests/HomeUITests.swift` 加 `testRoomScaffoldShowsAllSevenAnchors`（使用 `UITEST_FORCE_IN_ROOM` env）
- ✅ `iphone/PetApp.xcodeproj/project.pbxproj` 改动随 commit（xcodegen regen 结果）
- ✅ `bash iphone/scripts/build.sh --test` 全绿（~278 case）
- ✅ project.yml **不**手动改（通配规则自动 inclusion）
- ✅ RootView wire 不改 RealRoomViewModel（Story 12.1 决定切换时机）
- ✅ HomeRoomDispatcher / HomeContainerView 互斥状态机决策不动
- ✅ `RoomViewPlaceholder.swift` **不**删除（保 git history；Story 37.13 决定）

## Tasks / Subtasks

- [x] Task 1: RoomViewModel 基类（AC1）
  - [x] 1.1 新建 `iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift`：`@MainActor public class RoomViewModel: ObservableObject` + 4 个 @Published 字段（roomCodeForCopy / hostCatName / members / userIsHost）+ 2 abstract method fatalError 占位 + parameterless init()
  - [x] 1.2 显式 `import Foundation` + `import Combine`（防 transitive @Published；与 MockHomeViewModel round 4 [P0] hardening 同精神）
- [x] Task 2: Mock/Real 子类（AC2）
  - [x] 2.1 新建 `iphone/PetApp/Features/Room/ViewModels/MockRoomViewModel.swift`（final class + invocations 数组 + 2 override + 默认 4 成员 mock + 可配 init + fourMembersMock / twoMembersMock 静态属性）
  - [x] 2.2 新建 `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`（final class + appState 构造注入 + parameterless init() / init(appState:) 双路径 + bind(appState:) + 2 override 占位 stub + onLeaveTap 调 appState.setCurrentRoomId(nil)）
- [x] Task 3: RoomMember 值类型（AC3）
  - [x] 3.1 新建 `iphone/PetApp/Features/Room/Models/RoomMember.swift`（id/name/level/status/isHost + Equatable + Identifiable + Sendable）
- [x] Task 4: RoomScaffoldView struct + 5 区块（AC4）
  - [x] 4.1 新建 `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift`，含 ZStack 背景渐变 + ScrollView VStack 5 区块结构
  - [x] 4.2 落地 topBar 子视图（返回按钮 IconButton 调 onLeaveTap + 中标题 11pt+18pt + 右 40pt 空白占位）
  - [x] 4.3 落地 roomCodeCard 子视图（Card surface 22pt 圆角 + 左 VStack 房间号 monospace 3pt 字距 + 右 复制按钮 含 1.2s feedback @State + onCopyTap 调用 + UIPasteboard.general.string = state.roomCodeForCopy）
  - [x] 4.4 落地 sharedStage 子视图（粉橙固定渐变 Card 28pt 圆角 + 装饰 emoji 4 个 + "X 只小猫在玩耍" pill + MiniCat HStack 错峰 0.2s 弹跳动画）
  - [x] 4.5 落地 membersList 子视图（标题 + members 行 + 空位 dashed border 行；按 index 0-3 编号 a11y identifier roomMember_0..3）
  - [x] 4.6 落地 leaveButton 子视图（PrimaryButton variant: .secondary fullWidth: true 调 onLeaveTap）
  - [x] 4.7 7 个 a11y identifier inline 字符串：returnButton / roomIdDisplay / copyButton / roomMember_0..3 / leaveButton + sharedStage（额外锚，便于视觉抽样）
- [x] Task 5: HomeContainerView caller 替换 + RootView wire（AC5）
  - [x] 5.1 改 `HomeContainerView.swift` line 28-31：inRoom 分支 `RoomViewPlaceholder()` → `HomeContainerRoomViewBridge()`；新增 fileprivate `HomeContainerRoomViewBridge` 子视图（@EnvironmentObject roomViewModel: RoomViewModel + body: RoomScaffoldView(state: roomViewModel)）
  - [x] 5.2 改 `RootView.swift` 在 `homeViewModel` 同级新增 `@StateObject private var roomViewModel = RoomViewModel()` + `.environmentObject(roomViewModel)`
  - [x] 5.3 RootView 或 AppCoordinator 加 `UITEST_FORCE_IN_ROOM` env flag 处理（仅 Debug build；启动时 appState.setCurrentRoomId("1234567")）
- [x] Task 6: #Preview 4 配置（AC6）
  - [x] 6.1 RoomScaffoldView 文件底部 `#if DEBUG` 块加 4 个 `#Preview`（candy 4/2/1 成员 + dark 4 成员）
- [x] Task 7: 单元测试（AC7）
  - [x] 7.1 新建 `iphone/PetAppTests/Features/Room/RoomViewScaffoldTests.swift`，落地 5 case（Mock 默认状态 / 2 成员场景 / onCopyTap / onLeaveTap / RealRoomViewModel 构造）
- [x] Task 8: UITest（AC8）
  - [x] 8.1 在 `HomeUITests.swift` 加 `testRoomScaffoldShowsAllSevenAnchors`（使用 `UITEST_FORCE_IN_ROOM` env）
  - [x] 8.2 验证现有 `testHomeScaffoldShowsAllSevenAnchors` / `testHomeViewShowsAllSixPlaceholders` 不受影响（不动）
- [x] Task 9: xcodegen regen + build verify（AC9）
  - [x] 9.1 `cd iphone && xcodegen generate` 让 6 个新文件加入 target
  - [x] 9.2 `bash iphone/scripts/build.sh --test` 跑测试通过（~278 case 全绿）
  - [x] 9.3 grep 校验：RoomViewModel 含 `class RoomViewModel`（去 final）+ 2 个 fatalError；MockRoomViewModel / RealRoomViewModel 各含 2 个 override func；HomeContainerView 含 RoomScaffoldView 调用 + 不含 `RoomViewPlaceholder()` 调用
- [x] Task 10: Deliverable 清单确认（AC10）
  - [x] 10.1 6 个新文件 + 修改 3 个老文件（HomeContainerView.swift / RootView.swift / HomeUITests.swift）+ pbxproj regen 全部待 commit（不在本 dev-story 范围）

## Dev Notes

### RoomViewModel 改 class 而非 protocol any 模式（关键设计决策）

**选定**: 基类 `class RoomViewModel: ObservableObject`（非 final）+ 子类 `MockRoomViewModel: RoomViewModel` / `RealRoomViewModel: RoomViewModel` 各自 final。

**为何不走 `protocol RoomViewModelProtocol + any P`**：与 Story 37.7 HomeViewModel 同精神（v2.1 BLOCKER 7）—— SwiftUI `@ObservedObject` 不接受 `any P`；`HomeView<ChestSlot: View>` 模式让 caller 端类型膨胀。RoomScaffoldView 选择**非泛型 struct**（state: RoomViewModel 基类直接），更简洁——Room 没有"chestSlot 接缝点"这种泛型必要场景；若未来 Story 12.1 / 35.2 加 share button slot，再走泛型 ViewBuilder 路径。

### RoomViewPlaceholder 不删保 git history（关键约束）

`RoomViewPlaceholder.swift` 在 Story 37.3 落地时作为 inRoom 态占位 stub；本 story **不删除**该文件，理由：
1. **保 git history 可读**：dev 阅读 git log 能看到 `RoomViewPlaceholder` 是 Story 37.3 临时方案 → Story 37.8 替换为 RoomScaffoldView 的演进足迹；删除会让 git blame 失去线索。
2. **避免 caller 漏改判定模糊**：若删除 RoomViewPlaceholder.swift 同时改 HomeContainerView caller，编译错误消息会是"Cannot find type RoomViewPlaceholder"——这种错误信息让 reviewer 误以为是文件被误删；保留 type 但替换 caller 让编译错误明确指向"caller 还在用旧 type 但你已切到 RoomScaffoldView"。
3. **Story 37.13 a11y 总表归并时统一清理**：RoomViewPlaceholder 内的 `roomViewPlaceholder` a11y identifier 在 Story 37.13 范围；本 story 不收口。

> **关键决策**：本 story 范围是 caller 替换 + 新文件落地；**不**做任何旧文件删除（与 Story 37.7 删除老 PreviewProvider 的部分重写不同 —— 那是 HomeView 文件内 inline 内容；本 story 是独立文件层级处理）。

### state owner 边界：复制 feedback 走 @State 而非 ViewModel（与 showJoinModal 反向决策）

ADR-0010 §3.2 表格"Loading / 倒计时秒数 → ViewModel 或 SwiftUI @State"二选一；判断标准是**是否需要跨 View 触发**：

| 场景 | 选择 | 理由 |
|---|---|---|
| `showJoinModal`（Story 37.7） | ViewModel @Published | FriendsView "加入"按钮也可能让 HomeView 弹 modal（v2 提案 Story 37.12）—— 跨 View 触发只能通过共享 ObservableObject |
| `copiedFeedback`（本 story） | View @State | 仅 RoomScaffoldView 内"复制按钮 → 1.2s 视觉切到绿对勾"——纯本地视觉 transient，无跨 View 触发 |

**反例**：若把 copiedFeedback 放进 RoomViewModel @Published，会让 ViewModel 被 SwiftUI Preview 测试 / 单元测试不必要地依赖（每改 1.2s timer 都要改 mock）；放 @State 让 timer 逻辑封闭在 RoomScaffoldView 内，单元测试只测 `onCopyTap` 触发即可。

### RealRoomViewModel.onLeaveTap 调 appState.setCurrentRoomId(nil) 的依据

依据 Story 37.3 落地的互斥状态机数据流：`HomeRoomDispatcher.shouldShowRoom(currentRoomId:)` 返回 false（nil）时，HomeContainerView 自动切回 idle 态渲染 HomeView。RealRoomViewModel.onLeaveTap **不**调真实 LeaveRoomUseCase（属 Story 12.7 范围），仅写 appState 让 UI 立刻切回 idle —— 这是 Epic 37 Scaffold 红线"数据完全 mock + 禁 import APIClient/Repository/UseCase"的合规路径。

> **关键决策**：onLeaveTap 不通过 closure 让 RoomScaffoldView 内 SwiftUI dismiss / NavigationStack pop —— 因为 HomeContainerView 互斥状态机用的是数据驱动（appState.currentRoomId nil/non-nil → ZStack 切换）而非导航栈；切回 idle 态由 SwiftUI .animation 自动接管。

### UITest force-in-room flag 的设计

UITest 验证 RoomScaffoldView 7 锚需要把 App 启动后切到 inRoom 态。三种路径：

1. **手动模拟 join flow**：UITest 内点 HomeView "创建队伍"按钮 → 等 RealRoomViewModel.onCreateTap 执行 → ... 但本 story RealRoomViewModel.onCreateTap 是 stub（没接 CreateRoomUseCase）→ 流程不通。
2. **launch arg 注入 mock RoomViewModel**：在 RootView 内根据 env 切到 MockRoomViewModel —— 但需要改生产代码 wire 路径，污染面大。
3. **launch env 直接 set appState.currentRoomId**：在 RootView Debug 启动 hook 内识别 `UITEST_FORCE_IN_ROOM=1` 调 `appState.setCurrentRoomId("1234567")` —— 复用 Story 37.4 setCurrentRoomId 入口；不污染生产 wire；UITest 跑完 reset env 即恢复正常。

**选定**: 路径 3。

> **关键决策**：env flag 名 `UITEST_FORCE_IN_ROOM` —— 与现有 `UITEST_SKIP_GUEST_LOGIN` 同前缀（Story 2.2 落地的 UITest env flag 命名风格），便于 grep 找到所有 UITest hook 集中清理（Story 37.13 / 12.x 范围）。

> **关键决策**：env flag 让 RootView 走的是基类 RoomViewModel（带空 members）→ RoomScaffoldView 渲染 0 实成员 + 4 占位行；7 锚仍可定位（returnButton / roomIdDisplay / copyButton / roomMember_0..3 / leaveButton 都存在）。**不**额外注入 MockRoomViewModel mock 数据 —— 让 UITest 走最小路径覆盖 a11y 锚定位即可（视觉精度由 #Preview 兜底）。

### 5 区块视觉契约（详细 ui_design 翻译表）

按 `iphone/ui_design/source/screens/room.jsx` + `iphone/ui_design/README.md` §RoomScreen 像素级翻译：

#### topBar（room.jsx:18-31）

```swift
HStack(alignment: .center) {
    // 左：返回按钮（圆形 IconButton）
    Button(action: { state.onLeaveTap() }) {
        Image(systemName: Icons.symbol(for: "back"))
            .font(.system(size: 20, weight: .semibold))
            .foregroundColor(theme.colors.ink)
            .frame(width: 40, height: 40)
            .background(Circle().fill(theme.colors.surface))
            .overlay(Circle().stroke(theme.colors.border, lineWidth: 1))
            .shadow(color: theme.shadow.sm.color, radius: theme.shadow.sm.radius, x: theme.shadow.sm.x, y: theme.shadow.sm.y)
    }
    .accessibilityIdentifier("returnButton")

    Spacer()

    // 中：标题 VStack
    VStack(spacing: 0) {
        Text("队伍房间")
            .font(.system(size: 11, weight: .bold))
            .foregroundColor(theme.colors.inkSoft)
            .tracking(0.5)
        Text("\(state.hostCatName)的小屋")
            .font(.system(size: 18, weight: .heavy))
            .foregroundColor(theme.colors.ink)
    }

    Spacer()

    // 右：40pt 空白占位（Story 35.2 share button 占位）
    Color.clear.frame(width: 40, height: 40)
}
.padding(.top, 4)
```

> **关键**：returnButton 调 `state.onLeaveTap()` 与 leaveButton 共享同一回调 —— ui_design room.jsx 钦定（line 20-25 + 140-145 同 onLeave）。

#### roomCodeCard（room.jsx:33-56）

```swift
Card(cornerRadius: 22, padding: 14) {
    HStack {
        VStack(alignment: .leading, spacing: 2) {
            Text("房间代码")
                .font(.system(size: 11, weight: .bold))
                .foregroundColor(theme.colors.inkSoft)
            Text(state.roomCodeForCopy)
                .font(.system(size: 22, weight: .heavy, design: .monospaced))
                .tracking(3)
                .foregroundColor(theme.colors.accentDeep)
                .accessibilityIdentifier("roomIdDisplay")
        }
        Spacer()
        copyButton  // 见下方
    }
}
```

复制按钮：

```swift
private var copyButton: some View {
    Button(action: {
        UIPasteboard.general.string = state.roomCodeForCopy
        state.onCopyTap()
        // 1.2s feedback timer（与 HomeView resetTask 同模式 cancel 上一个 task 防 race）
        copyFeedbackTask?.cancel()
        copiedFeedback = true
        copyFeedbackTask = Task { @MainActor in
            try? await Task.sleep(nanoseconds: 1_200_000_000)
            if Task.isCancelled { return }
            copiedFeedback = false
        }
    }) {
        HStack(spacing: 6) {
            Image(systemName: copiedFeedback ? Icons.symbol(for: "check") : Icons.symbol(for: "copy"))
                .font(.system(size: 16, weight: .semibold))
            Text(copiedFeedback ? "已复制" : "复制")
                .font(.system(size: 12, weight: .heavy))
        }
        .foregroundColor(copiedFeedback ? .white : theme.colors.accentDeep)
        .padding(.vertical, 10)
        .padding(.horizontal, 14)
        .background(
            RoundedRectangle(cornerRadius: 16)
                .fill(copiedFeedback ? theme.colors.success : theme.colors.accentSoft)
        )
        .animation(.easeInOut(duration: 0.2), value: copiedFeedback)
    }
    .accessibilityIdentifier("copyButton")
}
```

> **关键决策**：UIPasteboard 调用直接在 SwiftUI Button action 内 —— ViewModel 层不依赖 UIKit（Epic 37 红线）。`state.onCopyTap()` 仅记录 invocation / log；实际复制行为是 UI 层职责。

#### sharedStage（room.jsx:58-100）

```swift
ZStack(alignment: .topLeading) {
    // 粉橙固定渐变（fixed colors，不走 theme — ui_design room.jsx:60 钦定）
    LinearGradient(
        colors: [
            Color(red: 1.00, green: 0.95, blue: 0.88),     // #fff2e0
            Color(red: 1.00, green: 0.88, blue: 0.91),     // #ffe0e9
        ],
        startPoint: .top,
        endPoint: .bottom
    )

    // 底部 50pt 棕色光晕装饰（rgba(218,165,132,0→0.25) 渐变）
    VStack {
        Spacer()
        LinearGradient(
            colors: [Color(red: 0.85, green: 0.65, blue: 0.52, opacity: 0), Color(red: 0.85, green: 0.65, blue: 0.52, opacity: 0.25)],
            startPoint: .top,
            endPoint: .bottom
        )
        .frame(height: 50)
    }

    // 4 个固定位 emoji 装饰
    Text("🧶").font(.system(size: 32)).position(x: UIScreen.main.bounds.width - 60, y: 220)
    Text("🐟").font(.system(size: 28)).position(x: 40, y: 220)
    Text("☁️").font(.system(size: 22)).opacity(0.6).position(x: UIScreen.main.bounds.width - 50, y: 30)
    Text("☁️").font(.system(size: 18)).opacity(0.5).position(x: 60, y: 50)

    // 主内容 VStack
    VStack(alignment: .leading, spacing: 10) {
        // "X 只小猫在玩耍" pill
        Text("\(state.members.count) 只小猫在玩耍")
            .font(.system(size: 11, weight: .bold))
            .foregroundColor(theme.colors.inkSoft)
            .padding(.vertical, 4)
            .padding(.horizontal, 10)
            .background(RoundedRectangle(cornerRadius: 10).fill(Color.white.opacity(0.7)))

        // MiniCat HStack
        HStack(alignment: .bottom, spacing: 8) {
            ForEach(Array(state.members.enumerated()), id: \.element.id) { index, member in
                miniCat(member: member, index: index)
            }
        }
        .padding(.vertical, 14)
        .frame(maxWidth: .infinity)
    }
    .padding(.horizontal, 14)
    .padding(.vertical, 16)
}
.frame(minHeight: 260)
.clipShape(RoundedRectangle(cornerRadius: 28))
.overlay(RoundedRectangle(cornerRadius: 28).stroke(theme.colors.border, lineWidth: 1))
.shadow(color: theme.shadow.md.color, radius: theme.shadow.md.radius, x: theme.shadow.md.x, y: theme.shadow.md.y)
.accessibilityIdentifier("sharedStage")

// MiniCat 子视图：4 色调色板 hash + 错峰弹跳
private func miniCat(member: RoomMember, index: Int) -> some View {
    VStack(spacing: 4) {
        // 占位 cat 圆形（Story 30.x 接真实 sprite 时替换）
        Circle()
            .fill([
                Color(red: 1.00, green: 0.84, blue: 0.87),  // #ffd6df
                Color(red: 0.87, green: 0.91, blue: 0.78),  // #dfe8c8
                Color(red: 0.81, green: 0.89, blue: 0.95),  // #cfe2f2
                Color(red: 0.96, green: 0.83, blue: 0.65),  // #f5d4a6
            ][index % 4])
            .frame(width: 68, height: 68)
            .overlay(
                Image(systemName: "cat.fill")
                    .font(.system(size: 28))
                    .foregroundColor(theme.colors.inkSoft)
            )
        Text(member.name)
            .font(.system(size: 10, weight: .bold))
            .foregroundColor(theme.colors.ink)
    }
    .scaleEffect(/* 错峰弹跳；用 @State or .symbolEffect */)
    // 简化版：用 phaseAnimator 或 .repeatForever() ；详见 Tasks 4.4
}
```

> **关键决策**：粉橙渐变 `#fff2e0 → #ffe0e9` 和 4 色 MiniCat 调色板**不走 theme** —— ui_design room.jsx 钦定 fixed colors（与"主题"无关，是房间舞台的固定视觉品牌）；与 Story 37.5 三主题 stub（matcha / sky / dark）演进时需评估，本 story 不预先抽 token。

#### membersList（room.jsx:102-137）

```swift
VStack(alignment: .leading, spacing: 8) {
    Text("成员 (\(state.members.count)/4)")
        .font(.system(size: 13, weight: .heavy))
        .foregroundColor(theme.colors.ink)
        .padding(.horizontal, 4)

    VStack(spacing: 8) {
        ForEach(Array(state.members.enumerated()), id: \.element.id) { index, member in
            memberRow(member: member, index: index)
        }
        // 空位（4 - members.count 个）
        ForEach(state.members.count..<4, id: \.self) { index in
            emptySlot(index: index)
        }
    }
}

// 已加入成员行
private func memberRow(member: RoomMember, index: Int) -> some View {
    HStack(spacing: 12) {
        Avatar(name: member.name, size: 40)
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 4) {
                Text(member.name)
                    .font(.system(size: 14, weight: .heavy))
                    .foregroundColor(theme.colors.ink)
                if member.isHost {
                    Text("队长")
                        .font(.system(size: 10, weight: .heavy))
                        .foregroundColor(theme.colors.accentDeep)
                        .padding(.vertical, 2)
                        .padding(.horizontal, 6)
                        .background(RoundedRectangle(cornerRadius: 8).fill(theme.colors.accentSoft))
                }
            }
            Text("小猫 Lv.\(member.level) · \(member.status)")
                .font(.system(size: 11, weight: .semibold))
                .foregroundColor(theme.colors.inkSoft)
        }
        Spacer()
        Image(systemName: Icons.symbol(for: "paw"))
            .font(.system(size: 16))
            .foregroundColor(theme.colors.accent)
    }
    .padding(10)
    .background(RoundedRectangle(cornerRadius: 16).fill(theme.colors.surface))
    .overlay(RoundedRectangle(cornerRadius: 16).stroke(theme.colors.border, lineWidth: 1))
    .shadow(color: theme.shadow.sm.color, radius: theme.shadow.sm.radius, x: theme.shadow.sm.x, y: theme.shadow.sm.y)
    .accessibilityIdentifier("roomMember_\(index)")
}

// 空位 dashed border 行
private func emptySlot(index: Int) -> some View {
    HStack {
        Text("+ 等待好友加入")
            .font(.system(size: 13, weight: .heavy))
            .foregroundColor(theme.colors.inkSoft.opacity(0.6))
    }
    .frame(maxWidth: .infinity)
    .frame(height: 60)
    .overlay(
        RoundedRectangle(cornerRadius: 16)
            .stroke(style: StrokeStyle(lineWidth: 2, dash: [6, 4]))
            .foregroundColor(theme.colors.border)
    )
    .accessibilityIdentifier("roomMember_\(index)")
}
```

> **关键**：a11y identifier 按 index 0-3 顺序编号（不论是真实成员还是占位），让 UITest case `roomMember_0..3` 全部命中（4 个 4 锚）。

#### leaveButton（room.jsx:139-147）

```swift
PrimaryButton(
    title: "离开房间",
    variant: .secondary,
    fullWidth: true
) {
    state.onLeaveTap()
}
.accessibilityIdentifier("leaveButton")
.padding(.top, 4)
```

> **关键决策**：用 `PrimaryButton` 现有 primitive（Story 37.6 落地）+ secondary variant —— 与 home.jsx TeamIdleCard "创建队伍" 同 variant 选择路径；不为本 story 扩 PrimaryButton variant。

### 测试边界（XCTest only）

本 story 测试**仅**用 XCTest + @testable import PetApp + UITest（XCUIApplication）—— **不**引：

- ❌ SnapshotTesting（视觉 diff）：视觉验证靠 #Preview + Story 37.13 visual-review-checklist
- ❌ ViewInspector（SwiftUI body 内省）：RoomScaffoldView body 渲染契约靠 ui_design 1:1 翻译 + Preview 抽样兜底
- ❌ Mockingbird / Cuckoo（mock codegen）：MockRoomViewModel 是手写 final class subclass

### Story 37.7 衔接：与 HomeView 同 patterns 全表

| 维度 | HomeView (Story 37.7) | RoomScaffoldView (本 story) |
|---|---|---|
| 文件命名 | `HomeView.swift` (改写) | `RoomScaffoldView.swift` (新建)（注：旧 `RoomViewPlaceholder.swift` 不动） |
| struct 签名 | `HomeView<ChestSlot: View>` 泛型 | `RoomScaffoldView` 非泛型 (Room 无 chestSlot 类似接缝) |
| state owner | `@ObservedObject var state: HomeViewModel` 基类 | `@ObservedObject var state: RoomViewModel` 基类 |
| ViewModel 基类 | `class HomeViewModel`（去 final）+ 5 字段 + 5 abstract method | `class RoomViewModel`（class）+ 4 字段 + 2 abstract method |
| Mock 子类 | `MockHomeViewModel: HomeViewModel`（final）+ invocations | `MockRoomViewModel: RoomViewModel`（final）+ invocations |
| Real 子类 | `RealHomeViewModel: HomeViewModel`（final）+ appState 注入 | `RealRoomViewModel: RoomViewModel`（final）+ appState 注入 |
| 数据模型 | `PetStats` / `AnimationState` 新建 | `RoomMember` 新建 |
| 5 区块 | statusBar / catStage / actionRow / chestSlot / teamIdleCard | topBar / roomCodeCard / sharedStage / membersList / leaveButton |
| State (transient) | `@State resetTask` | `@State copiedFeedback` + `@State copyFeedbackTask` |
| 老占位文件 | 无（HomeView 是改写） | `RoomViewPlaceholder.swift` 保留不删 |
| caller 改动 | `HomeContainerHomeViewBridge` 改新 init 签名 | `HomeContainerView` inRoom 分支改 caller + 新增 `HomeContainerRoomViewBridge` |
| RootView wire | 不改 (`@StateObject homeViewModel`) | 追加 `@StateObject roomViewModel = RoomViewModel()` + `.environmentObject(roomViewModel)` |
| #Preview 数 | 2（candy / dark） | 4（candy 4/2/1 成员 + dark 4 成员） |
| 单元测试 case 数 | 6（≥4 epic AC） | 5（≥4 epic AC） |
| UITest case | `testHomeScaffoldShowsAllSevenAnchors` | `testRoomScaffoldShowsAllSevenAnchors` + 新 env `UITEST_FORCE_IN_ROOM` |
| a11y identifier | inline 7 锚（homeStatusBar / homeCatStage / 3x homeAction* / 2x homeTeamIdleCard_*） | inline 7 锚（returnButton / roomIdDisplay / copyButton / 4x roomMember_*）+ leaveButton + sharedStage |
| 老 a11y 常量 | 保留 AccessibilityID.Home.* 7 个 | 不引入 AccessibilityID.Room（Story 37.13 归并） |

### EnvironmentKey 默认值的 fallback（与 Story 37.5 协调）

RoomScaffoldView 内全部 `@Environment(\.theme) var theme` 取主题；`Environment+Theme.swift` 已落地 `defaultValue: Theme = .candy` fallback。Preview 显式 `.environment(\.theme, ThemeName.candy.theme)` 注入；Production RootView 注入 currentTheme.theme（Story 37.5 落地）。

### xcodegen regen 节奏 + project.yml 兼容性

`iphone/project.yml` 内 `targets.PetApp.sources: - PetApp` + `targets.PetAppTests.sources: - PetAppTests` 通配；新增 6 个文件全部在 `PetApp/Features/Room/` + `PetAppTests/Features/Room/` 下 → 自动 inclusion，**不**改 project.yml。dev 必须 `cd iphone && xcodegen generate` 重生成 `PetApp.xcodeproj`，commit pbxproj diff。

### 与 ADR-0002 §3.1 测试栈钦定的对齐

本 story 测试**仅**用 XCTest + @testable import PetApp + UITest（XCUIApplication）—— 见"测试边界"段。

### 测试 case 数量取舍（≥4 / 实装 5 / 不再加）

epic AC line 4764-4768 钦定 ≥4 case；本 story 落地 5 case：
1. MockRoomViewModel 默认 4 成员状态（roomCodeForCopy / hostCatName / members[0] / userIsHost / invocations 全部就位）
2. MockRoomViewModel 2 成员场景注入（驱动 View 渲染 2 实 + 2 占位的 ViewModel 数据契约）
3. onCopyTap → invocations.append(.copyTap)
4. onLeaveTap → invocations.append(.leaveTap)
5. RealRoomViewModel(appState:) 构造 + 2 abstract method override 不 crash + onLeaveTap 写 appState.currentRoomId nil（间接证 fatalError 路径未被命中 + 验证互斥状态机集成路径）

### 顶部留白契约（与 Story 35.2 share button 协调）

room.jsx:30 钦定顶部右侧是 `<div style={{width: 40}}/>` 空白占位 —— 留给 Story 35.2 share button 落地。本 story RoomScaffoldView topBar 内右侧用 `Color.clear.frame(width: 40, height: 40)` 占位；Story 35.2 改为 share button。**不**预先建 share button SwiftUI structure（属 Story 35.2 范围）。

### 与 Story 12.1 衔接的红线（关键约束）

Story 12.1 缩窄后的范围（sprint-change-proposal-2026-04-29-v2.md §5.1）：
- 把 RealRoomViewModel 替换 MockRoomViewModel（RootView wire 切到 RealRoomViewModel）
- 给 RealRoomViewModel 加 `@Published var wsState: WSState / memberPetStates: [String: HomePetState]` 字段（本 story RoomViewModel 基类不预 over-design 这两字段；Story 12.1 决定加在基类还是 Real 子类内）
- 接 WS room.snapshot 真实消息驱动 members 字段（本 story members 是 mock；Story 12.1 后由 WS 派生）
- "退出房间"按钮 a11y identifier 改为 `roomIdDisplay`（**已在本 story 实装**；epic AC line 4769 钦定）

> **关键决策**：本 story 不预先加 `wsState / memberPetStates` 字段 —— Story 12.1 实装时根据真实 WS 消息形态决定字段 shape；预 over-design 反而让 Story 12.1 dev 在 reset / hydrate 路径上重写 RoomViewModel 浪费工作量（参考 ADR-0010 §4.4 缓解策略）。

### Project Structure Notes

- 新建目录 `iphone/PetApp/Features/Room/ViewModels/` + `iphone/PetApp/Features/Room/Models/`（已有 `iphone/PetApp/Features/Room/Views/`）
- 新建目录 `iphone/PetAppTests/Features/Room/`
- 全部走 `iphone/project.yml` 通配 inclusion；不改 project.yml

### References

- [Source: docs/宠物互动App_总体架构设计.md] —— 总体架构与产品规则（Room 概念）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md] —— iOS 工程目录结构（Features/Room/ViewModels|Models|Views/ 三层）
- [Source: docs/宠物互动App_V1接口设计.md §房间] —— Story 12.x 后接的 server 接口契约（本 story 不依赖）
- [Source: _bmad-output/planning-artifacts/epics.md §Story 37.8] —— 本 story epic AC（line 4747-4769）
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md §3.1] —— ADR-0002 测试栈钦定（XCTest only）
- [Source: _bmad-output/implementation-artifacts/decisions/0009-ios-navigation.md §3.5] —— ADR-0009 主入口 4 Tab + HomeContainerView 互斥状态机
- [Source: _bmad-output/implementation-artifacts/decisions/0010-app-state.md §3.1 §3.2] —— ADR-0010 ViewModel 注入规则 + AppState 范围白名单 + state owner 边界
- [Source: _bmad-output/planning-artifacts/sprint-change-proposal-2026-04-29-v2.md §5.1] —— Story 12.1 缩窄范围 + roomIdDisplay 命名钦定
- [Source: iphone/ui_design/source/screens/room.jsx] —— 5 区块视觉源
- [Source: iphone/ui_design/README.md §RoomScreen] —— RoomScreen 概述
- [Source: iphone/PetApp/Features/Home/Views/HomeView.swift] —— Story 37.7 落地的 HomeView，本 story 1:1 复刻 class 层次模式
- [Source: iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift / MockHomeViewModel.swift / RealHomeViewModel.swift] —— class 层次 + Mock/Real 三件套参考实现
- [Source: iphone/PetApp/Features/Home/Models/PetStats.swift / AnimationState.swift] —— 数据模型 value type 参考实现
- [Source: iphone/PetApp/Features/Home/Views/HomeContainerView.swift] —— 互斥状态机 + Bridge 子视图模式
- [Source: iphone/PetApp/Features/Home/Views/JoinRoomModal/JoinRoomModalPlaceholder.swift] —— 占位 stub 模式参考（与本 story RoomViewPlaceholder 不删保 git history 同精神）
- [Source: iphone/PetApp/Core/DesignSystem/Primitives/Card.swift / PrimaryButton.swift / Avatar.swift / Icons.swift] —— Story 37.6 落地的 primitives，本 story 复用

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) via Claude Code CLI, executing bmad-dev-story workflow.

### Debug Log References

`bash iphone/scripts/build.sh --test` 单次跑通：280 unit case 全绿，0 failure（Story 37.7 落地后基线 273 + 本 story 新增 7 case = 280）.

### Completion Notes List

- 实装严格按 AC1-AC10 落地：6 个新文件 + 修改 3 个老文件（HomeContainerView.swift / RootView.swift / HomeUITests.swift）+ pbxproj regen.
- **Lesson 预防性应用**（Story 37.7 5 轮 review 沉淀）：
  - `2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md`：RootView `@StateObject roomViewModel` 用 `RealRoomViewModel()` 而非基类 `RoomViewModel()`（基类 onLeaveTap/onCopyTap 是 fatalError 占位，inRoom 态点按钮即 crash）；与 RealHomeViewModel 同 parameterless init 模式 + .task 内 bind(appState:).
    - 偏离 spec line 437「不改 RootView wire 切到 Real」：lesson 优先级高于 spec（用户明确指示预防性应用）；UITest 路径 force-in-room flag 走 mock 数据通路，仍能完整定位 7 锚.
  - `2026-04-30-published-derived-state-needs-publisher-subscription.md` + `2026-04-30-realhomeviewmodel-greeting-and-empty-text-overlay.md`：RealRoomViewModel 派生 state（roomCodeForCopy / hostCatName）走 sink 订阅 appState.$currentRoomId / $currentPet —— 不在 init / bind 入口一次性 hydrate（reset → currentRoomId nil 后字段必须即时回 placeholder）；case#7 守护测试覆盖 hydrate → reset 回归路径.
  - `2026-04-30-swiftui-onchange-equatable-and-stale-task-cancel.md`：copyButton 启动 1.2s feedback timer 前 cancel 上一个 `copyFeedbackTask`（与 HomeView `resetTask` 同模式）+ task 内 `Task.isCancelled` double-check 防 sleep race.
  - `2026-04-30-swiftui-floating-emoji-needs-state-driven-position.md`：MiniCat 弹跳动画抽 `MiniCatView` 子视图 + `@State bouncing` + `.onAppear` 内 withAnimation `.repeatForever`（非常量 offset）；4 只猫 delay = index*0.2s 错峰.
- **设计变体**：
  - `sharedStage` 装饰 emoji 用 GeometryReader 替代 spec 的 `UIScreen.main.bounds.width`（避免 deprecated API + 适配 SwiftUI 布局上下文；视觉等价）.
  - 单元测试 7 case 而非 spec ≥4 case：5 case 是 spec 钦定的；额外 +2 case（parameterless init 守护 + bind+reset sink 守护）覆盖 Story 37.7 lesson 预防点.
  - `RoomViewPlaceholder.swift` 不删除（Dev Notes 钦定 "保 git history"）；`HomeContainerView` 内 caller 替换为 `HomeContainerRoomViewBridge()`.
- **AC9 grep 校验全部通过**：class RoomViewModel = 1（去 final）；fatalError = 4（含 2 注释和 2 调用，2 abstract method）；MockRoomViewModel override func = 2；RealRoomViewModel override func = 2；HomeContainerView 含 RoomScaffoldView = 3 处；HomeContainerView 含 `RoomViewPlaceholder()` 调用 = 0.
- **build/test 结果**：`bash iphone/scripts/build.sh --test` BUILD SUCCESS + 280 unit case 全绿（Story 37.7 后基线 ~273 + 7 新增 case；UITest case 在 --uitest flag 下跑，未在本次 --test 调用范围）.

### File List

**新建文件 (6)**：
- `iphone/PetApp/Features/Room/Models/RoomMember.swift`
- `iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift`
- `iphone/PetApp/Features/Room/ViewModels/MockRoomViewModel.swift`
- `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`
- `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift`
- `iphone/PetAppTests/Features/Room/RoomViewScaffoldTests.swift`

**修改文件 (4)**：
- `iphone/PetApp/Features/Home/Views/HomeContainerView.swift`（inRoom 分支 caller 替换 + 新增 `HomeContainerRoomViewBridge` 子视图）
- `iphone/PetApp/App/RootView.swift`（追加 `@StateObject roomViewModel: RoomViewModel = RealRoomViewModel()` + `.environmentObject(roomViewModel)` + .task 内 bind(appState:) + UITEST_FORCE_IN_ROOM env flag handling + LaunchedContentView 接收 roomViewModel 透传）
- `iphone/PetAppUITests/HomeUITests.swift`（追加 `testRoomScaffoldShowsAllSevenAnchors` UITest case）
- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen regen 结果）

### Change Log

| Date | Change | Author |
|---|---|---|
| 2026-05-01 | Story 37.8 implementation complete: RoomScaffoldView + RoomViewModel class hierarchy + Mock/Real subclasses + 7 unit cases + UITest anchors + RootView wire. All 280 unit tests pass. Status: review. | Claude Opus 4.7 (dev-story) |
