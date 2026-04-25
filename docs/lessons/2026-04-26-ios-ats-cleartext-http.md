---
date: 2026-04-26
source_review: codex review round 3 on Story 2.5 (file: /tmp/epic-loop-review-2-5-r3.md)
story: 2-5-ping-调用-主界面显示-server-version-信息
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — iOS ATS 默认拒 cleartext HTTP，Info.plist 必须显式加例外

## 背景

Story 2.5 在 round 1 fix 后把默认 baseURL 配置化到 Info.plist `PetAppBaseURL = http://localhost:8080`，
但**没有**同时配置 `NSAppTransportSecurity` 例外。codex round 3 review [P1] 指出：iOS App Transport
Security（ATS）从 iOS 9 起对应用默认禁用 cleartext HTTP；没有 ATS 例外的 cleartext URL 在 OS 层就会被
`URLSession` 拒绝（典型错误：`NSURLErrorAppTransportSecurityRequiresSecureConnection`，code -1022），
连不到 socket 就别提 server 了。后果：模拟器 + 真机所有环境的 `/ping` / `/version` 请求都被拒绝，
HomeView 永远停在 "offline" 退化态，但单元测试因为用 `URLProtocol` stub 完全绕开 OS 网络栈，根本测不到。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Info.plist 缺 NSAppTransportSecurity 例外，cleartext localhost ping/version 被 ATS 拒 | high (P1) | security | fix | `iphone/PetApp/Resources/Info.plist`、`iphone/project.yml`、`iphone/PetAppTests/App/AppContainerTests.swift` |
| 2 | localhost default for physical device（round 2 复述） | high (P1) | architecture | defer | round 2 已 defer；维持本 story 范围红线，由 Epic 3 demo 验收节点处理；登记到 `2026-04-26-baseurl-from-info-plist.md` 的 TODO 段 |

## Lesson 1: cleartext HTTP 必须在 Info.plist 显式声明 ATS 例外，且首选 NSAllowsLocalNetworking

- **Severity**: high (P1)
- **Category**: security
- **分诊**: fix
- **位置**: `iphone/PetApp/Resources/Info.plist:23-29`、`iphone/project.yml:34-43`

### 症状（Symptom）

Info.plist 配置：

```xml
<key>PetAppBaseURL</key>
<string>http://localhost:8080</string>
```

App 跑 `URLSession.shared.dataTask(with: URLRequest(url: ...))`，OS 层立刻返回错误：

```
Error Domain=NSURLErrorDomain Code=-1022 "The resource could not be loaded
because the App Transport Security policy requires the use of a secure connection."
```

Server 这边 socket 上根本看不到连接到达，因为请求在 client 自己的 OS 网络栈里就被拦截了。

单元测试看不出问题——因为 `URLProtocol` stub（如 `PingStubURLProtocol`）拦在 `URLSession` 之上、
OS 网络栈之下，整个 ATS 检查路径被绕开。所以 100% 单测绿 + 真机/模拟器跑起来全 offline 是这个 bug 的标志。

### 根因（Root cause）

iOS 9+ 默认开启 ATS：所有 `URLSession` 走的连接必须满足三件事：HTTPS、TLS 1.2+、强密码套件。
`http://` scheme 直接违反第 1 条，OS 在拨号前就拒绝。

历史上常见绕过姿势是给 Info.plist 加：

```xml
<key>NSAppTransportSecurity</key>
<dict>
    <key>NSAllowsArbitraryLoads</key>
    <true/>
</dict>
```

但 `NSAllowsArbitraryLoads = true` **关掉所有 ATS 检查**（含公网 cleartext），是 App Store 提交时
Apple 会 challenge 的安全反模式。MVP 阶段连本地 server 完全用不着这把大锤。

iOS 10+ 增加了 `NSAllowsLocalNetworking`：仅放开 .local 域名 + 私有 IP 段（10/8、172.16/12、192.168/16）
+ link-local，**不**放开公网 cleartext。这是连 localhost / Mac 上 dev server 的正确开关，比
ArbitraryLoads 安全得多。

### 修复（Fix）

`iphone/PetApp/Resources/Info.plist`：

```xml
<key>LSRequiresIPhoneOS</key>
<true/>
<key>NSAppTransportSecurity</key>
<dict>
    <key>NSAllowsLocalNetworking</key>
    <true/>
</dict>
<key>PetAppBaseURL</key>
<string>http://localhost:8080</string>
```

`iphone/project.yml`（关键：xcodegen 用户**必须**同步在 yml 里加，否则下次 `xcodegen generate` 会
覆盖 Info.plist 把 ATS key 丢掉，回到 bug 原状）：

```yaml
info:
  properties:
    # ...
    PetAppBaseURL: http://localhost:8080
    NSAppTransportSecurity:
      NSAllowsLocalNetworking: true
```

`iphone/PetAppTests/App/AppContainerTests.swift` 加测试 `testInfoPlistAllowsLocalNetworking`：
读 PetApp.app 的 Info.plist（`Bundle(for: AppContainer.self)`）断言
`NSAppTransportSecurity.NSAllowsLocalNetworking = true`。

注意：单测**不能**直接验证 ATS 行为（OS-level 决策需要真实 network call），但能验证 plist 配置存在。
配置存在 + Apple 文档承诺一起，作为 ATS 行为的代理证据。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 iOS 端写任何 `http://` cleartext URL（含 localhost、本机 IP、内网 IP
> 都算）时，**必须**同时在 Info.plist 加 `NSAppTransportSecurity → NSAllowsLocalNetworking = true`
> 例外；**禁止**用 `NSAllowsArbitraryLoads = true` 兜底（除非有公网 cleartext 真实需求且经过 review）。
> xcodegen 项目下，Info.plist 改完**必须**同时改 `project.yml` 的 `info.properties`，否则下次
> `xcodegen generate` 会反向覆盖。

> **展开**：
>
> - **触发条件**：你写或读到 `URL(string: "http://...")`、Info.plist `PetAppBaseURL` 是 http 开头、
>   xcconfig 里有 cleartext URL 占位符、或 review 反馈"feature 在真机/模拟器都连不上"。立刻自查
>   Info.plist 是否有 `NSAppTransportSecurity` 段。
> - **三档例外按需要从轻到重选**：
>   1. `NSAllowsLocalNetworking = true`：只放 .local 域 + 私有 IP + link-local。**dev/MVP 默认选这个**。
>   2. `NSExceptionDomains` per-domain 例外：放某个 specific public 域 cleartext。第三方 SDK 强依赖、
>      短期方案。
>   3. `NSAllowsArbitraryLoads = true`：放所有 cleartext。**禁止**作默认；只在有明确公网 cleartext
>      正当理由 + 走过 security review 时用。
> - **xcodegen 协议**：所有 Info.plist 改动**必须**先改 `project.yml` → `targets.<target>.info.properties`，
>   再 `xcodegen generate`。直接改 Info.plist 是临时解，下次 generate 就丢——这是 round 1 fix 已经踩过
>   的坑（见 `2026-04-26-baseurl-from-info-plist.md` 预防规则段），ATS 这次再次确认。
> - **测试覆盖策略**：ATS 行为是 OS-level，单元测试用 `URLProtocol` stub 时根本不经过 ATS，**测不到**。
>   退而求其次：测试断言 plist 配置**存在**（`Bundle.object(forInfoDictionaryKey:)` 读出预期 key
>   + 预期值），把"配置缺失 → ATS 拦截"的链路前半截守住。这是配置类问题的标准防御姿势。
> - **诊断技巧**：feature 测试全绿 + 真机/模拟器跑起来 OS 报错 -1022 / "App Transport Security policy"，
>   九成是这个问题。切莫怀疑 server / 网络 / firewall 链路，先看 plist。
> - **反例 1**：用 `NSAllowsArbitraryLoads = true` 一刀切。审查时会被 challenge；本质是把"我懒得想清楚
>   ATS 策略"暴露在配置里。
> - **反例 2**：只改 Info.plist 不改 project.yml（xcodegen 项目）—— 下次 regen 就回退。
> - **反例 3**：把 ATS 例外加到 PetAppTests 或 PetAppUITests target 的 plist 里——错位。被测的是
>   PetApp.app，例外要加到 PetApp target；test target 的 plist 是 test runner 用的，与被测 App 不通。
> - **反例 4**：写单测断言"URL 能正常 fetch"来"测 ATS"——这等于做集成测试，需要起真 server，违反
>   Story 2.5 测试金字塔决策（`docs/lessons/2026-04-26-no-real-network-in-tests.md` 系列规则）。
>   断言 plist 配置就够。

### 顺带改动

无（本 fix 只改 3 个文件：Info.plist、project.yml、AppContainerTests.swift）。

## Meta: 配置类 review finding 的两层防御

本 round 配合 round 1 / round 2 一起看，能提炼出客户端配置类问题的固定防御套路：

1. **配置可注入**（round 1 lesson）：硬编码 → Info.plist key + xcconfig 占位符 + bundle 读取 + 测试桩。
2. **配置必合法**（round 3 lesson）：iOS 平台对配置内容（cleartext / TLS / cookie / sandbox）有
   OS-level 强制策略，配置存在 ≠ 配置生效。配置后必须查 OS-level 拦截层（ATS、ATS 子规则、
   Sandbox、TCC、Background Mode）是否会一脚踢飞它。
3. **xcodegen 双写**：两步都必须**同时**改 Info.plist + project.yml，单写一边的就是定时炸弹。

未来 Claude 在 iOS 端做配置改动时，三步走（注入 / 合法 / 双写）一遍过，可以避开本 epic 重复出现
review round 1→2→3 都在这条线上反复 ping pong 的情况。
