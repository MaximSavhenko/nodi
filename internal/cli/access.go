package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"nodi/internal/config"
	"nodi/internal/storage"
	"strings"
	"time"
)

type accessChecker interface {
	CheckAccess(context.Context, bool) error
}

func checkAccess(ctx context.Context, s storage.Store, write bool) error {
	v, ok := s.(accessChecker)
	if !ok {
		return fmt.Errorf("storage provider does not support access diagnostics")
	}
	return v.CheckAccess(ctx, write)
}

// Read repository input without read-ahead, preserving hidden terminal token
// input and the completion consent that may follow it on the same stdin.
func readSetupLine(in io.Reader) (string, error) {
	var result strings.Builder
	b := make([]byte, 1)
	for result.Len() < 256 {
		n, err := in.Read(b)
		if n > 0 {
			if b[0] == '\n' {
				return strings.TrimSpace(result.String()), nil
			}
			result.WriteByte(b[0])
		}
		if err != nil {
			if err == io.EOF {
				return strings.TrimSpace(result.String()), nil
			}
			return "", err
		}
	}
	return "", fmt.Errorf("setup input too long")
}

func (a *App) setup(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var check, write, completion bool
	var repo string
	fs.BoolVar(&check, "check", false, "read-only diagnostics")
	fs.BoolVar(&write, "write", false, "configure separate sending credential")
	fs.BoolVar(&completion, "completion", false, "only configure persistent completion")
	fs.StringVar(&repo, "repo", "", "private OWNER/REPO")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || completion && (repo != "" || write) {
		return fmt.Errorf("use nodi setup [--repo OWNER/REPO] [--write] [--check], or nodi setup --completion [--check]")
	}
	if completion {
		return a.setupCompletion(ctx, check)
	}
	return a.setupAccess(ctx, repo, write, check)
}

func (a *App) setupAccess(ctx context.Context, repo string, write, check bool) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	current, loadErr := config.Load()
	if repo == "" {
		if loadErr == nil {
			repo = current.Repository
		} else if check {
			return loadErr
		} else {
			fmt.Fprint(a.Out, "Private storage repository (OWNER/REPO): ")
			var err error
			repo, err = readSetupLine(a.In)
			if err != nil {
				return err
			}
		}
	}
	if err := config.ValidateRepo(repo); err != nil {
		return err
	}
	if loadErr == nil && current.Repository != repo {
		if check {
			return fmt.Errorf("storage differs from saved repository %s; --check does not change settings", current.Repository)
		}
		fmt.Fprintf(a.Out, "Change storage from %s to %s? [y/N] ", current.Repository, repo)
		answer, err := readSetupLine(a.In)
		if err != nil {
			return err
		}
		if answer != "y" && answer != "yes" {
			return fmt.Errorf("storage change cancelled; settings preserved")
		}
	}
	fmt.Fprintln(a.Out, "Storage:", repo)
	found := false
	for _, roleWrite := range []bool{false, true} {
		_, tokenErr := config.Token(roleWrite)
		if tokenErr != nil {
			continue
		}
		s, err := a.StoreFor(repo, roleWrite)
		if err != nil {
			return err
		}
		if err = checkAccess(ctx, s, roleWrite && !check); err != nil {
			return fmt.Errorf("%s credential failed; use nodi auth login%s to replace it: %w", accessRole(roleWrite), map[bool]string{true: " --write", false: ""}[roleWrite], err)
		}
		found = true
		fmt.Fprintf(a.Out, "%s credential: read APIs verified.\n", accessRole(roleWrite))
		if !roleWrite && !check {
			switch e := checkAccess(ctx, s, true); {
			case storage.IsStatus(e, 403):
				fmt.Fprintln(a.Out, "Read-only role: verified (write endpoint denied).")
			case e == nil:
				fmt.Fprintln(a.Out, "Read slot has write capability. Configure a Contents read token on receivers for a read-only role.")
			default:
				fmt.Fprintln(a.Out, "Read-only role: NOT VERIFIED; read APIs remain available.")
			}
		}
		if roleWrite {
			if check {
				fmt.Fprintln(a.Out, "Write capability: NOT VERIFIED (--check sends only GET requests).")
			} else {
				fmt.Fprintln(a.Out, "Write capability: verified by rejected invalid-ref probe; no remote object created.")
			}
		}
	}
	_, roleErr := config.Token(write)
	if !found || write && roleErr != nil {
		if check {
			return fmt.Errorf("no usable %s credential; run nodi setup%s", accessRole(write), map[bool]string{true: " --write", false: ""}[write])
		}
		fmt.Fprintf(a.Out, "GitHub %s token (hidden; never pass as an argument): ", accessRole(write))
		token, err := config.ReadToken(a.In)
		fmt.Fprintln(a.Out)
		if err != nil {
			return err
		}
		g, err := a.Connect(repo, token)
		if err != nil {
			return err
		}
		if err = checkAccess(ctx, g, write); err != nil {
			return err
		}
		if err = config.SaveToken(token, write); err != nil {
			return err
		}
		fmt.Fprintln(a.Out, "Saved separate", accessRole(write), "credential.")
	}
	if !check {
		if err := config.Save(config.Config{Repository: repo}); err != nil {
			return err
		}
	}
	fmt.Fprintln(a.Out, "Access setup complete. Use nodi send PATH with a write credential; nodi list/get with a read credential.")
	if _, e := config.Token(false); e != nil {
		fmt.Fprintln(a.Out, "Read credential is missing; configure receivers with nodi setup before list/get.")
	}
	fmt.Fprintln(a.Out, "Persistent Tab completion: nodi setup --completion (existing NODI-04 consent and backups).")
	return nil
}

func accessRole(write bool) string {
	if write {
		return "write"
	}
	return "read"
}
