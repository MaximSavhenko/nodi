package storage

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"nodi/internal/archive"
	"nodi/internal/config"
	"nodi/internal/identity"
	"nodi/internal/model"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type publicationOperation struct {
	Schema       int            `json:"schema"`
	ID           string         `json:"id"`
	Repository   string         `json:"repository"`
	Endpoint     string         `json:"endpoint"`
	RepositoryID int64          `json:"repository_id"`
	Source       string         `json:"source"`
	SourceObject string         `json:"source_object,omitempty"`
	Includes     []string       `json:"includes,omitempty"`
	Manifest     model.Manifest `json:"manifest"`
}

type publicationCheckpoint struct {
	Sequence    int     `json:"sequence"`
	Phase       string  `json:"phase"`
	TagSHA      string  `json:"tag_sha,omitempty"`
	ReleaseID   int64   `json:"release_id,omitempty"`
	Assets      []asset `json:"assets,omitempty"`
	IntentAsset string  `json:"intent_asset,omitempty"`
}

type publicationJournal struct {
	Operation publicationOperation
	State     publicationCheckpoint
	dir       string
}

var checkpointName = regexp.MustCompile(`^[0-9]{6}\.json$`)
var gitSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)
var publicationPhases = map[string]int{"prepared": 0, "reserve_intent": 1, "tag_confirmed": 2, "create_intent": 3, "draft_confirmed": 4, "uploading": 5, "assets_verified": 6, "finalize_intent": 7, "verified": 8, "complete": 9}

func (g *GitHub) publicationRoot(project string) (string, error) {
	if err := model.ValidateProject(project); err != nil {
		return "", err
	}
	dir := g.publicationDir
	if dir == "" {
		var err error
		dir, err = config.Dir()
		if err != nil {
			return "", err
		}
	}
	key := sha256.Sum256([]byte(strings.ToLower(g.repo) + "\x00" + g.base + "\x00" + project))
	return filepath.Join(dir, "publications", hex.EncodeToString(key[:])), nil
}

func (g *GitHub) lockPublication(project string) (string, func(), error) {
	root, err := g.publicationRoot(project)
	if err != nil {
		return "", nil, err
	}
	if err = archive.CheckParents(root); err != nil {
		return "", nil, err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return "", nil, err
	}
	if st, err := os.Lstat(filepath.Join(root, ".lock")); err == nil && !st.Mode().IsRegular() {
		return "", nil, fmt.Errorf("invalid publication lock file")
	} else if err != nil && !os.IsNotExist(err) {
		return "", nil, err
	}
	unlock, err := publicationLock(filepath.Join(root, ".lock"))
	return root, unlock, err
}

// Append-only, checksummed checkpoints avoid replacing the sole copy of the
// journal. Any partial/corrupt record stops recovery rather than rolling back
// knowledge of a remote mutation. These hashes are corruption checks, not auth.
func writePublicationRecord(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	envelope, err := json.Marshal(struct {
		Payload json.RawMessage `json:"payload"`
		SHA256  string          `json:"sha256"`
	}{data, hex.EncodeToString(digest[:])})
	if err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("publication record already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".checkpoint-")
	if err != nil {
		return err
	}
	temporary := f.Name()
	defer os.Remove(temporary)
	_, err = f.Write(envelope)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temporary, path)
	}
	if err == nil {
		err = syncPublicationDirectory(filepath.Dir(path))
	}
	return err
}

func readPublicationRecord(path string, value any) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Size() > model.MaxManifest+65536 {
		return fmt.Errorf("invalid publication journal file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var envelope struct {
		Payload json.RawMessage `json:"payload"`
		SHA256  string          `json:"sha256"`
	}
	if err = strictPublicationJSON(data, &envelope); err != nil {
		return fmt.Errorf("corrupt publication journal: %w", err)
	}
	digest := sha256.Sum256(envelope.Payload)
	if envelope.SHA256 != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("publication journal checksum mismatch")
	}
	return strictPublicationJSON(envelope.Payload, value)
}

func strictPublicationJSON(data []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing publication journal data")
	}
	return nil
}

func (g *GitHub) loadPublication(dir string) (*publicationJournal, error) {
	if err := archive.CheckParents(dir); err != nil {
		return nil, err
	}
	j := &publicationJournal{dir: dir}
	if err := readPublicationRecord(filepath.Join(dir, "operation.json"), &j.Operation); err != nil {
		return nil, err
	}
	op := j.Operation
	if op.Schema != 1 || len(op.ID) != 32 || !model.ValidHash(op.ID+op.ID) || op.Repository != strings.ToLower(g.repo) || op.Endpoint != g.base || op.RepositoryID <= 0 || filepath.Base(dir) != op.Manifest.Version {
		return nil, fmt.Errorf("invalid publication operation identity")
	}
	if err := op.Manifest.Validate(); err != nil {
		return nil, err
	}
	if op.Source != "" && (!filepath.IsAbs(op.Source) || filepath.Clean(op.Source) != op.Source) {
		return nil, fmt.Errorf("invalid publication source identity")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Name() == "operation.json" || entry.Name() == "archive.zip" {
			continue
		}
		// Uncommitted temporary records cannot precede a remote mutation: the
		// writer must finish rename/sync before continuing. Retain but ignore them.
		if strings.HasPrefix(entry.Name(), ".checkpoint-") && entry.Type().IsRegular() {
			continue
		}
		if entry.IsDir() || !checkpointName.MatchString(entry.Name()) {
			return nil, fmt.Errorf("unknown publication journal entry")
		}
		var next publicationCheckpoint
		if err = readPublicationRecord(filepath.Join(dir, entry.Name()), &next); err != nil {
			return nil, err
		}
		phase, known := publicationPhases[next.Phase]
		prevPhase := publicationPhases[j.State.Phase]
		if j.State.Sequence == 0 && (next.Phase != "prepared" || next.TagSHA != "" || next.ReleaseID != 0 || len(next.Assets) != 0 || next.IntentAsset != "") {
			return nil, fmt.Errorf("invalid initial publication checkpoint")
		}
		if !known || next.Sequence != j.State.Sequence+1 || entry.Name() != fmt.Sprintf("%06d.json", next.Sequence) || next.Sequence > 512 || (j.State.Sequence > 0 && phase < prevPhase) {
			return nil, fmt.Errorf("invalid publication checkpoint sequence or phase")
		}
		if phase >= 1 && !gitSHA.MatchString(next.TagSHA) || phase >= 4 && next.ReleaseID <= 0 || len(next.Assets) > 2 {
			return nil, fmt.Errorf("invalid publication checkpoint identity")
		}
		if j.State.TagSHA != "" && next.TagSHA != j.State.TagSHA || j.State.ReleaseID != 0 && next.ReleaseID != j.State.ReleaseID {
			return nil, fmt.Errorf("publication identity changed in journal")
		}
		for _, old := range j.State.Assets {
			found := false
			for _, a := range next.Assets {
				if a == old {
					found = true
				}
			}
			if !found {
				return nil, fmt.Errorf("publication asset identity changed in journal")
			}
		}
		if next.IntentAsset != "" && (next.Phase != "uploading" || next.IntentAsset != model.ArchiveName && next.IntentAsset != model.ManifestName) {
			return nil, fmt.Errorf("invalid publication upload intent")
		}
		j.State = next
	}
	if j.State.Sequence == 0 {
		return nil, fmt.Errorf("publication journal has no durable checkpoint")
	}
	return j, nil
}

func (g *GitHub) pendingPublication(root string) (*publicationJournal, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var pending *publicationJournal
	for _, entry := range entries {
		if entry.Name() == ".lock" {
			continue
		}
		if !entry.IsDir() || model.ValidateVersion(entry.Name()) != nil {
			return nil, fmt.Errorf("unknown publication operation entry")
		}
		j, err := g.loadPublication(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("publication journal %s needs inspection: %w", entry.Name(), err)
		}
		if j.State.Phase != "complete" {
			if pending != nil {
				return nil, fmt.Errorf("multiple unfinished publications; automatic recovery refused")
			}
			pending = j
		}
	}
	return pending, nil
}

func (j *publicationJournal) checkpoint(phase string) error {
	if j.State.Sequence >= 512 {
		return fmt.Errorf("publication journal checkpoint limit reached; inspect retained operation")
	}
	j.State.Sequence++
	j.State.Phase = phase
	return writePublicationRecord(filepath.Join(j.dir, fmt.Sprintf("%06d.json", j.State.Sequence)), j.State)
}

func (g *GitHub) newPublication(root, source, name string, m model.Manifest, repoID int64, includes ...[]string) (*publicationJournal, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	dir := filepath.Join(root, m.Version)
	if err := os.Mkdir(dir, 0700); err != nil {
		return nil, err
	}
	in, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	out, err := os.OpenFile(filepath.Join(dir, "archive.zip"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(in, m.ArchiveSize+1))
	if err == nil && (n != m.ArchiveSize || hex.EncodeToString(h.Sum(nil)) != m.ArchiveSHA256) {
		err = fmt.Errorf("archive changed while preserving publication")
	}
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	j := &publicationJournal{dir: dir, Operation: publicationOperation{Schema: 1, ID: hex.EncodeToString(id[:]), Repository: strings.ToLower(g.repo), Endpoint: g.base, RepositoryID: repoID, Source: source, Manifest: m}}
	if len(includes) != 0 {
		j.Operation.Includes = append([]string(nil), includes[0]...)
	}
	if source != "" {
		j.Operation.SourceObject, err = identity.Key(source)
		if err != nil {
			return nil, err
		}
	}
	if err = writePublicationRecord(filepath.Join(dir, "operation.json"), j.Operation); err != nil {
		return nil, err
	}
	if err = j.checkpoint("prepared"); err != nil {
		return nil, err
	}
	return j, nil
}

func (j *publicationJournal) archivePath() string { return filepath.Join(j.dir, "archive.zip") }
func (j *publicationJournal) marker() string      { return "Nodi operation: " + j.Operation.ID }
func (j *publicationJournal) releasePath() string {
	return "/releases/" + strconv.FormatInt(j.State.ReleaseID, 10)
}
