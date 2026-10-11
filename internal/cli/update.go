package cli

import (
	"context"
	"fmt"
	"nodi/internal/model"
	"strings"
)

func (a *App) updateCheck(ctx context.Context) error {
	fmt.Fprintln(a.Out, "Installed:", cleanText(a.Version))
	latest, err := a.LatestStable(ctx)
	if err != nil {
		fmt.Fprintln(a.Out, "Latest stable: NOT VERIFIED")
		return err
	}
	fmt.Fprintln(a.Out, "Latest stable:", latest)
	installed := strings.TrimPrefix(a.Version, "v")
	if model.ValidateVersion(installed) != nil {
		fmt.Fprintln(a.Out, "Development build; version comparison unavailable.")
		return nil
	}
	if model.CompareVersion(latest, installed) > 0 {
		fmt.Fprintln(a.Out, "Update available. Windows: download the installer from https://github.com/MaximSavhenko/nodi/releases/latest ; macOS: brew update && brew upgrade MaximSavhenko/nodi/nodi")
	} else {
		fmt.Fprintln(a.Out, "No newer stable release available.")
	}
	return nil
}
