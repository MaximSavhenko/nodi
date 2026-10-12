package storage

import (
	"context"
	"fmt"
	"net/http"
	"nodi/internal/archive"
	"nodi/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func interruptedPublicationFixture(t *testing.T) (*GitHub, *testutil.GitHub, *publicationJournal, string) {
	t.Helper()
	mock := testutil.NewGitHub()
	t.Cleanup(mock.Close)
	g, err := NewGitHubWithClient("owner/private", "write-test", mock.Server.URL, mock.Server.URL, mock.Server.Client())
	if err != nil {
		t.Fatal(err)
	}
	g.publicationDir = t.TempDir()
	source := t.TempDir()
	if err = os.WriteFile(filepath.Join(source, "data.txt"), []byte("fictional upload"), 0600); err != nil {
		t.Fatal(err)
	}
	m, z, err := archive.Prepare(context.Background(), source, "demo", "0.1.0", "released", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(z)
	mock.Configure(func(g *testutil.GitHub) { g.FailUpload = true })
	if err = g.PublishFrom(context.Background(), m, z, source); err == nil {
		t.Fatal("expected interrupted upload")
	}
	root, err := g.publicationRoot("demo")
	if err != nil {
		t.Fatal(err)
	}
	j, err := g.pendingPublication(root)
	if err != nil || j == nil {
		t.Fatal(j, err)
	}
	mock.Configure(func(g *testutil.GitHub) { g.FailUpload = false })
	return g, mock, j, source
}

func TestPublicationJournalTemporaryRecordAndLock(t *testing.T) {
	g, _, j, source := interruptedPublicationFixture(t)
	if err := os.WriteFile(filepath.Join(j.dir, ".checkpoint-interrupted"), []byte("partial uncommitted record"), 0600); err != nil {
		t.Fatal(err)
	}
	_, unlock, err := g.lockPublication("demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = g.ResumePublish(context.Background(), "demo", source, "released", ""); err == nil {
		unlock()
		t.Fatal("concurrent local publication acquired held lock")
	}
	unlock()
	restarted, err := NewGitHubWithClient("owner/private", "write-test", g.base, g.uploads, g.client)
	if err != nil {
		t.Fatal(err)
	}
	restarted.publicationDir = g.publicationDir
	m, err := restarted.ResumePublish(context.Background(), "demo", source, "released", "")
	if err != nil || m == nil || m.Version != "0.1.0" {
		t.Fatal(m, err)
	}
	if err = restarted.AcknowledgePublish(context.Background(), "demo", m.Version, source); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(j.dir, ".checkpoint-interrupted")); err != nil {
		t.Fatal("uncommitted evidence was deleted:", err)
	}
}

func TestPublicationJournalWriteFailurePreventsRemoteMutation(t *testing.T) {
	g, mock, j, source := interruptedPublicationFixture(t)
	// Block the next immutable checkpoint path. The upload must not occur
	// when its intent cannot be persisted, even though the remote draft is ours.
	blocked := filepath.Join(j.dir, fmt.Sprintf("%06d.json", j.State.Sequence+1))
	injected := false
	g.client.Transport = deletionMutationTransport{g.client.Transport, func(req *http.Request, res *http.Response) {
		if !injected && req.Method == "GET" && req.URL.Path == "/repos/owner/private" && res.StatusCode == 200 {
			injected = true
			if err := os.Mkdir(blocked, 0700); err != nil {
				t.Fatal(err)
			}
		}
	}}
	before := 0
	mock.Configure(func(g *testutil.GitHub) { before = len(g.Requests) })
	if _, err := g.ResumePublish(context.Background(), "demo", source, "released", ""); err == nil || !strings.Contains(err.Error(), "record already exists") {
		t.Fatal("checkpoint write failure did not stop upload:", err)
	}
	if !injected {
		t.Fatal("checkpoint failure was not injected after journal load")
	}
	mock.Configure(func(g *testutil.GitHub) {
		for _, req := range g.Requests[before:] {
			if !strings.HasPrefix(req, "GET ") {
				t.Error("mutation without durable checkpoint:", req)
			}
		}
	})
}

func TestPublicationOtherProjectIsNotBlocked(t *testing.T) {
	g, _, _, source := interruptedPublicationFixture(t)
	m, z, err := archive.Prepare(context.Background(), source, "other", "0.1.0", "released", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(z)
	if err = g.Publish(context.Background(), m, z); err != nil {
		t.Fatal("another project's pending operation blocked publication:", err)
	}
}
