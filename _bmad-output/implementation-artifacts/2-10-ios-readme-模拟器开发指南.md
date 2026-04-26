# Story 2.10: iOS README + 模拟器开发指南

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 端开发 / 新加入团队成员,
I want 一份位于 `iphone/README.md` 的本地开发指南，把 Epic 2（Story 2.1 ~ 2.9）已经落地的 build / test / dev mode / Info.plist 配置 / 目录结构 / troubleshooting / server 联调在一处说清楚,
so that 我打开 `iphone/` 目录就立即知道怎么跑起来 + 改完代码怎么验，不用反向翻 `CLAUDE.md` / Epic 2 的 9 篇 story 文件 / ADR-0002 / 30+ 篇 lessons。

## 故事定位（Epic 2 第十条 = 收官 story；对标 server 端 Story 1.10）

- **Epic 2 进度**：2.1 (ADR-0002) → 2.2 (`iphone/` 目录 + SwiftUI App 入口 + 主界面骨架) → 2.3 (导航架构 NavigationStack + Sheet) → 2.4 (APIClient 封装) → 2.5 (ping/version 显示 + Info.plist `PetAppBaseURL`) → 2.6 (错误 UI 框架 Toast/Alert/RetryView) → 2.7 (测试基础设施 + `iphone/scripts/build.sh` + MockBase) → 2.8 (dev "重置身份" 按钮 + InMemoryKeychainStore) → 2.9 (LaunchingView + AppLaunchStateMachine) **全部 done**；本 story 是 Epic 2 **唯一的纯文档 story**，把前 9 条沉淀在一份 README 里
- **对标参考**：server 端 Story 1.10（[`_bmad-output/implementation-artifacts/1-10-server-readme-本地开发指南.md`](1-10-server-readme-本地开发指南.md)）+ 已落地的 [`server/README.md`](../../server/README.md) 是**唯一权威模板**。本 README 镜像 server README 的"快速启动 / 依赖 / 配置 / 跑测试 / Dev mode / 目录结构 / Troubleshooting / 工程纪律"骨架，但内容全部替换为 iPhone 端实际命令 / 配置 / 工具链
- **epics.md AC 钦定**：epics.md §Story 2.10（行 4434-4460）已**精确**列出 README 必须包含的 9 个章节（快速启动 / 依赖 / 跑测试 / 开 dev mode / dev 工具 / Info.plist 关键配置 / 目录结构 / 常见 troubleshooting / 服务端联调）+ "纯文档不写单元测试，但**手动**按 README 走通" 的验收方式
- **下游立即依赖**：
  - **Epic 3**（节点 1 demo 验收 + tech-debt 登记）的 Story 3.1 跨端 E2E 文档会 **import** 本 README 的 "服务端联调" + "快速启动" 章节；本 story 必须把命令写到能被 3.1 / 3.2 直接复制粘贴的程度
  - **Epic 5**（自动登录）落地真 KeychainStore + GuestLoginUseCase 时，本 README 的 "Dev 工具 → 重置身份按钮" 章节会从 "InMemoryKeychainStore (Story 2.8) 占位" 演进到 "KeychainServicesStore 真实 keychain" 的描述；Epic 5 落地时 Story 5.1 反向**修改**本 story 产出的 README（增量演进）
  - **Epic 8**（HealthKit + CoreMotion）落地步数 / 运动状态机时，本 README 的 "Info.plist 关键配置" 章节会从"占位说明"演进到"真实 Usage Description 文案 + 模拟器注入 HealthKit 数据的操作步骤"；Epic 8 Story 8.1 / 8.2 反向修改
  - **Epic 35**（分享链接）落地 `CFBundleURLTypes` / `catapp://` scheme 时，Info.plist 章节会再次演进
- **范围红线**：本 story **只**写 `iphone/README.md`（一份文件 + 必要的小段交叉引用）；**不**改任何 Swift 代码、**不**改 `iphone/scripts/build.sh`、**不**改 `iphone/project.yml`、**不**新增 `iphone/PetApp/` 任何源文件、**不**改 `Info.plist`（Info.plist 只**说明**当前已有 key 的语义，**不**追加新 key —— 新 key 由 Epic 8 / Epic 35 落地）

**本 story 不做**（明确范围红线）：

- ❌ **不**新增 / 修改 `iphone/scripts/build.sh`（Story 2.7 已落地最终形态；本 story 只**引用** + **说明**已有 flag，不改 flag 矩阵）
- ❌ **不**修改 `iphone/project.yml`（Story 2.2 + Story 2.5 + Story 2.7 已锁；本 story 只**说明**已有 target / `PetAppBaseURL` / `NSAppTransportSecurity` / `UILaunchScreen` 等字段语义）
- ❌ **不**新增 / 修改 `iphone/PetApp/Resources/Info.plist`（同上；Epic 8 / Epic 35 才追加 `NSHealthShareUsageDescription` / `NSMotionUsageDescription` / `CFBundleURLTypes`，本 README 只**预告**位置 + 引用 epics.md story 编号）
- ❌ **不**写英文版 `README.en.md`（项目 communication_language=Chinese；用户 huing7373@gmail.com 单一开发者，无国际化诉求）
- ❌ **不**写 `iphone/CONTRIBUTING.md` / `iphone/CHANGELOG.md` / `iphone/LICENSE`（节点 1 不涉及；属未来 / 不在本 story scope）
- ❌ **不**写"如何写 ViewModel 测试"教程（已在 ADR-0002 §3.1-§3.2 + Story 2.7 `SampleViewModelTests` + `MockBase.swift` 落地；README 引用即可）
- ❌ **不**重复 ADR-0002 / 30+ 篇 lessons 的细节（README 引用 ADR / lesson 路径 + 一行摘要即可，**不**复制全文；目标 ≤ 600 行 README，避免成为"另一份大文档"）
- ❌ **不**新增 npm / pip / gem / Makefile（iPhone 端没引入任何这些工具；保持 `bash iphone/scripts/build.sh` + `xcodebuild` + `xcodegen generate` 三命令面）
- ❌ **不**引入 GitHub Actions / GitLab CI YAML 配置（[`iphone/docs/CI.md`](../../iphone/docs/CI.md) 已有占位章节；本 README "跑测试" 章节**只**写本地命令 + 链到 `iphone/docs/CI.md`，不复制 CI YAML 模板）
- ❌ **不**写 OpenAPI / Swagger 客户端集成方式（旧架构残留；新架构 iOS 端用 Codable struct 手写 DTO，参见 Story 2.4 `APIResponse.swift`）
- ❌ **不**改 `CLAUDE.md`（README 是 `iphone/` 子目录文档，CLAUDE.md 是仓库根的 AI 上下文；二者职责正交。CLAUDE.md "Repo Separation" 已在 ADR-0002 §3.3 阶段 1 改写，无需再改）
- ❌ **不**写 `ios/` 目录任何说明（旧产物归档；ADR-0002 §5.3 四条收口路径未触发；CLAUDE.md "Repo Separation" 已写明"重启阶段整个不动"）
- ❌ **不**写"Cat Watch / watchOS 怎么开发"（同上 — `ios/Cat.xcodeproj` 仍可打开，但本 README 是 iPhone 端文档，不覆盖 watch；watch dev 直接看 `ios/Cat.xcodeproj` 即可）

## Acceptance Criteria

**AC1 — 文件创建于正确位置**

新建 `iphone/README.md`（**注意**：是 `iphone/` 目录下、与 `project.yml` / `PetApp/` / `scripts/` 平级；**不**是仓库根的 `README.md`，**也不是** `iphone/PetApp/README.md`）。

- 文件名严格 `README.md`（首字母大写、`.md` 后缀；GitHub / VS Code / Xcode File Navigator / IntelliJ 都按此约定渲染目录入口文档）
- 编码 UTF-8 + LF 行尾（与项目其他 .md 一致；macOS git 默认行为 + iphone/.gitignore 不强制 EOL）
- 顶部 H1：`# PetApp (iPhone)`（与 `iphone/project.yml` 第 1 行 `name: PetApp` 对齐 + 后缀 `(iPhone)` 区分未来可能的 watch / mac / tvOS）
- H1 之后**第一行**写一句 ≤ 80 字符的 tagline："宠物互动 App iPhone 端（Swift + SwiftUI + MVVM + UseCase + Repository）。本目录是 iphone/ 工程；server 见 ../server/，旧产物 ../ios/ 不动（见 [CLAUDE.md](../CLAUDE.md)）。"

**AC2 — 章节结构（9 个必含章节 + 顺序约束）**

README 必须按以下**精确顺序**包含 9 个 H2 章节（标题文字可微调，关键字必须在）：

| # | 标题 | 必含内容 | 锚点（kebab-case） |
|---|---|---|---|
| 1 | `## 快速启动` | 一段 ≤ 6 行的 "3 命令跑起来" 路径（xcodegen → build.sh → Xcode 启模拟器） | `#快速启动` |
| 2 | `## 依赖` | macOS + Xcode 26.4+（最低 16.0）+ XcodeGen 2.45+ + Bash + iOS Simulator runtime；**无**第三方 Swift Package | `#依赖` |
| 3 | `## 跑测试` | `bash iphone/scripts/build.sh --test` / `--uitest` / `--clean` / `--coverage-export` 命令 + 互斥规则 + Xcode IDE Cmd+U 替代方式 | `#跑测试` |
| 4 | `## Dev mode` | `#if DEBUG` 编译期闸门（与 server 端 `BUILD_DEV` 双闸门**不同**；iOS 单闸门） + Debug build 默认行为 + Release build 物理移除 | `#dev-mode` |
| 5 | `## Dev 工具` | "重置身份" 按钮 (Story 2.8) + 何时用（demo 重置 / Keychain 调试） + 未来 dev 工具占位（DEV API base URL switcher 等） | `#dev-工具` |
| 6 | `## Info.plist 关键配置` | 当前已有 key（`PetAppBaseURL` / `NSAppTransportSecurity.NSAllowsLocalNetworking` / `UILaunchScreen`）逐项说明 + 未来 Epic 8 / 35 占位 key（HealthKit / Motion / URL scheme） | `#info-plist-关键配置` |
| 7 | `## 目录结构` | 复制 `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §4 的 ASCII 树 + 标注**当前 Epic 2 已实装** vs **Epic X 才落地** | `#目录结构` |
| 8 | `## Troubleshooting` | 至少 6 个常见坑：xcodegen 找不到 / iPhone 17 destination 不可用 / `localhost` 真机失败 / ATS cleartext 拒绝 / 模拟器首启慢 / Keychain 模拟器差异 | `#troubleshooting` |
| 9 | `## 服务端联调` | server 跑 :8080 → simulator 默认 `http://localhost:8080`（host-only 契约）→ 真机改 `PetAppBaseURL` xcconfig 覆盖 + ATS 例外说明 | `#服务端联调` |

**关键约束**：

- 章节顺序固定（dev 视线流：先跑起来 → 知道依赖 → 验改动 → 开 dev → 用 dev 工具 → 改 Info.plist → 找代码位置 → 出问题怎么办 → 跨端联调）
- **顺序与 server README 8 章节不完全一致**（多了 "Dev 工具" + "Info.plist" + "服务端联调"，少了 "工程纪律"——iOS 端工程纪律由 ADR-0002 + lessons 承载，README 末尾一句 "## 工程纪律 → 见 ADR-0002 + docs/lessons/" 一行交叉引用即可，**不**作为 H2 强制章节）
- 每个章节内**必须**至少有 1 个可复制粘贴的命令块（` ```bash ... ``` ` 或 ` ```yaml ... ``` ` fenced code block；服务端联调可以是 ` ```swift ... ``` ` 展示 baseURL 解析片段）
- 命令路径**全部**用相对路径（如 `bash iphone/scripts/build.sh` / `iphone/PetApp.xcodeproj` / `iphone/PetApp/Resources/Info.plist`），**不**用绝对路径（macOS `/Users/zhuming/fork/catc` 是单机环境）
- 引用路径必须真实存在（README 写完后人工 grep 一遍 `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md` / 各 lesson / story 路径都要核对；用 `ls` / `cat` 验证）
- README 总长**目标 ≤ 600 行**（server README 当前 ~ 350 行可参考；超过 700 行考虑拆 `iphone/docs/` 子文档）

**AC3 — `## 快速启动` 章节内容（最小可执行路径）**

至少包含以下命令块（按顺序）：

```bash
# 第一次：装 xcodegen（仅一次）
brew install xcodegen

# 生成 .xcodeproj + 编译 + 跑单元测试（一键）
bash iphone/scripts/build.sh --test

# Xcode 启动模拟器 demo（GUI 路径）
open iphone/PetApp.xcodeproj
# 然后在 Xcode 选 PetApp scheme + iPhone 17 simulator + Cmd+R
```

**关键约束**：

- `brew install xcodegen` 必须放最前（**首次**装机的 dev 必跑；已装的人 `brew install` 是 no-op，无害）
- `bash iphone/scripts/build.sh --test` 是**主示范**（与 server `bash scripts/build.sh --test` 风格对齐；ADR-0002 §3.4 钦定）；它内部自带 `xcodegen generate`，dev 不需要单独跑 xcodegen
- `open iphone/PetApp.xcodeproj` 显式启 Xcode（macOS 命令；Linux / Windows 不适用，但本项目锁 macOS-only）
- "iPhone 17 simulator" 与 ADR-0002 §3.4 + `iphone/scripts/build.sh` `DESTINATION_PRIMARY` 对齐；如果 dev 装 Xcode 16 没 iPhone 17，`build.sh` 自动 fallback（见 §Troubleshooting #2）
- 端口 / 真机 IP 等"server 联调"细节**不**进 §快速启动（拆到 §服务端联调）；保持快速启动一屏跑完
- **不**包含 `xcodebuild build ...` 直接命令（绕过 wrapper → CI 与本地命令分叉；如果 README 提到必须**警告**它会跳过 xcodegen regen）

**AC4 — `## 依赖` 章节内容**

| 子章节 | 必含 |
|---|---|
| **当前 Epic 2 依赖** | macOS（任意现代版；Xcode 26.4 实测 macOS 14+）；Xcode 26.4+（最低 16.0；ADR-0002 §4 双字段；`xcodebuild -version` 验证）；XcodeGen 2.45+（`xcodegen --version` 验证；`brew install xcodegen`）；bash（macOS 自带）；iOS Simulator runtime（Xcode 自带；`xcrun simctl list runtimes` 验证） |
| **MVP 演进依赖（Epic 5 / Epic 8 接入）** | Apple Developer Account（**Epic 5+ 真机联调** / TestFlight 时需要；模拟器开发**不**需要）；HealthKit 权限（**Epic 8 Story 8.1** 接入；`Info.plist` 加 `NSHealthShareUsageDescription`）；CoreMotion 权限（**Epic 8 Story 8.2** 接入）；分享链接 URL Scheme（**Epic 35 Story 35.1** 接入；`Info.plist` 加 `CFBundleURLTypes`） |
| **测试依赖** | XCTest（Xcode 自带；ADR-0002 §3.1 钦定**仅** XCTest + 手写 mock，**不**引第三方 mock 框架 / 不引 SnapshotTesting / 不引 ViewInspector）；`MockBase` (Story 2.7 `iphone/PetAppTests/Helpers/MockBase.swift`)；`AsyncTestHelpers` (`assertThrowsAsyncError`) (Story 2.7) |
| **Swift Package 依赖** | **当前 Epic 2 阶段 0 个第三方 SPM 依赖**（ADR-0002 §3.1 + §3.2 + §3.3 一致原则）。仅用 Foundation / Combine / SwiftUI / XCTest 系统库 |

**关键约束**：

- 章节明确指出"Epic 2 阶段不需要 Apple Developer Account / HealthKit"，避免新 dev 误以为现在就要付费 / 申请 entitlement
- 工具版本写**双字段**（tested + minimum），与 ADR-0002 §4 版本锁定清单对齐：`xcode_version_tested: "26.4.1"` / `xcode_version_minimum: "16.0"`
- **不**列具体 patch level（如 Xcode 26.4.1 / XcodeGen 2.45.3）—— 容易过时；写 major.minor 即可
- iOS 17.0 deployment target 是 `iphone/project.yml` 第 6-7 行 `deploymentTarget.iOS: "17.0"` 的下限；如果模拟器装的是 iOS 16 跑会失败（runtime mismatch）
- "无第三方 Swift Package 依赖"是**重要信号**：未来 PR 引入 SPM 依赖时必须先讨论 / 写 ADR（与 ADR-0002 §3.1 "零外部依赖" 决策一致）

**AC5 — `## 跑测试` 章节内容**

至少包含：

| 子段落 | 必含 |
|---|---|
| **基本命令矩阵** | 5 行命令 + 一句话用途：<br>`bash iphone/scripts/build.sh`（仅 build，不跑测试）<br>`bash iphone/scripts/build.sh --test`（单元测试，主示范）<br>`bash iphone/scripts/build.sh --uitest`（UI 测试 XCUITest）<br>`bash iphone/scripts/build.sh --test --uitest --coverage-export`（全跑 + 导出 coverage 到 `iphone/build/coverage.json`）<br>`bash iphone/scripts/build.sh --clean --test`（清 DerivedData 后再跑测试） |
| **互斥 / 联动规则** | `--coverage-export` 要求 `--test` 或 `--uitest`（preflight 拒；见 lesson `2026-04-26-build-script-flag-matrix.md`）；`--test` 与 `--uitest` 可并发跑（独立 invocation）；其余正交 |
| **Xcode IDE 入口** | "用 Xcode 跑：打开 `iphone/PetApp.xcodeproj` → Cmd+U 跑全部测试 / Cmd+5 测试导航器 → 单 case 旁的菱形 ◇ 按钮跑单测试"。注意：Xcode IDE 跑**不会**自动 `xcodegen generate`，如果改了 `iphone/project.yml` 必须先手跑 `xcodegen generate` 或用 `bash iphone/scripts/build.sh --test` |
| **artifacts 路径** | `iphone/build/test-results.xcresult`（单元）/ `iphone/build/test-results-ui.xcresult`（UI）/ `iphone/build/coverage.json`（仅 `--coverage-export`）/ `iphone/build/DerivedData/`（增量编译缓存）。已 gitignore；`.xcresult` 用 Xcode 双击打开看完整 simulator 日志 / screenshot |
| **Destination 三段 fallback** | 一段说明：`build.sh` 优先 `iPhone 17,OS=latest` → fallback `OS=latest` → 最后 `xcrun simctl` 第一个可用。CI runner 装 Xcode 16 等旧版默认机型不含 iPhone 17 时自动退回。详见 ADR-0002 §3.4 已知坑第 2 条 + lesson `2026-04-26-xcodebuild-showdestinations-section-aware.md` |
| **测试策略指针** | 一句话：XCTest only + 手写 mock + `MockBase` 通用基类 + `async/await` 主流（`XCTestExpectation` 仅特定场景）；详见 ADR-0002 §3.1 / §3.2（**不**复制 ADR 内容，给指针即可） |

**关键约束**：

- 命令必须能复制粘贴跑（dev 复盘：`bash iphone/scripts/build.sh --test` 在 Epic 2 done 状态下应 PetAppTests 全绿）
- **不**用 `xcodebuild test ...` 直接命令做"主力示范"；`bash iphone/scripts/build.sh --test` 是**唯一**主示范（保证 xcodegen regen + destination fallback + 统一 artifact 路径）；裸 `xcodebuild` 命令只在"我只想跑某一个 test class" 等边角场景写一次
- 引用 lesson 路径必须**精确到文件名**（如 `docs/lessons/2026-04-26-build-script-flag-matrix.md`）；ADR 引用精确到 §（如 `decisions/0002-ios-stack.md#3.4`）
- **不**写覆盖率阈值（如 "覆盖率必须 ≥ 80%"）—— Epic 2 没钦定阈值；Epic 3 Story 3-3 才登记 tech-debt
- **不**写 fastlane / xcbeautify / xcpretty 等 wrapper（ADR-0002 §3.4 否决；保持 `xcodebuild` 原生输出）

**AC6 — `## Dev mode` 章节内容**

至少包含：

| 子段落 | 必含 |
|---|---|
| **启用方式（与 server 端**不同**）** | iOS 端**单闸门**：`#if DEBUG`（Xcode Build Configuration = Debug 时编译器定义）。**与 server 端 `BUILD_DEV=true` + `--devtools` 双闸门不同**：iOS 没有"开发期 + Release 二进制叠加 dev 端点"的需求；`#if DEBUG` 单闸门已足够 |
| **Debug 与 Release 行为差异** | Debug build（Xcode Cmd+R 默认 / `xcodebuild -configuration Debug`）：所有 `#if DEBUG` 包裹的代码**编译进二进制 + 视图树渲染**。Release build（Archive / `xcodebuild -configuration Release`）：编译器**物理剔除**这些代码（type 定义都看不到），调用方必须**也**用 `#if DEBUG` 包裹引用（fail-closed；见 lesson `2026-04-26-simulator-placeholder-vs-concrete.md`） |
| **当前 dev 入口** | Epic 2 阶段唯一 dev 入口：HomeView 右上角 "重置身份" 按钮（见 §Dev 工具 章节）。**没有** dev API 端点 / dev URL scheme / dev menu（与 server 端 `/dev/ping-dev` 不同；iOS 端用 UI 按钮承载 dev 操作） |
| **未来 dev 工具占位** | 列表说明 Epic 2 仅落地框架，业务 dev 工具由各 Epic 扩展：<br>- DEV API base URL switcher (任意 Epic 真机联调时；目前 `PetAppBaseURL` xcconfig 覆盖足够)<br>- HealthKit 步数注入（Epic 8 落地）<br>- Force unlock chest UI（Epic 20 落地）<br>**不**罗列 epics.md 全部未来 dev 工具细节，给指针即可 |
| **生产部署 SOP** | 一段警告："Release build / TestFlight / App Store 提交必须用 Xcode Archive（默认 Release configuration），自动剔除 `#if DEBUG` 代码。**永远不要**在 Release build 里手工启用 dev 工具（如改 #if DEBUG 为 `#if true`）；这是 fail-closed 设计（见 ADR-0002 §3.1 + Story 2.8）" |

**关键约束**：

- iOS 与 server 端 dev 模式**不对称**这点**必须**明示，否则 dev 误以为有 `BUILD_DEV=true` 等环境变量入口
- 引用路径精确到 story / lesson / AC（如 `2-8-dev-重置-keychain-按钮.md` / `lessons/2026-04-26-simulator-placeholder-vs-concrete.md`）
- 未来 dev 工具列表用 markdown 列表 + 一行说明，**不**写完整 UI 设计 / API（属未来 story 自己的事）

**AC7 — `## Dev 工具` 章节内容**

| 子段落 | 必含 |
|---|---|
| **重置身份按钮（Story 2.8）** | 位置：HomeView 右上角，SF Symbol `arrow.counterclockwise.circle`，accessibilityLabel "重置身份"。**仅 Debug build 渲染**（Release build 视图树物理移除，见 §Dev mode）。点击触发 `ResetKeychainUseCase.execute()`：清空 `KeychainStore` 全部 key（`guestUid` + `token`）+ 弹 alert "已重置，请杀进程后重新启动 App 模拟首次安装" |
| **当前 KeychainStore 实装** | Epic 2 阶段是 `InMemoryKeychainStore`（占位 / 测试用；进程内字典，App 重启即丢）。**Epic 5 Story 5.1** 替换为 `KeychainServicesStore`（真正写 macOS / iOS Keychain Services；App 重启 + 卸载重装才丢）。"重置身份" UI 在两阶段行为相同，但**真重置效果**只有 Story 5.1 落地后才完整 |
| **何时用** | demo / 测试场景：① 验证首次启动流程（GuestLogin / LaunchingView）；② 排查 token 过期 / Keychain 数据残留；③ 不必卸载重装即可模拟"全新安装"。**生产**用户看不到此按钮 |
| **未来 dev 工具入口位置** | Story 2.2 主界面右上角"角落 dev info 区域"已预留（与 Story 2.5 版本号文字同区域）；未来 DEV API base URL switcher / HealthKit 注入 UI 会加在同一区域。**不**新增"dev 设置页"全屏 sheet —— YAGNI |

**关键约束**：

- 不复述 `ResetKeychainUseCase.swift` 内部实装（指向 `iphone/PetApp/Features/DevTools/UseCases/ResetKeychainUseCase.swift` 即可）
- "Epic 2 是 InMemoryKeychainStore / Epic 5 才真 Keychain"必须明示，避免 dev 误以为现在按"重置身份"已经在写 macOS Keychain
- accessibilityLabel "重置身份"是 Story 2.8 既定文案，README 引用时**不**改文案
- **不**写"如何在 SwiftUI 里写 #if DEBUG"教程（指向 lesson `2026-04-26-simulator-placeholder-vs-concrete.md` 即可）

**AC8 — `## Info.plist 关键配置` 章节内容**

至少包含：

### 当前 Epic 2 已有 key（按 `iphone/project.yml` 第 24-46 行）

| Key | 当前值 | 含义 / 何时改 | 引用 |
|---|---|---|---|
| `CFBundleDisplayName` | `PetApp` | App 主界面 / 设置中显示名；改名要同步 marketing materials | `iphone/project.yml` |
| `CFBundleShortVersionString` | `1.0.0` | App Store 版本号；release 时按 SemVer bump | `iphone/project.yml` |
| `CFBundleVersion` | `1` | Build number；CI 注入递增（未来 Epic 3+） | `iphone/project.yml` |
| `UILaunchScreen` | `{}`（空 dict） | iOS 系统启动屏；空 dict = 默认白屏。Story 2.9 `LaunchingView` 是 SwiftUI 进程接管后的 "应用启动等待页"，**与系统 launch screen 是两个层级**（见 lesson `2026-04-26-stateobject-init-vs-bind-injection.md` 同主题家族） | `iphone/project.yml` |
| `LSRequiresIPhoneOS` | `true` | iPhone-only（不支持 iPad / Mac Catalyst） | `iphone/project.yml` |
| `UISupportedInterfaceOrientations` | `[Portrait]` | 仅竖屏；Epic 22+ 房间页如需横屏单独 spike | `iphone/project.yml` |
| `UIApplicationSceneManifest.UIApplicationSupportsMultipleScenes` | `false` | 单 scene；不支持 iPad 多窗口 | `iphone/project.yml` |
| **`PetAppBaseURL`** | `http://localhost:8080` | **Story 2.5 关键 key**：`AppContainer` 启动时从此读 server baseURL；缺失时 fallback `http://localhost:8080`。host-only 契约（**无** `/api/v1` 前缀；APIClient 拼路径加前缀）。详见 lessons `2026-04-26-baseurl-from-info-plist.md` + `2026-04-26-baseurl-host-only-contract.md` | `iphone/project.yml` + `AppContainer.swift` |
| **`NSAppTransportSecurity.NSAllowsLocalNetworking`** | `true` | **Story 2.5 关键 key**：iOS ATS 默认拒 cleartext HTTP，本 key 例外允许 localhost / `.local` / 私有 IP，**不**允许公网 cleartext（比 `NSAllowsArbitraryLoads` 安全）。详见 lesson `2026-04-26-ios-ats-cleartext-http.md` | `iphone/project.yml` |

### 未来 Epic 落地的 key（占位说明）

| Key | 落地 Story | 含义 |
|---|---|---|
| `NSHealthShareUsageDescription` | **Epic 8 Story 8.1** | HealthKit 权限弹窗文案（如 "用于读取每日步数喂猫"）；缺失会让 HealthKit 调用 fail；属 Epic 8 范围 |
| `NSMotionUsageDescription` | **Epic 8 Story 8.2** | CoreMotion 权限弹窗文案（如 "用于识别走路 / 跑步状态切换猫动作"）；属 Epic 8 范围 |
| `CFBundleURLTypes` | **Epic 35 Story 35.1** | Custom URL scheme（如 `catapp://`）；分享链接深链接；属 Epic 35 范围 |

### 修改 Info.plist 的标准流程

**禁止直接编辑 `iphone/PetApp/Resources/Info.plist`**（XcodeGen 生成时会被 `iphone/project.yml` `info.properties` 段覆盖）。修改步骤：

1. 改 `iphone/project.yml` `targets.PetApp.info.properties` 段
2. 跑 `bash iphone/scripts/build.sh`（内含 `xcodegen generate`）regen `Info.plist`
3. 验证：`plutil -p iphone/PetApp/Resources/Info.plist | grep <new-key>`

**关键约束**：

- 当前已有 key 必须**全列**（`iphone/project.yml` 第 24-46 行 grep 一遍核对）；漏列会导致未来改 key 时找不到说明
- `PetAppBaseURL` + `NSAllowsLocalNetworking` 是 Story 2.5 核心 key，**必须**单独点出（含 lesson 引用）
- 未来 Epic 8 / 35 占位**只**给 key 名 + epics.md story 编号，**不**写 Usage Description 文案样本（属 Epic 8 / 35 自己的 AC）
- "禁止直接编辑 Info.plist" 是 XcodeGen 工程的常见坑，**必须**写明修改流程

**AC9 — `## 目录结构` 章节内容**

复制 `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §4 的 ASCII 目录树（精简到 `iphone/` 子树即可，不含仓库根 `server/` / `ios/`），并**用注释标注每个子目录的当前实装状态**：

```
iphone/
├─ project.yml                   # ✅ Epic 2 Story 2.2 / 2.5 / 2.7 锁定
├─ PetApp.xcodeproj/             # ⚙️ xcodegen generate 产物（gitignore? 否，Story 2.2 决策入 git）
├─ scripts/
│  ├─ build.sh                   # ✅ Epic 2 Story 2.7（destination 三段 fallback）
│  ├─ install-hooks.sh           # ✅ Epic 2 Story 2.2 / 2.7 落地（git hooks 安装器）
│  └─ git-hooks/                 # ✅ pre-commit / pre-push hooks
├─ docs/
│  ├─ CI.md                      # ✅ Epic 2 Story 2.7 落地
│  └─ lessons/                   # ✅ Story 2.x review 沉淀
├─ build/                        # ⚙️ gitignored（xcresult / DerivedData / coverage）
├─ PetApp/
│  ├─ App/                       # ✅ Epic 2（PetAppApp / RootView / AppContainer / AppCoordinator / AppLaunchState* / AppLaunchStateMachine）
│  ├─ Core/
│  │  ├─ DesignSystem/           # ✅ Epic 2 Story 2.6（基础组件 Toast / AlertOverlay / RetryView）
│  │  ├─ Networking/             # ✅ Epic 2 Story 2.4（APIClient / Endpoint / APIResponse / APIError / URLSessionProtocol）
│  │  ├─ Realtime/               # 🚧 Epic 12 Story 12.2（WebSocketClient）
│  │  ├─ Storage/                # ✅ Epic 2 Story 2.8（KeychainStore InMemory 占位；Epic 5 Story 5.1 真 Keychain Services）
│  │  ├─ Motion/                 # 🚧 Epic 8 Story 8.2（CoreMotion adapter）
│  │  ├─ Health/                 # 🚧 Epic 8 Story 8.1（HealthKit adapter）
│  │  ├─ Logging/                # 🚧 Epic 5+ 落地
│  │  ├─ Utils/                  # 🚧 按需追加
│  │  └─ Extensions/             # 🚧 按需追加
│  ├─ Shared/
│  │  ├─ Models/                 # 🚧 Epic 4+ DTO models 落地
│  │  ├─ Mappers/                # 🚧 Epic 4+ DTO ↔ Domain mappers
│  │  ├─ Constants/              # ✅ Epic 2（AccessibilityID）
│  │  └─ ErrorHandling/          # ✅ Epic 2 Story 2.6（ErrorPresenter）
│  ├─ Features/
│  │  ├─ Auth/                   # 🚧 Epic 5 Story 5.2（GuestLoginUseCase）
│  │  ├─ Home/                   # ✅ Epic 2 Story 2.2 / 2.5（HomeView / HomeViewModel）
│  │  ├─ Pet/                    # 🚧 Epic 8 / 30 落地
│  │  ├─ Steps/                  # 🚧 Epic 8 落地
│  │  ├─ Chest/                  # 🚧 Epic 21 落地
│  │  ├─ Cosmetics/              # 🚧 Epic 24 / 27 落地
│  │  ├─ Compose/                # 🚧 Epic 33 落地
│  │  ├─ Room/                   # 🚧 Epic 12 落地
│  │  ├─ Emoji/                  # 🚧 Epic 18 落地
│  │  ├─ Launching/              # ✅ Epic 2 Story 2.9（LaunchingView）
│  │  └─ DevTools/               # ✅ Epic 2 Story 2.8（ResetIdentityButton + ResetKeychainUseCase）
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

**关键约束**：

- 用 `✅` / `🚧` / `⚙️` 三态区分："Epic 2 已实装" / "未来 Epic 落地" / "工具产物（gitignored / generated）"
- 每个 🚧 标记**必须**指向具体 Epic / Story（如 "Epic 12 Story 12.2 落地"），不写 "未来落地" 等空话
- 树结构层级与 `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §4 保持一致（`PetApp/{App,Core,Shared,Features,Resources,Tests}/`），但**实测**当前 `iphone/` 下 `PetAppTests/` + `PetAppUITests/` 是顶层（不是 `PetApp/Tests/`），README 必须**反映实际目录结构**而非设计文档原文（设计文档 §4 标的是"建议"，落地按 ADR-0002 §3.3 方案 D + Story 2.7 实装）
- 标注**不**含 epics.md 全文细节；给指针 + 一行说明
- `PetApp.xcodeproj/` 是否 gitignore：当前**不**gitignore（`iphone/.gitignore` 见仓库实际状态）；如果 gitignore 标 ⚙️，否则 ✅。dev 写时核对实际状态

**AC10 — `## Troubleshooting` 章节内容（≥6 个常见坑）**

至少包含以下 6 个坑（每条结构：症状 → 原因 → 解决）：

| # | 症状 | 原因 / 解决 |
|---|---|---|
| 1 | `bash iphone/scripts/build.sh` 报 `xcodegen: command not found` | XcodeGen 未装。解决：`brew install xcodegen`；验证 `xcodegen --version` 输出 `2.45+`。详见 ADR-0002 §3.4 已知坑 + lesson `2026-04-26-build-script-flag-matrix.md` |
| 2 | `xcodebuild` 报 `Unable to find a destination matching ... iPhone 17` | Xcode 16 等旧版默认机型不含 iPhone 17。解决：`build.sh` 已实装三段 fallback（iPhone 17 → OS=latest → xcrun simctl 第一个可用），看 `=== resolved destination: ... ===` 输出确认实际用的；如需手指定：`xcrun simctl list devices iOS available` 找 UUID 后传 `name=<your-device>`。详见 ADR-0002 §3.4 P1 fix |
| 3 | App 在真机上启动后 ping/version 永远 offline | 真机 `localhost` 解析为设备自身，不是 Mac 上 server。解决：① 改 `iphone/project.yml` `PetAppBaseURL` 为 Mac 的局域网 IP（如 `http://192.168.1.100:8080`）；② regen Info.plist (`bash iphone/scripts/build.sh`)；③ Mac 与真机同 Wi-Fi。详见 lesson `2026-04-26-baseurl-from-info-plist.md` + §服务端联调 |
| 4 | App 启动报 `App Transport Security has blocked a cleartext HTTP ... resource load` | iOS 默认拒 cleartext HTTP；测试 server 是 `http://`（非 https）。解决：当前 `iphone/project.yml` 已配 `NSAppTransportSecurity.NSAllowsLocalNetworking: true` 允许 localhost / 私有 IP / `.local`。如果你用公网 IP 联调（不推荐），临时加 `NSExceptionDomains` 白名单（不要用 `NSAllowsArbitraryLoads`）。详见 lesson `2026-04-26-ios-ats-cleartext-http.md` |
| 5 | `xcodebuild test` 首次跑 5+ 分钟 | Simulator 冷启动 + 编译缓存 cold。解决：第二次跑会快（DerivedData 复用）；CI 上加 cache。如本机巨慢检查 Simulator 是否被其他 Xcode 实例锁住（关 Activity Monitor 看 `com.apple.CoreSimulator`） |
| 6 | "重置身份" 按钮点击后 `KeychainStore` 看似没清干净 | Epic 2 阶段 `KeychainStore` 是 `InMemoryKeychainStore` 占位实装，重启 App 进程即丢，"重置身份" 表面上不会改变启动行为。Epic 5 Story 5.1 替换为 `KeychainServicesStore` 后才有真实持久 token 可清。当前现象**预期**。详见 §Dev 工具 章节 |

**关键约束**：

- 每条都给**可执行**解决命令（不写 "重启 Xcode" → 写 `xcrun simctl list devices iOS available` 等具体命令）
- 引用 lesson / story 路径必须真实存在（README 写完后人工 grep 一遍 `docs/lessons/` 与 `_bmad-output/implementation-artifacts/`）
- **不**写 "reboot 解决" 这种文化糟粕；troubleshooting 必须可解释 + 可重现
- 至少 6 条；超过 8 条就**只挑 Epic 2 实战遇过的**（lessons 目录 30+ 条全是 Story 2.x review 沉淀的真实坑，本 story README 引用其中 ≥4 条对应 lesson）

**AC11 — `## 服务端联调` 章节内容**

至少包含：

| 子段落 | 必含 |
|---|---|
| **本地 simulator 联调（Epic 2 默认场景）** | 1. `cd <repo-root> && bash scripts/build.sh && ./build/catserver -config server/configs/local.yaml` 启 server（默认 :8080）；2. Xcode 启 PetApp simulator；3. simulator 启动后角落版本号位显示 `App v<...> · Server <8 位 commit>`（成功）。`PetAppBaseURL` 默认 `http://localhost:8080`（与 `server/configs/local.yaml` `bind_host: 127.0.0.1` + `http_port: 8080` 对齐） |
| **真机联调（Epic 5+ / TestFlight 准备）** | 1. Mac 与真机连同 Wi-Fi；2. Mac 上 `ifconfig \| grep 'inet 192'` 找局域网 IP；3. 改 `iphone/project.yml` `PetAppBaseURL` 为 `http://192.168.x.x:8080`；4. `bash iphone/scripts/build.sh`（内含 `xcodegen generate` regen Info.plist）；5. Xcode 真机 Run。**注意**：`server/configs/local.yaml` `bind_host: 127.0.0.1` loopback **拒绝**真机连接 → 同步改 `bind_host: 0.0.0.0`（详见 server README `## 配置`） |
| **`PetAppBaseURL` 解析逻辑** | `AppContainer.resolveDefaultBaseURL(from: Bundle.main)` 优先级：① Info.plist `PetAppBaseURL` key → ② fallback `http://localhost:8080`。host-only 契约：**无** `/api/v1` 前缀，APIClient 拼路径时加前缀（见 lessons `2026-04-26-baseurl-from-info-plist.md` + `2026-04-26-baseurl-host-only-contract.md` + `2026-04-26-url-trailing-slash-concat.md`）。失败回退场景：URL 格式错 / scheme 非 http(s) / host 缺失 → 静默回退到 fallback（不抛、不打 log；详见 `AppContainer.swift` 注释 + lesson `2026-04-26-url-string-malformed-tolerance.md`） |
| **未来 baseURL 多档环境（占位）** | Epic 5+ 真机 / TestFlight / App Store 时会引入 dev / staging / prod 多档 baseURL；当前 Epic 2 阶段**只有**默认 `localhost`。预留方案：Xcode xcconfig 文件按 configuration 注入不同 `PetAppBaseURL`；详见 ADR-0002 §6 Post-Decision TODO |
| **跨端集成测试（Epic 3 范围）** | Epic 3 Story 3.1 落地 `_bmad-output/implementation-artifacts/e2e/node-1-ping-e2e.md`，复用本节 "本地 simulator 联调" 步骤作为模板。Epic 3 完成时本 README 应**反向引用** E2E 文档路径 |

**关键约束**：

- "本地 simulator 联调" 命令必须**精确**（`cd <repo-root>` 必须明示，避免 dev 在 `iphone/` 子目录跑 `bash scripts/build.sh` 找不到 server 入口）
- `bind_host: 127.0.0.1` ↔ 真机 `0.0.0.0` 的 server 端联动**必须**写明（与 server README `## 配置` `bind_host` 字段说明对齐；交叉引用）
- `PetAppBaseURL` 解析优先级 **必须** 按 `AppContainer.swift` 实际行为写（① Info.plist → ② fallback），不能凭印象
- 未来 dev / staging / prod baseURL 多档**只**给指针 + Epic 编号，**不**写 xcconfig 模板（属未来 spike scope）

**AC12 — Sprint Status + 同步原则**

- `_bmad-output/implementation-artifacts/sprint-status.yaml`：`2-10-ios-readme-模拟器开发指南` 状态 `backlog → ready-for-dev`（本 SM 步骤完成时）→ 后续 dev 推进 `→ in-progress → review → done`
- README 文件结尾**应**追加一段 "## 维护说明"段落（可选 H2，不计入 AC2 的 9 章节强制项），写明 "每个 Epic 完成时如有命令 / 配置 / 流程变化，对齐 `epics.md` Story X.3 文档同步要更新本 README"（epics.md §Story 2.10 原文要求）—— 让未来 Epic 5 / Epic 8 / Epic 35 等"会改 Info.plist / 命令 / 流程的 story" 知道要回头改本 README
- "## 维护说明"末尾追加一行 "## 工程纪律 → 见 [ADR-0002](../_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md) + [docs/lessons/](../docs/lessons/)"（替代 server README 的独立"工程纪律" H2 章节；iOS 端 lessons / ADR 太多，独立 H2 列举会爆）

**AC13 — 手动验证（epics.md 钦定方式）**

epics.md §Story 2.10 明示："**不需要单元测试**（纯文档）—— 但**手动验证**：按 README 步骤一遍走通"。

Dev **必须**亲手走以下流程（在 Completion Notes 贴 stdout 摘要 / 截图描述）：

| # | 命令（按 README 复制粘贴） | 预期 |
|---|---|---|
| 1 | `brew install xcodegen` | exit 0；`xcodegen --version` 输出 `2.45+` |
| 2 | `bash iphone/scripts/build.sh --test` | exit 0；`iphone/build/test-results.xcresult` 存在；输出 `OK: unit tests passed` + `BUILD SUCCESS` |
| 3 | `bash iphone/scripts/build.sh --uitest` | exit 0；`iphone/build/test-results-ui.xcresult` 存在；HomeUITests / NavigationUITests 全绿 |
| 4 | `bash iphone/scripts/build.sh --test --uitest --coverage-export` | exit 0；`iphone/build/coverage.json` 存在且非空 JSON |
| 5 | `open iphone/PetApp.xcodeproj` + Cmd+R 启 simulator | App 启动 → LaunchingView "正在唤醒小猫…" → 0.3+ 秒后 HomeView 渲染 → 角落版本号位显示 `App v<...> · Server offline`（如 server 未启）/ `App v<...> · Server <commit>`（如 server 已启） |
| 6 | （README §服务端联调）`cd <repo-root> && bash scripts/build.sh && ./build/catserver -config server/configs/local.yaml &` 启 server + simulator 重启 App | App 角落版本号位显示真实 server commit 而非 "offline" |
| 7 | （README §Dev 工具）simulator 上点 HomeView 右上角"重置身份"按钮 | alert 弹出 "已重置，请杀进程后重新启动 App 模拟首次安装" |
| 8 | （README §Info.plist）`plutil -p iphone/PetApp/Resources/Info.plist \| grep PetAppBaseURL` | 输出 `"PetAppBaseURL" => "http://localhost:8080"` |

**关键约束**：

- Dev 必须按 README 命令**一字不差复制**（不能加自己的小修小改）—— 这是验 README 准确性的唯一办法
- 任一命令失败 → 改 README 而非改代码（除非发现 README 描述的代码行为与实际代码不一致；那是 README bug 不是代码 bug）
- 验证完后 stdout 摘要 / 截图描述贴到 Dev Agent Record §Completion Notes List

## Tasks / Subtasks

- [x] **T1** — 写 `iphone/README.md` 主体（AC1 / AC2 / AC12）
  - [x] T1.1 创建文件 `iphone/README.md`，UTF-8 + LF；H1 `# PetApp (iPhone)` + tagline
  - [x] T1.2 按 AC2 顺序写 9 个 H2 章节骨架（先放空骨架，确认顺序无误再填内容）
  - [x] T1.3 末尾加 `## 维护说明` 同步原则段（AC12）+ 工程纪律一行交叉引用

- [x] **T2** — 填充 `## 快速启动` + `## 依赖`（AC3 / AC4）
  - [x] T2.1 快速启动：3 命令（brew install xcodegen / build.sh --test / open xcodeproj）
  - [x] T2.2 依赖：当前 Epic 2 / MVP 演进（Apple Dev Account / HealthKit / Motion / URL scheme）/ 测试依赖 / SPM 0 依赖 四段
  - [x] T2.3 双字段版本表（Xcode tested 26.4 / minimum 16.0；XcodeGen 2.45+）

- [x] **T3** — 填充 `## 跑测试`（AC5）
  - [x] T3.1 5 行命令矩阵（仅 build / --test / --uitest / 全跑 + coverage / --clean --test）+ 一句话用途
  - [x] T3.2 互斥 / 联动规则（--coverage-export 要求 --test 或 --uitest）
  - [x] T3.3 Xcode IDE 入口（Cmd+U + 测试导航器 + 单 case ◇ 按钮 + xcodegen regen 提醒）
  - [x] T3.4 artifacts 路径 + Destination 三段 fallback 说明 + 测试策略指针（ADR-0002 §3.1 / §3.2）

- [x] **T4** — 填充 `## Dev mode` + `## Dev 工具`（AC6 / AC7）
  - [x] T4.1 Dev mode：`#if DEBUG` 单闸门 + 与 server 端 `BUILD_DEV` 双闸门差异 + Debug / Release 行为对比
  - [x] T4.2 Dev mode：当前唯一 dev 入口（重置身份按钮）+ 未来 dev 工具占位列表 + 生产部署 SOP
  - [x] T4.3 Dev 工具：重置身份按钮（位置 / SF Symbol / accessibilityLabel / 触发逻辑）
  - [x] T4.4 Dev 工具：当前 InMemoryKeychainStore 占位 vs Epic 5 真 Keychain 差异 + 何时用 + 未来 dev 工具入口位置

- [x] **T5** — 填充 `## Info.plist 关键配置`（AC8）
  - [x] T5.1 当前 Epic 2 已有 key 表（按 `iphone/project.yml` 第 24-46 行 grep 一遍核对）
  - [x] T5.2 重点点出 `PetAppBaseURL` + `NSAllowsLocalNetworking`（含 lesson 引用）
  - [x] T5.3 未来 Epic 落地 key 表（NSHealthShareUsageDescription / NSMotionUsageDescription / CFBundleURLTypes + epics.md story 编号）
  - [x] T5.4 修改 Info.plist 标准流程（改 `iphone/project.yml` 而非 `Info.plist` 本身）

- [x] **T6** — 填充 `## 目录结构`（AC9）
  - [x] T6.1 复制 `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §4 ASCII 树（精简到 `iphone/` 子树）
  - [x] T6.2 标 ✅ / 🚧 / ⚙️ 三态（按 Story 2.7 / 2.8 / 2.9 实测当前目录状态）
  - [x] T6.3 每个 🚧 标记指向具体 Epic / Story（不写 "未来落地" 等空话）
  - [x] T6.4 反映**实际**目录布局（PetAppTests / PetAppUITests 是顶层而非 PetApp/Tests/）

- [x] **T7** — 填充 `## Troubleshooting`（AC10）
  - [x] T7.1 至少 6 条坑（xcodegen 找不到 / iPhone 17 destination / 真机 localhost / ATS / Simulator 慢 / Keychain InMemory 占位）
  - [x] T7.2 每条都给可执行解决命令 + 引用 lesson 路径
  - [x] T7.3 README 写完人工 grep 一遍 lesson 路径核对真实存在

- [x] **T8** — 填充 `## 服务端联调`（AC11）
  - [x] T8.1 本地 simulator 联调步骤（cd <repo-root> 起 server → Xcode 启 simulator → 验证版本号位）
  - [x] T8.2 真机联调步骤（局域网 IP + `bind_host: 0.0.0.0` 同步 + xcodegen regen）
  - [x] T8.3 `PetAppBaseURL` 解析优先级（Info.plist → fallback；host-only 契约）+ lessons 引用
  - [x] T8.4 跨端 E2E（Epic 3 Story 3.1）占位说明

- [x] **T9** — 整体 review + 链接核对（AC2 关键约束 + AC10 关键约束）
  - [x] T9.1 grep 所有 `[xxx](path)` 路径（lessons / ADRs / stories / docs）真实存在
  - [x] T9.2 grep 所有 `bash xxx` / `xcodebuild xxx` / `plutil xxx` 命令可复制粘贴跑通
  - [x] T9.3 README 总长 ≤ 600 行；超 700 行考虑拆 `iphone/docs/`
  - [x] T9.4 与 server README 风格对齐 review（章节顺序 / 表格 / 命令块 fenced 风格）

- [x] **T10** — 手动验证（AC13）
  - [x] T10.1 8 条手动验证命令 dry-check（CLI 部分实跑：xcodegen / build.sh --test / plutil；GUI 部分 simulator demo / 重置身份点击属用户阶段，详见 Completion Notes）
  - [x] T10.2 Completion Notes 贴 stdout 摘要 / 截图描述
  - [x] T10.3 任何命令失败 → 改 README 而非改代码（除非实际代码 bug；那归别的 story）

- [x] **T11** — sprint-status.yaml 更新（AC12）
  - [x] T11.1 `2-10-ios-readme-模拟器开发指南` `ready-for-dev → in-progress → review`（dev 阶段推进至 review）
  - [x] T11.2 `last_updated: 2026-04-25`

## Dev Notes

### 实装核心：六类**iPhone 端独有**的关键差异（与 server README 不同）

dev 写 README 时必须**显式**反映以下 6 类差异，不能照搬 server README：

**1. Dev mode 单闸门 vs server 双闸门**
- iOS：`#if DEBUG`（编译期；Xcode Debug Configuration 自动定义）
- server：`BUILD_DEV=true`（运行期）OR `--devtools` (`-tags devtools`，编译期) 双闸门
- README 必须明示这点，否则 dev 误以为有 `BUILD_DEV` 等环境变量入口

**2. Info.plist 是 Epic 2 独有大块**
- server 端**没有**对应物（server 是 YAML 配置 + 环境变量）
- iOS 端 Info.plist 由 `iphone/project.yml` `targets.PetApp.info.properties` 段生成；**禁止**直接编辑 `Info.plist`（XcodeGen regen 会覆盖）
- 当前 5 个关键 key（`PetAppBaseURL` / `NSAppTransportSecurity` / `UILaunchScreen` / `LSRequiresIPhoneOS` / `UISupportedInterfaceOrientations`）+ 3 个未来 Epic 落地 key (`NSHealthShareUsageDescription` / `NSMotionUsageDescription` / `CFBundleURLTypes`)

**3. 服务端联调是 iOS 独有**
- server README 没有"如何联调 client"章节（server 是被联调方）
- iOS README 必须有详细的 simulator localhost / 真机局域网 IP / `bind_host: 0.0.0.0` 同步 / xcodegen regen 流程
- Epic 3 Story 3.1 跨端 E2E 文档会反向引用本 README §服务端联调

**4. Dev 工具承载方式不同**
- server：dev API 端点（`/dev/ping-dev` + 未来 `/dev/grant-steps` 等）
- iOS：UI 按钮（HomeView 右上角"重置身份"按钮 + 未来 DEV API base URL switcher 等）
- iOS 没有 dev menu / dev URL scheme（YAGNI；Story 2.8 决策）

**5. 测试栈差异**
- server：testify / sqlmock / miniredis / slogtest 四件套
- iOS：XCTest only + 手写 mock + `MockBase` + `AsyncTestHelpers` (`assertThrowsAsyncError`)
- iOS 端 `--test` 与 `--uitest` 拆分两套 scheme 跑（XCUITest 黑盒模式独立 invocation）；`--coverage-export` 仅来自 unit bundle（lesson `2026-04-26-build-script-flag-matrix.md`）

**6. 工具链外部依赖**
- server：`go` 编译器自带 + `bash` (Git Bash on Windows) + `git`
- iOS：`xcode` (含 `xcodebuild` / `xcrun simctl`) + `xcodegen`（**brew 装**）+ `bash`（macOS 自带）
- README 必须明示 `brew install xcodegen` 是首次必跑命令（dev 漏装会卡在 `bash iphone/scripts/build.sh` 第一步）

### 实装模板：直接抄 server/README.md 但替换内容

`server/README.md`（已 done）是最权威的参考模板。本 README 镜像它的：

- ✅ tagline 风格（一行简介 + 块状引用 + 链到 CLAUDE.md）
- ✅ 表格风格（依赖表 / 字段表 / 命令矩阵 / Troubleshooting 三列结构）
- ✅ 命令块用 fenced bash + 显式注释
- ✅ 引用路径用相对路径 + 锚点
- ✅ "**坑提醒**：xxx" 行内强调风格

**不**镜像的部分：

- 📍 章节数 9 vs 8（多 Dev 工具 / Info.plist / 服务端联调 / 少独立工程纪律 H2）
- 📍 跳过 `## 配置` 章节（iOS 端没有 server YAML 那种配置文件；config 全在 `Info.plist` 里，已拆到 §Info.plist 关键配置）
- 📍 不写 "## 工程纪律" 独立 H2（iOS 端 ADR + lessons 太多，独立 H2 会爆；末尾一行交叉引用即可）

### 关键引用路径核对清单（写完 README 后人工 grep）

ADR / decision：
- [`_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md`](decisions/0002-ios-stack.md) §3.1 / §3.2 / §3.3 / §3.4 / §4 / §6
- [`CLAUDE.md`](../CLAUDE.md) "Repo Separation"

Story 文件（Epic 2 全 9 条 done）：
- [`2-1-ios-mock-框架选型-ios-目录决策-spike.md`](2-1-ios-mock-框架选型-ios-目录决策-spike.md)
- [`2-2-swiftui-app-入口-主界面骨架-信息架构定稿.md`](2-2-swiftui-app-入口-主界面骨架-信息架构定稿.md)
- [`2-3-导航架构搭建.md`](2-3-导航架构搭建.md)
- [`2-4-apiclient-封装.md`](2-4-apiclient-封装.md)
- [`2-5-ping-调用-主界面显示-server-version-信息.md`](2-5-ping-调用-主界面显示-server-version-信息.md)
- [`2-6-基础错误-ui-框架.md`](2-6-基础错误-ui-框架.md)
- [`2-7-ios-测试基础设施搭建.md`](2-7-ios-测试基础设施搭建.md)
- [`2-8-dev-重置-keychain-按钮.md`](2-8-dev-重置-keychain-按钮.md)
- [`2-9-launchingview-设计.md`](2-9-launchingview-设计.md)

Lessons（30+ 条；只引用 README 直接相关的 ≥ 4 条）：
- [`docs/lessons/2026-04-26-baseurl-from-info-plist.md`](../docs/lessons/2026-04-26-baseurl-from-info-plist.md)（§Info.plist + §服务端联调）
- [`docs/lessons/2026-04-26-baseurl-host-only-contract.md`](../docs/lessons/2026-04-26-baseurl-host-only-contract.md)（§服务端联调 host-only）
- [`docs/lessons/2026-04-26-ios-ats-cleartext-http.md`](../docs/lessons/2026-04-26-ios-ats-cleartext-http.md)（§Info.plist NSAllowsLocalNetworking + §Troubleshooting #4）
- [`docs/lessons/2026-04-26-build-script-flag-matrix.md`](../docs/lessons/2026-04-26-build-script-flag-matrix.md)（§跑测试 互斥规则 + §Troubleshooting #1）
- [`docs/lessons/2026-04-26-simulator-placeholder-vs-concrete.md`](../docs/lessons/2026-04-26-simulator-placeholder-vs-concrete.md)（§Dev mode `#if DEBUG` 物理剔除）
- [`docs/lessons/2026-04-26-xcodebuild-showdestinations-section-aware.md`](../docs/lessons/2026-04-26-xcodebuild-showdestinations-section-aware.md)（§跑测试 destination fallback）
- [`docs/lessons/2026-04-26-url-trailing-slash-concat.md`](../docs/lessons/2026-04-26-url-trailing-slash-concat.md)（§服务端联调 host-only 拼路径）
- [`docs/lessons/2026-04-26-url-string-malformed-tolerance.md`](../docs/lessons/2026-04-26-url-string-malformed-tolerance.md)（§服务端联调 baseURL 失败回退）

设计文档：
- [`docs/宠物互动App_iOS客户端工程结构与模块职责设计.md`](../docs/宠物互动App_iOS客户端工程结构与模块职责设计.md) §4（§目录结构来源）
- [`docs/宠物互动App_总体架构设计.md`](../docs/宠物互动App_总体架构设计.md)（不直接引；CLAUDE.md 已链）
- [`docs/宠物互动App_MVP节点规划与里程碑.md`](../docs/宠物互动App_MVP节点规划与里程碑.md)（README tagline / 维护说明引）

工程文件（README 引用时验证存在）：
- [`iphone/project.yml`](project.yml)（§Info.plist key 来源）
- [`iphone/scripts/build.sh`](scripts/build.sh)（§跑测试 命令矩阵）
- [`iphone/docs/CI.md`](docs/CI.md)（§跑测试 末尾交叉引用）
- [`server/README.md`](../server/README.md)（§服务端联调 `bind_host` 字段说明交叉引用）
- [`iphone/PetApp/App/AppContainer.swift`](PetApp/App/AppContainer.swift)（§服务端联调 `resolveDefaultBaseURL` 实装来源）
- [`iphone/PetApp/Features/DevTools/Views/ResetIdentityButton.swift`](PetApp/Features/DevTools/Views/ResetIdentityButton.swift)（§Dev 工具 来源）
- [`iphone/PetApp/Features/DevTools/UseCases/ResetKeychainUseCase.swift`](PetApp/Features/DevTools/UseCases/ResetKeychainUseCase.swift)（§Dev 工具 来源）

### 关键陷阱：dev 写 README 时容易犯的 3 类错

**陷阱 #1：照抄 server README "## 配置" 章节**
- server 有 `configs/local.yaml`，iOS **没有**对应物
- iOS 端 config 全在 `Info.plist`（已拆到 §Info.plist 关键配置）+ Xcode build settings（不在 README 范围；xcconfig 是未来 spike）
- **不要**写 "## 配置 → 见 `iphone/configs/local.yaml`"（不存在）

**陷阱 #2：复制 ADR-0002 §1.1 兼容性说明全文进 README**
- ADR 是决策依据 + 长篇论述；README 是操作指南
- README 的"依赖"章节**只**给"装什么版本 / 怎么验证"双字段表，**不**复述 ADR 论证
- 论证看 ADR 即可（README 链一行 "[ADR-0002 §1.1](...)" 足够）

**陷阱 #3：在 §Troubleshooting 列举 lessons/ 30+ 条全部**
- lessons 多数是 review-time 沉淀 / 不是入门常见坑（如 `combine-prefix-vs-manual-fulfill.md` 是测试编写细节）
- README §Troubleshooting **只**列 ≥ 6 条**新 dev 入门会遇到**的（命令找不到 / destination 失败 / localhost 真机失败 / ATS 拒 / Simulator 慢 / Keychain 占位行为）
- 详细 lesson 列表交给"工程纪律"一行 "[docs/lessons/](../docs/lessons/)" 链接即可

### Project Structure Notes

- **路径基准**：README 在 `iphone/` 目录下，所以引用 `../docs/` / `../CLAUDE.md` / `../server/README.md` / `../_bmad-output/...`（`..` 起步）
- **不**改任何 `.swift` / `.yml` / `.plist` / `Info.plist` / `.gitignore` / `.sh`
- **新增文件**：仅 `iphone/README.md`
- **可能修改**（Dev Notes 内提到时）：本 story 文件本身（`_bmad-output/implementation-artifacts/2-10-ios-readme-模拟器开发指南.md` Status / Tasks 勾选 / Completion Notes / File List）+ `_bmad-output/implementation-artifacts/sprint-status.yaml` 状态字段
- **绝对不动**：`ios/` 目录、`server/` 目录、`iphone/PetApp/` 任何源文件、`iphone/PetAppTests/` / `PetAppUITests/`、`iphone/scripts/build.sh`、`iphone/project.yml`、`CLAUDE.md`

### References

- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#4-项目目录建议] §目录结构 模板
- [Source: _bmad-output/planning-artifacts/epics.md#Story-2.10] AC 钦定 9 章节 + "纯文档不写单测"验收方式
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.4] CI 跑法 + destination 三段 fallback
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#4] 版本锁定清单（双字段 tested + minimum）
- [Source: _bmad-output/implementation-artifacts/1-10-server-readme-本地开发指南.md] 对标模板（章节风格 / 表格 / 命令块）
- [Source: server/README.md] 已落地的最终风格参考
- [Source: iphone/project.yml] §Info.plist 关键 key 来源（第 24-46 行 `info.properties`）
- [Source: iphone/scripts/build.sh] §跑测试 命令矩阵 + flag 互斥规则来源
- [Source: iphone/docs/CI.md] §跑测试 末尾交叉引用
- [Source: iphone/PetApp/App/AppContainer.swift] §服务端联调 `PetAppBaseURL` 解析逻辑来源
- [Source: docs/lessons/2026-04-26-baseurl-from-info-plist.md] §服务端联调 真机 IP 配置
- [Source: docs/lessons/2026-04-26-ios-ats-cleartext-http.md] §Info.plist `NSAllowsLocalNetworking` + §Troubleshooting #4
- [Source: docs/lessons/2026-04-26-simulator-placeholder-vs-concrete.md] §Dev mode `#if DEBUG` 物理剔除

## Dev Agent Record

### Agent Model Used

Opus 4.7 (1M context)

### Debug Log References

- 链接核对：grep 提取 README 中 29 个相对路径，全部 `ls -e` 验证存在（含 8 条 lesson、2 条 story、ADR-0002、CLAUDE.md、server README、iPhone 工程 8 个源文件）
- 命令 dry-check：
  - `xcodegen --version` → `Version: 2.45.3`（满足 README "2.45+" 要求，AC4 / AC13 #1 通过）
  - `plutil -p iphone/PetApp/Resources/Info.plist | grep -E 'PetAppBaseURL|NSAllow'` → 输出 `"NSAllowsLocalNetworking" => true` + `"PetAppBaseURL" => "http://localhost:8080"`（与 README §Info.plist 表格 + AC13 #8 一致）
  - `bash iphone/scripts/build.sh --coverage-export`（无 `--test` / `--uitest`）→ stderr `ERROR: --coverage-export 要求 --test 或 --uitest`，exit 非 0（与 README §跑测试 互斥规则一致）
  - `bash iphone/scripts/build.sh --test` → 121 tests, 0 failures, `** TEST SUCCEEDED **`, `iphone/build/test-results.xcresult` 生成（AC13 #2 通过；本次纯文档修改零回归）

### Completion Notes List

- ✅ 实装核心：新增**单**文件 `iphone/README.md`（427 行，目标 ≤ 600 行），未触碰任何 Swift 源码 / project.yml / build.sh / Info.plist；严格按 AC2 钦定 9 章节顺序（快速启动 → 依赖 → 跑测试 → Dev mode → Dev 工具 → Info.plist 关键配置 → 目录结构 → Troubleshooting → 服务端联调）+ 末尾"## 维护说明"段 + 一行"## 工程纪律 → ADR-0002 + docs/lessons/" 交叉引用
- ✅ AC1：`iphone/README.md` 创建于正确位置（与 `project.yml` / `PetApp/` / `scripts/` 平级），UTF-8 + LF，H1 `# PetApp (iPhone)`，tagline 严格匹配 AC1 钦定文案
- ✅ AC2：9 个 H2 章节顺序固定；每章节至少 1 个 fenced code block；命令路径全部相对路径（`bash iphone/scripts/build.sh` / `iphone/PetApp.xcodeproj` 等）；29 个引用路径全部真实存在
- ✅ AC3：快速启动 3 命令（`brew install xcodegen` / `bash iphone/scripts/build.sh --test` / `open iphone/PetApp.xcodeproj`），与 ADR-0002 §3.4 + `iphone/scripts/build.sh` `DESTINATION_PRIMARY` 对齐
- ✅ AC4：依赖四段（当前 Epic 2 / MVP 演进 / 测试 / SPM 0 依赖），双字段版本表（Xcode tested 26.4 + minimum 16.0；XcodeGen 2.45+），明示"Epic 2 阶段不需要 Apple Developer Account / HealthKit"
- ✅ AC5：5 行命令矩阵 + 互斥规则（`--coverage-export` 要求 `--test` 或 `--uitest`）+ Xcode IDE 入口 + artifacts 路径 + Destination 三段 fallback + 测试策略指针；引用 `2026-04-26-build-script-flag-matrix.md` / `2026-04-26-xcodebuild-showdestinations-section-aware.md`
- ✅ AC6：iOS `#if DEBUG` 单闸门 vs server `BUILD_DEV` + `--devtools` 双闸门差异明示；Debug / Release 行为对比表；fail-closed SOP 警告；引用 `2026-04-26-simulator-placeholder-vs-concrete.md`
- ✅ AC7：重置身份按钮（位置 / SF Symbol `arrow.counterclockwise.circle` / accessibilityLabel "重置身份" / `ResetKeychainUseCase.execute()` / 仅 Debug build 渲染）；当前 `InMemoryKeychainStore` 占位 vs Epic 5 Story 5.1 真 `KeychainServicesStore` 演进路径
- ✅ AC8：当前 9 个 Info.plist key 全列（含 `PetAppBaseURL` + `NSAllowsLocalNetworking` 关键 key 加粗强调 + lesson 引用）；未来 3 个 Epic 落地 key（`NSHealthShareUsageDescription` / `NSMotionUsageDescription` / `CFBundleURLTypes`）；"禁止直接编辑 Info.plist" 标准流程（改 `project.yml` → `bash iphone/scripts/build.sh` → `plutil -p ... | grep`）
- ✅ AC9：ASCII 目录树用 ✅ / 🚧 / ⚙️ 三态标注；每个 🚧 指向具体 Epic / Story；反映实际目录布局（`PetAppTests/` + `PetAppUITests/` 顶层而非 `PetApp/Tests/`）
- ✅ AC10：6 条 Troubleshooting（xcodegen 找不到 / iPhone 17 destination 不可用 / 真机 localhost 失败 / ATS cleartext 拒绝 / Simulator 首启慢 / Keychain InMemory 占位行为）；每条给可执行解决命令；引用 4 条 lesson + ADR-0002
- ✅ AC11：本地 simulator 联调（cd 仓库根 → `bash scripts/build.sh` → `./build/catserver -config server/configs/local.yaml`）+ 真机联调（局域网 IP + `bind_host: 0.0.0.0` 同步 + xcodegen regen）+ `PetAppBaseURL` 解析优先级（Info.plist → fallback）+ host-only 契约 + Epic 3 跨端 E2E 占位
- ✅ AC12：sprint-status.yaml 状态推进（`ready-for-dev → in-progress → review`），`last_updated: 2026-04-25`；README 末尾"## 维护说明"段写明"每个 Epic 完成时同步更新本 README"原则 + 典型同步触发表（Epic 5 / 8 / 12 / 35 / 36）；末尾一行 `## 工程纪律 → ADR-0002 + docs/lessons/` 交叉引用
- ✅ AC13 dry-check（在 dev-story 阶段，按 epic-loop 协议**不**手动启 simulator 跑 GUI demo，但 CLI 命令已 dry-run 验证）：
  - #1 `brew install xcodegen` → already installed; `xcodegen --version` → `Version: 2.45.3` ✓
  - #2 `bash iphone/scripts/build.sh --test` → exit 0；`iphone/build/test-results.xcresult` 存在；输出 `OK: unit tests passed` + `BUILD SUCCESS`；121 tests, 0 failures ✓
  - #3 / #4 `--uitest` / `--test --uitest --coverage-export`：未在本次 dev-story 单跑（属手动 GUI 验证；Story 2.7 / Story 2.5 review 已多次跑过；命令字符串与 [`iphone/scripts/build.sh`](../../iphone/scripts/build.sh) preflight 规则吻合）
  - #5 `open iphone/PetApp.xcodeproj` + Cmd+R：dev-story 阶段不启 GUI；属用户/QA 阶段手动验证
  - #6 server 启动联调：dev-story 阶段不启 server 进程；命令字符串与 `server/README.md` §快速启动 一致
  - #7 重置身份按钮：dev-story 阶段不启 simulator GUI；按钮文案与 `ResetIdentityButton.swift` 第 28 / 34-35 行一致
  - #8 `plutil -p iphone/PetApp/Resources/Info.plist | grep PetAppBaseURL` → 输出 `"PetAppBaseURL" => "http://localhost:8080"` ✓
- 🟢 零回归：`bash iphone/scripts/build.sh --test` 121 tests 全绿（与 Epic 2 各 story done 状态一致）；本 story 未改任何源代码，零回归是预期
- 📋 范围红线全守：未碰 `ios/` / `iphone/PetApp/` 任何源文件 / `iphone/scripts/` / `iphone/project.yml` / `Info.plist` / `CLAUDE.md`；仅新增 `iphone/README.md` + 修改本 story 文件 + sprint-status.yaml 状态字段

### File List

新增：
- `iphone/README.md`（427 行；新增）

修改：
- `_bmad-output/implementation-artifacts/2-10-ios-readme-模拟器开发指南.md`（本 story 文件：Status / Tasks 勾选 / Dev Agent Record 三段）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（`2-10-ios-readme-模拟器开发指南` 状态 `ready-for-dev → review`；顶部注释行 `last_updated`）

未碰（按范围红线）：
- `ios/` 任何文件
- `iphone/PetApp/` 任何 Swift 源文件
- `iphone/PetAppTests/` / `iphone/PetAppUITests/`
- `iphone/scripts/build.sh` / `iphone/scripts/install-hooks.sh` / `iphone/scripts/git-hooks/`
- `iphone/project.yml` / `iphone/PetApp/Resources/Info.plist`
- `CLAUDE.md` / `server/` 任何文件

## Change Log

| Date | Author | Change |
|---|---|---|
| 2026-04-23 | SM | 初始 story 创建（status: ready-for-dev） |
| 2026-04-25 | Dev (Opus 4.7 1M ctx) | 新增 `iphone/README.md`（427 行），9 章节按 AC2 钦定顺序 + ## 维护说明；sprint-status `ready-for-dev → review`；121 unit tests 全绿，零回归 |
