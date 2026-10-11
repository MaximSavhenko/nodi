package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"nodi/internal/archive"
	"nodi/internal/config"
	"nodi/internal/model"
	"nodi/internal/project"
	"nodi/internal/storage"
	"os"
	"strings"
	"time"
)

func (a *App) binding() (project.Binding, error) {
	cwd, e := a.Cwd()
	if e != nil {
		return project.Binding{}, e
	}
	return project.Find(cwd)
}
func (a *App) initProject(o options) error {
	repo := o.repo
	if repo == "" {
		c, e := config.Load()
		if e != nil {
			return e
		}
		repo = c.Repository
	}
	cwd, e := a.Cwd()
	if e != nil {
		return e
	}
	b, created, e := project.Init(cwd, project.Config{Schema: 1, Project: o.args[0], Repository: repo})
	if e != nil {
		return e
	}
	verb := "Already initialized"
	if created {
		verb = "Initialized"
	}
	fmt.Fprintf(a.Out, "%s %s in %s\nStorage: %s\n", verb, b.Config.Project, b.Root, b.Config.Repository)
	return nil
}
func (a *App) projectStatus(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	b, e := a.binding()
	if e != nil {
		return e
	}
	files, e := project.Scan(ctx, b)
	if e != nil {
		return e
	}
	localVersion := b.Config.LastVersion
	if localVersion == "" {
		localVersion = "none"
	}
	fmt.Fprintf(a.Out, "Project: %s\nStorage: %s\n\nLocal version: %s\nFiles: %d\n", b.Config.Project, b.Config.Repository, localVersion, len(files))
	// Write credentials can see drafts; recipients may inspect released-only state.
	all := true
	s, e := a.StoreFor(b.Config.Repository, true)
	if e != nil {
		all = false
		s, e = a.StoreFor(b.Config.Repository, false)
	}
	if e != nil {
		fmt.Fprintln(a.Out, "Remote state: NOT VERIFIED (credentials unavailable). Changes are unknown.")
		return nil
	}
	var state storage.ProjectState
	if all {
		state, e = s.State(ctx, b.Config.Project)
	} else {
		var ms []model.Manifest
		ms, e = s.List(ctx, false)
		for i, m := range ms {
			if m.Project == b.Config.Project && (state.Version == "" || model.CompareVersion(m.Version, state.Version) > 0) {
				state.Version = m.Version
				state.Manifest = &ms[i]
			}
		}
	}
	if e != nil {
		fmt.Fprintf(a.Out, "Remote state: NOT VERIFIED (%s). Changes are unknown.\n", cleanText(e.Error()))
		return nil
	}
	remoteVersion := state.Version
	if remoteVersion == "" {
		remoteVersion = "none"
	}
	var old []model.File
	if state.Manifest != nil {
		old = state.Manifest.Files
	}
	changes := project.Diff(files, old)
	fmt.Fprintf(a.Out, "Remote version: %s\n", remoteVersion)
	if state.Manifest != nil {
		fmt.Fprintln(a.Out, "Remote status:", state.Manifest.Status)
	}
	if !all {
		fmt.Fprintln(a.Out, "Remote scope: released versions only; drafts NOT VERIFIED (write credential required).")
	}
	fmt.Fprintf(a.Out, "Modified: %d\nAdded: %d\nDeleted: %d\n\n", changes.Modified, changes.Added, changes.Deleted)
	if !all && changes.Empty() {
		fmt.Fprintln(a.Out, "Released baseline matches local files. Full publication state NOT VERIFIED (drafts require a write credential).")
	} else if changes.Empty() {
		fmt.Fprintln(a.Out, "No changes detected.\nNothing to publish.")
	} else if len(files) == 0 {
		fmt.Fprintln(a.Out, "No supported files remain. Empty packages cannot be published.")
	} else if all {
		fmt.Fprintln(a.Out, "Ready to publish.")
	} else {
		fmt.Fprintln(a.Out, "Changes detected relative to released versions. Full publication state requires a write credential.")
	}
	return nil
}
func readMetadata(path string) (map[string]string, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, model.MaxManifest+1))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) > model.MaxManifest {
		return nil, fmt.Errorf("metadata too large")
	}
	var m map[string]string
	if e = json.Unmarshal(b, &m); e != nil {
		return nil, fmt.Errorf("metadata must be a JSON object of string values")
	}
	return m, nil
}
func (a *App) autoPublish(ctx context.Context, o options) error {
	b, e := a.binding()
	if e != nil {
		return e
	}
	s, e := a.StoreFor(b.Config.Repository, true)
	if e != nil {
		return e
	}
	state, e := s.State(ctx, b.Config.Project)
	if e != nil {
		return fmt.Errorf("remote state could not be verified; nothing published: %w", e)
	}
	bump := "patch"
	if o.minor {
		bump = "minor"
	}
	if o.major {
		bump = "major"
	}
	next, e := project.NextVersion(state.Version, bump)
	if e != nil {
		return e
	}
	if o.version != "" {
		next = o.version
		if state.Version != "" && model.CompareVersion(next, state.Version) <= 0 {
			return fmt.Errorf("version conflict: requested version must be newer than remote %s; retry the operation", state.Version)
		}
	}
	desc := b.Config.Description
	if o.description != "" {
		desc = o.description
	}
	metadata := b.Config.Metadata
	if o.metadata != "" {
		metadata, e = readMetadata(o.metadata)
		if e != nil {
			return e
		}
	}
	files, e := project.Scan(ctx, b)
	if e != nil {
		return e
	}
	var old []model.File
	if state.Manifest != nil {
		old = state.Manifest.Files
	}
	changes := project.Diff(files, old)
	if changes.Empty() {
		fmt.Fprintln(a.Out, "No changes detected.\nNothing to publish.")
		return nil
	}
	if len(files) == 0 {
		return fmt.Errorf("no supported files remain; empty packages cannot be published")
	}
	m, zip, e := archive.Prepare(ctx, b.Root, b.Config.Project, next, o.status, desc, metadata)
	if e != nil {
		return e
	}
	defer os.Remove(zip)
	if !project.Diff(m.Files, files).Empty() {
		return fmt.Errorf("files changed during packaging; retry the operation")
	}
	current := state.Version
	if current == "" {
		current = "none"
	}
	fmt.Fprintf(a.Out, "Project: %s\nStorage: %s\nFiles: %d\n\nAdded: %d\nModified: %d\nDeleted: %d\n\nCurrent version: %s\nNext version: %s\n\nStatus: %s\n", b.Config.Project, b.Config.Repository, len(m.Files), changes.Added, changes.Modified, changes.Deleted, current, next, o.status)
	input := bufio.NewReader(a.In)
	fmt.Fprint(a.Out, "\nPublish? [y/N] ")
	answer, e := input.ReadString('\n')
	if e != nil && e != io.EOF {
		return e
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" {
		fmt.Fprintln(a.Out, "Cancelled. Nothing published.")
		return nil
	}
	if o.status == "released" {
		phrase := fmt.Sprintf("release %s %s", b.Config.Project, next)
		fmt.Fprintf(a.Out, "Released does not confirm engineering/manufacturing validation.\nConfirm production publication by typing %q: ", phrase)
		answer, e = input.ReadString('\n')
		if e != nil && e != io.EOF {
			return e
		}
		if strings.TrimSpace(answer) != phrase {
			fmt.Fprintln(a.Out, "Cancelled. Nothing published.")
			return nil
		}
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	latest, e := s.State(ctx, b.Config.Project)
	if e != nil {
		return fmt.Errorf("remote recheck failed; nothing published: %w", e)
	}
	if latest.Version != state.Version || (state.Manifest != nil && (latest.Manifest == nil || latest.Manifest.ArchiveSHA256 != state.Manifest.ArchiveSHA256 || latest.Manifest.Status != state.Manifest.Status)) {
		return fmt.Errorf("version conflict: remote state changed after preview; retry the operation")
	}
	after, e := project.Scan(ctx, b)
	if e != nil {
		return e
	}
	if !project.Diff(after, m.Files).Empty() {
		return fmt.Errorf("files changed after preview; retry the operation")
	}
	if e = s.Publish(ctx, m, zip); e != nil {
		return e
	}
	fmt.Fprintf(a.Out, "Published %s %s (%s).\n", m.Project, m.Version, m.Status)
	if e = b.SaveVersion(next); e != nil {
		return fmt.Errorf("remote publication succeeded, but local version record failed; inspect GitHub before retrying: %w", e)
	}
	return nil
}
