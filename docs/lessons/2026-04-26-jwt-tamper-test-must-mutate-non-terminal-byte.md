---
date: 2026-04-26
source_review: codex review (epic-loop round 4) — /tmp/epic-loop-review-4-4-r4.md
story: 4-4-token-util
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — JWT 篡改测试必须改非末尾字节（base64url padding bits 共享导致末尾字符 flip 可能 decode 出相同字节）

## 背景

Story 4.4 引入 JWT util（`server/internal/pkg/auth/`），其中 `TestVerify_TamperedSignature_ReturnsErrTokenInvalid` 通过"flip token 最后一个字符"模拟签名篡改，但该实现存在 base64url padding bits 共享导致的 nondeterministic flake：约 25% 概率（取决于具体 signature 的最后一个字符 mod 4）tampered token decode 出**相同**的 32 字节 signature，Verify 通过 → 测试失败。codex review round 4 [P1] 指出该问题。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | JWT 篡改测试改末尾字符可能 decode 出相同字节（padding bits 共享） | high | testing | fix | `server/internal/pkg/auth/token_test.go:101-107` |

## Lesson 1: JWT 篡改测试必须改非末尾字节（首选改 payload 段，必然让签名重算 mismatch）

- **Severity**: high
- **Category**: testing（flake → CI 不可信）
- **分诊**: fix
- **位置**: `server/internal/pkg/auth/token_test.go:101-107`

### 症状（Symptom）

`TestVerify_TamperedSignature_ReturnsErrTokenInvalid` 在 `go test ./...` 偶尔失败：`require.Error(t, err)` 期望 Verify 返非 nil error，实际返 nil（即 tampered token 被验证通过）。复现概率取决于每次随机生成的 signature 末尾字符。

### 根因（Root cause）

base64url 编码的字节边界与字符边界**不对齐**，末尾字符的低位 bits 是 padding（decode 时丢弃）：

- HS256 signature 是 32 字节 = **256 bits**
- base64url 每字符 6 bits → 32 字节需要 ⌈256 / 6⌉ = **43 字符 = 258 bits**
- 多出来的 **2 bits** 是 padding，在 decode 时被丢弃
- 因此 base64url 末尾字符的低 2 bits 不影响 decode 出的字节
- charset `A-Z a-z 0-9 - _` 共 64 字符，每 4 个字符（共享高 4 bits）decode 出相同字节
- 末尾字符 `'A' (000000)` flip 到 `'B' (000001)` → 低 2 bits 变了但被丢弃 → decode 出相同 signature → Verify 通过

之所以踩这个坑：写测试时凭直觉假设"改 token 任何一个字符都会破坏签名"，没有意识到 base64url 编码长度与字节长度的 modular mismatch 造成末尾字符 padding bits。这是密码学测试中典型的"abstraction leak"——测试写在 base64url 字符串层（应用层），但被验证的不变量在 raw bytes 层（密码学层），两层之间存在 lossy 编码。

### 修复（Fix）

改为**篡改 payload 段首字符**（远离 padding 边界，且 HMAC 重算必然 mismatch）：

before:
```go
last := tok[len(tok)-1]
swap := byte('A')
if last == 'A' { swap = 'B' }
tampered := tok[:len(tok)-1] + string(swap)
```

after:
```go
firstDot := strings.Index(tok, ".")
require.Greater(t, firstDot, -1, ...)
secondDot := strings.Index(tok[firstDot+1:], ".")
require.Greater(t, secondDot, -1, ...)
payloadStart := firstDot + 1
original := tok[payloadStart]
swap := byte('A')
if original == 'A' { swap = 'B' }
tampered := tok[:payloadStart] + string(swap) + tok[payloadStart+1:]
```

并加详细注释解释**为什么不能改回末尾字符**（防未来人重构时踩同坑）。

注释里点名 lesson 文件路径作为 anchor。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写"篡改密码学输出 + 期望 Verify 失败"的测试** 时，**禁止 mutate 编码字符串的末尾字符**，**必须 mutate header / payload 段或 signature 段中部字节**（远离编码 padding 边界）。
>
> **展开**：
> - **JWT / base64url 场景**：HS256 signature = 32 bytes = 256 bits，base64url = 43 chars = 258 bits（多 2 bits padding）；末尾 base64url 字符的低 2 bits 在 decode 时被丢弃 → 末尾字符 flip 不一定改变 decoded bytes
> - **更广义**：任何 `bytes_per_unit × char_count_per_unit` 不整除（如 base64 = 6 bits/char，base32 = 5 bits/char）的编码方案都有此 padding bits 共享问题；末尾字符的低 `(8 × N) mod (bits_per_char)` bits 是 padding
> - **首选篡改方案（最稳）**：改 token 的 **header 或 payload 段任意字节**。HS256 / RS256 校验时把 `[header.payload]` 拼起来重算 HMAC / 重验 RSA signature，这两段任何 byte 变化都会让签名 mismatch（无 padding bits 共享风险，因为这两段独立 base64url 编码且 Verify 不需要它们 round-trip 回原 bytes，只需要 string-level 一致）
> - **次选方案**：改 signature 段**中部**字节（不是末尾），但需要确认改的不是 padding bits
> - **最差方案**：改 signature 段**末尾**字符 ← 本次踩的坑，禁止
> - **更稳但更繁琐**：base64 decode → flip bit → re-encode（绕过 padding bits 共享，但代码量大）
> - **反例 1**（踩坑）：`tampered := tok[:len(tok)-1] + "B"` —— 概率性失败
> - **反例 2**（不稳）：在 base32 / base64 编码字符串末尾做 char swap，假设"任意字符变化都改字节"
> - **正例**：定位 token 第一个 `.` 之后的位置，flip 那个字节，必然让签名 mismatch
> - **配套要求**：注释中**明确点名** lesson 文件路径作为 anchor，防止未来重构时回归到末尾篡改方案
> - **更通用的元规则**：测试涉及"编码 / 解码 / 哈希 / 加密"等 lossy 或 modular 变换时，**永远不要在编码字符串边缘做扰动**；要么作用在 raw bytes 层（decode → mutate → encode），要么作用在编码字符串中部（远离 padding / boundary）；同时跑 `go test -count=10` 验证 nondeterministic 行为

## Meta: 本次 review 的宏观教训

测试稳定性 / nondeterministic flake 是 review 的高价值发现领域：单次跑通的 test 不等于 flake-free。**任何涉及随机性 / 时序 / 编码边界的新测试，第一时间必须跑 `go test -count=10`（或更高）验证**，不能只看一次绿就 ship。本次 review 的 [P1] 就是 round 3 修完 [P1] 后没跑 count=10，导致 flake 漏到 round 4。
