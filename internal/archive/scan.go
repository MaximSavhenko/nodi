package archive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"nodi/internal/model"
	"os"
	"path/filepath"
	"strings"
)

// Scan is read-only: the same path/format rules as Prepare, without creating ZIPs.
func Scan(ctx context.Context, source string) ([]model.File, error) {
	source, names, e := sourceNames(ctx, source, false)
	if e != nil {
		return nil, e
	}
	result := []model.File{}
	seen := map[string]bool{}
	var total int64
	for _, name := range names {
		key := strings.ToLower(name)
		if seen[key] {
			return nil, fmt.Errorf("case-colliding source file: %s", name)
		}
		seen[key] = true
		path := filepath.Join(source, filepath.FromSlash(name))
		st, e := os.Lstat(path)
		if e != nil {
			return nil, e
		}
		if !st.Mode().IsRegular() {
			return nil, fmt.Errorf("not a regular file: %s", name)
		}
		f, e := os.Open(path)
		if e != nil {
			return nil, e
		}
		actual, e := f.Stat()
		if e != nil || !os.SameFile(st, actual) {
			f.Close()
			return nil, fmt.Errorf("source changed during scan: %s", name)
		}
		h := sha256.New()
		n, e := io.Copy(h, io.LimitReader(contextReader{ctx, f}, model.MaxExpanded-total+1))
		f.Close()
		if e != nil {
			return nil, e
		}
		total += n
		if total > model.MaxExpanded {
			return nil, fmt.Errorf("source exceeds 512 MiB")
		}
		result = append(result, model.File{Path: name, Size: n, SHA256: hex.EncodeToString(h.Sum(nil))})
	}
	return result, nil
}
