// PingResult.swift
// Story 2.5 AC1：PingUseCase 的输出 value type。
//
// 三态语义（按 story Dev Note #2 决策表）：
// - (reachable: true,  serverCommit: "abc1234") ping 成功 + version 成功 → 显示 "Server abc1234"
// - (reachable: true,  serverCommit: nil)       ping 成功 + version 失败 → 显示 "Server v?"（部分降级）
// - (reachable: false, serverCommit: nil)       ping 失败                → 显示 "Server offline"
//
// 不引入第四种 (reachable: false, serverCommit: 非空) 的状态：
// ping 失败时已经认为整个 server 不可达，即使理论上 version 调用先成功后被 ping 推翻，
// UI 上"server offline"语义优先级更高，commit 值无意义。

import Foundation

/// PingUseCase 的输出。
public struct PingResult: Equatable {
    /// 整体可达性：ping 成功 = true；ping 失败 = false。
    public let reachable: Bool

    /// version 接口返回的 commit 短哈希；version 失败 / ping 失败时为 nil。
    public let serverCommit: String?

    public init(reachable: Bool, serverCommit: String?) {
        self.reachable = reachable
        self.serverCommit = serverCommit
    }
}
