# Story 0.1: 项目骨架与开发工具链

Status: done

## Story

As a maintainer,
I want to establish the repo skeleton and dev toolchain per backend-architecture-guide §3,
so that every subsequent story lands on the same foundation and `bash scripts/build.sh --test` gates every PR.

## Acceptance Criteria

1. **Given** repo contains only `scripts/build.sh` and `docs/backend-architecture-guide.md` with no Go source, **When** directory structure is created per constitution §3, **Then** `server/go.mod` exists with module path `github.com/huing/cat/server`.

2. **Given** Story 0.1 skeleton setup, **When** directory tree is created under `server/`, **Then** these directories exist:
   - `cmd/cat/` (entrypoint)
   - `internal/{config,domain,service,repository,handler,dto,middleware,ws,cron,push}` (business layers)
   - `pkg/{logx,mongox,redisx,jwtx,ids,fsm,clockx}` (reusable packages)
   - Each directory has at minimum one `.go` placeholder or `.gitkeep`

3. **Given** the directory structure from AC#2, **When** config directory is initialized, **Then** `config/default.toml` and `config/local.toml.example` exist with field skeleton (no concrete values).

4. **Given** repo skeleton, **When** deployment configuration is created, **Then**:
   - `deploy/docker-compose.yml` starts 2 services: MongoDB (1-node replica set), Redis (local dev infra only, no app container)

5. **Given** root directory, **When** `.golangci.yml` is created, **Then** these linters are enabled: `errcheck`, `errorlint`, `forbidigo`, `gocritic`, `revive`, `unconvert`, `unparam`, `misspell`, `bodyclose`, `gofmt`, `goimports`, `govet`, `unused`, `ineffassign`. `forbidigo` blocks `^fmt\.(Printf|Println)$` and `^log\.(Print|Println|Printf)$`.

6. **Given** root directory, **When** auxiliary configs are created, **Then**:
   - `.editorconfig` exists (tab indentation)
   - `.gitignore` excludes `build/`, `*.out`, `.env.local`, IDE dirs
   - `.dockerignore` excludes `.git`, `build/`, `*.md`, `*_test.go`

7. **Given** repo structure, **When** `Makefile` is created, **Then** targets exist: `build`, `test`, `lint`, `docker-up`, `docker-down`, `ci`.

8. **Given** GitHub Actions setup, **When** `.github/workflows/ci.yml` is created, **Then** steps run sequentially: `golangci-lint run` → `bash scripts/build.sh` → `bash scripts/build.sh --test` → `bash scripts/build.sh --race --test`.

9. **Given** local dev setup, **When** pre-commit hooks are configured, **Then** `gofmt`, `goimports`, `go vet` run before each commit.

10. **Given** Story 0.1 complete, **When** `bash scripts/build.sh` is executed, **Then** `cmd/cat/main.go` exists with placeholder `func main() {}`, build produces `build/catserver`, and `go vet ./...` passes.

11. **Given** Story 0.1 changes committed, **When** CI workflow runs, **Then** CI passes green.

## Tasks / Subtasks

- [x] Task 1: Initialize Go module and directory structure (AC: #1, #2)
  - [x] 1.1 `go mod init github.com/huing/cat/server` in `server/`
  - [x] 1.2 Create `cmd/cat/main.go` with placeholder `package main; func main() {}`
  - [x] 1.3 Create all `internal/` subdirectories with `.gitkeep` or placeholder `.go` files
  - [x] 1.4 Create all `pkg/` subdirectories with `.gitkeep` or placeholder `.go` files
  - [x] 1.5 Create `tools/` and `docs/code-examples/` placeholder directories

- [x] Task 2: Configuration files (AC: #3)
  - [x] 2.1 Create `config/default.toml` with section skeleton: `[server]`, `[log]`, `[mongo]`, `[redis]`, `[jwt]`, `[apns]`, `[cdn]`
  - [x] 2.2 Create `config/local.toml.example` with same structure

- [x] Task 3: Docker & deployment (AC: #4)
  - [x] 3.1 Create `deploy/docker-compose.yml` with MongoDB (replica set) and Redis services (local dev infra)

- [x] Task 4: Linting and editor config (AC: #5, #6)
  - [x] 4.1 Create `.golangci.yml` with all required linters and forbidigo rules
  - [x] 4.2 Create `.editorconfig` (tab indentation for Go)
  - [x] 4.3 Create `.gitignore` (build/, *.out, .env.local, IDE dirs, config/local.toml)
  - [x] 4.4 Create `.dockerignore` (.git, build/, *.md, *_test.go)

- [x] Task 5: Build automation (AC: #7)
  - [x] 5.1 Create `Makefile` with targets: build, test, lint, docker-up, docker-down, ci

- [x] Task 6: CI/CD pipeline (AC: #8)
  - [x] 6.1 Create `.github/workflows/ci.yml` with lint → build → test → race-test sequence

- [x] Task 7: Pre-commit hooks (AC: #9)
  - [x] 7.1 Create lefthook config for gofmt + goimports + go vet

- [x] Task 8: Verification (AC: #10, #11)
  - [x] 8.1 Run `go mod tidy` (go.sum generated only if dependencies exist)
  - [x] 8.2 Run `bash scripts/build.sh` and verify `build/catserver` is produced
  - [x] 8.3 Run `go vet ./...` and verify zero warnings

## Dev Notes

### Architecture Constraints (MANDATORY)

- **Constitution**: `docs/backend-architecture-guide.md` is the authoritative reference. All directory names, package names, and conventions must match exactly.
- **Module path**: `github.com/huing/cat/server`
- **Go version**: 1.24+
- **No business logic**: `main.go` is placeholder only — `func main() {}`. No HTTP server, no DB connections. That's Story 0.2+.
- **No concrete config values**: TOML files contain section/key skeleton only, not real connection strings or secrets.

### Tech Stack (DO NOT deviate)

| Component | Library | Version |
|-----------|---------|---------|
| HTTP | `github.com/gin-gonic/gin` | latest |
| WebSocket | `github.com/gorilla/websocket` | latest |
| MongoDB | `go.mongodb.org/mongo-driver/v2/mongo` | v2.x |
| Redis | `github.com/redis/go-redis/v9` | v9.x |
| Config | `github.com/BurntSushi/toml` | latest |
| Logging | `github.com/rs/zerolog` | latest |
| JWT | `github.com/golang-jwt/jwt/v5` | v5.x |
| Validation | `github.com/go-playground/validator/v10` | v10.x |
| APNs | `github.com/sideshow/apns2` | latest |
| Cron | `github.com/robfig/cron/v3` | v3.x |
| IDs | `github.com/google/uuid` | latest |
| Testing | `github.com/stretchr/testify` | latest |
| Testcontainers | `github.com/testcontainers/testcontainers-go` | latest |

**DO NOT use**: GORM, Postgres, golang-migrate, env-based config, glog, protobuf, custom TCP RPC.

### Directory Structure (exact)

```
server/
├── cmd/cat/main.go
├── internal/
│   ├── config/
│   ├── domain/
│   ├── service/
│   ├── repository/
│   ├── handler/
│   ├── dto/
│   ├── middleware/
│   ├── ws/
│   ├── cron/
│   └── push/
├── pkg/
│   ├── logx/
│   ├── mongox/
│   ├── redisx/
│   ├── jwtx/
│   ├── ids/
│   ├── fsm/
│   └── clockx/
├── config/
│   ├── default.toml
│   └── local.toml.example
├── deploy/
│   └── docker-compose.yml
├── tools/
├── docs/
│   └── code-examples/
├── go.mod
├── Makefile
├── .golangci.yml
├── .editorconfig
├── .gitignore
├── .dockerignore
└── .github/workflows/ci.yml
```

### Package Naming Rules

- Singular lowercase short words: `user`, `blindbox`, `ws`
- NO plural package names (`services` ❌, `userManagement` ❌)
- `internal/` package name = directory name

### Naming Conventions

- Repository interfaces: `<Entity>Repository` (e.g., `UserRepository`)
- Single-method interfaces: `-er` suffix (e.g., `Broadcaster`)
- Constructors: `NewXxx(deps...) *Xxx` or `MustXxx(deps...) *Xxx` (startup-fatal)
- Typed IDs in `pkg/ids/`: `UserID`, `SkinID`, etc. — no raw `string` for IDs
- JSON: camelCase; BSON: snake_case; DTO fields have both tags

### Forbidden Patterns (enforced by linter + CI)

- `fmt.Printf` / `fmt.Println` in business code → use zerolog
- `log.Printf` / `log.Println` → use zerolog
- `func init()` with I/O or external calls
- `sync.Map` / global variables / singletons (except `internal/ws/hub.go`)
- Direct `*mongo.Client` / `*redis.Client` in handler or service

### Docker Compose Specifics

- **MongoDB**: Image `mongo:7.0`, 1-node replica set (required for Change Streams + transactions), port 27017, persistent volume
- **Redis**: Image `redis:7-alpine`, port 6379
- docker-compose.yml 仅提供本地开发基础设施，不包含 App 容器

### .golangci.yml Specifics

```yaml
linters:
  enable:
    - errcheck
    - errorlint
    - forbidigo
    - gocritic
    - revive
    - unconvert
    - unparam
    - misspell
    - bodyclose
    - gofmt
    - goimports
    - govet
    - unused
    - ineffassign

linters-settings:
  forbidigo:
    forbid:
      - pattern: '^fmt\.(Printf|Println)$'
      - pattern: '^log\.(Print|Println|Printf)$'
```

### default.toml Structure

```toml
[server]
host = ""
port = 0
tls = false

[log]
level = ""
output = ""

[mongo]
uri = ""
db = ""

[redis]
addr = ""
db = 0

[jwt]
secret = ""
issuer = ""
expiry = 0

[apns]
key_id = ""
team_id = ""
bundle_id = ""
key_path = ""

[cdn]
base_url = ""
```

### CI Pipeline (.github/workflows/ci.yml)

Sequential steps on push and PR:
1. Setup Go 1.24
2. Install golangci-lint
3. `golangci-lint run`
4. `bash scripts/build.sh`
5. `bash scripts/build.sh --test`
6. `bash scripts/build.sh --race --test`

### Pre-existing File

- `scripts/build.sh` already exists at `C:\fork\cat\scripts\build.sh` (NOT inside `server/`). It expects `server/cmd/cat/` as the build target and outputs to `build/catserver`. Do NOT recreate or modify this file.

### What This Story Does NOT Do

- No HTTP server startup (Story 0.2)
- No database connections (Story 0.3)
- No structured logging implementation (Story 0.5)
- No AppError implementation (Story 0.6)
- No runtime functionality — skeleton and toolchain only
- No `go get` for runtime dependencies beyond what `go mod tidy` pulls for the placeholder main.go

### Project Structure Notes

- Alignment with `docs/backend-architecture-guide.md` §3 directory layout: exact match required
- Multi-replica code-level invariants must be respected from the start (no global state)
- `config/local.toml` should be in `.gitignore` (not committed), only `local.toml.example` is committed

### References

- [Source: docs/backend-architecture-guide.md §3] — Directory structure constitution
- [Source: docs/backend-architecture-guide.md §4-5] — Application entry point (Story 0.2)
- [Source: _bmad-output/planning-artifacts/epics.md, Epic 0, Story 0.1] — Full acceptance criteria and technical requirements
- [Source: _bmad-output/planning-artifacts/architecture.md] — Architecture decisions and stack choices
- [Source: _bmad-output/planning-artifacts/prd.md] — Product requirements and NFR targets

## Dev Agent Record

### Agent Model Used

Claude Opus 4.6 (1M context)

### Debug Log References

### Completion Notes List

- Go module initialized: `github.com/huing/cat/server` (Go 1.24.2)
- All 10 internal packages created with doc.go placeholders: config, domain, service, repository, handler, dto, middleware, ws, cron, push
- All 7 pkg packages created with doc.go placeholders: logx, mongox, redisx, jwtx, ids, fsm, clockx
- docker-compose.yml: MongoDB 7.0 with 1-node replica set (healthcheck auto-initializes rs0), Redis 7-alpine (local dev infra only, no app container)
- .golangci.yml: all 15 linters enabled, forbidigo blocks fmt.Printf/Println and log.Print/Println/Printf
- CI pipeline: golangci-lint-action v6 + build.sh sequential steps
- Pre-commit: lefthook config for gofmt, goimports, go vet
- `bash scripts/build.sh` passes, produces build/catserver (1.5MB)
- `bash scripts/build.sh --test` passes (all packages report [no test files] which is expected for skeleton)
- `go vet ./...` passes with zero warnings

### Change Log

- 2026-04-17: Story 0.1 implemented — project skeleton, toolchain, CI/CD, Docker dev infra
- 2026-04-17: Code review fixes — removed Dockerfile/App container (no Docker build need), fixed lefthook glob to **/*.go, updated Go version spec to 1.24, removed go.sum from mandatory list

### File List

- server/go.mod (new)
- server/cmd/cat/main.go (new)
- server/internal/config/doc.go (new)
- server/internal/domain/doc.go (new)
- server/internal/service/doc.go (new)
- server/internal/repository/doc.go (new)
- server/internal/handler/doc.go (new)
- server/internal/dto/doc.go (new)
- server/internal/middleware/doc.go (new)
- server/internal/ws/doc.go (new)
- server/internal/cron/doc.go (new)
- server/internal/push/doc.go (new)
- server/pkg/logx/doc.go (new)
- server/pkg/mongox/doc.go (new)
- server/pkg/redisx/doc.go (new)
- server/pkg/jwtx/doc.go (new)
- server/pkg/ids/doc.go (new)
- server/pkg/fsm/doc.go (new)
- server/pkg/clockx/doc.go (new)
- server/config/default.toml (new)
- server/config/local.toml.example (new)
- server/deploy/docker-compose.yml (new)
- server/.golangci.yml (new)
- server/.editorconfig (new)
- server/.gitignore (new)
- server/.dockerignore (new)
- server/.lefthook.yml (new)
- server/Makefile (new)
- server/tools/.gitkeep (new)
- server/docs/code-examples/.gitkeep (new)
- .github/workflows/ci.yml (new)
