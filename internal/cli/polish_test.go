package cli

import (
	"bytes"
	"context"
	"fmt"
	"nodi/internal/archive"
	"nodi/internal/config"
	"nodi/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndependentNestedSendAndIdentity(t *testing.T) {
	a, _, out, root := projectApp(t)
	ctx := context.Background()
	before, _ := os.ReadFile(filepath.Join(root, ".nodi", "project.json"))
	child := filepath.Join(root, "DXF")
	os.Mkdir(child, 0755)
	os.WriteFile(filepath.Join(child, "plate.dxf"), []byte("child content"), 0644)
	os.WriteFile(filepath.Join(child, "secret.txt"), []byte("excluded"), 0600)
	for _, args := range [][]string{{"send", child, "--name", "macstand-dxf", "--dry-run"}, {"send", child, "--name", "macstand-dxf"}, {"send", child}} {
		out.Reset()
		if e := a.Run(ctx, args); e != nil {
			t.Fatal(args, e)
		}
	}
	after, _ := os.ReadFile(filepath.Join(root, ".nodi", "project.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("parent binding changed")
	}
	if _, e := os.Stat(filepath.Join(child, ".nodi")); !os.IsNotExist(e) {
		t.Fatal("child source mutated")
	}
	s, _ := a.StoreFor("owner/private", false)
	m, e := s.Get(ctx, "macstand-dxf", "")
	if e != nil || m.Version != "0.1.0" || len(m.Files) != 1 || m.Files[0].Path != "plate.dxf" {
		t.Fatal(m, e)
	}
	h, n, _ := archive.HashFile(filepath.Join(child, "plate.dxf"))
	if m.Files[0].SHA256 != h || m.Files[0].Size != n {
		t.Fatal("SHA differs")
	}
	other := filepath.Join(t.TempDir(), "DXF")
	os.Mkdir(other, 0755)
	os.WriteFile(filepath.Join(other, "other.dxf"), []byte("other"), 0644)
	for _, args := range [][]string{{"send", other, "--name", "macstand-dxf"}, {"send", other, "--name", "macstand-dxf", "--dry-run"}, {"send", root, "--name", "macstand-dxf"}, {"send", child, "--name", "new-name"}, {"send", filepath.Join(child, "plate.dxf")}} {
		if e := a.Run(ctx, args); e == nil {
			t.Fatal("identity/partial protection bypassed", args)
		}
	}
	if e := a.Run(ctx, []string{"send", child, "--name", "demo"}); e == nil {
		t.Fatal("child used parent namespace")
	}
	// Independent directory may publish changes to its own saved name.
	os.WriteFile(filepath.Join(child, "plate.dxf"), []byte("changed child"), 0644)
	if e := a.Run(ctx, []string{"send", child}); e != nil {
		t.Fatal(e)
	}
	m, e = s.Get(ctx, "macstand-dxf", "")
	if e != nil || m.Version != "0.1.1" {
		t.Fatal(m, e)
	}
}

func TestSendDryRunReadOnlyAndOffline(t *testing.T) {
	a, mock, out, root := projectApp(t)
	ctx := context.Background()
	os.WriteFile(filepath.Join(root, "credentials.json"), []byte("private"), 0600)
	dir, _ := config.Dir()
	snapshot := func() map[string]string {
		m := map[string]string{}
		for _, base := range []string{root, dir} {
			filepath.WalkDir(base, func(p string, d os.DirEntry, e error) error {
				if e == nil && !d.IsDir() {
					b, _ := os.ReadFile(p)
					m[p] = string(b)
				}
				return e
			})
		}
		return m
	}
	before := snapshot()
	for _, offline := range []bool{false, true} {
		if offline {
			mock.Configure(func(g *testutil.GitHub) { g.ForceStatus = 503 })
		}
		out.Reset()
		if e := a.Run(ctx, []string{"send", root, "--dry-run"}); e != nil {
			t.Fatal(e)
		}
		for _, want := range []string{"Package: demo", "Files: 1", "Total size:", "credentials.json", ".nodi/", "Final contents:", "part.dxf", "SHA-256"} {
			if !strings.Contains(out.String(), want) {
				t.Fatal(want, out.String())
			}
		}
		if offline && !strings.Contains(out.String(), "NOT VERIFIED") {
			t.Fatal(out.String())
		}
	}
	if fmt.Sprint(before) != fmt.Sprint(snapshot()) {
		t.Fatal("dry-run changed source/config")
	}
	mock.Configure(func(g *testutil.GitHub) {
		for _, r := range g.Requests {
			if !strings.HasPrefix(r, "GET") {
				t.Fatal("dry-run mutated remote", r)
			}
		}
	})
}

func TestDeleteCLIConfirmationDryRunAndPreservation(t *testing.T) {
	a, mock, out, root := projectApp(t)
	ctx := context.Background()
	if e := a.Run(ctx, []string{"send", root}); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(root, "part.dxf"), []byte("second version"), 0644)
	if e := a.Run(ctx, []string{"send", root}); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(filepath.Join(root, ".nodi", "project.json"))
	count := 0
	mock.Configure(func(g *testutil.GitHub) { count = len(g.Requests) })
	out.Reset()
	if e := a.Run(ctx, []string{"delete", "demo", "--all", "--dry-run"}); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "demo/v0.1.0") || !strings.Contains(out.String(), "demo/v0.1.1") {
		t.Fatal(out.String())
	}
	mock.Configure(func(g *testutil.GitHub) {
		for _, r := range g.Requests[count:] {
			if !strings.HasPrefix(r, "GET") {
				t.Fatal(r)
			}
		}
	})
	for _, input := range []string{"", "y\n", "demo\n", "DELETE ALL other\n"} {
		a.In = strings.NewReader(input)
		if e := a.Run(ctx, []string{"delete", "demo", "--all"}); e == nil {
			t.Fatal("weak confirmation accepted", input)
		}
	}
	// Both cache scopes are invalidated after a real deletion.
	dir, _ := config.Dir()
	os.MkdirAll(filepath.Join(dir, "completion"), 0700)
	cache := filepath.Join(dir, "completion", "fixture.json")
	os.WriteFile(cache, []byte("stale"), 0600)
	a.In = strings.NewReader("demo\n")
	if e := a.Run(ctx, []string{"delete", "demo", "--version", "0.1.1"}); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(cache); !os.IsNotExist(e) {
		t.Fatal("stale completion retained")
	}
	if e := a.Run(ctx, []string{"send", root}); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "Published demo 0.1.2") {
		t.Fatal(out.String())
	}
	// A remaining lower release is the unchanged baseline, not a deleted
	// historical number. Preview must never claim a deleted version exists.
	os.WriteFile(filepath.Join(root, "part.dxf"), []byte("fictional v1"), 0644)
	a.In = strings.NewReader("demo\n")
	if e := a.Run(ctx, []string{"delete", "demo", "--version", "0.1.2"}); e != nil {
		t.Fatal(e)
	}
	out.Reset()
	if e := a.Run(ctx, []string{"send", root, "--dry-run"}); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "Version: 0.1.0 (unchanged remaining release; highest used number: 0.1.2)") {
		t.Fatal(out.String())
	}
	os.WriteFile(filepath.Join(root, "part.dxf"), []byte("second version"), 0644)
	a.In = strings.NewReader("DELETE ALL demo\n")
	if e := a.Run(ctx, []string{"delete", "demo", "--all"}); e != nil {
		t.Fatal(e)
	}
	if e := a.Run(ctx, []string{"delete", "demo", "--all", "--dry-run"}); e == nil {
		t.Fatal("repeated deletion accepted")
	}
	out.Reset()
	if e := a.Run(ctx, []string{"list"}); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(out.String(), "demo") {
		t.Fatal("deleted project listed")
	}
	if e := a.Run(ctx, []string{"info", "demo"}); e == nil {
		t.Fatal("deleted info available")
	}
	if e := a.Run(ctx, []string{"send", root}); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "Published demo 0.1.3") {
		t.Fatal(out.String())
	}
	// Source and local binding were never removed by delete. Publication records
	// naturally update last_version; project/repository identity stays intact.
	after, _ := os.ReadFile(filepath.Join(root, ".nodi", "project.json"))
	if len(before) == 0 || len(after) == 0 {
		t.Fatal("binding lost")
	}
	b, _ := os.ReadFile(filepath.Join(root, "part.dxf"))
	if string(b) != "second version" {
		t.Fatal("source lost")
	}
}

func TestUpdateCheckAndArgumentSafety(t *testing.T) {
	out := new(bytes.Buffer)
	a := New("0.3.0", strings.NewReader(""), out)
	a.LatestStable = func(context.Context) (string, error) { return "0.4.0", nil }
	if e := a.Run(context.Background(), []string{"update", "--check"}); e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{"Installed: 0.3.0", "Latest stable: 0.4.0", "Update available"} {
		if !strings.Contains(out.String(), want) {
			t.Fatal(out.String())
		}
	}
	a.LatestStable = func(context.Context) (string, error) { return "", fmt.Errorf("offline") }
	if e := a.Run(context.Background(), []string{"update", "--check"}); e == nil || !strings.Contains(out.String(), "NOT VERIFIED") {
		t.Fatal(e, out.String())
	}
	for _, args := range [][]string{{"delete", "demo"}, {"delete", "demo", "--all", "--version", "0.1.0"}, {"delete", "demo*", "--all"}, {"delete", "demo", "--version", "latest"}, {"update"}, {"send", "file", "--name", "INVALID"}} {
		if _, e := parse(args[0], args[1:]); e == nil {
			t.Fatal("unsafe syntax", args)
		}
	}
}
