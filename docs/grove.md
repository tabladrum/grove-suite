---
title: Grove
layout: default
nav_order: 3
description: "Grove — persistent code knowledge graph. The long-term memory of your codebase, queryable by any AI agent."
permalink: /grove/
---

# Grove

**Your codebase's persistent long-term memory — queryable by any AI agent.**
{: .fs-5 .fw-300 }

[Install Grove]({{ '/installation/' | relative_url }}){: .btn .btn-primary .fs-5 .mb-4 .mb-md-0 .mr-2 }
[View source](https://github.com/tabladrum/grove-suite/tree/main/grove){: .btn .fs-5 .mb-4 .mb-md-0 }

---

Grep answers "does this string appear somewhere?" A language server answers "where is this symbol defined?" Grove answers the harder questions AI agents actually need:

- *What does changing this function break — across the entire codebase?*
- *Which tests cover this method, directly or transitively?*
- *What is the full dependency chain from this file?*
- *What symbols are semantically related to this task description?*

The difference is a graph. Grove indexes your source files into a persistent SQLite graph — 11 languages, 8 edge types, BFS traversal — and keeps it live with delta indexing (files whose git blob SHA hasn't changed are never re-parsed).

Every other tool in Grove Suite is built on Grove. Without it, Prism falls back to filename guessing, Fuse to line-level merge, Relay to coarse test selection.

---

## The Graph

A real Grove graph for an authentication module — 8 nodes, 9 typed edges. Drag any node to explore. Edge colors show relationship type.

<style>
.grove-graph-wrap {
  position: relative;
  margin: 1.5rem 0 2rem;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #f9fafb;
  overflow: hidden;
}
.grove-graph-wrap svg { display: block; }
.grove-graph-legend {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem 1.25rem;
  padding: 0.65rem 1rem;
  border-top: 1px solid #e5e7eb;
  background: #fff;
  font-size: 0.78rem;
  font-family: ui-sans-serif, system-ui, sans-serif;
}
.grove-graph-legend-item { display: flex; align-items: center; gap: 5px; color: #374151; }
.grove-graph-legend-item span {
  display: inline-block; width: 28px; height: 2.5px; border-radius: 2px;
}
.grove-node-legend {
  display: flex; flex-wrap: wrap; gap: 0.4rem 1rem;
  padding: 0.5rem 1rem 0.65rem;
  border-top: 1px solid #f3f4f6;
  background: #fff;
  font-size: 0.75rem;
  font-family: ui-sans-serif, system-ui, sans-serif;
}
.grove-node-legend-item { display: flex; align-items: center; gap: 5px; color: #6b7280; }
.grove-node-legend-item span {
  display: inline-block; width: 10px; height: 10px; border-radius: 50%;
}
.grove-tooltip {
  position: absolute;
  background: rgba(15,23,42,0.88);
  color: #f8fafc;
  padding: 4px 9px;
  border-radius: 5px;
  font-size: 11.5px;
  font-family: ui-sans-serif, system-ui, sans-serif;
  pointer-events: none;
  white-space: nowrap;
  opacity: 0;
  transition: opacity 0.12s;
  z-index: 10;
}
</style>

<div class="grove-graph-wrap" markdown="0">
  <div id="grove-graph"></div>
  <div class="grove-graph-legend">
    <div class="grove-graph-legend-item"><span style="background:#166534"></span>defines</div>
    <div class="grove-graph-legend-item"><span style="background:#2563eb"></span>calls</div>
    <div class="grove-graph-legend-item"><span style="background:#7c3aed"></span>imports</div>
    <div class="grove-graph-legend-item"><span style="background:#d97706"></span>uses-type</div>
    <div class="grove-graph-legend-item"><span style="background:#dc2626"></span>tests</div>
  </div>
  <div class="grove-node-legend">
    <div class="grove-node-legend-item"><span style="background:#0d3320"></span>file</div>
    <div class="grove-node-legend-item"><span style="background:#166534"></span>function</div>
    <div class="grove-node-legend-item"><span style="background:#1d4ed8"></span>type</div>
    <div class="grove-node-legend-item"><span style="background:#065f46"></span>test file</div>
    <div class="grove-node-legend-item"><span style="background:#16a34a"></span>test function</div>
  </div>
  <div class="grove-tooltip" id="grove-tip"></div>
</div>

<script src="https://cdn.jsdelivr.net/npm/d3@7/dist/d3.min.js"></script>
<script>
(function () {
  var el = document.getElementById('grove-graph');
  if (!el || typeof d3 === 'undefined') return;

  var W = el.parentElement.clientWidth || 720;
  var H = 390;

  var nodes = [
    { id: 'auth.go',          label: 'auth.go',             type: 'file',     r: 15 },
    { id: 'Login',            label: 'Login()',             type: 'function', r: 12 },
    { id: 'validatePassword', label: 'validatePassword()',  type: 'function', r: 11 },
    { id: 'User',             label: 'User',                type: 'type',     r: 11 },
    { id: 'session.go',       label: 'session.go',          type: 'file',     r: 14 },
    { id: 'NewSession',       label: 'NewSession()',        type: 'function', r: 11 },
    { id: 'auth_test.go',     label: 'auth_test.go',        type: 'testfile', r: 13 },
    { id: 'TestLogin',        label: 'TestLogin()',         type: 'test',     r: 11 },
  ];

  var links = [
    { source: 'auth.go',      target: 'Login',            etype: 'defines'   },
    { source: 'auth.go',      target: 'validatePassword', etype: 'defines'   },
    { source: 'auth.go',      target: 'User',             etype: 'uses-type' },
    { source: 'auth.go',      target: 'session.go',       etype: 'imports'   },
    { source: 'Login',        target: 'validatePassword', etype: 'calls'     },
    { source: 'Login',        target: 'NewSession',       etype: 'calls'     },
    { source: 'session.go',   target: 'NewSession',       etype: 'defines'   },
    { source: 'auth_test.go', target: 'TestLogin',        etype: 'defines'   },
    { source: 'TestLogin',    target: 'Login',            etype: 'tests'     },
  ];

  var NC = { file: '#0d3320', function: '#166534', type: '#1d4ed8', testfile: '#065f46', test: '#16a34a' };
  var EC = { defines: '#166534', calls: '#2563eb', imports: '#7c3aed', 'uses-type': '#d97706', tests: '#dc2626' };

  var svg = d3.select(el).append('svg')
    .attr('width', '100%').attr('height', H)
    .attr('viewBox', [0, 0, W, H].join(' '));

  // Arrow markers — refX=0, we offset line endpoints manually
  var defs = svg.append('defs');
  Object.keys(EC).forEach(function (t) {
    defs.append('marker')
      .attr('id', 'arr-' + t.replace(/[^a-z]/g, '-'))
      .attr('viewBox', '0 -5 10 10').attr('refX', 0).attr('refY', 0)
      .attr('markerWidth', 6).attr('markerHeight', 6).attr('orient', 'auto')
      .append('path').attr('d', 'M0,-5L10,0L0,5')
      .attr('fill', EC[t]).attr('opacity', 0.8);
  });

  var sim = d3.forceSimulation(nodes)
    .force('link', d3.forceLink(links).id(function (d) { return d.id; }).distance(110))
    .force('charge', d3.forceManyBody().strength(-360))
    .force('center', d3.forceCenter(W / 2, H / 2))
    .force('x', d3.forceX(W / 2).strength(0.04))
    .force('y', d3.forceY(H / 2).strength(0.04));

  // Warm up off-screen so graph appears settled
  sim.tick(120); sim.stop();
  nodes.forEach(function (n) {
    n.x = Math.max(n.r + 30, Math.min(W - n.r - 60, n.x));
    n.y = Math.max(n.r + 20, Math.min(H - n.r - 30, n.y));
  });

  var linkSel = svg.append('g').selectAll('line')
    .data(links).join('line')
    .attr('stroke', function (d) { return EC[d.etype]; })
    .attr('stroke-width', 1.9)
    .attr('stroke-opacity', 0.7)
    .attr('marker-end', function (d) { return 'url(#arr-' + d.etype.replace(/[^a-z]/g, '-') + ')'; });

  var nodeSel = svg.append('g').selectAll('g')
    .data(nodes).join('g')
    .attr('cursor', 'grab')
    .call(d3.drag()
      .on('start', function (ev, d) {
        if (!ev.active) sim.alphaTarget(0.3).restart();
        d.fx = d.x; d.fy = d.y;
      })
      .on('drag', function (ev, d) { d.fx = ev.x; d.fy = ev.y; })
      .on('end', function (ev, d) {
        if (!ev.active) sim.alphaTarget(0);
        d.fx = null; d.fy = null;
      }));

  nodeSel.append('circle')
    .attr('r', function (d) { return d.r; })
    .attr('fill', function (d) { return NC[d.type]; })
    .attr('stroke', '#fff').attr('stroke-width', 2);

  nodeSel.append('text')
    .attr('dy', function (d) { return d.r + 13; })
    .attr('text-anchor', 'middle')
    .attr('font-size', '10px')
    .attr('font-family', 'ui-monospace, SFMono-Regular, monospace')
    .attr('fill', '#374151')
    .text(function (d) { return d.label; });

  var tip = document.getElementById('grove-tip');

  nodeSel.on('mousemove', function (ev, d) {
    var rect = el.parentElement.getBoundingClientRect();
    tip.style.left = (ev.clientX - rect.left + 12) + 'px';
    tip.style.top  = (ev.clientY - rect.top  - 32) + 'px';
    tip.style.opacity = '1';
    tip.textContent = d.label + ' · ' + d.type;
  }).on('mouseleave', function () { tip.style.opacity = '0'; });

  function ticked() {
    // Offset line endpoints to node edge so arrows land cleanly
    linkSel
      .attr('x1', function (d) { return d.source.x; })
      .attr('y1', function (d) { return d.source.y; })
      .attr('x2', function (d) {
        var dx = d.target.x - d.source.x, dy = d.target.y - d.source.y;
        var dist = Math.sqrt(dx * dx + dy * dy) || 1;
        return d.target.x - dx / dist * (d.target.r + 5);
      })
      .attr('y2', function (d) {
        var dx = d.target.x - d.source.x, dy = d.target.y - d.source.y;
        var dist = Math.sqrt(dx * dx + dy * dy) || 1;
        return d.target.y - dy / dist * (d.target.r + 5);
      });
    nodeSel.attr('transform', function (d) { return 'translate(' + d.x + ',' + d.y + ')'; });
  }

  ticked(); // render initial settled positions immediately
  sim.on('tick', ticked).alpha(0.25).restart();
})();
</script>

---

## What It Does

| Capability | How |
|------------|-----|
| Parse | Tree-sitter AST walkers for 11 languages + regex fallback for syntax-error recovery |
| Store | SQLite WAL + FTS5 full-text search, delta indexing by git blob SHA |
| Query | BFS graph traversal across 8 edge types, FTS5 keyword search, Model2Vec semantic similarity |
| Serve | Embedded Go library (`grove/pkg/grove`) · CLI for one-shot queries · MCP stdio (`grove mcp`) for AI agents |
| Scale | 10K-file monorepo in 34 seconds cold; delta re-index on a one-file change in milliseconds |

---

## Languages Supported

**AST-parsed (with full symbol graph):**

Go · TypeScript · TSX · JavaScript (incl. JSX/MJS/CJS) · Python · Java · Rust · C · C++ · C# · PHP

**Indexed as documents (FTS5 + Model2Vec, no symbol graph):**

Markdown · YAML · JSON · XML · TOML · INI · shell scripts · Dockerfile · Makefile · SQL · GraphQL · Protobuf · CSV · plain text

A semantic query for "deployment configuration" returns both the Go function that reads the config *and* the Dockerfile that defines the runtime — together, ranked.

---

## Graph Edges (the secret sauce)

Eight typed edges connect symbols. Each one answers a different question.

| Edge | Question it answers |
|------|--------------------|
| `defines` | Where is this symbol defined? |
| `contains` | What does this class/namespace include? |
| `imports` | What files does this file pull in? |
| `extends` | What does this class inherit from? |
| `implements` | What interfaces does this class satisfy? |
| `calls` | What functions does this function call? (scoped to same-file + imports) |
| `uses-type` | What types does this function reference? (scoped to same-file + imports) |
| `tests` | What tests exercise this symbol? |

**Why `calls` is scoped:** Without scoping `calls` and `uses-type` edges to same-file and imported files, a function named `parse` in one package would appear to call every `parse` function in unrelated packages — producing roughly 5× the false-positive edges. This single design choice is why Grove's blast radius queries are useful instead of noisy.

---

## Performance

Benchmarks run on macOS against synthetic Go projects (2026-05-27). Numbers reflect a cold index. Subsequent runs on unchanged projects complete in milliseconds.

| Project | Files | Index time | Peak RSS | Query latency |
|---------|------:|-----------:|---------:|--------------:|
| Small | 61 | 0.06 s | 30 MB | 6 ms |
| Medium | 801 | 0.85 s | 55 MB | 6 ms |
| Large | 4,501 | 11.6 s | 117 MB | 9 ms |
| Monorepo | 9,901 | 34.0 s | 196 MB | 61 ms |

**Targets:** index 5,000 files < 5 s · BFS depth-3 on 50K nodes < 30 ms · FTS5 query < 10 ms

---

## How AI Agents Use It

Grove exposes **8 tools over MCP stdio** (Model Context Protocol) — accessible to any MCP-capable AI agent:

| Tool | Purpose |
|------|---------|
| `grove_index` | Index or reindex a directory |
| `grove_symbols` | Search for symbols by name |
| `grove_query` | Retrieve ranked context for an intent |
| `grove_impact` | Blast radius for a symbol or file |
| `grove_deps` | Dependency tree for a file |
| `grove_tests` | Tests that cover a symbol |
| `grove_icr` | Intent complexity rating |
| `grove_conflicts` | Potential conflict hotspots |

Most users don't run Grove directly — they install [Prism]({{ '/prism/' | relative_url }}), which wraps Grove with token-optimized context delivery. But for custom agent integrations, you can connect directly via MCP, HTTP, or gRPC.

---

## Quick Start

```bash
# Install (see the full installation guide for all options)
cd grove-suite/grove && make install

# Index a project
cd /your/project
grove index .

# Query symbols
grove symbols "AuthService"

# Get blast radius for a symbol
grove impact "validatePassword"

# Prism, Fuse, and Relay open Grove as an embedded library — no daemon to start.
```

[Full installation guide →]({{ '/installation/' | relative_url }})

---

## Security

Grove runs as an embedded library inside Prism, Fuse, and Relay — there is no
TCP listener, no port, and no bearer token. Index data lives at `.grove/grove.db`
locally; nothing leaves your machine.

Zero telemetry. Your code never leaves your machine.

---

## Read More

- [How Grove compares to LSP, Sourcegraph, ctags, Stack Graphs]({{ '/comparisons/#grove-vs-other-code-intelligence' | relative_url }})
- [Why Grove Suite exists]({{ '/why/' | relative_url }})
- [Full reference on GitHub](https://github.com/tabladrum/grove-suite/tree/main/grove)
- [Architecture and inter-product API contracts](https://github.com/tabladrum/grove-suite/blob/main/Architecture.md)
