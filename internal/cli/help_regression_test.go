package cli

import (
	"bytes"
	"context"
	"nodi/internal/storage"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type noHelpInput struct{}

func (noHelpInput) Read([]byte) (int, error) { panic("help read input") }
func TestRegressionB11PublicCommandHelp(t *testing.T) {
	for _, command := range []string{"send", "publish", "get", "delete", "list", "info", "open", "init", "status", "update", "version", "help", "config", "setup", "setup --completion", "completion", "completion powershell", "auth", "auth login", "auth logout"} {
		for _, flag := range []string{"-h", "--help"} {
			t.Run(command+" "+flag, func(t *testing.T) {
				dir := filepath.Join(t.TempDir(), "absent")
				t.Setenv("NODI_CONFIG_DIR", dir)
				out := new(bytes.Buffer)
				a := New("test", noHelpInput{}, out)
				a.Store = func(bool) (storage.Store, error) { panic("help accessed store") }
				a.StoreFor = func(string, bool) (storage.Store, error) { panic("help accessed store") }
				a.Cwd = func() (string, error) { panic("help accessed cwd") }
				a.Open = func(string) error { panic("help opened path") }
				a.Connect = func(string, string) (storage.Store, error) { panic("help connected") }
				if err := a.Run(context.Background(), append(strings.Fields(command), flag)); err != nil {
					t.Fatal("help must succeed", err)
				}
				if !strings.Contains(out.String(), "nodi "+strings.Fields(command)[0]) {
					t.Fatal("missing command usage", out.String())
				}
				if _, err := os.Lstat(dir); !os.IsNotExist(err) {
					t.Fatal("help created config")
				}
			})
		}
	}
}
func TestB11UnknownArgumentsAndLiteralHelp(t *testing.T) {
	a := New("dev", strings.NewReader(""), new(bytes.Buffer))
	t.Setenv("NODI_CONFIG_DIR", filepath.Join(t.TempDir(), "absent"))
	for _, args := range [][]string{{"unknown", "--help"}, {"send", "--unknown", "--help"}, {"auth", "unknown", "--help"}, {"completion", "unknown", "--help"}, {"version", "extra", "--help"}, {"send", "--", "--help"}} {
		if err := a.Run(context.Background(), args); err == nil {
			t.Fatal("unknown/literal arguments accepted", args)
		}
	}
	for _, args := range [][]string{{"send", "data.txt", "--help"}, {"get", "--help", "demo"}, {"setup", "--completion", "--help"}} {
		if err := a.Run(context.Background(), args); err != nil {
			t.Fatal(args, err)
		}
	}
}
