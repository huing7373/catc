# Done Story Workflow

**Goal:** 将 story 标记为 done，更新 sprint-status.yaml，提交 git。

---

## INITIALIZATION

### Configuration Loading

Load config from `{project-root}/_bmad/bmm/config.yaml` and resolve:

- `implementation_artifacts`
- `communication_language`

### Paths

- `sprint_status` = `{implementation_artifacts}/sprint-status.yaml`

---

## EXECUTION

<workflow>

<step n="1" goal="确定目标 story">
  <check if="用户指定了 story key（如 0-2 或 0-2-app-entry）">
    <action>使用用户指定的 story key</action>
  </check>

  <check if="用户未指定">
    <action>读取 {sprint_status}，找到第���个状态为 "in-progress" 或 "review" 的 story</action>
    <check if="找不到">
      <ask>没有找到 in-progress 或 review 状态的 story。请指定 story key：</ask>
    </check>
  </check>

  <action>确认 story_key（如 "0-2-app-entry-and-runnable-lifecycle"）</action>
  <action>定位 story 文件：{implementation_artifacts}/{story_key}.md</action>
</step>

<step n="2" goal="更新 story 文件状态">
  <action>读取 story 文件</action>
  <action>将 `Status:` 行更新为 `Status: done`</action>
  <action>保存 story 文件</action>
</step>

<step n="3" goal="更新 sprint-status.yaml">
  <action>读取完整的 {sprint_status}</action>
  <action>将 development_status 中 {story_key} 的状态更新为 "done"</action>
  <action>更新 last_updated 为当前日期</action>
  <action>保存文件，保留所有注释和结构</action>

  <!-- 检查该 epic 下所有 story 是否全部 done -->
  <action>从 story_key 提取 epic_num（第一个数字）</action>
  <action>扫描 development_status 中所有 {epic_num}-* 的 story（排除 epic 和 retrospective 条目）</action>
  <check if="该 epic 下所有 story 都是 done">
    <action>将 epic-{epic_num} 状态更新为 "done"</action>
    <output>Epic {epic_num} 所有 story 已完成，epic 状态已更新为 done。</output>
  </check>
</step>

<step n="4" goal="提交 git">
  <action>运行 `git status` 查看所有变更</action>
  <action>运行 `git diff` 查看变更内容</action>
  <action>运行 `git log --oneline -3` 查看提交风格</action>

  <action>暂存所有相关变更文件（代码文件 + story 文件 + sprint-status.yaml）</action>
  <note>不要暂存 .claude/settings.local.json 或其他无关文件</note>
  <note>如果有未暂存的代码变更（非 story/sprint 文件），一并提交</note>

  <action>生成 commit message，格式：
    - 以 "feat:" / "fix:" / "chore:" 开头
    - 简短描述 story 的核心交付内容
    - 末尾添加 Co-Authored-By 行
  </action>

  <action>执行 git commit</action>
  <action>运行 `git status` 确认提交成功</action>
</step>

<step n="5" goal="输出结果">
  <output>
  ✅ **Story {story_key} 已完成并提交**

  - Story 文件：{story_file} → done
  - Sprint 状态：{sprint_status} → done
  - Git commit：{commit_hash}
  </output>
</step>

</workflow>
