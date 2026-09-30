// Package blueprint discovers and renders reusable initial agent prompts.
package blueprint

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/pelletier/go-toml/v2"
)

type Blueprint struct {
	Schema      int      `toml:"schema"`
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Agent       string   `toml:"agent"`
	Args        []string `toml:"args"`
	Repos       []string `toml:"repos"`
	Vars        []string `toml:"vars"`
	Path        string   `toml:"-"`
	Source      string   `toml:"-"`
	body        *template.Template
}
type Crew struct{ Title, URL, Kind string }
type Data struct {
	Repo, Branch, Base, Ticket, Name, Worktree string
	Crew                                       Crew
	Vars                                       map[string]string
}

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

func Parse(path string, source []byte) (Blueprint, error) {
	b := Blueprint{Schema: 1, Name: strings.TrimSuffix(filepath.Base(path), ".md"), Agent: "claude", Path: path, Source: string(source)}
	// Normalise front matter only; preserve prompt body bytes, including newlines.
	first, rest, ok := strings.Cut(string(source), "\n")
	if !ok || strings.TrimSuffix(first, "\r") != "+++" {
		return b, fmt.Errorf("%s: expected +++ TOML front matter", path)
	}
	offset := 0
	header, body := "", ""
	closed := false
	for _, line := range strings.SplitAfter(rest, "\n") {
		if strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r") == "+++" {
			header = rest[:offset]
			body = rest[offset+len(line):]
			closed = true
			break
		}
		offset += len(line)
	}
	if !closed {
		return b, fmt.Errorf("%s: missing closing +++", path)
	}
	if err := toml.Unmarshal([]byte(header), &b); err != nil {
		return b, fmt.Errorf("%s: %w", path, err)
	}
	if b.Schema != 1 {
		return b, fmt.Errorf("%s: unsupported blueprint schema %d", path, b.Schema)
	}
	if !namePattern.MatchString(b.Name) {
		return b, fmt.Errorf("%s: invalid blueprint name %q", path, b.Name)
	}
	switch b.Agent {
	case "claude", "codex", "opencode":
	default:
		return b, fmt.Errorf("%s: unknown agent %q", path, b.Agent)
	}
	for _, arg := range b.Args {
		if strings.ContainsRune(arg, 0) {
			return b, fmt.Errorf("%s: agent args cannot contain NUL", path)
		}
	}
	for _, v := range b.Vars {
		if !namePattern.MatchString(v) {
			return b, fmt.Errorf("%s: invalid variable name %q", path, v)
		}
	}
	t, err := template.New(b.Name).Option("missingkey=zero").Parse(body)
	if err != nil {
		return b, fmt.Errorf("%s: %w", path, err)
	}
	b.body = t
	return b, nil
}
func (b Blueprint) Allows(repo string) bool {
	if len(b.Repos) == 0 {
		return true
	}
	for _, r := range b.Repos {
		if r == repo {
			return true
		}
	}
	return false
}

// Discover merges global and repository files by declared name. Within one
// directory duplicate names are errors; the repository wins across directories.
// An empty repo shows global blueprints, including their repository restrictions.
func Discover(repoPath, repo string) ([]Blueprint, error) {
	root := os.Getenv("XDG_CONFIG_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(home, ".config")
	}
	dirs := []string{filepath.Join(root, "colony", "blueprints")}
	if repoPath != "" {
		dirs = append(dirs, filepath.Join(repoPath, ".colony", "blueprints"))
	}
	merged := map[string]Blueprint{}
	for _, dir := range dirs {
		paths, err := filepath.Glob(filepath.Join(dir, "*.md"))
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for _, path := range paths {
			src, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			b, err := Parse(path, src)
			if err != nil {
				return nil, err
			}
			if seen[b.Name] {
				return nil, fmt.Errorf("%s: duplicate blueprint name %q", dir, b.Name)
			}
			seen[b.Name] = true
			merged[b.Name] = b
		}
	}
	result := []Blueprint{}
	for _, b := range merged {
		if repo == "" || b.Allows(repo) {
			result = append(result, b)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
func Find(blueprints []Blueprint, name string) (Blueprint, error) {
	for _, b := range blueprints {
		if b.Name == name {
			return b, nil
		}
	}
	return Blueprint{}, fmt.Errorf("blueprint %q not found for this repository", name)
}
func Variables(values []string) (map[string]string, error) {
	result := map[string]string{}
	for _, v := range values {
		k, value, ok := strings.Cut(v, "=")
		if !ok || !namePattern.MatchString(k) {
			return nil, fmt.Errorf("invalid --var %q; use key=value", v)
		}
		result[k] = value
	}
	return result, nil
}

// Keep the single prompt argument comfortably below platform argument limits.
const MaxPromptBytes = 64 * 1024
const PromptHeader = "<!-- schema = 1 -->\n"

type promptBuffer struct{ bytes.Buffer }

func (w *promptBuffer) Write(p []byte) (int, error) {
	if w.Len()+len(p) > MaxPromptBytes {
		return 0, fmt.Errorf("rendered prompt exceeds 64 KiB")
	}
	return w.Buffer.Write(p)
}
func (b Blueprint) Render(data Data) (string, error) {
	if b.body == nil {
		return "", fmt.Errorf("blueprint is not parsed")
	}
	var out promptBuffer
	if err := b.body.Execute(&out, data); err != nil {
		return "", fmt.Errorf("%s: %w", b.Path, err)
	}
	if strings.ContainsRune(out.String(), 0) {
		return "", fmt.Errorf("%s: prompt cannot contain NUL", b.Path)
	}
	return out.String(), nil
}
func ReadPrompt(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !bytes.HasPrefix(data, []byte(PromptHeader)) {
		return "", fmt.Errorf("%s: expected prompt schema 1", path)
	}
	prompt := string(data[len(PromptHeader):])
	if len(prompt) > MaxPromptBytes || strings.ContainsRune(prompt, 0) {
		return "", fmt.Errorf("%s: invalid prompt (NUL or over 64 KiB)", path)
	}
	return prompt, nil
}
