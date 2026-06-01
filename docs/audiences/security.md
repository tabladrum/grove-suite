---
title: Security Brief
layout: default
nav_order: 3
description: "How Relay keeps code local and limits exposure while supporting AI-assisted development."
permalink: /audiences/security/
---

# Security Brief

Relay is local-first by design — and so are the open-source components it embeds (Grove, Prism, Fuse). Everything runs on the developer machine or self-hosted infrastructure, and the default flow never sends source code to external services.

## Security Properties

- Code stays on the local machine unless you explicitly choose a remote workflow
- Grove is embedded as an in-process library — **no network listeners, no ports, no shared-secret tokens**. The only attack surface is the local filesystem.
- Relay signs admitted commits with a local Ed25519 key (`~/.relay/keys/admission.ed25519`, mode 0600)
- The audit trail stays in the repo and local state, not in a SaaS log
- Zero telemetry

## What This Means For Teams

Security teams get stronger provenance without forcing the workflow into a managed cloud platform. That makes it easier to review what the agent was asked to do, what gates ran, and what was admitted.

## Related Pages

- [Audit brief]({{ '/audiences/audit/' | relative_url }})
- [FAQ]({{ '/faq/' | relative_url }})
- [Relay README]({{ '/relay/' | relative_url }})