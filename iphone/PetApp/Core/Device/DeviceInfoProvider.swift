// DeviceInfoProvider.swift
// Story 5.2 AC5: 静态 helper 提供 GuestLoginRequest.Device。
//
// platform: 节点 2 硬编码 "ios"
// appVersion: 从 Bundle.main.infoDictionary 读 CFBundleShortVersionString，缺省 "0.0.0"
// deviceModel: 用 utsname.machine 拿硬件型号（如 "iPhone15,2"）；
//   **不**用 UIDevice.current.model —— 那只能拿到 "iPhone" / "iPad" 类目串，不符 V1 §4.1 钦定
//   "设备型号 如 iPhone15,2"。utsname 是 POSIX 标准 syscall，Foundation 已暴露，无需 import UIKit/Darwin。
//
// 设计：纯静态读 Bundle + utsname，无 mock 价值；UseCase 接受 () -> Device closure 注入则用闭包替代。

import Foundation

public enum DeviceInfoProvider {
    /// platform 硬编码 "ios"；与 V1 §4.1 钦定枚举对齐。
    public static let platform: String = "ios"

    /// 从 Bundle.main 读 CFBundleShortVersionString；缺省 "0.0.0"（与 HomeViewModel.readAppVersion() 同模式）。
    public static func appVersion() -> String {
        (Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String) ?? "0.0.0"
    }

    /// 读 utsname.machine 拿硬件型号（如 "iPhone15,2"）。
    /// utsname 是 POSIX syscall，比 UIDevice.current.model 精确。
    /// 失败回退 "unknown"（极少见，仅当 systemcall 错时；不抛错）。
    public static func deviceModel() -> String {
        var systemInfo = utsname()
        uname(&systemInfo)
        let mirror = Mirror(reflecting: systemInfo.machine)
        let identifier = mirror.children.reduce("") { id, element in
            guard let value = element.value as? Int8, value != 0 else { return id }
            return id + String(UnicodeScalar(UInt8(value)))
        }
        return identifier.isEmpty ? "unknown" : identifier
    }

    /// 一次性拿 Device 对象（GuestLoginUseCase 默认 deviceProvider closure 调）。
    public static func current() -> GuestLoginRequest.Device {
        GuestLoginRequest.Device(
            platform: platform,
            appVersion: appVersion(),
            deviceModel: deviceModel()
        )
    }
}
