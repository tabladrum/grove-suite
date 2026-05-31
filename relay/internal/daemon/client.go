package daemon

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Client speaks the daemon's JSON-Lines protocol over a Unix domain socket.
// Used by every laptop-side transport that proxies to the daemon: MCP stdio
// shims, the CLI, and the git pre-push hook.
type Client struct {
	socketPath string
	conn       net.Conn
	reader     *bufio.Reader
	id         int
}

// Dial opens a connection to the daemon. socketPath defaults to
// DefaultSocketPath(). A failed dial is the canonical signal that the daemon
// is not running; callers (e.g. CLI proxies) typically fall through to an
// in-process engine in that case.
func Dial(socketPath string) (*Client, error) {
	if socketPath == "" {
		p, err := DefaultSocketPath()
		if err != nil {
			return nil, err
		}
		socketPath = p
	}
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, err
	}
	return &Client{
		socketPath: socketPath,
		conn:       conn,
		reader:     bufio.NewReader(conn),
	}, nil
}

// Close releases the connection. Safe to call multiple times.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// Call sends one Request and reads one Response. Returns the wire-level
// error (parse/transport failure) and the application-level error (set in
// Response.Error) separately.
func (c *Client) Call(method string, params map[string]any) (any, error) {
	c.id++
	req := Request{Method: method, ID: c.id, Params: params}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if _, err := c.conn.Write(data); err != nil {
		return nil, err
	}
	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	return resp.Result, nil
}

// Ping is a convenience wrapper around `daemon.ping`. Returns the wire-level
// error only — a non-nil response from the daemon is always treated as
// success.
func (c *Client) Ping() error {
	_, err := c.Call("daemon.ping", nil)
	return err
}

// IsRunning returns whether a daemon is currently reachable at socketPath.
// It is a best-effort probe: it dials, pings, and disconnects.
func IsRunning(socketPath string) bool {
	if socketPath == "" {
		p, err := DefaultSocketPath()
		if err != nil {
			return false
		}
		socketPath = p
	}
	c, err := Dial(socketPath)
	if err != nil {
		return false
	}
	defer c.Close()
	// Give the daemon up to one second to respond before treating it as dead.
	deadline := time.Now().Add(time.Second)
	if d, ok := c.conn.(net.Conn); ok {
		_ = d.SetDeadline(deadline)
	}
	return c.Ping() == nil
}

// PIDFromFile returns the PID recorded in the daemon's pidfile, if any.
// Used by `relay daemon status` to detect a stale daemon (socket present
// but the process is gone).
func PIDFromFile(socketPath string) (int, error) {
	raw, err := os.ReadFile(PidFilePath(socketPath))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(raw)))
}

// FormatStartedAt is a small helper used by the CLI to render status output.
func FormatStartedAt(v any) string {
	if t, ok := v.(string); ok {
		if parsed, err := time.Parse(time.RFC3339Nano, t); err == nil {
			return parsed.Format(time.RFC3339)
		}
		return t
	}
	return fmt.Sprintf("%v", v)
}
