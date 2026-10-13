package storage

import (
	"context"
	"net/http"
	"nodi/internal/archive"
	"nodi/internal/testutil"
	"os"
	"strings"
	"testing"
)

func TestRegressionB07HealthyLatestIgnoresDamagedHistory(t *testing.T) {
	g, mock, _ := deleteFixture(t)
	ctx := context.Background()
	old, err := g.byTag(ctx, "demo/v0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	mock.AlterRelease(old.ID, func(r *testutil.Release) { r.Assets = nil })
	m, err := g.Get(ctx, "demo", "")
	if err != nil || m.Version != "0.1.1" {
		t.Fatal("healthy latest blocked by older version", m.Version, err)
	}
	ms, err := g.List(ctx, false)
	if len(ms) != 3 {
		t.Fatal("healthy list results lost", len(ms), err)
	}
	if err == nil {
		t.Fatal("damaged history silently hidden")
	}
}

type lateReleaseTransport struct {
	base  http.RoundTripper
	mock  *testutil.GitHub
	id    int64
	reads int
}

func (f *lateReleaseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == "GET" && strings.HasSuffix(req.URL.Path, "/releases") {
		f.reads++
		if f.reads == 2 {
			f.mock.AlterRelease(f.id, func(r *testutil.Release) { r.Tag = "demo/v0.1.2"; r.Draft = true })
		}
	}
	return f.base.RoundTrip(req)
}
func TestRegressionB08LateReleaseConflict(t *testing.T) {
	g, mock, source := deleteFixture(t)
	ctx := context.Background()
	other, err := g.byTag(ctx, "other/v0.1.1")
	if err != nil {
		t.Fatal(err)
	}
	client := mock.Server.Client()
	client.Transport = &lateReleaseTransport{base: client.Transport, mock: mock, id: other.ID}
	g.client = client
	m, z, err := archive.Prepare(ctx, source, "demo", "0.1.2", "released", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(z)
	mock.Configure(func(m *testutil.GitHub) { m.Requests = nil })
	err = g.Publish(ctx, m, z)
	if err == nil || !strings.Contains(err.Error(), "version conflict") {
		t.Fatal("late foreign release must be a safe version conflict", err)
	}
	mock.Configure(func(m *testutil.GitHub) {
		for _, r := range m.Requests {
			if !strings.HasPrefix(r, "GET ") {
				t.Fatal("late conflict caused mutation", r)
			}
		}
	})
}

func TestB07DamagedHighestNeverDowngrades(t *testing.T) {
	for _, mode := range []string{"missing assets", "duplicate highest", "bad status"} {
		t.Run(mode, func(t *testing.T) {
			g, mock, _ := deleteFixture(t)
			ctx := context.Background()
			latest, err := g.byTag(ctx, "demo/v0.1.1")
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "missing assets":
				mock.AlterRelease(latest.ID, func(r *testutil.Release) { r.Assets = nil })
			case "bad status":
				mock.AlterRelease(latest.ID, func(r *testutil.Release) { r.Body = "Nodi schema 1. Status: draft." })
			case "duplicate highest":
				old, _ := g.byTag(ctx, "demo/v0.1.0")
				mock.AlterRelease(old.ID, func(r *testutil.Release) { r.Tag = latest.Tag })
			}
			if mode == "bad status" {
				mock.AlterRelease(latest.ID, func(r *testutil.Release) { r.Assets[0].State = "new" })
			}
			if m, err := g.Get(ctx, "demo", ""); err == nil {
				t.Fatal("damaged latest silently downgraded", m.Version)
			}
		})
	}
}

func TestB08MalformedReservedRefIsReadOnly(t *testing.T) {
	g, mock, source := deleteFixture(t)
	mock.SetTag("refs/tags/demo/vbad", strings.Repeat("a", 40))
	ctx := context.Background()
	m, z, err := archive.Prepare(ctx, source, "demo", "0.1.2", "released", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(z)
	mock.Configure(func(m *testutil.GitHub) { m.Requests = nil })
	if err = g.Publish(ctx, m, z); err == nil {
		t.Fatal("invalid ref accepted")
	}
	mock.Configure(func(m *testutil.GitHub) {
		for _, r := range m.Requests {
			if !strings.HasPrefix(r, "GET ") {
				t.Fatal("invalid ref caused remote write", r)
			}
		}
	})
}

func TestRegressionB08ReservedHighWaterMark(t *testing.T) {
	g, mock, source := deleteFixture(t)
	ctx := context.Background()
	mock.SetTag("refs/tags/demo/v9.0.0", strings.Repeat("a", 40))
	m, z, err := archive.Prepare(ctx, source, "demo", "0.1.2", "released", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(z)
	mock.Configure(func(m *testutil.GitHub) { m.Requests = nil })
	if err = g.Publish(ctx, m, z); err == nil {
		t.Fatal("published below reserved high-water mark")
	}
	if !strings.Contains(err.Error(), "version conflict") {
		t.Fatal("reservation was not identified as a version conflict", err)
	}
	mock.Configure(func(m *testutil.GitHub) {
		for _, r := range m.Requests {
			if !strings.HasPrefix(r, "GET ") {
				t.Fatal("preflight mutated remote", r)
			}
		}
	})
}
