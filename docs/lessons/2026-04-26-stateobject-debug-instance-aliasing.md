---
date: 2026-04-26
source_review: codex review round 1 on Story 2.8 (file: /tmp/epic-loop-review-2-8-r1.md)
story: 2-8-dev-重置-keychain-按钮
commit: 3e5ad68
lesson_count: 1
---

# Review Lessons — 2026-04-26 — SwiftUI @StateObject init 阶段构造的 standalone container 与 RootView container 是别名陷阱

## 背景

Story 2.8 给 dev build 加了"重置身份"按钮：点一下调 `ResetKeychainUseCase` 把 `KeychainStore` 清空，
模拟首次安装。RootView 同时持有 `@StateObject container = AppContainer()` 与一个新的
`@StateObject resetIdentityViewModel`。

`@StateObject` 必须在属性 init 阶段给值，而 `container` 在 RootView init 阶段还没被 SwiftUI 实体化
（property wrapper 延迟构造），因此**不能**在 init 写 `_resetIdentityViewModel = StateObject(wrappedValue: container.makeResetIdentityViewModel())`。

dev sub-agent 当时选择"用 standalone `AppContainer()` bootstrap 喂初值"——以为两个 container 都用默认
`InMemoryKeychainStore()` type 所以"功能等价"。

codex round 1 [P1] 直接点穿：standalone container 的 `InMemoryKeychainStore` 是**另一个字典实例**，
与 `container.keychainStore` 不共享数据。当 App 真实写 keychain（→ Epic 5 实装时），写到的是
`container.keychainStore`；用户点"重置身份"，触发的 `useCase.execute()` 调的是 standalone container 那份。
UI 弹"已重置"成功 alert（standalone 字典确实清了），但 `container.keychainStore` 里数据**仍在**——重置功能形同虚设。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | resetIdentityViewModel 用 standalone container 实例化导致与 RootView container 的 keychainStore 不共享 | high (P1) | architecture | fix | `iphone/PetApp/App/RootView.swift:34-39` |

## Lesson 1: SwiftUI @StateObject 不能 lazy 初始化时，跨 container 共享 instance 必须靠 .onAppear lazy 注入而非 init 阶段 standalone 构造

- **Severity**: high (P1)
- **Category**: architecture（依赖图错位 → 功能性 bug）
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift`（去掉 `init() { let bootstrap = AppContainer(); _resetIdentityViewModel = ... }`，改 `@State Optional` + `.onAppear` 内 nil-guard 注入）

### 症状（Symptom）

```swift
// 早期实装（错）
struct RootView: View {
    @StateObject private var container = AppContainer()              // → keychainStore A
    #if DEBUG
    @StateObject private var resetIdentityViewModel: ResetIdentityViewModel
    #endif

    init() {
        #if DEBUG
        let bootstrap = AppContainer()                                // → keychainStore B（独立字典）
        _resetIdentityViewModel = StateObject(wrappedValue: bootstrap.makeResetIdentityViewModel())
        #endif
    }
}
```

实际后果：
1. App 业务路径：写值到 `container.keychainStore`（实例 A 的字典）
2. 用户点重置：`resetIdentityViewModel` → `useCase.execute()` → `keychainStore B.removeAll()`
3. `container.keychainStore` （实例 A）**仍持有数据**
4. UI 弹 `.success` alert（B 的 removeAll 真清了），但 App 实际状态没动

dev 当时贴的注释"两个 container 都用同一个 default `InMemoryKeychainStore()` type，功能等价"——**错把 type
等同 instance**。Default 参数 `keychainStore: KeychainStoreProtocol = InMemoryKeychainStore()` 是**调用时
求值**，每次 init 创建新字典。

### 根因（Root cause）

两层认知漏洞叠加：

1. **`@StateObject` 必须 init 阶段给值** —— 这是 SwiftUI 限制（property wrapper 不能 lazy）。dev 知道这点
   并据此选 init 阶段构造。
2. **没识别出"init 阶段还没有 self.container"是更深的问题** —— RootView 的 `container` 自己也是
   `@StateObject`，init 期间未实体化，`init() { ... container.make... }` 编译就过不去（属性顺序 + property
   wrapper 没就绪）。dev 绕开方式是另起一个 `let bootstrap = AppContainer()` —— 这一步让两个对象**永久分叉**。

类比：本想"复制 reference"实际"复制 type 然后按 type 构造一个独立 instance"，违反引用语义。

抽象规则：**当一个属性需要从另一个 `@StateObject` 拿 reference 实例化，init 路径行不通；必须切到运行时
（`.onAppear` / `.task`），用 `@State Optional` 或 `@StateObject + bind()` 模式**。早期 lesson
`2026-04-26-stateobject-init-vs-bind-injection.md` 描述的是 ViewModel 内部的 bind 模式（适合 ObservableObject
单例化场景）；本 lesson 是它的补集——dev 工具/可选子组件场景，更轻量的 `@State Optional + onAppear` 注入
更合适（避免给业务 ViewModel 强加 bind() 模板）。

### 修复（Fix）

```swift
// 修复后
struct RootView: View {
    @StateObject private var container = AppContainer()

    #if DEBUG
    @State private var resetIdentityViewModel: ResetIdentityViewModel?
    #endif

    var body: some View {
        homeView
            .onAppear {
                wireHomeViewModelClosures()
                #if DEBUG
                if resetIdentityViewModel == nil {
                    resetIdentityViewModel = container.makeResetIdentityViewModel()
                }
                #endif
            }
            // ...
    }

    @ViewBuilder
    private var homeView: some View {
        #if DEBUG
        HomeView(viewModel: homeViewModel, resetIdentityViewModel: resetIdentityViewModel)
        #else
        HomeView(viewModel: homeViewModel)
        #endif
    }
}
```

关键点：
- `@State Optional`：不要求 init 阶段给值，nil 默认。
- `.onAppear` 内 `nil` 守卫：第一次出现时构造，后续 RootView 重建（如旋转 / 离开返回）保持既有 instance。
- `HomeView` 已支持 `Optional` 参数（按钮在 nil 时不渲染），短暂 nil 期对 dev 工具 UX 无影响。

回归测试 `AppContainerTests.testResetIdentityViewModelSharesContainerKeychainStore`：

```swift
let container = AppContainer()
try container.keychainStore.set("test-token", forKey: "sessionToken")
let viewModel = container.makeResetIdentityViewModel()
await viewModel.tap()
XCTAssertNil(try container.keychainStore.get(forKey: "sessionToken"))
```

直接通过 container factory 验证"reset 影响 container 自己的 keychainStore"——若 ViewModel 拿到别的实例，
container 这份还会保留 `"test-token"`，断言挂掉。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 SwiftUI View 里需要"基于另一个 `@StateObject` 创建依赖子 ViewModel"时，
> **禁止**在 `init()` 里临时构造一个 standalone 同类型对象当 bootstrap，**必须**用
> `@State Optional + .onAppear` lazy 注入，让子 ViewModel 与父 `@StateObject` 共享同一 reference instance。

> **展开**：
>
> - **触发条件**：当一个 ViewModel / Service 持有 reference type 内部状态（class 字典 / class 缓存 / class
>   连接池），且需要被多个 SwiftUI 节点共享时，"用 default 参数构造另一个实例" = 制造别名分叉，**必然**导致
>   状态不同步。识别信号：`AppContainer()` / `XxxRepository()` / `KeychainStore()` 等无参 init 在两个地方
>   出现（`@StateObject` 一个 + `init` 内 bootstrap 一个），就是该警觉的时刻。
>
> - **审查清单**：
>   - 我新加的 `@StateObject` 字段，init expression 里有没有引用其他 `@StateObject`？有 → 不行，必须改 lazy。
>   - 我有没有"为了 init 阶段给值"另起一个 `let bootstrap = ContainerType()` ？有 → 99% 是 bug，因为 bootstrap
>     与 `@StateObject` 的 container 是两个 instance。
>   - 我能不能用 `@State Optional` + `.onAppear` 替代？多数情况能（dev 工具 / 可选子组件场景），且 SwiftUI
>     按值 diff，nil → non-nil 触发重新渲染，不会丢更新。
>   - 子 ViewModel 用的 `@StateObject` 还是 `@ObservedObject`？如果父用 `@State Optional`，子组件应改
>     `@ObservedObject`（已 own 引用，不需要再 own）。
>
> - **测试策略**：写"双 instance 共享"的回归测试不能停留在"两个对象都是同一 type"——必须验证**状态可见性
>   传递**：通过 ref A 写值 → 通过 ref B 读值，期望读到。本 story 的回归测试就是这个范式。
>
> - **反例 1**："default 参数 `keychainStore = InMemoryKeychainStore()` 让两边都用默认值，所以等价" —— 错。
>   default 参数表达式是**调用点求值**，每次 init 跑一遍，每次都新建。要 alias，必须显式传入同一 ref。
>
> - **反例 2**："Singleton 模式 `static let shared = InMemoryKeychainStore()`" —— 不要这么改。违反 ADR-0002
>   §3.1 "测试隔离"原则（mock 替换困难），且会让 release / dev / test 共享状态泄露，得不偿失。
>
> - **反例 3**："改 PetAppApp 显式持有 AppContainer 然后 init 传给 RootView" —— 改动面太大，且 SwiftUI App
>   protocol 的 body 计算属性每次访问都跑，反而引入新别名风险。`@State Optional + .onAppear` 是最小可行方案。
>
> - **替代方案的边界**：如果子 ViewModel 必须 `@StateObject`（强生命周期管理 + ObservableObject 订阅），
>   走"双 init + bind() 注入"模式（lesson `2026-04-26-stateobject-init-vs-bind-injection.md`）。本 lesson
>   解决的是"轻量 dev 工具 / 可选子组件"场景：状态机短，dispose 也短，`@State Optional` 更轻。
>
> - **dev 工具特别警告**：`#if DEBUG` 包裹的 dev 工具特别容易出这种问题——因为它们走的不是主业务路径，
>   review 时容易因为"反正只有 debug 用"放松审查。但 dev 工具一旦失效，下游所有"通过 dev 工具验证 epic
>   功能"的 demo 流程都会基于错误前提。**dev 工具的依赖图与业务依赖图必须同等严肃对待**。

### 顺带改动

- `iphone/PetApp/App/RootView.swift`：删 `init()`，`@StateObject` 改 `@State Optional`，`.onAppear` 内
  nil-guard 注入；文件头注释更新设计选择说明，指向本 lesson。
- `iphone/PetAppTests/App/AppContainerTests.swift` 新增 `#if DEBUG` 包裹的
  `testResetIdentityViewModelSharesContainerKeychainStore`：通过 container.keychainStore 写值 → 经
  ViewModel.tap() 验证清空，本 case 是 [P1] 修复的防回归断言。
- `ResetIdentityViewModelTests` 不变（仍是"传入的 mock UseCase"注入级测试，与本 lesson 关心的
  "container 与 ViewModel 共享 store"是正交的两个测试维度）。
