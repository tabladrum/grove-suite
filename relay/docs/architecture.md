# Relay — System Architecture

Derived from `product-proposal.md` (§8, §9) and the Codex/Gemini design reviews.

---

## 1. Architectural Principles

1. **Agent-agnostic.** Accept ChangeSets from any source (Claude Code, Cursor, Devin, Copilot Workspace, internal scripts).
2. **Same engine, three deployment modes.** A single binary runs on a developer's laptop, a team server, or a multi-tenant enterprise control plane. Mode is determined by configuration, not by which binary is installed.
3. **Same engine, three surfaces.** The certification engine is callable via MCP (for agents), CLI (for humans and scripts), and git hook (as backstop). All three surfaces evaluate identical gates and emit the same certificate format.
4. **Agent-in-loop, not agent-downstream.** Relay is a tool the agent invokes during its own iteration loop, not a gate the agent runs into afterwards. Findings are returned as structured tool results that agents can act on without human translation.
5. **Linear admission, no branches in critical path.** Every certified commit lands on a designated linear branch with full trailer metadata.
6. **Graph-driven isolation, never graph-only safety.** ICR + Fuse + certification together are the safety boundary; no single mechanism is treated as sufficient.
7. **Auditability is a first-class output of every mode.** Every admitted commit carries a signed certificate, even in laptop mode. Operational state lives in Postgres (or SQLite in laptop mode); immutable audit state lives in git via outbox pattern.
8. **Tenant isolation by construction.** Every resource (Postgres schema, Redis cluster, intent-store, K8s namespace, Grove instance) is tenant-scoped from Phase 2A onward, even when a single tenant is the only one running.
9. **Batteries included.** Static-analysis tooling (semgrep, gitleaks, govulncheck, language linters) ships with the binary or is fetched on first run. Zero CI configuration required to get value.
10. **Configuration in the repo.** Per-repo Relay configuration lives in `.relay/` inside the source repo. The same configuration applies whether running on laptop, team server, or enterprise control plane. Org-wide baselines (enterprise mode) layer on top; user/host config holds only credentials and transport. The repo defines its own Relay rules, not the platform.
11. **Open core.** Grove, Prism, Fuse are MIT and embeddable. Relay is BSL and is the commercial control plane.

---

## 1.5 Deployment Modes

The same Relay binary runs in three modes. Mode selection is configuration-driven; there is no separate "laptop edition" vs. "enterprise edition" build.

### 1.5.1 Laptop mode (solo developer)

```
┌──────────────────────────────────────────────────┐
│ Developer machine                                 │
│  ┌──────────────────────────────────────────┐    │
│  │ relay (single binary)                     │    │
│  │   - MCP server (stdio or TCP)             │    │
│  │   - CLI                                   │    │
│  │   - git pre-push hook handler             │    │
│  │   - embedded SQLite (operational state)   │    │
│  │   - local intent-store git repo (audit)   │    │
│  │   - bundled tool binaries                 │    │
│  └──────────────────────────────────────────┘    │
│            │                                      │
│            ▼                                      │
│       Grove (local instance, MIT, also bundled)   │
└──────────────────────────────────────────────────┘
```

- **No Postgres, no Redis.** SQLite for operational state. ICR locks are irrelevant (single-agent).
- **No server process required.** Relay runs as a child process of the IDE (via MCP stdio), as a CLI, or as a git hook.
- **Intent-store is a local git repo** (`~/.relay/intent-store/`). Certificates are committed there.
- **Tool binaries bundled** in the install (or fetched on first run): semgrep, gitleaks, govulncheck, golangci-lint, eslint, ruff.
- **Cost: zero.** Same binary as team/company mode.

### 1.5.2 Team mode (one server, small team)

```
┌──────────────────────────────────────────────────────────────┐
│ Team server (one VM)                                          │
│  ┌──────────────────┐  ┌──────────────────┐                  │
│  │ relayd            │  │ Postgres          │                 │
│  │ (control plane)   │←→│ (single instance) │                 │
│  └──────────────────┘  └──────────────────┘                  │
│         │              ┌──────────────────┐                  │
│         └─────────────→│ Redis            │                  │
│                        │ (ICR locks)      │                  │
│                        └──────────────────┘                  │
│         │              ┌──────────────────┐                  │
│         └─────────────→│ intent-store git │                  │
│                        │ (shared repo)    │                  │
│                        └──────────────────┘                  │
└──────────────────────────────────────────────────────────────┘
       ▲                       ▲
       │ MCP (HTTP+SSE) /      │
       │ CLI / git hook        │
       │                       │
   Developer A              Developer B
   (Claude Code + relay CLI) (Cursor + relay CLI)
```

- **Postgres + Redis required.** ICR locks become meaningful with multiple agents.
- **Single intent-store git repo** shared across the team.
- **Dashboard at `:9000`** for triage, queue state, certificate history.
- **Developers point their local MCP clients at the server.** Same `relay_check` / `relay_certify` tools, now hitting the shared service.

### 1.5.3 Company mode (multi-tenant, multi-region)

Full Phase 4 deployment topology — Grove Federation, sharded intent-stores, per-tenant Postgres/Redis, multi-cluster K8s, regional control planes, global Audit Aggregator. See product-proposal §9.4 for the full diagram.

### 1.5.4 Transition between modes

| From | To | What changes |
|------|-----|--------------|
| Laptop → Team | Add `--postgres-url`, `--redis-url`, `--intent-store-url` flags. Run `relay migrate sqlite-to-postgres` to migrate operational state. Certificates from laptop mode remain valid. |
| Team → Company | Enable multi-tenancy in config; add tenant lifecycle CLI. Existing single-tenant data becomes tenant 0. |

No binary swap. No data rewrite. Audit history carries forward across all transitions.

---

## 1.6 Configuration Layout (`.relay/` in the repo)

Per-repo Relay configuration is source-controlled in the repo alongside the code it governs. This is what makes "same Relay on laptop, team, company" actually true: the rules travel with the code.

### 1.6.1 Layering order

```
┌─────────────────────────────────────────────────┐
│ 1. Built-in defaults                            │
│    Ship with the Relay binary.                  │
└─────────────────────────────────────────────────┘
                      ▼ merged with
┌─────────────────────────────────────────────────┐
│ 2. Org baseline                                  │
│    platform-config repo (enterprise tier only). │
│    Can lock individual settings to prevent      │
│    per-repo override (CISO defense in depth).   │
└─────────────────────────────────────────────────┘
                      ▼ merged with
┌─────────────────────────────────────────────────┐
│ 3. Repo config (.relay/ in source repo)          │
│    Committed alongside code. PR-reviewed.        │
│    Applies in every deployment mode.             │
│    The primary configuration surface.            │
└─────────────────────────────────────────────────┘
                      ▼ merged with
┌─────────────────────────────────────────────────┐
│ 4. User/host config (~/.relay/config.yaml)       │
│    Personal credentials, transport, signer key.  │
│    Never contains policy.                        │
└─────────────────────────────────────────────────┘
                      │
                      ▼
              Effective config
              (cached, hash recorded in certificate)
```

### 1.6.2 Repo config layout

```
my-repo/
├── .relay/
│   ├── relay.yaml              # entry point: version pin, gate enable/disable, runners, cert stages
│   ├── policies/               # per-gate config
│   │   ├── path.yaml
│   │   ├── secrets.yaml
│   │   ├── fileclass.yaml
│   │   ├── deps.yaml
│   │   ├── size.yaml
│   │   └── coverage.yaml
│   ├── rulesets/               # custom rule bundles or imported profiles
│   │   └── acme-java.yaml      # e.g., output of `relay import sonarqube-profile`
│   ├── intents/                # source-controlled intents (optional, intent-driven workflow)
│   │   └── INT-2026-042.yaml
│   └── templates/              # intent templates for the team
│       └── feature.yaml
├── src/
└── tests/
```

Like `.github/`, `.vscode/`, `.husky/`, `.editorconfig` — committed to the repo, PR-reviewed, branch-able, fork-able.

### 1.6.3 Discovery

`relay_check`, `relay check`, the git pre-push hook, and the MCP server all walk upward from the working directory looking for the nearest `.relay/relay.yaml`. The first one found defines the project. Same pattern as git, npm, eslint, prettier, husky.

In monorepos, sub-paths can have their own `.relay/`; the deepest match wins. The repo's root `.relay/` provides defaults; per-package `.relay/` overrides specific gates.

### 1.6.4 Version pinning

`relay.yaml` declares the engine version range it expects:

```yaml
schema: relay.config/v1
relay_version: ">=0.5 <0.6"
```

If the running binary doesn't satisfy the range, it errors with a clear message instead of evaluating with the wrong semantics. This prevents "stale agent's binary silently passes new rules" failure modes.

### 1.6.5 Lockable settings (org baseline)

Org baseline can mark fields immutable:

```yaml
# platform-config/policies/baseline.yaml (enterprise only)
gates:
  secrets:
    enforce: lock              # repo config cannot disable or relax
    severity_block: [error]
  path:
    forbidden_paths:
      mode: union              # repo's forbidden_paths add to baseline; cannot remove
```

Lockable settings give the CISO defense in depth: no developer accidentally turns off secret scanning by editing their repo's `.relay/`.

### 1.6.6 What lives where — definitive split

| Lives in `.relay/` (repo) | Lives in `platform-config` (enterprise) | Lives in `~/.relay/config.yaml` (host) |
|---------------------------|-----------------------------------------|----------------------------------------|
| Gates enabled per project | Org-wide baseline gates                  | Postgres DSN, Redis URL                |
| Path policies              | Locked path policies                     | KMS key ID / local signer path         |
| Test runners + commands    | Mandatory cert stages                    | MCP transport (stdio/http) + port      |
| Coverage threshold         | Minimum coverage threshold (locked)      | OIDC token / tenant token              |
| Custom rulesets, imported profiles | Approved ruleset whitelist       | Intent-store git URL (team mode)       |
| Intent templates           | Org-wide intent templates                | Bundled tool path overrides            |
| Special-file-class paths   | Mandatory special-file-class paths       | Telemetry endpoint                     |
| ICR confidence thresholds  | Minimum ICR confidence (locked)          |                                        |
| Stack templates baseline (`relay init --stack=...`) | Org-required stack overrides | |

**Rule of thumb:** if it's about *what the project's code must satisfy*, it goes in `.relay/`. If it's about *organization-wide minimum standards*, it goes in `platform-config`. If it's about *where this Relay binary connects and how it authenticates*, it goes in `~/.relay/config.yaml`.

### 1.6.7 Config hash in the certificate

The effective merged config is hashed (sha256) and recorded in every certificate as `Policy-Version` and `Effective-Config-Hash`. Two certificates with the same hash were evaluated by identical rules. This is what makes audit reproducible: a regulator can replay the cert verification against the historical config hash.

### 1.6.8 Initial bootstrap

```bash
cd my-repo
relay init --stack=go-microservice    # creates .relay/ with stack-appropriate defaults
git add .relay/
git commit -m "Add Relay configuration"
```

After this, every Relay invocation against this repo — laptop, team, company; MCP, CLI, git hook — uses the same configuration. No further setup required.

---

## 2. Logical Component Map

```
┌──────────────────────────────────────────────────────────────────────┐
│                          Relay Control Plane                          │
│                                                                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────────┐    │
│  │ Ingest API   │  │ Orchestrator │  │ Admission Controller     │    │
│  │ (HTTP + CLI) │→→│ (state mach.)│→→│ (rebase, sign, commit)   │    │
│  └──────────────┘  └──────┬───────┘  └────────────┬─────────────┘    │
│                           │                       │                   │
│                    ┌──────▼──────┐         ┌──────▼──────┐           │
│                    │ Policy Gate │         │ Cert Engine │           │
│                    │ (paths,     │         │ (build,test,│           │
│                    │  classes,   │         │  SAST, slice│           │
│                    │  secrets)   │         │  via Grove) │           │
│                    └──────┬──────┘         └──────┬──────┘           │
│                           │                       │                   │
│                    ┌──────▼───────────────────────▼──────┐           │
│                    │  ICR Lock Manager (Redis + tokens)   │           │
│                    └──────┬───────────────────────────────┘           │
│                           │                                           │
│  ┌──────────────┐  ┌──────▼───────┐  ┌──────────────────────────┐    │
│  │ Dashboard /  │  │ Outbox       │  │ Agent Pod Runtime (P2B+) │    │
│  │ Audit Read   │  │ Reconciler   │  │ (K8s ephemeral)          │    │
│  └──────────────┘  └──────┬───────┘  └──────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────  │
                            │
   ┌────────────────────────┼─────────────────────────────────────┐
   │                        │                                     │
   ▼                        ▼                                     ▼
┌────────────┐      ┌───────────────┐                  ┌──────────────────┐
│ Postgres   │      │ intent-store  │                  │ Grove (per repo) │
│ (per       │      │ (git, per     │                  │ + Prism + Fuse   │
│  tenant)   │      │  tenant)      │                  │ (read-side)      │
└────────────┘      └───────────────┘                  └──────────────────┘
        ▲                                                       ▲
        │                                                       │
        └────────────── source-repo (per project, git) ─────────┘
                                  │
                                  ▼
                       ┌────────────────────┐
                       │ Redis (per tenant) │
                       │ ICR locks, queue   │
                       └────────────────────┘
```

---

## 3. Components

### 3.1 Ingest API
- HTTP `POST /api/v1/changesets` and `relay submit` CLI.
- Validates ChangeSet schema, extracts agent identity, model, prompt hash, base commit.
- Writes `Intent` + `ChangeSet` rows in Postgres; emits `ingested` event.

### 3.2 Orchestrator
- Drives the intent state machine: `ingested → analyzing → policy → locking → certifying → admitting → admitted | rejected | failed`.
- Stateless service; leader election via Postgres advisory locks for per-tenant work pools.

### 3.3 Policy Gate
- `allowed_paths` / `forbidden_paths` enforcement.
- Special file class detection (migrations, lockfiles, generated files, configs, OpenAPI, snapshots).
- Secret scanner, dependency change detector, risky API detector.
- Policies live in `platform-config` repo, versioned by Git SHA.

### 3.4 ICR Lock Manager
- Computes ICR via Grove (`/icr`, `/deps`, `/impact`).
- Classifies symbols: `exclusive`, `shared_read`, `boundary`.
- Computes ICR confidence from edge confidences + file class weights.
- Acquires Redis locks using `SET NX EX` with fencing tokens; admission verifies fencing token before commit.
- Confidence policy:

| Confidence | Action |
|------------|--------|
| ≥ 0.85 | Auto-admit on cert pass |
| 0.70–0.85 | Admit + notify human reviewer |
| 0.50–0.70 | Require human approval |
| < 0.50 | Escalate to file-level lock; possibly reject |

### 3.5 Certification Engine
- Stage 1: build, lint, Grove-selected unit tests.
- Stage 2: SAST, dependency/secret scan.
- Stage 3 (optional): integration tests in ephemeral env.
- Produces signed certificate (see §6).

### 3.6 Admission Controller
- Rebases ChangeSet onto current HEAD of target branch.
- If rebase produces conflicts: invokes Fuse for semantic merge.
- On rebase or post-Fuse change: re-runs Stage 1 (fast certification slice).
- Commits with full trailer set, signs certificate, releases Redis locks, triggers Grove incremental re-index.

### 3.7 Outbox Reconciler
- Reads `outbox` table; writes immutable snapshots + certificates to per-tenant `intent-store` git repo.
- Ensures Postgres ↔ git eventual consistency without dual-write hazards.

### 3.8 Audit Read API
- Federated read across tenant intent-stores.
- Powers dashboard + compliance export.

### 3.9 Agent Pod Runtime (Phase 2B+)
- K8s operator spawns ephemeral pod per intent.
- Pod image bundles: Claude Code SDK, Grove client, Prism client, git, language toolchains.
- Pod produces ChangeSet; submits via Ingest API; exits.
- Heartbeats to Redis; dead pods reaped, intent re-enqueued.

### 3.10 MCP Server (the primary agent surface)

The same certification engine that powers the admission flow also exposes itself as an MCP server. This is how agents (Claude Code, Cursor, Continue, Windsurf, etc.) integrate with Relay in their iteration loop.

**Transport:**
- Laptop mode: stdio (child process of the IDE) — zero network setup.
- Team / company mode: HTTP+SSE on the shared server.

**Tools (Phase 2A):**

| Tool | Purpose | Returns |
|------|---------|---------|
| `relay_check` | Run certification pipeline on uncommitted changes (or a provided diff) | Structured findings: `{file, line, rule, severity, message, fix_hint?}` + pass/fail per gate |
| `relay_certify` | Full certification + emit signed certificate (commit-ready) | Certificate payload + signature + commit trailer block |
| `relay_submit` | Submit ChangeSet to admission queue (team/company mode only) | Intent ID + queue position + estimated admission time |
| `relay_policy` | Fetch active policy for the current project | Policy YAML + version SHA |
| `relay_explain` | Human-readable explanation of a finding | Markdown explanation + rule documentation + remediation |

**Engine isomorphism.** The MCP tools, CLI commands, and git hook all invoke the same internal engine. There is no separate "MCP path" — only a different transport binding. Adding a gate to the engine adds it to all three surfaces simultaneously.

**Agent loop pattern.** The intended usage in an agent's system prompt is:

> "After making code changes, call `relay_check` before reporting completion. If findings are returned, fix them and re-check. Do not report success while `relay_check` returns findings."

This pattern shifts certification from post-push (CI feedback) to in-loop (agent self-correction). The agent never produces a PR that fails policy because it can't get past `relay_check`.

### 3.11 Signature Capability Services

These services sit on top of the core engine. None of them adds a new persistent store; each consumes data already produced by ingestion + certification + admission. They are the surface area that turns Relay from "a CI gate" into a product developers and auditors *talk about*. See [product-proposal.md §7B](product-proposal.md#7b-signature-capabilities-the-things-people-remember) for product rationale.

| Service | Phase | Inputs | Outputs |
|---------|-------|--------|---------|
| **Pre-Flight Autopilot** | 2A | `relay_check` findings schema + recommended agent prompt | Agent-loop UX; ships as `docs/agent-prompt.md`, no new component |
| **AI Code Passport** | 2A | Stored certificate + intent | `relay cert show`, PR bot comment, dashboard card, JSON-LD payload |
| **Diff Risk Heatmap** | 2A | ICR + Grove `/deps` + coverage delta + boundary policy tags | Per-file/per-symbol risk score embedded in PR comment + dashboard |
| **Surgical Revert** | 2B | Historical Intent + ICR + ChangeSet | New ChangeSet reverting only the recorded symbols; goes through normal admission |
| **Evidence Replay** | 2A-late / 2B | Certificate + `Repo-Config-SHA` + `Toolchain-Image` + stored ChangeSet | Verdict comparison report; pass = byte-reproducible |
| **Policy Marketplace** | 2A → 2B | Community-curated `.relay/` profile bundles | `relay init --profile=<...>`; pinned, versioned, signed |
| **Review Budget Optimizer** | 2B | Heatmap + cert confidence + boundary flags + reviewer policy | PR check that recommends `skim`/`standard`/`senior`/`two-person` |
| **Agent Scorecard** | 2B → 3 | Cert + event log aggregated per agent/model/stack/tenant | Dashboard report + Prometheus metrics + exportable CSV |

**Architectural rule.** No signature capability is allowed to introduce its own database, UI framework, or external dependency. Each is a read view + a thin write path that funnels back through the existing engine. If a proposed capability needs new infrastructure, it goes through a separate ADR.

**What is explicitly out of the architecture:**
- A standalone "Diff Comprehension UI" / interactive flow plan — replaced by the Risk Heatmap inside existing PR tooling.
- A "Self-Healing Sandbox" that auto-fixes failed certifications outside the agent loop — would create a new attack surface and erode the certificate's signal value. Pre-Flight Autopilot is the supported version of this idea.
- Any Relay-native code review chat or conversation product.

---

## 4. Data Topology

| Concern | Laptop mode | Team / Company mode | Scope |
|---------|------------|---------------------|-------|
| Mutable workflow state | Embedded SQLite | Postgres | Per tenant (schema or instance in company mode) |
| Audit snapshots + certificates | Local git (`~/.relay/intent-store`) | Git (`intent-store`) | Per tenant |
| Code graph | Grove (SQLite, local) | Grove (per source repo) | Per source repo |
| Locks, queue, heartbeats | Not needed (single-agent) | Redis | Per tenant (cluster in company mode) |
| Source code | Git (`source-repo`) | Git (`source-repo`) | Per project |
| Policies, domain config | Local file or `platform-config` repo | Git (`platform-config`) | Per tenant (or shared) |
| Agent runtime images | N/A | OCI registry | Global (Phase 2B+) |
| Bundled tools | Local binaries / fetched on first run | Container image | Global |

**Consistency.** Operational store (SQLite or Postgres) is the source of truth. Outbox + reconciler is the only path from operational store to git audit state. Git is never written directly by application code. This invariant holds in all three modes.

---

## 5. Tenancy and Region Model

Adopted from §9.4 of the proposal but mandatory from Phase 2A.

- A **tenant** is the unit of isolation: own Postgres schema (or instance), own Redis cluster (or namespace), own intent-store, own K8s namespace.
- A **region** holds a regional control plane + N tenants. Tenants do not span regions.
- A **global Audit Aggregator** (Phase 4) federates read-only audit across regions.

Phase 2A ships with single-region, single-tenant deployment but with all schemas and resource names parameterized by `tenant_id` and `region` so the scale-out in Phase 4 is mechanical.

---

## 6. Certificate Format

Commit trailers on every admitted commit:

```
Certificate-ID: cert-<date>-<short-id>
Certificate-Issued: <ISO8601>
Base-Commit: <sha>
Intent-ID: <id>
Agent-Identity: <name:version>
Model-Identity: <name:date>
Prompt-Template-Hash: sha256:<hex>
Context-Pack-Hash: sha256:<hex>
ICR-Hash: sha256:<hex>
ICR-Confidence: <0.0–1.0>
Tests-Selected: <n>
Tests-Passed: <n>
SAST-Passed: <bool>
Repo-Config-SHA: <commit-sha of .relay/ in the source repo>
Effective-Config-Hash: sha256:<hex>     # canonicalized merged config across all layers
Policy-Version: <platform-config@sha>   # enterprise mode only; empty in laptop/team
Toolchain-Image: <oci-digest>
Signed-By: <signer-key-id>
```

Certificate JSON snapshot also written to `intent-store` at `certificates/<cert-id>.json`.

---

## 7. External Interfaces

### 7.1 Public (consumed by users, agents, CI)

**MCP (primary agent surface):**
- `relay_check` — pre-flight certification on uncommitted changes; structured findings.
- `relay_certify` — full certification + signed certificate (commit-ready).
- `relay_submit` — submit ChangeSet to admission queue (team/company mode).
- `relay_policy` — fetch active project policy.
- `relay_explain` — human-readable finding explanation.

Transport: stdio (laptop) or HTTP+SSE (team/company).

**HTTP REST:**
- `POST /api/v1/changesets` — submit a ChangeSet.
- `POST /api/v1/intents` — create an intent (used by webhooks, CLI).
- `GET /api/v1/intents/{id}` — status.
- `GET /api/v1/certificates/{id}` — fetch certificate.
- `GET /api/v1/audit/query?...` — federated audit query (paged).
- Webhooks in: Jira, GitHub Issues.
- Webhooks out: bot comments back to source systems.

**CLI:** `relay check`, `relay certify`, `relay submit`, `relay policy`, `relay explain` — same engine, terminal surface.

**Git hook:** `pre-push` invokes `relay check` and blocks on findings (configurable).

### 7.2 Internal (Relay → suite components)
- Grove: `/icr`, `/impact`, `/deps`, `/tests`, `/symbols`, `/index`.
- Prism: `prism_query`, `prism_read`, `prism_search`, `prism_lookup` (used inside agent pods).
- Fuse: `fuse merge` driver invoked by Admission Controller.

### 7.3 Operational
- Postgres, Redis, OCI registry, K8s API, OIDC/SAML IDP (Phase 4A).

---

## 8. Cross-Cutting Concerns

| Concern | Approach |
|---------|----------|
| AuthN | OIDC/SAML for humans (P4A); mTLS / signed JWT for service-to-service from P2A. |
| AuthZ | RBAC roles: `viewer`, `intent-author`, `reviewer`, `admin`, `security-champion`. Tenant-scoped. |
| Secrets | External secret store (Vault / cloud KMS). Relay never persists raw secrets. Cert signing key in KMS. |
| Observability | OpenTelemetry traces from ingest → admission; metrics per stage; structured JSON logs with `intent_id`, `tenant_id`, `cert_id`. |
| Cost accounting | Per-intent cost sidecar (P2B): LLM tokens, pod-seconds, storage delta. Persisted with intent. |
| Failure handling | Idempotent stage retries; admission is the only non-retryable step (uses fencing token + atomic git push). |
| Backpressure | Per-tenant queue with priority; oldest-first within priority; budget enforcement at submit time. |
| Upgrade safety | Cert engine version pinned per intent; platform-config version pinned per certificate. |

---

## 9. What This Architecture Explicitly Does Not Include (yet)

Aligned with proposal §6.3 and reviewer guidance:

- No parallel decomposition of a single intent into sub-intents (Phase 3).
- No intent groups / atomic multi-commit admission (Phase 3).
- No canary deployment hooks in the critical path (Phase 3+ or never).
- No multi-provider model routing (Phase 3+).
- No replacement for GitHub PR UI (ever, for non-agent code).

These can be added as additive services (`Decomposer`, `GroupAdmissionController`, `CanaryGate`, `ModelRouter`) without changing the core admission contract.
