# Agent team roster

The dispatch table for autonomous work in this repo. Each pipeline stage names the
agent (Agent tool `subagent_type`) to spawn for it. Governance — which issues may
proceed without a human greenlight — lives in `issue-tracker.md`; this file only says
*who does the work* once an issue is cleared to run.

The repo is a monorepo: Go API (existing), React frontend under `web/`, CLI under
`cmd/ledgerly-cli/` (cobra). Route by the surface an issue touches.

## Pipeline stages

### 0. Discover — product management, before any issue exists

| Stage | Agent |
|---|---|
| Research features worth building (market scan, comparable products, user pain) | `search-specialist` |
| Turn a promising direction into a sized proposal | `dev-workflows-fullstack:requirement-analyzer` |

The PM stage looks outward: what do audit-log products (auditlog services, SIEM
ingest, compliance tooling) offer that Ledgerly doesn't, and what would tenants
actually pay attention to — retention policies, export formats, webhooks,
verification APIs, SDKs. Output is a GitHub issue per proposal with the research
attached.

**Governance:** discovered features are always filed as `needs-triage`, never
`ready-for-agent`. The PM agent proposes; only Chris greenlights. This stage runs
on request or as an occasional loop task when the queue is empty — it never
self-feeds the build pipeline.

### 1. Curate — from idea to accepted requirement

| Stage | Agent |
|---|---|
| Analyze a raw request, size it, find the essence | `dev-workflows-fullstack:requirement-analyzer` |
| Write the PRD for non-trivial features | `dev-workflows-fullstack:prd-creator` |

Output lands on the GitHub issue (body or comment). Small fixes skip the PRD;
anything that adds a user-visible capability gets one before planning.

### 2. Plan — from requirement to implementation plan

| Stage | Agent |
|---|---|
| Design the implementation (all surfaces) | `deep-plan` |

`deep-plan` loads the CLAUDE.md hierarchy and full toolset (jcodemunch, LSPs), so it
plans against this repo's actual conventions — async write path, tenant isolation,
hash-chain invariants. It is read-only; its plan goes on the issue.

### 3. Build — tests first, then implementation

Tests-first is **enforced, not trusted**: the test agent runs before the executor,
and the executor's brief is "make these failing tests pass," not "implement X."

| Stage | Agent |
|---|---|
| Write failing tests from the plan (RED) | `ecc:tdd-guide` |
| Implement Go API / CLI (GREEN) | `ledgerly-go-executor` (project agent) or `dev-workflows-fullstack:task-executor` |
| Implement React frontend (GREEN) | `ledgerly-web-executor` (project agent) or `dev-workflows-fullstack:task-executor-frontend` / `frontend-developer` |

The executors are repo-branded agents whose system prompts carry the audit-log
invariants (tenant isolation, frozen hash format, 202 contract, fail-closed
verification) so no dispatch can forget them. Any installed specialist executor from the accept-list above is compliant.
Generic `general-purpose` is a fallback only when none are available — the
dispatch-log audit flags it (with the required note explaining why). Build/fix
executors must also load the relevant installed skills (golang-*, frontend/react
patterns) and list them in their return; the audit checks skill usage too.

`make test` is the canonical check (keep `-p=1`). Frontend gets its own test
runner under `web/` once scaffolded; wire it into `make test`.

### 4. Review — before any merge

| Surface | Agent |
|---|---|
| Go changes | `ecc:go-reviewer` |
| React / TypeScript changes | `react-reviewer` |
| Auth, JWT, tenant-isolation, or crypto diffs | `ecc:security-reviewer` (in addition to the language reviewer) |

CRITICAL/HIGH findings block the merge; the executor fixes and re-reviews.

### 5. Fix & unblock

| Situation | Agent |
|---|---|
| Bug with a repro | `debugger` (diagnose) → tdd-guide writes the regression test → `ledgerly-go-executor` / `ledgerly-web-executor` fixes |
| Go build/vet failure | `ecc:go-build-resolver` |
| React build failure | `ecc:react-build-resolver` |

### 6. Maintain

| Task | Agent |
|---|---|
| Dead code, duplication cleanup | `ecc:refactor-cleaner` |
| Docs and codemap updates | `ecc:doc-updater` |

Maintenance runs opportunistically when the issue queue is empty, never
concurrently with an in-flight feature branch.

## Rules

- One issue in flight at a time per surface; branch per issue.
- Stages run in order — no skipping review, no implementation before failing tests exist.
- Reviewers and planners are read-only; only build-stage agents and resolvers edit.
- Cross-tenant isolation is the unforgivable bug: any diff touching tenant scoping
  automatically adds the security review stage.
