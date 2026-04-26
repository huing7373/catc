// AccessibilityID.swift
// 集中定义主界面 6 大占位区块的 accessibility identifier 常量。
// 测试侧（PetAppTests / PetAppUITests）通过 @testable import PetApp 引用，
// 避免 inline string 导致测试与生产代码字符串漂移。
//
// 命名风格：<feature>_<element>（小驼峰），AC6 约定。
// 后续 stories 扩展更多 feature 时遵循同样风格。

import Foundation

public enum AccessibilityID {
    public enum Home {
        public static let userInfo = "home_userInfo"
        public static let petArea = "home_petArea"
        public static let stepBalance = "home_stepBalance"
        public static let chestArea = "home_chestArea"
        public static let btnRoom = "home_btnRoom"
        public static let btnInventory = "home_btnInventory"
        public static let btnCompose = "home_btnCompose"
        public static let versionLabel = "home_versionLabel"
        // Story 2.8: dev "重置身份" 按钮（仅 Debug build 渲染）+ alert reserved identifier。
        public static let btnResetIdentity = "home_btnResetIdentity"
        public static let resetIdentityAlert = "home_resetIdentityAlert"
    }

    // Story 2.3 新增：主界面跳转的全屏 Sheet placeholder a11y 标识。
    // 后续 Epic 12 / 24 / 33 实装真实 Room/Inventory/Compose View 时整体下线。
    public enum SheetPlaceholder {
        public static let roomContainer = "sheetPlaceholder_room"
        public static let roomTitle = "sheetPlaceholder_roomTitle"
        public static let inventoryContainer = "sheetPlaceholder_inventory"
        public static let inventoryTitle = "sheetPlaceholder_inventoryTitle"
        public static let composeContainer = "sheetPlaceholder_compose"
        public static let composeTitle = "sheetPlaceholder_composeTitle"
        public static let btnClose = "sheetPlaceholder_btnClose"
    }

    /// Story 2.6 新增：错误 UI 组件的 a11y 标识。
    /// Toast / Alert / Retry 三组件 + 容器 / 内容 / 按钮的细分标识，便于未来 UITest 定位。
    /// 命名风格与 `Home` / `SheetPlaceholder` 保持一致：`errorUI_<element>`（小驼峰前缀 + 下划线 + 元素名）。
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
    }

    /// Story 2.9 新增：LaunchingView 的 a11y 标识。
    /// 命名风格：launching_<element>（小驼峰前缀），与 `Home` / `SheetPlaceholder` / `ErrorUI` 一致。
    public enum Launching {
        public static let container = "launching_container"
        public static let logo = "launching_logo"
        public static let text = "launching_text"
        public static let progressIndicator = "launching_progressIndicator"
    }
}
