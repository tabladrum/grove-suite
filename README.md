# Relay

**Certified delivery for AI coding agents.** Every AI-generated commit signed, tested, and traceable to the prompt that created it.

> Built on three open-source components — [Grove](grove/README.md) (code knowledge graph), [Prism](prism/README.md) (context delivery), [Fuse](fuse/README.md) (semantic merge) — all embedded into the Relay binary. No services to run. No ports to manage. One install.

```bash
# Let your agent install everything:
claude "Follow the setup instructions at https://tabladrum.github.io/grove-suite/assets/AGENT_SETUP_PROMPT.md"
```

---

## The Problem

AI coding agents are now writing a significant fraction of production code — GitHub reports 41% of code in Copilot-enabled repos is AI-generated, accelerating.

Nobody has built the delivery infrastructure on the other side.

Every agent — Devin, Cursor, Copilot Workspace, Claude Code — still pushes through a PR → CI loop designed for humans writing one commit at a time. CI catches issues, you send the work back, CI catches more. The agent has lost context. You loop 3–5 times on work that should have been right the first time.

More critically: when a security audit asks *what did the agent do, and who verified it was safe?* — the PR says "refactor auth," CI shows green, the agent session is gone, the original prompt is gone. There is no answer.

**Relay is the answer.**

---

## What Relay Does

An admission control layer between your AI agent and your main branch. Before agent-produced code touches main, Relay:

1. **Captures the original user prompt** as a signed YAML intent in your repo — the record of what the agent was asked to do.
2. **Runs quality gates in the agent loop** — build, tests, coverage, secrets scan, SAST, dependency audit — before any commit, in under 10 seconds.
3. **Issues an Ed25519-signed certificate** proving which gates ran, which tool versions, what passed — permanently linked to the commit.
4. **Enables audit replay** — `relay cert replay <id>` re-runs the same gates against a 6-month-old commit and tells you `byte_reproducible`, `tool_drift`, or `config_drift`.

The agent self-corrects before opening a PR. The CI loop drops from 3–5 iterations to 0–1. Every commit has a cryptographic proof of quality that survives the agent session.

```bash
# What the agent sees, via MCP:
relay_intent_open   → captures the user's prompt as a YAML artifact
relay_check         → structured findings in < 10 s; agent self-corrects
relay_submit        → Ed25519-signed admission certificate issued

# What the auditor sees, six months later:
relay cert show <id>     → original prompt + gates + toolchain versions + signature
relay cert replay <id>   → re-run the gates; prove they still hold
```

---

## Why Relay Is Different

| | Relay | CI (GitHub Actions, etc.) | CodeRabbit / Greptile | Sigstore / SLSA |
|---|---|---|---|---|
| Runs before the commit | Yes (in the agent loop) | No (after push) | No (after PR open) | No (build-time) |
| Captures the original prompt | Yes (signed YAML) | No | No | No |
| Replayable months later | Yes (`relay cert replay`) | No (logs expire) | No | Partial (provenance only) |
| Signed by your private key | Yes (Ed25519) | No | No | Yes |
| Knows your codebase graph | Yes (via Grove) | No | Partial | No |
| Local-first, no cloud | Yes | Cloud-dependent | Cloud | Hybrid |

Relay is the first tool to bind **prompt → certificate → commit** with cryptographic proof that survives the agent session.

[Full Relay docs →](relay/README.md)

---

## The Open-Source Foundation

Relay is built on three components. Each is useful on its own and independently licensed under MIT. You can adopt any of them without Relay.

### [Grove](grove/README.md) — Code Knowledge Graph

Tree-sitter parser, SQLite-backed graph (11 languages, 8 edge types), BFS traversal, FTS5 search, delta indexing by git blob SHA. The substrate Relay uses for impact analysis and test selection.

**Independent use:** `grove index .`, then `grove impact "validatePassword"`, `grove tests "Login"`. MIT licensed. Embed it in your own tools — Grove is a Go library, not a daemon.

### [Prism](prism/README.md) — Token-Optimized Context for AI Agents

Graph-ranked context delivery: target symbols, dependencies, tests, and docs ranked by graph distance, semantic similarity, recency, test relevance, and edit frequency. 35–92% fewer tokens on first reads, ~99% on re-reads (SHA pointer instead of full source).

**Independent use:** `prism init` auto-detects Claude Code / Copilot / Cursor / Codex / Windsurf / Zed and wires the MCP server. 5 minutes to install, visible savings on the first agent task. MIT licensed.

### [Fuse](fuse/README.md) — Semantic Git Merge Driver

Replaces git's line-based merge with symbol-level understanding. Parses the three-way merge as ASTs, resolves at the symbol boundary, queries Grove for cross-file breaking changes. ~85% of false git conflicts auto-resolve.

**Independent use:** `fuse install`, then `*.go merge=fuse` in `.gitattributes`. MIT licensed.

---

## Architecture

```
                  AI Coding Agent
        (Claude Code · Cursor · Copilot · Devin)
                        │
                        │  MCP
                        ▼
              ┌──────────────────┐
              │      Relay       │  ← the product
              │  Intent capture  │
              │  Quality gates   │
              │  Ed25519 certs   │
              │  Audit replay    │
              └─────────┬────────┘
                        │  in-process (embedded)
        ┌───────────────┼───────────────┐
        ▼               ▼               ▼
     Grove           Prism            Fuse
   (knowledge      (context         (semantic
     graph)         delivery)         merge)
        │               │               │
        └───────────────┼───────────────┘
                        ▼
                 Your codebase
```

**No daemons. No ports. No tokens.** Grove is compiled into Relay (and into Prism and Fuse when used standalone) as a Go library. Index data lives in `.grove/` per repo. SQLite handles concurrent readers.

---

## Get Started

### Fastest — let your agent do it

```bash
claude "Follow the setup instructions at https://tabladrum.github.io/grove-suite/assets/AGENT_SETUP_PROMPT.md"
```

The agent detects your platform, fetches the latest release, verifies checksums, and wires Relay into your project.

### Manual install

```bash
# Pre-built binaries (macOS, Linux, Windows):
curl -fsSL https://tabladrum.github.io/grove-suite/assets/install.sh | bash

# Or from source:
git clone https://github.com/tabladrum/grove-suite && cd grove-suite/relay && make install
```

### Initialize in your project

```bash
cd /your/project
relay init --stack=auto   # detects Go/Node/Python; generates Ed25519 key;
                          # writes agent instructions into CLAUDE.md, .cursorrules, etc.
relay hook install        # git pre-push backstop
```

Relay's MCP server is registered with every AI coding tool installed on your machine. Your agent's next session has `relay_intent_open`, `relay_check`, `relay_submit` available — and uses them automatically per the instructions in `CLAUDE.md`.

### Adopt the OSS components independently (optional)

```bash
# Better context for any AI agent, no Relay needed:
prism init && prism index

# Symbol-level git merge, no Relay needed:
fuse install
echo "*.go merge=fuse" >> .gitattributes
```

---

## Performance

Benchmarks on real hardware (macOS, 2026-05-27):

| Project | Files | Index (cold) | BFS query | Relay pre-flight |
|---------|------:|------------:|----------:|-----------------:|
| Small | 61 | 0.06 s | 6 ms | < 10 s |
| Medium | 801 | 0.85 s | 6 ms | < 10 s |
| Large | 4,501 | 11.6 s | 9 ms | < 10 s |
| Monorepo | 9,901 | 34.0 s | 61 ms | < 10 s |

Delta indexing: after the first run, unchanged files are never re-parsed. One-file change on a 9,901-file repo: milliseconds.

---

## Documentation

| Who you are | What to read |
|-------------|--------------|
| Developer wanting to try Relay | [Get Started](#get-started) above |
| Security / CISO evaluation | [FAQ — Security](docs/faq.md#for-security--ciso) · [Audiences: Security](docs/audiences/security.md) |
| Compliance / audit evidence | [FAQ — Compliance](docs/faq.md#for-compliance--audit) · [Audiences: Audit](docs/audiences/audit.md) |
| Adopting Grove / Prism / Fuse on their own | [Grove](grove/README.md) · [Prism](prism/README.md) · [Fuse](fuse/README.md) |
| Comparisons | [vs Copilot, Sourcegraph, CodeRabbit, Devin, Sigstore](docs/comparisons.md) |
| Architecture & internals | [Architecture.md](Architecture.md) |
| Full doc map | [docs/](docs/README.md) |

---

## Repository Layout

```
grove-suite/
├── relay/              the product — certified delivery (AGPL-3.0)
├── grove/              code knowledge graph (MIT) — also embedded in Relay
├── prism/              token-optimized context for AI agents (MIT)
│   └── vscode-extension/  VS Code native extension
├── fuse/               semantic git merge driver (MIT)
├── astkit/             shared AST utilities (MIT)
├── Architecture.md     inter-component contracts and data flows
└── go.work             Go workspace
```

**Licensing:** Relay is AGPL-3.0. Grove, Prism, and Fuse are MIT — adopt them independently in commercial products without obligation.

---

## Security

- Relay's Ed25519 admission key is at `~/.relay/keys/admission.ed25519` (mode 0600), generated once on `relay init`.
- Grove runs in-process as a library; index data is in `.grove/` per repo. No network ports, no shared secrets.
- Zero telemetry. Your code never leaves your machine.

See [Architecture.md](Architecture.md) for the full security model.

---

*Built on [Tree-sitter](https://tree-sitter.github.io), [SQLite](https://sqlite.org), and [Model2Vec](https://github.com/MinishLab/model2vec). No cloud. No GPU. No subscription.*
