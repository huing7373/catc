---
date: 2026-04-30
source_review: codex round 4 review of Story 37.13 a11y identifier 总表 (file: /tmp/epic-loop-review-37-13-r4.md)
story: 37-13-accessibility-identifier-总表
commit: 152c22b
lesson_count: 1
---

# Review Lessons — 2026-04-30 — identifier helper 必须 source 自声明常量而非重新拼接（防 single-source-of-truth 静默 drift）

## 背景

Story 37.13 把 iPhone Features 各处散落的 a11y identifier 内联字符串归并到 `AccessibilityID` enum
(`iphone/PetApp/Shared/Constants/AccessibilityID.swift`)，把 4 Tab 常量 `Tab.home = "tab_home"` 等
声明出来后，又给 `MainTabView` 提供了 helper `Tab.identifier(for: rawValue)`（caller `MainTabView` 用
`AppTab.rawValue` 调它）。Round 4 review 指出该 helper 实现是 `"tab_\(rawValue)"` 字符串拼接，
**重新构造** identifier 而不是引用同 enum 已声明的 4 个 static constants，single-source-of-truth 表面上
存在但实际有两条平行真值链路。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `AccessibilityID.Tab.identifier(for:)` 重新拼接而非引用声明常量，破坏 single source of truth | medium (P2) | architecture | fix | `iphone/PetApp/Shared/Constants/AccessibilityID.swift:97-103` |

## Lesson 1: 动态 identifier helper 必须 switch 到声明常量，禁止字符串拼接重建

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Shared/Constants/AccessibilityID.swift:97-103`

### 症状（Symptom）

`AccessibilityID.Tab` enum 同时存在两类东西：
1. 4 个声明常量：`static let home = "tab_home"` / `wardrobe` / `friends` / `profile`
2. 一个 helper：`identifier(for rawValue: String) -> String`，实现是 `"tab_\(rawValue)"` 字符串拼接

UITests / 其他业务代码用 (1) 类常量；`MainTabView` 走 (2) 类 helper。表面 single source of truth，
但 (2) 的拼接绕过了 (1)。一旦未来 `AppTab.rawValue` 改名（如 `home → dashboard`），(2) 会拼出
`"tab_dashboard"`，而 (1) 的 `Tab.home` 还是 `"tab_home"`，UITests 也仍然查 `"tab_home"`，
runtime 绑的 a11y id 与 test fixture 静默不一致 → UITest 找不到元素，但单测全绿（因为单测不跨 target）。

refactor 想消除的"两份真值"问题在 (1)+(2) 这对组合里**重新长出来**，且更隐蔽（只剩一个 enum，
看起来像同一份真值）。

### 根因（Root cause）

写 helper 时的"自然惯性"：参数已经是 `rawValue: String`，最短实现就是 `"tab_\(rawValue)"`。
顺手 + 看着对（同 enum 的 4 个 static let 也确实长这样）→ 没觉得有问题。

但**字符串拼接是数据流出口**，不是数据流"reference 同一份声明"的路径。只要 helper 的输出格式
和 declared constants 的字面量值在两处独立维护，就构成两条平行真值链路，迟早 drift。

类比：声明 `let MAX_RETRY = 3` 之后再写 `func retryCount() -> Int { return 3 }` —— 三这个值
出现在两处，未来调成 5 就会忘改一处。helper 必须 `return MAX_RETRY` 才算引用，不能 hardcode。

字符串场景比数字更隐蔽——`"tab_home"` 看着像"同一个值的两次拼写"而不是两份真值。

### 修复（Fix）

`identifier(for:)` 改为 switch rawValue 映射到 4 个声明常量；未知 rawValue 走 `assertionFailure`
（Debug 阶段抓未匹配 case，Release 走 fallback 拼接不挂 production app）。Before / After：

```swift
// Before
public static func identifier(for rawValue: String) -> String { "tab_\(rawValue)" }

// After
public static func identifier(for rawValue: String) -> String {
    switch rawValue {
    case "home":     return Tab.home       // "tab_home"
    case "wardrobe": return Tab.wardrobe   // "tab_wardrobe"
    case "friends":  return Tab.friends    // "tab_friends"
    case "profile":  return Tab.profile    // "tab_profile"
    default:
        assertionFailure("AccessibilityID.Tab.identifier(for:) called with unknown rawValue: \(rawValue) — add new case here when AppTab adds a new tab")
        return "tab_\(rawValue)"
    }
}
```

同步在 `iphone/PetAppTests/Shared/Constants/AccessibilityIDTests.swift` 加一个新测试方法
`testTabIdentifierHelperReturnsDeclaredConstants`，4 tab 各断言一次
`identifier(for: "home") == AccessibilityID.Tab.home` 等——守护"helper 输出 ≡ 声明常量"
不变量，未来 dev 把任何一条改回拼接会立刻红。

verify：

- `bash iphone/scripts/build.sh --test` 347/347 通过（baseline 346 + 新加 1 守护）
- `bash iphone/scripts/check_a11y_coverage.sh` ✅
- `bash iphone/scripts/check_no_apiclient_in_features.sh` ✅

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写"动态拼名"helper（identifier / cache key / log tag / event name 之类）**
> 时，**必须** **switch 到同一作用域已声明的常量并 return 该常量；禁止用字符串拼接重新构造已存在的常量值**。
>
> **展开**：
> - 触发条件 = enum / namespace / module 里同时存在 (a) 一组离散的 static 常量字符串 + (b) 一个把
>   `rawValue` 映射到那组字符串的 helper。这是"两条真值链路"的最常见结构。
> - 默认套路：helper 内部 `switch rawValue { case "x": return Foo.x; ... default: assertionFailure(...); return fallback }`，
>   把每个 case 显式映射到声明常量。`assertionFailure` 给 Debug 阶段加 case 的硬约束，
>   Release fallback 不挂 production。
> - 如果常量数量大到 switch 不优雅（如 50+ enum case），改用编译期联结：让 declared constants 本身
>   走 `static let x = prefix(for: "x")`，helper 就是单一真值源 —— 而不是反过来 helper 拼字符串。
> - 同 enum 跨 target 编译（如 `AccessibilityID.swift` 通过 project.yml 同时进 PetApp + PetAppUITests）
>   时**特别危险**：拼接 helper 的"漂移"在 UITest target 里只能靠运行时发现（因为 UITest 看不到
>   `AppTab` 类型，单测 build 不 fail）。switch + 声明常量是唯一不会跨 target 漂的写法。
> - **测试守护配方**：每加这种 helper，必须配一条断言 `helper(input) == declared_constant` 而不是
>   `helper(input) == "literal_string"`。前者捕捉 declared constant 改值时 helper 是否同步；
>   后者只捕捉 helper 输出值变化，无法守护"两条链路同步"这个本质不变量。
> - **反例 1（本次踩坑）**：`func identifier(for rawValue: String) -> String { "tab_\(rawValue)" }`
>   —— 看起来 "Tab" prefix 写在一处，实际和声明常量 `static let home = "tab_home"` 是两份字面量。
> - **反例 2**：`func identifier(for rawValue: String) -> String { "tab_" + rawValue.lowercased() }`
>   —— 拼接 + 大小写转换，更难发现 drift；且如果未来某个 case 决定不走 `tab_` 前缀（如某个特殊
>   tab 用 `nav_` 前缀），helper 没机会处理 → silently 拼错。
> - **正例**：`switch ... case "home": return Tab.home`，每个 case 都拿声明常量做返回值；
>   未来某个声明常量改字面量（如 `Tab.home = "primary_tab"`），helper 自动同步。
