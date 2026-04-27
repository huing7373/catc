// MockSilentReloginUseCase.swift
// Story 5.4 AC8: SilentReloginUseCaseProtocol 测试 mock；继承 MockBase（Story 2.7 落地）.
//
// stub 字段（executeStub / artificialDelayMs）由测试 setUp 阶段写入；method body 读取一次后立即用，
// 符合 MockBase snapshot-only 精神（lesson 2026-04-26-mockbase-snapshot-only-reads.md）.
//
// `artificialDelayMs` 是 SilentReloginCoordinatorTests case#2 (5 并发 coalesce) 必需 ——
// 故意让 execute 慢一点，让多并发都进入"等待既存 task" 路径，避免 race window 被错过.

@testable import PetApp
import Foundation

#if DEBUG

final class MockSilentReloginUseCase: MockBase, SilentReloginUseCaseProtocol, @unchecked Sendable {
    var executeStub: Result<String, Error> = .failure(MockError.notStubbed)

    /// 人工延迟（毫秒）。0 = 不延迟（立即返回）。
    /// 用途：SilentReloginCoordinatorTests case#2 (5 并发) 必须延迟，否则测试可能侥幸通过 race window 漏检.
    var artificialDelayMs: UInt64 = 0

    func execute() async throws -> String {
        record(method: "execute()")
        if artificialDelayMs > 0 {
            try? await Task.sleep(nanoseconds: artificialDelayMs * 1_000_000)
        }
        return try executeStub.get()
    }
}

#endif
