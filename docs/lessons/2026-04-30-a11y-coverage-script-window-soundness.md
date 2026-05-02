---
date: 2026-04-30
source_review: file:/tmp/epic-loop-review-37-13-r3.md (codex round 3)
story: 37-13-accessibility-identifier-总表
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-30 — a11y coverage CI 脚本 window 算法 sound 性 (multi-control 同 view 的 sibling 顺势遮蔽 false negative)

## 背景

Story 37.13 是 Epic 37 的 a11y identifier 总表 + 静态校验 story，r1 / r2 已分别修过 SCAN_DIRS 盲区 / regex 不覆盖 custom wrapper / ADR layering guard。本轮 (r3) codex review 用更严格的 python 算法重扫 `iphone/PetApp` 树，指出 `iphone/scripts/check_a11y_coverage.sh` 的 window 算法仍有一类 soundness bug：**多个 interactive control 同 view 时，首个漏挂 identifier 的 control 会被下方 sibling control 的 identifier 顺势"遮蔽"为 OK**。

复现 fixture：

```swift
Button(action: { ... }) { Text("first") }                // 漏挂

Button(action: { ... }) { Text("second") }
    .accessibilityIdentifier("second")
```

旧脚本对 first Button 取 `[line, line+80]` window 检查 `accessibilityIdentifier(`，window 内有下面 second 的 identifier → 错误判 OK。

codex 用 python 重扫报 15 个位置；逐个 verify 后实际真违规只有 2 个（ProfileScaffoldView 的 `headerIconButton(bell/settings)` callsite），其余 13 个是 codex 的 i+15 window 太小的 false positive（real Button body 较长所以 identifier 没在 15 行内）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | a11y coverage 脚本 window 上限不收紧 → sibling 顺势遮蔽 → CI gate bypass | medium | testing | fix | `iphone/scripts/check_a11y_coverage.sh` |
| 2 | 暴露后真违规：headerIconButton(bell/settings) callsite 漏挂 a11y identifier | medium | testing | fix | `iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift` + `AccessibilityID.swift` |

## Lesson 1: a11y coverage 脚本 window 算法必须收紧到下一个 interactive 行 (sibling sound)

- **Severity**: medium
- **Category**: testing
- **分诊**: fix
- **位置**: `iphone/scripts/check_a11y_coverage.sh:62-81`

### 症状（Symptom）

脚本对 `Button(...)` / `TextField(...)` / `Toggle(...)` 等 interactive line 取固定 `[line, line + 80]` 行 window，扫到任意 `accessibilityIdentifier(` 即判 OK。当 view body 内有多个 sibling controls 且首 control 漏挂 identifier 时，下方 sibling 的 identifier 落在同一个 window 内 → 首 control 被错判 OK，CI gate 静默放行。

### 根因（Root cause）

window 设计是「向后扫一段固定距离」而不是「扫到当前 control 自己 body / modifier 链结束」。两个语义在多 control 同 view 时不同：

- 「固定距离」：把 sibling control 的 identifier 也算进当前 control 的 coverage。
- 「自己 body 范围」：sibling control 的 identifier 不应归当前 control。

正确做法不需要 swift-syntax 真 AST —— 一个**最小近似**就足够 sound：用「下一个 interactive line - 1」当 window 上限（min(natural_end, line + 80)）。这样首 control 的 window 在到达下一个 control 之前就截止。该近似仍有边界 bug（中间夹一个属于上 control 的 `.accessibilityIdentifier(...)` 可能误归给当前 control），但这种结构罕见且会被人评 + 配套 self-test fixture 守护。

### 修复（Fix）

`iphone/scripts/check_a11y_coverage.sh` 增量改动：

1. **window 上限收紧**：每 interactive line 算 `end = min(line + A11Y_WINDOW_LINES, next_interactive_line - 1)`。next_interactive_line 来自一次 `awk -F: '{print $1}' | sort -n` 收集的 line 列表，在循环里 `awk -v cur="$line_num" '$1 > cur { print $1; exit }'` 取下一个。

2. **过滤注释行**：`stripped=$(printf '%s' "$content" | sed 's/^[[:space:]]*//')`；以 `//` / `///` / `/*` / `*` 开头则 continue。避免 `/// - 加回 ... PrimaryButton(...) ...` 这种文档注释被误判为 callsite。

3. **过滤 helper 函数定义行**：`*' func '*'('*` 的 case continue。例：`private func headerIconButton(iconKey: String, ...) -> some View {` 不是 callsite，是定义；callsite 在别处单独命中并各自 check。

4. **加 `--self-test` 守护模式**：构造一个含「first 漏挂 + second 挂」的 fixture .swift 文件 → 跑 scan_file → 期望 violations 计数 = 1。回归（!= 1）则 self-test 失败、退出 != 0。供 CI / 本地手验「算法 sound 性没被未来重构污染」。

before / after 验证（real codebase）：
- 旧算法：`bash check_a11y_coverage.sh` → ✅ a11y coverage OK（实际 2 处真违规漏检）.
- 新算法：暴露 1 处真违规 + 2 处 false positive → 修真违规 + filter false positive → ✅ a11y coverage OK；`--self-test` 也通过。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写**逐行扫描型 lint 脚本**（grep + window）时，**必须**让 window 上限收紧到「下一个同类 token 行 - 1」而非取固定 N 行；并同时在脚本内部加一个**对抗性 fixture self-test mode**。
>
> **展开**：
> - 多个 sibling structure 共存的代码（多 Button、多 case、多 method）时，固定 window 一定会让"前一个漏挂"被"后一个挂上"顺势遮蔽 → false negative，CI gate bypassed。
> - 算法不需要真 AST，bash + awk 就能算出「下一个同类 token 行号」做 window cap。
> - 写完 lint 脚本必须配套一个 self-test mode（脚本内部放 fixture）：构造一个「首项漏挂 + 末项挂」的最小 case，跑算法应当报 1 个 violation；如果报 0 → 算法被污染了。该 fixture 在脚本里以 `--self-test` flag 触发，CI 跑 main check 之后再跑一次 self-test 守护算法。
> - Lint 脚本的 `INTERACTIVE_PATTERN` 把 helper 名（如 `PrimaryButton(` / `headerIconButton(`）加进来时，要同步 filter 掉 `private func headerIconButton(...)` 这种 helper 定义本身（pattern 是 callsite 用，定义不是 callsite）。同样 filter 注释行（`//` / `///` / `/*` / `*`）—— 否则文档里写的 `PrimaryButton(...)` 例子会被当真 callsite。
> - **反例**：`tail_window="$(awk -v start=N -v end=N+80 ... "$file")"` 然后 `grep -q 'accessibilityIdentifier('` —— sibling 顺势遮蔽 false negative；CI 静默放行。

## Lesson 2: helper callsite 不挂 identifier 时必须由 caller 注入（不要寄希望于 helper 内部）

- **Severity**: medium
- **Category**: testing / architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift:116/119` + `AccessibilityID.swift`

### 症状（Symptom）

`ProfileScaffoldView` 的 `headerIconButton(iconKey:action:)` 是个 private SwiftUI helper，body 内有 `Button(action: action) { Image(...) }`，但 helper 本身 **没有挂** `.accessibilityIdentifier`；同时 caller `headerIconButton(iconKey: "bell") { ... }` 和 `headerIconButton(iconKey: "settings") { ... }` 也都没挂 → 两个按钮在 UITest 里没法定位。

旧版 a11y check 脚本因 window 算法 bug 漏检了这两处；本轮 sound 算法暴露后必须真挂 identifier。

### 根因（Root cause）

helper 模式有两条路：(a) helper 内部挂 identifier（caller 不必挂；适合"一类按钮一个固定 id"如 PrimaryButton 整层包样式时不挂的反例）；(b) helper 不挂、caller 各自挂（适合"同 helper 多 callsite 各 id 不同"如 bell vs settings）。

`headerIconButton` 走 (b) 模式 —— iconKey 不同 callsite 不同 → identifier 必须由 caller 注入。但 review 落地时漏挂了。

### 修复（Fix）

1. `iphone/PetApp/Shared/Constants/AccessibilityID.swift` Profile enum 加两个新 key：

   ```swift
   public static let bellButton = "profileBellButton"
   public static let settingsButton = "profileSettingsButton"
   ```

2. `iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift` line 116/119 各 callsite 后加 `.accessibilityIdentifier(...)`：

   ```swift
   headerIconButton(iconKey: "bell") { state.onBellTap() }
       .accessibilityIdentifier(AccessibilityID.Profile.bellButton)
   headerIconButton(iconKey: "settings") { state.onSettingsTap() }
       .accessibilityIdentifier(AccessibilityID.Profile.settingsButton)
   ```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写 SwiftUI helper 时，**必须**显式选 (a) helper 内部挂 / (b) caller 各自挂 二者其一并文档化，且 lint 脚本要同时覆盖该 helper 的 callsite 检查（即把 helper 名加进 INTERACTIVE_PATTERN）。
>
> **展开**：
> - helper 名进入 INTERACTIVE_PATTERN 后，每个 callsite 都会被 lint 当 interactive 行检查；如果 helper 走 (b) 模式，caller 必须各自挂 identifier，否则被脚本 catch；如果 helper 走 (a) 模式，应在 INTERACTIVE_PATTERN 把它**排除**（让 callsite 不再被强校验）—— 当前选择是「全部 helper 都走 (b) 模式 + 全进 INTERACTIVE_PATTERN」，统一性优先。
> - **反例**：headerIconButton 走 (b)（caller 注入）但 bell / settings callsite 都没挂 → UITest 无法定位 → 直到 sound algorithm 暴露才发现。
> - 写新 SwiftUI helper 时，spec / dev notes 必须写"a11y 在 helper 内挂 / 由 caller 注入"二选一并把 helper 名加进 INTERACTIVE_PATTERN（caller 模式）或从 pattern 排除（内部模式）。

---

## Meta: 本次 review 的宏观教训

a11y CI 脚本的 r1 / r2 / r3 三轮 review 暴露了**lint 脚本本身需要 lint** 的元问题：

- r1 修 SCAN_DIRS 缺 App/（路径覆盖盲区）
- r2 修 INTERACTIVE_PATTERN 不覆盖 PrimaryButton / headerIconButton 等 custom wrapper（识别盲区）+ ADR layering regex 不该 token-match
- r3 修 window 算法 sibling 顺势遮蔽（语义盲区）+ helper callsite 漏挂

每轮 review 都暴露**脚本算法的 sound 性边界** —— 元教训是：**lint 脚本必须 ship 一个对抗性 self-test fixture**，把"算法该 catch 但旧版漏 catch"的最小 case 写在脚本里，每次 CI 跑 main check 之后再跑 self-test。这样后续重构脚本时"无意改坏算法"会立刻被 fixture 抓到。

类似精神：测试代码不只是检查产品代码，**关键 lint / CI 脚本本身也要被自身的 fixture 守护**。
