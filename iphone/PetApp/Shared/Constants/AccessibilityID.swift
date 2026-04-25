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
    }
}
