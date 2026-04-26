---
date: 2026-04-26
source_review: file:/tmp/epic-loop-review-2-7-r4.md (codex P1)
story: 2-7-ios-测试基础设施搭建
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — `xcodebuild -showdestinations` 必须按段过滤，grep 全文会选中 Ineligible 段

## 背景

Story 2.7 round 4 review。`iphone/scripts/build.sh` 用 `xcodebuild -showdestinations | grep -q "iPhone 17"` 判定 destination 是否可用，但 `xcodebuild -showdestinations` 输出实际有两段：`Available destinations:` 与 `Ineligible destinations:`。原实现 grep 整段，导致在 iPhone 17 仅出现在 Ineligible 段（runtime 缺失 / scheme 不兼容等）的环境下脚本会错选不可用 destination，后续 build/test 失败而**不**触发设计好的 fallback。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `xcodebuild -showdestinations` 全文 grep 不区分 Available / Ineligible 段 | high | config | fix | `iphone/scripts/build.sh:116-118` |

## Lesson 1: `xcodebuild -showdestinations` 必须按段过滤

- **Severity**: high
- **Category**: config（构建脚本环境兼容性）
- **分诊**: fix
- **位置**: `iphone/scripts/build.sh:116-118`

### 症状（Symptom）

`xcodebuild -showdestinations` 输出包含两段：

```
	Available destinations for the "PetApp" scheme:
		{ platform:iOS Simulator, ..., name:iPhone 17 }
		{ platform:iOS Simulator, ..., name:iPhone 16 }

	Ineligible destinations for the "PetApp" scheme:
		{ platform:iOS Simulator, ..., name:iPhone 17, OS:13.0 }
```

原 fallback 代码：

```bash
if xcodebuild ... -showdestinations | grep -q "iPhone 17"; then
  RESOLVED_DESTINATION="$DESTINATION_PRIMARY"
elif xcodebuild ... -showdestinations | grep -q "iOS Simulator"; then
  ...
fi
```

`grep` 不分段，只要全文出现 `iPhone 17` 就匹配，包括 Ineligible 段的条目。Ineligible 条目意味着该 destination 在当前环境**不可用**（runtime 缺失、与 scheme 平台不兼容等）；脚本误选后下一步 `xcodebuild build/test -destination "name=iPhone 17,OS=latest"` 必失败，但失败时已经错过了 fallback 链（后续 `elif` 不会再跑），整个三段 fallback 设计被绕过。

### 根因（Root cause）

shell 的 `grep` 是行匹配，不感知文档结构。`xcodebuild -showdestinations` 的输出虽然人眼可读地分了两段（Available + Ineligible），但都是用同样的缩进和 `{ ... }` 格式输出 destination 条目，区别只在前面有一行 `Available destinations for ...:` / `Ineligible destinations for ...:` 标题。脚本作者读输出时直觉是"我想找 iPhone 17"，没意识到 Ineligible 段的存在 / 没意识到段意味着可用性差异。

类似坑：任何 CLI 工具的输出有"可用 / 不可用"分段都吃这一套（`brew list --installed` vs `outdated`、`docker ps` vs `ps -a`、`gh pr list` 默认 vs `--state=all` 等）—— 但这些工具一般通过 flag 控制输出范围；`xcodebuild -showdestinations` 不提供 `--available-only` 这样的 flag，**只能在客户端按段切**。

### 修复（Fix）

用 awk 范围过滤抽取 Available 段：

```bash
SHOWDEST_OUTPUT="$(xcodebuild -project "$PROJECT_PATH" -scheme "$SCHEME" -showdestinations 2>/dev/null || true)"
AVAILABLE_DESTINATIONS="$(echo "$SHOWDEST_OUTPUT" | awk '/Available destinations/{flag=1; next} /Ineligible destinations/{flag=0} flag')"

if echo "$AVAILABLE_DESTINATIONS" | grep -q "iPhone 17"; then
  RESOLVED_DESTINATION="$DESTINATION_PRIMARY"
elif echo "$AVAILABLE_DESTINATIONS" | grep -q "iOS Simulator"; then
  RESOLVED_DESTINATION="$DESTINATION_SECONDARY"
else
  ...
fi
```

awk 程序逻辑：
- 遇到 `Available destinations` 行 → `flag=1` 进入打印模式（`next` 跳过本行不打印标题）
- 遇到 `Ineligible destinations` 行 → `flag=0` 关闭打印模式
- 其他行：`flag` 为真则打印

边界情况验证（已 dogfood）：
1. 只有 Available 段（本机当前情况）→ awk 输出整段 ✓
2. 两段都有，iPhone 17 仅在 Ineligible → awk 输出 Available 段，grep 不匹配 → 走 secondary fallback ✓
3. 两段都有，iPhone 17 在 Available → awk 输出 Available 段，grep 匹配 ✓
4. Available 段为空 → awk 无输出，grep 全部不匹配 → 走第三段 `xcrun simctl` UUID fallback ✓

dogfood 验证：`bash iphone/scripts/build.sh --test` 通过，93 unit tests 全 pass，resolved destination 正确选中 `name=iPhone 17,OS=latest`。

shell 脚本本身难单元测试，但 awk 表达式可以独立用 heredoc 验证：

```bash
cat <<'EOF' | awk '/Available destinations/{flag=1; next} /Ineligible destinations/{flag=0} flag' | grep -q "iPhone 17" && echo MATCHED || echo NOT_MATCHED
	Available destinations:
		{ name:iPhone 16 }
	Ineligible destinations:
		{ name:iPhone 17, OS:13.0 }
EOF
# 期望：NOT_MATCHED
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **shell 脚本里 grep 一个 CLI 工具的多段输出（如 `xcodebuild -showdestinations`、`brew list`、`launchctl list` 等）** 时，**必须先按 section 标题分段切**，再在目标段内 grep；**禁止**对整段输出直接 grep 业务关键词。
>
> **展开**：
> - **触发条件识别**：如果 CLI 工具的输出有"Available / Ineligible"、"Installed / Available updates"、"Running / Stopped"等明显分段，全文 grep 就会跨段命中错误条目。`xcodebuild -showdestinations` 是典型例子。
> - **首选切段方式**：awk 范围 `/start/,/end/` 或状态机模式 `/start/{flag=1; next} /end/{flag=0} flag`。后者更灵活（可去掉边界行）。sed 范围 `/start/,/end/p` 也行但包含边界行。
> - **避免重复调用 CLI**：把 `xcodebuild -showdestinations` 的输出存到变量一次，再多次 grep；不要每个分支都重跑 CLI（旧代码就这么干，慢且竞态）。
> - **dogfood 验证 awk/sed 表达式**：用 heredoc 模拟两段都存在、只有 Available、目标在 Ineligible 三种 case，确认逻辑正确。
> - **反例**：
>   - `xcodebuild ... -showdestinations | grep -q "iPhone 17"` —— grep 全文，跨段命中，**不要这样写**。
>   - `if xcodebuild ... -showdestinations | grep ... ; then xcodebuild ... -showdestinations | grep ... ; fi` —— 重复执行 xcodebuild 拖慢脚本，且每次输出可能因为 simctl 状态变化不一致；**不要这样写**。
>   - `xcodebuild -showdestinations | head -20 | grep ...` —— 假设 Available 段在前 20 行，但段长不固定（机器有 20+ simulator 时会把 Ineligible 段也截进来）；**不要这样写**。

---

## Meta

构建脚本类的 review finding 通常聚焦"边界条件"——脚本在主路径（开发者本机一切正常）跑得通，但环境异常时（runtime 缺失、Xcode 版本不一致、simulator 没装）会静默选错或打印误导性错误。这一类 finding 的修复模式：**显式枚举所有可能的环境状态，每种状态走单独路径，避免依赖单一全局 grep 的偶然命中**。Story 2.7 的 build.sh 已经设计了三段 fallback（iPhone 17 → 通用 iOS Simulator → simctl UUID），但如果第一步的 detection 不严谨，整套 fallback 都被绕过——这是 review 真正抓的点。
