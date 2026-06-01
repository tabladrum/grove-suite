---
title: Other Developer Tools
layout: default
nav_order: 9
description: "Prism and Fuse — the open-source tools that share Relay's embedded Grove engine, usable standalone."
permalink: /other-tools/
---

# Other Developer Tools

Relay is the product this site is about. The same [Grove knowledge-graph engine]({{ '/architecture/#grove--the-knowledge-graph-engine-inside-relay' | relative_url }}) that powers Relay's impact analysis and test selection also powers two standalone, MIT-licensed tools you can adopt independently. Their full documentation lives in their repositories — this page is a short orientation.

---

## Prism — graph-ranked context for any AI agent

Prism gives an AI coding agent the *right* code instead of a dump of nearby files. It queries Grove, scores every candidate across five signals (graph distance, semantic similarity, recency, test relevance, edit frequency), allocates a token budget across five categories, and discloses progressively — full source on the first read, a tiny sha-pointer on re-reads.

- **35–92% fewer tokens** on first reads; **~99%** on same-session re-reads.
- Integrates over MCP (Claude Code, Cursor, Codex CLI, Windsurf, Continue) or as a native VS Code extension.
- Embeds Grove in-process — no daemon, no port.

[Prism README on GitHub →](https://github.com/tabladrum/grove-suite/tree/main/prism#readme){: .btn .btn-primary }

---

## Fuse — symbol-aware Git merge driver

Git merges lines; Fuse merges **symbols**. When one agent changes `Login()` and another changes `validatePassword()` in the same file, git sees adjacent-line conflicts where none exist. Fuse parses all three versions with Tree-sitter, queries Grove for cross-file blast radius, and resolves at symbol granularity.

- **~85% auto-resolution** on incremental conflicts.
- Genuinely ambiguous conflicts get markers plus a structured AI-handoff prompt at `.git/fuse/conflict-<hash>.md`.
- Drops in as a git merge driver: `fuse merge %O %A %B %P`.

[Fuse README on GitHub →](https://github.com/tabladrum/grove-suite/tree/main/fuse#readme){: .btn .btn-primary }

---

## How they relate to Relay

| | Role | License | Relationship to Relay |
|---|---|---|---|
| **Grove** | Code knowledge graph | MIT | **Embedded inside Relay** (see [Architecture]({{ '/architecture/' | relative_url }})) |
| **Prism** | Context delivery for agents | MIT | Standalone; shares the Grove engine |
| **Fuse** | Symbol-level merge driver | MIT | Standalone; shares the Grove engine |
| **Relay** | Certified delivery | AGPL-3.0 | **The product** — embeds Grove |

You do not need Prism or Fuse to use Relay. They solve adjacent problems in the AI-assisted workflow and happen to be built on the same foundation.
