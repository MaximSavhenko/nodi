package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"nodi/internal/archive"
	"nodi/internal/config"
	"nodi/internal/local"
	"nodi/internal/model"
	"nodi/internal/storage"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"text/tabwriter"
)

type App struct {
	Version  string
	In       io.Reader
	Out      io.Writer
	Store    func(bool) (storage.Store, error)
	StoreFor func(string, bool) (storage.Store, error)
	Cwd      func() (string, error)
	Open     func(string) error
}

func New(version string, in io.Reader, out io.Writer) *App {
	a := &App{Version: version, In: in, Out: out, Open: local.Open, Cwd: os.Getwd}
	a.StoreFor = func(repo string, write bool) (storage.Store, error) {
		t, e := config.Token(write)
		if e != nil {
			return nil, e
		}
		return storage.NewGitHub(repo, t)
	}
	a.Store = func(write bool) (storage.Store, error) {
		c, e := config.Load()
		if e != nil {
			return nil, e
		}
		return a.StoreFor(c.Repository, write)
	}
	return a
}

type options struct {
	version, output, status, description, metadata, repo string
	all, write, minor, major                             bool
	args                                                 []string
}

func parse(command string, args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	switch command {
	case "get":
		fs.StringVar(&o.version, "version", "", "version")
		fs.StringVar(&o.output, "output", "", "destination")
	case "publish":
		fs.StringVar(&o.version, "version", "", "version")
		fs.StringVar(&o.status, "status", "", "draft or released")
		fs.StringVar(&o.description, "description", "", "description")
		fs.StringVar(&o.metadata, "metadata", "", "JSON file")
		fs.BoolVar(&o.minor, "minor", false, "increment minor version")
		fs.BoolVar(&o.major, "major", false, "increment major version")
	case "list", "info":
		fs.BoolVar(&o.all, "all", false, "include drafts (write credential)")
	case "config":
		fs.StringVar(&o.repo, "repo", "", "OWNER/REPO")
	case "init":
		fs.StringVar(&o.repo, "repo", "", "OWNER/REPO")
	case "auth-login":
		fs.BoolVar(&o.write, "write", false, "store publish token")
	case "version", "help", "open", "auth-logout", "status":
	default:
		return o, fmt.Errorf("unknown command %q; run nodi help", command)
	}
	// flag.Parse stops at the first positional argument. Collect named arguments
	// so documented forms such as `get macstand --version 1.0.0` also work.
	var named, pos []string
	for i := 0; i < len(args); i++ {
		s := args[i]
		if s == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(s, "-") || s == "-" {
			pos = append(pos, s)
			continue
		}
		name, val, eq := strings.Cut(strings.TrimLeft(s, "-"), "=")
		f := fs.Lookup(name)
		if f == nil {
			return o, fmt.Errorf("unknown option %q for %s", s, command)
		}
		named = append(named, s)
		boolFlag, ok := f.Value.(interface{ IsBoolFlag() bool })
		if ok && boolFlag.IsBoolFlag() {
			continue
		}
		if eq {
			if val == "" {
				return o, fmt.Errorf("empty --%s value", name)
			}
			continue
		}
		i++
		if i >= len(args) {
			return o, fmt.Errorf("--%s requires a value", name)
		}
		named = append(named, args[i])
	}
	if e := fs.Parse(named); e != nil {
		return o, e
	}
	o.args = pos
	counts := map[string]int{"version": 0, "help": 0, "list": 0, "info": 1, "get": 1, "publish": 2, "open": 1, "config": 0, "auth-login": 0, "auth-logout": 0, "init": 1, "status": 0}
	if len(pos) != counts[command] && !(command == "publish" && len(pos) == 0) {
		return o, fmt.Errorf("%s expects %d positional arguments; run nodi help", command, counts[command])
	}
	if len(pos) > 0 {
		if e := model.ValidateProject(pos[0]); e != nil {
			return o, e
		}
	}
	if o.version != "" {
		if e := model.ValidateVersion(o.version); e != nil {
			return o, e
		}
	}
	if command == "publish" {
		if o.minor && o.major || (o.version != "" && (o.minor || o.major)) {
			return o, fmt.Errorf("choose only one of --version, --minor or --major")
		}
		if len(pos) == 2 && (o.minor || o.major) {
			return o, fmt.Errorf("--minor/--major require initialized-project publication")
		}
		if len(pos) == 2 && o.version == "" {
			return o, fmt.Errorf("publish requires --version MAJOR.MINOR.PATCH")
		}
		if len(pos) == 0 && o.status == "" {
			o.status = "draft"
		}
		if o.status != "draft" && o.status != "released" {
			return o, fmt.Errorf("publish requires explicit --status draft or --status released")
		}
	}
	return o, nil
}

const help = `Nodi — private engineering packages via GitHub Releases

  nodi version | nodi -v | nodi --version
  nodi completion [zsh|bash|powershell]
  nodi init PROJECT [--repo OWNER/PRIVATE-REPO]
  nodi status                     Read-only local/remote SHA-256 comparison
  nodi publish [--minor|--major]   Preview, confirm; default draft
  nodi publish --status released  Explicit manufacturing-publication confirmation
  nodi config --repo OWNER/PRIVATE-REPO
  nodi auth login                 Read token from hidden stdin
  nodi auth login --write         Separate publication token
  nodi auth logout                Remove saved credentials
  nodi list [--all]               Latest version per project
  nodi info macstand [--all]      Versions, formats, sizes and metadata
  nodi get macstand [--version 1.0.0] [--output PATH]
  nodi publish macstand ./manufacturing --version 1.0.0 --status draft
  nodi publish macstand ./manufacturing --version 1.0.0 --status released
       [--description TEXT] [--metadata metadata.json]
  nodi open macstand
  nodi help

Default destination: Desktop/Nodi/PROJECT/VERSION.
--output is the exact new destination directory. Existing paths are refused.
--all needs a write token because GitHub hides drafts from read-only users.
Automatic publication finds .nodi/project.json in this or an ancestor folder.
Drafts count towards versions; no changes creates no release. Legacy explicit syntax is preserved.
released is not CAD/manufacturing validation. Completion never edits shell profiles.
Versions: MAJOR.MINOR.PATCH, without a prefix. Projects: lowercase letters/digits/hyphens.
`

func (a *App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		_, e := fmt.Fprint(a.Out, help)
		return e
	}
	cmd := args[0]
	rest := args[1:]
	if cmd == "__complete" {
		if len(rest) == 1 && rest[0] == "--json" {
			if json.NewDecoder(io.LimitReader(a.In, 65536)).Decode(&rest) != nil {
				return nil
			}
		}
		if len(rest) > 256 {
			return nil
		}
		for _, word := range rest {
			if len(word) > 4096 {
				return nil
			}
		}
		a.complete(ctx, rest)
		return nil
	}
	if cmd == "completion" {
		return a.completion(rest)
	}
	if cmd == "--help" || cmd == "-h" {
		cmd = "help"
	}
	if cmd == "--version" || cmd == "-v" {
		cmd = "version"
	}
	if cmd == "auth" {
		if len(rest) == 0 || (rest[0] != "login" && rest[0] != "logout") {
			return fmt.Errorf("use nodi auth login [--write] or nodi auth logout")
		}
		cmd = "auth-" + rest[0]
		rest = rest[1:]
	}
	o, e := parse(cmd, rest)
	if e != nil {
		return e
	}
	switch cmd {
	case "help":
		_, e = fmt.Fprint(a.Out, help)
		return e
	case "version":
		_, e = fmt.Fprintf(a.Out, "nodi %s %s/%s\n", a.Version, runtime.GOOS, runtime.GOARCH)
		return e
	case "config":
		if e = config.Save(config.Config{Repository: o.repo}); e != nil {
			return e
		}
		fmt.Fprintln(a.Out, "Storage repository configured:", o.repo)
		return nil
	case "auth-login":
		c, e := config.Load()
		if e != nil {
			return e
		}
		fmt.Fprint(a.Out, "GitHub token (hidden; never pass as a command argument): ")
		t, e := config.ReadToken(a.In)
		fmt.Fprintln(a.Out)
		if e != nil {
			return e
		}
		g, e := storage.NewGitHub(c.Repository, t)
		if e != nil {
			return e
		}
		if e = g.VerifyPrivate(ctx); e != nil {
			return e
		}
		if e = config.SaveToken(t, o.write); e != nil {
			return e
		}
		role := "read"
		if o.write {
			role = "write"
		}
		fmt.Fprintln(a.Out, "Saved", role, "credential. GitHub enforces the token's actual repository permissions.")
		return nil
	case "auth-logout":
		if e = config.Logout(); e != nil {
			return e
		}
		fmt.Fprintln(a.Out, "Saved credentials removed.")
		return nil
	case "init":
		return a.initProject(o)
	case "status":
		return a.projectStatus(ctx)
	case "open":
		c, e := config.Load()
		if e != nil {
			return e
		}
		p, e := local.Latest(c.Repository, o.args[0])
		if e != nil {
			return e
		}
		if e = a.Open(p); e != nil {
			return e
		}
		fmt.Fprintln(a.Out, "Opened:", p)
		return nil
	}
	if cmd == "publish" {
		if len(o.args) == 0 {
			return a.autoPublish(ctx, o)
		}
		return a.publish(ctx, o)
	}
	store, e := a.Store(o.all)
	if e != nil {
		return e
	}
	switch cmd {
	case "list":
		ms, e := store.List(ctx, o.all)
		if e != nil {
			return e
		}
		sort.Slice(ms, func(i, j int) bool {
			if ms[i].Project != ms[j].Project {
				return ms[i].Project < ms[j].Project
			}
			return model.CompareVersion(ms[i].Version, ms[j].Version) > 0
		})
		w := tabwriter.NewWriter(a.Out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "PROJECT\tVERSION\tSTATUS\tDESCRIPTION")
		seen := map[string]bool{}
		for _, m := range ms {
			if seen[m.Project] {
				continue
			}
			seen[m.Project] = true
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", m.Project, m.Version, m.Status, cleanText(m.Description))
		}
		return w.Flush()
	case "info":
		ms, e := store.List(ctx, o.all)
		if e != nil {
			return e
		}
		var versions []model.Manifest
		for _, m := range ms {
			if m.Project == o.args[0] {
				versions = append(versions, m)
			}
		}
		if len(versions) == 0 {
			return fmt.Errorf("project %q is unavailable", o.args[0])
		}
		sort.Slice(versions, func(i, j int) bool { return model.CompareVersion(versions[i].Version, versions[j].Version) > 0 })
		fmt.Fprintln(a.Out, "Project:", o.args[0])
		for _, m := range versions {
			formats := map[string]bool{}
			for _, f := range m.Files {
				formats[strings.ToLower(filepath.Ext(f.Path))] = true
			}
			var exts []string
			for x := range formats {
				exts = append(exts, x)
			}
			sort.Strings(exts)
			fmt.Fprintf(a.Out, "\nVersion: %s (%s)\nDescription: %s\nFormats: %s\nPackage: %d bytes; %d files\nPublication: %s\n", m.Version, m.Status, cleanText(m.Description), strings.Join(exts, ", "), m.ArchiveSize, len(m.Files), m.PublishedAt.Format("2006-01-02T15:04:05Z07:00"))
			if len(m.Metadata) > 0 {
				b, _ := json.MarshalIndent(m.Metadata, "", "  ")
				fmt.Fprintln(a.Out, "Metadata:", string(b))
			}
		}
		return nil
	case "get":
		return a.get(ctx, store, o)
	}
	return fmt.Errorf("unknown command")
}
func cleanText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, s)
}
func (a *App) publish(ctx context.Context, o options) error {
	var metadata map[string]string
	if o.metadata != "" {
		f, e := os.Open(o.metadata)
		if e != nil {
			return e
		}
		b, e := io.ReadAll(io.LimitReader(f, model.MaxManifest+1))
		f.Close()
		if e != nil {
			return e
		}
		if int64(len(b)) > model.MaxManifest {
			return fmt.Errorf("metadata too large")
		}
		if e = json.Unmarshal(b, &metadata); e != nil {
			return fmt.Errorf("metadata must be a JSON object of string values")
		}
	}
	m, name, e := archive.Prepare(ctx, o.args[1], o.args[0], o.version, o.status, o.description, metadata)
	if e != nil {
		return e
	}
	defer os.Remove(name)
	store, e := a.Store(true)
	if e != nil {
		return e
	}
	fmt.Fprintf(a.Out, "Publishing %s %s: %d files, %d bytes, status %s\n", m.Project, m.Version, len(m.Files), m.ArchiveSize, m.Status)
	if e = store.Publish(ctx, m, name); e != nil {
		return e
	}
	fmt.Fprintf(a.Out, "Published %s %s (%s).\n", m.Project, m.Version, m.Status)
	return nil
}
func (a *App) get(ctx context.Context, store storage.Store, o options) error {
	m, e := store.Get(ctx, o.args[0], o.version)
	if e != nil {
		return e
	}
	dest := o.output
	if dest == "" {
		desktop, e := local.Desktop()
		if e != nil {
			return e
		}
		dest = filepath.Join(desktop, "Nodi", m.Project, m.Version)
	}
	dest, e = filepath.Abs(dest)
	if e != nil {
		return e
	}
	if _, e = os.Lstat(dest); !os.IsNotExist(e) {
		return fmt.Errorf("destination already exists or is inaccessible; choose another --output: %s", dest)
	}
	tmp, e := os.CreateTemp("", "nodi-download-*.zip")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	e = store.Download(ctx, m, tmp)
	closeErr := tmp.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if e = archive.Extract(ctx, name, m, dest); e != nil {
		return e
	}
	c, e := config.Load()
	if e != nil {
		return fmt.Errorf("download verified at %s, but local record failed: %w", dest, e)
	}
	if e = local.Record(c.Repository, m, dest); e != nil {
		return fmt.Errorf("download verified at %s, but local record failed: %w", dest, e)
	}
	fmt.Fprintf(a.Out, "Verified %d files (SHA-256). Saved: %s\n", len(m.Files), dest)
	return nil
}
