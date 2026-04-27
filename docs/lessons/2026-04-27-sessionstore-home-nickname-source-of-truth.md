---
date: 2026-04-27
source_review: codex /tmp/epic-loop-review-5-2-r1.md（Story 5.2 round 1）
story: 5-2-启动自动登录-usecase
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-27 — SessionStore 写入但视图未订阅会渲染陈旧身份

## 背景

Story 5.2 把游客登录响应写进新引入的 `SessionStore`，并在 `RootView.bootstrapStep1` 闭包内 `await MainActor.run { sessionStore.updateSession(...) }`，但首页 `HomeView` 的昵称仍来自 `HomeViewModel.nickname` 默认值 `"用户1001"`。当真实持久化的 `guestUid` 映射到不同 `user.id`（如 reset 后重登 / 多账号切换）时，启动后主界面会显示与已登录身份不一致的昵称。codex round 1 [P1] 把它定性为"main feature 引入的 user-facing correctness 问题"，必须在本轮修。

Story file 第 75 行原本声明"本 story 不动 view source —— Story 5.5 才会用 LoadHomeUseCase 填真实 nickname"。我们的判断：单一行 server response → SessionStore 的写入路径已建好，但展示侧不接 = 写入对用户不可见，问题域属于本 story 引入。让 HomeView **以最小破坏面订阅 SessionStore** 是 Story 5.2 范围内的合理收尾，而不是全量重构 HomeViewModel（那才是 5.5 的事）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | guest-login profile 未投影到首页渲染态 | high | architecture | fix | `iphone/PetApp/App/RootView.swift:166-174`、`iphone/PetApp/Features/Home/Views/HomeView.swift:55-76` |

## Lesson 1: SessionStore 写入但视图未订阅会渲染陈旧身份

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift:167-170`（write 端）、`iphone/PetApp/Features/Home/Views/HomeView.swift` userInfoBar（read 端）

### 症状（Symptom）

`bootstrapStep1` 闭包成功后调 `sessionStore.updateSession(SessionState(user: output.user, pet: output.pet))`，`@Published session` 已正确更新，但首页顶栏继续显示 `HomeViewModel.nickname` 的硬编码默认值 `"用户1001"`。任何 `guestUid → user.id` 不是 `1001` 的真实账号在登录成功后都看到错误身份；reset 身份后重登 / 多账号场景下尤其明显。

### 根因（Root cause）

引入新 `SessionStore` 时，**只完成了"领域状态写入"，没有同步把"展示状态读取"切到新源**。具体几个反模式：

1. **写入路径与读取路径双源**：`SessionStore.session.user.nickname`（真源）与 `HomeViewModel.nickname`（占位默认值）并存，且渲染侧（HomeView）继续读后者。`@Published` 的更新只能让"已订阅 publisher 的 view"刷新；HomeView 没订阅 = 写多少次都没用。
2. **story file 的"view source 留到下个 story"红线在 SessionStore 写入也属于本 story 的前提下站不住**：可以推迟"接 LoadHomeUseCase 改 ViewModel 内部状态"，但**不可推迟"展示侧能看到本 story 引入的写入"**。否则等于本 story 引入了一份对用户不可见的状态机。
3. **AC4 / AC7 的注释意图是"HomeView 通过 @ObservedObject 订阅 sessionStore.session"**，dev-story 只兑现了 AC4 / AC7 写入侧、忽略了 AC4 注释里"HomeView 订阅"这半句。

### 修复（Fix）

`HomeView` 加新 init 接受 optional `SessionStore`；userInfoBar 通过 `@ViewBuilder` 分发到两个 fileprivate 子视图（`SessionAwareUserInfoBar` 用 `@ObservedObject` 订阅，`StaticUserInfoBar` 走 fallback），nickname 决策抽到 `HomeNicknameResolver.resolve(session:fallback:)` 纯函数 helper 便于单测。

- `iphone/PetApp/Features/Home/Views/HomeView.swift`：
  - 新 init `init(viewModel:resetIdentityViewModel:sessionStore:)`，sessionStore 为 optional
  - 老两个 init 保持签名不变（Preview / 老测试 / UITest skip-guest-login 路径零回归 —— sessionStore 透传 nil → fallback 到 `viewModel.nickname`）
  - userInfoBar 改为 `@ViewBuilder`，根据 sessionStore 是否 nil 分发到 `SessionAwareUserInfoBar` 或 `StaticUserInfoBar`
  - 抽 `UserInfoBarLayout` 共享视觉 / a11y 实现，保证两条路径结构对齐
  - 新增 `public enum HomeNicknameResolver` 纯函数 helper（`resolve(session:fallback:) -> String`）
- `iphone/PetApp/App/RootView.swift`：`homeView` 私有 ViewBuilder 在 Debug / Release 两条分支都传 `sessionStore: container.sessionStore`
- 新增 `iphone/PetAppTests/Features/Home/HomeNicknameResolverTests.swift`：3 个 case 锁住 happy（session 优先）/ fallback（session=nil）/ edge（session.nickname 为空字符串仍按 session 显示）

为何**不**直接重构 HomeViewModel 接 SessionStore：那是 Story 5.5 LoadHomeUseCase 的范围（届时会同步引入 stepAccount snapshot / chest 状态等多字段，需要重新设计 HomeViewModel 的源），本 story 提前重构会和 5.5 打架。当前修法把"nickname 显示决策"局部在 HomeView 内部分流，不动 ViewModel 内部状态，5.5 时再统一收口。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**引入新 ObservableObject 状态源**时，**必须在同一 PR / story 内**至少把**一个真实视图订阅它**（`@ObservedObject` / `@StateObject` / `@EnvironmentObject`），否则不视为"该状态已落地"。
>
> **展开**：
> - "状态写入"和"状态投影到视图"是同一件事的两半，**不可拆 story**。可以推迟"用新状态做复杂业务渲染"（如 step 进度条 / chest 倒计时），但**不可推迟"让用户能看到刚才写入的最起码字段（如 nickname / id）"** —— 否则上线后用户看到的还是占位值，整个状态源等于死代码。
> - SwiftUI 中 `@Published` 的更新**只对已订阅 publisher 的 view 生效**；持有 `let store: SessionStore?` 不会让外层 view 随 `store.session` 变化重渲染。订阅必须发生在 `@ObservedObject` / `@StateObject` 修饰符下。
> - `@ObservedObject` 不接受 Optional —— 父 view 持 `Store?` 时，把"订阅生命周期"下沉到子视图：父 view 在 `@ViewBuilder` 内 `if let store = store { ChildSubscriber(store: store, ...) } else { Fallback(...) }`。`ChildSubscriber.@ObservedObject` 收紧为 non-optional store。同模式：本 repo 的 `ResetIdentityButton`、`LaunchedContentView`。
> - 决策类纯函数（"session 非 nil 用 session.nickname，否则 fallback"）抽出为 `enum`/`struct` 的静态方法（如 `HomeNicknameResolver.resolve(...)`），让单测可以 covering 无需 SwiftUI 渲染（XCTest only 的项目尤其重要 —— 没有 ViewInspector 时 view body 内的逻辑很难直接断言）。
> - **Story-file 的"view source 留到下一个 story"红线**只在"该 story 没引入新展示状态"时成立；本 story 引入了 SessionStore = 引入了新展示状态 = 必须同步改一处展示。否则 story 验收等于"写了一份没人读的状态"。
> - **反例**：
>   - 在 RootView 写 `let stored = container.sessionStore` 然后 `Text(stored.session?.user.nickname ?? "用户1001")` —— 不会订阅 publisher，session 变化后不重渲染。
>   - 把 SessionStore 注入进 HomeViewModel 让 ViewModel 转发 `@Published nickname` —— 跨 story 重构（HomeViewModel 5.5 还要再大改一次），违反"最小改动 + 不打架未来 story"。
>   - 在 HomeView body 顶层写 `@ObservedObject var sessionStore: SessionStore?` —— 编译错误，`@ObservedObject` 不支持 Optional。

## Meta: 本次 review 的宏观教训

引入新 ObservableObject 类型时，**写入**和**至少一个 view 订阅**是**同一个验收单元**。Story file 可以划分"哪个 view 怎么用这个状态"的 scope（如"5.5 才补步数显示"），但不能让"任何 view 都不订阅" —— 否则状态源就是死代码，code review 一定会抓。审视本 story 的 AC 时，应在 AC4（SessionStore）/ AC7（container 持有）/ AC8（bootstrap wire）之外补一条隐性 AC："至少一个 view 订阅 sessionStore.session 并展示某字段"。这条隐性 AC 在未来引入 `RoomStore` / `StepStore` / `ChestStore` 等新类型时同样成立。
