package project

import (
	"context"
	"nodi/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestInitFindPreserve(t *testing.T) {
	root := t.TempDir()
	c := Config{Schema: 1, Project: "demo", Repository: "owner/private", Description: "kept", Metadata: map[string]string{"units": "mm"}}
	b, created, e := Init(root, c)
	if e != nil || !created {
		t.Fatal(created, e)
	}
	before, _ := os.ReadFile(filepath.Join(root, ".nodi", "project.json"))
	_, created, e = Init(root, Config{Schema: 1, Project: "demo", Repository: "owner/private"})
	if e != nil || created {
		t.Fatal(created, e)
	}
	after, _ := os.ReadFile(filepath.Join(root, ".nodi", "project.json"))
	if string(after) != string(before) {
		t.Fatal("repeat init changed settings")
	}
	if _, _, e = Init(root, Config{Schema: 1, Project: "other", Repository: "owner/private"}); e == nil {
		t.Fatal("different project overwrote settings")
	}
	nested := filepath.Join(root, "parts", "nested")
	os.MkdirAll(nested, 0755)
	found, e := Find(nested)
	if e != nil || found.Root != root || found.Config.Description != "kept" {
		t.Fatal(found, e)
	}
	os.WriteFile(filepath.Join(root, "part.dxf"), []byte("fictional"), 0644)
	os.WriteFile(filepath.Join(root, ".nodi", "ignored.dxf"), []byte("not published"), 0600)
	os.WriteFile(filepath.Join(root, "ignored.go"), []byte("ignored"), 0644)
	files, e := Scan(context.Background(), b)
	if e != nil || len(files) != 1 || files[0].Path != "part.dxf" {
		t.Fatal(files, e)
	}
	if e = b.SaveVersion("0.1.0"); e != nil {
		t.Fatal(e)
	}
	found, e = Find(nested)
	if e != nil || found.Config.LastVersion != "0.1.0" {
		t.Fatal(found, e)
	}
	os.WriteFile(filepath.Join(root, ".nodi", "project.json"), []byte(`{"schema":1,"project":"demo","repository":"owner/private","token":"secret"}`), 0600)
	if _, e = Find(nested); e == nil {
		t.Fatal("secret/unknown config field accepted")
	}
}
func TestVersions(t *testing.T) {
	for _, tt := range []struct{ current, bump, want string }{{"", "patch", "0.1.0"}, {"", "major", "0.1.0"}, {"0.1.4", "patch", "0.1.5"}, {"0.1.4", "minor", "0.2.0"}, {"0.1.4", "major", "1.0.0"}, {"1.9.9", "patch", "1.9.10"}} {
		got, e := NextVersion(tt.current, tt.bump)
		if e != nil || got != tt.want {
			t.Fatal(tt, got, e)
		}
	}
	for _, v := range []string{"broken", "999999999.0.0"} {
		bump := "patch"
		if v[0] == '9' {
			bump = "major"
		}
		if _, e := NextVersion(v, bump); e == nil {
			t.Fatal("invalid/overflow accepted", v)
		}
	}
}
func TestContentChanges(t *testing.T) {
	old := []model.File{{Path: "same", SHA256: "a", Size: 1}, {Path: "changed", SHA256: "a", Size: 1}, {Path: "deleted", SHA256: "b", Size: 1}}
	local := []model.File{{Path: "same", SHA256: "a", Size: 1}, {Path: "changed", SHA256: "z", Size: 1}, {Path: "added", SHA256: "a", Size: 1}}
	got := Diff(local, old)
	if got != (Changes{Added: 1, Modified: 1, Deleted: 1}) {
		t.Fatal(got)
	}
	if !Diff(old, old).Empty() {
		t.Fatal("unchanged")
	}
}
