// ResetKeychainUseCase.swift
// Story 2.8: dev "重置身份" 按钮的 UseCase 层。
// 单一职责：调 keychainStore.removeAll()；任何 throw 透传给 ViewModel 转成 alert。
//
// 命名说明：本 story 命名 ResetKeychainUseCase；如未来扩展清理 UserDefaults / 缓存等
// （epics.md 当前 scope 不含），可改名 ResetIdentityUseCase（协议名已为 ViewModel 留空间）。

import Foundation

public protocol ResetKeychainUseCaseProtocol: Sendable {
    func execute() async throws
}

public struct DefaultResetKeychainUseCase: ResetKeychainUseCaseProtocol {
    private let keychainStore: KeychainStoreProtocol

    public init(keychainStore: KeychainStoreProtocol) {
        self.keychainStore = keychainStore
    }

    public func execute() async throws {
        try keychainStore.removeAll()
    }
}
