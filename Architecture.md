# Architecture

**The product is Provasign** — certified delivery for AI coding agents. Grove, Prism, and Fuse are the open-source foundation Provasign is built on; each is also usable on its own.

This document describes the **embedded architecture**: Grove is a Go library (`grove/pkg/grove`) linked directly into Provasign, Prism, and Fuse and called in-process. There is **no `grove serve` daemon, no HTTP/gRPC port, no `$GROVE_URL`, and no shared-secret token**. Each consumer opens the on-disk index at `<repo>/.grove/grove.db` directly via SQLite (WAL mode handles concurrent readers).

> Looking for the old client/server design (`:7777`, `:7778`, bearer tokens)? It was the v1 model and has been removed. See the git history of this file if you need it.

---

## Component Map

```
                         AI coding agent
              (Claude Code · Cursor · Copilot · Codex · …)
                               │
                               │  MCP over stdio
                               │  (newline-delimited JSON-RPC 2.0)
        ┌──────────────────────┼───────────────────────────┐
        ▼                      ▼                            ▼
   ┌─────────┐           ┌──────────┐                 ┌──────────┐
   │  RELAY  │           │  PRISM   │                 │   FUSE   │
   │ the     │           │ context  │                 │ semantic │
   │ product │           │ delivery │                 │  merge   │
   │ AGPL-3  │           │  MIT     │                 │   MIT    │
   └────┬────┘           └────┬─────┘                 └────┬─────┘
        │  import grove/pkg/grove (in-process Go calls)    │
        └───────────────────────┬──────────────────────────┘
                                 ▼
                       ┌────────────────────┐
                       │       GROVE        │  Go library (MIT)
                       │ Tree-sitter parser │  + standalone `grove` CLI
                       │ SQLite graph (WAL) │  + `grove mcp` stdio server
                       │ BFS · FTS5 · embed │
                       └─────────┬──────────┘
                                 ▼
                        <repo>/.grove/grove.db
```

- **Provasign** is the delivery platform — intent capture, in-loop quality gates, Ed25519 admission certificates, audit replay. It is the commercial product (AGPL-3.0).
- **Prism** and **Fuse** are standalone MIT tools that each embed Grove. You can adopt either without Provasign.
- **Grove** is the shared engine — a Go library plus a standalone `grove` CLI and a `grove mcp` stdio server for direct human/agent use.

**Dependency / build order:** build Grove first; it produces the library the other three link against.

---

## Inter-Component API — the Go library (`grove/pkg/grove`)

Provasign, Prism, and Fuse consume Grove entirely in-process. No network, no auth, no ports.

```go
import "github.com/provasign/provasign/grove/pkg/grove"

eng, err := grove.Open(ctx, grove.Config{RepoRoot: "/path/to/repo"})
if err != nil { /* ... */ }
defer eng.Close()                       // closes SQLite handles — see "Lifecycle"

res, err := eng.Query(ctx, grove.QueryRequest{Intent: "login flow", Limit: 20})
imp, err := eng.Impact(ctx, grove.ImpactRequest{File: "auth.go", Line: 42})
```

| Method | Purpose | Consumers |
|--------|---------|-----------|
| `Index(ctx, dir)` | Build/refresh the graph (delta-aware) | All |
| `Query(ctx, intent, limit)` | Intent → ranked symbols (FTS5 + BFS) | Prism, Provasign |
| `Impact(ctx, file, line)` | Blast radius of a change | Fuse, Provasign |
| `Deps(ctx, file)` | Cross-file dependency edges | Fuse, Prism, Provasign |
| `Symbols(ctx, query)` | Symbol lookup by name | Prism, Fuse |
| `Tests(ctx, symbol)` | Tests covering a symbol | Provasign |
| `Semantic(ctx, query)` | Embedding similarity | Prism |
| `Status(ctx)` | Index summary (files/symbols/edges) | Prism, Provasign |

### Lifecycle (important)

`grove.Open` acquires SQLite file handles under `<repo>/.grove/`. Every opener **must** call `Close()` (or the consumer's `client.Shutdown()`) before the process exits or before the repo directory is removed. A leaked engine keeps `grove.db`, `grove.db-wal`, and `grove.db-shm` open, which on Linux/Windows blocks directory removal and can corrupt WAL state. Long-lived servers (e.g. `prism mcp`) open the engine lazily in the background and close it when the stdio session ends.

---

## MCP transport

Provasign (`provasign mcp serve`), Prism (`prism mcp`), and the standalone `grove mcp` all speak **JSON-RPC 2.0 over stdio using the MCP stdio transport: newline-delimited JSON — one compact object per line, no `Content-Length` framing**. Emitting LSP-style `Content-Length` framing causes every newline-delimited client (Claude Code, Cursor, VS Code, Copilot) to block waiting for a terminating newline and time the connection out after ~30s. The `initialize` response echoes the client's requested `protocolVersion` when supported (`2024-11-05` / `2025-03-26` / `2025-06-18`), else falls back to `2025-03-26`.

---

## Grove — the engine

**Packages:**
- `pkg/grove/` — public API: `Engine` (`Index`, `Query`, `Impact`, `Deps`, `Symbols`, `Tests`, `Semantic`, `Status`). The stable surface Provasign/Prism/Fuse depend on.
- `internal/parser/` — Tree-sitter engine; language extractors in `strategies/`; all CGO isolated here.
- `internal/store/` — SQLite (WAL + FTS5); delta indexing skips files whose git blob SHA is unchanged.
- `internal/graph/` — in-memory `CodeGraph`, 8 edge types (defines, contains, imports, extends, implements, calls, uses-type, tests), BFS traversal.
- `internal/query/` — intent→symbols, blast radius, deps, test selection.
- `internal/mcp/` — the 8 MCP tools for the standalone `grove mcp` stdio server.
- `internal/embeddings/model2vec/` — potion-base-8M (29 MB) embedded via `go:embed`; pure-Go inference, no CGO.

**Key constraints:**
- Pure-Go SQLite (`modernc.org/sqlite`) — no CGO conflict with tree-sitter.
- `calls` / `uses-type` edges scoped to same-file + imported files only (unscoped → ~5× false positives).
- Delta indexing: never re-parse a file whose git blob SHA is unchanged.
- Symbol ID: `{filePath}::{qualifiedName}@{blobSHA}`.

---

## Data flows

### AI agent using Prism (MCP or VS Code extension)

```
Agent task ("add rate limiting to login")
   │  prism_query
   ▼
eng.Query / eng.Deps (in-process)
   ▼
5-signal ranking (graph distance · semantic · recency · test relevance · edit frequency)
   ▼
budget allocation (target 35% · deps 25% · tests 20% · doc 10% · summary 10%)
   ▼
progressive disclosure (full → signature → reference)
   ▼
session deduplication (LRU)
   ▼
token-optimized context pack → agent
```

### Git merge with Fuse

```
git merge <branch>          # .gitattributes: *.go merge=fuse
   ▼
fuse merge %O %A %B %P
   ├─ parse base/ours/theirs as ASTs (in-memory)
   ├─ eng.Impact / eng.Deps  → cross-file blast radius + breaking changes
   ▼
7-phase IntelliMerge: context → symbol extraction → recency →
graph context → breaking-change detection → conflict classification → strategy
   ├─ clean   → write merged file · exit 0
   └─ conflict→ conflict markers + .git/fuse/conflict-<sha>.md · exit 1
```

### Provasign certification pipeline (the product)

```
agent writes code
   ▼
provasign_intent_open → intent YAML captured (the prompt, recorded)
   ▼
provasign_check  (fast, in-loop, sub-10s target)
   └─ SAST on changed files + Grove-affected tests only
      structured findings (file, line, rule, severity, fix-hint) → agent
      agent self-corrects; loops up to 3× until Allowed=true
   ▼  (provasign_certify only after provasign_check Allowed=true)
provasign_certify
   ├─ Stage 1: build + full test suite (git worktree isolation)
   │   └─ coverage-of-changed-symbols gate (vs Grove tests edges)
   ├─ Stage 2: static analysis (semgrep, gitleaks, govulncheck, linters)
   ├─ Risk heatmap: ICR(0.30) + severity(0.30) + coverage(0.25) + touch(0.15)
   ├─ Policy gates (path, secrets, fileclass, deps, size, coverage)
   └─ Admission
        ├─ rebase onto target branch (linear main)
        ├─ Ed25519-sign CanonicalBytes
        ├─ linear commit with full trailer
        └─ certificate persisted + intent-store snapshot
   ▼
relay_intent_close → intent promoted + committed
```

---

## Provasign data model

```
Repo (id, name, url, default_branch)
  └── Project (many per repo — monorepo support)
        id, name, repo_id, source_path, gs_threshold, auto_approve, owner
        └── ProjectIntegration (M:M — jira | github_issues | github_projects | linear)
        └── Intent
              description, status, gs_score, icr_symbols
              source: jira | github_issue | native | mcp
        └── ChangeSet (intent_id, diff, agent_id, model_version)
        └── Certificate
              changeset_id, stage1_result, stage2_findings, risk_score
              signature (Ed25519), effective_config_hash
              admitted_commit_sha, admitted_branch
```

**Provasign's three git repos:** `source-repo` (application code, linear main), `intent-store` (YAML intents + audit trail), `platform-config` (policies). Redis, where used, is transient only (`appendonly no`) — all business state lives in git.

---

## Security model

- **Local-first, no cloud, zero telemetry.** Your code never leaves your machine. There are no network listeners in the embedded model.
- **Ed25519 admission key.** Provasign generates a keypair at `~/.provasign/keys/admission.ed25519` (mode 0600) on `provasign init`. Every admitted commit is signed over CanonicalBytes (config hash + ChangeSet + test results + findings). The admitted commit SHA is recorded in the trailer *after* signing — the certificate is valid before the commit exists.
- **No shared-secret tokens, no open ports.** The removed daemon's `127.0.0.1` binding and `.grove/.token` are gone; the embedded library has no attack surface beyond the local filesystem.
- **Threat boundary.** The model protects against network-adjacent and browser-based attacks by construction (nothing listens). It does not isolate against other local processes running as the same user — full isolation would need OS-level sandboxing, outside the scope of these tools.

---

## Module paths

```
github.com/provasign/provasign/grove     grove/go.mod   (engine: library + CLI)
github.com/provasign/provasign/prism     prism/go.mod   (MIT)
github.com/provasign/provasign/fuse      fuse/go.mod    (MIT)
github.com/provasign/provasign/provasign     provasign/go.mod   (AGPL-3.0 — the product)
github.com/provasign/provasign/astkit    astkit/go.mod  (shared AST utils)
```

`go.work` at the repo root references all modules.

---

## Key invariants

1. **Grove is the single source of graph truth.** No consumer rebuilds the symbol graph internally.
2. **Embedded only.** Provasign/Prism/Fuse call `grove/pkg/grove` in-process. No daemon, no ports, no tokens, no `$GROVE_URL`.
3. **Every engine opener closes it.** `Open` must be paired with `Close()`/`Shutdown()` before process exit or repo removal, or SQLite handles leak.
4. **MCP stdio is newline-delimited JSON.** No `Content-Length` framing — it breaks every MCP client.
5. **`initialize` echoes the client's protocol version** when supported, else falls back to `2025-03-26`.
6. **Delta indexing by git blob SHA.** Matching SHA → never re-parsed, regardless of repo size.
7. **Fuse parses merge versions in memory.** It never writes base/ours/theirs to disk; Grove is queried only for cross-file context.
8. **Provasign's CanonicalBytes excludes `admitted_commit_sha`.** Signed before the commit exists; SHA appended to the trailer post-commit.
9. **`provasign init` is the one-command agent wiring step.** It writes Pre-Flight Autopilot instructions to every per-agent instruction file and registers the provasign MCP server with every detected tool — idempotent on re-run.
10. **Runtime state is never committed.** `.grove/` (engine DB) and `.provasign/*.db` are git-ignored; per-agent `mcp.json` files carry machine-specific absolute paths and are ignored too.
