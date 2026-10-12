package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"nodi/internal/identity"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRegressionB05WindowsPathCase(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows case alias required")
	}
	a, _, _, root := projectApp(t)
	os.RemoveAll(filepath.Join(root, ".nodi"))
	source := filepath.Join(root, "part.dxf")
	alias := strings.ToUpper(source)
	x, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	y, err := os.Stat(alias)
	if err != nil || !os.SameFile(x, y) {
		t.Fatal("fixture is not a case alias", err)
	}
	if err = a.Run(context.Background(), []string{"send", source, "--name", "demo"}); err != nil {
		t.Fatal(err)
	}
	if err = a.Run(context.Background(), []string{"send", alias}); err != nil {
		t.Fatal("same object lost identity", err)
	}
	r, err := readSendReceipt(alias, "owner/private")
	if err != nil || r == nil || r.Name != "demo" {
		t.Fatal(r, err)
	}
}

func TestB05LegacyAndRecreatedSource(t *testing.T) {
	a, _, _, root := projectApp(t)
	os.RemoveAll(filepath.Join(root, ".nodi"))
	source := filepath.Join(root, "part.dxf")
	legacy := sendReceipt{Source: source, Repository: "owner/private", Name: "demo"}
	data, _ := json.Marshal(legacy)
	dir := filepath.Join(os.Getenv("NODI_CONFIG_DIR"), "sent")
	os.MkdirAll(dir, 0700)
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(source+"\x00owner/private")))
	os.WriteFile(filepath.Join(dir, key+".json"), data, 0600)
	if err := a.Run(context.Background(), []string{"send", source}); err != nil {
		t.Fatal(err)
	}
	r, err := readSendReceipt(source, "owner/private")
	if err != nil || r == nil || r.Name != "demo" {
		t.Fatal(r, err)
	}
	// Keep the old object alive at another path, so inode recycling is impossible.
	moved := filepath.Join(root, "prior.dxf")
	if err = os.Rename(source, moved); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(source, []byte("fictional v1"), 0644); err != nil {
		t.Fatal(err)
	}
	r, err = readSendReceipt(source, "owner/private")
	if err != nil {
		t.Fatal(err)
	}
	if r != nil {
		t.Fatal("new object at old pathname adopted identity")
	}
	if !identity.Same(moved, moved) || identity.Same(source, moved) {
		t.Fatal("fixture identity mismatch")
	}
}

func TestB05DistinctCaseObjects(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "case.txt")
	b := filepath.Join(root, "CASE.txt")
	os.WriteFile(a, []byte("a"), 0644)
	os.WriteFile(b, []byte("b"), 0644)
	if identity.Same(a, b) {
		t.Skip("filesystem is case insensitive; case-sensitive negative case NOT VERIFIED")
	}
	ka, err := identity.Key(a)
	if err != nil {
		t.Fatal(err)
	}
	kb, err := identity.Key(b)
	if err != nil {
		t.Fatal(err)
	}
	if ka == kb {
		t.Fatal("distinct case-sensitive objects combined")
	}
}
