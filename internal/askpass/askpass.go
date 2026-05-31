package askpass

import (
	"fmt"
	"os"
)

const askpassSecretEnvKey = "CODEX_SSH_ASKPASS_SECRET"

type Prepared struct {
	ScriptPath string
	Env        map[string]string
	Cleanup    func() error
}

func Prepare(dir string, password string) (Prepared, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Prepared{}, fmt.Errorf("create askpass dir: %w", err)
	}

	file, err := os.CreateTemp(dir, "askpass-*")
	if err != nil {
		return Prepared{}, fmt.Errorf("create askpass script: %w", err)
	}
	path := file.Name()

	script := "#!/bin/sh\nprintf '%s\\n' \"${" + askpassSecretEnvKey + "}\"\n"
	if _, err := file.WriteString(script); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return Prepared{}, fmt.Errorf("write askpass script: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return Prepared{}, fmt.Errorf("close askpass script: %w", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		_ = os.Remove(path)
		return Prepared{}, fmt.Errorf("chmod askpass script: %w", err)
	}

	cleanup := func() error {
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return Prepared{
		ScriptPath: path,
		Env: map[string]string{
			"SSH_ASKPASS":         path,
			"SSH_ASKPASS_REQUIRE": "force",
			"DISPLAY":             "dummy",
			askpassSecretEnvKey:   password,
		},
		Cleanup: cleanup,
	}, nil
}
