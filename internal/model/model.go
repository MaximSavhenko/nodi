package model

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const MaxArchive int64 = 256 << 20
const MaxExpanded int64 = 512 << 20
const MaxManifest int64 = 4 << 20
const MaxFiles = 10000
const ManifestName = "nodi-manifest.json"
const ArchiveName = "package.zip"

var projectRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var versionRE = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)

type File struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	Schema        int               `json:"schema"`
	Project       string            `json:"project"`
	Version       string            `json:"version"`
	Description   string            `json:"description,omitempty"`
	Status        string            `json:"status"`
	PublishedAt   time.Time         `json:"published_at"`
	ArchiveSHA256 string            `json:"archive_sha256"`
	ArchiveSize   int64             `json:"archive_size"`
	Files         []File            `json:"files"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

func ValidateProject(s string) error {
	if !projectRE.MatchString(s) || Reserved(s) {
		return fmt.Errorf("invalid project %q: use lowercase letters, digits and hyphens; start with a letter", s)
	}
	return nil
}
func ValidateVersion(s string) error {
	if !versionRE.MatchString(s) {
		return fmt.Errorf("invalid version %q: expected MAJOR.MINOR.PATCH (for example 1.0.0)", s)
	}
	return nil
}
func CompareVersion(a, b string) int {
	aa, bb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		x, _ := strconv.Atoi(aa[i])
		y, _ := strconv.Atoi(bb[i])
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}
func Tag(project, version string) string { return project + "/v" + version }
func ParseTag(tag string) (string, string, bool) {
	p, v, ok := strings.Cut(tag, "/v")
	return p, v, ok && ValidateProject(p) == nil && ValidateVersion(v) == nil
}
func Reserved(s string) bool {
	s = strings.ToUpper(strings.Split(s, ".")[0])
	return s == "CON" || s == "PRN" || s == "AUX" || s == "NUL" || (len(s) == 4 && (strings.HasPrefix(s, "COM") || strings.HasPrefix(s, "LPT")) && s[3] >= '0' && s[3] <= '9')
}
func SafePath(s string) error {
	if s == "" || len(s) > 240 || path.IsAbs(s) || path.Clean(s) != s || strings.ContainsAny(s, `\:<>"|?*`) {
		return fmt.Errorf("unsafe file path %q", s)
	}
	for _, p := range strings.Split(s, "/") {
		if p == "." || p == ".." || p == "" || strings.TrimRight(p, ". ") != p || Reserved(p) {
			return fmt.Errorf("unsafe file path %q", s)
		}
		for _, r := range p {
			if r < 32 || r == 127 {
				return fmt.Errorf("unsafe file path %q", s)
			}
		}
	}
	return nil
}
func ValidHash(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && s == strings.ToLower(s)
}
func (m Manifest) Validate() error {
	if m.Schema != 1 {
		return fmt.Errorf("unsupported manifest schema %d", m.Schema)
	}
	if e := ValidateProject(m.Project); e != nil {
		return e
	}
	if e := ValidateVersion(m.Version); e != nil {
		return e
	}
	if m.Status != "draft" && m.Status != "released" {
		return fmt.Errorf("invalid manifest status")
	}
	if m.PublishedAt.IsZero() || len(m.Description) > 4096 {
		return fmt.Errorf("invalid date or description")
	}
	if len(m.Metadata) > 100 {
		return fmt.Errorf("too many metadata fields")
	}
	for k, v := range m.Metadata {
		if len(k) > 128 || len(v) > 4096 {
			return fmt.Errorf("metadata field exceeds limits")
		}
	}
	if !ValidHash(m.ArchiveSHA256) || m.ArchiveSize <= 0 || m.ArchiveSize > MaxArchive {
		return fmt.Errorf("invalid archive checksum or size")
	}
	if len(m.Files) == 0 || len(m.Files) > MaxFiles {
		return fmt.Errorf("invalid file count")
	}
	seen := map[string]bool{}
	var total int64
	for _, f := range m.Files {
		if e := SafePath(f.Path); e != nil {
			return e
		}
		key := strings.ToLower(f.Path)
		if seen[key] || f.Size < 0 || f.Size > MaxExpanded || !ValidHash(f.SHA256) {
			return fmt.Errorf("invalid or duplicate file %q", f.Path)
		}
		seen[key] = true
		total += f.Size
		if total > MaxExpanded {
			return fmt.Errorf("expanded package exceeds 512 MiB")
		}
	}
	for name := range seen {
		for p := path.Dir(name); p != "."; p = path.Dir(p) {
			if seen[p] {
				return fmt.Errorf("file/directory collision %q", name)
			}
		}
	}
	return nil
}
func Decode(r io.Reader) (Manifest, error) {
	var m Manifest
	b, e := io.ReadAll(io.LimitReader(r, MaxManifest+1))
	if e != nil {
		return m, e
	}
	if int64(len(b)) > MaxManifest {
		return m, fmt.Errorf("manifest exceeds 4 MiB")
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if e = d.Decode(&m); e != nil {
		return m, fmt.Errorf("invalid manifest JSON: %w", e)
	}
	var extra any
	if e = d.Decode(&extra); e != io.EOF {
		return m, fmt.Errorf("trailing manifest data")
	}
	return m, m.Validate()
}
