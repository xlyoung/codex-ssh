package askpass

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareWritesExecutableOneTimeScriptAndEnv(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "askpass")

	prepared, err := Prepare(dir, "secret-value")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := prepared.Cleanup(); err != nil {
			t.Fatalf("cleanup failed: %v", err)
		}
	})

	if prepared.ScriptPath == "" {
		t.Fatal("expected script path")
	}
	if filepath.Dir(prepared.ScriptPath) != dir {
		t.Fatalf("expected script in %s, got %s", dir, prepared.ScriptPath)
	}
	if prepared.Env["SSH_ASKPASS"] != prepared.ScriptPath {
		t.Fatalf("expected SSH_ASKPASS=%s, got %q", prepared.ScriptPath, prepared.Env["SSH_ASKPASS"])
	}
	if prepared.Env["SSH_ASKPASS_REQUIRE"] != "force" {
		t.Fatalf("expected SSH_ASKPASS_REQUIRE=force, got %q", prepared.Env["SSH_ASKPASS_REQUIRE"])
	}
	if prepared.Env["DISPLAY"] != "dummy" {
		t.Fatalf("expected DISPLAY=dummy, got %q", prepared.Env["DISPLAY"])
	}
	if prepared.Env[askpassSecretEnvKey] != "secret-value" {
		t.Fatalf("expected %s to carry password", askpassSecretEnvKey)
	}

	info, err := os.Stat(prepared.ScriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("expected mode 0700, got %#o", got)
	}

	content, err := os.ReadFile(prepared.ScriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "secret-value") {
		t.Fatal("script must not contain password literal")
	}
}

func TestPrepareScriptExecutionPrintsPassword(t *testing.T) {
	prepared, err := Prepare(t.TempDir(), "p@$$w0rd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := prepared.Cleanup(); err != nil {
			t.Fatalf("cleanup failed: %v", err)
		}
	})

	cmd := exec.Command(prepared.ScriptPath)
	cmd.Env = os.Environ()
	for key, value := range prepared.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "p@$$w0rd\n" {
		t.Fatalf("unexpected output: %q", string(out))
	}
}

func TestCleanupRemovesScript(t *testing.T) {
	prepared, err := Prepare(t.TempDir(), "secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(prepared.ScriptPath); !os.IsNotExist(err) {
		t.Fatalf("expected script removed, stat err=%v", err)
	}
}
