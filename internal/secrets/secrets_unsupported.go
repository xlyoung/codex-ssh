//go:build !darwin

package secrets

func newStore() Store {
	return newUnavailableStore()
}
