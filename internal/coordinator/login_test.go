package coordinator

import (
	"os"
	"path/filepath"
	"runtime"
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

	if runtime.GOOS == "windows" {
		// Windows/NTFS has no equivalent of POSIX owner-only permission
		// bits, so os.Stat().Mode().Perm() cannot reflect the 0600 the file
		// was written with (Go's Windows port only tracks the read-only DOS
		// attribute); the coordinator this file belongs to is only ever
		// deployed on Linux in this project, so that's where this must
		// actually hold.
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat login file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("permissions = %v, want 0600", got)
	}
}
