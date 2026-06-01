# CLAUDE.md

Guidance for Claude Code when working in this repository.

## Product Overview

| Component | CLI | Role | License |
|---|---|---|---|
| **Relay** | `relay` | **The product.** Certified delivery for AI coding agents — intent capture, pre-commit gates, Ed25519 admission certificates, audit replay. | AGPL-3.0 |
| **Grove** | `grove` | Code knowledge graph — Tree-sitter parsing, SQLite storage, BFS traversal. **Embedded as a Go library** into Relay, Prism, and Fuse. Usable standalone via the `grove` CLI. | MIT |
| **Prism** | `prism` | Token-optimized context delivery for AI agents — ranking, compression, session deduplication. Standalone OSS; embeds Grove. | MIT |
| **Fuse** | `fuse` | Semantic Git merge driver — symbol-level three-way merge, breaking change detection. Standalone OSS; embeds Grove. | MIT |

## Architecture Decision — Embedded Grove (no daemon)

**Grove is a Go library, not a service.** Prism, Fuse, and Relay link against `grove/pkg/grove` and call it in-process. There is no `grove serve`, no HTTP port, no shared-secret token, no `$GROVE_URL`. Each consumer opens `.grove/grove.db` directly via SQLite (WAL mode handles concurrent readers).

Why: per-project Grove daemons created port collisions, per-repo token mismatches, and an opaque multi-process failure mode. The library model gives zero-config UX, hermetic per-product installs, and a single canonical place for the knowledge graph per repo.

The `grove` CLI still ships for direct human use (`grove index`, `grove impact`, `grove tests`) and for the standalone `grove mcp` stdio server. The HTTP/gRPC servers are removed.

**Dependency order:** Build Grove first — it produces the Go library that Prism, Fuse, and Relay link against (and the standalone `grove` binary).

## Build Commands

Each product is a Go 1.26 module with a consistent Makefile. From within any product directory:

```bash
make build      # compile binary
make test       # run all tests
make lint       # lint
make proto      # regenerate gRPC stubs (relay/ only)

# Run a single test
go test ./internal/parser/... -run TestGoExtractor
go test ./... -run TestName -v

# Install locally
make install    # installs binary to $GOPATH/bin
```

For the Prism VS Code extension (TypeScript, in `prism/vscode-extension/`):
```bash
npm install && npm run compile
vsce package   # build .vsix
```

## Architecture

### Grove — Core Graph Engine (Go library)

Grove is the only component with its own storage and parser. All others import its public API package `grove/pkg/grove`.

**Packages:**
- `pkg/grove/` — public API: `Engine` with methods `Index`, `Query`, `Impact`, `Deps`, `Symbols`, `Tests`, `Semantic`, `Status`. Stable surface that Prism/Fuse/Relay depend on.
- `internal/parser/` — Tree-sitter engine; language-specific extractors in `strategies/`; all CGO usage is isolated here
- `internal/store/` — SQLite (WAL + FTS5); delta indexing skips files whose git blob SHA is unchanged
- `internal/graph/` — In-memory `CodeGraph` with 8 edge types (defines, contains, imports, extends, implements, calls, uses-type, tests); BFS traversal
- `internal/query/` — Intent→symbols (FTS5 + BFS), blast radius, deps, test selection, ICR computation
- `internal/mcp/` — 8 MCP tools over JSON-RPC 2.0 stdio (the standalone `grove mcp` server). Wire format is newline-delimited JSON, per the MCP stdio transport.
- `internal/embeddings/model2vec/` — potion-base-8M (29 MB) embedded via `go:embed`; pure-Go inference, no CGO

The former HTTP (`:7777`) / gRPC (`:7778`) servers were removed with the daemon (see "Embedded Grove" above). Consumers use `pkg/grove` in-process; humans use the `grove` CLI / `grove mcp` stdio server.

**Key constraints:**
- Single binary, zero runtime dependencies (SQLite via `modernc.org/sqlite` — pure Go, no CGO conflict with tree-sitter)
- `calls` and `uses-type` edges scoped to same-file + imported files only — unscoped produces ~5× false positives
- Delta indexing: never re-parse a file whose git blob SHA hasn't changed
- Symbol ID format: `{filePath}::{qualifiedName}@{blobSHA}`

### Prism — Context Delivery Layer

Imports `grove/pkg/grove` and calls it in-process. Owns: 5-signal composite ranking (graph distance + semantic similarity + recency + test relevance + edit frequency), budget allocation across 5 categories (target 35%, deps 25%, tests 20%, doc 10%, summary 10%), progressive disclosure (full → signature → reference), O(1) LRU session deduplication.

Two integration paths: MCP mode (`prism serve`) and VS Code Extension (registers all 8 tools via `vscode.lm.registerTool`, no `prism serve` needed).

### Fuse — Merge Driver

Operates at symbol granularity using its own Tree-sitter parsing of in-memory content (three versions of a file during a merge). Imports `grove/pkg/grove` for cross-file blast radius and breaking change detection (`Engine.Impact`, `Engine.Deps`, `Engine.Symbols`).

**7-phase IntelliMerge pipeline:** Context building (Grove) → Symbol extraction → Recency analysis → Project graph context → Breaking change detection → Conflict classification → Strategy selection.

Git driver contract: `fuse merge %O %A %B %P`, exit 0 (clean) or exit 1 (conflict markers + `.git/fuse/conflict-<hash>.md`).

### Relay — Delivery Platform

Imports `grove/pkg/grove` for impact analysis and test selection during certification.

**External tools** (semgrep, gitleaks, govulncheck, eslint, ruff, sonarlint-ls, JRE) are downloaded on first use by `relay tools install`, not bundled. Pinned download URLs are in `internal/tools/registry.go`.

**Three git repos:** `source-repo` (application code, linear main), `intent-store` (YAML intents, audit trail), `platform-config` (policies).

**Redis is transient only** — `appendonly no`. All business state in git.

## Inter-Component API — Go library (`grove/pkg/grove`)

Prism, Fuse, and Relay all consume Grove via the in-process Go API. No HTTP, no gRPC, no auth, no ports.

```go
import "github.com/tabladrum/grove-suite/grove/pkg/grove"

eng, err := grove.Open(ctx, grove.Config{RepoRoot: "/path/to/repo"})
defer eng.Close()

res, err := eng.Query(ctx, grove.QueryRequest{Intent: "login flow", Limit: 20})
imp, err := eng.Impact(ctx, grove.ImpactRequest{File: "auth.go", Line: 42})
```

| Method | Consumer |
|---|---|
| `Index(ctx, dir)` | All |
| `Query(ctx, intent, limit)` | Prism, Relay |
| `Impact(ctx, file, line)` | Fuse, Relay |
| `Deps(ctx, file)` | Fuse, Prism, Relay |
| `Symbols(ctx, query)` | Prism, Fuse |
| `Tests(ctx, symbol)` | Relay |
| `Semantic(ctx, query)` | Prism |
| `Status(ctx)` | Prism, Relay |

The `grove` CLI is still useful for direct human use (`grove index`, `grove impact`, etc.) and for the standalone `grove mcp` stdio server.

## Testing

- **Unit tests:** Per-package `_test.go` files.
- **Integration tests:** `testdata/` fixture repos or merge fixtures.
- **Benchmarks:** Grove: index 5000 files < 5s, BFS depth-3 on 50K nodes < 30ms, FTS5 < 10ms. Fuse: 7-phase pipeline < 200ms. Prism: `prism_query` end-to-end < 200ms.

## Prism — context delivery (ALWAYS use these tools)

Prism MCP tools are registered in this session. Follow these rules:

1. **Start every task with `prism_query`** — before reading any files. Returns pre-ranked, compressed context covering targets, dependencies, and tests.
2. **Use `prism_read` instead of reading files directly** — full source on first read, signatures on second, references on third. Saves 35–92% tokens.
3. **Use `prism_search` instead of grep/find** — follow up with `prism_lookup` for full source.
4. **Call `prism_index` once at session start** (or after significant file changes). Do not re-index on every step.
5. **Call `prism_compact` when context window is near capacity.**
6. **If a Prism tool returns empty results**, run `prism_index` for the workspace root and retry before falling back to grep/read.

| Instead of...         | Use...        |
|-----------------------|---------------|
| read_file / open file | prism_read    |
| grep / ripgrep / find | prism_search  |
| manual context gather | prism_query   |
| symbol definition     | prism_lookup  |


## Relay — certified merge gate (ALWAYS use these tools)

This project uses [Relay](https://github.com/tabladrum/grove-suite) for
certified code admission. Relay MCP tools are registered. Follow this
workflow on EVERY coding task:

### Pre-Flight Autopilot

1. **Open an intent BEFORE making code changes** — call relay_intent_open with
   {title: <short summary>, description: <verbatim user request>}.
   Save the returned intent_id.

2. **Before asking the user to review** — call relay_check with the unified
   diff plus {intent: <intent_id>, brief: <one-liner>}.

3. **If Allowed=false** — for each policy entry with Verdict != "allow":
   - Call relay_explain {gate, rule} for the recommended fix.
   - Apply the fix, re-diff, and re-call relay_check.
   - Loop up to 3 times; on the 3rd failure surface the verdict to the user.

4. **Only call relay_submit when relay_check returns Allowed=true** on the
   EXACT same diff. Never call relay_submit speculatively.

5. **Close the intent when done** — call relay_intent_close {intent_id}.
   Pass the returned trailer_block to relay_submit so the commit is linked
   to the intent YAML.

### Tool quick-reference

| Tool                 | When                                          |
|----------------------|-----------------------------------------------|
| relay_intent_open    | First — capture the user request as an intent |
| relay_check          | Before every review request                   |
| relay_explain        | On any Verdict != allow                       |
| relay_submit         | Only after relay_check Allowed=true           |
| relay_policy         | Discover which gates are active               |
| relay_intent_close   | When the task is complete                     |
