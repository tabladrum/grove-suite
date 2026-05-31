# Relay — Product Proposal

**Author's note:** This document is written in the voice of a seasoned startup product owner who has read everything you've built, the design reviews from Codex and Gemini, the original vision document, and current market research. It is opinionated. It says what to build, what NOT to build, and where this likely wins and likely fails. It is meant to be argued with, not nodded at.

---

## 1. Executive Summary

**The bet:** Every coding agent in the world produces a pull request. Nobody is building the safety, certification, and governance layer that has to sit underneath those PRs as AI-generated code volume overruns human review capacity. Relay should be that layer.

**Positioning (one line):** *Relay is the certified delivery platform for autonomous coding agents — Grove understands your code, Fuse merges agent output without conflicts, Relay admits it to production with a cryptographic certificate.*

**What we should build first:** Not the full intent-driven CI/CD replacement. Not the branchless main. A **certified-merge wedge product** with a single binary that scales from a developer's laptop to a Fortune-50 multi-region deployment without code changes, integrates with any coding agent via MCP as a pre-flight check the agent runs *in-loop* (the agent self-corrects before any human sees the diff), ships with semgrep / gitleaks / govulncheck / language linters pre-bundled (zero CI yak-shaving), reads its **configuration from `.relay/` inside the source repo** (so the same rules apply on every developer's laptop, the team server, and the enterprise control plane — config travels with the code), accepts ChangeSets from any agent (Claude Code, Devin, Cursor Background Agents, GitHub Copilot Workspace, internal scripts), runs Grove-driven certification, resolves conflicts via Fuse, and admits to a linear branch with a signed certificate and full audit trail. Sell into the EU AI Act / SOC 2 / governance buyer. The branchless trunk and intent-as-review-artifact pieces come later — after the platform has earned trust.

**Honest probability:** The technology is real and differentiated. The market timing is right. A meaningful business outcome ($100M+ ARR, IPO-track) is possible but not the most likely scenario. A $200M–$500M strategic acquisition in 3–4 years is the realistic strong outcome. The weakest scenario — agent vendors build governance themselves before we get traction — is also plausible. Build for the strong outcome; design the open-source positioning so the weak outcome still leaves a respected platform behind.

---

## 2. The Problem

Three converging facts define the market we are entering.

### 2.1 AI-generated code is overrunning review capacity

- **41% of code pushed to production worldwide is AI-generated.** Gartner forecasts 60% by end of 2026.
- Developers using AI **merge 98% more pull requests** and produce **154% larger PRs**, but PR review time has **increased 91%**.
- By 2026, AI-generated code volume **outstrips human review capacity by 40%** — the "AI code generation gap."
- LinearB's 2026 analysis: developers **feel 20% faster but are actually 19% slower**, a 39-point perception gap.

The PR review UI was designed for human-to-human review of small, context-rich diffs. It breaks at AI volume. Reviewers rubber-stamp, skim, or give up. The result is **verification debt**: unreviewed agent code piling up while teams nod it through because the queue never empties.

### 2.2 The agent works but doesn't reach production

- **88% of agent pilots never reach production.** The blocker is rarely the agent itself — it is the deployment infrastructure: isolation, governance, compliance controls, data residency.
- **69% of teams report deployment problems** "always, nearly always, or frequently" when AI-generated code is involved.
- **45% of AI-generated code contains vulnerabilities** (hardcoded secrets, improper input validation, etc.).

The market has spent the last 18 months building agents. The infrastructure that makes agent output safely deployable has not been built.

### 2.3 Regulation is forcing the issue

- The **EU AI Act's high-risk activation date is August 2, 2026.** Production AI systems must have logging, oversight, and termination controls in place.
- **63% of organizations cannot enforce purpose limitations** on AI agents; **60% cannot quickly terminate a misbehaving one.**
- Only **21% of enterprises have mature governance models** for autonomous agents; **40% of agentic AI projects are projected to fail by 2027** due to inadequate governance.

This is not a niche concern. It is becoming a regulated requirement.

---

## 3. Market Landscape

The agentic coding stack has settled into three tiers. Relay should not compete in tiers 1 or 2.

### Tier 1: Agent Substrate (commoditizing fast)

The model providers and code completion layer.

| Player | Position | Why we don't compete |
|--------|----------|----------------------|
| Anthropic Claude / Sonnet / Opus | Frontier model | We use it |
| OpenAI Codex / GPT-5 | Frontier model | We can use it |
| Google Gemini | Frontier model | We can use it |
| GitHub Copilot (Visual Studio) | Incumbent, IDE-embedded | Distribution moat, not a fight to pick |

### Tier 2: Agent Products (high-funded, well-defended)

The "I am the AI engineer" products. They produce code; they stop at a PR.

| Player | Funding / Valuation | What they do | Where they stop |
|--------|---------------------|--------------|------------------|
| Cognition / Devin | **$25B valuation, $492M ARR** (May 2026), Devin 2.0 at $20/mo | "Autonomous AI software engineer." 89% of Cognition's own code is Devin-written. | Produces PRs into GitHub. Customer's existing CI/CD takes over. |
| Cursor | ~$9B+ valuation; Cloud / Background Agents | Background agents in isolated VMs, full dev environment, push branches and open PRs while user is offline | Produces PRs. Stops at the PR. |
| GitHub Copilot Workspace | Bundled with Copilot ($10–$39/mo) | Plans → writes code → runs tests → opens PR | Produces PRs. By design integrates with GitHub's own review tools. |
| Blitzy | **$1.4B valuation, $200M raised** | "Autonomous batch builder" — 50K–500K LoC per run, multiple QA agents | Hands code off; customer's CI/CD merges and deploys |
| Sourcegraph Amp / Cody | Established | Agentic coding with code graph context | Agent product, not a delivery platform |
| Augment Code | Established | Multi-agent coding workspace patterns | Agent orchestration, stops at quality gate |
| Lovable / Emergent / V0 | $50M–$550M raised | "Vibe coding" for non-developers | Generates apps, not enterprise infrastructure |

**The pattern:** every one of these stops at a pull request. None of them solve what happens after the PR — and that's where the verification debt is piling up.

### Tier 3: Delivery Infrastructure (where Relay should play, and where nobody serious is competing)

The layer that takes agent output and gets it safely into production with audit, certification, and governance. This is the missing tier. The closest analogs:

- **Aviator / Trunk.io / Mergify** — merge queue products. Solve PR-side concurrency. Don't certify the code itself. Don't understand agent output.
- **CircleCI / Harness / CodeAnt** — adding "autonomous validation" features. Still pipeline-based. Don't have code knowledge graphs.
- **CodeGraph (open source)** — local code knowledge graph for agents. Read-side only. Doesn't deliver code.

Nobody in this tier has all of:
1. A first-class code knowledge graph (Grove)
2. Semantic, symbol-level merge for parallel agent output (Fuse)
3. Token-optimized agent context delivery (Prism)
4. Intent-driven admission with cryptographic certificates (Relay)

That's the empty quadrant we should fill.

---

## 4. The Insight Nobody Else Is Acting On

> **Every player is competing to BE the agent. Nobody is building the platform underneath the agent.**

This is the most important sentence in the document.

- Cognition wants Devin to be your engineer.
- Cursor wants its background agents to be your engineer.
- GitHub wants Copilot to be your engineer.
- Blitzy wants its swarm to be your engineer.

All of them deliver code into the same broken downstream: a PR queue that no one reviews, a CI/CD pipeline that wasn't designed for agent volume, a merge model that produces conflicts when agents work in parallel, an audit trail that doesn't know which agent or which model produced which commit, and a governance posture that can't satisfy the EU AI Act, SOC 2, or any serious enterprise compliance review.

**The product that wins this category is not another agent. It is the platform that makes any agent's output safely deployable, certifiably correct, and auditable to regulators.**

That platform is what Relay should be.

---

## 4.5 How Relay Compares to Other Agentic Approaches

The agentic ecosystem today is overwhelmingly focused on *capability* — making agents do more, in more places, more autonomously, more coordinated. Skills, orchestration frameworks, background agents, MCP point tools — they all answer one question: "how do we make the agent more capable?"

Relay answers a different question: **"is what the agent produced *admissible* — provably correct, policy-compliant, audit-trail attached?"**

That's a governance question. Capability and governance compose. Relay isn't competing with the categories below; it's the layer all of them eventually need.

### 4.5.1 Where each approach sits in the agent lifecycle

```
INSTRUCTION → PLANNING → EXECUTION → OUTPUT → [ADMISSION] → AUDIT
              ────────────────────────────                 ─────
              Capability layer                              Storage
              
              Skills, Custom Instructions, Personas
              Orchestration: LangGraph, AutoGen, CrewAI, Agent SDK
              Background agents: Cursor BG, Devin, Copilot Workspace, Spark
              MCP point tools: semgrep-mcp, eslint-mcp, gitleaks-mcp
                                                  ↑
                                                  Relay
```

Relay occupies the *output-to-admission* boundary, with structured feedback flowing back into execution. **No other tool occupies this slot today.**

### 4.5.2 Category-by-category comparison

| Approach | What it solves | What it doesn't solve | Composes with Relay? |
|----------|---------------|----------------------|---------------------|
| **Skills / Custom Instructions** | Domain-specific capability (PDF, Excel, framework patterns) | Whether output is admissible | Yes — capable agent producing admissible code |
| **Orchestration (LangGraph, AutoGen, CrewAI)** | Multi-agent flow + state management | Acceptance criteria for output | Yes — coordinated agents whose outputs land cleanly |
| **Background agents (Cursor BG, Devin, Copilot Workspace, GitHub Spark)** | End-to-end agent runtime that produces a PR | What happens after the PR — they all stop there | Yes — certified commits to main instead of PRs to a queue |
| **MCP point tools (semgrep-mcp, eslint-mcp)** | One tool's capability exposed to the agent | Aggregation, policy, certification, audit | Yes — Relay IS an MCP server, runs the point tools internally, owns the verdict |
| **CI feedback loop (status quo)** | Final safety net post-push | Pre-push self-correction; agent-friendly structured output | Yes — CI remains as final safety net; Relay shifts the loop left |
| **git worktree** | Filesystem isolation for parallel agent execution | Admission coordination | Yes — worktree isolates execution, Relay coordinates admission |

### 4.5.3 The deeper point — capability vs. governance

**Capability approaches make the agent better at writing code. Relay makes the code admissible.**

- Skills + Relay = capable agent producing admissible code
- Orchestration + Relay = coordinated agents whose outputs land cleanly on main
- Background agent + Relay = vertical agent runtime emitting certified commits to main, not PRs to a queue
- MCP point tools + Relay = Relay aggregates them, applies team policy, signs the verdict
- CI + Relay = Relay catches issues in-loop; CI remains the final safety net
- git worktree + Relay = parallel execution + clean admission, no rebase storms

Other categories produce, coordinate, or execute. None of them admit. That's Relay's lane.

### 4.5.4 Why this slot is strategically defensible

Three properties make the governance slot durable:

1. **Agent-agnostic by design.** As the agent market consolidates (Cursor vs. Devin vs. Copilot vs. Claude Code), Relay wins regardless of which agent wins. Capability-layer competitors have to bet on a specific agent or build their own.

2. **Buyer is different.** Capability tools sell to developers ($20/month). Governance sells to platform engineering and security ($300–700/dev/month at enterprise tier). Different buyer means different sales motion, different competitive set, different defensibility.

3. **Auditability compounds.** Every admitted commit adds to the audit corpus. After 12 months a team has a structured trail of every AI-generated commit, agent identity, model version, policy version, and test plan — replaceable only by re-emitting equivalent certificates from scratch. The data moat compounds with usage.

The agent vendors could, in principle, build governance themselves. The honest weak-case scenario in §12.3 acknowledges this. But the agent vendors are still competing to be the agent, not the layer underneath. The first to credibly own the governance slot earns the category.

---

## 5. Differentiated Position

The technology stack you've already built is the moat. Most competitors would need 18+ months to replicate it.

### 5.1 Grove — Code Knowledge Graph
Persistent, content-addressed graph of every symbol, file, and edge in the codebase. Tree-sitter parsing, SQLite storage, BFS traversal. 11 languages, 8 edge types. Open source under MIT.

**Why it matters for the platform:** Test selection, blast radius computation, intent granularity scoring, ICR computation — none of these are solvable without a code graph. The 2026 industry trend is that "a pre-indexed code knowledge graph is becoming table stakes for serious coding agents."

### 5.2 Fuse — Semantic Merge Driver
Symbol-level three-way merge using Grove. Recognizes that agents working on structurally independent symbols cannot conflict at the symbol level, but may touch adjacent lines. Resolves correctly where line-level merge would produce false conflicts. Open source under MIT.

**Why it matters for the platform:** This is the only honest answer to the multi-agent merge conflict problem documented in the AgenticFlict dataset (932K AI agent PRs catalogued). Sequential, isolated worktrees are the industry workaround. Fuse is the actual solution.

### 5.3 Prism — Context Delivery
5-signal ranking, budget allocation, progressive disclosure, session deduplication. Token-optimized context packs for agents. Open source under MIT.

**Why it matters for the platform:** When the agent runs inside the Relay execution environment, Prism is what gives it the right context, in the right shape, within the right token budget. The competitive comparison shows CodeGraph cuts token spend 59% across Claude Code, Codex, Cursor, OpenCode, and Hermes — Prism is the same value proposition, owned by us, integrated into the platform.

### 5.4 Relay — Intent-Driven Delivery Platform
The orchestrator. Intent intake, project routing, granularity scoring, ICR computation, certification pipeline, admission controller, audit trail. BSL licensed (commercial offering).

**Why it matters:** This is the platform layer. Grove + Fuse + Prism are MIT and recruit the developer ecosystem. Relay is BSL and is the commercial offering. Enterprises pay for Relay because it answers the question: "How do I let agents merge code into our trunk safely, with full audit and certification, in a way that satisfies our auditors?"

### 5.5 What the moat looks like when stacked

A buyer in 2026 evaluating "how do we let our developers use Devin/Cursor/Claude Code safely" has three choices:

| Approach | Cost / Risk |
|----------|-------------|
| Build it yourself | 18–24 months, high risk, no governance pedigree |
| Aviator + CodeAnt + GitHub Actions + manual policy | Stitches together 4 vendors, still no code graph, still no semantic merge, governance is bolted on |
| **Relay** | One platform. Code graph, semantic merge, certified admission, audit trail, EU AI Act-ready. |

That comparison is winnable.

---

## 6. The Wedge Product

We do not start by replacing GitHub. We do not start with branchless main. We start narrow — but the narrow product has five properties that determine whether the platform gets adopted.

### 6.1 The seven MVP properties

**Property 1 — One binary, three deployment modes. Laptop = full Relay server, single user.**

```
Laptop mode (solo dev)        Team mode (small team)         Company mode (enterprise)
FULL RELAY SERVER, 1 USER     SAME SERVER, SHARED            SAME SERVER, MULTI-TENANT
─────────────────────────     ─────────────────────────      ──────────────────────────
Single binary                 Same binary                    Same binary
Long-running `relay daemon`   Multi-process control plane    Multi-tenant control plane
Embedded SQLite               Postgres                       Multi-tenant Postgres
No Redis (single writer)      Redis (ICR locks)              Redis Cluster
Local Ed25519 signer          KMS-backed signer              KMS, per-tenant keys
Local intent-store git        Shared intent-store git        Sharded intent-stores
Default admission target:     Default admission target:      Default admission target:
  current branch                relay-main                     relay-main
Zero config to start          One VM + shared dashboard      HA, multi-region, audit aggregator
$0                            Cheap                          Enterprise tier
```

**The laptop binary IS the full Relay server, running for one user.** The pipeline (ingest → ICR → policy → certification → admission → signing → intent-store → audit) is identical to team mode. The substitutions are: SQLite for Postgres, no Redis (the long-running `relay daemon` is the single writer; ICR locks are irrelevant for one agent), local Ed25519 key for KMS, local intent-store git for shared intent-store, current branch as the default admission target. A solo developer produces signed, audit-trail-attached certs from day one — those certs are the seed of the team's audit corpus on team-mode upgrade, never reset to zero.

Transition between modes is configuration only, plus a one-time `relay migrate sqlite-to-postgres` for the operational state. `.relay/` is unchanged across the upgrade. No rewrite. This is the Vercel / Supabase / Sentry adoption ramp: trivial to start, scales without leaving.

Cross-platform in Phase 2A: macOS (Intel + ARM) and Linux (x86_64 + ARM64). Windows = WSL. macOS binary is Developer-ID-signed and notarized.

**Property 2 — Agent-in-loop integration via MCP, with a cost contract.**

Relay exposes itself as an MCP server. Claude Code, Cursor, Continue, Windsurf, and every other MCP-aware client invokes Relay as native tools (9 in Phase 2A):

```
# Core
relay_check          → fast in-loop pre-flight (changed-files SAST + Grove-affected
                       unit tests; sub-10s target); structured findings, incl.
                       `infrastructure_error` class for non-actionable failures
relay_certify        → full certification + signed certificate (commit-ready)
relay_submit         → submit ChangeSet to admission queue (team/company mode)
relay_policy         → fetch active policy + Effective-Config-Hash
relay_explain        → human-readable explanation of a specific finding

# Auto-Intent Capture (Property 7)
relay_intent_open    → draft an Intent from the user's prompt (call BEFORE coding)
relay_intent_update  → refine title / description / acceptance criteria mid-session
relay_intent_close   → promote the draft to .relay/intents/ for commit
relay_intent_list    → list open and committed intents for this repo
```

The cost contract is what makes the in-loop pattern viable: `relay_check` is fast (sub-10-second), `relay_certify` runs the full pipeline. The agent iterates with `relay_check` and only invokes `relay_certify` at commit time.

The agent loop becomes:

```
1. User prompts Claude Code: "Add rate limiting to /api/auth/*"
2. Claude Code calls relay_intent_open (captures the prompt as a draft intent)
3. Claude Code writes code
4. Claude Code calls relay_check
5. Findings come back as structured tool result (file, line, rule, severity, fix-hint)
6. Claude Code fixes the findings
7. Loop 3–6 until relay_check returns clean
8. Claude Code calls relay_intent_close (promotes the intent to .relay/intents/)
9. relay_certify writes the commit with `Intent-ID:` trailer
10. Push to GitHub — PR shows the intent YAML in the diff
```

MCP client auto-registration is handled by `relay mcp install-for {claude-code,cursor,continue,windsurf}` — no hand-editing IDE config per machine.

This is the actual differentiator. Today's agents produce code; *something else* tells them it's broken (CI, the reviewer). Relay closes the loop inside the agent's own iteration. The agent never produces a PR that fails policy because it can't get past `relay_check`. Relay becomes the agent's *companion*, not the agent's *gate*. And the user's prompt becomes a durable artifact, not a session ghost.

**Property 3 — Batteries-included tooling. Lean default, opt-in SonarLint.**

`brew install relay` (or `curl | sh`) ships with the lean default (~50MB compressed; no JRE):

- semgrep + ruleset bundles (security-audit, owasp-top-ten, language-specific packs)
- gitleaks
- govulncheck, npm audit, pip-audit
- golangci-lint, eslint, ruff, checkstyle, pmd
- Default policy templates (`relay init --stack=go-microservice` / `node-api` / `python-service` / `java-spring`)
- SonarQube quality profile **importer surface** (`relay import sonarqube-profile`) — writes the profile XML into `.relay/rulesets/` (committed, travels with the repo)

**SonarQube parity (the full thing) is opt-in.** `relay tools install --with-sonar` fetches Eclipse Temurin JRE 21 + SonarLint Core + analyzer JARs for the project's languages + a thin `relay-sonar.jar` wrapper around `StandaloneSonarLintEngine`. Cert trailer records `Sonar-Engine: sonarlint-core@X.Y` or `none`. The engine itself ships in Phase 2B; the importer surface that lands quality-profile XML into `.relay/rulesets/` ships in Phase 2A. Modeled on how the VS Code "SonarQube for IDE" extension already runs Sonar analyzers locally — no server needed. Limitations (no taint analysis / COBOL / Apex / PL/SQL / T-SQL — those require commercial SonarQube Server) are documented up front. Full analysis in [`docs/sonarqube-no-server-investigation.md`](sonarqube-no-server-investigation.md).

Tool-set manifest pinned per Relay release; drift from the manifest is recorded in the certificate trailer.

No CI yak-shaving. No "install these 12 tools and write a workflow." It works the moment you run it. This is the single biggest adoption lever because it removes the most common reason engineers give up: tooling configuration.

**Property 4 — Three surfaces, one engine, single writer on laptop.**

The same Relay engine is callable via:

- **MCP (primary — for agents)** — structured tool results agents natively consume
- **CLI (`relay check`, `relay certify`, `relay submit`)** — for humans, scripts, and CI integration
- **Git hook (pre-push)** — backstop for unmanaged agents and humans, runs same gates

All three return the same findings, apply the same policies, and emit the same certificate format. The agent, the developer, and the git hook see the same truth.

On laptop, all three transports proxy over a Unix domain socket to the long-running `relay daemon` that owns SQLite, the local Grove client, the intent-store git repo, and the signer key. This guarantees a single writer to shared state even when multiple IDEs and terminals invoke Relay concurrently against the same repo.

**Property 5 — Audit trail in every mode.**

Even laptop mode emits signed commit trailers (Intent-ID, Intent-Hash, Agent, Model, Policy-Version, ICR-Hash, Test-Plan-Hash, Effective-Config-Hash, Sonar-Engine, Toolchain-Image-Drift). The audit trail is a property of the engine, not a feature of a paid tier. Enterprises get aggregation, federation, and compliance export on top — but a solo developer running laptop mode still produces auditable, signed commits.

Privacy by default: laptop mode does not phone home. OpenTelemetry traces default off. Agent identity, prompt hash, intent content, cert data never leave the machine unless the developer explicitly enables sync to a team server.

This is what makes the value transferable across modes: a team's audit trail accumulates from day one, regardless of whether they ever upgrade to team or company mode.

**Property 6 — Configuration lives in the repo (`.relay/`).**

The repo's Relay configuration is committed to the repo, alongside the code it governs. Like `.github/`, `.vscode/`, `.editorconfig`, `package.json`. The same `.relay/relay.yaml` applies whether Relay runs on a laptop, a team server, or a multi-region enterprise control plane.

```
my-repo/
├── .relay/
│   ├── relay.yaml              # version pin, gate config, runners, cert stages, admission target
│   ├── .gitignore              # written by `relay init`; covers .relay/.cache/
│   ├── policies/               # per-gate detail (defaults vary by mode — coverage: warn on
│   │                           # laptop, enforce on team)
│   ├── rulesets/               # custom rules + imported SonarQube profile XML (verbatim)
│   ├── intents/                # COMMITTED intents — Auto-Intent Capture lands them here
│   ├── templates/              # intent templates
│   └── .cache/                 # GITIGNORED — daemon state, intent drafts, indexer caches
├── src/
└── tests/
```

Config changes go through the same PR review as code changes. Onboarding is one `git clone`. Branches can experiment with different policies. Forks inherit the config for free.

**Why this is foundational, not optional:**
- It's what makes "same Relay everywhere" actually true. Without repo-local config, the laptop agent has no way to know what rules apply, and `relay_policy` (MCP tool) has no answer.
- Auto-discovery: `relay check` from any directory finds the nearest `.relay/` upward and knows the rules. Same pattern as git, npm, eslint.
- OSS projects can publish their Relay config publicly; downstream forks get it for free.

**Config layering (resolved in order):**
1. Relay built-in defaults
2. Org baseline (in `platform-config`, enterprise mode only — can lock individual settings)
3. Repo config (`.relay/` in the source repo) — primary surface
4. User/host config (`~/.relay/config.yaml`) — credentials and transport only, never policy

The org baseline can mark fields locked (e.g., "secret scanning cannot be disabled") so the CISO has defense in depth while teams customize within boundaries.

The merged effective config is hashed and recorded in every certificate as `Effective-Config-Hash`, so audit replay is byte-reproducible.

**Property 7 — Auto-Intent Capture: the prompt IS the intent.**

The most consequential capability falling out of the laptop-MVP audit. Today, every coding agent (Claude Code, Cursor, Devin, Copilot Workspace, Blitzy) treats the user's natural-language prompt as ephemeral — it dies with the agent session, never reaches the PR, never reaches the audit trail. Reviewers and auditors see only the resulting diff and have to reverse-engineer the intent.

Relay captures it.

```
1. User prompts Claude Code:
   "Add rate limiting to /api/auth/* endpoints, 100 req/min per IP"

2. Claude Code's system prompt (shipped by Relay) instructs:
   "Before making code changes, call relay_intent_open with the user's
    request as title and description."

3. Claude Code calls relay_intent_open. Relay drafts:
   .relay/.cache/intents/INT-2026-05-30-rate-limiting.draft.yaml
   - id, title, description (verbatim prompt)
   - originated_from: {agent: claude-code:1.4.2, model: claude-sonnet-4-6:..., conversation_ts: ...}
   - allowed_paths (inferred from prompt + .relay/templates)
   - acceptance_criteria (drafted from prompt; agent can refine)

4. Claude Code writes code, runs the relay_check loop until clean.

5. Claude Code calls relay_intent_close. Relay promotes:
   .relay/.cache/intents/INT-*.draft.yaml
     → .relay/intents/INT-2026-05-30-rate-limiting.yaml   (committed)

6. relay_certify writes the commit with trailer:
   Intent-ID: INT-2026-05-30-rate-limiting
   Intent-Hash: sha256:abcd...

7. On `git push`, the GitHub PR shows the intent YAML in the diff.
   Reviewers see what the agent was ASKED to do, alongside what it DID.
   Server-side admission (team mode) cross-validates the diff against
   the intent's `allowed_paths` and `acceptance_criteria` before signing.
```

The user types nothing extra. They prompt the agent like they already do. The agent does the bookkeeping. The intent lands in the repo as YAML next to the code, becomes PR-reviewable, and stays auditable forever.

**Why this is genuinely new:**

| Tool | What gets persisted about the change |
|------|--------------------------------------|
| Git commit message | What the dev decides to summarize |
| GitHub PR description | Same, manual, often empty |
| Claude Code / Cursor session | Lost on session close |
| Conventional Commits | A type prefix, no semantic content |
| Devin session replay | Vendor-locked URL on Cognition's servers |
| **Relay Intent** | **The user's actual prompt + agent's model/version + acceptance criteria, committed alongside the code as YAML — portable, auditable, PR-reviewable forever** |

This is the capability that turns Relay from "another linter" into "the canonical record of why each AI-generated commit exists." It is the prompt-as-artifact tier in a market that has so far thrown the prompt away.

### 6.2 What the wedge does

**Product name (working):** Relay Certified Merge

A single binary that:

1. **Runs in any mode (§6.1 Property 1).** Laptop = full Relay server, single user. Team = same binary on a shared VM. Company = same binary, multi-tenant.
2. **Integrates with any agent via MCP, CLI, or git hook (§6.1 Properties 2, 4).** Claude Code on a developer's laptop, a Cursor Background Agent in a cloud VM, Devin, GitHub Copilot Workspace, Blitzy's batch output, internal scripts — anything that produces a unified diff plus metadata. On laptop, all surfaces proxy to a long-running `relay daemon` (single writer). MCP client auto-registration is one command per IDE.
3. **Ships with the static-analysis stack pre-bundled (§6.1 Property 3).** Lean default: semgrep, gitleaks, govulncheck/npm audit, golangci-lint/eslint/ruff. Zero CI setup. SonarQube parity is opt-in via `relay tools install --with-sonar` (Phase 2B); the importer surface for quality profiles ships in 2A.
4. **Runs Grove-driven certification with a cost contract.** `relay_check` (fast, in-loop): changed-files SAST + Grove-affected unit tests; sub-10-second target. `relay_certify` (commit-ready): full build, full unit/integration tests (selective opt-in for monorepos), coverage-of-changed-symbols gate (warn by default on laptop, enforce on team), SAST, dependency policy, restricted-path enforcement.
5. **Captures the prompt as the intent (§6.1 Property 7).** Auto-Intent Capture turns the user's natural-language request into a `.relay/intents/INT-*.yaml` committed alongside the code. The commit trailer carries `Intent-ID:` linking back to the file. Reviewers and auditors see the prompt next to the diff, forever.
6. **Resolves conflicts via Fuse** if multiple ChangeSets land against the same base.
7. **Admits to a configurable target branch** — current branch on laptop, `relay-main` on team — with a **cryptographic certificate** as a commit trailer (§6.1 Property 5). The certificate records: agent identity, model version, prompt template hash, ICR hash, base commit, test selection, certification stages run, policy version, intent hash, Sonar engine, toolchain drift.
8. **Records an audit trail** in a local `intent-store` git repo on laptop (`~/.relay/intent-store/`) or a shared one in team mode. Same format. Solo dev's history becomes the team's audit corpus on upgrade.

It runs alongside GitHub. Teams keep their existing PR workflow for non-agent code. Relay handles the agent-generated PRs. A solo developer using Relay on a laptop produces signed, intent-tagged, replayable commits from day one — no server, no service to run, no remote dependencies (Grove auto-starts locally).

### 6.2 Why this wedge specifically

- **No cultural shift required.** Teams don't have to abandon GitHub or PRs. They opt into Relay for agent-produced code only.
- **It works regardless of which agent the team chose.** This is critical. The agent market is still consolidating. Betting on one agent (or being one) is risky. Being agent-agnostic is durable.
- **It addresses a real, named pain.** "Verification debt." "Wrong successful change." "Governance gap." Buyers will recognize these from their own status reports.
- **The buyer is identifiable.** VP Eng, Director of Platform, CISO. These are people with budget who are being asked uncomfortable questions about EU AI Act readiness right now.
- **It does not require us to solve the hard problem yet.** Parallel decomposition, branchless main, intent-as-review-artifact — these are Phase 3. The wedge is sequential and conservative, exactly what both Codex and Gemini reviews recommended.

### 6.3 What we explicitly DO NOT build in the wedge

Both reviews converged here, and the laptop-MVP audit ([`docs/laptop-mvp-audit.md`](laptop-mvp-audit.md)) confirmed them:

- **No parallel decomposition.** Phase 3.
- **No intent groups.** Phase 3.
- **No canary deployment.** Phase 3 (or never — we may decide it's better left to Argo Rollouts, Flagger, etc.).
- **No multi-provider agent routing.** Claude only. Decide later.
- **No branchless main as a marketing claim.** The reviews are right — "ICR isolation replaces branches" is overpromising. We re-position it as "Relay makes branches unnecessary for approved classes of agent-delivered work."
- **No SonarLint Core engine integration in 2A.** The importer that lands SonarQube quality profile XML into `.relay/rulesets/` ships in Phase 2A. The Eclipse Temurin JRE + SonarLint Core + analyzer-JAR runtime that actually evaluates those profiles ships in Phase 2B via `relay tools install --with-sonar`. Rationale: [`docs/sonarqube-no-server-investigation.md`](sonarqube-no-server-investigation.md).
- **No Windows native support in 2A.** macOS (Intel + ARM) and Linux (x86_64 + ARM64) only; Windows via WSL.

---

## 7. End-to-End User Journey

The most important test of any product proposal is: can you describe what a real user does on Monday morning? Three scenarios.

### 7.1 Solo developer using Claude Code

**Day 1.** Developer runs `brew install relay-suite` (Relay binary + Grove auto-bundled). In their repo: `relay init --stack=go-microservice`. This scaffolds `.relay/` (committed) and `.relay/.cache/` (gitignored), generates the local Ed25519 admission key, starts `relay daemon` as a user-level service, registers Relay with their installed IDE via `relay mcp install-for claude-code`, and kicks off background Grove indexing. `git add .relay/ && git commit -m "Add Relay configuration"`.

**Day 2 — first agent change.**
- Developer asks Claude Code: "add rate limiting to the /api/auth/* endpoints, 100 req/min per IP."
- Claude Code (with Relay's shipped system-prompt fragment) calls `relay_intent_open` first. Relay drafts `.relay/.cache/intents/INT-2026-05-30-rate-limiting.draft.yaml` capturing the verbatim prompt, agent identity, and inferred `allowed_paths: ["internal/auth/**", "tests/auth/**"]`.
- Claude Code writes the code.
- Claude Code calls `relay_check` after each edit pass. Relay returns structured findings: a missing test for the rate-limit denial path, a gitleaks finding on a hardcoded fallback secret. Claude Code fixes both and re-runs. `relay_check` returns clean in ~6 seconds.
- Claude Code calls `relay_intent_close`. Relay promotes the draft to `.relay/intents/INT-2026-05-30-rate-limiting.yaml`.
- Developer reviews the proposed commit. They run `relay certify`. Relay extracts the ChangeSet, calls Grove `/impact` and `/tests`, runs the full selected test slice (23 tests instead of the full 800), full SAST, validates path restrictions, computes ICR confidence, signs the certificate with the local Ed25519 key.
- Relay writes a single commit on the **current branch** (not `relay-main` — that's team-mode default) with the full trailer set including `Intent-ID: INT-2026-05-30-rate-limiting`.
- Developer pushes to GitHub. The PR shows the intent YAML and the code change side-by-side. Anyone reviewing sees what the agent was asked to do.

**Value:** Tests ran in 8 seconds instead of 4 minutes. The prompt is preserved in the repo as a first-class artifact, not lost in a chat history. Audit trail exists locally from day one — signed commits ready to hand to a future team or auditor. If they ever have to prove "an AI wrote this and was asked to do exactly that" for compliance, they can. No server. No subscription. No network call.

### 7.2 10-person team with shared codebase

**Day 1.** Platform engineer registers the repo and three projects (`auth-service`, `payments`, `notifications`) in Relay. Configures per-project GS thresholds and `allowed_paths`. Sets policy: "all agent commits require Relay certification; human commits go through normal PR flow."

**Day-to-day.**
- Three developers each have Claude Code or Cursor Background Agents running in parallel.
- Each submits ChangeSets to Relay.
- Relay computes ICR for each, acquires Redis locks on the exclusive symbol sets. Two ChangeSets touching non-overlapping symbols proceed in parallel. The third, which touches symbols overlapping the first, queues.
- When the queued ChangeSet eventually runs, Grove and Fuse rebase it against current HEAD, re-run the certification slice for affected dependencies, and admit it. No merge conflict.

**Value:** Three agents, no conflicts, fully certified. The PR review pile-up that would have happened in plain GitHub doesn't exist.

### 7.3 100-engineer company with SOC 2 and EU AI Act exposure

**Buyer:** VP Platform Engineering, CISO co-signs the budget.

**Pain:** Three teams have piloted Cursor Background Agents, Devin, and Copilot Workspace. The pilots work — code gets written — but security and audit have blocked production deployment because:
- No way to prove which commits were AI-generated vs. human
- No certification that AI commits passed security scans
- No way to enforce "agents cannot touch the payment service"
- No way to terminate a misbehaving agent and prove it was terminated

**Solution:** Relay sits between any of those agents and the production repo. Every agent commit carries the Relay certificate. Policy gates (`forbidden_paths: ["services/payments/**"]`) are enforced by Grove before agent execution begins. The intent-store git repo is the auditable export the EU AI Act compliance team needs.

**Sale:** Relay Enterprise license. Self-hosted (data residency). On-call support. Compliance attestation.

This is the buyer Relay should design for. Solo developers are evangelists; ten-person teams are the proof points; the enterprise is where the money is.

---

## 7B. Signature Capabilities ("the things people remember")

The wedge alone is a credible commercial product. But to be **memorable** — the kind of thing developers paste into team chats and security officers cite in board decks — Relay needs a small set of distinctive capabilities layered on top of the certification engine. These are the features that make Relay feel **new**, not just "another CI shell."

Each one builds on data the engine already collects. None of them is a separate product. None of them requires a new UI surface beyond what already ships in MCP / CLI / dashboard.

### 7B.1 Pre-Flight Autopilot (Phase 2A) — *the "agent self-correction" moment*

The recommended agent system prompt instructs the agent to invoke `relay_check` after every code change and before reporting completion. When findings come back, the agent **fixes its own work** (it has the structured findings + the source) and re-checks. The human only ever sees diffs that already pass.

This collapses the traditional review/fix/repush loop from minutes to seconds and changes the felt experience of using an AI agent. It is the killer demo for the wedge: "Claude fixed the security finding before I even saw it."

**Why it works without new infrastructure:** `relay_check` already returns structured findings in Phase 2A. Agents already iterate. The "feature" is a documented prompt pattern + a tight, agent-friendly findings schema. Ship it as `docs/agent-prompt.md` plus first-class examples.

### 7B.2 AI Code Passport (Phase 2A) — *make the certificate a thing developers see*

Every admitted commit already carries a signed certificate (`Intent-ID`, `Agent`, `Model`, `Prompt-Hash`, `Context-Hash`, `Policy-Version`, `Effective-Config-Hash`, `ICR-Hash`, `Test-Plan`, `Toolchain-Image`, `Signed-By`). Today it's commit trailers + a JSON file in the intent-store — invisible unless you go looking.

The Passport is the **viewable, shareable surface** of that certificate:

- `relay cert show <ref>` prints a human-readable passport for any commit.
- A bot comment posts the passport on every PR/MR.
- The dashboard renders it as a card with badge ("certified by Relay") + machine-readable QR/JSON-LD payload for compliance tooling.

This is what makes the audit trail real to humans without inventing a new data model.

### 7B.3 Diff Risk Heatmap (Phase 2A) — *graph-aware review prioritization*

Because Relay has Grove's blast radius and the ICR for every change, it can produce a per-file/per-symbol **risk score** at admission time:

- `auth-touched` / `payment-boundary` / `migration` / `public-api-change` boundary flags
- Test coverage delta on changed symbols (already computed by the coverage gate)
- ICR confidence
- Dependency-graph reach (number of downstream callers)
- Historical defect density of the touched files

The heatmap is rendered in the dashboard and embedded in the PR bot comment ("3 symbols touch `payment-boundary`; ICR confidence 0.71 — recommend senior review"). It is **not** a standalone review UI. It augments existing PR review.

### 7B.4 Surgical Revert by Intent (Phase 2B) — *the eye-popping feature*

Because every admitted commit is bound to an `Intent-ID` + a recorded ICR (the exact symbols changed), Relay can compute a **symbol-scoped revert** for any intent without disturbing later, unrelated changes that happen to live in the same files. `relay revert --intent INT-2026-042` produces a ChangeSet that:

1. Re-applies the inverse diff scoped to the symbols in the original ICR.
2. Re-runs Fuse against current HEAD to keep adjacent changes intact.
3. Goes through the standard certification + admission flow, producing a fresh certificate that links back to the reverted intent.

For incident response, this is genuinely new. Today the only revert tool is `git revert <merge-sha>`, which either rolls back too much or refuses to apply.

### 7B.5 Evidence Replay (Phase 2A-late / Phase 2B) — *the auditor-killer feature*

`relay cert replay <cert-id>` reconstructs the **exact** evaluation context of a historical certificate:

- Checks out the `Repo-Config-SHA` (the `.relay/` state at admission time)
- Re-applies the `Effective-Config-Hash`-pinned merged config
- Pulls the pinned `Toolchain-Image` (semgrep / gitleaks / linter versions)
- Re-runs the certification stages against the recorded ChangeSet
- Compares the recomputed findings + verdicts against the certificate payload

An auditor can independently verify, months later, that the policy in force at admission time produced the verdict the certificate claims. This is **byte-reproducible audit** — something no CI system today provides for AI-generated code.

### 7B.6 `.relay/` Policy Marketplace (Phase 2A → Phase 2B) — *adoption flywheel*

`relay init --stack=<...>` already scaffolds defaults per stack. The marketplace extends this with curated `.relay/` profile bundles maintained as version-pinned OSS templates:

- Stack profiles: `go-microservice`, `node-api`, `python-fastapi`, `java-spring`, `rust-axum`, `kotlin-ktor`, etc.
- Compliance profiles: `soc2-baseline`, `eu-ai-act-article-12`, `pci-dss-app`, `hipaa-app`, `fedramp-moderate`.
- Domain profiles: `fintech-api`, `healthcare-ehr`, `defense-classified-handling`.

`relay init --profile=soc2-go-api` lays down a `.relay/` that immediately produces useful governance. Profiles are PRs against a community repo; ratings/forks are visible. This makes Relay sticky for platform teams who otherwise would build their own policy stack from scratch.

### 7B.7 Human Review Budget Optimizer (Phase 2B) — *turn the gate into a triage tool*

The most underused signal in code review is risk. Today every PR competes for the same reviewer attention. With the certificate + heatmap, Relay can post a **recommendation** per ChangeSet:

- *Skim only* — high cert confidence, low blast radius, full coverage, no boundary flags.
- *Standard review* — typical.
- *Senior review required* — boundary flag (auth/payment/migration), or ICR confidence < 0.85, or coverage delta negative.
- *Two-person review* — policy-marked critical path.

This is posted as a PR check, not a wall. The reviewer keeps the final say. But for a team drowning in agent-generated PRs, this is the difference between "rubber-stamp everything" and "spend your attention where it matters."

### 7B.8 Agent Scorecard (Phase 2B → Phase 3) — *enterprise intelligence layer*

Every certificate records the agent + model identity. Aggregating across the intent-store gives the platform team:

- First-pass certification rate per agent / per model / per stack.
- Average self-correction loop length (how many `relay_check` cycles before passing).
- Distribution of failure reasons (secrets / SAST / coverage / size / policy).
- Cost per admitted intent.
- Defect rate of admitted commits over time (linked back via incident → intent).

This becomes the basis for objective agent-vendor procurement decisions ("Devin admits 73% of intents on first try; Claude Code admits 81% but takes 1.4× as many `relay_check` cycles"). Enterprises today have **no** vendor-neutral way to compare AI coding tools on actual production-readiness; the Scorecard is that benchmark, owned by the customer's own corpus.

---

### 7B.9 What we deliberately did *not* add (and why)

Reviewers proposed several other capabilities. We reject these — they trade focus for novelty.

| Proposed | Decision | Why |
|----------|----------|-----|
| Standalone "Diff Comprehension UI" / interactive Reviewer Flow Plan | **Reject** | Contradicts the principle of not owning a proprietary review surface. The Risk Heatmap delivers the same value inside GitHub/GitLab review tooling, which is where developers already are. |
| Self-Healing Compliance Sandbox (auto-fix agent on cert failure) | **Defer to Phase 3+, and only as a customer-supplied hook** | Two failure modes: (1) the auto-fix agent itself becomes a new attack surface; (2) it teaches teams that Relay "always passes," eroding the gate's signal value. Pre-Flight Autopilot already addresses the legitimate case (agent fixes its own work *before* claiming completion). |
| Multi-provider model routing in the agent runtime | **Defer to Phase 3+** | Premature optimization. Claude-only in Phase 2; revisit when we have cost + quality data. |
| A Relay-native "AI code review chat" UI | **Reject** | Out of scope. Relay is admission + evidence; it is not a conversation product. |
| Real-time canary deployment orchestration | **Defer to Phase 3+, as integration not implementation** | Argo Rollouts / Flagger / LaunchDarkly already do this well. Relay's job is to emit the signal ("this commit is high-risk"), not to own the rollout matrix. |

The discipline is unchanged: **Relay is the certification + evidence layer, not a competing product to every adjacent category.**

---

## 8. Technical Architecture

This section incorporates the design review feedback from Codex and Gemini. Both converged on the same recommendations; this architecture reflects them.

### 8.1 What Phase 2A (the wedge) actually builds

```
┌──────────────────────────────────────────────────────────────┐
│ Agent (Claude Code, Cursor, Devin, etc.)                     │
│   produces a diff against base commit                        │
└─────────────────────────┬────────────────────────────────────┘
                          │  relay submit  (or HTTP POST /api/changesets)
                          ▼
┌──────────────────────────────────────────────────────────────┐
│ Relay Ingest                                                 │
│   - validate ChangeSet schema                                │
│   - extract metadata (agent identity, model, prompt hash)    │
│   - record in Postgres                                       │
└─────────────────────────┬────────────────────────────────────┘
                          ▼
┌──────────────────────────────────────────────────────────────┐
│ Grove Impact Analysis                                        │
│   POST /impact → affected symbols + blast radius             │
│   POST /tests  → selected test set                           │
│   POST /deps   → cross-file dependency map                   │
│   compute ICR (exclusive / shared_read / boundary)           │
│   compute ICR confidence score                               │
└─────────────────────────┬────────────────────────────────────┘
                          ▼
┌──────────────────────────────────────────────────────────────┐
│ Policy Gate                                                  │
│   - allowed_paths / forbidden_paths                          │
│   - dependency change detection (lockfile, go.mod, etc.)     │
│   - secret scanner                                           │
│   - migration file class detector (auto-serialize)           │
│   - risky API detector (unsafe.*, eval, raw SQL, etc.)       │
└─────────────────────────┬────────────────────────────────────┘
                          ▼
┌──────────────────────────────────────────────────────────────┐
│ ICR Lock Acquisition (Redis SET NX EX + fencing token)       │
│   - if exclusive symbol set unlocked → proceed               │
│   - if conflict → enqueue                                    │
│   - if ICR confidence too low → escalate to human review    │
└─────────────────────────┬────────────────────────────────────┘
                          ▼
┌──────────────────────────────────────────────────────────────┐
│ Certification Pipeline                                       │
│   Stage 1 (static): build, lint, unit tests (Grove-selected) │
│   Stage 2 (security): SAST, dependency scan                  │
│   Stage 3 (integration, optional): ephemeral env             │
│   produces signed certificate                                │
└─────────────────────────┬────────────────────────────────────┘
                          ▼
┌──────────────────────────────────────────────────────────────┐
│ Rebase + Re-Certify                                          │
│   - rebase ChangeSet onto current HEAD                       │
│   - if rebase conflicts → Fuse semantic merge                │
│   - if Fuse cannot resolve → reject, report to author        │
│   - re-run Stage 1 against rebased state (fast slice)        │
└─────────────────────────┬────────────────────────────────────┘
                          ▼
┌──────────────────────────────────────────────────────────────┐
│ Admission Controller                                         │
│   - commit to relay-main with trailers                       │
│   - release Redis locks                                      │
│   - update Grove index incrementally                         │
│   - write snapshot + certificate to intent-store git repo    │
│   - update Postgres state                                    │
└──────────────────────────────────────────────────────────────┘
```

### 8.2 Storage model (revised after design review)

Both reviewers flagged that pure git as the operational store is wrong, and pure Postgres loses audit value. The hybrid:

| What | Where | Rationale |
|------|-------|-----------|
| Mutable workflow state (queue, locks, in-flight intents) | Postgres | Queryable, concurrent writes, sub-100ms latency |
| Audit snapshots, certificates | intent-store git repo | Immutable, signed, export-ready for compliance |
| Code knowledge graph | Grove (SQLite) | Already there |
| Coordination primitives (locks, queues, heartbeats) | Redis (transient, no persistence) | Already designed |
| Container images | OCI registry | Standard |

The Postgres → git path uses the **outbox pattern**: every state transition writes a Postgres row and an outbox row in the same transaction; a reconciler writes the snapshot to git and marks the outbox row complete. This is the only way to prevent drift between operational state and audit state.

### 8.3 ICR confidence as a first-class signal

The biggest single insight from the Codex review: **ICR confidence is not a footnote — it gates the entire admission decision.**

Every Grove edge has a confidence score (`calls` = 0.85 AST, 0.6 regex; `uses-type` = 0.5; etc.). The ICR confidence is computed from the edge confidences inside it. The admission policy uses this:

| ICR confidence | Action |
|----------------|--------|
| ≥ 0.85 | Auto-admit if certification passes |
| 0.70 – 0.85 | Admit with human review notification |
| 0.50 – 0.70 | Require human approval before admission |
| < 0.50 | Reject; ICR is unreliable, escalate to file-level locks |

Confidence comes from: edge type quality, file-class (low for migrations, config, generated code), and historical accuracy (Phase 3 — once we have data, calibrate).

### 8.4 Special file classes (the thing both reviewers hammered)

Symbol-level isolation breaks for files that aren't really symbols:

| File class | Lock strategy | Detection |
|------------|---------------|-----------|
| Migrations (`*_migrate.sql`, `migrations/**`) | Always serialize, file-level lock | Path pattern |
| Lockfiles (`go.sum`, `package-lock.json`, etc.) | Regenerate during admission | Path pattern |
| Generated files (`*.pb.go`, `*_generated.*`) | Forbid in ChangeSet OR regenerate in certification | Header marker or path |
| Config files (YAML, JSON config) | File-level lock; treat as opaque | Path pattern + content sniff |
| OpenAPI specs / schemas | File-level lock; trigger contract test | Path pattern |
| Test snapshots | File-level lock | Path pattern |

This is policy, not code. Lives in the `platform-config` repo as YAML.

### 8.5 Intent YAML schema additions (revised)

The Phase 1 schema is too thin. Both reviews proposed additions. The richer schema:

```yaml
apiVersion: intent/v1
kind: Intent
metadata:
  id: "INT-2026-042"
  parent: null                # null or parent ID if decomposed
  created_by: "alice@acme.com"

spec:
  title: "Add rate limiting to /api/auth/* endpoints"
  description: "..."
  domain: "auth"
  capability: "rate_limiting"
  acceptance_criteria: ["..."]
  constraints: ["..."]

  # Reviewer-recommended additions:
  allowed_paths: ["internal/auth/**", "tests/auth/**"]
  forbidden_paths: ["services/payments/**", "migrations/**"]
  verification_plan:
    must_pass_tests: ["auth_test.go::TestRateLimiter*"]
    must_run_sast: true
    must_check_secrets: true
  ambiguity_policy: "fail_with_questions"   # or "proceed_with_default"
  affected_interfaces: ["POST /api/auth/login", "POST /api/auth/refresh"]
  rollback_plan: "revert single commit; rate limiting is feature-flagged off by default"
  feature_flag: "rate_limiting_enabled"
  observability_expectations:
    - "rate_limit.allowed counter increments"
    - "rate_limit.denied counter increments on 429"
  security_considerations: "must not log raw IPs in production"
  risk_level: "low"                          # low|medium|high|critical
  related_artifacts:
    - type: "adr"
      url: "platform-config/adrs/0042-rate-limiting.md"

priority:
  level: "normal"
  reason: ""

execution:
  agent_class: "claude-sonnet-4-6"           # platform decides exact pod
  budget:
    cost_dollars_max: 5.00
    time_minutes_max: 15

status:
  state: "queued"
  granularity_score: 0.82
  icr_confidence: 0.91
```

### 8.6 Certificate format

Every admitted commit carries a certificate as a commit trailer. The certificate is what enterprises export to satisfy auditors.

```
Certificate-ID: cert-2026-05-28-a3f9b2
Certificate-Issued: 2026-05-28T14:33:09Z
Base-Commit: 9f2c1a4...
Intent-ID: INT-2026-042
Agent-Identity: claude-code:1.4.2
Model-Identity: claude-sonnet-4-6:2026-04-15
Prompt-Template-Hash: sha256:b1e9...
Context-Pack-Hash: sha256:7c2d...
ICR-Hash: sha256:d4e5...
ICR-Confidence: 0.91
Tests-Selected: 23
Tests-Passed: 23
SAST-Passed: true
Policy-Version: platform-config@4f8a...
Toolchain-Image: relay/cert-runtime@sha256:c3e7...
Signed-By: relay-admission-controller (key: 0xABCD1234)
```

This is what makes the platform auditable. Without this, there is no enterprise story.

---

## 9. Scaling to Enterprise (50,000–100,000 Repos)

The architecture in §8 is correct for the wedge buyer (100–5,000 engineers, 100–1,000 repos). It does **not** scale, as written, to a Fortune-50 enterprise with tens of thousands of projects. This section is what changes.

### 9.1 The numbers that force the rethink

A 100,000-repo enterprise looks roughly like:

- 30,000–60,000 active developers
- ~100,000–300,000 intents/day (peak 10–50/sec sustained, 1,000–10,000 concurrent in-flight)
- 20–150 TB/year of intent-store data (intent YAMLs + ChangeSets + certificates)
- ~25 billion symbols indexed across the suite (100K repos × ~5K files × ~50 symbols)
- $22M–$220M/year in raw LLM API spend before optimization

The Phase 2A design assumes one Postgres, one Redis, one intent-store git repo, one Grove. Each is a bottleneck at this scale.

### 9.2 What survives unchanged

Most of the design holds up. The per-intent execution loop, Fuse, Prism, the K8s pod model, the certificate format, the outbox pattern from Postgres to git — none of these change. What changes is topology, not concepts. The abstractions (intent, ICR, certification, admission, certificate, audit log) are correct at every scale.

### 9.3 Eight things that break and how to fix them

**Break 1 — Single Grove cannot index 100K repos.**
SQLite-backed Grove was built per-project. 25B symbols in one database is not viable.
*Fix:* **Grove Federation.** One Grove instance per source repo (or per tightly coupled monorepo group). A new **Grove Router** service holds the `repo → endpoint` map and forwards `/icr`, `/impact`, `/deps`, `/tests` calls. Hot tier (recent activity) keeps warm Groves; cold tier (archived repos) reads on demand from object-storage index snapshots. Grove's per-repo isolation makes the federation layer additive, not a rewrite.

**Break 2 — Single intent-store git repo cannot hold years of audit data.**
Hundreds of GB of pack files; clone times in hours; `git log` queries in minutes.
*Fix:* **Sharded intent-stores, hierarchical.** One repo per **tenant** (business unit / division / major product line — typically 50–500 source repos, 1K–50K intents/day). Monthly rollup commits archive older history to git pack files in object storage; the live repo stays manageable. A new **Audit Aggregator** service exposes a federated read API across all tenant intent-stores, used by compliance to answer queries like "show all AI-generated payments commits in Q2 2026" without cloning every shard.

**Break 3 — Single Postgres becomes a write hotspot.**
Several million intent-related writes/day plus event-log churn exceeds single-instance comfort.
*Fix:* Two tiers. **Mid-tier (10K–100K intents/day):** one Postgres cluster, schema-per-tenant, tables partitioned on `tenant_id` + `created_at`, read replicas for dashboards. **High-tier (100K+):** per-tenant Postgres instances. The data model already isolates by `project_id`; adding `tenant_id` as the parent is mechanical migration.

**Break 4 — Single Redis cannot hold ICR locks for 10K concurrent intents.**
*Fix:* **Redis Cluster per tenant.** ICR locks are tenant-scoped by definition — no intent in one tenant can ever lock symbols in another tenant's repos. Per-tenant sharding is natural; tenants share no Redis state.

**Break 5 — Single control plane becomes the bottleneck.**
*Fix:* **Multi-tenant control plane, stateless services.** Relay API stateless behind a load balancer. Orchestrator and admission controllers run as per-tenant worker pools with leader election (Postgres advisory locks or etcd). Dashboard stateless, reads via API. This is normal horizontal-scale service design; the design just has to commit to it from Phase 4A.

**Break 6 — K8s capacity for 5K–10K concurrent agent pods.**
*Fix:* **Multi-cluster K8s federation.** One cluster per region (data residency) and per major tenant (billing isolation). Karpenter or Cluster Autoscaler for spot/preemptible. **Critical:** node-level agent image caching — the agent runtime image (Claude Code SDK + Grove + Prism binaries) is multi-GB, and per-pod download kills cold-start latency. Pre-warmed pod pool for sub-30-second start times.

**Break 7 — LLM API spend explodes.**
$22M–$220M/year on-demand pricing is not survivable.
*Fix:* **Model routing + caching + dedicated capacity.**
- **Model routing:** Haiku for simple intents (GS > 0.85, ICR < 20 symbols), Sonnet for typical, Opus only for high-risk/complex. Phase 1's GS scorer already classifies — extend it to model selection.
- **Provider prompt caching:** Anthropic's system-prompt-prefix cache reduces token spend 40–70%.
- **Context pack caching:** same intent re-running with the same base commit reuses Prism's output.
- **Negotiated dedicated capacity:** at this scale, 30–50% off on-demand pricing is standard.
- **Realistic total reduction: 60–80%** — but still a multi-million-dollar annual line item. Customers buying at this scale know this is the explicit cost of governance ROI and budget for it.

**Break 8 — Identity, residency, sovereign compliance.**
A Fortune-50 has SSO, RBAC, EU data residency, sovereign deployment requirements that don't exist for the wedge customer.
*Fix:* **First-class multi-tenancy + multi-region.** OIDC/SAML SSO mapped to Relay tenants. RBAC roles: viewer, intent-author, reviewer, admin, security-champion. Per-tenant policy overrides on org-wide platform-config baseline. Tenant maps to region; no cross-region data flow without explicit federation policy. Separate Relay control plane per legal jurisdiction (EU, US, APAC), federated only for global audit queries.

### 9.4 Revised enterprise deployment topology

```
                    ┌────────────────────────────────┐
                    │  Global Audit Aggregator        │
                    │  (read-only federation API)     │
                    └─────────────┬──────────────────┘
                                  │
        ┌─────────────────────────┼────────────────────────┐
        ▼                         ▼                        ▼
   EU Region              US Region                  APAC Region
   Relay Control Plane    Relay Control Plane        Relay Control Plane
   (multi-tenant API,     (multi-tenant API,         (multi-tenant API,
    orchestrator,         orchestrator,              orchestrator,
    admission, dashboard) admission, dashboard)      admission, dashboard)
        │                         │                        │
    ┌───┴────┐                ┌───┴────┐                ┌───┴────┐
    Tenant A,B,C…             Tenant D,E,F…             Tenant G,H,I…
    each tenant:              each tenant:              each tenant:
      Postgres                  Postgres                  Postgres
      Redis Cluster             Redis Cluster             Redis Cluster
      intent-store git          intent-store git          intent-store git
      Grove Federation          Grove Federation          Grove Federation
      K8s namespace             K8s namespace             K8s namespace
```

A Fortune-50 customer deploys 3–10 regional control planes, 100–1,000 tenants across all regions, federated Grove indexes per source repo, multi-cluster K8s for agent execution, and a global Audit Aggregator. They run this with a 10–30 person internal platform team. Deployment is 6–12 months of professional services. ACV is $5M–$50M.

### 9.5 Phase 4 roadmap — Enterprise Scale

Roughly 12 months of engineering, gated on a real Fortune-500 design partner:

- **Phase 4A — Multi-tenancy primitives (12 weeks):** tenant data model, RBAC, per-tenant Postgres schemas + Redis clusters, SSO/OIDC integration
- **Phase 4B — Grove Federation (12 weeks):** Grove Router service, hot/cold tiering, object-storage index snapshots
- **Phase 4C — Intent-store sharding + Audit Aggregator (8 weeks):** per-tenant intent-stores, federated audit API, monthly rollup tooling
- **Phase 4D — Multi-region control plane (12 weeks):** regional deployment topology, data residency enforcement, global audit federation
- **Phase 4E — Cost optimization platform (ongoing):** model routing, prompt + context caching, per-tenant budget enforcement, billing-grade metering

**Total: ~18–24 months from now to enterprise-ready**, on top of Phase 2A/2B/3 (~9 months). Do not build Phase 4 speculatively — it requires a paying design partner whose requirements drive priorities.

### 9.6 What this means for go-to-market

The wedge tiers in §11.4 are correct up to ~1,000 developers. The enterprise tier needs re-anchoring:

- **Relay Enterprise (≥ 1,000 devs):** $300–$700/dev/month + platform deployment fee ($500K–$2M one-time) + annual support ($1M–$5M). **Total ACV: $5M–$50M.**
- **Anchors:** GitHub Enterprise Cloud ~$21/user, GitHub Advanced Security ~$49/user — Relay is 10–20× because it's platform infrastructure, comparable to Palantir / Snowflake Enterprise / Datadog Enterprise tier.

Requirements to sell at this price: dedicated enterprise sales (AE + SE + CSM per major account), 3–5 reference customers (the first 3 are concierge-deployed), SOC 2 Type II + ISO 27001 + FedRAMP Moderate minimum, 3–6 months of professional services capacity per customer. 2–3 year sales cycle per logo.

### 9.7 The honest answer

**No, the §8 design does not scale to 100K repos as written.** Yes, it scales with mechanical extensions, not a fundamental rethink. The core abstractions are correct at every scale; the deployment topology and operational tier are what change.

**Build for the wedge first.** Phase 4 is where the company becomes valuable, but it is not where the company is born. Trying to design and deploy directly to a Fortune-50 customer before the mid-market wedge is proven is the classic platform-startup death spiral: a 12-month custom deployment with no reference base, no productized playbook, and a single point of failure if that customer churns.

---

## 10. Roadmap

### Phase 2A — Certified Merge Wedge (target: 10–12 weeks)

**Goal:** A working end-to-end loop for a single agent's ChangeSet from any source.

- [ ] ChangeSet ingestion API (HTTP + CLI)
- [ ] Grove integration: `/impact`, `/tests`, `/deps` already wired in Phase 1; extend for ICR computation
- [ ] ICR computation with confidence scoring
- [ ] Policy gate (`allowed_paths`, `forbidden_paths`, secret scan, migration detector)
- [ ] Redis ICR locks with fencing tokens (not just SET NX EX — the review caught this)
- [ ] Certification pipeline (build, unit tests, SAST)
- [ ] Rebase + re-certify before admission
- [ ] Admission controller (linear commit with full trailer set)
- [ ] Certificate signing
- [ ] Outbox pattern for Postgres → intent-store git
- [ ] Intent YAML schema v2 (richer fields)
- [ ] Dashboard updates (cert details, ICR confidence, queue state)

**What's deferred from Phase 2A:** parallel agent decomposition, intent groups, canary, Fuse on critical path (Fuse exists but only invoked on rebase conflict, not as a normal step). Both reviews recommended this.

### Phase 2B — Self-Hosted Agent Execution (target: weeks 13–24)

**Goal:** Become a credible alternative to Cursor Background Agents / Devin for enterprise teams that need self-hosted, governance-aware agent execution.

- [ ] K8s operator for agent pods
- [ ] Ephemeral pod with: Grove + Prism + Claude Code/SDK + git clone
- [ ] Context delivery via Prism
- [ ] Cost tracking sidecar
- [ ] Heartbeat + dead-agent recovery
- [ ] Agent Decision Record in ChangeSet (Codex's recommendation)
- [ ] Ambiguity policy enforcement (`fail_with_questions` vs. `proceed_with_default`)

### Phase 3 — Parallel Decomposition, Intent Groups, Branchless (target: post-MVP, evidence-driven)

Both reviews are emphatic: **don't build this until Phase 2 produces enough data to validate ICR predictions empirically.** Specifically:

- Log every ChangeSet's ICR
- Log whether two consecutive ChangeSets would have been safe to parallelize
- After 6 months of real data, calibrate the confidence model
- Then, and only then, enable optional parallel execution
- Intent groups, canary, and the full intent-as-review-artifact UX come last

This is the discipline that separates a credible platform from one that overpromises.

### Phase 4 — Enterprise Scale

See §9.5 for full breakdown. Phase 4 is gated on a paying Fortune-500 design partner — do not start without one.

---

## 11. Go-to-Market Strategy

### 11.1 Licensing

- **Grove, Prism, Fuse: MIT.** Already done. Recruit the developer ecosystem. Make these projects the "obvious" code-graph + merge + context-delivery stack.
- **Relay: BSL.** Commercial offering. Source-available; production use above N seats / N intents per month requires a license.

This is the open-core strategy. Devin, Cursor, GitHub Copilot all benefit from using Grove and Prism. They cannot trivially replicate Relay, and they have no reason to compete on it because they sell agents, not infrastructure.

### 11.2 Target buyer

Not the developer. The **VP Platform Engineering** (champion) and **CISO** (co-signer). Their pain is:

- "We have to allow our teams to use AI coding tools."
- "We have no way to certify, audit, or constrain what those tools produce."
- "The EU AI Act audit is coming."

Relay sells against that pain.

### 11.3 Wedge customer profile

- Mid-market to enterprise (100–5000 engineers)
- Already using at least one AI coding tool (Cursor / Copilot / Claude Code)
- Active SOC 2 or in scope for EU AI Act / sector-specific compliance (fintech, healthcare, defense)
- Has a "platform team" — not a small startup

Avoid: very early-stage startups (no budget, no compliance), pure SMB (won't pay enterprise prices).

### 11.4 Pricing (anchoring)

- **Open Source (Grove + Prism + Fuse):** free, MIT
- **Relay Team (BSL):** free for ≤ 10 seats, ≤ 200 certified commits / month — community trial
- **Relay Business:** $50–$100 per developer per month, includes certification engine, audit export, policy library
- **Relay Enterprise:** $250–$500 per developer per month, includes self-hosted agent execution platform, on-call support, SOC 2 / EU AI Act compliance attestation, custom policy engineering

Reference: Cursor Pro is $20/month; Devin Core is $20/month; GitHub Copilot Enterprise is $39/month. Relay is more expensive per seat because it's platform infrastructure, not a developer tool — the buyer comparison is closer to GitHub Advanced Security ($49/month/user) or Snyk Enterprise.

### 11.5 Distribution

- **OSS-led developer adoption** via Grove and Prism. These should be GitHub-trending, Hacker News-discussed projects in their own right.
- **Inbound from compliance-driven enterprise.** Press the EU AI Act angle. "Relay is the certified delivery layer for AI-generated code" — write the white paper.
- **Partnerships with agent vendors.** Cursor, Devin, GitHub Copilot do not want to build governance infrastructure. We are a complement, not a competitor. The pitch to them is: "Your customers ask you for self-hosted, governance-aware execution — recommend Relay."
- **Conference presence:** KubeCon, Open Source Summit, Black Hat, RSA — where platform and security buyers live, not where developers live.

---

## 12. Honest Success Probability Assessment

I will be direct about each scenario.

### 12.1 The strong case (probability: 20–30%)

Relay becomes the canonical "certified delivery layer for AI code." Open-core revenue ramps to $20M ARR in 18 months, $80M in 36 months. Strategic acquirer (GitHub, Atlassian, Datadog, Snyk, Microsoft, or a hyperscaler) buys for $200M–$500M at the 3–4 year mark.

**What makes this happen:**
- EU AI Act enforcement creates urgent demand
- A high-profile incident (an AI-generated commit causes a security breach at a known enterprise) makes the governance story unmissable
- Cognition, Cursor, GitHub all start recommending Relay as the governance layer
- Grove and Prism become "the" code-graph stack and pull Relay enterprise sales through
- **Phase 4 is built on a real Fortune-500 design partner whose ACV ($5M–$50M) underwrites the engineering investment.** The strong case requires this — open-core mid-market revenue alone caps out around $20M ARR. Breaking past that requires winning at least one enterprise reference deployment in year 2–3.

### 12.2 The base case (probability: 40–50%)

Relay becomes a respected platform with 20–100 paying enterprise customers, $5M–$20M ARR. Solid business, not a category-defining outcome. Likely outcome: continues as an independent open-core company, or modest acquisition ($30M–$100M) by a CI/CD or security vendor.

**What makes this happen:**
- Sales cycles in enterprise are 9–18 months; the company grows steadily but not explosively
- Some agent vendors build basic governance features themselves, squeezing the differentiation
- Open-source competitors emerge for Grove and Fuse, commoditizing those layers

### 12.3 The weak case (probability: 25–35%)

Agent vendors (Cursor, Cognition, GitHub Copilot) ship "enterprise governance" features before Relay reaches scale. Relay remains a high-quality open source project; commercial offering does not find product-market fit at premium pricing. Founders shut down the commercial entity or pivot.

**What makes this happen:**
- The agent vendors realize governance is a moat and prioritize it
- Enterprises prefer bundled solutions over best-of-breed
- The Relay pitch is too platform-engineering for buyers who want one vendor

### 12.4 What you should bet on

Build for the strong case. Architect open-source so the weak case still leaves a respected, durable engineering footprint. The base case is the most likely numerical outcome — design the business so it is acceptable, but not the goal.

---

## 13. What Has to Be True for This to Win

Three load-bearing assumptions. If any of them fails, the strategy is wrong.

### 13.1 Assumption 1: Verification debt is a CISO/VP Eng problem, not a developer problem

If developers are willing to live with the PR review backlog because "the tests pass," there's no urgency. The Relay buyer needs to be someone whose job is on the line when an AI-generated commit causes an incident.

**How to test this:** Five 30-minute calls with VPs of Platform Engineering at companies using Cursor / Copilot at scale. If they all say "yeah, we have governance figured out" — the thesis is wrong. If three of five say "we are terrified of what's about to happen" — the thesis is right.

**Likelihood the assumption holds:** HIGH. EU AI Act regulatory pressure, recent industry data on agent code vulnerabilities, and the documented "verification debt" all point this way.

### 13.2 Assumption 2: Code knowledge graph is real differentiation, not a checkbox

If the agent vendors decide to bundle a "code graph" of their own — even a worse one — the Grove differentiation could collapse from a moat to a footnote.

**How to test this:** Track whether Cursor, Cognition, GitHub ship a "code graph" feature in the next 12 months. If they do, evaluate quality. Grove's depth (11 languages, content-addressed delta indexing, 8 edge types with confidence scores) is hard to replicate quickly, but not impossible.

**Likelihood the assumption holds:** MEDIUM. Industry research confirms code graphs are becoming table stakes — but most attempts are shallow. Grove's quality is real and durable for 18 months minimum.

### 13.3 Assumption 3: Open-core can compete with venture-funded incumbents

Cursor and Cognition have $1B+ of capital each. They can outspend us on marketing, sales, and integration partnerships. The open-source bottom-up motion has to actually work — Grove and Prism have to become the default infrastructure used by competing agents.

**How to test this:** GitHub stars, downloads, integration mentions in the next 6 months. If by month 6, Grove has < 1,000 stars and is not mentioned in any "best of" lists, the open-source strategy is failing. If it has 5,000+ stars and shows up in CodeGraph / Sourcegraph comparison articles, the strategy is working.

**Likelihood the assumption holds:** MEDIUM. Open-core has worked (HashiCorp, Elastic, MongoDB) and failed (many others). Grove and Prism are technically strong — the marketing/distribution work has to match.

---

## 14. Recommendation

**Build the Certified Merge wedge product in 10–12 weeks.**

**Do NOT build parallel decomposition, intent groups, branchless main, or full intent-as-review-artifact yet.** Both design reviews are right. The wedge proves the platform thesis with a single-intent loop, conservative ICR locking, mandatory rebase + re-certify, and full audit trail.

**Position the platform as agent-agnostic delivery infrastructure** — explicitly designed to work alongside Cursor, Claude Code, Devin, Copilot Workspace. Not a competitor to them. Not a replacement for GitHub. A platform layer underneath them.

**Sell to platform engineering and security buyers**, not developers. Price like infrastructure ($50–$500 per developer per month tiers), not like a developer tool.

**Open source Grove, Prism, and Fuse aggressively.** They are the moat — the more agent vendors that use them, the more inevitable Relay becomes for the customers of those vendors.

**Reserve the branchless / intent-driven vision for Phase 3.** Ship it when there is six months of real ICR data to back the claim. Until then, the marketing claim is the calibrated one: *"Relay makes branches unnecessary for approved classes of agent-delivered work."*

---

## Sources

Market data, competitor information, and adoption metrics cited in this document:

- Cognition AI / Devin valuation and revenue: [TechCrunch — AI coding startup Cognition raises $1B at $25B pre-money valuation](https://techcrunch.com/2026/05/27/ai-coding-startup-cognition-raises-1b-at-25b-pre-money-valuation/), [VentureBeat — Devin 2.0 is here](https://venturebeat.com/programming-development/devin-2-0-is-here-cognition-slashes-price-of-ai-software-engineer-to-20-per-month-from-500), [Sacra — Cognition revenue, valuation & funding](https://sacra.com/c/cognition/)
- Blitzy funding and platform architecture: [Crunchbase News — Blitzy Raises $200M At $1.4B Valuation](https://news.crunchbase.com/ai/blitzy-funding-valuation-autonomous-software-development-vibe-coding-startups/), [Blitzy — How It Works](https://blitzy.com/how_it_works), [BusinessWire — Blitzy $200M raise](https://www.businesswire.com/news/home/20260505342338/en/Blitzy-Raises-$200-Million-at-$1.4-Billion-Valuation-to-Advance-Autonomous-Software-Development-for-the-Enterprise)
- AI code volume / PR review bottleneck: [The New Stack — Hidden tax on every AI-generated merge request](https://thenewstack.io/hidden-tax-ai-code/), [DEV Community — The Review Bottleneck](https://dev.to/code-board/the-review-bottleneck-why-more-ai-code-means-slower-teams-in-2026-1e5n), [MetaCTO — Code Review Is the New Bottleneck](https://www.metacto.com/blogs/code-review-bottleneck-ai-development)
- Enterprise adoption / pilot-to-production gap: [Digital Applied — AI Agent Adoption 2026: 120+ Enterprise Data Points](https://www.digitalapplied.com/blog/ai-agent-adoption-2026-enterprise-data-points), [Northflank — Enterprise AI coding agent deployment](https://northflank.com/blog/enterprise-ai-coding-agent-deployment), [The New Stack — AI-generated code crisis](https://thenewstack.io/ai-generated-code-crisis/)
- GitHub Copilot Workspace and Cursor Background Agents: [GitHub Blog — Agent pull requests are everywhere](https://github.blog/ai-and-ml/generative-ai/agent-pull-requests-are-everywhere-heres-how-to-review-them/), [Developers Digest — GitHub Copilot Coding Agent](https://www.developersdigest.tech/blog/github-copilot-coding-agent-cli-2026), [TrueFoundry — Cursor vs Copilot 2026](https://www.truefoundry.com/blog/cursor-vs-github-copilot)
- Multi-agent coordination patterns: [Augment Code — How to Run a Multi-Agent Coding Workspace](https://www.augmentcode.com/guides/how-to-run-a-multi-agent-coding-workspace), [Mike Mason — AI Coding Agents in 2026: Coherence Through Orchestration](https://mikemason.ca/writing/ai-coding-agents-jan-2026/)
- Code knowledge graph trend: [Sourcegraph — Agentic Coding in 2026](https://sourcegraph.com/blog/agentic-coding), [AgentConn — CodeGraph: The Missing Knowledge Graph](https://agentconn.com/blog/codegraph-pre-indexed-knowledge-graph-multi-agent-claude-code-codex-2026/)
- Semantic merge / AgenticFlict dataset: [arXiv — AgenticFlict: A Large-Scale Dataset of Merge Conflicts in AI Coding Agent Pull Requests](https://arxiv.org/html/2604.03551v2)
- AI governance / EU AI Act: [CSA Labs — AI Agent Governance Framework Gap](https://labs.cloudsecurityalliance.org/research/csa-research-note-ai-agent-governance-framework-gap-20260403/), [Agentic AI Institute — Enterprise Adoption 2026: Governance Gap](https://agenticaiinstitute.org/agentic-ai-enterprise-adoption-2026-governance-gap/), [Ethyca — AI Governance Framework](https://www.ethyca.com/guides/ai-governance)
- Intent-driven development trend: [Sigma Junction — Intent-Driven Development: Why Writing Code Is Becoming Optional in 2026](https://sigmajunction.com/blog/intent-driven-development-writing-code-optional-2026), [KodeNerds — Intent-driven development 2026](https://www.kodenerds.com/intent-driven-development-2026), [Thoughtworks — AI and software delivery](https://www.thoughtworks.com/en-us/insights/looking-glass/looking-glass-2026/AI-and-software-delivery)
- Vibe coding / specification-driven platforms: [a16z — The Trillion Dollar AI Software Development Stack](https://a16z.com/the-trillion-dollar-ai-software-development-stack/), [Daily.dev — Vibe Coding in 2026](https://daily.dev/blog/vibe-coding-how-ai-changing-developers-code/)
