# server/CLAUDE.md

## MANDATORY: Read the architecture guide first

**Before writing, editing, or reviewing any Go code in this directory, read `../docs/backend-architecture-guide.md` in full.** It is the binding specification — not a suggestion. Non-compliant code must be rewritten, not merged.

If you (or any subagent you dispatch) are about to touch `server/**/*.go`, `server/go.mod`, `server/config/`, or `server/migrations/`, the guide MUST be loaded into your context first.

## Guide ToC (for quick navigation)

- §1 Philosophy · §2 Tech stack (Gin + MongoDB + Redis + zerolog + TOML)
- §3 Directory structure · §4 Initialize pattern · §5 Runnable + graceful shutdown
- §6 Layering (handler → service → repository → domain)
- §7 AppError · §8 Structured logging · §9 TOML config
- §10 MongoDB · §11 Redis · §12 WebSocket · §13 Middleware · §14 API conventions
- §15 Code style (imports, naming, typed IDs, comments, context)
- §16 Testing · §17 Cron/push · §18 P2 smells NOT to copy · §19 PR checklist · §20 Project constraints

## Top 10 rules you will violate most often if you skim

1. No `*gorm.DB`, no Postgres, no env-based config — all will be replaced. The stack is **MongoDB + TOML**.
2. Handler never holds `*mongo.Client` / `*redis.Client`; it depends on a service interface.
3. Constructor returns `*Struct`; interfaces live in the **consumer** package (service, handler).
4. All I/O functions take `ctx context.Context` as first arg.
5. Errors via sentinel + `AppError`; never `strings.Contains(err.Error(), ...)`.
6. Logs via zerolog `log.Ctx(ctx)`; never `fmt.Printf` / `log.Printf` / `log.Fatalf` (use `log.Fatal().Err(err)`).
7. IDs are typed (`ids.UserID`, not `string`).
8. Redis keys come from `redis_keys.go` functions; no string literals at call sites.
9. Every Redis `Set` has an explicit TTL.
10. Comments in English. No `// TODO` without an issue number.

## Rewrite status

Story 2-1a landed: this directory is now on MongoDB + TOML + P2 layering per the architecture guide. Story 2-1's Postgres/GORM prototype has been removed in full. Epic 2-2 (Sign in with Apple) and later stories build on the new foundation.

## Build & Test (reminder)

```bash
bash scripts/build.sh              # compile
bash scripts/build.sh --test       # compile + tests
bash scripts/build.sh --race --test
```

Always run `bash scripts/build.sh --test` after edits and read the output.
