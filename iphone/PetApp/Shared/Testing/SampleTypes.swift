// SampleTypes.swift
// Story 2.7 测试基础设施模板：SampleUseCase + SampleViewModel placeholder type。
//
// 存在目的：让 PetAppTests/Helpers/SampleViewModelTests.swift（AC5）有真正的被测对象，
// 即"业务相关 mock 单元测试"模板示范（满足 epics.md Story 2.7 AC "至少存在一条业务相关 mock 单元测试"）。
//
// 不是真业务代码：
// - 不导出给真业务 Feature 使用
// - 用 #if DEBUG 包裹，Release build 自动 strip
// - 命名以 `Sample` 前缀避免与未来真业务命名冲突
//
// 设计参考：
// - server 端 `internal/service/sample/service.go`（同样是测试基础设施模板，service 层 placeholder）
// - 后续业务 ViewModel（HomeViewModel / RoomViewModel / ChestViewModel 等）按本模板结构填业务
//
// lesson 2026-04-25-swift-explicit-import-combine.md：production 文件首次使用
// ObservableObject / @Published 必须显式 `import Combine`，否则 Swift 6 严格模式下会编译失败

#if DEBUG

import Combine
import Foundation

/// 演示性 UseCase 协议：异步 throws 单方法。
public protocol SampleUseCase: Sendable {
    func execute(input: String) async throws -> Int
}

/// 演示性 ViewModel：通过 SampleUseCase 取数据，driver 状态机切换。
@MainActor
public final class SampleViewModel: ObservableObject {
    public enum Status: Equatable {
        case idle
        case loading
        case ready(value: Int)
        case failed(message: String)
    }

    @Published public private(set) var status: Status = .idle

    private let useCase: SampleUseCase

    public init(useCase: SampleUseCase) {
        self.useCase = useCase
    }

    /// 触发一次 useCase 调用，driver `status` 走 `.idle → .loading → .ready/.failed`。
    public func load(input: String) async {
        status = .loading
        do {
            let value = try await useCase.execute(input: input)
            status = .ready(value: value)
        } catch {
            status = .failed(message: "\(error)")
        }
    }
}

#endif
