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
//
// **transient vs terminal 业务码区分（Story 5.5 round 5 [P1] → round 7 [P1] → round 8 [P1] 调整）**：
// .business 不再统一映射成 .alert。
// - `.alert` 语义是 "terminal-class 错误,bootstrap 路径下 RootView 渲染 TerminalErrorView (静态
//   全屏 page, 无按钮, user 必须主动 force-quit)". round 8 [P1] fix: 文案回归简洁形态 (round 5
//   风格), 不再带 "持续失败时请杀进程重启 App" suffix —— 该指引文本已经移到 TerminalErrorView
//   底部静态文本, 让 mapper 文案专注表达"什么错了" 而非"怎么办".
//   详见 docs/lessons/2026-04-27-bootstrap-terminal-error-static-fallback-page.md.
// - transient 业务码（1005 频繁 / 1007 冲突 / 1008 重复 / 1009 服务繁忙；网络/容量/限流类瞬时
//   错误）走 .retry —— 让冷启动 bootstrap 路径下 1009 等可恢复错误进 RetryView.
// 详见 docs/lessons/2026-04-27-business-error-transient-vs-terminal.md
// + docs/lessons/2026-04-27-bootstrap-alert-dismiss-must-be-user-driven-recovery.md
// + docs/lessons/2026-04-27-bootstrap-terminal-error-static-fallback-page.md.

import Foundation

/// `AppErrorMapper`：APIError → ErrorPresentation（呈现样式 + 文案）的映射器。
///
/// 映射规则（与 总体架构 §V1错误码规范 + iOS 架构 §8.3 对齐）：
///
/// - `.business(code, message, _)` → 取决于 code 的语义类:
///   - **transient**（瞬时类:1005/1007/1008/1009）→ `.retry`,文案取本地表（"操作过于频繁/数据冲突/操作重复/服务繁忙"）.
///   - **permanent**（其他 1xxx/2xxx/...）→ `.alert`,文案直接取本地表（round 8 [P1] fix: 不再带
///     "持续失败时请杀进程重启 App" suffix —— 该指引已 move 到 TerminalErrorView 底部静态文本,
///     mapper 文案专注表达"什么错了"; 详见文件头注释 + lesson 文档）.
/// - `.unauthorized` → bootstrap 路径渲染 TerminalErrorView；文案 "登录失败，请重新启动应用"
///   （Story 5.4 round 5 fix 修正：Story 5.4 落地 `AuthRetryingAPIClient` 后,业务层接到
///   `.unauthorized` 的语义已经反转 —— 不再是"server 第一次返 401"（那种已被 decorator 内部
///   静默重登 + 重试一次吞掉），而是"已经 exhaust 了那唯一一次静默重登尝试"（relogin 失败 /
///   重试后**仍**是 401）。继续 toast "正在重新登录..." 既误导（实际没有重登在跑）又非
///   recoverable（toast 2s 自动消失,用户无任何 action point）。
///   round 8 [P1] fix: 文案回归 round 5 风格 ("登录失败，请重新启动应用"), 不带 "请重试" 前缀
///   —— 因为 RootView 的 .alert 分支现在渲染 TerminalErrorView (无按钮 → 无重试入口),
///   文案不该 promise 一个 UI 不提供的动作.
/// - `.missingCredentials` → TerminalErrorView；文案 "登录信息丢失，请重启 App"（Story 5.4 round 2 fix
///   新增：本地态走"引导冷启动"路径，不该被 toast "正在重登"误导用户以为系统在自动恢复 —— 实际上
///   AuthRetryingAPIClient **不**会 catch 这个 case，需要 cold-start GuestLoginUseCase 接手。
///   本 case 的特点：retry() 也救不回（keychain 真没 token, repo 仍抛同样错误））。
/// - `.network(_)` → RetryView；文案 "网络异常，请检查后重试"。
/// - `.decoding(_)` → TerminalErrorView；文案 "数据异常，请稍后重试"
///   （round 8 [P1] fix: 文案回归简洁形态; user 看到 TerminalErrorView 自然知道要 force-quit,
///   mapper 文案不需重复 "持续失败时杀进程" 提示）.
///
/// **`.alert` vs `.retry` 语义区分（Story 5.5 round 5 [P1+P2] → round 7 → round 8 [P1] 调整）**：
/// - `.alert` = **terminal-class**：mapper 钦定的 alert 表示"client 认为这是 terminal 错误,
///   重试无效". round 8 [P1] fix 后, bootstrap 路径下 RootView 把 `.alert` 渲染为 TerminalErrorView
///   (静态全屏 page, 无按钮, user 必须主动 force-quit). round 5 用 exit(0) / round 7 让 OK 按钮
///   调 retry 都被验证为 dismiss 行为不可调和的反模式 (详见 RootView dismiss 行为迭代史).
///   非 bootstrap 路径仍用 AlertOverlayView (dismiss-able overlay 适合 transient business error).
/// - `.retry` = **transient**：用户可在 App 内点重试自愈（network / business 1005/1007/1008/1009 等瞬时错误）.
/// - `.toast` = **info-level**：非阻塞短提示（mapper 当前不派 toast,留给 ViewModel 自定义场景）.
///
/// 这条二分让 bootstrap 路径可以无脑分发：`.alert` → TerminalErrorView (force-quit-only)；`.retry` → RetryView（重跑 closure）.
/// 详见 docs/lessons/2026-04-27-business-error-transient-vs-terminal.md
/// + docs/lessons/2026-04-27-bootstrap-alert-dismiss-must-be-user-driven-recovery.md
/// + docs/lessons/2026-04-27-bootstrap-terminal-error-static-fallback-page.md.
public enum AppErrorMapper {

    /// transient（可在 App 内重试自愈）类业务码集合.
    /// 选取规则：V1 §3 字典里语义为"瞬时容量/限流/版本冲突/重复操作"的码 —— 这些码在 client 重试时大概率自愈.
    /// 未列入此集合的码默认走 `.alert`（terminal,需重启 App）.
    /// 详见 docs/lessons/2026-04-27-business-error-transient-vs-terminal.md.
    public static let transientBusinessCodes: Set<Int> = [
        1005, // 操作过于频繁,请稍后再试 —— 限流
        1007, // 数据冲突,请重试 —— 乐观锁冲突
        1008, // 操作重复,请稍后再试 —— 幂等键冲突
        1009, // 服务繁忙,请稍后重试 —— server 容量过载
    ]

    /// 把任意 Error 映射成 ErrorPresentation（呈现样式 + 文案）。
    /// 入参 error 必须是 APIError 才走具体分支；其它 Error 类型走 fallback `.alert("操作失败", "请稍后重试")`。
    public static func presentation(for error: Error) -> ErrorPresentation {
        guard let apiError = error as? APIError else {
            return ErrorPresentation.alert(title: "操作失败", message: "请稍后重试")
        }
        switch apiError {
        case let .business(code, message, _):
            let userMessage = localizedMessage(forBusinessCode: code, fallback: message)
            // Story 5.5 round 5 [P1] fix: transient 业务码（1005/1007/1008/1009 等）走 .retry,
            // 让 bootstrap 路径下 1009 "服务繁忙,请稍后重试" 等可恢复错误进 RetryView.
            // Story 5.5 round 8 [P1] fix: permanent 业务码走 .alert, 文案回归简洁形态 (round 5 风格),
            // 不再带 "持续失败时请杀进程重启 App" suffix —— 该指引已 move 到 TerminalErrorView 底部
            // 静态文本 (RootView 把 bootstrap .alert 渲染为 TerminalErrorView, 无按钮 force-quit only).
            // 详见 docs/lessons/2026-04-27-business-error-transient-vs-terminal.md
            // + docs/lessons/2026-04-27-bootstrap-terminal-error-static-fallback-page.md.
            if transientBusinessCodes.contains(code) {
                return ErrorPresentation.retry(message: userMessage)
            }
            return ErrorPresentation.alert(title: "提示", message: userMessage)

        case .unauthorized:
            // Story 5.4 round 5 fix: AuthRetryingAPIClient 上线后,业务层能接到的 .unauthorized
            // 必然是"已经 exhaust 一次静默重登尝试"的场景 —— 此时既没有 relogin 在跑（toast "正在
            // 重新登录" 是谎言），也无法靠点击 retry 在装饰器层自愈（同 generation 的 401 会被
            // dedup 短路返回旧 token，再失败仍走到这里形成 user-perceivable loop）。
            // Story 5.5 round 8 [P1] fix: 文案回归 round 5 风格 ("登录失败，请重新启动应用"),
            // 不带 "请重试" 前缀 —— RootView 的 .alert 分支现在渲染 TerminalErrorView (无按钮 →
            // 无重试入口), 文案不该 promise 一个 UI 不提供的动作.
            return ErrorPresentation.alert(title: "提示", message: "登录失败，请重新启动应用")

        case .missingCredentials:
            // Story 5.4 round 2 fix: 跟 .unauthorized 区分 —— 本地态需要冷启动接手，
            // 不能 toast "正在重登" 误导用户以为后台在自动恢复（AuthRetryingAPIClient 不接管）。
            // 文案天然就明确告知 user 应该重启 App (retry 救不回, keychain 真的没 token).
            return ErrorPresentation.alert(title: "提示", message: "登录信息丢失，请重启 App")

        case .network:
            return ErrorPresentation.retry(message: "网络异常，请检查后重试")

        case .decoding:
            // Story 5.5 round 8 [P1] fix: 文案回归简洁形态 ("数据异常，请稍后重试") —— RootView
            // 把 .alert 渲染为 TerminalErrorView 自带 "请杀进程重启" 静态指引, mapper 不重复.
            return ErrorPresentation.alert(title: "提示", message: "数据异常，请稍后重试")
        }
    }

    /// 抽出**纯文案**（不带 presentation 样式）的对外 helper.
    /// 给非"调 ErrorPresenter"路径用 —— 早期 bootstrap closure 用过本 helper, Story 5.5 round 2
    /// [P1] fix 后 bootstrap closure 改走 `presentation(for:)` 直接拿 ErrorPresentation
    /// （让状态机决定 retry vs alert vs toast）, 本 helper 仍保留供其他可能场景用.
    ///
    /// 历史背景: Story 5.5 codex round 1 [P2] fix bootstrap loadHomeUseCase.execute() 失败时
    /// 原走 `messageFor(error:)` → `APIError.errorDescription`,产出 "Network error: ..."
    /// 等 developer 文案. round 1 fix 把 closure 改用本 helper. round 2 [P1] fix 进一步把
    /// closure 改用 `presentation(for:)` 携带完整样式语义.
    /// 详见 docs/lessons/2026-04-27-bootstrap-error-must-route-via-mapper.md
    /// + docs/lessons/2026-04-27-launch-state-machine-must-carry-presentation.md.
    ///
    /// 实现：复用 `presentation(for:)` 然后从 ErrorPresentation 提取文案部分.
    /// 不直接重复 mapping switch —— 单一 source of truth,以后改 mapper 文案时
    /// 调用方自动跟上.
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
