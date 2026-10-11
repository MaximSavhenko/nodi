//go:build !windows

package local

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func Desktop() (string, error) {
	home, e := os.UserHomeDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(home, "Desktop"), nil
}
func Open(path string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("file manager opening is supported on Windows and macOS")
	}
	cmd := exec.Command("open", path)
	if e := cmd.Start(); e != nil {
		return e
	}
	return cmd.Process.Release()
}
