---
date: 2026-05-04
source_review: codex review round 5 of Story 8.4 (file: /tmp/epic-loop-review-8-4-r5.md)
story: 8-4-主界面猫-sprite-三态动画切换
commit: c457f93
lesson_count: 1
---

# Review Lessons — 2026-05-04 — auth-gated feature 切片必须显式分配"权限申请 caller" story，spec gap 不能静默漏到末端 story

## 背景

Story 8.4 round 5 codex P1 抓出：fresh install 路径上没有任何 production 代码调用
`MotionProvider.requestPermission()`，导致 `bind(motionProvider:)` 的"未授权"象限永远命中持引用
noop 路径，`petState` 卡 `.rest` → 8.4 的猫 sprite 三态切换 feature 对 fresh install 用户**实际未启用**.

但 8.4 spec 红线明确钦定"HomeViewModel 不调 requestPermission，权限申请由 8.5 / 8.6 / 8.1 等后续
story 统一处理"——这是**正确**的边界设计（让 ViewModel 只负责订阅链路 wire，权限申请由专门
入口统一管，避免 first-launch UX 在 4 个不同地方各弹一次系统弹窗）.

**问题不在 8.4 代码，而在 epic 8 切片设计**：epic-8 sprint-status.yaml 只有 8-1（HealthKit）/
8-2（CoreMotion adapter）/ 8-3（mapper）/ 8-4（订阅链路）/ 8-5（步数同步业务），**没有**专门
story 调 `motionProvider.requestPermission()`. 8.4 的红线把 caller 推给"后续 story"，但 epic
切片忘了实际造一个 caller. 结果 P1 是 spec gap 暴露而不是 8.4 实装 bug.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | fresh install 没人调 requestPermission，feature 实际未启用 | P1 | architecture | wontfix（升级为 epic-level tech debt） | `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift` + epic 8 sprint-status |

## Lesson 1: auth-gated feature 切片必须显式分配"权限申请 caller" story，spec gap 不能静默漏到末端 story

- **Severity**: P1 (high) — 但分诊为 wontfix（spec gap，不能在末端 story 私自破坏红线修代码）
- **Category**: architecture
- **分诊**: wontfix + 升级为 tech debt 显式 surface 在 8.4 story 文件 + epic-8 retrospective 立项决策
- **位置**: epic 8 sprint-status.yaml（缺 story）+ 8.4 story 文件红线（推卸 caller 到"后续 story"）

### 症状（Symptom）

1. 8.4 spec 红线：HomeViewModel 不调 requestPermission（避免 first-launch UX 破坏）→ 正确.
2. 8.4 实装：`bind(motionProvider:)` 在 `(false, false)` 象限持引用 noop（红线一致）→ 正确.
3. 但是 **`MotionProvider.requestPermission()` 在 production 代码里只有 1 个 caller —— DevTools probe**
   （仅 dev mode 跳调试视图后才走）. 普通用户走 RootView 主路径的 fresh install 时永远不会触发权限请求.
4. 结果：fresh install 用户进 home → bind 命中 (false, .notDetermined) → 持引用 return → handler
   永不被调用 → petState 永远 .rest → 三态切换 feature **实际未启用**.

### 根因（Root cause）

**epic 切片设计 gap**——把"权限申请"拆到独立 story 是好设计（避免 ViewModel 内嵌权限请求让 UX
破碎），但拆走的 caller 必须被**立即放回 sprint 计划的具体某个 story**. 不能用"后续 story 处理"
这种模糊推卸——"后续"如果实际不存在某个 story，spec gap 就漏到末端 story（review 才暴露）.

**第二层根因**：epic 切片的 dependency check 漏了"caller orphan 检查". sprint-planning 工作流
没有强制问"每个 spec 红线写'后续 story 处理 X'时，X 是不是 sprint-status.yaml 里某个具体 story
的 AC？". 单看 8.4 文件红线写得对，但 cross-epic 视角下"caller 在哪"是个 question mark.

**第三层根因**：review 时 codex 抓 P1 是基于"代码检查"——它看到红线说"8.5/8.6 处理"，但 epic
8 没有 8-6，8-5 是步数同步业务，8-4 自身红线又不让自己调 → 它正确地 surface 了 gap.
按 review 字面"修代码"会破坏 8.4 红线（违反 spec 边界）；按"忽略 codex"会让真问题漏到节点 3 demo.

正解是**升级为 spec-level tech debt**：代码侧不动（保持 8.4 红线一致），但在 story 文件 +
sprint-status 显式 surface gap，让 SM / PM 决定开 8-6 还是合并到 epic 9 / retrospective 立项.

### 修复（Fix）

**不修代码** —— 任何代码修改都会破坏 8.4 spec 红线（HomeViewModel 不调 requestPermission；
RootView 启动期调也违反"权限请求由后续 story 处理"的 epic 切片意图）.

**升级为 tech debt + lesson surface**:

1. **8.4 story 文件加 "Open Issues / Tech Debt" section**（已落地）：
   - 解释现状是 by design，不是 bug；
   - 指出 epic 8 sprint-status 缺一个调用 `requestPermission()` 的 caller story；
   - 建议三条路径让 SM / PM 决策：
     a. 新建 Story 8-6 "首启动流程权限申请统一入口";
     b. 挪到 epic 9 跨端集成测试做权限申请覆盖;
     c. 在 epic-8-retrospective（已 optional）立项决定.

2. **lesson 文档**（本文件）显式记录"epic 切片不能让 spec gap 漏到末端 story"的元教训.

3. **Change Log + Debug Log** 加一条记录"P1 wontfix surface 为 tech debt"（已落地）.

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude / SM 在做 **epic 切片** 时，每次 story spec 写"X 由后续 story 处理"
> （特别是权限申请 / 全局 setup / 单例初始化这类 caller-orphan 高发场景），**必须**立即在
> sprint-status.yaml 里指向**具体某个 story 的具体 AC**——不能用"8.5 / 8.6 之类"或"以后某个
> story"模糊语. **如果 sprint 里没有那个 story，立刻新建占位 story 或在 epic retrospective 立项**.
>
> **展开**：
> - 权限申请 / Info.plist usage description 触发 / global registry 注册 这类 setup-side-effect
>   非常容易在 epic 切片时被推卸到"统一入口 story"，而那个 story 实际从未被显式列入 sprint.
>   常见反例：`HomeViewModel` 红线说"8.5 处理权限"，但 8.5 spec 没写权限相关 AC——只读单 story
>   时看不出问题；review 才暴露.
> - 末端 story 的 review 抓到这种 P1 时，**不要**为了"清掉 review finding"违反 spec 边界私自
>   修代码（让 ViewModel / RootView 调 requestPermission）——这会让 spec 红线腐烂，未来 review
>   再看时不知道哪条还有效. **正解**：surface 为 tech debt，让上层决策.
> - 在 8.4 story 文件加 "Open Issues / Tech Debt" section 的范式可复用——任何 wontfix 但其实
>   是真问题的 review finding 都该在故事文件 surface，而不是只在 lesson 里 mention（lesson 太
>   分散，PM / 验收时不会逐个翻；故事文件 surface 让 demo 验收 walk-through 时强制看到）.
> - epic retrospective 工作流（`bmad-retrospective` skill）应当**专门检查** "spec 红线推卸的
>   caller 是否有具体 story"——这是 retrospective 的独立 checklist 项.
> - **反例**（本次 round 5 P1 暴露的反例）：
>   ```yaml
>   # epic-8 sprint-status.yaml
>   8-1-healthkit-接入: done            # HealthKit 权限：done
>   8-2-coremotion-接入: done            # 提供 MotionProvider.requestPermission() API
>   8-3-运动状态机映射: done             # mapper pure function
>   8-4-主界面猫-sprite-三态动画切换: review  # spec 红线："不调 requestPermission，由后续 story 处理"
>   8-5-步数同步触发器: backlog          # 步数业务，不涉及 motion 权限
>   # ↑ 这里"后续 story"指谁？没有 story 调 motionProvider.requestPermission！
>   ```
>   读 8.4 spec 单看正确；读 8.5 spec 单看也正确；只有把 epic 8 全 5 story 一起读 + grep
>   `requestPermission()` caller 才能发现 gap. epic 切片设计阶段必须做这个 cross-story check.
> - **正例**（应有的形态）：
>   ```yaml
>   8-1-healthkit-接入: done
>   8-2-coremotion-接入: done
>   8-3-运动状态机映射: done
>   8-4-主界面猫-sprite-三态动画切换: review
>   8-5-步数同步触发器: backlog
>   8-6-首启动流程权限申请统一入口: backlog   # ← 显式补上 caller story
>   ```
>   或在某个具体 story 的 AC 里明写"AC X: RootView .task 调 await
>   container.motionProvider.requestPermission()"——让 cross-grep 能找到 caller.

---

## Meta: 本次 review 的宏观教训

review round 5 同时抓出 P1（feature 未启用）+ P2（race 真 bug）. P2 是代码层面的真 bug 必须修；
P1 是 spec 层面的 gap，分诊为 wontfix + tech debt surface 而不是私自修代码——这两类 review
finding 的处置路径完全不同：

- **代码 bug**：fix + lesson + 单测覆盖.
- **spec gap**：wontfix + tech debt 在 story 文件 surface + epic retrospective 立项决策 +
  lesson 记录"切片设计教训"——**绝不**在末端 story 私自破坏 spec 边界修代码.

未来 Claude 拿到 review 时第一步分诊就要问"这是代码 bug 还是 spec gap？"——后者的处置路径
不是改代码而是 escalate 到 epic / sprint 层面.
