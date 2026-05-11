---
date: 2026-05-11
source_review: codex review (epic-loop round 5) — /tmp/epic-loop-review-12-7-r5.md
story: 12-7-创建-加入-退出-use-case-主界面入口完善
commit: 89176f8
lesson_count: 2
---

# Review Lessons — 2026-05-11 — leave thrown-error 也要 stale guard + create useCase nil 不能 hard no-op

## 背景

Story 12.7 落地"创建/加入/退出 UseCase + 主界面入口"全链路；r2 已让 `DefaultLeaveRoomUseCase` 内 200/6004
两路 guard `targetRoomId == currentRoomId` 防 stale-response 抹掉新房间。本轮 r5 codex 找出剩下两条：
（1）leave 的 thrown-error 路径（1009/network）也需要同样的 stale guard —— 老实装在 `RealRoomViewModel.onLeaveTap`
catch 块**无条件** `presenter?.present(error)`，stale 错误 overlay 会弹到用户已经切去的新房间之上；
（2）`UITEST_SKIP_GUEST_LOGIN=1` / RootView 老 wire 路径下 `createRoomUseCase` 为 nil 时 `onCreateTap` 直接 return —
join / leave / friend-join 都有 fallback，**只有 create 漏了** → create CTA 变成 hard no-op，UI tests / previews 无法
进 RoomView。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | leave thrown-error 路径必须 guard target==current 防 stale 错误 overlay | P2 (medium) | architecture | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:739-744` |
| 2 | create useCase nil 必须 fallback mutate appState（同 onJoinRoomConfirm 精神） | P3 (low) | architecture | fix | `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift:142-144` |

## Lesson 1: leave thrown-error 路径也要 guard target==current 才能 present errorPresenter

- **Severity**: medium (P2)
- **Category**: architecture（concurrency / state-machine race）
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:739-744`（旧版本）

### 症状（Symptom）

时序（codex r5 r1 给的复现序列）：

1. 用户在 room_A，点"离开房间" → `RealRoomViewModel.onLeaveTap` 调 `leaveRoomUseCase.execute()`（HTTP POST in-flight）
2. tab bar 仍可用 → 用户切 tab → 在 Home 加入 room_B → `appState.setCurrentRoomId("room_B")`
3. room_A 的 leave 请求 1009 / network error 迟到 → useCase throw
4. **老实装**：onLeaveTap catch 块**无条件** `presenter?.present(error)` → "网络错误，请重试" alert overlay 弹在 room_B 之上
5. 用户视角：刚加入新房间又看到不相关的网络错误，体验割裂

### 根因（Root cause）

- `DefaultLeaveRoomUseCase` 已经 guard 200 / 6004 stale-response（r2 P2 fix）：捕获入口时 `targetRoomId = currentRoomId`,
  await 返回后 `if appState.currentRoomId == targetRoomId` 才 `setCurrentRoomId(nil)`
- 但 r2 fix **只处理成功路径**（writeback 到 appState 的两个分支）；**thrown-error 直通 throw 给 caller**
- caller `onLeaveTap` catch 块没有 capture target / 没有 guard → 任何迟到的 leave 失败都弹 errorPresenter
- 错误 model：把 "stale response 不应改 appState" 误解为"只要不 mutate appState 就够了"；
  但 errorPresenter 也是**全局可见 UI mutation**（present alert / retry）—— 同样需要 stale guard

### 修复（Fix）

`RealRoomViewModel.onLeaveTap` 内：

- **修改前**：起 Task 前不 capture target；catch 块直接 `presenter?.present(error)`
- **修改后**：
  ```swift
  let target = self.appState?.currentRoomId
  Task { @MainActor [weak self] in
      guard let self else { return }
      do {
          try await useCase.execute()
      } catch {
          let liveRoomId = self.appState?.currentRoomId
          guard liveRoomId == target else {
              os_log(.info, "stale leave error (target=%{public}@, current=%{public}@); skip errorPresenter", ...)
              return
          }
          presenter?.present(error)
      }
  }
  ```

### 预防规则（Rule for future Claude）⚡

> **一句话**：在 ViewModel 内用 Task wrap async UseCase / repo / network call 时，凡是 catch 块要做**全局
> 可见的 UI mutation**（errorPresenter / setCurrentX / 跨 ViewModel state 写入），都**必须**先 capture
> 入口时刻的 identity（roomId / requestId / generation counter），catch 时 guard "identity 没变"才
> mutate；不匹配 silent skip + log info。
>
> **展开**：
> - "stale guard" 不只是"不写 appState"，也包括"不弹 alert / toast / retry"。任何用户能感知的 UI mutation 都要
>   gate。
> - capture 点：在起 Task **前**读 identity（即 caller MainActor context 里的现值），不在 catch 块内重新读
>   （catch 时已是新状态）。
> - 对比 r2 fix `DefaultLeaveRoomUseCase` 内：成功路径写 appState 已 guard；本 r5 fix 补 caller 层 thrown-error
>   guard，两者**同语义、不同位置**（use case 处理 server 端 race；caller 处理 caller-side state shift）。
> - **反例**：catch block 写 `presenter?.present(error)` 直接，不 guard → r5 重复犯的坑；或者只在 use case 内 guard
>   成功路径却忘了 thrown-error 路径也会跨界 mutate UI。
> - **核心约束**："caller 起 Task 跨 await 边界的任何 mutation 都要 stale-guard"。无 capture identity 的
>   `Task { catch presenter.present }` pattern 在 ViewModel 内**禁止**直接使用。

## Lesson 2: useCase nil 路径必须保留 fallback mutate appState —— 不能让 CTA 变 hard no-op

- **Severity**: low (P3)
- **Category**: architecture（UITEST / preview / launch-mode invariant）
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift:142-144`（旧版本）

### 症状（Symptom）

- `RootView` 在 `UITEST_SKIP_GUEST_LOGIN=1` 启动模式下故意**不**注入 `createRoomUseCase` / `joinRoomUseCase` /
  `leaveRoomUseCase`（让无 backend 的 UI tests / previews 也能跑 happy-path 视觉流）
- `onJoinRoomConfirm` / `onLeaveTap` / `RealFriendsViewModel.onJoinFriendTap` 都有 fallback 路径：useCase nil →
  直接 `appState.setCurrentRoomId(<placeholder/value>)` —— UI 仍能切到 RoomView / 返回 idle
- **但 `onCreateTap` 漏了**：useCase nil → 直接 `return` → 点 Create CTA 变成 hard no-op
- 后果：UITEST 路径下 `RoomUITests` 走 create 流的用例 / Preview 里点 Create 都看不到 RoomView 转场

### 根因（Root cause）

- Story 12.7 落地 useCase 接入时模板化了"guard let useCase else return"，但对 4 个 CTA 的处理不一致：
  - join / leave / friend-join：保留了 r1 之前的 "直接写 appState" 老 placeholder 行为作为 fallback
  - create：作者把 useCase nil 视为 "未 wire 就不响应"，忘了 RootView 在 UITEST 路径**故意**不 wire
- 决策时缺少一条 invariant："useCase nil 路径必须保留某种最小 UI feedback"，让 4 路 fallback 一致

### 修复（Fix）

`RealHomeViewModel.onCreateTap` 内：

- **修改前**：`guard let useCase = self.createRoomUseCase else { os_log("no-op"); return }`
- **修改后**：
  ```swift
  guard let useCase = self.createRoomUseCase else {
      os_log(.debug, "fallback: no CreateRoomUseCase wired; write placeholder roomId directly")
      self.localAppState?.setCurrentRoomId("1234567")
      return
  }
  ```
- 与 `onJoinRoomConfirm` fallback 同 `localAppState?.setCurrentRoomId(...)` 入口
- placeholder `"1234567"` 与 `MockHomeViewModel.onCreateTap` 同精神（mock 视觉占位）

### 预防规则（Rule for future Claude）⚡

> **一句话**：当 ViewModel 内有多个 CTA（create / join / leave / 等）都依赖**同一类** injected dependency（如
> useCase 协议）时，"dep == nil 路径"的处理**必须在所有 CTA 间保持一致**：要么全 fallback 到 visual-only
> 占位 mutation，要么全 hard no-op + log。**禁止一致性漂移**（某些有 fallback、某些 hard return）。
>
> **展开**：
> - iOS 项目本仓库的 `UITEST_SKIP_GUEST_LOGIN=1` / Preview / RootView 老 wire 路径是**明确的 launch mode**，
>   `RealXxxViewModel` 必须**所有 CTA** 都在 useCase nil 路径下保留 visual fallback —— 否则视觉测试 / 早期
>   preview 在某些 CTA 上变成黑洞。
> - 决策矩阵 check：写 `guard let useCase else { return }` 时 ASK "join / leave / friend-join 同模块对 nil 怎么处理？"
>   不一致 → 必须 align（要么全部 no-op + log，要么全部 fallback；不可一半一半）。
> - **反例**：`onCreateTap` `guard let else { return }` 但 `onJoinRoomConfirm` `guard let else { localAppState?.setCurrentRoomId(...); return }` —— 漂移导致 r5 P3。
> - 命名提示：fallback path 在 log 里用关键词 `"fallback"`（与现有 `onLeaveTap` / `onJoinRoomConfirm` log 一致），grep
>   `fallback` 即可在 review 阶段验证全部 CTA 一致性。

---

## Meta: r5 两条都是"r1 fix 边界没覆盖完"型

两条都是前几轮 fix 边界没覆盖完整：

- r2 fix `DefaultLeaveRoomUseCase` stale-guard 只覆盖 success / 6004 path，漏了 thrown-error path（caller 层）
- r2 fix `onJoinRoomConfirm` 加 fallback path，但同期改的 `onCreateTap` 不知为何漏了对称 fallback

教训：写"前一轮 fix 的延伸 / pair fix"时，**主动列举对称 path**（success vs throw；create vs join vs leave），逐 path
确认覆盖到。不要相信单 path 推理；review checklist 应包含"找到对称 path"环节。
