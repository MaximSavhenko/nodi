package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"nodi/internal/config"
	"nodi/internal/storage"
	"nodi/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStaticCompletionAndAliases(t *testing.T) {
	var out bytes.Buffer
	a := New("0.2.0", strings.NewReader(""), &out)
	for _, tt := range []struct {
		words []string
		want  string
	}{{[]string{"pub"}, "publish\n"}, {[]string{"auth", "lo"}, "login\nlogout\n"}, {[]string{"get", "demo", "--o"}, "--output\n"}, {[]string{"publish", "--status", "r"}, "released\n"}, {[]string{"publish", "--status=d"}, "--status=draft\n"}, {[]string{"get", "demo", "--output", "a"}, ":files\n"}, {[]string{"publish", "demo", ""}, ":files\n"}, {[]string{"completion", "z"}, "zsh\n"}} {
		out.Reset()
		if e := a.Run(context.Background(), append([]string{"__complete"}, tt.words...)); e != nil || out.String() != tt.want {
			t.Fatal(tt, out.String(), e)
		}
	}
	for _, cmd := range []string{"version", "-v", "--version"} {
		out.Reset()
		if e := a.Run(context.Background(), []string{cmd}); e != nil || !strings.Contains(out.String(), "nodi 0.2.0 ") {
			t.Fatal(cmd, out.String(), e)
		}
	}
	for _, shell := range []string{"bash", "zsh", "powershell"} {
		out.Reset()
		if e := a.Run(context.Background(), []string{"completion", shell}); e != nil || !strings.Contains(out.String(), "__complete") {
			t.Fatal(shell, e)
		}
	}
	out.Reset()
	a.In = strings.NewReader(`["get","demo","--output",""]`)
	if e := a.Run(context.Background(), []string{"__complete", "--json"}); e != nil || out.String() != ":files\n" {
		t.Fatal(out.String(), e)
	}
}
func TestDynamicCompletionCacheAndOffline(t *testing.T) {
	a, fixture, out, root := projectApp(t)
	t.Setenv("NODI_READ_TOKEN", "")
	t.Setenv("NODI_WRITE_TOKEN", "")
	if e := config.SaveToken("write-test", true); e != nil {
		t.Fatal(e)
	}
	a.In = strings.NewReader("y\n")
	if e := a.Run(context.Background(), []string{"publish"}); e != nil {
		t.Fatal(e)
	}
	complete := func(words ...string) string {
		out.Reset()
		if e := a.Run(context.Background(), append([]string{"__complete"}, words...)); e != nil {
			t.Fatal(e)
		}
		return out.String()
	}
	if got := complete("info", "--all", "d"); got != "demo\n" {
		t.Fatal(got)
	}
	if got := complete("publish", "demo", "--version", ""); got != "0.1.0\n" {
		t.Fatal(got)
	}
	if got := complete("publish", "--version", "0.1."); got != "0.1.0\n" {
		t.Fatal("implicit project version", got)
	}
	dir, _ := config.Dir()
	cacheFiles, _ := filepath.Glob(filepath.Join(dir, "completion", "*.json"))
	if len(cacheFiles) != 1 {
		t.Fatal(cacheFiles)
	}
	var c completionCache
	data, _ := os.ReadFile(cacheFiles[0])
	json.Unmarshal(data, &c)
	if strings.Contains(string(data), "write-test") {
		t.Fatal("credential in cache")
	}
	// Fresh cache avoids any API traffic; stale cache survives a network outage.
	count := 0
	fixture.Configure(func(g *testutil.GitHub) { count = len(g.Requests) })
	complete("info", "--all", "d")
	fixture.Configure(func(g *testutil.GitHub) {
		if len(g.Requests) != count {
			t.Fatal("fresh cache requested API")
		}
	})
	c.Updated = time.Now().Add(-2 * time.Minute)
	data, _ = json.Marshal(c)
	os.WriteFile(cacheFiles[0], data, 0600)
	fixture.Close()
	if got := complete("info", "--all", "d"); got != "demo\n" {
		t.Fatal("offline cache", got)
	}
	if got := complete("get", "d"); got != "" {
		t.Fatal("write cache leaked into read completion", got)
	}
	if e := config.SaveToken("changed-token", true); e != nil {
		t.Fatal(e)
	}
	if got := complete("info", "--all", "d"); got != "" {
		t.Fatal("cache reused after credential change", got)
	}
	_ = root
}

func TestCompletionUsesCommandRepository(t *testing.T) {
	a, fixture, out, _ := projectApp(t)
	t.Setenv("NODI_REPO", "other/global")
	t.Setenv("NODI_READ_TOKEN", "read-test")
	t.Setenv("NODI_WRITE_TOKEN", "write-test")
	var selected string
	a.StoreFor = func(repo string, write bool) (storage.Store, error) {
		selected = repo
		return storage.NewGitHubWithClient("owner/private", "read-test", fixture.Server.URL, fixture.Server.URL, fixture.Server.Client())
	}
	for _, tt := range []struct{ command, repo string }{
		{"get", "other/global"}, {"info", "other/global"}, {"open", "other/global"}, {"publish", "owner/private"},
	} {
		selected = ""
		out.Reset()
		a.complete(context.Background(), []string{tt.command, ""})
		if selected != tt.repo {
			t.Fatalf("%s completion queried %q, want %q", tt.command, selected, tt.repo)
		}
		// Empty indexes are cached too; remove them to observe every command's choice.
		dir, _ := config.Dir()
		paths, _ := filepath.Glob(filepath.Join(dir, "completion", "*.json"))
		for _, path := range paths {
			os.Remove(path)
		}
	}
}
