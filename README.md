# Grove Suite

A suite of four developer tools built on a shared code intelligence foundation.

## Why This Exists

AI coding agents are only as good as the context they receive and the gates that validate their output. Feed an agent too little context and it hallucinates. Feed it too much and it loses focus. Let it push without gates and code quality erodes, audit trails disappear, and you can't replay what the agent actually did.

Grove Suite attacks both problems at the infrastructure level:

- **Grove** builds and maintains a persistent knowledge graph of your codebase — symbols, edges, and fast traversal.
- **Prism** uses that graph to deliver token-optimized context to AI agents (35–92% savings over raw file delivery).
- **Fuse** uses that graph to merge agent-produced code changes at symbol granularity instead of line granularity.
- **Relay** uses that graph to certify agent-produced commits — running build, test, and static analysis locally, signing the result, and admitting it with a full audit trail.

```
┌────────────────────────────────────────────────────────────────┐
│                        Grove Suite                             │
│                                                                │
│   ┌──────────────────────────────────────────────────────┐    │
│   │                        Grove                         │    │
│   │  Tree-sitter parser · SQLite graph · BFS traversal  │    │
│   │  11 languages · 8 edge types · MCP + HTTP + gRPC    │    │
│   │  127.0.0.1:7777 (HTTP) · 127.0.0.1:7778 (gRPC)     │    │
│   └───────────────────────┬──────────────────────────────┘    │
│                           │  HTTP + shared-secret token        │
│              ┌────────────┼────────────────┐                   │
│              ▼            ▼                ▼                   │
│         ┌────────┐   ┌────────┐    ┌───────────┐              │
│         │ Prism  │   │  Fuse  │    │   Relay   │              │
│         │ :8888  │   │  (git  │    │   :9000   │              │
│         │  MCP   │   │ driver)│    │           │              │
│         └────────┘   └────────┘    └───────────┘              │
└────────────────────────────────────────────────────────────────┘
```

---

## The Four Products

### Grove — Code Knowledge Graph
**MIT license · `:7777` (HTTP) · `:7778` (gRPC)**

Grove indexes a codebase into a persistent SQLite graph. Tree-sitter AST walkers (11 languages) extract symbols, link them with 8 edge types, and store them with delta indexing by git blob SHA — unchanged files are never re-parsed. The graph is queryable via CLI, HTTP API, MCP stdio, and gRPC.

Grove is the only product with its own storage. All others delegate graph operations to it.

[Full documentation →](grove/README.md)

### Prism — Token-Optimized Context Delivery
**MIT license · `:8888` (optional HTTP)**

Prism sits between an AI agent and Grove. It receives a task description, queries the knowledge graph, ranks results across 5 signals (graph distance, semantic similarity, recency, test relevance, edit frequency), allocates tokens across 5 budget categories, and applies progressive disclosure (full → signature → reference) to maximize information density within a token budget.

Typical savings: 35–92% versus sending files manually. Works with Claude Code, GitHub Copilot (VS Code), Cursor, Codex CLI, Windsurf, Zed (MCP), and VS Code Copilot Agent (native extension).

[Full documentation →](prism/README.md)

### Fuse — Semantic Git Merge Driver
**MIT license · No port (invoked by git)**

Fuse replaces git's line-level merge with a symbol-aware one. When Git delegates to `fuse merge`, it parses all three file versions in-memory with Tree-sitter, extracts symbols, queries Grove for cross-file blast radius, then runs the 7-phase IntelliMerge pipeline. Incremental conflicts (different symbols, adjacent lines) resolve automatically. Complex ones produce conflict markers plus an AI-ready handoff prompt at `.git/fuse/conflict-<hash>.md`.

[Full documentation →](fuse/README.md)

### Relay — Certified Delivery for Coding Agents
**MIT license · `:9000` (HTTP)**

Relay is what the AI agent calls between writing code and pushing it. It runs build + test + static analysis locally, computes a risk heatmap, enforces policy gates, signs the result with Ed25519, and admits it as a linear commit with a full audit trail. The agent's original prompt is captured as a committed intent YAML (`Intent-ID:` in the commit trailer) — not just the output, but the request that produced it.

`relay init` auto-wires every detected AI tool in one step: it appends the **Pre-Flight Autopilot** workflow to `CLAUDE.md`, `.cursorrules`, `.github/copilot-instructions.md`, `AGENTS.md`, `GEMINI.md`, `.clinerules`, and more, then writes MCP server configs for Claude Code, GitHub Copilot / VS Code, Cursor, Codex CLI, and any of Windsurf / Zed / Claude Desktop / Kiro / Continue that are installed.

One binary: laptop mode (SQLite, local Ed25519 key, zero config) or team mode (Postgres + Redis + KMS) — same config, same certs.

[Full documentation →](relay/README.md)

---

## Quick Start

### Most common path — VS Code with Copilot Agent

```bash
# Install binaries
cd grove && make install
cd ../prism && make install

# Initialize Prism in your project — detects VS Code, writes languageModelTools config,
# writes Grove+Prism status items to left status bar, enables auto-index on save
cd /your/project
prism init
```

Restart VS Code. Grove and Prism status appear in the bottom-left status bar. All `#prism*` tools are available in Copilot Agent mode.

### Claude Code / GitHub Copilot / Cursor / Codex CLI / Windsurf / Zed

```bash
cd grove && make install
cd ../prism && make install

cd /your/project
prism init    # auto-detects your tool and writes MCP config
# restart your tool to pick up the MCP server config

prism index   # initial index
prism savings # verify token savings are accumulating
```

### Add git merge intelligence

```bash
cd fuse && make install

# In any git repo:
fuse install                           # writes ~/.gitconfig merge driver entry
echo "*.go merge=fuse" >> .gitattributes
```

### Add agent certification (laptop)

```bash
cd relay && make install

cd /your/project
relay init --stack=go-microservice     # scaffolds .relay/, generates Ed25519 key,
                                        # writes Pre-Flight Autopilot instructions to
                                        # CLAUDE.md / .cursorrules / .github/copilot-instructions.md
                                        # / AGENTS.md / GEMINI.md / .clinerules / …
                                        # registers MCP for Claude Code, Copilot, Cursor,
                                        # Codex CLI, VS Code, and any installed global tools
relay hook install                      # git pre-push backstop
git add .relay/ && git commit -m "Add Relay configuration"
```

---

## Dependency Order

Grove has no suite dependencies. Prism, Fuse, and Relay each require a running Grove instance and auto-start one if unreachable.

**Build Grove first.**

```bash
cd grove && make install   # required by Prism, Fuse, Relay

cd prism && make install   # context delivery
cd fuse  && make install   # merge driver
cd relay && make install   # certification
```

---

## Repository Layout

```
grove-suite/
├── grove/                    Go module — code knowledge graph
├── prism/                    Go module — context delivery
│   └── vscode-extension/     TypeScript — VS Code extension
├── fuse/                     Go module — semantic merge driver
├── relay/                    Go module — certified delivery
│   └── docs/                 Architecture, design, agent-prompt
├── astkit/                   Go module — shared AST utilities
├── Architecture.md           Inter-product API contracts and data flows
└── go.work                   Go workspace (all modules)
```

Each product has a consistent Makefile:

```bash
make build    # compile ./bin/<name>
make test     # run all tests
make install  # install to $GOPATH/bin
```

---

## Security Model

All HTTP servers bind to `127.0.0.1` — no LAN exposure. Grove generates a 64-char hex token at `.grove/.token` (mode 0600) from `crypto/rand` and requires `Authorization: Bearer <token>` on all non-health requests. Prism, Fuse, and Relay read this token automatically. Relay's Ed25519 admission key lives at `~/.relay/keys/admission.ed25519` (mode 0600).

See [Architecture.md](Architecture.md) for the full security model and data flows.
