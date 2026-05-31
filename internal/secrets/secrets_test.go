package secrets

import (
	"context"
	"errors"
	"testing"

	"codex-ssh-skill/pkg/model"
)

func TestRefForHostDefaultsToSSHURL(t *testing.T) {
	host := model.ResolvedHost{
		Host: "192.168.1.101",
		User: "appuser",
		Port: 22,
	}

	if got, want := RefForHost(host), "ssh://appuser@192.168.1.101:22"; got != want {
		t.Fatalf("unexpected ref, got %q want %q", got, want)
	}
}

func TestRefForHostPrefersExplicitValue(t *testing.T) {
	host := model.ResolvedHost{
		Host:      "192.168.1.101",
		User:      "appuser",
		Port:      22,
		SecretRef: "ssh://custom/app-171",
	}

	if got, want := RefForHost(host), "ssh://custom/app-171"; got != want {
		t.Fatalf("unexpected ref, got %q want %q", got, want)
	}
}

func TestUnavailableStoreSetReturnsBackendUnavailableError(t *testing.T) {
	store := newUnavailableStore()

	err := store.Set(context.Background(), "ssh://appuser@192.168.1.101:22", "secret")
	if !errors.Is(err, ErrBackendUnavailable) {
		t.Fatalf("Set() error = %v, want %v", err, ErrBackendUnavailable)
	}
	if got, want := err.Error(), "password secret backend is not available on this platform"; got != want {
		t.Fatalf("Set() error message = %q, want %q", got, want)
	}
}

func TestUnavailableStoreGetReturnsBackendUnavailableError(t *testing.T) {
	store := newUnavailableStore()

	value, err := store.Get(context.Background(), "ssh://appuser@192.168.1.101:22")
	if value != "" {
		t.Fatalf("Get() value = %q, want empty string", value)
	}
	if !errors.Is(err, ErrBackendUnavailable) {
		t.Fatalf("Get() error = %v, want %v", err, ErrBackendUnavailable)
	}
	if got, want := err.Error(), "password secret backend is not available on this platform"; got != want {
		t.Fatalf("Get() error message = %q, want %q", got, want)
	}
}

func TestUnavailableStoreDeleteReturnsBackendUnavailableError(t *testing.T) {
	store := newUnavailableStore()

	err := store.Delete(context.Background(), "ssh://appuser@192.168.1.101:22")
	if !errors.Is(err, ErrBackendUnavailable) {
		t.Fatalf("Delete() error = %v, want %v", err, ErrBackendUnavailable)
	}
	if got, want := err.Error(), "password secret backend is not available on this platform"; got != want {
		t.Fatalf("Delete() error message = %q, want %q", got, want)
	}
}

func TestIsSecretNotFoundMatchesWrappedError(t *testing.T) {
	err := errors.New("other")
	if IsSecretNotFound(err) {
		t.Fatalf("unexpected match for generic error: %v", err)
	}

	wrapped := errors.Join(ErrSecretNotFound, errors.New("keychain miss"))
	if !IsSecretNotFound(wrapped) {
		t.Fatalf("expected IsSecretNotFound to match wrapped ErrSecretNotFound, got %v", wrapped)
	}
}
