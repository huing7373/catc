// AppErrorMapperTests.swift
// Story 2.6 AC9：AppErrorMapper 单元测试。
//
// 覆盖：
// - APIError 四态映射（business/unauthorized/network/decoding）
// - business 错误码字典抽样（覆盖 V1 §3 各档代表）
// - 未知 code 退回 server message 的兜底策略
// - 非 APIError 的 generic Error fallback

import XCTest
@testable import PetApp

final class AppErrorMapperTests: XCTestCase {

    // MARK: - APIError 四态映射

    /// case#1：.business 业务错误 → AlertOverlay + 错误码字典文案
    func testBusinessErrorMapsToAlertWithLocalizedMessage() {
        let error = APIError.business(code: 3002, message: "原文(server)", requestId: "req_1")
        let presentation = AppErrorMapper.presentation(for: error)
        XCTAssertEqual(presentation, .alert(title: "提示", message: "步数不足，再走走吧"))
    }

    /// case#2：.business 但 code 不在字典内 → fallback 用 server message
    func testBusinessErrorWithUnknownCodeFallsBackToServerMessage() {
        let error = APIError.business(code: 9999, message: "未知错误描述", requestId: "req_x")
        let presentation = AppErrorMapper.presentation(for: error)
        XCTAssertEqual(presentation, .alert(title: "提示", message: "未知错误描述"))
    }

    /// case#3：.business 未知 code 且 server message 为空 → fallback 通用文案
    func testBusinessErrorWithUnknownCodeAndEmptyMessageUsesGenericFallback() {
        let error = APIError.business(code: 9999, message: "", requestId: "req_x")
        let presentation = AppErrorMapper.presentation(for: error)
        XCTAssertEqual(presentation, .alert(title: "提示", message: "操作失败，请稍后重试"))
    }

    /// case#4：.unauthorized → Toast
    func testUnauthorizedMapsToToast() {
        let presentation = AppErrorMapper.presentation(for: APIError.unauthorized)
        XCTAssertEqual(presentation, .toast(message: "登录已过期，正在重新登录..."))
    }

    /// case#5：.network → RetryView
    func testNetworkErrorMapsToRetry() {
        let presentation = AppErrorMapper.presentation(for: APIError.network(underlying: URLError(.timedOut)))
        XCTAssertEqual(presentation, .retry(message: "网络异常，请检查后重试"))
    }

    /// case#6：.decoding → Alert
    func testDecodingErrorMapsToAlert() {
        let presentation = AppErrorMapper.presentation(for: APIError.decoding(underlying: URLError(.cannotParseResponse)))
        XCTAssertEqual(presentation, .alert(title: "提示", message: "数据异常，请稍后重试"))
    }

    /// case#7：非 APIError 的 generic Error → fallback Alert
    func testGenericErrorMapsToFallbackAlert() {
        struct CustomError: Error {}
        let presentation = AppErrorMapper.presentation(for: CustomError())
        XCTAssertEqual(presentation, .alert(title: "操作失败", message: "请稍后重试"))
    }

    // MARK: - localizedMessage 错误码字典抽样

    /// case#8：错误码字典 spot-check（覆盖 V1 §3 各档代表）
    func testLocalizedMessageDictionarySpotCheck() {
        XCTAssertEqual(AppErrorMapper.localizedMessage(forBusinessCode: 1001, fallback: ""), "登录已过期，请重新登录")
        XCTAssertEqual(AppErrorMapper.localizedMessage(forBusinessCode: 1005, fallback: ""), "操作过于频繁，请稍后再试")
        XCTAssertEqual(AppErrorMapper.localizedMessage(forBusinessCode: 4002, fallback: ""), "宝箱尚未解锁")
        XCTAssertEqual(AppErrorMapper.localizedMessage(forBusinessCode: 5005, fallback: ""), "合成材料数量错误")
        XCTAssertEqual(AppErrorMapper.localizedMessage(forBusinessCode: 6002, fallback: ""), "房间已满")
        XCTAssertEqual(AppErrorMapper.localizedMessage(forBusinessCode: 7002, fallback: ""), "实时连接未就绪")
    }
}
