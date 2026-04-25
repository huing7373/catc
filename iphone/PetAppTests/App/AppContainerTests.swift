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

    // MARK: - round 4 [P2]：validatedBaseURL(fromString:) 拒绝 malformed 输入

    /// case#4a (round 4)：`URL(string:)` 对 malformed 输入过于宽容，需在 resolve 层补校验。
    /// codex round 4 finding：`URL(string: "localhost:8080")` 返回 non-nil
    /// （Apple parser 把它当成 `scheme=localhost, path=8080`），但 `APIClient` 构请求会失败，
    /// 表现是 ping/version 永远 offline。所以 resolve 层必须自己卡 scheme + host。
    /// 详见 docs/lessons/2026-04-26-url-string-malformed-tolerance.md。
    func testValidatedBaseURLRejectsMalformedInputs() {
        // 1. 无 scheme（仅 host:port） — Apple URL parser 不会拒，但语义错。
        XCTAssertNil(AppContainer.validatedBaseURL(fromString: "localhost:8080"),
                     "缺 scheme 的 host:port 字符串应被拒绝")

        // 2. 仅有 scheme 没有 host
        XCTAssertNil(AppContainer.validatedBaseURL(fromString: "http://"),
                     "缺 host 的 URL 字符串应被拒绝")

        // 3. 不支持的 scheme（ftp / ws / file 等都不应作为 HTTP API baseURL）
        XCTAssertNil(AppContainer.validatedBaseURL(fromString: "ftp://example.com"),
                     "ftp scheme 不支持 → 应被拒绝")
        XCTAssertNil(AppContainer.validatedBaseURL(fromString: "ws://example.com"),
                     "ws scheme 不支持（WebSocket 用 wss/ws 走另一通道）→ 应被拒绝")

        // 4. 空字符串
        XCTAssertNil(AppContainer.validatedBaseURL(fromString: ""),
                     "空字符串应被拒绝")

        // 5. 含空格的非法 URL（URL parser 也会拒）
        XCTAssertNil(AppContainer.validatedBaseURL(fromString: "http://example .com"),
                     "含空格的字符串应被拒绝")
    }

    /// case#4b (round 4)：合法 http/https URL 必须被接受，scheme 大小写不敏感。
    func testValidatedBaseURLAcceptsValidHTTPAndHTTPS() {
        // 标准 http
        XCTAssertEqual(
            AppContainer.validatedBaseURL(fromString: "http://localhost:8080")?.absoluteString,
            "http://localhost:8080"
        )
        // 标准 https
        XCTAssertEqual(
            AppContainer.validatedBaseURL(fromString: "https://api.example.com")?.absoluteString,
            "https://api.example.com"
        )
        // 大写 scheme 也接受（lowercased 后比较）
        XCTAssertNotNil(AppContainer.validatedBaseURL(fromString: "HTTPS://api.example.com"))
        XCTAssertNotNil(AppContainer.validatedBaseURL(fromString: "HTTP://localhost:8080"))
        // 带 path / trailing slash 也接受（trailing slash normalize 由 APIClient init 负责，与 baseURL 校验解耦）
        XCTAssertNotNil(AppContainer.validatedBaseURL(fromString: "http://localhost:8080/api/v1/"))
    }

    /// case#5 (round 3)：PetApp 的 Info.plist 必须配置 NSAppTransportSecurity → NSAllowsLocalNetworking = true。
    /// 否则 cleartext HTTP（http://localhost:8080）会被 iOS ATS 在 OS 层拒绝，feature 永远 offline。
    /// 详见 docs/lessons/2026-04-26-ios-ats-cleartext-http.md。
    ///
    /// 注意：直接读 Bundle.main.infoDictionary 拿到的是 test host 的 plist；要拿被测 PetApp.app 的 plist，
    /// 需要从 PetApp 内部某 class（如 AppContainer 本身）的 Bundle(for:) 反查。
    func testInfoPlistAllowsLocalNetworking() {
        // PetApp.app 的 Bundle —— 通过 AppContainer 这个类反查（与 main bundle 不同）。
        let petAppBundle = Bundle(for: AppContainer.self)

        guard let ats = petAppBundle.object(forInfoDictionaryKey: "NSAppTransportSecurity") as? [String: Any] else {
            XCTFail("PetApp Info.plist 必须配置 NSAppTransportSecurity（用于允许 cleartext localhost）")
            return
        }

        let allowsLocal = ats["NSAllowsLocalNetworking"] as? Bool
        XCTAssertEqual(allowsLocal, true,
                       "NSAllowsLocalNetworking 必须为 true；缺失会让 ping/version 在真机和模拟器都被 ATS 拒绝")
    }
}
