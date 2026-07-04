# ledgerly Go SDK

A `log/slog` handler that tees your application's logging to its existing
handler unconditionally, evaluates a locally cached trigger rule-set
against every eligible record, and ships matches into ledgerly's async
ingest path. See issue #26 and ADR-0001.

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

## Public API

- `NewHandler(fallback []Rule, next slog.Handler, opts ...Option) (*Handler, error)`
  — construction fails outright on an empty fallback rule-set
  (`ErrEmptyFallback`), a nil `next` (`ErrNilNext`), a missing buffer dir
  (`ErrMissingBufferDir`), or an insecure endpoint URL (below).
- `(*Handler).Close(ctx)` — stops polling, drains in-flight delivery
  (bounded by ctx; anything undelivered stays spilled on disk for the next
  instance), and checkpoints the sequence counter.
- `Rule`, `RuleList`, `FieldCondition`, `Match`, `MatchInput` — the trigger
  rule wire types and matcher, byte-for-byte identical to the server's
  `api/rules` (pinned by a server-side parity test).

### Options

| Option | Default | Meaning |
|---|---|---|
| `WithBufferDir(dir)` | required | Disk buffer + sequence state directory |
| `WithEventsURL(url)` | none | `POST /v1/events` endpoint; without it nothing is delivered |
| `WithRulesURL(url)` | none | `GET /v1/rules` endpoint; without it the fallback rules apply for life |
| `WithAPIKey(key)` | none | Bearer token for both endpoints |
| `WithRefreshInterval(d)` | 1m | Rules re-fetch interval |
| `WithHTTPClient(hc)` | `http.DefaultClient` | Client for delivery and polling |
| `WithQueueSize(n)` | 1000 | Bounded delivery queue |
| `WithBackoff(base, max)` | 500ms / 30s | Retry backoff bounds |
| `WithInsecureHTTP()` | off | Allow plain `http://` to non-loopback hosts |
| `WithAsyncFirstFetch()` | off (sync) | First rules fetch off the constructor's critical path |

## HTTPS requirement

`NewHandler` rejects `http://` events/rules URLs to non-loopback hosts:
events carry an API key and sensitive payloads. `localhost` / loopback IPs
are allowed without opt-in (the dev flow); anything else needs an explicit
`WithInsecureHTTP()`.

## Constructor fetch behavior

When a rules URL is configured, `NewHandler` records a chained
`sdk.started` event (regime=fallback) and then performs one synchronous
rules fetch with its own short timeout (2s, tighter than the 5s
steady-state poll timeout) so the server's rules are active the moment
construction returns; a hung endpoint costs at most those 2s and leaves
the fallback active. `WithAsyncFirstFetch()` moves that first fetch onto
the polling goroutine for latency-sensitive callers. The first successful
fetch records a chained `sdk.rules-activated` event.

## Attr conventions

- `event-type` (string) — required for a record to be matcher-eligible;
  records without it are never captured.
- `slog.Group("actor", ...)` / `slog.Group("resource", ...)` — routed into
  the event's actor/resource identity maps; matchable as `actor.*` /
  `resource.*` field conditions.
- `slog.Group("metadata", ...)` — explicit event metadata; loose top-level
  attrs also fold into metadata (explicit metadata wins on key collisions).

### Reserved `ledgerly.` metadata prefix

Metadata keys under the `ledgerly.` prefix belong to the SDK.
`ledgerly.sdk-instance-id` and `ledgerly.sdk-seq` are stamped
unconditionally on every captured event, overwriting any value the
application supplied under those keys (including nested in the metadata
group or bound via `logger.With`), so provenance stamps cannot be spoofed
through slog. `ledgerly.internal` is an informational tag on SDK-internal
diagnostics written to your own handler; it carries **no** capture
semantics — setting it on your own records does not exempt them from
capture.

**Trust-model limitation:** these guarantees hold only for events shipped
through this SDK. A caller POSTing to `/v1/events` directly with a valid
API key can write arbitrary `ledgerly.*` metadata keys; server-side
enforcement is tracked in issue #38.

## Delivery semantics

- **202 contract:** the server replies `202 Accepted` when the event enters
  its async ingest queue — acceptance is not persistence; an immediate read
  may not see the event until the server flushes.
- **At-least-once:** delivery retries (and restart replay) can duplicate an
  event; duplicates share the same instance-id + seq stamps, so consumers
  can deduplicate on them.
- **Gap semantics:** the per-instance `ledgerly.sdk-seq` is monotonic. A
  sequence number is burned *before* the non-blocking enqueue, so a record
  dropped on a full queue leaves a chain-evidenced gap. After an unclean
  shutdown a *false* gap of at most one reservation block (256 numbers) may
  appear; a clean `Close` checkpoints exactly and resumes gap-free.

## Disk buffer

`WithBufferDir` names a directory (created `0700`, files `0600` — treat it
as sensitive: it holds captured event payloads) containing:

- `spill.jsonl` — append-only JSONL of events that failed live delivery,
  replayed in order on retry and on restart with their original stamps.
- `spill.cursor` — fsync'd replay cursor.
- `instance.json` — per-instance identity + clean-close checkpoint.
- `seq.reserve` — fsync'd sequence reservation high-water mark.

The two counters (`spill.cursor`, `seq.reserve`) use checksummed
alternating slots: a torn write can never silently rewind a counter (which
for `seq.reserve` would duplicate sequence numbers); a fully corrupt
counter file fails construction loudly.

### Sequence block size

Sequence numbers are reserved in blocks of 256 (`defaultSeqBlockSize`) so
allocation stays memory-only off the disk. The block size bounds the false
gap after a crash: larger blocks mean fewer fsyncs but a larger possible
false gap for gap-auditing consumers.

Give each application instance its own buffer dir — the sequence chain and
instance identity are per-directory.
