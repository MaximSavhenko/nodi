package storage

import (
	"fmt"
	"os"
	"testing"
)

// Provider tests now persist publication evidence. Always isolate it from the
// user's real configuration, including tests written before journals.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "nodi-storage-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err = os.Setenv("NODI_CONFIG_DIR", dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
