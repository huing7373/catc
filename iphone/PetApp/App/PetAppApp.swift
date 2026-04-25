// PetAppApp.swift
// Story 2.2: SwiftUI App 入口（@main）
//
// 当前 RootView 直接渲染 HomeView。
// Story 2.9 改为路由 LaunchingView / HomeView / RetryView。

import SwiftUI

@main
struct PetAppApp: App {
    var body: some Scene {
        WindowGroup {
            RootView()
        }
    }
}
