package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"codex-ssh-skill/pkg/model"
)

var ErrBackendUnavailable = errors.New("password secret backend is not available on this platform")
var ErrSecretNotFound = errors.New("password secret not found")

type Store interface {
	Set(ctx context.Context, ref string, password string) error
	Get(ctx context.Context, ref string) (string, error)
	Delete(ctx context.Context, ref string) error
}

type unavailableStore struct{}

func NewStore() Store {
	return newStore()
}

func newUnavailableStore() Store {
	return unavailableStore{}
}

func RefForHost(host model.ResolvedHost) string {
	if strings.TrimSpace(host.SecretRef) != "" {
		return host.SecretRef
	}
	return fmt.Sprintf("ssh://%s@%s:%d", host.User, host.Host, host.Port)
}

func IsSecretNotFound(err error) bool {
	return errors.Is(err, ErrSecretNotFound)
}

func (unavailableStore) Set(_ context.Context, _ string, _ string) error {
	return ErrBackendUnavailable
}

func (unavailableStore) Get(_ context.Context, _ string) (string, error) {
	return "", ErrBackendUnavailable
}

func (unavailableStore) Delete(_ context.Context, _ string) error {
	return ErrBackendUnavailable
}
