//go:build !darwin

package archive

func nativeSystemAlias(string) bool { return false }
