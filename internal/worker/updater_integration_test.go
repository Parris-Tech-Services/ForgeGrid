package worker

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestUpdaterIntegrationSuccessfulUpdate(t *testing.T) {
	// 35. Local deterministic successful-update test
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	// A real subprocess would have to be a valid, platform-specific
	// executable (a Unix shebang script is not a valid Windows PE binary,
	// which is exactly what made this test fail on Windows CI). What this
	// test actually verifies is the transaction state machine, not OS
	// process spawning, so GetLifecycle is faked instead.
	useFakeLifecycle(t, nil)

	updateDir := filepath.Join(getWorkerDataDir(), "updates")
	os.MkdirAll(updateDir, 0755)

	// Candidate/primary/backup file contents are never executed now that
	// Start() is faked; they only need to exist for swapBinaries to move.
	candidatePath := filepath.Join(tmp, "candidate.exe")
	os.WriteFile(candidatePath, []byte("candidate"), 0755)

	primaryPath := filepath.Join(tmp, "ForgeGrid.exe")
	backupPath := filepath.Join(tmp, "previous-ForgeGrid.exe")
	os.WriteFile(primaryPath, []byte("old"), 0755)
	os.WriteFile(backupPath, []byte("old"), 0755)

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
	originalVerifyPoll := verifyPollInterval
	verifyPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { verifyPollInterval = originalVerifyPoll })

	// 36. Local deterministic rollback test
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	setSandboxedDataDir(t, tmp)

	originalLifecycle := GetLifecycle
	GetLifecycle = func(mode string) Lifecycle {
		return &fakeLifecycle{
			mode: mode,
			startFn: func(tx *UpdateTransaction) error {
				go func() {
					time.Sleep(5 * time.Millisecond)
					latest, err := readTx()
					if err == nil {
						latest.CurrentState = "ROLLED_BACK"
						writeTx(latest)
					}
				}()
				return nil
			},
		}
	}
	t.Cleanup(func() { GetLifecycle = originalLifecycle })
	updateDir := filepath.Join(getWorkerDataDir(), "updates")
	os.MkdirAll(updateDir, 0755)

	candidatePath := filepath.Join(tmp, "candidate.exe")
	os.WriteFile(candidatePath, []byte("candidate"), 0755)

	primaryPath := filepath.Join(tmp, "ForgeGrid.exe")
	backupPath := filepath.Join(tmp, "previous-ForgeGrid.exe")
	os.WriteFile(primaryPath, []byte("old"), 0755)
	os.WriteFile(backupPath, []byte("backup"), 0755)

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
	err := waitForHealth(tx)
	if err == nil {
		t.Fatalf("Expected wait for health to timeout/fail")
	}

	// It should then rollback
	rollback(tx)

	// Primary should be backup
	b, _ := os.ReadFile(primaryPath)
	if string(b) != "backup" {
		t.Fatalf("Primary was not restored: %s", string(b))
	}
}

func TestRollbackConcurrencyRace(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)

	updateDir := filepath.Join(getWorkerDataDir(), "updates")
	os.MkdirAll(updateDir, 0755)

	primaryPath := filepath.Join(tmp, "ForgeGrid.exe")
	backupPath := filepath.Join(tmp, "previous-ForgeGrid.exe")
	os.WriteFile(primaryPath, []byte("old"), 0755)
	os.WriteFile(backupPath, []byte("backup"), 0755)

	tx := &UpdateTransaction{
		ID:               "tx-race",
		CurrentState:     "VERIFYING_NEW_WORKER",
		OldBinaryPath:    primaryPath,
		BackupBinaryPath: backupPath,
		LifecycleMode:    "portable",
	}
	writeTx(tx)

	var mu sync.Mutex
	var startCount, replaceCount int

	originalLifecycle := GetLifecycle
	GetLifecycle = func(mode string) Lifecycle {
		return &fakeLifecycle{
			mode: mode,
			startFn: func(tx *UpdateTransaction) error {
				mu.Lock()
				startCount++
				mu.Unlock()
				// Simulate slow restart
				time.Sleep(200 * time.Millisecond)
				
				// Simulate the worker verifying its own health
				tx.CurrentState = "ROLLED_BACK"
				writeTx(tx)
				return nil
			},
		}
	}
	t.Cleanup(func() { GetLifecycle = originalLifecycle })

	originalReplace := safeReplace
	safeReplace = func(newPath, destPath string) error {
		// Simulate slow file operations
		time.Sleep(200 * time.Millisecond)
		err := originalReplace(newPath, destPath)
		if err == nil {
			mu.Lock()
			replaceCount++
			mu.Unlock()
		}
		return err
	}
	t.Cleanup(func() { safeReplace = originalReplace })

	originalStealWait := stealWaitDuration
	stealWaitDuration = 10 * time.Millisecond
	t.Cleanup(func() { stealWaitDuration = originalStealWait })

	originalVerifyPoll := verifyPollInterval
	verifyPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { verifyPollInterval = originalVerifyPoll })

	originalVerifyWait := verifyWaitDuration
	verifyWaitDuration = 30 * time.Millisecond
	t.Cleanup(func() { verifyWaitDuration = originalVerifyWait })

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		rollback(tx)
	}()

	go func() {
		defer wg.Done()
		// Launch slightly after to let the first one claim Phase 1
		time.Sleep(5 * time.Millisecond)
		rollback(tx)
	}()

	wg.Wait()

	current, err := readTx()
	if err != nil {
		t.Fatalf("Failed to read tx: %v", err)
	}
	if current.CurrentState != "ROLLED_BACK" {
		t.Fatalf("Expected state to be ROLLED_BACK, got %s", current.CurrentState)
	}

	b, _ := os.ReadFile(primaryPath)
	if string(b) != "backup" {
		t.Fatalf("Primary was not restored properly, got %s", string(b))
	}

	mu.Lock()
	sc := startCount
	rc := replaceCount
	mu.Unlock()

	if rc != 1 {
		t.Fatalf("safeReplace was executed %d times, expected exactly 1", rc)
	}
	if sc != 1 {
		t.Fatalf("Start() was executed %d times, expected exactly 1", sc)
	}
}

