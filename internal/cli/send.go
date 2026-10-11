package cli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"nodi/internal/config"
	"nodi/internal/model"
	"nodi/internal/project"
	"os"
	"path/filepath"
	"strings"
)

// send uses an exact local binding when present, never an ancestor's identity
// for a partial directory. Unbound sends do not write into the source folder.
func (a *App) send(ctx context.Context, path string) error {
	source, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	st, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("cannot send source: %w", err)
	}
	if !st.IsDir() && !st.Mode().IsRegular() {
		return fmt.Errorf("source must be a regular file or real directory")
	}
	root := source
	if !st.IsDir() {
		root = filepath.Dir(source)
	}
	var binding *project.Binding
	var repo, name string
	if _, err = os.Lstat(filepath.Join(root, ".nodi", "project.json")); err == nil {
		b, err := project.Read(root)
		if err != nil {
			return err
		}
		if !st.IsDir() {
			return fmt.Errorf("file belongs to bound project %s; send its whole directory to avoid replacing the package with one file", b.Config.Project)
		}
		binding, repo, name = &b, b.Config.Repository, b.Config.Project
	} else if !os.IsNotExist(err) {
		return err
	}
	if binding == nil {
		// A nested send could otherwise silently switch from the bound project.
		if b, err := project.Find(filepath.Dir(root)); err == nil {
			return fmt.Errorf("source is inside bound project %s; send its root %s or copy the intended files to an unbound folder", b.Config.Project, b.Root)
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
	}
	return a.publishSource(ctx, options{status: "released"}, source, name, repo, binding)
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
