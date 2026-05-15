// IdempotencyKeyGenerator.swift
// Story 21.3 AC2: 幂等键生成协议（V1 §7.2 钦定 1-128 字符长度 + [A-Za-z0-9_:-] 字符集）.
//
// 默认实装用 UUID v4 字面量（Foundation `UUID().uuidString`，形如 "E621E1F8-C36C-495A-93FC-0C247A3E6E5F"
// 36 字符长度落入 1-128 区间 + 全字符 [A-F0-9-] 满足 [A-Za-z0-9_:-] 字符集，无需额外清洗）.
//
// 拆协议的理由（与 DateProvider 同精神）:
//   - 单测可注入固定 key 验证"同一次 use case 调用复用同 key"；
//   - 未来若需切换到 nanoid / 时间戳前缀格式（如 "chest_open_{userId}_{nanoTimestamp}"），改默认 impl 不动 UseCase；
//   - 与 V1 §7.2 字段表行 940 "client 应在每次点击开箱按钮时生成新的 key" 钦定一致.
//
// 放 Core/Networking/ 而非 Features/Home/UseCases/（关键决策 3）:
//   - 与 APIClient / Endpoint / APIError 同 module（"网络层基础设施"）；
//   - 未来 Story 32.4 POST /api/v1/compose/upgrade 也复用 idempotencyKey（V1 §7.2 r11 跨接口影响段钦定）
//     —— 放 Core 让 Home / Compose 等多 feature 共享.

import Foundation

public protocol IdempotencyKeyGenerator: Sendable {
    /// 生成一个新的 idempotency key.
    /// 实装契约：每次调用必须返回**不同**的字符串 + 满足 V1 §7.2 字符集 [A-Za-z0-9_:-] + 长度 1-128.
    func generate() -> String
}

public struct DefaultIdempotencyKeyGenerator: IdempotencyKeyGenerator {
    public init() {}

    public func generate() -> String {
        // UUID v4: 36 字符（含 4 个连字符）; 字符集 [A-F0-9-] ⊂ [A-Za-z0-9_:-]; 满足 V1 §7.2 行 940 约束.
        return UUID().uuidString
    }
}
