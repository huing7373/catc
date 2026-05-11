---
date: 2026-05-11
source_review: codex review (epic-loop round 8) — /tmp/epic-loop-review-12-7-r8.md
story: 12-7-创建-加入-退出-use-case-主界面入口完善
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-11 — 业务错误码 fallback 必须 forward 原 error（禁止 rewrap 成空 message/requestId）

## 背景

Story 12.7 落地 `JoinRoomUseCase` 到 `RealHomeViewModel.onJoinRoomConfirm` 与
`RealFriendsViewModel.onJoinFriendTap`。catch block 对 business code 做 case-by-case
中文文案 mapping（6001/6002/6003/6005/1002 → `presentAlert(title:message:)`），其他
code 走 "透传给 ErrorPresenter 默认 mapper" 的 fallback 分支。

round 8 codex review 发现：fallback 实现写成
`presenter?.present(APIError.business(code: code, message: "", requestId: ""))`
—— 解构 `catch let APIError.business(code, _, _)` 时把 `message` 和 `requestId`
都用 `_` 丢弃，然后**合成一个新的 APIError.business** 喂给 presenter。结果
`AppErrorMapper.localizedMessage` 在未知 code（如 9999）走 default 分支
`fallback.isEmpty ? "操作失败，请稍后重试" : fallback` —— fallback 是空串 → 用户看到
generic 文案，丢失 server 真实解释 + telemetry requestId。两处 callsite 同源问题，
都是 P2。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | RealHomeViewModel join fallback lossy rewrap | P2 | error-handling | fix | `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift:249-251` |
| 2 | RealFriendsViewModel join fallback lossy rewrap | P2 | error-handling | fix | `iphone/PetApp/Features/Friends/ViewModels/RealFriendsViewModel.swift:167-168` |

## Lesson 1: 业务错误码 fallback 必须 forward 原 error，禁止 rewrap 成空 message/requestId

- **Severity**: P2 (×2 同源)
- **Category**: error-handling
- **分诊**: fix
- **位置**:
  - `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift:249-251`（onJoinRoomConfirm）
  - `iphone/PetApp/Features/Friends/ViewModels/RealFriendsViewModel.swift:167-168`（onJoinFriendTap）

### 症状（Symptom）

`JoinRoomUseCase` 抛出 unrecognized business code（不在 hardcoded `[6001, 6002,
6003, 6005, 1002]` mapping）时：

- 原 error 是 `APIError.business(code: 9999, message: "Server-defined explanation",
  requestId: "req-abc")`
- 但 catch fallback 把它合成 `APIError.business(code: 9999, message: "", requestId: "")` 给
  presenter
- `AppErrorMapper.localizedMessage(forBusinessCode: 9999, fallback: "")` 走 default 分支
  `fallback.isEmpty ? "操作失败，请稍后重试" : fallback`
- 用户看到 generic "操作失败，请稍后重试"，不是 server 真实文案；telemetry 也丢了 `requestId`

### 根因（Root cause）

`catch let APIError.business(code, _, _)` 这个语法看起来是"我只关心 code"，但实际上
**整个 error 已经被解构消费**了 —— 想再访问 message/requestId 必须重新合成或者用别的
catch 模式。当时 dev 选了"合成新的 APIError.business"作为"透传给默认 mapper"的实现，但
合成时 message/requestId 默认填空串 —— 这就丢了 server 原文。

正确做法是 catch 捕获整个 error（`catch { ... }`），然后用 `if case let
APIError.business(code, _, _) = error` 做模式匹配解构 code，但 **error 本身仍然在
scope 内可以 forward**。

### 修复（Fix）

两处 callsite 重写 catch 结构：

```swift
// before（丢 message/requestId）
} catch let APIError.business(code, _, _) {
    let message: String? = { switch code { case 6001: ...; default: nil } }()
    if let message {
        presenter?.presentAlert(title: "提示", message: message)
    } else {
        presenter?.present(APIError.business(code: code, message: "", requestId: ""))
    }
} catch {
    presenter?.present(error)
}

// after（forward 原 error）
} catch {
    if case let APIError.business(code, _, _) = error {
        let message: String? = { switch code { case 6001: ...; default: nil } }()
        if let message {
            presenter?.presentAlert(title: "提示", message: message)
        } else {
            // 透传**原** error（含原 message + requestId）给 AppErrorMapper
            presenter?.present(error)
        }
    } else {
        presenter?.present(error)
    }
}
```

**回归测试**（两处 callsite 各加一个）：

- `RealHomeViewModelTests.testOnJoinRoomConfirmUnknownBusinessCodeForwardsServerMessage`
- `RealFriendsViewModelTests.testOnJoinFriendTapUnknownBusinessCodeForwardsServerMessage`

mock useCase 抛 `APIError.business(code: 9999, message: "Server-defined message",
requestId: "req-abc")` → 断言 `presenter.current` 的 `.alert` message 等于
`"Server-defined message"`（而不是 generic `"操作失败，请稍后重试"`）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **ViewModel catch block 想做 case-by-case business code
> mapping + fallback 透传给 ErrorPresenter** 时，**禁止**用 `catch let
> APIError.business(code, _, _)` 然后**合成新的** `APIError.business(code:, message:
> "", requestId: "")` 喂给 presenter；**必须**用 `catch { if case let
> APIError.business(code, _, _) = error { ... }; presenter?.present(error) }`
> 把**原** error forward 给 AppErrorMapper（保留 server message + requestId）。
>
> **展开**：
> - `AppErrorMapper.localizedMessage(forBusinessCode:fallback:)` 对未知 code 走
>   `fallback.isEmpty ? "操作失败，请稍后重试" : fallback`，所以**空 fallback message
>   会被 generic 文案吃掉** —— 这是丢 server 解释的具体机制
> - server requestId 是 telemetry / bug 复现的关键字段；rewrap 时填空串等于在错误路径
>   上抹掉调试线索，比丢文案更难恢复
> - 一律先 `catch { }` 捕获 Error → `if case let .business(code, _, _) = error`
>   做 code 解构 → 命中 hardcoded mapping 用 `presentAlert(title:message:)`；
>   miss 时 `presenter?.present(error)`（forward 原 error，**不**合成新的）
> - **反例 1**：`catch let APIError.business(code, _, _) { presenter?.present(
>   APIError.business(code: code, message: "", requestId: "")) }` — 同上 root cause,
>   生产红线
> - **反例 2**：`catch let APIError.business(code, msg, rid) { presenter?.present(
>   APIError.business(code: code, message: msg, requestId: rid)) }` — 虽然技术上不丢
>   数据，但**毫无必要**的 rewrap 制造维护负担（未来若 APIError 加字段必须同步改 rewrap
>   点）；直接 `presenter?.present(error)` 更简洁正确
> - **正例**：`catch { if case let APIError.business(code, _, _) = error { ...; else
>   { presenter?.present(error) } } else { presenter?.present(error) } }` — 既能
>   按 code 做特化文案，又能在 unrecognized case forward 原 error

## Meta: 本次 review 的宏观教训

两处 callsite 同源问题 —— 一处 dev 写错，另一处显然是参照前者复制粘贴。后续在
review 阶段如果发现某种 anti-pattern，**立刻 grep 全仓**找有没有第二个 callsite 用了
同样模式（`grep -rn "APIError.business(code: .*, message: \"\""` 这种正则能直接 catch
住 rewrap 模式）。本轮 codex 主动找到第二处是 review 价值的体现。
