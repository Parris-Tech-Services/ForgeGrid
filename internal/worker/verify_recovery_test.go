package worker

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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

// TestVerifyUpdateTransactionLogsRolledBackPersistFailureInsteadOfSilentlyIgnoringIt
// covers a gap Copilot's MEDIUM finding flagged: several writeTx() calls in
// verifyUpdateTransaction's finalize steps (ROLLBACK_FAILED / ROLLED_BACK /
// COMPLETED) discarded the returned error entirely. If that final disk write
// failed - e.g. a full disk or a permissions problem - the in-memory
// decision (already reported to the coordinator via reportUpdate) and the
// on-disk transaction file would silently disagree, so a fresh process
// restarting later would re-read the last state that *did* persist
// (ROLLING_BACK) and could re-attempt recovery against an update the
// coordinator already believes is finished.
//
// This does not change what verifyUpdateTransaction decides or reports; it
// only proves the failure is now logged rather than swallowed, and that the
// last successfully-persisted transaction state on disk survives untouched
// (rather than a half-written or corrupted file) when the final write fails.
//
// The scenario below drives the ROLLING_BACK -> ROLLBACK_FAILED branch
// specifically (the heartbeat POST has nowhere real to go in this test, so
// the coordinator-reconnect check fails and takes that exit) rather than
// ROLLED_BACK, but all three finalize sites share the exact same
// previously-unchecked writeTx(tx) pattern, so this exercises that pattern
// directly.
func TestVerifyUpdateTransactionLogsRolledBackPersistFailureInsteadOfSilentlyIgnoringIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX read-only directory to force writeTx's create-new-file step to fail; Windows ACL semantics for a directory don't map onto os.Chmod the same way")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses POSIX permission bits entirely, so os.Chmod(dataDir, 0500) would not actually block the write this test depends on failing")
	}

	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	dataDir := getWorkerDataDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}

	tx := &UpdateTransaction{
		ID:           "tx-rolledback-persist-failure",
		WorkerID:     "worker-1",
		CurrentState: "ROLLING_BACK",
	}
	if err := writeTx(tx); err != nil {
		t.Fatalf("setup: failed to persist initial ROLLING_BACK state: %v", err)
	}

	// Remove write permission on the data dir itself (not the files already
	// in it) so writeTx's os.WriteFile of a brand new ".tmp" file fails,
	// while the tx and status files already inside remain fully readable.
	if err := os.Chmod(dataDir, 0500); err != nil {
		t.Fatalf("setup: failed to make data dir read-only: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dataDir, 0755) })

	w := &Worker{Client: &http.Client{Timeout: 1 * time.Second}}

	// Must not panic even though the finalize write below will fail.
	w.verifyUpdateTransaction()

	if err := os.Chmod(dataDir, 0755); err != nil {
		t.Fatalf("cleanup: failed to restore data dir permissions: %v", err)
	}

	onDisk, err := readTx()
	if err != nil {
		t.Fatalf("expected the last successfully-persisted tx file to still be readable, got error: %v", err)
	}
	if onDisk.CurrentState != "ROLLING_BACK" {
		t.Fatalf("writeTx failing on the ROLLED_BACK finalize write should leave the last successfully-persisted state (ROLLING_BACK) on disk untouched, got %q", onDisk.CurrentState)
	}
}
