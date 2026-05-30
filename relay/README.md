# Relay

**MIT licensed · Part of [Grove Suite](../README.md)**

Relay is the certified delivery layer for autonomous coding agents — a single binary that runs on a developer's laptop, a team server, or a multi-region enterprise control plane. It integrates with any coding agent (Claude Code, Cursor, Devin, GitHub Copilot Workspace, internal scripts) **as an MCP tool the agent calls in its own iteration loop**, providing pre-flight certification, graph-aware impact analysis, semantic merge resolution, and linear admission with cryptographic certificates.

The product thesis: AI-generated code volume has already overrun human PR review capacity. Reviewing every diff doesn't scale. Letting the agent self-correct against a machine-readable certification gate *before* the diff ever reaches a human does. Relay is what the agent calls between writing code and pushing it. See [docs/product-proposal.md](docs/product-proposal.md) for the full positioning and roadmap, and [docs/product-proposal.md §4.5](docs/product-proposal.md) for how Relay differs from skills, orchestration frameworks, and background agents.

## What Relay Does

- **Pre-flight certification (MCP-first)**: the agent calls `relay_check` after writing code; structured findings (file, line, rule, severity, fix-hint) flow back so the agent self-corrects in-loop before any human sees the diff
- **Same engine on three surfaces**: MCP server (for agents), CLI (for humans), git pre-push hook (backstop) — all run identical gates and emit identical certificates
- **Three deployment modes**: laptop (single binary + SQLite, zero config), team (Postgres + Redis, shared intent-store), company (multi-tenant, multi-region — Phase 4)
- **Configuration in the repo (`.relay/`)**: per-repo Relay config is source-controlled alongside the code (`relay.yaml`, policies, rulesets, intent templates). Same `.relay/` produces identical results in every deployment mode. Effective config hash recorded in every certificate, so audit replay is byte-reproducible
- **Batteries-included tooling**: ships with semgrep, gitleaks, govulncheck, golangci-lint, eslint, ruff, checkstyle, pmd. Zero CI configuration to get value
- **Intake**: accepts intents from Jira, GitHub Issues, CLI, MCP, or direct API; validates scope via two-stage granularity scoring (heuristic + Grove ICR symbol count)
- **Impact analysis**: uses Grove's `/icr`, `/impact`, `/deps` to compute the affected symbol set and blast radius
- **Policy gating**: six gates — path policy, secret scanner, special file class, dependency change, size limit, test coverage of changed symbols
- **Certification**: build + full test suite (selective opt-in for monorepos) + coverage-of-changed-symbols + standalone static-analysis suite. No external SonarQube/CI server required
- **SonarQube profile import**: bring your existing quality profile (`relay import sonarqube-profile`)
- **Admission**: linear commit to a designated branch with full trailer metadata (intent ID, certificate ID, agent identity, model version, ICR hash)
- **Audit in every mode**: signed certificates emitted in laptop mode too; intent-store git repo accumulates an append-only AI-code provenance trail, exportable for EU AI Act / SOC 2

Relay is designed to work *alongside* GitHub today. As confidence in agent output and graph isolation grows, it removes the need for branches for approved classes of agent-delivered work.

## Full Pipeline

```
Work item (Jira / GitHub Issues / CLI)
     │
     ▼
Project routing  (B → C → A, see below)
     │
     ▼
GS check  (heuristic + Grove ICR symbol count)
     │
     ├── too broad → needs_info + feedback to source
     └── scoped    → queued
     │
     ▼
Grove decomposition
POST grove:7777/icr  → affected symbol list
POST grove:7777/deps → edges between symbols
union-find → connected components
     │
     ├── Component A (independent)    ├── Component B (independent)
     │   Agent Pod in K8s             │   Agent Pod in K8s
     │   Prism context delivery       │   Prism context delivery
     │   Claude runs, produces diff   │   Claude runs, produces diff
     │                                │
     └──────────── Fuse merge ────────┘
                       │
                       ▼
              Certification
              POST grove:7777/impact → blast radius
              POST grove:7777/tests  → test selection
              lint + test + deploy dry-run
                       │
                       ▼
              Admission
              rebase → linear commit to main
                       │
                       ▼
              Canary → metric validation → realized
```

**Current status:** Phase 1 (intake + routing) is built. Phase 2 (decomposition + execution + certification + admission) is designed, not yet implemented.

---

## Data Model

The central concept is **Project** — the onboarding unit that ties a codebase location to its work item sources.

```
Repo
  url: https://github.com/acme/backend
  default_branch: main
    │
    │  one repo → many projects (monorepo support)
    ▼
Project
  name: auth-service
  source_path: /services/auth
  gs_threshold: 0.75
  auto_approve: false
  owner: eng-auth@acme.com
    │
    ├── ProjectIntegration (M:M with external boards)
    │     type: jira          external_id: AUTH
    │     type: github_issues external_id: acme/backend
    │     config: trigger_status, relay_project_field,
    │             label_trigger, component_filter, auto_approve
    │
    └── Intent
          project_id → Project   ← the structural link
          description, status, gs_score, icr_symbols
          source: jira | github_issue | native
          source_ref: AUTH-123 | acme/backend#42
```

### Onboarding a repo

```bash
# 1. Register the repo
relay repo add --name backend --url https://github.com/acme/backend

# 2. Create projects (one per logical service / monorepo path)
relay project add auth-service  --repo backend --path /services/auth  --owner eng-auth@acme.com
relay project add auth-worker   --repo backend --path /services/auth-worker
relay project add payments      --repo backend --path /services/payments

# 3. Link work item sources (M:M — AUTH board feeds both auth projects)
relay project link auth-service  jira AUTH           --trigger "Ready for Relay" --field "Relay Project"
relay project link auth-worker   jira AUTH           --trigger "Ready for Relay" --component auth-worker
relay project link payments      jira PAY            --trigger "Ready for Relay" --auto-approve
relay project link auth-service  github_issues acme/backend --label relay

# 4. Start the server
relay serve
```

### Source-controlled configuration (`.relay/`)

Each source repo carries its own Relay configuration in `.relay/`, committed alongside the code it governs. The same `.relay/` applies in laptop, team, and company modes — config travels with the repo.

```
my-repo/
├── .relay/
│   ├── relay.yaml              # version pin, gates enabled, runners, cert stages
│   ├── policies/               # per-gate detail (path, secrets, fileclass, deps, size, coverage)
│   ├── rulesets/               # custom rule bundles + imported SonarQube profiles
│   ├── intents/                # source-controlled intents (optional)
│   └── templates/              # intent templates for the team
├── src/
└── tests/
```

Bootstrap:

```bash
cd my-repo
relay init --stack=go-microservice    # scaffolds .relay/ with stack defaults
git add .relay/ && git commit -m "Add Relay configuration"
```

Configuration layering (resolved at every invocation): built-in defaults ⨁ org baseline from `platform-config` (enterprise only; can lock specific fields) ⨁ `.relay/` in the repo ⨁ `~/.relay/config.yaml` (credentials and transport only — never policy).

`relay_check`, `relay check`, and the git pre-push hook walk upward from the working directory to find the nearest `.relay/relay.yaml` — same discovery pattern as git, npm, eslint. Monorepos can have nested `.relay/` directories; the deepest match wins.

The effective merged config is hashed and recorded in every certificate as `Effective-Config-Hash` so audit replay is byte-reproducible.

---

## Project Routing (B → C → A)

When a work item arrives, Relay resolves which Project owns it. This matters because the same Jira board can feed multiple projects (e.g. AUTH board → auth-service and auth-worker).

```
[B] Explicit relay-project field (highest priority)
    Jira:   custom field "Relay Project" = "auth-service"
    GitHub: label "relay-project:auth-service"
    → direct route, no ambiguity

[C] Unrouted (fallback when B fails)
    Intent created with status: unrouted
    Appears in dashboard triage queue
    Human assigns: relay intent assign <id> --project auth-service

[A] Component/label filter (optional refinement)
    Configured per ProjectIntegration in config JSONB
    Jira: component_filter = "auth-worker"
    GitHub: component_label = "component:auth"
    → auto-routes if exactly one integration matches
    → falls through to C if multiple match
```

---

## Intent Lifecycle

```
ingest → draft
           │
    B resolved?─────yes──► validating
           │
           no
           │
           ▼
        unrouted ──► human assigns project ──► validating
                                                    │
                                          ┌─────────┼─────────┐
                                          ▼         ▼         ▼
                                      needs_info  queued  rejected
                                          │
                                      resubmit
                                          │
                                          ▼
                                      validating
```

Every transition is recorded as an `intent_event` (actor, timestamp, detail). Full event log: `GET /api/intents/:id/events`.

---

## Granularity Score (GS)

The GS check runs at intake and again after project assignment (using the project's configured threshold, not the global default).

**Stage 1 — heuristic (no Grove, fast):**
- Penalises descriptions under 8 words or over 150 words
- Penalises vague phrases: "refactor all", "update everything", "improve", "clean up"
- Rewards specificity: endpoint, function, method, or file names
- Penalises cross-domain scatter (3+ distinct system domains)

**Stage 2 — Grove ICR:**
- `POST grove:7777/icr {"intent": description}` → affected symbol count
- 1–50 symbols: GS 0.70–0.95 (well-scoped)
- 51–150 symbols: GS 0.40–0.70 (borderline)
- 151+ symbols: GS < 0.40 (too broad)

Final GS = 40% heuristic + 60% ICR. Per-project threshold (default 0.70).

**Note on decomposability (Phase 2):** A high symbol count does not automatically mean rejection. If those symbols form independent connected components in the Grove graph, the intent can be decomposed into parallel sub-tasks — each scoped appropriately. The GS check will account for this in Phase 2.

---

## Source System Integration

### Jira

- **Inbound**: `POST /integrations/jira/webhook` — `jira:issue_updated` events. Triggers on configured status transition. HMAC-SHA256 validation.
- **Manual pull**: `relay intent from-jira AUTH-123`
- **Outbound**: bot comments on status changes, transition ticket on realization (Phase 2)

### GitHub Issues

- **Inbound**: `POST /integrations/github/webhook` — `issues` events. Triggers when configured label added. `X-Hub-Signature-256` validation.
- **Manual pull**: `relay intent from-github owner/repo#123`
- **Outbound**: bot comments, close issue on realization (Phase 2)

### GitHub Projects

Supported via `type: github_projects` integration. Uses custom field for explicit routing (B) instead of label.

### Adding new sources

Implement the `core.Connector` interface:

```go
type Connector interface {
    Name() string
    FetchTicket(ref string) (*TicketData, error)
    PostComment(ref, message string) error
    TransitionTicket(ref, toStatus string) error
}
```

Register in the integration registry at startup. Routing and lifecycle work without modification.

---

## HTTP API

All `/api/*` routes require `Authorization: Bearer <token>` (auto-generated at `.relay/.token` on first start). Webhook routes and `/health` are unauthenticated.

```
GET  /health
GET  /                                    HTML dashboard

# Intents
GET  /api/intents                         list: ?status=&project_id=&source=&limit=&offset=
POST /api/intents                         create (body: description, project_id, author)
GET  /api/intents/unrouted                triage queue for unrouted intents
GET  /api/intents/:id
POST /api/intents/:id/approve             body: {"approved_by": "email"}
POST /api/intents/:id/reject              body: {"rejected_by": "email", "note": "..."}
POST /api/intents/:id/assign              body: {"project_name": "auth-service", "assigned_by": "email"}
GET  /api/intents/:id/events              full event log

# Repos
GET  /api/repos
POST /api/repos                           body: {"name": "backend", "url": "...", "default_branch": "main"}
GET  /api/repos/:id
DELETE /api/repos/:id

# Projects
GET  /api/projects                        ?repo_id=
POST /api/projects                        body: {"name": "auth-service", "repo_name": "backend", "source_path": "/services/auth"}
GET  /api/projects/:id
DELETE /api/projects/:id
GET  /api/projects/:id/integrations
POST /api/projects/:id/integrations       body: {"type": "jira", "external_id": "AUTH", "config": {...}}
DELETE /api/projects/:id/integrations/:integration_id

# Webhooks (HMAC-validated, unauthenticated)
POST /integrations/jira/webhook
POST /integrations/github/webhook
```

---

## Dashboard

`http://localhost:9000` shows:
- Pipeline strip: draft / unrouted / validating / needs_info / queued / rejected
- **Unrouted triage queue** — intents awaiting project assignment
- Recent intents: project, source badge, description, GS score, status, age
- Repos and projects registered with Relay
- Integration status: Jira / GitHub / Grove connectivity

---

## CLI Reference

```bash
relay serve [--port 9000] [--db <dsn>] [--config relay.yaml]

relay repo add --name <name> [--branch main] <url>
relay repo list
relay repo remove <id>

relay project add <name> --repo <repo-name> [--path /] [--owner eng@acme.com] [--gs-threshold 0.70]
relay project list
relay project show <id-or-name>
relay project link <project> <type> <external-id> [--trigger <status>] [--label <label>] [--field <field>] [--component <comp>] [--auto-approve]
relay project unlink <integration-id>

relay intent create <description> --project <name> [--domain <tag>] [--author <a>]
relay intent list [--status <s>] [--project <id>]
relay intent show <id>
relay intent approve <id> [--by <email>]
relay intent reject <id> [--by <email>] [--reason <text>]
relay intent assign <id> --project <name> [--by <email>]
relay intent from-jira <ticket-id>
relay intent from-github <owner/repo#number>

relay version
```

---

## Configuration

`relay.yaml`:

```yaml
port: 9000
grove_url: http://localhost:7777
db_url: postgres://localhost/relay
token_file: .relay/.token
gs_threshold: 0.70          # global default; per-project threshold overrides this

jira:
  url: https://company.atlassian.net
  token: ${JIRA_API_TOKEN}
  webhook_secret: ${JIRA_WEBHOOK_SECRET}
  relay_project_field: "Relay Project"   # Jira custom field name for B-routing

github:
  token: ${GITHUB_TOKEN}
  webhook_secret: ${GITHUB_WEBHOOK_SECRET}
  label_trigger: "relay"
```

Project-level integration config is stored in Postgres (via `relay project link`), not in `relay.yaml`. `relay.yaml` holds only global credentials and defaults.

Environment overrides: `RELAY_PORT`, `RELAY_DB_URL`, `GROVE_URL`, `RELAY_GS_THRESHOLD`, `JIRA_API_TOKEN`, `JIRA_WEBHOOK_SECRET`, `GITHUB_TOKEN`, `GITHUB_WEBHOOK_SECRET`.

---

## Grove Integration

Relay calls Grove at three points:

| When | Endpoint | Purpose |
|------|----------|---------|
| GS check (intake) | `POST /icr` | Symbol count → is intent scoped? |
| Decomposition (Phase 2) | `POST /icr` + `POST /deps` | Find independent connected components → parallel agents |
| Certification (Phase 2) | `POST /impact` + `POST /tests` | Blast radius + test selection |

Relay auto-starts Grove if unreachable at startup (same contract as Prism and Fuse).

---

## Storage

| What | Where | Why |
|------|-------|-----|
| Intent proposals, status, events | Postgres | Mutable, needs concurrent writes, complex queries |
| Project and repo config | Postgres | Operational state, changes frequently |
| Routing rules (ProjectIntegration) | Postgres | M:M relationships, queried on every webhook |
| Approved intent snapshots + certificates | Git (Phase 2) | Immutable, auditable, version-controlled |
| Agent execution state | Postgres + K8s Jobs (Phase 2) | Operational |

---

## Security

- HTTP server binds to `127.0.0.1:9000` — no LAN exposure
- Bearer token at `.relay/.token` (mode 0600) required on all `/api/*` routes
- Webhook routes are HMAC-validated (Jira: SHA256, GitHub: X-Hub-Signature-256) before any processing
- Postgres credentials via environment variables only

---

## Testing

```bash
make test
go test ./internal/ingestion/...           # GS scoring
go test ./internal/lifecycle/...           # state machine transitions
go test ./internal/routing/...             # B→C→A routing logic
go test ./internal/integration/jira/...    # HMAC + webhook parsing
go test ./internal/integration/github/...  # HMAC + webhook parsing
```
