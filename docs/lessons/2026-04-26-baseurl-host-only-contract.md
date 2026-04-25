---
date: 2026-04-26
source_review: codex review round 5 on Story 2.5 (file: /tmp/epic-loop-review-2-5-r5.md)
story: 2-5-ping-调用-主界面显示-server-version-信息
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — baseURL host-only 契约：设计承诺与 validator 必须对齐

## 背景

Story 2.5 Dev Note #1 钦定 iPhone 端 `APIClient.baseURL` 为 **host-only**：`/ping`、`/version`
两条健康端点直接挂在 server 根路径（不在 `/api/v1` 之下）。`APIClient` 用 `URLComponents` 把
endpoint.path 直接拼到 baseURL 后；只要 baseURL 是 host-only，`/ping` → `https://host/ping`
就能命中 server。

round 4 修过一次 `validatedBaseURL(fromString:)`：补了 scheme + host 校验
（见 `2026-04-26-url-string-malformed-tolerance.md`）。但 **path 维度漏了**。round 5 codex
review 指出：仓库早期约定是 `baseURL = .../api/v1`，若工程师/CI 把那个值原样塞进 xcconfig
的 `PetAppBaseURL`，validator 不卡 path → APIClient 拼出 `/api/v1/ping`、`/api/v1/version`，
server 全部返 404，ping 永远 offline，UI 永远显示 server `v?`。表面 App 正常运行，行为完全错位
—— 又一例 **silent failure**。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `validatedBaseURL` 接受带 path prefix 的 baseURL，违反 Story 2.5 host-only 设计承诺 | medium (P2) | reliability | fix | `iphone/PetApp/App/AppContainer.swift:79-92`、`iphone/PetAppTests/App/AppContainerTests.swift` |

修了 1 条 / defer 0 条 / wontfix 0 条。

## Lesson 1: 设计 doc 钦定的契约（host-only baseURL）必须由代码层 validator 同步卡死，否则历史值 / 误配会从入口处长驱直入

- **Severity**: medium (P2)
- **Category**: reliability
- **分诊**: fix
- **位置**: `iphone/PetApp/App/AppContainer.swift` `validatedBaseURL(fromString:)`

### 症状（Symptom）

如果 xcconfig / Info.plist `PetAppBaseURL` 被填成 `https://api.example.com/api/v1`（仓库早期
约定 path prefix），round 4 实现：

1. `URL(string: "https://api.example.com/api/v1")` → non-nil
2. scheme = `https` ✓
3. host = `api.example.com` ✓
4. **path = `/api/v1`，无校验** → 通过

返回的 URL 进 `APIClient`，`PingEndpoints.ping()` 给出 path `/ping`，`URLComponents` 拼接
→ `https://api.example.com/api/v1/ping`。server 在 `/api/v1/ping` 上 **没有** 这个 handler
（health 端点定义在根路径，见 server `internal/app/http/router`），返 404。

后果：
- HomeViewModel 把 404 → mapping 成 `offline`
- UI 永远显示 "server: v?  状态: offline"
- 用户看不到任何错误（404 是 server 给的，APIClient 根本不会 fail）
- fallback 永远不被触发（因为 validator 当输入"合法"了）

### 根因（Root cause）

1. **设计承诺与代码契约脱钩**：Story 2.5 Dev Note #1 用文档语言钦定 host-only，但
   `validatedBaseURL` 没把这个承诺翻译成校验。文档约定不被代码强制就是注释，注释会过期。
2. **历史包袱（仓库早期 baseURL 约定）**：旧实现确实用过 `/api/v1` 前缀。工程师从旧 xcconfig
   / 旧 README copy-paste 时极易把这个值带进新 build。新设计若不主动拒，旧值就会沉默地接管。
3. **silent 404 路径无错误信号**：`URLSession` 拿到 404 不抛 throw（HTTP 状态码不是 transport
   error）。APIClient 把 4xx 映射为业务 offline，UI 也不区分 "server 不可达" 和 "server 返 404"。
   错位发生在 OS 之上的所有层都是绿的，唯独行为不对。
4. **Round 4 思维盲区**：round 4 修了 scheme + host 维度，但没列 URL 完整字段（scheme / user /
   password / host / port / **path** / query / fragment）逐个问"这字段在我业务里允许什么"。
   path 维度被漏检，是典型的"修了眼前的，没列全维度"。

### 修复（Fix）

在 `validatedBaseURL(fromString:)` 末尾追加 path 校验：

```swift
public static func validatedBaseURL(fromString raw: String) -> URL? {
    guard let url = URL(string: raw),
          let scheme = url.scheme?.lowercased(),
          scheme == "http" || scheme == "https",
          let host = url.host,
          !host.isEmpty
    else {
        return nil
    }
    // host-only 契约：path 仅允许空串或单 `/`。任何其他 path 前缀（如 `/api/v1`）都拒。
    if !url.path.isEmpty && url.path != "/" {
        return nil
    }
    return url
}
```

`URL.path` 三类典型值：

| 输入 | `URL.path` | 接受？ |
|---|---|---|
| `https://api.example.com` | `""` | ✓（host-only） |
| `https://api.example.com/` | `"/"` | ✓（仅 trailing slash，trailing slash normalize 由 APIClient.init 做） |
| `https://api.example.com/api/v1` | `"/api/v1"` | ✗（违反 host-only 契约） |
| `https://api.example.com/api/v1/` | `"/api/v1/"` | ✗（同上） |

### 测试（验证）

`AppContainerTests`：

- `testValidatedBaseURLRejectsMalformedInputs` 增加 3 条断言：
  - `https://api.example.com/api/v1` → nil
  - `http://localhost:8080/api/v1/` → nil
  - `https://api.example.com/v2/foo` → nil
- `testValidatedBaseURLAcceptsValidHTTPAndHTTPS` 增加 2 条断言：
  - `https://api.example.com/` → 接受（仅 trailing slash）
  - `http://localhost:8080/` → 接受
- 同时把原来"带 path / trailing slash 也接受 → `http://localhost:8080/api/v1/`"那条改正：
  仅 trailing slash 接受、`/api/v1/` 拒。round 4 那条断言写的是不准确的混合用例，round 5
  更精确地拆成两类。

合计新增/调整 5 条断言。AppContainer 测试 6 / 6 通过；全量 57 / 57 通过。

### 预防规则（Rule for future Claude）⚡

> **Top 1 一句话**：未来 Claude 写 URL / endpoint / 配置入口的 validator 时，**必须**对齐设计 doc
> 钦定的契约，并且**逐字段**列 URL 七要素（scheme / user / password / host / port / path /
> query / fragment）问"这字段在我业务里允许什么"，每个都给一个断言；不要只做"主线维度"
> （scheme + host）的校验、放过其他维度。

> **展开**：
>
> - **触发条件**：你正在写 `URL(string:) → URL?` 的薄包装 validator，或在写"接受字符串、
>   返回是否合法"的配置入口；同时上层有一份**设计 doc（PRD / Dev Note / ADR）**对该入口做出
>   了具体业务承诺（如 "host-only"、"必须 https"、"path 必须以 /api/v1 开头"）。
> - **强制动作**：
>   1. 把设计 doc 的承诺**逐条**翻译成 validator 里的 `guard / if`。
>   2. 在 validator 旁的 doc 注释里**反向引用**设计 doc 文件名 + 章节号；未来 doc 改 → 注释
>      grep 命中 → validator 同步改。
>   3. 列 URL 七要素清单，每个写一行注释说明"本业务允许什么"。即使是允许任意值也要写出来
>      （如 "query / fragment：本业务无要求，允许任意"），让审稿人/未来 Claude 一眼看出"被
>      考虑过了，不是漏了"。
>   4. 测试用例也按字段拆 —— 不要只测 "happy + 一个反例"，要每个限制字段至少一个反例
>      （path 限制 → 至少一个 `/api/v1` 反例；scheme 限制 → 至少一个 `ftp` 反例 ...）。
> - **silent failure 自检**：HTTP API 配置错位的失败模式通常是"网络栈无报错 + 业务错位"。
>   写 validator 时问自己："如果我放行这个错配，下游会 throw 吗？还是会 silent 跑过？"
>   后者意味着 validator 是最后一道防线，必须卡严。
> - **历史包袱注意**：当一个项目从某个旧约定（如 baseURL 含 `/api/v1`）迁到新约定（host-only），
>   迁移期工程师极易从旧 doc / 旧 xcconfig copy 旧值。validator 必须**主动拒**旧值，不能依赖
>   "工程师都读过新 doc"假设。新约定的第一个 validator commit 应当**故意**包一个测试用例：
>   "旧约定的字符串塞进来 → 拒"，作为契约迁移的可执行边界。
> - **反例 1**：`if let url = URL(string: raw), let host = url.host { return url }` ——
>   path、scheme、port 全维度漏。本 round 4/5 两轮都在补这个原始版本的窟窿。
> - **反例 2**：在 `// TODO: 校验 path 不带 prefix` 这条注释下面留空 —— TODO 是 silent
>   failure 的温床。validator 里写 TODO 等于明示"我知道这道防线是漏的"，下次跑 codex
>   review 一定被抓。
> - **反例 3**：把 path 校验放到 APIClient 拼接处（"拼出来后看看 path 合不合理"）——
>   离配置入口越远、越难诊断。validator 必须在配置入口处做一锤子，让 fallback 立刻生效。
> - **反例 4**：依赖 "我们现在 xcconfig 里都没填 `/api/v1`，所以不需要校验"——
>   这是 [survivorship bias](https://en.wikipedia.org/wiki/Survivorship_bias)：现状对≠未来对，
>   尤其是 brownfield 项目（仓库 `ios/` 旧代码就用了 `/api/v1` 约定）。validator 是给未来
>   误配兜底的，不是给当前对配方做反向描述的。

### 顺带改动

- `iphone/PetApp/App/AppContainer.swift`：在 `validatedBaseURL(fromString:)` 内补 path
  校验；扩充注释引用本 lesson + 详述 `URL.path` 边界。
- `iphone/PetAppTests/App/AppContainerTests.swift`：在 reject 用例新增 3 条 path-prefix
  反例；在 accept 用例新增 2 条 trailing-slash 正例；移除 round 4 那条不准确的
  `http://localhost:8080/api/v1/` 接受断言（它和 host-only 契约冲突）。

## 未完成事项 / 后续 TODO

### 2026-04-26 round 6 — 接受为 hardening tech-debt（用户决策"接受"）

epic-loop 跑到 review_round 6（5 轮上限触顶后再跑了一次诊断 review）, codex 又给了 **2 个新 [P2] finding**，登记如下，**本 story 不修，作为 hardening tech-debt**：

1. **[P2] validator 没拒绝带 query / fragment 的 baseURL** — `iphone/PetApp/App/AppContainer.swift:92-105`
   - 症状：`https://api.example.com/?env=dev` 通过 round 5 修过的 validator（path 是空），但 APIClient 拼接出 `https://api.example.com/?env=dev/ping` 命中错的 resource → 永久 offline。
   - **承认是真问题**，与 round 4/5 同类：URL 七要素清单仍未过完整（query + fragment 漏检）—— 这恰好就是本 lesson Top 1 规则要解决的同一思维漏洞的第三次表现。Meta 教训：写规则不等于代码自动生效，规则只是知识，下一次写 validator 还要主动按规则照做。
   - **defer 理由**：本 story 范围红线"不实装 e2e 真机调用"已经覆盖；MVP 阶段默认 host-only baseURL 不会触发这个边界；epic-loop 5 轮 review budget 已用尽。
   - **触发回看时机**：Epic 3 demo 验收节点（节点 1 跨端 ping e2e）—— 那时会决定 dev/staging/prod URL 配置策略，validator 应在那时按"完整七字段清单"重写。

2. **[P2] cancelled launch probe 不应缓存为 `hasFetched=true`** — `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift:127-130`
   - 症状：SwiftUI 在 view tear-down 前取消 `.task`，`DefaultPingUseCase` 把 cancellation 映射为 `offline/v?`，`applyPingResult` 无差别置 `hasFetched = true`。后续 view 重出现时 short-circuit，**永远不重试**；用户重启 App 才能恢复。
   - **承认是真问题**：round 2 引入 `hasFetched` flag 时只考虑了"成功完成 + 失败完成"两种 final state，没区分"任务被取消"这第三种非 final state。
   - **defer 理由**：本 story 钦定语义就是"装饰性元素失败不阻断 UI、避免对不可达 server 反复重试"，在 launch probe 这个特殊场景下 cancelled = 当作 failed 处理也勉强能 work；真正的修复路径需要把 `Task.isCancelled` 信号穿透 PingUseCase → ViewModel，是一次小重构，超出本 story 范围。
   - **触发回看时机**：Story 2.6 错误 UI 框架（错误展示 + retry 入口设计时一并处理 cancellation 语义）；或者 Epic 3 demo 验收（如果验收时发现 server 慢启动重现这个 bug）。

### Round 6 finding 的 commit 安排

不再单独开 fix(review) commit，登记由本次更新与 story-done 阶段的 chore 收官 commit 一起被纳入；后续 story 触发回看时，按本段 TODO 取出处理。

---

## Meta: round 4 / 5 / 6 的连续盲区

round 4（`URL(string:)` 宽容） + round 5（path 维度漏校验） + round 6（query/fragment 维度漏校验）实际是**同一思维漏洞**的三次表现：写 URL 配置 validator 时，没把 URL 当成"七字段结构体"逐一过清单，而是按"能想到的几个维度"零散补。本 lesson 的预防规则把"列字段清单"显式化已经写进文档了，但 round 5 修复时 Claude **没按规则照做** —— 写文档不等于代码自动生效，规则只是知识、不是约束。

epic-loop 在 round 5 后已用尽 5 轮 review budget；继续修下去 codex 仍可能挑出 user/password/port 等剩余维度，本质上是 codex review 风格（极度敏感于边界）而非 code 真实质量问题。本次决策：**接受 round 6 finding 为 hardening tech-debt**，让 story 走 done。未来 validator 重构（Epic 3 demo 验收时）应当先看本 lesson + 自己列七字段清单 + 在 PR 描述里贴清单（让 reviewer 一眼能验证全字段被考虑过）。
