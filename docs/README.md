---
title: Documentation
layout: default
nav_order: 8
description: "Full documentation map for Grove Suite — organized by role, topic, product, and integration."
permalink: /docs/
---

# Grove Suite Documentation

**Welcome.** This is the home of the long-form documentation for Grove Suite — the infrastructure layer that makes AI coding agents production-safe.

If you just want to install and try it, the [installation guide](/installation) has the five-minute path. This page exists for the people who need to understand it deeper before they bring it to their team, their CISO, their CFO, or their auditor.

---

## Read by Role

| If you are a... | Start here |
|----------------|-----------|
| **Developer** evaluating Grove Suite for daily use | [Developer guide](audiences/developer.md) |
| **Team lead** rolling it out to your team | [Team lead guide](audiences/team-lead.md) |
| **Engineering executive** building the business case | [Executive brief](audiences/executive.md) |
| **CFO / finance** sizing the cost and savings | [Financial brief](audiences/financial.md) |
| **CISO / security** evaluating the threat model | [Security brief](audiences/security.md) |
| **Compliance / audit** evaluating evidence quality | [Audit brief](audiences/audit.md) |
| **Anyone** wanting the founder's pitch | [Why Grove Suite](why.md) |

---

## Read by Topic

| Topic | Document |
|-------|----------|
| **The case for Grove Suite** — what we built, why we built it, what we deliberately didn't build | [Why Grove Suite](why.md) |
| **Comparisons** — Prism vs Copilot semantic search · Grove vs LSP/Sourcegraph · Fuse vs git/AI-merge · Relay vs CI/CodeRabbit | [Comparisons](comparisons.md) |
| **FAQ** — top questions across technical, security, financial, and operational lenses | [FAQ](faq.md) |
| **Troubleshooting** — common issues, how to diagnose, how to fix | [Troubleshooting](troubleshooting.md) |
| **Architecture** — inter-product API contracts, data flows, security model | [Architecture](../Architecture.md) |

---

## Read by Product

| Product | One-line | Documentation |
|---------|---------|--------------|
| **Grove** | Persistent code knowledge graph | [Grove README](../grove/README.md) |
| **Prism** | Graph-ranked context for any AI agent | [Prism README](../prism/README.md) |
| **Fuse** | Symbol-aware git merge driver | [Fuse README](../fuse/README.md) |
| **Relay** | Certified delivery for agent-produced code | [Relay README](../relay/README.md) |

---

## Read by Concept

Each of these explains one technical idea in depth.

| Concept | Document |
|---------|----------|
| **Progressive disclosure** — how Prism cuts token use ~99% on session re-reads | [Progressive disclosure](concepts/progressive-disclosure.md) |
| **Symbol-level merge** — what Fuse does that line-level merge cannot | [Symbol-level merge](concepts/symbol-merge.md) |
| **In-loop certification** — moving quality gates from CI into the agent loop | [In-loop certification](concepts/certification.md) |
| **Intent capture** — committing the prompt as a YAML alongside the code | [Intent capture](concepts/intent-capture.md) |

---

## Read by Integration

How Grove Suite hooks into the tools you already use.

| Tool | Document |
|------|----------|
| Claude Code | [integrations/claude-code.md](integrations/claude-code.md) |
| GitHub Copilot (VS Code) | [integrations/copilot.md](integrations/copilot.md) |
| Cursor | [integrations/cursor.md](integrations/cursor.md) |
| Codex CLI | [integrations/codex-cli.md](integrations/codex-cli.md) |
| Windsurf | [integrations/windsurf.md](integrations/windsurf.md) |
| Zed | [integrations/zed.md](integrations/zed.md) |

---

## Status

This documentation is being built in phases. The table above shows the **complete map**; not every page is written yet.

| Phase | Documents | Status |
|-------|-----------|--------|
| 1 — Foundation | docs index, why, comparisons, FAQ, troubleshooting, GitHub Pages setup | **In progress** |
| 2 — Audience briefs | executive, financial, security, audit, developer, team-lead | Planned |
| 3 — Concept deep-dives | progressive disclosure, symbol merge, in-loop cert, intent capture | Planned |
| 4 — Integration guides | per-tool setup, troubleshooting, advanced config | Planned |

Pages that don't exist yet will return 404 — that's a feature, not a bug. We'd rather have five great pages than fifty mediocre ones.

---

## Contributing to the Docs

These docs live in [`docs/`](https://github.com/tabladrum/grove-suite/tree/main/docs) in the main repository. PRs welcome.

Quality bar:
- **Real numbers** over adjectives. "35–92% token savings" beats "huge savings."
- **Honest tradeoffs.** Every product has things it doesn't do well. Say so.
- **Specific competitors.** Vague "other tools in the space" is worth nothing. Name names.
- **One audience per page.** A doc trying to talk to a CFO and a developer at the same time talks to neither.
