package mcp

// toolDescriptors returns the MCP tool list metadata. Schemas are
// `additionalProperties: true` because every tool accepts a permissive
// JSON map; clients should consult the per-tool description for required
// keys.
func toolDescriptors() []map[string]any {
	return []map[string]any{
		{
			"name":        "relay_check",
			"description": "Run all policy gates (pre + Build + Analysis) on a proposed diff without admitting. Args: diff or diff_path, intent, brief, author, model, repo. Returns engine.Result with allowed=true/false and per-policy verdicts.",
			"inputSchema": map[string]any{"type": "object", "additionalProperties": true},
		},
		{
			"name":        "relay_certify",
			"description": "Alias for relay_check. Runs the full pipeline and returns a signed certificate without admitting to git.",
			"inputSchema": map[string]any{"type": "object", "additionalProperties": true},
		},
		{
			"name":        "relay_submit",
			"description": "Run relay_check; on allow, admit the diff via git (creates a signed commit). Args identical to relay_check. Returns engine.Result with commit_sha and certificate.",
			"inputSchema": map[string]any{"type": "object", "additionalProperties": true},
		},
		{
			"name":        "relay_policy",
			"description": "List the gates registered and their enabled/disabled state for the given repo. Args: repo (optional).",
			"inputSchema": map[string]any{"type": "object", "additionalProperties": true},
		},
		{
			"name":        "relay_explain",
			"description": "Return a long-form explanation of a policy gate or specific rule key. Args: gate (required), rule (optional).",
			"inputSchema": map[string]any{"type": "object", "additionalProperties": true},
		},
		// ── Auto-Intent Capture ─────────────────────────────────────────
		{
			"name": "relay_intent_open",
			"description": "Draft an Intent from the user's prompt BEFORE making code changes. " +
				"Stores a draft YAML at .relay/.cache/intents/INT-{id}.draft.yaml. " +
				"Returns the intent_id. The recommended agent system prompt instructs " +
				"calling this tool first whenever a user requests a code change.\n" +
				"Args: title (required, ≤80 chars summary of the user's request), " +
				"description (required, the verbatim user prompt), " +
				"agent (e.g. claude-code:1.4.2), model (e.g. claude-sonnet-4-6:2026-04-15), " +
				"conversation_ts (ISO8601), repo (optional).",
			"inputSchema": map[string]any{"type": "object", "additionalProperties": true},
		},
		{
			"name": "relay_intent_update",
			"description": "Refine an open intent draft. Args: intent_id (required), patch (map " +
				"of fields to merge: title, description, allowed_paths, forbidden_paths, " +
				"acceptance_criteria, domain, capability, ambiguity_policy, risk_level).",
			"inputSchema": map[string]any{"type": "object", "additionalProperties": true},
		},
		{
			"name": "relay_intent_close",
			"description": "Promote the intent draft to .relay/intents/{id}.yaml (committed). " +
				"Called when the agent reports complete. Returns the committed file path, " +
				"intent_hash, and a commit-trailer block to attach to the eventual " +
				"relay_certify or relay_submit commit.\n" +
				"Args: intent_id (required), agent (optional, identifies the closing agent).",
			"inputSchema": map[string]any{"type": "object", "additionalProperties": true},
		},
		{
			"name": "relay_intent_list",
			"description": "List intents in this repo. Args: status (\"draft\"|\"committed\"|\"all\"; " +
				"default \"all\"), repo (optional).",
			"inputSchema": map[string]any{"type": "object", "additionalProperties": true},
		},
	}
}
