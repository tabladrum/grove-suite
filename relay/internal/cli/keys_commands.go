package cli

// `relay keys gen|fingerprint|export|import` — laptop-mode Ed25519 admission
// key lifecycle. The key signs every laptop cert; portability is a hard
// requirement (devs change machines) but Relay does not pretend to provide
// enterprise-grade key escrow — `export` produces a plain tarball and the
// user is told to keep it safe (e.g. age/gpg-encrypt before transport).

import (
	"archive/tar"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tabladrum/grove-suite/relay/internal/signer"
)

// RunKeys dispatches `relay keys <sub>`.
func RunKeys(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay keys <gen|fingerprint|export|import>")
		return 1
	}
	switch args[0] {
	case "gen":
		return cmdKeysGen(args[1:])
	case "fingerprint":
		return cmdKeysFingerprint(args[1:])
	case "export":
		return cmdKeysExport(args[1:])
	case "import":
		return cmdKeysImport(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown keys subcommand: %s\n", args[0])
		return 1
	}
}

// keysDir resolves the directory for the admission key:
//   - --user: ~/.relay/keys/
//   - default: <repo>/.relay/keys/ (caller's cwd by default)
func keysDir(repo string, user bool) (string, error) {
	if user {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".relay", "keys"), nil
	}
	if repo == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		repo = wd
	}
	abs, err := filepath.Abs(repo)
	if err != nil {
		return "", err
	}
	return filepath.Join(abs, ".relay", "keys"), nil
}

func cmdKeysGen(args []string) int {
	fs := flag.NewFlagSet("keys gen", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repo root (default: cwd)")
	user := fs.Bool("user", false, "store key in ~/.relay/keys/ instead of <repo>/.relay/keys/")
	force := fs.Bool("force", false, "overwrite an existing key (irreversible — old certs unverifiable)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	dir, err := keysDir(*repo, *user)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	privPath := filepath.Join(dir, "signing.ed25519.key")
	if _, err := os.Stat(privPath); err == nil && !*force {
		fmt.Fprintf(os.Stderr, "key already exists at %s (pass --force to overwrite — old certs will not verify)\n", privPath)
		return 1
	}
	if *force {
		_ = os.Remove(privPath)
		_ = os.Remove(filepath.Join(dir, "signing.ed25519.pub"))
	}
	s, err := signer.LoadOrCreateLocal(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "generate:", err)
		return 1
	}
	fmt.Printf("generated %s\n", privPath)
	fmt.Printf("key_id:      %s\n", s.KeyID())
	fmt.Printf("public_key:  %x\n", s.PublicKey())
	fmt.Println()
	fmt.Println("Keep the private key safe — it signs every laptop-mode certificate. ")
	fmt.Println("Use `relay keys export` to copy it to a new machine.")
	return 0
}

func cmdKeysFingerprint(args []string) int {
	fs := flag.NewFlagSet("keys fingerprint", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repo root")
	user := fs.Bool("user", false, "read from ~/.relay/keys/ instead of <repo>/.relay/keys/")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	dir, err := keysDir(*repo, *user)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if _, err := os.Stat(filepath.Join(dir, "signing.ed25519.key")); err != nil {
		fmt.Fprintf(os.Stderr, "no key at %s; run `relay keys gen` first\n", dir)
		return 1
	}
	s, err := signer.LoadOrCreateLocal(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("%s  (public_key=%x)\n", s.KeyID(), s.PublicKey())
	return 0
}

// cmdKeysExport writes a plain tarball containing the key pair.
//
// We intentionally don't bundle scrypt/AES here — keeping the implementation
// to stdlib and being honest about the threat model: laptop-mode export is
// a portability convenience, not a vault. The user is told to age/gpg/etc-
// encrypt the tarball before transport.
func cmdKeysExport(args []string) int {
	fs := flag.NewFlagSet("keys export", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repo root")
	user := fs.Bool("user", false, "read from ~/.relay/keys/ instead of <repo>/.relay/keys/")
	out := fs.String("out", "", "output tar path (required)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *out == "" {
		fmt.Fprintln(os.Stderr, "usage: relay keys export --out <bundle.tar> [--repo <dir>|--user]")
		return 1
	}
	dir, err := keysDir(*repo, *user)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := writeKeyTarball(dir, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("exported %s → %s\n", dir, *out)
	fmt.Println()
	fmt.Println("WARNING: the tarball contains a plaintext Ed25519 private key.")
	fmt.Println("Encrypt before transport: `age -p -o bundle.age <bundle.tar>` or `gpg -c <bundle.tar>`.")
	return 0
}

func cmdKeysImport(args []string) int {
	fs := flag.NewFlagSet("keys import", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repo root")
	user := fs.Bool("user", false, "store key in ~/.relay/keys/ instead of <repo>/.relay/keys/")
	force := fs.Bool("force", false, "overwrite an existing key")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: relay keys import <bundle.tar> [--repo <dir>|--user] [--force]")
		return 1
	}
	dir, err := keysDir(*repo, *user)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if _, err := os.Stat(filepath.Join(dir, "signing.ed25519.key")); err == nil && !*force {
		fmt.Fprintf(os.Stderr, "key already exists at %s (pass --force to overwrite)\n", dir)
		return 1
	}
	if err := readKeyTarball(fs.Arg(0), dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	s, err := signer.LoadOrCreateLocal(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("imported into %s\n", dir)
	fmt.Printf("key_id:      %s\n", s.KeyID())
	fmt.Printf("public_key:  %x\n", s.PublicKey())
	return 0
}

// writeKeyTarball packs the signing.ed25519.key + .pub files into a USTAR.
// File mode 0600 is preserved on the private key.
func writeKeyTarball(dir, outPath string) error {
	priv := filepath.Join(dir, "signing.ed25519.key")
	pub := filepath.Join(dir, "signing.ed25519.pub")
	for _, p := range []string{priv, pub} {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("missing key file %s: %w", p, err)
		}
	}
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	tw := tar.NewWriter(out)
	defer tw.Close()
	for _, p := range []string{priv, pub} {
		if err := tarAddFile(tw, p, filepath.Base(p)); err != nil {
			return err
		}
	}
	return nil
}

func tarAddFile(tw *tar.Writer, srcPath, archiveName string) error {
	info, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	hdr := &tar.Header{
		Name: archiveName,
		Mode: int64(info.Mode().Perm()),
		Size: info.Size(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(tw, f)
	return err
}

// readKeyTarball extracts signing.ed25519.{key,pub} from a USTAR tarball.
// Refuses to write any other files (defense in depth against malicious archives).
func readKeyTarball(srcPath, destDir string) error {
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return err
	}
	tr := tar.NewReader(in)
	allowed := map[string]bool{
		"signing.ed25519.key": true,
		"signing.ed25519.pub": true,
	}
	found := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		if !allowed[hdr.Name] {
			return fmt.Errorf("unexpected file in bundle: %q (allowed: signing.ed25519.{key,pub})", hdr.Name)
		}
		mode := os.FileMode(0o600)
		if hdr.Name == "signing.ed25519.pub" {
			mode = 0o644
		}
		destPath := filepath.Join(destDir, hdr.Name)
		f, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return err
		}
		f.Close()
		found++
	}
	if found < 1 {
		return errors.New("bundle contained no key files")
	}
	return nil
}
