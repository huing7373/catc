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
