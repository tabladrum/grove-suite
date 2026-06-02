//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/provasign/provasign/internal/daemon"
)

// setSid puts the child process into a new session, detaching it from the
// calling terminal. Only available on Unix.
func setSid(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// installSignalHandler stops the daemon gracefully on SIGINT/SIGTERM.
func installSignalHandler(d *daemon.Daemon) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		_ = d.Stop()
	}()
}
