// PetSpriteViewTests.swift
// Story 15.3 AC3 / AC4: PetSpriteView struct 构造层单元测试（视图组装层契约锁定）.
//
// 测试基础设施约束（与 Story 2.7 / 12.1 / 15.1 / 15.2 + ADR-0002 §3.1 衔接）：
//   - XCTest only；@testable import PetApp.
//   - 不引 ViewInspector / SnapshotTesting（视图层 render-tree 行为交给 AC5 ios-simulator MCP 录屏验证）.
//   - 仅断言可构造 + 公共 `state` 字段读取正确（`currentIdentifier` / `accessibilityLabel` 是 private，
//     视觉契约由 UITest a11y 锚 + MCP 录屏覆盖）.
//
// 本文件是 PetSpriteView 第一个独立单元测试文件：Story 8.4 落地时仅依赖 HomeViewModelTests 覆盖
// state 流转 + RoomUITests 覆盖 a11y identifier，view-level 等价类构造没有锚点；本 story 新增独立测试
// 让"PetSpriteView(state:) struct 可构造 + state 字段读取正确"这条 view-level 契约有 XCTest 守护.

import XCTest
@testable import PetApp

@MainActor
final class PetSpriteViewTests: XCTestCase {

    // MARK: - case#A happy: PetSpriteView(state: .rest)

    /// Story 15.3 AC4 case#A: `PetSpriteView(state: .rest)` 可构造 + `state` 字段 == `.rest`.
    /// 退化为视图层等价类构造测试（同 Story 12.1 / 15.1 ViewModel 状态字段直读模式）.
    func testPetSpriteViewWithRestStateExposesRestField() {
        let view = PetSpriteView(state: .rest)
        XCTAssertEqual(view.state, .rest,
                       "PetSpriteView(state: .rest) → view.state 应为 .rest")
    }

    // MARK: - case#B happy: PetSpriteView(state: .walk)

    /// Story 15.3 AC4 case#B: `PetSpriteView(state: .walk)` 可构造 + `state` 字段 == `.walk`.
    func testPetSpriteViewWithWalkStateExposesWalkField() {
        let view = PetSpriteView(state: .walk)
        XCTAssertEqual(view.state, .walk,
                       "PetSpriteView(state: .walk) → view.state 应为 .walk")
    }

    // MARK: - case#C happy: PetSpriteView(state: .run)

    /// Story 15.3 AC4 case#C: `PetSpriteView(state: .run)` 可构造 + `state` 字段 == `.run`.
    func testPetSpriteViewWithRunStateExposesRunField() {
        let view = PetSpriteView(state: .run)
        XCTAssertEqual(view.state, .run,
                       "PetSpriteView(state: .run) → view.state 应为 .run")
    }

    // MARK: - case#G happy: 默认 size 参数 == 180（Story 15.1 review r1 锁定）

    /// Story 15.3 sanity check：PetSpriteView 默认 size 仍为 180（HomeView catStage 主界面尺寸基线）.
    /// 防止本 story `.transition` 升级误改 size 默认值 → HomeView catStage 视觉漂移.
    func testPetSpriteViewDefaultSizeIsOneHundredEighty() {
        let view = PetSpriteView(state: .rest)
        XCTAssertEqual(view.size, 180,
                       "PetSpriteView 默认 size 应为 180（HomeView catStage 视觉基线）")
    }

    // MARK: - case#H happy: 自定义 size 参数（RoomScaffoldView 成员行 40pt 路径）

    /// Story 15.3 sanity check：PetSpriteView(state:size:) 自定义 size 仍生效（Story 15.1 review r1 落地的
    /// `size: 40` 路径，RoomScaffoldView 成员行 PetSpriteView 调用契约不变）.
    func testPetSpriteViewCustomSizeIsRespected() {
        let view = PetSpriteView(state: .walk, size: 40)
        XCTAssertEqual(view.size, 40,
                       "PetSpriteView(state:size: 40) → view.size 应为 40（RoomScaffoldView 成员行调用契约）")
        XCTAssertEqual(view.state, .walk,
                       "size 自定义不应影响 state 字段")
    }
}
