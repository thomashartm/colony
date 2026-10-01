package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type replacement struct {
	target, staged, backup string
}

// Stage both commands first. Keep an installed alias symlink; replace standalone
// binaries together and restore earlier replacements if a later rename fails.
func install(dir string, binaries map[string][]byte, goos, arch string, rename func(string, string) error) error {
	var plans []replacement
	var temporary []string
	defer func() {
		for _, path := range temporary {
			_ = os.Remove(path)
		}
	}()
	stage := func(data []byte, mode os.FileMode) (string, error) {
		file, err := os.CreateTemp(dir, ".motley-update-*")
		if err != nil {
			return "", err
		}
		temporary = append(temporary, file.Name())
		defer func() { _ = file.Close() }()
		if _, err := file.Write(data); err != nil {
			return "", err
		}
		if err := file.Chmod(mode); err != nil {
			return "", err
		}
		if err := file.Sync(); err != nil {
			return "", err
		}
		return file.Name(), file.Close()
	}
	for _, name := range []string{"motley", "mtly"} {
		staged, err := stage(binaries[name], 0755)
		if err != nil {
			return err
		}
		if err := verifyBinary(staged, goos, arch); err != nil {
			return err
		}
		plan := replacement{target: filepath.Join(dir, name), staged: staged}
		info, err := os.Lstat(plan.target)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if info != nil && info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(plan.target)
			if err != nil {
				return err
			}
			if name != "mtly" || (target != "motley" && target != filepath.Join(dir, "motley")) {
				return fmt.Errorf("unrelated symlink preserved: %s", plan.target)
			}
			continue
		}
		if info != nil {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("not a regular executable: %s", plan.target)
			}
			if err := verifyBinary(plan.target, goos, arch); err != nil {
				return err
			}
			old, err := os.ReadFile(plan.target)
			if err != nil {
				return err
			}
			plan.backup, err = stage(old, info.Mode().Perm())
			if err != nil {
				return err
			}
		}
		plans = append(plans, plan)
	}
	for i, plan := range plans {
		if err := rename(plan.staged, plan.target); err != nil {
			failure := fmt.Errorf("install %s: %w", plan.target, err)
			for j := i - 1; j >= 0; j-- {
				previous := plans[j]
				var restoreErr error
				if previous.backup == "" {
					restoreErr = os.Remove(previous.target)
				} else {
					restoreErr = rename(previous.backup, previous.target)
				}
				if restoreErr != nil {
					// Keep a backup when rollback fails so manual recovery is possible.
					for k, path := range temporary {
						if path == previous.backup {
							temporary[k] = ""
						}
					}
					failure = errors.Join(failure, fmt.Errorf("restore %s from %s: %w", previous.target, previous.backup, restoreErr))
				}
			}
			return failure
		}
	}
	return nil
}
