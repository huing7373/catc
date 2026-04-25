---
date: 2026-04-24
source_review: "file: /tmp/epic-loop-review-1-10-r1.md (codex review --uncommitted, Story 1.10 README)"
story: 1-10-server-readme-本地开发指南
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-24 — README dev-mode 二进制名错配 & settings.local 硬编码 PID 漏到 tracked file

## 背景

Story 1.10 落地 `server/README.md` 本地开发指南。codex review --uncommitted 跑出 2 条 finding：(1) Troubleshooting 表第 6 行给的 cmd.exe workaround 命令引用了 `catserver-dev.exe`，但这个 binary 只有 `bash scripts/build.sh --devtools` 才产出，与 README 同节描述的"runtime 闸门 `BUILD_DEV=true` 配普通 binary 也能启用 dev 模式"矛盾；(2) 上下文里 dev-story sub-agent 调试时往 `.claude/settings.local.json` allowlist 加了 5 条 `kill <PID>` 命令，PID 是当时 shell session 抓的，离开 session 就 stale，且文件被 git 追踪 → 别的机器/下次跑可能误杀同 PID 的无关进程。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | cmd.exe workaround 引用了只有 --devtools 才存在的 binary | medium (P2) | docs | fix | `server/README.md:253` |
| 2 | settings.local allowlist 含 session-local 硬编码 PID | low (P3) | config | fix | `.claude/settings.local.json:112-117` |

## Lesson 1: README 示例命令的 binary 名必须匹配同节正文描述的 build 路径

- **Severity**: medium (P2)
- **Category**: docs
- **分诊**: fix
- **位置**: `server/README.md:253`

### 症状（Symptom）

`server/README.md` "Dev 模式双闸门" 节明确列了两条 OR 闸门：runtime `BUILD_DEV=true`（普通 build 即可）+ 编译期 `--devtools`（产出 `catserver-dev.exe`）。但 Troubleshooting 第 6 行给 cmd.exe 用户的 workaround 是 `set BUILD_DEV=true && .\build\catserver-dev.exe ...`，把 runtime 闸门和带后缀的 binary 名混搭 —— 用户走 quick start（普通 `bash scripts/build.sh`）后照着 troubleshooting 跑，会撞 `file not found`。

### 根因（Root cause）

写 README 例子时把"演示 cmd.exe 怎么设环境变量"和"演示 dev 模式"两件事合写了一行，复用了上一节随手写的 `catserver-dev.exe` 这个名字，没回头校验"这个例子选的是哪条闸门，对应的 binary 路径是什么"。runtime 闸门 + 编译期 binary 名混搭，是 OR 双闸门设计常见的文档面陷阱。

### 修复（Fix）

把第 6 行 cmd.exe workaround 命令改为：

```
set BUILD_DEV=true && .\build\catserver.exe -config server\configs\local.yaml
```

并补一段说明："普通构建即可，runtime 闸门会启用 dev 模式；如需 `catserver-dev.exe` 需先 `bash scripts/build.sh --devtools` 编译期闸门"，让两条闸门的对应关系在示例本地清晰。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **README 给"双闸门 / 多模式 / build flag 影响 binary 名"的示例命令时**，**必须** **逐字校验示例的每个命令片段属于哪条闸门，binary 路径要和那条闸门匹配**。
>
> **展开**：
> - 如果文档同节定义了 OR 语义闸门（runtime env var / 编译期 build tag），任何示例命令必须画出"我现在演示的是哪一条"
> - 给出示例时优先选最朴素的一条（用 quick start build 的默认 binary 名），把"另一条闸门怎么用"作为补充说明
> - **反例**：runtime 闸门示例 `set FLAG=true` 后跟一个只有编译期闸门才产出的 binary 名（`catserver-dev.exe`）；用户照抄会 file-not-found

---

## Lesson 2: tracked allowlist 文件不能含 session-local 临时值（PID / 端口快照 / tmpdir 句柄）

- **Severity**: low (P3)
- **Category**: config
- **分诊**: fix
- **位置**: `.claude/settings.local.json:112-117`

### 症状（Symptom）

Sub-agent 跑 dev-story 时为了停掉本地起的 `catserver` 进程，临时往 `.claude/settings.local.json` 的 Bash allowlist 加了 5 条 `kill 14942` / `kill 14944 11106` / `kill 14996` / `kill 14998`。这些 PID 是当时 shell session 抓的，session 结束就 stale；文件被 git 追踪 → 下次别的机器跑这些 PID 可能恰好属于无关进程，被 allowlist 静默批准 kill 掉。

### 根因（Root cause）

Sub-agent 接到 permission prompt 后直接选择"add to allowlist"，prompt 默认用了**当前调用的精确字符串**（含具体 PID）作为 allowlist 条目，而不是泛化形式。这是 Claude Code allowlist UX 的天然陷阱：精确匹配条目对 session-local 一次性命令是反模式。

### 修复（Fix）

把 5 条带 PID 的条目压成一条泛化的 `kill %1`（bash job control，足够覆盖 `kill $!` / `kill <jobspec>` 类后台进程清理）。同文件 line 101 已有一条按进程名 kill 的 powershell 条目（`Get-Process -Name 'catserver-dev'/'catserver' | Stop-Process`），实际清理流程用那条就够；带 PID 的条目纯粹是冗余冗余。

before：
```json
"Bash(kill 14942)",
"Bash(kill 14944 11106)",
"Bash(kill 14996)",
"Bash(kill 14998)",
```

after：
```json
"Bash(kill %1)",
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **被 Claude Code 提示"add Bash(...) to allowlist"且命令含 session-local 临时值（PID / pid file 路径 / tmpdir 句柄 / 端口快照）时**，**禁止** **直接接受默认精确字符串，必须在脑里把命令泛化成可复用形式（按进程名 / 按 jobspec `%1` / 按已知端口）再批准**。
>
> **展开**：
> - `.claude/settings.local.json` 是 tracked file，写进去的内容跟着 git 走到所有协作者机器；session-local 值在那里只会污染
> - 进程清理首选 `pkill -f <name>` / `Get-Process -Name <name> | Stop-Process` / `kill %1`（jobspec），都不依赖具体 PID
> - 一次性命令（grep / curl / awk 调试）干脆别加 allowlist，让 prompt 下次再问就好；只在确实会重复跑的命令上加 allowlist
> - **反例**：`Bash(kill 14942)` / `Bash(rm /tmp/foo-1747a3.log)` / `Bash(curl http://127.0.0.1:51234/ping)` 这三类带具体 PID/tmpdir/端口快照的条目，全是反模式

---

## Meta: 本次 review 的宏观教训

两条 finding 都属于"复制粘贴 / 接受默认值时丢上下文"——README 例子复用了上一节的 binary 名没回头校验，allowlist 接受了 prompt 的默认精确字符串没泛化。这类坑的共性是"动作本身没出错，但结果和当前上下文（哪条闸门 / 哪个 session）解耦"。未来 Claude 在 (1) 写示例命令 (2) 接受 allowlist prompt 默认值 时，都应该多走一步"这个具体值在新 session / 新机器上还成立吗"的反问。
