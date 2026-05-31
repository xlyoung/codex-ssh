package sshconfig

import (
	"strings"
	"testing"
)

func TestParseImportsCommonFields(t *testing.T) {
	content := `
Host 192.168.1.100
  HostName 192.168.1.100
  User root

Host app-171
  HostName 192.168.1.101
  User appuser
  Port 22
  ProxyJump 192.168.1.100
  IdentityFile ~/.ssh/id_ed25519
`

	inv, err := Parse(strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}

	if inv.Hosts["192.168.1.100"].Host != "192.168.1.100" {
		t.Fatalf("unexpected bastion host: %+v", inv.Hosts["192.168.1.100"])
	}
	app := inv.Hosts["app-171"]
	if app.Host != "192.168.1.101" || app.User != "appuser" || app.Port != 22 {
		t.Fatalf("unexpected app host: %+v", app)
	}
	if len(app.Via) != 1 || app.Via[0] != "192.168.1.100" {
		t.Fatalf("unexpected proxy jump: %+v", app.Via)
	}
	if app.IdentityFile == "" {
		t.Fatalf("expected identity file to be imported: %+v", app)
	}
}
