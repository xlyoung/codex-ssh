package audit

import (
	"bufio"
	"encoding/json"
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
	files, err := filepath.Glob(filepath.Join(l.dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	var events []model.AuditEvent
	for _, path := range files {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
		for scanner.Scan() {
			var event model.AuditEvent
			if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
				file.Close()
				return nil, err
			}
			if match(event, q) {
				events = append(events, event)
			}
		}
		if err := scanner.Err(); err != nil {
			file.Close()
			return nil, err
		}
		file.Close()
	}

	if q.Limit > 0 && len(events) > q.Limit {
		events = events[len(events)-q.Limit:]
	}
	return events, nil
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
