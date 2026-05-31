package tunnel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"codex-ssh-skill/internal/audit"
	"codex-ssh-skill/internal/executor"
	iruntime "codex-ssh-skill/internal/runtime"
	"codex-ssh-skill/internal/sshargs"
	"codex-ssh-skill/internal/validate"
	"codex-ssh-skill/pkg/model"
)

func Start(ctx context.Context, runner executor.Runner, logger audit.Logger, cfg model.Config, runDir string, req model.TunnelRequest) (model.ProcessState, error) {
	if req.LocalHost == "" {
		req.LocalHost = "127.0.0.1"
	}
	if err := validate.EnsurePortAvailable(req.LocalHost, req.LocalPort); err != nil {
		return model.ProcessState{}, err
	}

	if req.ID == "" {
		req.ID = fmt.Sprintf("tun_%d", time.Now().UnixNano())
	}

	args := sshargs.BuildTunnelArgs(cfg, req.ResolvedHost, req)
	state := model.ProcessState{
		ID:         req.ID,
		Kind:       "tunnel",
		Alias:      req.Alias,
		LocalHost:  req.LocalHost,
		LocalPort:  req.LocalPort,
		TargetHost: req.TargetHost,
		TargetPort: req.TargetPort,
		CreatedAt:  time.Now(),
		LogPath:    filepath.Join(runDir, req.ID+".log"),
	}

	if req.Background {
		pid, err := runner.Start(ctx, "ssh", args, state.LogPath)
		if err != nil {
			return model.ProcessState{}, err
		}
		state.PID = pid
		if err := iruntime.SaveState(filepath.Join(runDir, req.ID+".json"), state); err != nil {
			return model.ProcessState{}, err
		}
		_ = logger.Append(model.AuditEvent{
			Action:       "tunnel",
			HostAlias:    req.ResolvedHost.Alias,
			ResolvedHost: req.ResolvedHost.Host,
			User:         req.ResolvedHost.User,
			Port:         req.ResolvedHost.Port,
			Status:       "started",
			LocalHost:    state.LocalHost,
			LocalPort:    state.LocalPort,
			TargetHost:   state.TargetHost,
			TargetPort:   state.TargetPort,
			PID:          pid,
			Background:   true,
		})
		return state, nil
	}

	_, err := runner.Run(ctx, "ssh", args, false, req.AuthEnv)
	return state, err
}

func List(runDir string) ([]model.ProcessState, error) {
	paths, err := iruntime.ListStatePaths(runDir)
	if err != nil {
		return nil, err
	}
	states := make([]model.ProcessState, 0, len(paths))
	for _, path := range paths {
		state, err := iruntime.LoadState[model.ProcessState](path)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}

func Stop(runDir string, id string) error {
	path := filepath.Join(runDir, id+".json")
	state, err := iruntime.LoadState[model.ProcessState](path)
	if err != nil {
		return err
	}
	if state.PID > 0 {
		if err := syscall.Kill(-state.PID, syscall.SIGTERM); err != nil {
			process, findErr := os.FindProcess(state.PID)
			if findErr != nil {
				return err
			}
			if signalErr := process.Signal(syscall.SIGTERM); signalErr != nil {
				return err
			}
		}
	}
	return iruntime.RemoveState(path)
}
