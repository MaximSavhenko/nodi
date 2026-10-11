// Package project holds local project bindings, never credentials.
package project

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"nodi/internal/archive"
	"nodi/internal/config"
	"nodi/internal/model"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Schema      int               `json:"schema"`
	Project     string            `json:"project"`
	Repository  string            `json:"repository"`
	LastVersion string            `json:"last_version,omitempty"`
	Description string            `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}
type Binding struct {
	Root   string
	Config Config
}

func (c Config) Validate() error {
	if c.Schema != 1 {
		return fmt.Errorf("unsupported project configuration schema %d", c.Schema)
	}
	if e := model.ValidateProject(c.Project); e != nil {
		return e
	}
	if e := config.ValidateRepo(c.Repository); e != nil {
		return e
	}
	if c.LastVersion != "" {
		return model.ValidateVersion(c.LastVersion)
	}
	return nil
}
func Read(root string) (Binding, error) {
	var b Binding
	b.Root = root
	path := filepath.Join(root, ".nodi", "project.json")
	if e := archive.CheckParents(filepath.Dir(path)); e != nil {
		return b, e
	}
	st, e := os.Lstat(path)
	if e != nil {
		return b, e
	}
	if !st.Mode().IsRegular() {
		return b, fmt.Errorf("project config must be a regular file")
	}
	f, e := os.Open(path)
	if e != nil {
		return b, e
	}
	defer f.Close()
	data, e := io.ReadAll(io.LimitReader(f, model.MaxManifest+1))
	if e != nil {
		return b, e
	}
	if int64(len(data)) > model.MaxManifest {
		return b, fmt.Errorf("project config too large")
	}
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if e = d.Decode(&b.Config); e != nil {
		return b, fmt.Errorf("invalid project.json: %w", e)
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return b, fmt.Errorf("trailing project configuration data")
	}
	return b, b.Config.Validate()
}
func Find(start string) (Binding, error) {
	root, e := filepath.Abs(start)
	if e != nil {
		return Binding{}, e
	}
	for {
		_, e = os.Lstat(filepath.Join(root, ".nodi", "project.json"))
		if e == nil {
			return Read(root)
		}
		if !os.IsNotExist(e) {
			return Binding{}, e
		}
		parent := filepath.Dir(root)
		if parent == root {
			return Binding{}, fmt.Errorf("no initialized project found; run nodi init PROJECT in the project folder")
		}
		root = parent
	}
}
func Init(root string, c Config) (Binding, bool, error) {
	if e := c.Validate(); e != nil {
		return Binding{}, false, e
	}
	root, e := filepath.Abs(root)
	if e != nil {
		return Binding{}, false, e
	}
	if e = archive.CheckParents(root); e != nil {
		return Binding{}, false, e
	}
	path := filepath.Join(root, ".nodi", "project.json")
	if _, e = os.Lstat(path); e == nil {
		b, e := Read(root)
		if e == nil && (b.Config.Project != c.Project || b.Config.Repository != c.Repository) {
			e = fmt.Errorf("project already initialized as %s in %s; existing settings preserved", b.Config.Project, b.Config.Repository)
		}
		return b, false, e
	} else if !os.IsNotExist(e) {
		return Binding{}, false, e
	}
	if e = archive.CheckParents(filepath.Dir(path)); e != nil {
		return Binding{}, false, e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return Binding{}, false, e
	}
	data, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return Binding{}, false, e
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return Binding{}, false, e
	}
	_, e = f.Write(append(data, '\n'))
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	return Binding{root, c}, true, e
}
func (b Binding) SaveVersion(v string) error {
	if e := model.ValidateVersion(v); e != nil {
		return e
	}
	current, e := Read(b.Root)
	if e != nil {
		return e
	}
	if current.Config.Project != b.Config.Project || current.Config.Repository != b.Config.Repository {
		return fmt.Errorf("project binding changed during publication")
	}
	current.Config.LastVersion = v
	data, e := json.MarshalIndent(current.Config, "", "  ")
	if e != nil {
		return e
	}
	return config.WritePrivate(filepath.Join(b.Root, ".nodi", "project.json"), append(data, '\n'))
}

type Changes struct{ Added, Modified, Deleted int }

func Diff(local, remote []model.File) Changes {
	old := map[string]model.File{}
	for _, f := range remote {
		old[f.Path] = f
	}
	var c Changes
	for _, f := range local {
		if prev, ok := old[f.Path]; !ok {
			c.Added++
		} else if f.SHA256 != prev.SHA256 || f.Size != prev.Size {
			c.Modified++
		}
		delete(old, f.Path)
	}
	c.Deleted = len(old)
	return c
}
func (c Changes) Empty() bool { return c.Added+c.Modified+c.Deleted == 0 }
func NextVersion(current, bump string) (string, error) {
	if bump != "patch" && bump != "minor" && bump != "major" {
		return "", fmt.Errorf("invalid version increment")
	}
	if current == "" {
		return "0.1.0", nil
	}
	if e := model.ValidateVersion(current); e != nil {
		return "", e
	}
	parts := strings.Split(current, ".")
	n := make([]int, 3)
	for i, p := range parts {
		n[i], _ = strconv.Atoi(p)
	}
	switch bump {
	case "major":
		n[0]++
		n[1] = 0
		n[2] = 0
	case "minor":
		n[1]++
		n[2] = 0
	default:
		n[2]++
	}
	next := fmt.Sprintf("%d.%d.%d", n[0], n[1], n[2])
	return next, model.ValidateVersion(next)
}
func Scan(ctx context.Context, b Binding) ([]model.File, error) { return archive.Scan(ctx, b.Root) }
