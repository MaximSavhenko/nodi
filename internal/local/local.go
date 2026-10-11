package local

import (
	"encoding/json"
	"fmt"
	"nodi/internal/config"
	"nodi/internal/model"
	"os"
	"path/filepath"
	"time"
)

type Download struct {
	Repository   string    `json:"repository"`
	Project      string    `json:"project"`
	Version      string    `json:"version"`
	Path         string    `json:"path"`
	DownloadedAt time.Time `json:"downloaded_at"`
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
	record := Download{repo, m.Project, m.Version, path, time.Now().UTC()}
	b, e := json.MarshalIndent(record, "", "  ")
	if e != nil {
		return e
	}
	return config.WritePrivate(filepath.Join(dir, "downloads", m.Project, m.Version+".json"), b)
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
		if d.Repository != repo || d.Project != project || model.ValidateVersion(d.Version) != nil {
			continue
		}
		if d.DownloadedAt.After(latest.DownloadedAt) {
			st, e := os.Stat(d.Path)
			if e == nil && st.IsDir() {
				latest = d
			}
		}
	}
	if latest.Path == "" {
		return "", fmt.Errorf("no existing local download for %q", project)
	}
	return latest.Path, nil
}
