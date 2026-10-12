//go:build windows

package storage

import (
	"fmt"
	"syscall"
)

func publicationLock(path string) (func(), error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("publication lock unavailable; another publication may be active: %w", err)
	}
	return func() { _ = syscall.CloseHandle(h) }, nil
}

// Each record/archive is FlushFileBuffers'd by os.File.Sync. Windows has no
// portable directory fsync here; hardware power-loss durability is not claimed.
func syncPublicationDirectory(string) error { return nil }
