package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"codex-ssh-skill/internal/keys"
	"codex-ssh-skill/pkg/model"
)

func (a App) runKeys(paths model.Paths, cfg model.Config, inv model.Inventory, args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(a.Stderr, "Usage: codex-ssh keys <list|check|agent>\n")
		return 2
	}

	switch args[0] {
	case "list":
		return a.runKeysList()
	case "check":
		return a.runKeysCheck()
	case "agent":
		return a.runKeysAgent()
	default:
		fmt.Fprintf(a.Stderr, "Unknown subcommand: %s\n", args[0])
		fmt.Fprintf(a.Stderr, "Usage: codex-ssh keys <list|check|agent>\n")
		return 2
	}
}

func (a App) runKeysList() int {
	mgr := keys.NewManager()
	keyList, err := mgr.ListKeys(nil)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error listing keys: %v\n", err)
		return 1
	}

	if len(keyList) == 0 {
		fmt.Fprintln(a.Stdout, "No SSH keys found.")
		return 0
	}

	w := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "PATH\tTYPE\tFINGERPRINT\tAGENT\tKEYCHAIN\tMODIFIED\n")
	for _, k := range keyList {
		agent := "✗"
		if k.InAgent {
			agent = "✓"
		}
		keychain := "✗"
		if k.InKeychain {
			keychain = "✓"
		}
		modified := k.Modified.Format("2006-01-02 15:04")
		path := k.Path
		home := os.Getenv("HOME")
		if home != "" && strings.HasPrefix(path, home) {
			path = "~" + path[len(home):]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			path, k.Type, k.Fingerprint, agent, keychain, modified)
	}
	w.Flush()

	fmt.Fprintf(a.Stdout, "\n%d key(s) found across all sources.\n", len(keyList))
	return 0
}

func (a App) runKeysCheck() int {
	mgr := keys.NewManager()
	keyList, err := mgr.ListKeys(nil)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error listing keys: %v\n", err)
		return 1
	}

	issues := 0
	for _, k := range keyList {
		info, err := os.Stat(k.Path)
		if err != nil {
			fmt.Fprintf(a.Stdout, "⚠ %s: cannot stat\n", k.Path)
			issues++
			continue
		}

		perm := info.Mode().Perm()
		if perm != 0600 && perm != 0400 {
			fmt.Fprintf(a.Stdout, "⚠ %s: permissions %o (should be 600 or 400)\n", k.Path, perm)
			issues++
		}

		pubPath := k.Path + ".pub"
		if _, err := os.Stat(pubPath); err != nil {
			fmt.Fprintf(a.Stdout, "⚠ %s: missing public key\n", k.Path)
			issues++
		}

		if !k.InAgent {
			fmt.Fprintf(a.Stdout, "ℹ %s: not loaded in SSH agent\n", k.Path)
		}
	}

	if issues == 0 {
		fmt.Fprintln(a.Stdout, "✓ All keys healthy.")
	} else {
		fmt.Fprintf(a.Stdout, "\n%d issue(s) found.\n", issues)
	}
	return 0
}

func (a App) runKeysAgent() int {
	socket := keys.GetAgentSocket()
	if socket == "" {
		fmt.Fprintln(a.Stdout, "✗ SSH agent not running (SSH_AUTH_SOCK not set)")
		fmt.Fprintln(a.Stdout)
		fmt.Fprintln(a.Stdout, "To start an agent:")
		fmt.Fprintln(a.Stdout, "  eval $(ssh-agent -s)")
		fmt.Fprintln(a.Stdout, "  ssh-add ~/.ssh/id_rsa")
		return 0
	}

	fmt.Fprintf(a.Stdout, "✓ SSH agent socket: %s\n", socket)

	if keys.IsAgentRunning(context.Background()) {
		fmt.Fprintln(a.Stdout, "✓ Agent is responsive")
	} else {
		fmt.Fprintln(a.Stdout, "✗ Agent not responding (socket exists but agent dead?)")
	}

	return 0
}
