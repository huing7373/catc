---
date: 2026-05-04
source_review: codex review of Story 8.1 r2 (`/tmp/epic-loop-review-8-1-r2.md`)
story: 8-1-healthkit-接入
commit: <pending>
lesson_count: 3
---

# Review Lessons — 2026-05-04 — HK read 授权必须 probe-read 推断；UITest 0 是合法 fallback；preseed flag 不能只在 probe 路径生效

## 背景

Story 8.1 第 2 轮 codex review 抓出 3 条问题，全部围绕"API 合约一致性 + UITest robustness + DEBUG flag scope"
三个主题：

1. **[P2] API 合约不一致**：`HealthProviderImpl.requestPermission()` 永远不会返 `false`，
   因为 `HKHealthStore.requestAuthorization` 在用户拒绝时仍 callback `success == true`
   （Apple 故意防探测）；但 `HealthProvider` 协议和 `HealthProviderMock` 都把 `false` 当 deny 信号 ——
   生产路径永远不触发 deny 分支，mock 测出的 deny path 在生产下死代码。
2. **[P1] UITest 与文档自相矛盾**：`HealthProviderIntegrationTests` 把"任何数字"当 happy path
   并断言 `>= 5000`；但 AC7 红线和 ProbeView 注释都钦定 sandbox 返 `0` 是合法 fallback ——
   simulator 上 read 成功但拿到 0 时测试反而断言失败，AC7 直接 flaky。
3. **[P2] regression（round 1 修 race 引入）**：r1 把 `-PetAppPreseedHealthKitSteps` 的 seed 消费者
   收紧到 `HealthProviderProbeView.task` 里 await。这导致**非 probe 模式下** flag 完全失效 ——
   未来要在常规 UI 路径下 seed 步数（如演示步数累加 UI 的 dev tooling）就破功。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | HK read 授权不能 trust `requestAuthorization` 的 success；必须 probe-read 推断 | medium (P2) | architecture / api-contract | fix | `iphone/PetApp/Core/Health/HealthProviderImpl.swift`, `iphone/PetApp/Core/Health/HealthProvider.swift` |
| 2 | UITest 把"任何数字"当 happy path 与 sandbox `0` fallback 直接撞车 | high (P1) | testing | fix | `iphone/PetAppUITests/HealthProviderIntegrationTests.swift` |
| 3 | `-PetAppPreseedHealthKitSteps` 在 non-probe 路径下失效（round 1 fix 引入） | medium (P2) | architecture / regression | fix | `iphone/PetApp/App/PetAppApp.swift` |

## Lesson 1: HK read 授权不能 trust `requestAuthorization` 的 success；必须 probe-read 推断

- **Severity**: medium (P2)
- **Category**: architecture / api-contract
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Health/HealthProviderImpl.swift:22-38`

### 症状（Symptom）

`HealthProviderImpl.requestPermission()` 实装：

```swift
healthStore.requestAuthorization(toShare: [], read: [stepCountType]) { success, error in
    if let nsError = error as NSError? {
        continuation.resume(throwing: ...systemFailure...)
    } else {
        continuation.resume(returning: success)  // ← 总是 true
    }
}
```

`HKHealthStore.requestAuthorization` 在用户**拒绝 read 权限**时 Apple 钦定**仍返 `success == true`**
（隐私契约：禁止外部探测真实授权状态，否则可推断"是否使用过 Apple Health"等敏感信息）。
因此本实装的 `requestPermission()` **永远不会返 false**。但 `HealthProvider` 协议把 false 当 deny
信号，`HealthProviderMock.requestPermissionStub = .success(false)` 也支持显式 deny stub。
mock 测出的"deny → caller 走 false 分支"在生产下是死代码。

### 根因（Root cause）

未读透 Apple 隐私契约：`HKHealthStore.requestAuthorization` 的 `success` 字段语义不是
"用户授权了"，而是"调用流程没崩"。同样陷阱：`authorizationStatus(for:)` 对 read 类型也故意返
`.sharingDenied` 防探测（仅对 share/write 类型才返真实状态）。Apple 文档
（"Protecting User Privacy"）明示这两条限制，但写实装时按"普通 OS 权限弹窗 API 都返 grant/deny"
的直觉走，没核对 read-specific 行为。

### 修复（Fix）

`requestPermission()` 拆两步：requestAuthorization 完成后做一次 **probe-read**（试读当日步数），
让 `readDailyTotalSteps` 的真实错误路径决定返回值：

```swift
public func requestPermission() async throws -> Bool {
    guard HKHealthStore.isHealthDataAvailable() else { throw .healthDataNotAvailable }

    // Step 1: 走 requestAuthorization 完成系统弹窗（success 字段忽略——见上文）
    let _: Void = try await withCheckedThrowingContinuation { continuation in
        healthStore.requestAuthorization(toShare: [], read: [stepCountType]) { _, error in
            if let nsError = error as NSError? {
                continuation.resume(throwing: .systemFailure(underlying: nsError))
            } else {
                continuation.resume(returning: ())
            }
        }
    }

    // Step 2: probe-read 拿真实信号
    do {
        _ = try await readDailyTotalSteps(date: Date())
        return true
    } catch HealthProviderError.permissionDenied {
        return false
    } catch {
        throw error  // healthDataNotAvailable / systemFailure 直接上抛
    }
}
```

**为什么 probe-read 可靠（HK 规则关键）**：HK 在 read deny 时会让 query 走
`errorAuthorizationDenied`（NSError code），**不会**返 0 静默通过；read 已授权但当日无 sample
才会返 0。两条路径在 HK 实装上分叉清晰，所以 probe-read 是 deny-vs-grant 的可靠 idiom。

`HealthProvider.swift` 协议注释也同步更新，明示这条契约（替换原来"`authorizationStatus(for:)`
对 read 故意返 sharingDenied"那段，避免读者误以为"那就用 readDailyTotalSteps 自然失败抛"——
那是 caller 才能感知的，requestPermission 的返回值需要内部 probe）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写 HealthKit 权限申请代码** 时，**禁止** **trust `HKHealthStore.requestAuthorization` callback 的 `success` 字段或 `authorizationStatus(for:)` 对 read 类型的返回值作为"已授权"信号**；必须 **在 requestAuthorization 完成后立即做一次 probe-read，用 query 抛 `errorAuthorizationDenied` 与否推断真实授权**。
>
> **展开**：
> - HK 隐私契约（Apple 钦定）：read 类型故意 leak-resistant ——`success == true` 不代表授权，
>   `authorizationStatus(for:)` 对 read 故意返 `.sharingDenied`，不是真实状态.
>   仅 share/write 类型的状态查询是真实的.
> - probe-read 的可靠性：HK 在 read deny 时 query 抛 `errorAuthorizationDenied`，不会静默返 0.
>   read 已授权但当日无 sample 才返 0（合法值）. 错误路径是分叉的，所以 probe-read 不会假阳/假阴.
> - **反例**：写 `func requestPermission() -> Bool { ...callback 里 return success... }`
>   这种"OS 权限申请"模板通用代码 → 在 HK read 上死 false 分支 → mock 写出来的 deny 测试在生产
>   永远不触发. 类似陷阱：用 `authorizationStatus(for: .stepCount)` 判断 read 是否授权 —— 错误.

## Lesson 2: UITest 把"任何数字"当 happy path 与 sandbox `0` fallback 直接撞车

- **Severity**: high (P1)
- **Category**: testing
- **分诊**: fix
- **位置**: `iphone/PetAppUITests/HealthProviderIntegrationTests.swift:85-89`

### 症状（Symptom）

UITest 原写法：

```swift
if lastResultLabel != "-", let actual = Int(lastResultLabel) {
    XCTAssertGreaterThanOrEqual(actual, 5000, "happy path 期望 >= 5000；实际 \(actual)")
} else {
    // sandbox-limited 路径：errorLabel 必须非空 → PASS
}
```

但 ProbeView 注释和 AC7 红线都钦定：simulator HK 返 0 也是合法 sandbox fallback（read 已授权但
seed 写入未生效 / HK 不返本进程 sample / 模拟器 sandbox 行为）。simulator 在新 Xcode runtime 上
read 路径不抛错而是返 0 时，本测试的 `Int("0")` 解析成功 → 走第一分支 → `0 >= 5000` 断言失败.
AC7 在 simulator 上 flaky（甚至常红）。

### 根因（Root cause）

把"resultLabel 是数字"当成 happy path 的二元判断，没拆"数字 == 0 vs 数字 > 0"两个子状态。
红线文字写在 ProbeView 注释里却没在 UITest 复刻 ——"红线在 A 文件、断言在 B 文件"导致两边
独立演进时漏覆盖某个合法 sandbox path。

### 修复（Fix）

把 result label 路径拆三态：

```swift
if lastResultLabel != "-", let actual = Int(lastResultLabel) {
    if actual == 0 {
        // sandbox-limited 数值表达：read 成功但拿到 0
        // 不强求 >= 5000；记录 INFO，仍 PASS
        print("INFO: simulator HealthKit sandbox 已授权但 read 返 0...")
    } else {
        XCTAssertGreaterThanOrEqual(actual, 5000, "happy path（result>0）期望 >= 5000；实际 \(actual)")
    }
} else {
    // error path：errorLabel 必须非空
    XCTAssertFalse(lastErrorLabel.isEmpty, "...")
}
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **为带 sandbox 限制的系统 API（HealthKit / Camera / Location 等）写 UITest 断言** 时，**必须** **把"路径走通"和"数值符合预期"拆两层判断**——把 sandbox-limited 合法返回值（如 HK 的 `0`）显式列入 PASS 路径，而非把"出现数字"等同于 happy path.
>
> **展开**：
> - 三态而非二态：`-`（path 未走通，FAIL）/ `0`（path 走通 + sandbox-limited，PASS）/ `>0`（happy path，断言数值）.
>   只用"是否数字"二分会把 sandbox `0` 错当 happy path → 假阳 fail.
> - 红线必须双向同步：ProbeView 注释写 sandbox fallback 行为、UITest 必须复刻同样的判定. 用 lesson 文档
>   记录契约让未来 Claude 知道两个文件要同步.
> - **反例**：`if let n = Int(label) { XCTAssertGreaterThanOrEqual(n, 5000) }` —— 把 0 当 happy
>   path 强断言 → simulator HK 返 0 时测试常红，开发者反复重试又"偶尔过"，最终归类为 flaky 退出
>   CI 关键路径. 真实问题是断言写漏 sandbox-limited 子状态.

## Lesson 3: `-PetAppPreseedHealthKitSteps` 在 non-probe 路径下失效（round 1 fix 引入）

- **Severity**: medium (P2)
- **Category**: architecture / regression
- **分诊**: fix
- **位置**: `iphone/PetApp/App/PetAppApp.swift:57-63`

### 症状（Symptom）

round 1 codex 抓出 race（`PetAppApp.init` 用 detached Task fire-and-forget seed），lesson 钦定
"DEBUG seed 必须串到消费者 .task 里 await"。round 1 fix 把 seed 消费者从 detached Task 收紧
到 `HealthProviderProbeView.task` 里 await：

```swift
// PetAppApp.body
if useHealthProviderProbe {
    HealthProviderProbeView(healthProvider: ..., preseedSteps: preseedStepsForProbe)
} else {
    RootView()  // ← preseedSteps 在这条路径完全不消费
}
```

结果：`-PetAppPreseedHealthKitSteps 5000` 在**非 probe 模式**（即不带
`-PetAppRunHealthProviderIntegrationProbe`）下完全失效——RootView 不消费 seed，HKHealthStore
什么都没写。

未来场景比如"在常规 UI 跑 dev tooling 演示步数累加 + 同步动画"想用此 flag 预置环境就破功。

### 根因（Root cause）

round 1 fix 把"修 race"和"窄化 seed scope"两件事合并成一步——consumer 从 detached Task 收紧到
ProbeView 时，没考虑"未来非 probe consumer"。修 race 的核心约束是"串到 consumer .task 里
await"（一对一 happens-before 关系），但**不**要求"只能有一个 consumer"——这是过度收紧.

### 修复（Fix）

把 PetAppApp 重命名 `preseedStepsForProbe` → `preseedSteps`（去掉 "ForProbe"
suffix，反映现在两条路径都消费）；引入 `RootBootstrapView`（DEBUG-only wrapper），
让 RootView 路径也成为 seed consumer：

```swift
// PetAppApp.body
if useHealthProviderProbe {
    HealthProviderProbeView(healthProvider: ..., preseedSteps: preseedSteps)
} else {
    RootBootstrapView(preseedSteps: preseedSteps)  // ← 新增
}

#if DEBUG
struct RootBootstrapView: View {
    let preseedSteps: Int?
    @State private var seedDone: Bool = false

    var body: some View {
        Group {
            if preseedSteps == nil || seedDone {
                RootView()
            } else {
                Color.clear  // 极短窗口（HK seed < 100ms 通常）
            }
        }
        .task {
            guard let steps = preseedSteps, !seedDone else { return }
            try? await HealthKitDevSeedUseCase.preseedToday(steps: steps)
            await MainActor.run { seedDone = true }
        }
    }
}
#endif
```

约束保持：seed 串在 RootBootstrapView 的 `.task` 里 `await`（不是 detached），符合
round 1 lesson；preseedSteps == nil 时直接渲染 RootView，零开销路径。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **修复 race / fire-and-forget bug 时**，**必须** **只把"strict happens-before（串行 await）"作为约束，禁止顺手把"consumer 数量"或"consumer 路径"也一起收紧**——后者是 scope creep，会引入 regression.
>
> **展开**：
> - race 的修复约束是 happens-before 关系（producer 完成 → consumer 才用），不是"consumer 唯一性".
>   把"detached Task → 一个 consumer 的 .task"作为修法是对的；把"一个 consumer 的 .task → 全 app 只有
>   这一个 consumer"是过度.
> - 新增 wrapper view（如 RootBootstrapView）让多 consumer 路径都串行 await 是兼容的——
>   wrapper `.task` 入口 `await` 完 seed 再展示下游 view，约束保留.
> - DEBUG-only flag 的"任何模式都生效"是默认契约：用户写 launch arg 期望它生效，不期望它依赖
>   另一个 flag 才工作. 把 flag scope 隐性绑定到另一个 flag 上是反直觉的 regression.
> - **反例**：r1 的 fix 把 `preseedSteps` 从 instance var 直接挂到 `HealthProviderProbeView` 的
>   入参，且只在 probe 路径下传——consumer 数量从"detached Task" 收到"1 个具体 view"，
>   未来加另一种 dev tooling view 要 reuse 这个 flag 时只能再改 PetAppApp，违反"开放扩展、关闭修改"
>   的局部稳定性. 正确是定义"seed 消费契约"（任何下游 view 都可以承担），让 PetAppApp 不感知具体 consumer.

---

## Meta: 本次 review 的宏观教训

3 条 finding 都指向同一个深层模式：**单点真理（API 合约 / 红线契约 / flag scope）必须由代码 / 测试 / 文档**
**三方一致表达**，任何一方独立演进都会破坏单点。

- **Lesson 1**：协议层定义 deny 信号 → 实装必须真能产生 deny 信号 → mock 与实装在 deny 路径下行为一致.
- **Lesson 2**：ProbeView 注释钦定 0 是 fallback → UITest 必须把 0 列入 PASS 路径.
- **Lesson 3**：launch arg 的 user-facing 名字钦定"该 flag 控制 preseed" → 实装必须在所有渲染路径都消费 flag.

**Rule**：写跨文件契约时（协议-实装-mock；红线-断言；flag-消费者），用 lesson 文档/Story Dev Notes
做"契约 anchor"——所有独立演进的文件都引用 anchor，下次改动时反链回来检查兼容. 本仓库
`docs/lessons/` 已经在做这件事；Story 8.1 的 lesson 直接被 HealthProviderImpl / ProbeView /
UITest 三个文件引用，回填速度比 PR-review 反复来回快.
