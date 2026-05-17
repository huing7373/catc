---
date: 2026-05-17
source_review: "codex review --base output (file: /tmp/epic-loop-review-23-5-r2.md)"
story: 23-5-修改开箱事务-创建-user_cosmetic_items-实例-补-chest_open_logs-reward_user_cosmetic_item_id
commit: <pending>
lesson_count: 2
prev_lesson: 2026-05-17-random-limit-pool-shorter-than-count-silent-shortfall-23-5-r1.md
over_correction_chain: "23-5 dev grant pick 语义 —— 第 2 跳（r1 修复方向被本轮推翻）"
---

# Review Lessons — 2026-05-17 — dev grant 的 count 是实例数不是 distinct 配置数：over-correction chain 第 2 跳，根因是数量语义而非校验松紧（23-5 r2）

## 背景

Story 23.5 激活 `/dev/grant-cosmetic-batch` 真实写库。**这是一条 over-correction chain 的第 2 跳**，必须连着 r1 lesson（`2026-05-17-random-limit-pool-shorter-than-count-silent-shortfall-23-5-r1.md`）一起读：

- **dev-story 原始实装**：`FindRandomByRarity(rarity, count)` SQL `... ORDER BY RAND() LIMIT count`（返回 ≤ pool distinct ids）。
- **codex r1** flag「池 < count 时静默少发仍返 success」[P1]。**r1 修复方向**：在 service 加 `if len(ids) < count → 1009`，往「严格拒绝短发」方向走。
- **codex r2** finding #1 = **r1 修复的直接症状反弹**：r1 的拒绝逻辑把合法主 demo 用例（common seed 仅 8 件 distinct，但合成 demo 需 grant 10 个 common **实例**）打死返 1009，阻塞节点 11 合成。

codex r2 真实结论（review 文件末尾 `^codex$` 段）共 2 条 [P2]，均与 dev grant 路径相关，与 Epic 20 race-fix 开箱事务核心无关。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | count > rarity distinct enabled 池时 r1 的 `len<count→1009` 拒绝把合法主 demo 用例打死 | medium (P2) | architecture | fix | `server/internal/repo/mysql/cosmetic_item_repo.go` + `server/internal/service/dev_cosmetic_service.go` |
| 2 | dev batch grant 循环跑在 base DB handle 非事务，中途失败部分提交 + 返错 → 重试致部分授予/重复批次 | medium (P2) | error-handling | fix | `server/internal/service/dev_cosmetic_service.go` |

两条同根：**`count` 语义被搞错**。一次性根因解决。

## Lesson 1: 批量发放的「数量」是要产出的**实例数**，不是配置/distinct 池大小——pick 必须有放回，与池容量解耦

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/repo/mysql/cosmetic_item_repo.go`（pick 方法）+ `server/internal/service/dev_cosmetic_service.go`（GrantCosmeticBatch）

### 症状（Symptom）

`/dev/grant-cosmetic-batch?rarity=common&count=10` 在 seed common 仅 8 件 distinct 配置时被 r1 的 `len(ids) < count → 1009` 拒绝，无法产出 10 个 common 实例。而节点 11 合成 demo 的核心就是「喂 10 个同品质实例升级」——这是 story 钦定的主用例，不是边界。系统其它地方（开箱入仓）本就允许同 `cosmetic_item_id` 多实例。

### 根因（Root cause）

实装把 dev grant 的 `count` 误读成「要发放 count 个**不同配置**」，于是 SQL 用 `ORDER BY RAND() LIMIT count`（天然受 distinct 池上限约束），r1 又在此错误契约上加「不足即拒」。真正的语义（从 source-of-truth 推导）：

- `user_cosmetic_items`（数据库设计 §5.9）只有非唯一 `KEY idx_user_id_cosmetic_item_id`，**没有** `UNIQUE(user_id, cosmetic_item_id)`——同一 `cosmetic_item_id` 多实例**合法**。
- §22 合成 feature：「玩家手动选择 10 个同品质道具实例」——重复 `cosmetic_item_id` 实例不仅合法且是 feature 核心所**必需**。
- 故 `count` = 要授予的**实例（instance）数**，pick 必须从该 rarity enabled 池里**有放回（with repetition）**选 count 个。池大小与请求量是两个无关量纲，不该相互约束。

r1 走错的元原因：在「静默少发」这个症状上做**校验松紧的表层微调**（放行 ↔ 拒绝二选一），没有回退一层问「count 到底是什么量纲」。校验方向之争（松/紧）是 over-correction chain 的典型诱饵——真正的分歧在数量语义，不在阈值。

### 修复（Fix）

根因一次性解决（撤销 r1 的错误拒绝，不是再调阈值）：

1. **repo 层**：`FindRandomByRarity(ctx, rarity, count) ORDER BY RAND() LIMIT count` → `ListEnabledIDsByRarity(ctx, rarity)`：`SELECT id WHERE rarity=? AND is_enabled=1`（**无 LIMIT、无 ORDER BY RAND()**），只负责给出**池**（distinct enabled id 列表）。
2. **service 层**：取池后，若**池完全为空**（该 rarity 无任何 enabled 配置——这才是真正的 seed 数据完整性错误）→ 1009；否则在 Go 层 `for i:=0;i<count;i++ { ids[i]=pool[rand.Intn(len(pool))] }` **有放回**抽 count 个。**撤销** r1 的 `len < count → 1009`（保留「空池 → 1009」这一真正错误档）。
3. 守门测试 `PoolSmallerThanCount_GrantsExactlyCountWithRepetition`：池=8 + count=10 → 断言 ① 成功（非 1009）② CreateInTx 正好 10 次 ③ 抽出 id 全在池内且**必有重复**。任何人把「distinct 上限 / 不足即拒」hack 加回来，该测试立刻挂。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 实现「按某分类批量发放/产出 N 个实例」时，**必须**先确认 N 是**实例数**还是**distinct 配置数**（查目标表有无 `UNIQUE(owner, config_id)` + 查业务是否需要同配置多实例），**禁止**用 `LIMIT N` / 取 distinct 把发放量钉死在池容量上。

> **展开**：
> - 实例化库存表（每行一个唯一实例 id，无 `UNIQUE(owner, config_id)`）→ 发放量与 distinct 池容量解耦，pick 必须**有放回**（`pool[rand.Intn(len(pool))]` 循环 N 次），池小于 N 完全合法。
> - 唯一真正的错误档是**池为空**（该分类无任何可用配置），不是「池 < N」。错误码只为「空池」开，不为「池 < N」开。
> - 收到「静默少发 / 数量不对」类 review 时，**先回退一层问数量语义（哪个量纲），再决定校验**——不要在「放行 ↔ 拒绝」之间做阈值微调。校验松紧之争通常是数量语义错误的下游症状。
> - **反例**：`ids := repo.PickRandom(rarity, count) /* SQL LIMIT count */; if len(ids) < count { return err }; insert(ids)` —— 当业务允许同配置多实例时，`LIMIT count` 把发放上限错误锁死在 distinct 池大小，再加 `len<count` 拒绝则把合法主用例打死，是本 chain 第 1→2 跳的完整踩坑原型。

## Lesson 2: 批量资产写入必须整批包进一个事务——「逐条写在 base handle + 任一失败返错」会留下部分提交，重试致重复授予

- **Severity**: medium (P2)
- **Category**: error-handling
- **分诊**: fix
- **位置**: `server/internal/service/dev_cosmetic_service.go`（GrantCosmeticBatch 写库循环）

### 症状（Symptom）

`GrantCosmeticBatch` 的 `for { CreateInTx(ctx, item) }` 循环跑在 base DB handle（`tx.FromContext` 无 txCtx 时走 `r.db` 直连，逐条独立 autocommit）。若第 k 条 `CreateInTx` 失败，前 k-1 行已提交落库，service 却返 1009。调用方（demo 脚本 / 自动化 e2e）见失败重试 → 用户最终拿到部分授予 + 重复批次的脏库存。

### 根因（Root cause）

dev grant 的注释写「dev grant 走事务外批量发放（无 idempotency / 无步数语义）—— 逐条 CreateInTx，tx.FromContext 在无 txCtx 时走 r.db 直连，行为正确」。把「无幂等键 / 无步数扣减」错误地等同于「不需要事务」。但**多行写入 + 失败返错**这个组合本身就要求原子性，与有没有幂等键无关。CLAUDE.md 铁律「资产类操作必须事务（开箱、合成、穿戴、加入房间……）」——dev grant batch 是资产写入，循环 N 行更必须 all-or-nothing。

### 修复（Fix）

- `NewDevCosmeticService` 扩签名注入 `tx.Manager`（与既有 `NewDevChestService(deps.TxMgr, chestRepo)` 同模式；router.go 传 `deps.TxMgr`）。
- 写库循环包进 `s.txManager.WithTx(ctx, func(txCtx context.Context) error { for ... { CreateInTx(txCtx, ...) } })`——**循环内用 `txCtx` 而非外层 ctx**（CLAUDE.md ctx 必传 + tx.Manager 注释钦定）。任一条失败 fn return err → WithTx ROLLBACK → 整批回滚 → service wrap 1009。
- 这是 dev grant 自己的**独立事务**，与 Epic 20 race-fix 的 `runOpenChestTx` 是两条独立路径（**未动** runOpenChestTx 5a~5k / buildCacheableResponse / chest_open_service.go / `CreateInTx` 实现——CreateInTx 被新事务**复用但实现不改**）。
- 守门测试 `MidBatchFailure_RollsBackWholeBatch`：第 3 条 CreateInTx 抛错 → 断言 ① WithTx 被调 1 次 ② fn 在出错点立即中止不继续写第 4/5 条 ③ 返 1009。真实 DB 物理回滚（`user_cosmetic_items` 该批 COUNT=0）由 dockertest AC8 集成回滚 case 覆盖。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写「循环 N 次写副作用 + 任一失败向调用方返错」的代码时，**必须**把整个循环包进一个事务（all-or-nothing），**禁止**在 base / autocommit handle 上逐条写——无论有没有幂等键。

> **展开**：
> - 「无幂等键 / 无业务扣减语义」**不等于**「不需要事务」。判据是：是否有 ≥2 个写操作且失败时会向调用方报错（→ 调用方可能重试）。是 → 必须事务。
> - 资产类写入（库存 / 道具 / 余额 / 关系）循环发放，CLAUDE.md 铁律强制事务，与 idempotency 正交。
> - 事务内所有 repo 调用用 `txCtx`（WithTx 的 fn 参数），不用外层 ctx——否则写在 base handle 仍 autocommit，事务形同虚设。
> - 复用既有 `CreateInTx` 类「事务内写」方法时，**不改它的实现**，只是让新事务的 txCtx 流过它。
> - **反例**：`for _, x := range items { if err := repo.CreateInTx(ctx, x); err != nil { return wrap(err) } }`（ctx 无 tx）—— 第 k 条失败时前 k-1 条已 autocommit 落库却返错，调用方重试 → 部分授予 + 重复，是本 lesson 踩坑原型。

---

## Meta: 本次 review 的宏观教训

**这是 over-correction chain 第 2 跳的标准案例，留作未来识别同类的范式（参考 21.1 r4 / 23-1 系列 source-of-truth 锚定根因解决）：**

1. **症状不等于根因**。r1 看到「静默少发」就在校验松紧上做表层微调（加 `len<count→1009`），没回退问「count 是什么量纲」。r2 的 finding #1 正是 r1 微调的直接反弹。识别信号：连续两轮 review 在**同一处**反复横跳放行/拒绝 → 几乎一定是更上游的语义被搞错，不是阈值问题。

2. **根因要从 source-of-truth 推导，不从 review 文字推导**。本轮根因（count=实例数）来自数据库设计 §5.9 的「无 `UNIQUE(user_id, cosmetic_item_id)`」+ §22 合成 feature 定义，不是来自 codex 的措辞。codex 只说「allow duplicate picks」（症状级建议），真正的修复是改数量语义 + 加守门测试锁死，让 chain 在此终结。

3. **r1 lesson 不删**。r1 记录的是被推翻的错误方向——lesson 的蒸馏价值正在「为何走错」（症状级微调的诱惑）。本 lesson 通过 `prev_lesson` / `over_correction_chain` frontmatter 显式链到 r1，未来蒸馏时这对「错误方向 → 根因纠正」是高价值的负例-正例配对。

4. **守门测试是 chain 的终结锚**。两条根因约束（「池 < count 必须成功且产正好 count 个允许重复实例」+「中途失败整批回滚」）都写成断言。任何未来把「distinct 上限 / 不足即拒 / 事务外逐条写」hack 加回来的改动，测试立刻挂——这是阻止 chain 出现第 3 跳的机制保证。
