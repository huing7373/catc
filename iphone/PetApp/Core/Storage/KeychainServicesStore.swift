// KeychainServicesStore.swift
// Story 5.1 AC2: KeychainStoreProtocol 真实实装，基于 Apple Security.framework。
//
// 替换 Story 2.8 的占位 InMemoryKeychainStore（占位仍保留作为测试便利 + 模板示范）。
// 协议不变（Story 2.8 锁），上层 ResetKeychainUseCase / ResetIdentityViewModel
// / MockKeychainStore 测试零回归。
//
// 设计要点：
// 1. kSecClass = kSecClassGenericPassword：通用密码项，最适合 token / id 这类字符串
// 2. kSecAttrService = "com.zhuming.pet.app"（与 bundle ID 一致，硬编码）
//    硬编码而非 Bundle.main.bundleIdentifier：unit test target 跑时 bundle id 是
//    `com.zhuming.pet.app.tests`，会让测试与生产走不同 keychain namespace，
//    导致 InMemoryKeychainStoreTests 之外的测试场景难定位；硬编码统一 service
//    保证 test target 与生产 target 操作的是同一组 keychain items（已通过测试
//    隔离 setUp/tearDown 的 removeAll() 兜住，详见 AC4）。
// 3. kSecAttrAccount = key（调用方传入的 String，如 KeychainKey.guestUid.rawValue）
// 4. kSecAttrAccessible = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly:
//    - 设备解锁后可读（首次解锁前 App 起不来不是问题：iOS 启动 App 必定在解锁后）
//    - ThisDeviceOnly：不参与 iCloud Keychain sync，与 NFR7 "Keychain 持久识别同账号"
//      意图一致（跨设备同账号是 Post-MVP 微信绑定的事，不靠 Keychain sync）
// 5. set 实装：SecItemAdd → 若 errSecDuplicateItem 则 SecItemUpdate（标准 upsert 模式）
// 6. removeAll 实装：选 A 方案 —— 一次 SecItemDelete 带 kSecClass + service 整删，
//    比遍历 KeychainKey.allCases 更彻底（避免漏删未来新增 key）
// 7. 全部方法 throws KeychainError：失败的 OSStatus 包装成 KeychainError.osStatus；
//    `get` 拿到非 string data 包装成 KeychainError.unexpectedDataFormat
// 8. NSLock 包裹 set/get/remove/removeAll？—— **不需要**：Apple SecItem* API 内部
//    线程安全（Apple Security framework 文档承诺）；上层再加锁是过度防御。
//
// 不引入 actor：与 MockKeychainStore / InMemoryKeychainStore 同模式（@unchecked Sendable
// + 同步方法），SecItem* 本身同步阻塞，包 actor 增加 await 链无收益。

import Foundation
import Security

public final class KeychainServicesStore: KeychainStoreProtocol, @unchecked Sendable {
    /// keychain 命名空间：所有 item 共享此 service 字段（与 bundle ID 一致）。
    /// 硬编码而非 Bundle.main.bundleIdentifier —— 见文件头第 2 点。
    public static let service: String = "com.zhuming.pet.app"

    public init() {}

    public func set(_ value: String, forKey key: String) throws {
        guard let data = value.data(using: .utf8) else {
            throw KeychainError.unexpectedDataFormat(operation: "set")
        }

        // 标准 upsert 模式：先 add，撞重复时 update
        let addQuery: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: Self.service,
            kSecAttrAccount as String: key,
            kSecValueData as String: data,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        ]
        let addStatus = SecItemAdd(addQuery as CFDictionary, nil)

        if addStatus == errSecSuccess {
            return
        }
        if addStatus == errSecDuplicateItem {
            // update path：query 不含 value / accessible，attrs 含 value
            let updateQuery: [String: Any] = [
                kSecClass as String: kSecClassGenericPassword,
                kSecAttrService as String: Self.service,
                kSecAttrAccount as String: key
            ]
            let attrsToUpdate: [String: Any] = [
                kSecValueData as String: data
            ]
            let updateStatus = SecItemUpdate(updateQuery as CFDictionary, attrsToUpdate as CFDictionary)
            guard updateStatus == errSecSuccess else {
                throw KeychainError.osStatus(updateStatus, operation: "set.update")
            }
            return
        }
        throw KeychainError.osStatus(addStatus, operation: "set.add")
    }

    public func get(forKey key: String) throws -> String? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: Self.service,
            kSecAttrAccount as String: key,
            kSecReturnData as String: kCFBooleanTrue as Any,
            kSecMatchLimit as String: kSecMatchLimitOne
        ]
        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)

        if status == errSecItemNotFound {
            return nil
        }
        guard status == errSecSuccess else {
            throw KeychainError.osStatus(status, operation: "get")
        }
        guard let data = result as? Data, let value = String(data: data, encoding: .utf8) else {
            throw KeychainError.unexpectedDataFormat(operation: "get")
        }
        return value
    }

    public func remove(forKey key: String) throws {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: Self.service,
            kSecAttrAccount as String: key
        ]
        let status = SecItemDelete(query as CFDictionary)
        // 不存在的 key 不报错（与 InMemoryKeychainStore 行为一致）
        if status == errSecSuccess || status == errSecItemNotFound {
            return
        }
        throw KeychainError.osStatus(status, operation: "remove")
    }

    public func removeAll() throws {
        // 整删本 App service 下所有 generic password；比遍历 KeychainKey.allCases 更彻底
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: Self.service
        ]
        let status = SecItemDelete(query as CFDictionary)
        if status == errSecSuccess || status == errSecItemNotFound {
            return
        }
        throw KeychainError.osStatus(status, operation: "removeAll")
    }
}
