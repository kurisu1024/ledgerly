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
  event store in Phase B.
