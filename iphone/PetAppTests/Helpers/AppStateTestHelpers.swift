// AppStateTestHelpers.swift
// Story 37.4：AppState 测试便利构造器（ADR-0010 §3.6 测试影响 + Story 37.4 AC1 + AC8）.
//
// 设计：用 extension 形式给 AppState 加 testing helper，**不**新建 MockAppState 子类
// （AppState 是 final class，子类不可继承）.
// 节点 1 阶段相关字段（user/pet/stepAccount/chest/currentRoomId）就位；
// 其余字段（inventory/equips/emojiCatalog）保持默认空集合.
//
// 与 SessionStore 测试同模式（无 MockSessionStore 子类，直接构造实例 / 调 helper）.
//
// AC 锁定：与 AppStateTests.swift 内 hydrate / reset case 同模式.

import Foundation
@testable import PetApp

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
