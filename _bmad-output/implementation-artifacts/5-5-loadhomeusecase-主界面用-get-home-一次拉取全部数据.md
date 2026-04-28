# Story 5.5: LoadHomeUseCase + 主界面用 GET /home 一次拉取全部数据

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want App 启动**自动游客登录成功后**只调一次 `GET /api/v1/home`，把首屏所需的全部数据（user / pet / stepAccount / chest / room.currentRoomId）一次拉回；HomeViewModel 把整份 `HomeData` 投影到主界面 6 大占位区块（昵称 / 猫展示 / 步数 / 宝箱 / 三按钮 / 版本号）；任一字段缺失或 server 返 1009 → 由 ErrorPresenter 统一走 RetryView，**不**让主界面渲染半屏 placeholder；重试按钮触发 `LoadHomeUseCase` 重跑同一调用,
so that 节点 2 §4.2 验收硬指标"启动 → 主界面"路径上只发 **2** 个 HTTP 请求（`/auth/guest-login` + `/home`），相对旧串行 5 个 API（`/me` + `/steps/account` + `/chest/current` + `/cosmetics/inventory` + `/rooms/current`）方案首屏快约 800 ms（V1 §4.1 行 16 钦定 §5.1 schema 已**冻结**——契约不会再变，可放心绑定 DTO）；同时 iOS 端"读哪几个接口拼首屏"的胶水代码全部收敛在 `LoadHomeUseCase` 一处，节点 4 / 7 / 9 后续 epic 扩展（room.currentRoomId / chest 状态 / pet.equips）由 server `/home` 端 increment 升级，client 解析层**自动用上**而不必新增请求；在 Story 5.4 静默重登装饰器之上，业务层调一次 `/home` 拿到 401 也会被 `AuthRetryingAPIClient` 自动恢复，用户感知 0 中断。

## 故事定位（Epic 5 收尾 + 节点 2 验收闭环；上承 5.1 / 5.2 / 5.3 / 5.4，闭合 client 端 §4.2 验收）

这是 Epic 5 的**最后一条** story，把 5.1-5.4 已落地的 keychain + 自动游客登录 + token 注入 + 静默重登串成"启动 → 主界面 populate"的最终闭环。Epic 5 done 后 Epic 6（节点 2 §4.2 跨端 demo 验收）才能开。**直接前置**全部 done：

- **Story 4.8 (`done`)**：server `GET /api/v1/home` 已可用，wire 在 `/api/v1` 已认证子组（`router.go` 行 165）。响应 schema 已严格按 V1 §5.1 行 308-450 落地，且 V1 §4.1 行 16 钦定**自 2026-04-26 起 §5.1 schema 进入冻结状态**——本 story 直接把 wire schema 一一对应映射到 Swift `Codable` 结构，不必担心字段后续会被改名 / 改类型。本 story 以 server-side 实装 `homeResponseDTO`（`server/internal/app/http/handler/home_handler.go` 行 98-138）为契约**事实标准**：
  - `data.user`：`id` / `nickname` / `avatarUrl` 三字段，全 string，节点 2 全真实
  - `data.pet`：**可空对象** —— `out.Pet == nil` 时返 `"pet": null`（不是 `{}`）；非 nil 时含 `id` / `petType` / `name` / `currentState` / `equips: []`（节点 2 强制空数组）
  - `data.stepAccount`：`totalSteps` / `availableSteps` / `consumedSteps` 三 number，初次登录全 0
  - `data.chest`：`id` / `status`（动态判定 1=counting / 2=unlockable）/ `unlockAt`（RFC3339）/ `openCostSteps` / `remainingSeconds`（动态计算 ≥ 0 的 int）
  - `data.room`：`currentRoomId`（节点 2 强制 `null`；节点 4 起由 Story 11.10 在 server 端写入真实房间 ID）
- **Story 5.2 (`done`)**：`DefaultGuestLoginUseCase` 在 `iphone/PetApp/Features/Auth/UseCases/GuestLoginUseCase.swift` 已落地；其输出 `GuestLoginOutput(user, pet)` 由 `RootView.bootstrapStep1` closure 写入 `SessionStore`。**本 story 改造点**：在 `bootstrapStep1` 链尾继续接 `LoadHomeUseCase`（不是新建 `bootstrapStep2`，而是把 `LoadHomeUseCase` 的副作用合到 step 1 内的同一 closure 链 —— 详见 AC6 / Dev Note #2）；`AppLaunchStateMachine.bootstrapStep2` 注释说"Story 5.5 接 LoadHomeUseCase 时改这里"（`AppLaunchStateMachine.swift` 行 47 + 行 173），本 story **正式兑现**该插槽。
- **Story 5.3 (`done`)**：`APIClient` 自动从 `keychainStore` 读 token 并注入 `Authorization: Bearer <token>`；`endpoint.requiresAuth` 决定是否注入。本 story 新增的 `Endpoint(path: "/home", method: .get, requiresAuth: true)` 自动获得鉴权能力，**0** 改动 APIClient。
- **Story 5.4 (`done`)**：`AuthRetryingAPIClient` 装饰器已包在 `AppContainer.apiClient` 之上（`AppContainer.convenience init()` 行 75-95）—— 业务层（`makeAuthRepository()` / **未来** `makeHomeRepository()`）拿到的 `apiClient` 自动具备 401 → 静默重登 → 重试一次的能力。本 story 新建的 `DefaultHomeRepository` 走 `container.apiClient`（已 wrap 过），所以 `LoadHomeUseCase` 在 server 返 401 时**不需要**自己做任何重试 / 重登协调，装饰器一手 handle；本 story 只需保证：(a) `Endpoint.requiresAuth = true`（让装饰器认得这是受保护请求）；(b) 不在 UseCase 层 catch `.unauthorized` 做特殊处理（让装饰器路径生效）。
- **Story 4.1 (`done`)** 行 308-450 V1 §5.1 schema 已冻结：本 story 的 Swift DTO 字段名 / 类型 / 可空性必须**完全**对齐；任何字段变更需先改 V1 §5.1 + server `homeResponseDTO`，本 story 不能私自 rename / 加字段。

**本 story 的核心动作**（顺序无关，可分批落地）：

1. **新建** `iphone/PetApp/Features/Home/Models/HomeData.swift`（domain 层）+ `iphone/PetApp/Features/Home/Models/HomeResponse.swift`（DTO 层）：
   - `HomeResponse` (`Decodable`) 严格对齐 server `homeResponseDTO` 的字段名（`user` / `pet` / `stepAccount` / `chest` / `room`），子结构 `UserInfoDTO` / `HomePetDTO?` / `StepAccountDTO` / `ChestDTO` / `RoomDTO`
   - `HomeData` 是 domain 数据（业务可消费），从 `HomeResponse` 通过 `init(from response:)` 转换 —— 隔离 wire 层与业务层（让节点 4 / 7 / 9 后续扩展时只改 DTO + mapping，不污染 ViewModel）
   - **关键决策**：`HomePetDTO` / `HomePet` 不复用 `Auth/Models/GuestLoginResponse.swift` 的 `PetProfile`（详见"非范围"§5）；`UserInfoDTO` 不复用 `UserProfile`（同 §5 理由）。**新建独立类型**避免跨 Feature 耦合 + 让 `home` 字段集（`currentState` / `equips`）独立演进。
2. **新建** `iphone/PetApp/Features/Home/Repositories/HomeRepository.swift`：
   - 协议 `HomeRepositoryProtocol: Sendable`：`func loadHome() async throws -> HomeResponse`
   - 实装 `DefaultHomeRepository: HomeRepositoryProtocol`，依赖 `APIClientProtocol`，调 `Endpoint(path: "/home", method: .get, requiresAuth: true)`
   - 与 `DefaultAuthRepository` 同模式（struct + 单一方法 + 透传 `APIError`）
3. **新建** `iphone/PetApp/Features/Home/UseCases/HomeEndpoints.swift`：
   - `enum HomeEndpoints { static func loadHome() -> Endpoint { Endpoint(path: "/home", method: .get, body: nil, requiresAuth: true) } }`
   - 与 `Auth/UseCases/AuthEndpoints.swift` 同模式（避免 `HomeRepository` 内 inline `Endpoint(...)` 字面量散落）
4. **新建** `iphone/PetApp/Features/Home/UseCases/LoadHomeUseCase.swift`：
   - 协议 `LoadHomeUseCaseProtocol: Sendable`：`func execute() async throws -> HomeData`
   - 实装 `DefaultLoadHomeUseCase: LoadHomeUseCaseProtocol`，依赖 `HomeRepositoryProtocol`
   - 流程：调 `repo.loadHome()` → 拿 `HomeResponse` → `HomeData(from:)` 转 → 返回；任何错误（含 `.unauthorized` —— 装饰器已 retry 过仍失败 / 装饰器范围外的 .missingCredentials / .business / .network / .decoding）一律**透传** —— UseCase 不做错误码翻译，**不**直接接 ErrorPresenter（错误 UI 调度归调用方 ViewModel）
5. **修改** `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`：
   - **追加**新字段 `@Published var homeData: HomeData?`（初值 nil，启动 / 重试期间为 nil）
   - **追加**新字段 `@Published var loadingState: HomeLoadingState`（枚举 `.idle / .loading / .loaded / .failed(message:)`），用于 UI 区分"正在加载 vs 已加载 vs 失败"
   - **追加**新 init `init(loadHomeUseCase: LoadHomeUseCaseProtocol, errorPresenter: ErrorPresenter, ...)` 注入 UseCase + ErrorPresenter（让 ViewModel 自己调 ErrorPresenter.present(error:onRetry:) 呈现错误 + 重试按钮）
   - **追加**新方法 `func loadHome() async`：内部调 `loadHomeUseCase.execute()`；成功 → 把整份 HomeData 投影到 `homeData` + `loadingState = .loaded`；失败 → `loadingState = .failed(message:)` + 通过 `errorPresenter.present(error: error, onRetry: { Task { await self.loadHome() } })` 触发 RetryView
   - **追加**新方法 `func bind(loadHomeUseCase:errorPresenter:)`：与既有 `bind(pingUseCase:)` 同模式（与 RootView 的 lazy 注入路径配合）
   - **保留**所有既有字段 / 方法（`nickname` / `appVersion` / `serverInfo` / Ping `start()` / 三按钮 closure）—— Preview / 老测试 / UITest skip-guest-login 路径**零回归**
6. **修改** `iphone/PetApp/Features/Home/Views/HomeView.swift`：
   - `petAndChestRow` / `stepBalanceLabel` / `chestArea` 三块从 hardcode 文案升级为读 `viewModel.homeData?.<field>` —— `homeData` 为 nil 时仍显示 placeholder（保 Preview / UITest skip-guest-login 路径）
   - **关键**：本 story **不**渲染 `HomeData.pet` 的真实图片 / 动画（节点 5 / Epic 8 才接 sprite；本 story `petArea` 仍用灰色矩形 placeholder，但下方加一个新 `Text(viewModel.homeData?.pet?.name ?? "默认小猫")` 让"主界面 populate 完整"可肉眼验证）
   - `chestArea` 上方追加 `Text(viewModel.homeData?.chestRemainingDisplay ?? "--:--")`（minute:second 格式）让宝箱倒计时可见
   - `stepBalanceLabel` 文案改为 `"\(viewModel.homeData?.stepAccount.availableSteps ?? 0) 步"`
   - 复用 `userInfoBar` 的 `SessionStore` 订阅模式（**不**改 nickname 来源 —— Story 5.2 lesson 2026-04-27-sessionstore-home-nickname-source-of-truth.md 钦定 nickname 仍走 SessionStore；`HomeData.user.nickname` 与 `SessionStore.session.user.nickname` 是**冗余信息**，节点 2 阶段两者必定同值；**避免**把 nickname 渲染源切到 HomeData 引发 5.2 lesson 的回归）
   - `versionLabel` / 三按钮 / `chestArea` 既有 a11y identifier 全部保留 + 新增 `home_petName` / `home_chestRemaining` 两个 a11y identifier 让 UITest 可独立断言
7. **修改** `iphone/PetApp/App/AppContainer.swift`：
   - 新增 `func makeHomeRepository() -> HomeRepositoryProtocol`：`DefaultHomeRepository(apiClient: apiClient)`
   - 新增 `func makeLoadHomeUseCase() -> LoadHomeUseCaseProtocol`：`DefaultLoadHomeUseCase(repository: makeHomeRepository())`
   - **不**新增 `homeViewModel` 持有字段（HomeViewModel 仍由 RootView 持有 `@StateObject`，与 5.2 既有架构对齐）
8. **修改** `iphone/PetApp/App/RootView.swift`：
   - **关键改造**：`ensureLaunchStateMachineWired()` 内的 `bootstrapStep1` closure 在 `useCase.execute()` 成功后**串行**调 `loadHomeUseCase.execute()` 拿 `HomeData`，并通过 `homeViewModel.applyHomeData(_:)` 注入（详见 AC6 设计动机）；任一步抛错都让 `bootstrapStep1` 抛错 → 状态机走 `.needsAuth(message:)` → 显示 RetryView（与 5.2 已落地的失败路径同语义）
   - **不**新增 `bootstrapStep2` closure（让 LoadHome 与 GuestLogin 形成"同一启动事务"语义；详见 Dev Note #2）
   - `.task` 内 / `.onAppear` 内 wire `homeViewModel.bind(loadHomeUseCase:errorPresenter:)` —— 与既有 `bind(pingUseCase:)` 同模式；让"用户进入 .ready 后**不**自动重新调 /home"（启动期一次性拉取，避免 .task 重启重发请求）
   - UITEST hook（`UITEST_SKIP_GUEST_LOGIN == "1"`）路径继续走 no-op `AppLaunchStateMachine()`（不调 LoadHomeUseCase）—— 保 HomeUITests / NavigationUITests 不依赖真实 server
9. **新建** `iphone/PetAppTests/Features/Home/UseCases/LoadHomeUseCaseTests.swift`：覆盖 ≥ 5 case（详 AC9）
10. **新建** `iphone/PetAppTests/Features/Home/Repositories/HomeRepositoryTests.swift`：覆盖 ≥ 3 case（详 AC10）
11. **新建** `iphone/PetAppTests/Features/Home/UseCases/MockHomeRepository.swift`（继承 `MockBase`，含 `loadHomeStub: Result<HomeResponse, Error>`）
12. **新建** `iphone/PetAppTests/Features/Home/UseCases/MockLoadHomeUseCase.swift`（继承 `MockBase`，含 `executeStub: Result<HomeData, Error>`）—— 让 `HomeViewModel` 测试不必构造 repo
13. **新建** `iphone/PetAppTests/Features/Home/Models/HomeResponseDecodingTests.swift`：覆盖 ≥ 4 case 锁定 wire 解码契约（详 AC11）
14. **新建** `iphone/PetAppTests/Features/Home/ViewModels/HomeViewModelLoadHomeTests.swift`：覆盖 ≥ 4 case（详 AC12）
15. **不**改 Story 4.8 server 端任一文件（纯 client 端实装；`server/` 全程零改动）
16. **不**改 `APIClient.swift` / `APIError.swift` / `Endpoint.swift` / `AuthRetryingAPIClient.swift` —— 仅消费既有定义（新增 endpoint 走 `Endpoint.init(path: ..., requiresAuth: true)` 自动接装饰器）
17. **不**改 `Auth/` 目录任一文件（GuestLoginUseCase / AuthRepository / SessionStore / SessionState 等不动）
18. **不**改 `AppLaunchStateMachine` 主体逻辑（新 closure 直接走既有 `bootstrapStep1` 注入插槽）
19. **不**新增 `RefreshHomeUseCase`（iOS 架构 §6.2 行 340 列示但本 story 范围外 —— 节点 4+ 用户主动下拉刷新场景才需要；本 story 的"重试"通过 ErrorPresenter 的 onRetry → `loadHome()` 实现）

**测试基础设施**：

- **不**新增第三方 mock 库（与 ADR-0002 §3.1 一致）
- 复用 Story 2.5 落地的 `MockAPIClient`（`PetAppTests/Features/Home/UseCases/MockAPIClient.swift`，含 `stubResponse: [String: Stub]` + `invocations`）—— `HomeRepository` 单测直接复用
- 复用 Story 2.7 落地的 `MockBase`（`PetAppTests/Helpers/MockBase.swift`）—— `MockHomeRepository` / `MockLoadHomeUseCase` 都继承
- 复用 Story 2.6 落地的 `ErrorPresenter` —— `HomeViewModel` 失败路径 present(error:onRetry:) 走它；测试场景注入 `ErrorPresenter(toastDuration: 0.05)` 加速
- 复用 Story 2.5 落地的 `MockBundle` 模式（如适用）
- **新建** `MockHomeRepository`（继承 `MockBase` + `loadHomeStub: Result<HomeResponse, Error>`）—— 让 `LoadHomeUseCase` 测试不必构造 APIClient
- **新建** `MockLoadHomeUseCase`（继承 `MockBase` + `executeStub: Result<HomeData, Error>`）—— 让 `HomeViewModel` 测试不必构造 repo

**不涉及**：

1. **server 端任何改动**：纯客户端实装；`server/` 全程零改动；server `GET /home` 已 done（Story 4.8）
2. **节点 4 / 7 / 9 真实数据填充**：
   - `room.currentRoomId`：节点 2 阶段 server 强制 `null`，本 story 客户端解析 `String?` —— 节点 4 由 Story 11.10 在 server 端注入真实数据，client 解析层**自动**用上（不必改 client）
   - `chest.status` 真实开箱状态：本 story 解析 server 返回的 status 枚举（1=counting / 2=unlockable）；节点 7 / Epic 20 才有"用户开过箱后立即重建下一轮 chest"逻辑；本 story client 不预设 opened 状态（V1 §5.1 行 345 钦定 `/home` 永远不返 opened）
   - `pet.equips` 真实穿戴：本 story 客户端解析 `[]` 空数组 —— 节点 9 / Story 26.6 server 端填充真实数据，client 解析层**自动**用上（不必改 client）
   - `pet.equips[].renderConfig`：节点 10 / Story 29.6 才落地，本 story 不预解析（节点 10 时 DTO 内追加 `renderConfig: RenderConfig?` 子字段）
3. **真实猫 sprite / 动画**：节点 5 / Epic 8 才接（CoreMotion 状态映射 → 三态动画）；本 story `petArea` 仍灰色矩形 + 文字标签
4. **真实步数 HealthKit 接入**：节点 3 / Epic 8 才接；本 story `stepBalanceLabel` 仅显示 server 返回的 availableSteps（首次登录初始化为 0）
5. **真实宝箱倒计时**：本 story `chestArea` 静态显示 server 返回的 `remainingSeconds`，**不**起本地 timer 动态倒计时（节点 7 / Story 21.2 才接）
6. **下拉刷新 / pull-to-refresh**：iOS 架构 §6.2 列示 `RefreshHomeUseCase`，本 story 不做（节点 4+ 用户主动下拉刷新才需要）；用户主动重试通过 ErrorPresenter onRetry 走
7. **WebSocket 实时数据**：归 Epic 10+；本 story 是 REST 一次性拉取
8. **HomePetDTO 与 GuestLoginResponse.PetProfile 类型合并**：两者节点 2 阶段字段几乎重叠，但**有意分开**：(a) `HomePet` 必含 `currentState` 字段（GuestLoginResponse.PetProfile 没有）；(b) `HomePet` 节点 9 后会增 `equips: [HomeEquip]` 字段，与 GuestLogin 路径无关；(c) 跨 Feature 耦合会让节点 9+ 改一处必影响多处。**新建独立类型**让 Auth / Home 两个 Feature 各自演进。
9. **节点 2 阶段 nickname 渲染源切换**：本 story **不**把 HomeView nickname 渲染源从 SessionStore 切到 HomeData（详见 Story 2.5 lesson 2026-04-27-sessionstore-home-nickname-source-of-truth.md 与"§6 修改"段说明）—— 两源在节点 2 阶段必同值；切换源会引发 5.2 lesson 涉及的"双源 / 写入未订阅" 类回归。HomeData.user 字段仍解析 + 暴露（让节点 4 后某些场景可读 HomeData 拿 nickname），但**当前**渲染走 SessionStore
10. **APIClient 主体 / 装饰器逻辑**：本 story 仅消费既有装饰器（401 自动重登），**不**修改装饰器范围
11. **iOS UITest 真实 server 联调**：本 story 是 UseCase / Repository / ViewModel 单测可完整覆盖 + RootView wire 改动；UITest 路径走 `UITEST_SKIP_GUEST_LOGIN` 旁路，**不**新建 UITest（节点 2 §4.2 的"启动 → 主界面 populate"端到端验证归 Story 6.1 / Epic 6）
12. **AppLaunchStateMachine bootstrapStep2 插槽**：本 story 把 LoadHomeUseCase 调用合到 step 1 closure 链尾（详 AC6 + Dev Note #2），bootstrapStep2 插槽**仍保留**默认 `{ }` no-op，**不**用它（避免引入"step 1 成功但 step 2 失败"的微妙状态分裂；让"启动 → 拉首屏"是单次原子操作的失败语义）
13. **`ios/` 旧产物目录**：CLAUDE.md "Repo Separation" + ADR-0002 §3.3 既有约束。最终 `git status` 须确认 `ios/` 下零改动

**范围红线**：

- **不动 `ios/`**：CLAUDE.md "Repo Separation" + ADR-0002 §3.3
- **不动 `server/`**：纯 iPhone 端实装
- **不动 `iphone/scripts/` / `iphone/project.yml` / `iphone/.gitignore`**：新文件靠既有 `sources: - PetApp` / `sources: - PetAppTests` glob 自动纳入；**0 yml 改动**
- **不引入第三方依赖**：`Foundation` / `Combine` 即可；async-await 是 Swift 标准
- **不动 `APIClientProtocol` 协议签名 / `APIClient` 主体**：本 story 通过新建 `Endpoint(path: "/home", requiresAuth: true)` 自动接 5.3 token 注入 + 5.4 静默重登装饰器
- **不动 `APIError` / `Endpoint` 类型定义**：仅消费既有定义
- **不动 `GuestLoginUseCase` / `AuthRepository` / `SilentReloginUseCase` / `AuthRetryingAPIClient`**：本 story 与 5.4 装饰器**透明协作**（业务请求 401 → 装饰器自动救）
- **不动 `SessionStore` / `SessionState`**：HomeData 是独立的"首屏数据"概念，**不**与 SessionStore 合并；SessionStore 仍只持 user + pet（profile 级别）
- **不动 `AppLaunchStateMachine` 内部逻辑**：仅消费 `bootstrapStep1` closure 注入插槽
- **不动 既有 ErrorPresenter** 行为：`HomeViewModel` 调 `errorPresenter.present(error:onRetry:)` 复用 5.2 落地的 onRetry 闭包入队机制
- **不动 既有 HomeViewModel 字段 / 方法**：所有改动都是**追加**（新字段 / 新 init / 新方法）；老 init / 老字段 / 老方法签名零改动 —— Preview / 老测试 / UITest skip-guest-login 路径零回归
- **不动 既有 HomeView nickname 渲染源**：仍走 SessionStore（5.2 lesson 已固化）
- **不在 `APIError` 加新 case**：HomeData wire 解析失败走 `.decoding(underlying:)`；server 1009 走 `.business(code: 1009, ...)`；这两条都已是既有 case
- **每次启动只发一次 /home**：通过 `LoadHomeUseCase` 调用点收敛在 `bootstrapStep1` closure 内 + ViewModel 内的 `hasLoaded` flag 防 .task 重启重发（与 Story 2.5 `HomeViewModel.start()` 的 hasFetched 同模式）
- **失败路径**：UseCase 不直接接 ErrorPresenter；ViewModel 才接 —— 错误调度归调用方决定（让 UseCase 可被未来的非 UI 场景如后台刷新复用）

## Acceptance Criteria

**AC1 — `HomeResponse` DTO + 子结构：严格对齐 server `homeResponseDTO` wire 字段**

新建 `iphone/PetApp/Features/Home/Models/HomeResponse.swift`：

```swift
// HomeResponse.swift
// Story 5.5 AC1: GET /api/v1/home 的 wire DTO；严格对齐 V1 §5.1 + server homeResponseDTO（行 308-450）.
//
// 注：APIClient 已剥 envelope（code/message/data/requestId）；本类仅模型 envelope.data 字段内容.
//
// 字段可空性（V1 §4.1 行 16 钦定 §5.1 schema 已冻结，本 story 直接 wire）：
//   - data.pet: object | null —— 用户无默认 pet 时为 null（理论不应发生但 server Story 4.8 edge 强制覆盖）
//   - data.room.currentRoomId: string | null —— 节点 2 阶段强制 null；节点 4 后 Story 11.10 注入真实
//   - 其它字段全非空（pet 的子字段在 pet ≠ nil 时全非空）
//
// 与 GuestLoginResponse.UserProfile / PetProfile 的关系（Story 5.2 落地）：
//   - 不复用 —— UserProfile.hasBoundWechat 是 GuestLogin 特有；HomePet 必含 currentState（GuestLogin 没有）
//   - 节点 9 后 HomePet.equips 演化路径独立于 Auth 模块
//   - 详见本 story Dev Note #1 的"双 DTO 体系" 设计动机
//
// 节点 9+ 字段（pet.equips[].renderConfig）本 story 不预解析；节点 10 / Story 29.6 时追加 RenderConfig 子结构.

import Foundation

public struct HomeResponse: Decodable, Equatable {
    public let user: UserInfoDTO
    public let pet: HomePetDTO?           // 可空：V1 §5.1 行 335 钦定
    public let stepAccount: StepAccountDTO
    public let chest: ChestDTO
    public let room: RoomDTO

    public init(
        user: UserInfoDTO,
        pet: HomePetDTO?,
        stepAccount: StepAccountDTO,
        chest: ChestDTO,
        room: RoomDTO
    ) {
        self.user = user
        self.pet = pet
        self.stepAccount = stepAccount
        self.chest = chest
        self.room = room
    }
}

public struct UserInfoDTO: Decodable, Equatable, Sendable {
    public let id: String                 // BIGINT 序列化为 string（V1 §2.5）
    public let nickname: String
    public let avatarUrl: String          // 节点 2 阶段固定 ""（**不**为 null —— V1 §5.1 行 334 钦定）

    public init(id: String, nickname: String, avatarUrl: String) {
        self.id = id
        self.nickname = nickname
        self.avatarUrl = avatarUrl
    }
}

public struct HomePetDTO: Decodable, Equatable, Sendable {
    public let id: String
    public let petType: Int               // 节点 2 固定 1（猫）
    public let name: String
    public let currentState: Int          // 1=rest, 2=walk, 3=run（V1 §5.1 + 数据库设计 §6.4）
    public let equips: [EquipDTO]         // 节点 2 阶段强制 []；节点 9 由 Story 26.6 填充

    public init(id: String, petType: Int, name: String, currentState: Int, equips: [EquipDTO]) {
        self.id = id
        self.petType = petType
        self.name = name
        self.currentState = currentState
        self.equips = equips
    }
}

/// 装扮元素 DTO；节点 2 阶段 server 强制返回 `equips: []`，本类型仅做契约预留，实测不会被构造.
/// 节点 9 / Story 26.6 server 端填充真实数据时，client 自动解码（**0** 改动）.
public struct EquipDTO: Decodable, Equatable, Sendable {
    public let slot: Int
    public let userCosmeticItemId: String
    public let cosmeticItemId: String
    public let name: String
    public let rarity: Int
    public let assetUrl: String

    public init(
        slot: Int,
        userCosmeticItemId: String,
        cosmeticItemId: String,
        name: String,
        rarity: Int,
        assetUrl: String
    ) {
        self.slot = slot
        self.userCosmeticItemId = userCosmeticItemId
        self.cosmeticItemId = cosmeticItemId
        self.name = name
        self.rarity = rarity
        self.assetUrl = assetUrl
    }
}

public struct StepAccountDTO: Decodable, Equatable, Sendable {
    public let totalSteps: Int
    public let availableSteps: Int
    public let consumedSteps: Int

    public init(totalSteps: Int, availableSteps: Int, consumedSteps: Int) {
        self.totalSteps = totalSteps
        self.availableSteps = availableSteps
        self.consumedSteps = consumedSteps
    }
}

public struct ChestDTO: Decodable, Equatable, Sendable {
    public let id: String
    public let status: Int                // 1=counting, 2=unlockable（V1 §5.1 行 345）
    public let unlockAt: Date             // ISO 8601 RFC3339
    public let openCostSteps: Int
    public let remainingSeconds: Int      // ≥ 0；server 已算好

    public init(id: String, status: Int, unlockAt: Date, openCostSteps: Int, remainingSeconds: Int) {
        self.id = id
        self.status = status
        self.unlockAt = unlockAt
        self.openCostSteps = openCostSteps
        self.remainingSeconds = remainingSeconds
    }
}

public struct RoomDTO: Decodable, Equatable, Sendable {
    /// 节点 2 阶段强制 nil；节点 4 起 Story 11.10 注入真实房间 ID.
    public let currentRoomId: String?

    public init(currentRoomId: String?) {
        self.currentRoomId = currentRoomId
    }
}
```

**具体行为要求**：

- 严格对齐 V1 §5.1 + server `homeResponseDTO`（`server/internal/app/http/handler/home_handler.go` 行 98-138）
- `pet: HomePetDTO?` —— **必须 Optional**；server 返 `"pet": null` 时正确解析为 nil（**不**抛 decoding error）
- `room.currentRoomId: String?` —— **必须 Optional**；server 节点 2 阶段强制返 `null`
- `unlockAt: Date` —— 用 `Date` 类型（不是 String）；解码时让 APIClient 注入 `JSONDecoder.dateDecodingStrategy = .iso8601`（详 AC2）
- 全部子结构标 `Sendable`：让 HomeData 可跨 actor 边界传（与 GuestLoginResponse.UserProfile 同模式）
- 全部子结构标 `Equatable`：让单测断言简单（`XCTAssertEqual(actual.user, expected)`）
- 不复用 `Auth/Models/GuestLoginResponse.UserProfile` / `PetProfile`：**新建独立 `UserInfoDTO` / `HomePetDTO`**（详见 Dev Note #1 + 本 story 非范围 §8）

**AC2 — `HomeData` domain 数据 + `init(from response:)` 转换**

新建 `iphone/PetApp/Features/Home/Models/HomeData.swift`：

```swift
// HomeData.swift
// Story 5.5 AC2: 首屏 domain 数据；从 HomeResponse wire DTO 转换得到.
//
// 设计：HomeData 是 ViewModel / View 直接消费的"业务数据"层；HomeResponse 是 wire DTO 层.
// 隔离意义：节点 4 / 7 / 9 后续扩展时只改 DTO + 转换 mapping，不污染 ViewModel.
//
// 节点 2 阶段 HomeData 与 HomeResponse 字段几乎 1:1（除 chest.remainingDisplay 等 derived 字段），
// 但保留独立类型让未来 derived 字段（如本地 timer 动态计算的 chest 倒计时）有单一去处.

import Foundation

public struct HomeData: Equatable, Sendable {
    public let user: HomeUser
    public let pet: HomePet?
    public let stepAccount: HomeStepAccount
    public let chest: HomeChest
    public let room: HomeRoom

    public init(
        user: HomeUser,
        pet: HomePet?,
        stepAccount: HomeStepAccount,
        chest: HomeChest,
        room: HomeRoom
    ) {
        self.user = user
        self.pet = pet
        self.stepAccount = stepAccount
        self.chest = chest
        self.room = room
    }

    /// 从 wire DTO 构造 domain 数据；当前节点 2 阶段是直白复制；
    /// 未来节点扩展加 derived 字段时，转换逻辑集中在此 init 内.
    public init(from response: HomeResponse) {
        self.user = HomeUser(id: response.user.id, nickname: response.user.nickname, avatarUrl: response.user.avatarUrl)
        if let pet = response.pet {
            self.pet = HomePet(
                id: pet.id,
                petType: pet.petType,
                name: pet.name,
                currentState: HomePetState(rawValue: pet.currentState) ?? .rest,
                equips: pet.equips.map { HomeEquip(from: $0) }
            )
        } else {
            self.pet = nil
        }
        self.stepAccount = HomeStepAccount(
            totalSteps: response.stepAccount.totalSteps,
            availableSteps: response.stepAccount.availableSteps,
            consumedSteps: response.stepAccount.consumedSteps
        )
        self.chest = HomeChest(
            id: response.chest.id,
            status: HomeChestStatus(rawValue: response.chest.status) ?? .counting,
            unlockAt: response.chest.unlockAt,
            openCostSteps: response.chest.openCostSteps,
            remainingSeconds: response.chest.remainingSeconds
        )
        self.room = HomeRoom(currentRoomId: response.room.currentRoomId)
    }

    /// 宝箱倒计时显示（mm:ss 格式）；剩余 0 秒返 "00:00".
    public var chestRemainingDisplay: String {
        chest.remainingDisplay
    }
}

public struct HomeUser: Equatable, Sendable {
    public let id: String
    public let nickname: String
    public let avatarUrl: String

    public init(id: String, nickname: String, avatarUrl: String) {
        self.id = id
        self.nickname = nickname
        self.avatarUrl = avatarUrl
    }
}

public struct HomePet: Equatable, Sendable {
    public let id: String
    public let petType: Int
    public let name: String
    public let currentState: HomePetState
    public let equips: [HomeEquip]

    public init(id: String, petType: Int, name: String, currentState: HomePetState, equips: [HomeEquip]) {
        self.id = id
        self.petType = petType
        self.name = name
        self.currentState = currentState
        self.equips = equips
    }
}

public enum HomePetState: Int, Equatable, Sendable {
    case rest = 1
    case walk = 2
    case run = 3
}

public struct HomeEquip: Equatable, Sendable {
    public let slot: Int
    public let userCosmeticItemId: String
    public let cosmeticItemId: String
    public let name: String
    public let rarity: Int
    public let assetUrl: String

    public init(from dto: EquipDTO) {
        self.slot = dto.slot
        self.userCosmeticItemId = dto.userCosmeticItemId
        self.cosmeticItemId = dto.cosmeticItemId
        self.name = dto.name
        self.rarity = dto.rarity
        self.assetUrl = dto.assetUrl
    }
}

public struct HomeStepAccount: Equatable, Sendable {
    public let totalSteps: Int
    public let availableSteps: Int
    public let consumedSteps: Int

    public init(totalSteps: Int, availableSteps: Int, consumedSteps: Int) {
        self.totalSteps = totalSteps
        self.availableSteps = availableSteps
        self.consumedSteps = consumedSteps
    }
}

public struct HomeChest: Equatable, Sendable {
    public let id: String
    public let status: HomeChestStatus
    public let unlockAt: Date
    public let openCostSteps: Int
    public let remainingSeconds: Int

    public init(id: String, status: HomeChestStatus, unlockAt: Date, openCostSteps: Int, remainingSeconds: Int) {
        self.id = id
        self.status = status
        self.unlockAt = unlockAt
        self.openCostSteps = openCostSteps
        self.remainingSeconds = remainingSeconds
    }

    /// mm:ss 格式倒计时显示；负值钳到 0；秒数 ≥ 60 时分母进位.
    public var remainingDisplay: String {
        let safe = max(0, remainingSeconds)
        let minutes = safe / 60
        let seconds = safe % 60
        return String(format: "%02d:%02d", minutes, seconds)
    }
}

public enum HomeChestStatus: Int, Equatable, Sendable {
    case counting = 1
    case unlockable = 2
}

public struct HomeRoom: Equatable, Sendable {
    public let currentRoomId: String?

    public init(currentRoomId: String?) {
        self.currentRoomId = currentRoomId
    }
}
```

**具体行为要求**：

- `HomeData` / 子结构全 `Sendable`：跨 actor 传递无警告
- `HomeData(from response:)` 同步转换、纯函数：测试可稳定断言
- `HomePetState` / `HomeChestStatus` 用 `Int` raw value enum：让未识别的 server 状态值（理论不应出现，但防御性）走 fallback（`?? .rest` / `?? .counting`），**不**抛 decoding error 拖垮整个首屏
- `HomeChest.remainingDisplay` 是纯函数 derived 字段：单测可独立覆盖（"00:00" / "10:00" / "00:30"）
- 不持 `Date()` 等"现在时间"依赖：domain 层数据稳定可比；UI 层若需"距 unlockAt 还剩多久"动态显示，由 Story 21.2 引入本地 timer

**AC3 — `HomeRepository` + `HomeEndpoints`：调 GET /api/v1/home**

新建 `iphone/PetApp/Features/Home/Repositories/HomeRepository.swift`：

```swift
// HomeRepository.swift
// Story 5.5 AC3: HomeRepository 封装 GET /api/v1/home 调用；让 UseCase 不直接接触 APIClient.
//
// 与 AuthRepository（Story 5.2）同模式：协议方法返回原始 wire DTO；APIError 原样透传.
//
// 注入的 APIClient 是 container.apiClient —— 已被 Story 5.4 AuthRetryingAPIClient 包装；
// 业务请求 401 会自动触发静默重登 + 重试一次，HomeRepository 完全无感.
//
// `DefaultHomeRepository` 是 `struct`：value type，无内部状态，构造廉价；与 DefaultAuthRepository 同模式.

import Foundation

public protocol HomeRepositoryProtocol: Sendable {
    /// 调 GET /api/v1/home 拿首屏数据.
    /// - Returns: HomeResponse（含 user / pet / stepAccount / chest / room）
    /// - Throws: APIError.business(1001 / 1009) / APIError.network / APIError.unauthorized
    ///           / APIError.missingCredentials / APIError.decoding
    func loadHome() async throws -> HomeResponse
}

public struct DefaultHomeRepository: HomeRepositoryProtocol {
    private let apiClient: APIClientProtocol

    public init(apiClient: APIClientProtocol) {
        self.apiClient = apiClient
    }

    public func loadHome() async throws -> HomeResponse {
        try await apiClient.request(HomeEndpoints.loadHome())
    }
}
```

新建 `iphone/PetApp/Features/Home/UseCases/HomeEndpoints.swift`：

```swift
// HomeEndpoints.swift
// Story 5.5 AC3: GET /api/v1/home endpoint 工厂；与 AuthEndpoints 同模式.
//
// 提为独立 enum：避免 HomeRepository 内 inline `Endpoint(...)` 字面量散落；
// 当 V1 §5.1 path / requiresAuth 改时（理论上不会，已冻结），仅改本文件一处.

import Foundation

public enum HomeEndpoints {
    /// GET /api/v1/home —— 首屏聚合数据（user + pet + stepAccount + chest + room）.
    /// requiresAuth=true：自动经过 APIClient 的 token 注入（Story 5.3）+ AuthRetryingAPIClient
    /// 装饰器（Story 5.4 → 401 自动静默重登 + 重试一次）.
    public static func loadHome() -> Endpoint {
        Endpoint(path: "/home", method: .get, body: nil, requiresAuth: true)
    }
}
```

**具体行为要求**：

- 协议 `HomeRepositoryProtocol: Sendable`：让 UseCase 在 actor / Task 上下文调用安全
- `loadHome()` 返回原始 `HomeResponse`：UseCase 层负责转 `HomeData`（保持 repo 层职责"接 wire" + UseCase 层职责"转 domain"分离）
- 错误透传**严格**：APIError 全部原样抛，repo 不做错误码映射
- `Endpoint.requiresAuth = true`：让 Story 5.3 token 注入决策树识别为受保护接口；让 Story 5.4 AuthRetryingAPIClient 装饰器识别为可触发静默重登的请求

**AC4 — `LoadHomeUseCase` 协议 + 实装：调 repo + 转 HomeData + 透传错误**

新建 `iphone/PetApp/Features/Home/UseCases/LoadHomeUseCase.swift`：

```swift
// LoadHomeUseCase.swift
// Story 5.5 AC4: 首屏数据加载 UseCase（Epic 5 收尾 + iOS 架构 §6.2 钦定的 LoadHomeUseCase 落地点）.
//
// 流程：
//   1. 调 repo.loadHome() 拿 HomeResponse
//   2. HomeData(from: response) 转 domain 数据
//   3. 返回 HomeData（含 user / pet / stepAccount / chest / room）
//
// 错误处理：所有错误**原样**透传 throw；不在 UseCase 内吞错或转码.
//   - APIError.business(1001) → 装饰器层已 catch（Story 5.4 wrap）；理论不应抵达 UseCase
//   - APIError.business(1009) → server 任一聚合查询失败 → 透传 → ViewModel 走 ErrorPresenter 显示 RetryView
//   - APIError.unauthorized → 装饰器已重试过仍 401 → 透传 → ViewModel 同上
//   - APIError.missingCredentials → cold-start 路径未走通（keychain 空 / token 未写）→ 透传
//   - APIError.network → 透传 → ViewModel 同上
//   - APIError.decoding → server 返了不符合 schema 的数据（理论不应发生 —— V1 §4.1 行 16 schema 已冻结）
//                         → 透传 → ViewModel 同上（让用户重试或提示"App 需要更新"）
//
// 不在本 story 范围（设计选择）：
//   - 不直接接 ErrorPresenter（让 UseCase 可被未来非 UI 场景 / 后台刷新复用）
//   - 不做 retry / 指数退避（401 已被装饰器一次性 retry；其它错误由用户走 RetryView 主动重试）
//   - 不缓存上次结果（节点 2 阶段每次启动都拉新；节点 4 后引入缓存归未来 RefreshHomeUseCase）

import Foundation

public protocol LoadHomeUseCaseProtocol: Sendable {
    /// 调 GET /api/v1/home 拿首屏数据并转 domain.
    /// - Returns: HomeData（含 user / pet / stepAccount / chest / room）
    /// - Throws: APIError（全部 case 原样透传）
    func execute() async throws -> HomeData
}

public struct DefaultLoadHomeUseCase: LoadHomeUseCaseProtocol {
    private let repository: HomeRepositoryProtocol

    public init(repository: HomeRepositoryProtocol) {
        self.repository = repository
    }

    public func execute() async throws -> HomeData {
        let response = try await repository.loadHome()
        return HomeData(from: response)
    }
}
```

**具体行为要求**：

- 协议 `LoadHomeUseCaseProtocol: Sendable`：让 ViewModel / actor 持有跨 task 调用安全
- 实装 `DefaultLoadHomeUseCase: struct`：value type，构造廉价（与 `DefaultGuestLoginUseCase` / `DefaultPingUseCase` 同模式）
- 依赖注入：`HomeRepositoryProtocol` —— 测试场景注入 `MockHomeRepository`
- **不**注入 `errorPresenter` / `sessionStore` —— UseCase 单一职责：转 wire → domain；错误调度归 ViewModel；状态写入归 ViewModel
- 错误透传**严格**：不做错误码翻译 / 转换 —— ViewModel / ErrorPresenter / business 层负责自己关心的错误

**AC5 — `HomeViewModel` 追加 LoadHome 路径（不破坏既有接口）**

修改 `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`，**追加**（不删除既有内容）：

```swift
// 新增 @Published 字段
@Published public var homeData: HomeData?
@Published public var loadingState: HomeLoadingState = .idle

// 注入 UseCase + ErrorPresenter（init 路径 / bind 路径并存，与 PingUseCase 同模式）
private let loadHomeUseCase: LoadHomeUseCaseProtocol?
private var boundLoadHomeUseCase: LoadHomeUseCaseProtocol?
private let errorPresenter: ErrorPresenter?
private weak var boundErrorPresenter: ErrorPresenter?

// 跨边界短路 flag（与 hasFetched 同模式）
private var hasLoadedHome: Bool = false
private var loadHomeTask: Task<Void, Never>?

// 新 init：注入 LoadHomeUseCase + ErrorPresenter（与既有 init(pingUseCase:) 同模式）
public init(
    nickname: String = "用户1001",
    pingUseCase: PingUseCaseProtocol,
    loadHomeUseCase: LoadHomeUseCaseProtocol,
    errorPresenter: ErrorPresenter,
    appVersion: String = HomeViewModel.readAppVersion(),
    serverInfo: String = "----",
    onRoomTap: @escaping () -> Void = {},
    onInventoryTap: @escaping () -> Void = {},
    onComposeTap: @escaping () -> Void = {}
) { ... }

// 新 bind 方法（与 bind(pingUseCase:) 同模式）
public func bind(loadHomeUseCase: LoadHomeUseCaseProtocol, errorPresenter: ErrorPresenter) {
    guard self.boundLoadHomeUseCase == nil else { return }
    self.boundLoadHomeUseCase = loadHomeUseCase
    self.boundErrorPresenter = errorPresenter
}

// 新方法：LoadHome 入口（启动时由 RootView bootstrapStep1 注入路径调；用户重试时由 ErrorPresenter onRetry 闭包调）
public func loadHome() async {
    let useCase = loadHomeUseCase ?? boundLoadHomeUseCase
    guard let useCase = useCase else { return }
    guard !hasLoadedHome else { return }
    guard loadHomeTask == nil else { return }

    loadingState = .loading
    let task = Task { [weak self] in
        do {
            let data = try await useCase.execute()
            await self?.applyHomeData(data)
        } catch {
            await self?.applyHomeError(error)
        }
    }
    loadHomeTask = task
    await task.value
    loadHomeTask = nil
}

// 让 RootView bootstrapStep1 closure 可同步注入已经拿到的 HomeData（避免 RootView 二次 await execute()）
public func applyHomeData(_ data: HomeData) {
    self.homeData = data
    self.loadingState = .loaded
    self.hasLoadedHome = true
}

// 失败路径：写入 loadingState + 调 ErrorPresenter 弹 RetryView；onRetry 闭包让用户主动重试
private func applyHomeError(_ error: Error) {
    self.loadingState = .failed(message: errorMessageFor(error))
    self.hasLoadedHome = true   // 失败也置 true，避免反复重试；用户走 onRetry 显式重置
    boundErrorPresenter?.present(error: error, onRetry: { [weak self] in
        Task { @MainActor [weak self] in
            self?.resetLoadHomeForRetry()
            await self?.loadHome()
        }
    }) ?? errorPresenter?.present(error: error, onRetry: { ... })  // 同 fallback
}

// 重试入口：清 hasLoadedHome flag 让 loadHome() 可再跑一次
public func resetLoadHomeForRetry() {
    hasLoadedHome = false
    loadingState = .idle
}

// 错误转文案（短期为 LocalizedError? errorDescription : defaultMessage；与 AppLaunchStateMachine.messageFor 同精神）
private func errorMessageFor(_ error: Error) -> String {
    if let localized = error as? LocalizedError, let desc = localized.errorDescription, !desc.isEmpty {
        return desc
    }
    return "首屏加载失败，请重试"
}

public enum HomeLoadingState: Equatable {
    case idle
    case loading
    case loaded
    case failed(message: String)
}
```

**具体行为要求**：

- **追加**式改动：所有既有字段 / 方法 / init 签名零改动 —— Preview / 老测试 / UITest skip-guest-login 路径零回归
- `loadHome()` 三层短路：跨 task 边界（hasLoadedHome）/ 同 task 内并发（loadHomeTask）/ UseCase 未注入（no-op）—— 与 `start()` 同模式
- `loadHome()` **失败也置 hasLoadedHome=true**：避免 server 不可达时反复重试；用户主动重试通过 `resetLoadHomeForRetry()` + `loadHome()` 显式重入
- `applyHomeData(_:)` 是 public：让 RootView bootstrapStep1 closure 可同步注入（避免 ViewModel 重新 await execute() 引发"启动时拉一次 + ViewModel.loadHome() 又拉一次" 双发请求）
- `errorPresenter` 是 weak（避免 Closure 持 strong 引发的循环引用：`errorPresenter` → onRetry closure → ViewModel → ...）
- `loadingState` 是 `Equatable` enum：HomeView / 测试可断言 `XCTAssertEqual(viewModel.loadingState, .loaded)`
- `HomeLoadingState.failed(message:)` 携带文案：让 `home_loadingState` a11y 可暴露；测试可断言文案
- 与既有 `start()` / `pingUseCase` 路径并存：两条 await 在 `.task` 内是独立 closure（不互相阻塞）

**AC6 — RootView wire：bootstrapStep1 closure 串行调 GuestLogin → LoadHome**

修改 `iphone/PetApp/App/RootView.swift` 的 `ensureLaunchStateMachineWired()`：

```swift
private func ensureLaunchStateMachineWired() {
    guard launchStateMachine == nil else { return }

    #if DEBUG
    if ProcessInfo.processInfo.environment["UITEST_SKIP_GUEST_LOGIN"] == "1" {
        launchStateMachine = AppLaunchStateMachine()
        return
    }
    #endif

    let guestLoginUseCase = container.makeGuestLoginUseCase()
    let loadHomeUseCase = container.makeLoadHomeUseCase()  // 新增
    let sessionStore = container.sessionStore
    let homeViewModel = self.homeViewModel               // capture

    launchStateMachine = AppLaunchStateMachine(
        bootstrapStep1: { @Sendable in
            // Step 1a: 游客登录（既有）
            let output = try await guestLoginUseCase.execute()
            await MainActor.run {
                sessionStore.updateSession(SessionState(user: output.user, pet: output.pet))
            }
            // Step 1b: 立即拉首屏数据（新增 —— Story 5.5 AC6）
            // 失败抛错让 step1 整体失败 → 状态机走 .needsAuth(message:) → 显示 RetryView
            let homeData = try await loadHomeUseCase.execute()
            await MainActor.run {
                homeViewModel.applyHomeData(homeData)
            }
        }
        // bootstrapStep2 仍走默认 { } no-op；本 story 故意不用 step2 插槽
        // （让 GuestLogin + LoadHome 是单次原子启动事务的失败语义；详 Dev Note #2）
    )
}
```

**新增 `.task` wire 用于支持用户主动重试时的 ErrorPresenter onRetry 闭包路径**：

在 `RootView.body` 的 `.task` 内追加 wire `homeViewModel.bind(loadHomeUseCase:errorPresenter:)`：

```swift
.task {
    homeViewModel.bind(pingUseCase: container.makePingUseCase())
    homeViewModel.bind(
        loadHomeUseCase: container.makeLoadHomeUseCase(),
        errorPresenter: container.errorPresenter
    )  // 新增
    await homeViewModel.start()
}
```

**具体行为要求**：

- bootstrapStep1 closure 内**串行**（不并行）调 GuestLogin → LoadHome：因为 LoadHome 需要 token（GuestLogin 写 keychain 完成后才有 token）；并行会导致 LoadHome 先发 → 401 → 静默重登触发 → 复杂边界
- bootstrapStep1 任一步失败 → 整体抛错 → 状态机 `.needsAuth(message:)` → RetryView；用户点重试 → `retry()` → 重跑整个 closure（GuestLogin + LoadHome 都重跑）—— 与节点 2 §4.2 验收"启动失败可重试"对齐
- `applyHomeData(_:)` 同步注入避免 RootView 启动期拉一次 + ViewModel 自己再调 `loadHome()` 二次发起请求（hasLoadedHome 在 applyHomeData 内置 true，bind 后的 ViewModel.loadHome() 路径会被短路）
- `bind(loadHomeUseCase:errorPresenter:)` 在 `.task` 内 wire：让 ErrorPresenter onRetry 闭包能驱动 ViewModel 重试（refer AC5）—— 即便启动期 LoadHome 已成功，用户仍可在某些边界场景（如未来加 pull-to-refresh）走 ViewModel.loadHome() 路径，bind 后 ErrorPresenter 路径才完整
- UITEST_SKIP_GUEST_LOGIN 路径继续走 `AppLaunchStateMachine()` 默认无 closure → 启动立即 `.ready` 显示 HomeView placeholder（保 UITest 不依赖真实 server）

**AC7 — `HomeView` 投影 HomeData 到主界面区块**

修改 `iphone/PetApp/Features/Home/Views/HomeView.swift`，**追加**（不删除既有 view 结构）：

- `petAndChestRow` 内 `petArea` 下方追加 `Text(viewModel.homeData?.pet?.name ?? "默认小猫")`，加 a11y identifier `home_petName`
- `chestArea` 上方追加 `Text(viewModel.homeData?.chestRemainingDisplay ?? "--:--")`，加 a11y identifier `home_chestRemaining`
- `stepBalanceLabel` 文案改为 `Text("\(viewModel.homeData?.stepAccount.availableSteps ?? 0) 步")`，a11y identifier 保留 `home_stepBalance`
- `userInfoBar` **不动**（仍走 SessionStore；详 Story 5.2 lesson 2026-04-27-sessionstore-home-nickname-source-of-truth.md）
- 三按钮 / 版本号 / debug 重置按钮全部不动

修改 `iphone/PetApp/Shared/Constants/AccessibilityID.swift` 的 `Home` enum，**追加**：

```swift
public static let petName = "home_petName"
public static let chestRemaining = "home_chestRemaining"
```

**具体行为要求**：

- `homeData` 为 nil 时 placeholder（"默认小猫" / "--:--" / "0 步"）—— 保 Preview / UITest skip-guest-login 路径渲染正常
- `homeData` 非 nil 时显示真实数据 —— 启动成功 + LoadHome 成功后由 SwiftUI 重渲染
- a11y identifier 新增两个常量化：`home_petName` / `home_chestRemaining`
- 不渲染 chest.status 不同视觉（counting vs unlockable）—— 节点 7 / Story 21.2 才接；本 story 仅显示倒计时
- 不渲染 pet.equips —— 节点 9 / Story 30.x 才接（本 story 即便 server 返非空 equips 也不展示）
- 不渲染 room.currentRoomId —— 节点 4 / Story 12.x 才接

**AC8 — `AppContainer` 工厂方法：makeHomeRepository / makeLoadHomeUseCase**

修改 `iphone/PetApp/App/AppContainer.swift`，**追加**：

```swift
/// Story 5.5 新增：构造 HomeRepository（DefaultHomeRepository）.
/// Repository 是 value type struct；apiClient 单例由 container 持有（已被 Story 5.4 装饰器包装）.
public func makeHomeRepository() -> HomeRepositoryProtocol {
    DefaultHomeRepository(apiClient: apiClient)
}

/// Story 5.5 新增：构造 LoadHomeUseCase.
/// UseCase 是 value type struct；每次调用返回新实例；repository 也是新实例（廉价）.
public func makeLoadHomeUseCase() -> LoadHomeUseCaseProtocol {
    DefaultLoadHomeUseCase(repository: makeHomeRepository())
}
```

**具体行为要求**：

- 与既有 `makeAuthRepository` / `makeGuestLoginUseCase` 同模式（factory method 模式 —— UseCase / Repo 是 value type，按需构造）
- `apiClient` 自动是 wrap 过的（containerinit 行 89 包了 AuthRetryingAPIClient）—— LoadHomeUseCase 自动获得 401 静默重登能力
- 不持 `loadHomeUseCase` 字段（每次调 factory method 拿新实例，符合 value type 语义）
- 不修改既有 init / 其它字段

**AC9 — `LoadHomeUseCaseTests`：覆盖 ≥ 5 case**

新建 `iphone/PetAppTests/Features/Home/UseCases/LoadHomeUseCaseTests.swift`：

- **case#1 happy**：repo.loadHome 返回完整 HomeResponse → execute 返 HomeData，字段一一对应（user.id / pet.name / pet.currentState=.rest / stepAccount 全 0 / chest.status=.counting / chest.remainingDisplay="10:00" / room.currentRoomId=nil）
- **case#2 happy edge**：repo 返回 pet=nil → execute 返 HomeData.pet == nil（其它字段正常）
- **case#3 happy edge**：repo 返回 pet.equips=[]（节点 2 阶段固定情形）→ execute 返 HomeData.pet?.equips == []
- **case#4 happy edge**：repo 返回 chest.status=2 + remainingSeconds=0 → HomeData.chest.status=.unlockable + remainingDisplay="00:00"
- **case#5 happy edge**：repo 返回 room.currentRoomId="3001"（节点 4 后场景模拟）→ HomeData.room.currentRoomId=="3001"
- **case#6 edge**：repo 抛 APIError.business(1009, "服务繁忙", "req_xxx") → execute 透传 .business
- **case#7 edge**：repo 抛 APIError.network(URLError(.timedOut)) → execute 透传 .network
- **case#8 edge**：repo 抛 APIError.unauthorized → execute 透传 .unauthorized（不在 UseCase 层做特殊处理）
- **case#9 edge**：repo 返回 unknown chest.status=99（理论不应发生，防御性）→ HomeData.chest.status fallback 到 .counting（不抛 decoding error）
- **case#10 edge**：repo 返回 unknown pet.currentState=99 → HomePet.currentState fallback 到 .rest

**具体行为要求**：

- ≥ 5 case，至少覆盖 happy / pet=nil / equips=[] / failure 透传 / unknown enum fallback
- 用 `MockHomeRepository`（继承 MockBase + `loadHomeStub: Result<HomeResponse, Error>`）
- 验证 `mock.callCount(of: "loadHome()")` == 1（每个 happy case）
- 失败 case 用 `XCTAssertThrowsError` + 断言错误类型（`XCTAssertEqual(error as? APIError, .business(...))`）

**AC10 — `HomeRepositoryTests`：覆盖 ≥ 3 case**

新建 `iphone/PetAppTests/Features/Home/Repositories/HomeRepositoryTests.swift`：

- **case#1 happy**：mock APIClient stub `/home` → success(HomeResponse(...)) → loadHome() 返同 instance；验证 invocations.count == 1 + endpoint.path == "/home" + endpoint.method == .get + endpoint.requiresAuth == true
- **case#2 edge**：mock APIClient stub `/home` → failure(.business(1009, ...)) → loadHome() 透传 .business
- **case#3 edge**：mock APIClient stub `/home` → failure(.network(URLError(.notConnectedToInternet))) → loadHome() 透传 .network
- **case#4 edge**：mock APIClient stub `/home` → failure(.unauthorized) → loadHome() 透传 .unauthorized

**具体行为要求**：

- ≥ 3 case，覆盖 happy + 至少 2 个错误透传
- 用 Story 2.5 落地的 `MockAPIClient`（`PetAppTests/Features/Home/UseCases/MockAPIClient.swift`）
- 验证 endpoint **属性**：`path == "/home"` + `method == .get` + `requiresAuth == true` + `body == nil`

**AC11 — `HomeResponseDecodingTests`：锁定 wire 解码契约 ≥ 4 case**

新建 `iphone/PetAppTests/Features/Home/Models/HomeResponseDecodingTests.swift`：

- **case#1 happy**：完整 JSON（V1 §5.1 节点 2 阶段示例 —— 行 351-388） → 解码成 HomeResponse 全部字段正确（含 unlockAt 是 Date）
- **case#2 happy edge**：JSON `"pet": null` → 解码 HomeResponse.pet == nil（不抛 decoding error）
- **case#3 happy edge**：JSON `"room": { "currentRoomId": null }` → HomeResponse.room.currentRoomId == nil
- **case#4 happy edge**：JSON `"pet.equips": []` → HomeResponse.pet?.equips == []
- **case#5 edge**：JSON 缺 `unlockAt` 字段 → 抛 DecodingError（验证 schema 严格性）
- **case#6 edge**：JSON `unlockAt` 是非 ISO8601 字符串（如 "2026/04/23 10:20:00"）→ 抛 DecodingError（验证 dateDecodingStrategy 严格）
- **case#7 happy edge**：JSON `pet.equips` 含一个完整 equip 对象（节点 9 后真实场景模拟）→ 解码 HomePetDTO.equips.count == 1 + 字段全对

**具体行为要求**：

- ≥ 4 case，覆盖 happy + pet=null + room.currentRoomId=null + 字段缺失 / 类型错的 decoding 失败
- 用 `JSONDecoder` 直接解码 raw JSON（覆盖 wire schema 严格性）
- decoder 配置：`.iso8601` for dateDecodingStrategy（与 APIClient 内的 decoder 配置对齐 —— 详见下方 Dev Note #4）
- 让本测试**自带** 默认 decoder 配置（独立于 APIClient）—— 让 schema 解码契约与 APIClient 行为解耦，避免 APIClient 改 decoder 配置时本测试 silent pass

**AC12 — `HomeViewModelLoadHomeTests`：覆盖 ≥ 4 case**

新建 `iphone/PetAppTests/Features/Home/ViewModels/HomeViewModelLoadHomeTests.swift`：

- **case#1 happy**：mock UseCase stub success(完整 HomeData) → loadHome() → viewModel.homeData == 同 instance + loadingState == .loaded + hasLoadedHome == true（通过 callCount("execute()") 推断）
- **case#2 edge**：mock UseCase stub failure(.business(1009, ...)) → loadHome() → loadingState == .failed(message:) + errorPresenter.current 含 RetryView 呈现项
- **case#3 happy** 重复短路：mock UseCase 调用一次 → loadHome() 跑完 → 再调 loadHome() → mock.callCount == 1（hasLoadedHome 短路生效）
- **case#4 happy** 重试：先调一次 loadHome 失败 → resetLoadHomeForRetry() → 再调 loadHome（mock 改成 success） → loadingState == .loaded + mock.callCount == 2
- **case#5 edge**：未注入 UseCase → loadHome() no-op，viewModel.homeData == nil + loadingState == .idle
- **case#6 happy**：applyHomeData(homeData) 直接调 → viewModel.homeData == 同 instance + loadingState == .loaded + hasLoadedHome == true（之后 loadHome 短路）

**具体行为要求**：

- ≥ 4 case，覆盖 happy + failure + 重复短路 + 重试 + applyHomeData 直注入
- 用 `MockLoadHomeUseCase`（继承 MockBase + executeStub）
- ErrorPresenter 注入 `ErrorPresenter(toastDuration: 0.05)` 加速
- 验证 `errorPresenter.current` 是 `.retry(...)` 呈现项（断言 case，不必断言全部字段）
- 测试 `@MainActor`：用 `await MainActor.run { ... }` 或测试方法标 `@MainActor`

**AC13 — 全部 `bash scripts/build.sh --test`（如适用 iphone CI 入口）跑通**

注：`scripts/build.sh` 是 server 端入口（CLAUDE.md 明确 server-only）。iPhone 端跑测的命令在 `iphone/scripts/`（参考 Story 2.7 落地的 `iphone/scripts/test.sh` 或类似入口）。dev 跑：

```bash
bash iphone/scripts/test.sh
```

或在 Xcode 里 cmd+U（XCTest 全跑）。

**具体行为要求**：

- 新增的所有 .swift 测试文件都被 `iphone/project.yml` 的 `sources: - PetAppTests` glob 自动纳入（**不**改 yml）
- 全部测试 PASS（含本 story 新增 + 既有所有 stories 落地的测试零回归）
- `swift build` 无 warning（含 Sendable / @MainActor 类型推导无歧义）
- `git status` `ios/` 下零改动 / `server/` 下零改动 / `iphone/scripts/` 下零改动 / `iphone/project.yml` 下零改动

## Tasks / Subtasks

- [x] Task 1：DTO + domain 层（AC1 + AC2）
  - [x] 1.1 新建 `iphone/PetApp/Features/Home/Models/HomeResponse.swift`：HomeResponse + UserInfoDTO + HomePetDTO + EquipDTO + StepAccountDTO + ChestDTO + RoomDTO
  - [x] 1.2 新建 `iphone/PetApp/Features/Home/Models/HomeData.swift`：HomeData + HomeUser + HomePet + HomePetState + HomeEquip + HomeStepAccount + HomeChest + HomeChestStatus + HomeRoom + chestRemainingDisplay derived
- [x] Task 2：Repository + Endpoint（AC3）
  - [x] 2.1 新建 `iphone/PetApp/Features/Home/UseCases/HomeEndpoints.swift`
  - [x] 2.2 新建 `iphone/PetApp/Features/Home/Repositories/HomeRepository.swift`
- [x] Task 3：UseCase（AC4）
  - [x] 3.1 新建 `iphone/PetApp/Features/Home/UseCases/LoadHomeUseCase.swift`
- [x] Task 4：HomeViewModel 改造（AC5）
  - [x] 4.1 修改 `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`：追加新字段 + 新 init + bind + loadHome + applyHomeData + applyHomeError + resetLoadHomeForRetry + errorMessageFor + HomeLoadingState enum
- [x] Task 5：RootView wire（AC6）
  - [x] 5.1 修改 `iphone/PetApp/App/RootView.swift` `ensureLaunchStateMachineWired`：在 bootstrapStep1 closure 内串行调 GuestLogin → LoadHome → applyHomeData
  - [x] 5.2 修改 `.task` wire homeViewModel.bind(loadHomeUseCase:errorPresenter:)
- [x] Task 6：HomeView 投影（AC7）
  - [x] 6.1 修改 `iphone/PetApp/Features/Home/Views/HomeView.swift`：petArea 下加 Text(petName) / chestArea 上加 Text(remainingDisplay) / stepBalanceLabel 改 dynamic 文案
  - [x] 6.2 修改 `iphone/PetApp/Shared/Constants/AccessibilityID.swift`：追加 home_petName / home_chestRemaining
- [x] Task 7：AppContainer 工厂（AC8）
  - [x] 7.1 修改 `iphone/PetApp/App/AppContainer.swift`：追加 makeHomeRepository + makeLoadHomeUseCase
- [x] Task 8：测试基础设施
  - [x] 8.1 新建 `iphone/PetAppTests/Features/Home/UseCases/MockHomeRepository.swift`：继承 MockBase + loadHomeStub
  - [x] 8.2 新建 `iphone/PetAppTests/Features/Home/UseCases/MockLoadHomeUseCase.swift`：继承 MockBase + executeStub
- [x] Task 9：测试覆盖
  - [x] 9.1 新建 `iphone/PetAppTests/Features/Home/UseCases/LoadHomeUseCaseTests.swift`（10 case 覆盖 AC9）
  - [x] 9.2 新建 `iphone/PetAppTests/Features/Home/Repositories/HomeRepositoryTests.swift`（4 case 覆盖 AC10）
  - [x] 9.3 新建 `iphone/PetAppTests/Features/Home/Models/HomeResponseDecodingTests.swift`（7 case 覆盖 AC11）
  - [x] 9.4 新建 `iphone/PetAppTests/Features/Home/ViewModels/HomeViewModelLoadHomeTests.swift`（8 case 覆盖 AC12）
- [x] Task 10：CI / 回归
  - [x] 10.1 跑 `bash iphone/scripts/build.sh --test`（test.sh 不存在；scripts 入口实为 build.sh --test）—— 218 tests passed
  - [x] 10.2 `git status` 验证 `ios/` / `server/` / `iphone/scripts/` / `iphone/project.yml` 全 0 改动

## Dev Notes

### #1 双 DTO 体系：HomeResponse vs GuestLoginResponse 的字段重叠为何不合并

GuestLoginResponse.UserProfile / PetProfile（Story 5.2 落地）与 HomeResponse.UserInfoDTO / HomePetDTO 在节点 2 阶段字段几乎重叠（user 仅 hasBoundWechat 区别；pet 仅 currentState / equips 区别）。**不合并**的理由：

- **演化方向不同**：UserProfile 节点 2+ 会增 `hasBoundWechat` / 节点 12 后 `wechatNickname`（绑定微信链路）；HomeUser 节点 2 完全不需要 `hasBoundWechat`（首屏不显示是否绑定微信）。两者跨 epic 分别演化。
- **字段集不同**：HomePet 必含 `currentState`（GuestLogin 接口不返）+ `equips`（GuestLogin 接口不返）。把两者合并需要把 `currentState` / `equips` 升级为 Optional 并散落"在 GuestLogin 路径上不存在"的语义诅咒。
- **跨 Feature 耦合的 future 痛**：Story 26.6 / Story 11.10 / Story 29.6 都改 HomeResponse —— 如果 HomeResponse 复用 Auth 模块的类型，每次 server 改 home schema 都要冒"误伤 Auth 路径"的风险。
- **一致命名 + 独立类型**：`UserInfoDTO` / `HomeUser` 用"模块前缀 + 通用名"模式（与 `GuestLoginResponse.UserProfile` 区分开），让代码 reader 一眼分辨"this is from /home" vs "this is from /auth".

**反模式（不要这样写）**：用 typealias `typealias HomeUserInfoDTO = UserProfile` "复用" —— typealias 看似无害但让"两个 endpoint 的 wire 解码契约 共享一个类型定义"，未来 server 任一接口改 schema 时另一接口的解码会沉默 break。

### #2 LoadHome 调用为何合到 bootstrapStep1 而不用 bootstrapStep2 插槽

`AppLaunchStateMachine` 暴露了 bootstrapStep1 / bootstrapStep2 两个插槽（行 41-47）；初印象是"既然 5.5 是 5.2 的下一步，自然该用 step2"。**不用 step2** 的理由：

- **失败语义复杂化**：用 step2 → "step1 成功 + step2 失败" 是不同的中间态：keychain 里有了 token / SessionStore 已写入 user 的"半启动"状态。让 ErrorPresenter / 用户重试时看到的是"我已经登录但拉不到首屏"。这种状态的恢复语义没有清晰定义（重新走 step1 会重生 guest-login？只重跑 step2？state 机怎么表达？）。
- **业务上 LoadHome 是 GuestLogin 的延伸**：不能拉首屏的话 GuestLogin 的"成功"意义不大（用户进了 App 但首屏空白）；让两者合到同一原子事务的失败语义统一为"启动失败、点重试"。
- **rePI 重试时一并重跑**：合到 step1 → 用户在 RetryView 点重试 → 整个 step1 closure 重跑（GuestLogin + LoadHome 都重跑）—— 简单干净。
- **bootstrapStep2 仍保留**：未来如果有"启动期非阻塞预热"任务（如预拉缓存 / 预连 WebSocket）才用 step2。本 story 故意不用，保留干净的"启动 = 单原子事务" 语义。

### #3 HomeViewModel.applyHomeData 为何 public + 同步注入

RootView bootstrapStep1 closure 已经 await 完 `loadHomeUseCase.execute()` 拿到 HomeData；如果 ViewModel 自己再调 `loadHome()` 会**第二次**发起 GET /home 请求（hasLoadedHome 短路要等到 ViewModel 第一次 loadHome 跑完才生效）。**applyHomeData(_:) 同步注入**的目的：

- 让 RootView 调一次 LoadHome，结果**直接喂给** ViewModel；ViewModel 内 hasLoadedHome 立即置 true → 后续 ViewModel.loadHome() 路径短路 → 不会双发请求
- ViewModel.loadHome() 路径仍存在 —— 让 ErrorPresenter onRetry 闭包 / 未来 pull-to-refresh 走该路径
- 与 Story 5.2 SessionStore.updateSession 同精神：side effect 集中在 closure 注入点（RootView），ViewModel / Store 仅暴露 setter

**反模式**：让 ViewModel 自己持有 GuestLoginUseCase + LoadHomeUseCase 在 .task 内自己跑链 —— 把启动序列从 AppLaunchStateMachine 抽空，state 机变废物，且失败的 RetryView 路径要重新设计。

### #4 JSONDecoder 配置：APIClient 端 .iso8601 必须就位

`HomeResponse.ChestDTO.unlockAt` 是 `Date` 类型；wire JSON 是 ISO8601 字符串（server `homeResponseDTO` 用 `time.RFC3339`）。Swift 默认 JSONDecoder 不解 ISO8601，必须 `decoder.dateDecodingStrategy = .iso8601`。

**检查 APIClient 当前 dateDecodingStrategy 配置**（dev 在改前必须确认）：
- 若 APIClient 已配 `.iso8601` → 本 story **零** 改动 APIClient
- 若 APIClient 未配（默认 `.deferredToDate`，期望 number）→ **必须**先在 APIClient 内加 `decoder.dateDecodingStrategy = .iso8601`；这是 minor 改动，但会影响所有未来用 Date 的接口；记一条 lesson "all wire DTO use ISO8601 for Date"

预期路径：APIClient 内 decoder 已经是默认 `JSONDecoder()`（无 .iso8601）—— dev 必须改一次 + 提一次"所有 future endpoint 用 Date 字段时 server 端必须用 RFC3339" 的契约；或者 HomeResponse.ChestDTO.unlockAt 类型改为 `String` 让 ViewModel 自己 parse（**不推荐** —— 失去 Date 类型安全 + 多处 parse 散落）。

dev 自决（推荐改 APIClient）—— 如果选择改 APIClient，要在 PR 描述中显式说明该副作用；如果选择 String + ViewModel parse，要在 HomeChest 内提供 `unlockAtParsed: Date?` derived 字段并加单测。

### #5 ErrorPresenter weak vs strong：避免循环引用

`HomeViewModel` 持有 `errorPresenter`；errorPresenter.present(error:onRetry:) 的 onRetry closure capture self（ViewModel）—— 若 errorPresenter 是 strong 引用，且 ErrorPresenter 内部把 onRetry 入队，ViewModel 与 ErrorPresenter 互相持有，造成循环。

**解法**：ViewModel 持 `errorPresenter` 用 `weak var`（仅在 bound 路径上）；container.errorPresenter 是单例（stable），weak ref 不会过早释放。init 路径接受的 errorPresenter 测试场景独立 new 不需 weak（测试场景 ViewModel + ErrorPresenter 短生命周期）。

**反模式**：在 onRetry closure 内不写 `[weak self]` —— closure 持有 ViewModel strong，ErrorPresenter 持 closure strong → ViewModel 永远释放不掉。

### #6 hasLoadedHome 失败也置 true 的理由（与 hasFetched 同模式）

启动失败时 ErrorPresenter onRetry 路径走 `resetLoadHomeForRetry()` 显式清 flag —— 不通过自动重试。理由：
- server 不可达时反复重试 = 网络流量爆炸（参考 Story 2.5 lesson 2026-04-26-swiftui-task-modifier-reentrancy.md）
- 用户主动重试 = 显式信号 —— 走 onRetry 闭包时清 flag 让 loadHome() 可再跑一次
- 与 `HomeViewModel.start()` 的 hasFetched 同精神

### #7 服务端 chest.status 节点 2 阶段动态判定的客户端解析

V1 §5.1 行 345 + Story 4.8 落地 `homeResponseDTO`：server 在 节点 2 阶段动态判定 `time.Now().UTC() >= unlock_at ? 2 : 1`。客户端的 `HomeChestStatus` enum 同时支持 1=counting / 2=unlockable —— 两个状态都能解析 + 渲染。本 story **不**新增"客户端动态判定"逻辑（server 已动态判定）；但 `chest.status` 与 `chest.remainingSeconds` 需保证**同时下发**（同一次响应内一致），客户端不二次推断。

未来 Story 21.2（节点 7 chest 链路）才接"客户端起本地 timer 看着 remainingSeconds 倒数 + 到 0 自动转 unlockable"逻辑 —— 本 story 不预设。

### #8 与 Story 5.4 装饰器的透明协作

LoadHome 调 GET /home 拿到 401 → AuthRetryingAPIClient.request<T> 内 catch .unauthorized + endpoint.requiresAuth=true → coordinator.relogin() → inner.request 重试一次。整个流程对 LoadHomeUseCase / HomeViewModel / RootView **完全透明**。本 story 仅需保证：

- `Endpoint.requiresAuth = true`（让装饰器认得这是受保护请求）
- LoadHomeUseCase 不在内部 catch .unauthorized 做特殊处理（让装饰器路径生效）
- RootView bootstrapStep1 closure 收到 .unauthorized（即装饰器重试后仍 401）走 `.needsAuth(message:)` 路径（与 5.2 一致）

### Project Structure Notes

新建文件路径全部走 `iphone/PetApp/Features/Home/` 下的子目录（与既有 Auth Feature 同模式）：

- `Features/Home/Models/HomeData.swift`（domain 层）
- `Features/Home/Models/HomeResponse.swift`（wire DTO 层）
- `Features/Home/Repositories/HomeRepository.swift`
- `Features/Home/UseCases/HomeEndpoints.swift`
- `Features/Home/UseCases/LoadHomeUseCase.swift`

测试目录映射：

- `PetAppTests/Features/Home/Models/HomeResponseDecodingTests.swift`
- `PetAppTests/Features/Home/Repositories/HomeRepositoryTests.swift`
- `PetAppTests/Features/Home/UseCases/LoadHomeUseCaseTests.swift`
- `PetAppTests/Features/Home/UseCases/MockHomeRepository.swift`
- `PetAppTests/Features/Home/UseCases/MockLoadHomeUseCase.swift`
- `PetAppTests/Features/Home/ViewModels/HomeViewModelLoadHomeTests.swift`

`iphone/project.yml` 既有 `sources: - PetApp` / `sources: - PetAppTests` glob 自动纳入新文件 —— **不**改 yml。

### References

- 设计文档（必读）
  - [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#5.4 Repository 层] —— `HomeRepository` 列示
  - [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#5.3 UseCase 层] —— `LoadHomeUseCase` 列示
  - [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#6.2 Home 模块] —— Home 模块职责 + LoadHomeUseCase 落地点
  - [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#12.1 App 启动链路] —— 启动 → 登录 → 拉首屏 → 渲染主界面
  - [Source: docs/宠物互动App_V1接口设计.md#5.1 GET /api/v1/home] —— wire schema 冻结契约（行 312-450）
  - [Source: docs/宠物互动App_V1接口设计.md#§4.1 行 16] —— §5.1 schema 冻结声明
- 上承 stories
  - [Source: _bmad-output/implementation-artifacts/4-8-get-home-聚合接口.md] —— server `GET /home` 实装（已 done）
  - [Source: _bmad-output/implementation-artifacts/5-2-启动自动登录-usecase.md] —— GuestLoginUseCase 实装
  - [Source: _bmad-output/implementation-artifacts/5-3-apiclient-interceptor-自动注入-bearer-token.md] —— APIClient token 注入
  - [Source: _bmad-output/implementation-artifacts/5-4-无效-token-静默重新登录.md] —— AuthRetryingAPIClient 装饰器
- 实装契约（server-side）
  - [Source: server/internal/app/http/handler/home_handler.go#homeResponseDTO（行 98-138）] —— wire 字段 source of truth
- ADR / lesson（必读）
  - [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md] —— iPhone 端工程目录决策
  - [Source: _bmad-output/implementation-artifacts/decisions/0007-context-propagation.md（如适用）]
  - [Source: docs/lessons/2026-04-27-sessionstore-home-nickname-source-of-truth.md] —— nickname 渲染源（不切到 HomeData）
  - [Source: docs/lessons/2026-04-26-契约schema字段可空性必须显式声明.md] —— pet / room.currentRoomId 可空字段处理
  - [Source: docs/lessons/2026-04-26-v1接口设计-home-chest-status-必须严格按节点阶段限定状态空间.md] —— chest.status 节点 2 阶段范围
  - [Source: docs/lessons/2026-04-26-swiftui-task-modifier-reentrancy.md] —— hasLoadedHome 短路设计
  - [Source: docs/lessons/2026-04-26-stateobject-init-vs-bind-injection.md] —— bind path 设计
  - [Source: docs/lessons/2026-04-26-stateobject-debug-instance-aliasing.md] —— @StateObject lazy 注入模式
  - [Source: docs/lessons/2026-04-27-retry-decorator-changes-unauthorized-presentation-semantics.md] —— 装饰器透明协作

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- 测试运行：`bash iphone/scripts/build.sh --test` —— 218 tests passed (PetAppTests bundle)
- 中途修复 1：MainActor isolation —— LoadHomeUseCaseTests 的 `defaultPet` / `defaultChest` 静态 helper 当作 default argument 使用时被 Swift 6 拒（main-actor static 不能在 nonisolated context 评估）；改为 `nonisolated static`.
- 中途修复 2：HomeViewModelLoadHomeTests testLoadHomeFailureUpdatesStateAndPresentsRetry —— 误用 business(1009) 期望弹 .retry，实际 AppErrorMapper 把 business → .alert（仅 .network → .retry）；测试拆成两 case：network → retry / business → alert.

### Completion Notes List

实装完成情况（按 AC 维度）：

- AC1（HomeResponse + 子结构）✅：完整对齐 server homeResponseDTO；pet / room.currentRoomId 双 Optional；unlockAt 用 Date + .iso8601 解码策略
- AC2（HomeData + init(from:)）✅：HomeChestStatus / HomePetState 用 raw value enum + fallback；HomeChest.remainingDisplay 纯函数 derived 字段
- AC3（HomeRepository + HomeEndpoints）✅：path = `/api/v1/home`（与 AuthEndpoints 同模式，含 /api/v1 前缀），requiresAuth=true 让装饰器接管 401
- AC4（LoadHomeUseCase）✅：透传所有 APIError，不做错误码翻译
- AC5（HomeViewModel 追加 LoadHome 路径）✅：3 个 init 并存（老 / Story 2.5 / Story 5.5）；hasLoadedHome 三层短路；ErrorPresenter weak 引用避免循环
- AC6（RootView wire）✅：bootstrapStep1 closure 串行 GuestLogin → LoadHome → applyHomeData；.task 内 bind ErrorPresenter；UITEST_SKIP_GUEST_LOGIN 路径仍走 no-op
- AC7（HomeView 投影）✅：petArea 下加 home_petName Text；chestArea 上加 home_chestRemaining Text；stepBalanceLabel 改 dynamic 文案；homeData=nil 时显示 placeholder
- AC8（AppContainer 工厂）✅：makeHomeRepository / makeLoadHomeUseCase 与既有 factory method 同模式
- AC9（LoadHomeUseCaseTests）✅：10 case（happy + pet=nil + equips=[] + unlockable + room non-nil + business / network / unauthorized 透传 + chest.status fallback + pet.currentState fallback）
- AC10（HomeRepositoryTests）✅：4 case（happy endpoint 属性 + business / network / unauthorized 透传）
- AC11（HomeResponseDecodingTests）✅：7 case（完整 JSON + pet=null + room.currentRoomId=null + equips=[] + 缺 unlockAt → DecodingError + 非 ISO8601 unlockAt → DecodingError + 完整 equip）
- AC12（HomeViewModelLoadHomeTests）✅：8 case（success / network failure → retry / business failure → alert / 重复短路 / 重试 / 未注入 noop / applyHomeData 直注入 + 后续短路 / bind 路径）
- AC13（build + test 跑通）✅：218 tests passed；ios / server / iphone/scripts / iphone/project.yml 全 0 改动

副作用记录：
- APIClient.makeDecoder() 加 `dateDecodingStrategy = .iso8601` —— 全局副作用，未来所有 wire DTO 的 Date 字段都将依赖 server 以 RFC3339 字符串下发；与 server homeResponseDTO 钦定的 `time.Time.Format(time.RFC3339)` 对齐.
- AppLaunchStateMachine 主体未动；LoadHome 调用合到 bootstrapStep1 closure 内（Dev Note #2 钦定）.

### File List

新建（生产代码）:
- `iphone/PetApp/Features/Home/Models/HomeResponse.swift`
- `iphone/PetApp/Features/Home/Models/HomeData.swift`
- `iphone/PetApp/Features/Home/Repositories/HomeRepository.swift`
- `iphone/PetApp/Features/Home/UseCases/HomeEndpoints.swift`
- `iphone/PetApp/Features/Home/UseCases/LoadHomeUseCase.swift`

新建（测试）:
- `iphone/PetAppTests/Features/Home/UseCases/MockHomeRepository.swift`
- `iphone/PetAppTests/Features/Home/UseCases/MockLoadHomeUseCase.swift`
- `iphone/PetAppTests/Features/Home/UseCases/LoadHomeUseCaseTests.swift`
- `iphone/PetAppTests/Features/Home/Repositories/HomeRepositoryTests.swift`
- `iphone/PetAppTests/Features/Home/Models/HomeResponseDecodingTests.swift`
- `iphone/PetAppTests/Features/Home/ViewModels/HomeViewModelLoadHomeTests.swift`

修改（生产代码）:
- `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift` (新增 homeData/loadingState 字段、新 init、bind/loadHome/applyHomeData/applyHomeError/resetLoadHomeForRetry/HomeLoadingState)
- `iphone/PetApp/Features/Home/Views/HomeView.swift` (petColumn / chestColumn 子组件投影 homeData; stepBalanceLabel 改 dynamic)
- `iphone/PetApp/App/AppContainer.swift` (新增 makeHomeRepository / makeLoadHomeUseCase)
- `iphone/PetApp/App/RootView.swift` (ensureLaunchStateMachineWired 内 bootstrapStep1 串接 LoadHome; .task 内 bind LoadHomeUseCase + ErrorPresenter)
- `iphone/PetApp/Shared/Constants/AccessibilityID.swift` (追加 home_petName / home_chestRemaining)
- `iphone/PetApp/Core/Networking/APIClient.swift` (makeDecoder 加 .iso8601 dateDecodingStrategy —— Dev Note #4 副作用)

自动生成（不计入 source 改动）:
- `iphone/PetApp.xcodeproj/project.pbxproj` (xcodegen 每次 build 自动 regen)

### Change Log

| Date | Description |
| ---- | ----------- |
| 2026-04-28 | Story 5.5 实装完成：LoadHomeUseCase + HomeRepository + HomeData/HomeResponse DTO；HomeViewModel 追加 LoadHome 路径；RootView bootstrapStep1 串接；HomeView 投影；APIClient 加 ISO8601 date decoding |
| 2026-04-28 | 测试覆盖：LoadHomeUseCaseTests (10) + HomeRepositoryTests (4) + HomeResponseDecodingTests (7) + HomeViewModelLoadHomeTests (8)；218 tests passed |
