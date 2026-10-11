package cli

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"nodi/internal/config"
	"nodi/internal/model"
	"nodi/internal/storage"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed shells/nodi.bash
var bashCompletion string

//go:embed shells/_nodi
var zshCompletion string

//go:embed shells/nodi.ps1
var powershellCompletion string

func (a *App) completion(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(a.Out, `Generate a completion script: nodi completion zsh|bash|powershell
Enable in this session (no profile changes):
  zsh: autoload -Uz compinit; compinit; source <(nodi completion zsh)
  bash: source <(nodi completion bash)
  PowerShell: nodi completion powershell | Out-String | Invoke-Expression
Homebrew installs zsh/bash completion files automatically.
For persistent completion, run nodi setup --completion (asks before profile edits).
nodi setup --completion --check is read-only; nodi setup configures storage access.
Dynamic suggestions use saved credentials, a 1.5 second timeout and a 60 second cache.
Offline suggestions may be stale. No credentials prompt is ever shown on Tab.`)
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("use nodi completion [zsh|bash|powershell]")
	}
	var script string
	switch args[0] {
	case "bash":
		script = bashCompletion
	case "zsh":
		script = zshCompletion
	case "powershell":
		script = powershellCompletion
	default:
		return fmt.Errorf("supported shells: zsh, bash, powershell")
	}
	_, e := fmt.Fprint(a.Out, script)
	return e
}

type suggestion struct {
	Project string `json:"project"`
	Version string `json:"version"`
}
type completionCache struct {
	Schema  int          `json:"schema"`
	Updated time.Time    `json:"updated"`
	Items   []suggestion `json:"items"`
}

func validCache(c completionCache) bool {
	if c.Schema != 1 || len(c.Items) > model.MaxFiles || c.Updated.IsZero() {
		return false
	}
	for _, s := range c.Items {
		if model.ValidateProject(s.Project) != nil || model.ValidateVersion(s.Version) != nil {
			return false
		}
	}
	return true
}
func (a *App) remoteSuggestions(ctx context.Context, write, useBinding bool) []suggestion {
	repo := ""
	if useBinding {
		if b, e := a.binding(); e == nil {
			repo = b.Config.Repository
		}
	}
	if repo == "" {
		if c, e := config.Load(); e == nil {
			repo = c.Repository
		}
	}
	if repo == "" {
		return nil
	}
	token, e := config.Token(write)
	if e != nil {
		return nil
	}
	digest := sha256.Sum256([]byte(repo + "\x00" + fmt.Sprint(write) + "\x00" + token))
	dir, e := config.Dir()
	if e != nil {
		return nil
	}
	path := filepath.Join(dir, "completion", hex.EncodeToString(digest[:])+".json")
	var cached completionCache
	if st, e := os.Stat(path); e == nil && st.Size() <= model.MaxManifest {
		if data, e := os.ReadFile(path); e == nil {
			if json.Unmarshal(data, &cached) != nil || !validCache(cached) {
				cached = completionCache{}
			}
		}
	}
	age := time.Since(cached.Updated)
	if cached.Schema == 1 && age >= 0 && age < 60*time.Second {
		return cached.Items
	}
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	s, e := a.StoreFor(repo, write)
	if e != nil {
		return cached.Items
	}
	ms, e := s.Index(ctx, write)
	if e != nil {
		if storage.IsStatus(e, 401) || storage.IsStatus(e, 403) || storage.IsStatus(e, 404) {
			return nil
		}
		return cached.Items
	}
	items := []suggestion{}
	for _, m := range ms {
		if model.ValidateProject(m.Project) == nil && model.ValidateVersion(m.Version) == nil {
			items = append(items, suggestion{m.Project, m.Version})
		}
	}
	if len(items) > model.MaxFiles {
		return cached.Items
	}
	next := completionCache{Schema: 1, Updated: time.Now().UTC(), Items: items}
	data, e := json.Marshal(next)
	if e == nil && int64(len(data)) <= model.MaxManifest {
		_ = config.WritePrivate(path, data)
	}
	return items
}

var commandFlags = map[string][]string{
	"send": {"--name", "--dry-run"}, "delete": {"--version", "--all", "--dry-run"}, "update": {"--check"},
	"setup": {"--check", "--repo", "--write", "--completion"},
	"get":   {"--version", "--output"}, "publish": {"--version", "--status", "--draft", "--description", "--metadata", "--minor", "--major"},
	"list": {"--all"}, "info": {"--all"}, "config": {"--repo"}, "init": {"--repo"}, "auth": {"--write"},
}
var valueFlags = map[string]bool{"--version": true, "--output": true, "--status": true, "--description": true, "--metadata": true, "--repo": true, "--name": true}

func (a *App) complete(ctx context.Context, words []string) {
	// This internal protocol always exits successfully and only emits validated candidates.
	if len(words) == 0 {
		words = []string{""}
	}
	cur := words[len(words)-1]
	before := words[:len(words)-1]
	cmd := ""
	if len(before) > 0 {
		cmd = before[0]
	}
	emit := func(values []string, prefix string) {
		sort.Strings(values)
		seen := map[string]bool{}
		for _, v := range values {
			if !seen[v] && strings.HasPrefix(v, prefix) && !strings.ContainsAny(v, "\r\n\t") {
				fmt.Fprintln(a.Out, v)
				seen[v] = true
			}
		}
	}
	if len(before) == 0 {
		emit([]string{"version", "help", "completion", "setup", "init", "status", "publish", "send", "delete", "update", "get", "list", "info", "open", "config", "auth", "-v", "--version", "--help"}, cur)
		return
	}
	positionals := []string{}
	pending := ""
	write := cmd == "publish" || cmd == "delete"
	literal := false
	for _, w := range before[1:] {
		if pending != "" {
			pending = ""
			continue
		}
		if w == "--" {
			literal = true
			continue
		}
		if !literal && strings.HasPrefix(w, "--") {
			name, _, eq := strings.Cut(w, "=")
			if name == "--all" {
				write = true
			}
			if valueFlags[name] && !eq {
				pending = name
			}
			continue
		}
		positionals = append(positionals, w)
	}
	valuePrefix := cur
	equalPrefix := ""
	if !literal && strings.HasPrefix(cur, "--") {
		name, val, eq := strings.Cut(cur, "=")
		if eq && valueFlags[name] {
			pending = name
			valuePrefix = val
			equalPrefix = name + "="
		}
	}
	if pending != "" {
		switch pending {
		case "--output", "--metadata":
			fmt.Fprintln(a.Out, ":files")
		case "--status":
			emit([]string{equalPrefix + "draft", equalPrefix + "released"}, cur)
		case "--version":
			projectName := ""
			if len(positionals) > 0 {
				projectName = positionals[0]
			} else if cmd == "publish" {
				if b, e := a.binding(); e == nil {
					projectName = b.Config.Project
				}
			}
			if projectName != "" {
				var values []string
				for _, s := range a.remoteSuggestions(ctx, write, cmd == "publish") {
					if s.Project == projectName && strings.HasPrefix(s.Version, valuePrefix) {
						values = append(values, equalPrefix+s.Version)
					}
				}
				emit(values, cur)
			}
		}
		return
	}
	if !literal && strings.HasPrefix(cur, "-") {
		emit(commandFlags[cmd], cur)
		return
	}
	switch cmd {
	case "send":
		if len(positionals) == 0 {
			fmt.Fprintln(a.Out, ":files")
		}
	case "auth":
		if len(positionals) == 0 {
			emit([]string{"login", "logout"}, cur)
		}
	case "completion":
		if len(positionals) == 0 {
			emit([]string{"bash", "zsh", "powershell"}, cur)
		}
	case "get", "info", "open", "publish", "delete":
		if len(positionals) == 0 {
			var values []string
			for _, s := range a.remoteSuggestions(ctx, write, cmd == "publish") {
				values = append(values, s.Project)
			}
			emit(values, cur)
		} else if cmd == "publish" && len(positionals) == 1 {
			fmt.Fprintln(a.Out, ":files")
		}
	}
}
