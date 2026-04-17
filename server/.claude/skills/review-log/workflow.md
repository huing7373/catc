# Review Log Workflow

**目标：** 解析代码审查反馈，应用修复，将审查发现记录到经验日志，提交 git。

实际修复内容由 git commit diff 承载，日志只记录事实（错误模式 + 影响）。
未来蒸馏 Claude 可通过 `git log --grep="fix(review)"` 找到所有审查修复提交，
再用 `git show <hash>` 查看具体修改。

---

## INITIALIZATION

### Configuration Loading

Load config from `{project-root}/_bmad/bmm/config.yaml` and resolve:

- `implementation_artifacts`
- `communication_language`

### Paths

- `sprint_status` = `{implementation_artifacts}/sprint-status.yaml`
- `review_log` = `{project-root}/agent-experience/code-review-log.md`

---

## EXECUTION

<workflow>

<step n="1" goal="确定上下文">
  <check if="用户指定了 story key（如 0-2 或 0-2-app-entry）">
    <action>使用用户指定的 story key</action>
  </check>

  <check if="用户未指定">
    <action>读取 {sprint_status}，找到第一个状态为 "review" 或 "in-progress" 的 story</action>
    <check if="找不到">
      <ask>没有找到 review 或 in-progress 状态的 story。请指定 story key：</ask>
    </check>
  </check>

  <action>确认 story_key</action>

  <check if="review_log 文件已存在">
    <action>读取文件，统计该 story_key 已有多少 round 条目</action>
    <action>设 round_num = 该 story 已有 round 数 + 1</action>
  </check>
  <check if="review_log 文件不存在">
    <action>设 round_num = 1</action>
  </check>
</step>

<step n="2" goal="解析审查反馈">
  <check if="用户已在对话中粘贴审查结果">
    <action>直接解析对话中的审查结果</action>
  </check>
  <check if="用户未粘贴审查结果">
    <ask>请粘贴代码审查结果：</ask>
  </check>

  <action>从审查结果中提取所有 finding，识别：
    - 类别：patch / intent_gap / bad_spec / defer / 噪声驳回
    - 描述
    - 涉及的文件和行号
    - 具体问题和影响
  </action>

  <action>分类处理：
    - patch → 准备修复（Step 3）
    - intent_gap / bad_spec → 记录到日志，提示用户决定是否修复
    - defer / 噪声驳回 → 跳过，不记录
  </action>
</step>

<step n="3" goal="应用修复">
  <action>逐个修复 patch 类别的 finding：
    - 读取涉及的文件，理解问题上下文
    - 应用最小化修复，不做额外重构
  </action>

  <action>运行 `bash scripts/build.sh --test` 验证</action>

  <check if="构建或测试失败">
    <action>修复构建/测试问题，重新运行验证</action>
  </check>

  <check if="涉及 integration test 变更">
    <action>额外运行 `go vet -tags=integration ./...` 验证编译</action>
  </check>
</step>

<step n="4" goal="写入审查日志">
  <check if="review_log 文件不存在">
    <action>确保 agent-experience/ 目录存在</action>
    <action>创建文件，写入头部：

# 代码审查日志

记录每次代码审查的发现，供后续蒸馏提取编码规范。
实际修复内容见同一 git commit 的 diff（`git log --grep="fix(review)"` + `git show <hash>`）。

---
    </action>
  </check>

  <action>追加本轮条目，格式：

## [{story_key}] Round {round_num} — {YYYY-MM-DD}

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | {简述犯了什么错} | {file:line} | {为什么有问题} |
| 2 | ... | ... | ... | ... |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过 / ❌ 失败原因
  </action>

  <note>
    - 错误模式：用简短的技术语言描述犯了什么错（如"收到信号后未立即 cancel ctx"）
    - 影响：说明这个错误会导致什么后果（如"后续 Runnable 的 Start 循环收不到停止信号"）
    - 不记录"怎么修的"——git diff 就是修复记录
    - intent_gap / bad_spec 条目也记录在表格中，类别列标明
  </note>
</step>

<step n="5" goal="提交 git 并输出结果">
  <action>运行 `git status` 查看所有变更</action>
  <action>暂存所有相关变更文件（代码修复 + 审查日志）</action>
  <note>不要暂存 .claude/settings.local.json 或其他无关文件</note>

  <action>生成 commit message，格式：
    fix(review): {story_key} round {N} — {一句话概括本轮修复}

    Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
  </action>

  <action>执行 git commit</action>
  <action>运行 `git status` 确认提交成功</action>

  <output>
  ✅ **Review Round {round_num} for {story_key} 已完成并提交**

  - 修复了 {n} 个 patch
  - 记录了 {m} 个 intent_gap / bad_spec
  - 跳过了 {k} 个 defer / 噪声驳回
  - 审查日志：{review_log}
  - Git commit：{commit_hash}
  - 构建验证：通过

  如需下一轮审查，请运行 code review 后再次使用 /review-log。
  </output>
</step>

</workflow>
