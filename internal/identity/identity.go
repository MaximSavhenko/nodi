// Package identity compares filesystem objects, never content or folded paths.
package identity

import (
	"fmt"
	"os"
	"path/filepath"
)

func Same(a, b string) bool {
	x, e := os.Stat(a)
	if e != nil {
		return false
	}
	y, e := os.Stat(b)
	return e == nil && os.SameFile(x, y)
}
func Canonical(path string) (string, error) {
	p, e := filepath.Abs(path)
	if e != nil {
		return "", e
	}
	return filepath.Clean(p), nil
}
func Key(path string) (string, error) {
	p, e := Canonical(path)
	if e != nil {
		return "", e
	}
	st, e := os.Lstat(p)
	if e != nil {
		return "", e
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("symlink identity refused")
	}
	return objectKey(p, st)
}
