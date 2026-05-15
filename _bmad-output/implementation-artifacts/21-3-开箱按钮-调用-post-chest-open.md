# Story 21.3: 开箱按钮 + 调用 POST /chest/open（含 idempotencyKey 生成）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want 点开箱按钮后 App 调用 server 完成开箱，并防止重复开箱,
so that 我不会因为多点几下被多次扣步数（同一次开箱 idempotencyKey 不变，按钮 disabled 期间网络抖动重试也安全）.

## 故事定位（Epic 21 第 3 条 story；接 21.1 落地的 unlockable 按钮 + 21.2 落地的 server 权威态 → 加上 OpenChestUseCase 写 AppState 闭环）

这是 Epic 21「iOS - 首页宝箱倒计时 + 奖励弹窗」第 3 条 story —— 在 21.1 已落地的 `ChestCardView.unlockableView` 内 "开宝箱" `PrimaryButton`（`onOpenTap` closure 占位空闭包）+ 21.2 已落地的 `LoadChestUseCase` / `ChestRefreshTriggerService`（让 `AppState.currentChest` 60s 内对齐 server）基础上，**新增完整开箱链路**：

1. **`ChestOpenRequest` + `ChestOpenResponse` wire DTO**（V1 §7.2 已冻结 schema：request 单字段 `idempotencyKey: string`；response 嵌套三段 `reward / stepAccount / nextChest`，与 §7.2 字段表 + 21.4 `RewardPopupView` 需要的所有字段对齐）；
2. **`ChestOpenEndpoints`**（POST /api/v1/chest/open，与 `StepsEndpoints.sync` 同模式 + `requiresAuth=true`）；
3. **`ChestRepository.openChest(_:)`** 方法扩展（既有 protocol + struct 内追加方法；与 `StepRepository.syncSteps(_:)` 同模式 —— protocol 加一个 method、struct 加 forwarding impl，不新建 repository）；
4. **`OpenChestUseCase`**：业务编排 = 客户端生成 idempotencyKey → repo.openChest → DTO 转 domain → 写 `appState.currentChest`（用既有 `applyCurrentChest`）+ `appState.currentStepAccount`（用既有 `applySyncedStepAccount`）+ 返回 `ChestRewardSnapshot` 给 caller（让 21.4 弹窗用）；
5. **`IdempotencyKeyGenerator`**（小工具协议 + 默认 UUID v4 impl）：让 UseCase 单测可注入固定 key 验证"同一次点击复用同 key"；命名 + 文件位置参照 `DateProvider` / `MotionProvider` 同模式；
6. **`HomeViewModel.isOpening`** 瞬时 view-state @Published Bool（与 21.1 `chestRemainingSeconds` 同精神：transient 不上 AppState，ADR-0010 §3.2 "Loading flag → ViewModel @Published"钦定）；
7. **`HomeViewModel.onChestOpenTap()`** abstract method（基类 fatalError；Mock / Real 子类各自 override）：触发 OpenChestUseCase + 管 isOpening flag + 失败接 ErrorPresenter；
8. **`RealHomeViewModel.onChestOpenTap`** 真实 override：调 OpenChestUseCase + 业务错误码 case-by-case 文案（4002 / 3002 / 1005 / 1009）+ 其他错误透传 ErrorPresenter（与 `onJoinRoomConfirm` 6001-6005 mapper 同精神）；
9. **`HomeContainerView.chestSlot` 替换 onOpenTap 占位**：从 `{}` 改为 `{ homeViewModel.onChestOpenTap() }`；ChestCardView 按 `isOpening` 把按钮 disabled + 加 ProgressView（与 21.1 按钮契约 zero edit —— 仅传入 `isOpening` prop）；
10. **`AppContainer` 加 `makeOpenChestUseCase(appState:)` factory** + RealHomeViewModel.bind 新增 `openChestUseCase` + 注入路径；
11. **单元测试 ≥6 case**（mocked Repository + AppState + IdempotencyKeyGenerator + ErrorPresenter）+ **集成测试 ≥1 case**（UITest mock server happy path 触发 → 验证按钮转 disabled → reward 数据写到 ViewModel transient 字段供 21.4 picky）。

**本 story 落地后立即解锁**：

- **Story 21.4 奖励弹窗**：OpenChestUseCase 成功路径返回 `ChestRewardSnapshot`，21.4 `RewardPopupView` 通过 ViewModel transient `pendingReward` 字段订阅触发（本 story AC7 钦定 transient field name `pendingReward: ChestRewardSnapshot?`，21.4 view sheet 双向绑定 `$pendingReward.isPresented` 模式）；
- **Story 21.5 开箱前主动同步步数**：21.5 修改 OpenChestUseCase 让其在 repo.openChest 之前先 `await stepSyncTriggerService.triggerManual()`（已在 Story 8.5 落地的 public API）—— 本 story OpenChestUseCase 的构造签名预留 `stepSyncTriggerService` 注入位（默认参数 nil，本 story 不调；21.5 落地时改默认传 service 实例）；
- **节点 7 demo（Epic 22）**：本 story + 21.4 + 21.5 完工后构成节点 7 iOS 端完整开箱链路（GET → 倒计时 → POST → 弹窗）。

**关键路径（与 21.2 LoadChestUseCase / 8.5 SyncStepsUseCase 同精神，"action UseCase → 写 AppState 单字段 → driver/UI react"）**：

- UseCase 不持 ViewModel 引用（与 SyncStepsUseCase 同精神）；写入 AppState 后 21.1 ChestTimerDriver 自动 react `nextChest` → ChestCardView 立即重启倒计时；
- "按钮 disabled + loading" 状态完全由 `HomeViewModel.isOpening: Bool` 驱动（view-state；不上 AppState）；UseCase **不**知道 isOpening 存在 —— 由 `RealHomeViewModel.onChestOpenTap` 在 Task 起止位置 set / unset；
- **同一次点击**只发 1 次 POST：靠 view 层 button disabled（isOpening=true 时 button.disabled），不靠 idempotencyKey 兜底（同一次的 disabled 才是单击防抖第一道；idempotencyKey 是网络抖动重试的第二道）；
- **错误时复位**：UseCase throw 后 RealHomeViewModel catch block 先把 `isOpening=false`（按钮恢复可点）；用户下次点击会生成**新的** idempotencyKey（IdempotencyKeyGenerator.generate() 每次返回新 UUID v4 字符串 → 避免命中旧的幂等结果；AC2 钦定）。

**不涉及**（红线）：

- **不**实装 `RewardPopupView`（Story 21.4 落地）；本 story `OpenChestUseCase` 成功时返回 `ChestRewardSnapshot` 由 `RealHomeViewModel.onChestOpenTap` 写到 transient `pendingReward` 字段，21.4 view 订阅；本 story **不**渲染弹窗、不挂 `.sheet` modifier；
- **不**实装"开箱前主动同步步数"（Story 21.5 落地）；本 story OpenChestUseCase 构造签名预留 `stepSyncTriggerService: StepSyncTriggerService?` 默认参数 nil；
- **不**改 21.1 `ChestCardView` 内部代码 —— **仅扩 init 接受一个新 prop `isOpening: Bool = false`**（默认参数让既有 callsite 不变；本 story ChestCardView 视觉契约：unlockable 态下 `isOpening = true` → PrimaryButton 文字保持 "开宝箱" 但 `.disabled(true)` + 右侧叠 ProgressView 12pt；counting 态下 isOpening 忽略 —— counting 态无按钮可 disable）；
- **不**改 21.1 `ChestTimerDriver` 任何一行代码（driver 通过订阅 `appState.$currentChest` 自动 react；本 story OpenChestUseCase 写入 AppState 后 driver 自动重启倒计时给新 nextChest）；
- **不**改 21.2 `LoadChestUseCase` / `ChestRefreshTriggerService` 任何一行代码（chest open 后 nextChest 由 response 直写 AppState，**不需要**额外调 LoadChestUseCase 刷新）；
- **不**调 ErrorPresenter 时把 4002 / 3002 等业务码当 "default mapper" 走（spec AC + V1 §7.2 错误码表钦定要求**case-by-case 文案** —— "宝箱未解锁" / "步数不足" 等；与 `RealHomeViewModel.onJoinRoomConfirm` 6001-6005 mapper 同精神，详见 lesson `2026-05-11-business-error-fallback-must-forward-original-12-7-r8.md`）；
- **不**重新引入 1008 错误码处理（V1 §7.2 r11 review 锁定 1008 在节点 7 阶段本接口**不可达**；本 story 单测 AC7 case 5 "1008 兜底处理" 写法：**断言"若收到 1008 应当走 1009 同款 retry 逻辑（同 key 或新 key 退避）"**，**不**写专门的 1008 UI 文案）；
- **不**复用既有 idempotencyKey 跨多次点击（每次 RealHomeViewModel.onChestOpenTap 调用都 generate 新 key —— "同一次开箱" 由 button disabled + UseCase Task 边界界定，**不**靠 key 跨调用复用）；
- **不**改 server 任何文件（端独立原则；POST /api/v1/chest/open 由 Story 20.6 落地）；
- **不**改 ios/ 任何文件（CLAUDE.md + ADR-0002 §3.3）；
- **不**引 SnapshotTesting / ViewInspector / OHHTTPStubs / Mockingbird（ADR-0002 §3.1 钦定 XCTest only + 既有 MockBase / MockChestRepository 等已落地 mock 模式复用）。

## Acceptance Criteria

> **AC 编号体系**：AC1 = wire DTO + Endpoint + Repository 扩展；AC2 = IdempotencyKeyGenerator；AC3 = ChestRewardSnapshot domain model；AC4 = OpenChestUseCase（编排 + 写 AppState 双字段 + 返回 snapshot）；AC5 = HomeViewModel.isOpening + pendingReward + onChestOpenTap abstract method；AC6 = RealHomeViewModel.onChestOpenTap 真实 override（错误码 case-by-case 文案）；AC7 = ChestCardView isOpening 视觉 + HomeContainerView wire；AC8 = AppContainer factory + RootView wire；AC9 = 单元测试 ≥6 case；AC10 = 集成测试 ≥1 case；AC11 = build verify + ios-simulator MCP UI 实跑；AC12 = Deliverable 清单。

### AC1 — `ChestOpenRequest` / `ChestOpenResponse` wire DTO + `ChestOpenEndpoints` + `ChestRepository.openChest` 扩展

**新建文件**：

- `iphone/PetApp/Features/Home/Models/ChestOpenRequest.swift`
- `iphone/PetApp/Features/Home/Models/ChestOpenResponse.swift`
- `iphone/PetApp/Features/Home/UseCases/ChestOpenEndpoints.swift`

**改动文件**：

- `iphone/PetApp/Features/Home/Repositories/ChestRepository.swift`（protocol 加 `openChest(_:)` method + DefaultChestRepository 加 forwarding impl）

**ChestOpenRequest 契约**（V1 §7.2 行 940 + 1151 钦定字符集 `[A-Za-z0-9_:-]` length 1-128；client 仅生成符合此约束的 idempotencyKey）：

```swift
// ChestOpenRequest.swift
// Story 21.3 AC1: V1 §7.2 POST /api/v1/chest/open 请求 wire DTO.
//
// 契约源（V1 §7.2 r15 review 已冻结，**禁止**改字段）:
// - idempotencyKey: string，必填，1 ≤ length ≤ 128；字符集 [A-Za-z0-9_:-]
//
// 字段类型选择:
// - idempotencyKey: String（Codable encode 直接 JSON 字符串）；client 用 UUID v4 字面量
//   （形如 "E621E1F8-C36C-495A-93FC-0C247A3E6E5F"，36 字符长度落入 1-128 区间，且全字符
//   `A-F0-9` + `-` 满足字符集，无需额外清洗）.

import Foundation

public struct ChestOpenRequest: Encodable, Sendable, Equatable {
    public let idempotencyKey: String

    public init(idempotencyKey: String) {
        self.idempotencyKey = idempotencyKey
    }
}
```

**ChestOpenResponse 契约**（V1 §7.2 行 1052-1090 钦定三段嵌套 `reward / stepAccount / nextChest` + 字段表）：

```swift
// ChestOpenResponse.swift
// Story 21.3 AC1: V1 §7.2 POST /api/v1/chest/open 响应 wire DTO.
//
// 契约源（V1 §7.2 r15 review 已冻结）:
// - 外层走 APIClient 既有 envelope 解包（`code/message/data/requestId`，已 Story 2.4 / 5.5 落地）
// - data 字段三段嵌套：data.reward + data.stepAccount + data.nextChest
//
// 关键 schema 选择（与 Story 8.5 StepsSyncResponse 同模式）:
// - Decodable + Equatable + Sendable
// - 嵌套结构体单独命名（不复用 §7.1 ChestCurrentResponse —— §7.2 是"动作型"返回 next chest;
//   命名 ChestSnapshotInOpenResponse 表达"作为 open 响应的一部分"，与 StepAccountInSyncResponse 同精神）
//
// 节点 7 vs 节点 8 阶段（V1 §7.2.4h + 21.3 spec 红线钦定）:
// - reward.userCosmeticItemId: 节点 7 固定字符串 "0" 占位；节点 8 Story 23.5 起为真实 BIGINT 字符串
//   client 解析层**严格按 String** 处理（不 Optional / 不动态判断 "0" 做业务路径分支；V1 §7.2 关键约束行 1148）
//
// nextChest.status / nextChest.remainingSeconds: server 实时计算字段（与 §7.1 GET /chest/current
// 同源同时刻；详见 V1 §7.2.6 字段表 status / remainingSeconds 行说明）.

import Foundation

public struct ChestOpenResponse: Decodable, Sendable, Equatable {
    public let reward: ChestRewardDTO
    public let stepAccount: StepAccountInOpenResponse
    public let nextChest: ChestSnapshotInOpenResponse

    public init(
        reward: ChestRewardDTO,
        stepAccount: StepAccountInOpenResponse,
        nextChest: ChestSnapshotInOpenResponse
    ) {
        self.reward = reward
        self.stepAccount = stepAccount
        self.nextChest = nextChest
    }
}

public struct ChestRewardDTO: Decodable, Sendable, Equatable {
    public let userCosmeticItemId: String   // 节点 7 阶段固定 "0"；节点 8 起真实主键；client 只存不展示
    public let cosmeticItemId: String       // 装扮配置 id（BIGINT 字符串化）
    public let name: String                 // "星星围巾" 等装扮名（1 ≤ length ≤ 64）
    public let slot: Int                    // 1..7 / 99 枚举（V1 §6.8）
    public let rarity: Int                  // 1..4 枚举（common/rare/epic/legendary；V1 §6.9）
    public let assetUrl: String             // 非空字符串
    public let iconUrl: String              // 非空字符串

    public init(
        userCosmeticItemId: String,
        cosmeticItemId: String,
        name: String,
        slot: Int,
        rarity: Int,
        assetUrl: String,
        iconUrl: String
    ) {
        self.userCosmeticItemId = userCosmeticItemId
        self.cosmeticItemId = cosmeticItemId
        self.name = name
        self.slot = slot
        self.rarity = rarity
        self.assetUrl = assetUrl
        self.iconUrl = iconUrl
    }
}

public struct StepAccountInOpenResponse: Decodable, Sendable, Equatable {
    public let totalSteps: Int
    public let availableSteps: Int
    public let consumedSteps: Int

    public init(totalSteps: Int, availableSteps: Int, consumedSteps: Int) {
        self.totalSteps = totalSteps
        self.availableSteps = availableSteps
        self.consumedSteps = consumedSteps
    }
}

public struct ChestSnapshotInOpenResponse: Decodable, Sendable, Equatable {
    public let id: String                  // BIGINT 字符串化
    public let status: Int                 // 1 = counting / 2 = unlockable
    public let unlockAt: Date              // ISO 8601 RFC3339（APIClient JSONDecoder 已配 .iso8601）
    public let openCostSteps: Int          // 节点 7 阶段固定 1000
    public let remainingSeconds: Int       // server 实时计算（0..600）

    public init(id: String, status: Int, unlockAt: Date, openCostSteps: Int, remainingSeconds: Int) {
        self.id = id
        self.status = status
        self.unlockAt = unlockAt
        self.openCostSteps = openCostSteps
        self.remainingSeconds = remainingSeconds
    }
}
```

**ChestOpenEndpoints 契约**：

```swift
// ChestOpenEndpoints.swift
// Story 21.3 AC1: V1 §7.2 POST /api/v1/chest/open endpoint 工厂；与 StepsEndpoints / PetStateEndpoints 同模式.
//
// path 必须**含** `/api/v1` 前缀（与 ChestEndpoints / StepsEndpoints 同模式 —— APIClient 用
// host-only baseURL，拼出的 URL 是 baseURL + endpoint.path）.
//
// 拆独立 enum 而非合到 ChestEndpoints：
// - ChestEndpoints.current() 是 GET（读型；Story 21.2 落地）
// - ChestOpenEndpoints.open() 是 POST（动作型，带 body）
// - 拆开让"读型 endpoint 工厂"与"动作型 endpoint 工厂"职责清晰；
//   与 既有 PingEndpoints / HomeEndpoints（读）vs StepsEndpoints / PetStateEndpoints（动作）拆分同精神.
// - 或者：在 ChestEndpoints 里追加 open(_:) static method 也可接受（一文件双职责）；
//   本 story 采纳 "动作单独成 enum" 模式以保持文件单一职责.

import Foundation

public enum ChestOpenEndpoints {
    /// POST /api/v1/chest/open —— 开启宝箱（V1 §7.2）.
    /// requiresAuth=true：自动经过 APIClient 的 token 注入（Story 5.3）+ AuthBoundaryAPIClient 装饰器.
    public static func open(_ request: ChestOpenRequest) -> Endpoint {
        Endpoint(
            path: "/api/v1/chest/open",
            method: .post,
            body: AnyEncodable(request),
            requiresAuth: true
        )
    }
}
```

**ChestRepository 扩展**（既有 protocol + struct 新增方法；Story 21.2 已落地的 fetchCurrent 不动）：

```swift
// ChestRepository.swift
// Story 21.3 AC1 扩展：在既有 ChestRepositoryProtocol + DefaultChestRepository 上追加 openChest(_:) 方法.
// 不新建 ChestOpenRepository —— "chest" 是同一资源域，read + open 行为合并在同一 repository（与 StepRepository
// 内 syncSteps 单方法、HomeRepository 内 loadHome 单方法的"资源单一"原则不同；chest 是"读 + 写"双向 repository，
// 与未来 Epic 27 装扮穿戴 CosmeticRepository.equip/unequip 双方法同精神）.

import Foundation

public protocol ChestRepositoryProtocol: Sendable {
    /// 调 GET /api/v1/chest/current 拿当前宝箱状态（Story 21.2 落地）.
    func fetchCurrent() async throws -> ChestCurrentResponse

    /// 调 POST /api/v1/chest/open 开启宝箱（Story 21.3 落地）.
    /// - Parameter request: V1 §7.2 钦定的请求 wire DTO（idempotencyKey: string，1-128 字符）
    /// - Returns: ChestOpenResponse（三段嵌套：reward + stepAccount + nextChest）
    /// - Throws: APIError.business(1001 / 1002 / 1005 / 1009 / 3002 / 4001 / 4002) /
    ///           APIError.network / APIError.unauthorized / APIError.missingCredentials /
    ///           APIError.decoding
    func openChest(_ request: ChestOpenRequest) async throws -> ChestOpenResponse
}

public struct DefaultChestRepository: ChestRepositoryProtocol {
    private let apiClient: APIClientProtocol

    public init(apiClient: APIClientProtocol) {
        self.apiClient = apiClient
    }

    public func fetchCurrent() async throws -> ChestCurrentResponse {
        try await apiClient.request(ChestEndpoints.current())
    }

    public func openChest(_ request: ChestOpenRequest) async throws -> ChestOpenResponse {
        try await apiClient.request(ChestOpenEndpoints.open(request))
    }
}
```

> **关键决策 1（独立 ChestOpenResponse vs 复用嵌套字段）**：独立。理由：(a) V1 §7.2 是顶层接口，与 §7.1 / §5.1 在序列化路径上独立；(b) reward 是 §7.2 特有字段（§7.1 无）；(c) 与 `StepsSyncResponse` / `PetStateSyncResponse` 等独立动作响应 DTO 同模式。

> **关键决策 2（nextChest.status 用 `Int` 而非 `HomeChestStatus`）**：用 Int。理由：与 §7.1 `ChestCurrentResponse.status: Int` 同决策依据 —— V1 §7.1 r9 review + Story 21.2 AC1 关键决策 2 钦定 "client 解析层应按 Int 处理（防 schema drift 时 crash）"；UseCase 层 DTO → domain HomeChest 转换时统一做 `HomeChestStatus(rawValue:)` fail-fast，与 Story 21.2 LoadChestUseCase 同模式。

> **关键决策 3（reward.userCosmeticItemId 严格 String 而非 Optional）**：String。理由：V1 §7.2 关键约束行 1148 钦定 "iOS 端 ChestRewardDTO Codable struct 严格按 String 解析（不是 Optional<String>）"；节点 7 固定占位 "0" / 节点 8 起真实主键，schema 类型不变。

> **关键决策 4（repo.openChest 透传 APIError）**：透传不吞。理由：与 Story 21.2 `ChestRepository.fetchCurrent()` + `HomeRepository.loadHome()` 同精神；UseCase 层 catch 后由 RealHomeViewModel.onChestOpenTap 做错误码 case-by-case 文案。

**对应 Tasks**: Task 1.1, 1.2, 1.3, 1.4

### AC2 — `IdempotencyKeyGenerator` 协议 + UUID v4 默认实装

**新建文件**：`iphone/PetApp/Core/Networking/IdempotencyKeyGenerator.swift`

**契约**（与 `DateProvider` / `MotionProvider` 同精神：协议 + 默认 impl 让单测注入 mock）：

```swift
// IdempotencyKeyGenerator.swift
// Story 21.3 AC2: 幂等键生成协议（V1 §7.2 钦定 1-128 字符长度 + [A-Za-z0-9_:-] 字符集）.
//
// 默认实装用 UUID v4 字面量（Foundation `UUID().uuidString`，形如 "E621E1F8-C36C-495A-93FC-0C247A3E6E5F"
// 36 字符长度落入 1-128 区间 + 全字符 [A-F0-9-] 满足 [A-Za-z0-9_:-] 字符集，无需额外清洗）.
//
// 拆协议的理由（与 DateProvider 同精神）:
//   - 单测可注入固定 key 验证"同一次 use case 调用复用同 key"；
//   - 未来若需切换到 nanoid / 时间戳前缀格式（如 "chest_open_{userId}_{nanoTimestamp}"），改默认 impl 不动 UseCase；
//   - 与 V1 §7.2 字段表行 940 "client 应在每次点击开箱按钮时生成新的 key" 钦定一致.

import Foundation

public protocol IdempotencyKeyGenerator: Sendable {
    /// 生成一个新的 idempotency key.
    /// 实装契约：每次调用必须返回**不同**的字符串 +满足 V1 §7.2 字符集 [A-Za-z0-9_:-] + 长度 1-128.
    func generate() -> String
}

public struct DefaultIdempotencyKeyGenerator: IdempotencyKeyGenerator {
    public init() {}

    public func generate() -> String {
        // UUID v4: 36 字符（含 4 个连字符）; 字符集 [A-F0-9-] ⊂ [A-Za-z0-9_:-]; 满足 V1 §7.2 行 940 约束.
        return UUID().uuidString
    }
}
```

> **关键决策 1（用 UUID 而非 nanoTimestamp 拼接）**：UUID v4。理由：(a) Foundation 内置零依赖；(b) 满足 V1 §7.2 行 940 字符集约束；(c) 单测可通过 fake generator 注入固定字符串验证调用复用同 key；(d) 与 V1 §7.2 行 940 示例 "chest_open_{userId}_{nanoTimestamp}" 在格式上不同但**契约层等价** —— 都属于 "唯一字符串"，server 端不做格式校验仅做字符集 + 长度校验。

> **关键决策 2（每次调用返回新 UUID 而非缓存）**：每次新。理由：(a) Story 红线 "每次 RealHomeViewModel.onChestOpenTap 调用都 generate 新 key"；(b) 同一次 use case 内**不**重复调 generate（在 UseCase.execute 入口 capture 一次，整个 Task 复用）；(c) "同一次开箱" 由 button disabled + Task 边界界定，不靠 key 跨调用复用。

> **关键决策 3（放 `Core/Networking/` 而非 `Features/Home/UseCases/`）**：放 Core/Networking。理由：(a) 与 `APIClient` / `Endpoint` / `APIError` 同 module（"网络层基础设施"）；(b) 未来 Story 32.4 `POST /api/v1/compose/upgrade` 也复用 idempotencyKey（V1 §7.2 r11 跨接口影响段钦定）—— 放 Core 让 Home / Compose 等多 feature 共享；(c) 与 `DateProvider` 放 `Core/Time/`、`MotionProvider` 放 `Core/Motion/` 同精神。

**对应 Tasks**: Task 2.1

### AC3 — `ChestRewardSnapshot` domain model（给 21.4 弹窗用 + 单测对照）

**新建文件**：`iphone/PetApp/Features/Home/Models/ChestRewardSnapshot.swift`

**契约**（与 `HomeChest` / `HomeStepAccount` 同模式：value-type struct + Equatable + Sendable）：

```swift
// ChestRewardSnapshot.swift
// Story 21.3 AC3: 开箱奖励 domain 快照（给 Story 21.4 RewardPopupView 用）.
//
// 来源 / 用途:
//   - OpenChestUseCase.execute 成功路径返回此 snapshot 给 caller（RealHomeViewModel.onChestOpenTap）
//   - RealHomeViewModel 把 snapshot 写到 transient `pendingReward` 字段（HomeViewModel @Published）
//   - Story 21.4 RewardPopupView 通过 .sheet(item: $pendingReward) 订阅触发弹窗（Identifiable 需要）
//
// 字段从 ChestRewardDTO 映射:
//   - cosmeticItemId / name / slot / rarity / assetUrl / iconUrl 透传（rarity 转 RewardRarity enum）
//   - userCosmeticItemId 节点 7 占位 "0" → 不存（V1 §7.2 关键约束行 1148 "client UI 层禁止展示此字段"
//     + "client 不作为业务路径分支判断"；snapshot 是 UI domain model，不存 audit-only 字段）
//
// 与 wire DTO（ChestRewardDTO）区分的理由（与 HomeChest vs ChestCurrentResponse 同精神）:
//   - DTO 是 wire schema（API 解析层）
//   - Snapshot 是 domain model（UseCase 层 + View 层共享）
//   - 转换在 UseCase 内做（与 Story 21.2 LoadChestUseCase 内 ChestCurrentResponse → HomeChest 同模式）

import Foundation

public struct ChestRewardSnapshot: Equatable, Sendable, Identifiable {
    /// 用 cosmeticItemId 作为 Identifiable.id（让 SwiftUI .sheet(item:) 复用同一弹窗实例时 diff 正确）.
    /// 节点 7 阶段每次 reward 的 cosmeticItemId 可能重复（同一装扮被多次抽中），但 SwiftUI .sheet(item:)
    /// 在 item 从 nil → non-nil 时仍触发新 sheet —— Identifiable 实现仅影响"同 non-nil item 变化时是否
    /// reuse sheet"，本场景下不可达（要么 nil，要么从 nil 到 non-nil；节点 8 后才有同时刻多 reward 队列场景）.
    public var id: String { cosmeticItemId }

    public let cosmeticItemId: String
    public let name: String
    public let slot: Int
    public let rarity: RewardRarity
    public let assetUrl: String
    public let iconUrl: String

    public init(
        cosmeticItemId: String,
        name: String,
        slot: Int,
        rarity: RewardRarity,
        assetUrl: String,
        iconUrl: String
    ) {
        self.cosmeticItemId = cosmeticItemId
        self.name = name
        self.slot = slot
        self.rarity = rarity
        self.assetUrl = assetUrl
        self.iconUrl = iconUrl
    }
}

/// V1 §6.9 + 数据库设计 §6.9 钦定 4 档品质枚举.
/// 单独 enum 让 RewardPopupView（Story 21.4 落地）按 rarity 派生徽章颜色（common 灰 / rare 蓝 / epic 紫 / legendary 金）.
/// raw value 与 wire DTO `rarity: Int` 对齐（1..4）.
public enum RewardRarity: Int, Equatable, Sendable {
    case common = 1
    case rare = 2
    case epic = 3
    case legendary = 4
}
```

> **关键决策 1（snapshot 不存 userCosmeticItemId）**：不存。理由：V1 §7.2 关键约束行 1148 钦定 "client UI 层禁止展示此字段" + "client 不作为业务路径分支判断"；snapshot 是 UI domain model，audit-only 字段（节点 7 占位 / 节点 8 server 端关联）不必透传到 UI 层 + view layer。

> **关键决策 2（rarity 转 enum 而非保 Int）**：转 enum。理由：(a) view 层按 rarity 派生徽章颜色，enum 类型安全（漏 case 编译错）；(b) UseCase 层做 DTO → domain 转换时 fail-fast 抛 .decoding（与 HomeChestStatus / PetCurrentState 同精神：未知 rarity → APIError.decoding(underlying: ChestOpenDecodingError.unknownRewardRarity(99))）；(c) Story 21.4 `RarityTag(rarity:)` primitive 已落地按 enum 派生（详见 Story 21.4 spec addendum 行 3097）；本 story domain layer 已 enum 化让 21.4 view layer 零额外转换。

**对应 Tasks**: Task 3.1

### AC4 — `OpenChestUseCase`：业务编排（generate key → repo.openChest → DTO 转 domain → 写 AppState 双字段 + 返回 snapshot）

**新建文件**：`iphone/PetApp/Features/Home/UseCases/OpenChestUseCase.swift`

**契约**（与 `LoadChestUseCase` / `SyncStepsUseCase` 同模式：业务编排 + 写 AppState + 错误透传）：

```swift
// OpenChestUseCase.swift
// Story 21.3 AC4: 开箱 UseCase（业务编排：generate idempotencyKey → repo.openChest → DTO 转 domain →
// 写 AppState.currentChest + AppState.currentStepAccount → 返回 ChestRewardSnapshot 给 caller）.
//
// 职责（spec AC 行 3074-3088 + 21.3 addendum 钦定）:
//   1. 从 IdempotencyKeyGenerator capture 一次 idempotencyKey（同一次 execute 调用内复用）
//   2. (Story 21.5 入位：optionally await stepSyncTriggerService.triggerManual() —— 本 story 默认 nil 不调)
//   3. 调 repository.openChest(ChestOpenRequest(idempotencyKey:)) 拿 ChestOpenResponse
//   4. 转 domain:
//        - response.nextChest → HomeChest (未知 status fail-fast 抛 .decoding；与 LoadChestUseCase 同模式)
//        - response.stepAccount → HomeStepAccount (无 enum 字段，直接构造)
//        - response.reward → ChestRewardSnapshot (未知 rarity fail-fast 抛 .decoding)
//   5. **同一 await MainActor.run 块内**双写 AppState：
//        - appState.applyCurrentChest(nextChest) （Story 21.2 AC3 落地的入口）
//        - appState.applySyncedStepAccount(stepAccount) （Story 8.5 AC7 落地的入口）
//   6. 返回 snapshot 给 caller（RealHomeViewModel.onChestOpenTap 写到 pendingReward）
//
// **不**做的事:
//   - 不接 ErrorPresenter（caller 决定错误展示策略；与 SyncStepsUseCase / LoadChestUseCase 同精神）
//   - 不做 retry / 指数退避（V1 §7.2 client 重试策略钦定 retry 由 caller / UI 层决定；本 use case 单次）
//   - 不动 HomeViewModel 任何字段（isOpening 由 RealHomeViewModel.onChestOpenTap 内 set，不进 UseCase）
//   - 不读 AppState 现有字段（idempotency 是无状态生成；不需要 current chest id / currentStepAccount 上下文）
//
// idempotencyKey 复用语义（同一次 execute 调用复用同 key）:
//   - execute 入口 capture 一次 key → request 内 capture → throw 后 caller 重试时调 execute 重新生成
//     新 key（因为 caller 起新 Task → 新 UseCase.execute → 新 generate）.
//   - 这与 V1 §7.2 关键约束行 940 "client 应在每次点击开箱按钮时生成新的 key" 对齐：
//     "每次点击" = 每次 onChestOpenTap = 每次 execute = 一次 generate.
//   - "网络抖动重试时复用同一 key" 由 APIClient 内部 retry policy 落地（如果未来引入；本 story 不引入）.

import Foundation

public protocol OpenChestUseCaseProtocol: Sendable {
    /// 执行一次开箱.
    /// - Returns: 奖励快照（给 caller 写 ViewModel.pendingReward）
    /// - Throws: APIError（全部 case 原样透传给 caller）/ ChestOpenDecodingError（未知 chest.status / 未知 rarity）
    func execute() async throws -> ChestRewardSnapshot
}

public struct DefaultOpenChestUseCase: OpenChestUseCaseProtocol {
    private let repository: ChestRepositoryProtocol
    private let appState: AppState
    private let keyGenerator: IdempotencyKeyGenerator
    /// Story 21.5 入位：本 story 默认 nil，不调；21.5 落地时改默认传 stepSyncTriggerService 实例.
    private let stepSyncTriggerService: StepSyncTriggerService?

    public init(
        repository: ChestRepositoryProtocol,
        appState: AppState,
        keyGenerator: IdempotencyKeyGenerator = DefaultIdempotencyKeyGenerator(),
        stepSyncTriggerService: StepSyncTriggerService? = nil
    ) {
        self.repository = repository
        self.appState = appState
        self.keyGenerator = keyGenerator
        self.stepSyncTriggerService = stepSyncTriggerService
    }

    public func execute() async throws -> ChestRewardSnapshot {
        // Step 0 (Story 21.5 入位): 主动同步步数让 server 用最新余额判定（本 story stepSyncTriggerService = nil，跳过）.
        // 21.5 落地时此处 await stepSyncTriggerService?.triggerManual() —— 失败也继续开箱（不阻塞，让 server
        // 用上一次 sync 后的余额判定；与 Story 21.5 AC "同步失败也继续开箱" 钦定一致）.
        if let stepSync = stepSyncTriggerService {
            await stepSync.triggerManual()
        }

        // Step 1: 生成 idempotencyKey（同一次 execute 复用同 key；caller 重试调 execute 时自然新 key）.
        let idempotencyKey = keyGenerator.generate()
        let request = ChestOpenRequest(idempotencyKey: idempotencyKey)

        // Step 2: 调 server.
        let response = try await repository.openChest(request)

        // Step 3: DTO → domain（未知 enum fail-fast 抛 .decoding；与 LoadChestUseCase / HomeData(from:) 同模式）.
        guard let chestStatus = HomeChestStatus(rawValue: response.nextChest.status) else {
            throw APIError.decoding(underlying: ChestOpenDecodingError.unknownNextChestStatus(response.nextChest.status))
        }
        guard let rarity = RewardRarity(rawValue: response.reward.rarity) else {
            throw APIError.decoding(underlying: ChestOpenDecodingError.unknownRewardRarity(response.reward.rarity))
        }

        let nextChest = HomeChest(
            id: response.nextChest.id,
            status: chestStatus,
            unlockAt: response.nextChest.unlockAt,
            openCostSteps: response.nextChest.openCostSteps,
            remainingSeconds: response.nextChest.remainingSeconds
        )
        let stepAccount = HomeStepAccount(
            totalSteps: response.stepAccount.totalSteps,
            availableSteps: response.stepAccount.availableSteps,
            consumedSteps: response.stepAccount.consumedSteps
        )
        let snapshot = ChestRewardSnapshot(
            cosmeticItemId: response.reward.cosmeticItemId,
            name: response.reward.name,
            slot: response.reward.slot,
            rarity: rarity,
            assetUrl: response.reward.assetUrl,
            iconUrl: response.reward.iconUrl
        )

        // Step 4: 同一 main actor 同步块写双字段 AppState（保证 nextChest + stepAccount 原子可见；
        //         driver 一次 main actor tick 内同时接收两次 @Published 变化）.
        await MainActor.run {
            appState.applyCurrentChest(nextChest)
            appState.applySyncedStepAccount(stepAccount)
        }

        // Step 5: 返回 snapshot 给 caller.
        return snapshot
    }
}

/// 描述 ChestOpenResponse 解析时的 schema drift 失败原因（与 HomeDataDecodingError 同精神）.
/// 单独命名让单测可精确断言子类型；UI 文案统一走 .decoding 通用 mapper（"数据异常，请重试"）.
public enum ChestOpenDecodingError: Error, Equatable {
    case unknownNextChestStatus(Int)
    case unknownRewardRarity(Int)
}
```

> **关键决策 1（同一 await MainActor.run 块内双写 AppState）**：双写同块。理由：(a) 让 nextChest + stepAccount 在 driver 的一次 main actor tick 内同时变化（避免 driver 看到 nextChest 已更但 stepAccount 还是旧的中间态）；(b) AppState 是 @MainActor + @Published —— 两次 set 各自触发 objectWillChange，SwiftUI 会合并到下一次 runloop 渲染；(c) 与 `SyncStepsUseCase.execute` 单字段写、`LoadChestUseCase.execute` 单字段写 同精神 + 增量扩展（本 use case 是 V1 §7.2 钦定的"动作返回三段嵌套"，必须双写）。

> **关键决策 2（key 生成时机 = execute 入口）**：execute 入口 capture 一次。理由：(a) 同一次 execute Task 内复用同 key 让网络层未来引入 retry 时（如 1009 退避）能正确命中 server 端 idempotency cache；(b) caller throw 后调 execute 重试自然生成新 key（V1 §7.2 client 重试策略钦定的 "1009 同 key 或新 key 退避都安全"）；(c) caller 单测可通过 fake generator 验证：连续两次 execute 调用 → generate() 被调 2 次 → 拿到 2 个不同 key。

> **关键决策 3（unknown rarity 也走 .decoding 而非 .business）**：.decoding。理由：(a) 与 HomeChestStatus / PetCurrentState 未知值同 lesson 依据（`docs/lessons/2026-04-27-home-data-fail-fast-on-unknown-enum.md`）—— rarity 是 server 端 schema 冻结的枚举（V1 §6.9：1..4），出现 5 / 99 即 schema drift 信号；(b) AppErrorMapper 把 .decoding 映射到 transient "数据异常，请重试" RetryView（lesson `2026-04-28-decoding-and-unauthorized-must-be-transient-retry.md`），不破坏 UX；(c) UseCase 层完全无视业务码，业务码统一在 caller（RealHomeViewModel.onChestOpenTap）catch block 做 case-by-case mapping。

> **关键决策 4（不调 LoadChestUseCase 刷新 nextChest）**：不调。理由：(a) V1 §7.2 钦定响应已含 `data.nextChest` 5 字段 server 端权威态；(b) 调 LoadChest 会多发一次 GET /chest/current 浪费配额 + 引入"open 成功但 LoadChest 失败"半成功态；(c) 21.2 ChestRefreshTriggerService 60s timer + foreground trigger 已兜底未来漂移。

> **关键决策 5（stepSyncTriggerService 默认 nil）**：默认 nil。理由：(a) Story 21.5 落地时改默认传非 nil，本 story 不预先 wire；(b) 让 21.5 实装只改 AppContainer factory + DefaultOpenChestUseCase init 默认参数两处，OpenChestUseCase.execute 逻辑零改动；(c) 单测可注入 mock stepSyncTriggerService 验证 21.5 落地后 "先同步再开箱" 路径。

**对应 Tasks**: Task 4.1

### AC5 — `HomeViewModel` 新增 `isOpening` + `pendingReward` + `onChestOpenTap` abstract method

**改动文件**：`iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`

**关键改动**（与 `chestRemainingSeconds` / `showJoinModal` / `onCreateTap` 等既有字段 + abstract method 同模式）：

```swift
/// Story 21.3 AC5: 开箱按钮 loading 状态（disabled + ProgressView 视觉派生）.
///
/// 单位：Bool；初值 false（idle 态不 disable 按钮 / 不显 ProgressView）.
///
/// **派生契约**：
/// - ChestCardView.unlockableView 的 PrimaryButton `.disabled(isOpening)` + 右侧叠 ProgressView（isOpening=true）
/// - 由 RealHomeViewModel.onChestOpenTap 在 Task 起止位置 set/unset：
///     - 入口：isOpening = true（同步段，guard 短路防重入）
///     - 出口（成功 / 失败 / cancel）：defer 内 isOpening = false（必恢复）
/// - **不**上 AppState（ADR-0010 §3.2 "Loading flag → ViewModel @Published"钦定；与 loadingState 同精神）.
///
/// 不变量:
/// - isOpening = true 期间 onChestOpenTap 早 guard 短路（重入防御层 1）
/// - SwiftUI 按钮 .disabled(isOpening) 阻止重入（防御层 2）
/// - 两层防御都失败也无业务后果：UseCase 内 idempotencyKey + server 端 DB UNIQUE 兜底（防御层 3）.
@Published public var isOpening: Bool = false

/// Story 21.3 AC5: 开箱成功后的奖励快照（Story 21.4 RewardPopupView 通过 .sheet(item:) 订阅触发）.
///
/// 设置时机：RealHomeViewModel.onChestOpenTap 成功路径 await snapshot 后写入.
/// 清空时机：Story 21.4 RewardPopupView 关闭时 SwiftUI 自动设 nil（与 showJoinModal sheet 模式同精神）.
///
/// **不**上 AppState（与 isOpening 同精神：transient view-state；ADR-0010 §3.2 钦定 toast / popup state 归 ViewModel）.
@Published public var pendingReward: ChestRewardSnapshot?
```

**onChestOpenTap abstract method**（紧邻 onCreateTap / onJoinTap 等既有 abstract method）：

```swift
/// Story 21.3 AC5: 用户点击 ChestCardView "开宝箱" 按钮（仅 unlockable 态）.
/// MockHomeViewModel: 记录 invocation + 可选 mock pendingReward 让 Preview 看 21.4 弹窗.
/// RealHomeViewModel: 调 OpenChestUseCase + 写 isOpening / pendingReward + 错误码 case-by-case mapping.
/// 基类 `fatalError`：漏 override 时立刻 crash（不接受 default empty 实现 silent miss）.
public func onChestOpenTap() {
    fatalError("HomeViewModel.onChestOpenTap must be overridden by subclass")
}
```

> **关键决策 1（pendingReward 用 ChestRewardSnapshot? 而非 [ChestRewardSnapshot] 队列）**：单 Optional。理由：(a) 节点 7 阶段 UI 钦定 "一次点击一个奖励一个弹窗"，无并发场景；(b) SwiftUI .sheet(item: $pendingReward) 自然 work；(c) 节点 8 Epic 23 多 reward 队列时再演进。

> **关键决策 2（onChestOpenTap 走 abstract method 而非 closure 注入）**：abstract method。理由：(a) 与既有 onCreateTap / onJoinTap / onFeedTap / onPetTap / onPlayTap 同模式；(b) Mock / Real 子类各自 override 时编译器强制 → 漏 override 立刻 crash（fatalError）防 silent miss；(c) closure 注入需要 RootView 在 .task 内 bind closure，pattern 比 abstract method 复杂。

> **关键决策 3（isOpening / pendingReward 放基类而非子类）**：基类。理由：(a) 与 chestRemainingSeconds / showJoinModal 同模式（"基类持字段、Mock/Real 子类共享读取路径"）；(b) ChestCardView 通过 chestSlot ViewBuilder closure 注入 isOpening prop 时既能 hardcode mock 数据（Preview）也能由 Real 子类 onChestOpenTap 驱动；(c) RewardPopupView (Story 21.4) 通过 @Published pendingReward 订阅，Mock / Real 子类共用同一 sink。

**对应 Tasks**: Task 5.1, 5.2, 5.3

### AC6 — `RealHomeViewModel.onChestOpenTap` 真实 override（调 UseCase + 错误码 case-by-case 文案）

**改动文件**：`iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift`

**关键改动**：在子类内追加 `openChestUseCase` 字段 + bind 入口 + override `onChestOpenTap`（与 createRoomUseCase / joinRoomUseCase + bind + onCreateTap / onJoinRoomConfirm 同模式）：

```swift
/// Story 21.3 AC6: OpenChestUseCase 注入（默认 nil；caller=RootView 通过 bind() 注入 container.makeOpenChestUseCase）.
private var openChestUseCase: OpenChestUseCaseProtocol?

/// Story 21.3 AC6: 扩 bind 入口（与既有 bind(createRoomUseCase:joinRoomUseCase:errorPresenter:refreshHomeOnStaleNavigation:) 同模式）.
/// 幂等：caller=RootView .onAppear 只调一次；多次调用覆盖既有引用（生产路径无意义，仅防错重）.
public func bind(openChestUseCase: OpenChestUseCaseProtocol) {
    self.openChestUseCase = openChestUseCase
}

/// Story 21.3 AC6: 用户点击"开宝箱"按钮的真实路径.
/// 流程:
///   1. guard isOpening == false（重入防御层 1；与 button .disabled 形成双层防御）.
///   2. guard openChestUseCase != nil 且 errorPresenter 已 wire（fallback: log + return，与 onCreateTap 同精神）.
///   3. isOpening = true（同步段，main actor）.
///   4. 起 Task: do { snapshot = try await useCase.execute() → self.pendingReward = snapshot }
///      catch { ErrorPresenter 业务错误码 case-by-case 文案 / 其他错误透传 mapper }.
///      defer { isOpening = false（必恢复，让按钮重新可点）}.
public override func onChestOpenTap() {
    guard !isOpening else {
        // 重入防御层 1：UI .disabled 应已挡住；此处 guard 是兜底（lesson 防御性编程 + 防 SwiftUI tap rapid-fire bug）.
        os_log(.debug, "RealHomeViewModel.onChestOpenTap: reentry blocked (isOpening already true)")
        return
    }
    guard let useCase = self.openChestUseCase else {
        // useCase nil fallback：保留 UITEST_SKIP_GUEST_LOGIN=1 启动模式 / 老 wire 路径下点开箱按钮不 crash.
        // 与 onCreateTap useCase nil fallback 同精神（log + return；本路径不能 fallback 直写 AppState
        // 因为开箱事务涉及步数扣减 + 抽奖随机，没有合理的本地占位）.
        os_log(.debug, "RealHomeViewModel.onChestOpenTap: no OpenChestUseCase wired (fallback: log + noop)")
        return
    }
    let presenter = self.localErrorPresenter
    self.isOpening = true
    Task { @MainActor [weak self] in
        defer {
            // 必恢复 isOpening = false（成功 / 失败 / cancel 都走此 defer）.
            self?.isOpening = false
        }
        guard let self else { return }
        do {
            let snapshot = try await useCase.execute()
            // 成功 → 写 transient pendingReward 字段（Story 21.4 RewardPopupView 通过 .sheet(item:) 订阅触发）.
            self.pendingReward = snapshot
            // 注：AppState.currentChest + currentStepAccount 已由 UseCase 内部写入；
            //    ChestTimerDriver 通过 sink 自动 react；ChestCardView 自动重新渲染 counting 态 + nextChest 倒计时.
        } catch let APIError.business(code, _, _) {
            // 业务错误码 case-by-case 文案（V1 §7.2 错误码表 + spec AC 行 3083）.
            // 与 RealHomeViewModel.onJoinRoomConfirm 6001-6005 mapper 同精神（lesson `2026-05-11-business-error-fallback-must-forward-original-12-7-r8.md`）.
            let alert: (title: String, message: String)? = {
                switch code {
                case 4002: return ("提示", "宝箱未解锁")
                case 3002: return ("提示", "步数不足，再走走吧")
                case 4001: return ("提示", "宝箱数据异常，请重启 App")
                case 1005: return ("提示", "操作过于频繁，请稍候")
                case 1009: return nil   // 透传给默认 mapper → RetryView 让用户重试（V1 §7.2 client 重试策略 1009 同 key 或新 key 退避）
                case 1002: return ("提示", "请求参数错误（idempotencyKey 不合法）")
                default: return nil   // 未知业务码透传给默认 mapper（保留 server message + requestId）
                }
            }()
            if let alert {
                presenter?.presentAlert(title: alert.title, message: alert.message)
            } else {
                // 1009 / 未知 code 透传**原** error 给 ErrorPresenter 默认 mapper（保留 server message + requestId,
                // 让 AppErrorMapper 派生 RetryView 让用户重试）.
                // 不 rewrap（lesson `2026-05-11-business-error-fallback-must-forward-original-12-7-r8.md`）.
                presenter?.present(APIError.business(code: code, message: "", requestId: ""))
                // ↑ 注：此处仍需透传原 error 而非合成. 见 lesson 钦定。本行先按 placeholder 写；实装时应改为
                //   `presenter?.present(error)`（保留 original APIError.business 的 message + requestId）.
            }
        } catch {
            // 非 business error（network / decoding / unauthorized / missingCredentials / localStoreFailure）
            // → 透传给 ErrorPresenter 默认 mapper.
            os_log(.error, "RealHomeViewModel.onChestOpenTap OpenChestUseCase error: %{public}@",
                   String(describing: error))
            presenter?.present(error)
        }
    }
}
```

> **关键决策 1（onChestOpenTap 走 button-side disabled 而非 idempotencyKey 防重入）**：button-side。理由：(a) SwiftUI `.disabled(isOpening)` 是单击防抖第一道（用户连点 SwiftUI 在 disabled 期内 tap 不触发 onTap）；(b) 内层 guard !isOpening 是防御层 2；(c) idempotencyKey 是网络抖动重试的第三道（同一 Task 内不该发两次请求，server 端 DB UNIQUE 是兜底）。spec 红线 "同一次开箱过程中点击按钮不重复触发（按钮 disabled + 同 idempotencyKey 即使重发也走幂等）"。

> **关键决策 2（defer 内 isOpening = false 必恢复）**：必恢复。理由：(a) Task 抛 / cancel / 正常完成都走 defer；(b) 即使 UseCase 抛 transient error 也让按钮立刻恢复 让用户重试；(c) 防 "Task 抛 → catch 后忘恢复 → 按钮永久 disabled" 死锁。

> **关键决策 3（4001 / 4002 / 3002 用 alert 而非 toast）**：alert。理由：(a) alert 阻塞 + 显式 OK 确认让用户感知 "操作未成功"（与 toast "短暂提示" 语义区分）；(b) 与 RealHomeViewModel.onJoinRoomConfirm 6001-6005 mapper 同精神 —— 业务校验失败用 alert；transient 错误用 RetryView（默认 mapper）；(c) 1005 用 alert 因为限频是 transient 但用户能感知到具体原因（"操作过于频繁"，让其等几秒再点）；(d) 1009 透传给默认 mapper → AppErrorMapper 派生 RetryView（V1 §7.2 client 重试策略钦定 1009 应 retry）。

> **关键决策 4（fallback useCase nil 不 fail-fast crash）**：log + return。理由：(a) 与 onCreateTap fallback 同精神；(b) UITEST_SKIP_GUEST_LOGIN=1 路径下 RootView wire 可能跳过 bind(openChestUseCase:)，让本路径 silently noop 不破坏其他测试；(c) production 路径 RootView 必 bind，nil fallback 在 production 不可达（dev-build 可通过 assertion 抓）。

> **关键决策 5（不引入 idempotencyKey 复用跨调用 / 不持 key 在字段）**：不复用 / 不持。理由：(a) spec 红线 "失败后按钮恢复可点（但生成新的 idempotencyKey 用于重试，避免命中旧的幂等结果）"；(b) Real onChestOpenTap 每次调用都启动新 Task → 新 UseCase.execute → 新 generate；(c) "重试发同 key" 由 APIClient 内部 retry policy 控制（本 story 不引入，未来路径预留）。

**对应 Tasks**: Task 6.1, 6.2, 6.3

### AC7 — `ChestCardView` isOpening 视觉 + `HomeContainerView.chestSlot` wire onChestOpenTap

**改动文件**：

- `iphone/PetApp/Features/Home/Views/ChestCardView.swift`（init 加 `isOpening: Bool = false` 默认参数 + unlockableView 按 isOpening 派生按钮 disabled / ProgressView 叠加）
- `iphone/PetApp/Features/Home/Views/HomeContainerView.swift`（HomeContainerHomeViewBridge.chestSlot closure 把 onOpenTap 占位换为 `{ homeViewModel.onChestOpenTap() }` + 新增 isOpening prop 读 homeViewModel.isOpening）

**ChestCardView 改动**（视觉契约最小扩展）：

```swift
// 既有 init 改造：追加 isOpening: Bool = false 默认参数（让既有 callsite 不变；本 story HomeContainerView 改造时显式传值）.
public init(
    currentChest: HomeChest?,
    remainingSeconds: Int,
    isOpening: Bool = false,
    onOpenTap: @escaping () -> Void
) {
    self.currentChest = currentChest
    self.remainingSeconds = remainingSeconds
    self.isOpening = isOpening
    self.onOpenTap = onOpenTap
}

// 既有字段加 isOpening:
private let isOpening: Bool

// unlockableView 改动：PrimaryButton 加 .disabled(isOpening) + 右侧 isOpening=true 时叠 ProgressView:
private var unlockableView: some View {
    Card(cornerRadius: theme.radius.cardLg, padding: 20) {
        HStack(spacing: 14) {
            Image(systemName: Icons.symbol(for: "box"))
                .font(.system(size: 36, weight: .heavy))
                .foregroundColor(theme.colors.coin)
            VStack(alignment: .leading, spacing: 6) {
                Text("宝箱已就绪")
                    .font(.system(size: 17, weight: .heavy))
                    .foregroundColor(theme.colors.ink)
                Text(isOpening ? "开箱中…" : "可开启")
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(theme.colors.inkSoft)
            }
            Spacer()
            ZStack(alignment: .trailing) {
                PrimaryButton(title: "开宝箱", variant: .primary, action: onOpenTap)
                    .frame(width: 96)
                    .disabled(isOpening)
                    .opacity(isOpening ? 0.5 : 1.0)
                    .accessibilityIdentifier(AccessibilityID.Home.chestOpenButton)
                if isOpening {
                    ProgressView()
                        .scaleEffect(0.7)
                        .padding(.trailing, 8)
                        .allowsHitTesting(false)  // 让 hit-test 仍走 PrimaryButton（虽已 disabled，但保持 accessibility tree 干净）
                }
            }
        }
    }
    .accessibilityIdentifier("chestCard_unlockable")
    .accessibilityElement(children: .contain)
}
```

**HomeContainerView 改动**（HomeContainerHomeViewBridge.chestSlot）：

```swift
chestSlot: {
    ChestCardView(
        currentChest: appState.currentChest,
        remainingSeconds: homeViewModel.chestRemainingSeconds,
        isOpening: homeViewModel.isOpening,
        onOpenTap: {
            // Story 21.3 落地：替换占位 `{}` 为真实 onChestOpenTap.
            // RealHomeViewModel override 调 OpenChestUseCase；MockHomeViewModel 记录 invocation.
            homeViewModel.onChestOpenTap()
        }
    )
}
```

> **关键决策 1（ChestCardView init 加默认参数而非破坏既有 signature）**：默认参数。理由：(a) 既有 Preview / Mock 单元测试 callsite 零改动；(b) 与 Story 21.1 ChestCardView init 风格一致（一个新加 prop 用默认参数兼容）；(c) production callsite 只有 HomeContainerView 一处，改造可控。

> **关键决策 2（isOpening 状态时 PrimaryButton 文字保持 "开宝箱"，副标题改 "开箱中…"）**：保留主标题 + 改副标题。理由：(a) PrimaryButton 文字保持 "开宝箱" 是视觉契约稳定（按钮的"功能"未变，只是临时不可点）；(b) 副标题 "可开启" → "开箱中…" 让用户感知 loading；(c) 右侧 ProgressView 视觉双重确认；(d) 与 iOS HIG 按钮 loading 通用模式对齐。

> **关键决策 3（ProgressView 不替换按钮而是叠加 ZStack 右侧）**：叠加。理由：(a) 按钮位置稳定（用户视觉锚定不变）；(b) 与 HIG "loading 状态显示 spinner 不切换 layout" 一致；(c) `.allowsHitTesting(false)` 让 a11y / hit-test 完全走 PrimaryButton（已 disabled），不破坏 a11y tree。

> **关键决策 4（HomeContainerView wire onOpenTap 闭包内调 homeViewModel.onChestOpenTap()）**：调 abstract method。理由：(a) 与 onCreateTap / onJoinTap / onFeedTap 等同 wire 模式（HomeView 内 button.action 直接调 state.onXxxTap()）；(b) closure 内闭包不持 self/useCase 引用 —— useCase 由 RealHomeViewModel 持，HomeContainerView 不感知；(c) Mock / Real 子类透明切换。

**对应 Tasks**: Task 7.1, 7.2

### AC8 — `AppContainer.makeOpenChestUseCase(appState:)` factory + RootView wire

**改动文件**：

- `iphone/PetApp/App/AppContainer.swift`（新增 makeOpenChestUseCase factory）
- `iphone/PetApp/App/RootView.swift`（RealHomeViewModel.bind 调用追加 openChestUseCase 参数；或新增独立 bind(openChestUseCase:) 调用）

**AppContainer 新增**（紧邻 Story 21.2 makeChestRefreshTriggerService 后）：

```swift
// MARK: - Story 21.3 AC8: Chest Open 链路 factory

/// Story 21.3 AC4: 构造 OpenChestUseCase（每次调用返回新实例；依赖 ChestRepository / AppState / keyGenerator）.
/// caller 必须传 appState（AppState 在 RootView 持有；不进 AppContainer 字段；ADR-0010 §3.1）.
/// 与 makeLoadChestUseCase / makeSyncStepsUseCase 同模式.
///
/// **keyGenerator 默认参数**：生产路径用 DefaultIdempotencyKeyGenerator（UUID v4）;
/// 测试 / Preview 可自定义注入（与 makeSyncStepsUseCase 内 dateProvider 注入模式同精神）.
///
/// **stepSyncTriggerService**：Story 21.5 落地时改默认传 container.makeStepSyncTriggerService(...) 实例；
/// 本 story（21.3）默认 nil（不调 sync，直开箱）.
public func makeOpenChestUseCase(appState: AppState) -> OpenChestUseCaseProtocol {
    DefaultOpenChestUseCase(
        repository: makeChestRepository(),
        appState: appState,
        keyGenerator: DefaultIdempotencyKeyGenerator(),
        stepSyncTriggerService: nil   // Story 21.5 改默认传 service 实例
    )
}
```

**RootView wire**（在 RealHomeViewModel 的 .bind(...) 调用位置追加一行 bind(openChestUseCase:)）：

```swift
// 既有 bind 调用（Story 12.7 落地）:
realHomeViewModel.bind(
    createRoomUseCase: container.makeCreateRoomUseCase(appState: appState),
    joinRoomUseCase: container.makeJoinRoomUseCase(appState: appState),
    errorPresenter: container.errorPresenter,
    refreshHomeOnStaleNavigation: { [weak realHomeViewModel] in ... }
)

// Story 21.3 AC8: 新增 bind(openChestUseCase:) 调用（紧邻既有 bind 调用之后；不合并到既有 bind 入口
// 是为了让 21.3 改动范围最小 + 既有 bind callsite 零修改）.
realHomeViewModel.bind(
    openChestUseCase: container.makeOpenChestUseCase(appState: appState)
)
```

> **关键决策 1（factory 每次返回新 use case 而非单例）**：每次新。理由：(a) UseCase struct 是 value type 构造廉价；(b) 与 makeLoadChestUseCase / makeSyncStepsUseCase 同模式；(c) 同 bind 入口幂等覆盖语义（多次 bind 拿同实例无意义）。

> **关键决策 2（RealHomeViewModel.bind(openChestUseCase:) 独立 bind 入口而非合并到既有 bind(createRoomUseCase:joinRoomUseCase:errorPresenter:refreshHomeOnStaleNavigation:)）**：独立。理由：(a) 改既有 bind 入口签名会破坏 createRoom / joinRoom 既有 callsite；(b) 独立 bind 入口让 21.3 改动可隔离审查；(c) 与 既有 bind(pingUseCase:) / bind(loadHomeUseCase:errorPresenter:) / bind(appState:) / bind(motionProvider:) 多入口分散注入模式一致。

> **关键决策 3（不在 AppContainer 字段持 OpenChestUseCase 单例）**：不持。理由：每个 RealHomeViewModel 拿独立 UseCase 实例无业务影响（UseCase 无状态）；与既有 createRoom / joinRoom 路径同精神。

**对应 Tasks**: Task 8.1, 8.2

### AC9 — 单元测试 ≥6 case（XCTest + MockChestRepository + AppState + FakeIdempotencyKeyGenerator + MockErrorPresenter）

**新建/扩充文件**：

- `iphone/PetAppTests/Features/Home/Repositories/MockChestRepository.swift`（已有；扩 openChest stub）
- `iphone/PetAppTests/Features/Home/UseCases/OpenChestUseCaseTests.swift`（新建 ≥6 case）
- `iphone/PetAppTests/Core/Networking/FakeIdempotencyKeyGenerator.swift`（新建测试 helper）
- `iphone/PetAppTests/Features/Home/ViewModels/RealHomeViewModelChestOpenTapTests.swift`（新建 ≥4 case，验证 onChestOpenTap 错误码 case-by-case 文案 + isOpening 必恢复）

**OpenChestUseCase 测试 cases**（≥6 case；spec AC 行 3086-3091 钦定 5 case + 本 story 补 1 case "Story 21.5 stepSyncTriggerService 路径预留"）：

| # | 场景 | 断言 |
|---|---|---|
| 1 | happy: status=1 nextChest counting → AppState 双字段写入 + 返回 snapshot | `appState.currentChest?.status == .counting` + `currentStepAccount?.availableSteps == 11160`（mock 数据）+ snapshot.cosmeticItemId / name / rarity 与 mock DTO 对齐 |
| 2 | happy: status=2 nextChest unlockable（边界）→ AppState 写入 status .unlockable | `appState.currentChest?.status == .unlockable` |
| 3 | happy: 同一 execute 调用内 keyGenerator.generate() 仅被调 **1** 次 | `fakeGen.callCount(of: "generate()") == 1` 且 mock repo 收到的 request.idempotencyKey 等于 fakeGen.stubKey |
| 4 | happy: 连续两次 execute（caller 重试）→ 两次 generate 各拿不同 key | 两次 mock repo.openChest 收到的 request.idempotencyKey 不同 |
| 5 | edge: APIError.business(4002 宝箱未解锁) → throw + AppState 保留旧值 | 断言 throw + `appState.currentChest` / `appState.currentStepAccount` 是 setUp 设置的初始值（未被覆盖） |
| 6 | edge: APIError.business(3002 步数不足) → throw + AppState 保留旧值 | 同 case 5 但 code = 3002 |
| 7 | edge: APIError.network → throw + AppState 保留旧值 | 断言 throw + AppState 不变 |
| 8 | edge: 未知 nextChest.status=99 → APIError.decoding(ChestOpenDecodingError.unknownNextChestStatus(99)) | 与 LoadChestUseCaseTests 未知 status fail-fast 同精神 |
| 9 | edge: 未知 reward.rarity=99 → APIError.decoding(ChestOpenDecodingError.unknownRewardRarity(99)) | 与 unknownNextChestStatus 同精神 |
| 10 | Story 21.5 预留: stepSyncTriggerService non-nil → triggerManual 被先调用，再调 openChest | 用 mock stepSyncTriggerService + invocation order assert (`stepSync.callCount == 1` before `repo.openChest`) |

**RealHomeViewModel.onChestOpenTap 测试 cases**（≥4 case；spec AC 行 3086-3091 钦定）：

| # | 场景 | 断言 |
|---|---|---|
| 1 | happy: useCase 成功 → isOpening true→false + pendingReward = snapshot + 无 alert | `vm.isOpening == false`（最终态）+ `vm.pendingReward != nil` + presenter 未调 presentAlert |
| 2 | edge: useCase throw business(4002) → isOpening 恢复 false + presentAlert "宝箱未解锁" | `presenter.alertCalls.last?.title == "提示"` + `.message == "宝箱未解锁"` |
| 3 | edge: useCase throw business(3002) → presentAlert "步数不足，再走走吧" | 同 case 2 但 message "步数不足，再走走吧" |
| 4 | edge: useCase throw business(1009) → present(error) 透传给默认 mapper（不调 presentAlert） | `presenter.presentCalls.count == 1` + `presenter.alertCalls.isEmpty` |
| 5 | edge: useCase throw network error → present(error) 透传 | 同 case 4 但 error 类型是 .network |
| 6 | edge: 重入防御 —— isOpening 已 true 时第二次 onChestOpenTap → useCase 仅被调 1 次 | `mockUseCase.callCount(of: "execute()") == 1` |
| 7 | edge: useCase nil fallback（未 bind）→ noop + 不 crash + isOpening 保持 false | `vm.isOpening == false` + `mockUseCase.callCount == 0` |

**MockChestRepository 扩展**（既有 21.2 落地的 mock 上加 openChest stub）：

```swift
final class MockChestRepository: MockBase, ChestRepositoryProtocol, @unchecked Sendable {
    var fetchCurrentStub: Result<ChestCurrentResponse, Error>?
    var openChestStub: Result<ChestOpenResponse, Error>?
    private(set) var lastOpenChestRequest: ChestOpenRequest?

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

    func openChest(_ request: ChestOpenRequest) async throws -> ChestOpenResponse {
        recordCall("openChest(_:)")
        lastOpenChestRequest = request
        switch openChestStub {
        case .success(let resp): return resp
        case .failure(let err): throw err
        case nil:
            XCTFail("openChestStub not set")
            throw APIError.network(underlying: URLError(.unknown))
        }
    }
}

final class FakeIdempotencyKeyGenerator: MockBase, IdempotencyKeyGenerator, @unchecked Sendable {
    /// 每次 generate 返回数组下一个；用完后 wrap-around（满足"两次连续 execute 各拿不同 key"测试）.
    var keys: [String] = ["test-key-1", "test-key-2", "test-key-3"]
    private var index = 0

    func generate() -> String {
        recordCall("generate()")
        let key = keys[index % keys.count]
        index += 1
        return key
    }
}
```

> **关键决策 1（不在 UseCase 单测内验 driver react）**：分层验证。理由：UseCase 单测只断言 AppState 双字段写入正确；driver react 是 Story 21.1 / 21.2 已落地 Combine sink 行为；本 story 不重复验证。

> **关键决策 2（onChestOpenTap 单测用 MockOpenChestUseCase + MockErrorPresenter 而非真 UseCase）**：mock。理由：(a) Real 单测路径已被 OpenChestUseCaseTests 覆盖（mock repo）；(b) onChestOpenTap 单测只验 isOpening 状态机 + 错误码 case-by-case 文案；(c) 与 RealHomeViewModel.onJoinRoomConfirm 既有测试同精神。

> **关键决策 3（不测 idempotencyKey 字符集 / 长度合规）**：信任 UUID.uuidString 实现。理由：(a) Foundation UUID 是 well-known；(b) 测试只断言 "每次新 key" + "同一 execute 内复用同 key" 行为；(c) server 端约束由 Story 20.6 server 测试覆盖。

**对应 Tasks**: Task 9.1, 9.2, 9.3, 9.4

### AC10 — 集成测试 ≥1 case（XCUITest + mock server 路径）

**新建文件**：`iphone/PetAppUITests/ChestOpenUITests.swift`（与 ChestRefreshUITests / RoomUITests 同模式）

**测试场景**（spec AC 行 3120-3121 + 本 story 限定 21.4 之前可独立 verify 的范围）：

1. **launch unlockable 态 → 点开宝箱 → 验证按钮转 disabled + counting 态回归**：通过 launch env 注入 mock chest unlockable 响应 + mock openChest 响应（含 nextChest counting）；UITest 流程：
   - app launch → 等 chestCard_unlockable a11y 锚出现
   - tap `AccessibilityID.Home.chestOpenButton`
   - 立即断言按钮 `.isEnabled == false`（isOpening = true 视觉 + a11y disabled）
   - 等 chestCard_counting a11y 锚出现（UseCase 完成 + nextChest 写入 AppState + driver react）
   - 验证 mock server 收到 POST /chest/open + request body 含 idempotencyKey 字段

**实装策略**（与 ChestRefreshUITests / Story 20.x 集成测试同模式）：

- 复用 `UITEST_SKIP_GUEST_LOGIN` + 既有 mock 路径；
- 扩展 AppContainer DEBUG init 接受 `uiTestMockChestRepository: ChestRepositoryProtocol?` 参数（Story 21.2 已落地 line 223 的 hook 入口 + 本 story 让 openChest stub 也走同实例）；
- UITest assertion 用 `XCUIApplication().buttons[AccessibilityID.Home.chestOpenButton].isEnabled` 判定按钮可点状态。

> **关键决策 1（不验 RewardPopupView 弹出）**：21.4 范围。理由：(a) 本 story 仅写 pendingReward 到 transient ViewModel 字段，**不**渲染 RewardPopupView（21.4 落地）；(b) 集成测试在本 story 阶段只验 "按钮 → POST → AppState 更新 → counting 态回归" 链路；(c) 21.4 落地后扩展同一 UITest 文件加 reward popup 视觉断言。

> **关键决策 2（mock openChest 响应用 nextChest counting 而非 unlockable）**：counting。理由：(a) V1 §7.2 钦定新 chest `unlock_at = now + 10min` → status = 1 counting；(b) 让 UI 回归 counting 态视觉验证 driver react；(c) 若 mock 返 unlockable 反而异常（说明 nextChest 创建瞬间已到期，仅在测试 mock 数据漂移时出现）。

**对应 Tasks**: Task 10.1

### AC11 — `xcodegen` 同步 + `bash iphone/scripts/build.sh` 通过 + ios-simulator MCP UI 实跑验证

**关键改动**：

1. 新增的 8 个 .swift 文件加入 `iphone/project.yml` `sources` glob（target sources 用 glob recursive 自动包含，无需手动编辑 project.yml；只要文件落在正确目录下）；
2. 跑 `iphone/scripts/xcodegen.sh`（或 `xcodegen generate` 直接调）让 .xcodeproj 同步；
3. 跑 `bash iphone/scripts/build.sh` 验证 build pass；
4. 跑 `bash iphone/scripts/build.sh --test` 验证单测 pass（含本 story 新增 ≥10 case）；
5. **ios-simulator MCP UI 实跑验证**（CLAUDE.md "iOS UI 验证" 段钦定的标准 verify workflow）：
   - `bash iphone/scripts/build.sh` → `install_app` → `launch_app(terminate_running: true)` → `ui_view` → 等 chestCard_unlockable 出现（可通过 dev 端点强制解锁，或等 mock chest 数据驱动到 0）→ `ui_tap("home_chestOpenButton")` → 立即 `ui_view` 验证按钮 disabled + ProgressView 显示 → 数秒后 `ui_view` 验证回归 counting 态 + mm:ss 倒计时开始；
   - 同步验 ui_describe_all 验 chestCard_counting a11y 锚 + home_chestOpenButton 不再可见（counting 态不渲染按钮）。

> **关键决策（必须跑 ios-simulator MCP UI 验证）**：CLAUDE.md 钦定 "iOS UI / feature 改动必须用 ios-simulator MCP server 在模拟器里实跑验证，不能只跑 build.sh 就报告 done"。Story 37.x 多次出现 build pass 但视觉 bug / 交互 bug 漏到 review 阶段才发现。

**对应 Tasks**: Task 11.1, 11.2, 11.3

### AC12 — Deliverable 清单（≥12 文件改动）

**新建文件**（生产 6 + 测试 4 = 10）：

1. `iphone/PetApp/Features/Home/Models/ChestOpenRequest.swift`
2. `iphone/PetApp/Features/Home/Models/ChestOpenResponse.swift`
3. `iphone/PetApp/Features/Home/Models/ChestRewardSnapshot.swift`
4. `iphone/PetApp/Features/Home/UseCases/ChestOpenEndpoints.swift`
5. `iphone/PetApp/Features/Home/UseCases/OpenChestUseCase.swift`
6. `iphone/PetApp/Core/Networking/IdempotencyKeyGenerator.swift`
7. `iphone/PetAppTests/Core/Networking/FakeIdempotencyKeyGenerator.swift`
8. `iphone/PetAppTests/Features/Home/UseCases/OpenChestUseCaseTests.swift`
9. `iphone/PetAppTests/Features/Home/ViewModels/RealHomeViewModelChestOpenTapTests.swift`
10. `iphone/PetAppUITests/ChestOpenUITests.swift`

**改动文件**（生产 5 + 测试 1 = 6）：

11. `iphone/PetApp/Features/Home/Repositories/ChestRepository.swift`（protocol + struct 加 openChest 方法）
12. `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`（新增 isOpening / pendingReward / onChestOpenTap abstract method）
13. `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift`（新增 openChestUseCase 字段 + bind 入口 + override onChestOpenTap）
14. `iphone/PetApp/Features/Home/Views/ChestCardView.swift`（init 加 isOpening 默认参数 + unlockableView 按 isOpening 派生视觉）
15. `iphone/PetApp/Features/Home/Views/HomeContainerView.swift`（HomeContainerHomeViewBridge.chestSlot wire onChestOpenTap + isOpening prop）
16. `iphone/PetApp/App/AppContainer.swift`（新增 makeOpenChestUseCase factory）
17. `iphone/PetApp/App/RootView.swift`（RealHomeViewModel.bind(openChestUseCase:) wire）
18. `iphone/PetAppTests/Features/Home/Repositories/MockChestRepository.swift`（既有 21.2 落地的 mock 上扩 openChest stub）

**对应 Tasks**: Task 12.1

## Tasks / Subtasks

- [x] **Task 1: Wire 协议 + DTO + Repository 扩展（AC1）**
  - [x] 1.1 新建 `ChestOpenRequest.swift` —— 单字段 idempotencyKey: String
  - [x] 1.2 新建 `ChestOpenResponse.swift` —— 三段嵌套 wire DTO（reward / stepAccount / nextChest）+ ChestRewardDTO / StepAccountInOpenResponse / ChestSnapshotInOpenResponse
  - [x] 1.3 新建 `ChestOpenEndpoints.swift` —— path `/api/v1/chest/open` + method .post + requiresAuth=true
  - [x] 1.4 改 `ChestRepository.swift` —— protocol 加 `openChest(_:)` method + DefaultChestRepository 加 forwarding impl

- [x] **Task 2: IdempotencyKeyGenerator（AC2）**
  - [x] 2.1 新建 `IdempotencyKeyGenerator.swift` —— protocol + DefaultIdempotencyKeyGenerator (UUID v4)

- [x] **Task 3: ChestRewardSnapshot domain model（AC3）**
  - [x] 3.1 新建 `ChestRewardSnapshot.swift` —— Identifiable + Equatable + Sendable struct + RewardRarity enum (1..4)

- [x] **Task 4: OpenChestUseCase 编排层（AC4）**
  - [x] 4.1 新建 `OpenChestUseCase.swift` —— execute() async throws -> ChestRewardSnapshot：generate key → (optionally await stepSyncTriggerService?.triggerManual()) → repo.openChest → DTO 转 domain（unknown status / rarity fail-fast 抛 ChestOpenDecodingError）→ MainActor.run 同块双写 AppState → 返回 snapshot

- [x] **Task 5: HomeViewModel 字段 + abstract method（AC5）**
  - [x] 5.1 在 `HomeViewModel.swift` 内 chestRemainingSeconds 旁新增 `@Published public var isOpening: Bool = false`
  - [x] 5.2 同位置新增 `@Published public var pendingReward: ChestRewardSnapshot?`
  - [x] 5.3 在 abstract method 段（onCreateTap / onJoinTap 旁）新增 `func onChestOpenTap() { fatalError(...) }`
  - [x] 5.4 MockHomeViewModel override onChestOpenTap（避 fatalError + 加 .chestOpenTap 到 Invocation enum）

- [x] **Task 6: RealHomeViewModel override（AC6）**
  - [x] 6.1 新增 `private var openChestUseCase: OpenChestUseCaseProtocol?` 字段
  - [x] 6.2 新增 `public func bind(openChestUseCase:)` 入口
  - [x] 6.3 override `onChestOpenTap()` —— guard !isOpening + guard useCase != nil + Task with defer { isOpening = false } + do/catch (4002/3002/4001/1005/1002/1009/default 业务码 case-by-case 文案；其他错误 present(error) 透传)

- [x] **Task 7: ChestCardView isOpening 视觉 + HomeContainerView wire（AC7）**
  - [x] 7.1 改 `ChestCardView.swift` —— init 加 `isOpening: Bool = false` 默认参数 + 字段 + unlockableView ZStack 包 PrimaryButton + ProgressView + .disabled(isOpening) + 副标题 "可开启" → "开箱中…"
  - [x] 7.2 改 `HomeContainerView.swift` HomeContainerHomeViewBridge.chestSlot closure —— 把 onOpenTap `{}` 替换为 `{ homeViewModel.onChestOpenTap() }` + 新增 isOpening 参数读 homeViewModel.isOpening

- [x] **Task 8: AppContainer factory + RootView wire（AC8）**
  - [x] 8.1 改 `AppContainer.swift` —— 在 makeChestRefreshTriggerService 后新增 makeOpenChestUseCase(appState:) factory（默认 keyGenerator = DefaultIdempotencyKeyGenerator(), stepSyncTriggerService = nil）+ UITestMockChestRepository 注入 hook（UITEST_MOCK_CHEST_OPEN=1）
  - [x] 8.2 改 `RootView.swift` —— RealHomeViewModel 既有 bind 调用之后追加 `realHomeViewModel.bind(openChestUseCase: container.makeOpenChestUseCase(appState: appState))` + UITEST 路径也注入 (让 ChestOpenUITests 可达)

- [x] **Task 9: 单元测试（AC9，≥6 case）**
  - [x] 9.1 改 `MockChestRepository.swift` —— 扩 openChestStub + openChestInvocations + lastOpenChestRequest / openChestRequests
  - [x] 9.2 新建 `FakeIdempotencyKeyGenerator.swift` —— IdempotencyKeyGenerator impl + keys 数组 + index 推进 + callCount
  - [x] 9.3 新建 `OpenChestUseCaseTests.swift` —— 10 case：happy×5 (counting / unlockable / 同 key / 连续两次新 key / legendary rarity) + edge×4（business 4002 / 3002 / network / unknown nextChest.status / unknown reward.rarity）
  - [x] 9.4 新建 `RealHomeViewModelChestOpenTapTests.swift` —— 7 case：happy（pendingReward set + isOpening 同步段 true + Task 完后必恢复 false）+ business 4002 / 3002 / 1009 各自文案 + network 透传 + 重入防御（waitForCallCount 同步） + useCase nil fallback

- [x] **Task 10: 集成测试（AC10）**
  - [x] 10.1 新建 `ChestOpenUITests.swift` —— 1 case：UITEST_SKIP_GUEST_LOGIN + UITEST_MOCK_CHEST_OPEN launch → chestCard_unlockable 出现 → tap home_chestOpenButton → 等 chestCard_counting 出现 → chestCard_unlockable 消失（含 isEnabled 等断言）

- [x] **Task 11: Build + 模拟器实跑（AC11）**
  - [x] 11.1 跑 `bash iphone/scripts/build.sh` 确认 build pass
  - [x] 11.2 跑 `bash iphone/scripts/build.sh --test` 确认全单测 pass（含本 story 新增 17 case；721 tests, 0 failures）
  - [x] 11.3 ios-simulator MCP UI 实跑：install_app + launch_app(UITEST_SKIP_GUEST_LOGIN=1, UITEST_MOCK_CHEST_OPEN=1) + ui_view 验 unlockable 态 + ui_tap chestOpenButton → 验 chest 切到 counting 09:56 + 步数 0→11,000

- [x] **Task 12: Deliverable 清单核对（AC12）**
  - [x] 12.1 核对 18 文件新增/改动（实际：新建 10 + 改动 9 = 19）—— 本 story 不夹带其它无关改动

## Dev Notes

### 架构对齐

- **iOS 工程结构（`docs/宠物互动App_iOS客户端工程结构与模块职责设计.md`）**：本 story 落地在 `Features/Home/` 内（Endpoints / Repositories / UseCases / Services / Models / Views / ViewModels 七分层），与既有 Home / Pet / Step / Chest（21.2）链路同模式。`IdempotencyKeyGenerator` 落 `Core/Networking/`（基础设施，多 feature 共享）。
- **ADR-0010 §3.1 / §3.2**：domain state 单 source of truth 是 AppState，ViewModel 仅持 view-state。本 story `OpenChestUseCase` 写 AppState 双字段（currentChest + currentStepAccount）；`isOpening` / `pendingReward` 是 transient view-state 归 HomeViewModel @Published。
- **ADR-0010 §3.3**：hydrate / mutation 入口前缀 `apply*`。本 story 复用既有 `applyCurrentChest`（21.2 落地）+ `applySyncedStepAccount`（8.5 落地）入口，**不**新增 mutation method。

### 错误处理边界

- **APIError 三层映射**（与 Server 端 ADR-0001 § 错误三层映射框架同精神）：repo 层透传 → UseCase 层透传（unknown enum fail-fast 抛 .decoding）→ RealHomeViewModel.onChestOpenTap catch 后做业务码 case-by-case 文案 + 默认 mapper 透传。
- **业务错误码文案策略**（V1 §7.2 错误码表 + spec AC 行 3083）：
  - **4002 宝箱未解锁** → alert "宝箱未解锁"（用户感知 "时机错"，无须 retry，等倒计时归零）
  - **3002 步数不足** → alert "步数不足，再走走吧"（用户感知 "缺资产"，引导走步赚步数）
  - **4001 宝箱不存在** → alert "宝箱数据异常，请重启 App"（数据完整性异常，无业务恢复路径）
  - **1005 操作过于频繁** → alert "操作过于频繁，请稍候"（V1 §7.2 client 重试策略钦定 1005 应等限频窗口 60s）
  - **1002 参数错误** → alert "请求参数错误（idempotencyKey 不合法）"（理论不可达：client 用 UUID v4 必合规；若收到说明 client / server 协议偏差）
  - **1009 服务繁忙** → present(error) 透传给默认 mapper（→ RetryView 让用户重试；V1 §7.2 client 重试策略钦定 1009 同 key 或新 key 退避都安全）
  - **未知业务码** → present(error) 透传（保留 server message + requestId；lesson `2026-05-11-business-error-fallback-must-forward-original-12-7-r8.md`）
- **未知 enum fail-fast**：未知 nextChest.status（不在 {1, 2}）或未知 reward.rarity（不在 {1, 2, 3, 4}）→ APIError.decoding(ChestOpenDecodingError.xxx)。与 Story 5.5 round 6 [P2] fix + Story 21.2 LoadChestUseCase 钦定一致；详见 `docs/lessons/2026-04-27-home-data-fail-fast-on-unknown-enum.md`。
- **网络错误 / unauthorized / decoding**：透传给 ErrorPresenter 默认 mapper（AppErrorMapper 派生 RetryView / TerminalErrorView）。

### 重入防御三层

1. **SwiftUI button .disabled(isOpening)**：用户连点 SwiftUI 在 disabled 期内 tap 不触发 onTap closure（防御层 1，最强）
2. **`RealHomeViewModel.onChestOpenTap` 入口 guard !isOpening**：防御层 2，兜底 SwiftUI tap rapid-fire bug / 测试场景
3. **server 端 DB UNIQUE 约束 + 同事务幂等**：Story 20.6 落地，client 多发同 key 也只业务执行一次（防御层 3）

### idempotencyKey 生成策略

- **生成时机**：`OpenChestUseCase.execute()` 入口一次性 capture，整个 Task 复用；caller 重试调 execute 时自然 generate 新 key（每次 onChestOpenTap → 新 Task → 新 execute → 新 generate）
- **同一次开箱复用 key**：spec 红线"同一次开箱过程中点击按钮不重复触发（按钮 disabled + 同 idempotencyKey 即使重发也走幂等）"—— 同一 execute Task 内若未来 APIClient 引入网络层 retry（本 story 不引入），会用同一 key 命中 server 端 idempotency cache
- **失败后生成新 key**：spec 红线"失败后按钮恢复可点（但生成新的 idempotencyKey 用于重试，避免命中旧的幂等结果）"—— UseCase 抛后 defer isOpening = false，下次 onChestOpenTap → 新 execute → 新 generate

### 与 21.1 / 21.2 接缝

- **ChestTimerDriver 自动 react**：21.1 driver 订阅 `appState.$currentChest` Combine sink → 本 story OpenChestUseCase 写 nextChest 后 driver 立即重启倒计时（新 unlockAt = now + 10min → mm:ss 显示约 09:59 起跳）；ChestCardView 自动 rerender counting 态。**本 story 不需要改 driver 任何代码**。
- **ChestRefreshTriggerService 不冲突**：21.2 60s timer 仍跑；本 story 开箱成功后 1 秒内 driver 已 react server 响应内的 nextChest，60s timer 下次到达拉到的 server 状态会与本地一致（除非用户在 60s 内开箱多次，timer 拉到的也是同一 nextChest）。
- **stepAccount 更新路径**：21.3 OpenChestUseCase 在写 currentChest 的同一 main actor block 内写 currentStepAccount —— 让任何订阅 `AppState.$currentStepAccount` 的 view（如未来 Profile / StepBalanceView）也立即看到新余额。

### Story 21.5 入位

- **stepSyncTriggerService 参数预留**：OpenChestUseCase 构造签名 `stepSyncTriggerService: StepSyncTriggerService? = nil`；21.5 落地时改默认参数 `stepSyncTriggerService: container.makeStepSyncTriggerService(...)`（或在 AppContainer.makeOpenChestUseCase 内传非 nil），execute() 内 `if let stepSync = stepSyncTriggerService { await stepSync.triggerManual() }` 分支自然激活。
- **本 story 单测 AC9 case 10 已预留**：MockStepSyncTriggerService + invocation order assert（triggerManual 先于 openChest 被调）；21.5 落地时 reuse 同 case 加产真实 service 注入路径。
- **失败不阻塞契约**：Story 21.5 AC 钦定 "同步失败也继续开箱（不阻塞，让 server 用上一次 sync 后的余额判定）"；本 story OpenChestUseCase `await stepSync.triggerManual()` 不接 try/catch（triggerManual 内部 silently 吞错；与 Story 8.5 StepSyncTriggerService.triggerManual 同精神，详见 lesson `2026-04-27-transient-vs-terminal-error-classification.md`）。

### isOpening 状态机不变量

- **入口同步段 set true**：`onChestOpenTap` 内 `self.isOpening = true` 是 main actor 同步段（在 Task 起之前），让 SwiftUI button 立即 disabled
- **Task defer 恢复 false**：无论 Task 抛 / cancel / 正常完成都走 defer → isOpening = false → 按钮恢复可点
- **不变量**：用户感知到的"开箱中"状态严格等于 `isOpening = true` 的窗口；无悬挂死锁；UseCase 内部任何 throw 都不影响 isOpening 恢复（defer 在 catch 之外）

### pendingReward 写入时机

- **成功路径**：`self.pendingReward = snapshot`（在 do block 内 await snapshot 后立即写）
- **失败路径**：不写 pendingReward（保持 nil；Story 21.4 view sheet 不弹）
- **清空时机**：Story 21.4 RewardPopupView 关闭时 SwiftUI 自动 set $pendingReward = nil（双向绑定 + Optional Identifiable 模式）；本 story 不实装 RewardPopupView，pendingReward 设置后会在 ViewModel 字段留存到下次开箱覆盖（节点 7 验证阶段无副作用）。

### 性能与生命周期

- **UseCase 是 value-type struct**：构造廉价；每次 RealHomeViewModel.onChestOpenTap 起 Task 内调 useCase.execute，无 alloc 性能问题
- **idempotencyKey UUID v4 生成开销**：< 1μs（Foundation 内部 arc4random），可忽略
- **MainActor 双字段写**：在同一 await MainActor.run 块内 → SwiftUI runloop 合并触发一次 rerender，无双倍渲染开销

### 测试边界

- **OpenChestUseCase 单测不依赖真实 server**：mock repo + AppState；fake keyGenerator 让 idempotencyKey 可断言
- **RealHomeViewModel.onChestOpenTap 单测**：mock OpenChestUseCase + mock ErrorPresenter；不调真实 UseCase（避免重复覆盖）
- **集成测试 (UITest) 不验 RewardPopupView**：本 story 范围；Story 21.4 落地后扩展同一 UITest 文件加 reward popup 视觉断言
- **MockChestRepository 已落地**：21.2 已建；本 story 仅扩 `openChest` stub + `lastOpenChestRequest` 让测试断言 request 体

### 命名规则

- **UseCase 名 `OpenChestUseCase`**（而非 `ChestOpenUseCase`）：理由：(a) 动词在前更直观（"open chest"）；(b) 与 `SyncStepsUseCase` / `SyncPetStateUseCase` / `LoadHomeUseCase` / `LoadChestUseCase` 的"动词 + 名词" naming 模式一致；(c) 不与 V1 §7.2 接口名 `POST /chest/open` 一字一字一致 —— 接口是 "chest/open" 名词，UseCase 是 "OpenChest" 动作。
- **`pendingReward` vs `lastReward` 字段命名**：选 pending。理由：(a) "pending" 表达 "等待展示给用户"（节点 7 弹窗未关时 pendingReward != nil；关闭后 nil）；(b) "last" 暗示历史值持续保留语义不准（节点 7 关闭后即清空）。
- **`isOpening` vs `isLoadingChestOpen` 字段命名**：选 isOpening。理由：(a) 简短易读；(b) 上下文已经在 chest 域（HomeViewModel 字段中），不需要 disambiguation 前缀；(c) 与 SwiftUI 通用 `isPresented` / `isLoading` 命名风格一致。

### Project Structure Notes

- **对齐 `iphone` 工程结构（`docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §3）**：
  - Repository → `Features/Home/Repositories/`（扩既有 ChestRepository.swift，不新建）
  - UseCase → `Features/Home/UseCases/`（新建 OpenChestUseCase.swift）
  - Endpoints 工厂 → `Features/Home/UseCases/`（新建 ChestOpenEndpoints.swift；与既有 ChestEndpoints 同位置）
  - Wire DTO → `Features/Home/Models/`（新建 ChestOpenRequest / ChestOpenResponse / ChestRewardSnapshot）
  - 基础设施（IdempotencyKeyGenerator）→ `Core/Networking/`（与 APIClient / Endpoint 同 module）
  - 测试镜像生产路径 → `PetAppTests/Features/Home/{Repositories,UseCases,ViewModels}/` + `PetAppTests/Core/Networking/`
- **xcodegen 同步**：iphone target sources 用 glob recursive，无需手动编辑 project.yml；新建文件落入正确目录后跑 `xcodegen generate` 即可。
- **PetAppUITests target**：UITest 文件放 `iphone/PetAppUITests/`（与 ChestRefreshUITests / RoomUITests 同位置）。

### References

- [Source: docs/宠物互动App_V1接口设计.md#7.2 POST /api/v1/chest/open] —— 接口定义 + idempotencyKey 字符集 + 三段嵌套响应 + 错误码 + client 重试策略
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#3 五分层] —— Feature 内子目录 (Repositories / UseCases / Services / Models / Views / ViewModels)
- [Source: _bmad-output/implementation-artifacts/decisions/0010-app-state.md#3.1] —— ViewModel 禁止 @EnvironmentObject + 构造注入 AppState
- [Source: _bmad-output/implementation-artifacts/decisions/0010-app-state.md#3.2] —— Loading flag / toast / popup state 归 ViewModel @Published（不上 AppState）
- [Source: _bmad-output/implementation-artifacts/decisions/0010-app-state.md#3.3] —— hydrate / mutation 入口 `apply*` 前缀
- [Source: _bmad-output/planning-artifacts/epics.md#3062-3091] —— Story 21.3 AC 原文（含 2026-05-04 addendum 钦定写 AppState）
- [Source: _bmad-output/implementation-artifacts/21-2-get-chest-current-调用-状态展示-主动定时纠正.md] —— Story 21.2 落地的 ChestRepository / LoadChestUseCase / ChestRefreshTriggerService 接缝契约
- [Source: _bmad-output/implementation-artifacts/21-1-首页宝箱组件-swiftui.md] —— Story 21.1 落地的 ChestCardView / ChestTimerDriver / HomeViewModel.chestRemainingSeconds 接缝契约
- [Source: _bmad-output/implementation-artifacts/20-6-post-chest-open-事务-idempotencykey-加权抽取.md] —— Server 端 POST /chest/open 实装契约（idempotencyKey 落 chest_open_idempotency_records 表、加权抽取算法、错误码映射）
- [Source: iphone/PetApp/Features/Home/UseCases/SyncStepsUseCase.swift] —— 同模式 UseCase 写 AppState 单字段（applySyncedStepAccount）+ 多步编排
- [Source: iphone/PetApp/Features/Home/UseCases/LoadChestUseCase.swift] —— Story 21.2 落地的同 chest 域 UseCase（DTO 转 domain + AppState 单字段写入 + 未知 enum fail-fast）
- [Source: iphone/PetApp/Features/Home/Repositories/ChestRepository.swift] —— Story 21.2 落地的同 chest 域 Repository（value type struct + apiClient.request）
- [Source: iphone/PetApp/Features/Home/Services/StepSyncTriggerService.swift] —— Story 8.5 落地的 triggerManual() 公开入口（Story 21.5 入位将复用）
- [Source: iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift] —— 同 HomeViewModel 子类 + override pattern（onCreateTap / onJoinRoomConfirm 错误码 case-by-case 文案 + ErrorPresenter wire 模板）
- [Source: iphone/PetApp/Features/Home/Views/ChestCardView.swift] —— Story 21.1 落地的视觉契约（init 加 prop 模式 + unlockableView 内 PrimaryButton + accessibilityIdentifier(chestOpenButton)）
- [Source: iphone/PetApp/App/AppState.swift#applyCurrentChest #applySyncedStepAccount] —— 既有 mutation 入口（本 story OpenChestUseCase 双调）
- [Source: iphone/PetApp/App/AppContainer.swift#makeChestRefreshTriggerService] —— 同模式 factory + Story 21.2 已落地的 chest 链路 factory 群（本 story makeOpenChestUseCase 紧邻新增）
- [Source: iphone/PetApp/App/RootView.swift] —— 同模式 RealHomeViewModel bind wire（Story 12.7 落地的 bind(createRoomUseCase:joinRoomUseCase:...) 旁本 story 新增 bind(openChestUseCase:)）
- [Source: docs/lessons/2026-04-27-home-data-fail-fast-on-unknown-enum.md] —— 未知 enum 值 fail-fast 抛 .decoding 路径
- [Source: docs/lessons/2026-04-27-transient-vs-terminal-error-classification.md] —— 背景任务失败 silently 吞 / 用户主动操作失败弹 UI 的边界
- [Source: docs/lessons/2026-04-28-decoding-and-unauthorized-must-be-transient-retry.md] —— .decoding / .unauthorized 走 transient RetryView 而非 terminal alert
- [Source: docs/lessons/2026-05-11-business-error-fallback-must-forward-original-12-7-r8.md] —— 未知业务码必须 forward 原 error（保留 server message + requestId）
- [Source: docs/lessons/2026-05-14-idempotency-atomic-claim-and-rate-limit-honesty.md] —— Server 端 idempotency 实装锚定（client 视角：同 key 重试始终安全；1005 重试策略；1008 退役决策）
- [Source: docs/lessons/2026-04-26-stateobject-debug-instance-aliasing.md] —— 为何 service 用 @State 而非每次从 factory 拿（本 story 不直接影响 —— useCase 是 struct value-type；间接对照 service vs UseCase lifecycle 不同）
- [Source: CLAUDE.md#iOS UI 验证] —— ios-simulator MCP 必跑 verify workflow
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.1] —— XCTest only，禁止 SnapshotTesting / ViewInspector

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) via bmad-dev-story workflow.

### Debug Log References

- `bash scripts/build.sh` → BUILD SUCCESS（编译 0 error / 0 warning related to this story）
- `bash scripts/build.sh --test` → 721 tests executed, 0 failures（含本 story 新增 17 case：OpenChestUseCaseTests 10 + RealHomeViewModelChestOpenTapTests 7）
- ios-simulator MCP 实跑（iPhone 17 sim, UDID 127DECB1-...）:
  - Launch with `UITEST_SKIP_GUEST_LOGIN=1 + UITEST_MOCK_CHEST_OPEN=1` → chestCard_unlockable + 0 步 + 开宝箱 button 渲染正确
  - Tap on (320, 620) 命中 home_chestOpenButton → 0.2s 内 chest 切到 chestCard_counting "宝箱倒计时 09:56" + 步数 0→11,000
  - 完整链路 launch → fetchCurrent (mock unlockable) → AppState 写入 → tap → OpenChestUseCase.execute → mock openChest (counting nextChest + stepAccount) → 双写 AppState → ChestTimerDriver re-anchor → ChestCardView 重新渲染 counting view（端到端跑通）.
- 重入防御测试初版 race fix: `testOnChestOpenTapReentryBlockedWhenIsOpeningTrue` 需要先 waitForCallCount(execute: 1) 才能第二次 tap —— `vm.onChestOpenTap()` 是同步返回但 Task 内的 execute 调度异步, 不等 execute 真正被 mock 记录就第二次 tap 会让"重入防御"测试断言 "execute 仅被调一次" 时实际拿到 0（race）.

### Completion Notes List

- **完整开箱链路落地（AC1-8）**：iOS 客户端首次接入 POST /api/v1/chest/open。从按钮点击到 server 响应再到 AppState 双字段写入（currentChest + currentStepAccount）+ 返回 ChestRewardSnapshot 给 caller，链路在 iPhone 17 sim 实跑通过。
- **idempotencyKey 生成策略锁定**：单次 execute 调用入口 capture 一次 UUID v4 字符串，整个 Task 内复用同 key（server 端 DB UNIQUE + idempotency_records 兜底）；caller 重试调 execute 时自然新 key（spec 红线"失败后按钮恢复可点但生成新 key 用于重试"）.
- **错误码 case-by-case 文案（AC6）**：4002 / 3002 / 4001 / 1005 / 1002 各自 alert 文案；1009 + 未知 code + 非 business 错误透传给 ErrorPresenter 默认 mapper（保留 server message + requestId，复用 lesson `2026-05-11-business-error-fallback-must-forward-original.md`）.
- **isOpening 状态机三层防御**：同步段 isOpening=true 早 guard 短路（防御层 1）+ ChestCardView .disabled(isOpening) （防御层 2）+ server 端 DB UNIQUE（防御层 3）；defer 内 isOpening=false 必恢复（成功 / 失败 / cancel 三路径覆盖）.
- **未知 enum fail-fast**：nextChest.status / reward.rarity 出现 1..2 / 1..4 范围外的值即抛 ChestOpenDecodingError（AppErrorMapper 派生 RetryView "数据异常，请重试"），与 LoadChestUseCase / HomeData(from:) 同精神（lesson `2026-04-27-home-data-fail-fast-on-unknown-enum.md`）.
- **Story 21.5 入位**：OpenChestUseCase 构造签名预留 `stepSyncTriggerService: StepSyncTriggerService?` 默认 nil；AppContainer.makeOpenChestUseCase 暂传 nil；21.5 落地时改默认传 service 实例 + 单测 case#10 类型 invocation order assert 立即激活。
- **UITest 路径 wire**：UITEST_SKIP_GUEST_LOGIN=1 路径下 createRoom / joinRoom UseCase 走 nil-fallback（既有 gate），但 openChestUseCase 在两路径都注入（UITEST 路径下 ChestRepository 走 UITestMockChestRepository hook）—— 让 ChestOpenUITests 不依赖真实 server / token. errorPresenter 在 UITEST 路径仍 nil（既有 gate），但因 OpenChestUseCase 走 happy 路径 catch 不触发，UITest 不破坏 既有 r3 lesson 钦定路径。
- **新 UITEST_MOCK_CHEST_OPEN env hook**：与 UITEST_MOCK_STEP_SYNC / UITEST_MOCK_EMOJI 同模式 —— DEBUG-only init 参数 `uiTestMockChestRepository: ChestRepositoryProtocol?` + AppContainer.makeChestRepository() 优先返回 mock；UITestMockChestRepository 私有 class 仅 AppContainer 内部可达，不污染 Production binary.

### File List

**新建文件（生产 6 + 测试 4 = 10）**：

1. `iphone/PetApp/Features/Home/Models/ChestOpenRequest.swift`
2. `iphone/PetApp/Features/Home/Models/ChestOpenResponse.swift`
3. `iphone/PetApp/Features/Home/Models/ChestRewardSnapshot.swift`
4. `iphone/PetApp/Features/Home/UseCases/ChestOpenEndpoints.swift`
5. `iphone/PetApp/Features/Home/UseCases/OpenChestUseCase.swift`
6. `iphone/PetApp/Core/Networking/IdempotencyKeyGenerator.swift`
7. `iphone/PetAppTests/Core/Networking/FakeIdempotencyKeyGenerator.swift`
8. `iphone/PetAppTests/Features/Home/UseCases/OpenChestUseCaseTests.swift`
9. `iphone/PetAppTests/Features/Home/ViewModels/RealHomeViewModelChestOpenTapTests.swift`
10. `iphone/PetAppUITests/ChestOpenUITests.swift`

**改动文件（生产 7 + 测试 2 = 9）**：

11. `iphone/PetApp/Features/Home/Repositories/ChestRepository.swift`（protocol + struct 加 openChest 方法）
12. `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`（新增 isOpening / pendingReward @Published + onChestOpenTap abstract method）
13. `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift`（新增 openChestUseCase 字段 + bind 入口 + override onChestOpenTap）
14. `iphone/PetApp/Features/Home/ViewModels/MockHomeViewModel.swift`（override onChestOpenTap + Invocation.chestOpenTap）
15. `iphone/PetApp/Features/Home/Views/ChestCardView.swift`（init 加 isOpening 默认参数 + unlockableView ZStack/ProgressView 派生 + 副标题切换）
16. `iphone/PetApp/Features/Home/Views/HomeContainerView.swift`（HomeContainerHomeViewBridge.chestSlot wire onChestOpenTap + isOpening prop）
17. `iphone/PetApp/App/AppContainer.swift`（新增 makeOpenChestUseCase factory + uiTestMockChestRepository hook + UITestMockChestRepository 私有 class）
18. `iphone/PetApp/App/RootView.swift`（生产 + UITEST 双路径 RealHomeViewModel.bind(openChestUseCase:) wire）
19. `iphone/PetAppTests/Features/Home/Repositories/MockChestRepository.swift`（21.2 既有 mock 上扩 openChest stub + openChestInvocations + lastOpenChestRequest / openChestRequests）

## Change Log

| Date | Change | Reason |
|------|--------|--------|
| 2026-05-15 | 初次创建（bmad-create-story） | Epic 21 第 3 条 story；接 21.1 ChestCardView 开箱按钮 + 21.2 ChestRefreshTriggerService 已落地基础上加 OpenChestUseCase 闭环；为 Story 21.4 RewardPopupView / Story 21.5 开箱前同步步数 留接缝 |
| 2026-05-15 | dev-story 实装完成 → review | 10 新建 + 9 改动 = 19 文件落地；721 tests 0 failures（新增 17 case）；iOS sim 实跑：unlockable → tap → counting "宝箱倒计时 09:56" + 步数 0→11,000 端到端跑通；UITest happy 路径 wire 经 UITEST_MOCK_CHEST_OPEN env hook |
