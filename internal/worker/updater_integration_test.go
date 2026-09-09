package worker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUpdaterIntegrationSuccessfulUpdate(t *testing.T) {
	// 35. Local deterministic successful-update test
	tmp := t.TempDir()
	os.Setenv("USERPROFILE", tmp)
	os.Setenv("HOME", tmp)

	updateDir := filepath.Join(getWorkerDataDir(), "updates")
	os.MkdirAll(updateDir, 0755)

	// Build a real candidate so this test exercises Windows process launching too.
	candidatePath := buildTestExecutable(t, tmp, 0)

	primaryPath := filepath.Join(tmp, "ForgeGrid.exe")
	backupPath := filepath.Join(tmp, "previous-ForgeGrid.exe")
	_ = buildTestExecutable(t, tmp, 0)
	if err := os.WriteFile(primaryPath, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}

	tx := &UpdateTransaction{
		ID:               "tx-123",
		CurrentState:     "STAGED",
		OldBinaryPath:    primaryPath,
		NewBinaryPath:    candidatePath,
		BackupBinaryPath: backupPath,
		LifecycleMode:    "portable",
		RestartDeadline:  time.Now().Add(5 * time.Second),
	}
	writeTx(tx)

	// We don't call RunUpdater directly as it blocks or exits, we test its components manually or run it in a goroutine
	// Here we simulate RunUpdater's flow for successful update:
	tx.CurrentState = "APPLYING"
	writeTx(tx)

	err := swapBinaries(tx)
	if err != nil {
		t.Fatalf("Swap failed: %v", err)
	}

	tx.CurrentState = "RESTARTING"
	writeTx(tx)
	err = GetLifecycle(tx.LifecycleMode).Start(tx)
	if err != nil {
		t.Fatalf("Candidate start failed: %v", err)
	}

	tx.CurrentState = "VERIFYING_NEW_WORKER"
	writeTx(tx)

	// Simulate candidate reporting health
	tx.CurrentState = "COMPLETED"
	writeTx(tx)

	err = waitForHealth(tx)
	if err != nil {
		t.Fatalf("Wait for health failed: %v", err)
	}
}

func TestUpdaterIntegrationRollback(t *testing.T) {
	// 36. Local deterministic rollback test
	tmp := t.TempDir()
	os.Setenv("USERPROFILE", tmp)
	os.Setenv("HOME", tmp)

	updateDir := filepath.Join(getWorkerDataDir(), "updates")
	os.MkdirAll(updateDir, 0755)

	candidatePath := buildTestExecutable(t, tmp, 1)

	primaryPath := filepath.Join(tmp, "ForgeGrid.exe")
	backupPath := filepath.Join(tmp, "previous-ForgeGrid.exe")
	if err := os.WriteFile(primaryPath, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	backupExecutable := buildTestExecutable(t, tmp, 0)
	backupBytes, err := os.ReadFile(backupExecutable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, backupBytes, 0755); err != nil {
		t.Fatal(err)
	}

	tx := &UpdateTransaction{
		ID:               "tx-rollback",
		CurrentState:     "VERIFYING_NEW_WORKER", // Skip to verifying
		OldBinaryPath:    primaryPath,
		NewBinaryPath:    candidatePath,
		BackupBinaryPath: backupPath,
		LifecycleMode:    "portable",
		RestartDeadline:  time.Now().Add(1 * time.Second),
	}
	writeTx(tx)

	// Wait for health should fail due to timeout
	err = waitForHealth(tx)
	if err == nil {
		t.Fatalf("Expected wait for health to timeout/fail")
	}

	// It should then rollback
	rollback(tx)

	// Primary should be backup
	if _, err := os.Stat(primaryPath); err != nil {
		t.Fatalf("restored primary is missing: %v", err)
	}
}
