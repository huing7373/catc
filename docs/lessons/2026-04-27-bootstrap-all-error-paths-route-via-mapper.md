---
date: 2026-04-27
source_review: codex round 4 review of Story 5.5 (file: /tmp/epic-loop-review-5-5-r4.md)
story: 5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据
commit: <pending>
lesson_count: 3
---

# Review Lessons — 2026-04-27 — bootstrap 全部错误路径必经 mapper / ping 复活回 .ready 分支 / alert-only dismiss 不能隐式 retry

## 背景

Story 5.5 的第 4 轮 codex review。前 3 轮 fix-review 在修一条 finding 时把另一条规则破坏掉，形成"修一处坏一处"的 regression 链：

- round 1：把 LoadHome 失败包成 BootstrapMappedError 经 mapper（漏了 guest-login）
- round 2：把状态机 .needsAuth(message:) 升级为 .needsAuth(presentation:) 三态分发
- round 3：把启动期 ping 调用整个删掉控 cold-start HTTP 预算 ≤2（删过头，serverInfo 永远停在 placeholder）

round 4 的三条 P2 finding 全部是上述 round 1-3 修复的 regression。本轮是 epic-loop 设定的最后一轮 fix-review 机会，必须一次修干净。本 lesson 主旨不光是"这三条 finding 怎么修"，更要把"启动失败/重试/恢复"这五个交互节点的 mental model 钉死，让未来 Claude 1 分钟回查正确性。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | guest-login 失败必须经 AppErrorMapper（与 LoadHome 一致） | P2 | error-handling | fix | `iphone/PetApp/App/RootView.swift:201` |
| 2 | ping 复活 — 移到 .ready 分支异步触发，不再启动期短路 | P2 | architecture (regression) | fix (option A) | `iphone/PetApp/App/RootView.swift:124-145` |
| 3 | alert-only dismiss 不能隐式调 retry | P2 | error-handling | fix (option B) | `iphone/PetApp/App/RootView.swift:416-420` |

## Lesson 1: guest-login 失败必须经 AppErrorMapper（与 LoadHome 一致）

- **Severity**: P2
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift:201`

### 症状

bootstrap step1 closure 内 `useCase.execute()` (guest-login) 抛 raw `APIError`，没有经 `BootstrapMappedError` 包装。AppLaunchStateMachine 的 `presentationFor(error:)` 只识别 `BootstrapMappedError` → 拿 mapper 的 presentation；其他 LocalizedError 走 `errorDescription` fallback → 弹出 "Network error: ..." / "Business error 1009: ..." 等 developer-facing 串。同 closure 内 LoadHome 失败已被 round 1 修复包装，但 guest-login 失败被遗漏。

### 根因

round 1 修复 LoadHome 路径时，"修哪儿包哪儿"的局部思路。没有把"所有抛给状态机的 error 必须经 mapper"提升到 closure 层级的不变式。结果是同一个 closure 内有两条 throw 路径，一条经 mapper、一条直接 raw —— 视野没扫齐。

### 修复

把 guest-login 的 `useCase.execute()` 也用 `do/catch` 包成 `BootstrapMappedError`，与 LoadHome 路径完全一致：

```swift
let output: GuestLoginOutput
do {
    output = try await useCase.execute()
} catch {
    throw BootstrapMappedError(
        presentation: AppErrorMapper.presentation(for: error),
        underlying: error
    )
}
```

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **bootstrap step closure 内任何抛给 AppLaunchStateMachine 的 error**，**必须**经 `BootstrapMappedError(presentation: AppErrorMapper.presentation(for:), underlying:)` 包装。
>
> **展开**：
> - `presentationFor(error:)` 的 fallback (LocalizedError errorDescription / defaultFailurePresentation) 是"理论上不该走到"的兜底，不是产品文案路径。
> - bootstrap closure 内有 N 条 `try await xxxUseCase.execute()`，必须有 N 条对应的 `do/catch` + BootstrapMappedError 包装；不能只对其中一条做。
> - **反例**：`let output = try await useCase.execute()` 直接传播 raw APIError 给状态机；只对 LoadHome 一条做包装而 GuestLogin 一条直传 — 是修一处漏一处的典型。
> - 检查清单：每次改 bootstrap closure 时，grep 一下所有 `try await` 调用，逐一对照是否经过 BootstrapMappedError 包装。如果新增了 step1c / step1d，必须加同样的 catch 块。

## Lesson 2: ping 复活 — 移到 .ready 分支异步触发，不再启动期短路

- **Severity**: P2
- **Category**: architecture (regression)
- **分诊**: fix (选项 A — 移到 .ready 分支异步触发)
- **位置**: `iphone/PetApp/App/RootView.swift:124-145` + `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift:201-223`

### 症状

round 3 fix 删除了 RootView 启动 `.task` 内 `await homeViewModel.start()` 调用，并在 `start()` 内加 `if hasLoadedHome { return }` 第 4 层短路。结果是 ping 永远不被调用，`serverInfo` 永远停在初始 `"----"` placeholder，版本 footer 完全失效。round 3 修复"省一个 HTTP"的目标达成了，但代价是版本 footer 整个废掉 —— 不可接受的 trade-off。

### 根因

round 3 把约束 ("cold-start ≤ 2 HTTP") 从"启动链路"误解到了"App 整个生命周期"。spec 钦定的是"用户从冷启动到看到主界面"这段路径不能超过 2 个 HTTP；并不是说 ping 永远不能发。删除 `start()` 调用 + 加短路两件事都是为了同一个目标（防 ping 发出来），所以两件事都要回滚才能让 ping 真正复活。

第二个根因：缺少 "footer 显示什么内容" 的 acceptance test 兜底，导致这种"删干净了但功能消失"的 regression 没有自动测试网拦截。

### 修复

两步动作必须配对：

**RootView**：把 `start()` 调用从启动 `.task` 挪到 `.ready` 分支的 `onReadyTask` 闭包（在 LaunchedContentView 的 `.ready` case 内 `.task` 触发）：

```swift
case .ready:
    homeView()
        .onAppear { onReadyAppear() }
        .task {
            await onReadyTask()  // round 4: 调 homeViewModel.start() 发 ping
        }
```

**HomeViewModel.start()**：移除 round 3 引入的 `if hasLoadedHome { return }` 第 4 层短路。保留前 3 层（hasFetched / pingTask / 未注入 useCase 短路）。

cold-start HTTP 预算 (≤2) 保持不变：ping 只在 `.ready` 之后才发，是用户已经看到主界面后才悄悄填的版本号 — 不计入启动链路。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **为了控 HTTP 预算删除某个 nice-to-have UseCase 调用**时，**必须**思考"这个 UseCase 的 side-effect (UI 显示哪些字段) 谁来填"，而不是"简单删了就完事"。
>
> **展开**：
> - "省一次请求" vs "失去一个 UI 状态" 是两个独立维度，不能混淆。HTTP 预算约束 (≤2) 通常只针对启动链路，不是 App 整个生命周期。
> - 如果删除某调用，必须 grep 该调用的所有下游 UI 字段（serverInfo / nickname / etc.），逐一确认是否有别的路径填。
> - 把"装饰性 UI 字段的填充"挪到首屏渲染之后异步触发是惯用做法 — 不阻塞首屏，又能保证字段最终被填。SwiftUI 的 `.task` 在 `.ready` 分支内触发就是这模式的天然位点。
> - **反例**：`// round 3: 移除 start() 调用 + 加 hasLoadedHome 短路防止误调` —— 把"调用入口"和"短路防御"两层都加上，看起来很彻底，但实际上让"未来想恢复 ping"变成 4 层都得改的微创外科手术。简洁的写法是：要么完全删除 `start()` 方法（没人能调），要么保持调用但挪位置（不要短路）。
> - 检查清单：每次改启动链路调用时，跑一遍"如果我删掉这行，UI 上哪些字段会失效"的反向追溯。

## Lesson 3: alert-only dismiss 不能隐式调 retry

- **Severity**: P2
- **Category**: error-handling
- **分诊**: fix (选项 B — alert / toast 路径 onDismiss 设为 no-op)
- **位置**: `iphone/PetApp/App/RootView.swift:416-420`（LaunchedContentView.needsAuthContent）

### 症状

round 2 把 `.needsAuth(message:)` 升级为 `.needsAuth(presentation: ErrorPresentation)` 后，在 `LaunchedContentView.needsAuthContent` 的 `.alert` 与 `.toast` 分支，把 `AlertOverlayView.onDismiss` 接到了 `Task { await stateMachine.retry() }`。但 mapper 把 `.unauthorized` / `.decoding` / `.missingCredentials` 三类钦定为 `.alert("...请重启应用")` —— 文案在告诉用户"你必须重启 App"，但 dismiss 立即触发 retry，与文案直接矛盾。结果是：

- 用户点"知道了" → retry → cold-start 又跑 → 仍 401 / 仍 decoding 失败 → 同 alert 弹回
- 死循环或反复发请求，用户被困在 modal 里

### 根因

round 2 实装 `.alert` 分支的 dismiss 时，参考了"不能让 alert 关闭就死锁在白屏"这条朴素直觉 → 顺手让 dismiss 也调 retry()。但忽视了一件事：alert 分支的 state 渲染本来就是 stable 的 —— 如果 onDismiss 是 no-op，state 仍是 `.needsAuth(.alert)`，AlertOverlayView 仍会从同一份 state 继续渲染，**根本不会出现白屏**。"dismiss 后白屏" 是想象出来的失败模式，不是真实存在的。

更深层根因：`.alert` 这个 case 的 ErrorPresentation 是 mapper 钦定为"用户必须冷重启"的语义信号，UI 层的 dismiss 行为必须 **respect 这个语义**而不是给用户加额外恢复入口。

### 修复

`.alert` 与 `.toast` (toast 也是 alert-only fallback) 分支的 `onDismiss` 改为 no-op：

```swift
case let .alert(title, message):
    AlertOverlayView(
        title: title,
        message: message,
        onDismiss: { /* alert-only: 不调 retry, 让用户冷重启 */ }
    )
```

`.retry` 分支保持不变（RetryView 的 `onRetry` 仍接 `stateMachine.retry()` —— 这才是 mapper 钦定为"可重试"的语义）。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **实装 ErrorPresentation 三态分发的 UI 层**时，**必须** respect mapper 钦定的语义 — `.retry` 分支才能接重试入口，`.alert` / `.toast` 分支的 dismiss / 自动消失行为**不能**触发任何会重发请求的副作用。
>
> **展开**：
> - mapper 把错误分成 `.retry` vs `.alert` 的根本目的是区分"可恢复" vs "不可恢复"。`.alert` 文案"请重启应用"已经在引导用户走 cold-start 路径，UI 层不能多此一举给隐式 retry 入口。
> - "dismiss 后会白屏" 通常是想象的失败模式 — 如果 state 没变，state-driven 的 view 会继续显示同一份 UI。SwiftUI 这点尤其稳：source of truth 没变 → view 不会消失。
> - **反例**：把 alert 的 onDismiss 接到 retry()，理由是"防白屏"或"给用户一个入口" — 这两个理由都站不住。前者是想象的，后者是与 mapper 钦定语义矛盾。
> - 检查清单：每次实装 ErrorPresentation 三态分发的 UI 时，列出 `.retry` / `.alert` / `.toast` 各自的"用户能做的动作"是什么，确保 `.alert` / `.toast` 路径的所有 callback 都是 view-local 的（dismiss / 隐藏 view），不是 state-mutating 的（重置状态机 / 调 retry）。

## Mental Model: 5 个交互节点的行为表

future Claude 1 分钟回查正确性的检查表：

| 节点 | 触发条件 | 状态/UI 转移 | 是否走 mapper | 是否触发请求 |
|---|---|---|---|---|
| **guest-login 失败** | bootstrap step1 内 `useCase.execute()` throws | catch → `BootstrapMappedError(presentation: mapper.presentation(for: error))` → state = `.needsAuth(presentation:)` | ✅ 必须 | 否 (失败已发生) |
| **loadHome 失败** | bootstrap step1 内 `loadHomeUseCase.execute()` throws | catch → `BootstrapMappedError(presentation: mapper.presentation(for: error))` → state = `.needsAuth(presentation:)` | ✅ 必须 | 否 (失败已发生) |
| **static unauthorized** | mapper 把 `.unauthorized` / `.decoding` / `.missingCredentials` 钦定为 `.alert("提示", "请重启/重新启动应用")` | UI 层渲染 AlertOverlayView | (mapper 已经判过) | 否 |
| **retry trigger** | 用户在 `.needsAuth(.retry(_))` 状态下点 RetryView 重试按钮 | `RetryView.onRetry` → `stateMachine.retry()` → state = `.launching` → 重跑 step1 (含 guest-login + loadHome) | (重跑 closure 时还会经 mapper) | ✅ 重试发请求 |
| **alert dismiss** | 用户在 `.needsAuth(.alert(_))` / `.needsAuth(.toast(_))` 状态下点"知道了" | `AlertOverlayView.onDismiss` → no-op → state 保持不变 → AlertOverlayView 继续显示 (引导冷重启) | N/A | ❌ 不发请求 |

**关键不变式**：
1. 所有抛给状态机的 error 必经 mapper（lesson 1）
2. ping/version 等装饰性请求不能放在启动链路 .task；放 `.ready` 分支的 `.task` 才合规（lesson 2）
3. 只有 `.retry` 分支的 onRetry 接 `stateMachine.retry()`；`.alert` / `.toast` 分支的 onDismiss 是 no-op（lesson 3）
4. cold-start HTTP 预算 ≤2 = 只算"启动 → .ready"链路，不含 .ready 之后异步发的 ping

---

## Meta: 本次 review 的宏观教训

Story 5.5 已经做了 4 轮 fix-review。前 3 轮的修复都是"修这条破那条"的 regression chain，因为 review 视角是 finding-by-finding 的局部检查，没有 holistic mental model 的兜底。本轮（也是 epic-loop 给的最后一轮机会）必须一次修干净。

教训：

1. **修 closure 内某一条 throw 路径时，必须 grep 该 closure 内所有 throw 路径**。lesson 1 的 root cause 就是 round 1 只修了 LoadHome 一条，漏了 guest-login 一条。
2. **删除某调用 + 加短路防御 是双重修改**，未来想恢复要改两个地方。要么完全删除入口（让别人无法调用），要么保留调用但挪位置 / 加 short-circuit 单选其一。
3. **UI 层 callback 必须 respect 上游 mapper 钦定的语义**，不能基于"防白屏"等想象的失败模式补充 fallback 行为。state-driven UI 的稳定性已经够强，dismiss 不需要 mutate state。
4. **每轮 fix-review 必须有 mental model 自审环节**，列出所有交互节点的行为表（如上节），跑一遍"5 个节点的行为是不是都自洽"再 commit。round 4 把这个检查表纳入 lesson，让未来 Claude 1 分钟回查。
