package minion

import (
	"testing"

	"github.com/thomashartm/colony/internal/tmux"
)

func TestIdentity(t *testing.T) {
	tests := []struct {
		name      string
		opts      SpawnOptions
		manifests []Manifest
		sessions  []tmux.Session
		want      string
		fail      bool
	}{
		{name: "branch", opts: SpawnOptions{Repo: "api", Branch: "feat/cache"}, want: "feat-cache"},
		{name: "ticket name", opts: SpawnOptions{Repo: "api", Branch: "feat/cache", Ticket: "412", Name: "FX cache"}, want: "412-fx-cache"},
		{name: "ticket from branch suffix", opts: SpawnOptions{Repo: "api", Branch: "feat/412-fx-cache", Ticket: "412"}, want: "412-fx-cache"},
		{name: "manifest reserves id", opts: SpawnOptions{Repo: "api", Branch: "feat/cache"}, manifests: []Manifest{{ID: "feat-cache"}}, want: "api-feat-cache"},
		{name: "unmanaged session reserves name", opts: SpawnOptions{Repo: "api", Branch: "feat/cache"}, sessions: []tmux.Session{{Name: "feat-cache"}}, want: "api-feat-cache"},
		{name: "normalized collision", opts: SpawnOptions{Repo: "api", Branch: "feat/cache.v2"}, manifests: []Manifest{{ID: "feat-cache_v2"}}, want: "api-feat-cache.v2"},
		{name: "second collision", opts: SpawnOptions{Repo: "api", Branch: "feat/cache"}, manifests: []Manifest{{ID: "feat-cache"}, {ID: "api-feat-cache"}}, fail: true},
		{name: "unsafe ticket", opts: SpawnOptions{Repo: "api", Branch: "feat/cache", Ticket: "../../escape"}, fail: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, _, err := identity(tt.opts, tt.manifests, tt.sessions)
			if (err != nil) != tt.fail || (!tt.fail && id != tt.want) {
				t.Fatalf("id=%q err=%v; want id=%q failure=%v", id, err, tt.want, tt.fail)
			}
		})
	}
}
