package storage

import (
	"context"
	"fmt"
	"net/http"
	"nodi/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type deleteReplyLoss struct {
	base         http.RoundTripper
	target       string
	after, fired bool
	deletes      map[string]int
}

func TestA3UnsafeRestartRefusesWrites(t *testing.T) {
	for _, mode := range []string{"tag", "history", "missing history", "repository", "replacement draft", "changed assets", "corrupt journal", "different selection"} {
		t.Run(mode, func(t *testing.T) {
			g, mock, _ := deleteFixture(t)
			ctx := context.Background()
			plan, err := g.PlanDelete(ctx, "demo", "0.1.1", false)
			if err != nil {
				t.Fatal(err)
			}
			// Asset changes must be tested while the original release still exists.
			mock.Configure(func(m *testutil.GitHub) {
				if mode == "changed assets" {
					m.FailDeleteRelease = true
				} else {
					m.FailDeleteTag = true
				}
			})
			if err = g.Delete(ctx, plan); err == nil {
				t.Fatal("expected interruption")
			}
			switch mode {
			case "tag":
				mock.SetTag("refs/tags/demo/v0.1.1", strings.Repeat("b", 40))
			case "history":
				mock.SetTag("refs/tags/nodi-deleted/demo/v0.1.1", strings.Repeat("b", 40))
			case "missing history":
				mock.SetTag("refs/tags/nodi-deleted/demo/v0.1.1", "")
			case "repository":
				mock.Configure(func(m *testutil.GitHub) { m.RepositoryID++ })
			case "replacement draft":
				mock.AlterRelease(plan.Items[0].ReleaseID-3, func(r *testutil.Release) { r.Tag = plan.Items[0].Tag; r.Draft = true })
			case "changed assets":
				mock.AlterRelease(plan.Items[0].ReleaseID, func(r *testutil.Release) {
					r.Assets = append(r.Assets, testutil.Asset{ID: 999, Name: "foreign.txt", Size: 1, State: "uploaded"})
				})
			case "corrupt journal":
				root, _ := g.deletionRoot("demo")
				dirs, _ := os.ReadDir(root)
				os.WriteFile(filepath.Join(root, dirs[0].Name(), "operation.json"), []byte("broken"), 0600)
			case "different selection":
				plan.Version = "0.1.0"
			}
			mock.Configure(func(m *testutil.GitHub) { m.FailDeleteTag = false; m.FailDeleteRelease = false; m.Requests = nil })
			restart, _ := NewGitHubWithClient("owner/private", "write-test", mock.Server.URL, mock.Server.URL, mock.Server.Client())
			if err = restart.Delete(ctx, plan); err == nil {
				t.Fatal("unsafe restart accepted", mode)
			}
			mock.Configure(func(m *testutil.GitHub) {
				for _, r := range m.Requests {
					if !strings.HasPrefix(r, "GET ") && r != "POST /repos/owner/private/git/refs" {
						t.Fatal("unsafe remote write", r)
					}
				}
			})
		})
	}
}

func (f *deleteReplyLoss) RoundTrip(r *http.Request) (*http.Response, error) {
	match := r.Method == "DELETE" && strings.Contains(r.URL.Path, f.target)
	if match && !f.fired && !f.after {
		f.fired = true
		return nil, fmt.Errorf("fixture interruption")
	}
	res, err := f.base.RoundTrip(r)
	if match && err == nil && res.StatusCode < 300 {
		f.deletes[r.URL.Path]++
		if !f.fired && f.after {
			f.fired = true
			res.Body.Close()
			return nil, fmt.Errorf("fixture lost reply")
		}
	}
	return res, err
}
func TestRegressionA3DeleteRestart(t *testing.T) {
	for _, target := range []string{"/releases/", "/git/refs/tags/"} {
		for _, after := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/after=%t", target, after), func(t *testing.T) {
				g, mock, _ := deleteFixture(t)
				ctx := context.Background()
				client := mock.Server.Client()
				loss := &deleteReplyLoss{base: client.Transport, target: target, after: after, deletes: map[string]int{}}
				client.Transport = loss
				g.client = client
				plan, err := g.PlanDelete(ctx, "demo", "", true)
				if err != nil {
					t.Fatal(err)
				}
				if err = g.Delete(ctx, plan); err == nil {
					t.Fatal("failure not injected")
				}
				restarted, _ := NewGitHubWithClient("owner/private", "write-test", mock.Server.URL, mock.Server.URL, client)
				recovered, err := restarted.PlanDelete(ctx, "demo", "", true)
				if err != nil {
					t.Fatal("restart plan:", err)
				}
				if err = restarted.Delete(ctx, recovered); err != nil {
					t.Fatal("restart delete:", err)
				}
				state, err := restarted.State(ctx, "demo")
				if err != nil || state.Manifest != nil || state.Version != "0.1.1" {
					t.Fatal(state, err)
				}
				if err = restarted.Delete(ctx, recovered); err == nil {
					t.Fatal("stale completed plan accepted")
				}
				for p, n := range loss.deletes {
					if n != 1 {
						t.Fatalf("repeated committed DELETE %s: %d", p, n)
					}
				}
			})
		}
	}
}
