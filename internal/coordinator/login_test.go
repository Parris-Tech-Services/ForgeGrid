package coordinator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgegrid/internal/store"
)

func TestWriteDashboardLoginFile(t *testing.T) {
	s, err := store.NewStore(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	c := &Coordinator{Store: s}

	path, err := c.writeDashboardLoginFile("https://10.1.2.3:8443", "secret-token")
	if err != nil {
		t.Fatalf("write login file: %v", err)
	}
	if got, want := path, filepath.Join(s.Dir(), "dashboard-login.txt"); got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read login file: %v", err)
	}
	text := string(body)
	for _, want := range []string{
		"ForgeGrid Dashboard Login",
		"URL: https://10.1.2.3:8443",
		"Username: admin",
		"Password: secret-token",
		"Keep this file private.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("login file missing %q:\n%s", want, text)
		}
	}

	if !privateFileReadable(path) {
		t.Fatalf("dashboard login must be accessible only by its owner and privileged system accounts")
	}
}
