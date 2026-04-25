# Story 1.10: server README + 本地开发指南

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发 / 新加入团队成员,
I want 一份位于 `server/README.md` 的本地开发指南，把节点 1 已经落地的 build / test / dev-mode / 配置 / 目录结构 / troubleshooting 在一处说清楚,
so that 我打开 `server/` 目录就立即知道怎么跑起来 + 改完代码怎么验，不用反向翻 `CLAUDE.md` / 节点 1 的 9 篇 story 文件 / 6 篇 ADR / 6 篇 lessons.

## 故事定位（节点 1 第十条 = 收官 story）

- **节点 1 进度**：1.1 (ADR-0001) → 1.2 (cmd/server + ping) → 1.3 (request_id/recover/logging 中间件) → 1.4 (/version) → 1.5 (测试基础设施 + sample) → 1.6 (Dev Tools 框架) → 1.7 (重做 build.sh) → 1.8 (AppError + 三层错误映射) → 1.9 (ctx 传播 + ADR-0007) **全部 done**；本 story 是节点 1 **唯一的纯文档 story**，把前 9 条沉淀在一份 README 里
- **epics.md AC 钦定**：epics.md §Story 1.10 已**精确**列出 README 必须包含的 8 个章节（快速启动 / 依赖 / 配置 / migration / 跑测试 / dev mode / 目录结构 / troubleshooting）+ 同步原则（每个 epic 完成时如有命令变化要更新 README）+ "纯文档不写单元测试，但**手动**按 README 走通" 的验收方式
- **下游立即依赖**：
  - **Epic 2**（iOS 脚手架）开新 SwiftUI 工程时，iOS dev 会需要"server 怎么本地跑"的 README 来在模拟器跑 ping → server demo；本 README 是 iOS 团队的入口文档
  - **Epic 3**（节点 1 demo 验收 + tech-debt 登记）的 Story 3.3 文档同步会校验 README 与代码的一致性 —— 本 story 必须把 README 写到能通过 3.3 校验的程度
  - **Epic 4**（Auth + MySQL 接入）落地真 MySQL 连接代码时，本 README 的"依赖 - MySQL"章节会从"本地启动 MySQL 的命令"扩展到"server 配置怎么连 MySQL"；Epic 4 Story 4-2 会反向**修改**本 story 产出的 README（增量演进）
- **范围红线**：本 story **只**写 `server/README.md`（一份文件 + 必要的小段交叉引用）；**不**改任何 Go 代码、**不**改 build.sh、**不**改 docker-compose 文件（节点 1 不引入 docker-compose；MVP 节点 1 不接 MySQL/Redis 真连接，README 只**告知** dev "MySQL/Redis 在 Epic 4/10 才接入，节点 1 跑 server 不需要它们"）

**本 story 不做**（明确范围红线）：

- ❌ **不**新增 `migrations/` 目录或任何 SQL DDL 文件（Epic 4 Story 4-3 才落地 5 张表 migrations）
- ❌ **不**实装 `./build/catserver migrate up/down/status` 子命令（节点 1 没有 migration 二进制；epics.md AC 提到 migration 章节是"未来 Epic 4+ 接入"的占位 / 演进路径，本 story 在 README 里**说明**"migration 子命令 Epic 4 Story 4-3 落地"即可，**不**真写 cobra/flag 子命令）
- ❌ **不**写 `docker-compose.yml`（MVP 节点 1 不依赖 MySQL/Redis；docker-compose 是 Epic 4/10 的事，本 README 只**举例**官方 docker run 命令 + brew/winget install 命令作为参考）
- ❌ **不**改 `CLAUDE.md`（README 是 server/ 子目录文档，CLAUDE.md 是仓库根的 AI 上下文；二者职责正交。CLAUDE.md §Build & Test 已在 Story 1.7 改写为承接说明，无需再改）
- ❌ **不**写英文版 `README.en.md`（项目 communication_language=Chinese；用户 huing7373@gmail.com 单一开发者，无国际化诉求）
- ❌ **不**写 `CONTRIBUTING.md` / `CHANGELOG.md` / `LICENSE`（节点 1 不涉及；属未来 / 不在本 story scope）
- ❌ **不**写"如何写 unit test"教程（已在 ADR-0001 §3.5 + Story 1.5 落地；README 引用即可）
- ❌ **不**重复 ADR-0006 / ADR-0007 的细节（README 引用 ADR 路径 + 一行摘要即可，**不**复制全文）
- ❌ **不**新增 npm / pip / gem / Makefile（节点 1 没引入任何这些工具；保持 `bash scripts/build.sh` 单一命令面）
- ❌ **不**引入 GitHub Actions / GitLab CI YAML 配置（Epic 3 Story 3-3 文档同步阶段才登记；本 README "跑测试"章节**只**写本地命令，不假设 CI 配置）
- ❌ **不**在 README 写 OpenAPI / Swagger 文档生成方式（旧架构残留，已被 Story 1.7 物理清理；新架构以 `docs/宠物互动App_V1接口设计.md` 为唯一 API 权威）

## Acceptance Criteria

**AC1 — 文件创建于正确位置**

新建 `server/README.md`（**注意**：是 `server/` 目录下、与 `go.mod` / `cmd/` 平级；**不**是仓库根的 `README.md`）。

- 文件名严格 `README.md`（首字母大写、`.md` 后缀；GitHub / VS Code / IntelliJ Go 插件都按此约定渲染目录入口文档）
- 编码 UTF-8 + LF 行尾（与项目其他 .md 一致；Windows 上 git bash / VS Code 默认行为）
- 顶部 H1：`# Cat Server`（或等价 "宠物互动 App Server"；用 `Cat Server` 与 module path `github.com/huing/cat/server` 对齐）
- H1 之后**第一行**写一句 ≤80 字符的 tagline："宠物互动 App 后端（Go + Gin + MySQL + Redis 单体）。本目录是 server/ 工程；客户端见 ../ios/，watchOS 暂不考虑（见 CLAUDE.md）。"

**AC2 — 章节结构（8 个必含章节 + 顺序约束）**

README 必须按以下**精确顺序**包含 8 个 H2 章节（标题文字可微调，关键字必须在）：

| # | 标题 | 必含内容 | 锚点（kebab-case） |
|---|---|---|---|
| 1 | `## 快速启动` | 一段 ≤5 行的"3 行命令跑起来"路径 | `#快速启动` |
| 2 | `## 依赖` | 节点 1 本地依赖（Go 1.25+，git，bash）+ MVP 演进依赖（MySQL 8.0 / Redis 6+ 在 Epic 4/10 接入） | `#依赖` |
| 3 | `## 配置` | `configs/local.yaml` 各字段说明 + 环境变量覆盖（`CAT_HTTP_PORT` / `CAT_LOG_LEVEL`）+ `bind_host` 防火墙提示 | `#配置` |
| 4 | `## 跑测试` | `bash scripts/build.sh --test` / `--race --test` / `--test --coverage` / `--integration` 命令 + 互斥规则 + Windows TSAN 偏离说明 | `#跑测试` |
| 5 | `## Dev mode` | `BUILD_DEV=true` 环境变量 + `bash scripts/build.sh --devtools` build tag 双闸门 + 当前可用 dev 端点（`/dev/ping-dev`）+ 未来 dev 端点列表（占位） | `#dev-mode` |
| 6 | `## 目录结构` | 复制 `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 的 ASCII 树 + 标注**当前节点 1 已实装** vs **Epic X 才落地** | `#目录结构` |
| 7 | `## Troubleshooting` | 至少 5 个常见坑：端口占用 / Windows 防火墙弹窗 / `-race` TSAN 失败 / `/version` 返回 unknown / 配置路径找不到 | `#troubleshooting` |
| 8 | `## 工程纪律` | ctx 必传 / 错误三层映射 / `c.Error(apperror.Wrap(...))` 单一 envelope 入口 等约束的**指针**（一行 + 链接到 ADR / 设计文档），**不**复制全文 | `#工程纪律` |

**关键约束**：

- 章节顺序固定（dev 视线流：先跑起来 → 知道依赖 → 改配置 → 验改动 → 开 dev → 找代码位置 → 出问题怎么办 → 必须遵守的纪律）
- 每个章节内**必须**至少有 1 个可复制粘贴的命令块（` ```bash ... ``` ` fenced code block）
- 命令路径**全部**用相对路径（如 `bash scripts/build.sh` / `./build/catserver` / `server/configs/local.yaml`），**不**用绝对路径（Windows `C:\fork\cat` 是单机环境）
- README 写好后跑一遍 `markdown link checker` 心智（不需要真跑工具，但**必须**保证所有 `[xxx](path)` 链接路径在仓库内真实存在；ADR / lesson / 设计文档 / story 文件路径都要核对）

**AC3 — `## 快速启动` 章节内容（最小可执行路径）**

至少包含以下命令块（按顺序）：

```bash
# 第一次：编译 + 跑
bash scripts/build.sh
./build/catserver -config server/configs/local.yaml

# 验证（另开一个 shell）
curl http://127.0.0.1:8080/ping       # → {"code":0,"message":"pong",...}
curl http://127.0.0.1:8080/version    # → {"code":0,"data":{"commit":"<short hash>","builtAt":"..."}}
```

**关键约束**：

- 端口 `8080` 必须与 `server/configs/local.yaml` 的 `http_port: 8080` 默认值一致（**不**写 `:8090` / `:18090` 这类 ad-hoc 测试端口）
- `127.0.0.1` 必须与 `bind_host: 127.0.0.1` 默认值一致（**不**写 `localhost`，Windows 上 IPv6 hosts 解析慢；**不**写 `0.0.0.0`，那是生产配置）
- `-config server/configs/local.yaml` 显式传 path（虽然 `LocateDefault()` 能自动找，但 README 显式写出最稳；见 Story 1.2 + lesson `2026-04-24-config-path-and-bind-banner.md` Lesson 1）
- **不**包含 `go run ./cmd/server`（绕过 build.sh / `-ldflags` 注入 → `/version` 返回 `"unknown"`；见 Story 1.7 Dev Notes §1）；如果 README 提到 `go run` 必须**警告**它会让 `/version` 失真
- 命令块必须真能复制粘贴跑通（dev 验证：本地 `rm -rf build && bash scripts/build.sh && ./build/catserver.exe -config server/configs/local.yaml &` 能起 server）

**AC4 — `## 依赖` 章节内容**

| 子章节 | 必含 |
|---|---|
| **当前节点 1 依赖** | Go 1.25+（`go version` 验证）；git（`git --version`）；bash（Windows 用 Git Bash / WSL；macOS / Linux 自带）；**无**其它运行期依赖 |
| **MVP 演进依赖（Epic 4/10 接入）** | MySQL 8.0：本地推荐 `docker run -d --name cat-mysql -e MYSQL_ROOT_PASSWORD=catdev -p 3306:3306 mysql:8.0`，或 `brew install mysql@8.0` (macOS) / `winget install Oracle.MySQL` (Windows)；Redis 6+：`docker run -d --name cat-redis -p 6379:6379 redis:6-alpine`，或 `brew install redis` / `winget install Redis.Redis-x64`；**节点 1 跑 server 不需要这两个**，`server/configs/local.yaml` 也没有 MySQL/Redis 配置项 |
| **测试依赖** | `bash scripts/build.sh --test` 自动拉取 `go.mod` 里的测试依赖（testify / sqlmock / miniredis / yaml.v3 等）；首次 `go mod download` 走 `GOPROXY` |

**关键约束**：

- 章节明确指出"节点 1 不需要 MySQL/Redis"，避免新 dev 误以为现在就要装 docker / 装 brew 一堆服务
- docker run 命令的端口（3306 / 6379）是 MySQL/Redis 标准端口；本地多实例冲突时 dev 自行改 `-p 13306:3306` 等，README 不需要列穷尽变体
- **不**列具体版本号 patch level（如 mysql:8.0.35 / redis:6.2.7）—— 容易过时，写 major.minor 即可（mysql:8.0 / redis:6-alpine）
- Go 1.25+ 是 `server/go.mod` 第一行 `go 1.25.0` 的下限要求；如果 dev 装 1.24 跑 `bash scripts/build.sh` 会报 `module requires go >= 1.25.0`

**AC5 — `## 配置` 章节内容**

至少包含：

| 段落 | 必含 |
|---|---|
| **配置文件位置** | `server/configs/local.yaml`（Story 1.2 决策：local / dev / staging / prod 四档，节点 1 只有 local；Epic 4+ 增 dev/staging/prod）。指向 ADR-0001 §6 配置文件结构决策 |
| **字段说明表** | 表格列出当前 `local.yaml` 全部字段：`server.bind_host` / `server.http_port` / `server.read_timeout_sec` / `server.write_timeout_sec` / `log.level`，每个字段写：默认值 / 类型 / 含义 / 何时改 |
| **环境变量覆盖** | `CAT_HTTP_PORT` 覆盖 `server.http_port`（取 ASCII 整数；非法值启动报错）；`CAT_LOG_LEVEL` 覆盖 `log.level`（debug/info/warn/error）；CI / 临时 demo 用得上；命令例：`CAT_HTTP_PORT=18090 ./build/catserver -config server/configs/local.yaml` |
| **配置路径解析** | 不传 `-config` 时 `LocateDefault()` 按 CWD-relative 顺序查找 `server/configs/local.yaml` → `configs/local.yaml`（见 Story 1.2 + lesson `2026-04-24-config-path-and-bind-banner.md` Lesson 1）。生产部署应**显式**传 `-config /etc/cat/prod.yaml` 避免 CWD 漂移 |
| **bind_host 默认值** | `bind_host: 127.0.0.1` —— 本地 loopback 防 Windows Defender 防火墙弹窗；生产部署改 `0.0.0.0` 或删此行 |

**关键约束**：

- 字段说明表的"何时改"列**必须**给出具体场景（如 "`http_port`: 默认 8080；端口冲突时改其他可用端口"），不写"按需修改"等空话
- 环境变量章节必须给出**完整命令例**（变量赋值 + 二进制路径 + `-config` 参数），不只写 "`CAT_HTTP_PORT=xxx`"片段
- 引用 lesson 路径必须真实存在（`docs/lessons/2026-04-24-config-path-and-bind-banner.md`）
- **不**写"如何添加新的配置字段"教程（属未来扩展；本 README 只写当前已有字段）

**AC6 — `## 跑测试` 章节内容**

至少包含：

| 子段落 | 必含 |
|---|---|
| **基本命令矩阵** | 5 行命令 + 一句话用途：<br>`bash scripts/build.sh --test`（单元测试）<br>`bash scripts/build.sh --race --test`（race detector，**Windows 本机已知 TSAN 偏离**，归 CI Linux）<br>`bash scripts/build.sh --test --coverage`（覆盖率，产出 `build/coverage.out`，可选 `go tool cover -html=build/coverage.out`）<br>`bash scripts/build.sh --integration`（集成测试 `-tags=integration`；节点 1 暂无 integration 测试，Epic 4 Story 4-7 首次落地）<br>`bash scripts/build.sh --devtools --test`（dev tag 测试，对应 `-tags devtools` 路径） |
| **互斥规则** | `--coverage` 必须配 `--test`；`--devtools` 与 `--integration` 互斥；其余正交（见 Story 1.7 AC8） |
| **Windows TSAN 偏离** | 一句话："Windows 本机跑 `--race` 会因 ThreadSanitizer 内存分配限制（`error code: 87`）失败；归 CI Linux runner 执行；本机想 race 跑可用 WSL2"。引用 Story 1.5 AC7 + Story 1.7 T5.3 + Story 1.9 Debug Log 三处共识 |
| **测试策略指针** | 一句话：testify + sqlmock + miniredis + slogtest 四件套；详见 `_bmad-output/implementation-artifacts/decisions/0001-test-stack.md`（**不**复制 ADR 内容，给指针即可） |
| **单测 vs 集成测** | 节点 1 全部是单测（含 sample / middleware / config / logger / metrics 等共 12+ 个包）；集成测试（dockertest 起真 MySQL / miniredis）从 Epic 4 Story 4-7 开始引入 |

**关键约束**：

- 命令必须能复制粘贴跑（dev 复盘：`bash scripts/build.sh --test` 在节点 1 状态下应 12+ 包全绿，`OK: all tests passed`）
- 不用 `go test` 直接命令做"主力示范"；`bash scripts/build.sh --test` 是**唯一**主示范（保证 `go vet` 先跑 + 错误统一处理 + `-count=1` 禁缓存）；`go test` 命令只在"我只想跑某一个包"边角场景写一次 `go test ./internal/service/sample/`
- 引用 ADR / story 文件路径必须**精确到 § 章节**（如 `decisions/0001-test-stack.md#3.5`、不写 `decisions/0001-test-stack.md`）
- **不**写覆盖率阈值（如"覆盖率必须 ≥80%"）—— 节点 1 没钦定阈值；Epic 3 Story 3-3 才登记 tech-debt

**AC7 — `## Dev mode` 章节内容**

至少包含：

| 子段落 | 必含 |
|---|---|
| **启用方式** | "OR 语义双闸门"：运行期 `BUILD_DEV=true`（严格字面，不接受 `1`/`yes`/`TRUE`）OR 编译期 `bash scripts/build.sh --devtools`（产出 `build/catserver-dev[.exe]` + `-tags devtools`）。任一启用即 dev 模式 |
| **启动 banner** | "启用后 server 启动日志含两条 `WARN: DEV MODE ENABLED - DO NOT USE IN PRODUCTION`（main.go + devtools.Register 各一条）；生产二进制**不应**看到这条日志" |
| **当前 dev 端点** | 节点 1 唯一 dev 端点：`GET /dev/ping-dev` → `{"code":0,"data":{"mode":"dev"}}`（见 Story 1.6 AC8）。命令例：`curl http://127.0.0.1:8080/dev/ping-dev` |
| **未来 dev 端点（占位）** | 列表说明节点 1 仅落地框架，业务 dev 端点由各 Epic 扩展：<br>- `POST /dev/grant-steps`（Epic 7 Story 7-5）<br>- `POST /dev/force-unlock-chest`（Epic 20 Story 20-7）<br>- `POST /dev/grant-cosmetic-batch`（Epic 20 Story 20-8）<br>**不**罗列 epics.md 全部未来端点细节，给指针即可 |
| **生产部署 SOP** | 一段警告："生产二进制必须 `bash scripts/build.sh`（**不带** `--devtools`）+ 部署环境**禁止**设置 `BUILD_DEV` 环境变量；双闸门确保任一漏放都不会泄漏 dev 端点（见 Story 1.6 §防御纵深）" |

**关键约束**：

- `BUILD_DEV` 字面值要求 `=true`，**不**接受其他真值串 —— 这是 Story 1.6 钦定的严格语义；README 必须明确写出，否则 dev 误用 `BUILD_DEV=1` 会以为 dev 模式启用了其实没启用
- 引用路径精确到 story / AC（如 `1-6-dev-tools-框架.md#AC8`）
- 未来 dev 端点列表用 markdown 列表 + 一行说明，**不**写完整 request/response 体（属未来 story 自己的事）

**AC8 — `## 目录结构` 章节内容**

复制 `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 的 ASCII 目录树（精简到 server/ 子树即可，不含仓库根 `ios/` / `watch/`），并**用注释标注每个子目录的当前实装状态**：

```
server/
├─ cmd/
│  └─ server/main.go          # ✅ 节点 1 已实装（Story 1.2）
├─ configs/
│  └─ local.yaml              # ✅ 节点 1（local 唯一一档；dev/staging/prod 见 Epic 4+）
├─ migrations/                # 🚧 Epic 4 Story 4-3 落地（节点 1 暂无此目录）
├─ internal/
│  ├─ app/
│  │  ├─ bootstrap/           # ✅ 节点 1（router / server lifecycle）
│  │  ├─ http/
│  │  │  ├─ middleware/       # ✅ 节点 1（request_id/logging/recover/error_mapping）
│  │  │  ├─ handler/          # ✅ 节点 1（ping / version；业务 handler 各 Epic 加）
│  │  │  └─ devtools/         # ✅ 节点 1（Dev Tools 框架 Story 1.6）
│  │  └─ ws/                  # 🚧 Epic 10 Story 10-3 落地
│  ├─ domain/                 # 🚧 Epic 4+ 各 domain 子包（auth/user/pet/...）
│  ├─ service/                # ✅ 节点 1 sample 模板（Story 1.5）；业务 service 各 Epic 加
│  ├─ repo/
│  │  ├─ mysql/               # 🚧 Epic 4 Story 4-2 落地
│  │  ├─ redis/               # 🚧 Epic 10 Story 10-2 落地
│  │  └─ tx/                  # 🚧 Epic 4 落地（txManager.WithTx，见 ADR-0007 §2.4）
│  ├─ infra/
│  │  ├─ config/              # ✅ 节点 1（YAML 加载 + 环境变量覆盖）
│  │  ├─ logger/              # ✅ 节点 1（slog JSONHandler）
│  │  ├─ metrics/             # ✅ 节点 1（Prometheus /metrics）
│  │  ├─ db/                  # 🚧 Epic 4 Story 4-2
│  │  ├─ redis/               # 🚧 Epic 10 Story 10-2
│  │  ├─ clock/               # 🚧 Epic 4 落地（Clock interface for testability）
│  │  └─ idgen/               # 🚧 Epic 4 落地
│  ├─ pkg/
│  │  ├─ errors/              # ✅ 节点 1（AppError + 三层映射 Story 1.8）
│  │  ├─ response/            # ✅ 节点 1（统一 envelope）
│  │  ├─ testing/             # ✅ 节点 1（slogtest + helpers Story 1.5）
│  │  └─ auth/                # 🚧 Epic 4 落地（token util）
│  ├─ buildinfo/              # ✅ 节点 1（Story 1.4 ldflags 注入路径）
│  └─ service/sample/         # ✅ 节点 1（service 模板 + ctx cancel 测试 Story 1.5/1.9）
├─ go.mod                     # ✅ 节点 1
└─ go.sum                     # ✅ 节点 1
```

**关键约束**：

- 用 `✅` / `🚧` 区分"已实装" / "未来 Epic 落地"，新 dev 一眼看出节点 1 现状
- 每个 🚧 标记**必须**指向具体 Epic / Story（如 "Epic 10 Story 10-2 落地"），不写"未来落地"等空话
- 树结构层级与 `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 保持一致（不漏目录、不混层级）
- 标注**不**含 epics.md 全文细节；给指针 + 一行说明

**AC9 — `## Troubleshooting` 章节内容（≥5 个常见坑）**

至少包含以下 5 个坑（每条结构：症状 → 原因 → 解决）：

| # | 症状 | 原因 / 解决 |
|---|---|---|
| 1 | `bash scripts/build.sh` 报 `bind: address already in use` | 8080 端口已被其他进程占用。解决：`lsof -i :8080`（macOS/Linux）/ `netstat -ano \| findstr 8080`（Windows）找占用进程；或用 `CAT_HTTP_PORT=18090 ./build/catserver ...` 改端口 |
| 2 | Windows 启动 server 时弹"Windows Defender Firewall - 允许专用网络访问" | binary 重编译产生新 hash，防火墙不识别。解决：维持 `bind_host: 127.0.0.1`（local.yaml 默认；loopback 免防火墙）；**不**改 `0.0.0.0` 除非真要外部访问。见 lesson `2026-04-24-config-path-and-bind-banner.md` Lesson 1 |
| 3 | `bash scripts/build.sh --race --test` 报 `ThreadSanitizer failed to allocate ... error code: 87` | Windows TSAN 内存分配限制（OS 级，非代码 bug）。解决：归 CI Linux runner 跑；本机想跑改用 WSL2 / 真 Linux 机器。见 Story 1.5 AC7 / Story 1.7 T5.3 / Story 1.9 Debug Log |
| 4 | `curl /version` 返回 `{"commit":"unknown","builtAt":"unknown"}` | 没用 `bash scripts/build.sh` 编译（`-ldflags` 没注入 buildinfo），或用了 `go run ./cmd/server`（同样不注入）。解决：用 `bash scripts/build.sh && ./build/catserver -config server/configs/local.yaml`。见 Story 1.4 §陷阱 #1 + Story 1.7 Dev Notes §1 |
| 5 | server 启动报 `config file not found: ...` | `-config` 路径错或 CWD 漂移让 `LocateDefault()` 找不到。解决：从仓库根（`C:\fork\cat`）跑命令；或显式传 `-config server/configs/local.yaml`。见 lesson `2026-04-24-config-path-and-bind-banner.md` Lesson 1 |

**关键约束**：

- 每条都给**可执行**解决命令（不写"检查端口" → 写 `lsof -i :8080`）
- 引用 lesson / story 路径必须真实存在（README 写完后人工 grep 一遍 `docs/lessons/` 与 `_bmad-output/implementation-artifacts/`）
- **不**写"reboot 解决"这种 Windows 文化糟粕；troubleshooting 必须可解释 + 可重现
- 至少 5 条；超过 7 条就**只挑节点 1 实战遇过的**（lessons 目录 6 条全部都是 `/fix-review` 沉淀的真实坑，本 story README 引用其中 ≥3 条对应 lesson）

**AC10 — `## 工程纪律` 章节内容（指针型，**不**复制全文）**

写成短列表，每条一行 + 链接到 ADR / 设计文档 / story / lesson：

- **节点顺序不可乱跳**：见 [`docs/宠物互动App_MVP节点规划与里程碑.md`](../docs/宠物互动App_MVP节点规划与里程碑.md) §3 / §5
- **写代码前必读**：[`CLAUDE.md`](../CLAUDE.md) + [`MEMORY.md`](../MEMORY.md) + 当前节点对应的设计文档
- **测试栈**：testify / sqlmock / miniredis / slogtest，见 [ADR-0001](../_bmad-output/implementation-artifacts/decisions/0001-test-stack.md) §3
- **错误处理**：AppError + 三层映射（repo→service→handler），见 [ADR-0006](../_bmad-output/implementation-artifacts/decisions/0006-error-handling.md)
- **ctx 必传**：service / repo 第一参数 `ctx context.Context`；handler 用 `c.Request.Context()`；repo 用 `*WithContext`；tx fn 用 `txCtx`，见 [ADR-0007](../_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md)
- **error envelope 单一生产者**：所有 envelope 必须经 ErrorMappingMiddleware，业务代码用 `c.Error(apperror.Wrap(...))` 而非 `response.Error(c, ...)`，见 lesson [`2026-04-24-error-envelope-single-producer.md`](../docs/lessons/2026-04-24-error-envelope-single-producer.md)
- **资产事务**：开箱 / 合成 / 穿戴 / 加房 / 游客初始化必须包 MySQL 事务（节点 1 暂无落地，见 [`docs/宠物互动App_数据库设计.md`](../docs/宠物互动App_数据库设计.md) §8）
- **幂等键**：`/chest/open` 和 `/compose/upgrade` 用 `idempotencyKey` 存 Redis（节点 1 暂无落地）
- **配置路径**：CWD-relative，启动时显式传 `-config`，见 lesson [`2026-04-24-config-path-and-bind-banner.md`](../docs/lessons/2026-04-24-config-path-and-bind-banner.md) Lesson 1
- **slog 字段惯例**：`error_code` / `ctx_done` / `user_id` 缺省即"负向信号"省略字段，**不**显式写 false / 空串

**关键约束**：

- 每条 **必须** 一行 + 一个相对路径链接（`../` 起步因为 README 在 `server/` 而 docs 在仓库根）
- **不**复制 ADR / lesson 的论述细节；给一句话锚点 + 链接
- 顺序按"开发 lifecycle"：上手前要懂的（节点顺序 / CLAUDE.md）→ 写代码时要遵循的（测试 / 错误 / ctx）→ 写业务时要考虑的（事务 / 幂等 / 配置）→ 写日志时要留意的（slog 惯例）

**AC11 — Sprint Status + 同步原则**

- `_bmad-output/implementation-artifacts/sprint-status.yaml`：`1-10-server-readme-本地开发指南` 状态 `backlog → ready-for-dev`（本 SM 步骤完成时）→ 后续 dev 推进 `→ in-progress → review → done`
- README 文件结尾**应**追加一段"## 维护说明"段落（可选 H2，不计入 AC2 的 8 章节强制项），写明"每个 Epic 完成时如有命令 / 配置 / 流程变化，对齐 `epics.md` Story X.3 文档同步要更新本 README"（epics.md §Story 1.10 原文要求）—— 让未来 Epic 3/6/9/13/16... 等"文档同步" story 知道要回头改本 README

**AC12 — 手动验证（epics.md 钦定方式）**

epics.md §Story 1.10 明示："**不需要单元测试**（纯文档）—— 但**手动验证**：按 README 步骤一遍走通，确保命令 100% 可执行"。

Dev **必须**亲手走以下流程（在 Completion Notes 贴 stdout 摘要）：

| # | 命令（按 README 复制粘贴） | 预期 |
|---|---|---|
| 1 | `bash scripts/build.sh` | exit 0；`build/catserver[.exe]` 存在 |
| 2 | `./build/catserver -config server/configs/local.yaml &`（后台启动）| 启动日志含 `config loaded` + `http_port=8080` |
| 3 | `curl http://127.0.0.1:8080/ping` | 返回 `{"code":0,"message":"pong",...}` |
| 4 | `curl http://127.0.0.1:8080/version` | 返回 `commit` 是 short hash（**非** `"unknown"`），`builtAt` 是 ISO 8601 UTC |
| 5 | `bash scripts/build.sh --test` | 全绿 |
| 6 | `bash scripts/build.sh --devtools` | 产出 `build/catserver-dev[.exe]`；启动日志含两条 `DEV MODE ENABLED` WARN |
| 7 | `curl http://127.0.0.1:8080/dev/ping-dev`（用 `--devtools` 产出的 binary）| 返回 `{"code":0,"data":{"mode":"dev"}}` |

**关键约束**：

- Dev 必须按 README 命令**一字不差复制**（不能加自己的小修小改）—— 这是验 README 准确性的唯一办法
- 任一命令失败 → 改 README 而非改代码（除非发现 README 描述的代码行为与实际代码不一致；那是 README bug 不是代码 bug）
- 验证完后 stdout 摘要贴到 Dev Agent Record §Completion Notes List

## Tasks / Subtasks

- [x] **T1** — 写 `server/README.md` 主体（AC1 / AC2 / AC11）
  - [x] T1.1 创建文件 `server/README.md`，UTF-8 + LF；H1 `# Cat Server` + tagline
  - [x] T1.2 按 AC2 顺序写 8 个 H2 章节骨架（先放空骨架，确认顺序无误再填内容）
  - [x] T1.3 末尾加 `## 维护说明` 同步原则段（AC11）

- [x] **T2** — 填充 `## 快速启动` + `## 依赖`（AC3 / AC4）
  - [x] T2.1 快速启动：3 行命令（build / run / curl ping+version），端口 8080 + IP 127.0.0.1 严格对齐 local.yaml
  - [x] T2.2 依赖：当前节点 1 / MVP 演进（MySQL/Redis）/ 测试依赖 三段；docker run + brew install + winget install 命令
  - [x] T2.3 警告 `go run ./cmd/server` 会让 `/version=unknown`（如有提及）

- [x] **T3** — 填充 `## 配置`（AC5）
  - [x] T3.1 配置文件位置 + 4 档环境（local 已落地；dev/staging/prod Epic 4+）
  - [x] T3.2 字段说明表（5 个字段；每个含默认值 / 类型 / 含义 / 何时改）
  - [x] T3.3 环境变量章节（CAT_HTTP_PORT / CAT_LOG_LEVEL）+ 完整命令例
  - [x] T3.4 配置路径解析章节 + bind_host 防火墙提示 + 引用 lesson 1

- [x] **T4** — 填充 `## 跑测试`（AC6）
  - [x] T4.1 5 行命令矩阵 + 一句话用途
  - [x] T4.2 互斥规则 + Windows TSAN 偏离段
  - [x] T4.3 测试策略指针 + ADR-0001 §3.5 链接
  - [x] T4.4 单测 vs 集成测分界（Epic 4 Story 4-7 引入）

- [x] **T5** — 填充 `## Dev mode`（AC7）
  - [x] T5.1 启用方式 + 双闸门（BUILD_DEV / `--devtools`）
  - [x] T5.2 启动 banner 描述 + 当前 dev 端点 `/dev/ping-dev`
  - [x] T5.3 未来 dev 端点占位列表（grant-steps / force-unlock-chest / grant-cosmetic-batch）
  - [x] T5.4 生产部署 SOP 警告段

- [x] **T6** — 填充 `## 目录结构`（AC8）
  - [x] T6.1 复制 `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 的 server/ 子树（精简版）
  - [x] T6.2 每行加 `# ✅ 节点 1...` / `# 🚧 Epic X Story X-Y...` 注释
  - [x] T6.3 校对：本 story 跑前 `ls server/internal/` 确认现状与树注释一致

- [x] **T7** — 填充 `## Troubleshooting`（AC9）
  - [x] T7.1 至少 5 条坑（端口占用 / 防火墙 / TSAN / version=unknown / 配置路径；额外加第 6 条 cmd.exe）
  - [x] T7.2 每条结构：症状 → 原因 → 解决（含具体命令）
  - [x] T7.3 引用 ≥3 条 lesson（config-path-and-bind-banner 在 #2 / #5 各引用一次；其余 lesson 在 §工程纪律 引用，路径全部 `ls` 核实存在）

- [x] **T8** — 填充 `## 工程纪律`（AC10）
  - [x] T8.1 短列表 11 条；每条一行 + 一个 `../` 相对路径链接
  - [x] T8.2 验证所有链接路径在仓库内真实存在（已 ls 核对全部 20 个链接）

- [x] **T9** — 手动验证（AC12）
  - [x] T9.1 按 README §快速启动 复制粘贴跑：`bash scripts/build.sh` exit 0
  - [x] T9.2 启动 binary + curl ping/version；记录 commit/builtAt 实际值
  - [x] T9.3 `bash scripts/build.sh --test` 全绿
  - [x] T9.4 `bash scripts/build.sh --devtools` + 启 dev binary + curl `/dev/ping-dev`
  - [x] T9.5 stdout 摘要贴到 Dev Agent Record

- [x] **T10** — 收尾
  - [x] T10.1 Markdown 渲染检查（README 内表格 / fenced code / 标题层级人工目检）
  - [x] T10.2 链接巡检（Grep `\]\(\.\./[^)]+\)` 提取所有相对链接，逐个 `ls` 确认）
  - [x] T10.3 Completion Notes 补全 + File List 填充
  - [x] T10.4 状态流转 `ready-for-dev → in-progress → review`
  - [x] T10.5 sprint-status.yaml 同步 1-10 状态 + last_updated 时间戳

## Dev Notes

### 项目关键约束（必读，勿绕过）

1. **README 在 `server/` 子目录**：不是仓库根。理由：
   - 仓库根的 `README.md`（如有）应描述"本仓库三端：server/ ios/ watch/"的全局视图（节点 1 阶段不存在仓库根 README，本 story 也**不**新建仓库根 README）
   - `server/` 是独立 Go module（`go.mod` / `go.sum` 在此），子目录 README 是 Go 社区惯例（GitHub 渲染 module 子目录时优先显示该目录的 README）
   - VS Code 打开 `server/` 文件夹时第一眼看到 README，不需要返回仓库根

2. **不写英文版**：单一开发者 + 中文 communication_language；epics.md / planning-artifacts / lessons / ADR 全是中文。英文版后期国际化时单独 PR，不在本 story scope

3. **链接路径用相对路径**：README 在 `server/`，引用 `docs/` / `_bmad-output/` / `CLAUDE.md` 必须 `../docs/...` / `../_bmad-output/...` / `../CLAUDE.md`。GitHub 渲染按相对路径走，绝对路径 `/docs/...` 在 git 检出仓库根名变化时会断（如 fork 改名）

4. **commands 章节引用现状要新**：本 story 写 README 的时候，`scripts/build.sh` 已是 Story 1.7 重写版（5 开关），`local.yaml` 已是 Story 1.2 + lesson backfill 版（5 字段）；`/dev/ping-dev` 是 Story 1.6 落地的唯一 dev 端点；`/version` ldflags 注入是 Story 1.4 + 1.7 路径。**写 README 前**必须 `cat scripts/build.sh` / `cat server/configs/local.yaml` / `ls server/internal/app/http/devtools/` 确认现状与 README 描述一致 —— 否则会出"声明 vs 现实"的新型 lesson（见 lesson `2026-04-24-config-path-and-bind-banner.md` Lesson 2）

5. **不复制 ADR / lesson 全文**：README 给指针 + 一行摘要，不复制论述。理由：
   - 复制等于双源 → 改一份漏改另一份是常见 bug（见 lesson `2026-04-24-config-path-and-bind-banner.md` Lesson 2 "声明 vs 现实"反例）
   - ADR / lesson 文件本身是可点击的；GitHub / VS Code Markdown Preview 都能跳转
   - README 长度控制：节点 1 阶段建议 ≤500 行（参考类似规模项目如 cobra / kubectl 子模块 README 250-400 行）；本 story README 估计 ~350-450 行

6. **8 章节顺序固定**：dev 视线流："上手 → 依赖 → 配置 → 验证（测试） → 高级用法（dev mode） → 找代码（目录） → 出问题（troubleshoot） → 必须懂（纪律）"。这不是随便排的，每个章节的存在前提是前一个已经懂了。**不**调换顺序（如把 troubleshoot 提前到第 2 章）—— 那会让新 dev 在还没跑起来时被 troubleshoot 内容吓到

7. **快速启动是"三行复制粘贴"**：必须 dev 拿到仓库后**3 步内** server 跑起来 + ping 返回 200。这是 README 的"hello world 时刻"，体验上一切其他章节都为它服务。任何让快速启动 >5 行的内容都应该挪到下方专门章节

8. **Troubleshooting ≥5 条但 ≤8 条**：少于 5 条不够覆盖常见场景；多于 8 条变成"百科全书"反而难找。本 story 钦定 5 条核心 + dev 可加 1-2 条本机遇到的；超过 8 条建议拆 `docs/troubleshooting.md`（**不**在本 story scope）

9. **不写代码示例片段**：README **不是**教程文档；不写 "如何添加新 service" / "如何写新 handler" 之类的"how-to"。这些是 ADR / 设计文档 / sample 代码本身的事。README 只**指向**它们（"业务 service 模板见 `internal/service/sample/service.go`"）

10. **不在 README 写 commit message 规范**：项目 commit 规范见 [git log 历史](git log --oneline) + CLAUDE.md（如有提及）；README 不重复

11. **配置环境变量列表只列已实装的**：节点 1 只有 `CAT_HTTP_PORT` / `CAT_LOG_LEVEL` 两个（见 `loader.go:13-14`）；**不**列 epics.md 未来章节的环境变量（如 `CAT_MYSQL_DSN` / `CAT_REDIS_ADDR`，Epic 4/10 才落地）；如果 Epic 4 加了新环境变量，Story 4-2 自己去 README 同步（这正是 AC11 同步原则的存在意义）

12. **目录结构树**：用 ASCII（fenced code block ` ```text ``` `）；**不**用 mermaid（节点 1 阶段不引入 mermaid 渲染依赖；GitHub 现代版渲染 mermaid 但 IDE 不全部支持）；**不**用 PlantUML / Graphviz（同理）

13. **Markdown 表格**：所有"字段表" / "命令矩阵" / "troubleshoot" 用 markdown table，**不**用嵌套 list。表格在 GitHub / VS Code Preview 都渲染良好；嵌套 list 超过 2 层就乱

14. **不引入图片 / 截图**：节点 1 没必要；图片在仓库膨胀（git LFS 未启用）+ 跨平台路径敏感（Windows `\\` vs `/`）；future epic 真需要展示 ws 流程图等再单独引入

### 与上游 9 条 story 的契约兑现表

| 上游 story | 输出 → 本 story 引用方式 |
|---|---|
| 1.1 ADR-0001 §3.5 测试栈 / build.sh contract | `## 跑测试` 章节指针："详见 ADR-0001 §3"；不复制四件套清单 |
| 1.1 ADR-0001 §6 配置文件 4 档（local/dev/staging/prod）| `## 配置` 章节"四档环境"段引用 |
| 1.2 cmd/server + ping + LocateDefault | `## 快速启动` 显式 `-config server/configs/local.yaml`；`## 配置` "配置路径解析"段说 LocateDefault |
| 1.3 中间件三件套 + /metrics | `## 目录结构`目录注释中 `internal/app/http/middleware/` 标 ✅ 节点 1；不在 README 主体复制 middleware 实现细节 |
| 1.4 /version + buildinfo ldflags | `## 快速启动` curl /version 命令 + `## Troubleshooting` 第 4 条 `commit=unknown` |
| 1.5 sample + 测试基础设施 | `## 跑测试` "测试栈"指针 ADR-0001 §3；`## 目录结构` `internal/service/sample/` 标 ✅ 节点 1 |
| 1.6 Dev Tools 框架 + /dev/ping-dev | `## Dev mode` 整章；引用 Story 1.6 双闸门 |
| 1.7 build.sh 5 开关 | `## 快速启动` + `## 跑测试` 全部命令；`## 跑测试` 互斥规则段 |
| 1.8 AppError + 三层错误映射 | `## 工程纪律` 一行指针：ADR-0006 |
| 1.9 ctx 传播 + ADR-0007 + ctx_done | `## 工程纪律` 一行指针：ADR-0007；不在 README 复制 4 条 ctx 约定（ADR 已固化） |

### Lessons Index（与本 story 相关的过去教训）

- **直接相关**：[`docs/lessons/2026-04-24-config-path-and-bind-banner.md`](../../docs/lessons/2026-04-24-config-path-and-bind-banner.md)
  - **Lesson 1 "CWD-relative path"**：README `## 配置` 章节"配置路径解析"段必须显式说明 CWD-relative + 推荐显式传 `-config`（README 自己的 quick start 也按此写）
  - **Lesson 2 "声明 vs 现实"**：本 story **就是**为了防"README 说的命令与代码实际行为分裂"；写 README 前必须 `cat scripts/build.sh` / `cat configs/local.yaml` 等核对现状（Dev Notes §4）；写完后跑 AC12 手动验证保证一致

- **直接相关**：[`docs/lessons/2026-04-25-slog-init-before-startup-errors.md`](../../docs/lessons/2026-04-25-slog-init-before-startup-errors.md)
  - **Lesson 1**：README **不**承诺"server 启动失败时一定有 JSON 日志"—— 该话题归代码内部约束（main.go 启动顺序），不是 README 该写的内容。但 `## Troubleshooting` 第 5 条 "config file not found" 隐含承诺早期错误能被 dev 看到

- **间接相关**：[`docs/lessons/2026-04-24-sample-service-nil-dto-and-slog-test-group.md`](../../docs/lessons/2026-04-24-sample-service-nil-dto-and-slog-test-group.md)
  - **Lesson 1 "公共 artifact 质量门槛"**：README 是**最公共**的 artifact（每个新 dev 都看），质量门槛比 sample 还高。本 story 的"假设 N 个新 dev 各自拿这份 README 上手"评估视角与 lesson 1 的"假设 N 个 service 各自复制 sample"等价。具体：每条命令必须 100% 可复制粘贴；每个链接必须真实存在；每个章节必须有可执行价值（无空话章节）

- **间接相关**：[`docs/lessons/2026-04-24-go-vet-build-tags-consistency.md`](../../docs/lessons/2026-04-24-go-vet-build-tags-consistency.md)
  - **Lesson 1**：README `## 跑测试` 章节提到 `bash scripts/build.sh --devtools --test` 时**必须**说明它会跑 `-tags devtools` 路径下的测试（与默认 `--test` 跑的不同）；`go vet` 会跟随 build tag。但 README 不深入解释 build tag 文件分裂语义（属代码层面），只给指针到 ADR / story 1.6

- **间接相关**：[`docs/lessons/2026-04-24-middleware-canonical-decision-key.md`](../../docs/lessons/2026-04-24-middleware-canonical-decision-key.md) + [`2026-04-24-error-envelope-single-producer.md`](../../docs/lessons/2026-04-24-error-envelope-single-producer.md)
  - **两条 lesson 都属于 `## 工程纪律` 章节的"error envelope 单一生产者"指针**，README 用一行链接到 lesson 即可，不复述

### Git intelligence（最近 8 个 commit）

```
51ae73b chore(sprint-status): 1-10 → ready-for-dev
6336c33 chore(claude): 更新 Bash allowlist
22a24a1 chore: .gitignore 加 .ai-pipeline/ 排除
c713efd chore(commands): 新增 /epic-loop 命令
c5443a1 chore: 更新 CLAUDE.md（追加 ctx 必传约定）
d7f506b chore(claude): 更新 Bash allowlist
d6d4017 docs(bmad-output): 新增 ADR-0007 context 传播 + backfill 1-3 ctx_done 注释
ed7b143 chore(story-1-9): 收官 Story 1.9 + 归档 story 文件
```

最近实装向 commit 是 Story 1.9 落地（`d6d4017` ADR-0007 + `ed7b143` 收官）。本 story 紧随 1.9 done。本 story HEAD 是 `51ae73b`（sprint-status 1-10 ready-for-dev 的 chore 提交，已经 push）。

**commit message 风格**：Conventional Commits，中文 subject，scope `story-X-Y` / `server` / `scripts` / `claude` / `commands` / `docs`。
本 story 建议：`docs(server): Epic1/1.10 server README 本地开发指南` 或 `docs(server): Epic1/1.10 README 收官节点 1`
（理由：纯文档新增，scope 用 `server` 因为文件落在 server/ 目录；`docs` type 与节点 1 其他文档 commit 一致）

### 常见陷阱

1. **README 写完后链接断了**：本 story 引用大量 `../docs/...` / `../_bmad-output/...` / `../CLAUDE.md` 路径。**写完必须**逐个 `ls -l` 确认存在；**特别注意**：
   - `docs/lessons/` 下的 6 个文件名是**带日期前缀**的（`2026-04-24-xxx.md`），写错日期就 404
   - `_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md` 是 4 位数字编号 + kebab-case，写错编号就 404
   - `docs/宠物互动App_*.md` 7 份带中文连字符 `_`（不是 `-`），写错就 404

2. **README 命令拷贝粘贴跑不通**：dev 复制 README 的 `bash scripts/build.sh` 之后必须**真**能 exit 0；如果 README 写 `bash ./scripts/build.sh`（多了 `./`）在某些 Windows shell 下行为微妙不同。**统一**用 `bash scripts/build.sh`（无 `./`，与 CLAUDE.md §Build & Test 一致）

3. **端口冲突在不同机器表现不同**：8080 在 Windows 上常被 IIS / Skype（旧版）/ 某些 Java 应用占；macOS 上 8080 常空；Linux 8080 通常空。README 不假设具体冲突场景，只给"如何排查"命令（`lsof -i :8080` / `netstat -ano | findstr 8080`）

4. **README 提到的 dev 命令 `BUILD_DEV=true`**：在 Windows cmd 下 `BUILD_DEV=true ./build/catserver.exe` 是 bash 语法，**cmd.exe 不识别**。本项目用 Git Bash / WSL 不假设原生 cmd.exe，README **不**额外注释 cmd.exe 写法（CLAUDE.md Environment 已钦定 bash）。但 `## Troubleshooting` 可加一条"Windows cmd.exe 不支持 `VAR=val cmd` 语法，请用 Git Bash"作为 6th 条（如果空间允许）

5. **目录结构树渲染破损**：用 fenced ` ```text ` 而非 ` ``` `（无语言标签）—— GitHub markdown 渲染对无语言 code block 有时把 ` ├─ ` / ` └─ ` 等 box-drawing 字符 escape 错。`text` 标签强制纯文本

6. **README 太长导致 overview 丢失**：如果总行数 >500 建议在 H1 之后加目录（TOC）；目录 markdown 链接锚点（`[快速启动](#快速启动)`）—— GitHub 渲染时自动支持中文锚点（kebab-case 化）。本 story 不强求 TOC，但若 README 写完 ≥350 行**应**补 TOC

7. **不要说"详见我们的文档"**：每个引用必须**精确**到文件 + 章节。说"详见 ADR" 是反模式；说"详见 ADR-0001 §3.5" 是正模式

8. **不要为节点 1 不存在的特性写 README 章节**：`## 部署` / `## API 文档` / `## 性能调优` 这些章节都属未来；本 story **不**预先占位（占位会让 README 看起来像没写完）。Epic 3 / Epic 9 等"节点验收"story 自己加章节

9. **README 不写"作者"/ "联系方式"**：单一开发者 + 私有仓库；属冗余。git log 是事实来源

10. **`✅` / `🚧` emoji 在 GitHub / VS Code 都正常**：但终端 cat 时可能 box（不影响 README 用途）；本 story 用是因为节点 1 阶段需要"已实装 vs 未来 Epic"的视觉区分；MVP 收官（Epic 36 done）后可统一去掉 emoji（届时全部 ✅）

11. **README 应该是 git 友好的 plain markdown**：**不**用 GitHub 私有扩展（`> [!NOTE]` / `> [!WARNING]` 等 admonition 块；那是 GitHub 渲染特性，VS Code Preview 不支持）；用普通 quote `> 注意：...` 即可

12. **不在 README 直接放 `local.yaml` 全文**：用字段说明表 + 跨链接 `server/configs/local.yaml`；放全文等于双源（lesson `2026-04-24-config-path-and-bind-banner.md` Lesson 2）

### 与节点 1 之后业务 epic 的衔接（informational，非本 story scope）

Epic 3 / Epic 6 / Epic 9 / Epic 13 / Epic 16 / Epic 19 / Epic 22 / Epic 25 / Epic 28 / Epic 31 / Epic 34 / Epic 36 的"X.3 文档同步与 tech-debt 登记" story **必须**回头改 `server/README.md`：

| Epic 节点完成 | 本 story README 要改的章节 |
|---|---|
| Epic 4 (Auth + MySQL) | `## 依赖`：MySQL 8.0 从"未来"改"必需"；`## 配置`：加 `server.mysql.dsn` / 环境变量 `CAT_MYSQL_DSN`；`## 跑测试`：integration 测试段从"暂无"改"已落地，命令 `bash scripts/build.sh --integration`"；`## 目录结构`：`internal/repo/mysql/` / `migrations/` 标 ✅ |
| Epic 7 (Step) | `## Dev mode`：dev 端点列表加 `POST /dev/grant-steps` |
| Epic 10 (Redis + WS) | `## 依赖`：Redis 6+ 从"未来"改"必需"；`## 目录结构`：`internal/app/ws/` / `internal/repo/redis/` 标 ✅ |
| Epic 20 (Chest) | `## Dev mode`：dev 端点列表加 `POST /dev/force-unlock-chest` / `POST /dev/grant-cosmetic-batch` |
| Epic 36 (MVP 整体收官) | 全 README 一遍体检；🚧 标记应已全部消除变 ✅；`## 维护说明`段提示后续生产化 / 部署 SOP 单独 PR |

这是 AC11 "维护说明"段的存在意义。本 story 写 README 时**不**预填这些未来变更（属未来 Epic 的事）；只在维护说明段告诉未来 dev "你来对了，按这个原则改"。

### Project Structure Notes

**新增文件**（1 个）：
- `server/README.md` — 节点 1 收官文档，8 章节 + 维护说明段

**修改文件**（1 个）：
- `_bmad-output/implementation-artifacts/sprint-status.yaml` — 1-10 状态流转（backlog → ready-for-dev → in-progress → review → done；`last_updated` 时间戳）

**删除文件**：无

**不动文件**（明确范围红线）：
- `CLAUDE.md` — 仓库根 AI 上下文，不是 server/ README 的副本（已在 Story 1.7 改写为承接说明）
- `MEMORY.md` — Claude session memory，与 README 职责正交
- `docs/宠物互动App_*.md` — 设计文档不是 README 引用源（README 引用其路径但不修改）
- `_bmad-output/implementation-artifacts/decisions/*.md` — ADR 不是 README 副本（README 引用路径但不修改）
- `docs/lessons/*.md` — lesson 不是 README 副本（README 引用路径但不修改）
- `scripts/build.sh` — 命令面已在 Story 1.7 定型；本 story 只**描述**，不修改
- `server/cmd/server/main.go` / `server/internal/**/*.go` / `server/configs/local.yaml` / `server/go.mod` / `server/go.sum` — 全部代码已在 Story 1.1-1.9 定型；本 story 零代码改动
- `_bmad-output/planning-artifacts/epics.md` — epics 是 README 引用源（§Story 1.10 钦定本 story 范围）；不修改

**对目录结构的影响**：在 `server/` 增加一个 `README.md`；Go 编译 / build.sh / 测试 / IDE 工程文件**全不**受影响

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.10] — 本 story 原始 AC（README 8 章节 + 同步原则 + 手动验证）
- [Source: _bmad-output/planning-artifacts/epics.md#Epic-1] — Epic 1 scope "server README + 本地开发指南" 明示在范围内
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#4] — `## 目录结构` 章节复制源（精简到 server/ 子树）
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#5.1-5.3] — handler/service/repo 分层（`## 工程纪律` 章节锚点）
- [Source: docs/宠物互动App_MVP节点规划与里程碑.md#3] — 10 节点顺序（`## 工程纪律` 第一行"节点顺序不可乱跳"锚点）
- [Source: docs/宠物互动App_数据库设计.md#8] — 资产事务边界（`## 工程纪律` 锚点）
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#3] — 测试栈四件套（`## 跑测试` 章节锚点）
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#3.5] — build.sh contract（`## 跑测试` 章节锚点）
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#6] — 配置 4 档环境（`## 配置` 章节锚点）
- [Source: _bmad-output/implementation-artifacts/decisions/0006-error-handling.md] — AppError 三层映射（`## 工程纪律` 锚点）
- [Source: _bmad-output/implementation-artifacts/decisions/0007-context-propagation.md] — ctx 传播框架（`## 工程纪律` 锚点）
- [Source: _bmad-output/implementation-artifacts/1-2-cmd-server-入口-配置加载-gin-ping.md] — cmd/server 入口 + LocateDefault（`## 配置` 章节锚点）
- [Source: _bmad-output/implementation-artifacts/1-4-version-接口.md#AC3] — buildinfo ldflags 注入路径（`## Troubleshooting` 第 4 条锚点）
- [Source: _bmad-output/implementation-artifacts/1-5-测试基础设施搭建.md#AC7] — Windows TSAN 偏离原型（`## 跑测试` Windows TSAN 段锚点）
- [Source: _bmad-output/implementation-artifacts/1-6-dev-tools-框架.md#AC8] — `/dev/ping-dev` 端点（`## Dev mode` 当前 dev 端点段锚点）
- [Source: _bmad-output/implementation-artifacts/1-6-dev-tools-框架.md#AC2] — 双闸门机制（`## Dev mode` 启用方式段锚点）
- [Source: _bmad-output/implementation-artifacts/1-7-重做-scripts-build-sh.md#AC8] — build.sh 5 开关 + 互斥规则（`## 跑测试` 互斥规则段锚点）
- [Source: _bmad-output/implementation-artifacts/1-7-重做-scripts-build-sh.md#T5.3] — Windows TSAN 偏离实测（`## 跑测试` Windows TSAN 段锚点）
- [Source: _bmad-output/implementation-artifacts/1-9-go-context-传播框架-cancellation-验证.md#AC1] — ADR-0007 §1-§7 内容（`## 工程纪律` ctx 必传锚点）
- [Source: docs/lessons/2026-04-24-config-path-and-bind-banner.md] — 配置路径 / bind_host 防火墙（`## 配置` + `## Troubleshooting` 第 2 条 + 第 5 条锚点）
- [Source: docs/lessons/2026-04-25-slog-init-before-startup-errors.md] — 早期错误结构化日志（间接相关，README 不直引但精神保持）
- [Source: docs/lessons/2026-04-24-sample-service-nil-dto-and-slog-test-group.md] — 公共 artifact 质量门槛（README 写法的方法论锚点）
- [Source: docs/lessons/2026-04-24-go-vet-build-tags-consistency.md] — build tag 一致性（`## 跑测试` `--devtools --test` 段隐含锚点）
- [Source: docs/lessons/2026-04-24-middleware-canonical-decision-key.md] — middleware canonical decision（`## 工程纪律` error envelope 锚点）
- [Source: docs/lessons/2026-04-24-error-envelope-single-producer.md] — error envelope 单一生产者（`## 工程纪律` error envelope 锚点）
- [Source: server/configs/local.yaml] — 字段表数据源（`## 配置` 章节）
- [Source: server/internal/infra/config/loader.go:13-14] — 环境变量 `CAT_HTTP_PORT` / `CAT_LOG_LEVEL`（`## 配置` 环境变量段）
- [Source: server/internal/app/http/devtools/devtools.go:50] — `BUILD_DEV` 字面值要求（`## Dev mode` 启用方式段）
- [Source: scripts/build.sh] — 5 开关命令面（`## 快速启动` + `## 跑测试` 命令源）
- [Source: CLAUDE.md#Build-Test] — README 跑测试章节与 CLAUDE.md 命令面一致性（互为印证）
- [Source: CLAUDE.md#工作纪律] — README `## 工程纪律` 章节锚点（README 是 CLAUDE.md 工作纪律的"用户友好版"）
- [Source: CLAUDE.md#Repo-Separation] — server / ios / watch 三独立目录（README 顶部 tagline 锚点）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- 写 README 前 `cat scripts/build.sh` / `cat server/configs/local.yaml` / `cat server/internal/infra/config/loader.go` / `cat server/internal/app/http/devtools/devtools.go` 核对现状（防"声明 vs 现实"分裂；见 lesson `2026-04-24-config-path-and-bind-banner.md` Lesson 2）
- AC10 钦定的 `[MEMORY.md](../MEMORY.md)` 链接在仓库内**不存在**（`MEMORY.md` 只在用户 Claude profile dir 下作为 session memory，**非**仓库文件）；为避免破链，改写成"`CLAUDE.md` + `docs/` 设计文档七份" + 注明 Claude session memory 的 profile 路径。**这是 AC10 写法的微调，不削弱"上手前要懂的"指针的语义**
- 链接巡检：用 `Grep '\]\(\.\./[^)]+\)'` 提出 README 全部 20 个相对路径，逐个 `ls -e` 校验，全部存在
- AC12 手动验证全部 7 步通过（stdout 摘要见 Completion Notes）

### Completion Notes List

**实装总结**：

- ✅ AC1：`server/README.md` 创建于 `server/` 目录，与 `go.mod` / `cmd/` 平级；UTF-8 + LF；H1 `# Cat Server` + tagline 含 server / iOS / watchOS 三端说明
- ✅ AC2：8 个 H2 章节按钦定顺序：`快速启动 → 依赖 → 配置 → 跑测试 → Dev mode → 目录结构 → Troubleshooting → 工程纪律`，外加可选 `维护说明` H2（AC11）
- ✅ AC3：快速启动 3 行命令（build → run → curl ping+version）；端口 8080 + IP 127.0.0.1 与 `local.yaml` 默认对齐；显式传 `-config` + 警告 `go run` 会让 `/version=unknown`
- ✅ AC4：依赖三段（节点 1 / MVP 演进 / 测试依赖）；docker run + brew + winget 都给出
- ✅ AC5：配置 5 字段表（默认值 / 类型 / 含义 / 何时改）；环境变量 `CAT_HTTP_PORT` / `CAT_LOG_LEVEL` + 完整命令例；CWD-relative 解析说明 + bind_host 防火墙提示 + lesson 1 引用
- ✅ AC6：5 行命令矩阵（含 `--devtools --test` 第 5 行）；互斥规则 + Windows TSAN 偏离段 + 测试栈四件套指针 + 单测/集成测分界
- ✅ AC7：双闸门（`BUILD_DEV=true` 严格字面 + `--devtools`）+ 启动 banner（两条 WARN）+ 当前 `/dev/ping-dev` + 未来 3 个 dev 端点占位 + 生产部署 SOP
- ✅ AC8：ASCII 目录树（`text` 标签）；每行 `✅` / `🚧` 标注；`🚧` 全部指向具体 Epic / Story
- ✅ AC9：6 条 troubleshoot（端口占用 / 防火墙 / TSAN / `version=unknown` / 配置路径 / cmd.exe）；每条症状 → 原因 → 解决 + 具体命令；引用 lesson 路径 `ls` 核实
- ✅ AC10：11 条工程纪律 + 链接到 ADR / 设计文档 / lesson；`MEMORY.md` 链接根据 Debug Log 注明的"不存在"事实改写为 `docs/` + Claude profile session memory 路径
- ✅ AC11：`## 维护说明` 段落 + 5 行 Epic 完成 → 改 README 章节映射表
- ✅ AC12：手动验证 7 步全部通过（stdout 摘要如下）

**AC12 手动验证 stdout 摘要**：

```
# 步骤 1：bash scripts/build.sh
=== go vet ===

=== go build (commit=51ae73b, builtAt=2026-04-25T11:23:48Z) ===
OK: binary at build/catserver.exe

BUILD SUCCESS

# 步骤 2：./build/catserver.exe -config server/configs/local.yaml &
{"time":"2026-04-25T19:27:48.5311905+08:00","level":"INFO","msg":"config loaded","path":"server/configs/local.yaml","http_port":8080,"log_level":"info"}
[GIN-debug] GET /ping ... GET /version ... GET /metrics
{"time":"2026-04-25T19:27:48.5376644+08:00","level":"INFO","msg":"server started","addr":"127.0.0.1:8080"}

# 步骤 3：curl http://127.0.0.1:8080/ping
{"code":0,"message":"pong","data":{},"requestId":"fc7a14c7-c3bb-4d15-bfae-5e9d159b6187"}

# 步骤 4：curl http://127.0.0.1:8080/version
{"code":0,"message":"ok","data":{"commit":"51ae73b","builtAt":"2026-04-25T11:23:48Z"},"requestId":"ae29cc40-10d1-430a-961a-12467b569b73"}
（commit=51ae73b 是 short hash，**非** "unknown"；builtAt=ISO 8601 UTC ✅）

# 步骤 5：bash scripts/build.sh --test
=== go test ===
ok  github.com/huing/cat/server/internal/app/bootstrap   0.938s
ok  github.com/huing/cat/server/internal/app/http/devtools   0.772s
ok  github.com/huing/cat/server/internal/app/http/handler   0.130s
ok  github.com/huing/cat/server/internal/app/http/middleware   0.250s
ok  github.com/huing/cat/server/internal/infra/config   0.280s
ok  github.com/huing/cat/server/internal/infra/logger   0.294s
ok  github.com/huing/cat/server/internal/infra/metrics   0.541s
ok  github.com/huing/cat/server/internal/pkg/errors   0.357s
ok  github.com/huing/cat/server/internal/pkg/testing   0.426s
ok  github.com/huing/cat/server/internal/pkg/testing/slogtest   0.364s
ok  github.com/huing/cat/server/internal/service/sample   0.474s
OK: all tests passed
BUILD SUCCESS

# 步骤 6：bash scripts/build.sh --devtools
=== go build (commit=51ae73b, builtAt=2026-04-25T11:29:16Z) ===
OK: binary at build/catserver-dev.exe
BUILD SUCCESS

# 步骤 6 续：./build/catserver-dev.exe -config server/configs/local.yaml &
{"...","msg":"config loaded","http_port":8080,"log_level":"info"}
{"...","level":"WARN","msg":"DEV MODE ENABLED - DO NOT USE IN PRODUCTION"}                    # main.go 1 条
[GIN-debug] GET /dev/ping-dev --> ...PingDevHandler
{"...","level":"WARN","msg":"DEV MODE ENABLED - DO NOT USE IN PRODUCTION","build_tag_devtools":true,"env_build_dev":""}   # devtools.Register 1 条
（共 2 条 DEV MODE WARN ✅）

# 步骤 7：curl http://127.0.0.1:8080/dev/ping-dev
{"code":0,"message":"ok","data":{"mode":"dev"},"requestId":"3236175b-9d8a-4b2e-ad1e-24ed0a5623cf"}
（mode=dev ✅）
```

**关键发现**：

1. README 命令面与节点 1 现状（Story 1.1-1.9 落地版本）完全一致：build.sh 5 开关 / local.yaml 5 字段 / `/dev/ping-dev` 唯一 dev 端点 / `commit` 是 git short hash —— 与 Dev Notes §4 钦定的"声明 vs 现实"防分裂要求完全对齐
2. AC10 列出的 `MEMORY.md` 链接是仓库**不存在**的文件（只有用户 Claude profile dir 下有作为 session memory），按"链接巡检必须 ls 真实"原则改写为 `docs/` + profile path 注明 —— 见 Debug Log 第 2 条
3. README 总长 ~290 行（Dev Notes §5 估计 350-450，实际更精简），未触发"≥350 行加 TOC"门槛（Dev Notes §6）
4. 手动验证 100% 命令复制粘贴一次通过，未发现 README 与代码分裂

### File List

**新增**：
- `server/README.md` — 节点 1 收官文档；8 章节 + 维护说明段；中文；UTF-8 + LF

**修改**：
- `_bmad-output/implementation-artifacts/sprint-status.yaml` — `1-10-server-readme-本地开发指南: ready-for-dev → in-progress → review`；`last_updated` 时间戳更新
- `_bmad-output/implementation-artifacts/1-10-server-readme-本地开发指南.md` — Status: `ready-for-dev → review`；T1-T10 全部 `[x]`；Dev Agent Record 填充；Change Log 加版 0.2

**未动**：CLAUDE.md / 任何 Go 代码 / `scripts/build.sh` / `server/configs/local.yaml` / `server/go.mod` / `server/go.sum` / 任何 `docs/` 文件 / 任何 ADR 文件 / 任何 lesson 文件 — 与 Project Structure Notes §"不动文件"红线一致

## Change Log

| 日期 | 版本 | 描述 | 作者 |
|---|---|---|---|
| 2026-04-24 | 0.1 | 初稿（ready-for-dev） | SM |
| 2026-04-25 | 0.2 | 实装 server/README.md（10 任务全完）；AC12 手动验证 7 步通过；状态 → review | Dev (claude-opus-4-7[1m]) |
