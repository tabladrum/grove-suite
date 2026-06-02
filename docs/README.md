---
title: Documentation
layout: default
nav_order: 13
description: "Full documentation map for Provasign — and its open-source foundation Grove, Prism, and Fuse — organized by role, topic, product, and integration."
permalink: /docs/
---

# Provasign Documentation

**Welcome.** This is the home of the long-form documentation for **Provasign** — certified delivery for AI coding agents — and its open-source foundation: **Grove** (code knowledge graph), **Prism** (context delivery), and **Fuse** (semantic merge).

If you just want to install and try it, the [installation guide]({{ '/installation/' | relative_url }}) has the five-minute path. This page exists for the people who need to understand it deeper before they bring it to their team, their CISO, their CFO, or their auditor.

---

## Licensing

Provasign is the product; Grove, Prism, and Fuse are its open-source foundation under a split-license model:

| Component | Role | License |
|-----------|------|---------|
| **Provasign** | The product — certified delivery | AGPL-3.0 |
| Grove | Embedded code-graph engine (+ standalone CLI) | MIT |
| Prism | Context delivery for AI agents | MIT |
| Fuse | Semantic git merge driver | MIT |

Provasign's AGPL applies to the Provasign product and its Provasign-specific docs and source tree. Grove, Prism, and Fuse remain MIT licensed — adopt them independently in commercial products without obligation.

---

## Read by Role

| If you are a... | Start here |
|----------------|-----------|
| **Developer** evaluating Provasign for daily use | [Overview]({{ '/provasign/' | relative_url }}) · [Agent Setup]({{ '/setup/' | relative_url }}) |
| **Team lead** rolling it out to your team | [Use Cases → Change Management]({{ '/use-cases/change-management/' | relative_url }}) |
| **Engineering executive** building the business case | [Why Provasign]({{ '/why/' | relative_url }}) |
| **CFO / finance** sizing the cost and savings | [Why Provasign → What This Costs You]({{ '/why/#what-this-costs-you' | relative_url }}) |
| **CISO / security** evaluating the threat model | [Use Cases → Security]({{ '/use-cases/security/' | relative_url }}) |
| **Compliance / audit** evaluating evidence quality | [Use Cases → Audit]({{ '/use-cases/audit/' | relative_url }}) |
| **Anyone** wanting the founder's pitch | [Why Provasign]({{ '/why/' | relative_url }}) |

---

## Read by Topic

| Topic | Document |
|-------|----------|
| **The case for Provasign** — what we built, why we built it, what we deliberately didn't build | [Why Provasign]({{ '/why/' | relative_url }}) |
| **How It Works** — capture → gate → certify → sign → replay | [How It Works]({{ '/how-it-works/' | relative_url }}) |
| **Comparisons** — Provasign vs CI/CodeRabbit/Sigstore · Prism vs Copilot semantic search · Grove vs LSP/Sourcegraph · Fuse vs git/AI-merge | [Comparisons]({{ '/comparisons/' | relative_url }}) |
| **FAQ** — top questions across technical, security, financial, and operational lenses | [FAQ]({{ '/faq/' | relative_url }}) |
| **Troubleshooting** — common issues, how to diagnose, how to fix | [Troubleshooting]({{ '/troubleshooting/' | relative_url }}) |
| **Architecture** — embedded Grove engine, single-binary design, data flows, security model | [Architecture]({{ '/architecture/' | relative_url }}) |

---

## Read by Product

| Component | One-line | Documentation |
|-----------|---------|--------------|
| **Provasign** *(the product)* | Certified delivery for agent-produced code | [Overview]({{ '/provasign/' | relative_url }}) · [Provasign README](https://github.com/provasign/provasign/tree/main/provasign#readme) |
| **Grove** | Code knowledge graph embedded in Provasign (+ standalone CLI) | [Architecture]({{ '/architecture/' | relative_url }}) · [Grove README](https://github.com/provasign/provasign/tree/main/grove#readme) |
| **Prism** | Graph-ranked context for any AI agent | [Other Dev Tools]({{ '/other-tools/' | relative_url }}) · [Prism README](https://github.com/provasign/provasign/tree/main/prism#readme) |
| **Fuse** | Symbol-aware git merge driver | [Other Dev Tools]({{ '/other-tools/' | relative_url }}) · [Fuse README](https://github.com/provasign/provasign/tree/main/fuse#readme) |

---

## Read by Concept

Each of these explains one technical idea in depth.

| Concept | Document |
|---------|----------|
| **Progressive disclosure** — how Prism cuts token use ~99% on session re-reads | [Other Dev Tools → Prism]({{ '/other-tools/' | relative_url }}) |
| **Symbol-level merge** — what Fuse does that line-level merge cannot | [Other Dev Tools → Fuse]({{ '/other-tools/' | relative_url }}) |
| **In-loop certification** — moving quality gates from CI into the agent loop | [How It Works]({{ '/how-it-works/' | relative_url }}) |
| **Intent capture** — committing the prompt as a YAML alongside the code | [Use Cases → Traceability]({{ '/use-cases/traceability/' | relative_url }}) |

---

## Read by Integration

How Provasign hooks into the tools you already use. `provasign init` auto-detects each and writes its MCP config — see [Features → Works with your agent]({{ '/features/#works-with-your-agent' | relative_url }}) and [Agent Setup]({{ '/setup/' | relative_url }}).

| Tool | Setup |
|------|-------|
| Claude Code | [Agent Setup]({{ '/setup/' | relative_url }}) |
| GitHub Copilot (VS Code) | [Agent Setup]({{ '/setup/' | relative_url }}) |
| Cursor | [Agent Setup]({{ '/setup/' | relative_url }}) |
| Codex CLI | [Agent Setup]({{ '/setup/' | relative_url }}) |
| Windsurf | [Agent Setup]({{ '/setup/' | relative_url }}) |
| Zed | [Agent Setup]({{ '/setup/' | relative_url }}) |

---

## Status

The site is organized around Provasign. Every link on this page points to a live page; the open-source tools (Grove, Prism, Fuse) are documented in their repository READMEs, with Grove also covered in [Architecture]({{ '/architecture/' | relative_url }}).

| Area | Pages |
|------|-------|
| Product | [Overview]({{ '/provasign/' | relative_url }}), [Why Provasign]({{ '/why/' | relative_url }}), [How It Works]({{ '/how-it-works/' | relative_url }}), [Architecture]({{ '/architecture/' | relative_url }}), [Features]({{ '/features/' | relative_url }}) |
| Use cases | [Security]({{ '/use-cases/security/' | relative_url }}), [Audit]({{ '/use-cases/audit/' | relative_url }}), [Change Management]({{ '/use-cases/change-management/' | relative_url }}), [Traceability]({{ '/use-cases/traceability/' | relative_url }}) |
| Get started | [Installation]({{ '/installation/' | relative_url }}), [Agent Setup]({{ '/setup/' | relative_url }}) |
| Reference | [Comparisons]({{ '/comparisons/' | relative_url }}), [FAQ]({{ '/faq/' | relative_url }}), [Troubleshooting]({{ '/troubleshooting/' | relative_url }}), [Other Developer Tools]({{ '/other-tools/' | relative_url }}) |

---

## Contributing to the Docs

These docs live in [`docs/`](https://github.com/provasign/provasign/tree/main/docs) in the main repository. PRs welcome.

Quality bar:
- **Real numbers** over adjectives. "35–92% token savings" beats "huge savings."
- **Honest tradeoffs.** Every product has things it doesn't do well. Say so.
- **Specific competitors.** Vague "other tools in the space" is worth nothing. Name names.
- **One audience per page.** A doc trying to talk to a CFO and a developer at the same time talks to neither.
