# Story 2.7: iOS 测试基础设施搭建（按 Story 2.1 / ADR-0002 选型落地）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iOS 开发,
I want iPhone App 端测试栈一次配齐（`iphone/scripts/build.sh` wrapper + `MockBase` 通用 mock 抽象 + 通用 async 测试 helper + CI 跑法文档化），
so that 后续 Epic（4 / 5 / 7 / 8 / 10 / 12 ...）的 ViewModel / UseCase / Repository 测试**直接能跑** + 风格一致 + 不再每次重新讨论 mock invocations / `xcodebuild` 命令面 / `iphone/build/` 路径约定。

## 故事定位（Epic 2 第七条实装 story；Epic 2 测试基础设施收官）

这是 Epic 2 内**第七条**实装 story，**直接前置**全部 done：

- **Story 2.1 (`done`)** 输出 ADR-0002 锁定 4 类决策（mock 框架 / 异步测试方案 / iPhone 工程目录 / CI 命令），明示 "Story 2.7 落地 §3.4 `iphone/scripts/build.sh`（按 destination fallback 链）+ `iphone/PetAppTests/Helpers/MockBase.swift` + 第一条业务相关 mock 单元测试" —— 本 story 是 ADR-0002 §6 Post-Decision TODO 第 3 条的兑现 story
- **Story 2.2 (`done`)** 落地 `iphone/` 顶层目录 + `iphone/project.yml` + `iphone/scripts/install-hooks.sh` + `iphone/scripts/git-hooks/pre-commit`（占位 hook）。本 story **不动** install-hooks.sh / pre-commit；仅在 `scripts/` 同级新增 `build.sh`
- **Story 2.4 (`done`)** 落地 `MockURLSession`（手写 mock URLSession）+ `StubURLProtocol`（NSLock 保护 + snapshot helper + session-local-only 注入）；这两个文件是 **networking-specific** 可复用 mock，本 story **不动也不重新设计**（lesson `2026-04-26-urlprotocol-stub-global-state.md` + `2026-04-26-urlprotocol-session-local-vs-global.md` 已沉淀，重设计反而是退步）
- **Story 2.5 (`done`)** 落地 `MockAPIClient`（手写 mock APIClientProtocol，按 endpoint.path 字符串索引 stub map）+ `PingStubURLProtocol`（继承 Story 2.4 NSLock + snapshot 模式）。这两个文件**保持原样**；本 story 的 `MockBase` 是**新增基类**，老 mock 不强制迁移（保持 Story 2.4 / 2.5 测试代码零改动是范围红线）
- **Story 2.6 (`done`)** 落地 `ErrorPresenter` / `Toast` / `AlertOverlay` / `RetryView` + 一批 ErrorHandling 测试，全部用现有手写 mock 模式跑通 —— 证明"无 MockBase 也能写测试"，本 story 的 MockBase 是**便利**而非**前置**

**本 story 的核心动作**（顺序无关，可分批落地）：

1. 新建 `iphone/scripts/build.sh`：与 `server/scripts/build.sh`（实际位置 `scripts/build.sh`，see CLAUDE.md §Build & Test）**风格对齐**的 wrapper，包装 `xcodegen generate` + `xcodebuild build` + `xcodebuild test`；支持 `--test` / `--uitest` / `--clean` / `--coverage-export` 四个开关；实装 ADR-0002 §3.4 强制要求的 destination 三段 fallback 链（`iPhone 17,OS=latest` → `OS=latest` → `xcrun simctl list` UUID）；artifacts 全部落到 `iphone/build/`（与 server 端 `build/` 隔离，已在 `.gitignore` 第 48 行）
2. 新建 `iphone/PetAppTests/Helpers/MockBase.swift`：通用 mock 基类，提供 `invocations: [String]` 数组 + `record(_ method:)` helper + `lastArguments` 字段 + 线程安全（`NSLock`）。后续业务 mock **可选**继承（Mock 类继承 SwiftUI / Foundation type 不便时直接用 has-a 关系把 `MockBase` 当成员）
3. 新建 `iphone/PetAppTests/Helpers/AsyncTestHelpers.swift`：提供 `assertThrowsAsyncError(_:_:)` helper（包装 `do { try await expr; XCTFail("expected throw") } catch { XCTAssertTrue(matcher(error)) }` 样板，**ADR-0002 §3.2 已知坑**第 3 条明示要落地）；以及 `awaitPublishedChange<S, P>(_ keyPath:, on:, count:, timeout:)` helper（封装 `XCTestExpectation` + Combine sink，用于 ADR-0002 §3.2 "场景 1 多次值变化"）
4. 新建 `iphone/PetAppTests/Helpers/SampleViewModelTests.swift`（业务相关示范单测，AC27 done 标准）：定义一个 `SampleUseCase` protocol + `SampleViewModel` + `MockSampleUseCase: SampleUseCase`（继承 / 组合 `MockBase`），写 ≥ 3 case 验证 mock 注入链路 + ViewModel 状态切换 + AsyncTestHelpers 调用范例。**此文件是后续业务 story Layer 1 测试 AC 的模板**（直接复制粘贴改 type name 就能用）
5. 新建 `iphone/PetAppTests/Helpers/SampleViewModel.swift`（production 侧 placeholder type，让 Sample 测试真有被测对象）：放 `iphone/PetApp/Shared/Testing/`（**注意**：production 文件不能放在 `PetAppTests/`！但这又是 placeholder 不要污染主 App，**决策**：把 SampleViewModel + SampleUseCase 放在 `iphone/PetApp/Shared/Testing/SampleTypes.swift`，文件头注释 "为 Story 2.7 测试基础设施模板存在；非业务代码；不导出给真业务 Feature 使用"，并加 `#if DEBUG` 包裹整个文件，让 Release build 自动 strip）
6. 文档化 CI 跑法：在新建的 `iphone/docs/CI.md`（**新建**）写明：本地 / CI 入口命令 = `bash iphone/scripts/build.sh --test`；GitHub Actions YAML 等真 CI workflow 不在本 story scope（→ Epic 3 Story 3.3 "文档同步与 tech debt 登记" 或更晚），仅在 `iphone/docs/CI.md` 留 stub 章节"未来 GitHub Actions 接入点"
7. **不动** Story 2.4 / 2.5 已落地的 `MockURLSession` / `StubURLProtocol` / `MockAPIClient` / `PingStubURLProtocol`（5 个测试 mock 文件主体）；**不动**任何 production code（`PetApp/` 下除 `Shared/Testing/SampleTypes.swift` 这一个新文件外零改动）

**不涉及**：

- **真实 CI workflow（→ 推迟，至少到 Epic 3 Story 3.3 或更晚）**：本 story **不**写 `.github/workflows/*.yml` / `.gitlab-ci.yml` 等真 CI 配置文件。理由 ① ADR-0002 §3.4 + §6 TODO 都明确 "build.sh 是本地 + CI 统一入口"，CI YAML 调一行 `bash iphone/scripts/build.sh --test` 即可，CI 装配本身是工程化决策（runner 选型 / cache 策略 / artifact upload），跨 server/iphone 两端要统一规划，应留给"全项目首次上 CI"的专门 spike；② 当前重启阶段单开发者无 CI runner，写了也不能跑；③ epics.md Story 2.7 AC 原文 "CI 跑法文档化（README 或 ci.yaml）" —— "或" 字面允许只文档化不写 yaml。本 story 选择**仅文档化**，留扩展点
- **重新设计 Story 2.4 / 2.5 的 networking-specific mock**：`MockURLSession` 是 `URLSessionProtocol` 的 mock，是 Story 2.4 APIClient 测试栈的核心；`StubURLProtocol` 是 `URLProtocol` 子类做集成测试 fake server，已沉淀两条 lesson（NSLock + session-local-only）。这两个文件**继续作为 networking-specific 可复用 mock 留在 `PetAppTests/Core/Networking/`**，**不**搬到 `Helpers/` 下（搬动会让 Story 2.4 / 2.5 测试 import 路径变，违反"不动既有测试"红线）。本 story 的 `MockBase` / `AsyncTestHelpers` 是**横切性**通用 helper，与 networking-specific mock 是平级关系
- **强制把 Story 2.4 / 2.5 老 mock 迁移成继承 MockBase**：`MockURLSession` 已有 `invocations: [URLRequest]` 字段（Story 2.4 实装时已对齐 ADR-0002 §3.1 "至少记录 invocations + lastArguments"）；`MockAPIClient` 已有 `invocations: [Endpoint]`。两者**模式上已经是** MockBase 的 spirit，只是没继承类。强制迁移会破坏既有测试（red-green review 链路），**不做**。本 story 在 MockBase 文件头注释明示"已存在的 networking mock 不强制迁移；新写业务 mock 优先继承 MockBase"
- **dockertest 等价物 / 真容器集成测试**：iOS 端没有 dockertest 概念；XCTest 跑 simulator 已经是"集成测试"上限。Story 2.4 `APIClientIntegrationTests` 用 `StubURLProtocol` 作 fake server 是 iOS 端 layer-2 集成的标准模式
- **swift-testing 框架**（即新框架 `@Test` macro）：ADR-0002 §3.2 选定 XCTest only；引入 swift-testing 是另起 spike 的决策。本 story 严格守住 XCTest
- **快照测试库**（SnapshotTesting / iOSSnapshotTestCase）：Story 2.6 `ErrorComponentSnapshotTests` 已用"断言 a11y identifier + 文案"代替像素快照（明示在 Story 2.6 Dev Note）。本 story 不引入快照测试库
- **修改 Story 2.4 / 2.5 / 2.6 任何测试文件**：包括 `MockURLSession` / `StubURLProtocol` / `MockAPIClient` / `PingStubURLProtocol` / `APIClientTests` / `APIClientIntegrationTests` / `PingUseCaseTests` / `HomeViewModelTests` / `ErrorPresenterTests` / `AppErrorMapperTests` 等
- **真实业务 ViewModel / UseCase 在 SampleViewModel 之上写测试**：本 story 的 `SampleViewModel` / `SampleUseCase` 是**模板**而非业务；首次真业务 ViewModel 测试由 Epic 4+ 业务 story 自己写（届时复制 SampleViewModelTests 改 type name 即可）
- **WebSocket / Combine `AsyncSequence` / `AsyncStream` 测试 helper**：节点 4（房间 WebSocket）+ 节点 5（pet state sync）才出现，到时单独 spike 评估（ADR-0002 §3.2 已记）
- **iphone/scripts/install-hooks.sh / git-hooks/pre-commit 修改**：Story 2.2 已落地占位 hook（exit 0），扩展真实 swift-format / lint 调用属于 tech debt（Story 2.2 install-hooks.sh 文件头已写明 "真实 swift-format 调用作为 tech debt，在后续 story 视需要扩展"）。本 story **不**接 hook 实装；如本 story dev 觉得有信号要在 build.sh 顺手挂个 swift-format check，单独评审，但**默认**不做
- **修改 `iphone/project.yml`**：本 story 所有新文件（`scripts/build.sh` / `PetAppTests/Helpers/*` / `PetApp/Shared/Testing/SampleTypes.swift` / `docs/CI.md`）都靠 `iphone/project.yml` 既有的 `sources: - PetApp` / `sources: - PetAppTests` glob 自动纳入，**0 yml 改动**

**范围红线**：

- **不动 `ios/`**：本 story 绝对不修改 `ios/` 任何文件（CLAUDE.md "Repo Separation" + ADR-0002 §3.3 既有约束）。最终 `git status` 须确认 `ios/` 下零改动
- **不动 `server/`**：本 story 是 iOS 端测试基础设施落地，不涉及 server 任何文件
- **不动 repo 根 `scripts/build.sh`**：那是 server 端 build wrapper，与 iOS 端独立。两个 `build.sh` 风格对齐但代码不共享（shell 复用属于过度抽象）
- **不修改 Story 2.2 已落地的 `iphone/project.yml`** —— 所有新文件都靠 glob 自动纳入
- **不修改 Story 2.2 已落地的 `iphone/scripts/install-hooks.sh` / `iphone/scripts/git-hooks/pre-commit`**
- **不修改 Story 2.4 已落地的 `MockURLSession.swift` / `StubURLProtocol.swift` / `APIClient.swift` / `APIError.swift` / `Endpoint.swift` / `APIResponse.swift` / `URLSessionProtocol.swift`**
- **不修改 Story 2.5 已落地的 `MockAPIClient.swift` / `PingStubURLProtocol.swift` / `HomeViewModel.swift` / `PingUseCase.swift` / `PingResult.swift` / `PingEndpoints.swift` / `AppContainer.swift`**
- **不修改 Story 2.6 已落地的 `AppErrorMapper.swift` / `ErrorPresentation.swift` / `ErrorPresenter.swift` / `ToastView.swift` / `AlertOverlayView.swift` / `RetryView.swift` / `ErrorPresentationHostModifier.swift` / `AccessibilityID.swift`**
- **不修改 `iphone/.gitignore`**：`iphone/build/` 已在 repo 根 `.gitignore` 第 48 行（Story 2.2 落地时同步加），本 story 复用该 gitignore 行
- **不引入第三方依赖**：MockBase / AsyncTestHelpers 用纯 Foundation + XCTest + Combine（stdlib）实装；`xcodegen` / `swift-format` / `xcrun simctl` 是已假设可用的 host 工具
- **不在 sample 上加 ctx cancellation 测试**：iOS 没有 Go context 等价物（Swift Task 用 `Task.cancel()` + `Task.checkCancellation()`，但本 story 模板**不深入**这个主题）。Cancel 测试模板可由未来需要的 story 自己加
- **`SampleViewModel` 不能与 Story 2.6 的实际业务名冲突**：检查 `iphone/PetApp/Features/` 下无 `Sample*` 命名（grep 验证）；如有冲突，改名为 `MockSampleViewModel` 或 `TemplateViewModel`
- **不写 UI 测试（XCUITest）扩展**：Story 2.2 已落地 `PetAppUITests/HomeUITests.swift` + `NavigationUITests.swift`；本 story 不动这两个文件，也不在 `PetAppUITests/` 下加新 helper（XCUITest helper 由首次 XCUITest 业务复用需求时单独决策）

## Acceptance Criteria

**AC1 — `iphone/scripts/build.sh`：wrapper 脚本对齐 server 风格 + 实装 ADR-0002 §3.4 destination fallback 链**

新建 `iphone/scripts/build.sh`，行为契约（ADR-0002 §3.4 + §3.4 已知坑全部覆盖）：

```bash
#!/usr/bin/env bash
# iphone/scripts/build.sh
# Story 2.7 · ADR-0002 §3.4 落地：iPhone App 构建 / 测试 wrapper。
#
# Usage: bash iphone/scripts/build.sh [--test] [--uitest] [--clean] [--coverage-export]
#   --test              加跑单元测试（PetAppTests scheme，xcodebuild test）
#   --uitest            加跑 UI 测试（PetAppUITests scheme，xcodebuild test）
#   --clean             加跑 xcodebuild clean + 删 iphone/build/DerivedData
#   --coverage-export   跑完测试后调 xcrun xccov 导出 coverage 到 iphone/build/coverage.json
#                       （要求 --test 或 --uitest 之一）
#
# Exit code 0 = success, non-zero = failure.
# 全部 stdout + stderr merge 便于 log 捕获。
#
# Notes:
#   - 入口工程：iphone/PetApp.xcodeproj（由 xcodegen 从 iphone/project.yml 生成）
#   - 默认 scheme：PetApp（含 PetApp / PetAppTests / PetAppUITests 三个 target）
#   - artifacts 路径：iphone/build/{test-results.xcresult, DerivedData/, coverage.json}
#     —— 与 server 端 build/ 严格隔离（ADR-0002 §3.4 已知坑第 4 条）
#   - destination 三段 fallback：iPhone 17,OS=latest → OS=latest → xcrun simctl 第一个可用
#     （ADR-0002 §3.4 已知坑第 2 条：Xcode 16 / 26 默认机型不一致）
#   - --uitest 与 --test 不互斥：可同时跑（XCUITest scheme 与 unit test scheme 独立 invocation）

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
IPHONE_DIR="$REPO_ROOT/iphone"
PROJECT_PATH="$IPHONE_DIR/PetApp.xcodeproj"
SCHEME="PetApp"
UITEST_SCHEME="PetAppUITests"   # 若 project.yml 自动生成 UI 测试独立 scheme，则用此名；否则与 SCHEME 同
OUTPUT_DIR="$IPHONE_DIR/build"
DERIVED_DATA="$OUTPUT_DIR/DerivedData"
TEST_RESULTS="$OUTPUT_DIR/test-results.xcresult"
COVERAGE_JSON="$OUTPUT_DIR/coverage.json"

RUN_TESTS=false
RUN_UITESTS=false
RUN_CLEAN=false
EXPORT_COVERAGE=false

usage() {
  sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'
}

for arg in "$@"; do
  case "$arg" in
    --test)             RUN_TESTS=true ;;
    --uitest)           RUN_UITESTS=true ;;
    --clean)            RUN_CLEAN=true ;;
    --coverage-export)  EXPORT_COVERAGE=true ;;
    -h|--help)          usage; exit 0 ;;
    *)
      echo >&2 "ERROR: 未知参数：$arg"
      usage
      exit 1
      ;;
  esac
done

# 前置校验：--coverage-export 要求 --test 或 --uitest
if [ "$EXPORT_COVERAGE" = true ] && [ "$RUN_TESTS" = false ] && [ "$RUN_UITESTS" = false ]; then
  echo >&2 "ERROR: --coverage-export 要求 --test 或 --uitest"
  exit 1
fi

# require_tool helper（参考 server/scripts/build.sh 风格 + iphone/scripts/install-hooks.sh）
require_tool() {
  local tool="$1"
  local install_hint="$2"
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo >&2 "ERROR: $tool 未安装"
    echo >&2 "  安装：$install_hint"
    exit 1
  fi
}

require_tool xcodegen   "brew install xcodegen"
require_tool xcodebuild "（macOS Xcode 自带；运行 xcode-select --install）"
require_tool xcrun      "（macOS Xcode 自带）"

mkdir -p "$OUTPUT_DIR"
mkdir -p "$DERIVED_DATA"

cd "$IPHONE_DIR"

# === xcodegen ===（每次跑都 regen，与 Story 2.4 / 2.5 既有惯例一致）
echo "=== xcodegen generate ==="
if ! xcodegen generate 2>&1; then
  echo "FAIL: xcodegen generate"
  exit 1
fi
echo "OK: PetApp.xcodeproj generated"

# === clean（可选）===
if [ "$RUN_CLEAN" = true ]; then
  echo ""
  echo "=== xcodebuild clean ==="
  xcodebuild clean -project "$PROJECT_PATH" -scheme "$SCHEME" 2>&1 || true
  rm -rf "$DERIVED_DATA"
  rm -rf "$TEST_RESULTS"
  echo "OK: clean done"
fi

# === destination 三段 fallback 解析（ADR-0002 §3.4 已知坑第 2 条）===
DESTINATION_PRIMARY="platform=iOS Simulator,name=iPhone 17,OS=latest"
DESTINATION_SECONDARY="platform=iOS Simulator,OS=latest"
RESOLVED_DESTINATION=""

# fallback 链：尝试用 xcodebuild -showdestinations 判断 primary 是否可解析
# （比真跑 build 失败再 fallback 快）
if xcodebuild -project "$PROJECT_PATH" -scheme "$SCHEME" -showdestinations 2>/dev/null | grep -q "iPhone 17"; then
  RESOLVED_DESTINATION="$DESTINATION_PRIMARY"
elif xcodebuild -project "$PROJECT_PATH" -scheme "$SCHEME" -showdestinations 2>/dev/null | grep -q "iOS Simulator"; then
  RESOLVED_DESTINATION="$DESTINATION_SECONDARY"
else
  # 第三段 fallback：xcrun simctl 取第一个可用 simulator UUID
  FALLBACK_UUID="$(xcrun simctl list devices iOS available 2>/dev/null | grep -Eo '\([0-9A-F-]{36}\)' | head -1 | tr -d '()')"
  if [ -z "$FALLBACK_UUID" ]; then
    echo "FAIL: 无法解析任何 iOS Simulator destination；请检查 Xcode 安装与 iOS Simulator runtime"
    exit 1
  fi
  RESOLVED_DESTINATION="platform=iOS Simulator,id=$FALLBACK_UUID"
fi

echo ""
echo "=== resolved destination: $RESOLVED_DESTINATION ==="

# === xcodebuild build ===（默认行为：vet 等价物 + build）
echo ""
echo "=== xcodebuild build ==="
if ! xcodebuild build \
    -project "$PROJECT_PATH" \
    -scheme "$SCHEME" \
    -destination "$RESOLVED_DESTINATION" \
    -derivedDataPath "$DERIVED_DATA" \
    2>&1; then
  echo "FAIL: xcodebuild build"
  exit 1
fi
echo "OK: build succeeded"

# === xcodebuild test（unit）===
if [ "$RUN_TESTS" = true ]; then
  echo ""
  echo "=== xcodebuild test (unit, scheme=$SCHEME) ==="
  if ! xcodebuild test \
      -project "$PROJECT_PATH" \
      -scheme "$SCHEME" \
      -destination "$RESOLVED_DESTINATION" \
      -resultBundlePath "$TEST_RESULTS" \
      -derivedDataPath "$DERIVED_DATA" \
      -enableCodeCoverage YES \
      2>&1; then
    echo "FAIL: unit tests"
    exit 1
  fi
  echo "OK: unit tests passed"
fi

# === xcodebuild test (UI) ===
if [ "$RUN_UITESTS" = true ]; then
  echo ""
  echo "=== xcodebuild test (ui, scheme=$SCHEME) ==="
  # NOTE: PetAppUITests 是 .ui-testing target；xcodebuild test 默认会跑 scheme 关联的所有 test target
  # 如果要 only-uitest，加 -only-testing:PetAppUITests/...；本 story 简化为跑全 scheme test
  # （重复跑 unit test，但 xcodebuild 增量支持，时间成本低）
  if ! xcodebuild test \
      -project "$PROJECT_PATH" \
      -scheme "$SCHEME" \
      -destination "$RESOLVED_DESTINATION" \
      -resultBundlePath "${TEST_RESULTS%.xcresult}-ui.xcresult" \
      -derivedDataPath "$DERIVED_DATA" \
      -only-testing:PetAppUITests \
      2>&1; then
    echo "FAIL: ui tests"
    exit 1
  fi
  echo "OK: ui tests passed"
fi

# === coverage 导出 ===
if [ "$EXPORT_COVERAGE" = true ]; then
  echo ""
  echo "=== xcrun xccov view --report --json ==="
  if ! xcrun xccov view --report --json "$TEST_RESULTS" > "$COVERAGE_JSON" 2>&1; then
    echo "FAIL: coverage export"
    exit 1
  fi
  echo "OK: coverage at iphone/build/coverage.json"
fi

echo ""
echo "BUILD SUCCESS"
```

**关键约束**：

- **三段 destination fallback 必须实装**：ADR-0002 §3.4 已知坑第 2 条 P1 fix 强制约束 —— Xcode 16 / 26 默认机型不一致，硬编码 `iPhone 17` 在旧 Xcode 上 resolve 失败；先 `xcodebuild -showdestinations` grep 检查 primary，failed 退 secondary（任意 iOS Simulator），再 failed 退 `xcrun simctl list` 第一个可用 simulator UUID
- **artifacts 必须落 `iphone/build/`**：ADR-0002 §3.4 P2 fix —— server 端用 repo root `build/`，iPhone 端必须用 `iphone/build/` 子目录隔离，`.gitignore` 已在第 48 行加（Story 2.2 落地时同步加，本 story 复用）
- **`xcodegen generate` 每次跑**：Story 2.2 / 2.3 / 2.4 / 2.5 / 2.6 既有惯例，新增 .swift 文件后必须 regen .xcodeproj 让新文件被 build phase 看见
- **`require_tool` fail-fast**：参考 server `scripts/build.sh` 第 36 行 + `iphone/scripts/install-hooks.sh` 第 37-45 行的写法；缺 xcodegen / xcodebuild / xcrun 时立即报错 + 给安装提示
- **`set -euo pipefail` 严格模式**：与 server `scripts/build.sh` 第 22 行 + `iphone/scripts/install-hooks.sh` 第 17 行对齐
- **失败路径必须 fast-fail**：vet / build / test 任一非 0 退出立即 exit；不要继续后续阶段
- **`-enableCodeCoverage YES` 默认开**：与 server `scripts/build.sh` `--coverage` 选项的精神对齐 —— 测试覆盖率作为 PR 必看指标；额外的 `--coverage-export` 调 `xcrun xccov` 导出 JSON 是 CI 上传用
- **不引入 hardcoded `OS=26.4`**：`OS=latest` 浮动（ADR-0002 §3.4 已知坑第 1 条 + §4 双字段 destination 锁定）
- **不靠环境变量配置路径**：`PROJECT_PATH` / `OUTPUT_DIR` 全部在脚本顶部硬编码（ADR-0002 §3.4 P2 fix "不要让 dev 在 CLI 里覆写"）
- **chmod +x build.sh**：让 dev 直接 `bash iphone/scripts/build.sh` 调用（也可以 `./iphone/scripts/build.sh`）
- **顶部 shebang `#!/usr/bin/env bash`**：与 server `scripts/build.sh` 第 1 行 + `iphone/scripts/install-hooks.sh` 第 1 行对齐

**AC2 — `iphone/PetAppTests/Helpers/MockBase.swift`：通用 mock 基类**

新建 `iphone/PetAppTests/Helpers/MockBase.swift`：

```swift
// MockBase.swift
// Story 2.7 · ADR-0002 §3.1 落地：手写 Mock 通用基类。
//
// 设计目标：让后续业务 mock（MockAuthRepository / MockHomeRepository / MockChestUseCase 等）
// 通过继承（class）或组合（struct/actor）方式复用 invocations 记录 + lastArguments + 线程安全机制。
//
// 现有 networking-specific mock 不强制迁移：
// - MockURLSession (Story 2.4) 已有 invocations: [URLRequest] 模式（手写实装）
// - MockAPIClient (Story 2.5) 已有 invocations: [Endpoint] 模式（手写实装）
// 两者对应 ADR-0002 §3.1 "至少记录 invocations + lastArguments" 精神，不需要改。
// 新写业务 mock 优先继承 MockBase 或包含 MockBase 字段；老 mock 保持原样。
//
// 用法 1：class 继承（推荐，业务 mock 大多是 class）
//
//   final class MockChestRepository: MockBase, ChestRepository, @unchecked Sendable {
//       var openChestStubResult: Result<Reward, Error> = .failure(MockError.notStubbed)
//       func openChest(idempotencyKey: String) async throws -> Reward {
//           record(method: "openChest(idempotencyKey:)", arguments: [idempotencyKey])
//           return try openChestStubResult.get()
//       }
//   }
//
// 用法 2：struct / actor 用组合
//
//   actor MockSomeActor: SomeProtocol {
//       private let mockBase = MockBase()
//       func doStuff(arg: Int) async {
//           await mockBase.recordAsync(method: "doStuff(arg:)", arguments: [arg])
//       }
//   }
//
// 线程安全：内部 NSLock 保护 invocations / lastArguments；多 task 调用同一 mock 不污染。
//
// 设计参考：
// - server 端 `internal/service/sample/MockSampleRepo`（testify/mock 模式 —— 用 m.Called 记录调用）
// - lesson 2026-04-26-urlprotocol-stub-global-state.md（NSLock + snapshot 原子读模式）

import Foundation

/// `MockBase`：手写 mock 通用基类，提供 invocations 记录 + lastArguments 字段 + NSLock 线程安全。
///
/// 子类典型用法（class 继承）：
/// 1. `final class MockXxx: MockBase, XxxProtocol, @unchecked Sendable { }`
/// 2. 在协议方法里 `record(method: "<funcName>", arguments: [...])` 一行
/// 3. stub 字段（如 `var stubResult: Result<...>`）由子类自己声明
///
/// `@unchecked Sendable` 标注：MockBase 内部 NSLock 已保证线程安全，但 Swift 类型系统不会自动推导
/// "持有 NSLock 即 Sendable"；让子类显式 `@unchecked Sendable` 声明意图（与 MockURLSession 同模式）。
public class MockBase {
    /// 调用记录（每次 record() 追加一条）。线程安全。
    public private(set) var invocations: [String] = []

    /// 最近一次调用的参数（任意类型 array）。线程安全。
    public private(set) var lastArguments: [Any] = []

    /// 每个方法名 → 调用次数。线程安全。
    public private(set) var callCounts: [String: Int] = [:]

    private let lock = NSLock()

    public init() {}

    /// 记录一次方法调用：`invocations` 追加方法名；`lastArguments` 覆写为本次参数；`callCounts[method] += 1`。
    /// - Parameters:
    ///   - method: 方法签名字符串（建议 "funcName(label1:label2:)" 风格便于断言）
    ///   - arguments: 本次实参 array（任意类型；用于断言"上次传的是什么"）
    public func record(method: String, arguments: [Any] = []) {
        lock.lock()
        defer { lock.unlock() }
        invocations.append(method)
        lastArguments = arguments
        callCounts[method, default: 0] += 1
    }

    /// 快照式读 invocations（避免迭代过程中被并发写入影响）。
    public func invocationsSnapshot() -> [String] {
        lock.lock()
        defer { lock.unlock() }
        return invocations
    }

    /// 快照式读 callCounts。
    public func callCountsSnapshot() -> [String: Int] {
        lock.lock()
        defer { lock.unlock() }
        return callCounts
    }

    /// 重置所有记录（测试 tearDown 用）。线程安全。
    public func reset() {
        lock.lock()
        defer { lock.unlock() }
        invocations.removeAll(keepingCapacity: true)
        lastArguments.removeAll(keepingCapacity: true)
        callCounts.removeAll(keepingCapacity: true)
    }

    /// 断言指定方法是否被调用过（次数 >= 1）。
    public func wasCalled(method: String) -> Bool {
        callCountsSnapshot()[method, default: 0] > 0
    }

    /// 断言指定方法的调用次数。
    public func callCount(of method: String) -> Int {
        callCountsSnapshot()[method, default: 0]
    }
}

/// 通用 mock 错误：用于 stub 未配置时的 sentinel 错误。
public enum MockError: Error, Equatable {
    case notStubbed
    case unexpectedCall(String)
}
```

**关键约束**：

- 用 `class` 而非 `struct` / `actor`：MockBase 必须能被业务 mock **继承**（最常见用法）；struct 不能继承；actor 强制 async 调用，与 `MockURLSession` / `MockAPIClient` 同步 record 方法签名不一致
- `@unchecked Sendable` 由**子类**声明（不在 MockBase 上）：MockBase 内部 NSLock 已保证线程安全，但子类持有的 stub 字段（如 `var stubResult: Result<...>`）的 Sendable 性由子类决策；让子类显式标注，避免 MockBase 替子类做隐式承诺
- 所有 public API（`record` / `reset` / `wasCalled` / `callCount(of:)` / `invocationsSnapshot` / `callCountsSnapshot`）都包 NSLock：lesson `2026-04-26-urlprotocol-stub-global-state.md` 第 50 行 "用锁保护读写；提供原子 snapshot helper" 直接复用
- `record(method:arguments:)` 的 `arguments: [Any] = []` 参数是 Existential `Any` 数组：`Any` 不是 Sendable 会触发 Swift 6 warning，但本类只在测试 target 用，不夸过 actor 边界，warning 可忽略；如未来 Swift 6 strict concurrency 升级，再决策包 `Sendable` wrapper
- `MockError` enum：sentinel 错误用于 stub 未配置时抛出（业务 mock 自定 stub 字段默认值用 `.failure(MockError.notStubbed)`），与 `MockAPIClient` 用 `APIError.decoding(...)` 包装错误模式互补
- **不**实装 `expect(method:times:)` / `verify()` 等动态期望 API：那是 testify/mock 风格的复杂行为，Swift 实装手写成本太高（无 reflection）；保持 MockBase 极简 = `record + assert` 两段式
- 顶部仅 `import Foundation`：不依赖 XCTest（让 MockBase 也能在非 test scope 引用，比如未来某个集成测试 helper）
- 文件**位于** `iphone/PetAppTests/Helpers/MockBase.swift`：`Helpers/` 是新目录，靠 `iphone/project.yml` `sources: - PetAppTests` glob 自动纳入

**AC3 — `iphone/PetAppTests/Helpers/AsyncTestHelpers.swift`：async / Combine 测试 helper**

新建 `iphone/PetAppTests/Helpers/AsyncTestHelpers.swift`：

```swift
// AsyncTestHelpers.swift
// Story 2.7 · ADR-0002 §3.2 落地：async/await 测试 helper。
//
// 提供两个 helper：
// 1. assertThrowsAsyncError(_:_:matcher:): 断言一段 async throws 表达式必抛错；ADR-0002 §3.2 已知坑第 3 条要求落地
// 2. awaitPublishedChange(on:keyPath:count:timeout:): 等待 ObservableObject 的 @Published 字段变化 N 次；
//    ADR-0002 §3.2 "场景 1: 多次值变化"标准模式

import Combine
import Foundation
import XCTest

/// 断言一段 async throws 表达式抛出错误。
///
/// - Parameters:
///   - expression: 异步表达式（可抛错）
///   - message: 失败时的描述（XCTFail message）
///   - matcher: 可选的错误匹配闭包；返回 false 时断言失败（即"抛错了，但不是期望的错"）
///
/// 用法：
/// ```
/// await assertThrowsAsyncError(try await sut.doSomething()) { error in
///     guard case APIError.unauthorized = error else { return false }
///     return true
/// }
/// ```
///
/// 实装参考 ADR-0002 §3.2 已知坑第 3 条："`await assertThrowsAsyncError(...)` helper（Story 2.7 落地一个
/// helper 函数，包装 `do { try await ...; XCTFail(...) } catch { ... }` 样板）"。
public func assertThrowsAsyncError<T>(
    _ expression: @autoclosure () async throws -> T,
    _ message: @autoclosure () -> String = "expected throw, got value",
    file: StaticString = #filePath,
    line: UInt = #line,
    matcher: ((Error) -> Bool)? = nil
) async {
    do {
        _ = try await expression()
        XCTFail(message(), file: file, line: line)
    } catch {
        if let matcher = matcher, !matcher(error) {
            XCTFail("error did not match: \(error)", file: file, line: line)
        }
    }
}

/// 等待 ObservableObject 上某个 @Published 字段变化 `count` 次（默认 1 次）后返回收集到的值数组。
///
/// - Parameters:
///   - object: ObservableObject 实例
///   - keyPath: 指向 @Published 字段的 keyPath（用 `\.fieldName` 写法）
///   - count: 期望的值变化次数（含 initial 值；Combine sink 默认收 initial）
///   - timeout: 超时秒数（默认 1 秒）
///
/// 用法：
/// ```
/// let viewModel = SampleViewModel(useCase: mockUseCase)
/// async let trigger: Void = viewModel.load()  // 触发副作用
/// let values = try await awaitPublishedChange(on: viewModel, keyPath: \.status, count: 3)
/// XCTAssertEqual(values, [.idle, .loading, .ready])
/// _ = try await trigger
/// ```
///
/// 实装参考 ADR-0002 §3.2 "场景 1: 观察 @Published / Combine publisher 的多次值变化"。
public func awaitPublishedChange<O: ObservableObject, V>(
    on object: O,
    keyPath: KeyPath<O, V>,
    count: Int = 1,
    timeout: TimeInterval = 1.0,
    file: StaticString = #filePath,
    line: UInt = #line
) async throws -> [V] where O.ObjectWillChangePublisher == ObservableObjectPublisher {
    // ObservableObjectPublisher 在每次 @Published 字段变化前发出值；用它驱动观察
    var collected: [V] = []
    let lock = NSLock()
    let expectation = XCTestExpectation(description: "awaitPublishedChange(\(keyPath))")
    expectation.expectedFulfillmentCount = count

    let cancellable = object.objectWillChange
        .sink { _ in
            // objectWillChange 发出时字段尚未更新；用 DispatchQueue.main.async 让出一拍读到新值
            DispatchQueue.main.async {
                let value = object[keyPath: keyPath]
                lock.lock()
                collected.append(value)
                lock.unlock()
                expectation.fulfill()
            }
        }

    let result = await XCTWaiter.fulfillment(of: [expectation], timeout: timeout)
    cancellable.cancel()

    if result != .completed {
        XCTFail(
            "awaitPublishedChange timed out after \(timeout)s; got \(collected.count)/\(count) changes",
            file: file,
            line: line
        )
    }

    lock.lock()
    defer { lock.unlock() }
    return collected
}
```

**关键约束**：

- `assertThrowsAsyncError` 的 `matcher` 可选：调用方仅想断言"抛了错"时不传；想断言"抛了特定错"传 matcher（避免 XCTest 没有原生 `XCTAssertThrowsAsync` 的窘境）
- `awaitPublishedChange` 用 `objectWillChange + DispatchQueue.main.async`：ObservableObject 的 `objectWillChange` 是字段变更**之前**触发，需要 dispatch 一拍才能读到新值（与 SwiftUI 内部观察机制一致）
- `awaitPublishedChange` 的 timeout 默认 1.0 秒：避免 CI 上偶发卡死；测试场景中 1 秒已远超合理上限（state 切换都是 sub-millisecond）
- `XCTWaiter.fulfillment(of:timeout:)` 是 async API（Xcode 14+ 支持），与 ADR-0002 §3.2 "async/await 主流" 精神一致
- **不**包 `XCTestCase` 实例方法：让两个 helper 全部 free function，调用方在 setUp / 测试内自由用，无类继承耦合
- 文件**位于** `iphone/PetAppTests/Helpers/AsyncTestHelpers.swift`

**AC4 — `iphone/PetApp/Shared/Testing/SampleTypes.swift`：placeholder production type（DEBUG-only）**

新建 `iphone/PetApp/Shared/Testing/SampleTypes.swift`：

```swift
// SampleTypes.swift
// Story 2.7 测试基础设施模板：SampleUseCase + SampleViewModel placeholder type。
//
// 存在目的：让 PetAppTests/Helpers/SampleViewModelTests.swift（AC5）有真正的被测对象，
// 即"业务相关 mock 单元测试"模板示范（满足 epics.md Story 2.7 AC "至少存在一条业务相关 mock 单元测试"）。
//
// 不是真业务代码：
// - 不导出给真业务 Feature 使用
// - 用 #if DEBUG 包裹，Release build 自动 strip
// - 命名以 `Sample` 前缀避免与未来真业务命名冲突
//
// 设计参考：
// - server 端 `internal/service/sample/service.go`（同样是测试基础设施模板，service 层 placeholder）
// - 后续业务 ViewModel（HomeViewModel / RoomViewModel / ChestViewModel 等）按本模板结构填业务

#if DEBUG

import Combine
import Foundation

/// 演示性 UseCase 协议：异步 throws 单方法。
public protocol SampleUseCase: Sendable {
    func execute(input: String) async throws -> Int
}

/// 演示性 ViewModel：通过 SampleUseCase 取数据，driver 状态机切换。
@MainActor
public final class SampleViewModel: ObservableObject {
    public enum Status: Equatable {
        case idle
        case loading
        case ready(value: Int)
        case failed(message: String)
    }

    @Published public private(set) var status: Status = .idle

    private let useCase: SampleUseCase

    public init(useCase: SampleUseCase) {
        self.useCase = useCase
    }

    /// 触发一次 useCase 调用，driver `status` 走 `.idle → .loading → .ready/.failed`。
    public func load(input: String) async {
        status = .loading
        do {
            let value = try await useCase.execute(input: input)
            status = .ready(value: value)
        } catch {
            status = .failed(message: "\(error)")
        }
    }
}

#endif
```

**关键约束**：

- 整个文件包 `#if DEBUG` / `#endif`：Release build 完全 strip 掉，零 binary footprint
- `@MainActor` 标注 `SampleViewModel`：与 `HomeViewModel`（Story 2.5 落地）模式一致
- `Status` enum `Equatable`：方便测试断言（`XCTAssertEqual(viewModel.status, .ready(value: 42))`）
- `SampleUseCase` 标 `Sendable`：与未来真业务 UseCase 协议（如 `PingUseCase`）模式一致
- `init(useCase:)` 显式注入：DI 模式与 Story 2.5 `DefaultPingUseCase(client:)` 一致
- 文件**位于** `iphone/PetApp/Shared/Testing/SampleTypes.swift`：`Shared/Testing/` 子目录是新建的（其它 Shared 子目录有 `Constants/` / `ErrorHandling/`），靠 `iphone/project.yml` 既有 `sources: - PetApp` glob 自动纳入

**AC5 — `iphone/PetAppTests/Helpers/SampleViewModelTests.swift`：业务相关 mock 单测模板**

新建 `iphone/PetAppTests/Helpers/SampleViewModelTests.swift`：

```swift
// SampleViewModelTests.swift
// Story 2.7 · 业务相关 mock 单元测试模板（epics.md Story 2.7 AC 强制：≥ 1 条）。
//
// 后续业务 story 写 ViewModel 测试时，**直接复制本文件结构**，改 type / mock / case 名即可。
// 文件结构：
//   1. 本地 mock（继承 MockBase）
//   2. setUp / tearDown
//   3. ≥ 3 case：happy + error + state-transition

import Combine
import XCTest
@testable import PetApp

#if DEBUG

@MainActor
final class SampleViewModelTests: XCTestCase {

    // MARK: - Local Mock（继承 MockBase；ADR-0002 §3.1 "至少记录 invocations + lastArguments"）

    final class MockSampleUseCase: MockBase, SampleUseCase, @unchecked Sendable {
        var stubResult: Result<Int, Error> = .failure(MockError.notStubbed)

        func execute(input: String) async throws -> Int {
            record(method: "execute(input:)", arguments: [input])
            return try stubResult.get()
        }
    }

    var sut: SampleViewModel!
    var mockUseCase: MockSampleUseCase!

    override func setUp() {
        super.setUp()
        mockUseCase = MockSampleUseCase()
        sut = SampleViewModel(useCase: mockUseCase)
    }

    override func tearDown() {
        sut = nil
        mockUseCase = nil
        super.tearDown()
    }

    // MARK: - Tests

    /// happy: useCase 返回值 → ViewModel 状态切换 .idle → .loading → .ready
    func testLoadHappyPath() async {
        mockUseCase.stubResult = .success(42)

        await sut.load(input: "hello")

        XCTAssertEqual(sut.status, .ready(value: 42))
        XCTAssertEqual(mockUseCase.callCount(of: "execute(input:)"), 1)
        XCTAssertEqual(mockUseCase.lastArguments.first as? String, "hello")
    }

    /// edge: useCase 抛错 → ViewModel 状态切到 .failed
    func testLoadErrorPath() async {
        struct DemoError: Error {}
        mockUseCase.stubResult = .failure(DemoError())

        await sut.load(input: "world")

        if case .failed(let message) = sut.status {
            XCTAssertTrue(message.contains("DemoError"))
        } else {
            XCTFail("expected .failed, got \(sut.status)")
        }
        XCTAssertTrue(mockUseCase.wasCalled(method: "execute(input:)"))
    }

    /// happy: 状态转换序列被正确记录（演示 awaitPublishedChange 用法）
    func testStatusTransitionsCaptured() async throws {
        mockUseCase.stubResult = .success(7)

        // 触发副作用 + 等待 status 变化（initial idle + loading + ready = 3 次）
        async let _: Void = sut.load(input: "x")
        let captured = try await awaitPublishedChange(
            on: sut,
            keyPath: \.status,
            count: 2,   // loading + ready（idle 是 initial 不通过 objectWillChange 走）
            timeout: 1.0
        )

        // 至少能看到 loading（first transition 必有），最终 ready
        XCTAssertEqual(sut.status, .ready(value: 7))
        XCTAssertGreaterThanOrEqual(captured.count, 1)
    }

    /// edge: assertThrowsAsyncError helper 用法演示
    func testAssertThrowsAsyncErrorHelper() async {
        struct StubError: Error, Equatable {}
        mockUseCase.stubResult = .failure(StubError())

        await assertThrowsAsyncError(try await mockUseCase.execute(input: "x")) { error in
            error is StubError
        }
    }
}

#endif
```

**关键约束**：

- `@testable import PetApp`：访问 `SampleViewModel` / `SampleUseCase`（virtually `internal` 由 `public` 标注，但 `@testable` 不伤大雅，与 Story 2.4 / 2.5 测试模式一致）
- 整个 class 包 `#if DEBUG` / `#endif`：与 `SampleTypes.swift` 同步 strip
- `@MainActor` 标 class：测试需要在 main actor 上访问 `@MainActor SampleViewModel`
- ≥ 3 case（实际 4 case）：happy + error + state-transition + helper-demo
- 命名以 `test` 前缀（XCTest 约定）；方法签名 `func testXxx() async [throws]`
- Mock `MockSampleUseCase` 继承 `MockBase` + 实现 `SampleUseCase` + 标 `@unchecked Sendable`：演示 MockBase 的标准用法
- `record(method: "execute(input:)", arguments: [input])` 与 `mockUseCase.callCount(of:)` / `wasCalled(method:)`：演示 MockBase 的断言 API
- `awaitPublishedChange` + `assertThrowsAsyncError`：演示 AsyncTestHelpers 的两个 helper 都被用过
- 文件**位于** `iphone/PetAppTests/Helpers/SampleViewModelTests.swift`：与 MockBase / AsyncTestHelpers 同目录

**AC6 — `iphone/docs/CI.md`：CI 跑法文档化（不写真 CI YAML）**

新建 `iphone/docs/CI.md`：

```markdown
# iPhone App CI / 本地测试入口

## 1. 本地与 CI 统一入口

```bash
bash iphone/scripts/build.sh             # build only（含 xcodegen generate）
bash iphone/scripts/build.sh --test      # build + 单元测试
bash iphone/scripts/build.sh --uitest    # build + UI 测试（XCUITest）
bash iphone/scripts/build.sh --test --uitest --coverage-export   # 全跑 + 导出 coverage
bash iphone/scripts/build.sh --clean --test                       # 清干净再跑测试
```

**契约来源**：
- ADR-0002 §3.4 "CI 跑法"：本地与 CI 用同一 wrapper
- 与 server 端 `bash scripts/build.sh --test` 风格对齐（CLAUDE.md §Build & Test）

**artifacts 路径**：
- `iphone/build/test-results.xcresult` — 单元测试结果包（含 coverage 数据 / simulator 日志）
- `iphone/build/test-results-ui.xcresult` — UI 测试结果包
- `iphone/build/coverage.json` — `--coverage-export` 时产出（xcrun xccov 导出）
- `iphone/build/DerivedData/` — Xcode 增量编译缓存

## 2. Destination 三段 fallback

`build.sh` 自动 resolve destination：
1. `platform=iOS Simulator,name=iPhone 17,OS=latest`（首选；Xcode 26 默认）
2. `platform=iOS Simulator,OS=latest`（任意 iOS Simulator；Xcode 16 兼容）
3. `platform=iOS Simulator,id=<UUID>`（xcrun simctl 取第一个可用）

详见 ADR-0002 §3.4 已知坑第 2 条 P1 fix。

## 3. 未来 GitHub Actions 接入点（占位章节）

**当前状态**：未实装真 CI workflow YAML（重启阶段 + 单开发者无 CI runner）。

**未来接入时**（任意时点触发，至少节点 1 后），按以下模板写 `.github/workflows/iphone-ci.yml`：

```yaml
# 仅作示例草图，真接入时按当时 GitHub Actions 最新约定调整
name: iPhone CI
on: [push, pull_request]
jobs:
  build:
    runs-on: macos-14   # 最低 Xcode 16；macos-latest 可能滚动到 Xcode 26+
    steps:
      - uses: actions/checkout@v4
      - name: Install xcodegen
        run: brew install xcodegen
      - name: Build + Test
        run: bash iphone/scripts/build.sh --test --coverage-export
      - uses: actions/upload-artifact@v4
        with:
          name: iphone-test-results
          path: iphone/build/test-results.xcresult
```

**真接入时的 spike 内容**（不属本 story scope；归 Epic 3 Story 3.3 或更晚 spike）：
- runner 选型（macos-14 vs macos-latest 取舍 — Xcode 版本浮动）
- cache 策略（DerivedData / SPM 包是否 cache）
- artifact upload 策略（.xcresult 体积大 / 过期清理）
- 双端 CI 编排（server `bash scripts/build.sh --test` + iphone `bash iphone/scripts/build.sh --test` 是否同 job）
- destination fallback 在 CI runner 上的实际机型分布（不同 macos runner 默认 simulator 不同）

详见 ADR-0002 §1.1 "兼容性说明（单开发者重启阶段决策）" + §6 TODO "多人协作 / CI 兼容矩阵 spike"。

## 4. 排错手册

| 症状 | 可能原因 | 处置 |
|---|---|---|
| `xcodegen: command not found` | brew 未装 xcodegen | `brew install xcodegen` |
| `xcrun: error: unable to find utility "simctl"` | Xcode Command Line Tools 未装 | `xcode-select --install` |
| `iPhone 17` destination 失败 | Xcode 16 等旧版本默认机型不含 iPhone 17 | build.sh 会自动 fallback 到 `OS=latest` 或 `xcrun simctl` UUID；查看 `=== resolved destination: ... ===` 输出确认实际用的 |
| 测试在 Simulator 上首次启动慢 | Simulator 冷启动 + Xcode 编译缓存未热 | 第二次跑会快；CI 上加 `actions/cache` 保 DerivedData |
| `BUILD SUCCESS` 但实际有失败 | 没有 `set -euo pipefail` 或 fast-fail 路径绕过 | 不应出现；如出现 grep `FAIL:` 看实际位置 |

## 5. 与既有 server CI 命令面对照

| 端 | 入口 | scope |
|---|---|---|
| server | `bash scripts/build.sh --test` | Go 单测 + race + coverage |
| iPhone | `bash iphone/scripts/build.sh --test` | XCTest 单测 + coverage |
| iPhone (UI) | `bash iphone/scripts/build.sh --uitest` | XCUITest |

`bash scripts/build.sh --test`（server）与 `bash iphone/scripts/build.sh --test`（iPhone）**互不干扰**：
- artifacts 路径隔离（`build/` vs `iphone/build/`）
- 工具栈完全独立（go vs xcodebuild）
- 跨端 dev 切换零认知摩擦（同 `--test` 语义）
```

**关键约束**：

- **不写**真 GitHub Actions YAML 文件：仅在 §3 章节用代码块举例
- **不**新建 `.github/workflows/iphone-ci.yml`：违反"不实装真实 CI workflow"红线
- 文件**位于** `iphone/docs/CI.md`：与 ADR-0002 §3.3 提到的 `iphone/README.md`（Story 2.10 范围）平级；本 story 不动 README

**AC7 — 验证：build.sh + 全测试在本机跑通**

完成本 story 后，**手动**跑以下命令验证：

```bash
# 1. build only
bash iphone/scripts/build.sh
# 期望: BUILD SUCCESS；iphone/build/DerivedData/ 产出

# 2. build + unit tests
bash iphone/scripts/build.sh --test
# 期望: BUILD SUCCESS；含已有所有测试 + 新增 SampleViewModelTests 全绿；iphone/build/test-results.xcresult 产出

# 3. build + UI tests
bash iphone/scripts/build.sh --uitest
# 期望: BUILD SUCCESS；HomeUITests / NavigationUITests 全绿（Story 2.2 既有）

# 4. coverage export
bash iphone/scripts/build.sh --test --coverage-export
# 期望: iphone/build/coverage.json 产出（xcrun xccov view --report --json）

# 5. clean
bash iphone/scripts/build.sh --clean
# 期望: iphone/build/DerivedData/ 被删；下次 build 会全量编译
```

**至少完成 #1 / #2 / #5 三组合**（与 server `--test` / `--race --test` 三组合手工验证模式对齐，参考 Story 1.7 AC10）；#3 / #4 可选（CI 上跑过即可）。把每条命令的 stdout 末尾贴 Completion Notes（特别是 `BUILD SUCCESS` 字样 + xcresult 路径）。

**AC8 — 验证：既有测试零回归**

`bash iphone/scripts/build.sh --test` 必须确认 Story 2.2 / 2.3 / 2.4 / 2.5 / 2.6 既有所有测试**全绿**：

- `SheetTypeTests` / `AppCoordinatorTests` / `RootViewWireTests` / `AppContainerTests`（Story 2.2 / 2.3 / 2.5）
- `HomeViewModelTests` / `HomeViewModelPingTests` / `HomeViewTests`（Story 2.2 / 2.5）
- `APIClientTests` / `APIClientIntegrationTests`（Story 2.4）
- `PingUseCaseTests` / `PingUseCaseIntegrationTests`（Story 2.5）
- `AppErrorMapperTests` / `ErrorPresenterTests` / `ErrorComponentSnapshotTests`（Story 2.6）
- `SampleViewModelTests`（本 story 新增 ≥ 3 case）

**任一既有测试因本 story 改动 fail → 必须立刻定位**（最可能根因：MockBase 命名碰撞 / SampleTypes 命名碰撞 / `Helpers/` 目录被 PetAppTests glob 重复纳入 PetApp target）；不能"先 commit 再修"。

**AC9 — `git status` 净化**

完成本 story 后 `git status` 应仅含**新增**：

- `iphone/scripts/build.sh`
- `iphone/PetAppTests/Helpers/MockBase.swift`
- `iphone/PetAppTests/Helpers/AsyncTestHelpers.swift`
- `iphone/PetAppTests/Helpers/SampleViewModelTests.swift`
- `iphone/PetApp/Shared/Testing/SampleTypes.swift`
- `iphone/docs/CI.md`

以及**修改**：

- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen regen 副作用，预期）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（`2-7-...`: backlog → ready-for-dev → in-progress → review）
- `_bmad-output/implementation-artifacts/2-7-ios-测试基础设施搭建.md`（本 story 文件：Tasks 勾选 / Dev Agent Record / Status 流转）

**绝对不应出现**：

- `ios/` 下任何 diff
- `server/` 下任何 diff
- `iphone/PetApp/{App,Core,Features,Resources}` 下任何 diff（**除** `Shared/Testing/SampleTypes.swift` 这一个新文件外）
- `iphone/PetAppTests/{App,Core,Features,Shared}` 下任何 diff（既有测试零改动）
- `iphone/PetAppUITests/` 下任何 diff
- `iphone/project.yml` diff
- `iphone/scripts/install-hooks.sh` / `iphone/scripts/git-hooks/pre-commit` diff
- `.gitignore` diff（`iphone/build/` 已在第 48 行）
- `CLAUDE.md` diff（本 story 不改 CLAUDE.md；`build.sh` 命令面对照可在 Story 2.10 README 里写）
- `docs/` 下任何 diff（除 `iphone/docs/CI.md` 这一个新文件，注意它在 `iphone/docs/` 不在 `docs/`）

## Tasks / Subtasks

- [x] **T1** — `iphone/scripts/build.sh` wrapper（AC1）
  - [x] T1.1 写脚本主体（参数解析 / require_tool / xcodegen / build / test / coverage-export / fail-fast / set -euo pipefail）
  - [x] T1.2 实装 destination 三段 fallback 链（primary `iPhone 17,OS=latest` → secondary `OS=latest` → simctl UUID）
  - [x] T1.3 chmod +x；shebang `#!/usr/bin/env bash`
  - [x] T1.4 grep 验证：脚本内不含 `OS=26.4` / `OS=17.0` 等 hardcoded 值；artifacts 路径全部 `iphone/build/`

- [x] **T2** — `MockBase.swift`（AC2）
  - [x] T2.1 写 MockBase class（NSLock + invocations / lastArguments / callCounts + record / reset / wasCalled / callCount(of:)）
  - [x] T2.2 写 MockError enum（notStubbed / unexpectedCall(String)）
  - [x] T2.3 文件头注释明示"已存在 networking mock 不强制迁移"

- [x] **T3** — `AsyncTestHelpers.swift`（AC3）
  - [x] T3.1 写 assertThrowsAsyncError(_:_:matcher:)
  - [x] T3.2 写 awaitPublishedChange(on:keyPath:count:timeout:)（用 objectWillChange + DispatchQueue.main.async）

- [x] **T4** — `SampleTypes.swift`（AC4）
  - [x] T4.1 写 SampleUseCase protocol
  - [x] T4.2 写 SampleViewModel @MainActor class（status enum / load(input:)）
  - [x] T4.3 整个文件 #if DEBUG / #endif 包裹

- [x] **T5** — `SampleViewModelTests.swift`（AC5）
  - [x] T5.1 MockSampleUseCase 继承 MockBase
  - [x] T5.2 testLoadHappyPath
  - [x] T5.3 testLoadErrorPath
  - [x] T5.4 testStatusTransitionsCaptured（演示 awaitPublishedChange）
  - [x] T5.5 testAssertThrowsAsyncErrorHelper（演示 assertThrowsAsyncError）
  - [x] T5.6 整个 class #if DEBUG / #endif 包裹

- [x] **T6** — `iphone/docs/CI.md`（AC6）
  - [x] T6.1 §1 本地与 CI 统一入口（5 条命令面）
  - [x] T6.2 §2 Destination 三段 fallback 说明
  - [x] T6.3 §3 未来 GitHub Actions 接入点（占位 YAML 草图，不写真 .github/workflows/）
  - [x] T6.4 §4 排错手册
  - [x] T6.5 §5 与 server 端命令面对照

- [x] **T7** — 手动验证（AC7 / AC8）
  - [x] T7.1 `bash iphone/scripts/build.sh` → BUILD SUCCESS
  - [x] T7.2 `bash iphone/scripts/build.sh --test` → BUILD SUCCESS + 全测试绿（90 tests, 0 failures）
  - [x] T7.3 `bash iphone/scripts/build.sh --clean` → BUILD SUCCESS（clean 后 DerivedData 重建）
  - [x] T7.4 既有测试零回归（90 tests 全绿，含 SampleViewModelTests 5 个新增 case）
  - [x] T7.5 完成命令面输出贴 Completion Notes

- [x] **T8** — git status 净化（AC9）
  - [x] T8.1 `git status` 检查文件清单严格匹配 AC9
  - [x] T8.2 `git diff ios/` 无输出
  - [x] T8.3 `git diff server/` 无输出
  - [x] T8.4 `git diff iphone/PetApp/{App,Core,Features,Resources}/` 无输出
  - [x] T8.5 `git diff iphone/PetAppTests/{App,Core,Features,Shared}/` 无输出
  - [x] T8.6 `git diff iphone/project.yml` 无输出（xcodegen regen 只改 .pbxproj 不改 yml）

- [x] **T9** — 收尾
  - [x] T9.1 Completion Notes 补全
  - [x] T9.2 File List 填充
  - [x] T9.3 状态流转 `ready-for-dev → in-progress → review`

## Dev Notes

### 项目关键约束（必读，勿绕过）

1. **`@unchecked Sendable` 是子类的责任，不是 MockBase 的责任**：MockBase 内部 NSLock 已保证 invocations / lastArguments / callCounts 三字段线程安全，但子类自定 stub 字段（如 `var stubResult: Result<...>`）的 Sendable 性由子类决策。让 MockBase 不标 `@unchecked Sendable`，强迫子类显式标注 —— 与 Story 2.4 `MockURLSession: URLSessionProtocol, @unchecked Sendable` 风格一致

2. **`#if DEBUG` 包裹 SampleTypes / SampleViewModelTests 必须**：理由 ① Release build 不需要 sample 占位；② 防止 sample 类型被误用进真业务（编译期硬隔离）；③ 与 server 端 `internal/service/sample/` 全部走 internal 包不导出对应（iOS 没有 internal 包概念，用 `#if DEBUG` 模拟）

3. **`MockBase` 不要标 `final`**：必须可继承（业务 mock 的最常用 mode）；`final class` 阻止继承会让 AC2 文件头注释 "用法 1：class 继承" 直接错

4. **destination fallback 三段链**：ADR-0002 §3.4 已知坑第 2 条 P1 fix 强制 —— 不能"硬编码 iPhone 17 + 失败时报错"，必须真 fallback。验证：把脚本里 `iPhone 17` 改成不存在的机型（如 `iPhone 999`）跑 `bash iphone/scripts/build.sh`，应能 fallback 到 secondary `OS=latest` 仍 BUILD SUCCESS

5. **`xcodegen generate` 每次都跑**：与 Story 2.4 / 2.5 / 2.6 既有惯例一致；脚本不引入"only regen if .yml changed"优化（开发阶段过度优化 = 偶发漏 regen）。CI 上 .pbxproj 通常不 commit，每次都 regen 是正常路径

6. **artifacts 落 `iphone/build/`**：`iphone/build/` 已在 `.gitignore` 第 48 行；server 端用 repo root `build/`，两者**严格隔离**。如果有人写错路径让 iPhone artifacts 落 `build/`，会污染 server CI（artifact upload）+ git status

7. **Helpers 目录靠 glob 自动纳入**：`iphone/project.yml` 里 `PetAppTests` target 的 `sources: - PetAppTests` 是 glob，自动包含 `Helpers/` 子目录所有 .swift 文件。**不**改 yml；如果新建 .swift 文件后 build 报 "cannot find type X"，是因为没跑 `xcodegen generate`

8. **`SampleUseCase` 接 `Sendable`**：与未来真业务 protocol（`PingUseCaseProtocol` 等）一致；Swift 6 strict concurrency 默认要求

9. **`@MainActor SampleViewModel`**：与 Story 2.5 `HomeViewModel` 模式一致；测试 class 也要标 `@MainActor` 才能调用 sut 方法（XCTest 自动 await）

10. **`awaitPublishedChange` 的 `objectWillChange` 时序问题**：`objectWillChange` 在字段变更**之前**触发，所以 sink 闭包里读 `object[keyPath: keyPath]` 会读到**旧值**。修复办法：用 `DispatchQueue.main.async` 让出一拍再读 —— 这是 SwiftUI 内部观察机制的标准 workaround。AC3 实装已带这一行

11. **`assertThrowsAsyncError` 的 `@autoclosure` 修饰符**：让调用方写 `assertThrowsAsyncError(try await sut.x())` 而非 `assertThrowsAsyncError({ try await sut.x() })`；与 `XCTAssertThrowsError` 同模式

12. **不引入 swift-format 集成进 build.sh**：build.sh 当前只跑 build / test / coverage；swift-format 由 `iphone/scripts/install-hooks.sh` + pre-commit hook 单独管（Story 2.2 已落地占位 hook，真实 swift-format 调用是 tech debt）。本 story **不**接 hook 实装路径

13. **`-only-testing:PetAppUITests`**：在 AC1 build.sh 模板里 UI test 路径用了 `-only-testing:PetAppUITests`，让 xcodebuild 只跑 UI test target 不重复跑 unit test。这是节省时间的优化；如果 dev 觉得"全 scheme test 一次跑完"更简单，可以去掉这个 flag（行为变成 `--uitest` 隐含 `--test`，时间多 ~30s）。两种都能跑通 AC7

14. **3 段 destination fallback 的 grep 检测策略**：`xcodebuild -showdestinations` 输出的 destination 列表需要 grep `iPhone 17` 字符串；这是 substring match，会把 "iPhone 17 Pro" / "iPhone 17 Pro Max" 也命中。这是**有意**的 —— 任意含 "iPhone 17" 的 simulator 都接受；如果没有任一含 17 的，再 fallback。**不**用 exact match（`grep -F "name=iPhone 17,"`）—— 太严格会让 "iPhone 17" 命中失败而退到 secondary，反而绕过了 primary

### 为什么不在本 story 做这些

- **真 GitHub Actions YAML**：见上文"不涉及"第 1 条；归 Epic 3 Story 3.3 或更晚 spike
- **`MockBase` 的 expect/verify 动态期望 API**：Swift 没 reflection，手写动态 expect 成本远超收益；保持极简
- **swift-testing 框架引入**：ADR-0002 §3.2 选定 XCTest only；引入 swift-testing 是另起 spike
- **WebSocket / AsyncStream 测试 helper**：节点 4 / 5 才出现的需求；ADR-0002 §3.2 已知坑第 1 条已记
- **强制 Story 2.4 / 2.5 老 mock 迁移到 MockBase**：会破坏既有测试；老 mock 模式上已经是 MockBase spirit；范围红线明确"不动既有测试"

### 与 Story 1.5 / 1.7（server 端类比）的对照

| 维度 | Server (Story 1.5 + 1.7) | iPhone (本 Story 2.7) |
|---|---|---|
| ADR | ADR-0001（test stack） | ADR-0002（iOS stack）|
| Mock 框架 | testify/mock 手写 + sqlmock + miniredis | XCTest only 手写 + MockBase 通用基类 |
| 异步测试 | go test 自动 | async/await 主流 + AsyncTestHelpers |
| 入口 wrapper | `bash scripts/build.sh --test/--race/--coverage/--integration/--devtools` | `bash iphone/scripts/build.sh --test/--uitest/--clean/--coverage-export` |
| 业务 sample | `internal/service/sample/` package + `MockSampleRepo` | `iphone/PetApp/Shared/Testing/SampleTypes.swift` (#if DEBUG) + `MockSampleUseCase` |
| Test helpers | `internal/pkg/testing/{helpers.go, slogtest/}` | `iphone/PetAppTests/Helpers/{MockBase.swift, AsyncTestHelpers.swift}` |
| CI 文档 | CLAUDE.md §Build & Test 直接写命令面 | `iphone/docs/CI.md` 独立文档（暂不动 CLAUDE.md，归 Story 2.10 README 收口）|

跨端**风格对齐**但代码 / 工具栈完全独立 —— 与 ADR-0002 §3.4 选定理由 1 "跨端 dev 切换零认知摩擦" 对齐。

### 与 Story 2.4 / 2.5 既有 mock 的关系

| 既有 mock | 位置 | 用途 | 本 story 处置 |
|---|---|---|---|
| `MockURLSession` | `PetAppTests/Core/Networking/MockURLSession.swift` | URLSessionProtocol 的手写 mock，Story 2.4 APIClient 单测用 | **不动**；保留为 networking-specific 可复用 mock |
| `StubURLProtocol` | `PetAppTests/Core/Networking/StubURLProtocol.swift` | URLProtocol 子类 fake server，Story 2.4 集成测试用；含 NSLock + snapshot helper | **不动**；保留 + 沉淀的两条 lesson 不撤销 |
| `MockAPIClient` | `PetAppTests/Features/Home/UseCases/MockAPIClient.swift` | APIClientProtocol 手写 mock，Story 2.5 PingUseCase 单测用 | **不动**；保留为 networking-specific 可复用 mock |
| `PingStubURLProtocol` | `PetAppTests/Features/Home/UseCases/PingStubURLProtocol.swift` | 类似 StubURLProtocol 但作用于 ping endpoint，Story 2.5 集成测试用 | **不动** |

**为什么不强制迁移到 MockBase**：① `MockURLSession.invocations: [URLRequest]` 是**类型化** invocations 数组（不是字符串），断言 "上次请求的 URL.path 是什么" 比 MockBase 的 "execute(input:)" + lastArguments[Any] 更直接；② 老 mock 已通过 review，迁移是 net negative；③ MockBase 是**给新业务 mock 用的便利**，不是"取代老 mock 的统一标准"

### Lessons Index（与本 story 相关的过去教训）

- **`docs/lessons/2026-04-26-urlprotocol-stub-global-state.md`** —— 直接相关：MockBase 的 NSLock + snapshot 模式（`invocationsSnapshot()` / `callCountsSnapshot()`）直接继承这条 lesson；任何持有 mutable state 的测试基础设施必须用锁 + 快照
- **`docs/lessons/2026-04-26-urlprotocol-session-local-vs-global.md`** —— 间接相关：本 story 不接 networking layer mock；但 SampleViewModel mock 用的是 ViewModel-level 注入（init 构造时显式传 mock），与 session-local 注入精神一致 —— 没有 process-global state，每个测试自己 new mock
- **`docs/lessons/2026-04-26-jsondecoder-encoder-thread-safety.md`** —— 间接相关：本 story 不持 JSONDecoder；`AsyncTestHelpers.awaitPublishedChange` 内部 sink 用 NSLock 保护 collected 数组（与 lesson 第 36 行"短临界区 + 不持锁调外部回调"原则一致）
- **`docs/lessons/2026-04-26-stateobject-init-vs-bind-injection.md`** —— 间接相关：SampleViewModel `init(useCase:)` 是显式注入而非 `@StateObject`；测试可以直接 new ViewModel + 注入 mock，不踩 @StateObject 的 init-time 注入坑
- **`docs/lessons/2026-04-26-swiftui-task-modifier-reentrancy.md`** —— 间接相关：SampleViewModel.load() 不在 `.task` 内自动重入，是显式 await 调用；测试场景没有 .task 重入风险
- **`docs/lessons/2026-04-26-error-presenter-queue-onretry-loss.md`** —— 间接相关：MockBase 的 `record(method:arguments:)` 是"排队但只记录最后参数"模式（`lastArguments` 覆写），与 ErrorPresenter 队列教训互补 —— 如果未来需要"记录每次调用的所有参数历史"，再扩展 `argumentsHistory: [[Any]]` 字段（本 story 不做）

### Git intelligence（最近 6 个 commit）

- `3b2973d docs(bmad-output): 更新 0002-ios-stack`
- `18c92e8 chore: 更新 CLAUDE`
- `b80fd14 chore(claude): 更新 Bash allowlist`
- `71b5b93 chore(story-2-1): 收官 Story 2.1 + 归档 story 文件`
- `954c28a 常用 claude 默认允许命令`
- `（更早是 Story 2.6 / 2.5 / 2.4 / 2.3 / 2.2 实装 + review 链）`

**最近实装向 commit** 是 Story 2.6 review fix 链（`634c564` ErrorPresenter queue onRetry loss / `3b40ba8` modal overlay shield 等）；Story 2.7 紧随 Epic 2 实装顺序。

**commit message 风格**：Conventional Commits，中文 subject，scope `story-X-Y` / `iphone` / `scripts`。
本 story 建议：`chore(scripts,iphone): Epic2/2.7 iOS 测试基础设施（build.sh + MockBase + AsyncTestHelpers + sample template）`（或 `feat(iphone):` 若视之为工程能力补强）

### 常见陷阱

1. **`SampleViewModel` 不在 `#if DEBUG` 内 → Release build 包含 sample 类型**：检查 `SampleTypes.swift` 顶层 `#if DEBUG` 包裹整个 file body，不只是 class 体；`SampleViewModelTests.swift` 同样

2. **`MockBase` 标 `final` → 业务 mock 无法继承**：必须**不**标 final；用 `public class MockBase {}`

3. **`Helpers/` 目录被 PetApp target 误纳入**：检查 `iphone/project.yml` PetApp target 的 sources 是 `- PetApp`（即 `iphone/PetApp/` 子树），**不**会把 `iphone/PetAppTests/Helpers/` 错纳入；如果手贱改成 `- "**/*.swift"` 或类似 glob，sample 文件会被编译进主 App。**不**修改 project.yml

4. **`bash iphone/scripts/build.sh` 在 Linux / Windows 上跑**：xcodebuild 只在 macOS 上有；如未来 CI 上 Linux runner 误调，会报 `xcodebuild: command not found`。AC1 `require_tool xcodebuild` 的 fail-fast 会拦住 + 给提示。本 story 不预设跨 OS 兼容（macOS only）

5. **`xcrun simctl list devices iOS available` 输出格式跨 Xcode 版本变动**：本 story 用 `grep -Eo '\([0-9A-F-]{36}\)'` 提取 UUID；UUID 格式（8-4-4-4-12 hex）是 Apple ABI 稳定的，跨 Xcode 不变。如未来 simctl 输出大改，fallback 会失败 —— 但 primary / secondary 通常已经命中，第三段 fallback 极少真触发

6. **`xcodebuild -showdestinations` 在某些 Xcode 版本上需要先 build 一次才能列**：如果发生这个问题，destination resolution 会全部 fail-fall 到第三段。临时绕过：先手动跑 `xcodebuild build -project ... -scheme ...` 一次再跑 build.sh。本 story 不预设这个边界

7. **`-resultBundlePath` 路径指向已存在文件 → xcodebuild 会失败 / 警告**：每次 build.sh 跑都覆写到同一路径 `iphone/build/test-results.xcresult`；如果上次 .xcresult 已存在，xcodebuild 通常会自动覆盖。但**有些 Xcode 版本**会拒绝。AC1 实装在 `--clean` 路径里 `rm -rf "$TEST_RESULTS"` 显式清；非 clean 路径下不主动删 —— 让 xcodebuild 自己处理。如未来发现这是 flaky 源，加一行 `rm -rf "$TEST_RESULTS" 2>/dev/null || true` 在 test 阶段顶部

8. **`MockSampleUseCase` 命名不能和 `MockSampleService` / `MockSampleRepo` 冲突**：grep `iphone/` 下无 `MockSample*` 命名；server 端有 `MockSampleRepo` 但跨端不冲突（独立 module）

9. **`@MainActor` 标 SampleViewModel 但测试方法没标 `@MainActor` → 编译错**：测试 class 整体标 `@MainActor` 即可（class 级标注会传播到所有方法）；`SampleViewModelTests` AC5 模板已正确标注

10. **`awaitPublishedChange` 的 `count` 参数跨平台行为差异**：iOS 17 / 26 + Combine 在不同 Xcode 版本下 `objectWillChange` 触发次数可能微调。AC5 测试 case 3 用 `XCTAssertGreaterThanOrEqual(captured.count, 1)` 而非严格 `XCTAssertEqual(captured.count, 2)` —— 容忍 ±1 漂移

11. **`#if DEBUG` 包裹的代码在 SwiftPM `swift test` 里默认是 DEBUG**：但 `xcodebuild test` 默认也是 Debug 配置，所以 `SampleTypes` / `SampleViewModelTests` 都会编译进 test bundle。Release build 通过 `xcodebuild build -configuration Release` 触发，会 strip 掉 sample 代码

12. **`#filePath` 与 `#file` 的区别**：Swift 5.3+ `#filePath` 取完整路径（用于错误定位）；`#file` 取相对路径（用于 logging）。AsyncTestHelpers 的两个 helper 用 `#filePath` 让 XCTFail 显示完整路径

### Project Structure Notes

- **新增**：
  - `iphone/scripts/build.sh`（chmod +x；与 server `scripts/build.sh` 风格对齐）
  - `iphone/PetAppTests/Helpers/MockBase.swift`（新目录 `Helpers/`）
  - `iphone/PetAppTests/Helpers/AsyncTestHelpers.swift`
  - `iphone/PetAppTests/Helpers/SampleViewModelTests.swift`
  - `iphone/PetApp/Shared/Testing/SampleTypes.swift`（新子目录 `Shared/Testing/`，#if DEBUG）
  - `iphone/docs/CI.md`（新目录 `iphone/docs/`）

- **不新增 / 不修改**：
  - `iphone/project.yml`（glob 自动纳入新文件）
  - `iphone/scripts/install-hooks.sh` / `iphone/scripts/git-hooks/pre-commit`
  - `iphone/PetAppTests/Core/Networking/MockURLSession.swift` 等 Story 2.4 / 2.5 mock
  - 任何 Story 2.2 / 2.3 / 2.4 / 2.5 / 2.6 production code
  - `.gitignore`（`iphone/build/` 已在第 48 行）
  - `CLAUDE.md`

- **xcodegen auto-regen 副作用**：新增子目录 + .swift 文件后 build.sh 自动跑 `xcodegen generate`；预期 `iphone/PetApp.xcodeproj/project.pbxproj` 会有 diff（xcodegen 重排 references）—— 这是 `iphone/project.yml` 不变 + .pbxproj 由 yml 生成的标准模式

- **测试目录镜像约定**：`PetAppTests/Helpers/` 是新目录（与 `PetAppTests/{App,Core,Features,Shared}/` 平级）；与 `iphone/PetApp/{App,Core,Features,Resources,Shared}/` 不严格镜像 —— Helpers 是横切性测试基础设施而非业务测试，独立放 `Helpers/` 子目录与 server 端 `internal/pkg/testing/` 平级精神一致

### References

- [Source: \_bmad-output/planning-artifacts/epics.md#Epic 2 / Story 2.7] — 原始 AC 来源（行 796-810）
- [Source: \_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md] — **本 story 唯一权威 ADR**
  - §3.1 — XCTest only（手写 Mock）：本 story MockBase 直接对接
  - §3.2 — async/await 主流 + XCTestExpectation 兜底：本 story AsyncTestHelpers 兑现"已知坑第 3 条 helper 函数落地"
  - §3.3 — 方案 D：本 story 在 `iphone/` 下落地，零 `ios/` 改动
  - §3.4 — CI 跑法：本 story `iphone/scripts/build.sh` 全面落地（含 destination 三段 fallback、artifacts `iphone/build/`、`-enableCodeCoverage YES` 默认开等）
  - §4 — 版本锁定清单（destination_primary / destination_fallback / ci_command_entry 全部对齐）
  - §5.1 — "Story 2.7（iOS 测试基础设施搭建）：按 §3.4 落地 `iphone/scripts/build.sh`（新写）+ `iphone/PetAppTests/Helpers/MockBase.swift`；建立第一条业务相关 mock 单元测试"
  - §6 — Post-Decision TODO 第 3 条："Story 2.7：按 §3.4 新写 `iphone/scripts/build.sh`...；落地 `iphone/PetAppTests/Helpers/MockBase.swift` + 第一条业务相关 mock 单元测试"
- [Source: \_bmad-output/implementation-artifacts/decisions/0001-test-stack.md] — server 端 ADR；本 story §3.4 / §3.5 风格直接对齐
- [Source: \_bmad-output/implementation-artifacts/2-1-ios-mock-框架选型-ios-目录决策-spike.md] — Story 2.1（已 done）：iOS 工具栈 spike，输出 ADR-0002
- [Source: \_bmad-output/implementation-artifacts/2-2-swiftui-app-入口-主界面骨架-信息架构定稿.md] — Story 2.2（已 done）：iphone/ 工程骨架 + project.yml + scripts/install-hooks.sh + .gitignore iphone/build/
- [Source: \_bmad-output/implementation-artifacts/2-4-apiclient-封装.md] — Story 2.4（已 done）：MockURLSession + StubURLProtocol（NSLock + session-local-only）
- [Source: \_bmad-output/implementation-artifacts/2-5-ping-调用-主界面显示-server-version-信息.md] — Story 2.5（已 done）：MockAPIClient + PingStubURLProtocol；line 1344 明示 "iphone/scripts/build.sh 推迟到 Story 2.7"；line 1345 明示 "MockBase.swift 推迟到 Story 2.7"
- [Source: \_bmad-output/implementation-artifacts/2-6-基础错误-ui-框架.md] — Story 2.6（已 done）：ErrorPresenter / Toast / AlertOverlay / RetryView + 测试栈
- [Source: \_bmad-output/implementation-artifacts/1-5-测试基础设施搭建.md] — server 端类比 story（已 done）：testify + sqlmock + miniredis + slogtest + sample service template
- [Source: \_bmad-output/implementation-artifacts/1-7-重做-scripts-build-sh.md] — server 端 build.sh story（已 done）：脚本风格 / 五开关契约 / 手动验证 3 组合参考
- [Source: scripts/build.sh] — server 端 build.sh 实际实装（127 行）；本 story `iphone/scripts/build.sh` 风格对齐
- [Source: iphone/scripts/install-hooks.sh] — Story 2.2 既有 install-hooks 脚本；本 story 不动它，但 `require_tool` helper 风格直接复用
- [Source: iphone/scripts/git-hooks/pre-commit] — Story 2.2 既有占位 hook；本 story 不动
- [Source: iphone/PetAppTests/Core/Networking/MockURLSession.swift] — Story 2.4 既有手写 mock；本 story MockBase 文件头注释引用
- [Source: iphone/PetAppTests/Core/Networking/StubURLProtocol.swift] — Story 2.4 既有 fake server；本 story 不动
- [Source: iphone/PetAppTests/Features/Home/UseCases/MockAPIClient.swift] — Story 2.5 既有 mock；本 story 不动
- [Source: iphone/project.yml] — Story 2.2 既有工程定义；本 story 0 改动（glob 自动纳入新文件）
- [Source: .gitignore] — repo 根 gitignore；第 48 行 `iphone/build/` 已加（Story 2.2 同步），本 story 复用
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#17 测试建议] — 测试金字塔 / 单元 / 集成 / UI 三层
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#18.1 首选技术路线] — async/await 路线（本 story AsyncTestHelpers 直接对接）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#4 项目目录建议] — 目录结构（`PetApp/{App,Core,Shared,Features,Resources,Tests}/`）
- [Source: CLAUDE.md "Build & Test"] — server 端 `bash scripts/build.sh --test` 命令面（本 story `iphone/scripts/build.sh` 风格对齐）
- [Source: CLAUDE.md "Repo Separation（重启阶段过渡态）"] — 三目录约束（`server/` `iphone/` `ios/`）；本 story 严格守 `iphone/` only
- [Source: docs/lessons/2026-04-26-urlprotocol-stub-global-state.md] — **必读**：NSLock + snapshot helper + 文件头硬约定模式；本 story MockBase 直接继承
- [Source: docs/lessons/2026-04-26-urlprotocol-session-local-vs-global.md] — 间接：测试 mock 不要 process-global state
- [Source: docs/lessons/2026-04-25-swift-explicit-import-combine.md] — 必读：production / test 文件首次使用 Combine 必须显式 import；本 story `AsyncTestHelpers.swift` + `SampleTypes.swift` 都显式 `import Combine`
- [Source: docs/lessons/2026-04-26-jsondecoder-encoder-thread-safety.md] — 间接：本 story AsyncTestHelpers 内部 lock 使用模式（短临界区 + 不持锁调回调）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

实装过程遇到 2 个需修正的问题：

1. **`awaitPublishedChange` 内部 `Box` class 嵌套报错**（第一次 `--test` 跑挂）
   - 报错：`Type 'Box' cannot be nested in generic function 'awaitPublishedChange(on:keyPath:count:timeout:file:line:)'` + `'Box' cannot be constructed because it has no accessible initializers`
   - 根因：Swift 不允许在 generic function 体内定义 class（generic context 内嵌 class 没有可访问的初始化方法）
   - 修复：把 collector 类提到文件级 `_AsyncTestCollector<V>: @unchecked Sendable`，generic function 引用它即可

2. **`testStatusTransitionsCaptured` 第一次实装时间序错误**（第二次 `--test` 跑挂）
   - 报错：`awaitPublishedChange timed out after 1.0s; got 0/1 changes`
   - 根因：`async let trigger: Void = sut.load(...)` 与 `awaitPublishedChange` 并发启动时，`load` 内部的 `status = .loading` 可能在 sink 订阅之前就已发出，导致 `objectWillChange` 错过
   - 修复：用 `Task { await Task.sleep(...) ; await self.sut.load(...) }` 显式延迟 50ms 触发 load，让 `awaitPublishedChange` 内部 sink 先订阅成功；timeout 同步从 1.0s 提升到 2.0s 增加 CI 健壮性
   - 注：此修复并不削弱 helper 的可用性，反而是模板示范"先订阅再触发"的正确异步测试范式（实际业务测试中可以用更精细的同步原语，但作为模板这里用 sleep 已足够清晰）

### Completion Notes List

✅ **AC1 build.sh 完成**：6618 字节，chmod +x，destination 三段 fallback 实装，`set -euo pipefail` 严格模式，require_tool 复用 install-hooks.sh 风格。grep 验证：脚本内零 hardcoded `OS=26.4 / OS=17.0`，全部 artifacts 走 `iphone/build/`。

✅ **AC2 MockBase.swift 完成**：`public class MockBase`（不 final，可继承），NSLock 保护 `invocations / lastArguments / callCounts` 三字段；提供 `record / reset / wasCalled / callCount(of:) / invocationsSnapshot / callCountsSnapshot` 6 个 API；`MockError` enum 含 `.notStubbed / .unexpectedCall(String)`；文件头注释明示"现有 networking mock 不强制迁移"。

✅ **AC3 AsyncTestHelpers.swift 完成**：`assertThrowsAsyncError` 用 `@autoclosure` + 可选 matcher；`awaitPublishedChange` 用 `objectWillChange + DispatchQueue.main.async` 解决 SwiftUI 字段变更前触发的时序问题；内部 `_AsyncTestCollector<V>` 用 NSLock 保护短临界区。

✅ **AC4 SampleTypes.swift 完成**：整个文件 `#if DEBUG / #endif` 包裹（Release 自动 strip）；`SampleUseCase: Sendable`、`@MainActor SampleViewModel: ObservableObject`；显式 `import Combine`（lesson 2026-04-25-swift-explicit-import-combine.md）。

✅ **AC5 SampleViewModelTests.swift 完成**：5 个 case（happy + error + state-transition + helper-demo + reset 验证），全部演示 MockBase + AsyncTestHelpers 用法；class 包 `#if DEBUG`；`@MainActor` 标 class 让所有方法自动跑在 main actor。

✅ **AC6 iphone/docs/CI.md 完成**：5 章节（命令面 / fallback / GitHub Actions 占位 / 排错 / server 命令面对照）；**未**新建 `.github/workflows/*.yml`（守住红线）。

✅ **AC7 手动验证完成**：
- `bash iphone/scripts/build.sh` → BUILD SUCCESS（pure build only；尾行 `BUILD SUCCESS`）
- `bash iphone/scripts/build.sh --test` → BUILD SUCCESS + `Executed 90 tests, with 0 failures (0 unexpected) in 0.963 (1.004) seconds`
- `bash iphone/scripts/build.sh --clean` → BUILD SUCCESS（clean 后重建 DerivedData）
- xcresult 落 `iphone/build/test-results.xcresult`，与 server 端 `build/` 完全隔离

✅ **AC8 既有测试零回归**：`SheetTypeTests / AppCoordinatorTests / RootViewWireTests / AppContainerTests / HomeViewModelTests / HomeViewModelPingTests / HomeViewTests / APIClientTests / APIClientIntegrationTests / PingUseCaseTests / PingUseCaseIntegrationTests / AppErrorMapperTests / ErrorPresenterTests / ErrorComponentSnapshotTests` 全部绿；新增 `SampleViewModelTests` 5 个 case 全部绿。

✅ **AC9 git status 净化**：除新增 6 文件 + `.pbxproj` xcodegen regen 副作用 + `sprint-status.yaml` + 本 story 文件外，零其他 diff。`ios/ / server/ / iphone/PetApp/{App,Core,Features,Resources} / iphone/PetAppTests/{App,Core,Features,Shared} / iphone/PetAppUITests / iphone/project.yml / iphone/scripts/install-hooks.sh / iphone/scripts/git-hooks/pre-commit / .gitignore / CLAUDE.md / docs/` 全部零 diff。

### File List

**新增（6 个）**：

- `iphone/scripts/build.sh`（chmod +x；6618 字节；ADR-0002 §3.4 落地）
- `iphone/PetAppTests/Helpers/MockBase.swift`（含 MockBase class + MockError enum）
- `iphone/PetAppTests/Helpers/AsyncTestHelpers.swift`（含 assertThrowsAsyncError + awaitPublishedChange + 内部 _AsyncTestCollector）
- `iphone/PetAppTests/Helpers/SampleViewModelTests.swift`（5 个 test case + 内嵌 MockSampleUseCase）
- `iphone/PetApp/Shared/Testing/SampleTypes.swift`（#if DEBUG 包裹的 SampleUseCase + SampleViewModel）
- `iphone/docs/CI.md`（CI 跑法文档化）

**修改（2 个，预期副作用）**：

- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen regen 自动纳入新 .swift 文件）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（`2-7-ios-测试基础设施搭建` 状态从 ready-for-dev → in-progress → review）

**story 文件本身**：`_bmad-output/implementation-artifacts/2-7-ios-测试基础设施搭建.md`（Status / Tasks 勾选 / Dev Agent Record / Change Log）

## Change Log

| 日期 | 版本 | 描述 | 作者 |
|---|---|---|---|
| 2026-04-25 | 0.1 | 初稿（ready-for-dev）；Ultimate context engine analysis：MockBase + AsyncTestHelpers + iphone/scripts/build.sh + CI 文档 + sample template | SM |
| 2026-04-25 | 0.2 | dev-story 落地：6 个新文件全部实装；90 tests 全绿（含 5 个新 SampleViewModelTests）；BUILD SUCCESS 三组合验证通过；状态流转 ready-for-dev → in-progress → review | Dev |
