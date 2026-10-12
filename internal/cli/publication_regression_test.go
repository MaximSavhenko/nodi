package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"nodi/internal/project"
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
					before := len(failure.commits)
					if err := restarted.Run(ctx, []string{"send", root}); err == nil {
						t.Fatal("ambiguous tag ownership accepted")
					}
					if len(failure.commits) != before {
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

// Keep io imported for the response-body failure oracle used below.
var _ io.Reader
