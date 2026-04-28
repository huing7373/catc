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

    /// case#1：.business 业务错误（permanent 类）→ AlertOverlay + 错误码字典文案 + force-quit suffix.
    /// 3002 "步数不足" 是 permanent（用户操作上下文错,不是 server 瞬时容量问题）→ .alert.
    /// Story 5.5 round 7 [P1] fix: alert message 文案末尾追加 "持续失败时请杀进程重启 App"
    /// —— 配合 alert dismiss 调 retry()(user-driven recovery), 让 user 知道多次失败时该自己 force-quit.
    func testBusinessErrorMapsToAlertWithLocalizedMessage() {
        let error = APIError.business(code: 3002, message: "原文(server)", requestId: "req_1")
        let presentation = AppErrorMapper.presentation(for: error)
        XCTAssertEqual(
            presentation,
            .alert(title: "提示", message: "步数不足，再走走吧。持续失败时请杀进程重启 App")
        )
    }

    /// case#2：.business 但 code 不在字典内 → fallback 用 server message,默认 .alert（permanent）.
    /// Story 5.5 round 7 [P1] fix: 同样追加 force-quit suffix.
    func testBusinessErrorWithUnknownCodeFallsBackToServerMessage() {
        let error = APIError.business(code: 9999, message: "未知错误描述", requestId: "req_x")
        let presentation = AppErrorMapper.presentation(for: error)
        XCTAssertEqual(
            presentation,
            .alert(title: "提示", message: "未知错误描述。持续失败时请杀进程重启 App")
        )
    }

    /// case#3：.business 未知 code 且 server message 为空 → fallback 通用文案 + force-quit suffix.
    func testBusinessErrorWithUnknownCodeAndEmptyMessageUsesGenericFallback() {
        let error = APIError.business(code: 9999, message: "", requestId: "req_x")
        let presentation = AppErrorMapper.presentation(for: error)
        XCTAssertEqual(
            presentation,
            .alert(title: "提示", message: "操作失败，请稍后重试。持续失败时请杀进程重启 App")
        )
    }

    // MARK: - transient 业务码（Story 5.5 round 5 [P1] fix）

    /// transient 业务码（1005/1007/1008/1009）必须走 .retry 而非 .alert,
    /// 让 bootstrap 路径下 1009 "服务繁忙" 等可恢复错误能进 RetryView 而非 exit App.

    /// 1005 操作过于频繁 → .retry
    func testBusinessCode1005MapsToRetry() {
        let error = APIError.business(code: 1005, message: "原文", requestId: "req_x")
        let presentation = AppErrorMapper.presentation(for: error)
        XCTAssertEqual(presentation, .retry(message: "操作过于频繁，请稍后再试"))
    }

    /// 1007 数据冲突 → .retry
    func testBusinessCode1007MapsToRetry() {
        let error = APIError.business(code: 1007, message: "原文", requestId: "req_x")
        let presentation = AppErrorMapper.presentation(for: error)
        XCTAssertEqual(presentation, .retry(message: "数据冲突，请重试"))
    }

    /// 1008 操作重复 → .retry
    func testBusinessCode1008MapsToRetry() {
        let error = APIError.business(code: 1008, message: "原文", requestId: "req_x")
        let presentation = AppErrorMapper.presentation(for: error)
        XCTAssertEqual(presentation, .retry(message: "操作重复，请稍后再试"))
    }

    /// 1009 服务繁忙 → .retry —— round 5 [P1] regression fix:
    /// bootstrap path 下此码必须保留重试入口（之前 round 4 错误改成 .alert → AlertOverlay 死锁）.
    func testBusinessCode1009MapsToRetry() {
        let error = APIError.business(code: 1009, message: "原文", requestId: "req_x")
        let presentation = AppErrorMapper.presentation(for: error)
        XCTAssertEqual(presentation, .retry(message: "服务繁忙，请稍后重试"))
    }

    /// 反例：相邻的 1004 (权限不足) 是 permanent 类,必须走 .alert（用户行为不会因为 retry 自愈）.
    /// Story 5.5 round 7 [P1] fix: 同样追加 force-quit suffix.
    func testBusinessCode1004StaysAlert() {
        let error = APIError.business(code: 1004, message: "原文", requestId: "req_x")
        let presentation = AppErrorMapper.presentation(for: error)
        XCTAssertEqual(
            presentation,
            .alert(title: "提示", message: "权限不足。持续失败时请杀进程重启 App")
        )
    }

    /// case#4（Story 5.4 round 5 fix → Story 5.5 round 7 [P1] 调整）：.unauthorized → Alert.
    /// AuthRetryingAPIClient 上线后,业务层接到的 .unauthorized 必然是"已 exhaust 静默重登"的
    /// 场景（relogin 失败 / retry-after-relogin 仍 401）。
    /// round 7 fix: 文案从 "请重新启动应用" 改成 "请重试。持续失败时请杀进程重启 App" ——
    /// 配合 alert dismiss 调 retry() (user-driven recovery), 不再让 app exit(0) 替 user 决定.
    func testUnauthorizedMapsToAlertWithRetryHint() {
        let presentation = AppErrorMapper.presentation(for: APIError.unauthorized)
        XCTAssertEqual(
            presentation,
            .alert(title: "提示", message: "登录失败，请重试。持续失败时请杀进程重启 App")
        )
    }

    /// case#4b（Story 5.4 round 2 fix 新增）：.missingCredentials → Alert（"请重启 App"）.
    /// 跟 .unauthorized 区分：本地态需要 cold-start 接手，retry 救不回（keychain 真没 token）,
    /// 文案直接钦定 "请重启 App", 不加 "请重试" 前缀.
    func testMissingCredentialsMapsToAlertWithRestartHint() {
        let presentation = AppErrorMapper.presentation(for: APIError.missingCredentials)
        XCTAssertEqual(presentation, .alert(title: "提示", message: "登录信息丢失，请重启 App"))
    }

    /// case#5：.network → RetryView
    func testNetworkErrorMapsToRetry() {
        let presentation = AppErrorMapper.presentation(for: APIError.network(underlying: URLError(.timedOut)))
        XCTAssertEqual(presentation, .retry(message: "网络异常，请检查后重试"))
    }

    /// case#6：.decoding → Alert with retry hint (Story 5.5 round 7 [P1] fix).
    func testDecodingErrorMapsToAlert() {
        let presentation = AppErrorMapper.presentation(for: APIError.decoding(underlying: URLError(.cannotParseResponse)))
        XCTAssertEqual(
            presentation,
            .alert(title: "提示", message: "数据异常，请重试。持续失败时请杀进程重启 App")
        )
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

    /// business 错误（transient 1009）→ 走 .retry → 文案与 retry message 一致（错误码字典命中）.
    /// Story 5.5 round 5 [P1] fix: 1009 从 .alert 改 .retry 后,本测试仍验证 message 提取正确,
    /// 因为 userFacingMessage 内部 switch 三态都 return message 字段.
    func testUserFacingMessageForBusinessErrorMatchesRetryCopy() {
        let msg = AppErrorMapper.userFacingMessage(
            for: APIError.business(code: 1009, message: "原文(server)", requestId: "req_x")
        )
        XCTAssertEqual(msg, "服务繁忙，请稍后重试")
    }

    /// business 错误（permanent 3002）→ 走 .alert → 文案与 alert message 一致（带 force-quit suffix）.
    /// Story 5.5 round 7 [P1] fix: alert 文案末尾追加 force-quit suffix, helper 透传完整 message.
    func testUserFacingMessageForPermanentBusinessErrorMatchesAlertCopy() {
        let msg = AppErrorMapper.userFacingMessage(
            for: APIError.business(code: 3002, message: "原文(server)", requestId: "req_x")
        )
        XCTAssertEqual(msg, "步数不足，再走走吧。持续失败时请杀进程重启 App")
    }

    /// decoding 错误 → 走 .alert → 文案与 alert message 一致（Story 5.5 round 7 [P1] fix）.
    func testUserFacingMessageForDecodingErrorMatchesAlertCopy() {
        let msg = AppErrorMapper.userFacingMessage(for: APIError.decoding(underlying: URLError(.cannotParseResponse)))
        XCTAssertEqual(msg, "数据异常，请重试。持续失败时请杀进程重启 App")
    }

    /// 非 APIError → 走 fallback alert → 文案与 fallback message 一致
    func testUserFacingMessageForGenericErrorMatchesFallbackCopy() {
        struct CustomError: Error {}
        let msg = AppErrorMapper.userFacingMessage(for: CustomError())
        XCTAssertEqual(msg, "请稍后重试", "非 APIError 应走 fallback 文案,与 presentation 对齐")
    }
}
