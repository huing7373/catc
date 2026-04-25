# ADR-0002: iOS 测试 / Mock 框架 / 目录结构 / CI 命令选型

- **Status**: Accepted
- **Date**: 2026-04-25
- **Decider**: Developer
- **Supersedes**: N/A（旧方向的 `Cat.xcodeproj` / `CatPhone` / `CatShared` / `CatWatch*` 整体放弃，见 `CLAUDE.md` "状态：重启中"）
- **Related Stories**: 2.1（本决策）, 2.2（App 入口落地目录方案）, 2.4（APIClient 用 mock 框架）, 2.5（ping 调用）, 2.7（测试基础设施按本 spike CI 命令落地）, Epic 5+（全部 iOS 业务模块测试）

---

## 1. Context

当前项目处于"重启后节点 1"阶段，Epic 1（server 脚手架）已 done，但 Epic 2（iOS 脚手架）+ Epic 3（节点 1 demo 验收）仍未启动 —— 节点 1 整体未闭合。

旧方向的 iOS 产物（`ios/Cat.xcodeproj` / `ios/CatPhone` / `ios/CatShared` / `ios/CatWatch*` / `ios/CatPhoneTests` / `ios/CatWatchTests` / `ios/project.yml` / `ios/scripts/build.sh`）已**整体放弃**（`CLAUDE.md` "状态：重启中"）。新方向锁定为 **Swift + SwiftUI**，**MVVM + UseCase + Repository**，**REST + WebSocket**，目标目录形态见 `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §4：`PetApp/{App,Core,Shared,Features,Resources,Tests}/`。

`CLAUDE.md` 同时明示 **watch/ 当前重启阶段暂不考虑**，所以 `CatWatch*` / `CatWatchTests/` 不属于本节点 scope，但也不能简单 `rm` —— 要为未来 watchOS 恢复留路径。

后续 Epic 2 的 9 条 story（2.2~2.10）都需要一套**预先锁定、不再争议**的工具栈与目录骨架；否则每条 story 自己拼凑 → 风格漂移、目录决策反复。

本 ADR 的目的：**一次性锁定 4 类决策**（mock 框架 / 异步测试方案 / iPhone App 工程目录方案 / CI 命令），覆盖 Epic 2 到 Epic 36 全部 iPhone App 测试与工程组织。

**关键策略**：本 ADR 选定 **`ios/` 目录整体不动**（保留为旧产物归档 + watch 留守）；新 iPhone App 在**新建顶级目录 `iphone/`** 下从零开始 —— 与 `server/` 平级，符合 CLAUDE.md "三端独立目录"原则的精神（虽然具体目录名要 CLAUDE.md 同步更新）。详见 §3.3。

**本 ADR 不产出任何 `.swift` 代码、不创建 `iphone/` 目录、不修改 `ios/` 下任何文件、不跑 `xcodegen generate` / `xcodebuild`**（Story 2.1 AC6 强制约束）。实际 `iphone/` 工程建立由 Story 2.2 承担。

### 1.1 当前机器工具链快照（2026-04-25 实测）

| 项 | 实测值 | 旧 `project.yml` 声明值 | 备注 |
|---|---|---|---|
| Xcode | 26.4.1 (Build 17E202) | "16.0" | 旧 project.yml 已过时 |
| Swift | 6.3.1 (swiftlang-6.3.1.1.2) | （未声明） | 默认随 Xcode |
| iOS Simulator runtime | iOS 26.4（默认） | （未声明） | 旧 destination "iPhone 15,OS=17" 已不可用 |
| XcodeGen | 2.45.3 (brew) | "16.0" 时代版本 | 升级到当前 |
| `swift-tools-version` (CatShared/Package.swift) | 5.9 | — | 与 Swift 6.3.1 兼容（向下兼容） |

**Implication**：CI 命令模板必须用 `iPhone 17` / `OS=latest`，不能 hardcode `iPhone 15` / `OS=17.0`；deployment target 保持 17.0（向下兼容），不强升 26.0。

**兼容性说明（单开发者重启阶段决策）**：当前 ADR 锁定的工具链版本基于本机实测。当前项目处于"重启中 + 单开发者"阶段（`CLAUDE.md` "状态：重启中"），无 CI runner、无其它 contributor 机器，锁实测版本是合理的。`ios/project.yml` 内 `xcodeVersion: "16.0"` 是旧方向产物的历史声明，不是"当前支持基线"。

**未来恢复多人协作 / 上 CI 时**，需要在专门 spike 中重新评估兼容矩阵：
- 是否锁 Xcode 最低版本（如 16.0+）→ 影响 destination 默认机型可用性
- 是否需要 destination fallback 逻辑（`iphone/scripts/build.sh` 在硬编码机型 resolve 失败时退回到 `xcrun simctl list` 第一个可用 simulator）
- 是否需要双 simulator 矩阵 CI（iPhone 15 + iPhone 17 各跑一遍）

本 ADR 不预设兼容矩阵，但 §4 版本清单与 §3.4 已知坑给出**最低限度的 fallback 提示**，避免未来兼容性 spike 完全从零开始。

---

## 2. Decision Summary

| 领域 | 选定 | 版本 / 备注 |
|---|---|---|
| iOS Mock 框架 | **XCTest only（手写 Mock）** | stdlib，Xcode 26.x 自带，零外部依赖 |
| 异步测试方案 | **`async/await` 直接 await（主流）+ `XCTestExpectation`（特定场景兜底）** | XCTest 原生 `async throws` test，自 Xcode 13 起 |
| iPhone App 工程目录方案 | **方案 D：在新建顶级目录 `iphone/` 下从零建 PetApp 工程；`ios/` 整个原封不动作为旧产物归档** | XcodeGen 2.45.3 |
| CI 跑法 | **`xcodebuild test` + `bash iphone/scripts/build.sh --test` 等价 wrapper（与 server 端对齐）** | Xcode 26.4.1，destination 用 `iPhone 17,OS=latest` |

---

## 3. Decisions

### 3.1 iOS Mock 框架：XCTest only（手写 Mock）

- **选定**：仅用 `XCTest` 标准库，**手写 mock**（每个被 mock 的 protocol 写一个 `class MockXxx: XxxProtocol`，用 `var invocations: [String]` 数组或专用 struct 记录调用、用属性储存返回值/抛出错误）。**不引入** Mockingbird / Cuckoo / 其它 codegen 工具。

- **理由**：
  1. **零外部依赖**：与 server 端 ADR-0001 §3.4 决策原则一致（手写 testify/mock 而非 codegen）。每多一个工具链 = CI 多一步、依赖多一道升级路径，本 spike 阶段不引入。
  2. **Swift Concurrency 天然兼容**：手写 mock 可直接 `func bar() async throws -> Foo` 实现，无需任何工具适配；Mockingbird / Cuckoo 等 codegen 工具对 `async throws` / `AsyncSequence` / `AsyncStream` 的支持参差不齐，2026-04 当前生态需逐项验证。
  3. **Generic protocol 无障碍**：iOS 架构 §5.4 列出的 Repository 接口（`AuthRepository` / `HomeRepository` / `RoomRepository` 等）不少会带 generic constraint（如 `func fetch<T: Decodable>(_ endpoint: Endpoint) async throws -> T`）；codegen 工具在 generic constraint 上是历史 pain point，手写则零摩擦。
  4. **接口数量可控**：iOS 架构 §5.4 + §6 共预估 8-10 个 Repository protocol、~12 个 UseCase；每个 Repository 方法数 ≤ 5（按 §13 接口表），手写 mock 总样板量约 50~80 行/Repository × 10 个 = 不到 1000 行 —— 远低于学习一套 codegen DSL 的成本。
  5. **可读性与 review 友好**：手写 mock 一眼能看出"测试期望 repo 被调哪几次、返回什么"，code review 不需要先理解 codegen 输出格式。
  6. **与 0001 决策保持一致原则**：跨端工具栈风格统一（手写 + 标准库优先），降低 dev 跨端切换的认知负担。

- **否决候选**：
  - **Mockingbird**：否决 — ① 需要 build phase 注入 codegen 步骤（`mockingbird generate`），CI 多一道；② Swift 6 / Xcode 26 兼容性 2026-04 需查 GitHub commit 活跃度（生态历史多次因 Swift major 升级 codegen 断裂）；③ 项目 protocol 数量不大，DSL 学习成本不划算。
  - **Cuckoo**：否决 — 同 Mockingbird 的 codegen 顾虑，且历史曾长期停更（2020-2022 一段断档），稳定性信号比 Mockingbird 弱。
  - **Swift Mocks / 其它社区库**：否决 — 社区使用面窄，没有形成事实标准；引入风险高于收益。
  - **协议默认实现 + 测试覆盖宏（如 `@testable import` + extension）**：否决 — 不能记录 invocation 历史、无法断言"这个方法被调了 N 次"，对 service ↔ repo 边界测试场景不够。

- **已知坑 / 缓解措施**：
  - **大型 protocol（> 10 methods）手写样板增加** → 缓解：① 按 iOS 架构 §5.4 拆 Repository（每个 Repo 单一职责，方法数 ≤ 5）；② 在 `Tests/Helpers/MockBase.swift`（Story 2.7 落地）提供通用 invocations 数组与 `record(_ method:)` helper，业务 mock 继承复用。
  - **mock 状态记录全靠手写 → 容易漏断言**：缓解：建立 `MockBase` 模板，强制每个 mock 至少记录 invocations + lastArguments，code review 时 reviewer 检查"是否所有 input 都断言到了"。
  - **未来如果 Repository 数量爆炸（> 20 个）需要 codegen** → 不强求"决策永恒"：本 spike 锁的是当前阶段的工具栈，将来若手写成本超过 codegen 成本，可在专门 spike 评估迁移（与 0001 §3.6 zap 迁移路径同样思路）。

---

### 3.2 异步测试方案：`async/await` 主流 + `XCTestExpectation` 特定场景兜底

- **选定**：**`async/await` 直接 await 作为主流**（XCTest 自 Xcode 13 起原生支持 `func testFoo() async throws { ... }` 方法签名，可直接 `let result = try await sut.doSomething()` + `XCTAssertEqual`）；**`XCTestExpectation` 仅用于以下特定场景**：

  1. **观察 `@Published` / Combine publisher 的多次值变化**（`expectation.expectedFulfillmentCount > 1`）
  2. **验证某事件 *没* 发生**（`expectation.isInverted = true`）
  3. **跨 actor 隔离边界的事件等待**（如 `MainActor` 内的 SwiftUI 状态变化要在测试线程观察）
  4. **依赖 callback-based API 不可改造为 async 的场景**（如观察 NotificationCenter 多次发布、`SwiftUI.onChange(of:)` 副作用）

- **理由**：
  1. **iOS 架构 §18.1 已锁** —— 文档明确"`async/await` 为主，不强引入响应式框架"。本 spike 与架构文档对齐，不重新讨论。
  2. **代码简洁**：`async test` 比 `expectation + fulfill + waitForExpectations` 短 50%+，可读性显著提升。
  3. **Xcode 13+ 原生支持**：当前机器 Xcode 26.4.1 完全覆盖；`async throws` test 函数自动 await，失败信息也直接定位到 await 行。
  4. **与 architecture §3 + §5.3 / 5.4 协调**：UseCase / Repository 全部用 `async throws` 签名（架构 §3 "REST + WebSocket 双通道"，§5.4 "统一对外提供数据访问入口"），测试侧用 `async/await` 是自然延伸。
  5. **Swift 6.3 严格 concurrency**：Xcode 26 / Swift 6.3 默认开 strict concurrency checking；`async/await` test 直接 ride 编译器对数据竞争的检查，比 `expectation` 模式更安全。

- **否决候选**：
  - **全用 `XCTestExpectation`（不引入 async test）**：否决 — 项目大量 `async throws` 接口，测试 boilerplate 翻倍；与架构文档 §18.1 冲突。
  - **引入 `swift-async-algorithms` 测试 helper 库**：否决 — 当前阶段只测 `async throws` 普通函数；`AsyncSequence` / `AsyncStream` 测试场景在节点 4（房间 WebSocket）+ 节点 5（pet state sync）才出现，到时单独 spike 评估。
  - **第三方 reactive testing helpers**（如 RxBlocking）：否决 — 项目不引入 RxSwift / RxCocoa，不存在该基线。

- **已知坑 / 缓解措施**：
  - **`@Published` 状态变化在 SwiftUI 视图测试中不易直接 await**：缓解：在 ViewModel 暴露一个 `var statePublisher: AnyPublisher<State, Never>`，测试侧用 `expectation` + `sink { ... fulfill() }` 收敛 N 次值；这是 §3.2 选定中"场景 1"的标准模式。
  - **跨 `MainActor` 边界 + `async test` 易写出竞态**：缓解：测试方法明确标 `@MainActor func testFoo() async throws { ... }`；ViewModel 用 `@MainActor` 注解；Repository / UseCase 不强加 actor，由调用方决定上下文。
  - **`XCTAssertThrowsError` + async**：缓解：用 `await assertThrowsAsyncError(...)` helper（Story 2.7 落地一个 helper 函数，包装 `do { try await ...; XCTFail("expected throw") } catch { ... }` 样板）。

---

### 3.3 iPhone App 工程目录方案：方案 D —— 新建顶级目录 `iphone/`，`ios/` 整个原封不动作为旧产物归档

- **选定**：

  1. **新建顶级目录 `iphone/`**（与 `server/` / `ios/` 平级），iPhone App 全部代码在此新建，按 iOS 架构设计 §4 目标结构一次到位：

     ```text
     catc/
     ├─ server/                        # Go server（不变）
     ├─ iphone/                        # ★ 新建：iPhone App 全部代码
     │  ├─ PetApp.xcodeproj/           # 由 xcodegen 生成
     │  ├─ project.yml                 # 全新写，只定义 PetApp / PetAppTests / PetAppUITests 三个 target
     │  ├─ PetApp/                     # 主 App 源码
     │  │  ├─ App/                     # PetAppApp.swift / RootView.swift / AppContainer.swift / AppCoordinator.swift / AppEnvironment.swift
     │  │  ├─ Core/                    # DesignSystem / Networking / Realtime / Storage / Motion / Health / Logging / Utils / Extensions
     │  │  ├─ Shared/                  # Models / Mappers / Constants / ErrorHandling
     │  │  ├─ Features/                # Auth / Home / Pet / Steps / Chest / Cosmetics / Compose / Room / Emoji
     │  │  └─ Resources/               # Assets.xcassets / Localizable.strings / Configs/
     │  ├─ PetAppTests/                # 单元测试 target
     │  │  ├─ Helpers/                 # MockBase.swift 等
     │  │  └─ Features/                # 镜像 PetApp/Features/ 的测试
     │  ├─ PetAppUITests/              # UI 测试 target（XCUITest）
     │  ├─ scripts/
     │  │  └─ build.sh                 # 全新写，对齐 server 端 wrapper 风格
     │  └─ README.md                   # Story 2.10 写
     ├─ ios/                           # ★ 旧产物：整个原封不动
     │  ├─ Cat.xcodeproj/              # 不动；Xcode 仍可打开
     │  ├─ project.yml                 # 不动（仍含 CatPhone / CatWatch 4 target）
     │  ├─ CatPhone/ (空)              # 不动
     │  ├─ CatPhoneTests/              # 不动
     │  ├─ CatShared/                  # 不动（Package.swift / Sources / Tests）
     │  ├─ CatWatch/                   # 不动 → 在 ios/Cat.xcodeproj 内仍可见可 build
     │  ├─ CatWatchTests/              # 不动
     │  ├─ INSPIRATION_LIBRARY.md      # 不动
     │  └─ scripts/                    # 不动（含 build.sh / git-hooks / install-hooks.sh）
     └─ docs/                          # 设计文档（少量更新：iOS 架构 §4 目录路径标注 iphone/PetApp/）
     ```

  2. **`iphone/PetApp/` 自带 `Core/` 与 `Shared/`**：**不**复用 `ios/CatShared/` Swift Package。理由 ① `ios/` 整个不动是核心约束 —— 跨目录 link `path: ../ios/CatShared` 等于变相依赖 `ios/`，违背 zero-coupling 精神；② `ios/CatShared/` 是旧方向产物，业务价值未验证（CatPhone/ 为空说明业务代码从未填进来），从头写自己的 Core / Shared 没有可观成本损失；③ 二元分层（CatCore + CatShared）思路可作为模板参考，但实际代码新写。

  3. **`ios/` 整个原封不动**：包括 `Cat.xcodeproj`、`project.yml`、`CatPhone/`、`CatPhoneTests/`、`CatShared/`、`CatWatch/`、`CatWatchTests/`、`scripts/`、`INSPIRATION_LIBRARY.md` —— 全部不删、不改、不移动。`ios/Cat.xcodeproj` 仍能用 Xcode 打开（如果 watch 开发者愿意继续在那边做 watch；CatPhone/ 空目录可能 build 失败，但 watch target 应仍可 build）。

  4. **CLAUDE.md 一致性同步**（Story 3.3 阶段执行）：CLAUDE.md "三个独立目录：`server/` (Go) / `ios/` (iOS) / `watch/`（watchOS）" 描述需要更新为 4 目录或调整语义。本 ADR 选**保留 CLAUDE.md "三独立目录"作为目标态、本 ADR 是过渡态**的折中（详见 §6）。

- **理由**：
  1. **用户决策（2026-04-25）锁定**：① "不要修改 watch 相关的目录" ② "不要改名 catshared" ③ "完全不改动原来的，避免影响 watch，再独立的目录中开发 iphone app"。方案 D 严格遵守这三条 —— `ios/` 完全 zero touch，watch 在 `ios/Cat.xcodeproj` 内继续可打开可 build（如未来需要）。
  2. **改动面最小**：本 ADR 锁定后，Story 2.2 实际工作 = 新建 `iphone/` 目录 + 写 `iphone/project.yml` + `xcodegen generate` + 写第一批 .swift 源码。**完全不动 `ios/`**。
  3. **回滚成本最低**：若新方向再失败或决策反复，`rm -rf iphone/` 一条命令完成；旧 `ios/` 完整保留作为后备。
  4. **目标结构强对齐**：方案 D 直接按 iOS 架构 §4 的 `PetApp/{App,Core,Shared,Features,Resources,Tests}/` 在 `iphone/` 下一次到位生成；不存在历史包袱。
  5. **watch 不被打扰**：`ios/Cat.xcodeproj` 完整保留，watch 开发者不必学新工程结构、不必担心新 iphone app 进度影响 watch；未来 watchOS 恢复时直接在 `ios/Cat.xcodeproj` 内继续即可，或届时单独 spike 决定迁移到 `iphone/PetApp.xcodeproj` 内做 watch target / 还是建顶层 `watch/`。
  6. **`iphone/` 目录名理由**：① 与未来可能的顶层 `watch/`（watchOS）按设备命名风格对称；② 比 `phone/` 更精确（项目明确 only iPhone）；③ 比 `pet/` 更直白（不与 server 端 `pet-server/` 反向命名混淆）。
  7. **跨端一致性收益**：`server/` + `iphone/` + （未来）`watch/` 三个独立目录都是按"运行时端"组织 —— 与 CLAUDE.md "三端独立目录" 精神契合，仅目录名从 `ios/` 改 `iphone/`。
  8. **git history 完整保留**：方案 D 不做任何 `git mv` / `rm`，`ios/` 内所有文件的 `git log --follow` / `git blame` 链路完整。

- **否决候选**：
  - **方案 A（复用 `ios/Cat.xcodeproj` 改名）**：否决 — ① `ios/CatPhone/` 是空目录，"复用工程改名"实际仍要新建源码目录，没省事；② 旧 schema/target 名（CatPhone / CatWatch）和 `Cat.xcodeproj` 的 buildSettings 历史污染未来工程；③ watch 仍受 project.yml 改写影响。
  - **方案 B（早期：在 `ios/` 下新建 PetApp.xcodeproj + CatWatch* git mv 到顶层 watch/ + CatShared 改名 PetCore）**：被用户**否决** — "不要修改 watch 相关的目录，也不要改名 catshared"。
  - **方案 B'（在 `ios/` 下新建 PetApp.xcodeproj + CatWatch* / CatShared 原位保留，但移除 watch target 定义）**：被用户**否决** — "完全不改动原来的，避免影响 watch" → 方案 B' 仍要重写 `ios/project.yml`、删 `ios/Cat.xcodeproj` / `ios/CatPhone/` / `ios/CatPhoneTests/`、移除 watch target 定义后 Xcode 打开 `ios/Cat.xcodeproj` 看不到 watch（除非用 fileGroups 折中）。方案 D 比方案 B' 更彻底实现"零打扰 watch"。
  - **方案 C（完全 wipe `ios/`）**：否决 — 与"不动 ios/"的核心约束直接冲突。
  - **方案 D 变体：用 `phone/` / `pet/` / `app/` / 仍叫 `ios/`（但 `ios/` 已占用）**：否决 — 见 §3.3 选定理由 6。
  - **方案 D 变体：iphone/ 跨目录 link `../ios/CatShared`**：否决 — 把"我说不动 ios/"语义做成"我依赖 ios/" → 未来 ios/ 整个废弃前要先解耦，徒增技术债。
  - **方案 D 变体：把 CatShared 复制一份到 iphone/Shared/CatShared/**：否决 — 双份代码污染，源码漂移风险。

- **已知坑 / 缓解措施**：
  - **CLAUDE.md "三独立目录"描述与方案 D 表面冲突**（CLAUDE.md 写 `server/` `ios/` `watch/`，方案 D 实际是 `server/` `iphone/` `ios/`，且 `ios/` 含 watch 留守）→ 缓解：① §6 Post-Decision TODO 登记此问题留给 Story 3.3 处理；② 短期不擅自改 CLAUDE.md（决策权在用户）；③ 方案 D 把 CLAUDE.md "三独立目录"理解为"按运行时端独立"语义，未来废弃 `ios/` 时把 `iphone/` rename 回 `ios/` 即可对齐文字描述。**不算阻塞**。
  - **未来 `ios/` 怎么收口**：方案 D 是过渡态，长期看 `ios/` 应该收口（要么废弃 watchOS 计划 + 删 `ios/`、要么恢复 watchOS + 把 `iphone/` rename 回 `ios/` 与 watch 合并、要么把 watch 移到顶层 `watch/`）→ 缓解：本 ADR **不预设**收口路径；当 watchOS 决策点到来时另起 spike。**不算阻塞**。
  - **iOS 架构设计 §4 文档目录路径未标注顶层目录名**：原文写 `PetApp/{App,Core,Shared,Features,Resources,Tests}/`，没说 `iphone/PetApp/` 还是 `ios/PetApp/`。Story 3.3 阶段补一句"实际位于 `iphone/PetApp/`"即可，§4 主体不必大改。
  - **`iphone/scripts/build.sh` 是新写不是从旧 `ios/scripts/build.sh` fork**：缓解：写 `iphone/scripts/build.sh` 时**参考**旧 `ios/scripts/build.sh` 的 `require_tool` / `set -euo pipefail` 等良好实践，但 `PROJECT_PATH` / `SCHEME` 等具体值从 0 写。
  - **CatShared 不复用 → 重复造轮子风险**：缓解：① `ios/CatShared/Sources/` 当前业务价值未验证（CatPhone 为空）；② 若实施时发现 CatShared 内确实有可复用基础设施代码（如某些 model / utility），dev 可在 Story 2.2 评审时单独决策"复制片段到 `iphone/PetApp/`"（**仍然不**跨目录 link），不破坏方案 D。

- **AC4 残留产物处置表**（每行决策必须显式落实）：

  | 残留路径 | 内容性质 | 处置 | 一句理由 |
  |---|---|---|---|
  | `ios/Cat.xcodeproj/` | XcodeGen 生成的旧 Xcode 工程 | **不动**（保留原位） | 方案 D 不动 `ios/`；Xcode 仍可打开 `Cat.xcodeproj` 用于 watch 开发或归档浏览 |
  | `ios/project.yml` | 旧 XcodeGen 工程定义（含 CatPhone / CatWatch 4 target） | **不动**（保留原位） | 方案 D 不动 `ios/`；若未来某天废弃 `ios/`，再做迁移 spike |
  | `ios/CatPhone/` | 空目录 | **不动**（保留原位） | 方案 D 不动 `ios/`；空目录无害 |
  | `ios/CatPhoneTests/` | 单测 target 占位 | **不动**（保留原位） | 方案 D 不动 `ios/` |
  | `ios/CatShared/` | Swift Package（Package.swift + Sources/{CatCore,CatShared} + Tests） | **不动 + 不复用**（用户决策：不改名 + 整个 ios/ 不动） | `iphone/PetApp/Core/` + `iphone/PetApp/Shared/` 自己写；未来若发现 CatShared 内有有用代码，单独决策"复制片段"而非跨目录 link |
  | `ios/CatWatch/` | watchOS App 骨架 | **不动**（保留原位） | 用户决策"避免影响 watch"；watch 仍可在 `ios/Cat.xcodeproj` 内 build |
  | `ios/CatWatchTests/` | watchOS 单测 target 占位 | **不动**（保留原位） | 跟 CatWatch 一起保留 |
  | `ios/INSPIRATION_LIBRARY.md` | 纯文档（含未来 watchOS 灵感） | **不动**（保留原位） | 跨方向通用 |
  | `ios/scripts/build.sh` | 旧 build 脚本 | **不动**（保留原位） | 方案 D 不动 `ios/`；新 `iphone/scripts/build.sh` 是从 0 写但**参考**旧脚本的良好实践 |
  | `ios/scripts/install-hooks.sh` / `ios/scripts/git-hooks/` | git hook 安装 / 实体 | **不动**（保留原位） | 方案 D 不动 `ios/`；新 `iphone/scripts/` 视需要单独建 git hooks |

- **方案 D 分阶段生效**（codex review 2026-04-25 round-1 P1 + round-2 P2 fix）：

  方案 D 不能只改本 ADR 不动其它 repo 合同 —— 否则 repo 内会有两个冲突的 iOS root truth source。但 ADR 是决策文档，不是代码实装；新 `iphone/scripts/*` 的建立属于 Story 2.2 / 2.7 实装范围。所以方案 D 是**分阶段生效**而非"ADR commit 即立即全量生效"：

  | 阶段 | 时点 | 同步改动 |
  |---|---|---|
  | **阶段 1：决策对齐** | 本 ADR commit | ✅ CLAUDE.md "Repo Separation" 段更新（消除"CLAUDE.md 说 iOS 在 ios/" vs "ADR 说在 iphone/" 的合同冲突） |
  | **阶段 2：脚本切换** | Story 2.2 落地 | 新写 `iphone/scripts/install-hooks.sh` + `iphone/scripts/git-hooks/`；建 `iphone/README.md` 头部明示"活跃入口"；视情况建 `ios/README-DEPRECATED.md` 提示旧 `ios/scripts/install-hooks.sh` 已废弃 |
  | **阶段 3：build.sh 切换** | Story 2.7 落地 | 新写 `iphone/scripts/build.sh`（按 §3.4 + 双字段 destination + fallback 链）；同样的废弃公告体例 |

  方案 D "不动 `ios/`" 原则下，旧 `ios/scripts/install-hooks.sh` / `ios/scripts/build.sh` 脚本本身**不修改**（不加 DEPRECATED 注释、不改 echo 输出）—— 废弃信号通过 `iphone/README.md` 和（如建）`ios/README-DEPRECATED.md` 传达。

  **过渡期警告**（阶段 1 完成 ↔ 阶段 2/3 完成期间，约 Story 2.1 done → Story 2.7 done 之间）：repo 处于"CLAUDE.md 已指向 `iphone/`，但 `iphone/scripts/` 还不存在"的过渡态。在此期间：

  - dev **不要**调用 `bash ios/scripts/install-hooks.sh` 安装 git hooks（已废弃，针对的是旧 ios/ 工作流）
  - 如已 install 旧 hooks，可手工卸载（删 `.git/hooks/pre-commit` 等），或留着等 Story 2.2 装新版 —— 旧 hooks 不会主动破坏新 iphone/ 工作（只是不会触发 lint）
  - dev **不要**调用 `bash ios/scripts/build.sh`（已废弃）—— 过渡期 iPhone 工作主要是写决策 / 设计文档，不需要 build；如必须 build，等 Story 2.7 落地后用 `bash iphone/scripts/build.sh`

  **本 ADR commit 阶段（阶段 1）实际产物 / 改动**：
  1. ✅ 创建 `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md`（本文件）
  2. ✅ 修改 `CLAUDE.md` "Repo Separation" 段
  3. ✅ 修改 `_bmad-output/implementation-artifacts/sprint-status.yaml`（dev-story workflow 必需）
  4. ✅ 修改 `_bmad-output/implementation-artifacts/2-1-...-spike.md`（dev-story workflow 必需）
  5. ❌ **不**触碰 `ios/` 下任何文件
  6. ❌ **不**创建 `iphone/` 任何实体目录或 .xcodeproj（属于 Story 2.2 范围）

---

### 3.4 CI 跑法：`xcodebuild test` + `bash iphone/scripts/build.sh --test` 等价 wrapper（与 server 端对齐）

- **选定**：

  - **本地 / CI 统一入口**：`bash iphone/scripts/build.sh --test`（与 server 端 `bash scripts/build.sh --test` 风格对齐）。脚本内部包装 `xcodebuild test`。
  - **核心命令模板**（脚本内部用、CI 调用方也可直接用；从 repo root 跑）：

    ```bash
    xcodebuild test \
      -project iphone/PetApp.xcodeproj \
      -scheme PetApp \
      -destination 'platform=iOS Simulator,name=iPhone 17,OS=latest' \
      -resultBundlePath iphone/build/test-results.xcresult \
      -derivedDataPath iphone/build/DerivedData \
      -enableCodeCoverage YES
    ```

    **路径约定**（codex review 2026-04-25 round-3 P2 fix）：所有 iPhone 端 build artifacts（`.xcresult` / `DerivedData/` / coverage exports）都落到 **`iphone/build/`** 子目录，与 server 端 `build/`（含 `build/catserver` / `build/coverage.out`）**严格隔离**。理由：① server 端 `scripts/build.sh` 与 iPhone 端 `iphone/scripts/build.sh` 都从 repo root 跑（与 CLAUDE.md "Build & Test" 段对齐），用 repo-root 的 `build/` 会让两端 artifact 混在一起，cleanup / CI artifact collection 会断；② `iphone/build/` 应该被 `.gitignore` 加进去（Story 2.2 落地时同步加，与 server 端 `build/` 同等待遇）。

  - **`build.sh` 脚本契约**（Story 2.2 / 2.7 落地时按此实装）：

    | 调用 | 行为 |
    |---|---|
    | `bash iphone/scripts/build.sh` | 运行 `xcodegen generate` + `xcodebuild build`（不跑测试） |
    | `bash iphone/scripts/build.sh --test` | 加跑 `xcodebuild test` 单元测试（含 `-enableCodeCoverage YES`） |
    | `bash iphone/scripts/build.sh --uitest` | 加跑 `xcodebuild test` UI 测试（XCUITest scheme） |
    | `bash iphone/scripts/build.sh --clean` | 加跑 `xcodebuild clean` 清 derivedData |

  - **`-destination` 模板的 `name=iPhone 17,OS=latest`**：选 `iPhone 17`（当前默认机型），`OS=latest` 让脚本随 Xcode 升级浮动，不 hardcode `OS=26.4`。

- **理由**：
  1. **与 server 端入口风格一致**：server 是 `bash scripts/build.sh --test`，iPhone App 是 `bash iphone/scripts/build.sh --test`，跨端 dev 切换零认知摩擦。CLAUDE.md "Build & Test" 段已锁 server 风格，iPhone 端直接镜像。
  2. **wrapper 而非裸 xcodebuild**：① CI 调用方稳定（不论 build.sh 内部如何变都不影响 CI yaml）；② 本地与 CI 跑同一命令，"CI 过了我本地也过"；③ `xcodebuild` 命令长且参数易拼错，wrapper 收敛。
  3. **`-resultBundlePath` 强制开**：失败时 .xcresult 包含完整 simulator 日志、screenshot、coverage 数据，远优于 console output；CI artifact 上传 .xcresult 后可在 Xcode 直接打开诊断。
  4. **`-enableCodeCoverage YES` 默认开**：与 0001 §3.5 server 端 `--coverage` 选项对齐；测试覆盖率作为 PR 必看指标。
  5. **`OS=latest` 浮动**：避免 Xcode 升级（如未来 Xcode 27）后 hardcode 的 OS 不可用；CI runner 升级 Xcode 时 `OS=latest` 自动追新。
  6. **`iPhone 17` 而非 `iPhone 17 Pro`**：基础机型覆盖率最广，UI 测试结果最具代表性；架构 §17.3 列的 UI 测试场景（首次登录 / 开箱 / 穿戴 / 房间）都不依赖 Pro 独有特性。

- **否决候选**：
  - **直接 `xcodebuild test ...`（不走 wrapper）**：否决 — 与 server 端不对齐；CI 配置与本地 dev 命令分叉；命令长，新人记不住。
  - **`fastlane scan` 或 `xcbeautify` 做 wrapper**：否决 — ① fastlane 是 Ruby + 全工具链，引入成本高于本项目当前规模；② xcbeautify 只是 prettify 输出，不能替代 wrapper 决策；③ 团队当前不熟 fastlane DSL。
  - **`swift test`（SwiftPM 跑测）**：否决 — 仅适用于 PetCore Package 内部测试；UI 测试与 SwiftUI ViewModel 测试需 xcodebuild + Simulator + Xcode-only 的 iOS 17+ API，swift test 跑不动。**例外**：PetCore Package 内部测试可以同时支持 `swift test` 入口（Package.swift 自带），但**主 CI 入口仍是 build.sh --test**。
  - **GitHub Actions 直接调 `xcodebuild`，不要 wrapper**：否决 — 把 CI 命令耦合到 .github/workflows/，将来切换 GitLab CI / 本地 Jenkins 都要改；wrapper 让 CI yaml 只调一行 `bash iphone/scripts/build.sh --test --uitest`。
  - **`OS=26.4` hardcode**：否决 — Xcode 升级后会失效；`OS=latest` 是 xcodebuild 官方推荐写法。

- **已知坑 / 缓解措施**：
  - **`xcodebuild test` 首次运行慢**（5-10 分钟，simulator 启动 + 编译）→ 缓解：① CI 用 `derivedDataPath` cache 复用 build artifacts；② 本地 `--clean` 仅在切 branch 后用，日常 `--test` 走增量。
  - **simulator name 在 Xcode 升级后改名**（如 `iPhone 17` 可能未来改 `iPhone 17 Pro`）→ 缓解：build.sh 在 destination resolve 失败时 fallback 到 `xcrun simctl list devices iOS available | head -1` 取第一个可用机型；CI failure log 写明实际用的机型。
  - **destination 在不同 Xcode 版本上的可用机型不一致**（codex review 2026-04-25 P1 fix）：本机 Xcode 26.4.1 默认带 iPhone 17 系列，Xcode 16 默认带 iPhone 15 系列。如未来 contributor 装的是 Xcode 16，硬编码 `name=iPhone 17` 会 resolve 失败。`iphone/scripts/build.sh` **必须**实装两段 fallback：① 先尝试 `xcodebuild_destination_primary`（§4 锁定）；② resolve 失败时退回 `platform=iOS Simulator,OS=latest`（不指定 name，让 xcodebuild 自动选）；③ 仍失败时调 `xcrun simctl list devices iOS available | head -1` 取第一个可用 simulator UUID + 用 `id=<uuid>` 形式 destination。整个 fallback 链失败才报错。
  - **Code coverage 报告格式**（`.xcresult` 二进制 vs `.lcov` 文本）→ 缓解：build.sh 提供 `--coverage-export` 选项，调 `xcrun xccov view --report --json iphone/build/test-results.xcresult` 输出 JSON 到 `iphone/build/coverage.json` 给 CI 上传；非默认开（避免日常本地 dev 多花 5 秒）。
  - **build artifact 路径与 server 冲突**（codex review 2026-04-25 round-3 P2 fix）：repo root 的 `build/` 已被 server 端 `scripts/build.sh` 用于 `build/catserver` / `build/coverage.out`；iPhone 端必须用 `iphone/build/` 隔离 artifact。`iphone/scripts/build.sh` 实装时硬编码 `iphone/build/...` 路径（不要让 dev 在 CLI 里覆写）；`.gitignore` 同步加 `iphone/build/` 行（Story 2.2 范围）。
  - **CI runner 没装 XcodeGen** → 缓解：build.sh 顶部 `require_tool xcodegen "brew install xcodegen swift-format"`（参考旧 ios/scripts/build.sh 第 36 行的写法），fail-fast 报清晰错。

---

## 4. 版本锁定清单（AC3）

```yaml
# 2026-04-25 锁定，Story 2.2 / 2.7 落地依据
# 双字段：tested = 本机实测可用；minimum = 理论兼容下限（未来上 CI 时单独 spike 验证）
ios_deployment_target: "17.0"           # 向下兼容到 iOS 17（覆盖率 70%+ 设备）；不强升 26.0

xcode_version_tested: "26.4.1"          # 本机实测可用（当前唯一开发机器，Build 17E202）
xcode_version_minimum: "16.0"           # 理论兼容下限（swift-tools-version: 5.9 + iOS 17 deployment target 推算）；未来 CI 上需单独验证

swift_version_observed: "6.3.1"         # 随 Xcode 26.4.1 默认（实测 swiftlang-6.3.1.1.2）
swift_tools_version: "5.9"              # PetApp 内 Package（如未来引入）顶部声明保持 5.9（向下兼容）；不强升 6.0

# Build 工具
xcodegen: "2.45.3"                      # brew 当前；XcodeGen 用于由 iphone/project.yml 生成 iphone/PetApp.xcodeproj
swift_format: "602.0.0"                 # brew stable as of 2026-04-25（实测 brew info swift-format）；锁具体版本号避免不同机器/CI 拿不同 formatter 行为
                                        # 安装方式（codex review 2026-04-25 round-3 P2 fix）：brew **没有** versioned formula（`swift-format@602` 不存在），只有 unversioned `swift-format`；
                                        #   brew install swift-format
                                        #   swift-format --version  # 必须 startsWith "602."，否则 fail-closed
                                        # 当 brew stable 升级到 603.x 时，dev 需评估是否更新本 ADR §4 + commit 补丁；详见 §6 TODO

# 测试
mock_framework: "stdlib (XCTest)"       # 零外部依赖（§3.1 决策）
async_test_style: "async/await"         # 主流（§3.2 决策）

# CI / 模拟器
ci_command_entry: "bash iphone/scripts/build.sh --test"
xcodebuild_destination_primary: "platform=iOS Simulator,name=iPhone 17,OS=latest"   # 实测 Xcode 26.4 默认机型
xcodebuild_destination_fallback: "platform=iOS Simulator,OS=latest"                  # Xcode 16 等旧版本上 iPhone 17 不存在时退回任意 iOS simulator
default_simulator_device: "iPhone 17"   # 与 destination_primary 一致；UI 测试首选此机型
default_simulator_runtime: "iOS latest" # 实测 iOS 26.4，但用 latest 避免 hardcode

# Bundle ID（Story 2.2 落地时使用）
bundle_id_prefix: "com.zhuming.pet"     # 由 iphone/project.yml `options.bundleIdPrefix` 锁
bundle_id_app: "com.zhuming.pet.app"    # PetApp 主 target；与旧 com.zhuming.cat.phone 不同（重启隔离）
```

**Implication for `iphone/project.yml`**（Story 2.2 落地时**新建此文件**，旧 `ios/project.yml` 不动）：

- `name`: `PetApp`
- `options.bundleIdPrefix`: `com.zhuming.pet`（与旧 `com.zhuming.cat` 隔离）
- `options.deploymentTarget.iOS`: `"17.0"`
- `options.xcodeVersion`: `"26.4"`
- `options.createIntermediateGroups`: `true`
- `targets`: `PetApp`（type: application, platform: iOS）/ `PetAppTests`（type: bundle.unit-test）/ `PetAppUITests`（type: bundle.ui-testing）—— **不**包含 watchOS target
- **不引用** `ios/CatShared` Swift Package（方案 D：`iphone/PetApp/Core/` + `iphone/PetApp/Shared/` 自己写）
- 关键 SDK 依赖（按 iOS 架构 §3 + §10）：`HealthKit.framework` / `WidgetKit.framework`（如本节点需要）；CoreMotion 由 SDK 默认引入

---

## 5. Consequences

### 5.1 对 Epic 2 后续 story 的直接影响

- **Story 2.2（SwiftUI App 入口 + 主界面骨架）**：按 §3.3 方案 D 新建 `iphone/` 目录 + 写 `iphone/project.yml` + `xcodegen generate` + 写第一批 .swift 源码；按 §3.2 落第一条 `async test`；按 §4 版本清单设定 `xcodeVersion` 等字段。**`ios/` 全程零改动**。
- **Story 2.4（APIClient 封装）**：在 `iphone/PetApp/Core/Networking/` 落地；按 §3.1 用手写 mock URLSession（`class MockURLSession: URLSessionProtocol`）测试；按 §3.2 用 `async/await` 测试方法签名。
- **Story 2.5（ping 调用）**：按 §3.1 mock APIClient（`class MockAPIClient: APIClientProtocol`）；按 §3.2 `async test` 验证 ViewModel 状态切换。
- **Story 2.7（iOS 测试基础设施搭建）**：按 §3.4 落地 `iphone/scripts/build.sh`（新写）+ `iphone/PetAppTests/Helpers/MockBase.swift`；建立第一条业务相关 mock 单元测试（满足 AR27 done 标准的模板示范）。

### 5.2 对节点 1 / 节点 2 demo 验收的影响

- **节点 1 验收（Epic 3）**：要求 "iOS 端可跑 `xcodebuild test` 通过"（epics.md §Story 3.2 AC）。本 spike 锁定的 `bash iphone/scripts/build.sh --test` 命令直接满足该验收条件。
- **节点 1 文档同步（Story 3.3）**：本 ADR 选定 "在 `iphone/` 下做 iPhone App" 与 CLAUDE.md "三独立目录" 描述存在过渡期表面冲突 —— 见 §6 TODO；同时 iOS 架构设计 §4 文档需补一句 "实际位于 `iphone/PetApp/`"。

### 5.3 对未来 watchOS 恢复（暂不考虑节点）的影响

- `ios/Cat.xcodeproj` + `ios/CatWatch/` + `ios/CatWatchTests/` + `ios/CatShared/` **完全原封不动**，watch 开发者随时可在 `ios/Cat.xcodeproj` 内继续 watchOS 工作 —— 方案 D 对 watch **零打扰**。
- 当 watchOS 恢复决策点到来时，至少有 4 条路径供选择（**本 ADR 不预设**，届时单独 spike）：
  1. 继续在 `ios/Cat.xcodeproj` 做 watch，与 `iphone/PetApp.xcodeproj` 永久双工程并存
  2. 把 `iphone/PetApp.xcodeproj` 升级为 dual-platform 工程（加 watchOS target），把 `ios/CatWatch/` 迁移进 `iphone/PetWatch/`
  3. 在仓库根新建顶层 `watch/`，把 `ios/CatWatch/` 迁移进去（与 `iphone/` 平级）
  4. 废弃 watchOS 计划，删除 `ios/`

### 5.4 对跨端工具栈一致性的影响

- 与 ADR-0001 §3.4 决策（手写 mock）保持原则一致 → 跨端 dev 切换零认知摩擦。
- 与 ADR-0001 §3.5 决策（`bash scripts/build.sh --test` 统一入口）保持风格对齐 → CI 配置可统一用一份脚本调用 `bash scripts/build.sh --test`（server）和 `bash iphone/scripts/build.sh --test`（iPhone App）。
- **顶级目录命名风格**：`server/` (Go) + `iphone/` (Swift/SwiftUI) —— 都按运行时端命名。未来若恢复 watchOS 单独建 `watch/`（顶级，与 `iphone/` 对称），符合 CLAUDE.md "三端独立目录"语义。

---

## 6. Post-Decision TODO（不属本 ADR scope，但需登记）

- [x] **CLAUDE.md "Repo Separation" 段同步更新**（本 ADR commit 内已执行；codex review 2026-04-25 P1 fix）：旧"三个独立目录：`server/` `ios/` `watch/`" → 新"三个目录（重启阶段过渡态）：`server/` (Go server，新方向) + `iphone/` (iPhone App，新方向) + `ios/`（旧产物归档，含 watch 留守）"
- [ ] Story 2.2：按 §3.3 方案 D 在仓库根新建 `iphone/` 目录 + 写 `iphone/project.yml`（参考 §4 Implication）+ `xcodegen generate` + 写 `iphone/PetApp/App/PetAppApp.swift` 等首批源码；**`ios/` 全程零改动**
- [ ] Story 2.2：按 §3.3 "立即生效依赖项 §2"，新写 `iphone/scripts/install-hooks.sh` + `iphone/scripts/git-hooks/`；在 `iphone/README.md`（Story 2.10 范围）+ 视情况建 `ios/README.md` 头部明示 "`ios/scripts/install-hooks.sh` 已废弃；如 dev 已 install，应手工卸载"
- [ ] Story 2.7：按 §3.4 新写 `iphone/scripts/build.sh`（参考旧 `ios/scripts/build.sh` 的良好实践但路径与 SCHEME 从 0 写；**必须实装** §3.4 已知坑提到的 destination fallback 链）；落地 `iphone/PetAppTests/Helpers/MockBase.swift` + 第一条业务相关 mock 单元测试
- [ ] Story 3.3：在 `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §4 补一句"实际位于 `iphone/PetApp/`"（§4 主体目录结构 `PetApp/{App,Core,Shared,Features,Resources,Tests}/` 不必改）
- [ ] **未来 `ios/` 收口决策**（任意时点触发，至少节点 1 后）：方案 D 是过渡态，长期看 `ios/` 应该收口（参见 §5.3 四条路径）；当 watchOS 决策点或 `ios/` 占用问题（如 git status 噪音）显著时，单独起 spike 决定收口路径
- [ ] **多人协作 / CI 兼容矩阵 spike**（任意时点触发）：本 ADR 工具栈版本基于单开发者机器实测；未来恢复多人或上 CI 时需评估 destination fallback、Xcode 最低版本、双 simulator 矩阵 CI 等（详见 §1.1 兼容性说明）
- [ ] **swift-format 版本验证**（Story 2.2 实装 git hooks 时执行；codex review 2026-04-25 round-3 P2 fix 配套）：
  - 安装命令：`brew install swift-format`（**unversioned**；`swift-format@602` 在 brew 不存在）
  - 安装后验证：`swift-format --version` 输出必须 startsWith `602.`，否则 fail-closed（脚本退出 + 报错）
  - `iphone/scripts/install-hooks.sh` 在调 `swift-format` 前先跑此版本验证，避免 hook 在 dev 本机 vs CI 拿到不同 formatter 版本导致 lint diff
  - 当 brew stable 升级到 `603.0.0+` 时，dev 需评估：① 同时更新本 ADR §4 锁定值 + iphone/scripts/install-hooks.sh 验证 startsWith；② 或在 ADR 加新 lesson "swift-format 跨大版本兼容性变化记录"
