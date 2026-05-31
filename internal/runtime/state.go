package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func SaveState(path string, state any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func LoadState[T any](path string) (T, error) {
	var state T
	data, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}
	err = json.Unmarshal(data, &state)
	return state, err
}

func ListStatePaths(dir string) ([]string, error) {
	return filepath.Glob(filepath.Join(dir, "*.json"))
}

func RemoveState(path string) error {
	return os.Remove(path)
}
