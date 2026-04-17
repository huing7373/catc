# Story Summary 文档生成工作流

**目标：** 为已完成（done）或已审查（review）的 story 生成一份面向人类阅读的中文实现总结，保存到 `server/human_docs/`。

**用途：** 帮助开发者快速了解每个 story 做了什么、怎么实现的、后续 story 如何衔接。

---

## 执行步骤

### Step 1: 确定目标 story

- 如果用户指定了 story（如 "总结 story 0-2"），使用指定的 story
- 如果未指定，读取 `_bmad-output/implementation-artifacts/sprint-status.yaml`，找到最近一个状态为 `done` 或 `review` 的 story（从后往前扫描，优先最近完成的）
- 加载对应的 story 文件：`_bmad-output/implementation-artifacts/{story-key}.md`

### Step 2: 分析 story 实现

从 story 文件中提取：
- Story 标题和目标（从 Story 段落）
- 完成了哪些任务（从 Tasks/Subtasks）
- 创建/修改了哪些文件（从 File List）
- 实现笔记（从 Completion Notes）
- 变更记录（从 Change Log）

同时检查实际代码：
- 读取 File List 中列出的关键文件，理解实现方式
- 如果有测试文件，了解测试覆盖范围

### Step 3: 生成总结文档

用中文撰写总结，包含以下结构：

```markdown
# Story {id}: {title} — 实现总结

{一句话概括这个 story 的作用}

## 做了什么

### {分类标题 1}
- 要点...

### {分类标题 2}
- 要点...

## 怎么实现的

{关键实现细节，包括用了什么技术、为什么这样设计}

## 怎么验证的

{运行了什么命令、测试结果}

## 后续 story 怎么用

- {下一个 story 会在哪里扩展}
```

**写作原则：**
- 面向开发者，假设读者了解 Go 但不了解本项目
- 解释"为什么"而不仅仅是"是什么"
- 提到具体的文件路径和包名
- 用简洁的中文，技术术语保留英文

### Step 4: 保存文档

- 保存到 `server/human_docs/story-{story-key}-summary.md`
- 如果文件已存在，提示用户是否覆盖
