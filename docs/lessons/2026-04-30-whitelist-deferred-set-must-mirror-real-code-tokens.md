---
date: 2026-04-30
source_review: epic-loop r4 codex review (/tmp/epic-loop-review-37-14-r4.md)
story: 37-14-design-package-白名单
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-30 — 白名单 r4：deferred 集合的"成员名 / 数量"必须以真实 code token 为准而非凭印象写

## 背景

Story 37.14 给 Epic 37「显式不做」清单建立白名单文档 `iphone/docs/ui-design-scope-whitelist.md`。前三轮 review 已修「位置 cite 真实承载源 / 流程文档 / 渲染路径」「未来 routing cite 真实 Story scope」三类 routing 错误。第四轮 codex review 单条 P2：白名单 entry 1 + entry 2 把 deferred theme set 写成 `candy / dark / mono 三套`，但 Story 37.5 实装的 `iphone/PetApp/Core/DesignSystem/ThemeColors.swift` 实际有 4 个 `public static let` —— `candy / matcha / sky / dark`。`mono` 不存在；`matcha` / `sky` 被遗漏。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | deferred theme set 写错（应是 candy/matcha/sky/dark 四套，文档写成 candy/dark/mono 三套） | medium (P2) | docs | fix | `iphone/docs/ui-design-scope-whitelist.md:49,55` |

## Lesson 1: 白名单 entry 描述 deferred 集合时，"成员名 / 集合规模"必须以真实 code token 为准

- **Severity**: medium (P2)
- **Category**: docs
- **分诊**: fix
- **位置**: `iphone/docs/ui-design-scope-whitelist.md:49`（entry 1 理由段）+ `:55`（entry 2 理由段）

### 症状（Symptom）

白名单 entry 1（tweaks-panel）和 entry 2（用户级主题切换 UI）的"理由"段都写：

> Epic 37 Theme stub 已落地 **candy / dark / mono 三套** token

但 `iphone/PetApp/Core/DesignSystem/ThemeColors.swift` 实际有 **4 个** `public static let` 实例：`candy` / `matcha` / `sky` / `dark`。`mono` 不存在；`matcha` 和 `sky` 被遗漏。

副作用：白名单本身就是「未来 mini-epic 起 story 时的 routing 参考」，把"数量 + 名字"两项都写错，会让后续 mini-epic 朝向不存在的 `mono` theme 实装、漏掉 `matcha` / `sky` 已有 stub 的 placeholder upgrade 任务。

### 根因（Root cause）

写 deferred 集合的成员清单时，**没有去 grep 真实 code 验证**，而是凭印象 / 凭"模板感"（"应该是 light + dark + 单色三套吧"）写出。这是文档与代码 drift 的常见模式：白名单文档把"形式上看起来合理"和"事实正确"当成同一件事。具体到 Theme：`mono`（单色）听起来是"design system 应该有的中性主题"，但 Story 37.5 的实装从不引入 `mono`，从一开始就是 `candy/matcha/sky/dark` 的 4-theme 命名空间（candy 完整 + 3 套 stub）。

review 流的容错：
- r1/r2/r3 都在改"位置 cite 路径"类问题（routing 表达），review 视野没扫到"集合枚举内容"层；
- AC4 grep 只验证 `位置 / 理由 / 何时做` 三段都在场（10/10/10/10），**不**校验段内容跟 code 的真实性。

### 修复（Fix）

把两处"三套 candy / dark / mono"改成"四套 candy / matcha / sky / dark"。

- `iphone/docs/ui-design-scope-whitelist.md:49`（entry 1 理由）：
  - before: `已落地 candy / dark / mono 三套 token`
  - after: `已落地 candy / matcha / sky / dark 四套 token`
- `iphone/docs/ui-design-scope-whitelist.md:55`（entry 2 理由）：
  - before: `Theme stub（candy / dark / mono 三套 token，编译期可切，...`
  - after: `Theme stub（candy / matcha / sky / dark 四套 token，编译期可切，...`

事实依据 grep：

```
$ grep -nE 'public static let (candy|matcha|sky|dark|mono)' iphone/PetApp/Core/DesignSystem/ThemeColors.swift
79:    public static let candy = ThemeColors(
98:    public static let matcha = ThemeColors(
115:    public static let sky = ThemeColors(
133:    public static let dark = ThemeColors(
```

无 `mono`；4 套 candy / matcha / sky / dark。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **白名单 / scope 清单 / deferred 列表的"理由"段需要枚举一个集合的成员（如 theme 名、route 名、AC ID 集合）** 时，**必须先 grep code 拿到真实 token 列表再写文字**，不得凭"模板感 / 命名直觉"补集合。

> **展开**：
> - 触发条件包括但不限于：枚举 theme set / route 名 / 表名 / 静态常量集 / AC ID 列表 / Story ID 集合 / 组件名 / scaffold view 名。
> - 操作流程：(a) 先确定 source-of-truth 文件（如 `ThemeColors.swift` / `Routes.swift` / `epics.md` story 段），(b) `grep -nE 'public static let X'` 或等价 query 拿到真实 token，(c) 字面拷贝到文档 —— 不允许中间夹"我记得是 X"。
> - 写完后做交叉 grep 校验：从文档抽出每个集合成员名，反向 grep code 是否存在。任何 doc 里出现但 code 里 grep 不到的成员，必删。
> - 这条与 r1/r2/r3 的"cite 真实位置"是同一类规则但在另一个维度：r1-r3 管"指向（pointer 写到正确文件 + 行号）"，本条管"集合的内容（list 里每个元素都是 code 真实存在的 token）"。两条都通过才算 doc-code aligned。
> - 当用户说"deferred theme set"或"白名单已枚举"时，把这当作高风险信号 —— 集合枚举是"看起来正确就过审"的高发区。
> - **反例**（本次踩坑）：白名单 entry 1+2 写「Epic 37 Theme stub 已落地 candy / dark / mono 三套 token」。`mono` 在 ThemeColors.swift 内 grep 不到任何 `public static let mono`；同时 grep 出 4 个真实 let（含 `matcha` / `sky`），均未进文档。这种"add 个不存在的 + 漏 2 个真存在的"双向错，是凭命名直觉而不 grep 的典型产物。
> - **反例的反例（正确做法）**：写 deferred theme set 之前先开终端跑 `grep -nE 'public static let (\w+) = ThemeColors\('` 一行，把输出 4 个名字字面写进文档。

---

## Meta: 本次 review 的宏观教训

Story 37.14 r1→r4 四轮 review 全部命中**白名单文档与 code/spec drift**：

- r1：位置 cite 没指向流程文档（只指向视觉壳）→ 修
- r2：deferred artifact 位置写到视觉壳 entry 而非真实承载源 → 修
- r3：未来 routing 错挂到不相关 Story（3D spike 挂到 Story 30.x）→ 修
- r4：deferred 集合枚举内容与 code 真实 token 不符（写 mono 而非 matcha/sky）→ 修

四轮一致结论：**「白名单 / scope 文档」是 doc-code drift 的高发区**，因为它没有自动测试守护，只能靠"写文档时对照 code grep"这条人/Claude 纪律守护。后续起类似 scope-listing story 时，应在 AC 里加一条机器可校验的"枚举成员存在性 grep"检查（如 AC：白名单 entry 内每个 backtick code token 必须在 repo grep 出至少 1 处真实 source code 引用），否则永远会重蹈本系列 r1-r4 覆辙。
