# Relay Roadmap

Relay is an intent-driven delivery platform. No branches, linear history, AI agents in Kubernetes pods. Requires Grove.

## v0.1.0 — Intent Store & State Machine
_Target: Phase 1–4 of Implementation Plan_

- [ ] Intent YAML schema: `id`, `title`, `description`, `granularity_score`, `changesets`, `certification`, `state`
- [ ] Granularity scoring: GS = Specificity × (1 − Decomposability) ≥ 0.7 required
- [ ] Git-based intent store: linear append-only log, no branches
- [ ] State machine: `draft → proposed → signed → executing → review_pending → certifying → certified → merged → deployed → realized → failed`
- [ ] Redis ICR locks: `SET NX EX` (transient only, no persistence); max 3 critical intents/24h

## v0.2.0 — CKG & Conflict Detection
_Target: Phase 5–7 of Implementation Plan_

- [ ] CKG (Code Knowledge Graph) service: thin Grove proxy — no internal graph, all queries forwarded to `grove_query` / `grove_impact` / `grove_deps`
- [ ] 3-layer conflict detection:
  - Layer 1 (Structural): Grove symbol overlap via ICR lock table
  - Layer 2 (Contract): interface/API signature compatibility check
  - Layer 3 (Semantic): LLM-assisted conflict resolution prompt (local, no external API)
- [ ] ChangeSet validation: every ChangeSet must include tests (validated before `proposed` state)

## v0.3.0 — Agent Execution & K8s Operator
_Target: Phase 8–10 of Implementation Plan_

- [ ] Kubernetes operator: spawns agent pods for each Intent execution, lifecycle managed by operator
- [ ] Agent pod spec: OCI image, env injection (intent YAML, Grove URL, platform-config git URL)
- [ ] Agent execution protocol: agent writes ChangeSet back to intent-store git repo on completion
- [ ] 3-layer certification pipeline: automated (lint + test), peer review, compliance sign-off

## v0.4.0 — Deployment & Observability
_Target: Phase 11–14 of Implementation Plan_

- [ ] Linear admission controller: serializes all certified intents into main branch
- [ ] Canary deployment: always required — 5%→25%→100% traffic shift with automated rollback
- [ ] Deployment state tracking: `deployed → realized` on success, `failed` with rollback on error
- [ ] Observability: intent lifecycle events → audit log; Prometheus metrics for certification rates

## v1.0.0 — Production Ready

- [ ] E2E test: intent `draft → realized` in < 5 minutes on reference workload
- [ ] Linear admission: zero merge conflicts on main branch by design
- [ ] Operator published to OperatorHub
- [ ] BSL license with commercial use agreement for teams > 10
