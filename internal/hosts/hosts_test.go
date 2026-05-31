package hosts

import (
	"path/filepath"
	"testing"

	"codex-ssh-skill/pkg/model"
)

func TestResolveHostAppliesDefaultsAndViaChain(t *testing.T) {
	cfg := model.Config{DefaultUser: "ops", DefaultPort: 22, DefaultAuth: "agent"}
	inv := model.Inventory{
		Hosts: map[string]model.Host{
			"bastion": {Host: "10.0.0.1", User: "jump", Port: 2200},
			"app":     {Host: "10.0.1.10", Via: []string{"bastion"}},
		},
	}

	resolved, err := Resolve(inv, cfg, "app")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.User != "ops" {
		t.Fatalf("expected default user, got %s", resolved.User)
	}
	if len(resolved.Via) != 1 || resolved.Via[0].Alias != "bastion" {
		t.Fatalf("unexpected via chain: %+v", resolved.Via)
	}
	if resolved.Via[0].Port != 2200 {
		t.Fatalf("unexpected bastion port: %d", resolved.Via[0].Port)
	}
}

func TestResolveHostPreservesSecretRef(t *testing.T) {
	cfg := model.Config{DefaultUser: "ops", DefaultPort: 22, DefaultAuth: "agent"}
	inv := model.Inventory{
		Hosts: map[string]model.Host{
			"app": {Host: "10.0.1.10", SecretRef: "vault://ssh/app"},
		},
	}

	resolved, err := Resolve(inv, cfg, "app")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.SecretRef != "vault://ssh/app" {
		t.Fatalf("expected secret_ref preserved, got %q", resolved.SecretRef)
	}
}

func TestSecretRefRoundTripSaveLoadResolve(t *testing.T) {
	cfg := model.Config{DefaultUser: "ops", DefaultPort: 22, DefaultAuth: "agent"}
	inv := model.Inventory{
		Version: 1,
		Hosts: map[string]model.Host{
			"app": {Host: "10.0.1.10", SecretRef: "vault://ssh/app"},
		},
	}

	path := filepath.Join(t.TempDir(), "hosts.toml")
	if err := Save(path, inv); err != nil {
		t.Fatalf("save hosts: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load hosts: %v", err)
	}
	resolved, err := Resolve(loaded, cfg, "app")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.SecretRef != "vault://ssh/app" {
		t.Fatalf("expected secret_ref preserved after round-trip, got %q", resolved.SecretRef)
	}
}
