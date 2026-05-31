package grove
package grove

// Engine-mode Grove client extensions.
// The legacy ICR() (returning {SymbolCount, Rating}) is preserved for Phase-1
// callers (ingestion/gs.go); engine code uses the new ICRRegion() which returns
// the real IsolatedChangeRegion payload Grove emits.

// Symbol mirrors the subset of grove core.SymbolRecord fields engine code needs.
type Symbol struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	QualifiedName string   `json:"qualifiedName"`
	FilePath      string   `json:"filePath"`
	Kind          string   `json:"kind"`
	Language      string   `json:"language"`
	Tags          []string `json:"tags,omitempty"`
}

// Edge mirrors the subset of grove core.Edge fields engine code needs.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

// ICRRegion mirrors grove core.IsolatedChangeRegion. Duplicated here to avoid
// importing the grove module (relay must build without a grove source checkout).
type ICRRegion struct {
	IntentID       string   `json:"intentId"`
	Exclusive      []string `json:"exclusive"`
	SharedRead     []string `json:"sharedRead"`
	Boundary       []string `json:"boundary"`
	ExclusiveFiles []string `json:"exclusiveFiles"`
	ReadableFiles  []string `json:"readableFiles"`
	Confidence     float64  `json:"confidence"`
	LockKeys       []string `json:"lockKeys"`
}

// Impact returns the symbols affected by a query/file location.
// maxDepth is the BFS depth limit; 0 means Grove's default.
func (c *Client) Impact(query string, maxDepth int) ([]Symbol, error) {
	var resp struct {
		Nodes []Symbol `json:"nodes"`
	}
	body := map[string]any{"query": query}
	if maxDepth > 0 {
		body["maxDepth"] = maxDepth
	}
	if err := c.post("/impact", body, &resp); err != nil {
		return nil, err
	}
	return resp.Nodes, nil
}

// Deps returns the import/uses-type edges originating in the given file.
func (c *Client) Deps(file string) ([]Edge, error) {
	var resp struct {
		Edges []Edge `json:"edges"`
	}
	if err := c.post("/deps", map[string]string{"file": file}, &resp); err != nil {
		return nil, err
	}
	return resp.Edges, nil
}

// Symbols returns symbols whose qualified name matches the FTS query.
func (c *Client) Symbols(query string, limit int) ([]Symbol, error) {
	if limit <= 0 {
		limit = 20
	}
	var resp struct {
		Symbols []Symbol `json:"symbols"`
	}
	body := map[string]any{"query": query, "limit": limit}
	if err := c.post("/symbols", body, &resp); err != nil {
		return nil, err
	}
	return resp.Symbols, nil
}

// ICRRegion returns the full IsolatedChangeRegion for an intent description.
// This is the engine-facing replacement for the legacy ICR() method.
func (c *Client) ICRRegion(intent string) (*ICRRegion, error) {
	var out ICRRegion
	if err := c.post("/icr", map[string]string{"intent": intent}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
