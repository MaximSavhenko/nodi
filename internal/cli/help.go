package cli

import (
	"fmt"
	"strings"
)

var commandUsage = map[string]string{
	"send":    "nodi send PATH [--name PROJECT] [--include RELATIVE/FILE] [--dry-run]",
	"publish": "nodi publish [--minor|--major|--version VERSION] [--draft] [--include RELATIVE/FILE]\n  nodi publish PROJECT PATH --version VERSION [--status draft|released] [--description TEXT] [--metadata JSON] [--include RELATIVE/FILE]",
	"get":     "nodi get PROJECT [--version VERSION] [--output DIRECTORY]",
	"delete":  "nodi delete PROJECT --version VERSION|--all [--dry-run]\n  Exact typed confirmation is required, including a checked recovery retry.",
	"list":    "nodi list [--all]", "info": "nodi info PROJECT [--all]", "open": "nodi open PROJECT", "init": "nodi init PROJECT [--repo OWNER/REPO]", "status": "nodi status", "update": "nodi update --check", "version": "nodi version", "help": "nodi help", "config": "nodi config --repo OWNER/REPO",
	"setup":      "nodi setup [--repo OWNER/REPO] [--write] [--check]\n  nodi setup --completion [--check]",
	"completion": "nodi completion [powershell|bash|zsh]", "auth": "nodi auth login [--write]\n  nodi auth logout", "auth-login": "nodi auth login [--write]", "auth-logout": "nodi auth logout",
}

// Inspect help before config/network/action dispatch, while retaining errors
// for unknown options/commands and treating words after -- as literal data.
func commandHelp(command string, args []string) (string, bool, error) {
	original := command
	if command == "auth" && len(args) > 0 && args[0] != "--help" && args[0] != "-h" {
		if args[0] != "login" && args[0] != "logout" {
			return "", false, fmt.Errorf("unknown auth subcommand %q", args[0])
		}
		command = "auth-" + args[0]
		args = args[1:]
	}
	usage, known := commandUsage[command]
	if !known {
		return "", false, nil
	}
	flags := commandFlags[command]
	if strings.HasPrefix(command, "auth-") {
		flags = commandFlags["auth"]
		if command == "auth-logout" {
			flags = nil
		}
	}
	helpWanted := false
	var unknown error
	positionals := []string{}
	for i := 0; i < len(args); i++ {
		word := args[i]
		if word == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if word == "-h" || word == "--help" {
			helpWanted = true
			continue
		}
		if !strings.HasPrefix(word, "-") || word == "-" {
			positionals = append(positionals, word)
			continue
		}
		name, _, equal := strings.Cut(word, "=")
		name = "--" + strings.TrimLeft(name, "-")
		found := false
		for _, flag := range flags {
			if flag == name {
				found = true
			}
		}
		if !found {
			unknown = fmt.Errorf("unknown option %q for %s", word, original)
			continue
		}
		if valueFlags[name] && !equal {
			i++
			if i >= len(args) {
				unknown = fmt.Errorf("%s requires a value", name)
			}
		}
	}
	if !helpWanted {
		return "", false, nil
	}
	if unknown != nil {
		return "", true, unknown
	}
	max := map[string]int{"send": 1, "publish": 2, "get": 1, "delete": 1, "info": 1, "open": 1, "init": 1, "completion": 1}[command]
	if len(positionals) > max {
		return "", true, fmt.Errorf("unexpected positional arguments for %s", original)
	}
	if command == "completion" && len(positionals) == 1 && positionals[0] != "powershell" && positionals[0] != "bash" && positionals[0] != "zsh" {
		return "", true, fmt.Errorf("unknown completion shell")
	}
	return "Usage: " + usage + "\n  -h, --help  Show this help without accessing configuration or storage.\n", true, nil
}
