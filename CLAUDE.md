# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Ledgerly is an append-only, cryptographically verifiable audit-log API (multi-tenant, hash-chained events). Go 1.25, stdlib `net/http` (note: gin is in go.mod but the server actually uses `http.ServeMux` — don't assume gin idioms), zap for logging, pgx for the eventual Postgres backend.

## Commands

```bash
make test        # go fmt + go vet + go test -v -p=1 -cover ./...  (the canonical check)
make run         # go run ./cmd/ledgerly/main.go  (serves on :8080, in-memory storage)

# Single test
go test -v -run TestName ./api/http/

# Manual smoke test against a running server (crafts an unsigned dev JWT)
make load-events    # POSTs 5 events, saves tenant ID to .tenant_id
make export-events  # GETs /v1/export for that tenant → export.json
```

Tests run with `-p=1` deliberately — the e2e tests spin up real HTTP servers; keep that flag.

## Architecture

The write path is asynchronous — this is the core design to understand:

```
POST /v1/events
  → authMiddleware (api/http/auth.go): decodes JWT, extracts tenant_id into context
  → CreateEvent (api/http/handlers.go): validates, converts DTO → domain, enqueues
  → buffered channel (queue, non-blocking send; full queue = 503)
  → batchInsertWorker (internal/audit/worker.go): accumulates events into a
    per-tenant hash-chained EventChain; flushes when the chain hits ChainSize
    OR on the FlushInterval ticker OR on shutdown
  → ChainWriter (internal/storage/writer.go) → Storage.WriteBlock
```

Consequence: `POST /v1/events` returns **202 Accepted before anything is persisted**. An immediate read may not see the event until a flush happens.

### Layers

- `api/events/` — wire-format DTOs (string IDs) and conversion to/from the domain type. API JSON keys are **kebab-case** (`tenant-id`, `occurred-at`).
- `api/http/` — server wiring (`http.go`), handlers, JWT auth middleware, request logging middleware. Handlers live on `*T`; routes are registered in `New()`.
- `internal/audit/` — the domain: `Event`, `EventChain`, SHA-256 hash chaining (`events.go`), and the batching worker (`worker.go`). Hashing is deterministic: map fields are written sorted-by-key; chains start from a fixed genesis hash. **Changing any hashed field or the hash order breaks verification of existing chains.**
- `internal/storage/` — `Storage` interface (WriteBlock/FetchBlock/FetchBlocks over `Block`s, always tenant-scoped) plus the memory implementation in `memory/`. Postgres is the intended second backend (`db/db.go`, `db/schema.sql`) but is **not wired into the service yet** — `service.Run` hardcodes `memory.New()`.
- `service/` — composes storage + HTTP handler + graceful shutdown; `cmd/ledgerly/` is signal handling only.

### Multi-tenancy

Tenant ID comes exclusively from the JWT claim, never from the request body (handlers overwrite it). Every storage call is keyed by tenant ID. Preserve this — cross-tenant reads/writes are the one unforgivable bug in an audit-log product.

## Current known gaps (intentional, don't "fix" silently)

- JWT **signature verification is a TODO** — `parseJWT` only decodes the payload. The RSA public key is plumbed through `Config.JWTPublicKey` for when it lands.
- `db/db.go` hardcodes a dev DSN and nothing uses it; `config/config.go` is a stub not read by the service.
- `VerifyChain` skips the last event (`i < len-1`) — check with Chris before relying on it.

## Conventions

- Server structs named `T` within their package (`http.T`, `service.T`) — follow that pattern.
- Immutable domain ops: `AppendEvent(chain, e) EventChain` returns a new value rather than mutating; keep audit-domain functions in that style.
- Errors: wrap with `fmt.Errorf("...: %w", err)`; handlers map errors to plain `http.Error` responses.
- Commit style: short imperative subject lines, no strict conventional-commit prefixes.

## Agent skills

### Issue tracker

Issues live in this repo's GitHub Issues (`kurisu1024/ledgerly`) via the `gh` CLI; external PRs are not a triage surface. Includes the autonomy/greenlight policy for AFK loops. See `docs/agents/issue-tracker.md`.

### Triage labels

The five canonical labels, unmodified: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Team roster

The agent dispatch table for autonomous work — which agent runs each pipeline stage (curate → plan → build tests-first → review → fix → maintain), across the Go API, React frontend (`web/`), and CLI (`cmd/ledgerly-cli/`). See `docs/agents/team-roster.md`.

### Domain docs

Single-context: `CONTEXT.md` + `docs/adr/` at the repo root (created lazily — proceed silently if absent). See `docs/agents/domain.md`.
