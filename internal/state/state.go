// Package state manages motley's durable files.
package state

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func MembersDir() (string, error) {
	root := os.Getenv("XDG_STATE_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".local", "state")
	}
	return filepath.Abs(filepath.Join(root, "motley", "members"))
}

// LockSpawn serializes member lifecycle changes and identity allocation.
// The OS releases the lock even when a process crashes.
func LockSpawn(dir string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, ".spawn.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("another member lifecycle command is in progress; retry when it finishes: %w", err)
	}
	if _, err := f.WriteAt([]byte("schema = 1\n"), 0); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// WriteAtomic writes in the destination directory, syncs, then renames.
func WriteAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".manifest-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	defer func() { _ = f.Close() }()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
