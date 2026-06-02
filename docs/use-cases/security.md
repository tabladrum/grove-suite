---
title: Security
layout: default
parent: Use Cases
nav_order: 1
description: "How Provasign keeps source local, minimizes attack surface, and signs provenance without a managed cloud."
permalink: /use-cases/security/
---

# Security

Provasign is local-first by design. The default flow never sends source code to an external service, runs no network listener, and stores its trust material on the developer's machine.

## Security properties

- **Source stays local.** Code never leaves the machine unless you explicitly push it. Zero telemetry.
- **Minimal attack surface.** The Grove engine is embedded in-process — **no ports, no daemon, no shared-secret token**. The only surface is the local filesystem.
- **Signed admission.** Each admitted commit is signed with a local Ed25519 key at `<repo>/.provasign/keys/signing.ed25519.key` (mode `0600`), or `~/.provasign/keys/` with `--user`.
- **Secrets via environment only.** Credentials are never written into `.provasign/provasign.yaml`.
- **Findings before merge.** The `secrets` and SAST gates run *in the agent's loop*, so a leaked key is caught before it ever reaches a PR — not after it's in history.

## The signing key and verification scope

The private key is the one piece of laptop-mode trust material that must stay secret; it is gitignored and `0600`. In **laptop mode, verification is local**: `provasign cert verify` and `provasign cert replay` run on the machine that produced the certificate, because both the certificate store (`.provasign/engine.db`) and the key are local and gitignored.

**Verifying an admission on a *different* machine — an auditor's laptop, a CI runner, a reviewer's checkout — requires Provasign server (team) mode**, where certificates and a KMS-backed signing key live in shared infrastructure rather than on one developer's disk. Server mode is on the roadmap; see [Deployment modes]({{ '/architecture/#deployment-modes' | relative_url }}).

## What this means for security teams

You get stronger provenance on AI-generated code **without** forcing the workflow into a managed cloud platform. It is straightforward to review what the agent was asked to do, which gates ran, and what was admitted — all from artifacts that live in the repo.

This maps to SOC 2 CC6.1 (logical access), CC7.1 (vulnerability detection), and supports the security posture the EU AI Act expects for high-risk activities.

## Related

- [Audit]({{ '/use-cases/audit/' | relative_url }}) · [Traceability]({{ '/use-cases/traceability/' | relative_url }})
- [Architecture]({{ '/architecture/' | relative_url }}) · [FAQ]({{ '/faq/' | relative_url }})
