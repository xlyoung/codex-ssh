package audit

import (
	"bufio"
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codex-ssh-skill/pkg/model"
)

type Logger struct {
	dir string
}

func NewLogger(dir string) Logger {
	return Logger{dir: dir}
}

func (l Logger) Append(event model.AuditEvent) error {
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return err
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.EventID == "" {
		event.EventID = event.Timestamp.Format("20060102T150405.000000000")
	}
	path := filepath.Join(l.dir, event.Timestamp.Format("2006-01-02")+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(event)
}

func (l Logger) Query(q model.AuditQuery) ([]model.AuditEvent, error) {
	files, err := l.getLogFiles()
	if err != nil {
		return nil, err
	}

	var events []model.AuditEvent
	for _, path := range files {
		// Check if file date is within query range
		if !l.isFileRelevant(path, q.Since, q.Until) {
			continue
		}

		fileEvents, err := l.readFile(path)
		if err != nil {
			return nil, err
		}
		events = append(events, fileEvents...)
	}

	// Apply filters
	var filtered []model.AuditEvent
	for _, event := range events {
		if !match(event, q) {
			continue
		}
		// Time range filter
		if !q.Since.IsZero() && event.Timestamp.Before(q.Since) {
			continue
		}
		if !q.Until.IsZero() && event.Timestamp.After(q.Until) {
			continue
		}
		// Command substring filter
		if q.Command != "" && !strings.Contains(event.Command, q.Command) {
			continue
		}
		filtered = append(filtered, event)
	}

	if q.Limit > 0 && len(filtered) > q.Limit {
		filtered = filtered[len(filtered)-q.Limit:]
	}
	return filtered, nil
}

// readFile reads all events from a JSONL file (including gzipped)
func (l Logger) readFile(path string) ([]model.AuditEvent, error) {
	var reader io.Reader
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(file)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		reader = gz
	} else {
		reader = file
	}

	var events []model.AuditEvent
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		var event model.AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue // skip malformed lines
		}
		events = append(events, event)
	}
	return events, scanner.Err()
}

// isFileRelevant checks if a log file might contain events in the time range
func (l Logger) isFileRelevant(path string, since, until time.Time) bool {
	if since.IsZero() && until.IsZero() {
		return true
	}

	// Extract date from filename (2006-01-02.jsonl or 2006-01-02.jsonl.gz)
	base := filepath.Base(path)
	dateStr := strings.TrimSuffix(base, ".jsonl")
	dateStr = strings.TrimSuffix(dateStr, ".gz")

	fileDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return true // can't parse, include it
	}

	if !since.IsZero() && fileDate.Before(since.Truncate(24*time.Hour)) {
		return false
	}
	if !until.IsZero() && fileDate.After(until.Truncate(24*time.Hour)) {
		return false
	}
	return true
}

// getLogFiles returns all log files sorted by date
func (l Logger) getLogFiles() ([]string, error) {
	patterns := []string{
		filepath.Join(l.dir, "*.jsonl"),
		filepath.Join(l.dir, "*.jsonl.gz"),
	}

	var files []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		files = append(files, matches...)
	}
	sort.Strings(files)
	return files, nil
}

func match(event model.AuditEvent, q model.AuditQuery) bool {
	if q.HostAlias != "" && !strings.EqualFold(q.HostAlias, event.HostAlias) {
		return false
	}
	if q.Action != "" && !strings.EqualFold(q.Action, event.Action) {
		return false
	}
	if q.Status != "" && !strings.EqualFold(q.Status, event.Status) {
		return false
	}
	return true
}

// ─── Log Rotation ───

// Rotate compresses old log files (older than maxAge)
func (l Logger) Rotate(maxAge time.Duration) (int, error) {
	if maxAge == 0 {
		maxAge = 30 * 24 * time.Hour // default 30 days
	}

	cutoff := time.Now().Add(-maxAge)
	files, err := filepath.Glob(filepath.Join(l.dir, "*.jsonl"))
	if err != nil {
		return 0, err
	}

	rotated := 0
	for _, path := range files {
		// Extract date
		base := filepath.Base(path)
		dateStr := strings.TrimSuffix(base, ".jsonl")
		fileDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}

		if fileDate.After(cutoff) {
			continue // too recent
		}

		gzPath := path + ".gz"
		if err := compressFile(path, gzPath); err != nil {
			continue
		}
		os.Remove(path)
		rotated++
	}

	return rotated, nil
}

// DeleteOld removes log files older than maxAge
func (l Logger) DeleteOld(maxAge time.Duration) (int, error) {
	if maxAge == 0 {
		maxAge = 365 * 24 * time.Hour // default 1 year
	}

	cutoff := time.Now().Add(-maxAge)
	patterns := []string{
		filepath.Join(l.dir, "*.jsonl"),
		filepath.Join(l.dir, "*.jsonl.gz"),
	}

	deleted := 0
	for _, pattern := range patterns {
		files, _ := filepath.Glob(pattern)
		for _, path := range files {
			base := filepath.Base(path)
			dateStr := strings.TrimSuffix(base, ".jsonl")
			dateStr = strings.TrimSuffix(dateStr, ".gz")
			fileDate, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				continue
			}
			if fileDate.Before(cutoff) {
				os.Remove(path)
				deleted++
			}
		}
	}
	return deleted, nil
}

// ─── Export ───

// Export writes events to a file in the specified format
func (l Logger) Export(events []model.AuditEvent, format, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	switch format {
	case "json":
		return exportJSON(file, events)
	case "csv":
		return exportCSV(file, events)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func exportJSON(w io.Writer, events []model.AuditEvent) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	for _, e := range events {
		if err := encoder.Encode(e); err != nil {
			return err
		}
	}
	return nil
}

func exportCSV(w io.Writer, events []model.AuditEvent) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Header
	header := []string{"timestamp", "event_id", "action", "host_alias", "user", "command", "status", "exit_code", "duration_ms"}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, e := range events {
		row := []string{
			e.Timestamp.Format(time.RFC3339),
			e.EventID,
			e.Action,
			e.HostAlias,
			e.User,
			e.Command,
			e.Status,
			fmt.Sprintf("%d", e.ExitCode),
			fmt.Sprintf("%d", e.DurationMS),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	return nil
}

// ─── Statistics ───

type AuditStats struct {
	TotalEvents    int                    `json:"total_events"`
	Actions        map[string]int         `json:"actions"`
	Statuses       map[string]int         `json:"statuses"`
	Hosts          map[string]int         `json:"hosts"`
	Errors         int                    `json:"errors"`
	AvgDurationMS  float64                `json:"avg_duration_ms"`
	Period         string                 `json:"period"`
}

// GetStats computes statistics for a time range
func (l Logger) GetStats(since, until time.Time) (*AuditStats, error) {
	q := model.AuditQuery{Since: since, Until: until}
	events, err := l.Query(q)
	if err != nil {
		return nil, err
	}

	stats := &AuditStats{
		TotalEvents: len(events),
		Actions:     make(map[string]int),
		Statuses:    make(map[string]int),
		Hosts:       make(map[string]int),
	}

	var totalDuration int64
	for _, e := range events {
		stats.Actions[e.Action]++
		stats.Statuses[e.Status]++
		if e.HostAlias != "" {
			stats.Hosts[e.HostAlias]++
		}
		if e.Status == "error" {
			stats.Errors++
		}
		totalDuration += e.DurationMS
	}

	if len(events) > 0 {
		stats.AvgDurationMS = float64(totalDuration) / float64(len(events))
	}

	if !since.IsZero() && !until.IsZero() {
		stats.Period = fmt.Sprintf("%s to %s", since.Format("2006-01-02"), until.Format("2006-01-02"))
	} else {
		stats.Period = "all time"
	}

	return stats, nil
}

func compressFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	defer gz.Close()

	_, err = io.Copy(gz, in)
	return err
}
