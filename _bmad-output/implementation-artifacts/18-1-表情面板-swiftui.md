# Story 18.1: 表情面板 SwiftUI（首次落地 EmojisEndpoints + LoadEmojisUseCase + EmojiPanelView + EmojiCatalogStore 缓存 + 4 case 单测 + UI 测试）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iOS 开发,
I want **首次落地** iPhone 端表情链路第 1 层基础设施：
- `iphone/PetApp/Features/Emoji/UseCases/EmojisEndpoints.swift`（**新建**）—— `enum EmojisEndpoints` + `static func listEmojis() -> Endpoint`（`/api/v1/emojis` / `.get` / `body: nil` / `requiresAuth: true`，与 `HomeEndpoints` / `RoomEndpoints` 同模式；path 必含 `/api/v1` 前缀）
- `iphone/PetApp/Features/Emoji/Models/EmojiConfig.swift`（**新建**）—— `public struct EmojiConfig: Decodable, Equatable, Identifiable, Sendable` 含 `code: String` / `name: String` / `assetUrl: String` / `sortOrder: Int`；`id` computed = `code`（`code` UNIQUE KEY 保证全局唯一）；**严格对齐** V1 §11.1 `data.items[].*` 字段表 4 字段 1:1（**不**多加 `id` / `isEnabled` 等 server 未下发的字段）
- `iphone/PetApp/Features/Emoji/Models/EmojiListResponse.swift`（**新建**）—— `public struct EmojiListResponse: Decodable { public let items: [EmojiConfig] }`（与 server `GET /emojis` envelope.data 1:1，**永远非 nil**，server 端 17-4 钦定空列表返 `items: []` 而非 `null`，client 端 `items` 取 non-optional `[EmojiConfig]`）
- `iphone/PetApp/Features/Emoji/Repositories/EmojiRepository.swift`（**新建**）—— `protocol EmojiRepositoryProtocol: Sendable` + `func listEmojis() async throws -> [EmojiConfig]` + `struct DefaultEmojiRepository: EmojiRepositoryProtocol`（注入 `APIClientProtocol`，调 `apiClient.request(EmojisEndpoints.listEmojis()) as EmojiListResponse` → `response.items`）；与 `DefaultRoomRepository` / `DefaultHomeRepository` 同模式（value type struct + APIError 原样透传）
- `iphone/PetApp/Features/Emoji/UseCases/LoadEmojisUseCase.swift`（**新建**）—— `protocol LoadEmojisUseCaseProtocol: Sendable` + `func execute() async throws -> [EmojiConfig]` + `final class DefaultLoadEmojisUseCase`（class 不 struct，因要持 in-memory cache 字段；**App 生命周期内单例缓存**：首次 execute 走 repo，缓存 `[EmojiConfig]`；二次 execute 直接返缓存 + **不**再调 repo）；缓存命中读 / miss 写都用一把 `os_unfair_lock` 或 actor 保 thread-safe（首选 `actor LoadEmojisUseCase`，与 Swift 6 concurrency 兼容）
- `iphone/PetApp/Features/Emoji/Views/EmojiPanelView.swift`（**新建**）—— `public struct EmojiPanelView: View`，构造参数 `viewModel: EmojiPanelViewModel` + `onSelect: (String) -> Void` 闭包（emojiCode 回调）；启动 `.task` 调 `viewModel.load()` → load 中显示 `ProgressView`；load 失败显示 `RetryView`（复用 `Core/DesignSystem/Components/RetryView.swift`，传 `viewModel.errorMessage` + `viewModel.retry`）；load 成功用 `LazyVGrid(columns: Array(repeating: GridItem(.flexible()), count: 4))` 渲染表情网格；每个 cell `AsyncImage(url: URL(string: emoji.assetUrl))` 加载图（`.placeholder { ProgressView() }` + `.failure { Image(systemName: "questionmark.circle") }`）+ `Text(emoji.name)` 中文名 label；cell `.onTapGesture { onSelect(emoji.code) }`
- `iphone/PetApp/Features/Emoji/ViewModels/EmojiPanelViewModel.swift`（**新建**）—— `@MainActor public final class EmojiPanelViewModel: ObservableObject`；持 `@Published var state: EmojiPanelState`（enum `loading` / `loaded([EmojiConfig])` / `failed(String)`）+ `private let useCase: LoadEmojisUseCaseProtocol`；`func load() async` 路径：`state = .loading` → `try await useCase.execute()` → 成功 `state = .loaded(emojis)` / 失败 `state = .failed(mapError(error))`；`func retry()` 重新调 `load()`；error mapper 与 `Shared/ErrorHandling/ErrorPresenter.swift` 同精神（network 类 → "网络异常，请检查后重试"；business 1009 → "服务器繁忙，请稍后再试"；其他 → "加载失败，请重试"）
- `iphone/PetApp/App/AppContainer.swift` **修改** —— 在 `// MARK: - Story 15.4` 之后追加 `// MARK: - Story 18.1: Emoji UseCase factory` block：`func makeEmojiRepository() -> EmojiRepositoryProtocol` 返 `DefaultEmojiRepository(apiClient: apiClient)`；**新增** `private let loadEmojisUseCase: LoadEmojisUseCaseProtocol`（lazy 在 `init` 内 once-only 实例化为 `DefaultLoadEmojisUseCase(repository: makeEmojiRepository())`）+ `func makeLoadEmojisUseCase() -> LoadEmojisUseCaseProtocol` 返 `loadEmojisUseCase`（**返回同一实例保证 App 生命周期内缓存共享**，与 `errorPresenter` / `sessionStore` 同模式）；DEBUG / Release 两个 `init` 都要 wire（与 `webSocketClient` wire 同位置）
- `iphone/PetApp/Shared/Constants/AccessibilityID.swift` **修改** —— 追加 `public enum Emoji { ... }` nested enum：`panel`（"emojiPanel"）/ `panelLoading`（"emojiPanel_loading"）/ `panelError`（"emojiPanel_error"）/ `cell(_ code: String) -> String` 动态 helper（"emojiCell_{code}"，与 `Room.member(at:)` 同模式）
- 单元测试覆盖 ≥ 4 case：
  - `EmojiRepositoryTests.swift`（≥2 case，MockAPIClient stub）：happy 4 个 emoji response → repo.listEmojis() 返 4 项 + 字段值正确 / edge `items: []` → 返空数组不 panic
  - `LoadEmojisUseCaseTests.swift`（≥3 case，MockEmojiRepository）：happy 首次调用 → repo.listEmojis 调用 1 次 + 返 4 项 / **happy 缓存命中：第二次调用 → repo.listEmojis 调用次数仍为 1**（缓存有效）/ edge 首次失败 → 抛错 + 不缓存（缓存只在 success 路径写）
  - `EmojiPanelViewModelTests.swift`（≥3 case）：happy load 成功 → state = .loaded(4 项) / edge load 失败（mock UseCase 抛 APIError.network）→ state = .failed("网络异常...") / happy retry 后 mock 返成功 → state = .loaded
  - **UI 测试**（XCUITest，`PetAppUITests/Features/Emoji/EmojiPanelViewUITests.swift`，**新建**）：mock API mode（`UITEST_MOCK_EMOJI=1` + `UITEST_MOCK_EMOJI_JSON` env 注入 4 项 fixture）启动 App → 进任一可挂载 EmojiPanelView 的入口（**注**：18.2 才挂到房间页，本 story UI 测试用一个 **测试专用 stub host view**，仅 DEBUG build 编译，通过 `--uitest-emoji-panel-host` launch arg 触发渲染）→ 验证 4 个 `emojiCell_*` a11y 节点可见 + 点击其中 1 个验证 onSelect 回调（用 viewModel 内部 `lastSelectedCode` 字段断言）;

So that **Story 18.2（点击自己猫触发表情面板）+ Story 18.3（SendEmojiUseCase）+ Story 18.4（emoji.received 动效）+ Epic 19.1（节点 6 demo E2E）** 可以基于一个**已落地、严格符合 V1 §11.1 / V1 §12.2 client 缓存契约**的 EmojiPanelView + LoadEmojisUseCase 继续展开，不再出现"18.1 表情面板硬编码假数据 / 二次显示重复调 GET /emojis 浪费流量 / assetUrl 字段名漂移导致 client Codable 解析失败 / cache 跨 ViewModel 实例不共享导致每个 panel 实例独立缓存"的返工。

## 故事定位（Epic 18 第 1 条 = Epic 18 起点 story；上承 Epic 17 server 端 emoji 链路 done，下启 Story 18.2 ~ 18.4 + Epic 19 节点 6 demo 验收）

- **Epic 17（server emoji 链路）已 done**：17.1（契约定稿）→ 17.2（emoji_configs migration）→ 17.3（4 行 seed）→ 17.4（GET /emojis 端点）→ 17.5（WS emoji.send → emoji.received 广播）。server 端整套链路就位 + dockertest 集成测试通过；client 端 Epic 18 可放心开工，**不**需要 server 任何配套改动。
- **Epic 18 进度**：**18.1（本 story，表情面板 SwiftUI + GET /emojis 缓存）** → 18.2（房间页内点击自己猫触发表情面板）→ 18.3（选中表情触发本地立即动效 + WS emoji.send fire-and-forget）→ 18.4（接收 emoji.received 在对应成员猫上方播放飞出动效 + 去重自己 userId）。
- **本 story 是 Epic 18.2 / 18.3 / 18.4 + Epic 19.1 的强前置**：
  - **Story 18.2**：钦定"自己猫位 PetSpriteView `.onTapGesture` 触发 EmojiPanelView 弹出"（epics.md 行 2666）—— **直接依赖**本 story 落地的 `EmojiPanelView` 组件 + `EmojiPanelViewModel` 状态机
  - **Story 18.3**：钦定"用户选中表情 → SendEmojiUseCase 并行触发本地动效 + WS emoji.send fire-and-forget"（epics.md 行 2685）—— **直接依赖**本 story 落地的 `onSelect: (String) -> Void` 闭包契约（emojiCode 透传）+ `LoadEmojisUseCase` 缓存（V1 §12.2 行 2074 钦定 client 发送 emoji.send 前**应**校验 emojiCode 来自缓存的合法列表）
  - **Story 18.4**：钦定"接收 emoji.received 通过 emojiCode 查缓存拿 assetUrl 渲染飞出动效"（epics.md 行 2713 + V1 §12.3 行 2473 + 2481 钦定 emoji.received payload 仅含 `userId` / `emojiCode`，**不**含 assetUrl，client 必须查 §11.1 缓存表情列表）—— **直接依赖**本 story 落地的 `LoadEmojisUseCase.execute()` cache + `EmojiConfig` 类型 + assetUrl 字段
  - **Epic 19.1 节点 6 demo E2E**：钦定"验证场景 1（面板加载）：A 进房间 → 点自己猫 → 表情面板出现 → 验证 4 个表情图标都加载成功（assetUrl 可访问）"（epics.md 行 2742）—— **直接依赖**本 story 落地的 EmojiPanelView + assetUrl AsyncImage 加载链路
- **epics.md §Story 18.1 钦定**（行 2633-2654）：
  - **AC1**：实装 `EmojiPanelView` SwiftUI 组件 —— 启动时调 `LoadEmojisUseCase` → GET /emojis → 拿到表情列表
  - **AC2**：用 LazyVGrid 渲染（4 列），每个 cell: AsyncImage 加载 assetUrl + 表情名 label
  - **AC3**：选中后通过 closure 回调（`onSelect: (emojiCode) -> Void`）通知外部
  - **AC4**：加载中显示 ProgressView，加载失败显示 RetryView（复用 Epic 2 ErrorPresenter）
  - **AC5**：表情列表**首次加载后缓存**（同一 App 生命周期内不再重新拉取）
  - **AC6**：单元测试覆盖（≥4 case，mocked APIClient）—— happy 4 个表情 / happy 选中触发 onSelect / edge API 失败显示 RetryView / happy 缓存命中不再发 API
  - **AC7**：UI 测试覆盖（XCUITest，mock server）—— 启动 App → 进房间 → 触发表情面板 → 验证 4 个表情 cell 可见 + 点其中一个验证回调
- **V1 §11.1 钦定**（17.1 r2 review 锁定 + 冻结，行 1734-1837）：
  - HTTP Method: GET / Path: `/api/v1/emojis`
  - 认证：需要 Bearer token（`requiresAuth: true`）
  - 响应 envelope.data: `{items: [{code, name, assetUrl, sortOrder}]}` ——
    - **字段名严格小驼峰**：`assetUrl` 不是 `asset_url`，`sortOrder` 不是 `sort_order`（server 端 17.4 落地的 handler 已做 DTO 转换；client `Decodable` 模型直接对齐 server JSON 字段名，**不**走 `CodingKeys` snake_case 解析路径）
    - `items[].code`：string 必填 / length 1-64 / 字符集 `[a-z0-9_-]`（与数据库 `emoji_configs.code` UNIQUE KEY 一致）
    - `items[].name`：string 必填 / length 1-64（如 "挥手" / "爱心"）
    - `items[].assetUrl`：string 必填 / length 1-255 / **禁止空字符串 `""`**（17.3 seed 保证非空；client `String` 而非 `String?` 解码）
    - `items[].sortOrder`：number(int) 必填 / 0 ≤ value ≤ 2^31-1（int 不字符串化，与 §2.5 全局约定一致）
  - **server 端排序保证**：`ORDER BY sort_order ASC, id ASC`（次要键 id 升序保证 sort_order 相同时确定顺序）—— **client 接收时不需要二次排序**，直接按 server 顺序渲染
  - **client 缓存契约**（行 1817 钦定）：表情列表是**静态配置**，client **应**在 App 生命周期内首次拉取后缓存，后续不再重复拉取；server 端表情配置变更（admin 改 is_enabled / sort_order / 新增 emoji）需要 App 重启 / 主动刷新才能看到新值 —— 节点 6 MVP 无 push 通知机制，可接受
  - **不分页 / 不接受 query 参数**（**禁止** `?category=` / `?orderBy=` / `?page=` 等）
  - **空列表语义**：`items: []` 与 `items: null` 不同 —— server **永远**返 `[]`（17.4 钦定 `nil slice 强制 []`）；client 解码失败若遇 `null` 直接走 APIError.decoding 不做兜底
  - **错误码可能值**：1001（auth 失败 / token 过期）/ 1005（rate_limit，60 次/分）/ 1009（DB 异常）—— client 在 ViewModel 层 mapError 走 ErrorPresenter 同精神 mapper
- **V1 §12.2 行 2074 钦定**（client → server emoji.send 发送约束）："iOS client 在调用 `WebSocketClient.send` 前**应**校验 `emojiCode` 来自 §11.1 缓存的合法表情列表（取 `data.items[].code` 字段值），**禁止**直接 hardcode `emojiCode` 字面量（避免 client / server 不同步导致 7001 频发）；如 §11.1 GET /emojis 尚未拉取过 → client 应先拉取再启用表情面板"
  - **本 story 落地的影响**：`LoadEmojisUseCase.execute()` 是 §11.1 GET /emojis 的**唯一 client 入口**；18.3 SendEmojiUseCase 调 WebSocketClient.send 前**应**通过 `useCase.execute()` 取最新缓存的 emojis（cache hit 同步返；miss 才调 server）；本 story **不**实装 18.3 路径，但 `LoadEmojisUseCase.execute()` 必须满足"任意时刻调用都安全 + 缓存共享"语义
- **V1 §12.3 行 2473-2481 钦定**（emoji.received payload 仅含 `userId` / `emojiCode`，**不**含 assetUrl/name 等渲染字段）：
  - **本 story 落地的影响**：18.4 路径接收 emoji.received 时需要查 §11.1 缓存的 `[EmojiConfig]` 通过 emojiCode 取 assetUrl + name；本 story 落地的 `LoadEmojisUseCase` 缓存（**App 生命周期单例**）就是 18.4 的 single source of truth
- **iOS 架构设计 §6.9 Emoji 模块钦定**（行 400-407）：
  - 表情配置展示 / 表情面板 UI / 发送表情 / 接收表情广播后的动效提示
  - 模块组织建议：`iphone/PetApp/Features/Emoji/`（**新建**整个目录，按 Features 模块化分层组织 `Models/` / `Repositories/` / `UseCases/` / `ViewModels/` / `Views/`）—— 与 `Features/Home` / `Features/Room` 同结构
- **iOS 架构设计 §14 UI 组件建议钦定**（行 757）：`EmojiPanelView` 是建议抽出的通用业务复合组件之一；建议放 `Core/DesignSystem/Components` 或 `Shared/Components` —— **本 story 落地决策**：放 `Features/Emoji/Views/`，理由：
  - EmojiPanelView **强依赖** `EmojiPanelViewModel` + `LoadEmojisUseCase`，**不是纯样式组件**（与 RetryView / ProgressView / PrimaryButton 等纯 UI primitive 不同）
  - 与同 Features 模块下的其他 ViewModel / UseCase 文件物理位置相近，便于跨文件 navigation
  - 18.2 接入房间页时，`RoomView` 通过 sheet / overlay 弹出 `EmojiPanelView` —— 跨 Features 引用是 SwiftUI 公开 public View 的常规模式（与 `RoomMember` / `HomeUser` 跨 Features 引用同模式）
- **lesson 2026-04-25-swift-explicit-import-combine.md** 钦定：`ObservableObject` / `@Published` 必须显式 `import Combine`；`View` / `Text` / `ProgressView` 等显式 `import SwiftUI`；`URL` / `Decodable` 等显式 `import Foundation`。本 story 新建的所有文件 `import` 块必须严格遵守（看其他 ViewModel / Repository 模板）
- **lesson 2026-04-26-baseurl-host-only-contract.md** 钦定：`Endpoint.path` 自带完整路径（含 `/api/v1` 前缀），baseURL 是 host-only —— 本 story `EmojisEndpoints.listEmojis()` path 必为 `/api/v1/emojis`，**禁止**写成 `/emojis`（否则 APIClient 拼出 `http://localhost:8080/emojis` 走根路径 → server 端只有 `/ping` / `/version` 在根路径，业务接口都在 `/api/v1` 下 → 404）
- **lesson 2026-04-27-home-data-fail-fast-on-unknown-enum.md** 钦定：未知 enum 值应 fail-fast 不 silently coerce —— 本 story `EmojiConfig` 全字段都是基础类型（String / Int），**无** enum 字段（17.4 server 端 `is_enabled` 是数据库内字段不下发）；不存在该 lesson 触发风险，但保留作为类似新增 model 的参考模板
- **下游强依赖**（本 story 不动后才能开工）：
  - Story 18.2（点击自己猫触发 EmojiPanelView 弹出）—— 依赖 `EmojiPanelView` public 构造 + `onSelect` 闭包契约
  - Story 18.3（SendEmojiUseCase）—— 依赖 `LoadEmojisUseCase` 缓存共享语义（fire-and-forget 前先确认 emojiCode 在缓存）
  - Story 18.4（emoji.received 接收动效）—— 依赖 `LoadEmojisUseCase` 缓存 + `EmojiConfig.assetUrl` 字段
  - Epic 19.1（节点 6 demo E2E）—— 依赖完整 EmojiPanelView UI 链路 + AsyncImage 渲染
- **范围红线**：
  - 本 story **只**改 / 新建以下文件：
    - `iphone/PetApp/Features/Emoji/Models/EmojiConfig.swift`（**新建**）
    - `iphone/PetApp/Features/Emoji/Models/EmojiListResponse.swift`（**新建**）
    - `iphone/PetApp/Features/Emoji/UseCases/EmojisEndpoints.swift`（**新建**）
    - `iphone/PetApp/Features/Emoji/UseCases/LoadEmojisUseCase.swift`（**新建**，actor 形态首选）
    - `iphone/PetApp/Features/Emoji/Repositories/EmojiRepository.swift`（**新建**）
    - `iphone/PetApp/Features/Emoji/ViewModels/EmojiPanelViewModel.swift`（**新建**）
    - `iphone/PetApp/Features/Emoji/Views/EmojiPanelView.swift`（**新建**）
    - `iphone/PetApp/App/AppContainer.swift`（**修改**：追加 emoji factory + 单例 useCase 字段）
    - `iphone/PetApp/Shared/Constants/AccessibilityID.swift`（**修改**：追加 `enum Emoji` nested）
    - `iphone/PetAppTests/Features/Emoji/Repositories/EmojiRepositoryTests.swift`（**新建**）
    - `iphone/PetAppTests/Features/Emoji/UseCases/LoadEmojisUseCaseTests.swift`（**新建**）
    - `iphone/PetAppTests/Features/Emoji/ViewModels/EmojiPanelViewModelTests.swift`（**新建**）
    - `iphone/PetAppUITests/Features/Emoji/EmojiPanelViewUITests.swift`（**新建**）
    - 测试专用 stub host view（仅 DEBUG，路径 `iphone/PetApp/Features/DevTools/EmojiPanelHostView.swift` 或 inline 在 RootView `#if DEBUG` 块；具体由 dev 选）—— 用于 UI 测试 launch arg `--uitest-emoji-panel-host` 触发
    - 本 story 文件 + sprint-status.yaml 流转
  - **不**改 RoomView / RoomViewModel / 任何 Room 内现存文件（18.2 才接入房间页）
  - **不**实装 SendEmojiUseCase / WebSocketClient.send emoji.send（18.3 才做）
  - **不**实装 emoji.received 接收路径 / WSMessage.emojiReceived 枚举 case（18.4 才做）
  - **不**改 AppState.emojiCatalog 字段（节点 6 起 ADR-0010 §3.2 钦定占位，**但本 story 不接入** —— 缓存走 `LoadEmojisUseCase` 单例字段路径，避免 AppState 类型耦合到 `[String]` placeholder。AppState.emojiCatalog 字段未来 epic 决定如何 hydrate / 是否切换为 `[EmojiConfig]` 类型；本 story 不预判）
  - **不**修改 ADR / 不开新 ADR（按既有 ADR-0002 iOS stack / ADR-0009 navigation / ADR-0010 AppState 落地）

## Acceptance Criteria

> **AC 编号体系**：AC1 是数据 model + endpoint + repository 三层文件（V1 §11.1 字段 1:1 + path 正确）；AC2 是 LoadEmojisUseCase 缓存语义（首次 → repo / 二次 → cache hit）；AC3 是 EmojiPanelViewModel 状态机（loading / loaded / failed 三态 + retry）；AC4 是 EmojiPanelView 渲染（LazyVGrid 4 列 + AsyncImage + onSelect 闭包）；AC5 是 AppContainer wire（factory + 单例字段）；AC6 是 a11y 标识；AC7 是单元测试覆盖 ≥4 case；AC8 是 UI 测试覆盖；AC9 是 build verify + ios-simulator MCP 实跑验证；AC10 是 deliverable 清单。

### AC1: 数据 model + endpoint + repository 三层就位（V1 §11.1 字段 1:1）

**Given** Story 17.4 server `GET /api/v1/emojis` 已 done（response envelope.data = `{items: [...]}` + 4 项 seed）
**When** 完成本 story
**Then** 实装：
- `iphone/PetApp/Features/Emoji/Models/EmojiConfig.swift`：`public struct EmojiConfig: Decodable, Equatable, Identifiable, Sendable { public let code: String; public let name: String; public let assetUrl: String; public let sortOrder: Int; public var id: String { code } }` —— 字段名严格小驼峰对齐 server JSON（**禁止** `CodingKeys` snake_case 转换）+ 4 字段全部 non-optional + `Identifiable` 让 SwiftUI `ForEach` 可直接绑定 + `Sendable` 让 actor / Task 边界传递安全
- `iphone/PetApp/Features/Emoji/Models/EmojiListResponse.swift`：`public struct EmojiListResponse: Decodable { public let items: [EmojiConfig] }` —— `items` non-optional `[EmojiConfig]`（server 17.4 钦定 `nil slice 强制 []` → client 永远收到非 null array）
- `iphone/PetApp/Features/Emoji/UseCases/EmojisEndpoints.swift`：`public enum EmojisEndpoints { public static func listEmojis() -> Endpoint { Endpoint(path: "/api/v1/emojis", method: .get, body: nil, requiresAuth: true) } }` —— path 严格 `/api/v1/emojis`（host-only baseURL 契约）+ method `.get` + body `nil`（GET 无 body）+ requiresAuth `true`（V1 §11.1 元信息表"认证：需要 Bearer token"一致）
- `iphone/PetApp/Features/Emoji/Repositories/EmojiRepository.swift`：`public protocol EmojiRepositoryProtocol: Sendable { func listEmojis() async throws -> [EmojiConfig] }` + `public struct DefaultEmojiRepository: EmojiRepositoryProtocol { private let apiClient: APIClientProtocol; public init(apiClient: APIClientProtocol); public func listEmojis() async throws -> [EmojiConfig] { let response: EmojiListResponse = try await apiClient.request(EmojisEndpoints.listEmojis()); return response.items } }` —— 与 `DefaultRoomRepository` / `DefaultHomeRepository` 同模式（value type struct + APIError 原样透传，**不**在 Repository 层包错 / 转码）
**And** 所有新建文件顶部按 `iOS lesson 2026-04-25-swift-explicit-import-combine.md` 显式 import（`import Foundation` 必须，`EmojiConfig.swift` / `EmojiListResponse.swift` / `EmojisEndpoints.swift` / `EmojiRepository.swift` **无** `Combine` / `SwiftUI` 依赖，单独 `import Foundation` 即可）

### AC2: LoadEmojisUseCase 缓存语义（首次 → repo / 二次 → cache hit）

**Given** AC1 数据层就位
**When** 实装 `iphone/PetApp/Features/Emoji/UseCases/LoadEmojisUseCase.swift`
**Then**：
- 协议：`public protocol LoadEmojisUseCaseProtocol: Sendable { func execute() async throws -> [EmojiConfig] }`
- 实现：**首选 `actor DefaultLoadEmojisUseCase: LoadEmojisUseCaseProtocol`**（actor 自带串行 + thread-safe + Swift 6 concurrency 友好）；持 `private var cache: [EmojiConfig]?` 字段
- `execute()` 路径：
  1. `if let cached = cache { return cached }` —— 缓存命中直接返
  2. `let emojis = try await repository.listEmojis()` —— miss 调 repo
  3. `self.cache = emojis` —— 成功后写缓存（**只在 success 路径写**，失败保持 nil 不缓存 error）
  4. `return emojis`
- **不**实装 invalidate / reset 接口（节点 6 阶段 MVP 不需要；server 端 emoji_configs 变更要求 App 重启才生效，V1 §11.1 client 缓存契约钦定）
**And** 单元测试 `LoadEmojisUseCaseTests.swift` ≥3 case（MockEmojiRepository capture invocation count）：
- happy 首次调用 → `mockRepo.listEmojisCallCount == 1` + 返 4 项正确
- **happy 缓存命中**：先调一次 → 再调 `execute()` → `mockRepo.listEmojisCallCount` **仍为 1**（缓存有效）+ 返同一 array
- edge 首次失败：mockRepo 抛 `APIError.network` → `execute()` rethrow + `cache` 仍为 nil → 再调 `execute()` 时 mockRepo 被再次调用（不缓存失败结果；与 Story 5.5 `LoadHomeUseCase` 错误透传精神同源）

### AC3: EmojiPanelViewModel 状态机（loading / loaded / failed + retry）

**Given** AC2 LoadEmojisUseCase 就位
**When** 实装 `iphone/PetApp/Features/Emoji/ViewModels/EmojiPanelViewModel.swift`
**Then**：
- `@MainActor public final class EmojiPanelViewModel: ObservableObject`
- 状态枚举：`public enum EmojiPanelState: Equatable { case loading; case loaded([EmojiConfig]); case failed(String) }`
  - `Equatable` 让单元测试 `XCTAssertEqual(vm.state, .loaded(expectedEmojis))` 直接比对（`EmojiConfig` 已 `Equatable`）
  - `case failed(String)` payload 是 user-facing 错误文案（已经过 mapper 转换，view 层直接显示）
- `@Published public private(set) var state: EmojiPanelState = .loading`（初始 loading，view 启动 `.task` 调 `load()` 后切实际状态）
- `private let useCase: LoadEmojisUseCaseProtocol`
- `public init(useCase: LoadEmojisUseCaseProtocol)`
- `public func load() async`：
  1. `state = .loading`
  2. `do { let emojis = try await useCase.execute(); state = .loaded(emojis) } catch { state = .failed(mapError(error)) }`
- `public func retry() async`：等价 `await load()`（语义清晰：retry = 重试加载）
- `private func mapError(_ error: Error) -> String`：
  - `APIError.network` → "网络异常，请检查后重试"
  - `APIError.business(1009, _, _)` → "服务器繁忙，请稍后再试"
  - `APIError.business(1001, _, _)` / `.unauthorized` → "登录已失效，请重启 App"（理论 ADR-0008 v2 装饰器已拦截 401，但兜底）
  - `APIError.decoding` → "数据解析失败，请重试"
  - 其他（含 `.business` 其他 code）→ "加载失败，请重试"
- **不**做自动 retry / 指数退避（与 `LoadHomeUseCase` 同精神 —— 让 user 通过 RetryView 主动 retry）
**And** 单元测试 `EmojiPanelViewModelTests.swift` ≥3 case（用 `MockLoadEmojisUseCase` 实装 `LoadEmojisUseCaseProtocol`，stub `execute()` 行为）：
- happy load 成功 → `state == .loaded([4 项 emoji])`
- edge load 失败（mockUseCase 抛 `APIError.network`）→ `state == .failed("网络异常，请检查后重试")`
- happy retry 后成功：先 stub 失败 → load → state .failed → 切 stub 成功 → retry → state .loaded

### AC4: EmojiPanelView 渲染（LazyVGrid 4 列 + AsyncImage + onSelect 闭包）

**Given** AC3 ViewModel 就位
**When** 实装 `iphone/PetApp/Features/Emoji/Views/EmojiPanelView.swift`
**Then**：
- `public struct EmojiPanelView: View`
- 构造参数：
  - `@StateObject private var viewModel: EmojiPanelViewModel`（**注**：`@StateObject` 不是 `@ObservedObject` —— 让 view 自己持有 vm 生命周期 + 内部刷新时不重建 vm；caller 通过 `init(viewModel:)` 一次性传入 vm 实例，**不**让 caller 跨 view 重建 vm 时丢失 cache 状态）
  - `let onSelect: (String) -> Void`（emojiCode 闭包，caller 实装外部逻辑：18.2 关闭 panel + 18.3 触发 SendEmojiUseCase）
- `public init(viewModel: EmojiPanelViewModel, onSelect: @escaping (String) -> Void) { _viewModel = StateObject(wrappedValue: viewModel); self.onSelect = onSelect }` —— `_viewModel = StateObject(wrappedValue:)` 模式（SwiftUI 标准 @StateObject 构造注入路径，与 `RootView` 同精神）
- body 按 `viewModel.state` switch：
  - `.loading` → `ProgressView()` 居中 + `.accessibilityIdentifier(AccessibilityID.Emoji.panelLoading)`
  - `.loaded(let emojis)` → `LazyVGrid(columns: Array(repeating: GridItem(.flexible(), spacing: 12), count: 4), spacing: 12) { ForEach(emojis) { emoji in cellView(for: emoji) } }` 居中 + padding + `.accessibilityIdentifier(AccessibilityID.Emoji.panel)`
  - `.failed(let message)` → `RetryView(message: message, onRetry: { Task { await viewModel.retry() } })` + `.accessibilityIdentifier(AccessibilityID.Emoji.panelError)`
- `private func cellView(for emoji: EmojiConfig) -> some View`：
  - `VStack(spacing: 4) { AsyncImage(url: URL(string: emoji.assetUrl)) { phase in switch phase { case .empty: ProgressView(); case .success(let img): img.resizable().aspectRatio(contentMode: .fit); case .failure: Image(systemName: "questionmark.circle") } }.frame(width: 48, height: 48); Text(emoji.name).font(.caption) }`
  - `.frame(maxWidth: .infinity)` + `.padding(8)` + `.contentShape(Rectangle())`（让整个 cell 区域都可点）
  - `.onTapGesture { onSelect(emoji.code) }`
  - `.accessibilityIdentifier(AccessibilityID.Emoji.cell(emoji.code))`
- view 启动 `.task { await viewModel.load() }`（首次出现时触发加载；与 RoomView 同精神）
**And** import 必须含 `import SwiftUI` + `import Foundation`（**不**含 `Combine` —— 本文件无 ObservableObject 直接订阅；vm 通过 @StateObject 间接订阅，SwiftUI 自动处理）
**And** `#if DEBUG` 块加 `PreviewProvider` 提供 3 种预览：loading / loaded(4 emojis fixture) / failed("preview 错误文案")（让 dev 在 Xcode Canvas 直接验证 3 态视觉）

### AC5: AppContainer wire（factory + 单例字段共享缓存）

**Given** AC1 ~ AC4 全部就位
**When** 修改 `iphone/PetApp/App/AppContainer.swift`
**Then**：
- 在文件顶部字段声明区追加：
  ```swift
  /// Story 18.1: 全 App 共享的 LoadEmojisUseCase 单例.
  /// emojis 缓存语义钦定 "App 生命周期内一次加载" (V1 §11.1 client 缓存契约) —— actor 内部 cache 字段
  /// 必须跨 ViewModel 共享, 故走 stable singleton 模式 (与 errorPresenter / sessionStore 同精神),
  /// **禁止** 走 makeXxx() factory 模式 (每次 new 实例会让 cache 失效).
  public let loadEmojisUseCase: LoadEmojisUseCaseProtocol
  ```
- 在两个 `init` 内（DEBUG + Release）追加 wire：
  ```swift
  self.loadEmojisUseCase = DefaultLoadEmojisUseCase(repository: DefaultEmojiRepository(apiClient: apiClient))
  ```
  位置：放在 `self.errorPresenter = ErrorPresenter()` 附近 + 在 `self.sessionStore = SessionStore()` 之后（同属"全 App 共享 actor / value type 单例"组）
- 在 `// MARK: - Story 15.4` 之后追加新 MARK：
  ```swift
  // MARK: - Story 18.1 AC5: Emoji 链路 factory

  /// Story 18.1: 构造 EmojiRepository (每次调用返回新实例; apiClient 单例由 container 持有).
  /// 与 makeStepRepository / makeRoomRepository 同模式 (value type struct, 构造廉价).
  public func makeEmojiRepository() -> EmojiRepositoryProtocol {
      DefaultEmojiRepository(apiClient: apiClient)
  }

  /// Story 18.1: 构造 EmojiPanelViewModel (每次调用返回新实例; useCase 是 stable singleton 共享 cache).
  /// caller=任意挂载 EmojiPanelView 的 view (Story 18.2 RoomView; Story 18.1 UI 测试 stub host view).
  public func makeEmojiPanelViewModel() -> EmojiPanelViewModel {
      EmojiPanelViewModel(useCase: loadEmojisUseCase)
  }
  ```
**And** **不**让 `loadEmojisUseCase` 走 factory `makeLoadEmojisUseCase()` 模式（factory 模式 each call new 实例会让 cache 失效；与 errorPresenter / sessionStore 同精神，stable singleton 字段直接暴露给 caller）
**And** DEBUG / Release init 全部 wire 同样路径（**禁止**仅 DEBUG 落地导致 Release build 找不到 useCase 实例引发 nil crash）

### AC6: a11y 标识（emojiPanel / emojiPanel_loading / emojiPanel_error / emojiCell_{code}）

**Given** AC4 EmojiPanelView 就位
**When** 修改 `iphone/PetApp/Shared/Constants/AccessibilityID.swift`
**Then** 追加新 nested enum（位置：放在 `enum Compose { ... }` 之后，文件末尾倒数第二个 `}` 之前）：
```swift
/// Story 18.1 落地的 EmojiPanelView a11y identifier.
public enum Emoji {
    /// EmojiPanelView 根容器 (loaded 态时挂在 LazyVGrid 上, 用于 UITest 定位整个面板).
    public static let panel = "emojiPanel"
    /// loading 态 ProgressView 标识 (UITest 验证 loading 占位是否出现).
    public static let panelLoading = "emojiPanel_loading"
    /// failed 态 RetryView 标识 (UITest 验证错误态降级是否出现).
    public static let panelError = "emojiPanel_error"
    /// 单个表情 cell 模式: `emojiCell_<code>` (e.g. "emojiCell_wave"); caller 走 helper.
    /// 与 Room.member(at:) 同模式 —— UITest 用 `app.buttons["emojiCell_wave"].tap()` 选中具体表情.
    public static func cell(_ code: String) -> String { "emojiCell_\(code)" }
}
```
**And** EmojiPanelView 实装内严格用本 enum 常量（**禁止** inline `"emojiPanel"` 字符串字面量，与 Story 37.13 a11y 总表归并精神一致）

### AC7: 单元测试覆盖 ≥4 case（已在 AC1 ~ AC3 散点钦定，总计 8 case）

**Given** AC1 ~ AC4 全部就位
**When** 落地以下测试文件
**Then**：
- `iphone/PetAppTests/Features/Emoji/Repositories/EmojiRepositoryTests.swift`：≥2 case（MockAPIClient）
  - happy: stub `apiClient.request` 返 `EmojiListResponse(items: [wave, love, laugh, cry])` → `repo.listEmojis()` 返 4 项 + 字段值精确匹配
  - edge: stub `apiClient.request` 返 `EmojiListResponse(items: [])` → `repo.listEmojis()` 返 `[]` 不 panic
- `iphone/PetAppTests/Features/Emoji/UseCases/LoadEmojisUseCaseTests.swift`：≥3 case（MockEmojiRepository capture call count）
  - happy 首次调用 → `mockRepo.callCount == 1` + 返 4 项
  - **happy 缓存命中**：调一次 → 调第二次 → `mockRepo.callCount == 1`（仍是 1，缓存有效）+ 两次返回值 Equatable 相等
  - edge 失败不缓存：stub 抛 `APIError.network` → 第一次 execute 抛错 + `mockRepo.callCount == 1` → 切 stub 为成功 + 再调 execute → 成功 + `mockRepo.callCount == 2`（说明失败未污染 cache）
- `iphone/PetAppTests/Features/Emoji/ViewModels/EmojiPanelViewModelTests.swift`：≥3 case（MockLoadEmojisUseCase）
  - happy load 成功 → `vm.state == .loaded(expectedEmojis)`
  - edge load 失败 `APIError.network` → `vm.state == .failed("网络异常，请检查后重试")`
  - happy retry 后成功：mock 第一次抛错 → load → `.failed` → mock 切成功 → retry → `.loaded`
**And** mock 类型放各测试文件**内部** `private final class` 形态（与 PetAppTests 内 MockAPIClient / MockKeychainStore 同模式；**禁止**在 production 文件挂 Mock）
**And** 所有测试用 `XCTest` framework；命名 `func test_<scenario>_<expectation>()`（与 `RoomEndpointsTests` / `LoadHomeUseCaseTests` 风格一致）

### AC8: UI 测试覆盖（XCUITest，mock API mode）

**Given** AC1 ~ AC7 全部就位
**When** 落地 `iphone/PetAppUITests/Features/Emoji/EmojiPanelViewUITests.swift`
**Then**：
- 测试启动用 `app.launchArguments = ["--uitest-emoji-panel-host"]` + `app.launchEnvironment = ["UITEST_MOCK_EMOJI": "1", "UITEST_MOCK_EMOJI_JSON": <4 项 fixture JSON 字符串>]`（与 `UITEST_MOCK_STEP_SYNC` 同模式，注入 mock 数据避开真实 server 调用）
- AppContainer 需要追加 DEBUG-only mock support：
  - 类似 `UITestMockStepRepository` 模式，新增 `private final class UITestMockEmojiRepository: EmojiRepositoryProtocol` + `init(stubEmojis: [EmojiConfig])` + `listEmojis() async throws -> [EmojiConfig] { stubEmojis }`
  - `AppContainer` DEBUG `init` 内解析 `UITEST_MOCK_EMOJI=1` env 时 wire mock repo 进 `loadEmojisUseCase`（替换默认 `DefaultEmojiRepository`）
- RootView 需要追加 DEBUG-only stub host view path：
  - 检查 `ProcessInfo.processInfo.arguments.contains("--uitest-emoji-panel-host")` → 渲染一个全屏的 `EmojiPanelHostView` 而非 MainTabView（仅 UITest 路径触发，正常启动不变）
  - `EmojiPanelHostView`：内嵌 `EmojiPanelView(viewModel: container.makeEmojiPanelViewModel(), onSelect: { code in self.lastSelectedCode = code })` + 一个隐藏 `Text(lastSelectedCode ?? "").accessibilityIdentifier("emojiPanel_uitestSelectedCode")` 用于断言 onSelect 回调
- 测试 case ≥2：
  - happy: 启动 mock mode → 等 `app.otherElements["emojiPanel"]` 出现 → 验证 `app.buttons["emojiCell_wave"]` / `emojiCell_love` / `emojiCell_laugh` / `emojiCell_cry` 4 个 cell 可见
  - happy: 点 `emojiCell_wave` → 验证 `app.staticTexts["emojiPanel_uitestSelectedCode"].label == "wave"`（onSelect 回调验证）
**And** UI 测试 **不**做"二次显示不重新加载"验证（缓存命中行为已在 unit test AC7 覆盖；UI 测试只验证视觉 + 交互，不验证内部 cache 字段）
**And** 若 dev 评估 UITest mock injection 成本过高（DEBUG-only AppContainer 改动 + RootView stub host view + launch arg/env 解析）可降级为 unit-test-only 覆盖 + 在 dev notes 登记 tech debt（"AC8 UI 测试 deferred 到 18.2，待 EmojiPanelView 接入 RoomView 后通过房间页 mock 路径覆盖"）—— 但 **18.2 落地时必须补回**，不能跨 epic 拖到 Epic 19.1 E2E 才发现 UI 链路 bug

### AC9: build verify + ios-simulator MCP 实跑验证

**Given** AC1 ~ AC8 全部就位
**When** 验证
**Then**：
- `bash iphone/scripts/build.sh` 通过（vet + build → DerivedData/.../PetApp.app）
- 在 ios-simulator MCP 内实跑：
  1. `install_app(app_path: iphone/build/DerivedData/Build/Products/Debug-iphonesimulator/PetApp.app)`
  2. `launch_app(bundle_id: "com.zhuming.pet.app", arguments: ["--uitest-emoji-panel-host"], environment: {"UITEST_MOCK_EMOJI": "1", "UITEST_MOCK_EMOJI_JSON": "<4 项 fixture>"}, terminate_running: true)`
  3. `ui_view` —— 看 EmojiPanelView 渲染（4 个表情网格 + 中文名）
  4. `ui_tap(...)` 点 wave cell → 再 `ui_view` 验证（虽然 onSelect 在 stub host 内只更新隐藏 Text，但视觉应保持稳定不 crash）
  5. `ui_describe_all` 验证 a11y 树含 `emojiPanel` + `emojiCell_wave` 等节点
- **CLAUDE.md "iOS UI 验证（必跑）" 段**钦定：iOS UI / feature 改动必须用 `ios-simulator` MCP server 在模拟器里实跑验证；**不能只跑 `bash iphone/scripts/build.sh` 就报告 done** —— xcodebuild 通过只验证 code 编译正确，不验证 UI / feature 行为正确

### AC10: Deliverable 清单

**Given** AC1 ~ AC9 全部就位
**When** 收官
**Then** 产出以下文件（按"范围红线"清单一字不差）：

- **新建文件**（13 个）：
  1. `iphone/PetApp/Features/Emoji/Models/EmojiConfig.swift`
  2. `iphone/PetApp/Features/Emoji/Models/EmojiListResponse.swift`
  3. `iphone/PetApp/Features/Emoji/UseCases/EmojisEndpoints.swift`
  4. `iphone/PetApp/Features/Emoji/UseCases/LoadEmojisUseCase.swift`
  5. `iphone/PetApp/Features/Emoji/Repositories/EmojiRepository.swift`
  6. `iphone/PetApp/Features/Emoji/ViewModels/EmojiPanelViewModel.swift`
  7. `iphone/PetApp/Features/Emoji/Views/EmojiPanelView.swift`
  8. `iphone/PetAppTests/Features/Emoji/Repositories/EmojiRepositoryTests.swift`
  9. `iphone/PetAppTests/Features/Emoji/UseCases/LoadEmojisUseCaseTests.swift`
  10. `iphone/PetAppTests/Features/Emoji/ViewModels/EmojiPanelViewModelTests.swift`
  11. `iphone/PetAppUITests/Features/Emoji/EmojiPanelViewUITests.swift`
  12. （可选，AC8 stub host）`iphone/PetApp/Features/DevTools/EmojiPanelHostView.swift`（DEBUG-only）
  13. （可选，AC8 mock repo）若 dev 选独立文件而非 inline，新建 `iphone/PetApp/Features/Emoji/Testing/UITestMockEmojiRepository.swift`（DEBUG-only）
- **修改文件**（2 个）：
  1. `iphone/PetApp/App/AppContainer.swift`（追加 `loadEmojisUseCase` 字段 + `makeEmojiRepository` / `makeEmojiPanelViewModel` factory + DEBUG mock wire）
  2. `iphone/PetApp/Shared/Constants/AccessibilityID.swift`（追加 `enum Emoji`）
- **流程文件**（1 个）：
  - `_bmad-output/implementation-artifacts/sprint-status.yaml`（本 story `ready-for-dev` → 后续 `dev-story` → `in-progress` → `review` → `done`，epic-18 从 `backlog` 改 `in-progress`）

## Tasks / Subtasks

- [x] **Task 1**（AC1）：落地数据 model + endpoint + repository 三层
  - [x] 创建 `iphone/PetApp/Features/Emoji/` 目录结构（Models / UseCases / Repositories / ViewModels / Views）
  - [x] 写 `EmojiConfig.swift`（4 字段 + Decodable + Equatable + Identifiable + Sendable）
  - [x] 写 `EmojiListResponse.swift`（`items: [EmojiConfig]`）
  - [x] 写 `EmojisEndpoints.swift`（path `/api/v1/emojis` + .get + requiresAuth true）
  - [x] 写 `EmojiRepository.swift`（protocol + DefaultEmojiRepository struct，注入 APIClient）

- [x] **Task 2**（AC2）：落地 LoadEmojisUseCase + 缓存
  - [x] 写 `LoadEmojisUseCase.swift`（actor + protocol + cache 字段 + execute 路径）
  - [x] 写 `LoadEmojisUseCaseTests.swift`（≥3 case：首次 / cache 命中 / 失败不缓存 + 空列表也缓存，共 4 case）

- [x] **Task 3**（AC3）：落地 EmojiPanelViewModel 状态机
  - [x] 写 `EmojiPanelViewModel.swift`（@MainActor + @Published state + load / retry + mapError）
  - [x] 写 `EmojiPanelViewModelTests.swift`（≥3 case：load 成功 / load 失败 / retry 后成功 + business 1009 / unauthorized / decoding mapper，共 6 case）

- [x] **Task 4**（AC4）：落地 EmojiPanelView UI
  - [x] 写 `EmojiPanelView.swift`（@StateObject vm + onSelect 闭包 + 3 态分支 + LazyVGrid + AsyncImage cell + .task load）
  - [x] 写 PreviewProvider（loading / loaded / failed 3 态）
  - [x] cell `.accessibilityElement(children: .combine)` 折叠成单 a11y 节点（防 UITest 多匹配歧义）

- [x] **Task 5**（AC5）：AppContainer wire
  - [x] 追加 `loadEmojisUseCase` 字段（stable singleton 模式）
  - [x] DEBUG / Release `init` 双路径 wire 实例化
  - [x] 追加 `makeEmojiRepository` + `makeEmojiPanelViewModel` factory（@MainActor）

- [x] **Task 6**（AC6）：a11y 标识
  - [x] 修改 `AccessibilityID.swift` 追加 `enum Emoji`（panel / panelLoading / panelError / cell helper + uitestSelectedCode）
  - [x] EmojiPanelView 改用常量替换 inline 字符串

- [x] **Task 7**（AC7）：补 EmojiRepositoryTests（unit test）
  - [x] 写 `EmojiRepositoryTests.swift`（5 case：happy 4 项 / endpoint 严格契约 / edge 空列表 / business 1009 透传 / network 透传）

- [x] **Task 8**（AC8）：UI 测试 + DEBUG-only mock 注入路径
  - [x] 追加 `UITestMockEmojiRepository`（DEBUG-only，inline AppContainer 底部）
  - [x] 追加 `EmojiPanelHostView`（DEBUG-only stub host view，`Features/DevTools/Views/`）
  - [x] 修改 RootView 解析 `--uitest-emoji-panel-host` launch arg → 渲染 stub host（body 内 `#if DEBUG` 拦截）
  - [x] 修改 AppContainer DEBUG `convenience init` 解析 `UITEST_MOCK_EMOJI` env → wire mock repo（同时支持 `UITEST_MOCK_EMOJI_JSON` 自定义 fixture）
  - [x] 写 `EmojiPanelViewUITests.swift`（2 case：可见性 / onSelect 回调）

- [x] **Task 9**（AC9）：build verify + ios-simulator MCP 实跑
  - [x] xcodebuild build pass（destination 用 iPhone 17,OS=latest）
  - [x] 628 单测全绿（PetAppTests 含新增 15 case 全 pass，无回归）
  - [x] UI 测试 2 case 全 pass（EmojiPanelViewUITests，UDID `iPhone 17` 路径）
  - [x] ios-simulator MCP 实跑验证：install_app + launch + ui_view + ui_describe_all + ui_tap → 4 cell 渲染正确 + emojiPanel_uitestSelectedCode `wave` label 写入

- [x] **Task 10**（AC10）：deliverable + sprint-status 流转
  - [x] 自检 13 个新建 + 2 个修改文件全部就位
  - [x] sprint-status.yaml `ready-for-dev` → `in-progress`（dev-story 入口）→ `review`（本 story 收尾）

## Dev Notes

> 本段是给 dev agent 的"实装提示 + 选型理由 + 陷阱清单"，全部基于 V1 §11.1 / §12.2 / §12.3 + iOS 架构 §6.9 / §14 + 已落地 Story 17.4 / 17.5 contracts。

### 1. 字段命名严格小驼峰对齐 server JSON

- server 17.4 落地的 `emojis_handler.go` 已做 DTO 转换：`asset_url` → `assetUrl`、`sort_order` → `sortOrder`；client `EmojiConfig` 4 字段直接小驼峰命名匹配，**不**走 `CodingKeys` snake_case 解析路径
- **反例**（**禁止**）：
  ```swift
  // 错误：会导致解码失败，因为 server 发的是 assetUrl 而非 asset_url
  enum CodingKeys: String, CodingKey {
      case code
      case name
      case assetUrl = "asset_url"  // ❌
      case sortOrder = "sort_order"  // ❌
  }
  ```
- **正解**：4 字段名直接对齐 server JSON，**省略** `CodingKeys`（Swift Codable 默认按字段名映射）

### 2. AsyncImage 渲染细节

- iOS 16+ `AsyncImage(url:content:)` 支持 phase-based rendering（`.empty` / `.success` / `.failure`）—— PetApp deployment target 已是 iOS 16+（见 project.yml），可直接用
- `assetUrl` 是 server 下发的 placeholder URL（如 `https://placehold.co/64x64?text=Wave`），MVP 阶段不强求真实 CDN 图；AsyncImage 自带超时/缓存（URLCache 默认 4MB memory + 20MB disk），不需要 client 额外做图片缓存
- **陷阱**：AsyncImage 不带 retry 按钮；如 URL 加载失败，自动走 `.failure` 显示 `Image(systemName: "questionmark.circle")` —— 这是预期行为（contract 钦定 emoji.received 路径接收 emojiCode 找不到 assetUrl 时 fallback 问号；本 story 列表加载阶段同精神 fallback）
- **不**在 cell 内做 image retry 逻辑（V1 §11.1 client 缓存契约钦定 "App 生命周期内首次拉取 + 不重复拉取"，image URL 解析失败由 user 重启 App 或下次 GET /emojis 时获取新 URL；MVP 接受）

### 3. actor vs class 选择 LoadEmojisUseCase

- **首选 `actor DefaultLoadEmojisUseCase`**：
  - Swift 6 concurrency 友好（无需手动加锁）
  - cache 字段读写自动串行（多个 ViewModel 同时调 `execute()` 时不会触发 data race）
  - 与 Swift / SwiftUI 主流方向一致
- **备选 `final class DefaultLoadEmojisUseCase` + `os_unfair_lock`**（仅当 actor 出现意外限制时）：
  - 手动加锁包 cache 读 / 写
  - `Sendable` 通过 `@unchecked Sendable` 显式标记
  - 仅在 actor 路径有阻塞性问题时退路（**不建议**默认选择）
- **陷阱**：actor 内的 cache 字段 `execute()` 是 async；caller 必须 await；与 `LoadHomeUseCase` 直接 throws async 风格一致

### 4. AppContainer 单例字段 vs factory 模式

- **stable singleton 字段**（本 story 选择）：`loadEmojisUseCase` 在 init 内 once-only 实例化 → 跨 ViewModel 共享同一实例 → cache 自然共享
- **factory 模式**（**禁止**）：`makeLoadEmojisUseCase()` each call new 实例 → 每个 ViewModel 拿到自己的 useCase → cache 不共享 → 每个 EmojiPanelView 实例首次显示都触发 GET /emojis → 违反 V1 §11.1 client 缓存契约
- 模板：与 `errorPresenter` / `sessionStore` / `webSocketClient` 同精神（stable singleton）
- `makeEmojiPanelViewModel()` 仍走 factory 模式（每次 new EmojiPanelViewModel 但**注入同一 useCase 单例**）—— ViewModel 是 @MainActor + @Published 状态机，每个 EmojiPanelView 实例需要独立 ViewModel；但 useCase 共享 = cache 共享

### 5. SwiftUI @StateObject vs @ObservedObject

- **EmojiPanelView 选 @StateObject**：让 view 自己持有 vm 生命周期（view 重建时 SwiftUI 自动保持同一 vm 实例；不会因父 view 刷新而丢 state）
- **错误用法**（**禁止**）：`@ObservedObject` —— vm 由 caller 持有，caller 每次重建 view 时 vm 也重建 → load 路径反复触发 → 行为异常
- 构造模式：`@StateObject private var viewModel: EmojiPanelViewModel` + `init(viewModel: EmojiPanelViewModel, ...) { _viewModel = StateObject(wrappedValue: viewModel); ... }`（**SwiftUI 标准 @StateObject DI 模式**，与 RootView 模式同源）

### 6. 错误 mapper 与 ErrorPresenter 协同

- 本 story EmojiPanelView 走自管 RetryView 路径（vm.state == .failed 时直接渲染 RetryView 而非通过 ErrorPresenter）
- **理由**：表情面板是"局部 sheet/overlay"，加载失败不应阻塞整个 App（不像 LoadHomeUseCase 失败阻塞 bootstrap）；用 RetryView 局部展示让用户主动重试
- ErrorPresenter（全局 toast / alert / retry）由 18.3 SendEmojiUseCase 复用（"网络不佳，对方可能看不到"温和 toast，epics.md 行 2691 钦定）—— 本 story **不**走 ErrorPresenter 路径

### 7. 测试 mock 工厂模式

- 各测试文件**内部**用 `private final class MockXxx: XxxProtocol` 形态：
  - 与 `MockAPIClient` / `InMemoryKeychainStore` 同模式
  - 不污染 production 编译；不与其他 test target 共享 mock；测试间隔离
- 例（MockEmojiRepository）：
  ```swift
  private final class MockEmojiRepository: EmojiRepositoryProtocol, @unchecked Sendable {
      var stubResult: Result<[EmojiConfig], Error> = .success([])
      var callCount = 0
      func listEmojis() async throws -> [EmojiConfig] {
          callCount += 1
          return try stubResult.get()
      }
  }
  ```
- `@unchecked Sendable` 是因为 callCount 是 mutable 字段；测试串行调用所以 race-free（与 `UITestMockStepRepository` 同精神）

### 8. UI 测试 mock 注入路径决策

- **优选方案（AC8 钦定）**：
  - DEBUG-only `UITestMockEmojiRepository` + `UITEST_MOCK_EMOJI=1` env 触发 AppContainer wire mock
  - `--uitest-emoji-panel-host` launch arg + RootView 解析 → 渲染 `EmojiPanelHostView` 全屏 stub
  - `EmojiPanelHostView` 内挂 `EmojiPanelView` + 隐藏 `Text(lastSelectedCode).accessibilityIdentifier(...)` 用于断言 onSelect 回调
- **降级方案**（dev 可选）：
  - 不做 UITest，仅 unit test 覆盖（AC7 已覆盖 ViewModel / UseCase / Repository 单测全路径）
  - 在 dev notes 登记 tech debt "UI 测试 deferred to Story 18.2"
  - **但 18.2 落地时必须补回**：18.2 EmojiPanelView 接入房间页后，UITest 直接通过房间页路径覆盖（进房间 → 点自己猫 → EmojiPanelView 弹出 → 验证 4 cell + onSelect）
- **不能跨 epic 拖到 19.1 E2E 才发现 UI 链路 bug** —— epic 19 是 demo 验收，发现 bug 已经太晚

### 9. iOS 架构 §14 EmojiPanelView 放 Features/Emoji/Views 而非 Core/DesignSystem/Components

- 架构 §14 行 757 建议 `EmojiPanelView` 放 `Core/DesignSystem/Components` 或 `Shared/Components`
- **本 story 决策放 `Features/Emoji/Views/`**，理由：
  - EmojiPanelView **强依赖** EmojiPanelViewModel + LoadEmojisUseCase + EmojiConfig（业务复合组件）
  - 不是纯样式 primitive（与 RetryView / PrimaryButton / Avatar 等不同）
  - 与同模块的 ViewModel / UseCase 物理位置相近，便于跨文件 navigation
  - **架构 §14 是"建议"非"强制"**，遵循 ADR-0002 §3.3 "iOS 工程结构灵活按 Features 模块化"精神

### 10. lesson 引用清单（dev agent 实装前必读）

- `docs/lessons/2026-04-25-swift-explicit-import-combine.md` —— Combine / SwiftUI / Foundation 显式 import；本 story 所有新建文件必须遵守
- `docs/lessons/2026-04-26-baseurl-host-only-contract.md` —— Endpoint.path 自带 `/api/v1` 前缀；EmojisEndpoints.listEmojis() 必为 `/api/v1/emojis`
- `docs/lessons/2026-04-26-stateobject-debug-instance-aliasing.md` —— @StateObject 生命周期；EmojiPanelView 用 @StateObject 而非 @ObservedObject
- `docs/lessons/2026-04-27-home-data-fail-fast-on-unknown-enum.md` —— 未知字段 fail-fast；本 story 无 enum 字段，但模式参考
- `docs/lessons/2026-04-27-transient-vs-terminal-error-classification.md` —— 错误分类；EmojiPanelViewModel.mapError 按 transient（network/1009 → 文案 retry）/ terminal（unauthorized → "请重启 App"）分流
- `docs/lessons/2026-05-13-emoji-contract-self-consistency-and-1009-and-asset-url-17-1-r2.md` —— Story 17.1 r2 review 完整 lesson（assetUrl 必非空、1009 路径、字符集约束）；本 story client 端对齐 server 端契约保证一致性

### Project Structure Notes

- 与 `iOS 架构设计 §6.9 Emoji 模块`一致：新建 `iphone/PetApp/Features/Emoji/` 完整模块（Models / Repositories / UseCases / ViewModels / Views 五层）
- 与 `Features/Home` / `Features/Room` 模块结构对齐，遵循 ADR-0002 iPhone stack 钦定
- 测试侧 `iphone/PetAppTests/Features/Emoji/` 镜像 production 结构（Repositories / UseCases / ViewModels 三层）
- UI 测试 `iphone/PetAppUITests/Features/Emoji/` 单文件 `EmojiPanelViewUITests.swift`（与既有 PetAppUITests 风格一致，feature 名 + UITests.swift 后缀）

### References

- [Source: `_bmad-output/planning-artifacts/epics.md#Story 18.1`]（行 2633-2654 完整 AC + 6 段子项）
- [Source: `_bmad-output/planning-artifacts/epics.md#Epic 18 总体`]（行 2629-2729 完整 Epic 18 上下文）
- [Source: `docs/宠物互动App_V1接口设计.md#§11.1 GET /api/v1/emojis`]（行 1734-1837 完整 schema + client 缓存契约钦定）
- [Source: `docs/宠物互动App_V1接口设计.md#§12.2 emoji.send`]（行 1981-2080，特别是行 2074 client 端发送约束）
- [Source: `docs/宠物互动App_V1接口设计.md#§12.3 emoji.received`]（行 2435-2475 + 行 2481 emoji.received payload 不含 assetUrl/name 钦定）
- [Source: `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#§6.9 Emoji 模块`]（行 400-407 模块职责）
- [Source: `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#§14 UI 组件建议`]（行 757 EmojiPanelView 抽取建议）
- [Source: `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md`]（iPhone stack ADR，钦定 iphone/PetApp 目录 + Features 模块化）
- [Source: `_bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md`]（ADR-0009 iPhone navigation，本 story 不接入房间页但 18.2 依赖）
- [Source: `_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md`]（ADR-0010 AppState，emojiCatalog 字段占位语义；本 story 不动）
- [Source: `_bmad-output/implementation-artifacts/17-4-get-emojis-接口.md`]（server 端 GET /emojis 落地参考）
- [Source: `_bmad-output/implementation-artifacts/17-5-ws-emoji-send-处理-emoji-received-广播.md`]（server 端 WS emoji.send / emoji.received 落地参考）
- [Source: `iphone/PetApp/Features/Home/UseCases/LoadHomeUseCase.swift`]（UseCase 模板：协议 + DefaultXxxUseCase struct + repository 注入 + 错误透传）
- [Source: `iphone/PetApp/Features/Room/Repositories/RoomRepository.swift`]（Repository 模板：protocol Sendable + value type struct + apiClient 注入）
- [Source: `iphone/PetApp/Features/Home/UseCases/HomeEndpoints.swift`]（Endpoint 模板：path `/api/v1/...` + requiresAuth）
- [Source: `iphone/PetApp/Core/DesignSystem/Components/RetryView.swift`]（RetryView 复用模板）
- [Source: `iphone/PetApp/Shared/Constants/AccessibilityID.swift`]（a11y 总表 + nested enum 添加模板）
- [Source: `iphone/PetApp/App/AppContainer.swift`]（DI container 模板 + stable singleton 字段 vs factory 模式区别）
- [Source: `CLAUDE.md` "iOS UI 验证（必跑）" 段]（ios-simulator MCP 实跑验证强制要求）

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context), 通过 `bmad-dev-story` workflow + ios-simulator MCP 实跑验证.

### Debug Log References

- 红绿循环：先写测试 (5 + 4 + 6 共 15 unit case + 2 UITest case) → 实装 → xcodebuild test 全绿.
- xcodebuild destination 环境陷阱：iOS 26.5 SDK installed 但 iOS 26.5 runtime 未装；`-destination "platform=iOS Simulator,name=iPhone 17,OS=latest"` 可命中 iOS 26.4 runtime (走 fallback latest 解析)；
  指定 `id=<iPhone 17 Pro UDID>` 时 xcodebuild 间歇性 "Supported platforms for the buildables in the current scheme is empty" → 切到 `iPhone 17` (non-Pro) UDID 后稳定 pass.
- SwiftUI `.accessibilityIdentifier` 默认会传播到子 view (Image + Text 同时拿到 `emojiCell_wave`)；通过 `.accessibilityElement(children: .combine)` 把 cell 折叠成单一 a11y 节点 + `.accessibilityAddTraits(.isButton)` 让 UITest 拿到一个 AXButton 元素 (避免多匹配 tap 歧义).
- mock test 用 `actor MockEmojiRepository` 而非 class+lock：actor 自带串行 + Sendable，与 LoadEmojisUseCase actor 跨 actor 调用时 callCount 读写 race-free.
- stash/pop 时 xcodegen 重生的 project.pbxproj 会冲突；处理路径：`git checkout PetApp.xcodeproj/project.pbxproj` 后再 `git stash pop`.

### Completion Notes List

- ✅ AC1 数据三层 (Models/UseCases/Repositories) 严格按 V1 §11.1 字段名小驼峰对齐 server JSON，**省略** `CodingKeys` (lesson `2026-04-25-swift-explicit-import-combine.md` + Dev Note #1).
- ✅ AC2 LoadEmojisUseCase 采用 `actor` 形态实现 cache 共享 (Swift 6 concurrency 友好；多 ViewModel 并发 race-free)；只在 success 路径写 cache (失败不污染).
- ✅ AC3 EmojiPanelViewModel 状态机 + 完整 mapError 文案覆盖 (network / business 1009 / business 1001 / unauthorized / decoding / missingCredentials / localStoreFailure).
- ✅ AC4 EmojiPanelView 使用 `@StateObject` (而非 `@ObservedObject`) 保 vm 生命周期；3 态分支 (.loading / .loaded / .failed) + PreviewProvider 三态预览.
- ✅ AC5 AppContainer 走 stable singleton 模式 wire `loadEmojisUseCase` (跨 ViewModel cache 共享)；DEBUG / Release init 双路径都 wire；DEBUG init 多加 `emojiRepository: EmojiRepositoryProtocol?` 注入参数支持 UITest mock.
- ✅ AC6 a11y enum `Emoji.panel / panelLoading / panelError / cell(_:)` + 多加 `uitestSelectedCode` 给 AC8 用.
- ✅ AC7 单测 15 case 全绿 (Repo 5 + UseCase 4 + ViewModel 6)，超过 ≥4 case 钦定下限.
- ✅ AC8 UITest 2 case 全绿；mock 注入路径：`UITEST_MOCK_EMOJI=1` + `UITEST_MOCK_EMOJI_JSON` env + `--uitest-emoji-panel-host` launch arg + DEBUG-only `UITestMockEmojiRepository` (inline AppContainer 底部) + DEBUG-only `EmojiPanelHostView` (放 `Features/DevTools/Views/`).
- ✅ AC9 build verify: xcodebuild build pass (iPhone 17,OS=latest destination)；628 单测全绿无回归；ios-simulator MCP 实跑：install_app + launch (UITEST_MOCK_EMOJI=1 + --uitest-emoji-panel-host) + ui_view (4 cells 渲染 挥手/爱心/大笑/哭泣) + ui_describe_all (4 AXButton + emojiCell_<code> identifiers + AXLabel 中文名) + ui_tap (58, 450) → ui_describe_all 验证 `emojiPanel_uitestSelectedCode` AXLabel == "wave" (onSelect 回调正确传播).
- ✅ AC10 deliverable: 13 新建 + 2 修改 + 1 流程 sprint-status.yaml 全部落地.

### File List

**新建文件**（13 个 production + test）:
1. `iphone/PetApp/Features/Emoji/Models/EmojiConfig.swift`
2. `iphone/PetApp/Features/Emoji/Models/EmojiListResponse.swift`
3. `iphone/PetApp/Features/Emoji/UseCases/EmojisEndpoints.swift`
4. `iphone/PetApp/Features/Emoji/UseCases/LoadEmojisUseCase.swift`
5. `iphone/PetApp/Features/Emoji/Repositories/EmojiRepository.swift`
6. `iphone/PetApp/Features/Emoji/ViewModels/EmojiPanelViewModel.swift`
7. `iphone/PetApp/Features/Emoji/Views/EmojiPanelView.swift`
8. `iphone/PetApp/Features/DevTools/Views/EmojiPanelHostView.swift`（DEBUG-only stub host view）
9. `iphone/PetAppTests/Features/Emoji/Repositories/EmojiRepositoryTests.swift`
10. `iphone/PetAppTests/Features/Emoji/UseCases/LoadEmojisUseCaseTests.swift`
11. `iphone/PetAppTests/Features/Emoji/ViewModels/EmojiPanelViewModelTests.swift`
12. `iphone/PetAppUITests/Features/Emoji/EmojiPanelViewUITests.swift`
13. （注：`UITestMockEmojiRepository` inline 在 AppContainer.swift 底部 #if DEBUG 块，未走独立文件路径）

**修改文件**（3 个）:
1. `iphone/PetApp/App/AppContainer.swift`（追加 `loadEmojisUseCase` 字段 + `makeEmojiRepository` / `makeEmojiPanelViewModel` factory + DEBUG init `emojiRepository:` 注入参数 + DEBUG-only `UITestMockEmojiRepository` private final class + `parseUITestMockEmojiFixture` helper + convenience init UITEST_MOCK_EMOJI env 解析）
2. `iphone/PetApp/App/RootView.swift`（body 内 `#if DEBUG` 拦截 `--uitest-emoji-panel-host` launch arg → 渲染 EmojiPanelHostView；正常路径走 `mainBody`）
3. `iphone/PetApp/Shared/Constants/AccessibilityID.swift`（追加 `enum Emoji` nested：panel / panelLoading / panelError / cell(_:) helper / uitestSelectedCode）

**流程文件**（1 个）:
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（`ready-for-dev` → `in-progress` → `review`）

**xcodegen 自动重生**（1 个）:
- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen 把新文件 reference 写进项目；本 story 不手编 pbxproj，xcodegen generate 自动产出）

## Change Log

- 2026-05-14: Story 18.1 ready-for-dev → in-progress → review (Opus 4.7 dev-story workflow; 红绿循环完成 + ios-simulator MCP 实跑验证 onSelect 回调 + 4 cell 渲染 + emojiPanel_uitestSelectedCode 写入正确).
