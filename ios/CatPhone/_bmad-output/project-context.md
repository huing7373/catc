---
project_name: CatPhone
user_name: Developer
date: 2026-04-19
sections_completed:
  - technology_stack
  - language_specific_rules
  - framework_specific_rules
  - testing_rules
  - code_quality_style
  - dev_workflow
  - critical_dont_miss
status: complete
optimized_for_llm: true
existing_patterns_found: 0
status: discovery_complete
scan_sources:
  - /Users/zhuming/fork/catc/ (workspace root — 裤衩猫项目)
  - /Users/zhuming/fork/catc/ios/CatPhone/ (本 iOS 目标，SwiftUI + CatShared)
  - /Users/zhuming/fork/catc/ios/ (Cat.xcodeproj / CatWatch / CatShared siblings)
  - /Users/zhuming/fork/catc/server/ (Go 后端，Epic 0 已完成)
  - /Users/zhuming/fork/catc/docs/api/ (跨端契约 SSoT：openapi.yaml / ws-message-registry.md / integration-mvp-client-guide.md)
  - /Users/zhuming/fork/catc/docs/backend-architecture-guide.md (§21 工程纪律)
  - /Users/zhuming/fork/catc/server/_bmad-output/planning-artifacts/ (prd / architecture / epics 作为上游参考)
discovery_notes:
  ios_modules_in_scope:
    - 账号 (accounts)
    - 好友 (friends)
    - 仓库 (inventory / 盲盒权属)
    - 装扮 (dress-up / 皮肤)
    - 房间 (rooms + 实时在场)
    - 步数展示 (step count display)  # 2026-04-19 追加
  open_questions:
    - 步数数据源：HealthKit 本地读 vs Watch 同步 vs server 聚合（PRD / 架构阶段拍板）
    - 步数与 inventory 盲盒解锁门槛（200 步）如何共用同一数据视图
    - 房间模块走 MVP（3 条 DebugOnly）还是等 Epic 4.1 正式版
---

# Project Context for AI Agents — CatPhone (iOS)

_This file contains critical rules and patterns that AI agents must follow when implementing code in this project. Focus on unobvious details that agents might otherwise miss._

---

## Technology Stack & Versions

### iOS 目标（CatPhone）
- **Swift tools**: 5.9+（`CatShared/Package.swift` `swift-tools-version:5.9`）
- **Xcode**: 16.0（`project.yml`）
- **Deployment target**: **iOS 17.0**（不向下兼容 iOS 16，可安全使用 `@Observable` / `Observation` / `NavigationStack` / `ContentUnavailableView`）
- **Bundle ID**: `com.zhuming.cat.phone`
- **工程生成**: **XcodeGen** 驱动，`ios/project.yml` 是 SSoT
  - **禁止**直接编辑 `Cat.xcodeproj/`；改 `project.yml` 后 `xcodegen generate`
  - 新增 target / SDK 依赖 / 资源目录都写在 `project.yml` 里

### UI / 渲染
- **SwiftUI** — 主 UI 栈（`@main struct CatPhoneApp: App`）
- **SpriteKit.framework** — 承载 Spine 动画运行时
- **Spine 运行时** — 通过 **SwiftPM 拉 `esoteric-software/spine-runtimes`**（不 vendor 到 CatShared）
  - 升级策略：跟官方 tag，锁版本
  - 所有 Spine 资源按 `docs/api/Spine到AppleWatch资源导出方案.md`（待写）流程导出

### 系统框架（`project.yml` 已声明依赖）
- **HealthKit.framework** — **步数展示 + 盲盒解锁**的唯一本地数据源
- **WatchConnectivity.framework** — iPhone ↔ Watch 数据通道
- **SpriteKit.framework** — Spine 渲染承载

### 网络栈
- **HTTP + WebSocket 客户端硬性决定：仅用 `URLSession` + `URLSessionWebSocketTask`**
  - **不引**第三方（Alamofire / Starscream / Moya 等），与 server "不引过度依赖"纪律对齐
  - `CatShared/Sources/CatShared/Networking/` 当前空，由此目录承载统一 client
- **JSON 编解码硬约定**（来自 server DTO 事实）：
  - `JSONEncoder/Decoder` **不使用** `.convertFromSnakeCase` —— server 全 camelCase
  - 时间戳**不全局**用 `.iso8601`：
    - `serverTime` 等个别字段是 RFC3339 字符串 → per-field `dateDecodingStrategy` 或自定义
    - `tsMs` / `enqueuedAtMs` / `tsMs` 等是 `Int64` 毫秒 epoch → 解码为 `Int64` 不做日期转换
  - 小数：浮点字段谨慎；优先 `Int64` 毫秒 / 厘米等 **整数单位**
- **WS 信封硬约定**（与 server `internal/ws/envelope.go` 对齐）：
  - 请求：`{ id: String, type: String, payload?: JSON }`
  - 响应：`{ id?: String, ok: Bool, type: String, payload?: JSON, error?: { code, message } }`
  - 错误 code 是 `UPPER_SNAKE_CASE`（例 `UNKNOWN_MESSAGE_TYPE`）
- **HTTP 错误信封**：`{"error":{"code":"UPPER_SNAKE_CASE","message":"..."}}`，`retry_after` 类错误附 `Retry-After` header

### 持久化
- `CatShared/Persistence/LocalStore.swift` 已有骨架（作为本地持久化入口点）
- 优先使用 **SwiftData**（iOS 17 可用）或 `UserDefaults`（轻量偏好），**不引** Realm / CoreData 外挂

### 共享代码分层（Swift Package `CatShared`）
- `CatShared` library — Models / Networking / Persistence / Resources / UI / Utilities
  - iPhone + Watch 都依赖；放**跨端共享的原子能力**
- `CatCore` library — 依赖 `CatShared`；放更高层业务编排（use-case / 协议适配器 / domain service）
- **依赖方向硬规则**：`CatShared` **不得** `import CatCore`（反向依赖 = 编译错误 + PR 拒收）
- **CatPhone / CatWatch app target** 同时依赖 `CatShared` + `CatCore`（由 `project.yml` 绑定）

### 服务端契约版本锚点（跨端 SSoT）
- **OpenAPI**: `0.14.0-epic0`（`/Users/zhuming/fork/catc/docs/api/openapi.yaml`）
- **WS registry apiVersion**: `v1`（`/v1/platform/ws-registry` 响应）
- **启动必调**: `GET /v1/platform/ws-registry` 协商协议方言（FR59 / Story 0.14）
- **drift 守门**: server 侧 CI 有双 gate；iOS 发现不一致**以 server 代码为准**，反馈到后端仓

### 测试栈
- **XCTest** — `CatPhoneTests` / `CatSharedTests` / `CatCoreTests` 已在 `project.yml` 配置
- 新增 UI 测试需在 `project.yml` 显式加 target（如 `CatPhoneUITests`）

## Critical Implementation Rules

### Language-Specific Rules (Swift 5.9 / iOS 17)

#### 并发 (Concurrency)
- **Swift 6 严格并发：`-strict-concurrency=complete` 从一开始就开**
  - 所有新代码必须零并发 warning 过编译
  - 跨 actor 传 non-Sendable 引用类型禁止；必要时用值类型或 `@unchecked Sendable` 并写注释说明为什么安全
- 禁止 `DispatchQueue.main.async { ... }` 做 UI 更新 → 用 `@MainActor` + `await`
- 网络 / 耗时 API 一律 `async throws`；不写 completion handler 新 API
- WebSocket 读循环：`Task { while !Task.isCancelled { ... } }`，不用裸 GCD

#### Observation (iOS 17+)
- ViewModel / Store 用 **`@Observable`**，**禁用** `ObservableObject` + `@Published`
- View 端用 `@State var store = FooStore()` 或 `@Bindable`，**禁用** `@StateObject` / `@ObservedObject`

#### WebSocket 连接硬约定
- **单连接共享**：全 app 一条 `URLSessionWebSocketTask`，所有消息类型（room.* / action.* / friend.* / inventory.*）共用同一信封通道
  - 与 server `internal/ws/envelope.go` 设计一致
  - 重连逻辑集中在一处，不分散

#### 错误处理
- 域内错误用 **`enum: Error`**，不用 `NSError`
- server 错误映射：`ServerError { let code: String; let message: String }` 或按 code 分支的 enum
  - `UPPER_SNAKE_CASE` code 直接透传，**不**做大小写转换
- UI 层禁止 `try?` 吞错；要么冒泡要么显式展示
- `fatalError` 仅限"程序员错误"不变量断言；禁止出现在 release path 的业务分支

#### 值类型优先
- DTO / Model 一律 `struct: Codable`
- 仅在真正需要引用语义时用 class，配 `Hashable` / `Identifiable`

#### 命名 / 风格
- Type: `PascalCase`（遵循 `BlindBoxStatus` / `CatState` 样板）
- 属性 / 方法 / 枚举 case: `camelCase`
- 文件由 `swift-format` lint（CI 未接前先手工遵守；后续 epic 接入）

#### Optional / Nil
- API 边界返回的 Optional 必须 `guard let` 显式处理
- 禁止 `!` 强解（`IBOutlet` 等平台约定除外）
- WS `payload?` 空：`payload.map(decode) ?? EmptyPayload()`

### Framework-Specific Rules

#### SwiftUI (iOS 17)
- **导航骨架**：`TabView` **5 tab** — 账号 / 好友 / 仓库 / 装扮 / 房间
- 栈式导航用 `NavigationStack` + `NavigationPath`；**禁用** `NavigationView`
- 异步副作用一律 `.task { await ... }`；**禁用** `onAppear { Task { ... } }`（Task 无生命周期绑定会泄漏）
- DI：`@Environment(FooStore.self)` 注入 `@Observable` Store；**禁用** `@EnvironmentObject`
- 空状态用 `ContentUnavailableView`（iOS 17 内建）
- List vs ScrollView+LazyVStack：表格样式用 `List`；自由布局（仓库 / 装扮网格）用 `ScrollView + LazyVStack/LazyVGrid`
- `var body` 纯函数；副作用走 `.task` / `.onChange` / `.onReceive`
- Sheet / Alert modifier 挂在**触发它的 View** 上

#### 步数 SSoT 策略（**决定：方案 a 分工 SSoT**）
- **展示层**：iPhone 侧 **HealthKit 本地读**（`HKStatisticsCollectionQuery` 按天聚合 stepCount），离线可用、响应快
- **业务判定**（盲盒解锁 / 徽章 / 好友共同目标）：走 **server 权威值**
- UI 处理短暂不一致：显示本地步数即可；盲盒解锁态显示"解锁中..."等 server 回数
- **Agent 必守**：不得在 iPhone 侧用 HealthKit 数据**直接判定**能否开盲盒；所有解锁 UI 信号都从 server 事件驱动

#### HealthKit
- **Info.plist 必填** key（缺失即 crash）：
  - `NSHealthShareUsageDescription` — 读步数
  - `NSHealthUpdateUsageDescription` — 仅当需要写入（暂不需要，留空即可不加）
- **授权触发点**：首次进**仓库** tab 或**步数展示**首次渲染；**禁止** app launch 盲发授权请求
- **查询模式**：
  - 按天聚合 → `HKStatisticsCollectionQuery`（stepCount）
  - 增量更新 → `HKObserverQuery` + `HKAnchoredObjectQuery`
  - **禁止**高频轮询 `HKSampleQuery`（耗电 + iOS 可能吊销权限）
- 模拟器部分 query 返回空；真机验证归 Epic 9

#### WatchConnectivity
- `WCSession.default.activate()` 在 iPhone / Watch app 启动时各自调一次
- 通道选择（传错 = 延迟 / 掉包 / 电量炸）：
  - `updateApplicationContext` — 覆盖式最新状态（当前皮肤 / 当前房间 ID）
  - `transferUserInfo` — 队列化传递（盲盒解锁事件 / 签到事件）
  - `sendMessage` — 实时双工，需双方前台；不适合后台
- **禁止** `transferFile` 走 JSON（那是大资源用的）
- 每次调用前 `guard session.isReachable else { queue.append(...); return }`

#### WebSocket 订阅面划分（**iPhone 仅管理端**）
- iPhone 和 Watch **共用单条 WS 连接的协议**，但**订阅消息不同**：
  - iPhone 订阅：friend.* / inventory.* / skin.* / account.* / 房间邀请类
  - iPhone **不发**：`room.join` / `action.update` / `action.broadcast` — 这些是 Watch-only
  - iPhone 接收 `action.broadcast` 需看业务决定（可选：弹通知"你的好友在互动"）
- **iPhone "房间" tab 定位**（非实时）：
  - **未在房间**：显示"创建房间"+"加入房间"两个入口
  - **已在房间**：显示房间 ID / 成员列表（静态快照）/ 邀请好友 / 离开房间
  - **不做**实时 action 流（那只跑在 Watch 上）

#### Spine / SpriteKit
- SwiftUI 包 SpriteKit：用 **`SpriteView(scene:)`**（SwiftUI 原生），不手搓 `UIViewRepresentable`
- Spine 状态机**必须** wrap 成 `CatAnimationController` 级别的高层类
  - View 里只调语义动作：`.walk()` / `.sleep()` / `.emote(.heart)`
  - **禁止**在 View 里直接碰 `skeleton.animationState`
- Spine 渲染调用**必须 `@MainActor`**；后台计算态 → `await MainActor.run { ... }` 再切动画
- 资源导出遵循 `docs/api/Spine到AppleWatch资源导出方案.md`（文档待补）
- 发布版禁止 `SKView.showsFPS`

### Testing Rules

#### 测试金字塔
- **单元测试**（`XCTest`，占比最大）—— Model decode / domain 状态机 / use-case 编排
  - targets: `CatSharedTests` / `CatCoreTests` / `CatPhoneTests`
- **集成测试**（`XCTest`）—— WS 客户端对内存 mock / HealthKit 对 mock DataSource
  - **禁止**依赖真机 / 真实 server 进程 / Watch 配对
- **UI 测试**（`XCUITest`）—— 立 target **`CatPhoneUITests`**（project.yml 加 target）
  - 只覆盖关键用户流：登录 / 5 tab 切换 / 创建房间 / 解锁盲盒 / 首次 HealthKit 授权
  - 不做按钮级覆盖率 UI 测试

#### 测试文件组织
- 紧跟源文件：`Sources/.../BlindBoxStatus.swift` → `Tests/.../BlindBoxStatusTests.swift`
- 一个被测类型对应一个 `XCTestCase` 子类，命名 `<Type>Tests`
- 测试方法命名：`test_<场景>_<预期>()`，如 `test_decodeStepPayload_handlesMissingField()`

#### Mock / Stub 策略
- 所有**外部依赖 protocol 化**：Networking / HealthKit / WatchConnectivity 入口定 protocol
  - 例：`protocol StepDataSource { func todayStepCount() async throws -> Int }`
  - 实测：`HealthKitStepDataSource`；测试：`InMemoryStepDataSource`
- **禁止**直接子类化系统类型（`HKHealthStore` / `URLSession` / `WCSession`）做 mock
- WS 集成测试：内存 mock 或 `URLProtocol` stub；**不**起 server 进程

#### 测试数据
- JSON fixture 存 `<TestsTarget>/Fixtures/`
- **契约同步**：fixture 从 `docs/api/ws-message-registry.md` / `openapi.yaml` 抄示例；契约变更必须同步更新 fixture

#### 断言风格
- 标准：`XCTAssertEqual` / `XCTAssertTrue` / `await XCTAssertThrowsError(...)`
- 环境不满足用 `XCTSkipIf`，**禁止** `XCTFail("...")` 做流程控制
- 异步测试用 `async func test_...()`，**禁止**用 `XCTestExpectation` 写新测试

#### 超时与性能
- 单测默认 `timeout: 1.0` 够；超过 1 秒即视为有阻塞 bug，必须定位原因再动手改阈值

#### 覆盖率
- **目标**：`CatShared` / `CatCore` 70%+ 行覆盖率
- **软目标**，PR review 看，**不卡 CI**（避免"为测而测"）
- View 层（SwiftUI body）不硬卡覆盖率

#### 测试自包含硬规则（对齐 server §21.7）
- 所有单元 / 集成测试**必须** `xcodebuild test` 一条命令跑绿
- 不得依赖：真实 server / 真机 / Watch 配对 / 特定用户账号 / 外网
- 真机 / 物理联调类测试归 **Epic 9**，不得塞业务 epic 关键路径

### Code Quality & Style Rules

#### 文件 & 目录布局（SSoT 层级）
- **CatShared/Sources/CatShared/**（跨端原子能力，iPhone + Watch 共享）
  - `Models/` — DTO / domain value objects（`struct: Codable`）
  - `Networking/` — HTTP / WS client + envelope + 错误映射
  - `Persistence/` — LocalStore / SwiftData 模型
  - `Resources/` — 共享 asset（颜色 / 间距 / Spine 索引）
  - `UI/` — 跨端可复用 View（注意 watchOS 兼容）
  - `Utilities/` — 时间 / ID / 日志 facade
- **CatShared/Sources/CatCore/**（高层业务编排：use-case / service / coordinator）
- **CatPhone/Features/** —— Feature 模块式组织：
  - `Account/` / `Friends/` / `Inventory/` / `Dressup/` / `Room/` / `StepDisplay/`
  - 每个 Feature 下：`<Feature>View.swift` + `<Feature>Store.swift`（`@Observable`）+ 子 View
- **依赖方向**：Features → CatCore → CatShared；反向禁止

#### 命名约定
- Type: `PascalCase`（`InventoryStore`、`FriendListView`）
- 属性 / 方法 / 枚举 case: `camelCase`
- Protocol 用动名词 / ing 形，不加 `Protocol` 后缀（`StepDataSource`、`RoomJoining`）
- 文件名 = 主类型名（一个文件不放多个不相关的类）
- Feature 目录单数名词（`Account`、`Inventory`；集合语义的 `Friends` 例外）

#### 注释规则（对齐 CLAUDE.md "默认不写注释"）
- 默认不写；命名自解释优先
- 仅在以下 5 种场景写：
  1. 隐藏约束（"调用前必须 @MainActor"）
  2. 微妙不变量（"anchor 必须和 query 同线程释放"）
  3. Workaround 出处（"FB12345678 — iOS 17.2 HealthKit 背景查询崩溃"）
  4. server 契约锚点（"对应 ws-message-registry.md `inventory.unlock`"）
  5. Spine 状态机切换非显然副作用
- **禁止**解释 WHAT；**禁止**引用 PR / Story（会腐烂）

#### 资源管理
- 设计令牌（颜色 / 字体 / 间距）集中 `CatShared/Resources/DesignTokens.swift`
- 图片走 `Assets.xcassets`，按 feature 分组
- **本地化策略（当前阶段）：中文硬编码**
  - 直接写中文字面量：`Text("解锁中...")`
  - **不做**预留 key 化（`String(localized:)` 之类推迟到国际化 epic）
  - 后续国际化会集中扫描重构；为避免技术债放大，禁止掺入英文硬编码 UI 字符串（统一一种语言）

#### Git 文件层级约定
- **忽略** `Cat.xcodeproj/`（XcodeGen 生成产物）—— `.gitignore` 必须有
- **追踪**：`ios/project.yml`（SSoT）、`CatShared/Package.swift` + `Package.resolved`、`Assets.xcassets`、后续的 `Localizable.xcstrings`

#### 日志 facade（**现在立**）
- `CatShared/Sources/CatShared/Utilities/Log.swift` 封装 `OSLog` / `os_log`
- 分类常量：`Log.network` / `Log.ui` / `Log.spine` / `Log.health` / `Log.watch` / `Log.ws`
- 级别：`debug` / `info` / `notice` / `error` / `fault`
- 调用风格对齐 server 结构化日志（**分类 + 级别 + 关键字段**三件套）：
  - 例：`Log.ws.error("envelope decode failed", ["type": type, "id": id])`
- **禁止** `print(...)` 进 main 分支（测试 / debug 临时写过必须删）
- 敏感字段（token / userID 在 release 模式下）禁止明文打；需要时 hash 后记录

#### 依赖管理
- 三方库通过 **SwiftPM** 引入，在 `CatShared/Package.swift` 声明
- 加新依赖必须有理由（对齐 server "不引过度依赖"）：
  - 能用 Apple 框架 / 标准库的**不引**三方
  - PR 描述里解释为什么标准库不够

### Development Workflow Rules

#### Git 分支策略
- 主分支 `main`（保护；禁止直接 push）
- 开发分支命名：
  - `feat/<epic>-<story>-<slug>`（例 `feat/1-1-signinwithapple`）
  - `fix/<issue>-<slug>` / `chore/<slug>` / `docs/<slug>` / `refactor/<slug>` / `test/<slug>`
- 合并：PR + squash；禁止直推 main

#### Commit 消息（对齐 server 仓库样式）
- 首行 ≤ 70 字符；中文或英文均可但**单条消息不混用**
- 前缀：`feat:` / `fix:` / `chore:` / `docs:` / `refactor:` / `test:`
- Body 讲 WHY，不讲 WHAT
- 关联 story：`feat(inventory): 盲盒解锁联动 server 权威值 (Story 3.2)`

#### PR Checklist（作者自填）
- [ ] `bash ios/scripts/build.sh` 本地跑绿（编译 + 测试 + 零 warning）
- [ ] 严格并发无新 warning
- [ ] **语义正确性思考题**（对齐 server §21.8）：PR 描述里回答"跑通但结果错会误导谁？"
- [ ] 触及 server 契约 → `docs/api/*.md` 已先更新，iOS fixture 已同步
- [ ] 日志无敏感字段明文
- [ ] 真机验证项明确标为 Epic 9（不阻塞本 PR）

#### 契约变更流程（跨 server / iOS 仓）
- iOS 发现 server 契约问题 → PR 描述标注 + 到后端仓提 issue/PR → server 更新 `docs/api/*.md` + `internal/dto/*.go` → iOS fixture 同步
- server 主动改契约 → iOS 侧接到通知后跟进 fixture + client 代码
- 未来考虑：契约漂移对比脚本（fixture ↔ server OpenAPI / WS registry）

#### 本地 CI（**不接 GitHub Actions，纪律由本地 git hook 强制**）
- **`ios/scripts/build.sh`** —— 一条命令编译 + 测试
  - 对标 server `bash scripts/build.sh --test`
  - 流程：`xcodegen generate` → `xcodebuild build` → `xcodebuild test` → 零 warning 校验
  - 支持 `--skip-test` / `--ui` / `--scheme <name>` 等 flag
- **`.git/hooks/pre-push`** —— 自动触发 `build.sh`；不绿 reject push
  - hook 本体签入仓库（`ios/scripts/git-hooks/pre-push`），通过 `ios/scripts/install-hooks.sh` 安装到本地 `.git/hooks/`
  - README 首屏告知新 clone 者：**先跑 `bash ios/scripts/install-hooks.sh`**
- 真机 / Watch 配对类检查不在 hook 里跑（归 Epic 9 人工执行）

#### 发版（后续 epic 落地）
- **TestFlight** 内测早立
- 发版前清单：`SKView.showsFPS` / `print()` / debug-only flag 全清
- 发布工具选型（fastlane vs Xcode Cloud）待决

### Critical Don't-Miss Rules

#### 反模式（**禁止**）
- 直接改 `Cat.xcodeproj/` —— 改 `ios/project.yml` 后 `xcodegen generate`
- UI 更新用 `DispatchQueue.main.async` —— 用 `@MainActor` / `await MainActor.run`
- ViewModel 用 `ObservableObject + @Published` —— 用 `@Observable`
- `NavigationView` —— 用 `NavigationStack`
- `onAppear { Task { ... } }` —— 用 `.task { await ... }`
- WS 多连接 —— 单连接共享信封
- HealthKit 启动即请求授权 —— 首次进相关场景再请求
- `transferFile` 走 JSON —— JSON 用 `updateApplicationContext` / `transferUserInfo`
- 当前阶段 `String(localized:)` key 化 —— 中文硬编码，国际化 epic 再做
- `print(...)` 进 main 分支 —— 走 `Log.*` facade
- 直接碰 `skeleton.animationState` —— 通过 `CatAnimationController` 封装
- `try?` 吞错 —— 冒泡或显式展示
- iOS 侧用 HealthKit 数据**直接判定**盲盒能否解锁 —— 必须 server 权威（防作弊）
- Mock 系统类型（`HKHealthStore` / `URLSession` / `WCSession`）的子类 —— 用 protocol + 内存实现

#### 边界条件（**必须处理**）
- **WS 断网重连**：网络恢复自动重连 + 按顺序重放 pending envelope
- **WS 返回 `UNKNOWN_MESSAGE_TYPE`**：server 方言不兼容 → 提示"请更新 app"并屏蔽相关入口
- **`/v1/platform/ws-registry` 启动调用失败（决定：fail-closed）**：
  - 屏蔽主功能入口 + 显示"网络异常，请稍后重试"；禁止用本地缓存绕过
  - 理由：主功能依赖 WS，带病运行会掩盖更大问题（对齐 server §21.3）
  - 可观测信号：`Log.ws.error("registry_fetch_failed", ["error": ...])`
  - 允许"重试"按钮；重试成功前主功能维持屏蔽态
- **HealthKit 授权被拒**：不 crash；UI 引导"去设置开启权限"；步数显示 "—"
- **WatchConnectivity `isReachable = false`**：入队等 reachable 再发，不报错
- **JWT 过期**：refresh token 自动换；失败则清本地 token + 踢回登录页
- **JSON decode 容忍度（决定：宽松）**：
  - `Codable` 默认忽略未知字段，**禁止**加严格 extra-fields 检查
  - server 加新字段老 client 不崩，靠 `/v1/platform/ws-registry` + 契约漂移脚本兜底不兼容检测
- **HealthKit vs server 步数短暂不一致**：UI 显示本地值；盲盒解锁态显示 "解锁中..." 等 server 权威回数

#### 安全规则
- Token 存 **Keychain**（`kSecClassGenericPassword`）；**禁止** UserDefaults
- 设备标识用 `identifierForVendor`；**禁止** IDFA
- Sign in with Apple nonce：请求前随机生成，回调校验，防 replay
- 真机 WS 必须 `wss://` + ATS 合规；`ws://` 明文仅限 debug build 对特定联调域名豁免
- 敏感日志：release 模式禁止 token / email 明文；ID 可打，token 永远 hash
- Deep link（未来 epic）：严格校验 scheme + 参数；禁止直接执行 URL 参数

#### 性能规则
- **主线程 I/O 零容忍**：网络 / 磁盘 / HealthKit 全 `await`
- List 大数据 >200 行：`LazyVStack` / `List` + `id:`；禁止 `ForEach(array)` 暴力展开
- Spine 动画：前台 60fps；后台压 15fps 或暂停
- 图片走 `AsyncImage`（iOS 17 自带缓存）；不自己写缓存层
- HealthKit：按天聚合缓存 15 分钟；禁止秒级重查
- WatchConnectivity `updateApplicationContext`：状态变化去重，避免高频调（系统节流）

#### 架构纪律（对齐 server §21 的 iOS 映射）
- **§21.1 双 gate 漂移守门**：WS 消息类型枚举 `MessageType` ↔ server `WSMessages` ↔ `ws-message-registry.md` 三处同步；契约漂移对比脚本后续 epic 补
- **§21.2 Empty Provider 逐步填实**：骨架先立（Networking / LocalStore / StepDataSource 等），Feature 进来再填；**但空 target / 空 file / 空 protocol 不许长期存在**
- **§21.3 fail-closed vs fail-open**：每处外部依赖失败处理在 Dev Notes / PR 描述记下选择 + 可观测信号
- **§21.6 真机 / Watch 配对 / Spine 真机验证归 Epic 9**：不塞业务 epic 关键路径
- **§21.7 测试自包含**：`xcodebuild test` 一命令绿；不依赖真实 server / 真机 / Watch 配对
- **§21.8 语义正确性思考题**：每 PR 回答"跑通但结果错会误导谁？"

#### 跨端契约 SSoT 裁决顺序
- 顺序：**server `internal/dto/` > `docs/api/` > iOS `CatShared/Models/` fixture**
- iOS 发现不一致：反馈到 server 仓改，不在 iOS 侧临时绕过

---

## Usage Guidelines

### For AI Agents
- **每次 session 开始前读本文件**；这是 iOS 实现面的"开发宪法"
- 所有规则按原文执行；有疑义时**选更严格的那一侧**（例：fail-closed > fail-open；严格并发 > 宽松并发）
- 发现新规律或漏洞：更新本文件同一 PR 里合入
- 与 `/Users/zhuming/fork/catc/CLAUDE.md`（根 CLAUDE.md）冲突时：**以根 CLAUDE.md 为准**（它是跨端工程宪法）

### For Humans
- 保持文件精简，聚焦 agent 容易漏的**隐性规则**；不记录代码里显而易见的东西
- 技术栈变动（iOS 版本升级 / 新增三方库 / deploymentTarget 调整）立即更新
- 每个 epic 结束做一次 retro，看是否有规则过时或新增规律需要记下
- 规则不再是"规律"（被命名规范 / 工具自动化覆盖）就删掉，不要保留噪声

### Critical References
- 根工程宪法：`/Users/zhuming/fork/catc/CLAUDE.md`
- 后端架构 + §21 工程纪律：`/Users/zhuming/fork/catc/docs/backend-architecture-guide.md`
- 跨端契约 SSoT（人类阅读面）：`/Users/zhuming/fork/catc/docs/api/`
  - `openapi.yaml`（HTTP 契约）
  - `ws-message-registry.md`（WS 消息注册表）
  - `integration-mvp-client-guide.md`（联调 MVP 对接）
- server 错误码：`/Users/zhuming/fork/catc/server/docs/error-codes.md`
- iOS 工程 SSoT：`/Users/zhuming/fork/catc/ios/project.yml`
- iOS 模块规划（待立）：`/Users/zhuming/fork/catc/ios/CatPhone/_bmad-output/planning-artifacts/prd.md` + `architecture.md`

Last Updated: 2026-04-19
