package cli

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestB10InstallerCrashAndRollbackFailure(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("native installer platforms only")
	}
	for _, phase := range []string{"before", "after", "rollback"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "home", ".local", "bin")
			bundle := filepath.Join(root, "bundle")
			os.MkdirAll(bin, 0755)
			os.Mkdir(bundle, 0755)
			defer os.Chmod(bin, 0755)
			suffix := ""
			if runtime.GOOS == "windows" {
				suffix = ".exe"
			}
			installed := filepath.Join(bin, "nodi"+suffix)
			asset := "nodi-" + runtime.GOOS + "-" + runtime.GOARCH + suffix
			build := func(path, body string) {
				t.Helper()
				src := filepath.Join(root, "fixture.go")
				os.WriteFile(src, []byte(body), 0600)
				cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"+suffix), "build", "-o", path, src)
				if b, e := cmd.CombinedOutput(); e != nil {
					t.Fatal(string(b), e)
				}
			}
			version := "nodi 0.4.1 " + runtime.GOOS + "/" + runtime.GOARCH
			build(installed, fmt.Sprintf("package main;import \"fmt\";func main(){fmt.Println(%q)}", version))
			old, _ := os.ReadFile(installed)
			oldHash := sha256.Sum256(old)
			body := fmt.Sprintf(`package main
import("fmt";"os";"os/exec";"path/filepath";"runtime";"time")
func mark(){os.WriteFile(os.Getenv("NODI_INSTALL_TEST_MARKER"),[]byte(fmt.Sprint(os.Getpid())),0600)}
func main(){
 if len(os.Args)>1 && os.Args[1]=="hold" {f,e:=os.Open(os.Args[2]);if e!=nil {os.Exit(9)};defer f.Close();mark();time.Sleep(time.Minute);return}
 phase:=os.Getenv("NODI_INSTALL_TEST_PHASE");base:=filepath.Base(os.Args[0]);installed:=base=="nodi" || base=="nodi.exe";staged:=base=="candidate" || base=="candidate.exe"
 if phase=="before" && staged || phase=="after" && installed {mark();time.Sleep(time.Minute);return}
 if phase=="rollback" && installed {
  if runtime.GOOS=="windows" {cmd:=exec.Command(os.Args[0],"hold",os.Args[0]);if cmd.Start()!=nil {os.Exit(8)};for i:=0;i<1000;i++ {if _,e:=os.Stat(os.Getenv("NODI_INSTALL_TEST_MARKER"));e==nil {os.Exit(7)};time.Sleep(10*time.Millisecond)};os.Exit(8)}
  mark();os.Chmod(filepath.Dir(os.Args[0]),0500);os.Exit(7)
 }
 fmt.Println(%q)
}`, version)
			payload := filepath.Join(bundle, asset)
			build(payload, body)
			data, _ := os.ReadFile(payload)
			candidateHash := sha256.Sum256(data)
			os.WriteFile(filepath.Join(bundle, "SHA256SUMS"), []byte(fmt.Sprintf("%x  %s\n", candidateHash, asset)), 0600)
			marker := filepath.Join(root, "barrier")
			script, _ := filepath.Abs(filepath.Join("..", "..", "scripts", "install-windows.ps1"))
			var cmd *exec.Cmd
			if runtime.GOOS == "windows" {
				cmd = exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-File", script, "-BundleDirectory", bundle, "-InstallDirectory", bin, "-NoPath", "-NoCompletion")
			} else {
				script, _ = filepath.Abs(filepath.Join("..", "..", "scripts", "install-macos.sh"))
				cmd = exec.Command("sh", script, "--bundle", bundle, "--no-path", "--no-completion")
			}
			cmd.Env = append(os.Environ(), "HOME="+filepath.Join(root, "home"), "NODI_INSTALL_TEST_PHASE="+phase, "NODI_INSTALL_TEST_MARKER="+marker)
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			// The marker is written solely by our compiled fixture executable. Kill
			// exactly that process and this installer, never enumerate user processes.
			killFixture := func() {
				b, e := os.ReadFile(marker)
				if e == nil {
					pid, e := strconv.Atoi(string(b))
					if e == nil && pid > 0 && (phase != "rollback" || runtime.GOOS == "windows") {
						p, e := os.FindProcess(pid)
						if e == nil {
							p.Kill()
						}
					}
				}
			}
			defer killFixture()
			defer cmd.Process.Kill()
			if phase == "rollback" {
				if err := cmd.Wait(); err == nil || !strings.Contains(strings.ToLower(out.String()), "rollback failed") {
					t.Fatal("rollback failure not reported", err, out.String())
				}
				killFixture()
				os.Chmod(bin, 0755)
			} else {
				deadline := time.Now().Add(30 * time.Second)
				ready := false
				for time.Now().Before(deadline) {
					if _, err := os.Stat(marker); err == nil {
						ready = true
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
				if !ready {
					cmd.Process.Kill()
					cmd.Wait()
					t.Fatal("installer barrier never reached", out.String())
				}
				cmd.Process.Kill()
				killFixture()
				cmd.Wait()
			}
			current, err := os.ReadFile(installed)
			if err != nil {
				t.Fatal("lost installed binary", err)
			}
			if phase == "before" {
				if sha256.Sum256(current) != oldHash {
					t.Fatal("precommit crash lost working binary")
				}
				return
			}
			if sha256.Sum256(current) != candidateHash {
				t.Fatal("postcommit crash exposed a partial/unknown executable")
			}
			found := false
			err = filepath.WalkDir(bin, func(p string, d fs.DirEntry, e error) error {
				if e != nil {
					return e
				}
				if !d.IsDir() && (d.Name() == "previous.exe" || d.Name() == "previous") {
					b, e := os.ReadFile(p)
					if e == nil && sha256.Sum256(b) == oldHash {
						found = true
					}
				}
				return nil
			})
			if err != nil || !found {
				t.Fatal("exact old backup missing after interruption/rollback failure", err)
			}
		})
	}
}
