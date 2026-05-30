# Relay — Design Review Request

## What This Document Is

This is a request for a critical design review of **Relay**, an intent-driven software delivery platform being built as part of a larger code intelligence suite called Grove Suite.

The document gives you full context: what's been built, the thesis behind it, the architecture decisions made, and the specific questions we want challenged. Please be direct — we want genuine critique, not validation. Point out what's wrong, what's missing, what would fail in practice, and where we've made assumptions we haven't earned.

---

## The Problem We're Trying to Solve

Three observations drove this project:

**1. Nobody reviews large AI-generated PRs.**
When a developer uses a coding agent (Claude Code, Cursor, Copilot) to generate 2000–5000 lines of code, the resulting GitHub PR doesn't get meaningfully reviewed. Reviewers rubber-stamp it, skim it, or give up. The GitHub PR review UI was designed for human-to-human review of small, context-rich diffs. It breaks down at AI-generated volume. The review artifact should be the *intent* (what should change and why — reviewable by humans), not the diff (certifiable by machines).

**2. If merge conflicts are solved structurally, branches serve no purpose.**
Branches exist to isolate work-in-progress. If you solve isolation at the code-graph level — identify exactly which symbols each agent will touch, ensure no two in-flight agents touch overlapping symbols — then you've eliminated the reason branches exist. This enables a return to trunk-based development, which simplifies everything: no merge queue, no rebase fights, linear audit trail.

**3. No branches + no PR review = you don't need GitHub (the product), just git (the tool).**
If you strip away PR review UI and branch management, what's left is git as a content-addressable store with a linear commit history. CI/CD becomes a library call, not a pipeline configuration. Every commit carries intent trailers and a cryptographic certificate. The audit trail is the commit log.

---

## The Grove Suite — What's Already Built

Relay is one of four tools in the grove-suite. The others are foundational infrastructure that Relay depends on.

### Grove — Code Knowledge Graph
A persistent, Tree-sitter-based code graph stored in SQLite. Parses 11 languages, stores 8 edge types (`defines`, `contains`, `imports`, `extends`, `implements`, `calls`, `uses-type`, `tests`). Delta indexing via git blob SHAs. BFS traversal for impact radius, dependency analysis, and test selection.

Key API endpoints Relay uses:
- `POST /icr {"intent": string}` → affected symbol list + count (used for granularity scoring)
- `POST /deps {"file": string}` → edges between symbols (used for decomposition)
- `POST /impact {"file": string, "line": int}` → blast radius (used for certification)
- `POST /tests {"query": string}` → test selection (used for certification)

Grove is the intelligence layer. Without it, Relay is just a queue.

### Prism — Context Delivery
Sits on top of Grove. Given an agent task description, Prism delivers a token-optimized context pack: pre-ranked symbols, dependencies, tests, documentation, within a token budget. Uses 5-signal ranking (graph distance, semantic similarity, recency, test relevance, edit frequency). This is what the Claude agent inside an execution pod receives as its codebase context.

### Fuse — Semantic Merge Driver
A git merge driver that operates at symbol granularity using Grove's graph. When multiple agents produce ChangeSets from the same base commit, Fuse resolves them semantically rather than line-by-line. Agents working on structurally independent symbols cannot conflict at the symbol level — but they can touch adjacent lines. Fuse handles this correctly. This is the merge layer for parallel agent output.

### Relay — The Orchestrator (being designed)
Relay is the layer that connects intent authoring to production. It uses Grove, Prism, and Fuse as execution infrastructure.

---

## Core Architecture Decisions

### Decision 1: ICR Isolation Replaces Branches

An **Isolated Change Region (ICR)** is the set of symbols an intent will touch, computed by Grove from the intent description:
1. Grove's `/icr` endpoint returns the affected symbol set
2. BFS over the dependency graph computes the blast radius
3. Symbols are classified: `exclusive` (agent will modify), `shared_read` (agent may read), `boundary` (interface to the rest of the system)
4. Redis `SET NX EX` locks the exclusive set before agent execution begins
5. Two intents with non-overlapping exclusive sets run in parallel; overlapping ones queue

This replaces branches. There is no `feature/add-rate-limiting` branch. There is an ICR lock on `{AuthService, RateLimiter, LoginHandler}`.

### Decision 2: Linear Main, No Branches

The source repo has one branch: `main`. Every commit has trailers:
```
feat: add rate limiting to auth endpoints

Intent-ID: INT-2026-042
Certificate: cert-2026-05-28-a3f9
Agent: claude-sonnet-4-6:pod-abc123
ICR-Hash: sha256:d4e5f6...
Test-Plan: 12-unit/3-integration
```

Agents work on cloned snapshots, produce diffs (ChangeSets), and Relay's admission controller rebases and commits them. No merge queue. No branch management.

### Decision 3: Intent as the Review Artifact

Humans review and sign *intents*, not diffs. An intent specifies:
- What should change (title + description)
- Why (rationale)
- Acceptance criteria (testable assertions)
- Constraints (what must not change)
- Domain and capability (for routing and ICR computation)

After sign-off, the agent runs autonomously. Human code review is a configurable policy gate (none / optional / mandatory), positioned *before* certification so rejection wastes no compute. For most teams, review starts mandatory, then gets dialed down per domain as confidence builds.

### Decision 4: Git-Native Intent Store

Intents are YAML files committed to a dedicated `intent-store` git repo. Every state change is a git commit. The git log is the full audit trail. This was chosen over a database because: intents are append-mostly, audit history is first-class, and git already handles concurrent writes via its object model.

```
intent-store/
  active/
    INT-2026-042.yaml
    INT-2026-043.yaml
  completed/
    INT-2026-001.yaml  ... INT-2026-041.yaml
  failed/
```

The source repo (application code) is a separate git repo with linear history and intent trailers on every commit.

### Decision 5: Three Git Repos, Redis for Coordination

| Repo | Purpose |
|------|---------|
| `source-repo` | Application code. Linear main, no branches. |
| `intent-store` | Intent YAML files. Every state change = a commit. |
| `platform-config` | Policies, domain definitions, agent configs. |

Redis is transient coordination only — ICR locks, execution queue, agent heartbeats. `appendonly no`. Redis loss is recoverable (locks auto-expire, queue rebuilds from intent-store state).

### Decision 6: Jira/GitHub as Input Sources (Not Replaced)

Relay receives work from existing tools via webhooks:
- Jira issue transitions to "Ready for Relay" → creates an intent
- GitHub issue labeled "relay" → creates an intent
- CLI: `relay intent create` → creates an intent directly

Teams keep their existing planning tools. Relay is the execution layer that makes tickets executable. "Replace Jira" is a multi-year fight; "make Jira tickets execute autonomously" ships value now.

---

## What's Built (Phase 1)

The intake layer is built and running:
- Intent ingestion from Jira webhooks (HMAC-validated), GitHub webhooks, CLI
- Two-stage granularity scoring: heuristic pre-check + Grove ICR symbol count
- B→C→A project routing (explicit field → human triage → component filter)
- Intent state machine (draft → unrouted → validating → needs_info / queued / rejected)
- Per-project GS threshold, bidirectional feedback (bot comments back to Jira/GitHub)
- HTTP API, CLI, HTML dashboard
- Postgres for mutable operational state; intent snapshots go to git on approval (Phase 2)

Data model:
```
Repo (one per git repo)
  └── Project (one per logical service / monorepo path)
        ├── ProjectIntegration (M:M with external boards — Jira, GitHub Issues)
        └── Intent (project_id FK, status, gs_score, icr_symbols, source, source_ref)
              └── IntentEvent (audit log for every transition)
```

---

## What We're Proposing to Build Next (Phase 2)

**One working end-to-end execution loop:**

```
Queued intent
     │
     ▼
Grove decomposition
POST /icr → affected symbol list
POST /deps → edges between symbols
union-find → connected components (independent = no calls/uses-type/imports edges between them)
     │
     ├── Component A (independent)      Component B (independent)
     │   Redis ICR lock acquired         Redis ICR lock acquired
     │   K8s pod spawned                 K8s pod spawned
     │   git clone source-repo@HEAD      git clone source-repo@HEAD
     │   Prism delivers context          Prism delivers context
     │   Claude Sonnet runs              Claude Sonnet runs
     │   git diff → ChangeSet            git diff → ChangeSet
     │   Pod exits                       Pod exits
     │
     └──────── Fuse semantic merge ──────┘
                    │
                    ▼
             Certification
             Grove /impact → blast radius
             Grove /tests → test selection
             lint + unit tests + integration tests
             SAST scan
                    │
                    ▼
             Admission
             rebase onto HEAD
             linear commit with trailers
             release ICR locks
             re-index Grove
                    │
                    ▼
             Canary deployment (1% → 25% → 100%)
             metric validation
             intent marked "realized"
```

Agent model: Claude only (Sonnet default, configurable per-project). Multi-provider is complexity for no current benefit; revisit when there's evidence one model is better for specific task types.

---

## Open Questions We Want Challenged

These are the questions we've identified but not fully resolved. We want your honest assessment of each — whether our tentative answer is right, what we're missing, and whether there are better alternatives.

### Q1: Is ICR Isolation Strong Enough to Replace Branches?

Our claim: if two intents have non-overlapping exclusive symbol sets, they cannot produce merge conflicts.

The challenge: symbol-level isolation doesn't prevent all conflicts:
- Two intents both add a new function to the same file (different symbols, same file, adjacent lines)
- One intent changes a function signature; another adds a caller to the old signature (Grove should catch this via `calls` edges, but what if the edge confidence is 0.6?)
- A migration file is always a conflict candidate (it's a sequential log, not a symbol graph)

Is there a class of conflicts that ICR isolation + Fuse merge cannot handle? What's the failure mode when they occur?

### Q2: The Agent Gets Stuck — What's the Right Escalation Path?

During execution, the agent hits genuine ambiguity: the intent says "add rate limiting" but doesn't specify the algorithm, the limit value, or which storage backend. The agent has to make a choice.

Options:
- **Always autonomous**: Agent picks a reasonable default, notes the choice in the ChangeSet metadata. Human reviews the intent outcome, not the decision.
- **Fail with questions**: Agent exits with `status: needs_clarification` and a list of questions. Intent moves back to `needs_info`. Author updates the intent, re-executes.
- **Produce a draft**: Agent produces a partial ChangeSet with explicit TODOs. Review gate catches it.

The "always autonomous" path is fastest but risks agents making wrong choices that pass certification (tests pass, but design is wrong). The "fail with questions" path is slower but produces better outcomes. Is there a middle ground? What do other autonomous systems do here?

### Q3: Where Does This Conversation Go?

Design discussions — "should we use event sourcing for this service?", "how should the rate limiter integrate with the auth middleware?" — are not intents. They produce no code. But they inform intents.

Currently: developers have this conversation in Claude Code or Cursor, then write an intent based on the outcome. The conversation is ephemeral — no record in Relay.

The question: should Relay care about design conversations at all? Should there be an ADR (Architecture Decision Record) system where design decisions are recorded and referenced by intents? Or is "design happens outside Relay, intent is the boundary" the right model?

The risk: agents executing intents will be doing so without the design rationale that informed the intent. A well-written intent captures this in `constraints` and `rationale` fields, but that depends on the author doing it.

### Q4: The Admission Ordering Problem

Intent A is certified at 10:01. Intent B is certified at 10:02. Both are rebased onto HEAD and committed in order. Simple.

But: Intent C was certified at 09:58 and is part of an Intent Group (must admit atomically with Intents D and E). D certifies at 10:05. E fails certification twice, retries, certifies at 10:11.

Meanwhile, A, B, and 8 other intents have committed to main. When C+D+E are finally ready to admit as a batch, they rebase onto a HEAD that's 10 commits ahead of where they were certified.

Two problems:
1. The rebase may produce conflicts (symbols that were in their ICR have been modified by intervening commits)
2. The certification is now stale (tests passed against a version of the code that no longer exists)

Our current thinking: if rebase produces conflicts, the entire group fails and re-executes from scratch. If rebase is clean, re-run Stage 1 certification (fast: build + unit tests, no ephemeral env) before admitting.

Is this the right trade-off? Are there better approaches to managing certification staleness?

### Q5: What's the Right Scope for Phase 2?

We're proposing to build only the single-intent execution loop (above) and defer: intent groups, priority queue, canary deployment, multi-developer concurrency, DAST, semantic conflict detection, rollback.

The risk: a single-intent loop without ICR locking means you can only run one intent at a time. That's fine for a prototype but not for a team of 5.

Should Phase 2 include basic ICR locking (so at least non-conflicting intents can run in parallel), or is true serialization acceptable for the first working version? What's the minimum viable feature set for a team to actually adopt this?

### Q6: Is Git the Right Intent Store?

We chose git for the intent store because: append-mostly workload, audit trail is first-class, concurrent writes handled by git object model, no operational database to manage.

The challenge: git is not designed for querying. "Show me all intents in state X with GS score > 0.7 that arrived in the last 24 hours" requires either reading YAML files across the directory tree or maintaining a separate index. The Phase 1 implementation used Postgres for exactly this reason.

Proposed hybrid: Postgres for operational intent state (queryable, mutable); approved intent snapshots + certificates written to git (immutable, auditable). Git is the audit trail; Postgres is the working state.

Is this hybrid the right call? Or does it introduce dual-state consistency problems that are worse than the querying limitations of pure git?

---

## Specific Questions for the Reviewer

1. **Are there fundamental flaws in the ICR isolation model?** We believe graph-based symbol locking eliminates merge conflicts for typical code changes. Where does this assumption break down?

2. **Is the three-repo model (source + intent-store + platform-config) operationally sound?** What failure modes does it introduce that a single-repo or database-backed approach would avoid?

3. **Is "Claude only, no multi-provider" the right call for Phase 2?** What would change that decision?

4. **What's missing from the intent YAML schema?** The schema has: title, description, domain, capability, acceptance_criteria, constraints, priority, group. What fields would you add?

5. **What's the biggest risk in the Phase 2 execution loop that we haven't identified?** Not the questions above — something we haven't thought to ask.

6. **Should Relay try to handle infrastructure changes (Terraform, K8s manifests) in Phase 2, or code-only?** What's the cost of deferring infrastructure to Phase 3?

7. **Is there a simpler design that achieves the same goals?** The ICR + decomposition + parallel agents + Fuse merge pipeline is complex. Is there a version of "autonomous code delivery" that's significantly simpler and still solves the core problem?

---

## What We Are NOT Asking

- We are not asking whether autonomous code delivery is a good idea. We've committed to that thesis.
- We are not asking for a comparison to GitHub Actions, Jenkins, or ArgoCD. We know the tradeoffs.
- We are not asking whether we should use a different language (Go is fixed).

We are asking: given this design, where will it break, what are we missing, and is the Phase 2 scope right?

---

## Appendix: Intent YAML Schema (Current)

```yaml
apiVersion: intent/v1
kind: Intent
metadata:
  id: "INT-{year}-{sequence}"
  parent: null          # parent intent ID if this is a decomposed sub-intent
  created: "ISO8601"
  created_by: "email"

spec:
  title: "Short imperative description"
  description: "Detailed description of what should change and why"
  domain: "architectural-domain"
  capability: "specific-capability"
  acceptance_criteria:
    - "Testable assertion 1"
    - "Testable assertion 2"
  constraints:
    - "What must NOT change"
    - "Boundaries and limits"
  estimated_scope:
    files: <number>
    domains: ["list"]

group: null             # IntentGroup ID if part of an atomic group
priority:
  level: "normal"       # critical|high|normal|low
  reason: ""

status:
  state: "draft"        # draft|proposed|signed|executing|review_pending|certifying|certified|merged|deployed|realized|failed
  granularity_score: null

approvals: []           # GPG-signed approvals

execution:
  agent: "claude-sonnet-4-6"
  icr_hash: null
  cost_so_far: null
  retries: 0

certification:
  id: null
  build: null
  unit_tests: null
  integration_tests: null
  security: null
```

---

## Appendix: Granularity Score Formula

```
GS = 40% × heuristic_score + 60% × icr_score

Heuristic score (0.0–1.0):
  - Description < 8 words or > 150 words → penalty
  - Vague phrases ("refactor all", "update everything") → penalty
  - Specific identifiers (function/file/endpoint names) → bonus
  - 3+ distinct system domains → penalty

ICR score (mapped from Grove /icr symbol count):
  1–50 symbols   → 0.70–0.95 (well-scoped)
  51–150 symbols → 0.40–0.70 (borderline)
  151+ symbols   → 0.00–0.40 (too broad)

Threshold: GS ≥ 0.70 (per-project configurable)

Note (Phase 2): High symbol count does not mean automatic rejection if those symbols
decompose into independent connected components. Phase 2 GS will account for decomposability.
```
