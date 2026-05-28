
## Prism — context delivery (ALWAYS use these tools)

This project uses [Prism](https://github.com/tabladrum/grove-suite/prism) for token-optimized context delivery.
Prism tools are registered via MCP. Follow these rules in every task:

### Rules
1. **Start every task with prism_query** — call it with the task description before
   reading any files. It returns pre-ranked, compressed context covering targets,
   dependencies, and tests within the token budget.

2. **Use prism_read instead of read_file / cat** — it applies session-aware compression:
   full text on first read, signatures on second, references on third+.
   This saves 35–92% tokens and prevents context window overflow.

3. **Use prism_search instead of grep/ripgrep** — when you need to find a symbol
   by name, use prism_search. Follow up with prism_lookup for the full source.

4. **Call prism_index once at session start** (or after significant file changes).
   Do not re-index on every step — delta indexing is automatic.

5. **Call prism_compact when the context window is near capacity** — it summarizes
   older turns while preserving recent ones.

6. **If a Prism tool returns empty results, do not immediately fall back to grep/read.**
	First run prism_index for the current workspace root and retry the same Prism tool.
	Only use non-Prism fallback if the second Prism attempt is still empty.

### Tool priority order
| Instead of...          | Use...         |
|------------------------|----------------|
| read_file / open file  | prism_read     |
| grep / ripgrep / find  | prism_search   |
| manual context gather  | prism_query    |
| symbol definition      | prism_lookup   |
