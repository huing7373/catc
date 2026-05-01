---
date: 2026-04-30
source_review: /tmp/epic-loop-review-37-12-r1.md (codex P1)
story: 37-12-joinroommodal-跨屏跳转
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-30 — UITest 自补 helper 与 XCTest SDK 方法 redeclaration

## 背景

Story 37.12 round 1 review。dev_story 在 `iphone/PetAppUITests/HomeUITests.swift` 末尾给 `XCUIElement` 加了一个 `func waitForNonExistence(timeout:) -> Bool` 的扩展，注释写「SwiftUI / XCUI 标准库无此方法，自补」。事实上 `XCUIElement` SDK 自带同名同签名方法（`XCUIElement.h` 行 60 + `XCUIAutomation.apinotes` 显式映射 `waitForNonExistenceWithTimeout:` → Swift `waitForNonExistence(timeout:)`）。结果 UITest target 编译失败 invalid redeclaration。

dev_story 跑 `bash iphone/scripts/build.sh --test`（仅 unit）通过 333/333，**没编 UITest target**，所以自查没拦住这条 P1。codex review 在独立审视 diff 时识别。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | UITest 自补 `waitForNonExistence(timeout:)` 与 SDK 同名方法 redeclaration | high | testing | fix | `iphone/PetAppUITests/HomeUITests.swift:435-443` |

## Lesson 1: UITest helper 切勿重新声明 XCTest / XCUIAutomation SDK 已有方法

- **Severity**: high
- **Category**: testing
- **分诊**: fix
- **位置**: `iphone/PetAppUITests/HomeUITests.swift:435-443`

### 症状（Symptom）

UITest target 编译失败（invalid redeclaration of `waitForNonExistence(timeout:)`），任何 UITest 跑批 / Xcode 打开 PetAppUITests 都被卡住。但只跑 `--test`（unit）不会暴露这条编译错。

### 根因（Root cause）

写 helper 时假设「SwiftUI / XCUI 标准库无此方法，自补」是错的。`XCUIElement` 在 `XCUIAutomation.framework/Headers/XCUIElement.h:60` 暴露 `- (BOOL)waitForNonExistenceWithTimeout:(NSTimeInterval)timeout;`，并通过 `XCUIAutomation.apinotes` 显式 SwiftName 为 `waitForNonExistence(timeout:)`。同样在 `XCTest.framework/Headers/XCTest.apinotes` 也有此映射。属于 SDK 长期 API。

加 helper 时没真验「这个方法是不是已经存在」，仅基于直觉 / 类比 `waitForExistence(timeout:)`。验证手段（grep apinotes / 查 XCUIElement.h）都很便宜，但被跳过。

更深一层：Story 37.12 dev_story 自验证只跑 `--test`（unit tests），这条命令链路不接 UITest target，因此 UITest 编译错没在本地拦住。Story 涉及 UITest 改动时，**最低验证应包含 `--uitest`**（或至少把 UITest target 编进去）。

### 修复（Fix）

删掉 `iphone/PetAppUITests/HomeUITests.swift:435-443` 整段 extension（注释 + extension block 共 9 行）。第 420 行处 `modal.waitForNonExistence(timeout: 3)` 调用直接落到 SDK 自带方法，行为等价。

before：
```swift
}

// Story 37.12 AC6: waitForNonExistence helper（SwiftUI / XCUI 标准库无此方法，自补）.
// 与 Apple Sample Code 同精神.
extension XCUIElement {
    func waitForNonExistence(timeout: TimeInterval) -> Bool {
        let predicate = NSPredicate(format: "exists == false")
        let expectation = XCTNSPredicateExpectation(predicate: predicate, object: self)
        return XCTWaiter().wait(for: [expectation], timeout: timeout) == .completed
    }
}
```

after：扩展段整段删除；call site `modal.waitForNonExistence(timeout: 3)` 不变，落到 SDK 实现。

`grep -rn "func waitForNonExistence" iphone/PetAppUITests/` 确认全仓库只有这一处 redeclaration，无其他文件需同步删。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写 XCTest / XCUIAutomation 的 helper extension** 时，**必须**先 grep `XCUIAutomation.framework/Headers/*.h` 与 `*.apinotes`（或 LSP / Xcode 自动补全）确认目标方法**不存在于 SDK**，再决定是否自补。
>
> **展开**：
> - `XCUIElement` 已有的 wait 系：`waitForExistence(timeout:)` 与 `waitForNonExistence(timeout:)` 都是 SDK API，**不要**重新声明。
> - 验证手段（任选其一即可，<5 秒）：
>   - `find /Applications/Xcode.app/Contents/Developer/Platforms/*.platform -name 'XCUIElement.h' | xargs grep -l '<methodName>'`
>   - `find /Applications/Xcode.app/Contents/Developer/Platforms/*.platform -name '*.apinotes' | xargs grep '<SwiftName>'`
>   - 或在 Xcode 里直接对实例 `.<methodName>` 自动补全，看 SDK 是否提示
> - 真要扩展 SDK 类型时（确实 SDK 无此方法），命名加一个独有前缀避免未来 SDK 升级冲突，例如 `func cat_waitFor...` 或扩展到子类。
> - **反例**：基于「我记得 SwiftUI / XCUI 没这个」直觉就直接 `extension XCUIElement { func sameNameAsSDK(...) }`，在编译期撞 invalid redeclaration。
>
> **配套验证规则**：Story 改动涉及 `iphone/PetAppUITests/**` 时，dev_story 自验证**不能**只跑 `bash iphone/scripts/build.sh --test`，**至少**要补一次 `bash iphone/scripts/build.sh`（仅 build，把 UITest target 编进去）或 `--uitest`，确认 UITest target 编译通过；不然 P1 编译错只会到 codex review 阶段才暴露。
