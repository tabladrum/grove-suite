---
title: Grove
layout: default
nav_order: 3
description: "Grove — persistent code knowledge graph. The long-term memory of your codebase, queryable by any AI agent."
permalink: /grove/
---

# Grove

**Your codebase's persistent long-term memory — queryable by any AI agent.**
{: .fs-5 .fw-300 }

[Install Grove](/installation){: .btn .btn-primary .fs-5 .mb-4 .mb-md-0 .mr-2 }
[View source](https://github.com/tabladrum/grove-suite/tree/main/grove){: .btn .fs-5 .mb-4 .mb-md-0 }

---

Grep answers "does this string appear somewhere?" A language server answers "where is this symbol defined?" Grove answers the harder questions AI agents actually need:

- *What does changing this function break — across the entire codebase?*
- *Which tests cover this method, directly or transitively?*
- *What is the full dependency chain from this file?*
- *What symbols are semantically related to this task description?*

The difference is a graph. Grove indexes your source files into a persistent SQLite graph — 11 languages, 8 edge types, BFS traversal — and keeps it live with delta indexing (files whose git blob SHA hasn't changed are never re-parsed).

Every other tool in Grove Suite is built on Grove. Without it, Prism falls back to filename guessing, Fuse to line-level merge, Relay to coarse test selection.

---

## What It Does

| Capability | How |
|------------|-----|
| Parse | Tree-sitter AST walkers for 11 languages + regex fallback for syntax-error recovery |
| Store | SQLite WAL + FTS5 full-text search, delta indexing by git blob SHA |
| Query | BFS graph traversal across 8 edge types, FTS5 keyword search, Model2Vec semantic similarity |
| Serve | CLI · HTTP API (`:7777`) · gRPC (`:7778`) · MCP stdio for AI agents |
| Scale | 10K-file monorepo in 34 seconds cold; delta re-index on a one-file change in milliseconds |

---

## Languages Supported

**AST-parsed (with full symbol graph):**

Go · TypeScript · TSX · JavaScript (incl. JSX/MJS/CJS) · Python · Java · Rust · C · C++ · C# · PHP

**Indexed as documents (FTS5 + Model2Vec, no symbol graph):**

Markdown · YAML · JSON · XML · TOML · INI · shell scripts · Dockerfile · Makefile · SQL · GraphQL · Protobuf · CSV · plain text

A semantic query for "deployment configuration" returns both the Go function that reads the config *and* the Dockerfile that defines the runtime — together, ranked.

---

## Graph Edges (the secret sauce)

Eight typed edges connect symbols. Each one answers a different question.

| Edge | Question it answers |
|------|--------------------|
| `defines` | Where is this symbol defined? |
| `contains` | What does this class/namespace include? |
| `imports` | What files does this file pull in? |
| `extends` | What does this class inherit from? |
| `implements` | What interfaces does this class satisfy? |
| `calls` | What functions does this function call? (scoped to same-file + imports) |
| `uses-type` | What types does this function reference? (scoped to same-file + imports) |
| `tests` | What tests exercise this symbol? |

**Why `calls` is scoped:** Without scoping `calls` and `uses-type` edges to same-file and imported files, a function named `parse` in one package would appear to call every `parse` function in unrelated packages — producing roughly 5× the false-positive edges. This single design choice is why Grove's blast radius queries are useful instead of noisy.

---

## Performance

Benchmarks run on macOS against synthetic Go projects (2026-05-27). Numbers reflect a cold index. Subsequent runs on unchanged projects complete in milliseconds.

| Project | Files | Index time | Peak RSS | Query latency |
|---------|------:|-----------:|---------:|--------------:|
| Small | 61 | 0.06 s | 30 MB | 6 ms |
| Medium | 801 | 0.85 s | 55 MB | 6 ms |
| Large | 4,501 | 11.6 s | 117 MB | 9 ms |
| Monorepo | 9,901 | 34.0 s | 196 MB | 61 ms |

**Targets:** index 5,000 files < 5 s · BFS depth-3 on 50K nodes < 30 ms · FTS5 query < 10 ms

---

## How AI Agents Use It

Grove exposes **8 tools over MCP stdio** (Model Context Protocol) — accessible to any MCP-capable AI agent:

| Tool | Purpose |
|------|---------|
| `grove_index` | Index or reindex a directory |
| `grove_symbols` | Search for symbols by name |
| `grove_query` | Retrieve ranked context for an intent |
| `grove_impact` | Blast radius for a symbol or file |
| `grove_deps` | Dependency tree for a file |
| `grove_tests` | Tests that cover a symbol |
| `grove_icr` | Intent complexity rating |
| `grove_conflicts` | Potential conflict hotspots |

Most users don't run Grove directly — they install [Prism](/prism), which wraps Grove with token-optimized context delivery. But for custom agent integrations, you can connect directly via MCP, HTTP, or gRPC.

---

## Quick Start

```bash
# Install (see the full installation guide for all options)
cd grove-suite/grove && make install

# Index a project
cd /your/project
grove index .

# Query symbols
grove symbols "AuthService"

# Get blast radius for a symbol
grove impact "validatePassword"

# Start the HTTP server (used by Prism, Fuse, Relay)
grove serve --port 7777
```

[Full installation guide →](/installation)

---

## Security

Grove binds to `127.0.0.1` only — no LAN exposure. A 64-char random bearer token at `.grove/.token` (mode 0600, generated on first start) is required on every non-health request. Prism, Fuse, and Relay read this token automatically.

Zero telemetry. Your code never leaves your machine.

---

## Read More

- [How Grove compares to LSP, Sourcegraph, ctags, Stack Graphs](/comparisons#grove-vs-other-code-intelligence)
- [Why Grove Suite exists](/why)
- [Full reference on GitHub](https://github.com/tabladrum/grove-suite/tree/main/grove)
- [Architecture and inter-product API contracts](https://github.com/tabladrum/grove-suite/blob/main/Architecture.md)
