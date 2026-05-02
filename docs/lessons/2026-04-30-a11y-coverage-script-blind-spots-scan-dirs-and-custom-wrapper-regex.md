---
date: 2026-04-30
source_review: file:/tmp/epic-loop-review-37-13-r1.md (codex round 1)
story: 37-13-accessibility-identifier-总表
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-30 — A11y coverage CI 脚本盲区：SCAN_DIRS 缺 App/ + regex 不覆盖 custom wrapper

## 背景

Story 37.13 落地了 `iphone/scripts/check_a11y_coverage.sh` 作为 a11y identifier
覆盖度 CI 第一道防线（grep 文本匹配, 不引入 swift-syntax）。Codex round 1 review
扫到两处 blind spot：① SCAN_DIRS 漏了 `iphone/PetApp/App/`，导致 `MainTabView`
的 4 个 `tab_*` identifier 完全不在 CI 视线内；② regex 仅匹配原生 SwiftUI 控件
（Button / Toggle / TextField / NavigationLink / Picker / Slider / TextEditor），
对 `PrimaryButton` / `headerIconButton` 这类「body 内自己不挂 a11y, 全靠 caller 挂」
的 custom wrapper 未覆盖 — caller 漏挂时 CI 静默放行。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | check_a11y_coverage.sh 不 scan App/ 目录 | medium | testing | fix | `iphone/scripts/check_a11y_coverage.sh` |
| 2 | regex 不匹配 PrimaryButton / headerIconButton 等 custom wrapper | medium | testing | fix | `iphone/scripts/check_a11y_coverage.sh` |

## Lesson 1: a11y coverage 脚本的 SCAN_DIRS 漏掉 App/ 目录

- **Severity**: medium
- **Category**: testing
- **分诊**: fix
- **位置**: `iphone/scripts/check_a11y_coverage.sh:23-27`

### 症状（Symptom）

`MainTabView.swift` 落在 `iphone/PetApp/App/` 下, 含 4 个 `tab_*` a11y identifier
的 `tabButton(_)` helper, 但 SCAN_DIRS 只声明 `Features/` + `Shared/Modals/`,
脚本根本不打开 `MainTabView.swift`. 后续 PR 若在 App/ 下加任何交互控件并漏挂
identifier, CI 静默 green.

### 根因（Root cause）

写 lint 脚本时按「业务 view 都在 Features/ 下」的直觉划 SCAN_DIRS, 没考虑到
SwiftUI 工程结构里 App entry 层（root container, tab bar, 全局 sheet host）
也是合规 view 落地点之一。当一个目录被 hardcode 进 SCAN_DIRS 列表而不是从
工程中按规则枚举时, 任何「该被 scan 但漏报」的目录都靠人脑记忆补 — 容易遗漏。

### 修复（Fix）

在 SCAN_DIRS 数组顶部追加 `$REPO_ROOT/iphone/PetApp/App`：

```bash
SCAN_DIRS=(
    "$REPO_ROOT/iphone/PetApp/App"          # ← 新增
    "$REPO_ROOT/iphone/PetApp/Features"
    "$REPO_ROOT/iphone/PetApp/Shared/Modals"
)
```

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **写 a11y / lint 类目录扫描脚本** 时，**必须**
> **把工程内所有「可能落地 SwiftUI View 的目录」都列进 SCAN_DIRS, 至少包含 App
> entry 层 + Features 层 + Shared/Modals 层**。
>
> **展开**：
> - SwiftUI 工程的 view 至少落在 3 类位置：① App entry（root container, tab bar,
>   global modal host, 如 `App/MainTabView.swift` `App/RootView.swift`）；② 业务
>   feature view（`Features/<Feature>/Views/`）；③ 跨 feature 共享 view（`Shared/`,
>   尤其 `Shared/Modals/`）. 三类都得 scan, 缺一类就有盲区.
> - 写 SCAN_DIRS 时优先考虑「从 PetApp/ 子目录里枚举 — 排除 Resources / Assets /
>   Constants 等纯数据目录」, 而不是「列举我能想到的几个 view 目录」. 默认收口
>   比默认放行安全.
> - **反例**：`SCAN_DIRS=("$ROOT/Features" "$ROOT/Shared/Modals")` 漏掉 `App/`,
>   导致 `MainTabView` 的 tab a11y id 不在 CI 视线内. 任何 PR 在 App/ 下新增
>   不挂 identifier 的 Button, CI green —— 直到生产 UITest fail 才发现.

## Lesson 2: a11y 脚本的 INTERACTIVE_PATTERN 必须区分「self-attach helper」与「caller-attach wrapper」

- **Severity**: medium
- **Category**: testing
- **分诊**: fix
- **位置**: `iphone/scripts/check_a11y_coverage.sh:29-32`

### 症状（Symptom）

`PrimaryButton(...)` / `headerIconButton(...)` 等 custom wrapper 内部包了一层 SwiftUI
`Button { ... }` 但 wrapper body **自身不挂 `.accessibilityIdentifier(...)`**,
约定由 **caller 挂**. 但脚本的 INTERACTIVE_PATTERN 只匹配原生 `Button(`/`Button{`/
`Toggle(`/... — caller 漏挂 identifier 时, 脚本根本看不到 callsite, CI 静默放行.

注意区分：`tabButton(_)` 也是 helper, 但 helper body 内部已挂
`.accessibilityIdentifier(AccessibilityID.Tab.identifier(for: tab.rawValue))`,
这种「self-attach helper」**不需要** 加进 regex（caller 漏挂也 OK, helper 兜底）.
真正需要加进 regex 的是「caller-attach wrapper」.

### 根因（Root cause）

写 INTERACTIVE_PATTERN 时只想「匹配 SwiftUI 原生交互控件」, 没分类「项目内自定义
wrapper 是否在 body 内自挂 a11y id」这两类语义. 项目演化中 PrimaryButton
从 `Shared/Components/` 长出来后, lint 脚本的 regex 没同步更新 — 脚本和组件库的
契约「谁挂 a11y」漂移, lint 失效悄无声息.

### 修复（Fix）

把 `PrimaryButton` 和 `headerIconButton` 加进 regex（前置 `(^|[^A-Za-z_])` 避免匹配
到 identifier 上下文中的子串, 例如 `MyPrimaryButton` 不会误匹配 `PrimaryButton`）：

```bash
INTERACTIVE_PATTERN='(^|[^A-Za-z_])(Button[[:space:]]*\(|Button[[:space:]]*\{|Toggle[[:space:]]*\(|TextField[[:space:]]*\(|TextEditor[[:space:]]*\(|NavigationLink[[:space:]]*\(|Picker[[:space:]]*\(|Slider[[:space:]]*\(|PrimaryButton[[:space:]]*\(|headerIconButton[[:space:]]*\()'
```

并在脚本头注释里写清「此 pattern 包含两类：原生 SwiftUI 控件 + caller-attach wrapper；
后者每次新增同模式 wrapper 必须 同步追加 wrapper 名」.

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **加 lint 脚本扫 a11y / 其他注解** 时，**必须为
> 「caller 必须自挂注解」的 custom wrapper 把 wrapper 名加进 INTERACTIVE_PATTERN**;
> 「helper body 内已自挂注解」的 helper 则**不要**加进 regex（避免 caller 处误报）.
>
> **展开**：
> - 项目里组件库长出 custom wrapper（`PrimaryButton` / `SecondaryButton` /
>   `IconActionButton` ...）后, lint 脚本要 explicit 决定「谁挂 a11y」并锁死契约.
>   两种契约可选：A. wrapper body 自挂（helper 内部 `.accessibilityIdentifier(...)`,
>   caller 不需关心）→ 脚本不扫 callsite. B. caller 挂（wrapper 透明转发, caller
>   `.accessibilityIdentifier(...)` 在 wrapper 调用之后）→ 脚本必须扫 callsite.
> - 选哪种没标准答案, 但**必须二选一并固定**. 同一类 wrapper 内不同 caller 走不同
>   契约 = 灾难（lint 写不准）.
> - 写 lint 脚本头注释时**列出当前 caller-attach wrapper 清单**, 任何新增同类
>   wrapper 必须同步追加到 regex 并更新清单. 否则 6 个月后 review 才能发现盲区.
> - 前置 `(^|[^A-Za-z_])` 必须保留 — 否则 `MyPrimaryButton(` 会被错误识别成
>   `PrimaryButton(`.
> - **反例**：lint regex 只列 SwiftUI 原生 `Button|Toggle|TextField|...`, 项目内
>   `PrimaryButton(...)` 漏挂 `.accessibilityIdentifier(...)` 时 CI green; 上线
>   后 UITest 用 `app.buttons["xyz"]` 找不到按钮才发现盲区. lint 静默通过比 lint
>   报错更危险, 因为团队会以为 baseline OK.

---

## Meta: 本次 review 的宏观教训

两条 finding 指向同一个思维漏洞：**lint 脚本的覆盖范围（SCAN_DIRS + INTERACTIVE_PATTERN）
是有状态的, 必须随工程结构演化同步 — 但实际上一旦写完没人看, 容易和工程漂移**.

抽象规则：**任何「当前覆盖什么 / 不覆盖什么」由 hardcode list 控制的 lint 脚本,
都必须在脚本头部注释里 explicit 声明「未覆盖范围」+「何时该补」, 并在新增同模式
artifact（新目录 / 新 wrapper）时强制同步**. 缺这道纪律, lint 静默失效是必然.

未来 Claude 加新 component / 新顶层目录时, 应先**反查所有 lint 脚本**(grep
`scripts/check_*.sh`), 看 SCAN_DIRS / PATTERN 是否覆盖. 不要假定 lint 自动 keep up.
