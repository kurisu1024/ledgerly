# ledgerly

**Append-only audit logs for modern SaaS.**

Ledgerly is a developer-first API that provides **immutable, cryptographically verifiable audit logs** — without building and maintaining the infrastructure yourself. Self-hosted first, SaaS-compatible by design (ADR-0003).

---

## Why Ledgerly?

Most audit logs are:

- Editable
- Incomplete
- Hard to verify
- Built as an afterthought

Ledgerly is different — and it's no longer a pitch. The chain format, the async write path, the verification math, the SDK, and the CLI are **built, tested, and hash-locked**.

---

## What Ledgerly Guarantees

✅ Append-only storage — events land in per-tenant, SHA-256 hash-chained blocks
✅ Cryptographic hash chaining — deterministic hashing from a fixed genesis; the format is frozen because your old chains must verify forever
✅ Independent integrity verification — verify exports client-side with the CLI or in the browser; no server trust required
✅ Point-in-time proof — export **as-of any timestamp** and get a verifiable prefix cut of every chain
✅ Tenant isolation — tenant identity comes from the JWT (RS256-verified), never from the request body
✅ API-first design — kebab-case JSON, boring HTTP, a Go SDK that rides `log/slog`

No dashboards you don't need.
No SDK lock-in.
Just **proof**.

---

## Quick Start

```bash
make run           # serves on :8080 (in-memory storage)
make load-events   # POSTs 5 events with a dev JWT, saves the tenant ID
make export-events # pulls /v1/export for that tenant → export.json
```

For a durable backend, point `LEDGERLY_POSTGRES_DSN` at Postgres (see below). For production auth, configure the RS256 public key — signature verification is on unless you explicitly opt into dev mode (`AllowUnverifiedJWT`).

---

## Example Usage

Record an event (tenant comes from your JWT — a `tenant-id` in the body is overwritten, by design):

```bash
curl -X POST http://localhost:8080/v1/events \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{
    "occurred-at": "2026-07-08T18:32:11Z",
    "action": "project.delete",
    "actor": { "id": "user_123", "type": "user" },
    "resource": { "type": "project", "id": "proj_456" },
    "metadata": { "reason": "user request" }
  }'
```

Response — **202 Accepted**: the event is echoed back and queued. Persistence is asynchronous (a batching worker chains and flushes it); an immediate read may not see it yet. That's the write contract, stated honestly, not a bug.

Export everything — or the world **as it stood at a moment in time**:

```bash
curl -H "Authorization: Bearer $JWT" "http://localhost:8080/v1/export"
curl -H "Authorization: Bearer $JWT" \
  "http://localhost:8080/v1/export?as-of=2026-07-01T00:00:00Z"
```

The as-of export cuts every chain in append order at that timestamp and the result still verifies — a point-in-time proof you can hand an auditor.

Verify an export without trusting the server:

```bash
make build-cli
bin/ledgerly-cli verify export.json   # recomputes the hash chain locally
```

---

## The Go SDK — audit logging that rides your logger

The SDK (`sdk/go`, a zero-server-dependency module) is a `log/slog` handler: it tees to your existing handler, evaluates a locally cached, server-pushed trigger rule-set against every record, and ships matches into the async ingest path. Your app logs like it always has; the audit trail assembles itself.

```go
fallback := []ledgerly.Rule{
    {SchemaVersion: ledgerly.SchemaVersion, EventType: "project.delete", LevelAtLeast: "warn"},
}
h, err := ledgerly.NewHandler(fallback, slog.Default().Handler(),
    ledgerly.WithBufferDir("/var/lib/myapp/ledgerly"),
    ledgerly.WithEventsURL("https://ledgerly.example.com/v1/events"),
    ledgerly.WithRulesURL("https://ledgerly.example.com/v1/rules"),
    ledgerly.WithAPIKey(os.Getenv("LEDGERLY_API_KEY")),
)
if err != nil {
    log.Fatal(err)
}
defer h.Close(ctx)
slog.SetDefault(slog.New(h))
```

Rules are managed server-side (`/v1/rules`) and can only **widen** what the SDK captures, never what your app prints. See [`sdk/go/README.md`](sdk/go/README.md) and ADR-0001.

---

## API Surface

- `POST /v1/events` — record an audit event (202, async; bodies capped at 1 MiB → 413)
- `GET /v1/export` — export a tenant's chains; `?as-of=<RFC3339>` for a point-in-time prefix cut, `?blockID=<uuid>` for one block
- `GET|POST /v1/rules`, `GET|PUT|DELETE /v1/rules/{id}` — server-side SDK trigger rules (mutations serialized per tenant)

Every endpoint is JWT-authenticated and tenant-scoped. Verification is client-side today (CLI + web) — chains verify prefix-complete; detecting tail truncation needs an external anchor, and that's documented, not hidden.

A read-only web inspection UI (paste a JWT, view and verify chains client-side, forensic-ledger styling) lives in [`web/`](web/README.md).

---

## Running with Postgres

Set `LEDGERLY_POSTGRES_DSN` to run against a durable backend (apply the schema first with `psql "$DSN" -f db/schema.sql`); the Postgres integration tests run via `make test-postgres`. Unset, the server uses in-memory storage. Chains survive restarts — events are stored as authoritative JSONB precisely so nanosecond timestamps round-trip and old chains keep verifying.

**Warning:** never point `LEDGERLY_TEST_DSN` at a database you care about — the test harness creates **and drops** schemas in the target database. Use a disposable container (e.g. `postgres:17-alpine`).

---

## How This Repo Is Built (yes, really)

Ledgerly eats its own compliance cooking. The development process is an **agent pipeline with platform-enforced gates**, and we're thrilled about it:

- **Tests-first, enforced not trusted** — failing tests exist before implementation; the implementer's brief is "make these pass" (`docs/agents/team-roster.md`).
- **`main` is protected** — `test` and `test-postgres` are required CI checks (`.github/workflows/ci.yml`); nobody, human or agent, merges past red.
- **Every agent dispatch is logged and audited** — a dispatch log plus a skill-invocation telemetry hook mean process compliance is *verified against ground truth*, not self-reported. Proof over testimony — the same principle as the product.
- **Reviews gate merges** — language reviewers per surface, plus a mandatory security review on any diff touching auth, tenant scoping, or chain hashing.

An audit-log product whose own development leaves a verifiable audit trail. That symmetry is the point.

---

## Who Ledgerly is For

- B2B SaaS teams
- Fintech startups
- Healthtech applications
- Any team preparing for SOC 2, ISO, HIPAA, or compliance audits

---

## Ledgerly is Infrastructure for Trust

Ledgerly succeeds when it's **boring to use**, **easy to trust**, and **hard to replace**.

It's not a logging platform. It's **proof you can show an auditor**.
