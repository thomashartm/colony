package worktree

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CopyArtifacts ports wt's default artifact copy, including cp's native attribute
// preservation. Env-file search and artifact-directory search are separate passes.
func CopyArtifacts(source, target string, output io.Writer) error {
	source, err := filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		return err
	}
	count := 0
	for _, dirs := range []bool{false, true} {
		err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if within(target, path) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			name := entry.Name()
			if entry.IsDir() && (name == ".git" || name == "node_modules") {
				return filepath.SkipDir
			}
			if !dirs && entry.IsDir() && name == "graphify-out" {
				return filepath.SkipDir
			}
			match := entry.Type().IsRegular() && (name == ".env" || strings.HasPrefix(name, ".env."))
			if dirs {
				match = entry.IsDir() && name == "graphify-out"
			}
			if !match {
				return nil
			}
			rel, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			dest := filepath.Join(target, rel)
			if _, err := os.Lstat(dest); err == nil {
				return nil
			} else if !os.IsNotExist(err) {
				return err
			}
			if dirs && within(path, target) {
				return fmt.Errorf("cannot copy artifact %s containing the target worktree", path)
			}
			if err := makeParents(target, filepath.Dir(rel)); err != nil {
				return err
			}
			flags := "-p"
			if dirs {
				flags = "-Rp"
			}
			if out, err := exec.Command("cp", flags, path, dest).CombinedOutput(); err != nil {
				return fmt.Errorf("copy %s: %w: %s", rel, err, strings.TrimSpace(string(out)))
			}
			count++
			if dirs {
				rel += "/"
			}
			if _, err := fmt.Fprintf(output, "Copied %s\n", rel); err != nil {
				return err
			}
			if dirs {
				return filepath.SkipDir
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(output, "Copied %d local artifact(s)\n", count)
	return err
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Do not follow tracked destination symlinks out of the new worktree.
func makeParents(root, rel string) error {
	if rel == "." {
		return nil
	}
	path := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			if err := os.Mkdir(path, 0o755); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if !info.IsDir() {
			return fmt.Errorf("artifact destination parent is not a directory: %s", path)
		}
	}
	return nil
}
