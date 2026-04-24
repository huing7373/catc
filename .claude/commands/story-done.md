---
description: 一键收官当前 story：提交工作区所有修改 + 把 story 状态标记为 done
argument-hint: [story-key? | "custom commit message"?]
---

# /story-done

**目的**：把刚实装完的 story 收官 — 把工作区所有修改作为**单个 commit** 提交到当前 branch，并把该 story 在 `sprint-status.yaml` 和 story 文件中的状态更新为 `done`。

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

### 3. 生成 commit message 草稿（若用户未提供自定义 message）

- 先 `git log -5 --oneline` 看最近 5 条 commit，学该仓库的 Conventional Commits 风格（例如已有 `docs(mvp): ...` / `chore(sprint): ...` / `docs(decision): ...`）
- 按变更内容**推断 type**：
  - 仅 `.md` + yaml 文件 → `docs(<scope>)`
  - 含 `.go` / `go.mod` 文件 → `feat(<scope>)` 或 `fix(<scope>)`（看 story 性质：新功能用 feat，bug 修复用 fix）
  - 仅 `_bmad-output/**` → `chore(story-X-Y)` 或 `docs(story-X-Y)`
  - 包含 `scripts/` / `.claude/` → `chore(<scope>)`
- **scope 建议**：`story-<story-key-short>` 或 `epic-<N>` 或模块名（如 `decision` / `sprint` / `cmd`）
- **subject 格式**：简短中文，动宾结构，说清这个 story 产出了什么（从 story title + Dev Agent Record → Completion Notes 提取关键动作）
- **message body**（可选）：若有关键决策/否决点值得 commit log 中记住，补 1-3 行；否则省略

**Commit message 格式**（HEREDOC 传递，保留换行）：
```
<type>(<scope>): <subject>

[可选 body]

story-done: <story-key> → done

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
```

### 4. 先更新 story 状态（让一个 commit 同时含实装 + 状态）

- **编辑 `sprint-status.yaml`**：
  - 把 `<story-key>: review`（或 `in-progress`）改为 `<story-key>: done`
  - 更新顶部 `last_updated: YYYY-MM-DD` 为今天的日期（从系统读，不要让用户提供）
- **编辑 story 文件**：
  - 把 `Status: review`（或 `Status: in-progress`）改为 `Status: done`
- 不要动 story 文件里的 Tasks/Subtasks / Dev Agent Record / Change Log —— 只改 `Status:` 那一行

### 5. 向用户展示待提交内容，等待确认

**必须输出**：
- 目标 story：`<story-key> — <title>`
- 工作区变更清单（`git status --short`）
- 即将执行的 commit message（完整 HEREDOC 内容）
- 即将把 story 状态改为 `done` 的确认

**询问**：「确认执行吗？（yes 继续 / 让我调整 message / 取消）」

- 若用户要调整 message → 改完重新问确认
- 若用户取消 → **回滚步骤 4 的编辑**（把状态改回 `review` 或原始值），报告"已取消，工作区未提交"

### 6. 提交

- `git add` **逐个文件**添加（按 `git status --short` 的输出循环），**不用 `git add -A` / `git add .`**
  - 若变更文件超过 10 个，允许在确认阶段用 `git add -A`，但显式告诉用户
- `git commit -m "$(cat <<'EOF' ... EOF)"` 方式提交（保留 Conventional Commits 格式 + Co-Authored-By 尾行）
- 若 pre-commit hook 失败 → 修问题后**新建 commit**，**禁用 `--amend`**
- `git log -1 --stat` 验证提交成功

### 7. 输出最终状态

显示：
- ✅ Commit hash + 简短 message
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
