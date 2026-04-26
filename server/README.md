# Cat Server

宠物互动 App 后端（Go + Gin + MySQL + Redis 单体）。本目录是 server/ 工程；客户端见 ../ios/，watchOS 暂不考虑（见 [CLAUDE.md](../CLAUDE.md)）。

> 节点 1（App 与 Server 可运行）阶段；MySQL/Redis 在 Epic 4/10 才接入，本节点跑 server **不需要**它们。详见 [`docs/宠物互动App_MVP节点规划与里程碑.md`](../docs/宠物互动App_MVP节点规划与里程碑.md) §3。

---

## 快速启动

3 行命令把 server 跑起来 + 验证。**从仓库根目录** `C:\fork\cat` 跑（Windows 用 Git Bash / WSL；macOS / Linux 自带 bash）：

```bash
# 第一次：编译 + 跑
bash scripts/build.sh
./build/catserver -config server/configs/local.yaml

# 验证（另开一个 shell）
curl http://127.0.0.1:8080/ping       # → {"code":0,"message":"pong",...}
curl http://127.0.0.1:8080/version    # → {"code":0,"data":{"commit":"<short hash>","builtAt":"..."}}
```

> Windows 上 `./build/catserver` 实际是 `./build/catserver.exe`（`scripts/build.sh` 用 `$(go env GOEXE)` 自动加后缀）。

**坑提醒**：

- **不要**用 `go run ./cmd/server` 替代 `bash scripts/build.sh`：前者绕过 `-ldflags` 注入，`/version` 会返回 `"unknown"`（详见 [Troubleshooting #4](#troubleshooting)）。
- 端口 `8080` 与 IP `127.0.0.1` 与 [`server/configs/local.yaml`](configs/local.yaml) 默认值一致；改端口走环境变量 `CAT_HTTP_PORT`，不要在 README 命令里随手换成 `:8090`。
- `-config server/configs/local.yaml` 显式传 path 最稳；`LocateDefault()` 也能自动找，但 CWD 漂移会让自动查找失败（详见 [配置 → 配置路径解析](#配置)）。

---

## 依赖

### 当前节点 1 依赖

| 工具 | 版本 | 验证命令 |
|---|---|---|
| Go | 1.25+ | `go version` |
| git | 任意现代版 | `git --version` |
| bash | Git Bash (Windows) / WSL / 系统自带 (macOS/Linux) | `bash --version` |

**节点 1 阶段除上述工具外无其它运行期依赖**：MySQL / Redis / docker 都不需要。

> Go 1.25+ 是 [`server/go.mod`](go.mod) 第一行 `go 1.25.0` 的下限；如果装的是 1.24 跑 `bash scripts/build.sh` 会报 `module requires go >= 1.25.0`。

### MVP 演进依赖（Epic 4 / Epic 10 才接入）

| 服务 | 启用节点 | 推荐本地启动方式 |
|---|---|---|
| MySQL 8.0 | Epic 4（Auth + 五张表 migration） | `docker run -d --name cat-mysql -e MYSQL_ROOT_PASSWORD=catdev -e MYSQL_USER=cat -e MYSQL_PASSWORD=catdev -e MYSQL_DATABASE=cat -p 3306:3306 mysql:8.0`<br>或 `brew install mysql@8.0` (macOS) / `winget install Oracle.MySQL` (Windows)（非 docker 路径需手动 `CREATE USER 'cat'@'%' IDENTIFIED BY 'catdev'; CREATE DATABASE cat; GRANT ALL ON cat.* TO 'cat'@'%';`） |
| Redis 6+ | Epic 10（WS gateway + presence） | `docker run -d --name cat-redis -p 6379:6379 redis:6-alpine`<br>或 `brew install redis` (macOS) / `winget install Redis.Redis-x64` (Windows) |

> 端口 3306 / 6379 是 MySQL/Redis 标准端口；本地多实例冲突时自行改 `-p 13306:3306` / `-p 16379:6379`。
>
> **节点 1 跑 server 不需要这两个**：[`server/configs/local.yaml`](configs/local.yaml) 也尚无 MySQL/Redis 配置项（Epic 4 Story 4-2 / Epic 10 Story 10-2 才加）。

### 测试依赖

`bash scripts/build.sh --test` 首次运行时会通过 `go mod download` 拉取 [`server/go.mod`](go.mod) 里声明的测试依赖（testify / sqlmock / miniredis / yaml.v3 等），之后走本地缓存。如果 `GOPROXY` 不通需手动改成 `https://goproxy.cn,direct` 等可达代理。

---

## 配置

### 配置文件位置

`server/configs/local.yaml` 是当前唯一的配置文件。按 [ADR-0001 §6](../_bmad-output/implementation-artifacts/decisions/0001-test-stack.md) 的 4 档环境约定（local / dev / staging / prod），节点 1 阶段只落地 `local`；`dev` / `staging` / `prod` 见 Epic 4+。

### 字段说明（`server/configs/local.yaml`）

| 字段 | 默认值 | 类型 | 含义 | 何时改 |
|---|---|---|---|---|
| `server.bind_host` | `127.0.0.1` | string | server 监听网卡 IP；loopback 免 Windows Defender 防火墙弹窗 | 生产部署改 `0.0.0.0` 或删此行（监听所有网卡） |
| `server.http_port` | `8080` | int | HTTP 监听端口 | 端口冲突时改其他可用端口（如 18090），或走环境变量 `CAT_HTTP_PORT` |
| `server.read_timeout_sec` | `5` | int | HTTP read header / body 超时（秒） | 上传大体积 payload 时调大；MVP 阶段保持默认 |
| `server.write_timeout_sec` | `10` | int | HTTP response 写超时（秒） | server-side 流式响应 / 大文件下载时调大；MVP 阶段保持默认 |
| `log.level` | `info` | string | slog level（`debug` / `info` / `warn` / `error`） | 本地排障改 `debug`；生产保持 `info` |

> **不在 README 直接放 `local.yaml` 全文**：避免双源不一致。需要看完整内容直接打开 [`server/configs/local.yaml`](configs/local.yaml)。

### 环境变量覆盖

| 环境变量 | 覆盖字段 | 取值 | 备注 |
|---|---|---|---|
| `CAT_HTTP_PORT` | `server.http_port` | ASCII 整数（如 `18090`） | 非法值（如 `abc`）启动报错 |
| `CAT_LOG_LEVEL` | `log.level` | `debug` / `info` / `warn` / `error` | CI / 临时 demo 调级用 |

完整命令例：

```bash
CAT_HTTP_PORT=18090 CAT_LOG_LEVEL=debug ./build/catserver -config server/configs/local.yaml
```

> 字段实装见 [`server/internal/infra/config/loader.go`](internal/infra/config/loader.go) 第 13-14 行的 `envHTTPPort` / `envLogLevel` 常量。

### 配置路径解析

不传 `-config` 时 server 走 `LocateDefault()`，按 CWD-relative 顺序查找：

1. `server/configs/local.yaml`（仓库根 CWD）
2. `configs/local.yaml`（`server/` CWD）

**生产部署应显式传** `-config /etc/cat/prod.yaml` 避免 CWD 漂移导致找不到配置（详见 [`docs/lessons/2026-04-24-config-path-and-bind-banner.md`](../docs/lessons/2026-04-24-config-path-and-bind-banner.md) Lesson 1）。

### bind_host 防火墙提示

`bind_host: 127.0.0.1` 是本地开发默认值：Windows Defender 对 loopback 免检，不会因为每次重编译 binary 产生新 hash 弹"允许专用网络访问"窗。生产部署改 `0.0.0.0` 或删此行让 server 监听所有网卡。详见 [Troubleshooting #2](#troubleshooting)。

---

## 跑测试

### 命令矩阵

| 命令 | 用途 |
|---|---|
| `bash scripts/build.sh --test` | 单元测试（`go test -count=1 ./...`，禁缓存） |
| `bash scripts/build.sh --race --test` | 加 race detector；**Windows 本机已知 TSAN 偏离**，归 CI Linux 跑 |
| `bash scripts/build.sh --test --coverage` | 覆盖率，产出 `build/coverage.out`；`go tool cover -html=build/coverage.out` 看 HTML |
| `bash scripts/build.sh --integration` | 集成测试（`-tags=integration`，120s timeout）；节点 1 暂无 integration 测试，Epic 4 Story 4-7 首次落地 |
| `bash scripts/build.sh --devtools --test` | 跑 `-tags devtools` 路径下的测试（与默认 `--test` 不同；详见 [`docs/lessons/2026-04-24-go-vet-build-tags-consistency.md`](../docs/lessons/2026-04-24-go-vet-build-tags-consistency.md)） |

### 互斥规则

- `--coverage` 必须配 `--test`（脚本会报 `ERR: --coverage requires --test` 退出）
- `--devtools` 与 `--integration` 互斥（共用单 `-tags` slot；脚本会报 `ERR: --devtools and --integration are mutually exclusive` 退出）
- 其余开关正交（详见 [Story 1.7 #AC8](../_bmad-output/implementation-artifacts/1-7-重做-scripts-build-sh.md)）

### Windows TSAN 偏离

Windows 本机跑 `--race` 会因 ThreadSanitizer 内存分配限制（`ThreadSanitizer failed to allocate ... error code: 87`）失败 —— OS 级限制，非代码 bug。

- **本机想跑 race**：用 WSL2 / 真 Linux 机器
- **CI 上 race**：归 Linux runner 执行（节点 1 阶段未配 CI；Epic 3 Story 3-3 才登记）

详见 Story 1.5 #AC7 / Story 1.7 #T5.3 / Story 1.9 Debug Log 三处共识。

### 测试策略指针

testify + sqlmock + miniredis + slogtest 四件套；详见 [ADR-0001 §3](../_bmad-output/implementation-artifacts/decisions/0001-test-stack.md)（特别是 §3.5 build.sh contract）。本 README **不**复制 ADR 内容，给指针即可。

### 单测 vs 集成测

- **节点 1 阶段全部是单测**（含 sample / middleware / config / logger / metrics 等共 12+ 个包）
- **集成测试**（dockertest 起真 MySQL / miniredis）从 Epic 4 Story 4-7 开始引入

> 想只跑某一个包：`cd server && go test ./internal/service/sample/`。但**主力示范**是 `bash scripts/build.sh --test`（保证 `go vet` 先跑 + `-count=1` 禁缓存）。

---

## Dev mode

### 启用方式（OR 语义双闸门）

任一启用即 dev 模式：

| 闸门 | 作用范围 | 命令 |
|---|---|---|
| **运行期** | 环境变量 `BUILD_DEV=true`（**严格字面**，不接受 `1` / `yes` / `TRUE`） | `BUILD_DEV=true ./build/catserver -config server/configs/local.yaml` |
| **编译期** | build tag `-tags devtools`（产出 `build/catserver-dev[.exe]`） | `bash scripts/build.sh --devtools` |

> `BUILD_DEV=1` / `BUILD_DEV=yes` / `BUILD_DEV=TRUE` **不会**启用 dev 模式 —— Story 1.6 钦定的严格语义。详见 [`server/internal/app/http/devtools/devtools.go`](internal/app/http/devtools/devtools.go) 第 50 行 `envBuildDev` 常量。

### 启动 banner

启用 dev 模式后 server 启动日志含两条 `WARN: DEV MODE ENABLED - DO NOT USE IN PRODUCTION`（main.go 启动顺序提示一条 + `devtools.Register` 注册 `/dev/*` 路由组时一条）。生产二进制**不应**看到这条日志。

### 当前 dev 端点

节点 1 阶段唯一 dev 端点：

```bash
curl http://127.0.0.1:8080/dev/ping-dev
# → {"code":0,"data":{"mode":"dev"}}
```

详见 [Story 1.6 #AC8](../_bmad-output/implementation-artifacts/1-6-dev-tools-框架.md)。

### 未来 dev 端点（占位）

节点 1 仅落地框架，业务 dev 端点由各 Epic 扩展：

- `POST /dev/grant-steps`（Epic 7 Story 7-5；调试步数）
- `POST /dev/force-unlock-chest`（Epic 20 Story 20-7；调试宝箱）
- `POST /dev/grant-cosmetic-batch`（Epic 20 Story 20-8；批量发装扮）

> 完整 request / response 体留给各自 story 自己写；本 README **不**预填。

### 生产部署 SOP

`devtools.IsEnabled()` 的两个触发源是 **OR 语义**——**任一**成立即开 dev 模式：

1. 编译期：`-tags devtools`（即 `bash scripts/build.sh --devtools` 产出的 `catserver-dev`）
2. 运行期：环境变量 `BUILD_DEV=true`（严格字面匹配）

因此生产部署必须**同时关闭两个触发源**，**不存在**"任一漏放仍能兜住"的双重保险：

- 生产二进制走 `bash scripts/build.sh`（**不带** `--devtools`），让 `forceDevEnabled=false`
- 部署环境**禁止**设置 `BUILD_DEV` 环境变量（不设 / 设为非 `"true"` 字面值都行）

`devtools.go` 包注释里的"双闸门（防御纵深）"指的是**路由注册闸门**（`Register` 在 `IsEnabled()==false` 时不挂 `/dev/*`）+ **请求时闸门**（`DevOnlyMiddleware` 再 check 一次）——两道闸门**查的是同一个 `IsEnabled()` 表达式**，抵御的是"挂了路由但运行期热切关闭 BUILD_DEV"这种边缘情形（实现成本为零的 in-depth 防御），**不**抵御"build tag 关闭但 `BUILD_DEV=true` 误设"——后者只能靠运维 SOP 双重确认两个触发源都为关闭态。详见 [`server/internal/app/http/devtools/devtools.go`](internal/app/http/devtools/devtools.go) 包注释 §双闸门。

---

## 目录结构

源自 [`docs/宠物互动App_Go项目结构与模块职责设计.md`](../docs/宠物互动App_Go项目结构与模块职责设计.md) §4，精简到 `server/` 子树。`✅` = 节点 1 已实装；`🚧` = 未来 Epic 落地。

```text
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
│  │  │  ├─ middleware/       # ✅ 节点 1（request_id / logging / recover / error_mapping）
│  │  │  ├─ handler/          # ✅ 节点 1（ping / version；业务 handler 各 Epic 加）
│  │  │  └─ devtools/         # ✅ 节点 1（Dev Tools 框架 Story 1.6）
│  │  └─ ws/                  # 🚧 Epic 10 Story 10-3 落地
│  ├─ domain/                 # 🚧 Epic 4+ 各 domain 子包（auth / user / pet / step / chest / cosmetic / compose / room / emoji）
│  ├─ service/
│  │  └─ sample/              # ✅ 节点 1（service 模板 + ctx cancel 测试 Story 1.5 / 1.9）；业务 service 各 Epic 加
│  ├─ repo/
│  │  ├─ mysql/               # 🚧 Epic 4 Story 4-2 落地
│  │  ├─ redis/               # 🚧 Epic 10 Story 10-2 落地
│  │  └─ tx/                  # 🚧 Epic 4 落地（txManager.WithTx，见 ADR-0007 §2.4）
│  ├─ infra/
│  │  ├─ config/              # ✅ 节点 1（YAML 加载 + 环境变量覆盖 Story 1.2）
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
│  └─ buildinfo/              # ✅ 节点 1（Story 1.4 ldflags 注入路径：Commit / BuiltAt）
├─ go.mod                     # ✅ 节点 1
└─ go.sum                     # ✅ 节点 1
```

> 分层职责（handler / service / repo）详见 [`docs/宠物互动App_Go项目结构与模块职责设计.md`](../docs/宠物互动App_Go项目结构与模块职责设计.md) §5.1-5.3。

---

## Troubleshooting

| # | 症状 | 原因 / 解决 |
|---|---|---|
| 1 | `bash scripts/build.sh` 后跑 `./build/catserver` 报 `bind: address already in use` | 8080 端口已被其他进程占用。**排查**：`lsof -i :8080`（macOS / Linux）/ `netstat -ano \| findstr 8080`（Windows）找占用进程。**绕过**：`CAT_HTTP_PORT=18090 ./build/catserver -config server/configs/local.yaml` |
| 2 | Windows 启动 server 时弹"Windows Defender Firewall - 允许专用网络访问" | binary 重编译产生新 hash，防火墙不识别。**解决**：维持 `bind_host: 127.0.0.1`（[`local.yaml`](configs/local.yaml) 默认；loopback 免防火墙）；**不要**改 `0.0.0.0` 除非真要外部访问。详见 [`docs/lessons/2026-04-24-config-path-and-bind-banner.md`](../docs/lessons/2026-04-24-config-path-and-bind-banner.md) Lesson 1 |
| 3 | `bash scripts/build.sh --race --test` 报 `ThreadSanitizer failed to allocate ... error code: 87` | Windows TSAN 内存分配限制（OS 级，非代码 bug）。**解决**：归 CI Linux runner 跑；本机想跑改用 WSL2 / 真 Linux 机器。详见 Story 1.5 #AC7 / Story 1.7 #T5.3 / Story 1.9 Debug Log |
| 4 | `curl /version` 返回 `{"commit":"unknown","builtAt":"unknown"}` | 没用 `bash scripts/build.sh` 编译（`-ldflags` 没注入 buildinfo），或用了 `go run ./cmd/server`（同样不注入）。**解决**：`bash scripts/build.sh && ./build/catserver -config server/configs/local.yaml`。详见 Story 1.4 §陷阱 #1 + Story 1.7 Dev Notes §1 |
| 5 | server 启动报 `config file not found: ...` | `-config` 路径错或 CWD 漂移让 `LocateDefault()` 找不到。**解决**：从仓库根（`C:\fork\cat`）跑命令；或显式传 `-config server/configs/local.yaml`。详见 [`docs/lessons/2026-04-24-config-path-and-bind-banner.md`](../docs/lessons/2026-04-24-config-path-and-bind-banner.md) Lesson 1 |
| 6 | Windows cmd.exe 跑 `BUILD_DEV=true ./build/catserver.exe` 不识别 | `VAR=val cmd` 是 bash 语法，cmd.exe 不支持。**解决**：用 Git Bash / WSL2（项目环境钦定，详见 [CLAUDE.md](../CLAUDE.md)）；或 cmd.exe 用 `set BUILD_DEV=true && .\build\catserver.exe -config server\configs\local.yaml` 两步法（普通构建即可，runtime 闸门会启用 dev 模式；如需 `catserver-dev.exe` 需先 `bash scripts/build.sh --devtools` 编译期闸门） |

---

## 工程纪律

> 短列表 + 跨链接锚点；**不**复制 ADR / lesson 全文。

- **节点顺序不可乱跳**：见 [`docs/宠物互动App_MVP节点规划与里程碑.md`](../docs/宠物互动App_MVP节点规划与里程碑.md) §3 / §5
- **写代码前必读**：[`CLAUDE.md`](../CLAUDE.md) + 当前节点对应的设计文档（[`docs/`](../docs/) 下 `宠物互动App_*.md` 七份）；Claude 个人 session memory 见 `~/.claude/projects/.../memory/MEMORY.md`
- **测试栈**：testify / sqlmock / miniredis / slogtest，见 [ADR-0001 §3](../_bmad-output/implementation-artifacts/decisions/0001-test-stack.md)
- **错误处理**：AppError + 三层映射（repo → service → handler），见 [ADR-0006](../_bmad-output/implementation-artifacts/decisions/0006-error-handling.md)
- **ctx 必传**：service / repo 第一参数 `ctx context.Context`；handler 用 `c.Request.Context()`；repo 用 `*WithContext`；tx fn 用 `txCtx`，见 [ADR-0007](../_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md)
- **error envelope 单一生产者**：所有 envelope 必须经 `ErrorMappingMiddleware`，业务代码用 `c.Error(apperror.Wrap(...))` 而非 `response.Error(c, ...)`，见 [`docs/lessons/2026-04-24-error-envelope-single-producer.md`](../docs/lessons/2026-04-24-error-envelope-single-producer.md)
- **middleware canonical decision key**：见 [`docs/lessons/2026-04-24-middleware-canonical-decision-key.md`](../docs/lessons/2026-04-24-middleware-canonical-decision-key.md)
- **资产事务**：开箱 / 合成 / 穿戴 / 加房 / 游客初始化必须包 MySQL 事务（节点 1 暂无落地，见 [`docs/宠物互动App_数据库设计.md`](../docs/宠物互动App_数据库设计.md) §8）
- **幂等键**：`/chest/open` 和 `/compose/upgrade` 用 `idempotencyKey` 存 Redis（节点 1 暂无落地）
- **配置路径**：CWD-relative，启动时显式传 `-config`，见 [`docs/lessons/2026-04-24-config-path-and-bind-banner.md`](../docs/lessons/2026-04-24-config-path-and-bind-banner.md) Lesson 1
- **slog 字段惯例**：`error_code` / `ctx_done` / `user_id` 缺省即"负向信号"省略字段，**不**显式写 `false` / 空串
- **公共 artifact 质量门槛**：sample / README / lesson 等公共文档每条命令必须 100% 可复制粘贴，见 [`docs/lessons/2026-04-24-sample-service-nil-dto-and-slog-test-group.md`](../docs/lessons/2026-04-24-sample-service-nil-dto-and-slog-test-group.md)

---

## 维护说明

本 README 是节点 1 收官 story（Story 1.10）的产出。**每个 Epic 完成时如有命令 / 配置 / 流程变化，必须回头同步更新本 README**（属各 Epic 文档同步 story 的范畴，如 Epic 3 Story 3-3 / Epic 6 Story 6-3 / ... / Epic 36 Story 36-3）。

**典型同步触发**：

| Epic 完成 | 本 README 要改的章节 |
|---|---|
| Epic 4（Auth + MySQL） | `## 依赖`：MySQL 8.0 从"未来"改"必需"；`## 配置`：加 `server.mysql.dsn` 字段 + `CAT_MYSQL_DSN` 环境变量；`## 跑测试`：integration 测试段从"暂无"改"已落地"；`## 目录结构`：`internal/repo/mysql/` / `migrations/` 标 ✅ |
| Epic 7（Step） | `## Dev mode`：dev 端点列表加 `POST /dev/grant-steps` |
| Epic 10（Redis + WS） | `## 依赖`：Redis 6+ 从"未来"改"必需"；`## 目录结构`：`internal/app/ws/` / `internal/repo/redis/` 标 ✅ |
| Epic 20（Chest） | `## Dev mode`：dev 端点列表加 `POST /dev/force-unlock-chest` / `POST /dev/grant-cosmetic-batch` |
| Epic 36（MVP 整体收官） | 全 README 一遍体检；🚧 标记应已全部消除变 ✅；本段提示后续生产化 / 部署 SOP 单独 PR |

> 维护原则与 [`epics.md` §Story 1.10](../_bmad-output/planning-artifacts/epics.md) 钦定的"同步原则"一致：README 与代码不分裂。
