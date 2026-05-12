---
date: 2026-05-12
source_review: codex review r1 for Story 14.3 (/tmp/epic-loop-review-14-3-r1.md)
story: 14-3-修改-roomsnapshotbuilder-snapshot-含真实-pet-currentstate
commit: ddc1224
lesson_count: 2
---

# Review Lessons — 2026-05-12 — *int8 解引用必须 nil-guard 兜底 + integration 端到端必须用区别于 hardcoded 路径的真值证明切换

## 背景

Story 14.3 把 3 个 site（snapshot.go / room_service.GetRoomDetail / broadcastMemberJoined）的 `pet.currentState` 从 hardcoded `1` 切到从 DB 读真实值。codex r1 review 抓出两条：(1) 两个 RosterRow → DTO 转换 site 在 `r.PetID != nil` 分支内**直接** `int(*r.CurrentState)` / `*r.CurrentState`，没有 nil 守卫；(2) `broadcastMemberJoined` 的 integration test 仍 seed `current_state=1`、断言 `==1`，无法证明 site (iii) 真切了（hardcoded 路径也能通过相同断言）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `*int8 CurrentState` 解引用无 nil 守卫 | high | error-handling | fix | `server/internal/app/ws/snapshot.go:319` + `server/internal/service/room_service.go:1263` |
| 2 | `member.joined` integration test 仍断言 currentState==1，无法证明 site (iii) 切换 | medium | testing | fix | `server/internal/service/room_service_integration_test.go:1289` |

## Lesson 1: `*int8 / *uint64` 等 nullable 列解引用即使 schema 不变量保证非 nil，仍必须 nil-guard 兜底

- **Severity**: high
- **Category**: error-handling
- **分诊**: fix
- **位置**: `server/internal/app/ws/snapshot.go:319` + `server/internal/service/room_service.go:1263`

### 症状（Symptom）

两处 RosterRow → DTO 转换在 `r.PetID != nil` 分支内**直接** `int(*r.CurrentState)` / `*r.CurrentState` 解引用 `*int8`。schema §6.4 `pets.current_state NOT NULL DEFAULT 1` 保证 `PetID != nil → CurrentState != nil`，但代码本身不防御；恶意 / 损坏的扫描结果（如 future schema migration + 旧 binary 跑 / pets 表数据被外部进程改坏）会 panic 整个请求 / snapshot 路径。

### 根因（Root cause）

> "schema 不变量保证 X，所以代码可以省去 X 的 nil check" —— 这是**反模式**。

Schema 不变量是**事实**层面的保证，不是**类型系统**层面的保证。Go 的 `*int8` / `*uint64` 类型本身允许 nil，编译器不会因为某条 SQL 列 `NOT NULL DEFAULT 1` 就把对应 Go 字段从 `*int8` 升级成 `int8`。当 schema 不变量被外力破坏时（future migration / 第三方写入 / 旧 binary 跑新 schema），代码会从"理论不可能"路径走进 nil deref → panic 整个 goroutine（且 HTTP / WS 路径不被 recover middleware 拦时直接踢连接）。

更糟：因为"理论不可能"，这种 panic 不会有单测覆盖，CI 永远绿；线上爆出来时根本不知道是哪条 row 触发的（panic stack 不带 row 数据）。

### 修复（Fix）

在两处解引用之前加 nil guard，兜底默认值 `1`（与 §6.4 `NOT NULL DEFAULT 1` 一致）：

```go
// snapshot.go:319
if r.PetID != nil {
    currentState := 1
    if r.CurrentState != nil {
        currentState = int(*r.CurrentState)
    }
    m.Pet = &SnapshotPet{
        PetID:        strconv.FormatUint(*r.PetID, 10),
        CurrentState: currentState,
    }
}

// room_service.go:1263
if r.PetID != nil {
    var currentState int8 = 1
    if r.CurrentState != nil {
        currentState = *r.CurrentState
    }
    m.Pet = &MemberPetOutput{
        PetID:        *r.PetID,
        CurrentState: currentState,
        Equips:       []EquipOutput{},
    }
}
```

回归测试：`TestRealSnapshotBuilder_BuildSnapshot_Malformed_PetID_NonNil_CurrentState_Nil`（snapshot_test.go）+ `TestRoomService_GetRoomDetail_Malformed_PetID_NonNil_CurrentState_Nil`（room_service_test.go）覆盖 malformed row 场景。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **解引用任何 Go 指针字段（`*int8` / `*int64` / `*uint64` / `*string` / `*time.Time` 等）** 时，**必须**先检查 nil 并提供兜底，**不论** schema 是否保证该列 NOT NULL。
>
> **展开**：
> - 兜底值优先选 schema DEFAULT（如 `current_state` 的 DEFAULT 1）；其次选业务语义的"零态"；最后才考虑返 error。
> - Schema 不变量是**事实**层面保证，不是**类型系统**层面保证；Go 编译器不会因为 NOT NULL 就把字段从 `*int8` 升级成 `int8`。
> - "理论不可能"的路径恰好是**单测覆盖盲区** —— 因为 happy fixture 永远 seed 合法数据。必须**显式**写 malformed-row 测试（PetID != nil + CurrentState == nil）来锁回归。
> - **反例**：`m.Pet = &SnapshotPet{CurrentState: int(*r.CurrentState)}` —— 在 `if r.PetID != nil` 分支内**直接**解引用 `*r.CurrentState`，依赖外部 schema 不变量保证非 nil。这种"省 nil check"的代码**永远**算技术债，review 必须抓。
> - **反例**：把"不需要 nil check"的理由写在注释里（"schema §6.4 NOT NULL DEFAULT 1 → *r.CurrentState 必非 nil"）。注释**不防御 panic**；nil guard 才防御 panic。注释顶多用来解释**为什么**兜底值选 1 而不是 0 / 2。

## Lesson 2: Integration 端到端证明"hardcoded 切换到真实值"必须用**区别于 hardcoded 值**的 fixture

- **Severity**: medium
- **Category**: testing
- **分诊**: fix
- **位置**: `server/internal/service/room_service_integration_test.go:1289`（既有 J1a case）

### 症状（Symptom）

Story 14.3 把 `broadcastMemberJoined` 的 `pet.currentState` 从 hardcoded `1` 切到从 `mysql.Pet.CurrentState` 读真实值。但 integration test (case 14 = J1a) seed `insertPet(..., currentState=1, ...)` → 断言 `payload.pet.currentState == 1` —— 切换前 hardcoded 路径**也**能通过相同断言，**无法证明** site (iii) 真的切了。Unit 层 (`TestRoomService_BroadcastMemberJoined_PetCurrentState_2`) 已有 case 2，但 integration 层缺对应证据。

### 根因（Root cause）

"hardcoded → 真实值" 切换的回归测试有一个隐蔽要求：fixture 的 seed 值必须**区别于** hardcoded 值。否则两条路径产出相同结果，测试无法分辨"切换是否生效"。

进一步：当 unit 层已经有 `PetCurrentState_2` 的 case 时，作者会下意识觉得"切换已被测试覆盖"，从而**漏掉**对应的 integration 证据。但 unit 层用 mock petRepo 返 `&mysql.Pet{CurrentState: 2}`，证明的是 service 层从 `mysql.Pet.CurrentState` 字段读；integration 层证明的是 **mysql.Pet.CurrentState 字段真的从 DB row 的 current_state 列扫上来**（gorm 字段绑定 / column tag / scan 类型匹配都是真实的）。两层的语义**不重叠**。

### 修复（Fix）

新增 `TestRoomServiceIntegration_JoinRoom_BroadcastMemberJoined_PetCurrentState_2`（room_service_integration_test.go）：
- `insertPet(t, sqlDB, 8002, userB, 1, "PetB", 2, 1)` —— seed `current_state=2` (walk)
- B join → 断言 broadcast 出来的 `payload.pet.currentState == 2`
- hardcoded 路径（如果回滚）会返 1 → 测试挂

```go
// 关键 seed：current_state=2 (walk)，非 hardcoded 1
insertPet(t, sqlDB, 8002, userB, 1, "PetB", 2, 1)
// ...
if payload.Pet.CurrentState != 2 {
    t.Errorf("payload.pet.currentState = %d, want 2 (Story 14.3 真实驱动 mysql.Pet.CurrentState；hardcoded 路径会返 1)", ...)
}
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写"hardcoded 值 → DB 真实值"切换的回归测试** 时，**必须**让 fixture seed 值**区别于** hardcoded 默认值（如 hardcoded 是 `1` → seed `2` 或 `3`）。
>
> **展开**：
> - 写完 unit 层 mock 测试（`CurrentState_2`）后，**必须**也写 integration 层（real DB + real gorm scan）的对应 case；两层语义不重叠：unit 证明 service 读 struct 字段，integration 证明 struct 字段真的从 DB 列扫上来。
> - 既有 happy case 通常 seed hardcoded 默认值（`current_state=1`），断言也是 `==1`，看似覆盖切换，**实际上无法分辨切换是否生效**。这是 review 必须抓的盲区。
> - 当 story scope 是"3 个 site 全切真实值"时，**3 个 site 各自必须有**至少一条 fixture-seed-value-differs-from-hardcoded 的端到端 case；缺一个就是"site (iii) 没有切换证据"。
> - **反例**：`insertPet(..., currentState=1, ...)` 配 `assert == 1` —— 命名上像是"覆盖了 currentState"，实际上**与切换前的 hardcoded 路径无差别**。
> - **反例**：以为"unit 层有 `_PetCurrentState_2` case 就够了" —— unit 层 mock 掉了 DB scan / gorm column tag / 类型转换，对应的 integration 证据仍然缺失。

---

## Meta: 本次 review 的宏观教训

两条 finding 看似独立，背后的共同模式是 **"schema / 上层不变量被默认成代码 / 测试可以省的理由"**：
- Finding 1：schema §6.4 NOT NULL DEFAULT 1 被默认成"代码可以省 nil check"
- Finding 2：unit 层 `_PetCurrentState_2` 被默认成"integration 层可以省对应 case"

**通用规则**：上层不变量（schema / spec / 上一层测试）**只**是事实层面的保证，**不**自动传导成下一层代码 / 测试的"可以省"。每一层都要**自己**在自己的语境内防御 / 证明。

Schema 防御交给代码（nil guard）；代码防御交给测试（malformed fixture）；service 层语义交给 unit；DB scan 语义交给 integration。各层语义不重叠，省任何一层都是技术债。
