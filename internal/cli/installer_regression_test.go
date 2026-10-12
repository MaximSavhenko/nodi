package cli

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRegressionB10InstallerPreservesOldBinary(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("native Windows/macOS required")
	}
	for _, mode := range []string{"exit7", "invalid", "wrong platform", "postcheck", "locked", "requested version"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "locked" && runtime.GOOS != "windows" {
				t.Skip("Windows executable sharing semantics")
			}
			root := t.TempDir()
			bundle := filepath.Join(root, "bundle")
			bin := filepath.Join(root, "bin")
			os.Mkdir(bundle, 0755)
			os.Mkdir(bin, 0755)
			suffix := ""
			if runtime.GOOS == "windows" {
				suffix = ".exe"
			}
			asset := "nodi-" + runtime.GOOS + "-" + runtime.GOARCH + suffix
			installed := filepath.Join(bin, "nodi"+suffix)
			build := func(output, body string) {
				t.Helper()
				src := filepath.Join(root, "fixture.go")
				if err := os.WriteFile(src, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"+suffix), "build", "-o", output, src)
				if b, err := cmd.CombinedOutput(); err != nil {
					t.Fatal(string(b), err)
				}
			}
			build(installed, fmt.Sprintf("package main;import \"fmt\";func main(){fmt.Println(%q)}", "nodi 0.4.1 "+runtime.GOOS+"/"+runtime.GOARCH))
			old, _ := os.ReadFile(installed)
			before := sha256.Sum256(old)
			body := "package main;import \"os\";func main(){os.Exit(7)}"
			switch mode {
			case "locked", "requested version":
				v := "0.4.1"
				if mode == "requested version" {
					v = "0.4.2"
				}
				body = fmt.Sprintf("package main;import \"fmt\";func main(){fmt.Println(%q)}", "nodi "+v+" "+runtime.GOOS+"/"+runtime.GOARCH)
			case "invalid":
				body = "package main;import \"fmt\";func main(){fmt.Println(\"not a Nodi version\")}"
			case "wrong platform":
				body = "package main;import \"fmt\";func main(){fmt.Println(\"nodi 0.4.1 wrong/arch\")}"
			case "postcheck":
				body = fmt.Sprintf("package main;import(\"fmt\";\"os\";\"path/filepath\");func main(){if filepath.Base(os.Args[0])==%q {os.Exit(7)};fmt.Println(%q)}", "nodi"+suffix, "nodi 0.4.1 "+runtime.GOOS+"/"+runtime.GOARCH)
			}
			payload := filepath.Join(bundle, asset)
			build(payload, body)
			data, _ := os.ReadFile(payload)
			os.WriteFile(filepath.Join(bundle, "SHA256SUMS"), []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(data), asset)), 0600)
			script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install-windows.ps1"))
			if err != nil {
				t.Fatal(err)
			}
			var cmd *exec.Cmd
			if runtime.GOOS == "windows" {
				cmd = exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-File", script, "-BundleDirectory", bundle, "-InstallDirectory", bin, "-NoPath", "-NoCompletion")
			} else {
				script, _ = filepath.Abs(filepath.Join("..", "..", "scripts", "install-macos.sh"))
				home := filepath.Join(root, "home")
				os.MkdirAll(filepath.Join(home, ".local"), 0755)
				if err = os.Rename(bin, filepath.Join(home, ".local", "bin")); err != nil {
					t.Fatal(err)
				}
				installed = filepath.Join(home, ".local", "bin", "nodi")
				cmd = exec.Command("sh", script, "--bundle", bundle, "--no-path", "--no-completion")
				cmd.Env = append(os.Environ(), "HOME="+home)
			}
			if mode == "requested version" {
				if runtime.GOOS == "windows" {
					cmd.Args = append(cmd.Args, "-Version", "0.4.1")
				} else {
					cmd.Args = append(cmd.Args, "--version", "0.4.1")
				}
			}
			if mode == "locked" {
				lock, err := os.Open(installed)
				if err != nil {
					t.Fatal(err)
				}
				defer lock.Close()
			}
			output, err := cmd.CombinedOutput()
			afterData, readErr := os.ReadFile(installed)
			if err == nil || readErr != nil || sha256.Sum256(afterData) != before {
				t.Fatalf("installer must fail and preserve old binary (%s): err=%v read=%v\n%s", mode, err, readErr, output)
			}
			if runtime.GOOS == "windows" {
				if shell, err := exec.LookPath("pwsh"); err == nil {
					cmd7 := exec.Command(shell, cmd.Args[1:]...)
					output, err := cmd7.CombinedOutput()
					data, readErr := os.ReadFile(installed)
					if err == nil || readErr != nil || sha256.Sum256(data) != before {
						t.Fatalf("PowerShell 7 did not preserve old binary: %v %v\n%s", err, readErr, output)
					}
				} else {
					t.Log("PowerShell 7 NOT VERIFIED on this host; CI supplies it")
				}
			}
		})
	}
}
