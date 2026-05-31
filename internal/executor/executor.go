package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"codex-ssh-skill/internal/audit"
	"codex-ssh-skill/internal/sshargs"
	"codex-ssh-skill/pkg/model"
)

type Runner interface {
	Run(ctx context.Context, name string, args []string, interactive bool, env map[string]string) (model.CommandResult, error)
	Start(ctx context.Context, name string, args []string, logPath string) (int, error)
}

type OSRunner struct{}

type Service struct {
	Runner Runner
	Logger audit.Logger
	Config model.Config
}

func (r OSRunner) Run(ctx context.Context, name string, args []string, interactive bool, env map[string]string) (model.CommandResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = mergeCommandEnv(env)
	start := time.Now()
	result := model.CommandResult{}

	if interactive {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		result.Duration = time.Since(start)
		result.ExitCode = exitCode(err)
		return result, err
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if shouldUsePromptStdin(args) {
		cmd.Stdin = os.Stdin
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result.Duration = time.Since(start)
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.ExitCode = exitCode(err)
	return result, err
}

func (r OSRunner) Start(ctx context.Context, name string, args []string, logPath string) (int, error) {
	logWriter, err := openLogWriter(logPath)
	if err != nil {
		return 0, err
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = nil
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		logWriter.Close()
		return 0, err
	}
	if err := cmd.Process.Release(); err != nil {
		logWriter.Close()
		return 0, err
	}
	return cmd.Process.Pid, logWriter.Close()
}

func (s Service) Exec(ctx context.Context, req model.ExecRequest) (model.CommandResult, error) {
	args := sshargs.BuildExecArgs(s.Config, req.ResolvedHost, req)
	start := time.Now()
	result, err := s.runSSHWithRetry(ctx, args, false, req.AuthEnv)
	event := baseEvent("exec", req.ResolvedHost, req.Command, req.CWD)
	event.StartTime = start
	event.EndTime = time.Now()
	event.DurationMS = event.EndTime.Sub(start).Milliseconds()
	event.ExitCode = result.ExitCode
	event.StdoutBytes = len(result.Stdout)
	event.StderrBytes = len(result.Stderr)
	event.Status = statusFrom(result.ExitCode, err)
	if err != nil {
		event.ErrorMessage = err.Error()
	}
	_ = s.Logger.Append(event)
	return result, err
}

func (s Service) runSSHWithRetry(ctx context.Context, args []string, interactive bool, env map[string]string) (model.CommandResult, error) {
	result, err := s.Runner.Run(ctx, "ssh", args, interactive, env)
	if !shouldRetryAfterControlSocketFailure(result, err) {
		return result, err
	}
	if !removeControlSocket(args) {
		return result, err
	}
	return s.Runner.Run(ctx, "ssh", args, interactive, env)
}

func (s Service) Shell(ctx context.Context, req model.ShellRequest) error {
	args := sshargs.BuildShellArgs(s.Config, req.ResolvedHost, req)
	start := time.Now()
	_, err := s.Runner.Run(ctx, "ssh", args, true, req.AuthEnv)
	event := baseEvent("shell", req.ResolvedHost, "", req.CWD)
	event.StartTime = start
	event.EndTime = time.Now()
	event.DurationMS = event.EndTime.Sub(start).Milliseconds()
	event.Status = statusFrom(0, err)
	if err != nil {
		event.ErrorMessage = err.Error()
	}
	return errors.Join(err, s.Logger.Append(event))
}

func baseEvent(action string, host model.ResolvedHost, command string, cwd string) model.AuditEvent {
	return model.AuditEvent{
		Action:       action,
		HostAlias:    host.Alias,
		ResolvedHost: host.Host,
		User:         host.User,
		Port:         host.Port,
		Via:          viaAliases(host.Via),
		Command:      command,
		CWD:          cwd,
	}
}

func viaAliases(via []model.ResolvedHost) []string {
	aliases := make([]string, 0, len(via))
	for _, host := range via {
		aliases = append(aliases, host.Alias)
	}
	return aliases
}

func statusFrom(exitCode int, err error) string {
	if err == nil && exitCode == 0 {
		return "success"
	}
	return "error"
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

func openLogWriter(path string) (io.WriteCloser, error) {
	if path == "" {
		return os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	return file, nil
}

func shouldUsePromptStdin(args []string) bool {
	for _, arg := range args {
		if arg == "BatchMode=no" || arg == "PasswordAuthentication=yes" || arg == "KbdInteractiveAuthentication=yes" {
			return true
		}
	}
	return false
}

func shouldRetryAfterControlSocketFailure(result model.CommandResult, err error) bool {
	if err == nil || result.ExitCode != 255 {
		return false
	}
	stderr := strings.ToLower(strings.TrimSpace(result.Stderr))
	if stderr == "" {
		return false
	}
	return strings.Contains(stderr, "control socket connect(") ||
		strings.Contains(stderr, "mux_client_request_session: read from master failed") ||
		strings.Contains(stderr, "master socket")
}

func removeControlSocket(args []string) bool {
	path := controlPathFromArgs(args)
	if path == "" {
		return false
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}

func controlPathFromArgs(args []string) string {
	const prefix = "ControlPath="
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-o" && strings.HasPrefix(args[i+1], prefix) {
			return strings.TrimPrefix(args[i+1], prefix)
		}
	}
	return ""
}

func mergeCommandEnv(extra map[string]string) []string {
	if len(extra) == 0 {
		return nil
	}
	base := os.Environ()
	merged := make(map[string]string, len(base)+len(extra))
	for _, kv := range base {
		idx := strings.IndexByte(kv, '=')
		if idx <= 0 {
			continue
		}
		merged[kv[:idx]] = kv[idx+1:]
	}
	for key, value := range extra {
		merged[key] = value
	}

	result := make([]string, 0, len(merged))
	for key, value := range merged {
		result = append(result, key+"="+value)
	}
	return result
}
