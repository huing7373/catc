---
date: 2026-04-26
source_review: codex review round 2 on Story 2.4 (file: /tmp/epic-loop-review-2-4-r2.md)
story: 2-4-apiclient-封装
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — baseURL 与 endpoint.path 字符串拼接的双斜杠陷阱

## 背景

Story 2.4 `APIClient` 用 `URL(string: baseURL.absoluteString + endpoint.path)` 把 baseURL 与 endpoint.path 拼成最终 URL。codex round 2 review 指出：若调用方传入合法但带 trailing slash 的 baseURL（如 `https://api.example.com/api/v1/`），endpoint.path 又必须以 `/` 开头（既有契约），拼出来的 URL 是 `.../api/v1//version`，多出一个斜杠。许多反向代理 / 服务端会把 `//` 当不同路径处理，造成 404 或路由签名 mismatch。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | baseURL trailing slash 与 endpoint.path 前导 slash 拼接产生 `//` | medium (P2) | architecture | fix | `iphone/PetApp/Core/Networking/APIClient.swift:46-72` |

## Lesson 1: 字符串拼接 baseURL + path 时，必须在边界吸收一侧的 slash

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/APIClient.swift:46-72`（init 里 normalize）

### 症状（Symptom）

```swift
let baseURL = URL(string: "http://host/api/v1/")!     // 合法 URL，trailing slash
let path = "/version"                                  // Endpoint 契约要求 `/` 开头
let url = URL(string: baseURL.absoluteString + path)!  // → "http://host/api/v1//version"
```

`//version` 在 RFC 3986 里和 `/version` 是不同 path（前者在严格 server / 反向代理 / CDN 签名里会被当作 empty segment 起始的新路径）。NGINX 默认 collapse，但 Cloudflare / API Gateway / 自家代理签名校验都可能踢回 404 或拒签。客户端默认行为不能依赖某一类代理"恰好宽容"。

### 根因（Root cause）

两边都"觉得自己应该带 `/`"导致重叠：

- **baseURL 一侧**：URL 字符串以 `/` 结尾在 RFC 上是合法的、也是配置文件里更常见的写法（`https://api.example.com/api/v1/`），调用方没理由禁止。
- **endpoint.path 一侧**：Endpoint 契约里 `path` 必须 `/` 开头（保证 path 单独可见时即识别为绝对路径，不会被误读成相对当前请求 URL 的 path）。

任何一侧改契约都不够好（baseURL 侧禁 slash 等于把 caller 的合法输入判违规；endpoint.path 侧禁前导 slash 又让 path 字段失去自描述性）。**正确做法是 APIClient 内部吸收 baseURL 的 trailing slash**——init 时一次性 normalize，存进私有 stored property，后续拼接保证不重复。

为什么 `URL.appendingPathComponent` / `URLComponents` 不是首选：

- `appendingPathComponent("/version")` 会把前导 `/` 当作"想要追加一个 empty segment"，行为反直觉，且不同 iOS 版本结果有差异（社区有踩坑历史）。
- `URLComponents` 拼接需要分别管 path / query / fragment，对 MVP 范围（无 query 接口）是过度设计。

最小改动：init 里 strip baseURL 末尾 `/`，**单点修复 + 单点测试覆盖**。

### 修复（Fix）

`APIClient.init` 内对传入 baseURL 调用 `Self.normalize(_:)` 去掉 trailing slash 后才赋给 stored property。`buildURLRequest` 处的拼接代码不变（依赖 stored baseURL 已经规范化）。

```swift
// before
public init(baseURL: URL, session: URLSessionProtocol = URLSession.shared) {
    self.baseURL = baseURL          // 原样保存，trailing slash 流入拼接
    self.session = session
}

// after
public init(baseURL: URL, session: URLSessionProtocol = URLSession.shared) {
    self.baseURL = Self.normalize(baseURL)   // 一次吸收 trailing slash
    self.session = session
}

private static func normalize(_ url: URL) -> URL {
    let s = url.absoluteString
    guard s.hasSuffix("/") else { return url }
    let trimmed = String(s.dropLast())
    return URL(string: trimmed) ?? url       // 失败回退原值（极少见）
}
```

回归测试新增 `APIClientTests.testBaseURLTrailingSlashIsNormalizedAtInit`：传 `http://host/api/v1/` + `/version`，断言 `URLRequest.url.absoluteString == "http://host/api/v1/version"`（不是 `//version`）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写 "baseURL + path 字符串拼接" 时，**必须**在拼接边界处的某一侧（推荐：构造 client 时在 baseURL 侧）做一次 normalize，吸收掉两侧重叠的 `/`，并配一个回归测试覆盖 trailing slash 输入。

> **展开**：
>
> - **不要**把"调用方应该传无 trailing slash 的 baseURL"写成隐式假设。`https://host/api/v1/` 是 RFC 合法 URL string，配置文件里也合理，调用方无义务知道你内部怎么拼。
> - **吸收 slash 的位置**：优先 client init 时一次性 normalize（stored property 永远干净），不要在每次 `buildURLRequest` 里 strip——后者意味着每次拼接都要复算一次，且容易在多个拼接点漏掉一处。
> - **endpoint.path 侧坚持 `/` 开头的契约**：path 单独看时即知是绝对路径，移植 / log / debug 都更清晰。两边契约不对称没关系，只要 client 在边界吸收。
> - **不要**因为"现在的 server 默认配置会 collapse `//`"就放过——server / 代理 / CDN 任何一层换实现都会引爆。
> - **反例 1**：把 baseURL 当 `URL` 直接 `appendingPathComponent("/version")`——iOS 不同版本对前导 `/` 的处理有差，结果 URL 不稳定（不同 simulator 测得不一样）。
> - **反例 2**：在 buildURLRequest 里写 `if baseURL.absoluteString.hasSuffix("/") { ... } else { ... }` 分支——条件分支翻倍、拼接点一旦增加就每个都要复制一遍。
> - **反例 3**：禁止 caller 传 trailing-slash baseURL（在 init 里 fatalError 或 assert）——把合法输入判违规，违反"宽进严出"原则。

### 顺带改动

- `APIClientTests.swift` 新增 1 个回归 case `testBaseURLTrailingSlashIsNormalizedAtInit`（Story 2.4 AC7 单测列表里追加 1 条）。
- `APIClient.swift:149-152` 注释更新，明确"baseURL 已被 normalize，path 必须 `/` 开头，故拼接结果一定形如 `https://host/api/v1/path`"。
