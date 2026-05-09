---
date: 2026-05-09
source_review: codex review (epic-loop round 2 for story 12-4) — file: /tmp/epic-loop-review-12-4-r2.md
story: 12-4-成员加入-离开-ws-消息处理
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-09 — WS codec 必须在构造 payload 前校验 required 字段语义有效性（12-4 r2）

## 背景

Story 12.4 (`member.joined` / `member.left` WS 消息处理) round 2 codex review 发现：`WSMessageCodec.decode("member.joined")` 路径只依赖 Swift `Decodable` 自动解码，**Decodable 只挡 absent / type-mismatch，不挡语义无效**。当 server 推送 `userId == ""` 或 V1 §12.3 钦定非空的 `nickname == ""` 时，Decodable 仍成功解码出空字符串，codec 仍构造 `MemberJoinedPayload` → `RealRoomViewModel.applyMemberJoined` mutate roster（append 一条空 entry 或 enrich 既有 entry 时把 nickname 改空），直到下个 snapshot 才能恢复。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `member.joined` / `member.left` codec 必须在构造 payload 前校验 required 字段非空 | medium (P2) | error-handling | fix | `iphone/PetApp/Core/Networking/WSMessageCodec.swift:60-77` |

## Lesson 1: WS codec required-field 语义校验是 client 防线，不能依赖 Decodable

- **Severity**: medium (P2)
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/WSMessageCodec.swift:60-77`

### 症状（Symptom）

`member.joined` payload 在 server 端推送 `userId == ""` 或 `nickname == ""`（违反 V1 §12.3 钦定）时，codec 仍 decode 成功并 emit `.memberJoined(payload)`。`RealRoomViewModel.applyMemberJoined` 收到后会：
- userId 不在 members → append 一条 `RoomMember(id: "", name: "")`（"成员"占位）→ UI 显示一行空白成员
- userId 已存在 + 新 nickname 空 → enrich path 把已有 nickname 改成空串 → UI 上现有成员名字变占位

直到下个 `room.snapshot` 重建 members 才能恢复。

`member.left` payload `userId == ""` 同理 —— 即便 ViewModel.applyMemberLeft 会因 userId 不匹配走 ignore 路径不破坏，codec 层多走一道 fallback 更稳（防未来 ViewModel 实装变更踩坑）。

### 根因（Root cause）

Swift `Decodable` 的契约是「字段在不在 + 类型对不对」，**不**校验「字段值是否符合业务语义」（非空 / 长度 / 范围）。codec 实装写 `MemberJoinedPayloadDTO` Decodable struct 时只声明 `let userId: String` / `let nickname: String`，没追加「empty string 非法」语义校验，把这层防线漏给了下游 ViewModel。

ViewModel 层的 contract 是「拿到 typed payload 就 mutate state」—— 它**不应该**也**没义务**做 codec 层应该挡住的语义校验：codec 是 wire-format → typed model 的边界，wire-format 不合法的数据应该在边界处 fallback 为 `.unknown(rawType:)`，不应让 ill-formed model 进入 domain 层。

### 修复（Fix）

`WSMessageCodec.decode` 在 `member.joined` / `member.left` 路径，decode 出 DTO 后、`toDomain()` 之前，加 explicit guard：

```swift
case "member.joined":
    do {
        let dto = try makeDecoder().decode(MemberJoinedEnvelope.self, from: data).payload
        guard !dto.userId.isEmpty else {
            os_log(.error, log: logger, "member.joined rejected: empty userId")
            return .unknown(rawType: "member.joined")
        }
        guard !dto.nickname.isEmpty else {
            os_log(.error, log: logger, "member.joined rejected: empty nickname (V1 §12.3 钦定非空)")
            return .unknown(rawType: "member.joined")
        }
        return .memberJoined(dto.toDomain())
    } catch { ... return .unknown(rawType: "member.joined") }

case "member.left":
    do {
        let dto = try makeDecoder().decode(MemberLeftEnvelope.self, from: data).payload
        guard !dto.userId.isEmpty else {
            os_log(.error, log: logger, "member.left rejected: empty userId")
            return .unknown(rawType: "member.left")
        }
        return .memberLeft(dto.toDomain())
    } catch { ... return .unknown(rawType: "member.left") }
```

回归测试 4 条加在 `RealRoomViewModelTests` 末尾（baseline + 3 个 reject 路径）：
- `testCodecMemberJoinedValidPayloadDecodesAsMemberJoined` —— happy path 不被误伤
- `testCodecMemberJoinedEmptyUserIdFallsBackToUnknown`
- `testCodecMemberJoinedEmptyNicknameFallsBackToUnknown`
- `testCodecMemberLeftEmptyUserIdFallsBackToUnknown`

每条断言 `result` 是 `.unknown(rawType: "member.joined" / "member.left")`，rawType 区分语义校验失败 vs envelope 解码失败（`""`）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在实装**任何 wire-format codec**（WS / HTTP JSON DTO → Domain Model）的 decode 路径时，**必须在构造 Domain Model 之前**对 contract 钦定的 **required 字段做显式语义校验**（至少包含：string 字段 `isEmpty` / id 字段 `isEmpty` / 数值字段 range），失败时 **fallback 为 `.unknown(rawType: ...)` + log error**，**禁止**把语义校验下放给下游 ViewModel / UseCase。
>
> **展开**：
> - **Decodable 的契约边界**：Swift `Decodable` / Go `json.Unmarshal` / Kotlin `kotlinx.serialization` 这类 reflection-based decoder 只挡「字段在不在 + 类型对不对」。空字符串、负数、违反业务语义的合法 JSON 一律会 decode 成功。
> - **codec 层是 wire→domain 的最后一道边界**：domain 层（ViewModel / UseCase）拿到 typed value 就应该假设它符合语义；语义校验应该在 codec 层挡住，反向把 ill-formed model 推到 domain 层 = 把"server 不该发的数据"误差转嫁给 client state machine，污染 UI 直到下个 authoritative event 才能修复。
> - **校验失败的 fallback 形态**：用 `.unknown(rawType: <type>)`（rawType 携带语义信号 + 区分 envelope 解码失败的 `""`），不抛 throw 不破坏 stream —— 与 payload schema mismatch 的兜底路径同形。log 用 `.error` level（而非 `.info`），方便 production observability 抓异常 server 行为。
> - **测试三件套**：每个新加的 required-field 语义校验，至少加 1 条 baseline happy-path（防 reject 路径误伤合法 payload）+ 1 条 each-required-field 的 reject case。复用既有 ViewModel test target 即可（避免新建文件触发 Xcode pbxproj 改动），test 函数命名前缀 `testCodec` 区分纯 codec 单测 vs ViewModel 集成测试。
> - **反例**：
>   - `let payload = try decode(...).toDomain(); return .memberJoined(payload)` —— 缺校验直接 mutate domain。
>   - 把校验放在 ViewModel：`func applyMemberJoined(_ p) { guard !p.userId.isEmpty else { return } }` —— 同样能挡，但 codec 层依然 emit ill-formed event，分散语义边界，未来加新 consumer（如 analytics / debug log）会重复实现校验且容易漏。
>   - 用 `.unknown(rawType: "")` 当 reject 信号 —— 与 envelope 解码失败混淆，丢失语义。
>   - 仅校验 `userId.isEmpty` 不校验 `nickname.isEmpty` —— V1 §12.3 钦定 nickname 非空，client 把它当 contract 一部分来挡，否则 server 推送 ill-formed payload 时 UI 会出现 "" 占位行直到下次 snapshot。
