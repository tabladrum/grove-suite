# Grove Suite

**Four painful things that happen when you start using AI coding agents seriously. Grove Suite fixes all four.**

---

**Your agent is burning tokens on the wrong code.**
It reads the files you pointed at, guesses at what else matters, and spends your entire context budget on distant code that has nothing to do with the task. Critical dependencies it needed are missing. The output reflects that. You're paying for noise and getting hallucinations in return.
→ *[Prism](prism/README.md) delivers graph-ranked context: 35–92% fewer tokens on first reads, ~99% on re-reads.*

---

**Your agent and your developer just conflicted on the same file. For the third time today.**
One changed `Login()`. The other changed `validatePassword()`. Different functions. Different logic. Git declared a conflict anyway — they were on adjacent lines. A developer stopped, opened a merge tool, spent 20 minutes resolving something that was never actually conflicting.
→ *[Fuse](fuse/README.md) understands symbols, not lines. ~85% of those conflicts resolve automatically.*

---

**Your agent opened a PR. CI found three issues. You sent it back. CI found two more. Repeat.**
The quality gates live at the end of the pipeline — CI, PR review — but the agent is at the beginning. By the time it sees a finding, it has lost context. So you loop. Each loop costs 5–15 minutes of human attention to triage, explain, and re-trigger.
→ *[Relay](relay/README.md) moves the gates into the agent loop. `relay_check` returns structured findings in under 10 seconds. The agent self-corrects before opening a PR.*

---

**A security audit asks: what did the agent do, and who certified it was safe?**
The PR says "refactor auth." CI shows green. The agent session is gone. The original prompt is gone. Nobody knows which tests ran against the changed symbols, what the static analysis found, or what the user actually asked the agent to do.
→ *[Relay](relay/README.md) commits the original prompt as a YAML intent. Every admission is Ed25519-signed. `relay cert replay <id>` re-runs the gates at any time and tells you exactly what passed.*

---

All four products share one foundation: **[Grove](grove/README.md)** — a persistent knowledge graph of your codebase. 11 languages, 8 edge types, BFS traversal, delta indexing by git blob SHA. The graph that makes Prism's ranking possible, Fuse's conflict resolution accurate, and Relay's test selection meaningful.

---

## Documentation

| Who you are | What to read |
|-------------|--------------|
| **Developer** wanting to install and use | [Get Started](#get-started) below, then the per-product READMEs |
| **Anyone wanting the case for it** | [Why Grove Suite](docs/why.md) — the founder pitch |
| **Evaluating against alternatives** | [Comparisons](docs/comparisons.md) — vs Copilot, Sourcegraph, AI merge tools, CodeRabbit, Devin |
| **Top questions** | [FAQ](docs/faq.md) — technical, security, business, audit |
| **Diagnosing an issue** | [Troubleshooting](docs/troubleshooting.md) |
| **Inter-product API and security model** | [Architecture](Architecture.md) |
| **Full doc map** | [docs/](docs/README.md) |

---

## The Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Grove Suite                             │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                          Grove                            │  │
│  │  Tree-sitter (11 languages) · SQLite WAL · BFS traversal  │  │
│  │  8 edge types · delta SHA indexing · MCP + HTTP + gRPC   │  │
│  └────────────────────────┬──────────────────────────────────┘  │
│                           │  HTTP + bearer token                │
│              ┌────────────┼─────────────────┐                   │
│              ▼            ▼                 ▼                   │
│  ┌──────────────┐ ┌─────────────┐ ┌──────────────────────────┐ │
│  │    Prism     │ │    Fuse     │ │          Relay           │ │
│  │              │ │             │ │                          │ │
│  │ 5-signal     │ │ IntelliMerge│ │ relay_check (sub-10s)   │ │
│  │ ranking      │ │ 7-phase     │ │ relay_certify (full)    │ │
│  │ token budget │ │ pipeline    │ │ Ed25519 admission       │ │
│  │ progressive  │ │ ~85% auto-  │ │ Intent-ID: audit trail  │ │
│  │ disclosure   │ │ resolution  │ │ cert replay             │ │
│  └──────────────┘ └─────────────┘ └──────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

---

## Get Started

### Better context, fewer tokens (5 minutes)

```bash
git clone https://github.com/tabladrum/grove-suite && cd grove-suite
cd grove && make install && cd ..
cd prism && make install && cd ..

cd /your/project
prism init      # detects Claude Code / Copilot / Cursor / Codex / Windsurf / Zed
                # writes MCP config + agent steering instructions automatically
prism index     # initial index — subsequent runs only touch changed files
                # restart your coding tool to pick up the MCP server
prism savings   # watch token savings accumulate in real time
```

### Automatic merge conflict resolution (2 minutes)

```bash
cd /grove-suite && cd fuse && make install && cd ..

cd /your/project
fuse install    # registers driver in ~/.gitconfig
echo "*.go merge=fuse" >> .gitattributes
echo "*.ts merge=fuse" >> .gitattributes
echo "*.py merge=fuse" >> .gitattributes
# git merge now resolves symbol-level conflicts automatically
```

### Certified agent delivery with full audit trail (10 minutes)

```bash
cd /grove-suite && cd relay && make install && cd ..

cd /your/project
relay init --stack=go-microservice   # scaffolds .relay/, generates Ed25519 key
                                     # writes agent instructions + MCP config for
                                     # Claude Code, Copilot, Cursor, Codex CLI,
                                     # VS Code, Windsurf, Zed, and more
relay hook install                   # git pre-push backstop
git add .relay/ && git commit -m "Add Relay configuration"
# From now on: agent self-corrects before PRs, every commit is signed and traceable
```

---

## Performance

Numbers on real hardware (macOS, 2026-05-27):

| Project | Files | Grove index | Query latency | Fuse merge | Relay pre-flight |
|---------|------:|------------:|--------------:|-----------:|-----------------:|
| Small | 61 | 0.06 s | 6 ms | < 1 s | < 10 s |
| Medium | 801 | 0.85 s | 6 ms | < 1 s | < 10 s |
| Large | 4,501 | 11.6 s | 9 ms | < 1 s | < 10 s |
| Monorepo | 9,901 | 34.0 s | 61 ms | < 1 s | < 10 s |

After the first index, unchanged files are never re-parsed. One-file change on a 9,901-file repo: milliseconds.

**Prism token savings** (progressive disclosure across a session):

| | First read | Re-read (same session) |
|--|:----------:|:----------------------:|
| Relevance-filtered symbols | 35–92% saved | ~99% saved (sha-pointer) |
| All symbols | 0% saved | ~99% saved (sha-pointer) |

---

## Build Order

```bash
cd grove && make install   # foundation — required by everything else
cd prism && make install
cd fuse  && make install
cd relay && make install
```

Prism, Fuse, and Relay each auto-start Grove if unreachable at `$GROVE_URL` (default `http://localhost:7777`).

---

## Repository Layout

```
grove-suite/
├── grove/              Go — persistent code knowledge graph
├── prism/              Go — token-optimized context delivery
│   └── vscode-extension/  TypeScript — VS Code native extension
├── fuse/               Go — semantic git merge driver
├── relay/              Go — certified agent delivery platform
├── astkit/             Go — shared AST utilities
├── Architecture.md     Inter-product contracts and data flows
└── go.work             Go workspace
```

All four products: MIT licensed · single binary · no runtime dependencies beyond Go 1.22+ and git.

---

## Security

All HTTP servers bind to `127.0.0.1`. Grove generates a random 64-char token at `.grove/.token` (mode 0600) and requires it on every non-health request — Prism, Fuse, and Relay read it automatically. Relay's Ed25519 admission key lives at `~/.relay/keys/admission.ed25519` (mode 0600), generated once on `relay init`.

See [Architecture.md](Architecture.md) for full inter-product API contracts and the security model.

---

*Built on [Tree-sitter](https://tree-sitter.github.io), [SQLite](https://sqlite.org), and [Model2Vec](https://github.com/MinishLab/model2vec) — no cloud, no GPU, no subscription.*
