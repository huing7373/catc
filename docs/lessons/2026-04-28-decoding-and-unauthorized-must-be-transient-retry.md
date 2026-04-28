---
date: 2026-04-28
source_review: codex review (epic-loop round 9) — /tmp/epic-loop-review-5-5-r9.md
story: 5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-28 — `.decoding` / `.unauthorized` 必须按 transient 二分原则归到 `.retry`，不是 `.alert`；以及 9 轮 fix-review 累积出的 transient/terminal 二分判则

## 背景

Story 5.5 codex review **round 9**。前 8 轮把 bootstrap 错误处理 pipeline 从"throw String"一路改到"mapper → ErrorPresentation 三态 → RootView 分发到 RetryView/AlertOverlay/TerminalErrorView"。round 8 [P1] fix 把 bootstrap 路径下的 `.alert` 渲染改成 `TerminalErrorView`（静态全屏页、无任何按钮、user 必须主动 force-quit），同时收敛 mapper 文案不再带"持续失败时请杀进程"后缀，让指引文本上移到 view 层静态文本。

round 9 review 抓到的 [P2] finding：

> `AppErrorMapper.swift:123-126` 把 `.decoding` 分类成 `.alert` 让 transient decoding error（server partial rollout）卡 `TerminalErrorView` 要求杀进程，应该 retryable。同时建议把 `.unauthorized` exhausted 也改 `.retry`（transient 友好）。

review 的判断和我自己迭代到 round 8 时的"force-quit only"决策方向冲突 —— round 5/8 都钦定 `.unauthorized` exhausted 是 terminal（"AuthRetryingAPIClient 静默重登失败 = 不可恢复"），round 8 又把 `.decoding` 钦定为 terminal（"schema drift = client 必须更新"）。round 9 重新审视后，承认这两条判断**过度悲观**：

- `.decoding` 真有 transient 子集（server partial rollout / 一次性坏 payload / CDN 缓存毒）
- `.unauthorized` exhausted 不等于"重启都救不了"——重跑整个 bootstrap closure（cold-start GuestLoginUseCase + LoadHome）仍可能恢复，毕竟 401 抽风是 server-side 而非本地态

把 user trap 在 force-quit only 屏幕代价过高，应该优先给 in-app 自助恢复入口；即便 retry 失败再次看到，多发一次请求也比"必须杀进程"温柔。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `.decoding` / `.unauthorized` 必须归 transient → `.retry` | P2 | error-handling | fix | `iphone/PetApp/Shared/ErrorHandling/AppErrorMapper.swift`, `iphone/PetAppTests/Shared/ErrorHandling/AppErrorMapperTests.swift`, `iphone/PetAppTests/App/AppLaunchStateMachineTests.swift`, `iphone/PetAppTests/Core/DesignSystem/Components/ErrorComponentSnapshotTests.swift` |
| 2 | meta：9 轮 fix-review 后提炼 transient/terminal 通用二分判则 | meta | error-handling, process | meta | — |

## Lesson 1: `.decoding` / `.unauthorized` 不能盲钦定为 terminal —— transient possible 必须 → `.retry`

- **Severity**: P2
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/Shared/ErrorHandling/AppErrorMapper.swift:108-150` (case `.unauthorized` / case `.decoding`)

### 症状（Symptom）

冷启动 `/home` / `/auth/guest-login` 返：

- `APIError.unauthorized`（`AuthRetryingAPIClient` 已 exhaust 唯一一次静默重登尝试）→ mapper 派 `.alert`（"登录失败，请重新启动应用"）→ RootView 渲染 `TerminalErrorView`（无按钮）→ user 唯一出路 force-quit。
- `APIError.decoding(underlying:)`（server partial rollout / 一次性坏 payload）→ mapper 派 `.alert`（"数据异常，请稍后重试"）→ 同上 force-quit。

两种场景的实际故障域大概率是 transient（401 抽风 / server canary 翻车），但 UI 不给 user 任何 in-app 恢复入口，强制 force-quit 是过度悲观。

### 根因（Root cause）

round 5 / round 8 的"按 APIError case 一对一硬绑 presentation"决策思路把"client 第一次失败"当成"重启都救不了"。具体：

- round 5 [P1] fix 钦定 `.unauthorized` 走 `.alert`：理由是"AuthRetryingAPIClient 已经 exhaust 唯一一次静默重登 → 业务层接到的 401 必然 terminal"。
- round 8 [P1] fix 钦定 `.decoding` 走 `.alert`：理由是"V1 §4.1 schema 已 frozen，出现 decoding 错就是 schema drift，client 必须更新"。

两条判断**只看了 fail 的最坏情况**（永久 401 / client 版本过旧），没看 fail 的 transient 子集（401 抽风 / partial rollout）。当时的 mental model 是"宁可让 user 重启也别让他卡 retry loop"，但 round 9 重新审视后发现：

- 让 user **主动**点重试再失败一次 ≠ retry loop（loop 的定义是自动重试，user 主动每次都是新决策）
- force-quit 才是真"不可调和的反模式"——iOS HIG 里所有 fatal alert 都给 user "OK 重启" 入口而非"无按钮静态页"
- transient 错误 + retry 入口 = user 自助恢复；transient 错误 + force-quit only = 误伤 transient 子集

### 修复（Fix）

把 mapper 从"按 APIError case 硬绑"改为"按 transient/terminal 二分":

```swift
case .unauthorized:
    return ErrorPresentation.retry(message: "登录失败，请重试")     // round 9 fix
case .decoding:
    return ErrorPresentation.retry(message: "数据异常，请重试")     // round 9 fix
case .missingCredentials:
    return ErrorPresentation.alert(...)                             // 真 terminal 保留
case .business(transient code 1005/1007/1008/1009):
    return ErrorPresentation.retry(...)                             // round 5 fix 已生效
case .business(其他 permanent code):
    return ErrorPresentation.alert(...)                             // 保留
case .network:
    return ErrorPresentation.retry(...)                             // 一直如此
```

修复后的 mapper 分类表：

| APIError case | round 8 | round 9 |
|---|---|---|
| `.unauthorized` | `.alert` (TerminalErrorView) | `.retry` (RetryView) |
| `.decoding` | `.alert` (TerminalErrorView) | `.retry` (RetryView) |
| `.missingCredentials` | `.alert` | `.alert`（**真** terminal —— keychain 损坏，retry 救不回） |
| `.business(1005/1007/1008/1009 transient)` | `.retry` | `.retry`（不变） |
| `.business(其他 permanent)` | `.alert` | `.alert`（不变） |
| `.network` | `.retry` | `.retry`（不变） |
| 非 APIError generic | `.alert` (fallback) | `.alert` (fallback) |

测试同步：
- `AppErrorMapperTests.testUnauthorizedMapsToAlertWithRestartHint` → 改名 `testUnauthorizedMapsToRetry`，断言 `.retry(message: "登录失败，请重试")`
- `AppErrorMapperTests.testDecodingErrorMapsToAlert` → 改名 `testDecodingErrorMapsToRetry`，断言 `.retry(message: "数据异常，请重试")`
- `testUserFacingMessageForDecodingErrorMatchesAlertCopy` → 改名 `...MatchesRetryCopy`
- 新增 `testUserFacingMessageForUnauthorizedMatchesRetryCopy`
- `AppLaunchStateMachineTests.testBootstrapWithUnauthorizedRoutesToAlertPresentation` → 改名 `...RoutesToRetryPresentation`，断言 `.retry`
- `AppLaunchStateMachineTests.testBootstrapWithDecodingErrorRoutesToAlertPresentation` → 改名 `...RoutesToRetryPresentation`
- `AppLaunchStateMachineTests.testStateMachinePresentsAlertForTerminalErrorAndRetryStaysIdempotent` → 驱动 case 从 `.unauthorized` 换成 `.missingCredentials`（round 9 后真 terminal 的代表）
- `ErrorComponentSnapshotTests.testTerminalErrorViewRendersVariousErrorCopy` → 删 `.unauthorized` / `.decoding` 文案 case，留 `.missingCredentials` + permanent business 1004 文案

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**为某个 APIError case 选 `.alert` vs `.retry` 时**，**必须**先问"这个 case 是否存在 transient 子集"，存在即归 `.retry`；只有"重启 App 也救不了"（本地配置永久损坏 / business permanent code）才归 `.alert`。
>
> **展开**：
> - **transient possible → `.retry`**：这是默认决策。理由：transient 子集存在时，把整个 case 归 `.alert` = 误伤 transient 流量 = user 被迫 force-quit。即便 retry 大概率仍失败（如 client 版本永远过旧），user 主动点重试再失败也只是多发一次请求，比"必须杀进程"温柔。`.unauthorized` 有 401 抽风子集 / `.decoding` 有 partial rollout 子集 / `.network` 有限流子集 — 全部归 `.retry`。
> - **terminal `.alert` 的判则窄**：必须满足"重启 App 也无法恢复"才归 `.alert`。当前只剩两类：(a) 本地态损坏（`.missingCredentials` —— keychain 真没 token / 写不进去，cold-start GuestLoginUseCase 仍走同一份 KeychainTokenStore）；(b) `.business(permanent code)`（如 1004 权限不足 / 4001 宝箱不存在 —— server 状态没变化时永远 terminal）。
> - **"AuthRetryingAPIClient exhausted 后必然 terminal"是错觉**：decorator 的"exhausted"指的是"我这层不会自动再试"，不等于"client 重启也救不了"。bootstrap closure 重跑 = 重新走 cold-start guest-login → 拿新 token → retry LoadHome，跟 decorator 内的静默重登是两条独立路径。
> - **"V1 schema 已冻结，出现 decoding 错必然 client 版本过旧"也是错觉**：partial rollout / canary 翻车 / CDN 缓存毒能让 fully-deployed schema 临时返出非法 payload。fail-fast 抛 `.decoding` 让 dev 看到 underlying 是好事（保留），但渲染必须给 user retry 入口。
> - **反例**：`case .unauthorized: return .alert(...)`、`case .decoding: return .alert(...)` —— "字面看 case 名觉得 terminal" 是危险简化。每个 case 都要展开看其 transient/terminal 子集，再决定。
> - **测试反例**：snapshot 测试里只测一两个 .alert 文案，不测"transient 子集应该归 .retry" → mapper 错误归类不会被 CI 抓到。规则：每个 APIError case 在 mapper test 必须有**正反例对**（transient instance → .retry / terminal instance → .alert，如 `.business` 1009 vs 1004）。

## Lesson 2 (meta): 9 轮 fix-review 累积出的 transient/terminal 通用二分判则

- **Severity**: meta
- **Category**: error-handling, process
- **分诊**: meta
- **位置**: 跨 review 流程，针对 Story 5.5 整段 fix history

### 症状（Symptom）

Story 5.5 经历 9 轮 codex review fix，单是"transient vs terminal 错误分类"这一议题就在以下轮次反复推翻：

- round 1：mapper 输出 String → "alert" 是默认形态
- round 2 [P1]：mapper 输出 ErrorPresentation 三态 → `.unauthorized` / `.decoding` 该 `.alert` 还是 `.retry` 第一次成议题
- round 3：retry-after-failure 死循环 → mapper 不是元凶，但暴露 `.alert` 的 dismiss 行为不可调和
- round 4：guest-login 失败也走 mapper → 引入 1009 卡 alert
- round 5 [P1]：1005/1007/1008/1009 transient business code 必须 `.retry` —— 第一条二分线
- round 7：alert OK 按钮调 retry 是反模式 —— 揭示"`.alert` 必须真 terminal"
- round 8 [P1]：bootstrap `.alert` 改用 TerminalErrorView 静态全屏页 + 文案回归简洁 —— 强化"`.alert` = force-quit"
- round 9 [P2]：`.decoding` / `.unauthorized` 改 `.retry` —— 收窄 `.alert` 适用范围到"重启都救不了"

每一轮都做出了"当时看似合理"的决策，但下一轮往往把判断推翻。9 轮过后才算稳定下来一条**通用二分判则**。

### 根因（Root cause）

每轮 fix-review 都是"按 review finding 的 framing 改"——review 抓到 1009 卡 alert，就改 1009；抓到 alert OK 按钮 no-op，就改 onDismiss 行为；抓到 alert 文案 promise 重试但 UI 没按钮，就改文案。每条 fix 单看都对，但**没人在某一轮跳出来写一份"transient/terminal 通用判则"**，导致每条 APIError case 的归类都靠"那一轮 review framing 给我看到的子集"决定，下一轮 review 看到另一个子集时归类又翻盘。

更深层根因：error 分类的"判则文档"本身缺位。mapper 注释里写了"`.alert` = terminal、`.retry` = transient"，但没回答"哪些 case 算 terminal"。每次 fix 都是 case-by-case 决策，缺 cross-case 一致性 review。

### 修复（Fix）

落锤 transient/terminal 二分通用判则（写进 lesson + mapper 文件头注释）：

> **transient possible → `.retry`**（默认决策）
> - 故障域里**存在** transient 子集（server 抽风 / 限流 / partial rollout / CDN 毒 / 乐观锁 race / 短暂 401）
> - retry 即便失败再次看到，user 主动点的成本远低于 force-quit
> - 例：`.network` / `.decoding` / `.unauthorized` / `.business(transient code)`
>
> **重启 App 也救不了 → `.alert`**（窄类）
> - 故障域**保证**永久（本地配置永久损坏 / business 永久 permanent code）
> - retry 数学上不可能恢复，给 retry 入口反而误导
> - 例：`.missingCredentials`（keychain 真没 token，cold-start 也读不到） / `.business(permanent code)`（如 1004 权限不足，server 状态没变就永远 401）
> - **fallback `.alert`**：非 APIError 的 generic Error，无法判定 transient/terminal，保守归 `.alert`

判则的关键是"默认偏 retry"而非"默认偏 alert"——因为 `.retry` 误判（其实是 terminal 但归了 .retry）的最坏后果是"user 多发一次请求"，`.alert` 误判（其实是 transient 但归了 .alert）的最坏后果是"user 被强制 force-quit"。前者代价远小于后者。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**碰到 review 关于"某个 error case 该 `.alert` 还是 `.retry`" 的 finding 时**，**必须**先把所有 APIError case 摊在桌上按二分判则全量审一遍（不只改 review 提到的那一条），再做改动；尤其**多轮 fix-review 同一议题**时，必须主动跳出"按 review framing 改"的 mental model 写出全量判则表。
>
> **展开**：
> - **9 轮反复迭代是危险信号**：第 3 轮起每轮都该输出全量分类表（mapper 分类表 / error path 行为表），对照前几轮的 fix 是否破坏了表里其他 case。本次 round 9 finding 之所以爆出来，就是 round 8 在改 `.decoding` 时只看了"V1 schema 冻结"这一面，没在 transient/terminal 全量表里把 `.decoding` 跟 `.unauthorized` 一起按二分判则审。
> - **二分判则的文档化是必需品**：mapper 注释 + lesson 文档同步落定"transient possible → .retry / 重启救不了 → .alert"判则。每次新增 APIError case，都按此判则定归类，并在 mapper 注释里说理由。
> - **`.alert` 是窄类**：默认归 `.retry`，只有举证"重启 App 也救不了"才归 `.alert`。举证负担在 caller 一侧 —— 不能默认归 `.alert`。
> - **review 找到的 finding 永远是"已暴露的冰山"**：round 9 抓到 `.decoding` 让我们顺手把 `.unauthorized` 一并按二分判则重审。如果只改 `.decoding` 不改 `.unauthorized`，下一轮（round 10）十有八九再被抓出来。
> - **判则迭代史是宝藏 context**：本 story 9 轮 mapper 改动叠在一起就是一份"transient/terminal 二分判则的演化史"——每条 mapping 行的 `// round N fix:` 注释 + lesson 文档对应章节，让未来 Claude 可以直接看到"哪轮为什么改、跟前轮的关系"。规则：mapper 文件头 + 每条 case 的注释必须保留迭代史，禁止"清理旧注释"式的注释重写。
> - **反例**：只 patch `.decoding` 改 `.retry`，不顺手审 `.unauthorized` → 下一轮 review 必抓。`.alert` 默认作为"安全选择"（"反正让 user 重启总没错"）→ 误伤 transient 流量，user 体验跌回石器时代。

---

## Meta: 本次 review 的宏观教训

5-5 这条 story 走到 round 9 已经把"error handling 是横切关注点 + 单点 patch 反模式"和"transient/terminal 二分判则不能 case-by-case 决策"两条 meta 教训都坐实了。lesson 1 是表层 fix，lesson 2 才是终结这条 story 的判则。未来 Claude 在 epic-loop / fix-review 跑同一 story ≥ 5 轮时，**必须**主动跳出"按 review framing 改"的 mental model，写出全量判则表 / 全量 error path 行为表，对照矩阵全量审计。否则下一轮 review 几乎必然挖出新 regression。
