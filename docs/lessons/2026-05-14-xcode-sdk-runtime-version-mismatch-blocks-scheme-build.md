---
date: 2026-05-14
source_review: epic-18 retrospective action item A1（非 fix-review；环境 quirk 修复）
story: epic-18-action-item-a1
commit: 3cd2ef4
lesson_count: 1
---

# Lesson — 2026-05-14 — Xcode SDK 与 simulator runtime 版本错位会让所有 scheme-based xcodebuild 失败，必须装匹配 runtime（非 sub-agent 绕过）

## 背景

Epic 18 全程触发：本机 Xcode 26.5（自带 iOS 26.5 SDK，且为**唯一**装好的 iOS SDK）+ 仅装 iOS 26.4 / 26.4.1 simulator runtime（无 iOS 26.5 runtime）。整 epic 18 + retrospective A1 期间 5+ 次反复出现，sub-agent 每次都要重新摸索绕过路径。最终 retrospective 提的修脚本 / 写 lesson 两个方案实测都不是根因解（直接用 booted simulator UDID 也仍然挂），唯一的根因解是**装匹配 runtime**。

## 现象

```
$ bash iphone/scripts/build.sh
=== xcodegen generate ===
OK: PetApp.xcodeproj generated
=== resolved destination: platform=iOS Simulator,id=<某 booted UDID> ===
=== xcodebuild build ===
xcodebuild: error: Unable to find a destination matching the provided destination specifier:
		{ platform:iOS Simulator, id:<某 booted UDID> }
	Ineligible destinations for the "PetApp" scheme:
		{ platform:iOS, id:dvtdevice-DVTiPhonePlaceholder-iphoneos:placeholder, name:Any iOS Device, error:iOS 26.5 is not installed. Please download and install the platform from Xcode > Settings > Components. }
FAIL: xcodebuild build
```

同步症状（关键诊断信号）：
- `xcodebuild -showdestinations` 在 Available 段**没有**任何 concrete simulator entry（只有 `name:Any iOS Simulator Device` placeholder）
- xcodebuild 抛 `[MT] IDERunDestination: Supported platforms for the buildables in the current scheme is empty.` 警告
- 即使指定具体 booted simulator UDID 也被判 Ineligible
- `xcrun simctl list runtimes` 只显示 iOS 26.4 系列；`xcodebuild -showsdks | grep iOS` 只显示 iOS 26.5 SDK

## 根因

xcodebuild 跑 scheme 时按 `SDKROOT` 决定需要哪个 platform runtime。本机情况下 `SDKROOT = iphonesimulator26.5.sdk`（唯一装好的 iOS Simulator SDK），scheme 因此要求 **iOS 26.5** runtime；但本机只装了 iOS 26.4 runtime → 所有 simulator destination 被判 Ineligible → 整套 scheme-based 命令链全断（build / test / archive / showdestinations 全部）。

这是 SDK/runtime 版本必须严格匹配的硬约束，**不是** build.sh destination 解析逻辑的 bug。retrospective A1 初版以为可以通过"优先 booted UDID"绕过 → 实测错。即使用 booted UDID + UDID 直传，scheme 还是会因 SDK/runtime mismatch 判 Ineligible。

## 反例（之前以为可行的绕过路径，实际都有副作用，不是根因解）

1. **`-target` 替代 `-scheme`** —— 可以 build 但无法 `xcodebuild test`（test 命令要求 scheme / xctestrun / testProductsPath，三者都需要 scheme 至少跑通一次），`build.sh --test` / `--uitest` / `--coverage-export` flag 全失效。**仅适合**单纯编译验证场景，不适合 CI 测试链
2. **`-derivedDataPath` 与 `-target` 互斥**：`error: The flag -scheme, -testProductsPath, or -xctestrun is required when specifying -derivedDataPath` — 走 `-target` 路径还必须放弃 `-derivedDataPath`，artifacts 会落到 `iphone/build/Debug-iphonesimulator/`（Xcode 默认）而非 `iphone/build/DerivedData/`，与既有 build.sh 契约不一致
3. **`-skipPackagePluginValidation` / `-skipMacroValidation` 等 skip 标志** —— 不影响 destination 解析，仍挂
4. **修改脚本 destination 解析逻辑** —— 不论选 PRIMARY (`name=iPhone 17,OS=latest`) 还是 SECONDARY (`OS=latest`) 还是 UUID fallback，scheme 都拒，因为 mismatch 在 SDK level 不在 destination level

## 正确修复

安装匹配 Xcode SDK 版本的 simulator runtime：

```bash
xcodebuild -downloadPlatform iOS
# 跟随 Xcode 当前 SDK 版本自动下载对应 simulator runtime
# 本次实测: iOS 26.5 (23F77), arm64 variant, 约 8.5 GB
# 下载时间 5-15 分钟（取决于带宽）
# 默认不需要 admin 凭据（如果 Xcode CLI tools 装好）
```

下载完后 `xcrun simctl runtime list` 应能看到新 runtime `(Ready)` 状态。重跑 `bash iphone/scripts/build.sh` 应直接 `BUILD SUCCESS`。

## 触发条件 / 何时该 check 这条 lesson

任何**新机器 fresh clone** 或 **Xcode 大版本升级后**首跑 `bash iphone/scripts/build.sh` 看到上述错误信号（"Supported platforms ... is empty" + "iOS X.Y is not installed" 在 Ineligible 段）。Quick check 序列：

```bash
# 1. SDK 版本
xcodebuild -showsdks | grep -E "iphonesimulator"  # 看到的版本 X.Y
# 2. runtime 版本
xcrun simctl runtime list | grep -E "^iOS"        # 必须含 X.Y（精确匹配前两位即可）
# 3. 不匹配就装
xcodebuild -downloadPlatform iOS
```

## 给未来 Claude 的预防规则

1. **不要**第一次见到 "Supported platforms ... is empty" 就去改 `iphone/scripts/build.sh` —— 大概率是 SDK/runtime mismatch，先跑上面 quick check 序列
2. **不要**用 `-target` workaround 作为"长期解" —— 它把 `--test` 链路彻底断掉，副作用比省下来的 8.5 GB 下载严重得多
3. **fresh-clone setup 文档** 应明示这条约束：iphone/README.md 的"前置环境"段建议加一句"如果 Xcode 升级到 X.Y SDK，必须同步 `xcodebuild -downloadPlatform iOS` 装匹配 simulator runtime"
4. **sub-agent prompt 模板** 若曾 cite "环境陷阱：build.sh 因 xcodegen regen 失败" 的绕过路径段（Epic 18 期间普遍），现在可以全部移除，因为 build.sh 本身已能跑

## 相关 lesson

- `2026-04-26-xcodebuild-showdestinations-section-aware.md`（按 Available 段过滤）—— 不解决本 lesson 问题，只解决"`-showdestinations` 输出含 Ineligible 段干扰"的次级问题；两条互补
- `2026-04-26-simulator-placeholder-vs-concrete.md`（排除 `Any X Device` placeholder）—— 同样不解决本 lesson；它解决"没有 concrete simulator 时不该选 placeholder"
- 本 lesson 是上面两条之上的"如果 quick check 都过了仍挂"的最终根因诊断
