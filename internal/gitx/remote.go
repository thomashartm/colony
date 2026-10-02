package gitx

import (
	"net/url"
	"regexp"
	"strings"
)

// Remote identifies a GitHub repository behind a git remote URL.
type Remote struct{ Web, Owner, Name string }

var githubName = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// WebURL converts scp-like, ssh:// and https GitHub remotes to web URLs. Other
// hosts and shapes report false, so callers skip GitHub features quietly.
func WebURL(remote string) (Remote, bool) {
	remote = strings.TrimSpace(remote)
	var host, path string
	if u, err := url.Parse(remote); err == nil && u.Scheme != "" {
		if u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ssh" {
			return Remote{}, false
		}
		host, path = u.Hostname(), u.Path
	} else if user, rest, ok := strings.Cut(remote, "@"); ok && user != "" && !strings.Contains(user, "/") {
		host, path, ok = strings.Cut(rest, ":")
		if !ok {
			return Remote{}, false
		}
	} else {
		return Remote{}, false
	}
	if !strings.EqualFold(host, "github.com") {
		return Remote{}, false
	}
	parts := strings.Split(strings.TrimSuffix(strings.Trim(path, "/"), ".git"), "/")
	if len(parts) != 2 || !githubName.MatchString(parts[0]) || !githubName.MatchString(parts[1]) {
		return Remote{}, false
	}
	return Remote{Web: "https://github.com/" + parts[0] + "/" + parts[1], Owner: parts[0], Name: parts[1]}, true
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
