---
date: 2026-04-27
source_review: codex review (round 6) on Story 5.5 — /tmp/epic-loop-review-5-5-r6.md
story: 5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-27 — wire DTO → domain 转换：未知 enum 必须 fail-fast，禁止 silent fallback

## 背景

Story 5.5 落地 `HomeData(from: HomeResponse)` 时，把 `pet.currentState` 与 `chest.status`
两个 enum 字段用 `?? .rest` / `?? .counting` 兜底未知值，避免 init 抛错。codex round 6 [P2]
指出这是 schema drift 的反 pattern。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Reject unknown `/home` enum values instead of defaulting them | P2 | error-handling | fix | `iphone/PetApp/Features/Home/Models/HomeData.swift:46-59` |

## Lesson 1: frozen-schema 的 wire DTO 解 enum 失败必须抛 .decoding，不许 silent default

- **Severity**: P2 (medium)
- **Category**: error-handling（更准确：schema 兼容策略）
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/Models/HomeData.swift:35-65`（`init(from:)`）

### 症状（Symptom）

后端如果给 `pet.currentState` 或 `chest.status` 返回客户端未识别的 enum 值（无论是 server 加了
新 case 但客户端还没升级，还是真实数据损坏），客户端会**静默**把它当作 `.rest` / `.counting`
渲染首屏 —— 用户看到错的状态，dev / 监控收不到任何 signal。

### 根因（Root cause）

```swift
currentState: HomePetState(rawValue: pet.currentState) ?? .rest,
status: HomeChestStatus(rawValue: response.chest.status) ?? .counting,
```

`?? fallback` 把 "未识别" 当作 "等价于默认值" 处理。这种宽松解析在两类场景不可接受：

1. **frozen schema**：`/home` 协议在 V1 §4.1 行 16 已明示 frozen，理论上不可能出现未知值；
   一旦出现就意味着真实异常（server bug / 数据损坏 / 客户端落后于线上版本）。
2. **fail-fast 优先于 graceful degradation**：上游 mapper 已经为 `APIError.decoding` 配好了
   AlertOverlay 文案 "数据异常，请稍后重试"，alert 后用户重启 → 触发新冷启动 → dev / 监控
   能立刻在崩溃 / 错误上报里看到 schema drift 信号。silent fallback 把这个信号永久吞掉。

### 修复（Fix）

`HomeData(from:)` 改 `throws`；未知 enum 抛 `APIError.decoding(underlying: HomeDataDecodingError)`：

```swift
public init(from response: HomeResponse) throws {
    ...
    if let pet = response.pet {
        guard let petState = HomePetState(rawValue: pet.currentState) else {
            throw APIError.decoding(underlying: HomeDataDecodingError.unknownPetCurrentState(pet.currentState))
        }
        self.pet = HomePet(..., currentState: petState, ...)
    }
    guard let chestStatus = HomeChestStatus(rawValue: response.chest.status) else {
        throw APIError.decoding(underlying: HomeDataDecodingError.unknownChestStatus(response.chest.status))
    }
    self.chest = HomeChest(..., status: chestStatus, ...)
}

public enum HomeDataDecodingError: Error, Equatable {
    case unknownPetCurrentState(Int)
    case unknownChestStatus(Int)
}
```

下游传播：

- `LoadHomeUseCase.execute()` → `try HomeData(from: response)` → 透传 `APIError.decoding`
- 抛到 RootView `bootstrapStep1` → 包成 `BootstrapMappedError`，presentation 由
  `AppErrorMapper.presentation(for:)` 决定 → `.decoding` 钦定为
  `.alert(title:"提示", message:"数据异常，请稍后重试")`
- `LaunchedContentView.needsAuthContent` 渲染 `AlertOverlayView`，OK 按钮 `exit(0)` →
  用户重启 → 新冷启动 → dev / 监控立刻看到崩溃日志

测试同步更新：原 `testExecuteUnknownChestStatusFallsBackToCounting` /
`testExecuteUnknownPetStateFallsBackToRest` 改为
`testExecuteUnknownChestStatusThrowsDecoding` / `testExecuteUnknownPetStateThrowsDecoding`，
断言抛 `APIError.decoding(underlying: HomeDataDecodingError.unknown...(99))`。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 把 wire DTO 的 raw int / string 解析成本地 `enum` 时，**禁止**
> 用 `?? defaultCase` 或 `?? .unknown` 做宽松兜底；除非协议明确允许 forward-compatible 扩展，
> 否则**必须**让 init `throws`，让上游 fail-fast 抛 `APIError.decoding`。
>
> **展开**：
> - 判断准则：**协议是 frozen 还是 evolving**？
>   - frozen（如 V1 §4.1 行 16 钦定的 `/home`）→ 未知值是真实异常 → fail-fast。
>   - evolving（明示客户端要 forward-compatible，如 server 可以悄悄加新 case）→ 协议要先明确
>     fallback 语义（如新增一个 `.unknown` 真实 case 给 UI 用），再写 `?? .unknown`。
> - mapper 已经为 `.decoding` 配好 alert 文案 / OK→exit(0) 重启动作 → fail-fast 路径**完全
>   不需要新增任何 UI 代码**，下游基础设施已就位。
> - **反例 1**：`enum.init(rawValue: x) ?? .first` —— silent coerce，dev 期 0 signal。
> - **反例 2**：`#if DEBUG assertionFailure(); fallback in production` —— prod 仍是 silent；
>   dev 偶尔跑测试 / 模拟器才能发现，覆盖率不够。
> - **正例**：`guard let case = MyEnum(rawValue: x) else { throw APIError.decoding(...) }`。
> - **正例**：单独定义一个 `MyDataDecodingError: Error, Equatable` 携带未知 raw 值，作为
>   `APIError.decoding` 的 underlying，让测试可断言具体子类型 + log 能看到具体哪个字段坏了。
> - **诊断 hint**：写 wire DTO → domain 转换时，每看到一个 `enum.init(rawValue:)` 都问一遍
>   "这个 enum 是 frozen 还是 evolving？" 不能默认走 fallback。

## Meta

本轮（round 6）codex review 指出的两条 finding 之一。另一条（SwiftUI 多 `.task` 之间无序）
独立 lesson 在 `docs/lessons/2026-04-27-swiftui-multi-task-no-ordering.md`。

两条都是 [P2] dev-story 阶段就有的局部问题，与前 5 轮综合修复 regression 无关。
