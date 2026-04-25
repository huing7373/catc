---
date: 2026-04-25
source_review: codex review --base 51ae73b round 2 (file: /tmp/epic-loop-review-1-10-r2.md)
story: 1-10-server-readme-本地开发指南
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-25 — devtools 双闸门是 OR 语义，SOP 不能写"任一漏放兜得住"

## 背景

Story 1.10 的 `server/README.md` 写了"生产部署 SOP"段落，把 `devtools.IsEnabled()` 的两个触发源说成"双闸门确保任一漏放都不会泄漏 dev 端点"。codex round 2 review 指出：`IsEnabled()` 是 `forceDevEnabled || os.Getenv("BUILD_DEV") == "true"`，**OR 语义**——任一触发源成立即开 dev 模式，根本没有"任一漏放仍能兜住"的语义。当前 README 措辞会误导运维以为有双重保险，实际是"任一闸门误开即漏"。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | "双闸门确保任一漏放都不会泄漏 dev 端点" 措辞错误（OR 语义下任一闸门触发即开） | medium (P2) | docs | fix | `server/README.md:192` |

## Lesson 1: SOP 描述"双闸门 / 双重保险"前必须回头核对触发源是 AND 还是 OR

- **Severity**: medium (P2)
- **Category**: docs
- **分诊**: fix
- **位置**: `server/README.md:192`

### 症状（Symptom）

README 生产部署 SOP 写："双闸门确保任一漏放都不会泄漏 dev 端点"。但 `devtools.IsEnabled()` 实现是 `forceDevEnabled || os.Getenv("BUILD_DEV") == "true"`：build tag 与环境变量任一成立即返回 true。"任一漏放仍能兜住"暗示 AND 语义（必须两个都开才放）；OR 语义下完全相反——任一开即放，运维只要两边任一漏关一个就泄漏。

### 根因（Root cause）

写 SOP 时把"双闸门（防御纵深）"这个**实现层面的术语**和**运维层面的双重保险语义**等价了。devtools 包注释里的"双闸门"指的是**路由注册时机闸门**（`Register` 在 `IsEnabled()==false` 时不挂 `/dev/*`）+ **请求时机闸门**（`DevOnlyMiddleware` 收到请求时再 check 一次）——两道闸门**查的是同一个 `IsEnabled()` 表达式**，抵御的是"挂了路由但运行期 BUILD_DEV 被热切关闭"这种边缘情形（in-depth defense，实现成本为零）。它**不**抵御"build tag 关闭 + BUILD_DEV 误设"——这是**触发源层面**的问题，必须由运维 SOP 双重确认两个触发源都为关闭态来兜住，**不**由代码层面的"双闸门"自动兜底。

写 README 时把这两层（in-depth defense 在闸门层面 vs OR 在触发源层面）混淆了。

### 修复（Fix）

把 README:192 的整段重写：

before（错误）：
```markdown
生产二进制必须 `bash scripts/build.sh`（**不带** `--devtools`）+ 部署环境**禁止**设置 `BUILD_DEV` 环境变量。双闸门确保任一漏放都不会泄漏 dev 端点（详见 ... §防御纵深）。
```

after（准确）：
```markdown
`devtools.IsEnabled()` 的两个触发源是 **OR 语义**——**任一**成立即开 dev 模式：
1. 编译期：`-tags devtools` ...
2. 运行期：环境变量 `BUILD_DEV=true` ...

因此生产部署必须**同时关闭两个触发源**，**不存在**"任一漏放仍能兜住"的双重保险：
- 生产二进制走 `bash scripts/build.sh`（不带 `--devtools`），让 `forceDevEnabled=false`
- 部署环境**禁止**设置 `BUILD_DEV` 环境变量

`devtools.go` 里的"双闸门（防御纵深）"指**路由注册闸门 + 请求时闸门**，两道**查的是同一个 IsEnabled() 表达式**，抵御的是"挂了路由但运行期 BUILD_DEV 被热切关闭"这类边缘情形，**不**抵御"build tag 关 + BUILD_DEV 误设"——后者只能靠运维 SOP 双重确认。
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **README / SOP / 部署文档里写"双闸门 / 双重保险 / 任一漏放仍能兜住"类描述前**，**必须** **回到代码核对配置门控的布尔表达式：是 `&&` (AND) 还是 `||` (OR) ——只有 AND 才能写"任一漏放兜得住"，OR 必须写"任一开即漏"**。
>
> **展开**：
> - "防御纵深 / in-depth defense" 是**实现层面**术语，指同一条件在多个时机各 check 一次（如路由注册时 + 请求时）；它**不**等于运维层面的"双重保险"
> - 多触发源是 OR 时，文档必须明示"任一即开"+"运维必须同时关闭所有触发源"，**不**许写让人误以为多关一道仍兜得住的措辞
> - 如果代码里的多触发源真的需要"漏一道还能兜底"语义（如 `BUILD_DEV=true` 必须**同时**带 `-tags devtools` 才生效），那应该把代码改成 `&&` AND 语义，而不是把文档写成期望的样子糊弄过去
> - 涉及"生产部署不准开"的 feature flag / 调试端点 / 维护开关时，运维 SOP 段落必须列出**所有**触发源（编译期 tag、env var、配置文件 key、远程 flag 服务等），不能只点一个代表
> - **反例 1**：本次 README 把 `||` 关系的两个触发源说成"双闸门兜底"——逻辑学上错位
> - **反例 2**：把代码里的 `cfg.EnableX || os.Getenv("DEV") == "1"` 在 ops doc 里写成"必须两个都开才生效"——读者按这个 SOP 部署反而会泄漏
> - **反例 3**：把 `defense in depth`（多时机重复 check 同一条件）和 `belt-and-suspenders`（多个独立条件全部成立才放）混用——前者保护"同一条件被绕过"，后者才能"任一漏放仍兜住"