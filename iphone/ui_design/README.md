# Handoff: 小猫养成 App（Cat Companion App）

## Overview
这是一个 iPhone 端的"小猫养成 + 社交组队 + 装扮收集"App 的 UI 设计原型。用户可以养一只虚拟小猫，给它喂食、抚摸、玩耍；通过房间代码与好友组队互访，在共享房间内一起玩耍；并通过仓库系统收集和装扮帽子、饰品、围巾等道具。

主要业务目标：
- 养成与情感连接：用户与小猫建立长期关系
- 社交粘性：通过组队/邀请系统增加好友互动
- 收集驱动：通过装扮道具的稀有度（N/R/SR/SSR）激励留存

## About the Design Files
本包内的 HTML/JSX 文件是 **设计参考**（高保真原型），用于演示视觉与交互意图，**不是可直接发布的生产代码**。开发任务是把这些设计在目标 iOS 工程中重新实现。

推荐技术选型：
- **首选 SwiftUI**（原生 iOS）—— 与 iOS 26 Liquid Glass 风格一致，性能与体验最佳
- 备选 React Native / Flutter —— 如需跨端

设计参考使用 React + 内联 JSX 实现，运行在浏览器内的 iOS 设备框架（IOSDevice）中模拟 iPhone 的视觉。请按 SwiftUI（或所选框架）原生模式重写组件，不要直接搬运 React 代码。

## Fidelity
**高保真（Hifi）** —— 颜色、字号、间距、圆角、阴影、动效、文案均已确定。开发应按此实现像素级还原。占位符例外：

- 小猫本体目前是 **SVG 占位符**（带条纹纹理 + "猫 3D 模型" 标签）。生产环境需要替换为真实的 3D 模型（建议 USDZ + RealityKit / SceneKit）或高质量插画。所有装扮道具的图标也是 emoji 占位，需要替换为美术资源。

## App 结构（4 Tab + 互斥状态）

底部 Tab Bar 共 4 个入口：
1. **家（Home）** — 主养成界面
2. **仓库（Wardrobe）** — 装扮道具收藏 + 试衣间
3. **好友（Friends）** — 好友列表 + 邀请/加入
4. **我的（Profile）** — 用户信息 + 设置

> ⚠️ **关键互斥逻辑**：Home Tab 内有两个互斥状态。当用户**未加入队伍**时显示 `HomeScreen`（养成 + 创建/加入队伍按钮）；当用户**已加入队伍**时显示 `RoomScreen`（队伍房间界面），**完全替换** Home。Tab 仍然是"家"，但视图切换。`RoomScreen` 的"离开房间"按钮回到 `HomeScreen`。

## Screens / Views

### 1. HomeScreen（首页 - 未加入队伍）

**用途**：用户与自家小猫互动；查看状态；发起组队
**布局**：垂直滚动；从上到下：
1. 顶部信息条（天气问候 + 步数计）
2. 小猫舞台卡片（3D 模型 + 等级名牌 + 三条状态条：饱食/心情/活力）
3. 三个互动按钮（喂食/抚摸/玩耍）—— 点击会从猫身上飘出 emoji（🍥/💕/⭐）
4. 底部组队 CTA 卡片（粉色渐变，含"创建队伍" + "加入队伍"两个按钮）

**关键交互**：
- 点击"创建队伍" → 立刻进入 RoomScreen，自动生成 6 位房间代码（格式：3 字母-2 数字，如 `7K3-P2`）
- 点击"加入队伍" → 弹出 `JoinRoomModal` 输入代码

### 2. RoomScreen（首页 - 已加入队伍）

**用途**：与队友共处的虚拟空间；显示房间代码以邀请他人
**布局**：
1. 顶部：返回按钮 + 标题"队伍房间 / {猫名}的小屋"
2. 房间代码卡片（大字号代码 + 复制按钮，复制后变绿打勾 1.2s）
3. 共享舞台（粉橙渐变背景 + 云朵装饰 + 鱼/毛线球元素），多只 MiniCat 在底部上下弹跳（错峰动画 0.2s 间隔）
4. 成员列表（最多 4 人；空位显示虚线占位 "+ 等待好友加入"），队长有"队长"小标签
5. 底部"离开房间"按钮（次要按钮样式）

### 3. WardrobeScreen（仓库 - 3D 试衣间式）

**用途**：浏览拥有的装扮 / 装备到小猫身上 / 查看未解锁道具
**布局**：
1. 顶部：标题"{猫名} 的衣柜" + 收藏数（36/53）+ 钻石货币 (248)
2. **预览区**（关键 UX）：左侧是带当前装扮的小猫 3D 预览，右侧显示当前选中道具名 + 稀有度 + 已拥有/未解锁标签 + 装备/卸下按钮
3. **分类 Tab**（横向滚动）：帽子 🎩 / 饰品 🎀 / 围巾 🧣 / 服装 👘 / 背景 🏞️
4. **网格列表**（3 列）：每个道具一个方形卡片，含图标 + 名字 + 稀有度色条；未拥有的卡片半透明 + 锁图标；已装备的右上角有绿色对勾

**装扮系统数据模型**：
```
Item { id, name, category: 'hat'|'bow'|'scarf'|'outfit'|'bg', rarity: 'N'|'R'|'SR'|'SSR', owned: boolean }
Equipment { hat?: itemId, bow?: itemId, scarf?: itemId, ... }
```

**稀有度配色**：N=灰 #b0b0b0，R=蓝 #7db3e8，SR=紫 #c58ae8，SSR=金红渐变 `linear-gradient(90deg,#ffd166,#ef476f)`

### 4. FriendsScreen（好友）

**用途**：查看好友状态、邀请好友、加入好友房间
**布局**：
1. 顶部：在线人数统计 + 添加好友按钮（+）
2. 我的房间提示条（仅当用户在房间中时显示，含房间代码可直接分享）
3. Tab：在线 / 全部
4. 好友列表行：头像（带在线小绿点）+ 名字 + "房间中"角标 + 状态文字（"在房间 7K3-P2 中玩耍"）+ 右侧操作按钮

**操作按钮逻辑**：
- 好友在房间中 → 显示"加入"（实心按钮）
- 好友在线但不在房间 → 显示"邀请"（描边按钮）。如果当前用户没有房间，邀请会先自动创建房间再发送
- 好友离线 → 显示"离线"灰字

### 5. ProfileScreen（我的）

**用途**：个人信息 / 收藏成就总览 / 设置入口
**布局**：
1. 顶部渐变头图（粉色渐变背景）：头像（带光环描边）+ 用户名 + 用户 ID + 称号（"见习铲屎官"）+ "加入于 2024年3月15日"小药丸
2. 统计卡片（覆盖在头图底部）：4 列 - 收藏品 / 好友 / 小猫等级 / 成就，每列含图标 + 数值 + 标签
3. 最近收藏（横向滚动卡片，5 个）
4. 更多菜单列表：成就徽章 / 消息通知 / 喜欢的道具 / 设置

### 6. JoinRoomModal（加入房间弹窗）

**触发**：HomeScreen 点击"加入队伍"
**布局**：底部居中弹出（带遮罩，0.45 透明度黑），白色卡片，含：
- 标题"加入队伍" + 关闭按钮
- 说明文字
- 大输入框（猫爪图标 + 等宽字体，3px 字距，自动转大写）
- 格式提示"3 个字母 - 2 位数字"
- 取消 / 确定加入 按钮（≥3 字符才启用）

## Design Tokens

### Colors（CSS 变量，3 套主题）

**糖果粉（默认 candy 主题，浅色）**
```
--page-bg:    #f7e9e0
--accent:     #ff8fa3   /* 主品牌色 */
--accent-soft:#ffd6df   /* 主色浅版 - 背景/标签 */
--accent-deep:#e15f7c   /* 主色深版 - 文字/按钮按下 */
--surface:    #fff9f5   /* 卡片底色 */
--surface-2:  #fff1e8   /* 次级容器底色 */
--ink:        #4a2c36   /* 主文字 */
--ink-soft:   #8b6b75   /* 次要文字 */
--ink-mute:   #b99ba5   /* 弱化文字 / 描边 */
--success:    #7bc47f
--warn:       #ffb26b
--coin:       #ffb84d   /* 货币/金色 */
--border:     rgba(74,44,54,0.08)
```

**抹茶（matcha）**：accent #94b97c, accent-soft #dfe8c8, accent-deep #63894a
**天空（sky）**：accent #7bb3e0, accent-soft #cfe2f2, accent-deep #4e86b6
**深色模式**：page-bg #2a1c22, surface #3a2831, ink #fbe5ec（其余配色自动反转）

### Spacing
8 / 10 / 12 / 14 / 16 / 18 / 20 / 22 / 28

### Border Radius
- 小标签：6 / 8
- 中等元素：12 / 14 / 16
- 卡片：18 / 20 / 22 / 24
- 大卡片 / Modal：26 / 28
- 按钮：高度的一半（圆药丸）
- 头像 / 圆点：50%

### Shadows
```
--shadow-sm: 0 2px 0 rgba(180,100,120,0.08)              /* 卡片 */
--shadow-md: 0 6px 16px rgba(180,100,120,0.14)           /* Tab Bar / 主要卡片 */
--shadow-lg: 0 14px 38px rgba(180,100,120,0.18)          /* Modal */
```
按钮使用立体感"硬阴影"：`0 4px 0 var(--accent-deep)`，按下时 `translateY(2px)` 模拟回弹

### Typography（字号 / 字重）
- 大标题：22 / 800
- 中标题：17-18 / 800
- 卡片标题：14-15 / 800
- 正文：13 / 600-700
- 辅助文字：11-12 / 700
- 微小标签：9-10 / 800

字体家族：默认 `Nunito + Noto Sans SC`（圆润字体）；可选 `Baloo 2`（更卡通）/ `LXGW WenKai TC`（手写感）。**iOS 实现建议使用 SF Pro Rounded** + 中文字体 PingFang SC。

### iOS 设备规格（设计画布）
- 设计宽度：402px（iPhone 15 Pro 视区）
- 设计高度：874px
- 状态栏高度：约 62px（页面顶部 padding 设为 68px 避开）
- 底部 Tab Bar：浮动，高 72px，距底 14px，距左右各 12px
- Home Indicator：底部 34px 区域

## Interactions & Behavior

### 动画
- Tab 切换：内容淡入 + 上移（fadeIn 0.28s ease）
- 互动按钮按下：translateY(2px) 0.1s
- 喂食/抚摸/玩耍后：emoji 从猫身上飘起（floatUp 1.4s：0%→25% 渐显放大上移 -20px，→100% 缩小消失到 -110px）
- 房间内 MiniCat：上下弹跳 bounce 2.2s ease-in-out infinite，错峰 0.2s
- Modal 出现：背景渐入 0.2s + 卡片从下方 20px 上滑 0.3s
- Toast：底部上滑出现，1.8s 后消失
- 复制成功：按钮变绿 + 显示对勾，1.2s 后还原

### 状态机
```
HomeState: idle ⟷ inRoom
  - idle  → inRoom: 通过 createTeam() 或 joinTeam(code)
  - inRoom → idle: 通过 leaveRoom()
```

### 关键流程
**创建队伍流程**：HomeScreen → 点击"创建队伍" → 生成随机房间代码 → 替换 HomeScreen 为 RoomScreen → 等待好友通过代码加入

**加入队伍流程**：HomeScreen → 点击"加入队伍" → 弹出 JoinRoomModal → 用户输入代码（自动大写、限 8 字符）→ 点击"确定加入" → Modal 关闭 → 切换为 RoomScreen，成员列表预填队长 + 自己

**邀请好友流程**：FriendsScreen → 点击好友"邀请" → 若用户没房间则自动创建 → 显示 Toast "已邀请 {名字} 加入房间"

**加入好友房间流程**：FriendsScreen → 点击"在房间中"好友的"加入" → 解析 statusText 中的房间代码 → 调用 joinTeam(code) → 自动跳转到首页 Tab 显示 RoomScreen

## Data Models

```typescript
interface User {
  id: string
  name: string
  title: string         // 称号
  joinedAt: Date
  catLevel: number
  collectionCount: number
  friendCount: number
  achievementCount: number
}

interface Cat {
  name: string
  level: number
  mood: 'happy' | 'sad' | 'neutral'
  stats: { hunger: number; mood: number; energy: number }  // 0-100
  equipment: { hat?: string; bow?: string; scarf?: string; outfit?: string; bg?: string }
}

interface Item {
  id: string
  name: string
  category: 'hat' | 'bow' | 'scarf' | 'outfit' | 'bg'
  rarity: 'N' | 'R' | 'SR' | 'SSR'
  owned: boolean
  asset: string         // 美术资源路径
}

interface Friend {
  id: string
  name: string
  avatarColor: string
  online: boolean
  status: 'online' | 'inRoom' | 'offline'
  statusText: string
  currentRoomCode?: string
}

interface Room {
  code: string          // 格式 'XXX-NN'
  hostId: string
  members: Array<{ userId: string; catName: string; catLevel: number; isHost: boolean }>
  maxSize: 4
}
```

## Assets 需要替换的资源

1. **小猫主体（最重要）**：当前是 SVG 占位符（带条纹纹理）。生产需替换为：
   - 3D 模型（USDZ 格式，可在 RealityKit 中渲染）；或
   - 一组高质量的 2D Lottie 动画（待机/开心/喂食/抚摸/玩耍/睡觉）

2. **装扮道具图标**：当前用 emoji（🎩🎀🧣👘🏞️）。生产需替换为美术插画（推荐 PNG 透明 / Lottie / SVG）

3. **小图标**：原型中所有界面图标都是 inline SVG（线性圆角风），iOS 实现可直接用 SF Symbols（推荐 Rounded variant），名称对照见 `components/primitives.jsx` 中的 `Icons` 对象

4. **字体**：iOS 端使用系统 SF Pro Rounded + PingFang SC，无需引入第三方字体

## Files

设计源文件结构：
```
index.html                       # 入口，定义主题 CSS 变量 + 字体
app.jsx                          # 根 App 组件，状态管理 + 路由
ios-frame.jsx                    # iPhone 设备外框（开发时不需要，仅原型用）
tweaks-panel.jsx                 # 主题切换调试面板（开发时不需要）
components/
  primitives.jsx                 # Icons 集 / PrimaryButton / Card / Avatar / FadeIn
  cat-placeholder.jsx            # CatPlaceholder + MiniCat（待替换为真实资源）
  tab-bar.jsx                    # 底部 4 Tab 导航
screens/
  home.jsx                       # HomeScreen + StatusBar + ActionButton + TeamIdleCard
  room.jsx                       # RoomScreen
  wardrobe.jsx                   # WardrobeScreen + RarityTag
  friends.jsx                    # FriendsScreen + FriendRow
  profile.jsx                    # ProfileScreen + Stat / Divider / SectionHeader
```

## 开发建议

1. **优先用 SwiftUI 重写**——所有圆角卡片、模糊毛玻璃、弹簧动画都能在 SwiftUI 中原生实现得更流畅
2. **状态管理用 `@Observable` / TCA**——`hasTeam` 是全局状态需跨 Tab 共享
3. **房间系统需后端**——房间代码生成、成员同步、邀请通知建议用 WebSocket 或 Firebase Realtime
4. **小猫 3D 模型**先用占位实现整体骨架，再迭代美术资源
5. **3 套主题色**可保留作为用户个性化设置，写成 `@Environment(\.theme)`
