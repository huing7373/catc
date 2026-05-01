---
title: Real 子类 override abstract method 的"占位实装"必须 mutate state，不能只 log（否则 production no-op）
date: 2026-04-30
severity: 1
category: architecture, swift, ui-state
commit: 7094e69
related_stories: [37-9-wardrobeview-scaffold]
related_lessons: [2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md]
lesson_count: 1
---

# Review Lessons — 2026-04-30 — Real 子类 override 占位必须 mutate state（37-7 lesson 复犯）

## 背景

Story 37-9 落地 `WardrobeScaffoldView` + `WardrobeViewModel` abstract base + `MockWardrobeViewModel` / `RealWardrobeViewModel` 双子类。

`MockWardrobeViewModel.onEquipTap` 实装了本地 toggle `equipped[item.category]` 映射；
`RealWardrobeViewModel.onEquipTap` 仅 `os_log`，**不** mutate `equipped`，注释里写"Real 路径下 equipped 应当由 server EquipUseCase 成功后写入；本期保持 seed"。

但 `RootView` `@StateObject wardrobeViewModel = RealWardrobeViewModel()` 走 Real 子类。后果：production app 里用户点"装备/已装备"按钮 —— 预览区按钮 label / grid cell 对勾 badge / 装备状态全部 **不变**（按钮完全 no-op）。

单测 / Preview 走 `MockWardrobeViewModel` 路径覆盖不到本 bug（Mock 实装了 toggle）。

codex review 一眼抓住：**"the primary interaction on the new wardrobe screen non-functional outside previews/tests"**。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | RealWardrobeViewModel.onEquipTap 只 log 不 mutate equipped | P1 | architecture | fix | `iphone/PetApp/Features/Wardrobe/ViewModels/RealWardrobeViewModel.swift:122-126` |

## Lesson 1: Real 子类 override abstract method 的占位实装必须 mutate state

- **Severity**: P1
- **Category**: architecture / swift / ui-state
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Wardrobe/ViewModels/RealWardrobeViewModel.swift:122-126`

### 症状（Symptom）

- production app（`RootView` 走 Real 子类）下用户点"装备/已装备"按钮：
  - 按钮 label 不变（不切换 "装备" ↔ "✓ 已装备 (点击卸下)"）
  - grid cell 右上对勾 badge 不变
  - 预览区 active item 不更新装备状态
- Mock 路径（Preview / 单测）一切正常，掩盖 prod bug

### 根因（Root cause）

把 abstract method "占位 stub" 误解成 "什么都别干，只 log" —— 没意识到 production 注入路径走的是 Real 子类。

具体思维误区：
- "本 story 范围不调 server / UseCase" → 推到 "那就什么也别做" → 推到 "只 log 占位" → 写出 no-op override。
- 漏想一层：**Mock 子类同样不调 server，但它实装本地 toggle 让 UI 视觉工作**；Real 子类**也应该**实装等价的本地行为，等待未来 Story 落地真实 UseCase 时**替换**（不是新增）。

抽象表达：abstract method 的"占位 override"语义不是"空实装"，而是"本 story 范围内能让 UI 工作的最小 placeholder 行为"。Server 真实写入是**未来 story 的事**，但**本 story 范围内 production app 必须可用** —— 用户启动 app 不会因为"本 story 不接 server"就接受按钮 no-op。

**这是 Story 37-7 [P1] RealHomeViewModel.onJoinTap 同模式 lesson**（`2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md`）的**第二次复犯** —— 上一轮的 lesson 已经写明 "Real 子类的 abstract method override 必须实装能让 UI 视觉工作的占位"，但 Story 37-9 dev 实装时没把这条规则跨 story 传染过来。

### 修复（Fix）

`RealWardrobeViewModel.onEquipTap` override 实装本地 toggle（与 `MockWardrobeViewModel.onEquipTap` 同语义）：

```swift
public override func onEquipTap(item: CosmeticItem) {
    os_log(.debug, "RealWardrobeViewModel.onEquipTap (Story 27.1 will wire EquipUseCase) %{public}@", item.id)
    guard item.owned else { return }
    if equipped[item.category] == item.id {
        equipped[item.category] = nil  // 卸下
    } else {
        equipped[item.category] = item.id  // 装备
    }
}
```

注：review 建议代码用了 `equipped.contains(item.id)` / `insert / remove` set 语义，但 base class 实际 `equipped` 是 `[CosmeticCategory: String]` dict —— 按 base class 钦定调整为 dict 语义。

加守护测试 `testRealWardrobeViewModelOnEquipTapTogglesEquipped`（`WardrobeViewScaffoldTests.swift` case#11）覆盖三件事：
1. owned + 未装备 → 装备（equipped[bow] = b2）
2. owned + 已装备同 id → 卸下（equipped[bow] = nil）
3. unowned → equipped 不变（owned guard 保留）

把契约钉成机器可校验的测试 —— 未来 Claude 重构若把 override 改回 no-op，本测试立即 fail。

顺带改动：
- `WardrobeViewModel.swift` base class doc：`equipped` / `onEquipTap` 注释更新为"Real 子类也 mutate 本字段（占位）"
- `RealWardrobeViewModel.swift` 文件头注释：从"override onEquipTap 为占位 stub（仅 print log）" 改成"override onEquipTap：本地 toggle equipped 映射 + log（占位）"
- `WardrobeViewScaffoldTests.swift` case#7 注释更新（行 106-108）：删除"Real 路径不改 equipped"过期注释

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写 Real 子类对 abstract method 的 override 时**，**必须实装"本 story 范围内能让 UI 视觉工作的最小 placeholder 行为"**，**禁止只 log**。Server / UseCase 的真实写入是未来 story 的事，**本 story 范围内 production app 必须可用**。
>
> **展开**：
> - **判断标准**：拿本 story 的 production app 走 Real 路径触发该 method —— UI 是否立即视觉反馈？若否（按钮 no-op / 图标不变 / 状态文本不更新）→ override 缺占位行为。
> - **占位行为来源**：通常**与 Mock 子类同语义 copy**。Mock 子类是"用户视角能用"的最小实装，Real 子类的占位应该等价（差异只在"未来 server 落地后 Real 替换为真实 UseCase 调用"）。
> - **占位 → 真实的迁移路径**：未来 story 落地真实 UseCase 时，把"本地直接 mutate state" 改成 "调 UseCase → 成功后写 appState.xxx → 通过 sink 派生 state"。这是**替换不是新增**，所以本 story 写占位不会形成 tech debt（dev notes 写清楚迁移点即可）。
> - **守护测试必加**：`testRealXxxViewModelOnYyyTapMutatesZzz` —— 把"override 不能 no-op"契约钉成测试。未来 Claude 重构时若改回 log-only，测试 fail。
> - **反例 1**：`RealWardrobeViewModel.onEquipTap` 仅 `os_log` + 注释"Real 路径下 equipped 应当由 server 写入" → production no-op。✗
> - **反例 2**：把"Real 路径不调 server"等价于"Real 路径什么都别干" → 漏掉"production 必须可用"约束。✗
> - **反例 3**：lesson 已存在但**没有跨 story 传染** —— 上一轮 lesson 文档在 `docs/lessons/`，但本 story SM 写 spec 时只**引用** lesson 名（"Story 37.7 / 37.8 沉淀 lesson 预防性应用"），没把"Real 子类 override 必须 mutate state"作为 AC 子条目写进 acceptance criteria。结果 dev 实装时把"占位"误解成"空实装"。✗

## Meta: 为什么这条 lesson 第二次复犯

Story 37-7 round 1 lesson `2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md` 解决的是 **base class 留 fatalError 不被 override** 的 P1（用户点按钮 crash）。

Story 37-9 SM 把这条 lesson **预防性写进了** `RealWardrobeViewModel.swift` 文件头注释（行 12-14：`基类 onEquipTap 是 fatalError 占位，用户点装备按钮即 crash`）—— 所以 dev **正确地** override 了 onEquipTap，避免了 fatalError。

但 dev 把 override **写成了纯 log**（fatalError 的反面 = 啥也不做） —— 没意识到原始 lesson 的**完整语义**是"override 必须实装本 story 范围内的 placeholder 行为"，不是"override 必须存在但可以 no-op"。

抽象规则：**lesson 的精神在跨 story 传染时容易蒸发，只剩字面规则**（"必须 override" → 写了 override 就完事，忘了 override 内必须有占位行为让 UI 工作）。

### 元规则

把"Real 子类 override 必须实装最小占位行为让 production 可用" 写进 SM AC 模板，**和 RealXxxViewModel 同时新建时强制配套钦定**：

> AC：所有 abstract method 在 Real 子类的 override **必须包含本地 mutate state 的占位行为**（与 Mock 子类同语义），让本 story 范围内 production app 走 Real 路径时 UI 立即视觉反馈。守护测试覆盖每个 override 的 mutate 行为（防回归）。
