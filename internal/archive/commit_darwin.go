//go:build darwin

package archive

import (
	"syscall"
	"unsafe"
)

func commitDirectory(stage, dest string) error {
	a, e := syscall.BytePtrFromString(stage)
	if e != nil {
		return e
	}
	b, e := syscall.BytePtrFromString(dest)
	if e != nil {
		return e
	}
	// XNU renameatx_np = 488; AT_FDCWD=-2; RENAME_EXCL=4.
	// Unsupported volumes fail closed; never retry as plain rename/copy.
	cwd := int(-2)
	_, _, errno := syscall.Syscall6(488, uintptr(cwd), uintptr(unsafe.Pointer(a)), uintptr(cwd), uintptr(unsafe.Pointer(b)), 4, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
