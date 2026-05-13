# Story 15.4: 自己状态变化时上报 state-sync（节流 + 房间内才上报）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want App 自动把我猫的状态变化上报给服务端，但不上报得太频繁，且仅在我处于房间时才上报,
So that 房间内其他成员能看到我猫的实时状态，不浪费流量，且 server `pets.current_state` 与本地保持最终一致.

## 故事定位（Epic 15 第 4 条 story；节点 5 iOS 端"自己状态上报闭环"；上承 8.5（StepSyncTriggerService 同精神 sibling service）+ 14.2（server state-sync 接口）+ 15.1 / 15.2（snapshot / WS 入站路径）；下启 15.5 跨房间状态恢复）

- **Epic 15 进度**：15.1（成员猫位渲染 + snapshot 解析，**done**）→ 15.2（pet.state.changed WS 处理，**done**）→ 15.3（状态切换动画，**done**）→ **15.4（本 story，自己状态上报）** → 15.5（跨房间状态恢复）
- **本 story 是 Epic 15 第四条 story**：把 8.4 / 8.5 已落地的"本地 motion → HomeViewModel.petState"信号 → 通过新建的 `PetStateSyncTriggerService` + `SyncPetStateUseCase` + `PetRepository` 链路 → 调 V1 §5.2 `POST /api/v1/pets/current/state-sync`，让 server 把当前用户的 `pets.current_state` 持久化并通过 14.4 落地的 fan-out 路径广播 `pet.state.changed` 给同房间其他成员
- **本 story 是 Epic 15 后续 stories 的关键前置**：
  - **Story 15.5（跨房间状态恢复）**：依赖本 story 的 reactive subscription —— resume / room edge 时把当前 `petState` 与 server 对齐
  - **Story 16.1 验证场景 3（节流）**：验证短时间内连续 .walk → server 只收 1 次（本 story AC2 钦定 5 秒同 state 节流）
  - **Story 16.1 验证场景 4（不在房间不上报）**：验证 `currentRoomId == nil` 时不调 state-sync（本 story AC2 钦定 not-in-room preflight）
  - **Story 16.1 验证场景 6（自己也广播）**：依赖 14.4 的 self-broadcast 路径已落地 + Story 15.2 已能正确处理"接到自己 userId 的 pet.state.changed"
- **节点 5 验收要求（§4.5）**：本 story 落地后 + 15.5 完成后，节点 5 iOS 端 §4.5 全部要点齐 → Epic 16 demo 验收

---

## ⚠️ 实装边界（Attempt 1 教训：必读）

> **本 story 在 attempt 1 被实装过一次，跑了 13 轮 codex review 仍未收敛**（commits 已 reset 丢弃；recovery tag = `epic-loop/15-4-attempt-1-halt` 仅供事后查阅）。**主要复杂度来源**：把 `PetStateSyncTriggerService` 设计成"per-state 节流字典 + coalesce-to-latest + room edge 主动 sync + resume publisher subscribe-replay 区分 + stop 期间 in-flight Task cancel 回滚 + ..."，最终 service 738 行 + 测试 2508 行，每轮 review 都暴露新的 race / lifecycle 边界 case。
>
> **本 story attempt 2 必须从最小可行节流开始，spec 怎么写就怎么实装，不要为"未来可能的 race"提前抗蚀**。下方"实装路径建议"段落明确钦定哪些 attempt 1 的设计**禁止**复刻：

### 禁止复刻的 attempt 1 过度设计（红线，违反即 review reject）

1. **❌ per-state 节流字典 `[MotionState: Date]`** —— attempt 1 r9 引入；spec（epics.md §15.4 行 2440）只说 "同一 state 在 5 秒内不重复上报"，**单一 `(lastSentState, lastSentAt)` 二元组**就够。如果状态从 .walk 切到 .run 再切回 .walk，按"同一 state"判定就是看 `state == lastSentState && now - lastSentAt < 5s`。.walk 到 .run 是不同 state，必发。
2. **❌ coalesce-to-latest in-flight 队列 + pending state** —— attempt 1 r10 / r11 引入；spec **没有**任何"保留中间 state 重放"的要求。fire-and-forget 一次失败就丢失，下次状态变化再发，YAGNI。
3. **❌ Room edge 主动触发 sync（nil → non-nil 边沿自动 publish 当前 state）** —— attempt 1 r12 引入；spec 只说"在房间 + state 变化 → 发；不在房间 → 不发"，**没说**"join 房间瞬间要主动 push 当前 state"。这是 Story 15.5（跨房间状态恢复）的责任范围，**不**是本 story。
4. **❌ Stop 期间回滚 cancelled in-flight 的节流锚点** —— attempt 1 r13 引入；spec 没要求 stop 后还能保证下一次同 state 不被错挡。stop / start cycle 的边界状态由"节流锚点保留"语义自然处理，YAGNI。
5. **❌ Publisher subscribe-replay vs 真实 transition 的区分** —— attempt 1 r3 / r4 / r13 反复修；用 `.dropFirst()` 一刀切（first start + resume 同等处理）即可，不要去识别"这条 emit 是订阅瞬间 replay 还是真实 mutate"。
6. **❌ Service 内部 spawn task 链 / per-state in-flight gate / serialize semantics** —— spec 没要求 in-flight 不重叠（fire-and-forget 是钦定模式）。如果担心同 state 的 in-flight 重叠（5s 窗口内 user 不可能 emit 两次同 state），用 throttle 锚点已天然避免；不需要额外 in-flight gate。

### 允许的最小可行设计（attempt 2 路径）

1. **✅ 单一 `(lastSentState, lastSentAt)` 节流锚点** —— 命中规则：`state == lastSentState && now - lastSentAt < 5s` → 跳过；否则发送（在 commit-to-send 时同步写锚点 —— 这条是 attempt 1 r2 的合理 fix，保留）。
2. **✅ Room guard preflight in service** —— `appState.currentRoomId == nil` → 跳过（不消耗节流窗口；attempt 1 r1 / r2 P2 fix 合理，保留）。
3. **✅ Subscribe `homeViewModel.$petState`，`.dropFirst()` 抹掉订阅瞬间的 replay** —— first start / resume 都用 dropFirst 一致处理。
4. **✅ Fire-and-forget Task** —— sink 收到 emit → spawn 单个 `Task { @MainActor in await useCase.execute(...) }` → 失败 silently 吞掉（与 8.5 StepSyncTriggerService.runSync 同模式）。
5. **✅ start() / stop() 生命周期** —— 由 RootView 的 `.onReadyTask` / `.scenePhase .background` 调，幂等；与 StepSyncTriggerService 同精神（参考 `StepSyncTriggerService.swift` line 100-174 模板）。
6. **✅ AppState.$currentRoomId 不订阅**（YAGNI；spec 没要求"离开房间时清节流"或"join 时主动 sync"；`currentRoomId` 在 sink closure 内**同步读** snapshot 即可）。

> **如果未来 demo 验证（Story 16.1 验证场景 5：snapshot 对齐）发现"join 房间后他人看到自己 stale state"问题，那是 Story 15.5 的责任**（跨房间状态恢复 spec 钦定"重连后自动通过 Story 15.4 上报"，可能演化为 Story 15.5 调本 service 暴露的 `triggerManual()` 方法 —— 但本 story attempt 2 **不**预留 triggerManual，YAGNI；Story 15.5 实装时若需要再加）。

---

## Acceptance Criteria

> **AC 编号体系**：
> - AC1 = V1 §5.2 wire DTO 模型 + Endpoint + PetRepository 三件套（最小契约层；attempt 1 该层简洁干净，本 story 直接复用同设计）
> - AC2 = `SyncPetStateUseCase`：roomId guard + 调 repo + 不写 AppState（与 SyncStepsUseCase 同模式但不写 state）
> - AC3 = `PetStateSyncTriggerService`：subscribe `homeViewModel.$petState` → 5s 节流 + roomId preflight → fire-and-forget spawn UseCase Task
> - AC4 = `RootView` 集成：`@State` 持有 service + `onReadyTask` / `onLeaveReady` / `scenePhase` 生命周期接入（与 StepSyncTriggerService 同模式）+ `AppContainer` 工厂方法
> - AC5 = 单元测试覆盖（≥5 case + repo / endpoint / UseCase 各自小测试集）
> - AC6 = Build verify + ios-simulator MCP 实跑录屏验证（CLAUDE.md "iOS UI 验证（必跑）"）
> - AC7 = Deliverable 清单

---

### AC1 — V1 §5.2 wire DTO + Endpoint + PetRepository 三件套（最小契约层）

**给定**：

- `docs/宠物互动App_V1接口设计.md` §5.2（自 2026-05-12 Story 14.1 起冻结）：
  - Path: `POST /api/v1/pets/current/state-sync`
  - Request body: `{state: int}`，state ∈ {1, 2, 3}
  - Response body: `{state: int}`（server-acknowledged ack 信号；**禁止**用作权威信号源 —— 详见 V1 §5.2 line 610-613 + lesson `2026-05-12-state-sync-err-binary-and-placeholder-whitelist-self-http-ack-14-1-r6.md`）
  - **不**接受 `idempotencyKey` header（V1 §5.2 line 500：state-sync 不消耗资产）
  - **不**带 `petId` 字段（V1 §5.2 line 605：server 自查默认 pet）
- 既有同模式参考：`iphone/PetApp/Features/Home/Repositories/StepRepository.swift`（DefaultStepRepository: struct + apiClient.request 转发）+ `iphone/PetApp/Features/Home/Services/StepsEndpoints.swift`（enum + 静态工厂方法）

**预期文件**（attempt 1 该层无问题，照抄 attempt 1 设计）：

1. **`iphone/PetApp/Features/Home/Models/PetStateSyncRequest.swift`**（新建）：
   ```swift
   public struct PetStateSyncRequest: Encodable, Sendable, Equatable {
       public let state: Int
       public init(state: Int) { self.state = state }
   }
   ```
2. **`iphone/PetApp/Features/Home/Models/PetStateSyncResponse.swift`**（新建）：
   ```swift
   public struct PetStateSyncResponse: Decodable, Sendable, Equatable {
       public let state: Int
       public init(state: Int) { self.state = state }
   }
   ```
3. **`iphone/PetApp/Features/Home/Services/PetStateEndpoints.swift`**（新建）：
   ```swift
   public enum PetStateEndpoints {
       public static func sync(_ request: PetStateSyncRequest) -> Endpoint {
           Endpoint(
               path: "/api/v1/pets/current/state-sync",
               method: .post,
               body: AnyEncodable(request),
               requiresAuth: true
           )
       }
   }
   ```
   注：与既有 `StepsEndpoints.swift` 同位置 / 同模式（`Features/Home/Services/` 下，命名为 `*Endpoints.swift`）。
4. **`iphone/PetApp/Features/Home/Repositories/PetRepository.swift`**（新建）：
   ```swift
   public protocol PetRepositoryProtocol: Sendable {
       func syncPetState(_ request: PetStateSyncRequest) async throws -> PetStateSyncResponse
   }

   public struct DefaultPetRepository: PetRepositoryProtocol {
       private let apiClient: APIClientProtocol
       public init(apiClient: APIClientProtocol) { self.apiClient = apiClient }
       public func syncPetState(_ request: PetStateSyncRequest) async throws -> PetStateSyncResponse {
           try await apiClient.request(PetStateEndpoints.sync(request))
       }
   }
   ```

**红线**：

- **不**在 PetRepository 内实装 retry / 节流 / roomId guard —— 这些是 UseCase / TriggerService 的责任（与 StepRepository 一致：repo 仅做 endpoint 转发）
- **不**把 `PetStateEndpoints` 放到 `Features/Home/Endpoints/` 目录（既有目录结构是 Endpoints 与 UseCase / Service 同放在 `Services/` 下，命名后缀 `Endpoints.swift`；与 `StepsEndpoints.swift` 同位置）
- **不**用 `class` 实装 `DefaultPetRepository` —— `struct` value type 即可（与 DefaultStepRepository 一致）
- **不**写 `idempotencyKey` 字段或 header（V1 §5.2 line 500 钦定）

**对应 Tasks**: Task 1.1, 1.2, 1.3, 1.4

---

### AC2 — `SyncPetStateUseCase`：roomId guard + 调 repo + 不写 AppState

**给定**：

- 既有同模式参考：`iphone/PetApp/Features/Home/UseCases/SyncStepsUseCase.swift`（DefaultSyncStepsUseCase: struct + 注入 repository / appState）
- AppState 访问：`appState.currentRoomId` 是 `@MainActor isolated @Published String?`（`iphone/PetApp/App/AppState.swift:49`）
- V1 §5.2 line 547 钦定：state-sync 接口对"用户不在房间"是合法场景（仅不广播 WS，HTTP 仍 200 OK + code = 0）—— 但本 story 在 **client 端** preflight 拦截 not-in-room（节省流量；epics.md §15.4 行 2437-2438 钦定）

**预期文件**：

**`iphone/PetApp/Features/Home/UseCases/SyncPetStateUseCase.swift`**（新建）：

```swift
public protocol SyncPetStateUseCaseProtocol: Sendable {
    func execute(state: MotionState) async throws -> SyncPetStateUseCaseOutcome
}

public enum SyncPetStateUseCaseOutcome: Equatable, Sendable {
    case success(echoedState: Int)
    case skippedNotInRoom
}

public struct DefaultSyncPetStateUseCase: SyncPetStateUseCaseProtocol {
    private let repository: PetRepositoryProtocol
    private let appState: AppState
    public init(repository: PetRepositoryProtocol, appState: AppState) {
        self.repository = repository
        self.appState = appState
    }
    public func execute(state: MotionState) async throws -> SyncPetStateUseCaseOutcome {
        let roomId: String? = await MainActor.run { appState.currentRoomId }
        guard roomId != nil else {
            return .skippedNotInRoom
        }
        let request = PetStateSyncRequest(state: state.wireValue)
        let response = try await repository.syncPetState(request)
        return .success(echoedState: response.state)
    }
}
```

**预期行为**：

1. **roomId guard 在 UseCase 入口**（防御性 + 兼容其他 caller —— TriggerService 也会 preflight 一次，两层独立；与 StepSyncTriggerService 同精神）
2. **不写 AppState**（与 SyncStepsUseCase 不同 —— SyncStepsUseCase 写 `currentStepAccount`；本 UseCase 的 HTTP ack 是 server-only 信号，**禁止**驱动 UI；UI 由 motionProvider → HomeViewModel.petState 驱动；self-entry 房间页猫由 Story 15.2 落地的 self-broadcast WS 路径处理）
3. **不接 ErrorPresenter**（背景同步失败不弹 toast；与 epics.md §15.4 行 2441 钦定）
4. **HTTP 错误原样透传**（caller 在 service 层吞 / log）

**红线**：

- **不**用 HTTP `data.state` 回显信号驱动 HomeViewModel 或 RoomViewModel（V1 §5.2 line 610-613 + lesson `2026-05-12-state-sync-err-binary-and-placeholder-whitelist-self-http-ack-14-1-r6.md` 钦定 "HTTP ack 仅作 (a) state-sync 调用成功标志 (b) self-broadcast 兜底信号之一"）
- **不**做 retry / 指数退避（YAGNI；TriggerService 层依靠"下次状态变化"自然兜底）
- **不**在 UseCase 内部读 ViewModel（option A：caller 注入 state 值；与 SyncStepsUseCase 一致）

**对应 Tasks**: Task 2.1, 2.2

---

### AC3 — `PetStateSyncTriggerService`：subscribe `homeViewModel.$petState` → 5s 节流 + roomId preflight → fire-and-forget Task

**给定**：

- 既有 sibling 参考：`iphone/PetApp/Features/Home/Services/StepSyncTriggerService.swift`（249 行；4 触发器 + in-flight gate + 定时器循环）
- epics.md §15.4 钦定（行 2436-2447）：
  - 触发：HomeViewModel.petState 变化
  - roomId guard：不在房间 → 不上报
  - 节流：同一 state 5 秒内不重复上报
  - fire-and-forget：上报失败不重试 / 不阻塞 UI
- AppState.currentRoomId 在 sink closure 内同步读（@MainActor isolated；service 也 @MainActor → 直接 `appState.currentRoomId`）

**预期文件**：

**`iphone/PetApp/Features/Home/Services/PetStateSyncTriggerService.swift`**（新建；**目标 ≤ 200 行**包含注释；与 StepSyncTriggerService 量级相当）：

#### 实装契约

```swift
@MainActor
public final class PetStateSyncTriggerService {

    private let syncPetStateUseCase: SyncPetStateUseCaseProtocol
    private weak var homeViewModel: HomeViewModel?
    private weak var appState: AppState?

    // 5 秒节流锚点（单一二元组，**不**用 [MotionState: Date] 字典）.
    // commit-to-send 时同步写（attempt 1 r2 fix 保留）.
    private var lastSentState: MotionState?
    private var lastSentAt: Date?
    private static let throttleWindow: TimeInterval = 5.0

    // Combine subscription 引用；start() 建，stop() / deinit 清.
    private var subscription: AnyCancellable?

    public init(
        syncPetStateUseCase: SyncPetStateUseCaseProtocol,
        homeViewModel: HomeViewModel,
        appState: AppState
    ) {
        self.syncPetStateUseCase = syncPetStateUseCase
        self.homeViewModel = homeViewModel
        self.appState = appState
    }

    /// 启动 subscription（幂等：subscription != nil 时短路）.
    /// 与 StepSyncTriggerService.start 同精神：first start / resume 都走同一路径.
    public func start() {
        guard subscription == nil, let homeViewModel else { return }
        subscription = homeViewModel.$petState
            .dropFirst() // 抹掉 @Published 订阅瞬间的 currentValue replay
            .sink { [weak self] newState in
                self?.handlePetStateChange(newState)
            }
    }

    /// 停止 subscription（cancel 当前；不动 lastSentState/lastSentAt 节流锚点 —— 让 resume 后同 state 5s 内仍受节流）.
    /// **不**回滚 in-flight Task 的节流锚点（attempt 1 r13 复杂度，本 story 不要）.
    public func stop() {
        subscription?.cancel()
        subscription = nil
    }

    /// sink 回调入口：preflight throttle + roomId guard → fire-and-forget spawn.
    private func handlePetStateChange(_ newState: MotionState) {
        // Step 1: 节流命中（同 state + 5s 内）→ return（不消耗窗口）.
        if let lastState = lastSentState, let lastAt = lastSentAt,
           lastState == newState, Date().timeIntervalSince(lastAt) < Self.throttleWindow {
            return
        }
        // Step 2: roomId preflight（同步读 @MainActor isolated currentRoomId）→ nil → return（不消耗节流窗口）.
        guard let appState, appState.currentRoomId != nil else {
            return
        }
        // Step 3: commit-to-send：同步写节流锚点（attempt 1 r2 fix 保留 —— 防 await 期间同 state 再 emit
        //         看到 nil 锚点重复 spawn）.
        lastSentState = newState
        lastSentAt = Date()
        // Step 4: fire-and-forget spawn UseCase Task（失败 silently 吞）.
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                _ = try await self.syncPetStateUseCase.execute(state: newState)
            } catch {
                // 失败不阻塞 UI（与 StepSyncTriggerService.runSync catch 同模式）.
                // 节点 5 阶段不接 logger framework；下次状态变化再试.
            }
        }
    }

    deinit {
        subscription?.cancel()
    }
}
```

**预期行为**：

1. **subscription 由 `homeViewModel.$petState.dropFirst().sink` 建立**；first start / resume 都用 `.dropFirst()` 抹掉 @Published 订阅瞬间的 currentValue replay（避免无谓的 5s 内同 state 触发；attempt 1 r3 / r4 / r13 反复修这个边界，本 story 一刀切 dropFirst）
2. **节流锚点是单一 `(lastSentState, lastSentAt)` 二元组**（**不**用字典 / 不区分 per-state；attempt 1 r9 引入字典是过度设计，spec 没要求）
3. **handlePetStateChange 内顺序**：throttle preflight → roomId preflight → commit-to-send 写锚点 → spawn Task → Task 内 await UseCase
4. **commit-to-send 同步写**（attempt 1 r2 P1 fix 保留）：先写 lastSentState/lastSentAt 再 spawn Task → 防 Task await 期间同 state 再 emit 看到 nil 锚点重复 spawn
5. **roomId guard 不消耗节流窗口**（attempt 1 r1 P2 fix 保留）：not-in-room 短路时 lastSentState/lastSentAt 不写 → 用户回到房间后第一次合法 sync 不被错挡
6. **stop() 不动节流锚点 + 不回滚 in-flight Task 节流**（attempt 1 r13 不复刻）—— resume 后同 state 5s 内仍受节流是合理行为（用户切到 background 又回来，state 没变就不必重发）

**红线**：

- **不**实装 per-state `[MotionState: Date]` 节流字典（attempt 1 r9 红线，**单一二元组够用**）
- **不**实装 coalesce-to-latest pending state（attempt 1 r10 / r11 红线，**fire-and-forget 一次失败就丢**）
- **不**订阅 `appState.$currentRoomId`（attempt 1 r6 / r7 / r12 红线，**roomId 在 sink 内同步读 snapshot 即可**）
- **不**在 service 内实装 in-flight gate / serialize / Task chain（attempt 1 r9 / r10 / r11 / r13 红线，**spawn-and-forget 即可**）
- **不**在 stop() 内回滚 cancelled in-flight Task 的节流锚点（attempt 1 r13 红线）
- **不**在 start() 内做 stale room reconciliation（清节流字段；attempt 1 r4 / r7 红线）
- **不**实装 `triggerManual()` 公开方法（attempt 1 没引入这个；Story 15.5 若需要再加）
- **不**为 service 添加 publisher subscribe-replay 检测逻辑（attempt 1 r3 / r4 / r13 红线，**dropFirst() 一刀切**）

**对应 Tasks**: Task 3.1, 3.2

---

### AC4 — `RootView` 集成 + `AppContainer` 工厂方法

**给定**：

- 既有 sibling 参考：`iphone/PetApp/App/RootView.swift` line 110-115（@State stepSyncTriggerService）+ line 200-220（onReadyTask + onLeaveReady）+ line 380-400（ensureStepSyncWired）+ line 410-440（scenePhase background/active）
- 既有工厂参考：`iphone/PetApp/App/AppContainer.swift` `makeStepSyncTriggerService(...)`

**预期修改**：

#### `iphone/PetApp/App/AppContainer.swift`（修改 —— 加 Story 15.4 factory 段）

```swift
// MARK: - Story 15.4: Pet State Sync 链路 factory

public func makePetRepository() -> PetRepositoryProtocol {
    DefaultPetRepository(apiClient: apiClient)
}

public func makeSyncPetStateUseCase(appState: AppState) -> SyncPetStateUseCaseProtocol {
    DefaultSyncPetStateUseCase(
        repository: makePetRepository(),
        appState: appState
    )
}

public func makePetStateSyncTriggerService(
    appState: AppState,
    homeViewModel: HomeViewModel
) -> PetStateSyncTriggerService {
    PetStateSyncTriggerService(
        syncPetStateUseCase: makeSyncPetStateUseCase(appState: appState),
        homeViewModel: homeViewModel,
        appState: appState
    )
}
```

#### `iphone/PetApp/App/RootView.swift`（修改 —— 5 处接入点）

1. **加 `@State` 字段**（在 `stepSyncTriggerService` 字段下方加同模式字段）：
   ```swift
   /// Story 15.4 AC4: PetStateSyncTriggerService（订阅 HomeViewModel.$petState → 5s 节流 → 触发 SyncPetStateUseCase）.
   /// 由 RootView @State 持有，避免 body 重建时 service 重启（与 stepSyncTriggerService 同模式）.
   @State private var petStateSyncTriggerService: PetStateSyncTriggerService?
   ```

2. **`onReadyTask` 内追加 `petStateSyncTriggerService?.start()`**（在 `stepSyncTriggerService?.start()` 之后追加；与 8.5 同时机：launchStateMachine .ready 后 + SessionStore.token 已存在）

3. **`onLeaveReady` 内追加 `petStateSyncTriggerService?.stop()`**（在 `stepSyncTriggerService?.stop()` 之后追加；离开 .ready 时同停）

4. **`ensurePetStateSyncWired()` 私有方法 + 在 ensureStepSyncWired 之后调**：
   ```swift
   private func ensurePetStateSyncWired() {
       guard petStateSyncTriggerService == nil else { return }
       petStateSyncTriggerService = container.makePetStateSyncTriggerService(
           appState: appState,
           homeViewModel: homeViewModel
       )
   }
   ```
   并在 `ensureStepSyncWired()` 调用之后追加 `ensurePetStateSyncWired()`

5. **`scenePhase` 监听内追加 background → stop / active → start**（在 stepSync 同 modifier 内追加同模式调用，复用既有 `if launchStateMachine?.state == .ready` guard）：
   ```swift
   // background → stop
   if newPhase == .background {
       stepSyncTriggerService?.stop()
       petStateSyncTriggerService?.stop()  // ← Story 15.4
   }
   // active → start (复用既有 guard 内)
   if launchStateMachine?.state == .ready {
       stepSyncTriggerService?.start()
       petStateSyncTriggerService?.start()  // ← Story 15.4
   }
   ```

**红线**：

- **不**新建 `@StateObject` 持有 service（service 是 @MainActor final class 但**不**是 ObservableObject；用 `@State` 持有引用即可，与 stepSyncTriggerService 同模式）
- **不**在 `body` 内构造 service（每次 body 重建会丢 subscription；用 `ensure*Wired()` lazy init + nil guard）
- **不**改 `AppContainer` 的字段（service 不进 container 字段；与 stepSyncTriggerService 同模式：每次调 factory 返回新 instance；持有由 caller 用 @State 负责）
- **不**让 `petStateSyncTriggerService.start()` 在 cold-start 期间调用 —— 必须在 `launchStateMachine .ready` 之后（与 stepSync 同 guard）
- **不**改 `AppState` 字段（无新 @Published；`currentRoomId` 既有字段已够用）
- **不**改 `HomeViewModel` 接口（subscribe `$petState` 公开 publisher 已就绪；attempt 1 也未改 HomeViewModel）

**对应 Tasks**: Task 4.1, 4.2

---

### AC5 — 单元测试覆盖（≥5 case + 各层小测试集）

**测试文件清单**：

1. **`iphone/PetAppTests/Features/Home/Services/PetStateEndpointsTests.swift`**（新建，≥1 case）：
   - case#1: `PetStateEndpoints.sync(.init(state: 2))` → `endpoint.path == "/api/v1/pets/current/state-sync" && method == .post && requiresAuth == true`

2. **`iphone/PetAppTests/Features/Home/Repositories/MockPetRepository.swift`**（新建测试 helper —— 与 `MockStepRepository` 同模式）：
   - protocol conformance + recorded calls + scripted result/error

3. **`iphone/PetAppTests/Features/Home/UseCases/SyncPetStateUseCaseTests.swift`**（新建，≥4 case）：
   - case#A happy: `appState.currentRoomId = "X"` → execute(.walk) → repo 收到 `state: 2` request → 返 `.success(echoedState: 2)`
   - case#B edge: `appState.currentRoomId = nil` → execute(.walk) → repo **未被调用** → 返 `.skippedNotInRoom`
   - case#C edge: repo throw `APIError.network(...)` → execute throw 透传（测 try-await throws）
   - case#D edge: repo 返 `state != request.state`（如发 .walk 收 .run）→ 仍返 `.success(echoedState: ...)`（不做一致性断言；HTTP ack 仅作信号）

4. **`iphone/PetAppTests/Features/Home/Services/PetStateSyncTriggerServiceTests.swift`**（新建，≥5 case，**目标 ≤ 350 行**包含注释；attempt 1 该测试文件爆到 2508 行是过度测试反模式，**不要复刻**）：
   - case#E happy "在房间 + state 变化 .rest → .walk"：start service → mutate `homeViewModel.petState = .walk` → await Task.yield() N 次 → 断言 mockUseCase.executeCalls.count == 1 + 入参 state == .walk
   - case#F edge "不在房间 → state 变化也不调"：`appState.currentRoomId = nil` → mutate petState → 断言 mockUseCase.executeCalls.isEmpty
   - case#G edge "5 秒内重复 set 同一 state → 只调 1 次"：mutate .walk → 等几十 ms → mutate .walk → 断言 executeCalls.count == 1（**不**用真 sleep 5s；用 fake clock 或直接验证 5s 内的两次连续 mutate 行为；可用 `dateProvider` 注入或直接验证 throttle 锚点写入逻辑）
   - case#H edge "5 秒后重新 set 同一 state → 又调 1 次"：mutate .walk → **手动 mutate service 内 lastSentAt 退到 6s 前**（用 internal test seam 或 dateProvider 注入）→ mutate .walk → 断言 executeCalls.count == 2
   - case#I edge "API 失败 → 不抛错，下次状态变化照常上报"：mockUseCase scripted to throw → mutate .walk → 断言 service 不 crash + executeCalls.count == 1；再 mutate .run → 断言 executeCalls.count == 2

**测试基础设施约束**（与 8.5 / 12.x / 15.1-15.3 一致）：

- **XCTest only**（ADR-0002 §3.1 钦定 / 零外部依赖）
- **`@MainActor` 标注测试 class**（service 是 @MainActor）
- **不引入** ViewInspector / SnapshotTesting / Combine pipeline 测试库
- **time-related tests**: 优先在 service 暴露 `internal var nowProvider: () -> Date = { Date() }` test seam（**仅** internal access，protocol 不暴露）让测试可注入 fake date；或用直接修改 `lastSentAt` 的 test seam（@testable import + internal access）。**禁止** `Task.sleep(5_000_000_000)` 真等 5 秒（attempt 1 测试反模式）
- **测试不需要覆盖**："publisher subscribe-replay vs 真实 transition 的区分"、"stop / start cycle 期间 in-flight Task 行为"、"room edge nil → non-nil / non-nil → nil 边沿主动行为"、"per-state 节流字典 cross-state 干扰"等 attempt 1 引入的设计点 —— 这些不在本 story spec 范围

**红线**：

- **不**在测试中使用真实 `Task.sleep(seconds:)` 等 5 秒（attempt 1 测试时长爆炸根因）
- **不**测试 attempt 1 引入但本 story spec 范围外的边界（per-state 字典 / coalesce / room edge / publisher replay 区分 / stop in-flight rollback）
- **不**测试 SwiftUI render-tree 行为（service 是纯 ViewModel-adjacent，不接 view）
- **不**用 ViewInspector / SnapshotTesting 任何外部库（ADR-0002 §3.1 钦定）

**对应 Tasks**: Task 5.1, 5.2, 5.3, 5.4

---

### AC6 — Build verify + ios-simulator MCP 实跑录屏验证

**必须通过**：

```bash
bash iphone/scripts/build.sh --test
```

- xcodebuild 编译通过
- 所有单测通过（含本 story 新增 ≥5 case + 既有全部 case 不破）
- 既有 UITest 不破（本 story 不新增 UITest case —— 服务是 ViewModel-adjacent 后台逻辑，无 UI 表面；E2E 视觉验证留给 Story 16.1 / Story 16.2 的节点 5 demo）

**ios-simulator MCP 验证**（CLAUDE.md "iOS UI 验证（必跑）" + lesson `2026-05-12-swiftui-frame-clipped-does-not-scale-15-1-r1.md` 钦定模板）：

```
1. bash iphone/scripts/build.sh                          # build → DerivedData/.../PetApp.app
2. install_app(app_path: iphone/build/DerivedData/Build/Products/Debug-iphonesimulator/PetApp.app)
3. launch_app(bundle_id: "com.zhuming.pet.app", terminate_running: true,
              environment: { "UITEST_FORCE_IN_ROOM": "1", "UITEST_ROOM_THREE_MEMBERS": "1" })
4. ui_view + ui_describe_all                              # 基线 UI 状态
5. 通过 lldb / 测试钩子 / DevTools mock motionProvider 触发 HomeViewModel.petState mutate（.rest → .walk）
6. 观察 server 端 log（manual 联调时）/ 或观察 Network Inspector：
   - 单次切换：应触发 1 次 POST /api/v1/pets/current/state-sync
   - 5s 内同 state 重复：应只触发 1 次（节流生效）
   - currentRoomId == nil 时切换：应不触发（preflight 生效）
7. （可选）截图保存到 _bmad-output/implementation-artifacts/15-4-pet-state-sync-recording/<timestamp>/
```

**注**：本 story 没有强 UI 视觉变化（service 是后台 HTTP 触发器），MCP 实跑主要验证：
- (i) build pass + app launch 不 crash
- (ii) 既有 UI 表现不被破坏（HomeView petState 显示 + RoomScaffoldView 成员行渲染不破）
- (iii) 后台 sync trigger 生效（通过 server log / Network Inspector / lldb 间接验证；视情况存截图作为 dev artifact）

**lesson 必读 / 必遵守**：

- `docs/lessons/2026-05-12-fire-and-forget-boundary-must-include-prerequisite-io-14-4-r1.md`（fire-and-forget 边界必须包住 "决定是否 broadcast 的前置 IO" —— 本 story Task 内的 throttle/roomId preflight 在 Step 1-3 已同步完成，Task 内只跑 UseCase + catch；与 lesson 精神一致）
- `docs/lessons/2026-05-12-state-sync-err-binary-and-placeholder-whitelist-self-http-ack-14-1-r6.md`（HTTP ack `data.state` **禁止**作权威信号源；本 story UseCase 不消费 echoedState 字段，仅作测试断言用 + log）
- `docs/lessons/2026-05-12-state-sync-pet-less-noop-consistent-with-home-room-snapshot-14-1-r7.md`（pet-less 账号 server 走 noop 路径返回 200 OK + state 回显；本 story client 不需要为 pet-less 做 special-case suppress —— UseCase 收到 .success 即视为成功）
- `docs/lessons/2026-05-04-manual-trigger-must-await-in-flight.md`（StepSyncTriggerService 8.5 r3 修；本 story **不**实装 triggerManual —— 但保留 lesson 精神：若未来 Story 15.5 需要触发 sync，**禁止** fire-and-forget gate 短路返回 stale state）
- `docs/lessons/2026-05-04-scenephase-idempotent-start-no-duplicate-trigger.md`（StepSyncTriggerService 8.5 r1 修；本 story `start()` 用 `subscription == nil` guard 实现幂等，与同精神）
- `docs/lessons/2026-05-04-launch-state-leave-ready-must-stop-feature-services.md`（离开 .ready 时必须 stop feature services；本 story `onLeaveReady` 调 `petStateSyncTriggerService?.stop()` 与同精神）
- `docs/lessons/2026-04-26-stateobject-debug-instance-aliasing.md`（@State vs @StateObject 区别；service 用 @State 持有引用即可，**不**用 @StateObject）
- `docs/lessons/2026-05-12-fire-and-forget-boundary-must-include-prerequisite-io-14-4-r1.md`（重读：本 story 的 throttle / roomId guard 是 preflight 前置 IO，必须在 commit-to-send 之前做完，不能放进 Task await 之后）
- `docs/lessons/2026-04-25-swift-explicit-import-combine.md`（Combine 必须显式 import；本 story PetStateSyncTriggerService 必须 `import Combine`，否则 `.dropFirst()` / `AnyCancellable` 不可见）

**对应 Tasks**: Task 6.1

---

### AC7 — Deliverable 清单

**新建文件**：

- `iphone/PetApp/Features/Home/Models/PetStateSyncRequest.swift`（AC1）
- `iphone/PetApp/Features/Home/Models/PetStateSyncResponse.swift`（AC1）
- `iphone/PetApp/Features/Home/Services/PetStateEndpoints.swift`（AC1）
- `iphone/PetApp/Features/Home/Repositories/PetRepository.swift`（AC1）
- `iphone/PetApp/Features/Home/UseCases/SyncPetStateUseCase.swift`（AC2）
- `iphone/PetApp/Features/Home/Services/PetStateSyncTriggerService.swift`（AC3；**目标 ≤ 200 行**）
- `iphone/PetAppTests/Features/Home/Services/PetStateEndpointsTests.swift`（AC5）
- `iphone/PetAppTests/Features/Home/Repositories/MockPetRepository.swift`（AC5 helper）
- `iphone/PetAppTests/Features/Home/UseCases/SyncPetStateUseCaseTests.swift`（AC5）
- `iphone/PetAppTests/Features/Home/Services/PetStateSyncTriggerServiceTests.swift`（AC5；**目标 ≤ 350 行**）

**修改文件**：

- `iphone/PetApp/App/AppContainer.swift`（AC4：3 个 factory 方法）
- `iphone/PetApp/App/RootView.swift`（AC4：5 处接入点）
- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen 自动同步新文件 fileRef + buildFile + Sources phase）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（15-4 状态 ready-for-dev → in-progress → review）

**不需修改文件**：

- `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`（`$petState` publisher 既有；service 订阅即可）
- `iphone/PetApp/App/AppState.swift`（`currentRoomId` 既有 @Published；service 同步读 snapshot 即可）
- `iphone/PetApp/Features/Room/ViewModels/*`（本 story 不动 RoomViewModel；self-broadcast WS 由 Story 15.2 落地路径处理）
- `iphone/PetApp/Core/Networking/*`（不动 APIClient / WebSocketClient）

**对应 Tasks**: Task 6.2

---

## Tasks / Subtasks

- [x] **Task 1.1** — `PetStateSyncRequest.swift` 新建（AC1）
  - [x] 1.1.1 `Encodable + Sendable + Equatable` struct，单字段 `state: Int`
- [x] **Task 1.2** — `PetStateSyncResponse.swift` 新建（AC1）
  - [x] 1.2.1 `Decodable + Sendable + Equatable` struct，单字段 `state: Int`
- [x] **Task 1.3** — `PetStateEndpoints.swift` 新建（AC1）
  - [x] 1.3.1 enum + static func `sync(_:)` 工厂方法，path `/api/v1/pets/current/state-sync`，method .post，body AnyEncodable，requiresAuth true
- [x] **Task 1.4** — `PetRepository.swift` 新建（AC1）
  - [x] 1.4.1 `PetRepositoryProtocol: Sendable` + `DefaultPetRepository: struct` + `apiClient.request(PetStateEndpoints.sync(...))` 转发
- [x] **Task 2.1** — `SyncPetStateUseCase.swift` 新建（AC2）
  - [x] 2.1.1 `SyncPetStateUseCaseProtocol` + `SyncPetStateUseCaseOutcome` enum + `DefaultSyncPetStateUseCase: struct`
  - [x] 2.1.2 execute() 内：roomId guard（`await MainActor.run { appState.currentRoomId }`）→ 调 repo → 返 outcome
- [x] **Task 2.2** — UseCase 不写 AppState 验证（AC2 红线）
  - [x] 2.2.1 严格审视：execute() 内**不**调 `appState.apply*` / `appState.set*` 任何 mutation 方法（只读 currentRoomId snapshot）
- [x] **Task 3.1** — `PetStateSyncTriggerService.swift` 新建（AC3）
  - [x] 3.1.1 `@MainActor public final class`，注入 `syncPetStateUseCase / homeViewModel(weak) / appState(weak)`
  - [x] 3.1.2 `lastSentState/lastSentAt` 单一二元组节流锚点（**不**用字典）+ `throttleWindow = 5.0` static const
  - [x] 3.1.3 `start()` 幂等（subscription == nil guard）+ `homeViewModel.$petState.dropFirst().sink { ... }` 建 subscription
  - [x] 3.1.4 `stop()` cancel + nil 化 subscription（**不**清节流锚点；**不**回滚 in-flight Task）
  - [x] 3.1.5 `handlePetStateChange(_:)` 内 4 步：throttle preflight → roomId preflight → commit-to-send 同步写锚点 → fire-and-forget Task spawn UseCase
  - [x] 3.1.6 `deinit` cancel subscription（防泄漏；与 StepSyncTriggerService 同模式）
- [x] **Task 3.2** — Service 红线巡检（AC3 红线）
  - [x] 3.2.1 grep / 自查：service 文件**不**含 per-state `[MotionState: Date]` 字典 / `pendingState` / `inFlightTask` / `subscribeRoomIdForThrottleReset` / `triggerManual` / `lastKnownRoomId` 任何 attempt 1 复杂度符号
  - [x] 3.2.2 文件总行数（含注释）167 行 ≤ 200 预算 ✓（与 sibling StepSyncTriggerService 249 行同精神 / 体量更精简）
- [x] **Task 4.1** — `AppContainer.swift` 加 factory 段（AC4）
  - [x] 4.1.1 `makePetRepository()` / `makeSyncPetStateUseCase(appState:)` / `makePetStateSyncTriggerService(appState:homeViewModel:)` 三个 factory 方法
- [x] **Task 4.2** — `RootView.swift` 5 处接入（AC4）
  - [x] 4.2.1 加 `@State private var petStateSyncTriggerService: PetStateSyncTriggerService?` 字段
  - [x] 4.2.2 `onReadyTask` 内追加 `petStateSyncTriggerService?.start()`
  - [x] 4.2.3 `onLeaveReady` 内追加 `petStateSyncTriggerService?.stop()`
  - [x] 4.2.4 加 `ensurePetStateSyncWired()` 私有方法 + 在 `ensureStepSyncWired()` 之后追加调用
  - [x] 4.2.5 `scenePhase` 监听内追加 background → stop / active → start（复用既有 ready guard）
- [x] **Task 5.1** — `PetStateEndpointsTests.swift` 新建（AC5）
  - [x] 5.1.1 case#1: endpoint path / method / requiresAuth 字段断言
- [x] **Task 5.2** — `MockPetRepository.swift` 新建测试 helper（AC5）
  - [x] 5.2.1 `PetRepositoryProtocol` conformance + `private(set) var invocations: [PetStateSyncRequest] = []` + scripted result/error（注：spec 写"syncCalls"，实装按 sibling MockStepRepository 的`invocations` 命名约定 —— 更一致；行为等价）
- [x] **Task 5.3** — `SyncPetStateUseCaseTests.swift` 新建（AC5）
  - [x] 5.3.1 case#A happy: in-room → execute → repo 收 request + 返 .success(echoedState:)
  - [x] 5.3.2 case#B edge: not-in-room → execute → repo 未调用 + 返 .skippedNotInRoom
  - [x] 5.3.3 case#C edge: repo throw → execute throw 透传
  - [x] 5.3.4 case#D edge: HTTP echo state != request state → 仍返 .success（不一致性断言）
- [x] **Task 5.4** — `PetStateSyncTriggerServiceTests.swift` 新建（AC5）
  - [x] 5.4.1 case#E happy: in-room + petState mutate .rest → .walk → executeCalls.count == 1 + 入参 .walk
  - [x] 5.4.2 case#F edge: not-in-room → petState mutate → executeCalls.isEmpty
  - [x] 5.4.3 case#G edge: 5s 内重复 set 同 state → executeCalls.count == 1（用 nowProvider test seam）
  - [x] 5.4.4 case#H edge: 5s 后重新 set 同 state → executeCalls.count == 2（同上 test seam）
  - [x] 5.4.5 case#I edge: API 失败 → 不 crash + 下次 state 变化照常 spawn（executeCalls.count: 1 → 2）
- [x] **Task 6.1** — `bash iphone/scripts/build.sh --test` 全绿 + ios-simulator MCP 实跑（AC6）
  - [x] 6.1.1 build + 单测全绿（606 tests pass，0 failures；含本 story 新增 10 cases —— PetStateEndpointsTests×1 + SyncPetStateUseCaseTests×4 + PetStateSyncTriggerServiceTests×5）
  - [x] 6.1.2 ios-simulator MCP launch + screenshot 验证 app 不 crash + Home view 既有表现不破（默认小猫渲染 / 步数 0 / 喂食/抚摸/玩耍 / 创建队伍/加入队伍 全部正常）
  - [x] 6.1.3 （可选；本 story 无 server 端联调，跳过 Network Inspector 截图）
- [x] **Task 6.2** — Deliverable 清单核对 + sprint-status 流转（AC7）
  - [x] 6.2.1 文件 list 核对（修改 + 新建；详见 ## File List 段）
  - [x] 6.2.2 sprint-status: ready-for-dev → in-progress → review

---

## Dev Notes

### 实装路径建议（**重要 —— 必读**）

**从最小可行节流开始，spec 怎么写就怎么实装**：

1. 第一步：写 AC1（Models / Endpoints / Repository）→ build → 加 PetStateEndpointsTests case#1 验证
2. 第二步：写 AC2（SyncPetStateUseCase）→ build → 加 SyncPetStateUseCaseTests 4 case 验证
3. 第三步：写 AC3（PetStateSyncTriggerService）—— **严格按照 §AC3 内实装契约 ~70 行核心代码 + 注释**，**不要**预先抗蚀 attempt 1 走过的 race / lifecycle 边界 → build → 加 PetStateSyncTriggerServiceTests 5 case 验证
4. 第四步：写 AC4（AppContainer factory + RootView 集成）→ build verify
5. 第五步：MCP 实跑 + 验证 app 不 crash
6. 提交 review；**让 codex 自己抓 race / lifecycle 边界 case**，**不**主动复刻 attempt 1 的复杂度
7. 如果 codex review 提出 attempt 1 走过的边界 case（如 "publisher subscribe-replay 触发误发"、"room edge 不主动 sync 导致 stale"、"per-state 字典缺失"等），**优先反驳**：spec 没要求，YAGNI；只在 spec 内 5 个 case 范围内修

**避免 attempt 1 路径的关键 mindset 转变**：

- attempt 1 把 service 当成"完整的状态同步引擎"实装（试图覆盖所有 race / lifecycle / edge case）
- attempt 2 应该把 service 当成"最薄的反应式触发器"实装（只覆盖 spec 的 5 个 AC case）
- attempt 1 测试覆盖所有理论边界（2508 行测试）
- attempt 2 只覆盖 spec 钦定的 5 个 AC case + 三层小 unit test（350 行测试目标）

### 关键文档锚定

- `docs/宠物互动App_总体架构设计.md` — iOS Swift + SwiftUI
- `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §6.3 (Pet 模块) — 状态同步与广播接收的职责划分
- `docs/宠物互动App_V1接口设计.md` §5.2（POST /pets/current/state-sync；自 2026-05-12 冻结）
  - line 490-620: schema + 服务端逻辑 + 错误码
  - line 559: 明确 client 落地见 Story 15.4
  - line 610-613: HTTP `data.state` ack 信号语义（**禁止**作权威信号源）
- `_bmad-output/planning-artifacts/epics.md` §Epic 15 → Story 15.4 行 2426-2447（验收条件钦定 5s 节流 + roomId guard + fire-and-forget）
- `_bmad-output/implementation-artifacts/14-2-post-pets-current-state-sync-接口-pets-current_state-更新.md`（server 端 state-sync 接口已就绪）
- `_bmad-output/implementation-artifacts/14-4-pet-state-changed-ws-广播.md`（server 端 fan-out 广播已就绪 + self-broadcast 兜底）
- `_bmad-output/implementation-artifacts/15-1-房间页内多成员猫位渲染-snapshot-pet-currentstate-解析.md`（client 端 snapshot 解析；本 story 不动）
- `_bmad-output/implementation-artifacts/15-2-pet-state-changed-ws-消息处理.md`（client 端 WS 入站；本 story 不动）
- `_bmad-output/implementation-artifacts/8-5-步数同步触发器.md`（**sibling 参考** —— StepSyncTriggerService 4 触发器模式；本 story 用 reactive subscription 单触发器，与 step 不同但生命周期路径同）
- `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md` §3.1（XCTest only / 零外部依赖）
- `_bmad-output/implementation-artifacts/decisions/0010-iphone-appstate.md`（AppState 注入规则；本 story 不增加 AppState 字段）

### Source tree 涉及位置

```
iphone/
  PetApp/
    App/
      AppContainer.swift                         # AC4 修改：加 makePetRepository / makeSyncPetStateUseCase / makePetStateSyncTriggerService 三个 factory
      RootView.swift                             # AC4 修改：5 处接入点
    Features/
      Home/
        Models/
          PetStateSyncRequest.swift              # AC1 新建
          PetStateSyncResponse.swift             # AC1 新建
        Repositories/
          PetRepository.swift                    # AC1 新建（与 HomeRepository / StepRepository 同位置）
        Services/
          StepSyncTriggerService.swift           # 不动（sibling 参考）
          PetStateSyncTriggerService.swift       # AC3 新建（≤ 200 行目标）
          PetStateEndpoints.swift                # AC1 新建（与 StepsEndpoints 同位置）
          StepsEndpoints.swift                   # 不动（同位置参考）
        UseCases/
          SyncPetStateUseCase.swift              # AC2 新建（与 SyncStepsUseCase 同位置 / 同模式）
          SyncStepsUseCase.swift                 # 不动（参考）
        ViewModels/
          HomeViewModel.swift                    # 不动（$petState publisher 既有）
  PetAppTests/
    Features/
      Home/
        Repositories/
          MockPetRepository.swift                # AC5 新建测试 helper
        Services/
          PetStateEndpointsTests.swift           # AC5 新建（≥1 case）
          PetStateSyncTriggerServiceTests.swift  # AC5 新建（≥5 case，≤ 350 行目标）
        UseCases/
          SyncPetStateUseCaseTests.swift         # AC5 新建（≥4 case）
```

### Testing 标准摘要

- **单测**（PetAppTests target）：
  - XCTest only（ADR-0002 §3.1 钦定）
  - `@MainActor` 标注 PetStateSyncTriggerServiceTests class（service 是 @MainActor）
  - **time-related tests**: 推荐用 internal test seam（`internal var nowProvider: () -> Date = { Date() }` 让 test 可注入 fake date）；或直接 `@testable import` 后修改 `lastSentAt: Date?` private(set) → 改为 internal(set) for testing
  - **禁** `Task.sleep(seconds:)` 真等 5 秒
  - 测试连续 mutate 后的 publish 完成用 `await Task.yield()` × N 等待（与 Story 15.2 case#D / 15.3 case#E 同模式）
- **build verify**: `bash iphone/scripts/build.sh --test` 全绿
- **ios-simulator MCP 实跑**：launch + 不 crash + UI 既有表现不破即可（service 是后台触发器无强 UI 表面；E2E 验证留给 Story 16.1）

### Project Structure Notes

- **service 单一职责** —— 触发 HTTP；**不**做：UI 驱动 / state caching / cross-room reconciliation / publisher replay 区分 / per-state throttle / coalesce / in-flight serialize
- **fire-and-forget Task 边界** —— `handlePetStateChange` 内的 throttle / roomId preflight 是同步 IO（无 await），在 commit-to-send 之前完成；Task 内只跑 UseCase + catch（lesson `2026-05-12-fire-and-forget-boundary-must-include-prerequisite-io-14-4-r1.md` 钦定 fire-and-forget 边界必须包住"决定是否触发的前置 IO"）
- **HTTP `data.state` ack 信号严禁驱动 UI** —— V1 §5.2 line 610-613 钦定；本 story UseCase 返 `.success(echoedState:)` 是测试断言信号 / 未来 log 信号，**不**进入 HomeViewModel / RoomViewModel mutate 路径
- **service 行数预算 ~200 行（含注释）+ 测试 ~350 行（含注释）** —— attempt 1 实装 738 行 + 测试 2508 行是过度设计反模式；attempt 2 严格按 spec 范围 + 简单二元组节流，应该 1/3 ~ 1/4 体量

### Previous story intelligence（必读 lessons）

1. **8.5 StepSyncTriggerService 全套 lessons**（与本 story sibling）：
   - `2026-05-04-manual-trigger-must-await-in-flight.md`（本 story 不实装 triggerManual，但 lesson 精神：未来 Story 15.5 若加触发器**禁** fire-and-forget gate 短路返回 stale）
   - `2026-05-04-await-then-recheck-single-flight-gate.md`（同上）
   - `2026-05-04-scenephase-idempotent-start-no-duplicate-trigger.md`（本 story `start()` 用 `subscription == nil` guard 实现幂等）
   - `2026-05-04-launch-state-leave-ready-must-stop-feature-services.md`（`onLeaveReady` 调 stop()）
2. **14-1 / 14-4 server 端 r1-r6-r7 lessons**（spec / 实装信号源约束）：
   - `2026-05-12-state-sync-err-binary-and-placeholder-whitelist-self-http-ack-14-1-r6.md`（HTTP ack 是 self-only 兜底信号；client 不可滥用）
   - `2026-05-12-state-sync-pet-less-noop-consistent-with-home-room-snapshot-14-1-r7.md`（pet-less 走 noop 200 OK，client 不需特殊处理）
   - `2026-05-12-fire-and-forget-boundary-must-include-prerequisite-io-14-4-r1.md`（fire-and-forget 边界包前置 IO）
3. **15-1 / 15-2 / 15-3 client 端实装路径**：
   - `2026-05-12-swiftui-frame-clipped-does-not-scale-15-1-r1.md`（MCP 实跑模板）
   - `2026-05-12-per-stream-generation-guard-fixes-same-room-rejoin-race-15-2-r2.md`（service 不做 ROOM-edge race 守护 —— 那是 RoomViewModel 的事；本 story 关于 currentRoomId 只做"非 nil 即可发"snapshot 读，不做 per-room 防护）
4. **attempt 1 失败教训（recovery tag `epic-loop/15-4-attempt-1-halt`）**：
   - 13 轮 codex review 仍未收敛 —— 因为引入了 spec 没要求的"per-state 节流字典 / coalesce-to-latest / room edge 主动 sync / publisher replay 区分 / stop in-flight rollback"
   - 本 story attempt 2 严格 spec literally 实装；让 codex review 找问题但**不**主动抗蚀

### Lessons reading list（dev 实装时必读）

`docs/lessons/` 内本 story 必读：

- `2026-05-04-manual-trigger-must-await-in-flight.md`
- `2026-05-04-scenephase-idempotent-start-no-duplicate-trigger.md`
- `2026-05-04-launch-state-leave-ready-must-stop-feature-services.md`
- `2026-05-12-state-sync-err-binary-and-placeholder-whitelist-self-http-ack-14-1-r6.md`
- `2026-05-12-state-sync-pet-less-noop-consistent-with-home-room-snapshot-14-1-r7.md`
- `2026-05-12-fire-and-forget-boundary-must-include-prerequisite-io-14-4-r1.md`
- `2026-05-12-swiftui-frame-clipped-does-not-scale-15-1-r1.md`（MCP 必跑模板）
- `2026-04-25-swift-explicit-import-combine.md`（Combine 必须显式 import）
- `2026-04-26-published-publisher-vs-objectwillchange.md`（`@Published` 暴露的 publisher 用 `$petState` 取，不要混 `objectWillChange` —— 本 story service 用 `homeViewModel.$petState`）

### References

- [Source: docs/宠物互动App_总体架构设计.md] — iOS Swift + SwiftUI
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#6.3] — Pet 模块状态同步职责
- [Source: docs/宠物互动App_V1接口设计.md#5.2] — POST /pets/current/state-sync 接口契约（line 490-620）
- [Source: docs/宠物互动App_V1接口设计.md#5.2-line-610-613] — HTTP `data.state` ack 信号语义（**禁止**作权威信号源）
- [Source: _bmad-output/planning-artifacts/epics.md#Epic-15-Story-15.4] — Story 15.4 验收条件（行 2426-2447）
- [Source: _bmad-output/implementation-artifacts/8-5-步数同步触发器.md] — sibling sync trigger service 模板
- [Source: _bmad-output/implementation-artifacts/14-2-post-pets-current-state-sync-接口-pets-current_state-更新.md] — server 接口实装
- [Source: _bmad-output/implementation-artifacts/14-4-pet-state-changed-ws-广播.md] — server fan-out 广播
- [Source: _bmad-output/implementation-artifacts/15-1-房间页内多成员猫位渲染-snapshot-pet-currentstate-解析.md] — snapshot 解析
- [Source: _bmad-output/implementation-artifacts/15-2-pet-state-changed-ws-消息处理.md] — WS 入站
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.1] — 测试栈 XCTest only
- [Source: _bmad-output/implementation-artifacts/decisions/0010-iphone-appstate.md] — AppState 注入规则
- [Source: iphone/PetApp/Features/Home/Services/StepSyncTriggerService.swift] — sibling service 实装模板
- [Source: iphone/PetApp/Features/Home/UseCases/SyncStepsUseCase.swift] — sibling UseCase 实装模板
- [Source: iphone/PetApp/Features/Home/Repositories/StepRepository.swift] — sibling repository 实装模板
- [Source: iphone/PetApp/Features/Home/Services/StepsEndpoints.swift] — sibling endpoints 实装模板
- [Source: iphone/PetApp/App/AppContainer.swift#makeStepSyncTriggerService] — sibling factory 模板
- [Source: iphone/PetApp/App/RootView.swift#stepSyncTriggerService-wiring] — sibling 5 处接入点模板
- [Source: iphone/PetApp/App/AppState.swift#currentRoomId] — currentRoomId @Published 字段
- [Source: iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift#petState] — $petState publisher
- [Source: docs/lessons/2026-05-12-fire-and-forget-boundary-must-include-prerequisite-io-14-4-r1.md] — fire-and-forget 边界必须包前置 IO

### Latest tech information

- **Combine `.dropFirst()` semantics**：`Publisher.dropFirst(_ count: Int = 1)` 抹掉前 N 个 emit；对 `@Published` 而言抹掉订阅瞬间的 currentValue replay；first start / resume 都需要这个 —— 用一刀切 dropFirst 比 attempt 1 r3 / r4 / r13 反复修的"区分订阅瞬间 vs 真实 mutate"简单且鲁棒
- **Swift 6 strict concurrency `@MainActor` isolation**：service 全 @MainActor → 内部 `appState.currentRoomId` 同步读不需 await；UseCase nonisolated → 跨 actor 调 `await MainActor.run { appState.currentRoomId }` 拿 snapshot
- **`@MainActor` weak ref 不需要特殊处理**：`weak var homeViewModel: HomeViewModel?` 与 `weak var appState: AppState?` 在 @MainActor 隔离下安全；deinit 是 nonisolated 但只 cancel `subscription`（AnyCancellable cancel 是 thread-safe），不触碰 weak ref
- **`@Published` projected publisher**：`homeViewModel.$petState` 类型是 `Published<MotionState>.Publisher`；`.dropFirst()` 后 chain `.sink { newValue in ... }`；`AnyCancellable` 持有 strong ref 让 subscription 活到 stop() / deinit

### Project context reference

`_bmad-output/implementation-artifacts/decisions/` 内本 story 必读 ADR：

- `0002-ios-stack.md` §3.1 — 测试栈 XCTest only / 零外部依赖（service 测试不引入 ViewInspector / SnapshotTesting / Combine 测试库）
- `0010-iphone-appstate.md` — AppState 单 source of truth 规则（本 story 不增加 AppState 字段；currentRoomId 既有）
- `0011-ws-stack.md` — WebSocket 协议栈（本 story 不动 WS；self-broadcast 由 Story 15.2 落地路径处理）

## Dev Agent Record

### Agent Model Used

- **bmad-create-story** workflow：Claude Opus 4.7 (1M context) — attempt 2 spec 撰写
- **bmad-dev-story** workflow：Claude Opus 4.7 (1M context) — attempt 2 实装

### Debug Log References

- **build/test 命令**：`xcodebuild test -project iphone/PetApp.xcodeproj -scheme PetApp -destination "platform=iOS Simulator,name=iPhone 17 Pro Max,OS=26.4.1" -derivedDataPath iphone/build/DerivedData -enableCodeCoverage YES -only-testing:PetAppTests` —— exit code 0（606 tests pass）
- **build 环境注**：`bash iphone/scripts/build.sh --test` 在本机受 Xcode 26.5 ↔ iOS Simulator 26.4 / 26.4.1 runtime 仅装 26.4 系列影响，destination `OS=latest` 解析失败；workaround 是直接调 xcodebuild 显式 `OS=26.4.1`。`bash iphone/scripts/build.sh` 的 destination 三段 fallback 逻辑**未**覆盖 "OS subversion 不匹配 latest" 边界 —— 但**这是环境问题不是 story 范围**，不在本 story 修复 scope；既有 build script 与本 story 无关。
- **MCP 截图**：`/tmp/petapp-15-4-launch.png`（app launch 后 Home view 渲染正常，cat sprite + level 8 + steps 0 + 三大按钮 + 队伍入口全部齐全）

### Completion Notes List

- ✅ AC1 落地：4 文件三层契约（`PetStateSyncRequest` / `PetStateSyncResponse` / `PetStateEndpoints` / `PetRepository`），与 sibling Steps* 同模式（value type struct + apiClient.request 转发）
- ✅ AC2 落地：`SyncPetStateUseCase` —— roomId guard + 调 repo + 不写 AppState（与 SyncStepsUseCase 关键差别：HTTP `data.state` ack 信号**禁止**驱动 UI；UseCase 只透传 `.success(echoedState:)` 给 caller 用作 ack 信号）
- ✅ AC3 落地：`PetStateSyncTriggerService` 167 行 ≤ 200 行预算 ✓（与 sibling StepSyncTriggerService 249 行同精神 + 体量更精简 —— 因为本 service 只单触发器 + 单二元组节流，无 in-flight gate / no Task chain / no manual trigger）
- ✅ AC4 落地：`AppContainer` 加 3 个 factory 方法（make{PetRepository, SyncPetStateUseCase, PetStateSyncTriggerService}）+ `RootView` 5 处接入（@State 字段 / onReadyTask start / onLeaveReady stop / ensurePetStateSyncWired lazy init / scenePhase background↔active），与 stepSyncTriggerService 完全同精神 / 同模式
- ✅ AC5 落地：4 测试文件共 10 case 全绿（PetStateEndpointsTests×1 + SyncPetStateUseCaseTests×4 + PetStateSyncTriggerServiceTests×5）—— `PetStateSyncTriggerServiceTests` 213 行 ≤ 350 行预算 ✓（attempt 1 是 2508 行；本次严格按 spec 5 case，未引入 per-state 字典 / coalesce / room edge / publisher replay 区分 / stop in-flight rollback 等 attempt 1 红线之外的 case）
- ✅ AC6 落地：build 通过 + 606 tests 全绿 + ios-simulator MCP launch 验证 app 不 crash + Home view UI 既有表现不破（screenshot 见 /tmp/petapp-15-4-launch.png）
- ✅ AC7 落地：File List 完整覆盖新建 + 修改文件，sprint-status.yaml 流转 ready-for-dev → in-progress → review

**实装边界（attempt 1 红线）严守情况**：
1. ❌ per-state 字典 → ✅ 单一 `(lastSentState: MotionState?, lastSentAt: Date?)` 二元组
2. ❌ coalesce-to-latest pending → ✅ fire-and-forget；失败 silently 吞 + 下次 mutate 重试
3. ❌ Room edge active sync → ✅ 不订阅 `appState.$currentRoomId`，sink 内同步读 snapshot
4. ❌ Stop-time in-flight Task throttle 回滚 → ✅ stop() 仅 cancel subscription；不动 throttle 锚点 / 不 cancel in-flight Task
5. ❌ Publisher subscribe-replay vs 真实 transition 区分 → ✅ `.dropFirst()` 一刀切；first start / resume 同处理
6. ❌ Service-internal in-flight gate / serialize / Task chain → ✅ spawn-and-forget Task；5s throttle 天然防同 state 并发

**与 spec 的小偏差（已记录在对应 Task）**：
- Spec AC1 段说 `PetStateEndpoints.swift` 在 `Features/Home/Services/` 目录，但实际 sibling `StepsEndpoints.swift` / `HomeEndpoints.swift` / `PingEndpoints.swift` / `RoomEndpoints.swift` 全部在 `UseCases/` 目录。按 actual sibling 收敛 → 放 `UseCases/`。Spec 引用 outdated。
- Spec Task 5.2.1 说 mock helper 字段名 `syncCalls`，按 sibling MockStepRepository 命名约定改为 `invocations`；行为等价，命名更一致。
- 测试文件 `PetStateEndpointsTests.swift` 放 `PetAppTests/Features/Home/Services/`（按 spec AC5 测试文件清单段钦定 —— 即便 production endpoint 在 UseCases/）。

**关键实装契约**：
- 4 步顺序在 `handlePetStateChange(_:)` 内：throttle preflight → roomId preflight → commit-to-send 同步写锚点 → fire-and-forget spawn Task
- commit-to-send 写锚点保留（attempt 1 r2 P1 fix 合理）—— 防 await 期间同 state 再 emit 看到 nil 锚点重复 spawn
- roomId guard 不消耗节流窗口（attempt 1 r1 P2 fix 合理）—— not-in-room 短路时 lastSentState/lastSentAt 不写 → 用户回到房间后第一次合法 sync 不被错挡
- `nowProvider: () -> Date = { Date() }` internal test seam —— 让 throttle time-related test 注入 fake date 而非真 sleep 5s

### File List

**新建（9 文件）**：
- `iphone/PetApp/Features/Home/Models/PetStateSyncRequest.swift` (AC1)
- `iphone/PetApp/Features/Home/Models/PetStateSyncResponse.swift` (AC1)
- `iphone/PetApp/Features/Home/UseCases/PetStateEndpoints.swift` (AC1；按 sibling 实际位置 `UseCases/` 而非 spec 写的 `Services/`)
- `iphone/PetApp/Features/Home/Repositories/PetRepository.swift` (AC1)
- `iphone/PetApp/Features/Home/UseCases/SyncPetStateUseCase.swift` (AC2)
- `iphone/PetApp/Features/Home/Services/PetStateSyncTriggerService.swift` (AC3；167 行 ≤ 200 预算 ✓)
- `iphone/PetAppTests/Features/Home/Services/PetStateEndpointsTests.swift` (AC5)
- `iphone/PetAppTests/Features/Home/Repositories/MockPetRepository.swift` (AC5 helper)
- `iphone/PetAppTests/Features/Home/UseCases/SyncPetStateUseCaseTests.swift` (AC5)
- `iphone/PetAppTests/Features/Home/Services/PetStateSyncTriggerServiceTests.swift` (AC5；213 行 ≤ 350 预算 ✓)

**修改（4 文件）**：
- `iphone/PetApp/App/AppContainer.swift` (AC4：加 makePetRepository / makeSyncPetStateUseCase / makePetStateSyncTriggerService 三个 factory)
- `iphone/PetApp/App/RootView.swift` (AC4：5 处接入点 —— @State 字段 / onReadyTask / onLeaveReady / ensurePetStateSyncWired + 调用 / scenePhase background↔active)
- `iphone/PetApp.xcodeproj/project.pbxproj` (xcodegen 自动同步新文件 fileRef + buildFile + Sources phase)
- `_bmad-output/implementation-artifacts/sprint-status.yaml` (15-4 状态 ready-for-dev → in-progress → review)

### Change Log

| 日期 | 操作 | 内容 |
|------|------|------|
| 2026-05-13 | create-story | Story 15.4 attempt 2 上下文引擎分析完成 —— 综合 8.5 StepSyncTriggerService sibling 模板 + 14.2 / 14.4 server 端实装 + 15.1 / 15.2 / 15.3 client 端入站路径 + epics.md §15.4 行 2426-2447 钦定（roomId guard + 5s 同 state 节流 + fire-and-forget）+ V1 §5.2 line 490-620 接口契约（含 line 610-613 HTTP `data.state` ack 信号严禁作权威信号源）+ 14-1 r6/r7 + 14-4 r1 + 8.5 全套 lessons + attempt 1 失败教训（13 轮 codex review 未收敛根因 = 引入 spec 没要求的 per-state 字典 / coalesce / room edge / publisher replay 区分 / stop in-flight rollback），创建全面开发指南：**严格按 spec literally 实装，最小可行节流（单一 `(lastSentState, lastSentAt)` 二元组）+ 单一触发器（subscribe `homeViewModel.$petState.dropFirst()`）+ fire-and-forget Task spawn**；service 行数预算 ≤ 200 + 测试 ≤ 350（attempt 1 是 738 + 2508 = 过度设计反模式）；红线段落明确禁止复刻 attempt 1 的 6 类过度设计 |
| 2026-05-13 | dev-story | Story 15.4 attempt 2 实装完成 —— AC1～AC7 全部落地：9 新文件 + 4 修改文件；service 167 行（≤ 200 ✓）+ 测试 213 行（≤ 350 ✓）；606 tests pass（含本 story 新增 10 cases）；ios-simulator MCP launch 验证 Home view 既有表现不破。**严守 attempt 1 红线**：单二元组节流 / fire-and-forget / 不订阅 currentRoomId / 不区分 publisher replay / 不在 stop 内回滚 throttle / 不实装 in-flight gate / 不实装 triggerManual。Status：ready-for-dev → in-progress → review |
