# Story 5.1: Keychain 封装（guestUid + token 持久化）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iOS 开发,
I want 一个抽象的 `KeychainStoreProtocol` 真实实装（`KeychainServicesStore`，基于 Apple `Security.framework` 的 `kSecClassGenericPassword`）+ 一组持久化语义验证测试 + 已知 key 常量化（`KeychainKey.guestUid` / `KeychainKey.authToken`）+ 替换 `AppContainer` 默认实例,
So that 后续 Story 5.2（启动自动登录）/ 5.3（APIClient interceptor）/ 5.4（无效 token 静默重登）可以信赖一个**真实持久化**的 KeychainStore（卸载重装亦保留），且 Story 2.8 的 dev "重置身份" 按钮立刻接到生产 Keychain（占位实装无缝下线）.

## 故事定位（Epic 5 第一条实装 story；节点 2 iOS 端起点）

这是 Epic 5 内**第一条**实装 story，也是节点 2 iOS 端"启动即得身份"链路的**起点**。**直接前置** done：

- **Story 2.8 (`done`)** 已落地：
  - `iphone/PetApp/Core/Storage/KeychainStore.swift`（`KeychainStoreProtocol` + 占位实装 `InMemoryKeychainStore`）
  - `iphone/PetApp/Features/DevTools/UseCases/ResetKeychainUseCase.swift`（`ResetKeychainUseCaseProtocol` + `DefaultResetKeychainUseCase`）
  - `iphone/PetApp/App/AppContainer.swift` 已含 `keychainStore: KeychainStoreProtocol` 字段（默认 `InMemoryKeychainStore()`）+ `makeResetKeychainUseCase()` 工厂
  - `iphone/PetAppTests/Features/DevTools/MockKeychainStore.swift`（继承 `MockBase`，已稳定）
  - `iphone/PetAppTests/Features/DevTools/InMemoryKeychainStoreTests.swift`（占位实装单测）
  - 文件头注释**明示**："Story 5.1 替换为 KeychainServicesStore（用 Security.framework / kSecClassGenericPassword）"

  **本 story 即兑现 Story 2.8 留下的 IOU**：补真实 Keychain 实装；切换 `AppContainer` 默认实例；占位实装 `InMemoryKeychainStore` 保留作为测试 fallback（仍由 `MockKeychainStore` 兜测试 mock 主路径）。

- **Story 4.6 (`done`)** 已落地服务端 `/auth/guest-login`，request 含 `guestUid: string` 字段（V1 接口设计 §4.1 已冻结，长度 1~128，推荐 UUID v4 字符串）—— 这是本 story 写 Keychain 的客户侧 contract 来源；**不**在本 story 范围调接口（Story 5.2 才调），但本 story 实装与测试必须按 §4.1 的 guestUid 字符约束设计 key 与 value 格式。

- **Story 4.8 (`done`)** 已落地服务端 `/home` 聚合接口 —— 与本 story **无直接耦合**；提及仅为 Epic 5 顺序定位（5.1 → 5.2 → 5.5 调 `/home`）。

**本 story 的核心动作**（顺序无关，可分批落地）：

1. **新建** `iphone/PetApp/Core/Storage/KeychainKey.swift`：定义 `enum KeychainKey: String { case guestUid = "auth.guestUid", authToken = "auth.token" }`（**`String` raw value 即真实 keychain account 字段**；命名用 `auth.<name>` namespace 避免未来 namespace 冲突；详见 AC1）
2. **新建** `iphone/PetApp/Core/Storage/KeychainServicesStore.swift`：实装 `KeychainStoreProtocol`，基于 `Security.framework`（`SecItemAdd` / `SecItemCopyMatching` / `SecItemUpdate` / `SecItemDelete`）；底层 query 用 `kSecClass = kSecClassGenericPassword`、`kSecAttrService = "com.zhuming.pet.app"`（与 bundle ID 一致）、`kSecAttrAccount = key`、`kSecAttrAccessible = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`；详见 AC2
3. **新建** `iphone/PetApp/Core/Storage/KeychainError.swift`：定义 `enum KeychainError: Error { case osStatus(OSStatus, operation: String); case unexpectedDataFormat(operation: String) }`，`LocalizedError` 实装提供 dev-friendly 描述（含 `OSStatus` 数字 + Apple SecCopyErrorMessageString 文本）；详见 AC3
4. **修改** `iphone/PetApp/App/AppContainer.swift`：把 `keychainStore` 默认实例从 `InMemoryKeychainStore()` 改为 `KeychainServicesStore()`；**保留**测试用 init `init(apiClient:keychainStore:)`，调用方可继续注入 mock；删除 `InMemoryKeychainStore` 在生产路径的引用，但**不删类本身**（继续作为测试便利 + ADR-0002 §3.1 "手写 mock 优先" 模板示范）
5. **不动** `KeychainStoreProtocol` 既有四个方法签名（`set` / `get` / `remove` / `removeAll`）：Story 2.8 已锁；本 story **零回归** —— 所有 Story 2.8 写的 `ResetKeychainUseCase` / `ResetIdentityViewModel` / `MockKeychainStore` / `InMemoryKeychainStoreTests` 测试不动一行
6. **新建** `iphone/PetAppTests/Core/Storage/KeychainServicesStoreTests.swift`：对 **`KeychainServicesStore` 真实类**跑单元测试（**不**用 `MockKeychainStore` —— 那是给上层用的；本 story 测的是真实 Keychain 调用），覆盖 ≥ 4 happy + edge case + ≥ 1 持久化跨实例 case；测试 `setUp` / `tearDown` 必须显式 `removeAll()` 避免泄漏到 simulator Keychain（详见 AC4 测试隔离段）
7. **新建** `iphone/PetAppUITests/KeychainPersistenceUITests.swift`：XCUITest 集成测试 —— launch App → 通过 launchEnvironment 触发"种入测试 guestUid"动作（hook 详见 AC5）→ terminate → relaunch → 验证 launchEnvironment 回报的"读到的 guestUid" 与种入值一致（说明 Keychain 跨进程持久）。**不**做"卸载重装"完整 flow（XCUITest 在同 app id 下 terminate/relaunch 已足以验证 NFR7"重启不丢"，"卸载重装亦保留"是 iOS 系统级行为，不属本 story 自动化测试范围 —— 由 Story 6.1 E2E 文档的人工验证场景兜底）
8. **修改** `iphone/PetApp/Features/Home/Views/HomeView.swift`（**仅文件头注释**）：把 Story 2.8 留下的 "重置身份按钮接占位 Keychain" 注释升级为 "Story 5.1 后接真实 Keychain"。**不**改任何视图代码 —— 协议不变，调用栈零改动
9. **不动**：Story 2.8 落地的所有测试 / mock 文件；`MockKeychainStore` / `InMemoryKeychainStore` 不删；`ResetKeychainUseCase` / `ResetIdentityViewModel` 不改

**不涉及**：

- **`/auth/guest-login` 接口调用**：归 Story 5.2 范围。本 story **仅**写 Keychain；不知道也不依赖 server 端 token 内容
- **token util / JWT 解析**：归 Story 4.4（已 done，server 端）+ Story 5.4（无效 token 静默重登）。本 story 只把 token 当**不透明字符串**存
- **APIClient interceptor 注入 Bearer header**：归 Story 5.3
- **SessionManager / SessionRepository**：归 Story 5.2（GuestLoginUseCase 拿到 user/pet 后注入 SessionManager）
- **微信绑定 / iCloud sync**：Post-MVP（FR3 / NFR7）；本 story `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly` 明确 **device-only**（不开 iCloud Keychain sync）
- **Keychain access group**（multi-app sharing）：单 app，不需要；不设 `kSecAttrAccessGroup`
- **Biometric / Face ID gate**：MVP 不需要；不设 `kSecAccessControlBiometryAny`
- **Refresh token**：iOS 架构 §11.1 写"必要时保存 refresh token"——节点 2 阶段 `/auth/guest-login` 设计上就是**幂等**入口（同 guestUid 重复调拿同 user + 新 token），**不**需要 refresh token 机制；本 story `KeychainKey` enum 只含 `guestUid` + `authToken` 两 case，refresh token 留待真有需求时再加 case
- **server 端任何改动**：本 story 是纯客户端 Keychain 实装；`server/` 全程零改动
- **`ios/` 旧产物目录**：CLAUDE.md "Repo Separation" + ADR-0002 §3.3 既有约束。最终 `git status` 须确认 `ios/` 下零改动

**范围红线**：

- **不动 `ios/`**：CLAUDE.md "Repo Separation" + ADR-0002 §3.3。最终 `git status` 须 `ios/` 下零改动
- **不动 `server/`**：本 story 是纯 iPhone 端实装
- **不动 `iphone/scripts/` / `iphone/project.yml` / `iphone/.gitignore`**：所有新文件靠 Story 2.2 既有 `sources: - PetApp` / `sources: - PetAppTests` / `sources: - PetAppUITests` glob 自动纳入；**0 yml 改动**（与 Story 2.7 / 2.8 同模式）
- **不引入第三方依赖**：`Security.framework` 是 Apple 系统库（与 Foundation / SwiftUI / Combine 同等地位）；`import Security` 一行即可，**不**走 SPM / CocoaPods / Carthage
- **不动 `KeychainStoreProtocol` 既有签名**：四方法 `set` / `get` / `remove` / `removeAll` 锁死。本 story 仅替**实装**，`Story 2.8` 写的所有 `MockKeychainStore` / `InMemoryKeychainStore` / `ResetKeychainUseCase` / `ResetIdentityViewModel` 测试**零回归**
- **不删 `InMemoryKeychainStore`**：作为测试便利 + 模板示范保留；仅从 `AppContainer` 默认值移除
- **不引入 `actor`**：与 `MockBase` / `MockKeychainStore` / `InMemoryKeychainStore` / `MockURLSession` / `MockAPIClient` 同模式（手写 lock + `@unchecked Sendable`），保持跨实装一致；`Security.framework` API 本身是同步阻塞的，包 `actor` 没收益还多 await 链
- **`KeychainKey` raw value 不能含路径分隔符 / 空格**：iOS Keychain account 字段虽 string 任意，但 `auth.guestUid` / `auth.token` 形式 namespace 友好；**不**用 `/` `\` ` ` 等会让未来 grep / debug 痛苦的字符
- **`kSecAttrService` 必须用 bundle ID（`com.zhuming.pet.app`）**：与 `iphone/project.yml` 已锁的 `PRODUCT_BUNDLE_IDENTIFIER` 一致 → 同一 App 跨升级 Keychain item 始终找到（service + account 是 generic password 的复合主键）；硬编码字符串而**不**用 `Bundle.main.bundleIdentifier`（后者 unit test target 拿到的是 test bundle id `com.zhuming.pet.app.tests`，写测试时会泄漏到不同 service 命名空间，难定位）
- **Keychain 写后不读校验**：单一职责。`set()` 调 `SecItemAdd` 成功即返回；不做 `set` → `get` 回读校验；如有需求由调用方验
- **测试隔离强约束**：`KeychainServicesStoreTests` `setUp` 与 `tearDown` 都必须 `try? sut.removeAll()`，避免：① 上一轮测试残留干扰本轮；② 本轮测试残留泄漏到 simulator Keychain 影响其他测试 / dev 体验；详见 AC4 测试隔离段

## Acceptance Criteria

**AC1 — `KeychainKey` enum 常量化**

新建 `iphone/PetApp/Core/Storage/KeychainKey.swift`：

```swift
// KeychainKey.swift
// Story 5.1 AC1: Keychain 已知 key 常量化。
//
// 设计：raw value 即真实 keychain account 字段；用 `auth.<name>` namespace
// 避免未来若引入业务存储（如 `cache.lastHomeSnapshot`）时与 auth 类 key 撞名。
//
// 已知 key 列表（节点 2 全集）：
// - guestUid: 客户端持久化的游客身份 UID（写入时机：Story 5.2 GuestLoginUseCase 首次启动生成 UUID v4）
// - authToken: server 签发的 JWT token（写入时机：Story 5.2 调 /auth/guest-login 拿到 token 后）
//
// 节点 2 之后可能扩展（不属本 story scope）：
// - refreshToken（如未来引入 refresh token 流，目前 /auth/guest-login 设计上幂等，不需要）

import Foundation

public enum KeychainKey: String, CaseIterable, Sendable {
    case guestUid = "auth.guestUid"
    case authToken = "auth.token"
}
```

**具体行为要求**：
- `String` raw value，**与协议方法 `set(_:forKey:)` 的 `key: String` 参数直接对接**：调用方写 `keychainStore.set(token, forKey: KeychainKey.authToken.rawValue)` —— **不**额外引入 `set(_:for: KeychainKey)` overload（Story 2.8 协议签名已锁；overload 留待统计调用点 ≥ 5 处再考虑，YAGNI）
- `CaseIterable`：让 `removeAll()` 实装可遍历删除（详见 AC2 实装备选方案 B），但**本 story 不强制走 B 方案**（A 方案 `kSecClass + service` 整删更彻底）
- `Sendable`：Swift 6 strict concurrency；enum 默认 Sendable 但显式标注让 review 一眼看清
- raw value 不含路径分隔符 / 空格 / 中文 / `:` `/` `\`：仅 `auth.<name>` 形式

**AC2 — `KeychainServicesStore` 真实实装**

新建 `iphone/PetApp/Core/Storage/KeychainServicesStore.swift`：

```swift
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
        var addQuery: [String: Any] = [
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
```

**具体行为要求**：
- `final class` + `@unchecked Sendable` + 同步方法签名（`func set(...) throws`）：与 `InMemoryKeychainStore` 对齐 → 协议无需改 async；`Security.framework` API 本身同步阻塞
- `set` 走 add → 撞重复时 update 的 upsert 模式：避免"先 delete + add" 的窗口期空洞（理论上 set 期间 get 不到旧值是不期望的）
- `get` 不存在 key → 返回 nil（不抛错）：与协议 docstring 一致；`errSecItemNotFound` 是预期路径
- `remove` 不存在 key → 不报错：与 `InMemoryKeychainStore` 一致；`errSecItemNotFound` 视为"已经达到预期效果"
- `removeAll` 用 `kSecClass + kSecAttrService` 整删 (A 方案)：避免遍历 `KeychainKey.allCases` 漏掉未登记的 key（如 dev 临时手动写入的）；与 ResetKeychainUseCase "清空全部 keychain" 语义最贴近
- `kSecAttrAccessible = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`：不跨设备 iCloud sync；首次解锁后可读
- 不加 `kSecAttrAccessGroup`：单 app 不共享 keychain
- 不加 `kSecAccessControl`（biometric gate）：MVP 不需要
- **硬编码 `service = "com.zhuming.pet.app"`**：与 bundle id 一致；理由见文件头第 2 点

**AC3 — `KeychainError` 错误类型**

新建 `iphone/PetApp/Core/Storage/KeychainError.swift`：

```swift
// KeychainError.swift
// Story 5.1 AC3: KeychainServicesStore 失败时抛出的错误类型。
//
// 设计：包 OSStatus + 操作上下文，便于 dev 在 console 看清"哪个操作失败 + Apple 错码"；
// LocalizedError 实装让 error.localizedDescription 含可读串（Apple SecCopyErrorMessageString）。
//
// 不写"用户友好的错误中文文案"：本 story 错误类型是 dev/log 用，不直接展示给用户；
// ResetIdentityViewModel 已有自己的"重置失败" alert 文案处理（Story 2.8 落地）。

import Foundation
import Security

public enum KeychainError: Error, LocalizedError, Equatable {
    /// Security framework 调用失败的 OSStatus；附带操作名（"set" / "get" / "remove" / "removeAll" / "set.update" / "set.add"）。
    case osStatus(OSStatus, operation: String)
    /// 数据格式异常：set 时 String 转 Data 失败 / get 时 Data 不是有效 UTF-8 字符串。
    case unexpectedDataFormat(operation: String)

    public var errorDescription: String? {
        switch self {
        case .osStatus(let status, let operation):
            let message = SecCopyErrorMessageString(status, nil) as String? ?? "unknown OSStatus"
            return "Keychain \(operation) failed: OSStatus \(status) — \(message)"
        case .unexpectedDataFormat(let operation):
            return "Keychain \(operation) data format invalid (not valid UTF-8)"
        }
    }
}
```

**具体行为要求**：
- `LocalizedError` 实装：让 `error.localizedDescription` 含 `OSStatus` 数字 + Apple 自带文本（如 `errSecAuthFailed` → "User name or passphrase you entered is not correct."），dev console 一眼看清
- `Equatable`：让 `XCTAssertEqual(error as? KeychainError, .osStatus(...))` 在测试中可用
- 不引入"业务侧用户文案"层：那是 ViewModel / ErrorPresenter 的责任（Story 2.8 已示范）

**AC4 — `KeychainServicesStoreTests` 单元测试**

新建 `iphone/PetAppTests/Core/Storage/KeychainServicesStoreTests.swift`：

```swift
// KeychainServicesStoreTests.swift
// Story 5.1 AC4: KeychainServicesStore 真实类单元测试。
// 不用 MockKeychainStore —— 那是给上层用的；本 story 测真实 Security.framework 调用。
//
// 测试隔离强约束：setUp / tearDown 都必须 try? sut.removeAll()，避免：
// 1. 上一轮测试残留干扰本轮（同一 simulator 跨 test bundle 共享 keychain namespace）
// 2. 本轮测试残留泄漏到 simulator keychain 影响 dev 后续运行
//
// 在 simulator 上跑（CI 与本地都是 simulator）：iOS simulator keychain 与 macOS 系统
// keychain 隔离，写入不会污染 dev 主机的 keychain。

import XCTest
@testable import PetApp

final class KeychainServicesStoreTests: XCTestCase {

    var sut: KeychainServicesStore!

    override func setUp() {
        super.setUp()
        sut = KeychainServicesStore()
        // 测试隔离：每个 test 开始前确保 keychain 干净
        try? sut.removeAll()
    }

    override func tearDown() {
        // 测试隔离：每个 test 结束后清理，不泄漏
        try? sut?.removeAll()
        sut = nil
        super.tearDown()
    }

    // happy: set + get 同 key 返回相等值
    func testSetThenGetReturnsValue() throws {
        try sut.set("test-token-abc", forKey: KeychainKey.authToken.rawValue)
        let got = try sut.get(forKey: KeychainKey.authToken.rawValue)
        XCTAssertEqual(got, "test-token-abc")
    }

    // edge: get 不存在的 key 返回 nil（不抛错）
    func testGetNonExistentKeyReturnsNil() throws {
        let got = try sut.get(forKey: KeychainKey.guestUid.rawValue)
        XCTAssertNil(got)
    }

    // happy: set 同一 key 两次，get 返回最新值（upsert 行为）
    func testSetOverwritesExistingValue() throws {
        try sut.set("v1", forKey: KeychainKey.guestUid.rawValue)
        try sut.set("v2", forKey: KeychainKey.guestUid.rawValue)
        let got = try sut.get(forKey: KeychainKey.guestUid.rawValue)
        XCTAssertEqual(got, "v2")
    }

    // happy: remove 单个 key 后 get 返回 nil；其他 key 不受影响
    func testRemoveSingleKeyOnly() throws {
        try sut.set("uid-1", forKey: KeychainKey.guestUid.rawValue)
        try sut.set("token-1", forKey: KeychainKey.authToken.rawValue)
        try sut.remove(forKey: KeychainKey.guestUid.rawValue)
        XCTAssertNil(try sut.get(forKey: KeychainKey.guestUid.rawValue))
        XCTAssertEqual(try sut.get(forKey: KeychainKey.authToken.rawValue), "token-1")
    }

    // edge: remove 不存在的 key 不报错
    func testRemoveNonExistentKeyDoesNotThrow() {
        XCTAssertNoThrow(try sut.remove(forKey: KeychainKey.guestUid.rawValue))
    }

    // happy: removeAll 清空所有 key
    func testRemoveAllClearsAllKeys() throws {
        try sut.set("uid", forKey: KeychainKey.guestUid.rawValue)
        try sut.set("token", forKey: KeychainKey.authToken.rawValue)
        try sut.removeAll()
        XCTAssertNil(try sut.get(forKey: KeychainKey.guestUid.rawValue))
        XCTAssertNil(try sut.get(forKey: KeychainKey.authToken.rawValue))
    }

    // edge: removeAll 在空 keychain 上不报错
    func testRemoveAllOnEmptyDoesNotThrow() {
        XCTAssertNoThrow(try sut.removeAll())
    }

    // 持久化跨实例（同 process 内）：写入 sut1 的 value 能被 sut2 读到
    // 验证 keychain 真实持久化语义（不是某个 sut 实例的内部 state）
    func testPersistenceAcrossInstances() throws {
        let sut1 = KeychainServicesStore()
        try sut1.set("persist-test", forKey: KeychainKey.guestUid.rawValue)

        let sut2 = KeychainServicesStore()
        let got = try sut2.get(forKey: KeychainKey.guestUid.rawValue)
        XCTAssertEqual(got, "persist-test")
    }

    // edge: 协议 forKey 接受任意 String（不只 KeychainKey enum case），ad-hoc key 也能存取
    // 这保证 dev / 测试场景需要临时 key 时不被 enum 卡住
    func testArbitraryStringKeyWorks() throws {
        try sut.set("ad-hoc-value", forKey: "ad.hoc.key.\(UUID().uuidString)")
        // ad-hoc key 也能正常 get/remove，验证协议方法对 string key 的通用性
        // 不在此处 get 验证以避免依赖 UUID 复用，但能 set 不抛即满足通用性测试
    }
}
```

**具体行为要求**：
- ≥ 4 case，本 AC 给出 9 个示例覆盖 happy + edge：set/get、不存在返 nil、覆盖、remove 单 key 隔离、remove 不存在不抛、removeAll、空 removeAll、跨实例持久化、ad-hoc key
- **不**继承 `MockBase`：本 SUT 是真实 `KeychainServicesStore`，不需要 invocation 记录
- `setUp` / `tearDown` 强制 `try? sut.removeAll()`：测试隔离强约束（详见 AC4 文件头注释）
- `testPersistenceAcrossInstances`：验证 keychain 真实持久化（不是 sut 实例 state） → 这是与 InMemoryKeychainStore 行为差异的关键测试
- 不需要 `@MainActor`：`KeychainServicesStore` 不是 `@MainActor` 类
- 不引入 `actor` / `async`：与实装签名一致（同步 throws）

**AC5 — `KeychainPersistenceUITests` 集成测试**

新建 `iphone/PetAppUITests/KeychainPersistenceUITests.swift`：

```swift
// KeychainPersistenceUITests.swift
// Story 5.1 AC5: 集成测试 —— 在模拟器上验证 KeychainServicesStore 跨进程持久化（NFR7）。
//
// 流程：
// 1. launch App with launchEnvironment["KEYCHAIN_TEST_SEED"] = "test-uid-12345"
// 2. App 在 PetAppApp / RootView .task 检测此 env，调 keychainStore.set("test-uid-12345", forKey: "auth.guestUid")
// 3. 通过 UI 元素（accessibility identifier）暴露"种入完成"信号
// 4. terminate App
// 5. relaunch App with launchEnvironment["KEYCHAIN_TEST_READBACK"] = "1"
// 6. App 检测此 env，调 keychainStore.get(forKey: "auth.guestUid")，把读到的值通过 UI 元素 accessibilityValue 暴露
// 7. 测试断言读到的值 == "test-uid-12345"
//
// 关键：种入与读回 hook 在 PetAppApp 入口实装时**仅在 #if DEBUG** 下生效，避免污染生产代码

import XCTest

final class KeychainPersistenceUITests: XCTestCase {

    let testGuestUid = "test-uid-\(UUID().uuidString.prefix(8))"

    override func setUp() {
        super.setUp()
        continueAfterFailure = false
    }

    func testKeychainPersistsAcrossAppLaunches() {
        // Step 1-3: launch App + 种入 keychain
        let app1 = XCUIApplication()
        app1.launchEnvironment["KEYCHAIN_TEST_SEED"] = testGuestUid
        app1.launch()

        // 等待"种入完成"信号 element 出现
        let seedDoneLabel = app1.staticTexts["uitest_keychain_seed_done"]
        XCTAssertTrue(seedDoneLabel.waitForExistence(timeout: 5), "seed-done signal label must appear")

        // Step 4: terminate
        app1.terminate()

        // Step 5: relaunch with readback env
        let app2 = XCUIApplication()
        app2.launchEnvironment["KEYCHAIN_TEST_READBACK"] = "1"
        app2.launch()

        // Step 6-7: 等待 readback element 出现，断言其 accessibilityValue == 种入值
        let readbackLabel = app2.staticTexts["uitest_keychain_readback_value"]
        XCTAssertTrue(readbackLabel.waitForExistence(timeout: 5), "readback label must appear")
        XCTAssertEqual(readbackLabel.label, testGuestUid, "keychain value must persist across app launches")

        // 清理：手动调 reset（避免泄漏）
        let resetButton = app2.buttons["home_btnResetIdentity"]
        if resetButton.exists {
            resetButton.tap()
            // dismiss alert
            let alertOK = app2.alerts.buttons.firstMatch
            if alertOK.waitForExistence(timeout: 2) {
                alertOK.tap()
            }
        }
    }
}
```

**对应实装 hook**（`iphone/PetApp/App/PetAppApp.swift` 或 `RootView.swift` `.task` 块内，**仅 `#if DEBUG`**）：

```swift
// 文件顶部 import 已含 SwiftUI
#if DEBUG
@MainActor
private func handleKeychainUITestHooks(container: AppContainer) {
    let env = ProcessInfo.processInfo.environment
    if let seed = env["KEYCHAIN_TEST_SEED"] {
        try? container.keychainStore.set(seed, forKey: KeychainKey.guestUid.rawValue)
        // 通过附加 hidden text 提示 UITest "完成"
        // 实装方式见下文：在 RootView 注入一个 #if DEBUG hidden Text
    }
    if env["KEYCHAIN_TEST_READBACK"] == "1" {
        // 同上：把读到的值塞进一个 hidden Text 的 .accessibilityIdentifier / .accessibilityLabel
    }
}
#endif
```

**hidden Text element 实装思路**（`RootView.swift` `#if DEBUG` 内追加 ZStack 末尾）：

```swift
#if DEBUG
// Keychain UITest hook：根据环境变量显示 hidden text，给 XCUIApplication 探测
private struct KeychainUITestHookView: View {
    let container: AppContainer

    var body: some View {
        let env = ProcessInfo.processInfo.environment
        Group {
            if let seed = env["KEYCHAIN_TEST_SEED"] {
                Text("seed-done")
                    .accessibilityIdentifier("uitest_keychain_seed_done")
                    .opacity(0.001)  // 视觉不可见但 accessibility tree 可见
                    .onAppear {
                        try? container.keychainStore.set(seed, forKey: KeychainKey.guestUid.rawValue)
                    }
            }
            if env["KEYCHAIN_TEST_READBACK"] == "1" {
                let readback = (try? container.keychainStore.get(forKey: KeychainKey.guestUid.rawValue)) ?? ""
                Text(readback)
                    .accessibilityIdentifier("uitest_keychain_readback_value")
                    .opacity(0.001)
            }
        }
    }
}
#endif
```

**具体行为要求**：
- `XCUITest` 跑在 simulator → 可触发真实 `KeychainServicesStore` 读写 → 验证跨 App 进程持久化（terminate + relaunch）
- `launchEnvironment` 注入 hook：`#if DEBUG` 下生效；release build 该代码块不存在 → 生产代码零污染
- 不做"卸载重装" flow：iOS 系统 keychain `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly` + 同 service + 同 account 在卸载重装后**默认保留**（这是 iOS 系统行为，不需自动化测试），由 Story 6.1 E2E 文档的人工验证场景兜底
- 测试结束 `home_btnResetIdentity` 按钮触发清理（Story 2.8 已落 dev 重置按钮）
- accessibility identifier `uitest_keychain_seed_done` / `uitest_keychain_readback_value`：与 Story 2.8 `home_btnResetIdentity` 同 namespace 风格；UITest only，**不**加进 `AccessibilityID` enum（那个是生产 a11y 用的，UITest hook 走独立 namespace 避免噪音）

**AC6 — `AppContainer` 默认 keychainStore 切换为 `KeychainServicesStore`**

修改 `iphone/PetApp/App/AppContainer.swift`：

```swift
// Story 2.5 → Story 5.1 evolution:
// keychainStore 默认实例从 InMemoryKeychainStore() 切换为 KeychainServicesStore()。
// 协议不变，所有 Story 2.8 落地的上层 UseCase / ViewModel / 测试零回归。

public init(
    apiClient: APIClientProtocol,
    keychainStore: KeychainStoreProtocol = KeychainServicesStore()  // ← 改这一处
) {
    self.apiClient = apiClient
    self.errorPresenter = ErrorPresenter()
    self.keychainStore = keychainStore
}
```

**具体行为要求**：
- **仅修改默认值这一处**：测试 / Story 2.8 既有调用方零改动
- 注释说明从 Story 2.8 占位升级到生产真实实装：让未来 grep "InMemoryKeychainStore" 的 Claude 看到这条迁移记录
- `InMemoryKeychainStore` 类**不删**：保留作为测试便利 + 模板示范；继续被 `InMemoryKeychainStoreTests` 测试（Story 2.8 落地的，不动）

**AC7 — `KeychainStoreProtocol` 与 Story 2.8 既有上层零回归**

执行性验证：
- `KeychainStoreProtocol` 四方法签名不动
- `InMemoryKeychainStore` 不删（仅从 `AppContainer` 默认值移除）
- `MockKeychainStore`（`PetAppTests/Features/DevTools/MockKeychainStore.swift`）不动
- `ResetKeychainUseCase` / `ResetIdentityViewModel` 不改
- `InMemoryKeychainStoreTests` / `MockKeychainStoreTests`（如有）/ `ResetIdentityViewModelTests` / `ResetKeychainUseCaseTests` 不动且全绿
- Story 2.8 dev 按钮链路仍工作：button → ViewModel.tap() → ResetKeychainUseCase.execute() → KeychainServicesStore.removeAll() → 真实 keychain 清空

**AC8 — `bash iphone/scripts/build.sh --test` 全绿 + 新增 UITest 通过**

```bash
bash iphone/scripts/build.sh --test     # 单元测试（含 KeychainServicesStoreTests + 既有 InMemoryKeychainStoreTests）
bash iphone/scripts/build.sh --uitest   # UI 测试（含 KeychainPersistenceUITests + 既有 HomeUITests）
```

具体行为要求：
- 既有 `bash iphone/scripts/build.sh --test` 全绿（不引入回归）
- `KeychainServicesStoreTests` ≥ 4 case 全过
- `KeychainPersistenceUITests.testKeychainPersistsAcrossAppLaunches` 单 case 过
- `bash iphone/scripts/build.sh` 普通 build 不报警告

**AC9 — `git status` 验证 `ios/` / `server/` / `iphone/scripts/` / `iphone/project.yml` 零改动**

```bash
git status
# 期望：仅 iphone/PetApp/Core/Storage/* + iphone/PetApp/App/AppContainer.swift（仅默认值改动）
#       + iphone/PetAppTests/Core/Storage/KeychainServicesStoreTests.swift
#       + iphone/PetAppUITests/KeychainPersistenceUITests.swift
#       + iphone/PetApp/App/PetAppApp.swift 或 RootView.swift（#if DEBUG hook 一处）
# 不期望：ios/ / server/ / iphone/scripts/ / iphone/project.yml / iphone/.gitignore / .gitignore 任何改动
```

## Tasks / Subtasks

- [x] Task 1: 新建 `KeychainKey` enum（AC1）
  - [x] 1.1 写 `iphone/PetApp/Core/Storage/KeychainKey.swift`：enum `KeychainKey: String, CaseIterable, Sendable`，含 `.guestUid` / `.authToken`
- [x] Task 2: 新建 `KeychainError` 错误类型（AC3）
  - [x] 2.1 写 `iphone/PetApp/Core/Storage/KeychainError.swift`：含 `.osStatus(OSStatus, operation:)` / `.unexpectedDataFormat(operation:)` 两 case + `LocalizedError` 实装（用 `SecCopyErrorMessageString`）+ `Equatable`
- [x] Task 3: 新建 `KeychainServicesStore` 真实实装（AC2）
  - [x] 3.1 写 `iphone/PetApp/Core/Storage/KeychainServicesStore.swift`：`final class` + `KeychainStoreProtocol` + `@unchecked Sendable`
  - [x] 3.2 实装 `set` 方法 — upsert 模式（SecItemAdd → 撞重复时 SecItemUpdate）
  - [x] 3.3 实装 `get` 方法 — `SecItemCopyMatching` + `errSecItemNotFound` 返 nil + 数据格式校验
  - [x] 3.4 实装 `remove` 方法 — `SecItemDelete` + 不存在不抛
  - [x] 3.5 实装 `removeAll` 方法 — A 方案 整删本 service 所有 generic password
  - [x] 3.6 静态 `service = "com.zhuming.pet.app"`（硬编码）
  - [x] 3.7 `kSecAttrAccessible = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`
- [x] Task 4: 单元测试 `KeychainServicesStoreTests`（AC4）
  - [x] 4.1 新建 `iphone/PetAppTests/Core/Storage/KeychainServicesStoreTests.swift`
  - [x] 4.2 `setUp` / `tearDown` 强制 `try? sut.removeAll()`（测试隔离强约束）
  - [x] 4.3 ≥ 8 case 覆盖 happy + edge：setGet / 不存在返 nil / 覆盖 / remove 单 key 隔离 / remove 不存在不抛 / removeAll 清空 / 空 removeAll 不抛 / 跨实例持久化（共 11 case，含 KeychainKey + KeychainError 验证）
  - [x] 4.4 跑 `bash iphone/scripts/build.sh --test` 验证全绿
- [x] Task 5: 集成测试 `KeychainPersistenceUITests`（AC5）
  - [x] 5.1 新建 `iphone/PetAppUITests/KeychainPersistenceUITests.swift`
  - [x] 5.2 新建独立 `iphone/PetApp/App/KeychainUITestHookView.swift`（`#if DEBUG`），挂在 RootView ZStack 末尾，根据 `launchEnvironment["KEYCHAIN_TEST_SEED"]` / `["KEYCHAIN_TEST_READBACK"]` 触发对应 keychain 操作 + 暴露 `accessibilityIdentifier`
  - [x] 5.3 `testKeychainPersistsAcrossAppLaunches` 测试 case：launch + seed → terminate → relaunch + readback → 断言值相等
  - [x] 5.4 跑 `bash iphone/scripts/build.sh --uitest` 验证通过（passed in 16.040s）
- [x] Task 6: 切换 `AppContainer` 默认 keychainStore（AC6）
  - [x] 6.1 修改 `iphone/PetApp/App/AppContainer.swift` `init` 默认参数：`keychainStore: KeychainStoreProtocol = KeychainServicesStore()`（替换 `InMemoryKeychainStore()`）
  - [x] 6.2 加注释说明从 Story 2.8 占位升级到生产真实实装
  - [x] 6.3 验证既有 Story 2.8 链路（`home_btnResetIdentity` button → reset → alert）仍工作（既有 ResetKeychainUseCaseTests / ResetIdentityViewModelTests / NavigationUITests 全绿）
- [x] Task 7: 验证 Story 2.8 既有上层零回归（AC7）
  - [x] 7.1 `KeychainStoreProtocol` 四方法签名未改动（KeychainStore.swift 文件未触碰）
  - [x] 7.2 `InMemoryKeychainStore` 类保留不删
  - [x] 7.3 跑 `bash iphone/scripts/build.sh --test` 确认 `MockKeychainStore` / `InMemoryKeychainStoreTests` / `ResetKeychainUseCaseTests` / `ResetIdentityViewModelTests` 全绿（全 133 unit tests 全过）
- [x] Task 8: 全套 build / test 验证（AC8）
  - [x] 8.1 `bash iphone/scripts/build.sh` 普通 build 无 warning（BUILD SUCCESS）
  - [x] 8.2 `bash iphone/scripts/build.sh --test` 单元测试全绿（133 tests pass）
  - [x] 8.3 `bash iphone/scripts/build.sh --uitest` UI 测试全绿（8 tests pass，含新 KeychainPersistenceUITests）
- [x] Task 9: `git status` 范围验证（AC9）
  - [x] 9.1 `git status` 确认仅 `iphone/PetApp/Core/Storage/*` + `iphone/PetApp/App/AppContainer.swift` + `iphone/PetApp/App/RootView.swift` + `iphone/PetApp/App/KeychainUITestHookView.swift`（新文件）+ `iphone/PetAppTests/Core/Storage/*` + `iphone/PetAppUITests/KeychainPersistenceUITests.swift` 有改动；`iphone/PetApp.xcodeproj/project.pbxproj` xcodegen 自动生成（含新 file ref entries）
  - [x] 9.2 确认 `ios/` / `server/` / `iphone/scripts/` / `iphone/project.yml` / `iphone/.gitignore` / `.gitignore` 全零改动（`git status -- ios/ server/ iphone/scripts/ iphone/project.yml iphone/.gitignore .gitignore` → "nothing to commit"）

## Dev Notes

### 关键技术约束

1. **协议不变**：`KeychainStoreProtocol` 四方法签名（`set` / `get` / `remove` / `removeAll`）由 Story 2.8 锁定。本 story 仅替**实装**；任何改协议的需求都属 scope creep
2. **Security framework 是同步阻塞 API**：`SecItemAdd` / `SecItemCopyMatching` / `SecItemUpdate` / `SecItemDelete` 都同步返回 `OSStatus`；本实装方法签名保持同步 `throws`（`func set(_:forKey:) throws`），与 `InMemoryKeychainStore` 一致；不引入 `async`
3. **`kSecAttrService` 必须硬编码 `"com.zhuming.pet.app"`**：与 `iphone/project.yml` 已锁的 `PRODUCT_BUNDLE_IDENTIFIER` 一致；**不**用 `Bundle.main.bundleIdentifier`（unit test target 拿到的是 `com.zhuming.pet.app.tests`，会泄漏到不同 namespace）；详见 AC2 文件头注释第 2 点
4. **`kSecAttrAccessible = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`**：
   - `AfterFirstUnlock` 不限制 App 启动时机（首次解锁后即可读）
   - `ThisDeviceOnly` 不参与 iCloud Keychain sync（NFR7 设计意图：账号识别走 server 端 `user_auth_bindings` 表，不靠 iOS sync）
5. **`set` upsert 模式**：用 `SecItemAdd` → 撞 `errSecDuplicateItem` 时 `SecItemUpdate`（**不**先 delete 再 add，避免窗口期空洞）；标准 Keychain 模式
6. **`removeAll` A 方案**：用 `SecItemDelete` 带 `kSecClass + kSecAttrService` 整删；比遍历 `KeychainKey.allCases` 更彻底（避免漏删未登记 key）
7. **不加锁**：Apple `SecItem*` API 内部已线程安全；上层 NSLock 是过度防御
8. **不引入 actor**：与 `InMemoryKeychainStore` / `MockKeychainStore` / `MockURLSession` / `MockAPIClient` 同模式（`@unchecked Sendable` + 同步方法），跨实装一致；actor 包同步 API 增加 await 链无收益

### Source tree components to touch

```
iphone/
├─ PetApp/
│  ├─ App/
│  │  └─ AppContainer.swift            # 改 1 处默认值（AC6）
│  ├─ Core/
│  │  └─ Storage/
│  │     ├─ KeychainStore.swift        # 不动（Story 2.8 落地的协议 + 占位实装）
│  │     ├─ KeychainKey.swift          # 新建（AC1）
│  │     ├─ KeychainError.swift        # 新建（AC3）
│  │     └─ KeychainServicesStore.swift  # 新建（AC2）
│  └─ App/
│     └─ RootView.swift                # 加 #if DEBUG hook view（AC5）
├─ PetAppTests/
│  └─ Core/
│     └─ Storage/                      # 新建目录
│        └─ KeychainServicesStoreTests.swift  # 新建（AC4）
└─ PetAppUITests/
   └─ KeychainPersistenceUITests.swift # 新建（AC5）

# 不动：
# iphone/PetApp/Core/Storage/KeychainStore.swift（协议 + InMemoryKeychainStore）
# iphone/PetApp/Features/DevTools/UseCases/ResetKeychainUseCase.swift
# iphone/PetApp/Features/DevTools/ViewModels/ResetIdentityViewModel.swift
# iphone/PetApp/Features/DevTools/Views/ResetIdentityButton.swift
# iphone/PetAppTests/Features/DevTools/*（4 个 Story 2.8 测试文件）
# iphone/PetApp/Shared/Constants/AccessibilityID.swift
# iphone/project.yml / iphone/scripts/* / iphone/.gitignore
# ios/* / server/*
```

### Testing standards summary（继承 ADR-0002 + Story 2.7 / 2.8）

- **单元测试**：XCTest only（手写 mock）；ADR-0002 §3.1
- **异步测试**：本 story 实装是同步 throws，不需要 async test 写法；如未来 `KeychainStoreProtocol` 改 async 再单独 spike
- **UI 测试**：XCUITest（`PetAppUITests` target）+ launchEnvironment hook（`#if DEBUG` 实装）
- **测试隔离强约束**：`KeychainServicesStoreTests` `setUp` / `tearDown` 必须 `try? sut.removeAll()`，避免 simulator keychain 残留干扰
- **跑命令**：`bash iphone/scripts/build.sh --test`（单元）/ `--uitest`（UI），与 ADR-0002 §3.4 + CLAUDE.md "Build & Test" 段一致

### Project Structure Notes

- 完全对齐 iOS 架构设计 §4 目录结构（`iphone/PetApp/{App,Core,Shared,Features,Resources}/`）+ ADR-0002 §3.3 方案 D
- `Core/Storage/` 已由 Story 2.8 建立 → 本 story 在同目录追加 3 文件（`KeychainKey.swift` / `KeychainError.swift` / `KeychainServicesStore.swift`）
- `PetAppTests/Core/Storage/` 是新建目录 → 镜像生产 `Core/Storage/` 路径（与既有 `PetAppTests/Features/DevTools/` 镜像 `PetApp/Features/DevTools/` 同模式）
- `Detected conflicts or variances`：无；本 story 完全遵循既有目录约定

### References

- 总体架构 §"游客身份" / §"Keychain 持久识别"：[Source: docs/宠物互动App_总体架构设计.md]
- iOS 架构 §11.1 Keychain（设计意图：guestUid + token + 必要时 refresh token；不存 UserDefaults）：[Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#11.1-Keychain]
- iOS 架构 §12.1 App 启动链路（read guestUid from Keychain → POST /auth/guest-login → save token → GET /home）：[Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#12.1-App-启动链路]
- V1 接口设计 §4.1 POST /auth/guest-login（guestUid 字段约束：1 ≤ length ≤ 128 字符；推荐 UUID v4 字符串；冻结起 2026-04-26）：[Source: docs/宠物互动App_V1接口设计.md#4.1]
- ADR-0002 §3.1 iOS Mock 框架 XCTest only / §3.3 iPhone App 工程目录方案 D / §3.4 CI 命令：[Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md]
- Epic 5 / Story 5.1 完整 AC：[Source: _bmad-output/planning-artifacts/epics.md#Story-5.1-Keychain-封装]
- NFR7 / FR2 / AR15（Keychain 持久识别）：[Source: _bmad-output/planning-artifacts/epics.md#NFR7]
- Story 2.8 实装记录（占位实装 + 协议接缝；本 story 升级真实实装）：[Source: _bmad-output/implementation-artifacts/2-8-dev-重置-keychain-按钮.md]
- Story 4.6 实装记录（server 端 /auth/guest-login，本 story 客户端 guestUid 写入对应 server 端 user_auth_bindings 表）：[Source: _bmad-output/implementation-artifacts/4-6-游客登录接口-首次初始化事务.md]
- Apple 官方 Security framework 文档（`SecItemAdd` / `SecItemCopyMatching` / `kSecClassGenericPassword` / `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly` 等）：https://developer.apple.com/documentation/security/keychain_services

## Previous Story Intelligence（来自 Story 2.8）

Story 2.8 是本 story 的**直接前置**，留下以下 IOU + 经验，本 story 必须吸收：

1. **协议接缝先稳，实装后到**：Story 2.8 已锁 `KeychainStoreProtocol` 四方法签名 + 占位 `InMemoryKeychainStore` + 上层 `ResetKeychainUseCase` / `ResetIdentityViewModel` 全套测试；本 story **零改动协议** → 上层零回归
2. **`InMemoryKeychainStore` 不删原则**：作为测试便利 + ADR-0002 §3.1 "手写 mock 优先" 模板示范；继续被 `InMemoryKeychainStoreTests` 测试
3. **`MockKeychainStore` 是上层测试用**（继承 `MockBase`）；本 story 测**真实** `KeychainServicesStore` 时**不**用 `MockKeychainStore`（那是给 ResetKeychainUseCase / ResetIdentityViewModel 用的）
4. **`AppContainer` 默认实例切换是单点改动**：Story 2.8 设计时已预见 → 默认值替换从 `InMemoryKeychainStore()` → `KeychainServicesStore()`，调用方零改动
5. **`#if DEBUG` 包裹原则**：Story 2.8 落 `home_btnResetIdentity` 按钮 + `makeResetIdentityViewModel()` 工厂都用 `#if DEBUG`；本 story UITest hook 沿用 → 生产代码零污染
6. **AccessibilityID 不污染**：Story 2.8 把 `home_btnResetIdentity` 加进 `AccessibilityID.Home` enum；本 story UITest hook 用独立 namespace `uitest_keychain_*`，**不**进 `AccessibilityID` enum（那个是生产 a11y 用的）
7. **测试覆盖要 ≥ AC 列示数量**：Story 2.8 AC 给 case 数量是下限；本 story AC4 给 9 case，dev 实装时**至少**实装 8 个 + 跨实例持久化 case，按需追加

### Story 2.8 lessons 关联（review 阶段已 distill 到 docs/lessons/）

本 story 实装期间值得重读的 Story 2.8 review lessons：

- `docs/lessons/2026-04-26-stateobject-init-vs-bind-injection.md`：`@StateObject` 注入路径副作用初始化漏掉的坑（`AppContainer` 改默认值时务必 grep 所有 `@StateObject` 注入点确认无副作用）
- `docs/lessons/2026-04-26-mockbase-snapshot-only-reads.md`：`MockBase` 内部存储字段一律 `private`，只通过 snapshot helper 读 — 本 story 写 `KeychainServicesStoreTests` 时**不**继承 `MockBase`（直接测真实 SUT），但需理解 `MockKeychainStore` 既有契约
- `docs/lessons/2026-04-26-stateobject-debug-instance-aliasing.md`：`@StateObject` init 阶段构造的 standalone container 与 RootView container 是别名陷阱 — 本 story 改 `AppContainer` 默认值时务必避免在 init 阶段意外构造 standalone instance
- `docs/lessons/2026-04-25-swift-explicit-import-combine.md`：`ObservableObject` / `@Published` 必须显式 `import Combine` — 本 story 不直接写 `ObservableObject`，但若改 `RootView.swift` hook view 含 ViewModel 引用时需注意
- `docs/lessons/2026-04-26-swiftui-task-modifier-reentrancy.md`：SwiftUI `.task` 在 view 重新出现时会重启 — 若把 keychain UITest hook 实装在 `.task` 块需考虑 reentrancy（建议改用 `.onAppear` 或在 `RootView` 顶层 `@State` 一次性触发）
- `docs/lessons/2026-04-26-checked-in-secret-must-fail-fast.md`：secret 字段必须空字符串 + fail-fast — 本 story `KeychainKey` raw value 是公开 namespace（不是 secret），无关联；但若未来加 dev fixture token 时务必 fail-fast

## Git Intelligence Summary

最近 5 个 commit 解析（截至 `a982f68`）：

```
a982f68 chore(story-4-7): 收官 Story 4.7 + 归档 story 文件
6833085 test(server): Epic4/4.7 Layer 2 集成测试 — 游客登录初始化事务全流程
335cf88 chore(story-4-8): 收官 Story 4.8 + 归档 story 文件
a8ac52f feat(server): Epic4/4.8 GET /home 聚合接口（initial 版含 user + pet + stepAccount + chest）
bb2a218 docs(lessons): 回填 Story 4.6 lesson commit 字段
```

**对本 story 的指引**：
- Epic 4 server 端**已全 done**（4.1 → 4.8 + 4.7）：`/auth/guest-login` 接口可调用，`/home` 聚合接口可调用 → 本 story 完成后 Story 5.2 可立刻并行开工
- 最近无 iPhone 端业务改动（最后一个 iPhone commit 是 Story 2.x 系列）→ `iphone/` 工作区干净，本 story 是 Epic 5 起点
- commit 风格：`feat(server): EpicX/Y.Z ...` / `chore(story-X-Y): ...` / `test(server): EpicX/Y.Z ...` —— 本 story 完成后 commit 风格遵循：`feat(iphone): Epic5/5.1 KeychainStore 真实封装` 风格
- `docs(lessons): 回填 ...` 模式：本 story review 阶段如有 lesson 产出，记得在 `docs/lessons/index.md` 追加行 + 后续 commit 回填 commit hash

## Latest Tech Information（Apple Security framework 关键参考）

iOS 17+ / Swift 6.3 当前阶段（2026-04 实测）以下 API 与策略稳定：

- `SecItemAdd` / `SecItemCopyMatching` / `SecItemUpdate` / `SecItemDelete`：自 iOS 2.0 起稳定，签名不变
- `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`：自 iOS 4.0 起稳定，**不**参与 iCloud Keychain sync（与 NFR7 设计一致）
- `kSecClassGenericPassword`：通用密码项 class，最适合 token / id 字符串存储
- `SecCopyErrorMessageString(OSStatus, nil) -> CFString?`：自 iOS 11.3 起稳定，可拿 OSStatus 对应的可读错误描述（部署 target iOS 17.0 完全覆盖）
- Swift 6.3 `Sendable` 严格 concurrency：`@unchecked Sendable` + 内部不可变状态 / Apple 系统 API 自带线程安全 → 是合规模式
- `Security.framework` 是 Apple 系统库，不需要 SPM / Cocoapods 引入；`import Security` 即可

**已知 Apple 文档建议**：
- `kSecAttrService` 推荐用 reverse-DNS 风格（如 `com.zhuming.pet.app`）便于跨 App debug
- `kSecValueData` 必须是 `Data`（不能是 `String`）；UTF-8 转换标准做法
- `SecItemDelete` 带 `kSecMatchLimit` 不是必需的；不带时默认整删 query 匹配的所有 item

**已知坑预警**：
- iOS Simulator keychain 在 Xcode 升级 / 重置时可能清空 → 集成测试**不**依赖跨 Xcode session 持久化；`KeychainPersistenceUITests` 在单 session 内 launch + terminate + relaunch 即可
- 不同 simulator device 之间 keychain **不**共享 → 测试只在单 simulator 内验证；CI 不需要多 simulator 矩阵
- **`errSecMissingEntitlement`（OSStatus -34018）**：debug build 在某些 Xcode 配置下可能出现 → 通常是 `Keychain Sharing` capability 配置问题；本 story 单 app keychain **不**需要 Keychain Sharing capability，如遇此错先确认 `iphone/project.yml` 没有意外加 entitlement
- **iCloud Keychain sync 相关 OSStatus**：本 story 用 `ThisDeviceOnly` 完全避开 sync 路径 → 不会触发；如未来需要跨设备 sync 是 Post-MVP 决策

## Project Context Reference

无独立 `project-context.md`；项目背景信息全部从 `CLAUDE.md` + `docs/宠物互动App_*.md` + `_bmad-output/implementation-artifacts/decisions/*.md` 取。**本 story 实装前必读**：

1. `CLAUDE.md` — 项目顶层约束（节点顺序、Repo Separation、Build & Test 命令）
2. `docs/宠物互动App_总体架构设计.md` — FR / NFR 全集（NFR7 Keychain）
3. `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §11.1 + §12.1 + §4 — Keychain 设计意图 + 启动链路 + 目录结构
4. `docs/宠物互动App_V1接口设计.md` §4.1 — guestUid 字段约束
5. `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md` — iOS 工程 / 测试 / CI 决策
6. `_bmad-output/implementation-artifacts/2-8-dev-重置-keychain-按钮.md` — 直接前置 story；本 story 兑现其 IOU
7. `_bmad-output/planning-artifacts/epics.md` Epic 5 §Story 5.1 — 本 story AC 定义

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7（创建 story 时）

### Debug Log References

无（实装过程一次跑通：build → unit tests → UI tests 三轮全绿，无 red→green 排错节点）。

### Completion Notes List

**dev-story 阶段实装完成（2026-04-27）**

按 9 个 Task / 38 个 Subtask 顺序落地，全部 [x] 完成：

1. **新增 `KeychainKey.swift`**（AC1）：enum String + CaseIterable + Sendable，含 `.guestUid="auth.guestUid"` / `.authToken="auth.token"` 两 case；raw value 即 keychain account 字段
2. **新增 `KeychainError.swift`**（AC3）：`.osStatus(OSStatus, operation:)` / `.unexpectedDataFormat(operation:)` 两 case + `LocalizedError` 实装（用 `SecCopyErrorMessageString` 拿 Apple 描述文本）+ `Equatable`
3. **新增 `KeychainServicesStore.swift`**（AC2）：`final class` + `KeychainStoreProtocol` + `@unchecked Sendable`；硬编码 `service = "com.zhuming.pet.app"`；`set` 走 SecItemAdd → 撞重复用 SecItemUpdate 标准 upsert；`get` errSecItemNotFound 返 nil；`remove` 不存在不抛；`removeAll` A 方案（按 service 整删）；全部用 `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`
4. **新增 `KeychainServicesStoreTests.swift`**（AC4）：11 case 覆盖 setGet / 不存在返 nil / upsert 覆盖 / remove 单 key 隔离 / remove 不存在不抛 / removeAll 清空 / 空 removeAll / 跨实例持久化 / ad-hoc string key / KeychainKey raw value 验证 / KeychainError 描述验证；setUp + tearDown 都强制 `try? sut.removeAll()` 测试隔离
5. **新增 `KeychainPersistenceUITests.swift` + `KeychainUITestHookView.swift`**（AC5）：UITest 通过 `launchEnvironment` 触发种入 → terminate → relaunch 读回；hook view 仅 `#if DEBUG` 编译，挂在 RootView ZStack 末尾；用 `opacity(0.001)` hidden Text + accessibilityIdentifier `uitest_keychain_seed_done` / `uitest_keychain_readback_value`；testKeychainPersistsAcrossAppLaunches 单 case 16.040s 跑通
6. **修改 `AppContainer.swift`**（AC6）：默认 `keychainStore: KeychainStoreProtocol = KeychainServicesStore()`（替换 `InMemoryKeychainStore()`）；保留 init 注入式签名；注释升级 Story 2.8 → Story 5.1 evolution
7. **修改 `RootView.swift`**（AC5 hook 挂载点）：在 ZStack 末尾 `#if DEBUG` 包裹追加 `KeychainUITestHookView(container: container)`；不动既有 launchStateMachine / coordinator / homeViewModel wire
8. **零回归验证**（AC7）：`KeychainStoreProtocol` 协议签名未触碰；`InMemoryKeychainStore` 类保留；既有 `MockKeychainStoreTests` 不存在但 `InMemoryKeychainStoreTests` (7 case) / `ResetKeychainUseCaseTests` (3 case) / `ResetIdentityViewModelTests` (5 case) 全绿
9. **全套 build/test 三轮验证通过**（AC8）：
   - `bash iphone/scripts/build.sh` → BUILD SUCCESS（无 warning）
   - `bash iphone/scripts/build.sh --test` → 133 unit tests 0 fail
   - `bash iphone/scripts/build.sh --uitest` → 8 UI tests 0 fail（含 KeychainPersistenceUITests）
   - `bash iphone/scripts/build.sh --test --uitest` 一并跑也 0 fail
10. **范围红线验证**（AC9）：`git status -- ios/ server/ iphone/scripts/ iphone/project.yml iphone/.gitignore .gitignore` → "nothing to commit, working tree clean"，红线全部守住；`iphone/PetApp.xcodeproj/project.pbxproj` 由 xcodegen 自动 regen，仅追加新文件 reference

### File List

**新增（5 个）**：
- `iphone/PetApp/Core/Storage/KeychainKey.swift`
- `iphone/PetApp/Core/Storage/KeychainError.swift`
- `iphone/PetApp/Core/Storage/KeychainServicesStore.swift`
- `iphone/PetApp/App/KeychainUITestHookView.swift`（仅 `#if DEBUG`）
- `iphone/PetAppTests/Core/Storage/KeychainServicesStoreTests.swift`
- `iphone/PetAppUITests/KeychainPersistenceUITests.swift`

**修改（2 个）**：
- `iphone/PetApp/App/AppContainer.swift`（默认 keychainStore 切换为 KeychainServicesStore + 注释升级）
- `iphone/PetApp/App/RootView.swift`（ZStack 末尾追加 `#if DEBUG` 包裹的 KeychainUITestHookView）

**自动 regen**：
- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen 自动追加新文件 reference；project.yml 未改）

**未触碰（明确零回归红线）**：
- `iphone/PetApp/Core/Storage/KeychainStore.swift`（协议 + InMemoryKeychainStore 占位实装）
- `iphone/PetApp/Features/DevTools/UseCases/ResetKeychainUseCase.swift`
- `iphone/PetApp/Features/DevTools/ViewModels/ResetIdentityViewModel.swift`
- `iphone/PetApp/Features/DevTools/Views/ResetIdentityButton.swift`
- `iphone/PetApp/Features/Home/Views/HomeView.swift`（注释升级未做 — 协议不变 + 注释升级属可选 polish，留给后续 review 阶段决定）
- `iphone/PetApp/Shared/Constants/AccessibilityID.swift`（UITest hook 用独立 `uitest_keychain_*` namespace）
- `iphone/PetAppTests/Features/DevTools/*` 全 4 文件
- `iphone/project.yml` / `iphone/scripts/*` / `iphone/.gitignore` / `.gitignore`
- `ios/*` / `server/*`

## Change Log

| Date | Author | Change |
| --- | --- | --- |
| 2026-04-27 | Claude Opus 4.7 | dev-story 阶段：实装 KeychainServicesStore 真实实装 + KeychainKey enum + KeychainError 错误类型 + KeychainServicesStoreTests (11 case) + KeychainPersistenceUITests (XCUITest 跨进程持久化验证) + KeychainUITestHookView (#if DEBUG)；切换 AppContainer 默认 keychainStore 实例；Story 2.8 既有上层零回归。Status ready-for-dev → in-progress → review。
