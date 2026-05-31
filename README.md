# Grove Suite

> **The infrastructure layer that makes AI coding agents production-safe.**

---

Every major AI coding tool is racing to be a better agent — write more code, handle bigger tasks, work faster. GitHub Copilot. Cursor. Claude Code. Devin. Codex CLI.

Nobody is building the infrastructure *beneath* those agents.

That gap is real and growing. When an AI agent writes code today:

- It works **blind** — it reads the files you pointed at, guesses at what else matters, and hallucinates the rest of your codebase
- Its output lands in a **PR queue** that a human must review at a pace that cannot scale with agent volume
- It **conflicts** with the three other agents your team is running simultaneously, because git's line-level diff knows nothing about function boundaries
- Six months later, nobody knows **why** that code was written — the prompt is gone, the session is gone, the audit trail never existed

Grove Suite is the infrastructure that fills this gap. Four tools, one shared foundation:

```
┌────────────────────────────────────────────────────────────────┐
│                        Grove Suite                             │
│                                                                │
│   ┌──────────────────────────────────────────────────────┐    │
│   │                        Grove                         │    │
│   │         Your codebase's persistent memory            │    │
│   │  Tree-sitter · SQLite graph · 11 languages           │    │
│   │  8 edge types · BFS traversal · MCP + HTTP + gRPC   │    │
│   └───────────────────────┬──────────────────────────────┘    │
│                           │                                    │
│              ┌────────────┼────────────────┐                   │
│              ▼            ▼                ▼                   │
│         ┌────────┐   ┌────────┐    ┌───────────┐              │
│         │ Prism  │   │  Fuse  │    │   Relay   │              │
│         │        │   │        │    │           │              │
│         │ Focused│   │Symbol- │    │Certified  │              │
│         │ context│   │aware   │    │admission  │              │
│         │ for any│   │merge   │    │+ audit    │              │
│         │ agent  │   │driver  │    │trail      │              │
│         └────────┘   └────────┘    └───────────┘              │
└────────────────────────────────────────────────────────────────┘
```

---

## The Four Products

### Grove — Your Codebase's Long-Term Memory
**MIT license · `:7777` (HTTP) · `:7778` (gRPC)**

Static search (grep, ctags, a language server) answers "where is this symbol defined?" Grove answers harder questions: "what does this function transitively call?", "which tests cover this method?", "what is the full blast radius of changing this interface?"

Those are the questions that matter for AI task planning, merge conflict resolution, and certification. Grove indexes your code into a persistent SQLite graph — 11 languages, 8 edge types, delta-aware (files whose git blob SHA hasn't changed are never re-parsed). The graph is queryable over CLI, HTTP API, MCP stdio, and gRPC.

Grove is the only product with its own storage. All others delegate graph operations to it.

[Full documentation →](grove/README.md)

---

### Prism — Focused Context for Any AI Agent
**MIT license · `:8888` (optional HTTP)**

An agent that gets bad context produces bad code — not because it's a bad agent, but because it's working blind. Prism sits between an agent and Grove, receives the task description, queries the knowledge graph, scores candidates across 5 signals (graph distance, semantic similarity, recency, test relevance, edit frequency), allocates a token budget, and delivers exactly what matters for the task at hand.

Typical savings: **35–92%** versus sending files manually. Works with Claude Code, GitHub Copilot (VS Code), Cursor, Codex CLI, Windsurf, Zed, and any MCP-capable tool. A VS Code extension provides the same 8 tools natively via `vscode.lm.registerTool` — no MCP server required.

[Full documentation →](prism/README.md)

---

### Fuse — Symbol-Aware Git Merge Driver
**MIT license · No port (invoked by git)**

When multiple agents — or agents and humans — touch the same file simultaneously, git declares a conflict. Except most of those conflicts aren't real: one agent changed `Login()`, another changed `validatePassword()`. Different symbols, adjacent lines. Git sees a conflict. Fuse sees two independent changes and resolves automatically.

Fuse replaces git's line-level merge with a 7-phase IntelliMerge pipeline: parse all three file versions with Tree-sitter, query Grove for cross-file blast radius, classify the conflict, choose a resolution strategy. Incremental conflicts resolve at ~85% auto-resolution rate. Unresolvable ones produce conflict markers plus an AI-ready handoff prompt at `.git/fuse/conflict-<hash>.md`.

[Full documentation →](fuse/README.md)

---

### Relay — Certified Delivery for Coding Agents
**MIT license · `:9000` (HTTP)**

41% of production code is AI-generated (Gartner: 60% by end of 2026). "The developer reviewed the PR" is becoming a fiction. PRs are too big, too fast, too many. And even when someone reviews, the audit trail is gone — nobody recorded what the agent was actually asked to do.

Relay is what the agent calls between writing code and pushing it. It runs build + test + static analysis locally, computes a risk heatmap, enforces policy gates, signs the result with Ed25519, and admits it as a linear commit. The original user prompt is captured as a YAML intent (`Intent-ID:` in every commit trailer) — not just the output, but the request that produced it. Every cert is byte-reproducible: `relay cert replay <id>` re-runs the gates and tells you whether the result still matches.

One binary: laptop mode (SQLite, local Ed25519 key, zero config) or team mode (Postgres + Redis + KMS) — same config, same certs.

[Full documentation →](relay/README.md)

---

## Quick Start

### The common path — VS Code with Copilot Agent

```bash
cd grove && make install
cd ../prism && make install

cd /your/project
prism init    # auto-detects VS Code, writes MCP config + steering instructions
              # restart VS Code — all #prism* tools appear in Copilot Agent mode
prism index
prism savings # verify token savings are accumulating
```

### Claude Code / Cursor / Codex CLI / Windsurf / Zed

```bash
cd grove && make install
cd ../prism && make install

cd /your/project
prism init    # detects your tool, writes .claude/mcp.json or equivalent
              # restart your tool to pick up the MCP server config
prism index
```

### Add symbol-aware merge resolution

```bash
cd fuse && make install

cd /your/project
fuse install                        # writes ~/.gitconfig merge driver entry
echo "*.go merge=fuse" >> .gitattributes
echo "*.ts merge=fuse" >> .gitattributes
echo "*.py merge=fuse" >> .gitattributes
```

### Add agent certification (laptop — zero infrastructure)

```bash
cd relay && make install

cd /your/project
relay init --stack=go-microservice  # scaffolds .relay/, generates Ed25519 key,
                                    # writes Pre-Flight Autopilot to CLAUDE.md /
                                    # .cursorrules / .github/copilot-instructions.md /
                                    # AGENTS.md / GEMINI.md / .clinerules / …
                                    # registers MCP for all detected tools
relay hook install                  # git pre-push backstop

git add .relay/ && git commit -m "Add Relay configuration"
```

---

## Build Order

Grove has no suite dependencies. Build it first.

```bash
cd grove && make install   # required by Prism, Fuse, Relay
cd prism && make install
cd fuse  && make install
cd relay && make install
```

Prism, Fuse, and Relay auto-start Grove if unreachable at `$GROVE_URL` (default `http://localhost:7777`).

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

See [Architecture.md](Architecture.md) for the full security model and inter-product API contracts.
