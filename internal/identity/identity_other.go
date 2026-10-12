//go:build !windows && !darwin

package identity

import (
	"fmt"
	"os"
)

func objectKey(_ string, _ os.FileInfo) (string, error) {
	return "", fmt.Errorf("persistent filesystem identity is unsupported on this platform")
}
