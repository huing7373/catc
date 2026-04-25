---
date: 2026-04-25
source_review: codex round 2 review of Story 2.2（file: /tmp/epic-loop-review-2-2-r2.md）
story: 2-2-swiftui-app-入口-主界面骨架-信息架构定稿
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-25 — ObservableObject / @Published 必须显式 `import Combine`

## 背景

Story 2.2 的 `HomeViewModel.swift` 只 `import Foundation`，但用了 `ObservableObject` 与 `@Published`。Codex round 2 标 [P0] "build will fail"。实测 `xcodebuild build` 仍 SUCCEEDED —— iOS 平台 Foundation 通过 transitive import 提供了 Combine 的核心符号。但显式 `import Combine` 仍然是该补的最佳实践，本次 review 的修复价值不在"避免编译失败"，而在"避免依赖 transitive import 的脆弱性 + IDE 工具准确性 + 代码意图清晰"。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | HomeViewModel 缺 `import Combine` | low (codex 标 P0 但实测 build pass) | style / dependency | fix | `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift:12` |

## Lesson 1: ObservableObject / @Published 应显式 `import Combine`，不要躺在 transitive import 的脆弱保证上

- **Severity**: low（实际严重性，**非** codex 报告的 P0）
- **Category**: style / dependency
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift:12`

### 症状（Symptom）

`HomeViewModel.swift` 顶部仅 `import Foundation`，文件内使用 `ObservableObject` 协议与 `@Published` propertyWrapper —— 这两个符号的 canonical 定义来自 Combine framework。

### 根因（Root cause）

iOS / iPadOS / macOS 上的 Foundation framework 通过 module re-export 链路间接把 Combine 的核心符号（`ObservableObject` / `@Published` 等）带到了当前 module 的 lookup scope，所以**碰巧**编译通过。这是一种 transitive / implicit import，**不是 Swift 语言层的稳定保证**：

- 不同 SDK 版本 / 不同平台（如 Linux Foundation 不带 Combine）行为不一致
- Xcode / SourceKit 在做符号解析、autocompletion、jump-to-definition 时，遇到 transitive import 容易误判
- 未来某个 Xcode 版本若调整 Foundation 的 re-export 列表，代码会突然报红
- 看代码的人无法从 import 列表推断"这个文件依赖 Combine"，可读性差

写代码时**不能**用"反正现在能编"作为依据 —— 显式 import 才是契约。

### 修复（Fix）

在 `import Foundation` 之后追加一行 `import Combine`：

```swift
// Before
import Foundation

@MainActor
public final class HomeViewModel: ObservableObject { ... }

// After
import Foundation
import Combine

@MainActor
public final class HomeViewModel: ObservableObject { ... }
```

- 改动范围：单文件单行追加
- 不影响 build 结果（before / after 均 BUILD SUCCEEDED + TEST SUCCEEDED）
- 改动目的：把 implicit dependency 显式化

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 Swift 文件**首次使用某 framework 的 type / propertyWrapper / protocol** 时，**必须**在文件顶部显式 `import` 该 framework，**禁止**依赖"transitive import 让代码碰巧能跑"作为不写 import 的理由。
>
> **展开**：
> - `ObservableObject` / `@Published` / `PassthroughSubject` / `CurrentValueSubject` / `AnyCancellable` → `import Combine`
> - `View` / `@State` / `@StateObject` / `@EnvironmentObject` / `Binding` / `ViewBuilder` → `import SwiftUI`（注意：SwiftUI 也 re-export 了 Combine 的部分符号，但不应反向依赖此事实）
> - `URLSession` / `Date` / `JSONDecoder` → `import Foundation`
> - **判断方法**：写完文件后扫一遍非 stdlib 符号，每个符号问"这是哪个 framework 提供的"→ 把对应 framework 加入 import 列表
> - **反例 1**：仅 `import Foundation` + 用 `@Published` —— 当前 iOS 能编但脆弱（本次 review 命中的踩坑形态）
> - **反例 2**：用"反正 SwiftUI 已经 import 过了，所以本文件不用 import Combine" —— 这是 **module-level fallacy**：transitive 关系不是 Swift module 系统承诺的稳定行为
> - **反例 3**：因为"Xcode 没报错"就跳过补 import —— Xcode pass ≠ 语义正确
>
> **额外注意**：codex / LLM reviewer 可能把这类"显式 import 缺失"标成 [P0] "build will fail"。事实上**当前编译多半 pass**（transitive import 兜得住），实际严重性是 [P2]（最佳实践 / 防御性 / 可读性）。修是该修，但不要在 commit message / lesson 里复述 reviewer 的 "build will fail" 原话 —— 那是技术不准确的判断。
