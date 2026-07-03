# ADR-0002: Server-side trigger rules; every rule mutation is a chained audit event

Date: 2026-07-03
Status: Accepted

## Context

Trigger rules (ADR-0001) must live somewhere. Client-side-only rules (in app code) are
deterministic and auditor-friendly but require redeploys to change. Server-side rules
(Sentry-style) give tenants runtime control and a real SaaS story, but introduce a
serious weakness for an audit product: a mutable server config controls what evidence
exists. "Who changed the trigger rules, and when" becomes its own audit question.

## Decision

Rules are defined server-side — tenant-scoped CRUD on a rules API (`api/http/rules.go`),
tenant taken from the JWT as everywhere else — and fetched by SDKs on startup and on a
refresh interval with ETag caching. SDKs keep operating on cached rules through server
outages.

The price of mutability is paid in-product: **every rule mutation (create, update,
delete) is itself written as an audit event into the tenant's chain** — who, what, when,
prior rules, new rules. The trigger-rule history is as tamper-evident as the events the
rules capture.

## Consequences

- Tenants change capture behavior at runtime; no app redeploys. This is the first real
  server-side dependency of the SDK and fits the future SaaS tier.
- The evidence-integrity objection to server-side rules is answered structurally: an
  auditor can reconstruct exactly which rules were active when, from the chain itself.
- Gaps remain by design and must be documented honestly: rule propagation is
  eventually-consistent (refresh interval), so the chain records when a rule *changed*,
  not the instant each SDK instance adopted it. SDKs should report their active
  rule-set version on startup as an audit event to narrow this window.
- The rules store is new state to persist; in-memory first, Postgres alongside the
  event store (minimal wiring lands in Phase A per CONTEXT.md).

## Amendments (grilling session, 2026-07-03)

- **Server-enforced baseline triggers.** A tenant admin must not be able to "turn off
  the cameras": a small built-in trigger floor is evaluated server-side and cannot be
  disabled or narrowed by tenant rules — v1 floor: auth failures, rule mutations,
  admin/destructive API actions. Tenant-defined rules are purely additive on top.
  A rogue admin can still silence their own app's logs (client-side evaluation can
  always be bypassed by whoever owns the app), but everything the *server* witnesses
  is captured regardless of the rule-set.
- **Generalized: the control plane audits itself.** Rule-mutation chaining is one
  instance of a broader rule — *any* server-observed state change (rules, tenant
  config, admin actions, auth outcomes) is chained automatically, no rule required, no
  opt-out.
