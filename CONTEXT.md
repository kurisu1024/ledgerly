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
`slog.Handler` in Go), evaluates trigger rules **locally**, and ships only triggered
events into the existing async ingest path. Rules are defined server-side and fetched
by SDKs (ADR-0002). Guardrails from the 2026-07-03 grilling session:

- **No cold-start evidence gap:** the SDK constructor requires a compiled-in fallback
  rule-set, used until the first successful rules fetch; fallback usage is itself
  recorded in the chain.
- **Pre-chain loss is detectable:** every event carries a per-SDK-instance ID and
  monotonic sequence number inside the hashed payload (day one — retrofitting would be
  a chain-format change). The chain evidences its own gaps.
- **The control plane audits itself:** every server-observed state change — rule
  mutations, tenant config, admin actions, auth outcomes — is chained automatically via
  server-enforced baseline triggers that tenant rules cannot disable. Tenant rules are
  purely additive on top of this floor.

## Non-goals

- **Not a log-management system.** The test for any proposed feature or UI surface —
  including ones the PM agent hasn't imagined yet: **if the question is "prove it," it
  belongs in Ledgerly; if the question is "explore it," it belongs in a SIEM we export
  to.** In-bounds: viewing chains, verification status, gap/heal history, rules
  management, export slicing. Out-of-bounds forever: full-text search over payloads,
  aggregation dashboards, alerting on log contents. Exploration pressure is answered by
  making the SIEM export path excellent, not by growing our own Kibana.
- **Not an APM or error tracker.** We borrow Sentry's *integration shape*, not its job.
- **No billing/metering** until the SaaS decision is actually made.
- **No server-side trigger evaluation over raw log streams** (ADR-0001) — non-audit
  logs never leave the host app.

## Phasing

**Phase A — dogfood-complete.** Ledgerly audits itself: API, frontend, and CLI emit
their own audit events through the Go SDK, triggers configured via the rules API,
chains verifiable end to end. Order: scaffolds (#15/#16) → rules API → **minimal
Postgres wiring** (implement the existing `Storage` interface against pgx and
un-hardcode `memory.New()` — dogfood evidence must survive restarts; operability
polish stays in B) → Go SDK → dogfood wiring → point-in-time export (#20) and verify
endpoint (#23, after the VerifyChain last-event gap is fixed) → security events as a
server-enforced baseline trigger (#19).

Dogfood guards: the SDK's own internal logging is categorically untriggerable
(self-suppressed, in every SDK, forever), and the dogfood deployment logs to a
dedicated internal tenant (`ledgerly-self`).

**Phase B — adoption-ready OSS release.** Postgres operability polish (migrations,
docker-compose, tuning), JS/TS SDK + shared conformance suite, docs that sell the
story, outage-recovery tooling (drain SDK disk buffers, ingest stragglers as
append-only late arrivals with gap-closure events — never retroactive insertion), then
the demoted backlog: Merkle inclusion proofs (#17, built on
`github.com/kurisu1024/merkle`), SIEM export formats (#21), retention policies (#22).

Success for A: the log → trigger → chain → verify loop is real and demo-able on our own
stack. Success for B: someone we don't know runs it.

## Invariants (restated, non-negotiable)

- Tenant ID comes exclusively from the JWT; cross-tenant reads/writes are the
  unforgivable bug.
- The hash-chain format is compatibility-critical: changing hashed fields or hash order
  breaks verification of existing chains. Additive structures only.
- The write path stays async: 202 before persistence is the contract.
