---
title: FAQ
layout: default
nav_order: 11
description: "Frequently asked questions — technical, security, business, and audit perspectives."
permalink: /faq/
---

# Provasign FAQ

Top questions, grouped by who is asking them.

---

## General

### What is Provasign, in one sentence?

Provasign is certified delivery for AI coding agents: it captures the prompt, runs quality gates inside the agent loop, and issues an Ed25519-signed certificate binding prompt → gates → commit. It is built on three open-source command-line tools it embeds — Grove (code knowledge graph), Prism (token-optimized context), and Fuse (symbol-aware merge) — each usable on its own.

### Who built this and why?

A small team that was tired of watching AI coding agents — which are genuinely good at writing code — get bottlenecked by infrastructure that was designed for humans. PRs, line-based merge, post-hoc CI. We built Provasign as the replacement infrastructure: local-first, open-source, and designed for the volume and accountability requirements of agent-driven development.

### Is this production-ready?

Phase 1 is built and tested. We run it on our own work daily. The benchmarks in each product's README are real numbers on real hardware. That said:
- The open-source licenses are real: use at your own risk, no warranty, no SLA
- Phase 2 features (team mode with Postgres + Redis, agent execution platform) are on the roadmap, not shipped
- Edge cases in unusual codebases will exist — please file issues

### What licenses?

Grove, Prism, and Fuse are MIT licensed. Provasign is AGPL-3.0 licensed. The repository is at [github.com/provasign/provasign](https://github.com/provasign/provasign).

### Does it phone home?

No. There is zero telemetry. No analytics. No "improving our models with your usage." Provasign runs entirely on your machine. Grove is embedded in-process — there is no daemon, no port, and no socket between components. The only network calls are:
- Downloading pinned analyzer tools on first use (`provasign tools install`)
- Optional, opt-in calls to external embedding APIs (off by default — local Model2Vec is the default)

### How do I install all four?

```bash
git clone https://github.com/provasign/provasign
cd provasign
cd grove && make install && cd ..
cd prism && make install && cd ..
cd fuse  && make install && cd ..
cd provasign && make install && cd ..
```

Grove must be installed first — the other three depend on it. They auto-start Grove if it isn't running.

### Do I have to install all four?

No. You can install only what you need. The minimum useful subset:
- **Just Prism (+ Grove):** Better context for your agent. 5-minute install. Saves 35–92% tokens on first reads.
- **Just Fuse (+ Grove):** Symbol-level merge. 2-minute install. Solves false git conflicts.
- **Just Provasign (+ Grove):** Certified admission. 10-minute install. Eliminates the CI-loop with agents.

Grove is the only hard dependency.

---

## For Developers

### How long until I see value?

If you install Prism and start a coding agent task, **the first query.** Run `prism savings` and you'll see the token reduction. Most developers see immediate improvement in the agent's ability to find relevant code.

### Will it slow down my agent?

No. Prism `query` end-to-end is under 200 ms (target). The Grove BFS + ranking is faster than the agent's first generation token would have been.

### What languages are supported?

Eleven AST-parsed languages today: Go, TypeScript, TSX, JavaScript (incl. JSX/MJS/CJS), Python, Java, Rust, C, C++, C#, PHP.

Non-code files indexed as documents: Markdown, YAML, JSON, XML, TOML, INI, shell scripts, Dockerfiles, Makefiles, SQL, GraphQL, Protobuf, plain text, and more.

### My language isn't there. What now?

Two options:
1. **File an issue.** Most language additions are 1–2 days of work (Tree-sitter grammar + extractor in `grove/internal/parser/strategies/`). Ruby, Swift, Kotlin, and Elixir are commonly requested.
2. **Fall back to plaintext indexing.** Files with unknown extensions are still indexed for full-text search; you'll get FTS5 keyword matching but no symbol graph.

### Does it work with monorepos?

Yes. We benchmark against a 9,901-file monorepo. Index time is 34 seconds cold (delta is milliseconds on incremental). Query latency stays under 70 ms.

For very large monorepos (50K+ files), expect higher resident memory (the in-memory graph scales with project size). We're working on lazy-loading for monorepo scale.

### Does it work offline / on a flight?

Yes. Once installed, everything runs locally. No network calls. The Model2Vec embedding model (29 MB) is compiled into the Grove binary — no model download at runtime.

### What MCP-capable tools work with this?

Confirmed working today:
- Claude Code CLI
- GitHub Copilot in VS Code (and native VS Code extension without MCP)
- Cursor
- Codex CLI
- Windsurf
- Zed
- Claude Desktop
- Kiro
- Continue

`prism init` and `provasign init` auto-detect installed tools and write their MCP configs.

### I use [tool not in the list]. Will it work?

If it speaks MCP (Model Context Protocol over stdio), almost certainly. Point it at `prism mcp <dir>` or `provasign mcp serve` and the tools will appear.

### Does this work alongside my existing tools (LSP, ctags, etc.)?

Yes — Grove doesn't conflict with LSP or any other tool. It's a separate process exposing its own API. Keep your LSP for IDE features (hover, completion, refactor). Use Grove for AI agents.

---

## For Team Leads

### How do I roll this out to my team?

Start with **Prism alone**, on opt-in basis. Five-minute install, no shared infrastructure, immediate per-developer wins. After 1–2 weeks, you'll have data on which developers found it useful.

Then add **Fuse** as a per-repo opt-in via `.gitattributes`. Each developer who wants symbol-merge runs `fuse install` once on their laptop. Doesn't affect anyone who hasn't.

Add **Provasign** last, and start with `provasign check` in warning mode. After your team is comfortable with the findings, switch gates to enforce. The `provasign init` command writes Pre-Flight Autopilot instructions to your repo's agent files (CLAUDE.md, .cursorrules, etc.) so the agent calls Provasign automatically.

### Does it require special CI configuration?

Not initially. Provasign can run as a git pre-push hook with zero CI changes. When you want CI to enforce the same gates Provasign runs locally, point CI at `provasign certify` — same config, same gates, same results.

### How do I prevent developers from bypassing the hook?

Use `provasign hook install --enforce` to refuse `--no-verify` pushes. For team-scale enforcement, point your remote (GitHub branch protection, GitLab push rules) at the same `provasign.yaml` config — the gates run server-side too.

### What's the upgrade path from "individual laptops" to "team-wide"?

Phase 1 (today): SQLite + local Ed25519 key per developer. Each laptop has its own audit trail.

Phase 2 (on the roadmap): Same binary, same config, but pointing at a shared Postgres + Redis + KMS. Same certificate format. Shared audit trail. No re-training, no migration.

### How does it work with code review?

Provasign doesn't replace human code review. It moves the *mechanical* gates (build, tests, coverage, secrets, SAST, deps) out of the post-PR loop and into the agent loop. Human review focuses on what humans are good at: architectural fit, business logic, intent.

CodeRabbit-style AI review still adds value on top — Provasign doesn't try to replicate it.

---

## For Engineering Executives

### What problem is Provasign actually solving?

The bottleneck in AI-assisted development has moved. It's not the agent's code quality anymore — agents are good. It's:
1. Context delivery (agents work with too little information)
2. Merge friction (false conflicts on agent PRs)
3. CI loop churn (agents iterate against CI findings 3–5×)
4. Audit gap (no record of what the agent was asked to do)

Provasign addresses all four.

### Is there a business case I can present?

Yes — see [Why Provasign → What This Costs You]({{ '/why/#what-this-costs-you' | relative_url }}). The headline numbers:
- Token cost per developer drops 30–60% for any agent using Prism
- Merge time on parallel agent PRs drops to near-zero for incremental conflicts
- CI-loop iterations per PR drop from 3–5 to 0–1 with Provasign in the agent loop
- Audit prep time drops dramatically — every commit has a signed cert linked to the prompt

### What's the ROI argument?

Take a team of 10 developers each producing 5 agent-assisted PRs per week:
- 50 PRs/week × 3 average CI-loop iterations × 8 minutes per iteration = **20 developer-hours/week lost to CI-loop churn**
- 50 PRs/week × ~25% with merge conflicts × 15 minutes = **3 developer-hours/week lost to false conflicts**
- Token cost reduction: ~$200–$400/developer/month at typical usage

The combined operational savings, just from the loop reduction, are typically $30K–$80K/year for a 10-developer team — before counting the audit / compliance value.

These are estimates, not guarantees. The right move is to measure on your team with a 30-day trial.

### How is this different from Copilot Enterprise?

Copilot Enterprise is a managed agent + features. Provasign is infrastructure beneath any agent. They are not substitutes — they are complementary. You can run Copilot Enterprise on top of Prism + Fuse + Provasign. The agent gets better context (Prism), causes fewer merge conflicts (Fuse), and produces certified commits (Provasign) — without changing the developer's chosen agent.

### What's the path to commercialization?

Grove, Prism, and Fuse are MIT licensed. Provasign is AGPL-3.0 licensed. We expect the commercial path to be:
- Team mode (Postgres + Redis + KMS) — open-source binary, paid support for enterprise deployment
- Agent execution platform (Phase 3) — paid product, self-hostable, with optional managed
- Compliance attestation packs (SOC 2, FedRAMP, EU AI Act) — paid evidence-mapping consulting

Today, no paid path exists. We are intentionally pre-revenue.

---

## For Security / CISO

### Where does my code go?

Nowhere. Provasign is 100% local. The Grove engine is embedded in-process — no daemon, no listening port. No outbound network calls except downloading pinned analyzer tools on first use, and optional opt-in embedding APIs (off by default).

### What about telemetry?

None.

### How are secrets protected?

- Grove is now an embedded library — there is no HTTP daemon, no port, and no bearer token to leak
- Prism and Provasign MCP servers run as local stdio processes; they never bind a TCP socket
- The Ed25519 admission key for Provasign lives at `<repo>/.provasign/keys/signing.ed25519.key` (mode 0600), or `~/.provasign/keys/` with `--user`
- Credentials (Jira, GitHub) are environment variables only — never persisted to `.provasign/provasign.yaml`

### What if my codebase is in a regulated environment (HIPAA, FedRAMP, etc.)?

Provasign is well-suited to regulated environments precisely because it runs locally. The full code path:
- Source code never leaves your machine
- Index data lives in `.grove/grove.db` on your laptop (or your team's self-hosted server in Phase 2)
- Cryptographic admission certificates are reproducible — you can prove what gates passed at commit time
- Static analysis findings, test results, and signed commits all live in your git history

For specific compliance frameworks, see [Use Cases → Audit]({{ '/use-cases/audit/' | relative_url }}).

### Are the dependencies safe?

Provasign is built on:
- Go 1.22+ (compiled into static binaries — no Go runtime needed at runtime)
- Tree-sitter (C library, vendored)
- SQLite (embedded via `modernc.org/sqlite` — pure Go, no CGO)
- Model2Vec (potion-base-8M, embedded in the binary — no inference server)
- Standard Go crypto libraries for Ed25519, SHA-256, etc.

Each product's `go.mod` is auditable. We avoid heavy dependency trees deliberately.

### How does the Ed25519 signing chain work?

1. `provasign init` generates an Ed25519 keypair at `<repo>/.provasign/keys/signing.ed25519.key` (mode 0600); the public half is `signing.ed25519.pub`
2. On each admission, Provasign computes CanonicalBytes over: ChangeSet + effective config hash + toolchain versions + test results + findings
3. Ed25519 signature is computed before the commit SHA exists
4. After signing, Provasign creates the commit; the commit SHA is recorded in the trailer
5. `provasign cert verify <id>` re-validates the signature
6. `provasign cert replay <id>` re-runs the gates against current tools and returns `byte_reproducible` / `tool_drift` / `config_drift`

The signature attests to gates passing — not to the commit existing. This separation is what enables auditable replay.

### Can someone else verify a certificate on their machine?

In laptop mode, not yet. `provasign cert verify` and `provasign cert replay` need both the certificate store (`.provasign/engine.db`) and the signing key — both local and gitignored — so verification runs on the machine that produced the admission. **Independent, cross-machine verification by an auditor, a CI runner, or a reviewer on a fresh clone is a Provasign server (team) mode capability** (shared certificate store + KMS-backed signer), which is on the roadmap. See [Deployment modes]({{ '/architecture/#deployment-modes' | relative_url }}).

### What's the threat model?

See [Use Cases → Security]({{ '/use-cases/security/' | relative_url }}). Summary:
- Threat: agent produces malicious code → Mitigation: Provasign's gates run regardless of agent
- Threat: attacker forges a Provasign cert → Mitigation: Ed25519 signature with private key never leaving disk
- Threat: dependency supply-chain attack → Mitigation: Stage 2 dep audit (govulncheck, npm audit, pip-audit) + Sigstore/SLSA on the build artifact (out of scope for Provasign)

---

## For Compliance / Audit

### What evidence does Provasign produce?

For every admitted commit:
- The original user prompt, captured as a YAML intent in the repo
- The Ed25519-signed certificate proving build, test, coverage, secrets, SAST, deps gates passed
- The exact toolchain versions that ran (Stage 1 + Stage 2)
- The effective config hash, byte-reproducible
- The commit trailer linking all of the above to the commit SHA

### Can I audit a 6-month-old commit?

Yes. `provasign cert show <id-or-ref>` displays the full certificate. `provasign cert replay <id>` re-runs the gates and returns one of:
- `byte_reproducible` — current tools + config match the cert's gates
- `tool_drift` — tool versions have changed; gates would pass but not bit-identically
- `config_drift` — your `.provasign/provasign.yaml` has changed since the cert was created

This is the difference between "we had CI green at the time" (no longer verifiable after CI logs roll off) and "we have a cryptographic record we can re-validate today."

### Does this help with SOC 2?

It directly addresses several SOC 2 Type II controls:
- CC4.1, CC4.2: ongoing monitoring of controls — every commit's cert is a control evidence record
- CC6.1: logical access — per-developer Ed25519 admission keys (mode 0600); the engine is in-process with no network access path
- CC7.1: detection of vulnerabilities — Stage 2 SAST + dep audit on every commit
- CC8.1: change management — every change is admitted via Provasign, linked to an intent, signed

A detailed mapping is in [Use Cases → Audit]({{ '/use-cases/audit/' | relative_url }}).

### Does this help with EU AI Act compliance?

The EU AI Act's high-risk activation in August 2026 creates documentation and traceability requirements for AI-generated artifacts in regulated industries. Provasign's intent-capture + signed-admission flow gives you the traceability artefact the act requires: a record of what the AI was asked to do, what gates verified the output, and a cryptographic proof you can show an auditor.

This is not legal advice. Talk to your compliance team about which AI Act provisions apply to you.

---

## Operational / Troubleshooting

### Grove won't start — port 7777 already in use

Not applicable: Grove is now an embedded library and does not open any TCP
port. If you see a tool referencing port 7777, you're on a pre-embedded build
— upgrade to the latest release.

### Prism savings shows 0 even after using the agent

The agent isn't calling Prism tools. Check:
1. `prism init` was run in this directory — look for `.mcp.json` (Claude Code) or `.cursor/mcp.json` (Cursor)
2. The coding tool was restarted after `prism init`
3. The CLAUDE.md / .cursorrules instructions tell the agent to use Prism

### Fuse declares conflicts on files it should auto-resolve

Most common cause: the language isn't in the `.gitattributes` whitelist. Check:
```bash
cat .gitattributes
# Should have entries like *.go merge=fuse
```

If your file extension is included, the conflict may be genuinely ambiguous — Fuse intentionally falls back to conflict markers when confidence < 0.70. Read `.git/fuse/conflict-<hash>.md` for the structured handoff.

### Provasign certify is failing

The first thing to check: `provasign certify --verbose` shows which stage failed.
- Stage 1 fail: build or tests failed. Run them outside Provasign to confirm.
- Stage 2 fail: a SAST or secrets finding above your gate threshold. `provasign explain <finding-id>` shows the rule and fix.

### provasign init didn't detect my coding tool

The detection is based on global configs and executables. If your tool is installed in an unusual location, run:
```bash
provasign mcp install-for <tool>   # claude-code, cursor, windsurf, continue
```

### The pre-push hook is blocking my push and I need to push now

The hook is a backstop, not the only enforcement. To bypass for one push:
```bash
git push --no-verify
# This is detected and audited — see .provasign/.cache/audit.log
```

Use sparingly. If you find yourself bypassing often, the gates may be too strict — adjust `.provasign/provasign.yaml`.

### My index is stale — what triggers a re-index?

Grove uses git blob SHA as the freshness key. Files committed (or staged) get re-indexed automatically on the next `prism index` or via the auto-index hook. To force a clean re-index:
```bash
rm -rf .grove/grove.db
grove index .
```

---

## Anything else?

Open an issue at [github.com/provasign/provasign/issues](https://github.com/provasign/provasign/issues) or add to this FAQ via PR.
