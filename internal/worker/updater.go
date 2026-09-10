package worker

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"forgegrid/internal/network"
)

type UpdateTransaction struct {
	ID                string    `json:"transaction_id"`
	WorkerID          string    `json:"worker_id"`
	OldBinaryPath     string    `json:"old_binary_path"`
	NewBinaryPath     string    `json:"new_binary_path"`
	BackupBinaryPath  string    `json:"backup_binary_path"`
	UpdaterHelperPath string    `json:"updater_helper_path"`
	ExpectedSHA256    string    `json:"expected_sha256"`
	OldSHA256         string    `json:"old_sha256"`
	CurrentState      string    `json:"current_state"`
	StartedAt         time.Time `json:"started_at"`
	RestartDeadline   time.Time `json:"restart_deadline"`
	RollbackReason    string    `json:"rollback_reason"`
	WorkerPID         int       `json:"worker_pid"`
	LifecycleMode     string    `json:"lifecycle_mode"`
}

// launchUpdaterHelper starts the standalone updater helper binary (a copy
// of the worker executable, made before the swap so it survives the swap
// independently of whichever file is currently at OldBinaryPath) in
// "-mode update-helper", which runs RunUpdater(). It is a package variable
// so tests can substitute a stub instead of spawning a real process.
var launchUpdaterHelper = func(path string) error {
	cmd := exec.Command(path, "-mode", "update-helper")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}

func getTxPath() string {
	return filepath.Join(getWorkerDataDir(), "update_tx.json")
}

func readTx() (*UpdateTransaction, error) {
	b, err := os.ReadFile(getTxPath())
	if err != nil {
		return nil, err
	}
	var tx UpdateTransaction
	if err := json.Unmarshal(b, &tx); err != nil {
		return nil, err
	}
	return &tx, nil
}

func writeTx(tx *UpdateTransaction) error {
	b, err := json.MarshalIndent(tx, "", "  ")
	if err != nil {
		return err
	}
	tmp := getTxPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	err = os.Rename(tmp, getTxPath())
	if err == nil {
		reportTxState(tx)
	}
	return err
}

func reportTxState(tx *UpdateTransaction) {
	b, err := os.ReadFile(getWorkerCredsPath())
	if err != nil {
		return
	}
	var creds WorkerCredentials
	if err := json.Unmarshal(b, &creds); err != nil {
		return
	}

	payload := map[string]interface{}{
		"worker_id": tx.WorkerID,
		"update_id": tx.ID,
		"status":    strings.ToLower(tx.CurrentState),
		"message":   "Update state changed to " + tx.CurrentState,
	}
	pb, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", creds.CoordinatorURL+"/api/updates/report", bytes.NewReader(pb))
	if err == nil {
		req.Header.Set("Authorization", "Bearer "+creds.Token)
		client := &http.Client{Timeout: 5 * time.Second}
		// Match the same TLS model the rest of the worker uses (see
		// Worker.SetupClient): a bare default client verifies against the
		// system CA pool, which a self-signed coordinator certificate will
		// never pass, so every one of these state-change reports would
		// silently fail the TLS handshake whenever the worker is running
		// in its normal, non-insecure, fingerprint-pinned configuration -
		// only the deliberately-insecure case ever actually reached the
		// coordinator.
		if creds.Insecure {
			client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
		} else if creds.Fingerprint != "" {
			client.Transport = &http.Transport{TLSClientConfig: network.PinTLSConfig(creds.Fingerprint)}
		}
		client.Do(req)
	}
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func processIsRunning(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 is safe on Unix. On Windows, FindProcess always succeeds, we can test it by just waiting slightly.
	// But actually, we don't need a perfectly robust check, just polling or waiting enough time.
	// We will wait up to 10 seconds.
	_ = p
	return false // Simplified for cross-platform, we'll just wait
}

func RunUpdater() {
	tx, err := readTx()
	if err != nil {
		log.Fatalf("Failed to read transaction: %v", err)
	}

	// Give the old worker a few seconds to exit gracefully
	time.Sleep(3 * time.Second)

	if tx.CurrentState == "STAGED" {
		tx.CurrentState = "APPLYING"
		if err := writeTx(tx); err != nil {
			// Nothing destructive has happened yet - the on-disk
			// transaction still honestly says STAGED (writeTx's
			// write-to-.tmp-then-rename never touches the original file
			// on failure), so it is safer to stop here than to swap
			// binaries with no durable record of having started.
			log.Printf("[Update] Could not persist APPLYING state; aborting before touching any binaries: %v", err)
			osExit(1)
			return
		}

		log.Printf("[Update] Swapping binaries...")
		if err := swapBinaries(tx); err != nil {
			tx.RollbackReason = "Swap failed: " + err.Error()
			rollback(tx)
			return
		}

		tx.CurrentState = "RESTARTING"
		if err := writeTx(tx); err != nil {
			// The binaries are already swapped at this point: we cannot
			// pretend that never happened, so this must go through
			// rollback() rather than just logging and continuing as if
			// the state had been durably recorded.
			tx.RollbackReason = "Could not persist RESTARTING state after swap: " + err.Error()
			rollback(tx)
			return
		}

		log.Printf("[Update] Starting candidate worker...")
		if err := GetLifecycle(tx.LifecycleMode).Start(tx); err != nil {
			tx.RollbackReason = "Start failed: " + err.Error()
			rollback(tx)
			return
		}

		tx.CurrentState = "VERIFYING_NEW_WORKER"
		if err := writeTx(tx); err != nil {
			tx.RollbackReason = "Could not persist VERIFYING_NEW_WORKER state after starting candidate: " + err.Error()
			rollback(tx)
			return
		}
	}

	if tx.CurrentState == "VERIFYING_NEW_WORKER" {
		log.Printf("[Update] Waiting for health verification from candidate...")
		if err := waitForHealth(tx); err != nil {
			tx.RollbackReason = "Health check failed: " + err.Error()
			rollback(tx)
			return
		}
		// Candidate worker has marked tx as COMPLETED
		log.Printf("[Update] Candidate verified and update completed successfully!")
		os.Exit(0)
	}

	if tx.CurrentState == "ROLLING_BACK" || tx.CurrentState == "ROLLED_BACK" {
		rollback(tx)
	}
}

// safeReplace puts the file at newPath into place at destPath. A direct
// os.Rename(newPath, destPath) can fail on Windows with
// ERROR_SHARING_VIOLATION/ERROR_ACCESS_DENIED if anything (a lingering AV
// scan, a not-yet-released handle from the process that just exited) still
// holds destPath open, even briefly. Moving the existing file out of the
// way first, then renaming the replacement in, avoids requiring a
// rename-over-existing to succeed at all; if the final rename fails, the
// original file is restored so destPath is never left missing.
var safeReplace = func(newPath, destPath string) error {
	replacedPath := destPath + ".replaced"
	hadExisting := false
	if _, err := os.Stat(destPath); err == nil {
		hadExisting = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", destPath, err)
	}
	if hadExisting {
		if err := os.Remove(replacedPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("clear stale %s: %w", replacedPath, err)
		}
		if err := os.Rename(destPath, replacedPath); err != nil {
			return fmt.Errorf("move existing %s aside: %w", destPath, err)
		}
	}
	if err := os.Rename(newPath, destPath); err != nil {
		if hadExisting {
			if restoreErr := os.Rename(replacedPath, destPath); restoreErr != nil {
				return fmt.Errorf("rename %s to %s failed (%v), and restoring the original failed too: %w", newPath, destPath, err, restoreErr)
			}
		}
		return fmt.Errorf("rename %s to %s: %w", newPath, destPath, err)
	}
	if err := os.Chmod(destPath, 0755); err != nil {
		return fmt.Errorf("chmod %s: %w", destPath, err)
	}
	if hadExisting {
		if err := os.Remove(replacedPath); err != nil {
			return fmt.Errorf("remove stale %s after successful replace: %w", replacedPath, err)
		}
	}
	return nil
}

func swapBinaries(tx *UpdateTransaction) error {
	// stageUpdate already made a verified backup copy of the primary before
	// the old process exited, so it's safe to move the primary itself aside
	// here rather than deleting/overwriting it in place.
	return safeReplace(tx.NewBinaryPath, tx.OldBinaryPath)
}

func startCandidateWorker(tx *UpdateTransaction) error {
	return GetLifecycle(tx.LifecycleMode).Start(tx)
}

func waitForHealth(tx *UpdateTransaction) error {
	deadline := tx.RestartDeadline
	if deadline.IsZero() {
		deadline = time.Now().Add(45 * time.Second)
	}
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		current, err := readTx()
		if err != nil {
			continue
		}
		if current.CurrentState == "COMPLETED" {
			return nil
		}
		// The candidate worker itself already detected a failed
		// verification and triggered its own rollback (see
		// Worker.failCandidateVerification): stop waiting out the rest of
		// this deadline and let the caller's rollback() call become a
		// no-op against already-resolved state, rather than silently
		// racing a second safeReplace against the one already underway.
		if current.CurrentState == "ROLLING_BACK" || current.CurrentState == "ROLLED_BACK" || current.CurrentState == "ROLLBACK_FAILED" {
			return fmt.Errorf("candidate already triggered its own rollback: %s", current.RollbackReason)
		}
	}
	return fmt.Errorf("timeout waiting for healthy heartbeat from new worker")
}

func generateRandomID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

var stealWaitDuration = 10 * time.Second

func rollback(tx *UpdateTransaction) {
	if current, err := readTx(); err == nil && current.ID == tx.ID {
		switch current.CurrentState {
		case "ROLLED_BACK", "ROLLBACK_FAILED", "COMPLETED":
			log.Printf("[Update] Rollback already resolved (state=%s); skipping duplicate rollback", current.CurrentState)
			return
		}
	}

	myID := generateRandomID()
	t1Base := tx.BackupBinaryPath
	t2Base := filepath.Join(getWorkerDataDir(), "rollback_restart_"+tx.ID)

	phase1Done := false

	// PHASE 1: BINARY RESTORE
	for {
		hasBase := false
		if _, err := os.Stat(t1Base); err == nil {
			hasBase = true
		}

		existingClaims, _ := filepath.Glob(t1Base + ".claim.*")

		if !hasBase && len(existingClaims) == 0 {
			phase1Done = true
			break
		}

		myClaim := t1Base + ".claim." + myID
		claimed := false

		if hasBase {
			if err := os.Rename(t1Base, myClaim); err == nil {
				claimed = true
			}
		} else if len(existingClaims) > 0 {
			log.Printf("[Update] Phase 1 token claimed by another actor. Waiting to steal...")
			time.Sleep(stealWaitDuration)
			if err := os.Rename(existingClaims[0], myClaim); err == nil {
				claimed = true
				log.Printf("[Update] Stole Phase 1 token.")
			}
		}

		if claimed {
			// Prepare Phase 2 token BEFORE consuming Phase 1 token
			os.WriteFile(t2Base+".pending", []byte{}, 0644)

			log.Printf("[Update] Rolling back: %s", tx.RollbackReason)
			tx.CurrentState = "ROLLING_BACK"
			logWriteTxErr(tx, "ROLLING_BACK")

			// Consume Phase 1 token
			if err := safeReplace(myClaim, tx.OldBinaryPath); err != nil {
				if os.IsNotExist(err) {
					log.Printf("[Update] Fenced out of Phase 1! Retrying discovery.")
					continue
				}
				tx.CurrentState = "ROLLBACK_FAILED"
				tx.RollbackReason = tx.RollbackReason + " | restore from backup failed: " + err.Error()
				logWriteTxErr(tx, "ROLLBACK_FAILED (restore)")
				log.Printf("[Update] Rollback FAILED to restore the previous binary: %v", err)
				return
			}
			phase1Done = true
			break
		}
	}

	if !phase1Done {
		return
	}

	// PHASE 2: WORKER RESTART
	phase2Done := false
	for {
		if _, err := os.Stat(t2Base + ".consumed"); err == nil {
			phase2Done = true
			break
		}

		hasPending := false
		if _, err := os.Stat(t2Base + ".pending"); err == nil {
			hasPending = true
		}

		existingClaims, _ := filepath.Glob(t2Base + ".claim.*")

		if !hasPending && len(existingClaims) == 0 {
			tx.CurrentState = "ROLLBACK_FAILED"
			tx.RollbackReason = tx.RollbackReason + " | restore from backup failed: backup binary missing"
			logWriteTxErr(tx, "ROLLBACK_FAILED (missing backup)")
			return
		}

		myClaim := t2Base + ".claim." + myID
		claimed := false

		if hasPending {
			if err := os.Rename(t2Base+".pending", myClaim); err == nil {
				claimed = true
			}
		} else if len(existingClaims) > 0 {
			log.Printf("[Update] Phase 2 token claimed by another actor. Waiting to steal...")
			time.Sleep(stealWaitDuration)
			if err := os.Rename(existingClaims[0], myClaim); err == nil {
				claimed = true
				log.Printf("[Update] Stole Phase 2 token.")
			}
		}

		if claimed {
			// Consume Phase 2 token
			if err := os.Rename(myClaim, t2Base+".consumed"); err != nil {
				if os.IsNotExist(err) {
					log.Printf("[Update] Fenced out of Phase 2! Retrying discovery.")
					continue
				}
				tx.CurrentState = "ROLLBACK_FAILED"
				tx.RollbackReason = tx.RollbackReason + " | could not consume restart token: " + err.Error()
				logWriteTxErr(tx, "ROLLBACK_FAILED (token error)")
				return
			}

			// Authorized to restart exactly once
			log.Printf("[Update] Restarting previous worker...")
			if err := GetLifecycle(tx.LifecycleMode).Start(tx); err != nil {
				tx.CurrentState = "ROLLBACK_FAILED"
				tx.RollbackReason = tx.RollbackReason + " | restart of restored binary failed: " + err.Error()
				logWriteTxErr(tx, "ROLLBACK_FAILED (restart)")
				log.Printf("[Update] Rollback FAILED to restart the previous worker: %v", err)
				return
			}
			phase2Done = true
			break
		}
	}

	if phase2Done {
		tx.CurrentState = "ROLLED_BACK"
		logWriteTxErr(tx, "ROLLED_BACK")
		log.Printf("[Update] Rollback initiated. Waiting for previous worker to verify...")
	}
}

// logWriteTxErr persists tx and, on failure, logs it clearly rather than
// silently discarding the error - a transactional updater must not
// pretend state was durably recorded when the write itself failed. The
// destructive file operations around each of these call sites still need
// to proceed regardless (leaving a backup un-restored because we also
// couldn't write a status file would be worse), so this only logs; it
// does not change control flow the way the STAGED-branch checks in
// RunUpdater do, since those run before anything destructive has
// happened and can safely abort instead.
func logWriteTxErr(tx *UpdateTransaction, context string) {
	if err := writeTx(tx); err != nil {
		log.Printf("[Update] Could not persist transaction state (%s): %v", context, err)
	}
}

type WorkerStatus struct {
	State         string `json:"state"`
	TransactionID string `json:"transaction_id"`
	Message       string `json:"message"`
}

func readStatus() (*WorkerStatus, error) {
	b, err := os.ReadFile(WorkerStatusPath())
	if err != nil {
		return nil, err
	}
	var st WorkerStatus
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	return &st, nil
}
