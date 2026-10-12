package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"nodi/internal/archive"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type deletionOperation struct {
	Schema       int
	ID           string
	RepositoryID int64
	Plan         DeletionPlan
}
type deletionCheckpoint struct {
	Sequence, Item int
	Phase          string
}
type deletionJournal struct {
	dir       string
	Operation deletionOperation
	State     deletionCheckpoint
}

var deletionPhases = map[string]int{"prepared": 0, "release_intent": 1, "release_absent": 2, "tag_intent": 3, "tag_absent": 4, "complete": 5}

func (g *GitHub) deletionRoot(project string) (string, error) {
	root, err := g.publicationRoot(project)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(filepath.Dir(root)), "deletions", filepath.Base(root)), nil
}
func archiveCheckDeletionRoot(root string) error {
	if err := archive.CheckParents(root); err != nil {
		return err
	}
	return os.MkdirAll(root, 0700)
}
func (g *GitHub) pendingDeletion(root string) (*deletionJournal, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var pending *deletionJournal
	for _, entry := range entries {
		if !entry.IsDir() {
			return nil, fmt.Errorf("unknown deletion journal entry")
		}
		dir := filepath.Join(root, entry.Name())
		if err = archive.CheckParents(dir); err != nil {
			return nil, err
		}
		j := &deletionJournal{dir: dir}
		if err = readPublicationRecord(filepath.Join(dir, "operation.json"), &j.Operation); err != nil {
			return nil, err
		}
		op := j.Operation
		if op.Schema != 1 || len(op.ID) != 32 || entry.Name() != op.ID || op.RepositoryID <= 0 || len(op.Plan.Items) == 0 {
			return nil, fmt.Errorf("invalid deletion operation")
		}
		expected, err := g.deletionRoot(op.Plan.Project)
		if err != nil || expected != root {
			return nil, fmt.Errorf("deletion repository/project identity differs")
		}
		for _, item := range op.Plan.Items {
			if item.Manifest.Validate() != nil || item.Manifest.Project != op.Plan.Project || item.Tag != modelDeletionTag(item) || item.ReleaseID <= 0 || !gitSHA.MatchString(item.TagSHA) || len(item.Assets) != 2 {
				return nil, fmt.Errorf("invalid frozen deletion snapshot")
			}
		}
		steps, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, step := range steps {
			if step.Name() == "operation.json" || strings.HasPrefix(step.Name(), ".checkpoint-") && step.Type().IsRegular() {
				continue
			}
			var next deletionCheckpoint
			if !checkpointName.MatchString(step.Name()) {
				return nil, fmt.Errorf("unknown deletion checkpoint")
			}
			if err = readPublicationRecord(filepath.Join(dir, step.Name()), &next); err != nil {
				return nil, err
			}
			rank, known := deletionPhases[next.Phase]
			if !known || next.Sequence != j.State.Sequence+1 || next.Sequence > 512 || step.Name() != fmt.Sprintf("%06d.json", next.Sequence) || next.Item < 0 || next.Item >= len(op.Plan.Items) {
				return nil, fmt.Errorf("invalid deletion checkpoint sequence")
			}
			if j.State.Sequence == 0 {
				if next.Phase != "prepared" || next.Item != 0 {
					return nil, fmt.Errorf("invalid initial deletion checkpoint")
				}
			} else {
				if next.Item == j.State.Item {
					if rank < deletionPhases[j.State.Phase] {
						return nil, fmt.Errorf("deletion phase regressed")
					}
				} else if next.Item != j.State.Item+1 || j.State.Phase != "tag_absent" || next.Phase != "prepared" {
					return nil, fmt.Errorf("deletion item changed without completion")
				}
			}
			if next.Phase == "complete" && next.Item != len(op.Plan.Items)-1 {
				return nil, fmt.Errorf("premature deletion completion")
			}
			j.State = next
		}
		if j.State.Sequence == 0 {
			return nil, fmt.Errorf("deletion journal needs inspection")
		}
		if j.State.Phase != "complete" {
			if pending != nil {
				return nil, fmt.Errorf("multiple unfinished deletions")
			}
			pending = j
		}
	}
	return pending, nil
}
func modelDeletionTag(item Deletion) string {
	return item.Manifest.Project + "/v" + item.Manifest.Version
}
func (j *deletionJournal) checkpoint(phase string) error {
	if j.State.Sequence >= 512 {
		return fmt.Errorf("deletion checkpoint limit reached")
	}
	j.State.Sequence++
	j.State.Phase = phase
	return writePublicationRecord(filepath.Join(j.dir, fmt.Sprintf("%06d.json", j.State.Sequence)), j.State)
}
func (g *GitHub) newDeletion(root string, plan DeletionPlan, repoID int64) (*deletionJournal, error) {
	data, err := json.Marshal(deletionOperation{Schema: 1, Plan: plan})
	if err != nil {
		return nil, err
	}
	if len(plan.Items) > 80 || int64(len(data)) > 4*1024*1024 {
		return nil, fmt.Errorf("deletion batch exceeds durable journal limits; select fewer versions")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	j := &deletionJournal{Operation: deletionOperation{Schema: 1, ID: hex.EncodeToString(id[:]), RepositoryID: repoID, Plan: plan}}
	j.dir = filepath.Join(root, j.Operation.ID)
	if err := os.Mkdir(j.dir, 0700); err != nil {
		return nil, err
	}
	if err := writePublicationRecord(filepath.Join(j.dir, "operation.json"), j.Operation); err != nil {
		return nil, err
	}
	if err := j.checkpoint("prepared"); err != nil {
		return nil, err
	}
	return j, nil
}
func (g *GitHub) deletionAbsence(ctx context.Context, item Deletion) error {
	var r release
	err := g.json(ctx, "GET", g.endpoint("/releases/"+strconv.FormatInt(item.ReleaseID, 10)), nil, &r)
	if !IsStatus(err, 404) {
		return fmt.Errorf("original release absence NOT VERIFIED: %v", err)
	}
	if _, err = g.byTag(ctx, item.Tag); !IsStatus(err, 404) {
		return fmt.Errorf("release-by-tag absence NOT VERIFIED: %v", err)
	}
	rs, err := g.releases(ctx)
	if err != nil {
		return err
	}
	for _, r := range rs {
		if r.Tag == item.Tag || r.ID == item.ReleaseID {
			return fmt.Errorf("replacement release %d exists; tag retained", r.ID)
		}
	}
	return nil
}
func (g *GitHub) continueDeletion(ctx context.Context, j *deletionJournal) error {
	repo, err := g.publicationRepository(ctx)
	if err != nil {
		return err
	}
	if repo.ID != j.Operation.RepositoryID {
		return fmt.Errorf("deletion repository identity changed")
	}
	// Reserve all reviewed version numbers before any destructive request.
	if j.State.Sequence == 1 {
		for _, item := range j.Operation.Plan.Items {
			if err = g.preserveDeletedVersion(ctx, item); err != nil {
				return err
			}
		}
	}
	for i := j.State.Item; i < len(j.Operation.Plan.Items); i++ {
		item := j.Operation.Plan.Items[i]
		history, err := g.tagRef(ctx, deletedTag(item.Manifest.Project, item.Manifest.Version))
		if err != nil || history.Object.SHA != item.TagSHA {
			return fmt.Errorf("deletion history missing or changed; no destructive retry: %v", err)
		}
		if j.State.Phase == "prepared" || j.State.Phase == "release_intent" {
			var current release
			err = g.json(ctx, "GET", g.endpoint("/releases/"+strconv.FormatInt(item.ReleaseID, 10)), nil, &current)
			if err == nil {
				if err = g.verifyDeletionRelease(ctx, item); err != nil {
					return err
				}
				ref, err := g.tagRef(ctx, item.Tag)
				if err != nil || ref.Object.SHA != item.TagSHA {
					return fmt.Errorf("release or tag changed after confirmation: %v", err)
				}
				if j.State.Phase == "prepared" {
					if err = j.checkpoint("release_intent"); err != nil {
						return err
					}
				}
				if err = g.json(ctx, "DELETE", g.endpoint("/releases/"+strconv.FormatInt(item.ReleaseID, 10)), nil, nil); err != nil {
					return err
				}
			} else if !IsStatus(err, 404) || j.State.Phase != "release_intent" {
				return fmt.Errorf("confirmed release disappeared without deletion intent: %v", err)
			}
			if err = g.deletionAbsence(ctx, item); err != nil {
				return err
			}
			if err = j.checkpoint("release_absent"); err != nil {
				return err
			}
		}
		if j.State.Phase == "release_absent" || j.State.Phase == "tag_intent" {
			if err = g.deletionAbsence(ctx, item); err != nil {
				return err
			}
			ref, err := g.tagRef(ctx, item.Tag)
			if err == nil {
				if ref.Object.SHA != item.TagSHA {
					return fmt.Errorf("tag changed; retained")
				}
				if j.State.Phase == "release_absent" {
					if err = j.checkpoint("tag_intent"); err != nil {
						return err
					}
				}
				if err = g.json(ctx, "DELETE", g.endpoint("/git/refs/tags/"+url.PathEscape(item.Tag)), nil, nil); err != nil {
					return err
				}
				_, err = g.tagRef(ctx, item.Tag)
				if !IsStatus(err, 404) {
					return fmt.Errorf("tag absence NOT VERIFIED: %v", err)
				}
			} else if !IsStatus(err, 404) || j.State.Phase != "tag_intent" {
				return fmt.Errorf("tag absence without proven intent: %v", err)
			}
			if err = j.checkpoint("tag_absent"); err != nil {
				return err
			}
		}
		if i+1 < len(j.Operation.Plan.Items) {
			j.State.Item = i + 1
			if err = j.checkpoint("prepared"); err != nil {
				return err
			}
		}
	}
	return j.checkpoint("complete")
}
