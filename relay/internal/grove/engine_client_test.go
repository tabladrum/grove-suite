package grove

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newMockGrove returns a test server that records each request body
// and replies with the JSON given by responses keyed by path.
func newMockGrove(t *testing.T, responses map[string]string) (*httptest.Server, *[]string) {
	t.Helper()
	var received []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = append(received, r.URL.Path+":"+string(body))
		resp, ok := responses[r.URL.Path]
		if !ok {
			http.Error(w, "no mock", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return srv, &received
}

func TestImpact(t *testing.T) {
	srv, recv := newMockGrove(t, map[string]string{
		"/impact": `{"nodes":[{"id":"a","qualifiedName":"pkg.Foo","filePath":"a.go","kind":"func"}]}`,
	})
	c := NewClient(srv.URL)
	nodes, err := c.Impact("Foo", 3)
	if err != nil {
		t.Fatalf("impact: %v", err)
	}
	if len(nodes) != 1 || nodes[0].QualifiedName != "pkg.Foo" {
		t.Errorf("nodes: %+v", nodes)
	}
	if !strings.Contains((*recv)[0], `"maxDepth":3`) {
		t.Errorf("expected maxDepth in body, got %s", (*recv)[0])
	}
}

func TestImpact_OmitsMaxDepthWhenZero(t *testing.T) {
	srv, recv := newMockGrove(t, map[string]string{"/impact": `{"nodes":[]}`})
	c := NewClient(srv.URL)
	if _, err := c.Impact("Foo", 0); err != nil {
		t.Fatal(err)
	}
	if strings.Contains((*recv)[0], "maxDepth") {
		t.Errorf("maxDepth should be omitted: %s", (*recv)[0])
	}
}

func TestDeps(t *testing.T) {
	srv, _ := newMockGrove(t, map[string]string{
		"/deps": `{"edges":[{"from":"a.go","to":"b.go","kind":"imports"}]}`,
	})
	c := NewClient(srv.URL)
	edges, err := c.Deps("a.go")
	if err != nil {
		t.Fatalf("deps: %v", err)
	}
	if len(edges) != 1 || edges[0].Kind != "imports" {
		t.Errorf("edges: %+v", edges)
	}
}

func TestSymbols(t *testing.T) {
	srv, recv := newMockGrove(t, map[string]string{
		"/symbols": `{"symbols":[{"id":"s1","name":"Foo"}]}`,
	})
	c := NewClient(srv.URL)
	syms, err := c.Symbols("Foo", 0) // 0 → defaults to 20
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) != 1 {
		t.Errorf("symbols: %+v", syms)
	}
	var sent map[string]any
	parts := strings.SplitN((*recv)[0], ":", 2)
	_ = json.Unmarshal([]byte(parts[1]), &sent)
	if sent["limit"].(float64) != 20 {
		t.Errorf("expected default limit=20, got %v", sent["limit"])
	}
}

func TestICRRegion(t *testing.T) {
	srv, _ := newMockGrove(t, map[string]string{
		"/icr": `{"intentId":"i1","exclusiveFiles":["a.go"],"confidence":0.9,"lockKeys":["k1"]}`,
	})
	c := NewClient(srv.URL)
	icr, err := c.ICRRegion("fix login")
	if err != nil {
		t.Fatal(err)
	}
	if icr.IntentID != "i1" || icr.Confidence != 0.9 || len(icr.LockKeys) != 1 {
		t.Errorf("icr: %+v", icr)
	}
}

func TestPost_AuthHeader(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"nodes":[]}`))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL)
	c.token = "tok-xyz"
	if _, err := c.Impact("q", 0); err != nil {
		t.Fatal(err)
	}
	if seen != "Bearer tok-xyz" {
		t.Errorf("auth header: %q", seen)
	}
}

func TestPost_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL)
	_, err := c.Impact("q", 0)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 500 error, got %v", err)
	}
}
