package local

import (
	"nodi/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordsAndDesktop(t *testing.T) {
	t.Setenv("NODI_CONFIG_DIR", t.TempDir())
	p1 := t.TempDir()
	p2 := t.TempDir()
	m := model.Manifest{Project: "demo", Version: "1.0.0"}
	if e := Record("owner/private", m, p1); e != nil {
		t.Fatal(e)
	}
	m.Version = "0.1.0"
	if e := Record("owner/private", m, p2); e != nil {
		t.Fatal(e)
	}
	p, e := Latest("owner/private", "demo")
	if e != nil || p != p2 {
		t.Fatal("latest download must use download time", p, e)
	}
	if _, e = Latest("other/repo", "demo"); e == nil {
		t.Fatal("cross-repository record")
	}
	if e = os.Remove(p2); e != nil {
		t.Fatal(e)
	}
	p, e = Latest("owner/private", "demo")
	if e != nil || p != p1 {
		t.Fatal("existing fallback", p, e)
	}
	desktop, e := Desktop()
	if e != nil || !filepath.IsAbs(desktop) {
		t.Fatal("Desktop resolution", desktop, e)
	}
}
