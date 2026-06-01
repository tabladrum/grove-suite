---
title: Home
layout: default
nav_order: 1
description: "Grove Suite — the infrastructure layer that makes AI coding agents production-safe."
permalink: /
---

# Grove Suite
{: .fs-9 }

**The infrastructure layer that makes AI coding agents production-safe.**
{: .fs-6 .fw-300 }

[Agent Setup](/setup/){: .btn .btn-primary .fs-5 .mb-4 .mb-md-0 .mr-2 }
[Manual Install](/installation){: .btn .fs-5 .mb-4 .mb-md-0 .mr-2 }
[View on GitHub](https://github.com/tabladrum/grove-suite){: .btn .fs-5 .mb-4 .mb-md-0 }

---

## Four pains. Four fixes.

**Your agent burns tokens on the wrong code.** It reads the files you pointed at, guesses at what else matters, and spends your context budget on noise. → [Prism](/prism) delivers graph-ranked context: 35–92% fewer tokens on first reads, ~99% on re-reads.

**Your agent and your developer just conflicted on the same file. For the third time today.** Different functions, adjacent lines. Git declared a conflict anyway. → [Fuse](/fuse) understands symbols, not lines. ~85% of those conflicts auto-resolve.

**Your agent opened a PR. CI found three issues. You sent it back. CI found two more. Repeat.** Quality gates live at the end of the pipeline; the agent is at the beginning. → [Relay](/relay) moves the gates into the agent loop. Sub-10s findings. Agent self-corrects before any PR.

**A security audit asks: what did the agent actually do, and who certified it was safe?** The PR says "refactor auth." The agent session is gone. → [Relay](/relay) commits the original prompt as a YAML intent. Every commit is Ed25519-signed. `relay cert replay` re-runs the gates at any time.

All four products share one foundation: [Grove](/grove) — a persistent knowledge graph of your codebase.

---

## Get Started

**Fastest — let your AI agent do it:**

```bash
# In Claude Code:
claude "Follow the setup instructions at https://raw.githubusercontent.com/tabladrum/grove-suite/main/SETUP.md"

# In any other agent — paste this URL and say "follow the setup instructions":
# https://raw.githubusercontent.com/tabladrum/grove-suite/main/SETUP.md
```

The agent detects your platform, fetches the latest release, asks which products you want, verifies checksums, and wires everything into your project. → [How it works](/setup/)

**Manual install from source:**

```bash
git clone https://github.com/tabladrum/grove-suite && cd grove-suite
cd grove && make install && cd ..
cd prism && make install && cd ..

cd /your/project
prism init       # detects Claude Code / Copilot / Cursor / Codex / Windsurf / Zed
prism index      # initial index
prism savings    # watch token savings accumulate
```

**Pre-built binaries?** [Installation guide](/installation) covers macOS, Linux, and Windows downloads from GitHub Releases.

---

## Read More

- [**Agent Setup**](/setup/) — let your AI agent install and configure everything automatically
- [**Why Grove Suite**](/why) — the founder's pitch: problem, bet, what we deliberately didn't build
- [**Comparisons**](/comparisons) — honest comparisons vs Copilot semantic search, Sourcegraph, AI merge tools, CodeRabbit, Sigstore, Devin
- [**FAQ**](/faq) — top questions across technical, security, business, and audit perspectives
- [**Troubleshooting**](/troubleshooting) — common issues, how to diagnose, how to fix
- [**Documentation home**](/docs/) — the full doc map

---

## Why Local-First

Your code never leaves your machine. No telemetry. No "improving our models with your code." The Model2Vec embedding model (29 MB) is compiled into the Grove binary — no inference server, no GPU, no API key, no rate limit.

Built on [Tree-sitter](https://tree-sitter.github.io), [SQLite](https://sqlite.org), and [Model2Vec](https://github.com/MinishLab/model2vec).

---

*MIT licensed. Single binary per product. No cloud. No subscription. No GPU.*
