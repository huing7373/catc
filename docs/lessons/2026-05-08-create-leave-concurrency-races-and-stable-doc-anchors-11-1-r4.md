---
date: 2026-05-08
source_review: codex review round 4 of story 11-1（/tmp/epic-loop-review-11-1-r4.md）
story: 11-1-接口契约最终化
commit: c2996ba
lesson_count: 3
---

# Review Lessons — 2026-05-08 — 房间 create / leave 并发 race 必须落到正确业务码 / 冻结契约引用必须用稳定锚不依赖 commit hash（11-1 r4）

## 背景

Epic 11 Story 11.1（接口契约最终化）r3 之后契约已完成 WS 断线语义边界；codex r4 复检时发现两条**残留的并发 race 漏洞** + 一条**文档审计漏洞**：

- POST /rooms create 的 `room_members.UNIQUE(user_id)` 兜底写错了归属（落到 1009 服务繁忙而非 6003 用户已在房间）
- POST /rooms/{roomId}/leave 缺事务内 `RowsAffected == 0` 兜底，导致两次并发 leave 都返 200 + 触发重复 `member.left` 广播 + 重复 close 4007
- §12 章节末尾"业务消息延后锚定"段引用 Story 11.1 完成 revision 时硬填了 `<TBD-本 story 完成时回填>` placeholder，让冻结契约无法 audit 到具体 revision

本次 r4 把这三条全部修了。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | POST /rooms create 撞 UNIQUE → 6003 而非 1009 | high | architecture | fix | `docs/宠物互动App_V1接口设计.md` §10.1 服务端逻辑 / 错误码表 |
| 2 | POST /rooms/{roomId}/leave 必须在事务内 `RowsAffected == 0` 兜底 | high | architecture | fix | `docs/宠物互动App_V1接口设计.md` §10.5 服务端逻辑 / 错误码表 |
| 3 | 冻结契约引用从硬 commit hash placeholder 改为稳定锚 | low | docs | fix | `docs/宠物互动App_V1接口设计.md` §12.3 章末"业务消息延后锚定"段 |

## Lesson 1: POST /rooms create 撞 UNIQUE 必须归到 6003，**不**降级到 1009

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §10.1（POST /rooms 服务端逻辑步骤 3 + 错误码表）

### 症状（Symptom）

同一用户并发两次 `POST /rooms`：两个请求都通过步骤 1（`users.current_room_id == NULL` 预检），都开事务、都尝试 `INSERT room_members`。赢家先成功，输家撞 `room_members.UNIQUE(user_id)` —— 旧契约把所有事务回滚都归到 1009 服务繁忙，但这条路径是**正常并发**，client 应该收到 6003 才能复用既有"用户已在房间"恢复路径。

### 根因（Root cause）

§10.4 join 接口 r1 修复阶段已经识别了同样的 race 模式（步骤 5 撞 UNIQUE → 6003 兜底，与步骤 1 预检语义对齐），但 §10.1 create 接口的服务端逻辑只列了"事务任一步失败 → 1009"的粗粒度规则，没把 `room_members.UNIQUE(user_id)` 撞冲突这条**特定 race 路径**单独拎出来归类。"事务回滚 = 1009"的默认归类掩盖了"用户已在某房间"这个正常业务语义，让 client 错误地把 race 当成服务异常。

### 修复（Fix）

§10.1 服务端逻辑步骤 3 增加并发 race 兜底说明，同时改写"事务边界规则"和错误码表：

```
步骤 3: 插入 room_members ... ；如果遇到 DB UNIQUE(user_id) 兜底冲突（理论不会，
        因为步骤 1 已查过 users.current_room_id，但同一用户并发两次 POST /rooms 时
        可能发生 —— 两请求都通过步骤 1，赢家先插 room_members，输家在步骤 3 撞 UNIQUE）
        → 回滚 + 返回 6003（与 §10.4 join 接口步骤 5 兜底语义对齐）
```

错误码表 6003 触发条件从"步骤 1 预检"扩展为"步骤 1（预检）或步骤 3（DB UNIQUE 兜底）"；1009 触发条件明确剔除 UNIQUE 撞冲突路径。"关键约束"段对齐 §10.4 双路径 6003 设计。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写"事务步骤里有 INSERT 撞 UNIQUE 可能"的接口契约**时，**必须**把 UNIQUE 冲突按它**真实承载的业务语义归类**（如"用户已在某资源" → 6003 而非"事务异常" → 1009）。
>
> **展开**：
> - "事务任一步失败 → 1009 服务繁忙"是**默认归类**，不是兜底归类。任何带具体业务语义的失败路径（UNIQUE 冲突 / 行级状态校验 / 容量超限）必须**单独显式归类**到对应业务码，1009 仅留给"DB 异常 / 网络中断 / 内部 panic"等真正不可分类的故障
> - 跨接口对称性优先：同一资源 family 的接口（如 §10.1 create / §10.4 join / §10.5 leave 都操作 `room_members`）相同的 race 模式（撞 UNIQUE）必须归到同一业务码。Claude 写新接口时应先扫同 family 已有接口的并发兜底路径，对齐而不是新发明
> - **反例 1**：契约写"步骤 N 失败 → 1009"，没识别 UNIQUE 冲突是正常并发 → client 收到 1009 后做"重试"或"提示服务繁忙"，永远跑不到"用户已在房间"的恢复路径
> - **反例 2**：写一份新契约时不扫同 family 既有接口（如 §10.4 已有的 6003 兜底），独立发明一套错误归类，导致 create / join / leave 三接口对相同 race 给不同业务码

---

## Lesson 2: POST /rooms/{roomId}/leave 必须事务内 `RowsAffected == 0` 兜底防并发重复 leave

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §10.5（POST /rooms/{roomId}/leave 服务端逻辑步骤 2 + 错误码表）

### 症状（Symptom）

同一用户并发两次 `POST /rooms/{roomId}/leave`：两请求都通过步骤 1（`users.current_room_id == roomId` 预检），都开事务、都执行 `DELETE room_members WHERE room_id = ? AND user_id = ?`。赢家先 commit、删 1 行；输家的 DELETE 影响 0 行，但旧契约**没有 `RowsAffected` 检查** —— 输家继续走步骤 3（NULL → NULL idempotent）→ 4（房间剩余数已被赢家算过）→ 5（commit 空操作事务）→ 6（**重复广播 `member.left`** —— 房间内其他成员收到第二次离开事件）→ 7（试图关已不属于该房间的 leaver Session）。HTTP 200 + `data.left = true` 也是错的：契约钦定"重复 leave 应返回 6004"。

### 根因（Root cause）

step1 预检（`SELECT users.current_room_id`）和 step2 写操作（`DELETE room_members`）之间存在**经典 read-then-write race window**。仅用预检无法防御 —— 需要在事务内**用写操作的实际效果**（`RowsAffected`）做二次校验。旧契约设计时把"重复 leave 检测"全押在 step1 预检上，没意识到 step1 之后到 step2 之间的窗口在并发下可能让两个请求都通过。

### 修复（Fix）

§10.5 服务端逻辑步骤 2 增加事务内 `RowsAffected == 0` 兜底：

```
步骤 2: 开事务；DELETE room_members WHERE room_id = ? AND user_id = ?；
        检查 RowsAffected：若 == 0（同一用户两次并发 leave 都通过步骤 1，赢家已先删该行，
        输家此处 0 行受影响）→ 回滚 + 返回 6004（与步骤 1 同语义统一）；
        该兜底是关键并发护栏，否则输家会继续走完步骤 3 ~ 7 产生重复广播 + 重复 close 4007
```

事务边界规则补一条"**必须**在事务内回滚，不允许在事务外做 SELECT 校验后再开事务"；错误码表 6004 触发条件从"步骤 1 预检"扩展到"步骤 1 预检 + 步骤 2 事务内兜底"；明确说**不**用 `SELECT ... FOR UPDATE` 替代（虽然两者都能消除 race，但 `RowsAffected == 0` 更轻量）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写"step1 预检某状态 → step2 写操作改该状态"的事务契约**时，**必须**在 step2 写操作里检查 `RowsAffected`（DELETE / UPDATE 0 行命中 = race 输家），把 race 输家归到与 step1 预检失败**相同**的业务码，**不**让其继续走完后续副作用步骤。
>
> **展开**：
> - "read in step1 → write in step2"在事务级隔离下仍有 race window —— 哪怕 SQL 用了 REPEATABLE READ，step1 的 SELECT 拿到的是事务开始前的快照，无法防御另一并发事务在 step1 SELECT 之后、step2 DELETE/UPDATE 之前已 commit 的情况
> - **解决方案 A（推荐）**：step2 写操作后检查 `RowsAffected`，0 行命中即回滚 + 返 race 输家对应业务码。代价最低（单条 DML 自带的元数据）
> - **解决方案 B**：step1 改用 `SELECT ... FOR UPDATE` 锁住目标行。代价更高（持有行锁直到事务结束 + 引入死锁可能性），仅在 step1 预检需要做"复杂判定"（如容量校验 + 状态校验）时优于 A
> - **解决方案 C（错误）**：在事务**外**先 SELECT 校验然后开事务 —— 完全不能消除 race，只是把窗口换了个位置
> - 把 race 输家归到"和 step1 预检失败相同的业务码"，让 client UX 一致（无论是预检失败还是 race 失败，都是"我没在该房间 / 我已不在"）；**不**单独发明新错误码区分两条路径
> - **反例**：DELETE / UPDATE 后只看是否抛 error，不看 `RowsAffected` —— 0 行命中**不抛 error**（SQL 层面是合法操作），后续步骤继续跑产生重复副作用
> - **反例**：靠 step1 SELECT 防御并发，认为"两次 SELECT 都返 'not in room' 就两次都返 6004 了" —— 实际上两次 SELECT 都会返 'in room'，因为读快照都早于赢家 commit

---

## Lesson 3: 冻结契约的跨章节引用**禁止**用"待回填 commit hash"placeholder，改用稳定锚（git log grep / sprint-status / 相对引用）

- **Severity**: low
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §12.3 章节末尾"业务消息延后锚定"段

### 症状（Symptom）

§12.3 章末"业务消息延后锚定"段引用 Story 11.1 完成 revision 时写的是：

```
... commit hash `<TBD-本 story 完成时回填>`...
```

codex r4 评：冻结契约引用却 ship `<TBD>` placeholder，让契约无法 audit 到具体 revision。但现实中 epic-loop 框架还会跑 r5 / r6 / ... / r10，每轮 review 都产生新 fix commit，硬填的 hash 在下一轮 review 时就过时了。

### 根因（Root cause）

epic-loop 框架钦定 lesson md frontmatter 的 `commit:` 字段统一推迟到 story_done 阶段回填 —— 但那只针对 lesson 文件**本身**。文档**正文里的引用占位符**不在该 backfill 范畴内，且 V1 接口设计.md 是**冻结契约**，多轮 review 之间一直在改，硬绑某个 hash 必然过时。Claude 在 r0 起草这段时，没区分"lesson md frontmatter（story_done 统一回填）"和"文档正文引用（必须用稳定锚）"两种 placeholder 的处理策略。

### 修复（Fix）

把硬 hash 引用改成不依赖具体 hash 的稳定锚：

```
Before:
  Story 11.1（Epic 11 房间业务契约，节点 4 中段，commit hash `<TBD-本 story 完成时回填>`）
After:
  Story 11.1（Epic 11 房间业务契约，节点 4 中段；锚定 revision 见
  `git log --grep='story-11-1'` 检出的 Story 11.1 收官 commit
  `chore(story-11-1): ...`，以及
  `_bmad-output/implementation-artifacts/sprint-status.yaml` 中
  `11-1-接口契约最终化` 状态行）
```

稳定锚的 audit 路径：(a) `git log --grep='story-11-1'` 找 story-11-1 系列所有 commit、(b) sprint-status.yaml 状态行确认 done revision、(c) 收官 commit message 模板 `chore(story-11-1): ...`。三个锚都不依赖 commit hash，多轮 review 不会过时。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **冻结契约文档（V1接口设计 / 数据库设计 / 总体架构 等长期权威文档）正文里引用其他 story 完成 revision** 时，**禁止**用"待回填 commit hash"placeholder，**必须**用稳定锚（`git log --grep='<story-key>'` / `sprint-status.yaml` 状态行 / commit message 模板）。
>
> **展开**：
> - lesson md frontmatter 的 `commit:` 字段是**单 commit 文件**的元数据，由 `/story-done` 阶段统一回填一次 —— 与正文引用是两个不同概念，不能混用同一处理策略
> - 冻结契约文档可能跨多轮 review 演化（r1 / r2 / ... / r10），每轮都产生新 commit；硬填的 hash 在下一轮就过时，要么持续维护（高成本）要么留过期信息（误导）
> - 稳定锚的好处：(a) 一次写定，多轮 review 不维护；(b) 借助 git log grep / sprint-status / commit message 模板的稳定性，audit 路径明确；(c) 跨 epic 文档统一形式
> - 跨 story 引用模板：`见 git log --grep='<story-key>' 检出的 <story-key> 收官 commit chore(<story-key>): ...，以及 sprint-status.yaml 中 <story-id> 状态行`
> - **反例 1**：契约引用写 `commit hash <TBD-story 完成回填>` 期望 story_done 时统一处理 —— story_done 不会回填正文 placeholder，框架只回填 lesson frontmatter
> - **反例 2**：契约引用写硬 hash `commit abc1234` —— 该 hash 可能在下一轮 fix-review 后变成 history 中间节点（被 amend / rebase 覆盖），引用永久指向已被替换的旧版本
> - **反例 3**：契约引用写"见 commit message"但不指明 story-key 怎么找 —— 读者要扫遍 git log 才能定位

---

## Meta: 本次 review 的宏观教训

r4 的两条 P1 都是同一类思维漏洞的**变体**：**read-then-write race**。create 接口的"step1 SELECT current_room_id is null → step2 INSERT room_members"和 leave 接口的"step1 SELECT current_room_id == roomId → step2 DELETE room_members"是**结构对称的两个 race window**。Story 11.1 早期（r0）写 §10.4 join 时已经识别并兜底了同模式（步骤 5 INSERT 撞 UNIQUE → 6003），但写 §10.1 create / §10.5 leave 时**没复用同一兜底模式** —— 这是 BMAD 跨 story 一致性失败。

宏观规则：

> **未来 Claude 在写一组结构对称的接口契约（同 family / 同 resource / 同事务模式）时，必须先扫同 family 既有接口的并发兜底路径，把对应模式套用到新接口；**不**为每个接口独立设计并发兜底。**

具体到本场景：room family 的三个写接口（create / join / leave）都涉及 `room_members` 表的写操作，都有 step1-SELECT → step2-write race window，都应该用相同的"事务内 RowsAffected / UNIQUE 撞冲突 → 业务码"兜底模式。下次再加 room family 接口（如未来"踢人"），新接口契约写完后必须**显式列出**与 create / join / leave 三接口的并发模式对照表，证明兜底模式已套用。
