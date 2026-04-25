---
description: 自动循环跑完一个 epic 的全部 story（create-story → dev-story → codex review → fix-review → story-done），通过 sub-agent 隔离每步 context，全 epic done 自动停
argument-hint: [epic_num | 空（自动取当前 in-progress epic）]
---

# /epic-loop

**目的**：把一个 epic 的全部 story **机械化跑完** —— 主 agent 维护循环 + 状态校验，每个重活（创建 story、实装、修 review）派给独立 **sub-agent** 跑（独立 context 不污染主线）；review 阶段调外部 **`codex` CLI**（OpenAI Codex）做"不同 LLM 独立 review"。

## 设计前提（必读）

- **Sub-agent 隔离**：BMAD 的 `/bmad-create-story` / `/bmad-dev-story` / `/fix-review` / `/story-done` 直接在主 session 跑会**线性累积** context（每条 story 几万 token），10 条就爆窗口。本命令的核心架构是 **每步 dispatch 一个 `general-purpose` sub-agent**（用 Agent tool）—— sub-agent 跑完只回结构化摘要给主 agent，主 agent 重读 `sprint-status.yaml` 二次校验状态，不依赖 sub-agent 自报
- **codex CLI 必装**：`which codex` 必须能找到。本命令用 `codex review --uncommitted` 跑非交互式 review（等价于交互模式选 "review uncommitted changes"）
- **不同 LLM 做 review**：主线全程用 Claude；review 阶段调 codex（GPT 系）—— 满足 BMAD "code-review 推荐用不同 LLM" 原则
- **直接在当前 branch commit**，不开 feature branch，不 push（push 由用户手动触发）
- **demo 验收 epic 默认拒绝**：epic 3 / 6 / 9 / 13 / 16 / 19 / 22 / 25 / 28 / 31 / 34 / 36 是"跨端 demo 验收"节点，含真机联调，自动循环跑不动 —— 命令检测到这些 epic_num 直接报错退出，让用户手动跑

## 参数解析

用户参数：`$ARGUMENTS`

- **空**：从 `_bmad-output/implementation-artifacts/sprint-status.yaml` 读 `epic-N: in-progress` 的 N 作为目标
  - 0 个 in-progress epic → 报错"无 in-progress epic，跑 `/bmad-create-story` 启动下一个"
  - 多个 in-progress epic → 列出后让用户挑
- **整数**（如 `4`）：作为目标 epic 编号
  - 必须在 `sprint-status.yaml` 出现且不是 done
  - 在 demo epic 黑名单（3/6/9/13/16/19/22/25/28/31/34/36）→ 直接拒绝
  - 状态是 backlog → 警告 "epic-N 还是 backlog，会先跑 /bmad-create-story 把第一条 story 拉起，连带把 epic 状态变 in-progress（这是 bmad-create-story 的副作用），确认继续？"
- **整数 + flag `--allow-demo`**：放行 demo epic 黑名单（你必须知道自己在干啥；通常**不**用）
- **整数 + flag `--auto-stash`**：跳过步骤 1 工作区 dirty 时的交互询问，直接 `git stash push -u -m "epic-loop autosave <ISO timestamp>"` 把脏文件 stash 走再继续。适合"我知道自己在 resume，所有 dirty 都是上次中断留下"的场景。**不**用 stash 一律不删用户工作 —— stash 可用 `git stash list` 查看 + `git stash pop` 恢复
- 多 flag 可同时给：`/epic-loop 4 --auto-stash --allow-demo`

## 硬约束（绝不违反）

- ❌ 不跑 `git push` / `git rebase` / `git reset --hard` / `git commit --amend`
- ❌ 不切 branch / 不 merge / 不 cherry-pick
- ❌ 不**自动**跑 retrospective —— epic done 后停下让用户决定
- ❌ 不**自动**跨 epic（当前 epic 全 done 立即停，**不**自动跳下一个 epic 继续跑）
- ❌ sub-agent 跑完后不能直接相信结果 —— 必须重读 sprint-status.yaml 二次校验状态流转
- ❌ codex review 输出**不**直接 pipe 给 fix-review，必须主 agent 自己看一眼判通过/不通过（避免 codex 抽风产出空 review 时把"无 finding"误传给 fix-review）
- ❌ **主 agent 自己绝不 commit / 不改 sprint-status / 不改 story 文件** —— 所有这类副作用都通过 sub-agent 完成（dev-story / fix-review / story-done 各自 commit）。主 agent 只做：调度 sub-agent + 重读 sprint-status 校验 + 调 codex review + 输出进度 + （唯一例外）`--auto-stash` 模式下跑一次 `git stash push -u -m "epic-loop autosave ..."` 把脏文件备份。这条约束保证 commit 历史的作者归属清晰（每个 commit 都来自对应的 BMAD workflow），而非 epic-loop 偷偷做事；stash 不是 commit、且无损可恢复，作为唯一例外允许
- ❌ 主 agent 调 codex review Bash 时**必须**显式 `timeout: 600000`（10 分钟）—— 默认 120s 不够大 diff 跑完
- ❌ sub-agent prompt 里**必须**显式禁止递归调用 `Skill(skill="epic-loop")` / `Skill(skill="loop")` / `CronCreate`

## 执行流程

### 1. 启动校验

按顺序检查（任一失败 → 报错退出，除非另有说明）：

1. **不在 rebase / merge / cherry-pick**
2. **当前 branch 不是 detached HEAD**
3. **codex CLI 可用**：`which codex` 必须返回路径
4. **`sprint-status.yaml` 存在且能解析**
5. **目标 epic_num 解析成功且不在 demo 黑名单**（除非 `--allow-demo`）
6. **git 工作区状态**（`git status --short` 检查）：
   - **clean** → 直接进步骤 1.5
   - **dirty** + 给了 `--auto-stash` flag：
     ```bash
     git stash push -u -m "epic-loop autosave $(date -Iseconds)"
     ```
     然后输出"📦 已自动 stash 脏文件；恢复用 `git stash list` 查看 + `git stash pop`"，进步骤 1.5
   - **dirty** + 没给 `--auto-stash`：HALT 给用户 3 个选项：
     ```
     🛑 工作区有未提交修改：
     <git status --short 输出>

     这通常是以下两种情况之一：
       a) 你刚做完手动修改还没 commit
       b) 上次 epic-loop 中途 quota 耗尽 / 异常中断留下的半成品

     选择恢复方式：
       1) 我手动 commit / stash / checkout 想保留的部分，再重跑 /epic-loop
       2) 重跑命令时加 --auto-stash，让 epic-loop 自动备份再继续
       3) 取消

     （epic-loop 不主动 reset 任何工作 —— stash 是无损备份，能 git stash pop 恢复）
     ```
     用户选 1/3 → HALT 退出；选 2 → 退出，让用户带 flag 重跑

### 1.5 恢复点诊断 + 一致性校验

工作区已 clean 后（步骤 1 通过 / 已 auto-stash），做"上次跑到哪了"的诊断：

a) **从 sprint-status 找恢复点**：扫当前 epic 下第一条状态 != done 的 story，作为 `resume_point` (story_key + status)
b) **看 git log 推断实际进度**：跑 `git log --oneline -10`，识别本 epic 相关 commit：
   - `chore(story-<X-Y>): 收官 ...` → story X-Y 已完成 story-done 流程
   - `fix(review): ...` → 至少跑过 1 轮 fix-review（无法精确定位是哪个 story 的，commit subject 看 lesson 标题推断）
   - `feat(<端>): Epic<E>/<X.Y> ...` → story X-Y 走过了"无 fix-review 直接 story-done"路径
c) **状态-文件一致性校验**（resume_point 是否有对应产物）：
   - status = `backlog` → 期待 `_bmad-output/implementation-artifacts/<story_key>.md` **不存在**（文件由 create-story 阶段创建）
     - 文件**已存在** → ⚠️ 警告但**不** HALT；可能是上次 create-story 跑完但状态没流转，让主循环按 backlog dispatch 时 create-story 自己处理（idempotent）
   - status ∈ `{ready-for-dev, in-progress, review}` → 必须能找到 `_bmad-output/implementation-artifacts/<story_key>.md` 文件
     - 文件**不存在** → **HALT**：
       ```
       🛑 sprint-status 状态超前于产物：story 文件未创建

       sprint-status 说: <story_key> 是 <S>
       但文件 _bmad-output/implementation-artifacts/<story_key>.md **不存在**

       根因（最常见）：
         - 你手动把 sprint-status 状态改了，但没跑 /bmad-create-story 把文件蒸出来

       恢复方法（推荐）：
         1) 把 sprint-status 中 <story_key> 状态改回 backlog
         2) 重跑 /epic-loop <N>，主循环会进 case 'backlog' → 派 create-story 自动补文件 + 推到 ready-for-dev
         （或者手动跑一次 /bmad-create-story，但需要先把状态改回 backlog 让它"看到"该 story）
       ```

d) **状态-git log 一致性校验**：
   - 看 `resume_point.status` 与 git log 是否吻合：
     - status = `ready-for-dev` 或 `backlog` → 期待 git log 中**没有**该 story 相关 commit ✓
     - status = `review` → 期待 git log 中**没有** chore(story-<X-Y>) 但**可能**有 fix(review) ✓
     - status = `done` → 不应作为 resume_point（已被步骤 a 排除）
     - status = `in-progress` → 异常状态（详见步骤 3 case 'in-progress'）

e) **输出恢复诊断**（不询问，让用户看到状态）：
   ```
   🔄 恢复点诊断

   sprint-status 中断点: <story-key> (status: <S>)
   git log 最近 5 commit:
     <hash> <subject>
     ...
   一致性: <✓ 一致 | ⚠ 不一致：<具体差异>>
   下一步动作: <create | dev | review | done> sub-agent
   ```

f) **不一致处理**（步骤 d 的状态-git log 不吻合时）：
   ```
   🛑 sprint-status 与 git log 不一致

   sprint-status 说: <story-key> 是 <S>
   git log 显示: 最近 chore(story-X-Y)/fix(review) 是 <hash> <subject>
   矛盾点: <详情>

   可能原因：
     - 上次 sub-agent 改文件后 sprint-status 没及时同步
     - 你手动改过 sprint-status

   建议：
     1) 看 git log + git show 核对实际进度
     2) 手动校正 sprint-status 后重跑 /epic-loop <N>
   ```
   一致性 HALT **不**自动 reset / 不自动 stash —— 这是诊断错误的征兆，需要人介入

### 2. 报告执行计划（不询问 yes/no）

机械列出：

```
🔁 epic-loop 启动：epic-<N>

当前 epic-<N> 状态: <in-progress | backlog>
当前 epic 下 story 状态分布:
  done: A 条
  review: B 条 (会从这步恢复)
  in-progress: C 条 (异常状态，会触发 HALT)
  ready-for-dev: D 条
  backlog: E 条

预计循环动作（按 story 顺序，状态机驱动）：
  - <story-key-1>: <当前状态> → 接下来做 <action>
  - <story-key-2>: ...
  ...

每条 story 都会经过：
  create (如需) → dev → codex review → fix (最多 5 轮) → story-done
```

**不询问 yes/no**。直接进步骤 3 循环。用户要取消按 Ctrl+C。

### 3. 主循环

伪代码（**主 agent 在每个进度节点输出一行短消息**让用户能跟上）：

```
loop:
  1) 重读 sprint-status.yaml（每轮都重读，让用户能手动改 status 介入循环）
  
  2) 列出当前 epic-N 下所有 story 按 story_num 升序
  
  3) 找"first non-done story"（按顺序第一条状态 != done 的）
     如果没找到 → epic 全部 done → 跳到步骤 7 报告，退出循环
  
  4) 输出进度行（每轮都打）:
     "🔁 [epic-N] story <story-key> (<状态>) → 派 <action> sub-agent"
  
  5) 根据该 story 的当前状态 dispatch:
     
     case 'backlog':
       → DISPATCH create_story_subagent(epic=N)
       → 输出: "✓ create_story 完成；重读 sprint-status 校验中"
       → 重读 sprint-status，验证：该 story_key 状态从 backlog 变成 ready-for-dev
       → 如果状态不变 / 变成别的 → HALT 报告"create-story 未按预期推进"
     
     case 'ready-for-dev':
       → DISPATCH dev_story_subagent(story_key)
       → 输出: "✓ dev_story 完成；重读 sprint-status 校验中"
       → 重读 sprint-status，验证：状态从 ready-for-dev 变成 review
       → 如果状态变成 in-progress（dev-story HALT 中途）→ HALT
       → 如果状态没变 → HALT
     
     case 'review':
       → 输出: "▶ 进入 review/fix 子循环（最多 5 轮 codex review）"
       → 进入 review/fix 子循环（详见步骤 4）
       → 每轮 codex review 后输出: "  · review round <r> → <通过|不通过|HALT>"
       → 每次 fix-review sub-agent 后输出: "  · fix_review 完成 (commit <hash>)"
       → 子循环正常退出（review 通过）→ 输出: "✓ review 通过；派 story_done"
       → DISPATCH story_done_subagent(story_key)
       → 重读 sprint-status，验证：状态从 review 变成 done
       → 如果状态没变 → HALT
     
     case 'in-progress':
       → HALT 报告"sprint-status 状态异常: story X 是 in-progress 但没有 active session 在跑；可能上次循环异常中断；请手动核查工作区 + 决定是 resume（手动改回 ready-for-dev）还是放弃（手动改回 backlog）"
     
     case 'done':
       不应到达此分支（步骤 3 已过滤）→ logic error → HALT
     
     other status:
       → HALT 报告"未知 story 状态: <status>"
  
  6) 循环回到步骤 1（重读 sprint-status）
```

### 4. Review/fix 子循环（review 状态分支详解）

```
# 进入 review 子循环时，立即抓基线 HEAD —— 这是 dev_story_subagent 跑前
# （= 进入 review 状态前）的 HEAD。后续 fix-review 会在此之上 commit。
# 重启 /epic-loop（resume）时，每次进入此分支都重新抓一次 baseline，
# 因为 resume 后 review_round 计数器归零。
baseline_commit = $(git rev-parse HEAD)
review_round = 0

loop until break:
  review_round++
  if review_round > 5:
    → HALT 报告"5 轮 review 都没通过"
    → break
  
  # ========== 跑 codex review（命令选择按轮次切换）==========
  # ⚠️ 重要：codex review 的 --uncommitted / --base 选项跟 [PROMPT] 位置参数
  # **互斥**（codex CLI v0.121+ 限制：传 PROMPT 会报 "argument cannot be used
  # with [PROMPT]"）。所以**不传**自定义 prompt —— codex 自己会 agentic 地读
  # 项目根的 CLAUDE.md / docs/lessons/ / _bmad-output/.../decisions/ 等做上下文，
  # 实战表现良好（首跑 epic-1 story 1-10 时 3 轮 review 都准确识别问题）。
  if review_round == 1:
    # 第 1 轮：dev_story_subagent 留下的改动 = 工作区 dirty（dev-story 不 commit）
    review_cmd = "codex review --uncommitted"
  else:
    # 第 2+ 轮：上轮 fix-review 已 commit；working tree clean
    # 用 --base 看从 baseline 起的累计 diff（含上轮 fix-review 的 commit）
    review_cmd = "codex review --base ${baseline_commit}"
  
  output_path = "/tmp/epic-loop-review-<story-key>-r<round>.md"
  
  # 主 agent 用 Bash tool 调（关键：timeout 600000ms 而非默认 120000ms，
  # codex 大 diff + agentic 跑 build/test 验证一次可跑 2-5 分钟）
  Bash(
    command: "${review_cmd} > ${output_path} 2>&1",
    timeout: 600000,
    description: "codex review round <round> for <story-key>"
  )
  
  # ========== 主 agent 读 codex 输出，LLM 自己判断 ==========
  读 ${output_path}
  
  ⚠️ **第 2+ 轮的输出陷阱**：`codex review --base <baseline>` 看 base..HEAD
  累计 diff 时，会把上轮 fix-review 留下的 lesson md 全文也当 diff 内容输出
  在文件**前面**（比如 round 2 输出可能 2900+ 行，前面是 lesson 内容引用）。
  **真实的 review 结论永远在文件末尾的 "codex" 段**（最后约 30-100 行）—— 主
  agent 判断时**只看末尾 codex 段**，前面 lesson 引用部分忽略。
  
  判断（用 LLM 综合理解，不要硬卡格式）：
    - codex 实际输出格式不固定：可能是 `### Finding N` / `- [P2] xxx` /
      自然语言 "did not find any actionable issues" / 等。**不**强求标准格式
    - **通过**信号（任一）：
      - 末尾 codex 段含 "REVIEW APPROVED" / "no findings" / "no actionable
        correctness issues" / "no issues found" / 等明确通过语
      - 末尾 codex 段没有任何 finding 标记（无 `[P1/P2/P3]` / 无 `### Finding`
        / 无 "must fix" / 无 "should be addressed"）
    - **不通过**信号（任一）：
      - 末尾 codex 段含 `[P1]` / `[P2]` 标记（P1/P2 = high/medium → 必修）
      - 含 "should be addressed before considering" / "actionable issue" / 等
      - 含 `### Finding` + severity high/medium
    - **次要 finding**（仅 [P3] / low severity / "nit" / "style only"）→
      视为通过 + 在最终报告 flag "次要 finding 见 ${output_path}（未自动修，
      可后续单 PR 处理）"
    - 输出为空 / codex 报错（API quota / 网络挂）/ exit 非 0 → HALT，**不**
      消耗 review_round 计数（重启 /epic-loop 时计数器从 0 起算）
    - 含 "Reconnecting..." / "ERROR: ..." 但末尾仍有 codex 段 → 视为有效，
      按 codex 段判断（这是 codex 的瞬时网络抖动，能补救）
  
  if 视为通过 OR 通过 + flag:
    break  # 退出子循环，回主循环让 story-done 收官
  
  # ========== 视为不通过 → DISPATCH fix_review_subagent ==========
  注意：fix-review.md 步骤 2 要求"等用户确认分诊"——sub-agent 没 user 通道，
  必须在 prompt 里明示"你就是用户的代理，自己分诊自己确认，直接进步骤 3"。
  
  → DISPATCH fix_review_subagent(
       story_key=<story-key>,
       review_findings_path=${output_path},
     )
  
  # fix-review sub-agent 跑完后会 commit（按 fix-review.md 步骤 7）；
  # 回 loop 顶进入下一轮，下一轮用 --base baseline_commit 看累计 diff
```

**Commit 范围隐含行为**（不主动改 fix-review，仅文档说明）：

- 第 1 轮 fix-review 进入时 working tree dirty（dev-story 留下） → fix-review 步骤 7 一并 commit dev-story 改动 + fix + lesson（这是 fix-review.md 前置假设第 3 条钦定的语义）
- 第 2 轮起 working tree clean → fix-review 只 commit 当轮的 fix + lesson
- 如果 review 第 1 轮就通过（无 fix-review 介入）→ working tree 仍 dirty → 下游 story_done_subagent 按 /story-done 分组规则提交 dev-story 改动

### 5. Sub-agent 派发协议

每个 sub-agent 用 `Agent` tool 启动，`subagent_type` = `general-purpose`，`isolation` 不指定（默认在主 worktree 跑）。

**通用 prompt 框架**（每个 sub-agent 都按这个套）：

```
你被 /epic-loop 派来执行一个独立的 BMAD 任务，**不**是来做整个循环。
完成后用结构化 markdown 总结返回，主 agent 会重读 sprint-status.yaml 校验你的工作。

项目根目录: C:\fork\cat
当前 git branch: <branch>
当前 HEAD: <commit-hash>

任务: <task-specific>

工作流文档: <path-to-skill-workflow.md>
你**必须**用 Skill tool 调用对应 BMAD skill（不要自己重新理解 workflow）：
  Skill(skill="<skill-name>", args="<args>")

完成后返回（≤300 字）:
  - 最终 sprint-status 中目标 story_key 的状态值
  - 修改的文件清单（仅文件名）
  - 任何 HALT / 异常情况的原因（如果有）

**禁止递归调用**：不要调 Skill(skill="epic-loop")、Skill(skill="loop")、
Skill(skill="schedule")、CronCreate —— 你是 epic-loop 派出的子任务，
调它自己会无限递归。如果你判断当前任务超出 sub-agent 能力范围，HALT 报告即可。

不要尝试做循环外的事；不要修改 sprint-status.yaml 之外的任何配置；不要 push。
```

**4 类 sub-agent 的具体 task 段**：

#### 5a. `create_story_subagent(epic_num)`

```
task: 调用 Skill tool 执行 bmad-create-story，让它自动从 sprint-status.yaml
找出 epic-<epic_num> 下第一个 backlog 状态的 story 并生成 story 文件 +
更新状态为 ready-for-dev。

完成判定: sprint-status.yaml 里该 story_key 的状态从 backlog 变成 ready-for-dev。
```

→ Skill 调用：`Skill(skill="bmad-create-story", args="")`

#### 5b. `dev_story_subagent(story_key)`

```
task: 调用 Skill tool 执行 bmad-dev-story，目标 story 是 <story_key>，
按 workflow 完整跑红绿循环 + 实装 + 测试 + 把状态推到 review。

完成判定: sprint-status.yaml 里 <story_key> 状态从 ready-for-dev 变成 review。

注意: bmad-dev-story 会跑完整 workflow，包括 build + test 验证；如果遇到
任何 HALT 条件（构造函数缺依赖 / 测试持续失败等），按 workflow 指引 HALT
并在返回总结里**清晰**列出 HALT 原因（不要硬撑跑通）。
```

→ Skill 调用：`Skill(skill="bmad-dev-story", args="")`（dev-story 自动找 review/in-progress 优先）

#### 5c. `fix_review_subagent(story_key, review_findings_path)`

```
task: 按 /fix-review 命令的工作流（.claude/commands/fix-review.md）处理
codex review 的输出。

review 原文文件: <review_findings_path>
目标 story: <story_key>

⚠️ **关键 override #1**（必须严格遵守）:
  fix-review.md 步骤 2 写"把分诊结果以表格形式输出给用户，**此时先等
  用户确认**" —— 在你这个 sub-agent 上下文里，**你就是用户的代理**：
    1) 读 review 原文 + 自己分诊（按 fix-review.md 步骤 1-2 的启发式）
    2) 把分诊表格写在你的回复里供事后回查
    3) **不**等任何 ask / 确认 —— 你既没 user 通道也没主 agent 反馈通道
    4) 直接进步骤 3 修复
  违反这条会让你卡死等输入 → 主 agent 收到空响应 → epic-loop HALT。

⚠️ **关键 override #2 — review 文件解析陷阱**（必须严格遵守）:
  review_findings_path 文件**只有末尾的"codex"段是真实 review 结论**。第 2+ 轮
  时 codex 跑 `--base <baseline>` 会把上轮 fix-review commit 的 lesson md 全文
  也当成 diff 内容输出在文件**前面**（可能 2000+ 行）—— 那是上轮已修过的
  lesson 引用，**不是**本轮新 finding。
  
  解析顺序（必须按此）：
    1) tail -100 review_findings_path 先看末尾
    2) 找最后一个 `^codex$` 行，从那行往后读 = 本轮真实 review 结论
    3) 真实结论里的 finding（`[P1/P2/P3]` / `### Finding` / 自然语言指出的
       问题）才是你要修的
    4) 文件前面的 `### Lesson N:` / `## Lesson N` / `# Review Lessons —`
       等 lesson 文档片段**全部忽略**（那是上轮产物的 git diff 引用）
  
  反例：把上轮 lesson 里的"反例"段当成本轮 finding 又修一遍 → 重复修复 +
  浪费一轮 review_round 计数。

其他约束:
  - 修完后**正常 commit**（按 fix-review 步骤 7 既定行为）
  - lesson 文档归档到 docs/lessons/ 按 fix-review 标准模板
  - **不要**走 commit hash backfill 的"第二个 commit"环节（fix-review 步骤 7
    末尾的 `chore(lessons): backfill <hash>`）—— 占用主 agent 重读
    sprint-status 的判断成本；让 backfill commit 留给后续手工或后台 agent 做

完成判定:
  - 工作区 clean（修复已 commit）
  - 新 commit 已落地（git log -1 应能看到 fix(review): ... 类型 commit）
  - sprint-status.yaml 中 <story_key> 状态**仍是** review（fix-review 不动状态）

返回时附上:
  - 分诊表格（severity / category / fix|defer|wontfix）
  - 修了 N 条 / defer M 条 / wontfix K 条
  - 新 commit hash
```

→ Skill 调用：`Skill(skill="fix-review", args="<review_findings_path>")`

注意：fix-review 也注册成 skill（commands 都自动注册）。两种调法都加载同份
.md 文档进 context；用 Skill 与 5a/5b/5d 对齐（统一从 BMAD skill 入口走）。
**关键**：上面 ⚠️ 那段"自我代理用户分诊"的 override 必须保留 —— 不论 Skill
还是 Read 调法，加载的 fix-review.md 步骤 2 都有"等用户确认"指令，sub-agent
必须按 prompt 的 override 自己代理用户，否则会卡死

#### 5d. `story_done_subagent(story_key)`

```
task: 按 /story-done 命令的工作流（.claude/commands/story-done.md）收官
当前 story <story_key>，把 review → done + 按分组提交所有工作区残留改动。

注: story-done.md 自身就**不询问 yes/no**（步骤 5 末尾"不询问 yes/no。
直接进步骤 6 循环提交"）—— 不需要额外 override。直接读文档执行即可。

完成判定:
  - sprint-status.yaml 里 <story_key> 状态变成 done
  - story 文件第 3 行 "Status:" 变成 done
  - 工作区 clean
  - git log 至少多 1 条 commit（chore(story-X-Y): 收官 ...）
  - 如果 review 第 1 轮就通过（未走 fix-review）→ 还会有
    feat(server) / docs / chore(claude) 等多个 commit（按分组规则）
  - 如果走过 fix-review → working tree 进 story-done 时已 clean →
    只 1 个 chore(story-X-Y) commit
```

→ Skill 调用：`Skill(skill="story-done", args="")`

注意：story-done 也注册成 skill（commands 都自动注册）。空 args 让命令按既定
逻辑自动识别"唯一 review 状态的 story"作为目标 —— 当前进 story_done 子分支
时该 story 必然是唯一 review 状态（主循环顺序处理），所以参数省略最简单且不
易出错

### 6. HALT 处理

任意 HALT → 主 agent 立即停止循环，输出：

```
🛑 epic-loop HALT

Epic: <N>
中止位置: 处理 story <story-key> 的 <create | dev | review-r<N> | fix-r<N> | done> 阶段
原因: <详细原因；review 失败要带 codex output 节选；sub-agent 报错带原文>

当前 sprint 状态:
  - <story-key>: <最后已知状态>
  - 已完成 (本次循环): N 条
  - 剩余未完成: M 条

恢复方法:
  1. 检查工作区 / sprint-status.yaml 是否一致
  2. 修问题（手动跑 /bmad-dev-story 或 /fix-review 等单步命令）
  3. 把出问题的 story 状态改成期望的入口状态（如 review / ready-for-dev）
  4. 重新跑 /epic-loop <N> 继续（循环每轮重读 sprint-status，会从中断处自动恢复）

中间产物（保留供诊断）:
  - codex review 输出: /tmp/codex-review-<story-key>-r*.md
  - 上次 sub-agent 返回摘要: <最后一次 sub-agent 的返回原文>
```

### 7. 正常完成

epic 全 done 时输出：

```
✅ epic-<N> 完成

成果:
  - 完成 story 数: M 条（new: A 条 / 续做: B 条）
  - 总 commit 数: K（feat: X / chore: Y / docs: Z / fix(review): W）
  - codex review 跑了 R 次，第一次通过率 P%
  - 5 轮 review 才过的 story: <list>（如有）
  - 创建的 lesson 文档: L 个（路径列出）

epic 状态: in-progress → 仍然 in-progress（**不**自动改 done；用户决定）

📝 待手工 backfill（如有 lesson 文档）:
  fix_review_subagent 按 epic-loop override 跳过了 fix-review.md 步骤 7 末尾
  的 `chore(lessons): backfill <hash>` 二次 commit。lesson 文档里的 frontmatter
  `commit:` 字段 + index.md 末行 commit 列**保留为 <pending>**。
  
  要补 backfill：
    1) 跑 git log --oneline --grep='^fix(review):' -n L
       把每个 fix(review) commit hash 找出来
    2) 对每个 lesson 文档，把 frontmatter `commit: <pending>` 改成对应 hash
    3) 同步改 docs/lessons/index.md 末尾 N 行的 commit 列
    4) 单 commit 收：chore(lessons): backfill commit hash for <epic-N>

建议下一步:
  1. 跑 /bmad-retrospective <N> 收 epic 经验
  2. 手动把 epic-<N> 状态改 done
  3. 跑 /epic-loop <N+1> 推下一个 epic（如果不是 demo epic）
  4. 或者 git push 把这批 commit 推远程
  5. 补 lesson backfill commit（见上方"待手工 backfill"段）
```

### 8. 边界情况

- **当前 epic 已经全 done，再跑本命令**：步骤 3 第一轮就找不到 non-done story → 直接进步骤 7 报"epic 已经完成；建议下一步"
- **review 状态的 story 实际工作区已经 clean**（用户可能手动 commit 后改了状态）：跑 codex review --uncommitted 会无输出 → 主 agent 判"视为通过 + flag"，直接 story-done。这个行为是 OK 的（用户已经独立完成 review）
- **codex review 报 API quota error / 网络挂**：HALT 报告 + 不消耗 review_round 计数（让重启循环时不被 5 轮上限错杀）
- **sub-agent 返回时间过长**（如 dev-story 跑了 1 小时）：不主动超时；信任 sub-agent；只在它返回时校验状态。Claude Code 的 Agent tool 没有 default timeout 强制
- **同一轮循环里 sub-agent 把 sprint-status 改成意料外的状态**（如 dev-story 直接推到 done）：步骤 3 重读时按新状态 dispatch；不报警告（信任流程，sprint-status 是 single source of truth）

## 与现有命令的关系

- `/bmad-create-story`：被本命令的 sub-agent 调用；**不**修改其 workflow
- `/bmad-dev-story`：同上
- `/fix-review`：sub-agent 模式调用，跳过"等用户确认分诊"环节；其余 workflow 不动
- `/story-done`：sub-agent 模式调用，跳过"列 commit 计划等确认"环节
- `/bmad-sprint-status`：本命令运行期间用户可独立跑它查看进度（不冲突）
- `/bmad-retrospective`：本命令**不**调用，epic done 后让用户手动决定

## 实现备注（写命令时的关键技术点）

1. **主 agent 的 Skill tool 不能直接被 sub-agent "继承"** —— sub-agent 自己有 Skill tool（general-purpose 是全 tool 集），但要在 sub-agent prompt 里**显式**告诉它"用 Skill tool 调用 bmad-X-Y"，否则 sub-agent 会自己重新理解 BMAD 流程，浪费 token + 容易跑偏
2. **fix-review 是 command 不是 skill** —— sub-agent 要靠 Read tool 读 `.claude/commands/fix-review.md` 然后按文档执行；prompt 里要把这点明确写出来
3. **每条 sub-agent prompt 必须自包含**：项目根路径、git 当前 branch、当前 HEAD hash、要做的事、完成判定条件 —— 全写齐，sub-agent 不知道主 agent 的 conversation 历史
4. **不要把 codex review 输出直接传 sub-agent**：写到 `/tmp/codex-review-<story-key>-r<round>.md`，让 sub-agent 用 Read tool 读 —— 避免 prompt 体积爆掉
5. **每轮主循环开始前，主 agent 用 Read tool 重新读 sprint-status.yaml**（不缓存）；判断时只信文件内容，不信 sub-agent 自报
6. **codex review prompt 用 here-doc 传**避免 shell 转义地狱：

   ```bash
   codex review --uncommitted - <<'EOF' > /tmp/codex-review-X.md 2>&1
   Review the diff against this story spec: ...
   EOF
   ```

7. **本命令文档本身禁止 import 进 sub-agent**：sub-agent 只看自己被 dispatch 的那段 task；epic-loop.md 的设计是给主 agent 看的
