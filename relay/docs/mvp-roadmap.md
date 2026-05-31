# Relay MVP Roadmap (Laptop → Team → Phase 2B)

Operational plan for completing Phase 2A ("Certified Merge Wedge") as a sequence of shippable MVPs. Each MVP is **independently demoable** and **CI-green** before the next one starts. Phase numbering matches `ROADMAP.md`. Time estimates intentionally omitted — milestones gate on exit criteria.

---

## Laptop Track (single binary, SQLite, local Ed25519 signer, no Redis)

### MVP-L1 — Thin slice end-to-end
Goal: any agent submits a `ChangeSet` → 2 policy gates run → Grove computes ICR → admission produces a signed linear commit on `relay-main`.

Scope:
- `internal/core/`: add `ChangeSet`, `Certificate`, `PolicyResult`, `ICR`, `RelayConfig` types.
- `internal/config/`: `.relay/` upward discovery from cwd, minimal `relay.yaml` schema, `Effective-Config-Hash` (canonical JSON → sha256).
- `internal/sqlite/`: SQLite storage adapter for changesets + certificates (alongside existing Postgres adapter; same schema).
- `internal/grove/`: extend client with `Impact()`, `Deps()`, `ICR()`, `Symbols()`.
- `internal/policy/`: gate framework + `path` and `size` gates (the two simplest from design.md §10.2).
- `internal/signer/`: local Ed25519 keypair under `~/.relay/keys/` + verify.
- `internal/engine/`: `Engine.Check(cs)`, `Engine.Certify(cs)`, `Engine.Submit(cs)` (orchestrator).
- `internal/admission/`: rebase + signed commit on `relay-main` with full trailer set (Intent-ID, ICR-Hash, Certificate, Effective-Config-Hash, Signed-By).
- `internal/cli/`: `relay submit --diff <patch> --intent <id>`, `relay check`, `relay init`, `relay cert verify`.

Exit criteria:
- A scripted test: ingest 5 single-file Go ChangeSets sequentially → 5 signed commits on `relay-main` → each certificate verifies via `relay cert verify`.
- Same `.relay/` evaluated twice yields byte-identical `Effective-Config-Hash`.

Excludes: tests, linters, MCP, dashboard, outbox, Redis, K8s.

---

### MVP-L2 — Stage 1 (build + test) + coverage gate
Adds the gate that actually catches regressions.

Scope:
- `internal/runner/go/`: Go test runner → normalized JUnit XML.
- `internal/cert/stage1`: build + test orchestration; persists `TestRun`.
- `internal/policy/coverage`: coverage-of-changed-symbols vs Grove `tests` edges; supports `// relay:no-test-required` escape hatch.
- Engine wires Stage 1 between policy gates and admission.

Exit criteria:
- A ChangeSet with a deliberate test break is rejected with actionable diagnostics; same diff with passing test is admitted.

---

### MVP-L3 — Stage 2 (standalone static analysis)
The minimum security/quality bar developers expect.

Scope:
- `internal/cert/stage2`: aggregator that runs each analyzer in parallel and produces a single verdict.
- `internal/analyzers/`: `semgrep` (default OWASP ruleset), `gitleaks`, `govulncheck`.
- Add the remaining pre-cert gates: `secrets`, `fileclass`, `deps` (`internal/policy/{secrets,fileclass,deps}`).
- Findings emitted in `relay.findings/v1` schema (foundation for Pre-Flight Autopilot).

Exit criteria:
- Seeded secret in a fixture diff → secrets gate denies; verified `relay check` output includes `next_action`.
- Known-CVE dep bump → deps gate flags `ReviewRequired`.

---

### MVP-L4 — Batteries-included tooling
`relay tools install` + stack scaffolding + SonarQube parity.

Scope:
- `relay tools install`: fetch pinned binaries (semgrep, gitleaks, govulncheck, golangci-lint, eslint, ruff) into `~/.relay/tools/`. Build-time tarball embed for offline install.
- `relay init --stack=go-microservice|node-api|python-service|java-spring`: scaffolds `.relay/` from `templates/stacks/`.
- `relay import sonarqube-profile`: parse SQ XML → semgrep rule IDs via mapping table → write `.relay/rulesets/<profile>.yaml` + coverage-gap report.

Exit criteria:
- `relay init --stack=go-microservice && relay check` passes on a clean checkout.
- Importing a real-world SQ profile produces ≥ 70% rule coverage with explicit gaps listed.

---

### MVP-L5 — MCP surface + Pre-Flight Autopilot
The primary agent integration story.

Scope:
- `internal/api/mcp/` (stdio): tools `relay_check`, `relay_certify`, `relay_submit`, `relay_policy`, `relay_explain`. Same engine as CLI.
- `docs/agent-prompt.md`: the recommended system-prompt fragment.
- Integration test: real Claude Code session → `relay_check` returns findings → agent self-corrects → `relay_submit` succeeds.

Exit criteria:
- Round-trip: agent edits → `relay_check` → agent applies `next_action` → `relay_submit` admits in one session.

---

### MVP-L6 — Signature capabilities (Phase 2A set)
The features that make Relay memorable.

Scope:
- **AI Code Passport**: `relay cert show <ref>` CLI + JSON-LD export. PR bot comment (GitHub) reads same data.
- **Diff Risk Heatmap**: per-symbol score from ICR + `/deps` + coverage delta + boundary tags. Stored under `certificates.payload.risk_heatmap` with `risk_model_version`.
- **Evidence Replay (foundational)**: `relay cert replay <cert-id>` with verdicts `byte_reproducible` / `tool_drift` / `config_drift` / `unrecoverable`.
- **Policy Marketplace bootstrap**: `relay init --profile=<name>` from `grove-suite/relay-profiles`; ship 4 stack + 2 compliance profiles.

Exit criteria:
- A 100-cert corpus replays with ≥ 95% `byte_reproducible` verdict.

---

### MVP-L7 — Multi-language test runners + linters
Removes the "Go-only" caveat.

Scope:
- `internal/runner/{python,typescript,java}/` with JUnit-equivalent normalization.
- Linters: `eslint`, `ruff`, `pmd`, `checkstyle` wired through Stage 2.

Exit criteria:
- A repo per language passes the full pipeline end-to-end.

---

### MVP-L8 — Git pre-push hook + local outbox
Catches unmanaged agents and humans.

Scope:
- `internal/githook/` + `relay hook install` CLI.
- `internal/outbox/`: SQLite → local `intent-store` git repo (single-process file lock). Audit snapshots include `Effective-Config-Hash`.

Exit criteria:
- A push that would bypass MCP is intercepted; rejected push leaves no partial state.

---

**Laptop track end:** A solo developer can `relay init`, point Claude Code at the MCP server, and ship signed, certified commits without any external infrastructure.

---

## Team Track (Postgres + Redis + shared intent-store)

Same binary, configuration-driven. Each MVP below adds capability without breaking laptop mode.

### MVP-T1 — Storage adapter parity
- Promote `internal/sqlite/` and existing Postgres code behind a common `Store` interface.
- `relay migrate sqlite-to-postgres` with no audit-history loss.

### MVP-T2 — Distributed ICR locks
- `internal/lock/redis`: `SET NX EX` + fencing-token sequence + lease renewal.
- Concurrent ChangeSet test: 3 non-overlapping admitted in parallel; 1 overlapping queued.

### MVP-T3 — MCP HTTP+SSE + dashboard
- HTTP+SSE transport for MCP.
- Minimal team dashboard: intent list, certificate detail, coverage view, queue state. Reuses Phase 1 dashboard scaffolding.

### MVP-T4 — Shared outbox + KMS signer
- Outbox reconciler against shared `intent-store` (mutual exclusion via `git fetch --atomic` + advisory lock).
- KMS backend (AWS KMS first) behind same signer interface as local Ed25519.

### MVP-T5 — Chaos hardening
- Kill-worker / kill-Redis / restart-Postgres / git-push-race / MCP-disconnect-mid-run tests in CI.
- Config drift test: same diff against two `.relay/` commits produces two `Effective-Config-Hash` values.

**Team track end:** Phase 2A exit criteria from `docs/implementation-plan.md` satisfied (10-dev concurrent scenario, p95 ≤ 60 s, outbox lag < 5 s p95, zero divergence under chaos).

---

## Beyond Phase 2A

- **Phase 2B (self-hosted agent execution):** K8s operator + agent pods + Surgical Revert + Human Review Budget Optimizer + full Agent Scorecard + Policy Marketplace community flow.
- **Phase 3 (parallel decomposition):** gated on Phase 2A producing empirical ICR-accuracy data.
- **Phase 4 (enterprise):** gated on a paying Fortune-500 design partner.

---

## Cross-cutting invariants (apply to every MVP)

1. CI must stay green (cross-platform: Linux + macOS + Windows).
2. Coverage gates from existing CI policy must not regress.
3. `.relay/` config is portable: same config produces identical `Effective-Config-Hash` across laptop/team/company modes.
4. No new feature lands without an exit-criteria integration test.
5. Each MVP closes with a `docs/` update so the README always reflects shipped capability.
