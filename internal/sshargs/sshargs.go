package sshargs

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codex-ssh-skill/pkg/model"
)

func BuildExecArgs(cfg model.Config, host model.ResolvedHost, req model.ExecRequest) []string {
	args := commonArgs(cfg, host, false)
	args = append(args, targetArg(host))
	args = append(args, remoteCommand(req.CWD, req.Command))
	return args
}

func BuildShellArgs(cfg model.Config, host model.ResolvedHost, req model.ShellRequest) []string {
	args := commonArgs(cfg, host, false)
	args = append(args, "-tt", targetArg(host))
	if req.CWD != "" {
		args = append(args, remoteCommand(req.CWD, `exec "${SHELL:-/bin/bash}" -l`))
	}
	return args
}

func BuildTunnelArgs(cfg model.Config, host model.ResolvedHost, req model.TunnelRequest) []string {
	args := commonArgs(cfg, host, true)
	args = append(args, "-N")
	args = append(args, "-L", fmt.Sprintf("%s:%d:%s:%d", req.LocalHost, req.LocalPort, req.TargetHost, req.TargetPort))
	args = append(args, targetArg(host))
	return args
}

func BuildProxyArgs(cfg model.Config, host model.ResolvedHost, req model.ProxyRequest) []string {
	args := commonArgs(cfg, host, true)
	args = append(args, "-N")
	args = append(args, "-D", fmt.Sprintf("%s:%d", req.LocalHost, req.LocalPort))
	args = append(args, targetArg(host))
	return args
}

func commonArgs(cfg model.Config, host model.ResolvedHost, forward bool) []string {
	controlPath := controlSocketPath(cfg, host)
	batchMode := "yes"
	if usesPasswordAuth(cfg, host) {
		batchMode = "no"
	}
	args := []string{
		"-o", fmt.Sprintf("BatchMode=%s", batchMode),
		"-o", fmt.Sprintf("ConnectTimeout=%d", cfg.DefaultConnectTimeout),
		"-o", fmt.Sprintf("ServerAliveInterval=%d", cfg.DefaultKeepaliveInterval),
		"-o", fmt.Sprintf("ServerAliveCountMax=%d", cfg.DefaultKeepaliveCountMax),
	}
	if controlSocketExists(controlPath) {
		args = append(args,
			"-o", "ControlMaster=no",
			"-o", fmt.Sprintf("ControlPath=%s", controlPath),
		)
	} else {
		args = append(args,
			"-o", fmt.Sprintf("ControlMaster=%s", cfg.DefaultControlMaster),
			"-o", fmt.Sprintf("ControlPersist=%s", cfg.DefaultControlPersist),
			"-o", fmt.Sprintf("ControlPath=%s", controlPath),
		)
	}

	if cfg.Security.StrictHostKeyChecking {
		args = append(args, "-o", "StrictHostKeyChecking=yes")
	} else {
		args = append(args, "-o", "StrictHostKeyChecking=no")
	}
	if usesPasswordAuth(cfg, host) {
		args = append(args,
			"-o", "PasswordAuthentication=yes",
			"-o", "KbdInteractiveAuthentication=yes",
			"-o", "PreferredAuthentications=publickey,keyboard-interactive,password",
			"-o", "NumberOfPasswordPrompts=1",
		)
	}
	if forward {
		args = append(args, "-o", "ExitOnForwardFailure=yes")
	}
	if host.Auth == "identity_file" && host.IdentityFile != "" {
		args = append(args, "-i", host.IdentityFile)
	}
	if jump := jumpArg(host); jump != "" {
		args = append(args, "-J", jump)
	}
	if host.Port != 0 {
		args = append(args, "-p", fmt.Sprintf("%d", host.Port))
	}
	return args
}

func jumpArg(host model.ResolvedHost) string {
	if len(host.Via) == 0 {
		return ""
	}

	parts := make([]string, 0, len(host.Via))
	for _, via := range host.Via {
		parts = append(parts, targetWithPort(via))
	}
	return strings.Join(parts, ",")
}

func targetArg(host model.ResolvedHost) string {
	return fmt.Sprintf("%s@%s", host.User, host.Host)
}

func targetWithPort(host model.ResolvedHost) string {
	return fmt.Sprintf("%s@%s:%d", host.User, host.Host, host.Port)
}

func remoteCommand(cwd, command string) string {
	parts := []string{}
	if cwd != "" {
		parts = append(parts, fmt.Sprintf("cd %s", shellQuote(cwd)))
	}
	if command != "" {
		parts = append(parts, command)
	}
	return fmt.Sprintf("bash -lc %s", shellQuote(strings.Join(parts, " && ")))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func usesPasswordAuth(cfg model.Config, host model.ResolvedHost) bool {
	return cfg.Security.AllowPasswordAuth && host.Auth == "password"
}

func controlSocketPath(cfg model.Config, host model.ResolvedHost) string {
	sum := sha1.Sum([]byte(controlSocketKey(host)))
	name := hex.EncodeToString(sum[:16]) + ".sock"
	return filepath.Join(cfg.RunDir, "control", name)
}

func controlSocketKey(host model.ResolvedHost) string {
	parts := []string{
		host.User,
		host.Host,
		fmt.Sprintf("%d", host.Port),
	}
	if len(host.Via) == 0 {
		return strings.Join(parts, "|")
	}
	jumps := make([]string, 0, len(host.Via))
	for _, via := range host.Via {
		jumps = append(jumps, targetWithPort(via))
	}
	return strings.Join(append(parts, strings.Join(jumps, ",")), "|")
}

func controlSocketExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSocket == 0 {
		_ = os.Remove(path)
		return false
	}

	conn, err := net.DialTimeout("unix", path, 200*time.Millisecond)
	if err != nil {
		_ = os.Remove(path)
		return false
	}
	_ = conn.Close()
	return true
}
