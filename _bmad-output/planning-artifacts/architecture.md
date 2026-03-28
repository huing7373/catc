---
stepsCompleted: [1, 2, 3, 4, 5, 6, 7, 8]
lastStep: 8
status: 'complete'
completedAt: '2026-03-28'
inputDocuments: [prd.md, prd-validation-report.md, technical-kucha-cat-smartwatch-research-2026-03-26.md]
workflowType: 'architecture'
project_name: 'cat'
user_name: 'Zhuming'
date: '2026-03-27'
---

# Architecture Decision Document

_This document builds collaboratively through step-by-step discovery. Sections are appended as we work through each architectural decision together._

## Project Context Analysis

### Requirements Overview

**Functional Requirements:**
63 个 FR 横跨 11 个子系统。架构层面最关键的 FR 集群：
- **猫动画状态机（FR1-5）：** SpriteKit 帧动画 + CMMotionActivity 驱动，要求 ≥24fps 且额外耗电 ≤5%
- **序列化礼物系统（FR6-11, FR43）：** 服务端预分配序列 + 客户端离线消费 + 多设备冲突校准，是最复杂的一致性问题
- **分层皮肤渲染（FR17-22）：** 5 层 z-order 叠加 + CDN 按需下载 + 版本校验/撤回 + LRU 缓存管理
- **社交同步（FR23-32）：** MVP HTTP 轮询 30 秒 → Growth WebSocket，好友猫插值动画，触碰 <5 秒端到端
- **三端数据同步（FR20, FR39, FR58）：** 手表本地 → WatchConnectivity → iPhone → HTTPS → 服务端，需要多通道 fallback

**Non-Functional Requirements:**
28 个 NFR，架构驱动力最强的约束：
- **Performance：** 24fps 动画 + <2s 启动 + <5s 触碰延迟 + ≤50MB 内存 + ≤30MB 包体
- **电量硬约束：** ≤10% 额外耗电（16h 唤醒、80 次抬腕场景）
- **Security：** HTTPS/WSS + JWT + 健康数据不出端 + PIPL 境内部署
- **Scalability：** MVP 5K DAU → 爆发 50K DAU，单 Go 实例 ≥10K WebSocket 并发
- **Reliability：** ≥99.5% 可用率 + <0.1% 崩溃率 + 离线降级 100% 可用

**涌现组件（PRD 未定义，架构层必需）：**

| 涌现组件 | 角色 | 必要性 |
|---------|------|--------|
| **CatStateMachine** (Observable 单例) | 手表端中央事件总线，8 个下游系统订阅状态变化 | 避免 8 个组件各自监听传感器导致重复代码+状态不一致 |
| **EnergyBudgetManager** (4 档位制) | 全局资源调度器，监控并动态调整所有组件的资源消耗 | 电量优化必须统一管理，否则无法在 ≤10% 约束内 |
| **SyncCoordinator** (编排器) | 离线→在线同步编排，优先级队列 + 并发限制 | 避免联网时同步风暴导致 watchOS 杀掉 App |
| **NetworkReachabilityManager** (单例) | 统一网络状态广播，通过 Combine Publisher 通知所有订阅者 | 避免 8 个组件各自检测网络状态导致判断不一致 |

**Scale & Complexity:**
- Primary domain: watchOS-first 移动全栈（Watch + iPhone + Go Backend）
- Complexity level: Medium-High（三端同步 + 离线优先 + 实时社交 + 电量约束）
- Estimated architectural components: ~15（手表端 6-7 模块 + iPhone 端 3-4 模块 + 后端 4-5 模块）

### Technical Constraints & Dependencies

| 约束 | 影响范围 | 架构应对 |
|------|---------|---------|
| watchOS 后台运行受限（~15min 刷新） | 动画、同步、通知 | 抬腕恢复 + 离线计算 + APNs 兜底 |
| Apple Watch 内存 ~200-300MB 可用 | 动画、皮肤缓存 | 纹理内存 ≤20MB + LRU 10 套上限 |
| 手表端无 StoreKit | 内购 | iPhone App 承担所有支付 |
| WatchConnectivity 不可靠 | 皮肤同步 | 三通道瀑布：sendMessage(3s超时) → transferUserInfo(30s超时) → 服务端推送兜底 + 应用层 ACK |
| SpriteKit watchOS 功能受限 | 动画特效 | Sprite Sheet 帧动画为主 + 程序化补间辅助 |
| CMMotionActivity 3-5 秒延迟 | 猫状态映射 | 3 秒防抖 + 过渡动画缓冲 + 加速度计辅助预测 |
| Core Haptics watchOS 部分受限 | 触觉反馈 | WKHapticType 为底 + Core Haptics 增强 |
| Sign in with Apple 为唯一登录 | 认证流程 | JWT + 双密钥轮换（新签发+旧验证并行 24h） |
| Always-On Display 系统级限制 | 显示 | AOD 必须独立渲染路径，完全绕过 SpriteKit Scene |
| Motion & Fitness 权限可能被拒绝 | 核心循环 | 无权限降级体验：计时器微动 + 时间解锁替代步数解锁 |
| 社交配对需双方有 Apple Watch | 社交冷启动 | 架构抽象"好友能力等级"，FR61 iPhone-only 好友需高优先级 |
| CDN 下载可能中断 | 皮肤资源 | 原子下载（临时文件→哈希校验→重命名），不完整文件不进缓存 |

### Cross-Cutting Concerns Identified

1. **数据同步与一致性** — 贯穿所有模块。冲突策略："服务端权威 + 客户端乐观 + 联网校准"。序列冲突时不丢物品，补发等价物
2. **动态电量预算池** — EnergyBudgetManager 4 档位（正常/节能/极省/AOD），实时追踪能耗并动态降级所有组件
3. **离线/在线状态切换** — NetworkReachabilityManager 统一广播，SyncCoordinator 编排同步优先级队列（JWT刷新 → 序列列表 → 签到盲盒批量 → 皮肤 → 好友状态），最多 3 个并发请求
4. **皮肤资源管线** — CDN 原子下载 + manifest.json 校验 + 版本号/撤回机制 + LRU 淘汰（钉住当前穿戴） + 分层渲染
5. **安全与隐私** — JWT 双密钥轮换 + 健康数据隔离 + PIPL 境内部署 + 社交隐私
6. **猫状态机（中央事件总线）** — Observable 单例，独立于 SpriteKit，8 个下游订阅。60s 无转换自愈 → idle。状态持久化供 App 恢复
7. **同步通道瀑布策略** — WC sendMessage(即时+3sACK) → WC transferUserInfo(排队+接收端合并) → 服务端推送兜底。iPhone 同步三态 UI
8. **前台/后台同步区分** — HTTP 轮询仅前台运行，后台通过 APNs 静默推送偶尔更新好友状态
9. **资源完整性保障** — 原子下载 + manifest 帧数/层级/尺寸校验 + 校验失败回退默认皮肤 + 撤回在场景切换时统一处理
10. **连接生命周期管理** — WebSocket deadline + 心跳检测 + defer close + goroutine 计数监控。Redis maxmemory allkeys-lru + 状态 TTL 120s
11. **好友能力等级建模** — FriendCapability 枚举（fullWatch / iphoneOnly / inactive / deleted），每级对应渲染策略+同步策略+交互策略配置包
12. **皮肤配置多端冲突解决** — 皮肤库存服务端单一写入源，皮肤配置（穿戴）以服务端为权威 + last-write-wins with timestamp

## Starter Template Evaluation

### Primary Technology Domain

三端独立技术栈：watchOS-first 移动端（Swift/SwiftUI/SpriteKit）+ Go 后端单体服务。非典型 web 全栈，starter template 适用性有限。

### Starter Options Considered

#### Apple Watch + iPhone 端

| 选项 | 评估 | 结论 |
|------|------|------|
| Xcode "iOS App with Watch App" 模板 | 苹果官方标准模板，提供双 target 结构 + SwiftUI | ✅ 唯一选择 |
| 第三方 watchOS starter | 不存在成熟的社区 watchOS+SpriteKit starter | ❌ 不可用 |

#### Go 后端

| 选项 | 优点 | 缺点 | 结论 |
|------|------|------|------|
| pilinux/gorest | Gin+GORM+JWT+PG+Redis+Docker+2FA | 无 WebSocket、无 APNs、包含大量不需要的功能 | ❌ 改造成本 > 新建 |
| zetsux/gin-gorm-api-starter | CSR 模式清晰、自动化测试 | 无 Redis、无 WebSocket、过于基础 | ❌ 覆盖度不足 |
| 自定义项目结构 | 精确匹配需求、无冗余代码、架构完全可控 | 需要定义项目结构规范 | ✅ 推荐 |

### Selected Approach: 平台模板 + 自定义后端

**Rationale:**
1. watchOS 生态没有 starter 替代方案，Xcode 模板是唯一起点
2. Go 后端需求高度定制化（WebSocket 房间 + APNs + 序列化礼物），现有 starter 匹配度低
3. Claude 编码模式下，从零搭建项目结构的成本极低
4. 避免引入不需要的依赖和代码，保持代码库精简

### 代码仓库组织: Monorepo

**决策：单一 Monorepo**

```
cat/
├── ios/                          # Xcode 工程（watchOS + iPhone）
│   ├── Cat.xcworkspace
│   ├── Cat.xcodeproj
│   ├── CatWatch/                 # watchOS App target
│   │   ├── App/
│   │   │   └── CatWatchApp.swift
│   │   ├── Views/
│   │   │   ├── CatView.swift
│   │   │   ├── BlindBoxView.swift
│   │   │   ├── CheckInView.swift
│   │   │   └── QuickSkinView.swift
│   │   ├── Scenes/
│   │   │   ├── CatScene.swift
│   │   │   ├── CatNode.swift
│   │   │   └── FriendCatNode.swift
│   │   ├── ViewModels/
│   │   │   ├── CatViewModel.swift
│   │   │   ├── BlindBoxViewModel.swift
│   │   │   └── SocialViewModel.swift
│   │   ├── Core/                 # 涌现组件（手表端独有）
│   │   │   ├── CatStateMachine.swift
│   │   │   ├── EnergyBudgetManager.swift
│   │   │   ├── SyncCoordinator.swift
│   │   │   ├── HapticManager.swift
│   │   │   └── SensorManager.swift
│   │   ├── Complication/
│   │   │   └── ComplicationProvider.swift
│   │   └── Resources/
│   ├── CatPhone/                 # iPhone App target
│   │   ├── App/
│   │   │   └── CatPhoneApp.swift
│   │   ├── Views/
│   │   │   ├── HomeView.swift
│   │   │   ├── SkinGalleryView.swift
│   │   │   ├── FriendsView.swift
│   │   │   ├── SettingsView.swift
│   │   │   └── LoginView.swift
│   │   └── ViewModels/
│   │       ├── SkinViewModel.swift
│   │       ├── FriendViewModel.swift
│   │       └── AuthViewModel.swift
│   └── CatShared/               # 本地 Swift Package（共享代码）
│       ├── Package.swift
│       ├── Sources/CatShared/
│       │   ├── Models/
│       │   │   ├── User.swift
│       │   │   ├── SkinConfig.swift
│       │   │   ├── BlindBox.swift
│       │   │   └── FriendCapability.swift
│       │   ├── Networking/
│       │   │   ├── APIClient.swift
│       │   │   ├── APIEndpoints.swift
│       │   │   ├── WCManager.swift
│       │   │   └── Environment.swift
│       │   ├── Persistence/
│       │   │   └── LocalStore.swift
│       │   └── Utilities/
│       │       └── NetworkReachability.swift
│       └── Tests/
├── server/                       # Go 后端
│   ├── cmd/server/
│   │   └── main.go
│   ├── internal/
│   │   ├── config/
│   │   ├── middleware/
│   │   ├── handler/
│   │   │   ├── auth.go
│   │   │   ├── user.go
│   │   │   ├── blindbox.go
│   │   │   ├── checkin.go
│   │   │   ├── friend.go
│   │   │   ├── touch.go
│   │   │   ├── admin.go
│   │   │   └── ws.go
│   │   ├── service/
│   │   │   ├── auth_service.go
│   │   │   ├── blindbox_service.go
│   │   │   ├── skin_service.go
│   │   │   ├── friend_service.go
│   │   │   ├── touch_service.go
│   │   │   └── sequence_service.go
│   │   ├── repository/
│   │   │   ├── user_repo.go
│   │   │   ├── skin_repo.go
│   │   │   ├── friend_repo.go
│   │   │   └── blindbox_repo.go
│   │   ├── model/
│   │   │   ├── user.go
│   │   │   ├── cat.go
│   │   │   ├── skin.go
│   │   │   ├── friendship.go
│   │   │   ├── blindbox.go
│   │   │   ├── checkin.go
│   │   │   ├── touch_event.go
│   │   │   └── gift_sequence.go
│   │   ├── ws/
│   │   │   ├── hub.go
│   │   │   ├── client.go
│   │   │   └── room.go
│   │   ├── push/
│   │   │   └── apns.go
│   │   ├── cron/
│   │   │   ├── daily_stats.go
│   │   │   └── audit.go
│   │   └── dto/
│   ├── pkg/
│   │   ├── jwt/
│   │   ├── redis/
│   │   └── validator/
│   ├── migrations/
│   ├── deploy/
│   │   ├── Dockerfile
│   │   └── docker-compose.yml
│   ├── .env.development
│   ├── .env.staging
│   ├── go.mod
│   └── go.sum
├── assets/                       # 美术资源（Git LFS）
│   ├── sprites/
│   │   ├── body/
│   │   ├── expression/
│   │   ├── outfit/
│   │   ├── headwear/
│   │   └── accessory/
│   ├── complication/             # Complication 专用插画（独立目录）
│   │   ├── rectangular/
│   │   └── circular/
│   ├── effects/
│   ├── ui/
│   └── manifest.json             # 资产清单（CI 校验源）
├── scripts/                      # CLI 管理工具 + 部署脚本
├── docs/
├── .gitignore                    # 含 .env.production
└── .gitattributes                # Git LFS 配置
```

**Monorepo 理由：**
- Claude 编码只需一个 repo 上下文，跨端变更原子性
- 美术资源统一管理 + CI 自动校验
- API 变更在同一 PR 里同步更新两端 DTO
- 降低非工程师角色（Zhuming）的维护认知成本

### 架构模式决策

**Swift 端: MVVM + @Observable**
- SwiftUI 原生模式，Claude 编码友好
- 涌现组件（CatStateMachine 等）独立于 Core/，不需要 TCA/VIPER
- CatShared 本地 Swift Package 共享模型/网络/持久化

**Go 后端: Handler-Service-Repository 三层**
- Handler 只做参数解析和响应格式化
- Service 包含业务逻辑，可被多个 Handler 复用
- Repository 封装 GORM 查询，Service 不直接操作 DB
- ws/ 目录 MVP 建好空实现，Growth 阶段填充

**依赖注入: 手动构造函数注入（零框架）**
- main.go 显式构造所有依赖链，零魔法
- 拆分为 initDB(), initServices(), initRouter() 保持可读
- 裤衩猫预计 10-15 个 Service，手动注入完全可控

### 数据持久化策略

**Swift 端: 混合持久化**

| 存储方式 | 数据类型 | 理由 |
|---------|---------|------|
| SwiftData | 皮肤库存、签到历史、好友列表 | 结构化数据 + 需要查询能力 + SwiftUI @Query 联动 |
| UserDefaults | 用户偏好、当前盲盒进度、状态机最后状态 | 读写最快（<10ms）、App 恢复优先 |
| 文件系统 | 皮肤纹理缓存（PNG） | 原子下载 + LRU 管理，不膨胀数据库 |

盲盒状态双写（SwiftData + UserDefaults 备份），`LocalStore` 统一封装隐藏复杂度。

**Go 后端:**

| 存储 | 数据类型 |
|------|---------|
| PostgreSQL | 用户、皮肤、好友、签到、盲盒记录等持久数据 |
| Redis | 在线状态、WebSocket 会话、步数缓存、好友状态（TTL 120s） |

### API 契约管理

**手动 DTO + CI 集成测试**
- Go 端 `internal/dto/` 和 Swift 端 `CatShared/Models/` 手动保持一致
- Monorepo 中 Claude 改 API 时自然同时改两端
- CI 安全网：Go 端启动 → 发真实请求 → Swift Codable 解析 → 字段不匹配就红

### CI/CD 三管线分流

| 管线 | 触发路径 | 产出 |
|------|---------|------|
| server | `server/**` 变更 | Docker 镜像 → 部署云服务器 |
| ios | `ios/**` 变更 | Xcode 构建 → TestFlight |
| assets | `assets/**` 变更 | manifest 校验 → CDN 上传 |

### 环境配置

- `.env.development` / `.env.staging` 入 Git
- `.env.production` 不入 Git（.gitignore 写死），CI 从 GitHub Secrets 注入
- Swift 端通过编译 flag 切换环境（`#if DEBUG` → localhost）

### 美术资源工作流

- `assets/manifest.json` 为资产清单 source of truth
- CI 自动校验：文件是否齐全、尺寸是否正确、帧数是否匹配
- 设计师可选：直接 GitHub Desktop 提交 或 共享文件夹 → CI 自动同步
- 所有美术资源通过 Git LFS 管理

### Initialization

**Apple Watch + iPhone:**
Xcode → New Project → watchOS → "iOS App with Watch App"
- Interface: SwiftUI, Language: Swift
- 手动添加: SpriteKit, HealthKit, WatchConnectivity, WidgetKit
- 创建 CatShared 本地 Swift Package

**Go 后端:**
```bash
mkdir -p server/cmd/server server/internal/{config,middleware,handler,service,repository,model,ws,push,cron,dto} server/pkg/{jwt,redis,validator} server/migrations server/deploy
go mod init github.com/zhuming/cat-server
go get github.com/gin-gonic/gin gorm.io/gorm gorm.io/driver/postgres github.com/redis/go-redis/v9 github.com/gorilla/websocket github.com/sideshow/apns2 github.com/golang-jwt/jwt/v5 github.com/robfig/cron/v3
```

### Architectural Decisions Provided by Platform

**Language & Runtime:** Swift 6+ / Go 1.22+
**UI Framework:** SwiftUI（双端）+ SpriteKit（手表动画层）
**Build Tooling:** Xcode Build System + SPM（客户端）/ Go Modules（后端）
**Testing Framework:** XCTest（客户端）/ Go testing + testify（后端）
**Package Management:** SPM（客户端）/ Go Modules（后端）
**Development Experience:** Xcode Previews + Simulator（客户端）/ Air hot-reload（后端）

## Core Architectural Decisions

### Decision Priority Analysis

**Critical Decisions (Block Implementation):**
- 数据库迁移策略 → golang-migrate 手动迁移文件
- JWT Token 有效期 → Access 7天 / Refresh 30天
- WebSocket 消息协议 → Protobuf
- 手表导航模式 → NavigationStack
- SpriteKit ↔ SwiftUI 通信 → CatStateMachine 桥接

**Important Decisions (Shape Architecture):**
- Redis Write-Through 缓存模式
- API 限流分级策略
- REST 错误响应标准格式
- 日志策略 → zerolog 结构化 JSON
- MVP → Growth 双协议迁移路径

**Deferred Decisions (Post-MVP):**
- Grafana + Prometheus 监控（Growth 50K DAU 时）
- PostgreSQL 流复制（Growth 阶段）
- CDN 多区域分发（用户地理分布明确后）

### Data Architecture

#### 数据库迁移：golang-migrate 手动迁移文件

- **工具:** [golang-migrate v4.19.1](https://github.com/golang-migrate/migrate)
- **策略:** 每次变更生成 `{timestamp}_{description}.up.sql` + `.down.sql` 到 `server/migrations/`
- **不用 GORM AutoMigrate:** 生产环境不可控，无法回滚，无审计记录
- **CI 集成:** 自动检测未应用的迁移，阻止部署

#### Redis 缓存模式：Write-Through + TTL 分层

- **模式:** 写入时同时更新 PostgreSQL + Redis，读取优先 Redis，miss 时从 PG 回填
- **TTL 策略:**

| 数据类型 | TTL | 理由 |
|---------|-----|------|
| 好友在线状态 | 120s | 过期即视为离线 |
| 步数缓存 | 300s | 轮询间隔的倍数 |
| 序列消费位置 | 不过期 | 持久缓存，PG 为权威 |
| WebSocket 会话 | 连接生命周期 | 断连时删除 |
| 限流计数器 | 滑动窗口 60s | 自动过期 |

### Authentication & Security

#### JWT Token 策略

- **Access Token 有效期:** 7 天（手表使用场景——用户可能多天不联网，频繁要求重新登录体验极差）
- **Refresh Token 有效期:** 30 天
- **双密钥轮换:** 新密钥签发 + 旧密钥验证并行 24 小时
- **库:** [golang-jwt/jwt v5](https://github.com/golang-jwt/jwt)
- **Token 存储:** 手表端 Keychain，iPhone 端 Keychain
- **刷新流程:** Access Token 过期 → 自动用 Refresh Token 换取新对 → 静默对用户透明

#### API 限流策略

- **实现:** Gin 中间件 + Redis 滑动窗口计数器
- **分级阈值:**

| 端点类别 | 阈值 | 维度 | 理由 |
|---------|------|------|------|
| `/auth/*` | 10 次/分钟 | Per IP | 防暴力登录 |
| `/touch` | 6 次/分钟 | Per User | FR31（10分钟3次推送限制）|
| `/friends/status` | 2 次/分钟 | Per User | 30秒轮询 = 2次/分钟 |
| `/blindbox/sync` | 10 次/分钟 | Per User | 批量同步不频繁 |
| `/admin/*` | 30 次/分钟 | Per IP | 管理接口有限宽松 |
| 其他 | 60 次/分钟 | Per User | 通用保护 |

- **超限响应:** HTTP 429 + `Retry-After` 头

### API & Communication Patterns

#### REST 错误响应标准格式

```json
{
  "error": {
    "code": "SEQUENCE_CONFLICT",
    "message": "序列位置冲突，已从后续位置补发",
    "details": { "conflicted_position": 7, "new_position": 10 }
  }
}
```

- HTTP 状态码遵循标准：400/401/403/404/409/429/500
- `code` 字段为机器可读的业务错误码，Swift 端 `APIError` enum 一一对应
- `message` 供调试，不直接显示给用户
- `details` 可选，携带业务上下文

**核心业务错误码:**

| Code | HTTP | 场景 |
|------|------|------|
| `AUTH_EXPIRED` | 401 | Access Token 过期 |
| `AUTH_INVALID` | 401 | Token 无效或被吊销 |
| `SEQUENCE_CONFLICT` | 409 | 序列位置冲突 |
| `SEQUENCE_EXHAUSTED` | 409 | 本地序列缓存耗尽 |
| `SKIN_REVOKED` | 410 | 皮肤已被撤回 |
| `FRIEND_LIMIT` | 403 | 好友数达上限 |
| `RATE_LIMITED` | 429 | 请求过于频繁 |
| `DEVICE_CONFLICT` | 409 | 多设备同时操作冲突 |

#### WebSocket 消息协议：Protobuf

- **Proto 定义:** `api/proto/` 目录，CI 自动生成 Go + Swift 代码
- **Swift 端:** [apple/swift-protobuf](https://github.com/apple/swift-protobuf)（watchOS 已确认支持）
- **Go 端:** `google.golang.org/protobuf`
- **优势:** 比 JSON 节省 ~60% 带宽，对手表网络和电量有利
- **范围:** Growth 阶段 WebSocket 消息用 Protobuf；MVP HTTP 轮询响应仍用 JSON

**Proto 文件结构:**
```
api/
├── proto/
│   ├── friend_status.proto    # 好友状态消息
│   ├── touch_event.proto      # 触碰事件
│   ├── cat_state.proto        # 猫状态同步
│   └── room.proto             # 房间管理消息
└── gen/                       # CI 生成的代码（.gitignore）
    ├── go/
    └── swift/
```

#### MVP → Growth 双协议迁移路径

```
MVP 阶段:
  客户端 → GET /friends/status (JSON, 30s 轮询)
  客户端 → POST /touch (JSON, fire-and-forget)

Growth 阶段:
  客户端 → WS /ws/room (Protobuf, 实时双向)
  服务端同时保留 HTTP 端点（兼容旧版本客户端）
```

- `SocialViewModel` 抽象同步接口 `SocialSyncProtocol`
- MVP 实现 `HTTPPollingSyncService`，Growth 新增 `WebSocketSyncService`
- 客户端运行时检测 WebSocket 可用性 → 自动选择最优通道
- 不可用时回退 HTTP 轮询（优雅降级）

### Frontend Architecture

#### 手表端导航：NavigationStack

- watchOS 10+ 推荐 NavigationStack 替代旧 NavigationView
- 主猫界面是根视图（永远在栈底）
- 盲盒展示 / 签到 → `.sheet` 或 `.navigationDestination`
- 快速换装 → `.overlay` 手势覆盖层，非独立页面
- 好友列表 → NavigationStack push

#### SpriteKit ↔ SwiftUI 通信模式

```
SwiftUI 层                    SpriteKit 层
┌─────────────┐              ┌──────────────┐
│ CatView     │   SpriteView │ CatScene     │
│ (SwiftUI)   │◄────────────►│ (SKScene)    │
└──────┬──────┘              └──────┬───────┘
       │                            │
       ▼                            ▼
┌─────────────┐              ┌──────────────┐
│CatViewModel │◄─ @Observable│CatStateMachine│
│             │   订阅       │ (Core/)      │
└─────────────┘              └──────────────┘
```

- **桥梁:** `CatStateMachine` 是 SwiftUI 和 SpriteKit 的共享状态源
- **SwiftUI → SpriteKit:** ViewModel 更新 StateMachine 属性 → CatScene 通过 Combine 订阅响应
- **SpriteKit → SwiftUI:** Scene 触摸事件通过 delegate/closure 回调 ViewModel
- **不用 NotificationCenter:** 难追踪和调试，用 Combine Publisher 或 @Observable

### Infrastructure & Deployment

#### 日志策略：zerolog 结构化 JSON

- **库:** [zerolog](https://github.com/rs/zerolog) — 零内存分配，Go 生态性能最优
- **格式:** JSON 结构化日志输出到 stdout
- **日志级别:** Debug（开发）→ Info（生产默认）→ Warn → Error
- **标准字段:** timestamp, request_id, user_id, endpoint, duration_ms, status_code
- **收集:** Docker 日志驱动 → MVP 直接 `docker logs` 查看

#### 监控策略

| 层面 | MVP | Growth |
|------|-----|--------|
| 客户端崩溃 | Firebase Crashlytics | + 自定义事件追踪 |
| 后端健康 | `/health` 端点 + UptimeRobot（免费） | Grafana + Prometheus |
| 后端日志 | zerolog → stdout → docker logs | + ELK/Loki 集中日志 |
| 业务指标 | FR44 每日摘要 Go cron 脚本 | + 自定义 Dashboard |
| 性能指标 | 无 | Prometheus Go client 暴露指标 |

#### 数据库备份

- **MVP:** `pg_dump` 每日定时备份到对象存储（cron job），保留最近 30 天
- **Growth:** PostgreSQL 流复制 + 自动故障转移 + 增量备份

### Decision Impact Analysis

**Implementation Sequence:**
1. 数据库 Schema + golang-migrate 迁移基础 → 其他所有后端模块依赖
2. JWT 认证中间件 → 所有 API 端点依赖
3. CatStateMachine + SensorManager → 猫动画 + 盲盒 + 签到依赖
4. REST API 错误格式 + DTO → 所有客户端-服务端通信依赖
5. Redis Write-Through → 好友状态 + 限流 + 序列缓存依赖
6. APNs 推送 → 触碰社交依赖
7. Protobuf proto 定义 → Growth WebSocket 依赖（可延迟）

**Cross-Component Dependencies:**
- JWT 双密钥轮换 ↔ Redis（存储吊销列表）
- 限流中间件 ↔ Redis（滑动窗口计数器）
- SyncCoordinator ↔ JWT（联网后首先刷新 Token）
- EnergyBudgetManager ↔ 轮询频率（动态调整 /friends/status 间隔）
- Protobuf → 需要 CI 代码生成管线（`api/proto/` → `api/gen/`）
- CatStateMachine ↔ 好友状态上报（猫状态变化触发服务端更新）

## Implementation Patterns & Consistency Rules

### Pattern Categories Defined

**已识别 32 个潜在冲突点**，分布在命名、结构、格式、通信、流程 5 个类别中。

### Naming Patterns

#### 数据库命名

| 规则 | 约定 | 示例 |
|------|------|------|
| 表名 | 复数 snake_case | `users`, `gift_sequences`, `touch_events` |
| 列名 | snake_case | `user_id`, `created_at`, `skin_config` |
| 外键 | `{referenced_table_singular}_id` | `user_id`, `skin_id` |
| 索引 | `idx_{table}_{columns}` | `idx_users_apple_id`, `idx_friendships_user_id_friend_id` |
| 唯一约束 | `uq_{table}_{columns}` | `uq_users_apple_id` |
| 布尔列 | `is_` 前缀 | `is_active`, `is_revoked` |
| 时间列 | `_at` 后缀 | `created_at`, `last_active_at` |
| JSON 列 | 描述性名称 | `skin_config`, `sequence_data` |

#### API 命名

| 规则 | 约定 | 示例 |
|------|------|------|
| 端点 | 复数 snake_case，RESTful | `/friends/status`, `/blind_boxes/sync` |
| 路由参数 | `:param` (Gin 格式) | `/users/:user_id/skins` |
| 查询参数 | snake_case | `?page_size=20&last_id=abc` |
| HTTP 方法 | 语义化 | GET 读、POST 创建、PUT 全量更新、PATCH 部分更新、DELETE 删除 |
| API 版本 | URL 前缀 | `/v1/friends/status` |
| 自定义头 | `X-Cat-` 前缀 | `X-Cat-Device-Id`, `X-Cat-Client-Version` |

#### 代码命名

**Go 端：**

| 规则 | 约定 | 示例 |
|------|------|------|
| 文件名 | snake_case | `auth_service.go`, `user_repo.go` |
| 结构体 | PascalCase | `BlindBoxService`, `GiftSequence` |
| 方法 | PascalCase（导出）/ camelCase（私有） | `SyncSequence()`, `validateToken()` |
| 变量 | camelCase | `userID`, `skinConfig` |
| 常量 | PascalCase 或 ALL_CAPS | `MaxFriends = 10`, `DEFAULT_TTL` |
| 接口 | 动词+er / 名词 | `SkinRepository`, `TokenValidator` |
| 错误变量 | `Err` 前缀 | `ErrSequenceConflict`, `ErrTokenExpired` |

**Swift 端：**

| 规则 | 约定 | 示例 |
|------|------|------|
| 文件名 | PascalCase (类型名一致) | `CatStateMachine.swift`, `BlindBoxViewModel.swift` |
| 类型 | PascalCase | `SkinConfig`, `FriendCapability` |
| 属性/方法 | camelCase | `currentState`, `syncSequence()` |
| 枚举成员 | camelCase | `.idle`, `.walking`, `.fullWatch` |
| 协议 | 形容词 -able/-ing / 名词 | `SocialSyncProtocol`, `Observable` |
| SwiftUI View | 名词 + View | `CatView`, `BlindBoxView` |
| ViewModel | 名词 + ViewModel | `CatViewModel`, `SocialViewModel` |

### Structure Patterns

#### 测试文件位置

| 端 | 位置 | 示例 |
|---|------|------|
| Go | 同目录 `_test.go` | `service/auth_service_test.go` |
| Swift Shared | `CatShared/Tests/` | `Tests/CatSharedTests/APIClientTests.swift` |
| Swift App | 各 target 的 Tests group | `CatWatchTests/CatStateMachineTests.swift` |

#### 组件组织方式

- **Go:** 按技术层（handler/service/repository），不按功能域
- **Swift Watch/Phone:** 按类型（Views/ViewModels/Core），不按功能域
- **阈值:** 单目录超过 15 个文件时考虑按功能域拆分子目录

### Format Patterns

#### API 响应格式

**成功响应（直接返回数据，不包装）：**
```json
{
  "friends": [
    {"user_id": "abc", "cat_state": "walking", "last_active_at": "2026-03-27T10:30:00Z"}
  ]
}
```

**错误响应（统一包装）：**
```json
{
  "error": {
    "code": "SEQUENCE_CONFLICT",
    "message": "序列位置冲突",
    "details": {}
  }
}
```

**分页响应（cursor-based）：**
```json
{
  "items": [],
  "next_cursor": "eyJ...",
  "has_more": true
}
```

#### 数据交换格式

| 规则 | 约定 |
|------|------|
| JSON 字段 | snake_case（Go json tag + Swift CodingKeys） |
| 日期时间 | ISO 8601 字符串 `2026-03-27T10:30:00Z`（UTC） |
| 布尔值 | true/false（不用 1/0） |
| 空值 | JSON `null`，Go 用指针类型，Swift 用 Optional |
| ID 格式 | 字符串 UUID（不用整数，防枚举攻击） |
| 金额/积分 | 整数（步数、积分），不用浮点数 |

### Communication Patterns

#### 状态管理

**Swift ViewModel 统一模式：**
```swift
@Observable
class SomeViewModel {
    var items: [Item] = []
    var isLoading = false
    var error: AppError?

    private let service: SomeService

    init(service: SomeService) {
        self.service = service
    }

    func load() async { ... }
}
```

- 所有 ViewModel 用 `@Observable` 宏（不用 ObservableObject + @Published）
- 加载状态用 `isLoading` 布尔
- 错误用 `error: AppError?`，统一错误类型
- 依赖通过 init 注入

#### 事件/通知命名

| 场景 | 模式 | 示例 |
|------|------|------|
| CatStateMachine 状态变化 | Combine Publisher | `statePublisher: AnyPublisher<CatState, Never>` |
| 网络状态变化 | Combine Publisher | `reachabilityPublisher: AnyPublisher<Bool, Never>` |
| WatchConnectivity 消息 | 结构化 key：`cat.{domain}.{action}` | `cat.skin.configUpdate`, `cat.blindbox.sync` |
| Go 端内部通信 | channel 或直接调用 | 不引入事件总线框架 |

### Process Patterns

#### 错误处理

**Swift 端统一错误类型：**
```swift
enum AppError: Error {
    case network(URLError)
    case api(APIErrorResponse)
    case persistence(Error)
    case unauthorized
    case offline
}
```

**Go 端统一错误响应：**
```go
func respondError(c *gin.Context, status int, code string, message string) {
    c.JSON(status, gin.H{
        "error": gin.H{"code": code, "message": message},
    })
}
```

Service 层定义 sentinel error（`ErrSequenceConflict` 等），Handler 层映射为 HTTP 状态码 + 业务错误码。

#### 加载状态

| 场景 | 模式 |
|------|------|
| 首次加载 | `isLoading = true` + ProgressView |
| 刷新 | 不显示全屏 loading，数据到达后直接替换 |
| 离线 | 显示本地缓存 + 顶部"离线模式"横幅 |
| 错误 | 显示本地缓存 + 底部 toast 提示 |

#### 重试策略

| 场景 | 策略 |
|------|------|
| 网络请求失败 | 指数退避：1s → 2s → 4s，最多 3 次 |
| JWT 过期 | 自动 Refresh Token 刷新，失败 → 重新登录 |
| WC 消息失败 | 降级到 transferUserInfo（不重试 sendMessage） |
| 盲盒同步冲突 | 不重试，服务端自动补偿 |

### Enforcement Guidelines

**所有 Claude 编码会话必须遵守：**

1. 数据库表名复数 snake_case，不允许 PascalCase 或单数
2. API JSON 字段 snake_case，不允许 camelCase
3. 成功响应直接返回数据，不允许 `{data: ..., success: true}` 包装
4. 错误响应统一 `{error: {code, message}}` 格式
5. Swift ViewModel 用 `@Observable`，不允许 `ObservableObject`
6. ID 用字符串 UUID，不允许自增整数
7. 时间用 ISO 8601 UTC 字符串，不允许 Unix timestamp
8. Go 文件名 snake_case，Swift 文件名 PascalCase
9. 每个新 API 端点必须同时创建 Go DTO + Swift Codable 模型
10. WatchConnectivity 消息 key 用 `cat.{domain}.{action}` 格式

## Project Structure & Boundaries

### FR → 目录映射

| PRD 子系统 | Watch 端 | iPhone 端 | 后端 | 共享 |
|-----------|---------|-----------|------|------|
| 猫动画 FR1-5 | Scenes/CatScene,CatNode; Core/CatStateMachine,SensorManager; Views/AlwaysOnView | — | handler/user | Models/CatState |
| 盲盒 FR6-11 | Views/BlindBoxView,CrownUnlockView; VMs/BlindBoxVM; Scenes/BlindBoxScene | — | handler/blindbox; service/blindbox,sequence; model/blindbox,gift_sequence | Models/BlindBox |
| 签到 FR12-16 | Views/CheckInView; VMs/CheckInVM | — | handler/checkin; service/checkin; model/checkin | — |
| 皮肤 FR17-22 | Core/SkinCacheManager | Views/SkinGalleryView; VMs/SkinVM; Scenes/PreviewCatScene | handler/skin; service/skin; model/skin | Models/SkinConfig |
| 社交 FR23-28 | VMs/SocialVM; Scenes/FriendCatNode,SystemCatNode | Views/FriendsView; VMs/FriendVM; DeepLinkHandler | handler/friend; service/friend; model/friendship | Models/FriendCapability |
| 触觉 FR29-32 | Core/HapticManager; VMs/SocialVM | — | handler/touch; service/touch; push/apns | — |
| Complication FR33-35 | Complication/ComplicationProvider | — | — | Models/CatState |
| iPhone FR36-41 | — | Views/Home,Settings,Login,ShareCard; VMs/Auth,Store | handler/auth; service/auth | Networking/APIClient,WCManager |
| 运维 FR42-45 | — | — | cron/audit,daily_stats; handler/admin | — |
| 安全 FR46-55 | — | — | middleware/auth,rate_limiter; pkg/jwt | — |
| 统计 FR56-63 | — | — | handler/stats; cron/daily_stats | — |

### 需要端到端调用链的高复杂度 FR

以下 7 个 FR 跨越 3+ 层，在 Story 阶段必须附带完整调用链：

| FR | 描述 | 跨越 |
|----|------|------|
| FR8 | 离线开奖 + 联网校准 | Watch 本地 → 持久化 → 联网 → 服务端冲突 |
| FR20 | 皮肤同步 iPhone→Watch | iPhone VM → WCManager → Watch SyncCoordinator → SkinCacheManager |
| FR24 | 邀请链接自动配对 | Universal Link → DeepLinkHandler → API → 双方好友更新 |
| FR28 | 触碰发送全链路 | Watch Scene → API → APNs → 对方 Watch |
| FR43 | 序列冲突校准 | Watch 上报 → 服务端检测 → 补偿 → Watch 更新 |
| FR58 | 设备更换数据恢复 | 新设备 Sign in → 服务端拉取 → 本地重建 |
| FR60 | 低电量降级全链路 | EnergyBudgetManager → 所有 Core 模块档位切换 |

### Complete Project Directory Structure

```
cat/
├── .github/
│   └── workflows/
│       ├── server-ci.yml              # Go: build + test + 生成 fixtures
│       ├── ios-ci.yml                 # iOS: build + test (消费 fixtures)
│       └── assets-ci.yml             # 资产: generate-manifest + 校验 + CDN 上传
├── .gitignore                         # 含 .env.production, api/gen/
├── .gitattributes                     # Git LFS: *.png, *.jpg, *.atlas
│
├── ios/
│   ├── Cat.xcworkspace
│   ├── Cat.xcodeproj
│   │
│   ├── CatWatch/                      # ── watchOS App Target ──
│   │   ├── App/
│   │   │   └── CatWatchApp.swift      # @main + 推送处理 + 初始化
│   │   ├── Views/
│   │   │   ├── CatView.swift          # 主猫界面（SpriteView 容器）
│   │   │   ├── AlwaysOnView.swift     # AOD 纯 SwiftUI 静态渲染（独立路径）
│   │   │   ├── BlindBoxView.swift     # 盲盒展示 + 开奖动画
│   │   │   ├── CrownUnlockView.swift  # Digital Crown 解锁交互（FR7）
│   │   │   ├── CheckInView.swift      # 签到界面
│   │   │   ├── QuickSkinView.swift    # 快速换装覆盖层
│   │   │   ├── FriendListView.swift   # 好友列表
│   │   │   └── DailySummaryView.swift # 日摘要
│   │   ├── Scenes/
│   │   │   ├── CatScene.swift         # SpriteKit 主场景
│   │   │   ├── CatNode.swift          # 猫精灵节点（5层渲染）
│   │   │   ├── FriendCatNode.swift    # 好友猫（半分辨率）
│   │   │   ├── SystemCatNode.swift    # 系统 NPC 猫（FR59）
│   │   │   └── BlindBoxScene.swift    # 盲盒开奖动画
│   │   ├── ViewModels/
│   │   │   ├── CatViewModel.swift     # 猫状态 + 动画 + 副作用触发
│   │   │   ├── BlindBoxViewModel.swift
│   │   │   ├── CheckInViewModel.swift # 含签到天数计数（FR54 引导预留）
│   │   │   └── SocialViewModel.swift  # 好友状态 + 触碰 + 系统猫判断
│   │   ├── Core/
│   │   │   ├── CatStateMachine.swift  # 中央事件总线（纯发布，无副作用）
│   │   │   ├── EnergyBudgetManager.swift # 4 档位 + Protocol 配置注入
│   │   │   ├── SyncCoordinator.swift  # 优先级队列 + 并发限制
│   │   │   ├── SensorManager.swift    # 接受 SensorConfiguration protocol
│   │   │   ├── HapticManager.swift
│   │   │   └── SkinCacheManager.swift # LRU + 钉住活跃 + 原子下载 + SHA-256
│   │   ├── Complication/
│   │   │   └── ComplicationProvider.swift
│   │   ├── Resources/
│   │   │   └── Assets.xcassets
│   │   └── Info.plist
│   │
│   ├── CatPhone/                      # ── iPhone App Target ──
│   │   ├── App/
│   │   │   └── CatPhoneApp.swift      # @main + .onOpenURL(DeepLinkHandler)
│   │   ├── Views/
│   │   │   ├── HomeView.swift
│   │   │   ├── LoginView.swift
│   │   │   ├── SkinGalleryView.swift  # 含套装彩蛋检测（FR62）
│   │   │   ├── FriendsView.swift
│   │   │   ├── ShareCardView.swift    # 邀请分享卡片生成（FR53）
│   │   │   ├── StoreView.swift        # 内购（Growth）
│   │   │   └── SettingsView.swift     # 含免打扰时段 + 版本兼容提示
│   │   ├── ViewModels/
│   │   │   ├── AuthViewModel.swift
│   │   │   ├── SkinViewModel.swift    # 含套装匹配逻辑
│   │   │   ├── FriendViewModel.swift  # 订阅 DeepLinkHandler 邀请事件
│   │   │   └── StoreViewModel.swift
│   │   ├── Scenes/
│   │   │   └── PreviewCatScene.swift  # 皮肤动态预览（FR19）
│   │   ├── DeepLink/
│   │   │   └── DeepLinkHandler.swift  # URL 解析 + Combine 事件发布（不持有 VM）
│   │   ├── Resources/
│   │   │   └── Assets.xcassets
│   │   └── Info.plist
│   │
│   ├── CatShared/                     # ── 本地 Swift Package ──
│   │   ├── Package.swift
│   │   ├── Sources/CatShared/
│   │   │   ├── Models/
│   │   │   │   ├── User.swift
│   │   │   │   ├── CatState.swift
│   │   │   │   ├── SkinConfig.swift
│   │   │   │   ├── BlindBox.swift
│   │   │   │   ├── GiftSequence.swift
│   │   │   │   ├── Friendship.swift
│   │   │   │   ├── FriendCapability.swift
│   │   │   │   ├── CheckIn.swift
│   │   │   │   └── APIError.swift     # AppError enum
│   │   │   ├── Networking/
│   │   │   │   ├── APIClient.swift
│   │   │   │   ├── APIEndpoints.swift
│   │   │   │   ├── WCManager.swift    # 三通道瀑布 + 应用层 ACK + 版本检测
│   │   │   │   └── Environment.swift
│   │   │   ├── Persistence/
│   │   │   │   └── LocalStore.swift   # 混合持久化 + isFirstLaunch + 纯本地原则
│   │   │   └── Utilities/
│   │   │       └── NetworkReachability.swift
│   │   └── Tests/CatSharedTests/
│   │       ├── APIClientTests.swift
│   │       ├── WCManagerTests.swift
│   │       ├── LocalStoreTests.swift
│   │       └── DTOConsistencyTests.swift  # 消费 fixtures/ JSON
│   │
│   ├── CatWatchTests/
│   │   ├── CatStateMachineTests.swift
│   │   ├── EnergyBudgetManagerTests.swift
│   │   └── SyncCoordinatorTests.swift
│   └── CatPhoneTests/
│       ├── AuthViewModelTests.swift
│       └── SkinViewModelTests.swift
│
├── server/                            # ── Go 后端 ──
│   ├── cmd/server/
│   │   └── main.go                    # initDB → initServices → initRouter → Run
│   ├── internal/
│   │   ├── config/
│   │   │   └── config.go
│   │   ├── middleware/
│   │   │   ├── auth.go               # JWT + device_id 绑定校验
│   │   │   ├── rate_limiter.go       # Redis 滑动窗口 + 发送端/接收端双向限流
│   │   │   ├── cors.go
│   │   │   └── logger.go             # zerolog request_id + user_id
│   │   ├── handler/
│   │   │   ├── auth.go               # 含二次验证（破坏性操作）
│   │   │   ├── user.go
│   │   │   ├── blindbox.go           # 只接受 position，不接受步数
│   │   │   ├── checkin.go
│   │   │   ├── skin.go
│   │   │   ├── friend.go
│   │   │   ├── touch.go              # 含 DND 检查 + 接收端限流
│   │   │   ├── stats.go
│   │   │   ├── admin.go              # 含 debug/user/:id 调试端点
│   │   │   ├── health.go             # PG/Redis 状态 + goroutine 计数
│   │   │   └── ws.go                 # Growth 阶段
│   │   ├── service/
│   │   │   ├── auth_service.go
│   │   │   ├── blindbox_service.go
│   │   │   ├── sequence_service.go   # 一次性 token + position 严格校验
│   │   │   ├── skin_service.go
│   │   │   ├── friend_service.go     # 用 touch_repo 查询（不调 touch_service）
│   │   │   ├── touch_service.go
│   │   │   ├── checkin_service.go
│   │   │   └── stats_service.go
│   │   ├── repository/              # 所有查询强制带 user_id（防 IDOR）
│   │   │   ├── user_repo.go
│   │   │   ├── blindbox_repo.go
│   │   │   ├── skin_repo.go
│   │   │   ├── friend_repo.go
│   │   │   ├── checkin_repo.go
│   │   │   └── touch_repo.go
│   │   ├── model/
│   │   │   ├── user.go               # 含 dnd_start/dnd_end/device_id
│   │   │   ├── cat.go
│   │   │   ├── skin.go
│   │   │   ├── friendship.go         # 含 capability 字段
│   │   │   ├── blindbox.go
│   │   │   ├── gift_sequence.go      # 含 version + encrypted token
│   │   │   ├── checkin.go
│   │   │   └── touch_event.go
│   │   ├── ws/
│   │   │   ├── hub.go
│   │   │   ├── client.go
│   │   │   └── room.go
│   │   ├── push/
│   │   │   └── apns.go              # 含专注模式行为注释
│   │   ├── cron/
│   │   │   ├── daily_stats.go
│   │   │   └── audit.go
│   │   └── dto/
│   │       ├── auth_dto.go
│   │       ├── blindbox_dto.go
│   │       ├── skin_dto.go
│   │       ├── friend_dto.go
│   │       ├── touch_dto.go
│   │       ├── error_dto.go
│   │       └── dto_test.go           # 生成 fixtures/ JSON 样本
│   ├── pkg/
│   │   ├── jwt/
│   │   │   └── jwt.go               # 双密钥轮换 + device_id binding
│   │   ├── redis/
│   │   │   └── redis.go             # 带密码连接
│   │   └── validator/
│   │       └── validator.go
│   ├── migrations/
│   ├── deploy/
│   │   ├── Dockerfile
│   │   ├── docker-compose.yml        # PG/Redis 只绑 127.0.0.1 + 设密码
│   │   └── nginx.conf
│   ├── .env.development
│   ├── .env.staging
│   ├── go.mod
│   └── go.sum
│
├── api/
│   └── proto/
│       ├── friend_status.proto
│       ├── touch_event.proto
│       ├── cat_state.proto
│       └── room.proto
│
├── assets/                            # Git LFS
│   ├── sprites/                       # 命名规范: {layer}/{name}_{frame}.png
│   │   ├── body/
│   │   ├── expression/
│   │   ├── outfit/
│   │   ├── headwear/
│   │   └── accessory/
│   ├── complication/
│   │   ├── rectangular/
│   │   └── circular/
│   ├── effects/
│   ├── ui/
│   └── manifest.json                  # 由 generate-manifest.sh 自动生成
│
├── fixtures/                          # DTO 一致性测试 JSON 样本
│   ├── friend_status.json
│   ├── blindbox_response.json
│   ├── skin_config.json
│   └── ...
│
├── scripts/
│   ├── generate-manifest.sh           # 扫描 assets/ 自动生成 manifest.json
│   ├── migrate.sh
│   ├── seed.sh
│   ├── deploy.sh
│   └── asset-upload.sh
│
└── docs/
    └── api-endpoints.md

```

### Architectural Boundaries

#### 层级依赖规则

**Go 后端：**
- Handler → Service → Repository → Model（严格单向）
- Handler 禁止直接调用 Repository
- Service 间单向调用可以，**双向禁止**（如需反向数据，降级为直接用 Repository）
- Repository 只做 CRUD，不含业务逻辑
- Redis 操作在 Service 层发起
- Repository 层所有查询**强制带 user_id**（防 IDOR）

**Swift Watch 端：**
- Views → ViewModels → Core / CatShared（单向）
- Scenes → Core（单向）
- Core 模块间**禁止直接双向引用**，反向需求通过 Protocol 注入
- CatStateMachine **纯发布原则**：只发布状态变化，不触发任何副作用
- DeepLinkHandler **事件发布原则**：只解析 URL 并发布 Combine 事件，不持有 ViewModel
- LocalStore **纯本地原则**：只做本地读写，不触发网络请求
- CatShared 不依赖任何 App Target 代码

#### Protocol 抽象注入（防循环依赖）

| Protocol | 提供者 | 消费者 | 用途 |
|----------|--------|--------|------|
| `SensorConfiguration` | EnergyBudgetManager | SensorManager | 采样率配置 |
| `RenderConfiguration` | EnergyBudgetManager | CatScene | 目标帧率 |
| `PollingConfiguration` | EnergyBudgetManager | SocialViewModel | 轮询间隔 |
| `SyncAvailability` | SyncCoordinator | ViewModels | "能否同步"查询 |

#### 三端通信边界

- Watch 可直接调后端 API（不依赖 iPhone 中转）
- iPhone 通过 WC 向 Watch 推送皮肤配置变更
- APNs 是后端→Watch 的推送通道
- 好友状态同步增加 ±30秒随机延迟（隐私模糊化）

#### 数据边界

| 数据类型 | 权威源 | Watch 本地 | iPhone 本地 |
|---------|--------|-----------|------------|
| 用户账户 | PostgreSQL | JWT (Keychain) | JWT (Keychain) |
| 猫状态 | Watch 传感器 | CatStateMachine | — |
| 盲盒/序列 | PostgreSQL | SwiftData+UserDefaults | — |
| 皮肤库存 | PostgreSQL | SwiftData(缓存) | SwiftData(缓存) |
| 皮肤穿戴 | PostgreSQL | SwiftData | SwiftData |
| 皮肤纹理 | CDN | 文件系统(LRU) | 文件系统(LRU) |
| 好友状态 | Redis(TTL120s) | 内存 | — |
| 步数 | CMPedometer | UserDefaults | — |
| 健康数据 | HealthKit | 不出端 | 不出端 |

#### 安全边界

**JWT 加固：**
- payload 含 device_id，服务端校验来源设备
- 破坏性操作（删除账号、解除配对）需二次 Sign in with Apple
- 同一账号最多 2 个活跃会话（1 Watch + 1 iPhone）

**序列化礼物加固：**
- 不下发明文皮肤 ID，使用一次性加密 token
- position 严格校验：上报 = 服务端记录 + 1
- 开奖顺序：先持久化 position，后展示动画

**社交安全：**
- 触碰双向限流：发送端 6次/分 + 接收端 20次/小时
- 邀请码 UUID v4（128位随机）
- 举报 MVP 不自动封禁，人工审核

**基础设施安全：**
- Redis/PostgreSQL 必须设密码
- Docker Compose 数据库端口只绑 127.0.0.1
- manifest.json 从服务端 API 获取（HTTPS），不从 CDN
- 皮肤资源 SHA-256 校验 + 单文件上限 5MB
- 盲盒 API 不接受/不存储步数数据

### 外部集成点

| 外部服务 | 集成文件 | 用途 |
|---------|---------|------|
| Apple ID | service/auth_service.go | Sign in with Apple Token 验证 |
| APNs | push/apns.go | 触碰推送 + 静默同步推送 |
| CDN | service/skin_service.go | 皮肤资源 URL 签名 |
| Firebase Crashlytics | CatWatchApp + CatPhoneApp | 崩溃收集 |
| HealthKit | Core/SensorManager.swift | 步数兜底 |
| StoreKit | VMs/StoreViewModel.swift | 内购（仅 iPhone） |

### 美术资源工作流

**设计师命名规范：** `assets/sprites/{layer}/{name}_{frame}.png`
- 示例：`outfit/chef_01.png`, `headwear/chef_hat_01.png`
- `scripts/generate-manifest.sh` 自动扫描生成 manifest.json
- CI 校验尺寸、帧数、命名格式，校验失败阻止合并
- 设计师不碰 JSON/代码

### DTO 一致性测试

- Go 测试生成 `fixtures/*.json` 样本
- Swift 测试消费 `fixtures/*.json` 验证 Codable 解析
- CI 顺序：Go test（生成 fixtures）→ Swift test（消费 fixtures）
- 任何字段不匹配直接红

### 运维调试工具

**Admin Debug 端点（MVP Day 1）：**
`GET /v1/admin/debug/user/:user_id` — 返回用户完整状态快照（猫状态、Redis 在线、好友、序列位置、待同步数）

**Health 增强：**
`GET /health` — 含 PostgreSQL/Redis 连接状态 + goroutine 计数 + uptime

## Architecture Validation Results

### Coherence Validation ✅

**Decision Compatibility:** 10 组关键决策交叉验证，全部兼容。无矛盾。
**Pattern Consistency:** 命名链路（DB snake_case → JSON snake_case → Go camelCase → Swift camelCase）一致。错误处理链路（Go sentinel → HTTP code → Swift AppError）完整。
**Structure Alignment:** 12 个 Cross-Cutting Concerns + 4 个涌现组件全部有对应文件/模块承接。

### Requirements Coverage ✅

**63 个 FR：** 100% 覆盖（55 个初始映射 + 8 个 Gap Analysis 修复）
**28 个 NFR：** 全部有架构支撑（Performance / Security / Scalability / Accessibility / Reliability）

### Implementation Readiness ✅

- 12 条核心决策含版本号和 Rationale
- 10 条 Enforcement Guidelines 可机器检查
- ~85 个具体文件的完整目录树
- 16 条安全加固 + 5 条依赖规则

### MVP 分层实施计划

**Layer 0（Day 1）：猫能动起来**
- Xcode 工程 + CatShared Package + `go mod init`
- CatStateMachine + SensorManager（含权限请求+降级体验）
- CatScene + CatNode（基础 4 状态，用占位帧）
- CatView + AlwaysOnView
- LocalStore（UserDefaults 部分：isFirstLaunch + 状态机状态）
- Go: main.go + config + PG + 第一个迁移（users）+ auth handler/service + JWT
- **验证标准：** 真机上猫跟随走路/静坐切换

**Layer 1（Week 1-2）：单机核心循环**
- CatShared/Networking: APIClient + Endpoints + Environment + NetworkReachability
- 盲盒全链路：BlindBoxView + VM + Scene + handler/service/repo + sequence_service
- 签到全链路：CheckInView + VM + handler/service/repo
- 皮肤全链路：SkinCacheManager + SkinGalleryView(iPhone) + PreviewCatScene + handler/service
- LocalStore 完整版（SwiftData + UserDefaults + 文件系统）
- SyncCoordinator 简化版（JWT 刷新 + 盲盒同步 + 签到同步）
- HapticManager（盲盒开奖震动）
- Go: Redis + Docker Compose（PG + Redis + Server）
- **验证标准：** 断网下盲盒+签到可用，联网后同步成功

**Layer 2（Week 2-3）：社交功能**
- 好友系统：FriendsView + FriendVM + DeepLinkHandler + handler/service
- 触碰社交：SocialViewModel + touch handler/service + APNs
- 好友猫同屏：FriendCatNode + SystemCatNode（FR59）
- EnergyBudgetManager（社交轮询上线后需要电量管理）
- 限流中间件（rate_limiter.go）
- WCManager 三通道瀑布
- Admin Debug 端点 + /health 增强（排查社交问题需要）
- **验证标准：** 两台真机配对 + 触碰发送/接收 + 好友猫同屏

**Layer 3（Week 3-4）：加固与打磨**
- iPhone App 完善：ShareCardView + SettingsView(DND/版本兼容) + StoreView 骨架
- 安全加固：device_id binding、二次验证、接收端限流
- Complication（accessoryRectangular + accessoryCircular）
- DTO 一致性测试 CI
- CrownUnlockView（FR7 替代解锁）
- Protocol 抽象注入（如果 Layer 2 发现了循环依赖）
- **验证标准：** 全链路真机测试 + CI 全绿

### Growth 推迟功能及触发条件

| 推迟项 | 触发条件 | 理由 |
|--------|---------|------|
| 序列加密 token | DAU > 5,000 或发现序列被预览 | 用户量小时无攻击动机 |
| 状态 ±30s 随机延迟 | DAU > 10,000 或收到隐私投诉 | 小社区内隐私风险低 |
| Protocol 抽象注入 | 代码中出现循环依赖编译错误 | 不预防假想问题 |
| EnergyBudgetManager 4 档位 | 真机测试电量 >12% | 先测量再优化 |
| WebSocket 替代轮询 | DAU > 10,000 或服务端 CPU >60% | HTTP 轮询 MVP 够用 |
| Grafana + Prometheus | DAU > 10,000 | MVP 用日摘要脚本 |
| PostgreSQL 读写分离 | 数据库 CPU >70% | 单实例 MVP 够用 |

### Testing Strategy

#### 覆盖率目标

| 层级 | 要求 | 覆盖率 |
|------|------|--------|
| Go Service | 每个 public 方法单元测试 | ≥ 80% |
| Go Handler | happy path + 每种 HTTP 状态码至少 1 case | ≥ 70% |
| Go Middleware | auth + rate_limiter 必须测试 | ≥ 90% |
| Go Repository | 不单独测（通过 Service 覆盖） | — |
| Swift Core | 涌现组件（CatStateMachine, SyncCoordinator, EnergyBudget） | ≥ 90% |
| Swift ViewModel | 关键业务逻辑（盲盒, 签到, 触碰） | ≥ 70% |
| Swift View | 不测试（SwiftUI Preview 替代） | — |
| DTO 一致性 | 每个 API 端点的请求/响应 fixture | 100% |

#### 测试节奏

**每个 Story 完成时（同 PR）：**
1. Service 层单元测试
2. Handler 至少 happy path 测试
3. 新 API 端点的 fixture JSON

**每个 Sprint 结束时补全：**
4. Handler error path 覆盖
5. Middleware 测试
6. Swift ViewModel 测试

#### 端到端测试（Growth 引入）

Go test server（SQLite in-memory + mock Redis）→ 注册测试用户 → 配对 → 触碰 → 验证 DB。Mock APNs，不测 Swift 端（CI 无 watchOS 模拟器）。

### Architecture Completeness Checklist

**✅ Requirements Analysis**
- [x] 63 FR + 28 NFR 全部分析
- [x] 12 Cross-Cutting Concerns + 4 涌现组件
- [x] Pre-mortem 5 项 + Failure Mode 8 项 + Graph of Thoughts 3 项

**✅ Architectural Decisions**
- [x] 12 条核心决策含版本号
- [x] 数据架构 + 认证安全 + API 通信 + 基础设施

**✅ Implementation Patterns**
- [x] 32 个冲突点 → 10 条 Enforcement Guidelines
- [x] 命名 / 格式 / 通信 / 流程 4 类模式
- [x] 测试策略 + 覆盖率目标 + 测试节奏

**✅ Project Structure**
- [x] ~85 文件完整目录树 + FR 映射
- [x] 依赖规则 + Protocol 抽象
- [x] 安全边界 16 条 + 美术资源工作流

**✅ Validation**
- [x] 决策兼容性 + 模式一致性 + 结构对齐
- [x] MVP 分层实施 Layer 0-3
- [x] Growth 推迟 7 项含触发条件

### Architecture Readiness Assessment

**Overall Status: ✅ READY FOR IMPLEMENTATION**
**Confidence Level: HIGH**

**Key Strengths:**
1. 离线优先架构全面定义——断网时核心体验 100% 可用
2. 电量管理从架构层统一调度
3. 三端同步有编排器 + 优先级队列
4. 安全经过 Red Team 攻防验证
5. 依赖方向经过审计，Protocol 防循环
6. MVP 分层实施，Day 1 只需 ~20 文件 + 5 条核心规则

**Implementation Handoff:**

所有 Claude 编码会话必须遵守：
1. 10 条 Enforcement Guidelines
2. 新 API 端点同时创建 Go DTO + Swift Codable + fixture JSON
3. Core 模块间禁止双向引用
4. CatStateMachine 纯发布，副作用由 ViewModel 触发
5. Repository 查询强制带 user_id
6. 按 Layer 0-3 顺序实施

**First Implementation Priority (Layer 0):**
1. Xcode "iOS App with Watch App" + CatShared Package
2. `go mod init` + 依赖安装 + users 迁移
3. JWT 认证 + Sign in with Apple
4. CatStateMachine + SensorManager + CatScene 骨架
5. 真机验证：猫跟随走路/静坐切换
