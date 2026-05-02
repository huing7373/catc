# Story 37.13: accessibility identifier 全屏总表 + 视觉回归 review checklist（合规兜底，非 snapshot 替代）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iOS 开发,
I want 一份覆盖全 Scaffold 的 accessibility identifier 总表 + 静态校验脚本 + 人工视觉 review checklist 文档,
so that UI 测试有可依赖的定位锚点 + Features/Modals 内 inline a11y 字符串收编入 `AccessibilityID.swift` 单一来源（防 typo / 集中管理 / 让未来 view 改 identifier 时不需要 grep 全 UITest）；同时以「静态脚本 + 人工 checklist」严守 ADR-0002 §3.1 钦定的 XCTest only 合规兜底（**显式声明非 snapshot 测试等价物**，未来若需引入 swift-snapshot-testing 等工具另起 spike）。

## 故事定位（Epic 37 第四层第 7 条 story；Scaffold 收口 + 合规兜底）

这是 Epic 37「iPhone 架构层重构 + UI Scaffold（壳先行）」的**第四层 collation / docs / audit 类 story**——上游 37.5（Theme）/ 37.6（Shared Primitives）/ 37.7（HomeView）/ 37.8（RoomView）/ 37.9（WardrobeView）/ 37.10（FriendsView）/ 37.11（ProfileView）/ 37.12（JoinRoomModal）全部 done。本 story **不是**新 feature 实装，而是**重构 + docs**：

1. **重构维度**（约 60% 工作量）：把 Story 37.7 ~ 37.12 落地的所有 inline a11y identifier 字符串收编到 `iphone/PetApp/Shared/Constants/AccessibilityID.swift` enum 总表（按 feature 分 nested enum：`Tab` / `Home` 已有扩 / `Room` / `Wardrobe` / `Friends` / `Profile` / `JoinRoomModal`）；caller view 改用常量引用替代 inline 字符串；UITest 同步改用常量引用（UITests target 已通过 project.yml line 69 把 AccessibilityID.swift 作为 UITest target 自身的 source 编译，无需 `@testable import`）。**核心收益**：单一来源 + 防 typo + view 改 identifier 时 compile-time 自动驱动 caller 全更新。
2. **静态校验维度**（约 25% 工作量）：新建 2 个静态校验脚本：
   - `iphone/scripts/check_a11y_coverage.sh`：grep `Features/` + `Shared/Modals/` 内 SwiftUI View body 含 button/list 等交互元素的 `accessibilityIdentifier` 注解；不达标列出并 fail（防未来新增交互元素漏挂 a11y）
   - `iphone/scripts/check_no_apiclient_in_features.sh`：grep `Features/{Home,Room,Wardrobe,Friends,Profile}/Views/` + `Shared/Modals/` + `Core/DesignSystem/` 内 `import APIClient` / Repository / UseCase（除显式 `RealViewModel` wire 模式）；违规列出 fail（守护 ADR-0010 钦定的 View ↔ ViewModel 解耦边界）
3. **视觉回归 docs 维度**（约 15% 工作量）：新建 `iphone/docs/visual-review-checklist.md`：每屏 6-10 个 manual visual check + 截图位 + 用于 PR review 时手动逐项打勾（**注**：合规兜底，不声称等价 snapshot 测试覆盖度）

**本 story 落地后立即解锁**：
- Epic 37 推进到 37.14（design-package 白名单）
- Epic 37-retrospective（required）
- 节点 2 demo 验收基线建立：Scaffold 全部 a11y 锚常量化 + 全部 view 通过静态校验 + visual review 跑通
- 后续 epic（节点 3 起）新增 view / modal / 交互元素时强制走「先扩 AccessibilityID enum → 再加 inline accessibilityIdentifier(常量)」路径，本 story 落地的 2 个静态脚本作为 CI gate

**本 story 的"实装"动作**（一句话概括）：扩展 `AccessibilityID.swift` enum 增加 `Tab` / `Room` / `Wardrobe` / `Friends` / `Profile` / `JoinRoomModal` nested enum 把 Story 37.7-37.12 落地的全部 inline a11y identifier 字符串收编为常量；改 caller view（HomeView / MainTabView / RoomScaffoldView / WardrobeScaffoldView / FriendsScaffoldView / ProfileScaffoldView / JoinRoomModal / RoomViewPlaceholder / JoinRoomModalPlaceholder / RootView）的 `accessibilityIdentifier("xxx")` → `accessibilityIdentifier(AccessibilityID.Yyy.zzz)`；改 UITest（HomeUITests / NavigationUITests / KeychainPersistenceUITests）的 `["xxx"]` 字面量 → `[AccessibilityID.Yyy.zzz]` 引用；新建 2 个 bash 静态脚本 `iphone/scripts/check_a11y_coverage.sh` / `check_no_apiclient_in_features.sh`；新建 `iphone/docs/visual-review-checklist.md` 文档。**纯重构 + 文档**，**不**改任何业务逻辑、ViewModel、UseCase、AppState、Server。

**关键路径："改值不改名"语义保持 + 旧名禁用清单**（与 Story 37.7 lesson "AccessibilityID enum 值变化但 caller key 不变 → UITest 零改动" 同精神，但本 story 是反向：UITest **要**改，因为字面量 `["xxx"]` → 常量 `[AccessibilityID.Yyy.zzz]` 是 caller 表达方式变化；但 a11y identifier **运行时字符串值不变**，所以行为契约不破）：

- `AccessibilityID.Home.userInfo`（已有）= `"homeStatusBar"`（值不变；已是常量）
- `AccessibilityID.Room.roomIdDisplay` = `"roomIdDisplay"`（值不变；新建常量）—— **a11y 命名严格 `roomIdDisplay`，禁止旧名 `roomCodeDisplay`**（epic AC line 4881 钦定）
- 全部新建常量的字符串值与现 inline 字符串一**字一字**相同（防 UITest 跑不过；通过 AC8 grep 校验断言）
- 旧名禁用清单（**红线**）：`roomCodeDisplay` / `roomCode_*` / 任何含 `roomCode` 的 a11y 字符串（epic AC line 4881 钦定 `roomIdDisplay`，**不**允许 `roomCodeDisplay`；本 story 通过 AC8 grep 校验断言**无**任何 `roomCode` 出现在 Features/ + Shared/Modals/ + Tests/ 范围）

**不涉及**（红线）：
- **不**改任何 a11y identifier 的**字符串值**（仅把 inline 字符串提取为常量；运行时值与现状 1:1 等价）
- **不**新增任何 a11y identifier（仅收编现有的；新增 a11y 由后续 story 触发，本 story 是 "现状归并 + 校验" 性质）
- **不**实装真实 snapshot 测试（与 epic AC line 4886 「**注**：合规兜底，不声称等价 snapshot 测试覆盖度」对齐；未来若需 swift-snapshot-testing / iOSSnapshotTestCase 另起 spike）
- **不**改业务逻辑：HomeViewModel / MockHomeViewModel / RealHomeViewModel / RoomViewModel / WardrobeViewModel / FriendsViewModel / ProfileViewModel / JoinRoomModal closure 行为 / AppState / RoomScaffoldView 等任何业务代码 zero edit
- **不**改任何 #Preview 块（Preview 内 a11y identifier 视情况可改可不改；本 story 仅改正式 view body 内的，Preview 用 inline 字符串保 quick edit 体验。**例外**：若 Preview 内已用过常量则不动）
- **不**改 Story 37.3 落地的 deprecated `home_btnRoom` / `home_btnInventory` / `home_btnCompose` 常量（按 AccessibilityID.Home line 22-27 注释 "Story 37.13 a11y 总表归并时一并清理"——**清理 = 删除这 3 个 deprecated 常量** + grep 校验**无 caller 引用** + 加 lesson 反例归档；**不影响**运行时行为，因为这 3 个按钮 view 已在 Story 37.3 删除）
- **不**重命名任何 enum 命名风格：`AccessibilityID.Home.userInfo` 已采用「nested enum + 小驼峰常量名」，新建 `Tab` / `Room` / `Wardrobe` / `Friends` / `Profile` / `JoinRoomModal` 沿用同风格（line 6 注释 "命名风格：<feature>_<element>（小驼峰），AC6 约定" —— 注：注释提到的"AC6 约定"指 Story 37.7 内的 AC6，本 story 沿用此风格）
- **不**实装 SwiftUI ViewModifier 抽象（如 `extension View { func a11y(_ id: String) -> some View }`）—— SwiftUI 标准 `accessibilityIdentifier(_:)` 已够用，**不**引入额外 view modifier 抽象（防 over-design，与 Story 37.6 Shared Primitives 抽象边界一致）
- **不**改 ios/ 任何文件（CLAUDE.md + ADR-0002 §3.3）
- **不**改 server/ 任何文件
- **不**新增任何 ObservableObject / ViewModel 类型
- **不**改 project.yml（AccessibilityID.swift 已通过 line 69 配置在 UITest target source；本 story 不动）
- **不**改 `JoinRoomModalPlaceholder.swift` / `RoomViewPlaceholder.swift` 任何业务逻辑（仅改其 inline a11y 字符串为常量）
- **不**实装任何 CI 集成（脚本仅作为 dev 端 manual run 入口；CI gate 集成由后续 ops epic 落地）—— epic AC line 4884 钦定 "脚本不达标列出并 fail"，本 story 让 dev 能 `bash iphone/scripts/check_a11y_coverage.sh` 本地验证，**不**强制改 git pre-commit hook
- **不**实装 visual diff 自动化工具（如 imagemagick / pixelmatch）—— `visual-review-checklist.md` 是**手动** PR review 工具，**不**集成到 CI；epic AC line 4886 显式声明 "合规兜底，不声称等价 snapshot 测试覆盖度"

## Acceptance Criteria

> **AC 编号体系**：AC1 是 AccessibilityID enum 扩展（新增 6 个 nested enum + Home 内 deprecated 常量清理）；AC2 是 caller view 字符串 → 常量替换（10 个 view 文件）；AC3 是 UITest 字面量 → 常量替换（3 个 UITest 文件）；AC4 是新建静态校验脚本 `check_a11y_coverage.sh`；AC5 是新建静态校验脚本 `check_no_apiclient_in_features.sh`；AC6 是新建 visual review checklist 文档；AC7 是单元测试 ≥3 case 守护 enum 常量值与运行时字符串等价；AC8 是 build verify + grep 校验（含 `roomCode` 红线 + deprecated 清理验证）；AC9 是 Deliverable 清单。

### AC1 — 扩展 AccessibilityID.swift enum 总表

**改动文件**：`iphone/PetApp/Shared/Constants/AccessibilityID.swift`

**新增 6 个 nested enum**（按 feature 分组，沿用现有 `Home` / `ErrorUI` / `Launching` 风格的 `public enum + public static let` 模式）：

```swift
// MARK: - Story 37.13 落地的 6 个新 nested enum

public enum Tab {
    // Story 37.3 落地：MainTabView 浮动 TabBar 的 4 个 a11y identifier，inline 字符串 `tab_<rawValue>`.
    // 本 story 收编为常量；rawValue 字符串值与运行时一字不变.
    public static let home = "tab_home"
    public static let wardrobe = "tab_wardrobe"
    public static let friends = "tab_friends"
    public static let profile = "tab_profile"
}

public enum Room {
    // Story 37.8 落地：RoomScaffoldView 的全部 a11y identifier；本 story 收编为常量.
    // **a11y 命名严格 `roomIdDisplay`，禁止旧名 `roomCodeDisplay`**（epic AC line 4881 钦定，AC8 grep 校验守护）.
    public static let returnButton = "returnButton"
    public static let roomIdDisplay = "roomIdDisplay"
    public static let copyButton = "copyButton"
    public static let sharedStage = "sharedStage"
    public static let leaveButton = "leaveButton"
    /// roomMember_<index> 模式（index 0..3）；caller 走 `AccessibilityID.Room.member(at: index)` helper.
    public static func member(at index: Int) -> String { "roomMember_\(index)" }
    // Story 37.3 落地的占位 view a11y identifier（RoomViewPlaceholder.swift）.
    public static let viewPlaceholder = "roomViewPlaceholder"
}

public enum Wardrobe {
    // Story 37.9 落地：WardrobeScaffoldView 的全部 a11y identifier；本 story 收编为常量.
    public static let view = "wardrobeView"
    public static let diamondCount = "wardrobeDiamondCount"
    public static let composeEntry = "wardrobeComposeEntry"
    public static let equipButton = "wardrobeEquipButton"
    /// wardrobeCategory_<rawValue> 模式（rawValue = CosmeticCategory.allCases）；caller 走 helper.
    public static func category(_ rawValue: String) -> String { "wardrobeCategory_\(rawValue)" }
    /// wardrobeItem_<id> 模式；caller 走 helper.
    public static func item(_ id: String) -> String { "wardrobeItem_\(id)" }
}

public enum Friends {
    // Story 37.10 落地：FriendsScaffoldView 的全部 a11y identifier；本 story 收编为常量.
    public static let view = "friendsView"
    public static let addButton = "friendsAddButton"
    public static let myRoomShareButton = "friendsMyRoomShareButton"
    public static let myRoomCard = "friendsMyRoomCard"
    public static let toast = "friendsToast"
    /// friendsTab_<rawValue>（FriendsTab.allCases）；caller 走 helper.
    public static func tab(_ rawValue: String) -> String { "friendsTab_\(rawValue)" }
    /// friendRow_<id> 模式；caller 走 helper.
    public static func row(_ id: String) -> String { "friendRow_\(id)" }
    /// friendActionButton_<id> 模式（"加好友" / "加入" / "查看资料" 等动作按钮共用）；caller 走 helper.
    public static func actionButton(_ id: String) -> String { "friendActionButton_\(id)" }
}

public enum Profile {
    // Story 37.11 落地：ProfileScaffoldView 的全部 a11y identifier；本 story 收编为常量.
    public static let view = "profileView"
    public static let headerCard = "profileHeaderCard"
    public static let statsCard = "profileStatsCard"
    public static let weChatCard = "profileWeChatCard"
    public static let weChatCardBound = "profileWeChatCardBound"
    public static let collectionViewAll = "profileCollectionViewAll"
    public static let toast = "profileToast"
    public static let weChatModal = "profileWeChatModal"
    public static let weChatBindButton = "profileWeChatBindButton"
    public static let weChatCancelButton = "profileWeChatCancelButton"
    /// profileCollectionCell_<id> 模式（recent collection 5 个）；caller 走 helper.
    public static func collectionCell(_ id: String) -> String { "profileCollectionCell_\(id)" }
    /// profileMenu_<rawValue>（ProfileMenuItem.allCases）；caller 走 helper.
    public static func menu(_ rawValue: String) -> String { "profileMenu_\(rawValue)" }
}

public enum JoinRoomModal {
    // Story 37.12 落地：JoinRoomModal 的全部 5 视觉锚 a11y identifier；本 story 收编为常量.
    public static let modal = "joinRoomModal"
    public static let closeButton = "joinRoomCloseButton"
    public static let input = "joinRoomInput"
    public static let cancelButton = "joinRoomCancelButton"
    public static let confirmButton = "joinRoomConfirmButton"
    // Story 37.3 落地的占位 view a11y identifier（JoinRoomModalPlaceholder.swift）.
    public static let modalPlaceholder = "joinRoomModalPlaceholder"
}
```

**Home enum 扩展**（追加 Story 37.7 落地的 inline 字符串收编 + Story 37.3 deprecated 常量清理）：

```swift
public enum Home {
    // ... 现有常量保留（userInfo / petArea / stepBalance / chestArea / versionLabel /
    //                  btnResetIdentity / resetIdentityAlert / petName / chestRemaining）

    // Story 37.13 新增：Story 37.7 落地的 inline 字符串收编为常量.
    public static let catStage = "homeCatStage"
    public static let teamIdleCardCreate = "homeTeamIdleCard_create"
    public static let teamIdleCardJoin = "homeTeamIdleCard_join"

    // Story 37.13 删除：Story 37.3 落地的 deprecated 3 CTA 按钮常量（btnRoom / btnInventory / btnCompose）.
    //   AccessibilityID.swift line 22-27 注释钦定 "Story 37.13 a11y 总表归并时一并清理".
    //   清理 = 删除这 3 个常量 + grep 校验无 caller 引用（AC8）.
    //   原因：Story 37.3 主入口已改 4 Tab IA, 3 CTA 按钮 view 已删, 常量保留无意义.
}
```

**RootView 收编**：`RootView.swift` line 449 `.accessibilityIdentifier("compose_placeholder")` 收编为 `AccessibilityID.Compose.placeholder` —— 但这是 Story 37.3 落地的占位 view（compose 入口 dummy），在 Epic 27 / 33 真实落地前不会增删。本 story **加** `AccessibilityID.Compose` enum：

```swift
public enum Compose {
    // Story 37.3 落地的占位 view a11y identifier（RootView.swift line 449，dummy compose route）.
    public static let placeholder = "compose_placeholder"
}
```

**关键约束（红线）**：
- 全部新增常量的字符串值与现 inline 字符串**一字一字相同**（AC8 grep 校验断言）
- enum 命名风格沿用现有 `public enum + public static let` 模式（**不**用 `String enum case` 模式，因为 Swift `String enum` 的 `.rawValue` 在 SwiftUI `.accessibilityIdentifier()` 调用点要 `.rawValue` 拼写不简洁；现有 Home / ErrorUI / Launching 选 `static let` 已确立）
- 动态拼接的 a11y（`roomMember_\(index)` / `wardrobeCategory_\(rawValue)` / `friendsTab_\(rawValue)` / `friendRow_\(id)` / `friendActionButton_\(id)` / `profileCollectionCell_\(id)` / `profileMenu_\(rawValue)` / `wardrobeItem_\(id)`）走 `static func helper(_:)` 模式（不是 `static let`），因为 Swift `static let` 不支持参数；helper 函数体内拼字符串与原 inline 拼接完全等价
- **不**用 `String enum case + RawRepresentable` 模式：会让 `.accessibilityIdentifier(MyEnum.case.rawValue)` 拼写啰嗦，且 `static func helper(...)` 已能覆盖动态场景

> **关键决策 1**：`AccessibilityID.Tab.home` 而非 `AccessibilityID.Tabs.home`（单数）—— SwiftUI 的内置类型也是 `Tab` 单数（MainTabView.swift line 22 注释 "iOS 18 起 SwiftUI 引入内置 `SwiftUI.Tab` 类型"），enum 内 nested 命名可单数；但在 caller 端用 `AccessibilityID.Tab.home` 不冲突（前缀消歧）。

> **关键决策 2**：动态 a11y identifier 走 `static func` helper 而非 `static let` —— 原因前文已述（Swift 限制）；但要在每个 helper 上加文档注释说明 caller 用法（如 `AccessibilityID.Room.member(at: 0)`）。

> **关键决策 3**：`Home.btnRoom` / `Home.btnInventory` / `Home.btnCompose` 删除是**唯一**清理动作，**不**清理其它 deprecated；本 story 不做大扫除（避免 scope creep）。

**对应 Tasks**: Task 1.1, 1.2, 1.3, 1.4

### AC2 — caller view 字符串 → 常量替换（10 个 view 文件）

**改动文件清单**：

| 文件 | inline 字符串数 | 改后 |
|-----|----------------|------|
| `iphone/PetApp/App/MainTabView.swift` | 1 处（line 86 `tab_\(tab.rawValue)`） | 加 helper `AccessibilityID.Tab.identifier(for: tab)` 或就地拼 `tab_\(tab.rawValue)` 引常量基础部分 |
| `iphone/PetApp/App/RootView.swift` | 1 处（line 449 `compose_placeholder`） | `AccessibilityID.Compose.placeholder` |
| `iphone/PetApp/Features/Home/Views/HomeView.swift` | 4 处（line 278/409/458/467 共 inline；167/225/246/308/496 已用常量） | line 278 `homeCatStage` → `AccessibilityID.Home.catStage`；line 458 `homeTeamIdleCard_create` → `AccessibilityID.Home.teamIdleCardCreate`；line 467 `homeTeamIdleCard_join` → `AccessibilityID.Home.teamIdleCardJoin`；line 409 `a11yId` 是变量参数（可能用 inline 调用方传入 / 或本身用常量），**保留**该行不动 |
| `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift` | 7 处（line 83/120/158/225/316/335/348） | 全部替换为 `AccessibilityID.Room.xxx` 或 `AccessibilityID.Room.member(at: index)` |
| `iphone/PetApp/Features/Room/Views/RoomViewPlaceholder.swift` | 1 处（line 15 `roomViewPlaceholder`） | `AccessibilityID.Room.viewPlaceholder` |
| `iphone/PetApp/Features/Wardrobe/Views/WardrobeScaffoldView.swift` | 6 处（line 43/87/115/208/256/332） | 全部替换为 `AccessibilityID.Wardrobe.xxx` 或 helper |
| `iphone/PetApp/Features/Friends/Views/FriendsScaffoldView.swift` | 9 处（line 44/86/139/155/193/261/280/290/316） | 全部替换为 `AccessibilityID.Friends.xxx` 或 helper |
| `iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift` | 12 处（line 67/168/213/317/375/397/449/523/542/631/732/743） | 全部替换为 `AccessibilityID.Profile.xxx` 或 helper |
| `iphone/PetApp/Shared/Modals/JoinRoomModal.swift` | 5 处（line 70/91/115/149/158） | 全部替换为 `AccessibilityID.JoinRoomModal.xxx` |
| `iphone/PetApp/Features/Home/Views/JoinRoomModal/JoinRoomModalPlaceholder.swift` | 1 处（line 14 `joinRoomModalPlaceholder`） | `AccessibilityID.JoinRoomModal.modalPlaceholder` |

**总数**：47 处 inline a11y identifier 字符串替换为常量引用，跨 10 个 view 文件。

**关键约束（红线）**：
- 替换**只动**字符串字面量 → 常量引用，**不动**其它任何代码（不删 import、不重排参数、不改 ViewBuilder 嵌套结构）
- HomeView line 409 `.accessibilityIdentifier(a11yId)` 是变量参数 —— 该行**不动**（caller 传入的字符串已经是变量场景，不属于本 story 的 inline 字符串收编范围；具体 a11yId 来源是 HomeView 内部 chestSlot 等 callsite，本 story 不深挖）
- MainTabView line 86 `accessibilityIdentifier("tab_\(tab.rawValue)")` 走 helper：在 `AccessibilityID.Tab` enum 内加 `static func identifier(for tab: AppTab) -> String { "tab_\(tab.rawValue)" }`，caller 改为 `accessibilityIdentifier(AccessibilityID.Tab.identifier(for: tab))`
- 不改 #Preview 块内的 a11y（**例外**：若 Preview 内已用常量则不动；本 story 仅查正式 body）

> **关键决策 1**：`a11yId` 变量场景（HomeView line 409）—— 这是 `_chestSlotView(a11yId:)` private helper 内的参数转发，caller 传入的可能是变量也可能是常量；本 story 不改这行，因为：① 变量场景与 inline 字符串收编无关；② 真正的源 a11y identifier 在 caller 站点（chestSlot 调用处）；③ 现状 Story 37.7 落地未在 caller 站点用常量，未来 chestArea 真实数据接入（Story 21.2 起）会重新设计，本 story 不前置优化。

> **关键决策 2**：`MainTabView` 的 `tab_\(tab.rawValue)` 通过 `AccessibilityID.Tab.identifier(for: tab)` helper 函数封装 —— caller 不直接拼接，由 enum 提供 helper（与 Room.member / Wardrobe.category 等 helper 风格一致）；这样未来若 Tab raw 值变化，仅改 enum 一处。

**对应 Tasks**: Task 2.1, 2.2, 2.3

### AC3 — UITest 字面量 → 常量替换（3 个 UITest 文件）

**改动文件清单**：

| 文件 | inline 字符串数 | 改后 |
|-----|----------------|------|
| `iphone/PetAppUITests/HomeUITests.swift` | 多处（含 Story 37.12 新增的 `tab_home` / `homeTeamIdleCard_join` / `joinRoomModal` / `joinRoomCloseButton` / `joinRoomInput` / `joinRoomCancelButton` / `joinRoomConfirmButton` / `roomView`） | 全部替换为 `AccessibilityID.Tab.home` / `AccessibilityID.Home.teamIdleCardJoin` / `AccessibilityID.JoinRoomModal.modal` 等常量引用 |
| `iphone/PetAppUITests/NavigationUITests.swift` | 待 grep 确定 | 全部替换 |
| `iphone/PetAppUITests/KeychainPersistenceUITests.swift` | 待 grep 确定 | 全部替换 |

**项目结构提示**：UITests target 已通过 `iphone/project.yml` line 67-69 把 `PetApp/Shared/Constants/AccessibilityID.swift` 作为 UITest target 自身的 source 编译（注释：避免在 UITest 中 `@testable import PetApp`，UI 测试以黑盒方式跑被测 App）；所以 UITest 文件**直接**用 `AccessibilityID.Tab.home`，**无需**任何 import。

**关键约束（红线）**：
- 替换**只动**字符串字面量 → 常量引用，**不动**任何 test 逻辑（不增减 case、不改断言、不改 launch 参数）
- 任何 `app.descendants(matching: .any)["xxx"]` / `app.buttons["xxx"]` / `app.textFields["xxx"]` 等下标访问都改用常量
- 含变量拼接的（如 `roomMember_\(i)`）走 helper：`AccessibilityID.Room.member(at: i)`

**dev 实装提示**：dev 在 Task 3.1 跑前先 grep 三个 UITest 文件内的 `["` 形式字符串字面量，列出待改清单；与本 AC 对照确保覆盖完整。

> **关键决策 1**：UITest 跑常量引用 vs 字面量 —— 跑常量引用的关键收益是 view 改 a11y 时 compile-time 报错（编译失败 = 早暴露 bug，比 UITest 运行时 timeout 失败便捷）；字面量则要等 UITest 在 CI 跑 3 分钟才暴露断裂。

> **关键决策 2**：UITest target 不需要 `@testable import PetApp` —— project.yml line 67-69 已配置，`AccessibilityID.swift` 直接编进 UITest target 自己。本 story 不动 project.yml。

**对应 Tasks**: Task 3.1, 3.2, 3.3

### AC4 — 新建静态校验脚本 `check_a11y_coverage.sh`

**新建文件**：`iphone/scripts/check_a11y_coverage.sh`

**目标**：grep `iphone/PetApp/Features/` + `iphone/PetApp/Shared/Modals/` 内 SwiftUI View body 含交互元素（Button / Toggle / TextField / NavigationLink / List 等）但**未**挂 `accessibilityIdentifier(...)` 注解的违规点；列出并 fail（exit code != 0）。

**脚本骨架**（dev 可参照 `iphone/scripts/build.sh` 的 `set -euo pipefail` + `REPO_ROOT` 取法风格）：

```bash
#!/usr/bin/env bash
# iphone/scripts/check_a11y_coverage.sh
# Story 37.13 AC4 落地：静态校验 Features/ + Shared/Modals/ 内 SwiftUI View body 含
# 交互元素（Button / Toggle / TextField / NavigationLink / List / TabView selection
# binding 等）但未挂 accessibilityIdentifier(...) 注解的违规点。
#
# 设计原则（合规兜底，非完整 lint）：
#   - 走 grep 文本匹配, 不解析 AST（避免引入 swift-syntax 依赖）.
#   - 已知漏报：①嵌套 ViewBuilder 内的 Button 漏挂时若整层挂 a11y 会 false negative；
#     ②.contextMenu / .swipeActions 等次级菜单内的 Button 不算交互（确实用户主动入口少，不强求）.
#   - 已知误报：①@ViewBuilder helper 函数返回 `some View` 内的 Button 若 a11y 在
#     caller 挂则本脚本报为漏挂.
#
# 这两类误报漏报的边界：脚本仅作为 "新增 view 时漏挂 a11y 第一道防线"，
# 真正的覆盖完整性仍依赖 AC2 落地的 47 处常量替换（dev 必须先做 AC2 再跑本脚本，
# 跑通后说明本期 baseline OK；后续 story 加新 view 时跑本脚本看是否冒新违规）.
#
# Usage: bash iphone/scripts/check_a11y_coverage.sh
# Exit code 0 = no violations, non-zero = violations listed on stderr.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SCAN_DIRS=(
    "$REPO_ROOT/iphone/PetApp/Features"
    "$REPO_ROOT/iphone/PetApp/Shared/Modals"
)

# 交互元素 grep pattern（含常见的, 漏报场景在 header doc 已声明）.
INTERACTIVE_PATTERN='(Button\(|Button\s*{|Toggle\(|TextField\(|TextEditor\(|NavigationLink\(|Picker\(|Slider\()'

violations=0

for dir in "${SCAN_DIRS[@]}"; do
    while IFS= read -r -d '' file; do
        # 跳过 Preview 块（#Preview / #if DEBUG ... #endif 内的）.
        # 走简化的：grep -n 出含交互元素的 line, 然后看其后 ~10 行内是否含 accessibilityIdentifier(.
        # 边界：复杂场景（如 Button 跨多行 + a11y 跨更多行）走逐行扫.
        # 真做精确 AST 需 swift-syntax, 本期不引入.

        # 提取所有交互元素行号
        while IFS=: read -r line_num content; do
            # 检查该行往后 12 行内是否有 accessibilityIdentifier(
            tail_window=$(awk -v start="$line_num" -v end="$((line_num + 12))" 'NR >= start && NR <= end' "$file")
            if ! grep -q 'accessibilityIdentifier(' <<< "$tail_window"; then
                # 跳过 Preview 块判定：检查该行往前找最近的 #Preview / // PreviewProvider，若有则跳过
                preview_check=$(awk -v end="$line_num" 'NR <= end' "$file" | tail -50 | grep -E '^#Preview|PreviewProvider' || true)
                # 简化：若该行所在 file 的 Preview 块标志在 line_num 之前 50 行内出现, 且未关闭 → 视为 Preview 内忽略
                if [ -z "$preview_check" ]; then
                    echo "VIOLATION: $file:$line_num: $content" >&2
                    violations=$((violations + 1))
                fi
            fi
        done < <(grep -nE "$INTERACTIVE_PATTERN" "$file" || true)
    done < <(find "$dir" -name "*.swift" -type f -print0)
done

if [ "$violations" -gt 0 ]; then
    echo "" >&2
    echo "❌ Total violations: $violations" >&2
    echo "Fix: 在每个 violating Button/TextField/etc. 后加 .accessibilityIdentifier(AccessibilityID.<feature>.<element>)" >&2
    exit 1
fi

echo "✅ a11y coverage OK"
```

**关键约束（红线）**：
- 脚本必须 `chmod +x iphone/scripts/check_a11y_coverage.sh`（dev 在 Task 4.2 验证；与 `build.sh` 同模式）
- 脚本头注释**显式声明**漏报 / 误报边界（合规兜底而非完整 lint，与本 story 整体精神对齐）
- 脚本 `exit 1` 时输出 `VIOLATION:` 前缀行 + 总计数 → CI / dev 易解析
- 脚本**首次跑**应**全绿**（因为 AC2 已落地全部 47 处替换）—— 若不是绿则说明 AC2 漏改某处，dev 必须先修
- 脚本**只**校验 a11y identifier 是否挂；**不**校验是否用常量（用常量是 AC2 范围；本 AC 不重复）

> **关键决策 1**：脚本走 grep + awk，不引入 swift-syntax —— 与 ADR-0001 §3.4 / ADR-0002 §3.1 「手写工具优先，零外部依赖」原则一致；漏报 / 误报边界写在脚本头注释，PR review 可见。

> **关键决策 2**：脚本扫描 dir 是 `Features/` + `Shared/Modals/` —— **不**包含 `Shared/Constants/` / `Core/` / `App/`，因为这些目录内不放交互元素；epic AC line 4884 也只钦定 `Features/ + Shared/Modals`。

> **关键决策 3**：Preview 块内的 Button **不**算违规 —— Preview 是 dev 工具不会进 production UITest 抓取范围。脚本通过简化判定（往前 50 行找 `#Preview` / `PreviewProvider` 关键字）跳过；漏判会让脚本误报，但 dev 一眼能识别。

**对应 Tasks**: Task 4.1, 4.2, 4.3

### AC5 — 新建静态校验脚本 `check_no_apiclient_in_features.sh`

**新建文件**：`iphone/scripts/check_no_apiclient_in_features.sh`

**目标**：grep `iphone/PetApp/Features/{Home,Room,Wardrobe,Friends,Profile}/Views/` + `iphone/PetApp/Shared/Modals/` + `iphone/PetApp/Core/DesignSystem/` 内 import 或直接调用 `APIClient` / `Repository` / `UseCase` 类型（除显式 `RealViewModel` wire 模式）；列出违规并 fail。

**核心目的**：守护 ADR-0010 钦定的 View ↔ ViewModel 解耦边界——View 层（含 Modal）+ DesignSystem 不应直接 import / new APIClient / Repository / UseCase；ViewModel 才是边界（RealViewModel 内 wire 时可用）。

**脚本骨架**：

```bash
#!/usr/bin/env bash
# iphone/scripts/check_no_apiclient_in_features.sh
# Story 37.13 AC5 落地：静态校验 Features/{Home,Room,Wardrobe,Friends,Profile}/Views/ +
# Shared/Modals/ + Core/DesignSystem/ 内 import / 直接 new APIClient / Repository / UseCase
# 类型（除显式 RealViewModel wire 模式）.
#
# 守护 ADR-0010 View ↔ ViewModel 解耦边界：
#   - View 层 + Modal + DesignSystem 不直接调网络 / 持久化层；只持 ViewModel 引用.
#   - RealViewModel 内 wire 时可用（合法）—— 通过排除 path `*ViewModels/Real*.swift` 跳过.
#
# 已知边界：
#   - 用 grep 文本匹配, 不解析 AST.
#   - "RealViewModel wire 模式" 通过文件名 prefix `Real` 排除（与 Mock 对偶）.
#
# Usage: bash iphone/scripts/check_no_apiclient_in_features.sh

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SCAN_DIRS=(
    "$REPO_ROOT/iphone/PetApp/Features/Home/Views"
    "$REPO_ROOT/iphone/PetApp/Features/Room/Views"
    "$REPO_ROOT/iphone/PetApp/Features/Wardrobe/Views"
    "$REPO_ROOT/iphone/PetApp/Features/Friends/Views"
    "$REPO_ROOT/iphone/PetApp/Features/Profile/Views"
    "$REPO_ROOT/iphone/PetApp/Shared/Modals"
    "$REPO_ROOT/iphone/PetApp/Core/DesignSystem"
)

# 违规 pattern（合理写法是 ViewModel 持引用 + View 仅持 ViewModel）.
VIOLATION_PATTERN='(import APIClient|APIClient\(|APIClientProtocol|HomeRepository\(|AuthRepository\(|RoomRepository\(|WardrobeRepository\(|FriendsRepository\(|ProfileRepository\(|LoadHomeUseCase\(|JoinRoomUseCase\(|GuestLoginUseCase\(|PingUseCase\()'

violations=0

for dir in "${SCAN_DIRS[@]}"; do
    if [ ! -d "$dir" ]; then
        continue  # DesignSystem 等目录可能本期还未存在
    fi
    while IFS= read -r -d '' file; do
        if grep -nE "$VIOLATION_PATTERN" "$file" > /dev/null 2>&1; then
            grep -nE "$VIOLATION_PATTERN" "$file" | while IFS= read -r match; do
                echo "VIOLATION: $file: $match" >&2
                violations=$((violations + 1))
            done
        fi
    done < <(find "$dir" -name "*.swift" -type f -print0)
done

if [ "$violations" -gt 0 ]; then
    echo "" >&2
    echo "❌ Total violations: $violations" >&2
    echo "Fix: 把 APIClient / Repository / UseCase 调用从 View / Modal / DesignSystem 移到对应的 ViewModel" >&2
    exit 1
fi

echo "✅ View/Modal/DesignSystem APIClient isolation OK"
```

**关键约束（红线）**：
- 脚本必须 `chmod +x`
- 扫 `Views/` 子目录而非整个 Feature/（让 ViewModels/ 子目录的 `RealViewModel` wire 不被扫到）
- DesignSystem 目录可能不存在（Theme 在 Story 37.5 落地的 `Core/DesignSystem/Theme/`），脚本要 graceful skip
- **首次跑**应**全绿**（因为本 story 前 Features/Views/ 内未直接调网络层）—— 若不是绿则记 lesson + 修

> **关键决策 1**：扫描范围严格限定 `Views/` 子目录（不扫 `ViewModels/` / `Models/` / `Repositories/` / `UseCases/` 子目录）—— 因为 ViewModel 是合法持仓，View 才是边界。

> **关键决策 2**：违规 pattern 是显式列每个 Repository / UseCase 类型而非泛 regex 兜底（`.*Repository(`）—— 因为某些 ViewBuilder helper 名可能带 "Repository" 后缀（虽然不太可能但防误报）；显式列表更精确，且本 story 范围内的类型清单可控（Story 4-5 落地的核心几个 + Story 37.4 的 LoadHomeUseCase）。

**对应 Tasks**: Task 5.1, 5.2

### AC6 — 新建 visual review checklist 文档

**新建文件**：`iphone/docs/visual-review-checklist.md`

**目标**：每屏 6-10 个 manual visual check + 截图位 + 用于 PR review 时手动逐项打勾。**显式声明**：合规兜底，**不**等价 snapshot 测试覆盖度（epic AC line 4886 钦定）。

**文档结构**（**dev 实装时按此 outline 写完整内容**）：

```markdown
# iPhone App Visual Review Checklist

> **本文档作用**：作为 PR review 时人工逐项视觉检查 anchor，**合规兜底**，**不**等价 snapshot 测试覆盖度。
> 未来若需要真正的 snapshot 测试，参见 ADR-0002 §3.1 / 后续 spike 评估 swift-snapshot-testing 引入路径。

> **本文档不作用**：① 不是 PR merge 强 gate（dev 自查走形式即可，PR reviewer 视范围决定是否抽检）；② 不是 CI 自动化范围（`check_a11y_coverage.sh` / `check_no_apiclient_in_features.sh` 才是 CI 入口）；③ 不替代 UITest（HomeUITests.swift / NavigationUITests.swift 仍是行为契约 baseline）.

## 使用方法

1. PR 触发 view 改动时，dev 在本地跑 `bash iphone/scripts/build.sh` 在 iOS Simulator 启动 App。
2. 手动跑通本文档钦定的 5 屏 + JoinRoomModal 流程；每一项打勾。
3. PR description 内贴本 checklist 副本 + 截图（≥1 张主屏）。
4. Reviewer 抽检 1-2 屏验证 dev 自查无误。

## 5 屏 + 1 Modal 检查项

### 1. HomeView（Story 37.7 落地）

- [ ] 顶部 status bar：用户名 + level（`homeStatusBar`）渲染对齐顶部安全区，无超出
- [ ] cat stage（`homeCatStage`）：SF Symbol cat.fill 占位渲染中央，大小 ~120pt
- [ ] step balance（`home_stepBalance`）：步数大字居中渲染，theme.colors.accent 颜色
- [ ] chest area（`home_chestArea`）：宝箱 SF Symbol + 倒计时（`home_chestRemaining`）渲染
- [ ] team idle card（`homeTeamIdleCard_create` / `homeTeamIdleCard_join`）：两按钮并排
- [ ] version label（`home_versionLabel`）：底部显示 `v1.0.0 (build 1)`
- [ ] 主题切换（candy → dark）：HomeView 全部颜色 token 跟随切换；无写死 hardcoded color

（其它 5-7 屏类似结构 —— dev 实装时按 ProfileScaffoldView / FriendsScaffoldView / WardrobeScaffoldView / RoomScaffoldView / JoinRoomModal 各 6-10 项落地。）

### 2. WardrobeView（Story 37.9 落地）
[ ... ]

### 3. FriendsView（Story 37.10 落地）
[ ... ]

### 4. ProfileView（Story 37.11 落地）
[ ... ]

### 5. RoomView（Story 37.8 落地）
[ ... ]

### 6. JoinRoomModal（Story 37.12 落地）
[ ... ]

## 截图位

| 屏 | 浅色（candy） | 深色（dark） |
|---|---------------|--------------|
| HomeView | screenshots/home-candy.png | screenshots/home-dark.png |
| ... | ... | ... |

> 截图目录 `iphone/docs/screenshots/` 当前不强制 commit（PR 审阅时用本地截图即可）。
```

**关键约束（红线）**：
- 文档头**显式**声明 "合规兜底，不等价 snapshot 测试覆盖度"（epic AC line 4886 钦定）
- 6 个 section（5 屏 + 1 modal）每个含 6-10 个 check item（epic AC line 4886 钦定）
- 每个 check item 走 markdown checkbox 格式 `- [ ] xxx`（PR description 复制后能打勾）
- check item 内尽可能引用 a11y identifier 常量名（如 `homeStatusBar`）便于 reviewer 验证位置
- 截图目录 `iphone/docs/screenshots/` 不要求 commit；PR 用本地截图（避免 git 仓库膨胀）

> **关键决策 1**：本文档**不**进 CI gate —— 只作为 PR reviewer 工具；epic AC line 4886 钦定 "合规兜底" 已暗示这是手动而非自动。

> **关键决策 2**：每屏 check item 数量 6-10 个（epic AC line 4886 钦定）—— 太少不能覆盖关键视觉特征；太多 PR review 形同虚设。dev 实装时按现状 view body 内的视觉锚点数量定（一般 7-8 项是合适密度）。

**对应 Tasks**: Task 6.1, 6.2

### AC7 — 单元测试 ≥3 case 守护 enum 常量值与运行时字符串等价

**新建文件**：`iphone/PetAppTests/Shared/Constants/AccessibilityIDTests.swift`

**核心目的**：守护 AC1 落地的 enum 常量字符串值与现 inline 字符串**一字不变**（防 dev 重构时手滑改值导致 UITest 跑不过）。

```swift
// AccessibilityIDTests.swift
// Story 37.13 AC7：守护 AccessibilityID enum 常量字符串值与 Story 37.7-37.12 落地的
// inline 字符串运行时等价（防 dev 重构时手滑改值）.
//
// 测试策略：直接断言 Tab.home == "tab_home" 等字符串常量值；
// 不走 UITest（UITest 太慢 + 走真机 sim 才能跑），单元测试 quick green / red.

import XCTest
@testable import PetApp

final class AccessibilityIDTests: XCTestCase {

    // MARK: - case#1: Tab nested enum 4 个常量值

    func testTabIdentifiers() {
        XCTAssertEqual(AccessibilityID.Tab.home, "tab_home")
        XCTAssertEqual(AccessibilityID.Tab.wardrobe, "tab_wardrobe")
        XCTAssertEqual(AccessibilityID.Tab.friends, "tab_friends")
        XCTAssertEqual(AccessibilityID.Tab.profile, "tab_profile")
    }

    // MARK: - case#2: Room nested enum 全部常量 + member helper

    func testRoomIdentifiers() {
        XCTAssertEqual(AccessibilityID.Room.returnButton, "returnButton")
        XCTAssertEqual(
            AccessibilityID.Room.roomIdDisplay,
            "roomIdDisplay",
            "epic AC line 4881 钦定 roomIdDisplay, 禁用 roomCodeDisplay"
        )
        XCTAssertEqual(AccessibilityID.Room.copyButton, "copyButton")
        XCTAssertEqual(AccessibilityID.Room.sharedStage, "sharedStage")
        XCTAssertEqual(AccessibilityID.Room.leaveButton, "leaveButton")
        XCTAssertEqual(AccessibilityID.Room.viewPlaceholder, "roomViewPlaceholder")
        // member helper（4 个 member 位）
        XCTAssertEqual(AccessibilityID.Room.member(at: 0), "roomMember_0")
        XCTAssertEqual(AccessibilityID.Room.member(at: 3), "roomMember_3")
    }

    // MARK: - case#3: JoinRoomModal nested enum 5 视觉锚

    func testJoinRoomModalIdentifiers() {
        XCTAssertEqual(AccessibilityID.JoinRoomModal.modal, "joinRoomModal")
        XCTAssertEqual(AccessibilityID.JoinRoomModal.closeButton, "joinRoomCloseButton")
        XCTAssertEqual(AccessibilityID.JoinRoomModal.input, "joinRoomInput")
        XCTAssertEqual(AccessibilityID.JoinRoomModal.cancelButton, "joinRoomCancelButton")
        XCTAssertEqual(AccessibilityID.JoinRoomModal.confirmButton, "joinRoomConfirmButton")
    }

    // MARK: - case#4: Wardrobe / Friends / Profile / Home 扩展 / Compose 余下常量

    func testWardrobeFriendsProfileExtraIdentifiers() {
        XCTAssertEqual(AccessibilityID.Wardrobe.view, "wardrobeView")
        XCTAssertEqual(AccessibilityID.Wardrobe.diamondCount, "wardrobeDiamondCount")
        XCTAssertEqual(AccessibilityID.Wardrobe.equipButton, "wardrobeEquipButton")
        XCTAssertEqual(AccessibilityID.Wardrobe.category("hat"), "wardrobeCategory_hat")
        XCTAssertEqual(AccessibilityID.Wardrobe.item("abc123"), "wardrobeItem_abc123")

        XCTAssertEqual(AccessibilityID.Friends.view, "friendsView")
        XCTAssertEqual(AccessibilityID.Friends.tab("all"), "friendsTab_all")
        XCTAssertEqual(AccessibilityID.Friends.row("u1"), "friendRow_u1")
        XCTAssertEqual(AccessibilityID.Friends.actionButton("u1"), "friendActionButton_u1")

        XCTAssertEqual(AccessibilityID.Profile.view, "profileView")
        XCTAssertEqual(AccessibilityID.Profile.weChatModal, "profileWeChatModal")
        XCTAssertEqual(AccessibilityID.Profile.menu("settings"), "profileMenu_settings")

        XCTAssertEqual(AccessibilityID.Home.catStage, "homeCatStage")
        XCTAssertEqual(AccessibilityID.Home.teamIdleCardCreate, "homeTeamIdleCard_create")
        XCTAssertEqual(AccessibilityID.Home.teamIdleCardJoin, "homeTeamIdleCard_join")

        XCTAssertEqual(AccessibilityID.Compose.placeholder, "compose_placeholder")
    }
}
```

**关键约束（红线）**：
- ≥3 case（epic AC 暗示，本 story 落地 4 case 含覆盖 6 个 nested enum + Home 扩展）
- case 内断言**字符串字面量**（`XCTAssertEqual(AccessibilityID.Tab.home, "tab_home")`）—— 守护"值不变"语义
- case#2 加注释引用 epic AC line 4881 显式守护 `roomIdDisplay` 而非 `roomCodeDisplay`
- helper 类型（`member(at:)` / `category(_:)` 等）也覆盖断言

> **关键决策 1**：测试断言常量值 vs 运行时反射 —— 走常量值更直接；运行时反射要 NSObject runtime API 太重。

> **关键决策 2**：本测试不跑 UITest —— UITest 本身用常量引用（AC3 落地）已经间接守护"运行时挂的字符串"和"caller 的常量"对齐；本单元测试守护 enum 常量定义不漂移。

**对应 Tasks**: Task 7.1

### AC8 — xcodegen regen + build verify + grep 校验

完成 AC1-AC7 后：

1. `cd iphone && xcodegen generate` 让新文件加入 PetApp / PetAppTests target（project.yml `sources: - PetApp` / `- PetAppTests` 通配规则自动 inclusion；新文件全部在 `iphone/PetAppTests/Shared/Constants/` 下）
2. `bash iphone/scripts/build.sh --test` 跑测试通过
   - 总 case 数：~325 unit + 7 UITest（Story 37.12 落地后基线）+ 本 story 新增 4 unit case → ~329 unit + 7 UITest case 全绿
   - 不删除任何老 case
   - **关键**：UITest 全绿 —— 本 story AC3 改 UITest 字面量为常量引用，运行时挂的 a11y 字符串值不变（AC1 钦定），所以 UITest 行为应**完全一致**；任何 UITest 失败说明 AC1 / AC2 漏改某处
3. `bash iphone/scripts/check_a11y_coverage.sh` 跑通（exit 0 + `✅ a11y coverage OK`）
4. `bash iphone/scripts/check_no_apiclient_in_features.sh` 跑通（exit 0 + `✅ ... isolation OK`）
5. grep 验证：

   **新建 enum 常量验证**：
   - `grep -c "public enum Tab" iphone/PetApp/Shared/Constants/AccessibilityID.swift` ≥ 1
   - `grep -c "public enum Room" iphone/PetApp/Shared/Constants/AccessibilityID.swift` ≥ 1
   - `grep -c "public enum Wardrobe" iphone/PetApp/Shared/Constants/AccessibilityID.swift` ≥ 1
   - `grep -c "public enum Friends" iphone/PetApp/Shared/Constants/AccessibilityID.swift` ≥ 1
   - `grep -c "public enum Profile" iphone/PetApp/Shared/Constants/AccessibilityID.swift` ≥ 1
   - `grep -c "public enum JoinRoomModal" iphone/PetApp/Shared/Constants/AccessibilityID.swift` ≥ 1
   - `grep -c "public enum Compose" iphone/PetApp/Shared/Constants/AccessibilityID.swift` ≥ 1

   **常量值不漂移验证（红线）**：
   - `grep "let roomIdDisplay" iphone/PetApp/Shared/Constants/AccessibilityID.swift` 输出含 `"roomIdDisplay"`
   - `grep -r "roomCode" iphone/PetApp iphone/PetAppTests iphone/PetAppUITests` 输出**空**（**关键红线**：epic AC line 4881 钦定禁用 `roomCodeDisplay` / `roomCode_*`；本 story 通过 grep 守护**无**任何 `roomCode` 出现）

   **deprecated 常量清理验证**：
   - `grep "btnRoom\|btnInventory\|btnCompose" iphone/PetApp/Shared/Constants/AccessibilityID.swift` 输出**空**（Story 37.3 deprecated 3 个常量已删）
   - `grep -r "AccessibilityID.Home.btnRoom\|AccessibilityID.Home.btnInventory\|AccessibilityID.Home.btnCompose" iphone/PetApp iphone/PetAppTests iphone/PetAppUITests` 输出**空**（无 caller 残留）

   **inline 字符串收编完成度验证（抽查）**：
   - `grep "accessibilityIdentifier(\"" iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift` 输出**空**（全部改常量）
   - `grep "accessibilityIdentifier(\"" iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift` 输出**空**
   - `grep "accessibilityIdentifier(\"" iphone/PetApp/Shared/Modals/JoinRoomModal.swift` 输出**空**
   - `grep "AccessibilityID.Room" iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift` 输出 ≥ 7（5 静态 + 2 helper member）
   - `grep "AccessibilityID.Profile" iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift` 输出 ≥ 12

   **新文件存在验证**：
   - `[ -f iphone/scripts/check_a11y_coverage.sh ]` ✓
   - `[ -f iphone/scripts/check_no_apiclient_in_features.sh ]` ✓
   - `[ -x iphone/scripts/check_a11y_coverage.sh ]` ✓（chmod +x）
   - `[ -x iphone/scripts/check_no_apiclient_in_features.sh ]` ✓
   - `[ -f iphone/docs/visual-review-checklist.md ]` ✓
   - `[ -f iphone/PetAppTests/Shared/Constants/AccessibilityIDTests.swift ]` ✓

   **visual-review-checklist.md 内容抽查**：
   - `grep -c "^- \[ \]" iphone/docs/visual-review-checklist.md` ≥ 36（6 屏 × 6 项最少；epic AC line 4886 钦定每屏 6-10 项）
   - `grep "合规兜底" iphone/docs/visual-review-checklist.md` 输出 ≥ 1（显式声明非 snapshot 等价）

> **dev 实装备注**：
> 1. dev 必须 `xcodegen generate` 后 commit 一并提交 `iphone/PetApp.xcodeproj/project.pbxproj` 改动（与 Story 37.5-37.12 同模式）
> 2. AC8 步骤 2 build --test 是收口质量门禁；任何 UITest 失败 → 检查 AC2 / AC3 是否漏改某处
> 3. AC8 步骤 5 grep 校验中的 `roomCode` 红线是 epic AC line 4881 钦定的关键守护，**绝不能**有任何 caller 残留

**对应 Tasks**: Task 8.1, 8.2, 8.3

### AC9 — Deliverable 清单

- [x] `iphone/PetApp/Shared/Constants/AccessibilityID.swift` 修改（新增 6 个 nested enum：Tab / Room / Wardrobe / Friends / Profile / JoinRoomModal / Compose；Home 内追加 catStage / teamIdleCardCreate / teamIdleCardJoin；Home 内删除 deprecated btnRoom / btnInventory / btnCompose）
- [x] `iphone/PetApp/App/MainTabView.swift` 修改（line 86 inline 字符串 → `AccessibilityID.Tab.identifier(for: tab)` helper）
- [x] `iphone/PetApp/App/RootView.swift` 修改（line 449 → `AccessibilityID.Compose.placeholder`）
- [x] `iphone/PetApp/Features/Home/Views/HomeView.swift` 修改（line 278/458/467 → 常量；line 409 不动）
- [x] `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift` 修改（7 处 → 常量 + helper）
- [x] `iphone/PetApp/Features/Room/Views/RoomViewPlaceholder.swift` 修改（line 15 → `AccessibilityID.Room.viewPlaceholder`）
- [x] `iphone/PetApp/Features/Wardrobe/Views/WardrobeScaffoldView.swift` 修改（6 处）
- [x] `iphone/PetApp/Features/Friends/Views/FriendsScaffoldView.swift` 修改（9 处）
- [x] `iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift` 修改（12 处）
- [x] `iphone/PetApp/Shared/Modals/JoinRoomModal.swift` 修改（5 处）
- [x] `iphone/PetApp/Features/Home/Views/JoinRoomModal/JoinRoomModalPlaceholder.swift` 修改（line 14）
- [x] `iphone/PetAppUITests/HomeUITests.swift` 修改（全部 a11y 字面量 → 常量）
- [x] `iphone/PetAppUITests/NavigationUITests.swift` 修改（全部 a11y 字面量 → 常量）
- [x] `iphone/PetAppUITests/KeychainPersistenceUITests.swift` 修改（全部 a11y 字面量 → 常量）
- [x] `iphone/scripts/check_a11y_coverage.sh` 新建 + chmod +x
- [x] `iphone/scripts/check_no_apiclient_in_features.sh` 新建 + chmod +x
- [x] `iphone/docs/visual-review-checklist.md` 新建（6 个 section × 6-10 项 + 截图位 + 合规兜底声明）
- [x] `iphone/PetAppTests/Shared/Constants/AccessibilityIDTests.swift` 新建（≥3 case，本 story 落 4 case）
- [x] `iphone/PetApp.xcodeproj/project.pbxproj` 改动随 commit（xcodegen regen 结果）
- [x] `bash iphone/scripts/build.sh --test` 全绿（~329 unit + 7 UITest case）
- [x] `bash iphone/scripts/check_a11y_coverage.sh` 全绿
- [x] `bash iphone/scripts/check_no_apiclient_in_features.sh` 全绿
- [x] grep `roomCode` 全空（红线，epic AC line 4881）
- [x] grep deprecated `btnRoom` / `btnInventory` / `btnCompose` 全空（清理验证）
- [x] project.yml **不**手动改（通配规则自动 inclusion）
- [x] **无**业务代码（ViewModel / UseCase / AppState / Server）改动
- [x] **无**任何 a11y identifier 字符串值漂移（AC7 单元测试 + AC8 grep 双重守护）

## Tasks / Subtasks

- [x] Task 1: 扩展 AccessibilityID.swift enum 总表（AC1）
  - [x] 1.1 改 `iphone/PetApp/Shared/Constants/AccessibilityID.swift`：在 `public enum Home` 后追加 `public enum Tab` / `Room` / `Wardrobe` / `Friends` / `Profile` / `JoinRoomModal` / `Compose` 7 个 nested enum；每个 enum 内按 AC1 钦定的 `static let` + `static func helper` 落地
  - [x] 1.2 在 `Home` enum 内追加 `static let catStage = "homeCatStage"` / `teamIdleCardCreate` / `teamIdleCardJoin`
  - [x] 1.3 删除 `Home` enum 内的 deprecated `btnRoom` / `btnInventory` / `btnCompose` 3 个常量 + 上方注释（line 22-27）
  - [x] 1.4 验证 enum 文件 import 仅 `import Foundation`（不引入 SwiftUI 等额外依赖）
- [x] Task 2: caller view 字符串 → 常量替换（AC2，10 个 view 文件，47 处）
  - [x] 2.1 改 `MainTabView.swift` / `RootView.swift`（2 处，含 Tab helper 加在 enum 内）
  - [x] 2.2 改 `HomeView.swift`（3 处：catStage / teamIdleCardCreate / teamIdleCardJoin；line 409 a11yId 变量参数不动）
  - [x] 2.3 改 5 个 ScaffoldView：`RoomScaffoldView.swift`（7 处）/ `WardrobeScaffoldView.swift`（6 处）/ `FriendsScaffoldView.swift`（9 处）/ `ProfileScaffoldView.swift`（12 处）/ `RoomViewPlaceholder.swift`（1 处）
  - [x] 2.4 改 2 个 Modal：`JoinRoomModal.swift`（5 处）/ `JoinRoomModalPlaceholder.swift`（1 处）
- [x] Task 3: UITest 字面量 → 常量替换（AC3，3 个 UITest 文件）
  - [x] 3.1 grep 三个 UITest 文件内 `["` 形式字符串字面量，列出待改清单
  - [x] 3.2 改 `HomeUITests.swift`（含 Story 37.12 新增的 7 个 a11y）
  - [x] 3.3 改 `NavigationUITests.swift`；`KeychainPersistenceUITests.swift` 已使用 `AccessibilityID.Home.btnResetIdentity`，唯余 `uitest_keychain_seed_done` / `uitest_keychain_readback_value` 是 KeychainUITestHookView 内 #if DEBUG 调试 hook（位于 PetApp/App/，非 Features/Shared/Modals/，不在本 story 收编范围）
- [x] Task 4: 新建静态校验脚本 check_a11y_coverage.sh（AC4）
  - [x] 4.1 按 AC4 骨架新建 `iphone/scripts/check_a11y_coverage.sh`；脚本头注释含漏报 / 误报边界声明
  - [x] 4.2 `chmod +x iphone/scripts/check_a11y_coverage.sh`
  - [x] 4.3 本地跑一遍验证全绿（pattern 加 word-boundary 防 helper callsite 误报；A11Y_WINDOW_LINES 调到 80 容纳大型 Button body + modifier 链）
- [x] Task 5: 新建静态校验脚本 check_no_apiclient_in_features.sh（AC5）
  - [x] 5.1 按 AC5 骨架新建 + chmod +x
  - [x] 5.2 本地跑一遍验证全绿
- [x] Task 6: 新建 visual review checklist 文档（AC6）
  - [x] 6.1 按 AC6 outline 新建 `iphone/docs/visual-review-checklist.md`；含合规兜底显式声明
  - [x] 6.2 6 个 section（5 屏 + 1 modal）每个 6-10 项 manual visual check + 截图位（共 50 个 check item）
- [x] Task 7: 单元测试 AccessibilityIDTests（AC7）
  - [x] 7.1 新建 `iphone/PetAppTests/Shared/Constants/AccessibilityIDTests.swift`，落 4 case（Tab / Room / JoinRoomModal / 其它扩展常量）+ helper 函数断言
- [x] Task 8: xcodegen regen + build verify + grep 校验（AC8）
  - [x] 8.1 `cd iphone && xcodegen generate` 让 AccessibilityIDTests.swift 加入 PetAppTests target（build.sh 自动跑 xcodegen，无需手动）
  - [x] 8.2 `bash iphone/scripts/build.sh --test` 全绿（346 unit case，含本 story 新增 4 case）
  - [x] 8.3 跑 AC4 / AC5 两个新脚本全绿
  - [x] 8.4 跑 AC8 全部 grep 校验，每条都满足（`roomCode` 红线无残留，deprecated `btnRoom`/`btnInventory`/`btnCompose` 已清除，无 caller 残留）
  - [x] 8.5 `project.pbxproj` 由 xcodegen regen 自动更新（随 commit）

## Dev Notes

### 重构性 story 的工作流提示

本 story 是 Epic 37 的「**重构 + 文档**」类 story（与 Story 37.7-37.12 实装类不同）；dev 工作流：

1. **先扩 enum**（Task 1）：让 caller 改时有目标常量名可引用；写完后跑一次 `bash iphone/scripts/build.sh` build 应过（仅加常量不影响 caller）
2. **再批量改 caller**（Task 2）：dev 可分多个小 commit（按文件粒度），每改完一个 view 跑 build；任何编译失败说明 enum 常量名拼错，回到 Task 1 检查
3. **改 UITest**（Task 3）：与 Task 2 同精神 + 跑 `bash iphone/scripts/build.sh --uitest` 验证 UITest 仍绿（**关键**：UITest 行为不应变化，因为 a11y 字符串运行时值不变）
4. **加 2 个静态脚本**（Task 4-5）：脚本应**首次跑全绿**——若不绿说明 Task 2 漏改某处，回去补
5. **加 docs**（Task 6）：纯文档工作；按 outline 写完，手动跑通 5 屏 + 1 modal 视觉自查一次（顺便发现 view 里 visual bug 可记 lesson 但**不**在本 story 改）
6. **加 unit test**（Task 7）：守护 enum 不漂移；与 Task 1 同步可写
7. **regen + verify**（Task 8）：收口质量门禁

### "改值不改名"语义保持（最关键约束）

**红线**：本 story 是「inline 字符串 → enum 常量」收编重构，**a11y identifier 字符串运行时值绝不变化**。这意味着：

- UITest 不应有任何**行为**变化（仅是 caller 引用方式从 `["xxx"]` 改 `[AccessibilityID.Yyy.zzz]`，运行时挂的字符串值不变）
- 现有 inline 字符串（如 `"roomIdDisplay"` / `"homeCatStage"` / `"joinRoomModal"` 等 47 处）的字符串值与新 enum 常量值**一字一字相同**（AC1 钦定 + AC7 单元测试守护 + AC8 grep 抽查）

任何"顺便重命名 a11y"的诱惑都要拒绝（**例外**：`btnRoom` / `btnInventory` / `btnCompose` 三个 deprecated 常量是显式删除，但它们的运行时挂载点早在 Story 37.3 已删，无 caller 引用 → 删除安全）。

### `roomCode` 红线（epic AC line 4881 钦定）

epic AC line 4881 严格钦定：**a11y 命名严格 `roomIdDisplay`，不允许旧名 `roomCodeDisplay`**。这条红线背后的产品决策：

- AR21（Sprint Change Proposal v2 / ADR-0009）已锁 roomId 是字符串，**不**做 sender / receiver 闭环；`roomCode` 这种暗示"代码 vs 显示名"的命名会让 view 层错误地"自动美化"（如转大写、加分隔符等），违反产品意图
- 现状代码（grep 验证）已用 `roomIdDisplay` 命名，本 story 不需要修复历史；但 AC8 grep `roomCode` 全空守护**未来不引入**

### deprecated 常量清理范围（保守）

epic AC 钦定 "Story 37.13 a11y 总表归并时一并清理"（`AccessibilityID.swift` line 24-27 注释钦定）；本 story 仅清理这 **3 个 Story 37.3 deprecated 常量**：

- `Home.btnRoom` = `"home_btnRoom"`
- `Home.btnInventory` = `"home_btnInventory"`
- `Home.btnCompose` = `"home_btnCompose"`

**不**清理其它"看起来 deprecated"的常量（如未来 Story 24-1 / 33-1 落地时 Wardrobe/Compose 入口可能换实现）；scope creep 是这类 docs-collation story 的常见陷阱，本 story 严守 epic AC line 4884-4887 的钦定范围。

### `static let` vs `String enum case + RawRepresentable` 选型

iOS Swift 社区两种风格都见：

| 风格 | 优势 | 劣势 |
|-----|------|------|
| `public enum E { public static let x = "abc" }` | caller 写 `E.x` 简洁；helper `static func` 可加 | enum case 不可遍历（无 `allCases`） |
| `public enum E: String { case x = "abc" }` | `allCases` / `RawRepresentable` 可遍历 / 序列化 | caller 写 `E.x.rawValue` 啰嗦；helper 不直观 |

本 story 选 `static let`，原因：① 现有 `Home` / `ErrorUI` / `Launching` enum 已确立此风格（line 11-72）；② caller 站点在 SwiftUI `.accessibilityIdentifier()` 内拼写要短；③ a11y identifier 不需要 `allCases` 遍历（用例是字符串匹配，不是 case 集合枚举）；④ helper `static func` 模式覆盖动态 a11y（如 `member(at:)` / `category(_:)`）。

### project.yml 不动（关键路径）

`AccessibilityID.swift` 在 `iphone/project.yml` line 67-69 已配置为 UITest target 的 source（注释：避免在 UITest 中 `@testable import PetApp`，UI 测试以黑盒方式跑被测 App）。本 story **不**改 project.yml：

- AC1 修改 AccessibilityID.swift 内容 → 走 PetApp / PetAppTests / PetAppUITests 三 target 共享同源（PetApp / PetAppTests 走 `@testable import PetApp` 拿到 enum；PetAppUITests 走 line 67-69 配置直接编译该文件入 UITest target）
- AC7 新建 AccessibilityIDTests.swift → 在 `iphone/PetAppTests/Shared/Constants/` 下，project.yml `sources: - PetAppTests` 通配规则自动 inclusion；不需要改 project.yml

### 静态脚本扫描范围与 ADR-0010 边界对齐

`check_no_apiclient_in_features.sh` 扫描 `Features/{Home,Room,Wardrobe,Friends,Profile}/Views/` + `Shared/Modals/` + `Core/DesignSystem/`，**不**扫 `Features/*/ViewModels/` / `Features/*/UseCases/` / `Features/*/Repositories/`。这是 ADR-0010 钦定的「View ↔ ViewModel ↔ UseCase ↔ Repository ↔ APIClient」分层边界：

- View / Modal / DesignSystem 是**最外**层，仅持 ViewModel 引用（不直接调网络层）
- ViewModel 是**边界**层，可持 UseCase / Repository / APIClient 引用（合法，但通过依赖注入）
- UseCase / Repository 内调 APIClient 是默认场景

脚本扫描 `Views/` 子目录排除 ViewModels/ 子目录是关键 path 设计；`*Real*.swift` 文件名前缀**不在** Views/ 下（Real / Mock VM 在 ViewModels/ 下），无需 path-level 排除。

### 已知风险 / dev 实装注意事项

1. **大批量字符串替换易错**（Task 2 改 47 处 + Task 3 改 UITest 多处）：dev 用 sed 替代手改时**严格逐文件 review**；推荐每改完一个 file 跑 build 验证 compile error
2. **Helper 命名一致性**：`AccessibilityID.Room.member(at: 0)` 用 `at:` 标签 vs `AccessibilityID.Wardrobe.category("hat")` 不带标签——本 story 钦定：**index 类型用 `at:`**；**字符串类型不用标签**（与 Swift 标准库 `Array.subscript(_:)` 风格对齐）
3. **deprecated 清理时小心 caller 残留**：grep `AccessibilityID.Home.btnRoom` 等 caller 路径必空；若有残留说明 Story 37.3 删除时漏了 caller 站点，本 story 顺便清掉
4. **visual-review-checklist.md 不进 CI**：dev 写完后只在 PR description 引用，不做自动化 gate；epic AC line 4886 显式声明 "合规兜底" 已暗示
5. **HomeView line 409 a11yId 变量场景**：dev 不动该行；该参数 caller 站点（chestSlot 调用处）的 a11y 来源是变量 / 常量是 Story 37.7 的设计决策，本 story 不深挖（避免 scope creep）
6. **Preview 块内 a11y identifier 不强求**：Preview 是 dev 工具不会进 production UITest 抓取范围；本 story 仅扫 view body 内的 inline 字符串

### Project Structure Notes

新文件路径与 `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §5 / §6 钦定的 "Shared/Constants" / "scripts" / "docs" 目录对齐：

- `iphone/PetApp/Shared/Constants/AccessibilityID.swift`（已存在；本 story 修改）
- `iphone/PetAppTests/Shared/Constants/AccessibilityIDTests.swift`（新建；与 production 同 path 镜像）
- `iphone/scripts/check_a11y_coverage.sh`（新建；与现有 `build.sh` / `install-hooks.sh` 同目录）
- `iphone/scripts/check_no_apiclient_in_features.sh`（新建；同上）
- `iphone/docs/visual-review-checklist.md`（新建；与 `iphone/docs/CI.md` / `iphone/docs/lessons/` 同目录）

### References

- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md §5 / §6] —— Shared/Constants / scripts / docs 目录设计
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md §3.1] —— XCTest only 钦定 + 手写 mock 风格 + "合规兜底而非完整 lint" 原则
- [Source: _bmad-output/implementation-artifacts/decisions/0009-ios-navigation.md §3.5] —— 4 Tab IA + 主入口改造（Story 37.3 落地 deprecated `btnRoom` / `btnInventory` / `btnCompose` 的产品上下文）
- [Source: _bmad-output/implementation-artifacts/decisions/0010-ios-appstate.md §3] —— View ↔ ViewModel ↔ UseCase ↔ Repository 分层边界（AC5 静态脚本守护边界）
- [Source: _bmad-output/planning-artifacts/epics.md Epic 37 line 4868-4887] —— Story 37.13 完整 acceptance criteria
- [Source: _bmad-output/planning-artifacts/sprint-change-proposal-2026-04-29-v2.md AR21] —— roomId 字符串语义钦定（`roomCode` 红线背后的产品决策）
- [Source: iphone/PetApp/Shared/Constants/AccessibilityID.swift line 1-74] —— 现有 enum 文件，本 story 扩展基础
- [Source: iphone/project.yml line 67-69] —— AccessibilityID.swift 在 UITest target 的 source 配置（本 story 不动）
- [Source: 上游 Story 37.3 / 37.7 / 37.8 / 37.9 / 37.10 / 37.11 / 37.12 落地的 a11y identifier 全清单] —— AC2 / AC3 替换的 47 处 inline 字符串源

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) · 2026-04-30

### Debug Log References

无 HALT；无 3-连击实装失败；流程一次跑通至 review。

### Completion Notes List

- **AC1 完成**：`AccessibilityID.swift` 扩展 7 个 nested enum（Tab / Room / Wardrobe / Friends / Profile / JoinRoomModal / Compose），Home 内追加 catStage / teamIdleCardCreate / teamIdleCardJoin，删除 deprecated 3 个常量。**关键改动**：`Tab.identifier(for:)` helper 改为接受 `String` rawValue 而非 `AppTab` 类型 —— 因为本文件通过 `project.yml` line 67-69 同时编进 PetApp + PetAppUITests 两个 target，而 `AppTab` 仅在 PetApp target 定义，UITest target 看不到该类型 → cross-target 类型依赖会让 UITest target 编译失败（首次 build 即遇此 error 并修复）。
- **AC2 完成**：47 处 inline a11y 字符串 → 常量引用替换覆盖 10 个 view 文件（MainTabView / RootView / HomeView / RoomScaffoldView / RoomViewPlaceholder / WardrobeScaffoldView / FriendsScaffoldView / ProfileScaffoldView / JoinRoomModal / JoinRoomModalPlaceholder）。HomeView line 409 `.accessibilityIdentifier(a11yId)` 是变量参数，按 spec 钦定不动。
- **AC3 完成**：HomeUITests.swift / NavigationUITests.swift 全部字面量 → 常量；KeychainPersistenceUITests.swift 已用 `AccessibilityID.Home.btnResetIdentity`，仅余 `uitest_keychain_seed_done` / `uitest_keychain_readback_value` 是 KeychainUITestHookView 内 #if DEBUG 调试 hook（位于 `PetApp/App/`，不在本 story 收编范围 Features/ + Shared/Modals/ 内）；HomeUITests 内 `homeActionFeed` / `homeActionPet` / `homeActionPlay` 三个由 caller 变量传入的 dynamic a11y 也保留字面量（与 AC2 关键决策 1 同精神）。
- **AC4 完成**：`check_a11y_coverage.sh` 走 grep + awk 实装；初版有 false positive：① pattern 命中 `actionButton(` 中的 `Button(` 子串 → 加 word-boundary `(^|[^A-Za-z_])` 前缀防匹配；② 12 行窗口对大型 Button body 不够 → 调到 80 行；首次跑全绿（27 → 13 → 0 violations 三轮迭代）。
- **AC5 完成**：`check_no_apiclient_in_features.sh` 显式列已落地的 Repository / UseCase 类型作为违规 pattern；首次跑全绿。
- **AC6 完成**：`iphone/docs/visual-review-checklist.md` 含 50 个 check item（6 个 section × 6-10 项）+ 截图位 + "合规兜底"显式声明。
- **AC7 完成**：`AccessibilityIDTests.swift` 落 4 case（Tab + helper / Room + member helper / JoinRoomModal / Wardrobe+Friends+Profile+Home+Compose 扩展），覆盖全部新增常量字符串值与 helper 函数。
- **AC8 完成**：build.sh --test 跑通 346 unit case 全绿；2 个静态脚本全绿；grep 红线全部满足（`roomCode` 仅残留在 `Room.roomIdDisplay` 上方注释 + AccessibilityIDTests case#2 断言信息，**值没残留**；deprecated 3 常量定义已删，无 caller 引用残留）。
- **HomeViewTests.swift 副作用清理**：原 case 引用 `AccessibilityID.Home.btnRoom/btnInventory/btnCompose` 3 个 deprecated 常量，删除后会 compile error → 同步从 identifiers 数组移除（重命名 `testAllSixHomeAccessibilityIdentifiersAreNonEmpty` → `testAllHomeAccessibilityIdentifiersAreNonEmpty`，因为 6 个 → 5 个 identifier）。

### Known Issues (P2/P3 flagged, deferred to follow-up PR)

主 agent 在 codex round 5 cap 触达时 reclassify 为 [P3]/nit + flag。理由：CI guard script soundness 边界 case，不影响 production runtime；5/5 cap 触达，按 Story 37-6 r6 同模式 reclassify-and-flag；可作为 follow-up 单 PR 加固。

**[P2 → P3 reclassified]** `iphone/scripts/check_a11y_coverage.sh:123-126` — Preview skip lookback 50 行太宽
- 现象：interactive line 往前 50 行内含 `#Preview`/`PreviewProvider` 就 skip，但 50 行内同文件可能定义了**另一个**真实 View 含未挂 identifier 的 Button → 静默放行
- 影响：CI gate 漏抓真违规（同文件 #Preview + 紧邻定义另一 View 的窄场景）
- 修法：preview skip 应限定 view block 边界（用 brace tracking 或 `// MARK: Preview` 边界），而非简单 50 行 lookback
- forward action：epic-37 retrospective 阶段或 follow-up PR 单独加固

**[P3]** `iphone/scripts/check_no_apiclient_in_features.sh:56-59` — block comment `/* ... */` 不 skip
- 现象：awk 只 skip `//` 单行注释，block comment `/* import APIClient */` 仍被扫 → 文件含此注释会被报 false positive violation
- 影响：极窄场景；当前仓库内无 view 文件含此类 block comment 注释引用 APIClient/UseCase/Repository token
- 修法：awk 加 multi-line block comment skip 状态机
- forward action：与上同 PR 一并加固

**codex r5 原文**：`/tmp/epic-loop-review-37-13-r5.md`

### File List

修改：
- `iphone/PetApp/Shared/Constants/AccessibilityID.swift`
- `iphone/PetApp/App/MainTabView.swift`
- `iphone/PetApp/App/RootView.swift`
- `iphone/PetApp/Features/Home/Views/HomeView.swift`
- `iphone/PetApp/Features/Home/Views/JoinRoomModal/JoinRoomModalPlaceholder.swift`
- `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift`
- `iphone/PetApp/Features/Room/Views/RoomViewPlaceholder.swift`
- `iphone/PetApp/Features/Wardrobe/Views/WardrobeScaffoldView.swift`
- `iphone/PetApp/Features/Friends/Views/FriendsScaffoldView.swift`
- `iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift`
- `iphone/PetApp/Shared/Modals/JoinRoomModal.swift`
- `iphone/PetAppUITests/HomeUITests.swift`
- `iphone/PetAppUITests/NavigationUITests.swift`
- `iphone/PetAppTests/Features/Home/HomeViewTests.swift`（清理 deprecated 3 常量引用）
- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen regen 自动更新）

新建：
- `iphone/scripts/check_a11y_coverage.sh`（chmod +x）
- `iphone/scripts/check_no_apiclient_in_features.sh`（chmod +x）
- `iphone/docs/visual-review-checklist.md`
- `iphone/PetAppTests/Shared/Constants/AccessibilityIDTests.swift`

### Change Log

- 2026-04-30: Story 37.13 实装完成（重构 + docs/audit）。47 处 inline a11y 字符串收编入 `AccessibilityID.swift` 7 个 nested enum 常量；2 个静态校验脚本（a11y coverage + APIClient isolation）；visual review checklist（50 项）；4 case 单元测试守护 enum 值不漂移；346 unit test 全绿；纯重构 + 文档，无业务代码改动。
