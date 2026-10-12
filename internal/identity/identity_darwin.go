//go:build darwin

package identity

import (
	"fmt"
	"os"
	"syscall"
)

func objectKey(_ string, info os.FileInfo) (string, error) {
	st := info.Sys().(*syscall.Stat_t)
	return fmt.Sprintf("darwin:%d:%d:%d:%d", st.Dev, st.Ino, st.Birthtimespec.Sec, st.Birthtimespec.Nsec), nil
}
