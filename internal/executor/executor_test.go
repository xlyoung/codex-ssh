package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codex-ssh-skill/internal/audit"
	"codex-ssh-skill/internal/sshargs"
	"codex-ssh-skill/pkg/model"
)

type fakeRunner struct {
	stdout    string
	stderr    string
	exitCode  int
	startPID  int
	runCalled bool
	env       map[string]string
	results   []model.CommandResult
	errs      []error
	runCount  int
}

func (f *fakeRunner) Run(_ context.Context, _ string, _ []string, _ bool, env map[string]string) (model.CommandResult, error) {
	f.runCalled = true
	f.env = env
	if len(f.results) > 0 {
		idx := f.runCount
		if idx >= len(f.results) {
			idx = len(f.results) - 1
		}
		var err error
		if len(f.errs) > 0 {
			errIdx := f.runCount
			if errIdx >= len(f.errs) {
				errIdx = len(f.errs) - 1
			}
			err = f.errs[errIdx]
		}
		f.runCount++
		return f.results[idx], err
	}
	f.runCount++
	return model.CommandResult{Stdout: f.stdout, Stderr: f.stderr, ExitCode: f.exitCode}, nil
}

func (f *fakeRunner) Start(_ context.Context, _ string, _ []string, _ string) (int, error) {
	return f.startPID, nil
}

func TestExecRecordsAuditEvent(t *testing.T) {
	logger := audit.NewLogger(t.TempDir())
	runner := &fakeRunner{stdout: "Linux\n", exitCode: 0}
	svc := Service{
		Runner: runner,
		Logger: logger,
		Config: model.Config{
			RunDir:                   "/tmp/codex-ssh/run",
			DefaultKeepaliveInterval: 30,
			DefaultKeepaliveCountMax: 3,
			DefaultConnectTimeout:    10,
			DefaultControlMaster:     "auto",
			DefaultControlPersist:    "10m",
			Security:                 model.Security{StrictHostKeyChecking: true},
		},
	}
	req := model.ExecRequest{
		Command: "uname -a",
		ResolvedHost: model.ResolvedHost{
			Alias: "app",
			Host:  "10.0.0.10",
			User:  "ops",
			Port:  22,
		},
	}
	if _, err := svc.Exec(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	events, err := logger.Query(model.AuditQuery{Action: "exec"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Status != "success" {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestShouldUsePromptStdinForPasswordAuth(t *testing.T) {
	args := []string{"-o", "BatchMode=no", "-o", "PasswordAuthentication=yes"}
	if !shouldUsePromptStdin(args) {
		t.Fatal("expected prompt stdin for password auth")
	}
}

func TestExecPassesAuthEnvToRunner(t *testing.T) {
	logger := audit.NewLogger(t.TempDir())
	runner := &fakeRunner{}
	svc := Service{Runner: runner, Logger: logger, Config: model.Config{}}

	_, err := svc.Exec(context.Background(), model.ExecRequest{
		Command:      "hostname",
		ResolvedHost: model.ResolvedHost{Host: "192.168.1.101", User: "appuser", Port: 22},
		AuthEnv: map[string]string{
			"SSH_ASKPASS_REQUIRE": "force",
			"DISPLAY":             "dummy",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runner.env["SSH_ASKPASS_REQUIRE"] != "force" {
		t.Fatalf("expected askpass env, got %+v", runner.env)
	}
}

func TestShellPassesAuthEnvToRunner(t *testing.T) {
	logger := audit.NewLogger(t.TempDir())
	runner := &fakeRunner{}
	svc := Service{Runner: runner, Logger: logger, Config: model.Config{}}

	err := svc.Shell(context.Background(), model.ShellRequest{
		ResolvedHost: model.ResolvedHost{Host: "192.168.1.101", User: "appuser", Port: 22},
		AuthEnv: map[string]string{
			"SSH_ASKPASS_REQUIRE": "force",
			"DISPLAY":             "dummy",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runner.env["DISPLAY"] != "dummy" {
		t.Fatalf("expected display env passthrough, got %+v", runner.env)
	}
}

func TestOSRunnerRunMergesInjectedEnv(t *testing.T) {
	r := OSRunner{}
	result, err := r.Run(
		context.Background(),
		"sh",
		[]string{"-c", "printf '%s' \"$SSH_ASKPASS_REQUIRE|$DISPLAY\""},
		false,
		map[string]string{
			"SSH_ASKPASS_REQUIRE": "force",
			"DISPLAY":             "dummy",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "force|dummy" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
}

func TestExecAuditDoesNotLeakAskpassSecret(t *testing.T) {
	logDir := t.TempDir()
	logger := audit.NewLogger(logDir)
	runner := &fakeRunner{stdout: "ok\n", exitCode: 0}
	svc := Service{Runner: runner, Logger: logger, Config: model.Config{}}

	_, err := svc.Exec(context.Background(), model.ExecRequest{
		Command: "hostname",
		ResolvedHost: model.ResolvedHost{
			Host: "192.168.1.101",
			User: "appuser",
			Port: 22,
			Auth: "password",
		},
		AuthEnv: map[string]string{
			"SSH_ASKPASS_REQUIRE":      "force",
			"CODEX_SSH_ASKPASS_SECRET": "pw-audit",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	files, err := filepath.Glob(filepath.Join(logDir, "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 audit log file, got %d", len(files))
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range []string{"pw-audit", "CODEX_SSH_ASKPASS_SECRET"} {
		if strings.Contains(text, token) {
			t.Fatalf("audit log leaked secret token %q: %s", token, text)
		}
	}
}

func TestExecRetriesOnceAfterStaleControlSocketFailure(t *testing.T) {
	runDir := t.TempDir()
	cfg := model.Config{
		RunDir:                   runDir,
		DefaultKeepaliveInterval: 30,
		DefaultKeepaliveCountMax: 3,
		DefaultConnectTimeout:    10,
		DefaultControlMaster:     "auto",
		DefaultControlPersist:    "10m",
		Security:                 model.Security{StrictHostKeyChecking: true},
	}
	req := model.ExecRequest{
		Command: "hostname",
		ResolvedHost: model.ResolvedHost{
			Alias: "app",
			Host:  "192.168.1.103",
			User:  "root",
			Port:  22,
		},
	}
	controlPath := controlPathFromArgs(sshargs.BuildExecArgs(cfg, req.ResolvedHost, req))
	if controlPath == "" {
		t.Fatal("expected control path in ssh args")
	}
	if err := os.MkdirAll(filepath.Dir(controlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(controlPath, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	logger := audit.NewLogger(t.TempDir())
	runner := &fakeRunner{
		results: []model.CommandResult{
			{
				Stderr:   "Control socket connect(" + controlPath + "): Connection refused\n",
				ExitCode: 255,
			},
			{
				Stdout:   "ok\n",
				ExitCode: 0,
			},
		},
		errs: []error{
			errors.New("exit status 255"),
			nil,
		},
	}
	svc := Service{Runner: runner, Logger: logger, Config: cfg}

	result, err := svc.Exec(context.Background(), req)
	if err != nil {
		t.Fatalf("expected retry to recover, got %v", err)
	}
	if result.Stdout != "ok\n" {
		t.Fatalf("unexpected stdout after retry: %q", result.Stdout)
	}
	if runner.runCount != 2 {
		t.Fatalf("expected 2 runs, got %d", runner.runCount)
	}
	if _, err := os.Stat(controlPath); !os.IsNotExist(err) {
		t.Fatalf("expected stale control path to be removed, stat err=%v", err)
	}
}
