# Grove Suite

A suite of four developer tools built on a shared code intelligence graph.

## Products

| Product | Description | Language | License |
|---------|-------------|----------|---------|
| **Grove** | Core code intelligence graph — universal symbol extractor, SQLite graph, MCP server | Go | MIT |
| **Prism** | Token-optimized context delivery for AI agents — ranking, compression, session tracking | Go | MIT |
| **Fuse** | Semantic merge driver — symbol-aware three-way merge, breaking change detection | Go | MIT |
| **Relay** | Intent-driven delivery platform — no branches, linear history, certification pipeline | Go | BSL |

## Dependency Map

```
Relay ──requires──► Grove
Fuse  ──requires──► Grove
Prism ──requires──► Grove
```

Grove is the only product with no suite dependencies. All others require a running Grove instance and auto-start it on startup.

## Getting Started

```bash
# 1. Install Grove (required by everything else)
cd grove && make install
grove init

# 2. Pick what you need
cd prism && make install    # context delivery for AI agents
cd fuse  && make install    # semantic git merge driver
cd relay && make install    # intent-driven CI/CD platform
```

## Suite-Level Docs

- [Product Roadmap](Product-Roadmap.md) — coordinated milestones and release calendar
- [Architecture](Architecture.md) — dependency map, API contracts, versioning policy
