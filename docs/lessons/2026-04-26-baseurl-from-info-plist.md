---
date: 2026-04-26
source_review: codex review round 1 on Story 2.5 (file: /tmp/epic-loop-review-2-5-r1.md)
story: 2-5-ping-调用-主界面显示-server-version-信息
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — 默认 baseURL 应从 Info.plist 读，不应硬编码 localhost

## 背景

Story 2.5 落地 `AppContainer` 作为 App 全局 DI 容器，默认 `init()` 里直接写：

```swift
let baseURL = URL(string: "http://localhost:8080")!
```

理由：MVP 阶段先打通 simulator 上"App 启动 → 调本机 server"链路；真机 e2e 是 Epic 3 demo 验收的工作。
codex round 1 review 反馈：默认值不应硬编码 localhost，因为真机上 `localhost` 解析为设备自身（不是 Mac），
落地真机直接死。即使本 story 范围不做真机，"默认值是 localhost"也是**配置层的设计缺陷**，应配置化。

注意区分两个问题：
- "真机能不能跑 e2e" —— 本 story 范围不在，由 Epic 3 demo 验收负责。
- "默认 baseURL 怎么暴露给配置" —— **本 story 必须解决**，是工程实践问题。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | AppContainer 默认 baseURL 硬编码 localhost，真机落地无路可走 | medium (P2) | architecture | fix | `iphone/PetApp/App/AppContainer.swift:34-36`、`iphone/PetApp/Resources/Info.plist`、`iphone/project.yml` |

## Lesson 1: 默认 baseURL 必须可通过 Info.plist / xcconfig 覆盖；硬编码 localhost 仅作 last-resort fallback

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/App/AppContainer.swift`（resolve 函数 + 默认 init）、`iphone/project.yml`（info.properties 加 `PetAppBaseURL` 默认值）

### 症状（Symptom）

```swift
// before
public convenience init() {
    let baseURL = URL(string: "http://localhost:8080")!
    self.init(apiClient: APIClient(baseURL: baseURL))
}
```

- Simulator：localhost = host Mac，Mac 上 server 跑就行。✓
- 真机：localhost = 设备自己，永远 connection refused。✗
- 想切 dev / staging / prod：必须改源码 + 重新 build，无法通过配置切。✗
- 测试想验证"配置正确读取"：没有可注入的接口。✗

### 根因（Root cause）

把"运行环境特定的运行时配置"硬编码成"代码里的字面量字符串"——这是 12-factor 第 3 条（Config in environment）
在 iOS 端的常见违反姿势。iOS 没有 env var 直接对应物，但有等价机制：

- **Info.plist** key：xcconfig / build setting 注入 → plist 占位符替换 → runtime `Bundle.main.object(forInfoDictionaryKey:)` 读。
  适合"配置随 build 变（dev/staging/prod 三套 scheme）"。
- **UserDefaults / 本地配置**：适合"用户可改"（如 dev 工具里手动切 server）。
- **Remote config**：适合"运行时可改"（如 A/B 测试），MVP 不需要。

本 story 范围内最轻量、可扩展、与 xcconfig 切环境对接的方案就是 **Info.plist key**。
key 命名加产品前缀避免与 Apple 系统 key 冲突——选 `PetAppBaseURL`。

### 修复（Fix）

```swift
public static let baseURLInfoKey = "PetAppBaseURL"
public static let fallbackBaseURLString = "http://localhost:8080"

public convenience init() {
    let baseURL = AppContainer.resolveDefaultBaseURL(from: Bundle.main)
    self.init(apiClient: APIClient(baseURL: baseURL))
}

public static func resolveDefaultBaseURL(from bundle: Bundle) -> URL {
    if let raw = bundle.object(forInfoDictionaryKey: baseURLInfoKey) as? String,
       let url = URL(string: raw) {
        return url
    }
    return URL(string: fallbackBaseURLString)!
}
```

`resolveDefaultBaseURL(from:)` 提为 static + 接受 bundle 参数，正是为了测试能传入 mock bundle 验证读取逻辑
（见 `AppContainerTests.testResolveDefaultBaseURLFallsBackWhenKeyMissing` 用测试 target Bundle 验证 fallback 路径）。

`project.yml` 的 `PetApp.info.properties` 加 default 值 `PetAppBaseURL: http://localhost:8080`，xcodegen 后
Info.plist 自动包含此 key。后续切 staging / prod 通过新增 xcconfig 文件 + 在 `info.properties` 里改成
`$(PET_APP_BASE_URL)` 占位符即可，不需要再改源码。

**重要 scope 边界**：本 lesson 只管"配置层"（key 怎么读、fallback 怎么定）。"真机能不能 ping 通 server" 是
Epic 3 demo 验收的网络 / 部署问题，由那边的 story 负责验证。本 lesson 不要被理解为"提前实装真机 e2e 能力"。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在客户端代码里写"环境相关的字面量"（baseURL、API key、feature flag 默认值、
> 后端域名等）时，**必须**通过 `Bundle.main.object(forInfoDictionaryKey:)` + 一个有产品前缀的 key
> 读取，硬编码字面量只能作 last-resort fallback；并在 `project.yml`（xcodegen 项目）的
> `info.properties` 里写 default 值，使 Info.plist 持久包含该 key。

> **展开**：
>
> - **触发条件**：你正在写 `URL(string: "http://...")!`、`let apiKey = "sk-..."`、
>   `let environment = "production"` 这类字面量。停下来问："这个值在 dev/staging/prod 会一样吗？真机和
>   simulator 会一样吗？" 任一答案是"否"，立刻配置化。
> - **key 命名**：`<产品前缀>` + 语义名，例：`PetAppBaseURL`、`PetAppEnvironment`、`PetAppFeatureFlagX`。
>   不要用裸 `BaseURL`、`APIKey`——容易被未来引入的 SDK 抢走、读到错的值。
> - **fallback 必须存在**：`bundle.object(forInfoDictionaryKey:)` 在 key 不存在时返回 nil；要有 fallback
>   字面量值（如 localhost）保证 init 路径不抛错。fallback 仅作"开发期默认 / 配置缺失"安全网，不要作主路径。
> - **可测性**：把 resolve 逻辑提成 `static func resolveXxx(from bundle: Bundle) -> T` 接受 bundle 参数，
>   测试可传 mock bundle / 测试 target bundle 验证 happy + fallback。如果写在 init 里直接 `Bundle.main.xxx`，
>   测试没法独立验证 resolve 行为。
> - **xcodegen 用户的额外步骤**：`Info.plist` 是 xcodegen 生成的，每次 `xcodegen generate` 会**覆盖**手改。
>   key 必须落到 `project.yml` → `targets.<target>.info.properties` 才能持久化。手改 Info.plist 是临时
>   解，下次 generate 就丢——这一点容易在 review 后做 fix 时反复踩。
> - **反例 1**：直接 `URL(string: "http://localhost:8080")!` 一把梭——本 review 就是这个问题。
> - **反例 2**：用 `#if DEBUG` 区分 dev / release baseURL——build configuration 不止两档（还有 staging /
>   QA / beta），`#if` 分支不可扩展，xcconfig + Info.plist 才是。
> - **反例 3**：把 baseURL 放进 swift 源码常量再 `git checkout` 切环境——历史上人脑切 git branch 切环境
>   的方式是事故源（"我以为我在 dev branch 结果误打了 prod"）。配置文件随 build configuration 自动切，
>   人脑无负担。
> - **反例 4**：直接用 `UserDefaults` 存默认 baseURL——UserDefaults 适合用户行为类设置（夜间模式 / 通知开关），
>   不适合"build 时定死的环境配置"。两类配置混存会让 migration / debug 都更难。

### 顺带改动

- `iphone/PetApp/App/AppContainer.swift`：新增 `baseURLInfoKey`、`fallbackBaseURLString`、
  `resolveDefaultBaseURL(from:)`；默认 init 切到 resolve 路径。
- `iphone/PetApp/Resources/Info.plist`：xcodegen 生成时包含 `PetAppBaseURL` key，默认值
  `http://localhost:8080`（与原硬编码一致，行为对 simulator / 现有测试零差异）。
- `iphone/project.yml`：`targets.PetApp.info.properties` 追加 `PetAppBaseURL`，确保 `xcodegen generate`
  不会反向覆盖。
- `iphone/PetAppTests/App/AppContainerTests.swift`：新增 3 个 case 覆盖 main bundle 读取、测试 bundle
  fallback、默认 init sanity。

## 未完成事项 / 后续 TODO

- **2026-04-26（codex round 2 [P1]）**：codex round 2 review 又 flag 了 `project.yml:34-37` +
  `Info.plist` 的 default `PetAppBaseURL: http://localhost:8080` —— 真机上 ping 永远失败。
  - **Defer 决策**：不在 Story 2.5 修。本 story 范围明确"不实装 e2e 真机调用"（→ Epic 3 demo 验收
    才做）；MVP 阶段不存在"什么环境都能 work 的默认 URL"，因为 dev/staging/prod server URL 还没
    确定，dev/staging/prod 切换是 Epic 3+ 的工程任务。当前 fallback 行为对 simulator 开发友好，
    真机联调通过 xcconfig 覆盖（注释已说明）。
  - **后续动作**：留给 **Epic 3 demo 验收节点** 统一处理 dev / staging URL 配置策略（含选定哪种
    config 注入方式：xcconfig 多 scheme / Info.plist build setting 占位符 / debug menu 手动覆盖
    等）。原 review 见 `/tmp/epic-loop-review-2-5-r2.md`。
  - **触发回看本 lesson 的时机**：Epic 3 节点 1 demo 验收时，工程师需读"预防规则"段决定如何把
    default localhost 替换为真机可达的 URL（候选：xcconfig per-scheme / 启动期 debug menu /
    打包阶段写入）。
