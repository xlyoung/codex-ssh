package jobs

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"codex-ssh-skill/internal/audit"
	"codex-ssh-skill/internal/executor"
	iruntime "codex-ssh-skill/internal/runtime"
	"codex-ssh-skill/internal/sshargs"
	"codex-ssh-skill/pkg/model"
)

type Service struct {
	Runner  executor.Runner
	Logger  audit.Logger
	Config  model.Config
	JobsDir string
}

func (s Service) Run(ctx context.Context, req model.JobRequest) (model.JobState, error) {
	if req.ID == "" {
		req.ID = fmt.Sprintf("job_%d", time.Now().UnixNano())
	}
	modeHint := req.Mode
	if modeHint == "" {
		modeHint = "auto"
	}
	hostAlias := req.ResolvedHost.Alias
	if hostAlias == "" {
		hostAlias = req.Alias
	}
	statePath := filepath.Join(s.JobsDir, req.ID+".json")
	state := model.JobState{
		ID:         req.ID,
		Alias:      hostAlias,
		Mode:       modeHint,
		Status:     "starting",
		Command:    req.Command,
		CWD:        req.CWD,
		Connection: req.ResolvedHost,
		CreatedAt:  time.Now(),
	}
	if err := iruntime.SaveState(statePath, state); err != nil {
		return model.JobState{}, err
	}

	mode, err := s.pickMode(ctx, req)
	if err != nil {
		return model.JobState{}, err
	}
	state.Mode = mode

	var remote string
	switch mode {
	case "tmux":
		state.SessionName = sanitizeSessionName(req.ID)
		remote = BuildTmuxRunCommand(state.SessionName, req.CWD, req.Command)
	case "nohup":
		state.RemotePIDFile = fmt.Sprintf("~/.codex-ssh/jobs/%s.pid", req.ID)
		state.RemoteLogFile = fmt.Sprintf("~/.codex-ssh/jobs/%s.out", req.ID)
		remote = BuildNohupRunCommand(req.CWD, req.Command, state.RemotePIDFile, state.RemoteLogFile)
	default:
		return model.JobState{}, fmt.Errorf("unsupported job mode: %s", mode)
	}

	runReq := model.ExecRequest{
		Command:      remote,
		CWD:          "",
		ResolvedHost: req.ResolvedHost,
	}
	args := sshargs.BuildExecArgs(s.Config, req.ResolvedHost, runReq)
	result, err := s.Runner.Run(ctx, "ssh", args, false, req.AuthEnv)
	if err != nil || result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if err != nil {
			if message != "" {
				return model.JobState{}, fmt.Errorf("start job: %w: %s", err, message)
			}
			return model.JobState{}, fmt.Errorf("start job: %w", err)
		}
		return model.JobState{}, fmt.Errorf("start job: %s", message)
	}

	state.Status = "started"
	if err := iruntime.SaveState(statePath, state); err != nil {
		return model.JobState{}, fmt.Errorf("start job: remote job may have started; local state remains starting: %w", err)
	}
	_ = s.Logger.Append(model.AuditEvent{
		Action:       "job",
		HostAlias:    req.ResolvedHost.Alias,
		ResolvedHost: req.ResolvedHost.Host,
		User:         req.ResolvedHost.User,
		Port:         req.ResolvedHost.Port,
		Command:      req.Command,
		CWD:          req.CWD,
		Status:       "started",
		Mode:         mode,
		JobID:        req.ID,
		SessionName:  state.SessionName,
	})
	return state, nil
}

func (s Service) Status(ctx context.Context, req model.JobRequest) (string, error) {
	state, err := iruntime.LoadState[model.JobState](filepath.Join(s.JobsDir, req.ID+".json"))
	if err != nil {
		return "", err
	}
	command := buildStatusCommand(state)
	output, err := s.runSimple(ctx, req.ResolvedHost, command, req.AuthEnv)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (s Service) Attach(ctx context.Context, req model.JobRequest) error {
	state, err := iruntime.LoadState[model.JobState](filepath.Join(s.JobsDir, req.ID+".json"))
	if err != nil {
		return err
	}
	if state.Mode != "tmux" {
		return fmt.Errorf("job %s is not attachable because mode is %s", req.ID, state.Mode)
	}
	if _, err := s.runSimple(ctx, req.ResolvedHost, fmt.Sprintf("tmux has-session -t %s", state.SessionName), req.AuthEnv); err != nil {
		return fmt.Errorf("tmux session %s is unavailable: %w", state.SessionName, err)
	}
	shellReq := model.ShellRequest{
		Alias:   req.Alias,
		CWD:     "",
		AuthEnv: req.AuthEnv,
		ResolvedHost: model.ResolvedHost{
			Alias:        req.ResolvedHost.Alias,
			Host:         req.ResolvedHost.Host,
			User:         req.ResolvedHost.User,
			Port:         req.ResolvedHost.Port,
			Via:          req.ResolvedHost.Via,
			Auth:         req.ResolvedHost.Auth,
			IdentityFile: req.ResolvedHost.IdentityFile,
		},
	}
	args := sshargs.BuildShellArgs(s.Config, shellReq.ResolvedHost, shellReq)
	args = append(args, fmt.Sprintf("tmux attach -t %s", state.SessionName))
	_, err = s.Runner.Run(ctx, "ssh", args, true, shellReq.AuthEnv)
	return err
}

func (s Service) Stop(ctx context.Context, req model.JobRequest) error {
	statePath := filepath.Join(s.JobsDir, req.ID+".json")
	state, err := iruntime.LoadState[model.JobState](statePath)
	if err != nil {
		return err
	}
	state.Status = "stopping"
	if err := iruntime.SaveState(statePath, state); err != nil {
		return err
	}
	command := buildStopCommand(state)
	if _, err := s.runSimple(ctx, req.ResolvedHost, command, req.AuthEnv); err != nil {
		return err
	}
	state.Status = "stopped"
	if err := iruntime.SaveState(statePath, state); err != nil {
		return fmt.Errorf("stop job: remote job may have stopped; local state remains stopping: %w", err)
	}
	return nil
}

func (s Service) Logs(ctx context.Context, req model.JobRequest, lines int) (string, error) {
	if lines <= 0 {
		return "", fmt.Errorf("lines must be greater than 0")
	}
	state, err := iruntime.LoadState[model.JobState](filepath.Join(s.JobsDir, req.ID+".json"))
	if err != nil {
		return "", err
	}
	command := buildLogsCommand(state, lines)
	output, err := s.runSimple(ctx, req.ResolvedHost, command, req.AuthEnv)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(output, "\n"), nil
}

func (s Service) pickMode(ctx context.Context, req model.JobRequest) (string, error) {
	mode := req.Mode
	if mode == "" || mode == "auto" {
		if ok, err := s.hasTmux(ctx, req.ResolvedHost, req.AuthEnv); err == nil && ok {
			return "tmux", nil
		}
		return "nohup", nil
	}
	return mode, nil
}

func (s Service) hasTmux(ctx context.Context, host model.ResolvedHost, authEnv map[string]string) (bool, error) {
	req := model.ExecRequest{Command: "command -v tmux >/dev/null 2>&1", ResolvedHost: host}
	args := sshargs.BuildExecArgs(s.Config, host, req)
	result, err := s.Runner.Run(ctx, "ssh", args, false, authEnv)
	if err != nil {
		return false, err
	}
	return result.ExitCode == 0, nil
}

func (s Service) runSimple(ctx context.Context, host model.ResolvedHost, command string, authEnv map[string]string) (string, error) {
	if s.Runner == nil {
		return "", fmt.Errorf("runner is not configured")
	}
	req := model.ExecRequest{Command: command, ResolvedHost: host}
	args := sshargs.BuildExecArgs(s.Config, host, req)
	result, err := s.Runner.Run(ctx, "ssh", args, false, authEnv)
	if err != nil || result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if err != nil && message != "" {
			return result.Stdout, fmt.Errorf("remote command failed: %v: %s", err, message)
		}
		if err != nil {
			return result.Stdout, fmt.Errorf("remote command failed: %v", err)
		}
		return result.Stdout, fmt.Errorf("remote command failed with exit code %d: %s", result.ExitCode, message)
	}
	return result.Stdout, nil
}

func BuildTmuxRunCommand(session, cwd, command string) string {
	if cwd != "" {
		command = fmt.Sprintf("cd %s && %s", shellQuote(cwd), command)
	}
	return fmt.Sprintf("tmux new-session -d -s %s %s", session, shellQuote(command))
}

func BuildNohupRunCommand(cwd, command, pidFile, logFile string) string {
	prefix := "mkdir -p ~/.codex-ssh/jobs && "
	if cwd != "" {
		command = fmt.Sprintf("cd %s && %s", shellQuote(cwd), command)
	}
	return prefix + fmt.Sprintf("nohup bash -lc %s > %s 2>&1 < /dev/null & echo $! > %s", shellQuote(command), logFile, pidFile)
}

func buildStatusCommand(state model.JobState) string {
	if state.Mode == "tmux" {
		return fmt.Sprintf("tmux has-session -t %s >/dev/null 2>&1 && echo running || echo stopped", state.SessionName)
	}
	return fmt.Sprintf("if [ -f %s ] && kill -0 $(cat %s) >/dev/null 2>&1; then echo running; else echo stopped; fi", state.RemotePIDFile, state.RemotePIDFile)
}

func buildStopCommand(state model.JobState) string {
	if state.Mode == "tmux" {
		return fmt.Sprintf("tmux kill-session -t %s", state.SessionName)
	}
	return fmt.Sprintf("if [ -f %s ]; then kill $(cat %s); fi", state.RemotePIDFile, state.RemotePIDFile)
}

func buildLogsCommand(state model.JobState, lines int) string {
	if state.Mode == "tmux" {
		return fmt.Sprintf("tmux capture-pane -p -S -%d -t %s", lines, state.SessionName)
	}
	return fmt.Sprintf("tail -n %d %s", lines, state.RemoteLogFile)
}

func sanitizeSessionName(id string) string {
	return strings.NewReplacer("-", "_", ".", "_").Replace("codex_" + id)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
