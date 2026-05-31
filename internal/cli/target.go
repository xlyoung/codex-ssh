package cli

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	"codex-ssh-skill/internal/hosts"
	"codex-ssh-skill/pkg/model"
)

type targetInput struct {
	Host         string
	User         string
	Port         int
	Via          string
	Auth         string
	IdentityFile string
	Workdir      string
}

type secretTargetInput struct {
	Host string
	User string
	Port int
}

func addTargetFlags(fs *flag.FlagSet, input *targetInput, includeWorkdir bool) {
	fs.StringVar(&input.Host, "host", "", "target host or ip")
	fs.StringVar(&input.User, "user", "", "ssh user")
	fs.IntVar(&input.Port, "port", 0, "ssh port")
	fs.StringVar(&input.Via, "via", "", "comma separated jump aliases or hosts")
	fs.StringVar(&input.Auth, "auth", "", "auth mode: agent, identity_file, or password")
	fs.StringVar(&input.IdentityFile, "identity-file", "", "path to private key file")
	if includeWorkdir {
		fs.StringVar(&input.Workdir, "workdir", "", "default remote workdir")
	}
}

func addSecretTargetFlags(fs *flag.FlagSet, input *secretTargetInput) {
	fs.StringVar(&input.Host, "host", "", "target host or alias")
	fs.StringVar(&input.User, "user", "", "ssh user")
	fs.IntVar(&input.Port, "port", 0, "ssh port")
}

func resolveTarget(cfg model.Config, inv model.Inventory, alias string, input targetInput) (model.ResolvedHost, error) {
	if alias != "" {
		if _, ok := inv.Hosts[alias]; ok {
			if input.Host != "" || input.User != "" || input.Port != 0 || input.Via != "" || input.Auth != "" || input.IdentityFile != "" || input.Workdir != "" {
				return model.ResolvedHost{}, fmt.Errorf("do not mix alias with --host/--user/--port/--via/--auth/--identity-file/--workdir flags")
			}
			return hosts.Resolve(inv, cfg, alias)
		}
		if input.Host != "" {
			return model.ResolvedHost{}, fmt.Errorf("do not mix positional target with --host")
		}
		input.Host = alias
	}
	if strings.TrimSpace(input.Host) == "" {
		return model.ResolvedHost{}, fmt.Errorf("either alias or --host is required")
	}

	var (
		resolved model.ResolvedHost
		err      error
	)
	if _, ok := inv.Hosts[input.Host]; ok {
		resolved, err = hosts.Resolve(inv, cfg, input.Host)
	} else {
		resolved, err = parseEndpointSpec(input.Host, cfg)
	}
	if err != nil {
		return model.ResolvedHost{}, err
	}
	return applyTargetOverrides(cfg, inv, resolved, input)
}

func parseLeadingAlias(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

func resolveSecretTarget(cfg model.Config, inv model.Inventory, positional string, input secretTargetInput) (model.ResolvedHost, error) {
	positional = strings.TrimSpace(positional)
	input.Host = strings.TrimSpace(input.Host)
	if positional != "" && input.Host != "" {
		return model.ResolvedHost{}, fmt.Errorf("do not mix positional target with --host")
	}

	target := positional
	if target == "" {
		target = input.Host
	}
	if target == "" {
		return model.ResolvedHost{}, fmt.Errorf("either alias/target or --host is required")
	}

	var (
		resolved model.ResolvedHost
		err      error
	)
	if _, ok := inv.Hosts[target]; ok {
		resolved, err = hosts.Resolve(inv, cfg, target)
	} else {
		resolved, err = parseEndpointSpec(target, cfg)
	}
	if err != nil {
		return model.ResolvedHost{}, err
	}
	if input.User != "" {
		resolved.User = input.User
	}
	if input.Port != 0 {
		resolved.Port = input.Port
	}
	return resolved, nil
}

func parseEndpointSpec(spec string, cfg model.Config) (model.ResolvedHost, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return model.ResolvedHost{}, fmt.Errorf("empty host specification")
	}

	user := cfg.DefaultUser
	hostPort := spec
	if rawUser, rest, ok := strings.Cut(spec, "@"); ok {
		user = rawUser
		hostPort = rest
	}

	host := hostPort
	port := cfg.DefaultPort
	if rawHost, rawPort, ok := strings.Cut(hostPort, ":"); ok {
		parsedPort, err := strconv.Atoi(rawPort)
		if err != nil {
			return model.ResolvedHost{}, fmt.Errorf("invalid port in host specification %q: %w", spec, err)
		}
		host = rawHost
		port = parsedPort
	}

	return model.ResolvedHost{
		Alias: host,
		Host:  host,
		User:  user,
		Port:  port,
		Auth:  cfg.DefaultAuth,
	}, nil
}

func resolveVia(cfg model.Config, inv model.Inventory, raw []string) ([]model.ResolvedHost, error) {
	via := make([]model.ResolvedHost, 0, len(raw))
	for _, item := range raw {
		if item == "" {
			continue
		}
		if _, ok := inv.Hosts[item]; ok {
			resolved, err := hosts.Resolve(inv, cfg, item)
			if err != nil {
				return nil, err
			}
			via = append(via, flattenVia(resolved)...)
			continue
		}
		resolved, err := parseEndpointSpec(item, cfg)
		if err != nil {
			return nil, err
		}
		via = append(via, resolved)
	}
	return via, nil
}

func flattenVia(host model.ResolvedHost) []model.ResolvedHost {
	out := make([]model.ResolvedHost, 0, len(host.Via)+1)
	out = append(out, host.Via...)
	host.Via = nil
	out = append(out, host)
	return out
}

func applyTargetOverrides(cfg model.Config, inv model.Inventory, resolved model.ResolvedHost, input targetInput) (model.ResolvedHost, error) {
	if input.User != "" {
		resolved.User = input.User
	}
	if input.Port != 0 {
		resolved.Port = input.Port
	}
	if input.Auth != "" {
		resolved.Auth = input.Auth
	}
	if input.IdentityFile != "" {
		resolved.IdentityFile = input.IdentityFile
	}
	if input.Workdir != "" {
		resolved.Workdir = input.Workdir
	}
	if strings.TrimSpace(input.Via) != "" {
		via, err := resolveVia(cfg, inv, splitCSV(input.Via))
		if err != nil {
			return model.ResolvedHost{}, err
		}
		resolved.Via = via
	}
	return resolved, nil
}
