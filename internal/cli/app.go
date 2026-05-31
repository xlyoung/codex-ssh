package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codex-ssh-skill/internal/askpass"
	"codex-ssh-skill/internal/audit"
	"codex-ssh-skill/internal/config"
	"codex-ssh-skill/internal/executor"
	"codex-ssh-skill/internal/hosts"
	"codex-ssh-skill/internal/jobs"
	"codex-ssh-skill/internal/proxy"
	iruntime "codex-ssh-skill/internal/runtime"
	"codex-ssh-skill/internal/secrets"
	"codex-ssh-skill/internal/sshconfig"
	"codex-ssh-skill/internal/tunnel"
	"codex-ssh-skill/internal/validate"
	"codex-ssh-skill/pkg/model"
	"golang.org/x/term"
)

type App struct {
	Stdout           io.Writer
	Stderr           io.Writer
	Runner           executor.Runner
	LookPath         func(string) (string, error)
	KnownHostsLookup func(string, model.ResolvedHost) (bool, error)
	KnownHostsFetch  func(model.ResolvedHost) (string, error)
	SecretStore      secrets.Store
	PasswordReader   func(prompt string) (string, error)
}

func New(stdout io.Writer, stderr io.Writer, runner executor.Runner) App {
	return App{
		Stdout:         stdout,
		Stderr:         stderr,
		Runner:         runner,
		LookPath:       osexec.LookPath,
		SecretStore:    secrets.NewStore(),
		PasswordReader: defaultPasswordReader,
	}
}

func (a App) Run(args []string) int {
	if len(args) == 0 {
		a.printUsage()
		return 2
	}

	paths, cfg, inv, logger, err := a.loadContext()
	if err != nil {
		fmt.Fprintf(a.Stderr, "load context: %v\n", err)
		return 1
	}

	switch args[0] {
	case "hosts":
		return a.runHosts(paths, cfg, inv, logger, args[1:])
	case "secret":
		return a.runSecret(cfg, inv, args[1:])
	case "exec":
		return a.runExec(paths, cfg, inv, logger, args[1:])
	case "shell":
		return a.runShell(paths, cfg, inv, logger, args[1:])
	case "tunnel":
		return a.runTunnel(paths, cfg, inv, logger, args[1:])
	case "proxy":
		return a.runProxy(paths, cfg, inv, logger, args[1:])
	case "job":
		return a.runJob(paths, cfg, inv, logger, args[1:])
	case "audit":
		return a.runAudit(cfg, logger, args[1:])
	case "diagnose":
		return a.runDiagnose(paths, cfg, inv, logger, args[1:])
	case "help", "--help", "-h":
		a.printUsage()
		return 0
	default:
		fmt.Fprintf(a.Stderr, "unknown command: %s\n", args[0])
		a.printUsage()
		return 2
	}
}

func (a App) loadContext() (model.Paths, model.Config, model.Inventory, audit.Logger, error) {
	paths, err := config.ResolvePaths()
	if err != nil {
		return model.Paths{}, model.Config{}, model.Inventory{}, audit.Logger{}, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return model.Paths{}, model.Config{}, model.Inventory{}, audit.Logger{}, err
	}
	inv, err := hosts.Load(paths.HostsFile)
	if err != nil {
		return model.Paths{}, model.Config{}, model.Inventory{}, audit.Logger{}, err
	}
	return paths, cfg, inv, audit.NewLogger(cfg.LogDir), nil
}

func (a App) runHosts(paths model.Paths, cfg model.Config, inv model.Inventory, logger audit.Logger, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.Stderr, "hosts requires a subcommand")
		return 2
	}
	switch args[0] {
	case "list":
		if len(inv.Hosts) == 0 {
			a.printInventoryBootstrapGuidance()
			return 0
		}
		aliases := make([]string, 0, len(inv.Hosts))
		for alias := range inv.Hosts {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		for _, alias := range aliases {
			host := inv.Hosts[alias]
			fmt.Fprintf(a.Stdout, "%s\t%s\t%s\t%d\t%s\n", alias, host.Host, firstNonEmpty(host.User, cfg.DefaultUser), firstNonZero(host.Port, cfg.DefaultPort), strings.Join(host.Via, ","))
		}
		return 0
	case "show":
		if len(args) < 2 {
			fmt.Fprintln(a.Stderr, "usage: hosts show <alias>")
			return 2
		}
		host, ok := inv.Hosts[args[1]]
		if !ok {
			fmt.Fprintf(a.Stderr, "host not found: %s\n", args[1])
			return 1
		}
		data, _ := json.MarshalIndent(host, "", "  ")
		fmt.Fprintln(a.Stdout, string(data))
		return 0
	case "set":
		return a.runHostsSet(paths, inv, args[1:])
	case "import-ssh-config":
		return a.runHostsImportSSHConfig(paths, inv, args[1:])
	case "remove":
		if len(args) < 2 {
			fmt.Fprintln(a.Stderr, "usage: hosts remove <alias>")
			return 2
		}
		delete(inv.Hosts, args[1])
		if err := hosts.Save(paths.HostsFile, inv); err != nil {
			fmt.Fprintf(a.Stderr, "save hosts: %v\n", err)
			return 1
		}
		fmt.Fprintf(a.Stdout, "removed host %s\n", args[1])
		return 0
	case "test":
		if len(args) < 2 {
			fmt.Fprintln(a.Stderr, "usage: hosts test <alias>")
			return 2
		}
		fs := flag.NewFlagSet("hosts test", flag.ContinueOnError)
		fs.SetOutput(a.Stderr)
		timeout := fs.Duration("timeout", 10*time.Second, "test timeout")
		if err := fs.Parse(args[2:]); err != nil {
			return 2
		}
		if err := a.preflightSSH(); err != nil {
			fmt.Fprintf(a.Stderr, "ssh preflight failed: %v\n", err)
			return 1
		}
		resolved, err := hosts.Resolve(inv, cfg, args[1])
		if err != nil {
			fmt.Fprintf(a.Stderr, "resolve host: %v\n", err)
			return 1
		}
		if err := validateResolvedHost(cfg, resolved); err != nil {
			fmt.Fprintf(a.Stderr, "host validation failed: %v\n", err)
			return 1
		}
		if err := a.ensureKnownHosts(cfg, resolved); err != nil {
			fmt.Fprintf(a.Stderr, "accept host key failed: %v\n", err)
			return 1
		}
		ctx, cancel := timeoutContext(*timeout)
		defer cancel()
		authEnv, cleanupAuth, err := a.preparePasswordAuthEnv(ctx, paths, cfg, resolved, args[1])
		if err != nil {
			fmt.Fprintf(a.Stderr, "password auth setup failed: %v\n", err)
			return 1
		}
		defer cleanupAuth()
		svc := executor.Service{Runner: a.Runner, Logger: logger, Config: cfg}
		result, err := svc.Exec(ctx, model.ExecRequest{
			Alias:        args[1],
			Command:      hostProbeCommand(),
			AuthEnv:      authEnv,
			ResolvedHost: resolved,
		})
		if err != nil {
			a.printSSHFailure("connectivity test failed", err, result.Stderr)
			return 1
		}
		caps, err := parseHostProbe(result.Stdout)
		if err != nil {
			fmt.Fprintf(a.Stderr, "host probe parse failed: %v\n", err)
			return 1
		}
		via := "-"
		if len(resolved.Via) > 0 {
			via = strings.Join(viaAliases(resolved.Via), ",")
		}
		fmt.Fprintf(a.Stdout, "host %s reachable target=%s via=%s tmux=%s nohup=%s\n", args[1], resolved.Host, via, caps["tmux"], caps["nohup"])
		return 0
	default:
		fmt.Fprintf(a.Stderr, "unknown hosts subcommand: %s\n", args[0])
		return 2
	}
}

func (a App) runHostsImportSSHConfig(paths model.Paths, inv model.Inventory, args []string) int {
	fs := flag.NewFlagSet("hosts import-ssh-config", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	file := fs.String("file", "", "path to ssh config file")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	configPath := strings.TrimSpace(*file)
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(a.Stderr, "resolve home dir: %v\n", err)
			return 1
		}
		configPath = filepath.Join(home, ".ssh", "config")
	}

	imported, err := sshconfig.LoadFile(configPath)
	if err != nil {
		fmt.Fprintf(a.Stderr, "load ssh config: %v\n", err)
		return 1
	}
	if len(imported.Hosts) == 0 {
		fmt.Fprintf(a.Stdout, "no importable hosts found in %s\n", configPath)
		return 0
	}

	merged := sshconfig.Merge(inv, imported)
	if err := hosts.Save(paths.HostsFile, merged); err != nil {
		fmt.Fprintf(a.Stderr, "save hosts: %v\n", err)
		return 1
	}

	fmt.Fprintf(a.Stdout, "imported %d hosts from %s\n", len(imported.Hosts), configPath)
	return 0
}

func (a App) runHostsSet(paths model.Paths, inv model.Inventory, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.Stderr, "usage: hosts set <alias> --host <host> [options]")
		return 2
	}
	alias := args[0]
	fs := flag.NewFlagSet("hosts set", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	hostValue := fs.String("host", "", "target host or ip")
	user := fs.String("user", "", "ssh user")
	port := fs.Int("port", 0, "ssh port")
	via := fs.String("via", "", "comma separated jump aliases")
	auth := fs.String("auth", "", "auth mode: agent, identity_file, or password")
	identityFile := fs.String("identity-file", "", "path to private key file")
	workdir := fs.String("workdir", "", "default remote workdir")
	tags := fs.String("tags", "", "comma separated tags")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *hostValue == "" {
		fmt.Fprintln(a.Stderr, "--host is required")
		return 2
	}
	inv.Hosts[alias] = model.Host{
		Host:         *hostValue,
		User:         *user,
		Port:         *port,
		Via:          splitCSV(*via),
		Tags:         splitCSV(*tags),
		Workdir:      *workdir,
		Auth:         *auth,
		IdentityFile: *identityFile,
	}
	if err := hosts.Save(paths.HostsFile, inv); err != nil {
		fmt.Fprintf(a.Stderr, "save hosts: %v\n", err)
		return 1
	}
	fmt.Fprintf(a.Stdout, "saved host %s\n", alias)
	return 0
}

func (a App) runSecret(cfg model.Config, inv model.Inventory, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.Stderr, "usage: secret <set|get|delete> [<alias-or-target> | --host <host>] [--user <user>] [--port <port>]")
		return 2
	}

	subcommand := args[0]
	positional, parsed := parseLeadingAlias(args[1:])
	fs := flag.NewFlagSet("secret "+subcommand, flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	var target secretTargetInput
	addSecretTargetFlags(fs, &target)
	show := fs.Bool("show", false, "print password value (secret get only)")
	if err := fs.Parse(parsed); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(a.Stderr, "unexpected extra arguments")
		return 2
	}

	resolved, err := resolveSecretTarget(cfg, inv, positional, target)
	if err != nil {
		fmt.Fprintf(a.Stderr, "resolve target: %v\n", err)
		return 1
	}
	ref := secrets.RefForHost(resolved)
	store := a.secretStore()

	switch subcommand {
	case "set":
		password, err := a.readPassword("Password: ")
		if err != nil {
			fmt.Fprintf(a.Stderr, "read password: %v\n", err)
			return 1
		}
		if strings.TrimSpace(password) == "" {
			fmt.Fprintln(a.Stderr, "password cannot be empty")
			return 1
		}
		if err := store.Set(context.Background(), ref, password); err != nil {
			fmt.Fprintf(a.Stderr, "store secret: %v\n", err)
			return 1
		}
		fmt.Fprintf(a.Stdout, "secret stored for %s\n", ref)
		return 0
	case "get":
		password, err := store.Get(context.Background(), ref)
		if err != nil {
			fmt.Fprintf(a.Stderr, "load secret: %v\n", err)
			return 1
		}
		if *show {
			fmt.Fprintln(a.Stdout, password)
		} else {
			fmt.Fprintf(a.Stdout, "secret found for %s (hidden; use --show to print)\n", ref)
		}
		return 0
	case "delete":
		if err := store.Delete(context.Background(), ref); err != nil {
			fmt.Fprintf(a.Stderr, "delete secret: %v\n", err)
			return 1
		}
		fmt.Fprintf(a.Stdout, "secret deleted for %s\n", ref)
		return 0
	default:
		fmt.Fprintf(a.Stderr, "unknown secret subcommand: %s\n", subcommand)
		return 2
	}
}

func (a App) runExec(paths model.Paths, cfg model.Config, inv model.Inventory, logger audit.Logger, args []string) int {
	opts, command, ok := splitRemoteCommandArgs(args)
	if !ok {
		fmt.Fprintln(a.Stderr, "usage: exec [<alias> | --host <host>] [--user <user>] [--via <jump>] [--auth <mode>] [--cwd <dir>] [--timeout <duration>] -- <command>")
		return 2
	}
	alias, parsedArgs := parseLeadingAlias(opts)

	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	var target targetInput
	addTargetFlags(fs, &target, false)
	cwd := fs.String("cwd", "", "remote working directory")
	timeout := fs.Duration("timeout", 0, "command timeout, e.g. 30s")
	if err := fs.Parse(parsedArgs); err != nil {
		return 2
	}

	resolved, err := resolveTarget(cfg, inv, alias, target)
	if err != nil {
		fmt.Fprintf(a.Stderr, "resolve host: %v\n", err)
		return 1
	}
	if err := validateResolvedHost(cfg, resolved); err != nil {
		fmt.Fprintf(a.Stderr, "host validation failed: %v\n", err)
		return 1
	}
	if err := a.ensureKnownHosts(cfg, resolved); err != nil {
		fmt.Fprintf(a.Stderr, "accept host key failed: %v\n", err)
		return 1
	}
	commandCWD := *cwd
	if commandCWD == "" {
		commandCWD = resolved.Workdir
	}
	rawTarget := alias
	if rawTarget == "" {
		rawTarget = target.Host
	}

	ctx := context.Background()
	if *timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *timeout)
		defer cancel()
	}
	authEnv, cleanupAuth, err := a.preparePasswordAuthEnv(ctx, paths, cfg, resolved, rawTarget)
	if err != nil {
		fmt.Fprintf(a.Stderr, "password auth setup failed: %v\n", err)
		return 1
	}
	defer cleanupAuth()

	svc := executor.Service{Runner: a.Runner, Logger: logger, Config: cfg}
	result, err := svc.Exec(ctx, model.ExecRequest{
		Alias:        resolved.Alias,
		Command:      command,
		CWD:          commandCWD,
		Timeout:      *timeout,
		AuthEnv:      authEnv,
		ResolvedHost: resolved,
	})
	if result.Stdout != "" {
		fmt.Fprint(a.Stdout, result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprint(a.Stderr, result.Stderr)
	}
	if err != nil {
		fmt.Fprintf(a.Stderr, "exec failed: %v\n", err)
		return 1
	}
	return 0
}

func (a App) runShell(paths model.Paths, cfg model.Config, inv model.Inventory, logger audit.Logger, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.Stderr, "usage: shell [<alias> | --host <host>] [--user <user>] [--via <jump>] [--auth <mode>] [--cwd <dir>]")
		return 2
	}
	alias, parsedArgs := parseLeadingAlias(args)
	fs := flag.NewFlagSet("shell", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	var target targetInput
	addTargetFlags(fs, &target, false)
	cwd := fs.String("cwd", "", "remote working directory")
	if err := fs.Parse(parsedArgs); err != nil {
		return 2
	}
	resolved, err := resolveTarget(cfg, inv, alias, target)
	if err != nil {
		fmt.Fprintf(a.Stderr, "resolve host: %v\n", err)
		return 1
	}
	if err := validateResolvedHost(cfg, resolved); err != nil {
		fmt.Fprintf(a.Stderr, "host validation failed: %v\n", err)
		return 1
	}
	if err := a.ensureKnownHosts(cfg, resolved); err != nil {
		fmt.Fprintf(a.Stderr, "accept host key failed: %v\n", err)
		return 1
	}
	shellCWD := *cwd
	if shellCWD == "" {
		shellCWD = resolved.Workdir
	}
	rawTarget := alias
	if rawTarget == "" {
		rawTarget = target.Host
	}
	authEnv, cleanupAuth, err := a.preparePasswordAuthEnv(context.Background(), paths, cfg, resolved, rawTarget)
	if err != nil {
		fmt.Fprintf(a.Stderr, "password auth setup failed: %v\n", err)
		return 1
	}
	defer cleanupAuth()
	svc := executor.Service{Runner: a.Runner, Logger: logger, Config: cfg}
	if err := svc.Shell(context.Background(), model.ShellRequest{
		Alias:        resolved.Alias,
		CWD:          shellCWD,
		AuthEnv:      authEnv,
		ResolvedHost: resolved,
	}); err != nil {
		fmt.Fprintf(a.Stderr, "shell failed: %v\n", err)
		return 1
	}
	return 0
}

func (a App) runTunnel(paths model.Paths, cfg model.Config, inv model.Inventory, logger audit.Logger, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.Stderr, "usage: tunnel [<alias> | --host <host>] --local <port> --target <host:port> [--user <user>] [--via <jump>] [--ttl <duration>] [--background]")
		return 2
	}
	switch args[0] {
	case "list":
		states, err := tunnel.List(paths.TunnelsDir)
		if err != nil {
			fmt.Fprintf(a.Stderr, "list tunnels: %v\n", err)
			return 1
		}
		for _, state := range states {
			fmt.Fprintf(a.Stdout, "%s\t%s\t%s:%d -> %s:%d\tpid=%d\n", state.ID, state.Alias, state.LocalHost, state.LocalPort, state.TargetHost, state.TargetPort, state.PID)
		}
		return 0
	case "stop":
		if len(args) < 2 {
			fmt.Fprintln(a.Stderr, "usage: tunnel stop <id>")
			return 2
		}
		if err := tunnel.Stop(paths.TunnelsDir, args[1]); err != nil {
			fmt.Fprintf(a.Stderr, "stop tunnel: %v\n", err)
			return 1
		}
		fmt.Fprintf(a.Stdout, "stopped tunnel %s\n", args[1])
		return 0
	}

	alias, parsedArgs := parseLeadingAlias(args)
	fs := flag.NewFlagSet("tunnel", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	var targetInput targetInput
	addTargetFlags(fs, &targetInput, false)
	localPort := fs.Int("local", 0, "local port")
	target := fs.String("target", "", "target host:port")
	ttlRaw := fs.String("ttl", "", "auto-stop tunnel after duration, e.g. 30m; 0 disables auto-stop")
	background := fs.Bool("background", false, "run in background")
	bind := fs.String("bind", "127.0.0.1", "local bind address")
	if err := fs.Parse(parsedArgs); err != nil {
		return 2
	}
	resolved, err := resolveTarget(cfg, inv, alias, targetInput)
	if err != nil {
		fmt.Fprintf(a.Stderr, "resolve host: %v\n", err)
		return 1
	}
	if err := validateResolvedHost(cfg, resolved); err != nil {
		fmt.Fprintf(a.Stderr, "host validation failed: %v\n", err)
		return 1
	}
	if err := a.ensureKnownHosts(cfg, resolved); err != nil {
		fmt.Fprintf(a.Stderr, "accept host key failed: %v\n", err)
		return 1
	}
	if *localPort == 0 || *target == "" {
		fmt.Fprintln(a.Stderr, "--local and --target are required")
		return 2
	}
	if *background && resolved.Auth == "password" {
		fmt.Fprintln(a.Stderr, "background tunnel does not support password auth; use foreground mode or key-based auth")
		return 2
	}
	ttl, err := resolveTunnelTTL(cfg, *ttlRaw)
	if err != nil {
		fmt.Fprintf(a.Stderr, "resolve tunnel ttl: %v\n", err)
		return 2
	}
	ctx := context.Background()
	if !*background && ttl > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, ttl)
		defer cancel()
	}
	rawTarget := alias
	if rawTarget == "" {
		rawTarget = targetInput.Host
	}
	authEnv, cleanupAuth, err := a.preparePasswordAuthEnv(ctx, paths, cfg, resolved, rawTarget)
	if err != nil {
		fmt.Fprintf(a.Stderr, "password auth setup failed: %v\n", err)
		return 1
	}
	defer cleanupAuth()
	targetHost, targetPort, err := validate.ParseTarget(*target)
	if err != nil {
		fmt.Fprintf(a.Stderr, "parse target: %v\n", err)
		return 2
	}
	state, err := tunnel.Start(ctx, a.Runner, logger, cfg, paths.TunnelsDir, model.TunnelRequest{
		Alias:        resolved.Alias,
		LocalHost:    *bind,
		LocalPort:    *localPort,
		TargetHost:   targetHost,
		TargetPort:   targetPort,
		AuthEnv:      authEnv,
		Background:   *background,
		ResolvedHost: resolved,
	})
	if err != nil {
		fmt.Fprintf(a.Stderr, "start tunnel: %v\n", err)
		return 1
	}
	fmt.Fprintf(a.Stdout, "tunnel %s %s:%d -> %s:%d\n", state.ID, state.LocalHost, state.LocalPort, state.TargetHost, state.TargetPort)
	return 0
}

func (a App) runProxy(paths model.Paths, cfg model.Config, inv model.Inventory, logger audit.Logger, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.Stderr, "usage: proxy [<alias> | --host <host>] --local <port> [--user <user>] [--via <jump>] [--background]")
		return 2
	}
	switch args[0] {
	case "list":
		states, err := proxy.List(paths.ProxiesDir)
		if err != nil {
			fmt.Fprintf(a.Stderr, "list proxies: %v\n", err)
			return 1
		}
		for _, state := range states {
			fmt.Fprintf(a.Stdout, "%s\t%s\t%s:%d\tpid=%d\n", state.ID, state.Alias, state.LocalHost, state.LocalPort, state.PID)
		}
		return 0
	case "stop":
		if len(args) < 2 {
			fmt.Fprintln(a.Stderr, "usage: proxy stop <id>")
			return 2
		}
		if err := proxy.Stop(paths.ProxiesDir, args[1]); err != nil {
			fmt.Fprintf(a.Stderr, "stop proxy: %v\n", err)
			return 1
		}
		fmt.Fprintf(a.Stdout, "stopped proxy %s\n", args[1])
		return 0
	}

	alias, parsedArgs := parseLeadingAlias(args)
	fs := flag.NewFlagSet("proxy", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	var targetInput targetInput
	addTargetFlags(fs, &targetInput, false)
	localPort := fs.Int("local", 0, "local port")
	background := fs.Bool("background", false, "run in background")
	bind := fs.String("bind", "127.0.0.1", "local bind address")
	if err := fs.Parse(parsedArgs); err != nil {
		return 2
	}
	resolved, err := resolveTarget(cfg, inv, alias, targetInput)
	if err != nil {
		fmt.Fprintf(a.Stderr, "resolve host: %v\n", err)
		return 1
	}
	if err := validateResolvedHost(cfg, resolved); err != nil {
		fmt.Fprintf(a.Stderr, "host validation failed: %v\n", err)
		return 1
	}
	if err := a.ensureKnownHosts(cfg, resolved); err != nil {
		fmt.Fprintf(a.Stderr, "accept host key failed: %v\n", err)
		return 1
	}
	if *localPort == 0 {
		fmt.Fprintln(a.Stderr, "--local is required")
		return 2
	}
	if *background && resolved.Auth == "password" {
		fmt.Fprintln(a.Stderr, "background proxy does not support password auth; use foreground mode or key-based auth")
		return 2
	}
	rawTarget := alias
	if rawTarget == "" {
		rawTarget = targetInput.Host
	}
	authEnv, cleanupAuth, err := a.preparePasswordAuthEnv(context.Background(), paths, cfg, resolved, rawTarget)
	if err != nil {
		fmt.Fprintf(a.Stderr, "password auth setup failed: %v\n", err)
		return 1
	}
	defer cleanupAuth()
	state, err := proxy.Start(context.Background(), a.Runner, logger, cfg, paths.ProxiesDir, model.ProxyRequest{
		Alias:        resolved.Alias,
		LocalHost:    *bind,
		LocalPort:    *localPort,
		AuthEnv:      authEnv,
		Background:   *background,
		ResolvedHost: resolved,
	})
	if err != nil {
		fmt.Fprintf(a.Stderr, "start proxy: %v\n", err)
		return 1
	}
	fmt.Fprintf(a.Stdout, "proxy %s %s:%d\n", state.ID, state.LocalHost, state.LocalPort)
	return 0
}

func (a App) runJob(paths model.Paths, cfg model.Config, inv model.Inventory, logger audit.Logger, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.Stderr, "usage: job <run|status|attach|stop|logs> ...")
		return 2
	}
	service := jobs.Service{Runner: a.Runner, Logger: logger, Config: cfg, JobsDir: paths.JobsDir}

	switch args[0] {
	case "run":
		alias, opts, command, ok := splitCommandArgs(args[1:])
		if !ok {
			fmt.Fprintln(a.Stderr, "usage: job run <alias> [--cwd <dir>] [--mode <auto|tmux|nohup>] -- <command>")
			return 2
		}
		resolved, err := hosts.Resolve(inv, cfg, alias)
		if err != nil {
			fmt.Fprintf(a.Stderr, "resolve host: %v\n", err)
			return 1
		}
		if err := validateResolvedHost(cfg, resolved); err != nil {
			fmt.Fprintf(a.Stderr, "host validation failed: %v\n", err)
			return 1
		}
		if err := a.ensureKnownHosts(cfg, resolved); err != nil {
			fmt.Fprintf(a.Stderr, "accept host key failed: %v\n", err)
			return 1
		}
		fs := flag.NewFlagSet("job run", flag.ContinueOnError)
		fs.SetOutput(a.Stderr)
		cwd := fs.String("cwd", resolved.Workdir, "remote working directory")
		mode := fs.String("mode", "auto", "job mode: auto, tmux, nohup")
		if err := fs.Parse(opts); err != nil {
			return 2
		}
		ctx := context.Background()
		authEnv, cleanupAuth, err := a.preparePasswordAuthEnv(ctx, paths, cfg, resolved, alias)
		if err != nil {
			fmt.Fprintf(a.Stderr, "password auth setup failed: %v\n", err)
			return 1
		}
		defer cleanupAuth()
		state, err := service.Run(ctx, model.JobRequest{
			Alias:        alias,
			Command:      command,
			CWD:          *cwd,
			Mode:         *mode,
			AuthEnv:      authEnv,
			ResolvedHost: resolved,
		})
		if err != nil {
			fmt.Fprintf(a.Stderr, "run job: %v\n", err)
			return 1
		}
		fmt.Fprintf(a.Stdout, "job %s started mode=%s\n", state.ID, state.Mode)
		return 0
	case "status":
		return a.runJobSimple(paths, service, cfg, inv, args[1:], func(ctx context.Context, req model.JobRequest) (string, error) {
			return service.Status(ctx, req)
		})
	case "logs":
		if len(args) < 2 {
			fmt.Fprintln(a.Stderr, "usage: job logs <id> [--lines <n>]")
			return 2
		}
		fs := flag.NewFlagSet("job logs", flag.ContinueOnError)
		fs.SetOutput(a.Stderr)
		lines := fs.Int("lines", 200, "number of lines")
		if err := fs.Parse(args[2:]); err != nil {
			return 2
		}
		state, host, err := a.loadJobStateAndHost(paths, cfg, inv, args[1])
		if err != nil {
			fmt.Fprintf(a.Stderr, "load job: %v\n", err)
			return 1
		}
		if err := a.ensureKnownHosts(cfg, host); err != nil {
			fmt.Fprintf(a.Stderr, "accept host key failed: %v\n", err)
			return 1
		}
		ctx := context.Background()
		authEnv, cleanupAuth, err := a.preparePasswordAuthEnv(ctx, paths, cfg, host, state.Alias)
		if err != nil {
			fmt.Fprintf(a.Stderr, "password auth setup failed: %v\n", err)
			return 1
		}
		defer cleanupAuth()
		output, err := service.Logs(ctx, model.JobRequest{ID: state.ID, Alias: state.Alias, AuthEnv: authEnv, ResolvedHost: host}, *lines)
		if output != "" {
			fmt.Fprint(a.Stdout, output)
		}
		if err != nil {
			fmt.Fprintf(a.Stderr, "job logs: %v\n", err)
			return 1
		}
		return 0
	case "attach":
		if len(args) < 2 {
			fmt.Fprintln(a.Stderr, "usage: job attach <id>")
			return 2
		}
		state, host, err := a.loadJobStateAndHost(paths, cfg, inv, args[1])
		if err != nil {
			fmt.Fprintf(a.Stderr, "load job: %v\n", err)
			return 1
		}
		if err := a.ensureKnownHosts(cfg, host); err != nil {
			fmt.Fprintf(a.Stderr, "accept host key failed: %v\n", err)
			return 1
		}
		ctx := context.Background()
		authEnv, cleanupAuth, err := a.preparePasswordAuthEnv(ctx, paths, cfg, host, state.Alias)
		if err != nil {
			fmt.Fprintf(a.Stderr, "password auth setup failed: %v\n", err)
			return 1
		}
		defer cleanupAuth()
		if err := service.Attach(ctx, model.JobRequest{ID: state.ID, Alias: state.Alias, AuthEnv: authEnv, ResolvedHost: host}); err != nil {
			fmt.Fprintf(a.Stderr, "job attach: %v\n", err)
			return 1
		}
		return 0
	case "stop":
		if len(args) < 2 {
			fmt.Fprintln(a.Stderr, "usage: job stop <id>")
			return 2
		}
		state, host, err := a.loadJobStateAndHost(paths, cfg, inv, args[1])
		if err != nil {
			fmt.Fprintf(a.Stderr, "load job: %v\n", err)
			return 1
		}
		if err := a.ensureKnownHosts(cfg, host); err != nil {
			fmt.Fprintf(a.Stderr, "accept host key failed: %v\n", err)
			return 1
		}
		ctx := context.Background()
		authEnv, cleanupAuth, err := a.preparePasswordAuthEnv(ctx, paths, cfg, host, state.Alias)
		if err != nil {
			fmt.Fprintf(a.Stderr, "password auth setup failed: %v\n", err)
			return 1
		}
		defer cleanupAuth()
		if err := service.Stop(ctx, model.JobRequest{ID: state.ID, Alias: state.Alias, AuthEnv: authEnv, ResolvedHost: host}); err != nil {
			fmt.Fprintf(a.Stderr, "job stop: %v\n", err)
			return 1
		}
		fmt.Fprintf(a.Stdout, "job %s stopped\n", state.ID)
		return 0
	default:
		fmt.Fprintf(a.Stderr, "unknown job subcommand: %s\n", args[0])
		return 2
	}
}

func (a App) runJobSimple(paths model.Paths, service jobs.Service, cfg model.Config, inv model.Inventory, args []string, fn func(context.Context, model.JobRequest) (string, error)) int {
	if len(args) < 1 {
		fmt.Fprintln(a.Stderr, "job subcommand requires <id>")
		return 2
	}
	state, host, err := a.loadJobStateAndHost(paths, cfg, inv, args[0])
	if err != nil {
		fmt.Fprintf(a.Stderr, "load job: %v\n", err)
		return 1
	}
	if err := a.ensureKnownHosts(cfg, host); err != nil {
		fmt.Fprintf(a.Stderr, "accept host key failed: %v\n", err)
		return 1
	}
	ctx := context.Background()
	authEnv, cleanupAuth, err := a.preparePasswordAuthEnv(ctx, paths, cfg, host, state.Alias)
	if err != nil {
		fmt.Fprintf(a.Stderr, "password auth setup failed: %v\n", err)
		return 1
	}
	defer cleanupAuth()
	output, err := fn(ctx, model.JobRequest{ID: state.ID, Alias: state.Alias, AuthEnv: authEnv, ResolvedHost: host})
	if output != "" {
		fmt.Fprint(a.Stdout, output)
		if !strings.HasSuffix(output, "\n") {
			fmt.Fprintln(a.Stdout)
		}
	}
	if err != nil {
		fmt.Fprintf(a.Stderr, "job query failed: %v\n", err)
		return 1
	}
	return 0
}

func (a App) loadJobStateAndHost(paths model.Paths, cfg model.Config, inv model.Inventory, id string) (model.JobState, model.ResolvedHost, error) {
	state, err := iruntime.LoadState[model.JobState](filepath.Join(paths.JobsDir, id+".json"))
	if err != nil {
		return model.JobState{}, model.ResolvedHost{}, err
	}
	if strings.TrimSpace(state.Connection.Host) != "" {
		host := state.Connection
		if strings.TrimSpace(host.Alias) == "" {
			host.Alias = state.Alias
		}
		return state, host, nil
	}
	host, err := hosts.Resolve(inv, cfg, state.Alias)
	return state, host, err
}

func (a App) runAudit(_ model.Config, logger audit.Logger, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.Stderr, "usage: audit <tail|query> [filters]")
		return 2
	}

	switch args[0] {
	case "tail":
		fs := flag.NewFlagSet("audit tail", flag.ContinueOnError)
		fs.SetOutput(a.Stderr)
		lines := fs.Int("lines", 20, "lines to display")
		format := fs.String("format", "json", "output format: json or text")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		events, err := logger.Query(model.AuditQuery{Limit: *lines})
		if err != nil {
			fmt.Fprintf(a.Stderr, "audit tail: %v\n", err)
			return 1
		}
		return a.printAuditEvents(events, *format)
	case "query":
		fs := flag.NewFlagSet("audit query", flag.ContinueOnError)
		fs.SetOutput(a.Stderr)
		hostAlias := fs.String("host", "", "filter by host alias")
		action := fs.String("action", "", "filter by action")
		status := fs.String("status", "", "filter by status")
		limit := fs.Int("limit", 100, "max events")
		format := fs.String("format", "json", "output format: json or text")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		events, err := logger.Query(model.AuditQuery{HostAlias: *hostAlias, Action: *action, Status: *status, Limit: *limit})
		if err != nil {
			fmt.Fprintf(a.Stderr, "audit query: %v\n", err)
			return 1
		}
		return a.printAuditEvents(events, *format)
	default:
		fmt.Fprintf(a.Stderr, "unknown audit subcommand: %s\n", args[0])
		return 2
	}
}

func (a App) runDiagnose(paths model.Paths, cfg model.Config, inv model.Inventory, logger audit.Logger, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(a.Stderr, "usage: diagnose [<alias> | --host <host>] [--user <user>] [--via <jump>] [--auth <mode>]")
		return 2
	}
	if err := a.preflightSSH(); err != nil {
		fmt.Fprintf(a.Stderr, "ssh preflight failed: %v\n", err)
		return 1
	}

	alias, parsedArgs := parseLeadingAlias(args)
	fs := flag.NewFlagSet("diagnose", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	var target targetInput
	addTargetFlags(fs, &target, false)
	timeout := fs.Duration("timeout", 10*time.Second, "diagnose timeout")
	if err := fs.Parse(parsedArgs); err != nil {
		return 2
	}

	resolved, err := resolveTarget(cfg, inv, alias, target)
	if err != nil {
		fmt.Fprintf(a.Stderr, "resolve host: %v\n", err)
		return 1
	}
	if err := validateResolvedHost(cfg, resolved); err != nil {
		fmt.Fprintf(a.Stderr, "host validation failed: %v\n", err)
		return 1
	}
	if err := a.ensureKnownHosts(cfg, resolved); err != nil {
		fmt.Fprintf(a.Stderr, "accept host key failed: %v\n", err)
		return 1
	}

	ctx, cancel := timeoutContext(*timeout)
	defer cancel()
	rawTarget := alias
	if rawTarget == "" {
		rawTarget = target.Host
	}
	authEnv, cleanupAuth, err := a.preparePasswordAuthEnv(ctx, paths, cfg, resolved, rawTarget)
	if err != nil {
		fmt.Fprintf(a.Stderr, "password auth setup failed: %v\n", err)
		return 1
	}
	defer cleanupAuth()

	svc := executor.Service{Runner: a.Runner, Logger: logger, Config: cfg}
	result, err := svc.Exec(ctx, model.ExecRequest{
		Alias:        resolved.Alias,
		Command:      diagnoseProbeCommand(),
		AuthEnv:      authEnv,
		ResolvedHost: resolved,
	})
	if err != nil {
		a.printSSHFailure("diagnose failed", err, result.Stderr)
		return 1
	}

	caps, err := parseProbe(result.Stdout, "__codex_ssh_diag__", map[string]string{
		"tmux":   "unknown",
		"nohup":  "unknown",
		"docker": "unknown",
		"sudo":   "unknown",
	})
	if err != nil {
		fmt.Fprintf(a.Stderr, "diagnose probe parse failed: %v\n", err)
		return 1
	}

	sshPath := "unknown"
	if a.LookPath == nil {
		a.LookPath = osexec.LookPath
	}
	if path, lookupErr := a.LookPath("ssh"); lookupErr == nil {
		sshPath = path
	}

	fmt.Fprintf(a.Stdout, "target=%s user=%s port=%d via=%s auth=%s ssh_path=%s strict_host_key_checking=%t allow_password_auth=%t known_hosts=%s\n",
		resolved.Host,
		resolved.User,
		resolved.Port,
		formatViaSummary(resolved.Via),
		resolved.Auth,
		sshPath,
		cfg.Security.StrictHostKeyChecking,
		cfg.Security.AllowPasswordAuth,
		knownHostsStatus(cfg),
	)
	fmt.Fprintf(a.Stdout, "tmux=%s nohup=%s docker=%s sudo=%s\n", caps["tmux"], caps["nohup"], caps["docker"], caps["sudo"])
	return 0
}

func (a App) preparePasswordAuthEnv(ctx context.Context, paths model.Paths, cfg model.Config, resolved model.ResolvedHost, rawTarget string) (map[string]string, func(), error) {
	if resolved.Auth != "password" {
		return nil, func() {}, nil
	}
	if !cfg.Security.AllowPasswordAuth {
		return nil, func() {}, fmt.Errorf("password auth is disabled by configuration")
	}

	ref := secrets.RefForHost(resolved)
	password, err := a.secretStore().Get(ctx, ref)
	if err != nil {
		if secrets.IsSecretNotFound(err) {
			return nil, func() {}, fmt.Errorf("password secret not found for %s; run %s", ref, secretSetSuggestion(rawTarget, resolved))
		}
		return nil, func() {}, fmt.Errorf("load password secret for %s: %w", ref, err)
	}
	prepared, err := askpass.Prepare(paths.AskpassDir, password)
	if err != nil {
		return nil, func() {}, fmt.Errorf("prepare askpass script: %w", err)
	}
	cleanup := func() {
		_ = prepared.Cleanup()
	}
	return prepared.Env, cleanup, nil
}

func (a App) secretStore() secrets.Store {
	if a.SecretStore != nil {
		return a.SecretStore
	}
	return secrets.NewStore()
}

func (a App) readPassword(prompt string) (string, error) {
	if a.PasswordReader != nil {
		return a.PasswordReader(prompt)
	}
	return defaultPasswordReader(prompt)
}

func defaultPasswordReader(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		password, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return string(password), nil
	}
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func secretSetSuggestion(rawTarget string, resolved model.ResolvedHost) string {
	if strings.TrimSpace(rawTarget) != "" && strings.TrimSpace(resolved.SecretRef) != "" {
		return fmt.Sprintf("codex-ssh secret set %s", rawTarget)
	}
	return fmt.Sprintf("codex-ssh secret set --host %s --user %s --port %d", resolved.Host, resolved.User, resolved.Port)
}

func (a App) printAuditEvents(events []model.AuditEvent, format string) int {
	if format == "text" {
		for _, event := range events {
			fmt.Fprintf(a.Stdout, "%s\t%s\t%s\t%s\t%s\n",
				event.Timestamp.Format(time.RFC3339),
				event.Action,
				event.HostAlias,
				event.Status,
				compactCommand(event.Command),
			)
		}
		return 0
	}
	for _, event := range events {
		data, _ := json.Marshal(event)
		fmt.Fprintln(a.Stdout, string(data))
	}
	return 0
}

func (a App) printUsage() {
	fmt.Fprintln(a.Stderr, `codex-ssh: OpenSSH wrapper for hosts, exec, shell, tunnel, proxy, job and audit operations

Usage:
  codex-ssh hosts <list|show|set|import-ssh-config|remove|test> ...
  codex-ssh secret <set|get|delete> [<alias-or-target> | --host <host>] [--user <user>] [--port <port>]
  codex-ssh exec [<alias> | --host <host>] [--user <user>] [--via <jump>] [--auth <mode>] [--cwd <dir>] [--timeout <duration>] -- <command>
  codex-ssh shell [<alias> | --host <host>] [--user <user>] [--via <jump>] [--auth <mode>] [--cwd <dir>]
  codex-ssh tunnel [<alias> | --host <host>] --local <port> --target <host:port> [--user <user>] [--via <jump>] [--ttl <duration>] [--background]
  codex-ssh tunnel <list|stop> ...
  codex-ssh proxy [<alias> | --host <host>] --local <port> [--user <user>] [--via <jump>] [--background]
  codex-ssh proxy <list|stop> ...
  codex-ssh job <run|status|attach|stop|logs> ...
  codex-ssh audit <tail|query> ...
  codex-ssh diagnose [<alias> | --host <host>] [--user <user>] [--via <jump>] [--auth <mode>]`)
}

func resolveTunnelTTL(cfg model.Config, raw string) (time.Duration, error) {
	value := strings.TrimSpace(cfg.DefaultTunnelTTL)
	if strings.TrimSpace(raw) != "" {
		value = strings.TrimSpace(raw)
	}
	if value == "" {
		return 0, nil
	}
	ttl, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", value)
	}
	if ttl < 0 {
		return 0, fmt.Errorf("duration must be >= 0")
	}
	return ttl, nil
}

func splitCommandArgs(args []string) (string, []string, string, bool) {
	if len(args) == 0 {
		return "", nil, "", false
	}
	alias := args[0]
	for i, arg := range args[1:] {
		if arg == "--" {
			command := strings.Join(args[i+2:], " ")
			return alias, args[1 : i+1], command, command != ""
		}
	}
	return "", nil, "", false
}

func splitRemoteCommandArgs(args []string) ([]string, string, bool) {
	if len(args) == 0 {
		return nil, "", false
	}
	for i, arg := range args {
		if arg == "--" {
			command := strings.Join(args[i+1:], " ")
			return args[:i], command, command != ""
		}
	}
	return nil, "", false
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	items := strings.Split(value, ",")
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if item != "" {
			return item
		}
	}
	return ""
}

func firstNonZero(items ...int) int {
	for _, item := range items {
		if item != 0 {
			return item
		}
	}
	return 0
}

func timeoutContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.Background(), func() {}
	}
	return context.WithTimeout(context.Background(), timeout)
}

func (a App) preflightSSH() error {
	if a.LookPath == nil {
		a.LookPath = osexec.LookPath
	}
	if _, err := a.LookPath("ssh"); err != nil {
		return fmt.Errorf("ssh binary not found: %w", err)
	}
	return nil
}

func (a App) ensureKnownHosts(cfg model.Config, resolved model.ResolvedHost) error {
	if !cfg.Security.StrictHostKeyChecking || !cfg.Security.ReuseSystemKnownHosts {
		return nil
	}
	path, err := systemKnownHostsPath()
	if err != nil {
		return fmt.Errorf("resolve known_hosts path: %w", err)
	}
	if err := ensureKnownHostsFile(path); err != nil {
		return fmt.Errorf("prepare known_hosts file: %w", err)
	}

	seen := map[string]struct{}{}
	for _, host := range knownHostsTargets(resolved) {
		key := knownHostQuery(host)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		present, err := a.lookupKnownHost(path, host)
		if err != nil {
			return fmt.Errorf("lookup %s: %w", key, err)
		}
		if present {
			continue
		}

		entry, err := a.fetchKnownHost(host)
		if err != nil {
			return fmt.Errorf("scan %s: %w", key, err)
		}
		if strings.TrimSpace(entry) == "" {
			return fmt.Errorf("scan %s: empty ssh-keyscan output", key)
		}
		if err := appendKnownHosts(path, entry); err != nil {
			return fmt.Errorf("write %s: %w", key, err)
		}
	}
	return nil
}

func systemKnownHostsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", "known_hosts"), nil
}

func ensureKnownHostsFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	return file.Close()
}

func knownHostsTargets(resolved model.ResolvedHost) []model.ResolvedHost {
	targets := make([]model.ResolvedHost, 0, len(resolved.Via)+1)
	targets = append(targets, resolved.Via...)
	targets = append(targets, resolved)
	return targets
}

func knownHostQuery(host model.ResolvedHost) string {
	if host.Port == 0 || host.Port == 22 {
		return host.Host
	}
	return fmt.Sprintf("[%s]:%d", host.Host, host.Port)
}

func (a App) lookupKnownHost(path string, host model.ResolvedHost) (bool, error) {
	if a.KnownHostsLookup != nil {
		return a.KnownHostsLookup(path, host)
	}
	query := knownHostQuery(host)
	cmd := osexec.Command("ssh-keygen", "-F", query, "-f", path)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}
	var exitErr *osexec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
}

func (a App) fetchKnownHost(host model.ResolvedHost) (string, error) {
	if a.KnownHostsFetch != nil {
		return a.KnownHostsFetch(host)
	}
	port := host.Port
	if port == 0 {
		port = 22
	}
	cmd := osexec.Command("ssh-keyscan", "-H", "-T", "5", "-p", fmt.Sprintf("%d", port), host.Host)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func appendKnownHosts(path string, entries string) error {
	body := strings.TrimRight(entries, "\n")
	if body == "" {
		return nil
	}
	prefix := ""
	if existing, err := os.ReadFile(path); err == nil && len(existing) > 0 && existing[len(existing)-1] != '\n' {
		prefix = "\n"
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(prefix + body + "\n")
	return err
}

func (a App) printSSHFailure(prefix string, err error, stderr string) {
	fmt.Fprintf(a.Stderr, "%s: %v\n", prefix, err)
	if detail := strings.TrimSpace(stderr); detail != "" {
		fmt.Fprintln(a.Stderr, detail)
	}
}

func validateIdentityFile(host model.ResolvedHost) error {
	if host.Auth != "identity_file" {
		return nil
	}
	if host.IdentityFile == "" {
		return fmt.Errorf("identity file auth requires identity_file")
	}
	if _, err := os.Stat(host.IdentityFile); err != nil {
		return fmt.Errorf("identity file %s is not accessible: %w", host.IdentityFile, err)
	}
	return nil
}

func validateResolvedHost(cfg model.Config, host model.ResolvedHost) error {
	if err := validateIdentityFile(host); err != nil {
		return err
	}
	if host.Auth == "password" && !cfg.Security.AllowPasswordAuth {
		return fmt.Errorf("password auth is configured for host %s but security.allow_password_auth=false", host.Alias)
	}
	return nil
}

func hostProbeCommand() string {
	return "printf '__codex_ssh_test__\\n'; if command -v tmux >/dev/null 2>&1; then echo tmux=yes; else echo tmux=no; fi; if command -v nohup >/dev/null 2>&1; then echo nohup=yes; else echo nohup=no; fi"
}

func parseHostProbe(output string) (map[string]string, error) {
	return parseProbe(output, "__codex_ssh_test__", map[string]string{
		"tmux":  "unknown",
		"nohup": "unknown",
	})
}

func diagnoseProbeCommand() string {
	return "printf '__codex_ssh_diag__\\n'; if command -v tmux >/dev/null 2>&1; then echo tmux=yes; else echo tmux=no; fi; if command -v nohup >/dev/null 2>&1; then echo nohup=yes; else echo nohup=no; fi; if command -v docker >/dev/null 2>&1; then echo docker=yes; else echo docker=no; fi; if command -v sudo >/dev/null 2>&1; then echo sudo=yes; else echo sudo=no; fi"
}

func parseProbe(output string, marker string, defaults map[string]string) (map[string]string, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != marker {
		return nil, fmt.Errorf("missing probe marker")
	}
	result := map[string]string{}
	for key, value := range defaults {
		result[key] = value
	}
	for _, line := range lines[1:] {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			result[key] = value
		}
	}
	return result, nil
}

func viaAliases(via []model.ResolvedHost) []string {
	out := make([]string, 0, len(via))
	for _, host := range via {
		out = append(out, host.Alias)
	}
	return out
}

func formatViaSummary(via []model.ResolvedHost) string {
	if len(via) == 0 {
		return "-"
	}
	return strings.Join(viaAliases(via), ",")
}

func knownHostsStatus(cfg model.Config) string {
	if !cfg.Security.ReuseSystemKnownHosts {
		return "disabled"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "unknown"
	}
	path := filepath.Join(home, ".ssh", "known_hosts")
	if _, err := os.Stat(path); err == nil {
		return "present"
	} else if os.IsNotExist(err) {
		return "missing"
	}
	return "unknown"
}

func (a App) printInventoryBootstrapGuidance() {
	fmt.Fprintln(a.Stdout, "inventory is empty")
	fmt.Fprintln(a.Stdout, "next steps:")
	fmt.Fprintln(a.Stdout, "  codex-ssh hosts set <alias> --host <host> --user <user> [--via <jump>]")
	fmt.Fprintln(a.Stdout, "  codex-ssh hosts import-ssh-config")
	fmt.Fprintln(a.Stdout, "  codex-ssh exec --host <host> --user <user> -- \"uname -a\"")
}

func compactCommand(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return "-"
	}
	if len(command) > 80 {
		return command[:77] + "..."
	}
	return command
}
