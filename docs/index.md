---
title: Home
layout: default
nav_order: 1
description: "Grove Suite — certified delivery infrastructure for AI coding agents. The layer nobody else built."
permalink: /
---

# Grove Suite
{: .fs-9 }

**The infrastructure layer beneath AI coding agents. The part nobody else built.**
{: .fs-6 .fw-300 }

[Agent Setup]({{ '/setup/' | relative_url }}){: .btn .btn-primary .fs-5 .mb-4 .mb-md-0 .mr-2 }
[Manual Install]({{ '/installation/' | relative_url }}){: .btn .fs-5 .mb-4 .mb-md-0 .mr-2 }
[View on GitHub](https://github.com/tabladrum/grove-suite){: .btn .fs-5 .mb-4 .mb-md-0 }

---

## The problem nobody solved

AI agents write code. 41% of code in AI-assisted repos is now AI-generated, and that number is accelerating.

Here's what still doesn't exist: **certified delivery infrastructure on the other side.**

Every agent — Devin, Cursor, GitHub Copilot Workspace, Claude Code — still delivers through the same PR → CI loop built for humans. When an agent opens a PR, CI finds issues, you send it back, CI finds more. Each loop costs 5–15 minutes of human attention. The agent has lost context by then.

More critically: when a security audit asks *what did the agent do, and who certified it was safe?* — the PR says "refactor auth," CI shows green, the agent session is gone. There is no answer.

**Relay is the answer. No other product in this space does what it does.**

---

## Relay

**Certified delivery for AI coding agents. Greenfield.**

Before any agent-produced code touches main, Relay:

- **Captures the original user prompt** as a signed YAML intent — permanent record of what the agent was asked to do
- **Runs quality gates in the agent loop** — build, tests, coverage, secrets, SAST, dep audit — before the commit, in under 10 seconds
- **Issues an Ed25519-signed certificate** proving which gates ran, which tool versions, what passed — linked to the commit forever
- **Enables audit replay** — `relay cert replay <id>` re-runs gates against a 6-month-old commit and proves they still hold

The agent self-corrects before opening a PR. The CI loop drops from 3–5 iterations to 0–1. Every commit has a cryptographic proof of quality that survives the agent session.

```bash
relay init --stack=auto   # scaffolds policy, generates Ed25519 key,
                          # writes instructions into CLAUDE.md / .cursorrules / etc.
relay hook install        # pre-push backstop
```

The agent then uses Relay via MCP automatically:

| Agent calls | What happens |
|-------------|-------------|
| `relay_intent_open` | Captures the user prompt as a YAML artifact |
| `relay_check` | Structured findings in < 10 s; agent self-corrects |
| `relay_submit` | Ed25519-signed certificate issued; commit admitted |

[Learn more about Relay →]({{ '/relay/' | relative_url }})

---

## Grove — the foundation

Relay's certification is only as smart as the knowledge behind it. Grove is that knowledge.

**[Grove]({{ '/grove/' | relative_url }})** indexes your codebase into a persistent SQLite graph — 11 languages, 8 typed edge types, BFS traversal. When Relay certifies a commit, Grove tells it exactly which tests cover the changed symbols and what the real blast radius is. When it's done, Grove keeps the graph live with delta indexing — files whose git blob SHA hasn't changed are never re-parsed.

Grove is the only product in the suite with no dependencies. Everything else — Relay, Prism, Fuse — is a client of Grove.

---

## Prism — better context, fewer tokens

Agents that understand the code work better. Agents that read everything run out of context budget before getting there.

**[Prism]({{ '/prism/' | relative_url }})** delivers graph-ranked context to any MCP-capable agent: the symbols that matter for the task, their dependencies, their tests — ranked by graph distance, semantic similarity, recency, and edit frequency. 35–92% fewer tokens on first reads. ~99% on re-reads.

5-minute install. Visible token savings on the first task.

---

## Fuse — when two agents edit the same file

At scale, agents edit in parallel. Two agents touch the same file. Different functions. Adjacent lines. Git declares a conflict on code that never actually conflicted.

**[Fuse]({{ '/fuse/' | relative_url }})** replaces line-based merge with symbol-level understanding: parse all three versions as ASTs, merge at the symbol boundary, check cross-file blast radius with Grove. ~85% of false git conflicts auto-resolve.

---

## How they fit together

```
     AI Coding Agent
(Claude Code · Cursor · Devin · Copilot)
          │  MCP tools
   ┌──────┼──────────────────────┐
   ▼      ▼                      ▼
 Prism   Fuse                  Relay
 Better  Symbol-level          Certified delivery
 context merge                 Intent capture
                               Ed25519 certs
                               Audit replay
   └──────┴──────────────────────┘
                   │
                 Grove
         Knowledge graph
         11 languages · 8 edges
                   │
             Your codebase
```

---

## Get Started

**Fastest — let your agent do it:**

```bash
# In Claude Code:
claude "Follow the setup instructions at https://tabladrum.github.io/grove-suite/assets/AGENT_SETUP_PROMPT.md"
```

The agent detects your platform, fetches the latest release, asks which products you want, verifies checksums, and wires everything in. → [How it works]({{ '/setup/' | relative_url }})

**Manual install from source:**

```bash
git clone https://github.com/tabladrum/grove-suite && cd grove-suite
cd grove && make install && cd ..
cd relay && make install && cd ..   # the main event
cd prism && make install && cd ..   # optional: better context
cd fuse  && make install && cd ..   # optional: symbol-level merge
```

**Pre-built binaries:** [Installation guide]({{ '/installation/' | relative_url }}) covers macOS, Linux, and Windows downloads from GitHub Releases.

---

## Read More

- [**Why Grove Suite**]({{ '/why/' | relative_url }}) — the founder's pitch: the problem, the bet, what we deliberately didn't build
- [**Relay**]({{ '/relay/' | relative_url }}) — the full story on certified delivery
- [**Comparisons**]({{ '/comparisons/' | relative_url }}) — honest comparisons vs Copilot, Sourcegraph, CodeRabbit, Devin, Sigstore
- [**FAQ**]({{ '/faq/' | relative_url }}) — technical, security, business, and audit questions
- [**Agent Setup**]({{ '/setup/' | relative_url }}) — let your AI agent install and configure everything
- [**Documentation home**]({{ '/docs/' | relative_url }}) — the full doc map

---

*Grove, Prism, and Fuse are MIT licensed. Relay is AGPL-3.0 licensed. No cloud. No telemetry. No GPU. Your code never leaves your machine.*
