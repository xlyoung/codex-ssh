package cli

import (
	"flag"
	"fmt"
	"strings"

	"codex-ssh-skill/internal/sftp"
	"codex-ssh-skill/pkg/model"
)

func (a App) runPut(args []string) int {
	fs := flag.NewFlagSet("put", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if fs.NArg() < 3 {
		fmt.Fprintln(a.Stderr, "Usage: codex-ssh put <host> <local_path> <remote_path>")
		fmt.Fprintln(a.Stderr)
		fmt.Fprintln(a.Stderr, "Upload a local file to a remote server.")
		fmt.Fprintln(a.Stderr)
		fmt.Fprintln(a.Stderr, "Examples:")
		fmt.Fprintln(a.Stderr, "  codex-ssh put myserver ./app.tar.gz /opt/app/")
		fmt.Fprintln(a.Stderr, "  codex-ssh put deploy@10.0.0.1 ./config.yaml /etc/app/")
		return 2
	}

	hostSpec := fs.Arg(0)
	localPath := fs.Arg(1)
	remotePath := fs.Arg(2)

	inv, err := a.loadInventoryForSFTP()
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}

	resolvedHost, err := resolveHostForSFTP(hostSpec, inv)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}

	cfg := sftp.ParseHost(resolvedHost)
	cfg.User = a.resolveUserForSFTP(hostSpec, inv)

	fmt.Fprintf(a.Stdout, "📤 Uploading %s → %s@%s:%s\n", localPath, cfg.User, cfg.Host, remotePath)

	progress := func(transferred, total int64, filename string) {
		if total > 0 {
			pct := float64(transferred) / float64(total) * 100
			fmt.Fprintf(a.Stdout, "\r  %s: %.1f%%", filename, pct)
		}
	}

	if err := sftp.Put(cfg, localPath, remotePath, progress); err != nil {
		fmt.Fprintf(a.Stderr, "Error: upload failed: %v\n", err)
		return 1
	}

	fmt.Fprintln(a.Stdout, "\n✅ Upload complete")
	return 0
}

func (a App) runGet(args []string) int {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if fs.NArg() < 3 {
		fmt.Fprintln(a.Stderr, "Usage: codex-ssh get <host> <remote_path> <local_path>")
		fmt.Fprintln(a.Stderr)
		fmt.Fprintln(a.Stderr, "Download a remote file to local.")
		fmt.Fprintln(a.Stderr)
		fmt.Fprintln(a.Stderr, "Examples:")
		fmt.Fprintln(a.Stderr, "  codex-ssh get myserver /var/log/app.log ./app.log")
		fmt.Fprintln(a.Stderr, "  codex-ssh get deploy@10.0.0.1 /etc/nginx/nginx.conf ./nginx.conf")
		return 2
	}

	hostSpec := fs.Arg(0)
	remotePath := fs.Arg(1)
	localPath := fs.Arg(2)

	inv, err := a.loadInventoryForSFTP()
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}

	resolvedHost, err := resolveHostForSFTP(hostSpec, inv)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}

	cfg := sftp.ParseHost(resolvedHost)
	cfg.User = a.resolveUserForSFTP(hostSpec, inv)

	fmt.Fprintf(a.Stdout, "📥 Downloading %s@%s:%s → %s\n", cfg.User, cfg.Host, remotePath, localPath)

	progress := func(transferred, total int64, filename string) {
		if total > 0 {
			pct := float64(transferred) / float64(total) * 100
			fmt.Fprintf(a.Stdout, "\r  %s: %.1f%%", filename, pct)
		}
	}

	if err := sftp.Get(cfg, remotePath, localPath, progress); err != nil {
		fmt.Fprintf(a.Stderr, "Error: download failed: %v\n", err)
		return 1
	}

	fmt.Fprintln(a.Stdout, "\n✅ Download complete")
	return 0
}

func (a App) runSync(args []string) int {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if fs.NArg() < 3 {
		fmt.Fprintln(a.Stderr, "Usage: codex-ssh sync <host> <local_dir> <remote_dir>")
		fmt.Fprintln(a.Stderr)
		fmt.Fprintln(a.Stderr, "Sync a local directory to a remote directory.")
		fmt.Fprintln(a.Stderr, "Files with same size are skipped (incremental sync).")
		fmt.Fprintln(a.Stderr)
		fmt.Fprintln(a.Stderr, "Examples:")
		fmt.Fprintln(a.Stderr, "  codex-ssh sync myserver ./dist/ /var/www/html/")
		fmt.Fprintln(a.Stderr, "  codex-ssh sync deploy@10.0.0.1 ./config/ /etc/app/")
		return 2
	}

	hostSpec := fs.Arg(0)
	localDir := fs.Arg(1)
	remoteDir := fs.Arg(2)

	inv, err := a.loadInventoryForSFTP()
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}

	resolvedHost, err := resolveHostForSFTP(hostSpec, inv)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error: %v\n", err)
		return 1
	}

	cfg := sftp.ParseHost(resolvedHost)
	cfg.User = a.resolveUserForSFTP(hostSpec, inv)

	fmt.Fprintf(a.Stdout, "🔄 Syncing %s → %s@%s:%s\n", localDir, cfg.User, cfg.Host, remoteDir)

	progress := func(transferred, total int64, filename string) {
		fmt.Fprintf(a.Stdout, "  📄 %s\n", filename)
	}

	if err := sftp.Sync(cfg, localDir, remoteDir, progress); err != nil {
		fmt.Fprintf(a.Stderr, "Error: sync failed: %v\n", err)
		return 1
	}

	return 0
}

func (a App) loadInventoryForSFTP() (model.Inventory, error) {
	paths, err := a.loadPaths()
	if err != nil {
		return model.Inventory{}, err
	}
	return a.loadInventoryFromPaths(paths)
}

func (a App) loadPaths() (model.Paths, error) {
	// Use config to resolve paths
	return a.resolvePaths()
}

func (a App) resolvePaths() (model.Paths, error) {
	// Placeholder - will use config.ResolvePaths in real implementation
	return model.Paths{}, nil
}

func (a App) loadInventoryFromPaths(paths model.Paths) (model.Inventory, error) {
	// Placeholder - will use hosts.Load in real implementation
	return model.Inventory{}, nil
}

func resolveHostForSFTP(hostSpec string, inv model.Inventory) (string, error) {
	name := hostSpec
	if idx := strings.Index(hostSpec, "@"); idx >= 0 {
		name = hostSpec[idx+1:]
	}
	if idx := strings.Index(name, ":"); idx >= 0 {
		name = name[:idx]
	}

	for hostName, h := range inv.Hosts {
		if hostName == name {
			addr := h.Host
			if h.Port != 0 && h.Port != 22 {
				addr = fmt.Sprintf("%s:%d", addr, h.Port)
			}
			if h.User != "" {
				addr = h.User + "@" + addr
			}
			return addr, nil
		}
	}

	if strings.Contains(hostSpec, "@") || strings.Contains(hostSpec, ".") {
		return hostSpec, nil
	}

	return "", fmt.Errorf("host %q not found in inventory", name)
}

func (a App) resolveUserForSFTP(hostSpec string, inv model.Inventory) string {
	if idx := strings.Index(hostSpec, "@"); idx >= 0 {
		return hostSpec[:idx]
	}
	name := hostSpec
	if idx := strings.Index(name, ":"); idx >= 0 {
		name = name[:idx]
	}
	for hostName, h := range inv.Hosts {
		if hostName == name {
			if h.User != "" {
				return h.User
			}
		}
	}
	return "root"
}
