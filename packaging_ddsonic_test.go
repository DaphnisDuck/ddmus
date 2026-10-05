package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/bjarneo/cliamp/internal/appmeta"
)

// TestDesktopInstall runs packaging/desktop.sh as the release build, the AUR
// recipe and `make install` do, and checks what a launcher then reads.
func TestDesktopInstall(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the launcher entry is installed on Linux only")
	}

	tests := []struct {
		name string
		exe  string // second argument; "" leaves it out
		want string
	}{
		// The package's binary is in /usr/bin, on every session's PATH.
		{"on PATH", "", `Exec=ddsonic`},
		// A desktop session's PATH often lacks ~/.local/bin, and a launcher
		// drops an entry whose program it can't find.
		{"absolute", "/home/duck/.local/bin/ddsonic", `Exec="/home/duck/.local/bin/ddsonic"`},
		{"needs quoting", `/home/a b/$x/50%/ddsonic`, `Exec="/home/a b/\$x/50%%/ddsonic"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest := t.TempDir()
			args := []string{"packaging/desktop.sh", dest}
			if tt.exe != "" {
				args = append(args, tt.exe)
			}
			if out, err := exec.Command("sh", args...).CombinedOutput(); err != nil {
				t.Fatalf("desktop.sh: %v\n%s", err, out)
			}

			entry, err := os.ReadFile(filepath.Join(dest, "applications", "ddsonic.desktop"))
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(entry), "\n")
			for _, want := range []string{tt.want, "Icon=ddsonic", "Terminal=true"} {
				found := false
				for _, line := range lines {
					found = found || line == want
				}
				if !found {
					t.Errorf("the entry has no line %q:\n%s", want, entry)
				}
			}

			for _, size := range []string{"16", "24", "32", "48", "64", "128", "256", "512"} {
				want, err := os.ReadFile(filepath.Join("assets", "branding", "icons", "ddsonic-"+size+".png"))
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(filepath.Join(dest, "icons", "hicolor", size+"x"+size, "apps", "ddsonic.png"))
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, want) {
					t.Errorf("the %s px icon differs from the supplied one", size)
				}
			}
		})
	}
}

// TestMakeNamesTheBinary asks make what `make install` would run. Go names a
// plain build after the module path, cliamp's, so the Makefile has to name
// the binary itself.
func TestMakeNamesTheBinary(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make is not installed")
	}

	cmd := exec.Command("make", "-n", "install")
	// Under `make test`, the outer make's variables and flags would leak in.
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch name {
		case "BINARY", "HOME", "MAKEFLAGS", "MFLAGS", "MAKELEVEL", "MAKEOVERRIDES":
		default:
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "HOME=/home/duck")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("make -n install: %v\n%s", err, out)
	}

	var built, installed bool
	for _, line := range strings.Split(string(out), "\n") {
		built = built || strings.HasPrefix(line, "go build ") && strings.HasSuffix(line, " -o ddsonic .")
		installed = installed || line == "install -m 755 ddsonic /home/duck/.local/bin/ddsonic"
	}
	if !built {
		t.Errorf("make does not build ./ddsonic:\n%s", out)
	}
	if !installed {
		t.Errorf("make does not install ~/.local/bin/ddsonic:\n%s", out)
	}
}

// TestBrandingMatchesItsManifest holds assets/branding to the package as it
// was delivered: every file is the one manifest.json lists, and nothing has
// been added or removed. The artwork is the owner's and is never edited here.
func TestBrandingMatchesItsManifest(t *testing.T) {
	dir := filepath.Join("assets", "branding")
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Files []struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) == 0 {
		t.Fatal("the manifest lists no files")
	}

	listed := map[string]bool{"manifest.json": true}
	for _, f := range manifest.Files {
		listed[f.Path] = true
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f.Path)))
		if err != nil {
			t.Errorf("%s: %v", f.Path, err)
			continue
		}
		if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != f.SHA256 {
			t.Errorf("%s differs from the delivered file", f.Path)
		}
	}
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if !listed[filepath.ToSlash(rel)] {
			t.Errorf("%s is not in the manifest", filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The MPRIS DesktopEntry names a launcher entry that is really installed.
func TestMPRISDesktopEntryIsTheInstalledOne(t *testing.T) {
	entry := filepath.Join("packaging", "linux", appmeta.DesktopEntry()+".desktop")
	if _, err := os.Stat(entry); err != nil {
		t.Errorf("appmeta.DesktopEntry() names no launcher entry: %v", err)
	}
}
