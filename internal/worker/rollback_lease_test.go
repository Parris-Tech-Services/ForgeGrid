package worker

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestRollbackClaimLeaseKeepsLiveOwnerFromBeingStolen proves the core
// Finding B fix directly: a claim with a fresh, actively-refreshed lease
// must never be stolen, even after the old bare stealWaitDuration-style
// interval would have elapsed.
func TestRollbackClaimLeaseKeepsLiveOwnerFromBeingStolen(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)

	claimPath := filepath.Join(tmp, "previous-ForgeGrid.exe.claim.owner")
	os.WriteFile(claimPath, []byte("owned"), 0755)

	originalInterval := claimLeaseInterval
	claimLeaseInterval = 5 * time.Millisecond
	t.Cleanup(func() { claimLeaseInterval = originalInterval })
	originalStale := claimLeaseStaleAfter
	claimLeaseStaleAfter = 200 * time.Millisecond
	t.Cleanup(func() { claimLeaseStaleAfter = originalStale })

	stop := startLeaseKeeper(claimPath)
	defer stop()

	// Give the keeper time to write its first lease.
	time.Sleep(20 * time.Millisecond)

	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !claimIsLive(claimPath) {
			t.Fatalf("a claim with an actively-refreshed lease must be reported live throughout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestRollbackClaimLeaseGoesStaleWhenOwnerStopsRefreshing proves the
// complementary half: once refreshing genuinely stops (the owner crashed),
// the claim must eventually - but only after claimLeaseStaleAfter, not
// immediately - be reported stale so a new actor may safely reclaim it.
func TestRollbackClaimLeaseGoesStaleWhenOwnerStopsRefreshing(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)

	claimPath := filepath.Join(tmp, "previous-ForgeGrid.exe.claim.owner")
	os.WriteFile(claimPath, []byte("owned"), 0755)

	originalStale := claimLeaseStaleAfter
	claimLeaseStaleAfter = 40 * time.Millisecond
	t.Cleanup(func() { claimLeaseStaleAfter = originalStale })

	// Write exactly one lease, as if the owner wrote it once and then
	// crashed before its next scheduled refresh - never call
	// startLeaseKeeper's background goroutine at all.
	os.WriteFile(leasePath(claimPath), []byte(`{"pid":1}`), 0600)

	if !claimIsLive(claimPath) {
		t.Fatalf("a just-written lease must be live immediately")
	}
	time.Sleep(60 * time.Millisecond)
	if claimIsLive(claimPath) {
		t.Fatalf("a lease that stopped being refreshed must go stale after claimLeaseStaleAfter")
	}
}

// TestRollbackClaimWithNoLeaseAtAllIsNeverLive covers the crash-before-
// ever-writing-a-lease case: an orphaned claim with no lease file present
// at all must be treated as immediately stealable, not waited on.
func TestRollbackClaimWithNoLeaseAtAllIsNeverLive(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	claimPath := filepath.Join(tmp, "previous-ForgeGrid.exe.claim.crashed")
	os.WriteFile(claimPath, []byte("owned"), 0755)

	if claimIsLive(claimPath) {
		t.Fatalf("a claim with no lease file at all must never be considered live")
	}
}

// TestRollbackDeadOwnerClaimIsReclaimedThroughFullFlow exercises the real
// rollback() Phase 1 path end to end: a claim left behind by an owner that
// crashed (lease written once, then never refreshed and past
// claimLeaseStaleAfter) must be stolen and the rollback completed
// correctly - not left stuck waiting forever.
func TestRollbackDeadOwnerClaimIsReclaimedThroughFullFlow(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	os.MkdirAll(filepath.Join(getWorkerDataDir(), "updates"), 0755)

	primary := filepath.Join(tmp, "ForgeGrid.exe")
	os.WriteFile(primary, []byte("old"), 0755)

	tx := &UpdateTransaction{
		ID:               "tx-dead-owner",
		CurrentState:     "VERIFYING_NEW_WORKER",
		OldBinaryPath:    primary,
		BackupBinaryPath: filepath.Join(tmp, "previous-ForgeGrid.exe"),
		LifecycleMode:    "portable",
	}
	writeTx(tx)

	originalStale := claimLeaseStaleAfter
	claimLeaseStaleAfter = 20 * time.Millisecond
	t.Cleanup(func() { claimLeaseStaleAfter = originalStale })
	originalPoll := stealPollInterval
	stealPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { stealPollInterval = originalPoll })
	originalVerifyPoll := verifyPollInterval
	verifyPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { verifyPollInterval = originalVerifyPoll })
	originalVerifyWait := verifyWaitDuration
	verifyWaitDuration = 100 * time.Millisecond
	t.Cleanup(func() { verifyWaitDuration = originalVerifyWait })

	var startCalls int
	useFakeLifecycleCounting(t, &startCalls, nil)
	originalLifecycle := GetLifecycle
	GetLifecycle = func(mode string) Lifecycle {
		return &fakeLifecycle{mode: mode, startFn: func(tx *UpdateTransaction) error {
			startCalls++
			tx.CurrentState = "ROLLED_BACK"
			writeTx(tx)
			return nil
		}}
	}
	t.Cleanup(func() { GetLifecycle = originalLifecycle })

	// Simulate a genuinely dead owner: it claimed the backup and wrote one
	// lease, then crashed before ever refreshing it or consuming the claim.
	backupPath := filepath.Join(tmp, "previous-ForgeGrid.exe")
	deadOwnerClaim := backupPath + ".claim.dead-owner"
	os.WriteFile(deadOwnerClaim, []byte("backup"), 0755)
	os.WriteFile(leasePath(deadOwnerClaim), []byte(`{"pid":999999}`), 0600)
	time.Sleep(30 * time.Millisecond) // past claimLeaseStaleAfter

	rollback(tx)

	final, err := readTx()
	if err != nil {
		t.Fatalf("failed to read tx: %v", err)
	}
	if final.CurrentState != "ROLLED_BACK" {
		t.Fatalf("expected the dead owner's claim to be reclaimed and rollback completed, got %q", final.CurrentState)
	}
	if b, _ := os.ReadFile(primary); string(b) != "backup" {
		t.Fatalf("expected primary restored from the reclaimed backup, got %q", b)
	}
}

// TestRollbackConcurrentActorsNeverBothStartSimultaneously is a race-mode
// stress test: many rollback() calls for the same transaction run
// concurrently, and at no point may more than one of them be inside a
// lifecycle Start() call at once - proving the restart lease added
// alongside the claim lease (see updater.go's t2Base+".starting") actually
// serializes Start(), not merely the token hand-off.
func TestRollbackConcurrentActorsNeverBothStartSimultaneously(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	os.MkdirAll(filepath.Join(getWorkerDataDir(), "updates"), 0755)

	primary := filepath.Join(tmp, "ForgeGrid.exe")
	backup := filepath.Join(tmp, "previous-ForgeGrid.exe")
	os.WriteFile(primary, []byte("old"), 0755)
	os.WriteFile(backup, []byte("backup"), 0755)

	tx := &UpdateTransaction{
		ID:               "tx-concurrent-start-guard",
		CurrentState:     "VERIFYING_NEW_WORKER",
		OldBinaryPath:    primary,
		BackupBinaryPath: backup,
		LifecycleMode:    "portable",
	}
	writeTx(tx)

	originalInterval := claimLeaseInterval
	claimLeaseInterval = 5 * time.Millisecond
	t.Cleanup(func() { claimLeaseInterval = originalInterval })
	originalStale := claimLeaseStaleAfter
	claimLeaseStaleAfter = 300 * time.Millisecond
	t.Cleanup(func() { claimLeaseStaleAfter = originalStale })
	originalPoll := stealPollInterval
	stealPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { stealPollInterval = originalPoll })
	originalVerifyPoll := verifyPollInterval
	verifyPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { verifyPollInterval = originalVerifyPoll })
	originalVerifyWait := verifyWaitDuration
	verifyWaitDuration = 20 * time.Millisecond
	t.Cleanup(func() { verifyWaitDuration = originalVerifyWait })

	var mu sync.Mutex
	insideStart := 0
	maxConcurrentStart := 0
	originalLifecycle := GetLifecycle
	GetLifecycle = func(mode string) Lifecycle {
		return &fakeLifecycle{mode: mode, startFn: func(tx *UpdateTransaction) error {
			mu.Lock()
			insideStart++
			if insideStart > maxConcurrentStart {
				maxConcurrentStart = insideStart
			}
			mu.Unlock()

			time.Sleep(50 * time.Millisecond) // simulate a real, slow service start

			mu.Lock()
			insideStart--
			mu.Unlock()

			tx.CurrentState = "ROLLED_BACK"
			writeTx(tx)
			return nil
		}}
	}
	t.Cleanup(func() { GetLifecycle = originalLifecycle })

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			time.Sleep(time.Duration(n) * time.Millisecond)
			rollback(tx)
		}(i)
	}
	wg.Wait()

	mu.Lock()
	max := maxConcurrentStart
	mu.Unlock()
	if max > 1 {
		t.Fatalf("expected at most one concurrent Start() call across all rollback() actors, observed %d simultaneously", max)
	}

	final, err := readTx()
	if err != nil {
		t.Fatalf("failed to read tx: %v", err)
	}
	if final.CurrentState != "ROLLED_BACK" {
		t.Fatalf("expected final state ROLLED_BACK, got %q", final.CurrentState)
	}
}
