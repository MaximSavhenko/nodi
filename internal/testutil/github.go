// Package testutil contains only the in-process GitHub API fixture used by
// tests. It is not imported by the CLI and is never a running Nodi service.
package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
)

type Asset struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	State string `json:"state"`
}
type Release struct {
	ID         int64   `json:"id"`
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}
type GitHub struct {
	Server         *httptest.Server
	mu             sync.Mutex
	releases       []Release
	data           map[int64][]byte
	next           int64
	Private        bool
	FailUpload     bool
	FailFinalize   bool
	CorruptArchive bool
	ForceStatus    int
	Requests       []string
}

func NewGitHub() *GitHub {
	g := &GitHub{Private: true, data: map[int64][]byte{}, next: 1}
	g.Server = httptest.NewTLSServer(http.HandlerFunc(g.serve))
	return g
}
func (g *GitHub) Close() { g.Server.Close() }

// Configure serializes failure injection with the HTTP handler for race tests.
func (g *GitHub) Configure(fn func(*GitHub)) { g.mu.Lock(); defer g.mu.Unlock(); fn(g) }
func (g *GitHub) serve(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Requests = append(g.Requests, r.Method+" "+r.URL.RequestURI())
	if g.ForceStatus != 0 {
		w.WriteHeader(g.ForceStatus)
		return
	}
	if r.Header.Get("Authorization") != "Bearer read-test" && r.Header.Get("Authorization") != "Bearer write-test" {
		w.WriteHeader(401)
		return
	}
	if r.Method != "GET" && r.Header.Get("Authorization") != "Bearer write-test" {
		w.WriteHeader(403)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/repos/owner/private")
	if p == r.URL.Path {
		w.WriteHeader(404)
		return
	}
	respond := func(v any) { w.Header().Set("Content-Type", "application/json"); json.NewEncoder(w).Encode(v) }
	if p == "" {
		respond(map[string]bool{"private": g.Private})
		return
	}
	if strings.HasPrefix(p, "/git/ref/") {
		w.WriteHeader(404)
		return
	}
	if p == "/releases" && r.Method == "GET" {
		visible := []Release{}
		for _, release := range g.releases {
			if !release.Draft || r.Header.Get("Authorization") == "Bearer write-test" {
				visible = append(visible, release)
			}
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		start := (page - 1) * 100
		if start >= len(visible) {
			respond([]Release{})
			return
		}
		end := start + 100
		if end > len(visible) {
			end = len(visible)
		}
		respond(visible[start:end])
		return
	}
	if p == "/releases" && r.Method == "POST" {
		var release Release
		if json.NewDecoder(r.Body).Decode(&release) != nil {
			w.WriteHeader(400)
			return
		}
		for _, existing := range g.releases {
			if existing.Tag == release.Tag {
				w.WriteHeader(422)
				return
			}
		}
		release.ID = g.next
		g.next++
		g.releases = append(g.releases, release)
		respond(release)
		return
	}
	if strings.HasPrefix(p, "/releases/tags/") {
		tag := strings.TrimPrefix(p, "/releases/tags/")
		for _, release := range g.releases {
			if release.Tag == tag && (!release.Draft || r.Header.Get("Authorization") == "Bearer write-test") {
				respond(release)
				return
			}
		}
		w.WriteHeader(404)
		return
	}
	if strings.HasPrefix(p, "/releases/assets/") {
		id, _ := strconv.ParseInt(strings.TrimPrefix(p, "/releases/assets/"), 10, 64)
		b, ok := g.data[id]
		if !ok {
			w.WriteHeader(404)
			return
		}
		if g.CorruptArchive && len(b) > 2 && string(b[:2]) == "PK" {
			b = append([]byte(nil), b...)
			b[len(b)-1] ^= 1
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(b)
		return
	}
	if strings.HasPrefix(p, "/releases/") {
		parts := strings.Split(strings.TrimPrefix(p, "/releases/"), "/")
		id, _ := strconv.ParseInt(parts[0], 10, 64)
		index := -1
		for i, v := range g.releases {
			if v.ID == id {
				index = i
			}
		}
		if index < 0 {
			w.WriteHeader(404)
			return
		}
		if len(parts) == 2 && parts[1] == "assets" && r.Method == "POST" {
			if g.FailUpload {
				w.WriteHeader(503)
				return
			}
			b, e := io.ReadAll(r.Body)
			if e != nil {
				w.WriteHeader(500)
				return
			}
			a := Asset{ID: g.next, Name: r.URL.Query().Get("name"), Size: int64(len(b)), State: "uploaded"}
			g.next++
			g.data[a.ID] = b
			g.releases[index].Assets = append(g.releases[index].Assets, a)
			respond(a)
			return
		}
		if len(parts) == 1 && r.Method == "PATCH" {
			if g.FailFinalize {
				w.WriteHeader(503)
				return
			}
			var patch struct {
				Draft      bool `json:"draft"`
				Prerelease bool `json:"prerelease"`
			}
			if json.NewDecoder(r.Body).Decode(&patch) != nil {
				w.WriteHeader(400)
				return
			}
			g.releases[index].Draft = patch.Draft
			g.releases[index].Prerelease = patch.Prerelease
			respond(g.releases[index])
			return
		}
	}
	w.WriteHeader(404)
}
