package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codex-ssh-skill/internal/audit"
	iruntime "codex-ssh-skill/internal/runtime"
	"codex-ssh-skill/internal/secrets"
	"codex-ssh-skill/pkg/model"
)

type fakeRunner struct {
	result         model.CommandResult
	err            error
	args           [][]string
	envs           []map[string]string
	runHasDeadline []bool
	runDeadlines   []time.Time
}

func (f *fakeRunner) Run(ctx context.Context, _ string, args []string, _ bool, env map[string]string) (model.CommandResult, error) {
	f.args = append(f.args, append([]string(nil), args...))
	copied := make(map[string]string, len(env))
	for k, v := range env {
		copied[k] = v
	}
	f.envs = append(f.envs, copied)
	deadline, ok := ctx.Deadline()
	f.runHasDeadline = append(f.runHasDeadline, ok)
	f.runDeadlines = append(f.runDeadlines, deadline)
	return f.result, f.err
}

func (f *fakeRunner) Start(context.Context, string, []string, string) (int, error) {
	return 123, nil
}

type fakeSecretStore struct {
	values map[string]string
}

func newTestApp(stdout, stderr *bytes.Buffer, runner *fakeRunner) App {
	app := New(stdout, stderr, runner)
	app.KnownHostsLookup = func(string, model.ResolvedHost) (bool, error) { return true, nil }
	return app
}

func (f *fakeSecretStore) Set(_ context.Context, ref string, password string) error {
	if f.values == nil {
		f.values = map[string]string{}
	}
	f.values[ref] = password
	return nil
}

func (f *fakeSecretStore) Get(_ context.Context, ref string) (string, error) {
	if f.values == nil {
		f.values = map[string]string{}
	}
	password, ok := f.values[ref]
	if !ok {
		return "", secrets.ErrSecretNotFound
	}
	return password, nil
}

func (f *fakeSecretStore) Delete(_ context.Context, ref string) error {
	delete(f.values, ref)
	return nil
}

func TestHostsTestFailsWhenSSHMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeHostsFile(t, home, `
version = 1

[hosts.app]
host = "192.168.1.102"
user = "deploy"
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newTestApp(stdout, stderr, &fakeRunner{})
	app.LookPath = func(string) (string, error) { return "", errors.New("not found") }

	code := app.Run([]string{"hosts", "test", "app"})
	if code == 0 || !strings.Contains(stderr.String(), "ssh") {
		t.Fatalf("expected ssh preflight error, code=%d stderr=%s", code, stderr.String())
	}
}

func TestHostsTestRejectsMissingIdentityFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeHostsFile(t, home, `
version = 1

[hosts.app]
host = "192.168.1.102"
user = "deploy"
auth = "identity_file"
identity_file = "/tmp/not-found-key"
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newTestApp(stdout, stderr, &fakeRunner{})
	app.LookPath = func(string) (string, error) { return "/usr/bin/ssh", nil }

	code := app.Run([]string{"hosts", "test", "app"})
	if code == 0 || !strings.Contains(stderr.String(), "identity file") {
		t.Fatalf("expected identity file validation error, code=%d stderr=%s", code, stderr.String())
	}
}

func TestHostsTestRejectsPasswordAuthWhenDisabled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, `
version = 1

[security]
allow_password_auth = false
`)
	writeHostsFile(t, home, `
version = 1

[hosts.app]
host = "192.168.1.101"
user = "appuser"
auth = "password"
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newTestApp(stdout, stderr, &fakeRunner{})
	app.LookPath = func(string) (string, error) { return "/usr/bin/ssh", nil }

	code := app.Run([]string{"hosts", "test", "app"})
	if code == 0 || !strings.Contains(stderr.String(), "password auth") {
		t.Fatalf("expected password auth rejection, code=%d stderr=%s", code, stderr.String())
	}
}

func TestHostsTestAllowsPasswordAuthWhenEnabled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, `
version = 1

[security]
allow_password_auth = true
`)
	writeHostsFile(t, home, `
version = 1

[hosts.app]
host = "192.168.1.101"
user = "appuser"
auth = "password"
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runner := &fakeRunner{
		result: model.CommandResult{Stdout: "__codex_ssh_test__\ntmux=no\nnohup=yes\n", ExitCode: 0},
	}
	app := newTestApp(stdout, stderr, runner)
	app.LookPath = func(string) (string, error) { return "/usr/bin/ssh", nil }
	app.SecretStore = &fakeSecretStore{values: map[string]string{
		"ssh://appuser@192.168.1.101:22": "pw-hosts-test",
	}}

	code := app.Run([]string{"hosts", "test", "app"})
	if code != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
	}
	if len(runner.args) == 0 {
		t.Fatal("expected ssh invocation")
	}
	joined := strings.Join(runner.args[0], " ")
	if !strings.Contains(joined, "PasswordAuthentication=yes") {
		t.Fatalf("expected password auth ssh args, got %s", joined)
	}
}

func TestHostsTestPrintsRemoteCapabilities(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeHostsFile(t, home, `
version = 1

[hosts.bastion]
host = "192.168.1.100"
user = "ops"

[hosts.app]
host = "192.168.1.102"
user = "deploy"
via = ["bastion"]
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newTestApp(stdout, stderr, &fakeRunner{
		result: model.CommandResult{Stdout: "__codex_ssh_test__\ntmux=yes\n", ExitCode: 0},
	})
	app.LookPath = func(string) (string, error) { return "/usr/bin/ssh", nil }

	code := app.Run([]string{"hosts", "test", "app"})
	if code != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "tmux=yes") || !strings.Contains(stdout.String(), "bastion") {
		t.Fatalf("expected detailed remote capability output, got %s", stdout.String())
	}
}

func TestDiagnoseFailureIncludesSSHStderr(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeHostsFile(t, home, `
version = 1

[hosts.app]
host = "192.168.1.101"
user = "appuser"
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newTestApp(stdout, stderr, &fakeRunner{
		result: model.CommandResult{
			ExitCode: 255,
			Stderr:   "Host key verification failed.\n",
		},
		err: errors.New("exit status 255"),
	})
	app.LookPath = func(string) (string, error) { return "/usr/bin/ssh", nil }
	app.KnownHostsLookup = func(string, model.ResolvedHost) (bool, error) { return true, nil }

	code := app.Run([]string{"diagnose", "app"})
	if code == 0 {
		t.Fatalf("expected diagnose failure, stdout=%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "diagnose failed: exit status 255") {
		t.Fatalf("expected high-level diagnose error, stderr=%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Host key verification failed.") {
		t.Fatalf("expected ssh stderr details, stderr=%s", stderr.String())
	}
}

func TestDiagnoseAcceptsAndWritesMissingHostKeyBeforeConnect(t *testing.T) {
	homeRoot := t.TempDir()
	t.Setenv("HOME", homeRoot)

	codexHome := filepath.Join(homeRoot, ".codex-ssh-test")
	t.Setenv("CODEX_SSH_HOME", codexHome)
	writeHostsFile(t, codexHome, `
version = 1

[hosts.app]
host = "192.168.1.101"
user = "appuser"
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newTestApp(stdout, stderr, &fakeRunner{
		result: model.CommandResult{
			Stdout:   "__codex_ssh_diag__\ntmux=yes\nnohup=yes\ndocker=no\nsudo=yes\n",
			ExitCode: 0,
		},
	})
	app.LookPath = func(string) (string, error) { return "/usr/bin/ssh", nil }
	app.KnownHostsLookup = func(string, model.ResolvedHost) (bool, error) { return false, nil }
	app.KnownHostsFetch = func(host model.ResolvedHost) (string, error) {
		return host.Host + " ssh-ed25519 AAAATESTKEY\n", nil
	}

	code := app.Run([]string{"diagnose", "app"})
	if code != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
	}

	knownHostsPath := filepath.Join(homeRoot, ".ssh", "known_hosts")
	data, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("expected known_hosts file to be created: %v", err)
	}
	if !strings.Contains(string(data), "192.168.1.101 ssh-ed25519 AAAATESTKEY") {
		t.Fatalf("expected host key to be appended, got %q", string(data))
	}
}

func TestHostsListPrintsBootstrapGuidanceWhenInventoryEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newTestApp(stdout, stderr, &fakeRunner{})

	code := app.Run([]string{"hosts", "list"})
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	for _, token := range []string{"inventory is empty", "hosts set", "hosts import-ssh-config", "exec --host"} {
		if !strings.Contains(output, token) {
			t.Fatalf("expected guidance token %q in output: %s", token, output)
		}
	}
}

func TestExecSupportsAdHocHostAndViaFlags(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, `
version = 1
default_user = "root"

[security]
allow_password_auth = true
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runner := &fakeRunner{
		result: model.CommandResult{Stdout: "Linux\n", ExitCode: 0},
	}
	app := newTestApp(stdout, stderr, runner)
	app.SecretStore = &fakeSecretStore{values: map[string]string{
		"ssh://appuser@192.168.1.101:22": "pw-exec",
	}}

	code := app.Run([]string{"exec", "--host", "192.168.1.101", "--user", "appuser", "--via", "192.168.1.100", "--auth", "password", "--", "uname -a"})
	if code != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
	}
	if len(runner.args) == 0 {
		t.Fatal("expected ssh invocation")
	}
	joined := strings.Join(runner.args[0], " ")
	for _, token := range []string{"appuser@192.168.1.101", "-J root@192.168.1.100:22", "PasswordAuthentication=yes"} {
		if !strings.Contains(joined, token) {
			t.Fatalf("expected token %q in %s", token, joined)
		}
	}
}

func TestHostsImportSSHConfigCreatesInventory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_SSH_HOME", filepath.Join(home, ".codex", "ssh"))
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte(strings.TrimSpace(`
Host 192.168.1.100
  HostName 192.168.1.100
  User root

Host app-171
  HostName 192.168.1.101
  User appuser
  ProxyJump 192.168.1.100
`)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newTestApp(stdout, stderr, &fakeRunner{})

	code := app.Run([]string{"hosts", "import-ssh-config"})
	if code != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(home, ".codex", "ssh", "hosts.toml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, token := range []string{"[hosts.\"192.168.1.100\"]", "[hosts.app-171]", "via = [\"192.168.1.100\"]"} {
		if !strings.Contains(content, token) {
			t.Fatalf("expected %q in hosts.toml: %s", token, content)
		}
	}
}

func TestDiagnosePrintsResolvedSummary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, `
version = 1

[security]
allow_password_auth = true
`)
	writeHostsFile(t, home, `
version = 1

[hosts.bastion]
host = "192.168.1.100"
user = "root"

[hosts.app]
host = "192.168.1.101"
user = "appuser"
auth = "password"
via = ["bastion"]
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newTestApp(stdout, stderr, &fakeRunner{
		result: model.CommandResult{
			Stdout:   "__codex_ssh_diag__\ntmux=yes\nnohup=yes\ndocker=no\nsudo=yes\n",
			ExitCode: 0,
		},
	})
	app.LookPath = func(string) (string, error) { return "/usr/bin/ssh", nil }
	app.SecretStore = &fakeSecretStore{values: map[string]string{
		"ssh://appuser@192.168.1.101:22": "pw-diagnose",
	}}

	code := app.Run([]string{"diagnose", "app"})
	if code != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	for _, token := range []string{"target=192.168.1.101", "via=bastion", "auth=password", "docker=no", "sudo=yes"} {
		if !strings.Contains(output, token) {
			t.Fatalf("expected token %q in output: %s", token, output)
		}
	}
}

func TestAuditQueryTextFormatIncludesSummaryLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	logger := audit.NewLogger(filepath.Join(home, "logs"))
	if err := logger.Append(model.AuditEvent{
		Action:       "exec",
		HostAlias:    "app",
		ResolvedHost: "192.168.1.102",
		User:         "deploy",
		Status:       "success",
		Command:      "uname -a",
	}); err != nil {
		t.Fatal(err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newTestApp(stdout, stderr, &fakeRunner{})

	code := app.Run([]string{"audit", "query", "--format", "text", "--host", "app"})
	if code != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "exec") || !strings.Contains(stdout.String(), "app") {
		t.Fatalf("expected text summary line, got %s", stdout.String())
	}
}

func TestResolveTargetPositionalPrefersInventoryHostKey(t *testing.T) {
	cfg := model.Config{DefaultUser: "root", DefaultPort: 22, DefaultAuth: "agent"}
	inv := model.Inventory{
		Hosts: map[string]model.Host{
			"bastion":     {Host: "192.168.1.100", User: "jump", Port: 2200},
			"192.168.1.101": {Host: "app.internal", User: "appuser", Port: 2022, Via: []string{"bastion"}, Auth: "password", Workdir: "/srv/app", SecretRef: "vault://ssh/app"},
		},
	}

	resolved, err := resolveTarget(cfg, inv, "192.168.1.101", targetInput{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Host != "app.internal" {
		t.Fatalf("expected inventory host to win, got %s", resolved.Host)
	}
	if resolved.Auth != "password" || resolved.Workdir != "/srv/app" || resolved.SecretRef != "vault://ssh/app" {
		t.Fatalf("expected inventory auth/workdir/secret_ref preserved, got auth=%s workdir=%s secret_ref=%s", resolved.Auth, resolved.Workdir, resolved.SecretRef)
	}
	if len(resolved.Via) != 1 || resolved.Via[0].Alias != "bastion" {
		t.Fatalf("expected inventory via chain, got %+v", resolved.Via)
	}
}

func TestResolveTargetPositionalFallsBackToBareEndpointOnInventoryMiss(t *testing.T) {
	cfg := model.Config{DefaultUser: "root", DefaultPort: 22, DefaultAuth: "agent"}
	inv := model.Inventory{Hosts: map[string]model.Host{}}

	resolved, err := resolveTarget(cfg, inv, "appuser@192.168.1.101:2222", targetInput{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Host != "192.168.1.101" || resolved.User != "appuser" || resolved.Port != 2222 {
		t.Fatalf("unexpected resolved fallback endpoint: %+v", resolved)
	}
	if len(resolved.Via) != 0 {
		t.Fatalf("fallback endpoint should not invent via, got %+v", resolved.Via)
	}
}

func TestResolveTargetPositionalFallsBackToBareIPOnInventoryMiss(t *testing.T) {
	cfg := model.Config{DefaultUser: "root", DefaultPort: 22, DefaultAuth: "agent"}
	inv := model.Inventory{Hosts: map[string]model.Host{}}

	resolved, err := resolveTarget(cfg, inv, "192.168.1.101", targetInput{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Host != "192.168.1.101" || resolved.User != "root" || resolved.Port != 22 {
		t.Fatalf("unexpected resolved bare ip endpoint: %+v", resolved)
	}
	if len(resolved.Via) != 0 {
		t.Fatalf("fallback bare ip should not invent via, got %+v", resolved.Via)
	}
}

func TestResolveTargetPositionalFallsBackToUserAtHostOnInventoryMiss(t *testing.T) {
	cfg := model.Config{DefaultUser: "root", DefaultPort: 22, DefaultAuth: "agent"}
	inv := model.Inventory{Hosts: map[string]model.Host{}}

	resolved, err := resolveTarget(cfg, inv, "appuser@192.168.1.101", targetInput{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Host != "192.168.1.101" || resolved.User != "appuser" || resolved.Port != 22 {
		t.Fatalf("unexpected resolved user@host endpoint: %+v", resolved)
	}
	if len(resolved.Via) != 0 {
		t.Fatalf("fallback user@host should not invent via, got %+v", resolved.Via)
	}
}

func TestResolveTargetAliasModeErrorMessageMatchesRejectedFlags(t *testing.T) {
	cfg := model.Config{DefaultUser: "root", DefaultPort: 22, DefaultAuth: "agent"}
	inv := model.Inventory{
		Hosts: map[string]model.Host{
			"app": {Host: "192.168.1.101"},
		},
	}

	_, err := resolveTarget(cfg, inv, "app", targetInput{
		Port:         2222,
		IdentityFile: "/tmp/id_rsa",
		Workdir:      "/srv/app",
	})
	if err == nil {
		t.Fatal("expected alias+flags conflict error")
	}
	msg := err.Error()
	for _, token := range []string{"--port", "--identity-file", "--workdir"} {
		if !strings.Contains(msg, token) {
			t.Fatalf("expected error message to mention %s, got %q", token, msg)
		}
	}
}

func TestResolveTargetHostFlagPrefersInventoryKeyAndAllowsOverrides(t *testing.T) {
	cfg := model.Config{DefaultUser: "root", DefaultPort: 22, DefaultAuth: "agent"}
	inv := model.Inventory{
		Hosts: map[string]model.Host{
			"bastion":     {Host: "192.168.1.100", User: "jump", Port: 2200},
			"192.168.1.101": {Host: "app.internal", User: "appuser", Port: 2022, Via: []string{"bastion"}, Auth: "password", Workdir: "/srv/app", SecretRef: "ssh://custom/app"},
		},
	}

	resolved, err := resolveTarget(cfg, inv, "", targetInput{
		Host: "192.168.1.101",
		User: "deploy",
		Port: 2222,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Host != "app.internal" || resolved.Auth != "password" || resolved.SecretRef != "ssh://custom/app" || resolved.Workdir != "/srv/app" {
		t.Fatalf("expected inventory baseline preserved, got %+v", resolved)
	}
	if resolved.User != "deploy" || resolved.Port != 2222 {
		t.Fatalf("expected explicit user/port overrides, got %+v", resolved)
	}
	if len(resolved.Via) != 1 || resolved.Via[0].Alias != "bastion" {
		t.Fatalf("expected inventory via chain retained, got %+v", resolved.Via)
	}
}

func TestLoadJobStateAndHostPrefersStateConnectionSnapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, "version = 1")
	writeHostsFile(t, home, `
version = 1
[hosts.app]
host = "192.168.1.104"
user = "deploy"
auth = "agent"
`)

	app := New(&bytes.Buffer{}, &bytes.Buffer{}, &fakeRunner{})
	paths, cfg, inv, _, err := app.loadContext()
	if err != nil {
		t.Fatal(err)
	}
	state := model.JobState{
		ID:    "job_snapshot",
		Alias: "app",
		Connection: model.ResolvedHost{
			Alias: "app",
			Host:  "192.168.1.101",
			User:  "appuser",
			Port:  2222,
			Auth:  "password",
		},
	}
	if err := iruntime.SaveState(filepath.Join(paths.JobsDir, "job_snapshot.json"), state); err != nil {
		t.Fatal(err)
	}

	_, host, err := app.loadJobStateAndHost(paths, cfg, inv, "job_snapshot")
	if err != nil {
		t.Fatal(err)
	}
	if host.Host != "192.168.1.101" || host.User != "appuser" || host.Port != 2222 || host.Auth != "password" {
		t.Fatalf("expected state snapshot host, got %+v", host)
	}
}

func TestSecretSetStoresPasswordWithoutPrintingIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, "version = 1")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	store := &fakeSecretStore{}
	app := newTestApp(stdout, stderr, &fakeRunner{})
	app.SecretStore = store
	app.PasswordReader = func(string) (string, error) { return "1qa2ws", nil }

	code := app.Run([]string{"secret", "set", "--host", "192.168.1.101", "--user", "appuser"})
	if code != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
	}
	if got := store.values["ssh://appuser@192.168.1.101:22"]; got != "1qa2ws" {
		t.Fatalf("unexpected stored value for default ref: %q", got)
	}
	if strings.Contains(stdout.String(), "1qa2ws") || strings.Contains(stderr.String(), "1qa2ws") {
		t.Fatalf("password leaked to output, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestSecretGetDefaultHidesPasswordAndShowPrintsPassword(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, "version = 1")

	store := &fakeSecretStore{values: map[string]string{
		"ssh://appuser@192.168.1.101:22": "p@ss",
	}}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newTestApp(stdout, stderr, &fakeRunner{})
	app.SecretStore = store

	if code := app.Run([]string{"secret", "get", "--host", "192.168.1.101", "--user", "appuser"}); code != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "p@ss") {
		t.Fatalf("password must be hidden by default, stdout=%q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"secret", "get", "--host", "192.168.1.101", "--user", "appuser", "--show"}); code != 0 {
		t.Fatalf("expected success with --show, code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "p@ss") {
		t.Fatalf("expected password output with --show, stdout=%q", stdout.String())
	}
}

func TestSecretDeleteRemovesStoredEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, "version = 1")

	store := &fakeSecretStore{values: map[string]string{
		"ssh://appuser@192.168.1.101:22": "to-delete",
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newTestApp(stdout, stderr, &fakeRunner{})
	app.SecretStore = store

	code := app.Run([]string{"secret", "delete", "--host", "192.168.1.101", "--user", "appuser"})
	if code != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
	}
	if _, ok := store.values["ssh://appuser@192.168.1.101:22"]; ok {
		t.Fatal("expected secret removed")
	}
}

func TestShellUsesStoredSecretForInventoryIPAddressKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, `
version = 1
[security]
allow_password_auth = true
`)
	writeHostsFile(t, home, `
version = 1
[hosts.bastion]
host = "192.168.1.100"
user = "root"
[hosts."192.168.1.101"]
host = "192.168.1.101"
user = "appuser"
auth = "password"
via = ["bastion"]
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runner := &fakeRunner{}
	app := newTestApp(stdout, stderr, runner)
	app.SecretStore = &fakeSecretStore{values: map[string]string{
		"ssh://appuser@192.168.1.101:22": "pw-shell",
	}}

	code := app.Run([]string{"shell", "192.168.1.101"})
	if code != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
	}
	if len(runner.envs) == 0 {
		t.Fatal("expected auth env injected")
	}
	if runner.envs[0]["SSH_ASKPASS_REQUIRE"] != "force" {
		t.Fatalf("expected askpass env, got %+v", runner.envs[0])
	}
	if runner.envs[0]["CODEX_SSH_ASKPASS_SECRET"] != "pw-shell" {
		t.Fatalf("expected askpass secret in env, got %+v", runner.envs[0])
	}
	if strings.Contains(strings.Join(runner.args[0], " "), "pw-shell") {
		t.Fatalf("password leaked into command args: %v", runner.args[0])
	}
}

func TestShellHostFlagUsesInventoryKeySecretRefViaAndAskpass(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, `
version = 1
[security]
allow_password_auth = true
`)
	writeHostsFile(t, home, `
version = 1
[hosts.bastion]
host = "192.168.1.100"
user = "root"
[hosts."192.168.1.101"]
host = "192.168.1.101"
user = "appuser"
auth = "password"
via = ["bastion"]
secret_ref = "ssh://custom/app"
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runner := &fakeRunner{}
	app := newTestApp(stdout, stderr, runner)
	app.SecretStore = &fakeSecretStore{values: map[string]string{
		"ssh://custom/app": "pw-custom-ref",
	}}

	code := app.Run([]string{"shell", "--host", "192.168.1.101"})
	if code != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
	}
	if len(runner.envs) == 0 {
		t.Fatal("expected auth env injected")
	}
	env := runner.envs[0]
	if env["SSH_ASKPASS_REQUIRE"] != "force" || env["CODEX_SSH_ASKPASS_SECRET"] != "pw-custom-ref" {
		t.Fatalf("expected askpass env from custom secret_ref, got %+v", env)
	}
	joined := strings.Join(runner.args[0], " ")
	if !strings.Contains(joined, "-J root@192.168.1.100:22") {
		t.Fatalf("expected inventory via in ssh args, got %s", joined)
	}
}

func TestApprovedUserJourneyShellIPAddressKeyUsesViaAndSecretResolution(t *testing.T) {
	cases := []struct {
		name        string
		secretRef   string
		storeRef    string
		storeSecret string
	}{
		{
			name:        "default_ref",
			storeRef:    "ssh://appuser@192.168.1.101:22",
			storeSecret: "pw-approved-default",
		},
		{
			name:        "custom_secret_ref",
			secretRef:   "ssh://custom/approved-path",
			storeRef:    "ssh://custom/approved-path",
			storeSecret: "pw-approved-custom",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("CODEX_SSH_HOME", home)
			writeConfigFile(t, home, `
version = 1
[security]
allow_password_auth = true
`)
			secretRefLine := ""
			if tc.secretRef != "" {
				secretRefLine = "secret_ref = \"" + tc.secretRef + "\"\n"
			}
			writeHostsFile(t, home, `
version = 1
[hosts."192.168.1.100"]
host = "192.168.1.100"
user = "root"
[hosts."192.168.1.101"]
host = "192.168.1.101"
user = "appuser"
auth = "password"
via = ["192.168.1.100"]
`+secretRefLine)

			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			runner := &fakeRunner{}
			app := newTestApp(stdout, stderr, runner)
			app.SecretStore = &fakeSecretStore{values: map[string]string{
				tc.storeRef: tc.storeSecret,
			}}

			code := app.Run([]string{"shell", "192.168.1.101"})
			if code != 0 {
				t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
			}
			if len(runner.args) == 0 {
				t.Fatal("expected ssh invocation")
			}
			if len(runner.envs) == 0 {
				t.Fatal("expected askpass env")
			}
			env := runner.envs[0]
			if env["SSH_ASKPASS_REQUIRE"] != "force" || env["DISPLAY"] != "dummy" || env["CODEX_SSH_ASKPASS_SECRET"] != tc.storeSecret {
				t.Fatalf("expected askpass env with stored secret, got %+v", env)
			}
			joined := strings.Join(runner.args[0], " ")
			if !strings.Contains(joined, "-J root@192.168.1.100:22") {
				t.Fatalf("expected inventory via jump in ssh args, got %s", joined)
			}
			if !strings.Contains(joined, "appuser@192.168.1.101") {
				t.Fatalf("expected positional IP to hit inventory user mapping, got %s", joined)
			}
			if strings.Contains(joined, tc.storeSecret) {
				t.Fatalf("password leaked into command args: %s", joined)
			}
		})
	}
}

func TestPasswordAuthPathsInjectAskpassEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, `
version = 1
[security]
allow_password_auth = true
`)
	writeHostsFile(t, home, `
version = 1
[hosts.app]
host = "192.168.1.101"
user = "appuser"
auth = "password"
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runner := &fakeRunner{
		result: model.CommandResult{
			Stdout: "__codex_ssh_test__\ntmux=yes\nnohup=yes\n__codex_ssh_diag__\ntmux=yes\nnohup=yes\ndocker=yes\nsudo=yes\n",
		},
	}
	app := newTestApp(stdout, stderr, runner)
	app.SecretStore = &fakeSecretStore{values: map[string]string{
		"ssh://appuser@192.168.1.101:22": "pw-all",
	}}
	app.LookPath = func(string) (string, error) { return "/usr/bin/ssh", nil }

	cases := [][]string{
		{"hosts", "test", "app"},
		{"exec", "app", "--", "echo ok"},
		{"shell", "app"},
		{"diagnose", "app"},
	}
	for _, command := range cases {
		switch command[0] {
		case "hosts":
			runner.result.Stdout = "__codex_ssh_test__\ntmux=yes\nnohup=yes\n"
		case "diagnose":
			runner.result.Stdout = "__codex_ssh_diag__\ntmux=yes\nnohup=yes\ndocker=yes\nsudo=yes\n"
		default:
			runner.result.Stdout = "ok\n"
		}
		stdout.Reset()
		stderr.Reset()
		if code := app.Run(command); code != 0 {
			t.Fatalf("expected success for %v, code=%d stderr=%s", command, code, stderr.String())
		}
	}
	if len(runner.envs) != len(cases) {
		t.Fatalf("expected %d invocations, got %d", len(cases), len(runner.envs))
	}
	for i, env := range runner.envs {
		if env["SSH_ASKPASS_REQUIRE"] != "force" || env["DISPLAY"] != "dummy" || env["CODEX_SSH_ASKPASS_SECRET"] != "pw-all" {
			t.Fatalf("case %d missing askpass env: %+v", i, env)
		}
	}
}

func TestPasswordAuthMissingSecretSuggestsNextStepForCustomSecretRefAlias(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, `
version = 1
[security]
allow_password_auth = true
`)
	writeHostsFile(t, home, `
version = 1
[hosts.app]
host = "192.168.1.101"
user = "appuser"
auth = "password"
secret_ref = "ssh://custom/app"
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newTestApp(stdout, stderr, &fakeRunner{})
	app.SecretStore = &fakeSecretStore{}

	code := app.Run([]string{"exec", "app", "--", "hostname"})
	if code == 0 {
		t.Fatalf("expected failure when secret missing, stdout=%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "codex-ssh secret set app") {
		t.Fatalf("expected alias based next-step guidance, stderr=%s", stderr.String())
	}
}

func TestJobRunUsesStoredSecretAskpassEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, `
version = 1
[security]
allow_password_auth = true
`)
	writeHostsFile(t, home, `
version = 1
[hosts.app]
host = "192.168.1.101"
user = "appuser"
auth = "password"
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runner := &fakeRunner{
		result: model.CommandResult{ExitCode: 0},
	}
	app := newTestApp(stdout, stderr, runner)
	app.SecretStore = &fakeSecretStore{values: map[string]string{
		"ssh://appuser@192.168.1.101:22": "pw-job-run",
	}}

	code := app.Run([]string{"job", "run", "app", "--", "echo ok"})
	if code != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
	}
	if len(runner.envs) != 2 {
		t.Fatalf("expected mode probe + run invocation, got %d", len(runner.envs))
	}
	for i, env := range runner.envs {
		if env["SSH_ASKPASS_REQUIRE"] != "force" || env["CODEX_SSH_ASKPASS_SECRET"] != "pw-job-run" {
			t.Fatalf("call %d missing askpass env: %+v", i, env)
		}
	}
	for _, args := range runner.args {
		if strings.Contains(strings.Join(args, " "), "pw-job-run") {
			t.Fatalf("password leaked to args: %v", args)
		}
	}
}

func TestBackgroundProxyRejectsPasswordAuthEvenWithStoredSecret(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, `
version = 1
[security]
allow_password_auth = true
`)
	writeHostsFile(t, home, `
version = 1
[hosts.app]
host = "192.168.1.101"
user = "appuser"
auth = "password"
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newTestApp(stdout, stderr, &fakeRunner{})
	app.SecretStore = &fakeSecretStore{values: map[string]string{
		"ssh://appuser@192.168.1.101:22": "pw-proxy",
	}}

	code := app.Run([]string{"proxy", "app", "--local", "18080", "--background"})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "background proxy does not support password auth") {
		t.Fatalf("expected explicit rejection message, stderr=%s", stderr.String())
	}
}

func TestBackgroundTunnelRejectsPasswordAuthEvenWithStoredSecret(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, `
version = 1
[security]
allow_password_auth = true
`)
	writeHostsFile(t, home, `
version = 1
[hosts.app]
host = "192.168.1.101"
user = "appuser"
auth = "password"
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := newTestApp(stdout, stderr, &fakeRunner{})
	app.SecretStore = &fakeSecretStore{values: map[string]string{
		"ssh://appuser@192.168.1.101:22": "pw-tunnel",
	}}

	code := app.Run([]string{"tunnel", "app", "--local", "18081", "--target", "127.0.0.1:22", "--background"})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "background tunnel does not support password auth") {
		t.Fatalf("expected explicit rejection message, stderr=%s", stderr.String())
	}
}

func TestForegroundTunnelUsesAskpassEnvWhenPasswordAuthSecretExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, `
version = 1
[security]
allow_password_auth = true
`)
	writeHostsFile(t, home, `
version = 1
[hosts.app]
host = "192.168.1.101"
user = "appuser"
auth = "password"
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runner := &fakeRunner{}
	app := newTestApp(stdout, stderr, runner)
	app.SecretStore = &fakeSecretStore{values: map[string]string{
		"ssh://appuser@192.168.1.101:22": "pw-tunnel",
	}}

	code := app.Run([]string{"tunnel", "app", "--local", "18081", "--target", "127.0.0.1:22"})
	if code != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
	}
	if len(runner.envs) != 1 {
		t.Fatalf("expected one tunnel invocation, got %d", len(runner.envs))
	}
	env := runner.envs[0]
	if env["SSH_ASKPASS_REQUIRE"] != "force" || env["CODEX_SSH_ASKPASS_SECRET"] != "pw-tunnel" {
		t.Fatalf("expected askpass env, got %+v", env)
	}
	for _, args := range runner.args {
		if strings.Contains(strings.Join(args, " "), "pw-tunnel") {
			t.Fatalf("password leaked to args: %v", args)
		}
	}
}

func TestForegroundProxyUsesAskpassEnvWhenPasswordAuthSecretExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, `
version = 1
[security]
allow_password_auth = true
`)
	writeHostsFile(t, home, `
version = 1
[hosts.app]
host = "192.168.1.101"
user = "appuser"
auth = "password"
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runner := &fakeRunner{}
	app := newTestApp(stdout, stderr, runner)
	app.SecretStore = &fakeSecretStore{values: map[string]string{
		"ssh://appuser@192.168.1.101:22": "pw-proxy",
	}}

	code := app.Run([]string{"proxy", "app", "--local", "18081"})
	if code != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
	}
	if len(runner.envs) != 1 {
		t.Fatalf("expected one proxy invocation, got %d", len(runner.envs))
	}
	env := runner.envs[0]
	if env["SSH_ASKPASS_REQUIRE"] != "force" || env["CODEX_SSH_ASKPASS_SECRET"] != "pw-proxy" {
		t.Fatalf("expected askpass env, got %+v", env)
	}
	for _, args := range runner.args {
		if strings.Contains(strings.Join(args, " "), "pw-proxy") {
			t.Fatalf("password leaked to args: %v", args)
		}
	}
}

func TestTunnelUsesDefaultTTLFromConfigWhenFlagIsUnset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, `
version = 1
default_tunnel_ttl = "30m"
`)
	writeHostsFile(t, home, `
version = 1
[hosts.app]
host = "192.168.1.101"
user = "appuser"
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runner := &fakeRunner{}
	app := newTestApp(stdout, stderr, runner)

	before := time.Now()
	code := app.Run([]string{"tunnel", "app", "--local", "18081", "--target", "127.0.0.1:22"})
	if code != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
	}
	if len(runner.runHasDeadline) != 1 || !runner.runHasDeadline[0] {
		t.Fatalf("expected tunnel context deadline, got %+v", runner.runHasDeadline)
	}
	assertDeadlineNear(t, runner.runDeadlines[0], before.Add(30*time.Minute), 3*time.Second)
}

func TestTunnelTTLZeroDisablesDefaultAutoStop(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, `
version = 1
default_tunnel_ttl = "30m"
`)
	writeHostsFile(t, home, `
version = 1
[hosts.app]
host = "192.168.1.101"
user = "appuser"
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runner := &fakeRunner{}
	app := newTestApp(stdout, stderr, runner)

	code := app.Run([]string{"tunnel", "app", "--local", "18081", "--target", "127.0.0.1:22", "--ttl", "0"})
	if code != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
	}
	if len(runner.runHasDeadline) != 1 {
		t.Fatalf("expected one tunnel invocation, got %d", len(runner.runHasDeadline))
	}
	if runner.runHasDeadline[0] {
		t.Fatalf("expected ttl=0 to disable auto-stop, got deadline %v", runner.runDeadlines[0])
	}
}

func TestTunnelTTLFlagOverridesConfigDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_SSH_HOME", home)
	writeConfigFile(t, home, `
version = 1
default_tunnel_ttl = "30m"
`)
	writeHostsFile(t, home, `
version = 1
[hosts.app]
host = "192.168.1.101"
user = "appuser"
`)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runner := &fakeRunner{}
	app := newTestApp(stdout, stderr, runner)

	before := time.Now()
	code := app.Run([]string{"tunnel", "app", "--local", "18081", "--target", "127.0.0.1:22", "--ttl", "45s"})
	if code != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", code, stderr.String())
	}
	if len(runner.runHasDeadline) != 1 || !runner.runHasDeadline[0] {
		t.Fatalf("expected tunnel context deadline, got %+v", runner.runHasDeadline)
	}
	assertDeadlineNear(t, runner.runDeadlines[0], before.Add(45*time.Second), 2*time.Second)
}

func assertDeadlineNear(t *testing.T, got, want time.Time, tolerance time.Duration) {
	t.Helper()
	diff := got.Sub(want)
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		t.Fatalf("unexpected deadline: got %s want about %s diff=%s", got, want, diff)
	}
}

func writeHostsFile(t *testing.T, home string, content string) {
	t.Helper()
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "hosts.toml"), []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeConfigFile(t *testing.T, home string, content string) {
	t.Helper()
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
