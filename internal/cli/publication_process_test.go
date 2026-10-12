package cli

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net/http"
	"nodi/internal/archive"
	"nodi/internal/project"
	"nodi/internal/storage"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type pausePublicationUpload struct{ base http.RoundTripper }

func (p pausePublicationUpload) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == "POST" && req.URL.Query().Get("name") == "package.zip" {
		fmt.Println("B02_CHILD_DURABLE_UPLOAD_INTENT")
		select {}
	}
	return p.base.RoundTrip(req)
}

func TestB02PublicationAcrossProcesses(t *testing.T) {
	if source := os.Getenv("NODI_B02_CHILD_SOURCE"); source != "" {
		der, err := base64.StdEncoding.DecodeString(os.Getenv("NODI_B02_CHILD_CERT"))
		if err != nil {
			t.Fatal(err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		roots := x509.NewCertPool()
		roots.AddCert(cert)
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}, Timeout: 30 * time.Second}
		if os.Getenv("NODI_B02_CHILD_PAUSE") == "1" {
			client.Transport = pausePublicationUpload{client.Transport}
		}
		a := New("dev", strings.NewReader(""), os.Stdout)
		a.Cwd = func() (string, error) { return source, nil }
		a.StoreFor = func(repo string, write bool) (storage.Store, error) {
			token := "read-test"
			if write {
				token = "write-test"
			}
			endpoint := os.Getenv("NODI_B02_CHILD_ENDPOINT")
			return storage.NewGitHubWithClient(repo, token, endpoint, endpoint, client)
		}
		if err = a.Run(context.Background(), []string{"send", source}); err != nil {
			t.Fatal(err)
		}
		return
	}
	a, fixture, _, source := projectApp(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := func(pause bool) *exec.Cmd {
		cmd := exec.Command(executable, "-test.run=^TestB02PublicationAcrossProcesses$")
		cmd.Env = append(os.Environ(), "NODI_B02_CHILD_SOURCE="+source, "NODI_B02_CHILD_ENDPOINT="+fixture.Server.URL, "NODI_B02_CHILD_CERT="+base64.StdEncoding.EncodeToString(fixture.Server.Certificate().Raw))
		if pause {
			cmd.Env = append(cmd.Env, "NODI_B02_CHILD_PAUSE=1")
		}
		return cmd
	}
	crashing := child(true)
	stdout, err := crashing.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = crashing.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = crashing.Process.Kill() })
	deadline := time.AfterFunc(20*time.Second, func() { _ = crashing.Process.Kill() })
	defer deadline.Stop()
	ready := false
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if scanner.Text() == "B02_CHILD_DURABLE_UPLOAD_INTENT" {
			ready = true
			break
		}
	}
	if !ready {
		_ = crashing.Wait()
		t.Fatal("child did not durably prepare its upload")
	}
	dir := publicationTestDirectory(t)
	op := publicationTestPayload(t, filepath.Join(dir, "operation.json"))
	manifest := op["manifest"].(map[string]any)
	hash, size, err := archive.HashFile(filepath.Join(dir, "archive.zip"))
	if err != nil || hash != manifest["archive_sha256"] || float64(size) != manifest["archive_size"] {
		t.Fatal("prepared ZIP not preserved:", hash, size, err)
	}
	if err = crashing.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = crashing.Wait()
	deadline.Stop()
	if output, err := child(false).CombinedOutput(); err != nil {
		t.Fatalf("fresh process failed to resume: %v\n%s", err, output)
	}
	binding, err := project.Read(source)
	if err != nil || binding.Config.LastVersion != "0.1.0" {
		t.Fatal(binding, err)
	}
	if _, err = os.Stat(filepath.Join(dir, "archive.zip")); !os.IsNotExist(err) {
		t.Fatal("acknowledged spool not removed:", err)
	}
	if output, err := child(false).CombinedOutput(); err != nil {
		t.Fatalf("completed process retry failed: %v\n%s", err, output)
	}
	s, err := a.StoreFor("owner/private", true)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := s.ProjectList(context.Background(), "demo", true)
	if err != nil || len(versions) != 1 {
		t.Fatal("another version created after process restart:", versions, err)
	}
}
