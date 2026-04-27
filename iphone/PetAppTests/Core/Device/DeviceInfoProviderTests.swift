// DeviceInfoProviderTests.swift
// Story 5.2 AC11: DeviceInfoProvider 静态 helper 测试.
//
// 不 mock Bundle / utsname：测试在 simulator 上跑能拿到真实值（appVersion 来自测试 host bundle，
// deviceModel 来自 simulator 硬件）.
// 仅断言 "非空" + "platform 是 ios"：因为 deviceModel / appVersion 在不同 simulator / Xcode 版本
// 下值不同，不能硬编码.

import XCTest
@testable import PetApp

final class DeviceInfoProviderTests: XCTestCase {

    // MARK: - case#1: platform 永远是 "ios"

    func testPlatformIsAlwaysIos() {
        XCTAssertEqual(DeviceInfoProvider.platform, "ios", "platform 必须硬编码为 'ios'，与 V1 §4.1 enum 对齐")
    }

    // MARK: - case#2: appVersion 非空

    func testAppVersionIsNonEmpty() {
        let v = DeviceInfoProvider.appVersion()
        XCTAssertFalse(v.isEmpty, "appVersion 应非空，至少回退到 0.0.0")
    }

    // MARK: - case#3: deviceModel 非空

    func testDeviceModelIsNonEmpty() {
        let model = DeviceInfoProvider.deviceModel()
        XCTAssertFalse(model.isEmpty, "deviceModel 应非空")
    }

    // MARK: - case#4: current() 返回完整 Device

    func testCurrentReturnsCompleteDevice() {
        let device = DeviceInfoProvider.current()
        XCTAssertEqual(device.platform, "ios")
        XCTAssertFalse(device.appVersion.isEmpty)
        XCTAssertFalse(device.deviceModel.isEmpty)
    }
}
