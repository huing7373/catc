# iPhone App Visual Review Checklist

> **本文档作用**：作为 PR review 时人工逐项视觉检查 anchor，**合规兜底**，**不**等价 snapshot 测试覆盖度。
> 未来若需要真正的 snapshot 测试，参见 ADR-0002 §3.1 / 后续 spike 评估 swift-snapshot-testing 引入路径。

> **本文档不作用**：
> 1. 不是 PR merge 强 gate（dev 自查走形式即可，PR reviewer 视范围决定是否抽检）；
> 2. 不是 CI 自动化范围（`check_a11y_coverage.sh` / `check_no_apiclient_in_features.sh` 才是 CI 入口）；
> 3. 不替代 UITest（HomeUITests.swift / NavigationUITests.swift 仍是行为契约 baseline）。

## 使用方法

1. PR 触发 view 改动时，dev 在本地跑 `bash iphone/scripts/build.sh` 在 iOS Simulator 启动 App。
2. 手动跑通本文档钦定的 5 屏 + JoinRoomModal 流程；每一项打勾。
3. PR description 内贴本 checklist 副本 + 截图（≥1 张主屏）。
4. Reviewer 抽检 1-2 屏验证 dev 自查无误。

## 6 个 section（5 屏 + 1 modal）检查项

### 1. HomeView（Story 37.7 落地）

- [ ] 顶部 status bar：用户名 + level（`AccessibilityID.Home.userInfo` = `homeStatusBar`）渲染对齐顶部安全区，无超出
- [ ] cat stage（`AccessibilityID.Home.catStage` = `homeCatStage`）：SF Symbol cat.fill 占位渲染中央
- [ ] step balance（`AccessibilityID.Home.stepBalance` = `home_stepBalance`）：步数大字居中渲染，theme.colors.accent 颜色
- [ ] chest area（`AccessibilityID.Home.chestArea` = `home_chestArea`）：宝箱 SF Symbol + 倒计时（`home_chestRemaining`）渲染
- [ ] team idle card create（`AccessibilityID.Home.teamIdleCardCreate` = `homeTeamIdleCard_create`）：创建队伍按钮可见
- [ ] team idle card join（`AccessibilityID.Home.teamIdleCardJoin` = `homeTeamIdleCard_join`）：加入队伍按钮可见
- [ ] version label（`AccessibilityID.Home.versionLabel` = `home_versionLabel`）：底部显示 `v1.0.0 (build 1)`
- [ ] 主题切换（candy → dark）：HomeView 全部颜色 token 跟随切换；无写死 hardcoded color
- [ ] 浮动 TabBar：底部 4 Tab 自绘 TabBar 不遮挡内容（safeAreaInset 已让出空间）

### 2. WardrobeView（Story 37.9 落地）

- [ ] 主容器（`AccessibilityID.Wardrobe.view` = `wardrobeView`）：page 渲染，背景为 theme.colors.pageBg
- [ ] 钻石数（`AccessibilityID.Wardrobe.diamondCount` = `wardrobeDiamondCount`）：右上角显示，金色 chip 风格
- [ ] 合成入口（`AccessibilityID.Wardrobe.composeEntry` = `wardrobeComposeEntry`）：合成按钮可见 + 点击有 os_log
- [ ] 5 个分类 Tab（`wardrobeCategory_hat` / `_bow` / `_scarf` / `_outfit` / `_bg`）：横向滚动，选中态高亮
- [ ] 装备按钮（`AccessibilityID.Wardrobe.equipButton` = `wardrobeEquipButton`）：选中道具时显示
- [ ] 道具网格（`wardrobeItem_<id>`）：3 列 LazyVGrid，未拥有道具半透明（opacity 0.55）+ 锁图标
- [ ] 切换分类后 grid 内容刷新（如点 bow → 显示蝴蝶结道具）
- [ ] 主题切换（candy → dark）：surface / accent / border tokens 全部跟随

### 3. FriendsView（Story 37.10 落地）

- [ ] 主容器（`AccessibilityID.Friends.view` = `friendsView`）：page 渲染
- [ ] 添加好友按钮（`AccessibilityID.Friends.addButton` = `friendsAddButton`）：右上角圆形 IconButton
- [ ] My Room Card（`AccessibilityID.Friends.myRoomCard` = `friendsMyRoomCard`）：顶部 accent 渐变卡 + 分享按钮
- [ ] 分享按钮（`AccessibilityID.Friends.myRoomShareButton` = `friendsMyRoomShareButton`）：占位 toast 触发
- [ ] 2 个 Tab（`friendsTab_online` / `_all`）：segmented control 切换可见
- [ ] FriendRow（`friendRow_u1` 等）：头像 + 昵称 + 状态 chip + action button 三态
- [ ] 在线好友 action button（`friendActionButton_u1` 邀请；`friendActionButton_u3` 加入）：颜色 + 文案不同
- [ ] toast（`AccessibilityID.Friends.toast` = `friendsToast`）：分享 / 邀请触发后底部弹起 + 自动消失

### 4. ProfileView（Story 37.11 落地）

- [ ] 主容器（`AccessibilityID.Profile.view` = `profileView`）：page 渲染
- [ ] header card（`AccessibilityID.Profile.headerCard` = `profileHeaderCard`）：accent 渐变背景 + 用户头像 + 昵称
- [ ] stats card（`AccessibilityID.Profile.statsCard` = `profileStatsCard`）：3 列统计（步数 / 宝箱 / 装扮）
- [ ] WeChat 未绑定卡（`AccessibilityID.Profile.weChatCard` = `profileWeChatCard`）：黄色警告渐变 + warn icon
- [ ] WeChat 已绑定卡 toggle（`AccessibilityID.Profile.weChatCardBound` = `profileWeChatCardBound`）：surface 背景 + 已保护 chip
- [ ] 4 个菜单项（`profileMenu_achievements` / `_messages` / `_favorites` / `_settings`）：顺序 + chevronRight
- [ ] WeChat Modal（`AccessibilityID.Profile.weChatModal` = `profileWeChatModal`）：未绑定卡 tap 弹出
- [ ] modal 内主按钮（`AccessibilityID.Profile.weChatBindButton` = `profileWeChatBindButton`）：绿色微信按钮
- [ ] modal 内次按钮（`AccessibilityID.Profile.weChatCancelButton` = `profileWeChatCancelButton`）：稍后再说
- [ ] toast（`AccessibilityID.Profile.toast` = `profileToast`）：collection view-all / menu 占位触发

### 5. RoomView（Story 37.8 落地）

- [ ] 返回按钮（`AccessibilityID.Room.returnButton` = `returnButton`）：左上角圆形 IconButton
- [ ] 房间号显示（`AccessibilityID.Room.roomIdDisplay` = `roomIdDisplay`）：22pt 800 monospaced 字 + 3pt tracking
- [ ] 复制按钮（`AccessibilityID.Room.copyButton` = `copyButton`）：tap 复制 → 1.2s 反馈动画
- [ ] shared stage（`AccessibilityID.Room.sharedStage` = `sharedStage`）：粉橙渐变 Card + 4 emoji + MiniCat
- [ ] 4 个成员位（`roomMember_0` / `_1` / `_2` / `_3`）：占位 dashed border 行（"+ 等待好友加入"）
- [ ] 离开按钮（`AccessibilityID.Room.leaveButton` = `leaveButton`）：底部 PrimaryButton secondary variant
- [ ] 主题切换（candy → dark）：所有 surface / border / accent tokens 跟随；shared stage 渐变保留（fixed colors）

### 6. JoinRoomModal（Story 37.12 落地）

- [ ] modal 容器（`AccessibilityID.JoinRoomModal.modal` = `joinRoomModal`）：surface 背景 + 28pt 圆角
- [ ] 关闭按钮（`AccessibilityID.JoinRoomModal.closeButton` = `joinRoomCloseButton`）：右上角 32pt 圆形
- [ ] 输入框（`AccessibilityID.JoinRoomModal.input` = `joinRoomInput`）：accent 描边 + 20pt monospaced
- [ ] 取消按钮（`AccessibilityID.JoinRoomModal.cancelButton` = `joinRoomCancelButton`）：左半 secondary
- [ ] 确认按钮（`AccessibilityID.JoinRoomModal.confirmButton` = `joinRoomConfirmButton`）：右半 primary，输入空时 disabled
- [ ] 输入超过 64 字符自动截断（JoinRoomInputNormalizer.normalize）
- [ ] 点 close button → modal dismiss
- [ ] 点 confirm（输入有效）→ modal dismiss + 切到 RoomView

## 截图位

| 屏 | 浅色（candy） | 深色（dark） |
|---|---------------|--------------|
| HomeView | screenshots/home-candy.png | screenshots/home-dark.png |
| WardrobeView | screenshots/wardrobe-candy.png | screenshots/wardrobe-dark.png |
| FriendsView | screenshots/friends-candy.png | screenshots/friends-dark.png |
| ProfileView | screenshots/profile-candy.png | screenshots/profile-dark.png |
| RoomView | screenshots/room-candy.png | screenshots/room-dark.png |
| JoinRoomModal | screenshots/join-room-modal-candy.png | screenshots/join-room-modal-dark.png |

> 截图目录 `iphone/docs/screenshots/` 当前不强制 commit（PR 审阅时用本地截图即可，避免 git 仓库膨胀）。
