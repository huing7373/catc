# PetApp (iPhone)

宠物互动 App iPhone 端（Swift + SwiftUI + MVVM + UseCase + Repository）。本目录是 iphone/ 工程；server 见 ../server/，旧产物 ../ios/ 不动（见 [CLAUDE.md](../CLAUDE.md)）。

> 节点 1（App 与 Server 可运行）阶段；HealthKit / CoreMotion / 分享链接 URL Scheme 在 Epic 8 / Epic 35 才接入，本节点跑 simulator demo **不需要**它们。详见 [`docs/宠物互动App_MVP节点规划与里程碑.md`](../docs/宠物互动App_MVP节点规划与里程碑.md) §3。

---

## 快速启动

3 行命令把 simulator demo 跑起来。所有命令**从仓库根目录**跑（你 clone 到的目录，命令里所有 `iphone/...` / `server/...` 路径都相对它；macOS-only；锁定 macOS 14+ / Xcode 26.4+）：

```bash
# 第一次：装 xcodegen（仅一次；已装即 no-op）
brew install xcodegen

# 生成 .xcodeproj + 编译 + 跑单元测试（一键）
bash iphone/scripts/build.sh --test

# Xcode 启动模拟器 demo（GUI 路径）
open iphone/PetApp.xcodeproj
# 然后在 Xcode 选 PetApp scheme + iPhone 17 simulator + Cmd+R
```

> `bash iphone/scripts/build.sh --test` 内部已 `xcodegen generate`，dev **不需要**单独再跑 xcodegen；它也是 CI 与本地共用的统一入口（详见 [ADR-0002 §3.4](../_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md) + [`iphone/docs/CI.md`](docs/CI.md)）。

**坑提醒**：

- **不要**用裸 `xcodebuild build ...` 替代 wrapper：会绕过 xcodegen regen，`iphone/project.yml` 改了不会生效（详见 [Troubleshooting #1 / #2](#troubleshooting)）。
- 端口 `8080` 与 baseURL `http://localhost:8080` 与 [`iphone/project.yml`](project.yml) 第 37 行 `PetAppBaseURL` 默认值一致；真机联调改 baseURL 走 [§服务端联调](#服务端联调)，不要在快速启动命令里随手换。
- "iPhone 17 simulator" 与 [`iphone/scripts/build.sh`](scripts/build.sh) `DESTINATION_PRIMARY` 对齐；如果 dev 装 Xcode 16 没 iPhone 17，`build.sh` 自动 fallback（详见 [Troubleshooting #2](#troubleshooting)）。

---

## 依赖

### 当前 Epic 2 依赖

| 工具 | 版本 | 验证命令 |
|---|---|---|
| macOS | 任意现代版（实测 macOS 14+） | `sw_vers` |
| Xcode | tested 26.4 / minimum 16.0 | `xcodebuild -version` |
| XcodeGen | 2.45+ | `xcodegen --version` |
| bash | macOS 自带 | `bash --version` |
| iOS Simulator runtime | Xcode 自带 | `xcrun simctl list runtimes` |

> Xcode 双字段（tested + minimum）来自 [ADR-0002 §4](../_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md) 版本锁定清单。`iphone/project.yml` 第 7 行 `xcodeVersion: "26.4"` + 第 6 行 `deploymentTarget.iOS: "17.0"` 是工程钦定下限；模拟器 runtime 装的是 iOS 16 跑会失败（runtime mismatch）。
>
> XcodeGen 装：`brew install xcodegen`（macOS-only；首次必装；详见 [Troubleshooting #1](#troubleshooting)）。

### MVP 演进依赖（Epic 5 / Epic 8 / Epic 35 才接入）

| 服务 / 权限 | 启用节点 | 备注 |
|---|---|---|
| Apple Developer Account | Epic 5+ 真机联调 / TestFlight | 模拟器开发**不**需要 |
| HealthKit 权限 | **Epic 8 Story 8.1** | `Info.plist` 加 `NSHealthShareUsageDescription`；详见 [§Info.plist 关键配置 → 未来 Epic 落地的 key](#info-plist-关键配置) |
| CoreMotion 权限 | **Epic 8 Story 8.2** | `Info.plist` 加 `NSMotionUsageDescription` |
| 分享链接 URL Scheme | **Epic 35 Story 35.1** | `Info.plist` 加 `CFBundleURLTypes`（如 `catapp://`） |

> Epic 2 阶段不需要任何上述 entitlement / 付费账号，新 dev 不必先去 Apple Developer Portal 申请。

### 测试依赖

`bash iphone/scripts/build.sh --test` 跑的是 XCTest（Xcode 自带），无第三方测试框架。

| 依赖 | 来源 | 备注 |
|---|---|---|
| XCTest | Xcode 自带 | [ADR-0002 §3.1](../_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md) 钦定**仅** XCTest + 手写 mock，**不**引第三方 mock 框架 / 不引 SnapshotTesting / 不引 ViewInspector |
| `MockBase` | [`iphone/PetAppTests/Helpers/MockBase.swift`](PetAppTests/Helpers/MockBase.swift) | Story 2.7 通用 mock 基类 |
| `AsyncTestHelpers` | [`iphone/PetAppTests/Helpers/AsyncTestHelpers.swift`](PetAppTests/Helpers/AsyncTestHelpers.swift) | Story 2.7（含 `assertThrowsAsyncError`） |

### Swift Package 依赖

**当前 Epic 2 阶段 0 个第三方 SPM 依赖**（[ADR-0002 §3.1 / §3.2 / §3.3](../_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md) 一致原则：零外部依赖）。仅用 Foundation / Combine / SwiftUI / XCTest 系统库。未来 PR 引入 SPM 依赖前必须先讨论 / 写 ADR。

---

## 跑测试

### 命令矩阵

| 命令 | 用途 |
|---|---|
| `bash iphone/scripts/build.sh` | 仅 build，**不**跑测试（含 `xcodegen generate`） |
| `bash iphone/scripts/build.sh --test` | 单元测试（PetAppTests scheme，主示范） |
| `bash iphone/scripts/build.sh --uitest` | UI 测试（PetAppUITests scheme，XCUITest 黑盒） |
| `bash iphone/scripts/build.sh --test --uitest --coverage-export` | 全跑 + 导出 coverage 到 `iphone/build/coverage.json` |
| `bash iphone/scripts/build.sh --clean --test` | 清 DerivedData 后再跑测试（用于排查增量编译怪事） |

### 互斥 / 联动规则

- `--coverage-export` **必须**配 `--test` 或 `--uitest`，否则 preflight 拒（脚本会报 `ERROR: --coverage-export 要求 --test 或 --uitest` 退出）
- `--test` 与 `--uitest` 可**同时**跑（XCUITest 黑盒模式与 unit test 是两套独立 invocation；不互斥）
- 其余开关正交（详见 [`docs/lessons/2026-04-26-build-script-flag-matrix.md`](../docs/lessons/2026-04-26-build-script-flag-matrix.md)）

### Xcode IDE 入口

- 打开 `iphone/PetApp.xcodeproj` → `Cmd+U` 跑全部测试
- `Cmd+5` 打开 Test Navigator → 单 case 旁边的菱形 ◇ 按钮跑单测试
- **注意**：Xcode IDE 跑**不会**自动 `xcodegen generate`；如果改了 `iphone/project.yml`，必须先手跑 `xcodegen generate` 或用 `bash iphone/scripts/build.sh --test`。

### artifacts 路径

| 路径 | 何时产出 | 怎么看 |
|---|---|---|
| `iphone/build/test-results.xcresult` | `--test` 后 | Xcode 双击打开看完整 simulator 日志 / coverage / screenshot |
| `iphone/build/test-results-ui.xcresult` | `--uitest` 后 | 同上（含 UI 录制） |
| `iphone/build/coverage.json` | `--coverage-export` 后 | `cat` / `jq` 直接看（xcrun xccov 导出） |
| `iphone/build/DerivedData/` | 任意一次跑后 | Xcode 增量编译缓存；`--clean` 会删 |

> 全部 `iphone/build/` 已 gitignore（仓库根 `.gitignore` 末段 "iPhone App build artifacts" 一行）。

### Destination 三段 fallback

`build.sh` 自动按以下顺序选 simulator：

1. `platform=iOS Simulator,name=iPhone 17,OS=latest`（首选；Xcode 26.4 默认机型）
2. `platform=iOS Simulator,OS=latest`（fallback；任意 iPhone simulator）
3. `xcrun simctl list devices iOS available` 第一个可用 UUID（最后兜底；Xcode 16 等旧版默认机型不含 iPhone 17 时走这里）

详见 [ADR-0002 §3.4 已知坑第 2 条](../_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md) + [`docs/lessons/2026-04-26-xcodebuild-showdestinations-section-aware.md`](../docs/lessons/2026-04-26-xcodebuild-showdestinations-section-aware.md)。脚本 stdout 会打印 `=== resolved destination: ... ===` 让你确认实际用的。

### 测试策略指针

XCTest only + 手写 mock + `MockBase` 通用基类 + `async/await` 主流（`XCTestExpectation` 仅特定场景）；详见 [ADR-0002 §3.1 / §3.2](../_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md)。本 README **不**复制 ADR 内容，给指针即可。

> 想只跑某一个 test class：`xcodebuild test -project iphone/PetApp.xcodeproj -scheme PetApp -only-testing:PetAppTests/HomeViewModelTests -destination 'platform=iOS Simulator,name=iPhone 17,OS=latest'`。但**主力示范**是 `bash iphone/scripts/build.sh --test`（保证 xcodegen regen + destination fallback + 统一 artifact 路径）。

---

## Dev mode

### 启用方式（与 server 端**不同**）

iOS 端**单闸门**：`#if DEBUG`（Xcode Build Configuration = Debug 时编译器自动定义）。

| 闸门 | 作用范围 | 触发条件 |
|---|---|---|
| **编译期** | `#if DEBUG` 包裹的代码 | Xcode 默认 `Cmd+R` 走 Debug；`xcodebuild -configuration Debug` 同义 |

> **与 server 端 `BUILD_DEV=true`（运行期）+ `--devtools` (`-tags devtools`，编译期) 双闸门不同**：iOS 没有"开发期 + Release 二进制叠加 dev 端点"的需求，`#if DEBUG` 单闸门已足够。新 dev **不要**找 `BUILD_DEV` 等环境变量入口（不存在）。

### Debug 与 Release 行为差异

| 配置 | `#if DEBUG` 包裹的代码 | 备注 |
|---|---|---|
| **Debug** build（Xcode `Cmd+R` / `xcodebuild -configuration Debug`） | 编译进二进制 + 视图树渲染 | dev 默认走这条 |
| **Release** build（Xcode Archive / `xcodebuild -configuration Release`） | 编译器**物理剔除**（type 定义都看不到） | 调用方**也**必须用 `#if DEBUG` 包裹引用，否则 Release 编译失败；fail-closed 设计 |

> 详见 [`docs/lessons/2026-04-26-simulator-placeholder-vs-concrete.md`](../docs/lessons/2026-04-26-simulator-placeholder-vs-concrete.md)。

### 当前 dev 入口

Epic 2 阶段唯一 dev 入口：HomeView 右上角"重置身份"按钮（详见 [§Dev 工具](#dev-工具)）。

**没有** dev API 端点 / dev URL scheme / dev menu（与 server 端 `/dev/ping-dev` 不同；iOS 端用 UI 按钮承载 dev 操作）。

### 未来 dev 工具占位

Epic 2 仅落地框架，业务 dev 工具由各 Epic 扩展：

- DEV API base URL switcher（任意 Epic 真机联调时；目前 `PetAppBaseURL` xcconfig 覆盖足够）
- HealthKit 步数注入 UI（Epic 8 落地）
- Force unlock chest UI（Epic 20 Story 20-7 类比 server `POST /dev/force-unlock-chest`）

> 完整 UI 设计 / 触发逻辑留给各自 story 自己写；本 README **不**预填。

### 生产部署 SOP

Release build / TestFlight / App Store 提交**必须**用 Xcode Archive（默认 Release configuration），自动剔除 `#if DEBUG` 代码。**永远不要**在 Release build 里手工启用 dev 工具（如改 `#if DEBUG` 为 `#if true`）；这是 fail-closed 设计（详见 [ADR-0002 §3.1](../_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md) + [Story 2.8](../_bmad-output/implementation-artifacts/2-8-dev-重置-keychain-按钮.md)）。

---

## Dev 工具

### 重置身份按钮（Story 2.8）

| 字段 | 值 |
|---|---|
| 位置 | HomeView 右上角（与版本号文字同区域） |
| SF Symbol | `arrow.counterclockwise.circle` |
| accessibilityLabel | `重置身份` |
| 触发 | `ResetKeychainUseCase.execute()` → 清空 `KeychainStore` 全部 key（`guestUid` + `token`） → 弹 alert"已重置 / 请杀进程后重新启动 App 模拟首次安装" |
| 渲染条件 | **仅 Debug build**（Release build 视图树物理移除，详见 [§Dev mode](#dev-mode)） |

来源：[`iphone/PetApp/Features/DevTools/Views/ResetIdentityButton.swift`](PetApp/Features/DevTools/Views/ResetIdentityButton.swift) + [`iphone/PetApp/Features/DevTools/UseCases/ResetKeychainUseCase.swift`](PetApp/Features/DevTools/UseCases/ResetKeychainUseCase.swift)。

### 当前 KeychainStore 实装

| 阶段 | 实装 | 行为 |
|---|---|---|
| **Epic 2（当前）** | `InMemoryKeychainStore`（占位 / 测试用） | 进程内字典；App 重启即丢；"重置身份"表面上不会改变启动行为 |
| **Epic 5 Story 5.1** | `KeychainServicesStore`（真实 macOS / iOS Keychain Services） | App 重启 + 卸载重装才丢；此时"重置身份"才有真持久 token 可清 |

> "重置身份" UI 在两阶段行为相同，但**真重置效果**只有 Story 5.1 落地后才完整。当前 dev 阶段看到"按了之后启动行为没变"是**预期**（详见 [Troubleshooting #6](#troubleshooting)）。

### 何时用

demo / 测试场景：

1. 验证首次启动流程（GuestLogin / LaunchingView）
2. 排查 token 过期 / Keychain 数据残留
3. 不必卸载重装即可模拟"全新安装"

> **生产**用户看不到此按钮（Release build 物理剔除）。

### 未来 dev 工具入口位置

Story 2.2 主界面右上角"角落 dev info 区域"已预留（与 Story 2.5 版本号文字同区域）；未来 DEV API base URL switcher / HealthKit 注入 UI 会加在同一区域。**不**新增"dev 设置页"全屏 sheet（YAGNI）。

---

## Info.plist 关键配置

### 当前 Epic 2 已有 key（来源：[`iphone/project.yml`](project.yml) 第 24-46 行 `info.properties`）

| Key | 当前值 | 含义 / 何时改 |
|---|---|---|
| `CFBundleDisplayName` | `PetApp` | App 主界面 / 设置中显示名；改名要同步 marketing materials |
| `CFBundleShortVersionString` | `1.0.0` | App Store 版本号；release 时按 SemVer bump |
| `CFBundleVersion` | `1` | Build number；CI 注入递增（未来 Epic 3+） |
| `UILaunchScreen` | `{}`（空 dict） | iOS 系统启动屏；空 dict = 默认白屏。Story 2.9 [`LaunchingView`](PetApp/Features/Launching/Views/LaunchingView.swift) 是 SwiftUI 进程接管后的"应用启动等待页"，**与系统 launch screen 是两个层级** |
| `LSRequiresIPhoneOS` | `true` | iPhone-only（不支持 iPad / Mac Catalyst） |
| `UISupportedInterfaceOrientations` | `[Portrait]` | 仅竖屏；Epic 22+ 房间页如需横屏单独 spike |
| `UIApplicationSceneManifest.UIApplicationSupportsMultipleScenes` | `false` | 单 scene；不支持 iPad 多窗口 |
| **`PetAppBaseURL`** | `http://localhost:8080` | **Story 2.5 关键 key**：`AppContainer` 启动时从此读 server baseURL；缺失时 fallback `http://localhost:8080`。**host-only 契约**（**无** `/api/v1` 前缀，APIClient 拼路径加前缀）。详见 [`docs/lessons/2026-04-26-baseurl-from-info-plist.md`](../docs/lessons/2026-04-26-baseurl-from-info-plist.md) + [`docs/lessons/2026-04-26-baseurl-host-only-contract.md`](../docs/lessons/2026-04-26-baseurl-host-only-contract.md) |
| **`NSAppTransportSecurity.NSAllowsLocalNetworking`** | `true` | **Story 2.5 关键 key**：iOS ATS 默认拒 cleartext HTTP，本 key 例外允许 localhost / `.local` / 私有 IP，**不**允许公网 cleartext（比 `NSAllowsArbitraryLoads` 安全）。详见 [`docs/lessons/2026-04-26-ios-ats-cleartext-http.md`](../docs/lessons/2026-04-26-ios-ats-cleartext-http.md) |

### 未来 Epic 落地的 key（占位说明）

| Key | 落地 Story | 含义 |
|---|---|---|
| `NSHealthShareUsageDescription` | **Epic 8 Story 8.1** | HealthKit 权限弹窗文案（如"用于读取每日步数喂猫"）；缺失会让 HealthKit 调用 fail |
| `NSMotionUsageDescription` | **Epic 8 Story 8.2** | CoreMotion 权限弹窗文案（如"用于识别走路 / 跑步状态切换猫动作"） |
| `CFBundleURLTypes` | **Epic 35 Story 35.1** | Custom URL scheme（如 `catapp://`）；分享链接深链接 |

> 完整 Usage Description 文案样本属各 Epic 自己 AC 范畴，本 README **只**预告 key 名 + epics.md story 编号。

### 修改 Info.plist 的标准流程

**禁止直接编辑** [`iphone/PetApp/Resources/Info.plist`](PetApp/Resources/Info.plist)（XcodeGen 生成时会被 [`iphone/project.yml`](project.yml) `info.properties` 段覆盖）。修改步骤：

```bash
# 1. 改 iphone/project.yml 的 targets.PetApp.info.properties 段
$EDITOR iphone/project.yml

# 2. regen Info.plist（内含 xcodegen generate）
bash iphone/scripts/build.sh

# 3. 验证（如新增 key 名为 NewKey）
plutil -p iphone/PetApp/Resources/Info.plist | grep NewKey
```

---

## 目录结构

源自 [`docs/宠物互动App_iOS客户端工程结构与模块职责设计.md`](../docs/宠物互动App_iOS客户端工程结构与模块职责设计.md) §4，精简到 `iphone/` 子树。`✅` = Epic 2 已实装；`🚧` = 未来 Epic 落地；`⚙️` = 工具产物（gitignored / generated）。

```text
iphone/
├─ project.yml                   # ✅ Epic 2 Story 2.2 / 2.5 / 2.7 锁定（XcodeGen 工程描述）
├─ PetApp.xcodeproj/             # ✅ xcodegen generate 产物（入 git，便于 Xcode 直开）
├─ scripts/
│  ├─ build.sh                   # ✅ Epic 2 Story 2.7（destination 三段 fallback）
│  ├─ install-hooks.sh           # ✅ Epic 2 Story 2.2 / 2.7（git hooks 安装器）
│  └─ git-hooks/                 # ✅ pre-commit / pre-push hooks
├─ docs/
│  ├─ CI.md                      # ✅ Epic 2 Story 2.7（CI 入口约定）
│  └─ lessons/                   # 注：iPhone 端 lessons 实际写在 docs/lessons/（仓库根）
├─ build/                        # ⚙️ gitignored（xcresult / DerivedData / coverage.json）
├─ PetApp/
│  ├─ App/                       # ✅ Epic 2（PetAppApp / RootView / AppContainer / AppCoordinator / AppLaunchState* / AppLaunchStateMachine）
│  ├─ Core/
│  │  ├─ DesignSystem/           # ✅ Epic 2 Story 2.6（基础组件 Toast / AlertOverlay / RetryView）
│  │  ├─ Networking/             # ✅ Epic 2 Story 2.4（APIClient / Endpoint / APIResponse / APIError / URLSessionProtocol）
│  │  ├─ Storage/                # ✅ Epic 2 Story 2.8（KeychainStore InMemory 占位；Epic 5 Story 5.1 真 Keychain Services）
│  │  ├─ Realtime/               # 🚧 Epic 12 Story 12.2（WebSocketClient）
│  │  ├─ Motion/                 # 🚧 Epic 8 Story 8.2（CoreMotion adapter）
│  │  ├─ Health/                 # 🚧 Epic 8 Story 8.1（HealthKit adapter）
│  │  ├─ Logging/                # 🚧 Epic 5+ 落地
│  │  ├─ Utils/                  # 🚧 按需追加
│  │  └─ Extensions/             # 🚧 按需追加
│  ├─ Shared/
│  │  ├─ Constants/              # ✅ Epic 2（AccessibilityID）
│  │  ├─ ErrorHandling/          # ✅ Epic 2 Story 2.6（ErrorPresenter）
│  │  ├─ Testing/                # ✅ Epic 2（产品 target 内的 testing helper 占位）
│  │  ├─ Models/                 # 🚧 Epic 4+ DTO models 落地
│  │  └─ Mappers/                # 🚧 Epic 4+ DTO ↔ Domain mappers
│  ├─ Features/
│  │  ├─ Home/                   # ✅ Epic 2 Story 2.2 / 2.5（HomeView / HomeViewModel / PingUseCase）
│  │  ├─ Launching/              # ✅ Epic 2 Story 2.9（LaunchingView）
│  │  ├─ DevTools/               # ✅ Epic 2 Story 2.8（ResetIdentityButton + ResetKeychainUseCase）
│  │  ├─ Auth/                   # 🚧 Epic 5 Story 5.2（GuestLoginUseCase）
│  │  ├─ Pet/                    # 🚧 Epic 8 / 30 落地
│  │  ├─ Steps/                  # 🚧 Epic 8 落地
│  │  ├─ Chest/                  # 🚧 Epic 21 落地
│  │  ├─ Cosmetics/              # 🚧 Epic 24 / 27 落地
│  │  ├─ Compose/                # 🚧 Epic 33 落地
│  │  ├─ Room/                   # 🚧 Epic 12 落地
│  │  └─ Emoji/                  # 🚧 Epic 18 落地
│  └─ Resources/
│     ├─ Assets.xcassets/        # ✅ Epic 2（占位资产）
│     └─ Info.plist              # ⚙️ XcodeGen 生成（详见 §Info.plist 关键配置）
├─ PetAppTests/                  # ✅ Epic 2 Story 2.7 起逐 story 落测试
│  ├─ Helpers/                   # ✅ MockBase + AsyncTestHelpers + SampleViewModelTests
│  ├─ App/                       # ✅ AppContainer / AppCoordinator / AppLaunchStateMachine / RootViewWire / SheetType tests
│  ├─ Core/                      # ✅ DesignSystem / Networking tests
│  ├─ Features/                  # ✅ DevTools / Home / Launching tests
│  └─ Shared/                    # ✅ ErrorHandling tests
└─ PetAppUITests/                # ✅ Epic 2 Story 2.7 起逐 story 落（XCUITest）
   ├─ HomeUITests.swift          # ✅ Epic 2
   └─ NavigationUITests.swift    # ✅ Epic 2
```

> 分层职责（View / ViewModel / UseCase / Repository）详见 [`docs/宠物互动App_iOS客户端工程结构与模块职责设计.md`](../docs/宠物互动App_iOS客户端工程结构与模块职责设计.md) §5。
>
> 与设计文档 §4 的差异：实际落地 `PetAppTests/` + `PetAppUITests/` 是顶层（不是 `PetApp/Tests/`），按 [ADR-0002 §3.3 方案 D + Story 2.7 实装](../_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md)；README 反映实际目录结构。

---

## Troubleshooting

| # | 症状 | 原因 / 解决 |
|---|---|---|
| 1 | `bash iphone/scripts/build.sh` 报 `xcodegen: command not found` 或 `ERROR: xcodegen 未安装` | XcodeGen 未装。**解决**：`brew install xcodegen`；验证 `xcodegen --version` 输出 `2.45+`。详见 [ADR-0002 §3.4](../_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md) + [`docs/lessons/2026-04-26-build-script-flag-matrix.md`](../docs/lessons/2026-04-26-build-script-flag-matrix.md) |
| 2 | `xcodebuild` 报 `Unable to find a destination matching ... iPhone 17` | Xcode 16 等旧版默认机型不含 iPhone 17。**解决**：`build.sh` 已实装三段 fallback（iPhone 17 → OS=latest → xcrun simctl 第一个可用），看 stdout `=== resolved destination: ... ===` 输出确认实际用的；如需手指定：① `xcrun simctl list devices iOS available` 找你想用机型的 UUID；② 直接传 `id=<UUID>`，例如 `xcodebuild ... -destination "platform=iOS Simulator,id=<UUID>"`（**推荐**）。`xcodebuild` 的 `-destination` 字段语义要求**要么** `id=<UUID>` 单独成立，**要么** `name=<device>,OS=<X.Y>` 三件套同时给；只给 `name=<device>` 缺 `OS` 字段会报 `Unable to find a destination`。详见 [ADR-0002 §3.4 P1 fix](../_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md) + [`docs/lessons/2026-04-26-xcodebuild-showdestinations-section-aware.md`](../docs/lessons/2026-04-26-xcodebuild-showdestinations-section-aware.md) |
| 3 | App 在真机上启动后角落 server 信息永远显示 `Server offline` | 真机上 `localhost` 解析为设备自身，不是 Mac 上 server。**解决**：① 改 [`iphone/project.yml`](project.yml) 第 37 行 `PetAppBaseURL` 为 Mac 的局域网 IP（如 `http://192.168.1.100:8080`）；② regen Info.plist：`bash iphone/scripts/build.sh`；③ Mac 与真机同 Wi-Fi。详见 [§服务端联调](#服务端联调) + [`docs/lessons/2026-04-26-baseurl-from-info-plist.md`](../docs/lessons/2026-04-26-baseurl-from-info-plist.md) |
| 4 | App 启动报 `App Transport Security has blocked a cleartext HTTP ... resource load` | iOS 默认拒 cleartext HTTP；测试 server 是 `http://`（非 https）。**解决**：当前 [`iphone/project.yml`](project.yml) 已配 `NSAppTransportSecurity.NSAllowsLocalNetworking: true` 允许 localhost / 私有 IP / `.local`。如果你用公网 IP 联调（不推荐），临时加 `NSExceptionDomains` 白名单（**不要**用 `NSAllowsArbitraryLoads`）。详见 [`docs/lessons/2026-04-26-ios-ats-cleartext-http.md`](../docs/lessons/2026-04-26-ios-ats-cleartext-http.md) |
| 5 | `bash iphone/scripts/build.sh --test` 首次跑 5+ 分钟 | Simulator 冷启动 + 编译缓存 cold。**解决**：第二次跑会快（DerivedData 复用）；CI 上加 cache。如本机巨慢检查 Simulator 是否被其他 Xcode 实例锁住：Activity Monitor 看 `com.apple.CoreSimulator` 进程，必要时 `xcrun simctl shutdown all` 后重跑 |
| 6 | Simulator 上点"重置身份"按钮后 Keychain 看似没清干净 | Epic 2 阶段 `KeychainStore` 是 `InMemoryKeychainStore` 占位实装（[`iphone/PetApp/Core/Storage/KeychainStore.swift`](PetApp/Core/Storage/KeychainStore.swift)），重启 App 进程即丢，"重置身份"表面上不会改变启动行为。Epic 5 Story 5.1 替换为 `KeychainServicesStore` 后才有真持久 token 可清。当前现象**预期**。详见 [§Dev 工具 → 当前 KeychainStore 实装](#dev-工具) |

> **不**列举 `docs/lessons/` 30+ 条全部；多数是 review-time 沉淀 / 不是新 dev 入门会遇到的常见坑。完整 lesson 列表见 [`docs/lessons/`](../docs/lessons/) 目录。

---

## 服务端联调

### 本地 simulator 联调（Epic 2 默认场景）

```bash
# 1. 仓库根目录跑 server（默认监听 127.0.0.1:8080）
# 假设你已经 cd 到仓库根；如果还没，先 cd 到你 clone 的目录
bash scripts/build.sh
./build/catserver -config server/configs/local.yaml &

# 2. 另开 shell：跑 simulator demo
open iphone/PetApp.xcodeproj
# Xcode 选 PetApp scheme + iPhone 17 simulator + Cmd+R
```

成功标志：simulator 启动后 HomeView 右下角版本标签从 `v0.0.0 · ----` 变成 `v<X.Y.Z> · <8位commit>`（成功，X.Y.Z 是 iPhone App 版本号、8 位 commit 是 server 的 git short hash）；server 未启则显示 `v<X.Y.Z> · offline`；server 启了但 `/version` 解析异常会显示 `v<X.Y.Z> · v?`。

> `PetAppBaseURL` 默认 `http://localhost:8080`，与 [`server/configs/local.yaml`](../server/configs/local.yaml) `bind_host: 127.0.0.1` + `http_port: 8080` 对齐。simulator 上 `localhost` 实际指向 Mac 本机 loopback（与真机不同）。

### 真机联调（Epic 5+ / TestFlight 准备）

**前置步骤（仅真机首次必跑）—— 配置 code signing**：

仓库 `iphone/project.yml` 里 `DEVELOPMENT_TEAM: ""` 是空字符串（dev-tools 框架钦定的占位）。真机 `Cmd+R` 在签名前会 build fail，连 ATS / 网络都还没触发。每个开发者必须在本地 Xcode 里配 personal team（不会改 project.yml，是 Xcode local override）：

1. Xcode 打开 `iphone/PetApp.xcodeproj`
2. 左侧 navigator 选 PetApp project → TARGETS 选 PetApp → 顶部 tab 选 **Signing & Capabilities**
3. 勾选 **Automatically manage signing** → **Team** 下拉选你的 Apple ID
   - 首次需先去 **Xcode → Settings → Accounts** 加个人 Apple ID（免费 personal team 即可）
4. 真机 USB 连 Mac → Xcode 顶部 device picker 选你的真机

```bash
# 1. Mac 找局域网 IP
ifconfig | grep 'inet 192' | head -1
# 假设输出 inet 192.168.1.100 ...

# 2. 改 iphone/project.yml 第 37 行 PetAppBaseURL
# 从  PetAppBaseURL: http://localhost:8080
# 改  PetAppBaseURL: http://192.168.1.100:8080

# 3. regen Info.plist
bash iphone/scripts/build.sh

# 4. server 端同步改 bind_host（loopback 拒绝外部连接）
# 改 server/configs/local.yaml 的 server.bind_host: 0.0.0.0
# 或删此行让 server 监听所有网卡

# 5. 重启 server（必须！现有进程仍监听 127.0.0.1，配置文件改了不会热加载）
pkill catserver  # 或 Ctrl+C 之前那个进程
bash scripts/build.sh
./build/catserver -config server/configs/local.yaml &

# 6. Xcode 选真机 + Cmd+R
```

> **注意**：`server/configs/local.yaml` 默认 `bind_host: 127.0.0.1` loopback **拒绝**真机连接；必须改 `0.0.0.0` 或删此行（详见 [`server/README.md`](../server/README.md) §配置 `bind_host` 字段说明）。`bind_host` 改了**必须重启** catserver 进程（无 hot reload），否则现有进程仍监听旧地址，phone 显示 `offline`。Mac 与真机必须同 Wi-Fi。

### `PetAppBaseURL` 解析逻辑

`AppContainer.resolveDefaultBaseURL(from: Bundle.main)` 优先级（来源：[`iphone/PetApp/App/AppContainer.swift`](PetApp/App/AppContainer.swift)）：

1. Info.plist `PetAppBaseURL` key
2. fallback `http://localhost:8080`（[`AppContainer.swift`](PetApp/App/AppContainer.swift) `fallbackBaseURLString` 常量）

**host-only 契约**：baseURL **无** `/api/v1` 前缀，APIClient 拼路径时加前缀（详见 [`docs/lessons/2026-04-26-baseurl-host-only-contract.md`](../docs/lessons/2026-04-26-baseurl-host-only-contract.md) + [`docs/lessons/2026-04-26-url-trailing-slash-concat.md`](../docs/lessons/2026-04-26-url-trailing-slash-concat.md)）。

**失败回退场景**：URL 格式错 / scheme 非 http(s) / host 缺失 → 静默回退到 fallback（不抛、不打 log；详见 [`AppContainer.swift`](PetApp/App/AppContainer.swift) 注释 + [`docs/lessons/2026-04-26-url-string-malformed-tolerance.md`](../docs/lessons/2026-04-26-url-string-malformed-tolerance.md)）。

```swift
// iphone/PetApp/App/AppContainer.swift（节选）
public static let baseURLInfoKey = "PetAppBaseURL"
public static let fallbackBaseURLString = "http://localhost:8080"

public static func resolveDefaultBaseURL(from bundle: Bundle) -> URL {
    // 1. 优先读 Info.plist[PetAppBaseURL]
    // 2. malformed / 缺失 → fallback http://localhost:8080
}
```

### 未来 baseURL 多档环境（占位）

Epic 5+ 真机 / TestFlight / App Store 时会引入 dev / staging / prod 多档 baseURL；当前 Epic 2 阶段**只有**默认 `localhost`。预留方案：Xcode xcconfig 文件按 configuration 注入不同 `PetAppBaseURL`；详见 [ADR-0002 §6 Post-Decision TODO](../_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md)。

### 跨端集成测试（Epic 3 范围）

Epic 3 Story 3.1 落地 `_bmad-output/implementation-artifacts/e2e/node-1-ping-e2e.md`，复用本节 "本地 simulator 联调" 步骤作为模板。Epic 3 完成时本 README 应**反向引用** E2E 文档路径（参见 [§维护说明](#维护说明)）。

---

## 维护说明

本 README 是 Epic 2 收官 story（[Story 2.10](../_bmad-output/implementation-artifacts/2-10-ios-readme-模拟器开发指南.md)）的产出。**每个 Epic 完成时如有命令 / 配置 / 流程变化，必须回头同步更新本 README**（属各 Epic 文档同步 story 范畴，如 Epic 3 Story 3-3 / Epic 6 Story 6-3 / ... / Epic 36 Story 36-3）。

**典型同步触发**：

| Epic 完成 | 本 README 要改的章节 |
|---|---|
| Epic 5（Auth + 真 Keychain） | [§Dev 工具](#dev-工具)：`InMemoryKeychainStore` 描述改为 `KeychainServicesStore`；[§Troubleshooting #6](#troubleshooting) 移除"占位行为"说明 |
| Epic 8（HealthKit + CoreMotion） | [§依赖](#依赖)：HealthKit / CoreMotion 从"未来"改"必需"；[§Info.plist 关键配置](#info-plist-关键配置)：`NSHealthShareUsageDescription` / `NSMotionUsageDescription` 从"占位"改"已配置"+ 加模拟器注入步数操作步骤；[§目录结构](#目录结构)：`Core/Health/` / `Core/Motion/` 标 ✅ |
| Epic 12（WebSocket） | [§目录结构](#目录结构)：`Core/Realtime/` / `Features/Room/` 标 ✅ |
| Epic 35（分享链接） | [§Info.plist 关键配置](#info-plist-关键配置)：`CFBundleURLTypes` 从"占位"改"已配置" + URL scheme 实际值 |
| Epic 36（MVP 整体收官） | 全 README 一遍体检；🚧 标记应已全部消除变 ✅；本段提示后续生产化 / 部署 SOP 单独 PR |

> 维护原则与 [`epics.md` §Story 2.10](../_bmad-output/planning-artifacts/epics.md) 钦定的"同步原则"一致：README 与代码不分裂。

## 工程纪律 → 见 [ADR-0002](../_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md) + [docs/lessons/](../docs/lessons/)
