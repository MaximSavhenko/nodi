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
	"nodi/internal/model"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var formats = map[string]bool{".dxf": true, ".pdf": true, ".step": true, ".stp": true, ".iges": true, ".igs": true, ".stl": true, ".3mf": true, ".svg": true, ".dwg": true}

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
	source, e := filepath.Abs(source)
	if e != nil {
		return m, "", e
	}
	root, e := os.Lstat(source)
	if e != nil {
		return m, "", e
	}
	if !root.IsDir() || root.Mode()&os.ModeSymlink != 0 {
		return m, "", fmt.Errorf("source must be a real directory")
	}
	if e = CheckParents(source); e != nil {
		return m, "", e
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
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not allowed: %s", p)
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !formats[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil
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
		return m, "", e
	}
	if len(names) == 0 {
		return m, "", fmt.Errorf("no supported manufacturing files (DXF, PDF, STEP, IGES, STL, 3MF, SVG, DWG)")
	}
	sort.Strings(names)
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
			if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
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
	// Exclusively reserve the destination; never rename over an existing folder.
	if e = os.Mkdir(dest, 0755); e != nil {
		return fmt.Errorf("destination already exists or cannot be created: %w", e)
	}
	success := false
	defer func() {
		if !success {
			os.RemoveAll(dest)
		}
	}()
	entries, e := os.ReadDir(stage)
	if e != nil {
		return e
	}
	for _, entry := range entries {
		if e = os.Rename(filepath.Join(stage, entry.Name()), filepath.Join(dest, entry.Name())); e != nil {
			return e
		}
	}
	success = true
	return nil
}
