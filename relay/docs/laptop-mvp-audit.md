# Relay Laptop MVP — Audit and Resolved Position

Status: agreed direction as of 2026-05-30. This document captures the audit of the laptop-mode MVP, the resolution of what "laptop mode" actually means, the engineering gaps still to close, and the auto-intent-capture flow that becomes a first-class capability on laptop.

For positioning, market context, and the broader product proposal, see [`product-proposal.md`](product-proposal.md). For architecture, see [`architecture.md`](architecture.md). For the per-phase build plan, see [`implementation-plan.md`](implementation-plan.md).

---

## 1. The Model

**Laptop Mode is a complete Relay server running on the developer's machine, single user.**

Full pipeline: ingest → ICR → policy → certification → admission → signing → intent-store → audit. Backed by embedded SQLite instead of Postgres, no Redis (single-agent, locks irrelevant), local Ed25519 key instead of KMS, local `~/.relay/intent-store/` git repo instead of a shared one. The developer IS the team-of-one. They sign their own commits, accumulate their own audit trail, and run `relay certify` against their own admission controller.

**Team Mode** is the same server topology lifted off the laptop onto a shared machine. Multiple developers submit ChangeSets, Redis locks become meaningful, Postgres replaces SQLite, intent-store is shared, KMS replaces local key. Binary identical. `.relay/` identical. Certificate format identical.

**Company Mode** is the team server sharded across tenants and regions. Mechanical scale-out.

The transition is genuinely just configuration plus a one-time `relay migrate sqlite-to-postgres`. The dev's signed cert history from laptop mode is still verifiable in team mode because the same trailer format, same hash chain, same intent-store repo (now pushed to the team server) carry forward.

---

## 2. Why Full-Server-On-Laptop Is the Right Model

### 2.1 Architectural unity is the product's most defensible claim

"Same engine, three topologies" is the line that distinguishes Relay from every linter/CI/governance tool that requires you to adopt different tools at different scales. The moment laptop becomes "pre-flight only" and team becomes "full admission," the story fractures — now there are two products, two upgrade paths, two value propositions to explain to a buyer. Keeping the full stack runnable on a laptop preserves the single-narrative pitch.

### 2.2 The audit trail compounds from day one

A solo developer who's been using Relay for 14 months has a signed, intent-tagged history of every AI-assisted commit they ever made. When they hire developer #2, that history is the team's starting compliance corpus — not a reset-to-zero. That continuity is what makes the open-source-laptop-to-paid-team funnel actually work. Pre-flight-only laptop would break this completely.

### 2.3 Solo developers do have a use for signed certs

Contractors delivering AI-generated code to regulated clients, indie developers shipping to App Store / Play Store review, anyone publishing OSS who wants a verifiable record of "this commit was AI-assisted, here's the agent and model, here's the static-analysis result" — these are real audiences. Not every solo dev wants this, but the ones who do have no other tool that gives it to them.

### 2.4 The complexity is bounded

SQLite plus a local git repo is invisible infrastructure — every developer has both already. No Postgres, no Redis, no Kubernetes, no key vault on the laptop. The dev sees a binary, a `.relay/` directory, and signed commits. The "server" framing is a mental model; operationally it's a single Go binary plus two files (`~/.relay/db.sqlite` and `~/.relay/intent-store/.git/`).

---

## 3. What Stays in the Laptop Spec

These were briefly cut in a first audit pass and are reinstated under the full-server-on-laptop model.

| Element | Status | Nuance |
|---------|--------|--------|
| `relay-main` admission target branch | **Reinstated as configurable.** | Default admission target on laptop is the dev's current branch (or `main`); `relay-main` is the team-mode default. Same mechanism, different default. |
| Local intent-store git repo (`~/.relay/intent-store/`) | **Kept.** | Solo dev's audit log and seed of the team's audit trail. |
| Outbox reconciler | **Kept.** | Only correct path from SQLite to local intent-store git. No-op only when intent-store is explicitly disabled. |
| Fuse semantic merge on rebase conflict | **Kept.** | Cheap to ship; useful when a solo dev rebases against an upstream they don't control. |
| Coverage gate | **Kept with default = warn on laptop.** | Configurable to enforce. |
| Diff Risk Heatmap | **Kept.** | Reads from the certificate payload, which exists on laptop. |
| Evidence Replay (foundational) | **Kept.** | Genuinely useful for a dev's own past certs ("did this commit pass the rules in force at the time?"). |
| `relay migrate sqlite-to-postgres` | **Kept.** | Laptop-to-team upgrade story. Ship as part of Phase 2A so the funnel is real. |
| SonarQube profile importer | **Kept as P2.** | Useful for teams adopting Relay alongside existing SonarQube; not P0 for laptop MVP. See [§7](#7-sonarqube-without-a-remote-server-investigation) for the no-remote-server approach. |
| Full pipeline (ingest → admission → signing) | **Kept on laptop.** | Backed by SQLite + local Ed25519 key + local intent-store git. |

---

## 4. Real Engineering Gaps to Close Before Laptop MVP Ships

These are gaps in what the docs currently promise — not framing problems. Each must be specified and built before the laptop binary is fit for daily use.

### 4.1 Cross-platform distribution and signing chain

The docs say "single binary," "`brew install`", "`curl | sh`", and "single Docker image" but never commit to:

- **Platforms in Phase 2A.** Decision: **macOS (Intel + ARM) and Linux (x86_64 + ARM64) only in Phase 2A. Windows requires WSL.** Semgrep and other Sonar/Java-based analyzers are hostile on Windows native; an explicit stake-in-the-ground prevents Windows users from hitting a wall on day one.
- **macOS Gatekeeper.** The binary must be Developer-ID signed and notarized. Without it the first `relay` launch is blocked with no remediation.
- **Goreleaser configuration + checksums + Homebrew tap.** Add to deliverables.

### 4.2 Long-running laptop daemon for concurrent MCP clients

`relay mcp serve` as a stdio subprocess of each IDE means each Claude Code, Cursor, Continue, or Windsurf instance launches its own Relay subprocess. Two of the most common laptop scenarios both break:

- Developer runs Claude Code + Cursor in parallel against the same repo.
- Developer runs an agent in one terminal while editing in another with the git pre-push hook active.

Each subprocess opens its own SQLite, tries to auto-start Grove, and writes the intent-store git repo independently. SQLite file locking with WAL handles some of this; the intent-store git writes don't.

**Decision: ship a long-running `relay daemon` for laptop mode.** Stdio MCP transports are thin shims that proxy to the daemon over a Unix domain socket. The daemon owns SQLite, the local Grove client, the intent-store git, and the signer key. CLI commands and the git hook also proxy to the daemon when running.

### 4.3 MCP client auto-registration

Today, hooking Relay into each IDE requires hand-editing each client's MCP config and restarting. Multiplied across IDEs and machines this is fatal friction for a free-tier funnel.

**Add: `relay mcp install-for <client>` shims for Claude Code, Cursor, Continue, Windsurf.** Idempotent, reversible, detects already-installed.

### 4.4 Default admission target = current branch on laptop

The solo-dev journey today describes pushing to a `relay-main` branch parallel to `main` and fast-forwarding. That's a team-mode pattern. On laptop, default to commit on the current branch with trailers. Make `relay-main` a team-mode default, not the laptop default. Document accordingly.

### 4.5 `relay_check` vs `relay_certify` cost contract

The current spec says Stage 1 cert runs the "full unit/integration suite by default." That's correct for `relay_certify` (commit-ready), wrong for `relay_check` (agent in-loop). Every iteration of the agent cannot afford to run the full suite.

**Split the contract:**
- `relay_check` (in-loop, pre-flight): changed-files SAST + Grove-affected unit tests only. Target: sub-10-second on a typical change.
- `relay_certify` (commit-ready): full Stage 1 + Stage 2 + admission preparation.

### 4.6 Coverage gate defaults to warn on laptop

Coverage-of-changed-symbols is risky as a deny gate on a fresh laptop install when Grove's `tests`-edge inference for TypeScript and Python may be sparse. False positives cause the agent to "fix" non-bugs.

**Default: warn on laptop. Enforce only when `.relay/policies/coverage.yaml` sets `mode: enforce`.** Escape hatch annotation (`// relay:no-test-required <reason>`) already specified — keep it.

### 4.7 Per-repo state location

SQLite operational state is "embedded" — one DB per binary, or one per repo? Not specified.

**Decision: per-repo at `.relay/.cache/state.sqlite`, gitignored** by a `.relay/.gitignore` written by `relay init`. Same pattern as `.terraform/`, `.next/`. The daemon's higher-level coordination state lives at `~/.relay/daemon.sqlite`.

### 4.8 Ed25519 key lifecycle (load-bearing on laptop)

"Local Ed25519 keypair in laptop mode" never specifies:

- Where the key lives — **decision: `~/.relay/keys/admission.ed25519`**.
- Who generates it — **decision: `relay init` if absent, or first `relay certify` if `relay init` was skipped**.
- Mode bits — **0600**.
- What happens on reinstall — past certs become unverifiable on the new machine. **Decision: print fingerprint on every generation; allow `relay keys export` / `relay keys import` for portability.**
- Whether the public key is published anywhere — **on laptop, no.** Verifying party reads the fingerprint from the cert trailer and trusts on first use (TOFU). When the dev upgrades to team mode, the team server's key registry replaces TOFU.

### 4.9 Tool version pinning is in the engine release, not just `.relay/relay.yaml`

`relay_version` (engine pin) is specced; tool versions (semgrep, gitleaks, etc.) are not. Two developers on the same `.relay/` but different semgrep versions get different findings — and the `Effective-Config-Hash` determinism promise is silently broken.

**Decision: the Relay binary release ships a tool-set manifest — pinned versions of semgrep, gitleaks, govulncheck, lang-linters, and bundled SonarSource analyzers (see [§7](#7-sonarqube-without-a-remote-server-investigation)). `relay tools install` honors that pin. Deviation requires explicit `--tool-override` and is recorded in the certificate as `Toolchain-Image-Drift: true`.**

### 4.10 Privacy / telemetry posture

For an MCP product that runs in IDE context, security-aware developers ask the privacy question within the first minute. A vague answer kills enterprise pull-through later.

**Default: laptop mode does not phone home. OpenTelemetry traces default to local stdout, disabled by default. Agent identity, prompt hash, and intent content never leave the machine unless the developer explicitly enables sync to a team server. Document in `getting-started.md`.**

### 4.11 Cold-start UX

First Grove index of a 10K-file repo takes minutes. The user experience during indexing is unspecified.

**Decision: background indexing kicks off on `relay init`; `relay_check` and CLI calls return a friendly "indexing — N% complete, partial findings available" until index is warm. Indexer progress streamed to the dashboard (when running) and to stdout.**

### 4.12 Structured infrastructure-error surface

If Grove crashes mid-check, a bundled tool is missing, or `.relay/` cannot be found, `relay_check` must return a structured `infrastructure_error` finding — not a "fail" — so the agent does not try to "fix" a Relay problem.

**Add to `relay.findings/v1` schema: a finding class `infrastructure_error` with `cause`, `remediation`, and `is_actionable: false`. Agents are told (via the system-prompt fragment) to surface these to the user instead of attempting auto-fix.**

---

## 5. Auto-Intent Capture (New First-Class Capability on Laptop)

The most important new capability falling out of this audit: the prompt that produced the code is itself the most useful piece of context for reviewing it, and today it is thrown away. Git and GitHub treat the diff as primary; the prompt evaporates with the agent session.

Relay can capture it.

### 5.1 Flow

```
1. Developer prompts the agent (Claude Code / Cursor / etc.):
   "Add rate limiting to /api/auth/* endpoints, 100 req/min per IP"

2. Agent's system prompt (shipped as docs/agent-prompt.md with Relay) instructs:
   "Before making code changes, call relay_intent_open with the user's
    request as title and description."

3. Agent calls relay_intent_open. Relay drafts an Intent YAML:
   - id: INT-2026-05-30-rate-limiting
   - title: "Add rate limiting to /api/auth/* endpoints"
   - description: <user's verbatim prompt>
   - originated_from:
       agent: claude-code:1.4.2
       model: claude-sonnet-4-6:2026-04-15
       conversation_ts: 2026-05-30T14:33:09Z
   - allowed_paths: (inferred from prompt + .relay/intents/templates)
   - acceptance_criteria: (drafted from prompt; agent can refine)
   Stored at .relay/.cache/intents/INT-{id}.draft.yaml (gitignored)

4. Agent writes code. Each relay_check call passes the intent_id.
   Relay can additionally enforce: "does this diff stay inside allowed_paths?"
   as another in-loop signal.

5. When the agent reports complete (or developer runs relay certify):
   - Draft is promoted to .relay/intents/INT-{id}.yaml (committed)
   - Commit trailer: Intent-ID: INT-2026-05-30-rate-limiting
   - The intent file is part of the diff → shows up in the GitHub PR
   - Server-side admission (team mode) reads the intent + cross-validates
     against the diff before signing
```

The developer never types an intent. They prompt the agent like they already do. The agent does the bookkeeping. The intent lands in the repo as a YAML file next to the code, gets PR-reviewed alongside the code, and becomes the canonical "why this commit exists."

### 5.2 Why this is genuinely new

| Tool | What gets persisted about the change |
|------|--------------------------------------|
| Git commit message | What the dev decides to summarize |
| GitHub PR description | Same, manual, often empty |
| Claude Code / Cursor session | Lost on session close |
| Conventional Commits | A type prefix, no semantic content |
| Devin session replay | Vendor-locked URL on Cognition's servers |
| **Relay Intent** | **The user's actual prompt + agent's model/version + acceptance criteria, committed alongside the code as YAML** |

### 5.3 Storage options (decision: A, with C as the local draft surface)

| Option | Where | Verdict |
|--------|-------|---------|
| **A. Committed YAML** (`.relay/intents/{id}.yaml`) | In the repo | **Chosen.** PR-reviewable, server-side reads it, durable. |
| B. Commit trailer with inline intent | Commit message | Reference-only via `Intent-ID:` trailer pointing to (A). |
| **C. Local-only draft** (`.relay/.cache/intents/{id}.draft.yaml`) | Gitignored | **Chosen for drafts before commit.** Promoted to (A) at commit. |
| D. Git notes (`refs/notes/relay-intents`) | Separate git ref | Rejected — invisible to most developers by default. |

### 5.4 MCP tools added in Phase 2A

| Tool | Purpose |
|------|---------|
| `relay_intent_open` | Draft an intent from a user prompt; returns intent ID. Stores draft at `.relay/.cache/intents/`. |
| `relay_intent_close` | Promote the draft to `.relay/intents/{id}.yaml` for commit; called when the agent reports complete. |
| `relay_intent_update` | Refine title / description / acceptance criteria mid-session. |
| `relay_intent_list` | List open and committed intents for this repo. |

These tools are additive to the existing `relay_check`, `relay_certify`, `relay_submit`, `relay_policy`, `relay_explain` set.

### 5.5 How the agent actually generates the intent

Two paths, both real:

1. **Agent-drafted, dev-confirms (recommended).** The agent emits the YAML from the user's prompt plus its own plan. Dev sees the draft on first `relay_check` and can edit `.relay/.cache/intents/{id}.draft.yaml` before commit if they want. No extra LLM cost.
2. **Relay-drafted via LLM call.** Relay accepts a `context` blob from the agent (user's prompt + planned diff), calls a small model to extract structured fields, and emits the YAML. Lower friction; depends on Relay having model access.

**Default for laptop MVP: Path 1.** No additional model costs; the agent already has the context.

### 5.6 Server-side amplifies the value

When the dev pushes to GitHub, server-side Relay (a GitHub Action or a hosted instance) reads `.relay/intents/{id}.yaml`, validates that the diff stays inside `allowed_paths`, that acceptance criteria are met (or `ambiguity_policy: proceed_with_default` was set), and signs a certificate that includes `Intent-ID`, `Intent-Hash`, agent identity, etc. The intent becomes the *contract* the admission controller verifies against. Reviewers see it on the PR. Auditors can replay it.

On laptop, the same admission controller (now running locally) signs the same way. The artifact is identical — what differs is who runs the binary.

---

## 6. Action Items (Updated Implementation Plan Deltas)

Items to add to or change in [`implementation-plan.md`](implementation-plan.md) Phase 2A:

1. **Distribution & signing chain.** macOS notarized binary, Linux x86_64 + ARM64, Goreleaser config, Homebrew tap, install script. Windows explicitly = WSL.
2. **`relay daemon`.** Long-running local daemon for laptop mode; stdio MCP shims, CLI, and pre-push hook all proxy to it over Unix socket.
3. **MCP client auto-registration.** `relay mcp install-for {claude-code,cursor,continue,windsurf}`.
4. **`relay_check` cost contract.** Changed-files SAST + Grove-affected tests only. Sub-10-second target on typical change.
5. **Coverage gate default = warn on laptop.** Configurable to enforce.
6. **Per-repo SQLite location.** `.relay/.cache/state.sqlite`; written by `relay init` with `.relay/.gitignore`.
7. **Ed25519 key lifecycle.** Generation, location, mode bits, `relay keys export`/`import`, fingerprint print on every cert.
8. **Tool-set manifest pinned per Relay release.** `relay tools install` honors the manifest; drift recorded in cert trailer.
9. **Privacy / telemetry posture.** No phone-home by default; documented in `getting-started.md`.
10. **Cold-start UX.** Background indexing with partial-result `relay_check` during warm-up.
11. **Infrastructure-error finding class.** Added to `relay.findings/v1` schema; agent system prompt instructs to surface, not fix.
12. **Default admission target = current branch on laptop**, `relay-main` on team. Same mechanism, different default.
13. **Auto-Intent Capture.** Four new MCP tools (`relay_intent_open|close|update|list`); intent YAML schema; draft → committed promotion flow; `.relay/intents/` directory convention.
14. **Server-side ingest of `.relay/intents/`.** Team-mode admission reads the intent file and cross-validates against the diff.

Items deferred (P2):

15. **SonarQube profile importer** — see [§7](#7-sonarqube-without-a-remote-server-investigation) for the no-remote-server analysis. Implementation lands when the bundled-analyzer path is chosen.

---

## 7. SonarQube Without a Remote Server (Investigation Summary)

Full analysis: [`sonarqube-no-server-investigation.md`](sonarqube-no-server-investigation.md). Headline findings:

1. **Yes, real SonarQube-grade analysis is achievable on a laptop with no server.** The VS Code "SonarQube for IDE" extension already does this — it spawns a Java subprocess (`java -jar sonarlint-ls.jar`) that runs the same analyzer JARs SonarQube Server uses, against a default rule set or an imported quality profile, with no server contacted.

2. **The embedding target is `SonarSource/sonarlint-core`** (LGPL-3.0, actively maintained). Its `StandaloneSonarLintEngine` API is what every SonarLint IDE plugin wraps. The legacy `sonarlint-cli` repo is archived since January 2018 and not usable.

3. **Recommended path for Relay: Hybrid.** Default `relay tools install` stays lean (semgrep + gitleaks + lang linters, ~50MB, no JRE). When a user runs `relay tools install --with-sonar` or `relay import sonarqube-profile <profile.xml>`, Relay fetches a portable Eclipse Temurin JRE 21 + SonarLint Core + a small `relay-sonar.jar` wrapper + analyzer JARs for the project's languages (~200–400MB total opt-in cost). Cert trailer records the engine in use (`Sonar-Engine: sonarlint-core@X.Y` or `none`).

4. **Quality profile XML import is the right user-facing surface.** SonarQube Server exports profiles as XML via UI ("Back up") and API. The same XML loads directly into SonarLint Core. Relay's `relay import sonarqube-profile` writes the XML to `.relay/rulesets/<name>.xml` (committed; travels with the repo) and registers it for the engine to load at analyze time.

5. **What is NOT available in no-server mode** (LGPL-edition limits, not Relay limits): injection / taint vulnerability analysis (requires SonarQube Server Enterprise Edition); COBOL, Apex, PL/SQL, T-SQL (commercial editions only); cross-project dashboards and PR decoration (those are server products, not analyzers). Document these clearly so enterprises aren't surprised.

6. **Phasing:** Phase 2A ships the importer + ruleset surface only (lazy engine fetch). Phase 2A-late / 2B ships the `relay-sonar.jar` wrapper + JRE bundling + the runtime path that uses SonarLint instead of (or alongside) semgrep for relevant languages. Phase 4 adds connected-mode sync for enterprises that maintain canonical profiles on a SonarQube/Cloud server.

---

## 8. What Remains Unchanged

These claims, capabilities, and constraints are unchanged by this audit:

- The full ingest → ICR → policy → certification → admission → signing pipeline.
- The six policy gates (path, secrets, fileclass, deps, size, coverage).
- The Stage 1 (build + tests + coverage) and Stage 2 (SAST + linters) certification structure.
- The certificate format, with `Effective-Config-Hash`, `Repo-Config-SHA`, `ICR-Hash`, `Tests-Selected`, etc.
- The signature capabilities for Phase 2A: Pre-Flight Autopilot, AI Code Passport, Diff Risk Heatmap, Evidence Replay foundational, Policy Marketplace bootstrap.
- The signature capabilities deferred to Phase 2B: Surgical Revert by Intent, Human Review Budget Optimizer, Agent Scorecard (full).
- The architectural rejection of: standalone Diff Comprehension UI, Self-Healing Sandbox auto-fix loop outside the agent loop, Relay-native code-review chat.

---

## 9. Open Questions

1. Does the laptop daemon auto-launch as a launchd / systemd service, or only on first CLI / MCP use? Recommendation: on first use, then persist as a user-level service.
2. When the developer upgrades to team mode, what happens to the local `~/.relay/intent-store/`? Push as-is to the team server, or merge into the shared store? Decision needed before `relay migrate sqlite-to-postgres` ships.
3. SonarQube path — to be decided in [§7](#7-sonarqube-without-a-remote-server-investigation) follow-up.
4. Should `relay_intent_open` accept a structured `affected_files` hint from the agent, or always derive from the diff at `relay_intent_close` time? Likely both: hint at open, validate at close.
