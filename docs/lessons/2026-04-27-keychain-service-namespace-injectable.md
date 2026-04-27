---
date: 2026-04-27
source_review: codex round 2 review of story 5-1-keychain-封装 (file: /tmp/epic-loop-review-5-1-r2.md)
story: 5-1-keychain-封装
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-27 — Keychain service namespace 必须可注入，测试不得复用生产 namespace

## 背景

Story 5.1 落地 `KeychainServicesStore`（基于 Apple Security.framework 的 `KeychainStoreProtocol` 真实实装）。第二轮 codex review 在 base..HEAD 累计 diff 上发现：`KeychainServicesStore.service` 被 `static let` 硬编码为 `com.zhuming.pet.app`，而新增的 `KeychainServicesStoreTests` 在 `setUp/tearDown` 中调用了 `removeAll()`——这意味着每次跑测试都会清掉 simulator 上同 namespace 下的所有 keychain item，包括手动调试遗留的 `guestUid` / `authToken`，并且会与 PetAppUITests 的 `KeychainPersistenceUITests` 互相 cross-talk。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Keychain service namespace 必须可注入 | medium (P2) | testing / architecture | fix | `iphone/PetApp/Core/Storage/KeychainServicesStore.swift`, `iphone/PetAppTests/Core/Storage/KeychainServicesStoreTests.swift` |

## Lesson 1: Keychain service namespace 必须可注入，测试不得复用生产 namespace

- **Severity**: medium (P2)
- **Category**: testing / architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Storage/KeychainServicesStore.swift:38-40`

### 症状（Symptom）

Unit test 跑一遍 = simulator 上生产 namespace 的所有 keychain 被 `setUp/tearDown` 的 `removeAll()` 清空。表现：
- 手动 dev 联调登录拿到的 `guestUid` 跑完 PetAppTests 后消失，必须重新走游客登录
- PetAppTests 与 PetAppUITests 共享同一 simulator 时 cross-talk：unit test 的 `removeAll()` 清掉 `KeychainPersistenceUITests` 跨 launch 验证种入的值
- 多 test bundle 并行（例如 CI matrix）会出现幻觉 flaky：A 的 setUp 删了 B 的种入

### 根因（Root cause）

写实装时为了"避免 unit test target bundle id 跟生产 bundle id 不一致带来的 namespace 分歧"，把 `service` 硬编码为 `com.zhuming.pet.app` 并设成 `static let`。这个推理本身没错（统一 namespace 让定位 keychain item 容易），但**把"统一 namespace"和"测试也用生产 namespace"混为一谈**了——正确的做法是：
- 生产代码默认用生产 namespace（保持手动诊断时 `security` 命令一查就到）
- 测试代码在 init 处显式注入专属 namespace（带 UUID 后缀，跨进程/跨 bundle 都不会撞）

`static let` 让测试**没有**注入入口，于是只能跟着生产用同一个，副作用全部回流到 dev / 其他 test target。

### 修复（Fix）

1. `KeychainServicesStore`：`static let service` → `static let defaultService` + 实例 `let service: String` + `init(service: String = defaultService)`；所有内部查询从 `Self.service` 改为 `self.service`
2. `AppContainer` 默认参数 `KeychainServicesStore()` 不变（走 `defaultService`，编译器 SR 保留）
3. `KeychainServicesStoreTests`：`setUp` 里给每个 test 实例生成 `"com.zhuming.pet.app.tests.\(UUID)"`，所有 sut 用此专属 namespace
4. `testPersistenceAcrossInstances` 里的两个 store 实例共用本 test 的 `testService`（否则两个默认实例会落在不同 namespace，验证不到"持久化"语义）
5. 新增 `testDifferentServiceNamespacesAreIsolated`：验证不同 service 互不干扰——这是"修复有效"的根机制断言

代码 before/after 简化版：

```swift
// before
public final class KeychainServicesStore: KeychainStoreProtocol {
    public static let service: String = "com.zhuming.pet.app"
    public init() {}
    // ... uses Self.service everywhere
}

// after
public final class KeychainServicesStore: KeychainStoreProtocol {
    public static let defaultService: String = "com.zhuming.pet.app"
    public let service: String
    public init(service: String = KeychainServicesStore.defaultService) {
        self.service = service
    }
    // ... uses self.service everywhere
}
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：实装任何 **进程级/设备级共享外部存储** 的 wrapper（Keychain / UserDefaults / FileManager 子目录 / SQLite db file / Redis namespace）时，**namespace 字段必须经 init 注入**，禁止 `static let` 硬编码——哪怕生产代码只会用一个值。
>
> **展开**：
> - 默认参数走生产值 → 业务代码零负担：`init(namespace: String = .defaultNamespace)`
> - 测试代码必须传专属 namespace（推荐 `"\(.defaultNamespace).tests.\(UUID)"`）—— 让 setUp 的 `cleanup()` 永远只清自己的隔离区
> - 写测试发现"为了隔离要 cleanup 共享 state"时，**先反过来问**：能否换成"测试操作的就不是共享 state"？能注入就注入，不能注入再 cleanup
> - 检验机制是否真生效：写一个 `testDifferentNamespacesAreIsolated` 风格的测试，断言"A 写 + B 读返 nil + B 删不影响 A"
> - **反例（踩坑模式）**：
>   - 在 `static let serviceID = "...prod..."` 上加注释"测试与生产共享，靠 setUp/tearDown 兜底" —— 这就是本次踩的坑，注释承诺 ≠ 真隔离
>   - 测试 `tearDown` 写 `try? store.removeAll()` 但 `store` 用的是默认 init —— 永远在删生产 namespace
>   - "我把 namespace 用 bundle id 区分 test/prod 就行了" —— bundle id 在 unit test target 是 `<prod>.tests` 但 UI test target 又是 `<prod>.uitests`，分裂会让"跨 bundle 持久化验证"测试失败；显式注入比依赖 bundle id 推断稳

---

## Meta: 本次 review 的宏观教训

第二轮 review 用 `--base baseline` 看累计 diff，前轮 lesson 文档全文也会被卷进 diff，但**结论永远在文件末尾的 `^codex$` 段**。解析 review 文件时直接 `tail -100` 找最后一段，前面 3000+ 行的 lesson 复述全部忽略——避免被前轮 lesson 误导（"这条上轮已经修过了？"）。
