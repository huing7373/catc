---
title: spec 钦定 SF Symbol 字符串前必须物理验证可用性
date: 2026-04-30
severity: 1
category: spec-design, process
commit: <pending>
related_stories: [37-6-shared-primitives]
---

## 现象

Story 37.6（共享 primitives）dev-story 首轮 HALT：

- AC2 钦定 25 键 SF Symbol 映射，其中 `bowl → "bowl.fill"`
- AC9 case#3 (`testAllMappedSFSymbolsExistOnIOS17`) 要求全部 25 键映射的 SF Symbol 在 iOS 17+ 都能 `UIImage(systemName:)` 拿到非 nil
- AC10 要求 `bash iphone/scripts/build.sh --test` 256/256 全绿
- Story line 35 / Dev Notes line 33 红线 "**不接受** dev 自行替换映射"

dev 双路验证：

```bash
# 路 1：strings 扫 simruntime 内 SFSymbols 二进制
strings .../SFSymbols.framework/SFSymbols | grep -iE "bowl"
# 结果：仅 figure.bowling*（运动姿势），无 bowl / bowl.fill

# 路 2：UIImage(systemName: "bowl.fill") 在 iOS 26.4 simulator runtime 内返回 nil
```

→ `bowl.fill` 在 iOS 17+ Apple SF Symbols SDK 不存在，AC9 case#3 物理上不可能通过。

## 4 条约束死结（不可同时满足）

1. **mapping 必须是 X**（AC2 钦定 `bowl → "bowl.fill"`）
2. **dev 不许改 mapping**（Story line 35 / Dev Notes line 33 红线）
3. **test 必须验证 mapping 全部 SF Symbol 在 iOS 17+ 存在**（AC9 case#3）
4. **build 必须全绿才能合并**（AC10）

四角形成三角死锁：要满足 1 + 2 → 3 必 fail → 4 必 fail；要满足 3 + 4 → 必须改 1（违反 2）或删 3（违反 AC9）。

最后由 user 授权对 (2) 一次例外，替换 `bowl → "fork.knife"`（实存 + 与 ui_design FeedButton "喂食"语义最贴近）。

## 根因

SM 写 spec 阶段把 ui_design React 原型里的 Icons 对象 JS 字符串（`bowl`, `heart`, `paw`, ...）按"印象"映射到 SF Symbol（`bowl.fill`, `heart.fill`, `pawprint.fill`, ...），**没用 `UIImage(systemName:)` 探针实查**就把"未验证的 SF Symbol 字符串"当成 spec 钦定值写进 AC2 表 + 红线。

类似的"凭印象抄字符串"风险源还包括：

- API path（`/v1/foo`）—— spec 抄了文档里旧的 path 但 server 已改名
- framework 名（`@import Combine`）—— spec 抄了 sample 里的 import 但目标平台版本无该 framework
- asset name（`Image("avatar.placeholder")`）—— spec 抄了原型里的 asset 名但 xcassets 内未导入

任何"外部 SDK 钦定字符串"未在 spec 阶段物理验证存在性，**进入实装阶段后会被 dev 验证 test 命中** —— 此时若 spec 同时钦定红线"dev 不许改"，就形成死结。

## 教训

**spec 钦定外部 SDK 字符串前必须物理验证存在性**。不存在的字符串成为 spec 钦定值后会形成不可解死结：dev 改不了（违反红线），不改 build 不绿（违反 AC10），HALT 上交 SM/PM 决策 —— 浪费一轮 dev-story 时间。

## 预防规则（forward-actionable）

1. **SM 写 spec 涉及外部 SDK 字符串时（SF Symbol / API path / framework 名 / asset name 等），必须在 spec 创建时用 `UIImage(systemName:)` / 类似探针验证存在性**；validation evidence（探针命令 + 输出）写进 Dev Notes 或 AC 旁注。
   - SF Symbol: `xcrun --sdk iphonesimulator swift -e 'import UIKit; print(UIImage(systemName: "bowl.fill") != nil)'`，或在 SF Symbols.app 内搜
   - API path: 对照 `docs/宠物互动App_V1接口设计.md` 当前版本（不抄旧版本 / 不抄旧 commit）
   - framework: 对照目标平台 deployment target 的 framework availability
   - asset: 对照实际 xcassets / Resources 目录

2. **涉及"映射字符串数组/表"的 AC 应当配套写 escape hatch**：
   > "dev 在实装阶段如发现某条字符串在物理 SDK 不存在 + 与 spec 设计意图无矛盾，可在 dev_story HALT 前提下做 user-authorized 替换；不构成 dev 自行决策，由 user 拍板"
   
   ——给 dev 一个 escape hatch 而不是绝对红线，避免"red line + verification test + physical SDK"三角死结时强制 SM 重写 spec。

3. **反模式**：spec 红线 + 验证 test + 物理 SDK 矛盾三者同时锁死时，**dev 必须 HALT** 而非自己绕过 / 删 test / 假装通过。让 spec author 或 user 决策，HALT 报告必须含：
   - 物理验证证据（探针命令 + 输出）
   - 4 条约束死结的具体引用（AC 编号 + 行号）
   - 推荐替代值 + 替代值的物理验证证据
   - 不要在 dev sub-agent 层级"创造性绕过"

## 关联

- Story 37.6 (`_bmad-output/implementation-artifacts/37-6-shared-primitives.md`) AC2 inline 注解 + Dev Agent Record
- iPhone 端 `iphone/PetApp/Core/DesignSystem/Primitives/Icons.swift` mapping 内 user-authorized substitution 注释（bowl → fork.knife）
