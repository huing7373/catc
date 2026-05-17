// AccessibilityIDTests.swift
// Story 37.13 AC7：守护 AccessibilityID enum 常量字符串值与 Story 37.7-37.12 落地的
// inline 字符串运行时等价（防 dev 重构时手滑改值）.
//
// 测试策略：直接断言 Tab.home == "tab_home" 等字符串常量值；
// 不走 UITest（UITest 太慢 + 走真机 sim 才能跑），单元测试 quick green / red.

import XCTest
@testable import PetApp

final class AccessibilityIDTests: XCTestCase {

    // MARK: - case#1: Tab nested enum 4 个常量值 + identifier(for:) helper

    func testTabIdentifiers() {
        XCTAssertEqual(AccessibilityID.Tab.home, "tab_home")
        XCTAssertEqual(AccessibilityID.Tab.wardrobe, "tab_wardrobe")
        XCTAssertEqual(AccessibilityID.Tab.friends, "tab_friends")
        XCTAssertEqual(AccessibilityID.Tab.profile, "tab_profile")

        // helper 校验：identifier(for:) 应与 static let 值一致.
        XCTAssertEqual(AccessibilityID.Tab.identifier(for: AppTab.home.rawValue), "tab_home")
        XCTAssertEqual(AccessibilityID.Tab.identifier(for: AppTab.wardrobe.rawValue), "tab_wardrobe")
        XCTAssertEqual(AccessibilityID.Tab.identifier(for: AppTab.friends.rawValue), "tab_friends")
        XCTAssertEqual(AccessibilityID.Tab.identifier(for: AppTab.profile.rawValue), "tab_profile")
    }

    // MARK: - case#1b: Tab.identifier(for:) 必须返回声明常量本身（防 codex round 4 [P2] drift）

    /// 守护：`identifier(for: rawValue)` 返回的字符串必须 **逐字** 等于同 enum 声明常量；
    /// 不能仅靠 `"tab_\(rawValue)"` 拼接——这种拼接会让未来 AppTab.rawValue 改名时
    /// runtime 拼出新值但 declared constants / UITests 仍写旧值，refactor 想防的 drift 原地失效.
    /// codex round 4 [P2] 修：identifier(for:) 改为 switch 到声明常量，本测试守护该不变量.
    func testTabIdentifierHelperReturnsDeclaredConstants() {
        XCTAssertEqual(AccessibilityID.Tab.identifier(for: "home"),     AccessibilityID.Tab.home)
        XCTAssertEqual(AccessibilityID.Tab.identifier(for: "wardrobe"), AccessibilityID.Tab.wardrobe)
        XCTAssertEqual(AccessibilityID.Tab.identifier(for: "friends"), AccessibilityID.Tab.friends)
        XCTAssertEqual(AccessibilityID.Tab.identifier(for: "profile"),  AccessibilityID.Tab.profile)
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
        // member helper（4 个 member 位）.
        XCTAssertEqual(AccessibilityID.Room.member(at: 0), "roomMember_0")
        XCTAssertEqual(AccessibilityID.Room.member(at: 1), "roomMember_1")
        XCTAssertEqual(AccessibilityID.Room.member(at: 3), "roomMember_3")
    }

    // MARK: - case#3: JoinRoomModal nested enum 5 视觉锚 + placeholder

    func testJoinRoomModalIdentifiers() {
        XCTAssertEqual(AccessibilityID.JoinRoomModal.modal, "joinRoomModal")
        XCTAssertEqual(AccessibilityID.JoinRoomModal.closeButton, "joinRoomCloseButton")
        XCTAssertEqual(AccessibilityID.JoinRoomModal.input, "joinRoomInput")
        XCTAssertEqual(AccessibilityID.JoinRoomModal.cancelButton, "joinRoomCancelButton")
        XCTAssertEqual(AccessibilityID.JoinRoomModal.confirmButton, "joinRoomConfirmButton")
        XCTAssertEqual(AccessibilityID.JoinRoomModal.modalPlaceholder, "joinRoomModalPlaceholder")
    }

    // MARK: - case#4: Wardrobe / Friends / Profile / Home 扩展 / Compose 余下常量

    func testWardrobeFriendsProfileExtraIdentifiers() {
        // Wardrobe.
        XCTAssertEqual(AccessibilityID.Wardrobe.view, "wardrobeView")
        XCTAssertEqual(AccessibilityID.Wardrobe.diamondCount, "wardrobeDiamondCount")
        XCTAssertEqual(AccessibilityID.Wardrobe.composeEntry, "wardrobeComposeEntry")
        XCTAssertEqual(AccessibilityID.Wardrobe.equipButton, "wardrobeEquipButton")
        XCTAssertEqual(AccessibilityID.Wardrobe.category("hat"), "wardrobeCategory_hat")
        XCTAssertEqual(AccessibilityID.Wardrobe.item("abc123"), "wardrobeItem_abc123")
        // Story 24.3: 品质筛选条 chip helper（"全部"→"all" / 4 品质→Rarity.rawValue）.
        XCTAssertEqual(AccessibilityID.Wardrobe.rarityFilter("all"), "wardrobeRarityFilter_all")
        XCTAssertEqual(AccessibilityID.Wardrobe.rarityFilter("N"), "wardrobeRarityFilter_N")
        XCTAssertEqual(AccessibilityID.Wardrobe.rarityFilter("R"), "wardrobeRarityFilter_R")
        XCTAssertEqual(AccessibilityID.Wardrobe.rarityFilter("SR"), "wardrobeRarityFilter_SR")
        XCTAssertEqual(AccessibilityID.Wardrobe.rarityFilter("SSR"), "wardrobeRarityFilter_SSR")

        // Friends.
        XCTAssertEqual(AccessibilityID.Friends.view, "friendsView")
        XCTAssertEqual(AccessibilityID.Friends.addButton, "friendsAddButton")
        XCTAssertEqual(AccessibilityID.Friends.myRoomShareButton, "friendsMyRoomShareButton")
        XCTAssertEqual(AccessibilityID.Friends.myRoomCard, "friendsMyRoomCard")
        XCTAssertEqual(AccessibilityID.Friends.toast, "friendsToast")
        XCTAssertEqual(AccessibilityID.Friends.tab("all"), "friendsTab_all")
        XCTAssertEqual(AccessibilityID.Friends.row("u1"), "friendRow_u1")
        XCTAssertEqual(AccessibilityID.Friends.actionButton("u1"), "friendActionButton_u1")

        // Profile.
        XCTAssertEqual(AccessibilityID.Profile.view, "profileView")
        XCTAssertEqual(AccessibilityID.Profile.headerCard, "profileHeaderCard")
        XCTAssertEqual(AccessibilityID.Profile.statsCard, "profileStatsCard")
        XCTAssertEqual(AccessibilityID.Profile.weChatCard, "profileWeChatCard")
        XCTAssertEqual(AccessibilityID.Profile.weChatCardBound, "profileWeChatCardBound")
        XCTAssertEqual(AccessibilityID.Profile.collectionViewAll, "profileCollectionViewAll")
        XCTAssertEqual(AccessibilityID.Profile.toast, "profileToast")
        XCTAssertEqual(AccessibilityID.Profile.weChatModal, "profileWeChatModal")
        XCTAssertEqual(AccessibilityID.Profile.weChatBindButton, "profileWeChatBindButton")
        XCTAssertEqual(AccessibilityID.Profile.weChatCancelButton, "profileWeChatCancelButton")
        XCTAssertEqual(AccessibilityID.Profile.collectionCell("rc1"), "profileCollectionCell_rc1")
        XCTAssertEqual(AccessibilityID.Profile.menu("settings"), "profileMenu_settings")

        // Home 扩展.
        XCTAssertEqual(AccessibilityID.Home.catStage, "homeCatStage")
        XCTAssertEqual(AccessibilityID.Home.teamIdleCardCreate, "homeTeamIdleCard_create")
        XCTAssertEqual(AccessibilityID.Home.teamIdleCardJoin, "homeTeamIdleCard_join")

        // Compose.
        XCTAssertEqual(AccessibilityID.Compose.placeholder, "compose_placeholder")
    }
}
