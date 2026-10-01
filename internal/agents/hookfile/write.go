// Package hookfile writes integration files with a backup before changes.
package hookfile

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/thomashartm/motley/internal/state"
)

func Write(path string, data []byte) (backup string, changed bool, err error) {
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", false, err
	}
	if err == nil && bytes.Equal(old, data) {
		return "", false, nil
	}
	exists := err == nil
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", false, err
	}
	if exists {
		f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".motley-v1-backup-*")
		if err != nil {
			return "", false, err
		}
		backup = f.Name()
		_, writeErr := f.Write(old)
		closeErr := f.Close()
		if writeErr != nil {
			return backup, false, writeErr
		}
		if closeErr != nil {
			return backup, false, closeErr
		}
	}
	err = state.WriteAtomic(path, data)
	return backup, err == nil, err
}
