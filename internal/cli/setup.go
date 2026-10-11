package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

const setupStart = "# >>> nodi completion >>>"
const setupEnd = "# <<< nodi completion <<<"

type completionSetup struct {
	name, profile, block string
	// Other startup files are inspected only, never edited.
	other            []string
	shell            string
	nativeConfigured bool
}

func (a *App) setup(ctx context.Context, args []string) error {
	check := len(args) == 1 && args[0] == "--check"
	if len(args) != 0 && !check {
		return fmt.Errorf("use nodi setup [--check]")
	}
	plans, err := completionSetups(ctx)
	if err != nil {
		return err
	}
	return a.configureCompletion(plans, check)
}

func (a *App) configureCompletion(plans []completionSetup, check bool) error {
	fmt.Fprintln(a.Out, "Completion configuration check (profiles are not executed).")
	input := bufio.NewReader(a.In)
	for _, plan := range plans {
		data, err := readProfile(plan.profile)
		if err != nil {
			return err
		}
		text, encoding, err := decodeProfile(data)
		if err != nil {
			return fmt.Errorf("%s: %w", plan.profile, err)
		}
		state, err := setupState(text, plan.block, plan.shell)
		if err != nil {
			return fmt.Errorf("%s: %w", plan.profile, err)
		}
		if state == "MISSING" && plan.nativeConfigured {
			state = "HOMEBREW CONFIG DETECTED (runtime NOT VERIFIED)"
		}
		for _, other := range plan.other {
			b, e := readProfile(other)
			if e != nil {
				return e
			}
			s, _, e := decodeProfile(b)
			if e != nil {
				return fmt.Errorf("%s: %w", other, e)
			}
			if state == "MISSING" && existingCompletion(s, plan.shell) {
				state = "USER CONFIG DETECTED (runtime NOT VERIFIED): " + other
			}
		}
		fmt.Fprintf(a.Out, "%s: %s\n  Profile: %s\n", plan.name, state, plan.profile)
		if state != "MISSING" || check {
			continue
		}
		fmt.Fprintf(a.Out, "Append this block (existing content and encoding are preserved):\n%s\n", plan.block)
		if len(data) > 0 {
			fmt.Fprintln(a.Out, "A byte-for-byte backup will be saved next to the profile.")
		}
		fmt.Fprint(a.Out, "Allow this profile change? [y/N]: ")
		answer, e := input.ReadString('\n')
		if e != nil && e != io.EOF {
			return e
		}
		if answer = strings.ToLower(strings.TrimSpace(answer)); answer != "y" && answer != "yes" {
			fmt.Fprintln(a.Out, "Skipped; no profile changes.")
			continue
		}
		backup, e := appendProfile(plan.profile, data, text, encoding, plan.block)
		if e != nil {
			return e
		}
		fmt.Fprintln(a.Out, "Configured:", plan.profile)
		if backup != "" {
			fmt.Fprintln(a.Out, "Backup:", backup)
		}
	}
	fmt.Fprintln(a.Out, "Open a new terminal to verify Tab: nodi get nod<Tab>; nodi get PROJECT --version <Tab>.")
	fmt.Fprintln(a.Out, "Project/version suggestions require configured credentials or an available completion cache.")
	return nil
}

func readProfile(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return b, err
}

// Keep PowerShell 5.1 UTF-16 profiles, BOMs, and the original bytes intact.
func decodeProfile(b []byte) (string, string, error) {
	if bytes.HasPrefix(b, []byte{0xff, 0xfe}) || bytes.HasPrefix(b, []byte{0xfe, 0xff}) {
		if len(b)%2 != 0 {
			return "", "", fmt.Errorf("invalid UTF-16 profile; no changes made")
		}
		var order binary.ByteOrder = binary.LittleEndian
		encoding := "utf16le"
		if b[0] == 0xfe {
			order, encoding = binary.BigEndian, "utf16be"
		}
		units := make([]uint16, (len(b)-2)/2)
		for i := range units {
			units[i] = order.Uint16(b[2+i*2:])
		}
		return string(utf16.Decode(units)), encoding, nil
	}
	if !utf8.Valid(b) || bytes.ContainsRune(b, 0) {
		return "", "", fmt.Errorf("unsupported profile encoding; convert to UTF-8 or UTF-16 first; no changes made")
	}
	return strings.TrimPrefix(string(b), "\ufeff"), "utf8", nil
}

func encodeAppend(s, encoding string) []byte {
	if encoding == "utf8" {
		return []byte(s)
	}
	var order binary.ByteOrder = binary.LittleEndian
	if encoding == "utf16be" {
		order = binary.BigEndian
	}
	units := utf16.Encode([]rune(s))
	b := make([]byte, len(units)*2)
	for i, u := range units {
		order.PutUint16(b[i*2:], u)
	}
	return b
}

func activeProfile(s string) string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func existingCompletion(s, shell string) bool {
	s = strings.ToLower(activeProfile(s))
	if shell == "powershell" {
		return strings.Contains(s, "nodi completion powershell") && (strings.Contains(s, "invoke-expression") || strings.Contains(s, "iex")) ||
			strings.Contains(s, "nodi.ps1") && strings.Contains(s, ". ")
	}
	return strings.Contains(s, "nodi completion zsh") && strings.Contains(s, "source") ||
		strings.Contains(s, "_nodi") && (strings.Contains(s, "source") || strings.Contains(s, "compdef"))
}

func setupState(s, block, shell string) (string, error) {
	if strings.Contains(s, setupStart) || strings.Contains(s, setupEnd) {
		if strings.Count(s, setupStart) != 1 || strings.Count(s, setupEnd) != 1 {
			return "", fmt.Errorf("duplicate or incomplete Nodi block; inspect it manually; no changes made")
		}
		start, end := strings.Index(s, setupStart), strings.Index(s, setupEnd)
		if end < start || strings.ReplaceAll(s[start:end+len(setupEnd)], "\r\n", "\n") != block {
			return "", fmt.Errorf("Nodi block was edited or belongs to another installation; inspect it manually; no changes made")
		}
		return "CONFIGURED (managed block; runtime NOT VERIFIED)", nil
	}
	if existingCompletion(s, shell) {
		return "USER CONFIG DETECTED (runtime NOT VERIFIED)", nil
	}
	return "MISSING", nil
}

func appendProfile(path string, before []byte, text, encoding, block string) (string, error) {
	// Refuse symlinks instead of changing a dotfile manager's target silently.
	st, err := os.Lstat(path)
	if err == nil && (!st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0) {
		return "", fmt.Errorf("profile is not a regular file: %s; configure its source manually", path)
	}
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	flags := os.O_RDWR | os.O_APPEND
	if st == nil {
		flags |= os.O_CREATE | os.O_EXCL
	}
	f, err := os.OpenFile(path, flags, 0600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	current, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(current, before) {
		return "", fmt.Errorf("profile changed during setup; rerun nodi setup")
	}
	backup := ""
	if st != nil {
		b, e := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".nodi-backup-*")
		if e != nil {
			return "", e
		}
		backup = b.Name()
		_, e = b.Write(before)
		closeErr := b.Close()
		if e != nil {
			return "", e
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	newline := "\n"
	if strings.Contains(text, "\r\n") || (text == "" && runtime.GOOS == "windows") {
		newline = "\r\n"
	}
	addition := strings.ReplaceAll("\n"+block+"\n", "\n", newline)
	encoded := encodeAppend(addition, encoding)
	if len(before) == 0 && encoding == "utf8" && runtime.GOOS == "windows" {
		// Windows PowerShell 5.1 needs a BOM to read non-ASCII installation paths.
		encoded = append([]byte{0xef, 0xbb, 0xbf}, encoded...)
	}
	if _, err = f.Write(encoded); err != nil {
		return backup, err
	}
	return backup, f.Sync()
}

func psQuote(s string) string {
	for _, r := range s {
		if r > 127 {
			// An existing BOM-less ASCII profile is read as ANSI by PowerShell 5.1.
			// Keep the appended block ASCII even when the installation path is Unicode.
			return "([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + base64.StdEncoding.EncodeToString([]byte(s)) + "')))"
		}
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

func powershellSetupBlock(executable string) string {
	return setupStart + `
if (-not (Get-Command nodi -CommandType Application -ErrorAction SilentlyContinue)) {
    if (Test-Path -LiteralPath ` + psQuote(executable) + `) {
        $env:PATH = ` + psQuote(filepath.Dir(executable)) + ` + [IO.Path]::PathSeparator + $env:PATH
    }
}
if (Get-Command nodi -CommandType Application -ErrorAction SilentlyContinue) {
    nodi completion powershell | Out-String | Invoke-Expression
}
` + setupEnd
}

func zshSetupBlock(executable, brewPrefix string) string {
	dir := path.Dir(executable)
	if brewPrefix != "" {
		dir = path.Join(brewPrefix, "bin")
	}
	block := setupStart + "\nif (( ! $+commands[nodi] )) && [[ -x " + shQuote(path.Join(dir, "nodi")) + " ]]; then\n  path=(" + shQuote(dir) + " $path)\nfi\n"
	if brewPrefix != "" {
		// Use Homebrew's installed definition, including with an already initialized framework.
		block += "if [[ -r " + shQuote(path.Join(brewPrefix, "share/zsh/site-functions/_nodi")) + " ]]; then\n  fpath=(" + shQuote(path.Join(brewPrefix, "share/zsh/site-functions")) + " $fpath)\n"
	} else {
		block += "if (( $+commands[nodi] )); then\n"
	}
	block += "  if (( ! $+functions[compdef] )); then\n    autoload -Uz compinit\n    compinit\n  fi\n  if (( $+functions[compdef] )); then\n"
	if brewPrefix != "" {
		block += "    autoload -Uz _nodi\n    compdef _nodi nodi\n"
	} else {
		block += "    source <(command nodi completion zsh)\n"
	}
	return block + "  fi\nfi\n" + setupEnd
}

func completionSetups(ctx context.Context) ([]completionSetup, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	switch runtime.GOOS {
	case "windows":
		return windowsCompletionSetups(ctx, executable)
	case "darwin":
		home, e := os.UserHomeDir()
		if e != nil {
			return nil, e
		}
		zdot := os.Getenv("ZDOTDIR")
		if zdot == "" {
			zdot = home
			envFile, e := readProfile(filepath.Join(home, ".zshenv"))
			if e != nil {
				return nil, e
			}
			if strings.Contains(activeProfile(string(envFile)), "ZDOTDIR") {
				return nil, fmt.Errorf(".zshenv configures ZDOTDIR but it is not exported; run setup from Zsh with its effective ZDOTDIR exported; no changes made")
			}
		}
		if !filepath.IsAbs(zdot) {
			return nil, fmt.Errorf("ZDOTDIR must be absolute; no changes made")
		}
		prefix := homebrewCompletionPrefix(ctx)
		plan := completionSetup{name: "Zsh", shell: "zsh", profile: filepath.Join(zdot, ".zshrc"), block: zshSetupBlock(executable, prefix)}
		if prefix != "" {
			plan.name += " (Homebrew site-functions)"
			// A standard existing Homebrew setup needs no Nodi-specific startup block.
			rc, e := readProfile(plan.profile)
			if e != nil {
				return nil, e
			}
			login, e := readProfile(filepath.Join(zdot, ".zprofile"))
			if e != nil {
				return nil, e
			}
			rcText := activeProfile(string(rc))
			paths := rcText + "\n" + activeProfile(string(login))
			if (strings.Contains(paths, "brew shellenv") || strings.Contains(rcText, filepath.Join(prefix, "share/zsh/site-functions"))) &&
				(strings.Contains(rcText, "compinit") || strings.Contains(rcText, "oh-my-zsh.sh")) && !strings.Contains(string(rc), setupStart) {
				// Still report this conservatively: arbitrary user profiles cannot be proven statically.
				plan.nativeConfigured = true
			}
		}
		return []completionSetup{plan}, nil
	default:
		return nil, fmt.Errorf("persistent setup supports Windows PowerShell 5.1/7 and macOS Zsh; use nodi completion for other shells")
	}
}

func homebrewCompletionPrefix(ctx context.Context) string {
	candidates := []string{}
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if b, e := exec.CommandContext(probe, "brew", "--prefix").Output(); e == nil {
		candidates = append(candidates, strings.TrimSpace(string(b)))
	}
	candidates = append(candidates, "/opt/homebrew", "/usr/local")
	for _, prefix := range candidates {
		if filepath.IsAbs(prefix) {
			if st, e := os.Stat(filepath.Join(prefix, "share/zsh/site-functions/_nodi")); e == nil && st.Mode().IsRegular() {
				return prefix
			}
		}
	}
	return ""
}

func windowsCompletionSetups(ctx context.Context, executable string) ([]completionSetup, error) {
	ps := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	probe, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// Resolve the Windows known Documents folder (including OneDrive/redirection).
	b, err := exec.CommandContext(probe, ps, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", `[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false); [Environment]::GetFolderPath('MyDocuments') | ConvertTo-Json -Compress`).Output()
	if err != nil {
		return nil, fmt.Errorf("cannot resolve PowerShell profiles: %w", err)
	}
	var documents string
	if err = json.Unmarshal(bytes.TrimSpace(b), &documents); err != nil || documents == "" {
		return nil, fmt.Errorf("cannot resolve Windows Documents folder")
	}
	var plans []completionSetup
	for _, item := range []struct{ name, dir string }{{"PowerShell 5.1", "WindowsPowerShell"}, {"PowerShell 7", "PowerShell"}} {
		dir := filepath.Join(documents, item.dir)
		plans = append(plans, completionSetup{name: item.name, shell: "powershell", profile: filepath.Join(dir, "profile.ps1"), other: []string{filepath.Join(dir, "Microsoft.PowerShell_profile.ps1")}, block: powershellSetupBlock(executable)})
	}
	return plans, nil
}
