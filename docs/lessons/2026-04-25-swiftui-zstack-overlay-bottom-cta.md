---
date: 2026-04-25
source_review: codex review --uncommitted（epic-loop r1 for story 2-2-swiftui-app-入口-主界面骨架-信息架构定稿）
story: 2-2-swiftui-app-入口-主界面骨架-信息架构定稿
commit: 4de0140
lesson_count: 1
---

# Review Lessons — 2026-04-25 — SwiftUI ZStack overlay 不能盖在底部 CTA 行上

## 背景

Story 2.2 在 `iphone/PetApp/Features/Home/Views/HomeView.swift` 用一个 ZStack 把"6 大区块的主体 VStack"和"右下角版本号 caption"叠在一起：底层是主 VStack（最后一行是三按钮 `bottomButtonRow`），覆盖层是一个把 versionLabel 推到右下角的 VStack/HStack/Spacer 链。codex review 指出在 iPhone SE 这样的小屏上，覆盖层的 versionLabel 落在三按钮行的同一区域，会**视觉遮挡**最右侧的"合成"按钮，并且因为 SwiftUI 的 hit-test 顺序，可能**截获本应进按钮的点击**。Story 2.2 验收明确把 SE 作为目标尺寸之一，所以这是真问题不是 nit。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | versionLabel 用 ZStack overlay 落在底部 CTA 行上，小屏遮挡/截获"合成"按钮 | medium (P2) | architecture / layout | fix | `iphone/PetApp/Features/Home/Views/HomeView.swift` |

## Lesson 1: SwiftUI 里别用 ZStack overlay 把"装饰元素"压在交互行上

- **Severity**: medium
- **Category**: architecture / layout
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/Views/HomeView.swift:38-47`（修复前）

### 症状（Symptom）

`HomeView.body` 是 `ZStack { mainVStack; overlayVStackPushingVersionToBottomRight }`。在 375×667（iPhone SE）这种紧凑 width 上，overlay 的 versionLabel 与 mainVStack 末行的 `bottomButtonRow`（"进入房间 / 仓库 / 合成"）落在同一垂直区段，最右侧的"合成"按钮被 caption 视觉覆盖，并且 caption 在 ZStack 中位于上层、即便是小区域 Text 也会参与 hit-test，可能把 tap 截到 versionLabel 上。

### 根因（Root cause）

误把 SwiftUI 的 ZStack 当成 CSS `position: absolute` —— 想着"装饰性的小字嘛，绝对定位钉到右下角不就行了"。但 SwiftUI 没有"绝对定位 + ignore layout"的便捷写法，ZStack 的所有子视图都参与渲染层叠且都参与 hit-test。当下层的 `bottomButtonRow` 也想钉在底部时（被 `Spacer()` 推下去），上下两层就会在同一物理区段争夺空间和点击。版本号是低优先级装饰，CTA 是 primary action，让装饰盖在 primary action 上是优先级反转。

更深一层：原作者把"6 大区块"中第 ⑥ 项（版本号）当成"游离于主布局之外的水印"来设计，但 ⑥ 实际上就是一个普通的 footer 行，应该和 ① ~ ⑤ 一样在主 VStack 的纵向流里占一席之地。

### 修复（Fix）

把 versionLabel 从 ZStack overlay 拆出来，作为 main VStack 的最后一个子视图，新加一层 `versionFooter`（`HStack { Spacer(); versionLabel }`）把它推到右侧；删掉外层 ZStack，body 直接是单一 VStack。

before：
```swift
ZStack {
    VStack(spacing: 16) { userInfoBar; Spacer(); petAndChestRow; stepBalanceLabel; Spacer(); bottomButtonRow }
        .padding(...)
    VStack { Spacer(); HStack { Spacer(); versionLabel } }
        .padding(...)
}
```

after：
```swift
VStack(spacing: 16) {
    userInfoBar
    Spacer()
    petAndChestRow
    stepBalanceLabel
    Spacer()
    bottomButtonRow
    versionFooter   // = HStack { Spacer(); versionLabel }
}
.padding(...)
```

副作用：versionLabel 现在占据按钮行下方一行（约 caption 字号 + 16pt VStack spacing），主体内容上移；`Spacer()` 仍能在大屏上把内容撑开，所以视觉上仍接近"右下角小字"语义但严格不再压到按钮。a11y identifier 全部保持不变（测试无需改）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：在 SwiftUI 里，**当顶层是 ZStack 把"装饰性元素 (caption / 水印 / 角标)"叠在另一个含交互控件 (Button / TapGesture) 的子树之上**时，**必须**把装饰性元素改造为参与正常布局流的兄弟节点（VStack 子项 / overlay-on-specific-view），**禁止**用全屏 ZStack overlay 让装饰元素和 CTA 共享同一像素区域。
>
> **展开**：
> - SwiftUI 的 ZStack 子视图全部参与 hit-test，没有 CSS `pointer-events: none` 的等价物便捷开关。要让装饰元素不吃点击需要显式 `.allowsHitTesting(false)`，但首选**不让它们物理重叠**。
> - 想钉到屏幕角落的装饰元素：优先做主 VStack/HStack 的兄弟子项；只在确实需要悬浮（如 toast / FAB）时才用 `.overlay(_:alignment:)` **绑在某个具体 view 上**，并显式让出 hit area。
> - 紧凑屏（iPhone SE 375×667）是 layout 验收的 ground truth；任何"在大屏上看着对"的悬浮布局必须在 SE 模拟器复核一遍。Story 2.2 的 AC 显式列了 SE 尺寸，不是装饰要求。
> - 写 layout 代码前先问"这个元素在文档流里属于哪一行/哪一列"，再问"它需要打破文档流吗"。多数 caption / footer 的答案都是"它就是最后一行"，无需打破。
> - **反例 1**：`ZStack { mainContent; VStack { Spacer(); HStack { Spacer(); Text("watermark") } } }` —— 全屏 ZStack 把右下角 caption 盖在 mainContent 的底部交互行上，紧凑屏必踩坑。
> - **反例 2**：用 `.overlay(alignment: .bottomTrailing) { Text("v1.0") }` 直接挂在 root 上 —— 看似比 ZStack 优雅，但和 ZStack 同病：caption 仍然落在 root 底部 trailing 区域，仍可能盖在底部 CTA 上。要么挂在非交互 view 上（如 petArea），要么按 footer 处理。
> - **反例 3**：写 `.allowsHitTesting(false)` 让 caption 不吃点击但保留视觉重叠 —— 解决了点击截获，但视觉遮挡仍在；用户看到合成按钮被一行小字横切，仍是 bug。`allowsHitTesting` 是逃生通道，不是首选。
