---
date: 2026-04-30
source_review: codex review round 1 (file: /tmp/epic-loop-review-37-8-r1.md)
story: 37-8-roomview-scaffold
commit: 32a9d3c
lesson_count: 1
---

# Review Lessons — 2026-04-30 — Real ViewModel 的占位 init 必须 seed UI Scaffold 全部字段，不能"等 sink 派发"

## 背景

Story 37.8 落地 RoomScaffoldView 替代 Story 37.3 RoomViewPlaceholder，引入 `RoomViewModel`（基类）+ `MockRoomViewModel` / `RealRoomViewModel`（两子类）。codex round 1 review 发现 P2：`RealRoomViewModel.init()` / `init(appState:)` 仅 set `roomCodeForCopy` / `hostCatName` 占位，**不** seed `members` / `userIsHost` —— 在 Story 12.1 WS 接通前任何走 in-room state 的 Real path（`UITEST_FORCE_IN_ROOM` env / 手动 debug mutation / Story 37.12 后 JoinRoomModal 落地）都会让 RoomScaffoldView 渲染近乎空房间（0 实 + 4 虚线占位 + host cat 名缺失），让 Story 37.8 "RoomScaffoldView 替代 placeholder" 的 deliverable 在 Real 路径形同未交付。

修法走 option A：抽 `RoomScaffoldDefaults` 共享 struct，Mock 与 Real 双子类的 init 路径都以它 seed；Real 的 sink 路径作为 override（appState 派发首值 / reset 时 fallback 都用 defaults 占位）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | RealRoomViewModel 在 in-room state 暴露空 mock 数据 | P2 | architecture | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` |

## Lesson 1: Real ViewModel 占位 init 必须 seed UI Scaffold 用到的**所有**字段，不只是单值字段

- **Severity**: P2
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:42-58`

### 症状（Symptom）

`RealRoomViewModel` 两条 init（`init()` 与 `init(appState:)`）只 seed 了 `roomCodeForCopy` / `hostCatName` 这两个有 publisher 派生路径的 `@Published` 字段；而 `members` / `userIsHost` 这两个 Story 12.1 后才接 WS 的字段，被默认初始化为 `[]` / `false`。

后果：在 WS 落地前，任何走 RealRoomViewModel + in-room state 的路径（已存在的 `UITEST_FORCE_IN_ROOM` env，未来的 JoinRoomModal）都会让 RoomScaffoldView 收到空 `members` → 渲染 4 个 虚线占位 + 房主猫名为 "默认小猫" 占位 + userIsHost = false（与 Mock 满员状态相去甚远）。Story deliverable "RoomScaffoldView 替代 RoomViewPlaceholder" 在 Real 路径事实上**没生效**。

### 根因（Root cause）

写 RealRoomViewModel 时按"派生 state 走 sink、其它字段等 WS 落地"的二分法分类字段：

- `roomCodeForCopy` / `hostCatName`：appState 已有 publisher（`$currentRoomId` / `$currentPet`）→ sink 派生
- `members` / `userIsHost`：Story 12.1 才有数据源 → 暂置 `[]` / `false`

漏掉的认知：**"暂时没数据源" ≠ "暂时空着是 OK 的"**。MVP / scaffold 阶段的 Real ViewModel 在数据源未就绪前依然要让 UI 看起来"像那么回事"，否则等同于把 UI 退化到 placeholder 状态。Real 与 Mock 在 scaffold 阶段视觉上应该是**同等满足**的（mock 数据完全相同），区别只在于 Real 的 hookup 已为"未来覆盖"做好准备（sink 路径 / 注入入口）。

另一层：Story 37.8 epic 写"4 个 mock member 占位、host cat 占位都到位"是**对 RoomScaffoldView 的视觉契约**，不是仅对 Mock 的契约。Real 不 seed = 违约。

### 修复（Fix）

抽 `RoomScaffoldDefaults` 共享 enum（4 字段：`roomCodeForCopy` / `hostCatName` / `userIsHost` / `members`），Mock 与 Real 双子类 init 都用它 seed。Real 的 sink 路径在 fallback 分支（`nil` / 空字符串）也回到 defaults 占位（不再 fallback 到 `""` / `"默认小猫"`）。

Before（RealRoomViewModel.init()）：
```swift
self.roomCodeForCopy = ""
self.hostCatName = "默认小猫"
// members / userIsHost 用 base class default: [] / false
```

After：
```swift
self.roomCodeForCopy = RoomScaffoldDefaults.roomCodeForCopy   // "1234567"
self.hostCatName     = RoomScaffoldDefaults.hostCatName       // "小花"
self.members         = RoomScaffoldDefaults.members           // 4 mock members
self.userIsHost      = RoomScaffoldDefaults.userIsHost        // true
```

Sink fallback 也改：`roomId ?? RoomScaffoldDefaults.roomCodeForCopy`（不再 `?? ""`）。

`MockRoomViewModel.fourMembersMock` 改为 computed property 转发到 `RoomScaffoldDefaults.members`（外部 API 兼容；单源真值在 Defaults）。

新增守护测试 `testRealRoomViewModelInitSeedsRoomScaffoldDefaults`：断言两条 init 路径都 seed 了 `members.count >= 1` + `userIsHost == true`。

涉及文件：
- 新增 `iphone/PetApp/Features/Room/ViewModels/RoomScaffoldDefaults.swift`
- 改 `iphone/PetApp/Features/Room/ViewModels/MockRoomViewModel.swift`（init seed 走 Defaults）
- 改 `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`（init seed + sink fallback 走 Defaults）
- 改 `iphone/PetAppTests/Features/Room/RoomViewScaffoldTests.swift`（既有 case 断言对齐 + 新 case#8 守护）

测试结果：281/281 passing。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **写"UI Scaffold 阶段的 Real ViewModel"占位 init** 时，**必须 seed 视图用到的全部 `@Published` 字段为有意义的占位数据**，不能用"反正 Story X.Y 之后会接 WS"作为字段留空的理由。
>
> **展开**：
> - "Real" 与 "Mock" 在 scaffold 阶段应让 UI 视觉等价。区别只在 Real 的 hookup（sink / 注入入口）为未来覆盖做好准备，**不**在 Real 字段保持"未填充" 状态.
> - 抽 `XScaffoldDefaults` 共享 struct 是首选模式（避免 Mock / Real 双源 mock 数据漂移；Story 12.1 接 WS 后只需在 Real 内 sink 覆盖，Mock 不动）。
> - sink 路径的 fallback 分支（`appState.$xxx` 派 nil / 空时）也用 Defaults 占位，**不**用空字符串 / "默认 X" 之类的 ad-hoc placeholder（reset → in-room 不该让用户看到比初次进入更差的视觉）。
> - 加守护测试：`testRealXViewModelInitSeedsScaffoldDefaults` 直接断言 `RealXViewModel().keyField.count >= 1`（精确数依 Defaults），防未来人重构 init 时再次漏 seed。
> - **反例 1**：`self.members = []` 留 base class 默认值，等待 WS 填充 —— 在 WS 落地前任何 in-room 入口都会渲染空房间（Story 37.8 round 1 P2 原 bug）。
> - **反例 2**：Real 与 Mock 各自 hardcode 同一份 mock 数据 —— Story 12.1 改 WS 字段时容易漏改 Mock，单源真值规则在 epic 23 / 37 这种长 scaffold 期失效。
> - **反例 3**：sink fallback 写 `?? ""` —— reset 后 in-room state 重入时（少见但 UITEST 路径会触发）roomCode 退化成空，等于又回到了未交付状态。
> - **触发联想**：当看到一个 ViewModel 子类的 `@Published` 字段被注释成 "Story X.Y 接 WS 后填充" 而 init 内没有 seed → 立即想 "这字段在 WS 落地前 UI 看到的是什么？" 如果是空数组 / false / 空串 → 改 seed defaults。
