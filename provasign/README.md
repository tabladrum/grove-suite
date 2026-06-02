# Provasign

> **Certified delivery for AI coding agents. Every commit signed, tested, and traceable to the prompt that created it.**

**AGPL-3.0 licensed · Part of [Grove Suite](../README.md)**

---

## The Loop That Wastes Everyone's Time

Here is how AI-assisted development works today — at every company, every team:

1. Developer prompts the agent
2. Agent codes, writes tests, opens a PR
3. CI runs — catches a security finding, a coverage gap, a linting violation
4. Developer goes back to the agent with the CI output
5. Agent fixes it, updates the PR
6. CI runs again — different finding
7. Repeat until it passes
8. Human reviews the diff — but has no idea what the original prompt was

This loop is slow, expensive, and gets *worse* as agent output volume grows. The fundamental problem: quality gates live at the end of the pipeline (CI, PR review), but the agent is at the beginning. By the time it sees a finding, it's already forgotten the full context.

**Provasign moves the quality gate into the agent loop.**

The agent calls `provasign_check` every iteration — sub-10 seconds, structured findings (file, line, rule, severity, fix-hint) returned directly. The agent self-corrects before it commits. Before it opens a PR. Before anyone waits for CI.

When the code is ready, `provasign_certify` runs the full suite: build, tests, coverage, secrets scanning, SAST, dependency audit. It signs the result with Ed25519 and admits it as a linear commit — with cryptographic proof that those specific gates passed at that specific toolchain version, and with the original user prompt committed alongside the code as a YAML intent linked via `Intent-ID:` trailer.

---

41% of production code is now AI-generated. Gartner puts that at 60% by end of 2026. The bottleneck isn't the agent's code quality — it's the infrastructure around the agent that was built for humans. PRs were invented for human review. CI was designed to catch what developers missed. Neither was designed for the volume, speed, or accountability requirements of autonomous agent output.

Provasign is that infrastructure. One binary. Laptop mode (SQLite, local Ed25519 key, zero config) or team mode (Postgres + Redis + KMS) — same certs, same audit trail.

---

## What Changes When You Add Provasign

**Before Provasign — the expensive loop:**
```
Prompt → agent codes → opens PR → CI finds issue → back to agent 
→ agent fixes → CI finds another issue → repeat 3–5× → human reviews
→ commit merged → 6 months later: nobody knows what the prompt was
```

**After Provasign — quality moves into the agent loop:**
```
Prompt → intent captured as YAML → agent codes → provasign_check (sub-10s)
→ agent self-corrects against structured findings → provasign_certify
→ Ed25519-signed cert: proof of build + tests + coverage + SAST
→ linear commit with Intent-ID: trailer linking cert to prompt
→ PR opened already certified — CI is a formality, not a gatekeeper
```

Every admitted commit carries:
- **Cryptographic proof** (Ed25519 signature) of exactly what gates ran, what passed, and with which toolchain versions
- **Audit trail** — the original user prompt committed as a YAML intent, the cert linked to the commit via `Intent-ID:` trailer, byte-reproducible via `provasign cert replay <id>`
- **Risk score** — ICR + static analysis severity + coverage delta + change intensity, versioned so you can compare across time

---

## Capabilities

| # | Capability | How it works |
|---|------------|-------------|
| 1 | **In-loop pre-flight** | `provasign_check` / `provasign check`: SAST on changed files + Grove-affected unit tests. Sub-10 s. Findings are structured (file, line, rule, severity, fix-hint) — returned to the agent so it self-corrects before any PR is opened. |
| 2 | **Full certification** | `provasign_certify` / `provasign certify`: Stage 1 (build + full test suite + coverage) + Stage 2 (secrets, SAST, dependency audit, language linters). Runs in a clean git worktree — isolated from local state. |
| 3 | **Signed admission** | Linear commit to target branch with Ed25519 signature over the exact ChangeSet, effective config hash, toolchain versions, and test results. The signature is computed before the commit SHA exists — the admitted SHA is added to the trailer after signing. |
| 4 | **Risk heatmap** | Per-diff risk score: ICR (0.30) + stage2 severity (0.30) + coverage delta (0.25) + touch intensity (0.15). Versioned model — scores are comparable over time. |
| 5 | **Cryptographic audit trail** | Every admitted commit links to a certificate. `provasign cert show <id>` shows exactly what ran and what passed. `provasign cert replay <id>` re-runs the gates against current tools and config and returns `byte_reproducible` / `tool_drift` / `config_drift`. Proof that the cert is still valid, or explanation of why it isn't. |
| 6 | **Intent trail** | The user's natural-language prompt is captured as a committed YAML intent before coding starts, linked to every admission cert via `Intent-ID:` commit trailer. Not just what the agent produced — the request that caused it. The prompt is committed to the repo alongside the code, not lost when the agent session ends. |
| 7 | **Agent wiring** | `provasign init` auto-writes Pre-Flight Autopilot instructions and MCP config for every detected AI tool (Claude Code, GitHub Copilot, Cursor, Codex CLI, Windsurf, Zed, VS Code, and more). The agent is told to call `provasign_check` before every PR, automatically. |
| 8 | **Batteries-included** | Ships semgrep, gitleaks, govulncheck, eslint, ruff pre-bundled. No setup required. Import a SonarQube profile XML with one command. |
| 9 | **Policy profiles** | Built-in compliance profiles: `soc2-baseline`, `pci-dss-baseline`, stack-strict variants. Gates are configurable: `warn` / `enforce` / `off` per gate type. |

---

## Quick Start

### Laptop — zero infrastructure required

```bash
# 1. Build Grove first (Provasign requires it), then Provasign
cd grove && make install
cd provasign && make install

# 2. Initialize in your project
#    - Scaffolds .provasign/, generates local Ed25519 key
#    - Writes Pre-Flight Autopilot to CLAUDE.md / .cursorrules /
#      .github/copilot-instructions.md / AGENTS.md / GEMINI.md / .clinerules / …
#    - Registers MCP for every detected tool
#      (Claude Code, GitHub Copilot, Cursor, Codex CLI, VS Code, Claude Desktop, Windsurf, Zed, …)
cd /your/project
provasign init --stack=go-microservice

# 3. Commit the .provasign/ config alongside your code
git add .provasign/ && git commit -m "Add Provasign configuration"

# 4. Install the git pre-push hook (backstop for unmanaged pushes)
provasign hook install

# 5. Your AI agent now calls provasign_check in its iteration loop automatically
#    (Pre-Flight Autopilot instructions tell it to)
```

---

## Installation

```bash
make build    # compile ./bin/provasign
make install  # install to $GOPATH/bin
make test     # run all tests
```

**Requirements:**
- Go 1.22+
- Grove running at `http://localhost:7777` (Provasign auto-starts it if unreachable)
- Git 2.x

---

## Configuration

Configuration lives in `.provasign/` in your project root and is **committed alongside the code**. The same `.provasign/` works identically in laptop, team, and company mode.

```
.provasign/
├── provasign.yaml          # version pin, gates, runners, admission target
├── .gitignore          # written by provasign init; covers .provasign/.cache/
├── policies/           # per-gate config (path, secrets, fileclass, deps, size, coverage)
├── rulesets/           # custom rule bundles + imported SonarQube profile XML
├── intents/            # committed intent YAMLs (Auto-Intent Capture lands here)
├── templates/          # intent templates for the team
└── .cache/             # GITIGNORED — daemon state, intent drafts, indexer caches
```

**`provasign.yaml` (minimal laptop config):**

```yaml
version: "1"
admission_target: provasign-main    # branch to admit certified commits to
gates:
  coverage: warn                 # warn | enforce | off
  secrets: enforce
  fileclass: enforce
  deps: warn
  size: warn
```

**Configuration layering:** built-in defaults ⨁ `.provasign/provasign.yaml` ⨁ `~/.provasign/config.yaml` (credentials only).

The effective merged config is hashed into every certificate as `Effective-Config-Hash` — audit replay is byte-reproducible.

**Environment variables:**

| Variable | Default | Purpose |
|----------|---------|---------|
| `GROVE_URL` | `http://localhost:7777` | Grove instance URL |
| `RELAY_PORT` | `9000` | HTTP API / dashboard port |
| `RELAY_DB_URL` | `.provasign/.cache/state.sqlite` | Database (SQLite on laptop, Postgres on team) |
| `JIRA_API_TOKEN` | — | Jira integration credential |
| `GITHUB_TOKEN` | — | GitHub integration credential |

---

## MCP Tools

Provasign exposes 5 tools over MCP stdio, accessible from any MCP-capable AI agent (Claude Code, Cursor, Windsurf, Continue):

| Tool | What it does | When to call |
|------|-------------|--------------|
| `provasign_check` | Fast pre-flight: SAST on changed files + Grove-affected unit tests. Sub-10 s. | Every iteration before requesting human review |
| `provasign_certify` | Full certification: Stage 1 build + test + coverage + Stage 2 static analysis. | Before final admission |
| `provasign_submit` | Submit a ChangeSet for admission (calls `provasign_certify` internally). | When code is ready to commit |
| `relay_policy` | Query effective policy for the current workspace. | To understand what gates apply |
| `relay_explain` | Explain a specific finding with rule context and fix guidance. | When `provasign_check` returns findings |

**Register with your IDE:**

```bash
provasign mcp install-for claude-code    # writes ~/.claude.json (global user config)
provasign mcp install-for cursor         # writes .cursor/mcp.json
provasign mcp install-for windsurf       # writes .windsurf/mcp.json
provasign mcp install-for continue       # writes .continue/config.json
```

For the recommended agent system-prompt fragment, see [docs/agent-prompt.md](docs/agent-prompt.md).

---

## CLI Reference

### Core certification

```bash
provasign init [--stack <name>] [--profile <name>] [.]
            # Scaffold .provasign/, generate Ed25519 key, register MCP with IDEs
            # Stacks: go-microservice | node-api | python-service | java-spring
            # Profiles: soc2-baseline | pci-dss-baseline | go-microservice-strict | ...

provasign check [--repo .]
            # Fast pre-flight: SAST on changed files + Grove-affected tests

provasign certify [--repo .]
              # Full certification pipeline: Stage 1 + Stage 2 + admission

provasign submit --diff <patch> --intent <id> [--repo .]
             # Submit a specific ChangeSet diff for certification
```

### Certificates

```bash
provasign cert list [--limit 20]         # list recent certificates
provasign cert show <id-or-ref>          # certificate details
provasign cert show --jsonld <id>        # JSON-LD export (AI Code Passport)
provasign cert verify <id-or-ref>        # verify Ed25519 signature
provasign cert replay <id-or-ref>        # replay gates: byte_reproducible | tool_drift | config_drift
```

### Tools and profiles

```bash
provasign tools install                  # install bundled tools (semgrep, gitleaks, govulncheck, ...)
provasign tools uninstall                # remove provasign-managed tool cache (~/.provasign/tools)
provasign tools list                     # show installed tools and versions
provasign init --list-stacks             # list available stack templates
provasign init --list-profiles           # list available compliance profiles
provasign import sonarqube-profile <xml> # import SonarQube quality profile XML into .provasign/rulesets/
```

### Git hook

```bash
provasign hook install [--force] [--repo .]    # install git pre-push hook
provasign hook uninstall [--repo .]            # remove hook

# one-command laptop teardown (hooks, MCP registrations, provasign instruction blocks,
# and tool cache; use after provasign local init)
provasign local uninstall --repo .
```

### Intent management

```bash
provasign intent open --title <t> --description <d>   # capture intent before coding
provasign intent list                                   # list all intents (laptop: file-backed)
provasign intent get-captured --id <id>                # show a captured intent
provasign intent close --id <id>                       # commit draft intent
```

### Intent intake (Phase 1 team features)

```bash
provasign serve [--port 9000]            # start HTTP server + dashboard
provasign intent create <description> --project <name>
provasign intent approve <id>
provasign intent reject <id>
provasign intent from-jira <ticket-id>
provasign intent from-github <owner/repo#number>
provasign repo add --name <name> <url>
provasign project add <name> --repo <repo-name>
provasign project link <project> jira <board-key>
provasign project link <project> github_issues <owner/repo>
```

---

## Certification Pipeline

```
agent writes code
      │
      ▼
provasign_check  (fast, in-loop — sub-10s)
  ├── SAST on changed files (semgrep, gitleaks, inline-secrets)
  ├── Grove-affected unit tests only
  └── structured findings → agent self-corrects

      │  agent satisfied
      ▼
provasign_certify / provasign certify  (full, at commit time)
  │
  ├── Stage 1: build + full test suite (git worktree isolation)
  │     └── coverage-of-changed-symbols gate (vs Grove /tests edges)
  │
  ├── Stage 2: static analysis
  │     ├── inline-secrets (always available)
  │     ├── gitleaks (secrets)
  │     ├── semgrep (OWASP ruleset + custom .provasign/rulesets/)
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
provasign cert show HEAD
# Certificate: provasign-cert-abc123
# Intent-ID:   INT-0042
# Agent:        claude-sonnet-4-6
# Stage1:       PASS (42 tests, 87.3% coverage)
# Stage2:       PASS (0 HIGH, 1 MEDIUM suppressed by policy)
# Risk:         0.12 (low) — model v1
# Admitted:     a1b2c3d (provasign-main)
# Signed-By:    ~/.provasign/keys/admission.ed25519

provasign cert show --jsonld HEAD    # JSON-LD export for AI Code Passport / audit systems
provasign cert replay HEAD           # re-run gates against current tools → byte_reproducible
```

---

## Built-in Profiles

Provasign ships 6 profiles for `provasign init --profile=<name>`:

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

Provasign calls Grove at three points:

| Phase | Endpoint | Purpose |
|-------|----------|---------|
| Intake: GS scoring | `POST /icr` | Symbol count → is intent scoped tightly enough? |
| Certification: test selection | `POST /impact` + `POST /tests` | Which tests cover the changed symbols? |
| Certification: blast radius | `POST /impact` + `POST /deps` | What else might break? |

Provasign auto-starts Grove if unreachable at startup (`GROVE_URL` health check → `exec grove serve --port 7777`).

---

## Security

- All HTTP servers bind to `127.0.0.1` — no LAN exposure
- Bearer token at `.provasign/.token` (mode 0600) required on all `/api/*` routes
- Webhook routes validated with HMAC-SHA256 before any processing
- Ed25519 keypair at `~/.provasign/keys/admission.ed25519` (mode 0600), generated on `provasign init`
- Credentials via environment variables only — never in `.provasign/provasign.yaml`

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

See [docs/architecture.md](docs/architecture.md) and [docs/design.md](docs/design.md) for the full system design.
