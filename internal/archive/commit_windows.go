//go:build windows

package archive

import "syscall"

func commitDirectory(stage, dest string) error {
	a, e := syscall.UTF16PtrFromString(stage)
	if e != nil {
		return e
	}
	b, e := syscall.UTF16PtrFromString(dest)
	if e != nil {
		return e
	}
	// MoveFile has no replace or cross-volume copy flag. Both paths are siblings.
	return syscall.MoveFile(a, b)
}
