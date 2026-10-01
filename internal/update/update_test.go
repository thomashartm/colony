package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var oldBinary, newBinary []byte

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "motley-update-test-")
	if err != nil {
		panic(err)
	}
	build := func() error {
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+modulePath+"\n\ngo 1.22\n"), 0600); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nimport \"fmt\"\nvar version string\nfunc main(){fmt.Println(\"motley\",version)}\n"), 0600); err != nil {
			return err
		}
		for _, version := range []string{"old", "new"} {
			path := filepath.Join(dir, version)
			cmd := exec.Command("go", "build", "-o", path, "-ldflags=-X main.version="+version, ".")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOWORK=off")
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("build fixture: %w: %s", err, out)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if version == "old" {
				oldBinary = data
			} else {
				newBinary = data
			}
		}
		return nil
	}
	if err := build(); err != nil {
		_ = os.RemoveAll(dir)
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func installed(t *testing.T, alias bool) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"motley", "mtly"} {
		if name == "mtly" && alias {
			if err := os.Symlink("motley", filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(filepath.Join(dir, name), oldBinary, 0755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func releaseArchive(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	tarball := tar.NewWriter(gz)
	for name, body := range entries {
		if err := tarball.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0755, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarball.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarball.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func fakeGitHub(t *testing.T, archive []byte, checksumOK bool, downloads *int) github {
	t.Helper()
	name := fmt.Sprintf("motley_0.9.0_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	return func(args ...string) ([]byte, error) {
		if !strings.Contains(strings.Join(args, " "), "--repo "+repository) {
			t.Fatal("wrong release repository", args)
		}
		if args[1] == "view" {
			return json.Marshal(map[string]any{"tagName": "v0.9.0", "assets": []map[string]string{{"name": name}, {"name": "checksums.txt"}}})
		}
		if args[1] != "download" || args[2] != "v0.9.0" {
			t.Fatal("unexpected gh invocation", args)
		}
		*downloads++
		dir := args[len(args)-1]
		if err := os.WriteFile(filepath.Join(dir, name), archive, 0600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(archive)
		if !checksumOK {
			digest[0] ^= 1
		}
		if err := os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(fmt.Sprintf("%x  %s\n", digest, name)), 0600); err != nil {
			t.Fatal(err)
		}
		return nil, nil
	}
}

func TestUpdateBothCommandLayouts(t *testing.T) {
	archive := releaseArchive(t, map[string][]byte{"motley": newBinary, "mtly": newBinary, "../../ignored": []byte("never extracted")})
	for _, alias := range []bool{true, false} {
		t.Run(fmt.Sprint(alias), func(t *testing.T) {
			dir := installed(t, alias)
			downloads := 0
			var output bytes.Buffer
			if err := run("local-build", false, filepath.Join(dir, "mtly"), runtime.GOOS, runtime.GOARCH, fakeGitHub(t, archive, true, &downloads), &output); err != nil {
				t.Fatal(err)
			}
			if downloads != 1 || !strings.Contains(output.String(), "Installed v0.9.0") {
				t.Fatal(downloads, output.String())
			}
			for _, name := range []string{"motley", "mtly"} {
				out, err := exec.Command(filepath.Join(dir, name), "version").CombinedOutput()
				if err != nil || string(out) != "motley new\n" {
					t.Fatal(name, string(out), err)
				}
			}
			info, err := os.Lstat(filepath.Join(dir, "mtly"))
			if err != nil || (info.Mode()&os.ModeSymlink != 0) != alias {
				t.Fatal("alias layout changed", err)
			}
		})
	}
}

func TestCheckAndNoDowngrade(t *testing.T) {
	for _, current := range []string{"0.8.0", "v0.9.0", "0.10.0"} {
		for _, checkOnly := range []bool{true, false} {
			if current == "0.8.0" && !checkOnly {
				continue
			}
			downloads := 0
			if err := run(current, checkOnly, "/not/an/installation", runtime.GOOS, runtime.GOARCH, fakeGitHub(t, nil, true, &downloads), io.Discard); err != nil || downloads != 0 {
				t.Fatal(current, checkOnly, err, downloads)
			}
		}
	}
}

func TestFailedUpdatePreservesCommands(t *testing.T) {
	for _, tc := range []struct {
		name     string
		data     map[string][]byte
		checksum bool
	}{
		{"checksum", map[string][]byte{"motley": newBinary, "mtly": newBinary}, false},
		{"invalid binary", map[string][]byte{"motley": []byte("not executable"), "mtly": newBinary}, true},
		{"missing command", map[string][]byte{"motley": newBinary}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := installed(t, false)
			downloads := 0
			err := run("0.8.0", false, filepath.Join(dir, "motley"), runtime.GOOS, runtime.GOARCH, fakeGitHub(t, releaseArchive(t, tc.data), tc.checksum, &downloads), io.Discard)
			if err == nil {
				t.Fatal("invalid release accepted")
			}
			for _, name := range []string{"motley", "mtly"} {
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || !bytes.Equal(data, oldBinary) {
					t.Fatal("installed command changed", name, err)
				}
			}
		})
	}
}

func TestInstallRollbackAndOwnership(t *testing.T) {
	dir := installed(t, false)
	calls := 0
	err := install(dir, map[string][]byte{"motley": newBinary, "mtly": newBinary}, runtime.GOOS, runtime.GOARCH, func(a, b string) error {
		calls++
		if calls == 2 {
			return errors.New("simulated second rename failure")
		}
		return os.Rename(a, b)
	})
	if err == nil || calls != 3 {
		t.Fatal(err, calls)
	}
	for _, name := range []string{"motley", "mtly"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(data, oldBinary) {
			t.Fatal("rollback lost command", err)
		}
	}
	if err := os.Remove(filepath.Join(dir, "mtly")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("another-tool", filepath.Join(dir, "mtly")); err != nil {
		t.Fatal(err)
	}
	if err := install(dir, map[string][]byte{"motley": newBinary, "mtly": newBinary}, runtime.GOOS, runtime.GOARCH, os.Rename); err == nil {
		t.Fatal("replaced unrelated alias")
	}
}

func TestGitHubFailures(t *testing.T) {
	for _, message := range []string{"release not found", "authentication failed", "network error"} {
		err := run("dev", false, "", runtime.GOOS, runtime.GOARCH, func(...string) ([]byte, error) {
			return []byte(message), errors.New("gh exited 1")
		}, io.Discard)
		if (err == nil) != (message == "release not found") {
			t.Fatal(message, err)
		}
	}
}
