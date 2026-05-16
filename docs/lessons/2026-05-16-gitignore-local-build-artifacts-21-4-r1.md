---
date: 2026-05-16
source_review: codex review file /tmp/epic-loop-review-21-4-r1.md (epic-loop r1)
story: 21-4-奖励弹窗-popup
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-16 — 根级本地 build 产物未 gitignore，dev-story change set 携带 61MB 机器特定缓存（21-4 r1）

## 背景

Story 21.4 落地 SwiftUI 奖励弹窗（`RewardPopupView` + `RewardRarityTagMapper` + `HomeView` 接线 + AccessibilityID + 单测/UI 测试）。dev-story 阶段在 iOS 模拟器验证 UI 时，xcodebuild / 验证流程在项目根目录产出了 `.build.log`（144K Xcode 构建日志）和 `.derivedData/`（61MB DerivedData 缓存树）。这两个是未追踪（untracked）的本地构建产物，但 `.gitignore` 没有覆盖根级路径，导致它们出现在 `git status` 未追踪列表里——若后续用 `git add -A` / `git add .` 会被一并扫进 commit，污染仓库（机器特定的 Xcode 日志 + 巨型缓存树，churn + bloat）。codex r1 把这条标为 [P2] repo hygiene。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `.build.log` + `.derivedData/`（61MB）为根级未追踪本地 build 产物，未被 gitignore 覆盖 | P2 | hygiene | fix | `.gitignore` |

## Lesson 1: 根级本地 build 产物必须显式 gitignore，不能依赖"手动别 add 它"

- **Severity**: P2 (medium)
- **Category**: hygiene
- **分诊**: fix
- **位置**: `.gitignore`（根级，原文件无 `.build.log` / `.derivedData/` 规则）

### 症状（Symptom）

iOS UI 验证（CLAUDE.md 要求的 `ios-simulator` MCP 实跑流程 / xcodebuild）在 **项目根目录** 而非 `iphone/build/` 下产出 `.build.log`（114KB Xcode 日志）和 `.derivedData/`（61MB DerivedData 树）。既有 `.gitignore` 只有 `iphone/build/`、`.build/`（注意带斜杠是 SwiftPM 目录，**不**匹配 `.build.log` 文件）、`DerivedData/`（匹配名为 `DerivedData` 的目录，**不**匹配 `.derivedData`，大小写 + 前导点都不同）等规则，没有任何一条命中这两个根级路径。结果 `git status --short` 把它们列为 `??` 未追踪——一旦 dev-story / fix-review 用 `git add -A` 或 `git add .` 收尾，61MB 机器特定缓存就进 commit。

### 根因（Root cause）

- **新验证路径产生新产物位置**：CLAUDE.md "iOS UI 验证（必跑）" 章节要求用 ios-simulator MCP 实跑，该流程的 xcodebuild 输出落点是 **repo root**（`.build.log` / `.derivedData/`），而历史 gitignore 规则（Story 2.2 加的 `iphone/build/`）只覆盖了 `iphone/` 子目录下的产物，没预见根级落点。
- **相似规则的误覆盖错觉**：`.gitignore` 里已有 `.build/`（SwiftPM）和 `DerivedData/`（Xcode 默认）会让人误以为"build 产物已经 ignore 了"。但 glob 语义上 `.build/` 末尾斜杠只匹配目录、不匹配 `.build.log` 文件；`DerivedData/` 大小写敏感且无前导点，不匹配 `.derivedData`。**相似名 ≠ 实际匹配**。
- **"我不会手动 add 它" 不是防线**：untracked 产物不进 commit 依赖的是"每次都精确 `git add <file>`"。但 fix-review / story-done 流程在文件数 >10 时允许 `git add -A`，且人/agent 容易顺手 `git add .`——没有 gitignore 兜底时，巨型缓存进 commit 只是时间问题。

### 修复（Fix）

在 `.gitignore` 既有 "iPhone App build artifacts" 段之后，按相同的"注释标注来源 story + 规则"风格追加一段根级规则：

```diff
 # iPhone App build artifacts (Story 2.2 / ADR-0002 §3.4 round-3 P2 fix)
 iphone/build/
+
+# Root-level local Xcode build artifacts (Story 21.4 r1 P2 fix — codex review)
+.build.log
+.derivedData/
```

- 验证：`git check-ignore .build.log .derivedData/` 两者均命中；`git status --short` 不再列出这两个路径。
- **不** `git rm`：这两个文件/目录从未被 track（纯 untracked），加 gitignore 即足够，不需要从索引移除，也不删除磁盘上的文件本身（它们是有效的本地缓存，删了下次还得重建）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **commit 前 `git status --short` 看到未追踪的 build 日志/缓存（`.build.log`、`*DerivedData*`、`.derivedData/` 等）** 时，**必须先把对应路径加进 `.gitignore`**（而不是寄望于"我手动只 add 业务文件"），再继续 commit 流程。
>
> **展开**：
> - gitignore 规则要**精确到实际产物路径**：`.build/`（带斜杠）只匹配目录，**不**匹配 `.build.log` 文件；`DerivedData/` 大小写敏感，**不**匹配 `.derivedData`。看到相似的既有规则**不要**假设已覆盖——用 `git check-ignore <path>` 实测。
> - 加规则时**沿用既有 `.gitignore` 的注释风格**：本 repo 习惯 `# <用途> (Story X / ADR 来源)` 标注规则来源，新增段保持一致便于后续审计。
> - build 产物的位置取决于**构建命令的工作目录**：`iphone/scripts/build.sh` 产物落 `iphone/build/`（已有规则），但 ios-simulator MCP / 根目录 xcodebuild 可能落 **repo root**。新增验证路径时同步检查产物落点是否已被 ignore。
> - untracked 产物 **不需要 `git rm`**——它从来没进过索引；只加 gitignore。只有产物**已被 track**（历史误 commit）时才需要 `git rm --cached`。本例是纯 untracked，gitignore 一条解决。
> - **反例**：commit 前看到 `?? .derivedData/`（61MB）却想"我 commit 时只 `git add` 业务文件就行，不用动 gitignore"——这把仓库卫生押在每次手动精确 add 上；一旦某轮文件多用了 `git add -A`，机器特定的 61MB 缓存就永久进历史，事后 `git filter-branch` 清理代价极高。正确做法是**当场加 gitignore 兜底**，让仓库结构本身防住这类误 add。

---

## Meta: 本次 review 的宏观教训

本轮 review 的唯一 finding 与 21.4 的 Swift 业务代码无关——是**流程产物溢出到仓库**的卫生问题。教训：CLAUDE.md 引入"iOS UI 验证必跑"这类**新验证流程**时，要同步评估该流程的副产物（日志、缓存、临时文件）落点是否已被 `.gitignore` 覆盖；新流程的产物位置往往和旧脚本（`iphone/scripts/build.sh` → `iphone/build/`）不同，旧 gitignore 规则不会自动覆盖。验证流程与 gitignore 规则要**配套演进**。
