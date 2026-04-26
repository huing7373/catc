---
date: 2026-04-26
source_review: codex review round 2 of Story 2-8 (file: /tmp/epic-loop-review-2-8-r2.md)
story: 2-8-dev-重置-keychain-按钮
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — SwiftUI 父级 a11y `.contain` 必须保留 `.accessibilityLabel` 才不丢父 summary

## 背景

Story 2.8 在 `HomeView.userInfoBar` 引入 dev `ResetIdentityButton` 时，把父容器 a11y modifier 从
`.accessibilityElement(children: .ignore) + .accessibilityLabel(nickname)` 改成
`.accessibilityElement(children: .contain) + .accessibilityIdentifier(home_userInfo)`，以让子按钮被
XCUITest 独立定位。改动**漏掉了**保留父级 `.accessibilityLabel(nickname)`，导致 VoiceOver 用户读
home_userInfo 时不再听到 nickname summary —— round 2 codex review [P2] 指出。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | userInfoBar a11y label 丢失 | medium | a11y | fix | `iphone/PetApp/Features/Home/Views/HomeView.swift:68-74` |

## Lesson 1: 父容器 a11y `.contain` 与 `.accessibilityLabel` 必须并存才不丢父 summary

- **Severity**: medium
- **Category**: a11y
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/Views/HomeView.swift:68-74`

### 症状（Symptom）

`userInfoBar` 父容器仅声明 `.accessibilityElement(children: .contain)` + `.accessibilityIdentifier(home_userInfo)`。
VoiceOver 焦点落到 home_userInfo 时只通过子元素列表（nickname Text、ResetIdentityButton）发声，
父级没有自定义 label —— 原本由 Story 2.2 落地的"父读 nickname 作为 summary"语义丢失。

### 根因（Root cause）

误以为从 `.ignore` 切到 `.contain` 是 "label modifier 失效"，因此把 `.accessibilityLabel(nickname)` 一起删了。
实际上 SwiftUI 的 `.contain` 与 `.ignore` 只控制**子元素是否参与 a11y 树**：

- `.ignore` —— 子元素全部隐藏，父独占 element（必须用 `.accessibilityLabel` 给父显式命名）
- `.contain` —— 子元素仍存在于 a11y 树（可被 UITest 独立定位），父也仍是一个 a11y element，
  其 label 由 `.accessibilityLabel` 自定义；不写则 fallback 为子元素拼接（用户体验差且不稳定）

两者**不冲突**：父级 `.contain` + `.accessibilityLabel(...)` 让父能读到自定义 summary、子也能被独立访问。
本次回归是把"换 children 模式"和"删 label"误绑成一个动作。

### 修复（Fix）

```swift
// before（round 1 dev-story 落地）
.accessibilityElement(children: .contain)
.accessibilityIdentifier(AccessibilityID.Home.userInfo)

// after
.accessibilityElement(children: .contain)
.accessibilityLabel(Text(viewModel.nickname))   // 保留父 summary
.accessibilityIdentifier(AccessibilityID.Home.userInfo)
```

新增 UITest `testUserInfoBarRetainsNicknameAccessibilityLabel` 锁这条契约：
启动 App 后断言 `app.descendants(matching: .any)[home_userInfo].label == "用户1001"`
（`HomeViewModel.nickname` 默认值）。`bash iphone/scripts/build.sh --uitest` 6/6 通过。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 SwiftUI 父容器从 `.accessibilityElement(children: .ignore)` 切换到
> `.contain`（为了让子元素能被 XCUITest 或 VoiceOver 独立访问）时，**必须同时检查既有的
> `.accessibilityLabel(...)` 是否还需要保留** —— 默认答案是"保留"，除非明确确认父级不需要 summary。
>
> **展开**：
> - `.contain` 与 `.accessibilityLabel` **完全兼容**：父读自定义 label，子仍可被 a11y 树独立定位，两者并存
> - 切 children 模式（ignore ↔ contain）时，把 a11y modifier 链当作**三件套**整体审视：
>   `accessibilityElement(children:)` + `accessibilityLabel(...)` + `accessibilityIdentifier(...)`。
>   动一个之前先确认其余两个是否仍合理
> - 落 UITest 断言：用 `element.label == <预期字符串>` 锁父级 a11y label，防止后续重构再次回归
> - **反例**：「我要让子按钮 (`ResetIdentityButton`) 被 XCUITest 找到 → 把父从 `.ignore` 改成 `.contain` →
>   连带删了 `.accessibilityLabel(nickname)`，因为感觉 contain 就是 children 接管。」错。`.contain`
>   不接管父级 label；父 label 仍归 `.accessibilityLabel` modifier 管，删了就真的丢了
> - 与 `2026-04-26-stateobject-debug-instance-aliasing.md`（Story 2.8 round 1）和 Story 2.3 a11y propagation
>   lesson 同主题家族 —— iOS a11y modifier 链跨 view 重构是高密度踩坑区，下次动 a11y 链先 grep
>   `accessibilityElement\|accessibilityLabel\|accessibilityIdentifier` 三件套确认完整性
