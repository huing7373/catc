---
date: 2026-05-04
source_review: codex review (epic-loop round 2) — /tmp/epic-loop-review-8-2-r2.md
story: 8-2-coremotion-接入
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-04 — MotionProvider probe view 必须显式处理 deny 路径 + UI 集成测试不能依赖 simulator 自发 emit motion event

## 背景

Story 8.2（CoreMotion MotionProvider 接入）round 2 codex review 给了 2 条 P2 finding，全部成立：

1. **[P2]** `MotionProviderProbeView` 调 `requestPermission()` 后只 catch throw，**忽略 `false` 返回值**——CoreMotion 在 deny 状态下 `requestPermission` 不抛而返 false；probe view 继续调 `startUpdates`，CoreMotion 静默不响应，UITest 30s 超时却看不到 errorLabel，failure 被掩盖。
2. **[P2]** `MotionProviderIntegrationTests` 依赖 simulator 自发 emit `CMMotionActivity` event 才让 result label 脱离 `(waiting)`——idle CoreMotion simulator 30s 都不会 emit activity，导致 UITest **flaky**：本地手摇手机时绿、CI 上不动时红。

两条都是 wiring layer 的设计漏洞，配套修复：probe view 加 deny 处理 + 加 `subscribed` status marker；UITest 改成断言 statusLabel == "subscribed" 或 errorLabel 非空。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | requestPermission 返 false 时 probe 仍调 startUpdates | P2 / medium | error-handling / wiring | fix | `iphone/PetApp/Features/DevTools/Views/MotionProviderProbeView.swift` |
| 2 | UITest 依赖 simulator 自发 emit motion event → flaky | P2 / medium | testing | fix | `iphone/PetAppUITests/MotionProviderIntegrationTests.swift` |

## Lesson 1: requestPermission 的"三态返回"不能只 catch throw

- **Severity**: medium (P2)
- **Category**: error-handling / wiring
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/DevTools/Views/MotionProviderProbeView.swift:43-52`

### 症状（Symptom）

```swift
do {
    _ = try await motionProvider.requestPermission()   // 丢弃返回值
} catch {
    errorText = String(describing: error)
}
motionProvider.startUpdates { ... }                    // deny 路径直接走到这里
```

deny 状态下：
- `requestPermission()` 返 `false` 且**不抛**；catch 块没进；errorText 留空。
- `startUpdates(handler:)` 调用对 `CMMotionActivityManager` 注册 closure → 但 manager 在未授权下"静默不响应"——既不抛错也不调 handler。
- 结果：UI 永远停在 `(waiting)` 占位 + errorText 空 + 30s UITest 超时。**和 wiring dead 完全无法区分**（round 1 P3 收紧的"二态 PASS"也救不了这条，因为 deny 状态满足"两态都没出现"）。

### 根因（Root cause）

把 `requestPermission()` 当成"二态：成功（throw 不抛 / 返 true）or 失败（throw）"使用。CoreMotion / HealthKit 这类 system permission API 实际是**三态**：

1. `throw` — 系统调用失败 / 取消（systemFailure / cancelled）
2. `return true` — 已授权 / 用户允许
3. `return false` — 已拒绝 / restricted（**不抛**，仅返 false）

第 3 态是"用户已经在系统设置里拒绝过、本次直接 deny without prompt"——这是 sim/CI 上很常见的初始状态。`startUpdates` 在该状态下**不会**抛错也不会调 handler，silent fail。

### 修复（Fix）

probe view 显式处理三态：throw → catch 写 errorText 并 return；false → 写 errorText="permissionDenied" 并 return；true → 继续到 startUpdates 注册 handler，调用完成后立即 set status="subscribed"。

```swift
// after:
let granted: Bool
do {
    granted = try await motionProvider.requestPermission()
} catch {
    await MainActor.run { errorText = String(describing: error) }
    return                                       // 不走 startUpdates
}
guard granted else {
    await MainActor.run { errorText = "permissionDenied" }
    return                                       // 不走 startUpdates
}
motionProvider.startUpdates { activity in ... }
await MainActor.run { statusText = "subscribed" }   // wiring 链路通的标志
```

顺带改动：probe view 新增 `statusText` 字段 + `motionProviderProbeStatus` a11y identifier，作为"startUpdates 调用完成"marker（lesson 2 的 UITest 断言依赖此 marker）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：写 system permission API（CoreMotion `requestPermission`、HealthKit `requestAuthorization`、`CLLocationManager` authorization）的 await 调用时，**必须**显式处理"throw / return false / return true"三态，**禁止**把 `_ = try await ...` + 仅 catch 当作完整覆盖；deny（false）路径必须当成 terminal error，**绝不**继续调订阅类 API。
>
> **展开**：
> - permission API 的"返回 false 不抛"是 Apple SDK 的常见契约——`requestPermission` 表示"我问完了系统，结果就是没授权"，throw 留给"问的过程出错"。
> - 订阅类 API（`startActivityUpdates` / `startQuery` / observer）在未授权下**不抛也不回调 handler**——silent fail 是 system adapter 的契约边界。客户端必须先确认授权再调，不能"先调再看"。
> - 未授权状态在 simulator / CI / 测试机上是**正常初始态**，dev 真机自己开了权限不代表测试环境也开。
> - 修法标准模板：
>   ```swift
>   let granted: Bool
>   do {
>       granted = try await provider.requestPermission()
>   } catch {
>       reportError(error); return
>   }
>   guard granted else {
>       reportError("permissionDenied"); return    // 显式 terminal
>   }
>   provider.startUpdates { ... }                  // 仅授权后才调
>   ```
> - **反例**：`_ = try await provider.requestPermission()` + 紧接着 `provider.startUpdates(...)`——丢掉了 false 返回值，deny 路径走到 startUpdates 后 silent fail。8.2 dev 阶段初版就是这样写的（catch 块当成"covers all errors"），review round 2 P2 暴露。
> - **关联反模式**：probe view / 集成测试只暴露"成功"和"throw"两条 a11y 路径（result label + error label），**漏掉 deny 路径的可观测性**——必须给 deny 一条独立的 errorText 文本（如 `"permissionDenied"`），UITest 才能区分"deny"和"silent fail"。

---

## Lesson 2: UI 集成测试不能依赖 system event 自发触发——必须断言 wiring 完成 marker

- **Severity**: medium (P2)
- **Category**: testing
- **分诊**: fix
- **位置**: `iphone/PetAppUITests/MotionProviderIntegrationTests.swift:75-90`

### 症状（Symptom）

round 1 P3 修订后的 UITest 把"二态 PASS"定为：
1. result label 脱离 `(waiting)` → simulator 自发 emit 了 motion activity event；
2. error label 出现 → permissionDenied / systemFailure 路径。

第 1 态依赖 `CMMotionActivityManager` 在 simulator 主动 emit `CMMotionActivity`——但 idle simulator 不会 emit activity（`stationary` 也不一定 emit，CoreMotion 只在"activity 切换"时回调）。本地开发者把模拟器手动晃一下、CI 静态运行 → 30s 都看不到事件。结果：本地绿、CI 红，**flaky**。

### 根因（Root cause）

把"happy path 验证"等同于"实际收到 system event"。但集成测试的真正目的是验证 **wiring 链路**（launch arg 解析 → probe mount → permission grant → startUpdates 调用），**不是**验证 CoreMotion 系统本身能 emit event。后者既不可控也不必要——unit test + 真机 ad-hoc 验证已经覆盖了 system 层行为。

把"系统会 emit event"当成"可以稳定依赖"是 round 1 沿袭下来的隐含假设，到 round 2 才被点出。

### 修复（Fix）

probe view 在 `startUpdates` 调用完成后立即写入 status label "subscribed"——作为 wiring 完成 marker。UITest 改成断言：

```swift
// after: 二态 PASS（不再依赖 simulator 自发 emit event）
//   1) statusLabel == "subscribed"  → wiring 全链路通（launch arg + mount + grant + startUpdates 调用完成）
//   2) errorLabel 非空              → permissionDenied / systemFailure 路径
while Date() < deadline {
    if statusLabel.label == "subscribed" { resolved = true; break }
    if errorLabel.exists, !errorLabel.label.isEmpty { resolved = true; break }
    Thread.sleep(forTimeInterval: 1.0)
}
XCTAssertTrue(resolved, "30s 内 statusLabel 既未变成 'subscribed'，errorLabel 也未出现 → wiring 死链")
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：写 system adapter 集成测试的 happy path 时，**必须**断言"adapter 自己写入的 wiring 完成 marker"（如 `subscribed` / `registered` / `started`），**禁止**断言"system 自发 emit 了某个 event"——后者是 system 层行为，集成测试不该依赖。
>
> **展开**：
> - 集成测试的边界是 **wiring**：launch arg 解析、view mount、permission grant、API 调用注册——这些是 client 代码可控的，每一步都能写 marker。
> - System 层行为（CoreMotion emit activity、HealthKit observer fire、Location update）**不属于** wiring 测试范围——它们由 OS 决定何时 emit，simulator 静态状态 + CI 无人交互时常常不 emit。
> - probe view 实装时应主动暴露"link is up"marker：startUpdates 调用完成立即设 statusLabel="subscribed"；observer 注册完毕立即设 statusLabel="registered"——这些都是 client 端可控的事件。
> - 设计 marker 时区分两类 a11y label：
>   - **status / wiring marker**：startUpdates 调用完成等 client-side milestone（必须 cover happy path）
>   - **data marker**：handler 真收到 event 后写入的具体值（仅做诊断，不做 PASS 断言）
> - **反例**：UITest happy path 断言 `resultLabel != "(waiting)"`——这要求"system 真发 event 且 handler 真跑了"。idle simulator 上 30s 不发 event 是常态，断言会 flaky；本地 dev 手摇手机绿、CI 静止红 是典型表现。
> - **关联反模式**：把"已经看到 spinner 在转"当成"已经在 fetch"——同样是依赖系统/UI 的中间态而不是 client 主动写入的 milestone marker。

---

## Meta: 本次 review 的宏观教训

两条 finding 一根：**system adapter 接入时，permission API 的"三态返回"和集成测试的"system event 依赖"是同一个心智漏洞——把 system 层行为当成可控边界**。

- Lesson 1：把 system permission 的 false 返回当成"小概率失败" → 漏处理 deny → silent fail；修法是显式处理三态、deny 当 terminal。
- Lesson 2：把 system 自发 emit event 当成"测试可依赖" → idle simulator 不发 event → flaky；修法是断言 client 端可控的 wiring marker。

两条修法都依赖**同一个工具**：在 client 端主动暴露**三类 a11y label**——data label（result）/ status label（subscribed）/ error label（permissionDenied / systemFailure）。三类标签覆盖了"系统返事件 / 系统返成功但还没事件 / 系统返失败"三种 happy/edge case，UITest 二态 PASS 才能站得住。

未来 epic 8 / 节点 3 后续接入 **CMPedometer / CLLocationManager / HealthKit observer** 时，必须在 probe view 同时设计这三类 a11y label，再写集成测试。
