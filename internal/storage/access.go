package storage

import (
	"context"
	"fmt"
	"strings"
)

// CheckAccess verifies the APIs used for reading. A write check deliberately
// sends an invalid Git ref: GitHub requires refs/<namespace>/<name>, so this
// cannot create a tag, branch or release. 403 denies access; 422 confirms that
// the authorized write endpoint reached validation. Real Publish still checks
// every write and never treats this probe as a guarantee of future success.
func (g *GitHub) CheckAccess(ctx context.Context, write bool) error {
	if e := g.VerifyPrivate(ctx); e != nil {
		return e
	}
	if _, e := g.releases(ctx); e != nil {
		return e
	}
	var refs []gitRef
	if e := g.json(ctx, "GET", g.endpoint("/git/matching-refs/tags/nodi-access-check"), nil, &refs); e != nil {
		return e
	}
	if !write {
		return nil
	}
	e := g.json(ctx, "POST", g.endpoint("/git/refs"), map[string]string{"ref": "nodi-permission-check", "sha": strings.Repeat("0", 40)}, nil)
	if IsStatus(e, 422) {
		return nil
	}
	if e != nil {
		return e
	}
	return fmt.Errorf("GitHub unexpectedly accepted an invalid reference; write capability could not be verified")
}
