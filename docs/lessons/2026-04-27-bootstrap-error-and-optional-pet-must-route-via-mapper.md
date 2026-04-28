---
date: 2026-04-27
source_review: file:/tmp/epic-loop-review-5-5-r1.md（codex review · Story 5.5 round 1）
story: 5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据
commit: ac03578
lesson_count: 2
---

# Review Lessons — 2026-04-27 — bootstrap /home 失败必须经 AppErrorMapper + 可空 domain 字段必须区分 loading 与"明确无值"两种 placeholder

## 背景

Story 5.5 落地 `LoadHomeUseCase` + 把 `GET /home` 接到首屏 + 把 `pet`/`stepAccount`/`chest` 字段渲染到 `HomeView`。codex round 1 给了 2 条 P2 finding：

1. `HomeView.swift:120` 在 `homeData != nil && pet == nil`（V1 §5.1 schema 明确允许，例如首次注册或 Reset 后）的状态下仍然显示 placeholder 文案 `"默认小猫"`，让"server 明确说无宠物"的账号 UI 显示成"已有宠物且名字是默认小猫"。
2. `RootView.swift:193-195` 启动期 `loadHomeUseCase.execute()` 失败时走 `AppLaunchStateMachine.messageFor` → `APIError.errorDescription`，产出 `"Network error: ..."` / `"Business error 1009: ..."` 等 developer 串到 RetryView，绕过了 `AppErrorMapper` 已有的 user copy（"网络异常，请检查后重试" / "服务繁忙，请稍后重试"）。

两条都属于 user-visible correctness 问题，本次全部修。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Handle `pet: null` without fabricating a default pet name | P2 | architecture | fix | `iphone/PetApp/Features/Home/Views/HomeView.swift:120` |
| 2 | Route bootstrap `/home` failures through the user-facing error mapper | P2 | error-handling | fix | `iphone/PetApp/App/RootView.swift:193-195` |

## Lesson 1: 可空 domain 字段必须区分 loading 与"server 明确无值"两种 placeholder

- **Severity**: P2
- **Category**: architecture / ui
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/Views/HomeView.swift:120`

### 症状（Symptom）

`Text(viewModel.homeData?.pet?.name ?? "默认小猫")` 把"`homeData == nil`（loading）"和"`homeData != nil && pet == nil`（V1 §5.1 schema 明确允许的 server-side 'no pet' 状态）"两种语义合并到同一个 fallback 文案 `"默认小猫"`。结果：server 明确返回 `pet: null` 的账号在 UI 上看起来"已经有宠物，名字叫默认小猫"，掩盖 backend truth。

### 根因（Root cause）

写法 `optional?.optional?.field ?? placeholder` 在 SwiftUI 里很自然，看起来"安全 + 简洁"，但它**塌陷**了"未拿到数据"和"拿到数据但字段是 null"两个**语义不同的状态**。两者在 V1 schema 下都是合法的，但 UI 含义完全不一样：

- "未拿到数据" → 用户应该看到"加载中" placeholder（或 spinner，本节点用静态文案）。
- "拿到数据但字段是 null" → 用户应该看到"无 X" 真实状态（或操作引导："去领养小猫"等）。

设计文档 `V1接口设计.md §5.1` 明确写了 `pet: PetProfile | null`，但写代码时只看到 `HomePet?` 字段定义，没回到设计文档确认 null 的业务语义。

### 修复（Fix）

把决策抽到纯函数 helper（`HomePetNameResolver`，与既有 `HomeNicknameResolver` 同模式），三态分支显式列出：

```swift
public static func resolve(homeData: HomeData?) -> String {
    guard let homeData = homeData else { return loadingPlaceholder }   // "默认小猫"
    guard let pet = homeData.pet else { return noPetPlaceholder }      // "暂无宠物"
    return pet.name                                                     // "Mochi"
}
```

`HomeView.petColumn` 改用 `petNameDisplay` 属性。test 覆盖三个分支 + 一个 edge（pet.name 是空字符串时按 server 原样展示，与 `HomeNicknameResolver` 同精神 "以 server 为准"）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在为 SwiftUI / View / DTO 字段写 `optional?.optional?.field ?? placeholder` 时，**必须**先回到 V1 接口设计 / 数据库设计文档确认"内层 optional 是否是 schema 明确允许的合法值"——若是，就**不能**让两层 nil 共用同一个 fallback 文案，必须显式三态分支。
>
> **展开**：
> - 触发条件：嵌套 optional 链（`a?.b?.c`）尾部 `??` 一个字符串/数字/默认值。
> - 必须做：
>   1. 把"外层 optional == nil"（loading / 未注入 / 未加载）和"外层非 nil 但内层 optional == nil"（schema 明确无值）当成**两个不同 case** 决策。
>   2. 把决策抽成纯函数 helper（`enum + static func resolve(...)` 模式），再在 SwiftUI 里 `Text(resolver.resolve(...))`，让 case 可被单测独立锁住。
>   3. 测试覆盖**至少**三个 case：`outer == nil` / `outer != nil && inner == nil` / `inner != nil`。中间那个是核心 case——它就是 review 会指出来的那条。
> - 反例（fix 前）：
>   ```swift
>   Text(viewModel.homeData?.pet?.name ?? "默认小猫")  // ❌ 两种 nil 共用 fallback
>   ```
> - 正例（fix 后）：
>   ```swift
>   Text(petNameDisplay)
>   private var petNameDisplay: String {
>       HomePetNameResolver.resolve(homeData: viewModel.homeData)
>   }
>   ```
> - **跨语言通用**：Go / TypeScript 里同样适用——`resp?.pet?.name || "默认小猫"`、`resp.Pet?.Name ?? "默认小猫"` 都是同一个反模式。检查清单：每写完一个 `?.x?.y ?? z`，问自己"内层 nil 在 schema 里是合法值吗？合法的话 UI/log 应该和 outer nil 区分吗？"两个答案都是 yes 就**必须**拆分。

## Lesson 2: 启动状态机 / bootstrap 路径的错误必须经统一 user-facing mapper，禁止漏 developer 串到用户

- **Severity**: P2
- **Category**: error-handling / architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift:193-195` + `iphone/PetApp/App/AppLaunchStateMachine.swift:118-123`

### 症状（Symptom）

启动期 `loadHomeUseCase.execute()` 失败 → `AppLaunchStateMachine.messageFor(error:)` 用 `as? LocalizedError + errorDescription` 提取 message → 但 `APIError.errorDescription` 是 developer copy（`"Network error: timed out"` / `"Business error 1009: 服务繁忙"`）→ RetryView 展示 developer 串。`AppErrorMapper` 已经为同一组 APIError 定义了 user copy（"网络异常，请检查后重试" / "服务繁忙，请稍后重试"），但 bootstrap 路径绕过了它，UI 文案不一致。

### 根因（Root cause）

`AppLaunchStateMachine.messageFor` 是 Story 2.9 落地状态机骨架时写的，那时还没有 `AppErrorMapper`（Story 2.6 同期写的，但状态机选择了"轻量"——只看 `LocalizedError`），并且 lesson `2026-04-26-error-localizeddescription-system-fallback.md` 只解决了"非 LocalizedError 不要漏 NSError 系统串"——但它**没**约束"LocalizedError 的 errorDescription 是否就是 user copy"。Epic 5 给 `APIError` 实现 LocalizedError 时，`errorDescription` 直接写成了 developer 友好的串（方便 log），没人意识到这一串会原路漏到 RetryView。

更深层的根因：**两套错误映射并存**——`AppErrorMapper`（user-facing，用于 ErrorPresenter 路径）+ `APIError.errorDescription`（developer，用于 log / debug）——但没人挑明 bootstrap 路径应该走哪一套。Story 5.5 把 `loadHomeUseCase.execute()` 接到 bootstrap closure 时，**默认走了 errorDescription 路径**（因为 messageFor 是这么实现的），没主动选 mapper。

### 修复（Fix）

不动 `messageFor` / `APIError.errorDescription`（log 路径继续用 developer 串），而是在 bootstrap closure 边界做一层 wrapper：

```swift
// RootView.ensureLaunchStateMachineWired 内：
do {
    homeData = try await loadHomeUseCase.execute()
} catch {
    throw BootstrapMappedError(
        userFacingMessage: AppErrorMapper.userFacingMessage(for: error),
        underlying: error
    )
}
```

`BootstrapMappedError: LocalizedError` 把 `errorDescription` 实现为 mapper-derived user copy；`AppLaunchStateMachine.messageFor` 现在拿到的就是 user copy。新增 `AppErrorMapper.userFacingMessage(for:)` 静态 helper，复用 `presentation(for:)` switch 出 message 部分（避免重复维护两份文案表）。

测试侧两个新增 case：
- `AppErrorMapperTests.testUserFacingMessageFor*MatchesXxxCopy`（4 case）锁定 helper 与 presentation 文案对齐。
- `AppLaunchStateMachineTests.testBootstrapWithMappedErrorPropagatesUserFacingMessage` + business 变体（2 case）锁定"`BootstrapMappedError` + state machine = 用户看到 mapper 文案"。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在为 iOS app 写 / 接入 user-visible 错误展示路径时，**必须**让所有路径（ErrorPresenter / 启动状态机 / 离线提示 / 任何最终落到 SwiftUI Text 的 message）都经过同一个 `AppErrorMapper`，**禁止**直接用 `error.localizedDescription` 或 `LocalizedError.errorDescription`——后者是 developer copy，给 log / debug 用。
>
> **展开**：
> - 触发条件：写代码涉及"把 Error 转成字符串放到 user 能看到的地方"——RetryView message / Alert title / Toast / 状态机 `.failed(message:)` 类 case payload。
> - 必须做：
>   1. **所有**这种代码段都 import 并调用 `AppErrorMapper.presentation(for:)` 或 `AppErrorMapper.userFacingMessage(for:)`。
>   2. 如果上层接口（如 `AppLaunchStateMachine.bootstrap` 的 closure throw 边界）只能 throw Error，**必须**在 throw 之前用 `BootstrapMappedError` / 同等 wrapper 包一层 LocalizedError，把 mapper 输出塞进 `errorDescription`——让中间层不需要知道 mapper 存在也能拿到 user copy。
>   3. 新加 LocalizedError 实现时（如新 APIError case），`errorDescription` 写 developer copy（log 用），user copy 由 `AppErrorMapper` 表统一管。
> - 反例（fix 前）：
>   ```swift
>   private func messageFor(error: Error) -> String {
>       if let localized = error as? LocalizedError, let desc = localized.errorDescription, !desc.isEmpty {
>           return desc   // ❌ APIError.errorDescription 是 "Network error: ..."
>       }
>       return defaultMessage
>   }
>   // 调用方直接 throw APIError → user 看到 developer 串
>   ```
> - 正例（fix 后）：
>   ```swift
>   // 调用方 (bootstrap closure):
>   do {
>       homeData = try await loadHomeUseCase.execute()
>   } catch {
>       throw BootstrapMappedError(
>           userFacingMessage: AppErrorMapper.userFacingMessage(for: error),
>           underlying: error
>       )
>   }
>   // BootstrapMappedError.errorDescription 返回 mapper 文案 → 状态机 messageFor 拿到 user copy
>   ```
> - **审计触发器**：每次新加 / 改写 user-visible error 路径（grep `RetryView` / `AlertOverlay` / `Toast` / `.needsAuth(message:` / `.failed(message:`），都要回头检查"这条路径是否经过了 AppErrorMapper"。如果没有，**就是 review 会抓的 finding**。

---

## Meta: 本次 review 的宏观教训（可选）

两条 finding 看似主题不同（UI placeholder vs 错误映射），但**底层共享同一种思维漏洞**：**"在数据/错误的边界处过早收敛多种语义到同一个 fallback / 同一个文案"**。

- Lesson 1：把"未加载"和"加载完但 server 说无值"塌陷到同一个 placeholder。
- Lesson 2：把"developer log copy"和"user-facing copy"塌陷到同一个 LocalizedError.errorDescription。

未来在 review 自己代码时，**特别警惕"省事的 ?? / fallback / 默认值"**——它们经常隐藏不同语义的 case。检查清单：
1. 这个 fallback 后面的字符串/值在**哪些**输入下会被使用？两种输入的业务语义一样吗？
2. 这个 helper 函数（如 `messageFor` / `errorDescription`）的输出会**漏到**几条不同路径（log / UI / 调试）？这些路径对"什么是合理输出"的要求一样吗？

两个问题任一答案是"不一样"——就**不**能共用一个 fallback / helper，必须显式分支。
