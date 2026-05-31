# Relay — Implementation Plan (Per Phase)

Each phase below has: goal, scope, deliverables, exit criteria, dependencies, and risks. Phases gate on exit criteria, not calendar time.

Reference: `product-proposal.md` §10, §9.5; `architecture.md`; `design.md`.

---

## Phase 2A — Certified Merge Wedge

### Goal
End-to-end loop: any agent → ChangeSet → Grove-driven certification → signed admission to a linear branch with full audit trail. **Single binary running in laptop mode (full Relay server, single user, SQLite, no Redis, local Ed25519 key, long-running `relay daemon`) or team mode (Postgres + Redis, KMS, shared intent-store), with MCP as the primary agent surface, Auto-Intent Capture as a first-class capability, and pre-bundled static-analysis tooling.** Single tenant, single region, sequential. Audit findings landed via [`laptop-mvp-audit.md`](laptop-mvp-audit.md); SonarQube engine path defined in [`sonarqube-no-server-investigation.md`](sonarqube-no-server-investigation.md).

### Scope (in)

**Deployment modes (design.md §12.5):**
- **Laptop mode = full Relay server, single user.** Single binary, embedded SQLite at `.relay/.cache/state.sqlite` (per repo) + `~/.relay/daemon.sqlite` (daemon-level), local intent-store git repo at `~/.relay/intent-store/`, no Redis (single-agent, daemon is the single writer), local Ed25519 admission key at `~/.relay/keys/admission.ed25519`. Same engine as team mode. Default admission target = current branch.
- **Team mode.** Postgres + Redis + shared intent-store, KMS-backed signing. Same binary, configuration-driven. Default admission target = `relay-main`.
- **Company mode** (multi-tenant) deferred to Phase 4.
- Cross-platform support in Phase 2A: macOS (Intel + ARM) and Linux (x86_64 + ARM64) only. Windows = WSL. Goreleaser pipeline + Homebrew tap + Developer-ID-notarized macOS binaries.

**Engine surfaces (design.md §13) — same engine, three transports, single daemon on laptop:**
- **`relay daemon`** — long-running local daemon owns SQLite, the local Grove client, the intent-store git repo, and the signer key. MCP stdio shims, CLI invocations, and the git pre-push hook all proxy to it over a Unix domain socket.
- **MCP server** (primary agent surface) — stdio shims (laptop) proxy to the daemon; HTTP+SSE (team mode). Tools: `relay_check`, `relay_certify`, `relay_submit`, `relay_policy`, `relay_explain`, plus the Auto-Intent Capture tools `relay_intent_open`, `relay_intent_close`, `relay_intent_update`, `relay_intent_list`.
- **CLI** — identical engine, terminal surface; proxies to the daemon on laptop.
- **Git pre-push hook** — backstop for unmanaged agents and humans; proxies to the daemon on laptop.
- **MCP client auto-registration**: `relay mcp install-for {claude-code,cursor,continue,windsurf}` writes each client's MCP config idempotently.

**Configuration in the repo (design.md §12.6) — foundational invariant:**
- Per-repo config in `.relay/` (entry point `relay.yaml`, plus `policies/`, `rulesets/`, `intents/`, `templates/`).
- Per-repo mutable state in `.relay/.cache/` (SQLite, indexer caches, intent drafts) — gitignored by the `.relay/.gitignore` written by `relay init`.
- Config layering: built-in defaults ⨁ platform-config org baseline (enterprise only, can lock fields) ⨁ `.relay/` ⨁ `~/.relay/config.yaml` (credentials only).
- Discovery walks upward from cwd to nearest `.relay/relay.yaml`. Monorepos supported via nested `.relay/`.
- `relay_version` pin in `relay.yaml` — stale binary fails fast instead of mis-evaluating.
- Effective config hash recorded in every certificate (`Effective-Config-Hash`) — audit is byte-reproducible.
- `relay init --stack=<go-microservice|node-api|python-service|java-spring>` scaffolds `.relay/` with sensible defaults and writes `.relay/.gitignore`.
- Imported SonarQube profiles land in `.relay/rulesets/`, not platform-config. The profile XML itself is committed (travels with the repo). The engine that actually evaluates it (SonarLint Core + JRE) is fetched lazily via Phase 2B's `relay tools install --with-sonar`.

**Auto-Intent Capture (new in Phase 2A) — the prompt is the intent:**
- 4 MCP tools: `relay_intent_open`, `relay_intent_close`, `relay_intent_update`, `relay_intent_list`.
- The agent's shipped system-prompt fragment instructs: "Before making code changes, call `relay_intent_open` with the user's request as title and description."
- `relay_intent_open` drafts an intent YAML at `.relay/.cache/intents/INT-{id}.draft.yaml` (gitignored). Includes `originated_from: {agent, model, conversation_ts}`.
- `relay_intent_close` (or `relay certify`) promotes the draft to `.relay/intents/INT-{id}.yaml` (committed).
- Commit trailer carries `Intent-ID: INT-{id}`. Server-side admission reads the intent file and cross-validates the diff against `allowed_paths` and `acceptance_criteria`.

**Core pipeline:**
- Ingest API + `relay` CLI + MCP server (all proxy to `relay daemon` on laptop).
- Intent + ChangeSet schema + storage (SQLite or Postgres; same schema, same migrations).
- ICR computation against Grove (`/impact`, `/deps`, `/tests`, `/symbols-at`) with confidence scoring.
- Policy engine with six gates (`design.md` §10.2):
  1. Path policy (allowed/forbidden) — Deny
  2. Secret scanner (gitleaks-style patterns) — Deny
  3. Special file class (migration / lockfile / generated / config) — varies
  4. Dependency change — ReviewRequired on team, Warn on laptop
  5. Size limit — Deny (team hard) / Warn (laptop and team soft)
  6. Test coverage of changed symbols — **Warn by default on laptop, Enforce by default on team.** Configurable via `.relay/policies/coverage.yaml` mode.
- Redis ICR locks with fencing tokens (team mode only; laptop mode's daemon is the single writer).
- **Certification cost contract** — split into two surfaces of the same engine:
  - `relay_check` (in-loop, pre-flight): changed-files SAST + Grove-affected unit tests only, sub-10-second target on typical change. The agent's iteration loop calls this.
  - `relay_certify` (commit-ready): full Stage 1 (build + full unit/integration suite, selective opt-in per project) + full Stage 2 (standalone static analysis: semgrep, gitleaks, govulncheck/npm audit, language linters). Stage 3 deferred.
- Test runners for Go, Python, TypeScript, Java emitting normalized JUnit XML (`design.md` §7.4).
- Coverage-of-changed-symbols computation against Grove `tests` edges.
- Admission controller: rebase + (Fuse on conflict) + re-cert slice + signed commit. **Default admission target = current branch on laptop, `relay-main` on team** (configurable in `.relay/relay.yaml`).
- Outbox reconciler → `intent-store` git repo (local at `~/.relay/intent-store/` on laptop, shared in team).
- Certificate signing via KMS (team mode) or local Ed25519 keypair at `~/.relay/keys/admission.ed25519` mode 0600 (laptop mode). `relay keys export` / `relay keys import` for portability across laptops. Same certificate format in both modes.
- `infrastructure_error` finding class in `relay.findings/v1` schema — Grove down, tool missing, `.relay/` absent, etc. — agent system prompt instructs to surface (not auto-fix).
- Background cold-start indexing: Grove indexes in background on `relay init`; `relay_check` returns partial findings with "indexing N% complete" until warm.
- Privacy / telemetry posture: laptop mode does not phone home by default; OpenTelemetry traces default off; documented in `getting-started.md`.

**Batteries-included tooling — lean default (`relay tools install`):**
- semgrep + bundled rulesets (security-audit, owasp-top-ten, language-specific packs).
- gitleaks.
- govulncheck, npm audit, pip-audit.
- golangci-lint, eslint, ruff, checkstyle, pmd.
- Tool-set manifest pinned per Relay release; deviations from the manifest are recorded in the certificate trailer as `Toolchain-Image-Drift: true`.
- No JRE in the lean default — ~50MB compressed total.
- SonarQube quality profile **importer surface** (`relay import sonarqube-profile`) lands in Phase 2A: parses the XML, writes `.relay/rulesets/<name>.xml` (committed), registers it in `.relay/relay.yaml`. **The engine that evaluates the profile** (SonarLint Core + Eclipse Temurin JRE + analyzer JARs, fetched via `relay tools install --with-sonar`) ships in **Phase 2B**. Rationale: [`sonarqube-no-server-investigation.md`](sonarqube-no-server-investigation.md).
- Default policy templates (`relay init --stack=go-microservice` / `node-api` / `python-service` / `java-spring`).

**Operator UX:**
- Minimal dashboard (team mode): intent list, intent detail, certificate view, coverage report.
- Bootstrapping: `relay init` (laptop; scaffolds `.relay/` + `.relay/.gitignore`, generates local Ed25519 key, starts daemon); `relay tenant init` / `relay repo add` / `relay project add` (team).
- MCP client auto-registration: `relay mcp install-for {claude-code,cursor,continue,windsurf}` writes the client config idempotently.
- Key lifecycle: `relay keys gen` (called by `relay init`), `relay keys export`, `relay keys import`, `relay keys fingerprint`. Public-key TOFU on laptop (no central registry); team-mode key registry replaces TOFU.
- State migration: `relay migrate sqlite-to-postgres` for laptop → team upgrade with no audit-history loss. `.relay/` is unchanged across upgrade.

### Scope (out, by policy)
- Parallel decomposition, intent groups, canary, multi-provider routing, K8s agent pods, multi-tenancy UX, SSO.
- SonarLint Core engine integration + JRE bundling + `relay tools install --with-sonar` runtime path — **deferred to Phase 2B** (the importer surface lands in 2A; the engine that evaluates imported profiles lands in 2B).
- Windows native support — Phase 2A targets macOS Intel/ARM and Linux x86_64/ARM64; Windows runs through WSL.

### Deliverables
1. Single binary `cmd/relay` running in laptop or team mode (configuration-driven, no separate builds). Goreleaser config + Homebrew tap + Developer-ID-notarized macOS binaries + Linux tarballs for x86_64 and ARM64.
2. Go packages:
   - `internal/engine/` — orchestrator, policy, icr, lock, cert, admission, outbox, signer (shared engine across surfaces).
   - `internal/daemon/` — long-running laptop daemon owning SQLite, Grove client, intent-store git, signer; Unix-socket RPC.
   - `internal/api/{mcp,http}/` — MCP server + REST API transport bindings.
   - `internal/cli/` — CLI transport binding (proxies to daemon on laptop).
   - `internal/githook/` — pre-push hook handler (proxies to daemon on laptop).
   - `internal/intent/` — Auto-Intent Capture (draft → promote, YAML schema, originated_from metadata).
   - `internal/store/{sqlite,postgres}/` — storage adapters with identical schema.
3. Storage migrations under `internal/store/migrations/` (SQLite-compatible SQL works against Postgres).
4. `platform-config/policies/<tenant>.yaml` example including full Stage 2 quality config + policy templates per stack.
5. OpenAPI spec at `api/openapi.yaml` + MCP tool schemas at `api/mcp/` covering all 9 tools (5 core + 4 Auto-Intent Capture).
6. Integration test harness: fake agent → MCP `relay_check` → real Grove → SQLite or Postgres → assert signed commit + valid certificate. Includes intent-round-trip test (open → diff → close → commit trailer linkage).
7. Docs: `getting-started.md` (laptop quickstart with telemetry-off statement), `team-deployment.md`, `mcp-integration.md`, `operations.md`, `agent-prompt.md`.
8. SonarQube profile importer **surface only**: `relay import sonarqube-profile <profile.xml>` writes `.relay/rulesets/<name>.xml` and registers it in `.relay/relay.yaml`. Engine integration deferred to Phase 2B per [`sonarqube-no-server-investigation.md`](sonarqube-no-server-investigation.md). No SQ-key → semgrep-rule mapping (lossy approach abandoned — the importer writes the SonarLint-native XML for the Phase 2B engine to consume directly).
9. Tool bundler: `relay tools install` downloads pinned versions of semgrep, gitleaks, govulncheck, golangci-lint, eslint, ruff to `~/.relay/tools/<version>/`. Tool-set manifest pinned per Relay release. Drift recorded in cert trailer. Build-time embedding for the install tarball.
10. Recommended agent system-prompt fragment (`docs/agent-prompt.md`) describing the `relay_check` in-loop pattern AND the `relay_intent_open` / `relay_intent_close` capture pattern.
11. **Config loader (`internal/config/`)**: discovery (upward walk from cwd), layered merge (defaults ⨁ platform-config ⨁ `.relay/` ⨁ user/host), lock enforcement, version range check against `relay_version` pin, canonicalization + sha256 hash for the certificate's `Effective-Config-Hash` field. Writes `.relay/.gitignore` on `relay init`.
12. **Stack templates (`templates/stacks/`)**: scaffold `.relay/` for `go-microservice`, `node-api`, `python-service`, `java-spring`. Each template includes sensible defaults for gates, runners, rulesets, a starter `relay.yaml`, and a `.gitignore` covering `.relay/.cache/`.
13. **`.relay/` schema spec** (`docs/relay-config-schema.md`): formal schema for `relay.yaml` + each policy file + the Intent YAML v2 + intent draft format. Used by editor tooling (JSON Schema export) and config validators.
14. **`relay daemon`**: long-running local daemon. Auto-started on first MCP/CLI/hook invocation on laptop; persists as user-level service (launchd on macOS, systemd --user on Linux). `relay daemon status`, `relay daemon stop`, `relay daemon restart` for operator control.
15. **MCP client auto-registration shims**: `relay mcp install-for claude-code|cursor|continue|windsurf` — idempotent, reversible, detects already-installed.
16. **Key management CLI**: `relay keys gen|export|import|fingerprint`. `relay init` generates the local Ed25519 key automatically if absent.
17. **Auto-Intent Capture**: 4 MCP tools (`relay_intent_open|close|update|list`), Intent YAML schema v2, draft → promote lifecycle, `.relay/intents/` and `.relay/.cache/intents/` directory conventions, commit-trailer linkage.

### Exit criteria
- Solo-dev scenario (proposal §7.1) runs unattended: 100% certified-commit success on a curated 50-intent corpus across Go + TypeScript repos.
- 10-dev team scenario (§7.2): 3 concurrent non-overlapping ChangeSets admitted without conflict; 1 overlapping queued and admitted post-rebase.
- p95 `relay_certify` ingest→admission latency ≤ 60 s on a Grove-warm repo for an intent touching ≤ 200 LOC.
- **p95 `relay_check` latency ≤ 10 s on a Grove-warm repo for a typical iteration (≤ 20 changed files, ≤ 200 LOC).** Validates the in-loop cost contract.
- All Stage 1+2 failures produce actionable diagnostics in the dashboard, CLI, and structured MCP findings.
- Certificate verifies end-to-end via `relay cert verify`.
- Outbox lag < 5 s p95; zero divergence between Postgres and intent-store on chaos test (Redis kill, worker kill, Postgres restart).
- **Config portability test:** the same `.relay/` produces byte-identical `Effective-Config-Hash` when evaluated by Relay running in laptop mode, team mode, and a simulated company-mode merge (with platform-config baseline applied). Required to prove "same Relay everywhere."
- **Config discovery test:** `relay check` invoked from a sub-directory finds the repo's `.relay/relay.yaml` via upward walk; nested `.relay/` in a monorepo sub-package overrides specific gates as expected.
- **Lock enforcement test:** platform-config baseline marks a field locked; a per-repo `.relay/` that tries to override it causes the config loader to fail with a clear error pointing at the offending file and field.
- **`relay init --stack=...` test:** scaffolds a working `.relay/` (including `.relay/.gitignore`, generated Ed25519 key, and started daemon) for each supported stack; subsequent `relay check` against a clean checkout passes with stack defaults.
- **Daemon concurrent-client test:** Claude Code + Cursor + a CLI invocation + the pre-push hook all simultaneously calling `relay_check` against the same repo on a single laptop produce consistent SQLite state, no intent-store git lock contention, and zero double-write of the same intent.
- **MCP auto-registration test:** `relay mcp install-for claude-code` adds the entry to the Claude Code config; running it twice is a no-op; `--uninstall` cleanly reverses it.
- **Intent capture round-trip test:** agent calls `relay_intent_open` with a sample user prompt → writes draft at `.relay/.cache/intents/`; agent edits code; agent calls `relay_intent_close`; promotion creates `.relay/intents/INT-{id}.yaml`; subsequent `relay certify` produces a commit whose trailer carries `Intent-ID: INT-{id}` pointing to the file; reading the file at the commit SHA returns the original prompt verbatim.
- **Key lifecycle test:** `relay init` generates `~/.relay/keys/admission.ed25519` mode 0600; `relay keys export` produces a portable bundle; `relay keys import` on a second machine yields the same fingerprint; certs signed before export verify against the imported key.
- **Telemetry-off-by-default test:** stock laptop install, network monitor running — `relay init` + `relay check` + `relay certify` produce zero outbound network traffic except to localhost (Grove) and to the user's git remote on push.
- **Default-warn-coverage test:** on a fresh laptop install in a project without `tests`-edge coverage, `relay_check` emits coverage findings as `severity: warning` and the cert succeeds; setting `.relay/policies/coverage.yaml` to `mode: enforce` flips the same finding to `severity: error` and the cert fails.
- **Infrastructure-error class test:** kill the Grove subprocess mid-`relay_check`; the tool returns a finding with `class: infrastructure_error`, `cause: grove_unreachable`, `is_actionable: false`. Agent system-prompt evaluation confirms the agent surfaces the error to the user rather than attempting an auto-fix.
- **Default admission target test:** `relay certify` on a fresh laptop checkout on branch `feature/x` commits to `feature/x` (not `relay-main`); the same command in team mode commits to `relay-main`.
- **SonarQube importer (surface) test:** `relay import sonarqube-profile <profile.xml>` writes `.relay/rulesets/<name>.xml`, updates `.relay/relay.yaml`, and prints `Run 'relay tools install --with-sonar' to enable.` Engine evaluation of the imported profile is **not** tested in Phase 2A (Phase 2B).

### Dependencies
- Grove ≥ v0.1 with `/impact`, `/deps`, `/tests`, `/symbols-at`, `/index-delta`.
- Fuse ≥ v0.1 driver invocable from admission.
- Prism not required (only used by agent pods in Phase 2B).

### Risks
- Grove `/symbols-at` accuracy on diff hunks → mitigate with fallback to file-level when symbol resolution confidence < 0.4.
- Coverage gate false positives (Grove missed a `tests` edge) → annotation escape hatch `// relay:no-test-required <reason>` and metric `coverage_gate_overrides` to spot misuse.
- Selective test strategy regression (only relevant if opt-in is enabled) → mandatory nightly full-suite guard + freeze admission on drift detection.
- Fencing-token race on push → covered by atomic `git push --atomic` plus pre-receive verifier.
- Test runner heterogeneity → ship Go runner first, then Python; add TypeScript/Java only when a real project requires them. Don't speculatively build adapters.

### Suggested work breakdown (no time estimates)
1. **Foundations**: storage adapter interface (SQLite + Postgres implementations behind one interface), migrations, signer (local Ed25519 + KMS backends), `core` types (`Intent`, `ChangeSet`, `ICR`, `TestRun`, `Certificate`, `PolicyResult`).
2. **Config loader (`internal/config/`)** — early dependency for every downstream step. Implements: `.relay/` upward discovery from cwd, layered merge (defaults ⨁ platform-config ⨁ `.relay/` ⨁ user/host), lock enforcement at merge time, `relay_version` range check, canonicalization + sha256 hash. Exposes `Engine.LoadConfig(cwd)` and emits the `Effective-Config-Hash` consumed by the certificate. Includes JSON Schema export for editor tooling. Stack templates (`templates/stacks/go-microservice`, `node-api`, `python-service`, `java-spring`) scaffolded by `relay init`. Writes `.relay/.gitignore` covering `.relay/.cache/` on init.
2a. **Laptop daemon (`internal/daemon/`)** — long-running Unix-socket RPC daemon owning SQLite, the local Grove client, the intent-store git repo, and the signer key. Started by `relay init`, by first MCP/CLI/hook invocation, or manually via `relay daemon start`. Persists as a user-level service (launchd on macOS, systemd --user on Linux) on first start. Health endpoint, `relay daemon status|stop|restart` CLI. Single-writer guarantee for SQLite and intent-store git.
2b. **Key management** — `relay keys gen|export|import|fingerprint` CLI. `relay init` invokes `relay keys gen` if `~/.relay/keys/admission.ed25519` does not exist. Prints fingerprint on every cert and on `keys gen`. Mode 0600. Export is a small tarball encrypted with a user-supplied passphrase.
3. **Engine package**: `internal/engine/` exposes a single `Engine.Check()` / `Engine.Certify()` / `Engine.Submit()` API that the MCP, CLI, and git-hook surfaces all call. Surface code does transport, not logic. Depends on the config loader (step 2).
4. **Ingest**: HTTP API + CLI `relay submit` + Intent/ChangeSet persistence.
5. **ICR**: Grove client wiring, ICR computation, confidence scoring, hash canonicalization.
6. **Policy engine + 5 pre-cert gates** (`internal/policy/{path,secrets,fileclass,deps,size}`). Path gate ships first as the simplest; others follow. Each gate reads its config from `.relay/policies/<gate>.yaml` via the config loader.
7. **Lock manager** (team mode only): Redis SET NX EX + fencing-token sequence + lease renewal + storage mirror.
8. **Certification — Stage 1**: build + test runner (Go first), JUnit XML parsing, TestRun persistence.
9. **Coverage gate** (`internal/policy/coverage`): runs after Stage 1 tests pass; consumes TestRun + Grove `tests` edges.
10. **Certification — Stage 2**: standalone static analysis suite — SAST (semgrep with configurable rulesets), secrets (gitleaks), deps (govulncheck/npm audit/pip-audit), quality linters (golangci-lint, eslint, ruff, checkstyle/pmd). Relay aggregates findings and owns the quality gate decision. **SonarQube profile importer surface only** in Phase 2A: `relay import sonarqube-profile <profile.xml>` writes the profile XML to `.relay/rulesets/<name>.xml` (committed; travels with the repo), updates `.relay/relay.yaml` to reference it, and prints a "Run `relay tools install --with-sonar` to enable" hint. **The engine that actually evaluates the profile (SonarLint Core + Eclipse Temurin JRE + `relay-sonar.jar` wrapper + analyzer JARs) is Phase 2B work** per [`sonarqube-no-server-investigation.md`](sonarqube-no-server-investigation.md). The Phase 2A importer does **not** attempt the lossy SQ-key → semgrep-rule-ID mapping that earlier drafts proposed; the XML is preserved verbatim for the 2B engine.
11. **MCP server**: `internal/api/mcp/` exposing the 5 core tools (`relay_check`, `relay_certify`, `relay_submit`, `relay_policy`, `relay_explain`) **plus the 4 Auto-Intent Capture tools** (`relay_intent_open`, `relay_intent_close`, `relay_intent_update`, `relay_intent_list`). On laptop, the MCP server is a thin stdio shim that proxies to `relay daemon` over a Unix socket. HTTP+SSE transport for team mode. Tool schemas in `api/mcp/`. `relay_policy` returns the effective merged config plus its `Effective-Config-Hash`. `relay_check` returns findings in `relay.findings/v1` including the `infrastructure_error` class. `relay_check` cost contract: changed-files SAST + Grove-affected unit tests only, sub-10-second target.
11a. **MCP client auto-registration**: `relay mcp install-for {claude-code,cursor,continue,windsurf}` shims that write each client's MCP config file idempotently. Detect already-installed; `--uninstall` reverses.
11b. **Auto-Intent Capture (`internal/intent/`)**: draft → promote lifecycle. `relay_intent_open` writes `.relay/.cache/intents/INT-{id}.draft.yaml` (gitignored) with `originated_from: {agent, model, conversation_ts}` populated from MCP request metadata. `relay_intent_close` (or `relay certify` if the agent didn't close explicitly) promotes the draft to `.relay/intents/INT-{id}.yaml` (committed). Commit trailer `Intent-ID:` links back to the file. Server-side admission validates the diff against the intent's `allowed_paths` and `acceptance_criteria`.
12. **Git pre-push hook**: `internal/githook/` + `relay hook install` CLI. Same engine entry points as MCP; on laptop proxies to the daemon.
13. **Admission**: rebase + Fuse glue on conflict + re-cert slice + signed commit + atomic push. **Default admission target = current branch on laptop, `relay-main` on team.** Configurable in `.relay/relay.yaml`. Certificate trailers include `Intent-ID`, `Repo-Config-SHA`, `Effective-Config-Hash`, `Sonar-Engine` (or `none` in Phase 2A), and `Toolchain-Image-Drift` (boolean).
14. **Outbox reconciler** + intent-store git layout (local in laptop, shared in team). Outbox payload includes the effective config hash so audit replay can recover the rules in force at admission time.
15. **Tool bundler**: `relay tools install` fetches the lean default set (semgrep, gitleaks, govulncheck, golangci-lint, eslint, ruff) at versions from the tool-set manifest pinned in the Relay binary release. Stores under `~/.relay/tools/<version>/`. Build-time tarball embedding for offline install. Deviations from the manifest set `Toolchain-Image-Drift: true` in the cert. **Phase 2B adds `--with-sonar`** which fetches Eclipse Temurin JRE 21 + SonarLint Core + analyzer JARs for declared languages + the `relay-sonar.jar` wrapper.
16. **Dashboard**: intent list, intent detail (with coverage + blast radius views), certificate view; audit read API. Shows the `.relay/` config diff between two certificates when comparing intents.
17. **State migration**: `relay migrate sqlite-to-postgres` (laptop → team upgrade). Does NOT migrate `.relay/` — that already lives in the repo and is unchanged across the upgrade.
18. **Chaos + integration test suite** (worker kill, Redis kill, Postgres restart, git push race, MCP client disconnect mid-run). Includes a "config drift" test: same diff submitted against two different `.relay/` commits must produce two different `Effective-Config-Hash` values.

### Signature capabilities included in Phase 2A

These are the developer/auditor-facing features that make Relay memorable; specified in [design.md §16](design.md#16-signature-capabilities--data-contracts).

19. **Pre-Flight Autopilot.** Recommended agent system-prompt fragment (`docs/agent-prompt.md`) + tight `relay_check` findings schema (`relay.findings/v1`) including `next_action` and the `infrastructure_error` class. Validated end-to-end with a real Claude Code / Cursor / Continue session in the integration suite.
20. **AI Code Passport.** `relay cert show <ref>` CLI command, dashboard card, PR/MR bot comment (GitHub + GitLab), JSON-LD export. No new tables — reads from existing `certificates` row plus the `.relay/intents/INT-*.yaml` file referenced by `Intent-ID`.
21. **Diff Risk Heatmap.** Per-symbol risk score computed at admission time from ICR + Grove `/deps` + coverage delta + boundary tags + historical defect density. Stored under `certificates.payload.risk_heatmap` with a versioned `risk_model_version`. Rendered in the Passport and bot comment.
22. **Evidence Replay (foundational).** `relay cert replay <cert-id>` runs against any certificate produced by the same major version. Verdict values: `byte_reproducible`, `tool_drift`, `config_drift`, `unrecoverable`. Required in Phase 2A so the audit story is real on day one.
23. **Policy Marketplace (bootstrap).** Community profile bundles in a sibling repo (`grove-suite/relay-profiles`). `relay init --profile=<name>` fetches, verifies signature, lays into `.relay/`, records pin in `relay.yaml`. Phase 2A ships with 4 stack profiles + 2 compliance profiles (`soc2-baseline`, `eu-ai-act-article-12`).
24. **Auto-Intent Capture.** The user's prompt is the intent — turned into a YAML committed alongside the code. Specified above in deliverables §17 and work-breakdown §11b. This is the most consequential new capability falling out of the laptop-MVP audit: the conversation that produced the code becomes a first-class, PR-reviewable artifact rather than evaporating with the agent session. Cross-validated server-side; auditable forever.

### Signature capabilities deferred to Phase 2B

- Surgical Revert by Intent (`relay revert --intent <id>`) — requires the symbol-graph linkage to ICR to be production-validated first.
- Human Review Budget Optimizer — depends on heatmap data + a few months of reviewer-decision telemetry to calibrate the recommendation classes.
- Agent Scorecard (full version with post-admission defect rate) — depends on incident-tracker integration shipped in Phase 2B.

### Engine work deferred to Phase 2B

- **SonarLint Core runtime integration**: `relay tools install --with-sonar` fetches Eclipse Temurin JRE 21, SonarLint Core, analyzer JARs for declared languages, and the `relay-sonar.jar` wrapper (a thin Java module around `StandaloneSonarLintEngine` released by the same Goreleaser pipeline). At analyze time, Relay's certification engine prefers SonarLint over semgrep for languages and rules where the imported profile applies. Cert trailer records `Sonar-Engine: sonarlint-core@X.Y`. Limits documented (no taint analysis / COBOL / Apex / PL/SQL / T-SQL — those require commercial SonarQube Server + connected mode).

### Signature capabilities explicitly rejected

| Proposal | Rejection reason |
|----------|------------------|
| Standalone "Diff Comprehension UI" | Owning a proprietary review surface conflicts with the architecture's principle of meeting developers in GitHub/GitLab. Risk Heatmap delivers the value without the surface. |
| Self-Healing Sandbox (auto-fix agent on cert failure, outside the agent loop) | Creates a new attack surface and trains teams that Relay "always passes." Pre-Flight Autopilot is the supported version of this idea. |
| Relay-native code-review chat | Out of scope. Relay is admission + evidence, not a conversation product. |
| Multi-provider model routing in 2B | Premature without cost + quality data; Claude-only stands. |

---

## Phase 2B — Self-Hosted Agent Execution + SonarLint Engine

### Goal
Relay becomes a self-hosted alternative to Cursor Background Agents / Devin for teams that need governance-aware agent execution inside their perimeter, AND gains true SonarQube-rule fidelity locally via the SonarLint Core engine integration deferred from Phase 2A.

### Scope (in)
- K8s operator + CRDs: `IntentRun`, `AgentImage`.
- Ephemeral pod runtime bundling Claude Code SDK + Grove client + Prism client + git + language toolchains.
- Pod lifecycle: pre-warmed pool, sidecar for cost tracking and heartbeats, image cache on nodes.
- Prism integration inside pod for context delivery.
- Agent Decision Record (ADR) capture in every ChangeSet (schema in `design.md` §4).
- Ambiguity policy enforcement (`fail_with_questions` vs `proceed_with_default`).
- Per-intent budget enforcement (cost $, wall-clock).
- Bidirectional webhooks: Jira/GitHub issue → intent; intent terminal → comment.
- **SonarLint Core engine integration** (per [`sonarqube-no-server-investigation.md`](sonarqube-no-server-investigation.md)):
  - `relay-sonar.jar` — thin Java wrapper around `StandaloneSonarLintEngine` (sibling repo `grove-suite/relay-sonar`, LGPL-3.0, released by the same Goreleaser pipeline as Relay).
  - `relay tools install --with-sonar` — fetches Eclipse Temurin JRE 21 (~50MB compressed per platform), SonarLint Core + analyzer JARs for declared languages (~10–60MB per language), and the wrapper. Stored under `~/.relay/tools/sonar/<version>/`.
  - Implicit trigger: `relay import sonarqube-profile <profile.xml>` (Phase 2A surface) now auto-suggests `--with-sonar` if absent.
  - Runtime path: certification engine spawns `java -jar relay-sonar.jar --rules <profile.xml> --files <list> --out findings.json`, parses output, aggregates with semgrep, de-duplicates overlapping rules.
  - Certificate trailer: `Sonar-Engine: sonarlint-core@<version>` or `none`.
  - Documented limitations: no taint analysis / COBOL / Apex / PL/SQL / T-SQL (those require commercial SonarQube Server + connected mode).

### Deliverables
1. `cmd/relay-operator` Helm chart.
2. Container image `relay/agent-runtime:<lang-bundle>` (one per supported language group).
3. `internal/runner/k8s` package.
4. ADR schema v1 enforced at ingest.
5. Cost telemetry pipeline (OpenTelemetry → Postgres `intent_costs`).
6. Docs: `agent-runtime.md`, `webhooks.md`.
7. **Surgical Revert (`relay revert --intent <id>`)** — uses stored ICR + ChangeSet to synthesize symbol-scoped inverse; runs through standard admission; carries `Reverts:` trailer. Fails loudly (never widens scope silently) when the symbols have been further modified.
8. **Human Review Budget Optimizer** — PR check that recommends `skim` / `standard` / `senior` / `two-person` based on Risk Heatmap + cert confidence + boundary flags + a reviewer-policy file in `.relay/`. Tunable per project.
9. **Agent Scorecard (full)** — adds post-admission defect rate by joining certs against incident-tracker tags. Initial integrations: GitHub Issues, Jira, PagerDuty post-mortem links. Prometheus exporter + `relay scorecard --format csv`.
10. **Policy Marketplace (community)** — open contribution flow on `grove-suite/relay-profiles`: PR + signature verification + CI runs profile against a fixture corpus. Phase 2B target: 12 stack profiles + 5 compliance profiles.

### Exit criteria
- A real intent (`add rate limiting`) executed end-to-end inside the pod, producing a ChangeSet that passes Phase 2A admission with no human edits.
- Dead-agent recovery: kill a pod mid-run; intent re-enqueues; second attempt completes; no double-commit.
- Per-intent cost report shows token usage within ±5% of provider invoice for a 100-intent sample.
- Ambiguity policy: 100% of intents with unmet acceptance criteria either ask a question (annotated on the intent) or fail explicitly when policy = `fail_with_questions`.

### Dependencies
- Phase 2A admission contract frozen.
- Prism ≥ stable `prism_query` / `prism_read`.
- A K8s cluster (≥ 1.28); KMS access from pods.

### Risks
- Pod cold-start latency → pre-warmed pool + node-level image cache.
- Agent loops burning budget → hard wall-clock cap + per-stage cost ceiling.
- Secrets in pod → workload identity, no static creds; intent-scoped tokens.

---

## Phase 3 — Parallel Decomposition, Intent Groups, Branchless

### Goal
Unlock multi-intent parallel execution and atomic group admission, gated on empirical data collected during Phases 2A/2B.

### Gate (do not start until met)
- ≥ 6 months of production ICR data from Phase 2A+2B deployments.
- Empirical confusion matrix for ICR confidence vs actual conflict: precision ≥ 0.95 at confidence ≥ 0.85.
- A calibration model (logistic regression or gradient-boosted) trained on the data.

### Scope (in)
- `Decomposer` interface implementation: LLM-driven decomposition with GS and ICR pre-check; produces child intents linked by `parent_id`.
- Intent groups: atomic admission of N related intents using a multi-parent merge commit; group rolls back as one unit if any child fails.
- Calibrated confidence model replaces the heuristic in §5 of design.
- Optional `CanaryGate` interceptor (feature-flagged); first integration with Argo Rollouts / Flagger.
- Branchless trunk marketing repositioned: "branches unnecessary for approved classes" → backed by data.

### Deliverables
1. `internal/decomposer` package + Decomposer model prompt + eval suite.
2. `internal/groups` package; group state machine (`forming → executing → admitting | rejecting`).
3. Confidence model artifact + offline trainer + online evaluator.
4. CanaryGate adapter for Argo Rollouts.
5. Public dataset (sanitized) for ICR confusion matrix — basis for the white paper.

### Exit criteria
- Parallel admission of two non-overlapping child intents from a single decomposed parent, with end-to-end audit linkage.
- Group rollback test: 3-intent group where child 2 fails certification → none admitted, intent-store records the abort.
- Confidence model in production for ≥ 4 weeks with calibration drift < 5%.

### Dependencies
- Production telemetry pipeline (Phase 2B).
- A customer willing to opt into parallel execution.

### Risks
- Decomposer hallucinations create sub-intents that don't compose → require Decomposer output to pass GS ≥ threshold per child before lock attempt.
- Group admission deadlocks → topological lock ordering + deadlock detector in lock manager.

---

## Phase 4 — Enterprise Scale

### Goal
Run Relay at Fortune-50 scale: 10⁴–10⁵ repos, 10⁴–10⁵ developers, multi-region, multi-tenant, sovereign compliance. Mechanical extension of the wedge design; no fundamental rewrite.

### Gate
A signed Fortune-500 design partner with concrete deployment requirements. Do not begin without one.

### Sub-phases

#### 4A — Multi-tenancy primitives
- **Scope:** Tenant data model; RBAC roles (`viewer`, `intent-author`, `reviewer`, `admin`, `security-champion`); per-tenant Postgres schemas (or instances at high tier); per-tenant Redis clusters/namespaces; OIDC/SAML SSO; tenant-scoped CLI/API.
- **Deliverables:** `internal/tenant`, `internal/authz`, SSO adapters, tenant lifecycle CLI.
- **Exit:** 10 simulated tenants with strict isolation verified by red-team test.

#### 4B — Grove Federation
- **Scope:** Grove Router service; `repo → endpoint` registry; hot/cold tiering; object-storage index snapshots; Grove cluster operator.
- **Deliverables:** `cmd/grove-router`, snapshot tooling, cold-load benchmarks.
- **Exit:** 1,000-repo synthetic federation with p95 `/icr` ≤ 200 ms.

#### 4C — Intent-store sharding + Audit Aggregator
- **Scope:** Per-tenant intent-stores; monthly rollup tooling; federated read-only Audit Aggregator API; export pipelines (CSV, Parquet, S3).
- **Deliverables:** `cmd/audit-aggregator`, rollup cron, export adapters.
- **Exit:** Compliance query "all AI-generated commits to project X in Q2" returns in ≤ 30 s across 100 tenant stores.

#### 4D — Multi-region control plane
- **Scope:** Regional control planes (EU/US/APAC); data residency enforcement; cross-region federation for audit only; sovereign-deployment runbook.
- **Deliverables:** Helm umbrella chart, region routing, residency policy engine.
- **Exit:** Two-region deployment passes data residency conformance test; failover of one region without intent loss.

#### 4E — Cost optimization platform (ongoing)
- **Scope:** Model routing (Haiku/Sonnet/Opus by GS + ICR + risk); provider prompt-prefix caching; context-pack reuse; per-tenant budget enforcement; billing-grade metering and chargeback.
- **Deliverables:** `ModelRouter` implementation, caching layer, metering exporter (Prometheus + billing CSV).
- **Exit:** ≥ 60% reduction in LLM spend on a 10K-intent benchmark vs naive Sonnet-everywhere baseline.

#### 4F — SonarQube connected-mode sync (optional)
- **Scope:** For enterprises that maintain their canonical quality profile on a SonarQube Server or SonarCloud instance, Relay periodically syncs the profile via the `api/qualityprofiles/backup` HTTP endpoint and writes the result into `.relay/rulesets/` automatically. Profile-change auditing. SSO-aware credentialing.
- **Deliverables:** `internal/sonar/connected` package, sync cron, profile-diff dashboard widget.
- **Exit:** A SonarCloud-hosted profile change appears in the connected Relay's `.relay/rulesets/` within ≤ 5 minutes; the resulting `Effective-Config-Hash` change is reflected in subsequent certs.

### Cross-cutting compliance work
- SOC 2 Type II audit.
- ISO 27001 certification.
- FedRAMP Moderate readiness (if a US public-sector partner emerges).
- EU AI Act Article 12 (logging) and Article 14 (human oversight) attestation artifacts generated automatically from intent-store data.

### Risks
- Engineering effort without a design partner → enforce the gate.
- Vendor lock-in (KMS, K8s flavor) → abstract behind interfaces from Phase 4A.
- Cost overruns on agent runtime → Phase 4E is funded by the design partner's ACV, not by speculation.

---

## Cross-Phase: Always-On Workstreams

These run continuously alongside phased work.

| Workstream | Purpose |
|------------|---------|
| Security review | Every new endpoint, CRD, and policy gate gets a threat model entry. |
| Observability | OTel coverage ≥ 95% of state transitions; SLO dashboards published. |
| Open-source hygiene (Grove/Prism/Fuse) | Issue triage SLA, release cadence, contribution docs. Drives bottom-up adoption that funds top-down sales. |
| Compliance evidence collection | Generated automatically from intent-store; never hand-curated. |
| Performance regression suite | Phase 2A benchmark corpus runs on every PR; alerts on p95 latency or success-rate regression. |

---

## What This Plan Refuses to Promise

In line with proposal §6.3 and §13:

- No replacement of GitHub PR UI for human-authored code.
- No magic conflict resolution — Fuse is invoked only when rebase fails and is allowed to give up; rejection is a first-class outcome.
- No claim of "branchless main" until Phase 3 calibration data supports it.
- No multi-tenant or multi-region claim until Phase 4A is shipped and externally audited.
- No agent-vendor lock-in or feature parity with Cursor/Devin — Relay's value is downstream of any agent, not in competition with them.
