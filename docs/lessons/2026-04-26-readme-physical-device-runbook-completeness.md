---
date: 2026-04-26
source_review: codex review round 3 — /tmp/epic-loop-review-2-10-r3.md
story: 2-10-ios-readme-模拟器开发指南
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-26 — 真机联调 runbook 必须含 signing 步骤 + config-change-then-restart 序列

## 背景

Story 2.10 round 3 review。`iphone/README.md` 已经过 round 1（可移植性 / 相对链接）和 round 2（runbook 与工具语义对齐）两轮修复，round 3 codex 发现真机联调段（§356-375）仍**不可执行**：缺 code signing 前置步骤 → 真机 `Cmd+R` build fail；改 `bind_host` 不提示重启 → 现有进程仍监听旧地址。两条 finding 同属"真机联调 runbook 完整性"主题，合并到一个 lesson。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 真机联调缺 code signing 前置步骤 | high | docs | fix | `iphone/README.md:356-375` |
| 2 | 改 bind_host 后忘记 restart server 进程 | medium | docs | fix | `iphone/README.md:370-374` |

## Lesson 1: 真机 Xcode build 必须先配 code signing；空 DEVELOPMENT_TEAM 会在 ATS / 网络之前先 fail

- **Severity**: high
- **Category**: docs
- **分诊**: fix
- **位置**: `iphone/README.md:356-375`

### 症状（Symptom）

真机联调 runbook 第 1 步直接让用户改 `PetAppBaseURL` 然后 `Cmd+R`，但 `iphone/project.yml` 里 `DEVELOPMENT_TEAM: ""` 是空字符串。Xcode 没配 team 时真机 build 立刻 fail（"Signing for 'PetApp' requires a development team"），ATS / 网络 / `bind_host` 都根本没机会触发。文档里的所有后续步骤等于白搭。

### 根因（Root cause）

写真机 runbook 时**默认了"signing 是 Xcode 项目本身的事，不属于 README 范围"**。但实际上：

- 仓库 `project.yml` 钦定 `DEVELOPMENT_TEAM: ""`（dev-tools 框架的可移植性约定 —— 不能 hardcode 某个开发者的 team ID 入仓）
- 这意味着每个开发者**首次** clone 仓库后必须在自己 Xcode 里配 personal team，仓库本身没法替代
- README 真机段没说 → 用户卡 build fail，看不出是 signing 问题还是网络问题

简言之：仓库 portable 的代价是开发者要做一次 local override，README 必须显式说明这个补缺步骤。

### 修复（Fix）

在真机联调段开头加"**前置步骤（仅真机首次必跑）—— 配置 code signing**"小节，给出 4 步 Xcode UI 操作（打开项目 → Signing & Capabilities → Auto manage + Team 下拉 → 选 Apple ID）。强调"不会改 project.yml，是 Xcode local override"避免用户 commit 把 team ID 入仓。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 写 iOS 真机联调 runbook 时，**必须先列 code signing 配置步骤**，因为仓库 `project.yml` 通常 portable 不带 team ID，真机 build 在 ATS / 网络之前就会先卡 signing fail。
>
> **展开**：
> - 检查 `project.yml` 的 `DEVELOPMENT_TEAM` 字段：空字符串或缺失 → README 必须显式补"开发者本地配 team"步骤
> - 步骤至少含：① 打开 Xcode 项目 ② 选 target → Signing & Capabilities ③ 勾 Automatically manage signing ④ Team 下拉选 Apple ID
> - 强调 "Xcode local override，不会改 project.yml" —— 防止开发者 commit 把自己 team ID 入仓污染仓库
> - **反例**：写真机 runbook 上来就是 `Cmd+R`，跳过 signing 配置 —— 用户首次跑会卡 build fail，根本到不了网络层
> - **反例**：让用户改 `project.yml` 填 team ID —— 那是个人开发者凭据不应入仓，且会让 dev-tools 框架的 portable 约定形同虚设

## Lesson 2: 改完 server config 必须显式提示重启进程；不存在 hot reload

- **Severity**: medium
- **Category**: docs
- **分诊**: fix
- **位置**: `iphone/README.md:370-374`

### 症状（Symptom）

真机联调 runbook 第 4 步让用户改 `server/configs/local.yaml` 的 `bind_host: 0.0.0.0`，但下一步直接跳"Xcode 选真机 + Cmd+R"。如果 server 进程从前面 simulator 段就已经在跑（监听 `127.0.0.1`），改 yaml 不会触发 hot reload，现有进程仍只接受 loopback 连接。手机连过去 → 显示 `offline`，用户以为是网络问题，实际是 config-change-without-restart 陷阱。

### 根因（Root cause）

写 runbook 时**默认了"改配置文件 = 配置生效"**，没考虑 server 进程的生命周期。Go server（catserver）在启动时一次性读 yaml，运行中不监听文件变化。这是 Go server 的常规设计（详见 ADR / `server/README.md` 配置说明），但 README 真机段没写"重启"明示步骤，普通用户不会自己想到。

### 修复（Fix）

把第 4 步拆成"改 yaml + 重启 server"两个明确动作。重启序列写完整：`pkill catserver` → 重新 build → 重新启动。`> 注意` 段补一句"`bind_host` 改了**必须重启** catserver 进程（无 hot reload）"作冗余强调。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 写涉及 server 配置文件改动的 runbook 时，**必须显式写"重启进程"步骤**，因为本仓库的 catserver 一次性读配置启动后无 hot reload，文件改了进程不知道。
>
> **展开**：
> - 任何 "改 `*.yaml` / `*.toml` / `*.env`" 步骤之后必须紧跟 "重启 server" 子步骤
> - 重启序列要完整：kill 旧进程 + （如改了源码再 build） + 重新启动
> - 在用户已经在前段 runbook 启了 server 的场景下，更要明示"现有进程仍监听旧地址" —— 用户可能默认 yaml 改完就生效
> - **反例**：runbook 写"改 yaml → Cmd+R"，省略重启 → 用户连不上以为是网络问题，实际是 stale process 问题
> - **反例**：只在 `> 注意` 段说一句"需要重启"，但具体步骤序列里没有该动作 —— 用户照步骤走根本不会回头看注意段

---

## Meta: 本次 review 的宏观教训

Story 2.10 是**纯文档** story，连续三轮 review 都暴露 runbook 类文档的同一个反复踩点：**"runbook 看似自洽 / 跑过一次模拟器没问题"≠"覆盖了所有真实场景的前置 / 副作用 / 状态转换"**。

具体到本 round：

- round 1 修可移植性（hardcoded 路径、相对链接断）
- round 2 修工具语义对齐（命令实际行为 vs 文档描述不一致）
- round 3 修"跨场景的前置 / 副作用"（真机首次签名 / config 改完进程不重启）

未来 Claude 写 onboarding / runbook 类文档时，验收标准应该是：**"一个新人按文档从零跑到底，每个分支场景（simulator / 真机首次 / 真机第 N 次 / config 改完）都能跑通，不需要外部知识补缺"**。简单的"我跑通过一次"不够 —— 需要心算覆盖所有路径。
