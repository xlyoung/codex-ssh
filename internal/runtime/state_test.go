package runtime

import (
	"path/filepath"
	"testing"

	"codex-ssh-skill/pkg/model"
)

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tun_1.json")
	state := model.ProcessState{ID: "tun_1", Kind: "tunnel", PID: 123}
	if err := SaveState(path, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadState[model.ProcessState](path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PID != 123 {
		t.Fatalf("unexpected pid %d", loaded.PID)
	}
}
