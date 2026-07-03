# ADR-0003: Self-hosted open source first, with SaaS-compatible seams

Date: 2026-07-03
Status: Accepted

## Context

Ledgerly began as a portfolio/learning project. The market moment (AWS QLDB deprecated
July 2025; immudb, Sigstore Rekor, WorkOS/Retraced occupying the verifiable-log niche)
and the project's ambitions point at a real product. Three postures were considered:
self-hosted OSS infrastructure, hosted SaaS, or embeddable library.

## Decision

Self-hosted open source is the primary posture: success means strangers run Ledgerly in
production. The architecture keeps a hosted SaaS path open but does not build for it
yet. Concretely, the SaaS-compatible seams that already exist are preserved as
load-bearing: multi-tenancy with JWT-derived tenant identity, per-tenant chains and
storage scoping, and server-side rules (ADR-0002). SDKs in multiple languages (Go
first, JS/TS second) are the adoption surface.

## Consequences

- Feature priority favors operability for self-hosters: Postgres persistence,
  docker-compose, docs, and verification tooling rank ahead of anything SaaS-only.
- No billing, metering, org-management, or usage tiers until a SaaS decision is made —
  explicitly a non-goal in CONTEXT.md.
- Multi-tenancy stays first-class even though most self-hosters run one tenant; the
  cost is accepted because retrofitting isolation later is far harder than carrying it.
- Phase A (dogfood-complete) precedes Phase B (adoption-ready release): the SDK
  ergonomics are the bet, and we feel them ourselves before writing five SDKs.
