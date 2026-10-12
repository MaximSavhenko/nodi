package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"nodi/internal/model"
	"nodi/internal/testutil"
	"strconv"
	"strings"
	"testing"
)

// Inject after the fresh plan has passed, while the final release read can
// still observe the change. This is the window reproduced by the B01 audit.
type deletionMutationTransport struct {
	base  http.RoundTripper
	after func(*http.Request, *http.Response)
}

func (t deletionMutationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := t.base.RoundTrip(req)
	if err == nil {
		t.after(req, res)
	}
	return res, err
}

func TestRegressionB01AssetSnapshotChanges(t *testing.T) {
	for _, timing := range []string{"before_recheck", "after_fresh_plan"} {
		for _, change := range []string{"add", "remove", "replace_archive", "replace_manifest", "rename", "resize", "state", "same_size_archive_bytes"} {
			t.Run(timing+"/"+change, func(t *testing.T) {
				g, mock, _ := deleteFixture(t)
				ctx := context.Background()
				plan, err := g.PlanDelete(ctx, "demo", "0.1.1", false)
				if err != nil {
					t.Fatal(err)
				}
				item := plan.Items[0]
				mutate := func() {
					if change == "replace_archive" || change == "replace_manifest" {
						// Replace with identical bytes, so hashes alone cannot detect
						// that a different asset ID was not in the approved snapshot.
						name := model.ArchiveName
						if change == "replace_manifest" {
							name = model.ManifestName
						}
						var original testutil.Asset
						mock.AlterRelease(item.ReleaseID, func(r *testutil.Release) {
							for _, a := range r.Assets {
								if a.Name == name {
									original = a
								}
							}
						})
						res, err := g.request(ctx, "GET", g.endpoint("/releases/assets/"+strconv.FormatInt(original.ID, 10)), "application/octet-stream", nil, -1)
						if err != nil {
							t.Fatal(err)
						}
						data, err := io.ReadAll(res.Body)
						res.Body.Close()
						if err != nil {
							t.Fatal(err)
						}
						if err = g.upload(ctx, release{ID: item.ReleaseID}, name, "application/octet-stream", bytes.NewReader(data), int64(len(data))); err != nil {
							t.Fatal(err)
						}
						mock.AlterRelease(item.ReleaseID, func(r *testutil.Release) {
							for i, a := range r.Assets {
								if a.ID == original.ID {
									r.Assets = append(r.Assets[:i], r.Assets[i+1:]...)
									break
								}
							}
						})
						return
					}
					if change == "same_size_archive_bytes" {
						mock.Configure(func(g *testutil.GitHub) { g.CorruptArchive = true })
						return
					}
					mock.AlterRelease(item.ReleaseID, func(r *testutil.Release) {
						switch change {
						case "add":
							r.Assets = append(r.Assets, testutil.Asset{ID: 99999, Name: "unapproved-original.step", Size: 123, State: "uploaded"})
						case "remove":
							r.Assets = r.Assets[1:]
						case "rename":
							r.Assets[0].Name = "changed.zip"
						case "resize":
							r.Assets[0].Size++
						case "state":
							r.Assets[0].State = "starter"
						}
					})
				}
				injected := timing == "before_recheck"
				if injected {
					mutate()
				} else {
					g.client.Transport = deletionMutationTransport{g.client.Transport, func(req *http.Request, res *http.Response) {
						if !injected && req.Method == "POST" && strings.HasSuffix(req.URL.Path, "/git/refs") && res.StatusCode >= 200 && res.StatusCode < 300 {
							injected = true
							mutate()
						}
					}}
				}
				err = g.Delete(ctx, plan)
				if !injected {
					t.Fatal("mutation was not injected")
				}
				if err == nil {
					t.Error("changed assets were accepted for deletion")
				}
				// A refusal must preserve the changed release and the original tag,
				// not merely return an error after deleting either of them.
				var remaining release
				if err := g.json(ctx, "GET", g.endpoint("/releases/"+strconv.FormatInt(item.ReleaseID, 10)), nil, &remaining); err != nil {
					t.Error("changed release was removed:", err)
				}
				ref, err := g.tagRef(ctx, item.Tag)
				if err != nil || ref.Object.SHA != item.TagSHA {
					t.Error("approved tag was removed or changed:", ref, err)
				}
				mock.Configure(func(g *testutil.GitHub) {
					for _, req := range g.Requests {
						if strings.HasPrefix(req, "DELETE ") {
							t.Error("destructive request after asset change:", req)
						}
					}
				})
			})
		}
	}
}

func TestRegressionB01AssetOrderDoesNotChangeSnapshot(t *testing.T) {
	g, mock, _ := deleteFixture(t)
	ctx := context.Background()
	plan, err := g.PlanDelete(ctx, "demo", "0.1.1", false)
	if err != nil {
		t.Fatal(err)
	}
	item := plan.Items[0]
	reorder := func() {
		mock.AlterRelease(item.ReleaseID, func(r *testutil.Release) {
			r.Assets[0], r.Assets[1] = r.Assets[1], r.Assets[0]
		})
	}
	reorder()
	injected := false
	g.client.Transport = deletionMutationTransport{g.client.Transport, func(req *http.Request, res *http.Response) {
		if !injected && req.Method == "POST" && strings.HasSuffix(req.URL.Path, "/git/refs") && res.StatusCode >= 200 && res.StatusCode < 300 {
			injected = true
			reorder()
		}
	}}
	if err = g.Delete(ctx, plan); err != nil {
		t.Fatal("asset ordering must not invalidate an unchanged set:", err)
	}
	if !injected {
		t.Fatal("final reorder was not injected")
	}
}
