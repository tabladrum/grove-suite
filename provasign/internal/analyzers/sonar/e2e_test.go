package sonar

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/provasign/provasign/internal/core"
)

// TestE2E_RealSonarLint exercises the full LSP pipeline against a
// fixture file. Skipped unless `provasign tools install --with-sonar` has
// run on this machine (otherwise the JRE + jars aren't present).
//
// Run with: go test ./internal/analyzers/sonar/ -run TestE2E_RealSonarLint -v -timeout 5m
func TestE2E_RealSonarLint(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e under -short")
	}
	if WrapperPath() == "" || JavaPath() == "" {
		t.Skip("sonarlint-ls or java not installed; run `provasign tools install --with-sonar`")
	}
	dir := t.TempDir()
	// Deliberately bad Java: hard-coded password (S2068) and tautological
	// comparison (S1764). One of these should always trip a Sonar rule.
	bad := `public class Bad {
    public static void main(String[] args) {
        String password = "hunter2";
        if (args == args) {
            System.out.println(password);
        }
    }
}
`
	if err := os.WriteFile(filepath.Join(dir, "Bad.java"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := New("", core.AnalyzerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	findings, err := a.Run(ctx, &core.ChangeSet{}, dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(findings) == 0 {
		t.Fatal("expected at least one finding from SonarLint, got 0")
	}
	t.Logf("got %d findings", len(findings))
	if testing.Verbose() {
		out, _ := json.MarshalIndent(findings, "", "  ")
		t.Log(string(out))
	}
}
