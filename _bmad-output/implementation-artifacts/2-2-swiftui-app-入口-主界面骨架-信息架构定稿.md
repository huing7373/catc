# Story 2.2: SwiftUI App 入口 + 主界面骨架 + 信息架构定稿

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iOS 开发,
I want 一个能在模拟器跑、含主界面占位区块（猫 / 步数 / 宝箱 / 进房 / 仓库 / 合成 / 版本号）的 SwiftUI App,
so that 后续 Epic 是"在已存在的占位上填内容"，不再每次重做主界面.

## 故事定位（Epic 2 第一条实装 story；信息架构定稿）

这是 Epic 2 从决策态转入实装态的**第一条代码 story**，等价于 server 端的 Story 1.2：

- Story 2.1（Spike → done）已锁定工具栈与目录方案（XCTest only / `async/await` 主流 / **方案 D：在仓库根新建顶级目录 `iphone/`，`ios/` 整个原封不动**）。本 story 严格按 ADR-0002 §3.3 / §3.4 / §4 落地。
- 本 story 建立 `iphone/` 顶级目录骨架 + `iphone/PetApp.xcodeproj`（XcodeGen 生成）+ `PetApp/App/` 入口 + `PetApp/Features/Home/` 主界面骨架 + 6 大占位区块的 SwiftUI 实装 + 单元测试 + UI 测试。
- **不涉及**：导航 Sheet（→ 2.3）/ APIClient（→ 2.4）/ ping & version 真实调用（→ 2.5；本 story 版本号区域 hardcode `v0.0.0 · ----` 占位）/ 错误 UI（→ 2.6）/ 测试基础设施完整搭建（→ 2.7；**本 story 写好"够本 story 测试跑通"的最小 Test target，2.7 再补 MockBase / CI 文档化**）/ Dev 重置 Keychain（→ 2.8）/ LaunchingView（→ 2.9）/ README（→ 2.10）。
- 本 story 还**首次落地** `iphone/scripts/install-hooks.sh` + `iphone/scripts/git-hooks/`（按 ADR-0002 §3.3 "方案 D 分阶段生效" 阶段 2 要求；`iphone/scripts/build.sh` 留给 Story 2.7 落地）。

**范围红线**：

- **不动 `ios/`**：本 story **绝对不**修改 `ios/Cat.xcodeproj` / `ios/project.yml` / `ios/Cat*` / `ios/scripts/` 任何文件（ADR-0002 §3.3 + AC6 强约束）。`git status` 在最终 commit 前必须确认 `ios/` 下无任何 modified / staged 文件。
- **不写 APIClient / Networking**：`PetApp/Core/Networking/` 目录本 story 不创建任何 .swift 文件（避免空目录污染或提前实装）。
- **不引入 Combine 响应式框架**：按 iOS 架构 §18.1 + ADR-0002 §3.2，`async/await` 为主，`@Published` 用最小限度（仅在 `HomeViewModel` 暴露 UI 状态）。
- **不引入第三方 mock 库**：按 ADR-0002 §3.1，本 story 单测中如需 mock 任何 protocol（实际本 story 只测纯渲染，可能不涉及 mock），手写 `class MockXxx: XxxProtocol`。
- **不创建 `iphone/build/`**：`.gitignore` 加上即可，目录由 build.sh（Story 2.7）首次跑时自动生成。

## Acceptance Criteria

**AC1 — `iphone/` 顶级目录骨架 + XcodeGen 工程定义建立（对应 ADR-0002 §3.3 / §4）**

`iphone/` 顶级目录在仓库根新建，**与 `server/` / `ios/` 平级**。本 story 必须建立的目录与文件：

```text
iphone/
├─ project.yml                    # XcodeGen 工程定义，AC2 详述字段
├─ PetApp.xcodeproj/              # 由 `xcodegen generate` 生成（提交到 git；与 server 端 build artifact 不同，工程文件本身入库）
├─ PetApp/
│  ├─ App/
│  │  ├─ PetAppApp.swift          # @main，加载 RootView
│  │  └─ RootView.swift           # 当前直接渲染 HomeView（Story 2.9 改为路由 LaunchingView / HomeView / RetryView）
│  ├─ Core/
│  │  └─ DesignSystem/
│  │     └─ Components/           # 留空目录或加一个 .gitkeep；本 story 不实装组件
│  ├─ Shared/
│  │  └─ Constants/
│  │     └─ AccessibilityID.swift # 集中定义本 story 6 大占位区块的 a11y identifier 常量
│  ├─ Features/
│  │  └─ Home/
│  │     ├─ Views/
│  │     │  └─ HomeView.swift     # 主界面骨架，6 大占位区块
│  │     └─ ViewModels/
│  │        └─ HomeViewModel.swift # 占位 VM：当前只暴露 nickname / appVersion / serverInfo 三个 @Published（hardcode）；按钮 onTap 闭包
│  └─ Resources/
│     ├─ Assets.xcassets/         # 至少含 AppIcon（占位）+ AccentColor
│     └─ Info.plist               # XcodeGen 自动生成 vs 手写：本 story 让 XcodeGen `info.path` 自动生成，避免手维护
├─ PetAppTests/
│  ├─ Features/
│  │  └─ Home/
│  │     ├─ HomeViewTests.swift
│  │     └─ HomeViewModelTests.swift
├─ PetAppUITests/
│  └─ HomeUITests.swift
└─ scripts/
   ├─ install-hooks.sh            # 新写，参考 `ios/scripts/install-hooks.sh` 良好实践但路径全部从 0 写
   └─ git-hooks/
      └─ pre-commit               # 跑 swift-format（Story 2.2 落地版可仅占位 `exit 0`，由 dev 视情况扩；ADR-0002 §6 swift-format 验证 TODO 在此 hook 内实装）
```

**禁止**在本 story 建立：

- `iphone/scripts/build.sh`（→ Story 2.7）
- `iphone/PetApp/Core/Networking/` 下任何 .swift（→ Story 2.4）
- `iphone/PetApp/Features/{Auth,Pet,Steps,Chest,Cosmetics,Compose,Room,Emoji}/` 实体子目录（避免空目录；用到时再建，与 server 端 1.2 "禁止提前建 domain/repo/service 子目录" 同样思路）
- `iphone/README.md`（→ Story 2.10）
- `iphone/build/`（不建实体目录，仅 `.gitignore` 加规则）

**AC2 — `iphone/project.yml` 字段对齐 ADR-0002 §4 版本锁定清单**

`iphone/project.yml` **新建**（`ios/project.yml` 全程不动），最小内容如下（dev 可根据 XcodeGen 2.45.3 实际语法调整，但下列字段值必须严格匹配 ADR-0002 §4）：

```yaml
name: PetApp

options:
  bundleIdPrefix: com.zhuming.pet
  deploymentTarget:
    iOS: "17.0"
  xcodeVersion: "26.4"
  createIntermediateGroups: true
  generateEmptyDirectories: true

settings:
  base:
    SWIFT_VERSION: "5.9"
    DEVELOPMENT_TEAM: ""              # 留空；dev 在本机 Xcode 内手动配 signing（ADR-0002 §1.1 单开发者重启阶段策略）

targets:
  PetApp:
    type: application
    platform: iOS
    sources:
      - PetApp
    info:
      path: PetApp/Resources/Info.plist
      properties:
        CFBundleDisplayName: PetApp
        UILaunchScreen: {}
    settings:
      base:
        PRODUCT_BUNDLE_IDENTIFIER: com.zhuming.pet.app
        TARGETED_DEVICE_FAMILY: "1"   # iPhone only（不含 iPad；与方案 D 锁定 iPhone-only 一致）

  PetAppTests:
    type: bundle.unit-test
    platform: iOS
    sources:
      - PetAppTests
    dependencies:
      - target: PetApp
    settings:
      base:
        PRODUCT_BUNDLE_IDENTIFIER: com.zhuming.pet.app.tests

  PetAppUITests:
    type: bundle.ui-testing
    platform: iOS
    sources:
      - PetAppUITests
    dependencies:
      - target: PetApp
    settings:
      base:
        PRODUCT_BUNDLE_IDENTIFIER: com.zhuming.pet.app.uitests
```

**关键约束**：

- `bundleIdPrefix`：`com.zhuming.pet`（与旧 `com.zhuming.cat` 严格隔离；ADR-0002 §4）
- `deploymentTarget.iOS`：`"17.0"`（向下兼容到 iOS 17，ADR-0002 §4）
- `xcodeVersion`：`"26.4"`（实测当前机器，ADR-0002 §4 `xcode_version_tested`）
- **不引用** `ios/CatShared` Swift Package（ADR-0002 §3.3 方案 D 不复用旧产物；`PetApp/Core/` + `PetApp/Shared/` 自己写）
- **不**包含 watchOS target（ADR-0002 §3.3 方案 D `iphone/` 仅 iPhone-only）
- 完成后跑 `xcodegen generate` 在 `iphone/` 目录生成 `PetApp.xcodeproj`，并把 `.xcodeproj` 整个**提交到 git**（与 0001-test-stack.md / `ios/` 旧实践一致：工程文件入库）

**AC3 — 主界面 6 大占位区块 + accessibility identifier 严格命名**

`HomeView.swift` 渲染时必须包含**全部 6 个**占位区块，每个区块必须有 SwiftUI `.accessibilityIdentifier(...)` 修饰符（识别字符串集中放在 `Shared/Constants/AccessibilityID.swift` 常量中，方便测试侧 import）：

| 区块 | 位置 | 内容 | accessibility identifier 常量名 / 字符串 |
|---|---|---|---|
| ① 用户昵称 + 头像位 | 顶部 | `HStack { Text("用户1001"); Circle().fill(.gray).frame(width: 32, height: 32) }` | `AccessibilityID.Home.userInfo` = `"home_userInfo"` |
| ② 猫展示区 | 中间（屏幕中心区） | `Rectangle().fill(.gray).frame(width: 200, height: 200)` | `AccessibilityID.Home.petArea` = `"home_petArea"` |
| ③ 步数显示位 | 中间下方（猫展示区下方） | `Text("0 步")` | `AccessibilityID.Home.stepBalance` = `"home_stepBalance"` |
| ④ 宝箱位 | 中间右侧（猫展示区右侧） | `Rectangle().fill(.brown).frame(width: 64, height: 64)` | `AccessibilityID.Home.chestArea` = `"home_chestArea"` |
| ⑤ 三个主按钮 | 底部 | `HStack { Button("进入房间") { ... }; Button("仓库") { ... }; Button("合成") { ... } }`，三个按钮各自 a11y id | `AccessibilityID.Home.btnRoom` = `"home_btnRoom"` / `AccessibilityID.Home.btnInventory` = `"home_btnInventory"` / `AccessibilityID.Home.btnCompose` = `"home_btnCompose"` |
| ⑥ 版本号小字 | 角落（右下） | `Text("v0.0.0 · ----")` (hardcode 占位，Story 2.5 改为真实) | `AccessibilityID.Home.versionLabel` = `"home_versionLabel"` |

**关键约束**：

- 6 大区块的 a11y identifier 字符串**必须**集中定义在 `AccessibilityID.swift`（不允许 inline string，避免测试与生产代码字符串漂移）
- 区块内子元素（如三个按钮各自 a11y id）也走集中定义
- `HomeViewModel` 暴露三个 hardcode `@Published`：`nickname: String = "用户1001"` / `appVersion: String = "v0.0.0"` / `serverInfo: String = "----"`，HomeView 绑定显示（Story 2.5 把 hardcode 替换为 PingUseCase / FetchVersionUseCase 驱动；本 story 仅打底）
- 三个主按钮的 `onTap` 当前是**空函数**（`{ }`）—— 但必须真正注册 closure（不能不写 `action:`），下方 AC4 单测 case#3 验证 closure 被注册

**AC4 — 单元测试覆盖（≥3 case，对应 epics.md Story 2.2 AC）**

`PetAppTests/Features/Home/HomeViewTests.swift` + `HomeViewModelTests.swift` 至少包含以下 3 类 case（实际可拆多个测试方法）：

1. **happy（HomeViewTests）：HomeView body 渲染时包含全部 6 个占位区块的 accessibility identifier**
   - 用 SwiftUI inspection 思路（无 UITest，纯单测）：把 HomeView 的 body 渲染为 `UIHostingController` 的 view，遍历 view tree 查找包含 6 个 a11y identifier 的 subview。
   - **简化方案**（如 view tree 遍历过于繁琐）：使用 `@testable import PetApp` 直接测 `AccessibilityID.Home.*` 常量值不为空 + 测 `HomeViewModel` 暴露三个 closure 属性（room / inventory / compose）—— a11y identifier 的真实出现验证由 AC5 UI 测试覆盖。
   - 至少断言：6 个 a11y identifier 常量值不为空，且 6 个值两两不相等。
2. **edge（HomeViewTests，可选简化为快照测试）：不同尺寸（iPhone SE / iPhone 15 Pro Max）下布局不破**
   - 用 `UIHostingController(rootView: HomeView()).view.bounds = CGRect(...)` 在两个不同 size class 下渲染，断言不抛异常 + 关键 subview 的 frame.width > 0（即没有完全 collapse）。
   - **简化方案**：跳过严格断言，仅在两种 size 下调用 `_ = controller.view`，确保不 crash（防 layout-time 崩溃）。
3. **happy（HomeViewModelTests）：点击三个主按钮触发各自的 `onTap` action（暂时为空函数，验证回调注册）**
   - HomeViewModel 暴露 `onRoomTap: () -> Void`、`onInventoryTap: () -> Void`、`onComposeTap: () -> Void` 三个 closure 属性（init 默认空函数）。
   - 测试中给 VM 注入三个带 side effect 的 closure（如 `var roomTapped = false; vm.onRoomTap = { roomTapped = true }`），调用 `vm.onRoomTap()` 后断言 `roomTapped == true`。
   - 三个按钮各跑一遍，验证 closure 注册链路。

**测试基础设施约束**（与 Story 2.7 衔接）：

- 本 story 在 `PetAppTests/` target 写测试时，**仅依赖 stdlib（XCTest + @testable import PetApp）**，不引入任何 helper 文件（Story 2.7 才落地 `Helpers/MockBase.swift`）。
- 测试方法签名按 ADR-0002 §3.2 选定：纯渲染测试用 `func testFoo() throws { ... }`；如需 await 异步（本 story 应不需要），用 `func testFoo() async throws { ... }`。

**AC5 — UI 测试覆盖：UITest 启动模拟器 → 验证主界面 6 个区块的 accessibility identifier 都可定位**

`PetAppUITests/HomeUITests.swift` 至少 1 条测试方法 `testHomeViewShowsAllSixPlaceholders()`：

- 启动 App：`let app = XCUIApplication(); app.launch()`
- 6 次 `XCTAssertTrue(app.<elementType>[AccessibilityID.Home.<id>].waitForExistence(timeout: 3))` 验证 6 个 a11y id 在 view hierarchy 中可定位
  - userInfo / petArea / chestArea / stepBalance / versionLabel：通常用 `app.otherElements[...]` 或 `app.staticTexts[...]`，dev 实装时按 SwiftUI 实际渲染类型选
  - btnRoom / btnInventory / btnCompose：用 `app.buttons[...]`
- **不需要**点击行为验证（按钮 onTap 当前是空函数；点击行为验证留 Story 2.3 弹 Sheet 时做）

**AC6 — App 启动验证（手动 + 测试）**

- **手动验证**：dev 在本机 Xcode 26.4.1 + iOS 26.4 simulator 上 `cmd+R` 启动 PetApp，肉眼确认主界面 6 个占位区块都显示
- **AccessibilityID 字符串风格**：所有 a11y identifier 使用 `<feature>_<element>` 命名（小驼峰）；本 story 6 个 + 后续 stories 扩展时遵循同样风格

**AC7 — `iphone/scripts/install-hooks.sh` + git hooks 落地（ADR-0002 §3.3 方案 D 阶段 2 要求）**

`iphone/scripts/install-hooks.sh` 新写（参考 `ios/scripts/install-hooks.sh` 良好实践如 `set -euo pipefail` / `require_tool` 但路径全部从 0 写），功能：

- 把 `iphone/scripts/git-hooks/pre-commit` 拷贝到 `.git/hooks/pre-commit`
- 验证 `swift-format` 已安装（`brew install swift-format`，**unversioned**；ADR-0002 §6 TODO）+ 版本 `swift-format --version` 输出 startsWith `602.`，否则报错并退出非 0
- 输出明确成功 / 失败信息

`iphone/scripts/git-hooks/pre-commit` 本 story 落地版可以是**最小可用骨架**：

```bash
#!/usr/bin/env bash
set -euo pipefail
# Story 2.2 阶段：仅占位（防止后续 commit 时 hook 文件不存在报错）；
# 真实 swift-format 调用 / lint 规则在后续 story 视需要扩展。
exit 0
```

**理由**：本 story 关键交付是 SwiftUI 工程骨架；hook 实质内容（如 swift-format 全工程跑、SwiftLint 规则）属于 tech debt，可在 Story 2.7 / Story 3.3 单独评估。

**关键约束**：

- 本 story **不**触碰 `ios/scripts/install-hooks.sh`（方案 D：`ios/` 整个不动）
- 如 dev 此前已 run 旧 `ios/scripts/install-hooks.sh`，本 story 在 Completion Notes 提示 dev 手工卸载（删 `.git/hooks/pre-commit`），再 run 新 `iphone/scripts/install-hooks.sh`

**AC8 — `.gitignore` 同步加 `iphone/build/` 行（ADR-0002 §3.4 round-3 P2 fix）**

仓库根 `.gitignore` 文件**追加**一行：

```text
iphone/build/
```

**理由**：`iphone/scripts/build.sh`（Story 2.7 落地）会向 `iphone/build/` 输出 `test-results.xcresult` / `DerivedData/` / coverage exports；这些 artifact 不应入库。本 story 提前加 `.gitignore` 行，避免 Story 2.7 落地时多一个 commit。

**AC9 — `git status` 最终自检（防 scope creep / 防误改 `ios/`）**

最终 commit 前跑 `git status`，确认仅以下文件被 created / modified（必须，不允许其它）：

- `iphone/` 下所有新建文件（含 `iphone/PetApp.xcodeproj/`、`iphone/PetApp/...`、`iphone/PetAppTests/...`、`iphone/PetAppUITests/...`、`iphone/scripts/...`、`iphone/project.yml`）
- `.gitignore`（追加 `iphone/build/`）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（dev-story workflow 推 status：ready-for-dev → in-progress → review）
- `_bmad-output/implementation-artifacts/2-2-swiftui-app-入口-主界面骨架-信息架构定稿.md`（dev-story workflow 勾选 Tasks/Subtasks + 填 Dev Agent Record + Status: ready-for-dev → review）
- 可选：`.claude/settings.local.json`（如 dev 临时加 bash 白名单；非必须）

**绝对禁止**：

- `ios/` 下任何文件被 modified / staged
- `server/` 下任何文件被 modified / staged
- `CLAUDE.md` / `docs/` 下任何文件被 modified（非 story scope）

## Tasks / Subtasks

- [x] **T1**：建立 `iphone/` 顶级目录骨架 + XcodeGen 工程定义（AC1 / AC2）
  - [x] T1.1 在仓库根新建 `iphone/` 目录（与 `server/` / `ios/` 平级）
  - [x] T1.2 新建 `iphone/project.yml`，按 AC2 内容填充（关键字段：name=PetApp / bundleIdPrefix=com.zhuming.pet / deploymentTarget.iOS=17.0 / xcodeVersion=26.4 / SWIFT_VERSION=5.9 / 三个 target：PetApp / PetAppTests / PetAppUITests）
  - [x] T1.3 新建 `iphone/PetApp/Resources/Info.plist`（XcodeGen 路径模式生成，dev 可手写最小骨架或让 XcodeGen 默认生成；CFBundleDisplayName=PetApp）
  - [x] T1.4 新建 `iphone/PetApp/Resources/Assets.xcassets/`（含 AppIcon 占位 + AccentColor，可用 Xcode 默认）
  - [x] T1.5 跑 `cd iphone && xcodegen generate`，验证 `iphone/PetApp.xcodeproj/` 生成成功，无错误日志
  - [x] T1.6 在 Xcode 26.4.1 中 open `iphone/PetApp.xcodeproj`，验证三个 target（PetApp / PetAppTests / PetAppUITests）正常显示
        — 等价自动验证：`xcodebuild build -scheme PetApp ...` 成功 + `xcodebuild test ...` 三个 target 全跑通（PetApp/PetAppTests/PetAppUITests），结构正确

- [x] **T2**：实装 App 入口与 RootView（AC1 / AC3）
  - [x] T2.1 新建 `iphone/PetApp/App/PetAppApp.swift`：`@main struct PetAppApp: App` + `body: some Scene { WindowGroup { RootView() } }`
  - [x] T2.2 新建 `iphone/PetApp/App/RootView.swift`：`struct RootView: View { var body: some View { HomeView(viewModel: HomeViewModel()) } }`（Story 2.9 改为路由 LaunchingView/HomeView/RetryView）

- [x] **T3**：实装 Shared/Constants/AccessibilityID（AC3）
  - [x] T3.1 新建 `iphone/PetApp/Shared/Constants/AccessibilityID.swift`：定义 `enum AccessibilityID { enum Home { static let userInfo = "home_userInfo"; static let petArea = "home_petArea"; static let stepBalance = "home_stepBalance"; static let chestArea = "home_chestArea"; static let btnRoom = "home_btnRoom"; static let btnInventory = "home_btnInventory"; static let btnCompose = "home_btnCompose"; static let versionLabel = "home_versionLabel" } }`

- [x] **T4**：实装 HomeViewModel（AC3 / AC4 case#3）
  - [x] T4.1 新建 `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`：`@MainActor final class HomeViewModel: ObservableObject` + `@Published var nickname: String = "用户1001"` + `@Published var appVersion: String = "0.0.0"` + `@Published var serverInfo: String = "----"`（采用 Dev Note T5.5 推荐方案：appVersion 不带 v 前缀，由 View 拼接）
  - [x] T4.2 暴露三个闭包属性：`var onRoomTap: () -> Void = {}` / `var onInventoryTap: () -> Void = {}` / `var onComposeTap: () -> Void = {}`（init 默认空函数；Story 2.3 替换为 coordinator.present(...)）

- [x] **T5**：实装 HomeView（AC3）
  - [x] T5.1 新建 `iphone/PetApp/Features/Home/Views/HomeView.swift`：`struct HomeView: View { @ObservedObject var viewModel: HomeViewModel; var body: some View { ... } }`（按 Dev Note #2 选定 RootView 用 `@StateObject` 持有 + HomeView 用 `@ObservedObject`，详见 Completion Notes）
  - [x] T5.2 顶部 HStack：用户昵称（`Text(viewModel.nickname)`）+ 头像 Circle，整组 `.accessibilityIdentifier(AccessibilityID.Home.userInfo)` + `.accessibilityElement(children: .ignore)` 让 a11y 把这块当一个整体（避免 UITest hit 不到顶层 id）
  - [x] T5.3 中间垂直组合：猫展示区 Rectangle 200x200 灰色（`.accessibilityIdentifier(AccessibilityID.Home.petArea)`）+ 下方 步数 Text "0 步"（`.accessibilityIdentifier(AccessibilityID.Home.stepBalance)`）+ 右侧 宝箱 Rectangle 64x64 棕色（`.accessibilityIdentifier(AccessibilityID.Home.chestArea)`）；可用 ZStack / VStack + HStack 嵌套实现"中间 + 中间右侧 + 中间下方"布局
  - [x] T5.4 底部 HStack：三个 Button —— `Button("进入房间") { viewModel.onRoomTap() }.accessibilityIdentifier(AccessibilityID.Home.btnRoom)` / 同样 inventory / 同样 compose
  - [x] T5.5 右下角 Text "v\(viewModel.appVersion) · \(viewModel.serverInfo)"（`.accessibilityIdentifier(AccessibilityID.Home.versionLabel)`）—— 采用 Dev Note 推荐方案：VM 存 `appVersion = "0.0.0"`（不带 v）；View 拼 "v\(appVersion) · \(serverInfo)" → "v0.0.0 · ----"
  - [x] T5.6 整体 ZStack 安排：内层 VStack 主体内容（顶 / 中部 / 按钮底）；外层 ZStack 叠加右下角 versionLabel；不要求像素精准

- [x] **T6**：实装单元测试（AC4）
  - [x] T6.1 新建 `iphone/PetAppTests/Features/Home/HomeViewModelTests.swift`：5 个 case（≥ 3）验证 `onRoomTap` / `onInventoryTap` / `onComposeTap` 三条闭包注册链路 + 默认空函数无 crash + 默认 hardcode 值匹配 story spec
  - [x] T6.2 新建 `iphone/PetAppTests/Features/Home/HomeViewTests.swift`：3 个 a11y 常量类 case（非空 / 两两不等 / 命名前缀 home_）+ 2 个不同 size class 渲染不 crash case，共 5 case
  - [x] T6.3 跑 `xcodebuild test -project iphone/PetApp.xcodeproj -scheme PetApp -destination 'platform=iOS Simulator,name=iPhone 17,OS=latest'` 验证 PetAppTests 全部 10 case 通过

- [x] **T7**：实装 UI 测试（AC5）
  - [x] T7.1 新建 `iphone/PetAppUITests/HomeUITests.swift`：`testHomeViewShowsAllSixPlaceholders()` 启动 App + 8 次 `waitForExistence(timeout: 5)` 断言 8 个 a11y id（6 大区块 + 三个按钮各自）
  - [x] T7.2 跑 PetAppUITests target，UI 测试通过（11.7s）

- [x] **T8**：落地 `iphone/scripts/install-hooks.sh` + git hooks（AC7）
  - [x] T8.1 新建 `iphone/scripts/install-hooks.sh`（参考 `ios/scripts/install-hooks.sh` 良好实践但路径全部从 0 写；含 `set -euo pipefail` + `swift-format --version` 验证 startsWith `602.`，否则 fail-closed）
  - [x] T8.2 新建 `iphone/scripts/git-hooks/pre-commit`（最小骨架：`#!/usr/bin/env bash; set -euo pipefail; exit 0`；真实 lint 调用留 tech debt）
  - [x] T8.3 给 `iphone/scripts/install-hooks.sh` 与 `iphone/scripts/git-hooks/pre-commit` 加可执行权限（`chmod +x`）
  - [x] T8.4 Completion Notes 已提示 dev 过渡期手工卸载旧 hook（见下方）

- [x] **T9**：`.gitignore` 同步加 `iphone/build/`（AC8）
  - [x] T9.1 在仓库根 `.gitignore` 末尾追加：`iphone/build/`（同 server 端 `build/` 同等 ignore；ADR-0002 §3.4 round-3 P2 fix）

- [x] **T10**：自检并提交（AC6 / AC9）
  - [x] T10.1 通过 UITest `testHomeViewShowsAllSixPlaceholders` 自动验证 6 大区块（含三个按钮 + 版本号）共 8 个 a11y id 全部在真实 simulator 上可定位 —— 等价于"`cmd+R` 肉眼确认"的自动化版本（更可靠 + 可回归）；详见 Completion Notes
  - [x] T10.2 跑 `xcodebuild test -project iphone/PetApp.xcodeproj -scheme PetApp -destination 'platform=iOS Simulator,name=iPhone 17,OS=latest'` 验证单测 + UITest 全部通过：PetAppTests 10 case / PetAppUITests 1 case，0 失败
  - [x] T10.3 跑 `git status`，确认改动清单严格符合 AC9：`iphone/` 全新增 + `.gitignore` 修改 + `_bmad-output/.../sprint-status.yaml` 修改 + 当前 story 文件 untracked + `.claude/settings.local.json`（白名单，AC9 列为可选）；**`ios/` / `server/` / `CLAUDE.md` / `docs/` 全部零改动**
  - [x] T10.4 dev-story workflow：本步骤已勾选所有 Tasks/Subtasks + 填写 Dev Agent Record + Status: ready-for-dev → review
  - [ ] T10.5 commit message 由 story-done 阶段使用（dev-story 阶段不 commit）—— 建议：`feat(iphone): Story 2.2 - PetApp 入口 + HomeView 6 大占位区块 + 测试基础设施 minimum + iphone/scripts/install-hooks.sh`

## Dev Notes

### 项目关键约束（必读，勿绕过）

1. **iPhone 工程目录由 ADR-0002 锁定**（`_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md` §3.3 / §4）：
   - 在**新建顶级目录 `iphone/`** 下从零建工程
   - **`ios/` 整个原封不动**（`Cat.xcodeproj` / `project.yml` / `CatPhone/` / `CatShared/` / `CatWatch*/` / `scripts/` 全部不删不改不移动）
   - **不引用** `ios/CatShared` Swift Package（zero-coupling，自己写 `Core/` + `Shared/`）
2. **iOS Mock 框架**（ADR-0002 §3.1）：XCTest only（手写 Mock）；本 story 实际可能不涉及 mock（纯渲染测试），但写 mock 时遵循"手写 `class MockXxx: XxxProtocol`"原则
3. **异步测试方案**（ADR-0002 §3.2）：`async/await` 主流；本 story 测试基本是同步渲染验证，不涉及 await
4. **CLAUDE.md "Repo Separation"** 已同步更新为"`iphone/` (iPhone App，新方向；Story 2.2 起在此目录落地)"，本 story 是这个声明的首次落地
5. **本 story 是 Epic 2 第一条实装 story**：等价于 server 端的 Story 1.2（建工程骨架 + 第一个 endpoint）；后续 2.3 / 2.4 / 2.5 / 2.6 / 2.7 在本 story 骨架之上叠加
6. **节点 1 整体未闭合**：Epic 1（server）已 done，Epic 2（iOS）刚启动；本 story 完成后 Epic 2 才有第一个可跑的 SwiftUI App

### iOS 架构设计 §4 目标目录映射

本 story 不一次到位建全部目录（避免空目录污染），仅建本 story 用到的子目录：

| 架构 §4 目录 | 本 story 是否建 | 本 story 落地内容 |
|---|---|---|
| `PetApp/App/` | ✅ | `PetAppApp.swift` + `RootView.swift` |
| `PetApp/Core/DesignSystem/Components/` | ✅（空目录或 .gitkeep） | 不实装具体组件；后续 story 用到时实装 |
| `PetApp/Core/Networking/` | ❌ | → Story 2.4 |
| `PetApp/Core/Realtime/` | ❌ | → Epic 12 |
| `PetApp/Core/Storage/` | ❌ | → Story 5.1 |
| `PetApp/Core/Motion/` | ❌ | → Epic 8 |
| `PetApp/Core/Health/` | ❌ | → Epic 8 |
| `PetApp/Shared/Constants/` | ✅ | `AccessibilityID.swift` |
| `PetApp/Shared/Models/` | ❌ | → Story 2.4 / Epic 4+ |
| `PetApp/Features/Home/` | ✅ | `HomeView.swift` + `HomeViewModel.swift` |
| `PetApp/Features/{Auth,Pet,Steps,...}/` | ❌ | → 各自对应 epic |
| `PetApp/Resources/` | ✅ | `Info.plist` + `Assets.xcassets` |
| `PetAppTests/` | ✅ | `Features/Home/HomeViewTests.swift` + `HomeViewModelTests.swift` |
| `PetAppUITests/` | ✅ | `HomeUITests.swift` |

### 测试基础设施"最小可跑"vs Story 2.7"完整搭建"的边界

本 story 必须让 PetAppTests / PetAppUITests **能跑通本 story 自己的测试**，但**不需要**：

- 写 `PetAppTests/Helpers/MockBase.swift`（→ Story 2.7）
- 写 `iphone/scripts/build.sh`（→ Story 2.7）
- 文档化 CI 跑法（→ Story 2.7）
- 写 "AR27 业务相关 mock 单元测试"（→ Story 2.7，给后续 story 当模板示范）

本 story 测试只需 **3 个 testFile + 共 ≥ 4 个 test method**（HomeViewModelTests 3 case + HomeViewTests ≥ 1 case + HomeUITests 1 case），跑通即可，**不要求覆盖率指标**（覆盖率约束在 Story 2.7 落地）。

### epics.md Story 2.2 vs 本文件 AC 对照（AR 完整性核对）

| epics.md 原文 AC | 本文件 AC# |
|---|---|
| 模拟器可启动 App + 6 大占位区块 | AC1 / AC3 / AC6 |
| App 名称 / Bundle ID 按 0002-ios-stack.md 决策 | AC2 |
| 单元测试覆盖 ≥3 case（happy 渲染含 6 a11y id / edge 多尺寸 / happy 按钮回调注册） | AC4 |
| 集成测试覆盖：UITest 启动模拟器 + 验证 6 区块 a11y id 都可定位 | AC5 |

**本文件超出 epics.md 原 AC 的部分（合理扩展）**：

- AC2 把 ADR-0002 §4 版本锁定值搬进 `project.yml` 字段约束，避免 dev 凭印象 hardcode
- AC7（install-hooks.sh + git hooks）：ADR-0002 §3.3 "方案 D 分阶段生效" 阶段 2 要求；不在 epics.md 但在 ADR-0002 §6 TODO，本 story 是首次有机会落地的节点
- AC8（`.gitignore` 加 `iphone/build/`）：ADR-0002 §3.4 round-3 P2 fix 要求；不在 epics.md 但在 ADR-0002 §6 TODO
- AC9（`git status` 自检）：防 scope creep / 防误改 `ios/` 的硬约束，与 Story 2.1 AC6 "不动 ios/" 一脉相承

### 与 Story 2.1（已 done）的衔接

- Story 2.1 是 Spike，唯一交付物是 ADR-0002 决策文档；**未**触动任何代码 / 工程文件
- 本 story 是 ADR-0002 §3.3 方案 D 的**第一次代码落地**（建 `iphone/` 目录 + 写工程定义 + 跑 xcodegen + 写源码）
- ADR-0002 §6 TODO 中标 "Story 2.2" 的项目，本 story 必须满足：
  - ✅ 按 §3.3 方案 D 在仓库根新建 `iphone/` 目录 + 写 `iphone/project.yml`（参考 §4 Implication）+ `xcodegen generate` + 写 `iphone/PetApp/App/PetAppApp.swift` 等首批源码；**`ios/` 全程零改动**
  - ✅ 按 §3.3 "立即生效依赖项 §2"，新写 `iphone/scripts/install-hooks.sh` + `iphone/scripts/git-hooks/`

### 与 Story 1.2（server 端已 done）的对照

| 维度 | 1.2（server 端） | 2.2（iOS 端，本 story） |
|---|---|---|
| 故事定位 | Epic 1 第一条实装 story（Spike 后） | Epic 2 第一条实装 story（Spike 后） |
| 工程入口 | `server/cmd/server/main.go` | `iphone/PetApp/App/PetAppApp.swift` |
| 工程定义 | `server/go.mod`（`go 1.22`） | `iphone/project.yml` + `iphone/PetApp.xcodeproj`（XcodeGen 生成） |
| 配置文件 | `server/configs/local.yaml` | `iphone/PetApp/Resources/Info.plist`（XcodeGen 自动生成） |
| 第一个 endpoint | `GET /ping` 返回 `{code:0, data:{...}}` | `HomeView` 渲染 6 大占位区块 |
| 测试基础设施 | 仅 `cd server && go test ./...` 跑通 | 仅 `xcodebuild test ...` 跑通 |
| 测试基础设施完整搭建 | → Story 1.5 | → Story 2.7 |
| 范围红线 | 不建 domain/repo/service 子目录占位 | 不建 Features/{Auth,Pet,...} 实体子目录占位 |

### 关键技术细节

**1. SwiftUI a11y identifier 在测试中的定位**

iOS 17+ SwiftUI 的 `.accessibilityIdentifier(...)` 会绑定到底层 UIKit view 的 `accessibilityIdentifier` 属性，但 SwiftUI 的渲染会把多层 modifier 折叠/重组，导致：

- 单测中通过 `UIHostingController` 取 view tree 时，a11y identifier 可能在叶子 subview 而非顶层
- UITest 中通过 `app.staticTexts["..."]` / `app.buttons["..."]` / `app.otherElements["..."]` 定位，元素类型取决于 SwiftUI 渲染产物（如 Text → staticText, Button → button, Rectangle → otherElement）

dev 实装时建议：

- 单测优先验证**常量字符串**（AC4 case#1 简化方案），避免 view tree 遍历的脆弱性
- UITest 用 `XCUIElementQuery` 的多种 element type 都试一遍：`app.descendants(matching: .any)[id]`（最宽容；性能可接受）
- 如某个 a11y id 在 UITest 中始终定位不到，dev 可在 HomeView 中给该区块加 `.accessibilityElement(children: .combine)` 让该区块被识别为单一 a11y element

**2. SwiftUI `@StateObject` vs `@ObservedObject` 的选择**

`HomeView` 持有 `HomeViewModel`：

- 如果 `HomeView` 自己创建 VM 实例 → 用 `@StateObject`（VM 生命周期跟 View 一致；推荐 RootView 中 `HomeView()`）
- 如果 VM 是从外部注入（如 Story 18.2 依赖注入容器） → 用 `@ObservedObject`（VM 由外部管理）

本 story 用 `@StateObject`：`RootView` 中 `HomeView(viewModel: HomeViewModel())` 不行（`@StateObject` 只能在 init 时初始化一次，外部传入会导致 multiple init）。改为 `RootView` 中 `HomeView()`，`HomeView` 自己 `@StateObject var viewModel = HomeViewModel()`。或者更稳的做法：`RootView` 用 `HomeView(viewModel: HomeViewModel())` + `HomeView` 用 `@ObservedObject var viewModel: HomeViewModel`，本 story 选这个更清晰。

dev 实装时挑一个实现 + 在 Completion Notes 记录选择。

**3. `xcodegen generate` 的幂等性**

`xcodegen generate` 是幂等的：每次跑都会按 `project.yml` 重新生成 `.xcodeproj`，覆盖现有内容。本 story 跑一次即可；后续 story 如改了 `project.yml`（如加新 target），再跑一次。`.xcodeproj` 改动需要 commit。

**4. `iphone/` 在 xcodebuild 命令中的相对路径**

`xcodebuild` 命令模板（ADR-0002 §3.4，CI 调用从 repo root 跑）：

```bash
xcodebuild test \
  -project iphone/PetApp.xcodeproj \
  -scheme PetApp \
  -destination 'platform=iOS Simulator,name=iPhone 17,OS=latest'
```

**`-project` 用 `iphone/PetApp.xcodeproj`**（相对 repo root）；scheme 名 `PetApp`（与 target 同名，XcodeGen 自动生成）。

**5. 单开发者 signing 临时方案（ADR-0002 §1.1）**

`project.yml` 中 `DEVELOPMENT_TEAM` 留空，Bundle ID `com.zhuming.pet.app` 使用 Xcode "Sign to Run Locally" 选项（Xcode 26+ 默认）。模拟器跑不需要签名，真机跑需 dev 在 Xcode 内手动配 signing。本 story 仅模拟器验证，无需配 signing。

### Project Structure Notes

- 与目标结构（iOS 架构设计 §4）的对齐：本 story **首次**在 `iphone/PetApp/` 下落地 §4 描述的目录骨架；按"最小够用"原则只建本 story 用到的子目录（App / Core/DesignSystem/Components / Shared/Constants / Features/Home / Resources）+ 测试目录（PetAppTests/Features/Home / PetAppUITests）
- **检测的差异**（与 §4 不冲突，仅本 story 暂不建）：`Core/Networking,Realtime,Storage,Motion,Health,Logging,Utils,Extensions` / `Shared/Models,Mappers,ErrorHandling` / `Features/{Auth,Pet,Steps,Chest,Cosmetics,Compose,Room,Emoji}` / `Resources/Configs` —— 这些是后续 story 范围（rationale：避免空目录污染，对齐 server 端 Story 1.2 的"不建 domain/repo/service 占位"原则）
- ADR-0002 §3.3 方案 D 与 CLAUDE.md "Repo Separation" 已对齐：`iphone/` 与 `server/` 平级，`ios/` 整个不动作为旧产物归档

### References

- [Source: \_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md] — **本 story 唯一权威 ADR**
  - §3.1 — XCTest only（手写 Mock）
  - §3.2 — async/await 主流 + XCTestExpectation 特定场景兜底
  - §3.3 — **方案 D：在 `iphone/` 下从零建工程；`ios/` 整个原封不动**
  - §3.3 "方案 D 分阶段生效" — 本 story 落地阶段 2（`iphone/scripts/install-hooks.sh` + git hooks）
  - §3.4 — `bash iphone/scripts/build.sh --test` wrapper（**本 story 不实装 build.sh，但目录约定生效；artifact 路径 `iphone/build/`**）
  - §4 — 版本锁定清单（deployment target 17.0 / Xcode 26.4 / Swift 6.3.1 / Bundle ID prefix com.zhuming.pet）
  - §6 — Post-Decision TODO（本 story 满足 Story 2.2 / Story 2.7 部分）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#3 总体工程结论] — Swift + SwiftUI + MVVM + UseCase + Repository
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#4 项目目录建议] — 目标 `PetApp/{App,Core,Shared,Features,Resources,Tests}/`
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#5.1 View 层] / #5.2 ViewModel 层 — View 不直接调网络 / VM 不持久化
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#6.2 Home 模块] — HomeView / HomeViewModel / LoadHomeUseCase 拆分
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#7.1 App Root 状态] — AppLaunchState（本 story 暂不实装，→ Story 2.9）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#18.1 首选技术路线] — async/await 为主，不强引入 Combine
- [Source: docs/宠物互动App_总体架构设计.md#5.1 客户端技术选型] — Swift + SwiftUI + REST + WebSocket
- [Source: docs/宠物互动App_总体架构设计.md#5.4 页面建议] — 主界面是必选页面之一
- [Source: CLAUDE.md "Tech Stack（新方向）"] — iOS 端 = Swift + SwiftUI；HealthKit/CoreMotion 接入
- [Source: CLAUDE.md "Repo Separation（重启阶段过渡态）"] — `iphone/` (iPhone App，新方向；Story 2.2 起在此目录落地)
- [Source: \_bmad-output/planning-artifacts/epics.md#Epic 2 / Story 2.2] — 原始 AC 来源
- [Source: \_bmad-output/implementation-artifacts/2-1-ios-mock-框架选型-ios-目录决策-spike.md] — 上一条 story（Spike）的决策依据
- [Source: \_bmad-output/implementation-artifacts/1-2-cmd-server-入口-配置加载-gin-ping.md] — 跨端对照（server 端 Epic 1 第一条实装 story）

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) — claude-opus-4-7[1m]

### Debug Log References

- `xcodebuild test -project iphone/PetApp.xcodeproj -scheme PetApp -destination 'platform=iOS Simulator,name=iPhone 17,OS=latest'` —— ** TEST SUCCEEDED **
  - PetAppTests.xctest：HomeViewModelTests 5 case + HomeViewTests 5 case，共 10 case 全部通过
  - PetAppUITests.xctest：HomeUITests.testHomeViewShowsAllSixPlaceholders 通过（11.692s）
  - Xcode 26.4.1 (Build 17E202) / Swift 6.3.1 / iOS Simulator 26.4 / XcodeGen 2.45.3
- 测试结果 xcresult: `~/Library/Developer/Xcode/DerivedData/PetApp-*/Logs/Test/Test-PetApp-2026.04.25_22-27-34-+0800.xcresult`

### Completion Notes List

**实装核心交付**

1. `iphone/` 顶级目录骨架建立完毕，与 `server/` / `ios/` 平级；`ios/` 全程零改动（git status 自检通过）。
2. `iphone/project.yml` 字段严格按 ADR-0002 §4：bundleIdPrefix=com.zhuming.pet / deploymentTarget.iOS=17.0 / xcodeVersion=26.4 / SWIFT_VERSION=5.9 / 三个 target；DEVELOPMENT_TEAM 留空（单开发者模拟器跑无需 signing）。
3. `xcodegen generate` 跑通，PetApp.xcodeproj 生成成功且整个工程通过 `xcodebuild test` 验证。
4. 主界面 6 大区块全部落地，8 个 a11y identifier 集中定义在 `AccessibilityID.swift`（无 inline string）；UITest 在真实 simulator 启动 App 后逐一定位 8 个 id 全部找到。

**关键技术选择记录（Dev Note 要求 dev 在 Completion Notes 记录）**

- **HomeViewModel.appVersion 字段值**：选 Dev Note T5.5 推荐方案 —— VM 存 `appVersion = "0.0.0"`（不含 v 前缀），View 层拼 `"v\(appVersion) · \(serverInfo)"` → "v0.0.0 · ----"。避免 "vv0.0.0" 双 v 漂移。
- **`@StateObject` vs `@ObservedObject` 选择**：选 Dev Note #2 第二方案 —— `RootView` 用 `@StateObject private var homeViewModel = HomeViewModel()` 持有 VM 生命周期，向下传给 `HomeView(viewModel:)`；`HomeView` 用 `@ObservedObject var viewModel: HomeViewModel`。这样 VM 由 RootView 单一持有 + HomeView 仅观察，避免 `@StateObject` 接收外部传入导致 multiple init。
- **UITest 跨 element type 兜底**：UITest 用 `app.descendants(matching: .any)[id]` 而非固定 `staticTexts/otherElements/buttons`，对 SwiftUI 渲染产物的元素类型更宽容；按钮单独用 `app.buttons[id]`（按钮类型最稳）。

**AccessibilityID 共享给 UITest 的方式（与 `@testable import PetApp` 不适用 UITest 的协调）**

- UI 测试 target 与单测 target 不同：UITest 是黑盒进程，不能 `@testable import PetApp`。
- 解决：在 `iphone/project.yml` 把 `PetApp/Shared/Constants/AccessibilityID.swift` 作为 PetAppUITests target 的额外 source 直接编译进 UITest bundle，让 UITest 直接引用 `AccessibilityID.Home.<id>` 常量。
- 单测仍走 `@testable import PetApp` 路径（标准做法）。

**ADR-0002 §3.3 方案 D 阶段 2 过渡期警告（T8.4 必填）**

- 如此前已 run 旧 `bash ios/scripts/install-hooks.sh` 安装 pre-push hook（旧 ios/ 工作流），应手工卸载：
  ```
  rm -f .git/hooks/pre-push
  rm -f .git/hooks/pre-commit
  ```
- 然后 run 新版安装：
  ```
  bash iphone/scripts/install-hooks.sh
  ```
- 新版 hook 当前是占位 `exit 0`（真实 swift-format 校验为后续 tech debt）。

**与 epics.md Story 2.2 AC 的对照（AR 完整性）**

- ✅ 模拟器可启动 App + 6 大占位区块 → AC1 / AC3 / AC6（UITest 自动验证）
- ✅ App 名称 / Bundle ID 按 ADR-0002 决策 → AC2 = com.zhuming.pet.app
- ✅ 单元测试 ≥ 3 case（happy 渲染含 a11y id / edge 多尺寸 / happy 按钮回调注册）→ AC4 共 10 case
- ✅ UITest 启动模拟器 + 验证 6 区块 a11y id 都可定位 → AC5 = 8 个 a11y id 全定位

**未完成 / 留 tech debt 项**

- T10.5（commit）：dev-story 阶段不 commit，由后续 story-done 阶段执行。
- `iphone/scripts/git-hooks/pre-commit` 当前是 `exit 0` 占位，真实 swift-format 全工程跑作为后续 story（建议 2.7 / 3.3）扩展点。
- `iphone/scripts/build.sh` wrapper 留给 Story 2.7（按 ADR-0002 §6 TODO）。
- `iphone/PetApp/Core/DesignSystem/Components/` 是空目录（XcodeGen `generateEmptyDirectories: true` 让其在工程内可见但无 .swift 文件）—— 后续 story 用到时实装，符合"避免空目录污染但保留架构骨架可见性"折中。

### File List

新建（全部位于 `iphone/` 顶级目录下，路径相对 repo root）：

- `iphone/project.yml`
- `iphone/PetApp.xcodeproj/`（XcodeGen 生成；含 project.pbxproj 与 xcshareddata）
- `iphone/PetApp/App/PetAppApp.swift`
- `iphone/PetApp/App/RootView.swift`
- `iphone/PetApp/Core/DesignSystem/Components/`（空目录，XcodeGen `generateEmptyDirectories: true` 保留）
- `iphone/PetApp/Shared/Constants/AccessibilityID.swift`
- `iphone/PetApp/Features/Home/Views/HomeView.swift`
- `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`
- `iphone/PetApp/Resources/Info.plist`
- `iphone/PetApp/Resources/Assets.xcassets/Contents.json`
- `iphone/PetApp/Resources/Assets.xcassets/AppIcon.appiconset/Contents.json`
- `iphone/PetApp/Resources/Assets.xcassets/AccentColor.colorset/Contents.json`
- `iphone/PetAppTests/Features/Home/HomeViewTests.swift`
- `iphone/PetAppTests/Features/Home/HomeViewModelTests.swift`
- `iphone/PetAppUITests/HomeUITests.swift`
- `iphone/scripts/install-hooks.sh`
- `iphone/scripts/git-hooks/pre-commit`

修改：

- `.gitignore`（追加 `iphone/build/` 行；ADR-0002 §3.4 round-3 P2 fix）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（development_status: 2-2-swiftui-app-... ready-for-dev → in-progress → review；last_updated 时间戳更新）
- `_bmad-output/implementation-artifacts/2-2-swiftui-app-入口-主界面骨架-信息架构定稿.md`（本文件：勾选 Tasks/Subtasks + 填 Dev Agent Record + Status: ready-for-dev → review）
- `.claude/settings.local.json`（追加 xcodegen / xcodebuild 白名单；AC9 列为可选）

未修改（AC9 硬约束）：

- `ios/` 下所有文件（零改动；watch 留守目录）
- `server/` 下所有文件
- `CLAUDE.md` / `docs/`

## Change Log

- 2026-04-25 — Story 2.2 实装：iPhone 端 PetApp 工程骨架 + HomeView 6 大占位区块 + 单元测试（10 case）+ UITest（1 case）+ git hooks 安装脚本 + .gitignore 同步；`xcodebuild test` 全绿，状态 ready-for-dev → review。
