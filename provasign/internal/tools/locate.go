package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Locate returns the absolute path of the named binary, preferring the
// per-installation `~/.provasign/tools/bin/<name>` shim and falling back to
// `exec.LookPath` on $PATH. Returns "" when neither is available.
//
// Special names: "jre" resolves to <root>/jre/bin/java(.exe);
// "sonarlint-ls" resolves to <root>/sonar/sonarlint-ls.jar. These names
// are used by `provasign tools status` and by the sonar analyzer to verify
// the SonarLint stack is installed.
//
// This is the single chokepoint every Stage-2 analyzer should use: it lets
// `provasign tools install` results take precedence over whatever the developer
// happens to have on $PATH without needing `eval $(provasign tools shellenv)`.
func Locate(name string) string {
	root, err := DefaultRoot()
	if err == nil {
		switch name {
		case "jre":
			bin := "java"
			if runtime.GOOS == "windows" {
				bin = "java.exe"
			}
			p := filepath.Join(root, "jre-home", "bin", bin)
			if _, err := os.Stat(p); err == nil {
				return p
			}
			return ""
		case "sonarlint-ls":
			p := filepath.Join(root, "sonar", "sonarlint-ls.jar")
			if _, err := os.Stat(p); err == nil {
				return p
			}
			return ""
		}
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

// SonarPluginJars returns the absolute paths of every analyzer plugin
// jar that's been installed under <root>/sonar/analyzers/. These are
// passed to sonarlint-language-server via the `-analyzers` flag at
// startup, exactly mirroring sonarlint-vscode's launch contract.
func SonarPluginJars() []string {
	root, err := DefaultRoot()
	if err != nil {
		return nil
	}
	dir := filepath.Join(root, "sonar", "analyzers")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var jars []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jar" {
			continue
		}
		jars = append(jars, filepath.Join(dir, e.Name()))
	}
	return jars
}

// binaryFileName returns the on-disk filename of a tool's shim, accounting for
// the .exe suffix on Windows.
func binaryFileName(name string) string {
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		return name + ".exe"
	}
	return name
}
