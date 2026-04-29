// AppErrorMapper.swift
// Story 2.6 AC1：把 APIError 映射到面向用户的呈现样式 + 文案。
//
// 职责：
// - 接收任意 Error，先识别 APIError 各 case，其余走 generic fallback
// - 业务码 → 用户文案：覆盖 V1接口设计.md §3 全部 32 码
// - 不调 logger（→ Story 2.7 落地后再加）
// - 不做 i18n（MVP 阶段全中文 hardcode）
//
// 设计选择：
// - 用 enum + static func：本类是无状态查表器
// - presentation(for:) 接收 Error 而非 APIError：让 ViewModel catch block 不必 narrow
// - 错误码字典放 switch case 而非 dictionary：让 compiler 做穷举检查
//
// **transient vs terminal 二分（ADR-0008 v2 §4.2 钦定为 client 端 presentation heuristic，
//   不升级为协议层契约）**：
// - **transient (.retry)**: .network / .decoding / .localStoreFailure / .business 瞬时码 (1005/1007/1008/1009).
// - **terminal (.alert)**: .business 永久码 (1004/2002/4001 等).
// - **fallback (.retry)**: 非 APIError → 默认走 transient 分支（fallback 无法判定具体子集时，
//   transient possible → retry 比 force-quit 温柔）.
// - **不进 mapper 的 case**: `.unauthorized` / `.missingCredentials` 由 ADR-0008 §6 全局 401
//   catch (`AuthBoundaryAPIClient`) 接管 → 触发 cold-start sink，不走 ErrorPresentation 路径。
//   mapper 仍保留这两个 case 的兜底分支供以下场景用：① 未走 AuthBoundary 装饰器的测试路径；
//   ② 未来若直接显示这两个 error 的非常规路径。
//
// 二分原则: **transient possible → .retry**. user 主动重试失败也只是多发一次请求，
// 比 force-quit 温柔；只有真 terminal (重启都救不了 / 本地配置永久损坏) 才走 .alert。
// 详见 _bmad-output/implementation-artifacts/decisions/0008-error-protocol.md §4.2.

import Foundation

/// `AppErrorMapper`：APIError → ErrorPresentation（呈现样式 + 文案）的映射器。
///
/// 映射规则（ADR-0008 v2 §4.2 钦定的 presentation heuristic 表）：
///
/// - `.business(code, message, _)`:
///   - **transient**（1005/1007/1008/1009 瞬时类）→ `.retry`
///   - **permanent**（其他业务码）→ `.alert`
/// - `.network(_)` → `.retry`
/// - `.decoding(_)` → `.retry`
/// - `.localStoreFailure(_)` → `.retry`（keychain 抛错 = transient sandbox/OSStatus 抽风）
/// - `.unauthorized` → `.alert` 兜底文案（生产路径由 AuthBoundary 接管不进 mapper；保留供测试 / 非常规路径）
/// - `.missingCredentials` → `.alert`（本地 keychain 确认无 token，retry 救不了；保留供测试 / 非常规路径）
/// - **非 APIError fallback** → `.retry`（fallback 无法判定具体子集时默认走 transient）
///
/// **`.alert` vs `.retry` 语义**：
/// - `.alert` = terminal-class：bootstrap 路径渲染 `TerminalErrorView`（无按钮静态全屏，user force-quit）；
///   非 bootstrap 路径走 `AlertOverlayView`（dismiss-able overlay）.
/// - `.retry` = transient：用户可在 App 内点重试自愈.
/// - `.toast` = info-level：mapper 当前不派 toast，留给 ViewModel 自定义.
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
    /// APIError 走具体分支；其它 Error 走 fallback `.retry`（fallback 无法判定具体子集 → 默认 transient）。
    public static func presentation(for error: Error) -> ErrorPresentation {
        guard let apiError = error as? APIError else {
            return ErrorPresentation.retry(message: "操作失败，请重试")
        }
        switch apiError {
        case let .business(code, message, _):
            let userMessage = localizedMessage(forBusinessCode: code, fallback: message)
            if transientBusinessCodes.contains(code) {
                return ErrorPresentation.retry(message: userMessage)
            }
            return ErrorPresentation.alert(title: "提示", message: userMessage)

        case .unauthorized:
            // ADR-0008 v2 §6: 生产路径下 .unauthorized 由 AuthBoundaryAPIClient 全局 catch
            // 触发 cold-start，不进 mapper presentation。本分支保留作为兜底（测试 / 未走装饰器的
            // 非常规路径）。文案与 .missingCredentials 区分：前者 server 拒绝 token，重启可能恢复；
            // 后者本地真无凭证，必须重启走 cold-start。
            return ErrorPresentation.alert(title: "提示", message: "登录已过期，请重启 App")

        case .missingCredentials:
            // 本地 keychain 确认无 token（DI 没配 / 读 nil/空串）—— 真 terminal，
            // 重启 App cold-start 走同一份 KeychainTokenStore 仍读不到，retry 无意义。
            return ErrorPresentation.alert(title: "提示", message: "登录信息丢失，请重启 App")

        case .localStoreFailure:
            // keychain.get 抛错（sandbox 抽风 / OSStatus -25291 等 transient 场景）
            // —— 与 .missingCredentials 区分：前者临时不可用，retry 可能恢复；后者确认无 token。
            return ErrorPresentation.retry(message: "登录信息读取异常，请重试")

        case .network:
            return ErrorPresentation.retry(message: "网络异常，请检查后重试")

        case .decoding:
            return ErrorPresentation.retry(message: "数据异常，请重试")
        }
    }

    /// 抽出**纯文案**（不带 presentation 样式）的对外 helper. 复用 presentation(for:) 防 drift.
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
