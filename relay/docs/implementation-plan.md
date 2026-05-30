# Relay — Implementation Plan (Per Phase)

Each phase below has: goal, scope, deliverables, exit criteria, dependencies, and risks. Phases gate on exit criteria, not calendar time.

Reference: `product-proposal.md` §10, §9.5; `architecture.md`; `design.md`.

---

## Phase 2A — Certified Merge Wedge

### Goal
End-to-end loop: any agent → ChangeSet → Grove-driven certification → signed admission to a linear branch with full audit trail. **Single binary running in laptop mode (SQLite, no Redis) or team mode (Postgres + Redis), with MCP as the primary agent surface and pre-bundled static-analysis tooling.** Single tenant, single region, sequential.

### Scope (in)

**Deployment modes (design.md §12.5):**
- Laptop mode: single binary, embedded SQLite, local intent-store git repo, no Redis. Same engine as team mode.
- Team mode: Postgres + Redis + shared intent-store. Same binary, configuration-driven.
- Company mode (multi-tenant) deferred to Phase 4.

**Engine surfaces (design.md §13) — same engine, three transports:**
- MCP server (primary agent surface) — stdio in laptop mode, HTTP+SSE in team mode. Tools: `relay_check`, `relay_certify`, `relay_submit`, `relay_policy`, `relay_explain`.
- CLI — identical engine, terminal surface.
- Git pre-push hook — backstop for unmanaged agents and humans.

**Configuration in the repo (design.md §12.6) — foundational invariant:**
- Per-repo config in `.relay/` (entry point `relay.yaml`, plus `policies/`, `rulesets/`, `intents/`, `templates/`).
- Config layering: built-in defaults ⨁ platform-config org baseline (enterprise only, can lock fields) ⨁ `.relay/` ⨁ `~/.relay/config.yaml` (credentials only).
- Discovery walks upward from cwd to nearest `.relay/relay.yaml`. Monorepos supported via nested `.relay/`.
- `relay_version` pin in `relay.yaml` — stale binary fails fast instead of mis-evaluating.
- Effective config hash recorded in every certificate (`Effective-Config-Hash`) — audit is byte-reproducible.
- `relay init --stack=<go-microservice|node-api|python-service|java-spring>` scaffolds `.relay/` with sensible defaults.
- Imported SonarQube profiles land in `.relay/rulesets/`, not platform-config.

**Core pipeline:**
- Ingest API + `relay` CLI + MCP server.
- Intent + ChangeSet schema + storage (SQLite or Postgres; same schema, same migrations).
- ICR computation against Grove (`/impact`, `/deps`, `/tests`, `/symbols-at`) with confidence scoring.
- Policy engine with six gates (`design.md` §10.2):
  1. Path policy (allowed/forbidden) — Deny
  2. Secret scanner (gitleaks-style patterns) — Deny
  3. Special file class (migration / lockfile / generated / config) — varies
  4. Dependency change — ReviewRequired
  5. Size limit — Deny (hard) / Warn (soft)
  6. Test coverage of changed symbols — Deny (runs inside certification)
- Redis ICR locks with fencing tokens (team mode only; laptop mode is single-agent).
- Certification engine — Stage 1 (build + **full unit/integration suite by default**, selective opt-in per project) and Stage 2 (standalone static analysis: semgrep, gitleaks, govulncheck/npm audit, language-specific linters — no external server required). Stage 3 deferred.
- Test runners for Go, Python, TypeScript, Java emitting normalized JUnit XML (`design.md` §7.4).
- Coverage-of-changed-symbols computation against Grove `tests` edges.
- Admission controller: rebase + (Fuse on conflict) + re-cert slice + signed commit on `relay-main`.
- Outbox reconciler → `intent-store` git repo (local in laptop, shared in team).
- Certificate signing via KMS (team mode) or local Ed25519 keypair (laptop mode). Same certificate format in both.

**Batteries-included tooling (`relay tools install`):**
- semgrep + bundled rulesets (security-audit, owasp-top-ten, optionally `p/sonarqube` via profile import).
- gitleaks.
- govulncheck, npm audit, pip-audit.
- golangci-lint, eslint, ruff, checkstyle, pmd.
- SonarQube quality profile importer (`relay import sonarqube-profile`).
- Default policy templates (`relay init --stack=go-microservice` / `node-api` / `python-service` / `java-spring`).

**Operator UX:**
- Minimal dashboard (team mode): intent list, intent detail, certificate view, coverage report.
- Bootstrapping: `relay init` (laptop), `relay tenant init` / `relay repo add` / `relay project add` (team).
- State migration: `relay migrate sqlite-to-postgres` for laptop → team upgrade with no audit-history loss.

### Scope (out, by policy)
- Parallel decomposition, intent groups, canary, multi-provider routing, K8s agent pods, multi-tenancy UX, SSO.

### Deliverables
1. Single binary `cmd/relay` running in laptop or team mode (configuration-driven, no separate builds).
2. Go packages:
   - `internal/engine/` — orchestrator, policy, icr, lock, cert, admission, outbox, signer (shared engine across surfaces).
   - `internal/api/{mcp,http}/` — MCP server + REST API transport bindings.
   - `internal/cli/` — CLI transport binding.
   - `internal/githook/` — pre-push hook handler.
   - `internal/store/{sqlite,postgres}/` — storage adapters with identical schema.
3. Storage migrations under `internal/store/migrations/` (SQLite-compatible SQL works against Postgres).
4. `platform-config/policies/<tenant>.yaml` example including full Stage 2 quality config + policy templates per stack.
5. OpenAPI spec at `api/openapi.yaml` + MCP tool schemas at `api/mcp/`.
6. Integration test harness: fake agent → MCP `relay_check` → real Grove → SQLite or Postgres → assert signed commit + valid certificate.
7. Docs: `getting-started.md` (laptop quickstart), `team-deployment.md`, `mcp-integration.md`, `operations.md`.
8. SonarQube profile importer: `relay import sonarqube-profile` CLI command + SQ-key → semgrep-rule-ID mapping table + coverage gap report.
9. Tool bundler: `relay tools install` downloads pinned versions of semgrep, gitleaks, govulncheck, golangci-lint, eslint, ruff to `~/.relay/tools/`. Build-time embedding for the install tarball.
10. Recommended agent system-prompt fragment (`docs/agent-prompt.md`) describing the `relay_check` in-loop pattern.
11. **Config loader (`internal/config/`)**: discovery (upward walk from cwd), layered merge (defaults ⨁ platform-config ⨁ `.relay/` ⨁ user/host), lock enforcement, version range check against `relay_version` pin, canonicalization + sha256 hash for the certificate's `Effective-Config-Hash` field.
12. **Stack templates (`templates/stacks/`)**: scaffold `.relay/` for `go-microservice`, `node-api`, `python-service`, `java-spring`. Each template includes sensible defaults for gates, runners, rulesets, and a starter `relay.yaml`.
13. **`.relay/` schema spec** (`docs/relay-config-schema.md`): formal schema for `relay.yaml` + each policy file. Used by editor tooling (JSON Schema export) and config validators.

### Exit criteria
- Solo-dev scenario (proposal §7.1) runs unattended: 100% certified-commit success on a curated 50-intent corpus across Go + TypeScript repos.
- 10-dev team scenario (§7.2): 3 concurrent non-overlapping ChangeSets admitted without conflict; 1 overlapping queued and admitted post-rebase.
- p95 ingest→admission latency ≤ 60 s on a Grove-warm repo for an intent touching ≤ 200 LOC.
- All Stage 1+2 failures produce actionable diagnostics in the dashboard and CLI.
- Certificate verifies end-to-end via `relay cert verify`.
- Outbox lag < 5 s p95; zero divergence between Postgres and intent-store on chaos test (Redis kill, worker kill, Postgres restart).
- **Config portability test:** the same `.relay/` produces byte-identical `Effective-Config-Hash` when evaluated by Relay running in laptop mode, team mode, and a simulated company-mode merge (with platform-config baseline applied). Required to prove "same Relay everywhere."
- **Config discovery test:** `relay check` invoked from a sub-directory finds the repo's `.relay/relay.yaml` via upward walk; nested `.relay/` in a monorepo sub-package overrides specific gates as expected.
- **Lock enforcement test:** platform-config baseline marks a field locked; a per-repo `.relay/` that tries to override it causes the config loader to fail with a clear error pointing at the offending file and field.
- **`relay init --stack=...` test:** scaffolds a working `.relay/` for each supported stack; subsequent `relay check` against a clean checkout passes with stack defaults.

### Dependencies
- Grove ≥ v0.1 with `/impact`, `/deps`, `/tests`, `/symbols-at`, `/index-delta`.
- Fuse ≥ v0.1 driver invocable from admission.
- Prism not required (only used by agent pods in Phase 2B).

### Risks
- Grove `/symbols-at` accuracy on diff hunks → mitigate with fallback to file-level when symbol resolution confidence < 0.4.
- Coverage gate false positives (Grove missed a `tests` edge) → annotation escape hatch `// relay:no-test-required <reason>` and metric `coverage_gate_overrides` to spot misuse.
- Selective test strategy regression (only relevant if opt-in is enabled) → mandatory nightly full-suite guard + freeze admission on drift detection.
- Fencing-token race on push → covered by atomic `git push --atomic` plus pre-receive verifier.
- Test runner heterogeneity → ship Go runner first, then Python; add TypeScript/Java only when a real project requires them. Don't speculatively build adapters.

### Suggested work breakdown (no time estimates)
1. **Foundations**: storage adapter interface (SQLite + Postgres implementations behind one interface), migrations, signer (local Ed25519 + KMS backends), `core` types (`Intent`, `ChangeSet`, `ICR`, `TestRun`, `Certificate`, `PolicyResult`).
2. **Config loader (`internal/config/`)** — early dependency for every downstream step. Implements: `.relay/` upward discovery from cwd, layered merge (defaults ⨁ platform-config ⨁ `.relay/` ⨁ user/host), lock enforcement at merge time, `relay_version` range check, canonicalization + sha256 hash. Exposes `Engine.LoadConfig(cwd)` and emits the `Effective-Config-Hash` consumed by the certificate. Includes JSON Schema export for editor tooling. Stack templates (`templates/stacks/go-microservice`, `node-api`, `python-service`, `java-spring`) scaffolded by `relay init`.
3. **Engine package**: `internal/engine/` exposes a single `Engine.Check()` / `Engine.Certify()` / `Engine.Submit()` API that the MCP, CLI, and git-hook surfaces all call. Surface code does transport, not logic. Depends on the config loader (step 2).
4. **Ingest**: HTTP API + CLI `relay submit` + Intent/ChangeSet persistence.
5. **ICR**: Grove client wiring, ICR computation, confidence scoring, hash canonicalization.
6. **Policy engine + 5 pre-cert gates** (`internal/policy/{path,secrets,fileclass,deps,size}`). Path gate ships first as the simplest; others follow. Each gate reads its config from `.relay/policies/<gate>.yaml` via the config loader.
7. **Lock manager** (team mode only): Redis SET NX EX + fencing-token sequence + lease renewal + storage mirror.
8. **Certification — Stage 1**: build + test runner (Go first), JUnit XML parsing, TestRun persistence.
9. **Coverage gate** (`internal/policy/coverage`): runs after Stage 1 tests pass; consumes TestRun + Grove `tests` edges.
10. **Certification — Stage 2**: standalone static analysis suite — SAST (semgrep with configurable rulesets), secrets (gitleaks), deps (govulncheck/npm audit/pip-audit), quality linters (golangci-lint, eslint, ruff, checkstyle/pmd). Relay aggregates findings and owns the quality gate decision. SonarQube profile importer (`relay import sonarqube-profile`): parse SQ quality profile XML, map SQ rule keys to semgrep rule IDs via bundled mapping table, emit coverage gap report for unmatched rules, write generated Relay ruleset YAML to `.relay/rulesets/` in the repo (NOT platform-config — config travels with the repo).
11. **MCP server**: `internal/api/mcp/` exposing `relay_check`, `relay_certify`, `relay_submit`, `relay_policy`, `relay_explain`. Stdio transport first; HTTP+SSE second. Tool schemas in `api/mcp/`. `relay_policy` returns the effective merged config plus its `Effective-Config-Hash`.
12. **Git pre-push hook**: `internal/githook/` + `relay hook install` CLI. Same engine entry points as MCP.
13. **Admission**: rebase + Fuse glue on conflict + re-cert slice + signed commit + atomic push. Certificate trailers include `Repo-Config-SHA` and `Effective-Config-Hash`.
14. **Outbox reconciler** + intent-store git layout (local in laptop, shared in team). Outbox payload includes the effective config hash so audit replay can recover the rules in force at admission time.
15. **Tool bundler**: `relay tools install` fetches pinned semgrep, gitleaks, govulncheck, golangci-lint, eslint, ruff to `~/.relay/tools/`. Build-time tarball embedding for offline install.
16. **Dashboard**: intent list, intent detail (with coverage + blast radius views), certificate view; audit read API. Shows the `.relay/` config diff between two certificates when comparing intents.
17. **State migration**: `relay migrate sqlite-to-postgres` (laptop → team upgrade). Does NOT migrate `.relay/` — that already lives in the repo and is unchanged across the upgrade.
18. **Chaos + integration test suite** (worker kill, Redis kill, Postgres restart, git push race, MCP client disconnect mid-run). Includes a "config drift" test: same diff submitted against two different `.relay/` commits must produce two different `Effective-Config-Hash` values.

### Signature capabilities included in Phase 2A

These are the developer/auditor-facing features that make Relay memorable; specified in [design.md §16](design.md#16-signature-capabilities--data-contracts).

19. **Pre-Flight Autopilot.** Recommended agent system-prompt fragment (`docs/agent-prompt.md`) + tight `relay_check` findings schema (`relay.findings/v1`) including `next_action`. Validated end-to-end with a real Claude Code / Cursor / Continue session in the integration suite.
20. **AI Code Passport.** `relay cert show <ref>` CLI command, dashboard card, PR/MR bot comment (GitHub + GitLab), JSON-LD export. No new tables — reads from existing `certificates` row.
21. **Diff Risk Heatmap.** Per-symbol risk score computed at admission time from ICR + Grove `/deps` + coverage delta + boundary tags + historical defect density. Stored under `certificates.payload.risk_heatmap` with a versioned `risk_model_version`. Rendered in the Passport and bot comment.
22. **Evidence Replay (foundational).** `relay cert replay <cert-id>` runs against any certificate produced by the same major version. Verdict values: `byte_reproducible`, `tool_drift`, `config_drift`, `unrecoverable`. Required in Phase 2A so the audit story is real on day one.
23. **Policy Marketplace (bootstrap).** Community profile bundles in a sibling repo (`grove-suite/relay-profiles`). `relay init --profile=<name>` fetches, verifies signature, lays into `.relay/`, records pin in `relay.yaml`. Phase 2A ships with 4 stack profiles + 2 compliance profiles (`soc2-baseline`, `eu-ai-act-article-12`).

### Signature capabilities deferred to Phase 2B

- Surgical Revert by Intent (`relay revert --intent <id>`) — requires the symbol-graph linkage to ICR to be production-validated first.
- Human Review Budget Optimizer — depends on heatmap data + a few months of reviewer-decision telemetry to calibrate the recommendation classes.
- Agent Scorecard (full version with post-admission defect rate) — depends on incident-tracker integration shipped in Phase 2B.

### Signature capabilities explicitly rejected

| Proposal | Rejection reason |
|----------|------------------|
| Standalone "Diff Comprehension UI" | Owning a proprietary review surface conflicts with the architecture's principle of meeting developers in GitHub/GitLab. Risk Heatmap delivers the value without the surface. |
| Self-Healing Sandbox (auto-fix agent on cert failure, outside the agent loop) | Creates a new attack surface and trains teams that Relay "always passes." Pre-Flight Autopilot is the supported version of this idea. |
| Relay-native code-review chat | Out of scope. Relay is admission + evidence, not a conversation product. |
| Multi-provider model routing in 2B | Premature without cost + quality data; Claude-only stands. |

---

## Phase 2B — Self-Hosted Agent Execution

### Goal
Relay becomes a self-hosted alternative to Cursor Background Agents / Devin for teams that need governance-aware agent execution inside their perimeter.

### Scope (in)
- K8s operator + CRDs: `IntentRun`, `AgentImage`.
- Ephemeral pod runtime bundling Claude Code SDK + Grove client + Prism client + git + language toolchains.
- Pod lifecycle: pre-warmed pool, sidecar for cost tracking and heartbeats, image cache on nodes.
- Prism integration inside pod for context delivery.
- Agent Decision Record (ADR) capture in every ChangeSet (schema in `design.md` §4).
- Ambiguity policy enforcement (`fail_with_questions` vs `proceed_with_default`).
- Per-intent budget enforcement (cost $, wall-clock).
- Bidirectional webhooks: Jira/GitHub issue → intent; intent terminal → comment.

### Deliverables
1. `cmd/relay-operator` Helm chart.
2. Container image `relay/agent-runtime:<lang-bundle>` (one per supported language group).
3. `internal/runner/k8s` package.
4. ADR schema v1 enforced at ingest.
5. Cost telemetry pipeline (OpenTelemetry → Postgres `intent_costs`).
6. Docs: `agent-runtime.md`, `webhooks.md`.
7. **Surgical Revert (`relay revert --intent <id>`)** — uses stored ICR + ChangeSet to synthesize symbol-scoped inverse; runs through standard admission; carries `Reverts:` trailer. Fails loudly (never widens scope silently) when the symbols have been further modified.
8. **Human Review Budget Optimizer** — PR check that recommends `skim` / `standard` / `senior` / `two-person` based on Risk Heatmap + cert confidence + boundary flags + a reviewer-policy file in `.relay/`. Tunable per project.
9. **Agent Scorecard (full)** — adds post-admission defect rate by joining certs against incident-tracker tags. Initial integrations: GitHub Issues, Jira, PagerDuty post-mortem links. Prometheus exporter + `relay scorecard --format csv`.
10. **Policy Marketplace (community)** — open contribution flow on `grove-suite/relay-profiles`: PR + signature verification + CI runs profile against a fixture corpus. Phase 2B target: 12 stack profiles + 5 compliance profiles.

### Exit criteria
- A real intent (`add rate limiting`) executed end-to-end inside the pod, producing a ChangeSet that passes Phase 2A admission with no human edits.
- Dead-agent recovery: kill a pod mid-run; intent re-enqueues; second attempt completes; no double-commit.
- Per-intent cost report shows token usage within ±5% of provider invoice for a 100-intent sample.
- Ambiguity policy: 100% of intents with unmet acceptance criteria either ask a question (annotated on the intent) or fail explicitly when policy = `fail_with_questions`.

### Dependencies
- Phase 2A admission contract frozen.
- Prism ≥ stable `prism_query` / `prism_read`.
- A K8s cluster (≥ 1.28); KMS access from pods.

### Risks
- Pod cold-start latency → pre-warmed pool + node-level image cache.
- Agent loops burning budget → hard wall-clock cap + per-stage cost ceiling.
- Secrets in pod → workload identity, no static creds; intent-scoped tokens.

---

## Phase 3 — Parallel Decomposition, Intent Groups, Branchless

### Goal
Unlock multi-intent parallel execution and atomic group admission, gated on empirical data collected during Phases 2A/2B.

### Gate (do not start until met)
- ≥ 6 months of production ICR data from Phase 2A+2B deployments.
- Empirical confusion matrix for ICR confidence vs actual conflict: precision ≥ 0.95 at confidence ≥ 0.85.
- A calibration model (logistic regression or gradient-boosted) trained on the data.

### Scope (in)
- `Decomposer` interface implementation: LLM-driven decomposition with GS and ICR pre-check; produces child intents linked by `parent_id`.
- Intent groups: atomic admission of N related intents using a multi-parent merge commit; group rolls back as one unit if any child fails.
- Calibrated confidence model replaces the heuristic in §5 of design.
- Optional `CanaryGate` interceptor (feature-flagged); first integration with Argo Rollouts / Flagger.
- Branchless trunk marketing repositioned: "branches unnecessary for approved classes" → backed by data.

### Deliverables
1. `internal/decomposer` package + Decomposer model prompt + eval suite.
2. `internal/groups` package; group state machine (`forming → executing → admitting | rejecting`).
3. Confidence model artifact + offline trainer + online evaluator.
4. CanaryGate adapter for Argo Rollouts.
5. Public dataset (sanitized) for ICR confusion matrix — basis for the white paper.

### Exit criteria
- Parallel admission of two non-overlapping child intents from a single decomposed parent, with end-to-end audit linkage.
- Group rollback test: 3-intent group where child 2 fails certification → none admitted, intent-store records the abort.
- Confidence model in production for ≥ 4 weeks with calibration drift < 5%.

### Dependencies
- Production telemetry pipeline (Phase 2B).
- A customer willing to opt into parallel execution.

### Risks
- Decomposer hallucinations create sub-intents that don't compose → require Decomposer output to pass GS ≥ threshold per child before lock attempt.
- Group admission deadlocks → topological lock ordering + deadlock detector in lock manager.

---

## Phase 4 — Enterprise Scale

### Goal
Run Relay at Fortune-50 scale: 10⁴–10⁵ repos, 10⁴–10⁵ developers, multi-region, multi-tenant, sovereign compliance. Mechanical extension of the wedge design; no fundamental rewrite.

### Gate
A signed Fortune-500 design partner with concrete deployment requirements. Do not begin without one.

### Sub-phases

#### 4A — Multi-tenancy primitives
- **Scope:** Tenant data model; RBAC roles (`viewer`, `intent-author`, `reviewer`, `admin`, `security-champion`); per-tenant Postgres schemas (or instances at high tier); per-tenant Redis clusters/namespaces; OIDC/SAML SSO; tenant-scoped CLI/API.
- **Deliverables:** `internal/tenant`, `internal/authz`, SSO adapters, tenant lifecycle CLI.
- **Exit:** 10 simulated tenants with strict isolation verified by red-team test.

#### 4B — Grove Federation
- **Scope:** Grove Router service; `repo → endpoint` registry; hot/cold tiering; object-storage index snapshots; Grove cluster operator.
- **Deliverables:** `cmd/grove-router`, snapshot tooling, cold-load benchmarks.
- **Exit:** 1,000-repo synthetic federation with p95 `/icr` ≤ 200 ms.

#### 4C — Intent-store sharding + Audit Aggregator
- **Scope:** Per-tenant intent-stores; monthly rollup tooling; federated read-only Audit Aggregator API; export pipelines (CSV, Parquet, S3).
- **Deliverables:** `cmd/audit-aggregator`, rollup cron, export adapters.
- **Exit:** Compliance query "all AI-generated commits to project X in Q2" returns in ≤ 30 s across 100 tenant stores.

#### 4D — Multi-region control plane
- **Scope:** Regional control planes (EU/US/APAC); data residency enforcement; cross-region federation for audit only; sovereign-deployment runbook.
- **Deliverables:** Helm umbrella chart, region routing, residency policy engine.
- **Exit:** Two-region deployment passes data residency conformance test; failover of one region without intent loss.

#### 4E — Cost optimization platform (ongoing)
- **Scope:** Model routing (Haiku/Sonnet/Opus by GS + ICR + risk); provider prompt-prefix caching; context-pack reuse; per-tenant budget enforcement; billing-grade metering and chargeback.
- **Deliverables:** `ModelRouter` implementation, caching layer, metering exporter (Prometheus + billing CSV).
- **Exit:** ≥ 60% reduction in LLM spend on a 10K-intent benchmark vs naive Sonnet-everywhere baseline.

### Cross-cutting compliance work
- SOC 2 Type II audit.
- ISO 27001 certification.
- FedRAMP Moderate readiness (if a US public-sector partner emerges).
- EU AI Act Article 12 (logging) and Article 14 (human oversight) attestation artifacts generated automatically from intent-store data.

### Risks
- Engineering effort without a design partner → enforce the gate.
- Vendor lock-in (KMS, K8s flavor) → abstract behind interfaces from Phase 4A.
- Cost overruns on agent runtime → Phase 4E is funded by the design partner's ACV, not by speculation.

---

## Cross-Phase: Always-On Workstreams

These run continuously alongside phased work.

| Workstream | Purpose |
|------------|---------|
| Security review | Every new endpoint, CRD, and policy gate gets a threat model entry. |
| Observability | OTel coverage ≥ 95% of state transitions; SLO dashboards published. |
| Open-source hygiene (Grove/Prism/Fuse) | Issue triage SLA, release cadence, contribution docs. Drives bottom-up adoption that funds top-down sales. |
| Compliance evidence collection | Generated automatically from intent-store; never hand-curated. |
| Performance regression suite | Phase 2A benchmark corpus runs on every PR; alerts on p95 latency or success-rate regression. |

---

## What This Plan Refuses to Promise

In line with proposal §6.3 and §13:

- No replacement of GitHub PR UI for human-authored code.
- No magic conflict resolution — Fuse is invoked only when rebase fails and is allowed to give up; rejection is a first-class outcome.
- No claim of "branchless main" until Phase 3 calibration data supports it.
- No multi-tenant or multi-region claim until Phase 4A is shipped and externally audited.
- No agent-vendor lock-in or feature parity with Cursor/Devin — Relay's value is downstream of any agent, not in competition with them.
