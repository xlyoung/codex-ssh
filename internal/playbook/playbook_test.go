package playbook

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codex-ssh-skill/pkg/model"
)

func TestLoadValidPlaybook(t *testing.T) {
	content := `
name: Test Playbook
hosts: "@all"
steps:
  - name: Check disk
    exec: "df -h /"
  - name: Update system
    exec: "apt-get update"
    sudo: true
    retries: 3
    delay: 5
    ignore_errors: true
    when: "always"
`
	path := filepath.Join(t.TempDir(), "test.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	pb, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if pb.Name != "Test Playbook" {
		t.Errorf("Name = %q, want %q", pb.Name, "Test Playbook")
	}
	if pb.Hosts != "@all" {
		t.Errorf("Hosts = %q, want %q", pb.Hosts, "@all")
	}
	if len(pb.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(pb.Steps))
	}
	if pb.Steps[0].Name != "Check disk" {
		t.Errorf("Steps[0].Name = %q, want %q", pb.Steps[0].Name, "Check disk")
	}
	if !pb.Steps[1].Sudo {
		t.Error("Steps[1].Sudo should be true")
	}
	if pb.Steps[1].Retries != 3 {
		t.Errorf("Steps[1].Retries = %d, want 3", pb.Steps[1].Retries)
	}
	if pb.Steps[1].Delay != 5 {
		t.Errorf("Steps[1].Delay = %d, want 5", pb.Steps[1].Delay)
	}
	if !pb.Steps[1].IgnoreErrors {
		t.Error("Steps[1].IgnoreErrors should be true")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte("not: [valid: yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestValidateMissingName(t *testing.T) {
	pb := &Playbook{Hosts: "@all", Steps: []Step{{Exec: "echo hi"}}}
	err := Validate(pb)
	if err == nil || err.Error() != "playbook name is required" {
		t.Errorf("got %v, want 'playbook name is required'", err)
	}
}

func TestValidateMissingHosts(t *testing.T) {
	pb := &Playbook{Name: "test", Steps: []Step{{Exec: "echo hi"}}}
	err := Validate(pb)
	if err == nil || err.Error() != "hosts field is required" {
		t.Errorf("got %v, want 'hosts field is required'", err)
	}
}

func TestValidateNoSteps(t *testing.T) {
	pb := &Playbook{Name: "test", Hosts: "@all"}
	err := Validate(pb)
	if err == nil || err.Error() != "at least one step is required" {
		t.Errorf("got %v, want 'at least one step is required'", err)
	}
}

func TestValidateStepMissingExec(t *testing.T) {
	pb := &Playbook{
		Name:  "test",
		Hosts: "@all",
		Steps: []Step{{Name: "no exec"}},
	}
	err := Validate(pb)
	if err == nil || err.Error() != "step 1: exec is required" {
		t.Errorf("got %v, want 'step 1: exec is required'", err)
	}
}

func TestValidateNegativeRetries(t *testing.T) {
	pb := &Playbook{
		Name:  "test",
		Hosts: "@all",
		Steps: []Step{{Exec: "echo hi", Retries: -1}},
	}
	err := Validate(pb)
	if err == nil || err.Error() != "step 1: retries must be >= 0" {
		t.Errorf("got %v, want 'step 1: retries must be >= 0'", err)
	}
}

func TestEvaluateWhen(t *testing.T) {
	tests := []struct {
		cond   string
		alias  string
		tags   []string
		expect bool
	}{
		{"", "web1", nil, true},
		{"always", "web1", nil, true},
		{"never", "web1", nil, false},
		{"host: web1", "web1", nil, true},
		{"host: web1", "db1", nil, false},
		{"tag: web", "web1", []string{"web"}, true},
		{"tag: web", "db1", []string{"db"}, false},
		{"unknown: foo", "web1", nil, true},
	}
	for _, tt := range tests {
		got := evaluateWhen(tt.cond, tt.alias, tt.tags)
		if got != tt.expect {
			t.Errorf("evaluateWhen(%q, %q, %v) = %v, want %v", tt.cond, tt.alias, tt.tags, got, tt.expect)
		}
	}
}

func TestEvaluateFailedWhen(t *testing.T) {
	tests := []struct {
		cond string
		code int
		err  bool
		want bool
	}{
		{"", 0, false, false},
		{"nonzero", 1, false, true},
		{"nonzero", 0, true, true},
		{"nonzero", 0, false, false},
		{"always", 0, false, true},
		{"never", 1, false, false},
		{"exit_code: 2", 2, false, true},
		{"exit_code: 2", 1, false, false},
	}
	for _, tt := range tests {
		result := model.CommandResult{
			ExitCode: tt.code,
			Duration: time.Millisecond,
		}
		var execErr error
		if tt.err {
			execErr = fmt.Errorf("command failed")
		}
		got := evaluateFailedWhen(tt.cond, result, execErr)
		if got != tt.want {
			t.Errorf("evaluateFailedWhen(%q, code=%d, err=%v) = %v, want %v", tt.cond, tt.code, tt.err, got, tt.want)
		}
	}
}

func TestResolveTagSpec(t *testing.T) {
	got := resolveTagSpec("@web,@db")
	want := []string{"web", "db"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, v, want[i])
		}
	}

	got = resolveTagSpec("@all")
	if len(got) != 1 || got[0] != "all" {
		t.Errorf("@all resolved to %v, want [all]", got)
	}
}

func TestShellQuote(t *testing.T) {
	// shellQuote wraps in single quotes and escapes embedded single quotes
	// using the POSIX pattern: end quote, escaped single quote, start quote
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "'hello'"},
		{"it's", `'it'"'"'s'`},
		{"", "''"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
