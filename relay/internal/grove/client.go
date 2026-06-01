// Package grove is Relay's adapter to the in-process Grove engine. The
// previous HTTP client + auto-spawn of `grove serve` is gone: Relay now links
// against `grove/pkg/grove` and opens the on-disk index directly.
//
// Public API (Client, NewClient, WithTokenFromDir, EnsureRunning, Health,
// ICR, Impact, Deps, Symbols, ICRRegion) is preserved so the rest of Relay
// (ingestion, doctor, api/server, api/dashboard) compiles unchanged. baseURL
// and token-path fields are kept only so existing diagnostic strings work.
package grove

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	groveeng "github.com/tabladrum/grove-suite/grove/pkg/grove"
)

// Symbol is the subset of grove.Symbol that engine code consumes.
type Symbol struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	QualifiedName string   `json:"qualifiedName"`
	FilePath      string   `json:"filePath"`
	Kind          string   `json:"kind"`
	Language      string   `json:"language"`
	Tags          []string `json:"tags,omitempty"`
}

// Edge is the subset of grove.Edge engine code consumes.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

// ICRResponse is the legacy {SymbolCount, Rating} shape kept for the Phase-1
// ingestion path (ingestion/gs.go). New code should use ICRRegion.
type ICRResponse struct {
	SymbolCount int    `json:"symbol_count"`
	Rating      string `json:"rating"`
}

// ICRRegion mirrors grove.IsolatedChangeRegion.
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

// Client wraps an embedded Grove engine. baseURL / tokenDir / tokenPath are
// retained for compatibility with diagnostic code; no HTTP traffic occurs.
type Client struct {
	baseURL   string
	tokenDir  string
	tokenPath string

	mu  sync.Mutex
	eng *groveeng.Engine
}

// NewClient returns a Client. baseURL is retained for compatibility and
// reported by diagnostic code but not used to dial anything.
func NewClient(baseURL string) *Client {
	return &Client{baseURL: baseURL}
}

// WithTokenFromDir records the project directory so EnsureRunning can open
// the engine at <dir>/.grove/grove.db. The tokenPath field is populated for
// `relay doctor` diagnostics, not for auth.
func (c *Client) WithTokenFromDir(dir string) *Client {
	if abs, err := filepath.Abs(dir); err == nil {
		c.tokenDir = abs
	} else {
		c.tokenDir = dir
	}
	c.tokenPath = filepath.Join(c.tokenDir, ".grove", "grove.db")
	return c
}

// TokenPath returns the absolute path the client opens for its on-disk index.
// `relay doctor` shows this when grove is unreachable.
func (c *Client) TokenPath() string { return c.tokenPath }

// HasToken reports whether a project directory has been bound. Auth tokens
// no longer exist in the embedded model; this is true once the index is open.
func (c *Client) HasToken() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.eng != nil
}

// EnsureRunning opens the embedded Grove engine for projectDir.
func (c *Client) EnsureRunning(projectDir string) error {
	if projectDir != "" {
		c.WithTokenFromDir(projectDir)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.eng != nil {
		return nil
	}
	if c.tokenDir == "" {
		return errors.New("grove: projectDir is required")
	}
	eng, err := groveeng.Open(context.Background(), groveeng.Config{RepoRoot: c.tokenDir})
	if err != nil {
		return fmt.Errorf("grove open: %w", err)
	}
	c.eng = eng
	return nil
}

// Health returns nil if the engine is open.
func (c *Client) Health() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.eng == nil {
		return errors.New("grove engine not open; call EnsureRunning first")
	}
	return nil
}

// Close releases the engine.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.eng != nil {
		_ = c.eng.Close()
		c.eng = nil
	}
}

func (c *Client) engine() (*groveeng.Engine, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.eng == nil {
		return nil, errors.New("grove engine not open; call EnsureRunning first")
	}
	return c.eng, nil
}

// ICR returns the legacy {SymbolCount, Rating} payload derived from the
// IsolatedChangeRegion computation. Preserved for ingestion/gs.go.
func (c *Client) ICR(intent string) (*ICRResponse, error) {
	e, err := c.engine()
	if err != nil {
		return nil, err
	}
	region := e.ICR(context.Background(), intent)
	count := len(region.ExclusiveFiles) + len(region.ReadableFiles)
	rating := "unknown"
	switch {
	case count == 0:
		rating = "empty"
	case count <= 5:
		rating = "small"
	case count <= 20:
		rating = "medium"
	default:
		rating = "large"
	}
	return &ICRResponse{SymbolCount: count, Rating: rating}, nil
}

// ICRRegion returns the full IsolatedChangeRegion for an intent description.
func (c *Client) ICRRegion(intent string) (*ICRRegion, error) {
	e, err := c.engine()
	if err != nil {
		return nil, err
	}
	r := e.ICR(context.Background(), intent)
	return &ICRRegion{
		IntentID:       r.IntentID,
		Exclusive:      r.Exclusive,
		SharedRead:     r.SharedRead,
		Boundary:       r.Boundary,
		ExclusiveFiles: r.ExclusiveFiles,
		ReadableFiles:  r.ReadableFiles,
		Confidence:     r.Confidence,
		LockKeys:       r.LockKeys,
	}, nil
}

// Impact returns the symbols affected by changes to query.
func (c *Client) Impact(query string, maxDepth int) ([]Symbol, error) {
	e, err := c.engine()
	if err != nil {
		return nil, err
	}
	syms, err := e.Impact(context.Background(), query, maxDepth)
	if err != nil {
		return nil, err
	}
	out := make([]Symbol, 0, len(syms))
	for _, s := range syms {
		out = append(out, Symbol{
			ID:            s.ID,
			Name:          s.Name,
			QualifiedName: s.QualifiedName,
			FilePath:      s.FilePath,
			Kind:          string(s.Kind),
			Language:      s.Language,
		})
	}
	return out, nil
}

// Deps returns the import/uses-type edges originating in file.
func (c *Client) Deps(file string) ([]Edge, error) {
	e, err := c.engine()
	if err != nil {
		return nil, err
	}
	edges, err := e.Deps(context.Background(), file)
	if err != nil {
		return nil, err
	}
	out := make([]Edge, 0, len(edges))
	for _, ed := range edges {
		out = append(out, Edge{From: ed.From, To: ed.To, Kind: string(ed.Type)})
	}
	return out, nil
}

// Symbols returns symbols whose qualified name matches query.
func (c *Client) Symbols(query string, limit int) ([]Symbol, error) {
	if limit <= 0 {
		limit = 20
	}
	e, err := c.engine()
	if err != nil {
		return nil, err
	}
	syms, err := e.Symbols(context.Background(), query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Symbol, 0, len(syms))
	for _, s := range syms {
		out = append(out, Symbol{
			ID:            s.ID,
			Name:          s.Name,
			QualifiedName: s.QualifiedName,
			FilePath:      s.FilePath,
			Kind:          string(s.Kind),
			Language:      s.Language,
		})
	}
	return out, nil
}
