package storage

import (
	"context"
	"nodi/internal/archive"
	"nodi/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func deleteFixture(t *testing.T) (*GitHub, *testutil.GitHub, string) {
	t.Helper()
	mock := testutil.NewGitHub()
	t.Cleanup(mock.Close)
	g, _ := NewGitHubWithClient("owner/private", "write-test", mock.Server.URL, mock.Server.URL, mock.Server.Client())
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "data.txt"), []byte("test deletion fixture"), 0644)
	for _, p := range []string{"demo", "other"} {
		for _, v := range []string{"0.1.0", "0.1.1"} {
			m, z, e := archive.Prepare(context.Background(), root, p, v, "released", "", nil)
			if e != nil {
				t.Fatal(e)
			}
			e = g.Publish(context.Background(), m, z)
			os.Remove(z)
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	return g, mock, root
}

func TestDeletePreservesOtherProjectsAndVersionHistory(t *testing.T) {
	g, mock, root := deleteFixture(t)
	ctx := context.Background()
	plan, e := g.PlanDelete(ctx, "demo", "0.1.1", false)
	if e != nil || len(plan.Items) != 1 {
		t.Fatal(plan, e)
	}
	if e = g.Delete(ctx, plan); e != nil {
		t.Fatal(e)
	}
	state, e := g.State(ctx, "demo")
	if e != nil || state.Version != "0.1.1" || state.Manifest.Version != "0.1.0" {
		t.Fatal(state, e)
	}
	for _, v := range []string{"0.1.0", "0.1.1"} {
		m, z, _ := archive.Prepare(ctx, root, "demo", v, "released", "", nil)
		e = g.Publish(ctx, m, z)
		os.Remove(z)
		if e == nil {
			t.Fatal("reused deletion history", v)
		}
	}
	plan, e = g.PlanDelete(ctx, "demo", "", true)
	if e != nil {
		t.Fatal(e)
	}
	if e = g.Delete(ctx, plan); e != nil {
		t.Fatal(e)
	}
	state, e = g.State(ctx, "demo")
	if e != nil || state.Version != "0.1.1" || state.Manifest != nil {
		t.Fatal(state, e)
	}
	ms, e := g.List(ctx, true)
	if e != nil || len(ms) != 2 || ms[0].Project != "other" {
		t.Fatal(ms, e)
	}
	idx, e := g.Index(ctx, true)
	if e != nil || len(idx) != 2 {
		t.Fatal(idx, e)
	}
	if e = g.Delete(ctx, plan); e == nil {
		t.Fatal("repeated stale deletion succeeded")
	}
	mock.Configure(func(g *testutil.GitHub) {
		seenReleaseDelete := false
		for _, req := range g.Requests {
			if strings.HasPrefix(req, "DELETE ") {
				if strings.Contains(req, "/releases/") {
					seenReleaseDelete = true
				}
				if strings.Contains(req, "/git/refs/tags/") && !seenReleaseDelete {
					t.Fatal("tag removed first")
				}
			}
		}
	})
}

func TestDeleteReadOnlyAndDamagedReleaseRefused(t *testing.T) {
	g, mock, _ := deleteFixture(t)
	ctx := context.Background()
	plan, e := g.PlanDelete(ctx, "demo", "", true)
	if e != nil {
		t.Fatal(e)
	}
	read, _ := NewGitHubWithClient("owner/private", "read-test", mock.Server.URL, mock.Server.URL, mock.Server.Client())
	if e = read.Delete(ctx, plan); !IsStatus(e, 403) {
		t.Fatal("read-only delete", e)
	}
	mock.Configure(func(g *testutil.GitHub) { g.CorruptArchive = true })
	if _, e = g.PlanDelete(ctx, "demo", "", true); e == nil {
		t.Fatal("damaged package selected")
	}
	mock.Configure(func(g *testutil.GitHub) {
		for _, r := range g.Requests {
			if strings.HasPrefix(r, "DELETE ") {
				t.Fatal("unsafe delete", r)
			}
		}
	})
}

func TestDeletePartialFailuresReportExactState(t *testing.T) {
	for _, mode := range []string{"history", "release", "tag", "verification"} {
		t.Run(mode, func(t *testing.T) {
			g, mock, _ := deleteFixture(t)
			ctx := context.Background()
			plan, e := g.PlanDelete(ctx, "demo", "0.1.1", false)
			if e != nil {
				t.Fatal(e)
			}
			mock.Configure(func(g *testutil.GitHub) {
				switch mode {
				case "history":
					g.FailHistory = true
				case "release":
					g.FailDeleteRelease = true
				case "tag":
					g.FailDeleteTag = true
				case "verification":
					g.FailDeleteVerify = true
				}
			})
			e = g.Delete(ctx, plan)
			if e == nil {
				t.Fatal("partial failure hidden")
			}
			want := map[string]string{"history": "no release deleted", "release": "present; tag demo/v0.1.1: present", "tag": "absent (API verified); tag demo/v0.1.1: present", "verification": "NOT VERIFIED"}[mode]
			if !strings.Contains(e.Error(), want) {
				t.Fatal(want, e)
			}
			mock.Configure(func(g *testutil.GitHub) {
				if mode == "history" || mode == "verification" {
					for _, r := range g.Requests {
						if strings.HasPrefix(r, "DELETE ") && strings.Contains(r, "/git/refs/") {
							t.Fatal("removed tag without confirmed release absence", r)
						}
					}
				}
			})
		})
	}
}

func TestDeleteChangedPlanRefused(t *testing.T) {
	g, mock, root := deleteFixture(t)
	ctx := context.Background()
	plan, e := g.PlanDelete(ctx, "demo", "", true)
	if e != nil {
		t.Fatal(e)
	}
	m, z, _ := archive.Prepare(ctx, root, "demo", "0.1.2", "released", "", nil)
	defer os.Remove(z)
	if e = g.Publish(ctx, m, z); e != nil {
		t.Fatal(e)
	}
	if e = g.Delete(ctx, plan); e == nil || !strings.Contains(e.Error(), "selection changed") {
		t.Fatal(e)
	}
	mock.Configure(func(g *testutil.GitHub) {
		for _, r := range g.Requests {
			if strings.HasPrefix(r, "DELETE ") {
				t.Fatal("changed plan deleted", r)
			}
		}
	})
}

func TestDeleteUnknownAssetsMissingOrChangedTagsAndForgedPlans(t *testing.T) {
	for _, mode := range []string{"extra asset", "missing tag", "changed tag", "forged multiple", "status mismatch"} {
		t.Run(mode, func(t *testing.T) {
			g, mock, _ := deleteFixture(t)
			ctx := context.Background()
			plan, e := g.PlanDelete(ctx, "demo", "0.1.0", false)
			if e != nil {
				t.Fatal(e)
			}
			switch mode {
			case "extra asset":
				mock.AlterRelease(plan.Items[0].ReleaseID, func(r *testutil.Release) {
					r.Assets = append(r.Assets, testutil.Asset{ID: 999, Name: "unknown.dat", Size: 1, State: "uploaded"})
				})
			case "missing tag":
				mock.SetTag("refs/tags/demo/v0.1.0", "")
			case "changed tag":
				mock.SetTag("refs/tags/demo/v0.1.0", strings.Repeat("b", 40))
			case "forged multiple":
				plan.Items = append(plan.Items, plan.Items[0])
			case "status mismatch":
				mock.AlterRelease(plan.Items[0].ReleaseID, func(r *testutil.Release) { r.Prerelease = true })
			}
			if e = g.Delete(ctx, plan); e == nil {
				t.Fatal("unsafe deletion accepted", mode)
			}
			mock.Configure(func(g *testutil.GitHub) {
				for _, r := range g.Requests {
					if strings.HasPrefix(r, "DELETE ") {
						t.Fatal("unsafe deletion", r)
					}
				}
			})
		})
	}
}

func TestDeleteAllStopsAfterPartialFailure(t *testing.T) {
	g, mock, _ := deleteFixture(t)
	ctx := context.Background()
	plan, e := g.PlanDelete(ctx, "demo", "", true)
	if e != nil {
		t.Fatal(e)
	}
	mock.Configure(func(g *testutil.GitHub) { g.FailDeleteReleaseID = plan.Items[1].ReleaseID })
	if e = g.Delete(ctx, plan); e == nil || !strings.Contains(e.Error(), "1 earlier releases completed, 0 later releases untouched") {
		t.Fatal(e)
	}
	if _, e = g.Get(ctx, "demo", "0.1.0"); !IsStatus(e, 404) {
		t.Fatal("first still present", e)
	}
	if _, e = g.Get(ctx, "demo", "0.1.1"); e != nil {
		t.Fatal("failed item removed", e)
	}
}
