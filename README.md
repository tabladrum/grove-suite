# Grove Suite

**The missing infrastructure layer beneath AI coding agents.**

AI agents can write code. They have no way to certify it's safe before it hits your main branch. No product has built that — until Relay.

> **New?** Let your AI agent set everything up: `claude "Follow the setup instructions at https://tabladrum.github.io/grove-suite/assets/AGENT_SETUP_PROMPT.md"` — or paste [`AGENT_SETUP_PROMPT.md`](AGENT_SETUP_PROMPT.md) into any MCP-capable agent.

---

## The Core Problem

AI coding agents are now writing a significant fraction of production code — GitHub's own data shows 41% of code in Copilot-enabled repos is AI-generated. That number is accelerating.

Here's what nobody has built: **the delivery infrastructure on the other side.**

Every agent — Devin, Cursor, GitHub Copilot Workspace, Claude Code — still delivers through the same PR → CI loop designed for humans writing code one commit at a time. When an agent opens a PR, CI finds issues, you send it back, CI finds more. Each iteration costs human attention to triage. The agent has lost context by then. You loop 3–5 times on work that should have been right the first time.

More critically: when a security audit asks *what did the agent do, and who verified it was safe?* — the PR says "refactor auth," CI shows green, the agent session is gone, the original prompt is gone. There is no answer.

**Relay is the answer.**

---

## Relay — Certified Delivery for AI Agents

*No product in this space does what Relay does.*

**What it is:** An admission control layer that sits between your AI coding agent and your main branch. Before the agent's code touches main, Relay:

1. **Captures the original user prompt** as a signed YAML intent in your repo — the record of what the agent was asked to do
2. **Runs quality gates in the agent loop** — build, tests, coverage, secrets scan, SAST, dependency audit — before any commit, in under 10 seconds
3. **Issues an Ed25519-signed certificate** proving which gates ran, which versions of which tools, and what passed — permanently linked to the commit
4. **Enables audit replay** — `relay cert replay <id>` re-runs the exact same gates against a 6-month-old commit and tells you `byte_reproducible`, `tool_drift`, or `config_drift`

**What changes:** The agent self-corrects before opening a PR. The CI loop drops from 3–5 iterations to 0–1. Every commit has a cryptographic proof of quality that survives the agent session.

```bash
# What the agent sees, via MCP:
relay_intent_open   → captures the user's prompt as a YAML artifact
relay_check         → structured findings in < 10 s; agent self-corrects
relay_submit        → Ed25519-signed admission certificate issued
```

```bash
# What the auditor sees, six months later:
relay cert show <id>     → original prompt + gates + toolchain versions + signature
relay cert replay <id>   → re-run the gates; prove they still hold
```

[Relay README →](relay/README.md)

---

## The Foundation — Grove

Relay's quality gates aren't naive. They're informed by a persistent knowledge graph of your codebase.

**[Grove](grove/README.md)** indexes your source files into a SQLite graph — 11 languages, 8 edge types (defines, calls, imports, extends, implements, uses-type, tests, contains), BFS traversal. When Relay runs tests, Grove selects the tests that actually cover the changed symbols — not all tests, not a guess. When Relay runs impact analysis, Grove returns the real blast radius in milliseconds.

Grove is what makes Relay's certification *meaningful* instead of *mechanical*.

```bash
grove index .                    # 10K-file monorepo in 34s cold; delta re-index in ms
grove impact "validatePassword"  # blast radius across the entire codebase
grove tests "Login"              # tests that cover this symbol, directly or transitively
```

Grove is also the foundation for Prism and Fuse. Without it, they degrade to filename guessing and line-level merge.

---

## Prism — Context That Doesn't Waste Tokens

AI agents work better when they understand what code is actually relevant. They work poorly when they read everything and run out of context.

**[Prism](prism/README.md)** delivers graph-ranked context to any MCP-capable agent: the target symbols, their dependencies, their tests, and related documentation — ranked by graph distance, semantic similarity, recency, and edit frequency. On first reads, 35–92% fewer tokens. On re-reads within the same session, ~99% (SHA pointer instead of full source).

```bash
prism init      # auto-detects Claude Code / Copilot / Cursor / Codex / Windsurf / Zed
prism savings   # watch token reduction accumulate in real time
```

Prism is the fastest path to visible value — 5 minutes to install, visible savings on the first agent task.

---

## Fuse — When Two Agents Edit the Same File

As agent usage scales, so does parallel editing. Two agents touch the same file. Different functions. Adjacent lines. Git declares a conflict on code that never actually conflicted.

**[Fuse](fuse/README.md)** replaces git's line-based merge with symbol-level understanding. It parses the three-way merge as ASTs, resolves at the symbol boundary, and uses Grove to check for cross-file breaking changes. ~85% of false git conflicts auto-resolve.

```bash
fuse install                        # registers driver in ~/.gitconfig
echo "*.go merge=fuse" >> .gitattributes
# git merge now resolves symbol-level conflicts automatically
```

---

## How They Fit Together

```
┌─────────────────────────────────────────────────────────────────────┐
│                         AI Coding Agent                             │
│         (Claude Code · Cursor · Devin · Copilot Workspace)         │
└────────────────────────────┬────────────────────────────────────────┘
                             │  MCP tools
          ┌──────────────────┼──────────────────┐
          ▼                  ▼                  ▼
   ┌─────────────┐   ┌─────────────┐   ┌──────────────────────────┐
   │    Prism    │   │    Fuse     │   │         Relay            │
   │             │   │             │   │                          │
   │ Graph-ranked│   │ Symbol-level│   │ Pre-commit gates         │
   │ context     │   │ merge       │   │ Intent capture           │
   │ 35–92% less │   │ ~85% auto-  │   │ Ed25519 certificates     │
   │ tokens      │   │ resolve     │   │ Audit replay             │
   └──────┬──────┘   └──────┬──────┘   └────────────┬─────────────┘
          │                 │                        │
          └─────────────────┼────────────────────────┘
                            │  HTTP + bearer token
                            ▼
           ┌────────────────────────────────────────┐
           │               Grove                    │
           │  Tree-sitter · SQLite · BFS · Model2Vec │
           │  11 languages · 8 edge types           │
           └────────────────────────────────────────┘
                            │
                            ▼
                     Your codebase
```

---

## Documentation

| Who you are | What to read |
|-------------|--------------|
| **Developer** wanting to try it | [Get Started](#get-started) below |
| **Understanding the big picture** | [Why Grove Suite](docs/why.md) |
| **Evaluating against alternatives** | [Comparisons](docs/comparisons.md) — vs Copilot, Sourcegraph, CodeRabbit, Devin, Sigstore |
| **CISO / security evaluation** | [FAQ — Security section](docs/faq.md#for-security--ciso) |
| **Compliance / audit evidence** | [FAQ — Compliance section](docs/faq.md#for-compliance--audit) |
| **Top questions** | [FAQ](docs/faq.md) |
| **Inter-product API contracts** | [Architecture](Architecture.md) |
| **Full doc map** | [docs/](docs/README.md) |

---

## Get Started

### Fastest — let your agent do it

```bash
# In Claude Code:
claude "Follow the setup instructions at https://tabladrum.github.io/grove-suite/assets/AGENT_SETUP_PROMPT.md"
```

The agent detects your platform, fetches the latest release, asks which products you want, verifies checksums, and wires everything into your project.

### From source

```bash
git clone https://github.com/tabladrum/grove-suite && cd grove-suite

# Grove must go first — everything else depends on it
cd grove && make install && cd ..

# Then any combination of:
cd relay && make install && cd ..   # certified delivery — the main event
cd prism && make install && cd ..   # token-efficient context
cd fuse  && make install && cd ..   # symbol-level merge
```

### Initialize in your project

```bash
cd /your/project

# Relay — certified delivery + audit trail
relay init --stack=auto   # detects Go / Node / Python; generates Ed25519 key;
                          # writes agent instructions to CLAUDE.md, .cursorrules, etc.
relay hook install        # git pre-push backstop

# Prism — better context
prism init               # auto-detects your coding tool, writes MCP config
prism index              # initial index; restart your coding tool

# Fuse — symbol-level merge
fuse install
echo "*.go merge=fuse" >> .gitattributes
echo "*.ts merge=fuse" >> .gitattributes
```

---

## Performance

Benchmarks on real hardware (macOS, 2026-05-27):

| Project | Files | Grove index | BFS query | Relay pre-flight |
|---------|------:|------------:|----------:|-----------------:|
| Small | 61 | 0.06 s | 6 ms | < 10 s |
| Medium | 801 | 0.85 s | 6 ms | < 10 s |
| Large | 4,501 | 11.6 s | 9 ms | < 10 s |
| Monorepo | 9,901 | 34.0 s | 61 ms | < 10 s |

Delta indexing: after the first run, unchanged files are never re-parsed. One-file change on a 9,901-file repo: milliseconds.

---

## Repository Layout

```
grove-suite/
├── grove/              Go — persistent code knowledge graph (foundation)
├── relay/              Go — certified agent delivery (the main event)
├── prism/              Go — token-optimized context delivery
│   └── vscode-extension/  TypeScript — VS Code native extension
├── fuse/               Go — semantic git merge driver
├── astkit/             Go — shared AST utilities
├── Architecture.md     Inter-product contracts and data flows
└── go.work             Go workspace
```

Licensing: Grove, Prism, and Fuse are MIT. Relay is AGPL-3.0.

---

## Security

All HTTP servers bind to `127.0.0.1`. Grove generates a 64-char random bearer token at `.grove/.token` (mode 0600) required on every non-health request. Relay's Ed25519 admission key is at `~/.relay/keys/admission.ed25519` (mode 0600), generated once on `relay init`. Zero telemetry. Your code never leaves your machine.

See [Architecture.md](Architecture.md) for full inter-product API contracts and the security model.

---

*Built on [Tree-sitter](https://tree-sitter.github.io), [SQLite](https://sqlite.org), and [Model2Vec](https://github.com/MinishLab/model2vec). No cloud. No GPU. No subscription.*
