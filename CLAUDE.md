# CLAUDE.md

## Architecture Guide (MANDATORY)

**Before writing any server code, read `docs/backend-architecture-guide.md`.** It defines the architecture, tech stack (Go + Gin + MongoDB + Redis + zerolog + TOML), layering rules, error/logging/config conventions, and the PR checklist every new change must pass. All server code must conform — rewrite existing code if it conflicts.

**In particular, §21 "Epic 0 → Epic 1+ 继承的工程纪律" is binding for every Epic 1+ story.** Before creating or implementing a story, check which subsections apply:
- §21.1 双 gate 漂移守门 — when adding global constant sets (error codes / WS msg types / feature flags / Redis key prefixes)
- §21.2 Empty/Noop Provider 逐步填实 — when adding or replacing Providers
- §21.3 fail-closed vs fail-open — when handling external system failure (must document choice + observable signal in Dev Notes)
- §21.4 语义正确性 AC review 早启 — for tool / metric / guard / measurement stories (run AC review BEFORE implementation)
- §21.5 tools/* CLI 上线判据 — when adding new CLI tools
- §21.6 Spike / 真机 / 人工执行类工作归 Epic 9 — never block business epic critical path on physical / hardware work
- §21.7 server 测试自包含 — no test may require running iOS / watchOS app; all tests pass via `go test`
- §21.8 §19 PR checklist 语义正确性思考题 — every PR must answer "who gets misled if this code runs and produces a wrong result but doesn't crash?"

§22 提供快速指引：每次开 session 先读 CLAUDE.md → 架构指南 §21-§22 → sprint-status.yaml → 最新 epic retro → MEMORY.md。

**写 server 代码前额外读 `server/agent-experience/review-antipatterns.md`。** 这是 Epic 0 十九轮 code review 蒸馏出的战术级反模式清单（并发安全 / context 生命周期 / JWT 安全边界 / 配置 fail-fast / 注册表自证 / release-vs-debug gate / redis key injectivity / 滑动窗口 / 中间件顺序 / 度量语义等），开头有 TL;DR 自检清单。发现新反模式时同步更新：原始记录进 `code-review-log.md`，蒸馏条目进 `review-antipatterns.md`。

## Pending Cross-Repo Action Items (待处理跨仓需求)

**⚠️ Server 团队请在下次 epic 规划前处理**：

### 2026-04-20 · iPhone UX Step 10 Party Mode → Server 4 个新需求

iPhone UX 设计规范在 Step 10 User Journey Flows 评审中产生 4 个 server 契约新需求（S-SRV-15~18），以及架构哲学升级（**哲学 B · Server 为主，WC 为辅**）。

**详细技术契约文档**：`ios/CatPhone/_bmad-output/planning-artifacts/server-handoff-ux-step10-2026-04-20.md`

**核心新需求概览**：
- **S-SRV-15**：新建 `user_milestones` collection + API（承载账号级里程碑，替代客户端 UserDefaults）
- **S-SRV-16**：`box.state` 新增 `unlocked_pending_reveal` 态（Watch 不可达时延迟揭晓）
- **S-SRV-17**：取消 `emote.delivered` 发送者 ack 推送（fire-and-forget 硬约束）
- **S-SRV-18**：所有 fail 节点 Prometheus metric 打点（详见清单）

**建议行动**：
1. 阅读 handoff 文档
2. 跨仓 sync 会议对齐契约（建议 60 min）
3. server PRD 追加 S-SRV-15~18（或链接 iPhone PRD）
4. 排期到合适的 server epic（建议 `UserMilestones` 先行，是 iOS Epic 1/2 硬依赖）

**iPhone 端状态**：UX 规范 v0.3 完稿（`ios/CatPhone/_bmad-output/planning-artifacts/ux-design-specification.md`），iPhone PRD 已追加 v2 修订注记。

---

## Repo Separation (三端独立)

三个独立目录：`server/` (Go) / `app/` (iOS) / `watch/` (watchOS)。server repo **不**引用 APP/watch，也不被它们引用。跨端契约通过 `docs/api/` 下的文档同步（e.g. `ws-message-registry.md` / `integration-mvp-client-guide.md` / `openapi.yaml`），不通过共享代码。真机联调类工作归 Epic 9，不塞业务 epic。

## Build & Test

Server is a Go project under `server/`. Use the build script:

```bash
# Compile only (vet + build)
bash scripts/build.sh

# Compile + run all tests
bash scripts/build.sh --test

# With race detector
bash scripts/build.sh --race --test
```

Binary output: `build/catserver`

After writing or modifying Go code, run `bash scripts/build.sh --test` to verify compilation and tests pass. Read the output to check for errors.

## Project Structure (target, per architecture guide)

- `server/cmd/cat/` — entry point, initialize, app lifecycle
- `server/internal/` — application code (config, domain, service, repository, handler, dto, middleware, ws, cron, push)
- `server/pkg/` — reusable libraries (logx, mongox, redisx, jwtx, ids, fsm)
- `server/config/` — TOML configs (default/production/local)
- `server/tools/` — one-off data scripts
- `server/deploy/` — Dockerfile, docker-compose
- `scripts/` — build scripts

Note: Story 2-1 landed a Postgres+GORM+env prototype. Per user decision, it will be rewritten to match the architecture guide (MongoDB + TOML + full P2-style layering).
