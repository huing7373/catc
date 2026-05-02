---
date: 2026-04-30
source_review: file:/tmp/epic-loop-review-37-14-r3.md (codex r3)
story: 37-14-design-package-白名单
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-30 — 白名单未来 routing 必须 cite 真实 Story scope（不能把 3D spike 错挂 Story 30.x、不能把 SwiftUI 实装路径错写成 prototype 替换）

## 背景

Story 37.14 r2 修完 ui-design-scope-whitelist.md 的 cite + render path 区分，r3 codex 发现条目 8 / 9「何时做 / 替换路径」段仍有两条 epic-routing 误指：

1. 条目 8（cat 3D 模型）r2 修后写「节点 10 起 Story 30.x 落地 RealityKit / USDZ 3D model 替换 SwiftUI 实装侧」—— 但 epics.md Story 30.1-30.4 实际是 2D cosmetic rendering（RenderConfig / SpriteRenderer / EquippedCosmeticView），完全不涉及 USDZ / RealityKit
2. 条目 9（装扮 emoji）r2 修后写「节点 10 起 Story 30.x 替换 cat-placeholder.jsx 的 vector shape overlay」—— 但 Story 30.x 是定义在 SwiftUI 实装路径（SpriteRenderer / EquippedCosmeticView），不是替换 ui_design prototype 的 jsx artifact

两处都是「未来 routing 锚点指错 epic / 错 surface」—— 该文件作用是 future epic 启动时的 scope-exit 权威指南，错指会让后续 dev / PM 跑偏。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 条目 8 cat 3D 模型 routing 错指 Story 30.x | medium | docs | fix | `iphone/docs/ui-design-scope-whitelist.md:91` |
| 2 | 条目 9 装扮 emoji 替换路径错写 prototype 替换 | medium | docs | fix | `iphone/docs/ui-design-scope-whitelist.md:97-98` |

## Lesson 1: 白名单条目「何时做 / future routing」必须事先核对被 cite Story 的真实 scope，不能凭名字相近脑补

- **Severity**: medium
- **Category**: docs
- **分诊**: fix
- **位置**: `iphone/docs/ui-design-scope-whitelist.md:91`

### 症状（Symptom）

条目 8（猫 3D 模型）「理由」段写：「节点 10 起 Story 30.x 落地 RealityKit / USDZ 3D model 替换的是 SwiftUI 实装侧的 cat.fill 占位」。但 epics.md 中 Story 30.1（RenderConfig 数据模型）/ 30.2（SpriteRenderer 封装）/ 30.3（EquippedCosmeticView 升级）/ 30.4（槽位锚点常量化）全部是 **2D cosmetic 渲染**链路，scope 关键词是 RenderConfig / SpriteRenderer / 槽位锚点，**完全不涉及** USDZ / RealityKit / 3D model。

### 根因（Root cause）

- 误把 Story 30.x（节点 10 装扮渲染升级）的「视觉升级」语义直接挪用到「3D 模型升级」上 —— 两者都是「placeholder → 真材实料」的视觉升级，但**实现栈完全不同**（2D sprite vs 3D mesh，框架也不同：UIKit/SwiftUI Image vs RealityKit/SceneKit）
- 写文档时只看了 epic 编号 + 「节点 10 美化」标签，没回去 grep epics.md 里 Story 30.1-30.4 的 acceptance criteria 验证 scope
- 「节点 10 视觉升级」语义层在脑子里被压缩成「所有视觉占位的最终替换都走 Story 30.x」—— 但 3D 猫属于另起 spike（美术资源依赖 + 渲染栈切换）

### 修复（Fix）

把条目 8 理由段错误句子：

> 节点 10 起 Story 30.x 落地 RealityKit / USDZ 3D model 替换的是 **SwiftUI 实装侧**的 `cat.fill` 占位（不是 ui_design prototype 的 SVG CatPlaceholder —— prototype 不上线、不需替换）。

改为正确语义：

> 3D 模型属另起 spike，**非** Story 30.x（Story 30.1-30.4 是 2D cosmetic 渲染：RenderConfig + SpriteRenderer + EquippedCosmeticView + 槽位锚点，scope 完全不同；ui_design prototype 不上线、不需替换）。

并补一条 `[Source: epics.md#Story 30.1-30.4]` cite 让读者直接核验 scope。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **白名单 / scope 文档「何时做 / 后续做 / future routing」段写「节点 N 起 Story X.Y 替换 / 落地 Z」时**，**必须先 grep 被 cite 的 Story 标题 + acceptance criteria** 验证 Story scope 与「Z」语义一致；**禁止仅凭节点号 + 视觉升级标签脑补 routing**。
>
> **展开**：
> - cite Story X.Y 前必跑 `grep -n "Story X\.Y" _bmad-output/planning-artifacts/epics.md`，读 Story 标题 + AC 至少前 3 条，验证 scope 关键词（RenderConfig / SpriteRenderer / RealityKit / USDZ 等）与文档主张一致
> - 「节点 10 视觉升级」不是单一 Story —— 同节点可能含「2D sprite 替换」「3D 模型」「动画」等多条独立 spike；不能把 placeholder 类视觉升级一律 routing 到 Story 30.x
> - 当不确定 Story 是否覆盖某 scope 时，写「另起 spike，**非** Story 30.x（Story 30.x 是 ABC，scope 不同）」更安全 —— 把不确定性显式化、把 Story 30.x scope 同时写出来便于读者交叉核验
> - **反例**：在白名单条目里写「节点 10 起 Story 30.x 落地 RealityKit / USDZ 3D model」，而 Story 30.x 实际是 2D sprite 渲染 —— 后续 epic 启动时会被白名单文档误导，跑偏到错误 epic

## Lesson 2: 白名单条目「替换路径」必须区分 prototype / SwiftUI 实装侧 / inventory grid 三层 surface，不能笼统说「Story X.Y 替换 prototype 的 jsx」

- **Severity**: medium
- **Category**: docs
- **分诊**: fix
- **位置**: `iphone/docs/ui-design-scope-whitelist.md:97-98`

### 症状（Symptom）

条目 9（装扮 emoji 占位）「理由 / 何时做」段写「节点 10 起 Story 30.1-30.4 落地 ... 替换 cat-placeholder.jsx 的 vector shape overlay」。但 Story 30.x 在 epics.md 里定义在 **SwiftUI 实装路径**（RenderConfig / SpriteRenderer / EquippedCosmeticView），针对的是 SwiftUI 端可上线的 EquippedCosmeticView 占位；ui_design prototype 的 cat-placeholder.jsx 是设计参考资源，**不上线**、不在 Story 30.x 替换范围内。

### 根因（Root cause）

- r2 修复时把「ui_design prototype 的 placeholder 替换路径」与「SwiftUI 实装侧的 placeholder 替换路径」混为一谈 —— prototype（cat-placeholder.jsx vector shape）和实装（EquippedCosmeticView SwiftUI view）是**两套独立的占位**，分别在不同 surface 演进
- 「Story 30.x 替换 SpriteRenderer 图像渲染」这句默认 surface 是 SwiftUI 实装侧，但写白名单时下意识把 prototype jsx 也写成「替换对象」 —— prototype 不上线，本来就不需要被 Story 30.x 替换
- 三分边界（SwiftUI 实装侧 / 仓库 grid / ui_design prototype）的 prototype 一极在 r2 修复时遗漏，写成了「猫身上 overlay 替换 / 仓库 grid 保留」二分

### 修复（Fix）

把条目 9 理由段错误结构：

> 节点 10 起 Story 30.1-30.4 ... 仅替换猫身上的装扮为图像渲染（即替换 cat-placeholder.jsx 的 vector shape overlay）；... 二分边界：「猫身上装扮 overlay」节点 10 起 Story 30.x 替换为 SpriteRenderer 图像渲染；「仓库 grid emoji」永不替换。

改为三分边界 + 显式声明 prototype 不上线：

> 替换的是 **SwiftUI 实装侧**的 EquippedCosmeticView（当前可能用 SF Symbol / Text emoji 占位）走 SpriteRenderer 图像渲染；**ui_design prototype 的 cat-placeholder.jsx 是设计参考资源、不上线、不在替换范围内**；... **三分边界**：「猫身上装扮 overlay（SwiftUI 实装侧）」节点 10 起 Story 30.x 替换为 SpriteRenderer 图像渲染；「仓库 grid emoji」永不替换；「ui_design prototype（cat-placeholder.jsx vector overlay）」不上线，不在替换范围内。

「何时做」段同步写明三分边界。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **白名单 / scope 文档说「Story X.Y 替换 placeholder Z」时**，**必须显式标注 Z 所在 surface**（ui_design prototype / SwiftUI 实装 / inventory grid / Web prototype）；**禁止笼统说「Story X.Y 替换 prototype 的 jsx artifact」**，因为 prototype artifact 不上线，无替换义务。
>
> **展开**：
> - 一个产品视觉占位通常存在 **多套并行 surface**：(a) ui_design prototype（jsx / SVG，仅设计参考、不上线）；(b) SwiftUI / iOS 实装侧（视觉壳 placeholder，将被节点 N 起的 Story 替换为真材实料）；(c) inventory grid（emoji / 文字，可能永远保留）
> - Story X.Y 的「替换对象」永远是 **可上线的实装侧 surface**（即 b），不是 prototype（a）；prototype 的演进路径是「设计 freeze 后归档 / 永不替换」而非「Story X.Y 替换」
> - 写白名单条目时，**理由 / 何时做段必须列全 surface 的命运**：实装侧（替换为 X）/ inventory grid（保留）/ prototype（不上线）三极都要写明，避免 future epic 启动时 dev 把 prototype 当替换 target
> - **反例**：在白名单条目里写「Story 30.1-30.4 替换 cat-placeholder.jsx 的 vector shape overlay」 —— 把 prototype jsx 当替换对象，让后续 dev 误以为要去改 ui_design 里的 jsx，浪费工时且找错 surface

---

## Meta: 本次 review 的宏观教训

r1 / r2 / r3 三轮 review 都集中在「白名单条目的 cite / routing 准确性」上，宏观教训是：**scope 类文档的「未来 routing」段是最容易脑补、最难自查的位置** —— 因为「未来要做什么」无法被现有代码 / 测试反向校验，只能通过 grep 被 cite 的 epic / story 真实 scope 来核对。**写这类「未来 routing」段必须养成「先 grep cite 目标、再写主张」的反射动作**，而不是反过来「先写主张，再补 cite」（后者会把脑补的 routing 与权威 source 缝合，制造表面合规、实际错指的内容）。
