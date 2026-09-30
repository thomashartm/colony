package worktree

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Compare an explicit inventory and native attributes against both a golden
// file and the unchanged artifact-copy function extracted from reference/wt.
func TestArtifactCopyParity(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source with spaces")
	mtime := time.Unix(1700000000, 0)
	write := func(root, rel, value string, mode fs.FileMode) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	for rel, value := range map[string]string{
		".env": "source", "nested/.env.local": "nested", "not.env": "skip",
		".git/.env": "skip", "node_modules/pkg/.env": "skip",
		"nested/node_modules/graphify-out/data": "skip",
		"graphify-out/data":                     "graph", "graphify-out/.env": "inside artifact",
		"graphify-out/nested/graphify-out/data":          "nested graph",
		"existing/graphify-out/new":                      "skip existing directory",
		"existing/graphify-out/nested/graphify-out/data": "nested new artifact",
		".git/graphify-out/data":                         "skip", "nested/.env.production": "executable env",
		"nested/graphify-out/index": "nested artifact",
	} {
		write(source, rel, value, 0o640)
	}
	if err := os.Chmod(filepath.Join(source, "nested/.env.production"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("data", filepath.Join(source, "graphify-out/link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".env.local", filepath.Join(source, "nested/.env.link")); err != nil {
		t.Fatal(err)
	}
	target, reference := t.TempDir(), t.TempDir()
	for _, root := range []string{target, reference} {
		write(root, ".env", "keep", 0o600)
		write(root, "existing/graphify-out/old", "keep artifact", 0o600)
	}
	var progress bytes.Buffer
	if err := CopyArtifacts(source, target, &progress); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/artifacts.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got := inventory(t, target); got != string(want) {
		t.Fatalf("inventory:\n%s\nwant:\n%s", got, want)
	}
	if !strings.Contains(progress.String(), "Copied 5 local artifact(s)") {
		t.Fatalf("unexpected progress: %s", progress.String())
	}
	script, err := os.ReadFile("../../reference/wt")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(script), "copy_local_artifacts() {")
	if start < 0 {
		t.Fatal("reference copy function not found")
	}
	end := strings.Index(string(script[start:]), "\n}\n")
	if end < 0 {
		t.Fatal("reference copy function end not found")
	}
	body := "set -eu\nCFG_ENV_GLOBS=( '.env' '.env.*' )\nCFG_ARTIFACT_DIRS=( 'graphify-out' )\nCYAN='' NC=''\nprint_info() { :; }\nprint_success() { :; }\n" + string(script[start:start+end+3]) + "\ncopy_local_artifacts \"$1\" \"$2\"\n"
	if out, err := exec.Command("bash", "-c", body, "artifact-parity", source, reference).CombinedOutput(); err != nil {
		t.Fatalf("reference: %v\n%s", err, out)
	}
	if got := inventory(t, reference); got != string(want) {
		t.Fatalf("reference differs from golden:\n%s", got)
	}
	if err := filepath.WalkDir(target, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(target, path)
		if err != nil {
			return err
		}
		actual, err := os.Stat(path)
		if err != nil {
			return err
		}
		original, err := os.Stat(filepath.Join(reference, rel))
		if err != nil {
			return err
		}
		if actual.Mode() != original.Mode() || !actual.ModTime().Equal(original.ModTime()) {
			t.Errorf("attributes differ from reference for %s", rel)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestArtifactCopyNestedTargetAndSymlinkParent(t *testing.T) {
	source := t.TempDir()
	target := filepath.Join(source, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".env"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, ".env.local"), []byte("target only"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := CopyArtifacts(source, target, &output); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "target")); !os.IsNotExist(err) {
		t.Fatalf("nested target was recopied: %v", err)
	}
	if err := os.Mkdir(filepath.Join(source, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "config/.env"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(target, "config")); err != nil {
		t.Fatal(err)
	}
	if err := CopyArtifacts(source, target, &output); err == nil {
		t.Fatal("must refuse an artifact destination with a symlink parent")
	}
	if _, err := os.Stat(filepath.Join(outside, ".env")); !os.IsNotExist(err) {
		t.Fatalf("wrote outside target: %v", err)
	}
}

func inventory(t *testing.T, root string) string {
	t.Helper()
	var out strings.Builder
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(&out, "%s -> %s\n", rel, link)
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(&out, "%s = %s\n", rel, data)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out.String()
}
