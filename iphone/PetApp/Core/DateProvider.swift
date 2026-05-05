// DateProvider.swift
// Story 8.5 AC6: 时间抽象（让单测可锁固定时间，验 syncDate / clientTimestamp）.
//
// 用途：SyncStepsUseCase 拼请求体时需要:
//   - syncDate: 本机时区当日 YYYY-MM-DD（不能直接用 Date() 派生 — 单测无法验确定值）
//   - clientTimestamp: 当前毫秒时间戳（同上）
//   - now(): healthProvider.readDailyTotalSteps(date:) 入参
//
// 与 8.1 / 8.2 / 8.3 不同（那几个 story 的 system adapter 不需要时间抽象 —— 单测注入步数 /
// activity 序列即可）.本 story 单测必须覆盖"跨自然日"等时间敏感场景；DateProvider 是必备.

import Foundation

public protocol DateProvider: Sendable {
    /// 当前时间（healthProvider.readDailyTotalSteps 入参）.
    func now() -> Date
    /// 当前毫秒时间戳（V1 §6.1.4 clientTimestamp 字段；Int64 = Unix ms）.
    func nowMillis() -> Int64
    /// 本机时区当日 "YYYY-MM-DD"（V1 §6.1.1 syncDate 字段；严格 10 字符）.
    func todayString() -> String
}

public struct DefaultDateProvider: DateProvider {
    public init() {}

    public func now() -> Date { Date() }

    public func nowMillis() -> Int64 {
        Int64(Date().timeIntervalSince1970 * 1000)
    }

    public func todayString() -> String {
        // 本机时区当日 YYYY-MM-DD（与 server 钦定 [today-2d, today+2d] 容忍窗口对齐；
        // 不二次转 UTC，避免跨时区漂移；详见 V1 §6.1.1 + GAP E）.
        let formatter = DateFormatter()
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.dateFormat = "yyyy-MM-dd"
        formatter.locale = Locale(identifier: "en_US_POSIX")  // 防系统区域影响数字字符（i18n locale 安全）
        formatter.timeZone = TimeZone.current  // 本机时区（与 V1 钦定一致）
        return formatter.string(from: Date())
    }
}
