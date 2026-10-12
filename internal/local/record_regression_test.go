package local

import (
	"encoding/json"
	"nodi/internal/model"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRegressionB06IndependentRecords(t *testing.T) {
	for _, mode := range []string{"repositories", "destinations"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("NODI_CONFIG_DIR", t.TempDir())
			p1, p2 := t.TempDir(), t.TempDir()
			m := model.Manifest{Project: "demo", Version: "0.1.0"}
			repo2 := "owner/private"
			if mode == "repositories" {
				repo2 = "other/private"
			}
			if err := Record("owner/private", m, p1); err != nil {
				t.Fatal(err)
			}
			if err := Record(repo2, m, p2); err != nil {
				t.Fatal(err)
			}
			if mode == "destinations" {
				if err := os.Remove(p2); err != nil {
					t.Fatal(err)
				}
			}
			p, err := Latest("owner/private", "demo")
			if err != nil || p != p1 {
				t.Fatal("first independent record lost", p, err)
			}
		})
	}
}

func TestB06LegacyAndConcurrentRecords(t *testing.T) {
	t.Setenv("NODI_CONFIG_DIR", t.TempDir())
	dir := filepath.Join(os.Getenv("NODI_CONFIG_DIR"), "downloads", "demo")
	os.MkdirAll(dir, 0700)
	p := t.TempDir()
	data, _ := json.Marshal(Download{Repository: "Owner/Private", Project: "demo", Version: "0.1.0", Path: p, DownloadedAt: time.Now().UTC()})
	os.WriteFile(filepath.Join(dir, "0.1.0.json"), data, 0600)
	if got, err := Latest("owner/private", "demo"); err != nil || got != p {
		t.Fatal("legacy lookup", got, err)
	}
	var wg sync.WaitGroup
	errors := make(chan error, 12)
	paths := make([]string, 12)
	for i := range paths {
		paths[i] = t.TempDir()
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errors <- Record("owner/private", model.Manifest{Project: "demo", Version: "0.1.1"}, paths[i])
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 13 {
		t.Fatal("lost concurrent records", len(entries))
	}
}
