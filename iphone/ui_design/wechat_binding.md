# 微信绑定功能 — 开发说明（增量更新）

## 概述
在"我的（Profile）"界面新增了**微信账号绑定**功能，目的是引导用户在卸载 App 前完成账号绑定，保护其养成数据（小猫、收藏品、好友关系等）不丢失。包含两个相关 UI 组件：

1. **绑定微信卡片**（嵌入在 Profile 屏幕中，常驻显示）
2. **强制提醒浮窗 BindWechatModal**（首次进入 Profile 自动弹出，未绑定时）

## 业务逻辑

### 数据状态
```typescript
interface User {
  // ...其余字段
  wechatBound: boolean       // 是否已绑定微信
  wechatOpenId?: string      // 绑定后存储的 OpenID
  wechatNickname?: string    // 绑定后展示用昵称
  wechatBoundAt?: Date       // 绑定时间
}
```

### 触发规则
- **进入 Profile Tab 1.2 秒后**，若 `wechatBound === false`，自动弹出 `BindWechatModal`
- 用户**点击 Profile 中的"绑定微信卡片"**，立即弹出 `BindWechatModal`
- 用户**点击 Modal 中的"绑定微信"按钮**，调用微信开放平台 SDK（`WXApi.sendAuthReq`）拉起授权 → 后端换取 OpenID → 存储到用户表 → 关闭 Modal → 更新本地状态 → 显示 Toast "微信绑定成功，数据已受保护"
- 用户**点击"稍后再说"**，关闭 Modal；同一 session 内不再弹出（建议用 `@AppStorage("wechatBindModalDismissedAt")` 记录时间，每 24 小时再次弹一次）

## UI 规格

### 1. 绑定微信卡片（Profile 内嵌）

**位置**：Profile 屏幕中，"统计卡片"下方，"最近收藏"上方
**两种状态**：

#### 状态 A — 未绑定（黄色警告卡）
- 容器：背景 `linear-gradient(135deg, #fff8e1 0%, #ffe8b5 100%)`，圆角 18，内边距 12×14，描边 `1.5px solid #ffc94c`
- 左侧图标块：40×40，圆角 12，白底，警告三角图标（颜色 #e89400），白色阴影 `0 2px 6px rgba(255,180,0,0.3)`
- 中间文案（两行）：
  - 主标题："绑定微信，保护小猫数据"（字号 14，800 weight，颜色 #7a4f00）
  - 副标题："未绑定时卸载 App 将丢失全部数据"（字号 11，700 weight，颜色 #a06b00）
- 右侧 CTA：胶囊按钮，圆角 14，背景 #1aad19（微信绿），白字 "立即绑定"（字号 12，800 weight），含微信图标
- 整张卡可点击，点击触发 `BindWechatModal`

#### 状态 B — 已绑定（绿色确认卡）
- 容器：常规 `var(--surface)` 卡片样式
- 左侧图标块：40×40 圆角 12，浅绿底 #e8f7e0，微信图标（#1aad19）
- 主标题："微信已绑定" + 小绿标签 "已保护"（背景 #1aad19，白字，圆角 6）
- 副标题："数据已同步至云端，卸载重装不会丢失"
- 右侧：盾牌图标（#1aad19）

### 2. 强制提醒浮窗 BindWechatModal

**容器**：
- 全屏遮罩：`rgba(0,0,0,0.5)`，淡入 0.25s
- Modal 卡片：左右距 24，垂直居中，圆角 28，padding 24，背景 `var(--surface)`，阴影 `var(--shadow-lg)`
- 入场动画：从下方 20px 上滑 + 淡入，0.32s ease

**内部布局（自上而下）**：

1. **警告插画区**（88×88 圆形）
   - 渐变背景 `linear-gradient(135deg, #fff3d6 0%, #ffd97a 100%)`
   - 大号警告三角图标（46px，颜色 #e89400）
   - 外圈装饰：旋转的虚线圆环（`2px dashed #ffc94c`，向外偏 6px，旋转动画 18s linear infinite）

2. **标题**："数据可能丢失！"（字号 19，800 weight，居中，主色 var(--ink)）

3. **正文**（字号 13，居中，行高 1.6）
   - 默认色 var(--ink-soft)："您还未绑定微信账号，"
   - 高亮红字（颜色 #e15f7c，800 weight）："一旦卸载本 App，您的小猫、收藏品、好友关系等所有数据都将被永久删除，无法恢复。"

4. **数据风险清单**（红色背景框）
   - 容器：`#fff5f5` 背景，圆角 16，padding 12×14，描边 `1px solid #ffe0e0`
   - 4 行风险项，逐行虚线分隔（`1px dashed #ffd0d0`）
     - 🐱 小猫 Lv.8 · 奶团 — 将丢失
     - 💎 36 件收藏品 · 价值 248 钻石 — 将丢失
     - 🏆 15 个成就徽章 — 将丢失
     - 👥 12 位好友关系 — 将丢失
   - 每行：emoji + 描述文字（字号 12，颜色 #7a3a3a，700 weight）+ 右对齐红色"将丢失"标签（字号 10，颜色 #e15f7c，800 weight）
   - **数据从用户真实数据动态填充**，不要硬编码

5. **按钮组**（垂直，间距 10）
   - 主按钮："绑定微信，保护数据"
     - 高 52，圆角 26，背景 #1aad19，白字（字号 15，800 weight）
     - 立体阴影 `0 4px 0 #138a12`（按下时 `translateY(2px)`）
     - 含微信图标（左 8px 间距）
   - 次按钮："稍后再说（数据将不受保护）"
     - 高 40，透明背景，颜色 var(--ink-mute)，字号 12，700 weight
     - 注意括号内文字仍要让用户感到风险

## SwiftUI 实现要点

```swift
struct ProfileView: View {
  @State private var showBindModal = false
  @AppStorage("wechatBound") var wechatBound = false
  @AppStorage("lastWechatPromptAt") var lastPromptAt: Double = 0

  var body: some View {
    ScrollView {
      VStack(spacing: 14) {
        ProfileHeader()
        StatsCard()
        if wechatBound {
          WechatBoundCard()
        } else {
          WechatBindCard { showBindModal = true }
        }
        RecentCollections()
        MoreMenu()
      }
    }
    .onAppear {
      let now = Date().timeIntervalSince1970
      if !wechatBound && now - lastPromptAt > 86400 {
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.2) {
          showBindModal = true
          lastPromptAt = now
        }
      }
    }
    .sheet(isPresented: $showBindModal) {
      BindWechatModal(
        risks: currentUserRisks,   // 动态注入数据
        onBind: handleWechatBind,
        onDismiss: { showBindModal = false }
      )
      .presentationDetents([.fraction(0.78)])
      .presentationCornerRadius(28)
    }
  }

  func handleWechatBind() {
    WechatService.shared.requestAuth { result in
      switch result {
      case .success(let openId):
        Task { try await UserAPI.bindWechat(openId: openId) }
        wechatBound = true
        showBindModal = false
        Toast.show("微信绑定成功，数据已受保护")
      case .failure(let error):
        Toast.show("绑定失败：\(error.localizedDescription)")
      }
    }
  }
}
```

## 微信 SDK 集成

1. 在微信开放平台注册 App，获取 `AppID` 和 `AppSecret`
2. Podfile 添加 `pod 'WechatOpenSDK'`
3. `Info.plist` 配置 URL Scheme（wx + AppID）+ `LSApplicationQueriesSchemes` 添加 `weixin`、`weixinULAPI`、`weixinURLParamsAPI`
4. `AppDelegate` 注册：`WXApi.registerApp(appID, universalLink: ulink)`
5. 处理回调：实现 `WXApiDelegate.onResp` 解析 `SendAuthResp`
6. 后端用 `code` 换取 `access_token` 和 `openid`，关联到用户账号

## 文案要点
- **不要美化风险**：明确说"永久删除，无法恢复"
- **不要使用威胁性语气**：用关怀的语调（"保护小猫数据"而非"否则你会失去一切"）
- **风险清单要具体**：用真实数字（如"36 件收藏品"），让用户感受到具体损失
- **次按钮也要带提醒**："稍后再说（数据将不受保护）" 比单纯"稍后"更有效

## 文件
- `screens/profile.jsx` — Profile 屏幕主体 + WechatBindCard 状态切换 + 自动弹窗逻辑
- `screens/profile.jsx` — `BindWechatModal` 组件（同文件内）
- `components/primitives.jsx` — 新增图标 `Icons.wechat` / `Icons.shield` / `Icons.warn`
- `app.jsx` — `wechatBound` 状态接入根 App，绑定成功后调用 `flashToast`
