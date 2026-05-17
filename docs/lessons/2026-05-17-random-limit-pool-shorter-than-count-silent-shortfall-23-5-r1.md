---
date: 2026-05-17
source_review: "codex review --uncommitted output (file: /tmp/epic-loop-review-23-5-r1.md)"
story: 23-5-修改开箱事务-创建-user_cosmetic_items-实例-补-chest_open_logs-reward_user_cosmetic_item_id
commit: b5a17ba
lesson_count: 1
---

# Review Lessons — 2026-05-17 — ORDER BY RAND() LIMIT N 池小于 N 时静默少发：只判空集漏掉 0<len<count 这一档（23-5 r1）

## 背景

Story 23.5 激活了 Story 20.8 的 `/dev/grant-cosmetic-batch` 真实写库（节点 7 stub 退役）。`GrantCosmeticBatch` 调 `cosmeticItemRepo.FindRandomByRarity(ctx, rarity, count)`（SQL `... WHERE rarity=? AND is_enabled=1 ORDER BY RAND() LIMIT ?`）抽 count 个 cosmetic_item_id，逐条 `CreateInTx` 写 user_cosmetic_items。codex review（`--uncommitted`）针对未提交的 dev-story 改动，唯一 finding [P1] 指向 `dev_cosmetic_service.go:107-114`：池小于 count 时静默少发仍返 success。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | dev grant 池不足 count 时静默少发仍返 success | high (P1) | error-handling | fix | `server/internal/service/dev_cosmetic_service.go:107-114` |

## Lesson 1: `LIMIT N` 查询返回行数 ≤ N 是合法语义，"满量"校验必须比对 len 与请求量，不能只判空集

- **Severity**: high (P1)
- **Category**: error-handling
- **分诊**: fix
- **位置**: `server/internal/service/dev_cosmetic_service.go:107-114`

### 症状（Symptom）

`GrantCosmeticBatch` 只在 `len(cosmeticItemIDs) == 0` 时报 1009，其余情况一律逐条写库后 `return nil`（success）。但 `FindRandomByRarity` 用 `ORDER BY RAND() LIMIT ?`：当某 rarity 的 enabled 池 < count 时**合法**返回少于 count 个 id。seed 中 common 仅 8 件，handler 接受 count 至 100，story demo 明确用 count=10 → SQL 返 8 行，service 只插 8 行却返 success，调用方静默拿到比请求少的实例（典型 demo 路径已可触发，不是理论边界）。

### 根因（Root cause）

实装把 `FindRandomByRarity` 的契约误读成"要么返 count 个、要么返空（seed 数据完整性异常）"二值模型，因此只对 `len==0` 设防。实际 `LIMIT N` 的 SQL 契约是"返回**至多** N 行"——`0 < 返回行数 < N` 是一等公民状态，恰好发生在"池本身就比请求小"这一**正常配置**下（common 池 8 < demo count 10）。story AC6 注释也只写了"FindRandomByRarity 无数据 → 1009"，强化了这个二值误读，没人显式覆盖"池不足请求量"这一档。空集只是 `len < count`（当 count>0 时）的一个子集，单判空集必然漏掉 `0<len<count`。

### 修复（Fix）

`dev_cosmetic_service.go`：把 `if len(cosmeticItemIDs) == 0` 收紧为 `if len(cosmeticItemIDs) < int(count)`，在**写库前**返回 `ErrServiceBusy(1009)`（与既有空池异常同族；空集是其子集）。用 `FindRandomByRarity` **实际返回的 len** 与请求 count 直接比对——不引入"先 `SELECT COUNT` 再 fetch"的双查询（那会引入 count 与 fetch 之间的 race）。同步收紧 interface / impl 三处 doc 注释。新增守门单测 `...RarityPoolShorterThanCount_Returns1009_NoPartialInsert`：stub 返 8 个 id（模拟 common 池）+ count=10 → 断言 ① 返 1009 ② `CreateInTx` 一次都没被调（杜绝 8 行部分写入，原子性视角，参考 23-5 AC8 回滚 case 风格）。

before/after（核心 4 行）：

```go
// before
if len(cosmeticItemIDs) == 0 { return apperror.New(ErrServiceBusy, ...) }
// after
if len(cosmeticItemIDs) < int(count) { return apperror.New(ErrServiceBusy, ...) }
```

修复严格限定在新激活的 dev grant 路径；**未动** runOpenChestTx 5a~5k / buildCacheableResponse / replayFromCachedResponse / chest_open_service.go / user_cosmetic_item_repo.go CreateInTx（这些是 23-5 已 review 通过的 Epic 20 r1~r15 race-fix 核心，本 finding 不涉及）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在用 **`LIMIT N` / `ORDER BY RAND() LIMIT N` / "随机抽 N 个" 类查询拿一批结果且业务要求"足量"** 时，**必须**用 `len(返回) < 请求量` 作为失败判据，**禁止**只判 `len == 0`。

> **展开**：
> - `LIMIT N` 的 SQL 契约是"至多 N 行"，`0 < 行数 < N` 是合法且常见状态（池本身小于请求时必然命中），不是只有空集才异常。
> - "满量"校验优先用**已返回结果的 len** 与请求量比对（单查询、无 race）；不要为校验补一个 `SELECT COUNT` 前置查询——那会在 count 与 fetch 之间引入并发漂移。
> - 校验必须在**写副作用之前**（写库 / 发奖 / 扣账之前）拒绝整批，杜绝"部分插入后返 success"的静默少发；返回错误码沿用同族（空集与短发是同一"池无法满足请求"语义，空集是 `len<count` 子集，别为它单开一档代码路径）。
> - 加守门测试时同时断言"错误码 + 副作用零次调用"（不只断言报错，还要断言没留下部分写入），原子性视角。
> - **反例**：`if len(ids) == 0 { return err }; for _, id := range ids { create(id) }; return nil` —— 当 `len(ids)` 落在 `(0, count)` 时静默少发还报 success，是本 lesson 的踩坑原型；尤其当上游 handler 接受的 count 上限远大于配置池容量（如 count≤100 vs common 池 8）时一定会发生，不是理论边界。

---

## Meta: 本次 review 的宏观教训

`LIMIT`/采样类查询的返回基数与请求基数是两个量纲，写"足量"语义时永远比对二者而非只防空集——这是数据层契约误读的高频模式，与 23-1 系列"冻结契约必须枚举全部边界 case"同源：边界不是 {正常, 空}，而是 {足量, 部分, 空} 三态完整矩阵。
