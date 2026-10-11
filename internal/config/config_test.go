package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCredentials(t *testing.T) {
	t.Setenv("NODI_CONFIG_DIR", t.TempDir())
	t.Setenv("NODI_REPO", "")
	t.Setenv("NODI_READ_TOKEN", "")
	t.Setenv("NODI_WRITE_TOKEN", "")
	if e := Save(Config{Repository: "owner/private"}); e != nil {
		t.Fatal(e)
	}
	c, e := Load()
	if e != nil || c.Repository != "owner/private" {
		t.Fatal(c, e)
	}
	if e = SaveToken("read-test-token", false); e != nil {
		t.Fatal(e)
	}
	if e = SaveToken("write-test-token", true); e != nil {
		t.Fatal(e)
	}
	for write, want := range map[bool]string{false: "read-test-token", true: "write-test-token"} {
		s, e := Token(write)
		if e != nil || s != want {
			t.Fatal("credential round trip", e)
		}
	}
	p, _ := Dir()
	b, _ := os.ReadFile(filepath.Join(p, "config.json"))
	if bytes.Contains(b, []byte("token")) {
		t.Fatal("token in config")
	}
	if e = Logout(); e != nil {
		t.Fatal(e)
	}
	if _, e = Token(false); e == nil {
		t.Fatal("logout retained token")
	}
}
func TestTokenInputAndRepository(t *testing.T) {
	s, e := ReadToken(bytes.NewBufferString("test-token\n"))
	if e != nil || s != "test-token" {
		t.Fatal(e)
	}
	for _, repo := range []string{"", "owner", "../bad/repo", "https://github.com/owner/repo", "owner/repo/extra"} {
		if ValidateRepo(repo) == nil {
			t.Fatal("accepted", repo)
		}
	}
	if _, e := ReadToken(bytes.NewBufferString("a b\n")); e == nil {
		t.Fatal("invalid token")
	}
}
