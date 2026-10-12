// Package archive builds bounded manufacturing packages and verifies every
// byte before installing downloaded files. It never extracts arbitrary ZIPs.
package archive

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"nodi/internal/config"
	"nodi/internal/model"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.r.Read(b)
}
func HashFile(name string) (string, int64, error) {
	f, e := os.Open(name)
	if e != nil {
		return "", 0, e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, e
}

func Prepare(ctx context.Context, source, project, version, status, description string, metadata map[string]string) (model.Manifest, string, error) {
	m := model.Manifest{Schema: 1, Project: project, Version: version, Status: status, Description: description, PublishedAt: time.Now().UTC(), Metadata: metadata}
	if e := model.ValidateProject(project); e != nil {
		return m, "", e
	}
	if e := model.ValidateVersion(version); e != nil {
		return m, "", e
	}
	if status != "draft" && status != "released" {
		return m, "", fmt.Errorf("choose --status draft or --status released explicitly")
	}
	source, names, e := sourceNames(ctx, source, true)
	if e != nil {
		return m, "", e
	}
	out, e := os.CreateTemp("", "nodi-package-*.zip")
	if e != nil {
		return m, "", e
	}
	name := out.Name()
	success := false
	defer func() {
		out.Close()
		if !success {
			os.Remove(name)
		}
	}()
	z := zip.NewWriter(out)
	var total int64
	for _, rel := range names {
		p := filepath.Join(source, filepath.FromSlash(rel))
		st, e := os.Lstat(p)
		if e != nil {
			return m, "", e
		}
		if !st.Mode().IsRegular() {
			return m, "", fmt.Errorf("not a regular file: %s", rel)
		}
		f, e := os.Open(p)
		if e != nil {
			return m, "", e
		}
		actual, e := f.Stat()
		if e != nil {
			f.Close()
			return m, "", e
		}
		if !os.SameFile(st, actual) {
			f.Close()
			return m, "", fmt.Errorf("source changed during packaging: %s", rel)
		}
		hdr := &zip.FileHeader{Name: rel, Method: zip.Deflate}
		hdr.SetMode(0644)
		hdr.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
		w, e := z.CreateHeader(hdr)
		if e != nil {
			f.Close()
			return m, "", e
		}
		h := sha256.New()
		n, e := io.Copy(io.MultiWriter(w, h), io.LimitReader(contextReader{ctx, f}, model.MaxExpanded-total+1))
		f.Close()
		if e != nil {
			return m, "", e
		}
		total += n
		if total > model.MaxExpanded {
			return m, "", fmt.Errorf("source exceeds 512 MiB")
		}
		m.Files = append(m.Files, model.File{Path: rel, Size: n, SHA256: hex.EncodeToString(h.Sum(nil))})
	}
	if e = z.Close(); e != nil {
		return m, "", e
	}
	if e = out.Close(); e != nil {
		return m, "", e
	}
	m.ArchiveSHA256, m.ArchiveSize, e = HashFile(name)
	if e != nil {
		return m, "", e
	}
	if e = m.Validate(); e != nil {
		return m, "", e
	}
	success = true
	return m, name, nil
}

// CheckParents refuses symlink/reparse-point paths before creating or opening
// files. This is not a defence against an attacker racing local directory edits.
func CheckParents(p string) error {
	p, e := filepath.Abs(p)
	if e != nil {
		return e
	}
	for {
		st, e := os.Lstat(p)
		if e == nil {
			if st.Mode()&os.ModeSymlink != 0 && nativeSystemAlias(p) {
				// macOS owns /var, /tmp and /etc aliases; user-created links
				// and every archive link remain forbidden.
			} else if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
				return fmt.Errorf("unsafe parent directory: %s", p)
			}
		} else if !os.IsNotExist(e) {
			return e
		}
		next := filepath.Dir(p)
		if next == p {
			break
		}
		p = next
	}
	return nil
}

func Extract(ctx context.Context, name string, m model.Manifest, destination string) error {
	return ExtractWithCommit(ctx, name, m, destination, nil)
}
func ExtractWithCommit(ctx context.Context, name string, m model.Manifest, destination string, beforeCommit func(string) error) error {
	if e := m.Validate(); e != nil {
		return e
	}
	st, e := os.Lstat(name)
	if e != nil {
		return e
	}
	if !st.Mode().IsRegular() || st.Size() != m.ArchiveSize {
		return fmt.Errorf("archive file type or size mismatch")
	}
	hash, size, e := HashFile(name)
	if e != nil {
		return e
	}
	if hash != m.ArchiveSHA256 || size != m.ArchiveSize {
		return fmt.Errorf("archive SHA-256 or size mismatch")
	}
	dest, e := filepath.Abs(destination)
	if e != nil {
		return e
	}
	if _, e = os.Lstat(dest); !os.IsNotExist(e) {
		return fmt.Errorf("destination already exists or is inaccessible: %s", dest)
	}
	parent := filepath.Dir(dest)
	if e = CheckParents(parent); e != nil {
		return e
	}
	if e = os.MkdirAll(parent, 0755); e != nil {
		return e
	}
	stage, e := os.MkdirTemp(parent, ".nodi-stage-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(stage)
	z, e := zip.OpenReader(name)
	if e != nil {
		return e
	}
	defer z.Close()
	if len(z.File) != len(m.Files) {
		return fmt.Errorf("ZIP file count differs from manifest")
	}
	expected := map[string]model.File{}
	for _, f := range m.Files {
		expected[f.Path] = f
	}
	seen := map[string]bool{}
	for _, entry := range z.File {
		if e = ctx.Err(); e != nil {
			return e
		}
		if e = model.SafePath(entry.Name); e != nil {
			return e
		}
		if !entry.Mode().IsRegular() || entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("non-regular ZIP entry: %s", entry.Name)
		}
		f, ok := expected[entry.Name]
		if !ok || seen[entry.Name] {
			return fmt.Errorf("unexpected or duplicate ZIP entry: %s", entry.Name)
		}
		seen[entry.Name] = true
		if entry.UncompressedSize64 != uint64(f.Size) {
			return fmt.Errorf("ZIP size differs from manifest: %s", entry.Name)
		}
		p := filepath.Join(stage, filepath.FromSlash(entry.Name))
		if e = os.MkdirAll(filepath.Dir(p), 0755); e != nil {
			return e
		}
		r, e := entry.Open()
		if e != nil {
			return e
		}
		out, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if e != nil {
			r.Close()
			return e
		}
		h := sha256.New()
		n, copyErr := io.Copy(io.MultiWriter(out, h), io.LimitReader(contextReader{ctx, r}, f.Size+1))
		if copyErr == nil {
			copyErr = out.Sync()
		}
		closeErr := out.Close()
		r.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if n != f.Size || hex.EncodeToString(h.Sum(nil)) != f.SHA256 {
			return fmt.Errorf("file SHA-256 or size mismatch: %s", entry.Name)
		}
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if e = CheckParents(parent); e != nil {
		return e
	}
	if beforeCommit != nil {
		if e = beforeCommit(stage); e != nil {
			return e
		}
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if e = commitDirectory(stage, dest); e != nil {
		return fmt.Errorf("atomic no-replace installation refused: %w", e)
	}
	return nil
}
func sourceNames(ctx context.Context, source string, requireFiles bool) (string, []string, error) {
	return sourceNamesReport(ctx, source, requireFiles, nil)
}

// Excluded records a file or a whole pruned directory (with a trailing slash).
type Excluded struct{ Path, Reason string }

func Exclusions(ctx context.Context, source string) ([]Excluded, error) {
	var excluded []Excluded
	_, _, err := sourceNamesReport(ctx, source, false, &excluded)
	return excluded, err
}

func sourceNamesReport(ctx context.Context, source string, requireFiles bool, excluded *[]Excluded) (string, []string, error) {
	source, e := filepath.Abs(source)
	if e != nil {
		return "", nil, e
	}
	root, e := os.Lstat(source)
	if e != nil {
		return "", nil, e
	}
	if root.Mode()&os.ModeSymlink != 0 || (!root.IsDir() && !root.Mode().IsRegular()) {
		return "", nil, fmt.Errorf("source must be a regular file or real directory")
	}
	parent := source
	if !root.IsDir() {
		parent = filepath.Dir(source)
	}
	if e = CheckParents(parent); e != nil {
		return "", nil, e
	}
	configDir, e := config.Dir()
	if e != nil {
		return "", nil, e
	}
	if protectedPath(source, configDir) {
		return "", nil, fmt.Errorf("Nodi configuration and credentials cannot be sent")
	}
	if excludedName(filepath.Base(source)) {
		return "", nil, fmt.Errorf("source is excluded by the file policy: %s", filepath.Base(source))
	}
	if !root.IsDir() {
		name := filepath.Base(source)
		if excludedName(name) {
			return "", nil, fmt.Errorf("source is excluded by the file policy: %s", name)
		}
		if e = model.SafePath(name); e != nil {
			return "", nil, e
		}
		return parent, []string{name}, nil
	}
	var names []string
	e = filepath.WalkDir(source, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		if p == source {
			return nil
		}
		if excludedName(d.Name()) || protectedPath(p, configDir) {
			if excluded != nil {
				rel, _ := filepath.Rel(source, p)
				reason := "credential, key or service-file policy"
				if strings.HasPrefix(d.Name(), ".") {
					reason = "hidden path"
				}
				if protectedPath(p, configDir) {
					reason = "Nodi configuration and credentials"
				}
				if d.IsDir() {
					rel += "/"
					reason += " (entire directory excluded)"
				}
				*excluded = append(*excluded, Excluded{filepath.ToSlash(rel), reason})
			}
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not allowed: %s", p)
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("not a regular file: %s", p)
		}
		rel, e := filepath.Rel(source, p)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		if e = model.SafePath(rel); e != nil {
			return e
		}
		names = append(names, rel)
		if len(names) > model.MaxFiles {
			return fmt.Errorf("too many files")
		}
		return nil
	})
	if e != nil {
		return "", nil, e
	}
	if len(names) == 0 && requireFiles {
		return "", nil, fmt.Errorf("no transferable files after excluding hidden, credential and service files")
	}
	sort.Strings(names)
	return source, names, nil
}

func protectedPath(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Conservative name policy, not a content-based secret detector. Users must
// select a delivery folder; secrets disguised as ordinary documents cannot be
// recognized reliably. These exclusions also apply to explicitly sent files.
func excludedName(name string) bool {
	n := strings.ToLower(name)
	if strings.HasPrefix(n, ".") || strings.Contains(n, "credential") || strings.Contains(n, "secret") || strings.HasPrefix(n, "token.") || strings.Contains(n, "-token") || strings.Contains(n, "_token") || strings.HasPrefix(n, "password") || strings.HasPrefix(n, "id_rsa") || strings.HasPrefix(n, "id_ed25519") || strings.HasPrefix(n, "id_ecdsa") {
		return true
	}
	switch n {
	case "node_modules", "__pycache__", "venv", "env", "build", "dist", "target", "coverage", "logs", "thumbs.db", "desktop.ini", "config.json", "auth.json", "credential", "token", "tokens.json":
		return true
	}
	switch strings.ToLower(filepath.Ext(n)) {
	case ".credential", ".pem", ".key", ".p12", ".pfx", ".keystore", ".kdbx", ".log", ".tmp", ".bak":
		return true
	}
	return false
}
