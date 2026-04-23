---
stepsCompleted: ["step-01-validate-prerequisites", "step-02-design-epics", "step-03-create-stories", "step-04-final-validation"]
inputDocuments:
  - _bmad-output/planning-artifacts/prd.md
  - docs/宠物互动App_总体架构设计.md
  - docs/宠物互动App_MVP节点规划与里程碑.md
  - docs/宠物互动App_V1接口设计.md
  - docs/宠物互动App_数据库设计.md
  - docs/宠物互动App_时序图与核心业务流程设计.md
  - docs/宠物互动App_Go项目结构与模块职责设计.md
  - docs/宠物互动App_iOS客户端工程结构与模块职责设计.md
userConstraints:
  - 同一节点的 iPhone 端和 Server 端拆独立 Epic
  - 每节点末尾设"对齐 Epic"做联调验收，标志该节点真正完成
---

# cat - Epic Breakdown

## Overview

本文档汇总 cat（宠物互动 App）MVP 全部 Epic 与 Story，依据来自补充 PRD 与七份设计文档（见 frontmatter `inputDocuments`）。

Epic 拆分遵循两条用户约束：
1. 同一节点的 iPhone 端工作和 Server 端工作拆成独立 Epic
2. 每节点末尾安排一个"对齐 Epic"，验证两端真实联调通过，标志节点完成

## Requirements Inventory

### Functional Requirements

**身份与账号（Auth）**
- FR1: App 启动后自动发起游客登录，无显式登录页
- FR2: 游客身份基于客户端 Keychain 持久化的 `guestUid` 在服务端唯一识别同一账号
- FR3: 游客账号可后续绑定微信，绑定后保留原 `user_id` 与全部已有数据
- FR4: Server 签发 token，客户端持久化并自动注入后续 HTTP / WebSocket 请求

**宠物（Pet）**
- FR5: 用户首次登录时服务端在事务中发放一只默认猫咪
- FR6: 客户端按本地运动状态展示猫咪三种动作：rest / walk / run
- FR7: 客户端可向服务端同步当前宠物展示态（用于关键时机记录，不要求高频）

**步数（Step）**
- FR8: 客户端按"当日累计步数"上报，服务端按差值入账（拒绝倒退或重复）
- FR9: Server 维护步数账户，包含 total / available / consumed 三档与版本号
- FR10: 客户端可读取当前步数账户余额

**宝箱（Chest）**
- FR11: 用户同时只存在 1 个当前宝箱
- FR12: 宝箱倒计时 10 分钟，到期后服务端视为可开启；用户未开启则一直保留
- FR13: 开箱固定消耗 1000 步；扣减步数 + 创建奖励实例 + 写开箱日志 + 创建下一轮宝箱在同一事务
- FR14: 开箱奖励是 1 个新装扮实例，按配置加权抽取
- FR15: 开箱接口支持 `idempotencyKey` 防重；幂等键存 Redis（带 TTL）

**装扮（Cosmetic）**
- FR16: 玩家持有的每一件装扮都是唯一实例（`user_cosmetic_items.id`），同种配置可被持有多件
- FR17: 客户端可查询装扮配置目录（catalog）
- FR18: 客户端可查询自己的背包，含按配置聚合 + 实例列表两层信息
- FR19: 单件实例可穿戴；同槽位若已有装备需先卸下旧装备（事务）
- FR20: 单件实例可卸下；卸下后状态回 `in_bag`
- FR21: 同一槽位最多绑定 1 件实例；同一实例同一时间只能装备一次

**装扮渲染配置（Render Config）**
- FR42: 服务端在 `cosmetic_items` 表新增 `render_config JSON` 字段，存储相对槽位锚点的 `offset_x / offset_y / scale / rotation / z_layer` 等渲染参数（结构预留 per-state 扩展位）
- FR43: 客户端在 `render_config` 完整时按参数在宠物对应槽位渲染装扮图像，使每件装扮在猫身上的位置 / 大小 / 朝向有可分辨差异
- FR44: `render_config` 在 MVP 阶段通过 migration / seed JSON 维护并随 `catalog` / `inventory` 接口下发；客户端可拉取最新配置，无需 App 重发版
- FR45: **穿戴视觉降级** —— 当装扮的 `render_config` 未配置或 `asset_url` 为空时，客户端按文字降级渲染（道具名 + 槽位标签），保证节点 9（穿戴）在节点 10（渲染配置）上线前可独立验收

**合成（Compose）**
- FR22: 玩家手动选择 10 个同品质实例发起合成，不要求同部位 / 同配置 id
- FR23: 合成产出 1 个高一阶品质随机装扮实例（common→rare→epic→legendary；legendary 不可续合）
- FR24: 合成事务：锁定 10 个实例 → 校验归属/状态/品质 → 标记 consumed → 创建奖励实例 → 写合成日志 + 材料明细
- FR25: 合成接口支持 `idempotencyKey` 防重
- FR26: 客户端可查询合成概览（各品质可用数量与是否可合成）

**房间（Room）**
- FR27: 房间最多 4 人
- FR28: 用户同时只能加入 1 个房间
- FR29: 用户可创建房间并自动加入（事务）
- FR30: 用户可通过房间号加入存在且未满的房间（事务）
- FR31: 用户可主动退出房间；最后一人离开时房间状态置为 closed
- FR32: 不提供踢人 / 房主权限管理
- FR33: 客户端可查询当前所在房间与指定房间详情（含成员列表 + 简要宠物信息）

**实时通信（Realtime / Emoji）**
- FR34: 用户进入房间后建立 WebSocket 连接
- FR35: 连接建立成功后服务端推送当前房间快照
- FR36: 成员加入 / 离开时通过 WS 广播给在线成员
- FR37: 房间成员的猫咪当前动作状态可被房间内其他成员看到（通过快照或 WS 同步）
- FR38: 用户在房间内点击自己的猫可发送系统表情，广播给其他成员
- FR39: WS 心跳：客户端 ping，服务端 pong；断线后客户端可重连并恢复在线态

**分享（Sharing）**
- FR40: 可生成房间分享链接
- FR41: 通过链接可拉起 App 并解析房间号，已登录情况下尝试加入对应房间

### NonFunctional Requirements

**事务一致性**
- NFR1: 资产操作必须事务原子完成：游客登录初始化 / 开箱 / 合成 / 穿戴 / 加入房间 / 退出房间
- NFR2: 装扮实例状态必须与穿戴关系保持一致（`equipped` 状态必须存在对应 `user_pet_equips` 记录）
- NFR3: 状态以 Server 为最终权威：步数余额、宝箱状态、背包归属、合成结果、房间成员关系
- NFR4: 客户端可做本地预展示（倒计时、猫动画、表情飞出），但任何资产变化必须以 Server 响应为准

**幂等与防重**
- NFR5: 资产消耗类接口（开箱、合成）支持 `idempotencyKey`，结果存 Redis 带 TTL，键格式 `idem:{userId}:{apiName}:{key}`
- NFR6: 步数同步天然防重（按累计差值入账，重复同步增量为 0）

**安全**
- NFR7: 游客 `guestUid` 必须存 iOS Keychain（不存 UserDefaults）
- NFR8: 除登录接口外，所有 API 通过 `Authorization: Bearer <token>` 鉴权
- NFR9: 同一登录身份只能绑定一个账号（`UNIQUE(auth_type, auth_identifier)`）
- NFR10: 用户同一时间只在一个房间（`UNIQUE(user_id)` on `room_members`）
- NFR11: 一件装扮实例同一时间只能装备一次（`UNIQUE(user_cosmetic_item_id)` on `user_pet_equips`）

**性能与高频态分离**
- NFR12: 房间在线态 / 心跳 / 幂等键 / 限频控制放 Redis，不依赖 MySQL 高频读写
- NFR13: 资产、关系、配置类数据必须以 MySQL 为主存储

**可观测性**
- NFR14: 所有请求带 `request_id`，结构化日志包含 user_id / api_path / latency / business_result / error
- NFR15: 关键资产动作（登录初始化 / 步数同步 / 开箱 / 合成 / 房间生命周期 / WS 连接断开）必须打结构化业务日志
- NFR16: 至少预留指标：接口 QPS / 接口耗时 / 错误率 / 开箱成功次数 / 合成成功次数 / 当前在线房间数 / 当前在线连接数

**可维护性**
- NFR17: 严格分层：handler 不操作 DB / 不做事务；service 不依赖 HTTP 框架类型；repo 不决定业务规则
- NFR18: 错误三层映射：repo 返回底层错误 → service 转业务错误 → handler 映射统一响应结构（按 V1接口设计 §3 错误码表）
- NFR19: 模块化单体，按领域拆模块（auth / user / pet / step / chest / cosmetic / compose / room / emoji / realtime），不拆微服务

**可测性**
- NFR20: server 测试自包含，不依赖 iOS / watchOS 真机；所有测试通过 `go test ./...` 跑
- NFR21: 单元测试优先覆盖：合成材料校验 / 宝箱状态判断 / 房间人数判断 / 步数增量判断 / `CanUpgradeRarity`
- NFR22: 集成测试优先覆盖核心事务全流程：登录初始化 / 开箱 / 合成 / 房间生命周期

**装扮资产远端化**
- NFR23: 装扮的 `asset_url` 与 `render_config` 都从 server 拉取，调整美术 / 渲染参数无需 App 重发版
- NFR24: `render_config` 数据结构应支持后续按宠物状态（rest / walk / run）做偏移微调（MVP 阶段可仅实装基础参数，状态分支留扩展位）

### Additional Requirements

**Greenfield 起点**
- AR1: 当前 `server/` 目录基本为空，节点 1 需要从零搭建。**按需引入原则**：节点 1 仅做 `cmd/server/main.go` 入口 + 配置加载 + Gin + ping 接口 + Server 测试基础设施 + 重做 build.sh。**MySQL 推迟到节点 2 接入**（首次需要持久化）；**Redis + WebSocket 网关推迟到节点 4 接入**（首次需要 presence / 实时态）。避免节点 1 范围爆炸
- AR2: 当前 `scripts/build.sh` 仍引用旧架构（`cmd/cat`、`docs/api/openapi.yaml`、`scripts/check_time_now.sh`），节点 1 实装时一并重做以匹配新结构

**技术栈与框架**
- AR3: HTTP 框架：Gin（推荐），或 Echo / Chi
- AR4: ORM / DB 驱动：GORM 或 sqlx
- AR5: 主存储：MySQL 8.0，InnoDB，charset utf8mb4
- AR6: 缓存与实时态：Redis
- AR7: 配置：YAML 格式，支持 `local / dev / staging / prod` 多环境，环境变量可覆盖

**目录结构（节点 1 之后的目标形态）**
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

**基础设施 / 中间件**
- AR8: 数据库 schema 通过 `migrations/*.sql` 顺序管理；每个新表对应一个迁移文件
- **AR9a**: 节点 1 中间件 —— `request_id` / `recover` / `logging`（落 E1）
- **AR9b**: 节点 2 中间件 —— `auth`（Bearer token 校验）/ `rate_limit`（至少保护登录、开箱、合成等敏感接口）（落 E4）
- AR10: 抽象事务管理器：`txManager.WithTx(ctx, fn error) error` 作为统一事务入口
- AR11: WebSocket 网关与 HTTP 层分离（`internal/app/ws`），但共用业务 service 层

**iOS 客户端基础设施**
- AR12: Swift + SwiftUI，MVVM + UseCase + Repository + System Adapter 五层结构
- AR13: HealthKit 用于读取 iPhone 系统步数
- AR14: CoreMotion 用于识别 stationary / walking / running 三态
- AR15: Keychain 持久化 `guestUid` 与 `token`
- AR16: `URLSession` 作为 HTTP；`URLSessionWebSocketTask` 作为 WebSocket
- AR17: HealthKit / CoreMotion 权限按需触发申请，不在 App 启动时一次性弹完

**预置数据**
- AR18: `cosmetic_items` 必须预置足够广度的配置集合，保证开箱 / 合成 demo 不会反复产出同一件。**MVP 最小数量约束**：
  - common ≥ **8 件**，至少覆盖 4 个不同槽位
  - rare ≥ **4 件**
  - epic ≥ **2 件**
  - legendary ≥ **1 件**
  - 在 Epic 20（宝箱 server）的 cosmetic seed story acceptance 中硬性写入
  - **URL 字段约束**：`icon_url` 与 `asset_url` 必须为可访问 URL（MVP 阶段可用 placeholder，例如 `https://placehold.co/128x128?text=Hat`），不可为空字符串。否则 demo 时开箱 popup / 仓库页会显示破图
- AR19: `emoji_configs` 必须预置最小系统表情集合（≥ 4 个，覆盖典型情绪）；`asset_url` 同样必须可访问

**ID / 数据规则**
- AR20: 全部业务表使用 `BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT`
- AR21: 对前端返回 ID 时统一字符串，避免大整数客户端处理不一致

**仓库与协作纪律**
- AR22: 三端独立目录：`server/` (Go) / `ios/` (Swift) / `watch/` (watchOS，本 MVP 不涵盖)
- AR23: 跨端契约通过 `docs/宠物互动App_V1接口设计.md` 同步，不通过共享代码
- AR24: 节点顺序按 `docs/宠物互动App_MVP节点规划与里程碑.md` §5 推进，不可跳序

**用户约束（影响 Epic 拆分结构）**
- AR25: 同一节点的 iPhone 端和 Server 端工作必须拆独立 Epic
- AR26: 每个节点完成时设一个"对齐 Epic"做端到端联调验收，作为该节点真正 Done 的判据
- **AR27**: server + iOS 必须各自在**节点 1** 内建立测试基础设施（Server: mock 库 / test fixture / `go test` 本地跑法 / CI 配置；iOS: XCTest + Mock 框架 / CI 跑法）。不允许等到首次需要测试时才临时补
  - **Done 标准**：不能只是 `func TestNothing(t *testing.T) { t.Log("ok") }` 这种 dummy。必须**至少跑通一条业务相关的 mock 单元测试**：
    - Server 端：例如 mocked `StepRepo` + 一段调用 service 层的步数差值校验逻辑（即使该 service 还没真正实装，可以是 placeholder + mock 调用链跑通）
    - iOS 端：例如 mocked `UseCase` + 一个 ViewModel 状态切换的 XCTest（验证 mock 注入链路 + 状态流转）
- **AR28**: **跨端契约先于实装锚定** —— 每个 Server 业务 Epic 的**第一条 story 必须是"接口契约最终化"**：把该 Epic 涉及的 REST endpoint（request/response schema）+ WS message schema 落到 `docs/宠物互动App_V1接口设计.md`，作为契约锚点。然后对应的 iOS Epic 才允许并行开工，避免后期集成时接口对不上导致返工。对齐 Epic 的 Story X.3 仅做"实装与契约的最终一致性核对"，**不再承担首次落契约的职责**

### UX Design Requirements

无 —— 设计文档包不含独立 UX 规范。`docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §14 列出了通用 UI 组件建议（PetCardView / ChestCountdownView / InventoryGridView 等），但属于工程组件清单，不是 UX-DR 级别的设计要求。

如需正式 UX 规范（设计 token / 配色 / 可访问性 / 响应式），需要单独走 `bmad-create-ux-design` 工作流补齐。

### FR Coverage Map

| FR | Epic | 说明 |
|---|---|---|
| FR1 自动游客登录 | E4, E5 | server 接口 + iOS 自动调用 |
| FR2 guestUid Keychain 持久识别 | E4, E5 | server `user_auth_bindings` + iOS Keychain |
| **FR3 微信绑定** | **Post-MVP** | 节点规划"暂不做"，结构预留在 E4 的 `user_auth_bindings` 表 |
| FR4 token 签发与持久化 | E4, E5 | |
| FR5 默认猫咪发放 | E4 | 在游客登录初始化事务中 |
| FR6 本地猫动作展示 | E8 | iOS 本地驱动 |
| FR7 宠物状态同步 API | E14, E15 | server `state-sync` + iOS 上报 |
| FR8 步数差值入账 | E7 | server 按累计差值 |
| FR9 步数账户三档 | E7 | total / available / consumed |
| FR10 步数账户读取 | E7, E8 | server 接口 + iOS 显示 |
| FR11-15 宝箱链路 | E20, E21 | 单宝箱 + 倒计时 + 开箱事务 + 抽奖 + 幂等 |
| FR16-18 装扮配置 + 背包查询 | E23, E24 | catalog / inventory 接口 + 仓库页 |
| FR19-21 穿戴 / 卸下 + 一致性约束 | E26, E27 | 节点 9 |
| **FR42** 服务端 render_config 字段 | **E29** | 节点 10，A 方案：`cosmetic_items.render_config JSON` |
| **FR43** 客户端按 render_config 渲染 | **E30** | 节点 10 |
| **FR44** render_config 通过 seed/migration 维护并下发 | **E29** | MVP 不做 admin 接口 |
| **FR45** 穿戴视觉降级到文字 | **E27** | 节点 9 iOS，让节点 9 不依赖节点 10 即可验收 |
| FR22-26 合成 | E32, E33 | 节点 11 |
| FR27-33 房间 CRUD + 详情 | E11, E12 | 节点 4 业务侧 |
| FR34（WS 连接）, FR35（快照框架）, FR39（心跳） | E10, E12 | E10 提供 WS 网关基础设施；E12 是 iOS 客户端 |
| FR35（快照内容）, FR36（成员广播） | E11, E12 | E11 用 E10 的 broadcast primitive 发出业务事件 |
| FR37 房间内宠物状态同步 | E14, E15 | 节点 5 |
| FR38 表情广播 | E17, E18 | 节点 6 |
| FR40-41 房间分享链接 | E35 | 节点 12，仅 iOS（server 复用 `/rooms/{id}/join`） |

**NFR / AR 横切映射**：
- 基础设施核心（AR1, AR2, AR3, AR7, AR9 部分, AR10）→ 落 **E1**（Server 脚手架）
- 测试基础设施（**AR27**, NFR20-22）→ 拆分落 **E1（Server）+ E2（iOS）** 各自的"测试基础设施" story
- MySQL 接入（AR4, AR5, AR8）→ 推迟到 **E4**（节点 2 首次需要持久化）
- Redis 接入（AR6）→ 推迟到 **E10**（节点 4 首次需要 presence）
- WebSocket 网关（AR11）→ **E10**（节点 4 拆出的基础设施 epic）
- iOS 基础设施（AR12, AR15, AR16, AR17）→ **E2**；HealthKit / CoreMotion（AR13, AR14）→ **E8**
- 横切纪律（NFR1-5 事务 / NFR6 幂等 / NFR12-13 高频态分离 / NFR17-19 分层）→ 在所有相关业务 Epic 内自动适用
- 装扮资产远端化（NFR23-24）→ 横切到 **E29 + E30**
- 预置数据 emoji（AR19）→ **E17** seed story；预置数据 cosmetic（AR18 含数量约束）→ **E20** seed story
- 渲染配置 seed（FR44）→ **E29** seed story
- 用户约束（AR25-26）→ 已通过本 Epic 列表本身满足

## Epic List

> **总计 36 个 Epic，覆盖 12 个 MVP 节点。** 每节点拆分为 `Server` + `iOS` + `对齐` 三层；**节点 4 的 Server 端进一步拆为 WS 网关基础设施 + 房间业务两个 Epic**（避免单 Epic 过胖）；节点 12（分享）无新 server 工作故省略 Server Epic。
>
> **治理简化**：**对齐 Epic 不强制 retrospective**。Sprint Planning 生成时把对齐 Epic 的 `epic-{N}-retrospective` 标 `optional`（仅业务 Epic 强制）。
>
> **🟢 对齐 Epic 标准三段式骨架**（所有"对齐 Epic"统一按此结构在 Step 3 拆 story）：
> - **Story X.1 · 跨端集成测试场景** —— 编写并跑通该节点的 E2E 测试用例（覆盖该节点全部"验收标准"列项）
> - **Story X.2 · 节点 demo 验收** —— 按节点 acceptance criteria 走一遍真实流程（demo 录制可选），逐项打勾或登记"延期到节点 X+1"
> - **Story X.3 · 文档同步与 tech debt 登记** —— 接口契约（V1接口设计）/ 数据库设计 / known issues 同步到对应文档；登记任何未来需还的"快路径决定"
>
> **🧪 Layer 1 — Definition of Done（每 story 必含测试 AC）**：
> 每个含**生产代码**的 story，AC 必须显式包含测试项：
> - **单元测试**：列出至少 3 个 case（1 个 happy path + 至少 1-2 个边界 / 异常 / 失败 case）
> - **集成测试**（如适用，跨服务 / 跨表 / 走 DB 或 Redis 的逻辑）：端到端 API → DB / Redis / WS → response 链路
> - 例外：spike / 配置 / 文档同步 / build 脚本类 story 不强制单元测试，但 deliverable 必须明确
> - Layer 1 是**所有业务 story 都要遵守的基线**
>
> **🧪 Layer 2 — 核心事务专属集成测试 story**（NFR22 + §8 列出的 5 个最复杂事务，各加一条独立的深度集成测试 story）：
> | Epic | 事务 | 专属 story |
> |---|---|---|
> | E4 | 游客登录初始化（5 张表事务） | Story 4.7 |
> | E11 | 房间生命周期（创建 + 加入 + 退出 + 关闭） | Story 11.9 |
> | E20 | 开箱事务（扣步数 + 抽奖 + 发实例 + 写日志 + 刷下一轮） | Story 20.9 |
> | E26 | 穿戴事务（含同槽换装回滚） | Story 26.5 |
> | E32 | 合成事务（锁 10 实例 + 校验 + consumed + 加权产出 + 双日志） | Story 32.5 |
> 这些 story 专门覆盖 Layer 1 范围之外的：失败回滚 / 并发 / 边界值 / 幂等冲突等深度场景

### 节点 1：App + Server 可运行
> **顺序**：E1 + E2 可并行 → E3
- **Epic 1 · Server** · 工程脚手架 + ping 接口 + Server 测试基础设施 + Dev Tools 框架
  - **首条 story（spike）**：mock 库选型（sqlmock vs gomock vs testify mock；miniredis；测试容器策略 dockertest / testcontainers）
  - **范围**：`cmd/server/main.go` + 配置加载 (YAML) + Gin + ping 接口 + `/version` 接口（返回 git commit short hash + build time）+ 重做 `scripts/build.sh` + Server 测试基础设施（按选型搭建 mock + fixture + 本地 `go test` + race + cover + CI 配置）+ 中间件 `request_id` / `recover` / `logging` + **Dev Tools 框架**（`/dev/*` 路由组 + 通过 build flag `BUILD_DEV=true` 才注册 + 中间件强制非生产环境校验）
  - **不在范围**（按需引入原则）：MySQL → E4；Redis → E10；WebSocket 网关 → E10；`auth` / `rate_limit` 中间件 → E4
  - **Dev Tools 端点说明**：本 Epic 只做**框架**，具体 dev 端点（如 `/dev/grant-steps` / `/dev/force-unlock-chest`）由对应业务 Server Epic 增加（E7 / E20）
  - **FRs covered:** （基础设施 Epic，无 FR）
  - **落实**：AR1, AR2, AR3, AR7, **AR9a**, AR27, NFR14, NFR15, NFR17-19, NFR20-22
- **Epic 2 · iOS** · iOS 工程脚手架 + 首页骨架 + 导航架构 + ping 调用 + 测试基础设施
  - **首条 story（spike）**：iOS mock 框架选型（XCTest only / Mockingbird / Cuckoo / Swift Mocks）+ 决定 `ios/` 目录改造方案（复用 Cat.xcodeproj 还是新建）
  - **范围**：
    - **首页骨架与信息架构定稿**（占位区块：猫展示区 / 步数显示位 / 宝箱位 / 进房间按钮 / 仓库按钮 / 合成按钮 + 角落版本号）—— 后续 Epic 是"在已存在的占位上填内容"，不再各自重做主界面
    - **导航架构决定**（建议：主界面 + 全屏 Sheet 进入房间 / 仓库 / 合成；定 NavigationStack vs Sheet 边界）
    - **基础错误 UI 框架**（统一 `Toast` / `AlertOverlay` / `RetryView` 组件 + APIClient 401/网络错误 / 通用业务错误的统一处理钩子）
    - SwiftUI 入口 + `APIClient` + ping 调用
    - 显示版本号（小字角落，调用 `/version` 拿 server commit + 显示 App build hash）
    - iOS 测试基础设施（按选型搭建 XCTest + Mock + CI 跑法）
  - **FRs covered:** （基础设施 Epic）
  - **落实**：AR12, AR15, AR16, AR17, AR27, NFR20
- **Epic 3 · 对齐** · 端到端 ping 联调验收

### 节点 2：默认游客登录
> **顺序**：E4 → E5（E5 可基于 V1 接口设计文档预先做 Keychain + auto-login 框架，待 E4 接口可调用后联调）→ E6
- **Epic 4 · Server** · MySQL 接入 + auth/rate_limit 中间件 + 游客登录接口 + 首次初始化事务
  - **首条 story（契约）**：接口契约最终化 —— `POST /auth/guest-login` + `GET /me` 的 request / response schema 落到 V1接口设计文档；E5 (iOS) 才允许并行开工
  - **范围**：**首次引入 MySQL**（GORM/sqlx 选型 + 连接池 + `migrations/` 框架 + users / user_auth_bindings / pets / user_step_accounts / user_chests 五张表 migration）+ `auth` 中间件 + `rate_limit` 中间件 + 游客登录接口 + 初始化事务（user / binding / 默认 pet / step 账户 / 首个 chest）+ token 签发
  - **FRs covered:** FR1, FR4, FR5；NFR1, NFR3, NFR9
  - **落实**：AR4, AR5, AR8, **AR9b**, AR10
- **Epic 5 · iOS** · 自动游客登录与会话持久化
  - **范围**：guestUid 写 Keychain（不存 UserDefaults）+ 启动时自动调用 `/auth/guest-login` + token 存 Keychain + APIClient interceptor 自动注入 `Authorization: Bearer` header + 无效 token 静默重新登录
  - **FRs covered:** FR1, FR2, FR4；NFR7（Keychain）
- **Epic 6 · 对齐** · 启动即得身份联调

### 节点 3：小猫动作 + 步数上报
> **顺序**：E7 + E8 可并行（基于 V1 接口设计的 `step.sync` schema 锚定契约）→ E9
- **Epic 7 · Server** · 步数同步接口与账户记账 + dev 步数发放
  - **首条 story（契约）**：接口契约最终化 —— `POST /steps/sync` + `GET /steps/account` schema 落到 V1接口设计文档
  - **范围**：步数同步接口（按累计差值入账）+ 步数账户读取接口 + **dev 端点 `POST /dev/grant-steps`**（build flag gated，让 demo 时不必真走 1000 步）
  - **FRs covered:** FR8, FR9, FR10；NFR6
- **Epic 8 · iOS** · 小猫三态本地展示与 HealthKit / CoreMotion 接入
  - **范围**：HealthKit 步数读取 + CoreMotion 状态识别 + 状态机映射 stationary/walking/running → rest/walk/run + Sprite 三态动画切换 + 步数同步触发器（启动 / 回前台 / 定时 / 开箱前）
  - **FRs covered:** FR6, FR10；AR13, AR14
- **Epic 9 · 对齐** · 步数链路 + 本地猫动作联调

### 节点 4：基础房间功能（**Server 拆为两个 Epic**）
> **顺序**：E10（WS 基础设施）→ E11 + E12 可并行 → E13
- **Epic 10 · Server** · WebSocket 网关基础设施 + Redis 接入 + 心跳框架
  - **首条 story（契约）**：接口契约最终化 —— WS 连接 URL（`/ws/rooms/{roomId}?token=xxx`）+ `room.snapshot` / `ping` / `pong` 消息 schema 落到 V1接口设计文档；E11 + E12 才允许并行开工
  - **范围**：**首次引入 Redis** + WS 网关骨架（连接管理 / Session / Heartbeat / 广播 primitive `BroadcastToRoom(roomId, msg)`）+ Redis presence repo（`room:{id}:online_users`、`user:{id}:ws_session`）+ 心跳超时清理 + 房间快照下发框架（业务内容由 E11 填充）
  - **不含房间业务**（→ E11）
  - **FRs covered:** FR34（WS 连接）, FR35（快照框架）, FR39（心跳）；NFR12
  - **落实**：AR6, AR11
- **Epic 11 · Server** · 房间 CRUD + 房间快照内容 + 成员事件广播
  - **首条 story（契约）**：接口契约最终化 —— `POST /rooms` / `GET /rooms/current` / `GET /rooms/{roomId}` / `POST /rooms/{roomId}/join` / `POST /rooms/{roomId}/leave` + WS 消息 `member.joined` / `member.left` schema 落到 V1接口设计文档
  - **范围**：rooms / room_members 表 migration + 创建 / 加入 / 退出事务 + 房间快照构造（基于成员关系 + 在线 presence）+ 成员加入/离开事件通过 E10 的 `BroadcastToRoom` primitive 推送
  - **FRs covered:** FR27-33, FR35（快照内容）, FR36（成员广播）；NFR1, NFR3, NFR10
- **Epic 12 · iOS** · 房间页面 + WebSocket 客户端
  - **范围**：房间页面 SwiftUI（成员列表 + 进入/退出按钮 + 房间号展示）+ `WebSocketClient` 封装（基于 `URLSessionWebSocketTask`）+ 房间快照解析 + 成员加入/离开消息渲染 + 自动重连（指数退避）+ 心跳维护
  - **FRs covered:** FR29, FR30, FR31, FR33, FR34, FR35, FR36, FR39
- **Epic 13 · 对齐** · 房间生命周期 + 成员进出广播联调

### 节点 5：房间内猫动作同步
> **顺序**：E14 + E15 可并行 → E16
- **Epic 14 · Server** · 房间内宠物状态广播
  - **首条 story（契约）**：接口契约最终化 —— `POST /pets/current/state-sync` + WS 消息 `pet.state.changed`（或在 `room.snapshot` 中含成员宠物状态）schema 落到 V1接口设计文档
  - **FRs covered:** FR7, FR37
- **Epic 15 · iOS** · 房间内成员宠物状态展示
  - **范围**：接收房间快照中的成员宠物状态 + 订阅状态变化 WS 推送 + 在房间页内为每个成员渲染当前 walk/run/idle 状态 sprite + 状态切换动画过渡
  - **FRs covered:** FR7, FR37
- **Epic 16 · 对齐** · 跨端猫动作同步联调

### 节点 6：房间内表情
> **顺序**：E17 + E18 可并行 → E19
- **Epic 17 · Server** · 表情广播链路 + emoji_configs 预置
  - **首条 story（契约）**：接口契约最终化 —— `GET /emojis` + WS 消息 `emoji.send` (C→S) / `emoji.received` (S→C) schema 落到 V1接口设计文档
  - **FRs covered:** FR38；AR19
- **Epic 18 · iOS** · 表情面板交互 + 广播接收动效
  - **范围**：点击自己猫弹出表情面板 SwiftUI（系统表情列表 + 选择 UI）+ 选中后通过 WS 调用 `emoji.send` + 接收他人 `emoji.received` 后在房间页内为对应成员的猫上方播放飞出动效
  - **FRs covered:** FR38
- **Epic 19 · 对齐** · 房间内表情链路联调

### 节点 7：宝箱倒计时与开箱
> **顺序**：E20 + E21 可并行 → E22
- **Epic 20 · Server** · 宝箱状态机 + 开箱事务 + 奖励加权抽取 + **cosmetic_items 预置（含数量约束）** + dev 宝箱/道具发放
  - **首条 story（契约）**：接口契约最终化 —— `GET /chest/current` + `POST /chest/open`（含 `idempotencyKey` + 奖励 reward 结构）schema 落到 V1接口设计文档
  - **范围**：含 cosmetic seed story，必须满足 AR18 数量约束（common ≥ 8 / rare ≥ 4 / epic ≥ 2 / legendary ≥ 1，含 placeholder URL）+ **dev 端点 `POST /dev/force-unlock-chest`**（强制把当前宝箱状态切到 unlockable）+ **`POST /dev/grant-cosmetic-batch`**（批量发放指定品质道具，让 demo 时凑齐 10 件 common 用于合成）
  - **FRs covered:** FR11, FR12, FR13, FR14, FR15；NFR1, NFR3, NFR5；AR18
- **Epic 21 · iOS** · 首页宝箱倒计时 + 奖励弹窗
  - **范围**：首页 SwiftUI 加宝箱组件（倒计时 Timer 驱动 UI + 解锁状态切换 + 开箱按钮）+ 调用 `/chest/open` + 奖励弹窗 popup（基于 cosmetic 配置展示装扮名 / 品质 / 图标）+ 开箱前主动同步步数
  - **FRs covered:** FR11, FR12, FR13（展示侧）
- **Epic 22 · 对齐** · 宝箱开箱链路联调（**不含入仓**，奖励仅显示）

### 节点 8：仓库（仅展示）
> **顺序**：E23 + E24 可并行 → E25
- **Epic 23 · Server** · 背包查询接口（catalog + inventory，**不含 equip/unequip**）
  - **首条 story（契约）**：接口契约最终化 —— `GET /cosmetics/catalog` + `GET /cosmetics/inventory` schema 落到 V1接口设计文档
  - **FRs covered:** FR16, FR17, FR18
- **Epic 24 · iOS** · 仓库页面（聚合 + 实例列表）
  - **范围**：仓库页 SwiftUI（按 cosmetic_item 聚合 grid + 实例列表展开）+ 调用 `/cosmetics/inventory` + 基础筛选交互（按品质 / 槽位）+ **穿戴入口按钮预留位置**（行为留空，节点 9 实装真正的事务调用）
  - **FRs covered:** FR17, FR18
- **Epic 25 · 对齐** · 仓库链路联调（"开箱奖励 → 入仓 → 仓库可见"）

### 节点 9：穿戴 / 卸下
> **顺序**：E26 + E27 可并行（E27 可用 mocked 接口先做 UI + 文字降级）→ E28
- **Epic 26 · Server** · 穿戴 / 卸下事务 + 一致性约束
  - **首条 story（契约）**：接口契约最终化 —— `POST /cosmetics/equip` + `POST /cosmetics/unequip` schema 落到 V1接口设计文档
  - **FRs covered:** FR19, FR20, FR21；NFR1, NFR2, NFR11
- **Epic 27 · iOS** · 穿戴交互 + **文字降级渲染**
  - **范围**：仓库内点选实例 → 调用 `/cosmetics/equip` + 卸下 → `/cosmetics/unequip` + **文字降级 UI**（缺图时在猫身上对应槽位显示道具名 + 槽位标签）+ 同槽位换装动效
  - **FRs covered:** FR19, FR20；**FR45**（缺图时文字降级）
- **Epic 28 · 对齐** · 穿戴流程联调（多槽位换装、状态一致性、缺图文字降级可验）

### 节点 10：装扮渲染配置
> **顺序**：E29（schema + seed）→ E30（消费 schema）→ E31
- **Epic 29 · Server** · `cosmetic_items.render_config JSON` 字段 + seed + 接口下发 + **数据库设计文档同步**
  - **首条 story（契约）**：接口契约最终化 —— `cosmetic_items.render_config` 在 `GET /cosmetics/catalog` / `GET /cosmetics/inventory` response 中的 schema 字段（`offset_x` / `offset_y` / `scale` / `rotation` / `z_layer`）落到 V1接口设计文档
  - **范围**：A 方案（在 cosmetic_items 加列）；MVP 不做 admin 写入接口；通过 migration + seed 维护；**同步更新 `docs/宠物互动App_数据库设计.md` §5.8 增加 `render_config JSON` 字段说明**（作为本 Epic 必含的一条 story）
  - **FRs covered:** FR42, FR44；NFR23
- **Epic 30 · iOS** · 装扮渲染层（按 render_config 在猫身上正确显示装扮图像）
  - **范围**：`SpriteRenderer` 封装 + 解析 `cosmetic_items.render_config`（offset_x / offset_y / scale / rotation / z_layer）+ 按参数在猫身上对应槽位锚点定位渲染 + 配置缺失时退回 E27 的文字降级
  - **FRs covered:** FR43；NFR23, NFR24
- **Epic 31 · 对齐** · 装扮渲染验收（视觉差异明显、状态切换不错位、配置缺失时仍能文字降级）

### 节点 11：合成
> **顺序**：E32 + E33 可并行 → E34
- **Epic 32 · Server** · 合成事务 + idempotencyKey + 双日志
  - **首条 story（契约）**：接口契约最终化 —— `GET /compose/overview` + `POST /compose/upgrade`（含 `userCosmeticItemIds` + `idempotencyKey` + reward）schema 落到 V1接口设计文档
  - **FRs covered:** FR22, FR23, FR24, FR25, FR26；NFR1, NFR3, NFR5
- **Epic 33 · iOS** · 合成页面 + 手动选材交互
  - **范围**：合成页 SwiftUI（按品质 tab + 实例 grid 浏览）+ 手动选 10 个实例 + 已选数量校验（恰好 10 / 同品质）+ 提交 + 提交中态 + 成功 popup（展示产出实例）+ 失败错误处理
  - **FRs covered:** FR22, FR26
- **Epic 34 · 对齐** · 合成链路联调（**合成产出实例进入仓库可见即算 demo 通过**，不需要等节点 9/10 的可视化）

### 节点 12：分享房间链接
> **顺序**：E35 → E36
- **Epic 35 · iOS** · 房间链接生成 + Universal Link 解析 + 自动加入
  - **范围**：房间内分享按钮 + 链接生成（Custom URL Scheme `catapp://room/{roomId}` 或 Universal Link）+ 链接解析处理 + 已登录情况下自动调用 `/rooms/{roomId}/join` + 跳转到房间页
  - **FRs covered:** FR40, FR41
  - **Server 说明：** 复用 `/rooms/{id}/join`，无新接口
- **Epic 36 · 对齐** · 分享链接联调（链接生成 → 解析 → 拉起 App → 自动加入）

---

# Epic 与 Story 详情

> 本节由 Step 3 逐个 Epic 生成，按节点顺序排列。每个含生产代码的 Story 的 AC 严格遵循 Layer 1（含单元测试 + 集成测试条目），核心事务额外含 Layer 2 专属集成测试 Story。

## Epic 1: Server 工程脚手架 + ping 接口 + 测试基础设施 + Dev Tools 框架

完成最小可运行的 Go 服务骨架，能在本地 `bash scripts/build.sh && ./build/catserver` 启动并响应 ping，且为后续节点准备好测试基础设施 + Dev Tools 框架。**严格按需引入原则**：本 Epic 不接 MySQL（→ E4）/ Redis（→ E10）/ WebSocket（→ E10）/ auth-rate_limit 中间件（→ E4）。

### Story 1.1: Mock 库选型 spike + Logger / Metrics 框架选型

As a 服务端开发,
I want 在写第一行 service 代码前确定 mock + test + logger + metrics 工具栈,
So that 后续 Epic 写测试 / 写日志 / 写指标时不用临时拼凑，避免每个 epic 自己 println 风格漂移.

**Acceptance Criteria:**

**Given** server/ 目录基本为空
**When** 完成 spike
**Then** 输出选型决策文档 `_bmad-output/implementation-artifacts/decisions/0001-test-stack.md`，内容包含:
- **测试栈**:
  - DB mock 方案: sqlmock / 真 MySQL via dockertest / sqlmock + miniredis 中选一，附理由
  - HTTP handler 测试方案: httptest 标准库 / Gin 自带 test helpers / testify mock
  - 断言库: 标准库 testing / testify
  - CI 跑法: `go test -race -cover ./...` 还是按目录拆分
- **Logger 选型**（新增）: zerolog / slog (Go 1.21+) / zap 中选一，附理由 + 结构化字段约定（request_id / user_id / api_path / latency_ms / business_result）
- **Metrics 选型**（新增）: Prometheus client / OpenTelemetry / Vector 中选一，附理由 + 至少预留 NFR16 列出的指标位（QPS / 耗时 / 错误率 / 开箱次数 / 合成次数 / 在线房间数 / 在线连接数）
**And** 选型在 Epic 1 后续 story 落地时直接采用，不再二次讨论
**And** Story 1.3（中间件）的 logging 中间件按本 spike 选型实装

### Story 1.2: cmd/server 入口 + 配置加载 + Gin + ping

As a 服务端开发,
I want 一个能在本地 `go run cmd/server/main.go` 启动并监听 8080 的 Gin 应用,
So that 后续所有 server 工作都基于同一个入口推进.

**Acceptance Criteria:**

**Given** 已确定使用 YAML 配置 + Gin
**When** 实装本 story
**Then** `cmd/server/main.go` 存在并能编译启动
**And** 启动时从 `configs/local.yaml` 加载 `server.http_port`（默认 8080）和基础日志级别
**And** 路由 `GET /ping` 返回 `{"code":0,"message":"pong","data":{},"requestId":"req_xxx"}`（符合 V1接口设计 §2.4）
**And** 环境变量 `CAT_HTTP_PORT` 可覆盖配置文件
**And** 启动日志输出 "server started on :8080"
**And** **单元测试覆盖**（≥3 case）:
- happy: 配置文件存在 + 合法 → 加载成功，http_port = 8080
- edge: 配置文件缺失 → 优雅退出，日志输出 "config file not found"
- edge: `CAT_HTTP_PORT=9999` 环境变量 → 覆盖配置，实际监听 9999
- edge: 端口被占用 → 启动失败，error 明确
**And** **集成测试覆盖**: `httptest` 启动应用 → `GET /ping` → response code = 200, body 符合统一格式

### Story 1.3: 中间件 request_id / recover / logging

As a 服务端开发,
I want 所有请求自动带 request_id 并打印结构化日志、panic 时不挂掉进程,
So that 后续每个业务接口都自动满足 NFR14（请求带 request_id）和 NFR15（结构化日志）.

**Acceptance Criteria:**

**Given** Gin 已经能跑 ping
**When** 注册三个中间件
**Then** 每个请求 response header 含 `X-Request-Id`，值为 UUID v4
**And** 请求日志单行结构化输出: `request_id`, `method`, `path`, `status`, `latency_ms`, `client_ip`
**And** handler panic 时返回 500 + 错误响应，不挂掉服务进程
**And** panic 在日志里有完整 stack trace
**And** **单元测试覆盖**（≥4 case）:
- happy: 请求无 X-Request-Id header → 自动生成 UUID v4
- edge: 请求带已有 X-Request-Id header → 透传不覆盖
- happy: panic handler → recover 中间件返回 500 + 日志含 stack
- happy: 请求结束 → logging 中间件输出含全部字段
**And** **集成测试覆盖**: `httptest` 启动 → 请求 panic 路由 → 第二次请求 ping 仍正常响应（验证服务未挂）

### Story 1.4: /version 接口

As a 测试与运维者,
I want 调用 GET /version 看到当前服务的 git commit 和构建时间,
So that demo 时可以确认运行的是预期版本.

**Acceptance Criteria:**

**Given** Story 1.2 完成
**When** 调用 `GET /version`
**Then** 返回 `{"code":0,"data":{"commit":"<short-sha>","builtAt":"<ISO8601>"}}`
**And** commit 通过编译期 `-ldflags "-X main.commit=$(git rev-parse --short HEAD)"` 注入
**And** builtAt 通过编译期 `-ldflags "-X main.builtAt=$(date -u +...)"` 注入
**And** **单元测试覆盖**（≥3 case）:
- happy: ldflags 注入了 commit + builtAt → handler 返回正确 JSON
- edge: ldflags 未注入（空字符串）→ 返回 commit/builtAt 都是 "unknown"（而非空字符串导致 JSON 异常）
- edge: response 严格符合统一格式（含 code / data / requestId）
**And** **集成测试覆盖**: 编译时注入 mock commit `abc1234` → 启动 → curl `GET /version` → 返回的 commit = `abc1234`

### Story 1.5: 测试基础设施搭建（按 Story 1.1 选型）

As a 服务端开发,
I want server 端测试栈一次配齐（mock 库 + fixture + CI 命令）,
So that 后续 Epic 写第一条 service 测试时直接能跑，不被工具栈卡住.

**Acceptance Criteria:**

**Given** Story 1.1 已经输出选型决策
**When** 完成本 story
**Then** `go test ./...` 在 server/ 目录跑通（即使现在几乎没有业务测试）
**And** 至少存在一条**业务相关 mock 单元测试**（满足 AR27 done 标准）:
- 例如: 写一个 placeholder `internal/service/sample_service.go`，有 mocked 依赖，table-driven test 验证 mock 注入链路
- **此测试同时是后续业务 story Layer 1 测试 AC 的模板示范** —— 后续 Epic 写测试时可直接复制这个文件结构
**And** `go test -race -cover ./...` 跑通
**And** `bash scripts/build.sh --test`（Story 1.7 重做后）能调用上述测试

### Story 1.6: Dev Tools 框架（build flag gated）

As a 服务端开发,
I want 一个被 build flag 控制的 /dev/* 路由组,
So that 后续业务 Epic（E7 / E20）可以挂上自己的 dev 端点而不污染生产构建.

**Acceptance Criteria:**

**Given** Gin 已能跑
**When** 实装本 story
**Then** 仅当 `BUILD_DEV=true` 环境变量为真（或 build tag `-tags devtools`）时，`/dev/*` 路由组被注册
**And** 该路由组前置一个 `dev_only` 中间件，生产构建下任何请求 `/dev/*` 直接返回 404
**And** 注册一个示例端点 `/dev/ping-dev` 验证框架可用，返回 `{"code":0,"data":{"mode":"dev"}}`
**And** 开发模式启动时日志明确警告 "DEV MODE ENABLED - DO NOT USE IN PRODUCTION"
**And** **单元测试覆盖**（≥4 case）:
- happy: BUILD_DEV=true → /dev/ping-dev 返回 200 + dev mode body
- edge: BUILD_DEV=false → /dev/ping-dev 返回 404
- edge: BUILD_DEV=true 但请求其他非 dev 路由 → 不受影响
- edge: dev_only 中间件单独被测时返回 404 + 日志记录被拒请求
**And** **集成测试覆盖**: 两次启动应用（BUILD_DEV=true / BUILD_DEV=false）→ 同一请求 `/dev/ping-dev` → 分别返回 200 / 404

### Story 1.7: 重做 scripts/build.sh

As a 服务端开发,
I want 一个对齐新 cmd/server 入口、移除旧 cmd/cat / openapi / check_time_now 引用的 build 脚本,
So that 工程入口与 CLAUDE.md 描述一致，后续不必再绕开旧脚本残留.

**Acceptance Criteria:**

**Given** 旧脚本仍引用 `cmd/cat` / `docs/api/openapi.yaml` / `scripts/check_time_now.sh`
**When** 完成本 story
**Then** `bash scripts/build.sh` 跑 `go vet ./...` + 编译 `cmd/server/main.go` 到 `build/catserver`
**And** `bash scripts/build.sh --test` 额外跑 `go test ./...`
**And** `bash scripts/build.sh --race --test` 加 race detector
**And** 不再调用 `check_time_now.sh` 或检查 `openapi.yaml`
**And** 编译时通过 `-ldflags` 注入 commit + builtAt（供 Story 1.4 的 /version 使用）
**And** **手动验证**（非自动测试）: 三种参数组合（无参 / `--test` / `--race --test`）都能跑通且 exit code 0

### Story 1.8: AppError 类型 + 错误三层映射框架（NFR18）

As a 服务端开发,
I want 一个统一的 AppError 类型 + 三层映射工具，全 server 错误处理都基于此,
So that NFR18 "repo → service → handler 三层错误映射" 不靠开发自觉，避免 5 个月后 error 漂移成一团乱麻.

**Acceptance Criteria:**

**Given** Epic 1 脚手架完成 + Story 1.3 中间件已挂
**When** 完成本 story
**Then** `internal/pkg/errors/` 提供:
- `AppError struct { Code int, Message string, Cause error, Metadata map[string]any }`
- `New(code int, msg string) *AppError` 构造器
- `Wrap(err error, code int, msg string) *AppError`（保留 cause 原始 error 链）
- `As(err error) (*AppError, bool)` 类型断言工具
**And** 提供错误码常量：复用 V1接口设计 §3 列出的全部 26 个错误码（即使本节点没用上的也定义）：`ErrUnauthorized = 1001`, `ErrInvalidParam = 1002`, ..., `ErrEmojiNotFound = 7001`, `ErrWSNotConnected = 7002`
**And** 提供 handler 层中间件 `ErrorMappingMiddleware`：
- 在 handler 之后执行
- 检测到 panic / handler return *AppError → 自动 marshal 为 V1接口设计 §2.4 统一响应结构 `{code, message, data:null, requestId}`
- 非 *AppError 的原生 error → wrap 为 `AppError{Code: 1009, Message: "服务繁忙"}` + log error
**And** 提供约定文档 `_bmad-output/implementation-artifacts/decisions/0006-error-handling.md`：
- repo 层：返回原生 error（数据库 / Redis 错误），不 wrap
- service 层：catch repo error 后 `Wrap(err, 业务 code, 业务 msg)`
- handler 层：仅 return `*AppError`，不直接处理 HTTP response
**And** **单元测试覆盖**（≥6 case）:
- happy: AppError.Error() 实现 error 接口
- happy: Wrap 保留 cause + errors.Is / errors.As 正常工作
- happy: handler return AppError → middleware 转 JSON response 含 code / message / requestId
- happy: handler panic with non-AppError → middleware 转 1009
- happy: errors.As 多层 wrap 后仍能拿到原始 AppError
- edge: nil error 传入 Wrap → 不 panic，返回 nil
**And** **集成测试覆盖**: 启动一个测试 handler 主动 return `AppError{1002, "test"}` → curl → response code=1002, message="test"

### Story 1.9: Go context 传播框架 + cancellation 验证

As a 服务端开发,
I want 全 server 严格遵守 ctx propagation 约定（handler → service → repo），且至少有一个 mock 测试验证 ctx cancel 真的生效,
So that 客户端断开 / 请求超时时 server 能立即释放资源，不出现 hang 住事务 + 锁泄漏.

**Acceptance Criteria:**

**Given** Story 1.8 AppError 框架已就绪 + Gin 默认会把 client 断开的信号传到 ctx
**When** 完成本 story
**Then** 输出约定文档 `_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md`：
- 全 service / repo 函数签名第一个参数必须是 `ctx context.Context`
- handler 必须传 `c.Request.Context()`（Gin 已自动 cancel on client disconnect）
- repo 层调用 DB/Redis 必须用 `*WithContext` 方法（如 GORM 的 `db.WithContext(ctx)`）
- 长事务在 `txManager.WithTx(ctx, fn)` 内 fn 收到的 txCtx 必须传到所有 repo 调用
**And** Story 1.3 logging 中间件 AC 追加：每个请求结束后 log 一行包含 `ctx_done` 字段（true 表示请求被 cancel）
**And** Story 1.5 测试基础设施模板示例追加一个 ctx cancel 测试：
- mock 一个慢 repo（sleep 5 秒）
- service 调用时传 ctx，外层 100ms 后 cancel
- 验证 service 100ms 内返回 ctx.Err() 而非 wait 5 秒
**And** **单元测试覆盖**（≥3 case）:
- happy: ctx 正常 → service 正常返回
- happy: ctx 被 cancel → service 提前返回 ctx.Err()
- happy: handler 模拟 client 断开 → 通过 c.Request.Context() 传递 cancel → service / repo 链路全部提前返回

### Story 1.10: server README + 本地开发指南

As a 服务端开发 / 新加入团队成员,
I want 一个 server README 文档，含本地启动 / 跑测试 / 跑 migration / 开 dev mode 的全部命令,
So that 我打开 server/ 目录立即知道怎么开始，不用问 / 翻 epic.

**Acceptance Criteria:**

**Given** Epic 1 脚手架基本就绪
**When** 完成本 story
**Then** 输出 `server/README.md`，含至少以下章节:
- **快速启动**：`bash scripts/build.sh && ./build/catserver`
- **依赖**：MySQL 8.0 + Redis 6+ 本地启动方式（docker-compose 命令或 brew install）
- **配置**：`configs/local.yaml` 各字段说明 + 环境变量覆盖（`CAT_HTTP_PORT` 等）
- **跑 migration**：`./build/catserver migrate up / down / status`
- **跑测试**：
  - `bash scripts/build.sh --test` 单元测试
  - `bash scripts/build.sh --race --test` 加 race
  - `go test -tags integration ./...` 集成测试（需 dockertest）
- **开 dev mode**：`BUILD_DEV=true ./build/catserver` + 解释 dev 端点（`/dev/grant-steps`, `/dev/force-unlock-chest` 等）
- **目录结构**：简单复制 设计文档 §4 + 标注每个目录职责
- **常见 troubleshooting**：MySQL 连接失败 / Redis 连接失败 / migration 冲突 → 怎么办
**And** README **必须保持与代码同步**：每个 epic 完成时如有命令 / 配置 / 流程变化，对齐 epic Story X.3 文档同步要更新 README
**And** **不需要单元测试**（纯文档）—— 但**手动验证**：按 README 步骤一遍走通，确保命令 100% 可执行

## Epic 2: iOS 工程脚手架 + 首页骨架 + 导航架构 + ping 调用 + 测试基础设施

完成最小可运行的 SwiftUI App 骨架，含主界面信息架构定稿（占位区块）+ 导航架构（NavigationStack + Sheet 模板）+ APIClient 框架 + ping 调用 + 基础错误 UI 框架 + iOS 测试基础设施。**关键设计决定**：本 Epic 一次定稿主界面信息架构，避免后续 Epic 各自重做主界面拼贴。

### Story 2.1: iOS mock 框架选型 + ios/ 目录决策 spike

As an iOS 开发,
I want 在写第一行 SwiftUI 代码前确定 mock 工具栈和 ios/ 目录的去留,
So that 后续 Epic 写测试 + 集成代码时不被工具 / 目录改动反复打断.

**Acceptance Criteria:**

**Given** ios/ 目录残留 Cat.xcodeproj / CatShared / CatWatch* / CatPhoneTests 等历史产物
**When** 完成 spike
**Then** 输出选型决策文档 `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md`，含:
- iOS mock 框架选型: XCTest only / Mockingbird / Cuckoo / Swift Mocks 之间选一，附理由
- 异步测试方案: async/await + XCTestExpectation
- ios/ 目录方案: (A) 复用 Cat.xcodeproj 改名 + 清理 / (B) 在 ios/ 下新建独立 PetApp.xcodeproj + 把 CatWatch* 移到 watch/ 子目录 / (C) 完全 wipe 重建
- CI 跑法: `xcodebuild test -scheme ... -destination 'platform=iOS Simulator,...'`
**And** 选型在 Epic 2 后续 story 落地时直接采用，不再二次讨论

### Story 2.2: SwiftUI App 入口 + 主界面骨架 + 信息架构定稿

As an iOS 开发,
I want 一个能在模拟器跑、含主界面占位区块（猫 / 步数 / 宝箱 / 进房 / 仓库 / 合成 / 版本号）的 SwiftUI App,
So that 后续 Epic 是"在已存在的占位上填内容"，不再每次重做主界面.

**Acceptance Criteria:**

**Given** Story 2.1 已确定 ios/ 目录方案 + 完成清理
**When** 实装本 story
**Then** 模拟器可启动 App，看到主界面包含以下占位区块（可用 `Text("PET 占位")` 等占位文本）:
- 顶部: 用户昵称 + 头像位（`Text("用户1001") + Circle()`）
- 中间: 猫展示区（`Rectangle().fill(.gray)` 占位，宽高定好）
- 中间下方: 步数显示位（`Text("0 步")`）
- 中间右侧: 宝箱位（`Rectangle().fill(.brown)` 占位）
- 底部: 三个主按钮（进入房间 / 仓库 / 合成）
- 角落: 版本号小字（占位 `v0.0.0 · ----`）
**And** App 名称 / Bundle ID 按 0002-ios-stack.md 决策
**And** **单元测试覆盖**（≥3 case）:
- happy: HomeView body 渲染时包含全部 6 个占位区块的 accessibility identifier
- edge: 不同尺寸（iPhone SE / iPhone 15 Pro Max）下布局不破
- happy: 点击三个主按钮触发各自的 `onTap` action（暂时为空函数，验证回调注册）
**And** **集成测试覆盖**: UITest 启动模拟器 → 验证主界面 6 个区块的 accessibility identifier 都可定位

### Story 2.3: 导航架构搭建（NavigationStack + 全屏 Sheet 模板）

As an iOS 开发,
I want 主界面有清晰的导航架构（push 走 NavigationStack，全屏页面走 Sheet）,
So that 后续 Epic（房间 / 仓库 / 合成 / 设置）按既定模式接入，不混用导航 pattern.

**Acceptance Criteria:**

**Given** Story 2.2 主界面骨架完成
**When** 实装本 story
**Then** 主界面三个按钮（进入房间 / 仓库 / 合成）点击后弹出全屏 Sheet（`.fullScreenCover`），Sheet 内显示对应 placeholder 页面（如 `Text("Room Placeholder")`）
**And** Sheet 顶部有"关闭"按钮，点击关闭返回主界面
**And** 设计 `AppCoordinator` 或 `NavigationRouter`，集中管理 Sheet 状态（`@Published var presentedSheet: SheetType?`）
**And** **单元测试覆盖**（≥3 case）:
- happy: 调用 `coordinator.present(.room)` → presentedSheet == .room
- happy: 调用 `coordinator.dismiss()` → presentedSheet == nil
- edge: 当前已 present 一个 sheet，再 present 另一个 → 后者覆盖前者
**And** **集成测试覆盖**: UITest 点主界面"进入房间"按钮 → Sheet 弹出 + 含"Room Placeholder"文本 → 点关闭 → Sheet 消失

### Story 2.4: APIClient 封装

As an iOS 开发,
I want 一个统一的 APIClient（URLSession + 统一 response 解析 + 错误映射）,
So that 后续所有 REST 调用走同一入口，统一处理 401 / 网络错误 / 业务错误码.

**Acceptance Criteria:**

**Given** Story 2.1 已选定 mock 框架
**When** 实装本 story
**Then** `APIClient` 提供 `request<T: Decodable>(_ endpoint: Endpoint) async throws -> T`
**And** `Endpoint` 枚举包含 path / method / body / requiresAuth 等元信息
**And** 自动解析 V1接口设计 §2.4 的统一响应结构 `{code, message, data, requestId}`
**And** code != 0 时抛出 `APIError.business(code: Int, message: String, requestId: String)`
**And** HTTP 401 时抛出 `APIError.unauthorized`
**And** 网络错误时抛出 `APIError.network(underlying: Error)`
**And** 解码失败时抛出 `APIError.decoding(underlying: Error)`
**And** **单元测试覆盖**（≥5 case，使用 mock URLSession）:
- happy: 200 + `{code:0, data:{...}}` → 返回 data 的 T 类型
- edge: 200 + `{code:1002, message:"参数错误"}` → 抛 APIError.business
- edge: 401 → 抛 APIError.unauthorized
- edge: 网络超时 → 抛 APIError.network
- edge: 200 但 body 不符合统一结构 → 抛 APIError.decoding
**And** **集成测试覆盖**: 启动 XCTest mock HTTP server → APIClient 调用 → 各场景 response 路径正确

### Story 2.5: ping 调用 + 主界面显示 server /version 信息

As an iOS 开发,
I want App 启动后调用 server 的 ping + /version 接口，把 server commit 显示在主界面角落版本号位,
So that demo 时一眼能看到 client / server 是否在线 + 各自版本.

**Acceptance Criteria:**

**Given** Story 2.2 主界面有版本号占位、Story 2.4 APIClient 可用
**When** 实装本 story
**Then** App 启动时 `HomeViewModel` 触发 `PingUseCase` + `FetchVersionUseCase`
**And** 主界面版本号显示格式: `App v<App build hash> · Server <server commit>`
**And** ping 失败时版本号显示 `App v<...> · Server offline`
**And** 调用通过 APIClient（不是直接 URLSession）
**And** **单元测试覆盖**（≥4 case，mocked APIClient）:
- happy: ping 成功 + version 成功 → ViewModel 状态含 server commit
- edge: ping 失败 → ViewModel 状态显示 "Server offline"
- edge: ping 成功但 version 失败 → 显示 "Server v?"（部分降级）
- happy: 重复触发不会发起重复请求（debounce / 单次任务）
**And** **集成测试覆盖**: 跑 XCTest mock server → App 启动 → 主界面版本号显示 mock commit

### Story 2.6: 基础错误 UI 框架（Toast / AlertOverlay / RetryView）

As an iOS 开发,
I want 一套统一的错误 UI 组件（Toast / Alert / Retry）+ 与 APIError 自动联动的 ErrorPresenter,
So that 后续业务页不必各自实装错误展示，所有 APIError 自动用统一 UX.

**Acceptance Criteria:**

**Given** Story 2.4 APIClient 抛出 APIError 各类型
**When** 实装本 story
**Then** 提供三个组件:
- `Toast`(SwiftUI View): 顶部短暂浮现，1-3 秒自动消失，用于温和提示（如 "已同步"）
- `AlertOverlay`(SwiftUI View): 全屏阻塞 alert，含标题 + 消息 + 单 OK 按钮，用于业务错误（code != 0）
- `RetryView`(SwiftUI View): 全屏 placeholder，含 "出错了" 文本 + 重试按钮 + 错误描述，用于网络 / 严重错误
**And** 提供 `ErrorPresenter`：根据 APIError 类型自动选择对应组件
- `.business` → AlertOverlay（显示 message）
- `.unauthorized` → 跳转登录态 + Toast 提示
- `.network` → RetryView
- `.decoding` → AlertOverlay（"数据异常，请稍后重试"）
**And** ViewModel 通过 `errorPresenter.present(error)` 触发 UI，不直接操作 SwiftUI
**And** **单元测试覆盖**（≥4 case）:
- happy: APIError.business → ErrorPresenter 触发 AlertOverlay 状态
- happy: APIError.network → 触发 RetryView 状态
- happy: Toast 显示后 N 秒自动消失
- edge: 同时触发多个 alert → 后者排队，前者消失后再展示
**And** **集成测试覆盖**: UITest 触发各类错误（mock APIClient 返回不同 APIError）→ 验证对应 UI 组件出现

### Story 2.7: iOS 测试基础设施搭建（按 Story 2.1 选型）

As an iOS 开发,
I want iOS 测试栈一次配齐（XCTest 配置 + Mock 框架 + CI 跑法）,
So that 后续 Epic 写第一条 ViewModel 测试 / UseCase 测试时直接能跑.

**Acceptance Criteria:**

**Given** Story 2.1 已经输出选型决策
**When** 完成本 story
**Then** Xcode 项目含独立的 Test target，`xcodebuild test -scheme PetApp -destination 'platform=iOS Simulator,name=iPhone 15'` 跑通
**And** 至少存在一条**业务相关 mock 单元测试**（满足 AR27 done 标准）:
- 例如: 写一个 placeholder `SampleViewModel`，有 mocked `SampleUseCase`，XCTest 验证 mock 注入链路 + ViewModel 状态切换
- **此测试同时是后续业务 story Layer 1 测试 AC 的模板示范** —— 后续 Epic 写测试时可直接复制这个文件结构
**And** CI 跑法文档化（README 或 ci.yaml）

### Story 2.8: Dev 重置 Keychain 按钮（build flag gated）

As a 开发 / demo 者,
I want 一个 dev 模式按钮一键清空 Keychain（guestUid + token），让 App 重启后表现得像"全新安装",
So that demo / 测试时不必每次卸载重装就能模拟首次启动场景.

**Acceptance Criteria:**

**Given** Story 2.2 主界面已存在 + Story 5.1 KeychainStore 即将就绪（节点 2 后才能用，本 story 提前定义 UI）
**When** 完成本 story
**Then** 主界面右上角添加一个 dev-only 按钮：
- 仅在 `BUILD_DEV=true`（编译时 build flag 或 `-tags devtools`）时**显示**
- 按钮文案 "重置身份"，icon 用 SF Symbol `arrow.counterclockwise.circle`
- 点击触发 `ResetKeychainUseCase`:
  - 清空 KeychainStore 全部 key（guestUid + token）
  - 弹 alert "已重置，请杀进程后重新启动 App 模拟首次安装"
**And** 生产 build 下按钮**完全不渲染**（不只是 hidden，是不存在于视图树）
**And** 与 Story 2.5 版本号显示同区域（角落 dev info），不影响主界面其他区块
**And** **单元测试覆盖**（≥3 case）:
- happy: BUILD_DEV=true → 按钮可见 + 点击调 ResetKeychainUseCase + Keychain 清空
- happy: BUILD_DEV=false → 按钮不渲染（视图树中不存在）
- edge: ResetKeychainUseCase 失败（Keychain access error）→ 弹 alert "重置失败"
**And** **UI 测试覆盖**（XCUITest dev build）: 主界面有"重置身份"按钮 → 点击 → 验证 alert 出现 + 验证 KeychainStore 已清空

### Story 2.9: LaunchingView 设计（首次启动过场）

As an iPhone 用户,
I want App 启动时看到一个友好的过场画面（猫咪 logo + 加载提示），不是空白屏,
So that 首次启动 3-5 秒等待中不焦虑.

**Acceptance Criteria:**

**Given** Story 2.2 主界面骨架 + Story 5.2 GuestLoginUseCase 在 .task 中 await + Story 5.5 LoadHomeUseCase 在登录后调
**When** 完成本 story
**Then** 实装 `LaunchingView` SwiftUI 组件:
- 居中: 猫咪 logo 占位（用大号 SF Symbol `cat.fill` 或简单几何形状）
- 下方: "正在唤醒小猫…" 文字 + 圆形进度条（`ProgressView()`）
- 背景: App 主题色 / 简单渐变
**And** RootView 路由逻辑:
- AppLaunchState `.launching` → 显示 LaunchingView
- AppLaunchState `.ready` → 显示 HomeView
- AppLaunchState `.needsAuth`（理论不该发生）→ 显示 RetryView "登录失败，请重试"
**And** AppLaunchState 流转:
- App 启动 → `.launching`
- GuestLoginUseCase + LoadHomeUseCase 都成功 → `.ready`
- 任一失败 → `.needsAuth`
**And** LaunchingView 至少**显示 0.3 秒**（避免极快网络下 LaunchingView 闪一下就消失，造成视觉跳动）
**And** **单元测试覆盖**（≥4 case，UI snapshot + AppLaunchState mocked）:
- happy: AppLaunchState=.launching → LaunchingView 渲染 + ProgressView 转
- happy: 状态切到 .ready → 平滑过渡到 HomeView（淡入淡出 200ms）
- happy: GuestLoginUseCase 完成 + LoadHomeUseCase 也完成 → 状态 .ready
- edge: LoadHomeUseCase 失败 → 状态 .needsAuth → RetryView
**And** **UI 测试覆盖**（XCUITest）: 全新模拟器启动 → 看到 LaunchingView "正在唤醒小猫…" → 主界面渲染前不出现空白屏

## Epic 3: 对齐 - 节点 1 端到端 ping 联调验收

把 Epic 1 (Server 脚手架) + Epic 2 (iOS 脚手架) 真实串联，验证节点 1 §4.1 全部验收标准达成，对节点 1 实装中产生的偏离做文档同步与 tech debt 登记。

### Story 3.1: 跨端集成测试场景 - ping E2E

As a 跨端验收负责人,
I want 把 iOS 模拟器 + Server 真实串联跑通 ping 流程的步骤文档化,
So that 节点 1 完成时可以一键复现验收，也为后续节点对齐 Epic 的 E2E 测试提供模板.

**Acceptance Criteria:**

**Given** Epic 1 (Server) + Epic 2 (iOS) 全部完成
**When** 完成本 story
**Then** 输出文档 `_bmad-output/implementation-artifacts/e2e/node-1-ping-e2e.md`，含:
- 准备步骤: `bash scripts/build.sh && ./build/catserver` 在终端 A 启动 server (默认 8080)
- 准备步骤: Xcode 启动模拟器 (建议 iPhone 15) 安装并运行 PetApp
- 验证步骤 1: server 终端可见 "server started on :8080"
- 验证步骤 2: iOS 模拟器主界面右上角版本号显示 `App v<...> · Server <8 位 commit>`
- 验证步骤 3: 终端 B `curl http://localhost:8080/ping` 返回 `{"code":0,"message":"pong",...}`
- 验证步骤 4: 终端 B `curl http://localhost:8080/version` 返回正确 commit
- 验证步骤 5: 修改 `CAT_HTTP_PORT=9090` 重启 server，App 启动失败时显示 "Server offline"（验证错误 UI 框架）
**And** 文档含截图位（节点 1 demo 时填充）

### Story 3.2: 节点 1 demo 验收

As a 节点 1 owner,
I want 按 docs/宠物互动App_MVP节点规划与里程碑.md §4.1 的全部验收标准走一遍真实流程,
So that 节点 1 真正进入 done 状态，节点 2 可以开工.

**Acceptance Criteria:**

**Given** Story 3.1 的 E2E 文档已就绪
**When** 完成本 story
**Then** 按 §4.1 节点 1 验收标准逐项打勾或登记延期:
- [ ] App 在真机或模拟器可稳定启动 (用模拟器即可)
- [ ] Server 在本地可稳定启动
- [ ] `GET /ping` 返回成功
- [ ] App 与 Server 之间至少存在一条真实联调链路 (ping + /version)
- [ ] **server 端可跑 `bash scripts/build.sh --test` 通过**（AR27 done 标准）
- [ ] **iOS 端可跑 `xcodebuild test` 通过**
**And** 输出 demo 录屏或截图集到 `_bmad-output/implementation-artifacts/demo/node-1-demo-<date>/`
**And** 任何未通过项必须登记原因 + 是否后置到节点 2

### Story 3.3: 文档同步与 tech debt 登记

As a 节点 1 owner,
I want 把节点 1 实装过程中产生的接口契约 / 数据库设计 / 已知问题同步回设计文档,
So that 节点 2 开始时所有文档反映真实状态，不留隐性偏离.

**Acceptance Criteria:**

**Given** Epic 1-2 完成 + Story 3.2 demo 验收完成
**When** 完成本 story
**Then** 检查并更新以下文档（无变更也要明确标注"无更新"）:
- [ ] `docs/宠物互动App_V1接口设计.md`: 是否需要把 `/ping` / `/version` 接口加入文档？建议加，作为最简接口示范
- [ ] `docs/宠物互动App_Go项目结构与模块职责设计.md`: 节点 1 实装的目录结构是否与 §4 一致？有差异要么改文档要么改代码
- [ ] `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md`: ios/ 目录的最终方案 (按 0002-ios-stack.md) 是否与 §4 一致？
- [ ] `CLAUDE.md`: 节点 1 完成后 §"节点 1 之后的目录形态" 是否反映实际结构？
**And** 在 `_bmad-output/implementation-artifacts/tech-debt-log.md` 中登记节点 1 内的"延期决定" / "妥协" / "后续要还的债"，每条含: 描述 / 应在哪个节点偿还 / 风险评估

## Epic 4: Server - MySQL 接入 + auth/rate_limit 中间件 + 游客登录接口 + 首次初始化事务

节点 2 第一个业务 Epic。**首次引入 MySQL** + 5 张表 migrations + JWT token util + auth/rate_limit 中间件 + 游客登录接口 + 首次初始化事务。含 Layer 2 专属集成测试 Story 4.7。

### Story 4.1: 接口契约最终化（auth/guest-login + me）

As a 跨端契约负责人,
I want 在写第一行 server 业务代码前把 /auth/guest-login 和 /me 的 schema 锚定到 V1接口设计文档,
So that iOS Epic 5 可以基于稳定的契约并行开工，不出现"接口和文档对不上"的返工.

**Acceptance Criteria:**

**Given** docs/宠物互动App_V1接口设计.md §4.1 / §4.3 已有初稿
**When** 完成本 story
**Then** §4.1 (POST /auth/guest-login) schema 完全锚定，含:
- request: `{guestUid: string, device: {platform, appVersion, deviceModel}}` 字段类型 / 必填项 / 长度约束
- response data: `{token: string, user: {id, nickname, avatarUrl, hasBoundWechat}, pet: {id, petType, name}}`
- 错误码: 1002 参数错误 / 1009 服务繁忙
**And** §4.3 (GET /me) schema 完全锚定（response data: `{user: {id, nickname, avatarUrl, hasBoundWechat, currentRoomId}}`）
**And** **§5.1 (GET /home) schema 完全锚定**（节点 2 initial 版含 user + pet + stepAccount + chest；room / equips / renderConfig 字段在后续节点 increment 加入，本 contract 同步标注 future fields）—— 详见 Story 4.8
**And** 标注"本契约自此 Epic 起冻结，变更需触发 iOS Epic 5 重新评审"
**And** Git commit 单独提交契约定稿，便于 iOS 团队 cherry-pick 引用

### Story 4.2: MySQL 接入（GORM/sqlx 选型 + 连接池 + tx manager）

As a 服务端开发,
I want server 能连上 MySQL 并提供统一事务管理器,
So that 后续所有业务 Epic 可以直接用 `txManager.WithTx(...)` 写多表事务.

**Acceptance Criteria:**

**Given** Epic 1 已建好脚手架，无 DB 接入
**When** 完成本 story
**Then** 输出选型决策文档 `_bmad-output/implementation-artifacts/decisions/0003-orm-stack.md`，记录 GORM vs sqlx 选型理由
**And** `internal/infra/db/` 目录提供 MySQL 连接初始化（基于 YAML 配置 `mysql.dsn / max_open_conns / max_idle_conns`）
**And** server 启动时 ping MySQL 失败则**fail-fast**（启动失败而非容忍降级，符合 NFR3 状态以 server 为权威原则）
**And** `internal/repo/tx/manager.go` 提供 `WithTx(ctx, fn func(txCtx) error) error` 统一入口（AR10）
**And** **单元测试覆盖**（≥4 case）:
- happy: 配置正确 → 连接池建立成功
- edge: dsn 错误 → 启动 error 含明确原因
- happy: WithTx 函数 return nil → commit
- edge: WithTx 函数 return error → rollback，error 透传
**And** **集成测试覆盖**（用 dockertest 起真实 MySQL）:
- 启动 → ping MySQL → 跑 WithTx 写一行 + 故意 error 回滚 → 验证表为空

### Story 4.3: 五张表 migrations

As a 服务端开发,
I want migration 工具 + 节点 2 需要的 5 张表 DDL,
So that `go run cmd/server/main.go migrate up` 能一键建库.

**Acceptance Criteria:**

**Given** Story 4.2 MySQL 已接入
**When** 完成本 story
**Then** 引入 migration 工具（推荐 golang-migrate 或等价方案，在 0003-orm-stack.md 决策）
**And** `migrations/` 目录含编号文件:
- `0001_init_users.sql` (按 数据库设计.md §5.1)
- `0002_init_user_auth_bindings.sql` (按 §5.2)
- `0003_init_pets.sql` (按 §5.3)
- `0004_init_user_step_accounts.sql` (按 §5.4)
- `0005_init_user_chests.sql` (按 §5.6)
**And** 每个 migration 必须可逆（含 down.sql）
**And** server 提供 `cmd/server/main.go migrate up` / `migrate down` / `migrate status` 子命令
**And** **单元测试覆盖**（≥3 case）:
- happy: migrate up 后五张表存在 + 字段类型 + 索引 + 唯一约束都符合 §5
- happy: migrate down 后表全部删除
- edge: 重复 migrate up → 幂等不报错
**And** **集成测试覆盖**（dockertest 起 MySQL）: migrate up → 用 `SHOW CREATE TABLE` 对比 §5 schema → migrate down → 表为空

### Story 4.4: token util（JWT 签发 + 校验）

As a 服务端开发,
I want 一个独立的 token 签发与校验工具,
So that auth 中间件 + 游客登录 handler 可以分别复用，不重复实装 JWT 逻辑.

**Acceptance Criteria:**

**Given** Epic 1 脚手架完成
**When** 完成本 story
**Then** `internal/pkg/auth/token.go` 提供:
- `Sign(userID uint64, expireSec int) (string, error)` 用 HS256 + 配置 secret
- `Verify(token string) (claims, error)` 解析 + 校验签名 + 校验过期
**And** Token claims 含: `user_id`, `iat`, `exp`
**And** secret 从 YAML 配置 `auth.token_secret` 读取，启动时 secret 为空则 fail-fast
**And** 默认过期时间 `auth.token_expire_sec` (默认 7 天)
**And** **单元测试覆盖**（≥5 case）:
- happy: Sign + Verify → claims.user_id 正确
- edge: 过期 token → Verify 返回 error 明确
- edge: 签名被篡改 → Verify 返回 error
- edge: 格式不合法 token → Verify 返回 error
- edge: secret 为空时调用 Sign → 返回 error

### Story 4.5: auth + rate_limit 中间件（AR9b）

As a 服务端开发,
I want 一个 Bearer token 校验中间件 + 一个限频中间件,
So that 后续业务接口可以按需挂载，不必各自处理鉴权和限频.

**Acceptance Criteria:**

**Given** Story 4.4 token util 可用
**When** 完成本 story
**Then** `auth` 中间件:
- 从 `Authorization: Bearer <token>` header 解析
- 调用 token util Verify
- 成功后把 `userID` 塞进 gin.Context
- 失败返回 1001 未登录 / token 无效（按 V1接口设计 §3）
**And** `rate_limit` 中间件:
- 基于内存 token bucket 实现（MVP 不依赖 Redis）
- 默认每用户每分钟 60 次
- 配置可调 `ratelimit.per_user_per_min`
- 超限返回 1005 操作过于频繁
**And** 两个中间件按需挂载（auth 默认全局开，rate_limit 至少挂在 /auth/guest-login 上）
**And** **单元测试覆盖**（≥6 case）:
- auth happy: 合法 token → 通过 + userID 注入 context
- auth edge: 无 Authorization header → 1001
- auth edge: token 过期 → 1001
- rate_limit happy: 1 分钟内 60 次内 → 通过
- rate_limit edge: 1 分钟内第 61 次 → 1005
- rate_limit edge: 跨分钟边界 → 重置计数

### Story 4.6: 游客登录接口 + 首次初始化事务

As an iPhone 用户,
I want App 启动后自动获得游客身份，再次启动时复用同一身份,
So that 我不用经过登录页就能直接玩.

**Acceptance Criteria:**

**Given** Story 4.2-4.5 全部就绪
**When** 调用 `POST /auth/guest-login` 带合法 guestUid
**Then** 服务端按以下逻辑处理:
- 查 `user_auth_bindings WHERE auth_type=1 AND auth_identifier=guestUid`
- 命中 → 复用 user_id，加载 user + pet，返回 token
- 未命中 → 在事务中:
  - 创建 users（auto-generated nickname `用户{id}`）
  - 创建 user_auth_bindings（type=guest, identifier=guestUid）
  - 创建 pets（默认猫，is_default=1）
  - 创建 user_step_accounts
  - 创建 user_chests（status=counting, unlock_at=now+10min）
- 签发 token 返回
**And** rate_limit 中间件挂载（防 brute force）
**And** response 严格符合 V1接口设计 §4.1 schema
**And** **单元测试覆盖**（≥5 case，mocked repo）:
- happy: guestUid 已存在 → 走复用分支，不开事务
- happy: guestUid 不存在 → 开事务，5 个 repo 调用顺序正确
- edge: guestUid 为空 → 1002 参数错误
- edge: guestUid 长度超限（>128）→ 1002
- edge: 事务中某步失败（mock repo 抛 error）→ 整体回滚，返回 1009
**And** **集成测试覆盖**（dockertest）:
- 首次调用 → DB 五张表各新增 1 行
- 同一 guestUid 第二次调用 → 不新增行，返回同一 user_id
- 不同 guestUid 第三次调用 → DB 再新增 5 行

### Story 4.7: Layer 2 集成测试 - 游客登录初始化事务全流程

As a 资产事务负责人,
I want 一组深度集成测试覆盖游客登录初始化事务的失败回滚 / 并发 / 边界,
So that NFR1 (资产事务原子) 和 §8.1 (登录初始化事务) 有自动化保障，不只靠 Story 4.6 的 happy path.

**Acceptance Criteria:**

**Given** Story 4.6 happy path 已通过
**When** 完成本 story
**Then** 输出 `internal/service/auth_service_integration_test.go` 用 dockertest 起真实 MySQL，覆盖以下场景:
- **回滚 1**: mock pet repo 第 3 步抛 error → 验证 users / bindings 也回滚（DB 表为空）
- **回滚 2**: mock chest repo 第 5 步抛 error → 验证前 4 步全部回滚
- **回滚 3**: mock user repo 第 1 步抛 error → 验证什么都没创建
- **并发 1**: 100 个 goroutine 并发用同一 guestUid 调用 → 最终 DB 只有 1 个 user，所有 goroutine 拿到同一 user_id
- **并发 2**: 100 个 goroutine 并发用 100 个不同 guestUid → DB 100 个 user，每个 user 5 行关联数据，无串数据
- **边界 1**: guestUid 长度 128（最大允许）→ 成功
- **边界 2**: guestUid 长度 129 → 失败（在 handler 层就拦截）
- **重入**: 同一 guestUid 已成功登录 → 第二次调用走复用分支，DB 行数不变
**And** 全部场景用 dockertest 真实 MySQL 跑通（不用 sqlmock）
**And** 集成测试在 CI 标 `// +build integration` tag，本地 `go test -tags integration ./...` 跑

### Story 4.8: GET /home 聚合接口（initial 版含 user + pet + stepAccount + chest）

As an iPhone 用户,
I want App 启动后一次调用拿到主界面所需全部数据，避免 5 个串行 API,
So that 首屏 5x 串行 API 变 1 次 → 节省 ~800ms 首屏时间 + iOS 端错误处理代码减半.

**Acceptance Criteria:**

**Given** Story 4.6 五张表已通过登录初始化建立 + Story 4.1 已锚定 /home 契约
**When** 调用 `GET /home`
**Then** service 流程:
- 一次性查询: users + pets (默认猫) + user_pet_equips（节点 2 阶段为空）+ user_step_accounts + user_chests + users.current_room_id
- 返回 V1接口设计 §5.1 schema:
  ```json
  {
    "user": {"id", "nickname", "avatarUrl"},
    "pet": {"id", "petType", "name", "currentState", "equips": []},
    "stepAccount": {"totalSteps", "availableSteps", "consumedSteps"},
    "chest": {"id", "status", "unlockAt", "openCostSteps", "remainingSeconds"},
    "room": {"currentRoomId": null}
  }
  ```
- pet.equips 节点 2 阶段返回 `[]`（节点 9 由 Story 26.6 填充真实数据）
- room.currentRoomId 节点 2 阶段返回 null（节点 4 由 Story 11.10 填充）
- chest 字段直接复用 Story 20.5 的动态判定逻辑（节点 7 完成后才有 unlockable 状态判断；节点 2 阶段所有 chest 都是 counting）
**And** 接口要求 auth
**And** **单元测试覆盖**（≥4 case，mocked repo）:
- happy: 登录后立即调 → 返回 user + 默认 pet + step (全 0) + chest (counting)
- happy: chest unlock_at 已过 → status=2 unlockable
- edge: 用户无 pet（理论不该）→ pet 字段返回 null
- edge: 各部分 repo 错误 → 整体 1009 服务繁忙（不部分降级，避免主界面渲染异常）
**And** **集成测试覆盖**（dockertest）: 创建 user + 默认 pet + step_account + chest → curl GET /home → response 全部字段正确

### Story 4.9: Layer 2 集成测试 - 游客登录初始化事务全流程（即原 Story 4.7，重命名以保持 Layer 2 在 Epic 末尾）

> **注意**：原 Story 4.7 (Layer 2 集成测试) 在补 GAP A 后**逻辑上**应位于本 Epic 末尾（Layer 2 是收尾性 story）。**实际编号保持不变**（避免 sprint-status 重排），但 Sprint Planning 时按 4.1 → 4.6 → **4.8** → **4.7 (Layer 2)** 顺序执行。本 Story 4.9 仅作占位提示，实际内容见 Story 4.7。

## Epic 5: iOS - 自动游客登录与会话持久化

完成 iOS 端启动后自动获得游客身份的全链路：Keychain 持久化 guestUid 与 token、APIClient 自动注入 Bearer token、无效 token 静默重新登录。**节点 2 完成后用户应该 0 操作进入 App 即得身份**。

### Story 5.1: Keychain 封装（guestUid + token 持久化）

As an iOS 开发,
I want 一个抽象的 KeychainStore 协议 + 真实实现 + mock 实现,
So that 业务层可以无缝在测试中替换为 mock，生产环境用真实 Keychain.

**Acceptance Criteria:**

**Given** Epic 2 iOS 脚手架完成
**When** 完成本 story
**Then** 定义 `KeychainStore` 协议（含 `read(key:) async throws -> String?` / `write(key:, value:) async throws` / `delete(key:) async throws`）
**And** 实现 `KeychainStoreImpl`，基于 Apple Security framework 的 `kSecClassGenericPassword`
**And** 已知 key 常量化: `KeychainKey.guestUid` / `KeychainKey.authToken`
**And** Keychain 访问选 `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`（设备解锁后可读，不跨设备同步）
**And** 卸载重装后 Keychain 数据**仍可保留**（iOS 默认行为，验证 NFR7）
**And** 提供 `KeychainStoreMock` 用于测试（in-memory dictionary）
**And** **单元测试覆盖**（≥4 case，使用 KeychainStoreMock）:
- happy: write 后 read 返回相同值
- edge: read 不存在的 key → 返回 nil（不抛 error）
- happy: delete 后再 read → 返回 nil
- edge: 同一 key 写两次 → 第二次覆盖第一次
**And** **集成测试覆盖**（在模拟器跑 KeychainStoreImpl）: write guestUid → 终止 App 重启 → read 返回相同值

### Story 5.2: 启动自动登录 UseCase

As an iPhone 用户,
I want App 启动时自动登录（首次创建身份、再次启动复用）,
So that 我打开 App 直接进主界面，没有任何登录交互.

**Acceptance Criteria:**

**Given** Story 5.1 KeychainStore 可用 + Story 4.6 server /auth/guest-login 接口可用
**When** App 启动
**Then** `GuestLoginUseCase` 按以下逻辑执行:
- 从 Keychain 读 guestUid
- 不存在 → 生成新 UUID 字符串作为 guestUid，写 Keychain
- 调用 `/auth/guest-login` 带 guestUid + 设备信息（platform=ios, appVersion, deviceModel）
- 成功 → 把 token 写 Keychain，user / pet 注入 SessionManager
- 失败 → 抛错给 ErrorPresenter 走 RetryView
**And** UseCase 通过依赖注入接受 `KeychainStore` + `APIClient` 协议（便于测试 mock）
**And** App 入口（`PetAppApp.swift` 或 `RootView`）在 `.task { ... }` 中 await GuestLoginUseCase 完成才渲染主界面
**And** 登录中显示 launching state（占位 LoadingView）
**And** **单元测试覆盖**（≥5 case，mocked KeychainStore + mocked APIClient）:
- happy: Keychain 无 guestUid → 生成新 UUID → 写 Keychain → 调用接口成功 → 写 token → SessionManager 含 user
- happy: Keychain 已有 guestUid → 直接调用接口 → 拿同一 user_id（mock 返回固定 id）
- edge: APIClient 网络失败 → UseCase 抛 APIError.network
- edge: APIClient 业务失败（code=2001 等）→ UseCase 抛 APIError.business
- edge: Keychain write 失败 → UseCase 抛特定 error，不写 token
**And** **集成测试覆盖**（XCUITest）:
- 全新模拟器安装 App → 启动 → 等待主界面出现 → 验证主界面显示 SessionManager 中的 nickname
- 杀进程重启 → 验证主界面仍显示同一 nickname（说明 guestUid 复用成功）

### Story 5.3: APIClient interceptor 自动注入 Bearer token

As an iOS 开发,
I want APIClient 自动从 Keychain 读 token 并注入 Authorization header,
So that 业务层调用 API 不必手动管理 token，符合 V1接口设计 §2.3 鉴权约定.

**Acceptance Criteria:**

**Given** Story 5.1 KeychainStore + Epic 2 APIClient 已就绪
**When** 完成本 story
**Then** APIClient 在每个请求发出前自动:
- 检查 Endpoint 的 `requiresAuth`（默认 true，登录接口设 false）
- requiresAuth=true 时从 KeychainStore 读 token
- 存在 token → 注入 `Authorization: Bearer <token>` header
- 不存在 token 但 requiresAuth=true → 抛 `APIError.unauthorized`（不发请求）
**And** Endpoint enum 中 `/auth/guest-login` 等无需 auth 的接口标 `requiresAuth = false`
**And** 通过依赖注入持有 KeychainStore，便于 mock
**And** **单元测试覆盖**（≥4 case，mocked KeychainStore + mock URLSession）:
- happy: requiresAuth=true + Keychain 有 token → 请求 URL header 含 `Authorization: Bearer xxx`
- happy: requiresAuth=false → header 无 Authorization
- edge: requiresAuth=true 但 Keychain 无 token → 抛 APIError.unauthorized，不发请求
- edge: 同一 APIClient 实例并发 100 个请求 → 都正确注入 header（验证线程安全）

### Story 5.4: 无效 token 静默重新登录

As an iPhone 用户,
I want token 失效（如服务端重启 / 过期）时 App 自动重新登录,
So that 我不会突然被踢回登录状态，体验持续无感.

**Acceptance Criteria:**

**Given** Story 5.2-5.3 完成
**When** APIClient 收到任意接口返回 401（APIError.unauthorized）
**Then** 触发 `SilentReloginUseCase` 自动执行:
- 用 Keychain 中现有 guestUid 调 `/auth/guest-login`
- 拿到新 token → 更新 Keychain
- **重试原始请求一次**
- 如果重试仍失败 / 重新登录也失败 → 走 ErrorPresenter 显示 RetryView
**And** 每个原始请求最多触发 1 次静默重登（避免无限循环）
**And** 静默重登过程中不阻塞其他业务请求 UI（用 Combine / async-await 协调）
**And** 静默重登成功时不弹任何 UI 提示（用户无感）
**And** **单元测试覆盖**（≥5 case，mocked APIClient + mocked KeychainStore）:
- happy: 第一次 401 → 重登成功 → 重试原始请求成功 → 用户感知不到
- edge: 重登也失败（network error）→ 抛 APIError 走 RetryView
- edge: 重登成功但重试仍 401（异常情况）→ 不再二次重登，抛 unauthorized
- edge: 同一时间多个请求都 401 → 只触发一次重登，多个请求共享新 token
- edge: 重登过程中又收到 401 → 排队等待重登完成，复用结果

### Story 5.5: LoadHomeUseCase + 主界面用 GET /home 一次拉取全部数据

As an iPhone 用户,
I want App 启动后单次调用拿到主界面所有数据，不再 5 个串行 API,
So that 首屏快 ~800ms + 首页 loading 状态简化.

**Acceptance Criteria:**

**Given** Story 4.8 server GET /home 已可用 + Story 5.2 GuestLoginUseCase 已就绪
**When** 完成本 story
**Then** 实装 `LoadHomeUseCase`:
- 输入: 无参（依赖 SessionManager 中的 token）
- 输出: HomeData 含 user / pet / stepAccount / chest / room.currentRoomId
- 调 GET /home 单次拉取
- 错误处理: 失败走 Story 2.6 ErrorPresenter
**And** 改造 GuestLoginUseCase（Story 5.2）后续 chain:
- Story 5.2 GuestLoginUseCase 完成后，**接着调 LoadHomeUseCase**（不再让主界面自己分别调 /me + /steps/account + /chest/current 等）
- HomeData 注入 HomeViewModel，主界面所有占位区块（猫 / 步数 / 宝箱）一次性 populate
**And** 主界面**不再分别调** `/me` / `/steps/account` / `/chest/current` / `/cosmetics/inventory`（节点 2 阶段都不需要）
- 节点 4 后房间字段也通过 /home 拿（Story 11.10 server 端补，client 自动用上）
- 节点 7 后宝箱状态通过 /home 拿（chest 字段已包含）
- 节点 8 后装备字段通过 /home pet.equips 拿（Story 26.6 server 端补）
**And** **单元测试覆盖**（≥5 case，mocked APIClient）:
- happy: GET /home 返回完整 HomeData → ViewModel 状态全部 populate
- happy: pet.equips=[] → ViewModel.equippedItems = []，主界面猫无装备显示
- happy: room.currentRoomId=null → 主界面"进入房间"按钮文案为"创建 / 加入房间"
- edge: GET /home 失败 → ViewModel 走 RetryView，主界面不显示 placeholder
- happy: 重新登录后再调 → ViewModel 数据刷新
**And** **集成测试覆盖**（XCUITest + mock server）: 启动 → 验证 mock server 收到 1 次 /auth/guest-login + 1 次 /home（**不是** 5 次串行）+ 主界面 populate 完整

## Epic 6: 对齐 - 节点 2 启动即得身份联调

把 Epic 4 (Server) + Epic 5 (iOS) 真实串联，验证节点 2 §4.2 全部验收标准达成。重点：用户首次启动 + 重启都能拿到正确身份，token 失效场景自动恢复。

### Story 6.1: 跨端集成测试场景 - 自动登录 E2E

**Acceptance Criteria:**

**Given** Epic 4 + Epic 5 全部完成
**When** 完成本 story
**Then** 输出文档 `_bmad-output/implementation-artifacts/e2e/node-2-auth-e2e.md`，含:
- 准备步骤: server 启动 + 模拟器全新安装 App
- 验证场景 1: 首次启动 → 模拟器 Console 看到 GuestLoginUseCase 触发 → /auth/guest-login 响应 200 → 主界面显示 nickname `用户{id}`
- 验证场景 2: 杀进程 → 重启 App → 看到同一 nickname（说明 guestUid 复用）
- 验证场景 3: 用 `mysql -e "SELECT * FROM users"` 验证 DB 只有 1 行（不是每次启动新建）
- 验证场景 4: 用 `mysql -e "SELECT * FROM user_auth_bindings"` 验证 binding 存在 + auth_type=1
- 验证场景 5: server 端手动 `UPDATE user_chests SET unlock_at=NOW() WHERE user_id=?` 模拟首次 chest 创建（验证初始化事务确实创建了 chest）
- 验证场景 6 (token 失效): server 端重启（生成新 token_secret 让旧 token 失效）→ App 调用任意接口 401 → 静默重登成功 → 用户无感
**And** 文档含截图位

### Story 6.2: 节点 2 demo 验收

**Acceptance Criteria:**

**Given** Story 6.1 E2E 文档已就绪
**When** 完成本 story
**Then** 按 §4.2 节点 2 验收标准逐项打勾:
- [ ] 用户首次启动 App 可自动获得游客身份
- [ ] 再次启动后不会丢失身份
- [ ] Server 可识别同一个游客账号
- [ ] 首页后续接口可带登录态访问
- [ ] **新增**: token 失效后静默重登成功（节点 2 范围扩展，因为 Story 5.4 已经覆盖）
**And** 输出 demo 录屏到 `_bmad-output/implementation-artifacts/demo/node-2-demo-<date>/`
**And** 任何未通过项必须登记原因 + 是否后置

### Story 6.3: 文档同步与 tech debt 登记

**Acceptance Criteria:**

**Given** Epic 4-5 完成 + Story 6.2 demo 通过
**When** 完成本 story
**Then** 检查并更新（无变更也要明确标注）:
- [ ] `docs/宠物互动App_V1接口设计.md` §4.1 / §4.3: 实装是否与 Story 4.1 锚定的 schema 完全一致？有差异要么改文档要么改代码（按 AR28 契约不可改）
- [ ] `docs/宠物互动App_数据库设计.md` §5.1-5.6: migrations 实际生成的表结构是否与文档一致？特别是 5 张表的索引、唯一约束、字段类型
- [ ] `docs/宠物互动App_时序图与核心业务流程设计.md` §4 (游客登录初始化): 时序图与实装是否一致？
- [ ] `docs/宠物互动App_Go项目结构与模块职责设计.md`: 实际目录与 §4 是否对齐
**And** `_bmad-output/implementation-artifacts/tech-debt-log.md` 追加节点 2 的 tech debt，例如可能的:
- rate_limit 用内存 token bucket，多实例部署时各自计数（节点 9+ 接 Redis 后切换为 Redis-based）
- migration 工具未做生产环境保护（如禁止 down），生产部署前需补
- token secret 用了硬编码默认值，真上线前必须改成环境变量注入

## Epic 7: Server - 步数同步接口与账户记账 + dev 步数发放

节点 3 Server 端：步数差值入账 + 账户读取 + dev 端点（让 demo 不必真走 1000 步）。

### Story 7.1: 接口契约最终化（POST /steps/sync + GET /steps/account）

As a 跨端契约负责人,
I want 把步数同步与账户读取的 schema 锚定到 V1接口设计文档,
So that iOS Epic 8 可以基于稳定契约并行开工.

**Acceptance Criteria:**

**Given** docs/宠物互动App_V1接口设计.md §6.1 / §6.2 已有初稿
**When** 完成本 story
**Then** §6.1 (POST /steps/sync) schema 完全锚定:
- request: `{syncDate: string ISO date, clientTotalSteps: int, motionState: int (1/2/3), clientTimestamp: int64 ms}`
- response data: `{acceptedDeltaSteps: int, stepAccount: {totalSteps, availableSteps, consumedSteps}}`
- 错误码: 1002 / 3001 步数同步数据异常
**And** §6.2 (GET /steps/account) schema 完全锚定（response data: `{totalSteps, availableSteps, consumedSteps}`）
**And** 标注"契约自此 Epic 起冻结"
**And** Git commit 单独提交契约定稿

### Story 7.2: user_step_sync_logs migration

As a 服务端开发,
I want user_step_sync_logs 表的 migration 文件,
So that 步数同步可以记录每次客户端上报，便于审计和增量计算.

**Acceptance Criteria:**

**Given** Epic 4 migrations 框架已就绪（Story 4.3）
**When** 完成本 story
**Then** `migrations/0006_init_user_step_sync_logs.sql` 按 数据库设计.md §5.5 创建表:
- 字段全集（含 `id`, `user_id`, `sync_date`, `client_total_steps`, `accepted_delta_steps`, `motion_state`, `source`, `client_ts`, `created_at`）
- 索引 `idx_user_date (user_id, sync_date)` + `idx_user_created_at (user_id, created_at)`
**And** 含 down.sql
**And** **单元测试覆盖**（≥3 case）:
- happy: migrate up 后表存在 + 字段类型 + 索引都符合 §5.5
- happy: migrate down 后表删除
- edge: 重复 migrate up → 幂等不报错
**And** **集成测试覆盖**（dockertest）: migrate up → SHOW CREATE TABLE 对比 § schema → migrate down

### Story 7.3: POST /steps/sync 接口 + 累计差值入账 service

As an iPhone 用户,
I want 我每次上报当日累计步数后服务端正确入账增量,
So that 我的可用步数与现实步数同步增长，可用于开宝箱等消费.

**Acceptance Criteria:**

**Given** Story 7.2 表已就绪 + Epic 4 auth 中间件已挂载
**When** 调用 `POST /steps/sync` 带合法 payload
**Then** service 按以下逻辑:
- 查 `user_step_sync_logs WHERE user_id=? AND sync_date=?` 取最近一条
- 没有最近 → delta = clientTotalSteps（首次）
- 有最近 → delta = max(0, clientTotalSteps - lastClientTotalSteps)
- 在事务中: 更新 user_step_accounts (total_steps += delta, available_steps += delta) + 插入 user_step_sync_logs
- 返回 acceptedDeltaSteps + 最新账户
**And** 接口要求 auth（带 Bearer token），无 token 返回 1001
**And** **syncDate 由 client 提供（按 client 时区算"今天"），server 直接采用不二次转换**（避免跨时区漂移）—— GAP E 修补
**And** **步数防作弊阈值（GAP K 修补）**：
- 单次 sync 的 `delta` > **5000** → 视为可疑（log warning）+ delta 截断为 5000（不彻底拒绝，避免误伤真实跑步用户）
- 单日（同 sync_date）累计 `accepted_delta_steps` 总和 > **50000** → 当日后续 sync 一律 delta=0 + log warning + 返回 3001 步数同步数据异常
- 阈值通过配置 `steps.daily_cap` / `steps.single_sync_cap` 可调
- **单元测试**新增 case：单次 delta=10000 → 实际入账 5000 + log warning；单日累计已达 50000 后再 sync → delta=0 + 3001
- 该限制仅 server 端实施，iOS 端不加限制（server 是权威）
**And** **单元测试覆盖**（≥6 case，mocked repo）:
- happy: 首次同步（无历史记录）→ delta = clientTotalSteps
- happy: 非首次同步，clientTotalSteps 增加 → delta = 差值
- edge: clientTotalSteps 倒退（< lastClientTotalSteps）→ delta = 0，仍写日志
- edge: 重复同步（相同 clientTotalSteps）→ delta = 0
- edge: 跨自然日（sync_date 不同）→ 按新一天起算（不读上一天 lastTotal）
- edge: 同步时事务失败（mock account repo 抛 error）→ 整体回滚，sync_log 也未写入
**And** **集成测试覆盖**（dockertest）:
- 首次 sync clientTotalSteps=100 → DB sync_log + account.total=100, available=100
- 第二次 sync clientTotalSteps=180 → account.total=180, available=180
- 第三次 sync clientTotalSteps=150（倒退）→ account.total=180 不变，但 sync_log 仍新增

### Story 7.4: GET /steps/account 接口

As an iPhone 用户,
I want 任何时候可以查询当前步数账户的三档值,
So that 主界面可以显示我的总步数 / 可用 / 已消耗.

**Acceptance Criteria:**

**Given** Story 7.3 user_step_accounts 已经在登录时初始化（Epic 4）
**When** 调用 `GET /steps/account`
**Then** 返回 `{totalSteps, availableSteps, consumedSteps}` 来自 DB
**And** 接口要求 auth
**And** account 不存在（理论不该发生，因登录时已初始化）→ 返回 1003 资源不存在
**And** **单元测试覆盖**（≥3 case，mocked repo）:
- happy: account 存在 → 返回三档值
- edge: account 不存在 → 1003
- edge: 三档值都为 0（新用户）→ 返回 0/0/0
**And** **集成测试覆盖**（dockertest）: 创建用户 + step_account → curl GET /steps/account → 返回正确值

### Story 7.5: dev 端点 POST /dev/grant-steps

As a demo / 开发者,
I want 一个 build flag gated 的 dev 接口给指定用户加步数,
So that demo 时不必真走 1000 步就能演示开宝箱.

**Acceptance Criteria:**

**Given** Epic 1 Story 1.6 Dev Tools 框架已就绪 + Story 7.3 步数 service 可用
**When** 仅在 `BUILD_DEV=true` 模式调用 `POST /dev/grant-steps {userId: int64, steps: int}`
**Then** service 直接增加 `user_step_accounts.total_steps += steps, available_steps += steps`
**And** 同时写一条 sync_log（source=admin_grant，标识来源是 dev）
**And** 生产构建（BUILD_DEV=false）下访问该端点返回 404
**And** 接口**不**要求 auth（因为是 dev 内部用）
**And** **单元测试覆盖**（≥4 case）:
- happy: dev mode + 用户存在 → 正确加步数
- edge: dev mode + 用户不存在 → 返回 1003
- edge: dev mode + steps 为负数 → 返回 1002 参数错误
- edge: 非 dev mode → 路由返回 404（由 Epic 1 dev_only 中间件保证）
**And** **集成测试覆盖**（dockertest + BUILD_DEV=true）:
- 创建用户 → /dev/grant-steps 加 5000 → /steps/account 返回 available=5000

## Epic 8: iOS - 小猫三态本地展示与 HealthKit / CoreMotion 接入

节点 3 iOS 端：HealthKit 步数读取 + CoreMotion 状态识别 + sprite 三态动画 + 步数同步触发器。猫 sprite 资源用占位（节点 3 不阻塞美术）。

### Story 8.1: HealthKit 接入（权限 + 当日累计步数读取）

As an iOS 开发,
I want 一个抽象 HealthProvider 协议 + 真实实现 + mock 实现,
So that 业务层可以无缝在测试中替换为 mock.

**Acceptance Criteria:**

**Given** Epic 2 iOS 脚手架完成
**When** 完成本 story
**Then** 定义 `HealthProvider` 协议（含 `requestPermission() async throws -> Bool` / `readDailyTotalSteps(date: Date) async throws -> Int`）
**And** 实现 `HealthProviderImpl`，基于 `HKHealthStore` 读 `HKQuantityType(.stepCount)` 当日 total
**And** Info.plist 添加 `NSHealthShareUsageDescription`（用户可见的权限说明文案）
**And** 权限**按需**申请（不在 App 启动时一次性弹，符合 AR17）—— 第一次需要步数时才申请
**And** 提供 `HealthProviderMock`（in-memory 假步数）用于测试
**And** **单元测试覆盖**（≥4 case，使用 HealthProviderMock）:
- happy: requestPermission 返回 true → readDailyTotalSteps 返回设定值
- edge: requestPermission 返回 false → readDailyTotalSteps 抛 .permissionDenied
- happy: 同一天读两次 → 第二次直接返回缓存值（避免重复 HK 查询）
- edge: 跨自然日（系统时间跨过 0 点）→ 重新查询新一天的累计
**And** **集成测试覆盖**（在模拟器跑 HealthProviderImpl）: 模拟器 HealthKit 已预注入步数数据 → readDailyTotalSteps 返回正确值

### Story 8.2: CoreMotion 接入（权限 + 状态识别）

As an iOS 开发,
I want 一个抽象 MotionProvider 协议 + 真实实现 + mock 实现,
So that 业务层可以监听设备运动状态变化.

**Acceptance Criteria:**

**Given** Epic 2 iOS 脚手架完成
**When** 完成本 story
**Then** 定义 `MotionProvider` 协议（含 `requestPermission() async throws -> Bool` / `startUpdates(handler: (CMMotionActivity) -> Void)` / `stopUpdates()`）
**And** 实现 `MotionProviderImpl`，基于 `CMMotionActivityManager.startActivityUpdates`
**And** Info.plist 添加 `NSMotionUsageDescription`
**And** 权限**按需**申请（AR17）
**And** 提供 `MotionProviderMock`（手动注入 CMMotionActivity 序列）用于测试
**And** **单元测试覆盖**（≥4 case，使用 MotionProviderMock）:
- happy: requestPermission 成功 → startUpdates 后 handler 收到注入的事件
- edge: requestPermission 失败 → startUpdates 不触发任何回调
- happy: stopUpdates 后 handler 不再收到事件
- edge: 同时多次 startUpdates → 只生效第一次（防止重复订阅）

### Story 8.3: 运动状态机映射（CMMotion → rest/walk/run）

As an iOS 开发,
I want 一个 MotionStateMapper 把原始 CMMotionActivity 转换成业务三态,
So that ViewModel 不直接处理底层 CoreMotion 类型，规则集中可测.

**Acceptance Criteria:**

**Given** Story 8.2 提供 CMMotionActivity 事件流
**When** 完成本 story
**Then** 定义 `enum MotionState { case rest, walk, run }`
**And** `MotionStateMapper.map(_ activity: CMMotionActivity) -> MotionState` 按以下规则:
- `running == true` → .run
- `walking == true` → .walk
- `stationary == true` → .rest
- 其他（如 cycling / automotive）→ .rest（按设计文档约定，"坐下"按"静止"处理）
- confidence < .low → 保持上一次状态（防抖）
**And** **单元测试覆盖**（≥6 case）:
- happy: stationary=true → .rest
- happy: walking=true → .walk
- happy: running=true → .run
- happy: cycling=true（其他类型）→ .rest
- edge: 多个 flag 同时为 true（如 walking + stationary）→ 优先级 run > walk > rest
- edge: confidence=.low → 不切换状态，返回 nil 或上一次值（具体策略二选一并测试）

### Story 8.4: 主界面猫 sprite 三态动画切换

As an iPhone 用户,
I want 主界面的猫根据我当前的运动状态自动切换动画,
So that 我能直观看到自己运动时猫也在跑或走.

**Acceptance Criteria:**

**Given** Story 2.2 主界面有猫展示区占位 + Story 8.3 状态机可用
**When** 完成本 story
**Then** 实装 `PetSpriteView(state: MotionState)` SwiftUI 组件:
- state == .rest → 显示 idle 动画（呼吸 / 摇尾，loop）
- state == .walk → 显示 walk 动画（走路循环，loop）
- state == .run → 显示 run 动画（跑步循环，loop）
- state 切换时有平滑过渡（淡入淡出 200ms）
**And** Sprite 资源用占位 SF Symbol 或简单几何形状（美术资产由后续节点补，节点 3 阶段不阻塞）
**And** PetSpriteView 替换 Story 2.2 的猫展示区占位（`Rectangle().fill(.gray)` → 实际组件）
**And** `HomeViewModel` 持有 `@Published var petState: MotionState = .rest`，订阅 MotionProvider 状态变化驱动更新
**And** **单元测试覆盖**（≥4 case，mocked MotionProvider + ViewModel）:
- happy: ViewModel 启动时订阅 MotionProvider，初始状态 = .rest
- happy: MotionProvider 推 walk activity → mapper 转 .walk → ViewModel.petState = .walk
- happy: 连续切换 rest → walk → run → rest，ViewModel 状态正确流转
- edge: ViewModel deinit 时取消 MotionProvider 订阅（避免内存泄漏）
**And** **UI 测试覆盖**（XCUITest）: 在主界面手动注入 MockMotionProvider 触发 .run → 验证 PetSpriteView 的 accessibility identifier 切到 "petSprite_run"

### Story 8.5: 步数同步触发器（启动 / 回前台 / 定时 / 开箱前）

As an iPhone 用户,
I want App 在合适时机自动同步我的累计步数到服务端,
So that 我不必手动点同步，可用步数总是接近真实值.

**Acceptance Criteria:**

**Given** Story 7.3 server 端 /steps/sync 可用 + Story 8.1 HealthProvider 可读步数
**When** 完成本 story
**Then** 定义 `StepSyncTriggerService`，订阅以下时机触发 `SyncStepsUseCase`:
- App 启动后进入主界面
- App 从后台回到前台（UIScene `didActivate` 通知）
- 主界面停留期间每 5 分钟定时同步一次
- **手动触发接口**（开箱前供 ChestOpenUseCase 调用，节点 7 用）
**And** SyncStepsUseCase 内部:
- 读 HealthProvider 当日累计步数
- 调用 server `/steps/sync` 带 `{syncDate, clientTotalSteps, motionState, clientTimestamp}`
- 成功 → 更新 `HomeViewModel.stepAccount`
- 失败 → 不阻塞 UI（背景同步），下次再试
**And** 同步**不重叠**：当前同步未完成时，新触发被忽略（不排队）
**And** **单元测试覆盖**（≥6 case，mocked HealthProvider + APIClient）:
- happy: App 启动触发 → 读 health 步数 → 调 sync → 更新 stepAccount
- happy: 回前台触发 → 同上
- happy: 定时器每 5 分钟触发一次（用 fake timer）
- happy: 手动触发（开箱前）→ 同上
- edge: 上一次同步 in-flight，新触发到达 → 被忽略，不重复请求
- edge: 同步失败（network error）→ 不更新 stepAccount，下次定时器到达再试
**And** **集成测试覆盖**（XCUITest + mock server）: 模拟器启动 App → 验证 mock server 收到 /steps/sync 请求 → 主界面步数显示更新

## Epic 9: 对齐 - 节点 3 步数链路 + 本地猫动作联调

把 Epic 7 (Server) + Epic 8 (iOS) 真实串联，验证节点 3 §4.3 全部验收标准达成。重点：HealthKit 步数能正确同步到 server + 猫 sprite 随运动状态切换。

### Story 9.1: 跨端集成测试场景 - 步数 + 猫动作 E2E

**Acceptance Criteria:**

**Given** Epic 7 + Epic 8 全部完成
**When** 完成本 story
**Then** 输出文档 `_bmad-output/implementation-artifacts/e2e/node-3-steps-cat-e2e.md`，含:
- 准备步骤: server 启动 + 模拟器全新安装 App + 模拟器 HealthKit 预注入步数（如 2000 步）
- 验证场景 1（步数同步）: App 启动 → HealthKit 权限弹窗（首次）→ 同意 → /steps/sync 调用 → server 端 `mysql -e "SELECT * FROM user_step_accounts WHERE user_id=?"` 验证 total_steps=2000, available_steps=2000
- 验证场景 2（增量入账）: 模拟器追加 HealthKit 步数到 2500 → 等待 5 分钟定时同步（或 kill+重启 App）→ server account.total_steps=2500
- 验证场景 3（倒退保护）: 用 dev 接口手动 SET 用户 account.total_steps=3000 → App 同步 clientTotalSteps=2500（小于 server 已有）→ delta=0，total 不变
- 验证场景 4（猫 sprite 切换）: 用 MockMotionProvider 在 App 内注入 walking → 验证主界面猫 sprite 切到 walk 动画 → 注入 running → 切到 run → 注入 stationary → 切到 rest
- 验证场景 5（开箱前手动同步）: 在主界面手动触发 syncStepsUseCase（节点 3 还没开箱，可临时加 debug 按钮触发）→ 验证 server 收到 sync 请求
- 验证场景 6（dev 步数发放）: BUILD_DEV=true server，调 `/dev/grant-steps {userId, steps:5000}` → App 主界面步数显示 +5000
**And** 文档含截图位

### Story 9.2: 节点 3 demo 验收

**Acceptance Criteria:**

**Given** Story 9.1 E2E 文档已就绪
**When** 完成本 story
**Then** 按 §4.3 节点 3 验收标准逐项打勾:
- [ ] App 内小猫三种状态能正常展示（rest / walk / run）
- [ ] 客户端可发起步数同步请求
- [ ] 服务端能记录并返回步数同步结果
- [ ] **新增**: dev 步数发放可用（demo 必备）
**And** 输出 demo 录屏到 `_bmad-output/implementation-artifacts/demo/node-3-demo-<date>/`
**And** 任何未通过项必须登记原因 + 是否后置

### Story 9.3: 文档同步与 tech debt 登记

**Acceptance Criteria:**

**Given** Epic 7-8 完成 + Story 9.2 demo 通过
**When** 完成本 story
**Then** 检查并更新（无变更也要明确标注）:
- [ ] `docs/宠物互动App_V1接口设计.md` §6.1 / §6.2: 实装是否与 Story 7.1 锚定的 schema 完全一致
- [ ] `docs/宠物互动App_数据库设计.md` §5.5: user_step_sync_logs 表结构与文档一致
- [ ] `docs/宠物互动App_时序图与核心业务流程设计.md` §6 (步数同步): 时序图与实装一致
- [ ] `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §6.4 (Steps 模块): HealthKit / CoreMotion 抽象方式
**And** `_bmad-output/implementation-artifacts/tech-debt-log.md` 追加节点 3 tech debt，可能的:
- HealthKit 步数读取无背景刷新（App 不在前台时不更新），后续可考虑 HealthKit Background Delivery
- 猫 sprite 用占位资源，真美术资产待定（建议节点 7+ 一起定）
- StepSyncTriggerService 定时间隔写死 5 分钟，未抽到配置

## Epic 10: Server - WebSocket 网关基础设施 + Redis 接入 + 心跳框架

节点 4 第一个 Server Epic。**首次引入 Redis** + 完整 WS 框架（连接管理 / 心跳 / 广播 primitive / presence 抽象）。本 Epic **不含房间业务**，业务在 Epic 11。

### Story 10.1: 接口契约最终化（WS 协议）

As a 跨端契约负责人,
I want 把 WebSocket 协议（连接 URL / room.snapshot / ping / pong / error）schema 锚定到 V1接口设计文档,
So that iOS Epic 12 和 Server Epic 11 可以基于稳定 WS 协议并行开工.

**Acceptance Criteria:**

**Given** docs/宠物互动App_V1接口设计.md §12 已有 WS 协议初稿
**When** 完成本 story
**Then** §12.1 连接地址 schema 完全锚定: `GET /ws/rooms/{roomId}?token=xxx`，含 token 校验失败 / 用户不在该房间时的 close code
**And** §12.3 服务端→客户端消息 schema 锚定（基础消息）:
- `room.snapshot`: payload schema（room + members 数组结构，不含具体成员宠物状态字段，由 Epic 14 填）
- `pong`: payload schema
- `error`: payload schema (含 code / message)
**And** §12.2 客户端→服务端消息 schema 锚定:
- `ping`: payload schema
**And** **业务消息（member.joined / member.left / emoji / pet.state.changed）由后续 Epic 各自 Story X.1 锚定**，本 Story 不涉及
**And** 标注"WS 基础协议自此 Epic 起冻结"

### Story 10.2: Redis 接入（连接池 + 配置 + fail-fast）

As a 服务端开发,
I want server 能连上 Redis 并提供统一 client 抽象,
So that 后续 Story 10.6 (presence) + Epic 4 限频升级 + Epic 20 幂等键都能用同一个 Redis 实例.

**Acceptance Criteria:**

**Given** Epic 1 脚手架完成（无 Redis 接入）
**When** 完成本 story
**Then** `internal/infra/redis/` 目录提供 Redis 连接初始化（基于 YAML 配置 `redis.addr / password / db / pool_size`）
**And** server 启动时 Redis ping 失败则**fail-fast**
**And** 提供 `RedisClient` 抽象（含基础 GET / SET / DEL / EXPIRE / SADD / SREM / SMEMBERS）
**And** 提供 `RedisClientMock`（基于 miniredis 或 in-memory map）用于单元测试
**And** **单元测试覆盖**（≥4 case，使用 RedisClientMock）:
- happy: SET → GET 返回相同值
- happy: SET 带 EXPIRE → 过期后 GET 返回 nil
- happy: SADD 多次 + SMEMBERS → 返回去重集合
- edge: GET 不存在的 key → 返回 nil（不抛 error）
**And** **集成测试覆盖**（dockertest 起真实 Redis）:
- 启动 → ping → SET / GET / SADD / SMEMBERS → 验证一致

### Story 10.3: WS 网关骨架（连接管理 + Session 抽象）

As a 服务端开发,
I want 一个 WS gateway 管理所有客户端连接，每个连接抽象为 Session 对象,
So that 后续业务 Epic 通过 Session 收发消息，不直接处理 Gorilla / standard library WS 细节.

**Acceptance Criteria:**

**Given** Story 10.2 Redis 已接入 + Epic 4 auth 中间件可用
**When** 完成本 story
**Then** 选定 WS 库（推荐 gorilla/websocket 或 nhooyr/websocket，在 0004-ws-stack.md 决策文档记录）
**And** `internal/app/ws/gateway.go` 提供 `GET /ws/rooms/:roomId` handler:
- 解析 query `?token=xxx` → 校验 token（复用 Epic 4 token util）
- 校验通过后创建 `Session` 对象（含 userID, roomID, conn, sendChan, lastHeartbeatAt）
- 注册到全局 SessionManager
- 启动读 / 写 goroutine
**And** Session 提供 `Send(msg)` / `Close()` 接口
**And** 不在该房间的用户连接 → 返回 close code 4003 + 错误消息
**And** **单元测试覆盖**（≥5 case，mocked WS conn）:
- happy: 合法 token + 用户在该房间 → Session 创建成功
- edge: token 无效 → 拒绝连接，返回 close code 4001
- edge: 用户不在该房间（mocked room repo 返回 false）→ 拒绝，close code 4003
- happy: Send msg → mocked conn 收到正确序列化字节流
- happy: Close → 移除 SessionManager + 关闭 goroutine
**And** **集成测试覆盖**（真实 WS client + dockertest Redis + MySQL）:
- 真实 WS 拨号 → 连接建立 → 服务端 Send 一条消息 → 客户端收到

### Story 10.4: 心跳框架（ping/pong + 超时清理）

As a 服务端开发,
I want WS 连接自带心跳与超时清理,
So that 客户端断网时服务端能在合理时间内感知并释放资源.

**Acceptance Criteria:**

**Given** Story 10.3 Session 已就绪
**When** 完成本 story
**Then** Session 收到客户端 `ping` 消息 → 立即回 `pong`
**And** 后台定时任务（每 30 秒扫描）检查 `lastHeartbeatAt`，超过 60 秒（配置 `ws.heartbeat_timeout_sec`）未活跃的 Session 自动 Close
**And** Session Close 时触发清理钩子（注销 SessionManager + 触发广播 `member.left` 钩子，业务由 Epic 11 实装）
**And** **单元测试覆盖**（≥4 case）:
- happy: 收到 ping → 回 pong + 更新 lastHeartbeatAt
- happy: 60 秒内有 ping → Session 不被清理
- edge: 60 秒未活跃 → Session 被自动 Close + 清理钩子触发
- edge: 同时多个 Session 超时 → 都正确清理，无竞态
**And** **集成测试覆盖**（真实 WS）: 客户端连接 → 静默 70 秒 → 验证服务端 Session 已释放（SessionManager 中不存在）

### Story 10.5: BroadcastToRoom primitive

As a 服务端开发,
I want 一个 `BroadcastToRoom(roomID, msg)` 函数把消息推送给该房间内所有在线 Session,
So that 后续业务（成员事件 / 表情 / 宠物状态）可以直接调用，不必各自管理 Session 列表.

**Acceptance Criteria:**

**Given** Story 10.3 SessionManager 已就绪 + Story 10.6 Presence repo 即将就绪
**When** 完成本 story
**Then** `internal/app/ws/broadcast.go` 提供 `BroadcastToRoom(ctx, roomID, msg) error`:
- 从 Redis presence 拿 `room:{roomID}:online_users` 集合
- 对每个 userID 找对应 Session（从 SessionManager）
- 并发 Send 消息（用 goroutine）
- 返回成功广播数量
**And** Session Send 失败时不阻塞其他 Session（fire-and-forget + log）
**And** roomID 没有任何在线用户时 → 返回 0 + nil error（合法场景）
**And** **单元测试覆盖**（≥5 case，mocked SessionManager + presence）:
- happy: 房间 3 个在线用户 → Broadcast 调用 3 次 Send
- happy: 房间 0 个在线用户 → 返回 0 + nil
- edge: 1 个 Session Send 失败 → 其他 2 个仍正常发送
- edge: SessionManager 中 userID 不存在（presence 与 manager 不一致）→ skip 该 user，log 警告
- happy: 100 个并发 Broadcast 不同 room → 都正确
**And** **集成测试覆盖**: 启 3 个 WS 客户端 → 服务端 BroadcastToRoom → 验证 3 个客户端都收到消息

### Story 10.6: Redis presence repo（房间在线用户 + WS session 映射）

As a 服务端开发,
I want Redis 中维护"房间内在线 user 集合" + "user → ws session id" 映射,
So that BroadcastToRoom 可以快速查到目标 Session，且支持多服务实例（虽然 MVP 单实例，但提前抽好接口）.

**Acceptance Criteria:**

**Given** Story 10.2 Redis 接入 + Story 10.3 Session 创建/销毁钩子可挂
**When** 完成本 story
**Then** `internal/repo/redis/presence_repo.go` 提供:
- `AddOnline(roomID, userID, sessionID)`: SADD room:{roomID}:online_users userID + SET user:{userID}:ws_session sessionID + EXPIRE 自动过期保险
- `RemoveOnline(roomID, userID)`: SREM + DEL
- `IsOnline(roomID, userID) bool`
- `ListOnline(roomID) []userID`
**And** Session 创建时自动 AddOnline，Session Close 时自动 RemoveOnline（钩子在 10.3 的 Session lifecycle 内挂载）
**And** Key 自带 TTL（如 5 分钟，每次心跳 RENEW），防止崩溃后僵尸记录
**And** **单元测试覆盖**（≥5 case，使用 RedisClientMock / miniredis）:
- happy: AddOnline 后 IsOnline 返回 true
- happy: RemoveOnline 后 IsOnline 返回 false
- happy: ListOnline 返回正确 userID 列表
- edge: 同一 user 多次 AddOnline 同一 room → 只一份（SADD 去重）
- edge: TTL 到期后 ListOnline 不含该 user
**And** **集成测试覆盖**（dockertest Redis）: 50 个 user 并发 AddOnline → ListOnline 返回 50 个

### Story 10.7: 房间快照下发框架（业务内容由 E11 填充）

As a 服务端开发,
I want 一个 `SendRoomSnapshot(session, roomID)` 函数框架,
So that Session 建立后立即推送 room.snapshot 给客户端，后续 Epic 11 填充具体业务字段.

**Acceptance Criteria:**

**Given** Story 10.5 BroadcastToRoom + Story 10.6 Presence 都就绪
**When** 完成本 story
**Then** `internal/app/ws/snapshot.go` 提供 `SendRoomSnapshot(ctx, session, roomID)`:
- 调用 SnapshotBuilder 接口（接口定义在本 Story，**实现是 placeholder**：返回 `{room: {id: roomID, maxMembers: 4, memberCount: 0}, members: []}`）
- 通过 session.Send 发出 `room.snapshot` 消息
**And** SnapshotBuilder 接口签名: `BuildSnapshot(ctx, roomID) (Snapshot, error)`
**And** Epic 11 Story 11.7 会提供真实 SnapshotBuilder 实现（基于 room_members + presence）
**And** Session 创建后（Story 10.3 lifecycle 钩子）自动调用 SendRoomSnapshot
**And** **单元测试覆盖**（≥3 case，mocked SnapshotBuilder + Session）:
- happy: SnapshotBuilder 返回 placeholder snapshot → Session 收到 room.snapshot 消息
- edge: SnapshotBuilder 抛 error → Session 收到 error 消息（type=error, code=6005 房间状态异常）
- happy: 同一 Session 收到 snapshot 后再触发 SendRoomSnapshot → 正常发出（幂等）
**And** **集成测试覆盖**: WS 客户端连接 → 立即收到 placeholder room.snapshot 消息（room.id = 请求的 roomID, members = []）

## Epic 11: Server - 房间 CRUD + 房间快照内容 + 成员事件广播

节点 4 第二个 Server Epic。基于 Epic 10 的 WS 框架填充房间业务，含 Layer 2 集成测试 Story 11.9。

### Story 11.1: 接口契约最终化（房间 5 个 REST + 2 个 WS 消息）

As a 跨端契约负责人,
I want 把房间 CRUD 接口与 WS 业务消息 schema 锚定到 V1接口设计文档,
So that iOS Epic 12 可以基于稳定契约并行开工.

**Acceptance Criteria:**

**Given** docs/宠物互动App_V1接口设计.md §10 / §12.3 已有初稿
**When** 完成本 story
**Then** §10 REST 接口 schema 完全锚定:
- `POST /rooms`: 空 request, response `{room: {id, creatorUserId, maxMembers, memberCount, status}}`
- `GET /rooms/current`: response `{roomId: string | null}`
- `GET /rooms/{roomId}`: response `{room: ..., members: [{userId, nickname, avatarUrl, pet: {petId, currentState, equips}}]}`（注：pet.currentState 字段存在但本 Epic 暂返回固定值 1，由 Epic 14 真实驱动）
- `POST /rooms/{roomId}/join`: 空 request, response `{roomId, joined: true}`
- `POST /rooms/{roomId}/leave`: 空 request, response `{roomId, left: true}`
- 错误码: 6001 房间不存在 / 6002 房间已满 / 6003 用户已在房间中 / 6004 用户不在房间中 / 6005 房间状态异常
**And** §12.3 业务 WS 消息 schema 锚定:
- `member.joined`: payload `{userId, nickname}`
- `member.left`: payload `{userId}`
**And** 标注"契约自此 Epic 起冻结"

### Story 11.2: rooms + room_members migration

As a 服务端开发,
I want rooms 和 room_members 两张表的 migration 文件,
So that 节点 4 可以创建房间业务的持久化基础.

**Acceptance Criteria:**

**Given** Epic 4 migrations 框架已就绪
**When** 完成本 story
**Then** `migrations/0007_init_rooms.sql` 按 数据库设计.md §5.13 创建表（含 idx_creator_user_id + idx_status_created_at）
**And** `migrations/0008_init_room_members.sql` 按 §5.14 创建表，含**关键约束**:
- `UNIQUE(user_id)` （一个用户同时只能在一个房间）
- `UNIQUE(room_id, user_id)` （房间内同一用户只能出现一次）
- `KEY idx_room_id`
**And** 含 down.sql
**And** **单元测试覆盖**（≥3 case）:
- happy: migrate up 后两张表存在 + 字段类型 + 全部索引和唯一约束都符合 §5.13 / §5.14
- happy: migrate down 后表删除
- edge: 重复 migrate up → 幂等
**And** **集成测试覆盖**（dockertest）: migrate up → SHOW CREATE TABLE 对比 schema → 故意尝试违反 UNIQUE(user_id)（同 user 插两条 room_members）→ 数据库拒绝插入

### Story 11.3: 创建房间事务

As an iPhone 用户,
I want 我可以创建一个房间并自动作为第一个成员加入,
So that 我可以邀请朋友进来.

**Acceptance Criteria:**

**Given** Story 11.2 表已就绪 + Epic 4 tx manager 可用
**When** 调用 `POST /rooms`（带 auth token）
**Then** service 在事务中执行:
- 查 `users.current_room_id`，非 null → 返回 6003 用户已在房间中
- 插入 `rooms` (creator_user_id=当前 user, status=1, max_members=4)，拿到 room_id
- 插入 `room_members` (room_id, user_id)
- 更新 `users.current_room_id = room_id`
- 返回 `{room: {id, creatorUserId, maxMembers:4, memberCount:1, status:1}}`
**And** **单元测试覆盖**（≥4 case，mocked repo + tx manager）:
- happy: 用户不在任何房间 → 事务 3 步全部执行
- edge: 用户已在其他房间 → 立即返回 6003，不开事务
- edge: 插入 rooms 失败 → 事务回滚（mock）
- edge: 插入 room_members 失败 → rooms 也回滚
**And** **集成测试覆盖**（dockertest）:
- 创建用户 → POST /rooms → DB rooms 新增 1 行 + room_members 新增 1 行 + users.current_room_id 已更新
- 同一 user 再次 POST /rooms → 返回 6003

### Story 11.4: 加入房间事务

As an iPhone 用户,
I want 通过房间号加入一个已存在的房间,
So that 我可以和朋友在同一个房间互动.

**Acceptance Criteria:**

**Given** Story 11.3 已能创建房间
**When** 调用 `POST /rooms/{roomId}/join`
**Then** service 在事务中执行:
- 查 `users.current_room_id`，非 null → 6003
- 查目标 `rooms`，不存在 → 6001；status != 1 → 6005
- 查 `room_members WHERE room_id=?`，count >= 4 → 6002
- 插入 `room_members` (room_id, user_id)
- 更新 `users.current_room_id = room_id`
- 返回 `{roomId, joined: true}`
**And** **单元测试覆盖**（≥6 case，mocked repo）:
- happy: 房间存在 + 未满 + 用户不在其他房间 → 事务成功
- edge: 用户已在其他房间 → 6003
- edge: 房间不存在 → 6001
- edge: 房间已满 4 人 → 6002
- edge: 房间 status=closed → 6005
- edge: 并发竞态（用户同时被加入两个房间）→ DB UNIQUE(user_id) 兜底，第二个事务报 dup error，service 转 6003
**And** **集成测试覆盖**（dockertest）:
- 用户 A 创建房间 → 用户 B join → DB room_members 2 行 + B.current_room_id 更新
- 4 个用户都 join → 第 5 个 user join 返回 6002

### Story 11.5: 退出房间事务

As an iPhone 用户,
I want 我可以主动退出当前房间,
So that 我能换到别的房间或不再被通知.

**Acceptance Criteria:**

**Given** Story 11.4 已能加入房间
**When** 调用 `POST /rooms/{roomId}/leave`
**Then** service 在事务中执行:
- 查 `users.current_room_id`，非该 roomId → 6004 用户不在房间中
- 删除 `room_members WHERE room_id=? AND user_id=?`
- 更新 `users.current_room_id = NULL`
- 查 `room_members WHERE room_id=?` 剩余数，==0 → 更新 `rooms.status = 2 closed`
- 返回 `{roomId, left: true}`
**And** **单元测试覆盖**（≥5 case，mocked repo）:
- happy: 用户在房间且非最后一人 → 删除成员，房间仍 active
- happy: 用户是房间最后一人 → 删除后房间 closed
- edge: 用户不在该房间 → 6004
- edge: 房间已 closed（理论不会，因为用户必然不在 closed 房间）→ 6004
- edge: 删除 room_members 后查剩余出错（mock）→ 事务回滚，user 仍在房间
**And** **集成测试覆盖**（dockertest）:
- A + B 在房间 → A leave → DB room_members 剩 B 一行 + rooms.status 仍 = 1
- B leave → DB room_members 0 行 + rooms.status = 2

### Story 11.6: 房间详情查询

As an iPhone 用户,
I want 我可以查看自己当前所在房间号 + 任意房间的成员列表,
So that 主界面可以显示"当前房间"入口 + 房间页可以渲染成员.

**Acceptance Criteria:**

**Given** Story 11.3-11.5 完成
**When** 调用 `GET /rooms/current`
**Then** 返回 `{roomId: <users.current_room_id> | null}`
**When** 调用 `GET /rooms/{roomId}`
**Then** 返回 `{room: {id, creatorUserId, maxMembers, memberCount, status}, members: [...]}`
- members 含 `{userId, nickname, avatarUrl, pet: {petId, currentState, equips}}`
- pet.currentState 本 Epic 返回固定值 1（rest），节点 5 由 Epic 14 真实驱动
- pet.equips 本 Epic 返回 `[]`，节点 9-10 由 Epic 26 / 29 填充
**And** **单元测试覆盖**（≥4 case，mocked repo）:
- happy: 用户在房间 → /rooms/current 返回 roomId
- happy: 用户不在房间 → /rooms/current 返回 `{roomId: null}`
- happy: GET /rooms/{id} 返回正确 room + members 列表
- edge: GET /rooms/{id} 房间不存在 → 6001
**And** **集成测试覆盖**（dockertest）: 创建房间 + 3 个成员 → GET /rooms/{id} → members 数组长度 = 3 + 字段值正确

### Story 11.7: 房间快照真实实现（替换 E10.7 placeholder）

As a 服务端开发,
I want 实装 Story 10.7 留下的 SnapshotBuilder 接口,
So that WS 客户端连接后收到的 room.snapshot 含真实成员数据，不再是 placeholder.

**Acceptance Criteria:**

**Given** Story 10.7 已定义 SnapshotBuilder 接口 + Story 11.6 GET /rooms/{id} 查询逻辑可复用
**When** 完成本 story
**Then** 实装 `RoomSnapshotBuilder` 替换 placeholder:
- 读 `room_members WHERE room_id=?` + 关联 users / pets
- 读 Redis presence `room:{roomId}:online_users`（标识谁在线）
- 拼装 snapshot: `{room: {id, maxMembers, memberCount}, members: [{userId, nickname, pet: {petId, currentState}}]}`
- pet.currentState 仍返回 1（rest），由 Epic 14 改成真实状态
**And** Epic 1 的 wire / DI 容器把 placeholder 替换为 RoomSnapshotBuilder
**And** **单元测试覆盖**（≥4 case）:
- happy: 房间 3 成员 + 2 个在线 → snapshot.members 含 3 个 user，标注 isOnline 字段（按需）
- edge: 房间 0 成员 → snapshot.members = []
- edge: room_members 与 presence 不一致（DB 有 user 但 presence 没标在线）→ 仍返回 user，isOnline=false
- edge: room 不存在 → builder 返回 error
**And** **集成测试覆盖**（dockertest + Redis）: WS 客户端 A 连接到房间 X → 服务端 room.snapshot 含 A 自己

### Story 11.8: 成员加入 / 离开 WS 广播

As a 服务端开发,
I want 用户加入或退出房间时 WS 广播 member.joined / member.left 给该房间在线成员,
So that 客户端可以实时刷新成员列表.

**Acceptance Criteria:**

**Given** Story 11.4 / 11.5 已就绪 + Story 10.5 BroadcastToRoom primitive 可用
**When** 完成本 story
**Then** Story 11.4 加入房间事务**成功提交后**，调用 `BroadcastToRoom(roomID, {type: "member.joined", payload: {userId, nickname}})`
**And** Story 11.5 退出房间事务**成功提交后**，调用 `BroadcastToRoom(roomID, {type: "member.left", payload: {userId}})`
**And** 广播失败不影响事务结果（fire-and-forget，仅 log）
**And** Story 10.4 心跳超时清理钩子也触发 member.left 广播（被动断线场景）
**And** **单元测试覆盖**（≥4 case，mocked BroadcastToRoom）:
- happy: 加入成功 → broadcast 被调用 1 次，msg.type=member.joined
- happy: 退出成功 → broadcast 被调用 1 次，msg.type=member.left
- happy: 心跳超时 → broadcast member.left 触发
- edge: 加入事务 rollback → broadcast 不被调用
**And** **集成测试覆盖**（dockertest + Redis + 真实 WS）:
- A 在房间，建立 WS → B 调 join → A 收到 member.joined { userId: B }
- B 调 leave → A 收到 member.left { userId: B }

### Story 11.9: Layer 2 集成测试 - 房间生命周期全流程

As a 资产事务负责人,
I want 一组深度集成测试覆盖房间创建/加入/退出/关闭事务的失败回滚 / 并发 / 边界,
So that NFR1 (资产事务原子) 和 §8.6 / §8.7 (房间事务) 有自动化保障.

**Acceptance Criteria:**

**Given** Story 11.3-11.8 happy path 已通过
**When** 完成本 story
**Then** 输出 `internal/service/room_service_integration_test.go` 用 dockertest（MySQL + Redis）覆盖:
- **完整生命周期**: A 创建 → B/C/D 依次 join → 4 人满 → 第 5 个 E join 返回 6002 → A leave → B 仍在 → 全部 leave → 房间 closed
- **回滚 1**: 创建房间事务，mock room_members repo 第 2 步抛 error → 验证 rooms 也回滚（DB 表为空）
- **回滚 2**: 加入房间事务，mock users.current_room_id update 失败 → 验证 room_members 也回滚
- **回滚 3**: 退出房间事务，mock users 更新失败 → 验证 room_members 删除也回滚（用户仍在房间）
- **并发 1**: 4 个用户已在房间，5 个用户同时 join → 只有 1 个成功，其他 4 个全部返回 6002
- **并发 2**: 100 个不同用户同时 create + join 100 个不同房间 → 全部成功，DB rooms 100 行
- **边界 1**: 用户 A 在房间 X，A 又调 POST /rooms（创建新房间）→ 6003
- **边界 2**: 用户 A 在房间 X，A 调 POST /rooms/X/join（加入自己已在的房间）→ 6003
- **边界 3**: 房间最后一人 leave → 房间 closed + 第 N 个用户尝试 join 该 closed 房间 → 6005
- **WS 联动**: A + B 在房间，A 建 WS → B leave → A 收到 member.left
**And** 全部场景用 dockertest 真实 MySQL + Redis 跑通
**And** 集成测试在 CI 标 `// +build integration` tag

### Story 11.10: GET /home 扩展 - room.currentRoomId 真实数据（之前节点 2 返回 null）

As an iPhone 用户,
I want 主界面"进入房间"按钮能反映我当前是否已在房间（如已在，按钮文案变"返回房间 #xxx"）,
So that 启动 App 后立即知道自己房间状态，不需要先点按钮才发现.

**Acceptance Criteria:**

**Given** Story 4.8 GET /home 已可用 + Story 11.3 用户加入房间会更新 users.current_room_id
**When** 完成本 story
**Then** 修改 GET /home 实装:
- room 字段从写死 `{currentRoomId: null}` 改为读 `users.current_room_id`
- 如有 → `{currentRoomId: "xxx"}` 字符串形式（按 AR21 ID 字符串约定）
- 如无 → `{currentRoomId: null}`
**And** 不破坏 Story 4.8 既有 schema（仅填充 room 字段真实值）
**And** **单元测试覆盖**（≥3 case，mocked repo，扩展 Story 4.8 既有单测）:
- happy: 用户在房间 → /home response.room.currentRoomId = 该房间 id
- happy: 用户不在任何房间 → response.room.currentRoomId = null
- edge: users.current_room_id 指向已 closed 的房间（理论不该）→ 仍返回该 id（client 调 /rooms/{id} 时会得到 6005 自行处理）
**And** **集成测试覆盖**（dockertest）: 创建 user → join room → curl /home → room.currentRoomId 正确

## Epic 12: iOS - 房间页面 + WebSocket 客户端

节点 4 iOS 端：房间页 + WebSocketClient + 自动重连 + 心跳 + 创建/加入/退出 use case + 主界面入口完善。

### Story 12.1: 房间页面 SwiftUI 骨架

As an iOS 开发,
I want 房间页面的 SwiftUI 骨架（含成员列表占位 + 操作按钮 + 房间号展示）,
So that Story 12.3 / 12.4 / 12.7 可以填充各自的内容.

**Acceptance Criteria:**

**Given** Epic 2 Story 2.3 导航架构已就绪（主界面"进入房间"按钮 + Sheet 导航）
**When** 完成本 story
**Then** `RoomView` 替换 Story 2.3 的 placeholder Sheet 内容，含:
- 顶部: 房间号显示 + 关闭按钮（关闭 Sheet 不等于退出房间）
- 中间: 成员列表区域（占位 4 格，每格含头像 + 昵称 + 猫位）
- 底部: "退出房间"按钮（红色）
- 状态文字: "已连接 / 正在重连 / 已断开"（占位）
**And** 用 `RoomViewModel` 持有 `room: Room?, members: [Member], wsState: WSState`
**And** 无房间时（roomId == nil）显示 placeholder "未加入房间"
**And** **单元测试覆盖**（≥3 case）:
- happy: ViewModel 注入 mock room + 3 成员 → View body 渲染 3 个成员位
- happy: ViewModel members 为空 → 显示空房间 placeholder
- edge: members 数量 > 4（理论不应发生）→ 只渲染前 4 个，log warning
**And** **UI 测试覆盖**（XCUITest）: 主界面点"进入房间"→ Sheet 弹出 → 验证 RoomView 的 accessibility identifier "roomView" 可定位

### Story 12.2: WebSocketClient 封装（基于 URLSessionWebSocketTask）

As an iOS 开发,
I want 一个 WebSocketClient 协议 + 真实实现 + mock,
So that 业务层（RoomViewModel）可以无缝在测试中替换为 mock，且不直接处理 URLSession 细节.

**Acceptance Criteria:**

**Given** Epic 2 iOS 脚手架完成
**When** 完成本 story
**Then** 定义 `WebSocketClient` 协议:
- `connect(url: URL, token: String) async throws`
- `send(_ message: WSMessage) async throws`
- `messages: AsyncStream<WSMessage>` (incoming stream)
- `disconnect()`
**And** 实现 `WebSocketClientImpl`，基于 `URLSessionWebSocketTask`
**And** WSMessage 是 enum，覆盖 V1接口设计 §12 的所有消息类型（含 `roomSnapshot`, `memberJoined`, `memberLeft`, `ping`, `pong`, `error` 等）
**And** 自动 JSON 编解码（用 Codable）
**And** 提供 `WebSocketClientMock`（手动注入消息序列）用于测试
**And** **单元测试覆盖**（≥5 case，mocked URLSession）:
- happy: connect 成功 → 状态 = connected
- happy: send → URLSession.send 被调用 + 消息正确序列化
- happy: 服务端推消息 → AsyncStream 释放对应 WSMessage
- edge: connect 失败（network error）→ 抛 WSError.connectionFailed
- edge: 服务端推未知 type → 解码失败 + log warning + 不破坏 stream

### Story 12.3: 房间快照解析 + 成员列表渲染

As an iPhone 用户,
I want 进入房间后能立即看到房间内当前成员列表,
So that 我知道这个房间里有谁.

**Acceptance Criteria:**

**Given** Story 12.1 RoomView 骨架 + Story 12.2 WebSocketClient + Story 11.7 server 真实 snapshot
**When** RoomViewModel 收到 `room.snapshot` 消息
**Then** 解析 payload 更新 `room` 和 `members`
**And** RoomView 自动 SwiftUI 刷新成员列表（每个成员显示 nickname + 头像 placeholder + 猫 placeholder）
**And** **单元测试覆盖**（≥4 case，mocked WebSocketClient）:
- happy: WS 推 room.snapshot 含 3 成员 → ViewModel.members 长度 = 3
- happy: 同一 room.snapshot 推两次（快照对齐）→ ViewModel.members 仍是 3
- happy: WS 推空房间 snapshot → ViewModel.members = []
- edge: snapshot 解码失败（服务端返回畸形数据）→ ViewModel 不破坏现有 members + log error
**And** **UI 测试覆盖**: mock RoomViewModel 注入 3 成员 → RoomView 渲染 → 验证 3 个成员位 accessibility identifier 都可定位

### Story 12.4: 成员加入 / 离开 WS 消息处理

As an iPhone 用户,
I want 当其他用户进入或离开房间时，我的成员列表实时刷新,
So that 我能感知房间动态而不需要手动刷新.

**Acceptance Criteria:**

**Given** Story 12.3 已能渲染 snapshot 成员
**When** RoomViewModel 收到 `member.joined` / `member.left` 消息
**Then** 处理逻辑:
- `member.joined`: 检查是否已在 `members` 中（防重），不在则 append
- `member.left`: 从 `members` 中删除对应 userId
**And** RoomView 平滑动画（淡入淡出）展示成员变化
**And** 收到广播时**不重新拉 snapshot**（信任增量），但每次 reconnect 后重新拉 snapshot 对齐
**And** **单元测试覆盖**（≥5 case，mocked WebSocketClient）:
- happy: 收到 member.joined → ViewModel.members 多 1 个
- happy: 收到 member.left → ViewModel.members 少 1 个
- edge: 收到 member.joined 但 userId 已存在 → 不重复添加 + log
- edge: 收到 member.left 但 userId 不存在 → 不报错 + log warning
- edge: 连续收到 join + leave 同一 user → members 数量正确

### Story 12.5: 自动重连（指数退避）

As an iPhone 用户,
I want 网络抖动 / WS 断开后 App 自动重连，不需要我手动操作,
So that 我可以放心放在后台 / 弱网时使用.

**Acceptance Criteria:**

**Given** Story 12.2 WebSocketClient + Story 12.3 房间页可显示连接状态
**When** WebSocketClient 检测到连接断开（非用户主动 disconnect）
**Then** 触发 `WSReconnectStrategy`:
- 指数退避: 1s, 2s, 4s, 8s, 16s, 32s, max 60s
- 每次重连前更新 ViewModel.wsState = .reconnecting (RoomView 显示"正在重连…")
- 重连成功 → wsState = .connected + 立即重新拉 snapshot 对齐
- 重连失败超过 5 次 → wsState = .disconnected + 显示 RetryView 让用户手动重连
**And** App 进入后台时主动 disconnect（节省电量），回前台时自动 reconnect
**And** Story 12.4 `member.left` 不在断网期间触发（断网期间收不到 WS 消息，重连后由 snapshot 对齐）
**And** **单元测试覆盖**（≥5 case，mocked WebSocketClient + fake timer）:
- happy: 第一次断 → 1s 后重连，成功 → wsState 流转正确
- happy: 5 次失败 → 第 5 次后停止，wsState=disconnected
- happy: 重连成功 → 自动触发新 snapshot 拉取（mocked WS verify ping after reconnect）
- happy: App 进后台 → 主动 disconnect
- happy: App 回前台 → 重新 connect

### Story 12.6: 心跳维护（ping/pong 定时）

As an iOS 开发,
I want WebSocketClient 定时发送 ping 给服务端,
So that 服务端能感知客户端在线，避免被 server 端 60 秒超时清理.

**Acceptance Criteria:**

**Given** Story 12.2 WebSocketClient 可发消息
**When** 完成本 story
**Then** WebSocketClient 连接成功后启动 heartbeat task:
- 每 30 秒（配置可调）发一次 `ping` 消息
- 等待 `pong` 响应（5 秒超时）
- 5 秒未收到 pong → 视为连接失效，主动 disconnect 触发 Story 12.5 自动重连
**And** 用户主动 disconnect 时停止 heartbeat
**And** **单元测试覆盖**（≥4 case，fake timer + mocked URLSession）:
- happy: 30 秒到 → 发 ping → 收到 pong → 继续下一轮
- edge: 5 秒未收到 pong → 触发 disconnect
- happy: disconnect 后 heartbeat task 停止（不再发 ping）
- happy: 重连后 heartbeat task 重启

### Story 12.7: 创建 / 加入 / 退出 use case + 主界面入口完善

As an iPhone 用户,
I want 我可以从主界面创建房间或加入房间，从房间页面退出房间,
So that 节点 4 业务链路对用户完整闭合.

**Acceptance Criteria:**

**Given** Story 12.1-12.6 完成 + Story 11.3-11.5 server 端房间 CRUD 接口可用
**When** 完成本 story
**Then** 实装三个 UseCase:
- `CreateRoomUseCase`: 调 POST /rooms → 拿到 roomId → 把 roomId 给 RoomViewModel → 触发 WS 连接 → Sheet 弹出 RoomView
- `JoinRoomUseCase`: 接受 roomId 参数 → 调 POST /rooms/{id}/join → 同上
- `LeaveRoomUseCase`: 调 POST /rooms/{id}/leave → 主动 disconnect WS → 关闭 Sheet 回主界面
**And** 主界面"进入房间"按钮的实际行为（之前 Sheet 显示 placeholder）变成两个选项弹层:
- "创建新房间"（直接调 CreateRoomUseCase）
- "输入房间号加入"（弹 input → 调 JoinRoomUseCase）
**And** 主界面在用户已有房间时，按钮文案改为 "返回房间 #xxxx"，点击直接调 JoinRoomUseCase（其实是重新进入已加入的房间）
**And** **单元测试覆盖**（≥6 case，mocked APIClient + WebSocketClient）:
- happy: CreateRoomUseCase 成功 → 自动连 WS → RoomView 出现
- happy: JoinRoomUseCase 成功 → 自动连 WS
- edge: CreateRoomUseCase 返回 6003（用户已在房间）→ ErrorPresenter 弹 alert
- edge: JoinRoomUseCase 返回 6002（房间满）→ alert
- edge: JoinRoomUseCase 返回 6001（房间不存在）→ alert
- happy: LeaveRoomUseCase → WS disconnect + Sheet 关闭
**And** **UI 测试覆盖**（XCUITest）: 模拟器主界面点"进入房间"→ 选"创建新房间"→ Sheet 出现 → 验证房间页有自己一个成员

## Epic 13: 对齐 - 节点 4 房间生命周期 + 成员进出广播联调

把 Epic 10 (Server WS infra) + Epic 11 (Server 房间业务) + Epic 12 (iOS) 真实串联，验证节点 4 §4.4 全部验收标准达成。重点：4 人房间生命周期、成员加入/离开广播、WS 重连、心跳超时清理。

### Story 13.1: 跨端集成测试场景 - 房间 lifecycle + 成员事件 E2E

**Acceptance Criteria:**

**Given** Epic 10-12 全部完成
**When** 完成本 story
**Then** 输出文档 `_bmad-output/implementation-artifacts/e2e/node-4-room-e2e.md`，含:
- 准备步骤: server 启动（含 MySQL + Redis）+ 4 个模拟器 / 同一模拟器 4 个不同 user 账号准备
- 验证场景 1（创建 + 加入）: A 创建房间 → DB rooms 1 行 + room_members 1 行 → A 收到 room.snapshot 含自己
- 验证场景 2（B 加入）: B 用房间号 join → DB room_members 2 行 + A 的 RoomView 收到 member.joined { userId: B } + A.members 长度 = 2
- 验证场景 3（C / D 加入）: 同上，最终 4 人房间满
- 验证场景 4（第 5 人被拒）: E 尝试 join → 返回 6002 + E 看到 alert "房间已满"
- 验证场景 5（A 退出）: A leave → DB room_members 3 行 + B/C/D 收到 member.left { userId: A } + A 的 Sheet 关闭回主界面
- 验证场景 6（全部退出）: B/C/D 依次 leave → DB room_members 0 行 + rooms.status = 2 closed
- 验证场景 7（WS 重连）: A 在房间，断网 5 秒（wifi off）→ 验证 RoomView 显示"正在重连…" → 恢复网络 → 看到"已连接" + 重新拉 snapshot
- 验证场景 8（心跳超时）: A 在房间，强制 kill App（不走 leave 接口）→ 等 70 秒 → server 心跳超时清理 → B 收到 member.left { userId: A } + A 用 mysql 查 users.current_room_id 仍为该 room（因为 user 主观还在房间，只是 WS 断了）
- 验证场景 9（Background 模式）: A 进 App 后台 → WS 主动断开 → 30 秒后回前台 → WS 重新连上 + 收到最新 snapshot
**And** 文档含截图位

### Story 13.2: 节点 4 demo 验收

**Acceptance Criteria:**

**Given** Story 13.1 E2E 文档已就绪
**When** 完成本 story
**Then** 按 §4.4 节点 4 验收标准逐项打勾:
- [ ] 用户可创建房间并自动加入
- [ ] 其他用户可通过房间号加入
- [ ] 房间成员可收到加入/离开广播
- [ ] 用户退出后房间成员列表正确刷新
- [ ] **新增**: WS 自动重连可用
- [ ] **新增**: 心跳超时被动清理可用
**And** 输出 demo 录屏到 `_bmad-output/implementation-artifacts/demo/node-4-demo-<date>/`
**And** 任何未通过项必须登记原因 + 是否后置

### Story 13.3: 文档同步与 tech debt 登记

**Acceptance Criteria:**

**Given** Epic 10-12 完成 + Story 13.2 demo 通过
**When** 完成本 story
**Then** 检查并更新（无变更也要明确标注）:
- [ ] `docs/宠物互动App_V1接口设计.md` §10 + §12.1-12.3: 实装是否与 Story 11.1 / 10.1 锚定的 schema 完全一致
- [ ] `docs/宠物互动App_数据库设计.md` §5.13 / §5.14: rooms / room_members 表结构与文档一致
- [ ] `docs/宠物互动App_时序图与核心业务流程设计.md` §11 / §12 / §13: 房间创建 / 加入 / 退出 / WS 建连时序与实装一致
- [ ] `docs/宠物互动App_Go项目结构与模块职责设计.md` §6.8 / §6.10: Room / Realtime 模块结构对齐
- [ ] `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §6.8 / §9: Room / WebSocket 模块结构对齐
**And** `_bmad-output/implementation-artifacts/tech-debt-log.md` 追加节点 4 tech debt，可能的:
- 心跳超时清理只触发 member.left 广播，但不修改 users.current_room_id（用户被动断线后重连仍能回到原房间）—— 这是设计选择，不是 bug，但要记录
- WS gateway 单实例，多实例部署时 BroadcastToRoom 不能跨实例（节点 11+ 接 Redis pub/sub）
- iOS 后台 disconnect 在某些 iOS 版本可能延迟，建议监控

## Epic 14: Server - 房间内宠物状态广播

节点 5 Server 端：把节点 4 房间快照里 pet.currentState 的"固定值 1"替换为真实状态，新增 `/pets/current/state-sync` 接口，并通过 WS `pet.state.changed` 广播给同房间成员。

### Story 14.1: 接口契约最终化（POST /pets/current/state-sync + WS pet.state.changed）

As a 跨端契约负责人,
I want 把 state-sync 接口与 pet.state.changed WS 消息 schema 锚定到 V1接口设计文档,
So that iOS Epic 15 可以基于稳定契约并行开工.

**Acceptance Criteria:**

**Given** docs/宠物互动App_V1接口设计.md §5.2 / §12.3 已有初稿
**When** 完成本 story
**Then** §5.2 (POST /pets/current/state-sync) schema 完全锚定:
- request: `{state: int (1=rest, 2=walk, 3=run)}`
- response data: `{state: int}`
- 错误码: 1002 参数错误
**And** §12.3 业务 WS 消息 schema 锚定:
- `pet.state.changed`: payload `{userId, petId, currentState: int}`
**And** 标注"契约自此 Epic 起冻结"

### Story 14.2: POST /pets/current/state-sync 接口 + pets.current_state 更新

As an iPhone 用户,
I want App 可以把我猫的最新状态上报给服务端,
So that 我在房间里时其他成员能看到我猫的真实状态.

**Acceptance Criteria:**

**Given** Story 4.6 默认 pet 已在登录初始化时创建（pets.current_state 默认 1）+ Story 14.1 契约已定
**When** 调用 `POST /pets/current/state-sync` 带 state ∈ {1,2,3}
**Then** service 找到当前用户的默认 pet（pets WHERE user_id=? AND is_default=1）→ 更新 current_state = state
**And** 接口要求 auth
**And** state 不在 [1,2,3] → 1002 参数错误
**And** 用户没有 pet（理论不该发生，但兜底）→ 1003
**And** **状态写库不要求高频**：业务上 iOS 端只在状态切换瞬间上报，不每秒上报；server 不限频但应在日志中注意频率异常
**And** **单元测试覆盖**（≥4 case，mocked pet repo）:
- happy: state=2 → pet.current_state 更新为 2，返回 {state: 2}
- edge: state=4（非法）→ 1002，DB 不变
- edge: 用户无默认 pet → 1003
- happy: 同一 state 重复上报 → 接受但不报错（幂等）
**And** **集成测试覆盖**（dockertest）:
- 创建 user + 默认 pet（current_state=1）→ POST /pets/current/state-sync {state: 3} → DB pets.current_state = 3
- 再 POST {state: 1} → DB pets.current_state = 1

### Story 14.3: 修改 RoomSnapshotBuilder - snapshot 含真实 pet.currentState

As a 服务端开发,
I want RoomSnapshotBuilder 返回的 members[].pet.currentState 来自 DB pets.current_state，而非固定值 1,
So that 客户端进入房间立即看到所有成员猫的真实当前状态.

**Acceptance Criteria:**

**Given** Story 11.7 RoomSnapshotBuilder 已实装但 pet.currentState 写死 1
**When** 完成本 story
**Then** 修改 `RoomSnapshotBuilder.BuildSnapshot`:
- 在查 room_members 关联 users + pets 时，把 pets.current_state 真实读出
- snapshot.members[].pet.currentState = 该值
**And** 不破坏其他字段（pet.equips 仍 []，留给后续 Epic）
**And** **单元测试覆盖**（≥3 case）:
- happy: 房间 3 成员，各自 pet.current_state 分别为 1/2/3 → snapshot 中 currentState 字段正确对应
- happy: 房间成员的 pet 没有（理论不该发生）→ snapshot.members[].pet 为 null（或 default 1，二选一并测试）
- edge: 大量并发查 snapshot → 不竞态（DB 查询是只读）
**And** **集成测试覆盖**（dockertest）: 创建房间 + 3 成员 + 各自 set pet.current_state=2/3/1 → WS 客户端连入 → snapshot 验证 3 个 members 的 currentState 都正确

### Story 14.4: pet.state.changed WS 广播

As a 服务端开发,
I want POST /pets/current/state-sync 成功后，如用户当前在某个房间，则向该房间广播 pet.state.changed,
So that 房间内其他成员实时看到该用户猫的状态变化（不必等下次 snapshot 拉取）.

**Acceptance Criteria:**

**Given** Story 14.2 state-sync 接口已就绪 + Story 10.5 BroadcastToRoom 可用 + Story 14.1 WS 契约已定
**When** state-sync 成功更新 pets.current_state
**Then** service 检查 `users.current_room_id`:
- 非 null → 调用 `BroadcastToRoom(currentRoomId, {type: "pet.state.changed", payload: {userId, petId, currentState}})`
- null → 不广播（用户不在任何房间）
**And** 广播失败不影响 state-sync 接口结果（fire-and-forget）
**And** 同一秒多次 state-sync（即便业务上不该发生）→ 每次都广播，不去重（让 iOS 决定是否过滤）
**And** **单元测试覆盖**（≥4 case，mocked BroadcastToRoom）:
- happy: 用户在房间 → state-sync 成功 → broadcast 调用 1 次，msg.type=pet.state.changed + payload 字段正确
- happy: 用户不在房间 → broadcast 不被调用
- edge: state-sync 失败 → broadcast 不被调用
- edge: BroadcastToRoom 失败（网络 error）→ state-sync 接口仍返回成功
**And** **集成测试覆盖**（dockertest + Redis + 真实 WS）:
- A + B 都在房间 X，A 建立 WS → A 调 /pets/current/state-sync {state: 2} → A 收到自己的 pet.state.changed 消息（含自己的 userId）
- 注：广播给房间所有人含发起者自己，客户端逻辑统一

## Epic 15: iOS - 房间内成员宠物状态展示 + 自己状态上报

节点 5 iOS 端：房间页内多成员猫 sprite 渲染 + pet.state.changed 实时处理 + 自己状态变化上报（节流 + 仅在房间内）+ 重连后状态对齐。

### Story 15.1: 房间页内多成员猫位渲染 + snapshot pet.currentState 解析

As an iPhone 用户,
I want 进入房间后能立即看到房间内每个成员的猫和当前状态,
So that 我能直观感受到房间里大家在干什么.

**Acceptance Criteria:**

**Given** Story 12.3 房间页已能渲染成员列表 + Story 14.3 server snapshot 含真实 pet.currentState
**When** RoomViewModel 收到 room.snapshot
**Then** 解析 `members[].pet.currentState`（int 1/2/3）映射成 iOS 端 `MotionState` enum
**And** RoomView 每个成员位用 `PetSpriteView(state: petState)`（复用 Story 8.4 组件）渲染对应动画
**And** 自己的成员位也用相同方式渲染（不区分自己 / 别人）
**And** **单元测试覆盖**（≥4 case，mocked WebSocketClient）:
- happy: snapshot 含 3 成员 + currentState 分别为 1/2/3 → ViewModel.members[].petState 正确映射 .rest/.walk/.run
- edge: snapshot 含未知 currentState 值（如 99）→ 默认按 .rest 处理 + log warning
- happy: snapshot 缺 pet 字段（兜底）→ 默认 .rest
- happy: 同一房间多次刷新 snapshot → ViewModel.members 状态正确同步更新
**And** **UI 测试覆盖**（XCUITest）: mock RoomViewModel 注入 3 成员各自不同 state → RoomView 验证 3 个 PetSpriteView 的 accessibility identifier 各为 "petSprite_rest" / "petSprite_walk" / "petSprite_run"

### Story 15.2: pet.state.changed WS 消息处理

As an iPhone 用户,
I want 当房间里某个成员的猫状态变化时，我能立即看到（不必等下次刷新）,
So that 互动有实时反馈.

**Acceptance Criteria:**

**Given** Story 15.1 已能渲染 snapshot 状态 + Story 14.4 server WS 广播可用
**When** RoomViewModel 收到 `pet.state.changed` 消息
**Then** 找到对应 userId 的 member → 更新 member.petState
**And** 不在成员列表中的 userId（理论不该发生，可能是延迟到达）→ 忽略 + log warning
**And** 收到自己的 pet.state.changed → 也更新（保持本地 / 远端一致，避免本地猜测的 state 与 server 不一致）
**And** **单元测试覆盖**（≥4 case，mocked WebSocketClient）:
- happy: 收到 pet.state.changed { userId: B, currentState: 3 } → ViewModel.members 中 B.petState = .run
- happy: 收到自己的 pet.state.changed → 自己的 petState 同步更新
- edge: 收到 userId 不在房间的成员 → 忽略 + log warning，不报错
- edge: 同时收到多个 pet.state.changed → 各自正确路由

### Story 15.3: 状态切换动画过渡

As an iPhone 用户,
I want 猫的状态切换是平滑动画过渡，不是硬切换,
So that 视觉体验更流畅.

**Acceptance Criteria:**

**Given** Story 15.1 / 15.2 已能驱动 PetSpriteView 状态变化
**When** PetSpriteView 的 state prop 改变（任何路径触发：snapshot / WS / 自己上报）
**Then** sprite 动画用 `.transition(.opacity.combined(with: .scale))` 或类似 SwiftUI animation 平滑过渡（200-300ms）
**And** 连续多次状态切换（如 rest → walk → rest 在 1 秒内）→ 后一个动画覆盖前一个，不堆叠队列
**And** **单元测试覆盖**（≥3 case，UI snapshot 测试）:
- happy: state .rest → .walk → 验证过渡 modifier 已应用
- happy: 快速连续切换 → 不出现视觉残影
- edge: 同一 state 重复 set（state 没变）→ 不触发动画
**And** **UI 测试覆盖**（XCUITest 配合录屏）: 模拟器手动切换 → 录屏检查动画不闪烁

### Story 15.4: 自己状态变化时上报 state-sync（节流 + 房间内才上报）

As an iPhone 用户,
I want App 自动把我猫的状态变化上报给服务端，但不上报得太频繁,
So that 房间内其他成员能看到我猫的状态，且不浪费流量.

**Acceptance Criteria:**

**Given** Story 8.3 MotionStateMapper 已能输出 MotionState + Story 14.2 server state-sync 接口可用
**When** HomeViewModel 的 petState 变化
**Then** 触发 `SyncPetStateUseCase`:
- 检查当前是否在房间（RoomViewModel 状态）
  - 不在房间 → 不上报（节省流量，因为没有观察者）
  - 在房间 → 调 `POST /pets/current/state-sync` 带新 state
**And** 节流：同一 state 在 5 秒内不重复上报（防 MotionStateMapper 的抖动）
**And** 上报失败不重试 / 不阻塞 UI（背景 fire-and-forget）
**And** **单元测试覆盖**（≥5 case，mocked APIClient + RoomViewModel + fake timer）:
- happy: 在房间 + state 变化 .rest → .walk → 调 state-sync 1 次
- happy: 不在房间 → state 变化也不调
- edge: 5 秒内重复 set 同一 state → 只调 1 次
- edge: 5 秒后重新 set 同一 state → 又调 1 次
- edge: API 失败 → 不抛错，下次状态变化照常上报

### Story 15.5: 跨房间状态恢复（重连后由 snapshot 对齐）

As an iPhone 用户,
I want WS 重连后，所有成员的猫状态自动对齐到最新（不依赖断线期间的 WS 消息）,
So that 我不会在断线后看到错的猫状态.

**Acceptance Criteria:**

**Given** Story 12.5 重连成功后会重新触发 snapshot 拉取 + Story 15.1 snapshot 解析能更新所有成员 petState
**When** WS 断开后重连成功
**Then** RoomViewModel 接收新 snapshot → 全员 petState 重置为 server 最新值
**And** 断线期间收到的本地 MotionState 变化（如自己走了几步）→ 重连后自动通过 Story 15.4 上报
**And** **单元测试覆盖**（≥3 case，mocked WebSocketClient）:
- happy: WS 断 → 重连 → 收到新 snapshot → 所有成员 petState 重置为 snapshot 值
- happy: 断线期间自己 petState 变了 → 重连后自动上报当前 state
- edge: 断线期间收到旧的 pet.state.changed（晚到的）→ 被新 snapshot 覆盖（snapshot 始终为权威）

## Epic 16: 对齐 - 节点 5 跨端猫动作同步联调

把 Epic 14 (Server 状态广播) + Epic 15 (iOS 状态展示与上报) 真实串联，验证节点 5 §4.5 全部验收标准达成。重点：跨用户实时状态同步 + 重连后对齐。

### Story 16.1: 跨端集成测试场景 - 房间内猫动作同步 E2E

**Acceptance Criteria:**

**Given** Epic 14 + Epic 15 全部完成
**When** 完成本 story
**Then** 输出文档 `_bmad-output/implementation-artifacts/e2e/node-5-pet-state-sync-e2e.md`，含:
- 准备步骤: server 启动 + 2 个 user 账号（A、B）+ A、B 各自模拟器（或同模拟器切换账号）
- 验证场景 1（双向同步）: A、B 都在房间 X，建立 WS → A 切到 walk → B 看到 A 的猫切到 walk + DB pets.current_state for A = 2
- 验证场景 2（多次切换）: A 切到 run → B 看到 → A 切回 rest → B 看到。验证连续切换无视觉残影
- 验证场景 3（节流）: A 短时间内（5 秒内）连续切到 walk 多次 → server 只收到 1 次 state-sync 请求（用 server log 验证）
- 验证场景 4（不在房间不上报）: A 退出房间 → MotionState 切换 → 验证 server 没收到 state-sync
- 验证场景 5（snapshot 对齐）: A 在房间，状态 walk，断网 → 服务端 mysql 手动 UPDATE pets.current_state=3 给 A → A 重连 → snapshot 把 A 的状态强制对齐回 walk
- 验证场景 6（自己也广播）: A 切 walk → A 自己也收到 pet.state.changed { userId: A } → 验证客户端逻辑能正确处理"广播给自己"
**And** 文档含截图位

### Story 16.2: 节点 5 demo 验收

**Acceptance Criteria:**

**Given** Story 16.1 E2E 文档已就绪
**When** 完成本 story
**Then** 按 §4.5 节点 5 验收标准逐项打勾:
- [ ] 房间内至少两名用户在线时，可看到对方的猫状态变化
- [ ] 状态变化延迟可接受（< 1 秒）
- [ ] 断线重连后可恢复最近状态
- [ ] **新增**: 节流机制有效（5 秒内不重复上报）
**And** 输出 demo 录屏到 `_bmad-output/implementation-artifacts/demo/node-5-demo-<date>/`
**And** 任何未通过项必须登记原因 + 是否后置

### Story 16.3: 文档同步与 tech debt 登记

**Acceptance Criteria:**

**Given** Epic 14-15 完成 + Story 16.2 demo 通过
**When** 完成本 story
**Then** 检查并更新（无变更也要明确标注）:
- [ ] `docs/宠物互动App_V1接口设计.md` §5.2 + §12.3 (`pet.state.changed`): 实装是否与 Story 14.1 锚定的 schema 完全一致
- [ ] `docs/宠物互动App_数据库设计.md` §5.3 (pets): pets.current_state 字段使用方式与文档一致
- [ ] `docs/宠物互动App_时序图与核心业务流程设计.md`: 节点 5 没有专门时序图，可考虑补一张"房间内 pet 状态广播"
- [ ] `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §6.3 (Pet 模块): 状态同步与广播接收的职责划分
**And** `_bmad-output/implementation-artifacts/tech-debt-log.md` 追加节点 5 tech debt，可能的:
- pets.current_state 高频写库（每次状态变化都 UPDATE）—— 节流后频率可控，但仍可考虑用 Redis 缓存 + 后台批量回写（节点 11+ 优化）
- pet.state.changed 也广播给发起者自己，多余但简化客户端逻辑—— 是否在 server 端去重是后续优化点
- iOS 端 Story 15.4 的 5 秒节流是 hard-code，未抽到配置

## Epic 17: Server - 表情广播链路 + emoji_configs 预置

节点 6 Server 端：emoji_configs 表 + seed + GET /emojis 接口 + WS emoji.send / emoji.received 广播链路。

### Story 17.1: 接口契约最终化（GET /emojis + WS emoji.send / emoji.received）

As a 跨端契约负责人,
I want 把表情配置接口与 WS 表情消息 schema 锚定到 V1接口设计文档,
So that iOS Epic 18 可以基于稳定契约并行开工.

**Acceptance Criteria:**

**Given** docs/宠物互动App_V1接口设计.md §11 / §12.2 / §12.3 已有初稿
**When** 完成本 story
**Then** §11.1 (GET /emojis) schema 完全锚定:
- response data: `{items: [{code: string, name: string, assetUrl: string, sortOrder: int}]}`
**And** §12.2 客户端→服务端 schema 锚定:
- `emoji.send`: payload `{emojiCode: string}`
**And** §12.3 服务端→客户端 schema 锚定:
- `emoji.received`: payload `{userId, emojiCode}`
- `error`: 当 emojiCode 无效时返回 `{code: 7001, message: "emoji not found"}`
**And** 标注"契约自此 Epic 起冻结"

### Story 17.2: emoji_configs migration

As a 服务端开发,
I want emoji_configs 表的 migration,
So that 表情配置可以持久化.

**Acceptance Criteria:**

**Given** Epic 4 migrations 框架已就绪
**When** 完成本 story
**Then** `migrations/0009_init_emoji_configs.sql` 按 数据库设计.md §5.15 创建表:
- 字段全集（含 `id`, `code`, `name`, `asset_url`, `sort_order`, `is_enabled`, `created_at`, `updated_at`）
- `UNIQUE KEY uk_code` + `KEY idx_enabled_sort`
**And** 含 down.sql
**And** **单元测试覆盖**（≥3 case）:
- happy: migrate up 后表存在 + 字段类型 + 索引都符合 §5.15
- happy: migrate down 后表删除
- edge: 重复 migrate up → 幂等
**And** **集成测试覆盖**（dockertest）: migrate up → SHOW CREATE TABLE 对比 schema → migrate down

### Story 17.3: emoji_configs seed（≥4 个表情，含可访问 asset_url）

As a 服务端开发,
I want emoji_configs 表预置最小可用集合,
So that 节点 6 demo 时能至少展示 4 种不同表情，不出现"只有一个表情"的尴尬.

**Acceptance Criteria:**

**Given** Story 17.2 表已就绪
**When** 完成本 story
**Then** seed 至少 4 个表情，覆盖典型情绪（按 AR19）:
- `wave`（挥手）
- `love`（爱心）
- `laugh`（大笑）
- `cry`（哭）
**And** 每个表情的 `asset_url` 必须可访问（按 AR19 / AR18 URL 约束），MVP 阶段可用 placeholder URL（如 `https://placehold.co/64x64?text=Wave`）
**And** seed 通过 migration 文件 `migrations/0010_seed_emoji_configs.sql` 写入（INSERT IGNORE 防重复）
**And** **单元测试覆盖**（≥2 case）:
- happy: migrate up 后 emoji_configs 至少 4 行 + asset_url 都非空
- happy: 重复 migrate up → 不重复插入（INSERT IGNORE）
**And** **集成测试覆盖**（dockertest）: migrate up → SELECT * FROM emoji_configs → 验证 4 个表情存在 + URL 字段格式合法

### Story 17.4: GET /emojis 接口

As an iPhone 用户,
I want 我可以查询所有可用表情列表,
So that 表情面板能加载真实数据.

**Acceptance Criteria:**

**Given** Story 17.3 emoji_configs 已 seed
**When** 调用 `GET /emojis`
**Then** 返回 `{items: [...]}`，仅含 `is_enabled=1` 的表情，按 `sort_order` 排序
**And** 接口要求 auth
**And** 表情列表很短（4-20 个），不分页
**And** **单元测试覆盖**（≥4 case，mocked emoji repo）:
- happy: DB 4 个 enabled 表情 → response items 长度 = 4，按 sort_order 排序
- happy: DB 含 1 个 disabled → 不返回
- edge: DB 0 个 enabled → response items = []，不报错
- edge: 服务端 DB 错误 → 1009 服务繁忙
**And** **集成测试覆盖**（dockertest）: seed 4 个表情 → curl GET /emojis → response.items 长度 = 4 + 字段值正确

### Story 17.5: WS emoji.send 处理 + emoji.received 广播

As an iPhone 用户,
I want 我在房间内发送表情后，房间所有成员都能看到,
So that 互动有视觉反馈.

**Acceptance Criteria:**

**Given** Story 17.4 GET /emojis 可用 + Story 10.5 BroadcastToRoom 可用 + Story 14.1 WS 协议契约
**When** 客户端发 WS 消息 `emoji.send {emojiCode: string}`
**Then** WS gateway dispatcher 路由到 EmojiHandler:
- 校验当前用户在某个房间中（users.current_room_id 非 null）
  - 不在房间 → 回 error 6004 用户不在房间中
- 校验 emojiCode 在 emoji_configs 中存在且 enabled
  - 不存在 → 回 error 7001 emoji not found
- 调用 BroadcastToRoom(currentRoomId, {type: "emoji.received", payload: {userId, emojiCode}})
**And** 表情**不入库**（按 §14.3，MVP 不强制落库表情事件日志）
**And** 广播给房间所有人含发起者自己（一致性，与 pet.state.changed 同样规则）
**And** 限频建议: 同一用户每秒最多 5 个表情（防止刷屏，可在 Story 4.5 rate_limit 基础上扩展）—— **MVP 可不做，但 tech debt 登记**
**And** **单元测试覆盖**（≥5 case）:
- happy: 用户在房间 + emojiCode 合法 → broadcast 调用 1 次，msg.type=emoji.received + payload 字段正确
- edge: 用户不在房间 → 回 error 6004，不广播
- edge: emojiCode 不存在 → 回 error 7001，不广播
- edge: emojiCode 存在但 disabled → 回 error 7001
- edge: WS 消息缺 emojiCode → 回 error 1002 参数错误
**And** **集成测试覆盖**（dockertest + Redis + 真实 WS）:
- A + B 都在房间 X，建 WS → A 发 `emoji.send {emojiCode: "wave"}` → A、B 都收到 emoji.received {userId: A, emojiCode: "wave"}

## Epic 18: iOS - 表情面板交互 + 广播接收动效

节点 6 iOS 端：表情面板 + 点自己猫触发面板 + 选中后**本地立即动效**（optimistic）+ 接收别人 emoji.received 触发对应猫的动效（去重自己 userId）。

### Story 18.1: 表情面板 SwiftUI（GET /emojis + 网格选择 UI）

As an iOS 开发,
I want 一个 SwiftUI 表情面板组件，含从 server 加载的表情列表 + 网格选择交互,
So that Story 18.2 可以挂载该面板到房间页.

**Acceptance Criteria:**

**Given** Story 17.4 server GET /emojis 可用
**When** 完成本 story
**Then** 实装 `EmojiPanelView` SwiftUI 组件:
- 启动时调 `LoadEmojisUseCase` → GET /emojis → 拿到表情列表
- 用 LazyVGrid 渲染（4 列），每个 cell: AsyncImage 加载 assetUrl + 表情名 label
- 选中后通过 closure 回调（`onSelect: (emojiCode) -> Void`）通知外部
- 加载中显示 ProgressView，加载失败显示 RetryView（复用 Epic 2 ErrorPresenter）
**And** 表情列表**首次加载后缓存**（同一 App 生命周期内不再重新拉取）
**And** **单元测试覆盖**（≥4 case，mocked APIClient）:
- happy: API 返回 4 个表情 → 网格渲染 4 个 cell
- happy: 选中某个表情 → onSelect 回调被触发，emojiCode 正确
- edge: API 失败 → 显示 RetryView
- happy: 表情列表已缓存 → 二次显示面板不再发起 API 调用
**And** **UI 测试覆盖**（XCUITest，mock server）: 启动 App → 进房间 → 触发表情面板 → 验证 4 个表情 cell 可见 + 点其中一个验证回调

### Story 18.2: 点击自己猫触发表情面板（房间页内交互）

As an iPhone 用户,
I want 在房间页面点击自己的猫弹出表情面板,
So that 我能选择一个表情发出去.

**Acceptance Criteria:**

**Given** Story 12.3 房间页已能渲染成员列表（含自己） + Story 18.1 EmojiPanelView 可用
**When** 完成本 story
**Then** RoomView 中"自己的成员位"PetSpriteView 添加 `.onTapGesture` 触发 EmojiPanelView 弹出（用 SwiftUI sheet 或自定义 overlay）
**And** 别人的猫位**不可点击**触发表情面板（防误操作）
**And** 表情面板可关闭（点空白处 / 选中表情后自动关闭）
**And** **单元测试覆盖**（≥3 case，mocked RoomViewModel）:
- happy: 点击自己 PetSpriteView → ViewModel.isEmojiPanelPresented = true
- edge: 点击别人 PetSpriteView → ViewModel.isEmojiPanelPresented 仍为 false
- happy: 选中表情后 → ViewModel.isEmojiPanelPresented = false
**And** **UI 测试覆盖**（XCUITest）: 进房间 → 点自己猫 → 验证 EmojiPanelView 出现 → 点空白 → 验证消失

### Story 18.3: 选中表情 → 本地立即动效 + WS 发送 emoji.send（并行）

As an iPhone 用户,
I want 我点完表情后立即看到自己猫上方的飞出动效，不等服务端回复,
So that 互动有零延迟反馈.

**Acceptance Criteria:**

**Given** Story 18.1 / 18.2 已就绪 + Story 12.2 WebSocketClient 可发消息 + Story 18.4 即将提供动效组件
**When** 用户选中某个表情
**Then** 触发 `SendEmojiUseCase`，**并行**执行两件事:
- **A. 本地立即动效**：在自己猫位上方触发飞出动效（直接调用 Story 18.4 的 `activeEmojis` 队列 append 当前 emojiCode + 自己的 userId）
- **B. WS 发送**：WebSocketClient.send(`emoji.send {emojiCode}`)（fire-and-forget，不阻塞动效）
**And** 选中后立即关闭 EmojiPanelView（不等 WS ack）
**And** WS 发送失败时:
- 本地动效**仍正常播完**（不回滚）
- ErrorPresenter 弹一个温和 toast: "网络不佳，对方可能看不到"（不阻塞，不影响后续操作）
**And** **不依赖**服务端 emoji.received 触发自己的动效（自己的动效在本步触发；server 的 emoji.received 由 Story 18.4 处理，且会跳过自己 userId 的去重）
**And** **单元测试覆盖**（≥4 case，mocked WebSocketClient + RoomViewModel）:
- happy: 选中 wave → activeEmojis 立即多 1 项（自己 userId + wave）+ WebSocketClient.send 调用 1 次
- happy: 选中后 EmojiPanelView 关闭
- edge: WebSocketClient.send 抛 .notConnected → activeEmojis 仍添加（动效照播）+ toast 触发
- edge: 同一表情快速连点 3 次 → activeEmojis 添加 3 项 + WS 发送 3 次（如有 server 端限频，server 自己处理）

### Story 18.4: 接收 emoji.received → 在对应成员猫上方播放飞出动效（去重自己）

As an iPhone 用户,
I want 当其他成员发送表情时，我能看到表情图从该成员猫上方飞出的动效,
So that 互动有视觉反馈.

**Acceptance Criteria:**

**Given** Story 17.5 server 端 emoji.received 广播可用 + Story 18.1 表情列表已缓存（含 assetUrl）+ Story 18.3 自己的本地动效已可触发
**When** RoomViewModel 收到 `emoji.received {userId, emojiCode}`
**Then** 执行去重判定:
- **如果 userId == 当前用户自己** → **跳过**（本地动效已在 Story 18.3 播过，不重复触发）
- 否则 → 走动效流程
**And** 动效流程（也用于 Story 18.3 的本地触发路径）:
- 从缓存表情列表中查 emojiCode → 拿到 assetUrl
- 在该 userId 对应的猫位上方位置生成一个浮动 SwiftUI View（AsyncImage 加载 assetUrl，初始不透明 + 中心位置）
- 动画: 1.5 秒内向上飘移 100px + 透明度 1.0 → 0.0 + 缩放 1.0 → 1.5
- 动画结束后从视图层级移除
**And** 同时多个表情飞出（自己 + 别人 + 不同 emojiCode）→ 各自独立动效，不互相干扰
**And** 收到 userId 不在房间的成员 → 忽略 + log warning
**And** 收到 emojiCode 在缓存中找不到（理论不该）→ 显示 fallback 占位（默认问号）
**And** **单元测试覆盖**（≥5 case，mocked WebSocketClient + Animation 驱动）:
- happy: 收到 emoji.received {userId: B, emojiCode: "wave"}（B 是别人）→ ViewModel.activeEmojis 队列中多 1 项
- happy: 收到 emoji.received {userId: 自己, emojiCode: "wave"}（自己 userId）→ 跳过，activeEmojis 不变
- happy: 1.5 秒后该项从 activeEmojis 移除
- edge: 同时收到 5 个 emoji（不同 user）→ activeEmojis 队列 5 项，各自独立
- edge: emojiCode 在缓存找不到 → 显示问号 fallback，不报错
**And** **UI 测试覆盖**（XCUITest）:
- 自己发表情 → 0 延迟看到本地动效 → 等 200ms（模拟 server 来回）→ 验证不出现"第二次相同动效"（去重生效）
- 别人发表情 → 看到对应 user 猫上方的动效

## Epic 19: 对齐 - 节点 6 房间内表情链路联调

把 Epic 17 (Server 表情广播) + Epic 18 (iOS 表情交互 + 本地动效) 真实串联，验证节点 6 §4.6 全部验收标准达成。重点：表情面板加载 + 0 延迟本地动效 + 跨成员广播 + 去重自己 + 弱网降级。

### Story 19.1: 跨端集成测试场景 - 表情发送 + 接收动效 E2E

**Acceptance Criteria:**

**Given** Epic 17 + Epic 18 全部完成
**When** 完成本 story
**Then** 输出文档 `_bmad-output/implementation-artifacts/e2e/node-6-emoji-e2e.md`，含:
- 准备步骤: server 启动 + 2 个 user 账号（A、B）+ 表情 seed 已就绪
- 验证场景 1（面板加载）: A 进房间 → 点自己猫 → 表情面板出现 → 验证 4 个表情图标都加载成功（assetUrl 可访问）
- 验证场景 2（本地立即动效）: A 选 wave → 立即（< 50ms）看到自己猫上方有 wave 动效飞出 → 验证不等 server roundtrip
- 验证场景 3（跨成员广播）: A、B 都在房间 → A 发 wave → B 看到 A 猫上方有 wave 动效 → A 不出现"第二次"动效（去重生效）
- 验证场景 4（多种表情）: A 依次发 wave / love / laugh → B 依次看到 3 种动效，各自独立
- 验证场景 5（连续快发）: A 连点 5 次 wave → A 自己看到 5 次本地动效堆叠飞出 → B 看到 5 次（如 server 端有限频则按限频后数量）
- 验证场景 6（弱网降级）: A 关 wifi → A 选 wave → 自己仍看到本地动效 + 出现 toast "网络不佳，对方可能看不到" → B 不看到
- 验证场景 7（不在房间不能发）: A 退出房间 → A 没有"点猫弹面板"的 UI（不在房间页）→ 不可能误发
- 验证场景 8（误点别人猫）: A、B 都在房间，A 点 B 的猫位 → 不弹表情面板（防误操作）
**And** 文档含截图位

### Story 19.2: 节点 6 demo 验收

**Acceptance Criteria:**

**Given** Story 19.1 E2E 文档已就绪
**When** 完成本 story
**Then** 按 §4.6 节点 6 验收标准逐项打勾:
- [ ] 点击自己的猫能弹出表情面板
- [ ] 发送后服务端广播成功
- [ ] 其他成员可看到对应表情提示
- [ ] **新增**: 本地 optimistic 动效（0 延迟）
- [ ] **新增**: 弱网时自己仍看到动效 + toast 降级
**And** 输出 demo 录屏到 `_bmad-output/implementation-artifacts/demo/node-6-demo-<date>/`
**And** 任何未通过项必须登记原因 + 是否后置

### Story 19.3: 文档同步与 tech debt 登记

**Acceptance Criteria:**

**Given** Epic 17-18 完成 + Story 19.2 demo 通过
**When** 完成本 story
**Then** 检查并更新（无变更也要明确标注）:
- [ ] `docs/宠物互动App_V1接口设计.md` §11 + §12.2 / §12.3: 实装是否与 Story 17.1 锚定的 schema 完全一致
- [ ] `docs/宠物互动App_数据库设计.md` §5.15 (emoji_configs): 表结构与文档一致 + seed 数量符合 AR19
- [ ] `docs/宠物互动App_时序图与核心业务流程设计.md` §14: 表情广播时序与实装一致
- [ ] `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §6.9 (Emoji 模块): 本地 optimistic + 去重 自己 userId 的设计
**And** `_bmad-output/implementation-artifacts/tech-debt-log.md` 追加节点 6 tech debt，可能的:
- Story 17.5 同一用户每秒最多 5 个表情的限频建议未实装（防刷屏），需要后续补
- 断网期间发的表情不补发，UX 可能让用户误以为"对方看到了"（toast 算补救但不强制）
- 表情资产用 placeholder URL，真美术资产待补
- 表情数量 hard-code 4 个（wave / love / laugh / cry），后续可加配置接口让运营调整

## Epic 20: Server - 宝箱状态机 + 开箱事务 + 奖励加权抽取 + cosmetic_items 预置 + dev 端点

节点 7 最重的 Server Epic：宝箱状态判定、开箱事务（扣步数 + 抽奖 + 写日志 + 刷下一轮）、cosmetic_items seed（含 AR18 数量约束 + URL 占位）、dev 端点（force-unlock + grant-cosmetic-batch）、Layer 2 集成测试 Story 20.9。**节点 7 不入仓**（user_cosmetic_items 实例创建在 Story 23.5 节点 8 才补）。

### Story 20.1: 接口契约最终化（GET /chest/current + POST /chest/open）

As a 跨端契约负责人,
I want 把宝箱接口（含奖励 reward 结构）schema 锚定到 V1接口设计文档,
So that iOS Epic 21 可以基于稳定契约并行开工.

**Acceptance Criteria:**

**Given** docs/宠物互动App_V1接口设计.md §7.1 / §7.2 已有初稿
**When** 完成本 story
**Then** §7.1 (GET /chest/current) schema 完全锚定:
- response data: `{id, status, unlockAt: ISO8601, openCostSteps, remainingSeconds}`
- status 枚举: 1=counting, 2=unlockable
**And** §7.2 (POST /chest/open) schema 完全锚定:
- request: `{idempotencyKey: string}`
- response data: `{reward: {userCosmeticItemId, cosmeticItemId, name, slot, rarity, assetUrl, iconUrl}, stepAccount: {totalSteps, availableSteps, consumedSteps}, nextChest: {id, status, unlockAt, openCostSteps, remainingSeconds}}`
- 错误码: 4001 当前宝箱不存在 / 4002 宝箱尚未解锁 / 3002 可用步数不足 / 1008 幂等冲突
**And** 标注"契约自此 Epic 起冻结"

### Story 20.2: cosmetic_items migration

As a 服务端开发,
I want cosmetic_items 表的 migration,
So that 装扮配置可以持久化（开箱奖励 + 后续合成产出 + 仓库展示都依赖此表）.

**Acceptance Criteria:**

**Given** Epic 4 migrations 框架已就绪
**When** 完成本 story
**Then** `migrations/0011_init_cosmetic_items.sql` 按 数据库设计.md §5.8 创建表:
- 字段全集（含 `id`, `code`, `name`, `slot`, `rarity`, `asset_url`, `icon_url`, `drop_weight`, `is_enabled`, `created_at`, `updated_at`）
- `UNIQUE KEY uk_code` + `KEY idx_slot_rarity` + `KEY idx_enabled_weight`
**And** **不含 `render_config` 字段**（节点 10 才加，由 Epic 29 添加新 migration）
**And** 含 down.sql
**And** **单元测试覆盖**（≥3 case）:
- happy: migrate up 后表存在 + 字段类型 + 索引都符合 §5.8
- happy: migrate down 后表删除
- edge: 重复 migrate up → 幂等
**And** **集成测试覆盖**（dockertest）: migrate up → SHOW CREATE TABLE 对比 schema → migrate down

### Story 20.3: cosmetic_items seed（满足 AR18 数量约束 + URL 占位）

As a 服务端开发,
I want cosmetic_items 表预置足够广度的配置集合,
So that 开箱抽奖不会反复出同一件，节点 11 合成也有足够材料分布.

**Acceptance Criteria:**

**Given** Story 20.2 表已就绪
**When** 完成本 story
**Then** seed 数据量满足 AR18:
- common ≥ **8 件**，至少覆盖 4 个不同槽位（hat / gloves / glasses / neck 等）
- rare ≥ **4 件**
- epic ≥ **2 件**
- legendary ≥ **1 件**
**And** 每件 cosmetic 的 `icon_url` 与 `asset_url` 必须为可访问 URL（按 AR18 / AR19 URL 约束），MVP 阶段可用 placeholder（如 `https://placehold.co/128x128?text=Hat-Yellow`）
**And** `drop_weight` 按品质递减分布（如 common=100, rare=20, epic=4, legendary=1），保证抽奖比例合理
**And** seed 通过 migration 文件 `migrations/0012_seed_cosmetic_items.sql` 写入（INSERT IGNORE 防重复）
**And** **单元测试覆盖**（≥4 case）:
- happy: migrate up 后 cosmetic_items 至少 15 行（8+4+2+1）+ URL 都非空
- happy: 各品质数量符合 AR18 最小约束
- happy: common 至少覆盖 4 个不同 slot 值
- happy: 重复 migrate up → 不重复插入
**And** **集成测试覆盖**（dockertest）: migrate up → SELECT count(*) GROUP BY rarity → 验证各品质数量 ≥ AR18 约束

### Story 20.4: chest_open_logs migration

As a 服务端开发,
I want chest_open_logs 表的 migration,
So that 每次开箱都有审计记录，便于排查掉落问题与运营分析.

**Acceptance Criteria:**

**Given** Epic 4 migrations 框架已就绪
**When** 完成本 story
**Then** `migrations/0013_init_chest_open_logs.sql` 按 数据库设计.md §5.7 创建表:
- 字段全集（含 `id`, `user_id`, `chest_id`, `cost_steps`, `reward_user_cosmetic_item_id`, `reward_cosmetic_item_id`, `reward_rarity`, `created_at`）
- `KEY idx_user_id_created_at` + `KEY idx_reward_cosmetic_item_id`
**And** 含 down.sql
**And** **单元测试覆盖**（≥3 case）:
- happy: migrate up 后表存在 + 字段类型 + 索引都符合 §5.7
- happy: migrate down 后表删除
- edge: 重复 migrate up → 幂等
**And** **集成测试覆盖**（dockertest）: migrate up → SHOW CREATE TABLE 对比 schema → migrate down

### Story 20.5: GET /chest/current 接口（按 unlock_at 动态判定 unlockable）

As an iPhone 用户,
I want 查询我当前宝箱的状态 + 倒计时,
So that 主界面可以正确显示宝箱组件.

**Acceptance Criteria:**

**Given** Story 4.6 登录初始化已创建首个 chest（status=counting）
**When** 调用 `GET /chest/current`
**Then** 返回 `{id, status, unlockAt, openCostSteps:1000, remainingSeconds}`
**And** **status 动态判定**：
- 数据库 status=1 (counting) AND `unlock_at <= now` → 返回 status=2 (unlockable), remainingSeconds=0
- 数据库 status=1 AND `unlock_at > now` → 返回 status=1, remainingSeconds=`unlock_at - now`
- 数据库 status=2 → 返回 status=2, remainingSeconds=0（已解锁待开）
**And** 接口要求 auth
**And** 用户没有 chest（理论不该发生，因登录已创建）→ 4001
**And** **不更新 DB**（动态判定，节省写）—— 真正状态变更在开箱时
**And** **单元测试覆盖**（≥5 case，mocked chest repo + clock mock）:
- happy: counting + unlock_at 在未来 5 分钟 → 返回 status=1, remainingSeconds=300
- happy: counting + unlock_at 已过 → 返回 status=2, remainingSeconds=0
- happy: status=2 → 返回 status=2
- edge: 用户无 chest → 4001
- edge: clock 跨秒边界 → remainingSeconds 计算精度合理（不出现 -1 等异常）
**And** **集成测试覆盖**（dockertest）: 创建 user + chest with unlock_at=now+10min → curl GET /chest/current → status=1, remainingSeconds≈600

### Story 20.6: POST /chest/open 事务 + idempotencyKey + 加权抽取

As an iPhone 用户,
I want 我可以开启已解锁的宝箱，扣 1000 步数后获得一个装扮道具（仅展示，不入仓）,
So that 我能体验开宝箱玩法.

**Acceptance Criteria:**

**Given** Story 20.5 chest 状态可判定 + Story 20.3 cosmetic_items 已 seed + Story 7.3 user_step_accounts 可读写
**When** 调用 `POST /chest/open` 带 `idempotencyKey`
**Then** service 流程:
- 检查 Redis 幂等键 `idem:{userId}:chest_open:{idempotencyKey}` → 已有结果直接返回（防重）
- 在事务中:
  - 查 chest（含 FOR UPDATE 行锁），判定 unlockable → 否则 4002
  - 查 user_step_accounts FOR UPDATE，available_steps >= 1000 → 否则 3002
  - 扣 available_steps - 1000，consumed_steps + 1000，version + 1
  - 加权抽取 cosmetic_items（按 drop_weight，仅 is_enabled=1）→ 拿到 cosmetic_item_id
  - **本节点 7 不创建 user_cosmetic_items 实例**（按节点规划），由 Story 23.5 节点 8 修改本事务补"入仓"
  - 写 chest_open_logs（reward_user_cosmetic_item_id 字段填 0 或 NULL，节点 8 后填真实 id）
  - 创建下一轮 chest（删旧记录 + 新记录 status=1, unlock_at=now+10min）
- Redis 写幂等结果（TTL 24h）
- 返回 reward + stepAccount + nextChest
**And** 接口要求 auth + rate_limit
**And** **单元测试覆盖**（≥7 case，mocked repo + Redis）:
- happy: chest unlockable + steps=1500 → 扣 1000，余 500 + 抽取奖励 + 写 log + 刷新 chest
- edge: chest 不存在 → 4001
- edge: chest counting (unlock_at 未到) → 4002
- edge: steps < 1000 → 3002
- edge: 同一 idempotencyKey 第二次调用 → 不开事务，返回首次结果
- edge: 加权抽取（mock random）→ 验证按权重分布（多次跑统计）
- edge: 事务中 step 扣减失败（mock）→ 整体回滚，chest 仍 unlockable
**And** **集成测试覆盖**（dockertest + Redis）: 开 chest → DB user_step_accounts.available_steps -= 1000 + chest_open_logs 多 1 行 + 旧 chest 被新的替换

### Story 20.7: dev 端点 POST /dev/force-unlock-chest

As a demo / 开发者,
I want 一个 dev 接口强制把当前 chest 切到 unlockable 状态,
So that demo 时不必等 10 分钟倒计时.

**Acceptance Criteria:**

**Given** Epic 1 Story 1.6 Dev Tools 框架已就绪 + Story 20.5 chest 状态机可读
**When** 仅在 BUILD_DEV=true 模式调用 `POST /dev/force-unlock-chest {userId: int64}`
**Then** service 直接 UPDATE user_chests SET unlock_at = now WHERE user_id=?
**And** 生产构建下访问该端点返回 404
**And** 接口**不**要求 auth
**And** **单元测试覆盖**（≥3 case）:
- happy: dev mode + 用户存在 → unlock_at 更新为 now
- edge: dev mode + 用户无 chest → 1003
- edge: 非 dev mode → 路由返回 404
**And** **集成测试覆盖**（dockertest + BUILD_DEV=true）: 用户 chest unlock_at 在未来 → /dev/force-unlock-chest → GET /chest/current 返回 status=2

### Story 20.8: dev 端点 POST /dev/grant-cosmetic-batch（节点 8 完成后开放）

As a demo / 开发者,
I want 一个 dev 接口给指定用户批量发放指定品质 cosmetic 实例,
So that demo 节点 11 合成时可以一键凑齐 10 件 common 不必反复开箱.

**Acceptance Criteria:**

**Given** Epic 1 Dev Tools 框架已就绪 + Story 20.3 cosmetic_items 已 seed + **依赖 user_cosmetic_items 表（节点 8 Story 23.2 完成后才能真实写库）**
**When** 仅在 BUILD_DEV=true 模式调用 `POST /dev/grant-cosmetic-batch {userId, rarity, count}`
**Then** service 直接 INSERT 多条 user_cosmetic_items（按 rarity 从 cosmetic_items 中随机抽 count 个 cosmetic_item_id 创建实例）
**And** **节点 7 阶段实装策略**：路由注册 + handler 框架 + 单测 mock 都完成；handler 内部真实写库逻辑等 Story 23.2 user_cosmetic_items 表 migration 完成后开放（在 Epic 23 sprint 内打开开关）
**And** 生产构建下访问该端点返回 404
**And** **单元测试覆盖**（≥3 case）:
- happy: dev mode + rarity=1, count=10 → DB user_cosmetic_items 多 10 行（cosmetic_item_id 来自 common 池随机）
- edge: dev mode + rarity=99（非法）→ 1002
- edge: dev mode + count=0 → 1002
**And** **集成测试覆盖**（节点 8 完成后跑）: /dev/grant-cosmetic-batch {rarity:1, count:10} → DB 验证 10 行新实例

### Story 20.9: Layer 2 集成测试 - 开箱事务全流程

As a 资产事务负责人,
I want 一组深度集成测试覆盖开箱事务的失败回滚 / 并发 / 边界 / 幂等,
So that NFR1 / NFR5 / §8.3 (开箱事务) 有自动化保障.

**Acceptance Criteria:**

**Given** Story 20.6 happy path 已通过
**When** 完成本 story
**Then** 输出 `internal/service/chest_service_integration_test.go` 用 dockertest（MySQL + Redis）覆盖:
- **完整流程**: 创建 user 含 chest + 1500 步数 → 等 chest unlockable → 开箱 → 验证扣 1000 + 奖励抽取 + log + 下一轮 chest
- **回滚 1**: 扣步数 mock 抛 error → 验证 chest 仍 unlockable + 没有 chest_open_logs + steps 不变
- **回滚 2**: 抽奖 mock 抛 error → 验证 steps 也未扣
- **回滚 3**: 写 log mock 抛 error → 验证整体回滚
- **回滚 4**: 创建下一轮 chest mock 抛 error → 验证整体回滚（包括步数）
- **幂等 1**: 同一 idempotencyKey 重复调 100 次 → 只成功 1 次，DB chest_open_logs 只多 1 行，余下 99 次返回相同结果
- **幂等 2**: 不同 idempotencyKey + 步数足够 → 各次都能成功开箱（每次扣 1000）
- **并发 1**: 同一用户 100 个并发请求（同一 idempotencyKey）→ 只 1 次开箱成功
- **并发 2**: 同一用户 100 个并发请求（不同 idempotencyKey）+ 步数仅够开 1 次 → 只 1 次成功，其他 99 次返回 3002
- **边界 1**: 步数恰好 999 → 3002
- **边界 2**: 步数恰好 1000 → 成功，余 0
- **边界 3**: 步数恰好 1001 → 成功，余 1
- **边界 4**: chest unlock_at 比 now 早 1ms → unlockable
- **抽奖分布**: 1000 次开箱 → 各品质比例符合 drop_weight 设计
**And** 全部场景用 dockertest 真实 MySQL + Redis 跑通
**And** 集成测试在 CI 标 `// +build integration` tag

## Epic 21: iOS - 首页宝箱倒计时 + 奖励弹窗

节点 7 iOS 端：首页宝箱组件 + 倒计时 timer + 开箱按钮 + 奖励弹窗。**节点 7 不入仓**：弹窗仅展示，不更新仓库（节点 8 才有仓库）。

### Story 21.1: 首页宝箱组件 SwiftUI（倒计时 Timer + 状态切换 UI）

As an iPhone 用户,
I want 主界面有宝箱图标 + 倒计时数字显示，倒计时结束后变成可开启状态,
So that 我能看到宝箱的进度并知道何时可以开.

**Acceptance Criteria:**

**Given** Epic 2 Story 2.2 主界面有宝箱占位区
**When** 完成本 story
**Then** 实装 `ChestCardView` SwiftUI 组件:
- 状态 counting: 显示宝箱图标（灰色或锁定样式）+ `mm:ss` 倒计时数字（每秒刷新）+ "倒计时" 标签
- 状态 unlockable: 显示宝箱图标（金色或高亮）+ "可开启" 标签 + 开箱按钮
- 倒计时由本地 Timer 驱动（CADisplayLink 或 SwiftUI TimelineView），每秒更新
- 倒计时归零时**自动切到 unlockable 视觉状态**（不需要立即调 server，但 Story 21.2 会定时纠正）
**And** ChestCardView 替换 Story 2.2 主界面宝箱区占位
**And** ViewModel: `HomeViewModel.chestState: ChestState`，含 `id`, `status`, `unlockAt`, `openCostSteps`, `remainingSeconds`
**And** **单元测试覆盖**（≥4 case，fake timer + UI snapshot）:
- happy: ChestState{counting, remainingSeconds: 300} → 显示 "5:00 倒计时" + 灰色图标
- happy: 倒计时每秒减 1
- happy: 倒计时到 0 → 视觉切换到 unlockable
- happy: 直接给 ChestState{unlockable} → 显示金色图标 + 开箱按钮
**And** **UI 测试覆盖**: mock HomeViewModel.chestState → 主界面 ChestCardView 验证 accessibility identifier "chestCard_counting" / "chestCard_unlockable"

### Story 21.2: GET /chest/current 调用 + 状态展示 + 主动定时纠正

As an iPhone 用户,
I want App 启动时拉到准确的宝箱状态，且本地倒计时不要持续偏离 server,
So that 我看到的宝箱状态可信.

**Acceptance Criteria:**

**Given** Story 21.1 ChestCardView 可显示状态 + Story 20.5 server GET /chest/current 可用
**When** 完成本 story
**Then** 实装 `LoadChestUseCase`:
- App 启动后 / 回前台时 / **每 60 秒定时**调用 GET /chest/current
- 拿到 server 状态后强制覆盖本地 chestState
- 如果 server 返回 unlockable 但本地仍 counting → 立即切到 unlockable
- 如果 server 返回 counting 且 remainingSeconds 与本地差距 > 5 秒 → 校准本地 timer
**And** LoadChestUseCase 失败时不破坏 UI（继续按上次拉到的状态显示，下次重试）
**And** **单元测试覆盖**（≥5 case，mocked APIClient + fake timer）:
- happy: 启动 → 调 /chest/current → ViewModel.chestState 更新
- happy: 回前台触发拉取
- happy: 定时器每 60 秒触发一次
- happy: server unlockable + 本地 counting → ViewModel 状态切换
- edge: API 失败 → ViewModel 保留旧状态，不破坏 UI
**And** **集成测试覆盖**（XCUITest + mock server）: 启动 → ChestCardView 显示 server 返回的状态

### Story 21.3: 开箱按钮 + 调用 POST /chest/open（含 idempotencyKey 生成）

As an iPhone 用户,
I want 点开箱按钮后 App 调用 server 完成开箱，并防止重复开箱,
So that 我不会因为多点几下被多次扣步数.

**Acceptance Criteria:**

**Given** Story 21.1 unlockable 状态显示开箱按钮 + Story 20.6 server POST /chest/open 可用
**When** 用户点开箱按钮
**Then** 触发 `OpenChestUseCase`:
- 客户端生成 idempotencyKey（UUID v4）
- 显示 loading（按钮变 disabled + 转圈）
- 调用 POST /chest/open 带 idempotencyKey
- 成功 → 更新 ViewModel.chestState (用 response.nextChest) + 步数账户 + 触发 Story 21.4 奖励弹窗
- 失败 → ErrorPresenter 弹 alert（按错误码：4002 "宝箱未解锁" / 3002 "步数不足，再走走吧"）
**And** **同一次开箱过程中**点击按钮不重复触发（按钮 disabled + 同 idempotencyKey 即使重发也走幂等）
**And** UseCase 失败后按钮恢复可点（但生成**新的** idempotencyKey 用于重试，避免命中旧的幂等结果）
**And** **单元测试覆盖**（≥5 case，mocked APIClient）:
- happy: 调 chest/open → 拿到 reward → ViewModel.chestState 更新为 nextChest + 步数刷新 + 弹窗触发
- happy: 同一 use case 内多次点 → 只发 1 次 API（按钮 disabled）
- edge: 4002 → 弹 alert "宝箱未解锁"
- edge: 3002 → 弹 alert "步数不足"
- edge: 1008 幂等冲突 → 不应发生（同一 use case 内不重复发同一 key），但兜底处理

### Story 21.4: 奖励弹窗 popup

As an iPhone 用户,
I want 开箱成功后看到一个弹窗展示我获得的装扮（图标 + 名称 + 品质）,
So that 我有获得感.

**Acceptance Criteria:**

**Given** Story 21.3 开箱成功后 reward 数据已可用
**When** 完成本 story
**Then** 实装 `RewardPopupView` SwiftUI 组件:
- 中央显示 cosmetic 图标（AsyncImage 加载 reward.iconUrl）
- 下方显示 "获得 {reward.name}" + 品质徽章（按 rarity 颜色：common 灰 / rare 蓝 / epic 紫 / legendary 金）
- "确定"按钮关闭弹窗
- 弹窗用 SwiftUI sheet 或自定义 overlay，含淡入动画
**And** 节点 7 阶段弹窗显示后**不更新仓库状态**（节点 8 才有仓库），仅纯展示
**And** **单元测试覆盖**（≥3 case，UI snapshot）:
- happy: reward.rarity=2 (rare) → 弹窗显示蓝色徽章
- happy: reward.rarity=4 (legendary) → 显示金色徽章
- happy: 点确定 → 弹窗关闭，ViewModel.rewardPopup = nil
**And** **UI 测试覆盖**（XCUITest + mock server 返回固定 reward）: 开箱 → 弹窗出现 → 验证 accessibility identifier "rewardPopup" + 点关闭 → 弹窗消失

### Story 21.5: 开箱前主动同步步数（接 Story 8.5 手动触发接口）

As an iPhone 用户,
I want 我点开箱前 App 自动同步一次步数到 server,
So that server 端用最新可用步数判断我是否够开（避免本地步数已增加但 server 还没收到导致 3002 误报）.

**Acceptance Criteria:**

**Given** Story 8.5 提供 SyncStepsUseCase 手动触发接口 + Story 21.3 开箱按钮已就绪
**When** 用户点开箱按钮
**Then** OpenChestUseCase 在调 /chest/open **之前**先 await SyncStepsUseCase（同步当前步数到 server）
**And** 同步失败也继续开箱（不阻塞，让 server 用上一次 sync 后的余额判断）
**And** 同步耗时 > 2 秒时显示 loading 提示 "同步步数中…"
**And** **单元测试覆盖**（≥4 case，mocked SyncStepsUseCase + APIClient）:
- happy: 同步成功 → 调 chest/open → 用最新步数判定
- happy: 同步失败 → 仍调 chest/open → 用旧余额判定
- happy: 同步耗时 1 秒 → 不显示提示
- happy: 同步耗时 3 秒 → 显示提示 "同步步数中…"
**And** **集成测试覆盖**（XCUITest + mock server）: 模拟器 HealthKit 增加步数 → 立即点开箱 → 验证 mock server 收到先 sync 后 open 两次请求 + open 用的是新余额

## Epic 22: 对齐 - 节点 7 宝箱开箱链路联调（不含入仓）

把 Epic 20 (Server 宝箱) + Epic 21 (iOS 首页宝箱) 真实串联，验证节点 7 §4.7 全部验收标准达成。**重要边界**：本节点不入仓，奖励仅展示弹窗。

### Story 22.1: 跨端集成测试场景 - 宝箱开箱 E2E

**Acceptance Criteria:**

**Given** Epic 20 + Epic 21 全部完成
**When** 完成本 story
**Then** 输出文档 `_bmad-output/implementation-artifacts/e2e/node-7-chest-e2e.md`，含:
- 准备步骤: server 启动 + cosmetic seed 已就绪 + 模拟器登录后默认 chest 已创建（counting 10min）
- 验证场景 1（倒计时显示）: App 启动 → 主界面 ChestCardView 显示 "倒计时 10:00" + 灰色图标 → 等 1 分钟 → 显示 "9:00"
- 验证场景 2（dev force-unlock）: 用 /dev/force-unlock-chest 切到 unlockable → 主界面 60 秒内自动纠正显示 "可开启" 金色图标 + 开箱按钮
- 验证场景 3（步数不足拒开）: 用户步数 500 → 点开箱 → server 返回 3002 → alert "步数不足，再走走吧"
- 验证场景 4（dev grant-steps + 开箱）: /dev/grant-steps {steps:1500} → 步数变 2000（含原有 500）→ 点开箱 → 同步步数 → 调 chest/open → 弹奖励弹窗（含 cosmetic icon + name + rarity 徽章）→ 步数减为 1000 → ChestCardView 切回 counting + 新 unlock_at
- 验证场景 5（防重复开箱）: 同一开箱过程中快速点 5 次按钮 → server 收到 1 次请求（按钮 disabled）→ 弹 1 次弹窗 → DB chest_open_logs 只多 1 行
- 验证场景 6（开箱失败重试）: 第一次 open 时 mock server 故意返回 1009 服务繁忙 → alert "服务繁忙" → 用户点重试 → 生成**新 idempotencyKey** → 第二次成功
- 验证场景 7（不入仓验证）: 开箱后用 mysql 查 user_cosmetic_items 表 → **没有新增行**（节点 7 不入仓，节点 8 才补）+ chest_open_logs 多 1 行（reward_user_cosmetic_item_id=0 或 NULL）
- 验证场景 8（抽奖多样性）: 连开 20 次 → 弹窗里能看到至少 3-4 种不同 cosmetic（验证 AR18 数量约束生效）
**And** 文档含截图位

### Story 22.2: 节点 7 demo 验收

**Acceptance Criteria:**

**Given** Story 22.1 E2E 文档已就绪
**When** 完成本 story
**Then** 按 §4.7 节点 7 验收标准逐项打勾:
- [ ] 首页能展示宝箱剩余倒计时
- [ ] 倒计时结束后宝箱状态正确变更
- [ ] 用户可在步数足够时开启宝箱
- [ ] 开启后能看到奖励弹窗
- [ ] **明确**: 奖励**不入仓**（节点 8 验收）
- [ ] **新增**: dev force-unlock-chest 可用
- [ ] **新增**: cosmetic seed 数据多样性满足 demo
**And** 输出 demo 录屏到 `_bmad-output/implementation-artifacts/demo/node-7-demo-<date>/`
**And** 任何未通过项必须登记原因 + 是否后置

### Story 22.3: 文档同步与 tech debt 登记

**Acceptance Criteria:**

**Given** Epic 20-21 完成 + Story 22.2 demo 通过
**When** 完成本 story
**Then** 检查并更新（无变更也要明确标注）:
- [ ] `docs/宠物互动App_V1接口设计.md` §7.1 / §7.2: 实装与 Story 20.1 锚定的 schema 一致
- [ ] `docs/宠物互动App_数据库设计.md` §5.6 (user_chests) / §5.7 (chest_open_logs) / §5.8 (cosmetic_items): 表结构与文档一致
- [ ] `docs/宠物互动App_时序图与核心业务流程设计.md` §7-§8 (宝箱状态流转 + 开箱事务): 时序与实装一致
- [ ] `docs/宠物互动App_Go项目结构与模块职责设计.md` §6.5 (Chest 模块): 模块结构对齐
- [ ] `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §6.5 (Chest 模块): 倒计时 / 弹窗 设计
**And** `_bmad-output/implementation-artifacts/tech-debt-log.md` 追加节点 7 tech debt，可能的:
- chest_open_logs.reward_user_cosmetic_item_id 字段写 0 或 NULL，节点 8 后必须补真实 id（迁移既有 log 是另一个 ticket）
- 抽奖加权随机用 math/rand 全局源，并发场景不一定均匀；可后续切 crypto/rand 或 per-request seed
- 倒计时 60 秒纠正间隔 hard-code，未抽到配置
- iOS 端 RewardPopupView 资产用 placeholder URL（依赖 cosmetic seed 的 placeholder）
- 节点 7 阶段不入仓导致 chest_open_logs.reward_user_cosmetic_item_id 字段语义模糊，建议节点 8 完成后做一次"补 log id"的 backfill 任务（或接受历史 log 字段为空作为已知现象）

## Epic 23: Server - 背包查询接口 + 修改开箱事务补"入仓"

节点 8 Server 端：user_cosmetic_items 表 + catalog / inventory 查询接口 + **关键 Story 23.5 修改 Story 20.6 开箱事务补"入仓"**（让节点 7 延期的入仓逻辑落地）。

### Story 23.1: 接口契约最终化（GET /cosmetics/catalog + GET /cosmetics/inventory）

As a 跨端契约负责人,
I want 把仓库与配置查询接口 schema 锚定到 V1接口设计文档,
So that iOS Epic 24 可以基于稳定契约并行开工.

**Acceptance Criteria:**

**Given** docs/宠物互动App_V1接口设计.md §8.1 / §8.2 已有初稿
**When** 完成本 story
**Then** §8.1 (GET /cosmetics/catalog) schema 完全锚定:
- response data: `{items: [{cosmeticItemId, code, name, slot, rarity, iconUrl, assetUrl}]}`
**And** §8.2 (GET /cosmetics/inventory) schema 完全锚定:
- response data: `{groups: [{cosmeticItemId, name, slot, rarity, iconUrl, assetUrl, count, instances: [{userCosmeticItemId, status}]}]}`
- groups 按 cosmetic_item_id 聚合，每组含 count + instances 数组
- instance.status 枚举: 1=in_bag, 2=equipped, 3=consumed
**And** 标注"契约自此 Epic 起冻结"

### Story 23.2: user_cosmetic_items migration

As a 服务端开发,
I want user_cosmetic_items 表的 migration,
So that 玩家持有的装扮实例可以持久化（节点 8-11 全部依赖此表）.

**Acceptance Criteria:**

**Given** Epic 4 migrations 框架已就绪 + Story 20.2 cosmetic_items 已存在
**When** 完成本 story
**Then** `migrations/0014_init_user_cosmetic_items.sql` 按 数据库设计.md §5.9 创建表:
- 字段全集（含 `id`, `user_id`, `cosmetic_item_id`, `status`, `source`, `source_ref_id`, `obtained_at`, `consumed_at`, `created_at`, `updated_at`）
- `KEY idx_user_id_status` + `KEY idx_user_id_cosmetic_item_id` + `KEY idx_source`
**And** 含 down.sql
**And** **单元测试覆盖**（≥3 case）:
- happy: migrate up 后表存在 + 字段类型 + 索引都符合 §5.9
- happy: migrate down 后表删除
- edge: 重复 migrate up → 幂等
**And** **集成测试覆盖**（dockertest）: migrate up → SHOW CREATE TABLE 对比 schema → migrate down

### Story 23.3: GET /cosmetics/catalog 接口

As an iPhone 用户,
I want 我可以查询所有可用装扮配置,
So that 仓库页 / 合成页 / 装扮详情可以查到 cosmetic 元数据.

**Acceptance Criteria:**

**Given** Story 20.3 cosmetic_items 已 seed
**When** 调用 `GET /cosmetics/catalog`
**Then** 返回 `{items: [...]}`，仅含 `is_enabled=1` 的配置
**And** 接口要求 auth
**And** 列表按 rarity ASC + slot ASC 排序
**And** 列表大约 15-50 条，不分页
**And** **单元测试覆盖**（≥4 case，mocked cosmetic repo）:
- happy: DB 15 个 enabled → response items 长度 = 15，含全部字段
- happy: 1 个 disabled → 不返回
- edge: response 严格符合 §8.1 schema
- edge: DB 错误 → 1009
**And** **集成测试覆盖**（dockertest）: seed cosmetic_items → curl → response.items 含正确数量 + 字段值

### Story 23.4: GET /cosmetics/inventory 接口（聚合 + 实例列表）

As an iPhone 用户,
I want 我可以查询自己背包中的装扮实例（按配置聚合）,
So that 仓库页可以渲染"小黄帽 x 3"这种聚合展示 + 展开看每件实例 id.

**Acceptance Criteria:**

**Given** Story 23.2 user_cosmetic_items 表已就绪
**When** 调用 `GET /cosmetics/inventory`
**Then** service 流程:
- 查 `user_cosmetic_items WHERE user_id=? AND status IN (1, 2)` (in_bag + equipped，**不含 consumed**)
- 按 cosmetic_item_id 分组
- 每组关联 cosmetic_items 拿配置元信息
- 返回 `{groups: [{cosmeticItemId, name, slot, rarity, iconUrl, assetUrl, count, instances: [{userCosmeticItemId, status}]}]}`
**And** 接口要求 auth
**And** 用户背包为空 → 返回 `{groups: []}`
**And** 性能：单用户预期实例数 < 1000，单 SQL 查询足以，不分页
**And** **单元测试覆盖**（≥5 case，mocked repo）:
- happy: 用户有 3 件 hat (cosmeticId=12) + 1 件 scarf (cosmeticId=24) → groups 长度 = 2，第一组 count=3 + instances 长度=3
- happy: 用户有 0 件 → groups = []
- happy: 用户有 1 件 status=equipped → 仍包含在 inventory 中（status=2）
- edge: 用户有 1 件 status=consumed → 不出现在 inventory 中
- edge: 配置不存在的 cosmetic_item_id（理论不该有）→ skip + log warning
**And** **集成测试覆盖**（dockertest）: 创建 user + 5 个不同 cosmetic 实例 → curl GET /cosmetics/inventory → 验证 groups 数量 + count + instances

### Story 23.5: 修改开箱事务 - 创建 user_cosmetic_items 实例 + 补 chest_open_logs.reward_user_cosmetic_item_id（入仓）

As a 资产事务负责人,
I want 修改 Story 20.6 开箱事务，让奖励**真正进入仓库**,
So that 节点 8 验收的"开箱 → 入仓 → 仓库可见"链路成立.

**Acceptance Criteria:**

**Given** Story 23.2 user_cosmetic_items 表已就绪 + Story 20.6 开箱事务 happy path 已通过
**When** 完成本 story
**Then** 修改 `ChestService.OpenChest` 事务，**在抽奖产出 cosmetic_item_id 之后、写 chest_open_logs 之前**插入新步骤:
- 插入 user_cosmetic_items: `(user_id, cosmetic_item_id, status=1 in_bag, source=1 chest, source_ref_id=chest_id, obtained_at=now)`
- 拿到生成的 user_cosmetic_item_id
- 后续写 chest_open_logs 时把 `reward_user_cosmetic_item_id` 字段填这个真实 id（之前是 0 或 NULL）
- response.reward.userCosmeticItemId 也填这个真实 id
**And** **不破坏 Story 20.6 的其他逻辑**：扣步数、idempotencyKey、抽奖分布、刷新下一轮 chest 全部不变
**And** **不破坏 Layer 2 集成测试 Story 20.9**：所有现有测试场景都应该仍通过 + 增加新场景"开箱后 user_cosmetic_items 多 1 行"
**And** **单元测试覆盖**（≥4 case，扩展 Story 20.6 单测，mocked repo）:
- happy: 开箱成功 → user_cosmetic_items 创建 1 行（status=1, source=1）+ chest_open_logs.reward_user_cosmetic_item_id 非零 + response.reward.userCosmeticItemId 非零
- edge: user_cosmetic_items 插入失败（mock）→ 整体回滚，chest 仍 unlockable + 步数不变
- happy: 同一用户开 5 次箱 → DB user_cosmetic_items 多 5 行
- happy: idempotency 命中 → 不重复创建实例（依赖 Story 20.6 已有的幂等保护）
**And** **集成测试覆盖**（dockertest）:
- 创建 user + 1500 步 + force-unlock-chest → 开箱 → DB user_cosmetic_items 多 1 行 + chest_open_logs 多 1 行 + GET /cosmetics/inventory 返回该实例
**And** **Layer 2 测试更新**: Story 20.9 集成测试新增一组验证 "user_cosmetic_items 也回滚"（任何步骤失败时 user_cosmetic_items 也回滚）
**And** 本 Story 完成后，Story 20.8 (`/dev/grant-cosmetic-batch`) 可以打开真实写库逻辑（之前是 placeholder）

## Epic 24: iOS - 仓库页面（聚合 + 实例列表）+ 主界面入口完善

节点 8 iOS 端：仓库页骨架 + inventory API 接入 + 筛选 + 主界面入口完善 + 穿戴入口位置预留（节点 9 实装行为）。

### Story 24.1: 仓库页 SwiftUI 骨架（聚合 grid + 实例展开）

As an iPhone 用户,
I want 仓库页能按"小黄帽 x 3"这样的聚合方式展示我的装扮，且可以展开看每件实例,
So that 我能清晰看到自己拥有什么 + 每件实例的具体 id（合成时需要）.

**Acceptance Criteria:**

**Given** Epic 2 Story 2.3 主界面"仓库"按钮已开 Sheet placeholder
**When** 完成本 story
**Then** 实装 `InventoryView` SwiftUI 替换 placeholder Sheet 内容:
- 顶部：标题 "仓库" + 关闭按钮
- 中间：聚合 grid（LazyVGrid 3 列），每个 cell 显示 cosmetic icon + name + count badge（如 "x 3"）+ 品质边框颜色
- 点击某 cell 展开下方 instance 列表（含 userCosmeticItemId + 状态徽章 in_bag/equipped）
- 空仓库显示 placeholder "去开宝箱获得装扮吧 →"
**And** 用 `InventoryViewModel` 持有 `groups: [InventoryGroup], selectedCosmeticId: String?`
**And** **单元测试覆盖**（≥4 case，UI snapshot）:
- happy: ViewModel 注入 mock 5 个 groups → grid 渲染 5 个 cell
- happy: 点击某 cell → selectedCosmeticId 更新 → 实例列表展开
- happy: groups 为空 → 显示空仓库 placeholder
- edge: count > 99（理论不该）→ 显示 "99+"
**And** **UI 测试覆盖**（XCUITest）: 主界面点"仓库"→ Sheet 弹 InventoryView → 验证 accessibility identifier "inventoryView"

### Story 24.2: LoadInventoryUseCase + GET /cosmetics/inventory 调用

As an iPhone 用户,
I want 仓库页打开时立即从 server 加载我的最新装扮列表,
So that 我看到的是真实数据.

**Acceptance Criteria:**

**Given** Story 24.1 InventoryView 骨架 + Story 23.4 server GET /cosmetics/inventory 可用
**When** InventoryView .task / .onAppear 触发
**Then** 调 `LoadInventoryUseCase`:
- GET /cosmetics/inventory
- 解析 response.groups 给 ViewModel.groups
- 加载中显示 ProgressView
- 失败显示 RetryView（复用 ErrorPresenter）
**And** 每次打开 Sheet 都重新加载（不缓存）—— 因为开箱后需要立即看到新道具
**And** **单元测试覆盖**（≥4 case，mocked APIClient）:
- happy: API 返回 5 个 groups → ViewModel.groups 长度 = 5
- happy: API 返回空 → ViewModel.groups = []，View 显示空 placeholder
- edge: API 失败（network error）→ 显示 RetryView
- edge: API 失败后用户手动重试 → 重新发起请求

### Story 24.3: 基础筛选交互（按品质 / 槽位 tab）

As an iPhone 用户,
I want 仓库可以按品质或部位筛选,
So that 我有大量道具时也能快速找到想要的.

**Acceptance Criteria:**

**Given** Story 24.1 / 24.2 已就绪
**When** 完成本 story
**Then** InventoryView 顶部加 segment control: "全部 / 品质 / 槽位"
- "全部": 显示所有 groups
- "品质": 按品质（common / rare / epic / legendary）分段展示，4 个区段
- "槽位": 按 slot（hat / gloves / glasses / neck 等）分段展示
**And** 筛选**纯客户端**（不重新发 API），由 ViewModel 对 groups 做 filter / group_by
**And** 切换 tab 时有平滑动画
**And** **单元测试覆盖**（≥4 case，mocked groups）:
- happy: 5 个 cell 含 2 common + 2 rare + 1 epic → "品质" tab 渲染 3 个区段
- happy: 切到"槽位" → 按 slot 分组展示
- happy: 切回"全部" → 平铺所有
- edge: 某品质 0 件 → "品质" tab 该区段折叠或显示 "暂无"

### Story 24.4: 主界面"仓库"按钮入口完善

As an iPhone 用户,
I want 主界面的"仓库"按钮直接打开仓库页（之前是 placeholder Sheet）,
So that 我可以从主界面快速访问仓库.

**Acceptance Criteria:**

**Given** Epic 2 Story 2.3 主界面"仓库"按钮已存在但触发 placeholder
**When** 完成本 story
**Then** 主界面"仓库"按钮触发 NavigationRouter `present(.inventory)`，弹出 InventoryView Sheet
**And** 按钮可加 badge 提示新道具数量（**MVP 不做** badge，留 tech debt）
**And** **单元测试覆盖**（≥2 case）:
- happy: 点击"仓库"按钮 → coordinator.present(.inventory) 被调用
- happy: Sheet dismiss 后 → coordinator.dismiss() 被调用
**And** **UI 测试覆盖**（XCUITest）: 主界面点"仓库"→ Sheet 弹出 InventoryView → 关闭

### Story 24.5: 穿戴入口按钮位置预留（行为留空，节点 9 实装）

As an iOS 开发,
I want 在 instance 详情区预留"穿戴"按钮位置（disabled / 占位状态），不实装真实穿戴逻辑,
So that 节点 9 Story 27.1 可以直接接入这个按钮位置，不必返工 InventoryView 布局.

**Acceptance Criteria:**

**Given** Story 24.1 InventoryView instance 列表已展开
**When** 完成本 story
**Then** 每个 instance 行右侧显示"穿戴"按钮（占位）:
- 节点 8 阶段：按钮 disabled + 灰色 + 文案 "穿戴（节点 9）"
- 节点 9 Story 27.1 实装时：变为 enabled + 文案 "穿戴" + 触发 EquipUseCase
**And** instance.status=equipped 的实例显示"已装备"标签替代按钮
**And** 按钮位置 / 大小 / 间距按 UX 设计稿固定，节点 9 不需要重排版
**And** **单元测试覆盖**（≥3 case，UI snapshot）:
- happy: 节点 8 阶段 instance 行渲染 → 按钮 disabled + 文案正确
- happy: instance.status=equipped → 显示 "已装备" 标签，不显示按钮
- edge: 按钮 onTap 被点 → 不触发任何 action（节点 8 阶段是 no-op）

## Epic 25: 对齐 - 节点 8 仓库链路联调（开箱奖励 → 入仓 → 仓库可见）

把 Epic 23 (Server 仓库 + 修改开箱事务补入仓) + Epic 24 (iOS 仓库页) 真实串联，验证节点 8 §4.8 全部验收标准达成。**重要边界**：本节点验证开箱→入仓→仓库可见全链路，不含穿戴（节点 9）。

### Story 25.1: 跨端集成测试场景 - 仓库链路 E2E

**Acceptance Criteria:**

**Given** Epic 23 + Epic 24 全部完成
**When** 完成本 story
**Then** 输出文档 `_bmad-output/implementation-artifacts/e2e/node-8-inventory-e2e.md`，含:
- 准备步骤: server 启动 + 用户全新登录（仓库为空）
- 验证场景 1（空仓库）: 主界面点"仓库"→ InventoryView 显示 "去开宝箱获得装扮吧"
- 验证场景 2（开箱入仓）: 关 Sheet → /dev/grant-steps {steps:1500} + /dev/force-unlock-chest → 主界面点开箱 → 弹奖励弹窗 → 关闭弹窗 → 主界面 → 重开仓库 Sheet → InventoryView 多 1 个 cosmetic cell（count=1）
- 验证场景 3（多次开箱聚合展示）: 连开 3 次 chest（grant-steps 3000 + force-unlock 3 次）→ 仓库 cell 按 cosmetic 聚合（如同一 cosmetic 开了 2 次 → 显示 count=2）
- 验证场景 4（实例展开）: 点击某 cell → 展开 instance 列表 → 验证有正确数量的 instance + 每个 instance 显示 in_bag 状态 + userCosmeticItemId
- 验证场景 5（筛选）: 切到 "品质" tab → 按 common/rare/epic 分段展示 → 切到 "槽位" tab → 按 hat/gloves 分段展示
- 验证场景 6（dev grant-cosmetic-batch）: BUILD_DEV=true，调 `/dev/grant-cosmetic-batch {rarity:1, count:10}`（Story 20.8 节点 8 后开放）→ 仓库 InventoryView 多 10 件 common 实例 → 验证 group count + instance 数量正确
- 验证场景 7（穿戴按钮预留）: instance 行的"穿戴"按钮显示 disabled + 文案 "穿戴（节点 9）"
- 验证场景 8（DB 一致性）: 开箱后 mysql 查 user_cosmetic_items 表 → 数量与仓库 cell instance 数量完全一致 + chest_open_logs.reward_user_cosmetic_item_id 不为空
**And** 文档含截图位

### Story 25.2: 节点 8 demo 验收

**Acceptance Criteria:**

**Given** Story 25.1 E2E 文档已就绪
**When** 完成本 story
**Then** 按 §4.8 节点 8 验收标准逐项打勾:
- [ ] 开箱奖励可稳定进入仓库
- [ ] 仓库页能看到已有道具
- [ ] 仓库数据与后端一致
- [ ] **新增**: 开箱事务补入仓后，Story 20.9 Layer 2 集成测试仍全部通过（含 user_cosmetic_items 回滚）
- [ ] **新增**: dev grant-cosmetic-batch 可用（节点 11 demo 必备）
- [ ] **明确**: 穿戴入口按钮预留，行为延期到节点 9
**And** 输出 demo 录屏到 `_bmad-output/implementation-artifacts/demo/node-8-demo-<date>/`
**And** 任何未通过项必须登记原因 + 是否后置

### Story 25.3: 文档同步与 tech debt 登记

**Acceptance Criteria:**

**Given** Epic 23-24 完成 + Story 25.2 demo 通过
**When** 完成本 story
**Then** 检查并更新（无变更也要明确标注）:
- [ ] `docs/宠物互动App_V1接口设计.md` §8.1 / §8.2: 实装与 Story 23.1 锚定的 schema 一致
- [ ] `docs/宠物互动App_数据库设计.md` §5.9 (user_cosmetic_items): 表结构与文档一致
- [ ] `docs/宠物互动App_时序图与核心业务流程设计.md` §8 (开箱事务): 时序更新含"创建实例 + 写日志"步骤（之前节点 7 不含此步）
- [ ] `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §6.6 (Cosmetics 模块): 仓库页 + 筛选设计
**And** `_bmad-output/implementation-artifacts/tech-debt-log.md` 追加节点 8 tech debt，可能的:
- 节点 7 阶段 chest_open_logs.reward_user_cosmetic_item_id=NULL 的历史记录是否需要 backfill（建议接受历史 NULL，新记录正常填值）
- InventoryView 不缓存，频繁开关 Sheet 重复发请求；可后续加 5 分钟 cache + invalidate（开箱后立即失效）
- 仓库 badge 提示新道具数量未实装
- inventory 接口对单用户实例数 > 1000 的性能未压测（理论 MVP 用户量小，但需登记）

## Epic 26: Server - 穿戴 / 卸下事务 + 一致性约束

节点 9 Server 端：穿戴 / 卸下接口 + user_pet_equips 表 + 同槽换装事务 + Layer 2 集成测试 Story 26.5。**关键 DB 约束**：UNIQUE(pet_id, slot) + UNIQUE(user_cosmetic_item_id)。

### Story 26.1: 接口契约最终化（POST /cosmetics/equip + unequip）

As a 跨端契约负责人,
I want 把穿戴 / 卸下接口 schema 锚定到 V1接口设计文档,
So that iOS Epic 27 可以基于稳定契约并行开工.

**Acceptance Criteria:**

**Given** docs/宠物互动App_V1接口设计.md §8.3 / §8.4 已有初稿
**When** 完成本 story
**Then** §8.3 (POST /cosmetics/equip) schema 完全锚定:
- request: `{petId: string, userCosmeticItemId: string}`
- response data: `{petId, equipped: {slot, userCosmeticItemId, cosmeticItemId, name}}`
- 错误码: 5001 道具不存在 / 5002 道具不属于当前用户 / 5003 道具状态不可用 / 5008 装扮已装备
**And** §8.4 (POST /cosmetics/unequip) schema 完全锚定:
- request: `{petId: string, slot: int}`
- response data: `{petId, slot, unequipped: true}`
- 错误码: 5004 装备槽位不匹配
**And** 标注"契约自此 Epic 起冻结"

### Story 26.2: user_pet_equips migration

As a 服务端开发,
I want user_pet_equips 表的 migration,
So that 穿戴关系可以持久化（含关键唯一约束）.

**Acceptance Criteria:**

**Given** Epic 4 migrations 框架已就绪
**When** 完成本 story
**Then** `migrations/0015_init_user_pet_equips.sql` 按 数据库设计.md §5.10 创建表，含**关键约束**:
- `UNIQUE KEY uk_pet_slot (pet_id, slot)` （一个宠物同一部位只能穿一件）
- `UNIQUE KEY uk_user_cosmetic_item_id (user_cosmetic_item_id)` （一件实例同时只能装备一次）
- `KEY idx_user_pet (user_id, pet_id)`
**And** 含 down.sql
**And** **单元测试覆盖**（≥3 case）:
- happy: migrate up 后表存在 + 全部约束都符合 §5.10
- happy: migrate down 后表删除
- edge: 重复 migrate up → 幂等
**And** **集成测试覆盖**（dockertest）: migrate up → 故意尝试违反两个 UNIQUE 约束 → 数据库拒绝插入

### Story 26.3: POST /cosmetics/equip 事务（含同槽换装）

As an iPhone 用户,
I want 我可以从仓库选一件实例穿戴到我的猫上，且如果该槽位已有装备会自动先卸下,
So that 穿戴体验顺畅，不需要手动先卸再穿.

**Acceptance Criteria:**

**Given** Story 26.2 user_pet_equips 已就绪 + Story 23.2 user_cosmetic_items 已存在
**When** 调用 `POST /cosmetics/equip` 带 petId + userCosmeticItemId
**Then** service 在事务中执行:
- 查 user_cosmetic_items WHERE id=? AND user_id=current_user，不存在 → 5001 / 不属于当前用户 → 5002
- 校验实例 status=1 (in_bag) → 否则 5003
- 查 cosmetic_items 拿到 slot
- 查该 pet 该 slot 是否已有装备:
  - 已有 → 删除旧 user_pet_equips 行 + UPDATE 旧实例 status=1 (in_bag) [自动卸下]
- INSERT user_pet_equips (user_id, pet_id, slot, user_cosmetic_item_id)
- UPDATE 当前实例 status=2 (equipped)
- 返回 `{petId, equipped: {slot, userCosmeticItemId, cosmeticItemId, name}}`
**And** 接口要求 auth
**And** **单元测试覆盖**（≥6 case，mocked repo）:
- happy: 该 slot 无装备 → 直接装上，user_pet_equips 多 1 行
- happy: 该 slot 已有装备 → 旧装备 status 改 in_bag + user_pet_equips 行更新 + 新装备 status equipped
- edge: 实例不存在 → 5001
- edge: 实例不属于当前用户 → 5002
- edge: 实例 status=consumed → 5003
- edge: pet 不属于当前用户 → 5002
**And** **集成测试覆盖**（dockertest）:
- 创建 user + pet + 1 件 hat (in_bag) → equip → DB user_pet_equips 1 行 + 实例 status=2
- 再 equip 另一件 hat → DB user_pet_equips 仍 1 行（同槽位换装）+ 旧 hat status=1 + 新 hat status=2

### Story 26.4: POST /cosmetics/unequip 事务

As an iPhone 用户,
I want 我可以卸下当前装备（按槽位指定）,
So that 我可以让猫不戴这件装扮.

**Acceptance Criteria:**

**Given** Story 26.3 装备关系已建立
**When** 调用 `POST /cosmetics/unequip` 带 petId + slot
**Then** service 在事务中执行:
- 查 user_pet_equips WHERE pet_id=? AND slot=?，不存在 → 5004 装备槽位不匹配
- 校验 pet 属于当前用户 → 否则 5002
- 拿到 user_cosmetic_item_id
- DELETE user_pet_equips 行
- UPDATE 实例 status=1 (in_bag)
- 返回 `{petId, slot, unequipped: true}`
**And** **单元测试覆盖**（≥4 case，mocked repo）:
- happy: 该槽位有装备 → 卸下，DB user_pet_equips 行删除 + 实例 status 回 in_bag
- edge: 该槽位无装备 → 5004
- edge: pet 不属于当前用户 → 5002
- edge: 卸下时事务部分失败（mock）→ 整体回滚（user_pet_equips 仍存在 + 实例仍 equipped）
**And** **集成测试覆盖**（dockertest）: 装一件 hat → unequip → user_pet_equips 行无 + 实例 status=1

### Story 26.5: Layer 2 集成测试 - 穿戴事务全流程

As a 资产事务负责人,
I want 一组深度集成测试覆盖穿戴 / 卸下事务的失败回滚 / 并发 / 一致性,
So that NFR1 / NFR2 / NFR11 / §8.4 (穿戴事务) 有自动化保障.

**Acceptance Criteria:**

**Given** Story 26.3 / 26.4 happy path 已通过
**When** 完成本 story
**Then** 输出 `internal/service/cosmetic_equip_integration_test.go` 用 dockertest 覆盖:
- **完整流程**: 创建 user + pet + 5 件不同 cosmetic 实例 → 依次穿戴到 5 个槽位 → 验证 user_pet_equips 5 行 + 5 个实例 status=2
- **同槽换装**: 同 slot 已有 hat A，穿 hat B → 验证 A status 回 1 + B status 变 2 + user_pet_equips 行更新（不是新增）
- **回滚 1**: equip 事务 mock 第 2 步（删旧装备）失败 → 验证旧装备仍 equipped + 新装备仍 in_bag + user_pet_equips 不变
- **回滚 2**: equip 事务 mock 最后一步（更新实例 status）失败 → 验证 user_pet_equips 行也回滚
- **回滚 3**: unequip 事务 mock 最后一步失败 → 验证 user_pet_equips 行未删
- **并发 1**: 同一 pet 同一 slot 100 个并发 equip 不同实例 → 只 1 个成功，其他 99 个返回错误（DB UNIQUE(pet_id, slot) 兜底）
- **并发 2**: 同一实例 100 个并发 equip 到不同 pet（理论不发生，因为 1 user 1 pet，但测一致性约束）→ 只 1 个成功（DB UNIQUE(user_cosmetic_item_id) 兜底）
- **边界 1**: equip 实例 status=consumed → 5003
- **边界 2**: equip 实例不属于当前用户 → 5002
- **边界 3**: unequip 不存在的 slot → 5004
- **状态一致性**: 任意操作后，所有 status=2 (equipped) 的实例必然在 user_pet_equips 中有对应行；反之亦然（NFR2 一致性约束）
**And** 全部场景用 dockertest 真实 MySQL 跑通
**And** 集成测试在 CI 标 `// +build integration` tag

### Story 26.6: GET /home 扩展 - pet.equips 真实数据（之前节点 2 返回 []）

As an iPhone 用户,
I want 主界面猫立即显示我已装备的装扮，不需要先打开仓库再回来,
So that 启动 App 看到的猫和我离开 App 时一致.

**Acceptance Criteria:**

**Given** Story 4.8 GET /home 已可用 + Story 26.3 装备关系已就绪
**When** 完成本 story
**Then** 修改 GET /home 实装:
- pet.equips 字段从写死 `[]` 改为读 user_pet_equips JOIN cosmetic_items + user_cosmetic_items
- 返回 `[{slot, userCosmeticItemId, cosmeticItemId, name, rarity, assetUrl}]`
- 节点 10 后由 Story 29.6 进一步追加 renderConfig 字段
**And** 不破坏 Story 4.8 + Story 11.10 既有 schema（仅填充 pet.equips 数组）
**And** **单元测试覆盖**（≥4 case，mocked repo）:
- happy: 用户穿了 hat + gloves → pet.equips 长度 = 2，含正确字段
- happy: 用户没穿任何装备 → pet.equips = []
- happy: 装备的 cosmetic_item 配置缺失（理论不该）→ skip + log warning
- edge: 大量装备并发查 → 单 SQL JOIN 不退化（不出现 N+1）
**And** **集成测试覆盖**（dockertest）: 创建 user + 装 2 件装备 → curl /home → pet.equips 含 2 项 + 字段值正确

## Epic 27: iOS - 穿戴交互 + 文字降级渲染

节点 9 iOS 端：激活仓库 Story 24.5 预留按钮 + EquipUseCase / UnequipUseCase + 文字降级 UI（FR45）+ 同槽换装动效 + 主界面实时刷新。**节点 9 阶段所有 cosmetic 都走文字降级**（node 10 后变成图像渲染）。

### Story 27.1: 激活 Story 24.5 预留的"穿戴"按钮（实装 onTap）

As an iPhone 用户,
I want 仓库页 instance 旁的"穿戴"按钮真正可以点击穿戴,
So that 我可以装备道具.

**Acceptance Criteria:**

**Given** Story 24.5 已预留按钮位置（disabled + 文案"穿戴（节点 9）"）
**When** 完成本 story
**Then** 按钮变为 enabled + 文案 "穿戴" + 触发 EquipUseCase（Story 27.2 提供）
**And** 已装备的实例（status=equipped）按钮文案变 "卸下" + 触发 UnequipUseCase
**And** 操作中按钮 disabled + 显示 loading
**And** 操作完成后刷新 inventory（重新调 GET /cosmetics/inventory）
**And** **单元测试覆盖**（≥4 case）:
- happy: 实例 status=in_bag → 按钮 "穿戴" enabled + 点击触发 EquipUseCase
- happy: 实例 status=equipped → 按钮 "卸下" enabled + 点击触发 UnequipUseCase
- happy: 操作中按钮 disabled + 显示 loading
- happy: 操作完成 → 触发 inventory 刷新

### Story 27.2: EquipUseCase / UnequipUseCase + 调 server 接口

As an iOS 开发,
I want 一对 EquipUseCase / UnequipUseCase 封装穿戴 / 卸下流程,
So that ViewModel 不直接处理 API.

**Acceptance Criteria:**

**Given** Story 26.3 / 26.4 server 接口可用 + Story 27.1 按钮触发
**When** 完成本 story
**Then** 实装两个 UseCase:
- `EquipUseCase(petId, userCosmeticItemId)`: POST /cosmetics/equip → 成功更新 ViewModel.equippedItems → 失败 ErrorPresenter
- `UnequipUseCase(petId, slot)`: POST /cosmetics/unequip → 成功更新 → 失败 ErrorPresenter
**And** 失败错误码映射:
- 5001 → "道具已不存在"
- 5002 → "道具不属于你"
- 5003 → "道具状态不可装备"
- 5004 → "该槽位无装备可卸下"
**And** **单元测试覆盖**（≥5 case，mocked APIClient）:
- happy: equip 成功 → 返回 equipped data
- happy: unequip 成功 → 返回 unequipped=true
- edge: 5001 → ErrorPresenter 显示 "道具已不存在"
- edge: 5003 → "道具状态不可装备"
- edge: network error → ErrorPresenter RetryView

### Story 27.3: 装扮文字降级 UI 组件（FR45）

As an iPhone 用户,
I want 当装扮没有图片资源时，App 用文字（道具名 + 槽位）替代图标显示在猫上,
So that 我能验证穿戴功能可用，即使节点 10 渲染配置还没就绪.

**Acceptance Criteria:**

**Given** Story 8.4 PetSpriteView 已有猫展示区 + 节点 10 render_config 还没就绪
**When** 完成本 story
**Then** 实装 `EquippedCosmeticView(slot, cosmeticName, assetUrl?, renderConfig?)` SwiftUI 组件:
- 优先用 assetUrl + renderConfig 渲染图像（节点 10 后的真实路径）
- **assetUrl 为空 / renderConfig 为空 / 图像加载失败** → 文字降级渲染:
  - 在该槽位锚点位置显示一个小标签：`[hat] 小黄帽`
  - 不同槽位用不同颜色边框（hat=黄 / gloves=蓝 / glasses=紫 等）
- 节点 9 阶段所有 cosmetic 都走文字降级（因为 render_config 字段还没加 + asset_url 是 placeholder URL，节点 10 才配置实际值）
**And** 主界面 PetSpriteView 多个槽位的 EquippedCosmeticView 叠在猫上方
**And** **单元测试覆盖**（≥4 case，UI snapshot）:
- happy: assetUrl + renderConfig 都有 → 渲染图像
- happy: assetUrl 为空 → 文字降级 "[hat] 小黄帽"
- happy: 图像加载失败 → 文字降级
- happy: 不同 slot → 不同边框颜色

### Story 27.4: 同槽位换装动效

As an iPhone 用户,
I want 同一槽位换装时有平滑过渡动画（旧装备淡出 + 新装备淡入）,
So that 视觉体验不突兀.

**Acceptance Criteria:**

**Given** Story 27.3 EquippedCosmeticView 已可渲染 + Story 26.3 server 同槽换装事务
**When** 用户对同一 slot 装新装备
**Then** EquipUseCase 成功后:
- 旧装备 EquippedCosmeticView 淡出（200ms）
- 新装备 EquippedCosmeticView 淡入（200ms）
- 整体过渡 400ms
**And** 卸下动效：装备淡出 200ms → 消失
**And** **单元测试覆盖**（≥3 case）:
- happy: 同 slot 换装 → 旧 fade out + 新 fade in
- happy: 卸下 → fade out
- happy: 不同 slot 装备 → 各自独立动效

### Story 27.5: 主界面猫显示区按当前装备实时刷新

As an iPhone 用户,
I want 我穿戴 / 卸下后，主界面的猫立即显示新装备,
So that 视觉反馈即时.

**Acceptance Criteria:**

**Given** Story 27.2-27.4 已就绪 + Story 8.4 主界面猫展示区
**When** EquipUseCase / UnequipUseCase 成功
**Then** ViewModel.equippedItems 更新 → 主界面 PetSpriteView 自动 SwiftUI 刷新（含全部槽位 EquippedCosmeticView 叠加）
**And** App 启动时通过 GET /cosmetics/inventory（含 equipped 状态实例）+ GET /home（如有的话，含 pet 装备信息）初始化 equippedItems
**And** 房间页内自己的猫位也按相同逻辑展示装备（房间页的 pet snapshot.equips 字段提供数据）
**And** **单元测试覆盖**（≥4 case，mocked ViewModel）:
- happy: 启动时从 server 加载 equippedItems → 主界面渲染
- happy: 穿戴成功 → ViewModel.equippedItems 更新 → 主界面立即刷新
- happy: 卸下成功 → ViewModel.equippedItems 删除 → 主界面立即刷新
- happy: 房间页内自己的猫位也显示装备
**And** **UI 测试覆盖**（XCUITest）:
- 模拟器开箱获得 hat → 仓库点穿戴 → 关闭仓库 Sheet → 主界面猫上方有 "[hat] 小黄帽" 文字标签

## Epic 28: 对齐 - 节点 9 穿戴流程联调（多槽位换装、状态一致性、缺图文字降级可验）

把 Epic 26 (Server 穿戴事务) + Epic 27 (iOS 穿戴交互 + 文字降级) 真实串联，验证节点 9 §4.9 全部验收标准达成。**重点**：FR45 文字降级保证节点 9 不依赖节点 10 即可独立验收。

### Story 28.1: 跨端集成测试场景 - 穿戴流程 E2E

**Acceptance Criteria:**

**Given** Epic 26 + Epic 27 全部完成
**When** 完成本 story
**Then** 输出文档 `_bmad-output/implementation-artifacts/e2e/node-9-equip-e2e.md`，含:
- 准备步骤: server 启动 + 用户登录 + 用 dev 接口给用户 grant 5 件不同 cosmetic（覆盖 hat/gloves/neck/back 4 个槽位 + 1 件备用 hat）
- 验证场景 1（基本穿戴）: 仓库选 hat A → 点穿戴 → DB user_pet_equips 1 行 + hat A status=2 + 主界面猫上方 "[hat] hat A 名" 文字标签出现
- 验证场景 2（多槽位）: 依次穿戴 gloves / neck / back → 主界面猫上方 4 个文字标签（不同颜色边框，按槽位）
- 验证场景 3（同槽位换装）: 穿 hat A 后再穿 hat B → DB user_pet_equips hat 行更新（不是新增）+ hat A status 回 1 + hat B status=2 + 主界面 hat 标签从 "hat A" 平滑换到 "hat B"（动效 400ms）
- 验证场景 4（卸下）: 仓库点 hat B 的"卸下"按钮 → DB user_pet_equips hat 行删除 + hat B status=1 + 主界面 hat 标签淡出消失
- 验证场景 5（错误处理）: 调 unequip 一个无装备的槽位 → 5004 alert
- 验证场景 6（房间内自己的猫）: 进入房间 → 自己的猫位也显示装备文字标签（依赖 snapshot.equips 字段，节点 9 阶段如 server snapshot 仍返回空 equips 则**登记 tech debt**——本验收不阻塞，节点 10/11 后补完整）
- 验证场景 7（DB 一致性）: 任意操作后 mysql 查 → user_pet_equips 与 user_cosmetic_items.status 双向一致
**And** 文档含截图位

### Story 28.2: 节点 9 demo 验收

**Acceptance Criteria:**

**Given** Story 28.1 E2E 文档已就绪
**When** 完成本 story
**Then** 按 §4.9 节点 9 验收标准逐项打勾:
- [ ] 玩家可对自己拥有的实例穿戴 / 卸下，状态以 server 为准
- [ ] 同槽位换装时新旧实例状态正确切换
- [ ] 缺图实例可在猫身上以文字标签展示，不阻断穿戴流程（FR45 验证）
- [ ] UNIQUE(pet_id, slot) 与 UNIQUE(user_cosmetic_item_id) 约束在事务中维护
- [ ] **新增**: 房间内自己的猫位展示装备（如 snapshot.equips 仍空，登记节点 10 / 11 补）
**And** 输出 demo 录屏到 `_bmad-output/implementation-artifacts/demo/node-9-demo-<date>/`
**And** 任何未通过项必须登记原因 + 是否后置

### Story 28.3: 文档同步与 tech debt 登记

**Acceptance Criteria:**

**Given** Epic 26-27 完成 + Story 28.2 demo 通过
**When** 完成本 story
**Then** 检查并更新（无变更也要明确标注）:
- [ ] `docs/宠物互动App_V1接口设计.md` §8.3 / §8.4: 实装与 Story 26.1 锚定的 schema 一致
- [ ] `docs/宠物互动App_数据库设计.md` §5.10 (user_pet_equips): 表结构与文档一致 + 双 UNIQUE 约束
- [ ] `docs/宠物互动App_时序图与核心业务流程设计.md` §9 (穿戴流程): 时序与实装一致
- [ ] `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §6.6 (Cosmetics 模块): 文字降级 UI 设计
**And** `_bmad-output/implementation-artifacts/tech-debt-log.md` 追加节点 9 tech debt，可能的:
- snapshot.equips 字段在节点 11 (server 房间业务) 内未填充真实装备数据，节点 10 (Story 29.x) 或 11+ 才补
- 文字降级 UI 标签字体 / 颜色 / 大小 hard-code，未抽到设计系统
- 节点 9 阶段所有 cosmetic 走文字降级，节点 10 后才有图像渲染（这是设计选择，不是 bug）

## Epic 29: Server - cosmetic_items.render_config 字段 + seed + 接口下发 + 数据库设计文档同步

节点 10 Server 端：cosmetic_items 加 render_config JSON 列（A 方案）+ seed 给已有 15+ cosmetic 都补 render_config + catalog/inventory 接口返回该字段 + 同步更新数据库设计文档。

### Story 29.1: 接口契约最终化（render_config 字段 schema）

As a 跨端契约负责人,
I want 把 render_config JSON 字段在 catalog / inventory response 中的 schema 锚定到 V1接口设计文档,
So that iOS Epic 30 可以基于稳定契约并行开工.

**Acceptance Criteria:**

**Given** docs/宠物互动App_V1接口设计.md §8.1 / §8.2 已锚定 catalog / inventory 基础 schema
**When** 完成本 story
**Then** §8.1 / §8.2 response items / groups 中各 cosmetic 元数据**追加 `renderConfig` 字段**:
- 类型: object | null
- 字段: `{offsetX: float, offsetY: float, scale: float, rotation: float, zLayer: int}`
- 字段含义: 相对槽位锚点的 X/Y 偏移（pixels）/ 缩放比例（1.0 = 原始大小）/ 旋转角度（度）/ Z 轴层级
- null 时客户端走文字降级（FR45）
**And** 标注"renderConfig schema 留扩展位 per-state 偏移（future），MVP 仅支持基础参数"
**And** 标注"契约自此 Epic 起冻结"

### Story 29.2: cosmetic_items 加 render_config 列 migration（A 方案）

As a 服务端开发,
I want cosmetic_items 表新增 render_config JSON 列,
So that 装扮渲染参数可以持久化（A 方案：直接加列，不新表）.

**Acceptance Criteria:**

**Given** Story 20.2 cosmetic_items 表已存在
**When** 完成本 story
**Then** `migrations/0016_alter_cosmetic_items_add_render_config.sql`:
- `ALTER TABLE cosmetic_items ADD COLUMN render_config JSON NULL DEFAULT NULL`
- 含 down.sql: `ALTER TABLE cosmetic_items DROP COLUMN render_config`
**And** 现有 cosmetic_items 行 render_config 默认 NULL（向后兼容，节点 9 文字降级仍生效）
**And** **单元测试覆盖**（≥3 case）:
- happy: migrate up 后 cosmetic_items 表多 render_config 列（type=JSON, NULL allowed）
- happy: migrate down 后列被删除
- edge: 已有数据行 render_config 为 NULL（不报错）
**And** **集成测试覆盖**（dockertest）: migrate up → INSERT 一行带 render_config JSON → SELECT 返回正确解析

### Story 29.3: render_config seed（给已有 cosmetic 配置都补 render_config）

As a 服务端开发,
I want Story 20.3 seed 的 15+ 个 cosmetic 全部补上合理的 render_config 数据,
So that 节点 10 demo 时所有装扮都能正常视觉渲染（不出现部分文字 / 部分图像的混乱状态）.

**Acceptance Criteria:**

**Given** Story 29.2 render_config 列已存在 + Story 20.3 cosmetic 已 seed 15+ 条
**When** 完成本 story
**Then** `migrations/0017_seed_cosmetic_render_config.sql`:
- 对所有 enabled cosmetic UPDATE render_config 字段
- 同槽位（如所有 hat）使用相似的 offsetX/offsetY/scale 基础值，但每件略微不同（offsetX 微调 ±5px / scale 微调 ±0.1）以体现可分辨差异
- 槽位锚点对应：hat → 头顶 / gloves → 爪子 / glasses → 眼睛 / neck → 脖子 / back → 背
- zLayer 按槽位约定：body=0 / 衣服 / hat=10 / gloves=20 等（避免视觉穿模）
**And** seed 通过 migration 写入（INSERT ... ON DUPLICATE KEY UPDATE 或先 SELECT 再 UPDATE 各行）
**And** **单元测试覆盖**（≥3 case）:
- happy: migrate up 后所有 enabled cosmetic 的 render_config 都非 NULL
- happy: 同槽位（如 hat）多件的 offsetX 有可分辨差异（不全相同）
- happy: 重复 migrate up → 不报错（幂等）
**And** **集成测试覆盖**（dockertest）: migrate up → SELECT cosmetic_items WHERE render_config IS NULL → 0 行（全部已配置）

### Story 29.4: catalog / inventory 接口返回 render_config

As a 服务端开发,
I want GET /cosmetics/catalog 和 GET /cosmetics/inventory 接口在 response 中**包含** render_config 字段,
So that iOS Epic 30 可以拿到该字段用于真实图像渲染.

**Acceptance Criteria:**

**Given** Story 29.2 / 29.3 已就绪 + Story 23.3 / 23.4 接口已实装
**When** 调用 GET /cosmetics/catalog 或 GET /cosmetics/inventory
**Then** response 中各 cosmetic 字段**追加 renderConfig**:
- 类型: object（含 offsetX, offsetY, scale, rotation, zLayer）或 null
- DB 中 render_config 字段值反序列化到 response
**And** 旧客户端（节点 9 阶段，不识别此字段）**忽略该字段不报错**（向后兼容）
**And** **不破坏 Story 23.3 / 23.4 既有 schema**（其他字段全部不变，只是追加）
**And** **单元测试覆盖**（≥4 case，扩展 Story 23.3 / 23.4 单测）:
- happy: catalog 返回 cosmetic 含 renderConfig 字段（值与 DB 一致）
- happy: inventory 返回 group 中 cosmetic 也含 renderConfig
- edge: render_config DB 中是 NULL → response 字段为 null（不抛 error）
- edge: render_config DB 中 JSON 不合法（理论不该有）→ log error + response 字段返回 null
**And** **集成测试覆盖**（dockertest）: seed cosmetic + render_config → curl GET /cosmetics/catalog → 验证 response items 含 renderConfig

### Story 29.5: 同步更新 docs/宠物互动App_数据库设计.md §5.8 增加 render_config 字段说明

As a 文档维护者,
I want 数据库设计文档反映 cosmetic_items 表的最新 schema（含 render_config）,
So that 文档作为单一事实来源仍然可信.

**Acceptance Criteria:**

**Given** Story 29.2 已加列
**When** 完成本 story
**Then** 修改 `docs/宠物互动App_数据库设计.md` §5.8:
- 在 cosmetic_items DDL 中追加 `render_config JSON NULL DEFAULT NULL`
- 在"字段说明"列出 `render_config`：JSON 字段，含 offsetX/offsetY/scale/rotation/zLayer，控制装扮在猫身上的渲染参数；NULL 时客户端走文字降级
- 在"设计说明"补一段：节点 10 引入此字段（A 方案：直接加列，不新表），未来若需要 per-state 偏移可在 JSON 内扩展或迁移到独立表
**And** Git commit 单独提交此文档更新（便于追溯）
**And** **不需要单元测试**（纯文档更新）—— 但**手动验证**：把更新后的 §5.8 与本 Epic 实装的 migration 0016/0017 对照，确保字段 / 类型 / 约束完全一致

### Story 29.6: GET /home 扩展 - pet.equips[].renderConfig 字段下发

As an iPhone 用户,
I want 主界面猫一启动就能正确渲染装扮（含 render_config 决定的位置 / 大小 / 朝向）,
So that 不出现"主界面文字降级、仓库进出后才变图像"的违和感.

**Acceptance Criteria:**

**Given** Story 26.6 /home 已含 pet.equips + Story 29.4 catalog/inventory 已含 renderConfig
**When** 完成本 story
**Then** 修改 GET /home 实装:
- pet.equips[].renderConfig 字段下发（与 Story 29.4 catalog/inventory 同样格式）
- 数据来自 cosmetic_items.render_config JSON 字段
- 缺失时返回 null（client 走文字降级）
**And** 不破坏 Story 4.8 + Story 11.10 + Story 26.6 既有 schema
**And** **单元测试覆盖**（≥3 case，扩展 Story 26.6 单测）:
- happy: 装备的 cosmetic 含 render_config → /home pet.equips[i].renderConfig 非 null
- happy: 装备的 cosmetic render_config NULL → /home pet.equips[i].renderConfig = null
- edge: 多件装备各自独立 render_config → 各自正确字段
**And** **集成测试覆盖**（dockertest）: 装 2 件含 render_config 的装备 → curl /home → pet.equips 各项含正确 renderConfig

## Epic 30: iOS - 装扮渲染层（按 render_config 在猫身上正确显示装扮图像）

节点 10 iOS 端：RenderConfig 数据模型 + SpriteRenderer 封装 + 升级 EquippedCosmeticView（图像优先 + 文字降级 fallback）+ 槽位锚点常量化。

### Story 30.1: RenderConfig 数据模型 + Codable 解析

As an iOS 开发,
I want Swift 端的 RenderConfig 数据模型,
So that catalog / inventory response 中的 renderConfig JSON 可以无缝解析.

**Acceptance Criteria:**

**Given** Story 29.4 server 返回 renderConfig 字段
**When** 完成本 story
**Then** 定义 `struct RenderConfig: Codable, Equatable`:
- `offsetX: Float`, `offsetY: Float`, `scale: Float`, `rotation: Float`, `zLayer: Int`
**And** Cosmetic 模型扩展含 `renderConfig: RenderConfig?`（可选，对应 server null）
**And** **单元测试覆盖**（≥4 case）:
- happy: 标准 JSON `{"offsetX":5,"offsetY":-10,"scale":1.2,"rotation":15,"zLayer":10}` 解析正确
- happy: JSON null → renderConfig = nil
- edge: 缺字段 JSON `{"offsetX":5}` → 抛 DecodingError（严格模式）
- edge: 字段类型错误（offsetX 是 string）→ 抛 DecodingError

### Story 30.2: SpriteRenderer 封装（按 renderConfig 定位 / 缩放 / 旋转 / Z 层）

As an iOS 开发,
I want 一个 SwiftUI 组件 SpriteRenderer 接收 RenderConfig + assetUrl 返回正确变换的图像,
So that 业务层不直接操作 SwiftUI transform / overlay.

**Acceptance Criteria:**

**Given** Story 30.1 RenderConfig 模型已就绪
**When** 完成本 story
**Then** 定义 `struct SpriteRenderer: View` SwiftUI 组件，接收 `(assetUrl: URL, config: RenderConfig, slotAnchor: CGPoint)`
**And** 渲染逻辑:
- AsyncImage 加载 assetUrl
- 应用 `.offset(x: config.offsetX, y: config.offsetY)` 相对 slotAnchor
- 应用 `.scaleEffect(config.scale)`
- 应用 `.rotationEffect(.degrees(config.rotation))`
- 用 ZStack `.zIndex(config.zLayer)` 排序
**And** 图像加载失败 → 触发 fallback 回调（由 Story 30.3 EquippedCosmeticView 处理走文字降级）
**And** **单元测试覆盖**（≥4 case，UI snapshot）:
- happy: config 给定正常值 → 视图变换符合预期
- happy: scale=2.0 → 图像放大一倍
- happy: rotation=90 → 图像旋转 90 度
- edge: 多个 SpriteRenderer 不同 zLayer → 排序正确

### Story 30.3: 升级 EquippedCosmeticView - renderConfig 完整时走图像，缺失时退回文字降级

As an iPhone 用户,
I want 当装扮有 render_config 时看到漂亮的图像，没有时仍能看到文字标签,
So that 节点 9 文字降级 → 节点 10 图像渲染的过渡平滑.

**Acceptance Criteria:**

**Given** Story 27.3 EquippedCosmeticView 已实装文字降级 + Story 30.2 SpriteRenderer 已可用
**When** 完成本 story
**Then** 升级 EquippedCosmeticView 决策逻辑:
- assetUrl 非空 + renderConfig 非 nil → 走 SpriteRenderer 图像渲染
- assetUrl 为空 OR renderConfig 为 nil OR 图像加载失败 → 走文字降级（保留 Story 27.3 实现）
**And** 节点 10 完成后 demo 时所有装扮应该都走图像路径（因为 Story 29.3 给所有 cosmetic 补了 render_config）
**And** **单元测试覆盖**（≥5 case，UI snapshot）:
- happy: assetUrl + renderConfig 都有 → 走 SpriteRenderer
- happy: renderConfig 为 nil → 走文字降级
- happy: assetUrl 为空字符串 → 走文字降级
- edge: SpriteRenderer 图像加载失败（mocked AsyncImage error）→ 自动降级到文字
- happy: 节点 9 阶段保留的所有文字降级测试仍通过

### Story 30.4: 槽位锚点常量化

As an iOS 开发,
I want 一个常量字典 SlotAnchorRegistry 定义每个 slot 在猫 sprite 上的锚点位置,
So that SpriteRenderer 知道"hat 渲染在头顶 / gloves 在爪子" 等锚点.

**Acceptance Criteria:**

**Given** Story 30.2 SpriteRenderer 接收 slotAnchor 参数
**When** 完成本 story
**Then** 定义 `enum SlotAnchorRegistry`:
- `static func anchor(for slot: Int) -> CGPoint`
- 返回每个 slot 对应猫 sprite 上的锚点（基于猫身体相对坐标）:
  - slot=1 hat → 头顶
  - slot=2 gloves → 双爪
  - slot=3 glasses → 眼睛
  - slot=4 neck → 脖子
  - slot=5 back → 背
  - slot=6 body → 躯干中心
  - slot=7 tail → 尾巴
  - slot=99 other → 默认中心
**And** 锚点用相对坐标（如 `(0, -50)` = 猫中心向上 50 px）
**And** EquippedCosmeticView 调用 `SlotAnchorRegistry.anchor(for: slot)` 拿锚点传给 SpriteRenderer
**And** **单元测试覆盖**（≥3 case）:
- happy: anchor(for: 1) → 头顶坐标（如 (0, -50)）
- happy: anchor(for: 99) → 默认中心
- edge: 未定义 slot（如 100）→ 默认中心 + log warning

## Epic 31: 对齐 - 节点 10 装扮渲染验收（视觉差异明显、状态切换不错位、配置缺失时仍能文字降级）

把 Epic 29 (Server render_config + seed + 接口) + Epic 30 (iOS 渲染层) 真实串联，验证节点 10 §4.10 全部验收标准达成。**核心**：从节点 9 的文字降级升级到正式图像渲染，且 fallback 仍然可用。

### Story 31.1: 跨端集成测试场景 - 装扮渲染 E2E

**Acceptance Criteria:**

**Given** Epic 29 + Epic 30 全部完成
**When** 完成本 story
**Then** 输出文档 `_bmad-output/implementation-artifacts/e2e/node-10-render-e2e.md`，含:
- 准备步骤: server 启动 + render_config seed 已就绪 + 用户 grant 多件不同 cosmetic（不同 slot + 同 slot 多件）
- 验证场景 1（图像渲染）: 仓库穿戴 hat A → 主界面猫上方头顶位置出现 hat A 的**图像**（不再是文字降级）+ 视觉位置 / 大小 / 旋转符合 render_config
- 验证场景 2（多槽位渲染）: 依次穿戴 hat / gloves / neck / back → 主界面猫上叠加 4 个图像，各槽位锚点正确（hat 在头 / gloves 在爪子 / neck 在脖子 / back 在背）
- 验证场景 3（同槽位差异）: 穿 hat A → 卸下 → 穿 hat B → 视觉上**能看出 hat A 与 hat B 不同位置 / 大小 / 朝向**（验证 Story 29.3 同槽位略微差异生效）
- 验证场景 4（zLayer 排序）: 同时穿 body 和 hat → hat 在 body 上面（zLayer 高）
- 验证场景 5（猫动作切换不错位）: 走两步 → 猫 sprite 切到 walk → 装扮位置应**仍在合理位置**（MVP 接受 sprite 锚点固定，不强制 per-state 偏移）
- 验证场景 6（render_config 缺失降级）: 用 mysql 手动 UPDATE 一件 cosmetic 的 render_config = NULL → 重穿戴该件 → 主界面对应槽位**自动退回文字降级**
- 验证场景 7（图像加载失败降级）: 用 mysql 手动改一件 cosmetic 的 asset_url 为不可访问 URL → 重穿戴 → AsyncImage 加载失败 → 自动退回文字降级
- 验证场景 8（数据库设计文档已同步）: 查 `docs/宠物互动App_数据库设计.md` §5.8 → 确认含 render_config 字段说明
**And** 文档含截图位（特别是验证场景 1-3 的视觉效果）

### Story 31.2: 节点 10 demo 验收

**Acceptance Criteria:**

**Given** Story 31.1 E2E 文档已就绪
**When** 完成本 story
**Then** 按 §4.10 节点 10 验收标准逐项打勾:
- [ ] 同一槽位的不同装扮在猫身上可视化差异明显
- [ ] render_config 缺失时客户端仍能降级为文字（节点 9 行为不被破坏）
- [ ] 猫动作切换（rest/walk/run）时装扮位置不错位
- [ ] 通过修改 server 端 seed 即可调整任意装扮的渲染参数，无需 App 重发版
- [ ] **新增**: 数据库设计文档已同步（Story 29.5 验证）
**And** 输出 demo 录屏到 `_bmad-output/implementation-artifacts/demo/node-10-demo-<date>/`（重点录"穿戴前后视觉变化"）
**And** 任何未通过项必须登记原因 + 是否后置

### Story 31.3: 文档同步与 tech debt 登记

**Acceptance Criteria:**

**Given** Epic 29-30 完成 + Story 31.2 demo 通过
**When** 完成本 story
**Then** 检查并更新（无变更也要明确标注）:
- [ ] `docs/宠物互动App_V1接口设计.md` §8.1 / §8.2: renderConfig 字段已加入（Story 29.1 验证）
- [ ] `docs/宠物互动App_数据库设计.md` §5.8: cosmetic_items 含 render_config 字段（Story 29.5 验证）
- [ ] `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §6.6 (Cosmetics 模块): SpriteRenderer + SlotAnchorRegistry 设计补充
**And** `_bmad-output/implementation-artifacts/tech-debt-log.md` 追加节点 10 tech debt，可能的:
- per-state（rest/walk/run）偏移变体未实装，节点 10 接受装扮位置在猫动作切换时"差不多"即可，未来需要更精细可加 RenderConfig 嵌套字段
- render_config 通过 migration / seed 维护，无 admin 接口，美术每次调整需写 migration 重新部署
- SlotAnchorRegistry 锚点坐标 hard-code，未对接设计系统

## Epic 32: Server - 合成事务 + idempotencyKey + 双日志

节点 11 Server 端：合成事务（锁 10 实例 + 校验 + consumed + 加权产出 + 双日志）+ overview 接口 + Layer 2 集成测试 Story 32.5。

### Story 32.1: 接口契约最终化（GET /compose/overview + POST /compose/upgrade）

As a 跨端契约负责人,
I want 把合成接口（含 reward 结构）schema 锚定到 V1接口设计文档,
So that iOS Epic 33 可以基于稳定契约并行开工.

**Acceptance Criteria:**

**Given** docs/宠物互动App_V1接口设计.md §9.1 / §9.2 已有初稿
**When** 完成本 story
**Then** §9.1 (GET /compose/overview) schema 完全锚定:
- response data: `{rarities: [{rarity: int, availableCount: int, canCompose: bool}]}`
- canCompose: availableCount >= 10 AND rarity != 4 (legendary)
**And** §9.2 (POST /compose/upgrade) schema 完全锚定:
- request: `{fromRarity: int, userCosmeticItemIds: string[], idempotencyKey: string}`
- response data: `{fromRarity, toRarity, consumedItemIds, reward: {userCosmeticItemId, cosmeticItemId, name, slot, rarity, assetUrl}}`
- 错误码: 5005 合成材料数量错误 / 5006 合成材料品质不一致 / 5007 合成目标品质不合法 / 5002 道具不属于当前用户 / 5003 道具状态不可用 / 1008 幂等冲突
**And** 标注"契约自此 Epic 起冻结"

### Story 32.2: compose_logs + compose_log_materials migration

As a 服务端开发,
I want 合成日志相关两张表的 migration,
So that 合成行为有审计记录（含具体消耗的 10 个实例）.

**Acceptance Criteria:**

**Given** Epic 4 migrations 框架已就绪
**When** 完成本 story
**Then** `migrations/0018_init_compose_logs.sql` 按 数据库设计.md §5.11 创建 compose_logs:
- 字段: id, user_id, from_rarity, to_rarity, consumed_count, reward_user_cosmetic_item_id, reward_cosmetic_item_id, created_at
- KEY idx_user_id_created_at
**And** `migrations/0019_init_compose_log_materials.sql` 按 §5.12 创建 compose_log_materials:
- 字段: id, compose_log_id, user_cosmetic_item_id, cosmetic_item_id, created_at
- KEY idx_compose_log_id + KEY idx_user_cosmetic_item_id
**And** 含 down.sql
**And** **单元测试覆盖**（≥3 case）:
- happy: migrate up 后两张表存在 + 字段类型 + 索引都符合 §5.11 / §5.12
- happy: migrate down 后两张表删除
- edge: 重复 migrate up → 幂等
**And** **集成测试覆盖**（dockertest）: migrate up → SHOW CREATE TABLE 对比 schema → migrate down

### Story 32.3: GET /compose/overview 接口（按品质统计可合成数量）

As an iPhone 用户,
I want 查询我每个品质有多少件可用于合成的实例,
So that 合成页可以显示"common 24 件，可合成"等概览.

**Acceptance Criteria:**

**Given** Story 23.2 user_cosmetic_items 已就绪
**When** 调用 `GET /compose/overview`
**Then** service 流程:
- 查 user_cosmetic_items WHERE user_id=? AND status=1 (in_bag) GROUP BY cosmetic_item_id 关联 cosmetic_items.rarity
- 按 rarity 聚合 count
- 返回 4 个 rarity 段（含 0 件的）
- canCompose = (count >= 10) AND (rarity != 4)
**And** 接口要求 auth
**And** **单元测试覆盖**（≥4 case，mocked repo）:
- happy: 用户有 24 common + 8 rare + 12 epic + 2 legendary → response 4 段 count 正确，canCompose: 1=true, 2=false, 3=true, 4=false
- happy: 用户某品质 0 件 → 该段 count=0, canCompose=false
- edge: rarity=4 (legendary) 即使有 100 件也 canCompose=false
- edge: 用户 0 件道具 → 4 段 count 都 = 0, 全 false
**And** **集成测试覆盖**（dockertest）: 创建 user + grant 各品质若干件 → curl → 验证 rarity 段数据准确

### Story 32.4: POST /compose/upgrade 事务 + idempotencyKey + 加权产出

As an iPhone 用户,
I want 我手动选 10 个同品质实例提交合成，能稳定拿到 1 个高一阶品质随机实例,
So that 我可以体验合成升级玩法.

**Acceptance Criteria:**

**Given** Story 32.2 表已就绪 + Story 23.2 user_cosmetic_items 可读写 + Story 20.3 cosmetic_items 已 seed
**When** 调用 `POST /compose/upgrade` 带 fromRarity + 10 个 userCosmeticItemIds + idempotencyKey
**Then** service 流程:
- 检查 Redis 幂等键 → 已有结果直接返回
- 在事务中:
  - 锁定 10 个实例（FOR UPDATE）
  - 校验数量 = 10 → 否则 5005
  - 校验无重复 id → 否则 5005
  - 校验全部属于当前用户 → 否则 5002
  - 校验全部 status=1 (in_bag) → 否则 5003
  - 校验全部对应 cosmetic_items.rarity == fromRarity → 否则 5006
  - 校验 fromRarity 可升级（1/2/3）→ legendary 4 → 5007
  - 标记 10 个实例 status=3 (consumed) + consumed_at=now
  - 在 toRarity (=fromRarity+1) 的 cosmetic_items 池中按 drop_weight 加权随机抽 1 个 cosmetic_item_id
  - 创建 user_cosmetic_items: (user_id, cosmetic_item_id, status=1, source=2 compose, source_ref_id=待会的 compose_log_id)
  - 写 compose_logs（拿到 log_id）
  - 写 10 条 compose_log_materials
  - 回填 user_cosmetic_items.source_ref_id = log_id（需要事务内 update）
- Redis 写幂等结果（TTL 24h）
- 返回 reward + consumedItemIds
**And** 接口要求 auth + rate_limit
**And** **单元测试覆盖**（≥7 case，mocked repo + Redis）:
- happy: 10 件 common 合成 → 产出 1 件 rare，10 件 status=3
- edge: 数量 9 → 5005
- edge: 数量 11 → 5005
- edge: 含重复 id → 5005
- edge: 1 件不属于当前用户 → 5002
- edge: 1 件 status=equipped → 5003
- edge: 1 件 rarity 不一致 → 5006
- edge: fromRarity=4 (legendary) → 5007
**And** **集成测试覆盖**（dockertest + Redis）: grant 10 件 common → upgrade → DB user_cosmetic_items 新 1 件 rare + 旧 10 件 consumed + compose_logs 1 行 + compose_log_materials 10 行

### Story 32.5: Layer 2 集成测试 - 合成事务全流程

As a 资产事务负责人,
I want 一组深度集成测试覆盖合成事务的失败回滚 / 并发 / 边界 / 幂等,
So that NFR1 / NFR5 / §8.5 (合成事务) 有自动化保障.

**Acceptance Criteria:**

**Given** Story 32.4 happy path 已通过
**When** 完成本 story
**Then** 输出 `internal/service/compose_service_integration_test.go` 用 dockertest（MySQL + Redis）覆盖:
- **完整流程**: 用户 12 件 common → upgrade 10 件 → 验证 10 件 status=3 + 余 2 件 status=1 + 新增 1 件 rare + 双日志正确
- **回滚 1**: 锁定后 mock 抽奖步骤抛 error → 验证 10 件实例 status 回 1（未 consumed）+ 无新实例 + 无双日志
- **回滚 2**: 标记 consumed 成功，mock 写 compose_logs 抛 error → 验证 10 件回 status=1 + 新实例未创建
- **回滚 3**: mock 写 compose_log_materials 抛 error → 整体回滚（10 件回 1 + 新实例 + log 都不存在）
- **幂等 1**: 同一 idempotencyKey 重复调 100 次 → 只成功 1 次，DB compose_logs 只多 1 行
- **并发 1**: 用户有 11 件 common，2 个并发 upgrade（不同 idempotencyKey，但用同样的 10 件实例 id）→ 只 1 个成功（DB FOR UPDATE 锁兜底，第二个看到实例已被 consumed → 5003）
- **并发 2**: 用户有 30 件 common，3 个并发 upgrade（各自用独立的 10 件 id）→ 全部成功，DB 30 件 consumed + 新 3 件 rare + 3 行 compose_logs + 30 行 compose_log_materials
- **边界 1**: fromRarity=3 (epic) + 10 件 epic → 产出 1 件 legendary
- **边界 2**: fromRarity=4 (legendary) + 10 件 legendary → 5007 拒绝
- **边界 3**: 用户拿 9 件 common + 1 件 rare → 5006 品质不一致
- **抽奖分布**: 1000 次合成 common → rare → toRarity 中各品质比例符合 drop_weight
- **source_ref_id 回填**: 合成产出的新实例 source=2 + source_ref_id 是 compose_log_id
**And** 全部场景用 dockertest 真实 MySQL + Redis 跑通
**And** 集成测试在 CI 标 `// +build integration` tag

## Epic 33: iOS - 合成页面 + 手动选材交互

节点 11 iOS 端：合成页骨架 + 按品质 tab + 实例 grid + 手动选 10 件 + 数量/品质校验 + 提交 + 成功 popup + 主界面入口完善。

### Story 33.1: 合成页骨架（按品质 tab + 实例 grid + 已选区）

As an iOS 开发,
I want 合成页骨架，含按品质 tab 切换 + 实例 grid 浏览 + 底部已选材料区,
So that Story 33.2 / 33.3 可以填充交互逻辑.

**Acceptance Criteria:**

**Given** Epic 2 Story 2.3 主界面"合成"按钮已开 Sheet placeholder
**When** 完成本 story
**Then** 实装 `ComposeView` SwiftUI 替换 placeholder Sheet:
- 顶部: 标题 "合成" + 关闭按钮
- 上方: 品质 segment control "common / rare / epic"（legendary 不展示，因为不能合成）
- 中间: 当前品质的实例 grid（LazyVGrid 4 列），每个 cell 含 cosmetic icon + name + 选中态 checkmark
- 底部: 已选区，显示 "已选 N / 10" + 缩略图列表 + "合成"按钮
- 实例数量不足 10 件时灰色提示 "需要 10 件 common，当前 N 件"
**And** 用 `ComposeViewModel` 持有 `selectedRarity, instances, selectedIds, isSubmitting`
**And** **单元测试覆盖**（≥4 case，UI snapshot）:
- happy: ViewModel 注入 mock 24 件 common → grid 渲染 24 个 cell + 底部 "已选 0 / 10"
- happy: 切换 rarity tab → 实例列表更新
- happy: 已选 5 件 → 底部缩略图显示 5 件
- edge: 实例不足 10 件 → "合成"按钮 disabled + 灰色提示

### Story 33.2: LoadComposeOverviewUseCase + GET /compose/overview 调用

As an iPhone 用户,
I want 进合成页时立即看到我各品质的可合成情况,
So that 我知道哪些品质可以合成.

**Acceptance Criteria:**

**Given** Story 32.3 server GET /compose/overview 可用 + Story 33.1 ComposeView 骨架
**When** ComposeView .task / .onAppear
**Then** 并行调用:
- `GET /compose/overview` → ViewModel.rarities (4 段统计)
- `GET /cosmetics/inventory` → ViewModel.instances (按 status=in_bag 过滤的实例)
**And** 切换 rarity tab 时不重新调 API（用本地 instances 按 rarity filter）
**And** 加载中显示 ProgressView
**And** 合成成功后**重新加载**两个接口（更新 overview + inventory）
**And** **单元测试覆盖**（≥4 case，mocked APIClient）:
- happy: 两个 API 都成功 → ViewModel 状态正确
- happy: tab 切换 → 不发新 API，按本地 filter
- happy: 合成成功 → 触发重新加载
- edge: API 失败 → RetryView

### Story 33.3: 手动选 10 实例交互 + 数量 / 品质校验

As an iPhone 用户,
I want 我可以点选 10 个实例作为合成材料，且选中的实例必须同品质,
So that 我能控制合成的具体材料.

**Acceptance Criteria:**

**Given** Story 33.1 / 33.2 已就绪
**When** 用户点击实例 cell
**Then** 切换选中态:
- 未选 → 选中：加入 ViewModel.selectedIds（如已 10 件则拒绝 + Toast "最多选 10 件"）
- 选中 → 取消：从 selectedIds 移除
**And** **品质过滤**：切换 rarity tab 时如已有选中实例（同品质），保留选中；如切到不同品质则**清空已选**（带确认 alert "切换品质会清空已选，确认？"）
**And** 已选数量动态更新底部 "已选 N / 10"
**And** N == 10 时"合成"按钮 enabled
**And** **单元测试覆盖**（≥6 case，mocked ViewModel）:
- happy: 点选 5 件 common → selectedIds 长度 = 5
- happy: 点选已选的实例 → 取消，selectedIds 减少
- edge: 已选 10 件再点新实例 → 拒绝 + Toast
- happy: 选了 5 件 common 后切到 rare tab → alert "切换品质会清空" → 确认 → 清空 selectedIds
- happy: 选 5 件后切回相同 tab → 选中状态保留
- happy: 选满 10 件 → "合成"按钮 enabled

### Story 33.4: ComposeUpgradeUseCase + POST /compose/upgrade

As an iPhone 用户,
I want 点合成按钮后 App 调 server 完成合成，并防止重复提交,
So that 我得到一个新的高一阶品质道具.

**Acceptance Criteria:**

**Given** Story 33.3 已选 10 件 + Story 32.4 server POST /compose/upgrade 可用
**When** 用户点"合成"按钮
**Then** 触发 `ComposeUpgradeUseCase`:
- 客户端生成 idempotencyKey（UUID v4）
- 显示 loading（按钮 disabled + 转圈）
- 调用 POST /compose/upgrade 带 fromRarity + selectedIds + idempotencyKey
- 成功 → 触发 Story 33.5 reward popup + 重新加载 overview + inventory
- 失败 → ErrorPresenter 弹 alert（按错误码：5005 / 5006 / 5007 / 5002 / 5003 / 1008）
**And** **同一次提交过程中**点击不重复触发（按钮 disabled）
**And** UseCase 失败后按钮恢复可点（生成新 idempotencyKey）
**And** **单元测试覆盖**（≥5 case，mocked APIClient）:
- happy: 调 upgrade → 拿到 reward → 触发 popup + 重新加载
- happy: 同一 use case 内多次点 → 只发 1 次 API
- edge: 5006 品质不一致 → alert "材料品质不一致"（理论不该，因为 UI 已校验，但兜底）
- edge: 5003 道具状态不可用 → alert "其中有道具不可用"
- edge: network error → ErrorPresenter RetryView

### Story 33.5: 成功 popup + 失败错误处理

As an iPhone 用户,
I want 合成成功后看到一个弹窗展示新道具，且失败时有清晰提示,
So that 我有获得感 / 知道为什么失败.

**Acceptance Criteria:**

**Given** Story 33.4 ComposeUpgradeUseCase 已完成
**When** 完成本 story
**Then** 实装 `ComposeRewardPopupView`（复用 Story 21.4 RewardPopupView 结构）:
- 中央显示 reward.iconUrl + name + 高一阶品质徽章
- 文案 "合成成功！获得 {name}"
- "确定"按钮关闭弹窗
- 关闭后 ComposeView 自动刷新 overview + inventory（已在 Story 33.4 触发）
**And** 失败错误弹窗用通用 ErrorPresenter（Epic 2 Story 2.6）+ 文案按错误码映射
**And** **单元测试覆盖**（≥3 case，UI snapshot）:
- happy: reward.rarity=3 (epic) → 弹窗显示紫色徽章
- happy: 点确定 → 弹窗关闭，ViewModel.rewardPopup = nil
- happy: 关闭后 inventory 重新加载，已选 10 件实例从列表中消失（DB status=consumed 后不再返回）

### Story 33.6: 主界面"合成"按钮入口完善

As an iPhone 用户,
I want 主界面的"合成"按钮直接打开合成页,
So that 我可以快速访问合成功能.

**Acceptance Criteria:**

**Given** Epic 2 Story 2.3 主界面"合成"按钮已存在但触发 placeholder
**When** 完成本 story
**Then** 主界面"合成"按钮触发 NavigationRouter `present(.compose)`，弹出 ComposeView Sheet
**And** **单元测试覆盖**（≥2 case）:
- happy: 点击"合成"按钮 → coordinator.present(.compose) 被调用
- happy: Sheet dismiss 后 → coordinator.dismiss() 被调用
**And** **UI 测试覆盖**（XCUITest）: 主界面点"合成"→ Sheet 弹出 ComposeView → 关闭

## Epic 34: 对齐 - 节点 11 合成链路联调

把 Epic 32 (Server 合成事务) + Epic 33 (iOS 合成页) 真实串联，验证节点 11 §4.11 全部验收标准达成。**重点**：合成产出实例进入仓库可见即算 demo 通过，**不依赖节点 9/10 视觉**。

### Story 34.1: 跨端集成测试场景 - 合成链路 E2E

**Acceptance Criteria:**

**Given** Epic 32 + Epic 33 全部完成
**When** 完成本 story
**Then** 输出文档 `_bmad-output/implementation-artifacts/e2e/node-11-compose-e2e.md`，含:
- 准备步骤: server 启动 + 用户登录 + `/dev/grant-cosmetic-batch {rarity:1, count:12}` 给用户 12 件 common
- 验证场景 1（overview）: 主界面点"合成"→ ComposeView 显示 common segment + 12 件 cell + 已选 0/10
- 验证场景 2（选材交互）: 点选 5 件 → 已选 5/10 + 合成按钮 disabled → 继续选 5 件 → 已选 10/10 + 合成按钮 enabled
- 验证场景 3（超额拒绝）: 已选 10 件再点新 cell → Toast "最多选 10 件" + 不变
- 验证场景 4（品质切换清空）: 选 5 件 common → 切到 rare tab → alert "切换品质会清空已选" → 确认 → 清空
- 验证场景 5（合成提交）: 选 10 件 common → 点合成 → loading → 服务端事务 → 弹窗 "合成成功！获得 {新 rare 名}" + 紫色徽章
- 验证场景 6（仓库可见）: 关闭 popup → 关闭 Sheet → 进仓库 → 验证 common 减少 10 件 + rare 增加 1 件
- 验证场景 7（DB 一致性）: mysql 查 user_cosmetic_items → 10 件 common status=3 (consumed) + 1 件 rare status=1 (in_bag) + compose_logs 1 行 + compose_log_materials 10 行
- 验证场景 8（不依赖节点 10 视觉）: rare 实例无论穿不穿戴，仓库列表都能展示（图标 + 名称即可，不要求装在猫身上的视觉效果）
- 验证场景 9（连续合成）: 把 grant 提到 30 件 common → 连续合成 3 次 → 各自独立 idempotencyKey + DB compose_logs 3 行
- 验证场景 10（失败重试）: 故意把第一次 upgrade 的 selectedIds 中混入 1 件 rare → 5006 品质不一致 → alert + 不破坏选中态 → 修正后重试成功
**And** 文档含截图位

### Story 34.2: 节点 11 demo 验收

**Acceptance Criteria:**

**Given** Story 34.1 E2E 文档已就绪
**When** 完成本 story
**Then** 按 §4.11 节点 11 验收标准逐项打勾:
- [ ] 玩家可以手动选满 10 个材料
- [ ] 服务端能校验品质与归属
- [ ] 合成成功后材料被消耗
- [ ] 新道具进入仓库
- [ ] **demo 依据**: 合成产出实例进入仓库可见即算通过，**不依赖节点 9 / 10**
- [ ] **新增**: dev grant-cosmetic-batch 让 demo 凑齐 10 件不必反复开箱
**And** 输出 demo 录屏到 `_bmad-output/implementation-artifacts/demo/node-11-demo-<date>/`
**And** 任何未通过项必须登记原因 + 是否后置

### Story 34.3: 文档同步与 tech debt 登记

**Acceptance Criteria:**

**Given** Epic 32-33 完成 + Story 34.2 demo 通过
**When** 完成本 story
**Then** 检查并更新（无变更也要明确标注）:
- [ ] `docs/宠物互动App_V1接口设计.md` §9.1 / §9.2: 实装与 Story 32.1 锚定的 schema 一致
- [ ] `docs/宠物互动App_数据库设计.md` §5.11 / §5.12: compose_logs / compose_log_materials 表结构与文档一致
- [ ] `docs/宠物互动App_时序图与核心业务流程设计.md` §10 (合成事务): 时序与实装一致
- [ ] `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §6.7 (Compose 模块): 合成页设计 + 选材交互
**And** `_bmad-output/implementation-artifacts/tech-debt-log.md` 追加节点 11 tech debt，可能的:
- 合成抽奖加权随机用 math/rand 全局源（与 Story 20.6 同问题）
- ComposeView 切换 rarity 清空已选时的 confirm alert UX 待美术评审
- 没有"合成历史"页（用户看不到自己历次合成产出），可作为后续运营功能
- compose_log_materials 仅做审计，无线上查询入口

## Epic 35: iOS - 房间链接生成 + Universal Link 解析 + 自动加入

节点 12 iOS 端：链接 scheme 选型 spike + 房间页"分享"按钮 + 链接解析 + 自动 join + 跳转房间页。**节点 12 无 Server Epic**（复用 `/rooms/{id}/join`）。

### Story 35.1: 链接技术方案选型 spike + Info.plist 配置

As an iOS 开发,
I want 在写第一行链接相关代码前确定技术方案（Custom URL Scheme vs Universal Link）,
So that 不被域名 / HTTPS / apple-app-site-association 等基础设施前置卡死.

**Acceptance Criteria:**

**Given** MVP 阶段可能没有正式域名 + HTTPS
**When** 完成 spike
**Then** 输出选型决策文档 `_bmad-output/implementation-artifacts/decisions/0005-deep-link-stack.md`，含:
- 选型决策: **MVP 用 Custom URL Scheme `catapp://room/{roomId}`**（不需要域名 / HTTPS / apple-app-site-association）
- Universal Link 后置: 后续如果有正式域名 + HTTPS 部署，再补 Universal Link 作为更友好的方案（用户分享给微信 / Safari 时不需先安装 App 也能跳转 App Store）
- Info.plist 配置: 添加 `CFBundleURLTypes` 含 scheme `catapp`
**And** 完成 Info.plist 配置（`CFBundleURLTypes` + URL Scheme Identifier）
**And** 验证：模拟器 Safari 输入 `catapp://room/3001` → App 被拉起（跳到 onOpenURL handler）
**And** Universal Link 实装计划登记为 tech debt

### Story 35.2: 房间页内"分享"按钮 + 链接生成

As an iPhone 用户,
I want 在房间页面有一个"分享"按钮，点击后能复制链接 / 调起系统分享面板,
So that 我可以邀请朋友进我的房间.

**Acceptance Criteria:**

**Given** Story 35.1 链接 scheme 已确定 + Story 12.1 房间页已就绪
**When** 完成本 story
**Then** RoomView 顶部加"分享"按钮（icon: square.and.arrow.up）
**And** 点击触发 `ShareLinkUseCase`:
- 拼链接：`catapp://room/{currentRoomId}`
- 调起系统 `ShareLink` (iOS 16+) 或 `UIActivityViewController` (兼容老版本)
- 用户可选：复制 / 微信 / 短信 / 其他 App 分享
**And** 用户当前不在房间时该按钮 disabled
**And** **单元测试覆盖**（≥3 case，mocked RoomViewModel）:
- happy: 在房间 → 按钮 enabled + 点击触发 ShareLinkUseCase
- happy: ShareLinkUseCase 拼出 `catapp://room/3001` 字符串
- edge: 不在房间 → 按钮 disabled
**And** **UI 测试覆盖**（XCUITest）: 房间页点分享 → 系统分享面板出现 + 验证 share content 含正确链接

### Story 35.3: 链接解析处理（Custom URL Scheme + Universal Link 兜底）

As an iPhone 用户,
I want 我点击别人分享给我的 catapp://room/3001 链接，App 能拉起并解析出 roomId,
So that App 知道我想加入哪个房间.

**Acceptance Criteria:**

**Given** Story 35.1 Info.plist 已配置 scheme + Story 35.2 链接格式已定
**When** 完成本 story
**Then** 在 `PetAppApp` 入口实装 `.onOpenURL` handler:
- 解析 URL host=room + path=/{roomId} 拿到 roomId
- 路由到 `DeepLinkRouter.handle(.joinRoom(roomId))`
- DeepLinkRouter 处理逻辑见 Story 35.4
**And** 兼容性：URL 格式不正确（缺 host / 缺 roomId / 非 catapp scheme）→ 不报错，log warning，忽略
**And** Universal Link（如果未来配置）通过 `.onContinueUserActivity(.handlesURLs)` 同样路由到 DeepLinkRouter
**And** **单元测试覆盖**（≥5 case，mocked DeepLinkRouter）:
- happy: catapp://room/3001 → DeepLinkRouter.handle(.joinRoom(roomId: 3001)) 被调用
- edge: catapp://invalid → log warning，不调
- edge: catapp://room/（缺 roomId）→ log warning，不调
- edge: https://other.com/room/3001（非 catapp scheme）→ 系统不会路由到本 handler
- happy: roomId 是字符串可解析为 int → 正确传递

### Story 35.4: 已登录时自动 join + 跳转房间页

As an iPhone 用户,
I want 我点链接进 App 后自动加入房间并打开房间页面，不需要手动操作,
So that 分享 → 加入是流畅的一键体验.

**Acceptance Criteria:**

**Given** Story 35.3 DeepLinkRouter 收到 .joinRoom(roomId) + Story 12.7 JoinRoomUseCase 已实装
**When** DeepLinkRouter.handle(.joinRoom(roomId))
**Then** 处理流程:
- 检查 SessionManager.isLoggedIn
- 已登录 → 直接调 JoinRoomUseCase(roomId) → 成功后弹房间 Sheet
- 未登录（理论应不发生，因 App 启动时自动游客登录）→ 等待登录完成后再处理 deep link
- 已在其他房间（current_room_id 非 null 且 != 目标 roomId）→ 弹 alert "你正在房间 X，确认离开后加入新房间？" → 确认后自动 leave + join + 弹房间 Sheet
**And** Join 失败时显示对应错误 alert（6001 房间不存在 / 6002 房间已满 / 6005 房间状态异常）
**And** App 进 background 时点链接 → 回前台并处理（场景 7 验证）
**And** **单元测试覆盖**（≥6 case，mocked SessionManager + JoinRoomUseCase）:
- happy: 已登录 + 不在其他房间 → 自动 join + 弹房间 Sheet
- happy: 未登录 → DeepLinkRouter 排队等待登录完成
- happy: 已登录 + 已在其他房间 + 用户确认 → leave 当前 + join 新 + 弹 Sheet
- happy: 已登录 + 已在其他房间 + 用户取消 → 不变
- edge: 6002 房间已满 → alert + 不进入房间
- edge: 6001 房间不存在 → alert + 不进入房间
**And** **UI 测试覆盖**（XCUITest）:
- 模拟器 Safari 输入 catapp://room/{已存在 roomId} → App 拉起 + 自动 join 成功 + RoomView Sheet 出现

### Story 2.10: iOS README + 模拟器开发指南

As an iOS 开发 / 新加入团队成员,
I want 一个 ios/ 目录的 README，含模拟器启动 / 跑测试 / 切 build flag / Info.plist 配置位置,
So that 打开 ios/ 立即知道怎么开始.

**Acceptance Criteria:**

**Given** Epic 2 iOS 脚手架基本就绪
**When** 完成本 story
**Then** 输出 `ios/README.md`，含至少以下章节:
- **快速启动**：Xcode 打开 `Cat.xcodeproj` → 选 iPhone 15 模拟器 → Run
- **依赖**：Xcode 16+ / iOS 17+ deployment target / SwiftUI / mock 框架（按 Story 2.1 选型）
- **跑测试**：
  - 单元测试: `xcodebuild test -scheme PetApp -destination 'platform=iOS Simulator,name=iPhone 15'`
  - UI 测试: 同命令含 UITests target
- **开 dev mode**：编译时加 `-D BUILD_DEV` flag 或在 Xcode build settings 加 `BUILD_DEV=true`
- **dev 工具**：解释主界面右上角"重置身份"按钮（Story 2.8）+ 何时用
- **Info.plist 关键配置**：
  - `NSHealthShareUsageDescription`（HealthKit 权限）—— Story 8.1 要求
  - `NSMotionUsageDescription`（CoreMotion 权限）—— Story 8.2 要求
  - `CFBundleURLTypes` (catapp scheme) —— Story 35.1 要求
- **目录结构**：简单复制 iOS 设计文档 §4 + 标注每个目录职责
- **常见 troubleshooting**：模拟器 HealthKit 数据怎么注入 / Keychain 模拟器表现差异 / WS 连接失败 / 后台模式问题
- **服务端联调**：本地 server 跑在 :8080 → iOS 模拟器配置 `APIClient.baseURL = http://localhost:8080`
**And** README **必须保持与代码同步**：每个 epic 完成时如有变化，对齐 epic Story X.3 要更新
**And** **不需要单元测试**（纯文档）—— 但**手动验证**：按 README 步骤一遍走通

## Epic 36: 对齐 - 节点 12 分享链接联调（MVP 最后一个 Epic）

把 Epic 35 (iOS 分享 + 解析) 真实串联（Server 复用既有 /rooms/{id}/join），验证节点 12 §4.12 全部验收标准达成。**完成本 Epic 即 MVP 全部 12 节点 done**。

### Story 36.1: 跨端集成测试场景 - 分享链接 E2E

**Acceptance Criteria:**

**Given** Epic 35 全部完成
**When** 完成本 story
**Then** 输出文档 `_bmad-output/implementation-artifacts/e2e/node-12-share-e2e.md`，含:
- 准备步骤: server 启动 + 2 个用户账号 A、B（A、B 各自模拟器）
- 验证场景 1（链接生成）: A 创建房间 X → 进房间页点"分享"按钮 → 系统分享面板出现 → 选择"复制" → 剪贴板验证含 `catapp://room/X`
- 验证场景 2（链接解析）: B 模拟器 Safari 输入 `catapp://room/X` → App 拉起 → DeepLinkRouter 收到 → 自动 JoinRoom → B 进 X 房间
- 验证场景 3（A 看到 B 加入）: A 房间页收到 member.joined 广播 → 成员列表多 B
- 验证场景 4（已在其他房间）: B 创建另一个房间 Y → 用别人 share 的 X 链接 → alert "你正在房间 Y，确认离开后加入新房间？" → 确认 → leave Y + join X
- 验证场景 5（房间不存在）: 用 `catapp://room/99999`（不存在的 id）→ App 拉起 → 6001 alert "房间不存在"
- 验证场景 6（房间已满）: A、C、D、E 在 X 房间满员 → B 用 X 链接 → 6002 alert "房间已满"
- 验证场景 7（后台拉起）: B App 在后台 → 点链接 → App 自动唤起 + 处理 link
- 验证场景 8（未登录场景，理论不发生）: B 全新安装 + 启动还没完成 guest login → 点链接 → DeepLinkRouter 排队 → 登录完成后处理
**And** 文档含截图位

### Story 36.2: 节点 12 demo 验收

**Acceptance Criteria:**

**Given** Story 36.1 E2E 文档已就绪
**When** 完成本 story
**Then** 按 §4.12 节点 12 验收标准逐项打勾:
- [ ] 可生成有效房间链接
- [ ] 打开链接可解析房间号
- [ ] 已安装 App 时可拉起并尝试加入房间
- [ ] **新增**: 已在其他房间时弹 alert 确认
- [ ] **新增**: 房间不存在 / 已满 时正确报错
**And** 输出 demo 录屏到 `_bmad-output/implementation-artifacts/demo/node-12-demo-<date>/`
**And** 任何未通过项必须登记原因 + 是否后置

### Story 36.3: 文档同步与 tech debt 登记 + **MVP 整体收官**

**Acceptance Criteria:**

**Given** Epic 35 完成 + Story 36.2 demo 通过 + **全部 12 节点 done**
**When** 完成本 story
**Then** 检查并更新（无变更也要明确标注）:
- [ ] `docs/宠物互动App_V1接口设计.md`: 节点 12 无新接口，但深链 URL 协议（`catapp://room/{roomId}`）需要追加到文档（建议加 §16 "深链协议"）
- [ ] `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md`: DeepLinkRouter 设计补充
**And** `_bmad-output/implementation-artifacts/tech-debt-log.md` 追加节点 12 tech debt:
- Universal Link 未实装（仅 Custom URL Scheme），用户分享给未装 App 的人无法跳转 App Store
- 链接 scheme `catapp` 在生产前需要 Apple 审核确认（确保不与系统 / 知名 App 冲突）
- 未做"最近房间历史"功能，每次都要靠链接 / 房间号
**And** **MVP 整体收官清单**（本 Story 是 MVP 最后一个 story）:
- [ ] **全部 12 节点都已 demo 验收通过**（节点 1-12）
- [ ] **全部 5 个 Layer 2 集成测试 story 都已通过**（4.7 / 11.9 / 20.9 / 26.5 / 32.5）
- [ ] **全部接口契约已落 V1 接口设计文档**（AR28 验证）
- [ ] **全部数据库表已落 数据库设计 文档**（13 张业务表 + render_config 字段）
- [ ] **全部 tech debt 已登记** `_bmad-output/implementation-artifacts/tech-debt-log.md`
- [ ] **server 测试 + iOS 测试都能在 CI 跑通**（`bash scripts/build.sh --test` + `xcodebuild test`）
- [ ] CLAUDE.md / MEMORY.md 更新为"MVP 完成"状态
- [ ] **错误码全覆盖审计**（GAP Q 修补）：审计 V1接口设计 §3 列出的全部 26 个错误码：
  - 每个错误码标注"实装 epic / 客户端处理位置 / Post-MVP" 三档
  - 未实装的（如 1004 / 7002 / 2001-2003 微信相关 / 3001 GAP K 验收用）需说明原因 + 是否在 MVP 范围
  - 输出 `_bmad-output/implementation-artifacts/error-code-audit.md`
**And** Git tag `mvp-v1.0` 标记 MVP 完成
