package storage

import (
	"context"
	"fmt"
	"net/url"
	"nodi/internal/model"
	"strings"
)

type ProjectState struct {
	Version  string
	Manifest *model.Manifest
}
type Version struct{ Project, Version string }

// Index is the lightweight read used by Tab: two API reads, no manifest downloads.
func (g *GitHub) Index(ctx context.Context, all bool) ([]Version, error) {
	if e := g.VerifyPrivate(ctx); e != nil {
		return nil, e
	}
	rs, e := g.releases(ctx)
	if e != nil {
		return nil, e
	}
	result := []Version{}
	for _, r := range rs {
		p, v, ok := model.ParseTag(r.Tag)
		if !ok || (!all && (r.Draft || r.Prerelease)) {
			continue
		}
		if _, e := findAsset(r, model.ManifestName); e != nil {
			continue
		}
		if _, e := findAsset(r, model.ArchiveName); e != nil {
			continue
		}
		result = append(result, Version{p, v})
	}
	return result, nil
}

type gitRef struct {
	Ref    string `json:"ref"`
	Object struct {
		SHA string `json:"sha"`
	} `json:"object"`
}

// State considers drafts and reserved tags, including interrupted publications.
// It never silently skips an unfinished highest version to allocate a new one.
func (g *GitHub) State(ctx context.Context, project string) (ProjectState, error) {
	var state ProjectState
	if e := model.ValidateProject(project); e != nil {
		return state, e
	}
	if e := g.VerifyPrivate(ctx); e != nil {
		return state, e
	}
	releases, e := g.releases(ctx)
	if e != nil {
		return state, e
	}
	var latest *release
	seen := map[string]bool{}
	for i, r := range releases {
		p, v, ok := model.ParseTag(r.Tag)
		if !ok || p != project {
			continue
		}
		if seen[v] {
			return state, fmt.Errorf("duplicate remote releases for version %s; inspect GitHub before publishing", v)
		}
		seen[v] = true
		if state.Version == "" || model.CompareVersion(v, state.Version) > 0 {
			state.Version = v
			latest = &releases[i]
		}
	}
	var refs []gitRef
	if e = g.json(ctx, "GET", g.endpoint("/git/matching-refs/tags/"+url.PathEscape(project+"/v")), nil, &refs); e != nil {
		return state, e
	}
	for _, r := range refs {
		p, v, ok := model.ParseTag(strings.TrimPrefix(r.Ref, "refs/tags/"))
		if !ok || p != project {
			continue
		}
		if state.Version == "" || model.CompareVersion(v, state.Version) > 0 {
			state.Version = v
			latest = nil
		}
	}
	if state.Version != "" {
		if latest == nil {
			return state, fmt.Errorf("latest version %s has a reserved tag without a complete release; inspect GitHub before retrying", state.Version)
		}
		m, e := g.manifest(ctx, *latest)
		if e != nil {
			return state, fmt.Errorf("latest version %s is incomplete or unavailable; inspect GitHub before retrying: %w", state.Version, e)
		}
		state.Manifest = &m
	}
	return state, nil
}
func (g *GitHub) reserveTag(ctx context.Context, tag string) error {
	var repo struct {
		DefaultBranch string `json:"default_branch"`
	}
	if e := g.json(ctx, "GET", g.endpoint(""), nil, &repo); e != nil {
		return e
	}
	if repo.DefaultBranch == "" {
		return fmt.Errorf("repository needs an initial commit before publication")
	}
	var head gitRef
	if e := g.json(ctx, "GET", g.endpoint("/git/ref/heads/"+url.PathEscape(repo.DefaultBranch)), nil, &head); e != nil {
		return e
	}
	if len(head.Object.SHA) != 40 {
		return fmt.Errorf("GitHub returned an invalid branch commit")
	}
	var reserved gitRef
	if e := g.json(ctx, "POST", g.endpoint("/git/refs"), map[string]string{"ref": "refs/tags/" + tag, "sha": head.Object.SHA}, &reserved); e != nil {
		if IsStatus(e, 422) || IsStatus(e, 409) {
			return fmt.Errorf("version conflict: another author reserved this version; retry the operation: %w", e)
		}
		return fmt.Errorf("tag reservation was not confirmed; inspect GitHub before retrying: %w", e)
	}
	if reserved.Ref != "refs/tags/"+tag || reserved.Object.SHA != head.Object.SHA {
		return fmt.Errorf("tag reservation response is inconsistent; inspect GitHub before retrying")
	}
	return nil
}
