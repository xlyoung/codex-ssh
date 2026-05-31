package sshargs

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codex-ssh-skill/pkg/model"
)

func TestBuildExecArgsIncludesJumpAndKeepalive(t *testing.T) {
	cfg := model.Config{
		RunDir:                   "/tmp/codex-ssh/run",
		DefaultKeepaliveInterval: 30,
		DefaultKeepaliveCountMax: 3,
		DefaultConnectTimeout:    10,
		DefaultControlMaster:     "auto",
		DefaultControlPersist:    "10m",
		Security:                 model.Security{StrictHostKeyChecking: true},
	}
	host := model.ResolvedHost{
		Alias: "app",
		Host:  "10.0.1.10",
		User:  "deploy",
		Port:  22,
		Via:   []model.ResolvedHost{{Alias: "bastion", Host: "10.0.0.1", User: "jump", Port: 22}},
	}
	args := BuildExecArgs(cfg, host, model.ExecRequest{Command: "uname -a"})
	joined := strings.Join(args, " ")
	for _, token := range []string{"-J", "jump@10.0.0.1:22", "ServerAliveInterval=30"} {
		if !strings.Contains(joined, token) {
			t.Fatalf("missing token %q in %s", token, joined)
		}
	}
}

func TestBuildExecArgsEnablesPasswordAuthWhenConfigured(t *testing.T) {
	cfg := model.Config{
		RunDir:                   "/tmp/codex-ssh/run",
		DefaultKeepaliveInterval: 30,
		DefaultKeepaliveCountMax: 3,
		DefaultConnectTimeout:    10,
		DefaultControlMaster:     "auto",
		DefaultControlPersist:    "10m",
		Security: model.Security{
			StrictHostKeyChecking: true,
			AllowPasswordAuth:     true,
		},
	}
	host := model.ResolvedHost{
		Alias: "app",
		Host:  "10.0.1.10",
		User:  "deploy",
		Port:  22,
		Auth:  "password",
	}
	args := BuildExecArgs(cfg, host, model.ExecRequest{Command: "uname -a"})
	joined := strings.Join(args, " ")
	for _, token := range []string{
		"BatchMode=no",
		"PasswordAuthentication=yes",
		"KbdInteractiveAuthentication=yes",
		"PreferredAuthentications=publickey,keyboard-interactive,password",
	} {
		if !strings.Contains(joined, token) {
			t.Fatalf("missing token %q in %s", token, joined)
		}
	}
}

func TestBuildExecArgsUsesStableControlSocketPath(t *testing.T) {
	// Use a shorter temp directory path to avoid Unix socket path length limits
	runDir := filepath.Join(os.TempDir(), "css-test2")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(runDir)

	cfg := model.Config{
		RunDir:                   runDir,
		DefaultKeepaliveInterval: 30,
		DefaultKeepaliveCountMax: 3,
		DefaultConnectTimeout:    10,
		DefaultControlMaster:     "auto",
		DefaultControlPersist:    "10m",
		Security:                 model.Security{StrictHostKeyChecking: true},
	}
	host := model.ResolvedHost{
		Alias: "app",
		Host:  "10.0.1.10",
		User:  "deploy",
		Port:  22,
	}

	firstArgs := BuildExecArgs(cfg, host, model.ExecRequest{Command: "uname -a"})
	firstJoined := strings.Join(firstArgs, " ")
	if strings.Contains(firstJoined, "%C") {
		t.Fatalf("expected concrete control socket path, got %s", firstJoined)
	}
	controlPath := optionValue(firstArgs, "ControlPath")
	if controlPath == "" {
		t.Fatalf("expected control path option in %v", firstArgs)
	}
	if !strings.HasPrefix(controlPath, filepath.Join(runDir, "control")+string(os.PathSeparator)) {
		t.Fatalf("expected control path under run dir, got %s", controlPath)
	}
	if !strings.Contains(firstJoined, "ControlMaster=auto") {
		t.Fatalf("expected auto master on first connection, got %s", firstJoined)
	}

	if err := os.MkdirAll(filepath.Dir(controlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a Unix socket to simulate an existing control socket
	listener, err := net.Listen("unix", controlPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	secondArgs := BuildExecArgs(cfg, host, model.ExecRequest{Command: "uname -a"})
	secondJoined := strings.Join(secondArgs, " ")
	if !strings.Contains(secondJoined, "ControlMaster=no") {
		t.Fatalf("expected existing control socket to disable master creation, got %s", secondJoined)
	}
	if strings.Contains(secondJoined, "ControlPersist=10m") {
		t.Fatalf("expected no control persist option when reusing socket, got %s", secondJoined)
	}
}

func TestBuildExecArgsIgnoresStaleControlPathFile(t *testing.T) {
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
	host := model.ResolvedHost{
		Alias: "app",
		Host:  "10.0.1.10",
		User:  "deploy",
		Port:  22,
	}

	args := BuildExecArgs(cfg, host, model.ExecRequest{Command: "uname -a"})
	controlPath := optionValue(args, "ControlPath")
	if controlPath == "" {
		t.Fatalf("expected control path option in %v", args)
	}
	if err := os.MkdirAll(filepath.Dir(controlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(controlPath, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	rebuiltArgs := BuildExecArgs(cfg, host, model.ExecRequest{Command: "uname -a"})
	joined := strings.Join(rebuiltArgs, " ")
	if !strings.Contains(joined, "ControlMaster=auto") {
		t.Fatalf("expected stale non-socket file to force fresh master, got %s", joined)
	}
	if strings.Contains(joined, "ControlMaster=no") {
		t.Fatalf("expected stale non-socket file not to be reused, got %s", joined)
	}
}

func TestBuildExecArgsReusesLiveControlSocket(t *testing.T) {
	// Use a shorter temp directory path to avoid Unix socket path length limits
	runDir := filepath.Join(os.TempDir(), "css-test")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(runDir)

	cfg := model.Config{
		RunDir:                   runDir,
		DefaultKeepaliveInterval: 30,
		DefaultKeepaliveCountMax: 3,
		DefaultConnectTimeout:    10,
		DefaultControlMaster:     "auto",
		DefaultControlPersist:    "10m",
		Security:                 model.Security{StrictHostKeyChecking: true},
	}
	host := model.ResolvedHost{
		Alias: "app",
		Host:  "10.0.1.10",
		User:  "deploy",
		Port:  22,
	}

	args := BuildExecArgs(cfg, host, model.ExecRequest{Command: "uname -a"})
	controlPath := optionValue(args, "ControlPath")
	if controlPath == "" {
		t.Fatalf("expected control path option in %v", args)
	}
	if err := os.MkdirAll(filepath.Dir(controlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", controlPath)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	defer listener.Close()

	rebuiltArgs := BuildExecArgs(cfg, host, model.ExecRequest{Command: "uname -a"})
	joined := strings.Join(rebuiltArgs, " ")
	if !strings.Contains(joined, "ControlMaster=no") {
		t.Fatalf("expected live control socket to be reused, got %s", joined)
	}
}

func optionValue(args []string, key string) string {
	prefix := key + "="
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-o" && strings.HasPrefix(args[i+1], prefix) {
			return strings.TrimPrefix(args[i+1], prefix)
		}
	}
	return ""
}
