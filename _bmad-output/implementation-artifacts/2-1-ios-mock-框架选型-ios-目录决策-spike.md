# Story 2.1: iOS mock 框架选型 + ios/ 目录决策 spike

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iOS 开发,
I want 在写第一行 SwiftUI 代码前确定 mock 工具栈和 ios/ 目录的去留,
so that 后续 Epic 写测试 + 集成代码时不被工具 / 目录改动反复打断.

## 故事定位（Spike 性质）

这是 Epic 2 的**起手 spike**，与 Epic 1 的 Story 1.1（server 测试栈选型）等价。**唯一交付物是一份决策文档** `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md`。本 story 不写任何 SwiftUI / Swift 生产代码、不修改 `ios/project.yml`、不动 Xcode 工程。后续 Story 2.2（App 入口）/ 2.3（导航架构）/ 2.4（APIClient）/ 2.7（测试基础设施）按本 spike 选定的工具栈与目录方案直接落地，不再二次讨论。

## Acceptance Criteria

**AC1** — 输出 `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md`，按 ADR 模板（参考 `0001-test-stack.md` 头部：`# ADR-0002: ... / Status / Date / Decider / Supersedes / Related Stories`）起手，包含下列 4 类决策。每项必须含：**候选清单 / 选定项 / 理由（≥ 3 条）/ 被否候选的否决理由**。

1. **iOS mock 框架选型**：`XCTest only` / `Mockingbird` / `Cuckoo` / `Swift Mocks` 中选一。
2. **异步测试方案**：明确"用 `async/await` 直接 await 的纯 Swift Concurrency 方案" vs "`XCTestExpectation` + 回调" 的取舍 —— 当前架构（`docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §18.1）首选 `async/await`，决策文档需说明何时仍要用 `XCTestExpectation`（如观察 `@Published` 状态变化、SwiftUI 视图状态等场景）。
3. **iPhone App 工程目录方案**：在以下**四选一**并落地清理范围（dev 阶段在用户增量决策下追加方案 D）：
   - **方案 A**：复用 `ios/Cat.xcodeproj` 改名为 `PetApp.xcodeproj`，在原工程内调整目录到 `App/Core/Shared/Features/Resources/Tests/`（按 iOS 架构设计 §4），保留旧 schema 历史。
   - **方案 B**：在 `ios/` 下**新建独立** `PetApp.xcodeproj`，把 `CatWatch*` 等 watchOS 残留**移到 `watch/` 子目录归档**（CLAUDE.md 明确 watch/ 当前重启阶段暂不考虑）。
   - **方案 C**：`ios/` **完全 wipe 重建**，仅保留 `INSPIRATION_LIBRARY.md` 等纯文档，所有 Xcode 工程从头生成。
   - **方案 D**（dev 阶段按用户决策追加）：在仓库根**新建顶级目录** `iphone/`（与 `server/` / `ios/` 平级），iPhone App 在新目录从 0 建立；`ios/` 整个**原封不动**作为旧产物归档。
4. **CI 跑法**：给出明确的 `xcodebuild test` 命令模板（含 `-scheme` / `-destination 'platform=iOS Simulator,name=<机型>,OS=<版本>'` / `-resultBundlePath`），并说明本地与 CI 的对齐方式（与 server 端 `bash scripts/build.sh --test` 等价的 iPhone 端入口；具体路径取决于决策项 3 的选定方案 —— 如选 A/B/C 则在 `ios/scripts/build.sh`，如选 D 则在新顶级目录如 `iphone/scripts/build.sh`）。

**AC2** — 决策文档在每个决策末尾必须列出**已知坑 / 缓解措施**至少 1 条（参考 0001-test-stack.md §3.1 的 "已知坑" 段落体例）。例如：mock 框架若选 Mockingbird，要列出"对 generic protocol 的 codegen 限制"；目录方案若选 B，要列出"watchOS 后续恢复时 PetApp 工程是否能直接 link CatShared package 的回归路径"。

**AC3** — 决策文档末尾给出**版本锁定清单**（YAML / 表格形式），包含：

- iOS deployment target（建议保持当前 `project.yml` 的 `iOS 17.0`，除非有充分理由调整）
- Xcode 版本（**实测当前机器版本** + **理论兼容下限** 双字段；旧 `project.yml` 的 `xcodeVersion: "16.0"` 是历史声明，不是当前基线）
- Swift 工具链版本（observed + tools_version 双字段）
- 选定的 mock 框架版本（如有外部依赖）
- XcodeGen 版本（如保留使用）
- swift-format 版本（**必须锁定具体版本号**，不允许 "latest" 浮动）
- iPhone Simulator 默认机型与 OS 版本（CI 用，建议同时给 primary + fallback 两套，覆盖 contributor 不同 Xcode 版本机型差异）

**AC4** — `ios/` 目录方案决策必须明确**当前残留产物清单的处置**（见 Dev Notes "iOS 当前残留产物清单"段）。每个残留必须显式列出"保留 / 改名 / 移到 watch/ / 删除"四选一，不允许"看情况"或"待定"。

**AC5** — 决策在 Epic 2 后续 story 落地时**直接采用，不再二次讨论**：

- Story 2.2 按本 spike 选定的目录方案建立 `PetAppApp.swift` + `RootView.swift` 等入口文件。
- Story 2.4 / 2.5 测试用本 spike 选定的 mock 框架。
- Story 2.7 测试基础设施按本 spike 选定的 CI 命令落地 `ios/scripts/build.sh`（如选 A/B/C）或 `iphone/scripts/build.sh`（如选 D）等价入口，建立第一条业务相关 mock 单元测试（满足 AR27 done 标准的模板示范）。

**AC6** — 本 story **不产出任何 `.swift` 代码**、**不修改 `ios/project.yml`**、**不删除任何 `ios/` 下文件**、**不创建 `iphone/`（如选方案 D）实体目录或 `.xcodeproj`**、**不跑 `xcodegen generate`** 或 `xcodebuild`。所有目录清理 / 工程重建动作由 Story 2.2 实装时一并执行（按本 spike 选定的方案）。**例外**：本 ADR commit 可同步修改 repo 内的非 `ios/` 文档（如 CLAUDE.md），用于消除"ADR 决策" vs "其它 master 文档"的合同冲突（codex review F2 P1 fix 范围内允许）。

## Tasks / Subtasks

- [x] **T1**：建立决策文档骨架（AC1）
  - [x] T1.1 确认目录 `_bmad-output/implementation-artifacts/decisions/` 已存在（0001-test-stack.md 在内），无需新建
  - [x] T1.2 新建 `0002-ios-stack.md`，按 ADR 模板（参考 0001-test-stack.md 头部）起手
  - [x] T1.3 写 `## 1. Context` 段，引用 CLAUDE.md "Tech Stack（新方向）" + iOS 架构设计 §3 + §4 + §18.1，说明本 spike 锁定 4 类决策；额外加 §1.1 工具链快照表（Xcode 26.4.1 / Swift 6.3.1 与旧 project.yml 的差异）
- [x] **T2**：完成 4 类候选评估与决策（AC1 / AC2）
  - [x] T2.1 iOS mock 框架：选定 **XCTest only（手写 Mock）**，与 0001 §3.4 决策原则一致；否决 Mockingbird / Cuckoo / Swift Mocks
  - [x] T2.2 异步测试方案：选定 **`async/await` 主流 + `XCTestExpectation` 特定场景兜底**（@Published 观察 / isInverted / 跨 actor / callback API）
  - [x] T2.3 iPhone App 工程目录方案：选定 **方案 D（新建顶级目录 `iphone/`，`ios/` 整个原封不动作为旧产物归档）**（用户 2026-04-25 三条增量决策：① 不要修改 watch 相关的目录 ② 不要改名 catshared ③ 完全不改动原来的，避免影响 watch，再独立的目录中开发 iphone app）；否决方案 A（复用 Cat.xcodeproj 改名）/ 方案 B（git mv watch + 改名 PetCore）/ 方案 B'（在 ios/ 下新建 PetApp.xcodeproj 仍要重写 ios/project.yml）/ 方案 C（完全 wipe ios/）/ 方案 D 跨目录引用 CatShared 等变体
  - [x] T2.4 CI 命令：选定 **`bash iphone/scripts/build.sh --test` wrapper + 内部调 `xcodebuild test -project iphone/PetApp.xcodeproj -destination 'iPhone 17,OS=latest'`**；与 server 端 `bash scripts/build.sh --test` 风格对齐
- [x] **T3**：写入版本锁定清单（AC3）
  - [x] T3.1 查询 2026-04 当前工具链版本（Xcode 26.4.1 实测 / Swift 6.3.1 实测 / XcodeGen 2.45.3 brew）
  - [x] T3.2 以 YAML 形式落入决策文档 §4，含 deployment target / Xcode / Swift / XcodeGen / Bundle ID prefix 等
- [x] **T4**：写入残留产物处置表（AC4）
  - [x] T4.1 复制 story Dev Notes "iOS 当前残留产物清单" 9 行进决策文档 §3.3
  - [x] T4.2 每行补 **保留 / 改名 / 移到 watch/ / 删除** 决策 + 一句理由（11 行表，含表头）
- [x] **T5**：自检并提交（AC5 / AC6）
  - [x] T5.1 检查 git status：**未**创建任何 `.swift` 文件、**未**修改 `ios/project.yml`、**未**跑 `xcodegen` / `xcodebuild`
  - [x] T5.2 决策文档已写入 `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md`（commit 由 dev-story 流程的"将状态推到 review"环节同步提交，commit 实际由用户/下游 story-done 流程触发）
  - [x] T5.3 建议 commit message：`docs(decision): 0002 ios stack - XCTest only / 方案 D (新建 iphone/，ios/ 整个不动) / build.sh --test wrapper`

## Dev Notes

### 项目关键约束（必读，勿绕过）

1. **iOS 重启上下文**（CLAUDE.md "状态：重启中" + "Tech Stack（新方向）"）：
   - 旧架构（`Cat.xcodeproj` / `CatPhone` / `CatShared` / `CatWatch*` 全部）已**整体放弃**
   - 新方向：Swift + SwiftUI，MVVM + UseCase + Repository
   - **watch/ 当前重启阶段暂不考虑** —— watchOS 不在 Epic 2 范围内，相关残留要么归档要么保留为"未来扩展槽位"
2. **设计文档为唯一权威**：iOS 架构设计 §4 已锁定目标目录形态 = `PetApp/{App,Core,Shared,Features,Resources,Tests}/`；本 spike 不重新设计，只决定**如何从当前残留过渡到目标形态**
3. **当前是 macOS 开发环境**：本仓库工作目录在 macOS（`darwin 25.4.0`），`Xcode 16.0` / `xcodegen` 等工具链可用
4. **节点 1 整体未闭合**：Epic 1（server）已 done，Epic 2（iOS）+ Epic 3（demo 验收）还在 backlog；本 spike 是 Epic 2 第一条 story，**先决定怎么干**再开始 Epic 2 后续 story 的 SwiftUI 实装
5. **Spike 性质不写代码**：本 story **禁止**修改 `ios/` 下任何代码或工程文件、**禁止**跑 `xcodegen generate`、**禁止**跑 `xcodebuild`。决策文档以外的任何代码改动都是 scope creep，归 Story 2.2

### iOS 当前残留产物清单（决策必须逐项处置 - AC4）

跑 `ls /Users/zhuming/fork/catc/ios/` 与子目录抓出的当前残留：

| 残留路径 | 内容性质 | 来源 | 决策方案需指明处置 |
|---|---|---|---|
| `ios/Cat.xcodeproj/` | XcodeGen 生成的 Xcode 工程（4 targets：CatPhone/CatWatch/CatPhoneTests/CatWatchTests） | 旧方向 | 改名 / 重生成 / 删 |
| `ios/project.yml` | XcodeGen 工程定义文件，定义 Cat / CatPhone / CatWatch / CatPhoneTests / CatWatchTests 5 实体 | 旧方向 | 改写 / 重写 / 删 |
| `ios/CatPhone/` | **空目录**，无 source 文件 | 旧方向（未实装） | 改名为 `PetApp/` / 删 |
| `ios/CatPhoneTests/` | 单测 target 占位（含 1 个 default test） | 旧方向 | 改名 / 重生成 / 删 |
| `ios/CatShared/` | Swift Package（含 Sources/CatCore + Sources/CatShared + Tests），Package.swift 完整 | 旧方向 | 保留 / 改名为 `PetCore` / 删 |
| `ios/CatWatch/` | watchOS App 骨架（App / Complication / Resources / Info.plist） | 旧方向 watchOS | 移到 `watch/` 归档 / 删 |
| `ios/CatWatchTests/` | watchOS 单测 target 占位 | 旧方向 watchOS | 移到 `watch/` 归档 / 删 |
| `ios/INSPIRATION_LIBRARY.md` | 纯文档，记录"久坐提醒后正反馈"等暂缓想法（含 watchOS 灵感） | 跨方向通用 | 保留（建议）/ 删 |
| `ios/scripts/{build.sh,git-hooks/,install-hooks.sh}` | 旧的本地脚本（build.sh 内容是否仍可用待审） | 旧方向 | 改写 / 重写 / 删 |

### 选型评估轴（建议从这几个维度打分）

| 轴 | 备注 |
|---|---|
| **依赖数量** | 用 stdlib（XCTest）的能用 stdlib，减少后续升级负担；与 1-1 决策一致原则 |
| **Swift Concurrency 兼容** | 必须能优雅 mock `async throws` 函数 / `AsyncSequence` |
| **Generic protocol 支持** | 项目里 Repository / UseCase 大量 generic，mock 库不能在这块抽风 |
| **codegen 复杂度** | 是否依赖 `swift package run mockingbird generate` 类工具步骤 |
| **Xcode 16 / Swift 5.9+ 兼容** | 必须验证 2026-04 当前生态状态 |
| **社区活跃度** | 2026-04 仍在维护？放弃维护的（如 Cuckoo 历史曾长期停更）要明示风险 |
| **学习成本** | 团队 Swift 能力一般 → 优先选 API 简单的 |

### 候选关键事实（2026-04 生态快照，供 Spike 参考，非已选型）

**iOS Mock 框架**

- **XCTest only（手写 Mock）**：零依赖，纯 stdlib。手写 `class MockFoo: FooProtocol { var calls: [String] = []; func bar() async throws { calls.append("bar") } }`。优点：完全可控、零工具链负担、与 Swift Concurrency 天然兼容。缺点：generic protocol 多时手写工作量上升、断言匹配靠 `XCTAssert`。
- **Mockingbird**：codegen 工具（`mockingbird generate`），支持 Swift protocol mock。Swift 5.9+ 支持需查证；曾在 Swift 5.7/5.8 升级时出现 codegen breakage。优点：API 简洁、支持 partial mock。缺点：需在 build phase 注入 codegen 步骤，CI 配置复杂；维护活跃度需查 2026-04 状态。
- **Cuckoo**：用 `swift run cuckoo generate` 生成 mock，类似 Mockingbird。历史上有过较长停更期，2026-04 维护状态需查。
- **Swift Mocks（含其它社区库）**：散小工具，社区使用面窄。

**异步测试方案**

- **`async/await` 直接 await（推荐主流）**：`func testFoo() async throws { let result = try await sut.doSomething(); XCTAssertEqual(result, expected) }`。XCTest 从 Xcode 13 起原生支持 `async` test 方法，无需 `expectation`。
- **`XCTestExpectation`**：仍适用于：① 观察 SwiftUI / Combine `@Published` 多次状态变化（fulfillment count > 1）；② 验证某事件**没**发生（`expectation.isInverted = true`）；③ 跨 actor 隔离边界的事件等待。

**`ios/` 目录方案对比要点**（决策时具体填）

| 方案 | 上手成本 | 历史 git blame 保留 | 与目标结构契合 | watchOS 后续恢复路径 |
|---|---|---|---|---|
| A 原地改名 + 清理 | 中（要逐目录改 project.yml + 移动文件） | 完整保留 | 取决于改造彻底度 | 清晰（CatWatch* 留在 ios/ 内） |
| B 新建 PetApp + watch 移走 | 中高（新工程 + 旧 watch 归档） | watch 部分保留，phone 部分新生 | 强（直接按目标结构生成） | 复杂（CatShared package 跨目录 link） |
| C 完全 wipe 重建 | 低（一刀切）但风险高 | 全部丢失 | 最强（绝对干净） | 最差（watch 灵感全失） |

**CI 跑法对齐**

- server 端入口：`bash scripts/build.sh --test`（CLAUDE.md 已锁）
- 建议 iOS 端等价入口：`ios/scripts/build.sh --test` 或同根的 `bash ios/scripts/build.sh --test`
- 实际跑命令：`xcodebuild test -scheme PetApp -destination 'platform=iOS Simulator,name=iPhone 15,OS=latest' -resultBundlePath build/test-results.xcresult`
- macOS 上跑模拟器需要安装对应 iOS Simulator runtime（Xcode 16 默认带 iOS 17 runtime；新版机型需手动下）

### 与 1-1 spike 的对照（学习上一次成功 spike 的模式）

| 维度 | 1-1（server 端） | 2-1（iOS 端，本 story） |
|---|---|---|
| 决策类别数 | 6（DB mock / HTTP 测试 / 断言 / mock / CI / logger / metrics） | 4（mock / 异步测试 / 目录 / CI） |
| 决策文档路径 | `_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` | `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md` |
| 是否产生代码 | ❌（AC6） | ❌（AC6） |
| 是否动 server/ 目录 | ❌ | ❌（不动 ios/） |
| 落地 story | 1.3（logging 中间件）/ 1.5（测试基础设施）/ 1.7（build.sh 重做）/ 1.8（AppError） | 2.2（App 入口）/ 2.4（APIClient）/ 2.7（测试基础设施） |
| 跨 Epic 影响 | Epic 4+ 全部业务测试 / 日志 / 指标 | Epic 5+ 全部 iOS 业务模块 / 测试栈 |

### 与 0001-test-stack.md 的关系

- 不重复决策范围：0001 锁的是 server 端工具栈，本 spike 锁 iOS 端工具栈
- 风格保持一致：ADR 头部 / Decision Summary 表格 / 各 §选定 + 理由 + 否决 + 已知坑 / 版本锁定清单 / Consequences 段
- 引用上 0001 不重复展开：如 1-1 已锁的 "git commit message 格式"、"AR27 mock 单测要求" 等概念可直接引用

### Project Structure Notes

- 与目标结构（iOS 架构设计 §4）的对齐：
  - 本 spike **只产出决策文档**，不动 `ios/` 任何代码
  - 决策文档落位在 `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md`，与 0001 同目录
  - 后续 Story 2.2 按本 spike 选定方案具体落地目标结构
- 残留与目标的差异：
  - 当前 `ios/` 是按 `Cat`/`CatPhone`/`CatWatch` 命名，目标是 `PetApp/`
  - 当前同时有 `Cat.xcodeproj` + `project.yml`（XcodeGen），目标是单一工程
  - 当前混合 iOS + watchOS 两端，目标是 iOS only（watchOS 归档到 `watch/`）

### References

- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#3 总体工程结论] — 锁定 SwiftUI + MVVM + UseCase + Repository
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#4 项目目录建议] — 目标 `PetApp/{App,Core,Shared,Features,Resources,Tests}/`
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#17 测试建议] — 单元测试优先覆盖 UseCase / ViewModel / ErrorMapper
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#18.1 首选技术路线] — `async/await` 为主，不强引入响应式框架
- [Source: docs/宠物互动App_总体架构设计.md#1 文档说明] — iOS = Swift + SwiftUI，REST + WebSocket
- [Source: CLAUDE.md "Tech Stack（新方向）"] — iOS = Swift + SwiftUI + HealthKit / CoreMotion；watch/ 当前重启阶段暂不考虑
- [Source: \_bmad-output/planning-artifacts/epics.md#Epic 2 / Story 2.1] — 4 类决策项原始 AC 来源
- [Source: \_bmad-output/implementation-artifacts/decisions/0001-test-stack.md] — 上一次成功 spike 的模板（ADR 头部 / Decision Summary / 已知坑 / 版本清单 体例）
- [Source: \_bmad-output/implementation-artifacts/1-1-mock-库选型-spike-logger-metrics-框架选型.md] — 上一次 spike story 的结构参考

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) — `claude-opus-4-7[1m]`（通过 Claude Code CLI / `bmad-dev-story` skill 调用，2026-04-25）

### Debug Log References

- 工具链实测命令：`xcodegen --version` (2.45.3) / `xcodebuild -version` (Xcode 26.4.1 / Build 17E202) / `swift --version` (swiftlang-6.3.1.1.2) / `xcrun simctl list devices iOS available`（确认 iOS 26.4 runtime + iPhone 17 系列机型）
- 关键发现：`ios/project.yml` 内 `xcodeVersion: "16.0"` 与实测 Xcode 26.4.1 不一致 → 但方案 D 不动 `ios/project.yml`，新写 `iphone/project.yml` 时直接用 26.4
- `ios/CatShared/Package.swift` 实测：`swift-tools-version: 5.9` + `platforms: [.iOS(.v17), .watchOS(.v10), .macOS(.v14)]` + 二元 product 结构（CatShared / CatCore） → 方案 D 选定**不复用 + 不动**（`iphone/PetApp/Core/` + `iphone/PetApp/Shared/` 自己写）
- `ios/scripts/build.sh` 实测：`PROJECT_PATH=Cat.xcodeproj` / `SCHEME=CatPhone` 已不适用 → 方案 D 不动 `ios/scripts/build.sh`，新写 `iphone/scripts/build.sh` 时**参考**旧脚本的 `require_tool` / `set -euo pipefail` 等良好实践
- **用户决策（2026-04-25 三条增量，逐步收敛）**：① "不要修改 watch 相关的目录" ② "不要改名 catshared" ③ "完全不改动原来的，避免影响 watch，再独立的目录中开发 iphone app" → §3.3 由原方案 B（git mv watch + 改名 PetCore）→ 方案 B'（移除 watch target 定义但 watch 源码原位）→ **方案 D（新建顶级 `iphone/` + `ios/` 整个不动）**。其它决策（mock 框架 / 异步测试 / CI）不变；CI 入口路径由 `ios/scripts/build.sh` 改为 `iphone/scripts/build.sh`

### Completion Notes List

- ✅ **AC1 满足**：4 类决策段齐（§3.1-§3.4），每段含选定 / 理由（≥3 条）/ 否决候选 / 否决理由
- ✅ **AC2 满足**：每个决策段末尾均有"已知坑 / 缓解措施"（4 段，每段 2-4 条已知坑）
- ✅ **AC3 满足**：§4 YAML 版本锁定清单，含 ios_deployment_target / xcode_version / swift_version / xcodegen / mock_framework / ci_command_entry / xcodebuild_destination / bundle_id_prefix 等
- ✅ **AC4 满足**：§3.3 末尾 9 行残留处置表（11 行 markdown 含表头），每行明示"保留 / 改名 / 删除 / `git mv` 到 watch/" + 一句理由
- ✅ **AC5 满足**：§5 Consequences 明示对 Story 2.2 / 2.4 / 2.5 / 2.7 的直接影响；§6 Post-Decision TODO 列出 Story 2.2 / 2.7 / 3.3 的具体动作
- ✅ **AC6 满足**：`git status` 仅显示 `_bmad-output/` 下文件改动 + `.claude/settings.local.json`（白名单更新）+ sprint-status.yaml + CLAUDE.md（review-fix 同步改动，见下方）；**未**修改 `ios/` 任何文件、**未**跑 `xcodegen generate` / `xcodebuild`
- ✅ **codex review 2026-04-25 round-1 两条 P1 finding 全部 fix**：
  - **F1（toolchain lock）**：§1.1 加"兼容性说明"段；§4 版本清单改为 `xcode_version_tested: 26.4.1` + `xcode_version_minimum: 16.0` 双字段 + `xcodebuild_destination_primary` + `xcodebuild_destination_fallback`；§3.4 已知坑加 destination fallback 链具体实装要求（Story 2.7 按此实装）
  - **F2（app root）**：§3.3 末尾加 "方案 D 分阶段生效" 段（CLAUDE.md 同步 + ios/scripts/ 标记废弃 + 新 iphone/scripts/ 建立）；**本 ADR commit 同步执行 CLAUDE.md "Repo Separation" 段更新**（按用户 2026-04-25 拍板的"选项 A"风格）；§6 TODO "CLAUDE.md 三选一" 折叠为 [x] 已完成 + 加 Story 2.2 / 2.7 处理 ios/scripts 废弃公告的 TODO
- ✅ **codex review 2026-04-25 round-2 三条 finding（P2/P2/P3）全部 fix**：
  - **F1 round-2（AC vs ADR 不一致 P2）**：Story 2.1 AC1.3 由"A/B/C 三选一"扩展为"A/B/C/D 四选一"，加方案 D 描述；AC1.4 CI 入口路径加 D 方案分支（`iphone/scripts/build.sh`）；AC3 版本清单要求加双字段约束 + swift-format 锁具体版本约束 + simulator primary/fallback 双套；AC5 `ios/scripts/build.sh` 表述加 D 方案分支；AC6 加"不创建 iphone/ 实体"约束 + 例外条款（允许同步改 CLAUDE.md 等非 ios/ 文档）
  - **F2 round-2（"立即生效"措辞 P2）**：§3.3 "立即生效依赖项" 段改写为 "分阶段生效"，明示三阶段（决策对齐 / 脚本切换 / build.sh 切换）+ 过渡期警告段（dev 不要调用旧 ios/scripts/* 入口；过渡期 iPhone 工作主要是写决策文档不需 build）+ "本 ADR commit 阶段实际产物 / 改动" 6 条清单
  - **F3 round-2（swift_format 未锁 P3）**：§4 `swift_format: "latest"` 改为 `"602.0.0"`（brew stable as of 2026-04-25 实测）；§6 加 "swift-format 版本验证" TODO 给 Story 2.2 落地时跑 `swift-format --version` 验证
- ✅ **codex review 2026-04-25 round-3 两条 P2 finding 全部 fix**：
  - **F1 round-3（build/ 路径冲突 P2）**：§3.4 核心命令模板 `-resultBundlePath build/test-results.xcresult` → `iphone/build/test-results.xcresult`；同时加 `-derivedDataPath iphone/build/DerivedData`；新增"路径约定"段明示 server 端 `build/` 与 iPhone 端 `iphone/build/` 严格隔离；§3.4 已知坑加新一条"build artifact 路径与 server 冲突"；§3.4 coverage 已知坑同步改路径；Story 2.2 落地时 `.gitignore` 加 `iphone/build/` 行
  - **F2 round-3（swift-format brew 安装方式 P2）**：§4 swift_format 注释由"brew 安装命令应带 @602"改为明示"brew **没有** versioned formula；安装命令是 `brew install swift-format`（unversioned）+ `swift-format --version` 必须 startsWith `602.` 否则 fail-closed"；§6 swift-format 验证 TODO 扩展为完整三步 + brew 升级到 603.x 时的处理路径
- 与 ADR-0001 的呼应：mock 框架（§3.1）跟 server 端 testify/mock 手写策略一致；CI 入口（§3.4）跟 server 端 `bash scripts/build.sh --test` 风格对齐 → 跨端 dev 切换零认知摩擦
- 关键判断（与 story 提示的"选型评估轴"一致）：依赖数量 / Swift Concurrency 兼容 / Generic protocol 支持 / codegen 复杂度 / Xcode 26 兼容性 / 学习成本 → 6 维度 XCTest only 全胜 codegen 工具

### File List

- `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md` (新建；当前约 350+ 行，多次迭代：方案 B → B' → D；codex review 后再次扩展兼容性说明 + 立即生效依赖项段)
- `_bmad-output/implementation-artifacts/2-1-ios-mock-框架选型-ios-目录决策-spike.md` (修改：勾选 Tasks/Subtasks / 填 Dev Agent Record / Status: ready-for-dev → review)
- `_bmad-output/implementation-artifacts/sprint-status.yaml` (修改：epic-2: backlog → in-progress；2-1-...: backlog → ready-for-dev → in-progress → review；last_updated: 2026-04-24 → 2026-04-25)
- `CLAUDE.md` (修改：codex review F2 P1 fix；"Repo Separation（三端独立）" 段重写为"Repo Separation（重启阶段过渡态）"，按运行时端 + 当前真实状态列出 server / iphone / ios)

### Change Log

| 日期 | 改动 | Story |
|---|---|---|
| 2026-04-25 | 创建 ADR-0002 决策文档（mock 框架 / 异步测试 / 目录方案 / CI 命令）；勾选 5 个 Task；推 Status 到 review | 2.1 |
| 2026-04-25 | 按用户增量决策 1 + 2（不动 watch / 不改名 CatShared）将 §3.3 由方案 B 改为方案 B'；连带更新 §2 摘要 / §4 Implication / §5.1 §5.3 / §6 Post-Decision TODO | 2.1 |
| 2026-04-25 | 按用户增量决策 3（完全不改动 ios/，新建独立目录开发 iPhone App）将 §3.3 由方案 B' 进一步改为方案 D（新建顶级 `iphone/` + `ios/` 整个零改动）；CI 入口路径 `ios/scripts/build.sh` → `iphone/scripts/build.sh`；§4 改为新写 `iphone/project.yml`（不动 `ios/project.yml`）；§5.3 改为列 4 条 watchOS 恢复路径；§6 加 CLAUDE.md 三选一选项 + 未来 ios/ 收口 TODO | 2.1 |
| 2026-04-25 | codex review round-1 两条 P1 finding fix（F1 toolchain lock / F2 app root truth source）：① §1.1 加"兼容性说明"段；② §4 版本清单改为 tested + minimum 双字段 + destination primary/fallback；③ §3.4 已知坑加 destination fallback 链；④ §3.3 末尾加 "方案 D 立即生效的依赖项" 段；⑤ §6 TODO 把 CLAUDE.md 三选一折叠为已执行；⑥ 同步改 CLAUDE.md "Repo Separation" 段（按用户拍板"选项 A"风格） | 2.1 |
| 2026-04-25 | codex review round-2 三条 finding fix（P2/P2/P3）：① Story 2.1 AC1.3/AC1.4/AC3/AC5/AC6 与 ADR 方案 D 决策对齐（含"四选一"方案 D 描述、CI 入口路径分支、版本清单约束加严、AC6 加"不创建 iphone/ 实体"+ 例外条款）；② ADR §3.3 "立即生效" 改写为 "分阶段生效" + 过渡期警告段；③ ADR §4 `swift_format` 由 "latest" 改为锁 "602.0.0"（brew stable 实测）+ §6 加版本验证 TODO | 2.1 |
| 2026-04-25 | codex review round-3 两条 P2 finding fix：① ADR §3.4 核心命令模板 `build/test-results.xcresult` → `iphone/build/test-results.xcresult` + 加 `-derivedDataPath iphone/build/DerivedData`；新增"路径约定"段（server `build/` vs iPhone `iphone/build/` 严格隔离）+ §3.4 已知坑加 build artifact 隔离条目；②ADR §4 swift-format 安装方式注释改为明示 brew 无 versioned formula、必须 `brew install swift-format` + `swift-format --version` startsWith `602.` 验证；§6 验证 TODO 扩展为三步 + brew 升级处理路径 | 2.1 |
