package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/BurntSushi/toml"

	"codex-ssh-skill/pkg/model"
)

func TestResolvePathsUsesOverride(t *testing.T) {
	t.Setenv(envHome, filepath.Join(t.TempDir(), "custom"))

	paths, err := ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}

	if paths.DataDir == "" || filepath.Base(paths.DataDir) != "custom" {
		t.Fatalf("unexpected data dir: %s", paths.DataDir)
	}
}

func TestLoadReturnsDefaultsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envHome, dir)

	paths, err := ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(paths)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.DefaultLongJobMode != "tmux" {
		t.Fatalf("unexpected long job mode: %s", cfg.DefaultLongJobMode)
	}
	if _, err := os.Stat(paths.LogDir); err != nil {
		t.Fatalf("expected log dir to exist: %v", err)
	}
}

func TestResolvePathsIncludesAskpassDir(t *testing.T) {
	t.Setenv(envHome, filepath.Join(t.TempDir(), "custom"))

	paths, err := ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}

	if paths.AskpassDir == "" {
		t.Fatal("expected askpass dir")
	}
	if got, want := paths.AskpassDir, filepath.Join(paths.RunDir, "askpass"); got != want {
		t.Fatalf("unexpected askpass dir: got %s want %s", got, want)
	}
}

func TestLoadEnsuresAskpassDirExists(t *testing.T) {
	t.Setenv(envHome, filepath.Join(t.TempDir(), "custom"))

	paths, err := ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Load(paths); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(paths.AskpassDir)
	if err != nil {
		t.Fatalf("expected askpass dir to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected askpass dir to be dir: %s", paths.AskpassDir)
	}
}

func TestDefaultConfigMatchesDefaultsTomlAllowPasswordAuth(t *testing.T) {
	paths := buildPaths(t.TempDir())
	goDefault := DefaultConfig(paths)

	var fileDefault model.Config
	if _, err := toml.DecodeFile(defaultsFilePath(t, "config.toml"), &fileDefault); err != nil {
		t.Fatalf("decode defaults/config.toml: %v", err)
	}

	if goDefault.Security.AllowPasswordAuth != fileDefault.Security.AllowPasswordAuth {
		t.Fatalf(
			"allow_password_auth mismatch: go=%v file=%v",
			goDefault.Security.AllowPasswordAuth,
			fileDefault.Security.AllowPasswordAuth,
		)
	}
	if goDefault.DefaultTunnelTTL != fileDefault.DefaultTunnelTTL {
		t.Fatalf(
			"default_tunnel_ttl mismatch: go=%q file=%q",
			goDefault.DefaultTunnelTTL,
			fileDefault.DefaultTunnelTTL,
		)
	}
}

func TestDefaultsHostsExampleTomlPreservesSecretRefForAppServer(t *testing.T) {
	var inv model.Inventory
	if _, err := toml.DecodeFile(defaultsFilePath(t, "hosts.example.toml"), &inv); err != nil {
		t.Fatalf("decode defaults/hosts.example.toml: %v", err)
	}

	host, ok := inv.Hosts["app-server"]
	if !ok {
		t.Fatal("expected hosts.app-server in defaults/hosts.example.toml")
	}

	if got, want := host.SecretRef, "ssh://appuser@app.internal.example.com:22"; got != want {
		t.Fatalf("unexpected secret_ref for app-server: got %q want %q", got, want)
	}
}

func defaultsFilePath(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate current test file")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "defaults", name)
}
