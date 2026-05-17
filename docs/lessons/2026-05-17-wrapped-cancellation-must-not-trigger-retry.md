---
date: 2026-05-17
source_review: "file: /tmp/epic-loop-review-24-2-r1.md (codex review, epic-loop story 24-2 round 1)"
story: 24-2-loadinventoryusecase-get-cosmetics-inventory-调用
commit: a3736ad
lesson_count: 1
---

# Review Lessons — 2026-05-17 — 被网络层包裹的取消不能误弹 RetryView（catch is CancellationError 漏掉 APIError.network(URLError.cancelled)）

## 背景

Story 24.2 在 `WardrobeView` 接「Wardrobe Tab 出现 → `LoadInventoryUseCase.execute()`」，
触发机制是 `.task(id: coordinator.currentTab)`：切走 Tab 时 SwiftUI cancel in-flight task，
loading 可取消、不阻塞 Tab 切换。AC5 明确「CancellationError 静默吞」。
codex review 24-2 r1 指出实装只 `catch is CancellationError`，漏了 `APIClient` 把
`URLSession` 取消包成 `APIError.network(URLError.cancelled)` 这条主路径 →
用户正常切走 Tab 会误弹全屏 RetryView，破坏「不阻塞 Tab 切换」语义。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 切走 Tab 触发的取消被网络层包裹后误弹 RetryView | medium (P2) | error-handling | fix | `iphone/PetApp/Features/Wardrobe/Views/WardrobeView.swift:74-110` |

## Lesson 1: 被网络层包裹的取消不能落进通用 catch 误判为失败

- **Severity**: medium（review 标 P2）
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Wardrobe/Views/WardrobeView.swift:76`（修复前的 `catch is CancellationError`）

### 症状（Symptom）

用户在 inventory 请求 in-flight 时离开 Wardrobe Tab，`.task(id:)` cancel 任务。
`URLSession.data(for:)` 抛 `URLError(.cancelled)`，但 `APIClient.request` 的
`catch let urlError as URLError { throw APIError.network(underlying: urlError) }`
（`APIClient.swift:139-140`）把它包成 `APIError.network(URLError.cancelled)` —— **不**是裸
`CancellationError`。`WardrobeView.loadInventory()` 只 `catch is CancellationError`，
这条取消落进通用 `catch` → `errorPresenter.present(...)` → 弹全屏 RetryView，
但用户只是切了个 Tab。

### 根因（Root cause）

写 SwiftUI `.task(id:)` 取消处理时，默认心智模型是「取消 = `CancellationError`」。
但**取消可以被中间层重新分类**：网络层（`APIClient`）按「URLSession 抛 URLError → 一律
归 `.network`」的统一策略，把 `URLError(.cancelled)` 和 `URLError(.timedOut)` 同等
包成 `APIError.network`。取消的「身份」在穿过网络层后从 `CancellationError` /
`URLError.cancelled` 变成了 `APIError.network(underlying:)`，只按最外层类型
`catch is CancellationError` 必然漏判。判定「这是不是取消」必须**下钻 underlying**
到 `URLError.Code.cancelled`，不能只看最外层错误类型。

### 修复（Fix）

`WardrobeView` 加 testable static helper `isSilentCancellation(_:)`，统一识别三种取消形态，
`loadInventory()` 的 catch 改成「先问 helper 是否静默取消 → 是则 return，否则弹 RetryView」：

```swift
static func isSilentCancellation(_ error: Error) -> Bool {
    if error is CancellationError { return true }
    if let urlError = error as? URLError, urlError.code == .cancelled { return true }
    if case let .network(underlying) = (error as? APIError),
       let urlError = underlying as? URLError,
       urlError.code == .cancelled {
        return true
    }
    return false
}
```

- 形态 1：裸 `CancellationError`（Swift Concurrency 结构化取消）
- 形态 2：裸 `URLError(.cancelled)`（防御性，未经 APIClient 包裹直达）
- 形态 3：`APIError.network(URLError.cancelled)`（**本 finding 主路径**）
- **关键边界**：只下钻 underlying 是否恰为 `.cancelled`，**不**把所有 `APIError.network`
  当取消 —— 否则 timeout / 离线 / DNS 失败会被一起静默掉，用户切到 Wardrobe 永远看不到
  RetryView（over-correction，下一轮 review 必反弹）。
- 守门测试 `testIsSilentCancellationClassifiesAllCancellationFormsButNotRealNetworkFailures`
  （`WardrobeViewScaffoldTests.swift` case#12）：3 种取消形态断言 true，
  4 个反例（`.network(timedOut)` / `.network(notConnectedToInternet)` / `.business` /
  `.decoding`）断言 false，把契约钉成机器可校验。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写 `.task` / async 任务的 cancel 静默处理且链路里有网络层（APIClient/URLSession）** 时，**禁止**只 `catch is CancellationError` —— **必须**额外识别「被网络层包裹的取消」（`APIError.network(underlying:)` 内 `URLError.code == .cancelled`），且**禁止**因此把所有 `APIError.network` 都静默吞。
>
> **展开**：
> - 取消会被中间层重新分类：`URLSession` 取消 = `URLError(.cancelled)`，`APIClient` 按「URLError → 一律 .network」统一策略包成 `APIError.network`。判「是不是取消」要**下钻 underlying** 到 `URLError.Code.cancelled`，不能只看最外层 case。
> - 识别取消和「吞掉错误」是两件事：silent 只对**确证是取消**的错误生效；真实 transient 失败（timeout / offline / DNS）必须照常走 RetryView/error 态。窄判定（`code == .cancelled`）而非宽吞（整个 `.network`）。
> - 改完必须加守门测试，**正反都断言**：所有取消形态 → 静默(true)；至少 2 个真实网络失败 + 业务/解码错误 → 不静默(false)。只断言取消那一半会让 over-correct（宽吞 .network）测试照过。
> - **反例 1**：`catch is CancellationError { /* silent */ } catch { present(error) }` —— 漏掉 `APIError.network(URLError.cancelled)`，切 Tab 误弹 RetryView。
> - **反例 2**：`if case .network = error { return }` —— 把 timeout / 离线也静默掉，用户切到页面永远看不到 RetryView（过度修复，比原 bug 更隐蔽）。
