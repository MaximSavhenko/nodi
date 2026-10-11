package archive

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"nodi/internal/model"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	source := t.TempDir()
	os.Mkdir(filepath.Join(source, "nested"), 0755)
	os.WriteFile(filepath.Join(source, "nested", "деталь.dxf"), []byte("fake DXF"), 0644)
	os.WriteFile(filepath.Join(source, "drawing.pdf"), []byte("fake PDF"), 0644)
	os.WriteFile(filepath.Join(source, ".env"), []byte("secret"), 0600)
	os.WriteFile(filepath.Join(source, "token.json"), []byte("secret"), 0600)
	m, z, e := Prepare(ctx, source, "macstand", "1.0.0", "released", "demo", map[string]string{"material": "test"})
	if e != nil {
		t.Fatal(e)
	}
	defer os.Remove(z)
	if len(m.Files) != 2 {
		t.Fatal("exported unsupported/secret files")
	}
	b, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	parsed, e := model.Decode(bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	dest := filepath.Join(t.TempDir(), "download")
	if e = Extract(ctx, z, parsed, dest); e != nil {
		t.Fatal(e)
	}
	for _, f := range m.Files {
		hash, n, e := HashFile(filepath.Join(dest, filepath.FromSlash(f.Path)))
		if e != nil || n != f.Size || hash != f.SHA256 {
			t.Fatal("file integrity")
		}
	}
	if e = Extract(ctx, z, m, dest); e == nil {
		t.Fatal("repeat download overwrote files")
	}
	if _, e = model.Decode(bytes.NewReader(append(b, []byte(" {}")...))); e == nil {
		t.Fatal("trailing JSON accepted")
	}
	// Identical source contents give identical ZIPs, independent of publish time.
	second, z2, e := Prepare(ctx, source, "macstand", "1.0.1", "draft", "", nil)
	if e != nil {
		t.Fatal(e)
	}
	defer os.Remove(z2)
	if second.ArchiveSHA256 != m.ArchiveSHA256 {
		t.Fatal("non deterministic ZIP")
	}
}
func crafted(t *testing.T, name string, mode os.FileMode) (model.Manifest, string) {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	h := &zip.FileHeader{Name: name, Method: zip.Deflate}
	h.SetMode(mode)
	w, e := z.CreateHeader(h)
	if e != nil {
		t.Fatal(e)
	}
	w.Write([]byte("test"))
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	fhash := sha256.Sum256([]byte("test"))
	zhash := sha256.Sum256(b.Bytes())
	m := model.Manifest{Schema: 1, Project: "demo", Version: "1.0.0", Status: "released", PublishedAt: time.Now(), ArchiveSize: int64(b.Len()), ArchiveSHA256: hex.EncodeToString(zhash[:]), Files: []model.File{{Path: "part.dxf", Size: 4, SHA256: hex.EncodeToString(fhash[:])}}}
	file := filepath.Join(t.TempDir(), "test.zip")
	os.WriteFile(file, b.Bytes(), 0600)
	return m, file
}
func TestHostileZIP(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode os.FileMode
	}{{"../escaped.dxf", 0644}, {"/absolute.dxf", 0644}, {`C:\absolute.dxf`, 0644}, {"part.dxf", os.ModeSymlink | 0777}, {"part.dxf", os.ModeDir | 0755}, {"other.dxf", 0644}, {"CON.dxf", 0644}} {
		t.Run(tc.name, func(t *testing.T) {
			m, z := crafted(t, tc.name, tc.mode)
			dest := filepath.Join(t.TempDir(), "result")
			if e := Extract(context.Background(), z, m, dest); e == nil {
				t.Fatal("accepted hostile ZIP")
			}
			if _, e := os.Stat(dest); !os.IsNotExist(e) {
				t.Fatal("left unverified output")
			}
		})
	}
}
func TestCorruption(t *testing.T) {
	for _, kind := range []string{"archive", "file", "size"} {
		t.Run(kind, func(t *testing.T) {
			m, z := crafted(t, "part.dxf", 0644)
			switch kind {
			case "archive":
				m.ArchiveSHA256 = string(bytes.Repeat([]byte("0"), 64))
			case "file":
				m.Files[0].SHA256 = string(bytes.Repeat([]byte("0"), 64))
			case "size":
				m.Files[0].Size = 2
			}
			if e := Extract(context.Background(), z, m, filepath.Join(t.TempDir(), "result")); e == nil {
				t.Fatal("accepted corruption")
			}
		})
	}
}
func TestSymlinksAndEmptySource(t *testing.T) {
	source := t.TempDir()
	if _, _, e := Prepare(context.Background(), source, "demo", "1.0.0", "draft", "", nil); e == nil {
		t.Fatal("accepted empty source")
	}
	target := filepath.Join(t.TempDir(), "part.dxf")
	os.WriteFile(target, []byte("test"), 0644)
	if e := os.Symlink(target, filepath.Join(source, "part.dxf")); e != nil {
		t.Skip("host does not permit symlink creation")
	}
	if _, _, e := Prepare(context.Background(), source, "demo", "1.0.0", "draft", "", nil); e == nil {
		t.Fatal("accepted source symlink")
	}
}
func TestLimitsAndCollisions(t *testing.T) {
	m, _ := crafted(t, "part.dxf", 0644)
	m.ArchiveSize = model.MaxArchive + 1
	if m.Validate() == nil {
		t.Fatal("archive size limit")
	}
	m.ArchiveSize = 100
	m.Files = append(m.Files, model.File{Path: "PART.dxf", Size: 4, SHA256: m.Files[0].SHA256})
	if m.Validate() == nil {
		t.Fatal("case collision")
	}
	m.Files[1].Path = "part.dxf/nested.pdf"
	if m.Validate() == nil {
		t.Fatal("file directory collision")
	}
	m.Files = m.Files[:1]
	m.Files[0].Size = model.MaxExpanded + 1
	if m.Validate() == nil {
		t.Fatal("expanded size limit")
	}
}

func TestDuplicateZIPEntries(t *testing.T) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for i := 0; i < 2; i++ {
		w, e := z.Create("part.dxf")
		if e != nil {
			t.Fatal(e)
		}
		w.Write([]byte("test"))
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(t.TempDir(), "duplicate.zip")
	os.WriteFile(file, b.Bytes(), 0600)
	hash := sha256.Sum256(b.Bytes())
	fh := sha256.Sum256([]byte("test"))
	m := model.Manifest{Schema: 1, Project: "demo", Version: "1.0.0", Status: "released", PublishedAt: time.Now(), ArchiveSize: int64(b.Len()), ArchiveSHA256: hex.EncodeToString(hash[:]), Files: []model.File{{Path: "part.dxf", Size: 4, SHA256: hex.EncodeToString(fh[:])}, {Path: "second.dxf", Size: 4, SHA256: hex.EncodeToString(fh[:])}}}
	if e := Extract(context.Background(), file, m, filepath.Join(t.TempDir(), "result")); e == nil {
		t.Fatal("accepted duplicate ZIP paths")
	}
}

func TestCancelledPackaging(t *testing.T) {
	source := t.TempDir()
	os.WriteFile(filepath.Join(source, "part.dxf"), []byte("test"), 0644)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, e := Prepare(ctx, source, "demo", "1.0.0", "draft", "", nil); e == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestTemporaryParentsAndUserLinks(t *testing.T) {
	if e := CheckParents(t.TempDir()); e != nil {
		t.Fatalf("native temporary directory must work: %v", e)
	}
	parent := t.TempDir()
	real := t.TempDir()
	link := filepath.Join(parent, "user-link")
	if e := os.Symlink(real, link); e != nil {
		t.Skip("host does not permit directory symlinks")
	}
	if e := CheckParents(filepath.Join(link, "output")); e == nil {
		t.Fatal("accepted a user-created parent symlink")
	}
}
