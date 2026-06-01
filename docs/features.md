---
title: Features
layout: default
nav_order: 5
description: "Relay's capabilities: in-loop pre-flight, full certification, Ed25519-signed admission, policy gates, risk heatmap, intent trail, and agent wiring."
permalink: /features/
---

# Features
{: .no_toc }

1. TOC
{:toc}

---

## Capabilities

| # | Capability | How it works |
|---|---|---|
| 1 | **In-loop pre-flight** | `relay_check`: SAST + Grove-selected affected unit tests. Sub-10s. Structured findings returned to the agent. |
| 2 | **Full certification** | `relay_certify`: Stage 1 (build + tests + coverage) + Stage 2 (secrets, SAST, deps, linters). |
| 3 | **Signed admission** | Linear commit with an Ed25519 signature over the changeset + config hash + toolchain + results. |
| 4 | **Risk heatmap** | Versioned score: ICR + Stage 2 severity + coverage delta + touch intensity. |
| 5 | **Cryptographic audit** | `relay cert replay <id>` → `byte_reproducible` / `tool_drift` / `config_drift`. |
| 6 | **Intent trail** | The user's prompt committed as a YAML intent before coding starts, linked via `Intent-ID:` trailer. |
| 7 | **Agent wiring** | `relay init` writes Pre-Flight Autopilot instructions to CLAUDE.md / .cursorrules / .github/copilot-instructions.md / AGENTS.md / GEMINI.md / .clinerules. |
| 8 | **Batteries-included** | `relay tools install` fetches pinned analyzers on demand; SonarQube profile import included. |
| 9 | **Policy profiles** | `soc2-baseline`, `pci-dss-baseline`, stack-strict variants. Per-gate `warn` / `enforce` / `off`. |

---

## Policy gates

Every certification runs a set of gates; each returns `allow`, `warn`, or `deny`. The defaults merge with your `.relay/relay.yaml`.

| Gate | What it enforces |
|---|---|
| `path` | Deny-list of paths an admission may not touch |
| `secrets` | No credentials/keys introduced (gitleaks + inline scan) |
| `fileclass` | Sensitive file classes (CI config, infra, auth) require stricter handling |
| `deps` | Dependency audit — no known-vulnerable or disallowed packages |
| `size` | Change-size limits to keep admissions reviewable |
| `coverage` | Coverage of the *changed* symbols, measured against Grove's `tests` edges |

Discover what's active in a repo with `relay policy` (or the `relay_policy` MCP tool).

---

## Certificate format

Each certificate is an Ed25519-signed record. The signed bytes cover the changeset id, intent id, base SHA, ICR, policy results, effective config hash, policy version, toolchain, signer key id, and timestamp. The **admitted commit SHA is excluded** from the signature — the cert is valid before the commit exists, and the (commit → cert) mapping lives in the engine store.

```bash
relay cert verify <id>          # signature check
relay cert replay <id>          # re-run gates → byte_reproducible / tool_drift / config_drift
relay cert show --jsonld HEAD   # JSON-LD "AI code passport" for audit systems
```

---

## Built-in profiles

```bash
relay init --profile=<name>
```

| Profile | What it enforces |
|---|---|
| `soc2-baseline` | secrets + fileclass enforced; audit log required |
| `pci-dss-baseline` | secrets + deps + fileclass enforced; strict coverage |
| `go-microservice-strict` | go vet + govulncheck + coverage ≥ 80% + fileclass |
| `node-api-strict` | eslint + npm audit + coverage ≥ 75% |
| `python-service-strict` | ruff + pip-audit + coverage ≥ 75% |
| `java-spring-strict` | checkstyle + pmd + coverage ≥ 80% |

---

## Works with your agent

`relay init` detects installed tools and writes the right MCP config and steering instructions for each — no per-tool hand-editing.

| Tool | Integration |
|---|---|
| **Claude Code** | `relay mcp install-for claude-code` |
| **GitHub Copilot** (VS Code) | Detected by `relay init`, MCP config auto-written |
| **Cursor** | `relay mcp install-for cursor` |
| **Codex CLI** | Detected by `relay init` |
| **Windsurf** | `relay mcp install-for windsurf` |
| **Continue** | `relay mcp install-for continue` |
| **Any MCP-capable tool** | MCP stdio |

The agent reads the Pre-Flight Autopilot instructions on startup and calls `relay_check` automatically before opening a PR.

---

## MCP tools

| Tool | When the agent uses it |
|---|---|
| `relay_intent_open` | First — capture the user request as an intent |
| `relay_check` | Before every review request |
| `relay_explain` | On any verdict that isn't `allow` |
| `relay_certify` / `relay_submit` | Only after `relay_check` returns allowed |
| `relay_policy` | Discover which gates are active |
| `relay_intent_close` | When the task is complete |

---

## Read more

- [How It Works]({{ '/how-it-works/' | relative_url }})
- [Architecture]({{ '/architecture/' | relative_url }})
- [Installation]({{ '/installation/' | relative_url }}) · [Agent Setup]({{ '/setup/' | relative_url }})
