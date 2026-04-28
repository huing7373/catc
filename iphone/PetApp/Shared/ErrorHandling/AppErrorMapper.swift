// AppErrorMapper.swift
// Story 2.6 AC1：把 APIError 映射到面向用户的呈现样式 + 文案。
//
// 职责：
// - 接收任意 Error，先识别 APIError 四态（business/unauthorized/network/decoding），其余走 generic fallback
// - 业务码 → 用户文案：覆盖 V1接口设计.md §3 全部 32 码（1xxx/2xxx/3xxx/4xxx/5xxx/6xxx/7xxx）
// - 不调 logger（→ Story 2.7 落地后再加）
// - 不做 i18n（MVP 阶段全中文 hardcode）
//
// 设计选择：
// - 用 enum + static func（与 AccessibilityID 风格一致）：本类是无状态查表器
// - presentation(for:) 接收 Error 而非 APIError：让 ViewModel catch block 不必 narrow
// - 错误码字典放 switch case 而非 dictionary：let compiler 帮我们做穷举检查

import Foundation

/// `AppErrorMapper`：APIError → ErrorPresentation（呈现样式 + 文案）的映射器。
///
/// 映射规则（与 总体架构 §V1错误码规范 + iOS 架构 §8.3 对齐）：
///
/// - `.business(code, message, _)` → AlertOverlay；文案优先用本地 codeMessage 表（按 V1 错误码字典精挑短句），
///   未命中时退回 server 返回的 `message`（再为空就给通用兜底）。
/// - `.unauthorized` → AlertOverlay；文案 "登录失败，请重新启动应用"（Story 5.4 round 5 fix
///   修正：Story 5.4 落地 `AuthRetryingAPIClient` 后,业务层接到 `.unauthorized` 的语义已经反转 ——
///   不再是"server 第一次返 401"（那种已被 decorator 内部静默重登 + 重试一次吞掉），而是"已经
///   exhaust 了那唯一一次静默重登尝试"（relogin 失败 / 重试后**仍**是 401）。继续 toast
///   "正在重新登录..." 既误导（实际没有重登在跑）又非 recoverable（toast 2s 自动消失,用户无任何
///   action point）。改成 blocking alert + "请重启应用" 让用户走 cold-start GuestLoginUseCase
///   重新拿 token,跟 `.missingCredentials` 的处理一致。
/// - `.missingCredentials` → AlertOverlay；文案 "登录信息丢失，请重启应用"（Story 5.4 round 2 fix
///   新增：本地态走"引导冷启动"路径，不该被 toast "正在重登"误导用户以为系统在自动恢复 —— 实际上
///   AuthRetryingAPIClient **不**会 catch 这个 case，需要 cold-start GuestLoginUseCase 接手）。
/// - `.network(_)` → RetryView；文案 "网络异常，请检查后重试"。
/// - `.decoding(_)` → AlertOverlay；文案 "数据异常，请稍后重试"。
public enum AppErrorMapper {
    /// 把任意 Error 映射成 ErrorPresentation（呈现样式 + 文案）。
    /// 入参 error 必须是 APIError 才走具体分支；其它 Error 类型走 fallback `.alert("操作失败", "请稍后重试")`。
    public static func presentation(for error: Error) -> ErrorPresentation {
        guard let apiError = error as? APIError else {
            return ErrorPresentation.alert(title: "操作失败", message: "请稍后重试")
        }
        switch apiError {
        case let .business(code, message, _):
            let userMessage = localizedMessage(forBusinessCode: code, fallback: message)
            return ErrorPresentation.alert(title: "提示", message: userMessage)

        case .unauthorized:
            // Story 5.4 round 5 fix: AuthRetryingAPIClient 上线后,业务层能接到的 .unauthorized
            // 必然是"已经 exhaust 一次静默重登尝试"的场景 —— 此时既没有 relogin 在跑（toast "正在
            // 重新登录" 是谎言），也无法靠点击 retry 在装饰器层自愈（同 generation 的 401 会被
            // dedup 短路返回旧 token，再失败仍走到这里形成 user-perceivable loop）。
            // 改成 blocking alert + "请重启应用"：让用户走 cold-start 路径重新拿 token,与
            // .missingCredentials 的处理对齐.
            return ErrorPresentation.alert(title: "提示", message: "登录失败，请重新启动应用")

        case .missingCredentials:
            // Story 5.4 round 2 fix: 跟 .unauthorized 区分 —— 本地态需要冷启动接手，
            // 不能 toast "正在重登" 误导用户以为后台在自动恢复（AuthRetryingAPIClient 不接管）。
            return ErrorPresentation.alert(title: "提示", message: "登录信息丢失，请重启应用")

        case .network:
            return ErrorPresentation.retry(message: "网络异常，请检查后重试")

        case .decoding:
            return ErrorPresentation.alert(title: "提示", message: "数据异常，请稍后重试")
        }
    }

    /// 抽出**纯文案**（不带 presentation 样式）的对外 helper.
    /// 给非"调 ErrorPresenter"路径用,如启动状态机 bootstrap 的 step closure 失败时
    /// 需要把错误转成 user-facing message 再 throw 出去，让 AppLaunchStateMachine
    /// 的 .needsAuth(message:) 走 RetryView 而不是显示 developer 串.
    ///
    /// Story 5.5 codex round 1 [P2] fix: bootstrap loadHomeUseCase.execute() 失败时
    /// 原走 `messageFor(error:)` → `APIError.errorDescription`,产出 "Network error: ..."
    /// 等 developer 文案. 改用本 helper 让 mapper 唯一定义点 production user copy.
    /// 详见 docs/lessons/2026-04-27-bootstrap-error-must-route-via-mapper.md.
    ///
    /// 实现：复用 `presentation(for:)` 然后从 ErrorPresentation 提取文案部分.
    /// 不直接重复 mapping switch —— 单一 source of truth,以后改 mapper 文案时
    /// bootstrap 路径自动跟上.
    public static func userFacingMessage(for error: Error) -> String {
        switch presentation(for: error) {
        case let .toast(message):
            return message
        case let .alert(_, message):
            return message
        case let .retry(message):
            return message
        }
    }

    /// 错误码 → 用户文案。覆盖 V1接口设计 §3 全部 32 码。未命中（业务方传未知 code）退回 server 返回的 message。
    /// 表里短句**一律不超过 12 字**，避免 alert 排版换行；超过 12 字的（如"操作过于频繁，请稍后再试"）单独 review。
    public static func localizedMessage(forBusinessCode code: Int, fallback: String) -> String {
        switch code {
        // 1xxx 通用错误（V1 §3）
        case 1001: return "登录已过期，请重新登录"
        case 1002: return "请求参数错误"
        case 1003: return "资源不存在"
        case 1004: return "权限不足"
        case 1005: return "操作过于频繁，请稍后再试"
        case 1006: return "当前状态不支持此操作"
        case 1007: return "数据冲突，请重试"
        case 1008: return "操作重复，请稍后再试"
        case 1009: return "服务繁忙，请稍后重试"

        // 2xxx 账号
        case 2001: return "账号不存在"
        case 2002: return "微信已绑定其他账号"
        case 2003: return "当前账号已绑定微信"

        // 3xxx 步数
        case 3001: return "步数同步异常"
        case 3002: return "步数不足，再走走吧"

        // 4xxx 宝箱
        case 4001: return "宝箱不存在"
        case 4002: return "宝箱尚未解锁"
        case 4003: return "暂时不能开启宝箱"

        // 5xxx 道具 / 装扮 / 合成
        case 5001: return "道具不存在"
        case 5002: return "道具不属于你"
        case 5003: return "道具状态不可用"
        case 5004: return "装备槽位不匹配"
        case 5005: return "合成材料数量错误"
        case 5006: return "合成材料品质不一致"
        case 5007: return "合成目标品质不合法"
        case 5008: return "装扮已装备"

        // 6xxx 房间
        case 6001: return "房间不存在"
        case 6002: return "房间已满"
        case 6003: return "你已在房间中"
        case 6004: return "你不在房间中"
        case 6005: return "房间状态异常"

        // 7xxx 表情 / WS
        case 7001: return "表情不存在"
        case 7002: return "实时连接未就绪"

        // 未知 code：退回 server 原文（如 server message 也是空字符串则给通用文案）
        default:
            return fallback.isEmpty ? "操作失败，请稍后重试" : fallback
        }
    }
}
