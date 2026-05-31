package validate

import "testing"

func TestParseTarget(t *testing.T) {
	host, port, err := ParseTarget("192.168.1.102:8080")
	if err != nil {
		t.Fatal(err)
	}
	if host != "192.168.1.102" || port != 8080 {
		t.Fatalf("unexpected target %s:%d", host, port)
	}
}
