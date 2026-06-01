---
title: Overview
layout: default
nav_order: 1
description: "Relay — certified delivery for AI coding agents. Every commit signed, tested, and traceable to the prompt that created it."
permalink: /relay/
---

# Relay
{: .no_toc }

**Certified delivery for AI coding agents. Every commit signed, tested, and traceable to the prompt that created it.**
{: .fs-5 .fw-300 }

[Install Relay]({{ '/installation/' | relative_url }}){: .btn .btn-primary .fs-5 .mb-4 .mb-md-0 .mr-2 }
[View source](https://github.com/tabladrum/grove-suite/tree/main/relay#readme){: .btn .fs-5 .mb-4 .mb-md-0 }

---

AI agents now write more of the codebase than people do, but the delivery infrastructure around them — pull requests, CI, line-based merge — was built for humans reviewing humans. Relay is infrastructure built for the new reality: it moves quality gates **into the agent's loop**, and turns every admitted change into a cryptographically verifiable record.

It is a **single binary**. No server, no port, no token, no daemon. The [Grove knowledge-graph engine]({{ '/architecture/' | relative_url }}) is embedded in-process, and everything runs locally — your source never leaves the machine.

## Before and after

**Before:** prompt → agent codes → opens PR → CI finds an issue → back to agent → fix → CI finds another → repeat 3–5× → human reviews a diff with no idea what the prompt was.

**After:** prompt → captured as a YAML intent → agent codes → `relay_check` (sub-10s) → agent self-corrects → `relay_certify` signs the result → linear commit carrying `Intent-ID` + `Certificate-ID` → replayable forever.

## Start here

- **[Why Relay]({{ '/why/' | relative_url }})** — the bottlenecks Relay removes, and what it deliberately isn't.
- **[How It Works]({{ '/how-it-works/' | relative_url }})** — capture → gate → certify → sign → replay.
- **[Architecture]({{ '/architecture/' | relative_url }})** — the embedded Grove engine and the single-binary design.
- **[Features]({{ '/features/' | relative_url }})** — gates, certificates, profiles, and agent wiring.
- **[Use Cases]({{ '/use-cases/' | relative_url }})** — security, audit, change management, traceability.
- **[Installation]({{ '/installation/' | relative_url }})** · **[Agent Setup]({{ '/setup/' | relative_url }})** — get running in minutes.

## Quick start (laptop mode — zero infrastructure)

```bash
# Relay is a single binary with Grove embedded — nothing else to install.
cd grove-suite/relay && make install && cd ..

cd /your/project
relay init --stack=auto      # scaffolds .relay/, generates a local Ed25519 key,
                             # writes agent steering + MCP configs for every detected tool
relay hook install           # git pre-push backstop
relay tools install          # fetch pinned analyzers (semgrep/ruff via pipx)
relay doctor                 # verify analyzer readiness

git add .relay/ && git commit -m "Add Relay configuration"
# Your AI agent now calls relay_check before every PR — automatically.
```

## Compliance & audit

- **SOC 2 Type II** — the signed-admission flow maps to CC4.1/CC4.2, CC6.1, CC7.1, and CC8.1. See [Use Cases]({{ '/use-cases/' | relative_url }}).
- **EU AI Act** (high-risk activation August 2026) — intent capture + signed admission produces the traceability artifact the regulation expects. See [Traceability]({{ '/use-cases/traceability/' | relative_url }}).

The audit triple on every Relay-admitted commit: **the prompt** (committed YAML intent), **the proof** (Ed25519-signed certificate), **the replay** (`relay cert replay`).
