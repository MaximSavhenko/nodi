package cli

import (
	"bytes"
	"context"
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
	run := func(args ...string) {
		t.Helper()
		out.Reset()
		if e := a.Run(ctx, args); e != nil {
			t.Fatal(args, e)
		}
	}
	run("status")
	if !strings.Contains(out.String(), "Added: 1") {
		t.Fatal(out.String())
	}
	run("publish") // Empty stdin must never cancel ordinary publication.
	if !strings.Contains(out.String(), "Published demo 0.1.0 (released)") {
		t.Fatal(out.String())
	}
	run("status")
	if !strings.Contains(out.String(), "No changes detected") || !strings.Contains(out.String(), "released") {
		t.Fatal(out.String())
	}
	count := 0
	fixture.Configure(func(g *testutil.GitHub) { count = len(g.Requests) })
	run("publish")
	if out.String() != "No changes detected.\nNothing to publish.\n" {
		t.Fatal(out.String())
	}
	fixture.Configure(func(g *testutil.GitHub) {
		for _, r := range g.Requests[count:] {
			if !strings.HasPrefix(r, "GET") {
				t.Fatal("unchanged publication wrote to GitHub", r)
			}
		}
	})
	os.WriteFile(filepath.Join(root, "part.dxf"), []byte("fictional v2"), 0644)
	run("publish", "--draft")
	if !strings.Contains(out.String(), "Published demo 0.1.1 (draft)") {
		t.Fatal(out.String())
	}
	// An unchanged draft needs a new consistent released version, never a flag toggle.
	run("publish")
	if !strings.Contains(out.String(), "Published demo 0.1.2 (released)") {
		t.Fatal(out.String())
	}
	os.WriteFile(filepath.Join(root, "another.pdf"), []byte("fictional PDF"), 0644)
	os.Remove(filepath.Join(root, "part.dxf"))
	run("status")
	if !strings.Contains(out.String(), "Added: 1") || !strings.Contains(out.String(), "Deleted: 1") {
		t.Fatal(out.String())
	}
	run("publish", "--minor")
	if !strings.Contains(out.String(), "Published demo 0.2.0 (released)") {
		t.Fatal(out.String())
	}
	before, _ := os.ReadFile(filepath.Join(root, ".nodi", "project.json"))
	fixture.Configure(func(g *testutil.GitHub) { g.ForceStatus = 503 })
	run("status")
	if !strings.Contains(out.String(), "NOT VERIFIED") || strings.Contains(out.String(), "Ready to publish") {
		t.Fatal(out.String())
	}
	if e := a.Run(ctx, []string{"publish"}); e == nil {
		t.Fatal("network failure published")
	}
	after, _ := os.ReadFile(filepath.Join(root, ".nodi", "project.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("network error changed settings")
	}
}

type stateHook struct {
	storage.Store
	calls int
	fn    func()
}

func (s *stateHook) State(ctx context.Context, name string) (storage.ProjectState, error) {
	s.calls++
	if s.calls == 2 {
		s.fn()
	}
	return s.Store.State(ctx, name)
}

func TestRemoteConflictDuringPackaging(t *testing.T) {
	a, _, _, root := projectApp(t)
	base := a.StoreFor
	s, _ := base("owner/private", true)
	hook := &stateHook{Store: s, fn: func() {
		other := New("dev", strings.NewReader(""), new(bytes.Buffer))
		other.Store = func(bool) (storage.Store, error) { return s, nil }
		if e := other.Run(context.Background(), []string{"publish", "demo", root, "--version", "0.1.0", "--draft"}); e != nil {
			t.Fatal(e)
		}
	}}
	a.StoreFor = func(string, bool) (storage.Store, error) { return hook, nil }
	if e := a.Run(context.Background(), []string{"publish"}); e == nil || !strings.Contains(e.Error(), "version conflict") {
		t.Fatal(e)
	}
}

func TestChangesDuringPackaging(t *testing.T) {
	a, fixture, _, root := projectApp(t)
	s, _ := a.StoreFor("owner/private", true)
	hook := &stateHook{Store: s, fn: func() { os.WriteFile(filepath.Join(root, "part.dxf"), []byte("changed after packaging"), 0644) }}
	a.StoreFor = func(string, bool) (storage.Store, error) { return hook, nil }
	if e := a.Run(context.Background(), []string{"publish"}); e == nil || !strings.Contains(e.Error(), "files changed during packaging") {
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
