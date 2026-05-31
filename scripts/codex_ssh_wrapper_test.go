package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWrapperRebuildsWhenBinaryContentIsStale(t *testing.T) {
	repoDir := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoDir, "cmd", "codex-ssh"))
	mustWriteFile(t, filepath.Join(repoDir, "go.mod"), "module example.com/codex-ssh-wrapper-test\n\ngo 1.22\n")
	mustWriteFile(t, filepath.Join(repoDir, "cmd", "codex-ssh", "main.go"), `package main

import "fmt"

func main() {
	fmt.Println("fresh-build")
}
`)

	buildDir := filepath.Join(repoDir, ".build")
	mustMkdirAll(t, buildDir)
	staleBinary := filepath.Join(buildDir, "codex-ssh")
	mustWriteFile(t, staleBinary, "#!/bin/sh\necho stale-build\n")
	if err := os.Chmod(staleBinary, 0o755); err != nil {
		t.Fatalf("chmod stale binary: %v", err)
	}

	older := time.Unix(1_700_000_000, 0)
	newer := older.Add(2 * time.Hour)
	for _, path := range []string{
		filepath.Join(repoDir, "go.mod"),
		filepath.Join(repoDir, "cmd", "codex-ssh", "main.go"),
	} {
		if err := os.Chtimes(path, older, older); err != nil {
			t.Fatalf("chtimes source %s: %v", path, err)
		}
	}
	if err := os.Chtimes(staleBinary, newer, newer); err != nil {
		t.Fatalf("chtimes stale binary: %v", err)
	}

	// Use the actual wrapper script path from the repository
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate current test file")
	}
	wrapperPath := filepath.Join(filepath.Dir(thisFile), "codex-ssh.sh")

	cmd := exec.Command("bash", wrapperPath, "--help")
	cmd.Env = append(os.Environ(),
		"CODEX_SSH_REPO="+repoDir,
		"CODEX_SSH_BIN_DIR="+buildDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("wrapper failed: %v\n%s", err, output)
	}

	text := string(output)
	if strings.Contains(text, "stale-build") {
		t.Fatalf("wrapper reused stale binary output: %s", text)
	}
	if !strings.Contains(text, "fresh-build") {
		t.Fatalf("wrapper did not rebuild binary, got: %s", text)
	}
}

func TestWrapperBuildsOutsideRepoByDefault(t *testing.T) {
	homeDir := t.TempDir()
	repoParent := filepath.Join(t.TempDir(), "repo with spaces")
	repoDir := filepath.Join(repoParent, "codex-ssh")
	mustMkdirAll(t, filepath.Join(repoDir, "cmd", "codex-ssh"))
	mustWriteFile(t, filepath.Join(repoDir, "go.mod"), "module example.com/codex-ssh-wrapper-test\n\ngo 1.22\n")
	mustWriteFile(t, filepath.Join(repoDir, "cmd", "codex-ssh", "main.go"), `package main

func main() {}
`)

	// Use the actual wrapper script path from the repository
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate current test file")
	}
	wrapperPath := filepath.Join(filepath.Dir(thisFile), "codex-ssh.sh")

	cmd := exec.Command("bash", wrapperPath, "--help")
	cmd.Env = append(os.Environ(),
		"HOME="+homeDir,
		"CODEX_SSH_REPO="+repoDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("wrapper failed: %v\n%s", err, output)
	}

	if _, err := os.Stat(filepath.Join(repoDir, ".build", "codex-ssh")); !os.IsNotExist(err) {
		t.Fatalf("expected no repo-local binary, got err=%v", err)
	}

	cacheRoot := filepath.Join(homeDir, ".codex", "ssh", "build-cache")
	matches, err := filepath.Glob(filepath.Join(cacheRoot, "*", "codex-ssh"))
	if err != nil {
		t.Fatalf("glob cache binaries: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one cached binary, got %d (%v)", len(matches), matches)
	}
}

func TestWrapperHelpMentionsDoctor(t *testing.T) {
	repoDir := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoDir, "cmd", "codex-ssh"))
	mustWriteFile(t, filepath.Join(repoDir, "go.mod"), "module example.com/codex-ssh-wrapper-test\n\ngo 1.22\n")
	mustWriteFile(t, filepath.Join(repoDir, "cmd", "codex-ssh", "main.go"), `package main

import "fmt"

func main() {
	fmt.Println("fake-help")
}
`)

	// Use the actual wrapper script path from the repository
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate current test file")
	}
	wrapperPath := filepath.Join(filepath.Dir(thisFile), "codex-ssh.sh")

	cmd := exec.Command("bash", wrapperPath, "--help")
	cmd.Env = append(os.Environ(),
		"CODEX_SSH_REPO="+repoDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("wrapper failed: %v\n%s", err, output)
	}

	text := string(output)
	if !strings.Contains(text, "codex-ssh doctor [<alias>]") {
		t.Fatalf("expected help to mention doctor command, got: %s", text)
	}
}

func TestWrapperDoctorReportsLocalState(t *testing.T) {
	homeDir := t.TempDir()
	repoDir := filepath.Join(t.TempDir(), "repo with spaces", "codex-ssh")
	mustMkdirAll(t, filepath.Join(repoDir, "cmd", "codex-ssh"))
	mustWriteFile(t, filepath.Join(repoDir, "go.mod"), "module example.com/codex-ssh-wrapper-test\n\ngo 1.22\n")
	mustWriteFile(t, filepath.Join(repoDir, "cmd", "codex-ssh", "main.go"), `package main

func main() {}
`)

	// Use the actual wrapper script path from the repository
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate current test file")
	}
	wrapperPath := filepath.Join(filepath.Dir(thisFile), "codex-ssh.sh")

	cmd := exec.Command("bash", wrapperPath, "doctor")
	cmd.Env = append(os.Environ(),
		"HOME="+homeDir,
		"CODEX_SSH_REPO="+repoDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("wrapper doctor failed: %v\n%s", err, output)
	}

	text := string(output)
	for _, want := range []string{
		"codex-ssh wrapper doctor",
		"repo_dir=" + repoDir,
		"bin_path=" + filepath.Join(homeDir, ".codex", "ssh", "build-cache"),
		"stamp_status=match",
		"repo_in_icloud=no",
		"next_step: use",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected doctor output to contain %q, got: %s", want, text)
		}
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
