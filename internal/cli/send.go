package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"nodi/internal/archive"
	"nodi/internal/config"
	"nodi/internal/model"
	"nodi/internal/project"
	"os"
	"path/filepath"
	"strings"
)

// send uses an exact local binding when present, never an ancestor's identity
// for a partial directory. Unbound sends do not write into the source folder.
func (a *App) send(ctx context.Context, path string, o options) error {
	o.status = "released"
	source, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	st, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("cannot send source: %w; verify the path, then run nodi send %q --dry-run", err, source)
	}
	if !st.IsDir() && !st.Mode().IsRegular() {
		return fmt.Errorf("source must be a regular file or real directory")
	}
	root := source
	if !st.IsDir() {
		root = filepath.Dir(source)
	}
	var binding *project.Binding
	var ancestor *project.Binding
	var repo, name string
	if _, err = os.Lstat(filepath.Join(root, ".nodi", "project.json")); err == nil {
		b, err := project.Read(root)
		if err != nil {
			return err
		}
		if !st.IsDir() {
			return fmt.Errorf("file belongs to bound project %s; send its whole directory: nodi send %q", b.Config.Project, b.Root)
		}
		binding, repo, name = &b, b.Config.Repository, b.Config.Project
		if o.name != "" && o.name != name {
			return fmt.Errorf("directory is bound to %s; --name cannot change its binding; use nodi send %q, or choose a separate delivery folder", name, source)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if binding == nil {
		// Directories get their own identity. Individual files inside a bound
		// project still cannot replace that project with a partial package.
		if b, err := project.Find(filepath.Dir(root)); err == nil {
			if !st.IsDir() {
				return fmt.Errorf("file is inside bound project %s; send the whole project: nodi send %q, or send its containing directory as an independent package", b.Config.Project, b.Root)
			}
			fmt.Fprintf(a.Out, "Independent nested directory; parent %s binding is preserved.\n", b.Config.Project)
			ancestor = &b
		} else if !errors.Is(err, project.ErrNotInitialized) {
			return fmt.Errorf("ancestor project binding could not be verified: %w", err)
		}
		c, err := config.Load()
		if err != nil {
			return err
		}
		repo = c.Repository
		base := filepath.Base(source)
		if !st.IsDir() {
			base = strings.TrimSuffix(base, filepath.Ext(base))
		}
		name = packageName(base)
		if o.name != "" {
			name = o.name
		}
		receipt, err := readSendReceipt(source, repo)
		if err != nil {
			return err
		}
		if receipt != nil {
			if o.name != "" && o.name != receipt.Name {
				return fmt.Errorf("source was sent as %s; changing --name would change its identity; choose a separate delivery folder", receipt.Name)
			}
			name = receipt.Name
			o.ownsName = !receipt.Pending
		}
		if ancestor != nil && ancestor.Config.Repository == repo && ancestor.Config.Project == name {
			return fmt.Errorf("nested directory name %s matches its parent project; use nodi send %q --name unique-project to send it independently, or nodi send %q for the whole parent", name, source, ancestor.Root)
		}
	}
	if o.dryRun {
		return a.previewSend(ctx, source, name, repo, binding, o)
	}
	return a.publishSource(ctx, o, source, name, repo, binding)
}

type sendReceipt struct {
	Source, Repository, Name string
	Pending                  bool `json:"pending,omitempty"`
}

func sendReceiptPath(source, repo string) (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(source+"\x00"+repo)))
	return filepath.Join(dir, "sent", key+".json"), nil
}
func readSendReceipt(source, repo string) (*sendReceipt, error) {
	path, err := sendReceiptPath(source, repo)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r sendReceipt
	if json.Unmarshal(data, &r) != nil || r.Source != source || r.Repository != repo || model.ValidateProject(r.Name) != nil {
		return nil, fmt.Errorf("invalid local send identity; inspect %s", path)
	}
	return &r, nil
}
func saveSendReceipt(source, repo, name string) error {
	return savePendingSendReceipt(source, repo, name, false)
}
func savePendingSendReceipt(source, repo, name string, pending bool) error {
	path, err := sendReceiptPath(source, repo)
	if err != nil {
		return err
	}
	data, err := json.Marshal(sendReceipt{Source: source, Repository: repo, Name: name, Pending: pending})
	if err != nil {
		return err
	}
	return config.WritePrivate(path, data)
}
func nameConflict(name, source string) error {
	return fmt.Errorf("name conflict: %s already exists and this source has no matching send identity; use nodi send %q --name unique-project; for an existing package you own, explicitly bind its whole directory with nodi init %s", name, source, name)
}
func (a *App) previewSend(ctx context.Context, source, name, repo string, binding *project.Binding, o options) error {
	files, err := archive.Scan(ctx, source)
	if err != nil {
		return err
	}
	excluded, err := archive.Exclusions(ctx, source)
	if err != nil {
		return err
	}
	var size int64
	for _, f := range files {
		size += f.Size
	}
	fmt.Fprintf(a.Out, "Dry-run: no files or remote objects will be changed.\nPackage: %s\nStorage: %s\nFiles: %d\nTotal size: %d bytes\nExcluded:\n", name, repo, len(files), size)
	if len(excluded) == 0 {
		fmt.Fprintln(a.Out, "  none")
	}
	for _, f := range excluded {
		fmt.Fprintf(a.Out, "  %s: %s\n", cleanText(f.Path), f.Reason)
	}
	fmt.Fprintln(a.Out, "Final contents:")
	for _, f := range files {
		fmt.Fprintf(a.Out, "  %s (%d bytes; SHA-256 %s)\n", cleanText(f.Path), f.Size, f.SHA256)
	}
	if len(files) == 0 {
		return fmt.Errorf("no transferable files remain; add ordinary files to %q and run nodi send %q --dry-run", source, source)
	}
	s, err := a.StoreFor(repo, true)
	if err != nil {
		fmt.Fprintf(a.Out, "Remote version: NOT VERIFIED (%s); run nodi setup --write, then repeat this dry-run.\n", cleanText(err.Error()))
		return nil
	}
	state, err := s.State(ctx, name)
	if err != nil {
		fmt.Fprintf(a.Out, "Remote version: NOT VERIFIED (%s); retry nodi send %q --dry-run after restoring GitHub access.\n", cleanText(err.Error()), source)
		return nil
	}
	if binding == nil && !o.ownsName && state.Version != "" {
		return nameConflict(name, source)
	}
	if state.Manifest != nil && state.Manifest.Status == "released" && project.Diff(files, state.Manifest.Files).Empty() {
		fmt.Fprintf(a.Out, "Version: %s (unchanged remaining release; highest used number: %s). No changes detected. Nothing to publish.\n", state.Manifest.Version, state.Version)
		return nil
	}
	next, err := project.NextVersion(state.Version, "patch")
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Version: %s (preview; rechecked at send time)\n", next)
	return nil
}

func packageName(base string) string {
	// Stable Latin names work unchanged on both platforms; transliterate common
	// Cyrillic names, and use a digest if no portable name remains.
	letters := map[rune]string{'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e", 'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "kh", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "shch", 'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya", 'і': "i", 'ї': "yi", 'є': "ye", 'ґ': "g"}
	var out strings.Builder
	for _, r := range strings.ToLower(base) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			out.WriteRune(r)
		} else if s, ok := letters[r]; ok {
			out.WriteString(s)
		} else {
			out.WriteByte('-')
		}
	}
	name := strings.Trim(out.String(), "-")
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	if name == "" {
		return fmt.Sprintf("files-%x", sha256.Sum256([]byte(base)))[:22]
	}
	if name[0] < 'a' || name[0] > 'z' || model.Reserved(name) {
		name = "files-" + name
	}
	if len(name) > 63 {
		name = strings.TrimRight(name[:46], "-") + fmt.Sprintf("-%x", sha256.Sum256([]byte(base)))[:17]
	}
	return name
}
