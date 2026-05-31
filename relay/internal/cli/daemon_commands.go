package cli

// `relay daemon start|stop|status|restart` — manage the long-running
// laptop-mode daemon that serializes engine + intent operations across
// concurrent MCP clients, the pre-push hook, and direct CLI invocations.
//
// The daemon is optional: when it is not running, relay falls back to
// in-process engine execution (same behavior as before the daemon existed).
// The daemon is the single writer to the SQLite engine store and the
// intent-store git repo, eliminating the SQLite WAL writer-conflict and the
// git lock races that arise when multiple agents run against the same repo.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/tabladrum/grove-suite/relay/internal/daemon"
)

// RunDaemon dispatches `relay daemon <start|stop|status|restart>`.
func RunDaemon(args []string) int {
	return runDaemonIO(args, os.Stdout, os.Stderr)
}

func runDaemonIO(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: relay daemon <start|stop|status|restart>")
		return 1
	}
	switch args[0] {
	case "start":
		return cmdDaemonStart(args[1:], stdout, stderr)
	case "stop":
		return cmdDaemonStop(args[1:], stdout, stderr)
	case "status":
		return cmdDaemonStatus(args[1:], stdout, stderr)
	case "restart":
		if rc := cmdDaemonStop(args[1:], stdout, stderr); rc != 0 {
			return rc
		}
		return cmdDaemonStart(args[1:], stdout, stderr)
	case "_serve":
		// Internal subcommand: run the daemon in the foreground (detached
		// child launched by `daemon start` background mode).
		sock, err := parseDaemonSocketFlag(args[1:])
		if err != nil {
			fmt.Fprintln(stderr, "daemon _serve:", err)
			return 1
		}
		return serveDaemon(sock, stderr)
	default:
		fmt.Fprintf(stderr, "unknown daemon subcommand: %s\n", args[0])
		return 1
	}
}

// parseDaemonSocketFlag parses --socket from args, falling back to DefaultSocketPath.
func parseDaemonSocketFlag(args []string) (sock string, err error) {
	fg := flag.NewFlagSet("daemon", flag.ContinueOnError)
	sockFlag := fg.String("socket", "", "unix socket path (defaults to ~/.relay/daemon.sock)")
	_ = fg.Parse(args) // ignore unknown flags
	if *sockFlag != "" {
		return *sockFlag, nil
	}
	return daemon.DefaultSocketPath()
}

// cmdDaemonStart launches the daemon in the foreground when --fg is set, or
// re-executes this binary with `daemon _serve` (internal subcommand) as a
// background process otherwise.
func cmdDaemonStart(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("daemon start", flag.ContinueOnError)
	fg := fs.Bool("fg", false, "run in foreground (blocks until stopped)")
	sockFlag := fs.String("socket", "", "unix socket path (defaults to ~/.relay/daemon.sock)")
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}

	sock := *sockFlag
	if sock == "" {
		p, err := daemon.DefaultSocketPath()
		if err != nil {
			fmt.Fprintln(stderr, "daemon start:", err)
			return 1
		}
		sock = p
	}

	if daemon.IsRunning(sock) {
		fmt.Fprintln(stdout, "daemon is already running")
		return 0
	}

	if *fg {
		return serveDaemon(sock, stderr)
	}

	// Background mode: re-exec ourselves with `daemon _serve --socket <sock>`
	// and detach from the current process group.
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, "daemon start:", err)
		return 1
	}
	cmd := exec.Command(exe, "daemon", "_serve", "--socket", sock)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	setSid(cmd)
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(stderr, "daemon start:", err)
		return 1
	}
	// Don't wait — the child is detached.
	fmt.Fprintf(stdout, "daemon started (pid %d, socket %s)\n", cmd.Process.Pid, sock)
	return 0
}

func cmdDaemonStop(args []string, stdout, stderr *os.File) int {
	sock, err := parseDaemonSocketFlag(args)
	if err != nil {
		fmt.Fprintln(stderr, "daemon stop:", err)
		return 1
	}
	if !daemon.IsRunning(sock) {
		fmt.Fprintln(stdout, "daemon is not running")
		return 0
	}
	c, err := daemon.Dial(sock)
	if err != nil {
		fmt.Fprintln(stderr, "daemon stop:", err)
		return 1
	}
	defer c.Close()
	if _, err := c.Call("daemon.stop", nil); err != nil {
		// daemon.stop intentionally closes the socket before replying in some
		// cases; treat a short-read as success.
		fmt.Fprintln(stdout, "stop sent")
		return 0
	}
	fmt.Fprintln(stdout, "daemon stopped")
	return 0
}

func cmdDaemonStatus(args []string, stdout, stderr *os.File) int {
	sock, err := parseDaemonSocketFlag(args)
	if err != nil {
		fmt.Fprintln(stderr, "daemon status:", err)
		return 1
	}
	pidFile := daemon.PidFilePath(sock)
	pid, pidErr := daemon.PIDFromFile(sock)
	if !daemon.IsRunning(sock) {
		if pidErr == nil {
			// Socket exists but process is dead — stale PID file.
			fmt.Fprintf(stdout, "daemon: NOT RUNNING (stale pid %d at %s)\n", pid, pidFile)
		} else {
			fmt.Fprintln(stdout, "daemon: NOT RUNNING")
		}
		return 1
	}
	c, err := daemon.Dial(sock)
	if err != nil {
		fmt.Fprintln(stderr, "daemon status:", err)
		return 1
	}
	defer c.Close()
	result, err := c.Call("daemon.status", nil)
	if err != nil {
		fmt.Fprintln(stderr, "daemon status:", err)
		return 1
	}
	m, _ := result.(map[string]any)
	fmt.Fprintf(stdout, "daemon: RUNNING\n")
	fmt.Fprintf(stdout, "  version:    %v\n", m["version"])
	fmt.Fprintf(stdout, "  socket:     %s\n", sock)
	fmt.Fprintf(stdout, "  started:    %s\n", daemon.FormatStartedAt(m["started_at"]))
	if pid > 0 {
		fmt.Fprintf(stdout, "  pid:        %d\n", pid)
	}
	return 0
}

// serveDaemon is the foreground server path — blocks until interrupted.
func serveDaemon(sock string, stderr *os.File) int {
	if err := os.MkdirAll(filepath.Dir(sock), 0o755); err != nil {
		fmt.Fprintln(stderr, "daemon:", err)
		return 1
	}
	d := daemon.New(sock)
	// Handle SIGINT/SIGTERM gracefully.
	if runtime.GOOS != "windows" {
		installSignalHandler(d)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = ctx
	if err := d.Start(); err != nil {
		fmt.Fprintln(stderr, "daemon:", err)
		return 1
	}
	return 0
}
