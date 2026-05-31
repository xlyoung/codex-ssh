package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codex-ssh-skill/pkg/model"
)

func TestAppendAndQueryEvents(t *testing.T) {
	dir := t.TempDir()
	logger := NewLogger(dir)
	event := model.AuditEvent{Action: "exec", HostAlias: "app", Status: "success"}
	if err := logger.Append(event); err != nil {
		t.Fatal(err)
	}
	events, err := logger.Query(model.AuditQuery{HostAlias: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestAppendNeverSerializesAskpassSecretTokens(t *testing.T) {
	dir := t.TempDir()
	logger := NewLogger(dir)
	event := model.AuditEvent{
		Action:  "exec",
		Command: "hostname",
		Status:  "success",
	}
	if err := logger.Append(event); err != nil {
		t.Fatal(err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 jsonl file, got %d", len(files))
	}

	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range []string{"CODEX_SSH_ASKPASS_SECRET", "SSH_ASKPASS", "pw-audit"} {
		if strings.Contains(text, token) {
			t.Fatalf("audit payload unexpectedly contains token %q: %s", token, text)
		}
	}
}

func TestQueryHandlesLargeAuditLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-05-27.jsonl")
	payload := model.AuditEvent{
		Action:    "exec",
		HostAlias: "app-165",
		Status:    "success",
		Command:   strings.Repeat("x", 128*1024),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	logger := NewLogger(dir)
	events, err := logger.Query(model.AuditQuery{HostAlias: "app-165"})
	if err != nil {
		t.Fatalf("expected large audit line to be queryable, got %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}
