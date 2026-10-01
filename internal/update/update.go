// Package update installs published Motley release binaries through gh.
package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const repository = "thomashartm/motley"
const modulePath = "github.com/thomashartm/motley/cmd/motley"
const maxBinarySize = 128 << 20

type release struct {
	TagName string
	Assets  []struct{ Name string }
}

type github func(...string) ([]byte, error)

// Run checks the latest published release. Check-only never changes binaries.
func Run(current string, checkOnly bool, out io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".motley", "update-history")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	log, err := os.CreateTemp(dir, time.Now().UTC().Format("20060102T150405Z")+"-*.log")
	if err != nil {
		return err
	}
	defer func() { _ = log.Close() }()
	gh := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		data, err := exec.CommandContext(ctx, "gh", args...).CombinedOutput()
		_, _ = fmt.Fprintf(log, "gh %s\n%s\n", strings.Join(args, " "), data)
		return data, err
	}
	executable, err := os.Executable()
	if err == nil {
		err = run(current, checkOnly, executable, runtime.GOOS, runtime.GOARCH, gh, io.MultiWriter(out, log))
	}
	if err != nil {
		_, _ = fmt.Fprintf(log, "Failed: %v\n", err)
	}
	_, _ = fmt.Fprintf(out, "• History: %s\n", log.Name())
	return err
}

func run(current string, checkOnly bool, executable, goos, arch string, gh github, out io.Writer) error {
	if (goos != "darwin" && goos != "linux") || (arch != "amd64" && arch != "arm64") {
		return fmt.Errorf("unsupported release platform %s/%s", goos, arch)
	}
	data, err := gh("release", "view", "--repo", repository, "--json", "tagName,assets")
	if err != nil {
		if strings.TrimSpace(string(data)) == "release not found" {
			_, _ = fmt.Fprintln(out, "• No published release available yet")
			return nil
		}
		return fmt.Errorf("check GitHub release (install/authenticate gh if needed): %w", err)
	}
	var latest release
	if err := json.Unmarshal(data, &latest); err != nil {
		return fmt.Errorf("read release: %w", err)
	}
	latestVersion, ok := versionParts(latest.TagName)
	if !ok {
		return fmt.Errorf("unsupported release tag %q; expected vMAJOR.MINOR.PATCH", latest.TagName)
	}
	if installed, ok := versionParts(current); ok && compare(installed, latestVersion) >= 0 {
		_, _ = fmt.Fprintf(out, "• ✅ Already up to date (%s; latest %s)\n", current, latest.TagName)
		return nil
	}
	archive := fmt.Sprintf("motley_%s_%s_%s.tar.gz", strings.TrimPrefix(latest.TagName, "v"), goos, arch)
	assets := map[string]bool{}
	for _, asset := range latest.Assets {
		assets[asset.Name] = true
	}
	if !assets[archive] || !assets["checksums.txt"] {
		return fmt.Errorf("release %s needs %s and checksums.txt", latest.TagName, archive)
	}
	_, _ = fmt.Fprintf(out, "• ✅ Release %s available (installed: %s)\n", latest.TagName, current)
	if checkOnly {
		return nil
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	if name := filepath.Base(executable); name != "motley" && name != "mtly" {
		return fmt.Errorf("cannot update executable named %q; install Motley first", name)
	}
	dir := filepath.Dir(executable)
	lock, err := os.OpenFile(filepath.Join(dir, ".motley-update.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("installation directory must be writable: %w", err)
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("another update or uninstall is running: %w", err)
	}
	tmp, err := os.MkdirTemp("", "motley-update-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if _, err := gh("release", "download", latest.TagName, "--repo", repository, "--pattern", archive, "--pattern", "checksums.txt", "--dir", tmp); err != nil {
		return fmt.Errorf("download release: %w", err)
	}
	binaries, err := readArchive(tmp, archive)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "• ✅ Download and checksum verified")
	if err := install(dir, binaries, goos, arch, os.Rename); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "• ✅ Installed %s — motley / mtly\n• Restart the monitor to use the new version\n", latest.TagName)
	return nil
}

func versionParts(raw string) ([3]uint64, bool) {
	var result [3]uint64
	parts := strings.Split(strings.TrimPrefix(raw, "v"), ".")
	if len(parts) != 3 {
		return result, false
	}
	for i, part := range parts {
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return result, false
		}
		result[i] = n
	}
	return result, true
}

func compare(a, b [3]uint64) int {
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

func readArchive(dir, name string) (map[string][]byte, error) {
	checksums, err := os.ReadFile(filepath.Join(dir, "checksums.txt"))
	if err != nil {
		return nil, err
	}
	var expected []byte
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		if expected != nil {
			return nil, fmt.Errorf("duplicate checksum for %s", name)
		}
		expected, err = hex.DecodeString(fields[0])
		if err != nil || len(expected) != sha256.Size {
			return nil, fmt.Errorf("invalid checksum for %s", name)
		}
	}
	if expected == nil {
		return nil, fmt.Errorf("missing checksum for %s", name)
	}
	file, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return nil, err
	}
	if hex.EncodeToString(digest.Sum(nil)) != hex.EncodeToString(expected) {
		return nil, fmt.Errorf("checksum mismatch for %s; installed commands unchanged", name)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = compressed.Close() }()
	reader := tar.NewReader(compressed)
	binaries := map[string][]byte{}
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Name != "motley" && header.Name != "mtly" {
			continue // No paths from the archive are ever extracted to disk.
		}
		if header.Typeflag != tar.TypeReg || header.Size <= 0 || header.Size > maxBinarySize || binaries[header.Name] != nil {
			return nil, fmt.Errorf("invalid or duplicate executable %q in release", header.Name)
		}
		binaries[header.Name], err = io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
	}
	if len(binaries) != 2 {
		return nil, fmt.Errorf("release must contain both motley and mtly")
	}
	return binaries, nil
}

func verifyBinary(path, goos, arch string) error {
	info, err := buildinfo.ReadFile(path)
	if err != nil || info.Path != modulePath {
		return fmt.Errorf("not a Motley executable: %s", path)
	}
	settings := map[string]string{}
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	if settings["GOOS"] != goos || settings["GOARCH"] != arch {
		return fmt.Errorf("wrong executable platform in %s", path)
	}
	return nil
}
