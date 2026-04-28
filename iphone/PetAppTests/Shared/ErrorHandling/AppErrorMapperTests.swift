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

    /// case#4（Story 5.4 round 5 fix 修正）：.unauthorized → Alert（"请重启应用"）.
    /// AuthRetryingAPIClient 上线后,业务层接到的 .unauthorized 必然是"已 exhaust 静默重登"的
    /// 场景（relogin 失败 / retry-after-relogin 仍 401）。继续 toast "正在重新登录" 既误导
    /// （没有 relogin 在跑）又非 recoverable（toast 自动消失,用户无 action point）。
    /// 改成 blocking alert,让用户走 cold-start 路径,跟 .missingCredentials 对齐.
    func testUnauthorizedMapsToAlertWithRestartHint() {
        let presentation = AppErrorMapper.presentation(for: APIError.unauthorized)
        XCTAssertEqual(presentation, .alert(title: "提示", message: "登录失败，请重新启动应用"))
    }

    /// case#4b（Story 5.4 round 2 fix 新增）：.missingCredentials → Alert（"请重启应用"）.
    /// 跟 .unauthorized 区分：本地态需要 cold-start 接手，**不**该让用户以为系统在自动恢复.
    func testMissingCredentialsMapsToAlertWithRestartHint() {
        let presentation = AppErrorMapper.presentation(for: APIError.missingCredentials)
        XCTAssertEqual(presentation, .alert(title: "提示", message: "登录信息丢失，请重启应用"))
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

    // MARK: - userFacingMessage（Story 5.5 codex round 1 [P2] fix）

    /// 给非"调 ErrorPresenter" 路径用：bootstrap step closure 失败需把 APIError 转成 user-facing
    /// 文案抛回 AppLaunchStateMachine —— 此 helper 必须与 presentation(for:) 对齐 message 部分,
    /// 不能各自独立维护一套文案表（否则 RetryView / Alert 与 bootstrap message 会漂移）.

    /// network 错误 → 走 .retry → 文案与 retry message 一致
    func testUserFacingMessageForNetworkErrorMatchesRetryCopy() {
        let msg = AppErrorMapper.userFacingMessage(for: APIError.network(underlying: URLError(.timedOut)))
        XCTAssertEqual(msg, "网络异常，请检查后重试", "bootstrap 路径必须复用 mapper user copy,不再展示 'Network error: ...'")
    }

    /// business 错误 → 走 .alert → 文案与 alert message 一致（错误码字典命中）
    func testUserFacingMessageForBusinessErrorMatchesAlertCopy() {
        let msg = AppErrorMapper.userFacingMessage(
            for: APIError.business(code: 1009, message: "原文(server)", requestId: "req_x")
        )
        XCTAssertEqual(msg, "服务繁忙，请稍后重试")
    }

    /// decoding 错误 → 走 .alert → 文案与 alert message 一致
    func testUserFacingMessageForDecodingErrorMatchesAlertCopy() {
        let msg = AppErrorMapper.userFacingMessage(for: APIError.decoding(underlying: URLError(.cannotParseResponse)))
        XCTAssertEqual(msg, "数据异常，请稍后重试")
    }

    /// 非 APIError → 走 fallback alert → 文案与 fallback message 一致
    func testUserFacingMessageForGenericErrorMatchesFallbackCopy() {
        struct CustomError: Error {}
        let msg = AppErrorMapper.userFacingMessage(for: CustomError())
        XCTAssertEqual(msg, "请稍后重试", "非 APIError 应走 fallback 文案,与 presentation 对齐")
    }
}
