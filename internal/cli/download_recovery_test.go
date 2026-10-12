package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRegressionB04DownloadRecordGap(t *testing.T) {
	a, _, _, root := projectApp(t)
	ctx := context.Background()
	if err := a.Run(ctx, []string{"send", root}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "received")
	blocker := filepath.Join(os.Getenv("NODI_CONFIG_DIR"), "downloads")
	if err := os.WriteFile(blocker, []byte("fixture disk failure"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(ctx, []string{"get", "demo", "--version", "0.1.0", "--output", dest}); err == nil {
		t.Fatal("record failure not injected")
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(ctx, []string{"get", "demo", "--version", "0.1.0", "--output", dest}); err != nil {
		t.Fatal("committed download cannot reconcile:", err)
	}
}
