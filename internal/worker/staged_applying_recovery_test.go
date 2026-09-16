package worker

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ackingCoordinator returns an httptest.Server that acknowledges every
// /api/updates/report POST with 200, so reportUpdate calls inside recovery
// paths resolve immediately instead of exercising their own retry/persist
// logic - kept separate from what these tests are actually asserting.
func ackingCoordinator(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRecoverStagedResumesWhenCandidateAndHelperAreIntact(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	os.MkdirAll(filepath.Join(getWorkerDataDir(), "updates"), 0755)
	srv := ackingCoordinator(t)
	exitCalls := stubExit(t)

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	oldHash, err := fileSHA256(exe)
	if err != nil {
		t.Fatal(err)
	}

	candidatePath := filepath.Join(tmp, "staged-candidate")
	os.WriteFile(candidatePath, []byte("candidate-bytes"), 0755)
	newHash, _ := fileSHA256(candidatePath)

	helperPath := filepath.Join(tmp, "updater-helper")
	os.WriteFile(helperPath, []byte("helper"), 0755)

	tx := &UpdateTransaction{
		ID:                "tx-staged-resume",
		WorkerID:          "worker-1",
		CurrentState:      "STAGED",
		OldBinaryPath:     exe,
		NewBinaryPath:     candidatePath,
		UpdaterHelperPath: helperPath,
		OldSHA256:         oldHash,
		ExpectedSHA256:    newHash,
		LifecycleMode:     "portable",
	}
	writeTx(tx)

	var helperCalls []string
	originalLaunch := launchUpdaterHelper
	launchUpdaterHelper = func(path string) error {
		helperCalls = append(helperCalls, path)
		return nil
	}
	t.Cleanup(func() { launchUpdaterHelper = originalLaunch })

	w := &Worker{WorkerID: "worker-1", Token: "t", CoordinatorURL: srv.URL, Client: &http.Client{}}
	w.verifyUpdateTransaction()

	if len(helperCalls) != 1 || helperCalls[0] != helperPath {
		t.Fatalf("expected the updater helper to be relaunched exactly once at %q, got %v", helperPath, helperCalls)
	}
	if len(*exitCalls) != 1 || (*exitCalls)[0] != 0 {
		t.Fatalf("expected exactly one osExit(0), got %v", *exitCalls)
	}
	// Nothing destructive: STAGED proves the swap never ran, so the
	// candidate and the (test binary standing in for the) old binary must
	// both be exactly as they were.
	if b, _ := os.ReadFile(candidatePath); string(b) != "candidate-bytes" {
		t.Fatalf("staged candidate must be untouched, got %q", b)
	}
	if h, _ := fileSHA256(exe); h != oldHash {
		t.Fatalf("running binary must be untouched, hash changed from %s to %s", oldHash, h)
	}
}

func TestRecoverStagedFailsClosedWhenCandidateIsMissing(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	os.MkdirAll(filepath.Join(getWorkerDataDir(), "updates"), 0755)
	srv := ackingCoordinator(t)
	stubExit(t)

	exe, _ := os.Executable()
	oldHash, _ := fileSHA256(exe)

	var helperCalled bool
	originalLaunch := launchUpdaterHelper
	launchUpdaterHelper = func(path string) error { helperCalled = true; return nil }
	t.Cleanup(func() { launchUpdaterHelper = originalLaunch })

	tx := &UpdateTransaction{
		ID:                "tx-staged-missing-candidate",
		WorkerID:          "worker-1",
		CurrentState:      "STAGED",
		OldBinaryPath:     exe,
		NewBinaryPath:     filepath.Join(tmp, "never-written-candidate"),
		UpdaterHelperPath: filepath.Join(tmp, "never-written-helper"),
		OldSHA256:         oldHash,
		LifecycleMode:     "portable",
	}
	writeTx(tx)

	w := &Worker{WorkerID: "worker-1", Token: "t", CoordinatorURL: srv.URL, Client: &http.Client{}}
	w.verifyUpdateTransaction()

	if helperCalled {
		t.Fatalf("must not relaunch the updater helper against a missing/unverifiable candidate")
	}
	final, err := readTx()
	if err != nil {
		t.Fatalf("failed to read tx: %v", err)
	}
	if final.CurrentState != "FAILED" {
		t.Fatalf("expected CurrentState=FAILED (fail closed, nothing destructive happened), got %q", final.CurrentState)
	}
	if h, _ := fileSHA256(exe); h != oldHash {
		t.Fatalf("running binary must remain untouched on a failed-closed STAGED recovery")
	}
}

func TestRecoverApplyingSwapNotStartedRetriesSwap(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	os.MkdirAll(filepath.Join(getWorkerDataDir(), "updates"), 0755)
	srv := ackingCoordinator(t)
	stubExit(t)

	primary := filepath.Join(tmp, "ForgeGrid.exe")
	os.WriteFile(primary, []byte("old-binary-bytes"), 0755)
	oldHash, _ := fileSHA256(primary)

	candidate := filepath.Join(tmp, "staged-candidate")
	os.WriteFile(candidate, []byte("new-binary-bytes"), 0755)
	newHash, _ := fileSHA256(candidate)

	var startCalls int
	useFakeLifecycleCounting(t, &startCalls, nil)

	tx := &UpdateTransaction{
		ID:               "tx-applying-not-started",
		WorkerID:         "worker-1",
		CurrentState:     "APPLYING",
		OldBinaryPath:    primary,
		NewBinaryPath:    candidate,
		BackupBinaryPath: filepath.Join(tmp, "previous-ForgeGrid.exe"),
		OldSHA256:        oldHash,
		ExpectedSHA256:   newHash,
		LifecycleMode:    "portable",
	}
	writeTx(tx)

	w := &Worker{WorkerID: "worker-1", Token: "t", CoordinatorURL: srv.URL, Client: &http.Client{}}
	w.verifyUpdateTransaction()

	// The swap must actually have happened: the new binary's bytes now
	// live at the primary path.
	b, err := os.ReadFile(primary)
	if err != nil || string(b) != "new-binary-bytes" {
		t.Fatalf("expected primary to hold the new binary's bytes after a resumed swap, got %q err=%v", b, err)
	}
	if startCalls != 1 {
		t.Fatalf("expected the candidate to be started exactly once, got %d", startCalls)
	}
	final, err := readTx()
	if err != nil {
		t.Fatalf("failed to read tx: %v", err)
	}
	if final.CurrentState != "VERIFYING_NEW_WORKER" {
		t.Fatalf("expected CurrentState=VERIFYING_NEW_WORKER after a resumed swap, got %q", final.CurrentState)
	}
}

func TestRecoverApplyingAlreadyAppliedResumesVerification(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	os.MkdirAll(filepath.Join(getWorkerDataDir(), "updates"), 0755)
	srv := ackingCoordinator(t)
	stubExit(t)

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// The swap already landed before the crash: the running exe (this
	// test binary, via os.Executable()) IS the "new" binary, so its hash
	// must be recorded as ExpectedSHA256, proving swapAlreadyApplied.
	newHash, err := fileSHA256(exe)
	if err != nil {
		t.Fatal(err)
	}

	tx := &UpdateTransaction{
		ID:               "tx-applying-already-applied",
		WorkerID:         "worker-1",
		CurrentState:     "APPLYING",
		OldBinaryPath:    exe,
		NewBinaryPath:    filepath.Join(tmp, "no-longer-relevant-candidate"),
		BackupBinaryPath: filepath.Join(tmp, "previous-ForgeGrid.exe"),
		OldSHA256:        "0000000000000000000000000000000000000000000000000000000000000",
		ExpectedSHA256:   newHash,
		LifecycleMode:    "portable",
		RestartDeadline:  time.Now().Add(time.Second),
	}
	writeTx(tx)

	w := &Worker{WorkerID: "worker-1", Token: "t", CoordinatorURL: srv.URL, Client: &http.Client{}}
	w.verifyUpdateTransaction()

	final, err := readTx()
	if err != nil {
		t.Fatalf("failed to read tx: %v", err)
	}
	if final.CurrentState != "COMPLETED" {
		t.Fatalf("expected verification to run and complete once swapAlreadyApplied is detected, got %q (reason: %s)", final.CurrentState, final.RollbackReason)
	}
}

func TestRecoverApplyingInterruptedRestoresFromReplacedSideCopy(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	os.MkdirAll(filepath.Join(getWorkerDataDir(), "updates"), 0755)
	srv := ackingCoordinator(t)
	stubExit(t)

	primary := filepath.Join(tmp, "ForgeGrid.exe")
	oldBytes := []byte("known-good-old-binary")
	oldHash := writeAndHash(t, primary+".replaced", oldBytes)
	// Simulate the exact crash window inside safeReplace: the original was
	// already moved aside to ".replaced" but the new binary was never
	// renamed into place, so `primary` itself does not exist at all.
	os.Remove(primary)

	var startCalls int
	useFakeLifecycleCounting(t, &startCalls, nil)

	tx := &UpdateTransaction{
		ID:               "tx-applying-interrupted",
		WorkerID:         "worker-1",
		CurrentState:     "APPLYING",
		OldBinaryPath:    primary,
		NewBinaryPath:    filepath.Join(tmp, "candidate-irrelevant-here"),
		BackupBinaryPath: filepath.Join(tmp, "previous-ForgeGrid.exe"),
		OldSHA256:        oldHash,
		ExpectedSHA256:   "1111111111111111111111111111111111111111111111111111111111111",
		LifecycleMode:    "portable",
	}
	writeTx(tx)

	w := &Worker{WorkerID: "worker-1", Token: "t", CoordinatorURL: srv.URL, Client: &http.Client{}}
	w.verifyUpdateTransaction()

	b, err := os.ReadFile(primary)
	if err != nil || string(b) != string(oldBytes) {
		t.Fatalf("expected the previous binary restored to primary, got %q err=%v", b, err)
	}
	final, err := readTx()
	if err != nil {
		t.Fatalf("failed to read tx: %v", err)
	}
	if final.CurrentState != "ROLLED_BACK" {
		t.Fatalf("expected CurrentState=ROLLED_BACK after restoring from the interrupted-swap side copy, got %q", final.CurrentState)
	}
	if startCalls != 0 {
		t.Fatalf("recovering from the interrupted-swap side copy must not itself restart anything (that only happens via rollback()'s own Phase 2), got %d Start() calls", startCalls)
	}
}

func TestRecoverApplyingAmbiguousStateFailsClosedViaRollback(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	os.MkdirAll(filepath.Join(getWorkerDataDir(), "updates"), 0755)
	srv := ackingCoordinator(t)
	stubExit(t)

	primary := filepath.Join(tmp, "ForgeGrid.exe")
	// Corruption: primary exists but matches neither the old nor the
	// expected-new hash, and there is no ".replaced" side copy either -
	// genuinely nothing here proves what state the binary is in.
	os.WriteFile(primary, []byte("mystery-bytes-matching-nothing"), 0755)

	backup := filepath.Join(tmp, "previous-ForgeGrid.exe")
	backupBytes := []byte("known-good-backup-binary")
	os.WriteFile(backup, backupBytes, 0755)

	// rollback()'s Phase 3 waits for the restarted worker to report a
	// terminal state; the fake lifecycle must actually do that (as every
	// other rollback()-driving test in this package does), and the
	// production-length lease/verify timings must be shortened, or this
	// legitimately retries forever at real-world cadence like
	// TestRollbackCrashPhase1/2/3 and TestRollbackConcurrencyRace already
	// do for the same reason.
	originalVerifyPoll := verifyPollInterval
	verifyPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { verifyPollInterval = originalVerifyPoll })
	originalVerifyWait := verifyWaitDuration
	verifyWaitDuration = 30 * time.Millisecond
	t.Cleanup(func() { verifyWaitDuration = originalVerifyWait })

	var startCalls int
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

	tx := &UpdateTransaction{
		ID:               "tx-applying-ambiguous",
		WorkerID:         "worker-1",
		CurrentState:     "APPLYING",
		OldBinaryPath:    primary,
		NewBinaryPath:    filepath.Join(tmp, "candidate-irrelevant"),
		BackupBinaryPath: backup,
		OldSHA256:        "2222222222222222222222222222222222222222222222222222222222222",
		ExpectedSHA256:   "3333333333333333333333333333333333333333333333333333333333333",
		LifecycleMode:    "portable",
	}
	writeTx(tx)

	w := &Worker{WorkerID: "worker-1", Token: "t", CoordinatorURL: srv.URL, Client: &http.Client{}}
	w.verifyUpdateTransaction()

	// Ambiguity must fail closed via the existing, well-tested rollback()
	// path - restoring from the independent BackupBinaryPath, never
	// guessing that the mystery bytes at primary are safe to keep.
	b, err := os.ReadFile(primary)
	if err != nil || string(b) != string(backupBytes) {
		t.Fatalf("expected ambiguous APPLYING state to fail closed by restoring the known backup, got %q err=%v", b, err)
	}
	if startCalls != 1 {
		t.Fatalf("expected rollback()'s own Phase 2 restart exactly once, got %d", startCalls)
	}
}

func TestRecoverApplyingAmbiguousWithMissingBackupFailsClosedWithoutGuessing(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	os.MkdirAll(filepath.Join(getWorkerDataDir(), "updates"), 0755)
	srv := ackingCoordinator(t)
	stubExit(t)

	primary := filepath.Join(tmp, "ForgeGrid.exe")
	original := []byte("mystery-bytes-matching-nothing")
	os.WriteFile(primary, original, 0755)
	// No BackupBinaryPath file at all - the worst case.

	tx := &UpdateTransaction{
		ID:               "tx-applying-ambiguous-no-backup",
		WorkerID:         "worker-1",
		CurrentState:     "APPLYING",
		OldBinaryPath:    primary,
		NewBinaryPath:    filepath.Join(tmp, "candidate-irrelevant"),
		BackupBinaryPath: filepath.Join(tmp, "backup-that-was-never-written"),
		OldSHA256:        "4444444444444444444444444444444444444444444444444444444444444",
		ExpectedSHA256:   "5555555555555555555555555555555555555555555555555555555555555",
		LifecycleMode:    "portable",
	}
	writeTx(tx)

	w := &Worker{WorkerID: "worker-1", Token: "t", CoordinatorURL: srv.URL, Client: &http.Client{}}
	w.verifyUpdateTransaction()

	final, err := readTx()
	if err != nil {
		t.Fatalf("failed to read tx: %v", err)
	}
	if final.CurrentState != "ROLLBACK_FAILED" {
		t.Fatalf("expected CurrentState=ROLLBACK_FAILED when even the backup is missing, got %q", final.CurrentState)
	}
	// Must not have touched the (untrusted, unverifiable) primary binary
	// at all rather than guess it is safe to leave or overwrite.
	if b, _ := os.ReadFile(primary); string(b) != string(original) {
		t.Fatalf("must not modify an unverifiable primary binary when no known-good backup exists")
	}
}

func TestRecoverStagedOrApplyingIsIdempotentAcrossRepeatedRestarts(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	os.MkdirAll(filepath.Join(getWorkerDataDir(), "updates"), 0755)
	srv := ackingCoordinator(t)
	stubExit(t)

	exe, _ := os.Executable()
	oldHash, _ := fileSHA256(exe)

	candidate := filepath.Join(tmp, "staged-candidate")
	os.WriteFile(candidate, []byte("candidate-bytes"), 0755)
	newHash, _ := fileSHA256(candidate)
	helper := filepath.Join(tmp, "helper")
	os.WriteFile(helper, []byte("helper"), 0755)

	var helperCalls int
	originalLaunch := launchUpdaterHelper
	launchUpdaterHelper = func(path string) error { helperCalls++; return nil }
	t.Cleanup(func() { launchUpdaterHelper = originalLaunch })

	tx := &UpdateTransaction{
		ID:                "tx-staged-idempotent",
		WorkerID:          "worker-1",
		CurrentState:      "STAGED",
		OldBinaryPath:     exe,
		NewBinaryPath:     candidate,
		UpdaterHelperPath: helper,
		OldSHA256:         oldHash,
		ExpectedSHA256:    newHash,
		LifecycleMode:     "portable",
	}
	writeTx(tx)

	w := &Worker{WorkerID: "worker-1", Token: "t", CoordinatorURL: srv.URL, Client: &http.Client{}}
	// Simulate the process being restarted three times in a row before the
	// relaunched helper ever gets a chance to actually run (osExit is
	// stubbed, so control returns here each time) - each call must behave
	// identically and safely, not compound or corrupt anything.
	w.verifyUpdateTransaction()
	w.verifyUpdateTransaction()
	w.verifyUpdateTransaction()

	if helperCalls != 3 {
		t.Fatalf("expected each of the 3 restarts to independently and safely relaunch the helper, got %d calls", helperCalls)
	}
	if b, _ := os.ReadFile(candidate); string(b) != "candidate-bytes" {
		t.Fatalf("repeated STAGED recovery must never touch the staged candidate")
	}
	if h, _ := fileSHA256(exe); h != oldHash {
		t.Fatalf("repeated STAGED recovery must never touch the running binary")
	}
}
