package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupConsentCheckAndRepeat(t *testing.T) {
	for _, encoding := range []string{"utf8", "utf16le", "utf16be"} {
		t.Run(encoding, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "profile.ps1")
			original := encodeAppend("# Настройки пользователя\r\n$global:UserSetting = 42", encoding)
			if encoding == "utf16le" {
				original = append([]byte{0xff, 0xfe}, original...)
			} else if encoding == "utf16be" {
				original = append([]byte{0xfe, 0xff}, original...)
			} else {
				original = append([]byte{0xef, 0xbb, 0xbf}, original...)
			}
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			plans := []completionSetup{{name: "PowerShell", profile: path, shell: "powershell", block: powershellSetupBlock(filepath.Join(root, "bin/nodi.exe"))}}
			var out bytes.Buffer
			a := New("test", strings.NewReader("yes\n"), &out)
			for _, check := range []bool{true, false, false} {
				if !check {
					a.In = strings.NewReader("no\n")
				}
				if err := a.configureCompletion(plans, check); err != nil {
					t.Fatal(err)
				}
				got, _ := os.ReadFile(path)
				if !bytes.Equal(got, original) {
					t.Fatal("check/refusal changed profile")
				}
			}
			a.In = strings.NewReader("y\n")
			if err := a.configureCompletion(plans, false); err != nil {
				t.Fatal(err)
			}
			configured, _ := os.ReadFile(path)
			if !bytes.HasPrefix(configured, original) {
				t.Fatal("original bytes were rewritten")
			}
			backups, _ := filepath.Glob(path + ".nodi-backup-*")
			if len(backups) != 1 {
				t.Fatal(backups)
			}
			backup, _ := os.ReadFile(backups[0])
			if !bytes.Equal(backup, original) {
				t.Fatal("backup differs")
			}
			a.In = strings.NewReader("yes\n")
			for _, check := range []bool{false, true} {
				if err := a.configureCompletion(plans, check); err != nil {
					t.Fatal(err)
				}
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(after, configured) {
				t.Fatal("repeat/check changed configured profile")
			}
			backups, _ = filepath.Glob(path + ".nodi-backup-*")
			if len(backups) != 1 {
				t.Fatal("repeat created backup", backups)
			}
		})
	}
}

func TestSetupDoesNotCreateOnCheckEOFOrRefusal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "profile.ps1")
	plans := []completionSetup{{name: "PS", profile: path, shell: "powershell", block: powershellSetupBlock("nodi.exe")}}
	for _, answer := range []string{"", "no\n", "anything\n"} {
		a := New("test", strings.NewReader(answer), &bytes.Buffer{})
		for _, check := range []bool{true, false} {
			if err := a.configureCompletion(plans, check); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
				t.Fatal("created directory without consent")
			}
		}
	}
}

func TestSetupExistingAndConflictingProfiles(t *testing.T) {
	block := powershellSetupBlock("nodi.exe")
	for _, s := range []string{setupStart, setupEnd, block + "\n" + block, strings.Replace(block, "Out-String", "Edited", 1)} {
		if _, err := setupState(s, block, "powershell"); err == nil {
			t.Fatal("accepted conflicting block", s)
		}
	}
	for _, s := range []string{"# nodi completion powershell | Out-String | Invoke-Expression", "Write-Host 'custom profile'"} {
		if state, err := setupState(s, block, "powershell"); err != nil || state != "MISSING" {
			t.Fatal(state, err)
		}
	}
	root := t.TempDir()
	profile := filepath.Join(root, "profile.ps1")
	host := filepath.Join(root, "Microsoft.PowerShell_profile.ps1")
	if err := os.WriteFile(host, []byte("nodi completion powershell | Out-String | Invoke-Expression\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a := New("test", strings.NewReader("yes\n"), &bytes.Buffer{})
	if err := a.configureCompletion([]completionSetup{{profile: profile, other: []string{host}, block: block, shell: "powershell"}}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatal("duplicated host-profile setup")
	}
	if _, _, err := decodeProfile([]byte{0xff, 0x80}); err == nil {
		t.Fatal("accepted unknown encoding")
	}
}

func TestSetupChangedDuringConsentAndSymlink(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "profile")
	if err := os.WriteFile(path, []byte("new content"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := appendProfile(path, []byte("old content"), "old content", "utf8", "block"); err == nil {
		t.Fatal("overwrote concurrently changed profile")
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(path, link); err == nil {
		if _, err := appendProfile(link, []byte("new content"), "new content", "utf8", "block"); err == nil {
			t.Fatal("modified symlink target")
		}
	}
}

func TestZshSetupUsesNativeHomebrewAndStablePaths(t *testing.T) {
	for _, prefix := range []string{"/opt/homebrew", "/usr/local"} {
		block := zshSetupBlock(prefix+"/Cellar/nodi/0.2.1/bin/nodi", prefix)
		if strings.Contains(block, "Cellar") || strings.Contains(block, "source <(") || !strings.Contains(block, prefix+"/share/zsh/site-functions") || !strings.Contains(block, "compdef _nodi nodi") {
			t.Fatal(block)
		}
		state, err := setupState(block, zshSetupBlock(prefix+"/Cellar/nodi/0.2.2/bin/nodi", prefix), "zsh")
		if err != nil || !strings.HasPrefix(state, "CONFIGURED") {
			t.Fatal("upgrade changed block", state, err)
		}
	}
	standalone := zshSetupBlock("/Users/user/.local/bin/nodi", "")
	if !strings.Contains(standalone, "source <(command nodi completion zsh)") {
		t.Fatal(standalone)
	}
	if psQuote("C:\\user's\\nodi.exe") != "'C:\\user''s\\nodi.exe'" || shQuote("user's") != "'user'\\''s'" {
		t.Fatal("unsafe quoting")
	}
}

func TestSetupRejectsUnknownOptionsAndCompletes(t *testing.T) {
	var out bytes.Buffer
	a := New("test", strings.NewReader(""), &out)
	for _, args := range [][]string{{"setup", "--yes"}, {"setup", "--check", "extra"}, {"setup", "extra"}} {
		if err := a.Run(context.Background(), args); err == nil {
			t.Fatal(args)
		}
	}
	for _, tt := range []struct {
		words []string
		want  string
	}{{[]string{"se"}, "setup\n"}, {[]string{"setup", "--c"}, "--check\n"}} {
		out.Reset()
		if err := a.Run(context.Background(), append([]string{"__complete"}, tt.words...)); err != nil || out.String() != tt.want {
			t.Fatal(out.String(), err)
		}
	}
}
