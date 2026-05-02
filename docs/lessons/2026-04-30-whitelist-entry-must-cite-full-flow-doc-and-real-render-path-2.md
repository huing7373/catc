---
date: 2026-04-30
source_review: file:/tmp/epic-loop-review-37-14-r2.md（codex review round 2）
story: 37-14-design-package-白名单
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-30 — 白名单 r2：deferred artifact 的位置必须落到「真实承载源」而非视觉壳入口

## 背景

Story 37.14 落地 `iphone/docs/ui-design-scope-whitelist.md`。round-1 修了 entry 3（WeChat）/ entry 9（装扮 overlay）两条「位置 cite 不到完整流程 / 真实渲染机制」问题；round-2 codex 又抓到两条同质化 finding：

1. entry 10（K3M9P2 美化别名）的「位置」段把 `9X2-L8` placeholder / `JoinRoomModal` / 「3 个字母 - 2 位数字」格式说明等真实 alias artifacts 错挂到 `home.jsx line 162-183`，但该 range 是 toast + 创建/加入按钮，**不含** input field 和 modal 本身。真实 alias artifacts 全部位于 `iphone/ui_design/source/app.jsx`（`roomCode` state line 30 + 65、friends mock line 18 + 92、`JoinRoomModal` 定义 line 215-271 含 placeholder line 249 + 格式说明 line 259）。
2. entry 8（小猫 3D）的「理由」段把 ui_design prototype 的 placeholder 说成 `Image(systemName: "cat.fill")`，但 ui_design 侧 placeholder 是 SVG-based `CatPlaceholder` 组件（cat-placeholder.jsx 内 `<svg>` + `<circle>` / `<path>`）。`cat.fill` 是 **SwiftUI 实装侧**（Story 37.7 HomeView）的 placeholder。entry 是关于 ui_design scope 的，把 SwiftUI 实装的 placeholder 拼回 ui_design 的位置上，会让读者误以为节点 10 起 RealityKit / USDZ 替换的是 ui_design prototype（实际替换的是 SwiftUI 实装侧）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | entry 10「位置」段把 alias artifacts 错挂到 home.jsx 不含 input field 的 range | medium | docs | fix | `iphone/docs/ui-design-scope-whitelist.md` 条目 10 |
| 2 | entry 8「理由」段把 SwiftUI 实装的 cat.fill placeholder 错植到 ui_design prototype 描述里 | medium | docs | fix | `iphone/docs/ui-design-scope-whitelist.md` 条目 8 |

## Lesson 1: entry 10 alias artifacts 必须 cite app.jsx 而非 home.jsx 视觉入口

- **Severity**: medium
- **Category**: docs
- **分诊**: fix
- **位置**: `iphone/docs/ui-design-scope-whitelist.md` 条目 10「位置」段

### 症状（Symptom）

entry 10 cite `iphone/ui_design/source/screens/home.jsx` line 162-183 「加入队伍输入框 + 创建队伍按钮」作为 K3M9P2 / 9X2-L8 / 「3 个字母 - 2 位数字」等美化别名 artifacts 的位置。但 home.jsx line 162-183 实际是 toast 框 + 「创建队伍」（line 173）+ 「加入队伍」（line 183）按钮 —— 完全不含任何 input field 或 modal。`JoinRoomModal` 组件、`9X2-L8` placeholder、格式说明文案等真实 alias artifacts **全部在 `iphone/ui_design/source/app.jsx` 内**（line 30 / 65 `roomCode` state, line 215-271 `JoinRoomModal` 定义, line 249 `例如 9X2-L8`, line 259 `房间代码格式：3 个字母 - 2 位数字`）。读者顺着错指引去 home.jsx 找 alias 形式契约，会一无所获，从而误以为该契约不在 ui_design 中存在。

### 根因（Root cause）

写白名单条目时只查到「按钮在哪触发」就停 —— `home.jsx` line 162-183 是 modal 触发点（按钮），但 modal **定义和它的 placeholder / 格式说明文案** 在 `app.jsx` 顶层（因为 modal 由 app.jsx 管理 state，不属于 home screen）。React-style 拆分：screens/ 只放视觉壳的入口按钮，跨 screen 的 modal / 全局 state（`roomCode`）会被抬到顶层 `app.jsx`。如果 deferred artifact 涉及「跨 screen state / 全局 modal」，**单 cite 触发按钮的 screen 文件几乎一定漏掉真实承载源**。

### 修复（Fix）

把 entry 10「位置」段从单一 home.jsx range 扩到三段：

```
- 位置（ui_design）：
  iphone/ui_design/source/app.jsx
    line 30 + 65（roomCode state，初值 '7K3-P2' / fallback '9X2-L8'）
    + line 18 + 92（friends mock 数据 statusText "在房间 9X2-L8 中"）
    + line 215-271（JoinRoomModal 定义，line 249 placeholder "例如 9X2-L8"，line 259 "房间代码格式：3 个字母 - 2 位数字" 格式说明）
  + iphone/ui_design/source/screens/home.jsx line 173 + 183
    （"创建队伍" / "加入队伍" 按钮，触发 modal；该 range 不含 input field 本身）
  + iphone/ui_design/source/screens/room.jsx line 33-44
    （房间代码卡片，line 6 clipboard.writeText(roomCode) 复制；ui_design 内变量名仍是 roomCode，本期 SwiftUI 实装统一改为 roomId）
```

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在白名单条目（或任何 deferred-artifact 文档）写「位置」段时，**禁止只 cite「按钮触发点 / 视觉壳挂载入口」**，**必须**追到 modal / state / mock data 的定义文件（通常是 `app.jsx` / `index.jsx` / store/* 等顶层）。
>
> **展开**：
> - cite 一个 ui_design 元素的「位置」前，先问「这个元素的 state / 文案 / 格式契约 谁定义」。React-style 拆分：screens/* 只是视觉壳，全局 state / 跨 screen modal 抬到顶层 app.jsx。
> - 写完位置后，**用 grep 反验**：`grep -nE 'keyword1|keyword2' <cited-file>`，确认 cited file 真的含这些关键词。本例反例：`grep -nE 'JoinRoomModal|9X2-L8|3 个字母' iphone/ui_design/source/screens/home.jsx` 返回空 —— 这就是「位置错挂」的强信号。
> - **反例**：cite home.jsx line 162-183 作为 K3M9P2 美化别名 artifacts 的位置 —— 该 range 是 toast + 触发按钮，不含 modal / placeholder / 格式说明，读者顺路径找不到契约源头。

## Lesson 2: entry 描述 ui_design scope 的渲染路径时不能混入 SwiftUI 实装的 render path

- **Severity**: medium
- **Category**: docs
- **分诊**: fix
- **位置**: `iphone/docs/ui-design-scope-whitelist.md` 条目 8「理由」段

### 症状（Symptom）

entry 8（小猫 3D）「位置」段已经 cite ui_design 内 `cat-placeholder.jsx`（97 行 SVG-based 组件）作为 placeholder 承载源，但「理由」段紧接着说「Epic 37 Story 37.7 HomeView 视觉壳用 SF Symbol `cat.fill` 占位渲染」。这描述的是 **SwiftUI 实装侧** Story 37.7 HomeView 的 placeholder，不是 ui_design prototype 的 render path。两者是不同的 placeholder：

- ui_design prototype：SVG-based `CatPlaceholder`（`<svg viewBox=...>` + `<circle>` / `<path>` 勾勒头脸耳朵眼鼻嘴）
- SwiftUI 实装：`Image(systemName: "cat.fill")`

把 SwiftUI 实装的 render path 当 ui_design prototype 的 render path 写，会让读者以为节点 10 起 Story 30.x 的 RealityKit / USDZ 3D model 替换的是 ui_design prototype（其实只替换 SwiftUI 实装侧 —— prototype 本来就不上线，没有替换之必要）。

### 根因（Root cause）

ui_design prototype 和 SwiftUI 实装 是 **两条独立 render path**，但因为「都是 cat 占位」、「都为 Story 37.7」的视觉壳服务，写描述时容易把两者「合并叙述」。一旦合并，render path 的真实位置就被混淆。白名单是 ui_design scope 文档（`iphone/docs/ui-design-scope-whitelist.md` 标题第一行已经写明 scope = ui_design），entry 描述里如果出现 SwiftUI 实装的 render path 字符串（`Image(systemName: ...)` / `SwiftUI.View` / `@StateObject` 等），就是 scope 越线。

### 修复（Fix）

把 entry 8「理由」段重写，**显式区分两侧 placeholder**：

```
理由：本条目作用域是 ui_design prototype 的 placeholder 替换路径。
- ui_design 侧 placeholder = SVG-based CatPlaceholder（vector 勾勒猫头）
- SwiftUI 实装侧 placeholder（Story 37.7 HomeView 视觉壳）= Image(systemName: "cat.fill")
两者不是同一份 placeholder。
节点 10 起 Story 30.x 落地 RealityKit / USDZ 3D model 替换的是 SwiftUI 实装侧的 cat.fill 占位
（不是 ui_design prototype 的 SVG CatPlaceholder —— prototype 不上线、不需替换）。
```

并在「位置」段补一句对 cat-placeholder.jsx 真实 SVG 渲染源的精确 cite：`line 18 <svg viewBox=...>` + `line 28-36 <circle> / <path> 头脸耳朵眼鼻嘴`。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **ui_design scope** 类文档（如 `ui-design-scope-whitelist.md`）写 entry 时，**禁止**把 SwiftUI 实装侧的 render path（`Image(systemName: ...)` / `Text(...)` / `Color(...)` 等 SwiftUI API）写进描述；**必须**明确分两侧叙述（ui_design 侧 = SVG / JSX、SwiftUI 实装侧 = SwiftUI APIs）。
>
> **展开**：
> - 写描述前先问 self-check：「这一句话讲的是 ui_design 侧还是 SwiftUI 侧？」如果一段话里两侧都涉及，**必须**用「ui_design 侧 vs SwiftUI 实装侧」结构化分点，而不是合并写。
> - 节点 10+ 替换轨迹（如 RealityKit / SpriteRenderer 替换占位）必须**明示替换的是哪侧** —— 通常只替换 SwiftUI 实装侧，prototype 不上线、不替换。
> - **反例**：entry 8「理由」第一句「Epic 37 Story 37.7 HomeView 视觉壳用 SF Symbol cat.fill 占位渲染」—— 这句描述的是 SwiftUI 实装的 placeholder 形态，但被 cite 在 ui_design scope 文档的 entry 里，会让读者误以为 ui_design prototype 也用 cat.fill 占位，从而误判 3D 替换的目标。

---

## Meta: 本次 review 的宏观教训

r1 + r2 总共四条 P2 finding 全部聚焦同一个思维漏洞：**白名单 entry 的「位置」段只挂到视觉壳入口 / 触发按钮，没有追到真实承载源**（modal 定义、state 声明、SVG 真实绘制 path）；而「理由」段又容易把 ui_design 侧和 SwiftUI 实装侧的 render path 合并叙述。

这两条共性教训合成一条 SOP：**写 deferred artifact 文档前，先用 grep 反验「关键字 → 文件」的双向命中**，再把「ui_design 侧 vs 实装侧」用结构化分点显式分离 —— 视觉壳挂载入口、状态/modal 定义源、真实渲染 path 三层都要 cite，不能用其中一层代替另外两层。
