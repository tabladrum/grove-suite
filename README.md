# Grove Suite

**AI agents are writing your code. The infrastructure around them is still built for humans.**

---

Picture this. Your team shipped a feature on Thursday. A security audit on Monday finds a credential leak in that commit. Your VP of Engineering asks three questions:

1. What was the agent actually told to do?
2. Which tests ran before this reached production?
3. Who certified it was safe to ship?

Nobody knows. The agent session is gone. The PR says "add authentication." CI shows green. That's all you have.

This is the gap Grove Suite fills — not by building a better agent, but by building the infrastructure beneath every agent you already use.

---

## Four Tools. One Foundation.

**[Grove](grove/README.md)** — A persistent knowledge graph of your codebase. Not grep. Not an LSP. A graph with 8 edge types, BFS traversal, and delta indexing by git blob SHA. Grove answers "what breaks if I change this?" with a blast radius — not a list of filename matches. Every other tool in this suite is built on top of it.

**[Prism](prism/README.md)** — Graph-ranked context delivery for any AI agent. Instead of dumping files into the context window and hoping, Prism queries Grove, scores candidates across 5 signals, allocates a token budget, and returns exactly what matters for the current task — 35–92% fewer tokens on first reads, ~99% on re-reads (sha-pointer instead of content). Works with Claude Code, Copilot, Cursor, Codex CLI, Windsurf, Zed, and any MCP-capable tool.

**[Fuse](fuse/README.md)** — A symbol-aware Git merge driver. When two agents (or an agent and a human) change different functions in the same file, git declares a conflict. Fuse doesn't — it knows those are different symbols, resolves them automatically, and only stops for conflicts that are genuinely ambiguous. ~85% auto-resolution on incremental changes.

**[Relay](relay/README.md)** — Certified delivery for agent-produced code. The agent calls `relay_check` in its loop — structured findings (file, line, rule, severity) flow back so it self-corrects before opening a PR. When the code is ready, `relay_certify` runs the full suite, signs the result with Ed25519, and admits it as a linear commit. The user's original prompt is committed alongside the code as a YAML intent, linked to every cert via `Intent-ID:` trailer. Six months later, you can still answer all three questions.

---

## The Loop Relay Breaks

This is what AI-assisted development looks like at most teams today:

```
Prompt → agent codes → opens PR → CI catches an issue
→ developer goes back to agent → agent fixes → CI catches another issue
→ repeat 3–5× → human reviews diff with no idea what the original prompt was
→ merged → audit question 6 months later → nobody knows
```

This is what it looks like with Grove Suite:

```
Prompt → intent captured → agent codes with Prism context
→ relay_check in loop → agent self-corrects → relay_certify
→ Ed25519-signed cert: build ✓ tests ✓ coverage ✓ SAST ✓
→ linear commit: Intent-ID: INT-0042, Certificate: relay-cert-abc123
→ PR already certified — CI is a formality, not a gatekeeper
→ audit question 6 months later → relay cert show INT-0042 → full picture
```

---

## Get Started

### Smarter context for your agent (5 minutes)

```bash
# Build Grove first — everything depends on it
git clone https://github.com/tabladrum/grove-suite && cd grove-suite
cd grove && make install && cd ..
cd prism && make install && cd ..

# Wire into your project
cd /your/project
prism init      # detects Claude Code / Copilot / Cursor / Codex / Windsurf / Zed
                # writes MCP config + agent steering instructions
prism index     # initial index (delta indexing — subsequent runs are near-instant)
                # restart your coding tool to pick up the MCP server
prism savings   # see token savings accumulate in real time
```

### Symbol-aware merge (2 minutes)

```bash
cd /grove-suite && cd fuse && make install && cd ..

cd /your/project
fuse install    # writes ~/.gitconfig merge driver entry
echo "*.go merge=fuse" >> .gitattributes
echo "*.ts merge=fuse" >> .gitattributes
echo "*.py merge=fuse" >> .gitattributes
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
# From now on your agent calls relay_check before every PR. Automatically.
```

---

## Performance

Real numbers on real hardware (macOS, 2026-05-27):

| Project size | Grove index | Query latency | Fuse merge | Relay pre-flight |
|-------------|------------:|--------------:|-----------:|-----------------:|
| 61 files | 0.06 s | 6 ms | < 1 s | < 10 s |
| 801 files | 0.85 s | 6 ms | < 1 s | < 10 s |
| 4,501 files | 11.6 s | 9 ms | < 1 s | < 10 s |
| 9,901 files | 34.0 s | 61 ms | < 1 s | < 10 s |

After the first index, unchanged files are never re-parsed. Subsequent runs on a one-file change: milliseconds.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Grove Suite                             │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                          Grove                            │  │
│  │  Tree-sitter (11 languages) · SQLite WAL · BFS traversal  │  │
│  │  8 edge types · delta SHA indexing · MCP + HTTP + gRPC   │  │
│  │  127.0.0.1:7777 (HTTP) · 127.0.0.1:7778 (gRPC)          │  │
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
│  │ progressive  │ │ ~85% auto-  │ │ Intent-ID: trail        │ │
│  │ disclosure   │ │ resolution  │ │ cert replay             │ │
│  │ :8888 (opt)  │ │ (git driver)│ │ :9000 (HTTP)            │ │
│  └──────────────┘ └─────────────┘ └──────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Build order:** Grove has no suite dependencies — build it first. Prism, Fuse, and Relay each auto-start Grove if unreachable at `$GROVE_URL` (default `http://localhost:7777`).

```bash
cd grove && make install
cd prism && make install
cd fuse  && make install
cd relay && make install
```

---

## Security

All HTTP servers bind to `127.0.0.1`. Grove generates a random 64-char token at `.grove/.token` (mode 0600) and requires it on every non-health request — Prism, Fuse, and Relay read it automatically. Relay's Ed25519 admission key lives at `~/.relay/keys/admission.ed25519` (mode 0600), generated once on `relay init`.

---

## Repository

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

All four products: MIT licensed. All four: single binary, no runtime dependencies beyond a Go 1.22+ toolchain and git.

---

*Built on [Tree-sitter](https://tree-sitter.github.io), [SQLite](https://sqlite.org), and [Model2Vec](https://github.com/MinishLab/model2vec) — no cloud, no GPU, no subscription.*
