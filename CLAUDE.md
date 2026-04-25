# CLAUDE.md

## 状态：重启中（2026-04-23）

旧架构（MongoDB + TOML + P2 分层）和旧 server 实装已**全部放弃**。新设计文档包是**唯一权威来源**，全部位于 `docs/宠物互动App_*.md`：

1. `docs/宠物互动App_总体架构设计.md` — 总体架构、模块边界、产品规则
2. `docs/宠物互动App_MVP节点规划与里程碑.md` — 10 节点开发顺序与验收
3. `docs/宠物互动App_V1接口设计.md` — REST + WebSocket 协议
4. `docs/宠物互动App_数据库设计.md` — MySQL 表结构、索引、事务边界
5. `docs/宠物互动App_时序图与核心业务流程设计.md` — 关键链路时序
6. `docs/宠物互动App_Go项目结构与模块职责设计.md` — server 工程结构与分层
7. `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` — iOS 工程结构

**写任何 server 代码前，必须先读 1 / 6 / 4 / 3 / 5 这五份。** iOS 任务读 1 / 7 / 3。

## Tech Stack（新方向）

- **语言**：Go（server）/ Swift + SwiftUI（iOS）
- **HTTP 框架**：Gin / Echo / Chi 任选其一（建议 Gin）
- **ORM / DB 驱动**：GORM 或 sqlx
- **主存储**：**MySQL 8.0**（取代旧方案的 MongoDB）
- **缓存与实时态**：Redis
- **配置格式**：**YAML**（取代旧方案的 TOML），支持环境变量覆盖
- **协议**：REST + WebSocket
- **形态**：模块化单体（**不**拆微服务）
- **iOS**：MVVM + UseCase + Repository，HealthKit / CoreMotion 接入

## Repo Separation（重启阶段过渡态）

三个目录（按运行时端 + 当前真实状态）：

- **`server/`**（Go server，新方向）— Epic 1 已 done
- **`iphone/`**（iPhone App，新方向；Story 2.2 起在此目录落地）— 由 ADR-0002 §3.3 选定
- **`ios/`**（旧产物归档，含 `Cat.xcodeproj` / `CatPhone/` / `CatShared/` / `CatWatch*/` 等）— 重启阶段**整个不动**；watch 仍可在 `ios/Cat.xcodeproj` 内打开 / build；未来恢复 watchOS 或废弃旧产物时另做迁移决策（参见 ADR-0002 §5.3 四条路径）

**端独立原则**：server 测试自包含，不依赖 iPhone App / watch 真机；真机联调类工作单独排期，不塞业务节点。跨端契约通过 `docs/宠物互动App_V1接口设计.md` 统一同步，不通过共享代码。

**iPhone 端工程目录由 ADR-0002 锁定**：`_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md`。该 ADR 是 Story 2.2 / 2.7 等 iPhone 实装 story 的唯一权威。

## Build & Test

Server 是 `server/` 下的 Go 工程。build 脚本（入口 `./cmd/server/`，`-ldflags` 注入 `internal/buildinfo.Commit` / `.BuiltAt` 供 `/version` 端点用）：

```bash
bash scripts/build.sh                      # vet + build → build/catserver[.exe]
bash scripts/build.sh --test               # 加跑单测 go test -count=1 ./...
bash scripts/build.sh --race --test        # 加 -race 同时作用于 build 和 test
bash scripts/build.sh --test --coverage    # 产出 build/coverage.out（--coverage 要求 --test）
bash scripts/build.sh --integration        # 单跑 -tags=integration 集成测试
bash scripts/build.sh --devtools           # 加 -tags devtools → build/catserver-dev[.exe]
```

二进制输出：`build/catserver`（Windows 自动带 `.exe`，靠 `$(go env GOEXE)`）；`--devtools` 出 `build/catserver-dev`。`--devtools` 与 `--integration` 互斥。

写完或改完 Go 代码后跑 `bash scripts/build.sh --test` 验证。脚本契约来源：`_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.5 + `_bmad-output/implementation-artifacts/1-7-重做-scripts-build-sh.md`。

## 节点 1 之后的目录形态（target）

按 `docs/宠物互动App_Go项目结构与模块职责设计.md` §4：

```
server/
├─ cmd/server/main.go
├─ configs/{local,dev,staging,prod}.yaml
├─ migrations/                  # MySQL DDL 文件
├─ internal/
│  ├─ app/{bootstrap,http,ws}/
│  ├─ domain/{auth,user,pet,step,chest,cosmetic,compose,room,emoji}/
│  ├─ service/
│  ├─ repo/{mysql,redis,tx}/
│  ├─ infra/{config,db,redis,logger,clock,idgen}/
│  └─ pkg/{errors,response,auth,utils}/
└─ docs/architecture/
```

## 工作纪律

- **节点顺序不可乱跳**：`docs/宠物互动App_MVP节点规划与里程碑.md` §3 / §5 定义了 10 节点的依赖关系，按顺序推进。节点 1（App 与 Server 可运行）必须先完成。
- **资产类操作必须事务**：开箱、合成、穿戴、加入房间、游客登录初始化 —— 见 `数据库设计.md` §8，全部必须包在 MySQL 事务里。
- **幂等键**：`/chest/open` 和 `/compose/upgrade` 必须支持 `idempotencyKey`，存 Redis（`idem:{userId}:{apiName}:{key}`）并设 TTL。
- **状态以 server 为准**：步数余额、宝箱状态、背包归属、合成结果、房间成员关系都以 server 响应为最终态，客户端只能做本地预展示。
- **错误码统一**：见 `V1接口设计.md` §3，repo 返回底层错误 → service 转业务错误 → handler 映射统一响应结构。
- **ctx 必传**：service / repo 所有导出函数第一参数 `ctx context.Context`；handler 从 `c.Request.Context()` 取 ctx 向下传（**不**把 `*gin.Context` 直接当 ctx 用，它的 Done() 是 nil channel）；repo 调 DB / Redis 必用 `*WithContext` 方法；`txManager.WithTx(ctx, fn)` 里 fn 内所有 repo 调用用 **`txCtx`** 而非外层 ctx。见 ADR-0007。

## 开 session 起手式

每次新 session：
1. 读本文件
2. 读 `docs/宠物互动App_总体架构设计.md` + 当前节点对应的设计文档
3. 读 `MEMORY.md`
4. 看当前节点状态再开工
