// FakeIdempotencyKeyGenerator.swift
// Story 21.3 AC9 测试 helper：注入可控 idempotency key 序列让 OpenChestUseCase 单测断言
// "同一 execute 复用同 key" + "连续两次 execute 各拿不同 key" 行为.
//
// 设计（与 MockBase / MockChestRepository 同模式）:
//   - keys 数组按序消费；用完后 wrap-around（防 test 内多次 execute 越界）
//   - callCount 让 test 断言 generate 被调几次
//   - @unchecked Sendable（mutable index 字段；测试串行）.

import Foundation
@testable import PetApp

final class FakeIdempotencyKeyGenerator: IdempotencyKeyGenerator, @unchecked Sendable {
    /// 预设 key 序列；index 从 0 开始按调用顺序消费.
    var keys: [String] = ["test-key-1", "test-key-2", "test-key-3"]

    /// 调用计数（让测试断言 generate() 被调几次）.
    private(set) var callCount: Int = 0

    /// 内部下标 cursor（与 callCount 同步推进，但 callCount 是外部可见的累计计数器）.
    private var index: Int = 0

    func generate() -> String {
        let key = keys[index % keys.count]
        index += 1
        callCount += 1
        return key
    }
}
