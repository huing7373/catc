// AppLaunchState.swift
// Story 2.9: App 启动状态机的三态枚举。
//
// 用途：RootView 根据 launchStateMachine.state 路由到 LaunchingView / HomeView / 错误 UI。
// 状态流转见 AppLaunchStateMachine.bootstrap() / retry()。
//
// Epic 5 接入说明：
// - .launching：App 启动 → 默认初值 → bootstrap() 在跑
// - .ready：bootstrap() 全部 step 成功完成 → 进入主界面（HomeView）
// - .needsAuth(presentation:)：bootstrap() 任一 step 抛错 → 进入错误 UI（RetryView / AlertOverlayView /
//   ToastView 之一，由 presentation 决定）
//
// 设计选择：
// - .needsAuth 携带 `ErrorPresentation`（**Story 5.5 round 2 [P1] fix**）—— 而非纯 String message。
//   原方案 `.needsAuth(message:)` 把所有失败一律渲染 RetryView, 但 AppErrorMapper 把
//   .unauthorized / .missingCredentials / decoding 分类为 .alert（带"请重启应用" guidance），
//   collapse 到 retry 屏会让用户卡在 unrecoverable retry loop（点 retry 仍 401, 反复弹 retry）。
//   把 ErrorPresentation 三态（toast/alert/retry）下放到 state 让 RootView 直接路由不同 UI,
//   单一 source of truth 由 mapper 决定, RootView 不再分散判断 alert vs retry.
// - 状态机内部 fallback 用 `.retry(message: defaultFailureMessage)`：未知错误优先给用户重试入口
//   （比"卡 alert + 不可恢复"更宽容）；已知 .alert 类错误由 BootstrapMappedError 显式携带 presentation
//   传入, fallback 不会误降级.
// - 详见 docs/lessons/2026-04-27-launch-state-machine-must-carry-presentation.md.

import Foundation

public enum AppLaunchState: Equatable {
    case launching
    case ready
    case needsAuth(presentation: ErrorPresentation)
}
