---
date: 2026-05-06
source_review: codex review (epic-loop r6, /tmp/epic-loop-review-10-1-r6.md)
story: 10-1-接口契约最终化
commit: 11a1429
lesson_count: 3
---

# Review Lessons — 2026-05-06 — Redis presence 不能替代 membership 授权 + room.snapshot 单一视图原则 + close code 表 1006 数字冲突清理

## 背景

Story 10-1（V1 协议契约节点 4 冻结）codex review 第 6 轮。前 5 轮已经稳定了"error 双重语义"、"4005 心跳超时 close code"、"信封字段冻结"、"4xxx vs §3 数字空间隔离（1008/1009 改 4006）"等结构。r5 修过的两段优化注解被 r6 codex 抓出新问题：

1. r5 给握手第 5 步 DB 查询加的"应缓存到 Redis presence"性能优化注解 —— **破坏了授权语义**：presence 只是 ephemeral 在线态，不代表 membership。两个常见场景下用 presence 做 membership 校验会错误返回 4003（server 重启后第一个用户重连 / 合法成员 presence TTL 过期后重连）
2. `room.snapshot` 字段表三规则不可同时成立：`memberCount` 定义为"当前在线成员数"、`payload.members` 定义为"`room_members` JOIN 全成员"、不变量"`memberCount == members[].length`"。一旦房间有离线成员，三者必有一者撕裂
3. r5 加进 close code 表的 `1006` 行，与 §3 全局错误码 `1006`（状态不允许当前操作）数字撞车 —— 跟 r2 修过的 1008/1009 同类型 collision，被 r6 codex 抓回来

r6 是 10 轮上限的第 6 轮，还剩 4 轮，但本轮要把"协议契约文档冻结期的内部一致性"扫到位，避免再补丁。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | "DB 查询应缓存到 Redis presence" 注解破坏授权语义 — presence 是 ephemeral 在线态，不代表 room membership | high (P1) | architecture / docs | fix | `docs/宠物互动App_V1接口设计.md:1361` |
| 2 | `room.snapshot` 字段表自相矛盾：memberCount=在线 vs members=全成员 vs 不变量 memberCount==members.length 三规则不可同时成立 | high (P1) | architecture / docs | fix | `docs/宠物互动App_V1接口设计.md:1482-1483` `:1532` |
| 3 | close code 表的 `1006` 行与 §3 全局错误码 `1006`（状态不允许）数字空间冲突 — 跟 r2 修过的 1008/1009 同类型 | medium (P2) | architecture / docs | fix | `docs/宠物互动App_V1接口设计.md:1328` `:1336-1339` |

修了 3 条 / defer 0 条 / wontfix 0 条。

## Lesson 1: 协议层授权步骤**禁止**复用 ephemeral 缓存语义

- **Severity**: high (P1)
- **Category**: architecture / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1361`

### 症状（Symptom）

WS 握手序列第 5 步"用户房间归属校验"原本规定查 `room_members` 表（持久化数据）。r5 给它加了一段注解："DB 查询是热点路径，Story 10.3 / 10.6 实装时**应**将 `room_members` 在线态缓存到 Redis presence（避免每次连接都查 MySQL）"。该注解在协议契约层提示实装方用 Redis presence 替代 MySQL 查询。

但 Redis presence 在本设计里语义是 **ephemeral online state**（user 当前是否有活跃 WS 连接，TTL 短，连接断开就清），**不代表** user 是该 roomId 的合法成员。两个 hard-fail 场景：

- **(a) 冷启场景**：server 进程重启 / Redis 清空 / Redis 故障重启 → 所有 presence entry 都没了。第一个合法成员重连时，presence-backed check 会返回"非成员" → close 4003，UX 提示"未加入该房间"。但该用户**确实**在 `room_members` 表里。
- **(b) TTL 过期场景**：合法成员长时间断线，presence entry TTL 到期被清。该用户重连时，presence 为空 → close 4003。同样违反 contract。

任何 client / 监控 / 日志会看到大量"应该成功的合法重连被 close 4003"，难以排查（presence 是黑盒）。

### 根因（Root cause）

把"性能优化建议"放在协议层的危险：

- 协议层规定的是**语义契约**（这一步要拿到什么数据来回答什么问题）；优化层规定的是**实装**（怎么拿这数据更快）
- 当协议注解写"实装时**应**用 X 替代"，且 X 与协议要求的数据语义本质不同时，协议契约本身被改变 —— 即使原 step 描述还在，下游实装 reviewer 看到注解会按注解走
- 本案核心错误：把"在线 user set"（presence 的语义）等同于"member set"（room_members 的语义）。这两个集合**有交集但不相等**：member 集合是非空稳定集，online 集合是任意子集（含空）

更深层的：缓存方案设计的常见陷阱是"复用同一个 Redis key 满足多个语义"。这里 presence 是 transient（连接驱动 TTL），但 membership 是 durable（事务驱动失效）。两者必须分开存储，否则 lifecycle 撞车。

### 修复（Fix）

替换该段优化注解 —— 不再建议任何优化，明确**禁止**复用 presence 做 membership 校验，并把"未来如要为热路径降低 MySQL 读压"的方案约束为"必须引入 durable membership cache（与 presence 区分）"：

```markdown
**注意**：第 5 步是**协议层面强制的授权环节**，必须以**持久化 membership 数据**
（即 `room_members` 表）为 single source of truth —— **禁止**使用 Redis presence
替代该校验。理由：presence 仅表示 ephemeral 在线态，不代表 user 是房间合法成员；
以下两种常见场景下，"用 presence 做 membership 校验"会错误返回"非成员"并 close 4003：
(a) server 重启 / Redis 清空后第一个合法成员重连（presence 全空）；
(b) 合法成员的 presence entry TTL 过期后重连。
如未来需要为该热路径降低 MySQL 读压（Story 10.6 / 11.4 实装阶段评估），
**必须**引入与 presence 区分的"durable membership cache"
（持久层语义、由加入 / 退出房间事务原子失效），而**不是**复用 presence；
本协议层面的 §12.1 校验顺序不因任何缓存方案而改变。
```

### 预防规则（Rule for future Claude） ⚡

> **一句话**：未来 Claude 在 **协议契约层文档**里写"实装层**应**用某缓存替代 DB 查询"类注解时，**必须**先核对该缓存的 lifecycle / TTL / 语义是否与被替代的 DB 查询的语义**完全等价**；只要有一个反例 lifecycle 让缓存返回与 DB 不同结果，就**禁止**写该建议（要么删除注解让 DB 作 single source of truth，要么明确写"必须引入语义等价的 durable cache，不是复用现有的 ephemeral cache"）。
>
> **展开**：
> - 协议层的工作是规定"语义契约"，不是规定"实装优化方案"；契约层应只描述"这一步要拿到什么 data 回答什么问题"，不描述"怎么拿"
> - 缓存替代 DB 查询的等价性必须三选一明确：① 完全等价（任何场景缓存与 DB 返回相同）→ 可建议；② 部分等价（缓存只是 hint，miss 后必须 fallback DB）→ 必须写 fallback，不能写"应替代"；③ 不等价（缓存的 lifecycle 与 DB 不同）→ **禁止**用作替代，要么 DB-only，要么独立设计 cache
> - 常见陷阱：在线态 presence ≠ membership；session cache ≠ user identity；rate-limit counter ≠ 业务计数。任何 ephemeral / TTL-driven 数据**禁止**作为持久化授权 / membership / identity 的 single source
> - **反例**：写"X 表查询是热点路径，实装时应缓存到 Y（其中 Y 是 ephemeral cache）以提升性能"，且没说明 Y miss 时 fallback X 的语义。该写法在协议契约文档里**永远**是错的，不论 X / Y 是什么

## Lesson 2: 同一字段表的多个字段必须共享同一"视图模型"，不变量上方就要锁定

- **Severity**: high (P1)
- **Category**: architecture / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1482-1483` `:1532`

### 症状（Symptom）

`room.snapshot` 的 `payload` 三规则不可同时成立：

1. `payload.room.memberCount` 定义为"当前在线成员数"（含 Redis presence 在线态过滤）
2. `payload.members` 定义为"`room_members` JOIN `users` JOIN `pets` 聚合"（**全成员**，不过滤在线态）
3. 字段表后注（line 1531）写"`memberCount` 必须等于 `members[]` 数组长度"

只要房间有任一离线成员（合法成员但 presence 已断），(1) 给出 N（在线数），(2) 给出 M（全成员数），N < M，与 (3) 直接矛盾。

下游 Story 11.7 实装 reviewer 会困惑：到底过滤还是不过滤？两种实装都能挑出文档里相反规则做依据。

### 根因（Root cause）

字段表是**逐字段独立写**的，但表内字段**不独立** —— 它们共同构成一个数据视图（snapshot view）。视图必须先选定一个 mental model（"全 roster" or "online-only"），再让所有字段在该 model 下定义。

r5 修这块时，对 `memberCount` 加了"在线"语义（写"按 `room_members` 行数 + Redis presence 在线态计算"），但没扫到 `members` 字段还是全成员定义；下方不变量也没改。这是**字段间内部一致性 sweep 缺失**的典型 bug。

更深层的：字段表 row 由不同时刻 / 不同 review 轮次叠加修订时，每次修订只看 row 自己，不看 row 之间的隐式约束（如 `count` 字段必然约束于 `array` 字段的 length）。

### 修复（Fix）

锁定"full roster view"（含离线），三处对齐：

1. `payload.room.memberCount` 改"房间总成员数（**含离线**）；与 `payload.members` 数组长度严格相等"
2. `payload.members` 改"房间全成员列表（含离线，不区分在线 / 离线）"
3. 不变量注从 1 行扩充为 paragraph，明示"snapshot 是 full roster view，不是 online-only view"，列出三条理由：(a) 任一字段过滤都违反不变量；(b) 节点 4 阶段不广播 `member.joined`，client 无法靠后续推送补齐离线成员，snapshot 必须自包含；(c) 在线 / 离线状态由后续 epic 的独立 presence 推送通道维护

### 预防规则（Rule for future Claude） ⚡

> **一句话**：未来 Claude 在 **改字段表里某个字段定义** 时，必须先识别该字段是否与表内其他字段共享一个 view model（如 `count + array` / `total + items` / `online + roster`），如果是，**禁止**只改单字段语义；必须**同时**修订所有同 view 字段 + 不变量注 + 示例 placeholder。
>
> **展开**：
> - 字段表的"row 独立感"是错觉。`count` 字段约束 `array` 字段的 length；`max + current` 约束于 `items` 数量上限；`from + to` 约束于 range 一致性 —— 这类隐式约束必须在表前 / 表后用不变量注**显式**化
> - 视图模型必须先选定再写字段：full roster vs online-only / total vs window / latest snapshot vs cumulative。所有字段在同一 model 下解释，字段间"是否过滤"必须**统一动作**
> - 字段表附带的 example JSON 也是 view model 的一部分；改字段语义必须 sweep example 是否还自洽
> - **反例**：字段 A "在线 X"、字段 B "全 X"、注"A == B.length"。三规则任一改动都要 sweep 其他两条；只改一条就是 P1 contract 违规

## Lesson 3: 协议文档"应用错误码段位"vs"transport close code 段位"的数字空间隔离规则适用于**整个 1xxx 段**，不仅限 1008/1009

- **Severity**: medium (P2)
- **Category**: architecture / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1328` `:1336-1339`

### 症状（Symptom）

r5 把"消息超大"close code 从 1008 改 4006，并在关键约束里加了"不使用 RFC 1008 / 1009"段（理由：§3 已用 1008/1009 作应用错误码）。但 r5 同时给 close code 表加了 `1006` 行（注解 1006 是 client-only reserved code，server 不 emit）。

r6 codex 指出：§3 全局错误码表里 `1006 = 状态不允许当前操作`，跟 close code 表里出现的 `1006` 仍然撞数字空间。即使 1006 注明"server 不 emit"，下游 client / log / 监控仍会困惑：日志看到一个 `1006` 数字，到底是 application error 还是 transport close（即使概念不同，数字相同就要查表 + 看上下文 disambiguate）。

跟 r2 修过的 1008/1009 是**同类型** collision —— r5 漏掉了"§3 占用的 1xxx 段值不止 1008/1009"。

### 根因（Root cause）

r5 修 1008/1009 collision 时只 enumerate 了 1008/1009，没回头扫 §3 全局错误码表里**所有** 1xxx 段被占用的 code（应该至少检查 1000-1099 这段是否还有其他与 close code 表撞车的）。修 collision 时的"列举式 patch"特征 —— 只修当前 review 抓到的具体值，不归纳出整段规则。

另一个根因：1006 是 RFC 6455 reserved code（client-only synthetic），看似"不会撞"因为 server 不 emit；但日志 / 监控视角下，1006 仍会出现在某些 metric tag 或 client telemetry 里，跟 §3 应用错误码 1006 共享 namespace。

### 修复（Fix）

1. **从 close code 表删除 1006 行**（1006 永远不在 server-emitted code 集合，不该占用表的一行）
2. **扩展"不使用 RFC close code"段**为 `1006 / 1008 / 1009` 三个 code 的统一处理：
   - 1006 子项：保留"reserved code, client-only synthetic, server 禁止 emit"语义说明；新增"§3 已用 1006 作应用错误码"的 collision 理由
   - 1008/1009 子项：保留 r5 的版本
   - 共同根因小段：明示"§3 已占用的 1xxx 段值（含 1006/1008/1009）一律禁止作 close code"
3. **明示 server 主动 emit 的 close code 集合**：1000 / 1001 / 1011 + 4001-4006 共 9 个值（让 client / log / 监控有一个 closed enum 可对照）

### 预防规则（Rule for future Claude） ⚡

> **一句话**：未来 Claude 在 **修协议文档的 collision 类 review finding** 时，**禁止**只修被点名的具体值；必须先归纳出 collision 的**结构性规则**（如"§3 已占用的整个 1xxx 段都不能作 close code"），然后 enumerate 整段 / 整个 namespace 是否还有其他同类 collision，全部一次性修。
>
> **展开**：
> - "枚举式 patch" 是 collision 类 review 的 anti-pattern：reviewer 抓 1008/1009 → 只修 1008/1009 → 下一轮 reviewer 抓 1006 / 1010 → 又是补丁；本质是"没归纳出规则就修了一个 instance"
> - 修这类 finding 必须 sweep 整个 namespace：§3 错误码表里**所有** 1xxx 段值都列出来，跟 close code 表交集查 collision；交集为空才算修干净
> - close code 表的"行"应该只放 server actively emits 的 code；client-only synthetic（如 RFC reserved code 1005/1006/1015）禁止占行，最多在关键约束 prose 里描述
> - **反例**：close code 表里同时存在"server emit 的 4006"行 + "client-only 的 1006"行 —— 后者读者会以为 server 也可能 emit；混淆 namespace；下一轮 review 必抓

---

## Meta: 本次 review 的宏观教训

r6 修了 3 条都指向同一个高阶原则：**协议契约文档里的"性能优化建议 / client-only 注解 / 单字段语义改动"全是 trap zone**。三类 trap：

1. **优化建议 trap**（Lesson 1）：把"实装层应用 X 替代 DB 查询"写在契约层，X 与 DB 语义不等价 → 协议契约被悄悄改变
2. **单字段改动 trap**（Lesson 2）：字段表逐字段维护，忽略字段间隐式约束 → 视图模型撕裂
3. **client-only 注解 trap**（Lesson 3）：把 client-only 概念塞进 server-emit 表 → namespace 混淆

通用预防规则：**协议契约层修订必须做"sweep 三问"**：
- (a) 这次改动的字段 / step 是否与其他字段 / step 共享同一 view model 或 namespace？如有，sweep 是否对齐
- (b) 这次新增的注解 / 优化建议是否引入了新的语义假设？如有，sweep 假设是否在所有边界条件下成立
- (c) 这次新增 / 删除的表行是否与 closed enum 的"完整 9 个值"严丝合缝？如不，要么补全要么压缩

**fix-review 里"sweep 自检"的高优先动作清单**：
1. close code 表的行集合 vs server-emit closed enum 是否完全相等
2. §3 错误码段值 vs close code 表数字 vs other namespace 是否两两交集为空
3. 字段表里 `count + array` / `total + items` 类字段是否共享同一过滤规则
4. 协议层注解里"实装应 X 替代"句式 —— X 与原数据是否语义等价
