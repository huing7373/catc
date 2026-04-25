---
date: 2026-04-26
source_review: codex review round 4 on Story 2.5 (file: /tmp/epic-loop-review-2-5-r4.md)
story: 2-5-ping-调用-主界面显示-server-version-信息
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — `URL(string:)` 对 malformed 输入过于宽容；用作配置入口必须显式校验 scheme + host

## 背景

Story 2.5 round 1 把默认 baseURL 从硬编码改为从 Info.plist `PetAppBaseURL` 读取（见
`docs/lessons/2026-04-26-baseurl-from-info-plist.md`）。round 1 的 `resolveDefaultBaseURL(from:)`
长这样：

```swift
public static func resolveDefaultBaseURL(from bundle: Bundle) -> URL {
    if let raw = bundle.object(forInfoDictionaryKey: baseURLInfoKey) as? String,
       let url = URL(string: raw) {
        return url
    }
    return URL(string: fallbackBaseURLString)!
}
```

注释承诺"key 不存在 / 类型错 / URL 格式错一律静默回退到 fallback"。codex round 4 review 指出：
**这个承诺未兑现**。`URL(string:)` 对许多明显 malformed 的字符串仍返回 non-nil，导致校验形同虚设。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `URL(string:)` 接受 malformed 输入，违背 resolve 层 fallback 承诺 | medium (P2) | reliability | fix | `iphone/PetApp/App/AppContainer.swift:58-65`、`iphone/PetAppTests/App/AppContainerTests.swift` |

修了 1 条 / defer 0 条 / wontfix 0 条。

## Lesson 1: `URL(string:)` 不能当作"URL 是否合法"的充分判据；用作配置入口必须显式校验 scheme + host

- **Severity**: medium (P2)
- **Category**: reliability
- **分诊**: fix
- **位置**: `iphone/PetApp/App/AppContainer.swift`（resolve 函数）、新增 `validatedBaseURL(fromString:)` helper

### 症状（Symptom）

如果工程师通过 xcconfig 把 `PetAppBaseURL` 误填成 `localhost:8080`（漏写 `http://`）或 `http://`（漏写 host），
round 1 实现都会"接受"该值并返回构造的 URL。然后：

- `APIClient` 用这个 URL 构 `URLRequest` 时，`URLSession` 实际发请求会失败（不支持的 scheme / 缺 host）
- ping/version 永远走 offline 路径
- 表面 App 跑得动、UI 没崩，行为却完全错位 —— **silent failure**

且 fallback 永远不被触发，整个"安全网"形同虚设。

### 根因（Root cause）

`URL(string:)` 是 RFC 3986 通用 URL 解析器，**不**对 scheme / host 做语义校验：

| 输入 | `URL(string:)` 返回 | 语义 |
|---|---|---|
| `"localhost:8080"` | non-nil | scheme=`localhost`, path=`8080`（!） |
| `"http://"` | non-nil | scheme=`http`, host=nil |
| `"ftp://example.com"` | non-nil | scheme=`ftp`，HTTP API client 不支持 |
| `"file:///tmp/x"` | non-nil | scheme=`file`，本地文件 URL，错位 |
| `"http://example .com"` | nil | 含非法字符，URL parser 也会拒 |

`URL(string:)` 仅在严格语法错时返回 nil（含空格 / 非法字符）；**只要语法过得去，scheme/host 是什么都接受**。
这是 Apple URL parser 的设计——通用、宽容；但用作"业务上合法的 baseURL"判据就不够了。

### 修复（Fix）

提一个独立 static helper `validatedBaseURL(fromString:)`，集中卡 scheme + host：

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
    return url
}

public static func resolveDefaultBaseURL(from bundle: Bundle) -> URL {
    if let raw = bundle.object(forInfoDictionaryKey: baseURLInfoKey) as? String,
       let url = validatedBaseURL(fromString: raw) {
        return url
    }
    return URL(string: fallbackBaseURLString)!
}
```

设计要点：

- **拆 helper 而非 inline 进 resolve**：测试可直接传字符串覆盖 happy / malformed 矩阵，不需 mock Bundle。
- **`scheme?.lowercased()`**：scheme 大小写不敏感（RFC 3986 §3.1），`HTTPS://` 应被接受。
- **白名单 scheme（http / https）**：HTTP API client 只支持这两个；ws/wss 走 WebSocket 通道，file/ftp 直接错位。
  显式白名单优于黑名单（未来新 scheme 自动落到 fallback，安全默认）。
- **`host.isEmpty` 防御**：理论上 URL parser host 非空，但保留一道防线无成本。
- **trailing slash 不做 normalize**：那是 `APIClient.init` 的职责（见 `2026-04-26-url-trailing-slash-concat.md`），
  baseURL 校验只管"能不能用"。

### 测试（验证）

`AppContainerTests`：

- `testValidatedBaseURLRejectsMalformedInputs`：覆盖 5 类 malformed 输入（无 scheme / 仅 scheme / 错 scheme /
  空串 / 含空格）。
- `testValidatedBaseURLAcceptsValidHTTPAndHTTPS`：覆盖 http、https、大写 scheme、带 path/trailing slash 的合法值。

合计 6 个新断言，外加 3 个原有 case 不变。

### 预防规则（Rule for future Claude）⚡

> **Top 1 一句话**：未来 Claude 把 `URL(string:)` 用在"判断字符串是否是合法 URL"的场景时，**必须**额外校验
> `url.scheme` 在业务白名单内（HTTP API 客户端配置就是 `http`/`https`）且 `url.host` 非空；
> `URL(string:)` 仅是语法解析器，不是语义校验器，对 `localhost:8080`、`http://`、`ftp://x` 等
> malformed-but-parseable 输入一律返回 non-nil。

> **展开**：
>
> - **触发条件**：你正在写 `if let url = URL(string: someConfigString) { ... }` 这类代码，且
>   `someConfigString` 来自外部输入（Info.plist / UserDefaults / 远程配置 / 命令行参数 / 用户输入）。
>   立刻问自己："`URL(string:)` 通过 ≠ 业务上合法。我需要 URL 满足什么前置条件？" 列出来逐一卡。
> - **常见业务前置条件清单**：
>   - HTTP API baseURL：scheme ∈ {http, https}，host 非空。
>   - WebSocket URL：scheme ∈ {ws, wss}，host 非空。
>   - 图片下载 URL：scheme ∈ {http, https}，host 非空，可选要求 path 后缀在白名单。
>   - 本地文件 URL：scheme = file，path 非空。
> - **scheme 校验用 `?.lowercased()` + 白名单**：scheme 大小写不敏感（RFC 3986 §3.1）；
>   白名单优于黑名单，未来新 scheme 自动落到拒绝路径，安全默认。
> - **host 校验用 `host?.isEmpty == false`**：`URL(string: "http://")` 的 host 是 nil；某些边界
>   `URL(string: "http://:8080")` host 也是空串（部分 iOS 版本）。两都防。
> - **把校验提为独立 static func，不要 inline**：`validatedXxx(fromString:) -> URL?`。
>   - 单元测试可直接传字符串矩阵，无需构造 mock Bundle / mock UserDefaults。
>   - 校验规则集中一处，未来收紧（如加 host 不能是 IP / 端口必须在范围）只改一处。
> - **fallback 必须真正可达**：本 fix 后 fallback 才能真正生效。如果 fallback 本身可疑（比如硬编码
>   localhost 在真机上不可达），那是另一个问题，不要让 fallback 校验缺失把它掩盖了。
> - **反例 1**：`if let url = URL(string: raw) { return url }` —— 本 review 就是这个问题。
> - **反例 2**：依赖正则 `^https?://...$` 校验 —— 容易漏边界（如 `https://` 后空 host），
>   `URL(string:) + 字段校验` 比手写正则可靠。
> - **反例 3**：把校验留给下游 `URLSession.dataTask`，依赖运行时报错 —— silent failure 来源，
>   表现为"feature 永远 offline 但代码无错误"，最难诊断的一类 bug。校验就该在配置入口处做。
> - **反例 4**：用 `URLComponents(string:)` 替代 `URL(string:)` 也不解决问题 —— `URLComponents`
>   同样宽容，`URLComponents(string: "localhost:8080")?.scheme` 也是 `"localhost"`。
>   关键是**你**要主动卡白名单，工具不会替你卡。

### 顺带改动

- `iphone/PetApp/App/AppContainer.swift`：新增 `validatedBaseURL(fromString:)` static helper；
  `resolveDefaultBaseURL(from:)` 改用 helper；补充注释说明 `URL(string:)` 宽容性问题。
- `iphone/PetAppTests/App/AppContainerTests.swift`：新增 `testValidatedBaseURLRejectsMalformedInputs`、
  `testValidatedBaseURLAcceptsValidHTTPAndHTTPS` 两个 case，共 6 类输入断言。

## 未完成事项 / 后续 TODO

- 无。本 fix 收口 round 4 唯一 [P2]。round 5 review 触发条件下应 PASS（codex round 4 已不再
  flag round 3 已 defer 的 default localhost 问题，相当于接受了 ATS fix + defer 决定）。
