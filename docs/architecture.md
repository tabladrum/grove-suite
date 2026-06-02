---
title: Architecture
layout: default
nav_order: 4
description: "How Provasign is built: a single binary with the Grove code-knowledge-graph engine embedded in-process. No daemon, no ports, no tokens."
permalink: /architecture/
---

# Architecture
{: .no_toc }

Provasign is a **single Go binary**. There is no server to run, no port to open, no token to manage, and no background daemon. Everything — intent capture, policy gates, test selection, impact analysis, Ed25519 signing, and audit replay — happens in-process when your agent (or you) invokes it.

The engine that makes Provasign *understand your code* rather than just scan diffs is **Grove**, and it is compiled directly into the Provasign binary.

1. TOC
{:toc}

---

## Grove — the knowledge-graph engine inside Provasign

**Grove is the code knowledge graph that Provasign runs on.** It is not a separate service you install or connect to — Provasign links against the Go library `grove/pkg/grove` and calls it directly in the same process.

Where a diff scanner sees *lines changed*, Grove sees *symbols changed and everything connected to them*. That is what lets Provasign answer the questions a certification gate actually needs to answer:

- **What does this change break?** — blast radius across the whole repository, not just the edited file.
- **Which tests cover the changed symbols?** — so certification runs the *relevant* suite, not all of it or none of it.
- **What is the dependency chain from here?** — for breaking-change and policy reasoning.

### How Grove models your code

Grove parses your source with Tree-sitter into a persistent SQLite graph and keeps it current with delta indexing — a file whose git blob SHA hasn't changed is never re-parsed.

| Layer | What it does |
|---|---|
| **Parse** | Tree-sitter AST extractors for 11 languages (Go, TypeScript/TSX, JavaScript, Python, Java, Rust, C, C++, C#, PHP); all CGO is isolated here |
| **Store** | SQLite in **WAL mode** + **FTS5** full-text search; delta indexing keyed on git blob SHA |
| **Graph** | In-memory `CodeGraph` with **8 typed edges**, BFS traversal |
| **Query** | intent→symbols (FTS5 + BFS), blast radius, deps, test selection, ICR (Isolated Change Region) |
| **Semantic** | `potion-base-8M` Model2Vec embeddings (29 MB) compiled in via `go:embed` — pure-Go inference, **no GPU, no API key, no CGO** |

The eight edge types — `defines`, `contains`, `imports`, `extends`, `implements`, `calls`, `uses-type`, `tests` — are what turn a pile of files into a graph you can traverse. `calls` and `uses-type` are deliberately scoped to the same file plus imported files; unscoped, a function named `parse` would appear to call every `parse` in the repo and produce ~5× false-positive edges.

Symbols are addressed as `{filePath}::{qualifiedName}@{blobSHA}`, so a symbol identity is stable until its content actually changes.

### One graph per repository

Each repository has exactly one Grove database at `<repo>/.grove/grove.db`. Provasign opens it in-process; so can the standalone `grove` CLI, Prism, and Fuse if you use them. WAL mode handles concurrent readers, and Grove uses a single writer connection with a 30-second busy timeout so multiple agents (Claude Code + Copilot + a Provasign pre-push hook) don't collide.

> **Why embedded, not a daemon?** Per-project Grove daemons created port collisions, per-repo token mismatches, and an opaque multi-process failure mode. The library model gives zero-config startup, hermetic installs, and a single canonical knowledge graph per repo. There is no `grove serve`, no `$GROVE_URL`, no shared secret.

---

## The Provasign binary

Provasign sits on top of Grove and adds the certification machinery:

```
┌──────────────────────────────────────────────────────────┐
│  provasign  (single binary)                                    │
│                                                            │
│  ┌─────────────┐   ┌──────────────┐   ┌────────────────┐   │
│  │ Intent      │   │ Policy gates │   │ Signer         │   │
│  │ capture     │   │ path·secrets │   │ Ed25519        │   │
│  │ (YAML)      │   │ fileclass    │   │ local key      │   │
│  └─────────────┘   │ deps·size    │   └────────────────┘   │
│                    │ coverage     │                        │
│  ┌─────────────┐   └──────────────┘   ┌────────────────┐   │
│  │ Analyzers   │                       │ Engine store   │   │
│  │ semgrep …   │   ┌──────────────┐    │ certs +        │   │
│  │ (downloaded │   │  Grove        │   │ changesets     │   │
│  │  on demand) │◄──┤  pkg/grove    │   │ (.provasign/       │   │
│  └─────────────┘   │  EMBEDDED     │   │  engine.db)    │   │
│                    │  impact·tests │    └────────────────┘   │
│                    │  deps·query   │                        │
│                    └──────┬────────┘                        │
└───────────────────────────┼────────────────────────────────┘
                            ▼
                  <repo>/.grove/grove.db   (one graph per repo)
```

Provasign calls Grove for the analysis that needs code understanding:

| Grove method | Provasign uses it for |
|---|---|
| `Impact(file, line)` | blast radius of a change during certification |
| `Tests(symbol)` | selecting the tests that actually cover the change |
| `Deps(file)` | dependency reasoning for policy gates |
| `Query(intent, limit)` | relating the captured intent to touched symbols |
| `Status()` | index freshness checks |

### External analyzers are downloaded, not bundled

The heavy security tooling — semgrep, gitleaks, govulncheck, eslint, ruff, sonarlint-ls, a JRE — is **not** baked into the binary. `provasign tools install` fetches pinned versions on first use (Python/Node tools via pipx/npm). This keeps the binary small and lets you pin and audit exactly which analyzer versions produced a certificate.

---

## Local-first by design

| Property | Detail |
|---|---|
| **No network listeners** | Grove is in-process; Provasign's core flow opens no ports and runs no daemon |
| **No telemetry** | Nothing phones home; your source never leaves the machine |
| **Local signing key** | Ed25519 keypair at `<repo>/.provasign/keys/signing.ed25519.key` (mode `0600`), or `~/.provasign/keys/` with `--user`; generated on first use |
| **State in git + local SQLite** | Committed intents live in `.provasign/intents/`; certificates and changesets live in `.provasign/engine.db` (gitignored) |
| **Secrets via env only** | Credentials are never written into `.provasign/provasign.yaml` |

---

## Deployment modes

One binary, three modes — same config, same certificate format:

| Mode | State storage | Use case |
|---|---|---|
| **Laptop** *(shipped)* | SQLite + local Ed25519 key | Solo developer, zero infrastructure |
| **Team** *(roadmap)* | Postgres + Redis + KMS-backed signer | Shared audit trail across developers |
| **Air-gapped** *(roadmap)* | On-prem Postgres + HSM | Regulated industries, no external network |

The signer is an interface: laptop mode uses a local Ed25519 key; team mode swaps in a KMS-backed signer behind the same interface, so certificates keep the same shape.

> **Verification scope.** In laptop mode, certificates and the signing key live on the developer's machine (`.provasign/engine.db` and `.provasign/keys/`, both gitignored), so `provasign cert verify` / `provasign cert replay` are local to that machine. **Independent, cross-machine verification — by an auditor, a CI job, or a reviewer on a fresh clone — is a server (team) mode capability**, where a shared certificate store and a KMS-backed public key make admissions verifiable anywhere. Server mode is on the roadmap.

---

## Read more

- [How It Works]({{ '/how-it-works/' | relative_url }}) — the certify → sign → replay flow end to end
- [Features]({{ '/features/' | relative_url }}) — gates, certificates, and agent wiring
- [Other developer tools]({{ '/other-tools/' | relative_url }}) — Prism and Fuse, which embed the same Grove engine
- [Grove source on GitHub](https://github.com/provasign/provasign/tree/main/grove#readme)
