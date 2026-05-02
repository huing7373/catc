---
date: 2026-05-02
source_review: "/tmp/epic-loop-review-37-13-r2.md (codex P2 round 2)"
story: 37-13-accessibility-identifier-总表
commit: 331222b
lesson_count: 1
---

# Review Lessons — 2026-05-02 — ADR layering guard 必须 token-match 而非 `TypeName(` 构造调用 match

## 背景

Story 37.13 r1 落地了 `iphone/scripts/check_no_apiclient_in_features.sh` 作为 ADR-0010 View ↔ ViewModel 解耦边界的 CI gate；r2 review 指出该脚本只匹配 constructor call 形式（`LoadHomeUseCase(` / `HomeRepository(`），漏掉同样常见的违规拼写：property type annotation、parameter type、protocol type、concrete `Default*` impl reference。本 lesson 沉淀"layering guard 写正则的正确粒度"。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | layering guard 漏匹配 property/parameter/protocol/concrete 形式 | P2 (medium) | architecture | fix | `iphone/scripts/check_no_apiclient_in_features.sh:30-33` |

## Lesson 1: layering guard 必须 token-match 而非 constructor-call match

- **Severity**: medium (P2)
- **Category**: architecture（CI gate / 静态分析的有效性）
- **分诊**: fix
- **位置**: `iphone/scripts/check_no_apiclient_in_features.sh:30-33`

### 症状（Symptom）

r1 版本的 `VIOLATION_PATTERN` 用显式列表 + `\(` 后缀绑死：

```bash
VIOLATION_PATTERN='(import APIClient|APIClient\(|APIClientProtocol|HomeRepository\(|...|LoadHomeUseCase\(|...)'
```

这种写法对以下 view-layer 违规拼写**全部 false-negative**：

```swift
// 1. property type annotation —— 不出现 `(`，漏匹配
let useCase: LoadHomeUseCaseProtocol

// 2. parameter type —— 不出现 `(` 在 type 后，漏匹配
func wireUp(repo: HomeRepository)

// 3. concrete Default* implementation —— 不在显式列表里，漏匹配
var fallback: DefaultHomeRepository?

// 4. 任何未来新增的 *UseCase / *Repository 类型 —— 显式列表必须同步维护，漏匹配概率 100%
```

CI gate 对一大类 view-layer regression 失效。

### 根因（Root cause）

写 layering guard regex 时把"违规 = 构造调用"和"违规 = 类型出现在 view 文件"两个判据搞混了。

ADR-0010 的语义是 **view 文件不应**依赖 `*UseCase` / `*Repository` / `APIClient` 任何形式 —— 既不构造、也不持引用、也不当 protocol type 注解。所以 guard 的判据必须是"**类型 token 出现**"而非"**构造调用出现**"。

更深的根因：**显式列表式 regex** 必然漏 future-proofing —— 每加一个 UseCase / Repository 都要同步改脚本，违反 [open-closed]。改用 **PascalCase suffix-based token match**（`\b[A-Z]\w*UseCase\b` 等）让命名约定成为契约，新增类型自动被守护。

### 修复（Fix）

将正则改为 token-level 广泛匹配，并加 comment-line skip 避免 doc 引用误报：

```bash
# r2 加固后
VIOLATION_PATTERN='\b([A-Z][A-Za-z0-9]*UseCase|[A-Z][A-Za-z0-9]*Repository|APIClient)(Protocol)?\b'

# 用 awk 跳过纯注释行（`//` / `///`），保留行号
matches="$(awk '!/^[[:space:]]*\/\//{print NR":"$0}' "$file" \
    | grep -E "$VIOLATION_PATTERN" || true)"
```

覆盖矩阵（自测验证 6/6 命中 + 2/2 注释跳过）：

| 形式 | 例子 | r1 | r2 |
|---|---|---|---|
| import | `import APIClient` | ✅ | ✅ |
| 构造调用 | `LoadHomeUseCase()` | ✅ | ✅ |
| property type | `let x: LoadHomeUseCaseProtocol` | ❌ | ✅ |
| parameter type | `func f(r: HomeRepository)` | ❌ | ✅ |
| concrete Default | `var x: DefaultHomeRepository` | ❌ | ✅ |
| `*Protocol` 后缀 | `APIClientProtocol` | ✅ | ✅ |
| 行注释 `//` | `// see HomeRepository` | (false +) skip | skip |
| doc 注释 `///` | `/// LoadHomeUseCase` | (false +) skip | skip |

跑 `bash iphone/scripts/check_no_apiclient_in_features.sh` 当前实际 view 目录 → ✅ 0 violation（因 view 文件本就干净，所有 UseCase / Repository 引用都在 ViewModels/ 子目录，不在 SCAN_DIRS 里）。`bash iphone/scripts/build.sh --test` → 346/346 通过，无回归。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写 **layering / dependency boundary 的 grep guard 脚本**时，**必须用「类型 token + naming-convention suffix」级正则**（如 `\b[A-Z]\w*UseCase\b`），**禁止用「显式类型列表 + 构造调用 `(` 后缀」**。
>
> **展开**：
> - 判据必须对齐 ADR 语义：如果 ADR 说 "view 不应依赖 X"，guard 必须匹配"X 在 view 文件中以**任何**形式出现"，不只是"X 被构造"
> - 用 PascalCase + suffix 命名约定（`*UseCase` / `*Repository` / `*Protocol`）反向给 guard 当 anchor，让 future 新增类型自动入网
> - 必须加 comment-line skip（`//` / `///`），否则 forward-reference 注释 / doc 字符串会刷大量 false-positive 噪声 → dev 学会忽略 guard → guard 等于零
> - 写完 guard 后**自测**：用临时 fixture 模拟 4 种以上违规拼写（property / parameter / protocol / concrete impl），确认全命中；同时加 1-2 条 comment 行确认正确跳过
> - **反例 1**：`VIOLATION_PATTERN='(LoadHomeUseCase\(|HomeRepository\()'` —— 必须维护显式列表，新增类型必漏，property type 必漏
> - **反例 2**：`VIOLATION_PATTERN='UseCase|Repository'` —— 没有 `\b` 词边界 + 没有大写起始约束，会把 `myUseCaseHelper`、`testRepositoryAdapter` 之类无关 helper 名误报
> - **反例 3**：写完没自测就 commit —— guard 看似 OK 实际只在 review 触发时 false-negative，等于无 guard
> - **正例**：`\b([A-Z][A-Za-z0-9]*UseCase|[A-Z][A-Za-z0-9]*Repository|APIClient)(Protocol)?\b` + awk 跳 `//` —— token-level + suffix-based + comment-skip，三者缺一不可
