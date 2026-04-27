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
