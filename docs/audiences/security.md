---
title: Security Brief
layout: default
nav_order: 3
description: "How Grove Suite keeps code local and limits exposure while supporting AI-assisted development."
permalink: /audiences/security/
---

# Security Brief

Grove Suite is local-first by design. The core products run on the developer machine or on self-hosted infrastructure, and the default flow avoids sending source code to external services.

## Security Properties

- Code stays on the local machine unless you explicitly choose a remote workflow
- Grove binds to localhost and uses a bearer token for non-health requests
- Relay signs admitted commits with a local Ed25519 key
- The audit trail stays in the repo and local state, not in a SaaS log

## What This Means For Teams

Security teams get stronger provenance without forcing the workflow into a managed cloud platform. That makes it easier to review what the agent was asked to do, what gates ran, and what was admitted.

## Related Pages

- [Audit brief]({{ '/audiences/audit/' | relative_url }})
- [FAQ]({{ '/faq/' | relative_url }})
- [Relay README]({{ '/relay/' | relative_url }})