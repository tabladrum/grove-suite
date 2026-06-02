---
title: Features
layout: default
nav_order: 5
description: "Provasign's capabilities: in-loop pre-flight, full certification, Ed25519-signed admission, policy gates, risk heatmap, intent trail, and agent wiring."
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
| 1 | **In-loop pre-flight** | `provasign_check`: SAST + Grove-selected affected unit tests. Sub-10s. Structured findings returned to the agent. |
| 2 | **Full certification** | `provasign_certify`: Stage 1 (build + tests + coverage) + Stage 2 (secrets, SAST, deps, linters). |
| 3 | **Signed admission** | Linear commit with an Ed25519 signature over the changeset + config hash + toolchain + results. |
| 4 | **Risk heatmap** | Versioned score: ICR + Stage 2 severity + coverage delta + touch intensity. |
| 5 | **Cryptographic audit** | `provasign cert replay <id>` → `byte_reproducible` / `tool_drift` / `config_drift`. |
| 6 | **Intent trail** | The user's prompt committed as a YAML intent before coding starts, linked via `Intent-ID:` trailer. |
| 7 | **Agent wiring** | `provasign init` writes Pre-Flight Autopilot instructions to CLAUDE.md / .cursorrules / .github/copilot-instructions.md / AGENTS.md / GEMINI.md / .clinerules. |
| 8 | **Batteries-included** | `provasign tools install` fetches pinned analyzers on demand; SonarQube profile import included. |
| 9 | **Policy profiles** | `soc2-baseline`, `pci-dss-baseline`, stack-strict variants. Per-gate `warn` / `enforce` / `off`. |

---

## Policy gates

Every certification runs a set of gates; each returns `allow`, `warn`, or `deny`. The defaults merge with your `.provasign/provasign.yaml`.

| Gate | What it enforces |
|---|---|
| `path` | Deny-list of paths an admission may not touch |
| `secrets` | No credentials/keys introduced (gitleaks + inline scan) |
| `fileclass` | Sensitive file classes (CI config, infra, auth) require stricter handling |
| `deps` | Dependency audit — no known-vulnerable or disallowed packages |
| `size` | Change-size limits to keep admissions reviewable |
| `coverage` | Coverage of the *changed* symbols, measured against Grove's `tests` edges |

Discover what's active in a repo with `provasign policy` (or the `provasign_policy` MCP tool).

---

## Certificate format

Each certificate is an Ed25519-signed record. The signed bytes cover the changeset id, intent id, base SHA, ICR, policy results, effective config hash, policy version, toolchain, signer key id, and timestamp. The **admitted commit SHA is excluded** from the signature — the cert is valid before the commit exists, and the (commit → cert) mapping lives in the engine store.

```bash
provasign cert verify <id>          # signature check
provasign cert replay <id>          # re-run gates → byte_reproducible / tool_drift / config_drift
provasign cert show --jsonld HEAD   # JSON-LD "AI code passport" for audit systems
```

---

## Built-in profiles

```bash
provasign init --profile=<name>
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

`provasign init` detects installed tools and writes the right MCP config and steering instructions for each — no per-tool hand-editing.

| Tool | Integration |
|---|---|
| **Claude Code** | `provasign mcp install-for claude-code` |
| **GitHub Copilot** (VS Code) | Detected by `provasign init`, MCP config auto-written |
| **Cursor** | `provasign mcp install-for cursor` |
| **Codex CLI** | Detected by `provasign init` |
| **Windsurf** | `provasign mcp install-for windsurf` |
| **Continue** | `provasign mcp install-for continue` |
| **Any MCP-capable tool** | MCP stdio |

The agent reads the Pre-Flight Autopilot instructions on startup and calls `provasign_check` automatically before opening a PR.

---

## MCP tools

| Tool | When the agent uses it |
|---|---|
| `provasign_intent_open` | First — capture the user request as an intent |
| `provasign_check` | Before every review request |
| `provasign_explain` | On any verdict that isn't `allow` |
| `provasign_certify` / `provasign_submit` | Only after `provasign_check` returns allowed |
| `provasign_policy` | Discover which gates are active |
| `provasign_intent_close` | When the task is complete |

---

## Read more

- [How It Works]({{ '/how-it-works/' | relative_url }})
- [Architecture]({{ '/architecture/' | relative_url }})
- [Installation]({{ '/installation/' | relative_url }}) · [Agent Setup]({{ '/setup/' | relative_url }})
