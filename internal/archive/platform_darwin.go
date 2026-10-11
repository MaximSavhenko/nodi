//go:build darwin

package archive

import "path/filepath"

func nativeSystemAlias(path string) bool {
	expected := map[string]string{"/var": "/private/var", "/tmp": "/private/tmp", "/etc": "/private/etc"}
	target, ok := expected[path]
	if !ok {
		return false
	}
	actual, err := filepath.EvalSymlinks(path)
	return err == nil && actual == target
}
