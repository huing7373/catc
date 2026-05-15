# Story 21.2: GET /chest/current 调用 + 状态展示 + 主动定时纠正

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want App 启动时拉到准确的宝箱状态，且本地倒计时不要持续偏离 server,
so that 我看到的宝箱状态可信、不会出现"本地以为可开但 server 还在 counting"的错位。

## 故事定位（Epic 21 第 2 条 story；接 21.1 落地的本地 timer 驱动 → 加上 server 权威拉取 + 定时校准）

这是 Epic 21「iOS - 首页宝箱倒计时 + 奖励弹窗」第 2 条 story —— 在 21.1 已落地的 `ChestCardView`（按 `AppState.currentChest` 渲染）+ `ChestTimerDriver`（订阅 `AppState.currentChest` 变化驱动本地倒计时）基础上，**新增 `LoadChestUseCase` + `ChestRefreshTriggerService` 闭环**：

1. **`ChestRepository` + `ChestEndpoints`** 封装 GET /api/v1/chest/current（与 `HomeRepository` / `PetRepository` 同模式：value-type struct，apiClient.request 转发）；
2. **`LoadChestUseCase`** = repo.fetchCurrent() → 转 domain `HomeChest` → 写 `appState.currentChest`（与 `SyncStepsUseCase.execute` 写 `appState.currentStepAccount` 同模式）；
3. **`ChestRefreshTriggerService`** 持 3 触发器（launch / foreground / 60s timer）+ in-flight gate（与 `StepSyncTriggerService` 同模式但简化 —— 本 story 不需要 `triggerManual`），失败不破坏 UI（保留上次 hydrate 的 `appState.currentChest`，下次重试）；
4. **`AppContainer` 加 `makeChestRepository` / `makeLoadChestUseCase` / `makeChestRefreshTriggerService` 三个 factory**（与 Step / Pet 链路同模式）；
5. **`RootView` wire**：`@State chestRefreshTriggerService` + `ensureChestRefreshWired()` lazy init + `.scenePhase .active` → `start()` / `.background` → `stop()`（与 `stepSyncTriggerService` 100% 镜像，避免破坏既有 lifecycle race）；
6. **单元测试** ≥5 case（mocked repo + AppState + fake clock）+ **集成测试** 通过 UITest mock server 路径验证启动 → ChestCardView 显示 server 返回的状态。

**本 story 落地后立即解锁**：

- Story 21.3 OpenChestUseCase 调用前不再需要自己拉 server 状态（本 story 60s timer + foreground trigger 已保 `appState.currentChest` 新鲜）；
- 倒计时到 0 时 UI 视觉立刻切 unlockable（21.1 ChestCardView `remainingSeconds <= 0` 预切），同时本 story `ChestRefreshTriggerService` 60s 定时拉取在最迟 60s 内把 server 端 `status = 2` 拿回填到 AppState（让 domain 状态最终与 UI 一致）。

**关键路径（与 Step / Pet 链路同精神，"server-truth pull + 触发器 + AppState 写入"）**：

- 21.1 `ChestTimerDriver` 订阅 `appState.$currentChest` → 每次 AppState 变化（无论本地 timer 推进还是 server pull 校准）driver 都 react 重启倒计时；
- 本 story `LoadChestUseCase` 写 AppState → driver 立即重新计算 `chestRemainingSeconds` → ChestCardView 重新渲染；
- 触发器 (`ChestRefreshTriggerService`) 完全独立于 21.1 driver：driver 是 view-state 派生（绝对时间倒计时），refresh service 是 domain 拉取，两者通过 AppState 解耦。

**不涉及**（红线）：

- **不**实装 POST /chest/open 调用（Story 21.3 落地）；本 story 只读不写；
- **不**实装奖励弹窗（Story 21.4 落地）；
- **不**改 21.1 已落地的 `ChestCardView` / `ChestTimerDriver` / `HomeViewModel.chestRemainingSeconds` 任何一行代码（21.1 view-state 契约稳定，本 story 只新增 domain hydrate 通道）；
- **不**改 `ChestTimerDriver` 行为（V1 §7.1 关键约束 "本地倒计时归零时 client 应主动重新 GET" 由本 story `ChestRefreshTriggerService` 60s timer 兜底；不在 driver 内做主动拉取避免职责膨胀）；
- **不**在 `LoadChestUseCase` 内调 `ChestTimerDriver` 任何方法（driver 是 view-state 实现细节，UseCase 只写 AppState，driver 通过 Combine sink 自动 react）；
- **不**用 Foundation `Timer.publish` 做 60s 定时（参照 `StepSyncTriggerService` 选型决策：用 `Task.sleep(nanoseconds:)` + `while !Task.isCancelled` 循环，@MainActor 友好 + Swift 6 strict concurrency 干净）；
- **不**做 retry / 指数退避（背景拉取失败不阻塞 UI，下次 60s 定时器或 foreground 触发再试；与 `SyncStepsUseCase` AC 钦定 "失败不阻塞" 同精神）；
- **不**改 `RootView` 既有 `stepSyncTriggerService` / `petStateSyncTriggerService` 任何一行代码（仅在它们旁边新增 `chestRefreshTriggerService` 字段 + 镜像 wire 节奏）；
- **不**调 `ErrorPresenter`（背景拉取失败静默吞 + log；与 `StepSyncTriggerService.runSync` AC 钦定 "失败 silently 吞掉是 by design" 同精神）；
- **不**改 `AppState.applyHomeData(_:)` 既有路径（已 hydrate `currentChest` 字段；本 story 在 `LoadChestUseCase` 内单字段 mutation 走新加的 `AppState.applyCurrentChest(_:)` 入口，与 `applySyncedStepAccount(_:)` 同前缀同模式）；
- **不**改 server 任何文件（端独立原则；GET /api/v1/chest/current 由 Story 20.5 落地）；
- **不**改 ios/ 任何文件（CLAUDE.md + ADR-0002 §3.3）；
- **不**引 SnapshotTesting / ViewInspector / OHHTTPStubs / Mockingbird（ADR-0002 §3.1 钦定 XCTest only + 既有 MockBase / MockHomeRepository 等已落地 mock 模式复用）。

## Acceptance Criteria

> **AC 编号体系**：AC1 = ChestEndpoints + ChestRepository；AC2 = LoadChestUseCase；AC3 = AppState.applyCurrentChest 单字段 mutation 入口；AC4 = ChestRefreshTriggerService（3 触发器 + 60s timer + in-flight gate）；AC5 = AppContainer 3 个 factory；AC6 = RootView wire（@State + ensure*Wired + scenePhase start/stop）；AC7 = 单元测试 ≥5 case；AC8 = 集成测试（UITest + mock server）；AC9 = build verify + ios-simulator MCP UI 实跑；AC10 = Deliverable 清单。

### AC1 — `ChestEndpoints` + `ChestRepository` 封装 GET /api/v1/chest/current

**新建文件**：

- `iphone/PetApp/Features/Home/UseCases/ChestEndpoints.swift`（与 `HomeEndpoints` / `StepsEndpoints` 同模式）
- `iphone/PetApp/Features/Home/Repositories/ChestRepository.swift`（与 `HomeRepository` / `PetRepository` 同模式）

**ChestEndpoints 契约**：

```swift
// ChestEndpoints.swift
// Story 21.2 AC1: GET /api/v1/chest/current endpoint 工厂；与 HomeEndpoints / PetStateEndpoints 同模式.
//
// path 必须**含** `/api/v1` 前缀（与 HomeEndpoints 同模式 —— APIClient 用 host-only baseURL，
// 拼出的 URL 是 baseURL + endpoint.path = "http://localhost:8080" + "/api/v1/chest/current"）.

import Foundation

public enum ChestEndpoints {
    /// GET /api/v1/chest/current —— 当前宝箱状态查询（V1 §7.1）.
    /// requiresAuth=true：自动经过 APIClient 的 token 注入（Story 5.3）+ AuthBoundaryAPIClient
    /// 装饰器（ADR-0008 v2 → 401 自动触发 cold-start 重跑 bootstrap）.
    public static func current() -> Endpoint {
        Endpoint(path: "/api/v1/chest/current", method: .get, body: nil, requiresAuth: true)
    }
}
```

**ChestRepository 契约**：

```swift
// ChestRepository.swift
// Story 21.2 AC1: 封装 GET /api/v1/chest/current 调用；与 HomeRepository / PetRepository 同模式.
//
// `DefaultChestRepository` 是 `struct`：value type，无内部状态，构造廉价；与 DefaultHomeRepository / DefaultPetRepository 同模式.
//
// 错误处理（与 HomeRepository 同精神）：APIError 原样透传；不在 repo 层吞错或转码.

import Foundation

public protocol ChestRepositoryProtocol: Sendable {
    /// 调 GET /api/v1/chest/current 拿当前宝箱状态.
    /// - Returns: ChestCurrentResponse（5 字段：id / status / unlockAt / openCostSteps / remainingSeconds）
    /// - Throws: APIError.business(1001 / 1005 / 1009 / 4001) / APIError.network /
    ///           APIError.unauthorized / APIError.missingCredentials / APIError.decoding
    func fetchCurrent() async throws -> ChestCurrentResponse
}

public struct DefaultChestRepository: ChestRepositoryProtocol {
    private let apiClient: APIClientProtocol

    public init(apiClient: APIClientProtocol) {
        self.apiClient = apiClient
    }

    public func fetchCurrent() async throws -> ChestCurrentResponse {
        try await apiClient.request(ChestEndpoints.current())
    }
}
```

**ChestCurrentResponse wire DTO**（新建在 `iphone/PetApp/Features/Home/Models/ChestCurrentResponse.swift`，与 `HomeResponse` 同模式）：

```swift
// ChestCurrentResponse.swift
// Story 21.2 AC1: GET /api/v1/chest/current 响应 wire DTO；V1 §7.1 钦定 5 字段.
//
// 与 ChestDTO（HomeResponse.swift §6.7）字段名 / 类型 100% 对齐（V1 §7.1 行 1208 钦定跨接口字段对齐）.
// 不复用 ChestDTO 的原因：HomeResponse 内 ChestDTO 是嵌套字段；本 endpoint 直接顶层返回 5 字段；
// V1 §13 全局信封封装层在 APIClient 已处理；本 DTO 只描述 `data` 段 5 字段.

import Foundation

public struct ChestCurrentResponse: Decodable, Equatable, Sendable {
    public let id: String                 // BIGINT 字符串化（V1 §2.5）
    public let status: Int                // 1 = counting, 2 = unlockable（V1 §7.1）
    public let unlockAt: Date             // ISO 8601 RFC3339（APIClient JSONDecoder 已配 .iso8601）
    public let openCostSteps: Int         // 节点 7 阶段固定 1000
    public let remainingSeconds: Int      // server 端 max(0, ceil((unlock_at - now) / 1s)); 总是 ≥ 0

    public init(id: String, status: Int, unlockAt: Date, openCostSteps: Int, remainingSeconds: Int) {
        self.id = id
        self.status = status
        self.unlockAt = unlockAt
        self.openCostSteps = openCostSteps
        self.remainingSeconds = remainingSeconds
    }
}
```

> **关键决策 1（独立 DTO vs 复用 ChestDTO）**：独立 `ChestCurrentResponse`。理由：(a) V1 §7.1 是顶层接口，与 §5.1 GET /home 内嵌的 chest 字段在序列化路径上独立；(b) 未来若 V1 §7.1 加字段（如 server 端时钟戳）独立 DTO 不会污染 HomeResponse；(c) 与 `PetStateSyncResponse` / `StepsSyncResponse` 等独立响应 DTO 同模式。

> **关键决策 2（response 字段 `Int` 而非 `UInt`）**：`Int`。理由：V1 §7.1 关键约束行 912 钦定 "client 解析层**应**按 `Int` 处理（不是 `UInt` —— Swift 端 `UInt` 在解析时若收到负数会 crash）"；server 保证 `remainingSeconds ≥ 0` 但 client 防御性按 Int。

> **关键决策 3（repo 不做 ChestCurrentResponse → HomeChest 转换）**：repo 返回 wire DTO。理由：与 `HomeRepository.loadHome()` 返回 `HomeResponse` 同模式；UseCase 层做 DTO → domain 转换（保 repo 单一职责）。

**对应 Tasks**: Task 1.1, 1.2, 1.3

### AC2 — `LoadChestUseCase`：repo → DTO → HomeChest → 写 AppState

**新建文件**：`iphone/PetApp/Features/Home/UseCases/LoadChestUseCase.swift`

**契约**（与 `SyncStepsUseCase` 同模式：业务编排 + 写 AppState；与 `LoadHomeUseCase` 同精神：错误原样透传不吞）：

```swift
// LoadChestUseCase.swift
// Story 21.2 AC2: 宝箱状态加载 UseCase（业务编排：repo → DTO 转 domain → 写 AppState）.
//
// 职责（epics.md AC 行 3048-3052）:
//   1. 调 repository.fetchCurrent() 拿 ChestCurrentResponse
//   2. 转 domain HomeChest（与 HomeData(from:) 内 ChestDTO → HomeChest 同精神；未知 status fail-fast 抛 .decoding）
//   3. 成功 → 调 appState.applyCurrentChest(_:) 写入 AppState.currentChest（与 SyncStepsUseCase 写 stepAccount 同模式）
//   4. 失败 → throw 透传给上层 ChestRefreshTriggerService（service 决定不阻塞 UI / 下次再试）
//
// **不**做的事:
//   - 不接 ErrorPresenter（背景拉取失败不弹 toast；与 SyncStepsUseCase 同精神 → 失败不破坏 UI）
//   - 不做 retry / 指数退避（caller ChestRefreshTriggerService 用 60s timer 自然兜底；YAGNI）
//   - 不动 ChestTimerDriver / HomeViewModel.chestRemainingSeconds（driver 通过 Combine sink AppState.$currentChest 自动 react；
//     与 Story 21.1 AC2 钦定 "driver 订阅 AppState 而非 ViewModel 自己字段" 一致）
//   - 不读 HomeViewModel 任何字段（UseCase 不感知 view 层；与 SyncStepsUseCase 不读 ViewModel 同模式）

import Foundation

public protocol LoadChestUseCaseProtocol: Sendable {
    /// 执行一次宝箱状态拉取并写入 AppState.
    /// - Throws: APIError（全部 case 原样透传）/ HomeDataDecodingError（未知 chest.status 时）
    func execute() async throws
}

public struct DefaultLoadChestUseCase: LoadChestUseCaseProtocol {
    private let repository: ChestRepositoryProtocol
    private let appState: AppState

    public init(repository: ChestRepositoryProtocol, appState: AppState) {
        self.repository = repository
        self.appState = appState
    }

    public func execute() async throws {
        let response = try await repository.fetchCurrent()

        // DTO → domain（未知 chest.status fail-fast 抛 APIError.decoding；与 HomeData(from:) 同模式）.
        guard let status = HomeChestStatus(rawValue: response.status) else {
            throw APIError.decoding(underlying: HomeDataDecodingError.unknownChestStatus(response.status))
        }
        let chest = HomeChest(
            id: response.id,
            status: status,
            unlockAt: response.unlockAt,
            openCostSteps: response.openCostSteps,
            remainingSeconds: response.remainingSeconds
        )

        // 写 AppState（与 addendum 钦定一致；**不**写 ViewModel）.
        // 用 AppState 提供的 mutation 入口（AC3：applyCurrentChest(_:)）.
        await MainActor.run {
            appState.applyCurrentChest(chest)
        }
    }
}
```

> **关键决策 1（写 AppState 单字段而非 applyHomeData 全字段）**：单字段 mutation 走新加 `applyCurrentChest(_:)`。理由：(a) addendum 钦定 "LoadChestUseCase 拿到 server 状态后写入目标改为 `appState.currentChest`"；(b) 本 endpoint 只返 5 个 chest 字段，无法构造完整 `HomeData` 走 `applyHomeData`；(c) 与 `SyncStepsUseCase` → `applySyncedStepAccount(_:)` 同前缀同模式（`apply*` 前缀表示 hydrate / mutation 入口）。

> **关键决策 2（未知 status fail-fast 抛 .decoding 而非 silently coerce）**：fail-fast。理由：与 Story 5.5 round 6 [P2] fix 钦定一致（详见 `docs/lessons/2026-04-27-home-data-fail-fast-on-unknown-enum.md`）；V1 §7.1 字段表 `status` 枚举值冻结，出现 99 之类未知值即 schema drift 信号；ChestRefreshTriggerService 层 catch 后 silently 吞（log）+ 保留 AppState 上次值 → UI 不破坏，dev 仍能从 underlying error 看到 drift。

> **关键决策 3（4001 不在 UseCase 内特殊处理）**：原样透传。理由：(a) UseCase 层不感知错误码业务含义；(b) ChestRefreshTriggerService 层失败统一 silently 吞（log）；(c) 与 LoadHomeUseCase "所有错误**原样**透传" 同精神。Story 21.1 ChestCardView 已对 `currentChest == nil` 渲染 EmptyView()，4001 路径下 AppState.currentChest 保持上次值或仍是 nil 都不破坏 UI。

> **关键决策 4（`await MainActor.run` 包裹 AppState 写入）**：与 `SyncStepsUseCase` 同模式。理由：AppState 是 `@MainActor`，UseCase 自身不限定 actor（Sendable struct），写入时需显式 hop 到 main actor。

**对应 Tasks**: Task 2.1

### AC3 — `AppState` 新增 `applyCurrentChest(_:)` 单字段 mutation 入口

**改动文件**：`iphone/PetApp/App/AppState.swift`

**关键改动**：在 `applySyncedStepAccount(_:)` 旁边新增同模式方法：

```swift
/// Story 21.2 AC3: 宝箱状态拉取成功后写入 currentChest 单字段.
/// 由 LoadChestUseCase.execute() 在 GET /chest/current 成功后调；不动其它 6 字段
/// （与 applyHomeData 全字段写入区分；与 applySyncedStepAccount 同模式）.
///
/// 命名 `applyCurrentChest` 与 `applyHomeData` / `applySyncedStepAccount` 同前缀（apply* 前缀表示
/// "hydrate / mutation 入口"；详见 ADR-0010 §3.3）；后缀 `CurrentChest` 表达数据来源
/// （GET /chest/current 接口；与 SyncedStepAccount = 步数同步动作返回 同精神）.
///
/// **不**包装 Optional：caller LoadChestUseCase 收到 API success 路径必有 5 字段（V1 §7.1 响应字段必填；
/// 不可能为 nil，schema 已冻结）.
///
/// **不**触发 roomNavigationGeneration bump（chest mutation 与 room navigation 完全独立；与
/// applySyncedStepAccount 不 bump 同精神 / 同决策依据：Story 12.7 r12 [P2] fix 钦定 generation
/// 仅在 room 字段实际变更时 bump）.
public func applyCurrentChest(_ chest: HomeChest) {
    self.currentChest = chest
}
```

> **关键决策（不 bump roomNavigationGeneration）**：不 bump。理由：与 `applySyncedStepAccount(_:)` 钦定一致 —— `roomNavigationGeneration` 只在 room 实际 navigation 时 bump（Story 12.7 r12 [P2] fix lesson `2026-05-11-apply-home-data-bump-only-on-room-id-change.md`），chest mutation 与 room flow 无关。

**对应 Tasks**: Task 3.1

### AC4 — `ChestRefreshTriggerService`（3 触发器 + 60s timer + in-flight gate）

**新建文件**：`iphone/PetApp/Features/Home/Services/ChestRefreshTriggerService.swift`

**职责**（与 `StepSyncTriggerService` 同模式但简化：本 story 无 `triggerManual` 需求，3 触发器即可）：

3 触发时机（epics.md AC 行 3049）：
1. App 启动后进入主界面（RootView `.onReadyTask` 调 `service.start()` 触发首次）
2. App 从后台回到前台（RootView `.onChange(of: scenePhase) .active` 触发）
3. 主界面停留期间**每 60 秒**定时拉取一次（`service.start()` 内启动 Task.sleep 循环）

**契约骨架**（与 `StepSyncTriggerService` 镜像，去掉 `triggerManual` 和 `motionState` 借用）：

```swift
// ChestRefreshTriggerService.swift
// Story 21.2 AC4: 宝箱状态拉取触发器服务（3 触发器 + in-flight gate + 60s 定时器）.
//
// 3 触发时机（epics.md AC 行 3049）:
//   1. App 启动后进入主界面（RootView .onReadyTask 调 service.start() 触发首次）
//   2. App 从后台回到前台（RootView .onChange(of: scenePhase) .active 触发）
//   3. 主界面停留期间每 60 秒定时拉取一次（service.start() 内启动 Task.sleep 循环）
//
// in-flight gate（与 StepSyncTriggerService 同模式）:
//   - currentRefreshTask Task 引用追踪：当前 refresh in-flight 时新触发被忽略（不排队）
//   - 失败不破坏 UI（背景拉取；下次定时器到达或 foreground 再试；与 SyncStepsUseCase 同精神）
//
// 与 StepSyncTriggerService 的差异（简化）:
//   - **不**实装 triggerManual（本 story 无 manual await 场景；Story 21.3 OpenChestUseCase 直接 await
//     POST /chest/open 响应，**不**经本 service 主动 refresh —— server 端响应已含 nextChest）
//   - **不**借用 HomeViewModel（chest refresh 无需 motionState；service 完全独立于 ViewModel）
//   - **不**接 reconnect alignment delegate（chest 不走 WS 推送，仅 REST 60s 拉取兜底）
//
// 生命周期（与 StepSyncTriggerService 100% 镜像）:
//   - 由 RootView 通过 @State 持有；与 RootView 同生命周期
//   - start() 由 RootView .onReadyTask 内调（启动 + 定时器循环）
//   - stop() 由 RootView .onChange(of: scenePhase) .background 边沿调
//   - deinit 时 cancel timer 防泄漏
//
// 性能 / 资源约束:
//   - Timer 周期 60 秒（epics.md AC 行 3049 钦定）—— 不可配置（YAGNI；prod 默认值锚定 epic AC）
//   - Timer 用 `Task.sleep(nanoseconds:)` 循环，不用 Foundation `Timer`
//     （@MainActor 友好 + 可被 cancel；与 Swift 6 strict concurrency 一致；与 StepSyncTriggerService 同选型）

import Foundation

@MainActor
public final class ChestRefreshTriggerService {

    // MARK: - Dependencies

    private let loadChestUseCase: LoadChestUseCaseProtocol

    // MARK: - State

    /// in-flight gate.
    /// 当前 refresh 进行中时新触发被忽略（不排队）；非 nil 表示 in-flight.
    private var currentRefreshTask: Task<Void, Never>?

    /// 定时器循环 task；start() 启动；stop() / deinit 取消.
    private var timerTask: Task<Void, Never>?

    /// 是否已启动定时循环（防 .scenePhase .active 多次触发重复启动 timer；与 StepSyncTriggerService 同模式）.
    private var hasStartedTimer = false

    /// Timer 周期：60 秒（epics.md AC 行 3049）.
    private static let timerIntervalNanos: UInt64 = 60 * 1_000_000_000

    // MARK: - Init

    public init(loadChestUseCase: LoadChestUseCaseProtocol) {
        self.loadChestUseCase = loadChestUseCase
    }

    // MARK: - Public API

    /// 启动触发器：启动 60 秒定时循环 + 触发首次拉取.
    /// 由 RootView .onReadyTask 在主界面就绪后调；幂等（多次调安全）.
    ///
    /// 与 StepSyncTriggerService.start() 同精神（codex review round 1 [P3] fix 锁定路径）：
    ///   - 首次调用：startTimerIfNeeded() 启动 timer + spawn 一次 launch refresh；
    ///   - 已 hasStartedTimer 的后续调用：等同 triggerForeground()（只 spawn 一次 reactivate refresh,
    ///     **不**重启 timer，避免老 timer 还在跑就启动新的）.
    /// 详见 docs/lessons/2026-05-04-scenephase-idempotent-start-no-duplicate-trigger.md.
    public func start() {
        let wasFirstStart = !hasStartedTimer
        startTimerIfNeeded()
        spawnRefreshIfIdle(reason: wasFirstStart ? .launch : .foreground)
    }

    /// 停止触发器：cancel 定时器循环.
    /// 由 RootView .onChange(of: scenePhase) .background 边沿调.
    /// **不**清 currentRefreshTask：让正在 in-flight 的 refresh 自然完成；下次 start() 时 currentRefreshTask 自然 nil.
    public func stop() {
        timerTask?.cancel()
        timerTask = nil
        hasStartedTimer = false
    }

    // MARK: - Private

    private enum RefreshReason: String {
        case launch
        case foreground
        case timer
    }

    /// fire-and-forget 路径：launch / foreground / timer 用.
    /// 若有 in-flight refresh 直接忽略（与 StepSyncTriggerService 同模式）.
    private func spawnRefreshIfIdle(reason: RefreshReason) {
        guard currentRefreshTask == nil else {
            return
        }
        let task: Task<Void, Never> = Task { @MainActor [weak self] in
            guard let self else { return }
            await self.runRefresh(reason: reason)
        }
        currentRefreshTask = task
    }

    /// 拉取 + 错误吞咽 + currentRefreshTask 自清.
    private func runRefresh(reason: RefreshReason) async {
        defer { currentRefreshTask = nil }

        do {
            try await loadChestUseCase.execute()
        } catch {
            // 失败不破坏 UI（epics.md AC 行 3053）；下次触发再试.
            // 节点 7 阶段不做 logger framework；失败被 silently 吞掉是 by design.
            // future: 接 logger 后此处 log warning（与 StepSyncTriggerService 同精神）.
            _ = reason  // 防 unused 编译警告
            _ = error
        }
    }

    private func startTimerIfNeeded() {
        guard !hasStartedTimer else { return }
        hasStartedTimer = true
        timerTask = Task { @MainActor [weak self] in
            while !Task.isCancelled {
                do {
                    try await Task.sleep(nanoseconds: ChestRefreshTriggerService.timerIntervalNanos)
                } catch {
                    return
                }
                guard !Task.isCancelled else { return }
                self?.spawnRefreshIfIdle(reason: .timer)
            }
        }
    }

    deinit {
        timerTask?.cancel()
    }
}
```

> **关键决策 1（service 不持 HomeViewModel）**：不持。理由：与 `StepSyncTriggerService` "option A" 不同（chest refresh 无 motionState 来源需求）；本 service 完全独立于 view 层，AppState 通过 LoadChestUseCase 写入后由 21.1 ChestTimerDriver 自动 react；service 只关心 "what to fetch / when to fetch"。

> **关键决策 2（60s timer 周期硬编码）**：常量。理由：与 `StepSyncTriggerService.timerIntervalNanos` 同精神（YAGNI；prod 默认值锚定 epic AC）；如未来需要 config 化再做演进。

> **关键决策 3（无 triggerManual）**：本 story 不需要。理由：Story 21.3 OpenChestUseCase 直接 await POST /chest/open，响应已含 nextChest（V1 §7.2 钦定 `data.nextChest` 5 字段必填）；无需在 open 前后手动调 GET refresh；触发器只服务 "launch + foreground + 60s 定时" 这三个被动场景。

> **关键决策 4（service stop 不清 in-flight Task）**：让 in-flight 自然完成。理由：与 `StepSyncTriggerService.stop()` 同模式（"让正在 in-flight 的 sync 自然完成"）；in-flight Task 完成后 defer 自清 currentRefreshTask；scenePhase .background 期间 in-flight 完成后 task slot 已空，下次 .active 调 start() 不会被 in-flight gate 短路。

> **关键决策 5（不调 ErrorPresenter）**：与 `SyncStepsUseCase` / `StepSyncTriggerService` 同精神。理由：背景拉取（60s timer + launch + foreground）失败不应弹 toast 打扰用户；用户主动操作（如点开箱按钮，Story 21.3）路径才弹 alert。

**对应 Tasks**: Task 4.1

### AC5 — `AppContainer` 新增 3 个 factory：`makeChestRepository` / `makeLoadChestUseCase` / `makeChestRefreshTriggerService`

**改动文件**：`iphone/PetApp/App/AppContainer.swift`

**关键改动**：在 `makeSyncPetStateUseCase` / `makePetStateSyncTriggerService` 后面新增同模式 3 个 factory（按 Story 21.x 分段）：

```swift
// MARK: - Story 21.2 AC5: Chest Refresh 链路 factory

/// Story 21.2 AC1: 构造 ChestRepository（每次调用返回新实例；apiClient 单例由 container 持有）.
/// 与 makeHomeRepository / makePetRepository 同模式（value type struct，构造廉价）.
public func makeChestRepository() -> ChestRepositoryProtocol {
    DefaultChestRepository(apiClient: apiClient)
}

/// Story 21.2 AC2: 构造 LoadChestUseCase（每次调用返回新实例；依赖 repository / appState）.
/// caller 必须传 appState（AppState 在 RootView 持有；不进 AppContainer 字段；ADR-0010 §3.1）.
/// 与 makeSyncStepsUseCase / makeSyncPetStateUseCase 同模式.
public func makeLoadChestUseCase(appState: AppState) -> LoadChestUseCaseProtocol {
    DefaultLoadChestUseCase(
        repository: makeChestRepository(),
        appState: appState
    )
}

/// Story 21.2 AC4: 构造 ChestRefreshTriggerService（**每次调用返回新实例**；caller 应自行通过 @State 持有
/// strong 引用，避免 RootView body 重建时重启 timer —— 与 makeStepSyncTriggerService 同模式）.
public func makeChestRefreshTriggerService(appState: AppState) -> ChestRefreshTriggerService {
    ChestRefreshTriggerService(
        loadChestUseCase: makeLoadChestUseCase(appState: appState)
    )
}
```

> **关键决策（不在 container 持有 service 单例）**：每次返回新实例。理由：与 `makeStepSyncTriggerService` / `makePetStateSyncTriggerService` 同模式 + 同 lesson 依据（`docs/lessons/2026-04-26-stateobject-debug-instance-aliasing.md`）—— RootView body 重建时若每次拿同一 service 实例会导致 timer 重启 + subscription 重复；改由 RootView 通过 @State 持有 strong 引用保 lifecycle 稳定。

**对应 Tasks**: Task 5.1

### AC6 — `RootView` wire `chestRefreshTriggerService`（与 `stepSyncTriggerService` 100% 镜像）

**改动文件**：`iphone/PetApp/App/RootView.swift`

**关键改动**：在 `stepSyncTriggerService` 字段 / `ensureStepSyncWired` / scenePhase handler 旁边新增同模式 chest 路径。

1. **新增 @State 字段**（紧邻 `stepSyncTriggerService` / `petStateSyncTriggerService`）：

```swift
/// Story 21.2 AC6: ChestRefreshTriggerService（持 3 触发器 + in-flight gate + 60s 定时器）.
/// 由 RootView @State 持有，避免 body 重建时 service 重启
/// （rebuild → factory 返回新 service → 旧 timer 仍在跑 → 资源泄漏；与 stepSyncTriggerService 同模式 / 同 lesson 依据）.
/// `nil` 守卫让 `ensureChestRefreshWired()` 仅初始化一次（与 ensureStepSyncWired 同模式）.
@State private var chestRefreshTriggerService: ChestRefreshTriggerService?
```

2. **新增 `ensureChestRefreshWired()` 方法**（紧邻 `ensureStepSyncWired` / `ensurePetStateSyncWired`）：

```swift
/// Story 21.2 AC6: lazy 注入 `chestRefreshTriggerService`（与 ensureStepSyncWired 同模式）.
private func ensureChestRefreshWired() {
    guard chestRefreshTriggerService == nil else { return }
    chestRefreshTriggerService = container.makeChestRefreshTriggerService(
        appState: appState
    )
}
```

3. **`.onReadyTask` 处调用** `ensureChestRefreshWired()` + `chestRefreshTriggerService?.start()`（紧邻 `stepSyncTriggerService?.start()` 调用点）：

```swift
// Story 21.2 AC6: lazy 注入 chestRefreshTriggerService（与 ensureStepSyncWired 同模式）.
ensureChestRefreshWired()

// ... 其它 service start 调用 ...

chestRefreshTriggerService?.start()
```

4. **`.onChange(of: scenePhase)` `.active` / `.background` 边沿处理**（紧邻既有 stepSync / petStateSync 调用）：

```swift
// scenePhase .active 边沿：
chestRefreshTriggerService?.start()

// scenePhase .background 边沿：
chestRefreshTriggerService?.stop()
```

> **关键决策 1（@State 而非 @StateObject）**：@State。理由：与 `stepSyncTriggerService` / `petStateSyncTriggerService` 同模式；service 是 `@MainActor final class` 但**不**是 ObservableObject（无 @Published 字段需 SwiftUI 观察）；@State 持有 strong 引用即可。

> **关键决策 2（wire 100% 镜像 stepSync 节奏）**：不发明新位置。理由：lesson `2026-04-26-stateobject-debug-instance-aliasing.md` + `2026-05-04-scenephase-idempotent-start-no-duplicate-trigger.md` 已锁 service lifecycle race 边界；新 service 按相同位置 wire，保持 review 易读 + 减少新出 race 风险面。

> **关键决策 3（不在 LaunchedContentView 内 wire）**：在 RootView 外层 wire。理由：与 `stepSyncTriggerService` 同位置（详见 RootView line 107-111 / 447-449 / 267 / 289 / 491-492 / 513）；service 生命周期与 RootView 同等（跨 LaunchedContentView 重建仍稳定）。

**对应 Tasks**: Task 6.1, 6.2, 6.3, 6.4

### AC7 — 单元测试 ≥5 case（纯 XCTest + mocked Repository + AppState + fake clock）

**新建文件**：
- `iphone/PetAppTests/Features/Home/Repositories/ChestRepositoryTests.swift`（与 HomeRepositoryTests 同模式 ≥2 case）
- `iphone/PetAppTests/Features/Home/UseCases/LoadChestUseCaseTests.swift`（与 LoadHomeUseCaseTests 同模式 ≥4 case）
- `iphone/PetAppTests/Features/Home/Services/ChestRefreshTriggerServiceTests.swift`（与 StepSyncTriggerServiceTests 同模式 ≥3 case）

**LoadChestUseCase 测试 cases**（≥5 case；epic AC 行 3054-3059 钦定 5 case）：

| # | 场景 | 断言 |
|---|---|---|
| 1 | happy: status=1 counting → 写 AppState | `appState.currentChest?.status == .counting` + `unlockAt` / `remainingSeconds` 一一对应 |
| 2 | happy: status=2 unlockable → 写 AppState | `appState.currentChest?.status == .unlockable` |
| 3 | happy: 重复 execute 累加调 repo | `mock.callCount(of: "fetchCurrent()") == 2` 且 AppState 两次写入 |
| 4 | edge: APIError.business(4001) → throw + AppState 保留旧值 | 断言 throw + `appState.currentChest` 仍是 setUp 设置的初始值 |
| 5 | edge: APIError.network → throw + AppState 保留旧值 | 断言 throw + AppState 不变 |
| 6 | edge: 未知 status=99 → APIError.decoding(HomeDataDecodingError.unknownChestStatus(99)) | 与 LoadHomeUseCaseTests.testExecuteUnknownChestStatusThrowsDecoding 同精神 |

**ChestRefreshTriggerService 测试 cases**（≥3 case，与 StepSyncTriggerServiceTests 同精神：fake UseCase + 验证 launch / foreground / timer 触发逻辑 + in-flight gate）：

| # | 场景 | 断言 |
|---|---|---|
| 1 | happy: start() 触发首次 launch refresh | `mockUseCase.callCount(of: "execute()") == 1`（用 expectation 等异步完成） |
| 2 | happy: 已 hasStartedTimer 后再 start() 等同 foreground 触发 | 第二次 start() 只 +1 次 execute，不重启 timer |
| 3 | edge: in-flight gate —— start() 时 UseCase 未完成 + 再次 start() 被忽略 | `mockUseCase.callCount(of: "execute()") == 1`（gate 短路） |
| 4 | edge: UseCase throw → silently 吞 + 下次 start 重试 | 第二次 start 时 +1 次 execute |
| 5 | happy: stop() 取消 timer + 不清 in-flight | timerTask 已 nil + hasStartedTimer false |

**MockChestRepository / MockLoadChestUseCase**（继承 MockBase；与 MockHomeRepository / MockLoadHomeUseCase 同模式；详见 `iphone/PetAppTests/Helpers/MockBase.swift` 注释 line 15 已预留 chest mock 占位）：

```swift
final class MockChestRepository: MockBase, ChestRepositoryProtocol, @unchecked Sendable {
    var fetchCurrentStub: Result<ChestCurrentResponse, Error>?

    func fetchCurrent() async throws -> ChestCurrentResponse {
        recordCall("fetchCurrent()")
        switch fetchCurrentStub {
        case .success(let resp): return resp
        case .failure(let err): throw err
        case nil:
            XCTFail("fetchCurrentStub not set")
            throw APIError.network(underlying: URLError(.unknown))
        }
    }
}

final class MockLoadChestUseCase: MockBase, LoadChestUseCaseProtocol, @unchecked Sendable {
    var executeStub: Result<Void, Error> = .success(())
    var executeDelay: UInt64 = 0  // ns —— 用于测 in-flight gate

    func execute() async throws {
        recordCall("execute()")
        if executeDelay > 0 {
            try? await Task.sleep(nanoseconds: executeDelay)
        }
        switch executeStub {
        case .success: return
        case .failure(let err): throw err
        }
    }
}
```

> **关键决策 1（不在 UseCase 单测内验 ChestTimerDriver react）**：分层验证。理由：UseCase 单测只断言 `AppState.currentChest` 字段写入正确；driver react 是 Story 21.1 已落地的 Combine sink 行为，已被 21.1 ChestCardViewTests / ChestTimerDriverTests 覆盖；本 story 不重复验证（YAGNI）。

> **关键决策 2（service 单测用 XCTestExpectation 等异步完成而非 sleep 60s）**：用 expectation。理由：(a) Task 异步触发的 UseCase.execute 需 expectation.fulfill() 同步；(b) Timer 60s 间隔在单测中不可等（用 service 私有方法注入或测 mock UseCase 调用次数）；(c) 与 StepSyncTriggerServiceTests / SyncStepsUseCaseTests 既有测试模式一致。

> **关键决策 3（不测 60s timer 实际 fire）**：单测断言 timer Task 已启动 + 不重复启动。理由：测真实 60s 流逝违反单测原则；timer 行为已经被 `Task.sleep` API 契约 + 既有 StepSyncTriggerServiceTests 模式覆盖；如需端到端验证 timer 跑通，走集成测试（AC8）路径。

**对应 Tasks**: Task 7.1, 7.2, 7.3

### AC8 — 集成测试（XCUITest + mock server / Mock UseCase 路径）

**新建/扩充文件**：`iphone/PetAppUITests/ChestRefreshUITests.swift`（与 HomeUITests / RoomUITests 同模式）

**测试场景**（epic AC 行 3060 钦定）：

1. **launch → ChestCardView 显示 server 返回状态**：通过 launch args / env 注入 mock chest 响应（与 21.1 HomeUITests `--uitest-skip-guest-login` 路径同精神），断言 `chestCard_counting` accessibility identifier 可见 + remainingSeconds 文本与 mock 数据一致；
2. **foreground 重新拉取**：UITest 触发 background → foreground（`XCUIDevice.shared.press(.home)` + 重新 activate），断言 ChestCardView 仍按 mock 响应渲染（不依赖真实 60s timer fire，仅验证 trigger wire 正确）。

**实装策略**（与 21.1 HomeUITests / Story 20.x 集成测试同模式）：

- 复用 `--uitest-skip-guest-login` + 既有 mock 路径；
- 若需独立 mock chest endpoint，扩展 AppContainer DEBUG init 接受 `uiTestMockChestRepository: ChestRepositoryProtocol?` 参数（与 `uiTestMockStepRepository` 同模式 line 223），launch args 触发时注入 mock 返回固定 chest 状态；
- UITest assertion 用 `XCUIApplication().otherElements["chestCard_counting"]` / `.staticTexts` 验证 a11y identifier + 文本。

> **关键决策 1（不真实跑 60s timer）**：UITest 不等 60s。理由：单测已覆盖 timer Task 启动逻辑；UITest 只验证 launch / foreground 触发 + UI 渲染正确；YAGNI 不引 mock-clock 复杂度。

> **关键决策 2（mock chest repo 注入路径）**：扩展 AppContainer DEBUG init。理由：与既有 `uiTestMockStepRepository` 同模式 + 同 lesson 依据（避免 launch args 路径侵入 production code）。

**对应 Tasks**: Task 8.1

### AC9 — `xcodegen` 同步 + `bash iphone/scripts/build.sh` 通过 + ios-simulator MCP UI 实跑验证

**关键改动**：

1. 新增的 5 个 .swift 文件加入 `iphone/project.yml` `sources` glob（实际上 PetApp / PetAppTests / PetAppUITests target sources 用 glob recursive 自动包含，无需手动编辑 project.yml；只要文件落在正确目录下）；
2. 跑 `iphone/scripts/xcodegen.sh`（或 `xcodegen generate` 直接调）让 .xcodeproj 同步；
3. 跑 `bash iphone/scripts/build.sh` 验证 build pass；
4. 跑 `bash iphone/scripts/build.sh --test` 验证单测 pass；
5. **ios-simulator MCP UI 实跑验证**（CLAUDE.md "iOS UI 验证" 段钦定的标准 verify workflow）：
   - `bash iphone/scripts/build.sh` → `install_app` → `launch_app(terminate_running: true)` → `ui_view` → 验证 ChestCardView counting 态 mm:ss 倒计时显示 + 一秒后数字 -1（21.1 driver + 本 story refresh 联动）→ 后台 → 前台触发 foreground 拉取（断言 UI 不破坏，与 launch 状态一致）。

> **关键决策（必须跑 ios-simulator MCP UI 验证）**：CLAUDE.md 钦定 "iOS UI / feature 改动必须用 ios-simulator MCP server 在模拟器里实跑验证，不能只跑 build.sh 就报告 done"。Story 37.x 多次出现 build pass 但视觉 bug / 交互 bug 漏到 review 阶段才发现。

**对应 Tasks**: Task 9.1, 9.2, 9.3

### AC10 — Deliverable 清单（≥10 文件改动）

新建文件：

1. `iphone/PetApp/Features/Home/UseCases/ChestEndpoints.swift`
2. `iphone/PetApp/Features/Home/Repositories/ChestRepository.swift`
3. `iphone/PetApp/Features/Home/Models/ChestCurrentResponse.swift`
4. `iphone/PetApp/Features/Home/UseCases/LoadChestUseCase.swift`
5. `iphone/PetApp/Features/Home/Services/ChestRefreshTriggerService.swift`
6. `iphone/PetAppTests/Features/Home/Repositories/ChestRepositoryTests.swift`
7. `iphone/PetAppTests/Features/Home/UseCases/LoadChestUseCaseTests.swift`
8. `iphone/PetAppTests/Features/Home/Services/ChestRefreshTriggerServiceTests.swift`
9. `iphone/PetAppUITests/ChestRefreshUITests.swift`（可选；若复用 HomeUITests 则不新增）

改动文件：

10. `iphone/PetApp/App/AppState.swift`（新增 `applyCurrentChest(_:)` 方法）
11. `iphone/PetApp/App/AppContainer.swift`（新增 3 个 factory）
12. `iphone/PetApp/App/RootView.swift`（新增 @State + ensureChestRefreshWired + scenePhase wire）
13. `iphone/PetAppTests/Helpers/MockBase.swift`（如需新增 chest mock 模板示例，与既有 line 15 占位一致）
14. `iphone/PetAppTests/App/AppStateTests.swift`（新增 `applyCurrentChest` mutation 测试 case）

**对应 Tasks**: Task 10.1

## Tasks / Subtasks

- [x] **Task 1: Wire 协议 + Repository 层（AC1）**
  - [x] 1.1 新建 `ChestEndpoints.swift` —— path `/api/v1/chest/current` + method .get + requiresAuth=true
  - [x] 1.2 新建 `ChestCurrentResponse.swift` —— 5 字段 wire DTO（id / status / unlockAt / openCostSteps / remainingSeconds）
  - [x] 1.3 新建 `ChestRepository.swift` —— protocol + DefaultChestRepository struct

- [x] **Task 2: LoadChestUseCase 编排层（AC2）**
  - [x] 2.1 新建 `LoadChestUseCase.swift` —— execute() async throws：fetchCurrent → DTO 转 HomeChest（未知 status fail-fast 抛 .decoding）→ MainActor.run { appState.applyCurrentChest(_:) }

- [x] **Task 3: AppState mutation 入口（AC3）**
  - [x] 3.1 在 `AppState.swift` 内 `applySyncedStepAccount(_:)` 旁边新增 `applyCurrentChest(_ chest: HomeChest)` 方法 —— 单字段写 currentChest，不 bump roomNavigationGeneration

- [x] **Task 4: ChestRefreshTriggerService（AC4）**
  - [x] 4.1 新建 `ChestRefreshTriggerService.swift` —— @MainActor final class，3 触发器 + 60s timer + in-flight gate + deinit cancel timer；start() 幂等（首次启动 + reactivate 都安全）

- [x] **Task 5: AppContainer factory（AC5）**
  - [x] 5.1 在 `AppContainer.swift` 内 `makePetStateSyncTriggerService` 后新增 3 个 factory：`makeChestRepository` / `makeLoadChestUseCase(appState:)` / `makeChestRefreshTriggerService(appState:)`

- [x] **Task 6: RootView wire（AC6）**
  - [x] 6.1 新增 `@State private var chestRefreshTriggerService: ChestRefreshTriggerService?`（紧邻 stepSyncTriggerService）
  - [x] 6.2 新增 `ensureChestRefreshWired()` 方法（紧邻 ensureStepSyncWired / ensurePetStateSyncWired）
  - [x] 6.3 `.onReadyTask` 内调 `ensureChestRefreshWired()` + `chestRefreshTriggerService?.start()`（紧邻 stepSync / petStateSync 既有 wire 点）
  - [x] 6.4 `.onChange(of: scenePhase)` `.active` 调 `chestRefreshTriggerService?.start()`、`.background` 调 `chestRefreshTriggerService?.stop()` + onLeaveReady 同 stop（紧邻 stepSync / petStateSync 既有路径）

- [x] **Task 7: 单元测试（AC7，≥5 case）**
  - [x] 7.1 新建 `ChestRepositoryTests.swift` —— 5 case（happy endpoint shape + 5 字段透传 + business / network / unauthorized 错误透传）
  - [x] 7.2 新建 `LoadChestUseCaseTests.swift` —— 6 case（status=1 / status=2 / 重复 execute / business 4001 throw / network throw / 未知 status fail-fast）
  - [x] 7.3 新建 `ChestRefreshTriggerServiceTests.swift` —— 6 case（start 首次触发 / start 幂等 / in-flight gate / 失败不阻塞 / stop cancel timer / stop-start rebind）
  - [x] 7.4 扩充 `AppStateTests.swift` —— 新增 3 case（applyCurrentChest 单字段 + 不 bump generation + 空 AppState 写入）

- [x] **Task 8: 集成测试（AC8）**
  - [x] 8.1 新建 `ChestRefreshUITests.swift` —— 2 case 验证 launch → ChestCardView 渲染 chestCard_counting 锚 + 1.5s 后 UI 不破坏（counting 态稳定）

- [x] **Task 9: Build + 模拟器实跑（AC9）**
  - [x] 9.1 跑 `bash iphone/scripts/build.sh` 确认 build pass
  - [x] 9.2 跑 `bash iphone/scripts/build.sh --test` 确认 704 单测全 pass（含本 story 新增 20 case）
  - [x] 9.3 ios-simulator MCP UI 实跑：install_app + launch_app + ui_view 验证 counting 倒计时显示（09:52 → 08:10 → 07:56 driver tick 工作）+ ui_describe_all 验 chestCard_counting a11y 锚 + AppState hydrate 链路 OK

- [x] **Task 10: Deliverable 清单核对（AC10）**
  - [x] 10.1 核对 14 文件新增/改动（≥10），本 story commit 内不夹带其它无关改动

## Dev Notes

### 架构对齐

- **iOS 工程结构（`docs/宠物互动App_iOS客户端工程结构与模块职责设计.md`）**：本 story 落地在 `Features/Home/` 内（Endpoints / Repositories / UseCases / Services / Models 五分层），与既有 Home / Pet / Step / Emoji 链路同模式。
- **ADR-0010 §3.1 / §3.2**：domain state 单 source of truth 是 AppState，ViewModel 仅持 view-state（chestRemainingSeconds 已在 21.1 落地于 HomeViewModel）。本 story `LoadChestUseCase` 写 AppState；`ChestRefreshTriggerService` 不读不写 ViewModel。
- **ADR-0010 §3.3**：hydrate / mutation 入口前缀 `apply*`；本 story 新增 `applyCurrentChest(_:)` 与 `applyHomeData` / `applySyncedStepAccount` 同模式。

### 错误处理边界

- **APIError 三层映射**（与 Server 端 ADR-0001 § 错误三层映射框架同精神）：repo 层透传 → UseCase 层透传 → ChestRefreshTriggerService silently 吞（背景拉取场景；与 SyncStepsUseCase + StepSyncTriggerService 同精神，详见 `docs/lessons/2026-04-27-transient-vs-terminal-error-classification.md`）。
- **未知 chest.status fail-fast**：与 Story 5.5 round 6 [P2] fix 钦定一致；详见 `docs/lessons/2026-04-27-home-data-fail-fast-on-unknown-enum.md`。
- **4001 错误码**：V1 §7.1 钦定 "用户在 user_chests 表中无任何行（理论上 Story 4.6 登录初始化必然创建首个 chest，此错误表征数据完整性异常）"；本 story 不在 UseCase 内特殊处理，统一走 silently 吞路径（保留 AppState 上次值；UI 显示上次状态或 EmptyView）。

### 与 21.1 接缝

- **ChestTimerDriver 自动 react**：21.1 已落地 driver 订阅 `appState.$currentChest` Combine sink；本 story `LoadChestUseCase` 写 AppState 后 driver 自动重启 timer，重新计算 chestRemainingSeconds，21.1 ChestCardView 自动 rerender。**本 story 不需要改 driver 任何代码**。
- **倒计时归零的两条路径**：(a) 21.1 driver 本地倒计时归零 → ChestCardView 视觉切 unlockable（domain 状态仍 counting）；(b) 本 story 60s timer 拉取 server → 若 server 端 `unlock_at <= now` 则 V1 §7.1 服务端逻辑步骤 3 返回 `status = 2` → AppState 写入 → driver react → 21.1 ChestCardView 同样切 unlockable（domain 状态对齐 server）。两条路径 UI 表现一致，domain 状态在最迟 60s 内对齐 server。

### 性能与生命周期

- **60s timer 周期**：epic AC 行 3049 钦定（"每 60 秒定时调用"）；不可配置；与 V1 §7.1 关键约束 line 913 "client 应在 remainingSeconds 倒到 0 时主动调一次 GET /chest/current" 协同（本 story timer 兜底 + 21.1 ChestCardView 视觉切派生协同：60s 内必有一次校准）。
- **service lifecycle**：与 stepSyncTriggerService / petStateSyncTriggerService 100% 镜像，详见 lesson `2026-04-26-stateobject-debug-instance-aliasing.md` + `2026-05-04-scenephase-idempotent-start-no-duplicate-trigger.md`。
- **deinit cancel timer**：防 RootView 销毁后 timer Task 仍在跑（与 StepSyncTriggerService line 244 同模式）。

### 测试边界

- **单测不依赖真实 60s 流逝**：用 mock UseCase + XCTestExpectation 验证 trigger wire 正确；timer 60s 行为信任 Task.sleep + 既有 StepSyncTriggerServiceTests 模式。
- **集成测试 (UITest) 不真实跑 60s timer**：仅验证 launch / foreground 触发链路 + UI 渲染正确。
- **MockBase 已落地**：见 `iphone/PetAppTests/Helpers/MockBase.swift`；新加 MockChestRepository / MockLoadChestUseCase 与既有 MockHomeRepository / MockLoadHomeUseCase 同模式。

### 命名规则

- **service 名 `ChestRefreshTriggerService`**（而非 `LoadChestTriggerService` 或 `ChestSyncTriggerService`）：理由：(a) "Refresh" 表达 "周期性主动拉取最新状态" 语义（与 V1 §7.1 关键约束 "client 应在 remainingSeconds 倒到 0 时主动重新 GET 一次以确认 server 端 status 已切换" 一致）；(b) 区分于 SyncStepsUseCase 的双向 "sync"（client → server 写 + server → client 读）—— chest 是单向 read-only；(c) 与 V1 §3 错误码 / domain 术语对齐。

### Project Structure Notes

- **对齐 `iphone` 工程结构（`docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §3）**：
  - Repository → `Features/Home/Repositories/`
  - UseCase → `Features/Home/UseCases/`
  - Endpoints 工厂 → `Features/Home/UseCases/`（与既有 HomeEndpoints / StepsEndpoints 同位置）
  - Service（持续行为，与 UseCase 单次执行区分）→ `Features/Home/Services/`
  - Wire DTO → `Features/Home/Models/`
  - 测试镜像生产路径 → `PetAppTests/Features/Home/{Repositories,UseCases,Services}/`
- **xcodegen 同步**：iphone target sources 用 glob recursive，无需手动编辑 project.yml；新建文件落入正确目录后跑 `xcodegen generate` 即可。
- **PetAppUITests target**：UITest 文件放 `iphone/PetAppUITests/`（与 HomeUITests / RoomUITests 同位置）。

### References

- [Source: docs/宠物互动App_V1接口设计.md#7.1 GET /api/v1/chest/current] —— 接口定义 + 状态动态判定 + 关键约束 + 错误码
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#3 五分层] —— Feature 内子目录 (Repositories / UseCases / Services / Models / Views / ViewModels)
- [Source: _bmad-output/implementation-artifacts/decisions/0010-app-state.md#3.1] —— ViewModel 禁止 @EnvironmentObject + 构造注入 AppState
- [Source: _bmad-output/implementation-artifacts/decisions/0010-app-state.md#3.3] —— hydrate / mutation 入口 `apply*` 前缀
- [Source: _bmad-output/planning-artifacts/epics.md#3031-3060] —— Story 21.2 AC 原文（含 2026-05-04 addendum 钦定 LoadChestUseCase 写 AppState）
- [Source: _bmad-output/implementation-artifacts/21-1-首页宝箱组件-swiftui.md] —— Story 21.1 落地的 ChestTimerDriver / HomeViewModel.chestRemainingSeconds / ChestCardView 接缝契约
- [Source: iphone/PetApp/Features/Home/UseCases/SyncStepsUseCase.swift] —— 同模式 UseCase 写 AppState 单字段（applySyncedStepAccount）
- [Source: iphone/PetApp/Features/Home/UseCases/LoadHomeUseCase.swift] —— 同模式 UseCase + 错误透传 + DTO 转 domain fail-fast
- [Source: iphone/PetApp/Features/Home/Services/StepSyncTriggerService.swift] —— 同模式 Service（3 触发器 + in-flight gate + timer Task.sleep 循环 + scenePhase wire）
- [Source: iphone/PetApp/Features/Home/Repositories/HomeRepository.swift] —— 同模式 Repository（value type struct + apiClient.request）
- [Source: iphone/PetApp/App/AppState.swift#applySyncedStepAccount] —— 同模式 mutation 入口
- [Source: iphone/PetApp/App/AppContainer.swift#makeStepSyncTriggerService] —— 同模式 factory
- [Source: iphone/PetApp/App/RootView.swift#ensureStepSyncWired] —— 同模式 RootView wire
- [Source: docs/lessons/2026-04-27-home-data-fail-fast-on-unknown-enum.md] —— 未知 enum 值 fail-fast 抛 .decoding 路径
- [Source: docs/lessons/2026-04-26-stateobject-debug-instance-aliasing.md] —— 为何 service 用 @State 而非每次从 factory 拿
- [Source: docs/lessons/2026-05-04-scenephase-idempotent-start-no-duplicate-trigger.md] —— start() 幂等避免 scenePhase reactivate 触发重复 sync
- [Source: docs/lessons/2026-04-27-transient-vs-terminal-error-classification.md] —— 背景拉取失败 silently 吞的边界
- [Source: CLAUDE.md#iOS UI 验证] —— ios-simulator MCP 必跑 verify workflow
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.1] —— XCTest only，禁止 SnapshotTesting / ViewInspector

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) via bmad-dev-story workflow.

### Debug Log References

- 单元测试：`bash iphone/scripts/build.sh --test` → Executed 704 tests, 0 failures（含本 story 新增 20 case：ChestRepositoryTests 5 + LoadChestUseCaseTests 6 + ChestRefreshTriggerServiceTests 6 + AppStateTests 新增 3）.
- 构建验证：`bash iphone/scripts/build.sh` → BUILD SUCCEEDED.
- iOS 模拟器实跑（AC9）：iPhone 17 sim + UITEST_SKIP_GUEST_LOGIN + UITEST_CHEST_COUNTING → 截图验证 09:52 → 08:10 → 07:56 mm:ss 倒计时 driver tick 正常 + ChestCardView counting 态完整渲染（box 图标 + "宝箱倒计时" 标签 + mm:ss Text）.
- 已知问题（**非本 story 引入**）：Story 21.1 ChestCardView 内 `home_chestRemaining` a11y identifier 被父级 `chestCard_counting` 覆盖（SwiftUI `.accessibilityIdentifier` + `.accessibilityElement(children: .contain)` 父子布局行为）—— ui_describe_all 实测 mm:ss Text 的 AXUniqueId 回报为 `chestCard_counting` 而非 `home_chestRemaining`. 本 story UITest 用 `chestCard_counting` outer 锚验证整体渲染，不依赖 inner identifier；视觉验证由 ios-simulator MCP 截图覆盖. inner a11y 修复属 Story 21.1 范畴（pre-existing test `HomeUITests.testChestCardShowsCountingAndUnlockableAnchors` 同样命中此 bug）.

### Completion Notes List

- **新建 5 个生产文件 + 1 改 + 1 改 + 1 改 = 共 8 个生产代码改动**:
  - `ChestEndpoints.swift`（AC1，path 工厂）
  - `ChestCurrentResponse.swift`（AC1，wire DTO）
  - `ChestRepository.swift`（AC1，protocol + DefaultChestRepository struct）
  - `LoadChestUseCase.swift`（AC2，UseCase：repo → DTO 转 HomeChest → MainActor.run { appState.applyCurrentChest } + 未知 status fail-fast 抛 .decoding）
  - `ChestRefreshTriggerService.swift`（AC4，@MainActor final class：3 触发器 + 60s timer + in-flight gate + deinit cancel + start() 幂等模式与 StepSyncTriggerService 100% 镜像）
  - `AppState.swift`（AC3 改：新增 `applyCurrentChest(_:)` 单字段 mutation 入口，**不** bump roomNavigationGeneration）
  - `AppContainer.swift`（AC5 改：新增 3 factory makeChestRepository / makeLoadChestUseCase(appState:) / makeChestRefreshTriggerService(appState:)）
  - `RootView.swift`（AC6 改：新增 @State chestRefreshTriggerService 字段 + ensureChestRefreshWired() + 4 处 wire（onAppear 注入 + onReadyTask start + onLeaveReady stop + scenePhase active/background）

- **新建 6 个测试文件 = 共 6 个测试改动**:
  - `MockChestRepository.swift`（测试 helper，scripted Result + invocations count）
  - `ChestRepositoryTests.swift`（5 case：endpoint shape + 5 字段透传 + business / network / unauthorized 错误透传）
  - `LoadChestUseCaseTests.swift`（6 case：status=1 / status=2 / 重复 execute / business 4001 throw + AppState 保留旧值 / network throw + 保留旧值 / 未知 status fail-fast + 保留旧值）
  - `ChestRefreshTriggerServiceTests.swift`（6 case：start 首次 launch refresh / start 幂等等同 foreground / in-flight gate / 失败不阻塞下次 / stop cancel timer / stop-start rebind）
  - `AppStateTests.swift`（改：新增 3 case applyCurrentChest 单字段 mutation + 不 bump roomNavigationGeneration + 空 AppState 写入）
  - `ChestRefreshUITests.swift`（2 case：launch 渲染 chestCard_counting + 1.5s 后稳定）

- **设计要点 / 与既有链路对齐**:
  - Wire 模式：Endpoints / Repository / UseCase / Service / wire DTO 五层与既有 Home / Pet / Step 链路 100% 同构 → review 易读 + 减少 wire 新增的 race 风险面.
  - service lifecycle：与 stepSyncTriggerService / petStateSyncTriggerService 100% 镜像（@State 持有 + ensure*Wired + onReadyTask start + onLeaveReady stop + scenePhase active/background）→ 复用既有 lesson (`2026-04-26-stateobject-debug-instance-aliasing.md` / `2026-05-04-scenephase-idempotent-start-no-duplicate-trigger.md` / `2026-05-04-launch-state-leave-ready-must-stop-feature-services.md`).
  - 错误处理：repo / UseCase 透传 → service silently 吞（背景拉取场景；与 SyncStepsUseCase + StepSyncTriggerService 同精神）.
  - 未知 chest.status fail-fast 抛 .decoding（与 Story 5.5 round 6 [P2] fix 钦定一致；lesson `2026-04-27-home-data-fail-fast-on-unknown-enum.md`）.

- **HALT / 异常**: 无.

### File List

**新建（生产 5 + 测试 6 = 11）**:
- `iphone/PetApp/Features/Home/UseCases/ChestEndpoints.swift`
- `iphone/PetApp/Features/Home/Repositories/ChestRepository.swift`
- `iphone/PetApp/Features/Home/Models/ChestCurrentResponse.swift`
- `iphone/PetApp/Features/Home/UseCases/LoadChestUseCase.swift`
- `iphone/PetApp/Features/Home/Services/ChestRefreshTriggerService.swift`
- `iphone/PetAppTests/Features/Home/Repositories/MockChestRepository.swift`
- `iphone/PetAppTests/Features/Home/Repositories/ChestRepositoryTests.swift`
- `iphone/PetAppTests/Features/Home/UseCases/LoadChestUseCaseTests.swift`
- `iphone/PetAppTests/Features/Home/Services/ChestRefreshTriggerServiceTests.swift`
- `iphone/PetAppUITests/ChestRefreshUITests.swift`

**改动（生产 3 + 测试 1 = 4）**:
- `iphone/PetApp/App/AppState.swift`（新增 applyCurrentChest(_:) 方法）
- `iphone/PetApp/App/AppContainer.swift`（新增 3 个 factory）
- `iphone/PetApp/App/RootView.swift`（新增 @State chestRefreshTriggerService 字段 + ensureChestRefreshWired() + 4 处 wire）
- `iphone/PetAppTests/App/AppStateTests.swift`（新增 3 个 applyCurrentChest test case）

**Sprint Tracking 改动**:
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（21-2 status: ready-for-dev → in-progress → review）
- `_bmad-output/implementation-artifacts/21-2-get-chest-current-调用-状态展示-主动定时纠正.md`（story file Status + Tasks/Subtasks + Dev Agent Record + File List + Change Log）

## Change Log

| Date | Change | Reason |
|------|--------|--------|
| 2026-05-15 | Story 21.2 实装完成 → status review | dev-story workflow 完整跑红绿循环 + 实装 + 测试通过 + iOS 模拟器 MCP UI 实跑验证 |
