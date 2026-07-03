# ledgerly-web

A read-only inspection surface for Ledgerly's audit log: paste a dev JWT, see
every event grouped into its hash-chained blocks, and verify each chain
client-side against the same hashing rules the Go server uses
(`internal/audit/events.go`).

Zero-framework-bloat by design: Vite + React + TypeScript, no router, no
state library, no CSS framework. Verification runs entirely in the browser
via Web Crypto (`crypto.subtle`) — nothing is sent anywhere to check a hash.

## Dev flow

```bash
# terminal 1 — the Go API, in-memory storage, serves on :8080
make run

# terminal 2 — post a sample batch of events for a fresh tenant
make load-events

# terminal 3 — the web app, proxies /v1 -> localhost:8080 (see vite.config.ts)
npm --prefix web run dev
```

Open the printed Vite URL, paste the `JWT Token: ...` value that
`make load-events` printed, and hit **Continue**. The token is decoded
client-side only to read its `tenant_id` claim for display — the server
remains the sole authority on whether the token is actually valid; a bad
token still shows the paste form with a real 401 once you try to fetch.

The token is kept in `sessionStorage` (never `localStorage`): cleared when
the tab closes, not shared across tabs, and a smaller XSS blast radius for a
bearer token.

## The 202 contract

`POST /v1/events` returns **202 Accepted before anything is persisted or
chained** — the server enqueues the event and a background worker batches it
into the tenant's hash chain later (on a size threshold, a flush interval,
or shutdown). This client's `postEvent` reflects that honestly: it resolves
to `{ state: "accepted" }` and nothing stronger. There is no "persisted"
variant, because the API never reports one synchronously.

Practically: if you hit **Refresh** in the viewer immediately after posting
new events, you may not see them yet. That's not a bug in either side —
it's the asynchronous write path working as designed.

## Verification, and its one honest limitation

`lib/verify/verifyChain.ts` mirrors `internal/audit/events.go`'s
`VerifyChain` byte-for-byte: same genesis hash, same per-event hash inputs,
same linkage check. A "verified" badge always carries this caveat in its
own text, not just in this README:

> Verification cannot detect **tail truncation** — a valid prefix of a
> longer chain still verifies as complete. Detecting a silently-truncated
> tail requires an external anchor (e.g. storage recording the expected
> head hash) that neither the Go server nor this client has today.

When a chain fails, the viewer marks the exact row where verification broke
(`data-testid="chain-rupture"`) and the badge names *why*: `hash-mismatch`
(the event's own content was altered), `link-broken` (its `prev-hash`
doesn't point at the prior event), `foreign-chain` (its `chain-id` doesn't
match), or `tenant-mixed` (its `tenant-id` doesn't match the chain's first
event — a case Go's `VerifyChain` collapses into the same check as
`foreign-chain`, but this client reports separately for a clearer forensic
trail).

## Regenerating the golden fixtures

`src/lib/verify/__fixtures__/golden-chains.json` pins this client's hashing
and verification logic to the real Go implementation. It's generated (not
hand-written) by a gated Go test:

```bash
LEDGERLY_GEN_WEB_FIXTURES=1 go test ./internal/audit/... -run TestGenerateWebFixtures
```

Regenerate it whenever `internal/audit/events.go`'s hash inputs or ordering
change, and commit the result — client tests read this fixture directly, so
a stale fixture silently stops testing the real contract.

## Testing

```bash
npm --prefix web run test:run   # vitest, single run
npm --prefix web run test       # vitest, watch mode
npx tsc --noEmit                # from web/, typecheck only (build also runs this)
```

`make test` from the repo root runs the Go suite and this web suite together
(`test: test-go test-web` in the root `Makefile`) — Node is now a hard
requirement for a full green `make test`, which is a deliberate tradeoff,
not an oversight.
