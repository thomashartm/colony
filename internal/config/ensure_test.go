package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestEnsureOnceAndMigration(t *testing.T) {
	for _, migrate := range []bool{false, true} {
		t.Run(map[bool]string{false: "defaults", true: "legacy"}[migrate], func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "custom-config"))
			want := defaultConfig
			if migrate {
				want = "# keep my comment\nrepos_root = '~/custom'\nfuture = true\n"
				dir, err := Dir()
				if err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(want), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var wg sync.WaitGroup
			for i := 0; i < 12; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if _, err := Load(); err != nil {
						t.Error(err)
					}
				}()
			}
			wg.Wait()
			path := filepath.Join(home, ".motley", "config.toml")
			data, err := os.ReadFile(path)
			if err != nil || string(data) != want {
				t.Fatal("config differs", string(data), err)
			}
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Load(); err != nil {
				t.Fatal(err)
			}
			after, err := os.Stat(path)
			if err != nil || !before.ModTime().Equal(after.ModTime()) {
				t.Fatal("existing config rewritten", err)
			}
			custom := "# intentionally invalid; never replace user data\n["
			if err := os.WriteFile(path, []byte(custom), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Ensure(); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(); err == nil {
				t.Fatal("invalid user config silently reset")
			}
			data, err = os.ReadFile(path)
			if err != nil || string(data) != custom {
				t.Fatal("user config overwritten", err)
			}
		})
	}
}
