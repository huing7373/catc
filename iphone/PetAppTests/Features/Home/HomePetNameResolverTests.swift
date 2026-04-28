// HomePetNameResolverTests.swift
// Story 5.5 codex round 1 [P2] fix 验证：
// HomeView petColumn 在 server 返回 pet=null 时（V1 §5.1 schema 明确允许：首次注册 / Reset 后）
// 必须有独立的 "暂无宠物" 文案，不能 fallback 回 "默认小猫" placeholder —— 否则会让 server 明确说
// "无宠物" 的账号显示成 "已有宠物且名为默认小猫"，掩盖 bug / 误导用户.
//
// 三态语义:
// 1. homeData == nil（loading）→ "默认小猫"
// 2. homeData != nil && pet == nil（V1 §5.1 schema 允许）→ "暂无宠物"
// 3. pet != nil → pet.name
//
// 详见 docs/lessons/2026-04-27-optional-domain-field-vs-loading-placeholder.md.

import XCTest
@testable import PetApp

@MainActor
final class HomePetNameResolverTests: XCTestCase {

    // MARK: - 工厂

    private func makeHomeData(pet: HomePet?) -> HomeData {
        HomeData(
            user: HomeUser(id: "10001", nickname: "u", avatarUrl: ""),
            pet: pet,
            stepAccount: HomeStepAccount(totalSteps: 0, availableSteps: 0, consumedSteps: 0),
            chest: HomeChest(
                id: "30001",
                status: .counting,
                unlockAt: Date(timeIntervalSince1970: 0),
                openCostSteps: 100,
                remainingSeconds: 600
            ),
            room: HomeRoom(currentRoomId: nil)
        )
    }

    // MARK: - case#1 (loading)：homeData == nil → loadingPlaceholder

    func testResolveReturnsLoadingPlaceholderWhenHomeDataNil() {
        let result = HomePetNameResolver.resolve(homeData: nil)
        XCTAssertEqual(
            result,
            HomePetNameResolver.loadingPlaceholder,
            "homeData == nil（首屏未加载完）应展示 loading placeholder"
        )
        // 双保险：锁住具体文案,防 lookup 改了 const 但语义没改
        XCTAssertEqual(result, "默认小猫")
    }

    // MARK: - case#2 (no-pet)：homeData != nil && pet == nil → noPetPlaceholder

    /// **核心修复 case**：V1 §5.1 schema 明确允许 pet: null（首次注册或 Reset 后）.
    /// fix 前: `viewModel.homeData?.pet?.name ?? "默认小猫"` —— pet=null 时输出 "默认小猫",
    /// 让 server 明确说"无宠物"的账号 UI 上显示成"有宠物且名为默认小猫", regression for backend truth.
    /// fix 后: 走独立 noPetPlaceholder "暂无宠物".
    func testResolveReturnsNoPetPlaceholderWhenLoadedWithNullPet() {
        let homeData = makeHomeData(pet: nil)
        let result = HomePetNameResolver.resolve(homeData: homeData)
        XCTAssertEqual(
            result,
            HomePetNameResolver.noPetPlaceholder,
            "homeData != nil 但 pet == nil（server 明确返回无宠物）应展示 'no pet' placeholder, 不能 fallback 到 loading 文案"
        )
        XCTAssertEqual(result, "暂无宠物")
        XCTAssertNotEqual(
            result,
            HomePetNameResolver.loadingPlaceholder,
            "no-pet 状态必须与 loading 状态文案区分,否则 server pet=null 会被误显示成 '默认小猫'"
        )
    }

    // MARK: - case#3 (has-pet)：pet != nil → 渲染 pet.name

    func testResolveReturnsPetNameWhenPetPresent() {
        let homeData = makeHomeData(
            pet: HomePet(id: "p1", petType: 1, name: "Mochi", currentState: .rest, equips: [])
        )
        let result = HomePetNameResolver.resolve(homeData: homeData)
        XCTAssertEqual(result, "Mochi")
    }

    // MARK: - case#4 (edge)：pet.name 是空字符串时按 server 原样展示（不 fallback）

    /// 与 HomeNicknameResolver 同精神：以 server 为准.
    /// 即使 server 极端下发空串,client 不应 fallback 到 placeholder —— 反之会掩盖 server bug.
    func testResolveReturnsEmptyPetNameVerbatimWhenServerSendsEmpty() {
        let homeData = makeHomeData(
            pet: HomePet(id: "p1", petType: 1, name: "", currentState: .rest, equips: [])
        )
        let result = HomePetNameResolver.resolve(homeData: homeData)
        XCTAssertEqual(
            result,
            "",
            "pet 非 nil 时必须按 server 原样展示 pet.name,即便是空串—— '以 server 为准' 的纪律"
        )
    }
}
