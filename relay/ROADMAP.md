# Relay Roadmap

MIT licensed. Part of the Grove Suite. Requires Grove (`localhost:7777`) and Prism for context delivery.

For full product positioning, success thesis, and market context, see [`docs/product-proposal.md`](docs/product-proposal.md).

---

## Phase 1 — Intent Intake (✅ shipped)

Single-process service that ingests intents from Jira webhooks, GitHub Issues webhooks, and direct CLI, validates scope via two-stage granularity scoring, routes them to a registered project via B → C → A routing, and tracks their lifecycle in Postgres with bidirectional feedback to source systems.

- [x] Intent schema, state machine, event log
- [x] Two-stage granularity scoring (heuristic + Grove ICR symbol count)
- [x] Per-project GS threshold
- [x] Repo / Project / ProjectIntegration data model (M:M with external boards)
- [x] B → C → A project routing
- [x] Jira + GitHub webhook receivers with HMAC validation
- [x] Bot-comment feedback to source systems
- [x] Bearer-token authenticated HTTP API
- [x] HTML dashboard with triage queue
- [x] CLI: `relay serve / repo / project / intent`

---

## Phase 2A — Certified Merge Wedge (next: target 10–12 weeks)

Single binary that runs in **laptop, team, or company mode** (configuration-driven), exposes the certification engine via **MCP, CLI, and git pre-push hook**, and ships with the **static-analysis stack pre-bundled**. Accepts ChangeSets from any agent and admits certified, signed commits to a linear branch. The product's first commercial value proposition. See [`docs/architecture.md`](docs/architecture.md), [`docs/design.md`](docs/design.md), and [`docs/product-proposal.md`](docs/product-proposal.md) §6.

### MVP Properties

- [ ] **One binary, three deployment modes.** Laptop (SQLite, no Redis), team (Postgres + Redis), company-mode hooks (deferred to Phase 4). Config-driven, no separate builds.
- [ ] **MCP as primary agent surface.** Tools: `relay_check`, `relay_certify`, `relay_submit`, `relay_policy`, `relay_explain`. Stdio transport in laptop mode; HTTP+SSE in team mode.
- [ ] **Three surfaces, one engine.** Same Go packages execute via MCP, CLI, and git pre-push hook.
- [ ] **Agent-in-loop pattern.** Recommended system-prompt fragment ships with install; agents self-correct from `relay_check` findings before reporting completion.
- [ ] **Batteries included.** `relay tools install` bundles semgrep, gitleaks, govulncheck, npm audit, pip-audit, golangci-lint, eslint, ruff, checkstyle, pmd. Policy templates per stack (`relay init --stack=go-microservice`).
- [ ] **Signed audit trail in every mode.** Local Ed25519 keypair in laptop mode; KMS in team/company. Same certificate format across modes.
- [ ] **Configuration in the repo (`.relay/`).** Per-repo config (`relay.yaml`, `policies/`, `rulesets/`, `intents/`, `templates/`) committed alongside code. Layered merge: defaults ⨁ platform-config baseline (enterprise) ⨁ `.relay/` ⨁ `~/.relay/config.yaml` (credentials only). Effective config hash recorded in every certificate. `relay init --stack=...` scaffolds. Discovery walks upward from cwd. Same `.relay/` produces identical results on laptop, team, and company mode.

### Core Pipeline

- [ ] ChangeSet ingestion API (HTTP `POST /api/changesets` + `relay submit` CLI + MCP `relay_submit`)
- [ ] Grove `/impact` + `/tests` + `/deps` integration for blast radius + test selection
- [ ] ICR computation with confidence scoring (uses edge confidence)
- [ ] Policy engine with six gates: path policy, secret scanner, special file class, dependency change, size limit, test coverage of changed symbols
- [ ] Redis ICR locks with **fencing tokens** (team mode; laptop mode is single-agent)
- [ ] Certification — Stage 1: build + **full unit/integration suite by default** (selective opt-in for monorepos), normalized JUnit XML, coverage-of-changed-symbols gate against Grove `tests` edges
- [ ] Certification — Stage 2: standalone static analysis suite (no external server) — semgrep, gitleaks, govulncheck/npm audit/pip-audit, language linters; Relay owns the quality gate verdict
- [ ] SonarQube profile importer (`relay import sonarqube-profile`) — parse SQ XML, map rules to semgrep, emit coverage gap report
- [ ] Rebase + fast re-certify against HEAD before admission
- [ ] Fuse semantic merge invoked on rebase conflict (not as a normal step)
- [ ] Admission controller: linear commit with full trailer set (Intent-ID, Agent, Model, Certificate, ICR-Hash, Test-Plan, Policy-Version, Toolchain-Image, Signed-By)
- [ ] Certificate signing (Ed25519 local or KMS-backed)
- [ ] Outbox pattern: storage → intent-store git for audit snapshots
- [ ] Intent YAML schema v2: `allowed_paths`, `verification_plan`, `ambiguity_policy`, `affected_interfaces`, `rollback_plan`, `feature_flag`, `observability_expectations`, `security_considerations`, `risk_level`, `related_artifacts`
- [ ] Dashboard (team mode): cert details, ICR confidence, queue state, audit read API
- [ ] State migration: `relay migrate sqlite-to-postgres` for laptop → team upgrade

### Signature Capabilities (Phase 2A — see [docs/product-proposal.md §7B](docs/product-proposal.md#7b-signature-capabilities-the-things-people-remember))

- [ ] **Pre-Flight Autopilot** — recommended agent system-prompt fragment + `relay.findings/v1` schema with `next_action` so agents self-correct in-loop
- [ ] **AI Code Passport** — `relay cert show <ref>` + dashboard card + PR/MR bot comment + JSON-LD export over the existing certificate
- [ ] **Diff Risk Heatmap** — per-symbol risk score (ICR + Grove `/deps` + coverage delta + boundary tags + historical defect density), versioned `risk_model_version`
- [ ] **Evidence Replay (foundational)** — `relay cert replay <cert-id>` with verdicts `byte_reproducible` / `tool_drift` / `config_drift` / `unrecoverable`
- [ ] **Policy Marketplace (bootstrap)** — `grove-suite/relay-profiles` repo, `relay init --profile=<name>`, ship 4 stack + 2 compliance profiles (`soc2-baseline`, `eu-ai-act-article-12`)

**Explicitly NOT in Phase 2A** (deferred per Codex + Gemini design review):
- Parallel agent decomposition
- Intent groups
- Canary deployment
- Multi-provider agent routing (Claude only)
- Branchless main as a marketing claim
- Multi-tenant control plane / SSO / RBAC (Phase 4A)

---

## Phase 2B — Self-Hosted Agent Execution (target weeks 13–24)

Adds the K8s operator and ephemeral pod model so Relay can be the agent runtime for teams that need self-hosted, governance-aware execution. Positions Relay as an alternative to Cursor Background Agents / Devin for compliance-driven enterprises.

- [ ] K8s operator + `AgentExecution` CRD
- [ ] Ephemeral pod: Grove index + Prism context delivery + Claude Code SDK + git clone
- [ ] Cost tracking sidecar (per-intent budget enforcement)
- [ ] Heartbeat + dead-agent recovery (fencing-token aware)
- [ ] Agent Decision Record attached to every ChangeSet (decisions made, alternatives considered, assumptions, confidence)
- [ ] Ambiguity policy enforcement: `fail_with_questions` vs. `proceed_with_default` per intent

### Signature Capabilities (Phase 2B)

- [ ] **Surgical Revert by Intent** (`relay revert --intent <id>`) — symbol-scoped inverse via stored ICR; fails loudly rather than widening scope
- [ ] **Human Review Budget Optimizer** — PR check recommending `skim` / `standard` / `senior` / `two-person` based on heatmap + cert confidence
- [ ] **Agent Scorecard (full)** — adds post-admission defect rate via incident-tracker integration; Prometheus exporter + CSV export
- [ ] **Policy Marketplace (community)** — open contribution flow, signature verification CI, target 12 stack + 5 compliance profiles

---

## Phase 3 — Parallel Decomposition, Intent Groups, Branchless (evidence-driven)

**Do not start until Phase 2 produces enough data to validate ICR predictions empirically.** Specifically: log every ChangeSet's ICR, log whether two consecutive ChangeSets *would have been* safe to parallelize, and calibrate the confidence model from real data.

- [ ] Grove union-find decomposition (`/icr` + `/deps` → connected components)
- [ ] Parallel agent execution per independent component
- [ ] Fuse semantic merge as a first-class step (not just rebase fallback)
- [ ] Intent groups (atomic admission across multiple sub-intents)
- [ ] Optional canary deployment integration (or delegate to Argo Rollouts / Flagger)
- [ ] Full intent-as-review-artifact UX (configurable review policy: none / optional / mandatory)

---

## Phase 4 — Enterprise Scale (gated on a paying Fortune-500 design partner)

Multi-tenancy, federation, and multi-region for deployments at the 50K–100K repo scale. See [`docs/product-proposal.md`](docs/product-proposal.md) §9 for the full breakdown of why each component breaks at this scale.

- [ ] **4A — Multi-tenancy primitives** (12 wk): tenant model, RBAC, per-tenant Postgres schemas, per-tenant Redis clusters, SSO/OIDC
- [ ] **4B — Grove Federation** (12 wk): Grove Router service, hot/cold tiering, object-storage index snapshots
- [ ] **4C — Intent-store sharding + Audit Aggregator** (8 wk): per-tenant intent-stores, federated audit API, monthly rollup tooling
- [ ] **4D — Multi-region control plane** (12 wk): regional topology, data residency enforcement, global audit federation
- [ ] **4E — Cost optimization platform** (ongoing): model routing (Haiku/Sonnet/Opus per GS), prompt + context caching, dedicated capacity contracts, billing-grade metering

---

## Non-Goals (current)

- Replacing GitHub as the development frontend
- Multi-provider agent routing in Phase 2 (decided: Claude only; revisit when there's evidence)
- Owning the design / ADR conversation surface (Relay consumes design artifacts via `related_artifacts`; it does not own them)
- Infrastructure changes (Terraform, K8s manifests, production database migrations) — code-only for Phase 2
- A Relay-native intent authoring UI in Phase 2A (CLI + YAML are sufficient; richer authoring comes in Phase 2B with the dashboard)
- **A standalone "diff comprehension" or interactive code-review UI** — Risk Heatmap delivers the value inside existing PR tooling
- **A "self-healing sandbox" that auto-fixes failed certifications outside the agent loop** — Pre-Flight Autopilot is the supported version of this idea; an external auto-fixer would create a new attack surface and erode the certificate's signal value
- **A Relay-native code-review chat / conversation product** — out of scope; Relay is admission + evidence
