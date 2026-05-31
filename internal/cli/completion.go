package cli

import (
	"fmt"
	"strings"

	"codex-ssh-skill/pkg/model"
)

func (a App) runCompletion(inv model.Inventory, args []string) int {
	shell := "bash"
	if len(args) > 0 {
		shell = strings.ToLower(args[0])
	}

	var script string
	switch shell {
	case "bash":
		script = a.generateBashCompletion(inv)
	case "zsh":
		script = a.generateZshCompletion(inv)
	case "fish":
		script = a.generateFishCompletion(inv)
	default:
		fmt.Fprintf(a.Stderr, "unsupported shell: %s (supported: bash, zsh, fish)\n", shell)
		return 2
	}

	fmt.Fprint(a.Stdout, script)
	return 0
}

func (a App) hostAliases(inv model.Inventory) []string {
	aliases := make([]string, 0, len(inv.Hosts))
	for alias := range inv.Hosts {
		aliases = append(aliases, alias)
	}
	return aliases
}

func (a App) allTags(inv model.Inventory) []string {
	seen := map[string]struct{}{}
	for _, host := range inv.Hosts {
		for _, tag := range host.Tags {
			seen[tag] = struct{}{}
		}
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	return tags
}

func (a App) generateBashCompletion(inv model.Inventory) string {
	subcommands := []string{"hosts", "secret", "exec", "shell", "tunnel", "proxy", "job", "audit", "diagnose", "mcp", "completion", "help"}
	aliases := a.hostAliases(inv)
	tags := a.allTags(inv)

	var b strings.Builder
	b.WriteString(`_codex_ssh() {
    local cur prev words cword
    _init_completion || return

    local subcommands="`)
	b.WriteString(strings.Join(subcommands, " "))
	b.WriteString(`"

    local hosts="`)
	b.WriteString(strings.Join(aliases, " "))
	b.WriteString(`"

    local tags="`)
	b.WriteString(strings.Join(tags, " "))
	b.WriteString(`"

    # Complete subcommands at position 1
    if [ "$cword" -eq 1 ]; then
        COMPREPLY=($(compgen -W "$subcommands" -- "$cur"))
        return
    fi

    local subcmd="${words[1]}"

    case "$subcmd" in
        hosts)
            local host_subcommands="list show set import-ssh-config remove test"
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=($(compgen -W "$host_subcommands" -- "$cur"))
            elif [ "$cword" -eq 3 ]; then
                case "${words[2]}" in
                    show|remove|test)
                        COMPREPLY=($(compgen -W "$hosts" -- "$cur"))
                        ;;
                esac
            fi
            ;;
        exec)
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=($(compgen -W "$hosts @all $tags" -- "$cur"))
            fi
            COMPREPLY=($(compgen -o default -W "-host -user -via -auth -cwd -timeout" -- "$cur"))
            ;;
        shell)
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=($(compgen -W "$hosts" -- "$cur"))
            fi
            COMPREPLY=($(compgen -o default -W "-host -user -via -auth -cwd" -- "$cur"))
            ;;
        secret)
            local secret_subcommands="set get delete"
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=($(compgen -W "$secret_subcommands" -- "$cur"))
            fi
            ;;
        tunnel)
            local tunnel_subcommands="list stop"
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=($(compgen -W "$tunnel_subcommands $hosts" -- "$cur"))
            fi
            ;;
        proxy)
            local proxy_subcommands="list stop"
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=($(compgen -W "$proxy_subcommands $hosts" -- "$cur"))
            fi
            ;;
        job)
            local job_subcommands="run status attach stop logs"
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=($(compgen -W "$job_subcommands" -- "$cur"))
            fi
            ;;
        audit)
            local audit_subcommands="tail query"
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=($(compgen -W "$audit_subcommands" -- "$cur"))
            fi
            ;;
        diagnose)
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=($(compgen -W "$hosts" -- "$cur"))
            fi
            ;;
        mcp)
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=($(compgen -W "serve" -- "$cur"))
            fi
            ;;
        completion)
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=($(compgen -W "bash zsh fish" -- "$cur"))
            fi
            ;;
    esac
}

complete -F _codex_ssh codex-ssh
`)
	return b.String()
}

func (a App) generateZshCompletion(inv model.Inventory) string {
	subcommands := []string{"hosts", "secret", "exec", "shell", "tunnel", "proxy", "job", "audit", "diagnose", "mcp", "completion", "help"}
	aliases := a.hostAliases(inv)
	tags := a.allTags(inv)

	var b strings.Builder
	b.WriteString(`#compdef codex-ssh

_codex_ssh() {
    local -a subcommands hosts tags
    subcommands=(
`)
	for _, s := range subcommands {
		b.WriteString(fmt.Sprintf("        '%s'\n", s))
	}
	b.WriteString(`    )

    hosts=(
`)
	for _, h := range aliases {
		b.WriteString(fmt.Sprintf("        '%s'\n", h))
	}
	b.WriteString(`    )

    tags=(
`)
	for _, t := range tags {
		b.WriteString(fmt.Sprintf("        '@%s'\n", t))
	}
	b.WriteString(`    )

    _arguments -C \
        '1:command:->command' \
        '*::arg:->args'

    case $state in
        command)
            _describe 'command' subcommands
            ;;
        args)
            case $words[1] in
                hosts)
                    _arguments \
                        '1:subcommand:(list show set import-ssh-config remove test)' \
                        '*::hosts:_hosts'
                    ;;
                exec)
                    _arguments \
                        '1:host:->hosts_or_tags' \
                        '*::options:-host[Target host]-user[SSH user]-via[Jump host]-auth[Auth mode]-cwd[Working dir]-timeout[Command timeout]'
                    ;;
                shell)
                    _arguments \
                        '1:host:_hosts' \
                        '*::options:-host[Target host]-user[SSH user]-via[Jump host]-auth[Auth mode]-cwd[Working dir]'
                    ;;
                secret)
                    _arguments \
                        '1:subcommand:(set get delete)'
                    ;;
                tunnel)
                    _arguments \
                        '1:tunnel_action_or_host:(list stop)' \
                        '*::args:_tunnel_args'
                    ;;
                proxy)
                    _arguments \
                        '1:proxy_action_or_host:(list stop)' \
                        '*::args:_proxy_args'
                    ;;
                job)
                    _arguments \
                        '1:job_subcommand:(run status attach stop logs)'
                    ;;
                audit)
                    _arguments \
                        '1:audit_subcommand:(tail query)'
                    ;;
                diagnose)
                    _arguments \
                        '1:host:_hosts'
                    ;;
                mcp)
                    _arguments \
                        '1:mcp_subcommand:(serve)'
                    ;;
                completion)
                    _arguments \
                        '1:shell:(bash zsh fish)'
                    ;;
            esac
            ;;
    esac
}

_codex_ssh "$@"
`)
	return b.String()
}

func (a App) generateFishCompletion(inv model.Inventory) string {
	aliases := a.hostAliases(inv)
	tags := a.allTags(inv)

	var b strings.Builder
	b.WriteString(`# fish completions for codex-ssh

# Helper: all host aliases
function __codex_ssh_hosts
`)
	for _, h := range aliases {
		b.WriteString(fmt.Sprintf("    echo '%s'\n", h))
	}
	b.WriteString(`end

# Helper: all tags (with @ prefix)
function __codex_ssh_tags
    echo 'all'
`)
	for _, t := range tags {
		b.WriteString(fmt.Sprintf("    echo '%s'\n", t))
	}
	b.WriteString(`end

# Top-level subcommands
complete -c codex-ssh -f
complete -c codex-ssh -n '__fish_use_subcommand' -a 'hosts' -d 'Manage hosts inventory'
complete -c codex-ssh -n '__fish_use_subcommand' -a 'secret' -d 'Manage secrets'
complete -c codex-ssh -n '__fish_use_subcommand' -a 'exec' -d 'Execute command on remote host'
complete -c codex-ssh -n '__fish_use_subcommand' -a 'shell' -d 'Open interactive shell'
complete -c codex-ssh -n '__fish_use_subcommand' -a 'tunnel' -d 'Manage SSH tunnels'
complete -c codex-ssh -n '__fish_use_subcommand' -a 'proxy' -d 'Manage SSH proxies'
complete -c codex-ssh -n '__fish_use_subcommand' -a 'job' -d 'Manage background jobs'
complete -c codex-ssh -n '__fish_use_subcommand' -a 'audit' -d 'View audit log'
complete -c codex-ssh -n '__fish_use_subcommand' -a 'diagnose' -d 'Diagnose remote host'
complete -c codex-ssh -n '__fish_use_subcommand' -a 'mcp' -d 'MCP server'
complete -c codex-ssh -n '__fish_use_subcommand' -a 'completion' -d 'Generate shell completions'
complete -c codex-ssh -n '__fish_use_subcommand' -a 'help' -d 'Show help'

# hosts subcommands
complete -c codex-ssh -n '__fish_seen_subcommand_from hosts' -a 'list' -d 'List all hosts'
complete -c codex-ssh -n '__fish_seen_subcommand_from hosts' -a 'show' -d 'Show host details'
complete -c codex-ssh -n '__fish_seen_subcommand_from hosts' -a 'set' -d 'Add or update a host'
complete -c codex-ssh -n '__fish_seen_subcommand_from hosts' -a 'import-ssh-config' -d 'Import from ~/.ssh/config'
complete -c codex-ssh -n '__fish_seen_subcommand_from hosts' -a 'remove' -d 'Remove a host'
complete -c codex-ssh -n '__fish_seen_subcommand_from hosts' -a 'test' -d 'Test host connectivity'

# hosts show/remove/test take a host alias
complete -c codex-ssh -n '__fish_seen_subcommand_from show; and __fish_seen_subcommand_from hosts' -a '(__codex_ssh_hosts)'
complete -c codex-ssh -n '__fish_seen_subcommand_from remove; and __fish_seen_subcommand_from hosts' -a '(__codex_ssh_hosts)'
complete -c codex-ssh -n '__fish_seen_subcommand_from test; and __fish_seen_subcommand_from hosts' -a '(__codex_ssh_hosts)'

# exec: complete hosts and tags
complete -c codex-ssh -n '__fish_seen_subcommand_from exec' -a '(__codex_ssh_hosts)' -d 'Host alias'
complete -c codex-ssh -n '__fish_seen_subcommand_from exec' -a '(__codex_ssh_tags)' -d 'Tag group'
complete -c codex-ssh -n '__fish_seen_subcommand_from exec' -l 'host' -d 'Target host'
complete -c codex-ssh -n '__fish_seen_subcommand_from exec' -l 'user' -d 'SSH user'
complete -c codex-ssh -n '__fish_seen_subcommand_from exec' -l 'via' -d 'Jump host'
complete -c codex-ssh -n '__fish_seen_subcommand_from exec' -l 'auth' -d 'Auth mode'
complete -c codex-ssh -n '__fish_seen_subcommand_from exec' -l 'cwd' -d 'Working directory'
complete -c codex-ssh -n '__fish_seen_subcommand_from exec' -l 'timeout' -d 'Command timeout'

# shell
complete -c codex-ssh -n '__fish_seen_subcommand_from shell' -a '(__codex_ssh_hosts)'

# tunnel/proxy subcommands
complete -c codex-ssh -n '__fish_seen_subcommand_from tunnel' -a 'list' -d 'List tunnels'
complete -c codex-ssh -n '__fish_seen_subcommand_from tunnel' -a 'stop' -d 'Stop a tunnel'
complete -c codex-ssh -n '__fish_seen_subcommand_from proxy' -a 'list' -d 'List proxies'
complete -c codex-ssh -n '__fish_seen_subcommand_from proxy' -a 'stop' -d 'Stop a proxy'

# job subcommands
complete -c codex-ssh -n '__fish_seen_subcommand_from job' -a 'run' -d 'Run a background job'
complete -c codex-ssh -n '__fish_seen_subcommand_from job' -a 'status' -d 'Check job status'
complete -c codex-ssh -n '__fish_seen_subcommand_from job' -a 'attach' -d 'Attach to job output'
complete -c codex-ssh -n '__fish_seen_subcommand_from job' -a 'stop' -d 'Stop a job'
complete -c codex-ssh -n '__fish_seen_subcommand_from job' -a 'logs' -d 'View job logs'

# audit subcommands
complete -c codex-ssh -n '__fish_seen_subcommand_from audit' -a 'tail' -d 'Tail audit log'
complete -c codex-ssh -n '__fish_seen_subcommand_from audit' -a 'query' -d 'Query audit log'

# mcp subcommand
complete -c codex-ssh -n '__fish_seen_subcommand_from mcp' -a 'serve' -d 'Start MCP server'

# completion subcommand
complete -c codex-ssh -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'
`)
	return b.String()
}
