---
stepsCompleted: [step-01-validate-prerequisites, step-02-design-epics, step-03-create-stories, step-04-final-validation]
inputDocuments: [prd.md, architecture.md, ux-design-specification.md]
---

# cat - Epic Breakdown

## Overview

This document provides the complete epic and story breakdown for cat, decomposing the requirements from the PRD, UX Design if it exists, and Architecture requirements into implementable stories.

## Requirements Inventory

### Functional Requirements

FR1: 用户可以在手表上看到一只跟随自身运动状态实时变化的猫（走路/跑步/静坐/睡觉）
FR2: 猫可以随机展现 2 种微行为动画（打哈欠、伸懒腰），Growth 阶段扩展到 5+ 种
FR3: 用户首次打开 App 时，猫通过"镜像时刻"让用户无需文字引导即可理解猫在跟随自己。验收标准：内测 10 名用户中 ≥8 名在首次打开后无提示自行描述出"猫在跟着我动"
FR4: 用户可以在手表上通过左右滑动在最近 5 套皮肤之间快速切换
FR5: 猫在 Always-On Display 模式下显示为静态简化状态图
FR6: 系统每 30 分钟自动掉落一个盲盒（单盲盒队列，当前盲盒解锁后才掉落下一个）
FR7: 用户可以通过累积 200-300 步解锁盲盒。若盲盒持续 2 小时未解锁，提供替代解锁方式（旋转 Digital Crown 完成小游戏）
FR8: 免费盲盒可在离线状态下解锁（客户端开奖，基于服务端下发的序列化礼物列表）；付费盲盒需联网解锁。采用序列化礼物制，非随机概率
FR9: 盲盒开启时展示获得的皮肤组件（猫举起展示 + 触觉反馈震动），存入库存不自动穿戴
FR10: 盲盒状态在 App 被系统终止后可以恢复（持久化存储）
FR11: App 重启后可以从 CMPedometer 补查丢失的步数数据
FR12: 用户每日首次打开 App 即自动完成签到，获得皮肤组件。签到动画作为仪式感展示（猫举牌），零压力设计
FR13: 签到采用月累计制（非连续制），断签不影响进度
FR14: 用户每月可以补签 2-3 次
FR15: 签到可在离线状态下完成，联网后自动同步
FR16: 签到从去重队列按序发放皮肤（不重复）
FR17: 用户可以在 iPhone App 上浏览自己拥有的全部皮肤库存
FR18: 用户可以在 iPhone App 上对猫进行多层自由搭配（5 层 z-order：身体/表情/服装/头饰/配件，MVP ≥30 件组件，组合 ≥50 种）
FR19: 用户在 iPhone App 皮肤预览中可以看到猫的动态效果（猫会动）
FR20: 用户在 iPhone App 确认搭配后，手表端自动同步更新猫的外观（恢复连接后 ≤10 秒完成同步）
FR21: 皮肤资源支持 CDN 按需下载，带版本号校验，服务端可标记撤回
FR22: 手表端本地缓存最多 10 套皮肤资源，超出按 LRU 淘汰
FR23: 用户可以在 iPhone App 生成邀请链接，通过系统分享面板发送给好友
FR24: 好友点击邀请链接后可以自动完成配对（下载安装后自动关联）
FR25: 配对后用户可以在手表屏幕上同时看到好友的猫（MVP 最多 2 好友 = 3 只猫同屏）
FR26: 前台期间 HTTP 轮询每 30 秒拉取好友状态，状态切换动画插值平滑过渡
FR27: 好友状态超过 2 分钟未更新时，好友的猫显示离线标记（半透明 + 💤）
FR28: 用户可以点击好友的猫发送一次触碰，双方手腕同时震动
FR29: 用户在 App 后台或未打开时也能通过推送接收好友触碰的震动
FR30: 用户可以对单个好友设置静音
FR31: 系统自动限制同一好友 10 分钟内最多推送 3 次触碰
FR32: 当 iPhone 处于专注模式时，触碰通知自动降级为静默
FR33: 用户可以在表盘上添加裤衩猫 Complication，显示猫当前状态的专用像素级插画
FR34: Complication 根据猫的运动状态切换不同的静态插画（3-5 种）
FR35: 用户点击 Complication 可直接打开裤衩猫 App
FR36: 用户可以通过 Sign in with Apple 登录
FR37: 用户可以管理好友列表（查看、删除、解除配对、屏蔽、举报），含非活跃自动标记
FR38: 用户可以调整通知偏好设置（好友触碰/盲盒掉落独立开关）
FR39: iPhone App 显示手表端皮肤同步状态
FR40: iPhone App 检测配对手表的系统版本，不兼容时显示友好提示
FR41: 用户可以在 iPhone App 查看个人简要统计（今日步数、盲盒数、皮肤收集数、触碰次数）
FR42: 管理员可以通过 CLI 工具上传新皮肤资源到 CDN 并更新皮肤池配置
FR43: 系统在 App 启动时下发最新序列化礼物列表，客户端缓存供离线使用。服务端维护序列消费位置为权威，离线消费联网后校准
FR44: 系统每日自动生成数据摘要
FR45: 系统对盲盒开奖结果进行事后审计，检测统计异常
FR46: 用户可以屏蔽任意好友（屏蔽后双方猫互不可见，触碰不可达）
FR47: 用户可以举报好友的不当行为
FR48: 用户可以设置免打扰时段
FR49: 用户可以在 iPhone App 中删除自己的账号和所有关联数据（30 天冷却期）
FR50: 皮肤 UI 中不显示稀有度等级，所有皮肤统一视觉样式
FR51: 盲盒掉落时系统发送本地通知并触发手腕震动提醒用户
FR52: 用户发送触碰后，自己手腕收到一次确认震动（表示已发送）
FR53: iPhone App 生成邀请时附带精美分享卡片，适合社交平台传播
FR54: 第 3 天签到时猫自然引导用户设置表盘 Complication（P1，延迟引导）
FR55: 系统追踪邀请链接的生成、点击、下载、配对完成全链路转化数据
FR56: 系统统计并汇总 DAU、配对率、皮肤收集数分布、Complication 使用率、邀请转化率、盲盒解锁率、触碰频率
FR57: 皮肤掉率随用户收集进度动态调整，目标中度活跃用户 60 天收集 80% 独立组件
FR58: 用户更换设备后通过 Sign in with Apple 登录即可恢复云端数据
FR59: 无好友用户可看到一只"系统猫"（随机行为的 NPC 猫）陪伴在旁
FR60: 手表电量 ≤20% 时自动进入低电量模式（停止 SpriteKit 动画，Complication 保持更新）
FR61: iPhone-only 好友可通过 iPhone App 查看好友猫状态、接收触碰推送通知、发送触碰
FR62: iPhone App 皮肤库显示收集进度条，凑齐同系列 ≥3 件触发"套装发现"彩蛋动画
FR63: 皮肤资源版本校验——客户端启动时比对本地缓存版本与服务端最新版本，撤回时自动清除

### NonFunctional Requirements

NFR1: 动画帧率 ≥ 24fps（抬腕交互时），非交互时可降至 15fps
NFR2: App 启动到猫出现 < 2 秒（冷启动）
NFR3: 触碰延迟 < 5 秒（用户发送触碰到对方收到震动）
NFR4: 皮肤快速换装响应 < 0.5 秒（本地缓存命中时）
NFR5: 全天使用额外耗电 ≤ 总电量 10%（动画≤5% + 网络≤3% + 传感器≤2%）
NFR6: 手表端 App 包体 ≤ 30MB，iPhone 端 ≤ 50MB
NFR7: 手表端运行时内存 ≤ 50MB
NFR8: 皮肤同步 ≤10 秒完成
NFR9: 所有网络通信使用 HTTPS/WSS（TLS 1.2+）
NFR10: JWT Token 认证，Access 7天 / Refresh 30天
NFR11: 健康数据仅本地处理，不上传原始健康数据
NFR12: 服务端仅存储聚合数据，不存储运动轨迹或详细健康记录
NFR13: 账号删除后 30 天内清除所有关联数据（GDPR + PIPL 合规）
NFR14: 好友之间仅可见猫的运动状态，不可见具体位置、步数、健康数据
NFR15: PIPL 合规——服务端部署中国大陆境内，数据不出境
NFR16: MVP 支持 5,000 DAU / 2,000 同时在线
NFR17: 架构可在 24 小时内扩容到 50,000 DAU / 20,000 同时在线
NFR18: 单 Go 实例支持 ≥ 10,000 WebSocket 并发连接
NFR19: CDN 皮肤资源单资源下载 < 3 秒
NFR20: PostgreSQL 单实例支撑 MVP，Growth 预留读写分离
NFR21: 每种触觉反馈都有对应的视觉动画反馈（无障碍）
NFR22: 遵循 watchOS Dynamic Type，支持系统字体大小调整
NFR23: 文字满足 WCAG AA 标准（4.5:1 对比度）
NFR24: 关键交互元素支持 VoiceOver 标签
NFR25: 后端 ≥ 99.5% 月可用率
NFR26: 手表端 + iPhone 端崩溃率 < 0.1%
NFR27: 离线操作联网后最终一致，冲突时以服务端为准
NFR28: 后端不可达时，单机核心体验 100% 可用（猫动画+盲盒+签到）

### Additional Requirements

- 架构采用 Monorepo（ios/ + server/ + assets/），Claude 编码需一个 repo 上下文
- Apple Watch + iPhone 端：Xcode "iOS App with Watch App" 模板初始化
- Go 后端：自定义项目结构（Gin + GORM + PostgreSQL + Redis + WebSocket + APNs）
- 手动构造函数依赖注入（零框架），main.go 显式构造依赖链
- Swift 端 MVVM + @Observable，CatShared 本地 Swift Package 共享模型/网络/持久化
- Go 端 Handler-Service-Repository 三层，ws/ 目录 MVP 建好空实现
- 数据库迁移：golang-migrate 手动迁移文件，不用 GORM AutoMigrate
- Swift 端混合持久化：SwiftData（结构化数据）+ UserDefaults（快速读写状态）+ 文件系统（皮肤纹理缓存）
- Redis Write-Through + TTL 分层缓存
- JWT 双密钥轮换（24 小时并行验证期）
- API 限流分级策略（Gin 中间件 + Redis 滑动窗口计数器）
- REST 错误响应标准格式（统一 `{error: {code, message}}` 包装）
- Growth 阶段 WebSocket 消息用 Protobuf；MVP HTTP 轮询用 JSON
- 手表导航 NavigationStack，SpriteKit ↔ SwiftUI 通过 CatStateMachine 桥接
- 涌现组件：CatStateMachine（中央事件总线）、EnergyBudgetManager（4 档位）、SyncCoordinator（编排器）、NetworkReachabilityManager
- CI/CD 三管线分流：server/ios/assets 独立触发
- 日志：zerolog 结构化 JSON 输出 stdout
- 监控 MVP：Crashlytics + /health + UptimeRobot + 每日摘要脚本
- 数据库备份：pg_dump 每日定时备份到对象存储
- 代码命名规范：DB snake_case、API JSON snake_case、Go snake_case 文件/Swift PascalCase 文件、ID 用 UUID 字符串
- 手动 DTO + CI 集成测试保持 Go/Swift 端 API 一致性
- WatchConnectivity 消息 key 格式 `cat.{domain}.{action}`
- 7 个高复杂度端到端调用链 FR（FR8/FR20/FR24/FR28/FR43/FR58/FR60）需在 Story 阶段附带完整调用链

### UX Design Requirements

UX-DR1: 猫"被抓到"过渡动画（0.3 秒），每次抬腕猫正在做自己的事，察觉后才转向用户
UX-DR2: 三层视觉架构实现——前景层（自己的猫 40% 屏幕）、背景层（好友猫 60×60 12fps 降饱和 -20% 透明度 85%）、隐藏层（换装/设置手势触发）
UX-DR3: CatSprite 拆为 6 子系统——CatAnimationController、CatSkinRenderer（5 层 z-order）、CatBreathingEffect（±0.5s 随机微动）、CatGlowEffect（5-8px 环境光晕）、CatGazeController（视线引导系统）、CatTimeShader（时段色相偏移）
UX-DR4: 猫视线引导系统——猫看向事件（盲盒/礼物）引导用户视线，猫不主动看用户（被抓到感），猫不看 UI 元素
UX-DR5: 盲盒开启两阶段 1.5 秒动画——猫一爪拍开→皮肤飞出猫接住展示，配合三连短促+长震触觉
UX-DR6: 晨间仪式——签到重包装为"猫叼礼物回来"，猫叼着礼物等用户，自动入库。过夜盲盒+签到合并为 3 秒动画
UX-DR7: 触碰动画——点击好友猫后自己的猫走过去碰一下（0.8 秒），好友猫睡觉时加 0.5 秒"嘘…"缓冲，好友离线时显示"ta 下次抬腕会感受到"
UX-DR8: 触觉语义系统——盲盒惊喜=三连短促+长震，好友触碰=双跳心跳，久坐提醒=缓慢渐强。HapticManager 统一管理：语义化 API、优先级队列、防疲劳（同类≤6次/小时）、降级策略
UX-DR9: 事件优先级排队系统——触碰接收 > 盲盒解锁 > 晨间礼物 > 盲盒掉落提示 > 好友状态更新。同时最多 1 活跃事件 + 1 角落提示
UX-DR10: 猫态替代系统 UI——加载中=猫趴着摆尾巴，网络错误=猫歪头+"？"，好友离线=半透明+💤，空皮肤库=猫站空衣柜前歪头，配对等待=猫左右张望，触碰失败=手绘"❌"1秒消失
UX-DR11: Design Tokens 实现——watchOS 克制策略（纯黑背景+深灰表面+米白文字），iOS 活泼策略（奶油白底+橘猫暖主色+柔粉社交色）。SF Pro Rounded 双平台统一字体
UX-DR12: Personality Tokens 实现——catResponseDelay 0.3-0.8s、eventAppearance 滑入 0.5s、eventDismissal 淡出 1-2s、touchAnimationDuration 0.8s、caughtTransition 0.3s、blindboxOpen 1.5s、giftPresent 2s
UX-DR13: 呼吸微动规范——idle 2.5-3.5s/次 1-2px、walking ~0.5s/步 2-3px、sleeping 3.5-4.5s/次 3-4px，好友猫 1px。关键：加 ±0.5s 随机偏移避免固定节奏
UX-DR14: 多猫同屏布局——1只居中偏下，2只左右分列（自己左，好友右），3只自己居中偏下+好友左上右上。各有独立点击区域不重叠，触摸目标 ≥44pt
UX-DR15: iPhone 皮肤搭配界面——SkinCard（静态 PNG 缩略图+新获得绿点+未获得半透明）、SkinPreview（SpriteView 1:1 手表模拟+猫会动）、LayerPicker（一次一层 Tab 式 5 层切换）
UX-DR16: iPhone 5 Tab 导航——首页/皮肤库/搭配/好友/设置，iOS 甜度上限规则每页最多一种暖色主角
UX-DR17: 无障碍——VoiceOver 标签（猫状态/好友猫/触碰按钮）、Reduce Motion 支持（取消过渡+直切状态）、色盲友好（状态通过动作/姿态区分不靠颜色）、触觉替代（关闭震动改视觉确认）
UX-DR18: SpriteKit 猫世界色彩规则——暖色基调、时段用色相偏移不降亮度、猫最低 HSB Brightness ≥60%、深色皮肤加粗轮廓+提亮光晕、猫环境光晕 5-8px、好友猫降饱和 -20%、获得粒子效果 2-3 个微粒子 1-2 秒
UX-DR19: 线条美术规范——基础线宽 2-3pt 带微弱手绘抖动、粗细呼吸（柔软处变细 1.5pt/结构处粗实 2.5pt）、填充不到边留 0.5pt 间隙、Complication 专用线条加粗 3-4pt
UX-DR20: iPhone App 按钮层级——Primary 填充橘猫暖、Secondary 灰色描边、Destructive 系统红文字、Text 品牌橘色链接
UX-DR21: 错误恢复模式——触碰失败"❌"1秒消失（淡然）、盲盒离线正常开奖联网校准（无感）、皮肤同步失败"下次自动更新"（安心）、步数补查自动（无感）

### FR Coverage Map

FR1: Epic 1 - 猫跟随运动状态实时变化
FR2: Epic 1 - 猫随机微行为动画
FR3: Epic 1 - 镜像时刻零引导 Onboarding
FR4: Epic 3 - 手表快速换装（最近 5 套）
FR5: Epic 1 - AOD 静态简化显示
FR6: Epic 4 - 盲盒每 30 分钟自动掉落
FR7: Epic 4 - 步数解锁盲盒 + Digital Crown 替代
FR8: Epic 4 - 离线开奖（序列化礼物列表）
FR9: Epic 4 - 盲盒开启展示动画 + 触觉反馈
FR10: Epic 4 - 盲盒状态持久化
FR11: Epic 1 - CMPedometer 步数补查恢复
FR12: Epic 4 - 每日自动签到（猫叼礼物）
FR13: Epic 4 - 签到月累计制
FR14: Epic 4 - 每月补签 2-3 次
FR15: Epic 4 - 签到离线可用
FR16: Epic 4 - 签到去重队列按序发放
FR17: Epic 3 - iPhone 浏览皮肤库存
FR18: Epic 3 - iPhone 5 层自由搭配
FR19: Epic 3 - iPhone 皮肤动态预览
FR20: Epic 3 - 皮肤搭配跨设备同步
FR21: Epic 3 - CDN 皮肤按需下载 + 版本校验 + 撤回
FR22: Epic 3 - 手表皮肤缓存 LRU 淘汰
FR23: Epic 5 - iPhone 生成邀请链接
FR24: Epic 5 - 深度链接自动配对
FR25: Epic 5 - 好友猫同屏（MVP 最多 3 只猫）
FR26: Epic 5 - HTTP 轮询 30 秒 + 插值动画
FR27: Epic 5 - 好友离线标记（半透明 + 💤）
FR28: Epic 5 - 点击好友猫发送触碰
FR29: Epic 5 - 后台推送接收触碰震动
FR30: Epic 5 - 单个好友静音
FR31: Epic 5 - 触碰频率限制（10 分钟 3 次）
FR32: Epic 5 - 专注模式触碰降级
FR33: Epic 6 - Complication 猫状态插画
FR34: Epic 6 - Complication 状态切换
FR35: Epic 6 - Complication 点击跳转 App
FR36: Epic 2 - Sign in with Apple 登录
FR37: Epic 5 - 好友列表管理
FR38: Epic 5 - 通知偏好设置
FR39: Epic 3 - iPhone 显示皮肤同步状态
FR40: Epic 2 - 手表兼容性检测
FR41: Epic 7 - 个人简要统计
FR42: Epic 3 - CLI 皮肤上传 CDN
FR43: Epic 4 - 序列化礼物列表下发
FR44: Epic 7 - 每日数据摘要
FR45: Epic 7 - 盲盒开奖事后审计
FR46: Epic 5 - 屏蔽好友
FR47: Epic 5 - 举报好友
FR48: Epic 5 - 免打扰时段
FR49: Epic 2 - 账号删除（30 天冷却期）
FR50: Epic 3 - 皮肤无稀有度显示
FR51: Epic 4 - 盲盒掉落本地通知
FR52: Epic 5 - 触碰发送确认震动
FR53: Epic 5 - 邀请分享卡片
FR54: Epic 6 - 第 3 天 Complication 引导
FR55: Epic 5 - 邀请全链路转化追踪
FR56: Epic 7 - 业务指标汇总
FR57: Epic 3 - 皮肤掉率动态调整
FR58: Epic 2 - 换机数据恢复
FR59: Epic 5 - 系统猫（NPC 陪伴）
FR60: Epic 1 - 低电量模式
FR61: Epic 5 - iPhone-only 好友支持
FR62: Epic 3 - 套装发现彩蛋
FR63: Epic 3 - 皮肤版本校验

## Epic List

### Epic 1: 活的猫——手表核心体验
用户在手表上看到一只跟随运动状态实时变化的猫，含默认皮肤渲染（3 种基础体型），首次打开即感受到镜像时刻。完全离线可用。
**FRs covered:** FR1, FR2, FR3, FR5, FR11, FR60

### Epic 2: 账号与云端基础
用户通过 Sign in with Apple 登录，数据云端同步，支持换机恢复和账号删除。
**FRs covered:** FR36, FR40, FR49, FR58

### Epic 3: 皮肤收集与搭配
用户可以收集皮肤、在 iPhone 搭配猫的 5 层外观、手表快速切换、皮肤跨设备同步。iPhone App 显示同步状态。
**FRs covered:** FR4, FR17, FR18, FR19, FR20, FR21, FR22, FR39, FR42, FR50, FR57, FR62, FR63

### Epic 4: 盲盒奖励与每日签到
用户通过走路解锁盲盒获得皮肤惊喜，每天首次看猫自动签到获得礼物——零压力核心循环。
**FRs covered:** FR6, FR7, FR8, FR9, FR10, FR12, FR13, FR14, FR15, FR16, FR43, FR51

### Epic 5: 社交体验——配对、好友猫与触觉触碰
用户邀请好友配对，手表上看到好友猫实时状态，点击好友猫发送触碰双方震动。包含完整信任安全机制和好友管理。
**FRs covered:** FR23, FR24, FR25, FR26, FR27, FR28, FR29, FR30, FR31, FR32, FR37, FR38, FR46, FR47, FR48, FR52, FR53, FR55, FR59, FR61

### Epic 6: Complication 表盘入口
用户在表盘添加裤衩猫 Complication，一眼看到猫状态，点击进入 App。
**FRs covered:** FR33, FR34, FR35, FR54

### Epic 7: 运维与数据分析
系统提供每日数据摘要、盲盒审计、业务指标汇总和个人统计。
**FRs covered:** FR41, FR44, FR45, FR56

## Epic 1: 活的猫——手表核心体验

用户在手表上看到一只跟随运动状态实时变化的猫，含默认皮肤渲染（3 种基础体型），首次打开即感受到镜像时刻。完全离线可用。

### Story 1.1: 项目初始化与猫状态机核心

As a 开发者,
I want 建立 Monorepo 项目结构并实现 CatStateMachine 核心,
So that 后续所有模块有统一的项目基础和猫状态事件总线。

**Acceptance Criteria:**

**Given** 从零开始创建项目
**When** 按照架构文档初始化 Monorepo 结构
**Then** ios/ 目录包含 Xcode "iOS App with Watch App" 工程，CatWatch/CatPhone/CatShared 三个 target 正确配置
**And** CatStateMachine 作为 @Observable 单例实现，支持 idle/walking/running/sleeping 四个主状态和 micro_yawn/micro_stretch 两个微行为状态
**And** 状态转换规则与 PRD 一致（walking→idle 需 10s 静止，idle→sleeping 需 30min + 夜间）
**And** 60 秒无转换自愈回 idle
**And** 状态通过 Combine Publisher 广播，CatScene 和 ViewModel 均可订阅

### Story 1.2: 传感器集成与猫状态映射

As a 用户,
I want 手表上的猫跟随我的运动状态实时变化,
So that 我感受到"猫在跟着我"的活的体验。

**Acceptance Criteria:**

**Given** 用户授予 Motion & Fitness 权限
**When** 用户在走路
**Then** CMMotionActivity 检测到 walking 状态持续 ≥3 秒后，CatStateMachine 转为 walking 状态
**And** 加速度计辅助预测，3 秒防抖 + 过渡动画缓冲

**Given** 用户拒绝 Motion & Fitness 权限
**When** App 运行
**Then** 猫降级为计时器微动体验，盲盒改用时间解锁替代步数

**Given** App 被系统终止后重新启动
**When** App 恢复前台
**Then** 从 CMPedometer 补查丢失的步数数据（FR11），状态机从 UserDefaults 恢复最后状态

### Story 1.3: SpriteKit 猫渲染与默认皮肤

As a 用户,
I want 看到一只带默认外观的猫在手表屏幕上活动,
So that 猫不是"裸猫"，第一印象就有完整的视觉体验。

**Acceptance Criteria:**

**Given** App 启动
**When** 猫场景加载
**Then** CatScene（SpriteKit SKScene）渲染猫节点，包含基础身体层（3 种默认体型：白/橘/灰之一）
**And** 猫帧动画 ≥24fps（抬腕交互时），非交互降至 15fps（NFR1）
**And** 呼吸微动实现：idle 2.5-3.5s/次 ±0.5s 随机偏移 1-2px（UX-DR13）
**And** App 启动到猫出现 <2 秒（NFR2）
**And** 运行时内存 ≤50MB（NFR7）

### Story 1.4: 镜像时刻与零引导 Onboarding

As a 新用户,
I want 首次打开 App 时无需任何文字引导就能理解"猫在跟着我",
So that 60 秒内产生"啊哈"时刻，不会因为困惑而卸载。

**Acceptance Criteria:**

**Given** 用户首次打开 App
**When** 猫出现在屏幕上
**Then** 猫先 0.3 秒做自己的事（被抓到过渡 UX-DR1），然后映射用户当前状态
**And** 走路时猫走路，静坐时猫抬头看用户（抬腕触发），覆盖所有运动状态（FR3）
**And** 无文字引导、无教程弹窗、无权限请求弹窗（权限在后台按需请求）
**And** 每次抬腕都有 0.3 秒"被抓到"过渡（catResponseDelay UX-DR12）

### Story 1.5: 微行为动画与猫性格

As a 用户,
I want 猫偶尔展示随机的小动作（打哈欠、伸懒腰）,
So that 猫有自己的性格，不是机械循环播放的动画。

**Acceptance Criteria:**

**Given** 猫处于 idle 状态
**When** 经过随机时间间隔
**Then** 打哈欠平均每 5 分钟触发一次，伸懒腰平均每 8 分钟触发一次（FR2）
**And** 微行为播放完毕后自动回到 idle
**And** 微行为不打断其他优先级更高的状态转换

### Story 1.6: Always-On Display 与低电量模式

As a 用户,
I want 手表息屏时猫显示静态状态，低电量时自动省电,
So that 猫不过度消耗电量，手表续航不受明显影响。

**Acceptance Criteria:**

**Given** 手表进入 Always-On Display 模式
**When** 屏幕切换到 AOD
**Then** 猫显示为静态简化状态图，完全绕过 SpriteKit Scene（独立渲染路径）（FR5）

**Given** 手表电量 ≤20%
**When** EnergyBudgetManager 检测到低电量
**Then** 自动停止 SpriteKit 动画渲染，猫显示为静态图（FR60）
**And** 用户可在设置中关闭此行为
**And** 全天使用（16h 唤醒、80 次抬腕）额外耗电 ≤10%（NFR5）

## Epic 2: 账号与云端基础

用户通过 Sign in with Apple 登录，数据云端同步，支持换机恢复和账号删除。

### Story 2.1: Go 后端基础架构与数据库

As a 开发者,
I want 搭建 Go 后端服务基础框架和数据库迁移系统,
So that 后续所有服务端功能有统一的基础设施。

**Acceptance Criteria:**

**Given** server/ 目录从零搭建
**When** 按照架构文档初始化 Go 后端
**Then** 项目结构包含 cmd/server/main.go、internal/{config,middleware,handler,service,repository,model,dto}、pkg/{jwt,redis,validator}
**And** Gin 路由框架初始化，/health 端点返回 200
**And** PostgreSQL 连接通过 GORM 建立，golang-migrate 迁移系统就绪
**And** Redis 连接初始化
**And** zerolog 结构化 JSON 日志输出 stdout
**And** Docker Compose 包含 Go 服务 + PostgreSQL + Redis
**And** 环境配置通过 .env.development 管理

### Story 2.2: Sign in with Apple 认证系统

As a 用户,
I want 通过 Sign in with Apple 一键登录,
So that 我不需要记住额外的账号密码就能使用裤衩猫。

**Acceptance Criteria:**

**Given** 用户在 iPhone App 点击 Sign in with Apple
**When** Apple 认证完成返回 identity token
**Then** 客户端将 identity token 发送到 POST /v1/auth/login
**And** 服务端验证 Apple token，创建/查找用户记录，返回 JWT Access Token（7天有效）+ Refresh Token（30天有效）
**And** Token 存储在 iPhone Keychain 和 Watch Keychain
**And** JWT 双密钥轮换：新密钥签发 + 旧密钥验证并行 24 小时

**Given** Access Token 过期
**When** 客户端发起 API 请求
**Then** 自动用 Refresh Token 调用 POST /v1/auth/refresh 换取新 Token 对，对用户透明
**And** Refresh Token 也过期时，引导用户重新 Sign in with Apple

### Story 2.3: 手表兼容性检测

As a 用户,
I want iPhone App 检测我的 Apple Watch 版本是否兼容,
So that 不兼容时我能收到清晰的提示而不是遇到莫名错误。

**Acceptance Criteria:**

**Given** 用户的 iPhone 已配对 Apple Watch
**When** iPhone App 启动时
**Then** 检测配对手表的 watchOS 版本
**And** 若低于 watchOS 10，显示友好提示"裤衩猫需要 watchOS 10 及以上版本"（FR40）
**And** 不阻止 iPhone App 使用（仅提示）

### Story 2.4: 账号删除与数据清除

As a 用户,
I want 能在 iPhone App 中删除我的账号和所有数据,
So that 我的隐私得到保障，符合 GDPR 和 PIPL 合规要求。

**Acceptance Criteria:**

**Given** 用户在 iPhone App 设置中点击"删除账号"
**When** 用户确认删除操作
**Then** 进入 30 天冷却期，期间账号标记为"待删除"
**And** 冷却期内用户可通过 Sign in with Apple 重新登录撤销删除（FR49）
**And** 30 天后服务端自动清除所有关联数据（用户信息、皮肤库存、好友关系、签到记录、触碰统计）（NFR13）
**And** 对方好友列表中该用户显示为"已注销用户"，猫消失
**And** 触碰统计匿名化（计入总数但不显示来源）

### Story 2.5: 换机数据恢复

As a 用户,
I want 换了新手表或新手机后登录即可恢复数据,
So that 我不会因为换设备而丢失收集和好友。

**Acceptance Criteria:**

**Given** 用户在新设备上通过 Sign in with Apple 登录
**When** 认证成功
**Then** 服务端返回用户完整数据（好友列表、皮肤库存、签到历史）（FR58）
**And** 本地临时数据（当前盲盒进度）从服务端最新状态重建
**And** 皮肤资源从 CDN 按需重新下载

## Epic 3: 皮肤收集与搭配

用户可以收集皮肤、在 iPhone 搭配猫的 5 层外观、手表快速切换、皮肤跨设备同步。iPhone App 显示同步状态。

### Story 3.1: 皮肤数据模型与服务端 API

As a 开发者,
I want 建立皮肤系统的数据模型和服务端 API,
So that 客户端可以获取皮肤目录、管理库存和同步搭配。

**Acceptance Criteria:**

**Given** 服务端需要支持皮肤系统
**When** 创建皮肤相关数据库表和 API
**Then** skin_catalog 表包含 id/name/layer/category/asset_url/version/is_active 字段
**And** user_skin_inventory 表记录用户拥有的皮肤（user_id/skin_id/obtained_at/source）
**And** user_skin_outfit 表记录用户当前搭配（5 层各选中哪个皮肤）
**And** GET /v1/skins/catalog 返回可用皮肤目录（支持版本增量同步）
**And** GET /v1/users/me/skins 返回用户皮肤库存
**And** PUT /v1/users/me/outfit 保存搭配方案
**And** 所有皮肤统一视觉样式，API 不返回稀有度字段（FR50）

### Story 3.2: CDN 皮肤资源管线与 CLI 上传工具

As a 管理员,
I want 通过 CLI 工具上传皮肤资源到 CDN 并更新皮肤池配置,
So that 新皮肤可以快速上线，旧皮肤可以撤回。

**Acceptance Criteria:**

**Given** 管理员准备好皮肤资源文件
**When** 运行 CLI 工具 `cat-admin skin upload --file xxx --layer body --name "冬日围巾"`
**Then** 资源上传到 CDN，生成带版本号的 URL（FR42）
**And** skin_catalog 自动新增记录
**And** 支持 `cat-admin skin revoke --id xxx` 标记撤回，客户端下次启动时清除本地缓存（FR21）
**And** 单资源 CDN 下载 <3 秒（NFR19）

### Story 3.3: 手表端皮肤缓存与 5 层渲染

As a 用户,
I want 手表上的猫按照我搭配的 5 层外观渲染,
So that 我看到的猫是自己定制的独特样子。

**Acceptance Criteria:**

**Given** 用户有已搭配的皮肤方案
**When** 猫场景渲染
**Then** CatSkinRenderer 按 5 层 z-order 渲染：身体/表情/服装/头饰/配件（UX-DR3）
**And** 手表端本地缓存最多 10 套皮肤资源，超出按 LRU 淘汰（FR22）
**And** 缓存命中时换装响应 <0.5 秒（NFR4）
**And** 缺失的皮肤资源从 CDN 按需下载，下载中显示默认体型

### Story 3.4: 手表端快速换装

As a 用户,
I want 在手表上左右滑动快速切换最近 5 套搭配,
So that 我不用打开手机就能快速换个心情。

**Acceptance Criteria:**

**Given** 用户在手表猫场景界面
**When** 左右滑动
**Then** 在最近 5 套搭配方案间切换（FR4）
**And** 切换有滑动过渡动画
**And** 本地缓存命中时换装响应 <0.5 秒（NFR4）
**And** 当前选中方案同步到服务端

### Story 3.5: iPhone 皮肤库浏览

As a 用户,
I want 在 iPhone App 上浏览我拥有的全部皮肤,
So that 我能查看收集进度和发现新获得的皮肤。

**Acceptance Criteria:**

**Given** 用户打开 iPhone App 皮肤库 Tab
**When** 页面加载
**Then** 按分类展示全部皮肤，SkinCard 组件显示静态 PNG 缩略图（UX-DR15）
**And** 新获得的皮肤显示绿点标记，未获得的皮肤显示半透明（UX-DR15）
**And** 显示收集进度条（FR62）
**And** 凑齐同系列 ≥3 件触发"套装发现"彩蛋动画（FR62）
**And** 空皮肤库时显示"猫站在空衣柜前歪头"（UX-DR10）

### Story 3.6: iPhone 皮肤搭配与动态预览

As a 用户,
I want 在 iPhone App 上搭配猫的外观并看到动态预览效果,
So that 我能满意地定制自己的猫再同步到手表。

**Acceptance Criteria:**

**Given** 用户打开 iPhone App 搭配 Tab
**When** 进入搭配界面
**Then** SkinPreview 区域用 SpriteView 1:1 模拟手表显示，猫会动（FR19 + UX-DR15）
**And** LayerPicker 提供 5 层 Tab 式切换（身体/表情/服装/头饰/配件）（FR18 + UX-DR15）
**And** 切换任意皮肤组件时预览实时更新
**And** MVP ≥30 件组件可搭配，组合 ≥50 种（FR18）

### Story 3.7: 皮肤跨设备同步

As a 用户,
I want iPhone 确认搭配后手表自动同步更新猫的外观,
So that 我在任一设备上的操作都能无缝反映到另一端。

**Acceptance Criteria:**

**Given** 用户在 iPhone App 确认新搭配
**When** 点击确认按钮
**Then** 搭配方案通过 WatchConnectivity 同步到手表（key: `cat.skin.outfit_update`）
**And** 恢复连接后 ≤10 秒完成同步（FR20 + NFR8）
**And** iPhone App 显示手表端皮肤同步状态（已同步/同步中/等待连接）（FR39）
**And** 同步失败时显示"下次自动更新"安心提示（UX-DR21）

### Story 3.8: 皮肤版本校验与掉率调整

As a 系统,
I want 自动校验皮肤版本并动态调整掉率,
So that 撤回的皮肤及时清除，用户收集进度平衡合理。

**Acceptance Criteria:**

**Given** 客户端启动
**When** 加载皮肤缓存
**Then** 比对本地版本与服务端最新版本，被撤回的皮肤自动清除本地缓存（FR63）
**And** 已撤回皮肤从用户库存中保留记录但标记为"不可用"

**Given** 系统配置掉率规则
**When** 用户通过盲盒/签到获得皮肤
**Then** 掉率随用户收集进度动态调整，目标：中度活跃用户 60 天收集 80% 独立组件（FR57）
**And** 掉率配置由服务端控制，客户端不可篡改

## Epic 4: 盲盒奖励与每日签到

用户通过走路解锁盲盒获得皮肤惊喜，每天首次看猫自动签到获得礼物——零压力核心循环。

### Story 4.1: 序列化礼物系统（服务端）

As a 系统,
I want 为每个用户预分配盲盒和签到的皮肤奖励序列,
So that 客户端可以离线开奖，联网后校准一致性。

**Acceptance Criteria:**

**Given** 用户首次登录或序列耗尽
**When** 客户端请求 GET /v1/users/me/gift_sequence
**Then** 服务端返回预分配的序列化礼物列表（含序列 ID、皮肤 ID 列表、版本号）（FR43）
**And** 客户端缓存序列供离线使用
**And** 服务端维护序列消费位置为权威

**Given** 客户端离线消费了序列位置 7-9
**When** 联网后上报消费记录
**Then** 若服务端检测到位置冲突（另一设备已消费同一位置），以服务端为准
**And** 客户端获得的皮肤标记"待确认"，联网后重新校准
**And** 冲突时不丢物品，补发等价物

### Story 4.2: 盲盒掉落与步数解锁

As a 用户,
I want 系统自动掉落盲盒，我通过走路就能解锁获得皮肤,
So that 我有一个"起身走几步的可爱理由"。

**Acceptance Criteria:**

**Given** 当前没有待解锁盲盒
**When** 经过 30 分钟
**Then** 系统自动掉落一个盲盒，单盲盒队列——必须解锁后才掉下一个（FR6）
**And** 盲盒从屏幕边缘滑入，猫转头看向盲盒（视线引导 UX-DR4）
**And** 发送本地通知 + 手腕震动（FR51）

**Given** 有一个待解锁的盲盒
**When** 用户累积 200-300 步
**Then** 盲盒自动解锁（FR7）

**Given** 盲盒持续 2 小时未解锁
**When** 超时
**Then** 提供替代解锁方式：旋转 Digital Crown 完成小游戏（FR7）

### Story 4.3: 盲盒离线开奖与展示动画

As a 用户,
I want 离线也能开盲盒并看到惊喜展示动画,
So that 我不需要联网就能享受核心循环。

**Acceptance Criteria:**

**Given** 盲盒解锁条件达成（步数/Digital Crown/时间替代）
**When** 开启盲盒
**Then** 基于本地缓存的序列化礼物列表确定奖品（FR8）
**And** 播放两阶段 1.5 秒动画：猫一爪拍开→皮肤飞出猫接住展示（UX-DR5）
**And** 触觉反馈：三连短促+长震（UX-DR8）
**And** 皮肤存入库存不自动穿戴（FR9）
**And** 离线时正常开奖，联网后校准（UX-DR21）

### Story 4.4: 盲盒状态持久化与恢复

As a 用户,
I want 盲盒进度在 App 被终止后可以恢复,
So that 我不会因为系统杀掉 App 而丢失走路积累的步数。

**Acceptance Criteria:**

**Given** 有一个待解锁的盲盒（已累积部分步数）
**When** App 被系统终止后重新启动
**Then** 盲盒状态从持久化存储恢复（SwiftData + UserDefaults 双写备份）（FR10）
**And** 从 CMPedometer 补查 App 暂停期间的步数（复用 Story 1.2 的 FR11 能力）
**And** 用户感知不到中断

### Story 4.5: 每日签到——猫叼礼物

As a 用户,
I want 每天首次看猫时猫叼着礼物等我,
So that 我感受到"猫给我带了东西"的温暖，而不是"签到任务"的压力。

**Acceptance Criteria:**

**Given** 用户当天首次打开 App
**When** 猫场景加载
**Then** 猫叼着礼物等用户，自动完成签到（FR12）
**And** 猫把礼物放到面前，自动入库，短暂显示皮肤名称 1 秒后消失（UX-DR6）
**And** 签到动画作为仪式感展示（猫举牌），不要求额外点击
**And** 签到奖品从去重队列按序发放，不重复（FR16）
**And** 事件优先级：晨间礼物低于触碰接收和盲盒解锁（UX-DR9）

### Story 4.6: 签到累计制与补签

As a 用户,
I want 签到断签不影响进度，还能补签,
So that 我永远不会因为忘记签到而焦虑。

**Acceptance Criteria:**

**Given** 用户签到记录
**When** 查看签到进度
**Then** 采用月累计制，断签不影响已有进度（FR13）
**And** 用户每月可补签 2-3 次（FR14）
**And** 签到可在离线状态下完成，联网后自动同步（FR15）
**And** 签到数据存储在 SwiftData 本地 + 联网同步服务端

## Epic 5: 社交体验——配对、好友猫与触觉触碰

用户邀请好友配对，手表上看到好友猫实时状态，点击好友猫发送触碰双方震动。包含完整信任安全机制和好友管理。

### Story 5.1: 社交数据模型与好友 API

As a 开发者,
I want 建立社交系统的数据模型和服务端 API,
So that 支持好友配对、状态同步和触碰传递。

**Acceptance Criteria:**

**Given** 服务端需要支持社交功能
**When** 创建社交相关数据库表和 API
**Then** friendships 表记录好友关系（user_id_a/user_id_b/status/created_at）
**And** friend_states 表记录好友最新猫状态（user_id/cat_state/updated_at），Redis 缓存热数据
**And** POST /v1/friends/invite 生成邀请链接
**And** POST /v1/friends/accept 接受邀请完成配对
**And** GET /v1/friends 返回好友列表（含在线状态）
**And** GET /v1/friends/states 返回好友猫状态（轮询接口）
**And** POST /v1/friends/{id}/touch 发送触碰
**And** DELETE /v1/friends/{id} 删除好友
**And** 好友之间仅可见猫运动状态，不可见具体步数、位置或健康数据（NFR14）

### Story 5.2: 邀请链接与自动配对

As a 用户,
I want 在 iPhone App 生成邀请链接发给好友，好友点击后自动配对,
So that 加好友的过程简单到只需一个链接。

**Acceptance Criteria:**

**Given** 用户在 iPhone App 好友 Tab 点击"邀请好友"
**When** 生成邀请
**Then** 创建带唯一 token 的深度链接（FR23）
**And** 附带精美分享卡片，适合微信/朋友圈等社交平台传播（FR53）
**And** 通过系统分享面板发送

**Given** 好友点击邀请链接
**When** 已安装裤衩猫
**Then** App 打开后自动完成配对（FR24）
**And** 双方收到配对成功反馈

**Given** 好友点击邀请链接
**When** 未安装裤衩猫
**Then** 跳转 App Store，下载安装登录后自动关联配对（FR24）

**Given** 系统追踪邀请链路
**When** 各阶段事件发生
**Then** 记录邀请链接生成→点击→下载→配对完成全链路转化数据（FR55）

### Story 5.3: 好友猫同屏显示

As a 用户,
I want 在手表屏幕上同时看到好友的猫,
So that 我感受到"好友在身边"的陪伴。

**Acceptance Criteria:**

**Given** 用户有已配对好友
**When** 手表猫场景渲染
**Then** 同屏显示好友猫（MVP 最多 2 好友 = 3 只猫同屏）（FR25）
**And** 多猫布局：1 只居中偏下；2 只左右分列（自己左、好友右）；3 只自己居中偏下+好友左上右上（UX-DR14）
**And** 好友猫缩小到 60×60，降至 12fps，降饱和 -20%，透明度 85%（UX-DR2 背景层）
**And** 各猫独立点击区域不重叠，触摸目标 ≥44pt（UX-DR14）

**Given** 好友状态更新
**When** 前台期间
**Then** 每 30 秒 HTTP 轮询拉取好友猫状态（FR26）
**And** 状态切换动画插值平滑过渡

**Given** 好友状态超过 2 分钟未更新
**When** 轮询返回陈旧数据
**Then** 好友猫显示离线标记：半透明 + 💤（FR27 + UX-DR10）

### Story 5.4: 系统猫（NPC 陪伴）

As a 无好友用户,
I want 有一只系统猫陪伴在旁,
So that 我不会觉得屏幕空荡荡的。

**Acceptance Criteria:**

**Given** 用户没有任何已配对好友
**When** 手表猫场景渲染
**Then** 屏幕上出现一只"系统猫"——行为随机的 NPC 猫（FR59）
**And** 系统猫使用背景层渲染规则（60×60，12fps，降饱和）
**And** 系统猫不可触碰（点击无反应或提示"添加好友"）
**And** 用户首次配对好友后系统猫自然消失

### Story 5.5: 触碰发送与接收

As a 用户,
I want 点击好友的猫发送触碰，双方手腕同时震动,
So that 我能用最简单的方式告诉好友"我在想你"。

**Acceptance Criteria:**

**Given** 用户在手表屏幕上点击好友的猫
**When** 触发触碰
**Then** 自己的猫走过去碰一下好友猫（0.8 秒动画 UX-DR7）
**And** 自己手腕收到一次确认震动（"已发送" FR52）
**And** 服务端通过 APNs 推送到好友设备
**And** 好友手腕收到触觉反馈：双跳心跳（UX-DR8）
**And** 从发送到好友收到震动 <5 秒（NFR3）

**Given** 好友猫处于睡觉状态
**When** 发送触碰
**Then** 加 0.5 秒"嘘…"缓冲动画（UX-DR7）

**Given** 好友离线
**When** 发送触碰
**Then** 显示"ta 下次抬腕会感受到"（UX-DR7）
**And** 服务端缓存触碰，好友上线后推送

### Story 5.6: 触碰推送与后台接收

As a 用户,
I want App 没打开时也能收到好友触碰的震动,
So that 触碰随时能到达我。

**Acceptance Criteria:**

**Given** 用户 App 在后台或未打开
**When** 好友发送触碰
**Then** 通过 APNs 推送通知到手表（FR29）
**And** 手腕震动：双跳心跳触觉

**Given** iPhone 处于专注模式
**When** 收到触碰推送
**Then** 通知自动降级为静默，不震动不弹窗（FR32）

### Story 5.7: 触碰频率限制与静音

As a 用户,
I want 不被过多触碰打扰,
So that 触碰保持"特别"的感觉而不变成噪音。

**Acceptance Criteria:**

**Given** 同一好友连续发送触碰
**When** 10 分钟内超过 3 次
**Then** 后续触碰被服务端限流，不推送（FR31）
**And** 发送方不感知限流（照常显示发送动画，不提示被限制）

**Given** 用户想静音某个好友
**When** 设置静音
**Then** 该好友触碰不再推送震动（FR30）
**And** 触碰记录仍然保存

**Given** 用户设置免打扰时段
**When** 在免打扰时段内
**Then** 所有触碰推送静默（FR48）

### Story 5.8: 好友管理与信任安全

As a 用户,
I want 管理好友列表，可以屏蔽和举报不当行为,
So that 我的社交空间是安全的。

**Acceptance Criteria:**

**Given** 用户在 iPhone App 好友 Tab
**When** 查看好友列表
**Then** 显示所有好友（含在线/离线状态），非活跃好友自动标记（FR37）

**Given** 用户屏蔽某好友
**When** 执行屏蔽
**Then** 双方猫互不可见，触碰不可达（FR46）
**And** 不通知被屏蔽方

**Given** 用户举报某好友
**When** 提交举报
**Then** 记录举报信息并提交到管理后台（FR47）
**And** 举报后自动屏蔽

**Given** 用户删除/解除配对
**When** 执行删除
**Then** 好友关系解除，双方猫消失（FR37）

### Story 5.9: 通知偏好设置

As a 用户,
I want 分别控制好友触碰和盲盒掉落的通知开关,
So that 我能自定义哪些通知是我想要的。

**Acceptance Criteria:**

**Given** 用户在 iPhone App 设置页面
**When** 调整通知偏好
**Then** 好友触碰通知和盲盒掉落通知提供独立开关（FR38）
**And** 设置通过 WatchConnectivity 同步到手表端
**And** 关闭后对应通知不推送、不震动

### Story 5.10: iPhone-only 好友支持

As a 没有 Apple Watch 的好友,
I want 通过 iPhone App 也能查看好友猫和发送触碰,
So that 没手表也能参与社交。

**Acceptance Criteria:**

**Given** 用户没有配对 Apple Watch
**When** 在 iPhone App 好友 Tab
**Then** 可以查看好友猫的实时状态（FR61）
**And** 可以发送触碰（好友手表收到震动）
**And** 可以接收触碰推送通知（iPhone 震动）（FR61）

## Epic 6: Complication 表盘入口

用户在表盘添加裤衩猫 Complication，一眼看到猫状态，点击进入 App。

### Story 6.1: Complication 猫状态插画

As a 用户,
I want 在表盘上看到猫当前状态的专用像素级插画,
So that 不用打开 App 就能一眼看到猫在做什么。

**Acceptance Criteria:**

**Given** 用户在表盘添加裤衩猫 Complication
**When** Complication 渲染
**Then** 显示猫当前运动状态的专用像素级插画（FR33）
**And** 支持 3-5 种状态静态插画切换（idle/walking/running/sleeping + 至少 1 种微行为）（FR34）
**And** 线条加粗 3-4pt 适配小尺寸（UX-DR19）
**And** 支持主流 Complication Family：graphicCircular、graphicRectangular、graphicCorner

### Story 6.2: Complication 交互与 App 跳转

As a 用户,
I want 点击 Complication 直接打开裤衩猫 App,
So that 从表盘到 App 的入口最短。

**Acceptance Criteria:**

**Given** 用户点击表盘上的裤衩猫 Complication
**When** 触发点击
**Then** 直接打开裤衩猫 App 主界面（FR35）
**And** 如果 App 已在后台，恢复到猫场景

### Story 6.3: 第 3 天 Complication 延迟引导

As a 用户,
I want 猫在合适的时机自然引导我设置 Complication,
So that 我知道有这个功能但不觉得被强迫。

**Acceptance Criteria:**

**Given** 用户使用 App 满 3 天
**When** 第 3 天签到完成后
**Then** 猫播放一个自然引导动画（猫指向表盘方向或做出暗示动作）（FR54）
**And** 显示简洁提示"把我放到表盘上？"
**And** 提供"好的"（打开表盘设置）和"以后再说"两个选项
**And** 用户选择"以后再说"后不再重复引导
**And** 如果用户已设置 Complication，跳过此引导

## Epic 7: 运维与数据分析

系统提供每日数据摘要、盲盒审计、业务指标汇总和个人统计。

### Story 7.1: 个人简要统计

As a 用户,
I want 在 iPhone App 查看今天的简要统计,
So that 我能了解自己的使用情况。

**Acceptance Criteria:**

**Given** 用户打开 iPhone App 首页
**When** 页面加载
**Then** 显示今日步数、已开盲盒数、皮肤收集总数、今日触碰次数（FR41）
**And** 数据实时从本地 + 服务端聚合
**And** 离线时显示本地缓存数据

### Story 7.2: 每日数据摘要与数据库备份

As a 运维人员,
I want 系统每日自动生成数据摘要并备份数据库,
So that 我能监控系统健康并确保数据安全。

**Acceptance Criteria:**

**Given** 每天凌晨 3:00
**When** 定时任务触发
**Then** 自动生成数据摘要（FR44）：DAU、新增用户、配对数、盲盒解锁数、触碰总数
**And** pg_dump 执行数据库备份到对象存储
**And** 摘要通过日志输出（zerolog），可接入告警系统

### Story 7.3: 盲盒开奖审计

As a 运维人员,
I want 系统对盲盒开奖结果进行事后审计,
So that 检测统计异常（刷奖、序列篡改等）。

**Acceptance Criteria:**

**Given** 每日数据摘要触发时
**When** 审计脚本运行
**Then** 检测单用户每日开奖次数异常（超过理论上限）（FR45）
**And** 检测序列消费跳跃（跳过位置）
**And** 检测离线消费与服务端记录不一致的比例
**And** 异常记录写入审计日志，超过阈值触发告警

### Story 7.4: 业务指标汇总

As a 产品负责人,
I want 查看核心业务指标汇总,
So that 我能了解产品运营状况。

**Acceptance Criteria:**

**Given** 系统持续收集运营数据
**When** 查看指标面板（初期可通过 API 或日志查询）
**Then** 统计并汇总以下指标（FR56）：
- DAU / MAU
- 配对率（有至少 1 个好友的用户占比）
- 皮肤收集数分布（P25/P50/P75）
- Complication 使用率
- 邀请转化率（生成→点击→下载→配对）
- 盲盒解锁率（掉落→解锁的比例）
- 触碰频率（日均每对好友触碰次数）
