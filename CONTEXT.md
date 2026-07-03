# Ledgerly — Product Direction

The standing rudder for all work in this repo. Planning agents (deep-plan), the Stage 0
PM research agent, and human contributors judge proposals against this document. When a
task conflicts with it, the task is wrong or this document needs an explicit revision —
never silent drift.

Decided 2026-07-03 in a direction session with Chris. Load-bearing decisions have ADRs
in `docs/adr/`.

## What Ledgerly is

**Sentry for audit trails.** You keep your existing logger; one added handler and a
trigger rule later, the log lines that matter become cryptographically chained,
tamper-evident, independently verifiable audit events.

Self-hosted open-source first — the immudb/Retraced niche, sharpened by AWS QLDB's
deprecation — with the architecture keeping a hosted SaaS path open. Multi-tenancy,
server-side trigger rules, and per-tenant isolation are load-bearing from day one, not
bolted on.

## The differentiator

**Verifiability, not storage.** Plenty of things store logs. Ledgerly's promise is that
an auditor — or a customer of *your* product — can take an export and prove, without
trusting your server, that nothing was altered, backdated, or deleted.

Every feature is judged against one question: does it make the proof story stronger or
adoption cheaper? If neither, it's out.

## Integration model

Sentry-shaped (ADR-0001): the SDK rides along on the app's existing logging (a
`slog.Handler` in Go), evaluates tenant-defined trigger rules **locally**, and ships
only triggered events into the existing async ingest path. Rules are defined
server-side and fetched by SDKs (ADR-0002); every rule mutation is itself a chained
audit event.

## Non-goals

- **Not a log-management system.** No search-all-your-debug-logs, no
  dashboards-over-everything. SIEMs exist; we export to them instead.
- **Not an APM or error tracker.** We borrow Sentry's *integration shape*, not its job.
- **No billing/metering** until the SaaS decision is actually made.
- **No server-side trigger evaluation over raw log streams** (ADR-0001) — non-audit
  logs never leave the host app.

## Phasing

**Phase A — dogfood-complete.** Ledgerly audits itself: API, frontend, and CLI emit
their own audit events through the Go SDK, triggers configured via the rules API,
chains verifiable end to end. Order: scaffolds (#15/#16) → rules API → Go SDK →
dogfood wiring → point-in-time export (#20) and verify endpoint (#23, after the
VerifyChain last-event gap is fixed) → security events as a built-in trigger (#19).

**Phase B — adoption-ready OSS release.** Postgres backend wired for real, JS/TS SDK +
shared conformance suite, docker-compose + docs that sell the story, then the demoted
backlog: Merkle inclusion proofs (#17, built on `github.com/kurisu1024/merkle`), SIEM
export formats (#21), retention policies (#22, unblocked by Postgres).

Success for A: the log → trigger → chain → verify loop is real and demo-able on our own
stack. Success for B: someone we don't know runs it.

## Invariants (restated, non-negotiable)

- Tenant ID comes exclusively from the JWT; cross-tenant reads/writes are the
  unforgivable bug.
- The hash-chain format is compatibility-critical: changing hashed fields or hash order
  breaks verification of existing chains. Additive structures only.
- The write path stays async: 202 before persistence is the contract.
