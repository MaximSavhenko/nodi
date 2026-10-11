package archive

import (
	"context"
	"nodi/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func TestTransferPolicyExcludesSecretsAndServices(t *testing.T) {
	source := t.TempDir()
	t.Setenv("NODI_CONFIG_DIR", filepath.Join(source, "user-nodi"))
	for _, name := range []string{".env", ".nodi/read.credential", ".git/config", "credentials.json", "aws_credentials.txt", "my-secret.txt", "github-token.txt", "private.key", "cert.pem", "passwords.csv", "config.json", "auth.json", "node_modules/a.txt", "build/a.txt", "dist/a.txt", "logs/output.txt", "user-nodi/custom-name.txt"} {
		p := filepath.Join(source, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0700)
		os.WriteFile(p, []byte("do not deliver"), 0600)
	}
	os.WriteFile(filepath.Join(source, "data.bin"), []byte("safe fixture"), 0644)
	files, e := Scan(context.Background(), source)
	if e != nil || len(files) != 1 || files[0].Path != "data.bin" {
		t.Fatal(files, e)
	}
	for _, name := range []string{"credentials.json", "private.key", ".nodi", "user-nodi"} {
		if _, _, e := Prepare(context.Background(), filepath.Join(source, name), "demo", "0.1.0", "released", "", nil); e == nil {
			t.Fatal("explicit secret source accepted", name)
		}
	}
	dir, _ := config.Dir()
	if _, e := Scan(context.Background(), dir); e == nil {
		t.Fatal("config directory accepted")
	}
}
