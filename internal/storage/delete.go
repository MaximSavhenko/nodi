package storage

import (
	"context"
	"fmt"
	"net/url"
	"nodi/internal/model"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Deletion is a verified snapshot, not a name mask. Provider details stay here.
type Deletion struct {
	Manifest          model.Manifest
	ReleaseID         int64
	Tag, TagSHA       string
	Body              string
	Assets            []asset
	Draft, Prerelease bool
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
	if e == nil && (r.Ref != "refs/tags/"+tag || !gitSHA.MatchString(r.Object.SHA)) {
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
		if !ok || p != project || !gitSHA.MatchString(r.Object.SHA) {
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
	root, e := g.deletionRoot(project)
	if e != nil {
		return plan, e
	}
	if _, e = os.Lstat(root); e == nil {
		j, e := g.pendingDeletion(root)
		if e != nil {
			return plan, e
		}
		if j != nil {
			if j.Operation.Plan.Version != version || j.Operation.Plan.All != all {
				return plan, fmt.Errorf("unfinished deletion has a different exact selection")
			}
			return j.Operation.Plan, nil
		}
	} else if !os.IsNotExist(e) {
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
		plan.Items = append(plan.Items, Deletion{
			Manifest: m, ReleaseID: r.ID, Tag: r.Tag, TagSHA: ref.Object.SHA,
			Body: r.Body, Assets: append([]asset(nil), r.Assets...), Draft: r.Draft, Prerelease: r.Prerelease,
		})
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
		if x.Body != y.Body || x.ReleaseID != y.ReleaseID || x.Tag != y.Tag || x.TagSHA != y.TagSHA || x.Draft != y.Draft || x.Prerelease != y.Prerelease || !sameAssets(x.Assets, y.Assets) || !sameManifest(x.Manifest, y.Manifest) {
			return false
		}
	}
	return true
}

// Asset order is not identity. Compare full metadata without mutating either
// snapshot, including IDs even when replacement assets contain identical bytes.
func sameAssets(a, b []asset) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := append([]asset(nil), a...), append([]asset(nil), b...)
	sort.Slice(x, func(i, j int) bool { return x[i].ID < x[j].ID })
	sort.Slice(y, func(i, j int) bool { return y[i].ID < y[j].ID })
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func sameDeletionRelease(item Deletion, r release) bool {
	return r.Body == item.Body && r.ID == item.ReleaseID && r.Tag == item.Tag && r.Draft == item.Draft && r.Prerelease == item.Prerelease && sameAssets(item.Assets, r.Assets)
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
func (g *GitHub) verifyDeletionRelease(ctx context.Context, item Deletion) error {
	var current release
	if err := g.json(ctx, "GET", g.endpoint("/releases/"+strconv.FormatInt(item.ReleaseID, 10)), nil, &current); err != nil {
		return err
	}
	if !sameDeletionRelease(item, current) {
		return fmt.Errorf("release or assets changed after confirmation")
	}
	m, err := g.manifest(ctx, current)
	if err != nil {
		return err
	}
	if !sameManifest(m, item.Manifest) {
		return fmt.Errorf("manifest changed after confirmation")
	}
	if err = g.verifyAssets(ctx, current, item.Manifest); err != nil {
		return fmt.Errorf("assets failed final SHA-256 verification: %w", err)
	}
	if err = g.json(ctx, "GET", g.endpoint("/releases/"+strconv.FormatInt(item.ReleaseID, 10)), nil, &current); err != nil {
		return err
	}
	if !sameDeletionRelease(item, current) {
		return fmt.Errorf("release or assets changed during final verification")
	}
	return nil
}
func (g *GitHub) Delete(ctx context.Context, plan DeletionPlan) error {
	if err := g.CheckAccess(ctx, true); err != nil {
		return fmt.Errorf("deletion denied; run nodi setup --write: %w", err)
	}
	_, unlock, err := g.lockPublication(plan.Project)
	if err != nil {
		return err
	}
	defer unlock()
	root, err := g.deletionRoot(plan.Project)
	if err != nil {
		return err
	}
	if err = archiveCheckDeletionRoot(root); err != nil {
		return err
	}
	j, err := g.pendingDeletion(root)
	if err != nil {
		return err
	}
	if j == nil {
		fresh, err := g.PlanDelete(ctx, plan.Project, plan.Version, plan.All)
		if err != nil {
			return err
		}
		if !sameDeletionPlan(plan, fresh) {
			return fmt.Errorf("deletion selection changed; repeat dry-run and confirmation")
		}
		repo, err := g.publicationRepository(ctx)
		if err != nil {
			return err
		}
		j, err = g.newDeletion(root, plan, repo.ID)
		if err != nil {
			return err
		}
	} else if !sameDeletionPlan(plan, j.Operation.Plan) {
		return fmt.Errorf("deletion selection changed; retained operation requires its exact confirmation")
	}
	if err = g.continueDeletion(ctx, j); err != nil {
		item := j.Operation.Plan.Items[j.State.Item]
		return fmt.Errorf("deletion stopped at %s; %d earlier releases completed, %d later releases untouched; %s; repeat the same delete and confirmation for checked recovery: %w", item.Tag, j.State.Item, len(plan.Items)-j.State.Item-1, g.deletionState(ctx, item), err)
	}
	return nil
}
