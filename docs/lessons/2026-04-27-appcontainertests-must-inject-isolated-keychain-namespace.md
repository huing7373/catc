---
date: 2026-04-27
source_review: codex round 3 review of story 5-1-keychain-封装 (file: /tmp/epic-loop-review-5-1-r3.md)
story: 5-1-keychain-封装
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-27 — DI 容器的 production 默认值切换后，所有触发外部存储副作用的容器测试都必须改走注入路径

## 背景

Story 5.1 round 2 fix-review 已经把 `KeychainServicesStore.service` 改成 init 注入参数（默认值 = bundle id `com.zhuming.pet.app`），并把 `KeychainServicesStoreTests` 切到专属 namespace。但 round 3 codex review 发现还有漏网：`AppContainer` 的默认 init 此刻已切到 `KeychainServicesStore()`（生产 namespace），而 `AppContainerTests.testResetIdentityViewModelSharesContainerKeychainStore` 仍用 `AppContainer()` 默认 init 构造容器，再通过 `container.keychainStore` 走 `set("test-token", ...)` + `viewModel.tap()`（内部 `removeAll()`）—— 整套副作用全部命中生产 namespace。表现：

- 跑一次 `AppContainerTests` = simulator 上 `com.zhuming.pet.app` namespace 的所有 keychain item 被清
- PetAppUITests `KeychainPersistenceUITests` 跨 launch 持久化测试种入的 `guestUid` 被 unit test 顺手清掉，UI 测试随机失败
- 手动调试游客登录拿到的 token 跑完 unit test 也不见了

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | AppContainerTests 必须注入隔离 KeychainServicesStore namespace | medium (P2) | testing | fix | `iphone/PetAppTests/App/AppContainerTests.swift` |

## Lesson 1: DI 容器 production 默认值切换 = 触发外部存储副作用的容器测试**全部**要走注入路径

- **Severity**: medium (P2)
- **Category**: testing
- **分诊**: fix
- **位置**: `iphone/PetAppTests/App/AppContainerTests.swift:135`（修复前）

### 症状（Symptom）

`AppContainer()` 默认 init 切到 `KeychainServicesStore()`（生产 namespace `com.zhuming.pet.app`）后：

```swift
// 修复前 — 命中生产 namespace
func testResetIdentityViewModelSharesContainerKeychainStore() async throws {
    let container = AppContainer()  // → keychainStore 是 KeychainServicesStore() = 生产 namespace
    try container.keychainStore.set("test-token", forKey: "sessionToken")  // 写生产 keychain
    let viewModel = container.makeResetIdentityViewModel()
    await viewModel.tap()  // 内部 removeAll() → 清生产 namespace 全部 item
    XCTAssertNil(try container.keychainStore.get(forKey: "sessionToken"))
}
```

每次跑 unit test：手动 dev 联调写入的 `guestUid` / `authToken` 没了，PetAppUITests 跨 launch 验证用例随机失败。

### 根因（Root cause）

Round 2 fix-review 修了 `KeychainServicesStoreTests`（直接 `new KeychainServicesStore(service:)` 的测试）但**漏修了 `AppContainerTests`**（间接通过 `AppContainer()` 拿 keychain 的测试）。漏网原因：

1. **思维盲区"测试隔离 = 改测试 sut"**：以为只要测试直接构造 `KeychainServicesStore` 的地方传专属 namespace 就够了；忘了 `AppContainer` 的默认 init 也是一个测试入口，且其内部用的还是默认 namespace
2. **DI 容器是间接路径，污染源不显眼**：测试代码看上去只是 `let container = AppContainer()` —— 看不到 keychain 字样，扫 `KeychainServicesStore` 关键字也搜不到。但 production 默认值已经从占位 `InMemoryKeychainStore` 切到真实的 `KeychainServicesStore()`，所有走容器默认 init 的测试链路自动继承生产副作用
3. **codex round 2 review 自身的范围限制**：codex 当时关注点是 `KeychainServicesStore` 本身的 namespace 硬编码，没扫到 `AppContainer` 这一层间接污染源

### 修复（Fix）

只改 `AppContainerTests.testResetIdentityViewModelSharesContainerKeychainStore`：

```swift
// 修复后 — 显式注入隔离 namespace
let testService = "com.zhuming.pet.app.tests.\(UUID().uuidString)"
let isolatedKeychain = KeychainServicesStore(service: testService)
defer { try? isolatedKeychain.removeAll() }

let container = AppContainer(
    apiClient: APIClient(baseURL: AppContainer.resolveDefaultBaseURL(from: Bundle.main)),
    keychainStore: isolatedKeychain
)

// 后续 set / get / viewModel.tap() 全部在 isolatedKeychain 的 testService namespace 内做
```

**不**改 app code（`AppContainer` 默认 init 维持 `KeychainServicesStore()`）—— production 默认值就该是生产 namespace。

**不**改其他两个用 `AppContainer()` 的测试（`testDefaultInitProducesUsableContainer` / `testErrorPresenterIsStableSingletonWithinContainer`）—— 它们只构造 `KeychainServicesStore()`、不调 keychain 方法（构造仅存 namespace 字符串、不打开 SecItem），无副作用。

### 预防规则（Rule for future Claude）⚡

> **一句话**：DI 容器（`AppContainer` 类）的某个字段从"占位 in-memory 实现"切到"产线外部存储实现"时，**必须立刻搜全部 `<Container>()`（默认 init）的调用方**，把所有**会触发该字段方法调用**的测试改成注入隔离 instance；不能只看"是否直接 new 该字段类型"。
>
> **展开**：
> - 检查清单（默认 init 切产线值时立即跑）：
>   1. `grep -rn "<Container>()" --include="*.swift"` 找全部默认 init 调用方
>   2. 对每个调用方过滤："只构造 / 不调方法" → 安全；"调任何 set/get/remove/write/delete 类方法" → 必须改注入路径
>   3. 即使没调，长期看也建议为可注入路径预留 helper（如 `makeIsolatedTestContainer()`），让未来扩展不踩坑
> - **fix-review 自检**：当某轮 review 修了"namespace 可注入"但没动 `AppContainer` 默认 init，下一轮一定要专门 audit `AppContainer*Tests`；review baseline 的累计 diff 视野能看到 default value 变更，但 codex 未必扫到二级链路
> - **反例（踩坑模式）**：
>   - "我修了 KeychainServicesStoreTests 让它注入 namespace 就够了" —— 漏了 `AppContainerTests` 这层间接调用
>   - "AppContainer() 默认 init 没传 keychainStore，所以它和 keychain 隔离没关系" —— 默认参数让 `KeychainServicesStore()` 实例化生产 namespace，间接污染源更隐蔽
>   - "只要 production 默认走生产值就对" —— 对的；但同步要审"测试是不是无意继承了这个生产值"，两件事必须配对做
> - **更好的结构性预防**：长期看，DI 容器的产线默认值字段建议提供配套 `makeForTests()` static factory（注入隔离 instance），让所有测试默认走 `makeForTests()`、显式标注"我要测产线默认行为"的测试才走 `init()`。本 story 暂不引入此 factory（YAGNI），但记此规则供后续 epic 落地参考

---

## Meta: 本次 review 的宏观教训

Round 2 fix-review 已把 `KeychainServicesStore.service` 注入化，并修了直接 sut 的测试；round 3 codex 看 `--base baseline` 累计 diff 才发现 `AppContainer` 默认值切换 + `AppContainerTests` 用默认 init 这条**间接污染链**还在。教训：**fix-review 后写 lesson 时，"修复了哪些直接路径"和"哪些间接调用方需要联动改"是两个独立审查项**，前者由 review 直接驱动、后者要 fix 实施者自己 audit。今后 fix-review 改产线默认值类的 finding 时，lesson 末尾建议加一条 "联动改动检查清单" 字段。
