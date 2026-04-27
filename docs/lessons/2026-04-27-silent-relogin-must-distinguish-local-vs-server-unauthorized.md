---
date: 2026-04-27
source_review: codex review (epic-loop round 2) — /tmp/epic-loop-review-5-4-r2.md
story: 5-4-无效-token-静默重新登录
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-27 — 静默重登必须区分"本地无凭证"vs"server 拒绝 token"，前者**不**走 relogin

## 背景

Story 5.4 落地了 `AuthRetryingAPIClient`（APIClient decorator），catch `APIError.unauthorized` 触发 `SilentReloginCoordinator.relogin()` + 重试一次。round 1 实装时 dev-story 阶段在 `AuthRetryingAPIClient.swift` 注释 22-28 行刻意写明"`buildURLRequest` 阶段抛 `.unauthorized` 跟 server 返 401 都走同一恢复路径"——把本地 token 缺失 / keychain 配置错也并入静默重登流。

codex review round 2 [P2] 指出这违反 dev-story 5-4 文档**非范围 §3** 钦定的 scope —— 该 story 明确"只处理 server 401，**不**处理本地无 token / token 空 / keychain 配置错（那是 cold-start `GuestLoginUseCase` 的责任）"。round 1 的统一处理会：
1. 屏蔽真实的 keychain DI 配置错（开发者看不到 fail-fast）
2. 用户主动 reset / 卸载重装等 cold-start 场景被隐式 relogin 偷偷接管，违反"reset = 完全退出"的产品语义
3. 用户只丢本地 token 但 guestUid 还在的场景，会被隐式 relogin —— 而 dev-story 钦定这种情况应该走 cold-start 而非"复用既有身份重登"

这是**round 1 fix-review 跟 dev-story 设计冲突**的特殊情况：round 1 注释把"统一处理"当成 feature 写进了源码，但其实违反了上游 spec。本轮 round 2 codex 把它点出来，必须把 round 1 的注释和实装回退到符合 dev-story spec 的方向。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 本地态 unauthorized 不应触发静默重登 | P2 (medium) | architecture / error-handling | fix | `iphone/PetApp/Core/Networking/AuthRetryingAPIClient.swift:44-46` |

## Lesson 1: 静默重登必须区分"本地无凭证"vs"server 拒绝 token"，前者**不**走 relogin

- **Severity**: medium (P2)
- **Category**: architecture / error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/AuthRetryingAPIClient.swift:44-46`、`iphone/PetApp/Core/Networking/APIClient.swift:217-228`、`iphone/PetApp/Features/Auth/UseCases/SilentReloginUseCase.swift:57`

### 症状（Symptom）

`AuthRetryingAPIClient.request<T>(_:)` 用 `catch APIError.unauthorized where endpoint.requiresAuth` 拦截，触发 `coordinator.relogin()`。但 `APIClient.buildURLRequest` 在三种**本地态**下也会抛 `.unauthorized`：
1. `keychainStore == nil`（DI 配置错）
2. `keychainStore.get` 抛错（沙箱权限 / 平台异常）
3. `token == nil` 或空字符串（首次启动 / reset 后 / 卸载重装）

这些情况 server 还没看见过这次请求 —— 跟 server 真的返 401（"我看到你了，但你的 token 不行"）语义完全不同。round 1 把它们并入同一恢复路径，导致：

- DI 配置错被静默重登屏蔽 → 不会 fail-fast，开发者要等用户上报才发现
- cold-start 路径被静默接管 → 跟 `GuestLoginUseCase` 职责冲突
- reset 不彻底（只清了 token 但 guestUid 残留）的异常状态被偷偷"恢复"成"已登录"，违反 reset 语义
- 在"连本地 token 都没有"的场景下，relogin 内部 `SilentReloginUseCase` 也会读 guestUid，如果同样缺失也会抛错 → 把"本来 1 次报错"放大成"重登 + 再报错"的 N+1 调用浪费

### 根因（Root cause）

**思维漏洞 1**：把"看起来一样的 error case"当成"语义一样"。`.unauthorized` 这个 case name 不区分"server 拒绝"vs"本地缺失"，让"统一处理"看起来理所当然 —— 但这两种情况的恢复责任、产品语义、调用代价都不同。round 1 的注释甚至主动**为这种合并辩护**（"行为统一，但每个原请求最多重登 1 次（防无限循环）"），把"统一处理"当成正面 feature 写进源码，没意识到这跟 dev-story §3 非范围钦定**直接冲突**。

**思维漏洞 2**：dev-story 文档 § 非范围 写得很明确（"本 story **不**处理 buildURLRequest 阶段抛的 unauthorized，那归 cold-start GuestLoginUseCase"），但 round 1 在写 `AuthRetryingAPIClient` 注释时只看了"AC2 拦 .unauthorized"这一句话，没回去对照 § 非范围。**当 spec 和实装的便利性冲突时，spec 优先**。

**思维漏洞 3**：error type 设计粗粒度时，`catch ... where ...` 模式无法表达细粒度区分 —— `where endpoint.requiresAuth` 只能区分"哪个 endpoint"，不能区分"errror 来源是哪一层"。这种情况应当**修 error type 让它编码 source**，而不是在 catch 处加 ad-hoc 条件分支。

### 修复（Fix）

**单 commit 改动**（fix + 4 处生产代码 + 4 处测试）：

1. `iphone/PetApp/Core/Networking/APIError.swift`：新增 `case missingCredentials`（"本地端凭证缺失：请求**未发出**"），`.unauthorized` 注释收紧为"server 拒绝当前 token"。Equatable / LocalizedError 同步加分支
2. `iphone/PetApp/Core/Networking/APIClient.swift`：`buildURLRequest` 在三处本地态分支全部从 `throw .unauthorized` 改 `throw .missingCredentials`；HTTP 401 / envelope 1001 路径**不变**仍抛 `.unauthorized`
3. `iphone/PetApp/Core/Networking/AuthRetryingAPIClient.swift`：catch pattern 不变（仍 `catch APIError.unauthorized where ...`），靠新 case 自动让本地态走默认 propagate 路径；注释 22-28 行旧"统一处理"段**改成反过来的说明**："为何**不**拦 .missingCredentials"，并写明 round 1 注释已废弃 + 引用 dev-story 非范围 §3
4. `iphone/PetApp/Features/Auth/UseCases/SilentReloginUseCase.swift`：内部"无 guestUid → throw .unauthorized"也改 `.missingCredentials`（同样属于"本地无身份"语义；让 AuthRetryingAPIClient 不会把"relogin 自己缺凭证"误当 server 401 触发"用 relogin 救 relogin 自己"的悖论）
5. `iphone/PetApp/Shared/ErrorHandling/AppErrorMapper.swift`：新增 `.missingCredentials → ErrorPresentation.alert("登录信息丢失，请重启应用")`（不能用 `.unauthorized` 的 toast "正在重登"，会误导用户以为后台在自动恢复 —— 实际上 decorator 不接管，需要 cold-start）
6. 测试：
   - `APIClientAuthInjectionTests.swift` case#3-#6 断言改 `.missingCredentials`
   - `SilentReloginUseCaseTests.swift` case#2-#3 断言改 `.missingCredentials`
   - `APIClientTests.swift` case#3 / case#4（HTTP 401 / envelope 1001）endpoint 改 `requiresAuth: false` —— 它们测的是 server 路径决策，不该被 keychain 注入步骤拦截
   - `AppErrorMapperTests.swift` 新增 case#4b `.missingCredentials → alert`
   - `AuthRetryingAPIClientTests.swift` 新增 case#7 / case#8 锁死 `.missingCredentials` **绝不**触发 relogin（关键回归保护）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 设计 / review **error 类型 + 拦截器**时，**必须**先问"这个 error case 的恢复语义是否唯一？"——如果同一个 case 名能从"本地未发请求"和"server 已返失败"两种来源抛出，**必须**把它拆成两个 case（或加 source metadata），让拦截器的 catch pattern 能机械区分 —— 否则任何 `catch X where ...` 的拦截策略都会无意中把不该接管的路径接管掉。
>
> **展开**：
> - error type 是 API 契约的一部分，不仅是 dev-only 的 type tag。每个 case name 应该承载"语义来源" + "恢复责任"两层信息
> - 写 decorator / interceptor 时，**先列举上游所有可能的 throw site**（不只是看 catch 的那个名字，要看代码里所有 throw 这个 case 的地方）；任何一个 throw site 的语义不在 catch 的责任范围内，就要么修拆 case、要么 catch 时加额外 guard
> - 当 dev-story 文档有 § 非范围 时，写实装代码前**对照 § 非范围 章节再读一遍**——它列举的"不该做的事"通常不是冗余说明，而是 reviewer / PM 故意写下来防止实装跑偏的 trap
> - **fix-review round N+1 改 round N 自己写的注释 / 设计**是合法且必要的 —— 当 dev-story spec 跟 round N 实装冲突时，spec 优先；不要为了"保持上轮决策稳定"而保留违反 spec 的实装。round N 注释只是当时的理解快照，不是不可挑战的约定
> - **反例 1**：catch `APIError.unauthorized` 不区分来源，把 `buildURLRequest` 抛的本地态也送进 server-side 恢复流程 —— 屏蔽 cold-start / 配置错信号
> - **反例 2**：发现 dev-story 跟自己写的实装冲突，反而修改实装注释为冲突辩护（round 1 那段 22-28 行注释），而不是回头读 spec
> - **反例 3**：用 `where` clause 在 catch 阶段做 ad-hoc 区分（如 `where endpoint.requiresAuth`），而不是修 error type —— ad-hoc 区分无法 enumerate（编译器不会帮你 exhaustive 检查"是否所有 source 都被妥善分流"），以后加新 throw site 时容易漏

## Meta: 本次 review 的宏观教训

round 1 的 fix-review 刚刚才修过本 story 的另一个 P2 finding（actor coalesce 清理），fix-review 当时**新增**了 22-28 行注释解释"buildURLRequest 抛 unauthorized 跟 server 返 401 都走同一路径"。round 2 codex 直接把这段注释作为 finding 的核心论据反对。

**经验**：fix-review 阶段**新写**的注释 / 文档段落，本身可能引入新的设计错误。注释不是 fix 的免责声明 —— 它跟 source code 一样要接受下一轮 review 的检验。当注释跟上游 spec 冲突时，注释 = 错的，spec = 对的，回去改实装 + 改注释，不要试图用注释为冲突辩护。
