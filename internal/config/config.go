package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Config struct {
	Repository string `json:"repository"`
}

var repoRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}/[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)

func ValidateRepo(repo string) error {
	if !repoRE.MatchString(repo) {
		return fmt.Errorf("repository must be OWNER/REPO")
	}
	return nil
}
func Dir() (string, error) {
	if p := os.Getenv("NODI_CONFIG_DIR"); p != "" {
		return filepath.Abs(p)
	}
	p, e := os.UserConfigDir()
	return filepath.Join(p, "nodi"), e
}
func Load() (Config, error) {
	var c Config
	p, e := Dir()
	if e != nil {
		return c, e
	}
	b, e := os.ReadFile(filepath.Join(p, "config.json"))
	if os.IsNotExist(e) {
		e = nil
	} else if e == nil {
		e = json.Unmarshal(b, &c)
	}
	if e != nil {
		return c, e
	}
	if repo := os.Getenv("NODI_REPO"); repo != "" {
		c.Repository = repo
	}
	if e = ValidateRepo(c.Repository); e != nil {
		return c, fmt.Errorf("configure storage with nodi config --repo OWNER/REPO or NODI_REPO")
	}
	return c, nil
}
func Save(c Config) error {
	if e := ValidateRepo(c.Repository); e != nil {
		return e
	}
	p, e := Dir()
	if e != nil {
		return e
	}
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	return WritePrivate(filepath.Join(p, "config.json"), b)
}
func WritePrivate(name string, b []byte) error {
	if e := os.MkdirAll(filepath.Dir(name), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(name), ".nodi-config-")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e != nil {
		f.Close()
		return e
	}
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(tmp, name)
}
func Token(write bool) (string, error) {
	role := "read"
	if write {
		role = "write"
	}
	if t := os.Getenv("NODI_" + strings.ToUpper(role) + "_TOKEN"); t != "" {
		return validateToken(t)
	}
	p, e := Dir()
	if e != nil {
		return "", e
	}
	b, e := os.ReadFile(filepath.Join(p, role+".credential"))
	if e != nil {
		return "", fmt.Errorf("no %s token: run nodi auth login%s", role, map[bool]string{true: " --write", false: ""}[write])
	}
	b, e = unprotect(b)
	if e != nil {
		return "", fmt.Errorf("cannot unlock %s credential for this user", role)
	}
	return validateToken(string(b))
}
func validateToken(t string) (string, error) {
	t = strings.TrimSpace(t)
	if t == "" || len(t) > 4096 || strings.ContainsAny(t, "\r\n\x00 ") {
		return "", fmt.Errorf("invalid token input")
	}
	return t, nil
}
func SaveToken(t string, write bool) error {
	t, e := validateToken(t)
	if e != nil {
		return e
	}
	b, e := protect([]byte(t))
	if e != nil {
		return e
	}
	p, e := Dir()
	if e != nil {
		return e
	}
	role := "read"
	if write {
		role = "write"
	}
	return WritePrivate(filepath.Join(p, role+".credential"), b)
}
func Logout() error {
	p, e := Dir()
	if e != nil {
		return e
	}
	for _, role := range []string{"read", "write"} {
		e = os.Remove(filepath.Join(p, role+".credential"))
		if e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	return nil
}
func ReadToken(in io.Reader) (string, error) {
	if f, ok := in.(*os.File); ok {
		st, e := f.Stat()
		if e != nil {
			return "", e
		}
		if st.Mode()&os.ModeCharDevice != 0 {
			return hiddenToken(f)
		}
	}
	b, e := bufio.NewReader(io.LimitReader(in, 4097)).ReadString('\n')
	if e != nil && e != io.EOF {
		return "", e
	}
	return validateToken(b)
}
