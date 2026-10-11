package cli

import (
	"context"
	"fmt"
	"nodi/internal/config"
	"nodi/internal/storage"
	"os"
	"path/filepath"
	"text/tabwriter"
)

func (a *App) deletePackage(ctx context.Context, o options) error {
	s, err := a.Store(true)
	if err != nil {
		return fmt.Errorf("deletion requires a separate write credential; run nodi setup --write: %w", err)
	}
	d, ok := s.(storage.Deleter)
	if !ok {
		return fmt.Errorf("storage provider does not support safe deletion")
	}
	plan, err := d.PlanDelete(ctx, o.args[0], o.version, o.all)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(a.Out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PROJECT\tVERSION\tSTATUS\tSIZE\tRELEASE ID\tTAG")
	for _, item := range plan.Items {
		m := item.Manifest
		fmt.Fprintf(w, "%s\t%s\t%s\t%d bytes\t%d\t%s\n", m.Project, m.Version, m.Status, m.ArchiveSize, item.ReleaseID, item.Tag)
	}
	if err = w.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, "Only these remote releases and their verified tags are selected. Local downloads are preserved. Version-history refs are retained permanently.")
	if o.dryRun {
		fmt.Fprintln(a.Out, "Write capability: NOT VERIFIED (dry-run uses only GET requests; checked before deletion).")
		fmt.Fprintln(a.Out, "Dry-run complete. No local or remote changes.")
		return nil
	}
	phrase := plan.Project
	if o.all {
		phrase = "DELETE ALL " + plan.Project
	}
	fmt.Fprintf(a.Out, "Type %q to delete %d release(s): ", phrase, len(plan.Items))
	answer, err := readSetupLine(a.In)
	if err != nil {
		return err
	}
	if answer != phrase {
		return fmt.Errorf("deletion cancelled: confirmation did not match; preview again with nodi delete %s %s --dry-run", plan.Project, map[bool]string{true: "--all", false: "--version " + plan.Version}[plan.All])
	}
	err = d.Delete(ctx, plan)
	// Clear both credential scopes even after partial failure. list/info always
	// read live state; completion re-fetches on the next Tab.
	invalidateCompletion()
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Deleted %d release(s) for %s. Release and tag absence verified via GitHub API.\n", len(plan.Items), plan.Project)
	return nil
}
func invalidateCompletion() {
	dir, e := config.Dir()
	if e != nil {
		return
	}
	files, _ := filepath.Glob(filepath.Join(dir, "completion", "*.json"))
	for _, file := range files {
		_ = os.Remove(file)
	}
}
