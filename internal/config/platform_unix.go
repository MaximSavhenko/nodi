//go:build !windows

package config

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// MVP: macOS credentials use a 0600 file in a private 0700 directory.
func protect(b []byte) ([]byte, error)   { return b, nil }
func unprotect(b []byte) ([]byte, error) { return b, nil }
func hiddenToken(f *os.File) (string, error) {
	get := exec.Command("stty", "-g")
	get.Stdin = f
	b, e := get.Output()
	if e != nil {
		return "", fmt.Errorf("cannot read terminal settings")
	}
	off := exec.Command("stty", "-echo")
	off.Stdin = f
	if e = off.Run(); e != nil {
		return "", fmt.Errorf("cannot disable token echo")
	}
	defer func() {
		restore := exec.Command("stty", strings.TrimSpace(string(b)))
		restore.Stdin = f
		_ = restore.Run()
	}()
	s, e := bufio.NewReader(f).ReadString('\n')
	if e != nil {
		return "", e
	}
	return validateToken(s)
}
