// AccessibilityID.swift
// 集中定义全 Scaffold 的 accessibility identifier 常量。
// 测试侧（PetAppTests / PetAppUITests）通过 @testable import PetApp（unit）或 project.yml line 67-69
// 把本文件直接编进 UITest target（黑盒）引用本文件，避免 inline string 漂移。
//
// 命名风格：<feature>_<element>（小驼峰），AC6 约定（注：AC6 来自 Story 37.7 的 AC 编号系统）。
// 后续 stories 扩展更多 feature 时遵循同样风格。
//
// Story 37.13 a11y 总表归并：
//   - 新增 7 个 nested enum（Tab / Room / Wardrobe / Friends / Profile / JoinRoomModal / Compose），
//     收编 Story 37.3 / 37.7 / 37.8 / 37.9 / 37.10 / 37.11 / 37.12 落地的 inline a11y identifier 字符串。
//   - 删除 Home 内 deprecated `btnRoom` / `btnInventory` / `btnCompose`（Story 37.3 主入口已改 4 Tab IA，
//     view 已删，常量保留无意义）。
//   - 全部新增常量字符串值与原 inline 字符串**一字一字相同**（运行时行为零漂移；UITest 不应有任何行为变化）。
//   - 动态拼接的 a11y 走 `static func helper(_:)` 模式（Swift `static let` 不支持参数）。

import Foundation

public enum AccessibilityID {
    public enum Home {
        // Story 37.7 codex round 3 [P2-B] fix：值从 "home_userInfo" 改 "homeStatusBar".
        //   原因：老方案在父 HStack 挂 home_userInfo + overlay 内挂空 Text("") 双 identifier 共存,
        //   空 Text 让 VoiceOver 把 zero-sized node 当 focusable element → 用户滑过 statusBar 顶部
        //   会卡在空白. 改为父级单 identifier "homeStatusBar"（既满 Story 37.7 AC8 新锚约定,
        //   又自然兼容老 UITest —— 它们用的是 enum 引用 AccessibilityID.Home.userInfo, 值改了不破坏 caller）.
        //   同精神 lesson: docs/lessons/2026-04-30-swiftui-empty-text-overlay-voiceover-trap.md.
        public static let userInfo = "homeStatusBar"
        public static let petArea = "home_petArea"
        public static let stepBalance = "home_stepBalance"
        public static let chestArea = "home_chestArea"
        public static let versionLabel = "home_versionLabel"
        // Story 2.8: dev "重置身份" 按钮（仅 Debug build 渲染）+ alert reserved identifier。
        public static let btnResetIdentity = "home_btnResetIdentity"
        public static let resetIdentityAlert = "home_resetIdentityAlert"
        // Story 5.5: petArea 下的 pet 名称 + chestArea 上的倒计时显示
        public static let petName = "home_petName"
        public static let chestRemaining = "home_chestRemaining"

        // Story 37.13 新增：Story 37.7 落地的 inline 字符串收编为常量（值不变）.
        public static let catStage = "homeCatStage"
        public static let teamIdleCardCreate = "homeTeamIdleCard_create"
        public static let teamIdleCardJoin = "homeTeamIdleCard_join"

        // Story 37.13 删除：Story 37.3 落地的 deprecated 3 CTA 按钮常量
        //   (`btnRoom` = "home_btnRoom" / `btnInventory` = "home_btnInventory" / `btnCompose` = "home_btnCompose").
        //   原因：Story 37.3 主入口已改 4 Tab IA, 3 CTA 按钮 view 已删, 常量保留无意义.
        //   清理 = 删除这 3 个常量 + grep 校验无 caller 引用（AC8）.
    }

    // Story 37.3：原 SheetPlaceholder enum 整段删除（关联文件 SheetPlaceholders/ 已删除,常量无任何引用）.
    // Story 37.13: 4 Tab a11y identifier (`tab_home` / `tab_wardrobe` / `tab_friends` / `tab_profile`)
    // 与占位 view a11y identifier (`wardrobeView` / `friendsView` / `profileView` / `roomViewPlaceholder`
    // / `joinRoomModalPlaceholder`) 走下方新建的 7 个 nested enum 收编为常量.

    /// Story 2.6 新增：错误 UI 组件的 a11y 标识。
    /// Toast / Alert / Retry 三组件 + 容器 / 内容 / 按钮的细分标识，便于未来 UITest 定位。
    /// 命名风格与 `Home` 保持一致：`errorUI_<element>`（小驼峰前缀 + 下划线 + 元素名）。
    public enum ErrorUI {
        public static let toast = "errorUI_toast"
        public static let toastMessage = "errorUI_toastMessage"
        public static let alertOverlay = "errorUI_alertOverlay"
        public static let alertTitle = "errorUI_alertTitle"
        public static let alertMessage = "errorUI_alertMessage"
        public static let alertOKButton = "errorUI_alertOKButton"
        public static let retryView = "errorUI_retryView"
        public static let retryMessage = "errorUI_retryMessage"
        public static let retryButton = "errorUI_retryButton"
        // Story 5.5 round 8 [P1] fix: terminal/unrecoverable error 的静态全屏 fallback page
        // (TerminalErrorView). 用于 bootstrap 路径的 .unauthorized / .missingCredentials /
        // .decoding / permanent business error —— 无任何按钮, user 必须主动杀进程退出.
        // 详见 docs/lessons/2026-04-27-bootstrap-terminal-error-static-fallback-page.md.
        public static let terminalView = "errorUI_terminalView"
        public static let terminalTitle = "errorUI_terminalTitle"
        public static let terminalMessage = "errorUI_terminalMessage"
        public static let terminalHelp = "errorUI_terminalHelp"
    }

    /// Story 2.9 新增：LaunchingView 的 a11y 标识。
    /// 命名风格：launching_<element>（小驼峰前缀），与 `Home` / `ErrorUI` 一致。
    public enum Launching {
        public static let container = "launching_container"
        public static let logo = "launching_logo"
        public static let text = "launching_text"
        public static let progressIndicator = "launching_progressIndicator"
    }

    // MARK: - Story 37.13 落地的 7 个新 nested enum（Tab / Room / Wardrobe / Friends / Profile / JoinRoomModal / Compose）

    /// Story 37.3 落地的 4 Tab a11y identifier（MainTabView 浮动 TabBar）.
    /// 命名 `Tab` 而非 `Tabs`：SwiftUI 内置类型也是 `Tab` 单数；caller 写 `AccessibilityID.Tab.home` 前缀消歧.
    public enum Tab {
        public static let home = "tab_home"
        public static let wardrobe = "tab_wardrobe"
        public static let friends = "tab_friends"
        public static let profile = "tab_profile"

        /// 动态拼 `tab_<rawValue>` 模式 helper（caller: MainTabView line 86）.
        /// caller 走 `AccessibilityID.Tab.identifier(for: tab.rawValue)` 而非 inline 拼字符串.
        ///
        /// 关键设计：参数取 `String`（rawValue）而非 `AppTab` 类型 —— `AccessibilityID.swift` 通过
        /// project.yml line 67-69 同时编进 PetApp + PetAppUITests 两个 target；`AppTab` 类型仅在
        /// PetApp target 定义，UITest target 看不到 → 取 String 让本文件无 cross-target 类型依赖.
        public static func identifier(for rawValue: String) -> String { "tab_\(rawValue)" }
    }

    /// Story 37.8 / 37.3 落地的 RoomScaffoldView + RoomViewPlaceholder a11y identifier.
    /// **a11y 命名严格 `roomIdDisplay`，禁止旧名 `roomCodeDisplay`**（epic AC line 4881 钦定，AC8 grep 校验守护）.
    public enum Room {
        public static let returnButton = "returnButton"
        public static let roomIdDisplay = "roomIdDisplay"
        public static let copyButton = "copyButton"
        public static let sharedStage = "sharedStage"
        public static let leaveButton = "leaveButton"
        /// `roomMember_<index>` 模式（index 0..3）；caller 走 `AccessibilityID.Room.member(at: index)` helper.
        public static func member(at index: Int) -> String { "roomMember_\(index)" }
        /// Story 37.3 落地的占位 view a11y identifier（RoomViewPlaceholder.swift）.
        public static let viewPlaceholder = "roomViewPlaceholder"
    }

    /// Story 37.9 / 37.3 落地的 WardrobeScaffoldView a11y identifier.
    public enum Wardrobe {
        public static let view = "wardrobeView"
        public static let diamondCount = "wardrobeDiamondCount"
        public static let composeEntry = "wardrobeComposeEntry"
        public static let equipButton = "wardrobeEquipButton"
        /// `wardrobeCategory_<rawValue>` 模式（rawValue = CosmeticCategory.allCases）；caller 走 helper.
        public static func category(_ rawValue: String) -> String { "wardrobeCategory_\(rawValue)" }
        /// `wardrobeItem_<id>` 模式；caller 走 helper.
        public static func item(_ id: String) -> String { "wardrobeItem_\(id)" }
    }

    /// Story 37.10 / 37.3 落地的 FriendsScaffoldView a11y identifier.
    public enum Friends {
        public static let view = "friendsView"
        public static let addButton = "friendsAddButton"
        public static let myRoomShareButton = "friendsMyRoomShareButton"
        public static let myRoomCard = "friendsMyRoomCard"
        public static let toast = "friendsToast"
        /// `friendsTab_<rawValue>`（FriendsTab.allCases）；caller 走 helper.
        public static func tab(_ rawValue: String) -> String { "friendsTab_\(rawValue)" }
        /// `friendRow_<id>` 模式；caller 走 helper.
        public static func row(_ id: String) -> String { "friendRow_\(id)" }
        /// `friendActionButton_<id>` 模式（"加好友" / "加入" / "查看资料" 等动作按钮共用）；caller 走 helper.
        public static func actionButton(_ id: String) -> String { "friendActionButton_\(id)" }
    }

    /// Story 37.11 / 37.3 落地的 ProfileScaffoldView a11y identifier.
    public enum Profile {
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
        /// `profileCollectionCell_<id>` 模式（recent collection 5 个）；caller 走 helper.
        public static func collectionCell(_ id: String) -> String { "profileCollectionCell_\(id)" }
        /// `profileMenu_<rawValue>`（ProfileMenuItem.allCases）；caller 走 helper.
        public static func menu(_ rawValue: String) -> String { "profileMenu_\(rawValue)" }
    }

    /// Story 37.12 / 37.3 落地的 JoinRoomModal + JoinRoomModalPlaceholder a11y identifier.
    public enum JoinRoomModal {
        public static let modal = "joinRoomModal"
        public static let closeButton = "joinRoomCloseButton"
        public static let input = "joinRoomInput"
        public static let cancelButton = "joinRoomCancelButton"
        public static let confirmButton = "joinRoomConfirmButton"
        /// Story 37.3 落地的占位 view a11y identifier（JoinRoomModalPlaceholder.swift）.
        public static let modalPlaceholder = "joinRoomModalPlaceholder"
    }

    /// Story 37.3 落地的占位 view a11y identifier（RootView.swift line 449 ComposeSheetPlaceholder, dummy compose route）.
    /// ADR-0009 §3.4 SheetType 白名单仍保留 `.compose`（Story 33.1 决定具体形式 / 落地真实合成 view）.
    public enum Compose {
        public static let placeholder = "compose_placeholder"
    }
}
