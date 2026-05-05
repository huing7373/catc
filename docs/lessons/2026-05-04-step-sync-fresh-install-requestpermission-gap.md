---
date: 2026-05-04
source_review: codex review round 1 (resumed) of Story 8-5-步数同步触发器（/tmp/epic-loop-review-8-5-r1-resumed.md）
story: 8-5-步数同步触发器
commit: 8f24404
lesson_count: 1
related_lessons:
  - 2026-05-04-auth-gated-feature-needs-explicit-requester-story.md
---

# Review Lessons — 2026-05-04 — fresh install HealthKit requestPermission gap：与 Story 8.4 motion 权限同坑（epic 切片漏专门 caller story 的二次复发）

## 背景

Story 8.5 落地 `SyncStepsUseCase` + `StepSyncTriggerService`. codex round 1 resumed review 抓到
**[P1]**：

> `iphone/PetApp/Features/Home/UseCases/SyncStepsUseCase.swift:56-57` — fresh install 路径
> 上没人调 `healthProvider.requestPermission()`，`readDailyTotalSteps` 抛 `.permissionDenied`，
> `StepSyncTriggerService` 吞错 → `/steps/sync` 在 fresh install 用户的 App 上 **silently fail
> forever**.

这是与 Story 8.4 round 5 P1 finding（`MotionProvider.requestPermission()` 没有 production caller）
**完全同 spec gap 的二次复发**——只是这次落到 HealthKit 而不是 CoreMotion. 8.5 story spec 文件
"Open Issues / Tech Debt" 段已**预先** surface 此 gap（落地前已写入），但 codex 看不到 spec 文档，
仅看代码 → 它正确地再次 surface 了同一个 gap.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | fresh install 没人调 `healthProvider.requestPermission()` → step sync silent fail forever | P1 | architecture | wontfix（与 Story 8.4 round 5 finding 1 同 precedent） | `iphone/PetApp/Features/Home/UseCases/SyncStepsUseCase.swift:56-57` + epic 8 sprint-status |

## Lesson 1: epic 切片的"权限申请 caller orphan"是元 spec gap，会在每个消费权限的 story 重复复发

- **Severity**: P1（high）— 但分诊为 wontfix（spec gap，与 8.4 round 5 finding 1 同 precedent）
- **Category**: architecture
- **分诊**: wontfix + 在本 story 文件 Open Issues 段已 surface（落地前）+ 本 lesson 二次记录元教训
- **位置**: epic 8 sprint-status.yaml（缺专门 caller story）+ 8.5 story 文件红线（钦定不调 requestPermission）

### 症状（Symptom）

1. 8.5 spec 红线：本 story **不**调 `healthProvider.requestPermission()`（与 Story 8.4 同红线 — 权限申请由统一入口处理）→ 正确.
2. 8.5 实装：`SyncStepsUseCase.execute` 直接调 `healthProvider.readDailyTotalSteps(date:)`，未授权时 `HealthProvider` 抛 `.permissionDenied` → 正确（与 8.1 协议契约一致）.
3. 但是 `healthProvider.requestPermission()` 在 production 代码里**只有 1 个 caller —— DevTools 入口的 HealthKit Probe View**（仅 dev mode 跳调试视图后才走）. 普通用户走 RootView 主路径的 fresh install 时永远不会触发权限请求.
4. 结果：fresh install 用户进 home → `service.start()` → `SyncStepsUseCase.execute` → `readDailyTotalSteps` 抛 `.permissionDenied` → `StepSyncTriggerService` `do { try await ... } catch { _ = error }` 吞错 → `AppState.currentStepAccount` 永远不更新 → HomeView `stepBalance` 永远显示 0 → 节点 3 demo 实际验收必然失败.

这与 Story 8.4 round 5 finding 1 是**对偶问题**：8.4 是 CoreMotion 权限没人申请 → `petState` 卡 `.rest`；8.5 是 HealthKit 权限没人申请 → `stepBalance` 卡 0. 同一个 epic-切片 gap 在不同 sensor adapter 的消费 story 上重复触发.

### 根因（Root cause）

**第一层根因**：与 Story 8.4 round 5 lesson `2026-05-04-auth-gated-feature-needs-explicit-requester-story.md` **完全同根因**——epic 切片把"权限申请"推卸给"后续 story 处理"，但 sprint-status 里没有那个 story.

**第二层根因（8.5 特有）**：fix-review 在末端 story 反复 surface 同一个 spec gap 时，Claude 的行为模式应当是**直接 wontfix + 引用前次 lesson**，而不是重新写一份 lesson 解释根因——但仍要写一份独立 lesson 是因为：

1. 元教训需要二次强化（一次复发还可以归咎为 8.4 切片漏写；二次复发说明 epic 切片设计阶段的**强制 cross-grep "权限申请 caller orphan" 检查**没有进 sprint-planning checklist —— 这是工作流级别的改进诉求，需要独立 lesson 让未来 sprint-planning 工作流知道要加这一项 check）.
2. 8.5 spec 钦定的"Open Issues 段已 surface"路径走通了（落地前 PM/SM 可以预读 spec 看到 gap），证明该范式可复用——下个权限消费 story（epic 9 跨端集成测试或后续 story）应当同样在 spec Open Issues 段预先 surface，而不是等 review 抓.
3. epic 8 切片如果**当初就开 8-6 "首启动流程权限申请统一入口" story**，HealthKit + CoreMotion 两个权限申请可以**合并到同一个 story** 处理——避免 8.4 / 8.5 两次重复 surface 同 gap. 这是 sprint-planning 工作流的 cross-story dependency 视角缺失.

**第三层根因**：review-loop 不应将"同一 spec gap 在不同 story 反复抓"视为 review 失败. codex 看代码视角下，每个独立 story 都会再次 surface 这个 gap，这是 review tool 的设计行为. Claude 的处置路径应当一致：wontfix + 引用前次 precedent + 在本 story spec 文件已有 Open Issues 段验证 gap 已 surface + 写独立 lesson 强化元教训.

### 修复（Fix）

**不修代码** —— 任何代码修改都会破坏 8.5 spec 红线（"本 story 不调 healthProvider.requestPermission；权限申请由统一入口处理"）.

**确认 spec gap 已 surface**：

1. **8.5 story 文件 Open Issues / Tech Debt 段**（落地前已 surface，commit `<pending>` —— 即本次 super-commit）：
   - 解释现状是 by design，不是 bug；
   - 与 Story 8.4 Open Issues 显式合并；
   - 建议三条路径让 SM / PM 决策：
     a. 新建 Story 8-6 "首启动流程权限申请统一入口"（同时处理 HealthKit + CoreMotion）；
     b. 合并到 epic-9 跨端集成测试做权限申请覆盖；
     c. 在 epic-8-retrospective 立项决定.

2. **本 lesson 文档**显式记录"epic 切片漏 requestPermission caller story 是会在多个消费 story 重复复发的元 spec gap"——为未来 epic 切片 / sprint-planning 工作流提供 checklist 依据.

3. **r1 lesson 文件**（`2026-05-04-cross-midnight-single-captured-date-and-idempotent-start.md`）已在同 commit 内独立落地（2 条代码 fix lesson）——本 lesson 是该 commit 的第三个 lesson 文件，commit hash 在 super-commit 完成后由 r1 lesson backfill 流程一并 backfill.

### 预防规则（Rule for future Claude）⚡

> **一句话**：fix-review 抓到"前次 review 已分诊为 wontfix 的 spec gap 在不同 story 二次复发"时，
> 处置路径**不变**（wontfix + lesson + 引用前次 precedent），但要在 lesson 里**显式标记元教训**：
> 这不是 review 漏抓，也不是 dev 漏修，是 epic 切片阶段漏写专门 caller story 的复发——下次
> sprint-planning 工作流必须 cross-grep `requestPermission` / `register` / `setup-side-effect`
> 类 API，确保每个 production caller 都有 story owner.
>
> **展开**：
> - **二次复发本身就是信号**：同一 spec gap 在 8.4 / 8.5 / 8.6 ... 不同 story 重复出现说明 epic
>   切片设计阶段没有专门检查 "这个 epic 内所有 sensor / API 的 setup 入口（requestPermission /
>   register / configure / reset）是否都有 caller story"——这是 sprint-planning 工作流应当强制
>   做的 cross-story dependency check.
> - **Spec 文件 Open Issues 段是关键 firewall**：落地前在 spec 写 Open Issues 段（与 8.4 同范式）
>   能让 PM / SM 在 review 前看到 gap，避免 review 阶段才暴露. 8.5 spec 已落地此范式 → 证明可
>   推广. 未来权限消费 / 资源消费类 story 都应在 spec 阶段预先 surface "如果某个 caller 没有
>   owner story，会发生什么"的 worst-case 描述.
> - **review tool 不区分"已 surface 的 spec gap" vs "新 bug"** —— codex 看到代码缺 caller 就抓
>   P1，不会读 spec Open Issues 段判断已 surface. 这是 tool 设计行为. 处置路径不能是"训练 codex
>   不抓已 surface 的 gap"（codex 不可控），而是"Claude 在 fix-review 阶段先 cross-ref spec Open
>   Issues 段判断是否已 surface，已 surface 就 wontfix + 引用前次 lesson + 写元教训 lesson".
> - **lesson 二次复发应当链接前次 lesson**：本 lesson frontmatter `related_lessons` 字段链接
>   `2026-05-04-auth-gated-feature-needs-explicit-requester-story.md`，让 distillator 蒸馏时
>   能识别"复发 lesson"模式 → 蒸馏成单一 cheatsheet 条目"epic 切片必须 cross-grep
>   setup-side-effect API 的 caller orphan".
> - **三次复发是红线**：如果 8-6 / 8-7 等后续 story 又抓到同 gap（例如未来 NotificationCenter
>   permission / Microphone permission），就不能再 wontfix 了——必须立即 escalate 为 epic-level
>   blocker 强制开 caller story 才能继续 sprint. 二次复发是元教训信号，三次复发是 sprint 流程失败.
>
> **判断模板（Claude 拿到 fix-review 时）**：
> ```
> if review_finding.是已 surface 的 spec gap（在 spec 文件 Open Issues 段有同条目）:
>     if 前次同 gap 已有 lesson:
>         分诊 = wontfix
>         actions = [
>             reference_prev_lesson,
>             confirm_spec_open_issues_section_already_surfaces,
>             write_meta_lesson_strengthening_epic_slicing_rule,
>             commit_with_other_dev_changes,
>         ]
>     else:
>         分诊 = wontfix + 写首次 spec gap lesson（如 8.4 round 5）
> else:
>     按代码 bug 路径 fix + lesson + 单测覆盖
> ```

---

## Meta: 本次 review 的宏观教训（与 8.4 round 5 Meta 段对偶）

8.4 round 5 Meta 段写："review round 5 同时抓出 P1（feature 未启用）+ P2（race 真 bug）. P2 是
代码层面的真 bug 必须修；P1 是 spec 层面的 gap，分诊为 wontfix + tech debt surface."

8.5 round 1 (resumed) Meta 段：本次 codex 第一次 review 给了 3 条 finding（P1 spec gap + P2 race +
P3 duplicate sync），上次 sub-agent 已修了 P2 / P3，本次 resumed review 只剩 P1 spec gap. 这说明：

- **代码 bug** review-loop 走"fix + lesson + 单测覆盖"路径正确（P2 / P3 round 1 fix 成功，resumed
  review 不再 surface）.
- **spec gap** review-loop 在每次 review 都会再次 surface（P1 round 1 已分诊为应 wontfix，本次
  resumed 仍被 codex 抓）——这是 review tool 设计行为，不是 review-loop bug.
- **fix-review 工作流的"resumed review"分支**应当识别 P1 类 spec gap 已在前次（或同次落地前 spec）
  surface，直接 wontfix + lesson 而不重新尝试修代码. 本次（5-04）就是按此路径处置.

未来 fix-review 工作流改进点：在 `分诊` 阶段加一步 "cross-ref story spec Open Issues 段"，
看 review finding 是否已被 spec 预先 surface ——若是，直接走 wontfix + 引用前次 lesson 路径，
不进入"评估是否修代码"分支.
