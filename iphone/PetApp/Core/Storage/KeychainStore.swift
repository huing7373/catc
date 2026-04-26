// KeychainStore.swift
// Story 2.8: 占位实装 + 协议接缝。Story 5.1 真实 Security.framework 实装时**仅替换实装**，
// 协议不变；本 story 的所有 UseCase / ViewModel 测试在 Story 5.1 落地时零回归。
//
// 设计：协议提供 set / get / remove / removeAll 四方法；本 story 仅 removeAll 真实可用
// （足以驱动 dev "重置身份" 按钮）；其它三方法占位实装亦正确，但生产语义未验证（Story 5.1 验证）。
//
// **本 story 占位实装；Story 5.1 替换为 KeychainServicesStore（用 Security.framework / kSecClassGenericPassword）。**
// 命名前缀 `InMemory*` 表明非生产；切勿误以为本类是生产 Keychain 包装。

import Foundation

public protocol KeychainStoreProtocol: Sendable {
    /// 保存 key-value（覆盖已存在）。throws：底层 Keychain access 错误。
    func set(_ value: String, forKey key: String) throws
    /// 读取 value；不存在返回 nil。throws：底层 Keychain access 错误（**不**含 itemNotFound）。
    func get(forKey key: String) throws -> String?
    /// 删除单个 key（不存在不报错）。throws：底层 Keychain access 错误。
    func remove(forKey key: String) throws
    /// 删除该 App **全部** Keychain 项（"重置身份" 按钮触发）。throws：底层 Keychain access 错误。
    func removeAll() throws
}

/// 占位实装：内部 `[String: String]` 字典 + NSLock；功能等价但**不持久化**（App 重启丢失）。
/// 不是生产代码：① 命名 `InMemory*` 前缀；② 文件头硬注明"Story 5.1 替换为 KeychainServicesStore"；
/// ③ 不接触 Security.framework，不调 kSecClass*。
///
/// 为什么本 story 用占位而非真 Keychain：
/// - Story 5.1（Epic 5）才是 KeychainStore 实装 story，节点 2 才到；本 story 是节点 1 dev 工具
/// - 占位实装 `removeAll()` 清空字典 = "清 Keychain" 语义等价（dev demo 视角）
/// - 协议先稳，未来替实装零回归
public final class InMemoryKeychainStore: KeychainStoreProtocol, @unchecked Sendable {
    private var storage: [String: String] = [:]
    private let lock = NSLock()

    public init() {}

    public func set(_ value: String, forKey key: String) throws {
        lock.lock(); defer { lock.unlock() }
        storage[key] = value
    }

    public func get(forKey key: String) throws -> String? {
        lock.lock(); defer { lock.unlock() }
        return storage[key]
    }

    public func remove(forKey key: String) throws {
        lock.lock(); defer { lock.unlock() }
        storage.removeValue(forKey: key)
    }

    public func removeAll() throws {
        lock.lock(); defer { lock.unlock() }
        storage.removeAll()
    }
}
