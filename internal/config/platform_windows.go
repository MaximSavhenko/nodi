//go:build windows

package config

import (
	"bufio"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

type blob struct {
	Size uint32
	Data *byte
}

var crypt = syscall.NewLazyDLL("crypt32.dll")
var kernel = syscall.NewLazyDLL("kernel32.dll")

func cryptData(b []byte, decrypt bool) ([]byte, error) {
	in := blob{Size: uint32(len(b)), Data: &b[0]}
	var out blob
	proc := crypt.NewProc("CryptProtectData")
	if decrypt {
		proc = crypt.NewProc("CryptUnprotectData")
	}
	r, _, e := proc.Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return nil, fmt.Errorf("Windows credential protection failed: %w", e)
	}
	defer kernel.NewProc("LocalFree").Call(uintptr(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
func protect(b []byte) ([]byte, error) { return cryptData(b, false) }
func unprotect(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("empty credential")
	}
	return cryptData(b, true)
}
func hiddenToken(f *os.File) (string, error) {
	var mode uint32
	get := kernel.NewProc("GetConsoleMode")
	set := kernel.NewProc("SetConsoleMode")
	r, _, e := get.Call(f.Fd(), uintptr(unsafe.Pointer(&mode)))
	if r == 0 {
		return "", fmt.Errorf("cannot read console mode: %w", e)
	}
	r, _, e = set.Call(f.Fd(), uintptr(mode&^4))
	if r == 0 {
		return "", fmt.Errorf("cannot disable token echo: %w", e)
	}
	defer set.Call(f.Fd(), uintptr(mode))
	s, e := bufio.NewReader(f).ReadString('\n')
	if e != nil {
		return "", e
	}
	return validateToken(s)
}
