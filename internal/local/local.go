package local

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"nodi/internal/config"
	"nodi/internal/identity"
	"nodi/internal/model"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Download struct {
	Repository   string    `json:"repository"`
	Project      string    `json:"project"`
	Version      string    `json:"version"`
	Path         string    `json:"path"`
	DownloadedAt time.Time `json:"downloaded_at"`
	Object       string    `json:"object,omitempty"`
}

func Record(repo string, m model.Manifest, path string) error {
	path, e := filepath.Abs(path)
	if e != nil {
		return e
	}
	dir, e := config.Dir()
	if e != nil {
		return e
	}
	// One record per version/path avoids a shared mutable index or lost updates.
	if e := config.ValidateRepo(repo); e != nil {
		return e
	}
	if e := model.ValidateProject(m.Project); e != nil {
		return e
	}
	if e := model.ValidateVersion(m.Version); e != nil {
		return e
	}
	repo = strings.ToLower(repo)
	object, e := identity.Key(path)
	if e != nil {
		return e
	}
	record := Download{Repository: repo, Project: m.Project, Version: m.Version, Path: path, DownloadedAt: time.Now().UTC(), Object: object}
	b, e := json.MarshalIndent(record, "", "  ")
	if e != nil {
		return e
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(repo+"\x00"+m.Version+"\x00"+object)))
	return config.WritePrivate(filepath.Join(dir, "downloads", m.Project, key+".json"), b)
}
func Latest(repo, project string) (string, error) {
	if e := model.ValidateProject(project); e != nil {
		return "", e
	}
	dir, e := config.Dir()
	if e != nil {
		return "", e
	}
	entries, e := os.ReadDir(filepath.Join(dir, "downloads", project))
	if e != nil {
		return "", fmt.Errorf("no local downloads for %q", project)
	}
	var latest Download
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		b, e := os.ReadFile(filepath.Join(dir, "downloads", project, entry.Name()))
		if e != nil {
			return "", e
		}
		var d Download
		if e = json.Unmarshal(b, &d); e != nil {
			return "", fmt.Errorf("invalid local download record")
		}
		if !strings.EqualFold(d.Repository, repo) || d.Project != project || model.ValidateVersion(d.Version) != nil || !filepath.IsAbs(d.Path) {
			continue
		}
		if d.DownloadedAt.After(latest.DownloadedAt) || d.DownloadedAt.Equal(latest.DownloadedAt) && d.Path < latest.Path {
			st, e := os.Stat(d.Path)
			if e == nil && st.IsDir() {
				if d.Object != "" {
					key, err := identity.Key(d.Path)
					if err != nil || key != d.Object {
						continue
					}
				}
				latest = d
			}
		}
	}
	if latest.Path == "" {
		return "", fmt.Errorf("no existing local download for %q", project)
	}
	return latest.Path, nil
}
