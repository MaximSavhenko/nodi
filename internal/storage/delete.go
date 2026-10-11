package storage

import (
	"context"
	"fmt"
	"net/url"
	"nodi/internal/model"
	"sort"
	"strconv"
	"strings"
)

// Deletion is a verified snapshot, not a name mask. Provider details stay here.
type Deletion struct {
	Manifest    model.Manifest
	ReleaseID   int64
	Tag, TagSHA string
}
type DeletionPlan struct {
	Project, Version string
	All              bool
	Items            []Deletion
}
type Deleter interface {
	PlanDelete(context.Context, string, string, bool) (DeletionPlan, error)
	Delete(context.Context, DeletionPlan) error
}

const deletedPrefix = "nodi-deleted/"

func deletedTag(project, version string) string { return deletedPrefix + model.Tag(project, version) }
func (g *GitHub) tagRef(ctx context.Context, tag string) (gitRef, error) {
	var r gitRef
	e := g.json(ctx, "GET", g.endpoint("/git/ref/tags/"+url.PathEscape(tag)), nil, &r)
	if e == nil && (r.Ref != "refs/tags/"+tag || len(r.Object.SHA) != 40) {
		e = fmt.Errorf("tag identity is inconsistent: %s", tag)
	}
	return r, e
}
func (g *GitHub) tombstones(ctx context.Context, project string) (map[string]string, error) {
	var refs []gitRef
	if e := g.json(ctx, "GET", g.endpoint("/git/matching-refs/tags/"+url.PathEscape(deletedPrefix+project+"/v")), nil, &refs); e != nil {
		return nil, e
	}
	used := map[string]string{}
	for _, r := range refs {
		p, v, ok := model.ParseTag(strings.TrimPrefix(r.Ref, "refs/tags/"+deletedPrefix))
		if !ok || p != project || len(r.Object.SHA) != 40 {
			return nil, fmt.Errorf("invalid deletion history ref %s; inspect GitHub before publishing", r.Ref)
		}
		used[v] = r.Object.SHA
	}
	return used, nil
}
func (g *GitHub) PlanDelete(ctx context.Context, project, version string, all bool) (DeletionPlan, error) {
	plan := DeletionPlan{Project: project, Version: version, All: all}
	if e := model.ValidateProject(project); e != nil {
		return plan, e
	}
	if all == (version != "") {
		return plan, fmt.Errorf("deletion requires exactly one version or explicit all")
	}
	if !all {
		if e := model.ValidateVersion(version); e != nil {
			return plan, e
		}
	}
	if e := g.VerifyPrivate(ctx); e != nil {
		return plan, e
	}
	rs, e := g.releases(ctx)
	if e != nil {
		return plan, e
	}
	seen := map[string]bool{}
	for _, r := range rs {
		p, v, ok := model.ParseTag(r.Tag)
		if !ok || p != project || (!all && v != version) {
			continue
		}
		if r.ID <= 0 || seen[v] {
			return plan, fmt.Errorf("invalid or duplicate release for %s; deletion refused", r.Tag)
		}
		seen[v] = true
		m, e := g.manifest(ctx, r)
		if e != nil {
			return plan, fmt.Errorf("release %d %s is unknown or damaged; deletion refused: %w", r.ID, r.Tag, e)
		}
		if len(r.Assets) != 2 {
			return plan, fmt.Errorf("release %s has unknown additional assets; deletion refused", r.Tag)
		}
		if e = g.verifyAssets(ctx, r, m); e != nil {
			return plan, fmt.Errorf("release %s failed SHA-256 verification; deletion refused: %w", r.Tag, e)
		}
		ref, e := g.tagRef(ctx, r.Tag)
		if e != nil {
			return plan, fmt.Errorf("release %s tag ownership could not be verified; deletion refused: %w", r.Tag, e)
		}
		plan.Items = append(plan.Items, Deletion{m, r.ID, r.Tag, ref.Object.SHA})
	}
	if len(plan.Items) == 0 {
		return plan, fmt.Errorf("no matching releases for %s (already deleted or unavailable); run nodi info %s --all; orphan tags require separate recovery", project, project)
	}
	sort.Slice(plan.Items, func(i, j int) bool {
		return model.CompareVersion(plan.Items[i].Manifest.Version, plan.Items[j].Manifest.Version) < 0
	})
	return plan, nil
}
func sameDeletionPlan(a, b DeletionPlan) bool {
	if a.Project != b.Project || a.Version != b.Version || a.All != b.All || len(a.Items) != len(b.Items) {
		return false
	}
	for i, x := range a.Items {
		y := b.Items[i]
		if x.ReleaseID != y.ReleaseID || x.Tag != y.Tag || x.TagSHA != y.TagSHA || !sameManifest(x.Manifest, y.Manifest) {
			return false
		}
	}
	return true
}
func (g *GitHub) preserveDeletedVersion(ctx context.Context, item Deletion) error {
	tag := deletedTag(item.Manifest.Project, item.Manifest.Version)
	ref, e := g.tagRef(ctx, tag)
	if e == nil {
		if ref.Object.SHA != item.TagSHA {
			return fmt.Errorf("deletion history differs; no release deleted")
		}
		return nil
	}
	if !IsStatus(e, 404) {
		return e
	}
	e = g.json(ctx, "POST", g.endpoint("/git/refs"), map[string]string{"ref": "refs/tags/" + tag, "sha": item.TagSHA}, nil)
	// A lost POST response can leave a ref; verify it rather than trusting status.
	ref, verify := g.tagRef(ctx, tag)
	if verify != nil || ref.Object.SHA != item.TagSHA {
		return fmt.Errorf("deletion history NOT VERIFIED; no release deleted (create: %v; verify: %v)", e, verify)
	}
	return nil
}
func (g *GitHub) deletionState(ctx context.Context, item Deletion) string {
	status := func(err error) string {
		if IsStatus(err, 404) {
			return "absent (API verified)"
		}
		if err != nil {
			return "NOT VERIFIED: " + err.Error()
		}
		return "present"
	}
	var r release
	re := g.json(ctx, "GET", g.endpoint("/releases/"+strconv.FormatInt(item.ReleaseID, 10)), nil, &r)
	ref, te := g.tagRef(ctx, item.Tag)
	_, he := g.tagRef(ctx, deletedTag(item.Manifest.Project, item.Manifest.Version))
	tagStatus := status(te)
	if te == nil {
		tagStatus += " (SHA " + ref.Object.SHA + ")"
	}
	return fmt.Sprintf("release %d: %s; tag %s: %s; version-history ref: %s", item.ReleaseID, status(re), item.Tag, tagStatus, status(he))
}
func (g *GitHub) Delete(ctx context.Context, plan DeletionPlan) error {
	// Even a token saved in the write slot must have real write permission.
	if e := g.CheckAccess(ctx, true); e != nil {
		return fmt.Errorf("deletion denied; run nodi setup --write: %w", e)
	}
	fresh, e := g.PlanDelete(ctx, plan.Project, plan.Version, plan.All)
	if e != nil {
		return e
	}
	if !sameDeletionPlan(plan, fresh) {
		return fmt.Errorf("deletion selection changed; repeat dry-run and confirmation")
	}
	// Preserve all selected numbers before removing anything. These refs are
	// append-only, share the old tag commit, and are never completion entries.
	for _, item := range plan.Items {
		if e = g.preserveDeletedVersion(ctx, item); e != nil {
			return e
		}
	}
	for i, item := range plan.Items {
		fail := func(err error) error {
			return fmt.Errorf("deletion stopped at %s; %d earlier releases completed, %d later releases untouched; %s; retry only after inspection: %w", item.Tag, i, len(plan.Items)-i-1, g.deletionState(ctx, item), err)
		}
		// GitHub's release-by-tag endpoint only returns published releases.
		// The approved ID also works for drafts and prevents identity changes.
		var current release
		e := g.json(ctx, "GET", g.endpoint("/releases/"+strconv.FormatInt(item.ReleaseID, 10)), nil, &current)
		if e != nil {
			return fail(e)
		}
		m, e := g.manifest(ctx, current)
		if e != nil {
			return fail(e)
		}
		ref, e := g.tagRef(ctx, item.Tag)
		if e != nil {
			return fail(e)
		}
		if current.ID != item.ReleaseID || current.Tag != item.Tag || !sameManifest(m, item.Manifest) || ref.Object.SHA != item.TagSHA {
			return fail(fmt.Errorf("release or tag changed after confirmation"))
		}
		e = g.json(ctx, "DELETE", g.endpoint("/releases/"+strconv.FormatInt(item.ReleaseID, 10)), nil, nil)
		var r release
		verify := g.json(ctx, "GET", g.endpoint("/releases/"+strconv.FormatInt(item.ReleaseID, 10)), nil, &r)
		if e != nil || !IsStatus(verify, 404) {
			return fail(fmt.Errorf("release deletion: %v; verification: %v", e, verify))
		}
		// A replacement Release must not have appeared under the same tag.
		if _, e = g.byTag(ctx, item.Tag); !IsStatus(e, 404) {
			return fail(fmt.Errorf("release-by-tag absence not verified: %v", e))
		}
		// A replacement draft is invisible to release-by-tag. Check the complete
		// authenticated release inventory before removing its shared tag.
		remaining, e := g.releases(ctx)
		if e != nil {
			return fail(fmt.Errorf("release inventory absence not verified: %w", e))
		}
		for _, r := range remaining {
			if r.Tag == item.Tag {
				return fail(fmt.Errorf("replacement release %d exists under %s; tag retained", r.ID, item.Tag))
			}
		}
		ref, e = g.tagRef(ctx, item.Tag)
		if e != nil || ref.Object.SHA != item.TagSHA {
			return fail(fmt.Errorf("tag changed or cannot be verified; retained: %v", e))
		}
		e = g.json(ctx, "DELETE", g.endpoint("/git/refs/tags/"+url.PathEscape(item.Tag)), nil, nil)
		_, verify = g.tagRef(ctx, item.Tag)
		if e != nil || !IsStatus(verify, 404) {
			return fail(fmt.Errorf("tag deletion: %v; verification: %v", e, verify))
		}
	}
	return nil
}
