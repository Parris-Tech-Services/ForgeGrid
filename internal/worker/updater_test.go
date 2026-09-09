package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUpdaterTransaction(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("USERPROFILE", tmp)
	os.Setenv("HOME", tmp)

	tx := &UpdateTransaction{
		ID:              "tx-123",
		CurrentState:    "STAGED",
		ExpectedSHA256:  "abcd",
		LifecycleMode:   "portable",
		RestartDeadline: time.Now().Add(60 * time.Second),
	}

	// Create mock updates dir
	os.MkdirAll(filepath.Join(getWorkerDataDir(), "updates"), 0755)

	err := writeTx(tx)
	if err != nil {
		t.Fatalf("Failed to write tx: %v", err)
	}

	read, err := readTx()
	if err != nil || read.ID != "tx-123" {
		t.Fatalf("Failed to read tx: %v", err)
	}
}

func TestUpdaterCleanup(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("USERPROFILE", tmp)
	os.Setenv("HOME", tmp)

	w := &Worker{}
	updateDir := filepath.Join(getWorkerDataDir(), "updates")
	os.MkdirAll(updateDir, 0755)

	// Create dummy helper
	helperPath := filepath.Join(updateDir, "updater-helper-forgegrid")
	os.WriteFile(helperPath, []byte("fake"), 0755)

	// No active tx
	w.cleanupUpdateFiles()

	if _, err := os.Stat(helperPath); !os.IsNotExist(err) {
		t.Fatalf("Helper was not cleaned up")
	}
}

func TestRecoveryStates(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("USERPROFILE", tmp)
	os.Setenv("HOME", tmp)

	updateDir := filepath.Join(getWorkerDataDir(), "updates")
	os.MkdirAll(updateDir, 0755)

	primary := filepath.Join(tmp, "primary.exe")
	backup := filepath.Join(tmp, "previous-primary.exe")
	candidate := buildTestExecutable(t, tmp, 0)

	if err := os.WriteFile(primary, []byte("primary"), 0755); err != nil {
		t.Fatal(err)
	}
	backupExecutable := buildTestExecutable(t, tmp, 0)
	backupBytes, err := os.ReadFile(backupExecutable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, backupBytes, 0755); err != nil {
		t.Fatal(err)
	}

	tx := &UpdateTransaction{
		ID:               "tx-123",
		CurrentState:     "STAGED",
		OldBinaryPath:    primary,
		NewBinaryPath:    candidate,
		BackupBinaryPath: backup,
		LifecycleMode:    "portable",
		RestartDeadline:  time.Now().Add(60 * time.Second),
	}

	err = swapBinaries(tx)
	if err != nil {
		t.Fatalf("Failed to swap: %v", err)
	}

	if _, err := os.Stat(primary); err != nil {
		t.Fatalf("Primary candidate is missing: %v", err)
	}

	tx.RollbackReason = "test"
	rollback(tx) // tests rollback idempotence

	if _, err := os.Stat(primary); err != nil {
		t.Fatalf("restored primary is missing: %v", err)
	}
}

func TestRollbackReportsFailureInsteadOfImplyingRecovery(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("USERPROFILE", tmp)
	os.Setenv("HOME", tmp)

	updateDir := filepath.Join(getWorkerDataDir(), "updates")
	os.MkdirAll(updateDir, 0755)

	primary := filepath.Join(tmp, "primary.exe")
	os.WriteFile(primary, []byte("candidate-that-failed-health-check"), 0755)

	tx := &UpdateTransaction{
		ID:               "tx-456",
		CurrentState:     "VERIFYING_NEW_WORKER",
		OldBinaryPath:    primary,
		BackupBinaryPath: filepath.Join(tmp, "does-not-exist-backup.exe"),
		LifecycleMode:    "portable",
		RestartDeadline:  time.Now().Add(60 * time.Second),
	}

	tx.RollbackReason = "health check failed"
	rollback(tx)

	if tx.CurrentState != "ROLLBACK_FAILED" {
		t.Fatalf("expected CurrentState to honestly report ROLLBACK_FAILED when the backup is missing, got %q", tx.CurrentState)
	}
	if !strings.Contains(tx.RollbackReason, "restore from backup failed") {
		t.Fatalf("expected RollbackReason to explain the restore failure, got %q", tx.RollbackReason)
	}
	// The primary must be left untouched (still the failed candidate) rather
	// than silently deleted, since we could not put a verified binary back.
	b, err := os.ReadFile(primary)
	if err != nil || string(b) != "candidate-that-failed-health-check" {
		t.Fatalf("primary binary was modified despite a failed restore: content=%q err=%v", b, err)
	}
}
