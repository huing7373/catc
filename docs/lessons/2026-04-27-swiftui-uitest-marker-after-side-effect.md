---
date: 2026-04-27
source_review: codex review round 1 for Story 5-1-keychain-封装 (file: /tmp/epic-loop-review-5-1-r1.md)
story: 5-1-keychain-封装
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-27 — UITest a11y marker 必须在副作用 returned 之后才进入 view tree

## 背景

Story 5.1 KeychainServicesStore 落地，配套 `KeychainPersistenceUITests` 在模拟器上验证跨 launch 持久化。
Hook view (`KeychainUITestHookView`) 通过 `launchEnvironment` 触发 keychain.set / keychain.get，把"完成信号"
通过 hidden `Text` + `accessibilityIdentifier` 暴露给 XCUIApplication。codex round 1 [P2] 指出原实装把 marker
Text 与触发 onAppear 写在同一分支：Text 永远渲染，副作用在 `.onAppear` 异步执行 —— `waitForExistence` 看到
element 时 keychain 操作未必已 returned，造成 UITest 在 busy simulator 上 flake。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | UITest a11y marker 必须 gate 在副作用 returned 之后 | medium (P2) | testing | fix | `iphone/PetApp/App/KeychainUITestHookView.swift:42-57` |

## Lesson 1: UITest a11y marker 必须 gate 在副作用 returned 之后

- **Severity**: medium (P2)
- **Category**: testing
- **分诊**: fix
- **位置**: `iphone/PetApp/App/KeychainUITestHookView.swift:42-57`

### 症状（Symptom）

`KeychainPersistenceUITests.testKeychainPersistsAcrossAppLaunches` 在 busy simulator 上间歇失败：

- 第一次 launch（KEYCHAIN_TEST_SEED）—— `waitForExistence` 命中 `uitest_keychain_seed_done` 后 `terminate()`，
  但 keychain.set 还没 return；下一次 launch 读不到 seed value。
- 第二次 launch（KEYCHAIN_TEST_READBACK）—— `waitForExistence` 命中 `uitest_keychain_readback_value` 时
  `readbackValue` 仍是 `@State` 的 initial 空字符串；`label` 读到 `""` 而非种入的 uid，断言 fail。

### 根因（Root cause）

SwiftUI 中 `Text(...)` 在 body 求值阶段就被加入 view tree → a11y 树立刻可见；
而 `.onAppear { ... }` 的闭包是在 element appeared 之后**异步**执行的（main runloop 调度），
绝非"渲染前先跑完"。原实装两个 marker 都用 `Text(...).accessibilityIdentifier(...).onAppear { keychain... }`
模式，UITest 用 `waitForExistence` 探测 element 出现 ≠ "副作用完成" —— 是个**先后假设错误**。

更深层：把"a11y element 出现"当成"副作用完成信号"是一个常见 anti-pattern。正确语义是
"a11y element 出现 ⇒ 副作用已 returned"，需要让 marker view 仅在 flag = true 之后才进入 view tree。

### 修复（Fix）

把 hook view 拆成"触发器" + "marker"两块：

- 触发器：`Color.clear.frame(width: 0, height: 0).onAppear { 副作用; flag = true }`
- marker：`if flag { Text(...).accessibilityIdentifier(...) }`

before（关键片段，省略 #if/import）：

```swift
if let seedUid = seedUid {
    Text("seed-done")
        .accessibilityIdentifier("uitest_keychain_seed_done")
        .opacity(0.001)
        .onAppear {
            try? container.keychainStore.set(seedUid, forKey: ...)
        }
}
```

after：

```swift
if let seedUid = seedUid {
    Color.clear.frame(width: 0, height: 0)
        .onAppear {
            try? container.keychainStore.set(seedUid, forKey: ...)
            seedDone = true
        }
}
if seedDone {
    Text("seed-done")
        .accessibilityIdentifier("uitest_keychain_seed_done")
        .opacity(0.001)
}
```

readback 同理：`Color.clear` 触发器写完 `readbackValue` + 翻 `readbackDone` flag → marker `if readbackDone { Text(readbackValue)... }`。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **用 SwiftUI a11y element 给 UITest 暴露"完成信号"** 时，
> **必须** **把 marker view 包在 `if doneFlag { ... }` 后面，先在 `.onAppear` 内同步跑完副作用 → 翻 flag → 下一帧 marker 才入 view tree**。
>
> **展开**：
> - `Text(...).onAppear { sideEffect }` 模式下，element 加入 a11y 树发生在 `sideEffect` 执行**之前**；
>   UITest `waitForExistence` 拿到 element ≠ 副作用 returned。这是先后假设错误，不是时序"运气"问题。
> - 拆"触发器 + marker"两块：触发器用 `Color.clear` 占位承接 `.onAppear`，marker 用 `if flag { ... }`
>   gate；二者通过 `@State` flag 串起来，flag = true 后下一帧 marker 进 view tree、a11y 才宣告完成。
> - 同理适用于：将"操作结果"通过 `Text(value)` label 暴露给 UITest 比对——必须等 `value` 已被赋值
>   （通过 `if doneFlag` gate）才让 Text 出现，否则 UITest 可能读到 initial empty state。
> - **反例**：把 `Text("done").onAppear { try? someStore.write() }` 当 "写完后 Text 才出现" 的信号 ——
>   实际是 "Text 先出现、写在不久后异步执行"。busy simulator / 慢硬件上极易踩。
> - 不要靠 `sleep(0.1)` 给 UITest "等一下让副作用跑完"——那只是把概率推回 99.x%，不解决根因。
