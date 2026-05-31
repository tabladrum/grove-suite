package mcp

// toolDescriptors returns the MCP tool list metadata. Schemas are
// `additionalProperties: true` because every tool accepts a permissive
// JSON map; clients should consult the per-tool description for required
// keys.
func toolDescriptors() []map[string]any {
	return []map[string]any{
		{
			"name":        "relay_check",
			"description": "Run all policy gates (pre + Stage1 + Stage2) on a proposed diff without admitting. Args: diff or diff_path, intent, brief, author, model, repo. Returns engine.Result with allowed=true/false and per-policy verdicts.",
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
	}
}
