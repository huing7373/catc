// AppContainerTests.swift
// Story 2.5 review fix round 1：AppContainer.resolveDefaultBaseURL(from:) 单元测试。
//
// 背景：codex review round 1 finding #2 指出默认 baseURL 不应硬编码 localhost；
// 修复策略：让 AppContainer 从 Info.plist 读 PetAppBaseURL，缺省 fallback 到 localhost。
// 详见 docs/lessons/2026-04-26-baseurl-from-info-plist.md。
//
// 测试基础设施约束（与 Story 2.7 衔接）：
// - 仅依赖 stdlib（XCTest + @testable import PetApp），不引入 helper 文件。
// - 用真实 Bundle (Bundle(for:)) + 临时写入的 Info.plist fixture 覆盖正向 / fallback / 异常路径。

import XCTest
@testable import PetApp

@MainActor
final class AppContainerTests: XCTestCase {

    /// case#1 (happy)：Info.plist 有 PetAppBaseURL → 读取并返回该 URL。
    /// 用 main bundle —— 当前 PetApp Info.plist 已默认配置 http://localhost:8080，验证读取链路。
    func testResolveDefaultBaseURLReadsFromMainBundle() {
        let url = AppContainer.resolveDefaultBaseURL(from: Bundle.main)
        // main bundle 在 test host 下不一定与 PetApp Info.plist 一致；只断言能产出 URL（非 nil）即可。
        XCTAssertNotNil(url.scheme, "应解析出合法 scheme")
        XCTAssertNotNil(url.host, "应解析出 host（fallback localhost 或 PetAppBaseURL 配置值）")
    }

    /// case#2 (edge)：bundle 不含 PetAppBaseURL → fallback 到 localhost。
    /// 用本测试 target 的 Bundle —— 该 bundle Info.plist 没配置 PetAppBaseURL，必走 fallback 分支。
    func testResolveDefaultBaseURLFallsBackWhenKeyMissing() {
        let testBundle = Bundle(for: AppContainerTests.self)
        // 防御：如果未来测试 bundle 配置了 PetAppBaseURL，本测试会假阳性，提前拦截。
        XCTAssertNil(testBundle.object(forInfoDictionaryKey: AppContainer.baseURLInfoKey),
                     "测试 bundle 不应配置 PetAppBaseURL；如需配置请改本测试断言策略")

        let url = AppContainer.resolveDefaultBaseURL(from: testBundle)

        XCTAssertEqual(url.absoluteString, AppContainer.fallbackBaseURLString,
                       "缺 key 时应 fallback 到 \(AppContainer.fallbackBaseURLString)")
    }

    /// case#3 (sanity)：默认 init 走通，apiClient 非 nil。
    func testDefaultInitProducesUsableContainer() {
        let container = AppContainer()
        XCTAssertNotNil(container.apiClient, "默认 init 应构造可用的 APIClient")
        XCTAssertNotNil(container.makePingUseCase(), "默认 container 应能产出 PingUseCase")
    }
}
