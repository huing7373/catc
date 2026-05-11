---
date: 2026-05-10
source_review: "file: /tmp/epic-loop-review-12-7-r1.md (codex --uncommitted, Story 12.7 round 1)"
story: 12-7-创建-加入-退出-use-case-主界面入口完善
commit: 0e631e8
lesson_count: 2
---

# Review Lessons — 2026-05-10 — WS connect 同步抛错与 URL path roomId 注入风险

## 背景

Story 12.7（创建/加入/退出 UseCase + 主界面入口完善）round 1 codex review 发现 2 条 P2：

1. `RealRoomViewModel.subscribeRoomIdConnect` 内 nil→A / A→B 分支**先**设置 `wsState = .connected` **再** spawn Task `try? await client.connect(roomId:)` —— `try?` 静默吞 sync failure（如 `WSError.tokenMissing`、URL 构造异常），UI 卡死在错误的 connected 占位状态但实际无 socket。同模式在 12.7 引入的多个 `connect(roomId:)` callsite 重复（bind 路径 + sink nil→A + sink A→B 共 3 处）。
2. `RoomEndpoints.joinRoom(roomId:)` / `leaveRoom(roomId:)` 把 raw `roomId` 直接 string interpolate 到 URL path（`"/rooms/\(roomId)/join"`）。设计意图是 join flow 容许任意输入依赖 server 返回 1002 "房间号格式不合法"，但 raw input 含 URL reserved 字符（`/`、`?`、`#`）会改变 request path/query —— server-side 1002 校验**永远拿不到该输入**，client 看到的是 transport 错误 / 路由到错误 endpoint。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | WS connect 同步抛错路径 wsState 卡 .connected 假态 | P2 | error-handling | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:191-208,288-322,338-371` |
| 2 | roomId 直插 URL path 不做 percent-encoding 让 server 1002 路径走不通 | P2 | security | fix | `iphone/PetApp/Features/Room/UseCases/RoomEndpoints.swift:32-38,46-52` |

## Lesson 1: WS connect 同步抛错路径 wsState 卡 .connected 假态

- **Severity**: P2
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:263-278` 主路径 + 290-322 A→B 分支 + 174-208 bind 分支

### 症状（Symptom）

`subscribeRoomIdConnect` 在 nil→A 进入房间分支：

```swift
// 旧实装（buggy）
self.webSocketClient?.prepareForReconnect()
if self.webSocketClient != nil {
    self.wsState = .connected  // ← 同步 set，假定 connect 会成功
}
self.startConsumingMessages()
if !roomId.isEmpty {
    let client = self.webSocketClient
    Task { [weak client] in
        try? await client?.connect(roomId: roomId)  // ← try? 吞掉 sync failure
    }
}
```

当 `client.connect(roomId:)` 同步抛错（最典型的两种：`WSError.tokenMissing` / `WSError.invalidURL`）时：
- `try?` 把 throw 转成 `nil`，调用者完全感知不到失败
- `wsState` 已经 sync set 成 `.connected`，receive loop 没机会 emit `.connectionStateChanged(.disconnected)`（因为根本没建立 underlying task）
- UI 显示用户在房间内、socket 已连，但实际 WS 无连接 → 用户在房间里无任何成员推送、无心跳、无聊天 → 沉默故障（silent failure）

A→B 切换分支与 bind first-injection 分支同模式重复同样问题。

### 根因（Root cause）

两层认知漏洞：

1. **"同步前置占位 + 异步真实拨号"模式忽略了同步异常路径**：开发时只考虑了"成功路径占位 .connected → 等 receive loop 自然 emit 真实 .connected/.reconnecting/.disconnected 接管"的 happy flow，没考虑 `connect(roomId:)` 在 await **之前**就 sync throw 的场景（token 取不到 / URL 构造失败这类 fail-fast 场景）。
2. **`try? await` 反模式**：Swift 的 `try?` 是个无差别错误吞噬器；用在副作用调用（不关心返回值的网络 I/O）上时**特别危险**，因为 caller 唯一的错误反馈通道（throw）被静默丢弃，状态机无法纠错。

### 修复（Fix）

3 处 callsite 全部改为 `do/catch` + 失败纠错：

```swift
// 新实装
self.webSocketClient?.prepareForReconnect()
if self.webSocketClient != nil {
    self.wsState = .connected  // 占位仍 set（in-room scaffold 同步反馈）
}
self.startConsumingMessages()
if !roomId.isEmpty {
    let client = self.webSocketClient
    let presenter = self.errorPresenter
    Task { @MainActor [weak self, weak client] in
        guard let client else { return }
        do {
            try await client.connect(roomId: roomId)
            // 成功路径：占位 .connected 已 set，不重写避免与 .connectionStateChanged
            // reactive 路径竞争（receive loop emit .connected 时已是真实信号）.
        } catch {
            os_log(.error, "RealRoomViewModel: nil→A connect(roomId:%{public}@) failed: %{public}@",
                   roomId, String(describing: error))
            // **关键纠错**：旧 try? 在此处吞掉信号；新实装把占位
            // .connected 还原成真实 .disconnected，UI 不再卡在"假 connected".
            self?.wsState = .disconnected
            presenter?.present(error)
        }
    }
}
```

3 处改动（同模式应用）：
- `RealRoomViewModel.swift:191-208` —— bind 路径（first-injection / swap 后追加 connect 触发）
- `RealRoomViewModel.swift:288-322` —— sink nil→A 分支
- `RealRoomViewModel.swift:338-371` —— sink A→B 分支

回归测试 `testNilToAConnectFailureKeepsWsStateDisconnected`：mock `connectError = .tokenMissing` → 验证 `vm.wsState` 最终是 `.disconnected` 而非 `.connected`。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写"sync 占位状态 + spawn Task 异步真实操作"模式时，**必须**在 Task 内用 `do/catch` 处理 throw —— **禁止**用 `try?` 吞掉网络/I/O 异步调用的 sync failure；catch 路径必须把占位状态纠错回真实失败状态 + 通过 `errorPresenter.present(error)` 让用户可见。
>
> **展开**：
> - "成功靠 reactive 信号自然 emit（如 receive loop 推 .connectionStateChanged）"是合理设计，但**不能**用来掩盖 sync failure —— 那条信号路径根本没建起来时，receive loop 永远不会 emit。
> - 占位状态（如 `.connected` 占位）属于"乐观假定"；任何乐观假定都必须在 catch 路径有"悲观纠错"出口。
> - `try?` 在副作用调用上是反模式 —— 调用 `client.connect()` / `repository.save()` / `db.insert()` 这种"我不要返回值，但失败必须感知"的场景，永远用 `do { try await ... } catch { ... }`，**不**用 `try?`。
> - 同模式在多 callsite 重复时必须**全部**修（搜全文 `try? await`）；review 通常只列举一处但提示"其他 callsite 同样"，不要只补一处然后下轮被同 reviewer 抓回。
> - **反例**：`Task { try? await client.connect(roomId: roomId) }` —— sync failure 路径下 wsState / state 机器永远停在乐观占位；**正例**：`Task { do { try await client.connect(...) } catch { state = .failed; presenter.present(error) } }`。
> - **反例 2**：`if condition { state = .connected }` 紧跟 `Task { try? await connect() }` —— 占位与真实拨号脱钩，sync failure 时占位永远不被纠错；**正例 2**：占位 set 后必须有 catch 路径反向 set `.disconnected`/`.failed`。

## Lesson 2: roomId 直插 URL path 让 server 业务校验路径走不通

- **Severity**: P2
- **Category**: security
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/UseCases/RoomEndpoints.swift:32-38, 46-52`

### 症状（Symptom）

```swift
// 旧实装
public static func joinRoom(roomId: String) -> Endpoint {
    Endpoint(
        path: "/api/v1/rooms/\(roomId)/join",  // ← raw roomId 直插
        ...
    )
}
```

设计意图：join flow 容许 modal 输入任意房间号字符串，**故意**不做 client 校验，依赖 server 返回 business code 1002 "房间号格式不合法"。但当 raw input 含 URL reserved 字符时：
- `roomId = "AA/BB"` → path 变 `/api/v1/rooms/AA/BB/join` → server 收到完全错位的 URL，路由到完全不同的 endpoint（或 404）
- `roomId = "1234?evil=1"` → path 变 `/api/v1/rooms/1234`，query 变 `?evil=1` → server 收到 path = "1234"，1002 校验"看到的不是用户输入"
- `roomId = "1234#frag"` → fragment 在 client 侧被切掉根本不发 server → request path 短了一截，server 无从校验

结果：**1002 业务错误处理路径走不通** —— 用户输入非法字符触发的不是预期的 alert "房间号格式不合法"，而是 transport error / 404 / 路由错位等不一致体验。

### 根因（Root cause）

两点：
1. **混淆 "client 不校验" 与 "client 不 escape"**：把"逻辑校验委托给 server"误推广到"URL 编码也委托给 server"；URL path 编码是**传输层**约束，不是业务校验，client 必须负责保证 raw input 不被 reserved 字符 hijack URL 结构。
2. **依赖 Foundation `URLComponents` 但用 string interpolation**：Swift 的 `String(format:)` / `"\(value)"` 对 URL component 完全无感知，需要显式调 `addingPercentEncoding(withAllowedCharacters:)`。

### 修复（Fix）

```swift
// 新实装
public static func joinRoom(roomId: String) -> Endpoint {
    Endpoint(
        path: "/api/v1/rooms/\(escapePathSegment(roomId))/join",
        ...
    )
}

private static let roomIdPathAllowed: CharacterSet = {
    var allowed = CharacterSet.urlPathAllowed
    // urlPathAllowed 默认含 `/`（path 分隔符），必须 subtract;
    // `?` `#` 已不在 urlPathAllowed 默认集，自动 escape.
    allowed.remove(charactersIn: "/")
    return allowed
}()

static func escapePathSegment(_ segment: String) -> String {
    segment.addingPercentEncoding(withAllowedCharacters: roomIdPathAllowed) ?? segment
}
```

`leaveRoom(roomId:)` 同模式 escape。CreateRoom path 不含 roomId（POST /rooms 由 server 返回 roomId），不受影响。

回归测试 `RoomEndpointsTests`：覆盖 `/`、`?`、`#`、`..`、纯数字、空字符串等 6 个 case，断言 escape 行为正确。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在把任何 user-provided / 未知来源的字符串插入 URL path 时，**必须**用 `addingPercentEncoding(withAllowedCharacters: <urlPathAllowed - "/?#">)` 做 percent-encoding —— **禁止**直接 string interpolate `"\(value)"` 进 URL path / query / fragment。
>
> **展开**：
> - "client 不做业务校验" ≠ "client 不做传输层 escape"；URL 编码是传输协议要求，不是业务校验。
> - `CharacterSet.urlPathAllowed` 默认包含 `/`（path separator），需要显式 subtract；`?` `#` 默认不在该集合内，自动会被 escape。
> - 同样原则适用于 query value / header value / form-data field：每个 URL component 有自己的 allowed character set（`urlQueryAllowed` / `urlHostAllowed` / etc.）。
> - server 的业务校验（如"房间号格式不合法"1002）只在 raw input **完整无损**到达 server 时才能正确触发；client 把 input 编码错位会让 server 永远看不到原文。
> - **反例**：`path: "/api/v1/rooms/\(roomId)/join"` 把任意 user input 直插 path；**正例**：`path: "/api/v1/rooms/\(escapePathSegment(roomId))/join"` 显式 escape。
> - **反例 2**：用 `URLComponents` 但只 set `path = "/rooms/" + roomId` —— `URLComponents` 不会自动 escape path components 内部的 reserved 字符；正例 2：用 `URLComponents.percentEncodedPath` 或自己显式 `addingPercentEncoding`。
> - 即使输入"应该"是数字（如 BIGINT roomId），也要 escape ——"应该"不等于"实际"；防御性 escape 没有运行成本（纯数字 path 通过 `addingPercentEncoding` 后字符无变化），但能挡住 caller bug + proxy 改写 + 攻击 input。

---

## Meta: 本次 review 的宏观教训（可选）

两条 finding 表面无关（一条 error-handling、一条 security），但根因都是**"对失败路径的乐观假定"**：
- Lesson 1：假定 `connect(roomId:)` 不会同步抛错 → `try?` 吞错；
- Lesson 2：假定 `roomId` 不含 reserved 字符 → string interpolate 直插。

未来 Claude 写 "I/O 调用 + 字符串拼接进 URL/SQL/path" 这类代码时，应该**默认执行三步检查**：
1. 这个 I/O 调用的所有同步 throw case 我都覆盖了吗？（特别是 token / URL / 编码 / DNS 等 fail-fast 场景）
2. 我用 try? 是因为"真的不关心错误"，还是因为"我懒得想错误"？后者必须改 do/catch。
3. 拼接进 URL / path / SQL 的字符串变量，我做了对应层级的 escape（percent / SQL bind / shell quote）吗？

这三步检查在 review 时也是 reviewer 的高频抓手；提前自检能省下一轮 round-trip。
