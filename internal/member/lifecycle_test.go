package member

import "testing"

func TestRetireRisksCountCommits(t *testing.T) {
	for ahead, want := range map[int]string{1: "1 commit not on origin/main", 3: "3 commits not on origin/main"} {
		got := RetireCheck{Dirty: true, Ahead: ahead, ComparedTo: "origin/main"}.Risks()
		if len(got) != 2 || got[0] != "uncommitted or untracked files" || got[1] != want {
			t.Errorf("ahead %d: %q", ahead, got)
		}
	}
	if got := (RetireCheck{}).Risks(); len(got) != 0 {
		t.Errorf("clean: %q", got)
	}
}
