---
date: 2026-04-27
source_review: codex /tmp/epic-loop-review-5-2-r2.md（Story 5.2 round 2）
story: 5-2-启动自动登录-usecase
commit: 9ed4f97
lesson_count: 1
---

# Review Lessons — 2026-04-27 — Reset 类操作必须同步清空 in-memory session 状态

## 背景

Story 5.2 round 1 已经把 `HomeView` 的 nickname 改成"优先订阅 `SessionStore.session`，session 为 nil 时 fallback 到 `HomeViewModel.nickname`"（见 lesson 2026-04-27-sessionstore-home-nickname-source-of-truth.md）。但 dev "重置身份" 按钮（Story 2.8 落地）调用链 `ResetIdentityViewModel.tap → ResetKeychainUseCase.execute → keychainStore.removeAll` **只清 Keychain，没清 `SessionStore.session`**。结果是：开发者按下重置后，`HomeView` 的 SessionAwareUserInfoBar 仍订阅着 `sessionStore.session`，于是继续渲染 reset 前的旧 nickname/avatar 直到 kill app 才清空。codex round 2 [P2] 把它定性为"由本 story 引入的 user-visible inconsistency"——因为 `session` 现在已是 UI source of truth，reset 路径成了漏修的反向边。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Clear the in-memory session after Reset Identity | P2 (≈ medium) | architecture | fix | `PetApp/Features/DevTools/ViewModels/ResetIdentityViewModel.swift` / `PetApp/App/AppContainer.swift` |

## Lesson 1: Reset 类操作必须同步清空 in-memory session 状态

- **Severity**: medium（P2，user-visible 但 dev-only 路径）
- **Category**: architecture
- **分诊**: fix
- **位置**: `PetApp/Features/DevTools/ViewModels/ResetIdentityViewModel.swift:32-39`、`PetApp/App/AppContainer.swift:171-182`

### 症状（Symptom）

按下"重置身份"按钮：alert 显示"已重置"成功，但 `HomeView` 顶部 nickname 仍是 reset 前的旧值；只有 kill app + 重启才会真正变回 fallback `"用户1001"`。

### 根因（Root cause）

引入 `SessionStore` 当 UI source of truth 时，**所有以前只动持久化层（Keychain）的清理路径都漏修了 in-memory mirror**：

- Story 2.8 落地 reset 按钮时 `SessionStore` 还不存在，那时 keychain 是唯一身份载体，`removeAll()` 清完即整链清空。
- Story 5.2 round 1 引入 `SessionStore` 并让 `HomeView` 订阅；
  但 reset 链路 `ResetKeychainUseCase.execute` 仍只调 `keychainStore.removeAll()`，没调 `sessionStore.clear()`。
- 持久化层 (Keychain) 与内存表征 (SessionStore) 是**两条独立的状态线**：
  写入侧 `GuestLoginUseCase` 由 `RootView.bootstrapStep1` 协调（先 keychain 后 sessionStore），
  清除侧的对称协调被遗漏，导致两条线不再同步。

可类比的反面是 GuestLoginUseCase 的设计：UseCase 故意**不**直接调 `SessionStore.updateSession`，
由 RootView 闭包协调；既然写入侧已用"协调器"模式，清除侧就该有同样的协调点。

### 修复（Fix）

在**清除侧**对称地补一个协调点：让 `ResetIdentityViewModel`（已是 `@MainActor`）持有可选的 `SessionStore`，在 `useCase.execute()` **成功后**额外调 `sessionStore?.clear()`。`AppContainer.makeResetIdentityViewModel()` 注入 `container.sessionStore` 同一 instance，保证清的是 App 实际订阅的那份。

```swift
// ResetIdentityViewModel.swift（节选）
public init(useCase: ResetKeychainUseCaseProtocol, sessionStore: SessionStore? = nil) {
    self.useCase = useCase
    self.sessionStore = sessionStore
}

public func tap() async {
    do {
        try await useCase.execute()
        sessionStore?.clear()       // 新增：keychain 清完才清内存表征（fail-open）
        alertContent = .success
    } catch {
        alertContent = .failure(message: "重置失败：\(error.localizedDescription)")
    }
}
```

```swift
// AppContainer.swift（节选）
public func makeResetIdentityViewModel() -> ResetIdentityViewModel {
    ResetIdentityViewModel(
        useCase: makeResetKeychainUseCase(),
        sessionStore: sessionStore     // 注入同一 instance
    )
}
```

设计选择说明：

- `sessionStore` 为 **Optional 默认 nil**：保留 Story 2.8 老测试 `init(useCase:)` 签名兼容，
  避免 mass-update 既有 ViewModel 单测；新增 4 个单测覆盖 with/without sessionStore 三态 + 失败路径不清。
- **不**让 `ResetKeychainUseCase` 持有 SessionStore：UseCase 是 `Sendable struct`，让它跨 actor 持有 `@MainActor class` 引用会破坏 Sendable 契约；
  且与对称侧 `GuestLoginUseCase` 不调 SessionStore 的设计保持一致（UseCase 单一职责，协调由调用方做）。
- **顺序**：先 `useCase.execute()` 后 `sessionStore.clear()`——失败时不清 session（fail-open），避免 keychain 没清成功但内存被错误置 nil。
- 测试新增：
  - `ResetIdentityViewModelTests.testTapHappyPathClearsSessionStore`（happy）
  - `ResetIdentityViewModelTests.testTapErrorPathDoesNotClearSessionStore`（fail-open 兜底）
  - `ResetIdentityViewModelTests.testTapWorksWithoutSessionStore`（兼容老 init 签名）
  - `AppContainerTests.testResetIdentityViewModelClearsContainerSessionStore`（container 注入正确性）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在引入新的 in-memory 状态镜像（observable store / cache / session）作为某条 UI 路径的 source of truth 时，**必须**同步审计**所有**清除/重置/退出登录路径，逐条确认是否需要补一个对称的 `store.clear()` 调用。
>
> **展开**：
> - **状态镜像引入清单**：每当新增一个"持久层 ↔ 内存表征"的镜像（如 `SessionStore` 镜像 Keychain 的 token / userId），必须**同时**列出它的所有突变路径——写入侧 (login / refresh) 和清除侧 (reset / logout / silent relogin failure / token expired)，**两侧都协调过才算完成**。
> - **写入侧已用协调器（如 RootView closure 协调 useCase + store）→ 清除侧必须有同样的协调点**：要么在 ViewModel 层（如 ResetIdentityViewModel），要么在另一个 closure / coordinator；不要让 UseCase 自己跨 actor 持有 store（会破坏 Sendable）。
> - **失败路径不清**：写入失败时 store 不应被半提交；清除失败时 store 不应被错误置 nil。Reset 这类破坏性操作尤其要 fail-open——keychain 没清成功就保留 session，让用户看到 "已重置" 失败 alert 而不是看到 home 上一片空白。
> - **测试 framework**：每条 store-coordinated UseCase ViewModel 至少 3 个 case：
>   1) happy path：UseCase 成功 → store 状态被正确改；
>   2) error path：UseCase 抛错 → store 状态保持原值；
>   3) container factory wiring：通过 `container.makeXxxViewModel()` 拿到的 ViewModel 改的是 `container.xxxStore` 同一 instance（防 standalone instance aliasing 反例，参考 lesson 2026-04-26-stateobject-debug-instance-aliasing.md）。
> - **反例**：在 `SessionStore` 这种 in-memory observable holder 引入后，只改写入侧（login）就 ship，没在 reset / logout 路径补 `clear()`——典型表现是 "操作显示成功但 UI 不刷新，必须杀进程"。本 lesson 即此反例的修复实录。
> - **Cross-link**：与 lesson `2026-04-27-sessionstore-home-nickname-source-of-truth.md`（写入侧让 HomeView 订阅 SessionStore）配对——那条解决"写了不显示"，本条解决"清了不显示"，构成 SessionStore 生命周期的两半。

## Meta: 本次 review 的宏观教训

引入 SessionStore 这种 "in-memory mirror of persisted state" 抽象时，最常见的漏洞模式是**只想着 happy 主流程的写入路径**，忘记主流程之外的破坏性 / 恢复性路径（reset、logout、silent relogin failure、token expired）。round 1 解决了 "写了 HomeView 不显示"，round 2 解决了 "清了 HomeView 不刷新"——这两轮其实是同一个根因的两半：**source of truth 切换时必须做完整的"写 + 清 + 失败回退" 三向审计**，单做一向就 ship 必有漏洞。
