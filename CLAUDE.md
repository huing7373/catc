# CLAUDE.md

## Architecture Guide (MANDATORY)

**Before writing any server code, read `docs/backend-architecture-guide.md`.** It defines the architecture, tech stack (Go + Gin + MongoDB + Redis + zerolog + TOML), layering rules, error/logging/config conventions, and the PR checklist every new change must pass. All server code must conform — rewrite existing code if it conflicts.

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
