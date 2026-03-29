# 裤衩猫 (Cat)

Apple Watch 触觉社交宠物 App。手腕上住着一只猫，实时映射你的运动状态，点击好友的猫发送手腕震动。

## 目录结构

```
cat/
├── ios/                    # Apple 客户端（Xcode Monorepo）
│   ├── CatWatch/           # watchOS App Target
│   │   ├── App/            # @main 入口、推送处理、初始化
│   │   ├── Views/          # SwiftUI 视图（盲盒、签到、换装等）
│   │   ├── Scenes/         # SpriteKit 场景（猫渲染、好友猫、盲盒动画）
│   │   ├── ViewModels/     # MVVM ViewModel 层
│   │   ├── Core/           # 核心模块（状态机、电量管理、触觉、皮肤缓存、同步）
│   │   ├── Complication/   # 表盘 Complication（WidgetKit）
│   │   └── Resources/      # Assets.xcassets 等资源
│   ├── CatPhone/           # iPhone 伴侣 App Target
│   │   ├── App/            # @main 入口、深度链接处理
│   │   ├── Views/          # 首页、皮肤库、好友管理、设置等
│   │   ├── ViewModels/     # 认证、皮肤、好友、内购 ViewModel
│   │   ├── Scenes/         # 皮肤动态预览（SpriteKit 嵌入）
│   │   ├── DeepLink/       # URL 解析、邀请配对处理
│   │   └── Resources/      # Assets.xcassets 等资源
│   ├── CatShared/          # 本地 Swift Package（双端共享）
│   │   └── Sources/CatShared/
│   │       ├── Models/     # 数据模型（User、CatState、Skin、BlindBox 等）
│   │       ├── Networking/  # APIClient、WatchConnectivity、环境配置
│   │       ├── Persistence/ # 混合持久化（SwiftData + UserDefaults + 文件缓存）
│   │       └── Utilities/   # 网络可达性等工具
│   ├── CatWatchTests/      # 手表端单元测试
│   └── CatPhoneTests/      # iPhone 端单元测试
│
├── server/                 # Go 后端服务
│   ├── cmd/server/         # main.go 入口
│   ├── internal/
│   │   ├── config/         # 环境配置加载
│   │   ├── middleware/     # JWT 认证、限流、CORS、日志
│   │   ├── handler/        # HTTP 路由处理（Gin）
│   │   ├── service/        # 业务逻辑层
│   │   ├── repository/     # 数据访问层（GORM + PostgreSQL）
│   │   ├── model/          # 数据库模型
│   │   ├── dto/            # 请求/响应数据传输对象
│   │   ├── ws/             # WebSocket（Growth 阶段）
│   │   ├── push/           # APNs 推送
│   │   └── cron/           # 定时任务（每日摘要、盲盒审计）
│   ├── pkg/                # 可复用工具包
│   │   ├── jwt/            # JWT 双密钥轮换
│   │   ├── redis/          # Redis 客户端封装
│   │   └── validator/      # 请求参数校验
│   ├── migrations/         # 数据库迁移文件（golang-migrate）
│   └── deploy/             # Dockerfile、docker-compose、nginx
│
├── api/proto/              # Protobuf 定义（Growth 阶段 WebSocket 消息）
│
├── assets/                 # 美术资源（Git LFS）
│   ├── sprites/            # 猫皮肤精灵图（按层分目录）
│   │   ├── body/           # 身体层（白/橘/灰）
│   │   ├── expression/     # 表情层
│   │   ├── outfit/         # 服装层
│   │   ├── headwear/       # 头饰层
│   │   └── accessory/      # 配件层
│   ├── complication/       # 表盘插画（rectangular / circular）
│   ├── effects/            # 粒子特效
│   ├── ui/                 # UI 图标素材
│   └── manifest.json       # 资源清单（脚本自动生成）
│
├── fixtures/               # DTO 一致性测试 JSON（Go 生成，Swift 消费）
├── scripts/                # 构建/部署/迁移脚本
├── docs/                   # API 文档等
└── .github/workflows/      # CI/CD（server / ios / assets 三管线）
```

## 技术栈

| 端 | 技术 |
|----|------|
| watchOS | Swift / SwiftUI / SpriteKit / WidgetKit |
| iPhone | Swift / SwiftUI / SpriteKit（预览） |
| 后端 | Go / Gin / GORM / PostgreSQL / Redis |
| 通信 | REST (MVP) / WebSocket (Growth) / APNs / WatchConnectivity |
| 部署 | Docker / Nginx / CDN |

## 开发环境

- **iOS/watchOS**: macOS + Xcode 15+ (watchOS 10+ / iOS 17+)
- **Go 后端**: 任意平台 + Go 1.22+ + Docker (PostgreSQL + Redis)

## 快速开始（后端）

```bash
cd server/deploy
docker-compose up -d          # 启动 PG + Redis
cd ..
go run ./cmd/server           # 启动服务
```
