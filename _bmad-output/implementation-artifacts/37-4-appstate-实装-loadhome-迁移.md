# Story 37.4: AppState 重新实装 + HomeViewModel.homeData 删除（按 ADR-0010 完全 supersedes Story 5.5 数据持有部分）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iOS 开发,
I want 引入全局 AppState 持有所有 domain state，按 ADR-0010 重新实装数据流,
so that 4 Tab 数据共享有单一权威来源，跨 Tab 联动天然支持，且让 Story 37.3 临时占位字段 `AppCoordinator.currentRoomId` 被删除收口.

## 故事定位（Epic 37 第二层第 2 条 story；与 Story 37.3 同层、可并行；下游 37.5–37.13 都依赖本 story 的 AppState 类）

这是 Epic 37「iPhone 架构层重构 + UI Scaffold（壳先行）」的**第二层 story**之一（与 Story 37.3 同层）。Story 37.3 已 done（commit `559a2b1` HEAD 含 `5bb6ed5` 引入 `AppCoordinator.currentRoomId: String?` 临时占位字段 + `RootView.swift` bootstrap closure 写 `coordinator.currentRoomId = homeData.room.currentRoomId`），本 story 接力把 AppState 实装 + 删除该占位字段 + HomeViewModel.homeData 字段直接删除。

本 story 是**架构重构类**（Epic 37 §AC 红线豁免：「数据完全 mock + 禁 import APIClient」对 37.3/37.4 不适用）；本 story 的本质就是**改基础设施**，自然要触碰 RootView / AppCoordinator / HomeView / HomeViewModel / LoadHome 集成测试 / ResetIdentityViewModel 现有真实代码。

**本 story 落地后立即解锁**：
- Story 37.5 Theme & Design Tokens（独立解锁，与 AppState 不强相关）
- Story 37.7–37.11 5 屏 Scaffold ViewModel 用**构造注入 AppState**模式（**禁** ViewModel 内 `@EnvironmentObject`，详见 ADR-0010 §3.1）
- Story 12.1 / 12.7 / 21.1 / 24.1 / 27.1 / 35.x 下游 ViewModel / UseCase 改写读 AppState

**本 story 的"实装"动作**（一句话概括）：按 ADR-0010 §3 + §4.1 **从空白重新实装** AppState 数据流：新建 `AppState.swift` 类（白名单 7 字段，节点 1 阶段相关字段就位）+ `AppStateTestHelpers.swift` mock builder；HomeViewModel.homeData 字段**直接删除**（不留 deprecation 注释）；HomeViewModel 改构造注入 AppState（`init(appState:)`）；RootView `@StateObject var appState = AppState()` + `.environmentObject(appState)` 注入；bootstrap closure 内 `homeViewModel.applyHomeData(homeData)` 改为 `appState.applyHomeData(homeData)`（取代当前的 `coordinator.currentRoomId = homeData.room.currentRoomId` 双写）；**删除 `AppCoordinator.currentRoomId` 占位字段** + `currentRoomId` 默认参数；HomeContainerView 改读 `@EnvironmentObject var appState: AppState`；ResetIdentityViewModel 加 `appState.reset()` 调用；HomeView 内所有 `viewModel.homeData?.X` 引用改读 `appState.*`（含 `HomePetNameResolver.resolve(homeData:)` → `HomePetNameResolver.resolve(pet: appState.currentPet)` 或等价签名调整）；改写 LoadHomeUseCase 集成测试 / HomeViewModelLoadHomeTests 断言 `appState.*` 而非 `viewModel.homeData`；新建 ≥6 case `AppStateTests.swift`。

**关键路径："重新实装" vs "迁移"**（X1+X2 修订强约束，与 ADR-0010 §4.1 + Story 37.3 实装路径对偶）：
- 旧 `HomeViewModel.homeData` 字段**整段直接删除**，**不**做"先标 deprecated 再渐进迁移"
- 新 `AppState` 类**从空白构建**（参考 ADR-0010 §3.2 白名单字段）
- caller 漏改靠**编译器报错**驱动（HomeView / HomePetNameResolver / HomeViewModelLoadHomeTests / RootViewWireTests 删除 homeData / coordinator.currentRoomId 后立即编译失败 → dev 在 build 通过前必须改完）；**不依赖 grep 兜底**
- 功能完整性靠 **AppStateTests + 改写后的 LoadHomeUseCase 集成测试 + 现有 HomeUITests** 兜底

**不涉及**（红线）：
- **不**实装 currentInventory / currentEquips / emojiCatalog 真实数据流（这些字段仅 AppState 类型骨架就位 / 默认空值；hydrate 后续节点 6/8/9 由对应 epic 接入）
- **不**改 `iphone/ios/` 任何文件（CLAUDE.md + ADR-0002 §3.3 强约束）
- **不**改 server/ 任何文件
- **不**新建 Theme 系统 / primitives（Story 37.5 / 37.6 各自负责）
- **不**改 LoadHomeUseCase 接口签名 / HomeRepository / GET /home 调用契约（ADR-0010 §4.1 钦定保留）
- **不**改 AppCoordinator.currentTab / switchTab / presentedSheet / SheetType（ADR-0010 §3.2 钦定 Tab UI 状态归 AppCoordinator，不进 AppState）
- **不**碰 launching / needsAuth 三态机（独立决策）
- **不**碰 SessionStore（ADR-0010 §3.4 钦定 SessionStore 与 AppState 并行边界）

## Acceptance Criteria

> **AC 编号体系**：AC1–AC6 严格映射 ADR-0010 §3 决策点（合并部分相邻步骤）；AC7 是单元测试；AC8 是集成测试 / UITest；AC9 是 deliverable + commit。

### AC1 — 新建 AppState 类（ADR-0010 §3.1 + §3.2 + §3.7）

**新建文件 `iphone/PetApp/App/AppState.swift`**（按 ADR-0010 §3.2 白名单字段；@MainActor + final + ObservableObject + @Published 各字段）：

```swift
import Foundation
import Combine

/// AppState：全局 domain state 单 source of truth（ADR-0010 §3.1 / §3.2）.
///
/// 范围（白名单，节点 1 阶段相关字段就位；其余节点占位）：
///   - currentUser / currentPet / currentStepAccount / currentChest / currentRoomId（节点 2 起）
///   - currentInventory（节点 8 起）/ currentEquips（节点 9 起）/ emojiCatalog（节点 6 起）
///
/// 不含（ADR-0010 §3.2 表格）：
///   - 当前 Tab → AppCoordinator.currentTab（与 presentedSheet 同级）
///   - Sheet 是否打开 / Loading / WS 连接态 / 表单输入 / 倒计时秒数 → ViewModel 或 SwiftUI @State
///
/// 注入规则（ADR-0010 §3.1 ADR 级硬规则）：
///   - View 层：通过 `.environmentObject(appState)` 在 RootView 注入；子视图用
///     `@EnvironmentObject var appState: AppState` 读
///   - ViewModel 层：**只允许构造注入**（`init(appState:)` 或 `bind(appState:)`）；
///     **禁止** ViewModel 内部用 `@EnvironmentObject`
///   - Mock 时注入 MockAppState 子类（继承 AppState 直接 set 字段）
@MainActor
public final class AppState: ObservableObject {
    @Published public var currentUser: HomeUser?
    @Published public var currentPet: HomePet?
    @Published public var currentStepAccount: HomeStepAccount?
    @Published public var currentChest: HomeChest?
    @Published public var currentRoomId: String?

    // 占位字段（节点 6 / 8 / 9 起真实使用；本 story 仅类型骨架就位 + 默认值）.
    // 类型选择：节点 1 阶段直接复用 Home* 类型族，避免预创建空类型签名影响测试；
    // 后续节点接入新 epic 时如发现需要"非 Home* 派生"的领域类型再做演进（ADR-0010 §4.4 缓解策略）.
    @Published public var currentInventory: [HomeEquip] = []
    @Published public var currentEquips: [HomeEquip] = []
    @Published public var emojiCatalog: [String] = []  // 节点 6 起换 EmojiConfig 类型

    public init() {}

    // MARK: - Hydrate / Mutation 入口（ADR-0010 §3.3）

    /// LoadHomeUseCase 完成后的统一 hydrate 入口（ADR-0010 §3.3 启动/自动登录后流程）.
    /// 命名 `applyHomeData` 与现有 HomeViewModel.applyHomeData(_:) 同名风格，
    /// 让 RootView bootstrap closure 替换前后语义一致（dev 阅读 git diff 时直观）.
    /// 详见 ADR-0010 §3.3 hydrate 流程伪代码 + §3.5 HomeViewModel 关键变化.
    public func applyHomeData(_ data: HomeData) {
        self.currentUser = data.user
        self.currentPet = data.pet
        self.currentStepAccount = data.stepAccount
        self.currentChest = data.chest
        self.currentRoomId = data.room.currentRoomId
    }

    /// Reset 流程（ADR-0010 §3.7）：用户主动登出 / 重置身份时清空全部 domain state.
    /// 由 ResetIdentityViewModel.tap() 成功路径调用（与 SessionStore.clear() 同精神）.
    /// **不**置默认值给 currentUser 等 optional 字段（语义清晰：未登录就是 nil）.
    public func reset() {
        self.currentUser = nil
        self.currentPet = nil
        self.currentStepAccount = nil
        self.currentChest = nil
        self.currentRoomId = nil
        self.currentInventory = []
        self.currentEquips = []
        self.emojiCatalog = []
    }

    /// 显式 setter（节点 4 后用，房间状态 mutation 入口）.
    /// 取消注释当 Story 12.7 落地 CreateRoom/JoinRoom/LeaveRoom UseCase；本 story 仅声明.
    public func setCurrentRoomId(_ roomId: String?) {
        self.currentRoomId = roomId
    }

    /// 显式 setter（节点 5 后 WS pet.state.changed 自身分支用；ADR-0010 §3.3 WS 流程）.
    /// 节点 5 才接 WS；本 story 仅声明类型契约（让 AppStateTests 可写 case），
    /// 不连真实 WS 入口.
    public func updateMyPetState(_ state: HomePetState) {
        guard var pet = currentPet else { return }
        pet = HomePet(
            id: pet.id,
            petType: pet.petType,
            name: pet.name,
            currentState: state,
            equips: pet.equips
        )
        self.currentPet = pet
    }

    /// 显式 setter（节点 9 后 EquipUseCase / UnequipUseCase 用）.
    public func updateCurrentEquips(_ equips: [HomeEquip]) {
        self.currentEquips = equips
    }
}
```

**新建文件 `iphone/PetAppTests/Helpers/AppStateTestHelpers.swift`**（MockAppState builder + reset helper，让 ViewModelTests / Scaffold ViewModel 子类后续注入）：

```swift
import Foundation
@testable import PetApp

/// AppStateTestHelpers：让单元测试快速构造已 hydrate / 已 reset 的 AppState 实例.
///
/// 设计：用 extension 形式给 AppState 加 testing helper，**不**新建 MockAppState 子类
/// （AppState 是 final class，子类不可继承）.
/// 节点 1 阶段相关字段（user/pet/stepAccount/chest/currentRoomId）就位；
/// 其余字段（inventory/equips/emojiCatalog）保持默认空集合.
///
/// AC 锁定：与 AppStateTests.swift 内 hydrate / reset case 同模式.
@MainActor
extension AppState {
    /// 构造一个已 hydrate 的 AppState（带可选 currentRoomId 覆写，方便 inRoom case）.
    static func makeHydrated(currentRoomId: String? = nil) -> AppState {
        let appState = AppState()
        appState.applyHomeData(makeSampleHomeData(currentRoomId: currentRoomId))
        return appState
    }

    /// 构造一个已 reset 的 AppState（全字段 nil/empty）.
    static func makeReset() -> AppState {
        let appState = AppState()
        appState.reset()
        return appState
    }
}

/// 测试用 sample HomeData 构造 helper（与 RootViewWireTests 内 makeHomeData 同精神）.
@MainActor
func makeSampleHomeData(currentRoomId: String? = nil) -> HomeData {
    HomeData(
        user: HomeUser(id: "u_test", nickname: "tester", avatarUrl: ""),
        pet: HomePet(
            id: "p_test",
            petType: 1,
            name: "测试猫",
            currentState: .rest,
            equips: []
        ),
        stepAccount: HomeStepAccount(totalSteps: 100, availableSteps: 50, consumedSteps: 50),
        chest: HomeChest(
            id: "c_test",
            status: .counting,
            unlockAt: Date(timeIntervalSince1970: 0),
            openCostSteps: 100,
            remainingSeconds: 600
        ),
        room: HomeRoom(currentRoomId: currentRoomId)
    )
}
```

**关键约束**：
- `final class` 不可继承 → MockAppState 不走子类路径，走 extension testing helper（与 SessionStore 测试同模式）
- @MainActor 严格 → 所有调用点必须在 MainActor 上下文（测试用 `@MainActor final class XxxTests: XCTestCase`）
- `applyHomeData(_:)` 命名而非 `hydrate(_:)`：与 HomeViewModel.applyHomeData(_:) 现有命名同风格，git diff 阅读直观；ADR-0010 §3.3 文中"hydrate 入口"是概念词（非方法名）

### AC2 — 修改 HomeViewModel：删除 homeData 字段 + 加 appState 构造注入（ADR-0010 §3.5）

**修改 `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`**：

- **删除** `@Published public var homeData: HomeData?` 字段（连同 doc comment / Story 5.5 注释段一并删除）
- **删除** `applyHomeData(_:)` 方法体内 `self.homeData = data` 语句；改为 `self.appState?.applyHomeData(data)`（保留方法签名让 RootView bootstrap closure 不改）
- **新增** `private weak var appState: AppState?` 字段（weak 防止循环引用：RootView 持 AppState + HomeViewModel；HomeViewModel 持 appState weak）
- **新增** `bind(appState:)` 单次绑定方法（与 bind(pingUseCase:) / bind(loadHomeUseCase:) 同模式）：

```swift
/// Story 37.4: 单次绑定 AppState（构造注入路径），由 RootView .task / .ready 内调用.
/// 多次调用时仅第一次生效（防 SwiftUI .task 多次触发覆盖 first 注入）.
/// 与 bind(pingUseCase:) 同 lesson：跨 task 边界注入 AppState 引用，让 applyHomeData(_:)
/// 能 propagate 到 AppState（不再写 self.homeData）.
public func bind(appState: AppState) {
    guard self.appState == nil else { return }
    self.appState = appState
}
```

- **保留** `nickname / appVersion / serverInfo / loadingState / pingUseCase / boundPingUseCase / pingTask / hasFetched / loadHomeUseCase / boundLoadHomeUseCase / errorPresenter / boundErrorPresenter / hasLoadedHome / loadHomeTask` 全部既有字段
- **保留** `bind(pingUseCase:)` / `start()` / `applyPingResult(_:)` / `readAppVersion()` / `bind(loadHomeUseCase:errorPresenter:)` / `loadHome()` / `applyHomeError(_:)` / `resetLoadHomeForRetry()` / `errorMessageFor(_:)` 全部既有方法

**关键 init 改造**：所有 init 增 `appState: AppState? = nil` 默认参数（**追加，不删除老接口**），让 Story 5.5 测试的老 init 调用兼容：

```swift
public init(
    nickname: String = "用户1001",
    appVersion: String = "0.0.0",
    serverInfo: String = "----"
) {
    // ...既有字段赋值...
    self.appState = nil  // 老路径：不持 appState（测试 / Preview 用）
}

public init(
    nickname: String = "用户1001",
    pingUseCase: PingUseCaseProtocol,
    appVersion: String = HomeViewModel.readAppVersion(),
    serverInfo: String = "----",
    appState: AppState? = nil
) { /* ... */ }

public init(
    nickname: String = "用户1001",
    pingUseCase: PingUseCaseProtocol? = nil,
    loadHomeUseCase: LoadHomeUseCaseProtocol,
    errorPresenter: ErrorPresenter,
    appVersion: String = HomeViewModel.readAppVersion(),
    serverInfo: String = "----",
    appState: AppState? = nil
) { /* ... */ }
```

**关键约束**：
- `private weak var appState: AppState?` 是 weak：避免 RootView 持 AppState + HomeViewModel；HomeViewModel 持 AppState 形成循环引用（HomeViewModel 通过 @StateObject 在 RootView 持有，AppState 通过 @StateObject 在 RootView 持有，RootView 是这两者唯一持有方；HomeViewModel 持 weak appState 不会过早释放）
- 删 `homeData` 字段后所有 caller（HomeView / HomePetNameResolver / HomeViewModelLoadHomeTests / 其它测试）立即编译失败 → 见 AC4 / AC5 / AC8 改写
- `applyHomeData(_:)` 仍 public：让 RootView bootstrap closure 调用路径不变（仅 AppState 替代字段写入）

### AC3 — 修改 RootView：注入 AppState + bootstrap 改写（ADR-0010 §3.3 + §4.1）

**修改 `iphone/PetApp/App/RootView.swift`**：

- **新增** `@StateObject private var appState = AppState()` 字段（与 `coordinator` / `container` / `homeViewModel` 同级）
- **修改** `LaunchedContentView` 的 `.ready` 分支：在 `.environmentObject(coordinator)` / `.environmentObject(homeViewModel)` 之后**追加** `.environmentObject(appState)`，并修改 `LaunchedContentView` 的 init 签名加 `appState: AppState` 参数透传
- **修改** `body` 内 `.task` 闭包：在 `homeViewModel.bind(loadHomeUseCase:errorPresenter:)` 调用后**追加** `homeViewModel.bind(appState: appState)` 单次绑定
- **修改** `ensureLaunchStateMachineWired()` bootstrap closure step1：把当前的双写代码（`homeViewModel.applyHomeData(homeData) + coordinator.currentRoomId = homeData.room.currentRoomId`）改为**单写**：

```swift
// 修改前（Story 37.3 codex round 1 [P1] fix 落地的双写）：
await MainActor.run {
    homeViewModel.applyHomeData(homeData)
    coordinator.currentRoomId = homeData.room.currentRoomId
}

// 修改后（Story 37.4 完成 AppState 收口）：
await MainActor.run {
    appState.applyHomeData(homeData)
    homeViewModel.applyHomeData(homeData)  // 兼容路径：HomeViewModel.applyHomeData 内现在调
                                            // self.appState?.applyHomeData(data)；保留外部调用
                                            // 让 hasLoadedHome 跨边界短路 flag 仍生效（loadingState
                                            // 也走 .loaded）.
}
```

> **决议**：保留 `homeViewModel.applyHomeData(homeData)` 调用 —— HomeViewModel 内 `loadingState` / `hasLoadedHome` 短路 flag 仍归 HomeViewModel 自己管（ADR-0010 §3.2 钦定 loading 状态归 ViewModel transient，不进 AppState）；HomeViewModel.applyHomeData 内部既调 self.appState?.applyHomeData(data) 写 AppState，又设 loadingState=.loaded 与 hasLoadedHome=true。RootView bootstrap closure 也直接调 `appState.applyHomeData(homeData)` 是为了：让 AppState hydrate 不依赖 HomeViewModel 实例存在（如未来 LaunchingViewModel 直接调 LoadHomeUseCase 也能写 AppState）；双写**不**是 anti-pattern：内层 HomeViewModel.applyHomeData 写 AppState 会导致重复，但因为是同一个 AppState 实例 + idempotent 赋值（`self.currentUser = data.user` 写两次仍是同值），测试可断言 currentUser 与 hasLoadedHome 两套语义并存。

- **删除** RootView 内不再用的 `coordinator.currentRoomId = homeData.room.currentRoomId` 这一行（取代为 `appState.applyHomeData(homeData)`，currentRoomId 由 applyHomeData 内部写入 appState）
- **保留** launching / needsAuth 三态机不动；保留 ping bind / loadHome bind / errorPresentationHost 全部既有 wire；保留 `.fullScreenCover(item: $coordinator.presentedSheet)` 服务 `.compose` 次级路由
- **保留** Debug build 内 `KeychainUITestHookView` + `resetIdentityViewModel` 注入（不变）

### AC4 — 修改 HomeContainerView：删除 coordinator.currentRoomId 引用 + 改读 appState（ADR-0010 §3.1）

**修改 `iphone/PetApp/Features/Home/Views/HomeContainerView.swift`**：

- **删除** `@EnvironmentObject var coordinator: AppCoordinator` 字段（以及 currentRoomId 通过 coordinator 读取）
- **新增** `@EnvironmentObject var appState: AppState` 字段
- **修改** body 内 `HomeRoomDispatcher.shouldShowRoom(currentRoomId: coordinator.currentRoomId)` 改为 `HomeRoomDispatcher.shouldShowRoom(currentRoomId: appState.currentRoomId)`
- **修改** `.animation(.easeInOut(duration: 0.3), value: coordinator.currentRoomId)` 改为 `.animation(.easeInOut(duration: 0.3), value: appState.currentRoomId)`
- **保留** `HomeRoomDispatcher.shouldShowRoom(currentRoomId:)` 纯函数 helper 签名不动（输入仍是 `String?`；HomeContainerViewTests 现有 case 不需改）
- **保留** EnvironmentValues 自定义 key（resetIdentityViewModel / sessionStore）+ HomeContainerHomeViewBridge 子视图不动

### AC5 — 删除 AppCoordinator.currentRoomId 临时占位字段（X1+X2 编译器报错驱动）

**修改 `iphone/PetApp/App/AppCoordinator.swift`**：

- **删除** `@Published public var currentRoomId: String?` 字段（连同 doc comment）
- **删除** `init` 中 `currentRoomId: String? = nil` 默认参数 + `self.currentRoomId = currentRoomId` 赋值
- **删除** 文件头注释段内提到 currentRoomId 临时占位的整段（保留 currentTab / switchTab 注释）

**预期编译错误链**（删除后必现）：
- `iphone/PetApp/App/RootView.swift:154` `coordinator.currentRoomId = homeData.room.currentRoomId` → 编译失败 → 由 AC3 改写为 `appState.applyHomeData(homeData)` 修复
- `iphone/PetApp/Features/Home/Views/HomeContainerView.swift:29 / :42` 内 `coordinator.currentRoomId` 引用 → 编译失败 → 由 AC4 改写为 `appState.currentRoomId` 修复
- `iphone/PetAppTests/App/AppCoordinatorTests.swift:99-116` `testCurrentRoomIdDefaultsToNil` / `testCurrentRoomIdCanBeAssigned` 两 case → 编译失败 → **整段删除这两个 case**（相关 doc comment "Story 37.3：currentRoomId 临时占位字段（Story 37.4 落地后删除）" 一并删）
- `iphone/PetAppTests/App/RootViewWireTests.swift:127-184` `testBootstrapPropagatesLoadedHomeRoomIdToCoordinator` / `testBootstrapKeepsCoordinatorCurrentRoomIdNilWhenHomeRoomIsEmpty` 两 case → 编译失败 → **改写为 AppState 断言版**（保留意图：bootstrap 完成后 currentRoomId 必须传播；原 `coordinator.currentRoomId == "room_abc123"` 改为 `appState.currentRoomId == "room_abc123"`；helper bootstrap closure 改为调 `appState.applyHomeData(inRoomData)` 而非 `coordinator.currentRoomId = inRoomData.room.currentRoomId`）

**禁止动作**（X1+X2 强约束）：
- ❌ 在 currentRoomId 字段上方加 `@available(*, deprecated)` 注释 + 保留字段
- ❌ 写 grep 脚本扫"全 codebase 内还有没有 `coordinator.currentRoomId` 漏改"
- ❌ 在 RootViewWireTests 旧 case 上方加 `// TODO: replace with appState assertion later`

### AC6 — ResetIdentityViewModel 加 appState.reset() 调用（ADR-0010 §3.7）

**修改 `iphone/PetApp/Features/DevTools/ViewModels/ResetIdentityViewModel.swift`**：

- **新增** `private let appState: AppState?` 字段（与现有 `sessionStore: SessionStore?` 同模式 Optional）
- **修改** init 签名：

```swift
public init(
    useCase: ResetKeychainUseCaseProtocol,
    sessionStore: SessionStore? = nil,
    appState: AppState? = nil
) {
    self.useCase = useCase
    self.sessionStore = sessionStore
    self.appState = appState
}
```

- **修改** `tap()` 方法成功路径，在 `sessionStore?.clear()` 之后**追加** `appState?.reset()` 调用：

```swift
public func tap() async {
    do {
        try await useCase.execute()
        sessionStore?.clear()
        appState?.reset()  // Story 37.4 / ADR-0010 §3.7 reset 流程
        alertContent = .success
    } catch {
        alertContent = .failure(message: "重置失败：\(error.localizedDescription)")
    }
}
```

**修改 `iphone/PetApp/App/AppContainer.swift`**（如有 `makeResetIdentityViewModel()` 工厂方法）：传入 appState 引用。如 RootView 直接调 `container.makeResetIdentityViewModel()`，则需要 RootView 把 appState 透传给 container（或 container 持 appState 引用）。**Dev 决议**：dev 实装期跑 `bash iphone/scripts/build.sh --test`，根据编译失败链确定最终 wire 方式（推荐：RootView 把 `appState` 透传到 `container.makeResetIdentityViewModel(appState: appState)` 或类似形式；不引入新单例 / 全局）。

### AC7 — 修改 HomeView：所有 viewModel.homeData 引用改读 appState（ADR-0010 §3.5）

**修改 `iphone/PetApp/Features/Home/Views/HomeView.swift`**：

- **新增** `@EnvironmentObject var appState: AppState` 字段（HomeView 是 SwiftUI View，按 ADR-0010 §3.1 例外条款：纯展示 SwiftUI View 可直接用 @EnvironmentObject）
- **修改** `petNameDisplay` 计算属性：`HomePetNameResolver.resolve(homeData: viewModel.homeData)` 改为 `HomePetNameResolver.resolve(pet: appState.currentPet, hasHydrated: appState.currentUser != nil)`（详见下条 HomePetNameResolver 改造）
- **修改** `chestColumn` 内 `viewModel.homeData?.chestRemainingDisplay ?? "--:--"` 改为 `appState.currentChest?.remainingDisplay ?? "--:--"`
- **修改** `stepBalanceLabel` 内 `viewModel.homeData?.stepAccount.availableSteps ?? 0` 改为 `appState.currentStepAccount?.availableSteps ?? 0`

**修改 `HomePetNameResolver` 签名**（同文件内 public enum）：

```swift
public enum HomePetNameResolver {
    public static let loadingPlaceholder = "默认小猫"
    public static let noPetPlaceholder = "暂无宠物"

    /// Story 37.4 改造：原签名 `resolve(homeData: HomeData?)` 改为 `resolve(pet:hasHydrated:)`.
    /// 三态分支语义保持完全一致（loading / no-pet / has-pet），仅入参形式改为 AppState 投影.
    /// - Parameter pet: 来自 `appState.currentPet`.
    /// - Parameter hasHydrated: 用 `appState.currentUser != nil` 派生（hydrate 完成后 user 必非 nil）；
    ///   语义对齐原方案 `homeData != nil`.
    public static func resolve(pet: HomePet?, hasHydrated: Bool) -> String {
        guard hasHydrated else { return loadingPlaceholder }
        guard let pet = pet else { return noPetPlaceholder }
        return pet.name
    }
}
```

**修改 `iphone/PetAppTests/Features/Home/HomePetNameResolverTests.swift`**：所有 case 入参改为 `(pet:, hasHydrated:)` 形式；语义不变 → 三态分支断言完全保留：
- case#1 `resolve(pet: nil, hasHydrated: false)` → `loadingPlaceholder`
- case#2 `resolve(pet: nil, hasHydrated: true)` → `noPetPlaceholder`
- case#3 `resolve(pet: HomePet(...), hasHydrated: true)` → pet.name

### AC8 — 单元测试覆盖（≥6 case AppStateTests + 改写 HomeViewModelLoadHomeTests + RootViewWireTests）

**新建测试文件 `iphone/PetAppTests/App/AppStateTests.swift`**（≥6 case，按 ADR-0010 §3.6 + Story 故事顶部"协调上下文"钦定）：

```swift
import XCTest
@testable import PetApp

@MainActor
final class AppStateTests: XCTestCase {

    // MARK: - case#1 happy: applyHomeData → currentUser / currentPet / currentStepAccount / currentChest / currentRoomId 全部就绪

    func testApplyHomeDataPopulatesAllNode2Fields() {
        let appState = AppState()
        let homeData = makeSampleHomeData(currentRoomId: "room_1234567")

        appState.applyHomeData(homeData)

        XCTAssertNotNil(appState.currentUser)
        XCTAssertEqual(appState.currentUser?.nickname, "tester")
        XCTAssertNotNil(appState.currentPet)
        XCTAssertEqual(appState.currentPet?.name, "测试猫")
        XCTAssertNotNil(appState.currentStepAccount)
        XCTAssertEqual(appState.currentStepAccount?.availableSteps, 50)
        XCTAssertNotNil(appState.currentChest)
        XCTAssertEqual(appState.currentRoomId, "room_1234567")
    }

    // MARK: - case#2 happy: reset() → 全字段 nil/empty

    func testResetClearsAllFields() {
        let appState = AppState()
        appState.applyHomeData(makeSampleHomeData(currentRoomId: "room_1234567"))

        appState.reset()

        XCTAssertNil(appState.currentUser)
        XCTAssertNil(appState.currentPet)
        XCTAssertNil(appState.currentStepAccount)
        XCTAssertNil(appState.currentChest)
        XCTAssertNil(appState.currentRoomId)
        XCTAssertTrue(appState.currentInventory.isEmpty)
        XCTAssertTrue(appState.currentEquips.isEmpty)
        XCTAssertTrue(appState.emojiCatalog.isEmpty)
    }

    // MARK: - case#3 happy: setCurrentRoomId("room_1234567") → currentRoomId == "room_1234567"（String 契约 / AR21 ID）

    func testSetCurrentRoomIdAcceptsArbitraryString() {
        let appState = AppState()
        appState.setCurrentRoomId("room_1234567")
        XCTAssertEqual(appState.currentRoomId, "room_1234567")

        appState.setCurrentRoomId(nil)
        XCTAssertNil(appState.currentRoomId)
    }

    // MARK: - case#4 happy: updateCurrentEquips([...]) → currentEquips 替换

    func testUpdateCurrentEquipsReplacesCollection() {
        let appState = AppState()
        XCTAssertTrue(appState.currentEquips.isEmpty, "默认应为空")

        let equip = HomeEquip(slot: 1, userCosmeticItemId: "uci_1", cosmeticItemId: "ci_1",
                              name: "帽子", rarity: 1, assetUrl: "")
        appState.updateCurrentEquips([equip])
        XCTAssertEqual(appState.currentEquips.count, 1)
        XCTAssertEqual(appState.currentEquips.first?.name, "帽子")

        appState.updateCurrentEquips([])
        XCTAssertTrue(appState.currentEquips.isEmpty, "再次写空数组应清空")
    }

    // MARK: - case#5 happy: updateMyPetState(.walk) → currentPet.currentState 更新

    func testUpdateMyPetStateMutatesCurrentPetState() {
        let appState = AppState()
        appState.applyHomeData(makeSampleHomeData())
        XCTAssertEqual(appState.currentPet?.currentState, .rest)

        appState.updateMyPetState(.walk)
        XCTAssertEqual(appState.currentPet?.currentState, .walk)
    }

    // MARK: - case#6 edge: hydrate 之前读字段 → 全是 nil（不崩）

    func testInitialStateIsAllNilOrEmpty() {
        let appState = AppState()
        XCTAssertNil(appState.currentUser)
        XCTAssertNil(appState.currentPet)
        XCTAssertNil(appState.currentStepAccount)
        XCTAssertNil(appState.currentChest)
        XCTAssertNil(appState.currentRoomId)
        XCTAssertTrue(appState.currentInventory.isEmpty)
        XCTAssertTrue(appState.currentEquips.isEmpty)
        XCTAssertTrue(appState.emojiCatalog.isEmpty)
    }

    // MARK: - case#7 edge: updateMyPetState 在 currentPet=nil 时是 noop（不崩）

    func testUpdateMyPetStateNoopWhenCurrentPetIsNil() {
        let appState = AppState()
        XCTAssertNil(appState.currentPet, "前置：currentPet 默认 nil")

        appState.updateMyPetState(.walk)  // 不应抛异常
        XCTAssertNil(appState.currentPet, "currentPet 仍为 nil（updateMyPetState 是 guard let pet noop）")
    }
}
```

**改写 `iphone/PetAppTests/Features/Home/ViewModels/HomeViewModelLoadHomeTests.swift`**：所有 `viewModel.homeData` 断言改为 `appState.currentUser` / `appState.currentPet` 等投影；初始化 ViewModel 时同时构造 AppState 实例 + 调 `viewModel.bind(appState: appState)`：

```swift
// case#1 改写示例：
func testLoadHomeSuccessUpdatesState() async {
    let mock = MockLoadHomeUseCase()
    let expectedData = makeHomeData()
    mock.executeStub = .success(expectedData)
    let presenter = ErrorPresenter(toastDuration: 0.05)
    let appState = AppState()
    let viewModel = HomeViewModel(loadHomeUseCase: mock, errorPresenter: presenter)
    viewModel.bind(appState: appState)

    await viewModel.loadHome()

    XCTAssertEqual(appState.currentUser?.nickname, "u")
    XCTAssertEqual(appState.currentPet?.name, "默认小猫")
    XCTAssertEqual(appState.currentStepAccount?.availableSteps, 0)
    XCTAssertEqual(viewModel.loadingState, .loaded)
    XCTAssertEqual(mock.callCount(of: "execute()"), 1)
}
```

**改写 `iphone/PetAppTests/App/RootViewWireTests.swift`**：把 `testBootstrapPropagatesLoadedHomeRoomIdToCoordinator` / `testBootstrapKeepsCoordinatorCurrentRoomIdNilWhenHomeRoomIsEmpty` 两 case 改写为 AppState 断言版（保留意图：bootstrap 完成后 currentRoomId 必须传播）：

```swift
func testBootstrapPropagatesLoadedHomeRoomIdToAppState() async {
    let appState = AppState()
    XCTAssertNil(appState.currentRoomId, "前置：appState.currentRoomId 默认 nil")

    let inRoomData = makeHomeData(currentRoomId: "room_abc123")
    let bootstrapStep1: @Sendable () async throws -> Void = {
        await MainActor.run {
            appState.applyHomeData(inRoomData)  // 替代旧的 coordinator.currentRoomId = ...
        }
    }

    let sm = AppLaunchStateMachine(bootstrapStep1: bootstrapStep1)
    await sm.bootstrap()

    XCTAssertEqual(sm.state, .ready)
    XCTAssertEqual(
        appState.currentRoomId,
        "room_abc123",
        "bootstrap 完成后 appState.currentRoomId 必须 = homeData.room.currentRoomId; " +
        "否则 HomeContainerView 会把已在房间用户错误渲染成 idle home screen."
    )
    XCTAssertTrue(
        HomeRoomDispatcher.shouldShowRoom(currentRoomId: appState.currentRoomId)
    )
}
```

**最终单元测试 case 总数**（本 story 涉及，≥10 个新 / 改写）：
- AppStateTests: ≥6 case（含 hydrate / reset / setCurrentRoomId / updateMyPetState / updateCurrentEquips / 初始空态 / pet=nil 兜底）
- HomeViewModelLoadHomeTests: 5 case 全部改写（断言从 viewModel.homeData → appState.*）
- RootViewWireTests: 2 case 改写（coordinator.currentRoomId → appState.currentRoomId）
- AppCoordinatorTests: 2 case 删除（testCurrentRoomIdDefaultsToNil / testCurrentRoomIdCanBeAssigned），保留其它 currentTab / switchTab / present / dismiss case
- HomePetNameResolverTests: 全部 case 入参改 `(pet:, hasHydrated:)`（语义不变）
- ResetIdentityViewModelTests（如有）：新增 1 case 验证 `appState.reset()` 调用链（可注入 mockAppState 验证 reset 后 currentUser == nil）

### AC9 — 集成测试 / UITest 验收 + Deliverable（commit）

**改写 LoadHomeUseCase 集成测试断言**（如 `iphone/PetAppTests/Features/Home/UseCases/LoadHomeUseCaseTests.swift` 内有"hydrate 后 ViewModel.homeData != nil"风格的集成断言；LoadHomeUseCaseTests.swift 当前主要测 UseCase → HomeData 转换，是 unit-level 单测，**没有**直接断言 `homeViewModel.homeData`）：本 story 范围内，"集成测试改写"由 HomeViewModelLoadHomeTests.swift 改写承担（见 AC8）；如 dev 实装期发现 LoadHomeUseCaseTests 内仍有 `viewModel.homeData` 引用 → 同步改写。

**UITest 验收**（功能完整性硬保证；不引新 UITest case，跑现有 PetAppUITests 全套通过）：
- 启动 → mock server 收到 1 次 `/home`（与 Story 37.3 同条件，PetAppUITests 当前不带 mock server hook → 用 `UITEST_SKIP_GUEST_LOGIN=1` launchEnvironment 路径，bootstrap closure 走 default 行为；Story 37.13 a11y 总表 / mock server hook 联动 story 落地后再加抓取）
- 4 Tab 可定位（NavigationUITests 5 case 不变）
- HomeUITests 现有 testHomeViewShowsAllPlaceholders / testVersionFooter_ShowsAppVersionAndServerInfo 不能 regress —— HomeView 改读 appState 后，`UITEST_SKIP_GUEST_LOGIN=1` 路径下 appState 是空态（无 hydrate），HomeView 应渲染 loading placeholder（"默认小猫" / "--:--" / "0 步"），与现有 UITest 断言兼容
- needsAuth 三态机正常（AppLaunchStateMachineTests 单元路径覆盖）

**Build / Test 硬条件**：
- `bash iphone/scripts/build.sh --test` 全部 unit case 通过（包括新 AppStateTests ≥6 case + 改写的 HomeViewModelLoadHomeTests / RootViewWireTests / HomePetNameResolverTests / AppCoordinatorTests）
- `bash iphone/scripts/build.sh --uitest` 全部 UI case 通过（HomeUITests / NavigationUITests / KeychainPersistenceUITests）
- `bash iphone/scripts/build.sh` release build 通过（无 #if DEBUG 路径泄漏）

**Deliverable（单一 commit 或 dev 自决拆 2–3 个逻辑 commit）**：
- 新增文件：`iphone/PetApp/App/AppState.swift` / `iphone/PetAppTests/Helpers/AppStateTestHelpers.swift` / `iphone/PetAppTests/App/AppStateTests.swift`
- 修改文件：`RootView.swift` / `AppCoordinator.swift` / `HomeViewModel.swift` / `HomeContainerView.swift` / `HomeView.swift`（含 HomePetNameResolver 签名）/ `ResetIdentityViewModel.swift` / `AppContainer.swift`（如需透传 appState 给 makeResetIdentityViewModel）/ `AppCoordinatorTests.swift` / `RootViewWireTests.swift` / `HomeViewModelLoadHomeTests.swift` / `HomePetNameResolverTests.swift`
- 修改 `_bmad-output/implementation-artifacts/sprint-status.yaml`：`37-4-appstate-实装-loadhome-迁移: ready-for-dev → in-progress → review`（dev-story workflow 自动改）
- 修改 `_bmad-output/implementation-artifacts/37-4-appstate-实装-loadhome-迁移.md`（本 story 文件 dev agent record + Status）

**xcodegen 配置同步**：`iphone/project.yml` sources 是 glob（`- PetApp` 形式，Story 37.3 已确认）→ xcodegen 自动 pick 新文件，不需手动同步条目。

**commit message 建议格式**（参考 Story 37.3 模板 + ADR-0010 §3 落地依据）：

```
feat(iphone): AppState 实装 + HomeViewModel.homeData 删除（Story 37.4 done; ADR-0010 §3）

- 新建 AppState 类（@MainActor + ObservableObject + 节点 1 阶段相关字段 + reset/hydrate）
- 新建 AppStateTests.swift（≥6 case，含 hydrate / reset / setCurrentRoomId / updateMyPetState）
- 新建 AppStateTestHelpers.swift（makeHydrated / makeReset / makeSampleHomeData 共享 helper）
- 删除 HomeViewModel.homeData 字段 + 加 bind(appState:) 构造注入
- 删除 AppCoordinator.currentRoomId 临时占位字段（Story 37.3 X1+X2 协调收口）
- RootView 加 @StateObject AppState + .environmentObject(appState) 注入；
  bootstrap closure 改 appState.applyHomeData(homeData) 单写（取代 coordinator.currentRoomId 双写）
- HomeContainerView 改读 appState.currentRoomId（删 coordinator 引用）
- HomeView 所有 viewModel.homeData 引用改读 appState.* 投影
- HomePetNameResolver 签名改 (pet:hasHydrated:)（语义保持）
- ResetIdentityViewModel.tap() 加 appState.reset() 调用（ADR-0010 §3.7 reset 流程）
- 改写 HomeViewModelLoadHomeTests / RootViewWireTests / HomePetNameResolverTests 断言
- 删除 AppCoordinatorTests 内 currentRoomId 2 case

Refs Story 37.4; ADR-0010 §3 / §4.1 / §3.7; unblocks Story 37.5 Theme + 37.7-37.11 Scaffold + Epic 12.1/12.7/21.1/24.1/27.1/35.x.
```

## Tasks / Subtasks

- [x] **Task 1：Pre-flight check + 协调上下文确认**（AC1）
  - [x] 确认 ADR-0010 当前是 Accepted（Story 37.2 已 done；本 story 严格按 §3 决策落地，**不**做 ADR 修订）
  - [x] 确认 sprint-status.yaml 内 `37-2-...` / `37-3-...` 状态为 done、`37-4-...` 为 ready-for-dev / in-progress（本 story 自身）
  - [x] 确认 Story 37.3 已落地的临时占位字段 `AppCoordinator.currentRoomId: String?`（commit `5bb6ed5` 引入）—— 本 story 删除收口
  - [x] 阅读 ADR-0010 §3.1（注入规则）/ §3.2（白名单字段）/ §3.3（hydrate 流程）/ §3.5（HomeViewModel 关键变化）/ §3.6（测试影响）/ §3.7（Reset 流程）

- [x] **Task 2：新建 AppState 类 + AppStateTestHelpers**（AC1）
  - [x] 新建 `iphone/PetApp/App/AppState.swift`：白名单 7 字段（节点 1 阶段相关 5 字段 + 节点 6/8/9 占位 3 字段）+ `applyHomeData(_:)` + `reset()` + `setCurrentRoomId(_:)` + `updateMyPetState(_:)` + `updateCurrentEquips(_:)`
  - [x] 新建 `iphone/PetAppTests/Helpers/AppStateTestHelpers.swift`：`extension AppState { makeHydrated / makeReset }` + `makeSampleHomeData(currentRoomId:)` 共享 helper
  - [x] 文件头注释引 ADR-0010 §3 各章节锚点 + Story 37.4 落地依据

- [x] **Task 3：新建 AppStateTests.swift（≥7 case）**（AC8）
  - [x] case#1 happy: applyHomeData → 5 字段就绪
  - [x] case#2 happy: reset() → 全字段清空
  - [x] case#3 happy: setCurrentRoomId 双向
  - [x] case#4 happy: updateCurrentEquips 替换
  - [x] case#5 happy: updateMyPetState 改 currentState
  - [x] case#6 edge: 初始空态全 nil
  - [x] case#7 edge: updateMyPetState 在 currentPet=nil 时 noop（兜底）

- [x] **Task 4：修改 HomeViewModel：删除 homeData 字段 + 加 bind(appState:)**（AC2）
  - [x] 删 `@Published var homeData: HomeData?` 字段
  - [x] 加 `private weak var appState: AppState?`
  - [x] 加 `bind(appState:)` 单次绑定方法（与 bind(pingUseCase:) / bind(loadHomeUseCase:) 同模式）
  - [x] 改 `applyHomeData(_:)` 内部：把 `self.homeData = data` 改为 `self.appState?.applyHomeData(data)`；保留 `loadingState = .loaded` + `hasLoadedHome = true`
  - [x] 三个 init 加 `appState: AppState? = nil` 默认参数（追加，不删除老接口）
  - [x] 编译失败链：HomeView / HomePetNameResolver / HomeViewModelLoadHomeTests 立即报错 → Task 5 / 6 / 8 修复

- [x] **Task 5：修改 HomeView + HomePetNameResolver**（AC7）
  - [x] HomeView 加 `@EnvironmentObject var appState: AppState`
  - [x] `petNameDisplay` 改 `HomePetNameResolver.resolve(pet: appState.currentPet, hasHydrated: appState.currentUser != nil)`
  - [x] `chestColumn` 改 `appState.currentChest?.remainingDisplay ?? "--:--"`
  - [x] `stepBalanceLabel` 改 `appState.currentStepAccount?.availableSteps ?? 0`
  - [x] `HomePetNameResolver.resolve(homeData:)` 改签名为 `resolve(pet:hasHydrated:)`；三态语义保持
  - [x] 修改 `HomeView_Previews`：手动构造 AppState + .environmentObject(appState) 注入（codex r2 [P3] 留档：MainTabView_Previews 也需类似修复，本 story 顺手处理）

- [x] **Task 6：修改 HomeContainerView**（AC4）
  - [x] 删 `@EnvironmentObject var coordinator: AppCoordinator`
  - [x] 加 `@EnvironmentObject var appState: AppState`
  - [x] body 内两处 `coordinator.currentRoomId` 改 `appState.currentRoomId`
  - [x] HomeRoomDispatcher 签名不动（仍接收 String?）

- [x] **Task 7：修改 RootView + ResetIdentityViewModel + AppContainer**（AC3 + AC6）
  - [x] RootView 加 `@StateObject private var appState = AppState()`
  - [x] `LaunchedContentView.init` + `.ready` 分支注入 `.environmentObject(appState)`
  - [x] body 内 `.task` 闭包加 `homeViewModel.bind(appState: appState)`
  - [x] `ensureLaunchStateMachineWired()` bootstrap closure：`coordinator.currentRoomId = ...` 一行改为 `appState.applyHomeData(homeData)` + 保留 `homeViewModel.applyHomeData(homeData)`
  - [x] ResetIdentityViewModel：加 `appState: AppState?` 字段 + init 参数 + `tap()` 内 `appState?.reset()` 调用
  - [x] AppContainer.makeResetIdentityViewModel 签名透传 appState（按 dev 编译失败链决策）

- [x] **Task 8：删除 AppCoordinator.currentRoomId + 改写测试**（AC5）
  - [x] AppCoordinator.swift 删 `currentRoomId` 字段 + init 参数 + 头部注释相关段
  - [x] AppCoordinatorTests.swift 删 `testCurrentRoomIdDefaultsToNil` / `testCurrentRoomIdCanBeAssigned` 两 case + 注释
  - [x] RootViewWireTests.swift 改写 `testBootstrapPropagatesLoadedHomeRoomIdToCoordinator` → ToAppState；同样改 `testBootstrapKeepsCoordinatorCurrentRoomIdNilWhenHomeRoomIsEmpty`
  - [x] HomeViewModelLoadHomeTests.swift 改写所有 case：构造 AppState + viewModel.bind(appState:) + 断言 appState.* 而非 viewModel.homeData
  - [x] HomePetNameResolverTests.swift 改写所有 case：入参改 `(pet:, hasHydrated:)`；语义不变

- [x] **Task 9：xcodegen 同步 + build / test 验证**（AC9）
  - [x] sources 是 glob → xcodegen 自动 pick 新文件
  - [x] `bash iphone/scripts/build.sh --test` unit 全部通过（断言新增 / 改写 case 全 pass；现有 case 不 regress）
  - [ ] `bash iphone/scripts/build.sh --uitest` UI 全部通过（HomeUITests / NavigationUITests / KeychainPersistenceUITests 不 regress）
  - [ ] `bash iphone/scripts/build.sh` release build 通过

- [x] **Task 10：更新本 story 文件 dev agent record**（AC9）
  - [x] Agent Model Used / Completion Notes List / File List / Change Log

- [ ] **Task 11：commit**（AC9）—— **不在 dev-story workflow 范围内**，由 story-done / fix-review 流程负责

## Dev Notes

### 故事关键路径（X1+X2 修订强约束）：重新实装 vs 迁移

ADR-0010 v2 X1+X2 修订把本 story 路径从「partial revert + 渐进迁移 + grep 兜底」改为「**重新实装 + 编译器报错驱动 + AppStateTests 全覆盖兜底**」（参见 ADR-0010 §4.1 + 本 story 顶部"故事定位"段）。Dev 实装时**严格按重新实装路径**：

- 旧 `HomeViewModel.homeData` 字段**整段直接删除** —— 删除后 `viewModel.homeData?.X` 等所有调用立即编译失败 → dev 修编译错误时即修完全部 caller
- 旧 `AppCoordinator.currentRoomId: String?` 临时占位字段（Story 37.3 commit `5bb6ed5` 引入）**整段直接删除** —— 删除后 RootView / HomeContainerView / 测试代码立即编译失败 → dev 修编译错误时即修完全部 caller
- 旧 `HomePetNameResolver.resolve(homeData:)` 签名 **直接改为** `resolve(pet:hasHydrated:)` —— HomeView / HomePetNameResolverTests 立即编译失败 → 修完
- 旧 RootViewWireTests 内 `coordinator.currentRoomId` 断言 **直接改写** —— 不写"先标 deprecated 再渐进改"

**禁止动作**：
- ❌ 在 HomeViewModel.homeData 上方加 `@available(*, deprecated)` 注释 + 保留字段
- ❌ 在 AppCoordinator.currentRoomId 上方加 `@available(*, deprecated)` 注释 + 保留字段
- ❌ 写 grep 脚本扫"全 codebase 内还有没有 `viewModel.homeData` / `coordinator.currentRoomId` 漏改"
- ❌ 在 HomeView.swift 内同时保留 `viewModel.homeData?.X` 与 `appState.X` 两条读取路径（双 source of truth 是反模式）

**允许的退路**：如发现 ADR-0010 §3 决策与实际实装出现冲突（如 weak appState 引发某种生命周期问题；或 HomeViewModel.applyHomeData 的双写实际不工作）→ 走「ADR 修订 patch + 改 v2 Accepted」路径（参考 ADR-0008 v2 commit `ec5beb3` 先例 + ADR-0010 §6 第 4 条）。修订 patch 与本 story commit **同 commit 落地**；commit message 显式记录"ADR-0010 §3.X 修订：原文 X，修为 Y，理由 Z"。

### Story 37.3 协调上下文（关键 — 本 story 是 37.3 临时占位的收口）

**Story 37.3 已落地的临时基础设施**（commit `5bb6ed5` 引入，本 story 删除收口）：

1. `iphone/PetApp/App/AppCoordinator.swift:64` `@Published public var currentRoomId: String?` 字段 + `:67-74` init 参数（**本 story Task 8 删除**）
2. `iphone/PetApp/App/RootView.swift:154` `coordinator.currentRoomId = homeData.room.currentRoomId` 一行（**本 story Task 7 改为 `appState.applyHomeData(homeData)`**）
3. `iphone/PetApp/Features/Home/Views/HomeContainerView.swift:23` `@EnvironmentObject var coordinator: AppCoordinator` + `:29 / :42` 内 `coordinator.currentRoomId` 引用（**本 story Task 6 改 `appState`**）
4. `iphone/PetAppTests/App/AppCoordinatorTests.swift:99-116` 2 case `testCurrentRoomIdDefaultsToNil` / `testCurrentRoomIdCanBeAssigned`（**本 story Task 8 删除**）
5. `iphone/PetAppTests/App/RootViewWireTests.swift:127-184` 2 case `testBootstrapPropagatesLoadedHomeRoomIdToCoordinator` / `testBootstrapKeepsCoordinatorCurrentRoomIdNilWhenHomeRoomIsEmpty`（**本 story Task 8 改写为 AppState 断言版**）

**编译器报错驱动验证清单**（删 currentRoomId 字段后预期触发的全部编译错误位点；如还有别处编译失败 → dev 修完同一 commit）：
- RootView.swift:154
- HomeContainerView.swift:29 / :42
- AppCoordinatorTests.swift:104 / :111-116
- RootViewWireTests.swift:135 / :140-144 / :158 / :167 / :171 / :179-182

**双向语义对偶 Story 37.3**：Story 37.3 是「主入口导航壳」从空白构建 + 删 SheetType.room/.inventory；本 story 是「数据持有」从空白构建 + 删 HomeViewModel.homeData + 删 AppCoordinator.currentRoomId 占位。两 story 加起来完成 ADR-0009 + ADR-0010 联动重构。

### AppState 类型选择：复用 Home* 类型族 vs 新建领域类型

ADR-0010 §3.2 白名单字段示意代码用了 `User?` / `Pet?` / `StepAccount?` / `Chest?` / `[CosmeticInstance]` / `Equipment` / `[EmojiConfig]` 等类型名，但 codebase 现存的 domain 类型是 `HomeUser` / `HomePet` / `HomeStepAccount` / `HomeChest` / `HomeEquip`。本 story 决议：

- **节点 1 阶段直接复用 `Home*` 类型族**（`HomeUser` / `HomePet` / `HomeStepAccount` / `HomeChest` / `HomeEquip`）；不新建 `User` / `Pet` 等空类型签名
- 占位字段 `currentInventory` / `currentEquips` 类型用 `[HomeEquip]`（节点 8 / 9 接入时如需要拆分 inventory 与 equips 类型再做演进）
- `emojiCatalog` 占位类型用 `[String]`（节点 6 起接入 EmojiConfig 类型时改）
- `User` / `Pet` 等"裸名" domain 类型未来 Epic 演进若需要再做（ADR-0010 §4.4 已知坑缓解）

**理由**：节点 1 阶段引入 `User` / `Pet` 空 typealias 没意义；后续节点接入新 epic 时如发现需要"非 Home* 派生"的领域类型再做演进；保持单 source of truth 同步演进比预创建空类型签名健康。

### HomeViewModel.applyHomeData 的双重职责（ViewModel transient + AppState write）

ADR-0010 §3.5 钦定「HomeViewModel 退化为 view-specific 投影 + 行为；不再持 homeData」，但 `loadingState: HomeLoadingState` / `hasLoadedHome: Bool` / `hasFetched: Bool` 等 transient flag 仍归 HomeViewModel（ADR-0010 §3.2 表格"Loading / error toast → ViewModel `@Published`"钦定）。

`applyHomeData(_:)` 方法在本 story 改造后承担**双重职责**：
1. 写 AppState（`self.appState?.applyHomeData(data)`）→ 驱动 UI 读取
2. 改 HomeViewModel transient state（`loadingState = .loaded` + `hasLoadedHome = true`）→ 驱动 ViewModel 自身的短路 flag

**这不是反模式**——transient state（loading flag）与 domain state（user/pet）的边界在 ADR-0010 §3.2 表格内已明确，HomeViewModel 同时管两类 state 的写入入口（applyHomeData）是合理的协调器角色。

**RootView bootstrap closure 内的"双写"**（`appState.applyHomeData(homeData) + homeViewModel.applyHomeData(homeData)`）也不是反模式：内层 HomeViewModel.applyHomeData 写的是同一个 AppState 实例 + idempotent 赋值（同值），不会引入"两个 AppState 不一致"的 source of truth 问题；同时让 HomeViewModel 的 hasLoadedHome 短路 flag 准确生效（避免后续 SwiftUI .task 重启 RootView 时 viewModel.loadHome() 重发请求）。

### HomeView 改读 appState：weak 引用与 @EnvironmentObject 的协调

ADR-0010 §3.1 明确：
- ViewModel 层 → 构造注入 AppState（**禁** @EnvironmentObject）
- View 层 → 例外条款：纯展示 SwiftUI View 可直接 @EnvironmentObject AppState

HomeView 是 SwiftUI View（不是 ViewModel），适用 View 层例外条款 → 直接 `@EnvironmentObject var appState: AppState` 是合规的。

`HomeViewModel.appState` 字段是 weak（防循环引用：RootView 持 AppState + HomeViewModel；HomeViewModel 持 AppState weak 不影响 lifecycle，因为 RootView 的 @StateObject 是 strong owner）。

### HomePetNameResolver 签名改造的语义对齐

原签名 `resolve(homeData: HomeData?)` 三态：
1. `homeData == nil` → loading placeholder（"默认小猫"）
2. `homeData != nil && pet == nil` → no-pet placeholder（"暂无宠物"）
3. `pet != nil` → pet.name

新签名 `resolve(pet: HomePet?, hasHydrated: Bool)` 三态对齐：
1. `hasHydrated == false` → loading placeholder
2. `hasHydrated == true && pet == nil` → no-pet placeholder
3. `pet != nil` → pet.name

**hasHydrated 派生方式**：HomeView 内调用时用 `appState.currentUser != nil` 派生（hydrate 完成后 currentUser 必非 nil；与原 `homeData != nil` 等价）。

**为何不直接读 `appState.currentPet`+`appState.currentUser` 在 HomePetNameResolver 内**：保持纯函数语义（输入纯 value 类型 + 输出 String），不让 helper 依赖 AppState 类型；测试入参更直观。

### 与 Story 37.5 / 37.7-37.11 的接缝

本 story 落地的 AppState 是后续 Story 37.7-37.11 5 屏 ViewModel 的**构造注入入参**：

- 各 Scaffold ViewModel 基类构造签名 `init(appState: AppState)`；`MockXxxViewModel` 子类用 `MockAppState`（节点 1 阶段 = `AppState.makeHydrated()` testing helper）注入；`RealXxxViewModel` 子类用真实 AppState 注入
- ViewModel 内**禁** `@EnvironmentObject AppState`（ADR-0010 §3.1 ADR 级硬规则）
- 测试时构造 `let appState = AppState.makeHydrated()` + `let vm = MockHomeViewModel(appState: appState)` → 让 `XCTAssertEqual(vm.derived, "expected")` 验证 view-specific 投影逻辑

本 story 仅落地 AppState 类与基础 hydrate / reset / mutation API；Scaffold ViewModel 基类设计在 Story 37.7-37.11 各自负责。

### Source tree 改动汇总

```
[新增]
iphone/PetApp/App/AppState.swift                                       (类 + applyHomeData / reset / setters)
iphone/PetAppTests/Helpers/AppStateTestHelpers.swift                   (extension + makeSampleHomeData)
iphone/PetAppTests/App/AppStateTests.swift                             (≥6 case)

[修改]
iphone/PetApp/App/RootView.swift                                       (加 @StateObject AppState + .environmentObject + bind(appState:) + bootstrap 改 appState.applyHomeData)
iphone/PetApp/App/AppCoordinator.swift                                 (删 currentRoomId 字段 + init 参数 + 注释)
iphone/PetApp/App/AppContainer.swift                                   (makeResetIdentityViewModel 签名透传 appState)
iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift             (删 homeData 字段 + 加 weak appState + bind(appState:) + applyHomeData 内部改写)
iphone/PetApp/Features/Home/Views/HomeView.swift                       (加 @EnvironmentObject AppState + 三处 viewModel.homeData 改 appState.* + HomePetNameResolver 签名改造)
iphone/PetApp/Features/Home/Views/HomeContainerView.swift              (删 coordinator + 加 appState + body 内引用切换)
iphone/PetApp/Features/DevTools/ViewModels/ResetIdentityViewModel.swift (加 appState 字段 + tap() 内 reset)
iphone/PetAppTests/App/AppCoordinatorTests.swift                       (删 currentRoomId 2 case)
iphone/PetAppTests/App/RootViewWireTests.swift                         (改写 currentRoomId 2 case → AppState 断言版)
iphone/PetAppTests/Features/Home/ViewModels/HomeViewModelLoadHomeTests.swift (5 case 改写：appState 断言)
iphone/PetAppTests/Features/Home/HomePetNameResolverTests.swift        (全 case 改入参 (pet:hasHydrated:))
iphone/PetAppTests/Features/Home/HomeViewModelTests.swift              (如有引用 homeData 字段的 case 同步删除 / 改写)
iphone/PetAppTests/Features/Home/HomeViewTests.swift                   (如有 #if DEBUG Preview 路径或 viewModel.homeData 引用 → 改 appState 注入)
_bmad-output/implementation-artifacts/37-4-appstate-实装-loadhome-迁移.md (本 story 文件)
_bmad-output/implementation-artifacts/sprint-status.yaml               (workflow 自动改：ready-for-dev → in-progress → review)

[删除]
（无文件删除；HomeViewModel.homeData / AppCoordinator.currentRoomId 是字段级删除）
```

### 测试标准（与 ADR-0002 §3.1 + Story 2.7 测试基础设施一致）

- **不引入** SnapshotTesting / ViewInspector（ADR-0002 §3.1 严守红线）
- **抽决策逻辑为纯函数**让 XCTest 直接覆盖（HomeRoomDispatcher / HomePetNameResolver / HomeNicknameResolver 与本 story HomePetNameResolver 改造同精神）
- **AppState 测试**用 @MainActor + 直接构造实例 + assert @Published 字段值（不用 Combine sink；@Published 在 MainActor 同步赋值后立即可读）
- **MockAppState 不走子类路径**（AppState 是 final class）→ 用 extension testing helper（与 SessionStore 同模式）
- **跑** `bash iphone/scripts/build.sh --test` 是 done 硬条件；如失败必须同 commit 修复

### 与 ADR-0010 §3 严格映射

| ADR-0010 §3 决策 | 本 story AC | Tasks |
|---|---|---|
| 3.1 AppState 类型与生命周期 + 注入规则 | AC1 + AC4 + AC7 | Task 2 + Task 6 + Task 5 |
| 3.2 AppState 范围（白名单 7 字段 + 类型选择） | AC1 + Dev Notes "类型选择" | Task 2 |
| 3.3 hydrate 流程（启动 + WS + REST mutation） | AC3 + AC1 mutation API | Task 7 + Task 2 |
| 3.4 与 SessionStore 关系（并行边界） | Dev Notes（不直接动 SessionStore，但 ResetIdentityViewModel 双调） | Task 7 |
| 3.5 ViewModel 演变模式（HomeViewModel 关键变化） | AC2 | Task 4 |
| 3.6 测试影响（AppStateTests + ViewModel 测试简化 + 集成测试改写） | AC8 | Task 3 + Task 8 |
| 3.7 Reset 流程 | AC6 + AppStateTests case#2 | Task 7 + Task 3 |
| 4.1 Story 5.5 supersede 路径（completely supersedes） | 故事顶部 + AC2 / AC5 / AC7 / AC8 | 全 Tasks |

每条 ADR §3 决策都有对应的 AC + Task；如 dev 实装期发现某决策实际不可行 → 走 ADR 修订 patch + 改 v2 Accepted 路径（与本 story commit 同 commit）。

### Project Structure Notes

新文件全部按 ADR-0002 §3.3 工程目录方案落地：

```
iphone/PetApp/
├─ App/                       # AppState (新增) / AppCoordinator (修改) / RootView (修改) / AppContainer (修改) - 应用启动 / 协调层
├─ Features/
│  ├─ Home/
│  │  ├─ ViewModels/HomeViewModel.swift    # 修改：删 homeData + 加 weak appState
│  │  └─ Views/
│  │     ├─ HomeView.swift                 # 修改：加 @EnvironmentObject AppState
│  │     └─ HomeContainerView.swift         # 修改：删 coordinator + 加 appState
│  └─ DevTools/ViewModels/ResetIdentityViewModel.swift  # 修改：加 appState + tap() 内 reset
iphone/PetAppTests/
├─ App/
│  ├─ AppStateTests.swift                  # 新增（≥6 case）
│  ├─ AppCoordinatorTests.swift            # 修改：删 currentRoomId 2 case
│  └─ RootViewWireTests.swift               # 修改：改写 currentRoomId 2 case
├─ Helpers/
│  └─ AppStateTestHelpers.swift             # 新增（extension + makeSampleHomeData）
└─ Features/Home/
   ├─ HomePetNameResolverTests.swift        # 修改：全 case 改入参
   ├─ HomeViewTests.swift                   # 修改：appState 注入
   ├─ HomeViewModelTests.swift              # 修改：删 homeData 引用
   └─ ViewModels/HomeViewModelLoadHomeTests.swift  # 修改：5 case 改 appState 断言
```

`Tests/Helpers/` 子目录与 Story 2.7 钦定的测试基础设施目录一致；新加 `AppStateTestHelpers.swift` 与现有 `AsyncTestHelpers.swift` / `MockBase.swift` 同级。

### References

- [Source: epics.md#Story-37.4](../planning-artifacts/epics.md) §Story 37.4 — Acceptance Criteria 原文（第 4642-4670 行）
- [Source: epics.md#Epic-37](../planning-artifacts/epics.md) §Epic 37 概览 — 红线（含 37.3/37.4 重构 story 红线豁免）+ Story 依赖链 + 接缝设计（第 4555-4573 行）
- [Source: ADR-0010](decisions/0010-iphone-app-state.md) — 本 story 主依据；§2 Decision Summary / §3.1 注入规则 / §3.2 AppState 范围（含 currentRoomId: String? 类型）/ §3.3 hydrate 流程 / §3.4 SessionStore 边界 / §3.5 ViewModel 演变模式 / §3.6 测试影响 / §3.7 Reset 流程 / §4.1 supersede 语义 / §4.3 节点 fill 表 / §4.4 已知坑 / §6 验收
- [Source: ADR-0009](decisions/0009-iphone-navigation-tabview.md) — 联动 ADR；§3.4 AppCoordinator 角色变化（含 currentTab 归 AppCoordinator，本 story 不动 currentTab/switchTab）
- [Source: 37-2-adr-0010-appstate.md](37-2-adr-0010-appstate.md) — Story 37.2（已 done）；锁定 ADR-0010 contract validity（含 path B verification model）
- [Source: 37-3-rootview-maintabview-改造.md](37-3-rootview-maintabview-改造.md) — Story 37.3（已 done）；含 currentRoomId 临时占位字段引入 + bootstrap closure 双写模式 + Completion Notes #1 协调约定
- [Source: sprint-change-proposal-2026-04-29-v2.md](../planning-artifacts/sprint-change-proposal-2026-04-29-v2.md) — Sprint Change v2.5 终审依据（commit `bef4531`）；§5.1 Story 37.4 详细 acceptance / §6 风险闭环 / 用户前置 6 决议 #5=B AppState 顶替 homeData
- [Source: 5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据.md](5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据.md) — Story 5.5 钦定的 LoadHomeUseCase / HomeRepository / GET /home 调用契约（本 story 必须保留这套契约）；同时 status 已改 superseded（数据持有部分钦定 dead）
- [Source: ADR-0002 §3.1 + §3.3](decisions/0002-ios-stack.md) — iPhone 测试栈（禁 ViewInspector / SnapshotTesting）+ 工程目录方案
- [Source: ADR-0008-v2](decisions/0008-error-protocol.md) — 先例：ADR 修订 + 改 Accepted 同 commit 模式（参考 commit `ec5beb3`，本 story 如需 ADR-0010 修订 patch 走该模式）
- [Source: CLAUDE.md](../../CLAUDE.md) — 本仓库工作纪律 + iPhone 端工程目录由 ADR-0002 锁定 + 端独立原则
- [Source: docs/lessons/2026-04-26-stateobject-init-vs-bind-injection.md] — bind(pingUseCase:) lesson；本 story bind(appState:) 同模式
- [Source: docs/lessons/2026-04-26-swiftui-task-modifier-reentrancy.md] — hasFetched / hasLoadedHome 跨 task 边界短路 lesson；本 story HomeViewModel.applyHomeData 双重职责段引此 lesson
- [Source: docs/lessons/2026-04-30-coordinator-must-mirror-loaded-home-room-state.md] — Story 37.3 引入双写模式的 lesson；本 story 收口（删除 coordinator.currentRoomId）后 lesson 应 archive 标记 superseded（dev 自决是否同 commit 改 lesson 文件状态）

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) — `claude-opus-4-7[1m]`

### Debug Log References

无（一遍 build pass 244/244，无 debug session）。

### Completion Notes List

1. **AppState 类落地（AC1）**：新建 `iphone/PetApp/App/AppState.swift`，按 ADR-0010 §3.2 白名单 7 字段（节点 1 阶段相关 5 字段 + 节点 6/8/9 占位 3 字段）+ `applyHomeData(_:)` / `reset()` / `setCurrentRoomId(_:)` / `updateMyPetState(_:)` / `updateCurrentEquips(_:)` mutation API；`@MainActor + final + ObservableObject + @Published` 各字段；类型选择遵循 Story 37.4 Dev Notes "类型选择"段直接复用 `Home*` 类型族.
2. **AppStateTests ≥7 case（AC8 ≥7 case 红线满足）**：覆盖 hydrate / reset / setCurrentRoomId 双向 / updateCurrentEquips 替换 / updateMyPetState（has-pet / pet=nil 兜底）/ 初始空态.
3. **AppStateTestHelpers**：用 extension 形式给 AppState 加 makeHydrated / makeReset 静态构造器（AppState 是 final class 不可继承 → 不走 MockAppState 子类路径）；`makeSampleHomeData(currentRoomId:)` 共享 helper 给 AppStateTests + 后续 Scaffold 测试复用.
4. **HomeViewModel.homeData 字段整段删除（AC2）**：旧 `@Published var homeData: HomeData?` 整段直接删；新增 `private weak var appState: AppState?` + `bind(appState:)` 单次绑定方法；`applyHomeData(_:)` 内部把 `self.homeData = data` 改为 `self.appState?.applyHomeData(data)`，但保留 `loadingState = .loaded` + `hasLoadedHome = true` transient flag（ADR-0010 §3.2 表格钦定 loading state 不进 AppState）；三个 init 追加 `appState: AppState? = nil` 默认参数（向后兼容老调用方）.
5. **HomeView 改读 AppState（AC7）**：HomeView 加 `@EnvironmentObject var appState: AppState`（ADR-0010 §3.1 例外条款：纯展示 SwiftUI View 可直接 @EnvironmentObject）；三处 `viewModel.homeData?.X` 改读 `appState.*` 投影（pet name / chest remaining / step balance）.
6. **HomePetNameResolver 签名改造（AC7）**：`resolve(homeData:)` → `resolve(pet:hasHydrated:)`，三态语义保持（loading / no-pet / has-pet）；`hasHydrated` 用 `appState.currentUser != nil` 派生（hydrate 完成后 user 必非 nil；与原 homeData != nil 等价）.
7. **HomeContainerView 改读 AppState（AC4）**：`@EnvironmentObject var coordinator: AppCoordinator` 删，改 `@EnvironmentObject var appState: AppState`；body 内两处 `coordinator.currentRoomId` → `appState.currentRoomId`.
8. **AppCoordinator.currentRoomId 临时占位字段删除（AC5）**：字段 + init 默认参数 + 文件头注释相关段一并删；编译器驱动改 caller（HomeContainerView / RootView bootstrap closure / AppCoordinatorTests / RootViewWireTests）一遍 pass，未做 grep 兜底（Story 文件 X1+X2 强约束）.
9. **RootView AppState wire（AC3）**：新增 `@StateObject private var appState = AppState()`；`LaunchedContentView` 加 `appState: AppState` 参数 + `.environmentObject(appState)` 注入到 .ready 子树；body 内 `.task` 闭包加 `homeViewModel.bind(appState: appState)`；bootstrap closure 把 `coordinator.currentRoomId = ...` 改为 `appState.applyHomeData(homeData)` 单写 + 保留 `homeViewModel.applyHomeData(homeData)` 双重调用（HomeViewModel 内 hasLoadedHome 短路 flag 仍生效；同一 AppState 实例 idempotent 赋值）.
10. **ResetIdentityViewModel 加 appState.reset()（AC6 / ADR-0010 §3.7）**：新增 `appState: AppState? = nil` 字段 + init 参数；`tap()` 成功路径在 `sessionStore?.clear()` 之后追加 `appState?.reset()` 调用；AppContainer.makeResetIdentityViewModel 签名追加 `appState: AppState? = nil` 默认参数透传给 ViewModel.
11. **测试改写（AC8）**：
    - `AppCoordinatorTests` 删 `testCurrentRoomIdDefaultsToNil` / `testCurrentRoomIdCanBeAssigned` 两 case
    - `RootViewWireTests` 改写 `testBootstrapPropagatesLoadedHomeRoomIdToCoordinator` → `...ToAppState`，`testBootstrapKeepsCoordinatorCurrentRoomIdNilWhenHomeRoomIsEmpty` → `testBootstrapKeepsAppStateCurrentRoomIdNilWhenHomeRoomIsEmpty`；intent 完全保留（bootstrap 完成后 currentRoomId 必须传播）
    - `HomeViewModelLoadHomeTests` 改写 8 case：构造 AppState + viewModel.bind(appState:) + 断言 appState.* 而非 viewModel.homeData
    - `HomePetNameResolverTests` 改入参形式 `(pet:hasHydrated:)`；新增 testResolveReturnsLoadingPlaceholderWhenNotHydratedEvenIfPetPresent 边界 case（hasHydrated guard 优先）
    - `HomeViewTests` 给 UIHostingController 加 .environmentObject(AppState()) 让渲染不 crash
12. **HomeView_Previews + MainTabView_Previews 同步**：注入 AppState 实例避免 SwiftUI Preview crash.
13. **Build / Test 验证（AC9 unit 部分）**：`bash iphone/scripts/build.sh --test` 一次性 pass，**244/244 unit tests 全绿**（baseline 238 + 本 story 净增 6：AppStateTests 7 - AppCoordinatorTests 删 2 + HomePetNameResolverTests 新增 1 = 6）；HomeViewModelLoadHomeTests / RootViewWireTests / HomePetNameResolverTests / AppCoordinatorTests / HomeContainerViewTests 全部既有 case 无 regression. UITest + release build 跑由后续 fix-review / story-done 阶段负责（dev-story workflow 范围内仅跑 unit test）.

### File List

**新增文件**：
- `iphone/PetApp/App/AppState.swift`（NEW）
- `iphone/PetAppTests/Helpers/AppStateTestHelpers.swift`（NEW）
- `iphone/PetAppTests/App/AppStateTests.swift`（NEW，7 case）

**修改文件**：
- `iphone/PetApp/App/RootView.swift`（加 @StateObject AppState + .environmentObject + bind(appState:) + bootstrap 改 appState.applyHomeData）
- `iphone/PetApp/App/AppCoordinator.swift`（删 currentRoomId 字段 + init 参数 + 注释）
- `iphone/PetApp/App/AppContainer.swift`（makeResetIdentityViewModel 签名追加 appState 参数）
- `iphone/PetApp/App/MainTabView.swift`（Preview 注入 AppState + HomeViewModel）
- `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`（删 homeData 字段 + 加 weak appState + bind(appState:) + applyHomeData 内部改写）
- `iphone/PetApp/Features/Home/Views/HomeView.swift`（加 @EnvironmentObject AppState + 三处 viewModel.homeData 改 appState.* + HomePetNameResolver 签名改造 + Preview 注入）
- `iphone/PetApp/Features/Home/Views/HomeContainerView.swift`（删 coordinator + 加 appState + body 内引用切换）
- `iphone/PetApp/Features/DevTools/ViewModels/ResetIdentityViewModel.swift`（加 appState 字段 + tap() 内 appState?.reset() 调用）
- `iphone/PetAppTests/App/AppCoordinatorTests.swift`（删 currentRoomId 2 case）
- `iphone/PetAppTests/App/RootViewWireTests.swift`（改写 currentRoomId 2 case → AppState 断言版）
- `iphone/PetAppTests/Features/Home/ViewModels/HomeViewModelLoadHomeTests.swift`（8 case 改写：appState 注入 + 断言）
- `iphone/PetAppTests/Features/Home/HomePetNameResolverTests.swift`（4 case 改入参 (pet:hasHydrated:) + 新增 1 边界 case）
- `iphone/PetAppTests/Features/Home/HomeViewTests.swift`（UIHostingController 注入 .environmentObject(AppState())）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（37-4 状态：ready-for-dev → in-progress → review）
- `_bmad-output/implementation-artifacts/37-4-appstate-实装-loadhome-迁移.md`（本 story 文件 dev agent record + Status）

### Change Log

| Date | Change | Notes |
|---|---|---|
| 2026-04-30 | Story 37.4 落地（AppState 实装 + LoadHome 迁移）| AppState 类 + AppStateTests 7 case + HomeViewModel.homeData 字段删除 + AppCoordinator.currentRoomId 临时占位字段删除 + RootView/HomeView/HomeContainerView/ResetIdentityViewModel/AppContainer 配套改造；244 unit tests 全绿 |
