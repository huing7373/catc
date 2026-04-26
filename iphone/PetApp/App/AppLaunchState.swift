// AppLaunchState.swift
// Story 2.9: App 启动状态机的三态枚举。
//
// 用途：RootView 根据 launchStateMachine.state 路由到 LaunchingView / HomeView / RetryView。
// 状态流转见 AppLaunchStateMachine.bootstrap() / retry()。
//
// Epic 5 接入说明：
// - .launching：App 启动 → 默认初值 → bootstrap() 在跑
// - .ready：bootstrap() 全部 step 成功完成 → 进入主界面（HomeView）
// - .needsAuth(message:)：bootstrap() 任一 step 抛错 → 进入 RetryView 整页提示
//
// 设计选择：.needsAuth 携带 message 字段（而非用纯 case）—— 让 Epic 5 真实 GuestLoginUseCase /
// LoadHomeUseCase 接入时能把具体错误描述（如 "网络不可达" / "服务器维护中"）透传到 UI；
// 本 story 占位场景下 message 默认 "登录失败，请重试"。

import Foundation

public enum AppLaunchState: Equatable {
    case launching
    case ready
    case needsAuth(message: String)
}
