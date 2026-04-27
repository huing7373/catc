// KeychainServicesStore.swift
// Story 5.1 AC2: KeychainStoreProtocol 真实实装，基于 Apple Security.framework。
//
// 替换 Story 2.8 的占位 InMemoryKeychainStore（占位仍保留作为测试便利 + 模板示范）。
// 协议不变（Story 2.8 锁），上层 ResetKeychainUseCase / ResetIdentityViewModel
// / MockKeychainStore 测试零回归。
//
// 设计要点：
// 1. kSecClass = kSecClassGenericPassword：通用密码项，最适合 token / id 这类字符串
// 2. kSecAttrService 默认 = "com.zhuming.pet.app"（与 bundle ID 一致），但 **可通过
//    `init(service:)` 注入**：单元测试传专属 namespace（带 UUID 后缀），避免 setUp/tearDown
//    的 `removeAll()` 清掉生产/手动调试遗留的 keychain items，也避免跨 test bundle
//    （PetAppTests vs PetAppUITests）cross-talk。生产 App（AppContainer）继续走默认值。
//    详见 docs/lessons/2026-04-27-keychain-service-namespace-injectable.md（codex round 2 [P2] finding）。
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
    /// 生产环境默认 keychain 命名空间（与 bundle ID 一致）。
    /// 测试通过 `init(service:)` 注入专属 namespace，**禁止**测试用此默认值（见文件头第 2 点）。
    public static let defaultService: String = "com.zhuming.pet.app"

    /// 本实例使用的 keychain `kSecAttrService` 值。
    /// 所有 set/get/remove/removeAll 都拼此 service —— 不同 service 的 keychain item 完全隔离。
    public let service: String

    /// - Parameter service: keychain `kSecAttrService` 命名空间。
    ///   - 生产代码（AppContainer）传默认值 `defaultService`（= bundle ID）
    ///   - 单元测试传专属 namespace（推荐 `"com.zhuming.pet.app.tests.\(UUID)"`），避免污染生产 namespace
    public init(service: String = KeychainServicesStore.defaultService) {
        self.service = service
    }

    public func set(_ value: String, forKey key: String) throws {
        guard let data = value.data(using: .utf8) else {
            throw KeychainError.unexpectedDataFormat(operation: "set")
        }

        // 标准 upsert 模式：先 add，撞重复时 update
        let addQuery: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: self.service,
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
                kSecAttrService as String: self.service,
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
            kSecAttrService as String: self.service,
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
            kSecAttrService as String: self.service,
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
            kSecAttrService as String: self.service
        ]
        let status = SecItemDelete(query as CFDictionary)
        if status == errSecSuccess || status == errSecItemNotFound {
            return
        }
        throw KeychainError.osStatus(status, operation: "removeAll")
    }
}
