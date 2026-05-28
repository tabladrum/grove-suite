# Grove Suite — Architecture

## Dependency Map

```
┌──────────────────────────────────────────────────┐
│                   Suite Boundary                 │
│                                                  │
│   ┌────────────────────────────────────────┐     │
│   │               Grove                   │     │
│   │  - Tree-sitter parser (7 languages)   │     │
│   │  - SQLite graph (WAL + FTS5)          │     │
│   │  - 8 MCP tools (grove_*)              │     │
│   │  - HTTP API  :7777                    │     │
│   │  - gRPC API  :7778                    │     │
│   └───────────┬────────────────────────────┘     │
│               │  HTTP/gRPC                        │
│      ┌────────┴───────┬──────────────┐            │
│      ▼                ▼              ▼            │
│  ┌───────┐       ┌───────┐     ┌──────────┐      │
│  │ Prism │       │  Fuse │     │  Relay   │      │
│  │ :8888 │       │ (git  │     │  :9000   │      │
│  │ MCP   │       │  driver)    │  gRPC    │      │
│  └───────┘       └───────┘     └──────────┘      │
└──────────────────────────────────────────────────┘
```

## Inter-Product API Contracts

### Grove HTTP API (consumed by Prism, Fuse, Relay)

| Endpoint | Method | Consumer |
|----------|--------|----------|
| `/health` | GET | Prism, Fuse, Relay (startup check) |
| `/index` | POST `{"dir": string}` | Prism, Fuse, Relay |
| `/query` | POST `{"intent": string, "limit": int}` | Prism |
| `/impact` | POST `{"file": string, "line": int}` | Fuse, Relay |
| `/deps` | POST `{"file": string}` | Fuse, Prism, Relay |
| `/symbols` | POST `{"query": string}` | Prism, Fuse |
| `/status` | GET | Prism, Relay |

### Grove startup contract (all consumers)

All three products implement identical startup logic:
1. Check `GROVE_URL/health` (default `http://localhost:7777`)
2. If unreachable: `exec grove serve --port 7777`
3. Wait ≤ 10 s; if still unreachable → fatal exit with setup instructions

## Versioning Policy

- Each product follows independent semantic versioning
- Grove API is stable within a minor version; breaking changes bump major
- Products declare Grove compatibility in their `go.mod`:
  ```
  require github.com/org/grove v0.X.Y
  ```
- Suite-level milestones are tracked in [Product-Roadmap.md](Product-Roadmap.md)

## Repo Layout

| Folder | Git Repo | Module path |
|--------|----------|-------------|
| `suite/` | `github.com/org/grove-suite` | docs only |
| `grove/` | `github.com/org/grove` | `github.com/org/grove` |
| `prism/` | `github.com/org/prism` | `github.com/org/prism` |
| `fuse/` | `github.com/org/fuse` | `github.com/org/fuse` |
| `relay/` | `github.com/org/relay` | `github.com/org/relay` |

## Data Flow: AI agent using Prism + VS Code

```
User types task in Copilot Chat
        │
        ▼
VS Code Copilot Agent
        │ vscode.lm.registerTool call
        ▼
Prism VS Code Extension (TypeScript)
        │ child_process.spawn("prism")
        ▼
prism binary (Go)
        │ HTTP GET grove:7777/query
        ▼
grove binary (Go)
        │ SQLite FTS5 + graph BFS
        ▼
Symbol set → ranked → budget-selected → compressed
        │
        ▼
Token-optimized context returned to Copilot
```

## Data Flow: Git merge with Fuse

```
git merge <branch>
        │ .gitattributes merge=fuse
        ▼
fuse merge <base> <ours> <theirs> <path>
        │ grove:7777/impact + /deps
        ▼
IntelliMerge 7-phase pipeline
        │
        ├── auto-resolved → writes merged file, exit 0
        └── unresolvable → writes conflict markers + AI handoff prompt, exit 1
```
