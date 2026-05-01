---
date: 2026-04-30
source_review: /tmp/epic-loop-review-37-10-r1.md (codex review of Story 37.10 round 1)
story: 37-10-friendsview-scaffold
commit: b87d373
lesson_count: 1
---

# Review Lessons — 2026-04-30 — Spec 边界灰区的 fallback 路径在 review 触发时必须立即兑现 epic AC

## 背景

Story 37.10 实装 FriendsScaffoldView。Epic 37 AC line 4807 字面钦定 myRoomCard 含「分享给好友」次要按钮（占位 toast，本 epic 不真分享）。SM 在 spec 关键决策 2 / Dev Notes 把它列为**边界灰区**：默认实装路径**不渲染**该按钮（理由：减少占位噪声 + ui_design friends.jsx 视觉里也没有），但显式声明 fallback —— 「若 review 反馈要求保留 → 在 fix-review round 加（一行 `PrimaryButton(variant: .secondary)` 即可；a11y identifier `friendsMyRoomShareButton`）」。

Round 1 codex review 援引 epic AC 字面与 Story 37.13 a11y 锚要求，flag 漏了该按钮。这是 spec 钦定 fallback 的**预期触发**，不是 spec 漏洞。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | FriendsScaffoldView myRoomCard 漏「分享给好友」按钮（spec 边界灰区 fallback 触发） | medium (P2) | architecture / ui-fidelity | fix | `iphone/PetApp/Features/Friends/Views/FriendsScaffoldView.swift` |

## Lesson 1: Spec 边界灰区的 fallback 路径在 review 触发时必须立即兑现 epic AC

- **Severity**: medium (P2)
- **Category**: architecture / ui-fidelity
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Friends/Views/FriendsScaffoldView.swift:95-140` + `iphone/PetApp/Features/Friends/ViewModels/FriendsViewModel.swift:71-79`

### 症状（Symptom）

myRoomCard（`state.currentRoomId != nil` 时渲染的"我的房间"提示条）只有 paw icon + room code 静态展示，**漏渲染 epic AC line 4807 钦定的「分享给好友」次要按钮**。Story 37.13 a11y 总表 / Story 37.12 后续真实 share flow 都假设该按钮存在。

### 根因（Root cause）

**spec 边界灰区在实装时倾向"砍占位按钮"** —— SM 写关键决策 2 时给了两条候选路径：
1. 默认路径：**不渲染**该按钮（减少占位噪声 + 跟 ui_design friends.jsx 视觉一致）
2. fallback 路径：若 review 反馈要求保留 → 加一行 `PrimaryButton(variant: .secondary)`

dev 选了路径 1（spec 默认推荐）。codex review 援引 epic AC 字面要求 flag 漏按钮 —— 这是 spec **预期的 fallback 触发**（spec 字面写了 "若 review 反馈要求保留 → 在 fix-review round 加"），不是 dev 路径选错。

**核心思维结构**：spec 边界灰区设计本质是"二选一委托给 review 决断"。dev 选默认路径 + review 触发 fallback → 系统按设计运转，不算 bug，但需要在 fix-review round 立即兑现 epic AC，不能再次推迟（再推会让 Story 37.13 a11y 总表 / Story 37.12 真实 share flow 都依赖一个尚未渲染的锚）.

### 修复（Fix）

**1. ViewModel 加 concrete (非 abstract) method `onShareMyRoomTap()`**（`FriendsViewModel.swift`）

```swift
public func onShareMyRoomTap() {
    self.lastToastMessage = "分享功能敬请期待"
}
```

**关键设计选择**：`onShareMyRoomTap` 是基类 **concrete** method（不是 abstract），让 Mock / Real 共享行为 —— 主动规避 lesson `2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md` 反模式（abstract + fatalError 路径在 production 注入路径下漏 override 即 crash）。本 epic 红线"UI Scaffold 数据完全 mock + 不调真实 share flow"决定了 Mock/Real 在该方法上没有分化需求。

**2. View 层 myRoomCard 改 VStack（顶部 HStack + 底部按钮）**（`FriendsScaffoldView.swift`）

```swift
VStack(spacing: 8) {
    HStack(spacing: 10) { /* paw icon + 你的房间 + roomId */ }
    PrimaryButton(
        title: "分享给好友",
        variant: .secondary,
        icon: Icons.symbol(for: "wechat"),
        fullWidth: true,
        action: { state.onShareMyRoomTap() }
    )
    .accessibilityIdentifier("friendsMyRoomShareButton")
}
```

**a11y identifier 用 spec 钦定的 `friendsMyRoomShareButton`**（命名风格与 `friendsMyRoomCard` 同 prefix；review 建议的 `friendsInRoomCard_share` 不采纳 —— spec 优先于 review 建议，review 给的命名是建议而非钦定）。

**3. 加守护测试 case#12**（`FriendsViewScaffoldTests.swift`）

- Mock 路径：`onShareMyRoomTap()` 写 `lastToastMessage == "分享功能敬请期待"`
- Real 路径（parameterless init + bind init 两条）：调用不 fatalError + 写同一文案

防未来 Claude 重构时把 `onShareMyRoomTap` 改成 abstract（重蹈 fatalError 反模式）或漏掉占位 toast 文案。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **spec 显式声明的"边界灰区 + fallback 路径"** 被 review 触发时，**必须**在当前 fix-review round 立即兑现 epic AC 钦定行为，**不**推迟到下一个 story。
>
> **展开**：
> - **识别"spec 边界灰区"信号**：spec Dev Notes / 关键决策段含 "若 review 反馈要求 X → 在 fix-review round 加 Y" / "保留这个边界灰区让 dev / reviewer 在实装时根据 Z 做最终决定" / "默认路径 A 但若 fallback 路径 B" 等措辞 —— 这些是 SM 显式委托给 review 决断的二选一节点
> - **review 触发时的兑现节奏**：fix-review round 1 必须落地 fallback 路径（不要等 round 2 / 不要让 reviewer 重复 flag）—— 否则下游 story（如 a11y 总表 / 真实 share flow）会基于不存在的锚做规划
> - **a11y identifier 命名优先级**：spec 钦定 > review 建议（review 给的命名是 informative 而非 normative；spec 钦定的命名通常已与现有命名风格 align）—— 本 case spec 钦定 `friendsMyRoomShareButton`，review 建议 `friendsInRoomCard_share`，按 spec 走
> - **占位行为在边界灰区里 prefer concrete 不是 abstract**：如果占位行为在本 epic 红线范围（如"不调真实 UseCase"），在基类直接 concrete 实装，不要走 abstract + fatalError 路径 —— 否则未来重构 caller 时漏 override 直接 production crash（lesson `2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md` 教训）
> - **反例**：dev 看到 spec 默认路径推荐"不渲染按钮"就只实装默认路径，把 fallback 路径当"未来如果需要可以加"延期 → 后续 story（37.12 真实 share flow / 37.13 a11y 总表）做时发现锚点不存在，要回头改 Story 37.10 的代码 + 改测试 + 改 spec 三处，scope 回流到已 done 的 story 是反模式
> - **反例 2**：把 review 给的 a11y identifier 字面采纳（哪怕 spec 已钦定不同命名）—— 让 a11y 命名在 codebase 里出现两套风格（`friendsMyRoomCard` 与 `friendsInRoomCard_share` 并存），未来 a11y 总表收口时要做归并
> - **反例 3**：占位行为加 abstract + fatalError，理由是"未来真实流程上线时强制子类 override" —— 等真实流程上线时改 abstract 是单点改动；现在加 abstract 让 Mock/Real 立刻要写两份占位实装 + 漏 override 直接 crash，是过早抽象

## Meta: 本次 review 的宏观教训

**spec 写 fallback 路径是 SM 的显式契约 —— review 触发 fallback 不是 spec 失败而是 spec 设计的运行**。dev 看到这种"二选一让 review 决断"的边界灰区时，**默认路径选错不丢人**（SM 自己也没拍板），但 fallback 触发时必须立即兑现 —— 这是 SM 把决策权委托给 reviewer 的契约语义。把 fallback 兑现推到下一个 story 等于背叛这个契约。
