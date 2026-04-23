# 宠物互动 App iOS 客户端工程结构与模块职责设计

## 1. 文档说明

本文档用于定义当前版本 iPhone 客户端工程的落地结构，重点说明：

- iOS 客户端总体工程组织方式
- UI / ViewModel / UseCase / Repository / System Adapter 的职责边界
- 首页、背包、合成、房间等模块的拆分方式
- 与后端 V1 接口、数据库模型的对应关系
- 步数采集、运动状态识别、WebSocket 实时通信的接入方式
- 状态管理、导航、错误处理、缓存、测试建议

本文档基于当前已经确认的业务规则编写，并与以下文档保持一致：

- `宠物互动App_总体架构设计.md`
- `宠物互动App_V1接口设计.md`
- `宠物互动App_数据库设计.md`
- `宠物互动App_Go项目结构与模块职责设计.md`

---

## 2. 客户端工程设计目标

当前版本客户端最适合的形态是：

- **SwiftUI 为主的 iPhone App**
- **按业务 Feature 拆模块**
- **按层次组织代码**
- **REST 处理普通业务请求**
- **WebSocket 处理房间实时互动**
- **HealthKit / CoreMotion 负责步数与运动状态采集**

设计目标主要有四个：

1. **页面开发快**
   - 目前产品规则仍在细化
   - 首页、合成、房间交互后续都可能继续迭代

2. **让业务逻辑不要直接写死在 View 中**
   - SwiftUI 页面应该保持轻量
   - 复杂规则放到 ViewModel / UseCase / Repository

3. **为后续功能扩展留空间**
   - 未来可能增加更多装扮部位
   - 可能增加好友系统、更多房间玩法、更多宠物种类

4. **保证本地动画体验和远端数据一致性之间的平衡**
   - 猫咪状态切换要流畅
   - 步数、宝箱、背包、房间等关键数据仍以后端为准

---

## 3. 总体工程结论

当前建议采用：

- **Swift + SwiftUI**
- **MVVM + UseCase + Repository**
- **单 App 工程，按 Feature 模块组织**
- **RESTClient + WebSocketClient 双通道**
- **HealthKit + CoreMotion 作为系统能力接入层**
- **本地只缓存展示态与会话态，关键资产以后端为准**

### 3.1 客户端总体结构

```text
[Presentation]
  ├─ App Entry
  ├─ Navigation
  ├─ Screens
  ├─ Components
  └─ ViewModels

[Domain]
  ├─ UseCases
  ├─ Entities / DTO Mappers
  └─ Business Rules

[Data]
  ├─ REST Repositories
  ├─ WebSocket Repositories
  ├─ Local Cache Repositories
  └─ Config Repositories

[System Adapter]
  ├─ HealthKit Adapter
  ├─ Motion Adapter
  ├─ Keychain Adapter
  ├─ Timer Adapter
  └─ Reachability Adapter

[Infrastructure]
  ├─ Networking
  ├─ Realtime
  ├─ Storage
  ├─ Logging
  ├─ App Settings
  └─ Error Mapper
```

---

## 4. 项目目录建议

建议目录如下：

```text
PetApp/
├─ App/
│  ├─ PetAppApp.swift
│  ├─ AppContainer.swift
│  ├─ AppCoordinator.swift
│  ├─ AppEnvironment.swift
│  └─ RootView.swift
├─ Core/
│  ├─ DesignSystem/
│  │  ├─ Colors/
│  │  ├─ Fonts/
│  │  ├─ Components/
│  │  └─ Animations/
│  ├─ Networking/
│  │  ├─ APIClient.swift
│  │  ├─ APIRequest.swift
│  │  ├─ APIError.swift
│  │  ├─ Endpoint.swift
│  │  └─ AuthInterceptor.swift
│  ├─ Realtime/
│  │  ├─ WebSocketClient.swift
│  │  ├─ RealtimeMessage.swift
│  │  ├─ RealtimeRouter.swift
│  │  └─ HeartbeatManager.swift
│  ├─ Storage/
│  │  ├─ KeychainStore.swift
│  │  ├─ UserDefaultsStore.swift
│  │  └─ LocalCache.swift
│  ├─ Motion/
│  │  ├─ MotionStateProvider.swift
│  │  ├─ MotionState.swift
│  │  └─ MotionPermissionManager.swift
│  ├─ Health/
│  │  ├─ StepProvider.swift
│  │  ├─ HealthPermissionManager.swift
│  │  └─ StepSnapshot.swift
│  ├─ Logging/
│  ├─ Utils/
│  └─ Extensions/
├─ Shared/
│  ├─ Models/
│  ├─ Mappers/
│  ├─ Constants/
│  └─ ErrorHandling/
├─ Features/
│  ├─ Auth/
│  │  ├─ Views/
│  │  ├─ ViewModels/
│  │  ├─ UseCases/
│  │  ├─ Repositories/
│  │  └─ Models/
│  ├─ Home/
│  ├─ Pet/
│  ├─ Steps/
│  ├─ Chest/
│  ├─ Cosmetics/
│  ├─ Compose/
│  ├─ Room/
│  └─ Emoji/
├─ Resources/
│  ├─ Assets.xcassets
│  ├─ Localizable.strings
│  └─ Configs/
├─ Tests/
│  ├─ UnitTests/
│  └─ UITests/
└─ README.md
```

---

## 5. 分层职责设计

为了避免把所有逻辑都写进 SwiftUI View，建议明确划分以下几层。

## 5.1 View 层

负责：

- 页面布局
- 组件组合
- 动画展示
- 用户点击事件转发
- 绑定 ViewModel 状态

不负责：

- 直接调网络接口
- 直接处理复杂业务逻辑
- 拼接 WebSocket 协议
- 直接读系统步数

### 典型页面

- 登录页
- 首页
- 宝箱弹窗
- 背包页
- 合成页
- 房间页
- 表情面板

---

## 5.2 ViewModel 层

负责：

- 页面状态管理
- 发起 UseCase
- loading / error / empty 状态控制
- 页面倒计时与展示态协调
- 事件派发与 UI 更新

不负责：

- 持久化底层实现
- 网络协议细节
- 复杂的跨模块资产规则

### 典型 ViewModel

- `AuthViewModel`
- `HomeViewModel`
- `InventoryViewModel`
- `ComposeViewModel`
- `RoomViewModel`

---

## 5.3 UseCase 层

负责：

- 对一个明确业务动作进行封装
- 组织多个 Repository 协作
- 做轻量业务编排
- 输出页面真正需要的数据

### 典型 UseCase

- `GuestLoginUseCase`
- `LoadHomeUseCase`
- `SyncStepsUseCase`
- `OpenChestUseCase`
- `LoadInventoryUseCase`
- `EquipCosmeticUseCase`
- `UnequipCosmeticUseCase`
- `ComposeUpgradeUseCase`
- `CreateRoomUseCase`
- `JoinRoomUseCase`
- `LeaveRoomUseCase`
- `ConnectRoomRealtimeUseCase`
- `SendEmojiUseCase`

---

## 5.4 Repository 层

负责：

- 统一对外提供数据访问入口
- 隐藏 REST / WebSocket / 本地缓存 / 系统能力细节
- 让上层不感知具体实现来源

### Repository 分类

- `AuthRepository`
- `HomeRepository`
- `StepRepository`
- `ChestRepository`
- `CosmeticRepository`
- `ComposeRepository`
- `RoomRepository`
- `EmojiRepository`
- `RealtimeRepository`
- `SessionRepository`

---

## 5.5 System Adapter 层

负责：

- HealthKit 步数读取
- CoreMotion 活动状态识别
- Keychain 持久化游客身份
- 定时器封装
- 网络状态检测

这是 iOS 客户端特有的一层，不建议把这些能力直接散落在 Feature 中。

---

## 6. 模块拆分建议

建议客户端按业务 Feature 拆模块，而不是按页面文件随机堆放。

### 6.1 Auth 模块

负责：

- 游客身份初始化
- 游客登录
- 微信绑定入口
- 登录态恢复

建议包含：

- `AuthView`
- `AuthViewModel`
- `GuestLoginUseCase`
- `BindWechatUseCase`
- `AuthRepository`

### 6.2 Home 模块

负责：

- 首页聚合展示
- 用户信息
- 当前宠物展示态
- 步数账户展示
- 当前宝箱展示
- 当前房间入口信息

建议包含：

- `HomeView`
- `HomeViewModel`
- `LoadHomeUseCase`
- `RefreshHomeUseCase`

### 6.3 Pet 模块

负责：

- 猫咪视觉状态切换
- 宠物穿戴展示模型
- 状态同步触发

注意：

- 宠物动画切换以本地即时判断为主
- 服务端只保存最近展示态，不做高频状态驱动

### 6.4 Steps 模块

负责：

- 读取系统步数
- 读取运动状态
- 触发步数同步
- 管理同步频率与时机

### 6.5 Chest 模块

负责：

- 宝箱剩余时间展示
- 可开启状态展示
- 开箱请求
- 开箱结果弹窗

### 6.6 Cosmetics 模块

负责：

- 背包展示
- 装扮实例列表展示
- 穿戴 / 卸下
- 道具筛选与分类

### 6.7 Compose 模块

负责：

- 合成页展示
- 手动选择材料实例
- 已选数量统计
- 发起合成请求

### 6.8 Room 模块

负责：

- 创建 / 加入 / 退出房间
- 房间成员展示
- 房间页面状态管理
- WebSocket 会话管理入口

### 6.9 Emoji 模块

负责：

- 表情配置展示
- 表情面板 UI
- 发送表情
- 接收表情广播后的动效提示

---

## 7. 关键页面与状态组织

## 7.1 App Root 状态

建议 App 根状态只保留几类高层信息：

- 当前是否已登录
- 当前 token 是否有效
- 当前是否已完成权限准备
- 当前是否在房间中

建议定义类似：

```text
AppLaunchState
  ├─ launching
  ├─ needsAuth
  └─ ready
```

---

## 7.2 首页状态

首页状态建议拆成：

- `user`
- `pet`
- `stepAccount`
- `chest`
- `roomSummary`
- `uiFlags`

其中：

- `pet.currentState` 可以受本地运动状态驱动
- `chest.remainingSeconds` 可以由本地 timer 驱动，但服务端数据定期纠正

---

## 7.3 合成页状态

建议状态包含：

- 当前选择品质 `fromRarity`
- 可选实例列表
- 已选实例 id 集合
- 已选数量
- 是否可提交
- 提交中状态

由于玩家需要手动选择道具实例，所以合成页不能只做总数按钮，而要展示实例化可选数据。

---

## 7.4 房间页状态

建议状态包含：

- 房间基础信息
- 房间成员列表
- WebSocket 连接状态
- 最近收到的表情事件
- 当前是否正在重连

---

## 8. 网络层设计

## 8.1 REST Client

建议封装统一的 `APIClient`，能力包括：

- 请求构建
- 自动注入 token
- 通用解码
- 业务错误映射
- 401 处理
- 请求日志

### 推荐模式

```text
ViewModel -> UseCase -> Repository -> APIClient -> Endpoint
```

不要让 ViewModel 直接拼 URLRequest。

---

## 8.2 Endpoint 设计

建议每个接口使用枚举或结构体定义，例如：

- `AuthEndpoint.guestLogin`
- `HomeEndpoint.loadHome`
- `StepEndpoint.sync`
- `ChestEndpoint.open`
- `ComposeEndpoint.upgrade`

这样便于统一维护请求路径、方法、参数和响应类型。

---

## 8.3 错误映射

后端已有统一错误码，客户端建议做一层 `AppErrorMapper`：

- `1001` -> 登录失效
- `3002` -> 步数不足
- `4002` -> 宝箱未解锁
- `5005` -> 合成材料数量错误
- `6002` -> 房间已满

View 层只关心用户可理解的文案，不直接处理原始 error code。

---

## 9. WebSocket 设计

## 9.1 连接职责

WebSocket 只服务于房间内实时互动，不建议用于普通业务请求。

连接建立后负责：

- 房间快照接收
- 成员加入/离开通知
- 表情广播接收
- 心跳维持
- 断线重连

---

## 9.2 客户端对象建议

建议封装：

- `WebSocketClient`
- `RoomRealtimeRepository`
- `RoomRealtimeViewModelBridge`

这样可以让 WebSocket 消息先被统一解析，再更新到房间 ViewModel。

---

## 9.3 重连策略

建议：

- 前台且在房间中时保持连接
- 断开后指数退避重连
- 进入后台时可以断开或降频
- 回到前台时主动重连

前提仍然是：
- 用户 token 有效
- 用户当前仍在该房间中

---

## 10. 步数与运动状态设计

## 10.1 步数来源

步数读取建议使用系统汇总步数，而不是自己累积传感器事件。

原因：

- 与产品规则一致
- 数据源稳定
- 与系统权限模型更匹配

客户端上传的是：

- 当日总步数
- 当前运动状态
- 客户端时间戳

最终是否入账由服务端判断。

---

## 10.2 运动状态映射

用户说“坐下的时候休息”，从工程实现上建议映射成“静止”。

建议本地状态使用：

- `rest`
- `walk`
- `run`

可由系统原始状态映射：

- `stationary` -> `rest`
- `walking` -> `walk`
- `running` -> `run`

---

## 10.3 同步时机建议

建议以下时机触发步数同步：

- App 启动后进入首页
- App 从后台回到前台
- 首页停留一段时间后定时同步
- 开箱前主动同步一次

不建议过高频同步，避免：

- 耗电
- 接口压力增加
- 界面反复刷新

---

## 11. 本地存储设计

## 11.1 Keychain

建议保存：

- `guestUid`
- `token`
- 必要时保存 refresh token

`guestUid` 一定要存在 Keychain，而不是 UserDefaults，否则卸载重装或系统清理后更容易丢失游客身份。

---

## 11.2 UserDefaults / 轻缓存

建议保存：

- 最近一次首页快照
- 最近选择的合成品质页签
- 房间页面 UI 偏好
- 是否已弹过某些引导

不建议保存：

- 可用步数
- 宝箱最终状态
- 背包装扮真实归属

这些都应以后端数据为准。

---

## 12. 与后端接口的对应关系

## 12.1 App 启动链路

```text
App Launch
  -> read guestUid from Keychain
  -> POST /auth/guest-login
  -> save token
  -> GET /home
  -> render HomeView
```

---

## 12.2 开箱链路

```text
HomeView
  -> SyncStepsUseCase
     -> POST /steps/sync
  -> OpenChestUseCase
     -> POST /chest/open
  -> update local home state
  -> show reward popup
```

---

## 12.3 穿戴链路

```text
InventoryView
  -> select userCosmeticItemId
  -> EquipCosmeticUseCase
     -> POST /cosmetics/equip
  -> refresh local pet equips
  -> refresh inventory states
```

---

## 12.4 合成链路

```text
ComposeView
  -> load inventory groups
  -> user selects 10 item instances
  -> ComposeUpgradeUseCase
     -> POST /compose/upgrade
  -> remove consumed instances locally
  -> insert reward instance locally
  -> show reward popup
```

---

## 12.5 房间链路

```text
RoomEntry
  -> POST /rooms or /rooms/{id}/join
  -> GET /rooms/{id}
  -> connect /ws/rooms/{id}
  -> receive snapshot / member events / emoji events
```

---

## 13. 模块职责与接口映射

| 模块 | 主要接口 | 主要本地能力 |
|---|---|---|
| Auth | `/auth/guest-login`, `/auth/bind-wechat`, `/me` | Keychain |
| Home | `/home` | Timer |
| Steps | `/steps/sync`, `/steps/account` | HealthKit, CoreMotion |
| Chest | `/chest/current`, `/chest/open` | Timer |
| Cosmetics | `/cosmetics/catalog`, `/cosmetics/inventory`, `/cosmetics/equip`, `/cosmetics/unequip` | Local cache |
| Compose | `/compose/overview`, `/compose/upgrade` | Selection state |
| Room | `/rooms`, `/rooms/current`, `/rooms/{id}`, `/rooms/{id}/join`, `/rooms/{id}/leave` | WebSocket |
| Emoji | `/emojis`, `/ws/rooms/{id}` | WebSocket |

---

## 14. UI 组件建议

为了避免页面重复实现，建议抽出以下通用组件：

- `PetCardView`
- `ChestCountdownView`
- `StepBalanceView`
- `InventoryGridView`
- `CosmeticItemCell`
- `ComposeSelectionFooter`
- `RoomMemberCardView`
- `EmojiPanelView`
- `LoadingOverlay`
- `AppErrorToast`

这些组件建议放在：

- `Core/DesignSystem/Components`
- 或 `Shared/Components`

取决于它是偏样式基础组件，还是偏业务复合组件。

---

## 15. 状态同步与一致性建议

## 15.1 本地优先展示、服务端校正

适合本地即时更新的内容：

- 猫咪行走/跑步/休息状态
- 宝箱倒计时秒数展示
- 表情飞出动效

必须以后端为准的内容：

- 可用步数
- 宝箱是否可开启
- 背包实例数量
- 穿戴关系
- 合成结果
- 房间成员关系

---

## 15.2 乐观更新范围

建议可以做轻量乐观更新的场景：

- 房间内表情发送动画
- 页面内局部 loading 状态

不建议做强乐观更新的场景：

- 开箱直接本地扣步数
- 合成直接本地销毁 10 个实例
- 房间成员直接本地伪造加入成功

这些都应等待服务端确认。

---

## 16. 权限管理建议

需要管理的主要权限有：

- HealthKit 步数读取权限
- Motion 活动识别权限
- 推送权限（未来可扩展）

建议在 App 启动后不要一次性弹完所有权限，而是在相关功能真正需要时触发。

例如：

- 首页第一次进入时引导开启步数权限
- 房间功能不依赖步数权限，可以独立使用

---

## 17. 测试建议

## 17.1 单元测试

优先覆盖：

- UseCase 逻辑
- ViewModel 状态切换
- ErrorMapper
- Compose 选择规则
- Chest 倒计时状态计算

## 17.2 集成测试

优先覆盖：

- 登录后拉首页
- 步数同步后刷新首页
- 开箱后更新背包
- 合成后更新背包与奖励弹窗
- 房间连接与断线重连

## 17.3 UI 测试

优先覆盖：

- 首次登录流程
- 开箱流程
- 穿戴流程
- 合成选择 10 个材料流程
- 创建房间 / 发送表情流程

---

## 18. 工程实现建议

## 18.1 首选技术路线

建议使用：

- `SwiftUI`：主界面与业务页面
- `Combine` 或 `async/await`：异步状态驱动
- `URLSession`：HTTP
- `URLSessionWebSocketTask` 或成熟封装：WebSocket

如果团队更熟悉 `async/await`，当前版本完全可以以它为主，不一定要全面引入复杂响应式框架。

---

## 18.2 依赖注入

建议通过 `AppContainer` 管理：

- APIClient
- RealtimeClient
- Repositories
- UseCases
- Permission Managers

避免在 View 中到处 `new` 对象。

---

## 19. 后续演进建议

当前客户端结构已经能支持 MVP。后续如有需要，可继续演进：

1. **拆分为多个 Swift Package**
   - Core
   - Shared
   - Feature modules

2. **引入更清晰的本地缓存层**
   - 支持离线展示与更精细的缓存更新

3. **增强房间实时态管理**
   - 房间内更多宠物动作同步
   - 未来扩展多人互动玩法

4. **增加好友与邀请链路**
   - 目前房间邀请可以独立存在
   - 后续再增加好友系统不会破坏现有模块结构

---

## 20. 当前版本结论

当前 iOS 客户端最合适的落地方案是：

- 用 **SwiftUI** 构建页面
- 用 **MVVM + UseCase + Repository** 组织业务
- 用 **Feature 模块化** 组织工程目录
- 用 **HealthKit / CoreMotion** 提供运动能力
- 用 **REST + WebSocket** 组合完成业务请求与实时互动
- 用 **Keychain** 保存游客身份与登录态
- 把“动画展示的即时性”和“资产状态的一致性”明确分层处理

这样可以在不把工程做得过重的前提下，兼顾：

- 开发速度
- 代码可维护性
- 功能继续扩展的空间

