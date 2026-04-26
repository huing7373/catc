---
date: 2026-04-26
source_review: file:/tmp/epic-loop-review-2-7-r5.md (codex P1 + P3)
story: 2-7-ios-测试基础设施搭建
commit: e328838
lesson_count: 1
---

# Review Lessons — 2026-04-26 — Shell 判 simulator 可用性必须排除 `Any iOS Simulator Device` placeholder，concrete entry 才算真有 runtime

## 背景

Story 2.7 round 5 review。`iphone/scripts/build.sh` round 4 修了"按 Available 段过滤 `xcodebuild -showdestinations` 输出"（lesson `2026-04-26-xcodebuild-showdestinations-section-aware.md`），让 fallback 链不再被 Ineligible 段干扰。round 5 codex 抓到这个 hardening 不彻底：Available 段总会包含一条 `name:Any iOS Simulator Device` placeholder entry，**即使机器上没装任何具体 iOS Simulator runtime / CoreSimulator 不可用**这条仍在。`grep -q "iOS Simulator"` 命中 placeholder 后脚本会落到 `platform=iOS Simulator,OS=latest` 这条不可运行的 destination 上，build 阶段才挂 — 跳过了设计好的"`xcrun simctl` UUID fallback"分支。

本 lesson 与 round 4 lesson 是**同一主题的连续 hardening**：从"按段过滤"进一步到"段内仍要排除 placeholder entry"。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `xcodebuild -showdestinations` Available 段含 `Any iOS Simulator Device` placeholder，grep 不排除会选到不可运行 destination | P1 (high) | testing/config | fix | `iphone/scripts/build.sh:127-130` |
| 2 | `awaitPublishedChange` 未拒绝 `count == 0`，XCTestExpectation 不支持 0 → API violation | P3 (low) | testing | fix | `iphone/PetAppTests/Helpers/AsyncTestHelpers.swift:111` |

## Lesson 1: shell 判 simulator 可用性必须排除 `Any X Device` placeholder entry

- **Severity**: P1 (high)
- **Category**: testing/config（构建脚本环境兼容性）
- **分诊**: fix
- **位置**: `iphone/scripts/build.sh:127-130`

### 症状（Symptom）

`xcodebuild -showdestinations -scheme PetApp` 在没装任何 iOS Simulator runtime 的机器上输出（典型形态）：

```
Available destinations for the "PetApp" scheme:
  { platform:iOS Simulator, id:dvtdevice-DVTiOSDeviceSimulatorPlaceholder-iphonesimulator:placeholder, name:Any iOS Simulator Device }

Ineligible destinations for the "PetApp" scheme:
  { platform:iOS, ... }
```

round 4 修了"只看 Available 段"，但 Available 段里那条 `name:Any iOS Simulator Device` 是 Xcode 永远会塞进去的 generic placeholder，并不代表系统真有 simulator runtime。`grep -q "iOS Simulator"` 命中它后 RESOLVED_DESTINATION 落到 SECONDARY (`platform=iOS Simulator,OS=latest`)，xcodebuild 实际跑时报 "no usable simulator" 或类似错误，且**不会**进入第三段 fallback (`xcrun simctl` UUID 查找)。

### 根因（Root cause）

把"`xcodebuild -showdestinations` 列出 iOS Simulator destination" 误等同于 "系统有可运行的 iOS Simulator"。实际上 `Any X Device` 形态的 placeholder 是 Xcode 在所有平台 (`Any iOS Device` / `Any iOS Simulator Device` / `Any Mac Catalyst Device` / `Any tvOS Simulator Device` 等) 都会无条件塞的 generic destination，作用是让用户在 IDE 里选"我也不知道用哪个具体设备，xcodebuild 你看着办"。但 xcodebuild 命令行**不能**真用这条 destination 跑 — 必须有具体 device/runtime entry。

shell 判定可用性时必须区分：
- **placeholder entry** (`name:Any X Device`)：永远存在，不证明任何东西
- **concrete entry** (`name:iPhone 17, OS:18.0` 之类带 OS 字段)：真正可用的具体设备

### 修复（Fix）

抽取 `CONCRETE_SIMULATORS` 中间变量，从 Available 段进一步过滤掉 placeholder：

```bash
# before（round 4 修完的状态，含本次 round 5 抓到的漏洞）
if echo "$AVAILABLE_DESTINATIONS" | grep -q "iPhone 17"; then
  RESOLVED_DESTINATION="$DESTINATION_PRIMARY"
elif echo "$AVAILABLE_DESTINATIONS" | grep -q "iOS Simulator"; then
  RESOLVED_DESTINATION="$DESTINATION_SECONDARY"
else
  # UUID fallback ...

# after（round 5 修复）
CONCRETE_SIMULATORS="$(echo "$AVAILABLE_DESTINATIONS" | grep 'iOS Simulator' | grep -v 'Any iOS Simulator Device' || true)"

if echo "$CONCRETE_SIMULATORS" | grep -q "iPhone 17"; then
  RESOLVED_DESTINATION="$DESTINATION_PRIMARY"
elif [ -n "$CONCRETE_SIMULATORS" ]; then
  RESOLVED_DESTINATION="$DESTINATION_SECONDARY"
else
  # UUID fallback ...
```

测试：dogfood `bash iphone/scripts/build.sh --test` 在装有 simulator 的开发机上仍通过（93 tests, 0 fail）。仅 placeholder 环境的回归较难做严格自动化（需 Xcode 卸载所有 simulator runtime），通过代码注释 + lesson 文档化场景。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **shell 脚本里用 `xcodebuild -showdestinations` 输出判断平台可用性**时，**必须**排除 `Any X Device` 形式的 placeholder entry（grep `-v 'Any X Device'`），**只信带具体 device name 或 OS 字段的 concrete entry**。
>
> **展开**：
> - Xcode `xcodebuild -showdestinations` 在所有平台都会无条件塞 `Any <Platform> Device` placeholder，跨 iOS / iOS Simulator / Mac Catalyst / tvOS / tvOS Simulator / watchOS / watchOS Simulator / xrOS 都有。判可用性必须先去除这一条。
> - 段过滤（Available vs Ineligible）和 placeholder 过滤是**两层独立 hardening**：段过滤防 Ineligible 误命中，placeholder 过滤防 Available 段里 generic entry 误命中。两层都要做，缺一不可（参考 round 4 lesson `2026-04-26-xcodebuild-showdestinations-section-aware.md`）。
> - 用中间变量 `CONCRETE_SIMULATORS=$(... | grep -v 'Any X Device')` 记录"真有的具体设备列表"，后续 `grep <specific-name>` 和 `[ -n "$..." ]` 的 fallback 链都基于它，避免每个分支独立 grep 时漏过滤。
> - **反例 1**（round 4 修完的状态）：`grep -q "iOS Simulator"` 整段匹配，placeholder entry 命中即认为 simulator 可用，跳过 UUID fallback。
> - **反例 2**：用 `xcrun simctl list devices` 但不加 `available` 过滤，把 unavailable 的 device 也算进可用列表（这是另一个常见坑，但本次未中招）。
> - **正例**（本次修复）：先 `awk` 段过滤拿 Available 段，再 `grep -v 'Any iOS Simulator Device'` 过滤 placeholder，最后基于 `CONCRETE_SIMULATORS` 是否非空决定走 PRIMARY / SECONDARY / UUID fallback 哪一档。
> - **判定准则**：写 build 脚本判 device/runtime 可用性时，问自己 "这个判断在**完全没装该 runtime** 的机器上会得到什么结果？"如果答案是"仍然命中 placeholder"，说明判断条件需要 hardening。

## Lesson 2 (TODO 段补遗): `awaitPublishedChange` 显式拒绝 `count == 0`

`awaitPublishedChange(on:publisher:count:)` 接受任意 `Int` 作为 count，但 `XCTestExpectation.expectedFulfillmentCount` 不支持 0。调用方若错把 `count: 0` 当成"断言无变化"用，会从 XCTest 内部抛出 API violation，错误位置不在调用栈顶，难定位。

**修复**：函数顶部加 `precondition(count > 0, ...)`，给出明确报错。

```swift
precondition(count > 0, "awaitPublishedChange requires count > 0; to assert no changes, sample @Published value directly after a settled delay")
```

**未补 hard test case**：precondition 触发是 trap 而非 throw，`XCTAssertThrowsError` 抓不到。简单做法是用注释 + lesson 文档化"调用方需自检"。如果未来要严格自动化测试 precondition，需要 fork 子进程跑（XCTest 不原生支持 trap 测试）。

**预防规则补遗**（merge 进既有 helper lesson `2026-04-26-combine-prefix-vs-manual-fulfill.md` 的设计准则）：
- 写 XCTestExpectation 包装 helper 时，**`expectedFulfillmentCount` 字段不接受 0 或负数**；helper 入口必须 `precondition(count > 0)` 防御。"断言无变化" 的语义不能用 `count == 0` 表达，应该用别的手段（如 `wait(for: [unfulfilled], timeout: short)` + `XCTAssertEqual(expectation.fulfillCount, 0)`，或直接 sample 状态值）。

## Meta: 本次 review 的宏观教训

round 4 / round 5 的两轮发现都集中在同一个文件 (`iphone/scripts/build.sh`) 的同一段 destination 解析逻辑，每轮都修了一层但没修透。教训：**写 shell 脚本里的 "环境探测 + fallback 链" 时，要对每个判定条件做"反向问"**：
- 这条判定在 **runtime 完全缺失** 的机器上会怎样？
- 这条判定在 **被 Xcode 塞了 generic placeholder** 的输出上会怎样？
- 这条判定在 **scheme 本身有问题** 导致 `-showdestinations` 输出空的情况下会怎样？

每个反向问对应一个 hardening 维度，初版脚本通常只通过其中一两个；后续 review 会逐个揪出剩下的。如果一开始就把这些反向问列成清单，可以一次到位避免连续多轮。

## 未完成事项 / 后续 TODO

### 2026-04-26 round 6 — 接受为 hardening tech-debt（用户决策"接受"）

epic-loop 跑到 review_round 6（5 轮上限触顶后再跑了一次诊断 review），codex 又给了 **1 个新 [P1] finding**，登记如下，**本 story 不修，作为 hardening tech-debt**：

**[P1] `set -euo pipefail` + `xcrun simctl | grep | head | tr` pipeline 提前退出** — `iphone/scripts/build.sh:141`
- **症状**：当机器没装 iOS Simulator runtime（CI/fresh macOS），`-showdestinations` 只输出 `Any iOS Simulator Device` placeholder（已在 round 5 处理）→ 落到 UUID fallback 分支 → `xcrun simctl list devices iOS available 2>/dev/null | grep -Eo '\([0-9A-F-]{36}\)' | head -1 | tr -d '()'` 这条 pipeline 中 `grep` 没匹配返 1，触发 `set -e`，脚本在赋值时直接退出，永远到不了下面写好的 `if [ -z "$FALLBACK_UUID" ]` 错误路径，最终 fail 时也没有 actionable 错误信息。
- **承认是真问题**：`set -euo pipefail` 与 pipeline 中 grep 可能空命中天然冲突，是经典 shell 坑。修法极简：行末加 `|| true`，让赋值即使 grep 没匹配也成功 → 进入 `[ -z "$FALLBACK_UUID" ]` 分支输出明确错误。
- **defer 理由**：本 story 范围红线"测试基础设施 + 本机开发"已覆盖；CI / 无 simulator runtime 环境本 story 没承诺支持（Epic 3 demo 验收 / 后续 CI 接入 story 才会触发）；epic-loop 5 轮 review budget 已用尽（本 story 已修 7 个 [P1] / 3 个 [P2] / 1 [P3]，质量已超 MVP 所需）。
- **触发回看时机**：Story 2-7 范围内的 CI 文档化任务（`iphone/docs/CI.md`）+ 后续真正接入 GitHub Actions / 任何 fresh CI runner 时；那时一并处理 `set -e` + pipeline 兼容性。
- **简易修法预览**（留给将来）：
  ```bash
  FALLBACK_UUID="$(xcrun simctl list devices iOS available 2>/dev/null | grep -Eo '\([0-9A-F-]{36}\)' | head -1 | tr -d '()' || true)"
  ```

### Round 6 finding 的 commit 安排

不再单独开 fix(review) commit；登记在本 lesson 的 TODO 段，由 Story 2-7 的 chore(story-2-7) 收官 commit 一起带走。后续 CI 接入 story 触发回看时，按本段取出处理。

---

## Meta: round 4 / 5 / 6 三轮 build.sh hardening 的连续盲区

`iphone/scripts/build.sh` destination 解析与 fallback 链跨 3 轮被 codex 揪出三种独立 bug：
- **round 4**：`xcodebuild -showdestinations` 输出按段切分，Ineligible 段不能命中
- **round 5**：Available 段里 `Any X Device` placeholder 不算"真可用"
- **round 6**：`set -euo pipefail` 下 `xcrun simctl | grep` pipeline grep 空命中会让赋值失败 → 整脚本提前退出

三轮 finding 都是同一个文件同一段逻辑的不同盲区，本质上都是 "环境探测 + fallback" 这种 shell 模式的天然脆弱性。如果一开始按 Meta 段列出的"反向问"清单 + 把 `set -e` 与 pipeline 可空命中行的兼容性也纳入清单（"这条赋值在 grep 没匹配时会发生什么？"），可以一次到位避免连续多轮。

epic-loop 在 round 5 后已用尽 5 轮 review budget；继续修下去 codex 仍可能挖更多边界。本次决策：**接受 round 6 finding 为 hardening tech-debt**，让 story 走 done。未来 build.sh 改动应当先看本 lesson 的"反向问 + set -e 清单" + 在 PR 描述里贴清单（让 reviewer 一眼能验证全维度被考虑过）。
