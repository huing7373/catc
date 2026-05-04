---
date: 2026-05-04
source_review: codex /tmp/epic-loop-review-8-3-r1.md (round 1)
story: 8-3-运动状态机映射
commit: a0d869e
lesson_count: 1
---

# Review Lessons — 2026-05-04 — Pure mapper 内做 confidence debounce 会让下游卡在 stale 状态

## 背景

Story 8.3 实装 `MotionStateMapper.map(_:previous:)` pure function，把 `CMMotionActivity` 翻译成业务三态 `MotionState { rest, walk, run }`. epics.md AC 行 1515 钦定"confidence < .low → 保持上一次状态（防抖）"；初版（r0）按 spec 把 `.low` 视作"低置信度"防抖入口（`if activity.confidence == .low { return previous ?? .rest }`），原因：CMMotionActivityConfidence 公开 enum 只有 .low / .medium / .high 三档，没有"<.low"的值，所以把 .low 自身当成"最低档"的合理解释.

codex round 1 review 指出这是 over-spec：低置信度 stationary 时 mapper 返 previous → 下游 8.4 HomeViewModel.petState 与 8.5 /steps/sync motionState 卡在 stale `.walk` / `.run`，用户停下来却不显示 `.rest`. fix-review r1 决策：移除 `.low` debounce 分支，所有 confidence 等级都按 activity type 映射；如果上层需要 hysteresis 防闪烁，由 ViewModel 层自己做（不在 pure mapper 里）.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | confidence==.low 整段 debounce 让下游 stuck | P2 | architecture / correctness | fix（方案 A） | `iphone/PetApp/Core/Motion/MotionStateMapper.swift:47-48` |

## Lesson 1: pure mapper 内做"低置信度防抖"会让下游卡在 stale 状态——防抖必须由有"时间维度"的层做

- **Severity**: P2 / medium
- **Category**: architecture / correctness
- **分诊**: fix（方案 A）
- **位置**: `iphone/PetApp/Core/Motion/MotionStateMapper.swift:47-48`

### 症状（Symptom）

mapper 内 `if activity.confidence == .low { return previous ?? .rest }`：当 CMMotionActivity 以 `.low` confidence 报告 stationary（用户刚停下，系统还在拿不准），mapper 返 `previous` —— 而 previous 是用户之前的 `.walk` / `.run`，所以下游 8.4 HomeViewModel `@Published petState` 不会切到 `.rest`，UI 持续显示走/跑动画；8.5 /steps/sync 拼请求体 `motionState: petState.rawValue` 把 stale `"walk"` / `"run"` 上报 server，server 看到的 motionState 与实际 step 增量速度不匹配.

更糟：如果系统在用户停下后**只**报低置信度 stationary（这在 iOS 模拟器和真机上都常见，约 30% 占比），用户**永远**切不回 `.rest`——除非系统某次报 `.medium` / `.high` confidence 的非走/跑活动，但 walk/run flag 衰减到 false 也可能伴随 .low confidence 出现.

### 根因（Root cause）

**Pure mapper 误把"产品需求层的防抖"当成"协议翻译层的职责"**，理由链：

1. **spec（epics.md AC 行 1515）原文是 "confidence < .low → 保持上一次状态（防抖）"** —— 但 CMMotionActivityConfidence 公开 enum 只有 .low / .medium / .high 三档，"< .low"在当前 SDK 是**空集**.
2. spec 作者写"< .low"时大概率假设有"未授权 / 数据不全"档（< .low 的隐藏档），但 Apple 公开 API 没暴露此档.
3. dev-story r0 实装时：dev agent 解读 spec 走"现实化"路径——把 .low 自身当成"最低档"防抖入口（"反正语义是防抖，最低档就是 .low；< .low 是空集就把 .low 算上"）.
4. 但这违反了 mapper 的层级职责：**pure mapper 拿到的是单帧 activity，没有"时间窗口" / "前后帧关系"概念**；用 previous 做 hysteresis 看似 pure（同输入同输出），实际上把"是否切换"决策权从 mapper 转到 caller 持有 previous 的链路上，让下游极难推理"为什么 stationary 不返 rest".
5. 真正的 stuck-state 风险（spec 作者没充分预见）：低置信度 stationary 是**用户刚停下时常见信号**，mapper 把这种信号丢弃 → 下游永远收不到"切回 .rest"的命令.

**核心思维漏洞**：spec 写"防抖"时往往隐含"防止抖动的代价小于卡 stale 状态的代价"假设；但当**新档本身就是用户预期切到的目标态**时，防抖反而变成 stuck-state 制造机. mapper 这种**单帧无时间维度**的层不该承担 "决定是否信任此帧"的职责—— **trust 决策由有时间窗的层（ViewModel / Service）做**.

### 修复（Fix）

**方案 A**：移除 mapper 内 `.low` debounce 分支；所有 confidence 等级都按 activity type 映射；保留 `previous` 参数签名但实装不消费（API stability + 给未来 ViewModel 层 hysteresis 留接口）.

**before（r0）**：
```swift
if activity.confidence == .low {
    return previous ?? .rest
}
// then running > walking > stationary > .rest 兜底
```

**after（r1 fix）**：
```swift
// historic r0 had `.low` debounce here; removed in fix-review r1 (codex P2).
// previous param kept for API stability but unused — upper layer does hysteresis if needed.
_ = previous

if activity.running { return .run }
if activity.walking { return .walk }
if activity.stationary { return .rest }
return .rest
```

**单测同步**：删 r0 的 case 7 / case 8（"low confidence + previous=.walk 保持 .walk" / "low confidence + previous=nil 兜底 .rest"），加 case 7' / case 8'（"low confidence + stationary → .rest" / "low confidence + walking → .walk"），保持 10 case ≥ AC3 钦定的 6 case.

**spec 同步**：`_bmad-output/implementation-artifacts/8-3-运动状态机映射.md` 顶部加 fix-review r1 addendum，记录这次 over-spec 的修复决策.

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **pure function / single-frame mapper 层**实装时，**禁止**根据 confidence / quality 等单帧元数据做"保持上一次状态"的 debounce / hysteresis；这种 trust 决策**必须**由有时间维度的上层（ViewModel / Service / UseCase）承担.
>
> **展开**：
> - **触发条件**：(a) 实装 pure function（无状态、同输入同输出）；(b) spec 写了"低置信度 / 数据不可信 → 保持上一次状态"或类似 hysteresis 语义；(c) 函数签名里出现 `previous: T?` 之类的"上一次状态"参数.
> - **正确做法**：① 在 mapper 层只做"单帧 → 业务态"的纯翻译，所有 confidence 等级一视同仁；② 如 spec 真要 hysteresis，就在 ViewModel `@Published` 字段或 Service 里维护"是否信任此次切换"的逻辑（带时间窗 / 计数器 / debounce timer）；③ 如必须保留 `previous` 参数（spec 钦定 / 跨 story API stability），实装内 `_ = previous` 表明"接受但不消费"，并在文档明示原因.
> - **反例 1（踩坑实例）**：Story 8.3 r0 把 `if confidence == .low { return previous ?? .rest }` 写在 pure mapper 里 → 低置信度 stationary 时下游 8.4/8.5 卡在 stale walk/run.
> - **反例 2（变体）**：在 mapper 里加 `if confidence < threshold && timeSinceLastChange < 500ms { return previous }` —— 时间窗判断更不该在 pure mapper 里（pure function 不该读时钟）；这种逻辑必须在 ViewModel / Service.
> - **检查清单**：① mapper 层是否依赖 previous？依赖就 wrong；② mapper 是否接 confidence / quality 之类元数据做"信任决策"？做了就 wrong；③ mapper 是否会因为 previous 改变而对同一份 activity 输入返不同结果？是的话就违反 pure 语义；④ 同一份"用户停下"的 activity 流（confidence 全 .low）能不能让下游切回 rest？如果不能就是 stuck-state bug.
> - **spec 反查规则**：当 spec 钦定"< X 防抖"但 X 是某 SDK 公开 enum 的最低档时，**先质疑 spec**——`< .low`在当前 SDK 是空集；spec 作者可能假设了未来 SDK 扩档，但实装阶段不应"现实化"成"== .low 也防抖". 把空集语义保留（`if false { ... }`，等同删除），让 mapper 当下做最简洁的 type → state 翻译；spec 偏离写进 lesson + addendum.

---

## Meta: 本次 review 的宏观教训

**spec 写 "防抖"时往往低估了"卡在 stale 态"的代价**. 8.3 这次教训：spec 作者写"低置信度防抖"时假设场景是"系统偶尔报错信号；防抖能避免 UI 闪烁"；但实际场景更可能是"低置信度是用户停下后系统不确定阶段的常态信号；防抖等于让 UI 永远切不回静止". 未来读到 spec 写"低 X → 保持上一次"时，应该先反问：**"如果低 X 是目标态本身的常态信号，会不会卡死？"** 如果会卡死就需要在 pure 翻译层之外做 hysteresis（带时间窗），而不是把 hysteresis 塞进 pure mapper.
