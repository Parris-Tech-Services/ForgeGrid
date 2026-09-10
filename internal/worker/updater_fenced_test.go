package worker

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRollbackCrashPhase1(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)

	updateDir := filepath.Join(getWorkerDataDir(), "updates")
	os.MkdirAll(updateDir, 0755)

	primaryPath := filepath.Join(tmp, "ForgeGrid.exe")
	backupPath := filepath.Join(tmp, "previous-ForgeGrid.exe")
	os.WriteFile(primaryPath, []byte("old"), 0755)
	os.WriteFile(backupPath, []byte("backup"), 0755)

	tx := &UpdateTransaction{
		ID:               "tx-crash-p1",
		CurrentState:     "VERIFYING_NEW_WORKER",
		OldBinaryPath:    primaryPath,
		BackupBinaryPath: backupPath,
		LifecycleMode:    "portable",
	}
	writeTx(tx)

	// Speed up tests
	originalStealWait := stealWaitDuration
	stealWaitDuration = 10 * time.Millisecond
	t.Cleanup(func() { stealWaitDuration = originalStealWait })

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
				return nil
			},
		}
	}
	t.Cleanup(func() { GetLifecycle = originalLifecycle })

	originalReplace := safeReplace
	safeReplace = func(newPath, destPath string) error {
		mu.Lock()
		replaceCount++
		mu.Unlock()
		return originalReplace(newPath, destPath)
	}
	t.Cleanup(func() { safeReplace = originalReplace })

	// SIMULATE CRASH IN PHASE 1 BEFORE safeReplace
	// A crashed worker leaves the claim file but never resumes.
	os.Rename(backupPath, backupPath+".claim.crashed")

	// The second worker arrives, sees the claim, waits, steals it, and finishes.
	rollback(tx)

	current, _ := readTx()
	if current.CurrentState != "ROLLED_BACK" {
		t.Fatalf("Expected state to be ROLLED_BACK, got %s", current.CurrentState)
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

func TestRollbackCrashPhase2(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)

	updateDir := filepath.Join(getWorkerDataDir(), "updates")
	os.MkdirAll(updateDir, 0755)

	primaryPath := filepath.Join(tmp, "ForgeGrid.exe")
	backupPath := filepath.Join(tmp, "previous-ForgeGrid.exe")
	os.WriteFile(primaryPath, []byte("backup"), 0755) // Already restored

	tx := &UpdateTransaction{
		ID:               "tx-crash-p2",
		CurrentState:     "ROLLING_BACK",
		OldBinaryPath:    primaryPath,
		BackupBinaryPath: backupPath,
		LifecycleMode:    "portable",
	}
	writeTx(tx)

	originalStealWait := stealWaitDuration
	stealWaitDuration = 10 * time.Millisecond
	t.Cleanup(func() { stealWaitDuration = originalStealWait })

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
				return nil
			},
		}
	}
	t.Cleanup(func() { GetLifecycle = originalLifecycle })

	originalReplace := safeReplace
	safeReplace = func(newPath, destPath string) error {
		mu.Lock()
		replaceCount++
		mu.Unlock()
		return originalReplace(newPath, destPath)
	}
	t.Cleanup(func() { safeReplace = originalReplace })

	// SIMULATE CRASH IN PHASE 2 BEFORE Start()
	// Backup is completely gone. Phase 2 pending exists.
	t2Base := filepath.Join(getWorkerDataDir(), "rollback_restart_"+tx.ID)
	os.WriteFile(t2Base+".pending", []byte{}, 0644)
	
	// Wait, actually, let's claim it and simulate crash right before consume!
	os.Rename(t2Base+".pending", t2Base+".claim.crashed")

	rollback(tx)

	current, _ := readTx()
	if current.CurrentState != "ROLLED_BACK" {
		t.Fatalf("Expected state to be ROLLED_BACK, got %s", current.CurrentState)
	}

	mu.Lock()
	sc := startCount
	rc := replaceCount
	mu.Unlock()

	if rc != 0 {
		t.Fatalf("safeReplace was executed %d times, expected exactly 0 since Phase 1 was skipped", rc)
	}
	if sc != 1 {
		t.Fatalf("Start() was executed %d times, expected exactly 1", sc)
	}
}

func TestRollbackCrashPhase3(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)

	updateDir := filepath.Join(getWorkerDataDir(), "updates")
	os.MkdirAll(updateDir, 0755)

	primaryPath := filepath.Join(tmp, "ForgeGrid.exe")
	backupPath := filepath.Join(tmp, "previous-ForgeGrid.exe")
	os.WriteFile(primaryPath, []byte("backup"), 0755) // Already restored

	tx := &UpdateTransaction{
		ID:               "tx-crash-p3",
		CurrentState:     "ROLLING_BACK",
		OldBinaryPath:    primaryPath,
		BackupBinaryPath: backupPath,
		LifecycleMode:    "portable",
	}
	writeTx(tx)

	originalStealWait := stealWaitDuration
	stealWaitDuration = 10 * time.Millisecond
	t.Cleanup(func() { stealWaitDuration = originalStealWait })

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
				return nil
			},
		}
	}
	t.Cleanup(func() { GetLifecycle = originalLifecycle })

	originalReplace := safeReplace
	safeReplace = func(newPath, destPath string) error {
		mu.Lock()
		replaceCount++
		mu.Unlock()
		return originalReplace(newPath, destPath)
	}
	t.Cleanup(func() { safeReplace = originalReplace })

	// SIMULATE CRASH IN PHASE 3 (After Start, before state update)
	// Backup is completely gone. Phase 2 consumed exists.
	t2Base := filepath.Join(getWorkerDataDir(), "rollback_restart_"+tx.ID)
	os.WriteFile(t2Base+".consumed", []byte{}, 0644)

	rollback(tx)

	current, _ := readTx()
	if current.CurrentState != "ROLLED_BACK" {
		t.Fatalf("Expected state to be ROLLED_BACK, got %s", current.CurrentState)
	}

	mu.Lock()
	sc := startCount
	rc := replaceCount
	mu.Unlock()

	if rc != 0 {
		t.Fatalf("safeReplace was executed %d times, expected exactly 0", rc)
	}
	if sc != 0 {
		t.Fatalf("Start() was executed %d times, expected exactly 0 since Phase 2 was skipped", sc)
	}
}
