package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"nodi/internal/model"
	"strings"
	"time"
)

// The public CLI repository is independent of private package storage and
// credentials. This request never installs software or creates local files.
func LatestCLIRelease(ctx context.Context) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return latestCLIRelease(ctx, client, "https://api.github.com/repos/MaximSavhenko/nodi/releases/latest")
}
func latestCLIRelease(ctx context.Context, client *http.Client, endpoint string) (string, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if e != nil {
		return "", e
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "nodi-update-check")
	res, e := client.Do(req)
	if e != nil {
		return "", fmt.Errorf("GitHub unavailable; check connectivity and retry nodi update --check")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("GitHub update check failed (HTTP %d); retry nodi update --check", res.StatusCode)
	}
	var r struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	data, e := io.ReadAll(io.LimitReader(res.Body, 1<<20+1))
	if e != nil {
		return "", fmt.Errorf("cannot read GitHub update response")
	}
	if len(data) > 1<<20 || json.Unmarshal(data, &r) != nil {
		return "", fmt.Errorf("invalid GitHub update response")
	}
	version := strings.TrimPrefix(r.Tag, "v")
	if r.Draft || r.Prerelease || r.Tag != "v"+version || model.ValidateVersion(version) != nil {
		return "", fmt.Errorf("GitHub latest release is not a stable Nodi version")
	}
	return version, nil
}
