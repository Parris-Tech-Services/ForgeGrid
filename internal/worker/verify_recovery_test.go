package worker

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// stubExit swaps osExit for the duration of the test so a failed-
// verification path can be exercised without actually terminating the test
// binary. Records every call.
func stubExit(t *testing.T) *[]int {
	t.Helper()
	calls := []int{}
	var mu sync.Mutex
	original := osExit
	osExit = func(code int) {
		mu.Lock()
		calls = append(calls, code)
		mu.Unlock()
	}
	t.Cleanup(func() { osExit = original })
	return &calls
}

func TestFailCandidateVerificationTriggersRollbackHelperInsteadOfLoopingForever(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	os.MkdirAll(filepath.Join(getWorkerDataDir(), "updates"), 0755)

	exitCalls := stubExit(t)

	helperCalls := []string{}
	originalLaunch := launchUpdaterHelper
	launchUpdaterHelper = func(path string) error {
		helperCalls = append(helperCalls, path)
		return nil
	}
	t.Cleanup(func() { launchUpdaterHelper = originalLaunch })

	tx := &UpdateTransaction{
		ID:                "tx-verify-fail",
		WorkerID:          "worker-1",
		CurrentState:      "VERIFYING_NEW_WORKER",
		UpdaterHelperPath: filepath.Join(tmp, "updater-helper-forgegrid"),
		LifecycleMode:     "portable",
	}
	writeTx(tx)

	w := &Worker{}
	w.failCandidateVerification(tx, "running hash did not match expected")

	if tx.CurrentState != "ROLLING_BACK" {
		t.Fatalf("expected in-memory tx to be marked ROLLING_BACK, got %q", tx.CurrentState)
	}

	onDisk, err := readTx()
	if err != nil {
		t.Fatalf("failed to read persisted tx: %v", err)
	}
	if onDisk.CurrentState != "ROLLING_BACK" {
		t.Fatalf("expected persisted tx to be ROLLING_BACK (so a fresh process picks up recovery), got %q", onDisk.CurrentState)
	}
	if !strings.Contains(onDisk.RollbackReason, "running hash did not match expected") {
		t.Fatalf("expected RollbackReason to explain why, got %q", onDisk.RollbackReason)
	}

	if len(helperCalls) != 1 || helperCalls[0] != tx.UpdaterHelperPath {
		t.Fatalf("expected the updater helper to be relaunched at %q exactly once, got %v", tx.UpdaterHelperPath, helperCalls)
	}

	if len(*exitCalls) != 1 || (*exitCalls)[0] != 1 {
		t.Fatalf("expected exactly one osExit(1) call, got %v", *exitCalls)
	}
}

// TestVerifyUpdateTransactionRecoversAfterVerificationFailureAcrossRestart
// reproduces the exact scenario Copilot's review flagged as HIGH:
//
//	old binary backed up
//	-> new binary replaced
//	-> transaction persisted as VERIFYING_NEW_WORKER
//	-> machine/process dies
//	-> worker starts again
//	-> new worker verification fails
//
// launchUpdaterHelper is stubbed to run RunUpdater() synchronously, which
// is a faithful simulation of "a fresh process starts and runs the
// updater helper" without needing a real subprocess. Before the fix,
// verifyUpdateTransaction called os.Exit(1) directly with no state change
// at all here, which on a real Windows service would just have the SCM
// restart the same broken candidate into the same failure forever - never
// touching the verified-good backup sitting right there.
func TestVerifyUpdateTransactionRecoversAfterVerificationFailureAcrossRestart(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	os.MkdirAll(filepath.Join(getWorkerDataDir(), "updates"), 0755)
	useFakeLifecycle(t, nil)
	stubExit(t)

	primary := filepath.Join(tmp, "ForgeGrid.exe")
	backup := filepath.Join(tmp, "previous-ForgeGrid.exe")
	os.WriteFile(primary, []byte("broken-candidate-binary"), 0755)
	os.WriteFile(backup, []byte("known-good-old-binary"), 0755)

	originalLaunch := launchUpdaterHelper
	launchUpdaterHelper = func(path string) error {
		// Simulate "a fresh process starts and runs -mode update-helper"
		// synchronously, the same way a real relaunch would end up
		// running RunUpdater() from a brand new process.
		RunUpdater()
		return nil
	}
	t.Cleanup(func() { launchUpdaterHelper = originalLaunch })

	tx := &UpdateTransaction{
		ID:                "tx-restart-recovery",
		WorkerID:          "worker-1",
		CurrentState:      "VERIFYING_NEW_WORKER",
		OldBinaryPath:     primary,
		BackupBinaryPath:  backup,
		UpdaterHelperPath: filepath.Join(tmp, "updater-helper-forgegrid"),
		// Deliberately wrong, so the candidate's own hash check fails -
		// this stands in for "new worker verification fails" from a
		// corrupted/incompatible binary after a reboot.
		ExpectedSHA256:  "0000000000000000000000000000000000000000000000000000000000000",
		LifecycleMode:   "portable",
		RestartDeadline: time.Now().Add(1 * time.Second),
	}
	writeTx(tx)

	w := &Worker{}
	// os.Executable() inside verifyUpdateTransaction reports this test
	// binary's own real path, not `primary` - fileSHA256 against the real
	// test binary still won't match the deliberately-wrong
	// ExpectedSHA256 above, so the failure path is exercised the same way
	// either way.
	w.verifyUpdateTransaction()

	final, err := readTx()
	if err != nil {
		t.Fatalf("failed to read final tx state: %v", err)
	}
	if final.CurrentState != "ROLLED_BACK" {
		t.Fatalf("expected deterministic recovery to ROLLED_BACK, got %q (reason: %s)", final.CurrentState, final.RollbackReason)
	}

	b, err := os.ReadFile(primary)
	if err != nil || string(b) != "known-good-old-binary" {
		t.Fatalf("expected the known-good backup to be restored to primary, got content=%q err=%v", b, err)
	}
}
