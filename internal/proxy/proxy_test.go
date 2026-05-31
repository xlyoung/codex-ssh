package proxy

import (
	"testing"
	"time"

	"codex-ssh-skill/pkg/model"
)

func TestBuildProxyProcessState(t *testing.T) {
	state := model.ProcessState{
		ID:        "proxy_1",
		Kind:      "proxy",
		LocalHost: "127.0.0.1",
		LocalPort: 1080,
		CreatedAt: time.Now(),
	}
	if state.Kind != "proxy" || state.LocalPort != 1080 {
		t.Fatalf("unexpected state: %+v", state)
	}
}
