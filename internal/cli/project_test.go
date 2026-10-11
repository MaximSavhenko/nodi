package cli

import (
	"bytes"
	"context"
	"nodi/internal/project"
	"nodi/internal/storage"
	"nodi/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func projectApp(t *testing.T) (*App, *testutil.GitHub, *bytes.Buffer, string) {
	t.Helper()
	fixture := testutil.NewGitHub()
	t.Cleanup(fixture.Close)
	root := t.TempDir()
	t.Setenv("NODI_CONFIG_DIR", t.TempDir())
	t.Setenv("NODI_REPO", "owner/private")
	out := new(bytes.Buffer)
	a := New("0.2.0", strings.NewReader(""), out)
	a.Cwd = func() (string, error) { return root, nil }
	a.StoreFor = func(repo string, write bool) (storage.Store, error) {
		if repo != "owner/private" {
			t.Fatalf("wrong repository %s", repo)
		}
		token := "read-test"
		if write {
			token = "write-test"
		}
		return storage.NewGitHubWithClient(repo, token, fixture.Server.URL, fixture.Server.URL, fixture.Server.Client())
	}
	os.WriteFile(filepath.Join(root, "part.dxf"), []byte("fictional v1"), 0644)
	if e := a.Run(context.Background(), []string{"init", "demo"}); e != nil {
		t.Fatal(e)
	}
	return a, fixture, out, root
}
func TestAutomaticPublishStatusAndNoChanges(t *testing.T) {
	a, fixture, out, root := projectApp(t)
	ctx := context.Background()
	run := func(input string, args ...string) {
		t.Helper()
		out.Reset()
		a.In = strings.NewReader(input)
		if e := a.Run(ctx, args); e != nil {
			t.Fatal(args, e)
		}
	}
	run("", "status")
	if !strings.Contains(out.String(), "Added: 1") {
		t.Fatal(out.String())
	}
	before, _ := os.ReadFile(filepath.Join(root, ".nodi", "project.json"))
	run("", "publish")
	if !strings.Contains(out.String(), "Cancelled") {
		t.Fatal(out.String())
	}
	after, _ := os.ReadFile(filepath.Join(root, ".nodi", "project.json"))
	if string(before) != string(after) {
		t.Fatal("cancel changed local state")
	}
	run("y\n", "publish")
	if !strings.Contains(out.String(), "Published demo 0.1.0 (draft)") {
		t.Fatal(out.String())
	}
	run("", "status")
	if !strings.Contains(out.String(), "No changes detected") || !strings.Contains(out.String(), "draft") {
		t.Fatal(out.String())
	}
	count := 0
	fixture.Configure(func(g *testutil.GitHub) { count = len(g.Requests) })
	run("y\n", "publish")
	if out.String() != "No changes detected.\nNothing to publish.\n" {
		t.Fatal(out.String())
	}
	fixture.Configure(func(g *testutil.GitHub) {
		for _, r := range g.Requests[count:] {
			if strings.HasPrefix(r, "POST") {
				t.Fatal("unchanged publication wrote to GitHub", r)
			}
		}
	})
	os.WriteFile(filepath.Join(root, "part.dxf"), []byte("fictional v2"), 0644)
	run("y\nwrong\n", "publish", "--status", "released")
	if !strings.Contains(out.String(), "Cancelled") {
		t.Fatal(out.String())
	}
	run("y\nrelease demo 0.1.1\n", "publish", "--status", "released")
	if !strings.Contains(out.String(), "Published demo 0.1.1 (released)") {
		t.Fatal(out.String())
	}
	os.WriteFile(filepath.Join(root, "another.pdf"), []byte("fictional PDF"), 0644)
	os.Remove(filepath.Join(root, "part.dxf"))
	run("", "status")
	if !strings.Contains(out.String(), "Added: 1") || !strings.Contains(out.String(), "Deleted: 1") {
		t.Fatal(out.String())
	}
	run("y\n", "publish", "--minor")
	if !strings.Contains(out.String(), "Published demo 0.2.0 (draft)") {
		t.Fatal(out.String())
	}
	// Local version deliberately stale: GitHub, including drafts, controls the next version.
	b, _ := project.Find(root)
	b.SaveVersion("0.0.1")
	os.WriteFile(filepath.Join(root, "another.pdf"), []byte("fictional changed PDF"), 0644)
	run("y\n", "publish", "--major")
	if !strings.Contains(out.String(), "Published demo 1.0.0 (draft)") {
		t.Fatal(out.String())
	}
	before, _ = os.ReadFile(filepath.Join(root, ".nodi", "project.json"))
	fixture.Configure(func(g *testutil.GitHub) { g.ForceStatus = 503 })
	run("", "status")
	if !strings.Contains(out.String(), "NOT VERIFIED") || strings.Contains(out.String(), "Ready to publish") {
		t.Fatal(out.String())
	}
	if e := a.Run(ctx, []string{"publish"}); e == nil {
		t.Fatal("network failure published")
	}
	after, _ = os.ReadFile(filepath.Join(root, ".nodi", "project.json"))
	if string(before) != string(after) {
		t.Fatal("status/network error changed settings")
	}
}

type confirmReader struct {
	fired bool
	fn    func()
	input *strings.Reader
}

func (r *confirmReader) Read(p []byte) (int, error) {
	if !r.fired {
		r.fired = true
		r.fn()
	}
	return r.input.Read(p)
}
func TestRemoteConflictAfterPreview(t *testing.T) {
	a, _, _, root := projectApp(t)
	ctx := context.Background()
	other := New("0.2.0", strings.NewReader(""), new(bytes.Buffer))
	other.Store = a.Store
	other.StoreFor = a.StoreFor
	a.In = &confirmReader{input: strings.NewReader("y\n"), fn: func() {
		s, e := a.StoreFor("owner/private", true)
		if e != nil {
			t.Fatal(e)
		}
		_ = s
		other.Store = func(bool) (storage.Store, error) { return a.StoreFor("owner/private", true) }
		if e = other.Run(ctx, []string{"publish", "demo", root, "--version", "0.1.0", "--status", "draft"}); e != nil {
			t.Fatal(e)
		}
	}}
	if e := a.Run(ctx, []string{"publish"}); e == nil || !strings.Contains(e.Error(), "version conflict") {
		t.Fatal(e)
	}
	b, _ := project.Find(root)
	if b.Config.LastVersion != "" {
		t.Fatal("conflict updated local state")
	}
}
func TestChangesAfterPreview(t *testing.T) {
	a, fixture, _, root := projectApp(t)
	a.In = &confirmReader{input: strings.NewReader("y\n"), fn: func() { os.WriteFile(filepath.Join(root, "part.dxf"), []byte("changed after preview"), 0644) }}
	if e := a.Run(context.Background(), []string{"publish"}); e == nil || !strings.Contains(e.Error(), "files changed after preview") {
		t.Fatal(e)
	}
	fixture.Configure(func(g *testutil.GitHub) {
		for _, r := range g.Requests {
			if strings.HasPrefix(r, "POST") {
				t.Fatal("changed files uploaded")
			}
		}
	})
}
