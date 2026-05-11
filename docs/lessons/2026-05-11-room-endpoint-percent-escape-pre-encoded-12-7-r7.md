---
date: 2026-05-11
source_review: codex review (epic-loop r7) file:/tmp/epic-loop-review-12-7-r7.md
story: 12-7-创建-加入-退出-use-case-主界面入口完善
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-11 — RoomEndpoints percent-encode pre-escaped 输入也要 escape `%`

## 背景

Story 12.7 round 7 codex review。前 6 轮 review 找到的 finding 都集中在 stale-response guard 与 UITEST
启动路径上；本轮 codex 把目光转到了同 story 早先轮已加固的 `RoomEndpoints.escapePathSegment` ——
该 helper 在 round 1 P2 修复时只从 `CharacterSet.urlPathAllowed` 移除了 `/`、`?`、`#`，但**没有移除 `%`**。
对 pre-escaped 输入（如 `AA%2FBB` 或 `1234%3Fevil=1`）来说，`%` 原样透传到 server →
server URL decode 后看到 `AA/BB` / `1234?evil=1` → 请求路由到错误 endpoint，绕过 server-side 1002
"房间号格式不合法" 校验路径。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | escapePathSegment 必须移除 `%` 防 pre-escaped 输入绕过 1002 校验 | P2 (medium) | security | fix | `iphone/PetApp/Features/Room/UseCases/RoomEndpoints.swift:64-74` |

## Lesson 1: percent-encoding helper 必须把 `%` 自己也加进 escape set，否则 pre-escaped 输入会绕过校验

- **Severity**: P2 (medium)
- **Category**: security
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/UseCases/RoomEndpoints.swift:64-74`

### 症状（Symptom）

`escapePathSegment("AA%2FBB")` 返回 `"AA%2FBB"` 原样（`%` 不被认为是需要 escape 的字符），
拼接成 URL path `/api/v1/rooms/AA%2FBB/join`。当 server-side URL parser / 中间层 proxy decode 这段
percent sequence 时会得到 `AA/BB`，于是请求实际路由到 `/api/v1/rooms/AA/BB/join` ——
完全不命中 `POST /api/v1/rooms/{roomId}/join` 路由表，server 端的 1002 "房间号格式不合法" 业务
错误处理路径永远走不到，client 看到的是 404 / 405 等 transport 错误，与"raw `/` 直传"完全相同的
hijack 后果，**round 1 P2 修复其实没盖住这一类输入**。

### 根因（Root cause）

`CharacterSet.urlPathAllowed` 是 Foundation 定义的"在 path 内合法的字符集"，**包含 `%`**——
因为 `%` 是 percent-encoding 的引导符，已 escape 的 path（`/foo/%2Fbar`）本身就是合法 URL。
`addingPercentEncoding(withAllowedCharacters:)` 把 input 中**不在** allowed set 的字符做 escape，
所以 `%` 默认不会被 escape。Round 1 修复时只想着"path 分隔语义字符"（`/?#`），漏掉了 `%`
自己：**当目标是"把 input 当作单一 opaque path segment"时，pre-escaped 序列也必须被 escape
一次**，否则 already-escaped 字符在 server 端 decode 之后会变成"真正的 reserved 字符"，
重新触发原始的 hijack 攻击面。

更深一层：percent-encoding 是**幂等可逆**的设计 —— escape 一次 / decode 一次 / 净增/净减 为零。
如果你把 user input 当作 opaque 数据塞进 URL，必须**把 `%` 当成 user 数据的一部分**一起 escape，
让 server decode 一次后拿回原 user input；如果不 escape `%`，相当于让 user 的 input 经过一次
"被动 decode"，user 可以注入任意 reserved 字符。

### 修复（Fix）

`RoomEndpoints.roomIdPathAllowed` 的 allowed set 移除字符串改为 `"/%"`（原为 `"/"`）：

```swift
private static let roomIdPathAllowed: CharacterSet = {
    var allowed = CharacterSet.urlPathAllowed
    allowed.remove(charactersIn: "/%")   // ← 加 `%`
    return allowed
}()
```

效果：
- `escapePathSegment("AA%2FBB")` 现在返回 `"AA%252FBB"`（每个 `%` 自身被 escape 为 `%25`）.
- Server 端 URL parser decode 一次后看到字面 `AA%2FBB` 字符串作为 path segment, 命中
  `POST /api/v1/rooms/{roomId}/join` 路由 → 触发 server-side 1002 "房间号格式不合法" 校验 ——
  恢复 round 1 P2 想要保留的业务错误处理路径.
- `1234%3Fevil=1` → `1234%253Fevil=1`，server decode 后是字面 `1234%3Fevil=1`，不触发 query 解析.
- 多 `%` 输入（`AA%2FBB%23CC`）→ `AA%252FBB%2523CC`，所有 `%` 都被 escape.

测试补强（`RoomEndpointsTests.swift`）：
- `testEscapePathSegmentPercentEscapedSlash` — 锁住 `AA%2FBB` → `AA%252FBB`.
- `testEscapePathSegmentPercentEscapedQuestionMark` — 锁住 `1234%3Fevil=1` → `1234%253Fevil=1`.
- `testEscapePathSegmentMultiplePercentsAllEscaped` — 锁住多 `%` 全部 escape.
- `testEscapePathSegmentNormalRoomIdStillPassesThrough` — 既有合法纯数字 input 仍直通不变.
- `testJoinRoomPercentEscapedInputDoubleEscapesPath` — 锁住完整 join endpoint path.

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **用 `addingPercentEncoding(withAllowedCharacters:)` 把 user input 塞进 URL path/query** 时，**必须**把 **`%` 一起从 allowed set 移除**，否则 pre-escaped 输入会绕过你想保护的所有 reserved-字符规则.
>
> **展开**：
> - `CharacterSet.urlPathAllowed` / `.urlQueryAllowed` 都**含** `%` —— 因为 percent-encoded sequence 本身在 URL 内合法.
> - 当目标是"把 user input 当 opaque payload 塞进 URL"时，**必须**让 user 的每个 `%` 都被 escape 成 `%25`，否则 server decode 一次后 user 的输入会"被动 decode"一层，user 可以注入原本你 escape 掉的 reserved 字符.
> - 模式化写法（适合 path segment）：
>   ```swift
>   var allowed = CharacterSet.urlPathAllowed
>   allowed.remove(charactersIn: "/?#%")   // 4 个字符一起 subtract
>   return input.addingPercentEncoding(withAllowedCharacters: allowed) ?? input
>   ```
>   `/` `?` `#` 是 URL 内的语义分隔符；`%` 是 percent-encoding 的引导符 —— 4 个一起 subtract 才完整.
> - 同精神适用于 query value segment（用 `urlQueryAllowed` 减去 `&=+#%`）/ host segment 等场景.
> - 测试模式：除了基本的 `/`/`?`/`#` escape 用例，**必须**加一条 pre-escaped 输入（`AA%2FBB`）用例 ——
>   断言 `%` 被 escape 成 `%25` —— 这条用例**专门防回归到"只 escape 语义字符不 escape `%`"的旧形态**.
> - **反例 1**（本次踩坑）：`allowed.remove(charactersIn: "/")` 单独移除 `/`，没动 `%` —— pre-escaped `AA%2FBB`
>   会原样透传，server decode 后变成 `AA/BB` → 路由 hijack.
> - **反例 2**（同类思维误区）：`URLComponents.percentEncodedPath = userInput` 直接赋值 —— Foundation 不会
>   做二次校验，user input 含 `%2F` 会被原样写入；正确做法是先 escape 再赋值.
> - **反例 3**：依赖 server-side 校验"就行" —— 是的 server 必然要校验，但 client 端 percent-encoding 的**前提
>   是"server 拿到的是 user 的原始 input"**；如果 client 让 user 注入了语义字符，server 根本不知道 user 输入
>   的是 `AA%2FBB` 还是 `AA/BB`，校验路径完全错位（404 而非 1002）.

### 关联 lesson

- `2026-05-11-uitest-fallback-and-leave-room-stale-response-guard.md` —— round 1 同一文件 P2 修复的初始版本（只 escape `/?#`），本 lesson 是其补充.
- 同 story 早期 stale-response guard 系列 lesson —— 与本 finding 独立但属同一 review session.
