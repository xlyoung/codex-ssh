package cli

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"codex-ssh-skill/internal/audit"
	"codex-ssh-skill/internal/executor"
	"codex-ssh-skill/internal/hosts"
	"codex-ssh-skill/pkg/model"
)

type multiExecResult struct {
	Alias  string
	Result model.CommandResult
	Err    error
}

// resolveTagSpec parses a tag spec like "@web,@db" or "@all" and returns
// the list of individual tag names (without the @ prefix).
func resolveTagSpec(spec string) []string {
	spec = strings.TrimPrefix(spec, "@")
	parts := strings.Split(spec, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			tags = append(tags, p)
		}
	}
	return tags
}

// hostsForTags returns all host aliases matching any of the given tags.
// If "all" is among the tags, every host is returned.
func hostsForTags(inv model.Inventory, tags []string) ([]string, error) {
	tagSet := map[string]bool{}
	for _, t := range tags {
		tagSet[t] = true
	}
	selectAll := tagSet["all"]

	var matched []string
	for alias, host := range inv.Hosts {
		if selectAll {
			matched = append(matched, alias)
			continue
		}
		for _, t := range host.Tags {
			if tagSet[t] {
				matched = append(matched, alias)
				break
			}
		}
	}
	if len(matched) == 0 {
		return nil, fmt.Errorf("no hosts found matching tags: %s", strings.Join(tags, ","))
	}
	return matched, nil
}

// runMultiExec executes a command on multiple hosts in parallel, prefixing
// output with the hostname and reporting failures at the end.
func (a App) runMultiExec(
	paths model.Paths,
	cfg model.Config,
	inv model.Inventory,
	logger audit.Logger,
	tagSpec string,
	command string,
	cwd string,
	timeout time.Duration,
) int {
	tags := resolveTagSpec(tagSpec)
	aliases, err := hostsForTags(inv, tags)
	if err != nil {
		fmt.Fprintf(a.Stderr, "resolve hosts: %v\n", err)
		return 1
	}

	// Resolve all hosts first
	type resolvedAlias struct {
		Alias  string
		Host   model.ResolvedHost
	}
	var resolved []resolvedAlias
	for _, alias := range aliases {
		r, err := hosts.Resolve(inv, cfg, alias)
		if err != nil {
			fmt.Fprintf(a.Stderr, "resolve host %s: %v\n", alias, err)
			return 1
		}
		if err := validateResolvedHost(cfg, r); err != nil {
			fmt.Fprintf(a.Stderr, "validate host %s: %v\n", alias, err)
			return 1
		}
		resolved = append(resolved, resolvedAlias{Alias: alias, Host: r})
	}

	if len(resolved) == 0 {
		fmt.Fprintln(a.Stderr, "no hosts to execute on")
		return 1
	}

	fmt.Fprintf(a.Stderr, "Executing on %d host(s): %s\n", len(resolved), strings.Join(aliases, ", "))

	// Execute in parallel
	results := make([]multiExecResult, len(resolved))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, ra := range resolved {
		wg.Add(1)
		go func(idx int, ra resolvedAlias) {
			defer wg.Done()

			ctx := context.Background()
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}

			if err := a.ensureKnownHosts(cfg, ra.Host); err != nil {
				mu.Lock()
				results[idx] = multiExecResult{
					Alias: ra.Alias,
					Err:   fmt.Errorf("accept host key: %w", err),
				}
				mu.Unlock()
				return
			}

			rawTarget := ra.Alias
			authEnv, cleanupAuth, err := a.preparePasswordAuthEnv(ctx, paths, cfg, ra.Host, rawTarget)
			if err != nil {
				mu.Lock()
				results[idx] = multiExecResult{
					Alias: ra.Alias,
					Err:   fmt.Errorf("password auth: %w", err),
				}
				mu.Unlock()
				return
			}
			defer cleanupAuth()

			cmdCWD := cwd
			if cmdCWD == "" {
				cmdCWD = ra.Host.Workdir
			}

			svc := executor.Service{Runner: a.Runner, Logger: logger, Config: cfg}
			result, execErr := svc.Exec(ctx, model.ExecRequest{
				Alias:        ra.Host.Alias,
				Command:      command,
				CWD:          cmdCWD,
				Timeout:      timeout,
				AuthEnv:      authEnv,
				ResolvedHost: ra.Host,
			})

			mu.Lock()
			results[idx] = multiExecResult{
				Alias:  ra.Alias,
				Result: result,
				Err:    execErr,
			}
			mu.Unlock()
		}(i, ra)
	}
	wg.Wait()

	// Print results prefixed with hostname
	var failures []multiExecResult
	for _, res := range results {
		prefix := fmt.Sprintf("[%s] ", res.Alias)
		if res.Result.Stdout != "" {
			for _, line := range strings.Split(strings.TrimRight(res.Result.Stdout, "\n"), "\n") {
				fmt.Fprintf(a.Stdout, "%s%s\n", prefix, line)
			}
		}
		if res.Result.Stderr != "" {
			for _, line := range strings.Split(strings.TrimRight(res.Result.Stderr, "\n"), "\n") {
				fmt.Fprintf(a.Stderr, "%s%s\n", prefix, line)
			}
		}
		if res.Err != nil || res.Result.ExitCode != 0 {
			failures = append(failures, res)
		}
	}

	// Report failures
	if len(failures) > 0 {
		fmt.Fprintf(a.Stderr, "\n--- %d host(s) failed ---\n", len(failures))
		for _, f := range failures {
			if f.Err != nil {
				fmt.Fprintf(a.Stderr, "[%s] error: %v\n", f.Alias, f.Err)
			} else {
				fmt.Fprintf(a.Stderr, "[%s] exit code %d\n", f.Alias, f.Result.ExitCode)
			}
		}
		return 1
	}
	return 0
}
