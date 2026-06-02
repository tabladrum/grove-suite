# Provasign Roadmap

AGPL-3.0 licensed. Part of the Grove Suite. Requires Grove (`localhost:7777`) and Prism for context delivery.

---

## Phase 1 — Intent Intake ✅ shipped

Single-process service that ingests intents from Jira webhooks, GitHub Issues webhooks, and direct CLI, validates scope via two-stage granularity scoring, routes them to a registered project via B → C → A routing, and tracks their lifecycle in Postgres with bidirectional feedback to source systems.

- [x] Intent schema, state machine, event log
- [x] Two-stage granularity scoring (heuristic + Grove ICR symbol count)
- [x] Per-project GS threshold
- [x] Repo / Project / ProjectIntegration data model (M:M with external boards)
- [x] B → C → A project routing
- [x] Jira + GitHub webhook receivers with HMAC validation
- [x] Bot-comment feedback to source systems
- [x] Bearer-token authenticated HTTP API
- [x] HTML dashboard with triage queue
- [x] CLI: `provasign serve / repo / project / intent`

---

## Phase 2A — Certified Merge Wedge ✅ laptop track shipped

Single binary (laptop mode: SQLite + local Ed25519 key, no Redis) that exposes the certification engine via MCP, CLI, and git pre-push hook, and admits certified signed commits to a linear branch. Team mode (Postgres + Redis, KMS) and the remaining items below are next.

### Laptop track (MVP-L1 through L8) — all shipped

- [x] **MVP-L1** — Thin-slice end-to-end: ChangeSet → policy gates → Grove ICR → signed linear admission (`provasign-main`)
  - `provasign submit`, `provasign check`, `provasign init`, `provasign cert verify`
  - Ed25519 local keypair at `~/.provasign/keys/admission.ed25519`
  - CanonicalBytes excludes `admitted_commit_sha` (signed before commit exists)
- [x] **MVP-L2** — Stage 1 build + test + coverage gate
  - Go test runner → normalized JUnit XML; `git worktree` isolation
  - Coverage-of-changed-symbols gate; `// provasign:no-test-required` escape hatch
- [x] **MVP-L3** — Stage 2 standalone static analysis
  - Analyzers: inline-secrets (always available), gitleaks, semgrep, govulncheck
  - `provasign.findings/v1` schema; gates: secrets / fileclass / deps
- [x] **MVP-L4** — Batteries-included tooling
  - `provasign tools install` bundles semgrep, gitleaks, govulncheck, golangci-lint, eslint, ruff
  - Stack templates (`provasign init --stack=go-microservice|node-api|python-service|java-spring`)
  - `provasign import sonarqube-profile` — parse SQ XML into `.provasign/rulesets/`
- [x] **MVP-L5** — MCP stdio surface
  - Tools: `provasign_check`, `provasign_certify`, `provasign_submit`, `relay_policy`, `relay_explain`
  - Intent tools: `provasign_intent_open`, `relay_intent_update`, `relay_intent_close`, `relay_intent_list`
  - `provasign mcp serve`; `provasign mcp install-for {claude-code,cursor,codex,windsurf,continue,vscode,claude-desktop,zed,kiro}`
  - `provasign init` auto-writes Pre-Flight Autopilot steering instructions to CLAUDE.md, .cursorrules, .github/copilot-instructions.md, AGENTS.md, GEMINI.md, .clinerules, .kiro/steering/, .devin/, .amp/
  - `provasign init` auto-registers MCP for Claude Code (.mcp.json at project root), GitHub Copilot / VS Code (.vscode/mcp.json), Cursor (.cursor/mcp.json), Codex CLI (~/.codex/config.toml), plus Claude Desktop / Windsurf / Zed / Kiro / Continue when installed
  - System-prompt fragment at `docs/agent-prompt.md`
- [x] **MVP-L6** — Cert JSON-LD + replay + risk heatmap + profiles
  - `provasign cert show [--jsonld] <id>`, `provasign cert replay <id>`
  - Diff risk heatmap: ICR + stage2 severity + coverage delta + touch intensity
  - 6 built-in profiles: `soc2-baseline`, `pci-dss-baseline`, 4 stack-strict profiles
- [x] **MVP-L7** — Multi-language Stage 1 + eslint/ruff analyzers
  - `LangRunner` dispatcher: Go (gotest), Python (pytest), Node (vitest/jest/npm)
  - eslint severity 0/1/2 → info/medium/high; ruff E/F/S → high, W → medium
- [x] **MVP-L8** — Git pre-push hook + local outbox
  - `provasign hook install [--force]`, `provasign hook uninstall`
  - Outbox: O_EXCL lock, one JSON snapshot per cert, `certificates/<id>.json`
  - `provasign outbox push --intent-store=<path>`

### Phase 2A remaining (team mode + advanced features)

- [ ] One binary, three deployment modes (team: Postgres + Redis + KMS + shared intent-store)
- [ ] `provasign daemon` as single writer proxy on laptop (Unix socket; MCP stdio + CLI + hook all proxy to it)
- [x] Auto-Intent Capture: `provasign_intent_open` / `relay_intent_close` / `relay_intent_update` / `relay_intent_list`; draft-at-`.provasign/.cache/intents/` → promote-to-`.provasign/intents/` lifecycle
- [ ] Fast-slice `provasign_check`: changed-files SAST + Grove-affected unit tests only (sub-10 s)
- [ ] `infrastructure_error` finding class (Grove down, tool missing, `.provasign/` absent)
- [ ] SonarLint Core engine integration (Phase 2B) — importer surface shipped in MVP-L4
- [ ] Dashboard (team mode): cert details, ICR confidence, queue state, audit read API
- [ ] State migration: `provasign migrate sqlite-to-postgres`

---

## Phase 2B — Self-Hosted Agent Execution

Adds K8s operator + ephemeral pod model for teams that need governance-aware self-hosted execution.

- [ ] K8s operator + `AgentExecution` CRD
- [ ] Ephemeral pod: Grove index + Prism context delivery + Claude Code SDK + git clone
- [ ] Cost tracking sidecar (per-intent budget enforcement)
- [ ] Heartbeat + dead-agent recovery (fencing-token aware)
- [ ] Agent Decision Record attached to every ChangeSet
- [ ] SonarLint Core engine + Eclipse Temurin JRE 21 via `provasign tools install --with-sonar`
- [ ] Ambiguity policy enforcement: `fail_with_questions` vs `proceed_with_default`

---

## Phase 3 — Parallel Decomposition, Intent Groups, Branchless

**Start only after Phase 2 produces empirical ICR calibration data.**

- [ ] Grove union-find decomposition (`/icr` + `/deps` → connected components)
- [ ] Parallel agent execution per independent component
- [ ] Fuse semantic merge as a first-class step (not just rebase fallback)
- [ ] Intent groups (atomic admission across multiple sub-intents)
- [ ] Optional canary deployment integration
- [ ] Full intent-as-review-artifact UX (configurable review policy)

---

## Phase 4 — Enterprise Scale

Gated on a paying Fortune-500 design partner.

- [ ] **4A** — Multi-tenancy: tenant model, RBAC, per-tenant Postgres, SSO/OIDC
- [ ] **4B** — Grove Federation: Grove Router, hot/cold tiering, object-storage index snapshots
- [ ] **4C** — Intent-store sharding + Audit Aggregator
- [ ] **4D** — Multi-region control plane, data residency enforcement
- [ ] **4E** — Cost optimization: model routing, prompt/context caching, metering

---

## Non-Goals (current)

- Replacing GitHub as the development frontend
- Multi-provider agent routing in Phase 2 (Claude only; revisit with evidence)
- Owning the design / ADR conversation surface
- Infrastructure changes (Terraform, K8s manifests, production DB migrations) — code-only
- A standalone diff comprehension or interactive code-review UI
- A "self-healing sandbox" that auto-fixes failed certifications outside the agent loop
