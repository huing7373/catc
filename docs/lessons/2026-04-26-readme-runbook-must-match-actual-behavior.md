---
date: 2026-04-26
source_review: codex review round 2 of /tmp/epic-loop-review-2-10-r2.md
story: 2-10-ios-readme-模拟器开发指南
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-26 — Onboarding README 的 runbook 必须与工具语义 / 实际 UI 文案对齐

## 背景

Story 2-10 给 `iphone/README.md` 加了一份新 dev 入门 / 模拟器联调指南，第 r2 轮 codex review 发现两处事实性偏差：① troubleshooting 步骤里给读者的 `xcodebuild` 手动 destination workaround 写法实际不可执行；② 联调成功标志的 UI 文案描述与 `HomeView.swift` 实际渲染不一致。两条都属于"runbook 自身有 bug"，照做的人会得到错误结果或 false negative。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | xcodebuild destination workaround 混合 `name` + UUID 不可执行 | medium (P2) | docs | fix | `iphone/README.md:327` |
| 2 | HomeView footer 成功标志文案与实际 UI 不符 | low (P3) | docs | fix | `iphone/README.md:352` |

## Lesson 1: xcodebuild `-destination` 字段语义 —— 要么 id，要么 name+OS+platform 三件套

- **Severity**: medium (P2)
- **Category**: docs
- **分诊**: fix
- **位置**: `iphone/README.md:327`

### 症状

troubleshooting 表第 2 行（iPhone 17 不存在时的手动 fallback）原文：
> 如需手指定：`xcrun simctl list devices iOS available` 找 UUID 后传 `name=<your-device>`

读者照做会失败：先让用户找了 **UUID**，下一句却让用户传 `name=<device>`。两条路径混在一起，且 `name=<device>` 缺 `OS=` / `platform=` 字段，xcodebuild 会报 `Unable to find a destination matching the provided destination specifier`。

### 根因

`xcodebuild -destination` 的字段语义是：**要么** `id=<UUID>` 单独成立，**要么** `platform=iOS Simulator,name=<device>,OS=<X.Y>` 三件套同时给。写 README 时没区分这两条路径，把"找 UUID"和"用 name"两步串起来当成一条流程，结果产出一个语法上不存在的格式（只有 `name=<device>` 单字段）。本质：**Apple 工具链文档稀薄，写 runbook 时容易把"通过 UUID 找设备"误当成"识别设备"，再用 name 字段写命令；其实 UUID 找完就直接 `id=<UUID>` 用，name 路径独立**。

### 修复

把 troubleshooting 行改成单一 UUID 路径 + 显式说明字段语义：

```
如需手指定：① xcrun simctl list devices iOS available 找你想用机型的 UUID；
            ② 直接传 id=<UUID>，例如 -destination "platform=iOS Simulator,id=<UUID>"（推荐）。
xcodebuild 的 -destination 字段语义要求要么 id=<UUID> 单独成立，要么 name=<device>,OS=<X.Y>
三件套同时给；只给 name=<device> 缺 OS 字段会报 Unable to find a destination。
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 README / runbook 里给 `xcodebuild -destination` 手动指定示例时，**必须只给一条单一路径**：要么 `id=<UUID>`，要么 `platform=iOS Simulator,name=<device>,OS=<X.Y>` 三件套，**禁止**只给 `name=<device>` 单字段或把"找 UUID"和"用 name"步骤串成一条流程。
>
> **展开**：
> - 涉及 Apple 工具链命令的 runbook，写完后必须 mental dry-run 一次：把命令 copy 出来逐字段读，每个字段在工具语义里是否独立成立。
> - 当 README 步骤是"先 X 后 Y"的两步流程时，X 和 Y 必须用同一对象。如果 X 找的是 UUID，Y 必须使用这个 UUID（不是另一个属性）。
> - **反例 1**：写 `xcrun simctl list devices ... 找 UUID 后传 name=<your-device>`（找的是 UUID 但用了 name 字段，路径错位）。
> - **反例 2**：写 `-destination "name=iPhone 16"`（缺 `platform=` / `OS=`，xcodebuild 拒绝）。
> - **正例**：`-destination "platform=iOS Simulator,id=ABC-1234"` 或 `-destination "platform=iOS Simulator,name=iPhone 16,OS=17.0"`。

## Lesson 2: 联调 / 验证步骤的"成功标志"必须复读实际 View 渲染源码

- **Severity**: low (P3)
- **Category**: docs
- **分诊**: fix
- **位置**: `iphone/README.md:352`

### 症状

README 写"成功标志：HomeView 角落显示 `App v<...> · Server <8 位 commit>`"。但实际 `HomeView.swift:145` 渲染的是 `Text("v\(viewModel.appVersion) · \(viewModel.serverInfo)")`，`serverInfo` 在 ping 成功时是裸 short commit，offline 时是字符串 `"offline"`，解析异常时是 `"v?"`。所以真实显示是 `v1.0.0 · abc12345`（不是 `App v1.0.0 · Server abc12345`），README 描述的"App ... Server ..." 模板里的两个标签词 (`App` / `Server`) 在 UI 上根本不存在。读者按 README 找不到 "Server" 这两个字会以为联调失败。

### 根因

README 起草时把 footer 文案"凭直觉"写出来，没复读 View 的渲染代码。HomeView 的设计是简洁的 `v<ver> · <serverInfo>`（只有 `·` 分隔符 + `v` 前缀），而不是带显式标签 `App` / `Server` 的语义模板。**根本性问题**：UI 文案描述属于"代码状态投影"，runbook 写到这类点时必须 grep 实际 SwiftUI Text / Label 源码，不能凭对设计意图的记忆。

### 修复

把成功标志改成复述 ViewModel 三态：

```
成功标志：HomeView 右下角版本标签从 `v0.0.0 · ----` 变成 `v<X.Y.Z> · <8位commit>`
（X.Y.Z 是 iPhone App 版本号、8 位 commit 是 server 的 git short hash）；
server 未启则显示 `v<X.Y.Z> · offline`；
server 启了但 /version 解析异常会显示 `v<X.Y.Z> · v?`。
```

新文案三大改进：① 显式给出**变化前**初始态 `v0.0.0 · ----`（让读者知道"看到啥说明 ping 还没回")；② 三种结束态全列（成功 / offline / v?）；③ 完全复述 ViewModel/View 的实际字符串拼接，不杜撰 `App` / `Server` 标签词。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 onboarding doc / runbook 里写"成功标志：UI 上显示 X" 这类**期望文案**时，**必须** grep View 层（SwiftUI `Text(...)` / UIKit `setText(...)` / Web `innerText` / 等）找到实际渲染源码，**逐字符**对齐 README 描述，**禁止**凭设计意图记忆杜撰带语义标签的版本（如 "App ... Server ..."）。
>
> **展开**：
> - 验证 / 联调步骤的"成功标志"是 runbook 最关键 bit —— 读者拿这个判断"我做对了没"。一字之差就让人怀疑联调失败。
> - 写之前先 `grep -rn "Text(" <view-dir>` 或读 ViewModel 的 `@Published` 投影逻辑，把字符串拼接表达式（如 `"v\(version) · \(info)"`）字面照抄到 README，再把 `\(...)` 换成读者视角的占位符 `<...>`。
> - 列出**所有可能终态**（成功 / 失败 / 异常），不只列成功态 —— 失败态文案让读者排查问题。
> - **反例**：README 写 `App v1.0.0 · Server abc12345` 但 View 渲染 `v1.0.0 · abc12345`（杜撰了 `App` / `Server` 标签词）。
> - **正例**：README 写 `v<X.Y.Z> · <8位commit>`（完全照抄渲染表达式，只把变量插值符换成尖括号占位符）。

---

## Meta: 本次 review 的宏观教训

两条 finding 指向同一个思维漏洞：**onboarding doc 是"代码外的代码"，必须与代码 / 工具链语义保持一致**。Story 2-10 是纯文档 story，但文档里嵌的命令、UI 文案描述都是有"语义合约"的 —— 命令必须能跑、文案必须能匹配。写 runbook 时的检查清单：

1. 每条命令例子：copy 出来 mental dry-run，确认 CLI 字段语义合法。
2. 每个"成功标志" / "应该看到 X" 的描述：grep 对应代码源（View / log 输出 / API response shape），逐字段比对。
3. 链接路径：runbook 跨平台运行时（macOS / Linux / 不同 cwd）路径假设是否成立 —— 这条已有先例 lesson `2026-04-26-readme-portable-paths-and-relative-links.md`。

把这三步当作 Tech writer skill 的固定 verify checklist，写文档时不偷懒走完，比 review 时被找出来便宜得多。
