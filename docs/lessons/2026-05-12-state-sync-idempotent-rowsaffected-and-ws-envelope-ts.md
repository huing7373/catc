---
date: 2026-05-12
source_review: file:/tmp/epic-loop-review-14-1-r1.md (codex review on Story 14.1 r1)
story: 14-1-接口契约最终化
commit: 7aa5b8f
lesson_count: 2
---

# Review Lessons — 2026-05-12 — state-sync 幂等矛盾（RowsAffected 误判）& WS 消息冻结声明里的 envelope/payload 字段归属

## 背景

Story 14.1（节点 5 接口契约最终化）在 `docs/宠物互动App_V1接口设计.md` 落地了两块契约：
1. §5.2 `POST /api/v1/pets/current/state-sync`（REST 接口）
2. §12.3 `### 宠物状态变更（pet.state.changed）`（WS 业务消息）

并在 §1 节点 5 冻结块里登记了冻结范围 + 触发回归条件。

codex review r1 在这两块的内部一致性上抓出 2 条 finding：(1) §5.2 元信息表说"幂等"，但 service 步骤 4 把 `UPDATE ... RowsAffected == 0` 归为 1009 服务繁忙，与 MySQL/GORM 实际语义冲突；(2) §1 冻结声明把 `ts` 写成 `pet.state.changed payload` 字段，但 §12.3 字段表 + JSON 示例都把 `ts` 放在消息顶层 envelope，文档内部不一致。

两条都属"契约文档自身矛盾"类问题（**不**是实装 bug，因为节点 5 server 实装由后续 Story 14.2 ~ 14.4 才会落地）—— 但契约文档是后续实装的唯一权威，矛盾点放到实装阶段才发现，会让 Story 14.2 service 实装阶段在"该按元信息表的幂等声明做、还是按服务端逻辑的 1009 分支做"之间反复横跳；ts 字段归属错误则会让冻结审查 / iOS Codable / DTO 设计、回归检查全部基于错误字段路径执行。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | state-sync 幂等矛盾：`RowsAffected == 0` 在 MySQL/GORM 下是合法重试场景，不能归 1009 | High | docs / architecture | fix | `docs/宠物互动App_V1接口设计.md` §5.2（line 500 元信息 + line 532-534 服务端逻辑 + line 581 错误码表） |
| 2 | `pet.state.changed` 冻结声明把 envelope 字段 `ts` 误归入 payload 字段集合 | Medium | docs | fix | `docs/宠物互动App_V1接口设计.md` §1 节点 5 冻结块（line 49） |

## Lesson 1: state-sync 幂等矛盾 —— `UPDATE pets SET current_state` 的 `RowsAffected == 0` 是合法幂等成功，不是 DB 损坏

- **Severity**: high
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §5.2 line 500 / line 532-534 / line 581

### 症状（Symptom）

§5.2 元信息表（line 500）明确声明本接口"**幂等**（同 user 同 state 重复上报 → 仍 200 OK + code = 0）"；但同一节"服务端逻辑"步骤 4（line 532-534）写：

> `UPDATE pets SET current_state = ?, updated_at = NOW() WHERE id = ?`
> - DB 异常 / `RowsAffected == 0`（极罕见竞态：pet 在步骤 3 后被删 ...）→ 返回 1009 服务繁忙
> - `RowsAffected == 1` → 进入步骤 5

错误码表（line 581）同样登记 "UPDATE RowsAffected == 0（极罕见竞态）→ 1009"。

矛盾：MySQL/GORM 在执行"把字段更新为原值"的 UPDATE 时，`RowsAffected` 行为依赖具体 driver + server 配置 + 是否带 `updated_at = NOW()`：
- 常见场景：MySQL 实际 server 默认 client flag `CLIENT_FOUND_ROWS` **未开启** → 当所有列值与入参完全一致时返回 0；带 `updated_at = NOW()` 通常会让某行真发生变化 → 返回 1
- 但某些 driver/cfg 边界下（特别是 go-sql-driver/mysql + GORM `Updates` 链路 + skip hooks 模式）依然可能在 client/server time-zone 抖动 + millisecond 截断 / 实际写入 row 与 WHERE 命中 row 完全一致时返回 0
- 业务期望：同 user 同 state 重复上报 **必须** 走 200 OK 路径

按原文档实装 Story 14.2 service 层将出现："iOS 端因网络抖动或后台→前台恢复重发同一 state 上报 → 第一次 200 OK 入库；第二次进来 SELECT 仍命中同 pet 行，UPDATE 写入同值，driver 返回 `RowsAffected == 0` → service 层照原契约抛 1009 → client 命中重试逻辑或弹错"。

### 根因（Root cause）

在写"服务端逻辑步骤 4"时，把 `RowsAffected == 0` 当成了"WHERE 没命中任何 row"（这在 INSERT/DELETE 语义下确实成立，且在没有前置 SELECT 的 UPDATE 也常用此判断 race condition）。

但在本接口的语义下：
- 步骤 3 已经做了 `SELECT id FROM pets WHERE user_id=? AND is_default=1`，**WHERE 命中已被前一步证实**
- 步骤 4 的 UPDATE 是按 `WHERE id = ?`（步骤 3 查到的 pet.id）执行，命中性问题已不存在
- 此时 `RowsAffected == 0` 的**主要**触发条件是 MySQL/GORM 在"WHERE 命中 1 行但所有列值与入参完全一致"的处理 —— 这是合法的幂等场景，**不**是数据损坏

二级根因：把"UPDATE 单语句 → RowsAffected 校验"作为通用 race condition 兜底模式，**不**区分接口的业务幂等语义。state-sync 是"低消耗 + 重复合法 + 无副作用"接口，跟"扣库存 / 入账资产"类接口的 RowsAffected 校验必要性完全不同。

### 修复（Fix）

文档三处对齐：

1. **§5.2 服务端逻辑步骤 4**（line 532-534）改写为：
   - `err != nil` → 返回 1009 服务繁忙
   - `err == nil` → **不读 RowsAffected**，一律视为成功，进入步骤 5
   - 添加注释：解释 MySQL/GORM "UPDATE 原值" 的 RowsAffected 语义 + 为何 service 层不依赖该值

2. **§5.2 元信息表幂等列**（line 500）补充："DB `err == nil` 即视为成功，**不**读 `RowsAffected`，因为 MySQL/GORM 语义下'把字段更新为原值'常见 `RowsAffected == 0` 但不代表失败"

3. **§5.2 错误码表 1009 行**（line 581）改写为：仅 `err != nil`（含 driver / 网络 / 约束冲突）/ panic；**不**包含 `RowsAffected == 0`，并反向引用步骤 4

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **撰写"幂等"REST 接口的服务端逻辑、且涉及 `UPDATE ... SET col = ?` 单语句** 时，**禁止** **使用 `RowsAffected == 0` 作为"失败 / DB 损坏 / race condition" 的判定条件**。
>
> **展开**：
> - MySQL/GORM 的 `RowsAffected == 0` 在"把字段更新为原值"场景下是常见返回值（不带 `CLIENT_FOUND_ROWS` flag），**不**等价于"WHERE 没命中 row"或"DB 数据损坏"
> - "UPDATE 前先 SELECT 确认行存在"的双语句模式下，UPDATE 步骤的 `RowsAffected` **已无 race condition 校验价值** —— 命中性问题已被 SELECT 步骤兜底
> - 幂等接口的成功条件应是 `err == nil`，**不**叠加 `RowsAffected == 1` 这种额外校验 —— 后者会让"合法重试 = 入参与现状一致"的最核心幂等场景翻车
> - **反例**：
>   ```go
>   // 错误：把 RowsAffected == 0 当 DB 损坏
>   result := db.Exec("UPDATE pets SET current_state = ? WHERE id = ?", state, petID)
>   if result.Error != nil || result.RowsAffected == 0 {
>       return ErrCode1009  // 重复上报合法幂等场景被错误抛错
>   }
>   ```
>   ```go
>   // 正确：err == nil 即成功，不读 RowsAffected
>   result := db.Exec("UPDATE pets SET current_state = ? WHERE id = ?", state, petID)
>   if result.Error != nil {
>       return ErrCode1009
>   }
>   // RowsAffected 任意（0 或 1 都成功）
>   ```
> - **例外**（**不**适用上述规则）：依赖 `RowsAffected == 1` 做乐观锁 / version check / 条件 update 的语义（如 `UPDATE pets SET version=? WHERE id=? AND version=?`）—— 这类语句的 `RowsAffected == 0` 真的等价于"version 不匹配 → race condition"，必须保留校验。判断标准：WHERE 子句是否包含"业务条件"而非纯主键。
> - **二阶教训**：写接口契约时，"幂等"声明 + 服务端逻辑的成功/失败分支必须**作为一体设计**，元信息表和服务端逻辑的语义需要逐字对齐；写到"DB 异常 / RowsAffected 校验"分支时，要先回头检查 §元信息表"幂等"是否声明了，如果是幂等接口，**先**写出"err == nil 即成功"这条线，再列其他错误码

## Lesson 2: 冻结声明区分 envelope 字段 vs payload 字段 —— `ts` 是 §12.3 通用信封顶层字段，不是 payload 字段

- **Severity**: medium
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §1 line 49

### 症状（Symptom）

§1 节点 5 冻结声明（line 49）原文：
> 任何字段名 / 字段类型 / `state` 枚举值 ... / `pet.state.changed` payload 字段（`userId` / `petId` / `currentState` / `ts`）/ 广播范围 ... 的修改都必须 ...

但 §12.3 `### 宠物状态变更` 字段表（line 2200-2205）与 JSON 示例（line 2210-2218）都把 `ts` 写在消息顶层 envelope，与 `type` / `requestId` 同级；`payload` 子对象只有 `userId` / `petId` / `currentState` 三个字段。

冻结声明把 envelope 字段 `ts` 误归入 payload 字段集合，会让后续审查（"冻结范围检查"）、客户端 Codable/DTO 设计（iOS Story 15.x 解析 WS 消息时把 `ts` 字段放进 payload 子结构 → JSON 路径错位 → 解析失败）、回归检查（自动化 schema check 走错字段路径）全部基于错误字段归属执行。

### 根因（Root cause）

在写冻结声明时，把 `pet.state.changed` 这条 WS 消息的"所有契约关心字段"一次性平铺列出，没有显式区分"envelope 层"和"payload 层"。

而 §12.3 通用信封约定（节点 4 Story 11.1 落地）规定所有业务 WS 消息共享同一信封结构：
```
{
  "type": <string, 消息类型>,
  "requestId": <string, 主动推送固定 "">,
  "payload": <object, 消息特有字段>,
  "ts": <number, server 发送时间戳>
}
```

`ts` 是**所有**业务 WS 消息共享的 envelope 字段（与 `member.joined` / `member.left` / `room.snapshot` 等同语义），不是 `pet.state.changed` 私有的 payload 字段。

二级根因：冻结声明本身是一个"压缩列表"，把 envelope + payload 字段混在一起列，省字数但失精度。

### 修复（Fix）

§1 line 49 改为：
> 任何字段名 / 字段类型 / `state` 枚举值（1 / 2 / 3）/ 错误码 ... 触发条件 / `pet.state.changed` payload 字段（`userId` / `petId` / `currentState`）+ 顶层 envelope 字段（`type` / `requestId` / `ts`，遵循 §12.3 通用信封）/ 广播范围 ...

显式拆开 payload 字段和 envelope 字段；envelope 部分反向引用 §12.3 通用信封约定，保证读者能定位到原始约定。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **撰写 WS 消息契约的冻结声明 / 字段清单时**，**必须** **显式区分 envelope 字段 vs payload 字段两层**，**禁止**把两层字段平铺成单一列表。
>
> **展开**：
> - 节点 4 Story 11.1 已锚定 WS 通用信封结构（`type` / `requestId` / `payload` / `ts`）—— `payload` 是子对象，与 envelope 顶层字段语义/作用域**不同**
> - `ts` / `type` / `requestId` 是**所有**业务 WS 消息共享的 envelope 字段，**不**属于任一具体消息的 payload
> - 冻结声明的字段清单写法应统一为："payload 字段（A / B / C）+ 顶层 envelope 字段（type / requestId / ts，遵循 §12.3 通用信封）"两段式
> - **反例**：
>   ```markdown
>   - 任何 `pet.state.changed` payload 字段（userId / petId / currentState / ts）的修改都必须 ...
>   <!-- 错误：把 envelope 字段 ts 混入 payload 列表 -->
>   ```
>   ```markdown
>   - 任何 `pet.state.changed` payload 字段（userId / petId / currentState）+ 顶层 envelope 字段（type / requestId / ts，遵循 §12.3 通用信封）的修改都必须 ...
>   <!-- 正确：两层显式拆开 -->
>   ```
> - **核验启发式**：写完冻结声明后，回到该消息的字段表 + JSON 示例对照一遍 —— JSON 示例中 `ts` 是缩进在 `}` 之后还是 `payload: { ... }` 之内？前者是 envelope（顶层），后者才是 payload。**别**靠记忆，每次都看一眼示例。

---

## Meta: 本次 review 的宏观教训（可选）

两条 finding 的共同模式是"**同一节文档内部多处对同一契约点的描述不对齐**"：
- Finding 1：§5.2 元信息表（声明幂等）vs 服务端逻辑步骤 4（按非幂等校验 RowsAffected）vs 错误码表 1009（把 RowsAffected == 0 列为错误）—— 三处描述同一接口的"成功条件"，三处不同
- Finding 2：§1 冻结声明（把 ts 列为 payload）vs §12.3 字段表（ts 在顶层）vs §12.3 JSON 示例（ts 在顶层）—— 三处描述同一字段的"层级归属"，第一处不同

**宏观规则**：撰写或修改某个接口/消息的契约描述时，**禁止**只改其中 1 处而不顺手核对其他 2-N 处。建议工作流：
1. 改前先 `grep -n` 列出该契约点的**所有**出现位置（接口元信息表 / 服务端逻辑 / 错误码表 / 字段清单 / JSON 示例 / 冻结声明 / 跨章节反向引用）
2. 改后用同一组关键词回 grep 一遍，确认每处都已对齐
3. 写"幂等"/"非幂等" 之类的元信息标签时，把它当作"语义合同" —— 服务端逻辑 + 错误码表 + 任何下游引用都必须严格执行该合同，**不**允许局部分支偏离

这条 meta 教训与 `docs/lessons/2026-04-26-契约文档全文sweep与跨文件副本同步.md` 主题同源，可在后续蒸馏时合并为"契约文档全局一致性"系列。
