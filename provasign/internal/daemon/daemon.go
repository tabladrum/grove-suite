// Package daemon implements the long-running laptop-mode daemon described in
// docs/architecture.md §1.5.1 and docs/laptop-mvp-audit.md §4.2.
//
// On a laptop, multiple MCP clients (Claude Code + Cursor + a pre-push hook +
// a CLI invocation) can all run against the same repo concurrently. If each
// one opened its own SQLite, intent-store git, and signer key, they would
// race. The daemon is the **single writer**: every transport (MCP stdio
// shim, CLI, git hook) proxies its requests over a Unix domain socket to the
// daemon, which serializes engine and intent operations per repo.
//
// Protocol is JSON-Lines: one JSON request per line on the wire, one JSON
// response per line in reply. Methods are namespaced by service (engine.*,
// intent.*, daemon.*). This is intentionally simpler than full JSON-RPC —
// the daemon is a local component on a trusted socket and the wire surface
// stays small.
package daemon

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/provasign/provasign/internal/intent"
	"github.com/provasign/provasign/internal/version"
)

// DefaultSocketPath returns the laptop-default Unix socket path under the
// user's home directory. Falls back to a relative path if HOME is unset
// (rare; mostly happens inside hermetic CI containers).
func DefaultSocketPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".provasign", "daemon.sock"), nil
}

// PidFilePath returns the canonical PID file path adjacent to the socket.
// Used by `relay daemon status` to detect a stale daemon (socket present,
// process dead).
func PidFilePath(sock string) string {
	return sock + ".pid"
}

// Request is the wire format. Method is dotted (e.g. "intent.open"), ID is
// echoed in the reply, Params is the per-method payload.
type Request struct {
	Method string         `json:"method"`
	ID     int            `json:"id,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}

// Response mirrors Request. Exactly one of Result or Error is set.
type Response struct {
	ID     int    `json:"id,omitempty"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Daemon holds the state every laptop transport proxies through.
//
// The intent.Service is created lazily per repo and cached, with a per-repo
// mutex so concurrent open/close calls against the same repo are serialized.
// Engine wiring is the caller's responsibility — we intentionally keep this
// package free of engine imports so the daemon can be exercised independently
// in tests.
type Daemon struct {
	socketPath string

	mu       sync.Mutex
	services map[string]*intentEntry

	listener net.Listener
	startedAt time.Time
}

type intentEntry struct {
	mu      sync.Mutex
	service *intent.Service
}

// New constructs a Daemon listening at socketPath. The socket is not opened
// yet — call Start.
func New(socketPath string) *Daemon {
	return &Daemon{
		socketPath: socketPath,
		services:   map[string]*intentEntry{},
	}
}

// Start binds the Unix socket and serves requests until ctx is cancelled or
// Stop is called. Returns nil on graceful shutdown.
func (d *Daemon) Start() error {
	if err := os.MkdirAll(filepath.Dir(d.socketPath), 0o755); err != nil {
		return fmt.Errorf("mkdir socket dir: %w", err)
	}
	// Clean any stale socket file from a previous crash.
	_ = os.Remove(d.socketPath)
	ln, err := net.Listen("unix", d.socketPath)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	if err := os.Chmod(d.socketPath, 0o600); err != nil {
		_ = ln.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}
	d.listener = ln
	d.startedAt = time.Now()

	// Write PID file so `status` can detect a stale daemon.
	pid := os.Getpid()
	if err := os.WriteFile(PidFilePath(d.socketPath), []byte(fmt.Sprintf("%d\n", pid)), 0o644); err != nil {
		return fmt.Errorf("write pidfile: %w", err)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go d.serveConn(conn)
	}
}

// Stop closes the listener (causing Start to return) and removes the socket
// + PID file. Safe to call from any goroutine; subsequent calls are no-ops.
func (d *Daemon) Stop() error {
	if d.listener == nil {
		return nil
	}
	err := d.listener.Close()
	d.listener = nil
	_ = os.Remove(d.socketPath)
	_ = os.Remove(PidFilePath(d.socketPath))
	return err
}

// serveConn processes one connection from a client (MCP shim, CLI, or hook).
// One request per line; we keep the connection open for the agent's session.
func (d *Daemon) serveConn(c net.Conn) {
	defer c.Close()
	reader := bufio.NewReader(c)
	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			return
		}
		if err != nil {
			return
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = writeResp(c, Response{Error: "parse: " + err.Error()})
			continue
		}
		resp := d.handle(req)
		resp.ID = req.ID
		if err := writeResp(c, resp); err != nil {
			return
		}
	}
}

func writeResp(c io.Writer, r Response) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = c.Write(data)
	return err
}

// handle dispatches a single request to the right service. The supported
// methods are listed in §13 of docs/design.md; daemon.* is the daemon's own
// surface (ping/stop/status), and intent.* mirrors the Auto-Intent Capture
// MCP tools.
func (d *Daemon) handle(req Request) Response {
	switch req.Method {
	case "daemon.ping":
		return Response{Result: map[string]any{
			"pong":        true,
			"version":     version.Version,
			"started_at":  d.startedAt,
			"socket_path": d.socketPath,
		}}
	case "daemon.status":
		return Response{Result: map[string]any{
			"version":    version.Version,
			"started_at": d.startedAt,
			"uptime_ns":  time.Since(d.startedAt).Nanoseconds(),
		}}
	case "daemon.stop":
		go func() {
			// Give the current response a chance to flush before we
			// kill the listener.
			time.Sleep(50 * time.Millisecond)
			_ = d.Stop()
		}()
		return Response{Result: map[string]any{"stopping": true}}
	case "intent.open":
		return d.handleIntentOpen(req.Params)
	case "intent.update":
		return d.handleIntentUpdate(req.Params)
	case "intent.close":
		return d.handleIntentClose(req.Params)
	case "intent.list":
		return d.handleIntentList(req.Params)
	default:
		return Response{Error: "unknown method: " + req.Method}
	}
}

// intentEntryFor returns the per-repo intent.Service + its serializing mutex,
// creating it on first use. The map lookup is briefly under the daemon-wide
// mutex; the per-repo mutex governs all subsequent work for that repo.
func (d *Daemon) intentEntryFor(repo string) *intentEntry {
	d.mu.Lock()
	defer d.mu.Unlock()
	abs, _ := filepath.Abs(repo)
	if e, ok := d.services[abs]; ok {
		return e
	}
	e := &intentEntry{service: intent.New(abs)}
	d.services[abs] = e
	return e
}

func (d *Daemon) handleIntentOpen(params map[string]any) Response {
	repo, _ := params["repo"].(string)
	if repo == "" {
		return Response{Error: "repo is required"}
	}
	title, _ := params["title"].(string)
	description, _ := params["description"].(string)
	if title == "" || description == "" {
		return Response{Error: "title and description are required"}
	}
	from := intent.OriginatedFrom{}
	if v, ok := params["agent"].(string); ok {
		from.Agent = v
	}
	if v, ok := params["model"].(string); ok {
		from.Model = v
	}
	if v, ok := params["conversation_ts"].(string); ok && v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from.ConversationTS = t
		}
	}
	e := d.intentEntryFor(repo)
	e.mu.Lock()
	defer e.mu.Unlock()
	i, path, err := e.service.Open(title, description, from)
	if err != nil {
		return Response{Error: err.Error()}
	}
	return Response{Result: map[string]any{
		"intent_id":  i.ID,
		"draft_path": path,
		"status":     string(i.Status),
	}}
}

func (d *Daemon) handleIntentUpdate(params map[string]any) Response {
	repo, _ := params["repo"].(string)
	id, _ := params["intent_id"].(string)
	if repo == "" || id == "" {
		return Response{Error: "repo and intent_id are required"}
	}
	patch, _ := params["patch"].(map[string]any)
	if patch == nil {
		return Response{Error: "patch is required"}
	}
	e := d.intentEntryFor(repo)
	e.mu.Lock()
	defer e.mu.Unlock()
	i, err := e.service.Update(id, patch)
	if err != nil {
		return Response{Error: err.Error()}
	}
	return Response{Result: map[string]any{
		"intent_id": i.ID,
		"title":     i.Title,
		"status":    string(i.Status),
	}}
}

func (d *Daemon) handleIntentClose(params map[string]any) Response {
	repo, _ := params["repo"].(string)
	id, _ := params["intent_id"].(string)
	if repo == "" || id == "" {
		return Response{Error: "repo and intent_id are required"}
	}
	agent, _ := params["agent"].(string)
	e := d.intentEntryFor(repo)
	e.mu.Lock()
	defer e.mu.Unlock()
	committedPath, hash, trailer, err := e.service.Close(id, agent)
	if err != nil {
		return Response{Error: err.Error()}
	}
	return Response{Result: map[string]any{
		"intent_id":      id,
		"committed_path": committedPath,
		"intent_hash":    "sha256:" + hash,
		"trailer_block":  trailer,
	}}
}

func (d *Daemon) handleIntentList(params map[string]any) Response {
	repo, _ := params["repo"].(string)
	if repo == "" {
		return Response{Error: "repo is required"}
	}
	status, _ := params["status"].(string)
	e := d.intentEntryFor(repo)
	// Listing is read-only but still goes through the mutex so it sees a
	// consistent snapshot relative to open/close in-flight on the same repo.
	e.mu.Lock()
	defer e.mu.Unlock()
	entries, err := e.service.List(status)
	if err != nil {
		return Response{Error: err.Error()}
	}
	return Response{Result: map[string]any{"intents": entries}}
}
