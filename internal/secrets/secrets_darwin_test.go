//go:build darwin

package secrets

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeCommandRunner struct {
	calls   []commandCall
	results []commandResult
}

type commandCall struct {
	ctx  context.Context
	name string
	args []string
}

type commandResult struct {
	output []byte
	err    error
}

func (f *fakeCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, commandCall{ctx: ctx, name: name, args: append([]string(nil), args...)})
	if len(f.results) == 0 {
		return nil, nil
	}
	next := f.results[0]
	f.results = f.results[1:]
	return next.output, next.err
}

func TestDarwinStoreSetBuildsSecurityCommand(t *testing.T) {
	runner := &fakeCommandRunner{}
	store := newDarwinStore(runner)
	ctx := context.WithValue(context.Background(), "k", "v")

	if err := store.Set(ctx, "ssh://appuser@192.168.1.101:22", "p@ss"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	assertSingleCall(t, runner, ctx, []string{
		"add-generic-password",
		"-U",
		"-s", serviceName,
		"-a", "ssh://appuser@192.168.1.101:22",
		"-w", "p@ss",
	})
}

func TestDarwinStoreGetBuildsSecurityCommandAndReturnsPassword(t *testing.T) {
	runner := &fakeCommandRunner{
		results: []commandResult{
			{output: []byte("secret-from-keychain\n")},
		},
	}
	store := newDarwinStore(runner)
	ctx := context.WithValue(context.Background(), "k", "v")

	got, err := store.Get(ctx, "ssh://appuser@192.168.1.101:22")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "secret-from-keychain" {
		t.Fatalf("Get() = %q, want %q", got, "secret-from-keychain")
	}

	assertSingleCall(t, runner, ctx, []string{
		"find-generic-password",
		"-w",
		"-s", serviceName,
		"-a", "ssh://appuser@192.168.1.101:22",
	})
}

func TestDarwinStoreDeleteBuildsSecurityCommand(t *testing.T) {
	runner := &fakeCommandRunner{}
	store := newDarwinStore(runner)
	ctx := context.WithValue(context.Background(), "k", "v")

	if err := store.Delete(ctx, "ssh://appuser@192.168.1.101:22"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	assertSingleCall(t, runner, ctx, []string{
		"delete-generic-password",
		"-s", serviceName,
		"-a", "ssh://appuser@192.168.1.101:22",
	})
}

func TestDarwinStoreWrapsCommandError(t *testing.T) {
	baseErr := errors.New("exit status 44")
	runner := &fakeCommandRunner{
		results: []commandResult{
			{output: []byte("security: item not found"), err: baseErr},
		},
	}
	store := newDarwinStore(runner)

	err := store.Delete(context.Background(), "ssh://missing")
	if err == nil {
		t.Fatal("expected error from Delete(), got nil")
	}
	if !errors.Is(err, baseErr) {
		t.Fatalf("errors.Is(err, baseErr) = false, err=%v", err)
	}
	if got := err.Error(); !strings.Contains(got, "security delete-generic-password failed") {
		t.Fatalf("missing operation detail in error: %q", got)
	}
	if got := err.Error(); !strings.Contains(got, "security: item not found") {
		t.Fatalf("missing stderr detail in error: %q", got)
	}
}

func TestDarwinStoreGetNotFoundReturnsStableError(t *testing.T) {
	baseErr := errors.New("exit status 44")
	runner := &fakeCommandRunner{
		results: []commandResult{
			{output: []byte("security: SecKeychainSearchCopyNext: The specified item could not be found in the keychain."), err: baseErr},
		},
	}
	store := newDarwinStore(runner)

	_, err := store.Get(context.Background(), "ssh://missing")
	if err == nil {
		t.Fatal("expected error from Get(), got nil")
	}
	if !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}
	if !errors.Is(err, baseErr) {
		t.Fatalf("expected wrapped base error, got %v", err)
	}
}

func assertSingleCall(t *testing.T, runner *fakeCommandRunner, wantCtx context.Context, wantArgs []string) {
	t.Helper()
	if len(runner.calls) != 1 {
		t.Fatalf("call count = %d, want 1", len(runner.calls))
	}
	if runner.calls[0].ctx != wantCtx {
		t.Fatalf("context not forwarded to runner")
	}
	if runner.calls[0].name != "security" {
		t.Fatalf("command name = %q, want %q", runner.calls[0].name, "security")
	}
	got := runner.calls[0].args
	if len(got) != len(wantArgs) {
		t.Fatalf("args len = %d, want %d; got=%v", len(got), len(wantArgs), got)
	}
	for i := range wantArgs {
		if got[i] != wantArgs[i] {
			t.Fatalf("arg[%d] = %q, want %q; got=%v", i, got[i], wantArgs[i], got)
		}
	}
}
