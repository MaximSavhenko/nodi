//go:build windows

package local

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"
)

type guid struct {
	A    uint32
	B, C uint16
	D    [8]byte
}

func Desktop() (string, error) {
	id := guid{0xB4BFCC3A, 0xDB2C, 0x424C, [8]byte{0xB0, 0x29, 0x7F, 0xE9, 0x9A, 0x87, 0xC6, 0x41}}
	var p *uint16
	r, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("SHGetKnownFolderPath").Call(uintptr(unsafe.Pointer(&id)), 0, 0, uintptr(unsafe.Pointer(&p)))
	if r != 0 {
		return "", fmt.Errorf("cannot resolve Windows Desktop; use --output")
	}
	defer syscall.NewLazyDLL("ole32.dll").NewProc("CoTaskMemFree").Call(uintptr(unsafe.Pointer(p)))
	var words []uint16
	for i := 0; i < 32768; i++ {
		v := *(*uint16)(unsafe.Add(unsafe.Pointer(p), uintptr(i)*2))
		if v == 0 {
			break
		}
		words = append(words, v)
	}
	return syscall.UTF16ToString(words), nil
}
func Open(path string) error {
	cmd := exec.Command("explorer.exe", path)
	if e := cmd.Start(); e != nil {
		return e
	}
	return cmd.Process.Release()
}
