package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"nodi/internal/archive"
	"nodi/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAPIErrors(t *testing.T) {
	for _, status := range []int{401, 403, 404, 422, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			mock := testutil.NewGitHub()
			defer mock.Close()
			mock.Configure(func(g *testutil.GitHub) { g.ForceStatus = status })
			g, e := NewGitHubWithClient("owner/private", "read-test", mock.Server.URL, mock.Server.URL, mock.Server.Client())
			if e != nil {
				t.Fatal(e)
			}
			_, e = g.List(context.Background(), false)
			if !IsStatus(e, status) {
				t.Fatal(e)
			}
			if strings.Contains(e.Error(), "read-test") {
				t.Fatal("token leaked")
			}
		})
	}
}
func TestPrivateAndNetwork(t *testing.T) {
	mock := testutil.NewGitHub()
	g, e := NewGitHubWithClient("owner/private", "read-test", mock.Server.URL, mock.Server.URL, mock.Server.Client())
	if e != nil {
		t.Fatal(e)
	}
	mock.Configure(func(g *testutil.GitHub) { g.Private = false })
	if e = g.VerifyPrivate(context.Background()); e == nil {
		t.Fatal("public repository accepted")
	}
	mock.Close()
	if e = g.VerifyPrivate(context.Background()); e == nil || strings.Contains(e.Error(), "read-test") {
		t.Fatal("network error", e)
	}
	if _, e = NewGitHubWithClient("owner/private", "read-test", "http://localhost", "https://localhost", &http.Client{}); e == nil {
		t.Fatal("HTTP accepted")
	}
}
func TestPagination(t *testing.T) {
	var pages atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/private" {
			io.WriteString(w, `{"private":true}`)
			return
		}
		pages.Add(1)
		if r.URL.Query().Get("page") == "1" {
			io.WriteString(w, "["+strings.TrimSuffix(strings.Repeat(`{"tag_name":"unrelated"},`, 100), ",")+"]")
		} else {
			io.WriteString(w, "[]")
		}
	}))
	defer server.Close()
	g, _ := NewGitHubWithClient("owner/private", "read-test", server.URL, server.URL, server.Client())
	ms, e := g.List(context.Background(), false)
	if e != nil || len(ms) != 0 || pages.Load() != 2 {
		t.Fatal("pagination", pages.Load(), e)
	}
}
func TestFailedUploadRetainsDraft(t *testing.T) {
	mock := testutil.NewGitHub()
	defer mock.Close()
	mock.Configure(func(g *testutil.GitHub) { g.FailUpload = true })
	g, _ := NewGitHubWithClient("owner/private", "write-test", mock.Server.URL, mock.Server.URL, mock.Server.Client())
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "part.dxf"), []byte("fake"), 0644)
	m, z, e := archive.Prepare(context.Background(), src, "demo", "1.0.0", "released", "", nil)
	if e != nil {
		t.Fatal(e)
	}
	defer os.Remove(z)
	e = g.Publish(context.Background(), m, z)
	if e == nil || !strings.Contains(e.Error(), "draft release") {
		t.Fatal(e)
	}
	ms, e := g.List(context.Background(), false)
	if e != nil || len(ms) != 0 {
		t.Fatal("partial published", e)
	}
	if e = g.Publish(context.Background(), m, z); e == nil || !strings.Contains(e.Error(), "already exists") {
		t.Fatal("overwrote failed draft", e)
	}
}
func TestRedirectSecurity(t *testing.T) {
	targetAuth := make(chan string, 1)
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetAuth <- r.Header.Get("Authorization")
		io.WriteString(w, `{"private":true}`)
	}))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer source.Close()
	g, _ := NewGitHubWithClient("owner/private", "read-test", source.URL, source.URL, source.Client())
	if e := g.VerifyPrivate(context.Background()); e != nil {
		t.Fatal(e)
	}
	if <-targetAuth != "" {
		t.Fatal("forwarded token to redirect target")
	}
	downgrade := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://localhost/private", http.StatusFound)
	}))
	defer downgrade.Close()
	g, _ = NewGitHubWithClient("owner/private", "read-test", downgrade.URL, downgrade.URL, downgrade.Client())
	if e := g.VerifyPrivate(context.Background()); e == nil {
		t.Fatal("HTTPS downgrade accepted")
	}
}
func TestReadTokenCannotPublish(t *testing.T) {
	mock := testutil.NewGitHub()
	defer mock.Close()
	g, _ := NewGitHubWithClient("owner/private", "read-test", mock.Server.URL, mock.Server.URL, mock.Server.Client())
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "part.dxf"), []byte("fake"), 0644)
	m, z, e := archive.Prepare(context.Background(), src, "demo", "1.0.0", "draft", "", nil)
	if e != nil {
		t.Fatal(e)
	}
	defer os.Remove(z)
	if e = g.Publish(context.Background(), m, z); !IsStatus(e, 403) {
		t.Fatal(e)
	}
	var buf bytes.Buffer
	if e = g.Download(context.Background(), m, &buf); e == nil {
		t.Fatal("missing asset downloaded")
	}
}

func TestFinalizeFailure(t *testing.T) {
	mock := testutil.NewGitHub()
	defer mock.Close()
	mock.Configure(func(g *testutil.GitHub) { g.FailFinalize = true })
	g, _ := NewGitHubWithClient("owner/private", "write-test", mock.Server.URL, mock.Server.URL, mock.Server.Client())
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "part.dxf"), []byte("fake"), 0644)
	m, z, e := archive.Prepare(context.Background(), src, "demo", "1.0.0", "released", "", nil)
	if e != nil {
		t.Fatal(e)
	}
	defer os.Remove(z)
	e = g.Publish(context.Background(), m, z)
	if e == nil || !strings.Contains(e.Error(), "not confirmed") {
		t.Fatal(e)
	}
	ms, e := g.List(context.Background(), false)
	if e != nil || len(ms) != 0 {
		t.Fatal("released partial upload", e)
	}
}
