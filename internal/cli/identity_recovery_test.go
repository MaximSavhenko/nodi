package cli

import (
	"bytes"
	"context"
	"nodi/internal/storage"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegressionA4FirstSendIdentity(t *testing.T) {
	for _, kind := range []string{"file", "directory"} {
		t.Run(kind, func(t *testing.T) {
			a, fixture, _, root := projectApp(t)
			os.RemoveAll(filepath.Join(root, ".nodi"))
			source := root
			if kind == "file" {
				source = filepath.Join(root, "part.dxf")
			}
			client := fixture.Server.Client()
			loss := &publicationFailureTransport{base: client.Transport, phase: "final_read", commits: map[string]int{}}
			client.Transport = loss
			a.StoreFor = func(repo string, write bool) (storage.Store, error) {
				return storage.NewGitHubWithClient(repo, "write-test", fixture.Server.URL, fixture.Server.URL, client)
			}
			ctx := context.Background()
			if err := a.Run(ctx, []string{"send", source, "--name", "demo"}); err == nil || !loss.fired {
				t.Fatal("final GET failure not injected", err)
			}
			out := new(bytes.Buffer)
			restart := New("dev", strings.NewReader(""), out)
			restart.StoreFor = a.StoreFor
			if err := restart.Run(ctx, []string{"send", source}); err != nil {
				t.Fatal("identity recovery:", err)
			}
			if !strings.Contains(out.String(), "Recovered demo 0.1.0") {
				t.Fatal(out.String())
			}
			receipt, err := readSendReceipt(source, "owner/private")
			if err != nil || receipt == nil || receipt.Name != "demo" {
				t.Fatal(receipt, err)
			}
			for _, n := range loss.commits {
				if n != 1 {
					t.Fatal("recovery mutated remote twice")
				}
			}
			// Identical bytes at another source cannot inherit the established name.
			foreign := filepath.Join(t.TempDir(), "part.dxf")
			data, _ := os.ReadFile(filepath.Join(root, "part.dxf"))
			os.WriteFile(foreign, data, 0644)
			if err := restart.Run(ctx, []string{"send", foreign, "--name", "demo"}); err == nil {
				t.Fatal("foreign source adopted matching content")
			}
		})
	}
}
