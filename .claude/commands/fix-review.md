---
description: 接收 code review 结果 → 逐条修复 + 归档 lesson → 单 commit 提交，lesson 文档供后续蒸馏喂给未来 Claude
argument-hint: [review-file-path | "<review 正文>" | 空（进入粘贴模式）]
---

# /fix-review

**目的**：把一次 code review 的发现转成"修复 + 经验沉淀"。所有 fix 和所有 lesson **作为单个 commit** 落地；lesson 以结构化 markdown 沉在 `docs/lessons/` 下，形成可蒸馏语料，未来 Claude 读到同类场景时不再踩同一个坑。

## 前置假设

- 项目工作区是 git repo，当前 branch 可直接 commit（不自动新建 branch）
- Code review 的产出形态是自由文本（可能是人类写的、也可能是 LLM 产出的结构化报告），**不**强求特定模板
- 目标修复范围 = 当前工作区的未提交代码（通常是 `/bmad-dev-story` 刚产出、还没 `/story-done` 的那批）。若 review 是针对已 merge 的旧代码，command 同样工作 —— 只是 commit 落在当前 branch
- Lesson 归档位置：`docs/lessons/`（不在 `_bmad-output/`，因为 lesson 是跨 story / 跨 epic 的长期知识）

## 参数解析

用户参数：`$ARGUMENTS`

- **空**：进入粘贴模式 —— 提示「把 review 全文粘贴进来，空行 + `EOF` 结束」。读到 `EOF` 为止作为 review 原文
- **首 token 是文件路径且文件存在**（用 `Read` 工具核实）：把整个文件内容作为 review 原文；剩余 token 作为 scope hint（可选）
- **被引号包起来的长文本** 或 **多行文本**：直接作为 review 原文
- **无法判定形态**：直接回显 $ARGUMENTS 给用户，询问是文件路径还是 review 正文

## 执行步骤

### 1. 解析 review 原文 → findings 列表

- 获得 review 原文后，把它拆成独立 findings。识别启发式：
  - 结构化报告（如 `### Finding 1` / `- [High]` / `## Issue:` 编号）→ 按编号拆
  - 自由文本 → 按段落 + 严重性关键词（`critical` / `must` / `should` / `nit`）拆
- 对每条 finding 提取以下字段（部分可留空，但 severity 必须归类）：
  - `title`：一句话主题
  - `severity`：`high` / `medium` / `low` / `nit`（没明写时按影响面保守归类）
  - `category`：从 `testing` / `config` / `error-handling` / `security` / `perf` / `style` / `docs` / `architecture` / `dependency` / `other` 里选一个
  - `location`：相关文件 + 行号（若 review 提到）
  - `evidence`：review 里对应的原话引用（1-3 行）
  - `proposed_fix`：review 建议的修复（可能没给）
- **不要**在此步骤做修复，只做结构化抽取

### 2. 逐条分诊 → 决定修还是不修

- 对每条 finding 明确分诊结果：
  - `fix`：确认是真问题，本命令修
  - `defer`：是真问题但不属于本命令范围（如需要大重构、跨 story），记为 lesson 但留着不修，commit message 里提醒用户开新 story
  - `wontfix`：review 结论站不住（技术错误 / 和 story AC 冲突 / 有意设计），**不修但必须 lesson 里记录为何不修** —— 这是蒸馏里很有价值的一类（防止未来 Claude 盲目听 review）
- 分诊要给理由 —— 尤其 `wontfix`，理由必须引用具体技术依据（文档 / 代码 / 约束）
- 把分诊结果以表格形式输出给用户，**此时先等用户确认**（「这份分诊同意吗？要调整哪条？」），不自动进入修复
  - 用户确认后才进步骤 3
  - 用户提出调整（如"这条我要改成 wontfix"）→ 更新表格后重新确认

### 3. 逐条应用修复（仅对 `fix` 类）

对每条标为 `fix` 的 finding：

- **先复现/定位**：读相关文件，确认 review 描述的位置与代码现状一致；若已自愈（别的改动顺带修了）→ 转为 `wontfix` 并在 lesson 记录
- **最小改动**修复：只改引起问题的代码，不顺手重构无关部分（遵循 CLAUDE.md 的"不做多余事"纪律）
- **跑相关测试**：
  - 若该模块有单测（如 `internal/infra/config/...`）→ `go test ./path/...`
  - 若涉及多模块 → 跑相关包
  - **不**每条 finding 后都跑全量测试，那浪费时间；全量回归留到步骤 4
- 单条 fix 失败（测试挂 / 实现卡住）→ 报告给用户，让用户选：① 跳过这条转 `defer`；② 换策略再试；③ 中止整个命令

### 4. 全量回归

- 跑项目主测试命令：
  - Go：`cd server && go vet ./... && go test ./...`
  - iOS：按 Xcode 约定（目前节点 1 iOS 还没铺，可跳过）
  - 若仓库有 `scripts/build.sh` 的新形态（Story 1.7 之后）→ 跑它
- 有失败 → 修完再跑；三轮修复仍失败 → HALT 并让用户介入
- **回归必须绿才能进步骤 5**

### 5. 生成 lesson 文档

**路径**：`docs/lessons/YYYY-MM-DD-<slug>.md`

- `YYYY-MM-DD` 从系统当前日期读
- `<slug>`：3-6 个 kebab-case 英文/拼音单词，概括本次 review 核心主题（例如 `gin-middleware-misuse` / `config-env-override-leak`）。slug 从 findings 主题中归纳，避免和同一天已存在的文件重名（重名则追加 `-2` / `-3`）
- 若 `docs/lessons/` 目录不存在 → 先 `mkdir -p`，同时创建/更新 `docs/lessons/index.md`（见步骤 5 末尾）

**文件结构**：

```markdown
---
date: YYYY-MM-DD
source_review: <review 的来源标记 —— 如 "/ultrareview output" / "manual review by user" / "file: reports/xxx.md">
story: <关联的 story key，若能识别；如 1-2-cmd-server-入口-配置加载-gin-ping>
commit: <稍后 commit 产生的 hash，步骤 7 完成后回填>
lesson_count: <条数>
---

# Review Lessons — <date> — <slug 的中文一句话概括>

## 背景

<1-3 句：这次 review 针对什么代码/story，review 来源是什么。帮未来读者定位。>

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | ... | high | testing | fix | `server/internal/app/bootstrap/router_test.go` |
| 2 | ... | medium | config | wontfix | `server/internal/infra/config/loader.go` |
| ... | | | | | |

## Lesson 1: <title>

- **Severity**: high / medium / low / nit
- **Category**: testing / config / ...
- **分诊**: fix / defer / wontfix
- **位置**: `path/to/file.go:42`（如有）

### 症状（Symptom）

<review 指出的问题表现，用简洁语言复述；避免拷贝 review 全文。>

### 根因（Root cause）

<为什么会犯这个错？是误解了哪个约定？是哪个文档没读？是哪个默认行为反直觉？**这是蒸馏的关键字段之一** —— 写清楚"Claude 当时的思维漏洞"。>

### 修复（Fix）

<做了什么改动。若 `wontfix` 则写"不修，理由见下"并在"预防规则"里直接给出为何 review 结论不成立。>

- diff 级描述或 before/after 片段（≤ 10 行）
- 若涉及多文件，分点列

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **<触发条件>** 时，**<必须 / 禁止 / 优先>** **<具体动作>**。
>
> **展开**：
> - <补充细节 1>
> - <补充细节 2>
> - **反例**：<描述什么样的实现算踩坑；越具体越好，蒸馏后作为 few-shot 负例>

## Lesson 2: <title>

<同上结构重复>

---

## Meta: 本次 review 的宏观教训（可选）

<如果本次 review 的多条 findings 指向同一个思维漏洞（如"总是忘记读 CLAUDE.md 的某节"），单独写一段 meta lesson；没有则省略本节。>
```

**index.md 维护**（`docs/lessons/index.md`）：

- 文件顶部有一行声明：`<!-- auto-maintained by /fix-review; human edits OK, next run preserves non-list text -->`
- 命令追加一行到文件末尾的表格：
  ```
  | <date> | [<slug 的中文概括>](<date>-<slug>.md) | <lesson 数> | <逗号分隔的 category> | <commit hash 短格式> |
  ```
- 表头（首次创建时写入）：
  ```
  | 日期 | 主题 | 条数 | 分类 | commit |
  |---|---|---|---|---|
  ```

### 6. 向用户展示待提交内容 → 等待确认

**不询问 commit message** —— message 由步骤 7 按固定模板机械生成（见步骤 7 的"commit message 构造规则"），**不**在此步骤展示草稿、**不**允许"调整 message"作为选项。用户通过步骤 2 的分诊确认已经锁定语义，message 只是这份语义的机械表达，不需要二次议事。

输出 3 块：

1. **分诊总表**（复用步骤 2 的结果，提醒用户上下文）
2. **代码变更清单**：`git status --short`
3. **将创建的 lesson 文件路径 + 预览**：展示 lesson 文件的开头和 "预防规则" 部分（全文可能较长，不必 dump）

询问：「确认执行吗？（yes / 调整 lesson / 取消）」

- **yes**：进入步骤 7，按模板生成 commit message 并直接 commit，**不再询问 message**
- **调整 lesson**：用户给出调整点 → 改完重新展示 → 再问
- **取消**：**回滚**：删除步骤 5 生成的 lesson 文件、撤回对 index.md 的追加、`git checkout --` 所有本命令做的代码修改（或更安全地用 `git stash push` + 报告 stash 名让用户自行恢复）。**必须向用户明示回滚动作**

### 7. 提交

**commit message 构造规则（机械生成，不询问用户）**：

- **Subject**（第一行）：`fix(review): <主旨>`
  - `<主旨>` 来源：直接用步骤 5 生成的 lesson 文件 H1 标题里 `—` 之后的片段（如 lesson 标题是 `Review Lessons — 2026-04-24 — Sample 模板的 nil DTO 兜底 & slog 测试 fixture 的 WithGroup 语义`，subject 主旨 = `Sample 模板的 nil DTO 兜底 & slog 测试 fixture 的 WithGroup 语义`）
  - 主旨过长（> 60 字符）时，用 "severity × 数量 + Category 关键词" 压缩，如 `P2×2 — architecture / testing`
- **Body**（空行后）：4 行固定结构，按模板填充
  ```
  - 修复 N 条 finding（High X / Med Y / Low Z）
  - <逐条 lesson 标题一句话复述，最多 3 条；超过 3 条省略号>
  - 归档经验：docs/lessons/<date>-<slug>.md
  - 未修复：K 条（wontfix R，defer D；详见 lesson 文档）
  ```
  - 若 K = 0，"未修复" 行改成 `- 未修复：0 条`（保留这行，**不**省略）
- **Footer**（空行后）：
  ```
  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  ```

**严格遵守**：按上述规则直接生成 message 并 `git commit`，**不向用户展示草稿、不询问"是否调整 message"**。这是本命令的**硬约定**，由用户在命令设计阶段明确要求（避免每次 commit 前都被打断问一遍）。

**提交机制**：

- `git add` 逐个文件添加（代码变更 + lesson md + index.md），**不用 `git add -A`**
- 超过 10 个文件才允许 `git add -A`，且必须先告知用户
- `git commit -m "$(cat <<'EOF' ... EOF)"` 方式提交；保留 `Co-Authored-By` 尾行
- pre-commit hook 失败 → 修完**新建 commit**（绝不 `--amend`）
- 拿到 commit hash 后 → **回填**到刚生成的 lesson 文件 frontmatter 的 `commit: <hash>` 字段，以及 index.md 那一行的 commit 列
  - 回填本身也要产生一次 diff —— 此时用 `git commit --amend --no-edit`？**不允许**。改成**第二个 commit**（`chore(lessons): backfill commit hash for <date>-<slug>`）
  - 或者更简单：把回填放到步骤 8 之前，**在主 commit 前先算出 commit hash**（`git commit` 后 `git rev-parse HEAD`），然后做回填 commit。两个 commit 是本命令的例外（主 commit + hash 回填 commit），**不违反"一次 review 一次 commit"的精神**，因为回填是机械操作
- 更优方案（推荐）：主 commit 时 lesson 文件里 `commit:` 字段留 `<pending>`，主 commit 完成后做一次 `chore(lessons): backfill <hash>` 的小 commit。向用户明示这两次 commit 的语义区分

### 8. 输出最终状态

显示：
- ✅ 主 commit hash + message 首行
- ✅ 回填 commit hash（若有）
- 📄 lesson 文件路径
- 📊 分诊统计：`Fixed: N / Deferred: D / Wontfix: W`
- 🎯 给未来 Claude 的 top 1 预防规则（从 lesson 文件里挑 severity 最高的那条的"预防规则"一句话回显）
- 💡 下一步建议：
  - 若分诊含 `defer` → 提示「需要开新 story 跟进 defer 项：<列表>」
  - 若本次 fix 改动涉及多个 epic 的代码 → 提示「考虑把 lesson 也反链到对应 story 的 Change Log」
  - 若 lessons 累计到一定数量（≥ 20 条文件）→ 提示「考虑 /bmad-distillator 把 lessons 目录蒸馏成紧凑 cheatsheet」

## 硬约束（绝不违反）

- ❌ 不跑 `git push`
- ❌ 不 `git commit --amend` / `--no-verify`（pre-commit 失败就修）
- ❌ 不切 branch / 不 rebase / 不合并
- ❌ **不盲从 review**：任何条 finding 分诊为 `wontfix` 时，必须给技术理由并写入 lesson
- ❌ **不隐藏 wontfix**：wontfix 条目和 fixed 条目用同一套结构记录，不能只写"已修"回避没修的
- ❌ 不在 lesson 里写"Claude 犯了这个错"这种自我批判句式 —— lesson 是给未来 Claude 看的规则手册，不是道歉信。写**客观的触发条件 + 行动规则**
- ❌ 不修改 review 原文 / 不在 commit message 里引用 review 的任何 PII（如 reviewer 姓名 / 内部 ticket 号，除非原文已包含）

## 边界情况

- **review 里全是 nit / style 吐槽，没有 high/med**：命令仍跑，但分诊阶段会明确提醒"本次 review 无关键问题，lesson 价值较低，是否仍归档？"让用户决定
- **review 完全误解了代码**（每条都 wontfix）：依然生成 lesson，主题改为"为什么这些 review 意见不成立"—— 这种反面 lesson 对未来 Claude 特别有用（避免被错误 review 误导）
- **修复牵连到不在 review 范围的代码**：允许，但在 lesson 的"修复"字段里明示"顺带改动：<path>"并解释为何必要（避免 scope creep）
- **lesson 文件同名冲突**：同一天多次跑本命令 → slug 追加 `-2` / `-3` 做区分；**不**合并到单一文件（不同 review 的语境可能不同）
- **工作区本来就是脏的 + review 对应的是某次尚未 commit 的改动**：正常流程；commit 会把 fix + lesson + 原改动一起打包。若用户希望原改动和 fix 分开 commit，需要用户自己先 `git stash` 隔离，本命令不自动处理
- **无可用测试命令**（极小仓库 / 纯文档变更）：步骤 4 的"全量回归"降级为"人眼 diff 复核 + 让用户确认"
