package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"nodi/internal/archive"
	"nodi/internal/config"
	"nodi/internal/identity"
	"nodi/internal/model"
	"os"
	"path/filepath"
	"strings"
)

type installIntent struct {
	Schema                          int
	Repository, Destination, Object string
	Manifest                        model.Manifest
}

func installIntentPath(repo, dest string) (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	key := sha256.Sum256([]byte(strings.ToLower(repo) + "\x00" + dest))
	return filepath.Join(dir, "download-installs", hex.EncodeToString(key[:])+".json"), nil
}

func Install(ctx context.Context, repo string, m model.Manifest, name, dest string) error {
	dest, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	intentPath, err := installIntentPath(repo, dest)
	if err != nil {
		return err
	}
	// The entire directory identity is recorded after extraction and before rename.
	err = archive.ExtractWithCommit(ctx, name, m, dest, func(stage string) error {
		object, err := identity.Key(stage)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(installIntent{1, strings.ToLower(repo), dest, object, m})
		if err != nil {
			return err
		}
		digest := sha256.Sum256(payload)
		data, err := json.Marshal(struct {
			Payload json.RawMessage
			SHA256  string
		}{payload, hex.EncodeToString(digest[:])})
		if err != nil {
			return err
		}
		return config.WritePrivate(intentPath, data)
	})
	if err != nil {
		return err
	}
	if err = Record(repo, m, dest); err != nil {
		return fmt.Errorf("download committed at %s; repeat get to reconcile its saved intent: %w", dest, err)
	}
	return os.Remove(intentPath)
}

// Reconcile accepts only the exact directory moved by a saved local intent.
// Equal names or bytes alone do not authorize adoption of another destination.
func Reconcile(ctx context.Context, repo string, m model.Manifest, dest string) (bool, error) {
	dest, err := filepath.Abs(dest)
	if err != nil {
		return false, err
	}
	path, err := installIntentPath(repo, dest)
	if err != nil {
		return false, err
	}
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !st.Mode().IsRegular() || st.Size() > model.MaxManifest+65536 {
		return false, fmt.Errorf("invalid download intent")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var envelope struct {
		Payload json.RawMessage
		SHA256  string
	}
	if err = json.Unmarshal(data, &envelope); err != nil {
		return false, err
	}
	h := sha256.Sum256(envelope.Payload)
	if hex.EncodeToString(h[:]) != envelope.SHA256 {
		return false, fmt.Errorf("download intent checksum mismatch")
	}
	var intent installIntent
	if err = json.Unmarshal(envelope.Payload, &intent); err != nil {
		return false, err
	}
	a, _ := json.Marshal(m)
	b, _ := json.Marshal(intent.Manifest)
	if intent.Schema != 1 || intent.Repository != strings.ToLower(repo) || intent.Destination != dest || string(a) != string(b) || intent.Manifest.Validate() != nil {
		return false, fmt.Errorf("download intent identity or manifest differs")
	}
	if _, err = os.Lstat(dest); os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = archive.CheckParents(dest); err != nil {
		return false, err
	}
	object, err := identity.Key(dest)
	if err != nil {
		return false, err
	}
	if object != intent.Object {
		return false, fmt.Errorf("destination is not the directory from the saved download intent")
	}
	expected := map[string]model.File{}
	for _, f := range m.Files {
		expected[f.Path] = f
	}
	count := 0
	err = filepath.WalkDir(dest, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in committed download")
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("nonregular committed download")
		}
		rel, e := filepath.Rel(dest, p)
		if e != nil {
			return e
		}
		f, ok := expected[filepath.ToSlash(rel)]
		if !ok {
			return fmt.Errorf("unknown file in committed download")
		}
		h, n, e := archive.HashFile(p)
		if e != nil || h != f.SHA256 || n != f.Size {
			return fmt.Errorf("committed download hash mismatch: %s", rel)
		}
		count++
		return nil
	})
	if err != nil {
		return false, err
	}
	if count != len(m.Files) {
		return false, fmt.Errorf("committed download is incomplete")
	}
	if err = Record(repo, m, dest); err != nil {
		return false, err
	}
	if err = os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
}
