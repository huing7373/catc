# CLAUDE.md

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

## Project Structure

- `server/cmd/server/main.go` — entry point
- `server/internal/` — application code (config, handler, middleware, model, repository, ws, dto)
- `server/pkg/` — shared libraries (jwt, redis)
- `server/migrations/` — SQL migration files
- `server/deploy/` — Dockerfile, docker-compose
- `scripts/` — build and utility scripts
