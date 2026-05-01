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
        // Story 37.3 deprecated: 3 CTA 按钮已删除（ADR-0009 §3.5 步骤 4 / 主入口改 4 Tab IA）;
        // 常量保留避免触发其它 import 站点漂移; Story 37.13 a11y 总表归并时一并清理.
        public static let btnRoom = "home_btnRoom"
        public static let btnInventory = "home_btnInventory"
        public static let btnCompose = "home_btnCompose"
        public static let versionLabel = "home_versionLabel"
        // Story 2.8: dev "重置身份" 按钮（仅 Debug build 渲染）+ alert reserved identifier。
        public static let btnResetIdentity = "home_btnResetIdentity"
        public static let resetIdentityAlert = "home_resetIdentityAlert"
        // Story 5.5: petArea 下的 pet 名称 + chestArea 上的倒计时显示
        public static let petName = "home_petName"
        public static let chestRemaining = "home_chestRemaining"
    }

    // Story 37.3：原 SheetPlaceholder enum 整段删除（关联文件 SheetPlaceholders/ 已删除,常量无任何引用）.
    // 4 Tab a11y identifier (`tab_home` / `tab_wardrobe` / `tab_friends` / `tab_profile`) 与
    // 占位 view a11y identifier (`wardrobeView` / `friendsView` / `profileView` / `roomViewPlaceholder`
    // / `joinRoomModalPlaceholder`) 本 story 走 inline 字符串路径; Story 37.13 a11y 总表归并时
    // 一次性建立 AccessibilityID.Tab / AccessibilityID.Wardrobe 等 enum 并替换为常量.

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
    /// 命名风格：launching_<element>（小驼峰前缀），与 `Home` / `SheetPlaceholder` / `ErrorUI` 一致。
    public enum Launching {
        public static let container = "launching_container"
        public static let logo = "launching_logo"
        public static let text = "launching_text"
        public static let progressIndicator = "launching_progressIndicator"
    }
}
