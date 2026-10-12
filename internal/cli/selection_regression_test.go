package cli

import (
	"context"
	"nodi/internal/storage"
	"nodi/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegressionB09ExclusionsVisible(t *testing.T) {
	a, _, out, root := projectApp(t)
	os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"units":"mm"}`), 0644)
	for i := 0; i < 2; i++ {
		out.Reset()
		if err := a.Run(context.Background(), []string{"send", root}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "config.json") || !strings.Contains(out.String(), "Excluded:") {
			t.Fatal("normal send hid exclusions", out.String())
		}
	}
}

func TestB09HardDenialsAndSelectionRecovery(t *testing.T) {
	a, fixture, _, root := projectApp(t)
	ctx := context.Background()
	os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"units":"mm"}`), 0644)
	os.WriteFile(filepath.Join(root, "credentials.json"), []byte("fictional secret"), 0600)
	for _, name := range []string{"credentials.json", ".nodi/project.json", "../config.json", "*.json"} {
		if err := a.Run(ctx, []string{"send", root, "--include", name}); err == nil {
			t.Fatal("protected include accepted", name)
		}
	}
	fixture.Configure(func(m *testutil.GitHub) {
		for _, r := range m.Requests {
			if !strings.HasPrefix(r, "GET ") {
				t.Fatal("unsafe selection mutated remote", r)
			}
		}
	})
	client := fixture.Server.Client()
	loss := &publicationFailureTransport{base: client.Transport, phase: "archive", commits: map[string]int{}}
	client.Transport = loss
	a.StoreFor = func(repo string, _ bool) (storage.Store, error) {
		return storage.NewGitHubWithClient(repo, "write-test", fixture.Server.URL, fixture.Server.URL, client)
	}
	if err := a.Run(ctx, []string{"send", root, "--include", "config.json"}); err == nil {
		t.Fatal("failure not injected")
	}
	if err := a.Run(ctx, []string{"send", root}); err == nil {
		t.Fatal("changed selection adopted pending operation")
	}
	if err := a.Run(ctx, []string{"send", root, "--include", "config.json"}); err != nil {
		t.Fatal("selected recovery failed", err)
	}
	s, _ := a.StoreFor("owner/private", false)
	m, err := s.Get(ctx, "demo", "")
	if err != nil || len(m.Files) != 2 {
		t.Fatal(m.Files, err)
	}
}

func TestB09ExplicitBuildAndConfigRoots(t *testing.T) {
	a, _, _, root := projectApp(t)
	ctx := context.Background()
	for _, name := range []string{"build", "dist"} {
		source := filepath.Join(t.TempDir(), name)
		os.Mkdir(source, 0755)
		os.WriteFile(filepath.Join(source, "data.txt"), []byte("fixture"), 0644)
		os.WriteFile(filepath.Join(source, "token.json"), []byte("fictional secret"), 0600)
		if err := a.Run(ctx, []string{"send", source, "--name", name + "-package"}); err != nil {
			t.Fatal(err)
		}
	}
	source := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(source, []byte(`{"units":"mm"}`), 0644)
	if err := a.Run(ctx, []string{"send", source, "--name", "ordinary-config"}); err != nil {
		t.Fatal(err)
	}
	private := filepath.Join(os.Getenv("NODI_CONFIG_DIR"), "config.json")
	os.WriteFile(private, []byte(`{"repository":"owner/private"}`), 0600)
	if err := a.Run(ctx, []string{"send", private}); err == nil {
		t.Fatal("private Nodi config sent")
	}
	_ = root
}
func TestRegressionB09ExplicitConfig(t *testing.T) {
	a, _, out, root := projectApp(t)
	os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"units":"mm"}`), 0644)
	if err := a.Run(context.Background(), []string{"send", root, "--include", "config.json", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "config.json (") {
		t.Fatal("preview omitted exact include", out.String())
	}
	if err := a.Run(context.Background(), []string{"send", root, "--include", "config.json"}); err != nil {
		t.Fatal(err)
	}
	s, _ := a.StoreFor("owner/private", false)
	m, err := s.Get(context.Background(), "demo", "")
	if err != nil || len(m.Files) != 2 {
		t.Fatal("actual manifest differs", m.Files, err)
	}
}
