---
date: 2026-04-30
source_review: file:/tmp/epic-loop-review-37-14-r1.md（codex review round 1）
story: 37-14-design-package-白名单
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-30 — 白名单条目必须 cite「完整流程文档 / 真实渲染路径」而非只挂视觉壳入口

## 背景

Story 37.14 落地 `iphone/docs/ui-design-scope-whitelist.md`，登记 Epic 37 显式选择不做的 10 条 ui_design 元素。codex round-1 review 抓到 2 条 P2 文档准确性 finding：均指向「白名单条目的"位置"段只引用了 view 视觉壳的挂载点 / 占位入口，但读者顺着该路径打开看到的不是被 deferred 的真实流程或真实渲染机制」。本次修订把"位置"段从「视觉壳入口」扩到「视觉壳入口 + 真实流程文档 / 真实渲染源文件」。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | WeChat binding 「位置」段漏引完整 OAuth 流程文档 | medium | docs | fix | `iphone/docs/ui-design-scope-whitelist.md`（条目 3） |
| 2 | cat-stage 装扮 overlay 描述错位（emoji vs vector shapes / 错指 home.jsx:52） | medium | docs | fix | `iphone/docs/ui-design-scope-whitelist.md`（条目 9） |

## Lesson 1: 白名单条目「位置」段必须包含完整流程文档（不只是视觉壳挂载点）

- **Severity**: medium
- **Category**: docs
- **分诊**: fix
- **位置**: `iphone/docs/ui-design-scope-whitelist.md`（条目 3「wechat_binding 真实 OAuth 流程」）

### 症状（Symptom）

条目 3 「位置」段只引 `profile.jsx:75-118`（绑定卡片）+ `profile.jsx:194-239`（绑定浮窗）—— 这是视觉壳里"绑定微信"的两个入口控件。但 ui_design 工程内**另有**一份文档 `iphone/ui_design/wechat_binding.md` 钦定了完整 OAuth / SDK 集成流程（唤起授权 / 回调换 code / server 换 unionId / openId / SDK 选型）。读者顺着「位置」段只看 profile.jsx 视觉壳，会以为 deferred scope 仅是「按钮 + toast 占位壳」，漏掉**真正被 defer 的内容**：完整 OAuth 端到端流程。

### 根因（Root cause）

写白名单时把"位置"段当作「在 ui_design view 里这个不做项视觉占位是哪几行代码」的 GPS 标记。这对纯视觉占位（如 cat 3D 模型 / emoji 道具）成立 —— ui_design 内只有视觉占位，没有更深的流程文档。但对**功能复合**的 deferred scope（OAuth、推送、商城支付等），ui_design 工程内可能并存 `*.md` 流程文档（产品/技术规格），它们才是真正描述 deferred 行为的"位置"。仅指向 view 视觉壳会让读者低估 deferred scope 的体量。

### 修复（Fix）

条目 3「位置」段在 profile.jsx 行号后追加：

```diff
- - **位置（ui_design）**：`iphone/ui_design/source/screens/profile.jsx` line 75-118（绑定微信卡片）+ line 194-239（绑定微信浮窗）
+ - **位置（ui_design）**：`iphone/ui_design/source/screens/profile.jsx` line 75-118（绑定微信卡片）+ line 194-239（绑定微信浮窗）+ `iphone/ui_design/wechat_binding.md`（完整 OAuth / SDK 集成流程文档：唤起微信授权、回调换 code、server 换 unionId / openId 的端到端时序与 SDK 选型）
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **登记 scope-whitelist 条目时**，**必须** 在 `iphone/ui_design/` 根目录 `find -name "*.md"` 一遍，把任何与 deferred scope 主题字面相关的流程文档（如 `wechat_binding.md`、`payment_*.md`、`push_*.md`）一并写进「位置」段，而非只引 view 视觉壳的代码行号。
>
> **展开**：
> - 「位置」段的语义是「这个 deferred scope 在 ui_design 工程内有哪些 artifact」，包括：① view 视觉壳代码行号（如 profile.jsx:75-118）；② 流程文档 `*.md`（如有）；③ 组件级源文件（如有 vector shape 占位等非纯 emoji 元素）。
> - 写每条新条目前，先做这两个 grep / find：
>   - `find iphone/ui_design -iname "*<主题关键词>*"`（找同名 md / 同名组件）
>   - `grep -rn "<主题关键词>" iphone/ui_design/source/`（找隐藏在其他 view 内的引用）
> - **反例**：条目 3 只写「`profile.jsx` line 75-118 + 194-239」，让读者以为 wechat 绑定的 deferred scope 只是「壳里两段 SwiftUI 视觉」。真实 deferred scope 远大于此 —— OAuth 时序 + SDK 选型 + server 端 unionId 逻辑都钦定在 `iphone/ui_design/wechat_binding.md` 里。漏引该 md 让 PM / dev 在「踢出白名单」决策时低估工作量。

## Lesson 2: 描述 deferred 视觉占位时必须实读组件源码确认真实渲染机制（emoji vs vector shapes vs SF Symbol）

- **Severity**: medium
- **Category**: docs
- **分诊**: fix
- **位置**: `iphone/docs/ui-design-scope-whitelist.md`（条目 9「装扮道具 emoji 占位」）

### 症状（Symptom）

条目 9 旧文：

> 位置：`wardrobe.jsx` line 9-10（道具 grid 占位 🎩 / 🎀 等 emoji）+ `home.jsx` line 52 的 cat stage 装扮 overlay 占位
> 二分边界：「猫身上 emoji」节点 10 起替换；「仓库 grid emoji」永不替换。

但 `home.jsx:52` 实际只 mount `<CatPlaceholder size={220} mood={mood} label="猫 3D 模型"/>`，cat stage 装扮 overlay 的真实渲染**不在** home.jsx，而在 `iphone/ui_design/source/components/cat-placeholder.jsx` line 39-58 内：bow / hat / scarf 三种装扮用 `<path>` / `<rect>` / `<ellipse>` / `<circle>` 等 SVG vector shapes 勾勒，**完全不是 emoji**。所以「猫身上 emoji 节点 10 起替换」这句二分边界中的"emoji"用词错位 —— Story 30.1-30.4 SpriteRenderer 真正替换的是这些 vector shape overlay，不是 emoji。

### 根因（Root cause）

写白名单时对 view 内的 `<CatPlaceholder>` 这类自定义组件做了"按 view 内出现的代码行号定位 + 凭直觉描述视觉占位形态"。直觉根据是「wardrobe.jsx grid 用 emoji 占位 → 假设 cat stage 装扮也是 emoji」。这是一种**类比谬误**：同一个 epic 内不同 view 的占位策略可能不同（grid 用 emoji + overlay 用 vector / SF Symbol），不实读组件源码会得到错误的视觉特征断言。

### 修复（Fix）

条目 9 改写为：

```diff
- - **位置（ui_design）**：`iphone/ui_design/source/screens/wardrobe.jsx` line 9-10（道具 grid 占位 `🎩` / `🎀` 等 emoji）+ `iphone/ui_design/source/screens/home.jsx` line 52 的 cat stage 装扮 overlay 占位
+ - **位置（ui_design）**：`iphone/ui_design/source/screens/wardrobe.jsx` line 9-10（道具 grid 占位 `🎩` / `🎀` 等 emoji）+ `iphone/ui_design/source/components/cat-placeholder.jsx` line 39-58（cat stage 上的 bow / hat / scarf 装扮 overlay 由 `<path>` / `<rect>` / `<ellipse>` 等 SVG vector shapes 勾勒，非 emoji；home.jsx line 52 仅 mount `<CatPlaceholder>`，真实 accessory 渲染在 cat-placeholder.jsx 内）
```

并改述二分边界：「猫身上 vector shape overlay」节点 10 起 Story 30.1-30.4 替换为 SpriteRenderer 图像渲染；「仓库 grid emoji」永不替换。同时把对应 Story 编号从虚指的 "Story 30.x" 锁实到 "Story 30.1-30.4"（对应 RenderConfig 数据模型 / SpriteRenderer 封装 / EquippedCosmeticView 升级 / 槽位锚点常量化）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **描述 deferred 视觉占位的渲染形态（emoji / vector / SF Symbol / 图像）时**，**必须** 实读真实渲染源文件（特别是 `<XxxPlaceholder>` / `<XxxOverlay>` 之类的自定义组件），不能凭"同 view 内其他元素的占位形态"做类比假设。
>
> **展开**：
> - 「位置」段引用的代码行号必须指向**真实渲染发生的源文件**。如果 view 内只是 mount 一个自定义组件（`<CatPlaceholder>`、`<EquippedCosmeticView>` 等），「位置」必须**穿透到组件源文件**，不能停在 view 的 mount 行。
> - 描述视觉形态前先 `Read` 组件源码扫一眼 SVG / SwiftUI 标签，确认是 `<path>` (vector) / `Image(systemName:)` (SF Symbol) / 字面 emoji string / `Image(<file>)` (asset) 中哪一种。
> - 引 Story 编号时具体到子号（`Story 30.1-30.4`），不要用 `Story 30.x` 这种虚指 —— 后者在「踢出白名单」决策时无法机械对齐到具体 AC。
> - **反例**：旧文「`home.jsx` line 52 的 cat stage 装扮 overlay 占位」+「猫身上 emoji 节点 10 起替换」。这句话在两个层面错：① home.jsx:52 不渲染 accessory，只 mount placeholder；② 真实 overlay 是 SVG vector shapes 不是 emoji。「踢出白名单」流程会基于这条错误描述去找替换工作量，而漏掉 cat-placeholder.jsx 这个真正要被改的文件。

---

## Meta: 本次 review 的宏观教训

两条 finding 指向同一组思维漏洞：**「位置」段被理解成"在视觉壳里这一项的占位代码出现在哪儿"，而非"这个 deferred scope 在 ui_design 工程内的全部 artifact"**。前者是 GPS 标记，后者是 scope 描述。当 deferred scope 是「功能复合」（条目 3：含完整 OAuth 流程文档）或「占位机制非最浅显形态」（条目 9：cat overlay 是 SVG vector 不是 emoji），仅写视觉壳行号会**低估** deferred scope 体量 / **错描** 占位形态。

未来写 scope-whitelist 类文档时，「位置」段的写作 SOP：

1. **find 同名 md** —— `find iphone/ui_design -iname "*<主题>*"`，把流程文档一并引入。
2. **穿透自定义组件** —— view 内 mount `<XxxPlaceholder>` 时，必须 Read 组件源文件确认真实渲染机制，并把组件源文件路径 + 行号写进「位置」段。
3. **形态字面化** —— 描述占位时只写实测过的形态字面（"SVG vector shapes" / "SF Symbol cat.fill" / "字面 emoji string `🎩`"），禁止凭类比假设。
4. **Story 编号锁实到子号** —— 引 Story 30.x / 27.x 时具体到 30.1 / 30.2 / 30.3 / 30.4，避免在「踢出白名单」决策时虚指。
