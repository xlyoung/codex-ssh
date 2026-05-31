package tunnel

import (
	"testing"
	"time"

	"codex-ssh-skill/pkg/model"
)

func TestBuildTunnelProcessState(t *testing.T) {
	state := model.ProcessState{
		ID:         "tun_1",
		Kind:       "tunnel",
		LocalHost:  "127.0.0.1",
		LocalPort:  18080,
		TargetHost: "192.168.1.102",
		TargetPort: 8080,
		CreatedAt:  time.Now(),
	}
	if state.Kind != "tunnel" || state.LocalPort != 18080 {
		t.Fatalf("unexpected state: %+v", state)
	}
}
