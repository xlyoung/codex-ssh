package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"codex-ssh-skill/pkg/model"
)

const envHome = "CODEX_SSH_HOME"

func ResolvePaths() (model.Paths, error) {
	if dir := strings.TrimSpace(os.Getenv(envHome)); dir != "" {
		return buildPaths(dir), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return model.Paths{}, err
	}
	return buildPaths(filepath.Join(home, ".codex", "ssh")), nil
}

func buildPaths(dataDir string) model.Paths {
	runDir := filepath.Join(dataDir, "run")
	return model.Paths{
		DataDir:    dataDir,
		ConfigFile: filepath.Join(dataDir, "config.toml"),
		HostsFile:  filepath.Join(dataDir, "hosts.toml"),
		LogDir:     filepath.Join(dataDir, "logs"),
		RunDir:     runDir,
		ControlDir: filepath.Join(runDir, "control"),
		TunnelsDir: filepath.Join(runDir, "tunnels"),
		ProxiesDir: filepath.Join(runDir, "proxies"),
		JobsDir:    filepath.Join(runDir, "jobs"),
		AskpassDir: filepath.Join(runDir, "askpass"),
	}
}

func DefaultConfig(paths model.Paths) model.Config {
	return model.Config{
		Version:                  1,
		DataDir:                  paths.DataDir,
		LogDir:                   paths.LogDir,
		RunDir:                   paths.RunDir,
		DefaultUser:              "root",
		DefaultPort:              22,
		DefaultAuth:              "agent",
		DefaultKeepaliveInterval: 30,
		DefaultKeepaliveCountMax: 3,
		DefaultConnectTimeout:    10,
		DefaultControlMaster:     "auto",
		DefaultControlPersist:    "10m",
		DefaultTunnelTTL:         "30m",
		DefaultLongJobMode:       "tmux",
		Security: model.Security{
			StrictHostKeyChecking: true,
			ReuseSystemKnownHosts: true,
			AllowPasswordAuth:     true,
			AllowRoot:             true,
		},
		Audit: model.AuditConfig{
			Format:         "jsonl",
			CaptureStdout:  true,
			CaptureStderr:  true,
			RedactEnv:      true,
			MaxOutputBytes: 65536,
		},
	}
}

func Load(paths model.Paths) (model.Config, error) {
	cfg := DefaultConfig(paths)
	if err := EnsureDirs(paths); err != nil {
		return model.Config{}, err
	}

	if _, err := os.Stat(paths.ConfigFile); errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	} else if err != nil {
		return model.Config{}, err
	}

	if _, err := toml.DecodeFile(paths.ConfigFile, &cfg); err != nil {
		return model.Config{}, err
	}

	if cfg.DataDir == "" {
		cfg.DataDir = paths.DataDir
	}
	if cfg.LogDir == "" {
		cfg.LogDir = paths.LogDir
	}
	if cfg.RunDir == "" {
		cfg.RunDir = paths.RunDir
	}
	return cfg, nil
}

func Save(paths model.Paths, cfg model.Config) error {
	if err := EnsureDirs(paths); err != nil {
		return err
	}
	file, err := os.Create(paths.ConfigFile)
	if err != nil {
		return err
	}
	defer file.Close()
	return toml.NewEncoder(file).Encode(cfg)
}

func EnsureDirs(paths model.Paths) error {
	for _, dir := range []string{
		paths.DataDir,
		paths.LogDir,
		paths.RunDir,
		paths.ControlDir,
		paths.TunnelsDir,
		paths.ProxiesDir,
		paths.JobsDir,
		paths.AskpassDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}
