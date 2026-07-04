# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Ledgerly is an append-only, cryptographically verifiable audit-log API (multi-tenant, hash-chained events). Go 1.25, stdlib `net/http` (note: gin is in go.mod but the server actually uses `http.ServeMux` — don't assume gin idioms), zap for logging, pgx for the Postgres backend.

## Commands

```bash
make test        # go fmt + go vet + go test -v -p=1 -cover ./...  (the canonical check)
make run         # go run ./cmd/ledgerly/main.go  (serves on :8080; in-memory storage,
                 # or Postgres when LEDGERLY_POSTGRES_DSN is set — apply the schema first
                 # with `psql "$DSN" -f db/schema.sql`)
make test-postgres  # Postgres integration tests against a real DB (LEDGERLY_TEST_DSN)

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
- `internal/storage/` — `Storage` interface (WriteBlock/FetchBlock/FetchBlocks over `Block`s, always tenant-scoped) plus two backends: `memory/` (dev default) and `postgres/` (pgx, `db/schema.sql`). Postgres stores each event as authoritative JSONB keyed by (tenant_id, chain_id, position) — a typed TIMESTAMPTZ column would truncate the nanosecond `OccurredAt` and break `VerifyChain` after a restart. `MutateRules` serializes per tenant via `pg_advisory_xact_lock`. Its integration tests skip unless `LEDGERLY_TEST_DSN` is set.
- `service/` — composes storage + HTTP handler + graceful shutdown; picks the backend from `LEDGERLY_POSTGRES_DSN` (set → Postgres, fail-fast on a bad DSN; unset → memory). `cmd/ledgerly/` is signal handling only.
- `internal/cli/` + `cmd/ledgerly-cli/` — the `ledgerly-cli` client (cobra): `token`, `events post`, `export`, `verify`. Command logic lives in `internal/cli` with injectable `Deps{HTTPClient, Now}`; `cmd/ledgerly-cli/main.go` only wires real deps and exits via `cli.Run`. Build with `make build-cli`.
- `sdk/go/` — the Go SDK (issue #26), a **separate Go module** (`github.com/kurisu1024/ledgerly/sdk/go`, zero dependency on the server module) joined to the repo via `go.work`; tested by `make test-sdk` / `cd sdk/go && go test -race ./...`. See `sdk/go/README.md`.

### Multi-tenancy

Tenant ID comes exclusively from the JWT claim, never from the request body (handlers overwrite it). Every storage call is keyed by tenant ID. Preserve this — cross-tenant reads/writes are the one unforgivable bug in an audit-log product.

## Current known gaps (intentional, don't "fix" silently)

- JWT **signature verification is a TODO** — `parseJWT` only decodes the payload. The RSA public key is plumbed through `Config.JWTPublicKey` for when it lands.
- `config/config.go` is a stub not read by the service — backend selection happens directly from the `LEDGERLY_POSTGRES_DSN` env var in `service/service.go`.
- Chain verification is **prefix-complete only**: truncating events from the tail of a chain leaves a valid prefix that still verifies. Detecting tail truncation needs an external anchor (e.g. storage recording the expected head hash). `VerifyChain(chain) bool` delegates to `VerifyChainReport(chain) VerifyResult` (internal/audit/verify.go), the structured API that reports status/reason/failed-index — use that when you need to know *what* failed.

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

### Team roster — MANDATORY routing, not just for loops

**Any feature, bug fix, or refactor in this repo — interactive session or autonomous loop — MUST run through the pipeline in `docs/agents/team-roster.md`.** Read that file before starting substantive work; it names the agent (Agent tool `subagent_type`) for each stage: curate → plan (`deep-plan`) → build tests-first → review → fix → maintain, across the Go API, React frontend (`web/`), and CLI (`cmd/ledgerly-cli/`).

Non-negotiable even when the prompt pulls the other way:

- **Failing tests exist before implementation code.** `ecc:tdd-guide` (or equivalent RED pass) first; the implementer's brief is "make these tests pass."
- **Review before merge**: `ecc:go-reviewer` / `react-reviewer` by surface; **any diff touching auth, JWT, tenant scoping, chain hashing, or the rules API additionally gets `ecc:security-reviewer`**. CRITICAL/HIGH findings block.
- Direct-to-implementation is allowed only for trivial mechanical edits (typos, doc wording, one-line config) — never for anything with a runtime surface.

If you cannot spawn subagents in your context, follow the stages yourself in order and say so; do not skip stages silently.

### Domain docs

Single-context: `CONTEXT.md` + `docs/adr/` at the repo root (created lazily — proceed silently if absent). See `docs/agents/domain.md`.
