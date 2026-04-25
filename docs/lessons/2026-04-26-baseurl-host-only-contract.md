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

- 无。本 fix 收口 round 5 唯一 [P2]。round 6 review 触发条件下应 PASS（除非 codex 又翻出
  新维度，但 round 5 与 round 4 的 finding 都属于 URL 七要素清单覆盖范围，本 lesson 的
  Top 1 规则即"逐字段过清单"已把后续同类风险打包了）。

## Meta: round 4 vs round 5 的连续盲区

round 4（`URL(string:)` 宽容） + round 5（path 维度漏校验）实际是**同一思维漏洞**的两次
表现：写 URL 配置 validator 时，没把 URL 当成"七字段结构体"逐一过清单，而是按"能想到的几个
维度"零散补。本 lesson 的预防规则把"列字段清单"显式化，让未来 Claude 写第一版 validator
时就能一次性 cover 所有维度，避免"修一个补一个"的循环。
