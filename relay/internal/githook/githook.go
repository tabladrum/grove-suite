// Package githook installs and uninstalls relay's pre-push hook in a local
// git repository. The hook intercepts pushes whose pending changes have not
// been certified by `relay check`, catching unmanaged agents and humans
// pushing through `git push` directly.
//
// The hook is written to `<repo>/.git/hooks/pre-push` as a POSIX shell
// script. It is idempotent: a second `Install` either no-ops when the hook
// is already managed, or fails when a foreign hook exists unless Force is
// set. `Uninstall` removes the managed hook but leaves foreign hooks
// untouched.
package githook

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ManagedMarker appears in the first comment of every relay-installed hook.
// It is used to detect whether an existing hook was installed by relay.
const ManagedMarker = "# relay-managed-hook v1"

// HookBody is the shell script written into .git/hooks/pre-push. It reads
// the per-ref push spec from stdin (git contract) and aborts when any
// pending push has not been certified. The script intentionally has no
// dependencies beyond /bin/sh + relay on $PATH.
const HookBody = `#!/bin/sh
` + ManagedMarker + `
# Auto-installed by 'relay hook install'. Do not edit manually; run
# 'relay hook install --force' to refresh or 'relay hook uninstall' to remove.
set -eu

# git passes pushed refs on stdin; capture for relay check.
push_input=$(cat || true)

if ! command -v relay >/dev/null 2>&1; then
  echo "relay: binary not in PATH; refusing push to avoid silent bypass" >&2
  exit 1
fi

# Pipe the push spec back so relay can decide which refs to certify.
printf '%s\n' "$push_input" | relay check --pre-push || exit $?
`

// HookFileName is the on-disk filename of the pre-push hook.
const HookFileName = "pre-push"

// hooksDir returns <repo>/.git/hooks, validating that the repo looks like a
// git working tree.
func hooksDir(repoRoot string) (string, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return "", errors.New("githook: empty repoRoot")
	}
	gitDir := filepath.Join(repoRoot, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return "", fmt.Errorf("githook: %s is not a git repo: %w", repoRoot, err)
	}
	if !info.IsDir() {
		// .git may be a file in worktrees; we don't support that yet.
		return "", fmt.Errorf("githook: %s/.git is not a directory", repoRoot)
	}
	dir := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// IsManaged returns true when path points at a hook script previously
// installed by relay. A missing file is not managed.
func IsManaged(path string) (bool, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return strings.Contains(string(body), ManagedMarker), nil
}

// Install writes the pre-push hook. When a non-managed hook already exists
// and force is false, Install returns an error rather than clobbering it.
// Returns the absolute path to the written file.
func Install(repoRoot string, force bool) (string, error) {
	dir, err := hooksDir(repoRoot)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, HookFileName)
	if _, err := os.Stat(path); err == nil {
		managed, mErr := IsManaged(path)
		if mErr != nil {
			return "", mErr
		}
		if !managed && !force {
			return "", fmt.Errorf("githook: %s exists and is not relay-managed; pass --force to overwrite", path)
		}
	}
	if err := os.WriteFile(path, []byte(HookBody), 0o755); err != nil {
		return "", err
	}
	return path, nil
}

// Uninstall removes the pre-push hook only when it is relay-managed.
// Returns (true, nil) when a managed hook was removed, (false, nil) when no
// hook (or a foreign hook) is present.
func Uninstall(repoRoot string) (bool, error) {
	dir, err := hooksDir(repoRoot)
	if err != nil {
		return false, err
	}
	path := filepath.Join(dir, HookFileName)
	managed, err := IsManaged(path)
	if err != nil {
		return false, err
	}
	if !managed {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
}
