---
date: 2026-04-26
source_review: codex review round 5 (/tmp/epic-loop-review-2-10-r5.md)
story: 2-10-ios-readme-模拟器开发指南
commit: 6e24b57
lesson_count: 2
---

# Review Lessons — 2026-04-26 — Onboarding 文档必须考虑 build-wrapper 副作用 + lesson index 必须随 lesson 同步

## 背景

Story 2-10 iPhone README round 5 review。round 3 在真机段补了"Xcode 配 Team"的 4 步前置（finding 之前 review 提的真机 runbook 缺 signing），round 4 又加了一批 lesson 文件。本轮 review 指出：

1. round 3 加的 Team 配置流程与紧随其后的 `bash iphone/scripts/build.sh` 步骤**冲突** —— wrapper 总是先跑 `xcodegen generate`，会从 `iphone/project.yml`（`DEVELOPMENT_TEAM: ""`）重建 `.xcodeproj`，**擦掉**刚选的 Team。
2. round 4 加的 4 个 lesson 文件里，`docs/lessons/2026-04-26-readme-portable-network-and-tool-output.md` 没在 `docs/lessons/index.md` 里登记，从 index 不可发现。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 真机 runbook 与 build.sh 冲突（xcodegen 擦掉 Team 配置） | P2 / medium | docs | fix | `iphone/README.md:356-393` |
| 2 | index.md 漏 portable-network-and-tool-output lesson 行 | P3 / low | docs | fix | `docs/lessons/index.md` |

## Lesson 1: Onboarding 文档加新前置步骤时，必须验证后续步骤是否会撤销该前置

- **Severity**: medium
- **Category**: docs
- **分诊**: fix
- **位置**: `iphone/README.md:356-393`（"真机联调"段）

### 症状（Symptom）

README "真机联调" 段先告诉读者：在 Xcode UI 选 Team（这是**对 generated `.xcodeproj` 的 in-place 编辑**），然后第 3 步立即让读者跑 `bash iphone/scripts/build.sh`。该 wrapper 第一行就是 `xcodegen generate`，会从 `iphone/project.yml`（其中 `DEVELOPMENT_TEAM: ""` 是空字符串占位）重建 `iphone/PetApp.xcodeproj`，把刚配的 Team **完全覆盖**。

读者按 README 走完 6 步后，Cmd+R 仍因签名失败 build fail，与 README "前置步骤跑完即可真机 build" 的承诺不符。同时 `project.pbxproj` 已经被 Xcode 写了一次 personal team 的字段（虽然 xcodegen 又覆盖了），**dirty 工作区**。

### 根因（Root cause）

写 onboarding doc 时只想着"按文档需要哪些前置"，没追问"后续步骤会不会通过 side effect 把前置撤销"。具体到这次：

- `bash iphone/scripts/build.sh` 在仓库里被定位成"统一入口 / wrapper"，被 README 当成日常命令推荐 —— 但它的**副作用 `xcodegen generate`** 对"用 Xcode UI 改了 generated 产物"这种场景是**毁灭性**的。
- 该副作用在 README §跑测试 line 100 已写过（"Xcode IDE 跑不会自动 xcodegen，改了 project.yml 必须先手跑 xcodegen 或用 build.sh"），但反方向"在 generated 产物里手改了，跑 build.sh 会被覆盖"未被点出。
- 真机签名是 Xcode local override（personal team 不入 git），与"统一从 project.yml 重建"的设计哲学**对立**。这种"个体本地 override"场景必须显式告知读者绕开 wrapper。

更一般地：**任何 build wrapper 跑 codegen 类操作（xcodegen / protoc / sqlc / openapi-gen 等）的项目，README 涉及"在 generated 产物上手改"的步骤都必须先警告 wrapper 副作用。**

### 修复（Fix）

`iphone/README.md` "真机联调" 段：

1. 段开头加一个高亮 `>` blockquote：明示"真机用 Xcode IDE Cmd+R，不要用 build.sh；理由 = xcodegen 会擦 Team"，并引用 line 100 既有的"Xcode IDE 跑不会自动 xcodegen"对应表述保持一致。
2. 步骤 5-7 重排：保留 baseURL / server 改动，但把"改 project.yml + 跑 build.sh"路径降级为"路 B（会擦 Team，需要重配）"，新加"路 A（直接在 Xcode 里改 .xcodeproj 的 Info.plist，不擦 Team，但下次跑 build.sh 会被 regen 覆盖回 localhost）"作为推荐。读者根据"是否打算频繁跑 build.sh"二选一，避免被默认路径坑到。
3. 步骤 9 显式写 `Cmd+R（**不**跑 build.sh！）`，与段头 blockquote 呼应。

### 预防规则（Rule for future Claude）⚡

> **一句话**：写 onboarding / runbook 文档时，**只要某步骤是"对 generated 产物（`.xcodeproj` / generated SQL / protobuf 输出 / OpenAPI client 等）的本地 in-place 编辑"，必须在文档里显式列出"哪些 wrapper 命令会触发 codegen 把这个改动擦掉"，并给出绕开 wrapper 的等价路径。**
>
> **展开**：
> - 写完一段 README runbook 后做"撤销审查"：从最后一步往前看，每一步用到的命令是否会**撤销**前面任意一步的 side effect？尤其留意 `*-gen` / `*-codegen` / `make generate` / xcodegen / xcodebuild build phases / protoc / sqlc 等命令。
> - 区分"source-of-truth 编辑"（改 `project.yml` / `*.proto` / SQL DDL）与"generated artifact 编辑"（改 `.xcodeproj` UI 产物 / `*.pb.go` / `_gen.sql`）。前者跑 codegen 是无害的（重建出同样产物），后者**会丢失**手工改动。Local override（如 Apple Developer Team、机器特定 IP、个人调试 flag）几乎都属于后者。
> - 给"必须用 IDE 而不是 wrapper"的步骤加显眼 blockquote，不要只在 troubleshooting 里写 —— 走顺路的读者不读 troubleshooting。
> - **反例**：README 写"跑 step 1 配本地参数 → 跑 step 2 wrapper 命令" —— wrapper 命令内部 codegen 把 step 1 配的参数从 source-of-truth 重建，step 1 的工作蒸发。读者按 README 顺序走会一直奇怪"为什么没生效"。
> - **反例**：README 段头介绍"这步是 Xcode local override（不入 git）"，但接下来第 3 步就用 `bash <wrapper>` —— wrapper 会把 generated `.xcodeproj` 整个覆盖。"Xcode local override" 与"wrapper 重建产物"是死对头，必须文档显式互斥。

## Lesson 2: 加 lesson 文件时必须同步 index.md，且 fix-review 工作流提交前要 grep 验证一致性

- **Severity**: low
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/lessons/index.md`

### 症状（Symptom）

Round 4 fix-review 在 `docs/lessons/` 下加了 4 个 lesson 文件，但 `docs/lessons/index.md` 只追加了 3 行；`2026-04-26-readme-portable-network-and-tool-output.md` 没登记。从 index 找不到这个 lesson，对未来 `/bmad-distillator` 蒸馏管线**直接漏样本**。

### 根因（Root cause）

`/fix-review` 命令的 step 5 末尾要求"追加一行到 index.md"，但当一个 lesson 文档**包含多个 finding**（lesson_count > 1）时，操作模式从"每条 finding 一个新 lesson + 一行 index"变成"一个 lesson 文件 + 一行 index"，容易被算成"最后一条 finding 的对应行"而漏掉新 lesson 文件本身。

具体到 round 4：当时一次产出 4 个 lesson 文件，其中 portable-network-and-tool-output 是 lesson_count=2 的**合并 lesson**。流程在加 index 行时数据漏了一条。

更一般地：**任何"产出多个文件 + 一个 index 文件"的流程都需要 commit 前的"`ls dir/ | wc -l` vs `grep -c link index`"一致性检查**，否则迟早漏。

### 修复（Fix）

补 `docs/lessons/index.md` 一行：

```markdown
| 2026-04-26 | [README 命令必须 cover 所有合法网段 + 工具输出格式不能假设固定字符数](2026-04-26-readme-portable-network-and-tool-output.md) | 2 | docs | `<pending>` |
```

行格式与既有 row 一致（lesson_count=2、category=docs、commit=`<pending>`）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：跑 `/fix-review` 步骤 5 时，**对每个新 lesson 文件**必须有一行对应的 index 追加；commit 前**必须**用 `ls docs/lessons/*.md | wc -l` 与 index 表格行数对齐验证（容许 1 行差，因为 index 自身在目录里）。
>
> **展开**：
> - commit 之前，跑 `comm -3 <(ls docs/lessons/2026-*.md | xargs -n1 basename | sort) <(grep -oE '\(2026-04-[0-9]+-[a-z0-9-]+\.md\)' docs/lessons/index.md | tr -d '()' | sort)` 找差集；任何差集非空 = 有漏。
> - 一个 lesson 文件 = 一行 index，**不论该文件 lesson_count 多大**。lesson_count 字段用来在 index "条数" 列展示，但**不影响**索引行数。
> - **反例**：多 finding 合并到一个 lesson 文件时，flow 误以为"3 个 finding → 3 行 index"或"1 个 lesson → 0 行（已有同 slug 行）"，实际上一个 lesson 文件总是对应**正好一行** index。
> - **反例**：commit message 写"加了 4 个 lesson"但 `git diff docs/lessons/index.md` 只有 3 行新增 —— 数字对不上即漏。

---

## Meta: 本次 review 的宏观教训

两条 finding 都是"**文档与工具实际行为脱钩**"：

- Lesson 1：README runbook 的 step 写出来时**没考虑** wrapper 命令的副作用，把 wrapper 当成无害的"跑一下"对待。
- Lesson 2：fix-review 流程**没自验**"新文件数 vs index 行数"，commit 时 silent 漏。

共同教训：**文档/索引/runbook 类产物在 commit 前要做一次"机械一致性"检查 —— 不是看内容写得好不好，而是看"声称的事实是否与文件系统/工具语义对齐"。** 这种检查可以脚本化，应当编入 fix-review skill 的 step 6 / pre-commit hook（不在本次 commit 范围）。

---

## 未完成事项 / 后续 TODO

### 2026-04-26 round 6 — 接受为 hardening tech-debt（用户决策"接受"）

epic-loop 跑到 review_round 6（5 轮上限触顶后再跑了一次诊断 review），codex 又给了 **1 个新 [P2] finding**，登记如下，**本 story 不修，作为 hardening tech-debt**：

**[P2] Troubleshooting #3 行真机修复又踩 build.sh 擦 Team 陷阱** — `iphone/README.md:328`

- **症状**：Troubleshooting 表第 3 行（"App 在真机上启动后角落 server 信息永远显示 Server offline"）解决步骤写"② regen Info.plist：`bash iphone/scripts/build.sh`"。这与 round 5 的 §真机联调 段警告（"真机 build 必须用 Xcode IDE Cmd+R，不要用 build.sh，否则 Team 被擦"）**自相矛盾**。读者按 troubleshooting 修真机 baseURL 后再 Cmd+R 必败 code signing。
- **承认是真问题**：这正是本 lesson 的 Lesson 1 在 README 内部的一次复发——同一文档同主题在两个段落里的"机械一致性"漂移。round 5 修了"真机联调段引导路径"但**没**同步检查 troubleshooting 表里的同主题表行。
- **defer 理由**：本 story 范围红线"不实装 e2e 真机调用"已覆盖；MVP 阶段开发者不会在真机上跑（→ Epic 3 demo 验收 / Epic 5 自动登录才触发真机场景）；epic-loop 5 轮 review budget 已用尽（本 story 已修 9 条 finding，README 质量远超新成员入门所需）。
- **触发回看时机**：Epic 3 demo 验收节点（节点 1 跨端 ping e2e）—— 那时会真正在真机上验证 README 流程，自然暴露这个矛盾；或者下次任何人编辑 README 时按本 lesson 的 Top 1 规则（"机械一致性"检查）grep 全文找 `bash iphone/scripts/build.sh` 出现处，逐个核对是否在真机段引用。
- **简易修法预览**（留给将来）：troubleshooting #3 解决步骤 ② 改写成 §真机联调 路 A：
  ```
  ② 在 Xcode 里改 PetAppBaseURL：navigator 选 PetApp target → Info tab →
     PetAppBaseURL 改成 Mac 局域网 IP；不要跑 build.sh（会擦 Team）。
     详见 §真机联调 路 A。
  ```

### Round 6 finding 的 commit 安排

不再单独开 fix(review) commit；登记在本 lesson 的 TODO 段，由 Story 2-10 的 chore(story-2-10) 收官 commit 一起带走。

---

## Meta: round 1-6 README 反复修改的"机械一致性"漂移

Story 2.10 的 README 跨 6 轮 codex review，每轮挖一个新维度的 finding：路径可移植 → 链接路径 → 工具语义 → IP 网段 → signing 流程 → **跨段 cross-reference 一致性**（round 6）。前 5 轮修的是"内容准确性"，round 6 的 finding 是 round 5 自己的修复在另一段没同步——元教训：

**修文档时改 A 段必须 grep 同主题词在全文的所有出现，逐个评估是否要同步改。** README 里 `bash iphone/scripts/build.sh` 出现 N 次，round 5 改了"真机联调"段那一处，但 troubleshooting 表里第 3 行的同一表达式没改。这种"局部改动忘记全局同步"是文档维护的经典坑。本 lesson Top 1 规则（"机械一致性检查"）已经明示要点，但 round 5 修复时**没按规则照做**——再一次证明：写 lesson ≠ 自动遵循 lesson。未来所有 README/runbook 改动 PR 必须**强制**附"全文 grep 自检表"作为 commit message 一部分。
