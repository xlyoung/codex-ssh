package cli

import (
	"flag"
	"fmt"

	"codex-ssh-skill/internal/health"
	"codex-ssh-skill/pkg/model"
)

func (a App) runHealth(paths model.Paths, cfg model.Config, inv model.Inventory, args []string) int {
	fs := flag.NewFlagSet("health", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)

	tagFilter := fs.String("tag", "", "Only check hosts with this tag")
	outputJSON := fs.Bool("json", false, "Output in JSON format")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Collect hosts to check
	var hostsToCheck []string
	for hostName := range inv.Hosts {
		h := inv.Hosts[hostName]
		if *tagFilter != "" {
			found := false
			for _, t := range h.Tags {
				if t == *tagFilter {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		hostsToCheck = append(hostsToCheck, hostName)
	}

	if len(hostsToCheck) == 0 {
		fmt.Fprintln(a.Stderr, "No hosts found to check")
		return 1
	}

	fmt.Fprintf(a.Stdout, "🏥 Health checking %d host(s)...\n\n", len(hostsToCheck))

	healthy := 0
	degraded := 0
	critical := 0

	for _, hostName := range hostsToCheck {
		report := health.CheckHost(hostName)

		if *outputJSON {
			fmt.Fprintln(a.Stdout, health.FormatJSON(report))
		} else {
			fmt.Fprint(a.Stdout, health.FormatTable(report))
			fmt.Fprintln(a.Stdout)
		}

		switch report.Status {
		case health.Healthy:
			healthy++
		case health.Degraded:
			degraded++
		case health.Critical:
			critical++
		}
	}

	// Summary
	fmt.Fprintln(a.Stdout, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintf(a.Stdout, "📊 Summary: ✅ %d healthy, ⚠️ %d degraded, ❌ %d critical\n", healthy, degraded, critical)

	if critical > 0 {
		return 1
	}
	return 0
}
