package archive

import (
	"context"
	"encoding/json"
	"fmt"
	"nodi/internal/model"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestB04ExtractionChild(t *testing.T) {
	if os.Getenv("NODI_EXTRACT_CHILD") == "" {
		return
	}
	var m model.Manifest
	b, err := os.ReadFile(os.Getenv("NODI_EXTRACT_MANIFEST"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if err = Extract(context.Background(), os.Getenv("NODI_EXTRACT_ZIP"), m, os.Getenv("NODI_EXTRACT_DEST")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Minute)
}
func TestRegressionB04CrashNeverPartial(t *testing.T) {
	source := t.TempDir()
	for i := 0; i < 1200; i++ {
		if err := os.WriteFile(filepath.Join(source, fmt.Sprintf("f%04d.txt", i)), []byte("fixture"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	m, z, err := Prepare(context.Background(), source, "demo", "0.1.0", "released", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(z)
	root := t.TempDir()
	dest := filepath.Join(root, "installed")
	manifest := filepath.Join(root, "manifest.json")
	data, _ := json.Marshal(m)
	os.WriteFile(manifest, data, 0600)
	child := exec.Command(os.Args[0], "-test.run=^TestB04ExtractionChild$")
	child.Env = append(os.Environ(), "NODI_EXTRACT_CHILD=1", "NODI_EXTRACT_MANIFEST="+manifest, "NODI_EXTRACT_ZIP="+z, "NODI_EXTRACT_DEST="+dest)
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { child.Process.Kill(); child.Wait() }()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err = os.Lstat(dest); err == nil {
			child.Process.Kill()
			child.Wait()
			entries, err := os.ReadDir(dest)
			if err != nil || len(entries) != len(m.Files) {
				t.Fatalf("visible final folder is incomplete after crash: %d/%d (%v)", len(entries), len(m.Files), err)
			}
			for _, f := range m.Files {
				h, n, err := HashFile(filepath.Join(dest, f.Path))
				if err != nil || h != f.SHA256 || n != f.Size {
					t.Fatal("corrupt installed file", f.Path, err)
				}
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("child never published destination")
}

func TestB04NoReplaceAtCommit(t *testing.T) {
	source := t.TempDir()
	os.WriteFile(filepath.Join(source, "data.txt"), []byte("fixture"), 0644)
	m, z, err := Prepare(context.Background(), source, "demo", "0.1.0", "released", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(z)
	for _, kind := range []string{"empty", "nonempty", "file", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "destination")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := ExtractWithCommit(ctx, z, m, dest, func(string) error {
				switch kind {
				case "file":
					return os.WriteFile(dest, []byte("existing"), 0644)
				case "cancel":
					cancel()
					return nil
				default:
					if err := os.Mkdir(dest, 0755); err != nil {
						return err
					}
					if kind == "nonempty" {
						return os.WriteFile(filepath.Join(dest, "keep.txt"), []byte("existing"), 0644)
					}
					return nil
				}
			})
			if err == nil {
				t.Fatal("unsafe destination accepted")
			}
			if kind == "cancel" {
				if _, err = os.Stat(dest); !os.IsNotExist(err) {
					t.Fatal("cancel published destination")
				}
			} else if kind == "file" {
				b, _ := os.ReadFile(dest)
				if string(b) != "existing" {
					t.Fatal("overwrote destination")
				}
			} else {
				if _, err = os.Stat(filepath.Join(dest, "data.txt")); !os.IsNotExist(err) {
					t.Fatal("installed into existing destination")
				}
			}
		})
	}
}
