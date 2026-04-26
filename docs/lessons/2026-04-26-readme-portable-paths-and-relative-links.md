---
date: 2026-04-26
source_review: codex review of Story 2.10 — file: /tmp/epic-loop-review-2-10-r1.md
story: 2-10-ios-readme-模拟器开发指南
commit: c95b1a6
lesson_count: 2
---

# Review Lessons — 2026-04-26 — onboarding 文档的可移植性 & 跨目录 markdown 相对链接

## 背景

Story 2.10 落地 `iphone/README.md`：Epic 2 收官 onboarding / 模拟器开发指南。codex round 1 review 指出两个文档可靠性 finding：① 命令里嵌入了机器特定路径 `~/fork/catc`，新成员 clone 到任意其他目录 copy-paste 即 fail；② 一批指向 story 文档的相对链接没考虑 README 自己所在目录是 `iphone/`，链接相对解析后落到不存在的 `iphone/2-X-...md`。两者都不影响 build / 测试，但 README 是 onboarding 的第一公里，任何"复制粘贴失败"或"点链接 404"都直接拖慢新成员上手速度。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | onboarding 命令里嵌入 hard-coded 本机 checkout 路径 | medium (P2) | docs | fix | `iphone/README.md:11`, `:343` |
| 2 | 跨目录文档的 markdown 相对链接漏算 README 自身目录 | low (P3) | docs | fix | `iphone/README.md:170`, `:413` |

## Lesson 1: onboarding 命令里嵌入 hard-coded 本机 checkout 路径

- **Severity**: medium (P2)
- **Category**: docs
- **分诊**: fix
- **位置**: `iphone/README.md:11`、`:343`

### 症状

README "快速启动" 段写：

> 3 行命令把 simulator demo 跑起来。**从仓库根目录** `~/fork/catc` 跑（macOS-only；锁定 macOS 14+ / Xcode 26.4+）：

"服务端联调" 代码块写：

```bash
# 1. 仓库根目录跑 server（默认监听 127.0.0.1:8080）
cd ~/fork/catc
bash scripts/build.sh
```

任何新成员 clone 到不是 `~/fork/catc` 的目录（默认 GitHub 命令是 `git clone <url>` clone 到当前目录的子目录，不太可能恰好是这个绝对路径），copy-paste 这段命令的 `cd ~/fork/catc` 就会 `cd: no such file or directory`，第一公里就卡住。

### 根因

写 README 的时候 Claude 在自己的 cwd 操作（`/Users/zhuming/fork/catc`），下意识把这个绝对路径写进了文档，把"我自己的 checkout 路径"等同于"项目正典路径"。但 README 是给所有 clone 这个 repo 的人看的，没人保证 clone 到一样的位置。onboarding 文档的视角应该是 repo-relative，不是 author-machine-relative。

第二次出现（`cd ~/fork/catc`）甚至更冗余 —— 上一段 README 已经声明"从仓库根目录跑"，再写一行 `cd <author 的本机路径>` 既不可移植又重复。

### 修复

两处都改成 repo-relative 表述：

- 行 11：`从仓库根目录 ~/fork/catc 跑` → `从仓库根目录跑（你 clone 到的目录，命令里所有 iphone/... / server/... 路径都相对它）`，明示"仓库根目录 = 读者自己 clone 到的位置"
- 行 343：`cd ~/fork/catc` 整行删掉（前面的注释已经说"仓库根目录跑 server"），把语义换成 `# 假设你已经 cd 到仓库根；如果还没，先 cd 到你 clone 的目录`，让读者自己决定 cd 到哪

不引入 `<repo-root>` 占位符变量（写成 `cd <repo-root>` 读者要自己替换，体验比"前置假设你已经在那"更差）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写**任何** repo-internal onboarding 文档（README / CONTRIBUTING / CI 文档 / 故障排查页）的 shell 命令时，**禁止**写绝对路径形式的 `cd <author 本地 checkout>`，**必须**用相对仓库根的路径或文字表述（"从仓库根目录"/"假设你已经 cd 到仓库根"）。
>
> **展开**：
> - 检查清单：写完文档全文 grep 一遍 `~/`、`/Users/`、`/home/`、`C:\`、author 自己的目录名 —— 命中即必须替换
> - "仓库根目录"作为一种隐含 cwd，在文档头部用一句话锁定（如"以下所有命令从仓库根目录跑"），后面所有命令都相对它写，不要在每段重复 `cd <root>`
> - 如果同一个文档同时引用 `iphone/...` 和 `server/...`（多模块 monorepo），尤其要写明"仓库根目录"语义，否则读者会困惑相对路径起点
> - **反例**：`cd ~/fork/catc && bash scripts/build.sh` —— `~/fork/catc` 是 author 的本机绝对路径，把作者的 cwd 当成项目正典路径暴露出来；正确写法是直接 `bash scripts/build.sh` 并在文档头部说明 cwd
> - **反例**：`cd /home/foo/projects/catc/iphone && bash scripts/build.sh` —— 同上，linux 路径同样是 author-machine-specific
> - **反例**：写 `cd <repo-root>` 占位符 —— 比 author 路径强但仍要求读者替换，弱于"假设你在仓库根目录"的纯文字声明

## Lesson 2: 跨目录文档的 markdown 相对链接漏算 README 自身目录

- **Severity**: low (P3)
- **Category**: docs
- **分诊**: fix
- **位置**: `iphone/README.md:170`、`:413`

### 症状

README 里写：

```markdown
[Story 2.8](2-8-dev-重置-keychain-按钮.md)
[Story 2.10](2-10-ios-readme-模拟器开发指南.md)
```

GitHub / Xcode / 任何 markdown renderer 解析相对链接时，都是相对包含 README 的目录（`iphone/`）解析。两个 link 实际指向 `iphone/2-8-...md` 和 `iphone/2-10-...md`，但两个 story 文件实际位于 `_bmad-output/implementation-artifacts/`，所以这两个链接都是 404。

同一个 README 里其他链接（`../docs/...`、`../_bmad-output/...`、`../server/...`、`../CLAUDE.md`、同目录的 `project.yml` / `docs/CI.md` / `PetApp/...`）全都正确写了 `../` 前缀或保持同目录相对，唯独这两条 story 链接漏了。

### 根因

写 README 时 Claude 的"心理 cwd"是仓库根目录（开发活动的中心），但 markdown 链接的语法 cwd 永远是 README 自己所在目录。两者错位时，"看起来对的"相对路径在渲染时其实是错的。

更具体：story 文件实际在 `_bmad-output/implementation-artifacts/`，从 `iphone/README.md` 出发的正确相对路径是 `../_bmad-output/implementation-artifacts/<file>.md`。Claude 写链接时省略了 `../_bmad-output/implementation-artifacts/` 前缀，直接写文件名，看起来"在同一个项目里所以这样写就行"，但实际 markdown 链接没有"项目根目录"的概念，只有"我自己所在的目录"。

### 修复

两处都加全相对路径前缀：

- 行 170：`[Story 2.8](2-8-dev-重置-keychain-按钮.md)` → `[Story 2.8](../_bmad-output/implementation-artifacts/2-8-dev-重置-keychain-按钮.md)`
- 行 413：`[Story 2.10](2-10-ios-readme-模拟器开发指南.md)` → `[Story 2.10](../_bmad-output/implementation-artifacts/2-10-ios-readme-模拟器开发指南.md)`

并跑了一段 Python 脚本对全文 53 条相对链接做 dry check（`os.path.normpath(os.path.join(readme_dir, target))` 验文件存在），全 53 条 verified 通过。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写非根目录 markdown 文档（`<subdir>/README.md` / `docs/foo.md` 等）的相对链接时，**必须**意识到链接 cwd = 文档自身所在目录，**禁止**直接写文件名而无视目录层级；写完后**必须**对全文相对链接做存在性 dry check（脚本或人眼）才能 commit。
>
> **展开**：
> - 心理模型：markdown 链接是 `dirname(this_file) + target` 解析；任何不在 `dirname(this_file)` 下的目标，链接都必须以 `../` 开头
> - dry check 脚本（保留以下逻辑）：从文档抽取所有形如 `[text](path)` 的相对链接（跳过 `http://` / `https://` / `#` 锚点），对每个 path 做 `os.path.normpath(os.path.join(doc_dir, path))` 然后 `os.path.exists()` 检验，broken 全报出来
> - "都在同一个 repo 里" ≠ "可以省略相对路径前缀" —— repo 是文件系统树，链接是树上节点之间的相对路径
> - 文档跨多个目录引用时（例如 `iphone/README.md` 引 `_bmad-output/`、`docs/`、`server/`、自身目录、`CLAUDE.md` 在仓库根），所有链接都按"从我自己的目录走到目标"写，不要用"从仓库根目录走到目标"
> - **反例**：`iphone/README.md` 里写 `[Story 2.8](2-8-...md)` —— 漏掉 `../_bmad-output/implementation-artifacts/` 前缀，链接相对 `iphone/` 解析后落空
> - **反例**：`docs/foo.md` 里写 `[Architecture](_bmad-output/...)` —— 同样漏掉 `../`
> - **正例**：`iphone/README.md` 里写 `[CLAUDE.md](../CLAUDE.md)`、`[ADR-0002](../_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md)`、`[project.yml](project.yml)`（同目录无 `../`）、`[CI docs](docs/CI.md)`（子目录无 `../`）

## Meta: 本次 review 的宏观教训

两条 finding 都属于"onboarding / 文档的可移植性"同一根因 ——
**写文档时 Claude 把"我自己的 cwd / 心理项目根"等同于"读者眼中的 cwd"**。
但读者读 README 时：
- shell cwd = 自己 clone 到的随机目录（不一定是 `~/fork/catc`）
- markdown link cwd = README 自己所在目录（不是仓库根）

两个 cwd 都不是 author 写文档时的"心理仓库根"。

文档发布前必须做两件事：
1. 全文 grep author 本机绝对路径（`~/` / `/Users/` / `/home/` / 作者用户名）
2. 全文相对链接 dry check（脚本或人眼）

这两件事可以做成一个 5 行的 shell helper（如 `scripts/check-md-links.sh`），未来类似 onboarding 文档 PR 前都跑一下。本次没做成 helper（scope creep），但记录下来作为预防规则的物化候选。
