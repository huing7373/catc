// RootView.swift
// Story 2.2 占位 RootView：直接渲染 HomeView。
//
// Story 2.9 落地 AppLaunchState 时改为根据状态路由到 LaunchingView / HomeView / RetryView。

import SwiftUI

struct RootView: View {
    @StateObject private var homeViewModel = HomeViewModel()

    var body: some View {
        HomeView(viewModel: homeViewModel)
    }
}
