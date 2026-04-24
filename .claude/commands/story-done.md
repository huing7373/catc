---
description: 一键收官当前 story：提交工作区所有修改 + 把 story 状态标记为 done
argument-hint: [story-key? | "custom commit message"?]
---

# /story-done

**目的**：把刚实装完的 story 收官 — 把工作区所有修改**按端 / 职能分组提交**到当前 branch（端级实装 → 收官 chore → 其他杂项），并把该 story 在 `sprint-status.yaml` 和 story 文件中的状态更新为 `done`。

## 前置假设

- 项目使用 BMAD 的 sprint-status.yaml 工作流
  - 路径：`_bmad-output/implementation-artifacts/sprint-status.yaml`
  - Story 文件：`_bmad-output/implementation-artifacts/<story-key>.md`，其中第 3 行左右有一行 `Status: review`
- 正常情况下，目标 story 应处于 `review` 状态（刚跑完 `/bmad-dev-story` 的产物）
- 直接在当前 branch commit，不创建 feature branch，不做 push

## 参数解析

用户参数：`$ARGUMENTS`

- **空**：自动识别 `sprint-status.yaml` 中**唯一**处于 `review` 状态的 story key
- **匹配 `^\d+-\d+` 正则**（如 `1-1` 或 `1-1-mock-xxx`）：作为目标 story key
- **被引号包起来的其他字符串**：作为自定义 commit message，story 识别仍走自动逻辑
- **同时提供 key 和引号 message**：按参数顺序分别使用

## 执行步骤

### 1. 安全前检

- 跑 `git status --short` 和 `git rev-parse --abbrev-ref HEAD`，确认：
  - 不在 rebase / merge / cherry-pick 进行中（`git status` 不能含 `rebase in progress` / `You have unmerged paths`）
  - 不是 detached HEAD（`git rev-parse --abbrev-ref HEAD` 不返回 `HEAD`）
- 扫 `git status` 的输出，若含以下任一 → **警告并要求用户明示确认**（不直接拒绝）：
  - `.env` / `.env.*`
  - `*credentials*` / `*secret*` / `*token*`（文件名）
  - `*.key` / `*.pem` / `*.p12`
  - `*.sqlite` / `*.db`（大二进制）
- 若工作区无任何修改 → 提示"无变更可提交"，继续步骤 2 但跳过步骤 4（允许仅更新 story 状态）

### 2. 识别目标 story

- 读取 `_bmad-output/implementation-artifacts/sprint-status.yaml`
- **若参数给了 story key**：
  - 在 yaml 中 grep 该 key，必须存在，且状态应为 `in-progress` 或 `review`（否则警告用户，让他确认是否继续）
- **若参数未给**：
  - 在 `development_status` 段找所有状态为 `review` 的 story key
  - 若**恰好 1 个** → 用它
  - 若**多个** → 列出后询问用户选哪个
  - 若**零个** → 回退扫描 `in-progress`；还是零个则报告"无可收官的 story，你可能需要先跑 `/bmad-dev-story`"并退出
- 用 `Glob` 在 `_bmad-output/implementation-artifacts/` 下查找 `<story-key>*.md`，确保只有 1 个匹配；读文件第一行提取 Title（形如 `# Story X.Y: 具体标题`）

### 3. 分组工作区变更 + 机械生成每组 commit message

**分组规则（机械执行，不询问用户）**：按 `git status --short` 输出里的文件路径前缀把所有变更拆成独立分组，每组对应一个 commit。顺序：Group A（story-done 主）先提交，其余按字母序。

> 用户明确要求："每次都分开提交，有无关文件也一起提交"—— 本命令默认**把工作区所有变更分组提交干净**，不留在工作区。若用户想排除某些文件，在调用本命令前自行 `git stash push <path>`。

**Group 0 — 端级实装**（零到多个，按端字母序排在最前）：

对每个"端"子目录（`server/` / `ios/` / `watch/`）独立分组，**前提**是工作区有该前缀的未 commit 文件（.go / Swift / go.mod / go.sum / 该端目录下的 configs / migrations / 资源文件等所有 `<端>/**` 文件）。

这是项目以往 commit 风格的固化（见 `7a12492 feat(server): Epic1/1.5 ...` / `0a0d108 feat(server): Epic1/1.3 ...` / `eade914 feat(server): Epic1/1.4 ...`）：实装类变更用 `feat(<端>)` 作为独立 commit，与收官 chore 分离 → 审计清晰 + revert 粒度天然对齐 story。

- **文件**：`<端>/**` 下所有工作区变更（含 modified + untracked + deleted；不含本命令异常条目如 `.gitignore` / `.claude/` / `_bmad-output/` / `docs/`）
- **Commit message 模板**：
  ```
  feat(<端>): Epic<E>/<X.Y> <story 标题>

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  ```
  - `<端>` = 目录名（`server` / `ios` / `watch`）
  - `<E>` = epic 编号（story key 第一段，例如 `1-6-...` → `1`）
  - `<X.Y>` = 点分编号（例如 `1.6`）
  - `<story 标题>` = 从 story file 第一行 `# Story X.Y: <标题>` 机械提取（不做二次精简，保持可复现）
- **顺序**：多个端同时实装时按 `ios` → `server` → `watch` 字母序分别 commit

**Group A — story-done 主**（必然存在，排在所有 Group 0 之后）：

- **文件**：`_bmad-output/implementation-artifacts/sprint-status.yaml` + `_bmad-output/implementation-artifacts/<story-key>.md`
- **Commit message（固定模板，不可调整）**：
  ```
  chore(story-<X-Y>): 收官 Story X.Y + 归档 story 文件

  story-done: <story-key> → done

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  ```
  - `<X-Y>` = story key 的前两段（例如 `1-5-测试基础设施搭建` → `1-5`）
  - `X.Y` = 同上转成点分（例如 `1.5`）

**Group B+（按路径前缀机械分组）**：除 Group 0 / Group A 外所有文件，按下表归类。同组内多文件共用一个 commit；不同组必须分别 commit：

| 文件路径模式 | Type | Scope | Subject 模板 |
|---|---|---|---|
| `.claude/commands/*.md`（单个） | chore | commands | `更新 /<命令名> 命令` |
| `.claude/commands/*.md`（多个） | chore | commands | `更新 N 个命令定义` |
| `.claude/settings*.json` | chore | claude | `更新 Bash allowlist` |
| `.claude/agents/**` | chore | agents | `更新 <agent 名>` |
| `.claude/skills/**` | chore | skills | `更新 <skill 名>` |
| `.claude/*`（其他） | chore | claude | `更新 <文件 stem>` |
| `docs/lessons/*.md` | docs | lessons | `补充 <lesson 标题>`（从 H1 提取） |
| `docs/**`（其他） | docs | `<一级子目录名>` 或 `docs` | `更新 <文件 stem>` |
| `scripts/**` | chore | scripts | `更新 <文件 stem>` |
| `_bmad-output/**`（不属于 Group A 的 .md / .yaml） | docs | bmad-output | `更新 <文件 stem>` |
| 其他 | chore | — | `更新 <文件 stem>` |

**Body 默认空**。除非分组内涉及多文件且主题非平凡（例如一组命令定义同时改了 3+ 个文件），可在 body 自动列出文件名清单；否则省略 body。

**严格遵守**：按上述规则直接生成每组 message 并提交，**不向用户展示草稿、不询问"调整 message"**。这是本命令的**硬约定**，由用户在命令设计阶段明确要求（与 /fix-review 一致）。

**异常中断条件**：无（原"server 代码未 commit → HALT"规则已改为 Group 0 默认行为；用户 2026-04-24 确认后续一律按此处理）。

### 4. 先更新 story 状态（让 Group A commit 同时含状态）

- **编辑 `sprint-status.yaml`**：
  - 把 `<story-key>: review`（或 `in-progress`）改为 `<story-key>: done`
  - 更新顶部 `last_updated: YYYY-MM-DD` 为今天的日期（从系统读，不要让用户提供）
- **编辑 story 文件**：
  - 把 `Status: review`（或 `Status: in-progress`）改为 `Status: done`
- 不要动 story 文件里的 Tasks/Subtasks / Dev Agent Record / Change Log —— 只改 `Status:` 那一行

### 5. 输出执行摘要（不询问）

机械列出即将发生的 N 个 commit：

```
📦 本次将创建 N 个 commit（顺序：Group 0 → Group A → Group B+）：

Group 0 (server): feat(server): Epic<E>/<X.Y> <story 标题>
  files: server/...
  msg:   feat(server): ...

Group A: story-done 主
  files: sprint-status.yaml, <story-key>.md
  msg:   chore(story-<X-Y>): 收官 Story X.Y + 归档 story 文件

Group B: <type>(<scope>)
  files: ...
  msg:   <subject>

...（其他组）
```

**不询问 yes/no**。直接进步骤 6 循环提交。

> 用户若要取消必须手动 `Ctrl+C`；或者在调用前自行 `git stash` 排除不想入 commit 的文件。

### 6. 按分组循环提交

对每个 Group 依次执行：
1. `git add <组内文件，逐个显式列出>` — **不用 `git add -A`**
2. `git commit -m "$(cat <<'EOF' ... EOF)"`（保留 Conventional Commits + Co-Authored-By 尾行）
3. 记录 commit hash
4. 若 pre-commit hook 失败 → 修问题后**新建 commit**，禁用 `--amend`

### 7. 输出最终状态

- ✅ Group A commit hash + message 首行（story-done 主）
- ✅ Group B+ commit hash 列表（每组一行）
- ✅ Story `<story-key>` 状态：`review → done`
- 📋 剩余 sprint 统计：`grep -c 'ready-for-dev\|in-progress\|review' sprint-status.yaml`
- 💡 建议下一步：
  - 若有下一个 `ready-for-dev` → 建议 `/bmad-dev-story` 开始下一个
  - 若都跑完 epic → 建议 `git push` 推送到远程
  - 若 story 是"跨端 demo 验收"节点 → 建议人工真机测试

## 硬约束（绝不违反）

- ❌ 不跑 `git push`（push 由用户手动触发）
- ❌ 不跑 `git commit --amend`（始终创建新 commit）
- ❌ 不跑 `git commit --no-verify`（hook 失败就修，不跳过）
- ❌ 不改 story 文件里 `Status:` 以外的任何部分
- ❌ 不删除 / 移动文件，只做 add + edit + commit
- ❌ 不切 branch / 不 rebase / 不合并

## 边界情况

- **story 文件里 `Status: review` 已经不是 `review`**（例如被手动改过）：警告用户当前 Status 值，询问是否仍改成 `done`
- **sprint-status.yaml 和 story 文件状态不一致**：报告不一致，让用户先对齐再跑本命令
- **目标 story 在 epic 内是最后一个**：commit 之后提示用户 epic 可以标记 `done`（但不自动改 epic 状态，让用户决定是否跑 retrospective）
