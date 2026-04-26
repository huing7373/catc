// PetAppApp.swift
// Story 2.2: SwiftUI App 入口（@main）
//
// Story 2.9 起：RootView 内含 AppLaunchStateMachine 启动状态机；
// 根据 launchStateMachine.state 路由到 LaunchingView / HomeView / RetryView。
// 详见 RootView.swift + AppLaunchStateMachine.swift。

import SwiftUI

@main
struct PetAppApp: App {
    var body: some Scene {
        WindowGroup {
            RootView()
        }
    }
}
