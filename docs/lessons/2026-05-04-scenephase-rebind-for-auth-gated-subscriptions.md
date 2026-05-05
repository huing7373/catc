---
date: 2026-05-04
source_review: codex review round 2 of Story 8-4 (file: /tmp/epic-loop-review-8-4-r2.md)
story: 8-4-主界面猫-sprite-三态动画切换
commit: 28ee7c9
lesson_count: 1
---

# Review Lessons — 2026-05-04 — auth-gated bind 必须挂 ScenePhase rebind 才能覆盖 "用户去 Settings 改权限再回来" 路径

## 背景

Story 8-4 review round 1 P1 fix 给 `HomeViewModel.bind(motionProvider:)` 加了 authorization gate
（未授权时仅持引用、不调 startUpdates，避免 first-launch 弹权限 sheet）。round 2 codex 抓到：
**没人**在用户后续授权（去 Settings 切 Motion 权限后回 app）时再调一次 bind ——
production 路径 `RootView.onAppear` 只触发一次（首次 view appear），随后 RootView 不会重新 .onAppear，
导致 first-launch 未授权的 user 即便后来授权 motion，**petState 也卡在 `.rest` 直到 app relaunch**。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | auth-gated bind 缺 rebind 触发点（first-launch 后授权 path 失效） | P2 | architecture | fix | `iphone/PetApp/App/RootView.swift` |

## Lesson 1: auth-gated 异步资源订阅必须挂 ScenePhase listener，让 background → foreground reactivate 重新尝试 bind

- **Severity**: P2
- **Category**: architecture（生命周期 / 权限链路 / SwiftUI scene）
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift:107-220`

### 症状（Symptom）

- round 1 fix 让 `bind(motionProvider:)` 在未授权时仅持引用 return，不订阅；
- production 调用方仅 `RootView.onAppear` 调一次 bind；
- 用户在同一 app session 中走完 Settings → Motion 授权后回 app 时，
  RootView 不会再触发 .onAppear（SwiftUI scene reactivate 不等于 view reappear），
- 没有任何路径让 `bind(motionProvider:)` 被再次调用 → ViewModel `hasStartedMotionUpdates == false` 永远停在那
  → motion event 永远不到达 → `petState` 卡 `.rest`，要 relaunch 才修。
- round 1 测试新增的 "unauthorized → authorized rebind" 单测覆盖了 ViewModel 升级路径，
  但**生产 wire 中没有谁触发第二次 bind**，单测覆盖空转。

### 根因（Root cause）

SwiftUI 生命周期与 UIKit / OS scene 生命周期分离：

- `View.onAppear` 触发条件：view 第一次进入 view hierarchy / 从 navigation pop 回来 / sheet dismiss 回来；
- **不**在 app reactivate（用户切回 app）时触发——RootView 在 background 中**仍在** view hierarchy 内，只是 scene 处于 .background / .inactive。
- 用户去 Settings App 改权限再回 app 走的是 **scene 生命周期**：`.background → .inactive → .active`。
- 因此任何"授权 / 网络 / 设备资源"类 auth-gated 订阅，如果只挂 `.onAppear` 触发 → 生产只跑一次 → 后续状态变化无 hook。

### 修复（Fix）

在 `RootView` 内加 `@Environment(\.scenePhase)` + `.onChange(of: scenePhase)` modifier：

```swift
@Environment(\.scenePhase) private var scenePhase

var body: some View {
    ZStack { ... }
        .onAppear {
            // 既有 first-launch wire 保留不动
            homeViewModel.bind(motionProvider: container.motionProvider)
            ...
        }
        .onChange(of: scenePhase) { oldPhase, newPhase in
            // 只在 background / inactive → active 边沿触发，避免重复
            guard newPhase == .active, oldPhase != .active else { return }
            homeViewModel.bind(motionProvider: container.motionProvider)
        }
}
```

bind 入口本就 idempotent（hasStartedMotionUpdates short-circuit + auth gate），
重复调用安全：已订阅 → return；仍未授权 → return；首次到达 .authorized → 注册 handler。

不删 `.onAppear` 端 bind（双触发点保险）；ViewModel.bind 行为契约一行不动。

iOS 17+ 用 `.onChange(of:_:)` 两参数 closure（oldPhase / newPhase）让"边沿过滤"语义明确；
如果 deployment target 低于 iOS 17，要切回单参数版 + 自己存 oldPhase。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 SwiftUI **给 ViewModel 挂 auth-gated / device-resource-gated 订阅**（motion / location / health / camera / mic / notifications）时，**必须**在调用方同时挂 `.onAppear` + `@Environment(\.scenePhase)` + `.onChange(of: scenePhase)` 三个触发点；**禁止**只挂 `.onAppear` 一个点然后假设权限不会在 session 中改变。
>
> **展开**：
> - `.onAppear` 不在 scene reactivate 时触发，遗漏"用户去 Settings 改权限再回来"路径。
> - ScenePhase listener 触发条件：`background → active` / `inactive → active`，覆盖用户从其他 app（含 Settings）切回的所有路径。
> - 边沿过滤必须显式写：`guard newPhase == .active, oldPhase != .active else { return }` —— 避免 `.active → .inactive` 反向边沿（如系统通知 banner）误触发。
> - 调用的 bind 必须 idempotent：已订阅短路、未授权短路、首次 .authorized 才走 startXxx；否则重复触发会双倍订阅 / 触发权限弹窗破坏 first-launch 红线。
> - 双触发点设计（`.onAppear` + `scenePhase`）是冗余但安全：tab 切换 / sheet dismiss 走 onAppear，scene reactivate 走 scenePhase，互补。
> - **反例 1**：把 bind 只挂 RootView.onAppear，再写注释说"未授权时调用方 responsible 后续 rebind"——production 没有 responsible 路径就是 silent dead code（本次踩坑）。
> - **反例 2**：把 bind 挂 .task —— `.task` 仅 view 首次构造时跑一次，scene reactivate 不重跑（与 .onAppear 一样的盲区）。
> - **反例 3**：用 NotificationCenter 监听 `UIApplication.willEnterForegroundNotification` —— SwiftUI 已有 ScenePhase 是 idiomatic 解，绕开走 UIKit 通知是 anti-pattern。
> - **反例 4**：写 `.onChange(of: scenePhase) { ... bind() ... }` 不做 `oldPhase != .active` 边沿过滤 → 任何 phase 变化（包括 active → inactive）都触发 bind → 把"重新 try 升级到 authorized"语义破坏成"每次 phase 跳动都重 bind"。
> - **验证方法**：单测覆盖 ViewModel.bind idempotent + auth-gate 行为；生产 wire 验证靠"模拟器实跑：先 deny 权限启动 → 去 Settings 改 Allow → 切回 app → 看 motion event 是否开始驱动 petState"。

### 顺带改动

无 —— 仅 RootView 加 scenePhase listener；ViewModel.bind 入口契约一行不动；其他 ViewModel.bind（appState / loadHome / pingUseCase）不需要 scenePhase rebind（它们不依赖外部权限或运行期变化的资源）。
