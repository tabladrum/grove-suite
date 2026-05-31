//go:build windows

package cli

import (
	"os/exec"

	"github.com/tabladrum/grove-suite/relay/internal/daemon"
)

// setSid is a no-op on Windows — new session creation requires a different
// mechanism (CreateProcess with CREATE_NEW_PROCESS_GROUP) that is not needed
// for the current laptop-mode use case.
func setSid(_ *exec.Cmd) {}

// installSignalHandler is a no-op on Windows.
func installSignalHandler(_ *daemon.Daemon) {}
