// Minimal LSP / JSON-RPC 2.0 client over stdio, just enough to drive
// sonarlint-language-server.jar the way sonarlint-vscode does:
//
//  1. Spawn `java -jar sonarlint-ls.jar -stdio -analyzers <plugin1.jar> ...`
//  2. Send `initialize` (synchronous), then `initialized` (notification)
//  3. Send `textDocument/didOpen` for each file to scan
//  4. Drain `textDocument/publishDiagnostics` notifications until idle
//  5. Send `shutdown` then `exit`
//
// We deliberately do not depend on a general-purpose LSP library — the
// surface area we need is tiny, and inlining keeps the analyzer self-
// contained (zero new go.mod entries).
package sonar

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func stderrDebugEnabled() bool     { return os.Getenv("RELAY_SONAR_DEBUG") != "" }
func stderrDebugWriter() io.Writer { return os.Stderr }

// lspClient is a single-conversation JSON-RPC 2.0 client. Not safe for
// reuse across analyses.
type lspClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	writeMu sync.Mutex
	nextID  atomic.Int64

	pendingMu sync.Mutex
	pending   map[int64]chan *lspResponse

	diagMu  sync.Mutex
	diagBuf []publishDiagnosticsParams
	diagSig chan struct{} // closed/recreated each time a new diag arrives; lets WaitDiagnostics block

	readerDone chan struct{}
	readerErr  error
}

type lspMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *lspError        `json:"error,omitempty"`
}

type lspError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type lspResponse struct {
	Result json.RawMessage
	Err    *lspError
}

// LSP diagnostic types — only the fields we consume.
type position struct {
	Line      int `json:"line"`      // 0-based
	Character int `json:"character"` // 0-based
}

type lspRange struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type diagnostic struct {
	Range    lspRange        `json:"range"`
	Severity int             `json:"severity"` // 1=Error,2=Warning,3=Info,4=Hint
	Code     json.RawMessage `json:"code"`     // can be string or int
	Source   string          `json:"source"`
	Message  string          `json:"message"`
}

type publishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []diagnostic `json:"diagnostics"`
}

// codeString normalizes Code (string|int|null) into a printable rule id.
func (d *diagnostic) codeString() string {
	if len(d.Code) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(d.Code, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(d.Code, &n); err == nil {
		return n.String()
	}
	return strings.Trim(string(d.Code), "\"")
}

// newLSPClient starts cmd and begins reading messages. Caller must
// invoke Shutdown to clean up.
func newLSPClient(cmd *exec.Cmd) (*lspClient, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	c := &lspClient{
		cmd:        cmd,
		stdin:      stdin,
		stdout:     stdout,
		stderr:     stderr,
		pending:    make(map[int64]chan *lspResponse),
		diagSig:    make(chan struct{}),
		readerDone: make(chan struct{}),
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	// Drain stderr so the JVM doesn't block on a full pipe. Output is
	// discarded by default; set RELAY_SONAR_DEBUG=1 to mirror it to the
	// host process stderr (useful when chasing "why are there no
	// diagnostics?" bugs).
	go drainStderr(c.stderr)
	go c.readLoop()
	return c, nil
}

func drainStderr(r io.Reader) {
	debug := stderrDebugEnabled()
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 && debug {
			_, _ = fmt.Fprint(stderrDebugWriter(), "[sonarlint] ", line)
		}
		if err != nil {
			return
		}
	}
}

func (c *lspClient) readLoop() {
	defer close(c.readerDone)
	br := bufio.NewReader(c.stdout)
	for {
		msg, err := readMessage(br)
		if err != nil {
			c.readerErr = err
			// Wake up any outstanding requests so they don't hang.
			c.pendingMu.Lock()
			for id, ch := range c.pending {
				close(ch)
				delete(c.pending, id)
			}
			c.pendingMu.Unlock()
			return
		}
		c.dispatch(msg)
	}
}

func (c *lspClient) dispatch(m *lspMessage) {
	if m.ID != nil && m.Method == "" {
		// Response to one of our requests.
		var id int64
		if err := json.Unmarshal(*m.ID, &id); err == nil {
			c.pendingMu.Lock()
			ch, ok := c.pending[id]
			delete(c.pending, id)
			c.pendingMu.Unlock()
			if ok {
				ch <- &lspResponse{Result: m.Result, Err: m.Error}
				close(ch)
			}
		}
		return
	}
	// Notification or server-to-client request.
	if stderrDebugEnabled() && m.Method != "textDocument/publishDiagnostics" {
		if m.Method == "window/logMessage" || m.Method == "window/showMessage" {
			var p struct {
				Type    int    `json:"type"`
				Message string `json:"message"`
			}
			_ = json.Unmarshal(m.Params, &p)
			fmt.Fprintf(stderrDebugWriter(), "[sonarlint] log[%d] %s\n", p.Type, p.Message)
		} else {
			fmt.Fprintf(stderrDebugWriter(), "[sonarlint] ← %s %s\n", m.Method, string(m.Params))
		}
	}
	switch m.Method {
	case "textDocument/publishDiagnostics":
		var p publishDiagnosticsParams
		if err := json.Unmarshal(m.Params, &p); err == nil {
			c.diagMu.Lock()
			c.diagBuf = append(c.diagBuf, p)
			close(c.diagSig)
			c.diagSig = make(chan struct{})
			c.diagMu.Unlock()
		}
	default:
		if m.ID != nil {
			c.writeResponse(*m.ID, sonarRequestReply(m.Method, m.Params))
		}
	}
}

// sonarRequestReply produces a sensible default reply for the
// server→client requests sonarlint-language-server makes during a scan.
// Returning bare null (the previous behaviour) made the server treat
// every file as ignored, so no diagnostics ever fired.
//
// Method names come from sonarlint-vscode's ExtendedClient protocol —
// we match by suffix so future renames don't silently break.
func sonarRequestReply(method string, params json.RawMessage) any {
	short := method
	if i := strings.LastIndex(short, "/"); i >= 0 {
		short = short[i+1:]
	}
	switch {
	case strings.EqualFold(short, "shouldAnalyseFile"),
		strings.EqualFold(short, "shouldAnalyseFileCheck"),
		strings.EqualFold(short, "isOpenInEditor"),
		strings.EqualFold(short, "canShowMissingRequirementNotification"):
		return true
	case strings.EqualFold(short, "scmCheck"),
		strings.HasPrefix(strings.ToLower(short), "isignoredbyscm"),
		strings.EqualFold(short, "hasJoinedIdeLabs"):
		return false
	case strings.EqualFold(short, "filterOutExcludedFiles"):
		// Echo the input fileUris back unchanged → nothing excluded.
		var in struct {
			FileUris []string `json:"fileUris"`
		}
		_ = json.Unmarshal(params, &in)
		return map[string]any{"fileUris": in.FileUris}
	case strings.EqualFold(short, "listFilesInFolder"):
		return map[string]any{"foundFiles": []any{}}
	case strings.EqualFold(short, "getTokenForServer"):
		return ""
	case strings.EqualFold(short, "getJavaConfig"):
		// Returning nil here is the correct "I have no java
		// classpath info" signal; SonarLint falls back to source-
		// only analysis, which is still enough for our Java rules.
		return nil
	}
	return nil
}

// readMessage parses one Content-Length-framed JSON-RPC message.
func readMessage(br *bufio.Reader) (*lspMessage, error) {
	contentLen := -1
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			n, err := strconv.Atoi(strings.TrimSpace(line[len("Content-Length:"):]))
			if err != nil {
				return nil, fmt.Errorf("bad Content-Length: %q", line)
			}
			contentLen = n
		}
	}
	if contentLen < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}
	body := make([]byte, contentLen)
	if _, err := io.ReadFull(br, body); err != nil {
		return nil, err
	}
	var m lspMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("decode JSON-RPC: %w (body=%s)", err, string(body))
	}
	return &m, nil
}

func (c *lspClient) writeRaw(payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return err
	}
	_, err := c.stdin.Write(payload)
	return err
}

func (c *lspClient) writeResponse(id json.RawMessage, result any) {
	payload, _ := json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result"`
	}{"2.0", id, result})
	_ = c.writeRaw(payload)
}

// request sends a JSON-RPC request and blocks until a response arrives
// or ctx is cancelled.
func (c *lspClient) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan *lspResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	payload, err := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int64  `json:"id"`
		Method  string `json:"method"`
		Params  any    `json:"params,omitempty"`
	}{"2.0", id, method, params})
	if err != nil {
		return nil, err
	}
	if err := c.writeRaw(payload); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	case resp, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("lsp connection closed before reply to %s", method)
		}
		if resp.Err != nil {
			return nil, fmt.Errorf("%s: %s (%d)", method, resp.Err.Message, resp.Err.Code)
		}
		return resp.Result, nil
	}
}

// notify sends a fire-and-forget notification.
func (c *lspClient) notify(method string, params any) error {
	payload, err := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params,omitempty"`
	}{"2.0", method, params})
	if err != nil {
		return err
	}
	return c.writeRaw(payload)
}

// drainDiagnostics waits until no new diagnostics arrive for `idle`
// duration, or until the absolute deadline `maxWait` from the call.
// Returns every PublishDiagnostics observed so far (resetting the
// internal buffer).
func (c *lspClient) drainDiagnostics(idle, maxWait time.Duration) []publishDiagnosticsParams {
	deadline := time.Now().Add(maxWait)
	for {
		c.diagMu.Lock()
		sig := c.diagSig
		c.diagMu.Unlock()

		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		wait := idle
		if remaining < wait {
			wait = remaining
		}
		select {
		case <-sig:
			// More diagnostics arrived; loop and wait for the next idle window.
			continue
		case <-time.After(wait):
			// Idle window elapsed with no new diagnostics → done.
			c.diagMu.Lock()
			out := c.diagBuf
			c.diagBuf = nil
			c.diagMu.Unlock()
			return out
		case <-c.readerDone:
			c.diagMu.Lock()
			out := c.diagBuf
			c.diagBuf = nil
			c.diagMu.Unlock()
			return out
		}
	}
	c.diagMu.Lock()
	out := c.diagBuf
	c.diagBuf = nil
	c.diagMu.Unlock()
	return out
}

// shutdown sends `shutdown` + `exit` and waits for the JVM to exit.
func (c *lspClient) shutdown(ctx context.Context) error {
	_, _ = c.request(ctx, "shutdown", nil)
	_ = c.notify("exit", nil)
	_ = c.stdin.Close()
	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()
	select {
	case <-time.After(5 * time.Second):
		_ = c.cmd.Process.Kill()
		<-done
		return nil
	case err := <-done:
		// `exit` results in JVM returning code 0 if shutdown was
		// called first; treat anything as success.
		_ = err
		return nil
	}
}
