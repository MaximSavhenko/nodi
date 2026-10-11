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

func TestParsing(t *testing.T) {
	valid := [][]string{{"version"}, {"help"}, {"list", "--all"}, {"info", "demo"}, {"get", "demo", "--version", "1.0.0", "--output", "some folder"}, {"get", "--version=1.0.0", "demo"}, {"publish", "demo", "./parts", "--version", "1.0.0", "--status", "draft"}, {"open", "demo"}, {"config", "--repo", "owner/private"}, {"auth-login", "--write"}}
	for _, args := range valid {
		if _, e := parse(args[0], args[1:]); e != nil {
			t.Fatal(args, e)
		}
	}
	invalid := [][]string{{"unknown"}, {"get"}, {"get", "../demo"}, {"get", "demo", "--version"}, {"get", "demo", "--version", "v1"}, {"get", "demo", "--force"}, {"publish", "demo", "./parts"}, {"publish", "--draft", "--status", "released"}, {"send"}, {"version", "extra"}, {"open", "demo", "--output", "x"}}
	for _, args := range invalid {
		if _, e := parse(args[0], args[1:]); e == nil {
			t.Fatal("accepted", args)
		}
	}
}
func TestMockEndToEnd(t *testing.T) {
	fixture := testutil.NewGitHub()
	defer fixture.Close()
	t.Setenv("NODI_CONFIG_DIR", t.TempDir())
	t.Setenv("NODI_REPO", "owner/private")
	var out bytes.Buffer
	app := New("0.1.0", strings.NewReader(""), &out)
	app.Store = func(write bool) (storage.Store, error) {
		token := "read-test"
		if write {
			token = "write-test"
		}
		return storage.NewGitHubWithClient("owner/private", token, fixture.Server.URL, fixture.Server.URL, fixture.Server.Client())
	}
	source := filepath.Join("..", "..", "examples", "manufacturing")
	dest := filepath.Join(t.TempDir(), "custom destination")
	run := func(args ...string) {
		t.Helper()
		out.Reset()
		if e := app.Run(context.Background(), args); e != nil {
			t.Fatal(args, e)
		}
	}
	run("publish", "demo", source, "--version", "1.0.0", "--status", "released", "--description", "Mock engineering package")
	run("list")
	if !strings.Contains(out.String(), "demo") || !strings.Contains(out.String(), "released") {
		t.Fatal(out.String())
	}
	run("info", "demo")
	if !strings.Contains(out.String(), ".dxf") || !strings.Contains(out.String(), ".pdf") {
		t.Fatal(out.String())
	}
	run("get", "demo", "--version", "1.0.0", "--output", dest)
	if !strings.Contains(out.String(), "Verified") {
		t.Fatal(out.String())
	}
	original, e := os.ReadFile(filepath.Join(dest, "plate.dxf"))
	if e != nil {
		t.Fatal(e)
	}
	if e = app.Run(context.Background(), []string{"get", "demo", "--output", dest}); e == nil {
		t.Fatal("repeat download succeeded")
	}
	after, _ := os.ReadFile(filepath.Join(dest, "plate.dxf"))
	if !bytes.Equal(original, after) {
		t.Fatal("repeat changed original")
	}
	opened := ""
	app.Open = func(p string) error { opened = p; return nil }
	run("open", "demo")
	if opened != dest {
		t.Fatal("opened wrong path", opened)
	}
	if e = app.Run(context.Background(), []string{"publish", "demo", source, "--version", "1.0.0", "--status", "released"}); e == nil {
		t.Fatal("duplicate publication succeeded")
	}
	run("publish", "draftdemo", source, "--version", "0.1.0", "--status", "draft")
	run("list")
	if strings.Contains(out.String(), "draftdemo") {
		t.Fatal("draft listed by default")
	}
	run("list", "--all")
	if !strings.Contains(out.String(), "draftdemo") {
		t.Fatal("draft missing")
	}
	if e = app.Run(context.Background(), []string{"get", "draftdemo", "--version", "0.1.0", "--output", filepath.Join(t.TempDir(), "draft")}); e == nil {
		t.Fatal("downloaded draft")
	}
	if e = app.Run(context.Background(), []string{"get", "missing", "--output", filepath.Join(t.TempDir(), "missing")}); e == nil {
		t.Fatal("missing project succeeded")
	}
	fixture.Configure(func(g *testutil.GitHub) { g.CorruptArchive = true })
	bad := filepath.Join(t.TempDir(), "corrupt")
	if e = app.Run(context.Background(), []string{"get", "demo", "--output", bad}); e == nil {
		t.Fatal("corrupt archive accepted")
	}
	if _, e = os.Stat(bad); !os.IsNotExist(e) {
		t.Fatal("left corrupt output")
	}
	if strings.Contains(out.String(), "read-test") || strings.Contains(out.String(), "write-test") {
		t.Fatal("printed token")
	}
	t.Log("PASS: TLS mock publish/list/info/versioned get/SHA-256/open callback/repeat refusal/draft isolation/duplicate refusal/missing project/corruption")
}
func TestStandaloneCommands(t *testing.T) {
	var out bytes.Buffer
	a := New("0.1.0", strings.NewReader(""), &out)
	for _, args := range [][]string{{"version"}, {"help"}, nil} {
		if e := a.Run(context.Background(), args); e != nil {
			t.Fatal(e)
		}
	}
	if !strings.Contains(out.String(), "nodi 0.1.0") {
		t.Fatal(out.String())
	}
	if e := a.Run(context.Background(), []string{"auth", "login", "--token", "secret"}); e == nil {
		t.Fatal("token argument accepted")
	}
}
