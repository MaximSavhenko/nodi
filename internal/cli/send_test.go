package cli

import (
	"bytes"
	"context"
	"nodi/internal/archive"
	"nodi/internal/config"
	"nodi/internal/model"
	"nodi/internal/storage"
	"nodi/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSendGetVersionsAndUnchanged(t *testing.T) {
	t.Setenv("NODI_CONFIG_DIR", t.TempDir())
	t.Setenv("NODI_REPO", "owner/private")
	mock := testutil.NewGitHub()
	defer mock.Close()
	out := new(bytes.Buffer)
	a := New("dev", strings.NewReader(""), out)
	a.StoreFor = func(repo string, write bool) (storage.Store, error) {
		token := "read-test"
		if write {
			token = "write-test"
		}
		return storage.NewGitHubWithClient(repo, token, mock.Server.URL, mock.Server.URL, mock.Server.Client())
	}
	source := filepath.Join(t.TempDir(), "Макет с пробелами")
	os.MkdirAll(filepath.Join(source, "Parts"), 0755)
	for _, name := range []string{"first.txt", "plate.dxf", "guide.pdf", "data.csv", "drawing.svg", "Parts/деталь.step"} {
		if e := os.WriteFile(filepath.Join(source, filepath.FromSlash(name)), []byte("fixture "+name), 0644); e != nil {
			t.Fatal(e)
		}
	}
	os.MkdirAll(filepath.Join(source, ".nodi"), 0700)
	os.WriteFile(filepath.Join(source, ".nodi", "read.credential"), []byte("must not upload"), 0600)
	os.WriteFile(filepath.Join(source, "credentials.json"), []byte("must not upload"), 0600)
	run := func(args ...string) {
		t.Helper()
		out.Reset()
		if e := a.Run(context.Background(), args); e != nil {
			t.Fatal(args, e)
		}
	}
	name := "maket-s-probelami"
	run("send", source)
	if !strings.Contains(out.String(), "Published "+name+" 0.1.0 (released)") || strings.Contains(out.String(), "[y/N]") {
		t.Fatal(out.String())
	}
	run("list")
	if !strings.Contains(out.String(), name) {
		t.Fatal(out.String())
	}
	s, _ := a.Store(false)
	first, e := s.Get(context.Background(), name, "0.1.0")
	if e != nil || len(first.Files) != 6 {
		t.Fatal(first, e)
	}
	getAndCompare := func(version string, manifest model.Manifest) string {
		t.Helper()
		dest := filepath.Join(t.TempDir(), "Получено с пробелами")
		run("get", name, "--version", version, "--output", dest)
		for _, f := range manifest.Files {
			h, n, e := archive.HashFile(filepath.Join(dest, filepath.FromSlash(f.Path)))
			if e != nil || h != f.SHA256 || n != f.Size {
				t.Fatal(f.Path, e)
			}
		}
		return dest
	}
	dest := getAndCompare("0.1.0", first)
	os.WriteFile(filepath.Join(source, "first.txt"), []byte("changed content"), 0644)
	run("send", source)
	second, e := s.Get(context.Background(), name, "")
	if e != nil || second.Version != "0.1.1" {
		t.Fatal(second, e)
	}
	getAndCompare("0.1.1", second)
	getAndCompare("0.1.0", first)
	before := 0
	mock.Configure(func(g *testutil.GitHub) { before = len(g.Requests) })
	run("send", source)
	mock.Configure(func(g *testutil.GitHub) {
		for _, request := range g.Requests[before:] {
			if !strings.HasPrefix(request, "GET") {
				t.Fatal("unchanged send wrote", request)
			}
		}
	})
	os.WriteFile(filepath.Join(dest, "first.txt"), []byte("recipient edits"), 0644)
	if e := a.Run(context.Background(), []string{"get", name, "--version", "0.1.0", "--output", dest}); e == nil {
		t.Fatal("overwrote user edits")
	}
	contents, _ := os.ReadFile(filepath.Join(dest, "first.txt"))
	if string(contents) != "recipient edits" {
		t.Fatal("lost user edits")
	}
	if _, e := os.Stat(filepath.Join(source, ".nodi", "project.json")); !os.IsNotExist(e) {
		t.Fatal("unbound send created a binding")
	}
}

func TestSendSingleFileAndBoundSource(t *testing.T) {
	a, _, out, root := projectApp(t)
	ctx := context.Background()
	if e := a.Run(ctx, []string{"send", root}); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "Published demo") {
		t.Fatal(out.String())
	}
	for _, source := range []string{filepath.Join(root, "part.dxf"), filepath.Join(root, "nested"), filepath.Join(root, "missing")} {
		if filepath.Base(source) == "nested" {
			os.Mkdir(source, 0755)
		}
		if e := a.Run(ctx, []string{"send", source}); e == nil {
			t.Fatal("accepted partial/missing bound source", source)
		}
	}
	file := filepath.Join(t.TempDir(), "Отчёт v1.txt")
	os.WriteFile(file, []byte("report"), 0644)
	out.Reset()
	if e := a.Run(ctx, []string{"send", file}); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "Published otchet-v1 0.1.0 (released)") {
		t.Fatal(out.String())
	}
}

func TestAccessSetupStoredRolesAndReadOnlyCheck(t *testing.T) {
	a, mock, out, _ := projectApp(t)
	t.Setenv("NODI_READ_TOKEN", "read-test")
	t.Setenv("NODI_WRITE_TOKEN", "write-test")
	if e := a.Run(context.Background(), []string{"setup"}); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(out.String(), "read-test") || strings.Contains(out.String(), "write-test") || !strings.Contains(out.String(), "Write capability: verified") {
		t.Fatal(out.String())
	}
	before, _ := os.ReadFile(filepath.Join(mustConfigDir(t), "config.json"))
	count := 0
	mock.Configure(func(g *testutil.GitHub) { count = len(g.Requests) })
	if e := a.Run(context.Background(), []string{"setup", "--check"}); e != nil {
		t.Fatal(e)
	}
	mock.Configure(func(g *testutil.GitHub) {
		for _, request := range g.Requests[count:] {
			if !strings.HasPrefix(request, "GET") {
				t.Fatal("check mutated remote", request)
			}
		}
	})
	after, _ := os.ReadFile(filepath.Join(mustConfigDir(t), "config.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("check wrote config")
	}
	mock.Configure(func(g *testutil.GitHub) { g.ForceStatus = 401 })
	if e := a.Run(context.Background(), []string{"setup", "--check"}); e == nil || strings.Contains(e.Error(), "read-test") {
		t.Fatal(e)
	}
}

func mustConfigDir(t *testing.T) string {
	t.Helper()
	p, e := config.Dir()
	if e != nil {
		t.Fatal(e)
	}
	return p
}

func TestInitialAccessSetupPersistsOnlyVerifiedCredentials(t *testing.T) {
	for _, token := range []string{"read-test", "invalid-test"} {
		t.Run(token, func(t *testing.T) {
			t.Setenv("NODI_CONFIG_DIR", t.TempDir())
			t.Setenv("NODI_REPO", "")
			t.Setenv("NODI_READ_TOKEN", "")
			t.Setenv("NODI_WRITE_TOKEN", "")
			mock := testutil.NewGitHub()
			defer mock.Close()
			out := new(bytes.Buffer)
			input := strings.NewReader("owner/private\n" + token + "\nremaining\n")
			a := New("dev", input, out)
			a.Connect = func(repo, token string) (storage.Store, error) {
				return storage.NewGitHubWithClient(repo, token, mock.Server.URL, mock.Server.URL, mock.Server.Client())
			}
			e := a.Run(context.Background(), []string{"setup"})
			if strings.Contains(out.String(), token) {
				t.Fatal("token leaked", out.String())
			}
			if token == "invalid-test" {
				if !storage.IsStatus(e, 401) {
					t.Fatal(e)
				}
				if _, e := config.Token(false); e == nil {
					t.Fatal("invalid credential saved")
				}
				if _, e := config.Load(); e == nil {
					t.Fatal("invalid setup saved repository")
				}
			} else {
				if e != nil {
					t.Fatal(e)
				}
				c, e := config.Load()
				if e != nil || c.Repository != "owner/private" {
					t.Fatal(c, e)
				}
				token, e := config.Token(false)
				if e != nil || token != "read-test" {
					t.Fatal("saved credential roundtrip failed")
				}
				rest := make([]byte, input.Len())
				input.Read(rest)
				if string(rest) != "remaining\n" {
					t.Fatal("setup consumed subsequent stdin", string(rest))
				}
			}
		})
	}
}

func TestPackageName(t *testing.T) {
	for input, want := range map[string]string{"MacStand": "macstand", "Mac Stand": "mac-stand", "Отчёт v1": "otchet-v1", "123": "files-123", "CON": "files-con"} {
		if got := packageName(input); got != want {
			t.Fatal(input, got, want)
		}
	}
	for _, input := range []string{"😀", strings.Repeat("a", 100), "..."} {
		if e := model.ValidateProject(packageName(input)); e != nil {
			t.Fatal(input, e)
		}
	}
}
