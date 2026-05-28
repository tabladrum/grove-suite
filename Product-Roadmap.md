# Product Suite: Naming, Vision & Sequenced Roadmap

**Last Updated:** May 23, 2026
**Status:** Pre-build — architecture validated, implementation not started

-----

## The Suite at a Glance

|Product  |Old Name     |New Name |CLI    |What It Does                                             |
|---------|-------------|---------|-------|---------------------------------------------------------|
|Step 0   |(new)        |**Grove**|`grove`|Universal code intelligence graph — the shared foundation|
|Product 1|gctx         |**Prism**|`prism`|Token-optimized context delivery for AI agents           |
|Product 2|git-semantic |**Fuse** |`fuse` |Intelligent semantic merge driver                        |
|Product 3|Next-gen CICD|**Relay**|`relay`|Intent-driven autonomous software delivery platform      |

-----

## Why These Names

**Grove** — A grove is a collection of trees. Tree-sitter parses trees. Your graph is a forest of symbol relationships. Short, spellable, memorable. The CLI command `grove` is clean and available. A grove is also something that grows — fitting for a living index of a codebase.

**Prism** — A prism takes a full spectrum of light and refracts only the wavelengths that matter. That’s exactly what this product does: takes a full codebase and delivers only the relevant context to an AI agent. Clean metaphor, easy to say, nothing else in this space uses it.

**Fuse** — Fuses join two things intelligently. A fuse also protects a circuit from damage — appropriate for a tool that prevents bad merges from breaking production. Short, strong, one syllable.

**Relay** — Intents flow from human to agent to production like a relay race — each stage hands off to the next with precision. A relay also implies speed and coordination, which is the core promise of the platform.

-----

## The Dependency Map

```
         GROVE (Step 0)
         ─────────────
         The graph engine
         Powers everything below

         ┌──────┬──────┬──────┐
         ▼      ▼      ▼      ▼
       PRISM   FUSE        RELAY
   (context) (merge)  (delivery)

PRISM uses Grove for:        FUSE uses Grove for:         RELAY uses Grove for:
- Symbol ranking             - Cross-file merge context   - ICR computation
- Dependency traversal       - Breaking change detection  - Conflict detection
- Budget-aware selection     - Impact blast radius        - Test selection
- Session deduplication      - Dependency awareness       - Agent isolation
                                                          - Admission control
```

Everything is built on Grove. Grove is the reason the other three products are coherent rather than three unrelated tools.

-----

# Step 0: Grove

## One-Line Pitch

*Structural memory for AI coding agents — a persistent, queryable graph of any codebase exposed via MCP, CLI, and HTTP.*

## The Problem It Solves

Every AI coding agent — Claude Code, Cursor, Copilot, Codex — starts each session blind. It greps around, reads files one at a time, burns context window on exploration before it can do any real work. When it makes a change it cannot know what else it might break. When two agents work simultaneously they have no awareness of each other.

Grove gives agents permanent structural memory. It answers three questions instantly:

1. **“What is this code?”** — symbols, relationships, dependencies
1. **“What does changing this affect?”** — blast radius, downstream impact, test coverage
1. **“Can I safely change this right now?”** — isolation regions, conflict detection

## Architecture

```
┌──────────────────────────────────────────┐
│            GROVE CORE (Go binary)        │
│                                          │
│  ┌──────────┐  ┌──────────┐  ┌────────┐ │
│  │  Parser  │  │  Graph   │  │ Query  │ │
│  │  Engine  │  │  Store   │  │ Engine │ │
│  │ 26 langs │  │ SQLite   │  │  BFS   │ │
│  │tree-sitter│  │delta idx │  │  ICR   │ │
│  └──────────┘  └──────────┘  └────────┘ │
└─────────────────────┬────────────────────┘
                      │
         ┌────────────┼────────────┐
         ▼            ▼            ▼
     MCP Server    REST/gRPC      CLI
     (stdio/SSE)   (HTTP API)  (terminal)
         │            │            │
         ▼            ▼            ▼
    Claude Code   Any Agent    Any Script
    Cursor        or Service   or Pipeline
    Copilot/Wind
```

**One binary. Three interfaces. Zero external dependencies.**

SQLite for storage — embedded, zero-ops, single file. No Redis, no Postgres, no setup. Install one binary and it works immediately.

## The Graph

### Nodes

- Functions, methods, classes, interfaces, types, constants
- Files, modules, packages
- Intents (YAML — used by Relay later)

### Edges (8 types)

```
defines      → file contains symbol        (confidence: 1.0)
imports      → symbol depends on file      (confidence: 0.9)
calls        → symbol invokes symbol       (confidence: 0.85 AST / 0.6 regex)
extends      → inheritance                 (confidence: 0.85)
implements   → interface satisfaction      (confidence: 0.85)
uses-type    → type dependency             (confidence: 0.5)
tests        → test covers symbol          (confidence: 0.8)
contains     → parent/child symbol         (confidence: 1.0)
```

Every edge carries:

- **Confidence score** (0.0–1.0)
- **Delta tracking** — git blob SHA, only re-parse what changed
- **Temporal signals** — edit frequency, recency from git log
- **Lock state** — concurrency awareness for Relay’s ICR system

## CLI

```bash
# Setup
grove init                          # initialize .grove workspace
grove index [dir]                   # build/update graph (delta-aware)
grove status                        # graph stats, staleness report

# Query
grove query "add rate limiting to auth"     # intent → relevant symbols
grove impact src/auth/login.go:45           # blast radius of a symbol
grove deps src/payments/service.go          # full dependency chain
grove tests src/api/handler.go              # minimal test set for a file
grove symbols AuthService                   # find symbol by name

# Agent coordination (used by Relay later)
grove icr "add OAuth to user service"       # compute isolated change region
grove conflicts icr-a.json icr-b.json       # detect conflicts between ICRs
grove lock <icr-id>                         # acquire lock for agent execution
grove unlock <icr-id>                       # release lock

# Server
grove serve                         # MCP server (stdio)
grove serve --port 7777             # MCP + HTTP server
grove serve --mode http             # HTTP only (for remote/team use)
```

## MCP Tools

|Tool             |What It Does                       |Used By     |
|-----------------|-----------------------------------|------------|
|`grove_index`    |Index or re-index a repo           |All products|
|`grove_query`    |Find symbols relevant to a task    |Prism, Relay|
|`grove_impact`   |Blast radius of a symbol change    |Fuse, Relay |
|`grove_deps`     |Full dependency chain              |Prism, Fuse |
|`grove_tests`    |Minimal test set for a changeset   |Relay       |
|`grove_icr`      |Compute safe isolated change region|Relay       |
|`grove_conflicts`|Detect conflicts between two ICRs  |Relay       |
|`grove_symbols`  |Full symbol lookup by name         |Prism, Fuse |

An agent with these 8 tools stops guessing what it might break. It knows before it touches a line.

## VS Code Extension

Thin TypeScript wrapper that shells out to the Grove binary.

- Auto-indexes on file save
- Registers all MCP tools with Copilot Agent mode via `vscode.lm.registerTool`
- Sidebar showing graph stats and staleness
- Inline decorations showing impact score on symbol hover
- Works alongside Cursor, Copilot, Windsurf — doesn’t replace them

## Open Source Strategy

**Everything is open source from day one. MIT license. No feature gating.**

You are not selling the graph. You are building the standard.

### Launch sequence

- **Week 1** — GitHub release with 60-second demo GIF: Claude Code using Grove to understand a 10,000-line codebase before making a change
- **Week 1** — Hacker News: *“Show HN: Grove — structural memory for AI coding agents”*
- **Week 1** — Submit to official MCP server registry
- **Week 2** — Blog: *“Why your AI agent is blind and how to fix it”*
- **Week 2** — Post in Claude Code Discord, Cursor Discord, r/ClaudeAI, r/LocalLLaMA
- **Week 3** — Blog: *“How we cut Claude Code tool calls by 80% with a code graph”*
- **Week 4** — VS Code extension on marketplace
- **Ongoing** — Amplify every community build on top of Grove loudly

### Why open source wins here

The more people use it, the better the confidence calibration gets. More languages get contributed by community. More integrations appear. By the time competitors catch up to the feature set you have the ecosystem. The three products built on top become the commercial layer.

## Immediate Developer Use Cases

### Use case 1: Stop your agent from breaking things

```bash
grove serve &
# In Claude Code:
"Refactor the authentication module"
# Agent calls grove_impact before every edit
# Knows exactly what tests to run, what might break
# No more "oops I changed something unrelated"
```

### Use case 2: Instant codebase onboarding

```bash
cd unfamiliar-project && grove index .
grove query "how does user authentication work"
# Ranked map of every relevant symbol
# New developer or agent understands structure in seconds not hours
```

### Use case 3: Safe parallel agents

```bash
# Agent A: working on auth
grove icr "add OAuth provider support" → locks auth/* symbols

# Agent B: working on payments
grove icr "add Stripe webhooks"
# → detects no overlap → proceeds safely
# → if overlap detected → queues automatically
```

### Use case 4: Smarter CI pipelines

```yaml
# .github/workflows/ci.yml
- name: Selective test execution
  run: |
    grove index .
    grove tests $(git diff --name-only origin/main) > affected.txt
    go test $(cat affected.txt)
# Only runs tests actually affected by the change
# 60-80% faster CI on average
```

### Use case 5: Blast radius before code review

```bash
git diff main..feature | grove impact --stdin
# Output:
# This change affects 14 downstream symbols
# 3 tests must pass: TestAuth, TestLogin, TestSession
# WARNING: export signature change breaks 2 consumers
```

## Build Order

**Month 1 — Core that works**

- Graph data structures and SQLite persistence
- Tree-sitter parsing: Go, TypeScript, Python (three languages, done right)
- All 8 edge types with confidence scoring
- Delta indexing via git blob SHA
- CLI: `init`, `index`, `query`, `impact`, `deps`, `symbols`
- Ship it to GitHub. Get feedback.

**Month 2 — Agent integration**

- MCP server with all 8 tools
- ICR computation algorithm
- Conflict detection (structural layer)
- `grove serve` working cleanly with Claude Code
- HN launch + demo videos

**Month 3 — Ecosystem**

- VS Code extension
- Remaining 23 languages
- HTTP API for remote/team use
- `grove tests` and `grove lock/unlock`
- First integrations with Prism and Fuse

-----

# Product 1: Prism

*Previously: gctx*

## One-Line Pitch

*Token-optimized context delivery — gives AI agents only the code that matters, at the right fidelity, without re-delivering what they’ve already seen.*

## What Changes From gctx

gctx built its own internal graph from scratch. Prism is a client of Grove. The graph module in gctx gets replaced entirely by Grove queries. Prism becomes purely the context delivery layer — ranking, budget allocation, compression, session deduplication, and MCP tool exposure.

This means Prism is smaller, faster to build, and dramatically smarter because it has Grove’s full persistent graph rather than a per-session in-memory one.

## Core Value

- 35–92% fewer tokens on file reads
- 99.7% savings on re-reads via session deduplication
- Progressive disclosure: full → signature → reference based on relevance score
- Budget-aware: allocates tokens across target/dependency/test/doc/summary categories

## How It Uses Grove

```
prism query "implement OAuth" 
  → calls grove_query for ranked symbols
  → calls grove_deps for dependency chain
  → applies budget allocation (target 35%, deps 25%, tests 20%...)
  → applies progressive disclosure (full/signature/reference)
  → delivers compressed, deduplicated context pack to agent
```

## CLI

```bash
prism init
prism index [dir]           # delegates to grove index
prism query "task desc"     # ranked, budgeted context pack
prism read src/auth.go      # code-aware compressed file read
prism search AuthService    # symbol search
prism savings               # session token savings dashboard
prism serve                 # MCP server (8 tools)
prism feedback              # rate context quality (calibration)
```

## When To Build

**After Grove Month 2.** Prism needs a working Grove MCP server before it can replace gctx’s internal graph. Estimated build time: 6–8 weeks on top of Grove.

-----

# Product 2: Fuse

*Previously: git-semantic*

## One-Line Pitch

*Intelligent semantic merge driver — understands code structure so AI-generated changes merge without conflicts 80%+ of the time.*

## What Changes From git-semantic

git-semantic built per-file symbol graphs at merge time. Fuse delegates all graph operations to Grove, gaining persistent cross-file memory and full project context that git-semantic never had. Merge decisions become dramatically smarter — Fuse knows the full dependency chain, not just the two files being merged.

## Core Value

- Auto-merges 80%+ of AI-generated conflicts that would choke traditional Git
- Symbol-level three-way merge (not line-based)
- Breaking change detection with blast radius
- Generates AI-ready prompts for the 20% it cannot auto-resolve
- Zero external API calls — all local, privacy-first
- 26 language support

## How It Uses Grove

```
git merge feature-branch
  → conflict in src/auth/login.py
  → fuse calls grove_impact on conflicting symbols
  → gets full dependency context, breaking change analysis
  → calls grove_deps for cross-file awareness
  → applies merge strategy with full project knowledge
  → auto-merges or generates targeted AI prompt
```

## CLI

```bash
fuse merge %O %A %B -- %P   # git merge driver invocation
fuse install                 # register as git merge driver
fuse status                  # show pending conflicts
fuse preview                 # preview merge without applying
fuse resolve <conflict-id>   # complete AI-assisted resolution
fuse cache                   # manage dependency cache
```

## When To Build

**After Grove Month 2, parallel with Prism.** Fuse needs `grove_impact`, `grove_deps`, and `grove_symbols`. Estimated build time: 6–8 weeks on top of Grove.

-----

# Product 3: Relay

*Previously: Next-gen CICD / Intent-Driven Software Delivery Platform*

## One-Line Pitch

*Intent-driven autonomous software delivery — humans define what should change, agents implement it safely, the platform certifies and ships it.*

## What Changes From the Original Spec

The original spec included a full CKG service as a component. That component is now Grove — battle-tested in production by thousands of developers across Prism and Fuse before Relay is built. Every phase of the Relay pipeline becomes a Grove query. The platform is smaller to build, more reliable from day one, and easier to explain.

## Core Value

- Replaces Jira + GitHub PRs + Jenkins + deployment pipelines
- No branches — ICR isolation (via Grove) replaces branching entirely
- Agents execute in isolated K8s pods within Grove-computed change regions
- Machine certification replaces traditional CI
- Progressive canary deployment with intent-metric validation
- Full audit trail: every commit traces to an intent and a certificate

## How It Uses Grove

```
Intent: "Add rate limiting to /api/auth/*"
  → relay calls grove_icr → computes exclusive change region
  → relay calls grove_conflicts → checks against active agents
  → agent executes within ICR boundaries
  → relay calls grove_tests → selects certification test suite
  → admission controller uses grove_impact → validates no regressions
  → canary deployment begins
```

## The Pipeline

```
Human defines Intent (YAML)
        ↓
Granularity check (GS >= 0.7)
        ↓
grove_icr → compute isolated change region
        ↓
grove_conflicts → check against active agents
        ↓
Redis lock acquired → agent pod dispatched
        ↓
Agent executes within ICR boundaries
        ↓
Optional human review gate
        ↓
Machine certification (build + test + security + semantic)
        ↓
Admission controller → rebase + commit to linear main
        ↓
Canary → Progressive → Full deployment
        ↓
Production validates intent preservation
```

## When To Build

**After Grove is stable AND Prism/Fuse have real users.** Relay is the most complex product and the hardest sell. Building it after Grove has traction means you have validated the core technology, have a developer community, and have evidence that the graph-based approach works in production. Estimated build time: 12–16 weeks on top of Grove. This is a startup-scale effort requiring a team.

-----

# Full Sequenced Roadmap

## Phase 0 — Grove Core (Months 1–3)

*Goal: Working graph engine used by real developers with real codebases*

|Month|Milestone                                                                                                              |
|-----|-----------------------------------------------------------------------------------------------------------------------|
|1    |Graph engine, SQLite persistence, tree-sitter parsing (Go/TS/Python), all 8 edge types, delta indexing, CLI basics     |
|2    |MCP server, ICR computation, conflict detection layer 1, `grove serve` working with Claude Code, GitHub launch, HN post|
|3    |VS Code extension, 23 remaining languages, HTTP API, `grove tests` + locking, first community contributors             |

**Exit criteria:** 500+ GitHub stars, 50+ developers using it daily, at least 3 community language contributions

-----

## Phase 1 — Prism + Fuse (Months 4–6)

*Goal: Two working products built on Grove, validating the platform concept*

|Month|Milestone                                                                             |
|-----|--------------------------------------------------------------------------------------|
|4    |Prism core — context delivery layer on top of Grove, replacing gctx’s internal graph  |
|5    |Fuse core — merge driver on top of Grove, replacing git-semantic’s internal graph     |
|6    |Prism + Fuse public release, cross-product integration testing, first production users|

**Exit criteria:** 10+ teams using Prism or Fuse in production, measurable token savings and merge conflict reduction data

-----

## Phase 2 — Relay Foundation (Months 7–10)

*Goal: First working end-to-end intent → agent → deployment flow*

|Month|Milestone                                                              |
|-----|-----------------------------------------------------------------------|
|7    |Intent registry + orchestrator (Grove as CKG, not rebuilt)             |
|8    |Agent execution platform (K8s operator, ICR enforcement, cost tracking)|
|9    |Certification engine (build + test + security + semantic)              |
|10   |Admission controller + canary deployment engine                        |

**Exit criteria:** One internal team running real intents through Relay end-to-end

-----

## Phase 3 — Relay Production (Months 11–14)

*Goal: Relay ready for early enterprise customers*

|Month|Milestone                                                |
|-----|---------------------------------------------------------|
|11   |Dashboard + review UX, CLI polish, human review gate     |
|12   |Integration testing, chaos testing, performance hardening|
|13   |Security hardening, SOC2 prep, enterprise auth (OIDC/SSO)|
|14   |First paying customer onboarded                          |

-----

## The Commercial Model

**Grove** — Always free, always open source. This is your distribution engine and your moat.

**Prism** — Open source core, paid managed cloud service for teams. Teams pay for hosted Prism with shared graph state, multi-repo support, and team analytics. Target: $20–50/developer/month.

**Fuse** — Open source core, paid enterprise features (compliance reporting, mandatory review policies, audit exports). Target: $15–30/developer/month.

**Relay** — Commercial product, self-hosted or managed cloud. This is your enterprise play. Target: $100–200/developer/month. Replaces Jira + CI/CD = significant budget available.

**The flywheel:**

```
Grove gets adopted widely (free)
    ↓
Developers discover Prism and Fuse (free core)
    ↓
Teams want managed hosting and enterprise features (paid)
    ↓
Enterprises want the full Relay platform (enterprise contract)
    ↓
Grove gets more contributors and better coverage
    ↓
repeat
```

-----

## Competitive Position

|Tool             |What it does                     |What they miss                               |
|-----------------|---------------------------------|---------------------------------------------|
|GitNexus         |Graph RAG for context            |No concurrency, no ICR, no merge intelligence|
|CodeGraphContext |MCP + graph for context          |Python only, no coordination layer           |
|gctx / Prism v0  |Token optimization               |No persistent graph, no multi-agent          |
|Greptile         |Codebase search + review         |Cloud-only, no structural isolation          |
|Sourcegraph      |Enterprise code search           |Not agent-native, no ICR concept             |
|**Grove + Suite**|**Full agent coordination layer**|**Nothing comparable exists**                |

The gap nobody has filled: a graph that serves not just context delivery but **concurrent agent coordination and isolation**. That is Grove’s unique position, and it is what makes Relay possible.

-----

## The One Thing To Remember

Every competitor is building context tools.
You are building coordination infrastructure.

Context tools make one agent smarter.
Coordination infrastructure makes ten agents safe.

That is a fundamentally different and significantly more valuable category. Grove is the entry point. Relay is the destination. Prism and Fuse prove the graph works in production before you bet the company on the platform.

Start with `grove init`.