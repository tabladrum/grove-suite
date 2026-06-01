---
title: Relay
layout: default
nav_order: 6
description: "Relay — certified delivery for AI coding agents. Every commit signed, tested, and traceable to the prompt that created it."
permalink: /relay/
---

# Relay

**Certified delivery for AI coding agents. Every commit signed, tested, and traceable to the prompt that created it.**
{: .fs-5 .fw-300 }

[Install Relay](/installation){: .btn .btn-primary .fs-5 .mb-4 .mb-md-0 .mr-2 }
[View source](https://github.com/tabladrum/grove-suite/tree/main/relay){: .btn .fs-5 .mb-4 .mb-md-0 }

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

**Relay moves the quality gate into the agent loop.**

The agent calls `relay_check` every iteration — sub-10 seconds, structured findings (file, line, rule, severity, fix-hint) returned directly. The agent self-corrects before it commits. Before it opens a PR. Before anyone waits for CI.

When the code is ready, `relay_certify` runs the full suite: build, tests, coverage, secrets scanning, SAST, dependency audit. It signs the result with Ed25519 and admits it as a linear commit — with cryptographic proof that those specific gates passed at that specific toolchain version, and with the original user prompt committed alongside the code as a YAML intent linked via `Intent-ID:` trailer.

---

## What Changes When You Add Relay

**Before:**

```
Prompt → agent codes → opens PR → CI finds issue → back to agent
→ agent fixes → CI finds another issue → repeat 3–5×
→ human reviews diff → 6 months later: nobody knows what the prompt was
```

**After:**

```
Prompt → intent captured as YAML → agent codes with Prism context
→ relay_check (sub-10s) → agent self-corrects → relay_certify
→ Ed25519-signed cert: build ✓ tests ✓ coverage ✓ SAST ✓
→ linear commit: Intent-ID: INT-0042, Certificate: relay-cert-abc123
→ PR already certified — CI is a formality, not a gatekeeper
→ 6 months later: relay cert show INT-0042 → full picture
```

---

## Capabilities

| # | Capability | How it works |
|---|-----------|--------------|
| 1 | **In-loop pre-flight** | `relay_check`: SAST + Grove-affected unit tests. Sub-10s. Structured findings returned to the agent. |
| 2 | **Full certification** | `relay_certify`: Stage 1 (build + tests + coverage) + Stage 2 (secrets, SAST, deps, linters). |
| 3 | **Signed admission** | Linear commit with Ed25519 signature over ChangeSet + config hash + toolchain + results. |
| 4 | **Risk heatmap** | ICR (0.30) + Stage 2 severity (0.30) + coverage delta (0.25) + touch intensity (0.15). Versioned. |
| 5 | **Cryptographic audit** | `relay cert replay <id>` returns `byte_reproducible` / `tool_drift` / `config_drift`. |
| 6 | **Intent trail** | User's prompt committed as YAML intent before coding starts. Linked via `Intent-ID:` trailer. |
| 7 | **Agent wiring** | `relay init` auto-writes Pre-Flight Autopilot to CLAUDE.md / .cursorrules / .github/copilot-instructions.md / AGENTS.md / GEMINI.md / .clinerules. |
| 8 | **Batteries-included** | Ships semgrep, gitleaks, govulncheck, eslint, ruff pre-bundled. SonarQube profile import. |
| 9 | **Policy profiles** | `soc2-baseline`, `pci-dss-baseline`, stack-strict variants. Per-gate `warn` / `enforce` / `off`. |

---

## Certification Pipeline

```
agent writes code
      │
      ▼
relay_check  (fast, in-loop — sub-10s)
  ├── SAST on changed files (semgrep, gitleaks, inline-secrets)
  ├── Grove-affected unit tests only
  └── structured findings → agent self-corrects

      │  agent satisfied
      ▼
relay_certify  (full, at commit time)
  │
  ├── Stage 1: build + full test suite (git worktree isolation)
  │     └── coverage-of-changed-symbols gate (vs Grove /tests edges)
  │
  ├── Stage 2: static analysis
  │     ├── inline-secrets (always available)
  │     ├── gitleaks (secrets)
  │     ├── semgrep (OWASP + custom .relay/rulesets/)
  │     ├── govulncheck / npm audit / pip-audit (deps)
  │     └── eslint / ruff / golangci-lint (language linters)
  │
  ├── Risk heatmap (versioned)
  │
  ├── Policy gates (path, secrets, fileclass, deps, size, coverage)
  │
  └── Admission
        ├── rebase onto target branch
        ├── Ed25519 sign (CanonicalBytes, commit SHA excluded)
        ├── linear commit with full trailer:
        │     Intent-ID: · Agent: · Model: · Certificate: · ICR-Hash:
        │     Test-Plan: · Policy-Version: · Toolchain-Image: · Signed-By:
        └── certificate persisted to store + intent-store git
```

---

## Certificate Format

Every certificate is an Ed25519-signed record. The admitted commit SHA is added to the trailer *after* signing — so the cert is valid before the commit exists.

```bash
relay cert show HEAD
# Certificate: relay-cert-abc123
# Intent-ID:    INT-0042
# Agent:        claude-sonnet-4-6
# Stage1:       PASS (42 tests, 87.3% coverage)
# Stage2:       PASS (0 HIGH, 1 MEDIUM suppressed by policy)
# Risk:         0.12 (low) — model v1
# Admitted:     a1b2c3d (relay-main)
# Signed-By:    ~/.relay/keys/admission.ed25519

# Re-verify any commit at any time
relay cert verify <id>     # signature check
relay cert replay  <id>    # re-run gates → byte_reproducible / tool_drift / config_drift

# JSON-LD export for audit systems (AI Code Passport)
relay cert show --jsonld HEAD
```

---

## Works With

| Tool | Integration |
|------|------------|
| **Claude Code** | `relay mcp install-for claude-code` |
| **GitHub Copilot** | Detected by `relay init`, MCP config auto-written |
| **Cursor** | `relay mcp install-for cursor` |
| **Codex CLI** | Detected by `relay init` |
| **Windsurf** | `relay mcp install-for windsurf` |
| **Continue** | `relay mcp install-for continue` |
| **VS Code** (any agent extension) | Auto-detected by `relay init` |

`relay init` writes Pre-Flight Autopilot instructions to your repo's agent files (CLAUDE.md, .cursorrules, .github/copilot-instructions.md, AGENTS.md, GEMINI.md, .clinerules) — the agent reads these on startup and calls `relay_check` automatically.

---

## Quick Start (Laptop Mode — Zero Infrastructure)

```bash
# Install Grove first, then Relay
cd grove-suite/grove && make install && cd ..
cd grove-suite/relay && make install && cd ..

# Initialize in your project
cd /your/project
relay init --stack=go-microservice
# Scaffolds .relay/, generates local Ed25519 key,
# writes agent steering instructions + MCP configs for every detected tool

# Install the git pre-push backstop
relay hook install

# Pre-download all analyzer binaries NOW — avoids silent skips and delays
# on first relay_check or pre-push hook invocation.
relay tools install                   # gitleaks, govulncheck, golangci-lint (~30 MB)
relay tools install --with-sonar      # optional: adds JRE + SonarLint jars (~500 MB)
# Semgrep is a Python package — install separately if you want it:
# pipx install semgrep

# Commit the .relay/ config
git add .relay/ && git commit -m "Add Relay configuration"

# Your AI agent now calls relay_check before every PR. Automatically.
```

[Full installation guide →](/installation)

---

## Compliance & Audit

**SOC 2 Type II:** Relay's signed-admission flow maps directly to CC4.1, CC4.2 (control monitoring), CC6.1 (logical access), CC7.1 (vulnerability detection), and CC8.1 (change management).

**EU AI Act (high-risk activation August 2026):** Relay's intent-capture + signed-admission produces the traceability artefact the regulation requires — what the AI was asked to do, what gates verified the output, and a cryptographic proof you can show an auditor.

**The audit triple every Relay-admitted commit carries:**

1. **The prompt** — committed as a YAML intent in `.relay/intents/`
2. **The proof** — Ed25519-signed certificate over ChangeSet + config + toolchain + results
3. **The replay** — `relay cert replay <id>` re-runs gates against current tools and tells you what still matches

---

## Built-in Profiles

```bash
relay init --profile=<name>
```

| Profile | What it enforces |
|---------|-----------------|
| `soc2-baseline` | secrets + fileclass enforced; audit log required |
| `pci-dss-baseline` | secrets + deps + fileclass enforced; strict coverage |
| `go-microservice-strict` | go vet + govulncheck + coverage ≥ 80% + fileclass |
| `node-api-strict` | eslint + npm audit + coverage ≥ 75% |
| `python-service-strict` | ruff + pip-audit + coverage ≥ 75% |
| `java-spring-strict` | checkstyle + pmd + coverage ≥ 80% |

---

## Deployment Modes

One binary, three modes — same config, same certificate format:

| Mode | State storage | Use case |
|------|--------------|----------|
| **Laptop** | SQLite + local Ed25519 key | Solo developer, zero infrastructure |
| **Team** *(roadmap)* | Postgres + Redis + KMS | Shared audit trail across developers |
| **Air-gapped** *(roadmap)* | On-prem Postgres + HSM | Regulated industries, no external network |

---

## Security

- All HTTP servers bind to `127.0.0.1` — no LAN exposure
- Bearer token at `.relay/.token` (mode 0600) required on all `/api/*` routes
- Webhook routes validated with HMAC-SHA256
- Ed25519 keypair at `~/.relay/keys/admission.ed25519` (mode 0600), generated on `relay init`
- Credentials via environment variables only — never in `.relay/relay.yaml`

---

## Read More

- [How Relay compares to CI/CD, CodeRabbit, Greptile, Sigstore, SLSA](/comparisons#relay-vs-other-delivery-tools)
- [Why Grove Suite exists](/why)
- [Full reference on GitHub](https://github.com/tabladrum/grove-suite/tree/main/relay)
- [Troubleshooting](/troubleshooting#relay)
