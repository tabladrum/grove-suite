# Grove Suite

A suite of four developer tools that share a common code intelligence foundation.

## Why This Exists

AI coding agents are only as good as the context they receive. Feed them too little and they hallucinate; feed them too much and they overflow their context window or lose focus. The problem compounds as codebases grow: an agent working on an authentication service needs to know about the session store and the user model, not about the unrelated payment module two directories away.

The grove suite attacks this problem at the infrastructure level. Grove builds and maintains a persistent knowledge graph of your codebase — symbols, edges between them, and the ability to traverse those edges quickly. Everything else in the suite is a consumer of that graph.

```
┌────────────────────────────────────────────────────────────────┐
│                        Grove Suite                             │
│                                                                │
│   ┌──────────────────────────────────────────────────────┐    │
│   │                        Grove                         │    │
│   │  Tree-sitter parser · SQLite graph · BFS traversal  │    │
│   │  11 languages · 8 edge types · MCP + HTTP + gRPC    │    │
│   └───────────────────────┬──────────────────────────────┘    │
│                           │  HTTP :7777 / gRPC :7778           │
│              ┌────────────┼────────────────┐                   │
│              ▼            ▼                ▼                   │
│         ┌────────┐   ┌────────┐    ┌───────────┐              │
│         │ Prism  │   │  Fuse  │    │   Relay   │              │
│         │ :8888  │   │  (git  │    │   :9000   │              │
│         │  MCP   │   │ driver)│    │   gRPC    │              │
│         └────────┘   └────────┘    └───────────┘              │
└────────────────────────────────────────────────────────────────┘
```

## The Four Products

### Grove — Code Knowledge Graph
**MIT license · Port 7777 (HTTP), 7778 (gRPC)**

Grove indexes a codebase into a persistent SQLite graph. It extracts symbols from source files using Tree-sitter AST walkers (with regex fallback for partial/broken files), links them with eight edge types (defines, contains, imports, extends, implements, calls, uses-type, tests), and exposes the graph through a CLI, HTTP API, MCP stdio server, and gRPC service.

Grove is the only product in the suite with its own storage. All others delegate graph operations to it.

[Full documentation →](grove/README.md)

### Prism — Context Delivery
**MIT license · Port 8888 (optional HTTP)**

Prism sits between your AI agent and Grove. It receives a task description, queries the knowledge graph, ranks results across five signals (graph distance, semantic similarity, recency, test relevance, edit frequency), allocates tokens across five budget categories, and applies progressive disclosure (full source → signature → reference) to maximize information density within a token budget.

The result is context that is scoped to what matters for the current task, not a raw dump of the files a human would open.

[Full documentation →](prism/README.md)

### Fuse — Semantic Merge Driver
**MIT license · No port**

Fuse replaces `git`'s line-level merge algorithm with a symbol-aware one. When Git delegates a merge to Fuse, it parses all three versions of the file using Tree-sitter, extracts symbols, queries Grove for cross-file blast radius, then runs a 7-phase IntelliMerge pipeline. Simple conflicts (incremental changes to different symbols) are resolved automatically. Complex ones produce conflict markers plus an AI-ready handoff prompt in `.git/fuse/conflict-<hash>.md`.

[Full documentation →](fuse/README.md)

### Relay — Certified Delivery for Coding Agents
**MIT license · Port 9000 (gRPC)**

Relay is the certified delivery layer for autonomous coding agents. It accepts ChangeSets from any agent (Claude Code, Cursor, Devin, Copilot Workspace, internal scripts), runs Grove-driven certification (impact analysis, intelligent test selection, policy gates), resolves multi-agent conflicts via Fuse semantic merge, and admits the result to a linear branch with a cryptographic certificate and full audit trail. Designed to work alongside GitHub today; makes branches unnecessary for approved classes of agent-delivered work as confidence builds.

## Dependency Order

Grove has no suite dependencies. Prism, Fuse, and Relay each require a running Grove instance and auto-start one at `$GROVE_URL` (default `http://localhost:7777`) if unreachable on startup.

**Build Grove first.**

```bash
cd grove && make install   # required by everything else

cd prism && make install   # context delivery for AI agents
cd fuse  && make install   # semantic git merge driver
```

## Repository Layout

```
grove-suite/
├── grove/          Go module — code knowledge graph
├── prism/          Go module — context delivery layer
│   └── vscode-extension/   TypeScript — VS Code integration
├── fuse/           Go module — semantic merge driver
├── Architecture.md           inter-product API contracts
└── Product-Roadmap.md        coordinated milestones
```

Each product is an independent Go module with a consistent `Makefile`:

```bash
make build    # compile binary
make test     # run all tests
make lint     # lint
make install  # install to $GOPATH/bin
```

## Security Model

All HTTP servers bind to `127.0.0.1` only — no LAN exposure. Grove generates a shared secret token at `.grove/.token` (0600 permissions) on first start and requires `Authorization: Bearer <token>` on every request except `/health`. Prism reads the token from the same file automatically.
