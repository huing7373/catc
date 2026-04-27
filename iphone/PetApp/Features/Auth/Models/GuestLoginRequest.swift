// GuestLoginRequest.swift
// Story 5.2 AC1: POST /api/v1/auth/guest-login 请求体；严格对齐 V1 §4.1 行 144-152 schema。
//
// V1 §4.1 钦定字段约束：
// - guestUid: 1-128 字符（utf8.RuneCountInString，按 V1 §2.5 钦定 —— 不是字节数）
// - device.platform: enum "ios" / "android"（节点 2 仅 "ios"）
// - device.appVersion: 1-32 字符
// - device.deviceModel: 1-64 字符
//
// 客户端**不**做长度校验（server 端 1002 兜底）；客户端只保证：
// - guestUid: 调 UUID().uuidString 拿到固定 36 字符（远少于 128）
// - device.appVersion: 从 Bundle 读，正常 < 32 字符
// - device.deviceModel: utsname.machine 正常 < 64 字符（如 "iPhone15,2"）

import Foundation

public struct GuestLoginRequest: Encodable, Equatable {
    public let guestUid: String
    public let device: Device

    public init(guestUid: String, device: Device) {
        self.guestUid = guestUid
        self.device = device
    }

    public struct Device: Encodable, Equatable, Sendable {
        public let platform: String   // "ios" / "android"，节点 2 仅 "ios"
        public let appVersion: String // 如 "1.0.0"
        public let deviceModel: String // 如 "iPhone15,2"

        public init(platform: String, appVersion: String, deviceModel: String) {
            self.platform = platform
            self.appVersion = appVersion
            self.deviceModel = deviceModel
        }
    }
}
