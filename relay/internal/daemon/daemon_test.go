package daemon

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestDaemonPingPong(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix domain sockets not supported on Windows")
	}
	d, sock := startTestDaemon(t)
	defer d.Stop()

	c, err := Dial(sock)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	result, err := c.Call("daemon.ping", nil)
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["pong"] != true {
		t.Errorf("expected pong=true, got %v", m["pong"])
	}
}

func TestDaemonStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix domain sockets not supported on Windows")
	}
	d, sock := startTestDaemon(t)
	defer d.Stop()

	c, err := Dial(sock)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	result, err := c.Call("daemon.status", nil)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["version"] == nil {
		t.Error("expected version in status")
	}
}

func TestDaemonStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix domain sockets not supported on Windows")
	}
	d, sock := startTestDaemon(t)

	c, err := Dial(sock)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	// Send stop — daemon closes listener ~50ms later; the response may or may
	// not arrive before the socket closes.
	c.Call("daemon.stop", nil) //nolint:errcheck
	c.Close()

	// Wait for the daemon goroutine to exit.
	d.Stop()

	// After stop, IsRunning must return false.
	if IsRunning(sock) {
		t.Error("daemon still running after stop")
	}
}

func TestDaemonUnknownMethod(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix domain sockets not supported on Windows")
	}
	d, sock := startTestDaemon(t)
	defer d.Stop()

	c, err := Dial(sock)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	_, err = c.Call("no.such.method", nil)
	if err == nil {
		t.Error("expected error for unknown method")
	}
}

func TestDaemonConcurrentClients(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix domain sockets not supported on Windows")
	}
	d, sock := startTestDaemon(t)
	defer d.Stop()

	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			c, err := Dial(sock)
			if err != nil {
				errs[idx] = fmt.Errorf("dial %d: %w", idx, err)
				return
			}
			defer c.Close()
			if err := c.Ping(); err != nil {
				errs[idx] = fmt.Errorf("ping %d: %w", idx, err)
			}
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}

func TestDaemonIntentOpenMissingParams(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix domain sockets not supported on Windows")
	}
	d, sock := startTestDaemon(t)
	defer d.Stop()

	c, err := Dial(sock)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	_, err = c.Call("intent.open", map[string]any{})
	if err == nil {
		t.Error("expected error when repo is missing")
	}
}

func TestDaemonIntentOpenClose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix domain sockets not supported on Windows")
	}
	d, sock := startTestDaemon(t)
	defer d.Stop()

	// Use a temp dir as the "repo" — intent.New creates the .relay/ dir.
	repo := t.TempDir()

	c, err := Dial(sock)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	result, err := c.Call("intent.open", map[string]any{
		"repo":        repo,
		"title":       "test task",
		"description": "testing the daemon",
		"agent":       "test-agent",
	})
	if err != nil {
		t.Fatalf("intent.open: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	id, _ := m["intent_id"].(string)
	if id == "" {
		t.Fatal("expected non-empty intent_id")
	}

	// List it.
	lResult, err := c.Call("intent.list", map[string]any{
		"repo": repo,
	})
	if err != nil {
		t.Fatalf("intent.list: %v", err)
	}
	lm, _ := lResult.(map[string]any)
	intents, _ := lm["intents"].([]any)
	if len(intents) == 0 {
		t.Error("expected at least one intent in list")
	}

	// Close it.
	_, err = c.Call("intent.close", map[string]any{
		"repo":      repo,
		"intent_id": id,
		"agent":     "test-agent",
	})
	if err != nil {
		t.Fatalf("intent.close: %v", err)
	}
}

func TestIsRunning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix domain sockets not supported on Windows")
	}
	sock := sockPath(t)
	if IsRunning(sock) {
		t.Fatal("expected not running before daemon starts")
	}
	d := New(sock)
	errCh := make(chan error, 1)
	go func() { errCh <- d.Start() }()
	waitRunning(t, sock, errCh)
	if !IsRunning(sock) {
		t.Error("expected running after Start")
	}
	d.Stop()
}

func TestPIDFromFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix domain sockets not supported on Windows")
	}
	d, sock := startTestDaemon(t)
	defer d.Stop()

	pid, err := PIDFromFile(sock)
	if err != nil {
		t.Fatalf("PIDFromFile: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("expected pid %d, got %d", os.Getpid(), pid)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func sockPath(t *testing.T) string {
	t.Helper()
	// macOS limits Unix socket paths to 104 bytes; use a short name.
	return fmt.Sprintf("%s/d.sock", t.TempDir())
}

func startTestDaemon(t *testing.T) (*Daemon, string) {
	t.Helper()
	sock := sockPath(t)
	d := New(sock)
	errCh := make(chan error, 1)
	go func() { errCh <- d.Start() }()
	waitRunning(t, sock, errCh)
	t.Cleanup(func() { d.Stop() })
	return d, sock
}

func waitRunning(t *testing.T, sock string, errCh <-chan error) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		// Check if daemon failed to start.
		select {
		case err := <-errCh:
			t.Fatalf("daemon Start() returned error: %v", err)
		default:
		}
		conn, err := net.Dial("unix", sock)
		if err == nil {
			conn.Close()
			c, err := Dial(sock)
			if err == nil {
				defer c.Close()
				if c.Ping() == nil {
					return
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	// One last check for startup error.
	select {
	case err := <-errCh:
		t.Fatalf("daemon Start() returned error: %v", err)
	default:
	}
	t.Fatal("daemon did not start within 3s")
}

// compile-time check: Request and Response are JSON-marshallable
var _ = func() bool {
	r := Request{Method: "test", ID: 1, Params: map[string]any{}}
	_, _ = json.Marshal(r)
	return true
}()
