---
date: 2026-04-26
source_review: codex review round 4 (/tmp/epic-loop-review-2-10-r4.md)
story: 2-10-ios-readme-模拟器开发指南
commit: 0da147e
lesson_count: 2
---

# Review Lessons — 2026-04-26 — README 命令必须 cover 所有合法网段 + 工具输出格式不能假设固定字符数

## 背景

Story 2-10（iPhone README 模拟器/真机开发指南）round 4 codex review。两条 finding 同属 docs / runbook 类，但都是"作者在自己的环境里验证 OK，对其他环境的合法变体没考虑全"这类思维漏洞——一个假设网段都是 `192.168/16`，另一个假设 git short hash 永远 8 位（实际由 `core.abbrev` 决定，本仓库默认 7）。两条合并归档，强调 README 里出现的 shell 命令 / 工具输出引用都必须是**网段无关 / 长度无关**的写法。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | LAN IP discovery 限于 192.168/16 | P2 / medium | docs | fix | `iphone/README.md:369-370` |
| 2 | "8位 commit" 长度不准确 | P3 / low | docs | fix | `iphone/README.md:352` |

## Lesson 1: README 里的局域网 IP 发现命令必须 cover 所有 RFC1918 私有网段

- **Severity**: P2 / medium
- **Category**: docs
- **分诊**: fix
- **位置**: `iphone/README.md:369-370`

### 症状（Symptom）

真机联调 runbook 第 1 步给的命令：

```bash
ifconfig | grep 'inet 192' | head -1
```

只能匹配 `192.168.x.x` 网段。在公司网 / VPN 常见的 `10.x.x.x` / `172.16-31.x.x` 网络下，命令输出空，开发者无法推进到下一步（改 `PetAppBaseURL`）——尽管 Mac 和真机其实在同一 Wi-Fi 上。

### 根因（Root cause）

写作者在自己的家庭网（默认 `192.168/16`）验证命令 OK，没把命令的"覆盖面"和"我自己环境"分开思考。RFC1918 定义了三个私有网段，家庭路由器最常见的是 `192.168.0.0/16`，但企业网 / 云内网 / VPN 大量用 `10.0.0.0/8` 和 `172.16.0.0/12`。任何 hardcode 网段前缀的命令都会在另外两类环境直接失败，而 README 是**所有环境**新人 onboarding 都要照抄的。

### 修复（Fix）

改成 macOS 标准命令 `ipconfig getifaddr`，按接口取 IP，不关心网段；并给 fallback 链：

```bash
# Before
ifconfig | grep 'inet 192' | head -1

# After
ipconfig getifaddr en0 || ipconfig getifaddr en1
# fallback：ifconfig | grep 'inet ' | grep -v 127.0.0.1
```

`ipconfig getifaddr <iface>` 是 macOS 自带工具，返回指定接口的 IPv4，输出干净（仅 IP，无 `inet` 前缀），对所有合法私有 / 公网 IP 一致工作。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **README / runbook 写"找局域网 IP"或类似网络发现命令** 时，**禁止** **hardcode 网段前缀做 grep 过滤**（如 `grep 'inet 192'`、`grep '10\.'`）。
>
> **展开**：
> - macOS：优先 `ipconfig getifaddr en0` / `en1`（按接口取，输出干净）
> - 如必须用 `ifconfig`：grep `'inet '` + 排除 `127.0.0.1`，**不**指定网段前缀
> - Linux：`ip -4 addr show <iface>` 或 `hostname -I`（同样不假设网段）
> - 写命令前自问"这条命令在 `10.x` / `172.16-31.x` / `192.168.x` 三个 RFC1918 网段是否都能产出有用输出"——任一答案为 No 即不合格
> - **反例**：`ifconfig | grep 'inet 192'`、`ip addr | grep '192.168'`、`netstat -rn | grep '10\.'`——任何把网段写死在 grep 模式里的命令都属于此类

## Lesson 2: README 不能给"工具输出固定字符数"的承诺

- **Severity**: P3 / low
- **Category**: docs
- **分诊**: fix
- **位置**: `iphone/README.md:352`

### 症状（Symptom）

成功标志写"`v<X.Y.Z> · <8位commit>`"，并补一句"8 位 commit 是 server 的 git short hash"。但 server build 注入的实际是 `git rev-parse --short HEAD`，长度由 `core.abbrev` 决定，**默认 7 字符**（本仓库当前就是 7）。健康环境下 footer 显示 `v0.0.0 · abc1234`（7 字符），开发者照 README 字面对照就会误判为联调失败——实际上是 README 说错了。

### 根因（Root cause）

写作者可能见过某个 8 字符的 hash（git 在仓库 commit 数变多 / hash 冲突时会自动加长 abbrev），把那个特定长度泛化成"git short hash 都是 8 位"。但 `git rev-parse --short HEAD` 的输出长度是动态的：由 `core.abbrev` 配置决定，默认值由 git 根据仓库大小算（小仓库 7，大仓库可能更长）。任何对工具输出"固定长度"的承诺都是脆弱契约。

### 修复（Fix）

把 README 里"8 位 commit"改成对长度无承诺的描述，并解释长度来源：

```diff
- v<X.Y.Z> · <8位commit>（成功，X.Y.Z 是 iPhone App 版本号、8 位 commit 是 server 的 git short hash）
+ v<X.Y.Z> · <short-commit>（成功，X.Y.Z 是 iPhone App 版本号；<short-commit> 是 server `git rev-parse --short HEAD` 的输出，长度由 git 决定，本仓库当前为 7 字符，未来可能因 hash 冲突自动加长）
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **README / 文档描述命令行工具的输出格式** 时，**禁止** **承诺固定字符数 / 固定行数 / 固定字段顺序**，除非 man page 明确钦定。
>
> **展开**：
> - `git rev-parse --short` 长度由 `core.abbrev` 决定，**不固定**——用占位符 `<short-commit>` 或 `<git-short-hash>`，不写"7 位"或"8 位"
> - `uuidgen` 每个版本输出格式可能微调（大小写、是否带 `-`）——别承诺
> - `date` 默认 locale 输出会变——always specify format string
> - 写"成功标志"时，**只承诺 schema / 模式**（如 `v<X.Y.Z> · <hash>`），不承诺**长度**
> - 若必须给具体例子，加注"示例长度仅当前仓库取值"+ 解释长度来源
> - **反例**：「应输出 8 位 commit」、「时间戳应为 19 个字符」、「version 必为 3 段点分十进制」（如果工具支持 SemVer 预发布标签，3 段就不准）

---

## Meta: 本次 review 的宏观教训

两条 finding 都属于"作者把自己环境观察到的特定值当成普世常量"。共同的预防策略：**runbook / README 里出现的命令和工具输出引用都要先做"环境无关性自检"**——包括但不限于：

- 网段（RFC1918 三个网段都验过吗）
- 字符数 / 行数（man page 是否钦定）
- 版本号格式（工具升级会不会变 schema）
- 路径分隔符（macOS / Linux / Windows）
- locale-sensitive 输出（date / sort / numfmt 等）

review 是发现这类盲点的最后一道线，但更省事的做法是写命令时主动跑一次 mental "其他环境会怎样"——本质上是 negative testing 的文档版本。
