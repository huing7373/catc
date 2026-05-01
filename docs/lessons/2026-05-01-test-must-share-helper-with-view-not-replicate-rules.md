---
date: 2026-05-01
source_review: codex r2 review of Story 37.12 (file: /tmp/epic-loop-review-37-12-r2.md)
story: 37-12-joinroommodal-跨屏跳转
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-01 — 单测复刻 view 内规则等于零守护：必须把规则下沉到纯函数 helper 与 view 共用

## 背景

Story 37.12 落地 `JoinRoomModal` 时，单元测试在 ADR-0002 §3.1（禁用 ViewInspector / SnapshotTesting）约束下，写法是**直接调 `modal.onConfirm("…")` 闭包**并**在测试体内本地复刻 `trim` / `prefix(64)` 规则**。codex r2 review 指出这等于零守护：view body 内 `action: { onConfirm(trimmed) }` / `.disabled(trimmedIsEmpty)` / `.onChange { 截断 64 }` 三处规则若发生回归（例如改成不 trim 直接转发原值、或改成不截断长度），测试**仍然 pass**。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 测试本地复刻 trim/截断规则 → view body 共享规则回归不抓 | medium (P2) | testing | fix (option A) | `iphone/PetApp/Shared/Modals/JoinRoomModal.swift` + `iphone/PetAppTests/Shared/Modals/JoinRoomModalTests.swift` |

## Lesson 1: 单测复刻 view 内规则 = 零守护：把规则下沉到纯函数 helper 与 view 共用

- **Severity**: medium (P2)
- **Category**: testing
- **分诊**: fix (option A)
- **位置**: `iphone/PetAppTests/Shared/Modals/JoinRoomModalTests.swift:28-29`（旧）

### 症状（Symptom）

JoinRoomModalTests 的 case 写法是：

```swift
let trimmed = "  abc-123  ".trimmingCharacters(in: .whitespacesAndNewlines)
modal.onConfirm(trimmed)
XCTAssertEqual(capturedRoomId, "abc-123")
```

或：

```swift
let truncated = longInput.count > 64 ? String(longInput.prefix(64)) : longInput
XCTAssertEqual(truncated.count, 64)
```

测试体内做 trim / 截断，然后把结果当成 view 行为去断言。view body 内 `action: { onConfirm(trimmed) }`、`.disabled(trimmedIsEmpty)`、`.onChange { 截断 64 }` 任意一处回归（例如改成 `action: { onConfirm(roomIdInput) }` 不 trim、或 `.onChange` 删掉），测试**全部仍 pass**——因为测试断言的是测试本地变量算出的字符串值，不是 view 调用的字符串值。

### 根因（Root cause）

ADR-0002 §3.1 禁用 ViewInspector / SnapshotTesting 后，常见的诱惑是「在测试里手动复刻 view body 内逻辑然后断言计算结果」。这种写法的根本错误：**测试和 view 没有共享同一个被测函数**——测试在断言 `Foundation.trimmingCharacters` 的行为（已被 Foundation 自身覆盖），而**不是**断言 `JoinRoomModal` 在 confirm 按下时是否真的调了 trim。view body regression 与测试断言完全脱节。

正确思路：在不能直接断言 view body 时，把 view body 内的**规则**抽出来放到**纯函数 helper（namespace + static func）**，view 直接调 helper、tests 直接断言 helper —— 测试与 view **共享同一函数源**，断言 helper 等价于断言 view 行为（因为 view 直接调 helper，无中间转换）。这是 codebase 既有的 `HomeRoomDispatcher.shouldShowRoom(currentRoomId:)` / `HomePetNameResolver.resolve(pet:hasHydrated:)` 模式。

### 修复（Fix）

新增 `iphone/PetApp/Shared/Modals/JoinRoomInputNormalizer.swift`：

```swift
public enum JoinRoomInputNormalizer {
    public static func normalize(_ raw: String) -> String {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        return String(trimmed.prefix(64))
    }
    public static func isSubmitDisabled(_ raw: String) -> Bool {
        normalize(raw).isEmpty
    }
}
```

`JoinRoomModal.swift` body 内三处共用规则全部改为直接调 helper：

```swift
// .onChange
.onChange(of: roomIdInput) { _, newValue in
    let normalized = JoinRoomInputNormalizer.normalize(newValue)
    if normalized != newValue { roomIdInput = normalized }
}
// confirm button
PrimaryButton(
    ...,
    isDisabled: JoinRoomInputNormalizer.isSubmitDisabled(roomIdInput),
    action: { onConfirm(JoinRoomInputNormalizer.normalize(roomIdInput)) }
)
```

`JoinRoomModalTests.swift` 重写：删掉所有「测试本地 trim / 截断后断言计算结果」的 case，改为直接断言 `JoinRoomInputNormalizer.normalize(...)` / `.isSubmitDisabled(...)` 的输入输出。覆盖：trim 前后空白 / 内部空白保留 / 换行 Tab / 长度边界 (==64, >64) / trim 与 prefix 顺序 / 空 / 全空白 / 含换行全空白 / 非空 enable / 含空白非空 enable —— 共 12 case helper 测试 + 2 case modal closure 透传守护 + 4 case ViewModel 守护 = 18 case（旧 9 → 新 18）。

build 结果：`bash iphone/scripts/build.sh --test` → 342 tests passed (旧 333 → 新 342, +9 case)，0 failure。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **iOS / SwiftUI 项目下、ADR-0002 §3.1 禁用 ViewInspector / SnapshotTesting 时**，**禁止**在测试体内**复刻 view body 内规则然后断言计算结果**；**必须**把规则**抽到纯函数 helper（`public enum FooNormalizer { static func ...}`）**让 view 与 tests **共用同一函数源**。
>
> **展开**：
> - 触发场景：view body 内有 `.onChange { 转换 input }` / `.disabled(isXxx)` / `action: { onConfirm(transformed) }` 等**确定性纯函数规则**且测试想守护这些规则不被回归。
> - 必做：抽 `public enum XxxNormalizer { public static func normalize(_:) -> ... ; public static func isXxxDisabled(_:) -> Bool }`，view body 与 tests 都直接调；测试断言 helper 输入输出，不再在测试体内复刻规则。
> - 已有同模式参考：`HomeRoomDispatcher.shouldShowRoom(currentRoomId:)`（`HomeContainerView.swift`）/ `HomePetNameResolver.resolve(pet:hasHydrated:)`（`HomeView.swift`）/ `HomeNicknameResolver`。
> - **反例 1**（不抓 trim 回归）：测试写 `let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines); modal.onConfirm(trimmed); XCTAssertEqual(captured, trimmed)`。这里测试断言的是 Foundation `trimmingCharacters` 的恒等性，不是 view 是否调了 trim。view body 把 `action: { onConfirm(trimmed) }` 改成 `action: { onConfirm(roomIdInput) }`（不 trim），此 case 仍 pass。
> - **反例 2**（不抓截断回归）：测试写 `let truncated = longInput.count > 64 ? String(longInput.prefix(64)) : longInput; XCTAssertEqual(truncated.count, 64)`。这是断言 `String.prefix(64).count == 64`（Swift stdlib 性质），与 view 是否在 `.onChange` 内做截断完全无关；view 删掉 `.onChange` 块此 case 仍 pass。
> - **反例 3**（不抓 disabled 回归）：测试写 `let empty = "".trimmingCharacters(...); XCTAssertTrue(empty.isEmpty)`。这是断言 `String.isEmpty` 的性质，不是 view 是否真的把 `trimmedIsEmpty` 接到 `PrimaryButton.isDisabled`。view 改成 `isDisabled: false` 写死此 case 仍 pass。
> - **正例**：抽 `JoinRoomInputNormalizer.normalize(_:)` / `.isSubmitDisabled(_:)`，view body 三处直接调 helper；测试断言 `XCTAssertEqual(JoinRoomInputNormalizer.normalize("  abc  "), "abc")` —— view body 删 trim → helper 没动 → view 行为偏离 helper → 但更关键的是：view body 在编译期就要求字面量调 `JoinRoomInputNormalizer.normalize(...)`，不可能"半改"，重构者要么完整删 helper 调用（那 grep 就抓得到）要么必须改 helper（那测试就 fail）。
> - **附加**：跑 `grep -rn "trimmingCharacters\|prefix(64)" path/to/View.swift` 应**只**匹配到 helper 内部一份；view body 不应直接出现这些字面规则。

## Meta: 本次 review 的宏观教训

ADR-0002 §3.1 关掉 ViewInspector / SnapshotTesting 之后，「能直接断言 SwiftUI body 的工具被禁了」是事实，但**这不等于「单测无法守护 view body 行为」**——对于「确定性纯函数规则」类型的 view body 行为（trim / 截断 / disabled 判定 / 派生展示文本），**正确路径是把规则下沉到 helper 让 view 调用，而不是在测试体内复刻**。这条规则放大成 codebase 级原则后，过去几个月已经反复被 review 提示过（HomeRoomDispatcher / HomePetNameResolver / HomeNicknameResolver 都是同一精神）；新视图 / 新规则落地时，"先想 helper 再写 view body" 应该是默认动作。
