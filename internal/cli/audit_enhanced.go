package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"time"

	"codex-ssh-skill/internal/audit"
	"codex-ssh-skill/pkg/model"
)

func (a App) runAuditQueryEnhanced(logger audit.Logger, args []string) int {
	fs := flag.NewFlagSet("audit query", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)

	hostFilter := fs.String("host", "", "Filter by host alias")
	actionFilter := fs.String("action", "", "Filter by action")
	statusFilter := fs.String("status", "", "Filter by status")
	commandFilter := fs.String("command", "", "Filter by command substring")
	limit := fs.Int("limit", 50, "Max events to show")
	sinceStr := fs.String("since", "", "Start date (YYYY-MM-DD)")
	untilStr := fs.String("until", "", "End date (YYYY-MM-DD)")
	format := fs.String("format", "text", "Output format: json, text, or csv")
	exportPath := fs.String("export", "", "Export to file path")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	var since, until time.Time
	if *sinceStr != "" {
		since, _ = time.Parse("2006-01-02", *sinceStr)
	}
	if *untilStr != "" {
		until, _ = time.Parse("2006-01-02", *untilStr)
		until = until.Add(24*time.Hour - time.Second)
	}

	q := model.AuditQuery{
		HostAlias: *hostFilter,
		Action:    *actionFilter,
		Status:    *statusFilter,
		Command:   *commandFilter,
		Since:     since,
		Until:     until,
		Limit:     *limit,
	}

	events, err := logger.Query(q)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error querying audit log: %v\n", err)
		return 1
	}

	if len(events) == 0 {
		fmt.Fprintln(a.Stdout, "No matching audit events found.")
		return 0
	}

	// Export mode
	if *exportPath != "" {
		format := "json"
		if len(*exportPath) > 4 && (*exportPath)[len(*exportPath)-4:] == ".csv" {
			format = "csv"
		}
		if err := logger.Export(events, format, *exportPath); err != nil {
			fmt.Fprintf(a.Stderr, "Error exporting: %v\n", err)
			return 1
		}
		fmt.Fprintf(a.Stdout, "✓ Exported %d events to %s\n", len(events), *exportPath)
		return 0
	}

	// Display mode
	if *format == "json" {
		enc := json.NewEncoder(a.Stdout)
		enc.SetIndent("", "  ")
		for _, e := range events {
			enc.Encode(e)
		}
	} else if *format == "csv" {
		fmt.Fprintln(a.Stdout, "timestamp,event_id,action,host_alias,user,command,status,exit_code,duration_ms")
		for _, e := range events {
			fmt.Fprintf(a.Stdout, "%s,%s,%s,%s,%s,%q,%s,%d,%d\n",
				e.Timestamp.Format(time.RFC3339), e.EventID, e.Action,
				e.HostAlias, e.User, e.Command, e.Status, e.ExitCode, e.DurationMS)
		}
	} else {
		for _, e := range events {
			ts := e.Timestamp.Format("2006-01-02 15:04:05")
			status := "✓"
			if e.Status == "error" {
				status = "✗"
			}
			fmt.Fprintf(a.Stdout, "[%s] %s %s %s@%s %s (%dms, exit=%d)\n",
				ts, status, e.Action, e.User, e.HostAlias, e.Command, e.DurationMS, e.ExitCode)
		}
	}

	fmt.Fprintf(a.Stdout, "\n%d event(s) found.\n", len(events))
	return 0
}

func (a App) runAuditStatsEnhanced(logger audit.Logger, args []string) int {
	fs := flag.NewFlagSet("audit stats", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)

	sinceStr := fs.String("since", "", "Start date (YYYY-MM-DD)")
	untilStr := fs.String("until", "", "End date (YYYY-MM-DD)")
	outputJSON := fs.Bool("json", false, "Output in JSON format")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	var since, until time.Time
	if *sinceStr != "" {
		since, _ = time.Parse("2006-01-02", *sinceStr)
	}
	if *untilStr != "" {
		until, _ = time.Parse("2006-01-02", *untilStr)
		until = until.Add(24*time.Hour - time.Second)
	}

	stats, err := logger.GetStats(since, until)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error computing stats: %v\n", err)
		return 1
	}

	if *outputJSON {
		enc := json.NewEncoder(a.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(stats)
		return 0
	}

	fmt.Fprintf(a.Stdout, "📊 Audit Statistics (%s)\n", stats.Period)
	fmt.Fprintf(a.Stdout, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(a.Stdout, "Total events: %d\n", stats.TotalEvents)
	fmt.Fprintf(a.Stdout, "Errors: %d\n", stats.Errors)
	fmt.Fprintf(a.Stdout, "Avg duration: %.0fms\n", stats.AvgDurationMS)

	if len(stats.Actions) > 0 {
		fmt.Fprintf(a.Stdout, "\nActions:\n")
		for action, count := range stats.Actions {
			fmt.Fprintf(a.Stdout, "  %-20s %d\n", action, count)
		}
	}

	if len(stats.Hosts) > 0 {
		fmt.Fprintf(a.Stdout, "\nHosts:\n")
		for host, count := range stats.Hosts {
			fmt.Fprintf(a.Stdout, "  %-20s %d\n", host, count)
		}
	}

	return 0
}

func (a App) runAuditRotateEnhanced(logger audit.Logger, args []string) int {
	fs := flag.NewFlagSet("audit rotate", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)

	maxAgeDays := fs.Int("max-age", 30, "Compress logs older than N days")
	deleteDays := fs.Int("delete-age", 365, "Delete logs older than N days (0 to skip)")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Compress old logs
	rotated, err := logger.Rotate(time.Duration(*maxAgeDays) * 24 * time.Hour)
	if err != nil {
		fmt.Fprintf(a.Stderr, "Error rotating logs: %v\n", err)
		return 1
	}

	// Delete very old logs
	deleted := 0
	if *deleteDays > 0 {
		deleted, _ = logger.DeleteOld(time.Duration(*deleteDays) * 24 * time.Hour)
	}

	fmt.Fprintf(a.Stdout, "✓ Rotation complete: %d compressed, %d deleted\n", rotated, deleted)
	return 0
}
