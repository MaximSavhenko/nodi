package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"nodi/internal/archive"
	"nodi/internal/config"
	"nodi/internal/model"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type GitHub struct {
	repo, token, base, uploads string
	client                     *http.Client
}
type asset struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	State string `json:"state"`
}
type release struct {
	ID         int64   `json:"id"`
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []asset `json:"assets"`
}
type APIError struct{ Status int }

func (e *APIError) Error() string {
	switch e.Status {
	case 401:
		return "GitHub authentication failed (401): run nodi setup to replace the read token, or nodi setup --write for sending/deletion"
	case 403:
		return "GitHub access denied or rate limited (403): run nodi setup --check; verify token repository permissions or retry after the rate limit resets"
	case 404:
		return "GitHub resource unavailable (404): check repository, project and token access"
	case 422:
		return "GitHub rejected request (422): version/tag may already exist or repository may need an initial commit"
	case 429:
		return "GitHub rate limit exceeded (429): retry later"
	}
	return fmt.Sprintf("GitHub request failed (HTTP %d)", e.Status)
}
func IsStatus(err error, status int) bool {
	var e *APIError
	return errors.As(err, &e) && e.Status == status
}

func NewGitHub(repo, token string) (*GitHub, error) {
	return NewGitHubWithClient(repo, token, "https://api.github.com", "https://uploads.github.com", &http.Client{Timeout: 10 * time.Minute})
}

// Custom HTTPS endpoints/client are used by TLS mock tests; the CLI exposes no
// endpoint override and never accepts a token through its command arguments.
func NewGitHubWithClient(repo, token, base, uploads string, client *http.Client) (*GitHub, error) {
	if e := config.ValidateRepo(repo); e != nil {
		return nil, e
	}
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return nil, fmt.Errorf("GitHub token is required")
	}
	for _, s := range []string{base, uploads} {
		u, e := url.Parse(s)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, fmt.Errorf("GitHub endpoints must use HTTPS")
		}
	}
	c := *client
	if c.Timeout == 0 {
		c.Timeout = 10 * time.Minute
	}
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		if req.URL.Scheme != "https" {
			return fmt.Errorf("HTTPS downgrade refused")
		}
		if req.Method != "GET" {
			return fmt.Errorf("upload redirect refused")
		}
		req.Header.Del("Authorization")
		return nil
	}
	return &GitHub{repo: repo, token: token, base: strings.TrimRight(base, "/"), uploads: strings.TrimRight(uploads, "/"), client: &c}, nil
}
func (g *GitHub) endpoint(s string) string { return g.base + "/repos/" + g.repo + s }
func (g *GitHub) request(ctx context.Context, method, endpoint, contentType string, body io.Reader, length int64) (*http.Response, error) {
	req, e := http.NewRequestWithContext(ctx, method, endpoint, body)
	if e != nil {
		return nil, fmt.Errorf("cannot create GitHub request")
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "nodi/0.1")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if length >= 0 {
		req.ContentLength = length
	}
	if contentType == "application/octet-stream" && method == "GET" {
		req.Header.Set("Accept", "application/octet-stream")
	}
	res, e := g.client.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("GitHub network or TLS request failed; restore connectivity, run nodi setup --check and retry the command")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		res.Body.Close()
		return nil, &APIError{res.StatusCode}
	}
	return res, nil
}
func (g *GitHub) json(ctx context.Context, method, endpoint string, input, output any) error {
	var b []byte
	var e error
	if input != nil {
		b, e = json.Marshal(input)
		if e != nil {
			return e
		}
	}
	res, e := g.request(ctx, method, endpoint, "application/json", bytes.NewReader(b), int64(len(b)))
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if output == nil {
		return nil
	}
	data, e := io.ReadAll(io.LimitReader(res.Body, 16<<20+1))
	if e != nil {
		return fmt.Errorf("cannot read GitHub response")
	}
	if len(data) > 16<<20 {
		return fmt.Errorf("GitHub response exceeds 16 MiB")
	}
	if e = json.Unmarshal(data, output); e != nil {
		return fmt.Errorf("invalid GitHub JSON response")
	}
	return nil
}
func (g *GitHub) VerifyPrivate(ctx context.Context) error {
	var repo struct {
		Private bool `json:"private"`
	}
	if e := g.json(ctx, "GET", g.endpoint(""), nil, &repo); e != nil {
		return e
	}
	if !repo.Private {
		return fmt.Errorf("Nodi storage must be a private repository")
	}
	return nil
}
func (g *GitHub) releases(ctx context.Context) ([]release, error) {
	var result []release
	for page := 1; page <= 1000; page++ {
		var batch []release
		e := g.json(ctx, "GET", g.endpoint("/releases?per_page=100&page="+strconv.Itoa(page)), nil, &batch)
		if e != nil {
			return nil, e
		}
		result = append(result, batch...)
		if len(batch) < 100 {
			return result, nil
		}
	}
	return nil, fmt.Errorf("too many release pages; indexing stopped")
}
func findAsset(r release, name string) (asset, error) {
	var found asset
	count := 0
	for _, a := range r.Assets {
		if a.Name == name {
			found = a
			count++
		}
	}
	if count != 1 || found.ID <= 0 || found.State != "uploaded" {
		return asset{}, fmt.Errorf("release %s needs one complete %s asset", r.Tag, name)
	}
	return found, nil
}
func (g *GitHub) manifest(ctx context.Context, r release) (model.Manifest, error) {
	m, e := g.manifestAssets(ctx, r)
	if e != nil {
		return m, e
	}
	if (m.Status == "draft") != r.Draft || (!r.Draft && r.Prerelease) {
		return model.Manifest{}, fmt.Errorf("GitHub and Nodi release statuses differ for %s (GitHub draft=%t, prerelease=%t; manifest status=%s); do not toggle GitHub flags alone; preserve the assets and publish a verified new version with a consistent manifest", r.Tag, r.Draft, r.Prerelease, m.Status)
	}
	return m, nil
}

// Used only while the new release is still staged as a draft. Public readers
// always use manifest, which enforces both GitHub flags and manifest status.
func (g *GitHub) manifestAssets(ctx context.Context, r release) (model.Manifest, error) {
	var empty model.Manifest
	a, e := findAsset(r, model.ManifestName)
	if e != nil {
		return empty, e
	}
	if a.Size <= 0 || a.Size > model.MaxManifest {
		return empty, fmt.Errorf("manifest asset exceeds limits")
	}
	res, e := g.request(ctx, "GET", g.endpoint("/releases/assets/"+strconv.FormatInt(a.ID, 10)), "application/octet-stream", nil, -1)
	if e != nil {
		return empty, e
	}
	defer res.Body.Close()
	m, e := model.Decode(res.Body)
	if e != nil {
		return empty, e
	}
	if model.Tag(m.Project, m.Version) != r.Tag {
		return empty, fmt.Errorf("tag and manifest identity differ")
	}
	zip, e := findAsset(r, model.ArchiveName)
	if e != nil {
		return empty, e
	}
	if zip.Size != m.ArchiveSize {
		return empty, fmt.Errorf("archive asset size differs from manifest")
	}
	return m, nil
}

func (g *GitHub) verifyAssets(ctx context.Context, r release, expected model.Manifest) error {
	actual, e := g.manifestAssets(ctx, r)
	if e != nil {
		return e
	}
	if !sameManifest(actual, expected) {
		return fmt.Errorf("uploaded manifest differs from the prepared package")
	}
	a, e := findAsset(r, model.ArchiveName)
	if e != nil {
		return e
	}
	res, e := g.request(ctx, "GET", g.endpoint("/releases/assets/"+strconv.FormatInt(a.ID, 10)), "application/octet-stream", nil, -1)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(res.Body, expected.ArchiveSize+1))
	if e != nil {
		return fmt.Errorf("uploaded archive verification interrupted")
	}
	if n != expected.ArchiveSize || hex.EncodeToString(h.Sum(nil)) != expected.ArchiveSHA256 {
		return fmt.Errorf("uploaded archive SHA-256 or size mismatch")
	}
	return nil
}
func (g *GitHub) List(ctx context.Context, all bool) ([]model.Manifest, error) {
	return g.list(ctx, "", all)
}

// Project reads validate all selected releases, without letting a broken
// unrelated package block info or latest get for this project.
func (g *GitHub) ProjectList(ctx context.Context, project string, all bool) ([]model.Manifest, error) {
	if e := model.ValidateProject(project); e != nil {
		return nil, e
	}
	return g.list(ctx, project, all)
}

func (g *GitHub) list(ctx context.Context, project string, all bool) ([]model.Manifest, error) {
	if e := g.VerifyPrivate(ctx); e != nil {
		return nil, e
	}
	rs, e := g.releases(ctx)
	if e != nil {
		return nil, e
	}
	result := []model.Manifest{}
	for _, r := range rs {
		if p, _, ok := model.ParseTag(r.Tag); !ok || project != "" && p != project {
			continue
		}
		if !all && (r.Draft || r.Prerelease) {
			continue
		}
		if r.Draft {
			if _, e := findAsset(r, model.ManifestName); e != nil {
				continue
			}
		}
		m, e := g.manifest(ctx, r)
		if e != nil {
			return nil, fmt.Errorf("release %s: %w", r.Tag, e)
		}
		result = append(result, m)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Project != result[j].Project {
			return result[i].Project < result[j].Project
		}
		return model.CompareVersion(result[i].Version, result[j].Version) > 0
	})
	return result, nil
}
func (g *GitHub) byTag(ctx context.Context, tag string) (release, error) {
	var r release
	e := g.json(ctx, "GET", g.endpoint("/releases/tags/"+url.PathEscape(tag)), nil, &r)
	return r, e
}
func (g *GitHub) Get(ctx context.Context, project, version string) (model.Manifest, error) {
	if e := model.ValidateProject(project); e != nil {
		return model.Manifest{}, e
	}
	if version != "" {
		if e := model.ValidateVersion(version); e != nil {
			return model.Manifest{}, e
		}
		if e := g.VerifyPrivate(ctx); e != nil {
			return model.Manifest{}, e
		}
		r, e := g.byTag(ctx, model.Tag(project, version))
		if e != nil {
			return model.Manifest{}, e
		}
		if r.Draft || r.Prerelease {
			return model.Manifest{}, fmt.Errorf("project version is not released")
		}
		return g.manifest(ctx, r)
	}
	ms, e := g.ProjectList(ctx, project, false)
	if e != nil {
		return model.Manifest{}, e
	}
	for _, m := range ms {
		if m.Project == project {
			return m, nil
		}
	}
	return model.Manifest{}, fmt.Errorf("released project %q is unavailable", project)
}
func (g *GitHub) Download(ctx context.Context, m model.Manifest, w io.Writer) error {
	if e := m.Validate(); e != nil {
		return e
	}
	if e := g.VerifyPrivate(ctx); e != nil {
		return e
	}
	r, e := g.byTag(ctx, model.Tag(m.Project, m.Version))
	if e != nil {
		return e
	}
	if r.Draft || r.Prerelease {
		return fmt.Errorf("project version is not released")
	}
	actual, e := g.manifest(ctx, r)
	if e != nil {
		return e
	}
	if !sameManifest(actual, m) {
		return fmt.Errorf("manifest changed during download")
	}
	a, e := findAsset(r, model.ArchiveName)
	if e != nil {
		return e
	}
	res, e := g.request(ctx, "GET", g.endpoint("/releases/assets/"+strconv.FormatInt(a.ID, 10)), "application/octet-stream", nil, -1)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	n, e := io.Copy(w, io.LimitReader(res.Body, m.ArchiveSize+1))
	if e != nil {
		return fmt.Errorf("archive download interrupted")
	}
	if n != m.ArchiveSize {
		return fmt.Errorf("download size differs from manifest")
	}
	return nil
}

func sameManifest(a, b model.Manifest) bool {
	x, ex := json.Marshal(a)
	y, ey := json.Marshal(b)
	return ex == nil && ey == nil && bytes.Equal(x, y)
}
func (g *GitHub) upload(ctx context.Context, r release, name, mime string, reader io.Reader, size int64) error {
	endpoint := g.uploads + "/repos/" + g.repo + "/releases/" + strconv.FormatInt(r.ID, 10) + "/assets?name=" + url.QueryEscape(name)
	res, e := g.request(ctx, "POST", endpoint, mime, reader, size)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	var a asset
	b, e := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if e != nil {
		return fmt.Errorf("cannot read upload result")
	}
	if e = json.Unmarshal(b, &a); e != nil {
		return fmt.Errorf("invalid upload result")
	}
	if a.State != "uploaded" || a.Size != size || a.Name != name {
		return fmt.Errorf("GitHub did not confirm complete asset upload")
	}
	return nil
}
func (g *GitHub) Publish(ctx context.Context, m model.Manifest, name string) error {
	if e := m.Validate(); e != nil {
		return e
	}
	if e := g.VerifyPrivate(ctx); e != nil {
		return e
	}
	used, e := g.tombstones(ctx, m.Project)
	if e != nil {
		return e
	}
	for version := range used {
		if model.CompareVersion(m.Version, version) <= 0 {
			return fmt.Errorf("version %s was deleted or predates deletion history %s; choose a newer version", m.Version, version)
		}
	}
	hash, size, e := archive.HashFile(name)
	if e != nil {
		return e
	}
	if hash != m.ArchiveSHA256 || size != m.ArchiveSize {
		return fmt.Errorf("package checksum or size changed before publication")
	}
	// Listing includes drafts for write tokens. The release-by-tag endpoint alone
	// is insufficient to detect unpublished drafts.
	rs, e := g.releases(ctx)
	if e != nil {
		return e
	}
	for _, r := range rs {
		if r.Tag == model.Tag(m.Project, m.Version) {
			return fmt.Errorf("version already exists (including draft); choose a new version")
		}
		if p, v, ok := model.ParseTag(r.Tag); ok && p == m.Project && model.CompareVersion(m.Version, v) <= 0 {
			return fmt.Errorf("version must be newer than existing %s; choose a newer --version or use nodi send PATH", v)
		}
	}
	var ref any
	e = g.json(ctx, "GET", g.endpoint("/git/ref/tags/"+url.PathEscape(model.Tag(m.Project, m.Version))), nil, &ref)
	if e == nil {
		return fmt.Errorf("version tag already exists; choose a new version")
	}
	if !IsStatus(e, 404) {
		return e
	}
	f, e := os.Open(name)
	if e != nil {
		return e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return e
	}
	if st.Size() != m.ArchiveSize {
		return fmt.Errorf("package size changed before publication")
	}
	input := map[string]any{"tag_name": model.Tag(m.Project, m.Version), "name": m.Project + " " + m.Version, "body": "Nodi schema 1. Status: " + m.Status + ".\n" + m.Description, "draft": true, "prerelease": m.Status == "draft", "make_latest": "false"}
	// Git refs are created atomically. Draft releases alone do not reserve a tag.
	if e = g.reserveTag(ctx, model.Tag(m.Project, m.Version)); e != nil {
		return e
	}
	var r release
	if e = g.json(ctx, "POST", g.endpoint("/releases"), input, &r); e != nil {
		return fmt.Errorf("release creation was not confirmed; reserved tag retained; inspect GitHub before retrying: %w", e)
	}
	if r.ID <= 0 {
		return fmt.Errorf("GitHub returned an invalid release")
	}
	incomplete := func(err error) error {
		return fmt.Errorf("upload incomplete; draft release %d retained for inspection; no version was released: %w", r.ID, err)
	}
	if e = g.upload(ctx, r, model.ArchiveName, "application/zip", f, m.ArchiveSize); e != nil {
		return incomplete(e)
	}
	b, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return incomplete(e)
	}
	if int64(len(b)) > model.MaxManifest {
		return incomplete(fmt.Errorf("manifest too large"))
	}
	if e = g.upload(ctx, r, model.ManifestName, "application/json", bytes.NewReader(b), int64(len(b))); e != nil {
		return incomplete(e)
	}
	var staged release
	if e = g.json(ctx, "GET", g.endpoint("/releases/"+strconv.FormatInt(r.ID, 10)), nil, &staged); e != nil {
		return incomplete(e)
	}
	if staged.ID != r.ID || !staged.Draft || staged.Tag != model.Tag(m.Project, m.Version) {
		return incomplete(fmt.Errorf("staged release identity or draft state changed; inspect GitHub"))
	}
	if e = g.verifyAssets(ctx, staged, m); e != nil {
		return incomplete(e)
	}
	if m.Status == "released" {
		var completed release
		if e = g.json(ctx, "PATCH", g.endpoint("/releases/"+strconv.FormatInt(r.ID, 10)), map[string]any{"draft": false, "prerelease": false, "make_latest": "false"}, &completed); e != nil {
			return fmt.Errorf("release finalization was not confirmed; inspect GitHub release %d before retrying: %w", r.ID, e)
		}
		if completed.Draft || completed.Prerelease {
			return fmt.Errorf("GitHub did not confirm released status")
		}
	}
	var final release
	if m.Status == "released" {
		final, e = g.byTag(ctx, model.Tag(m.Project, m.Version))
	} else {
		e = g.json(ctx, "GET", g.endpoint("/releases/"+strconv.FormatInt(r.ID, 10)), nil, &final)
	}
	if e == nil && final.ID != r.ID {
		e = fmt.Errorf("release identity changed")
	}
	if e == nil {
		_, e = g.manifest(ctx, final)
	}
	if e == nil {
		e = g.verifyAssets(ctx, final, m)
	}
	if e != nil {
		return fmt.Errorf("final package availability was not verified; release %d retained; inspect GitHub before retrying: %w", r.ID, e)
	}
	return nil
}
