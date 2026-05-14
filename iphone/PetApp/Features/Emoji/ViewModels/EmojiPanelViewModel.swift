// EmojiPanelViewModel.swift
// Story 18.1 AC3: EmojiPanelView 状态机 (loading / loaded / failed + retry).
//
// 设计:
//   - `@MainActor` —— 所有 state 更新都在 MainActor 上 (SwiftUI @Published 要求).
//   - `ObservableObject` + `@Published var state` —— SwiftUI view 通过 @StateObject 订阅.
//   - 初始 `state = .loading` —— EmojiPanelView 启动 `.task` 调 `load()` 切实际状态.
//
// 状态机:
//   - `.loading` —— 加载中，view 显示 ProgressView
//   - `.loaded([EmojiConfig])` —— 加载成功，view 显示 LazyVGrid
//   - `.failed(String)` —— 加载失败，view 显示 RetryView；payload 是 user-facing 错误文案
//     (已经过 mapError 转换，view 层直接显示)
//
// `mapError` (与 ErrorPresenter 同精神，但表情面板局部 RetryView 不走全局 toast/alert):
//   - APIError.network → "网络异常，请检查后重试"
//   - APIError.business(1009) → "服务器繁忙，请稍后再试"
//   - APIError.business(1001) / .unauthorized / .missingCredentials → "登录已失效，请重启 App" (terminal 类)
//     (理论 ADR-0008 v2 装饰器已拦截 401，但兜底)
//   - APIError.decoding → "数据解析失败，请重试"
//   - APIError.localStoreFailure → "登录信息读取异常，请重试" (**transient** retry，与 AppErrorMapper §line 90-93 对齐；
//     APIError.swift §.localStoreFailure 钦定 transient: keychain 抛错 retry 可能自愈，**不**与 .missingCredentials 合并)
//   - 其他 (含 business 其他 code) → "加载失败，请重试"
//
// `retry()` 路径：等价 await load() (语义清晰：retry = 重试加载).
//
// import 显式：Foundation (Error / async / throws) + Combine (ObservableObject / @Published)
// —— lesson 2026-04-25-swift-explicit-import-combine.md 钦定.

import Foundation
import Combine

/// EmojiPanelView 三态枚举 + Equatable (单测断言 `XCTAssertEqual(vm.state, .loaded(...))` 直接比对).
public enum EmojiPanelState: Equatable {
    case loading
    case loaded([EmojiConfig])
    case failed(String)
}

@MainActor
public final class EmojiPanelViewModel: ObservableObject {
    @Published public private(set) var state: EmojiPanelState = .loading

    private let useCase: LoadEmojisUseCaseProtocol

    public init(useCase: LoadEmojisUseCaseProtocol) {
        self.useCase = useCase
    }

    /// 加载表情列表 (view `.task` 启动时调).
    /// 路径：state = .loading → useCase.execute() → 成功 .loaded / 失败 .failed.
    public func load() async {
        state = .loading
        do {
            let emojis = try await useCase.execute()
            state = .loaded(emojis)
        } catch {
            state = .failed(mapError(error))
        }
    }

    /// 用户点 RetryView "重试" 按钮触发 —— 重新走 load() 路径.
    public func retry() async {
        await load()
    }

    /// APIError → user-facing 文案 mapper (transient 类走 retry-able 文案；terminal 类走"请重启 App").
    /// 详见 docs/lessons/2026-04-27-transient-vs-terminal-error-classification.md.
    private func mapError(_ error: Error) -> String {
        guard let apiError = error as? APIError else {
            return "加载失败，请重试"
        }
        switch apiError {
        case .network:
            return "网络异常，请检查后重试"
        case .business(let code, _, _):
            switch code {
            case 1009:
                return "服务器繁忙，请稍后再试"
            case 1001:
                return "登录已失效，请重启 App"
            default:
                return "加载失败，请重试"
            }
        case .unauthorized:
            return "登录已失效，请重启 App"
        case .decoding:
            return "数据解析失败，请重试"
        case .missingCredentials:
            // terminal: 本地 keychain 确认无 token，重启 App cold-start 走同一份 store 仍读不到，
            // retry 无意义 —— 与 AppErrorMapper §line 85-88 ".alert(登录信息丢失)" 同语义.
            return "登录已失效，请重启 App"
        case .localStoreFailure:
            // **transient**: keychain.get 抛错 (sandbox 抽风 / OSStatus -25291 等)，retry 可能自愈.
            // 与 AppErrorMapper §line 90-93 ".retry(登录信息读取异常)" 同语义 —— 不与 .missingCredentials 合并.
            // 依据：APIError.swift §.localStoreFailure 钦定 transient + AppErrorMapper 分支已成定例.
            return "登录信息读取异常，请重试"
        }
    }
}
