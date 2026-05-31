package jobs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codex-ssh-skill/internal/audit"
	iruntime "codex-ssh-skill/internal/runtime"
	"codex-ssh-skill/pkg/model"
)

func TestBuildTmuxRunCommand(t *testing.T) {
	cmd := BuildTmuxRunCommand("codex_job_1", "/srv/app", "bash deploy.sh")
	if !strings.Contains(cmd, "tmux new-session -d -s codex_job_1") {
		t.Fatalf("unexpected cmd: %s", cmd)
	}
}

type fakeRunner struct {
	results []model.CommandResult
	errors  []error
	args    [][]string
	envs    []map[string]string
	onRun   func(call int, args []string)
}

func (f *fakeRunner) Run(_ context.Context, _ string, args []string, _ bool, env map[string]string) (model.CommandResult, error) {
	f.args = append(f.args, append([]string(nil), args...))
	call := len(f.args)
	if f.onRun != nil {
		f.onRun(call, args)
	}
	copied := make(map[string]string, len(env))
	for k, v := range env {
		copied[k] = v
	}
	f.envs = append(f.envs, copied)
	if len(f.results) == 0 {
		return model.CommandResult{}, nil
	}
	result := f.results[0]
	f.results = f.results[1:]
	var err error
	if len(f.errors) > 0 {
		err = f.errors[0]
		f.errors = f.errors[1:]
	}
	return result, err
}

func (f *fakeRunner) Start(context.Context, string, []string, string) (int, error) {
	return 0, nil
}

func TestStatusReturnsTrimmedRunningState(t *testing.T) {
	runner := &fakeRunner{results: []model.CommandResult{{Stdout: "running\n", ExitCode: 0}}}
	service := Service{
		Runner:  runner,
		Logger:  audit.NewLogger(t.TempDir()),
		Config:  model.Config{RunDir: "/tmp/codex-ssh/run"},
		JobsDir: t.TempDir(),
	}
	state := model.JobState{ID: "job_1", Alias: "app", Mode: "tmux", SessionName: "codex_job_1"}
	if err := saveJobState(filepath.Join(service.JobsDir, "job_1.json"), state); err != nil {
		t.Fatal(err)
	}
	output, err := service.Status(context.Background(), model.JobRequest{
		ID: "job_1",
		ResolvedHost: model.ResolvedHost{
			Alias: "app",
			Host:  "10.0.0.10",
			User:  "ops",
			Port:  22,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if output != "running" {
		t.Fatalf("expected trimmed status, got %q", output)
	}
}

func TestAttachRejectsNonTmuxJobs(t *testing.T) {
	service := Service{JobsDir: t.TempDir()}
	state := model.JobState{ID: "job_2", Alias: "app", Mode: "nohup"}
	if err := saveJobState(filepath.Join(service.JobsDir, "job_2.json"), state); err != nil {
		t.Fatal(err)
	}
	err := service.Attach(context.Background(), model.JobRequest{ID: "job_2"})
	if err == nil || !strings.Contains(err.Error(), "not attachable") {
		t.Fatalf("expected non-tmux attach error, got %v", err)
	}
}

func TestLogsRejectsZeroLines(t *testing.T) {
	service := Service{JobsDir: t.TempDir()}
	state := model.JobState{ID: "job_3", Alias: "app", Mode: "nohup", RemoteLogFile: "/tmp/job_3.out"}
	if err := saveJobState(filepath.Join(service.JobsDir, "job_3.json"), state); err != nil {
		t.Fatal(err)
	}
	_, err := service.Logs(context.Background(), model.JobRequest{ID: "job_3"}, 0)
	if err == nil || !strings.Contains(err.Error(), "must be greater than 0") {
		t.Fatalf("expected line validation error, got %v", err)
	}
}

func TestRunFormatsErrorWithoutNilWrapWhenExitCodeNonZero(t *testing.T) {
	runner := &fakeRunner{
		results: []model.CommandResult{{ExitCode: 23, Stderr: "permission denied"}},
	}
	service := Service{
		Runner:  runner,
		Logger:  audit.NewLogger(t.TempDir()),
		Config:  model.Config{RunDir: "/tmp/codex-ssh/run"},
		JobsDir: t.TempDir(),
	}

	_, err := service.Run(context.Background(), model.JobRequest{
		ID:      "job_err",
		Command: "echo hi",
		Mode:    "tmux",
		ResolvedHost: model.ResolvedHost{
			Alias: "app",
			Host:  "10.0.0.10",
			User:  "ops",
			Port:  22,
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "%!w(<nil>)") {
		t.Fatalf("unexpected nil wrap marker: %v", err)
	}
	if !strings.Contains(err.Error(), "start job") {
		t.Fatalf("expected start job prefix, got: %v", err)
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected stderr detail, got: %v", err)
	}
}

func TestRunPropagatesAuthEnvToModeProbeAndLaunch(t *testing.T) {
	runner := &fakeRunner{
		results: []model.CommandResult{
			{ExitCode: 1},
			{ExitCode: 0},
		},
	}
	service := Service{
		Runner:  runner,
		Logger:  audit.NewLogger(t.TempDir()),
		Config:  model.Config{RunDir: "/tmp/codex-ssh/run"},
		JobsDir: t.TempDir(),
	}

	_, err := service.Run(context.Background(), model.JobRequest{
		ID:      "job_auth_env",
		Command: "echo hi",
		Mode:    "auto",
		AuthEnv: map[string]string{
			"SSH_ASKPASS_REQUIRE":      "force",
			"CODEX_SSH_ASKPASS_SECRET": "pw-job",
		},
		ResolvedHost: model.ResolvedHost{
			Alias: "app",
			Host:  "10.0.0.10",
			User:  "ops",
			Port:  22,
			Auth:  "password",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.envs) != 2 {
		t.Fatalf("expected 2 ssh invocations, got %d", len(runner.envs))
	}
	for i, env := range runner.envs {
		if env["SSH_ASKPASS_REQUIRE"] != "force" || env["CODEX_SSH_ASKPASS_SECRET"] != "pw-job" {
			t.Fatalf("call %d missing auth env: %+v", i, env)
		}
	}
}

func TestRunDoesNotExecuteRemoteWhenInitialStateSaveFails(t *testing.T) {
	runner := &fakeRunner{}
	jobsDir := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(jobsDir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := Service{
		Runner:  runner,
		Logger:  audit.NewLogger(t.TempDir()),
		Config:  model.Config{RunDir: "/tmp/codex-ssh/run"},
		JobsDir: jobsDir,
	}

	_, err := service.Run(context.Background(), model.JobRequest{
		ID:      "job_prewrite_fail",
		Command: "echo hi",
		Mode:    "auto",
		ResolvedHost: model.ResolvedHost{
			Alias: "app",
			Host:  "10.0.0.10",
			User:  "ops",
			Port:  22,
		},
	})
	if err == nil {
		t.Fatal("expected prewrite failure")
	}
	if len(runner.args) != 0 {
		t.Fatalf("expected no remote call when initial state save fails, got %d", len(runner.args))
	}
}

func TestStopDoesNotExecuteRemoteWhenStoppingStateSaveFails(t *testing.T) {
	runner := &fakeRunner{}
	jobsDir := t.TempDir()
	service := Service{
		Runner:  runner,
		Logger:  audit.NewLogger(t.TempDir()),
		Config:  model.Config{RunDir: "/tmp/codex-ssh/run"},
		JobsDir: jobsDir,
	}
	statePath := filepath.Join(jobsDir, "job_stop_prewrite_fail.json")
	if err := saveJobState(statePath, model.JobState{
		ID:            "job_stop_prewrite_fail",
		Alias:         "app",
		Mode:          "nohup",
		RemotePIDFile: "/tmp/job.pid",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(statePath, 0o400); err != nil {
		t.Fatal(err)
	}

	err := service.Stop(context.Background(), model.JobRequest{
		ID: "job_stop_prewrite_fail",
		ResolvedHost: model.ResolvedHost{
			Alias: "app",
			Host:  "10.0.0.10",
			User:  "ops",
			Port:  22,
		},
	})
	if err == nil {
		t.Fatal("expected stopping prewrite failure")
	}
	if len(runner.args) != 0 {
		t.Fatalf("expected no remote stop when stopping state save fails, got %d", len(runner.args))
	}
}

func TestRunFinalSaveFailureReportsRemoteMayHaveStartedAndKeepsStartingState(t *testing.T) {
	jobsDir := t.TempDir()
	statePath := filepath.Join(jobsDir, "job_final_save_fail.json")
	runner := &fakeRunner{
		results: []model.CommandResult{{ExitCode: 0}},
		onRun: func(_ int, _ []string) {
			if err := os.Chmod(statePath, 0o400); err != nil {
				t.Fatalf("chmod state path: %v", err)
			}
		},
	}
	service := Service{
		Runner:  runner,
		Logger:  audit.NewLogger(t.TempDir()),
		Config:  model.Config{RunDir: "/tmp/codex-ssh/run"},
		JobsDir: jobsDir,
	}

	_, err := service.Run(context.Background(), model.JobRequest{
		ID:      "job_final_save_fail",
		Command: "echo hi",
		Mode:    "tmux",
		ResolvedHost: model.ResolvedHost{
			Alias: "app",
			Host:  "10.0.0.10",
			User:  "ops",
			Port:  22,
		},
	})
	if err == nil {
		t.Fatal("expected final save failure")
	}
	if !strings.Contains(err.Error(), "starting") || !strings.Contains(err.Error(), "may have started") {
		t.Fatalf("expected recovery guidance in error, got %v", err)
	}
	state, loadErr := iruntime.LoadState[model.JobState](statePath)
	if loadErr != nil {
		t.Fatalf("load state: %v", loadErr)
	}
	if state.Status != "starting" {
		t.Fatalf("expected persisted starting state, got %s", state.Status)
	}
}

func TestStopFinalSaveFailureReportsRemoteMayHaveStoppedAndKeepsStoppingState(t *testing.T) {
	jobsDir := t.TempDir()
	statePath := filepath.Join(jobsDir, "job_stop_final_save_fail.json")
	if err := saveJobState(statePath, model.JobState{
		ID:            "job_stop_final_save_fail",
		Alias:         "app",
		Mode:          "nohup",
		Status:        "started",
		RemotePIDFile: "/tmp/job.pid",
	}); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		results: []model.CommandResult{{ExitCode: 0}},
		onRun: func(_ int, _ []string) {
			if err := os.Chmod(statePath, 0o400); err != nil {
				t.Fatalf("chmod state path: %v", err)
			}
		},
	}
	service := Service{
		Runner:  runner,
		Logger:  audit.NewLogger(t.TempDir()),
		Config:  model.Config{RunDir: "/tmp/codex-ssh/run"},
		JobsDir: jobsDir,
	}

	err := service.Stop(context.Background(), model.JobRequest{
		ID: "job_stop_final_save_fail",
		ResolvedHost: model.ResolvedHost{
			Alias: "app",
			Host:  "10.0.0.10",
			User:  "ops",
			Port:  22,
		},
	})
	if err == nil {
		t.Fatal("expected final save failure")
	}
	if !strings.Contains(err.Error(), "stopping") || !strings.Contains(err.Error(), "may have stopped") {
		t.Fatalf("expected recovery guidance in error, got %v", err)
	}
	state, loadErr := iruntime.LoadState[model.JobState](statePath)
	if loadErr != nil {
		t.Fatalf("load state: %v", loadErr)
	}
	if state.Status != "stopping" {
		t.Fatalf("expected persisted stopping state, got %s", state.Status)
	}
}

func saveJobState(path string, state model.JobState) error {
	return os.WriteFile(path, []byte(`{
  "id":"`+state.ID+`",
  "alias":"`+state.Alias+`",
  "mode":"`+state.Mode+`",
  "status":"`+state.Status+`",
  "command":"`+state.Command+`",
  "cwd":"`+state.CWD+`",
  "session_name":"`+state.SessionName+`",
  "remote_pid_file":"`+state.RemotePIDFile+`",
  "remote_log_file":"`+state.RemoteLogFile+`"
}`), 0o600)
}
