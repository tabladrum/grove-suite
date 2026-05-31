# Relay

**MIT licensed · Part of [Grove Suite](../README.md)**

Relay is the certified delivery layer for autonomous coding agents. It sits between an AI agent writing code and that code reaching your codebase — running build, tests, and static analysis locally, signing the result with a cryptographic certificate, and admitting it as a linear commit with a full audit trail.

---

## Why Relay

AI-generated code volume has outrun human PR review capacity. Reviewing every agent-produced diff at the same depth as human-written code doesn't scale — but letting agents self-merge without any gate erodes code quality and destroys the audit trail.

Relay is what the agent calls between writing code and pushing it. It provides:

- **Pre-flight certification** — the agent calls `relay_check` in its iteration loop; structured findings (file, line, rule, severity, fix-hint) flow back so the agent self-corrects before any human sees the diff.
- **Signed certificates** — every admitted commit carries an Ed25519 signature over the exact config, toolchain, test results, and findings. The cert is byte-reproducible: `relay cert replay <id>` re-runs gates and tells you whether the result still matches.
- **Audit-proof intent trail** — the user's natural-language prompt is captured as a YAML intent, committed alongside the code, and linked to the admission cert via `Intent-ID:` trailer. Not just the output — also the request that produced it.

The core design constraint: laptop mode requires zero infrastructure. One binary, SQLite, a local Ed25519 key. The same binary scales to team (Postgres + Redis + KMS) via config.

---

## What Relay Does

| Capability | How it works |
|------------|-------------|
| Pre-flight check | `relay_check` / `relay check`: SAST on changed files + Grove-affected unit tests. Sub-10 s target. |
| Full certification | `relay_certify` / `relay certify`: Stage 1 (build + test + coverage) + Stage 2 (static analysis suite). |
| Signed admission | Linear commit to target branch with Ed25519 signature and full trailer metadata. |
| Risk heatmap | Per-diff risk score: ICR + stage2 severity + coverage delta + touch intensity. Versioned model. |
| Certificate replay | `relay cert replay <id>`: re-runs gates, returns `byte_reproducible` / `tool_drift` / `config_drift`. |
| Intent capture | Auto-captures the user's prompt as a committed YAML intent before coding starts. |
| Agent wiring | `relay init` auto-writes Pre-Flight Autopilot instructions and MCP config for every detected AI tool (Claude Code, GitHub Copilot, Cursor, Codex CLI, Windsurf, Zed, VS Code, and more). |
| Batteries-included | Ships semgrep, gitleaks, govulncheck, eslint, ruff pre-bundled. No setup required. |
| Policy profiles | Built-in compliance profiles: `soc2-baseline`, `pci-dss-baseline`, stack-strict variants. |

---

## Quick Start

### Laptop (single binary, SQLite, no server required)

```bash
# 1. Build and install
cd relay && make install

# 2. Initialize in your project — scaffolds .relay/, generates local Ed25519 key,
#    writes Pre-Flight Autopilot instructions to CLAUDE.md / .cursorrules / .github/copilot-instructions.md
#    / AGENTS.md / GEMINI.md / .clinerules and registers MCP for every detected tool
#    (Claude Code, GitHub Copilot, Cursor, Codex CLI, VS Code, Claude Desktop, Windsurf, Zed, …)
cd /your/project
relay init --stack=go-microservice

# 3. Commit the .relay/ config alongside your code
git add .relay/ && git commit -m "Add Relay configuration"

# 4. Install the git pre-push hook (backstop for unmanaged pushes)
relay hook install

# 5. Have your AI agent use relay_check in its loop
#    System-prompt fragment: see docs/agent-prompt.md
```

---

## Installation

```bash
make build    # compile ./bin/relay
make install  # install to $GOPATH/bin
make test     # run all tests
```

**Requirements:**
- Go 1.22+
- Grove running at `http://localhost:7777` (Relay auto-starts it if unreachable)
- Git 2.x

---

## Configuration

Configuration lives in `.relay/` in your project root and is **committed alongside the code**. The same `.relay/` works identically in laptop, team, and company mode.

```
.relay/
├── relay.yaml          # version pin, gates, runners, admission target
├── .gitignore          # written by relay init; covers .relay/.cache/
├── policies/           # per-gate config (path, secrets, fileclass, deps, size, coverage)
├── rulesets/           # custom rule bundles + imported SonarQube profile XML
├── intents/            # committed intent YAMLs (Auto-Intent Capture lands here)
├── templates/          # intent templates for the team
└── .cache/             # GITIGNORED — daemon state, intent drafts, indexer caches
```

**`relay.yaml` (minimal laptop config):**

```yaml
version: "1"
admission_target: relay-main    # branch to admit certified commits to
gates:
  coverage: warn                 # warn | enforce | off
  secrets: enforce
  fileclass: enforce
  deps: warn
  size: warn
```

**Configuration layering:** built-in defaults ⨁ `.relay/relay.yaml` ⨁ `~/.relay/config.yaml` (credentials only).

The effective merged config is hashed into every certificate as `Effective-Config-Hash` — audit replay is byte-reproducible.

**Environment variables:**

| Variable | Default | Purpose |
|----------|---------|---------|
| `GROVE_URL` | `http://localhost:7777` | Grove instance URL |
| `RELAY_PORT` | `9000` | HTTP API / dashboard port |
| `RELAY_DB_URL` | `.relay/.cache/state.sqlite` | Database (SQLite on laptop, Postgres on team) |
| `JIRA_API_TOKEN` | — | Jira integration credential |
| `GITHUB_TOKEN` | — | GitHub integration credential |

---

## MCP Tools

Relay exposes 5 tools over MCP stdio, accessible from any MCP-capable AI agent (Claude Code, Cursor, Windsurf, Continue):

| Tool | What it does | When to call |
|------|-------------|--------------|
| `relay_check` | Fast pre-flight: SAST on changed files + Grove-affected unit tests. Sub-10 s. | Every iteration before requesting human review |
| `relay_certify` | Full certification: Stage 1 build + test + coverage + Stage 2 static analysis. | Before final admission |
| `relay_submit` | Submit a ChangeSet for admission (calls `relay_certify` internally). | When code is ready to commit |
| `relay_policy` | Query effective policy for the current workspace. | To understand what gates apply |
| `relay_explain` | Explain a specific finding with rule context and fix guidance. | When `relay_check` returns findings |

**Register with your IDE:**

```bash
relay mcp install-for claude-code    # writes .claude/mcp.json
relay mcp install-for cursor         # writes .cursor/mcp.json
relay mcp install-for windsurf       # writes .windsurf/mcp.json
relay mcp install-for continue       # writes .continue/config.json
```

**Start MCP stdio server directly:**

```bash
relay mcp serve [--repo .]
```

For the recommended agent system-prompt fragment, see [docs/agent-prompt.md](docs/agent-prompt.md).

---

## CLI Reference

### Core certification

```bash
relay init [--stack <name>] [--profile <name>] [.]
            # Scaffold .relay/, generate Ed25519 key, register MCP with IDEs
            # Stacks: go-microservice | node-api | python-service | java-spring
            # Profiles: soc2-baseline | pci-dss-baseline | go-microservice-strict | ...

relay check [--repo .]
            # Fast pre-flight: SAST on changed files + Grove-affected tests

relay certify [--repo .]
              # Full certification pipeline: Stage 1 + Stage 2 + admission

relay submit --diff <patch> --intent <id> [--repo .]
             # Submit a specific ChangeSet diff for certification
```

### Certificates

```bash
relay cert list [--limit 20]         # list recent certificates
relay cert show <id-or-ref>          # certificate details
relay cert show --jsonld <id>        # JSON-LD export (AI Code Passport)
relay cert verify <id-or-ref>        # verify Ed25519 signature
relay cert replay <id-or-ref>        # replay gates: byte_reproducible | tool_drift | config_drift
```

### Tools and profiles

```bash
relay tools install                  # install bundled tools (semgrep, gitleaks, govulncheck, ...)
relay tools list                     # show installed tools and versions
relay init --list-stacks             # list available stack templates
relay init --list-profiles           # list available compliance profiles
relay import sonarqube-profile <xml> # import SonarQube quality profile XML into .relay/rulesets/
```

### Git hook

```bash
relay hook install [--force] [--repo .]    # install git pre-push hook
relay hook uninstall [--repo .]            # remove hook
```

### Outbox

```bash
relay outbox push --intent-store=<path> [--repo .] [--batch 10]
# Push certified changesets from local store to the intent-store git repo
```

### Intent intake (Phase 1)

```bash
relay serve [--port 9000]            # start HTTP server + dashboard
relay intent create <description> --project <name>
relay intent list [--status <s>]
relay intent show <id>
relay intent approve <id>
relay intent reject <id>
relay intent from-jira <ticket-id>
relay intent from-github <owner/repo#number>
relay repo add --name <name> <url>
relay project add <name> --repo <repo-name> [--path /]
relay project link <project> jira <board-key>
relay project link <project> github_issues <owner/repo>
```

---

## Certification Pipeline

```
agent writes code
      │
      ▼
relay_check  (fast, in-loop)
  ├── SAST on changed files (semgrep, gitleaks, inline-secrets)
  ├── Grove-affected unit tests only
  └── findings → agent self-corrects

      │  agent satisfied
      ▼
relay_certify / relay certify  (full, at commit time)
  │
  ├── Stage 1: build + full test suite (git worktree isolation)
  │     └── coverage-of-changed-symbols gate (vs Grove /tests edges)
  │
  ├── Stage 2: static analysis
  │     ├── inline-secrets (always available)
  │     ├── gitleaks (secrets)
  │     ├── semgrep (OWASP ruleset + custom .relay/rulesets/)
  │     ├── govulncheck / npm audit / pip-audit (deps)
  │     └── eslint / ruff / golangci-lint (language linters)
  │
  ├── Risk heatmap
  │     └── ICR (0.30) + stage2 severity (0.30) + coverage delta (0.25) + touch intensity (0.15)
  │
  ├── Policy gates (path, secrets, fileclass, deps, size, coverage)
  │
  └── Admission
        ├── rebase onto target branch
        ├── Ed25519 sign (CanonicalBytes over config + results, excludes admitted commit SHA)
        ├── linear commit with full trailer:
        │     Intent-ID: · Agent: · Model: · Certificate: · ICR-Hash:
        │     Test-Plan: · Policy-Version: · Toolchain-Image: · Signed-By:
        └── certificate persisted to store + intent-store git
```

---

## Certificate Format

Every certificate is an Ed25519-signed record over: the ChangeSet, the effective config hash, toolchain versions, test results, and findings. The admitted commit SHA is added to the trailer *after* signing.

```bash
relay cert show HEAD
# Certificate: relay-cert-abc123
# Intent-ID:   INT-0042
# Agent:       claude-sonnet-4-6
# Stage1:      PASS (42 tests, 87.3% coverage)
# Stage2:      PASS (0 HIGH, 1 MEDIUM suppressed by policy)
# Risk:        0.12 (low) — model v1
# Admitted:    a1b2c3d (relay-main)
# Signed-By:   ~/.relay/keys/admission.ed25519

relay cert show --jsonld HEAD    # JSON-LD export for AI Code Passport / audit systems
relay cert replay HEAD           # re-run gates against current tools → byte_reproducible
```

---

## Built-in Profiles

Relay ships 6 profiles for `relay init --profile=<name>`:

| Profile | What it enforces |
|---------|-----------------|
| `soc2-baseline` | secrets + fileclass gates enforced; audit log required |
| `pci-dss-baseline` | secrets + deps + fileclass enforced; strict coverage threshold |
| `go-microservice-strict` | go vet + govulncheck + coverage ≥ 80% + fileclass |
| `node-api-strict` | eslint + npm audit + coverage ≥ 75% |
| `python-service-strict` | ruff + pip-audit + coverage ≥ 75% |
| `java-spring-strict` | checkstyle + pmd + coverage ≥ 80% |

---

## Grove Integration

Relay calls Grove at three points:

| Phase | Endpoint | Purpose |
|-------|----------|---------|
| Intake: GS scoring | `POST /icr` | Symbol count → is intent scoped tightly enough? |
| Certification: test selection | `POST /impact` + `POST /tests` | Which tests cover the changed symbols? |
| Certification: blast radius | `POST /impact` + `POST /deps` | What else might break? |

Relay auto-starts Grove if unreachable at startup (`GROVE_URL` health check → `exec grove serve --port 7777`).

---

## Security

- All HTTP servers bind to `127.0.0.1` — no LAN exposure
- Bearer token at `.relay/.token` (mode 0600) required on all `/api/*` routes
- Webhook routes validated with HMAC-SHA256 before any processing
- Ed25519 keypair at `~/.relay/keys/admission.ed25519` (mode 0600), generated on `relay init`
- Credentials via environment variables only — never in `.relay/relay.yaml`

---

## Testing

```bash
make test                                    # all packages
go test ./internal/cert/...                  # certification pipeline
go test ./internal/engine/...                # engine orchestrator
go test ./internal/analyzers/...             # static analysis adapters
go test ./internal/admission/...             # linear admission
go test ./internal/runner/...                # test runners (Go, Python, Node)
go test ./internal/ingestion/...             # GS scoring
go test ./internal/lifecycle/...             # state machine
go test ./internal/routing/...               # B→C→A routing
```

---

## Intent Intake (Phase 1)

Relay can also ingest work items from Jira and GitHub Issues, validate their scope via GS scoring, and route them to registered projects. This surface is independent of the certification engine.

```bash
relay serve --port 9000   # start HTTP server + dashboard at http://localhost:9000

# Register repos and projects
relay repo add --name backend --url https://github.com/acme/backend
relay project add auth-service --repo backend --path /services/auth
relay project link auth-service jira AUTH --trigger "Ready for Relay"
relay project link auth-service github_issues acme/backend --label relay
```

Dashboard at `http://localhost:9000` shows the triage queue, intent pipeline, GS scores, Grove connectivity, and registered projects.

See [docs/architecture.md](docs/architecture.md) and [docs/design.md](docs/design.md) for the full system design.
