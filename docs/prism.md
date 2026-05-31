---
title: Prism
layout: default
nav_order: 4
description: "Prism — focused, graph-ranked context for any AI coding agent. 35–92% fewer tokens on first reads, ~99% on re-reads."
permalink: /prism/
---

# Prism

**Focused, graph-ranked context for any AI coding agent — 35–92% fewer tokens on first reads, ~99% on re-reads.**
{: .fs-5 .fw-300 }

[Install Prism](/installation){: .btn .btn-primary .fs-5 .mb-4 .mb-md-0 .mr-2 }
[View source](https://github.com/tabladrum/grove-suite/tree/main/prism){: .btn .fs-5 .mb-4 .mb-md-0 }

---

An AI coding agent that gets bad context produces bad code. Not because it's a bad agent — because it's working blind.

The naive approach is to dump related files into the context window and hope the agent figures out what matters. This fails in two directions at once: it wastes tokens on code that's nearby in the file tree but irrelevant to the task, and it misses code that *is* critical but has no obvious filename match. The agent hallucinates the gaps.

Prism solves this. Given a task description, it queries [Grove's knowledge graph](/grove), scores every candidate across five signals, allocates a token budget across five categories, and returns exactly what matters — full source for the first read, signatures on the second, one-line references on the third.

The agent gets more signal per token. Every time.

---

## Works With

| Tool | Integration |
|------|------------|
| **Claude Code** | MCP stdio (`prism mcp`) |
| **GitHub Copilot** (VS Code) | Native VS Code extension via `vscode.lm.registerTool` (no MCP needed) |
| **Cursor** | MCP stdio |
| **Codex CLI** | MCP stdio |
| **Windsurf** | MCP stdio |
| **Zed** | MCP stdio |
| **Continue** | MCP stdio |
| **Any MCP-capable tool** | MCP stdio |

`prism init` auto-detects installed tools and writes their MCP configs — no per-tool hand-editing.

---

## How It Works

```
Task description ("add rate limiting to the login endpoint")
     │
     ▼
prism_query
     │
     ├──► Grove: FTS5 symbol search
     ├──► Grove: BFS graph traversal (depth 2–3)
     │
     ▼
┌─────────────────────────────────────────────────────┐
│  5-Signal Ranking                                   │
│  1. Graph distance      — BFS hops from seed        │
│  2. Semantic similarity — Model2Vec cosine          │
│  3. Recency             — recent git commits        │
│  4. Test relevance      — is this a test for target?│
│  5. Edit frequency      — hot files get priority    │
└─────────────────────────┬───────────────────────────┘
                          ▼
┌─────────────────────────────────────────────────────┐
│  Budget Allocation                                  │
│  Target symbols    35%                              │
│  Dependencies      25%                              │
│  Tests             20%                              │
│  Documentation     10%                              │
│  Summary           10%                              │
└─────────────────────────┬───────────────────────────┘
                          ▼
┌─────────────────────────────────────────────────────┐
│  Progressive Disclosure                             │
│  First read:    full source (or ranked-compressed)  │
│  Second read:   sha-pointer (~10 tokens)  ~99% ↓   │
│  Third read:    sha-pointer or signature            │
│  Fourth+:       full resend (scrolled out)          │
└─────────────────────────┬───────────────────────────┘
                          ▼
              Token-optimized context pack
```

---

## Token Savings

Measured on the Grove Suite repo itself, May 2026:

| | First read | Re-read (same session) |
|--|:----------:|:----------------------:|
| Relevance-filtered symbols | 35–92% saved | ~99% saved (sha-pointer) |
| All symbols (no filtering) | 0% saved | ~99% saved (sha-pointer) |

**First-read savings** reflect relevance scoring: symbols below the threshold are shown at signature level instead of full source. When nearly all symbols in a file are relevant to the current task, first-read savings approach 0% — you receive everything. This is correct behaviour.

**Second and third reads** apply session deduplication regardless of project size. A file read three times within one session always receives a tiny `// [prism:cached] file.go @sha:a1b2c3d4 (×N)` pointer instead of the full content.

**[See the full performance table →](https://github.com/tabladrum/grove-suite/blob/main/prism/README.md#performance)**

---

## MCP Tools

| Tool | What it does |
|------|-------------|
| `prism_query` | Ranked context pack for a task description — call this first |
| `prism_read` | Progressive-disclosure file read (full → signature → reference) |
| `prism_search` | Symbol search across the indexed graph |
| `prism_lookup` | Full source for a named symbol |
| `prism_index` | Trigger or check reindex |
| `prism_savings` | Token savings report for this session |
| `prism_feedback` | Rate a context result (trains future ranking) |
| `prism_compact` | Summarize older conversation turns to free context |

**Rule of thumb for agents:** start every task with `prism_query`. Use `prism_read` instead of reading files directly. Use `prism_search` instead of grep.

---

## Quick Start

```bash
# Install Grove first (required), then Prism
cd grove-suite/grove && make install && cd ..
cd grove-suite/prism && make install && cd ..

# Wire into your project
cd /your/project
prism init      # detects your coding tool, writes MCP config + steering instructions
prism index     # initial index — delta indexing for subsequent runs

# Restart your coding tool to pick up the MCP server

# Verify savings
prism savings
```

[Full installation guide →](/installation)

---

## CLI (for scripting and debugging)

The `prism` binary works standalone, without an agent:

```bash
prism query "add rate limiting to the login endpoint"
prism read internal/auth/login.go
prism search AuthService
prism lookup internal/auth.LoginHandler
prism savings
```

Add `--json` for machine-readable output.

---

## VS Code Extension

The Prism extension does not use MCP — it registers all 8 tools natively via `vscode.lm.registerTool`. No `prism serve` required, no port.

After install:
- Tools appear in Copilot Chat as `#prismQuery`, `#prismRead`, `#prismSearch`, etc.
- **Grove symbols** status item in the left status bar — live symbol count, click to re-index
- **Prism savings** status item in the left status bar — session token savings %, click for details

[Extension documentation →](https://github.com/tabladrum/grove-suite/tree/main/prism/vscode-extension)

---

## Read More

- [How Prism compares to Copilot semantic search, Claude Code, Cursor, Codex CLI](/comparisons#prism-vs-other-context-delivery)
- [Why Grove Suite exists](/why)
- [Full reference on GitHub](https://github.com/tabladrum/grove-suite/tree/main/prism)
- [Configuration and troubleshooting](/troubleshooting#prism)
