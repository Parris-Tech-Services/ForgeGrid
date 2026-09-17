package research

import (
	"context"
	"net"
	"testing"
)

func TestValidateURLRejectsUnsafeSchemesPortsAndUserinfo(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "javascript:alert(1)", "data:text/plain,x", "http://x:8080/", "http://user:pass@example.com/"} {
		if u, err := validateURL(raw); err == nil || u != nil {
			t.Errorf("validateURL(%q) accepted unsafe URL", raw)
		}
	}
}

func TestBlockedIPRanges(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.245.173.1", "192.168.50.2", "169.254.169.254", "100.64.0.1", "198.18.0.1", "::1", "fc00::1", "fe80::1", "::ffff:10.0.0.1"} {
		if !blockedIP(net.ParseIP(raw)) {
			t.Errorf("blockedIP(%q) = false", raw)
		}
	}
}

func TestFetchRejectsPrivateDestinationBeforeRequest(t *testing.T) {
	_, err := Fetch(context.Background(), "http://127.0.0.1/")
	if err == nil {
		t.Fatal("private destination unexpectedly fetched")
	}
}
