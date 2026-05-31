package playbook

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"codex-ssh-skill/internal/executor"
	"codex-ssh-skill/internal/hosts"
	"codex-ssh-skill/pkg/model"
	"gopkg.in/yaml.v3"
)

// Options controls how a playbook is executed.
type Options struct {
	DryRun bool
}

// Load parses a YAML playbook file.
func Load(path string) (*Playbook, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read playbook: %w", err)
	}
	var pb Playbook
	if err := yaml.Unmarshal(data, &pb); err != nil {
		return nil, fmt.Errorf("parse playbook: %w", err)
	}
	return &pb, nil
}

// Validate checks a playbook for structural errors without executing it.
func Validate(pb *Playbook) error {
	if strings.TrimSpace(pb.Name) == "" {
		return fmt.Errorf("playbook name is required")
	}
	if strings.TrimSpace(pb.Hosts) == "" {
		return fmt.Errorf("hosts field is required")
	}
	if len(pb.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}
	for i, step := range pb.Steps {
		if strings.TrimSpace(step.Exec) == "" {
			return fmt.Errorf("step %d: exec is required", i+1)
		}
		if step.Retries < 0 {
			return fmt.Errorf("step %d: retries must be >= 0", i+1)
		}
		if step.Delay < 0 {
			return fmt.Errorf("step %d: delay must be >= 0", i+1)
		}
	}
	return nil
}

// resolveHosts resolves the hosts field to a list of host aliases.
func resolveHosts(inv model.Inventory, hostsSpec string) ([]string, error) {
	spec := strings.TrimSpace(hostsSpec)
	if strings.HasPrefix(spec, "@") {
		tags := resolveTagSpec(spec)
		return hostsForTags(inv, tags)
	}
	// Comma-separated list of individual host aliases
	parts := strings.Split(spec, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := inv.Hosts[p]; !ok {
			return nil, fmt.Errorf("host not found: %s", p)
		}
		result = append(result, p)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no hosts resolved from: %s", spec)
	}
	return result, nil
}

// resolveTagSpec parses "@web,@db" or "@all" into tag names.
func resolveTagSpec(spec string) []string {
	spec = strings.TrimPrefix(spec, "@")
	parts := strings.Split(spec, ",")
	var tags []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.TrimPrefix(p, "@"))
		if p != "" {
			tags = append(tags, p)
		}
	}
	return tags
}

// hostsForTags returns all host aliases matching any of the given tags.
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

// evaluateWhen evaluates a simple condition string.
// Supported forms:
//   - "always" / "" -> true
//   - "never" -> false
//   - "host: <alias>" -> true if current host matches
//   - "tag: <tag>" -> true if current host has the tag
func evaluateWhen(condition string, alias string, hostTags []string) bool {
	condition = strings.TrimSpace(condition)
	if condition == "" || condition == "always" {
		return true
	}
	if condition == "never" {
		return false
	}

	if key, val, ok := strings.Cut(condition, ":"); ok {
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "host":
			return val == alias
		case "tag":
			for _, t := range hostTags {
				if t == val {
					return true
				}
			}
			return false
		}
	}
	// Unknown condition: treat as always
	return true
}

// evaluateFailedWhen checks if the step result matches a failure condition.
func evaluateFailedWhen(condition string, result model.CommandResult, execErr error) bool {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return false
	}
	switch condition {
	case "nonzero":
		return execErr != nil || result.ExitCode != 0
	case "always":
		return true
	case "never":
		return false
	}
	// Check for "exit_code: <N>" pattern
	if key, val, ok := strings.Cut(condition, ":"); ok {
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "exit_code" {
			var code int
			if _, err := fmt.Sscanf(val, "%d", &code); err == nil {
				return result.ExitCode == code
			}
		}
	}
	return false
}

// Run executes a playbook against resolved hosts.
func Run(
	pb *Playbook,
	inv model.Inventory,
	cfg model.Config,
	svc executor.Service,
	opts Options,
) (*PlaybookResult, error) {
	if err := Validate(pb); err != nil {
		return nil, err
	}

_aliases, err := resolveHosts(inv, pb.Hosts)
	if err != nil {
		return nil, err
	}

	result := &PlaybookResult{
		Name:  pb.Name,
		Hosts: _aliases,
	}

	for _, step := range pb.Steps {
		if step.Retries <= 0 {
			step.Retries = 1
		}

		for _, alias := range _aliases {
			resolved, err := hosts.Resolve(inv, cfg, alias)
			if err != nil {
				return nil, fmt.Errorf("resolve host %s: %w", alias, err)
			}

			// Evaluate when condition
			if !evaluateWhen(step.When, alias, resolved.Tags) {
				result.Results = append(result.Results, StepResult{
					Alias:    alias,
					StepName: step.Name,
					Exec:     step.Exec,
					Skipped:  true,
				})
				result.Skipped++
				continue
			}

			if opts.DryRun {
				fmt.Fprintf(os.Stderr, "[dry-run] %s: %s -> %s\n", step.Name, alias, step.Exec)
				result.Results = append(result.Results, StepResult{
					Alias:    alias,
					StepName: step.Name,
					Exec:     step.Exec,
					Skipped:  true,
				})
				continue
			}

			command := step.Exec
			if step.Sudo {
				command = fmt.Sprintf("sudo -S sh -c %s", shellQuote(command))
			}

			var lastResult model.CommandResult
			var lastErr error
			for attempt := 0; attempt < step.Retries; attempt++ {
				if attempt > 0 {
					fmt.Fprintf(os.Stderr, "[retry] %s.%s attempt %d/%d on %s\n", step.Name, alias, attempt+1, step.Retries, alias)
					time.Sleep(time.Duration(step.Delay) * time.Second)
				}

				ctx := context.Background()
				lastResult, lastErr = svc.Exec(ctx, model.ExecRequest{
					Alias:        alias,
					Command:      command,
					ResolvedHost: resolved,
				})

				// Check failed_when condition
				if evaluateFailedWhen(step.FailedWhen, lastResult, lastErr) {
					break
				}

				// If no error and exit code 0, success
				if lastErr == nil && lastResult.ExitCode == 0 {
					break
				}
			}

			sr := StepResult{
				Alias:    alias,
				StepName: step.Name,
				Exec:     step.Exec,
				Stdout:   lastResult.Stdout,
				Stderr:   lastResult.Stderr,
				ExitCode: lastResult.ExitCode,
			}

			isFailed := lastErr != nil || lastResult.ExitCode != 0
			if evaluateFailedWhen(step.FailedWhen, lastResult, lastErr) {
				isFailed = true
			}

			if isFailed && !step.IgnoreErrors {
				sr.Err = lastErr
				result.Results = append(result.Results, sr)
				result.Failed = true
				result.Errors++
				return result, nil
			}

			if isFailed && step.IgnoreErrors {
				sr.Err = lastErr
				result.Results = append(result.Results, sr)
				continue
			}

			result.Results = append(result.Results, sr)
			if lastResult.ExitCode == 0 {
				result.Changed++
			}
		}
	}

	return result, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\"'\"'`) + "'"
}
