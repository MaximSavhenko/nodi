package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"nodi/internal/project"
	"nodi/internal/storage"
	"nodi/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type publicationFailureTransport struct {
	base                    http.RoundTripper
	phase                   string
	after, fired, finalized bool
	commits                 map[string]int
}

func publicationStep(req *http.Request) string {
	p := req.URL.Path
	if req.Method == "POST" && strings.HasSuffix(p, "/git/refs") && req.GetBody != nil {
		body, _ := req.GetBody()
		defer body.Close()
		var input struct {
			Ref string `json:"ref"`
		}
		_ = json.NewDecoder(body).Decode(&input)
		if strings.HasPrefix(input.Ref, "refs/tags/demo/v") {
			return "reserve"
		}
	}
	if req.Method == "POST" && strings.HasSuffix(p, "/releases") {
		return "create"
	}
	if req.Method == "POST" && strings.HasSuffix(p, "/assets") {
		switch req.URL.Query().Get("name") {
		case "package.zip":
			return "archive"
		case "nodi-manifest.json":
			return "manifest"
		}
	}
	if req.Method == "PATCH" && strings.Contains(p, "/releases/") {
		return "finalize"
	}
	return ""
}

func (f *publicationFailureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	step := publicationStep(req)
	finalRead := f.finalized && req.Method == "GET" && strings.Contains(req.URL.Path, "/releases/tags/")
	match := step == f.phase || f.phase == "final_read" && finalRead
	if !f.fired && match && !f.after {
		f.fired = true
		return nil, fmt.Errorf("fictional lost response before mutation")
	}
	res, err := f.base.RoundTrip(req)
	if err == nil && res.StatusCode >= 200 && res.StatusCode < 300 {
		if step != "" {
			f.commits[step]++
		}
		if step == "finalize" {
			f.finalized = true
		}
		if !f.fired && match && f.after {
			f.fired = true
			res.Body.Close()
			return nil, fmt.Errorf("fictional lost response after mutation")
		}
	}
	return res, err
}

func TestRegressionB02InterruptedBoundSend(t *testing.T) {
	for _, phase := range []string{"reserve", "create", "archive", "manifest", "finalize", "final_read"} {
		for _, after := range []bool{false, true} {
			if phase == "final_read" && after {
				continue
			}
			t.Run(fmt.Sprintf("%s/after=%t", phase, after), func(t *testing.T) {
				a, fixture, _, root := projectApp(t)
				client := fixture.Server.Client()
				failure := &publicationFailureTransport{base: client.Transport, phase: phase, after: after, commits: map[string]int{}}
				client.Transport = failure
				ctx := context.Background()
				if err := a.Run(ctx, []string{"send", root}); err == nil {
					t.Fatal("interruption was hidden")
				}
				if !failure.fired {
					t.Fatal("failure was not injected")
				}
				// New App and provider instances must use persistent state, not the
				// first call's in-memory state or its temporary prepared archive.
				restarted := New("dev", strings.NewReader(""), new(bytes.Buffer))
				restarted.Cwd, restarted.StoreFor = a.Cwd, a.StoreFor
				if phase == "reserve" && after {
					before := 0
					for _, count := range failure.commits {
						before += count
					}
					if err := restarted.Run(ctx, []string{"send", root}); err == nil {
						t.Fatal("ambiguous tag ownership accepted")
					}
					afterCount := 0
					for _, count := range failure.commits {
						afterCount += count
					}
					if afterCount != before {
						t.Fatal("ambiguous retry performed a mutation")
					}
					return
				}
				if err := restarted.Run(ctx, []string{"send", root}); err != nil {
					t.Fatal("safe retry did not recover:", err)
				}
				binding, err := project.Read(root)
				if err != nil || binding.Config.LastVersion != "0.1.0" {
					t.Fatal("remote completion was not reconciled locally:", binding.Config.LastVersion, err)
				}
				s, err := restarted.StoreFor("owner/private", true)
				if err != nil {
					t.Fatal(err)
				}
				versions, err := s.ProjectList(ctx, "demo", true)
				if err != nil || len(versions) != 1 || versions[0].Version != "0.1.0" || versions[0].Status != "released" {
					t.Fatal("recovery created another version or retained incomplete state:", versions, err)
				}
				for _, step := range []string{"reserve", "create", "archive", "manifest", "finalize"} {
					if failure.commits[step] != 1 {
						t.Errorf("remote %s committed %d times, want one", step, failure.commits[step])
					}
				}
				if err := restarted.Run(ctx, []string{"send", root}); err != nil {
					t.Fatal("completed retry failed:", err)
				}
				for _, n := range failure.commits {
					if n != 1 {
						t.Fatal("completed retry performed another mutation")
					}
				}
			})
		}
	}
}

func publicationTestDirectory(t *testing.T) string {
	t.Helper()
	var found string
	err := filepath.WalkDir(filepath.Join(os.Getenv("NODI_CONFIG_DIR"), "publications"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Name() == "operation.json" {
			if found != "" {
				return fmt.Errorf("multiple test operations")
			}
			found = filepath.Dir(path)
		}
		return nil
	})
	if err != nil || found == "" {
		t.Fatal("persistent operation not found:", err)
	}
	return found
}

func publicationTestPayload(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err = json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err = json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestB02UnsafeRecoveryIsReadOnly(t *testing.T) {
	for _, mode := range []string{"corrupt_operation", "corrupt_checkpoint", "missing_archive", "changed_archive", "changed_source", "different_source", "foreign_release", "replaced_release", "changed_tag", "foreign_asset", "replaced_asset", "incomplete_asset", "premature_release", "changed_repository"} {
		t.Run(mode, func(t *testing.T) {
			a, fixture, _, root := projectApp(t)
			client := fixture.Server.Client()
			failure := &publicationFailureTransport{base: client.Transport, phase: "manifest", commits: map[string]int{}}
			client.Transport = failure
			ctx := context.Background()
			if err := a.Run(ctx, []string{"send", root}); err == nil {
				t.Fatal("interruption hidden")
			}
			dir := publicationTestDirectory(t)
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			var checkpoint string
			for _, entry := range entries {
				if len(entry.Name()) == 11 && strings.HasSuffix(entry.Name(), ".json") {
					checkpoint = filepath.Join(dir, entry.Name())
				}
			}
			state := publicationTestPayload(t, checkpoint)
			id := int64(state["release_id"].(float64))
			retrySource := root
			switch mode {
			case "corrupt_operation":
				err = os.WriteFile(filepath.Join(dir, "operation.json"), []byte("{partial"), 0600)
			case "corrupt_checkpoint":
				err = os.WriteFile(checkpoint, []byte("{partial"), 0600)
			case "missing_archive":
				err = os.Remove(filepath.Join(dir, "archive.zip"))
			case "changed_archive":
				err = os.WriteFile(filepath.Join(dir, "archive.zip"), []byte("different archive"), 0600)
			case "changed_source":
				err = os.WriteFile(filepath.Join(root, "part.dxf"), []byte("changed source"), 0644)
			case "different_source":
				retrySource = t.TempDir()
				data, readErr := os.ReadFile(filepath.Join(root, "part.dxf"))
				if readErr != nil {
					t.Fatal(readErr)
				}
				err = os.WriteFile(filepath.Join(retrySource, "part.dxf"), data, 0644)
			case "foreign_release":
				fixture.AlterRelease(id, func(r *testutil.Release) { r.Body = "a different writer" })
			case "replaced_release":
				fixture.AlterRelease(id, func(r *testutil.Release) { r.ID += 10000 })
			case "changed_tag":
				fixture.SetTag("refs/tags/demo/v0.1.0", strings.Repeat("b", 40))
			case "foreign_asset":
				fixture.AlterRelease(id, func(r *testutil.Release) {
					r.Assets = append(r.Assets, testutil.Asset{ID: 99999, Name: "original.step", Size: 9, State: "uploaded"})
				})
			case "replaced_asset":
				fixture.AlterRelease(id, func(r *testutil.Release) { r.Assets[0].ID += 10000 })
			case "incomplete_asset":
				fixture.AlterRelease(id, func(r *testutil.Release) { r.Assets[0].State = "starter" })
			case "premature_release":
				fixture.AlterRelease(id, func(r *testutil.Release) { r.Draft = false })
			case "changed_repository":
				fixture.Configure(func(g *testutil.GitHub) { g.RepositoryID = 202 })
			}
			if err != nil {
				t.Fatal(err)
			}
			before := 0
			fixture.Configure(func(g *testutil.GitHub) { before = len(g.Requests) })
			restarted := New("dev", strings.NewReader(""), new(bytes.Buffer))
			restarted.Cwd, restarted.StoreFor = a.Cwd, a.StoreFor
			var retryErr error
			if mode == "different_source" {
				s, err := restarted.StoreFor("owner/private", true)
				if err != nil {
					t.Fatal(err)
				}
				_, retryErr = s.(storage.PublicationResumer).ResumePublish(ctx, "demo", retrySource, "released", "")
			} else {
				retryErr = restarted.Run(ctx, []string{"send", retrySource})
			}
			if retryErr == nil {
				t.Fatal("unsafe state accepted:", mode)
			}
			fixture.Configure(func(g *testutil.GitHub) {
				for _, request := range g.Requests[before:] {
					if !strings.HasPrefix(request, "GET ") {
						t.Error("unsafe recovery mutated remote state:", request)
					}
				}
			})
			if _, err = os.Stat(filepath.Join(dir, "operation.json")); err != nil {
				t.Fatal("operation evidence removed:", err)
			}
		})
	}
}

func TestB02CompletedRemoteDoesNotNeedLocalArchive(t *testing.T) {
	a, fixture, _, root := projectApp(t)
	client := fixture.Server.Client()
	failure := &publicationFailureTransport{base: client.Transport, phase: "finalize", after: true, commits: map[string]int{}}
	client.Transport = failure
	if err := a.Run(context.Background(), []string{"send", root}); err == nil {
		t.Fatal("interruption hidden")
	}
	dir := publicationTestDirectory(t)
	if err := os.Remove(filepath.Join(dir, "archive.zip")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "part.dxf"), []byte("later source change"), 0644); err != nil {
		t.Fatal(err)
	}
	before := 0
	fixture.Configure(func(g *testutil.GitHub) { before = len(g.Requests) })
	restarted := New("dev", strings.NewReader(""), new(bytes.Buffer))
	restarted.Cwd, restarted.StoreFor = a.Cwd, a.StoreFor
	if err := restarted.Run(context.Background(), []string{"send", root}); err != nil {
		t.Fatal("proven remote completion did not reconcile:", err)
	}
	binding, err := project.Read(root)
	if err != nil || binding.Config.LastVersion != "0.1.0" {
		t.Fatal(binding, err)
	}
	fixture.Configure(func(g *testutil.GitHub) {
		for _, request := range g.Requests[before:] {
			if !strings.HasPrefix(request, "GET ") {
				t.Error("completed recovery repeated mutation:", request)
			}
		}
	})
}

func TestB02ExplicitDraftRecoveryPreservesDraftContract(t *testing.T) {
	a, fixture, _, root := projectApp(t)
	fixture.Configure(func(g *testutil.GitHub) { g.DraftTagUnavailable = true })
	client := fixture.Server.Client()
	failure := &publicationFailureTransport{base: client.Transport, phase: "archive", after: true, commits: map[string]int{}}
	client.Transport = failure
	args := []string{"publish", "demo", root, "--version", "0.1.0", "--draft"}
	if err := a.Run(context.Background(), args); err == nil {
		t.Fatal("draft interruption hidden")
	}
	restarted := New("dev", strings.NewReader(""), new(bytes.Buffer))
	restarted.Cwd, restarted.StoreFor = a.Cwd, a.StoreFor
	if err := restarted.Run(context.Background(), args); err != nil {
		t.Fatal("draft did not recover by exact ID:", err)
	}
	s, err := restarted.StoreFor("owner/private", true)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := s.ProjectList(context.Background(), "demo", true)
	if err != nil || len(versions) != 1 || versions[0].Status != "draft" {
		t.Fatal(versions, err)
	}
	if failure.commits["create"] != 1 || failure.commits["archive"] != 1 || failure.commits["manifest"] != 1 || failure.commits["finalize"] != 0 {
		t.Fatal("draft was duplicated or released:", failure.commits)
	}
}
