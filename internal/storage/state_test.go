package storage

import (
	"context"
	"net/http"
	"nodi/internal/archive"
	"nodi/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type barrierTransport struct {
	base  http.RoundTripper
	mu    sync.Mutex
	count int
	ready chan struct{}
}

func (b *barrierTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	res, e := b.base.RoundTrip(r)
	if e == nil && res.StatusCode == 404 && strings.Contains(r.URL.Path, "/git/ref/tags/") {
		b.mu.Lock()
		b.count++
		if b.count == 2 {
			close(b.ready)
		}
		b.mu.Unlock()
		select {
		case <-b.ready:
		case <-r.Context().Done():
		}
	}
	return res, e
}
func TestConcurrentPublicationReservesOneTag(t *testing.T) {
	mock := testutil.NewGitHub()
	defer mock.Close()
	client := *mock.Server.Client()
	client.Transport = &barrierTransport{base: client.Transport, ready: make(chan struct{})}
	g, _ := NewGitHubWithClient("owner/private", "write-test", mock.Server.URL, mock.Server.URL, &client)
	other, _ := NewGitHubWithClient("owner/private", "write-test", mock.Server.URL, mock.Server.URL, &client)
	g.publicationDir, other.publicationDir = t.TempDir(), t.TempDir()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "part.dxf"), []byte("fictional concurrent test"), 0644)
	m, z, e := archive.Prepare(context.Background(), root, "demo", "0.1.0", "draft", "", nil)
	if e != nil {
		t.Fatal(e)
	}
	defer os.Remove(z)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, sender := range []*GitHub{g, other} {
		wg.Add(1)
		go func(sender *GitHub) { defer wg.Done(); results <- sender.Publish(context.Background(), m, z) }(sender)
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for e := range results {
		if e == nil {
			successes++
		} else if strings.Contains(e.Error(), "version conflict") || strings.Contains(e.Error(), "version tag already exists") || strings.Contains(e.Error(), "ownership is ambiguous") {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal(successes, conflicts)
	}
	ms, e := g.List(context.Background(), true)
	if e != nil || len(ms) != 1 {
		t.Fatal(ms, e)
	}
	state, e := g.State(context.Background(), "demo")
	if e != nil || state.Version != "0.1.0" || state.Manifest.Status != "draft" {
		t.Fatal(state, e)
	}
}
func TestInterruptedPublicationResumesOnlyItsJournal(t *testing.T) {
	mock := testutil.NewGitHub()
	defer mock.Close()
	g, _ := NewGitHubWithClient("owner/private", "write-test", mock.Server.URL, mock.Server.URL, mock.Server.Client())
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "part.dxf"), []byte("fictional"), 0644)
	m, z, e := archive.Prepare(context.Background(), root, "demo", "0.1.0", "draft", "", nil)
	if e != nil {
		t.Fatal(e)
	}
	defer os.Remove(z)
	mock.Configure(func(g *testutil.GitHub) { g.FailUpload = true })
	if e = g.Publish(context.Background(), m, z); e == nil {
		t.Fatal("failed upload succeeded")
	}
	state, e := g.State(context.Background(), "demo")
	if e == nil || state.Version != "0.1.0" || !strings.Contains(e.Error(), "incomplete") {
		t.Fatal(state, e)
	}
	mock.Configure(func(g *testutil.GitHub) { g.FailUpload = false })
	if e = g.Publish(context.Background(), m, z); e != nil {
		t.Fatal("verified same-operation recovery failed", e)
	}
	// A tag reserved before a disconnected release-creation response is also a blocker.
	mock.SetTag("refs/tags/orphan/v0.1.0", strings.Repeat("a", 40))
	state, e = g.State(context.Background(), "orphan")
	if e == nil || state.Version != "0.1.0" || !strings.Contains(e.Error(), "reserved tag") {
		t.Fatal(state, e)
	}
}
