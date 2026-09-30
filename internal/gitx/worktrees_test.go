package gitx

import "testing"

func TestWorktreePorcelain(t *testing.T) {
	out := "worktree /repo main\x00HEAD 1234567890\x00branch refs/heads/main\x00\x00worktree /tree \"quoted\"\nline\x00HEAD abcdef1234\x00detached\x00locked reason\x00\x00worktree /bare\x00bare\x00\x00"
	rows, err := ParseWorktrees(out)
	if err != nil || len(rows) != 3 {
		t.Fatal(rows, err)
	}
	if rows[0].Branch != "main" || rows[1].Path != "/tree \"quoted\"\nline" || rows[1].Branch != "" || rows[1].HEAD != "abcdef1234" || !rows[2].Bare {
		t.Fatalf("porcelain fields: %+v", rows)
	}
	if _, err := ParseWorktrees("branch refs/heads/main\x00"); err == nil {
		t.Fatal("accepted missing worktree path")
	}
}
