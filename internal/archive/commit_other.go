//go:build !windows && !darwin

package archive

import "fmt"

func commitDirectory(stage, dest string) error {
	return fmt.Errorf("atomic no-replace directory installation unsupported on this platform")
}
