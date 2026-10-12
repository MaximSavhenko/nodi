package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"nodi/internal/archive"
	"nodi/internal/identity"
	"nodi/internal/model"
	"os"
	"path/filepath"
)

// PublicationResumer is the narrow capability needed to retain a prepared
// upload across CLI restarts. Store and manifest schema 1 stay unchanged.
type PublicationResumer interface {
	PublishFrom(context.Context, model.Manifest, string, string) error
	ResumePublish(context.Context, string, string, string, string) (*model.Manifest, error)
	AcknowledgePublish(context.Context, string, string, string) error
}

type publicationRepository struct {
	ID            int64  `json:"id"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
}

func (g *GitHub) publicationRepository(ctx context.Context) (publicationRepository, error) {
	var r publicationRepository
	err := g.json(ctx, "GET", g.endpoint(""), nil, &r)
	if err == nil && (r.ID <= 0 || !r.Private) {
		err = fmt.Errorf("publication requires a verified private repository identity")
	}
	return r, err
}

func (g *GitHub) Publish(ctx context.Context, m model.Manifest, name string) error {
	if err := g.PublishFrom(ctx, m, name, ""); err != nil {
		return err
	}
	return g.AcknowledgePublish(ctx, m.Project, m.Version, "")
}

func (g *GitHub) PublishFrom(ctx context.Context, m model.Manifest, name, source string) error {
	if err := m.Validate(); err != nil {
		return err
	}
	manifestBytes, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if int64(len(manifestBytes)) > model.MaxManifest {
		return fmt.Errorf("manifest exceeds limits")
	}
	if source != "" {
		var err error
		source, err = filepath.Abs(source)
		if err != nil {
			return err
		}
	}
	root, unlock, err := g.lockPublication(m.Project)
	if err != nil {
		return err
	}
	defer unlock()
	j, err := g.pendingPublication(root)
	if err != nil {
		return err
	}
	if j != nil {
		if !publicationSourceMatches(j.Operation, source) || !sameManifest(j.Operation.Manifest, m) {
			return fmt.Errorf("another unfinished publication owns this project; prepared operation retained")
		}
	} else {
		if err = g.publicationPreflight(ctx, m, name); err != nil {
			return err
		}
		repo, err := g.publicationRepository(ctx)
		if err != nil {
			return err
		}
		j, err = g.newPublication(root, source, name, m, repo.ID)
		if err != nil {
			return fmt.Errorf("cannot durably prepare publication; no remote mutation: %w", err)
		}
	}
	return g.attemptPublication(ctx, j)
}

func (g *GitHub) ResumePublish(ctx context.Context, project, source, status, version string) (*model.Manifest, error) {
	root, err := g.publicationRoot(project)
	if err != nil {
		return nil, err
	}
	if _, err = os.Lstat(root); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	root, unlock, err := g.lockPublication(project)
	if err != nil {
		return nil, err
	}
	defer unlock()
	j, err := g.pendingPublication(root)
	if err != nil || j == nil {
		return nil, err
	}
	if source != "" {
		source, err = filepath.Abs(source)
		if err != nil {
			return nil, err
		}
	}
	m := j.Operation.Manifest
	if !publicationSourceMatches(j.Operation, source) || m.Project != project || status != m.Status || version != "" && version != m.Version {
		return nil, fmt.Errorf("retry does not match the unfinished publication identity or requested status/version")
	}
	if err = g.attemptPublication(ctx, j); err != nil {
		return nil, err
	}
	return &m, nil
}

func (g *GitHub) attemptPublication(ctx context.Context, j *publicationJournal) error {
	if err := g.continuePublication(ctx, j); err != nil {
		target := "tag reservation"
		if j.State.ReleaseID != 0 {
			target = fmt.Sprintf("release %d", j.State.ReleaseID)
			if publicationPhases[j.State.Phase] < publicationPhases["finalize_intent"] {
				target = "staging draft " + target
			}
		}
		return fmt.Errorf("publication not confirmed; operation %s, %s and prepared archive retained; repeat the same command for checked recovery: %w", j.Operation.ID, target, err)
	}
	return nil
}

func (g *GitHub) AcknowledgePublish(ctx context.Context, project, version, source string) error {
	if err := model.ValidateVersion(version); err != nil {
		return err
	}
	root, unlock, err := g.lockPublication(project)
	if err != nil {
		return err
	}
	defer unlock()
	j, err := g.loadPublication(filepath.Join(root, version))
	if err != nil {
		return err
	}
	if source != "" {
		source, err = filepath.Abs(source)
		if err != nil {
			return err
		}
	}
	if !publicationSourceMatches(j.Operation, source) || j.Operation.Manifest.Project != project {
		return fmt.Errorf("publication acknowledgement identity mismatch")
	}
	if j.State.Phase == "complete" {
		return nil
	}
	if j.State.Phase != "verified" {
		return fmt.Errorf("publication is not remotely verified")
	}
	if err = j.checkpoint("complete"); err != nil {
		return err
	}
	// Only this operation's verified spool is removed, never source/user data.
	if err = os.Remove(j.archivePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return syncPublicationDirectory(j.dir)
}

func publicationSourceMatches(op publicationOperation, source string) bool {
	if op.Source == "" || source == "" {
		return op.Source == source
	}
	if op.SourceObject == "" {
		return identity.Same(op.Source, source)
	}
	key, err := identity.Key(source)
	return err == nil && key == op.SourceObject
}

func (j *publicationJournal) releaseBody() string {
	return "Nodi schema 1. Status: " + j.Operation.Manifest.Status + ".\n" + j.Operation.Manifest.Description + "\n\n" + j.marker()
}

func (g *GitHub) verifyPublicationTag(ctx context.Context, j *publicationJournal) error {
	m := j.Operation.Manifest
	ref, err := g.tagRef(ctx, model.Tag(m.Project, m.Version))
	if err != nil {
		return err
	}
	if ref.Object.SHA != j.State.TagSHA {
		return fmt.Errorf("confirmed publication tag changed; recovery refused")
	}
	return nil
}

func (g *GitHub) findPublicationRelease(ctx context.Context, j *publicationJournal) (*release, error) {
	rs, err := g.releases(ctx)
	if err != nil {
		return nil, err
	}
	var found *release
	for i := range rs {
		r := &rs[i]
		if r.Tag != model.Tag(j.Operation.Manifest.Project, j.Operation.Manifest.Version) {
			continue
		}
		if found != nil || r.ID <= 0 || r.Body != j.releaseBody() || j.State.ReleaseID != 0 && r.ID != j.State.ReleaseID {
			return nil, fmt.Errorf("release identity conflicts with another writer; automatic recovery refused")
		}
		found = r
	}
	if found == nil && j.State.ReleaseID != 0 {
		return nil, fmt.Errorf("confirmed release disappeared; automatic recreation refused")
	}
	return found, nil
}

func (g *GitHub) publicationRelease(ctx context.Context, j *publicationJournal) (release, error) {
	var r release
	if err := g.json(ctx, "GET", g.endpoint(j.releasePath()), nil, &r); err != nil {
		return r, err
	}
	if r.ID != j.State.ReleaseID || r.Tag != model.Tag(j.Operation.Manifest.Project, j.Operation.Manifest.Version) || r.Body != j.releaseBody() {
		return r, fmt.Errorf("publication release identity changed")
	}
	if r.Prerelease != (j.Operation.Manifest.Status == "draft") || !r.Draft && (j.Operation.Manifest.Status != "released" || publicationPhases[j.State.Phase] < publicationPhases["assets_verified"]) {
		return r, fmt.Errorf("publication flags changed before verified finalization")
	}
	if len(r.Assets) > 2 {
		return r, fmt.Errorf("unknown assets added to publication; retained for inspection")
	}
	for _, a := range r.Assets {
		if a.Name != model.ArchiveName && a.Name != model.ManifestName {
			return r, fmt.Errorf("unknown publication asset")
		}
		if _, err := findAsset(r, a.Name); err != nil {
			return r, err
		}
		known := false
		for _, old := range j.State.Assets {
			if old == a {
				known = true
			}
		}
		if !known && j.State.IntentAsset != a.Name {
			return r, fmt.Errorf("unconfirmed or replaced publication asset; retained for inspection")
		}
	}
	for _, old := range j.State.Assets {
		found := false
		for _, a := range r.Assets {
			if old == a {
				found = true
			}
		}
		if !found {
			return r, fmt.Errorf("confirmed publication asset changed or disappeared")
		}
	}
	return r, nil
}

func (g *GitHub) verifyPublicationAsset(ctx context.Context, a asset, m model.Manifest) error {
	expectedSize, expectedHash := m.ArchiveSize, m.ArchiveSHA256
	if a.Name == model.ManifestName {
		data, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			return err
		}
		h := sha256.Sum256(data)
		expectedSize = int64(len(data))
		expectedHash = hex.EncodeToString(h[:])
	}
	if a.Size != expectedSize {
		return fmt.Errorf("publication asset size differs from prepared operation")
	}
	res, err := g.request(ctx, "GET", g.endpoint("/releases/assets/"+fmt.Sprint(a.ID)), "application/octet-stream", nil, -1)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(res.Body, expectedSize+1))
	if err != nil {
		return err
	}
	if n != expectedSize || hex.EncodeToString(h.Sum(nil)) != expectedHash {
		return fmt.Errorf("publication asset SHA-256 mismatch")
	}
	return nil
}

func (g *GitHub) verifyCompletedPublication(ctx context.Context, j *publicationJournal, r release) error {
	m := j.Operation.Manifest
	if (m.Status == "draft") != r.Draft || r.Prerelease != (m.Status == "draft") {
		return fmt.Errorf("publication status was not confirmed")
	}
	if len(r.Assets) != 2 || !sameAssets(j.State.Assets, r.Assets) {
		return fmt.Errorf("final publication asset snapshot differs")
	}
	if _, err := g.manifest(ctx, r); err != nil {
		return err
	}
	if err := g.verifyAssets(ctx, r, m); err != nil {
		return err
	}
	if m.Status == "released" {
		final, err := g.byTag(ctx, r.Tag)
		if err != nil {
			return err
		}
		if final.ID != r.ID || final.Body != j.releaseBody() || final.Draft || final.Prerelease || !sameAssets(final.Assets, r.Assets) {
			return fmt.Errorf("final release availability or identity changed")
		}
	}
	if j.State.Phase != "verified" {
		return j.checkpoint("verified")
	}
	return nil
}

func (g *GitHub) continuePublication(ctx context.Context, j *publicationJournal) error {
	m := j.Operation.Manifest
	repo, err := g.publicationRepository(ctx)
	if err != nil {
		return err
	}
	if repo.ID != j.Operation.RepositoryID {
		return fmt.Errorf("repository identity changed; publication retained")
	}
	if publicationPhases[j.State.Phase] >= publicationPhases["tag_confirmed"] {
		ref, err := g.tagRef(ctx, model.Tag(m.Project, m.Version))
		if err != nil {
			return err
		}
		if ref.Object.SHA != j.State.TagSHA {
			return fmt.Errorf("confirmed publication tag changed; recovery refused")
		}
	}
	// Once bytes and finalization were verified, only reconcile the exact remote
	// result. A deleted local spool or changed source cannot cause a new upload.
	if publicationPhases[j.State.Phase] >= publicationPhases["assets_verified"] {
		r, err := g.publicationRelease(ctx, j)
		if err != nil {
			return err
		}
		if !r.Draft || m.Status == "draft" {
			return g.verifyCompletedPublication(ctx, j, r)
		}
	}
	st, err := os.Lstat(j.archivePath())
	if err != nil {
		return fmt.Errorf("prepared archive missing; publication retained: %w", err)
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("prepared archive is not a regular file")
	}
	hash, size, err := archive.HashFile(j.archivePath())
	if err != nil {
		return err
	}
	if hash != m.ArchiveSHA256 || size != m.ArchiveSize {
		return fmt.Errorf("prepared archive checksum mismatch; publication retained")
	}
	if j.Operation.Source != "" {
		files, err := archive.Scan(ctx, j.Operation.Source)
		if err != nil {
			return err
		}
		if len(files) != len(m.Files) {
			return fmt.Errorf("source changed since interrupted publication; prepared operation retained")
		}
		for i := range files {
			if files[i] != m.Files[i] {
				return fmt.Errorf("source changed since interrupted publication; prepared operation retained")
			}
		}
	}
	if j.State.Phase == "prepared" {
		if err = g.publicationPreflight(ctx, m, j.archivePath()); err != nil {
			return err
		}
		if repo.DefaultBranch == "" {
			return fmt.Errorf("repository needs an initial commit before publication")
		}
		var head gitRef
		if err = g.json(ctx, "GET", g.endpoint("/git/ref/heads/"+url.PathEscape(repo.DefaultBranch)), nil, &head); err != nil {
			return err
		}
		if !gitSHA.MatchString(head.Object.SHA) {
			return fmt.Errorf("invalid repository branch commit")
		}
		j.State.TagSHA = head.Object.SHA
		if err = j.checkpoint("reserve_intent"); err != nil {
			return err
		}
	}
	if j.State.Phase == "reserve_intent" {
		if _, err = g.tagRef(ctx, model.Tag(m.Project, m.Version)); err == nil {
			return fmt.Errorf("tag reservation ownership is ambiguous; inspect the retained journal before retrying")
		}
		if !IsStatus(err, 404) {
			return err
		}
		var reserved gitRef
		if err = g.json(ctx, "POST", g.endpoint("/git/refs"), map[string]string{"ref": "refs/tags/" + model.Tag(m.Project, m.Version), "sha": j.State.TagSHA}, &reserved); err != nil {
			if IsStatus(err, 409) || IsStatus(err, 422) {
				return fmt.Errorf("version conflict: another author reserved this version; journal retained: %w", err)
			}
			return err
		}
		if reserved.Ref != "refs/tags/"+model.Tag(m.Project, m.Version) || reserved.Object.SHA != j.State.TagSHA {
			return fmt.Errorf("tag reservation response identity differs")
		}
		if err = j.checkpoint("tag_confirmed"); err != nil {
			return err
		}
	}
	ref, err := g.tagRef(ctx, model.Tag(m.Project, m.Version))
	if err != nil {
		return err
	}
	if ref.Object.SHA != j.State.TagSHA {
		return fmt.Errorf("confirmed publication tag changed; recovery refused")
	}
	if j.State.Phase == "tag_confirmed" {
		if err = j.checkpoint("create_intent"); err != nil {
			return err
		}
	}
	found, err := g.findPublicationRelease(ctx, j)
	if err != nil {
		return err
	}
	if found == nil {
		if j.State.Phase != "create_intent" {
			return fmt.Errorf("publication release missing")
		}
		var created release
		input := map[string]any{"tag_name": model.Tag(m.Project, m.Version), "name": m.Project + " " + m.Version, "body": j.releaseBody(), "draft": true, "prerelease": m.Status == "draft", "make_latest": "false"}
		if err = g.verifyPublicationTag(ctx, j); err != nil {
			return err
		}
		if err = g.json(ctx, "POST", g.endpoint("/releases"), input, &created); err != nil {
			return err
		}
		if created.ID <= 0 || created.Tag != model.Tag(m.Project, m.Version) || created.Body != j.releaseBody() {
			return fmt.Errorf("release creation identity was not confirmed")
		}
		j.State.ReleaseID = created.ID
		if err = j.checkpoint("draft_confirmed"); err != nil {
			return err
		}
	} else if j.State.ReleaseID == 0 {
		j.State.ReleaseID = found.ID
		if err = j.checkpoint("draft_confirmed"); err != nil {
			return err
		}
	}
	if _, err = g.findPublicationRelease(ctx, j); err != nil {
		return err
	}
	r, err := g.publicationRelease(ctx, j)
	if err != nil {
		return err
	}
	if !r.Draft || r.Prerelease != (m.Status == "draft") {
		return fmt.Errorf("staged publication flags changed")
	}
	for _, name := range []string{model.ArchiveName, model.ManifestName} {
		r, err = g.publicationRelease(ctx, j)
		if err != nil {
			return err
		}
		var current *asset
		for i := range r.Assets {
			if r.Assets[i].Name == name {
				current = &r.Assets[i]
			}
		}
		if current == nil {
			j.State.IntentAsset = name
			if err = j.checkpoint("uploading"); err != nil {
				return err
			}
			if err = g.verifyPublicationTag(ctx, j); err != nil {
				return err
			}
			if name == model.ArchiveName {
				f, err := os.Open(j.archivePath())
				if err != nil {
					return err
				}
				err = g.upload(ctx, r, name, "application/zip", f, m.ArchiveSize)
				f.Close()
				if err != nil {
					return err
				}
			} else {
				data, err := json.MarshalIndent(m, "", "  ")
				if err != nil {
					return err
				}
				if int64(len(data)) > model.MaxManifest {
					return fmt.Errorf("manifest exceeds limits")
				}
				if err = g.upload(ctx, r, name, "application/json", bytes.NewReader(data), int64(len(data))); err != nil {
					return err
				}
			}
			r, err = g.publicationRelease(ctx, j)
			if err != nil {
				return err
			}
			a, err := findAsset(r, name)
			if err != nil {
				return err
			}
			current = &a
		}
		if err = g.verifyPublicationAsset(ctx, *current, m); err != nil {
			return err
		}
		known := false
		for _, a := range j.State.Assets {
			if a == *current {
				known = true
			}
		}
		if !known {
			j.State.Assets = append(j.State.Assets, *current)
			j.State.IntentAsset = ""
			if err = j.checkpoint("uploading"); err != nil {
				return err
			}
		}
	}
	r, err = g.publicationRelease(ctx, j)
	if err != nil {
		return err
	}
	if err = g.verifyAssets(ctx, r, m); err != nil {
		return err
	}
	if publicationPhases[j.State.Phase] < publicationPhases["assets_verified"] {
		if err = j.checkpoint("assets_verified"); err != nil {
			return err
		}
	}
	if m.Status == "released" {
		if j.State.Phase != "finalize_intent" {
			if err = j.checkpoint("finalize_intent"); err != nil {
				return err
			}
		}
		r, err = g.publicationRelease(ctx, j)
		if err != nil {
			return err
		}
		if r.Draft {
			if err = g.verifyPublicationTag(ctx, j); err != nil {
				return err
			}
			if err = g.json(ctx, "PATCH", g.endpoint(j.releasePath()), map[string]any{"draft": false, "prerelease": false, "make_latest": "false"}, nil); err != nil {
				return err
			}
		}
	}
	r, err = g.publicationRelease(ctx, j)
	if err != nil {
		return err
	}
	return g.verifyCompletedPublication(ctx, j, r)
}
