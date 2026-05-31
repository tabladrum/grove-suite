package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Locate returns the absolute path of the named binary, preferring the
// per-installation `~/.relay/tools/bin/<name>` shim and falling back to
// `exec.LookPath` on $PATH. Returns "" when neither is available.
//
// This is the single chokepoint every Stage-2 analyzer should use: it lets
// `relay tools install` results take precedence over whatever the developer
// happens to have on $PATH without needing `eval $(relay tools shellenv)`.
func Locate(name string) string {
	root, err := DefaultRoot()
	if err == nil {
		shim := filepath.Join(root, "bin", binaryFileName(name))
		if _, err := os.Stat(shim); err == nil {
			return shim
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

// Available is a convenience for `Locate(name) != ""`.
func Available(name string) bool { return Locate(name) != "" }

// binaryFileName returns the on-disk filename of a tool's shim, accounting for
// the .exe suffix on Windows.
func binaryFileName(name string) string {
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		return name + ".exe"
	}
	return name
}
