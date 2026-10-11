package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"nodi/internal/storage"
	"strings"
	"testing"
	"time"
)

func TestCompletionDeadline(t *testing.T) {
	a, _, out, _ := projectApp(t)
	t.Setenv("NODI_READ_TOKEN", "read-test")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	a.StoreFor = func(repo string, write bool) (storage.Store, error) {
		return storage.NewGitHubWithClient(repo, "read-test", server.URL, server.URL, server.Client())
	}
	out.Reset()
	start := time.Now()
	if e := a.Run(context.Background(), []string{"__complete", "get", "d"}); e != nil {
		t.Fatal(e)
	}
	if time.Since(start) > 2500*time.Millisecond {
		t.Fatal("completion did not respect short deadline")
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatal("completion error leaked into terminal", out.String())
	}
}
