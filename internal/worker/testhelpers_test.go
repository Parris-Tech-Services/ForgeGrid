package worker

import (
	"os"
	"testing"
)

// setSandboxedDataDir points every home/app-data environment variable
// getWorkerDataDir() might consult - XDG_DATA_HOME on POSIX,
// LOCALAPPDATA/APPDATA on Windows - at a per-test temp directory, via
// t.Setenv so it is automatically restored. A test that only sets
// HOME/USERPROFILE silently does nothing on Windows, since
// getWorkerDataDir() never reads those there: that gap is what let Windows
// CI runs for several tests in this package quietly read and write the
// real CI runner profile directory instead of an isolated t.TempDir(),
// including cross-test pollution when more than one test's transaction
// file landed in that same real, shared location.
func setSandboxedDataDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

// fakeLifecycle is a Lifecycle stub for tests that exercise the update
// transaction state machine (swap/backup/rollback/state transitions)
// without needing to actually spawn a real OS process.
type fakeLifecycle struct {
	mode    string
	startFn func(tx *UpdateTransaction) error
}

func (f *fakeLifecycle) Mode() string { return f.mode }

func (f *fakeLifecycle) Start(tx *UpdateTransaction) error {
	if f.startFn != nil {
		return f.startFn(tx)
	}
	return nil
}

// useFakeLifecycle swaps GetLifecycle for the duration of the test so any
// tx.LifecycleMode resolves to a stub whose Start() returns startErr
// (nil for success) instead of executing tx.OldBinaryPath/NewBinaryPath as
// a real subprocess. Restored automatically via t.Cleanup.
func useFakeLifecycle(t *testing.T, startErr error) {
	t.Helper()
	original := GetLifecycle
	GetLifecycle = func(mode string) Lifecycle {
		return &fakeLifecycle{mode: mode, startFn: func(*UpdateTransaction) error { return startErr }}
	}
	t.Cleanup(func() { GetLifecycle = original })
}

// useFakeLifecycleCounting is useFakeLifecycle plus a call counter, for
// tests that need to assert exactly how many times Start() was invoked
// (e.g. proving a concurrency fix stopped a duplicate restart).
func useFakeLifecycleCounting(t *testing.T, calls *int, startErr error) {
	t.Helper()
	original := GetLifecycle
	GetLifecycle = func(mode string) Lifecycle {
		return &fakeLifecycle{mode: mode, startFn: func(*UpdateTransaction) error {
			*calls++
			return startErr
		}}
	}
	t.Cleanup(func() { GetLifecycle = original })
}

// writeAndHash writes data to path and returns its hex SHA-256, for tests
// that need to construct a transaction whose OldSHA256/ExpectedSHA256
// matches a specific on-disk file.
func writeAndHash(t *testing.T, path string, data []byte) string {
	t.Helper()
	if err := os.WriteFile(path, data, 0755); err != nil {
		t.Fatalf("writeAndHash: %v", err)
	}
	h, err := fileSHA256(path)
	if err != nil {
		t.Fatalf("writeAndHash: %v", err)
	}
	return h
}
