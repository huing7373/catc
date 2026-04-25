---
date: 2026-04-26
source_review: codex review round 1 on Story 2.5 (file: /tmp/epic-loop-review-2-5-r1.md)
story: 2-5-ping-调用-主界面显示-server-version-信息
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — @StateObject 老 init + bind() 注入路径漏掉副作用初始化

## 背景

Story 2.5 给 `HomeViewModel` 加了"启动时 ping server 取 commit、显示在主界面 footer"的能力。
为规避 SwiftUI `@StateObject` 在 init 阶段的注入限制，工程上选择了"双 init + `bind()` 单次绑定"模式：

- 老 init `HomeViewModel()`：保留给 Story 2.2 / 2.3 的 Preview / 测试，`appVersion` 默认 hardcode `"0.0.0"`。
- 新 init `HomeViewModel(pingUseCase:)`：`appVersion` 默认参数从 Bundle 读 `CFBundleShortVersionString`。
- `bind(pingUseCase:)`：production 路径用，配合 `RootView` 的 `@StateObject + .task`。

`RootView` 走的就是 `HomeViewModel()` + `bind()` 路径。问题：`bind()` 只赋值 `boundPingUseCase`，**没**调
`readAppVersion()`。结果 production runtime 永远显示 `v0.0.0` —— bundle 里其实是 `1.0.0`。

codex round 1 review 直接点名："the version footer never picks up CFBundleShortVersionString and will keep
rendering v0.0.0 in the real app."

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | bind() 路径漏调 readAppVersion()，production appVersion 停在 hardcode 默认值 | medium (P2) | bug | fix | `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift:85-88` |

## Lesson 1: 当一个能力被切成"老 init / 新 init / bind() 注入"三入口，副作用初始化必须在所有入口对齐覆盖

- **Severity**: medium (P2)
- **Category**: bug（功能不正确：UI 显示错版本）
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift:85-88`（bind() 内追加 `appVersion = readAppVersion()`）

### 症状（Symptom）

```swift
// HomeViewModel
public init(...) { self.appVersion = "0.0.0"; self.pingUseCase = nil }              // 老 init
public init(pingUseCase: ..., appVersion: String = readAppVersion(), ...) { ... }   // 新 init：默认参数读 Bundle
public func bind(pingUseCase: PingUseCaseProtocol) {                                 // production 入口
    guard self.boundPingUseCase == nil else { return }
    self.boundPingUseCase = pingUseCase    // ← 只设了 useCase，appVersion 还停在 "0.0.0"
}

// RootView
@StateObject private var homeViewModel = HomeViewModel()           // 走老 init → appVersion = "0.0.0"
.task { homeViewModel.bind(pingUseCase: container.makePingUseCase()); await homeViewModel.start() }
// → bind 只更新 useCase，没刷 version → footer 永远显示 v0.0.0
```

测试也没抓到这个 bug：HomeViewModelPingTests 显式不断言 `appVersion`（注释写"避免依赖测试 host bundle"），
HomeViewModelTests 的 `testHardcodedDefaultStateMatchesStorySpec` 走的是 `HomeViewModel()` 不调 bind 的纯老路径，
也合理地停在 `"0.0.0"`。两层测试都"对自己测的路径正确"，但是 production 真实路径（老 init + bind）介于两者之间，
没有任何测试覆盖。

### 根因（Root cause）

`@StateObject` 在 init 阶段不是"真正的实例"——SwiftUI 用属性 wrapper 延迟构造，无法在 init 表达式里依赖运行时
环境（Bundle、容器、coordinator 等）；所以 production 不能直接 `HomeViewModel(pingUseCase: container.make...())`。
解法是延迟到 `.onAppear` / `.task` 里通过 `bind()` 注入。

但当工程师切到"双 init + bind"模式时，**只把"被注入的依赖"复制过去了**，没意识到新 init 的"默认参数"里其实
还有一个隐式副作用——`appVersion: String = readAppVersion()`。这个"默认参数计算"看起来像 init 的事，
其实它等价于"实例构造时调一次 readAppVersion"，是一个**初始化副作用**。bind() 路径漏掉它，等于漏了一半构造逻辑。

抽象后的规则：**"通过 init 默认参数表达的隐式初始化副作用"在加上 bind/late-inject 入口后必须显式复制一遍**，
否则 production 路径会比测试路径少做一些事。

### 修复（Fix）

`bind()` 内部追加同步刷 `appVersion`：

```swift
public func bind(pingUseCase: PingUseCaseProtocol) {
    guard self.boundPingUseCase == nil else { return }
    self.boundPingUseCase = pingUseCase
    self.appVersion = HomeViewModel.readAppVersion()   // 副作用初始化对齐新 init 的默认参数
}
```

回归测试 `testBindUpdatesAppVersionFromBundle`：先建 `HomeViewModel()`，断言 `appVersion == "0.0.0"`；
调 `bind()`；断言 `appVersion == HomeViewModel.readAppVersion()`（不写死值，避免依赖测试 host bundle）。

老测试 `testHardcodedDefaultStateMatchesStorySpec` 不变（它故意走纯老路径，`appVersion` 应该停在 "0.0.0"）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 给 SwiftUI `@StateObject` 加"运行时延迟注入"的 `bind()` 入口时，**必须**把对应新
> init 默认参数里隐含的所有副作用初始化（如 `Bundle.main` 读取、`Date.now`、`UUID()` 之类）**显式**复制到
> `bind()` 内部，并补一条"bind() 后状态等于新 init 后状态"的回归测试。

> **展开**：
>
> - **触发条件**：当 ViewModel 同时存在"测试用 init（依赖显式注入）"和"production 用 bind/late-inject"时
>   高度警惕。两条路径必须在状态可观察输出上对齐。
> - **审查清单**（review 自己的 PR 时按这个 grep）：
>   - 新 init 的默认参数表达式列表是什么？比如 `appVersion: String = readSomething()`、
>     `createdAt: Date = .now`、`requestID: String = UUID().uuidString`。
>   - bind() 函数是不是只赋值了"新加的依赖字段"，对上述默认参数计算是不是一个都没复制？如果是，**必然漏初始化**。
> - **测试策略**：永远要有一条 case 形如 `testBindThenStateMatchesInjectedInit`：分别用老 init+bind 与新 init
>   构造两个实例，断言"对外可见状态字段一致"。这条测试单点拦截整类问题。
> - **反例 1**："默认参数副作用很少没人会出错"——错。Swift 默认参数表达式延迟到 call site 求值，看起来像一个值
>   实际是一个**调用**。当你切到 bind() 路径就是切走了这个 call site，副作用一并丢失。
> - **反例 2**："bind() 只管注入依赖，副作用应该 caller 自己负责"——会让 caller（这里是 RootView）必须知道
>   ViewModel 内部用了什么 bundle key，违反封装。把副作用收敛在 ViewModel 内部更稳。
> - **反例 3**：在 RootView `.onAppear` 里手动调 `homeViewModel.appVersion = ...`——分散初始化逻辑，下次再
>   加新副作用又会遗漏。集中在 bind() 单点。
> - **替代方案的边界**：如果新 init 默认参数表达式有 N 个，bind() 内手动复制 N 行很丑——这时考虑提取一个
>   `private func applyEnvironmentDefaults()` 让两个 init + bind() 都调一次。本 story N=1（只一个
>   readAppVersion()），单行 inline 够用，不上抽取。

### 顺带改动

- `iphone/PetAppTests/Features/Home/HomeViewModelPingTests.swift` 新增 case#9
  `testBindUpdatesAppVersionFromBundle`，断言 bind() 后 `appVersion == readAppVersion()`。
- `HomeViewModel.bind()` 注释扩写，说明为什么 bind 内要刷 appVersion，并指向本 lesson 文件。
