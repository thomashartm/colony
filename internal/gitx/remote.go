package gitx

import (
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// Remote identifies a GitHub repository behind a git remote URL.
type Remote struct{ Web, Owner, Name string }

var githubName = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// Web converts a git remote in scp-like (user@host:path), ssh:// or http(s) form
// to the repository's https web URL. Local paths, other schemes, credentials in
// http(s) remotes, control characters and paths without owner and name report false.
func Web(remote string) (*url.URL, bool) {
	remote = strings.TrimSpace(remote)
	if strings.IndexFunc(remote, unicode.IsControl) >= 0 {
		return nil, false
	}
	var host, path string
	if u, err := url.Parse(remote); err == nil && u.Scheme != "" {
		switch u.Scheme {
		case "https", "http":
			if u.User != nil {
				return nil, false
			}
		case "ssh":
		default:
			return nil, false
		}
		host, path = u.Hostname(), u.Path
	} else if user, rest, ok := strings.Cut(remote, "@"); ok && user != "" && !strings.Contains(user, "/") {
		if host, path, ok = strings.Cut(rest, ":"); !ok || strings.Contains(host, "/") {
			return nil, false
		}
	} else {
		return nil, false
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	parts := strings.Split(path, "/")
	if host == "" || len(parts) < 2 || slices.Contains(parts, "") {
		return nil, false
	}
	return &url.URL{Scheme: "https", Host: strings.ToLower(host), Path: "/" + path}, true
}

// WebURL identifies a GitHub repository remote; other hosts report false, so
// callers skip GitHub features quietly.
func WebURL(remote string) (Remote, bool) {
	u, ok := Web(remote)
	if !ok || u.Host != "github.com" {
		return Remote{}, false
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 2 || !githubName.MatchString(parts[0]) || !githubName.MatchString(parts[1]) {
		return Remote{}, false
	}
	return Remote{Web: u.String(), Owner: parts[0], Name: parts[1]}, true
}

// BranchURL is the GitHub tree view of branch.
func (r Remote) BranchURL(branch string) string { return r.Web + "/tree/" + escapeRef(branch) }

// CompareURL is the GitHub compare view of branch against base.
func (r Remote) CompareURL(base, branch string) string {
	return r.Web + "/compare/" + escapeRef(base) + "..." + escapeRef(branch)
}

// escapeRef keeps ref slashes as path separators and escapes everything else.
func escapeRef(ref string) string {
	parts := strings.Split(ref, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}
