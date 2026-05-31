//go:build darwin

package secrets

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const serviceName = "codex-ssh-skill"

type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

type darwinStore struct {
	runner commandRunner
}

func newStore() Store {
	return newDarwinStore(execCommandRunner{})
}

func newDarwinStore(runner commandRunner) Store {
	return &darwinStore{runner: runner}
}

func (s *darwinStore) Set(ctx context.Context, ref string, password string) error {
	_, err := s.runSecurity(ctx, "add-generic-password", "-U", "-s", serviceName, "-a", ref, "-w", password)
	return err
}

func (s *darwinStore) Get(ctx context.Context, ref string) (string, error) {
	output, err := s.runner.Run(ctx, "security", "find-generic-password", "-w", "-s", serviceName, "-a", ref)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		wrapped := formatSecurityError("find-generic-password", err, detail)
		if isSecurityItemNotFound(detail) {
			return "", errors.Join(ErrSecretNotFound, wrapped)
		}
		return "", wrapped
	}
	return strings.TrimRight(string(output), "\r\n"), nil
}

func (s *darwinStore) Delete(ctx context.Context, ref string) error {
	_, err := s.runSecurity(ctx, "delete-generic-password", "-s", serviceName, "-a", ref)
	return err
}

func (s *darwinStore) runSecurity(ctx context.Context, args ...string) ([]byte, error) {
	output, err := s.runner.Run(ctx, "security", args...)
	if err == nil {
		return output, nil
	}

	detail := strings.TrimSpace(string(output))
	return nil, formatSecurityError(args[0], err, detail)
}

func formatSecurityError(operation string, err error, detail string) error {
	if detail == "" {
		return fmt.Errorf("security %s failed: %w", operation, err)
	}
	return fmt.Errorf("security %s failed: %w: %s", operation, err, detail)
}

func isSecurityItemNotFound(detail string) bool {
	normalized := strings.ToLower(detail)
	return strings.Contains(normalized, "could not be found") || strings.Contains(normalized, "item not found")
}
