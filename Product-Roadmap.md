# Relay — Product Vision & Roadmap

**The product is Relay:** certified delivery for AI coding agents. Every AI-generated commit signed, tested, and traceable to the prompt that created it.

Relay stands on three open-source components — **Grove** (code knowledge graph), **Prism** (context delivery), **Fuse** (semantic merge) — each useful on its own and **embedded directly into Relay** as Go libraries. No daemon, no ports, no tokens: one install.

**Last updated:** June 1, 2026

---

## Status at a glance

| Component | CLI | Role | License | Status |
|-----------|-----|------|---------|--------|
| **Relay** | `relay` | **The product** — intent capture, in-loop gates, Ed25519 admission certs, audit replay | AGPL-3.0 | Phase 1 (intake) + Phase 2A laptop MVP shipped; team mode in progress |
| **Grove** | `grove` | Shared engine — Tree-sitter graph, embedded as a Go library; standalone CLI + `grove mcp` | MIT | Shipped; embedded into Prism/Fuse/Relay |
| **Prism** | `prism` | Token-optimized context for any AI agent | MIT | Shipped (MCP + VS Code extension) |
| **Fuse** | `fuse` | Symbol-level semantic git merge driver | MIT | Shipped |

---

## Why Relay

AI agents now write a large and growing fraction of production code, but the delivery path on the other side is still the human-era PR → CI loop: push, wait, CI fails, the agent has lost context, loop 3–5 times. And when a security or compliance review asks *"what did the agent do, and who verified it was safe?"* — the PR says "refactor auth," CI is green, the agent session and the original prompt are both gone. There is no answer.

**Relay is the answer.** It is an admission-control layer between the agent and `main`:

1. **Captures the prompt** as a signed YAML intent committed to the repo.
2. **Runs the quality gates inside the agent loop** — build, tests, coverage, secrets, SAST, dependency audit — in under 10 seconds, so the agent self-corrects *before* opening a PR.
3. **Issues an Ed25519-signed certificate** binding prompt → gates → toolchain versions → commit.
4. **Replays the audit** months later: `relay cert replay <id>` re-runs the same gates and reports `byte_reproducible`, `tool_drift`, or `config_drift`.

The CI loop drops from 3–5 iterations to 0–1, and every commit carries cryptographic proof of quality that outlives the agent session.

---

## The open-source foundation

Relay is coherent — rather than three unrelated tools — because all three components share one engine. Each is independently adoptable under MIT; you can use any of them without Relay.

```
                         RELAY  (the product, AGPL-3.0)
                  intent · gates · certificates · replay
                                  │
                 embeds (in-process Go library)
        ┌─────────────────────────┼─────────────────────────┐
        ▼                         ▼                          ▼
     GROVE                      PRISM                       FUSE
   code graph              context delivery             semantic merge
   (the engine)            (MIT, standalone)           (MIT, standalone)
        ▲                         │                          │
        └──────── embeds ─────────┴──────────────────────────┘
```

- **Grove** — Tree-sitter parser, SQLite-backed graph (multi-language, 8 edge types), BFS traversal, FTS5 search, delta indexing by git blob SHA. Embedded as `grove/pkg/grove`; also a standalone CLI and `grove mcp` stdio server.
- **Prism** — graph-ranked context for agents: 5-signal ranking, budget allocation, progressive disclosure, session dedup. 35–92% fewer tokens on first reads, ~99% on re-reads.
- **Fuse** — symbol-level three-way merge with Grove-backed breaking-change detection; auto-resolves the majority of false git conflicts.

---

## Architecture decision — embedded, no daemon

The v1 design ran Grove as a per-repo HTTP/gRPC daemon (`grove serve` on `:7777`/`:7778`) that Prism/Fuse/Relay talked to over HTTP with a shared-secret token. That created port collisions, per-repo token mismatches, and an opaque multi-process failure mode.

**Now Grove is a Go library.** Prism, Fuse, and Relay link `grove/pkg/grove` and open `<repo>/.grove/grove.db` directly (SQLite WAL handles concurrent readers). Zero config, hermetic per-product installs, one canonical graph per repo. The `grove` CLI and `grove mcp` stdio server remain for direct human/agent use. See [Architecture.md](Architecture.md).

---

## Roadmap

### Shipped

- **Grove** — graph engine, multi-language Tree-sitter parsing, 8 edge types, delta indexing, FTS5, embeddings; embedded library + CLI + `grove mcp`.
- **Prism** — context delivery (ranking, budgets, progressive disclosure, session dedup); MCP server (`prism mcp`) + VS Code extension.
- **Fuse** — semantic merge driver (`fuse install`, `.gitattributes`); 7-phase IntelliMerge.
- **Relay Phase 1 (intake)** — intent capture, GS scoring.
- **Relay Phase 2A laptop MVP** — in-loop `relay_check`, Stage-1/Stage-2 certification, Ed25519 admission, audit replay, one-command agent wiring (`relay init`) and intent MCP tools across Claude Code / Copilot / Cursor / Codex / Windsurf / Zed / Kiro.
- **Cross-agent MCP** — newline-delimited stdio transport + protocol-version negotiation so every MCP client connects.
- **Install UX** — agent setup prompt + one-command `install.sh` (download, checksum-verify, init).

### In progress — Relay team mode (Phase 2B)

- Shared intent-store and certificate history across a team.
- Server-side policy distribution (`platform-config`).
- Review UX for the human approval gate.
- Hardening of certification reproducibility (`tool_drift` / `config_drift` reporting).

### Next — Relay for organizations (Phase 3)

- Dashboard + audit-evidence export (security / compliance).
- Enterprise auth (OIDC/SSO), policy-as-code at org scope.
- Integration depth: Jira / GitHub Issues / Linear intent sources, CI attestation handoff.
- Performance and chaos hardening at monorepo scale.

### Ongoing — the open-source foundation

- More languages and higher edge-confidence calibration in Grove (community-driven).
- Prism ranking-profile tuning and VS Code surface polish.
- Fuse language coverage and conflict-resolution quality.

---

## Commercial model

- **Grove, Prism, Fuse** — free and open source (MIT). This is the distribution engine and the credibility moat: developers adopt the components, validate the graph in production, and discover Relay.
- **Relay** — the commercial product (AGPL-3.0). Local-first for individuals and small teams; paid team/enterprise features (shared intent-store, policy distribution, audit-evidence export, SSO). It replaces a slice of the Jira + CI + review-tooling budget, so there is real budget to address.

```
Grove/Prism/Fuse adopted widely (free, MIT)
        ↓
teams see the value of in-loop certification
        ↓
teams adopt Relay (team features)
        ↓
organizations standardize on Relay (enterprise)
        ↓
more contributors → better engine → repeat
```

---

## Competitive position

| Tool | What it does | What it misses |
|------|--------------|----------------|
| CI (GitHub Actions, etc.) | Gates after push | Not in the agent loop; logs expire; no prompt, no signature |
| CodeRabbit / Greptile | Cloud PR review | After PR open; cloud-only; no replayable certificate |
| Sigstore / SLSA | Build-time provenance | No prompt capture; no in-loop gates; provenance only |
| Sourcegraph / LSP | Code search / intelligence | Not agent-native; no certification or admission |
| **Relay** | **Prompt → gates → Ed25519 cert → commit, in the agent loop, local-first** | **Nothing binds prompt to certificate to commit the way Relay does** |

---

## The one thing to remember

Every competitor makes one agent smarter. Relay makes agent-produced code **accountable** — provable, replayable, and traceable to the prompt that asked for it. Grove, Prism, and Fuse prove the engine works in production; Relay is what you bet the company on.

Start with `relay init`.
